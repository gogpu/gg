package cache

import (
	"fmt"
	"sync"
	"testing"

	"github.com/gogpu/gg/text"
)

// =============================================================================
// LRU List Benchmarks
// =============================================================================

func BenchmarkLRUList_PushFront(b *testing.B) {
	l := newLRUList[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.PushFront(i)
	}
}

func BenchmarkLRUList_MoveToFront(b *testing.B) {
	l := newLRUList[int]()
	nodes := make([]*lruNode[int], 1000)
	for i := 0; i < 1000; i++ {
		nodes[i] = l.PushFront(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.MoveToFront(nodes[i%1000])
	}
}

func BenchmarkLRUList_RemoveOldest(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		l := newLRUList[int]()
		for j := 0; j < 1000; j++ {
			l.PushFront(j)
		}
		b.StartTimer()
		for l.Len() > 0 {
			_, _ = l.RemoveOldest()
		}
	}
}

// =============================================================================
// ShapingKey Benchmarks
// =============================================================================

func BenchmarkShapingKey_New(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewShapingKey("hello world", 1, 16.0, text.DirectionLTR, 0)
	}
}

func BenchmarkShapingKey_Hash(b *testing.B) {
	key := NewShapingKey("hello world", 1, 16.0, text.DirectionLTR, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = key.keyHash()
	}
}

func BenchmarkHashString(b *testing.B) {
	tests := []struct {
		name string
		text string
	}{
		{"short", "hello"},
		{"medium", "The quick brown fox jumps over the lazy dog."},
		{"long", "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."},
	}

	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = hashString(tc.text)
			}
		})
	}
}

func BenchmarkHashFeatures(b *testing.B) {
	features := map[string]int{
		"liga": 1,
		"kern": 1,
		"calt": 1,
		"smcp": 1,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HashFeatures(features)
	}
}

// =============================================================================
// ShapingCache Get Benchmarks (TARGET: < 50ns for cache hit)
// =============================================================================

func BenchmarkShapingCache_Get_Hit(b *testing.B) {
	c := NewShapingCache(1000)
	key := NewShapingKey("hello world", 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{Advance: 100.0})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get(key)
	}
}

func BenchmarkShapingCache_Get_Miss(b *testing.B) {
	c := NewShapingCache(1000)
	key := NewShapingKey("nonexistent", 1, 16.0, text.DirectionLTR, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get(key)
	}
}

func BenchmarkShapingCache_Get_Hot(b *testing.B) {
	c := NewShapingCache(1000)
	// Pre-populate with many entries
	for i := 0; i < 500; i++ {
		key := NewShapingKey(fmt.Sprintf("text_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{Advance: float64(i)})
	}

	// Hot key
	hotKey := NewShapingKey("hot_key", 1, 16.0, text.DirectionLTR, 0)
	c.Set(hotKey, &text.ShapedRun{Advance: 100.0})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get(hotKey)
	}
}

// =============================================================================
// ShapingCache Set Benchmarks
// =============================================================================

func BenchmarkShapingCache_Set(b *testing.B) {
	c := NewShapingCache(100000)
	run := &text.ShapedRun{Advance: 100.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := NewShapingKey(fmt.Sprintf("text_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, run)
	}
}

func BenchmarkShapingCache_Set_Overwrite(b *testing.B) {
	c := NewShapingCache(1000)
	key := NewShapingKey("overwrite", 1, 16.0, text.DirectionLTR, 0)
	run := &text.ShapedRun{Advance: 100.0}
	c.Set(key, run)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(key, run)
	}
}

func BenchmarkShapingCache_Set_WithEviction(b *testing.B) {
	c := NewShapingCache(100) // Small capacity to force eviction
	run := &text.ShapedRun{Advance: 100.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := NewShapingKey(fmt.Sprintf("evict_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, run)
	}
}

// =============================================================================
// ShapingCache GetOrCreate Benchmarks
// =============================================================================

func BenchmarkShapingCache_GetOrCreate_Hit(b *testing.B) {
	c := NewShapingCache(1000)
	key := NewShapingKey("hello world", 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{Advance: 100.0})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.GetOrCreate(key, func() *text.ShapedRun {
			return &text.ShapedRun{Advance: 200.0}
		})
	}
}

func BenchmarkShapingCache_GetOrCreate_Miss(b *testing.B) {
	c := NewShapingCache(100000)
	run := &text.ShapedRun{Advance: 100.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := NewShapingKey(fmt.Sprintf("create_%d", i), 1, 16.0, text.DirectionLTR, 0)
		_ = c.GetOrCreate(key, func() *text.ShapedRun {
			return run
		})
	}
}

// =============================================================================
// Concurrent Benchmarks
// =============================================================================

func BenchmarkShapingCache_Get_Parallel(b *testing.B) {
	c := NewShapingCache(1000)
	// Pre-populate with entries distributed across shards
	for i := 0; i < 100; i++ {
		key := NewShapingKey(fmt.Sprintf("parallel_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{Advance: float64(i)})
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := NewShapingKey(fmt.Sprintf("parallel_%d", i%100), 1, 16.0, text.DirectionLTR, 0)
			_, _ = c.Get(key)
			i++
		}
	})
}

func BenchmarkShapingCache_Set_Parallel(b *testing.B) {
	c := NewShapingCache(100000)
	run := &text.ShapedRun{Advance: 100.0}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := NewShapingKey(fmt.Sprintf("parallel_set_%d", i), 1, 16.0, text.DirectionLTR, 0)
			c.Set(key, run)
			i++
		}
	})
}

