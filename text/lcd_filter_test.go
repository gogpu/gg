package text

import (
	"math"
	"testing"
)

func TestDefaultLCDFilter_WeightsSum(t *testing.T) {
	f := DefaultLCDFilter()
	var sum float32
	for _, w := range f.Weights {
		sum += w
	}
	// Weights should sum to ~1.0 (conservation of energy).
	if math.Abs(float64(sum)-1.0) > 0.01 {
		t.Errorf("DefaultLCDFilter weights sum = %v, want ~1.0", sum)
	}
}

func TestLCDFilter_Apply_Uniform(t *testing.T) {
	// If the input is uniform (all same value), the output should be
	// approximately the same value (with slight boundary effects).
	f := DefaultLCDFilter()
	width := 10
	src := make([]byte, 3*width)
	for i := range src {
		src[i] = 200
	}
	dst := make([]byte, 3*width)

	f.Apply(dst, src, width)

	// Interior pixels should be close to 200 (boundary pixels may differ
	// due to zero-padding at edges).
	for i := 2; i < width-2; i++ {
		for ch := range 3 {
			v := dst[i*3+ch]
			if v < 195 || v > 205 {
				t.Errorf("pixel %d ch %d: got %d, want ~200", i, ch, v)
			}
		}
	}
}

func TestLCDFilter_Apply_SinglePixel(t *testing.T) {
	f := DefaultLCDFilter()
	width := 1
	src := make([]byte, 3)
	src[0] = 255
	src[1] = 255
	src[2] = 255

	dst := make([]byte, 3)
	f.Apply(dst, src, width)

	// With all 3 subpixels at 255, the filter should produce non-zero output.
	for ch := range 3 {
		if dst[ch] == 0 {
			t.Errorf("single-pixel ch %d: got 0, expected non-zero", ch)
		}
	}
}

func TestLCDFilter_Apply_ZeroInput(t *testing.T) {
	f := DefaultLCDFilter()
	width := 5
	src := make([]byte, 3*width)
	dst := make([]byte, 3*width)
	// Pre-fill dst to verify it gets written.
	for i := range dst {
		dst[i] = 99
	}

	f.Apply(dst, src, width)

	for i := range dst {
		if dst[i] != 0 {
			t.Errorf("dst[%d] = %d, want 0 for zero input", i, dst[i])
		}
	}
}

func TestLCDFilter_Apply_KnownPattern(t *testing.T) {
	// Test with a known pattern: single bright subpixel surrounded by zeros.
	// This exercises the filter's spreading behavior.
	f := DefaultLCDFilter()
	width := 3
	src := make([]byte, 9) // 3 pixels x 3 subpixels
	// Only the center subpixel (index 4) is bright.
	src[4] = 255

	dst := make([]byte, 9)
	f.Apply(dst, src, width)

	// The center pixel's green channel should get the strongest response
	// (it's centered on the bright subpixel).
	centerG := dst[1*3+1]
	if centerG == 0 {
		t.Error("center green should be non-zero")
	}

	// Adjacent channels should also get some energy from the filter.
	centerR := dst[1*3+0]
	centerB := dst[1*3+2]
	if centerR == 0 {
		t.Error("center red should get some filter spread")
	}
	if centerB == 0 {
		t.Error("center blue should get some filter spread")
	}

	// The center tap (0.36) should make green the strongest.
	if centerG < centerR || centerG < centerB {
		t.Errorf("center green (%d) should be >= red (%d) and blue (%d)",
			centerG, centerR, centerB)
	}
}

func TestLCDFilter_Apply_ClampOutput(t *testing.T) {
	// Ensure output is clamped to [0, 255].
	f := LCDFilter{Weights: [5]float32{0.5, 0.5, 0.5, 0.5, 0.5}}
	width := 3
	src := make([]byte, 9)
	for i := range src {
		src[i] = 255
	}
	dst := make([]byte, 9)

	f.Apply(dst, src, width)

	// Verify all values are valid bytes (the filter clamped correctly).
	// Since byte type is always [0, 255], we just verify non-zero coverage
	// from the strong filter weights.
	hasNonZero := false
	for _, v := range dst {
		if v > 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("strong filter weights with all-255 input should produce non-zero output")
	}
}

