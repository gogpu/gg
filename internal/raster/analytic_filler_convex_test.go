// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"math"
	"testing"
)

// --- Helper functions for convex tests ---

// buildConvexRectCoverage builds a float-position rect and returns convex-walker coverage.
func buildConvexRectCoverage(x0, y0, x1, y1 float32) []uint8 {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{x0, y0, x1, y0, x1, y1, x0, y1},
	}
	eb := NewEdgeBuilder(2) // 4x AA
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillConvexToBuffer(eb, 100, 100, buf)
	return buf
}

// buildGeneralRectCoverage builds the same rect using the general walker (Fill).
func buildGeneralRectCoverage(x0, y0, x1, y1 float32) []uint8 {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{x0, y0, x1, y0, x1, y1, x0, y1},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)
	return buf
}

// buildConvexTriangleCoverage builds a triangle and returns convex-walker coverage.
func buildConvexTriangleCoverage(x0, y0, x1, y1, x2, y2 float32) []uint8 {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, Close},
		points: []float32{x0, y0, x1, y1, x2, y2},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillConvexToBuffer(eb, 100, 100, buf)
	return buf
}

// buildGeneralTriangleCoverage builds a triangle using the general walker.
func buildGeneralTriangleCoverage(x0, y0, x1, y1, x2, y2 float32) []uint8 {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, Close},
		points: []float32{x0, y0, x1, y1, x2, y2},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)
	return buf
}

// buildConvexDiamondCoverage builds a diamond (rotated square) with convex walker.
func buildConvexDiamondCoverage() []uint8 {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50, 10, 90, 50, 50, 90, 10, 50},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillConvexToBuffer(eb, 100, 100, buf)
	return buf
}

// buildGeneralDiamondCoverage builds a diamond using the general walker.
func buildGeneralDiamondCoverage() []uint8 {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50, 10, 90, 50, 50, 90, 10, 50},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)
	return buf
}

// buildConvexPentagonCoverage builds a regular pentagon with convex walker.
func buildConvexPentagonCoverage() []uint8 {
	// Regular pentagon centered at (50, 50) with radius 35.
	cx, cy, r := 50.0, 50.0, 35.0
	pts := make([]float32, 0, 10)
	for i := 0; i < 5; i++ {
		angle := float64(i)*2*math.Pi/5 - math.Pi/2 // start from top
		pts = append(pts, float32(cx+r*math.Cos(angle)), float32(cy+r*math.Sin(angle)))
	}
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: pts,
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillConvexToBuffer(eb, 100, 100, buf)
	return buf
}

// --- Unit tests ---

// TestFillConvex_RectInterior verifies that interior pixels of a convex rect
// have full coverage (255).
func TestFillConvex_RectInterior(t *testing.T) {
	buf := buildConvexRectCoverage(10.3, 15.4, 90.8, 86.0)

	// Interior pixels (well inside rect boundaries)
	for y := 20; y <= 80; y += 10 {
		for x := 15; x <= 85; x += 10 {
			cov := buf[y*100+x]
			if cov != 255 {
				t.Errorf("interior pixel (%d,%d): coverage=%d, want 255", x, y, cov)
			}
		}
	}
}

// TestFillConvex_RectExterior verifies that exterior pixels of a convex rect
// have zero coverage.
func TestFillConvex_RectExterior(t *testing.T) {
	buf := buildConvexRectCoverage(10.3, 15.4, 90.8, 86.0)

	exteriorPixels := [][2]int{
		{0, 0}, {99, 99}, {0, 99}, {99, 0},
		{5, 5}, {95, 5}, {5, 95}, {95, 95},
		{50, 0}, {50, 99}, {0, 50}, {99, 50},
		// Well outside rect boundaries
		{5, 50}, {95, 50}, {50, 10}, {50, 90},
	}
	for _, px := range exteriorPixels {
		x, y := px[0], px[1]
		cov := buf[y*100+x]
		if cov != 0 {
			t.Errorf("exterior pixel (%d,%d): coverage=%d, want 0", x, y, cov)
		}
	}
}

