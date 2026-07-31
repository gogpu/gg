package gg

import (
	"testing"
)

// Hairline rasterizer TDD tests.
//
// These tests have STRICT assertions that FAIL without a proper hairline
// rasterizer and PASS with one. A hairline rasterizer renders 1px strokes
// directly (Wu's AA line algorithm) without stroke expansion, producing
// crisp lines with high peak coverage regardless of sub-pixel position.
//
// Without hairline rasterizer (current): stroke expansion → fill → analytic AA
// → peak alpha ~128 for non-aligned strokes (pixel straddling).
//
// With hairline rasterizer (target): direct line drawing → peak alpha ≥ 200
// for any position, ≥ 240 for axis-aligned.

// TestHairline_HorizontalLine_PeakAlpha verifies that a 1px horizontal stroke
// at a non-pixel-center Y position produces high peak alpha.
// Without hairline: stroke at y=50.3 straddles rows 50/51 → peak ~179.
// With hairline: direct draw → peak ≥ 200.
func TestHairline_HorizontalLine_PeakAlpha(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(1.0)

	// Deliberately non-aligned Y to trigger straddling.
	dc.MoveTo(10, 50.3)
	dc.LineTo(90, 50.3)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke: %v", err)
	}

	peak := peakDarkness(dc, 0, 45, 100, 56)
	t.Logf("horizontal y=50.3: peak darkness = %d/255", peak)

	if peak < 200 {
		t.Errorf("peak darkness = %d, want ≥ 200 (hairline should produce crisp line at any Y)", peak)
	}
}

// TestHairline_VerticalLine_PeakAlpha verifies that a 1px vertical stroke
// at a non-pixel-center X position produces high peak alpha.
func TestHairline_VerticalLine_PeakAlpha(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(1.0)

	dc.MoveTo(50.3, 10)
	dc.LineTo(50.3, 90)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke: %v", err)
	}

	peak := peakDarkness(dc, 45, 0, 56, 100)
	t.Logf("vertical x=50.3: peak darkness = %d/255", peak)

	if peak < 200 {
		t.Errorf("peak darkness = %d, want ≥ 200 (hairline should produce crisp line at any X)", peak)
	}
}

// TestHairline_DiagonalLine_PeakAlpha verifies that a 1px diagonal stroke
// produces reasonable peak alpha (not faint).
func TestHairline_DiagonalLine_PeakAlpha(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(1.0)

	dc.MoveTo(10, 10)
	dc.LineTo(90, 90)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke: %v", err)
	}

	peak := peakDarkness(dc, 0, 0, 100, 100)
	t.Logf("diagonal: peak darkness = %d/255", peak)

	if peak < 180 {
		t.Errorf("peak darkness = %d, want ≥ 180 (hairline diagonal should be visible)", peak)
	}
}

// TestHairline_AlignedLine_FullCoverage verifies that a 1px horizontal stroke
// at pixel center (y=N.5) produces near-full coverage (≥ 240).
func TestHairline_AlignedLine_FullCoverage(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(1.0)

	dc.MoveTo(10, 50.5)
	dc.LineTo(90, 50.5)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke: %v", err)
	}

	peak := peakDarkness(dc, 10, 49, 90, 52)
	t.Logf("aligned y=50.5: peak darkness = %d/255", peak)

	if peak < 240 {
		t.Errorf("peak darkness = %d, want ≥ 240 (pixel-aligned hairline should be near-opaque)", peak)
	}
}

// TestHairline_SubPixelWidth_ReducedAlpha verifies that stroke-width < 1.0
// renders with proportionally reduced alpha (Skia pattern: alpha * coverage).
func TestHairline_SubPixelWidth_ReducedAlpha(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(0.5) // Half-pixel width

	dc.MoveTo(10, 50.5)
	dc.LineTo(90, 50.5)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke: %v", err)
	}

	peak := peakDarkness(dc, 10, 49, 90, 52)
	t.Logf("0.5px width: peak darkness = %d/255", peak)

	// 0.5px stroke should be visible but fainter than 1.0px.
	if peak < 80 {
		t.Errorf("peak darkness = %d, want ≥ 80 (0.5px stroke should be visible)", peak)
	}
	if peak > 200 {
		t.Errorf("peak darkness = %d, want ≤ 200 (0.5px stroke should be fainter than 1.0px)", peak)
	}
}

// TestHairline_Circle_PeakAlpha verifies that a stroked circle with width=1
// produces high peak coverage on the circle ring.
func TestHairline_Circle_PeakAlpha(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(1.0)

	dc.DrawCircle(50, 50, 20)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke: %v", err)
	}

	// Sample the right side of the circle (x≈70, y≈50)
	peak := peakDarkness(dc, 68, 48, 73, 53)
	t.Logf("circle right edge: peak darkness = %d/255", peak)

	if peak < 160 {
		t.Errorf("peak darkness = %d, want ≥ 160 (circle hairline via SDF)", peak)
	}
}

// TestHairline_ThickStroke_Unaffected verifies that strokes > 1.5px are NOT
// affected by hairline rendering — they use normal stroke expansion.
func TestHairline_ThickStroke_Unaffected(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(3.0) // Thick stroke — should NOT use hairline

	dc.MoveTo(10, 50)
	dc.LineTo(90, 50)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke: %v", err)
	}

	// 3px stroke should have multiple fully-opaque rows.
	peak := peakDarkness(dc, 10, 48, 90, 53)
	if peak < 250 {
		t.Errorf("3px stroke peak darkness = %d, want ≥ 250", peak)
	}
}

// peakDarkness returns the maximum "darkness" (255 - luminance) in the region.
// On a white background with black strokes, higher = more opaque stroke pixel.
func peakDarkness(dc *Context, x0, y0, x1, y1 int) int {
	img := dc.Image()
	peak := 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c := img.At(x, y)
			r, _, _, _ := c.RGBA()
			lum := int(r >> 8)         // 0-255
			darkness := 255 - lum       // 0 = white, 255 = black
			if darkness > peak {
				peak = darkness
			}
		}
	}
	return peak
}
