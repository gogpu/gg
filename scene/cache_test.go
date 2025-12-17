package scene

import (
	"container/list"
	"sync"
	"testing"

	"github.com/gogpu/gg"
)

// Helper to create a pixmap of specific size
func newTestPixmap(width, height int) *gg.Pixmap {
	return gg.NewPixmap(width, height)
}

func TestNewLayerCache(t *testing.T) {
	tests := []struct {
		name        string
		maxSizeMB   int
		wantMaxSize int64
	}{
		{"positive size", 32, 32 * bytesPerMB},
		{"zero defaults to 64MB", 0, DefaultMaxSizeMB * bytesPerMB},
		{"negative defaults to 64MB", -1, DefaultMaxSizeMB * bytesPerMB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewLayerCache(tt.maxSizeMB)
			if cache.MaxSize() != tt.wantMaxSize {
				t.Errorf("MaxSize() = %d, want %d", cache.MaxSize(), tt.wantMaxSize)
			}
			if cache.Size() != 0 {
				t.Errorf("Size() = %d, want 0", cache.Size())
			}
			if cache.EntryCount() != 0 {
				t.Errorf("EntryCount() = %d, want 0", cache.EntryCount())
			}
		})
	}
}

func TestDefaultLayerCache(t *testing.T) {
	cache := DefaultLayerCache()
	if cache.MaxSize() != DefaultMaxSizeMB*bytesPerMB {
		t.Errorf("MaxSize() = %d, want %d", cache.MaxSize(), DefaultMaxSizeMB*bytesPerMB)
	}
}

func TestLayerCache_PutGet(t *testing.T) {
	cache := NewLayerCache(1) // 1MB cache
	pixmap := newTestPixmap(100, 100)
	hash := uint64(12345)
	version := uint64(1)

	// Initially not in cache
	_, ok := cache.Get(hash)
	if ok {
		t.Error("Get should return false for non-existent entry")
	}

	// Put and get
	cache.Put(hash, pixmap, version)

	got, ok := cache.Get(hash)
	if !ok {
		t.Error("Get should return true for existing entry")
	}
	if got != pixmap {
		t.Error("Get should return the same pixmap")
	}
}

func TestLayerCache_PutNil(t *testing.T) {
	cache := NewLayerCache(1)

	// Should not panic or add entry for nil pixmap
	cache.Put(12345, nil, 1)
	if cache.EntryCount() != 0 {
		t.Error("Put with nil pixmap should not add entry")
	}
}

func TestLayerCache_PutReplace(t *testing.T) {
	cache := NewLayerCache(1)
	hash := uint64(12345)

	pixmap1 := newTestPixmap(100, 100)
	pixmap2 := newTestPixmap(200, 200)

	cache.Put(hash, pixmap1, 1)
	cache.Put(hash, pixmap2, 2)

	got, ok := cache.Get(hash)
	if !ok {
		t.Error("Get should return true")
	}
	if got != pixmap2 {
		t.Error("Get should return the replaced pixmap")
	}
	if cache.EntryCount() != 1 {
		t.Errorf("EntryCount() = %d, want 1", cache.EntryCount())
	}
}

func TestLayerCache_LRUEviction(t *testing.T) {
	// Create a cache with 100KB limit
	cache := &LayerCache{
		entries: make(map[uint64]*CacheEntry),
		lru:     newList(),
		maxSize: 100 * 1024, // 100KB
	}

	// 50x50 pixmap = 10,000 bytes
	// We can fit ~10 of these

	for i := uint64(0); i < 10; i++ {
		cache.Put(i, newTestPixmap(50, 50), 1)
	}

	if cache.EntryCount() != 10 {
		t.Errorf("EntryCount() = %d, want 10", cache.EntryCount())
	}

	// Add one more, should evict the oldest (hash 0)
	cache.Put(10, newTestPixmap(50, 50), 1)

	if cache.Contains(0) {
		t.Error("Entry 0 should have been evicted")
	}
	if !cache.Contains(10) {
		t.Error("Entry 10 should be in cache")
	}
}

