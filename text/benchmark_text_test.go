package text

import (
	"testing"
)

// BenchmarkGlyphOutlineExtraction benchmarks outline extraction from fonts.
func BenchmarkGlyphOutlineExtraction(b *testing.B) {
	// Create a mock outline extractor scenario
	// Since we can't load real fonts in benchmarks easily, we test the cache path
	cache := NewGlyphCache()
	extractor := NewOutlineExtractor()

	// Pre-populate cache with a sample outline
	sampleOutline := createSampleOutline()
	key := OutlineCacheKey{FontID: 1, GID: 65, Size: 16, Hinting: HintingNone}
	cache.Set(key, sampleOutline)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Benchmark cache hit path (most common in real usage)
		_ = cache.Get(key)
	}

	// Avoid compiler optimization
	_ = extractor
}

// BenchmarkGlyphCacheHit benchmarks cache hit performance.
func BenchmarkGlyphCacheHit(b *testing.B) {
	cache := NewGlyphCache()

	// Pre-populate cache with entries
	for i := 0; i < 100; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i), Size: 16, Hinting: HintingNone}
		cache.Set(key, createSampleOutline())
	}

	// Benchmark key to look up (should be in cache)
	key := OutlineCacheKey{FontID: 1, GID: 50, Size: 16, Hinting: HintingNone}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		outline := cache.Get(key)
		if outline == nil {
			b.Fatal("expected cache hit")
		}
	}
}

// BenchmarkGlyphCacheMiss benchmarks cache miss performance.
func BenchmarkGlyphCacheMiss(b *testing.B) {
	cache := NewGlyphCache()

	// Pre-populate cache with entries, but we'll look for different keys
	for i := 0; i < 100; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i), Size: 16, Hinting: HintingNone}
		cache.Set(key, createSampleOutline())
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Miss: different font ID
		key := OutlineCacheKey{FontID: 2, GID: 50, Size: 16, Hinting: HintingNone}
		_ = cache.Get(key)
	}
}

// BenchmarkGlyphCacheGetOrCreate benchmarks GetOrCreate operation.
func BenchmarkGlyphCacheGetOrCreate(b *testing.B) {
	cache := NewGlyphCache()
	sampleOutline := createSampleOutline()

	b.Run("Hit", func(b *testing.B) {
		// Pre-populate
		key := OutlineCacheKey{FontID: 1, GID: 65, Size: 16, Hinting: HintingNone}
		cache.Set(key, sampleOutline)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = cache.GetOrCreate(key, func() *GlyphOutline {
				return sampleOutline
			})
		}
	})

	b.Run("Miss", func(b *testing.B) {
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			key := OutlineCacheKey{FontID: 3, GID: GlyphID(i % 256), Size: 16, Hinting: HintingNone}
			_ = cache.GetOrCreate(key, func() *GlyphOutline {
				return sampleOutline
			})
		}
	})
}

// BenchmarkRenderGlyphs benchmarks rendering glyphs to outlines.
func BenchmarkRenderGlyphs(b *testing.B) {
	renderer := NewGlyphRenderer()
	params := DefaultRenderParams()

	// Create mock glyphs and cache their outlines
	glyphs := make([]ShapedGlyph, 10)
	for i := range glyphs {
		glyphs[i] = ShapedGlyph{
			GID:      GlyphID(65 + i), // A, B, C, ...
			X:        float64(i) * 10,
			Y:        0,
			XAdvance: 10,
		}
	}

	// Pre-populate cache
	mockFont := &benchmarkMockFont{name: "BenchmarkFont", unitsPerEm: 2048, numGlyphs: 256}
	fontID := computeFontID(mockFont)
	for _, g := range glyphs {
		key := OutlineCacheKey{FontID: fontID, GID: g.GID, Size: 16, Hinting: HintingNone}
		renderer.cache.Set(key, createSampleOutline())
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = renderer.RenderGlyphs(glyphs, mockFont, 16, params)
	}
}

// BenchmarkRenderLayout benchmarks rendering a complete layout.
func BenchmarkRenderLayout(b *testing.B) {
	renderer := NewGlyphRenderer()
	params := DefaultRenderParams()

	// Create a simple layout with multiple lines
	layout := createBenchmarkLayout()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = renderer.RenderLayout(layout, params)
	}
}

// BenchmarkTransformCalculation benchmarks glyph transform calculation.
func BenchmarkTransformCalculation(b *testing.B) {
	renderer := NewGlyphRenderer()
	sampleOutline := createSampleOutline()

	b.Run("NoUserTransform", func(b *testing.B) {
		glyph := &ShapedGlyph{X: 100, Y: 50}
		params := DefaultRenderParams()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = renderer.transformOutline(sampleOutline, glyph, params)
		}
	})

	b.Run("WithUserTransform", func(b *testing.B) {
		glyph := &ShapedGlyph{X: 100, Y: 50}
		params := RenderParams{Transform: TranslateTransform(10, 20)}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = renderer.transformOutline(sampleOutline, glyph, params)
		}
	})

	b.Run("WithScaleTransform", func(b *testing.B) {
		glyph := &ShapedGlyph{X: 100, Y: 50}
		params := RenderParams{Transform: ScaleTransform(2, 2)}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = renderer.transformOutline(sampleOutline, glyph, params)
		}
	})
}

