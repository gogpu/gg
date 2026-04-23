// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"testing"
)

// Regression tests for AnalyticFiller — Skia AAA port.
// These tests assert EXACT pixel coverage values for known shapes.
// Any change to rasterization code that alters coverage must update these tests.

// buildStarCoverage builds the star shape and returns coverage buffer.
func buildStarCoverage() []uint8 {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)
	return buf
}

// buildRectCoverage builds the float rect and returns coverage buffer.
func buildRectCoverage() []uint8 {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{10.3, 15.4, 90.8, 15.4, 90.8, 86.0, 10.3, 86.0},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)
	return buf
}

// buildPolygonCoverage builds the polygon (no AA) and returns coverage buffer.
func buildPolygonCoverage() []uint8 {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo},
		points: []float32{75.160671, 88.756136, 24.797274, 88.734053, 9.255130, 40.828792, 50.012955, 11.243795, 90.744819, 40.864522},
	}
	eb := NewEdgeBuilder(0) // no AA
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)
	return buf
}

// TestRegression_StarInteriorCoverage verifies that interior star pixels
// have full coverage (255). Any deviation indicates a sub-strip accumulation bug.
func TestRegression_StarInteriorCoverage(t *testing.T) {
	buf := buildStarCoverage()

	// Interior pixels that MUST be 255 (fully inside star with NonZero winding).
	// Note: pentagram interior is complex — center pentagon has winding=2,
	// triangular arms have winding=1. Both are "inside" for NonZero fill.
	// Pixels at (50,70)/(50,80) are OUTSIDE the star (between arms).
	interiorPixels := [][2]int{
		{50, 30}, {50, 50}, {40, 40}, {60, 40},
		{50, 20}, {50, 60},
		{30, 50}, {70, 50}, {40, 60}, {60, 60},
	}
	for _, px := range interiorPixels {
		x, y := px[0], px[1]
		cov := buf[y*100+x]
		if cov != 255 {
			t.Errorf("interior pixel (%d,%d): coverage=%d, want 255", x, y, cov)
		}
	}
}

// TestRegression_StarExteriorCoverage verifies that exterior pixels
// have zero coverage. Leaking coverage indicates edge bounds errors.
func TestRegression_StarExteriorCoverage(t *testing.T) {
	buf := buildStarCoverage()

	exteriorPixels := [][2]int{
		{0, 0}, {99, 99}, {0, 99}, {99, 0},
		{5, 5}, {95, 5}, {5, 95}, {95, 95},
		{50, 0}, {50, 99}, {0, 50}, {99, 50},
	}
	for _, px := range exteriorPixels {
		x, y := px[0], px[1]
		cov := buf[y*100+x]
		if cov != 0 {
			t.Errorf("exterior pixel (%d,%d): coverage=%d, want 0", x, y, cov)
		}
	}
}

// TestRegression_StarEdgePixels verifies coverage at specific edge pixels
// where known fixes were applied. Values from Skia-exact C++ ground truth.
func TestRegression_StarEdgePixels(t *testing.T) {
	buf := buildStarCoverage()

	// These exact values come from C++ Skia-exact tool (verbatim Skia source).
	// Changes to these values indicate rasterization regression.
	type pixelVal struct {
		x, y int
		cov  uint8
		desc string
	}
	cases := []pixelVal{
		// Top vertex edge pixels (E0/E4 from vertex 50,7.5)
		{49, 8, 80, "top vertex, left pixel y=8"},
		{50, 8, 80, "top vertex, right pixel y=8"},
		{49, 9, 160, "top vertex, left pixel y=9"},
		{50, 9, 160, "top vertex, right pixel y=9"},

		// Edge boundary pixels — fixed by trapezoid_to_alpha (area>>8)
		{49, 10, 236, "edge boundary y=10"},
		{50, 10, 236, "edge boundary y=10"},
		{48, 12, 144, "edge boundary y=12"},
		{51, 12, 144, "edge boundary y=12"},

		// Crossing vertex pixels (y=56, edges very close)
		{34, 56, 129, "crossing vertex y=56 — adaptive sub-strip"},
		{65, 56, 129, "crossing vertex y=56 — adaptive sub-strip"},

		// Crossing vertex y=68 (edges_too_close triggers sub-strips)
		{31, 68, 252, "crossing vertex y=68 — sub-strip split"},
		{68, 68, 252, "crossing vertex y=68 — sub-strip split"},

		// Bottom edge pixels
		{25, 86, 165, "bottom edge y=86"},
		{74, 86, 165, "bottom edge y=86"},
	}

	for _, c := range cases {
		cov := buf[c.y*100+c.x]
		if cov != c.cov {
			t.Errorf("pixel (%d,%d) [%s]: coverage=%d, want %d", c.x, c.y, c.desc, cov, c.cov)
		}
	}
}

// TestRegression_RectInteriorCoverage verifies float rect interior is fully covered.
func TestRegression_RectInteriorCoverage(t *testing.T) {
	buf := buildRectCoverage()

	// Interior pixels (well inside rect 10.3,15.4 - 90.8,86.0)
	for y := 20; y <= 80; y += 10 {
		for x := 15; x <= 85; x += 10 {
			cov := buf[y*100+x]
			if cov != 255 {
				t.Errorf("rect interior (%d,%d): coverage=%d, want 255", x, y, cov)
			}
		}
	}
}

