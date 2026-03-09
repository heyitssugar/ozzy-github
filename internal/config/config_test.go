package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{"valid config", func(c *Config) {}, false},
		{"missing domain", func(c *Config) { c.Domain = "" }, true},
		{"missing tokens", func(c *Config) { c.Tokens = nil }, true},
		{"invalid format", func(c *Config) { c.OutputFormat = "xml" }, true},
		{"concurrency too low", func(c *Config) { c.Concurrency = 0 }, true},
		{"concurrency too high", func(c *Config) { c.Concurrency = 999 }, true},
		{"negative timeout", func(c *Config) { c.Timeout = -1 }, true},
		{"invalid proxy", func(c *Config) { c.ProxyURL = "://invalid" }, true},
		{"invalid source", func(c *Config) { c.SearchSources = []string{"unknown"} }, true},
		{"json format", func(c *Config) { c.OutputFormat = "json" }, false},
		{"jsonl format", func(c *Config) { c.OutputFormat = "jsonl" }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetDefaults(t *testing.T) {
	cfg := Config{
		Domain: "example.com",
	}
	cfg.SetDefaults()

	if cfg.OutputFormat != "text" {
		t.Errorf("default OutputFormat = %q, want 'text'", cfg.OutputFormat)
	}
	if cfg.Concurrency != 30 {
		t.Errorf("default Concurrency = %d, want 30", cfg.Concurrency)
	}
	if cfg.Timeout != 10*time.Second {
		t.Errorf("default Timeout = %v, want 10s", cfg.Timeout)
	}
	if cfg.GitHubBaseURL != "https://api.github.com" {
		t.Errorf("default GitHubBaseURL = %q", cfg.GitHubBaseURL)
	}
	if len(cfg.SearchSources) != 1 || cfg.SearchSources[0] != "code" {
		t.Errorf("default SearchSources = %v, want [code]", cfg.SearchSources)
	}
	if cfg.Languages == nil {
		t.Error("default Languages should not be nil")
	}
}

func TestSetDefaultsQuickMode(t *testing.T) {
	cfg := Config{
		Domain:    "example.com",
		QuickMode: true,
	}
	cfg.SetDefaults()

	if cfg.Languages != nil {
		t.Error("QuickMode should set Languages to nil")
	}
	if cfg.Noise != nil {
		t.Error("QuickMode should set Noise to nil")
	}
}

func TestLoadTokens(t *testing.T) {
	tokens := LoadTokens("tok1,tok2,tok3")
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}
}

func TestLoadTokensFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".tokens")
	os.WriteFile(path, []byte("token1\ntoken2\ntoken1\n"), 0o644)

	tokens := LoadTokens(path)
	if len(tokens) != 2 {
		t.Errorf("expected 2 unique tokens, got %d: %v", len(tokens), tokens)
	}
}

func TestLoadTokensFromEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "envtoken1,envtoken2")
	tokens := LoadTokens("")
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens from env, got %d", len(tokens))
	}
}

func TestLoadListFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list.txt")
	os.WriteFile(path, []byte("alpha\nbeta\nalpha\ngamma\n"), 0o644)

	items, err := LoadListFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 unique items, got %d", len(items))
	}
}

func TestLoadListFromFileNone(t *testing.T) {
	items, err := LoadListFromFile("none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Error("expected nil for 'none'")
	}
}

func validConfig() Config {
	return Config{
		Domain:        "example.com",
		Tokens:        []string{"test-token"},
		OutputFormat:  "text",
		Concurrency:   30,
		Timeout:       10 * time.Second,
		SearchSources: []string{"code"},
	}
}
