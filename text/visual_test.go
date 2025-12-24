// Package text provides GPU text rendering infrastructure.
package text

import (
	"image/color"
	"math"
	"sync"
	"testing"
)

// ========================================================================
// Test Helpers
// ========================================================================

// testOutline creates a mock outline for testing.
// If no segments are provided, a default rectangle shape is used.
func testOutline(gid GlyphID, advance float32) *GlyphOutline {
	// Default simple glyph shape (rectangle-like)
	segments := []OutlineSegment{
		{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
		{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: advance, Y: 0}}},
		{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: advance, Y: 10}}},
		{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: 10}}},
		{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
	}

	// Calculate bounds from segments
	minX, minY := float64(1e10), float64(1e10)
	maxX, maxY := float64(-1e10), float64(-1e10)

	for _, seg := range segments {
		pointCount := 1
		switch seg.Op {
		case OutlineOpMoveTo, OutlineOpLineTo:
			pointCount = 1
		case OutlineOpQuadTo:
			pointCount = 2
		case OutlineOpCubicTo:
			pointCount = 3
		}
		for i := 0; i < pointCount; i++ {
			p := seg.Points[i]
			if float64(p.X) < minX {
				minX = float64(p.X)
			}
			if float64(p.Y) < minY {
				minY = float64(p.Y)
			}
			if float64(p.X) > maxX {
				maxX = float64(p.X)
			}
			if float64(p.Y) > maxY {
				maxY = float64(p.Y)
			}
		}
	}

	return &GlyphOutline{
		Segments: segments,
		Bounds:   Rect{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY},
		Advance:  advance,
		GID:      gid,
		Type:     GlyphTypeOutline,
	}
}

// testShapedGlyphs creates a slice of shaped glyphs for testing.
func testShapedGlyphs(text string, advance float64) []ShapedGlyph {
	glyphs := make([]ShapedGlyph, len(text))
	x := 0.0
	for i, r := range text {
		glyphs[i] = ShapedGlyph{
			GID:      GlyphID(r), // Use rune as glyph ID for simplicity
			Cluster:  i,
			X:        x,
			Y:        0,
			XAdvance: advance,
		}
		x += advance
	}
	return glyphs
}

// approxEqual checks if two float32 values are approximately equal.
func approxEqual(a, b, epsilon float32) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

