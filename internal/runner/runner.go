package runner

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/gwen001/github-subdomains/internal/checkpoint"
	"github.com/gwen001/github-subdomains/internal/config"
	"github.com/gwen001/github-subdomains/internal/dedup"
	"github.com/gwen001/github-subdomains/internal/extractor"
	"github.com/gwen001/github-subdomains/internal/github"
	"github.com/gwen001/github-subdomains/internal/output"
	"github.com/gwen001/github-subdomains/internal/token"
)

// Runner orchestrates the entire subdomain discovery process.
type Runner struct {
	config       *config.Config
	client       *github.Client
	tokenMgr     *token.Manager
	extractor    extractor.Extractor
	urlSet       *dedup.Set
	subdomainSet *dedup.Set
	writer       output.Writer
	logger       *slog.Logger
	progress     *output.ProgressTracker
	searchIndex  int
}

// New creates a new Runner with all dependencies.
func New(
	cfg *config.Config,
	client *github.Client,
	tokenMgr *token.Manager,
	ext extractor.Extractor,
	writer output.Writer,
	logger *slog.Logger,
) *Runner {
	return &Runner{
		config:       cfg,
		client:       client,
		tokenMgr:     tokenMgr,
		extractor:    ext,
		urlSet:       dedup.New(),
		subdomainSet: dedup.New(),
		writer:       writer,
		logger:       logger,
	}
}

// Run executes the subdomain discovery.
func (r *Runner) Run(ctx context.Context) error {
	// Load checkpoint if resuming
	if r.config.ResumeFile != "" {
		if err := r.loadCheckpoint(); err != nil {
			r.logger.Warn("could not load checkpoint, starting fresh", "error", err)
		}
	}

	// Setup auto-save checkpoint
	checkpointPath := checkpoint.DefaultPath(r.config.Domain)
	cpCtx, cpCancel := context.WithCancel(ctx)
	defer cpCancel()
	go checkpoint.AutoSave(cpCtx, checkpointPath, 30*time.Second, r.buildCheckpointState)

	// Build search queries from all strategies
	encodedDomain := github.EncodeDomain(r.config.Domain)
	strategies := github.GetStrategies(r.config.SearchSources)

	var allQueries []github.SearchQuery
	searchSigs := dedup.New()

	tokenCount := r.tokenMgr.Total()
	for _, strategy := range strategies {
		queries := strategy.BuildQueries(encodedDomain, github.SearchOptions{
			Languages:  r.config.Languages,
			Noise:      r.config.Noise,
			QuickMode:  r.config.QuickMode,
			Extend:     r.config.ExtendMode,
			TokenCount: tokenCount,
		})
		for _, q := range queries {
			if q.Signature == "" {
				q.Signature = github.GenerateSignature(q)
			}
			if searchSigs.Add(q.Signature) {
				allQueries = append(allQueries, q)
			}
		}
	}

	// Sort queries by priority — high-value queries run first
	sort.Slice(allQueries, func(i, j int) bool {
		return allQueries[i].Priority < allQueries[j].Priority
	})

	// Token-aware query limiting: with few tokens, skip low-priority queries
	maxPriority := github.PriorityLow
	if tokenCount <= 1 {
		maxPriority = github.PriorityMedium // Skip Priority 3 with 1 token
	}
	filtered := allQueries[:0]
	for _, q := range allQueries {
		if q.Priority <= maxPriority {
			filtered = append(filtered, q)
		}
	}
	allQueries = filtered

	// Skip already-processed searches from checkpoint
	if r.searchIndex > 0 && r.searchIndex < len(allQueries) {
		allQueries = allQueries[r.searchIndex:]
	}

	r.progress = output.NewProgressTracker(len(allQueries), r.config.RawOutput)
	defer r.progress.Finish()

	r.logger.Info("starting scan",
		"domain", r.config.Domain,
		"tokens", tokenCount,
		"searches", len(allQueries),
		"sources", r.config.SearchSources,
		"concurrency", r.config.Concurrency,
	)

	// Early termination tracking: stop when too many consecutive queries find nothing new
	const dryStreakLimit = 25 // Stop after 25 consecutive empty queries
	dryStreak := 0
	prevFound := 0

	// Process each search query
	for i, query := range allQueries {
		if ctx.Err() != nil {
			break
		}

		// Early termination: if many consecutive queries found nothing new, stop
		if dryStreak >= dryStreakLimit && r.subdomainSet.Len() > 0 {
			r.logger.Info("early termination: no new results in last consecutive queries",
				"dry_streak", dryStreak,
				"total_found", r.subdomainSet.Len(),
			)
			break
		}

		r.searchIndex = i
		r.logger.Debug("executing search",
			"keyword", query.Keyword,
			"sort", query.Sort,
			"order", query.Order,
			"language", query.Language,
			"noise", query.Noise,
			"priority", query.Priority,
			"search_num", i+1,
			"total", len(allQueries),
		)

		newQueries, err := r.executeSearch(ctx, query, searchSigs)
		if err != nil {
			r.logger.Error("search failed", "error", err)

			// Save checkpoint on error so no progress is lost
			if state := r.buildCheckpointState(); state != nil {
				_ = checkpoint.Save(checkpointPath, state)
			}

			// On token exhaustion, either exit or wait
			if r.config.StopOnNoToken {
				return fmt.Errorf("no tokens available: %w", err)
			}
			// Wait for a token to become available
			r.logger.Info("waiting for token to become available...")
			if waitErr := r.tokenMgr.WaitForAvailable(ctx); waitErr != nil {
				return waitErr
			}
			continue
		}

		// Track dry streak for early termination
		currentFound := r.subdomainSet.Len()
		if currentFound > prevFound {
			dryStreak = 0
		} else {
			dryStreak++
		}
		prevFound = currentFound

		// Add dynamically generated sub-queries (language/noise splits)
		for _, nq := range newQueries {
			if searchSigs.Add(nq.Signature) {
				allQueries = append(allQueries, nq)
			}
		}
		r.progress.UpdateTotal(len(allQueries))
		r.progress.Increment()
		r.progress.SetFound(r.subdomainSet.Len())
	}

	// Final checkpoint save
	if state := r.buildCheckpointState(); state != nil {
		_ = checkpoint.Save(checkpointPath, state)
	}

	r.logger.Info("scan complete",
		"searches_performed", len(allQueries),
		"subdomains_found", r.subdomainSet.Len(),
	)

	return nil
}

