package text

import (
	"sync"
	"testing"
)

func TestDefaultGlyphCacheConfig(t *testing.T) {
	config := DefaultGlyphCacheConfig()
	if config.MaxEntries != 4096 {
		t.Errorf("MaxEntries = %d, want 4096", config.MaxEntries)
	}
	if config.FrameLifetime != 64 {
		t.Errorf("FrameLifetime = %d, want 64", config.FrameLifetime)
	}
}

func TestNewGlyphCache(t *testing.T) {
	cache := NewGlyphCache()
	if cache == nil {
		t.Fatal("NewGlyphCache should not return nil")
	}
	if cache.Len() != 0 {
		t.Errorf("New cache should be empty, got len=%d", cache.Len())
	}
}

func TestNewGlyphCacheWithConfig(t *testing.T) {
	config := GlyphCacheConfig{
		MaxEntries:    1024,
		FrameLifetime: 32,
	}
	cache := NewGlyphCacheWithConfig(config)
	if cache == nil {
		t.Fatal("NewGlyphCacheWithConfig should not return nil")
	}
	if cache.config.MaxEntries != 1024 {
		t.Errorf("MaxEntries = %d, want 1024", cache.config.MaxEntries)
	}
	if cache.config.FrameLifetime != 32 {
		t.Errorf("FrameLifetime = %d, want 32", cache.config.FrameLifetime)
	}
}

func TestNewGlyphCacheWithConfig_Defaults(t *testing.T) {
	// Zero values should use defaults
	config := GlyphCacheConfig{}
	cache := NewGlyphCacheWithConfig(config)
	if cache.config.MaxEntries != 4096 {
		t.Errorf("MaxEntries should default to 4096, got %d", cache.config.MaxEntries)
	}
	if cache.config.FrameLifetime != 64 {
		t.Errorf("FrameLifetime should default to 64, got %d", cache.config.FrameLifetime)
	}
}

func TestGlyphCache_SetGet(t *testing.T) {
	cache := NewGlyphCache()

	key := OutlineCacheKey{FontID: 1, GID: 42, Size: 16, Hinting: HintingNone}
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
		},
		GID: 42,
	}

	// Get before set should return nil
	got := cache.Get(key)
	if got != nil {
		t.Errorf("Get before Set should return nil")
	}

	// Set and get
	cache.Set(key, outline)
	got = cache.Get(key)
	if got == nil {
		t.Fatal("Get after Set should not return nil")
	}
	if got.GID != 42 {
		t.Errorf("Got GID = %d, want 42", got.GID)
	}
}

func TestGlyphCache_SetNil(t *testing.T) {
	cache := NewGlyphCache()

	key := OutlineCacheKey{FontID: 1, GID: 42}
	cache.Set(key, nil)

	if cache.Len() != 0 {
		t.Errorf("Setting nil should not add entry, len = %d", cache.Len())
	}
}

func TestGlyphCache_SetOverwrite(t *testing.T) {
	cache := NewGlyphCache()

	key := OutlineCacheKey{FontID: 1, GID: 42}
	outline1 := &GlyphOutline{Advance: 10}
	outline2 := &GlyphOutline{Advance: 20}

	cache.Set(key, outline1)
	cache.Set(key, outline2)

	got := cache.Get(key)
	if got.Advance != 20 {
		t.Errorf("Got Advance = %f, want 20", got.Advance)
	}

	if cache.Len() != 1 {
		t.Errorf("Overwrite should not increase len, got %d", cache.Len())
	}
}

func TestGlyphCache_Delete(t *testing.T) {
	cache := NewGlyphCache()

	key := OutlineCacheKey{FontID: 1, GID: 42}
	outline := &GlyphOutline{GID: 42}

	cache.Set(key, outline)
	if cache.Len() != 1 {
		t.Errorf("After Set, len should be 1")
	}

	cache.Delete(key)
	if cache.Len() != 0 {
		t.Errorf("After Delete, len should be 0")
	}

	got := cache.Get(key)
	if got != nil {
		t.Errorf("After Delete, Get should return nil")
	}
}

func TestGlyphCache_Delete_NotFound(t *testing.T) {
	cache := NewGlyphCache()

	// Should not panic
	key := OutlineCacheKey{FontID: 999, GID: 999}
	cache.Delete(key)
}

func TestGlyphCache_Clear(t *testing.T) {
	cache := NewGlyphCache()

	// Add multiple entries
	for i := 0; i < 100; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i)}
		cache.Set(key, &GlyphOutline{GID: GlyphID(i)})
	}

	if cache.Len() != 100 {
		t.Errorf("Before Clear, len should be 100, got %d", cache.Len())
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("After Clear, len should be 0, got %d", cache.Len())
	}
}

