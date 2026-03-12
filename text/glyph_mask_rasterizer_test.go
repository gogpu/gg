package text

import (
	"testing"
)

func TestGlyphMaskRasterizer_New(t *testing.T) {
	r := NewGlyphMaskRasterizer()
	if r == nil {
		t.Fatal("NewGlyphMaskRasterizer() returned nil")
	}
	if r.extractor == nil {
		t.Error("extractor is nil")
	}
}

func TestGlyphMaskRasterizer_RasterizeNilOutline(t *testing.T) {
	r := NewGlyphMaskRasterizer()

	result, err := r.RasterizeOutline(nil, 0, 0)
	if err != nil {
		t.Fatalf("RasterizeOutline(nil) error = %v", err)
	}
	if result != nil {
		t.Error("RasterizeOutline(nil) should return nil result")
	}
}

func TestGlyphMaskRasterizer_RasterizeEmptyOutline(t *testing.T) {
	r := NewGlyphMaskRasterizer()

	outline := &GlyphOutline{
		Segments: nil,
		GID:      0,
	}

	result, err := r.RasterizeOutline(outline, 0, 0)
	if err != nil {
		t.Fatalf("RasterizeOutline(empty) error = %v", err)
	}
	if result != nil {
		t.Error("RasterizeOutline(empty) should return nil result")
	}
}

func TestGlyphMaskRasterizer_RasterizeTriangle(t *testing.T) {
	r := NewGlyphMaskRasterizer()

	// Create a simple triangle outline (10x10 pixels)
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 5, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 10}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: 10}}},
		},
		Bounds: Rect{
			MinX: 0, MinY: 0,
			MaxX: 10, MaxY: 10,
		},
		GID: 1,
	}

	result, err := r.RasterizeOutline(outline, 0, 0)
	if err != nil {
		t.Fatalf("RasterizeOutline() error = %v", err)
	}
	if result == nil {
		t.Fatal("RasterizeOutline() returned nil for valid outline")
	}

	// Check dimensions are reasonable (should be ~12x12 with AA margin)
	if result.Width <= 0 || result.Height <= 0 {
		t.Errorf("invalid dimensions: %dx%d", result.Width, result.Height)
	}

	// Check mask has some non-zero coverage
	hasNonZero := false
	for _, b := range result.Mask {
		if b > 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("mask has no coverage — triangle not rasterized")
	}

	// Check mask is correct size
	if len(result.Mask) != result.Width*result.Height {
		t.Errorf("mask length = %d, want %d", len(result.Mask), result.Width*result.Height)
	}
}

func TestGlyphMaskRasterizer_RasterizeSquare(t *testing.T) {
	r := NewGlyphMaskRasterizer()

	// Create a 10x10 square outline
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 10}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: 10}}},
		},
		Bounds: Rect{
			MinX: 0, MinY: 0,
			MaxX: 10, MaxY: 10,
		},
		GID: 2,
	}

	result, err := r.RasterizeOutline(outline, 0, 0)
	if err != nil {
		t.Fatalf("RasterizeOutline() error = %v", err)
	}
	if result == nil {
		t.Fatal("RasterizeOutline() returned nil for valid square")
	}

	// A 10x10 square should have many fully-covered interior pixels
	fullCoverage := 0
	for _, b := range result.Mask {
		if b == 255 {
			fullCoverage++
		}
	}

	// Interior of a 10x10 square should have at least 64 fully covered pixels
	// (8x8 interior minus AA edges)
	if fullCoverage < 40 {
		t.Errorf("only %d fully covered pixels for 10x10 square, expected at least 40", fullCoverage)
	}
}

func TestGlyphMaskRasterizer_SubpixelOffset(t *testing.T) {
	r := NewGlyphMaskRasterizer()

	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 8, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 8, Y: 8}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: 8}}},
		},
		Bounds: Rect{
			MinX: 0, MinY: 0,
			MaxX: 8, MaxY: 8,
		},
		GID: 3,
	}

	// Rasterize at 0 subpixel and 0.5 subpixel — masks should differ
	result0, err := r.RasterizeOutline(outline, 0, 0)
	if err != nil {
		t.Fatalf("RasterizeOutline(0) error = %v", err)
	}

	result05, err := r.RasterizeOutline(outline, 0.5, 0)
	if err != nil {
		t.Fatalf("RasterizeOutline(0.5) error = %v", err)
	}

	if result0 == nil || result05 == nil {
		t.Fatal("one of the results is nil")
	}

	// The two masks should differ — either in bearings, dimensions, or pixel content.
	differs := result0.BearingX != result05.BearingX ||
		result0.BearingY != result05.BearingY ||
		result0.Width != result05.Width ||
		result0.Height != result05.Height ||
		!masksEqual(result0.Mask, result05.Mask)

	if !differs {
		t.Error("subpixel=0 and subpixel=0.5 produced identical masks — subpixel positioning not working")
	}
}

// masksEqual reports whether two alpha masks have identical content.
func masksEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGlyphPath_Interface(t *testing.T) {
	// Verify glyphPath implements raster.PathLike
	p := &glyphPath{}
	if !p.IsEmpty() {
		t.Error("empty glyphPath should report IsEmpty() = true")
	}
	if p.Verbs() != nil {
		t.Error("empty glyphPath Verbs() should be nil")
	}
	if p.Points() != nil {
		t.Error("empty glyphPath Points() should be nil")
	}
}
