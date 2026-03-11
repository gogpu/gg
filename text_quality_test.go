package gg

import (
	"testing"

	"github.com/gogpu/gg/text"
)

// --------------------------------------------------------------------------
// Test 1: TextMode Strategy Coverage
// --------------------------------------------------------------------------

// TestTextQuality_AllStrategies verifies that each CPU text mode (Bitmap, Vector)
// produces visible pixels and that their ink counts are comparable.
func TestTextQuality_AllStrategies(t *testing.T) {
	dc, source := setupTextContext(t, 400, 200, 16)
	defer func() { _ = source.Close() }()

	type strategyResult struct {
		name   string
		mode   TextMode
		pixels int
	}

	strategies := []struct {
		name string
		mode TextMode
		// applyTransform forces the vector path (outline rendering).
		// Bitmap uses identity; Vector uses a tiny scale to trigger outlines.
		applyTransform func(dc *Context)
		restore        func(dc *Context)
	}{
		{
			name:           "Bitmap",
			mode:           TextModeBitmap,
			applyTransform: func(_ *Context) {},
			restore:        func(_ *Context) {},
		},
		{
			name: "Vector",
			mode: TextModeVector,
			applyTransform: func(dc *Context) {
				dc.Push()
				dc.Scale(1.001, 1.001) // tiny non-identity to trigger outline path
			},
			restore: func(dc *Context) {
				dc.Pop()
			},
		},
	}

	results := make([]strategyResult, 0, len(strategies))

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			dc.ClearWithColor(White)
			dc.SetTextMode(s.mode)
			s.applyTransform(dc)
			dc.DrawString("Hello World", 20, 100)
			s.restore(dc)

			pixels := countNonWhitePixels(dc, 0, 0, 400, 200)
			if pixels == 0 {
				t.Errorf("%s strategy produced no visible pixels", s.name)
			}
			results = append(results, strategyResult{
				name:   s.name,
				mode:   s.mode,
				pixels: pixels,
			})
			t.Logf("%s: %d non-white pixels", s.name, pixels)
		})
	}

	// Cross-strategy comparison: ink counts should be within 3x of each other.
	t.Run("CrossComparison", func(t *testing.T) {
		if len(results) < 2 {
			t.Skip("Not enough strategy results to compare")
		}
		for i := 0; i < len(results); i++ {
			for j := i + 1; j < len(results); j++ {
				a, b := results[i], results[j]
				if a.pixels == 0 || b.pixels == 0 {
					continue // already reported above
				}
				ratio := float64(a.pixels) / float64(b.pixels)
				if ratio < 0.33 || ratio > 3.0 {
					t.Errorf("%s (%d px) vs %s (%d px): ratio %.2f exceeds 3x tolerance",
						a.name, a.pixels, b.name, b.pixels, ratio)
				}
			}
		}
	})
}

// --------------------------------------------------------------------------
// Test 2: Size Proportionality
// --------------------------------------------------------------------------

// TestTextQuality_SizeProportionality verifies that ink count increases
// monotonically with font size using TextModeBitmap.
func TestTextQuality_SizeProportionality(t *testing.T) {
	fontPath := findSystemFont(t)
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	sizes := []float64{12, 16, 24, 32, 48}
	pixelCounts := make([]int, len(sizes))

	for i, sz := range sizes {
		// Canvas large enough for the biggest size.
		dc := NewContext(600, 200)
		dc.ClearWithColor(White)
		dc.SetFont(source.Face(sz))
		dc.SetRGB(0, 0, 0)
		dc.SetTextMode(TextModeBitmap)
		dc.DrawString("ABC", 20, 100)

		pixels := countNonWhitePixels(dc, 0, 0, 600, 200)
		pixelCounts[i] = pixels
		t.Logf("Size %.0f: %d pixels", sz, pixels)
		dc.Close()
	}

	// Verify monotonic increase.
	for i := 1; i < len(sizes); i++ {
		if pixelCounts[i] <= pixelCounts[i-1] {
			t.Errorf("Size %.0f (%d px) should produce more ink than size %.0f (%d px)",
				sizes[i], pixelCounts[i], sizes[i-1], pixelCounts[i-1])
		}
	}

	// Verify rough quadratic scaling: doubling size should produce ~4x ink (within 50%).
	// Compare size 12 vs 24 (2x) and 24 vs 48 (2x).
	doublings := [][2]int{{0, 2}, {2, 4}} // indices: 12→24, 24→48
	for _, pair := range doublings {
		small := pixelCounts[pair[0]]
		large := pixelCounts[pair[1]]
		if small == 0 {
			continue
		}
		ratio := float64(large) / float64(small)
		// Expect ~4x, allow 2x to 8x range.
		if ratio < 2.0 || ratio > 8.0 {
			t.Errorf("Size %.0f→%.0f (2x): pixel ratio %.2f outside [2.0, 8.0] range",
				sizes[pair[0]], sizes[pair[1]], ratio)
		}
	}
}