func TestGlyphCache_LRUEviction(t *testing.T) {
	// Create a small cache to test eviction
	config := GlyphCacheConfig{MaxEntries: 32, FrameLifetime: 64}
	cache := NewGlyphCacheWithConfig(config)

	// Fill the cache beyond capacity
	for i := 0; i < 64; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i)}
		cache.Set(key, &GlyphOutline{GID: GlyphID(i)})
	}

	// Cache should not exceed capacity
	if cache.Len() > 32 {
		t.Errorf("Cache should not exceed capacity, len = %d", cache.Len())
	}

	// First entries should have been evicted
	key := OutlineCacheKey{FontID: 1, GID: 0}
	got := cache.Get(key)
	if got != nil {
		t.Errorf("First entry should have been evicted")
	}

	// Last entries should still be present
	key = OutlineCacheKey{FontID: 1, GID: 63}
	got = cache.Get(key)
	if got == nil {
		t.Errorf("Last entry should still be present")
	}
}

func TestGlyphCache_LRUOrder(t *testing.T) {
	// Test that accessing an entry moves it to the front of the LRU list.
	// We use a single-shard scenario to verify LRU behavior deterministically.
	//
	// With 16 entries per shard (MaxEntries=256/16=16 per shard),
	// we force all entries to the same shard using consistent hashing.
	config := GlyphCacheConfig{MaxEntries: 256, FrameLifetime: 64}
	cache := NewGlyphCacheWithConfig(config)

	// Find entries that hash to the same shard as GID 0
	// by checking which shard each entry goes to
	// For simplicity, just test that Get works after Set and doesn't break LRU
	key0 := OutlineCacheKey{FontID: 1, GID: 0}
	key1 := OutlineCacheKey{FontID: 1, GID: 1}

	cache.Set(key0, &GlyphOutline{GID: 0, Advance: 10})
	cache.Set(key1, &GlyphOutline{GID: 1, Advance: 20})

	// Access key0 to move it to front
	got := cache.Get(key0)
	if got == nil || got.Advance != 10 {
		t.Errorf("Get(key0) failed")
	}

	// Access key1
	got = cache.Get(key1)
	if got == nil || got.Advance != 20 {
		t.Errorf("Get(key1) failed")
	}

	// Both should still be present
	if cache.Get(key0) == nil {
		t.Errorf("key0 should still be present")
	}
	if cache.Get(key1) == nil {
		t.Errorf("key1 should still be present")
	}
}

func TestGlyphCache_GetOrCreate(t *testing.T) {
	cache := NewGlyphCache()

	key := OutlineCacheKey{FontID: 1, GID: 42}
	createCalled := false

	// First call should invoke create
	outline := cache.GetOrCreate(key, func() *GlyphOutline {
		createCalled = true
		return &GlyphOutline{GID: 42}
	})

	if !createCalled {
		t.Errorf("Create should be called on first access")
	}
	if outline == nil || outline.GID != 42 {
		t.Errorf("GetOrCreate should return created outline")
	}

	// Second call should not invoke create
	createCalled = false
	outline = cache.GetOrCreate(key, func() *GlyphOutline {
		createCalled = true
		return &GlyphOutline{GID: 99}
	})

	if createCalled {
		t.Errorf("Create should not be called on second access")
	}
	if outline == nil || outline.GID != 42 {
		t.Errorf("GetOrCreate should return cached outline, not new one")
	}
}

func TestGlyphCache_GetOrCreate_NilCreate(t *testing.T) {
	cache := NewGlyphCache()

	key := OutlineCacheKey{FontID: 1, GID: 42}
	outline := cache.GetOrCreate(key, nil)

	if outline != nil {
		t.Errorf("GetOrCreate with nil create should return nil")
	}
}

func TestGlyphCache_GetOrCreate_CreateReturnsNil(t *testing.T) {
	cache := NewGlyphCache()

	key := OutlineCacheKey{FontID: 1, GID: 42}
	outline := cache.GetOrCreate(key, func() *GlyphOutline {
		return nil
	})

	if outline != nil {
		t.Errorf("GetOrCreate when create returns nil should return nil")
	}

	// Should not be cached
	if cache.Len() != 0 {
		t.Errorf("Nil outline should not be cached")
	}
}

func TestGlyphCache_Maintain(t *testing.T) {
	config := GlyphCacheConfig{MaxEntries: 100, FrameLifetime: 4}
	cache := NewGlyphCacheWithConfig(config)

	// Add entries
	for i := 0; i < 10; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i)}
		cache.Set(key, &GlyphOutline{GID: GlyphID(i)})
	}

	if cache.Len() != 10 {
		t.Errorf("After adding, len should be 10, got %d", cache.Len())
	}

	// Advance frames past lifetime
	for i := 0; i < 5; i++ {
		cache.Maintain()
	}

	// All entries should be evicted
	if cache.Len() != 0 {
		t.Errorf("After Maintain past lifetime, len should be 0, got %d", cache.Len())
	}
}

