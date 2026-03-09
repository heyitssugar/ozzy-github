package github

import (
	"strings"
	"testing"
)

func TestGenerateSignature(t *testing.T) {
	q1 := SearchQuery{Keyword: "test", Language: "Go", Sort: "indexed", Order: "desc", Noise: []string{"api", "dev"}}
	q2 := SearchQuery{Keyword: "test", Language: "Go", Sort: "indexed", Order: "desc", Noise: []string{"dev", "api"}} // Same noise, different order
	q3 := SearchQuery{Keyword: "test", Language: "Python", Sort: "indexed", Order: "desc", Noise: []string{"api", "dev"}}

	sig1 := GenerateSignature(q1)
	sig2 := GenerateSignature(q2)
	sig3 := GenerateSignature(q3)

	if sig1 != sig2 {
		t.Error("same queries with different noise order should have same signature")
	}
	if sig1 == sig3 {
		t.Error("different queries should have different signatures")
	}

	// Different SourceType should produce different signatures
	q4 := SearchQuery{Keyword: "test", Sort: "indexed", Order: "desc", SourceType: SourceCode}
	q5 := SearchQuery{Keyword: "test", Sort: "indexed", Order: "desc", SourceType: SourceCommit}
	if GenerateSignature(q4) == GenerateSignature(q5) {
		t.Error("different SourceTypes should have different signatures")
	}

	// Different sort/order should produce different signatures
	q6 := SearchQuery{Keyword: "test", Sort: "indexed", Order: "asc"}
	q7 := SearchQuery{Keyword: "test", Sort: "indexed", Order: "desc"}
	if GenerateSignature(q6) == GenerateSignature(q7) {
		t.Error("different sort orders should have different signatures")
	}
}

func TestEncodeDomain(t *testing.T) {
	tests := []struct {
		domain   string
		expected string
	}{
		{"example.com", "%22example.com%22"},
		{"my-site.co.uk", "%22my%2Dsite.co.uk%22"},
	}

	for _, tt := range tests {
		got := EncodeDomain(tt.domain)
		if got != tt.expected {
			t.Errorf("EncodeDomain(%q) = %q, want %q", tt.domain, got, tt.expected)
		}
	}
}

func TestBuildLanguageQueries(t *testing.T) {
	base := SearchQuery{Keyword: "test", Sort: "indexed", Order: "desc"}
	langs := []string{"Go", "Python", "JavaScript"}

	queries := BuildLanguageQueries(base, langs)
	if len(queries) != 3 {
		t.Fatalf("expected 3 language queries, got %d", len(queries))
	}

	for i, q := range queries {
		if q.Language != langs[i] {
			t.Errorf("query[%d].Language = %q, want %q", i, q.Language, langs[i])
		}
		if q.Keyword != "test" {
			t.Errorf("query[%d].Keyword = %q, want 'test'", i, q.Keyword)
		}
		if q.Signature == "" {
			t.Errorf("query[%d] missing signature", i)
		}
	}
}

func TestBuildNoiseQueries(t *testing.T) {
	base := SearchQuery{Keyword: "test", Noise: []string{"api"}}
	noise := []string{"api", "dev", "staging"}

	queries := BuildNoiseQueries(base, noise)

	// "api" is already in base, so should be skipped
	if len(queries) != 2 {
		t.Fatalf("expected 2 noise queries (skipping existing), got %d", len(queries))
	}
}

func TestGetStrategies(t *testing.T) {
	strategies := GetStrategies([]string{"code", "commits", "issues"})
	if len(strategies) != 3 {
		t.Errorf("expected 3 strategies, got %d", len(strategies))
	}

	names := make(map[string]bool)
	for _, s := range strategies {
		names[s.Name()] = true
	}

	for _, expected := range []string{"code", "commits", "issues"} {
		if !names[expected] {
			t.Errorf("missing strategy: %s", expected)
		}
	}
}

