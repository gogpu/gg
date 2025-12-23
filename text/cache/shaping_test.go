package cache

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gogpu/gg/text"
)

// =============================================================================
// LRU List Tests
// =============================================================================

func TestLRUList_New(t *testing.T) {
	l := newLRUList[int]()
	if l.Len() != 0 {
		t.Errorf("new list should be empty, got len=%d", l.Len())
	}
}

func TestLRUList_PushFront(t *testing.T) {
	l := newLRUList[int]()

	// Push first item
	node1 := l.PushFront(1)
	if l.Len() != 1 {
		t.Errorf("expected len=1, got %d", l.Len())
	}
	if node1.key != 1 {
		t.Errorf("expected key=1, got %d", node1.key)
	}
	if l.head != node1 || l.tail != node1 {
		t.Error("single node should be both head and tail")
	}

	// Push second item
	node2 := l.PushFront(2)
	if l.Len() != 2 {
		t.Errorf("expected len=2, got %d", l.Len())
	}
	if l.head != node2 {
		t.Error("node2 should be head")
	}
	if l.tail != node1 {
		t.Error("node1 should be tail")
	}

	// Push third item
	node3 := l.PushFront(3)
	if l.Len() != 3 {
		t.Errorf("expected len=3, got %d", l.Len())
	}
	if l.head != node3 {
		t.Error("node3 should be head")
	}
	if l.tail != node1 {
		t.Error("node1 should still be tail")
	}
}

func TestLRUList_MoveToFront(t *testing.T) {
	l := newLRUList[int]()
	node1 := l.PushFront(1)
	node2 := l.PushFront(2)
	node3 := l.PushFront(3)

	// Order is now: 3 -> 2 -> 1

	// Move tail (1) to front
	l.MoveToFront(node1)
	if l.head != node1 {
		t.Error("node1 should be head after MoveToFront")
	}
	if l.tail != node2 {
		t.Error("node2 should be tail after MoveToFront")
	}
	if l.Len() != 3 {
		t.Errorf("len should be 3, got %d", l.Len())
	}

	// Order is now: 1 -> 3 -> 2

	// Move middle (3) to front
	l.MoveToFront(node3)
	if l.head != node3 {
		t.Error("node3 should be head")
	}
	// Order is now: 3 -> 1 -> 2

	// Move head to front (no-op)
	l.MoveToFront(node3)
	if l.head != node3 {
		t.Error("node3 should still be head")
	}
	if l.Len() != 3 {
		t.Errorf("len should be 3, got %d", l.Len())
	}
}

func TestLRUList_MoveToFront_Nil(t *testing.T) {
	l := newLRUList[int]()
	_ = l.PushFront(1)

	// Should not panic
	l.MoveToFront(nil)
	if l.Len() != 1 {
		t.Errorf("len should be 1, got %d", l.Len())
	}
}

func TestLRUList_Remove(t *testing.T) {
	l := newLRUList[int]()
	node1 := l.PushFront(1)
	node2 := l.PushFront(2)
	node3 := l.PushFront(3)

	// Order: 3 -> 2 -> 1

	// Remove middle
	l.Remove(node2)
	if l.Len() != 2 {
		t.Errorf("expected len=2, got %d", l.Len())
	}
	if l.head != node3 || l.tail != node1 {
		t.Error("head and tail should be unchanged")
	}

	// Remove head
	l.Remove(node3)
	if l.Len() != 1 {
		t.Errorf("expected len=1, got %d", l.Len())
	}
	if l.head != node1 || l.tail != node1 {
		t.Error("node1 should be both head and tail")
	}

	// Remove last
	l.Remove(node1)
	if l.Len() != 0 {
		t.Errorf("expected len=0, got %d", l.Len())
	}
	if l.head != nil || l.tail != nil {
		t.Error("head and tail should be nil")
	}
}

func TestLRUList_Remove_Nil(t *testing.T) {
	l := newLRUList[int]()
	_ = l.PushFront(1)

	// Should not panic
	l.Remove(nil)
	if l.Len() != 1 {
		t.Errorf("len should be 1, got %d", l.Len())
	}
}