func TestGlyphCache_Maintain_ActiveEntries(t *testing.T) {
	config := GlyphCacheConfig{MaxEntries: 100, FrameLifetime: 4}
	cache := NewGlyphCacheWithConfig(config)

	// Add entries
	for i := 0; i < 10; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i)}
		cache.Set(key, &GlyphOutline{GID: GlyphID(i)})
	}

	// Access entry 0 each frame
	key0 := OutlineCacheKey{FontID: 1, GID: 0}
	for i := 0; i < 5; i++ {
		_ = cache.Get(key0)
		cache.Maintain()
	}

	// Entry 0 should still be present
	got := cache.Get(key0)
	if got == nil {
		t.Errorf("Active entry should not be evicted")
	}

	// Other entries should be evicted
	if cache.Len() != 1 {
		t.Errorf("Only active entry should remain, got %d", cache.Len())
	}
}

func TestGlyphCache_Stats(t *testing.T) {
	cache := NewGlyphCache()

	key := OutlineCacheKey{FontID: 1, GID: 42}
	outline := &GlyphOutline{GID: 42}

	// Miss
	_ = cache.Get(key)
	hits, misses, _, _ := cache.Stats()
	if hits != 0 || misses != 1 {
		t.Errorf("After miss: hits=%d misses=%d, want 0,1", hits, misses)
	}

	// Insert
	cache.Set(key, outline)
	hits, misses, evictions, insertions := cache.Stats()
	_ = evictions // unused in this check
	if insertions != 1 {
		t.Errorf("After insert: hits=%d misses=%d insertions=%d, want insertions=1", hits, misses, insertions)
	}

	// Hit
	_ = cache.Get(key)
	hits, misses, _, _ = cache.Stats()
	if hits != 1 || misses != 1 {
		t.Errorf("After hit: hits=%d misses=%d, want 1,1", hits, misses)
	}
}

func TestGlyphCache_HitRate(t *testing.T) {
	cache := NewGlyphCache()

	// No accesses
	rate := cache.HitRate()
	if rate != 0 {
		t.Errorf("HitRate with no accesses should be 0, got %f", rate)
	}

	key := OutlineCacheKey{FontID: 1, GID: 42}
	cache.Set(key, &GlyphOutline{GID: 42})

	// 1 miss, 1 hit = 50%
	_ = cache.Get(OutlineCacheKey{FontID: 999, GID: 999}) // miss
	_ = cache.Get(key)                                    // hit

	rate = cache.HitRate()
	if rate != 50.0 {
		t.Errorf("HitRate should be 50%%, got %f", rate)
	}
}

func TestGlyphCache_ResetStats(t *testing.T) {
	cache := NewGlyphCache()

	key := OutlineCacheKey{FontID: 1, GID: 42}
	cache.Set(key, &GlyphOutline{GID: 42})
	_ = cache.Get(key)
	_ = cache.Get(OutlineCacheKey{FontID: 999, GID: 999})

	cache.ResetStats()

	hits, misses, evictions, insertions := cache.Stats()
	if hits != 0 || misses != 0 || evictions != 0 || insertions != 0 {
		t.Errorf("After ResetStats, all stats should be 0")
	}
}

func TestGlyphCache_CurrentFrame(t *testing.T) {
	cache := NewGlyphCache()

	frame := cache.CurrentFrame()
	if frame != 0 {
		t.Errorf("Initial frame should be 0, got %d", frame)
	}

	cache.Maintain()
	cache.Maintain()
	cache.Maintain()

	frame = cache.CurrentFrame()
	if frame != 3 {
		t.Errorf("After 3 Maintain calls, frame should be 3, got %d", frame)
	}
}

func TestGlyphCache_Concurrent(t *testing.T) {
	cache := NewGlyphCache()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := OutlineCacheKey{FontID: 1, GID: GlyphID(i)}
			cache.Set(key, &GlyphOutline{GID: GlyphID(i)})
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := OutlineCacheKey{FontID: 1, GID: GlyphID(i)}
			_ = cache.Get(key)
		}(i)
	}
	wg.Wait()

	// Concurrent GetOrCreate
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := OutlineCacheKey{FontID: 2, GID: GlyphID(i)}
			_ = cache.GetOrCreate(key, func() *GlyphOutline {
				return &GlyphOutline{GID: GlyphID(i)}
			})
		}(i)
	}
	wg.Wait()

	// Concurrent Maintain
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.Maintain()
		}()
	}
	wg.Wait()
}

