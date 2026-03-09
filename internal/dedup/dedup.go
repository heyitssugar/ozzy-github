package dedup

import (
	"sort"
	"sync"
)

// Set is a thread-safe string set for deduplication.
type Set struct {
	mu    sync.RWMutex
	items map[string]struct{}
}

// New creates a new empty Set.
func New() *Set {
	return &Set{
		items: make(map[string]struct{}),
	}
}

// NewFromSlice creates a Set pre-populated with the given items.
func NewFromSlice(items []string) *Set {
	s := &Set{
		items: make(map[string]struct{}, len(items)),
	}
	for _, item := range items {
		s.items[item] = struct{}{}
	}
	return s
}

// Add inserts an item into the set.
// Returns true if the item was new (not previously in the set).
func (s *Set) Add(item string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.items[item]; exists {
		return false
	}
	s.items[item] = struct{}{}
	return true
}

// Contains checks if an item exists in the set.
func (s *Set) Contains(item string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.items[item]
	return exists
}

// Len returns the number of items in the set.
func (s *Set) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// Items returns all items in the set, sorted alphabetically.
func (s *Set) Items() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.items))
	for item := range s.items {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}
