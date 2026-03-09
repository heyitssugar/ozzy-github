package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gwen001/github-subdomains/internal/config"
	"github.com/gwen001/github-subdomains/internal/extractor"
	"github.com/gwen001/github-subdomains/internal/github"
	"github.com/gwen001/github-subdomains/internal/output"
	"github.com/gwen001/github-subdomains/internal/runner"
	"github.com/gwen001/github-subdomains/internal/token"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	rootCmd := buildRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildRootCmd() *cobra.Command {
	var cfg config.Config
	var tokenRaw string

	cmd := &cobra.Command{
		Use:   "github-subdomains",
		Short: "Find subdomains on GitHub",
		Long: `github-subdomains searches GitHub repositories for subdomains of a target domain.

It leverages the GitHub Code Search, Commit Search, and Issue Search APIs to discover
subdomains exposed in publicly available source code, configuration files, commit messages,
and issue discussions.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, &cfg, tokenRaw)
		},
	}

	// Required flags
	cmd.Flags().StringVarP(&cfg.Domain, "domain", "d", "", "target domain (required)")
	_ = cmd.MarkFlagRequired("domain")

	// Token flags
	cmd.Flags().StringVarP(&tokenRaw, "token", "t", "", "GitHub token(s): single token, comma-separated, or file path")

	// Output flags
	cmd.Flags().StringVarP(&cfg.OutputPath, "output", "o", "", "output file (default: <domain>.txt)")
	cmd.Flags().StringVarP(&cfg.OutputFormat, "format", "f", "text", "output format: text, json, jsonl")
	cmd.Flags().BoolVar(&cfg.RawOutput, "raw", false, "raw output (subdomains only, no banner/colors)")

	// Mode flags
	cmd.Flags().BoolVarP(&cfg.ExtendMode, "extend", "e", false, "extended mode: also search for <prefix>domain variants")
	cmd.Flags().BoolVarP(&cfg.QuickMode, "quick", "q", false, "quick mode: skip language/noise filter refinement")
	cmd.Flags().BoolVarP(&cfg.StopOnNoToken, "kill-on-empty", "k", false, "exit when all tokens are disabled")

	// Performance flags
	cmd.Flags().IntVarP(&cfg.Concurrency, "concurrency", "c", 30, "max concurrent requests")
	cmd.Flags().DurationVar(&cfg.Timeout, "timeout", 10*time.Second, "HTTP request timeout")

	// Advanced flags
	cmd.Flags().StringVar(&cfg.ProxyURL, "proxy", "", "HTTP/SOCKS5 proxy URL")
	cmd.Flags().StringVar(&cfg.GitHubBaseURL, "github-url", "https://api.github.com", "GitHub API base URL")
	cmd.Flags().StringVar(&cfg.ResumeFile, "resume", "", "resume from checkpoint file")
	cmd.Flags().StringSliceVar(&cfg.SearchSources, "sources", []string{"code"}, "search sources: code,commits,issues")
	cmd.Flags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "verbose debug output")

	// Version command
	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("github-subdomains %s (commit: %s, built: %s)\n", version, commit, date)
		},
	})

	return cmd
}

func runSearch(cmd *cobra.Command, cfg *config.Config, tokenRaw string) error {
	// Load tokens
	cfg.Tokens = config.LoadTokens(tokenRaw)

	// Set defaults and validate
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// Setup logger
	logger := setupLogger(cfg.Verbose, cfg.RawOutput)

	// Print banner
	if !cfg.RawOutput {
		printBanner()
		printConfig(cfg, logger)
	}

	// Create token manager
	tokenMgr := token.NewManager(cfg.Tokens, 70*time.Second)
	if tokenMgr.Total() == 0 {
		return fmt.Errorf("no valid tokens found")
	}

	// Calculate delay based on token count
	nTokens := float64(tokenMgr.Total())
	delay := time.Duration(60.0/(30*nTokens)*1000+200) * time.Millisecond

	// Create GitHub client
	clientOpts := []github.ClientOption{
		github.WithBaseURL(cfg.GitHubBaseURL),
		github.WithTimeout(cfg.Timeout),
		github.WithLogger(logger),
		github.WithDelay(delay),
	}
	if cfg.ProxyURL != "" {
		clientOpts = append(clientOpts, github.WithProxy(cfg.ProxyURL))
	}
	client := github.NewClient(tokenMgr, clientOpts...)

	// Create extractor
	ext, err := extractor.NewRegexpExtractor(cfg.Domain, cfg.ExtendMode)
	if err != nil {
		return fmt.Errorf("creating extractor: %w", err)
	}

	// Create output writer
	writer, err := output.NewWriter(cfg.OutputPath, cfg.OutputFormat)
	if err != nil {
		return fmt.Errorf("creating output writer: %w", err)
	}
	defer writer.Close()

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down gracefully...")
		cancel()
	}()

	// Run
	r := runner.New(cfg, client, tokenMgr, ext, writer, logger)
	return r.Run(ctx)
}

func setupLogger(verbose, raw bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	if raw {
		// Discard all logging in raw mode
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.Level(100)}))
	}

	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Replace time with short format
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(time.Now().Format("15:04:05"))
			}
			return a
		},
	}

	return slog.New(NewColorHandler(os.Stderr, opts))
}

func printBanner() {
	fmt.Fprint(os.Stderr, `
   ▗▐  ▌     ▌          ▌    ▌          ▗
▞▀▌▄▜▀ ▛▀▖▌ ▌▛▀▖  ▞▀▘▌ ▌▛▀▖▞▀▌▞▀▖▛▚▀▖▝▀▖▄ ▛▀▖▞▀▘
▚▄▌▐▐ ▖▌ ▌▌ ▌▌ ▌  ▝▀▖▌ ▌▌ ▌▌ ▌▌ ▌▌▐ ▌▞▀▌▐ ▌ ▌▝▀▖
▗▄▘▀▘▀ ▘ ▘▝▀▘▀▀   ▀▀ ▝▀▘▀▀ ▝▀▘▝▀ ▘▝ ▘▝▀▘▀▘▘ ▘▀▀
`)
	fmt.Fprintf(os.Stderr, "       v%s by @gwen001\n\n", version)
}

func printConfig(cfg *config.Config, logger *slog.Logger) {
	logger.Info("configuration",
		"domain", cfg.Domain,
		"output", cfg.OutputPath,
		"format", cfg.OutputFormat,
		"tokens", len(cfg.Tokens),
		"concurrency", cfg.Concurrency,
		"quick_mode", cfg.QuickMode,
		"extend_mode", cfg.ExtendMode,
		"sources", strings.Join(cfg.SearchSources, ","),
	)
}
