package output

import "time"

// Result represents a single discovered subdomain with metadata.
type Result struct {
	Subdomain   string    `json:"subdomain"`
	Source      string    `json:"source,omitempty"`
	FoundAt     time.Time `json:"found_at"`
	SearchQuery string    `json:"search_query,omitempty"`
}

// Writer defines the interface for output backends.
type Writer interface {
	Write(result Result) error
	Flush() error
	Close() error
}