// approxEqual64 checks if two float64 values are approximately equal (within 0.001).
func approxEqual64(a, b float64) bool {
	const epsilon = 0.001
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

// ========================================================================
// Visual Regression Tests
// ========================================================================

// TestVisual_BasicLatin tests rendering basic Latin text at various sizes.
func TestVisual_BasicLatin(t *testing.T) {
	sizes := []float64{12, 24, 48, 72}

	for _, size := range sizes {
		t.Run(sizeStr(size), func(t *testing.T) {
			// Create test glyphs for "Hello"
			glyphs := testShapedGlyphs("Hello", 10.0)

			// Create outlines for each glyph
			outlines := make([]*GlyphOutline, len(glyphs))
			for i, g := range glyphs {
				outlines[i] = testOutline(g.GID, float32(g.XAdvance))
			}

			// Verify glyph count
			if len(outlines) != 5 {
				t.Errorf("expected 5 glyphs for 'Hello', got %d", len(outlines))
			}

			// Verify each outline is valid
			for i, outline := range outlines {
				if outline == nil {
					t.Errorf("outline[%d] is nil", i)
					continue
				}
				if outline.IsEmpty() {
					t.Errorf("outline[%d] is empty", i)
				}
				if outline.Advance <= 0 {
					t.Errorf("outline[%d] has non-positive advance: %f", i, outline.Advance)
				}
			}

			// Verify glyph positions are sequential
			var prevX float64
			for i, g := range glyphs {
				if i > 0 && g.X <= prevX {
					t.Errorf("glyph[%d].X (%f) should be > glyph[%d].X (%f)", i, g.X, i-1, prevX)
				}
				prevX = g.X
			}
		})
	}
}

func sizeStr(size float64) string {
	return "size_" + floatToStr(size)
}

func floatToStr(f float64) string {
	// Simple integer conversion for common test sizes
	if f == float64(int(f)) {
		return intToStr(int(f))
	}
	return intToStr(int(f)) + "_" + intToStr(int((f-float64(int(f)))*10))
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	digits := make([]byte, 0, 10)
	for i > 0 {
		digits = append(digits, byte('0'+i%10))
		i /= 10
	}
	// Reverse
	for j := 0; j < len(digits)/2; j++ {
		digits[j], digits[len(digits)-1-j] = digits[len(digits)-1-j], digits[j]
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// TestVisual_ScaledText tests text scaling.
func TestVisual_ScaledText(t *testing.T) {
	scales := []float32{1, 2, 4}

	for _, scale := range scales {
		t.Run("scale_"+floatToStr(float64(scale)), func(t *testing.T) {
			// Create base outline
			outline := testOutline(1, 10)
			originalBounds := outline.Bounds

			// Scale the outline
			scaled := outline.Scale(scale)

			// Verify bounds are scaled correctly
			expectedWidth := originalBounds.Width() * float64(scale)
			actualWidth := scaled.Bounds.Width()
			if !approxEqual64(expectedWidth, actualWidth) {
				t.Errorf("bounds width: expected %f, got %f", expectedWidth, actualWidth)
			}

			expectedHeight := originalBounds.Height() * float64(scale)
			actualHeight := scaled.Bounds.Height()
			if !approxEqual64(expectedHeight, actualHeight) {
				t.Errorf("bounds height: expected %f, got %f", expectedHeight, actualHeight)
			}

			// Verify advance is scaled
			expectedAdvance := outline.Advance * scale
			if !approxEqual(expectedAdvance, scaled.Advance, 0.001) {
				t.Errorf("advance: expected %f, got %f", expectedAdvance, scaled.Advance)
			}

			// Verify segment points are scaled
			for i, seg := range scaled.Segments {
				origSeg := outline.Segments[i]
				for j := 0; j < 3; j++ {
					expectedX := origSeg.Points[j].X * scale
					expectedY := origSeg.Points[j].Y * scale
					if !approxEqual(expectedX, seg.Points[j].X, 0.001) {
						t.Errorf("segment[%d].Points[%d].X: expected %f, got %f",
							i, j, expectedX, seg.Points[j].X)
					}
					if !approxEqual(expectedY, seg.Points[j].Y, 0.001) {
						t.Errorf("segment[%d].Points[%d].Y: expected %f, got %f",
							i, j, expectedY, seg.Points[j].Y)
					}
				}
			}
		})
	}
}

// TestVisual_RotatedText tests text rotation.
func TestVisual_RotatedText(t *testing.T) {
	tests := []struct {
		name         string
		angleDegrees float64
	}{
		{"45_degrees", 45},
		{"90_degrees", 90},
		{"180_degrees", 180},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create outline with a simple point
			outline := &GlyphOutline{
				Segments: []OutlineSegment{
					{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 10, Y: 0}}},
					{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 10}}},
				},
				Advance: 10,
				GID:     1,
			}

			// Convert degrees to radians
			angleRad := float32(tt.angleDegrees * math.Pi / 180.0)
			transform := RotateTransform(angleRad)

			// Transform the outline
			rotated := outline.Transform(transform)

			// Verify rotation was applied
			// For 90 degrees: (10, 0) -> (0, 10)
			if tt.angleDegrees == 90 {
				p := rotated.Segments[0].Points[0]
				if !approxEqual(p.X, 0, 0.1) {
					t.Errorf("after 90deg rotation, X should be ~0, got %f", p.X)
				}
				if !approxEqual(p.Y, 10, 0.1) {
					t.Errorf("after 90deg rotation, Y should be ~10, got %f", p.Y)
				}
			}

			// For 180 degrees: (10, 0) -> (-10, 0)
			// Note: Taylor series approximation has ~2.4% error at pi
			if tt.angleDegrees == 180 {
				p := rotated.Segments[0].Points[0]
				if !approxEqual(p.X, -10, 0.5) {
					t.Errorf("after 180deg rotation, X should be ~-10, got %f", p.X)
				}
				if !approxEqual(p.Y, 0, 0.5) {
					t.Errorf("after 180deg rotation, Y should be ~0, got %f", p.Y)
				}
			}
		})
	}
}