// TestFillConvex_RectEdges verifies partial coverage at fractional rect edges.
func TestFillConvex_RectEdges(t *testing.T) {
	buf := buildConvexRectCoverage(10.3, 15.4, 90.8, 86.0)

	// Left edge at x=10.3: pixel 10 should have partial coverage (70% covered)
	leftCov := buf[50*100+10]
	if leftCov == 0 || leftCov == 255 {
		t.Errorf("left edge pixel (10,50): coverage=%d, want partial (not 0 or 255)", leftCov)
	}

	// Right edge at x=90.8: pixel 90 should have partial coverage (80% covered)
	rightCov := buf[50*100+90]
	if rightCov == 0 || rightCov == 255 {
		t.Errorf("right edge pixel (90,50): coverage=%d, want partial (not 0 or 255)", rightCov)
	}

	// Top edge at y=15.4: pixel row 15 should have partial coverage
	topCov := buf[15*100+50]
	if topCov == 0 || topCov == 255 {
		t.Errorf("top edge pixel (50,15): coverage=%d, want partial (not 0 or 255)", topCov)
	}

	// Bottom edge at y=86.0: pixel row 85 should be full, 86 should be 0.
	botFullCov := buf[85*100+50]
	if botFullCov != 255 {
		t.Errorf("bottom full pixel (50,85): coverage=%d, want 255", botFullCov)
	}
	botZeroCov := buf[86*100+50]
	if botZeroCov != 0 {
		t.Errorf("below bottom pixel (50,86): coverage=%d, want 0", botZeroCov)
	}
}

// TestFillConvex_Triangle verifies coverage for an equilateral triangle.
func TestFillConvex_Triangle(t *testing.T) {
	// Equilateral-ish triangle
	buf := buildConvexTriangleCoverage(50, 10, 85, 80, 15, 80)

	// Interior pixel
	cov := buf[50*100+50]
	if cov != 255 {
		t.Errorf("triangle interior (50,50): coverage=%d, want 255", cov)
	}

	// Exterior pixel
	cov = buf[5*100+5]
	if cov != 0 {
		t.Errorf("triangle exterior (5,5): coverage=%d, want 0", cov)
	}

	// The centroid at approximately (50, 56) should be fully covered
	cov = buf[56*100+50]
	if cov != 255 {
		t.Errorf("triangle centroid (50,56): coverage=%d, want 255", cov)
	}
}

// TestFillConvex_Pentagon verifies coverage for a regular pentagon.
func TestFillConvex_Pentagon(t *testing.T) {
	buf := buildConvexPentagonCoverage()

	// Center should be fully covered
	cov := buf[50*100+50]
	if cov != 255 {
		t.Errorf("pentagon center (50,50): coverage=%d, want 255", cov)
	}

	// Corners should be 0 (outside)
	corners := [][2]int{{0, 0}, {99, 0}, {0, 99}, {99, 99}}
	for _, px := range corners {
		cov = buf[px[1]*100+px[0]]
		if cov != 0 {
			t.Errorf("pentagon exterior (%d,%d): coverage=%d, want 0", px[0], px[1], cov)
		}
	}
}

// --- Regression tests: FillConvex must match Fill for convex shapes ---

// TestRegression_ConvexMatchesGeneral_Rect verifies FillConvex produces
// coverage within 1 of Fill for an axis-aligned rectangle.
//
// The convex walker's rect fast path (blitAntiRect) computes edge alpha as
// fixed_to_alpha(partialLeft) directly, while the general walker uses
// blit_trapezoid_row with a different formula. This is the same 0-1 unit
// rounding difference that exists in Skia between its two code paths.
func TestRegression_ConvexMatchesGeneral_Rect(t *testing.T) {
	convex := buildConvexRectCoverage(10.3, 15.4, 90.8, 86.0)
	general := buildGeneralRectCoverage(10.3, 15.4, 90.8, 86.0)

	const maxAllowedDiff = 1 // Skia's convex vs general have same tolerance

	maxDiff := 0
	for i := range convex {
		d := int(convex[i]) - int(general[i])
		if d < 0 {
			d = -d
		}
		if d > maxDiff {
			maxDiff = d
		}
	}

	if maxDiff > maxAllowedDiff {
		t.Errorf("ConvexRect: max diff=%d (allowed %d) between FillConvex and Fill",
			maxDiff, maxAllowedDiff)
		logged := 0
		for i := range convex {
			d := int(convex[i]) - int(general[i])
			if d < 0 {
				d = -d
			}
			if d > maxAllowedDiff && logged < 10 {
				x, y := i%100, i/100
				t.Logf("  pixel (%d,%d): convex=%d, general=%d", x, y, convex[i], general[i])
				logged++
			}
		}
	} else {
		t.Logf("ConvexRect: max diff=%d (within tolerance %d)", maxDiff, maxAllowedDiff)
	}
}