// executeSearch runs a single search query across all pages and processes results.
func (r *Runner) executeSearch(ctx context.Context, query github.SearchQuery, searchSigs *dedup.Set) ([]github.SearchQuery, error) {
	var newQueries []github.SearchQuery
	maxPage := 1

	for page := 1; page <= maxPage; page++ {
		if ctx.Err() != nil {
			return newQueries, ctx.Err()
		}

		// Adaptive rate limiting — waits the right amount based on API headers and jitter
		r.client.WaitForRateLimit(ctx)

		// Route to correct search API based on SourceType
		var resp *github.SearchResponse
		var err error

		switch query.SourceType {
		case github.SourceCommit:
			resp, err = r.client.SearchCommits(ctx, query, page)
		case github.SourceIssue:
			resp, err = r.client.SearchIssues(ctx, query, page)
		default:
			resp, err = r.client.SearchCode(ctx, query, page)
		}

		if err != nil {
			return newQueries, err
		}

		if resp == nil {
			break
		}

		// On first page, determine pagination and refine searches
		if page == 1 {
			maxPage = int(math.Ceil(float64(resp.TotalCount) / 100.0))
			if maxPage > 10 {
				maxPage = 10
			}

			r.logger.Debug("search results",
				"total_count", resp.TotalCount,
				"pages", maxPage,
			)

			// If results exceed 1000 and not in quick mode, add refined searches
			if resp.TotalCount > 1000 && !r.config.QuickMode {
				// First try language splits (most effective)
				if query.Language == "" && len(r.config.Languages) > 0 {
					langQueries := github.BuildLanguageQueries(query, r.config.Languages)
					newQueries = append(newQueries, langQueries...)
					r.logger.Debug("added language-filtered searches", "count", len(langQueries))
				}
				// Also add noise splits (complementary — catches different result sets)
				if len(r.config.Noise) > 0 {
					noiseQueries := github.BuildNoiseQueries(query, r.config.Noise)
					newQueries = append(newQueries, noiseQueries...)
					r.logger.Debug("added noise-filtered searches", "count", len(noiseQueries))
				}
			}
		}

		// Check for search limit message
		if resp.Message != "" && strings.HasPrefix(resp.Message, "Only the first") {
			r.logger.Debug("search limit reached")
			break
		}

		// Process items concurrently
		r.processItems(ctx, resp.Items)

		// Small inter-page delay to avoid hammering when paginating
		if page < maxPage {
			select {
			case <-ctx.Done():
				return newQueries, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	return newQueries, nil
}

// processItems fetches raw content and extracts subdomains concurrently.
func (r *Runner) processItems(ctx context.Context, items []github.SearchItem) {
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(r.config.Concurrency)

	for _, item := range items {
		item := item // capture loop variable
		g.Go(func() error {
			r.processItem(gCtx, item)
			return nil // Don't propagate individual item errors
		})
	}

	_ = g.Wait()
}

// processItem handles a single search result item.
// Optimization: first extracts from text_matches (free, already in API response).
// Only fetches raw content if text_matches had no results — avoids wasting requests.
func (r *Runner) processItem(ctx context.Context, item github.SearchItem) {
	// Deduplicate URLs
	if !r.urlSet.Add(item.HTMLURL) {
		return
	}

	// Phase 1: Extract from text_matches (no extra API call needed)
	var textMatchContent []string
	for _, tm := range item.TextMatches {
		if tm.Fragment != "" {
			textMatchContent = append(textMatchContent, tm.Fragment)
		}
	}

	var allSubdomains []extractor.Subdomain
	if len(textMatchContent) > 0 {
		content := strings.Join(textMatchContent, "\n")
		allSubdomains = r.extractor.Extract(content, item.HTMLURL)
	}

	// Phase 2: Only fetch raw content if text_matches yielded nothing
	// This saves significant API calls — raw fetches are the biggest throughput cost.
	if len(allSubdomains) == 0 && item.Path != "" {
		rawContent, err := r.client.GetRawContent(ctx, item.HTMLURL)
		if err != nil {
			r.logger.Debug("failed to fetch raw content", "url", item.HTMLURL, "error", err)
		} else if rawContent != "" {
			allSubdomains = r.extractor.Extract(rawContent, item.HTMLURL)
		}
	}

	if len(allSubdomains) == 0 {
		return
	}

	printedSource := false
	for _, sub := range allSubdomains {
		if r.subdomainSet.Add(sub.Value) {
			if !printedSource {
				printedSource = true
				r.logger.Info("source", "url", item.HTMLURL)
			}

			r.logger.Info("found", "subdomain", sub.Value)

			result := output.Result{
				Subdomain: sub.Value,
				Source:    item.HTMLURL,
				FoundAt:  time.Now(),
			}

			if err := r.writer.Write(result); err != nil {
				r.logger.Error("write failed", "error", err)
			}
		}
	}
}

// loadCheckpoint restores state from a checkpoint file.
func (r *Runner) loadCheckpoint() error {
	state, err := checkpoint.Load(r.config.ResumeFile)
	if err != nil {
		return err
	}

	if state.Domain != r.config.Domain {
		return fmt.Errorf("checkpoint domain %q does not match target %q", state.Domain, r.config.Domain)
	}

	r.searchIndex = state.SearchIndex
	r.urlSet = dedup.NewFromSlice(state.CompletedURLs)
	r.subdomainSet = dedup.NewFromSlice(state.FoundSubdomains)

	r.logger.Info("resumed from checkpoint",
		"search_index", state.SearchIndex,
		"urls_processed", len(state.CompletedURLs),
		"subdomains_found", len(state.FoundSubdomains),
	)

	return nil
}

// buildCheckpointState creates a snapshot of current progress.
func (r *Runner) buildCheckpointState() *checkpoint.State {
	return &checkpoint.State{
		Domain:          r.config.Domain,
		SearchIndex:     r.searchIndex,
		CompletedURLs:   r.urlSet.Items(),
		FoundSubdomains: r.subdomainSet.Items(),
	}
}