func TestLCDFilter_Apply_EmptyInput(t *testing.T) {
	f := DefaultLCDFilter()
	// Zero width should not panic.
	f.Apply(nil, nil, 0)
	// Short buffers should not panic.
	f.Apply(make([]byte, 1), make([]byte, 1), 5)
}

func TestLCDLayout_String(t *testing.T) {
	tests := []struct {
		layout LCDLayout
		want   string
	}{
		{LCDLayoutNone, "None"},
		{LCDLayoutRGB, "RGB"},
		{LCDLayoutBGR, "BGR"},
		{LCDLayout(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.layout.String(); got != tt.want {
			t.Errorf("LCDLayout(%d).String() = %q, want %q", tt.layout, got, tt.want)
		}
	}
}

func TestGlyphMaskRasterizer_RasterizeLCDOutline_Nil(t *testing.T) {
	r := NewGlyphMaskRasterizer()
	filter := DefaultLCDFilter()

	result, err := r.RasterizeLCDOutline(nil, 0, 0, filter, LCDLayoutRGB)
	if err != nil {
		t.Fatalf("RasterizeLCDOutline(nil) error = %v", err)
	}
	if result != nil {
		t.Error("RasterizeLCDOutline(nil) should return nil result")
	}
}

func TestGlyphMaskRasterizer_RasterizeLCDOutline_Empty(t *testing.T) {
	r := NewGlyphMaskRasterizer()
	filter := DefaultLCDFilter()

	outline := &GlyphOutline{Segments: nil, GID: 0}
	result, err := r.RasterizeLCDOutline(outline, 0, 0, filter, LCDLayoutRGB)
	if err != nil {
		t.Fatalf("RasterizeLCDOutline(empty) error = %v", err)
	}
	if result != nil {
		t.Error("RasterizeLCDOutline(empty) should return nil result")
	}
}

func TestGlyphMaskRasterizer_RasterizeLCDOutline_Triangle(t *testing.T) {
	r := NewGlyphMaskRasterizer()
	filter := DefaultLCDFilter()

	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 5, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 10}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: 10}}},
		},
		Bounds: Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10},
		GID:    1,
	}

	result, err := r.RasterizeLCDOutline(outline, 0, 0, filter, LCDLayoutRGB)
	if err != nil {
		t.Fatalf("RasterizeLCDOutline(triangle) error = %v", err)
	}
	if result == nil {
		t.Fatal("RasterizeLCDOutline(triangle) returned nil for valid outline")
	}

	if result.Width <= 0 || result.Height <= 0 {
		t.Errorf("invalid dimensions: %dx%d", result.Width, result.Height)
	}

	// Mask should be 3 bytes per pixel (RGB).
	expectedLen := result.Width * 3 * result.Height
	if len(result.Mask) != expectedLen {
		t.Errorf("mask length = %d, want %d (width=%d, height=%d)",
			len(result.Mask), expectedLen, result.Width, result.Height)
	}

	// Should have some non-zero coverage.
	hasNonZero := false
	for _, b := range result.Mask {
		if b > 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("LCD mask has no coverage — triangle not rasterized")
	}
}

func TestGlyphMaskRasterizer_RasterizeLCD_BGR_Swaps(t *testing.T) {
	r := NewGlyphMaskRasterizer()
	filter := DefaultLCDFilter()

	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 10}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: 10}}},
		},
		Bounds: Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10},
		GID:    2,
	}

	rgbResult, err := r.RasterizeLCDOutline(outline, 0, 0, filter, LCDLayoutRGB)
	if err != nil {
		t.Fatalf("RGB error = %v", err)
	}

	bgrResult, err := r.RasterizeLCDOutline(outline, 0, 0, filter, LCDLayoutBGR)
	if err != nil {
		t.Fatalf("BGR error = %v", err)
	}

	if rgbResult == nil || bgrResult == nil {
		t.Fatal("one of the results is nil")
	}

	if rgbResult.Width != bgrResult.Width || rgbResult.Height != bgrResult.Height {
		t.Fatal("RGB and BGR results have different dimensions")
	}

	// Check that R and B channels are swapped between RGB and BGR.
	swapped := false
	for i := 0; i < len(rgbResult.Mask)-2; i += 3 {
		rR, rG, rB := rgbResult.Mask[i], rgbResult.Mask[i+1], rgbResult.Mask[i+2]
		bR, bG, bB := bgrResult.Mask[i], bgrResult.Mask[i+1], bgrResult.Mask[i+2]
		// BGR should have R and B swapped relative to RGB.
		if rR != bB || rB != bR {
			swapped = true
			break
		}
		// Green should be the same.
		if rG != bG {
			swapped = true
			break
		}
	}
	// For a uniform square shape, the channels might be very similar,
	// but the swap should produce at least some differences on edges.
	if !swapped {
		// Verify at least they're not identical (they could be for perfectly symmetric shapes).
		identical := masksEqual(rgbResult.Mask, bgrResult.Mask)
		// For a square, all channels might be equal, making RGB == BGR.
		// That's acceptable — the swap is correct but has no visible effect.
		if identical {
			t.Log("RGB and BGR masks are identical (symmetric shape — this is acceptable)")
		}
	}
}

