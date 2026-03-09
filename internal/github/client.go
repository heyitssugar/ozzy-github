package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gwen001/github-subdomains/internal/token"
)

// Client wraps HTTP interactions with the GitHub API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	tokenMgr   *token.Manager
	logger     *slog.Logger
	delay      time.Duration // minimum delay between search API calls

	// Rate spreading state — evenly distributes requests across the rate window
	rateMu       sync.Mutex
	lastCall     time.Time         // timestamp of last search API call
	optimalDelay time.Duration     // computed delay from rate limit headers
	resetAt      time.Time         // when the current rate window resets
	remaining    int               // remaining requests in current window
}

// ClientOption configures the Client.
type ClientOption func(*Client)

// WithBaseURL sets a custom GitHub API base URL (for GitHub Enterprise).
func WithBaseURL(u string) ClientOption {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(u, "/")
	}
}

// WithProxy configures an HTTP/SOCKS5 proxy.
func WithProxy(proxyURL string) ClientOption {
	return func(c *Client) {
		if proxyURL == "" {
			return
		}
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return
		}
		transport := &http.Transport{
			Proxy: http.ProxyURL(parsed),
		}
		c.httpClient.Transport = transport
	}
}

// WithTimeout sets the HTTP request timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithLogger sets the structured logger.
func WithLogger(l *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

// WithDelay sets the delay between API requests.
func WithDelay(d time.Duration) ClientOption {
	return func(c *Client) {
		c.delay = d
	}
}

// NewClient creates a new GitHub API client.
func NewClient(tokenMgr *token.Manager, opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    "https://api.github.com",
		tokenMgr:   tokenMgr,
		logger:     slog.Default(),
		delay:      200 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// SearchCode performs a GitHub code search with text_match support for better extraction.
func (c *Client) SearchCode(ctx context.Context, query SearchQuery, page int) (*SearchResponse, error) {
	search := query.Keyword
	if query.Language != "" {
		search = fmt.Sprintf("%s+language:%s", search, query.Language)
	}
	if len(query.Noise) > 0 {
		search = fmt.Sprintf("%s+%s", search, strings.Join(query.Noise, "+"))
	}

	apiURL := fmt.Sprintf("%s/search/code?per_page=100&s=%s&type=Code&o=%s&q=%s&page=%d",
		c.baseURL, query.Sort, query.Order, search, page)

	// Use text-match+json Accept header to get text_matches with highlighted fragments
	return c.doSearchWithAccept(ctx, apiURL, "application/vnd.github.v3.text-match+json")
}

// SearchCommits performs a GitHub commit search.
func (c *Client) SearchCommits(ctx context.Context, query SearchQuery, page int) (*SearchResponse, error) {
	search := query.Keyword

	apiURL := fmt.Sprintf("%s/search/commits?per_page=100&sort=%s&order=%s&q=%s&page=%d",
		c.baseURL, query.Sort, query.Order, search, page)

	// Commit search needs cloak-preview, combine with text-match
	return c.doSearchWithAccept(ctx, apiURL, "application/vnd.github.cloak-preview+json,application/vnd.github.v3.text-match+json")
}

// SearchIssues performs a GitHub issue/PR search with text_match support.
func (c *Client) SearchIssues(ctx context.Context, query SearchQuery, page int) (*SearchResponse, error) {
	search := query.Keyword

	apiURL := fmt.Sprintf("%s/search/issues?per_page=100&sort=%s&order=%s&q=%s&page=%d",
		c.baseURL, query.Sort, query.Order, search, page)

	return c.doSearchWithAccept(ctx, apiURL, "application/vnd.github.v3.text-match+json")
}

// GetRawContent fetches the raw file content from a GitHub HTML URL with authentication.
func (c *Client) GetRawContent(ctx context.Context, htmlURL string) (string, error) {
	rawURL := htmlToRawURL(htmlURL)

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request for %s: %w", rawURL, err)
	}

	// Authenticate raw content fetches to avoid rate limits on raw.githubusercontent.com
	tok, tokErr := c.tokenMgr.Next()
	if tokErr == nil {
		req.Header.Set("Authorization", "token "+tok)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching raw content %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("raw content %s returned status %d", rawURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading raw content: %w", err)
	}

	return string(body), nil
}

// Delay returns the configured base delay between requests.
func (c *Client) Delay() time.Duration {
	return c.delay
}

// WaitForRateLimit uses rate spreading to evenly distribute requests across
// the GitHub rate limit window. Instead of bursting then waiting 40s, we space
// requests optimally so we never actually hit the hard limit.
//
// Strategy: after each response, handleRateLimitHeaders computes
// optimalDelay = timeUntilReset / remainingRequests. We wait that long
// before the next request, plus small jitter to avoid patterns.
func (c *Client) WaitForRateLimit(ctx context.Context) {
	c.rateMu.Lock()

	// Use the optimal delay from rate limit headers, falling back to base delay
	interval := c.optimalDelay
	if interval < c.delay {
		interval = c.delay
	}

	elapsed := time.Since(c.lastCall)
	c.rateMu.Unlock()

	if elapsed >= interval {
		c.rateMu.Lock()
		c.lastCall = time.Now()
		c.rateMu.Unlock()
		return
	}

	wait := interval - elapsed
	// Add small jitter (5-15% of wait) to avoid burst detection
	if wait > 0 {
		jitterPct := 5 + rand.Intn(11) // 5-15%
		jitter := time.Duration(int64(wait) * int64(jitterPct) / 100)
		wait += jitter
	}

	c.logger.Debug("rate spreading wait", "wait", wait.Round(time.Millisecond), "interval", interval.Round(time.Millisecond))

	select {
	case <-ctx.Done():
		return
	case <-time.After(wait):
	}

	c.rateMu.Lock()
	c.lastCall = time.Now()
	c.rateMu.Unlock()
}

// doSearch performs an authenticated GitHub API search with retry logic.
func (c *Client) doSearch(ctx context.Context, apiURL string) (*SearchResponse, error) {
	return c.doSearchWithAccept(ctx, apiURL, "")
}

func (c *Client) doSearchWithAccept(ctx context.Context, apiURL string, accept string) (*SearchResponse, error) {
	const maxRetries = 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		tok, err := c.tokenMgr.Next()
		if err != nil {
			return nil, fmt.Errorf("getting token: %w", err)
		}

		c.logger.Debug("API request", "url", apiURL, "attempt", attempt+1)

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Authorization", "token "+tok)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.logger.Warn("request failed", "error", err, "attempt", attempt+1)
			backoff := time.Duration(attempt+1) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		// Read rate limit headers for adaptive timing
		c.handleRateLimitHeaders(resp, tok)

		// Handle HTTP status codes
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			c.logger.Warn("bad credentials, removing token")
			c.tokenMgr.Remove(tok)
			continue

		case resp.StatusCode == http.StatusForbidden:
			// Check if this is a rate limit (403) or secondary rate limit
			resetHeader := resp.Header.Get("X-RateLimit-Reset")
			if resetHeader != "" {
				resetTime, _ := strconv.ParseInt(resetHeader, 10, 64)
				waitDuration := time.Until(time.Unix(resetTime, 0))
				if waitDuration > 0 && waitDuration < 120*time.Second {
					c.logger.Warn("rate limited, waiting for reset",
						"wait", waitDuration, "token_suffix", tok[len(tok)-4:])
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(waitDuration + time.Second):
					}
					continue
				}
			}
			// Secondary rate limit or abuse detection — longer backoff
			retryAfter := resp.Header.Get("Retry-After")
			if retryAfter != "" {
				if secs, err := strconv.Atoi(retryAfter); err == nil {
					wait := time.Duration(secs) * time.Second
					c.logger.Warn("secondary rate limit, waiting", "wait", wait)
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(wait):
					}
					continue
				}
			}
			c.logger.Warn("rate limited, disabling token", "token_suffix", tok[len(tok)-4:])
			c.tokenMgr.Disable(tok)
			continue

		case resp.StatusCode == http.StatusUnprocessableEntity:
			// 422 can be transient — retry once with backoff
			if attempt == 0 {
				c.logger.Warn("validation error (422), retrying", "body", string(body))
				time.Sleep(2 * time.Second)
				continue
			}
			return nil, fmt.Errorf("validation error (422): %s", string(body))

		case resp.StatusCode >= 500:
			c.logger.Warn("server error", "status", resp.StatusCode, "attempt", attempt+1)
			backoff := time.Duration(attempt+1) * 2 * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			continue

		case resp.StatusCode != http.StatusOK:
			return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		}

		var result SearchResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("parsing response: %w", err)
		}

		// Check for API error messages in the response body
		if result.Message != "" {
			if strings.HasPrefix(result.Message, "Only the first") {
				c.logger.Debug("search result limit reached")
				return &result, nil
			}
			if strings.HasPrefix(result.Message, "Bad credentials") {
				c.tokenMgr.Remove(tok)
				continue
			}
			if strings.HasPrefix(result.Message, "You have triggered an abuse detection mechanism") {
				c.logger.Warn("abuse detection triggered, backing off 60s")
				c.tokenMgr.Disable(tok)
				// Wait 60s before retrying with any token — abuse detection is global
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(60 * time.Second):
				}
				continue
			}
		}

		return &result, nil
	}

	return nil, fmt.Errorf("max retries (%d) exceeded for %s", maxRetries, apiURL)
}

