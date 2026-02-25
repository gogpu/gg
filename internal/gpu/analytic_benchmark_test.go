//go:build !nogpu

// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package gpu

import (
	"github.com/gogpu/gg/internal/raster"
	"testing"

	"github.com/gogpu/gg/scene"
)

// =============================================================================
// Comprehensive Benchmark Suite for Analytic Anti-Aliasing
// This file compares analytic AA performance against the baseline implementation
// =============================================================================

// createCirclePath creates a circle path for testing.
func createCirclePath(centerX, centerY, radius float32) *scene.Path {
	return scene.NewPath().Circle(centerX, centerY, radius)
}

// createRectanglePath creates a rectangle path for testing.
func createRectanglePath(x, y, w, h float32) *scene.Path {
	path := scene.NewPath()
	path.MoveTo(x, y)
	path.LineTo(x+w, y)
	path.LineTo(x+w, y+h)
	path.LineTo(x, y+h)
	path.Close()
	return path
}

// createComplexCurvesPath creates a path with multiple curves.
func createComplexCurvesPath() *scene.Path {
	path := scene.NewPath()
	// Start
	path.MoveTo(50, 10)
	// Multiple quadratic curves
	for i := 0; i < 10; i++ {
		x := float32(50 + (i+1)*30)
		y := float32(10 + (i%2)*80)
		cx := float32(50 + i*30 + 15)
		cy := float32(50)
		path.QuadTo(cx, cy, x, y)
	}
	// Close with cubic
	path.CubicTo(400, 50, 400, 90, 50, 90)
	path.Close()
	return path
}

