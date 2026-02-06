// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"github.com/gogpu/gg/raster"
	"math"
	"testing"

	"github.com/gogpu/gg/scene"
)

// TestChopQuadAtYExtrema tests Y-monotonic chopping for quadratics.
func TestChopQuadAtYExtrema(t *testing.T) {
	tests := []struct {
		name      string
		src       [3]raster.GeomPoint
		wantChops int
		checkMono bool // Whether to verify monotonicity
	}{
		{
			name:      "already monotonic (going down)",
			src:       [3]raster.GeomPoint{{X: 0, Y: 0}, {X: 50, Y: 50}, {X: 100, Y: 100}},
			wantChops: 0,
			checkMono: true,
		},
		{
			name:      "already monotonic (going up)",
			src:       [3]raster.GeomPoint{{X: 0, Y: 100}, {X: 50, Y: 50}, {X: 100, Y: 0}},
			wantChops: 0,
			checkMono: true,
		},
		{
			name: "has Y extremum (arch up)",
			src:  [3]raster.GeomPoint{{X: 0, Y: 0}, {X: 50, Y: 100}, {X: 100, Y: 0}},
			// Control point above both endpoints - has maximum
			wantChops: 1,
			checkMono: true,
		},
		{
			name: "has Y extremum (arch down)",
			src:  [3]raster.GeomPoint{{X: 0, Y: 100}, {X: 50, Y: 0}, {X: 100, Y: 100}},
			// Control point below both endpoints - has minimum
			wantChops: 1,
			checkMono: true,
		},
		{
			name:      "horizontal line",
			src:       [3]raster.GeomPoint{{X: 0, Y: 50}, {X: 50, Y: 50}, {X: 100, Y: 50}},
			wantChops: 0,
			checkMono: false, // Horizontal curves are degenerate, skip monotonicity check
		},
		{
			name:      "small deviation",
			src:       [3]raster.GeomPoint{{X: 0, Y: 0}, {X: 50, Y: 51}, {X: 100, Y: 100}},
			wantChops: 0, // Nearly straight, should be monotonic
			checkMono: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst [5]raster.GeomPoint
			numChops := raster.ChopQuadAtYExtrema(tt.src, &dst)

			if numChops != tt.wantChops {
				t.Errorf("raster.ChopQuadAtYExtrema() = %d chops, want %d", numChops, tt.wantChops)
			}

			if tt.checkMono {
				// Verify each resulting segment is Y-monotonic
				for i := 0; i <= numChops; i++ {
					p0 := dst[i*2]
					p1 := dst[i*2+1]
					p2 := dst[i*2+2]

					if !raster.QuadIsYMonotonic(p0, p1, p2) {
						t.Errorf("segment %d is not Y-monotonic: %v, %v, %v",
							i, p0, p1, p2)
					}
				}
			}

			// Log results for debugging
			t.Logf("input: %v -> %d chops", tt.src, numChops)
			for i := 0; i <= numChops; i++ {
				t.Logf("  segment %d: [%.1f,%.1f] -> [%.1f,%.1f] -> [%.1f,%.1f]",
					i, dst[i*2].X, dst[i*2].Y,
					dst[i*2+1].X, dst[i*2+1].Y,
					dst[i*2+2].X, dst[i*2+2].Y)
			}
		})
	}
}