// --------------------------------------------------------------------------
// Test 3: Vector Text at Multiple Sizes
// --------------------------------------------------------------------------

// TestTextQuality_VectorSizes verifies that TextModeVector produces visible ink
// at multiple font sizes and that ink increases with size.
func TestTextQuality_VectorSizes(t *testing.T) {
	fontPath := findSystemFont(t)
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	sizes := []float64{12, 16, 24, 48}
	pixelCounts := make([]int, len(sizes))

	for i, sz := range sizes {
		dc := NewContext(800, 300)
		dc.ClearWithColor(White)
		dc.SetFont(source.Face(sz))
		dc.SetRGB(0, 0, 0)
		dc.SetTextMode(TextModeVector)

		// Apply Scale(2,1) to force vector outline path.
		dc.Push()
		dc.Scale(2, 1)
		dc.DrawString("ABC", 20, 150)
		dc.Pop()

		pixels := countNonWhitePixels(dc, 0, 0, 800, 300)
		pixelCounts[i] = pixels
		t.Logf("Vector size %.0f: %d pixels", sz, pixels)

		if pixels == 0 {
			t.Errorf("Vector mode at size %.0f produced no visible pixels", sz)
		}
		dc.Close()
	}

	// Verify ink increases with size (monotonic).
	for i := 1; i < len(sizes); i++ {
		if pixelCounts[i] <= pixelCounts[i-1] {
			t.Errorf("Vector size %.0f (%d px) should produce more ink than %.0f (%d px)",
				sizes[i], pixelCounts[i], sizes[i-1], pixelCounts[i-1])
		}
	}
}

// --------------------------------------------------------------------------
// Test 4: Strategy Consistency
// --------------------------------------------------------------------------

// TestTextQuality_StrategyConsistency compares Bitmap and Vector strategies
// rendering the same text, verifying both produce ink in the same order of magnitude.
func TestTextQuality_StrategyConsistency(t *testing.T) {
	fontPath := findSystemFont(t)
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	const fontSize = 24.0
	const drawText = "ABCD"

	// Bitmap rendering (identity matrix).
	dcBitmap := NewContext(400, 200)
	dcBitmap.ClearWithColor(White)
	dcBitmap.SetFont(source.Face(fontSize))
	dcBitmap.SetRGB(0, 0, 0)
	dcBitmap.SetTextMode(TextModeBitmap)
	dcBitmap.DrawString(drawText, 20, 100)
	bitmapPixels := countNonWhitePixels(dcBitmap, 0, 0, 400, 200)
	dcBitmap.Close()

	// Vector rendering (tiny scale to trigger outline path).
	dcVector := NewContext(400, 200)
	dcVector.ClearWithColor(White)
	dcVector.SetFont(source.Face(fontSize))
	dcVector.SetRGB(0, 0, 0)
	dcVector.SetTextMode(TextModeVector)
	dcVector.Push()
	dcVector.Scale(1.001, 1.001) // near-identity, forces vector outlines
	dcVector.DrawString(drawText, 20, 100)
	dcVector.Pop()
	vectorPixels := countNonWhitePixels(dcVector, 0, 0, 400, 200)
	dcVector.Close()

	t.Logf("Bitmap: %d pixels, Vector: %d pixels", bitmapPixels, vectorPixels)

	if bitmapPixels == 0 {
		t.Error("Bitmap strategy produced no visible pixels")
	}
	if vectorPixels == 0 {
		t.Error("Vector strategy produced no visible pixels")
	}

	// Both should be in the same order of magnitude.
	if bitmapPixels > 0 && vectorPixels > 0 {
		ratio := float64(bitmapPixels) / float64(vectorPixels)
		if ratio < 0.2 || ratio > 5.0 {
			t.Errorf("Bitmap/Vector pixel ratio %.2f outside [0.2, 5.0] tolerance "+
				"(bitmap=%d, vector=%d)", ratio, bitmapPixels, vectorPixels)
		}
	}
}