// TestRegression_RectEdgePixels verifies float rect edge coverage values.
func TestRegression_RectEdgePixels(t *testing.T) {
	buf := buildRectCoverage()

	// Left edge at x=10.3: pixel 10 should have partial coverage
	// Right edge at x=90.8: pixel 90 should have partial coverage
	// Top edge at y=15.4: pixel row 15 should have partial coverage
	// Bottom edge at y=86.0: pixel row 85 should be full, row 86 should be 0

	// Left edge pixel x=10, y=50 (mid-height, well away from corners)
	leftCov := buf[50*100+10]
	if leftCov == 0 || leftCov == 255 {
		t.Errorf("rect left edge (10,50): coverage=%d, want partial (not 0 or 255)", leftCov)
	}

	// Right edge pixel x=90, y=50
	rightCov := buf[50*100+90]
	if rightCov == 0 || rightCov == 255 {
		t.Errorf("rect right edge (90,50): coverage=%d, want partial (not 0 or 255)", rightCov)
	}

	// Fully inside pixel at x=50, y=50
	if buf[50*100+50] != 255 {
		t.Errorf("rect interior (50,50): coverage=%d, want 255", buf[50*100+50])
	}

	// Outside pixel
	if buf[10*100+5] != 0 {
		t.Errorf("rect exterior (5,10): coverage=%d, want 0", buf[10*100+5])
	}
}

// TestRegression_TrapezoidToAlpha verifies the area>>8 formula fix.
// This was the key fix that brought star coverage from 38 diff to 0 diff.
// Skia source: SkScan_AAAPath.cpp:535-538.
func TestRegression_TrapezoidToAlpha(t *testing.T) {
	// area>>8 must be used, NOT (255*area+32768)>>16
	// For area=40960 (0.625 pixel): >>8 gives 160, rounded gives 159
	cases := []struct {
		l1, l2 int32
		want   uint8
	}{
		{51200, 30720, 160}, // star pixel (49,9) — the original failing case
		{30720, 51200, 160}, // symmetric
		{65536, 65536, 255}, // full pixel both sides
		{32768, 32768, 128}, // half pixel both sides
		{0, 0, 0},           // empty
		{65536, 0, 128},     // triangle zero to full
	}
	for _, c := range cases {
		got := trapezoidToAlphaScaled(c.l1, c.l2, 255)
		if got != c.want {
			t.Errorf("trapezoidToAlphaScaled(%d,%d,255)=%d, want %d", c.l1, c.l2, got, c.want)
		}
	}
}

// TestRegression_PolygonKnownBug documents BUG-RAST-011.
// Polygon with near-horizontal edges at aaShift=0 has coverage diff vs Skia.
// Root cause: edges starting mid-row are not inserted until next pixel row.
func TestRegression_PolygonKnownBug(t *testing.T) {
	buf := buildPolygonCoverage()

	// y=40: edges 2/3 end at UpperY=40.75, edges 1/4 start at 40.75
	// Our code gives coverage=191 for interior (only 3/4 strip)
	// Skia gives 255 (both sub-strips contribute)
	cov40 := buf[40*100+50]
	if cov40 == 255 {
		// BUG-RAST-011 is fixed! Update this test.
		t.Logf("BUG-RAST-011 appears FIXED at y=40: coverage=%d", cov40)
	} else if cov40 != 191 {
		t.Errorf("unexpected coverage at (50,40): got=%d, want 191 (known bug) or 255 (fixed)",
			cov40)
	}

	// Interior pixels well inside polygon must be 255
	if buf[30*100+50] != 255 {
		t.Errorf("polygon interior (50,30): coverage=%d, want 255", buf[30*100+50])
	}
	if buf[60*100+50] != 255 {
		t.Errorf("polygon interior (50,60): coverage=%d, want 255", buf[60*100+50])
	}
}

// TestRegression_StarNonZeroCoverage verifies that total non-zero coverage
// pixel count matches Skia-exact. Any change indicates edge bounds shift.
func TestRegression_StarNonZeroCoverage(t *testing.T) {
	buf := buildStarCoverage()
	nonZero := 0
	for _, v := range buf {
		if v > 0 {
			nonZero++
		}
	}
	// Skia-exact C++ tool reports 2282 non-zero pixels
	if nonZero != 2282 {
		t.Errorf("star non-zero pixels=%d, want 2282 (Skia-exact)", nonZero)
	}
}