// createStarPath creates a star shape with many vertices.
func createStarPath(centerX, centerY float32, outerRadius, innerRadius float32, points int) *scene.Path {
	path := scene.NewPath()
	for i := 0; i <= points*2; i++ {
		angle := float64(i) * 3.14159265358979 / float64(points)
		r := outerRadius
		if i%2 == 1 {
			r = innerRadius
		}
		x := centerX + r*float32(cosApprox(angle))
		y := centerY + r*float32(sinApprox(angle))
		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	path.Close()
	return path
}

// Simple sine/cosine approximation for star path (avoid math import issues in tests).
func sinApprox(x float64) float64 {
	// Taylor series approximation
	const twoPi = 2 * 3.14159265358979
	x -= float64(int(x/twoPi)) * twoPi
	if x > 3.14159265358979 {
		x -= twoPi
	}
	return x - x*x*x/6 + x*x*x*x*x/120
}

func cosApprox(x float64) float64 {
	return sinApprox(x + 3.14159265358979/2)
}

// createSCurvePath creates an S-curve with cubic beziers.
func createSCurvePath() *scene.Path {
	path := scene.NewPath()
	path.MoveTo(50, 100)
	path.CubicTo(150, 50, 250, 150, 350, 100)
	path.CubicTo(450, 50, 550, 150, 650, 100)
	path.LineTo(650, 400)
	path.CubicTo(550, 450, 450, 350, 350, 400)
	path.CubicTo(250, 450, 150, 350, 50, 400)
	path.Close()
	return path
}

// =============================================================================
// Benchmark: Analytic Filler Core Operations
// =============================================================================

// BenchmarkAnalyticFillerSimpleRect benchmarks filling a simple rectangle.
func BenchmarkAnalyticFillerSimpleRect(b *testing.B) {
	path := createRectanglePath(100, 100, 200, 150)

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(500, 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// BenchmarkAnalyticFillerCircle benchmarks filling a circle.
func BenchmarkAnalyticFillerCircle(b *testing.B) {
	path := createCirclePath(200, 200, 100)

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(500, 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// BenchmarkAnalyticFillerComplexCurves benchmarks complex curved paths.
func BenchmarkAnalyticFillerComplexCurves(b *testing.B) {
	path := createComplexCurvesPath()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(500, 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// BenchmarkAnalyticFillerStar benchmarks a complex star shape.
func BenchmarkAnalyticFillerStar(b *testing.B) {
	path := createStarPath(250, 250, 200, 80, 12)

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(500, 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// BenchmarkAnalyticFillerSCurve benchmarks S-curve rendering.
func BenchmarkAnalyticFillerSCurve(b *testing.B) {
	path := createSCurvePath()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(800, 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// =============================================================================
// Benchmark: EdgeBuilder Performance
// =============================================================================

// BenchmarkEdgeBuilderCircle100 benchmarks building edges for 100 circles.
func BenchmarkEdgeBuilderCircle100(b *testing.B) {
	// Create 100 circles
	paths := make([]*scene.Path, 100)
	for i := 0; i < 100; i++ {
		x := float32(50 + (i%10)*80)
		y := float32(50 + (i/10)*50)
		paths[i] = createCirclePath(x, y, 20)
	}

	eb := raster.NewEdgeBuilder(2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eb.Reset()
		for _, p := range paths {
			BuildEdgesFromScenePath(eb, p, scene.IdentityAffine())
		}
	}
}

// BenchmarkEdgeBuilderMixedShapes benchmarks building edges for mixed shapes.
func BenchmarkEdgeBuilderMixedShapes(b *testing.B) {
	paths := []*scene.Path{
		createCirclePath(100, 100, 50),
		createRectanglePath(200, 50, 100, 100),
		createComplexCurvesPath(),
		createStarPath(400, 400, 80, 30, 8),
		createSCurvePath(),
	}

	eb := raster.NewEdgeBuilder(2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eb.Reset()
		for _, p := range paths {
			BuildEdgesFromScenePath(eb, p, scene.IdentityAffine())
		}
	}
}

// =============================================================================
// Benchmark: Fill Rules Comparison
// =============================================================================

// BenchmarkFillRuleNonZero benchmarks NonZero fill rule.
func BenchmarkFillRuleNonZero(b *testing.B) {
	path := createStarPath(250, 250, 200, 80, 6)

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(500, 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// BenchmarkFillRuleEvenOdd benchmarks EvenOdd fill rule.
func BenchmarkFillRuleEvenOdd(b *testing.B) {
	path := createStarPath(250, 250, 200, 80, 6)

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(500, 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleEvenOdd, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// =============================================================================
// Benchmark: AA Quality Levels
// =============================================================================

// BenchmarkAAShift0 benchmarks no AA (shift=0).
func BenchmarkAAShift0(b *testing.B) {
	path := createCirclePath(200, 200, 100)

	eb := raster.NewEdgeBuilder(0) // No AA
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(500, 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// BenchmarkAAShift2 benchmarks 4x AA (shift=2).
func BenchmarkAAShift2(b *testing.B) {
	path := createCirclePath(200, 200, 100)

	eb := raster.NewEdgeBuilder(2) // 4x AA
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(500, 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// =============================================================================
// Benchmark: Resolution Scaling
// =============================================================================

// BenchmarkResolution256 benchmarks 256x256 canvas.
func BenchmarkResolution256(b *testing.B) {
	path := createCirclePath(128, 128, 100)

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(256, 256)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// BenchmarkResolution512 benchmarks 512x512 canvas.
func BenchmarkResolution512(b *testing.B) {
	path := createCirclePath(256, 256, 200)

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(512, 512)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// BenchmarkResolution1024 benchmarks 1024x1024 canvas.
func BenchmarkResolution1024(b *testing.B) {
	path := createCirclePath(512, 512, 400)

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(1024, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// BenchmarkResolution1080p benchmarks 1920x1080 (Full HD) canvas.
func BenchmarkResolution1080p(b *testing.B) {
	path := createCirclePath(960, 540, 400)

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	filler := NewAnalyticFiller(1920, 1080)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(_ int, _ *raster.AlphaRuns) {})
	}
}

// =============================================================================
// Benchmark: raster.AlphaRuns Operations
// =============================================================================

// BenchmarkAlphaRunsAdd benchmarks adding runs.
func BenchmarkAlphaRunsAdd(b *testing.B) {
	ar := raster.NewAlphaRuns(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ar.Reset()
		// Simulate typical usage: multiple adds per scanline
		ar.Add(50, 64, 100, 64)
		ar.SetOffset(0)
		ar.Add(200, 128, 50, 128)
		ar.SetOffset(0)
		ar.Add(300, 255, 200, 255)
	}
}

// BenchmarkAlphaRunsIter benchmarks iterating runs.
func BenchmarkAlphaRunsIter(b *testing.B) {
	ar := raster.NewAlphaRuns(1000)
	ar.Add(50, 128, 300, 128)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		for range ar.Iter() {
			count++
		}
	}
}

// BenchmarkAlphaRunsCopyTo benchmarks copying to buffer.
func BenchmarkAlphaRunsCopyTo(b *testing.B) {
	ar := raster.NewAlphaRuns(1000)
	ar.Add(50, 128, 300, 128)

	dst := make([]uint8, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ar.CopyTo(dst)
	}
}

// =============================================================================
// Benchmark: raster.CurveAwareAET Operations
// =============================================================================

// BenchmarkAETInsert100 benchmarks inserting 100 edges.
func BenchmarkAETInsert100(b *testing.B) {
	aet := raster.NewCurveAwareAET()

	// Pre-create edges
	edges := make([]raster.CurveEdgeVariant, 100)
	for i := range 100 {
		x := float32(i * 10)
		e := raster.NewLineEdgeVariant(
			raster.CurvePoint{X: x, Y: 0},
			raster.CurvePoint{X: x + 5, Y: 100},
			0,
		)
		if e != nil {
			edges[i] = *e
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		aet.Reset()
		for j := range edges {
			aet.Insert(edges[j])
		}
	}
}

// BenchmarkAETSortByX100 benchmarks sorting 100 edges.
func BenchmarkAETSortByX100(b *testing.B) {
	aet := raster.NewCurveAwareAET()

	// Insert edges in reverse order
	for i := 99; i >= 0; i-- {
		x := float32(i * 10)
		e := raster.NewLineEdgeVariant(
			raster.CurvePoint{X: x, Y: 0},
			raster.CurvePoint{X: x + 5, Y: 100},
			0,
		)
		if e != nil {
			aet.Insert(*e)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		aet.SortByX()
	}
}

// =============================================================================
// Benchmark: FillToBuffer End-to-End
// =============================================================================

// BenchmarkFillToBufferCircle benchmarks complete fill to buffer.
func BenchmarkFillToBufferCircle(b *testing.B) {
	path := createCirclePath(128, 128, 100)

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	buffer := make([]uint8, 256*256)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FillToBuffer(eb, 256, 256, raster.FillRuleNonZero, buffer)
	}
}

// BenchmarkFillToBufferComplex benchmarks complex path fill to buffer.
func BenchmarkFillToBufferComplex(b *testing.B) {
	path := createComplexCurvesPath()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	buffer := make([]uint8, 500*500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FillToBuffer(eb, 500, 500, raster.FillRuleNonZero, buffer)
	}
}

// =============================================================================
// Benchmark: Forward Differencing vs Naive
// =============================================================================

// BenchmarkForwardDiffQuadratic benchmarks forward differencing for quadratics.
func BenchmarkForwardDiffQuadratic(b *testing.B) {
	p0 := raster.CurvePoint{X: 0, Y: 0}
	p1 := raster.CurvePoint{X: 50, Y: 100}
	p2 := raster.CurvePoint{X: 100, Y: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		edge := raster.NewQuadraticEdge(p0, p1, p2, 0)
		if edge != nil {
			for edge.Update() {
				// O(1) per step
			}
		}
	}
}

// BenchmarkForwardDiffCubic benchmarks forward differencing for cubics.
func BenchmarkForwardDiffCubic(b *testing.B) {
	p0 := raster.CurvePoint{X: 0, Y: 0}
	p1 := raster.CurvePoint{X: 33, Y: 100}
	p2 := raster.CurvePoint{X: 66, Y: 100}
	p3 := raster.CurvePoint{X: 100, Y: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		edge := raster.NewCubicEdge(p0, p1, p2, p3, 0)
		if edge != nil {
			for edge.Update() {
				// O(1) per step
			}
		}
	}
}
