package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// State holds the current scan progress for resume capability.
type State struct {
	Domain          string    `json:"domain"`
	SearchIndex     int       `json:"search_index"`
	PageIndex       int       `json:"page_index"`
	CompletedURLs   []string  `json:"completed_urls"`
	FoundSubdomains []string  `json:"found_subdomains"`
	Timestamp       time.Time `json:"timestamp"`
}

// Save writes the checkpoint state to a file atomically (write temp + rename).
func Save(path string, state *State) error {
	state.Timestamp = time.Now()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling checkpoint: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("writing checkpoint temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming checkpoint: %w", err)
	}

	return nil
}

// Load reads a checkpoint state from a file.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading checkpoint: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing checkpoint: %w", err)
	}

	return &state, nil
}

// AutoSave periodically saves checkpoint state in the background.
// It stops when the context is cancelled.
func AutoSave(ctx context.Context, path string, interval time.Duration, fn func() *State) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final save on shutdown
			if state := fn(); state != nil {
				_ = Save(path, state)
			}
			return
		case <-ticker.C:
			if state := fn(); state != nil {
				_ = Save(path, state)
			}
		}
	}
}

// DefaultPath returns the default checkpoint file path for a domain.
func DefaultPath(domain string) string {
	return domain + ".checkpoint.json"
}