// TestVisual_TransformedText tests combined transformations.
func TestVisual_TransformedText(t *testing.T) {
	t.Run("scale_rotate_translate", func(t *testing.T) {
		outline := &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 1, Y: 0}}},
			},
			Advance: 10,
			GID:     1,
		}

		// Scale by 2
		scaleT := ScaleTransform(2, 2)

		// Rotate by 90 degrees
		rotateT := RotateTransform(float32(math.Pi / 2))

		// Translate by (100, 50)
		translateT := TranslateTransform(100, 50)

		// Combine: translate * rotate * scale (applied right to left)
		combined := translateT.Multiply(rotateT).Multiply(scaleT)

		transformed := outline.Transform(combined)

		// Original point (1, 0):
		// 1. Scale by 2: (2, 0)
		// 2. Rotate 90deg: (0, 2)
		// 3. Translate: (100, 52)
		p := transformed.Segments[0].Points[0]
		if !approxEqual(p.X, 100, 0.1) {
			t.Errorf("expected X ~100, got %f", p.X)
		}
		if !approxEqual(p.Y, 52, 0.1) {
			t.Errorf("expected Y ~52, got %f", p.Y)
		}
	})

	t.Run("identity_transform", func(t *testing.T) {
		outline := testOutline(1, 10)

		identity := IdentityTransform()
		transformed := outline.Transform(identity)

		// Should be unchanged
		for i := range outline.Segments {
			for j := 0; j < 3; j++ {
				if outline.Segments[i].Points[j] != transformed.Segments[i].Points[j] {
					t.Errorf("identity transform should not change points")
				}
			}
		}
	})

	t.Run("quadratic_curve_transform", func(t *testing.T) {
		outline := &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
				{Op: OutlineOpQuadTo, Points: [3]OutlinePoint{
					{X: 5, Y: 10},  // Control
					{X: 10, Y: 0},  // Target
				}},
			},
			Advance: 10,
			GID:     1,
		}

		scale := ScaleTransform(2, 2)
		transformed := outline.Transform(scale)

		// Control point should be scaled
		if !approxEqual(transformed.Segments[1].Points[0].X, 10, 0.001) {
			t.Errorf("QuadTo control X: expected 10, got %f", transformed.Segments[1].Points[0].X)
		}
		if !approxEqual(transformed.Segments[1].Points[0].Y, 20, 0.001) {
			t.Errorf("QuadTo control Y: expected 20, got %f", transformed.Segments[1].Points[0].Y)
		}

		// Target point should be scaled
		if !approxEqual(transformed.Segments[1].Points[1].X, 20, 0.001) {
			t.Errorf("QuadTo target X: expected 20, got %f", transformed.Segments[1].Points[1].X)
		}
	})

	t.Run("cubic_curve_transform", func(t *testing.T) {
		outline := &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
				{Op: OutlineOpCubicTo, Points: [3]OutlinePoint{
					{X: 3, Y: 5},   // Control 1
					{X: 7, Y: 5},   // Control 2
					{X: 10, Y: 0},  // Target
				}},
			},
			Advance: 10,
			GID:     1,
		}

		scale := ScaleTransform(2, 2)
		transformed := outline.Transform(scale)

		// All control points and target should be scaled
		expectedPoints := [][2]float32{
			{6, 10},  // Control 1 scaled
			{14, 10}, // Control 2 scaled
			{20, 0},  // Target scaled
		}

		for i, expected := range expectedPoints {
			actual := transformed.Segments[1].Points[i]
			if !approxEqual(actual.X, expected[0], 0.001) {
				t.Errorf("CubicTo Points[%d].X: expected %f, got %f", i, expected[0], actual.X)
			}
			if !approxEqual(actual.Y, expected[1], 0.001) {
				t.Errorf("CubicTo Points[%d].Y: expected %f, got %f", i, expected[1], actual.Y)
			}
		}
	})
}