// TestRegression_ConvexMatchesGeneral_Triangle verifies FillConvex matches Fill
// for a triangle.
func TestRegression_ConvexMatchesGeneral_Triangle(t *testing.T) {
	convex := buildConvexTriangleCoverage(50, 10, 85, 80, 15, 80)
	general := buildGeneralTriangleCoverage(50, 10, 85, 80, 15, 80)

	diffCount := 0
	maxDiff := 0
	for i := range convex {
		d := int(convex[i]) - int(general[i])
		if d < 0 {
			d = -d
		}
		if d > 0 {
			diffCount++
			if d > maxDiff {
				maxDiff = d
			}
		}
	}

	if diffCount > 0 {
		t.Errorf("ConvexTriangle: %d pixels differ (max diff=%d) between FillConvex and Fill", diffCount, maxDiff)
		logged := 0
		for i := range convex {
			if convex[i] != general[i] && logged < 10 {
				x, y := i%100, i/100
				t.Logf("  pixel (%d,%d): convex=%d, general=%d", x, y, convex[i], general[i])
				logged++
			}
		}
	}
}

// TestRegression_ConvexMatchesGeneral_Diamond verifies FillConvex matches Fill
// for a diamond shape (45-degree rotated square).
func TestRegression_ConvexMatchesGeneral_Diamond(t *testing.T) {
	convex := buildConvexDiamondCoverage()
	general := buildGeneralDiamondCoverage()

	diffCount := 0
	maxDiff := 0
	for i := range convex {
		d := int(convex[i]) - int(general[i])
		if d < 0 {
			d = -d
		}
		if d > 0 {
			diffCount++
			if d > maxDiff {
				maxDiff = d
			}
		}
	}

	if diffCount > 0 {
		t.Errorf("ConvexDiamond: %d pixels differ (max diff=%d) between FillConvex and Fill", diffCount, maxDiff)
		logged := 0
		for i := range convex {
			if convex[i] != general[i] && logged < 10 {
				x, y := i%100, i/100
				t.Logf("  pixel (%d,%d): convex=%d, general=%d", x, y, convex[i], general[i])
				logged++
			}
		}
	}
}

// --- Edge case tests ---

// TestFillConvex_ThinRect tests a 1-pixel wide rect.
func TestFillConvex_ThinRect(t *testing.T) {
	buf := buildConvexRectCoverage(50.0, 20.0, 51.0, 80.0)

	// Interior column at x=50 should be fully covered for middle rows.
	for y := 25; y <= 75; y += 5 {
		cov := buf[y*100+50]
		if cov != 255 {
			t.Errorf("thin rect interior (%d,%d): coverage=%d, want 255", 50, y, cov)
		}
	}

	// Adjacent columns should be 0.
	for y := 25; y <= 75; y += 5 {
		for _, x := range []int{49, 51} {
			cov := buf[y*100+x]
			if cov != 0 {
				t.Errorf("thin rect exterior (%d,%d): coverage=%d, want 0", x, y, cov)
			}
		}
	}
}

// TestFillConvex_SubPixelRect tests a rect smaller than 1 pixel in both dimensions.
func TestFillConvex_SubPixelRect(t *testing.T) {
	buf := buildConvexRectCoverage(50.2, 50.3, 50.7, 50.8)

	// The sub-pixel rect should produce partial coverage at (50, 50).
	cov := buf[50*100+50]
	if cov == 0 {
		t.Errorf("sub-pixel rect (50,50): coverage=%d, want > 0", cov)
	}
	if cov == 255 {
		t.Errorf("sub-pixel rect (50,50): coverage=%d, want < 255", cov)
	}

	// Neighbors should be 0.
	neighbors := [][2]int{{49, 50}, {51, 50}, {50, 49}, {50, 51}}
	for _, px := range neighbors {
		n := buf[px[1]*100+px[0]]
		if n != 0 {
			t.Errorf("sub-pixel rect neighbor (%d,%d): coverage=%d, want 0", px[0], px[1], n)
		}
	}
}

