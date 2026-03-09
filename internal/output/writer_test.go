package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTextWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	w, err := NewTextWriter(path)
	if err != nil {
		t.Fatalf("NewTextWriter failed: %v", err)
	}

	results := []Result{
		{Subdomain: "api.example.com", FoundAt: time.Now()},
		{Subdomain: "staging.example.com", FoundAt: time.Now()},
	}

	for _, r := range results {
		if err := w.Write(r); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "api.example.com" {
		t.Errorf("line 0 = %q, want %q", lines[0], "api.example.com")
	}
}

func TestJSONWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	w, err := NewJSONWriter(path)
	if err != nil {
		t.Fatalf("NewJSONWriter failed: %v", err)
	}

	w.Write(Result{Subdomain: "api.example.com", FoundAt: time.Now()})
	w.Write(Result{Subdomain: "staging.example.com", FoundAt: time.Now()})

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var results []Result
	if err := json.Unmarshal(content, &results); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestJSONLWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	w, err := NewJSONLWriter(path)
	if err != nil {
		t.Fatalf("NewJSONLWriter failed: %v", err)
	}

	w.Write(Result{Subdomain: "api.example.com", FoundAt: time.Now()})
	w.Write(Result{Subdomain: "staging.example.com", FoundAt: time.Now()})

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}

	// Each line should be valid JSON
	for i, line := range lines {
		var r Result
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestNewWriter(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		format string
		ext    string
	}{
		{"text", ".txt"},
		{"json", ".json"},
		{"jsonl", ".jsonl"},
	}

	for _, tt := range tests {
		path := filepath.Join(dir, "output"+tt.ext)
		w, err := NewWriter(path, tt.format)
		if err != nil {
			t.Errorf("NewWriter(%q) failed: %v", tt.format, err)
			continue
		}
		w.Close()
	}
}

func TestTextWriterCloseFlushes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flush.txt")

	w, _ := NewTextWriter(path)
	w.Write(Result{Subdomain: "test.example.com", FoundAt: time.Now()})
	w.Close()

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "test.example.com") {
		t.Error("data should be flushed on Close")
	}
}