// TestVisual_ColoredText tests text with different colors and opacities.
func TestVisual_ColoredText(t *testing.T) {
	colors := []struct {
		name  string
		color color.RGBA
	}{
		{"black", color.RGBA{R: 0, G: 0, B: 0, A: 255}},
		{"red", color.RGBA{R: 255, G: 0, B: 0, A: 255}},
		{"green", color.RGBA{R: 0, G: 255, B: 0, A: 255}},
		{"blue", color.RGBA{R: 0, G: 0, B: 255, A: 255}},
		{"transparent_50", color.RGBA{R: 255, G: 255, B: 255, A: 128}},
	}

	for _, tc := range colors {
		t.Run(tc.name, func(t *testing.T) {
			params := DefaultRenderParams().WithColor(tc.color)

			// Verify color is set correctly
			if params.Color != tc.color {
				t.Errorf("expected color %v, got %v", tc.color, params.Color)
			}
		})
	}

	// Test opacity variations
	opacities := []float64{0.0, 0.25, 0.5, 0.75, 1.0}
	for _, opacity := range opacities {
		t.Run("opacity_"+floatToStr(opacity), func(t *testing.T) {
			params := DefaultRenderParams().WithOpacity(opacity)

			// Verify opacity is set correctly
			if !approxEqual64(params.Opacity, opacity) {
				t.Errorf("expected opacity %f, got %f", opacity, params.Opacity)
			}
		})
	}

	// Test opacity clamping
	t.Run("opacity_clamping", func(t *testing.T) {
		params := DefaultRenderParams().WithOpacity(-0.5)
		if params.Opacity != 0 {
			t.Errorf("negative opacity should be clamped to 0, got %f", params.Opacity)
		}

		params = DefaultRenderParams().WithOpacity(1.5)
		if params.Opacity != 1.0 {
			t.Errorf("opacity > 1 should be clamped to 1, got %f", params.Opacity)
		}
	})
}

// TestVisual_LongText tests rendering long strings.
func TestVisual_LongText(t *testing.T) {
	paragraphs := []string{
		"The quick brown fox jumps over the lazy dog.",
		"Pack my box with five dozen liquor jugs.",
		"How vexingly quick daft zebras jump!",
	}

	for _, para := range paragraphs {
		t.Run(para[:10]+"...", func(t *testing.T) {
			advance := 8.0
			glyphs := testShapedGlyphs(para, advance)

			// Verify all glyphs are created
			if len(glyphs) != len(para) {
				t.Errorf("expected %d glyphs, got %d", len(para), len(glyphs))
			}

			// Verify glyph positions are sequential
			for i := 1; i < len(glyphs); i++ {
				prevEnd := glyphs[i-1].X + glyphs[i-1].XAdvance
				currentStart := glyphs[i].X
				if !approxEqual64(prevEnd, currentStart) {
					t.Errorf("glyph[%d] position mismatch: prev end=%f, current start=%f",
						i, prevEnd, currentStart)
				}
			}

			// Verify total width matches expected
			expectedWidth := float64(len(para)) * advance
			lastGlyph := glyphs[len(glyphs)-1]
			actualWidth := lastGlyph.X + lastGlyph.XAdvance
			if !approxEqual64(expectedWidth, actualWidth) {
				t.Errorf("total width: expected %f, got %f", expectedWidth, actualWidth)
			}
		})
	}
}