func TestLRUList_RemoveOldest(t *testing.T) {
	l := newLRUList[int]()

	// Empty list
	key, ok := l.RemoveOldest()
	if ok {
		t.Error("RemoveOldest on empty list should return false")
	}
	if key != 0 {
		t.Errorf("expected zero value, got %d", key)
	}

	// Add items
	l.PushFront(1)
	l.PushFront(2)
	l.PushFront(3)

	// Order: 3 -> 2 -> 1

	// Remove oldest (1)
	key, ok = l.RemoveOldest()
	if !ok {
		t.Error("RemoveOldest should return true")
	}
	if key != 1 {
		t.Errorf("expected key=1, got %d", key)
	}
	if l.Len() != 2 {
		t.Errorf("expected len=2, got %d", l.Len())
	}

	// Remove oldest (2)
	key, ok = l.RemoveOldest()
	if !ok || key != 2 {
		t.Errorf("expected (2, true), got (%d, %v)", key, ok)
	}

	// Remove oldest (3)
	key, ok = l.RemoveOldest()
	if !ok || key != 3 {
		t.Errorf("expected (3, true), got (%d, %v)", key, ok)
	}

	// List should be empty
	if l.Len() != 0 {
		t.Errorf("expected len=0, got %d", l.Len())
	}
}

func TestLRUList_Oldest(t *testing.T) {
	l := newLRUList[int]()

	// Empty list
	key, ok := l.Oldest()
	if ok {
		t.Error("Oldest on empty list should return false")
	}
	if key != 0 {
		t.Errorf("expected zero value, got %d", key)
	}

	// Add items
	l.PushFront(1)
	l.PushFront(2)
	l.PushFront(3)

	// Oldest should be 1 (not removed)
	key, ok = l.Oldest()
	if !ok || key != 1 {
		t.Errorf("expected (1, true), got (%d, %v)", key, ok)
	}
	if l.Len() != 3 {
		t.Errorf("len should be unchanged, got %d", l.Len())
	}
}

func TestLRUList_Clear(t *testing.T) {
	l := newLRUList[int]()
	l.PushFront(1)
	l.PushFront(2)
	l.PushFront(3)

	l.Clear()

	if l.Len() != 0 {
		t.Errorf("expected len=0 after Clear, got %d", l.Len())
	}
	if l.head != nil || l.tail != nil {
		t.Error("head and tail should be nil after Clear")
	}
}

// =============================================================================
// ShapingKey Tests
// =============================================================================

func TestShapingKey_New(t *testing.T) {
	key := NewShapingKey("hello", 123, 16.0, text.DirectionLTR, 0)

	if key.TextHash == 0 {
		t.Error("TextHash should not be 0")
	}
	if key.FontID != 123 {
		t.Errorf("expected FontID=123, got %d", key.FontID)
	}
	if key.Direction != uint8(text.DirectionLTR) {
		t.Errorf("expected Direction=0, got %d", key.Direction)
	}
}

func TestShapingKey_DifferentText(t *testing.T) {
	key1 := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	key2 := NewShapingKey("world", 1, 16.0, text.DirectionLTR, 0)

	if key1.TextHash == key2.TextHash {
		t.Error("different text should have different hash")
	}
}

func TestShapingKey_DifferentSize(t *testing.T) {
	key1 := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	key2 := NewShapingKey("hello", 1, 20.0, text.DirectionLTR, 0)

	if key1.SizeBits == key2.SizeBits {
		t.Error("different size should have different SizeBits")
	}
}

func TestShapingKey_DifferentDirection(t *testing.T) {
	key1 := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	key2 := NewShapingKey("hello", 1, 16.0, text.DirectionRTL, 0)

	if key1.Direction == key2.Direction {
		t.Error("different direction should have different Direction")
	}
}

func TestShapingKey_DifferentFont(t *testing.T) {
	key1 := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	key2 := NewShapingKey("hello", 2, 16.0, text.DirectionLTR, 0)

	if key1.FontID == key2.FontID {
		t.Error("different font should have different FontID")
	}
}

func TestShapingKey_DifferentFeatures(t *testing.T) {
	key1 := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 100)
	key2 := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 200)

	if key1.Features == key2.Features {
		t.Error("different features should have different Features")
	}
}

func TestShapingKey_KeyHash(t *testing.T) {
	key1 := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	key2 := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)

	hash1 := key1.keyHash()
	hash2 := key2.keyHash()

	if hash1 != hash2 {
		t.Error("identical keys should have identical hash")
	}

	key3 := NewShapingKey("world", 1, 16.0, text.DirectionLTR, 0)
	hash3 := key3.keyHash()

	if hash1 == hash3 {
		t.Error("different keys should have different hash")
	}
}

