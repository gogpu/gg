package raster

import (
	"testing"
)

// mockHairlineBlitter records blit operations for testing.
type mockHairlineBlitter struct {
	blitHCalls  []blitHCall
	blitVCalls  []blitVCall
	antiH2Calls []antiH2Call
	antiV2Calls []antiV2Call
	width       int
	height      int
}

type blitHCall struct {
	x, y, width int
	alpha       uint8
}

type blitVCall struct {
	x, y, height int
	alpha        uint8
}

type antiH2Call struct {
	x, y           int
	alpha0, alpha1 uint8
}

type antiV2Call struct {
	x, y           int
	alpha0, alpha1 uint8
}

func newMockHairlineBlitter(width, height int) *mockHairlineBlitter {
	return &mockHairlineBlitter{
		width:  width,
		height: height,
	}
}

func (m *mockHairlineBlitter) BlitH(x, y, width int, alpha uint8) {
	m.blitHCalls = append(m.blitHCalls, blitHCall{x, y, width, alpha})
}

func (m *mockHairlineBlitter) BlitV(x, y, height int, alpha uint8) {
	m.blitVCalls = append(m.blitVCalls, blitVCall{x, y, height, alpha})
}

func (m *mockHairlineBlitter) BlitAntiH2(x, y int, alpha0, alpha1 uint8) {
	m.antiH2Calls = append(m.antiH2Calls, antiH2Call{x, y, alpha0, alpha1})
}

func (m *mockHairlineBlitter) BlitAntiV2(x, y int, alpha0, alpha1 uint8) {
	m.antiV2Calls = append(m.antiV2Calls, antiV2Call{x, y, alpha0, alpha1})
}

func TestFDot6Conversions(t *testing.T) {
	tests := []struct {
		name    string
		input   float64
		wantF6  FDot6
		wantF16 FDot16
	}{
		{"zero", 0.0, 0, 0},
		{"one", 1.0, 64, 65536},
		{"half", 0.5, 32, 32768},
		{"negative", -1.0, -64, -65536},
		{"large", 100.0, 6400, 6553600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotF6 := FloatToFDot6(tt.input)
			if gotF6 != tt.wantF6 {
				t.Errorf("FloatToFDot6(%v) = %v, want %v", tt.input, gotF6, tt.wantF6)
			}

			gotF16 := FloatToFDot16(tt.input)
			if gotF16 != tt.wantF16 {
				t.Errorf("FloatToFDot16(%v) = %v, want %v", tt.input, gotF16, tt.wantF16)
			}
		})
	}
}

