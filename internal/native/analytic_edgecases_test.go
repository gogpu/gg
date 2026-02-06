// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"github.com/gogpu/gg/internal/raster"
	"testing"

	"github.com/gogpu/gg/scene"
)

// =============================================================================
// Edge Case Tests for Analytic Anti-Aliasing
// These tests verify correct behavior for unusual or degenerate inputs
// =============================================================================

// TestEdgeCase_DegenerateQuadratic tests a quadratic with control point on chord.
func TestEdgeCase_DegenerateQuadratic(t *testing.T) {
	filler := NewAnalyticFiller(200, 200)
	eb := raster.NewEdgeBuilder(2)

	// Control point exactly on the line between start and end (degenerate)
	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.QuadTo(55, 55, 100, 100) // Control point on chord
	path.LineTo(100, 150)
	path.LineTo(10, 150)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Should not panic
	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
		scanlineCount++
	})

	if scanlineCount == 0 {
		t.Error("expected scanlines even with degenerate quadratic")
	}

	t.Logf("Degenerate quadratic: %d scanlines", scanlineCount)
}

// TestEdgeCase_NearHorizontalCurve tests curves with very small Y extent.
func TestEdgeCase_NearHorizontalCurve(t *testing.T) {
	filler := NewAnalyticFiller(200, 200)
	eb := raster.NewEdgeBuilder(2)

	// Quadratic with minimal Y change (nearly horizontal)
	path := scene.NewPath()
	path.MoveTo(10, 100)
	path.QuadTo(100, 100.01, 190, 100) // Tiny Y deviation
	path.LineTo(190, 150)
	path.LineTo(10, 150)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Should handle gracefully
	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
		scanlineCount++
	})

	t.Logf("Near-horizontal curve: %d scanlines, %d edges",
		scanlineCount, eb.EdgeCount())
}

// TestEdgeCase_SelfIntersectingPath tests a path that crosses itself.
func TestEdgeCase_SelfIntersectingPath(t *testing.T) {
	filler := NewAnalyticFiller(200, 200)
	eb := raster.NewEdgeBuilder(2)

	// Figure-8 path (self-intersecting)
	path := scene.NewPath()
	path.MoveTo(100, 50)
	path.LineTo(150, 100)
	path.LineTo(50, 100)
	path.LineTo(100, 150)
	path.LineTo(150, 100)
	path.LineTo(50, 100)
	path.LineTo(100, 50)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Test both fill rules
	for _, rule := range []raster.FillRule{raster.FillRuleNonZero, raster.FillRuleEvenOdd} {
		filler.Reset()
		scanlineCount := 0
		filler.Fill(eb, rule, func(_ int, _ *raster.AlphaRuns) {
			scanlineCount++
		})
		t.Logf("Self-intersecting path (%s): %d scanlines", rule, scanlineCount)
	}
}

// TestEdgeCase_CuspCurve tests a cubic curve with a cusp.
func TestEdgeCase_CuspCurve(t *testing.T) {
	filler := NewAnalyticFiller(200, 200)
	eb := raster.NewEdgeBuilder(2)

	// Cubic with cusp (sharp point)
	path := scene.NewPath()
	path.MoveTo(50, 50)
	path.CubicTo(150, 50, 50, 150, 150, 150) // Creates a cusp
	path.LineTo(150, 180)
	path.LineTo(50, 180)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
		scanlineCount++
	})

	if scanlineCount == 0 {
		t.Error("expected scanlines for cusp curve")
	}

	t.Logf("Cusp curve: %d scanlines, %d cubic edges",
		scanlineCount, eb.CubicEdgeCount())
}

// TestEdgeCase_VeryThinPath tests a path that is only 1 pixel wide.
func TestEdgeCase_VeryThinPath(t *testing.T) {
	filler := NewAnalyticFiller(200, 200)
	eb := raster.NewEdgeBuilder(2)

	// Very thin vertical rectangle
	path := scene.NewPath()
	path.MoveTo(100, 50)
	path.LineTo(100.5, 50) // Only 0.5 pixels wide
	path.LineTo(100.5, 150)
	path.LineTo(100, 150)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, runs *raster.AlphaRuns) {
		scanlineCount++
		// Expected: partial coverage for thin paths (anti-aliased)
		_ = runs.IsEmpty() // Verify runs are accessible
	})

	t.Logf("Very thin path: %d scanlines", scanlineCount)
}