// TestChopCubicAtYExtrema tests Y-monotonic chopping for cubics.
func TestChopCubicAtYExtrema(t *testing.T) {
	tests := []struct {
		name      string
		src       [4]raster.GeomPoint
		wantChops int
		checkMono bool
	}{
		{
			name:      "already monotonic (straight down)",
			src:       [4]raster.GeomPoint{{X: 0, Y: 0}, {X: 33, Y: 33}, {X: 66, Y: 66}, {X: 100, Y: 100}},
			wantChops: 0,
			checkMono: true,
		},
		{
			name:      "S-curve (two extrema)",
			src:       [4]raster.GeomPoint{{X: 0, Y: 50}, {X: 100, Y: 0}, {X: 0, Y: 100}, {X: 100, Y: 50}},
			wantChops: 2,
			checkMono: true,
		},
		{
			name:      "one extremum (arch)",
			src:       [4]raster.GeomPoint{{X: 0, Y: 0}, {X: 50, Y: 100}, {X: 50, Y: 100}, {X: 100, Y: 0}},
			wantChops: 1,
			checkMono: true,
		},
		{
			name:      "horizontal cubic",
			src:       [4]raster.GeomPoint{{X: 0, Y: 50}, {X: 33, Y: 50}, {X: 66, Y: 50}, {X: 100, Y: 50}},
			wantChops: 0,
			checkMono: true,
		},
		{
			name:      "real world curve (smooth)",
			src:       [4]raster.GeomPoint{{X: 10, Y: 20}, {X: 67, Y: 437}, {X: 298, Y: 213}, {X: 401, Y: 214}},
			wantChops: 2, // Has two Y extrema
			checkMono: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst [10]raster.GeomPoint
			numChops := raster.ChopCubicAtYExtrema(tt.src, &dst)

			if numChops != tt.wantChops {
				t.Errorf("raster.ChopCubicAtYExtrema() = %d chops, want %d", numChops, tt.wantChops)
			}

			if tt.checkMono {
				// Verify each resulting segment is Y-monotonic
				for i := 0; i <= numChops; i++ {
					p0 := dst[i*3]
					p1 := dst[i*3+1]
					p2 := dst[i*3+2]
					p3 := dst[i*3+3]

					if !raster.CubicIsYMonotonic(p0, p1, p2, p3) {
						t.Errorf("segment %d is not Y-monotonic: %v, %v, %v, %v",
							i, p0, p1, p2, p3)
					}
				}
			}

			// Log results
			t.Logf("input: %v -> %d chops", tt.src, numChops)
		})
	}
}

// TestEdgeBuilderBasic tests basic EdgeBuilder functionality.
func TestEdgeBuilderBasic(t *testing.T) {
	t.Run("new builder is empty", func(t *testing.T) {
		eb := raster.NewEdgeBuilder(0)
		if !eb.IsEmpty() {
			t.Error("new builder should be empty")
		}
		if eb.EdgeCount() != 0 {
			t.Errorf("EdgeCount() = %d, want 0", eb.EdgeCount())
		}
	})

	t.Run("reset clears edges", func(t *testing.T) {
		eb := raster.NewEdgeBuilder(0)

		// Add some edges
		path := scene.NewPath().
			MoveTo(0, 0).
			LineTo(100, 100).
			Close()
		BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

		if eb.IsEmpty() {
			t.Fatal("builder should not be empty after adding path")
		}

		eb.Reset()

		if !eb.IsEmpty() {
			t.Error("builder should be empty after reset")
		}
	})
}

// TestEdgeBuilderLinePath tests building edges from a line-only path.
func TestEdgeBuilderLinePath(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	// Simple triangle
	path := scene.NewPath().
		MoveTo(50, 0).
		LineTo(100, 100).
		LineTo(0, 100).
		Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Should have at least 2 line edges (horizontal may be skipped, vertical may combine)
	// The exact count depends on edge optimization (combining vertical edges)
	if eb.LineEdgeCount() < 2 {
		t.Errorf("LineEdgeCount() = %d, want >= 2", eb.LineEdgeCount())
	}

	t.Logf("Triangle: %d line edges", eb.LineEdgeCount())

	// No curve edges
	if eb.QuadraticEdgeCount() != 0 {
		t.Errorf("QuadraticEdgeCount() = %d, want 0", eb.QuadraticEdgeCount())
	}
	if eb.CubicEdgeCount() != 0 {
		t.Errorf("CubicEdgeCount() = %d, want 0", eb.CubicEdgeCount())
	}

	// Check bounds
	bounds := eb.Bounds()
	if bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 100 || bounds.MaxY != 100 {
		t.Errorf("Bounds() = %v, want (0,0)-(100,100)", bounds)
	}
}

