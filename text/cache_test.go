package text

import (
	"sync"
	"testing"
)

// TestCacheBasicOperations tests basic Get/Set operations.
func TestCacheBasicOperations(t *testing.T) {
	cache := NewCache[string, int](0) // Unlimited

	// Test Get on empty cache
	if _, ok := cache.Get("key1"); ok {
		t.Error("Expected Get to return false for non-existent key")
	}

	// Test Set and Get
	cache.Set("key1", 42)
	if val, ok := cache.Get("key1"); !ok || val != 42 {
		t.Errorf("Expected Get to return (42, true), got (%v, %v)", val, ok)
	}

	// Test overwrite
	cache.Set("key1", 100)
	if val, ok := cache.Get("key1"); !ok || val != 100 {
		t.Errorf("Expected Get to return (100, true), got (%v, %v)", val, ok)
	}

	// Test multiple keys
	cache.Set("key2", 200)
	cache.Set("key3", 300)

	if val, ok := cache.Get("key2"); !ok || val != 200 {
		t.Errorf("Expected Get(key2) to return (200, true), got (%v, %v)", val, ok)
	}
	if val, ok := cache.Get("key3"); !ok || val != 300 {
		t.Errorf("Expected Get(key3) to return (300, true), got (%v, %v)", val, ok)
	}
}

// TestCacheGetOrCreate tests GetOrCreate functionality.
func TestCacheGetOrCreate(t *testing.T) {
	cache := NewCache[string, int](0) // Unlimited

	createCount := 0
	create := func() int {
		createCount++
		return 42
	}

	// First call should create
	val := cache.GetOrCreate("key1", create)
	if val != 42 {
		t.Errorf("Expected GetOrCreate to return 42, got %v", val)
	}
	if createCount != 1 {
		t.Errorf("Expected create to be called once, got %d", createCount)
	}

	// Second call should use cached value
	val = cache.GetOrCreate("key1", create)
	if val != 42 {
		t.Errorf("Expected GetOrCreate to return 42, got %v", val)
	}
	if createCount != 1 {
		t.Errorf("Expected create to not be called again, got %d calls", createCount)
	}

	// Different key should create again
	val = cache.GetOrCreate("key2", create)
	if val != 42 {
		t.Errorf("Expected GetOrCreate to return 42, got %v", val)
	}
	if createCount != 2 {
		t.Errorf("Expected create to be called twice, got %d", createCount)
	}
}

// TestCacheLRUEviction tests LRU eviction logic.
func TestCacheLRUEviction(t *testing.T) {
	cache := NewCache[string, int](10) // Soft limit of 10

	// Fill cache beyond soft limit
	for i := 0; i < 20; i++ {
		key := string(rune('a' + i))
		cache.Set(key, i)
	}

	// Cache should have evicted oldest entries
	if size := cache.Len(); size > 10 {
		t.Errorf("Expected cache size <= 10 after eviction, got %d", size)
	}

	// Most recent entries should still be present
	// Last few entries should be in cache
	if _, ok := cache.Get("t"); !ok { // 't' is 19th entry (index 19)
		t.Error("Expected recent entry 't' to be in cache")
	}
	if _, ok := cache.Get("s"); !ok { // 's' is 18th entry
		t.Error("Expected recent entry 's' to be in cache")
	}

	// Oldest entries should be evicted
	if _, ok := cache.Get("a"); ok { // 'a' is first entry
		t.Error("Expected oldest entry 'a' to be evicted")
	}
}

// TestCacheLRUAccessUpdate tests that Get updates access time.
func TestCacheLRUAccessUpdate(t *testing.T) {
	cache := NewCache[string, int](5)

	// Fill cache to soft limit
	cache.Set("a", 1)
	cache.Set("b", 2)
	cache.Set("c", 3)
	cache.Set("d", 4)
	cache.Set("e", 5)

	// Access "a" to make it recent
	_, _ = cache.Get("a")

	// Add more entries to trigger eviction
	cache.Set("f", 6)
	cache.Set("g", 7)

	// "a" should still be in cache (was accessed recently)
	if _, ok := cache.Get("a"); !ok {
		t.Error("Expected recently accessed entry 'a' to still be in cache")
	}

	// "b" should be evicted (oldest unaccessed)
	if _, ok := cache.Get("b"); ok {
		t.Error("Expected oldest unaccessed entry 'b' to be evicted")
	}
}

// TestCacheClear tests Clear functionality.
func TestCacheClear(t *testing.T) {
	cache := NewCache[string, int](0)

	cache.Set("key1", 1)
	cache.Set("key2", 2)
	cache.Set("key3", 3)

	if size := cache.Len(); size != 3 {
		t.Errorf("Expected cache size 3, got %d", size)
	}

	cache.Clear()

	if size := cache.Len(); size != 0 {
		t.Errorf("Expected cache size 0 after Clear, got %d", size)
	}

	if _, ok := cache.Get("key1"); ok {
		t.Error("Expected key1 to be gone after Clear")
	}
}

// TestCacheThreadSafety tests concurrent access.
func TestCacheThreadSafety(t *testing.T) {
	cache := NewCache[int, int](100)

	const numGoroutines = 10
	const numOps = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()

			for i := 0; i < numOps; i++ {
				key := id*numOps + i
				cache.Set(key, key*2)
				_, _ = cache.Get(key)
			}
		}(g)
	}

	wg.Wait()

	// No assertion needed - just checking for race conditions
	// Run with: go test -race
}

