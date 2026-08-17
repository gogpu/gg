package text

import "testing"

func TestCPUGlyphMaskCacheKeyIncludesRenderingParameters(t *testing.T) {
	base := makeCPUGlyphMaskCacheKey(1, 16, 0.25, 0.5, HintingFull, rasterModeAA)
	tests := []struct {
		name string
		key  cpuGlyphMaskCacheKey
	}{
		{"glyph", makeCPUGlyphMaskCacheKey(2, 16, 0.25, 0.5, HintingFull, rasterModeAA)},
		{"size", makeCPUGlyphMaskCacheKey(1, 17, 0.25, 0.5, HintingFull, rasterModeAA)},
		{"subpixel X", makeCPUGlyphMaskCacheKey(1, 16, 0.5, 0.5, HintingFull, rasterModeAA)},
		{"subpixel Y", makeCPUGlyphMaskCacheKey(1, 16, 0.25, 0.75, HintingFull, rasterModeAA)},
		{"hinting", makeCPUGlyphMaskCacheKey(1, 16, 0.25, 0.5, HintingNone, rasterModeAA)},
		{"raster mode", makeCPUGlyphMaskCacheKey(1, 16, 0.25, 0.5, HintingFull, rasterModeAliased)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.key == base {
				t.Fatal("changing rendering parameter did not change cache key")
			}
		})
	}
}

func TestCPUGlyphMaskCacheIsBounded(t *testing.T) {
	source := &FontSource{}
	cache := source.cpuGlyphMaskCache()
	for glyphID := range cache.TotalCapacity() * 4 {
		key := makeCPUGlyphMaskCacheKey(GlyphID(glyphID), 16, 0, 0, HintingFull, rasterModeAA)
		cache.Set(key, cpuGlyphMaskCacheValue{})
	}

	if got, limit := cache.Len(), cache.TotalCapacity(); got > limit {
		t.Fatalf("cache length = %d, want at most %d", got, limit)
	}
}