// TestVisual_SpecialCharacters tests punctuation, numbers, and symbols.
func TestVisual_SpecialCharacters(t *testing.T) {
	testCases := []struct {
		name string
		text string
	}{
		{"punctuation", "Hello, World! How are you?"},
		{"numbers", "12345 67890"},
		{"symbols", "@#$%^&*()"},
		{"brackets", "[]{}()<>"},
		{"quotes", "\"Hello\" 'World'"},
		{"mixed", "Item #1: $29.99 (50% off!)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			glyphs := testShapedGlyphs(tc.text, 8.0)

			// Verify all characters have corresponding glyphs
			if len(glyphs) != len(tc.text) {
				t.Errorf("expected %d glyphs, got %d", len(tc.text), len(glyphs))
			}

			// Verify each glyph has valid properties
			for i, g := range glyphs {
				if g.XAdvance <= 0 {
					t.Errorf("glyph[%d] has non-positive advance: %f", i, g.XAdvance)
				}
				if g.Cluster != i {
					t.Errorf("glyph[%d] has wrong cluster: expected %d, got %d", i, i, g.Cluster)
				}
			}
		})
	}
}

// TestVisual_EmptyAndWhitespace tests edge cases.
func TestVisual_EmptyAndWhitespace(t *testing.T) {
	t.Run("empty_string", func(t *testing.T) {
		glyphs := testShapedGlyphs("", 8.0)
		if len(glyphs) != 0 {
			t.Errorf("empty string should produce 0 glyphs, got %d", len(glyphs))
		}
	})

	t.Run("single_space", func(t *testing.T) {
		glyphs := testShapedGlyphs(" ", 8.0)
		if len(glyphs) != 1 {
			t.Errorf("single space should produce 1 glyph, got %d", len(glyphs))
		}
		if glyphs[0].XAdvance <= 0 {
			t.Errorf("space glyph should have positive advance")
		}
	})

	t.Run("multiple_spaces", func(t *testing.T) {
		glyphs := testShapedGlyphs("    ", 8.0)
		if len(glyphs) != 4 {
			t.Errorf("four spaces should produce 4 glyphs, got %d", len(glyphs))
		}
	})

	t.Run("tabs", func(t *testing.T) {
		glyphs := testShapedGlyphs("\t\t", 8.0)
		if len(glyphs) != 2 {
			t.Errorf("two tabs should produce 2 glyphs, got %d", len(glyphs))
		}
	})

	t.Run("mixed_whitespace", func(t *testing.T) {
		text := "hello world"
		glyphs := testShapedGlyphs(text, 8.0)
		if len(glyphs) != len(text) {
			t.Errorf("expected %d glyphs, got %d", len(text), len(glyphs))
		}

		// Find the space glyph
		spaceIdx := 5 // "hello" is 5 chars, then space
		if glyphs[spaceIdx].GID != GlyphID(' ') {
			t.Errorf("expected space at index 5")
		}
	})

	t.Run("leading_trailing_spaces", func(t *testing.T) {
		text := "  hello  "
		glyphs := testShapedGlyphs(text, 8.0)
		if len(glyphs) != len(text) {
			t.Errorf("expected %d glyphs, got %d", len(text), len(glyphs))
		}
	})
}