// --------------------------------------------------------------------------
// Test 5: Thin Stroke Characters
// --------------------------------------------------------------------------

// TestTextQuality_ThinStrokes verifies that thin-stroke characters like "illIl1|"
// produce visible ink with TextModeVector at multiple sizes.
func TestTextQuality_ThinStrokes(t *testing.T) {
	fontPath := findSystemFont(t)
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	const thinText = "illIl1|"

	tests := []struct {
		name     string
		fontSize float64
	}{
		{"14px", 14},
		{"20px", 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := NewContext(400, 200)
			dc.ClearWithColor(White)
			dc.SetFont(source.Face(tt.fontSize))
			dc.SetRGB(0, 0, 0)
			dc.SetTextMode(TextModeVector)

			// Apply slight rotation to force outline path.
			dc.Push()
			dc.Translate(200, 100)
			dc.Rotate(0.001) // barely perceptible rotation
			dc.DrawString(thinText, -80, 0)
			dc.Pop()

			pixels := countNonWhitePixels(dc, 0, 0, 400, 200)
			t.Logf("Thin strokes at %s: %d pixels", tt.name, pixels)

			if pixels == 0 {
				t.Errorf("Thin-stroke characters at %s produced no visible pixels", tt.name)
			}
			dc.Close()
		})
	}
}

// --------------------------------------------------------------------------
// Test 6: GlyphCache Integration
// --------------------------------------------------------------------------

// TestTextQuality_GlyphCacheHits verifies that rendering the same text twice
// with TextModeVector results in glyph cache hits on the second pass.
func TestTextQuality_GlyphCacheHits(t *testing.T) {
	fontPath := findSystemFont(t)
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	cache := text.GetGlobalGlyphCache()
	if cache == nil {
		t.Fatal("GetGlobalGlyphCache returned nil")
	}
	cache.ResetStats()

	dc := NewContext(400, 200)
	dc.SetFont(source.Face(24))
	dc.SetRGB(0, 0, 0)
	dc.SetTextMode(TextModeVector)

	const drawText = "CacheTest"

	// First draw — populates cache (all misses, no hits).
	dc.ClearWithColor(White)
	dc.Push()
	dc.Scale(2, 1) // non-identity to force outline path
	dc.DrawString(drawText, 10, 100)
	dc.Pop()

	hits1, misses1, _, insertions1 := cache.Stats()
	t.Logf("After first draw: hits=%d, misses=%d, insertions=%d", hits1, misses1, insertions1)

	// Second draw — same text, same size: should hit cache.
	dc.ClearWithColor(White)
	dc.Push()
	dc.Scale(2, 1)
	dc.DrawString(drawText, 10, 100)
	dc.Pop()

	hits2, misses2, _, insertions2 := cache.Stats()
	t.Logf("After second draw: hits=%d, misses=%d, insertions=%d", hits2, misses2, insertions2)

	dc.Close()

	// After the second draw, we should have more hits than after the first.
	if hits2 <= hits1 {
		t.Errorf("Expected cache hits to increase after second draw (before=%d, after=%d)",
			hits1, hits2)
	}

	// Insertions should have occurred on first draw.
	if insertions1 == 0 && insertions2 == 0 {
		t.Error("Expected at least one glyph cache insertion")
	}
}