// TestRegression_CoverageMonotonicity verifies that interior pixels have
// monotonically increasing coverage as we move inward from the edge.
func TestRegression_CoverageMonotonicity(t *testing.T) {
	buf := buildStarCoverage()

	// At y=50 (middle of star), coverage should increase from left edge inward
	// Find first non-zero pixel from left
	var firstNonZero, lastNonZero int
	for x := 0; x < 100; x++ {
		if buf[50*100+x] > 0 {
			firstNonZero = x
			break
		}
	}
	for x := 99; x >= 0; x-- {
		if buf[50*100+x] > 0 {
			lastNonZero = x
			break
		}
	}

	// From edge inward, coverage should be non-decreasing until 255
	prev := uint8(0)
	for x := firstNonZero; x <= (firstNonZero+lastNonZero)/2; x++ {
		cov := buf[50*100+x]
		if cov < prev && prev != 255 {
			t.Errorf("non-monotonic coverage at y=50, x=%d: %d < prev %d", x, cov, prev)
		}
		prev = cov
	}
}

// TestRegression_NearHorizontalEdgeBleed reproduces BUG-RAST-011 (#235):
// near-horizontal edges from stroke expansion cause coverage to bleed
// far beyond the shape boundary.
//
// A thin near-horizontal parallelogram (typical stroke of a horizontal line)
// should have coverage only within ~2px of the shape. Coverage 10+ pixels
// away indicates slope blowup.
func TestRegression_NearHorizontalEdgeBleed(t *testing.T) {
	// Near-horizontal parallelogram simulating a 1px stroke of a line
	// from (10, 50) to (90, 50.3) — dy=0.3 over 80px, dx/dy ≈ 267.
	// Stroke offset ±0.5px perpendicular creates:
	//   top:    (10, 49.5) → (90, 49.8)   dy=0.3
	//   right:  (90, 49.8) → (90, 50.8)   dy=1.0
	//   bottom: (90, 50.8) → (10, 50.5)   dy=-0.3
	//   left:   (10, 50.5) → (10, 49.5)   dy=-1.0
	path := &testPath{
		verbs: []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{
			10.0, 49.5,
			90.0, 49.8,
			90.0, 50.8,
			10.0, 50.5,
		},
	}

	eb := NewEdgeBuilder(2) // 4x AA
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	const w, h = 100, 100
	buf := make([]uint8, w*h)
	FillToBuffer(eb, w, h, FillRuleNonZero, buf)

	// The parallelogram spans y=49.5 to y=50.8, so coverage should be
	// confined to rows 49-51 (with AA fringe).
	// Rows far away (y ≤ 45, y ≥ 55) MUST have zero coverage.
	for y := 0; y <= 45; y++ {
		for x := 0; x < w; x++ {
			if buf[y*w+x] != 0 {
				t.Errorf("bleed above shape: pixel (%d,%d) coverage=%d, want 0", x, y, buf[y*w+x])
				return
			}
		}
	}
	for y := 55; y < h; y++ {
		for x := 0; x < w; x++ {
			if buf[y*w+x] != 0 {
				t.Errorf("bleed below shape: pixel (%d,%d) coverage=%d, want 0", x, y, buf[y*w+x])
				return
			}
		}
	}

	// Within the shape (x=30..70, y=50): should have non-zero coverage
	hasInterior := false
	for x := 30; x <= 70; x++ {
		if buf[50*w+x] > 0 {
			hasInterior = true
			break
		}
	}
	if !hasInterior {
		t.Error("no interior coverage found at y=50 — shape not rendered at all")
	}

	// Check horizontal bleed: coverage outside x=8..92 should be zero
	// for rows 49-51 (the shape rows).
	for y := 49; y <= 51; y++ {
		for x := 0; x < 8; x++ {
			if buf[y*w+x] != 0 {
				t.Errorf("horizontal bleed left: pixel (%d,%d) coverage=%d, want 0", x, y, buf[y*w+x])
			}
		}
		for x := 93; x < w; x++ {
			if buf[y*w+x] != 0 {
				t.Errorf("horizontal bleed right: pixel (%d,%d) coverage=%d, want 0", x, y, buf[y*w+x])
			}
		}
	}
}

// TestRegression_SteepNearHorizontalBleed tests an even more extreme case:
// dy=0.1 over 80px (slope ratio 800:1). This is the GIS coastline scenario.
func TestRegression_SteepNearHorizontalBleed(t *testing.T) {
	// Extreme near-horizontal: dy=0.1 over 80px
	path := &testPath{
		verbs: []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{
			10.0, 49.95,
			90.0, 50.05,
			90.0, 51.05,
			10.0, 50.95,
		},
	}

	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	const w, h = 100, 100
	buf := make([]uint8, w*h)
	FillToBuffer(eb, w, h, FillRuleNonZero, buf)

	// Coverage must be confined to rows 49-52 at most.
	// Rows y ≤ 45 and y ≥ 55 MUST have zero coverage.
	for y := 0; y <= 45; y++ {
		for x := 0; x < w; x++ {
			if buf[y*w+x] != 0 {
				t.Errorf("extreme bleed above: pixel (%d,%d) coverage=%d, want 0", x, y, buf[y*w+x])
				return
			}
		}
	}
	for y := 55; y < h; y++ {
		for x := 0; x < w; x++ {
			if buf[y*w+x] != 0 {
				t.Errorf("extreme bleed below: pixel (%d,%d) coverage=%d, want 0", x, y, buf[y*w+x])
				return
			}
		}
	}
}