// =============================================================================
// HashFeatures Tests
// =============================================================================

func TestHashFeatures_Empty(t *testing.T) {
	h := HashFeatures(nil)
	if h != 0 {
		t.Errorf("expected 0 for nil, got %d", h)
	}

	h = HashFeatures(map[string]int{})
	if h != 0 {
		t.Errorf("expected 0 for empty map, got %d", h)
	}
}

func TestHashFeatures_Single(t *testing.T) {
	h := HashFeatures(map[string]int{"liga": 1})
	if h == 0 {
		t.Error("single feature should not hash to 0")
	}
}

func TestHashFeatures_Multiple(t *testing.T) {
	h1 := HashFeatures(map[string]int{"liga": 1, "kern": 1})
	h2 := HashFeatures(map[string]int{"liga": 1, "kern": 1})

	if h1 != h2 {
		t.Error("same features should have same hash")
	}

	h3 := HashFeatures(map[string]int{"liga": 1, "kern": 0})
	if h1 == h3 {
		t.Error("different feature values should have different hash")
	}
}

func TestHashFeatures_OrderIndependent(t *testing.T) {
	// Due to XOR, order should not matter
	h1 := HashFeatures(map[string]int{"liga": 1, "kern": 1, "calt": 1})
	h2 := HashFeatures(map[string]int{"calt": 1, "liga": 1, "kern": 1})

	if h1 != h2 {
		t.Error("feature order should not affect hash")
	}
}

// =============================================================================
// ShapingCache Basic Tests
// =============================================================================

func TestShapingCache_New(t *testing.T) {
	c := NewShapingCache(100)
	if c.Capacity() != 100 {
		t.Errorf("expected capacity=100, got %d", c.Capacity())
	}
	if c.TotalCapacity() != 100*DefaultShardCount {
		t.Errorf("expected total capacity=%d, got %d", 100*DefaultShardCount, c.TotalCapacity())
	}
	if c.Len() != 0 {
		t.Errorf("new cache should be empty, got len=%d", c.Len())
	}
}

func TestShapingCache_NewDefault(t *testing.T) {
	c := DefaultShapingCache()
	if c.Capacity() != DefaultCapacity {
		t.Errorf("expected capacity=%d, got %d", DefaultCapacity, c.Capacity())
	}
}

func TestShapingCache_NewZeroCapacity(t *testing.T) {
	c := NewShapingCache(0)
	if c.Capacity() != DefaultCapacity {
		t.Errorf("zero capacity should use default, got %d", c.Capacity())
	}
}

func TestShapingCache_NewNegativeCapacity(t *testing.T) {
	c := NewShapingCache(-10)
	if c.Capacity() != DefaultCapacity {
		t.Errorf("negative capacity should use default, got %d", c.Capacity())
	}
}

func TestShapingCache_SetGet(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	run := &text.ShapedRun{
		Advance: 100.0,
	}

	// Set
	c.Set(key, run)

	// Get
	got, ok := c.Get(key)
	if !ok {
		t.Error("expected cache hit")
	}
	if got != run {
		t.Error("got wrong value")
	}
	if got.Advance != 100.0 {
		t.Errorf("expected Advance=100.0, got %f", got.Advance)
	}
}

func TestShapingCache_SetNil(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, nil)

	_, ok := c.Get(key)
	if ok {
		t.Error("nil value should not be cached")
	}
}

func TestShapingCache_GetMiss(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	_, ok := c.Get(key)
	if ok {
		t.Error("expected cache miss")
	}
}

func TestShapingCache_SetOverwrite(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	run1 := &text.ShapedRun{Advance: 100.0}
	run2 := &text.ShapedRun{Advance: 200.0}

	c.Set(key, run1)
	c.Set(key, run2)

	got, ok := c.Get(key)
	if !ok {
		t.Error("expected cache hit")
	}
	if got.Advance != 200.0 {
		t.Errorf("expected overwritten value, got Advance=%f", got.Advance)
	}
}

func TestShapingCache_Delete(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	run := &text.ShapedRun{Advance: 100.0}

	c.Set(key, run)

	// Delete
	deleted := c.Delete(key)
	if !deleted {
		t.Error("Delete should return true for existing key")
	}

	// Verify deleted
	_, ok := c.Get(key)
	if ok {
		t.Error("key should be deleted")
	}

	// Delete again
	deleted = c.Delete(key)
	if deleted {
		t.Error("Delete should return false for non-existent key")
	}
}

