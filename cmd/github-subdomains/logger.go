package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31;1m"
	colorGreen  = "\033[32;1m"
	colorYellow = "\033[33;1m"
	colorGray   = "\033[37m"
	colorWhite  = "\033[37;1m"
)

// ColorHandler is a slog.Handler that outputs colored log lines.
type ColorHandler struct {
	opts  *slog.HandlerOptions
	w     io.Writer
	mu    sync.Mutex
	attrs []slog.Attr
}

// NewColorHandler creates a handler that writes colored output.
func NewColorHandler(w io.Writer, opts *slog.HandlerOptions) *ColorHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &ColorHandler{opts: opts, w: w}
}

func (h *ColorHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *ColorHandler) Handle(_ context.Context, r slog.Record) error {
	// Format time
	timeStr := r.Time.Format("15:04:05")

	// Pick color based on level or special message patterns
	color := colorWhite
	switch {
	case r.Level >= slog.LevelError:
		color = colorRed
	case r.Level >= slog.LevelWarn:
		color = colorYellow
	case r.Level >= slog.LevelInfo:
		color = colorWhite
	default:
		color = colorGray
	}

	// Check for special "found" or "source" messages
	if r.Message == "found" {
		color = colorGreen
	} else if r.Message == "source" {
		color = colorYellow
	}

	// Build attribute string
	var attrStr string
	r.Attrs(func(a slog.Attr) bool {
		if h.opts.ReplaceAttr != nil {
			a = h.opts.ReplaceAttr(nil, a)
		}
		attrStr += fmt.Sprintf(" %s=%v", a.Key, a.Value.Any())
		return true
	})

	// Include pre-set attrs
	for _, a := range h.attrs {
		attrStr += fmt.Sprintf(" %s=%v", a.Key, a.Value.Any())
	}

	line := fmt.Sprintf("%s[%s] %s%s%s\n", color, timeStr, r.Message, attrStr, colorReset)

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write([]byte(line))
	return err
}

func (h *ColorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ColorHandler{
		opts:  h.opts,
		w:     h.w,
		attrs: append(h.attrs, attrs...),
	}
}

func (h *ColorHandler) WithGroup(name string) slog.Handler {
	// Groups not needed for this CLI tool
	return h
}
