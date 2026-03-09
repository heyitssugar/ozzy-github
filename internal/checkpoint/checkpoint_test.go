package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.checkpoint.json")

	state := &State{
		Domain:          "example.com",
		SearchIndex:     5,
		PageIndex:       3,
		CompletedURLs:   []string{"https://github.com/a/b", "https://github.com/c/d"},
		FoundSubdomains: []string{"api.example.com", "staging.example.com"},
	}

	if err := Save(path, state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Domain != state.Domain {
		t.Errorf("Domain = %q, want %q", loaded.Domain, state.Domain)
	}
	if loaded.SearchIndex != state.SearchIndex {
		t.Errorf("SearchIndex = %d, want %d", loaded.SearchIndex, state.SearchIndex)
	}
	if len(loaded.CompletedURLs) != len(state.CompletedURLs) {
		t.Errorf("CompletedURLs length = %d, want %d", len(loaded.CompletedURLs), len(state.CompletedURLs))
	}
	if len(loaded.FoundSubdomains) != len(state.FoundSubdomains) {
		t.Errorf("FoundSubdomains length = %d, want %d", len(loaded.FoundSubdomains), len(state.FoundSubdomains))
	}
	if loaded.Timestamp.IsZero() {
		t.Error("Timestamp should be set after Save")
	}
}

func TestSaveAtomicity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.checkpoint.json")

	state := &State{Domain: "example.com", SearchIndex: 1}

	if err := Save(path, state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Temp file should not exist
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful save")
	}

	// Final file should exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("checkpoint file should exist after save")
	}
}

func TestLoadNonExistent(t *testing.T) {
	_, err := Load("/nonexistent/path/file.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestDefaultPath(t *testing.T) {
	path := DefaultPath("example.com")
	if path != "example.com.checkpoint.json" {
		t.Errorf("DefaultPath = %q, want %q", path, "example.com.checkpoint.json")
	}
}

func TestSaveTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "time.checkpoint.json")

	before := time.Now()
	state := &State{Domain: "test.com"}
	_ = Save(path, state)
	after := time.Now()

	loaded, _ := Load(path)
	if loaded.Timestamp.Before(before) || loaded.Timestamp.After(after) {
		t.Error("Timestamp should be set to current time during Save")
	}
}