func TestCodeSearchStrategyBuildQueries(t *testing.T) {
	s := &CodeSearchStrategy{}
	queries := s.BuildQueries("%22example.com%22", SearchOptions{})

	if len(queries) == 0 {
		t.Fatal("expected at least 1 query")
	}

	// Should have base queries + sort variations + filename + extension + path + in: + wildcard + protocol + org
	// This should be well over 100 queries now
	if len(queries) < 50 {
		t.Errorf("expected at least 50 queries (advanced search engine), got %d", len(queries))
	}

	// Verify all queries have SourceType set to SourceCode
	for i, q := range queries {
		if q.SourceType != SourceCode {
			t.Errorf("query[%d] has SourceType %d, want SourceCode (0)", i, q.SourceType)
		}
	}

	// Verify all queries have signatures
	for i, q := range queries {
		if q.Signature == "" {
			t.Errorf("query[%d] missing signature", i)
		}
	}

	// Verify no duplicate signatures
	sigs := make(map[string]int)
	for i, q := range queries {
		if prev, exists := sigs[q.Signature]; exists {
			t.Errorf("duplicate signature between query[%d] and query[%d]: %s", prev, i, q.Signature)
		}
		sigs[q.Signature] = i
	}
}

func TestCommitSearchStrategyBuildQueries(t *testing.T) {
	s := &CommitSearchStrategy{}
	queries := s.BuildQueries("%22example.com%22", SearchOptions{})

	if len(queries) < 5 {
		t.Errorf("expected at least 5 commit queries, got %d", len(queries))
	}

	// Verify SourceType
	for i, q := range queries {
		if q.SourceType != SourceCommit {
			t.Errorf("commit query[%d] has SourceType %d, want SourceCommit (1)", i, q.SourceType)
		}
	}
}

func TestIssueSearchStrategyBuildQueries(t *testing.T) {
	s := &IssueSearchStrategy{}
	queries := s.BuildQueries("%22example.com%22", SearchOptions{})

	if len(queries) < 5 {
		t.Errorf("expected at least 5 issue queries, got %d", len(queries))
	}

	// Verify SourceType
	for i, q := range queries {
		if q.SourceType != SourceIssue {
			t.Errorf("issue query[%d] has SourceType %d, want SourceIssue (2)", i, q.SourceType)
		}
	}
}

func TestCodeSearchWithSubdomainPrefixes(t *testing.T) {
	s := &CodeSearchStrategy{}
	// Non-quick mode enables subdomain prefix searches
	queries := s.BuildQueries("%22example.com%22", SearchOptions{QuickMode: false})

	// Should include subdomain prefix queries like api.example.com, staging.example.com
	foundPrefix := false
	for _, q := range queries {
		if strings.Contains(q.Keyword, "api") && strings.Contains(q.Keyword, "example.com") {
			foundPrefix = true
			break
		}
	}
	if !foundPrefix {
		t.Error("expected to find subdomain prefix queries (e.g., api.example.com)")
	}
}

func TestCodeSearchSortOrderVariations(t *testing.T) {
	s := &CodeSearchStrategy{}
	queries := s.BuildQueries("%22example.com%22", SearchOptions{})

	// Should have queries with different sort/order combos
	sortOrders := make(map[string]bool)
	for _, q := range queries {
		key := q.Sort + ":" + q.Order
		sortOrders[key] = true
	}

	if len(sortOrders) < 2 {
		t.Errorf("expected at least 2 different sort/order combinations, got %d", len(sortOrders))
	}
}

func TestBuildLanguageQueriesPreservesSourceType(t *testing.T) {
	base := SearchQuery{Keyword: "test", Sort: "indexed", Order: "desc", SourceType: SourceCode}
	queries := BuildLanguageQueries(base, []string{"Go", "Python"})

	for i, q := range queries {
		if q.SourceType != SourceCode {
			t.Errorf("language query[%d] lost SourceType, got %d want %d", i, q.SourceType, SourceCode)
		}
	}
}

func TestQueryCountForHackerone(t *testing.T) {
	encoded := EncodeDomain("hackerone.com")
	opts := SearchOptions{QuickMode: false}

	code := &CodeSearchStrategy{}
	codeQ := code.BuildQueries(encoded, opts)
	t.Logf("Code queries: %d", len(codeQ))

	commit := &CommitSearchStrategy{}
	commitQ := commit.BuildQueries(encoded, opts)
	t.Logf("Commit queries: %d", len(commitQ))

	issue := &IssueSearchStrategy{}
	issueQ := issue.BuildQueries(encoded, opts)
	t.Logf("Issue queries: %d", len(issueQ))

	total := len(codeQ) + len(commitQ) + len(issueQ)
	t.Logf("Total initial queries: %d", total)
}