func TestLayerCache_LRUOrder(t *testing.T) {
	cache := &LayerCache{
		entries: make(map[uint64]*CacheEntry),
		lru:     newList(),
		maxSize: 30 * 1024, // 30KB - fits 3 x 50x50 pixmaps
	}

	// Add 3 entries
	cache.Put(1, newTestPixmap(50, 50), 1)
	cache.Put(2, newTestPixmap(50, 50), 1)
	cache.Put(3, newTestPixmap(50, 50), 1)

	// Access entry 1 to make it recently used
	_, _ = cache.Get(1)

	// Add entry 4, should evict entry 2 (oldest unused)
	cache.Put(4, newTestPixmap(50, 50), 1)

	if cache.Contains(2) {
		t.Error("Entry 2 should have been evicted (least recently used)")
	}
	if !cache.Contains(1) {
		t.Error("Entry 1 should still be in cache (recently accessed)")
	}
	if !cache.Contains(3) {
		t.Error("Entry 3 should still be in cache")
	}
	if !cache.Contains(4) {
		t.Error("Entry 4 should be in cache")
	}
}

func TestLayerCache_Invalidate(t *testing.T) {
	cache := NewLayerCache(1)
	hash := uint64(12345)

	cache.Put(hash, newTestPixmap(100, 100), 1)
	if !cache.Contains(hash) {
		t.Error("Entry should be in cache after Put")
	}

	cache.Invalidate(hash)
	if cache.Contains(hash) {
		t.Error("Entry should not be in cache after Invalidate")
	}
	if cache.Size() != 0 {
		t.Errorf("Size() = %d, want 0 after Invalidate", cache.Size())
	}

	// Invalidate non-existent entry should not panic
	cache.Invalidate(99999)
}

func TestLayerCache_InvalidateAll(t *testing.T) {
	cache := NewLayerCache(1)

	for i := uint64(0); i < 10; i++ {
		cache.Put(i, newTestPixmap(50, 50), 1)
	}

	if cache.EntryCount() == 0 {
		t.Error("Cache should have entries before InvalidateAll")
	}

	cache.InvalidateAll()

	if cache.EntryCount() != 0 {
		t.Errorf("EntryCount() = %d, want 0 after InvalidateAll", cache.EntryCount())
	}
	if cache.Size() != 0 {
		t.Errorf("Size() = %d, want 0 after InvalidateAll", cache.Size())
	}
}

func TestLayerCache_Trim(t *testing.T) {
	cache := &LayerCache{
		entries: make(map[uint64]*CacheEntry),
		lru:     newList(),
		maxSize: 100 * 1024,
	}

	// Add 10 entries (10KB each = 50x50 pixmaps)
	for i := uint64(0); i < 10; i++ {
		cache.Put(i, newTestPixmap(50, 50), 1)
	}

	initialSize := cache.Size()

	// Trim to 50KB (should keep ~5 entries)
	cache.Trim(50 * 1024)

	if cache.Size() > 50*1024 {
		t.Errorf("Size() = %d, should be <= 50KB after Trim", cache.Size())
	}
	if cache.Size() >= initialSize {
		t.Error("Size should have decreased after Trim")
	}

	// Trim to 0 should clear all
	cache.Trim(0)
	if cache.EntryCount() != 0 {
		t.Errorf("EntryCount() = %d, want 0 after Trim(0)", cache.EntryCount())
	}
}

func TestLayerCache_TrimNegative(t *testing.T) {
	cache := NewLayerCache(1)
	cache.Put(1, newTestPixmap(50, 50), 1)

	// Negative trim should be treated as 0
	cache.Trim(-100)
	if cache.EntryCount() != 0 {
		t.Errorf("EntryCount() = %d, want 0 after Trim(-100)", cache.EntryCount())
	}
}