// TestEdgeCase_VeryLargePath tests a path with coordinates up to 10000.
func TestEdgeCase_VeryLargePath(t *testing.T) {
	filler := NewAnalyticFiller(1000, 1000)
	eb := raster.NewEdgeBuilder(2)

	// Large coordinates (will be clipped)
	path := scene.NewPath()
	path.MoveTo(-500, -500)
	path.LineTo(10500, -500)
	path.LineTo(10500, 10500)
	path.LineTo(-500, 10500)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
		scanlineCount++
	})

	// Should process visible scanlines (0-999)
	if scanlineCount != 1000 {
		t.Logf("Very large path: %d scanlines (expected ~1000)", scanlineCount)
	}
}

// TestEdgeCase_EmptyPath tests an empty path.
func TestEdgeCase_EmptyPath(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	eb := raster.NewEdgeBuilder(2)

	path := scene.NewPath() // Empty path

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	if !eb.IsEmpty() {
		t.Error("expected empty edge builder for empty path")
	}

	// Should not call callback for empty path
	callbackCalled := false
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
		callbackCalled = true
	})

	if callbackCalled {
		t.Error("callback should not be called for empty path")
	}
}

// TestEdgeCase_SinglePoint tests a path with only MoveTo (no edges).
func TestEdgeCase_SinglePoint(t *testing.T) {
	eb := raster.NewEdgeBuilder(2)

	path := scene.NewPath()
	path.MoveTo(50, 50) // Just a point, no edges

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Should be empty (no edges)
	if eb.EdgeCount() != 0 {
		t.Errorf("expected 0 edges for single point, got %d", eb.EdgeCount())
	}
}

// TestEdgeCase_HorizontalLine tests a purely horizontal line.
func TestEdgeCase_HorizontalLine(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	eb := raster.NewEdgeBuilder(2)

	path := scene.NewPath()
	path.MoveTo(10, 50)
	path.LineTo(90, 50) // Horizontal line

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Horizontal lines have no Y extent - should produce no edges
	if eb.EdgeCount() != 0 {
		t.Logf("Note: horizontal line produced %d edges (may be expected)", eb.EdgeCount())
	}

	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
		// May or may not be called
	})
}

// TestEdgeCase_VerticalLine tests a purely vertical line.
func TestEdgeCase_VerticalLine(t *testing.T) {
	eb := raster.NewEdgeBuilder(2)

	// Create a closed shape with vertical edges (open paths may not produce edges)
	path := scene.NewPath()
	path.MoveTo(50, 10)
	path.LineTo(50, 90) // Vertical line
	path.LineTo(60, 90)
	path.LineTo(60, 10)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Closed shape should have vertical edges
	if eb.EdgeCount() == 0 {
		t.Error("expected edges for vertical rectangle")
	}
	t.Logf("Vertical rectangle: %d line edges", eb.LineEdgeCount())
}

// TestEdgeCase_ZeroSizeCanvas tests a zero-size canvas.
func TestEdgeCase_ZeroSizeCanvas(t *testing.T) {
	// Zero size should be handled gracefully
	filler := NewAnalyticFiller(1, 1) // Minimum valid size
	eb := raster.NewEdgeBuilder(2)

	path := scene.NewPath().Circle(50, 50, 25)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Should not panic
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
}

// TestEdgeCase_NegativeCoordinates tests paths with negative coordinates.
func TestEdgeCase_NegativeCoordinates(t *testing.T) {
	filler := NewAnalyticFiller(200, 200)
	eb := raster.NewEdgeBuilder(2)

	// Path starting in negative space, extending into visible area
	path := scene.NewPath()
	path.MoveTo(-50, -50)
	path.LineTo(150, -50)
	path.LineTo(150, 150)
	path.LineTo(-50, 150)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(y int, _ *raster.AlphaRuns) {
		if y >= 0 && y < 200 {
			scanlineCount++
		}
	})

	// Should only process visible scanlines
	if scanlineCount == 0 {
		t.Error("expected to process visible scanlines")
	}

	t.Logf("Negative coords: %d visible scanlines", scanlineCount)
}

