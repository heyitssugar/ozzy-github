package output

import (
	"fmt"
	"os"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
)

// ProgressTracker wraps a progress bar for search progress display.
type ProgressTracker struct {
	bar       *progressbar.ProgressBar
	found     int
	enabled   bool
}

// NewProgressTracker creates a progress tracker.
// It is disabled if raw mode is active or stdout is not a terminal.
func NewProgressTracker(totalSearches int, rawMode bool) *ProgressTracker {
	isTTY := term.IsTerminal(int(os.Stderr.Fd()))
	enabled := !rawMode && isTTY && totalSearches > 0

	pt := &ProgressTracker{enabled: enabled}

	if enabled {
		pt.bar = progressbar.NewOptions(totalSearches,
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionSetDescription("Searching"),
			progressbar.OptionShowCount(),
			progressbar.OptionSetPredictTime(true),
			progressbar.OptionClearOnFinish(),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "=",
				SaucerHead:    ">",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)
	}

	return pt
}

// Increment advances the progress bar by one search.
func (p *ProgressTracker) Increment() {
	if !p.enabled || p.bar == nil {
		return
	}
	_ = p.bar.Add(1)
}

// SetFound updates the number of found subdomains displayed.
func (p *ProgressTracker) SetFound(count int) {
	p.found = count
	if p.enabled && p.bar != nil {
		p.bar.Describe(fmt.Sprintf("Searching [%d found]", count))
	}
}

// UpdateTotal updates the total number of searches.
func (p *ProgressTracker) UpdateTotal(total int) {
	if p.enabled && p.bar != nil {
		p.bar.ChangeMax(total)
	}
}

// Finish completes the progress bar.
func (p *ProgressTracker) Finish() {
	if p.enabled && p.bar != nil {
		_ = p.bar.Finish()
	}
}