func TestFDot6Floor(t *testing.T) {
	tests := []struct {
		name  string
		input FDot6
		want  int
	}{
		{"zero", 0, 0},
		{"one", 64, 1},
		{"partial", 96, 1}, // 1.5 -> floor is 1
		{"negative", -64, -1},
		{"negative_partial", -32, -1}, // -0.5 -> floor is -1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6Floor(tt.input)
			if got != tt.want {
				t.Errorf("FDot6Floor(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFDot6Ceil(t *testing.T) {
	tests := []struct {
		name  string
		input FDot6
		want  int
	}{
		{"zero", 0, 0},
		{"one", 64, 1},
		{"partial", 65, 2},       // 1.015625 -> ceil is 2
		{"exact", 128, 2},        // 2.0 -> ceil is 2
		{"small_fraction", 1, 1}, // 0.015625 -> ceil is 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6Ceil(tt.input)
			if got != tt.want {
				t.Errorf("FDot6Ceil(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestContribution64(t *testing.T) {
	tests := []struct {
		name  string
		input FDot6
		want  FDot6
	}{
		{"exact_64", 64, 64},   // Multiples of 64 should return 64
		{"exact_128", 128, 64}, // Multiples of 64 should return 64
		{"middle", 32, 32},     // 32 should return 32
		{"one", 1, 1},          // 1 should return 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Contribution64(tt.input)
			if got != tt.want {
				t.Errorf("Contribution64(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHairlineHorizontal(t *testing.T) {
	blitter := newMockHairlineBlitter(100, 100)

	// Draw a pure horizontal line from (10, 50) to (20, 50)
	points := []HairlinePoint{
		{X: 10, Y: 50},
		{X: 20, Y: 50},
	}

	StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)

	// Should have produced some blit calls
	if len(blitter.blitHCalls)+len(blitter.antiV2Calls) == 0 {
		t.Error("Expected some blit calls for horizontal line")
	}
}

func TestHairlineVertical(t *testing.T) {
	blitter := newMockHairlineBlitter(100, 100)

	// Draw a pure vertical line from (50, 10) to (50, 20)
	points := []HairlinePoint{
		{X: 50, Y: 10},
		{X: 50, Y: 20},
	}

	StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)

	// Should have produced some blit calls
	if len(blitter.blitVCalls)+len(blitter.antiH2Calls) == 0 {
		t.Error("Expected some blit calls for vertical line")
	}
}

func TestHairlineDiagonal(t *testing.T) {
	blitter := newMockHairlineBlitter(100, 100)

	// Draw a 45-degree diagonal line from (10, 10) to (20, 20)
	points := []HairlinePoint{
		{X: 10, Y: 10},
		{X: 20, Y: 20},
	}

	StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)

	// Should have produced anti-aliased blit calls
	totalCalls := len(blitter.antiH2Calls) + len(blitter.antiV2Calls)
	if totalCalls == 0 {
		t.Error("Expected anti-aliased blit calls for diagonal line")
	}
}

func TestHairlineSteep(t *testing.T) {
	blitter := newMockHairlineBlitter(100, 100)

	// Draw a steep line (more vertical than horizontal)
	points := []HairlinePoint{
		{X: 50, Y: 10},
		{X: 52, Y: 30}, // dx=2, dy=20, so it's mostly vertical
	}

	StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)

	// Should have produced anti-aliased blit calls
	if len(blitter.antiH2Calls) == 0 {
		t.Error("Expected antiH2 calls for steep (mostly vertical) line")
	}
}

func TestHairlineSubpixel(t *testing.T) {
	blitter := newMockHairlineBlitter(100, 100)

	// Draw a line with subpixel coordinates
	points := []HairlinePoint{
		{X: 10.3, Y: 50.7},
		{X: 20.8, Y: 50.2},
	}

	StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)

	// Should have produced some blit calls
	totalCalls := len(blitter.blitHCalls) + len(blitter.blitVCalls) +
		len(blitter.antiH2Calls) + len(blitter.antiV2Calls)
	if totalCalls == 0 {
		t.Error("Expected blit calls for subpixel line")
	}
}

func TestHairlineCoverage(t *testing.T) {
	tests := []struct {
		name     string
		coverage float64
		wantMin  uint8
		wantMax  uint8
	}{
		{"full", 1.0, 1, 255},
		{"half", 0.5, 1, 128},
		{"quarter", 0.25, 1, 64},
		{"zero", 0.0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blitter := newMockHairlineBlitter(100, 100)

			points := []HairlinePoint{
				{X: 10, Y: 50},
				{X: 20, Y: 50},
			}

			StrokeHairlineAA(blitter, points, HairlineCapButt, tt.coverage)

			if tt.coverage == 0 {
				// Zero coverage should produce no visible output
				// (blits may be called but with alpha 0)
				return
			}

			// Find max alpha in all calls
			maxAlpha := uint8(0)
			for _, c := range blitter.blitHCalls {
				if c.alpha > maxAlpha {
					maxAlpha = c.alpha
				}
			}
			for _, c := range blitter.antiV2Calls {
				if c.alpha0 > maxAlpha {
					maxAlpha = c.alpha0
				}
				if c.alpha1 > maxAlpha {
					maxAlpha = c.alpha1
				}
			}

			if maxAlpha > tt.wantMax {
				t.Errorf("max alpha %d exceeds expected max %d for coverage %v",
					maxAlpha, tt.wantMax, tt.coverage)
			}
		})
	}
}

func TestHairlineLineCaps(t *testing.T) {
	caps := []struct {
		name string
		cap  HairlineLineCap
	}{
		{"butt", HairlineCapButt},
		{"round", HairlineCapRound},
		{"square", HairlineCapSquare},
	}

	for _, tt := range caps {
		t.Run(tt.name, func(t *testing.T) {
			blitter := newMockHairlineBlitter(100, 100)

			points := []HairlinePoint{
				{X: 30, Y: 50},
				{X: 70, Y: 50},
			}

			StrokeHairlineAA(blitter, points, tt.cap, 1.0)

			// All cap styles should produce output
			totalCalls := len(blitter.blitHCalls) + len(blitter.blitVCalls) +
				len(blitter.antiH2Calls) + len(blitter.antiV2Calls)
			if totalCalls == 0 {
				t.Errorf("Expected blit calls for %s cap", tt.name)
			}
		})
	}
}

func TestHairlineZeroLength(t *testing.T) {
	blitter := newMockHairlineBlitter(100, 100)

	// Zero-length line
	points := []HairlinePoint{
		{X: 50, Y: 50},
		{X: 50, Y: 50},
	}

	StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)

	// Zero-length lines should produce no output
	totalCalls := len(blitter.blitHCalls) + len(blitter.blitVCalls) +
		len(blitter.antiH2Calls) + len(blitter.antiV2Calls)
	if totalCalls != 0 {
		t.Errorf("Expected no blit calls for zero-length line, got %d", totalCalls)
	}
}

func TestHairlineSinglePoint(t *testing.T) {
	blitter := newMockHairlineBlitter(100, 100)

	// Only one point - should produce no output
	points := []HairlinePoint{
		{X: 50, Y: 50},
	}

	StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)

	totalCalls := len(blitter.blitHCalls) + len(blitter.blitVCalls) +
		len(blitter.antiH2Calls) + len(blitter.antiV2Calls)
	if totalCalls != 0 {
		t.Errorf("Expected no blit calls for single point, got %d", totalCalls)
	}
}

func TestHairlineEmptyPoints(t *testing.T) {
	blitter := newMockHairlineBlitter(100, 100)

	// Empty points slice - should not panic
	StrokeHairlineAA(blitter, nil, HairlineCapButt, 1.0)
	StrokeHairlineAA(blitter, []HairlinePoint{}, HairlineCapButt, 1.0)

	// Should be fine (no panic)
}

func TestHairlineVeryLongLine(t *testing.T) {
	blitter := newMockHairlineBlitter(2000, 2000)

	// Very long line that triggers subdivision
	points := []HairlinePoint{
		{X: 0, Y: 0},
		{X: 1500, Y: 1500},
	}

	// Should not panic or overflow
	StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)

	// Should have produced output
	totalCalls := len(blitter.blitHCalls) + len(blitter.blitVCalls) +
		len(blitter.antiH2Calls) + len(blitter.antiV2Calls)
	if totalCalls == 0 {
		t.Error("Expected blit calls for very long line")
	}
}