func TestLayerCache_Stats(t *testing.T) {
	cache := NewLayerCache(1)

	// Initial stats
	stats := cache.Stats()
	if stats.Size != 0 {
		t.Errorf("initial Size = %d, want 0", stats.Size)
	}
	if stats.Entries != 0 {
		t.Errorf("initial Entries = %d, want 0", stats.Entries)
	}
	if stats.Hits != 0 {
		t.Errorf("initial Hits = %d, want 0", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("initial Misses = %d, want 0", stats.Misses)
	}
	if stats.HitRate != 0 {
		t.Errorf("initial HitRate = %f, want 0", stats.HitRate)
	}

	// Add entry and check stats
	cache.Put(1, newTestPixmap(100, 100), 1)
	stats = cache.Stats()
	expectedSize := int64(100 * 100 * 4)
	if stats.Size != expectedSize {
		t.Errorf("Size = %d, want %d", stats.Size, expectedSize)
	}
	if stats.Entries != 1 {
		t.Errorf("Entries = %d, want 1", stats.Entries)
	}

	// Cache miss
	_, _ = cache.Get(999)
	stats = cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}

	// Cache hit
	_, _ = cache.Get(1)
	stats = cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Hits = %d, want 1", stats.Hits)
	}

	// Hit rate should be 50% (1 hit, 1 miss)
	if stats.HitRate != 0.5 {
		t.Errorf("HitRate = %f, want 0.5", stats.HitRate)
	}
}

func TestLayerCache_SetMaxSize(t *testing.T) {
	cache := NewLayerCache(1) // 1MB

	// Add entries
	for i := uint64(0); i < 100; i++ {
		cache.Put(i, newTestPixmap(100, 100), 1) // 40KB each
	}

	// Reduce max size to 100KB
	cache.SetMaxSize(1) // Minimum practical size

	// Should still have valid state
	if cache.Size() > cache.MaxSize() {
		t.Error("Size should not exceed MaxSize after SetMaxSize")
	}

	// Set to invalid values should default to 64MB
	cache.SetMaxSize(0)
	if cache.MaxSize() != DefaultMaxSizeMB*bytesPerMB {
		t.Errorf("MaxSize() = %d, want %d after SetMaxSize(0)",
			cache.MaxSize(), DefaultMaxSizeMB*bytesPerMB)
	}

	cache.SetMaxSize(-5)
	if cache.MaxSize() != DefaultMaxSizeMB*bytesPerMB {
		t.Errorf("MaxSize() = %d, want %d after SetMaxSize(-5)",
			cache.MaxSize(), DefaultMaxSizeMB*bytesPerMB)
	}
}

func TestLayerCache_GetVersion(t *testing.T) {
	cache := NewLayerCache(1)

	// Non-existent entry
	version, ok := cache.GetVersion(12345)
	if ok {
		t.Error("GetVersion should return false for non-existent entry")
	}
	if version != 0 {
		t.Errorf("version = %d, want 0 for non-existent entry", version)
	}

	// Add entry with version
	cache.Put(12345, newTestPixmap(50, 50), 42)
	version, ok = cache.GetVersion(12345)
	if !ok {
		t.Error("GetVersion should return true for existing entry")
	}
	if version != 42 {
		t.Errorf("version = %d, want 42", version)
	}
}

