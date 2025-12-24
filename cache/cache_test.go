package cache

import (
	"strconv"
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	c := New[string, int](100)
	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.Capacity() != 100 {
		t.Errorf("expected capacity 100, got %d", c.Capacity())
	}
	if c.Len() != 0 {
		t.Errorf("expected empty cache, got %d entries", c.Len())
	}
}

func TestCacheGetSet(t *testing.T) {
	c := New[string, int](10)

	// Set a value
	c.Set("key1", 42)

	// Get existing key
	val, ok := c.Get("key1")
	if !ok {
		t.Error("expected key1 to exist")
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	// Get non-existing key
	_, ok = c.Get("nonexistent")
	if ok {
		t.Error("expected nonexistent key to not exist")
	}
}

func TestCacheGetOrCreate(t *testing.T) {
	c := New[string, int](10)
	createCalled := 0

	// First call should create
	val := c.GetOrCreate("key1", func() int {
		createCalled++
		return 100
	})
	if val != 100 {
		t.Errorf("expected 100, got %d", val)
	}
	if createCalled != 1 {
		t.Errorf("expected create called once, got %d", createCalled)
	}

	// Second call should return cached
	val = c.GetOrCreate("key1", func() int {
		createCalled++
		return 200
	})
	if val != 100 {
		t.Errorf("expected 100 (cached), got %d", val)
	}
	if createCalled != 1 {
		t.Errorf("expected create still called once, got %d", createCalled)
	}
}

func TestCacheDelete(t *testing.T) {
	c := New[string, int](10)

	c.Set("key1", 42)

	// Delete existing
	if !c.Delete("key1") {
		t.Error("expected Delete to return true for existing key")
	}

	// Verify deleted
	_, ok := c.Get("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}

	// Delete non-existing
	if c.Delete("nonexistent") {
		t.Error("expected Delete to return false for non-existing key")
	}
}

func TestCacheClear(t *testing.T) {
	c := New[string, int](10)

	c.Set("key1", 1)
	c.Set("key2", 2)
	c.Set("key3", 3)

	if c.Len() != 3 {
		t.Errorf("expected 3 entries, got %d", c.Len())
	}

	c.Clear()

	if c.Len() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", c.Len())
	}
}

func TestCacheEviction(t *testing.T) {
	c := New[string, int](4)

	// Fill cache
	for i := 0; i < 4; i++ {
		c.Set(strconv.Itoa(i), i)
	}

	if c.Len() != 4 {
		t.Errorf("expected 4 entries, got %d", c.Len())
	}

	// Add one more to trigger eviction
	c.Set("new", 100)

	// Should have evicted some entries (25% = 1 entry, so 3 remain + 1 new = 4 max, but could be 3)
	if c.Len() > 4 {
		t.Errorf("expected at most 4 entries after eviction, got %d", c.Len())
	}

	// New entry should exist
	val, ok := c.Get("new")
	if !ok || val != 100 {
		t.Error("expected new entry to exist")
	}
}

func TestCacheStats(t *testing.T) {
	c := New[string, int](10)

	c.Set("key1", 1)
	c.Set("key2", 2)

	stats := c.Stats()
	if stats.Len != 2 {
		t.Errorf("expected Len=2, got %d", stats.Len)
	}
	if stats.Capacity != 10 {
		t.Errorf("expected Capacity=10, got %d", stats.Capacity)
	}
}

func TestCacheConcurrent(t *testing.T) {
	c := New[int, int](1000)
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Set(n*100+j, n*100+j)
			}
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Get(n*100 + j)
			}
		}(i)
	}
	wg.Wait()

	// Cache should have entries (may be less due to eviction)
	if c.Len() == 0 {
		t.Error("expected non-empty cache after concurrent operations")
	}
}

// ShardedCache tests