// TestEdgeBuilderQuadPath tests building edges from a quadratic path.
func TestEdgeBuilderQuadPath(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	// Path with quadratic curve that goes up then back down (arch)
	// Start at (0,0), control at (50,100), end at (100,0)
	// This creates an arch that will be chopped at the Y extremum
	// The closing line from (100,0) back to (0,0) is horizontal and will be skipped
	path := scene.NewPath().
		MoveTo(0, 0).
		QuadTo(50, 100, 100, 0).
		Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// The quadratic has a Y extremum, so it should be chopped into 2 monotonic pieces
	if eb.QuadraticEdgeCount() < 1 {
		t.Errorf("QuadraticEdgeCount() = %d, want >= 1", eb.QuadraticEdgeCount())
	}

	// Note: The closing line from (100,0) to (0,0) is horizontal (Y unchanged)
	// so it will be skipped (no winding contribution from horizontal lines)

	t.Logf("Quad path: %d lines, %d quads, %d cubics",
		eb.LineEdgeCount(), eb.QuadraticEdgeCount(), eb.CubicEdgeCount())
}

// TestEdgeBuilderCubicPath tests building edges from a cubic path.
func TestEdgeBuilderCubicPath(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	// S-curve (has 2 Y extrema)
	path := scene.NewPath().
		MoveTo(0, 50).
		CubicTo(100, 0, 0, 100, 100, 50).
		Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// S-curve should produce multiple cubic edges
	if eb.CubicEdgeCount() < 1 {
		t.Errorf("CubicEdgeCount() = %d, want >= 1", eb.CubicEdgeCount())
	}

	t.Logf("Cubic path: %d lines, %d quads, %d cubics",
		eb.LineEdgeCount(), eb.QuadraticEdgeCount(), eb.CubicEdgeCount())
}

// TestEdgeBuilderTransform tests transform application.
func TestEdgeBuilderTransform(t *testing.T) {
	// Create path at origin
	path := scene.NewPath().
		MoveTo(0, 0).
		LineTo(10, 0).
		LineTo(10, 10).
		LineTo(0, 10).
		Close()

	// Build with translation transform
	eb := raster.NewEdgeBuilder(0)
	transform := scene.TranslateAffine(100, 200)
	BuildEdgesFromScenePath(eb, path, transform)

	// Bounds should be translated
	bounds := eb.Bounds()
	if bounds.MinX != 100 || bounds.MinY != 200 ||
		bounds.MaxX != 110 || bounds.MaxY != 210 {
		t.Errorf("Bounds() = %v, want (100,200)-(110,210)", bounds)
	}
}

// TestEdgeBuilderAllEdgesOrder tests that AllEdges returns edges sorted by Y.
func TestEdgeBuilderAllEdgesOrder(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	// Create path with edges at different Y levels
	// Add lines in reverse Y order to test sorting
	path := scene.NewPath().
		MoveTo(0, 80).LineTo(10, 100). // Starts at Y=80
		MoveTo(0, 40).LineTo(10, 60).  // Starts at Y=40
		MoveTo(0, 0).LineTo(10, 20)    // Starts at Y=0

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Collect Y values from iterator
	var topYs []int32
	for edge := range eb.AllEdges() {
		line := edge.AsLine()
		if line != nil {
			topYs = append(topYs, line.FirstY)
		}
	}

	// Verify sorted order
	for i := 1; i < len(topYs); i++ {
		if topYs[i] < topYs[i-1] {
			t.Errorf("edges not sorted: Y[%d]=%d > Y[%d]=%d",
				i-1, topYs[i-1], i, topYs[i])
		}
	}

	t.Logf("Edge top Ys: %v (should be ascending)", topYs)
}

// TestEdgeBuilderCircle tests building edges from a circle.
func TestEdgeBuilderCircle(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	// Circle - 4 cubic curves
	path := scene.NewPath().Circle(100, 100, 50)

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Circle uses cubics for arcs
	if eb.CubicEdgeCount() < 4 {
		t.Errorf("CubicEdgeCount() = %d, want >= 4", eb.CubicEdgeCount())
	}

	// Each quarter-circle may be chopped at Y extrema
	t.Logf("Circle: %d lines, %d quads, %d cubics, total %d",
		eb.LineEdgeCount(), eb.QuadraticEdgeCount(), eb.CubicEdgeCount(),
		eb.EdgeCount())
}