func TestLayerCache_ResetStats(t *testing.T) {
	cache := NewLayerCache(1)
	cache.Put(1, newTestPixmap(50, 50), 1)

	// Generate some stats
	_, _ = cache.Get(1) // hit
	_, _ = cache.Get(2) // miss
	cache.Invalidate(1) // eviction

	stats := cache.Stats()
	if stats.Hits == 0 && stats.Misses == 0 && stats.Evictions == 0 {
		t.Error("Stats should have values before reset")
	}

	cache.ResetStats()
	stats = cache.Stats()
	if stats.Hits != 0 {
		t.Errorf("Hits = %d, want 0 after reset", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("Misses = %d, want 0 after reset", stats.Misses)
	}
	if stats.Evictions != 0 {
		t.Errorf("Evictions = %d, want 0 after reset", stats.Evictions)
	}
}

func TestLayerCache_OversizedEntry(t *testing.T) {
	// Create a 1KB cache
	cache := &LayerCache{
		entries: make(map[uint64]*CacheEntry),
		lru:     newList(),
		maxSize: 1024, // 1KB
	}

	// Try to add a 10KB entry (100x25 = 10,000 bytes)
	cache.Put(1, newTestPixmap(100, 25), 1)

	// Entry should not be added
	if cache.EntryCount() != 0 {
		t.Error("Oversized entry should not be added to cache")
	}
	if cache.Size() != 0 {
		t.Errorf("Size() = %d, want 0", cache.Size())
	}
}

func TestLayerCache_Concurrent(t *testing.T) {
	cache := NewLayerCache(10) // 10MB
	var wg sync.WaitGroup
	numGoroutines := 100
	opsPerGoroutine := 100

	wg.Add(numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				hash := uint64(id*opsPerGoroutine + i)
				pixmap := newTestPixmap(10, 10) // Small pixmaps

				// Mix of operations
				switch i % 5 {
				case 0:
					cache.Put(hash, pixmap, 1)
				case 1:
					_, _ = cache.Get(hash)
				case 2:
					_ = cache.Contains(hash)
				case 3:
					_ = cache.Stats()
				case 4:
					cache.Invalidate(hash)
				}
			}
		}(g)
	}

	wg.Wait()

	// Cache should be in consistent state
	stats := cache.Stats()
	if stats.Size < 0 {
		t.Error("Size should not be negative")
	}
	if stats.Entries < 0 {
		t.Error("Entries should not be negative")
	}
}

func TestLayerCache_ConcurrentGetAfterPut(t *testing.T) {
	cache := NewLayerCache(10)
	var wg sync.WaitGroup

	// Pre-populate cache
	for i := uint64(0); i < 100; i++ {
		cache.Put(i, newTestPixmap(10, 10), 1)
	}

	// Concurrent reads and writes
	numReaders := 50
	numWriters := 10

	wg.Add(numReaders + numWriters)

	// Readers
	for r := 0; r < numReaders; r++ {
		go func() {
			defer wg.Done()
			for i := uint64(0); i < 100; i++ {
				_, _ = cache.Get(i)
			}
		}()
	}

	// Writers
	for w := 0; w < numWriters; w++ {
		go func(id int) {
			defer wg.Done()
			for i := uint64(0); i < 10; i++ {
				hash := uint64(1000 + id*10 + int(i))
				cache.Put(hash, newTestPixmap(10, 10), 1)
			}
		}(w)
	}

	wg.Wait()

	// Should complete without deadlock or panic
	if cache.Size() < 0 {
		t.Error("Size should not be negative after concurrent operations")
	}
}

func TestLayerCache_SizeCalculation(t *testing.T) {
	cache := NewLayerCache(1)

	tests := []struct {
		width    int
		height   int
		wantSize int64
	}{
		{100, 100, 40000},
		{256, 256, 262144},
		{1, 1, 4},
		{0, 100, 0}, // Zero dimension should result in zero size (won't be added)
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if tt.width == 0 || tt.height == 0 {
				// Skip zero dimension test for Put
				return
			}
			cache.InvalidateAll()
			cache.Put(1, newTestPixmap(tt.width, tt.height), 1)
			if cache.Size() != tt.wantSize {
				t.Errorf("Size for %dx%d = %d, want %d",
					tt.width, tt.height, cache.Size(), tt.wantSize)
			}
		})
	}
}