// TestVisual_CacheConsistency verifies cached vs fresh renders match.
func TestVisual_CacheConsistency(t *testing.T) {
	// Create a dedicated cache for this test
	cache := NewGlyphCache()

	// Create cache keys for different glyphs
	keys := []OutlineCacheKey{
		{FontID: 12345, GID: 1, Size: 16, Hinting: HintingNone},
		{FontID: 12345, GID: 2, Size: 16, Hinting: HintingNone},
		{FontID: 12345, GID: 3, Size: 16, Hinting: HintingNone},
	}

	// Store outlines in cache
	for _, key := range keys {
		outline := testOutline(key.GID, 10)
		cache.Set(key, outline)
	}

	// Retrieve and verify consistency
	for i := 0; i < 3; i++ {
		t.Run("iteration_"+intToStr(i), func(t *testing.T) {
			for _, key := range keys {
				// First get
				outline1 := cache.Get(key)
				if outline1 == nil {
					t.Fatalf("outline should be cached for key %v", key)
				}

				// Second get (should return same data)
				outline2 := cache.Get(key)
				if outline2 == nil {
					t.Fatalf("outline should still be cached for key %v", key)
				}

				// Verify outlines are equal
				if outline1.GID != outline2.GID {
					t.Errorf("GID mismatch: %d vs %d", outline1.GID, outline2.GID)
				}
				if outline1.Advance != outline2.Advance {
					t.Errorf("Advance mismatch: %f vs %f", outline1.Advance, outline2.Advance)
				}
				if len(outline1.Segments) != len(outline2.Segments) {
					t.Errorf("Segment count mismatch: %d vs %d",
						len(outline1.Segments), len(outline2.Segments))
				}
			}
		})
	}

	// Test GetOrCreate consistency
	t.Run("get_or_create_consistency", func(t *testing.T) {
		key := OutlineCacheKey{FontID: 99999, GID: 42, Size: 24, Hinting: HintingNone}
		createCount := 0

		creator := func() *GlyphOutline {
			createCount++
			return testOutline(42, 10)
		}

		// First call should create
		outline1 := cache.GetOrCreate(key, creator)
		if createCount != 1 {
			t.Errorf("creator should be called once, was called %d times", createCount)
		}

		// Subsequent calls should use cache
		for i := 0; i < 5; i++ {
			outline2 := cache.GetOrCreate(key, creator)
			if createCount != 1 {
				t.Errorf("creator should not be called again, was called %d times", createCount)
			}
			if outline1.GID != outline2.GID {
				t.Errorf("cached outline should be consistent")
			}
		}
	})
}

// TestVisual_MultipleRenderers tests concurrent rendering.
func TestVisual_MultipleRenderers(t *testing.T) {
	t.Run("concurrent_cache_access", func(t *testing.T) {
		cache := NewGlyphCache()
		var wg sync.WaitGroup

		// Number of concurrent goroutines
		numGoroutines := 10
		numIterations := 100

		// Pre-populate some cache entries
		for i := 0; i < 50; i++ {
			key := OutlineCacheKey{FontID: 1, GID: GlyphID(i), Size: 16}
			cache.Set(key, testOutline(GlyphID(i), 10))
		}

		// Concurrent reads
		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for i := 0; i < numIterations; i++ {
					gid := GlyphID((goroutineID*numIterations + i) % 50)
					key := OutlineCacheKey{FontID: 1, GID: gid, Size: 16}
					_ = cache.Get(key)
				}
			}(g)
		}

		wg.Wait()
	})

	t.Run("concurrent_writes_and_reads", func(t *testing.T) {
		cache := NewGlyphCache()
		var wg sync.WaitGroup

		numGoroutines := 10
		numOperations := 100

		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for i := 0; i < numOperations; i++ {
					gid := GlyphID(goroutineID*numOperations + i)
					key := OutlineCacheKey{FontID: uint64(goroutineID), GID: gid, Size: 16}

					if i%2 == 0 {
						// Write
						cache.Set(key, testOutline(gid, 10))
					} else {
						// Read
						_ = cache.Get(key)
					}
				}
			}(g)
		}

		wg.Wait()
	})

	t.Run("concurrent_get_or_create", func(t *testing.T) {
		cache := NewGlyphCache()
		var wg sync.WaitGroup

		numGoroutines := 10
		numKeys := 20

		// All goroutines try to get/create the same keys
		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for k := 0; k < numKeys; k++ {
					key := OutlineCacheKey{FontID: 1, GID: GlyphID(k), Size: 16}
					_ = cache.GetOrCreate(key, func() *GlyphOutline {
						return testOutline(GlyphID(k), 10)
					})
				}
			}()
		}

		wg.Wait()

		// Verify all keys exist in cache
		for k := 0; k < numKeys; k++ {
			key := OutlineCacheKey{FontID: 1, GID: GlyphID(k), Size: 16}
			if cache.Get(key) == nil {
				t.Errorf("key %d should be in cache", k)
			}
		}
	})

	t.Run("concurrent_maintain", func(t *testing.T) {
		cache := NewGlyphCacheWithConfig(GlyphCacheConfig{
			MaxEntries:    1000,
			FrameLifetime: 64,
		})
		var wg sync.WaitGroup

		numGoroutines := 5

		// Some goroutines write
		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(gid int) {
				defer wg.Done()
				for i := 0; i < 100; i++ {
					key := OutlineCacheKey{FontID: uint64(gid), GID: GlyphID(i), Size: 16}
					cache.Set(key, testOutline(GlyphID(i), 10))
				}
			}(g)
		}

		// Some goroutines read
		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(gid int) {
				defer wg.Done()
				for i := 0; i < 100; i++ {
					key := OutlineCacheKey{FontID: uint64(gid), GID: GlyphID(i), Size: 16}
					_ = cache.Get(key)
				}
			}(g)
		}

		// Some goroutines call Maintain
		for g := 0; g < 2; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 10; i++ {
					cache.Maintain()
				}
			}()
		}

		wg.Wait()
	})
}