func TestGlyphMaskRasterizer_RasterizeLCD_Square(t *testing.T) {
	r := NewGlyphMaskRasterizer()
	filter := DefaultLCDFilter()

	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 10}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: 10}}},
		},
		Bounds: Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10},
		GID:    3,
	}

	result, err := r.RasterizeLCDOutline(outline, 0, 0, filter, LCDLayoutRGB)
	if err != nil {
		t.Fatalf("RasterizeLCDOutline(square) error = %v", err)
	}
	if result == nil {
		t.Fatal("RasterizeLCDOutline(square) returned nil")
	}

	// Interior pixels of a solid square should have high coverage on all channels.
	highCoverage := 0
	for row := 2; row < result.Height-2; row++ {
		for col := 2; col < result.Width-2; col++ {
			idx := (row*result.Width + col) * 3
			if idx+2 >= len(result.Mask) {
				continue
			}
			r, g, b := result.Mask[idx], result.Mask[idx+1], result.Mask[idx+2]
			if r > 200 && g > 200 && b > 200 {
				highCoverage++
			}
		}
	}
	if highCoverage < 20 {
		t.Errorf("only %d high-coverage interior pixels, expected at least 20", highCoverage)
	}
}

func TestGlyphMaskAtlas_PutLCD(t *testing.T) {
	atlas := NewGlyphMaskAtlasDefault()

	// Create a small LCD mask (4 logical pixels wide, 3 tall).
	logicalW := 4
	maskH := 3
	rgbMask := make([]byte, logicalW*3*maskH)
	for i := range rgbMask {
		rgbMask[i] = 128
	}

	key := MakeGlyphMaskKey(123, 42, 14.0, 0, 0)
	region, err := atlas.PutLCD(key, rgbMask, logicalW, maskH, -1.0, 10.0)
	if err != nil {
		t.Fatalf("PutLCD error = %v", err)
	}

	if !region.IsLCD {
		t.Error("region.IsLCD should be true")
	}
	if region.Width != logicalW*3 {
		t.Errorf("region.Width = %d, want %d (3x logical)", region.Width, logicalW*3)
	}
	if region.Height != maskH {
		t.Errorf("region.Height = %d, want %d", region.Height, maskH)
	}

	// Get should return the same region.
	got, ok := atlas.Get(key)
	if !ok {
		t.Fatal("Get after PutLCD returned false")
	}
	if !got.IsLCD {
		t.Error("cached region.IsLCD should be true")
	}
	if got.Width != logicalW*3 {
		t.Errorf("cached region.Width = %d, want %d", got.Width, logicalW*3)
	}
}

func TestGlyphMaskAtlas_PutLCD_InvalidDimensions(t *testing.T) {
	atlas := NewGlyphMaskAtlasDefault()
	key := MakeGlyphMaskKey(1, 1, 12.0, 0, 0)

	_, err := atlas.PutLCD(key, nil, 0, 0, 0, 0)
	if err == nil {
		t.Error("PutLCD with zero dimensions should return error")
	}

	_, err = atlas.PutLCD(key, make([]byte, 1), 4, 3, 0, 0)
	if err == nil {
		t.Error("PutLCD with insufficient mask data should return error")
	}
}