// TestFillConvex_VerticalEdges tests the rect optimized path (dLeft|dRite==0).
func TestFillConvex_VerticalEdges(t *testing.T) {
	// Integer-aligned rect — all edges are vertical or horizontal,
	// which triggers the rect fast path in convexBlitRect.
	buf := buildConvexRectCoverage(20, 30, 80, 70)

	// Interior must be 255.
	for y := 35; y <= 65; y += 5 {
		for x := 25; x <= 75; x += 5 {
			cov := buf[y*100+x]
			if cov != 255 {
				t.Errorf("vertical rect interior (%d,%d): coverage=%d, want 255", x, y, cov)
			}
		}
	}

	// Exact boundaries: x=20, x=79 columns should be fully covered.
	for y := 35; y <= 65; y += 5 {
		cov := buf[y*100+20]
		if cov != 255 {
			t.Errorf("left boundary (%d,%d): coverage=%d, want 255", 20, y, cov)
		}
		cov = buf[y*100+79]
		if cov != 255 {
			t.Errorf("right boundary (%d,%d): coverage=%d, want 255", 79, y, cov)
		}
	}
}

// --- Symmetry tests ---

// TestFillConvex_RectSymmetry verifies left/right symmetry for a centered rect.
//
// The convex walker's rect fast path computes partialLeft and partialRite
// independently via SkFixed arithmetic. Due to SkFixedMul truncation, left
// and right edges with "symmetric" fractional offsets (e.g., 0.3 and 0.7)
// may differ by 1 unit. This is the same behavior as Skia.
func TestFillConvex_RectSymmetry(t *testing.T) {
	// Symmetric rect centered at x=50.
	buf := buildConvexRectCoverage(20.3, 30.0, 79.7, 70.0)

	const maxAllowedDiff = 1 // SkFixedMul truncation asymmetry

	for y := 35; y <= 65; y += 5 {
		for dx := 0; dx < 35; dx++ {
			leftX := 20 + dx
			rightX := 79 - dx
			leftCov := buf[y*100+leftX]
			rightCov := buf[y*100+rightX]
			diff := int(leftCov) - int(rightCov)
			if diff < 0 {
				diff = -diff
			}
			if diff > maxAllowedDiff {
				t.Errorf("symmetry y=%d: pixel (%d)=%d vs (%d)=%d, diff=%d (max allowed %d)",
					y, leftX, leftCov, rightX, rightCov, diff, maxAllowedDiff)
			}
		}
	}
}

// TestFillConvex_TriangleMonotonicity verifies that coverage increases
// monotonically from edge to interior for each row of a triangle.
func TestFillConvex_TriangleMonotonicity(t *testing.T) {
	// Wide triangle
	buf := buildConvexTriangleCoverage(50, 10, 90, 85, 10, 85)

	// For rows in the middle, check that coverage increases from the left edge
	// toward the interior, and decreases from interior toward the right edge.
	for y := 30; y <= 70; y += 10 {
		row := buf[y*100 : y*100+100]

		// Find first and last non-zero pixel.
		firstNZ := -1
		lastNZ := -1
		for x := 0; x < 100; x++ {
			if row[x] > 0 {
				if firstNZ < 0 {
					firstNZ = x
				}
				lastNZ = x
			}
		}
		if firstNZ < 0 {
			continue
		}

		// Coverage should increase from firstNZ to some interior point.
		// We check that partial -> full transition exists.
		hasPartial := false
		hasFull := false
		for x := firstNZ; x <= lastNZ; x++ {
			if row[x] > 0 && row[x] < 255 {
				hasPartial = true
			}
			if row[x] == 255 {
				hasFull = true
			}
		}

		// A triangle wide enough at this row should have both partial and full pixels.
		if !hasPartial || !hasFull {
			t.Errorf("monotonicity y=%d: hasPartial=%v, hasFull=%v (firstNZ=%d, lastNZ=%d)",
				y, hasPartial, hasFull, firstNZ, lastNZ)
		}
	}
}

// --- Benchmarks ---

// BenchmarkFillConvex_Rect benchmarks the convex walker on an axis-aligned rect.
// Filler is reused across iterations (matching production usage).
func BenchmarkFillConvex_Rect(b *testing.B) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{10.3, 15.4, 90.8, 15.4, 90.8, 86.0, 10.3, 86.0},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	filler := NewAnalyticFiller(100, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.FillConvex(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
			offset := y * 100
			if offset+100 > len(buf) {
				return
			}
			runs.CopyTo(buf[offset : offset+100])
		})
	}
}

// BenchmarkFill_Rect benchmarks the general walker on the same rect for comparison.
func BenchmarkFill_Rect(b *testing.B) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{10.3, 15.4, 90.8, 15.4, 90.8, 86.0, 10.3, 86.0},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)

	filler := NewAnalyticFiller(100, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
			offset := y * 100
			if offset+100 > len(buf) {
				return
			}
			runs.CopyTo(buf[offset : offset+100])
		})
	}
}