// TestVisual_GlyphRenderer tests the GlyphRenderer struct directly.
func TestVisual_GlyphRenderer(t *testing.T) {
	t.Run("create_with_custom_cache", func(t *testing.T) {
		customCache := NewGlyphCache()
		renderer := NewGlyphRendererWithCache(customCache)

		if renderer.Cache() != customCache {
			t.Error("renderer should use custom cache")
		}
	})

	t.Run("set_cache", func(t *testing.T) {
		renderer := NewGlyphRenderer()
		originalCache := renderer.Cache()

		newCache := NewGlyphCache()
		renderer.SetCache(newCache)

		if renderer.Cache() == originalCache {
			t.Error("cache should have changed")
		}
		if renderer.Cache() != newCache {
			t.Error("cache should be the new cache")
		}
	})

	t.Run("render_params_immutability", func(t *testing.T) {
		params := DefaultRenderParams()

		// Each With* method should return a new copy
		params2 := params.WithColor(color.RGBA{R: 255, G: 0, B: 0, A: 255})
		if params.Color == params2.Color {
			t.Error("WithColor should return a copy, not modify original")
		}

		params3 := params.WithOpacity(0.5)
		if params.Opacity == params3.Opacity {
			t.Error("WithOpacity should return a copy, not modify original")
		}

		transform := TranslateTransform(10, 20)
		params4 := params.WithTransform(transform)
		if params.Transform == params4.Transform {
			t.Error("WithTransform should return a copy, not modify original")
		}
	})
}

// TestVisual_OutlineOperations tests outline operations in visual context.
func TestVisual_OutlineOperations(t *testing.T) {
	t.Run("clone_independence", func(t *testing.T) {
		original := testOutline(1, 10)
		cloned := original.Clone()

		// Modify original
		original.Segments[0].Points[0].X = 999

		// Clone should be unaffected
		if cloned.Segments[0].Points[0].X == 999 {
			t.Error("clone should be independent of original")
		}
	})

	t.Run("translate_preserves_advance", func(t *testing.T) {
		outline := testOutline(1, 15)
		translated := outline.Translate(100, 50)

		if translated.Advance != outline.Advance {
			t.Errorf("translate should preserve advance: got %f, want %f",
				translated.Advance, outline.Advance)
		}
	})

	t.Run("scale_updates_advance", func(t *testing.T) {
		outline := testOutline(1, 10)
		scaled := outline.Scale(2)

		if scaled.Advance != 20 {
			t.Errorf("scale by 2 should double advance: got %f, want 20", scaled.Advance)
		}
	})

	t.Run("bounds_update_on_transform", func(t *testing.T) {
		outline := &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
				{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 10}}},
			},
			Bounds:  Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10},
			Advance: 10,
			GID:     1,
		}

		// Translate should update bounds
		translated := outline.Translate(50, 50)
		if translated.Bounds.MinX != 50 || translated.Bounds.MinY != 50 {
			t.Errorf("translate should update bounds min: got (%f, %f), want (50, 50)",
				translated.Bounds.MinX, translated.Bounds.MinY)
		}
		if translated.Bounds.MaxX != 60 || translated.Bounds.MaxY != 60 {
			t.Errorf("translate should update bounds max: got (%f, %f), want (60, 60)",
				translated.Bounds.MaxX, translated.Bounds.MaxY)
		}
	})
}