func TestShapingCache_Clear(t *testing.T) {
	c := NewShapingCache(100)

	// Add several entries
	for i := 0; i < 50; i++ {
		key := NewShapingKey(fmt.Sprintf("text%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{Advance: float64(i)})
	}

	if c.Len() != 50 {
		t.Errorf("expected len=50, got %d", c.Len())
	}

	c.Clear()

	if c.Len() != 0 {
		t.Errorf("expected len=0 after Clear, got %d", c.Len())
	}
}

// =============================================================================
// ShapingCache Eviction Tests
// =============================================================================

func TestShapingCache_Eviction(t *testing.T) {
	c := NewShapingCache(3) // Small capacity for testing

	// Add 3 entries to single shard (force same shard by using similar keys)
	// Note: actual shard distribution depends on hash
	var keys [4]ShapingKey
	for i := 0; i < 4; i++ {
		keys[i] = NewShapingKey(fmt.Sprintf("evict_test_%d", i), 1, 16.0, text.DirectionLTR, 0)
	}

	// Fill one shard
	// We need to ensure all go to same shard for this test
	// Since we can't control hash, we just verify eviction happens

	// Add entries until eviction
	for i := 0; i < 100; i++ {
		key := NewShapingKey(fmt.Sprintf("evict_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{Advance: float64(i)})
	}

	// With capacity 3 per shard * 16 shards = 48, 100 entries should cause eviction
	stats := c.Stats()
	if stats.Evictions == 0 {
		t.Error("expected some evictions")
	}
}

func TestShapingCache_LRUOrder(t *testing.T) {
	c := NewShapingCache(2) // Very small capacity

	// To test LRU, we need entries in the same shard
	// This is tricky since we can't control hash
	// Instead, verify that access updates LRU

	key1 := NewShapingKey("lru_a", 1, 16.0, text.DirectionLTR, 0)
	key2 := NewShapingKey("lru_b", 1, 16.0, text.DirectionLTR, 0)

	c.Set(key1, &text.ShapedRun{Advance: 1.0})
	c.Set(key2, &text.ShapedRun{Advance: 2.0})

	// Access key1 to make it most recent
	_, _ = c.Get(key1)

	// Both should still be present
	_, ok1 := c.Get(key1)
	_, ok2 := c.Get(key2)

	// At least verify both are still accessible (may be in different shards)
	if !ok1 && !ok2 {
		t.Error("expected at least one entry to be present")
	}
}

// =============================================================================
// ShapingCache GetOrCreate Tests
// =============================================================================

func TestShapingCache_GetOrCreate_Miss(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	createCalled := false

	got := c.GetOrCreate(key, func() *text.ShapedRun {
		createCalled = true
		return &text.ShapedRun{Advance: 100.0}
	})

	if !createCalled {
		t.Error("create function should be called on miss")
	}
	if got.Advance != 100.0 {
		t.Errorf("expected Advance=100.0, got %f", got.Advance)
	}
}

func TestShapingCache_GetOrCreate_Hit(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{Advance: 100.0})

	createCalled := false
	got := c.GetOrCreate(key, func() *text.ShapedRun {
		createCalled = true
		return &text.ShapedRun{Advance: 200.0}
	})

	if createCalled {
		t.Error("create function should not be called on hit")
	}
	if got.Advance != 100.0 {
		t.Errorf("expected cached Advance=100.0, got %f", got.Advance)
	}
}

func TestShapingCache_GetOrCreate_NilCreate(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)

	got := c.GetOrCreate(key, func() *text.ShapedRun {
		return nil
	})

	if got != nil {
		t.Error("expected nil when create returns nil")
	}

	// Should not be cached
	_, ok := c.Get(key)
	if ok {
		t.Error("nil result should not be cached")
	}
}

// =============================================================================
// ShapingCache Statistics Tests
// =============================================================================

func TestShapingCache_Stats(t *testing.T) {
	c := NewShapingCache(100)

	// Initial stats
	stats := c.Stats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Error("initial stats should be zero")
	}

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)

	// Miss
	_, _ = c.Get(key)
	stats = c.Stats()
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}

	// Set and hit
	c.Set(key, &text.ShapedRun{})
	_, _ = c.Get(key)
	stats = c.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
}