// handleRateLimitHeaders implements rate spreading by computing the optimal
// delay between requests based on GitHub's rate limit headers.
//
// Formula: optimalDelay = timeUntilReset / (remaining + 1)
//
// This distributes remaining requests evenly across the rate window.
// Example: 8 remaining, 50s until reset → 50/9 ≈ 5.5s between requests
// Result: steady 10 req/min throughput with zero 403 errors.
func (c *Client) handleRateLimitHeaders(resp *http.Response, tok string) {
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	resetHeader := resp.Header.Get("X-RateLimit-Reset")

	if remaining == "" || resetHeader == "" {
		return
	}

	rem, err := strconv.Atoi(remaining)
	if err != nil {
		return
	}

	resetTime, err := strconv.ParseInt(resetHeader, 10, 64)
	if err != nil {
		return
	}

	timeUntilReset := time.Until(time.Unix(resetTime, 0))
	if timeUntilReset <= 0 {
		// Window already reset — clear any delay
		c.rateMu.Lock()
		c.optimalDelay = c.delay
		c.remaining = rem
		c.resetAt = time.Unix(resetTime, 0)
		c.rateMu.Unlock()
		return
	}

	// Spread remaining requests evenly across the window.
	// +1 because we need spacing between requests, not after the last one.
	var optimal time.Duration
	if rem > 0 {
		optimal = timeUntilReset / time.Duration(rem+1)
	} else {
		// No requests remaining — wait for reset
		optimal = timeUntilReset + time.Second
	}

	// Floor at base delay, cap at 2 minutes
	if optimal < c.delay {
		optimal = c.delay
	}
	if optimal > 2*time.Minute {
		optimal = 2 * time.Minute
	}

	c.rateMu.Lock()
	c.optimalDelay = optimal
	c.remaining = rem
	c.resetAt = time.Unix(resetTime, 0)
	c.rateMu.Unlock()

	c.logger.Debug("rate spreading",
		"remaining", rem,
		"reset_in", timeUntilReset.Round(time.Second),
		"optimal_delay", optimal.Round(time.Millisecond),
		"token_suffix", tok[len(tok)-4:],
	)
}

// htmlToRawURL converts a GitHub HTML URL to a raw.githubusercontent.com URL.
func htmlToRawURL(htmlURL string) string {
	raw := strings.Replace(htmlURL, "https://github.com/", "https://raw.githubusercontent.com/", 1)
	raw = strings.Replace(raw, "/blob/", "/", 1)
	return raw
}