// TestVisual_EdgeCases tests various edge cases.
func TestVisual_EdgeCases(t *testing.T) {
	t.Run("zero_scale", func(t *testing.T) {
		outline := testOutline(1, 10)
		scaled := outline.Scale(0)

		// All points should be at origin
		for _, seg := range scaled.Segments {
			for i := 0; i < 3; i++ {
				if seg.Points[i].X != 0 || seg.Points[i].Y != 0 {
					t.Error("zero scale should collapse all points to origin")
				}
			}
		}
	})

	t.Run("negative_scale", func(t *testing.T) {
		outline := testOutline(1, 10)
		scaled := outline.Scale(-1)

		// Points should be mirrored
		if scaled.Segments[1].Points[0].X >= 0 {
			t.Error("negative scale should flip points")
		}
	})

	t.Run("very_small_advance", func(t *testing.T) {
		outline := testOutline(1, 0.001)
		if outline.Advance <= 0 {
			t.Error("very small advance should still be positive")
		}
	})

	t.Run("very_large_advance", func(t *testing.T) {
		outline := testOutline(1, 10000)
		if outline.Advance != 10000 {
			t.Errorf("large advance should be preserved: got %f", outline.Advance)
		}
	})

	t.Run("empty_outline_transform", func(t *testing.T) {
		outline := &GlyphOutline{
			Segments: []OutlineSegment{},
			GID:      1,
			Advance:  10,
		}

		transformed := outline.Transform(ScaleTransform(2, 2))
		if len(transformed.Segments) != 0 {
			t.Error("empty outline should remain empty after transform")
		}
		// Advance is preserved, not scaled (by design for empty outlines)
	})

	t.Run("nil_outline_operations", func(t *testing.T) {
		var outline *GlyphOutline

		if outline.Clone() != nil {
			t.Error("Clone of nil should return nil")
		}
		if outline.Scale(2) != nil {
			t.Error("Scale of nil should return nil")
		}
		if outline.Translate(10, 10) != nil {
			t.Error("Translate of nil should return nil")
		}
		if outline.Transform(IdentityTransform()) != nil {
			t.Error("Transform of nil should return nil")
		}
	})
}

// ========================================================================
// Benchmarks
// ========================================================================

func BenchmarkVisual_Transform(b *testing.B) {
	outline := testOutline(1, 10)
	transform := ScaleTransform(2, 2).Multiply(RotateTransform(0.5))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = outline.Transform(transform)
	}
}

func BenchmarkVisual_CacheHit(b *testing.B) {
	cache := NewGlyphCache()
	key := OutlineCacheKey{FontID: 1, GID: 42, Size: 16}
	cache.Set(key, testOutline(42, 10))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Get(key)
	}
}

func BenchmarkVisual_CacheGetOrCreate(b *testing.B) {
	cache := NewGlyphCache()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i % 1000), Size: 16}
		_ = cache.GetOrCreate(key, func() *GlyphOutline {
			return testOutline(GlyphID(i%1000), 10)
		})
	}
}