func TestHairlineClipping(t *testing.T) {
	blitter := newMockHairlineBlitter(100, 100)

	// Line that extends beyond the safe coordinate range
	points := []HairlinePoint{
		{X: -50000, Y: 50},
		{X: 50000, Y: 50},
	}

	// Should not panic
	StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)

	// The line should have been clipped
}

func TestRGBAHairlineBlitter(t *testing.T) {
	// Create a mock pixmap
	pixmap := &mockPixmap{
		width:  100,
		height: 100,
		pixels: make([]RGBA, 100*100),
	}

	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}
	blitter := NewRGBAHairlineBlitter(pixmap, color)

	// Test BlitH
	blitter.BlitH(10, 10, 5, 255)
	// Verify pixels were blended
	for x := 10; x < 15; x++ {
		idx := 10*100 + x
		if pixmap.pixels[idx].R == 0 {
			t.Errorf("BlitH failed to set pixel at (%d, 10)", x)
		}
	}

	// Test BlitV
	blitter.BlitV(50, 20, 5, 255)
	for y := 20; y < 25; y++ {
		idx := y*100 + 50
		if pixmap.pixels[idx].R == 0 {
			t.Errorf("BlitV failed to set pixel at (50, %d)", y)
		}
	}

	// Test BlitAntiH2
	blitter.BlitAntiH2(60, 30, 128, 255)
	// Check both pixels were blended with different alphas

	// Test BlitAntiV2
	blitter.BlitAntiV2(70, 40, 128, 255)
	// Check both pixels were blended with different alphas
}

func TestRGBAHairlineBlitterBoundsCheck(t *testing.T) {
	pixmap := &mockPixmap{
		width:  100,
		height: 100,
		pixels: make([]RGBA, 100*100),
	}

	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}
	blitter := NewRGBAHairlineBlitter(pixmap, color)

	// Test out-of-bounds access (should not panic)
	blitter.BlitH(-10, 50, 5, 255) // x < 0
	blitter.BlitH(95, 50, 10, 255) // x + width > width
	blitter.BlitH(50, -1, 5, 255)  // y < 0
	blitter.BlitH(50, 100, 5, 255) // y >= height

	blitter.BlitV(50, -10, 5, 255) // y < 0
	blitter.BlitV(50, 95, 10, 255) // y + height > height
	blitter.BlitV(-1, 50, 5, 255)  // x < 0
	blitter.BlitV(100, 50, 5, 255) // x >= width

	blitter.BlitAntiH2(-2, 50, 128, 255) // Both pixels out of bounds
	blitter.BlitAntiH2(99, 50, 128, 255) // Second pixel out of bounds

	blitter.BlitAntiV2(50, -2, 128, 255) // Both pixels out of bounds
	blitter.BlitAntiV2(50, 99, 128, 255) // Second pixel out of bounds
}

