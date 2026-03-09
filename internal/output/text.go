package output

import (
	"bufio"
	"os"
	"sync"
)

// TextWriter writes one subdomain per line to a file.
// Thread-safe: multiple goroutines may call Write concurrently.
type TextWriter struct {
	mu     sync.Mutex
	file   *os.File
	writer *bufio.Writer
}

// NewTextWriter creates a text output writer for the given file path.
func NewTextWriter(path string) (*TextWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	return &TextWriter{
		file:   f,
		writer: bufio.NewWriter(f),
	}, nil
}

// Write outputs a single subdomain as a line.
func (w *TextWriter) Write(result Result) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, err := w.writer.WriteString(result.Subdomain + "\n")
	if err != nil {
		return err
	}
	// Flush immediately for real-time output (tail -f friendly)
	return w.writer.Flush()
}

// Flush writes any buffered data to the underlying file.
func (w *TextWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Flush()
}

// Close flushes and closes the output file.
func (w *TextWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.writer.Flush(); err != nil {
		return err
	}
	return w.file.Close()
}