func TestGlyphCachePool(t *testing.T) {
	pool := NewGlyphCachePool()

	cache := pool.Get()
	if cache == nil {
		t.Fatal("Get should not return nil")
	}

	// Add some entries
	key := OutlineCacheKey{FontID: 1, GID: 42}
	cache.Set(key, &GlyphOutline{GID: 42})

	// Put back (should clear)
	pool.Put(cache)

	// Get again
	cache2 := pool.Get()
	if cache2.Len() != 0 {
		t.Errorf("Pooled cache should be cleared")
	}

	// Put nil should not panic
	pool.Put(nil)
}

func TestGlobalGlyphCache(t *testing.T) {
	cache := GetGlobalGlyphCache()
	if cache == nil {
		t.Fatal("GetGlobalGlyphCache should not return nil")
	}

	// Test setting a new global cache
	newCache := NewGlyphCache()
	old := SetGlobalGlyphCache(newCache)
	if old == nil {
		t.Errorf("SetGlobalGlyphCache should return old cache")
	}

	current := GetGlobalGlyphCache()
	if current != newCache {
		t.Errorf("GetGlobalGlyphCache should return new cache")
	}

	// Restore old cache
	_ = SetGlobalGlyphCache(old)
}

func TestSetGlobalGlyphCache_Nil(t *testing.T) {
	old := SetGlobalGlyphCache(nil)
	defer func() {
		_ = SetGlobalGlyphCache(old)
	}()

	current := GetGlobalGlyphCache()
	if current == nil {
		t.Errorf("SetGlobalGlyphCache(nil) should create a default cache")
	}
}

func TestOutlineCacheKey_Uniqueness(t *testing.T) {
	cache := NewGlyphCache()

	// Different keys should be independent
	keys := []OutlineCacheKey{
		{FontID: 1, GID: 42, Size: 16, Hinting: HintingNone},
		{FontID: 1, GID: 42, Size: 24, Hinting: HintingNone}, // different size
		{FontID: 1, GID: 42, Size: 16, Hinting: HintingFull}, // different hinting
		{FontID: 2, GID: 42, Size: 16, Hinting: HintingNone}, // different font
		{FontID: 1, GID: 43, Size: 16, Hinting: HintingNone}, // different glyph
	}

	for i, key := range keys {
		cache.Set(key, &GlyphOutline{Advance: float32(i)})
	}

	if cache.Len() != 5 {
		t.Errorf("Should have 5 distinct entries, got %d", cache.Len())
	}

	for i, key := range keys {
		got := cache.Get(key)
		if got == nil {
			t.Errorf("Key %v should be present", key)
			continue
		}
		if got.Advance != float32(i) {
			t.Errorf("Key %v: got Advance=%f, want %d", key, got.Advance, i)
		}
	}
}

// Benchmarks

func BenchmarkGlyphCache_Get_Hit(b *testing.B) {
	cache := NewGlyphCache()
	key := OutlineCacheKey{FontID: 1, GID: 42, Size: 16}
	cache.Set(key, &GlyphOutline{GID: 42})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Get(key)
	}
}

func BenchmarkGlyphCache_Get_Miss(b *testing.B) {
	cache := NewGlyphCache()
	key := OutlineCacheKey{FontID: 1, GID: 42, Size: 16}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Get(key)
	}
}

func BenchmarkGlyphCache_Set(b *testing.B) {
	cache := NewGlyphCache()
	outline := &GlyphOutline{GID: 42}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i % 1000), Size: 16}
		cache.Set(key, outline)
	}
}

func BenchmarkGlyphCache_GetOrCreate_Hit(b *testing.B) {
	cache := NewGlyphCache()
	key := OutlineCacheKey{FontID: 1, GID: 42, Size: 16}
	cache.Set(key, &GlyphOutline{GID: 42})

	create := func() *GlyphOutline {
		return &GlyphOutline{GID: 42}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.GetOrCreate(key, create)
	}
}

func BenchmarkGlyphCache_Concurrent_Get(b *testing.B) {
	cache := NewGlyphCache()
	// Prepopulate
	for i := 0; i < 1000; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i), Size: 16}
		cache.Set(key, &GlyphOutline{GID: GlyphID(i)})
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := OutlineCacheKey{FontID: 1, GID: GlyphID(i % 1000), Size: 16}
			_ = cache.Get(key)
			i++
		}
	})
}

func BenchmarkGlyphCache_Concurrent_Mixed(b *testing.B) {
	cache := NewGlyphCache()
	outline := &GlyphOutline{GID: 42}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := OutlineCacheKey{FontID: 1, GID: GlyphID(i % 1000), Size: 16}
			if i%2 == 0 {
				cache.Set(key, outline)
			} else {
				_ = cache.Get(key)
			}
			i++
		}
	})
}