func BenchmarkShapingCache_Mixed_Parallel(b *testing.B) {
	c := NewShapingCache(10000)
	run := &text.ShapedRun{Advance: 100.0}

	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := NewShapingKey(fmt.Sprintf("mixed_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, run)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 < 7 {
				// 70% reads
				key := NewShapingKey(fmt.Sprintf("mixed_%d", i%1000), 1, 16.0, text.DirectionLTR, 0)
				_, _ = c.Get(key)
			} else {
				// 30% writes
				key := NewShapingKey(fmt.Sprintf("mixed_%d", i), 1, 16.0, text.DirectionLTR, 0)
				c.Set(key, run)
			}
			i++
		}
	})
}

func BenchmarkShapingCache_Contention(b *testing.B) {
	// Test high contention scenario: all goroutines access same key
	c := NewShapingCache(1000)
	key := NewShapingKey("contention", 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{Advance: 100.0})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = c.Get(key)
		}
	})
}

// =============================================================================
// Shard Distribution Benchmark
// =============================================================================

func BenchmarkShapingCache_ShardDistribution(b *testing.B) {
	c := NewShapingCache(10000)
	run := &text.ShapedRun{Advance: 100.0}

	// Fill cache and measure distribution
	for i := 0; i < b.N; i++ {
		key := NewShapingKey(fmt.Sprintf("dist_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, run)
	}

	b.StopTimer()

	// Report distribution
	lens := c.ShardLen()
	var minLen, maxLen int
	for i, l := range lens {
		if i == 0 || l < minLen {
			minLen = l
		}
		if i == 0 || l > maxLen {
			maxLen = l
		}
	}
	b.ReportMetric(float64(maxLen-minLen), "shard_imbalance")
}

// =============================================================================
// Stats Benchmark
// =============================================================================

func BenchmarkShapingCache_Stats(b *testing.B) {
	c := NewShapingCache(1000)
	// Pre-populate
	for i := 0; i < 500; i++ {
		key := NewShapingKey(fmt.Sprintf("stats_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Stats()
	}
}

// =============================================================================
// Real-World Simulation Benchmarks
// =============================================================================

func BenchmarkShapingCache_RealWorld_TextEditor(b *testing.B) {
	// Simulates text editor: repeated access to same text with variations
	c := NewShapingCache(1000)

	// Common text patterns
	texts := []string{
		"func ",
		"return ",
		"if err != nil {",
		"package main",
		"import (",
		"type ",
		"for i := 0; i < n; i++ {",
	}

	// Pre-populate
	for _, txt := range texts {
		key := NewShapingKey(txt, 1, 14.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{Advance: float64(len(txt)) * 8.0})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		txt := texts[i%len(texts)]
		key := NewShapingKey(txt, 1, 14.0, text.DirectionLTR, 0)
		_, _ = c.Get(key)
	}
}

func BenchmarkShapingCache_RealWorld_Browser(b *testing.B) {
	// Simulates browser: mix of cached and new text
	c := NewShapingCache(2000)

	// Common words (cached)
	common := []string{"the", "a", "is", "of", "and", "to", "in", "for", "on", "with"}
	for _, word := range common {
		key := NewShapingKey(word, 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{Advance: float64(len(word)) * 10.0})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%10 < 8 {
			// 80% common words (cache hit)
			word := common[i%len(common)]
			key := NewShapingKey(word, 1, 16.0, text.DirectionLTR, 0)
			_, _ = c.Get(key)
		} else {
			// 20% new words (cache miss + insert)
			key := NewShapingKey(fmt.Sprintf("unique_%d", i), 1, 16.0, text.DirectionLTR, 0)
			c.Set(key, &text.ShapedRun{Advance: 80.0})
		}
	}
}

func BenchmarkShapingCache_RealWorld_UI(b *testing.B) {
	// Simulates UI: labels, buttons, menus (highly cached)
	c := NewShapingCache(500)

	// UI strings
	uiStrings := []string{
		"File", "Edit", "View", "Help",
		"OK", "Cancel", "Apply", "Close",
		"Save", "Open", "New", "Delete",
		"Settings", "Preferences", "About",
	}

	// Pre-populate all UI strings
	for _, s := range uiStrings {
		key := NewShapingKey(s, 1, 14.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{Advance: float64(len(s)) * 8.0})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := uiStrings[i%len(uiStrings)]
		key := NewShapingKey(s, 1, 14.0, text.DirectionLTR, 0)
		_, _ = c.Get(key)
	}
}

// =============================================================================
// Memory Allocation Benchmarks
// =============================================================================

func BenchmarkShapingCache_Get_Allocs(b *testing.B) {
	c := NewShapingCache(1000)
	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{Advance: 100.0})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get(key)
	}
}

func BenchmarkShapingKey_New_Allocs(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewShapingKey("hello world", 1, 16.0, text.DirectionLTR, 0)
	}
}

// =============================================================================
// Scalability Benchmarks
// =============================================================================

func BenchmarkShapingCache_Scalability(b *testing.B) {
	sizes := []int{100, 1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			c := NewShapingCache(size)
			run := &text.ShapedRun{Advance: 100.0}

			// Fill to 50% capacity
			for i := 0; i < size*DefaultShardCount/2; i++ {
				key := NewShapingKey(fmt.Sprintf("scale_%d", i), 1, 16.0, text.DirectionLTR, 0)
				c.Set(key, run)
			}

			// Benchmark get operations
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key := NewShapingKey(fmt.Sprintf("scale_%d", i%(size*DefaultShardCount/2)), 1, 16.0, text.DirectionLTR, 0)
				_, _ = c.Get(key)
			}
		})
	}
}