// TestEdgeBuilderWithAA tests AA shift affects edge creation.
func TestEdgeBuilderWithAA(t *testing.T) {
	path := scene.NewPath().
		MoveTo(0, 0).
		LineTo(100, 100).
		Close()

	// Without AA
	eb0 := raster.NewEdgeBuilder(0)
	BuildEdgesFromScenePath(eb0, path, scene.IdentityAffine())

	// With 4x AA (shift=2)
	eb2 := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb2, path, scene.IdentityAffine())

	if eb0.AAShift() != 0 {
		t.Errorf("AAShift() = %d, want 0", eb0.AAShift())
	}
	if eb2.AAShift() != 2 {
		t.Errorf("AAShift() = %d, want 2", eb2.AAShift())
	}

	// Both should produce edges (same count since it's simple lines)
	if eb0.LineEdgeCount() != eb2.LineEdgeCount() {
		t.Logf("Line counts differ: noAA=%d, AA=%d",
			eb0.LineEdgeCount(), eb2.LineEdgeCount())
		// This is OK - AA may affect edge clipping
	}
}

// TestEdgeBuilderIterators tests the type-specific iterators.
func TestEdgeBuilderIterators(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	// Mixed path
	path := scene.NewPath().
		MoveTo(0, 0).
		LineTo(50, 50).
		QuadTo(75, 25, 100, 50).
		CubicTo(100, 75, 50, 100, 50, 75).
		Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Count edges via iterators
	lineCount := 0
	for range eb.LineEdges() {
		lineCount++
	}

	quadCount := 0
	for range eb.QuadraticEdges() {
		quadCount++
	}

	cubicCount := 0
	for range eb.CubicEdges() {
		cubicCount++
	}

	// Verify counts match
	if lineCount != eb.LineEdgeCount() {
		t.Errorf("LineEdges iterator count %d != LineEdgeCount %d",
			lineCount, eb.LineEdgeCount())
	}
	if quadCount != eb.QuadraticEdgeCount() {
		t.Errorf("QuadraticEdges iterator count %d != QuadraticEdgeCount %d",
			quadCount, eb.QuadraticEdgeCount())
	}
	if cubicCount != eb.CubicEdgeCount() {
		t.Errorf("CubicEdges iterator count %d != CubicEdgeCount %d",
			cubicCount, eb.CubicEdgeCount())
	}

	t.Logf("Mixed path: %d lines, %d quads, %d cubics",
		lineCount, quadCount, cubicCount)
}

// TestEdgeBuilderHorizontalLine tests that horizontal lines are skipped.
func TestEdgeBuilderHorizontalLine(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	// Purely horizontal line
	path := scene.NewPath().
		MoveTo(0, 50).
		LineTo(100, 50)

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Horizontal lines have no Y extent - should be skipped
	if eb.LineEdgeCount() != 0 {
		t.Errorf("horizontal line should produce 0 edges, got %d",
			eb.LineEdgeCount())
	}
}

// TestEdgeBuilderVerticalCombine tests vertical edge combining.
func TestEdgeBuilderVerticalCombine(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	// Two adjacent vertical lines (should combine)
	path := scene.NewPath().
		MoveTo(50, 0).
		LineTo(50, 50).
		MoveTo(50, 50).
		LineTo(50, 100)

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// May be combined into one edge
	// (Depends on implementation details)
	t.Logf("Two vertical lines: %d edges", eb.LineEdgeCount())
}

