package dedup

import (
	"fmt"
	"sync"
	"testing"
)

func TestSetAdd(t *testing.T) {
	s := New()

	if !s.Add("foo") {
		t.Error("first Add should return true")
	}
	if s.Add("foo") {
		t.Error("duplicate Add should return false")
	}
	if s.Len() != 1 {
		t.Errorf("expected length 1, got %d", s.Len())
	}
}

func TestSetContains(t *testing.T) {
	s := New()
	s.Add("hello")

	if !s.Contains("hello") {
		t.Error("expected Contains to return true for added item")
	}
	if s.Contains("world") {
		t.Error("expected Contains to return false for missing item")
	}
}

func TestSetItems(t *testing.T) {
	s := New()
	s.Add("charlie")
	s.Add("alpha")
	s.Add("bravo")

	items := s.Items()
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Items should be sorted
	expected := []string{"alpha", "bravo", "charlie"}
	for i, want := range expected {
		if items[i] != want {
			t.Errorf("items[%d] = %q, want %q", i, items[i], want)
		}
	}
}

func TestSetNewFromSlice(t *testing.T) {
	s := NewFromSlice([]string{"a", "b", "c", "a"})

	if s.Len() != 3 {
		t.Errorf("expected 3 unique items, got %d", s.Len())
	}
	if !s.Contains("a") || !s.Contains("b") || !s.Contains("c") {
		t.Error("missing expected items")
	}
}

func TestSetConcurrentAccess(t *testing.T) {
	s := New()

	var wg sync.WaitGroup
	const goroutines = 100
	const itemsPerGoroutine = 50

	// Concurrent writers
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				s.Add(fmt.Sprintf("item-%d-%d", id, j))
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Len()
			_ = s.Contains("item-0-0")
		}()
	}

	wg.Wait()

	if s.Len() != goroutines*itemsPerGoroutine {
		t.Errorf("expected %d items, got %d", goroutines*itemsPerGoroutine, s.Len())
	}
}

func TestSetDuplicateConcurrent(t *testing.T) {
	s := New()

	var wg sync.WaitGroup
	const goroutines = 100
	added := make([]bool, goroutines)

	// All goroutines try to add the same item
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			added[idx] = s.Add("same-item")
		}(i)
	}

	wg.Wait()

	// Exactly one goroutine should have added successfully
	trueCount := 0
	for _, v := range added {
		if v {
			trueCount++
		}
	}
	if trueCount != 1 {
		t.Errorf("expected exactly 1 successful add, got %d", trueCount)
	}

	if s.Len() != 1 {
		t.Errorf("expected set length 1, got %d", s.Len())
	}
}