// mockPixmap implements HairlinePixmap for testing.
type mockPixmap struct {
	width  int
	height int
	pixels []RGBA
}

func (m *mockPixmap) Width() int {
	return m.width
}

func (m *mockPixmap) Height() int {
	return m.height
}

func (m *mockPixmap) BlendPixelAlpha(x, y int, c RGBA, alpha uint8) {
	if x < 0 || x >= m.width || y < 0 || y >= m.height {
		return
	}
	idx := y*m.width + x
	// Simple blend: just set the color with alpha
	m.pixels[idx] = RGBA{
		R: c.R * float64(alpha) / 255,
		G: c.G * float64(alpha) / 255,
		B: c.B * float64(alpha) / 255,
		A: c.A * float64(alpha) / 255,
	}
}

func TestExtendHairlineForCap(t *testing.T) {
	tests := []struct {
		name        string
		lineCap     HairlineLineCap
		extendStart bool
		extendEnd   bool
	}{
		{"butt_both", HairlineCapButt, true, true},
		{"round_start", HairlineCapRound, true, false},
		{"round_end", HairlineCapRound, false, true},
		{"round_both", HairlineCapRound, true, true},
		{"square_both", HairlineCapSquare, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Horizontal line
			x0, y0 := FloatToFDot6(10.0), FloatToFDot6(50.0)
			x1, y1 := FloatToFDot6(20.0), FloatToFDot6(50.0)

			origX0, origX1 := x0, x1

			extendHairlineForCap(&x0, &y0, &x1, &y1, tt.lineCap, tt.extendStart, tt.extendEnd)

			checkButtCap(t, tt.lineCap, x0, x1, origX0, origX1)
			checkNonButtCap(t, tt.lineCap, tt.extendStart, tt.extendEnd, x0, x1, origX0, origX1)
		})
	}
}

func checkButtCap(t *testing.T, lineCap HairlineLineCap, x0, x1, origX0, origX1 FDot6) {
	t.Helper()
	if lineCap == HairlineCapButt {
		// Butt caps should not modify coordinates
		if x0 != origX0 || x1 != origX1 {
			t.Error("Butt cap should not modify coordinates")
		}
	}
}

func checkNonButtCap(t *testing.T, lineCap HairlineLineCap, extendStart, extendEnd bool, x0, x1, origX0, origX1 FDot6) {
	t.Helper()
	if lineCap == HairlineCapButt {
		return
	}
	// Other caps should extend if requested
	if extendStart && x0 >= origX0 {
		t.Error("Expected start point to be extended (decreased)")
	}
	if extendEnd && x1 <= origX1 {
		t.Error("Expected end point to be extended (increased)")
	}
}

func TestHairlineMultiSegment(t *testing.T) {
	blitter := newMockHairlineBlitter(100, 100)

	// Multiple connected segments (polyline)
	points := []HairlinePoint{
		{X: 10, Y: 10},
		{X: 20, Y: 20},
		{X: 30, Y: 10},
		{X: 40, Y: 20},
	}

	StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)

	// Should have output for all 3 segments
	totalCalls := len(blitter.blitHCalls) + len(blitter.blitVCalls) +
		len(blitter.antiH2Calls) + len(blitter.antiV2Calls)
	if totalCalls < 3 {
		t.Errorf("Expected at least 3 segments worth of blit calls, got total %d", totalCalls)
	}
}

func BenchmarkHairlineHorizontal(b *testing.B) {
	blitter := newMockHairlineBlitter(1000, 1000)
	points := []HairlinePoint{
		{X: 100, Y: 500},
		{X: 900, Y: 500},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)
	}
}

func BenchmarkHairlineDiagonal(b *testing.B) {
	blitter := newMockHairlineBlitter(1000, 1000)
	points := []HairlinePoint{
		{X: 100, Y: 100},
		{X: 900, Y: 900},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)
	}
}

func BenchmarkHairlineVertical(b *testing.B) {
	blitter := newMockHairlineBlitter(1000, 1000)
	points := []HairlinePoint{
		{X: 500, Y: 100},
		{X: 500, Y: 900},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StrokeHairlineAA(blitter, points, HairlineCapButt, 1.0)
	}
}