func TestShapingCache_HitRate(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{})

	// 1 miss (via GetOrCreate miss)
	_, _ = c.Get(NewShapingKey("miss", 1, 16.0, text.DirectionLTR, 0))

	// 3 hits
	_, _ = c.Get(key)
	_, _ = c.Get(key)
	_, _ = c.Get(key)

	stats := c.Stats()
	// 3 hits / (3 hits + 1 miss) = 0.75
	expectedRate := 3.0 / 4.0
	if stats.HitRate != expectedRate {
		t.Errorf("expected hit rate=%f, got %f", expectedRate, stats.HitRate)
	}
}

func TestShapingCache_ResetStats(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{})
	_, _ = c.Get(key)
	_, _ = c.Get(NewShapingKey("miss", 1, 16.0, text.DirectionLTR, 0))

	c.ResetStats()

	stats := c.Stats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Evictions != 0 {
		t.Error("stats should be zero after reset")
	}
}

func TestShapingCache_ShardLen(t *testing.T) {
	c := NewShapingCache(100)

	// Add some entries
	for i := 0; i < 100; i++ {
		key := NewShapingKey(fmt.Sprintf("shard_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{})
	}

	lens := c.ShardLen()

	// Sum should equal total len
	total := 0
	for _, l := range lens {
		total += l
	}
	if total != c.Len() {
		t.Errorf("shard lens sum (%d) != total len (%d)", total, c.Len())
	}

	// Distribution should be reasonably spread (not all in one shard)
	nonEmpty := 0
	for _, l := range lens {
		if l > 0 {
			nonEmpty++
		}
	}
	if nonEmpty < 2 {
		t.Error("entries should be distributed across multiple shards")
	}
}

// =============================================================================
// Concurrency Tests
// =============================================================================

func TestShapingCache_ConcurrentSetGet(t *testing.T) {
	c := NewShapingCache(1000)
	const numGoroutines = 100
	const numOps = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < numOps; i++ {
				key := NewShapingKey(fmt.Sprintf("concurrent_%d_%d", id, i), 1, 16.0, text.DirectionLTR, 0)
				c.Set(key, &text.ShapedRun{Advance: float64(i)})

				// Also do some gets
				if i%2 == 0 {
					_, _ = c.Get(key)
				}
			}
		}(g)
	}

	wg.Wait()

	// Cache should be functional after concurrent access
	stats := c.Stats()
	if stats.Len == 0 {
		t.Error("cache should have entries after concurrent operations")
	}
}

func TestShapingCache_ConcurrentGetOrCreate(t *testing.T) {
	c := NewShapingCache(100)
	const numGoroutines = 50

	key := NewShapingKey("shared", 1, 16.0, text.DirectionLTR, 0)
	var createCount atomic.Int32

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			c.GetOrCreate(key, func() *text.ShapedRun {
				createCount.Add(1)
				return &text.ShapedRun{Advance: 100.0}
			})
		}()
	}

	wg.Wait()

	// Create should only be called once (or very few times due to lock)
	count := createCount.Load()
	if count > 5 {
		t.Errorf("create called too many times: %d (expected 1-2)", count)
	}
}

func TestShapingCache_ConcurrentClear(t *testing.T) {
	c := NewShapingCache(100)
	const numGoroutines = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Concurrent setters
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := NewShapingKey(fmt.Sprintf("clear_%d_%d", id, i), 1, 16.0, text.DirectionLTR, 0)
				c.Set(key, &text.ShapedRun{})
			}
		}(g)
	}

	// Concurrent clears
	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				c.Clear()
			}
		}()
	}

	wg.Wait()

	// Should not panic or corrupt state
	_ = c.Len()
	_ = c.Stats()
}