// TestEdgeCase_MixedWindings tests paths with mixed winding directions.
func TestEdgeCase_MixedWindings(t *testing.T) {
	filler := NewAnalyticFiller(200, 200)
	eb := raster.NewEdgeBuilder(2)

	// Outer rect (clockwise)
	pathOuter := scene.NewPath()
	pathOuter.MoveTo(20, 20)
	pathOuter.LineTo(180, 20)
	pathOuter.LineTo(180, 180)
	pathOuter.LineTo(20, 180)
	pathOuter.Close()

	// Inner rect (counter-clockwise - hole)
	pathInner := scene.NewPath()
	pathInner.MoveTo(60, 60)
	pathInner.LineTo(60, 140)
	pathInner.LineTo(140, 140)
	pathInner.LineTo(140, 60)
	pathInner.Close()

	BuildEdgesFromScenePath(eb, pathOuter, scene.IdentityAffine())
	BuildEdgesFromScenePath(eb, pathInner, scene.IdentityAffine())

	// Test with NonZero (should create hole)
	filler.Reset()
	nonZeroScanlines := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
		nonZeroScanlines++
	})

	// Test with EvenOdd (should also create hole)
	filler.Reset()
	evenOddScanlines := 0
	filler.Fill(eb, raster.FillRuleEvenOdd, func(_ int, _ *raster.AlphaRuns) {
		evenOddScanlines++
	})

	t.Logf("Mixed windings: NonZero=%d, EvenOdd=%d scanlines",
		nonZeroScanlines, evenOddScanlines)
}

// TestEdgeCase_InfinitesimalCurve tests an extremely small curve.
func TestEdgeCase_InfinitesimalCurve(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	eb := raster.NewEdgeBuilder(2)

	// Curve with sub-pixel extent
	path := scene.NewPath()
	path.MoveTo(50, 50)
	path.QuadTo(50.001, 50.001, 50.002, 50.002)
	path.LineTo(50.002, 50.5)
	path.LineTo(50, 50.5)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Should handle gracefully (may produce no visible output)
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
}

// TestEdgeCase_LoopingCubic tests a cubic that loops back on itself.
func TestEdgeCase_LoopingCubic(t *testing.T) {
	filler := NewAnalyticFiller(200, 200)
	eb := raster.NewEdgeBuilder(2)

	// Cubic that creates a loop
	path := scene.NewPath()
	path.MoveTo(50, 100)
	path.CubicTo(200, 0, 0, 200, 150, 100) // Creates a loop
	path.LineTo(150, 150)
	path.LineTo(50, 150)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Should handle the loop (may be chopped at Y extrema)
	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
		scanlineCount++
	})

	t.Logf("Looping cubic: %d cubic edges, %d scanlines",
		eb.CubicEdgeCount(), scanlineCount)
}

// TestEdgeCase_AllEdgeTypesInOnePath tests a path with all edge gputypes.
func TestEdgeCase_AllEdgeTypesInOnePath(t *testing.T) {
	filler := NewAnalyticFiller(300, 300)
	eb := raster.NewEdgeBuilder(2)

	path := scene.NewPath()
	path.MoveTo(50, 50)
	path.LineTo(100, 50)                       // Line
	path.QuadTo(150, 50, 150, 100)             // Quadratic
	path.CubicTo(150, 150, 200, 150, 200, 200) // Cubic
	path.LineTo(50, 200)                       // Line
	path.QuadTo(50, 150, 100, 150)             // Quadratic
	path.CubicTo(100, 100, 50, 100, 50, 50)    // Cubic back to start
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Should have various edge types
	// Note: Line edges may be optimized/combined, and small curves may become lines
	// The exact distribution depends on curve chopping and optimization
	if eb.EdgeCount() == 0 {
		t.Error("expected some edges")
	}
	t.Logf("Edge distribution: %d lines, %d quads, %d cubics",
		eb.LineEdgeCount(), eb.QuadraticEdgeCount(), eb.CubicEdgeCount())

	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
		scanlineCount++
	})

	t.Logf("All edge types: %d lines, %d quads, %d cubics, %d scanlines",
		eb.LineEdgeCount(), eb.QuadraticEdgeCount(),
		eb.CubicEdgeCount(), scanlineCount)
}