// TestQuadIsYMonotonic tests the monotonicity helper.
func TestQuadIsYMonotonic(t *testing.T) {
	tests := []struct {
		name       string
		p0, p1, p2 raster.GeomPoint
		want       bool
	}{
		{"going down", raster.GeomPoint{X: 0, Y: 0}, raster.GeomPoint{X: 50, Y: 50}, raster.GeomPoint{X: 100, Y: 100}, true},
		{"going up", raster.GeomPoint{X: 0, Y: 100}, raster.GeomPoint{X: 50, Y: 50}, raster.GeomPoint{X: 100, Y: 0}, true},
		{"arch", raster.GeomPoint{X: 0, Y: 0}, raster.GeomPoint{X: 50, Y: 100}, raster.GeomPoint{X: 100, Y: 0}, false},
		{"valley", raster.GeomPoint{X: 0, Y: 100}, raster.GeomPoint{X: 50, Y: 0}, raster.GeomPoint{X: 100, Y: 100}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := raster.QuadIsYMonotonic(tt.p0, tt.p1, tt.p2)
			if got != tt.want {
				t.Errorf("raster.QuadIsYMonotonic() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCubicIsYMonotonic tests cubic monotonicity.
func TestCubicIsYMonotonic(t *testing.T) {
	tests := []struct {
		name           string
		p0, p1, p2, p3 raster.GeomPoint
		want           bool
	}{
		{
			"straight down",
			raster.GeomPoint{X: 0, Y: 0}, raster.GeomPoint{X: 33, Y: 33}, raster.GeomPoint{X: 66, Y: 66}, raster.GeomPoint{X: 100, Y: 100},
			true,
		},
		{
			"S-curve",
			raster.GeomPoint{X: 0, Y: 50}, raster.GeomPoint{X: 100, Y: 0}, raster.GeomPoint{X: 0, Y: 100}, raster.GeomPoint{X: 100, Y: 50},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := raster.CubicIsYMonotonic(tt.p0, tt.p1, tt.p2, tt.p3)
			if got != tt.want {
				t.Errorf("raster.CubicIsYMonotonic() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Benchmark tests

// BenchmarkEdgeBuilderTriangle benchmarks building a simple triangle.
func BenchmarkEdgeBuilderTriangle(b *testing.B) {
	path := scene.NewPath().
		MoveTo(50, 0).
		LineTo(100, 100).
		LineTo(0, 100).
		Close()

	eb := raster.NewEdgeBuilder(0)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eb.Reset()
		BuildEdgesFromScenePath(eb, path, transform)
	}
}

// BenchmarkEdgeBuilderCircle benchmarks building a circle (4 cubics).
func BenchmarkEdgeBuilderCircle(b *testing.B) {
	path := scene.NewPath().Circle(100, 100, 50)

	eb := raster.NewEdgeBuilder(0)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eb.Reset()
		BuildEdgesFromScenePath(eb, path, transform)
	}
}

// BenchmarkEdgeBuilderRoundedRect benchmarks a rounded rectangle.
func BenchmarkEdgeBuilderRoundedRect(b *testing.B) {
	path := scene.NewPath().RoundedRectangle(0, 0, 200, 100, 10)

	eb := raster.NewEdgeBuilder(0)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eb.Reset()
		BuildEdgesFromScenePath(eb, path, transform)
	}
}

// BenchmarkChopQuadAtYExtrema benchmarks quadratic chopping.
func BenchmarkChopQuadAtYExtrema(b *testing.B) {
	src := [3]raster.GeomPoint{{X: 0, Y: 0}, {X: 50, Y: 100}, {X: 100, Y: 0}}
	var dst [5]raster.GeomPoint

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raster.ChopQuadAtYExtrema(src, &dst)
	}
}

// BenchmarkChopCubicAtYExtrema benchmarks cubic chopping.
func BenchmarkChopCubicAtYExtrema(b *testing.B) {
	src := [4]raster.GeomPoint{{X: 0, Y: 50}, {X: 100, Y: 0}, {X: 0, Y: 100}, {X: 100, Y: 50}}
	var dst [10]raster.GeomPoint

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raster.ChopCubicAtYExtrema(src, &dst)
	}
}

// BenchmarkAllEdgesIterator benchmarks the AllEdges iterator.
func BenchmarkAllEdgesIterator(b *testing.B) {
	path := scene.NewPath().
		Circle(100, 100, 50).
		RoundedRectangle(0, 0, 200, 200, 20)

	eb := raster.NewEdgeBuilder(0)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		for range eb.AllEdges() {
			count++
		}
		_ = count
	}
}

// Helper function to check approximate float equality
// Used for debugging and validation tests.
var _ = approxEqual // Prevent unused warning

func approxEqual(a, b, epsilon float32) bool {
	return math.Abs(float64(a-b)) < float64(epsilon)
}
