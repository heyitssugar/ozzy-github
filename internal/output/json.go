package output

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
)

// JSONWriter accumulates results and writes a JSON array on Close.
type JSONWriter struct {
	file    *os.File
	results []Result
}

// NewJSONWriter creates a JSON array output writer.
func NewJSONWriter(path string) (*JSONWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &JSONWriter{file: f}, nil
}

// Write accumulates a result for the final JSON array.
func (w *JSONWriter) Write(result Result) error {
	w.results = append(w.results, result)
	return nil
}

// Flush is a no-op for JSON writer (writes happen on Close).
func (w *JSONWriter) Flush() error {
	return nil
}

// Close writes the accumulated results as a JSON array and closes the file.
func (w *JSONWriter) Close() error {
	encoder := json.NewEncoder(w.file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(w.results); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}

// JSONLWriter writes one JSON object per line (streaming-friendly).
// Thread-safe: multiple goroutines may call Write concurrently.
type JSONLWriter struct {
	mu     sync.Mutex
	file   *os.File
	writer *bufio.Writer
}

// NewJSONLWriter creates a JSON Lines output writer.
func NewJSONLWriter(path string) (*JSONLWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &JSONLWriter{
		file:   f,
		writer: bufio.NewWriter(f),
	}, nil
}

// Write outputs a single result as a JSON line.
func (w *JSONLWriter) Write(result Result) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if _, err := w.writer.Write(data); err != nil {
		return err
	}
	if _, err := w.writer.WriteString("\n"); err != nil {
		return err
	}
	return w.writer.Flush()
}

// Flush writes any buffered data.
func (w *JSONLWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Flush()
}

// Close flushes and closes the file.
func (w *JSONLWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.writer.Flush(); err != nil {
		return err
	}
	return w.file.Close()
}

// NewWriter creates the appropriate writer based on the output format.
func NewWriter(path string, format string) (Writer, error) {
	switch format {
	case "json":
		return NewJSONWriter(path)
	case "jsonl":
		return NewJSONLWriter(path)
	default:
		return NewTextWriter(path)
	}
}