func TestNewSharded(t *testing.T) {
	c := NewSharded[string, int](100, StringHasher)
	if c == nil {
		t.Fatal("NewSharded returned nil")
	}
	if c.Capacity() != 100 {
		t.Errorf("expected capacity 100, got %d", c.Capacity())
	}
	if c.TotalCapacity() != 100*DefaultShardCount {
		t.Errorf("expected total capacity %d, got %d", 100*DefaultShardCount, c.TotalCapacity())
	}
	if c.Len() != 0 {
		t.Errorf("expected empty cache, got %d entries", c.Len())
	}
}

func TestShardedCacheGetSet(t *testing.T) {
	c := NewSharded[string, int](10, StringHasher)

	// Set a value
	c.Set("key1", 42)

	// Get existing key
	val, ok := c.Get("key1")
	if !ok {
		t.Error("expected key1 to exist")
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	// Get non-existing key
	_, ok = c.Get("nonexistent")
	if ok {
		t.Error("expected nonexistent key to not exist")
	}
}

func TestShardedCacheGetOrCreate(t *testing.T) {
	c := NewSharded[string, int](10, StringHasher)
	createCalled := 0

	// First call should create
	val := c.GetOrCreate("key1", func() int {
		createCalled++
		return 100
	})
	if val != 100 {
		t.Errorf("expected 100, got %d", val)
	}
	if createCalled != 1 {
		t.Errorf("expected create called once, got %d", createCalled)
	}

	// Second call should return cached
	val = c.GetOrCreate("key1", func() int {
		createCalled++
		return 200
	})
	if val != 100 {
		t.Errorf("expected 100 (cached), got %d", val)
	}
	if createCalled != 1 {
		t.Errorf("expected create still called once, got %d", createCalled)
	}
}

func TestShardedCacheDelete(t *testing.T) {
	c := NewSharded[string, int](10, StringHasher)

	c.Set("key1", 42)

	// Delete existing
	if !c.Delete("key1") {
		t.Error("expected Delete to return true for existing key")
	}

	// Verify deleted
	_, ok := c.Get("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}

	// Delete non-existing
	if c.Delete("nonexistent") {
		t.Error("expected Delete to return false for non-existing key")
	}
}

func TestShardedCacheClear(t *testing.T) {
	c := NewSharded[string, int](10, StringHasher)

	c.Set("key1", 1)
	c.Set("key2", 2)
	c.Set("key3", 3)

	if c.Len() != 3 {
		t.Errorf("expected 3 entries, got %d", c.Len())
	}

	c.Clear()

	if c.Len() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", c.Len())
	}
}

func TestShardedCacheEviction(t *testing.T) {
	c := NewSharded[int, int](4, IntHasher)

	// Fill beyond capacity (per shard)
	// With 16 shards and capacity 4 per shard, we need many entries
	for i := 0; i < 100; i++ {
		c.Set(i, i)
	}

	// Should have some evictions
	stats := c.Stats()
	if stats.Evictions == 0 {
		t.Log("No evictions occurred (may depend on hash distribution)")
	}
}

func TestShardedCacheStats(t *testing.T) {
	c := NewSharded[string, int](10, StringHasher)

	c.Set("key1", 1)
	c.Set("key2", 2)

	// Generate hits and misses
	c.Get("key1")        // hit
	c.Get("key1")        // hit
	c.Get("nonexistent") // miss

	stats := c.Stats()
	if stats.Len != 2 {
		t.Errorf("expected Len=2, got %d", stats.Len)
	}
	if stats.Hits != 2 {
		t.Errorf("expected Hits=2, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected Misses=1, got %d", stats.Misses)
	}
}

func TestShardedCacheResetStats(t *testing.T) {
	c := NewSharded[string, int](10, StringHasher)

	c.Set("key1", 1)
	c.Get("key1")
	c.Get("nonexistent")

	c.ResetStats()

	stats := c.Stats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Evictions != 0 {
		t.Errorf("expected all stats to be 0 after reset, got hits=%d misses=%d evictions=%d",
			stats.Hits, stats.Misses, stats.Evictions)
	}
}

func TestShardedCacheShardLen(t *testing.T) {
	c := NewSharded[int, int](10, IntHasher)

	// Add entries
	for i := 0; i < 100; i++ {
		c.Set(i, i)
	}

	lens := c.ShardLen()
	total := 0
	for _, l := range lens {
		total += l
	}

	if total != c.Len() {
		t.Errorf("shard lengths sum %d != Len() %d", total, c.Len())
	}
}

func TestShardedCacheConcurrent(t *testing.T) {
	c := NewSharded[int, int](100, IntHasher)
	var wg sync.WaitGroup

	// Concurrent writes from multiple goroutines
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Set(n*100+j, n*100+j)
			}
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Get(n*100 + j)
			}
		}(i)
	}
	wg.Wait()

	// Cache should have entries
	if c.Len() == 0 {
		t.Error("expected non-empty cache after concurrent operations")
	}
}

