package token

import (
	"context"
	"errors"
	"math/rand"
	"regexp"
	"sync"
	"time"
)

// ErrNoTokenAvailable is returned when all tokens are disabled or removed.
var ErrNoTokenAvailable = errors.New("no token available")

var tokenRegexp = regexp.MustCompile(`^([0-9a-f]{40}|ghp_[a-zA-Z0-9]{36}|github_pat_[_a-zA-Z0-9]{82})$`)

// Token represents a single GitHub API token.
type Token struct {
	Value    string
	disabled time.Time
	cooldown time.Duration
}

// IsDisabled returns true if the token is currently in cooldown.
func (t *Token) IsDisabled() bool {
	if t.disabled.IsZero() {
		return false
	}
	return time.Now().Before(t.disabled.Add(t.cooldown))
}

// Manager handles token rotation, disabling, and removal.
type Manager struct {
	mu       sync.RWMutex
	tokens   []Token
	index    int
	cooldown time.Duration
}

// NewManager creates a token manager from raw token strings.
// Invalid tokens are silently skipped. Tokens are shuffled for load distribution.
func NewManager(rawTokens []string, cooldown time.Duration) *Manager {
	var tokens []Token
	for _, raw := range rawTokens {
		if IsValidToken(raw) {
			tokens = append(tokens, Token{
				Value:    raw,
				cooldown: cooldown,
			})
		}
	}

	// Shuffle tokens for distribution across multiple runs
	rand.Shuffle(len(tokens), func(i, j int) {
		tokens[i], tokens[j] = tokens[j], tokens[i]
	})

	return &Manager{
		tokens:   tokens,
		index:    -1,
		cooldown: cooldown,
	}
}

// Next returns the next available token using round-robin rotation.
// Returns ErrNoTokenAvailable if all tokens are disabled or removed.
func (m *Manager) Next() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.tokens) == 0 {
		return "", ErrNoTokenAvailable
	}

	n := len(m.tokens)
	for i := 0; i < n; i++ {
		m.index = (m.index + 1) % n
		t := &m.tokens[m.index]
		if !t.IsDisabled() {
			return t.Value, nil
		}
	}

	return "", ErrNoTokenAvailable
}

// Disable temporarily disables a token due to rate limiting.
func (m *Manager) Disable(tokenValue string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.tokens {
		if m.tokens[i].Value == tokenValue {
			m.tokens[i].disabled = time.Now()
			return
		}
	}
}

// Remove permanently removes a token (e.g., bad credentials).
func (m *Manager) Remove(tokenValue string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.tokens {
		if m.tokens[i].Value == tokenValue {
			m.tokens = append(m.tokens[:i], m.tokens[i+1:]...)
			if m.index >= len(m.tokens) && len(m.tokens) > 0 {
				m.index = m.index % len(m.tokens)
			}
			return
		}
	}
}

// Count returns the number of currently enabled tokens.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for i := range m.tokens {
		if !m.tokens[i].IsDisabled() {
			count++
		}
	}
	return count
}

// Total returns the total number of tokens (enabled + disabled).
func (m *Manager) Total() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tokens)
}

// WaitForAvailable blocks until a token becomes available or context is cancelled.
func (m *Manager) WaitForAvailable(ctx context.Context) error {
	for {
		if _, err := m.Next(); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Check again
		}
	}
}

// IsValidToken checks if a string matches known GitHub token formats.
func IsValidToken(s string) bool {
	return tokenRegexp.MatchString(s)
}
