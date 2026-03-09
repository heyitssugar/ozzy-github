package token

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestIsValidToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{"classic 40-char hex", "abcdef0123456789abcdef0123456789abcdef01", true},
		{"ghp format", "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef0123", true},
		{"github_pat format", "github_pat_" + "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_ABCDEFGHIJKLMNOPQRS", true},
		{"too short", "abc123", false},
		{"empty", "", false},
		{"random string", "not-a-valid-token-at-all", false},
		{"almost classic (39 chars)", "abcdef0123456789abcdef0123456789abcdef0", false},
		{"ghp wrong prefix", "gho_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef0123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidToken(tt.token)
			if got != tt.valid {
				t.Errorf("IsValidToken(%q) = %v, want %v", tt.token, got, tt.valid)
			}
		})
	}
}

func TestManagerRoundRobin(t *testing.T) {
	tokens := []string{
		"abcdef0123456789abcdef0123456789abcdef01",
		"abcdef0123456789abcdef0123456789abcdef02",
	}
	mgr := NewManager(tokens, time.Minute)

	if mgr.Total() != 2 {
		t.Fatalf("expected 2 tokens, got %d", mgr.Total())
	}

	seen := make(map[string]int)
	for i := 0; i < 10; i++ {
		tok, err := mgr.Next()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		seen[tok]++
	}

	if len(seen) != 2 {
		t.Errorf("expected 2 different tokens, got %d", len(seen))
	}
}

func TestManagerDisableAndRecover(t *testing.T) {
	tokens := []string{"abcdef0123456789abcdef0123456789abcdef01"}
	mgr := NewManager(tokens, 100*time.Millisecond) // Very short cooldown for testing

	tok, _ := mgr.Next()
	mgr.Disable(tok)

	// Token should be unavailable
	_, err := mgr.Next()
	if err != ErrNoTokenAvailable {
		t.Fatalf("expected ErrNoTokenAvailable, got %v", err)
	}

	if mgr.Count() != 0 {
		t.Errorf("expected 0 enabled tokens, got %d", mgr.Count())
	}

	// Wait for cooldown
	time.Sleep(150 * time.Millisecond)

	// Token should be available again
	recovered, err := mgr.Next()
	if err != nil {
		t.Fatalf("expected token to recover, got error: %v", err)
	}
	if recovered != tok {
		t.Errorf("expected same token, got different one")
	}
}

func TestManagerRemove(t *testing.T) {
	tokens := []string{
		"abcdef0123456789abcdef0123456789abcdef01",
		"abcdef0123456789abcdef0123456789abcdef02",
	}
	mgr := NewManager(tokens, time.Minute)

	tok, _ := mgr.Next()
	mgr.Remove(tok)

	if mgr.Total() != 1 {
		t.Errorf("expected 1 token after removal, got %d", mgr.Total())
	}

	// Should still be able to get the other token
	other, err := mgr.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if other == tok {
		t.Error("got removed token, expected the other one")
	}
}

func TestManagerEmptyTokens(t *testing.T) {
	mgr := NewManager(nil, time.Minute)

	_, err := mgr.Next()
	if err != ErrNoTokenAvailable {
		t.Errorf("expected ErrNoTokenAvailable for empty manager, got %v", err)
	}
}

func TestManagerInvalidTokensFiltered(t *testing.T) {
	tokens := []string{
		"abcdef0123456789abcdef0123456789abcdef01",
		"invalid-token",
		"too-short",
	}
	mgr := NewManager(tokens, time.Minute)

	if mgr.Total() != 1 {
		t.Errorf("expected 1 valid token, got %d", mgr.Total())
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	tokens := []string{
		"abcdef0123456789abcdef0123456789abcdef01",
		"abcdef0123456789abcdef0123456789abcdef02",
		"abcdef0123456789abcdef0123456789abcdef03",
	}
	mgr := NewManager(tokens, 50*time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := mgr.Next()
			if err == nil {
				if i%3 == 0 {
					mgr.Disable(tok)
				}
			}
		}()
	}
	wg.Wait()
}

func TestManagerWaitForAvailable(t *testing.T) {
	tokens := []string{"abcdef0123456789abcdef0123456789abcdef01"}
	mgr := NewManager(tokens, 100*time.Millisecond)

	tok, _ := mgr.Next()
	mgr.Disable(tok)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := mgr.WaitForAvailable(ctx)
	if err != nil {
		t.Fatalf("expected token to become available, got: %v", err)
	}
}

func TestManagerWaitForAvailableCancelled(t *testing.T) {
	tokens := []string{"abcdef0123456789abcdef0123456789abcdef01"}
	mgr := NewManager(tokens, 10*time.Second)

	tok, _ := mgr.Next()
	mgr.Disable(tok)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := mgr.WaitForAvailable(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