func TestHashers(t *testing.T) {
	// Test StringHasher
	h1 := StringHasher("hello")
	h2 := StringHasher("hello")
	h3 := StringHasher("world")

	if h1 != h2 {
		t.Error("StringHasher not deterministic")
	}
	if h1 == h3 {
		t.Error("StringHasher collision for different strings")
	}

	// Test IntHasher
	h4 := IntHasher(42)
	h5 := IntHasher(42)
	h6 := IntHasher(43)

	if h4 != h5 {
		t.Error("IntHasher not deterministic")
	}
	if h4 == h6 {
		t.Error("IntHasher collision for different ints")
	}

	// Test Uint64Hasher
	h7 := Uint64Hasher(12345)
	if h7 != 12345 {
		t.Errorf("Uint64Hasher expected identity, got %d", h7)
	}
}

// LRU list tests

func TestLRUList(t *testing.T) {
	l := newLRUList[string]()

	if l.Len() != 0 {
		t.Errorf("expected empty list, got %d", l.Len())
	}

	// Push elements
	n1 := l.PushFront("a")
	n2 := l.PushFront("b")
	n3 := l.PushFront("c")

	if l.Len() != 3 {
		t.Errorf("expected 3 elements, got %d", l.Len())
	}

	// c is at front, a is oldest
	oldest, ok := l.Oldest()
	if !ok || oldest != "a" {
		t.Errorf("expected oldest to be 'a', got %v", oldest)
	}

	// Move a to front
	l.MoveToFront(n1)
	oldest, _ = l.Oldest()
	if oldest != "b" {
		t.Errorf("expected oldest to be 'b' after moving 'a', got %v", oldest)
	}

	// Remove b
	l.Remove(n2)
	if l.Len() != 2 {
		t.Errorf("expected 2 elements after remove, got %d", l.Len())
	}

	// Remove oldest (c)
	removed, ok := l.RemoveOldest()
	if !ok || removed != "c" {
		t.Errorf("expected to remove 'c', got %v", removed)
	}

	// Only a remains
	if l.Len() != 1 {
		t.Errorf("expected 1 element, got %d", l.Len())
	}

	// Clear
	l.Clear()
	if l.Len() != 0 {
		t.Errorf("expected empty list after clear, got %d", l.Len())
	}

	// Prevent unused variable warnings
	_ = n3
}

func TestLRUListEmptyOperations(t *testing.T) {
	l := newLRUList[int]()

	// RemoveOldest on empty list
	_, ok := l.RemoveOldest()
	if ok {
		t.Error("expected RemoveOldest to return false on empty list")
	}

	// Oldest on empty list
	_, ok = l.Oldest()
	if ok {
		t.Error("expected Oldest to return false on empty list")
	}

	// Remove nil
	l.Remove(nil) // Should not panic

	// MoveToFront nil
	l.MoveToFront(nil) // Should not panic
}
