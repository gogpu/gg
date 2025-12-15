package text

import (
	"sync"
	"testing"
)

// BenchmarkCacheHit benchmarks cache hit performance.
func BenchmarkCacheHit(b *testing.B) {
	cache := NewCache[string, int](1000)

	// Pre-populate cache
	for i := 0; i < 100; i++ {
		cache.Set(string(rune('a'+i)), i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + (i % 100)))
		_, _ = cache.Get(key)
	}
}

// BenchmarkCacheMiss benchmarks cache miss performance.
func BenchmarkCacheMiss(b *testing.B) {
	cache := NewCache[string, int](1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + (i % 100)))
		_, _ = cache.Get(key)
	}
}

// BenchmarkCacheSet benchmarks cache Set performance.
func BenchmarkCacheSet(b *testing.B) {
	cache := NewCache[string, int](1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + (i % 100)))
		cache.Set(key, i)
	}
}

// BenchmarkCacheGetOrCreate benchmarks GetOrCreate with cache hits.
func BenchmarkCacheGetOrCreate(b *testing.B) {
	cache := NewCache[string, int](1000)

	// Pre-populate cache
	for i := 0; i < 100; i++ {
		cache.Set(string(rune('a'+i)), i)
	}

	create := func() int { return 42 }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + (i % 100)))
		_ = cache.GetOrCreate(key, create)
	}
}

// BenchmarkCacheGetOrCreateMiss benchmarks GetOrCreate with cache misses.
func BenchmarkCacheGetOrCreateMiss(b *testing.B) {
	cache := NewCache[int, int](1000)

	create := func() int { return 42 }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.GetOrCreate(i, create)
	}
}

// BenchmarkCacheEviction benchmarks cache eviction performance.
func BenchmarkCacheEviction(b *testing.B) {
	cache := NewCache[int, int](100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(i, i*2)
	}
}

// BenchmarkCacheConcurrent benchmarks concurrent cache access.
func BenchmarkCacheConcurrent(b *testing.B) {
	cache := NewCache[int, int](1000)

	// Pre-populate
	for i := 0; i < 100; i++ {
		cache.Set(i, i*2)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := i % 100
			if i%2 == 0 {
				_, _ = cache.Get(key)
			} else {
				cache.Set(key, i)
			}
			i++
		}
	})
}

// BenchmarkRuneToBoolMapGet benchmarks Get performance.
func BenchmarkRuneToBoolMapGet(b *testing.B) {
	m := NewRuneToBoolMap()

	// Pre-populate with common ASCII range
	for r := rune('A'); r <= 'Z'; r++ {
		m.Set(r, true)
	}
	for r := rune('a'); r <= 'z'; r++ {
		m.Set(r, true)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := rune('A' + (i % 26))
		_, _ = m.Get(r)
	}
}

// BenchmarkRuneToBoolMapSet benchmarks Set performance.
func BenchmarkRuneToBoolMapSet(b *testing.B) {
	m := NewRuneToBoolMap()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := rune('A' + (i % 26))
		m.Set(r, i%2 == 0)
	}
}

// BenchmarkRuneToBoolMapSparse benchmarks with sparse Unicode access.
func BenchmarkRuneToBoolMapSparse(b *testing.B) {
	m := NewRuneToBoolMap()

	// Sparse runes across different Unicode blocks
	runes := []rune{
		'A',          // ASCII
		'Ω',          // Greek
		'中',          // CJK
		'🎨',          // Emoji
		'\U0001F600', // Another emoji
	}

	// Pre-populate
	for _, r := range runes {
		m.Set(r, true)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := runes[i%len(runes)]
		_, _ = m.Get(r)
	}
}

// BenchmarkRuneToBoolMapConcurrent benchmarks concurrent access.
func BenchmarkRuneToBoolMapConcurrent(b *testing.B) {
	m := NewRuneToBoolMap()

	// Pre-populate
	for r := rune('A'); r <= 'Z'; r++ {
		m.Set(r, true)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			r := rune('A' + (i % 26))
			if i%2 == 0 {
				_, _ = m.Get(r)
			} else {
				m.Set(r, true)
			}
			i++
		}
	})
}

// BenchmarkShapingKeyCache benchmarks using ShapingKey in cache.
func BenchmarkShapingKeyCache(b *testing.B) {
	cache := NewCache[ShapingKey, []Glyph](1000)

	// Pre-populate with common strings
	texts := []string{"Hello", "World", "Test", "Cache", "Performance"}
	for _, text := range texts {
		key := ShapingKey{
			Text:      text,
			Size:      12.0,
			Direction: DirectionLTR,
		}
		glyphs := []Glyph{{Rune: rune(text[0])}}
		cache.Set(key, glyphs)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := ShapingKey{
			Text:      texts[i%len(texts)],
			Size:      12.0,
			Direction: DirectionLTR,
		}
		_, _ = cache.Get(key)
	}
}

// BenchmarkGlyphKeyCache benchmarks using GlyphKey in cache.
func BenchmarkGlyphKeyCache(b *testing.B) {
	cache := NewCache[GlyphKey, int](1000)

	// Pre-populate with common glyph IDs
	for gid := GlyphID(1); gid <= 100; gid++ {
		key := GlyphKey{GID: gid, Size: 12.0}
		cache.Set(key, int(gid)*2)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := GlyphKey{
			GID:  GlyphID(1 + (i % 100)),
			Size: 12.0,
		}
		_, _ = cache.Get(key)
	}
}

// BenchmarkCacheVsMap compares cache performance to sync.Map.
func BenchmarkCacheVsMap(b *testing.B) {
	b.Run("Cache", func(b *testing.B) {
		cache := NewCache[string, int](1000)
		for i := 0; i < 100; i++ {
			cache.Set(string(rune('a'+i)), i)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := string(rune('a' + (i % 100)))
			_, _ = cache.Get(key)
		}
	})

	b.Run("SyncMap", func(b *testing.B) {
		var m sync.Map
		for i := 0; i < 100; i++ {
			m.Store(string(rune('a'+i)), i)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := string(rune('a' + (i % 100)))
			_, _ = m.Load(key)
		}
	})
}