// BenchmarkComputeFontID benchmarks font ID computation.
func BenchmarkComputeFontID(b *testing.B) {
	font := &benchmarkMockFont{name: "BenchmarkFont", unitsPerEm: 2048, numGlyphs: 256}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = computeFontID(font)
	}
}

// BenchmarkTrigApprox benchmarks sine/cosine approximations.
func BenchmarkTrigApprox(b *testing.B) {
	angles := []float64{0, 0.5, 1.0, 1.5707, 3.14159}

	b.Run("Sine", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, a := range angles {
				_ = sineApprox(a)
			}
		}
	})

	b.Run("Cosine", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, a := range angles {
				_ = cosineApprox(a)
			}
		}
	})
}

// BenchmarkRenderParams benchmarks parameter operations.
func BenchmarkRenderParams(b *testing.B) {
	b.Run("Default", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = DefaultRenderParams()
		}
	})

	b.Run("WithOpacity", func(b *testing.B) {
		params := DefaultRenderParams()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = params.WithOpacity(0.5)
		}
	})

	b.Run("WithTransform", func(b *testing.B) {
		params := DefaultRenderParams()
		t := TranslateTransform(10, 20)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = params.WithTransform(t)
		}
	})
}

// BenchmarkCacheKeyHashing benchmarks cache key operations.
func BenchmarkCacheKeyHashing(b *testing.B) {
	cache := NewGlyphCache()

	b.Run("GetShard", func(b *testing.B) {
		key := OutlineCacheKey{FontID: 12345, GID: 65, Size: 16, Hinting: HintingNone}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = cache.getShard(key)
		}
	})
}

// Helper functions for benchmarks

func createSampleOutline() *GlyphOutline {
	return &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 10}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: 10}}},
		},
		Bounds:  Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10},
		Advance: 12,
		GID:     65,
		Type:    GlyphTypeOutline,
	}
}

func createBenchmarkLayout() *Layout {
	// Create a layout with 5 lines
	lines := make([]Line, 5)
	for i := range lines {
		glyphs := make([]ShapedGlyph, 10)
		for j := range glyphs {
			glyphs[j] = ShapedGlyph{
				GID:      GlyphID(65 + j),
				X:        float64(j) * 10,
				Y:        0,
				XAdvance: 10,
			}
		}

		lines[i] = Line{
			Glyphs:  glyphs,
			Width:   100,
			Ascent:  12,
			Descent: 4,
			Y:       float64(i) * 20,
		}
	}

	return &Layout{
		Lines:  lines,
		Width:  100,
		Height: 100,
	}
}

// benchmarkMockFont implements ParsedFont for benchmarks.
type benchmarkMockFont struct {
	name       string
	unitsPerEm int
	numGlyphs  int
}

func (m *benchmarkMockFont) Name() string                                { return m.name }
func (m *benchmarkMockFont) FullName() string                            { return m.name + " Regular" }
func (m *benchmarkMockFont) UnitsPerEm() int                             { return m.unitsPerEm }
func (m *benchmarkMockFont) NumGlyphs() int                              { return m.numGlyphs }
func (m *benchmarkMockFont) GlyphIndex(_ rune) uint16                    { return 1 }
func (m *benchmarkMockFont) GlyphAdvance(_ uint16, _ float64) float64    { return 10.0 }
func (m *benchmarkMockFont) GlyphBounds(_ uint16, _ float64) Rect        { return Rect{0, 0, 10, 10} }
func (m *benchmarkMockFont) Metrics(_ float64) FontMetrics               { return FontMetrics{} }

// BenchmarkConcurrentCacheAccess benchmarks concurrent cache operations.
func BenchmarkConcurrentCacheAccess(b *testing.B) {
	cache := NewGlyphCache()

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i % 256), Size: int16(12 + i%10), Hinting: HintingNone}
		cache.Set(key, createSampleOutline())
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := OutlineCacheKey{FontID: 1, GID: GlyphID(i % 256), Size: int16(12 + i%10), Hinting: HintingNone}
			_ = cache.Get(key)
			i++
		}
	})
}

// BenchmarkConcurrentRenderGlyphs benchmarks concurrent rendering.
func BenchmarkConcurrentRenderGlyphs(b *testing.B) {
	renderer := NewGlyphRenderer()
	mockFont := &benchmarkMockFont{name: "BenchmarkFont", unitsPerEm: 2048, numGlyphs: 256}
	fontID := computeFontID(mockFont)

	// Pre-populate cache
	for i := 0; i < 256; i++ {
		key := OutlineCacheKey{FontID: fontID, GID: GlyphID(i), Size: 16, Hinting: HintingNone}
		renderer.cache.Set(key, createSampleOutline())
	}

	// Create test glyphs
	glyphs := make([]ShapedGlyph, 20)
	for i := range glyphs {
		glyphs[i] = ShapedGlyph{
			GID:      GlyphID(65 + i%26),
			X:        float64(i) * 10,
			Y:        0,
			XAdvance: 10,
		}
	}

	params := DefaultRenderParams()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = renderer.RenderGlyphs(glyphs, mockFont, 16, params)
		}
	})
}