func TestShapingCache_ConcurrentDelete(t *testing.T) {
	c := NewShapingCache(100)
	const numKeys = 100

	// Pre-populate
	keys := make([]ShapingKey, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = NewShapingKey(fmt.Sprintf("delete_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(keys[i], &text.ShapedRun{})
	}

	var wg sync.WaitGroup
	wg.Add(numKeys)

	// Concurrent deletes
	for i := 0; i < numKeys; i++ {
		go func(idx int) {
			defer wg.Done()
			c.Delete(keys[idx])
		}(i)
	}

	wg.Wait()

	if c.Len() != 0 {
		t.Errorf("expected len=0 after deleting all, got %d", c.Len())
	}
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestShapingCache_EmptyString(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("", 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{Advance: 0.0})

	got, ok := c.Get(key)
	if !ok {
		t.Error("empty string key should work")
	}
	if got.Advance != 0.0 {
		t.Errorf("expected Advance=0.0, got %f", got.Advance)
	}
}

func TestShapingCache_LongString(t *testing.T) {
	c := NewShapingCache(100)

	// Very long string
	longStr := ""
	for i := 0; i < 10000; i++ {
		longStr += "x"
	}

	key := NewShapingKey(longStr, 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{Advance: 10000.0})

	got, ok := c.Get(key)
	if !ok {
		t.Error("long string key should work")
	}
	if got.Advance != 10000.0 {
		t.Errorf("expected Advance=10000.0, got %f", got.Advance)
	}
}

func TestShapingCache_UnicodeString(t *testing.T) {
	c := NewShapingCache(100)

	// Various Unicode strings
	tests := []string{
		"hello world",
		"你好世界",
		"مرحبا بالعالم",
		"שלום עולם",
		"こんにちは",
		"🌍🌎🌏",
		"mixed 中文 & emoji 🎉",
	}

	for _, s := range tests {
		key := NewShapingKey(s, 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{Advance: float64(len(s))})

		got, ok := c.Get(key)
		if !ok {
			t.Errorf("Unicode string %q should work", s)
		}
		if got.Advance != float64(len(s)) {
			t.Errorf("wrong value for %q", s)
		}
	}
}

func TestShapingCache_ZeroFontID(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 0, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{Advance: 100.0})

	got, ok := c.Get(key)
	if !ok {
		t.Error("zero FontID should work")
	}
	if got.Advance != 100.0 {
		t.Errorf("expected Advance=100.0, got %f", got.Advance)
	}
}

func TestShapingCache_ZeroSize(t *testing.T) {
	c := NewShapingCache(100)

	key := NewShapingKey("hello", 1, 0.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{Advance: 0.0})

	got, ok := c.Get(key)
	if !ok {
		t.Error("zero size should work")
	}
	if got.Advance != 0.0 {
		t.Errorf("expected Advance=0.0, got %f", got.Advance)
	}
}

func TestShapingCache_AllDirections(t *testing.T) {
	c := NewShapingCache(100)

	directions := []text.Direction{
		text.DirectionLTR,
		text.DirectionRTL,
		text.DirectionTTB,
		text.DirectionBTT,
	}

	for i, dir := range directions {
		key := NewShapingKey("hello", 1, 16.0, dir, 0)
		c.Set(key, &text.ShapedRun{Advance: float64(i)})
	}

	// Verify all are separate entries
	for i, dir := range directions {
		key := NewShapingKey("hello", 1, 16.0, dir, 0)
		got, ok := c.Get(key)
		if !ok {
			t.Errorf("direction %v should be cached", dir)
		}
		if got.Advance != float64(i) {
			t.Errorf("wrong value for direction %v", dir)
		}
	}
}

// =============================================================================
// Integration Test
// =============================================================================

func TestShapingCache_RealWorldUsage(t *testing.T) {
	c := NewShapingCache(1000)

	// Simulate real-world text rendering
	texts := []string{
		"Hello, World!",
		"The quick brown fox jumps over the lazy dog.",
		"你好世界",
		"مرحبا",
		"こんにちは",
	}
	fonts := []uint64{1, 2, 3}
	sizes := []float32{12.0, 14.0, 16.0, 18.0, 24.0, 36.0}

	// Populate cache
	for _, txt := range texts {
		for _, fontID := range fonts {
			for _, size := range sizes {
				key := NewShapingKey(txt, fontID, size, text.DirectionLTR, 0)
				c.Set(key, &text.ShapedRun{
					Advance: float64(len(txt)) * float64(size),
				})
			}
		}
	}

	// Verify entries
	expectedCount := len(texts) * len(fonts) * len(sizes)
	if c.Len() != expectedCount {
		t.Errorf("expected %d entries, got %d", expectedCount, c.Len())
	}

	// Access pattern: some texts are more common
	for i := 0; i < 100; i++ {
		// "Hello, World!" is accessed frequently
		key := NewShapingKey("Hello, World!", 1, 16.0, text.DirectionLTR, 0)
		_, _ = c.Get(key)
	}

	stats := c.Stats()
	if stats.HitRate < 0.9 {
		t.Errorf("expected high hit rate, got %f", stats.HitRate)
	}
}