// TestRuneToBoolMapBasic tests basic Get/Set operations.
func TestRuneToBoolMapBasic(t *testing.T) {
	m := NewRuneToBoolMap()

	// Test Get on empty map
	hasGlyph, checked := m.Get('A')
	if checked {
		t.Error("Expected unchecked rune to return checked=false")
	}
	if hasGlyph {
		t.Error("Expected unchecked rune to return hasGlyph=false")
	}

	// Test Set and Get (hasGlyph=true)
	m.Set('A', true)
	hasGlyph, checked = m.Get('A')
	if !checked {
		t.Error("Expected checked rune to return checked=true")
	}
	if !hasGlyph {
		t.Error("Expected hasGlyph=true")
	}

	// Test Set and Get (hasGlyph=false)
	m.Set('B', false)
	hasGlyph, checked = m.Get('B')
	if !checked {
		t.Error("Expected checked rune to return checked=true")
	}
	if hasGlyph {
		t.Error("Expected hasGlyph=false")
	}

	// Test overwrite
	m.Set('A', false)
	hasGlyph, checked = m.Get('A')
	if !checked {
		t.Error("Expected checked rune to return checked=true")
	}
	if hasGlyph {
		t.Error("Expected overwritten hasGlyph=false")
	}
}

// TestRuneToBoolMapSparseAccess tests memory efficiency with sparse runes.
func TestRuneToBoolMapSparseAccess(t *testing.T) {
	m := NewRuneToBoolMap()

	// Test runes from different Unicode blocks
	testRunes := []rune{
		'A',          // ASCII (block 0)
		'Ω',          // Greek (block 3)
		'中',          // CJK (block 78)
		'🎨',          // Emoji (block 499)
		'\U0001F600', // Another emoji
	}

	for i, r := range testRunes {
		expected := i%2 == 0 // Alternate true/false
		m.Set(r, expected)
	}

	// Verify all values
	for i, r := range testRunes {
		expected := i%2 == 0
		hasGlyph, checked := m.Get(r)
		if !checked {
			t.Errorf("Expected rune %U to be checked", r)
		}
		if hasGlyph != expected {
			t.Errorf("Expected rune %U hasGlyph=%v, got %v", r, expected, hasGlyph)
		}
	}

	// Verify that only accessed blocks are allocated
	if len(m.blocks) != len(testRunes) {
		t.Errorf("Expected %d blocks allocated, got %d", len(testRunes), len(m.blocks))
	}
}

// TestRuneToBoolMapClear tests Clear functionality.
func TestRuneToBoolMapClear(t *testing.T) {
	m := NewRuneToBoolMap()

	m.Set('A', true)
	m.Set('B', false)
	m.Set('中', true)

	if len(m.blocks) == 0 {
		t.Error("Expected blocks to be allocated")
	}

	m.Clear()

	if len(m.blocks) != 0 {
		t.Errorf("Expected 0 blocks after Clear, got %d", len(m.blocks))
	}

	hasGlyph, checked := m.Get('A')
	if checked {
		t.Error("Expected rune 'A' to be unchecked after Clear")
	}
	if hasGlyph {
		t.Error("Expected rune 'A' hasGlyph=false after Clear")
	}
}

// TestRuneToBoolMapThreadSafety tests concurrent access.
func TestRuneToBoolMapThreadSafety(t *testing.T) {
	m := NewRuneToBoolMap()

	const numGoroutines = 10
	const numOps = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()

			for i := 0; i < numOps; i++ {
				r := rune(id*numOps + i)
				m.Set(r, i%2 == 0)
				_, _ = m.Get(r)
			}
		}(g)
	}

	wg.Wait()

	// No assertion needed - just checking for race conditions
	// Run with: go test -race
}

// TestShapingKey tests ShapingKey as cache key.
func TestShapingKey(t *testing.T) {
	cache := NewCache[ShapingKey, []Glyph](0)

	key1 := ShapingKey{
		Text:      "Hello",
		Size:      12.0,
		Direction: DirectionLTR,
	}

	key2 := ShapingKey{
		Text:      "Hello",
		Size:      12.0,
		Direction: DirectionLTR,
	}

	key3 := ShapingKey{
		Text:      "Hello",
		Size:      14.0, // Different size
		Direction: DirectionLTR,
	}

	glyphs := []Glyph{{Rune: 'H'}}

	cache.Set(key1, glyphs)

	// key2 should match key1 (same values)
	if val, ok := cache.Get(key2); !ok || len(val) != 1 || val[0].Rune != 'H' {
		t.Error("Expected key2 to match key1")
	}

	// key3 should not match (different size)
	if _, ok := cache.Get(key3); ok {
		t.Error("Expected key3 to not match key1")
	}
}

// TestGlyphKey tests GlyphKey as cache key.
func TestGlyphKey(t *testing.T) {
	cache := NewCache[GlyphKey, int](0)

	key1 := GlyphKey{GID: 42, Size: 12.0}
	key2 := GlyphKey{GID: 42, Size: 12.0}
	key3 := GlyphKey{GID: 43, Size: 12.0} // Different GID

	cache.Set(key1, 100)

	// key2 should match key1
	if val, ok := cache.Get(key2); !ok || val != 100 {
		t.Error("Expected key2 to match key1")
	}

	// key3 should not match
	if _, ok := cache.Get(key3); ok {
		t.Error("Expected key3 to not match key1")
	}
}