func BenchmarkShapingCache_ParallelScalability(b *testing.B) {
	goroutines := []int{1, 2, 4, 8, 16, 32}

	for _, numG := range goroutines {
		b.Run(fmt.Sprintf("goroutines_%d", numG), func(b *testing.B) {
			c := NewShapingCache(10000)
			run := &text.ShapedRun{Advance: 100.0}

			// Pre-populate
			for i := 0; i < 5000; i++ {
				key := NewShapingKey(fmt.Sprintf("parallel_scale_%d", i), 1, 16.0, text.DirectionLTR, 0)
				c.Set(key, run)
			}

			b.ResetTimer()
			b.SetParallelism(numG)
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					key := NewShapingKey(fmt.Sprintf("parallel_scale_%d", i%5000), 1, 16.0, text.DirectionLTR, 0)
					_, _ = c.Get(key)
					i++
				}
			})
		})
	}
}

// =============================================================================
// Comparison: Sharded vs Single Lock (for validation)
// =============================================================================

// singleLockCache is a simple cache with single mutex for comparison
type singleLockCache struct {
	mu      sync.RWMutex
	entries map[ShapingKey]*text.ShapedRun
}

func newSingleLockCache() *singleLockCache {
	return &singleLockCache{
		entries: make(map[ShapingKey]*text.ShapedRun),
	}
}

func (c *singleLockCache) Get(key ShapingKey) (*text.ShapedRun, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.entries[key]
	return v, ok
}

func (c *singleLockCache) Set(key ShapingKey, value *text.ShapedRun) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = value
}

func BenchmarkComparison_SingleLock_Get(b *testing.B) {
	c := newSingleLockCache()
	key := NewShapingKey("hello", 1, 16.0, text.DirectionLTR, 0)
	c.Set(key, &text.ShapedRun{Advance: 100.0})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get(key)
	}
}

func BenchmarkComparison_SingleLock_Get_Parallel(b *testing.B) {
	c := newSingleLockCache()
	for i := 0; i < 100; i++ {
		key := NewShapingKey(fmt.Sprintf("parallel_%d", i), 1, 16.0, text.DirectionLTR, 0)
		c.Set(key, &text.ShapedRun{Advance: float64(i)})
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := NewShapingKey(fmt.Sprintf("parallel_%d", i%100), 1, 16.0, text.DirectionLTR, 0)
			_, _ = c.Get(key)
			i++
		}
	})
}
