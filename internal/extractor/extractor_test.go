package extractor

import (
	"testing"
)

func TestRegexpExtractorNormalMode(t *testing.T) {
	ext, err := NewRegexpExtractor("example.com", false)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			"basic subdomain",
			`url = "https://api.example.com/v1/endpoint"`,
			[]string{"api.example.com"},
		},
		{
			"multiple subdomains",
			`hosts: ["staging.example.com", "prod.example.com"]`,
			[]string{"staging.example.com", "prod.example.com"},
		},
		{
			"deep subdomain",
			`server: a.b.c.example.com`,
			[]string{"a.b.c.example.com"},
		},
		{
			"apex domain only",
			`site: example.com`,
			[]string{"example.com"},
		},
		{
			"no match",
			`site: notexample.org`,
			nil,
		},
		{
			"underscore subdomain",
			`_dmarc.example.com TXT "v=DMARC1"`,
			[]string{"_dmarc.example.com"},
		},
		{
			"hyphenated subdomain",
			`url: my-api-server.example.com`,
			[]string{"my-api-server.example.com"},
		},
		{
			"case insensitive",
			`HOST = "API.Example.COM"`,
			[]string{"api.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := ext.Extract(tt.content, "test-source")

			if tt.expected == nil {
				if len(results) != 0 {
					t.Errorf("expected no results, got %d: %v", len(results), results)
				}
				return
			}

			if len(results) != len(tt.expected) {
				t.Errorf("expected %d results, got %d: %v", len(tt.expected), len(results), results)
				return
			}

			for i, want := range tt.expected {
				if results[i].Value != want {
					t.Errorf("result[%d] = %q, want %q", i, results[i].Value, want)
				}
				if results[i].Source != "test-source" {
					t.Errorf("result[%d].Source = %q, want %q", i, results[i].Source, "test-source")
				}
			}
		})
	}
}

func TestRegexpExtractorExtendMode(t *testing.T) {
	ext, err := NewRegexpExtractor("example.com", true)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	tests := []struct {
		name    string
		content string
		hasMatch bool
	}{
		{"normal subdomain", "api.example.com", true},
		{"prefix variant", "testexample.org", true},
		{"suffix variant", "exampletest.com", true},
		{"hyphen variant", "my-example.net", true},
		{"unrelated domain", "other.domain.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := ext.Extract(tt.content, "test")
			hasMatch := len(results) > 0
			if hasMatch != tt.hasMatch {
				t.Errorf("expected match=%v, got %v (results: %v)", tt.hasMatch, hasMatch, results)
			}
		})
	}
}

func TestRegexpExtractorDeduplication(t *testing.T) {
	ext, err := NewRegexpExtractor("example.com", false)
	if err != nil {
		t.Fatalf("failed to create extractor: %v", err)
	}

	content := `
		api.example.com
		api.example.com
		api.example.com
		staging.example.com
	`

	results := ext.Extract(content, "test")
	if len(results) != 2 {
		t.Errorf("expected 2 unique results, got %d: %v", len(results), results)
	}
}