func TestPixmapSize(t *testing.T) {
	tests := []struct {
		pixmap *gg.Pixmap
		want   int64
	}{
		{nil, 0},
		{newTestPixmap(10, 10), 400},
		{newTestPixmap(100, 100), 40000},
		{newTestPixmap(256, 256), 262144},
	}

	for _, tt := range tests {
		got := pixmapSize(tt.pixmap)
		if got != tt.want {
			t.Errorf("pixmapSize() = %d, want %d", got, tt.want)
		}
	}
}

// newList creates a new list.List for testing.
func newList() *list.List {
	return list.New()
}

// newRectPath creates a path with a rectangle for testing.
func newRectPath(x, y, w, h float64) *gg.Path {
	p := gg.NewPath()
	p.Rectangle(x, y, w, h)
	return p
}

func TestEncoding_Hash(t *testing.T) {
	// Test that Hash produces consistent results
	enc1 := NewEncoding()
	enc1.EncodePath(newRectPath(0, 0, 100, 100))
	enc1.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	enc2 := NewEncoding()
	enc2.EncodePath(newRectPath(0, 0, 100, 100))
	enc2.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	enc3 := NewEncoding()
	enc3.EncodePath(newRectPath(0, 0, 200, 200)) // Different
	enc3.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	hash1 := enc1.Hash()
	hash2 := enc2.Hash()
	hash3 := enc3.Hash()

	if hash1 != hash2 {
		t.Errorf("Same encoding should produce same hash: %d != %d", hash1, hash2)
	}
	if hash1 == hash3 {
		t.Error("Different encodings should produce different hashes")
	}
	if hash1 == 0 {
		t.Error("Hash should not be zero for non-empty encoding")
	}
}

func TestEncoding_HashEmpty(t *testing.T) {
	enc := NewEncoding()
	hash := enc.Hash()

	// Empty encoding should still produce a deterministic hash
	enc2 := NewEncoding()
	hash2 := enc2.Hash()

	if hash != hash2 {
		t.Error("Empty encodings should produce same hash")
	}
}

// Benchmarks

func BenchmarkLayerCache_Get_Hit(b *testing.B) {
	cache := NewLayerCache(64)
	hash := uint64(12345)
	cache.Put(hash, newTestPixmap(100, 100), 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Get(hash)
	}
}

func BenchmarkLayerCache_Get_Miss(b *testing.B) {
	cache := NewLayerCache(64)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Get(uint64(i))
	}
}

func BenchmarkLayerCache_Put(b *testing.B) {
	cache := NewLayerCache(64)
	pixmaps := make([]*gg.Pixmap, b.N)
	for i := range pixmaps {
		pixmaps[i] = newTestPixmap(50, 50)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(uint64(i), pixmaps[i], 1)
	}
}

func BenchmarkLayerCache_Stats(b *testing.B) {
	cache := NewLayerCache(64)
	cache.Put(1, newTestPixmap(100, 100), 1)
	_, _ = cache.Get(1)
	_, _ = cache.Get(2) // miss

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Stats()
	}
}

func BenchmarkLayerCache_Concurrent(b *testing.B) {
	cache := NewLayerCache(64)
	var wg sync.WaitGroup

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		wg.Add(4)
		go func(id int) {
			defer wg.Done()
			cache.Put(uint64(id), newTestPixmap(10, 10), 1)
		}(i)
		go func(id int) {
			defer wg.Done()
			_, _ = cache.Get(uint64(id))
		}(i)
		go func() {
			defer wg.Done()
			_ = cache.Stats()
		}()
		go func() {
			defer wg.Done()
			_ = cache.Size()
		}()
	}
	wg.Wait()
}

func BenchmarkEncoding_Hash(b *testing.B) {
	enc := NewEncoding()
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 0)
	path.LineTo(100, 100)
	path.LineTo(0, 100)
	path.Close()
	enc.EncodePath(path)
	enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = enc.Hash()
	}
}