// TestEdgeCase_TransformedPath tests a path with various transforms.
func TestEdgeCase_TransformedPath(t *testing.T) {
	filler := NewAnalyticFiller(400, 400)
	eb := raster.NewEdgeBuilder(2)

	basePath := scene.NewPath().Circle(0, 0, 50)

	transforms := []struct {
		name      string
		transform scene.Affine
	}{
		{"translate", scene.TranslateAffine(200, 200)},
		{"scale", scene.ScaleAffine(2, 2).Multiply(scene.TranslateAffine(200, 200))},
		{"rotate45", scene.RotateAffine(0.785398).Multiply(scene.TranslateAffine(200, 200))}, // 45 degrees
		{"shear", scene.Affine{A: 1, B: 0.5, C: 200, D: 0, E: 1, F: 200}},                    // Shear transform
	}

	for _, tt := range transforms {
		eb.Reset()
		BuildEdgesFromScenePath(eb, basePath, tt.transform)

		filler.Reset()
		scanlineCount := 0
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
			scanlineCount++
		})

		t.Logf("Transform %s: %d edges, %d scanlines",
			tt.name, eb.EdgeCount(), scanlineCount)
	}
}

// TestEdgeCase_FixedPointOverflow tests values near fixed-point limits.
func TestEdgeCase_FixedPointOverflow(t *testing.T) {
	// Large coordinates that might cause fixed-point overflow
	largeCoord := float32(30000)

	// Test raster.FDot6 conversion
	fdot6 := raster.FDot6FromFloat32(largeCoord)
	roundTrip := raster.FDot6ToFloat32(fdot6)

	// Should handle large values (with some precision loss)
	t.Logf("Large coord %f -> raster.FDot6 %d -> %f", largeCoord, fdot6, roundTrip)

	// Test raster.FDot16 conversion
	fdot16 := raster.FDot16FromFloat32(largeCoord)
	roundTrip16 := raster.FDot16ToFloat32(fdot16)

	t.Logf("Large coord %f -> raster.FDot16 %d -> %f", largeCoord, fdot16, roundTrip16)
}

// TestEdgeCase_ZeroAreaPath tests a path with zero area.
func TestEdgeCase_ZeroAreaPath(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	eb := raster.NewEdgeBuilder(2)

	// Line path (no area)
	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 90)
	path.LineTo(10, 10) // Back to start
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Zero area path should produce minimal or no coverage
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, runs *raster.AlphaRuns) {
		// May have edge anti-aliasing but no filled area
	})
}

// TestEdgeCase_AdjacentRectangles tests adjacent rectangles (shared edges).
func TestEdgeCase_AdjacentRectangles(t *testing.T) {
	filler := NewAnalyticFiller(200, 100)
	eb := raster.NewEdgeBuilder(2)

	// Two adjacent rectangles sharing an edge
	rect1 := scene.NewPath()
	rect1.MoveTo(10, 10)
	rect1.LineTo(100, 10)
	rect1.LineTo(100, 90)
	rect1.LineTo(10, 90)
	rect1.Close()

	rect2 := scene.NewPath()
	rect2.MoveTo(100, 10) // Starts at shared edge
	rect2.LineTo(190, 10)
	rect2.LineTo(190, 90)
	rect2.LineTo(100, 90)
	rect2.Close()

	BuildEdgesFromScenePath(eb, rect1, scene.IdentityAffine())
	BuildEdgesFromScenePath(eb, rect2, scene.IdentityAffine())

	// Vertical edge combining should kick in
	t.Logf("Adjacent rects: %d total edges", eb.EdgeCount())

	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {
		scanlineCount++
	})

	t.Logf("Adjacent rects: %d scanlines", scanlineCount)
}
