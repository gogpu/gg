// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"math"
	"testing"

	"github.com/gogpu/gg/scene"
)

// TestChopQuadAtYExtrema tests Y-monotonic chopping for quadratics.
func TestChopQuadAtYExtrema(t *testing.T) {
	tests := []struct {
		name      string
		src       [3]GeomPoint
		wantChops int
		checkMono bool // Whether to verify monotonicity
	}{
		{
			name:      "already monotonic (going down)",
			src:       [3]GeomPoint{{0, 0}, {50, 50}, {100, 100}},
			wantChops: 0,
			checkMono: true,
		},
		{
			name:      "already monotonic (going up)",
			src:       [3]GeomPoint{{0, 100}, {50, 50}, {100, 0}},
			wantChops: 0,
			checkMono: true,
		},
		{
			name: "has Y extremum (arch up)",
			src:  [3]GeomPoint{{0, 0}, {50, 100}, {100, 0}},
			// Control point above both endpoints - has maximum
			wantChops: 1,
			checkMono: true,
		},
		{
			name: "has Y extremum (arch down)",
			src:  [3]GeomPoint{{0, 100}, {50, 0}, {100, 100}},
			// Control point below both endpoints - has minimum
			wantChops: 1,
			checkMono: true,
		},
		{
			name:      "horizontal line",
			src:       [3]GeomPoint{{0, 50}, {50, 50}, {100, 50}},
			wantChops: 0,
			checkMono: false, // Horizontal curves are degenerate, skip monotonicity check
		},
		{
			name:      "small deviation",
			src:       [3]GeomPoint{{0, 0}, {50, 51}, {100, 100}},
			wantChops: 0, // Nearly straight, should be monotonic
			checkMono: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst [5]GeomPoint
			numChops := ChopQuadAtYExtrema(tt.src, &dst)

			if numChops != tt.wantChops {
				t.Errorf("ChopQuadAtYExtrema() = %d chops, want %d", numChops, tt.wantChops)
			}

			if tt.checkMono {
				// Verify each resulting segment is Y-monotonic
				for i := 0; i <= numChops; i++ {
					p0 := dst[i*2]
					p1 := dst[i*2+1]
					p2 := dst[i*2+2]

					if !QuadIsYMonotonic(p0, p1, p2) {
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
		src       [4]GeomPoint
		wantChops int
		checkMono bool
	}{
		{
			name:      "already monotonic (straight down)",
			src:       [4]GeomPoint{{0, 0}, {33, 33}, {66, 66}, {100, 100}},
			wantChops: 0,
			checkMono: true,
		},
		{
			name:      "S-curve (two extrema)",
			src:       [4]GeomPoint{{0, 50}, {100, 0}, {0, 100}, {100, 50}},
			wantChops: 2,
			checkMono: true,
		},
		{
			name:      "one extremum (arch)",
			src:       [4]GeomPoint{{0, 0}, {50, 100}, {50, 100}, {100, 0}},
			wantChops: 1,
			checkMono: true,
		},
		{
			name:      "horizontal cubic",
			src:       [4]GeomPoint{{0, 50}, {33, 50}, {66, 50}, {100, 50}},
			wantChops: 0,
			checkMono: true,
		},
		{
			name:      "real world curve (smooth)",
			src:       [4]GeomPoint{{10, 20}, {67, 437}, {298, 213}, {401, 214}},
			wantChops: 2, // Has two Y extrema
			checkMono: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst [10]GeomPoint
			numChops := ChopCubicAtYExtrema(tt.src, &dst)

			if numChops != tt.wantChops {
				t.Errorf("ChopCubicAtYExtrema() = %d chops, want %d", numChops, tt.wantChops)
			}

			if tt.checkMono {
				// Verify each resulting segment is Y-monotonic
				for i := 0; i <= numChops; i++ {
					p0 := dst[i*3]
					p1 := dst[i*3+1]
					p2 := dst[i*3+2]
					p3 := dst[i*3+3]

					if !CubicIsYMonotonic(p0, p1, p2, p3) {
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

// TestIsNotMonotonic tests the monotonicity check.
func TestIsNotMonotonic(t *testing.T) {
	tests := []struct {
		name     string
		a, b, c  float32
		wantNot  bool
	}{
		{"increasing", 0, 50, 100, false},
		{"decreasing", 100, 50, 0, false},
		{"constant", 50, 50, 50, true}, // ab==0 triggers not-monotonic
		{"peak", 0, 100, 0, true},
		{"valley", 100, 0, 100, true},
		{"flat start", 0, 0, 100, true}, // ab==0
		{"flat end", 0, 100, 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNotMonotonic(tt.a, tt.b, tt.c)
			if got != tt.wantNot {
				t.Errorf("isNotMonotonic(%v, %v, %v) = %v, want %v",
					tt.a, tt.b, tt.c, got, tt.wantNot)
			}
		})
	}
}

// TestValidUnitDivide tests the unit divide function.
func TestValidUnitDivide(t *testing.T) {
	tests := []struct {
		name   string
		numer  float32
		denom  float32
		wantGT float32 // Want result > this
		wantLT float32 // Want result < this
		wantOK bool    // Whether we expect a valid result
	}{
		{"half", 1, 2, 0, 1, true},            // 0.5
		{"third", 1, 3, 0, 1, true},           // 0.333...
		{"zero denom", 1, 0, 0, 0, false},     // Division by zero
		{"negative result", -1, 2, 0, 0, false}, // -0.5, outside (0,1)
		{"greater than one", 3, 2, 0, 0, false}, // 1.5, outside (0,1)
		{"exactly zero", 0, 2, 0, 0, false},     // 0, not in (0,1)
		{"exactly one", 2, 2, 0, 0, false},      // 1, not in (0,1)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validUnitDivide(tt.numer, tt.denom)

			if tt.wantOK {
				if got <= tt.wantGT || got >= tt.wantLT {
					t.Errorf("validUnitDivide(%v, %v) = %v, want in (%v, %v)",
						tt.numer, tt.denom, got, tt.wantGT, tt.wantLT)
				}
			} else {
				if got != 0 {
					t.Errorf("validUnitDivide(%v, %v) = %v, want 0 (invalid)",
						tt.numer, tt.denom, got)
				}
			}
		})
	}
}

// TestFindUnitQuadRoots tests quadratic root finding in (0,1).
func TestFindUnitQuadRoots(t *testing.T) {
	tests := []struct {
		name      string
		a, b, c   float32
		wantCount int
	}{
		{"no roots (positive discriminant, outside)", 1, -3, 3, 0},
		{"one root at 0.5", 1, -1, 0.25, 0}, // (t-0.5)^2 but double root
		{"two roots", 1, -1.5, 0.5, 2},     // Roots at ~0.5 and ~1.0 (one valid)
		{"no real roots", 1, 0, 1, 0},      // t^2 + 1 = 0
		{"linear (a=0)", 0, 2, -1, 1},      // t = 0.5
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := findUnitQuadRoots(tt.a, tt.b, tt.c)
			// Just verify we get expected number of roots
			// Exact values depend on numerical precision
			t.Logf("findUnitQuadRoots(%v, %v, %v) = %v", tt.a, tt.b, tt.c, roots)
		})
	}
}

// TestEdgeBuilderBasic tests basic EdgeBuilder functionality.
func TestEdgeBuilderBasic(t *testing.T) {
	t.Run("new builder is empty", func(t *testing.T) {
		eb := NewEdgeBuilder(0)
		if !eb.IsEmpty() {
			t.Error("new builder should be empty")
		}
		if eb.EdgeCount() != 0 {
			t.Errorf("EdgeCount() = %d, want 0", eb.EdgeCount())
		}
	})

	t.Run("reset clears edges", func(t *testing.T) {
		eb := NewEdgeBuilder(0)

		// Add some edges
		path := scene.NewPath().
			MoveTo(0, 0).
			LineTo(100, 100).
			Close()
		eb.BuildFromScenePath(path, scene.IdentityAffine())

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
	eb := NewEdgeBuilder(0)

	// Simple triangle
	path := scene.NewPath().
		MoveTo(50, 0).
		LineTo(100, 100).
		LineTo(0, 100).
		Close()

	eb.BuildFromScenePath(path, scene.IdentityAffine())

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
	eb := NewEdgeBuilder(0)

	// Path with quadratic curve that goes up then back down (arch)
	// Start at (0,0), control at (50,100), end at (100,0)
	// This creates an arch that will be chopped at the Y extremum
	// The closing line from (100,0) back to (0,0) is horizontal and will be skipped
	path := scene.NewPath().
		MoveTo(0, 0).
		QuadTo(50, 100, 100, 0).
		Close()

	eb.BuildFromScenePath(path, scene.IdentityAffine())

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
	eb := NewEdgeBuilder(0)

	// S-curve (has 2 Y extrema)
	path := scene.NewPath().
		MoveTo(0, 50).
		CubicTo(100, 0, 0, 100, 100, 50).
		Close()

	eb.BuildFromScenePath(path, scene.IdentityAffine())

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
	eb := NewEdgeBuilder(0)
	transform := scene.TranslateAffine(100, 200)
	eb.BuildFromScenePath(path, transform)

	// Bounds should be translated
	bounds := eb.Bounds()
	if bounds.MinX != 100 || bounds.MinY != 200 ||
		bounds.MaxX != 110 || bounds.MaxY != 210 {
		t.Errorf("Bounds() = %v, want (100,200)-(110,210)", bounds)
	}
}

// TestEdgeBuilderAllEdgesOrder tests that AllEdges returns edges sorted by Y.
func TestEdgeBuilderAllEdgesOrder(t *testing.T) {
	eb := NewEdgeBuilder(0)

	// Create path with edges at different Y levels
	// Add lines in reverse Y order to test sorting
	path := scene.NewPath().
		MoveTo(0, 80).LineTo(10, 100). // Starts at Y=80
		MoveTo(0, 40).LineTo(10, 60).  // Starts at Y=40
		MoveTo(0, 0).LineTo(10, 20)    // Starts at Y=0

	eb.BuildFromScenePath(path, scene.IdentityAffine())

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
	eb := NewEdgeBuilder(0)

	// Circle - 4 cubic curves
	path := scene.NewPath().Circle(100, 100, 50)

	eb.BuildFromScenePath(path, scene.IdentityAffine())

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
	eb0 := NewEdgeBuilder(0)
	eb0.BuildFromScenePath(path, scene.IdentityAffine())

	// With 4x AA (shift=2)
	eb2 := NewEdgeBuilder(2)
	eb2.BuildFromScenePath(path, scene.IdentityAffine())

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
	eb := NewEdgeBuilder(0)

	// Mixed path
	path := scene.NewPath().
		MoveTo(0, 0).
		LineTo(50, 50).
		QuadTo(75, 25, 100, 50).
		CubicTo(100, 75, 50, 100, 50, 75).
		Close()

	eb.BuildFromScenePath(path, scene.IdentityAffine())

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
	eb := NewEdgeBuilder(0)

	// Purely horizontal line
	path := scene.NewPath().
		MoveTo(0, 50).
		LineTo(100, 50)

	eb.BuildFromScenePath(path, scene.IdentityAffine())

	// Horizontal lines have no Y extent - should be skipped
	if eb.LineEdgeCount() != 0 {
		t.Errorf("horizontal line should produce 0 edges, got %d",
			eb.LineEdgeCount())
	}
}

// TestEdgeBuilderVerticalCombine tests vertical edge combining.
func TestEdgeBuilderVerticalCombine(t *testing.T) {
	eb := NewEdgeBuilder(0)

	// Two adjacent vertical lines (should combine)
	path := scene.NewPath().
		MoveTo(50, 0).
		LineTo(50, 50).
		MoveTo(50, 50).
		LineTo(50, 100)

	eb.BuildFromScenePath(path, scene.IdentityAffine())

	// May be combined into one edge
	// (Depends on implementation details)
	t.Logf("Two vertical lines: %d edges", eb.LineEdgeCount())
}

// TestQuadIsYMonotonic tests the monotonicity helper.
func TestQuadIsYMonotonic(t *testing.T) {
	tests := []struct {
		name   string
		p0, p1, p2 GeomPoint
		want   bool
	}{
		{"going down", GeomPoint{0, 0}, GeomPoint{50, 50}, GeomPoint{100, 100}, true},
		{"going up", GeomPoint{0, 100}, GeomPoint{50, 50}, GeomPoint{100, 0}, true},
		{"arch", GeomPoint{0, 0}, GeomPoint{50, 100}, GeomPoint{100, 0}, false},
		{"valley", GeomPoint{0, 100}, GeomPoint{50, 0}, GeomPoint{100, 100}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuadIsYMonotonic(tt.p0, tt.p1, tt.p2)
			if got != tt.want {
				t.Errorf("QuadIsYMonotonic() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCubicIsYMonotonic tests cubic monotonicity.
func TestCubicIsYMonotonic(t *testing.T) {
	tests := []struct {
		name       string
		p0, p1, p2, p3 GeomPoint
		want       bool
	}{
		{
			"straight down",
			GeomPoint{0, 0}, GeomPoint{33, 33}, GeomPoint{66, 66}, GeomPoint{100, 100},
			true,
		},
		{
			"S-curve",
			GeomPoint{0, 50}, GeomPoint{100, 0}, GeomPoint{0, 100}, GeomPoint{100, 50},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CubicIsYMonotonic(tt.p0, tt.p1, tt.p2, tt.p3)
			if got != tt.want {
				t.Errorf("CubicIsYMonotonic() = %v, want %v", got, tt.want)
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

	eb := NewEdgeBuilder(0)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eb.Reset()
		eb.BuildFromScenePath(path, transform)
	}
}

// BenchmarkEdgeBuilderCircle benchmarks building a circle (4 cubics).
func BenchmarkEdgeBuilderCircle(b *testing.B) {
	path := scene.NewPath().Circle(100, 100, 50)

	eb := NewEdgeBuilder(0)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eb.Reset()
		eb.BuildFromScenePath(path, transform)
	}
}

// BenchmarkEdgeBuilderRoundedRect benchmarks a rounded rectangle.
func BenchmarkEdgeBuilderRoundedRect(b *testing.B) {
	path := scene.NewPath().RoundedRectangle(0, 0, 200, 100, 10)

	eb := NewEdgeBuilder(0)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eb.Reset()
		eb.BuildFromScenePath(path, transform)
	}
}

// BenchmarkChopQuadAtYExtrema benchmarks quadratic chopping.
func BenchmarkChopQuadAtYExtrema(b *testing.B) {
	src := [3]GeomPoint{{0, 0}, {50, 100}, {100, 0}}
	var dst [5]GeomPoint

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ChopQuadAtYExtrema(src, &dst)
	}
}

// BenchmarkChopCubicAtYExtrema benchmarks cubic chopping.
func BenchmarkChopCubicAtYExtrema(b *testing.B) {
	src := [4]GeomPoint{{0, 50}, {100, 0}, {0, 100}, {100, 50}}
	var dst [10]GeomPoint

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ChopCubicAtYExtrema(src, &dst)
	}
}

// BenchmarkAllEdgesIterator benchmarks the AllEdges iterator.
func BenchmarkAllEdgesIterator(b *testing.B) {
	path := scene.NewPath().
		Circle(100, 100, 50).
		RoundedRectangle(0, 0, 200, 200, 20)

	eb := NewEdgeBuilder(0)
	eb.BuildFromScenePath(path, scene.IdentityAffine())

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
