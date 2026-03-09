package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultLanguages used for search refinement when results exceed 1000.
var DefaultLanguages = []string{
	"JavaScript", "Python", "Java", "Go", "Ruby", "PHP", "Shell",
	"CSV", "Markdown", "XML", "JSON", "Text", "CSS", "HTML",
	"Perl", "ActionScript", "Lua", "C", "C%2B%2B", "C%23",
	"TypeScript", "Rust", "Kotlin", "Swift", "Scala",
	"Terraform", "HCL", "YAML", "TOML", "Dockerfile",
}

// DefaultNoise keywords added to searches for result refinement.
var DefaultNoise = []string{
	"api", "private", "secret", "internal", "corp",
	"development", "production", "staging", "test",
	"admin", "dev", "sandbox", "demo",
}

// Config holds all tool configuration.
type Config struct {
	Domain        string
	OutputPath    string
	OutputFormat  string // "text", "json", "jsonl"
	Tokens        []string
	QuickMode     bool
	ExtendMode    bool
	RawOutput     bool
	StopOnNoToken bool
	Concurrency   int
	Timeout       time.Duration
	Delay         time.Duration
	Languages     []string
	Noise         []string
	ResumeFile    string
	ProxyURL      string
	GitHubBaseURL string
	Verbose       bool
	SearchSources []string // "code", "commits", "issues"
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.Domain == "" {
		return fmt.Errorf("domain is required")
	}

	if len(c.Tokens) == 0 {
		return fmt.Errorf("at least one GitHub token is required")
	}

	switch c.OutputFormat {
	case "text", "json", "jsonl":
	default:
		return fmt.Errorf("invalid output format %q: must be text, json, or jsonl", c.OutputFormat)
	}

	if c.Concurrency < 1 || c.Concurrency > 200 {
		return fmt.Errorf("concurrency must be between 1 and 200, got %d", c.Concurrency)
	}

	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if c.ProxyURL != "" {
		if _, err := url.Parse(c.ProxyURL); err != nil {
			return fmt.Errorf("invalid proxy URL: %w", err)
		}
	}

	for _, src := range c.SearchSources {
		switch src {
		case "code", "commits", "issues":
		default:
			return fmt.Errorf("invalid search source %q: must be code, commits, or issues", src)
		}
	}

	return nil
}

// SetDefaults fills in default values for unset fields.
func (c *Config) SetDefaults() {
	if c.OutputFormat == "" {
		c.OutputFormat = "text"
	}
	if c.Concurrency == 0 {
		c.Concurrency = 30
	}
	if c.Timeout == 0 {
		c.Timeout = 10 * time.Second
	}
	if c.GitHubBaseURL == "" {
		c.GitHubBaseURL = "https://api.github.com"
	}
	if len(c.SearchSources) == 0 {
		c.SearchSources = []string{"code"}
	}
	if c.OutputPath == "" {
		dir, _ := os.Getwd()
		c.OutputPath = dir + "/" + c.Domain + ".txt"
	}
	if c.Languages == nil && !c.QuickMode {
		c.Languages = DefaultLanguages
	}
	if c.Noise == nil && !c.QuickMode {
		c.Noise = DefaultNoise
	}
	if c.QuickMode {
		c.Languages = nil
		c.Noise = nil
	}
}

// LoadTokens resolves tokens from a raw string which can be:
// - a single token
// - comma-separated tokens
// - a file path containing one token per line
// Falls back to GITHUB_TOKEN environment variable.
func LoadTokens(raw string) []string {
	if raw == "" {
		raw = os.Getenv("GITHUB_TOKEN")
		if raw == "" {
			raw = readTokenFile(".tokens")
		}
	} else {
		if info, err := os.Stat(raw); err == nil && !info.IsDir() {
			raw = readTokenFile(raw)
		}
	}

	if raw == "" {
		return nil
	}

	var tokens []string
	seen := make(map[string]struct{})
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, exists := seen[t]; exists {
			continue
		}
		seen[t] = struct{}{}
		tokens = append(tokens, t)
	}
	return tokens
}

// LoadListFromFile reads a newline-separated file into a deduplicated string slice.
func LoadListFromFile(filename string) ([]string, error) {
	if filename == "" || filename == "none" {
		return nil, nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}

	var items []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, exists := seen[line]; exists {
			continue
		}
		seen[line] = struct{}{}
		items = append(items, line)
	}
	return items, nil
}

func readTokenFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var tokens []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, exists := seen[line]; exists {
			continue
		}
		seen[line] = struct{}{}
		tokens = append(tokens, line)
	}
	return strings.Join(tokens, ",")
}
