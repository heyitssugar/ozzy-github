package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gwen001/github-subdomains/internal/token"
)

func newTestManager(tokens ...string) *token.Manager {
	return token.NewManager(tokens, 50*time.Millisecond)
}

func TestSearchCodeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		resp := SearchResponse{
			TotalCount: 2,
			Items: []SearchItem{
				{HTMLURL: "https://github.com/user/repo/blob/main/config.yml", Path: "config.yml"},
				{HTMLURL: "https://github.com/user/repo/blob/main/hosts.txt", Path: "hosts.txt"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	mgr := newTestManager("abcdef0123456789abcdef0123456789abcdef01")
	client := NewClient(mgr, WithBaseURL(server.URL))

	resp, err := client.SearchCode(context.Background(), SearchQuery{
		Keyword: "%22example.com%22",
		Sort:    "indexed",
		Order:   "desc",
	}, 1)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected TotalCount=2, got %d", resp.TotalCount)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
}

func TestSearchCodeUnauthorized(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 1 {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(SearchResponse{Message: "Bad credentials"})
			return
		}
		json.NewEncoder(w).Encode(SearchResponse{TotalCount: 1})
	}))
	defer server.Close()

	mgr := newTestManager(
		"abcdef0123456789abcdef0123456789abcdef01",
		"abcdef0123456789abcdef0123456789abcdef02",
	)
	client := NewClient(mgr, WithBaseURL(server.URL))

	resp, err := client.SearchCode(context.Background(), SearchQuery{Keyword: "test"}, 1)
	if err != nil {
		t.Fatalf("should retry with next token: %v", err)
	}
	if resp.TotalCount != 1 {
		t.Errorf("expected TotalCount=1, got %d", resp.TotalCount)
	}
	if mgr.Total() != 1 {
		t.Errorf("expected 1 token remaining after removal, got %d", mgr.Total())
	}
}

func TestSearchCodeRateLimited(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 1 {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(SearchResponse{Message: "API rate limit exceeded"})
			return
		}
		json.NewEncoder(w).Encode(SearchResponse{TotalCount: 5})
	}))
	defer server.Close()

	mgr := newTestManager(
		"abcdef0123456789abcdef0123456789abcdef01",
		"abcdef0123456789abcdef0123456789abcdef02",
	)
	client := NewClient(mgr, WithBaseURL(server.URL))

	resp, err := client.SearchCode(context.Background(), SearchQuery{Keyword: "test"}, 1)
	if err != nil {
		t.Fatalf("should retry with different token: %v", err)
	}
	if resp.TotalCount != 5 {
		t.Errorf("expected TotalCount=5, got %d", resp.TotalCount)
	}
}

func TestSearchCodeServerError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(SearchResponse{TotalCount: 1})
	}))
	defer server.Close()

	mgr := newTestManager("abcdef0123456789abcdef0123456789abcdef01")
	client := NewClient(mgr, WithBaseURL(server.URL))

	resp, err := client.SearchCode(context.Background(), SearchQuery{Keyword: "test"}, 1)
	if err != nil {
		t.Fatalf("should retry on server error: %v", err)
	}
	if resp.TotalCount != 1 {
		t.Errorf("expected TotalCount=1, got %d", resp.TotalCount)
	}
}

func TestSearchCodeContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	mgr := newTestManager("abcdef0123456789abcdef0123456789abcdef01")
	client := NewClient(mgr, WithBaseURL(server.URL), WithTimeout(100*time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.SearchCode(ctx, SearchQuery{Keyword: "test"}, 1)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGetRawContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("file content with api.example.com and staging.example.com"))
	}))
	defer server.Close()

	mgr := newTestManager("abcdef0123456789abcdef0123456789abcdef01")
	client := NewClient(mgr, WithBaseURL(server.URL))

	content, err := client.GetRawContent(context.Background(), server.URL+"/some/file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestHTMLToRawURL(t *testing.T) {
	tests := []struct {
		html string
		raw  string
	}{
		{
			"https://github.com/user/repo/blob/main/file.txt",
			"https://raw.githubusercontent.com/user/repo/main/file.txt",
		},
		{
			"https://github.com/org/project/blob/develop/config/hosts.yml",
			"https://raw.githubusercontent.com/org/project/develop/config/hosts.yml",
		},
	}

	for _, tt := range tests {
		got := htmlToRawURL(tt.html)
		if got != tt.raw {
			t.Errorf("htmlToRawURL(%q) = %q, want %q", tt.html, got, tt.raw)
		}
	}
}
