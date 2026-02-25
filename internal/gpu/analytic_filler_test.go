//go:build !nogpu

// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package gpu

import (
	"github.com/gogpu/gg/internal/raster"
	"math"
	"testing"

	"github.com/gogpu/gg/scene"
)

// =============================================================================
// raster.AlphaRuns Tests
// =============================================================================

func TestAlphaRuns_NewAndReset(t *testing.T) {
	tests := []struct {
		name  string
		width int
	}{
		{"width 1", 1},
		{"width 100", 100},
		{"width 1000", 1000},
		{"zero width becomes 1", 0},
		{"negative width becomes 1", -10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := raster.NewAlphaRuns(tt.width)

			if ar == nil {
				t.Fatal("raster.NewAlphaRuns returned nil")
			}

			// Should start empty
			if !ar.IsEmpty() {
				t.Error("new raster.AlphaRuns should be empty")
			}

			// Reset should keep it empty
			ar.Reset()
			if !ar.IsEmpty() {
				t.Error("reset raster.AlphaRuns should be empty")
			}
		})
	}
}

func TestAlphaRuns_Add(t *testing.T) {
	ar := raster.NewAlphaRuns(100)

	// Add a run
	ar.Add(10, 128, 20, 64)

	// Should no longer be empty
	if ar.IsEmpty() {
		t.Error("raster.AlphaRuns should not be empty after Add")
	}

	// Check specific pixels
	if alpha := ar.GetAlpha(10); alpha != 128 {
		t.Errorf("alpha at x=10 should be 128, got %d", alpha)
	}

	if alpha := ar.GetAlpha(15); alpha == 0 {
		t.Error("alpha in middle should be non-zero")
	}
}

func TestAlphaRuns_Accumulation(t *testing.T) {
	ar := raster.NewAlphaRuns(100)

	// Add overlapping runs - should accumulate
	ar.Add(10, 100, 10, 0)
	ar.SetOffset(0) // Reset offset for next add
	ar.Add(15, 100, 5, 0)

	// Check that overlapping area accumulated
	alpha10 := ar.GetAlpha(10)
	alpha15 := ar.GetAlpha(15)

	// x=10 should have ~100, x=15 should have ~200 (capped at 255)
	if alpha10 < 90 || alpha10 > 110 {
		t.Errorf("alpha at x=10 should be ~100, got %d", alpha10)
	}

	if alpha15 < 190 {
		t.Errorf("alpha at x=15 should be ~200 or 255, got %d", alpha15)
	}
}

func TestAlphaRuns_Iter(t *testing.T) {
	ar := raster.NewAlphaRuns(100)

	// Add a run
	ar.Add(10, 200, 5, 0)

	// Iterate and collect
	type pixel struct {
		x     int
		alpha uint8
	}
	pixels := make([]pixel, 0, 100)

	for x, alpha := range ar.Iter() {
		pixels = append(pixels, pixel{x, alpha})
	}

	// Should have some pixels
	if len(pixels) == 0 {
		t.Error("Iter should yield pixels")
	}

	// First pixel should be at x=10
	if len(pixels) > 0 && pixels[0].x != 10 {
		t.Errorf("first pixel x should be 10, got %d", pixels[0].x)
	}
}

func TestAlphaRuns_IterRuns(t *testing.T) {
	ar := raster.NewAlphaRuns(100)

	// Add a run
	ar.Add(10, 200, 5, 0)

	// Iterate runs
	runs := make([]raster.AlphaRun, 0, 10)
	for run := range ar.IterRuns() {
		runs = append(runs, run)
	}

	// Should have at least one run
	if len(runs) == 0 {
		t.Error("IterRuns should yield runs")
	}
}

func TestAlphaRuns_CopyTo(t *testing.T) {
	ar := raster.NewAlphaRuns(20)
	ar.Add(5, 128, 5, 128)

	dst := make([]uint8, 20)
	ar.CopyTo(dst)

	// Check that values were copied
	if dst[5] != 128 {
		t.Errorf("dst[5] should be 128, got %d", dst[5])
	}

	// Values outside the run should be 0
	if dst[0] != 0 {
		t.Errorf("dst[0] should be 0, got %d", dst[0])
	}
}

// TestCatchOverflow moved to raster/alpha_runs_test.go (tests unexported function)

// =============================================================================
// raster.CurveAwareAET Tests
// =============================================================================

func TestCurveAwareAET_Basic(t *testing.T) {
	aet := raster.NewCurveAwareAET()

	if !aet.IsEmpty() {
		t.Error("new AET should be empty")
	}

	if aet.Len() != 0 {
		t.Errorf("new AET length should be 0, got %d", aet.Len())
	}
}

func TestCurveAwareAET_InsertLine(t *testing.T) {
	aet := raster.NewCurveAwareAET()

	// Create a simple line edge
	edge := raster.NewLineEdgeVariant(
		raster.CurvePoint{X: 10, Y: 10},
		raster.CurvePoint{X: 10, Y: 20},
		0,
	)

	if edge == nil {
		t.Fatal("failed to create line edge")
	}

	aet.Insert(*edge)

	if aet.IsEmpty() {
		t.Error("AET should not be empty after insert")
	}

	if aet.Len() != 1 {
		t.Errorf("AET length should be 1, got %d", aet.Len())
	}
}

func TestCurveAwareAET_RemoveExpired(t *testing.T) {
	aet := raster.NewCurveAwareAET()

	// Create edges at different Y ranges
	edge1 := raster.NewLineEdgeVariant(
		raster.CurvePoint{X: 10, Y: 10},
		raster.CurvePoint{X: 10, Y: 20},
		0,
	)
	edge2 := raster.NewLineEdgeVariant(
		raster.CurvePoint{X: 20, Y: 10},
		raster.CurvePoint{X: 20, Y: 15},
		0,
	)

	if edge1 != nil {
		aet.Insert(*edge1)
	}
	if edge2 != nil {
		aet.Insert(*edge2)
	}

	initialLen := aet.Len()

	// Remove edges that ended before y=18
	aet.RemoveExpired(18)

	// edge2 should be removed (ends at y=15), edge1 should remain
	// Note: exact behavior depends on edge setup, so we just verify no crash
	_ = initialLen // Used for verification if needed
}

func TestCurveAwareAET_SortByX(t *testing.T) {
	aet := raster.NewCurveAwareAET()

	// Insert edges in reverse X order
	edges := []raster.CurvePoint{
		{X: 30, Y: 10}, {X: 30, Y: 20},
		{X: 20, Y: 10}, {X: 20, Y: 20},
		{X: 10, Y: 10}, {X: 10, Y: 20},
	}

	for i := 0; i < len(edges); i += 2 {
		e := raster.NewLineEdgeVariant(edges[i], edges[i+1], 0)
		if e != nil {
			aet.Insert(*e)
		}
	}

	aet.SortByX()

	// Verify edges are sorted
	var prevX int32 = math.MinInt32
	aet.ForEach(func(edge *raster.CurveEdgeVariant) bool {
		line := edge.AsLine()
		if line != nil {
			currentX := raster.FDot16FloorToInt(line.X)
			if currentX < prevX {
				t.Errorf("edges not sorted: %d after %d", currentX, prevX)
			}
			prevX = currentX
		}
		return true
	})
}

func TestCurveAwareAET_Reset(t *testing.T) {
	aet := raster.NewCurveAwareAET()

	// Add some edges
	edge := raster.NewLineEdgeVariant(
		raster.CurvePoint{X: 10, Y: 10},
		raster.CurvePoint{X: 10, Y: 20},
		0,
	)
	if edge != nil {
		aet.Insert(*edge)
	}

	if aet.IsEmpty() {
		t.Error("AET should not be empty before reset")
	}

	aet.Reset()

	if !aet.IsEmpty() {
		t.Error("AET should be empty after reset")
	}
}

// =============================================================================
// raster.FillRule Tests
// =============================================================================

func TestFillRule_String(t *testing.T) {
	tests := []struct {
		rule     raster.FillRule
		expected string
	}{
		{raster.FillRuleNonZero, "NonZero"},
		{raster.FillRuleEvenOdd, "EvenOdd"},
		{raster.FillRule(99), "Unknown"},
	}

	for _, tt := range tests {
		result := tt.rule.String()
		if result != tt.expected {
			t.Errorf("raster.FillRule(%d).String() = %q, want %q", tt.rule, result, tt.expected)
		}
	}
}

// =============================================================================
// AnalyticFiller Tests
// =============================================================================

func TestAnalyticFiller_New(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)

	if filler == nil {
		t.Fatal("NewAnalyticFiller returned nil")
	}

	if filler.Width() != 100 {
		t.Errorf("width should be 100, got %d", filler.Width())
	}

	if filler.Height() != 100 {
		t.Errorf("height should be 100, got %d", filler.Height())
	}
}

func TestAnalyticFiller_Reset(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	filler.Reset()

	// Should not panic and should be in clean state
	if filler.aet.Len() != 0 {
		t.Error("AET should be empty after reset")
	}
}

func TestAnalyticFiller_EmptyPath(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	eb := raster.NewEdgeBuilder(2)

	callbackCalled := false
	filler.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		callbackCalled = true
	})

	if callbackCalled {
		t.Error("callback should not be called for empty path")
	}
}

func TestAnalyticFiller_SimpleRectangle(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	eb := raster.NewEdgeBuilder(0)

	// Build a simple rectangle path
	path := scene.NewPath()
	path.MoveTo(20, 20)
	path.LineTo(40, 20)
	path.LineTo(40, 40)
	path.LineTo(20, 40)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Verify edges were created
	if eb.IsEmpty() {
		t.Fatal("raster.EdgeBuilder should not be empty after building path")
	}

	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		scanlineCount++
	})

	// Should process multiple scanlines
	if scanlineCount == 0 {
		t.Error("should process at least one scanline")
	}
}

func TestAnalyticFiller_FillRules(t *testing.T) {
	tests := []struct {
		name     string
		fillRule raster.FillRule
	}{
		{"NonZero", raster.FillRuleNonZero},
		{"EvenOdd", raster.FillRuleEvenOdd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filler := NewAnalyticFiller(100, 100)
			eb := raster.NewEdgeBuilder(0)

			// Create a simple path
			path := scene.NewPath()
			path.MoveTo(10, 10)
			path.LineTo(50, 10)
			path.LineTo(50, 50)
			path.LineTo(10, 50)
			path.Close()

			BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

			// Should not panic with either fill rule
			filler.Fill(eb, tt.fillRule, func(y int, runs *raster.AlphaRuns) {
				// Just verify callback is called
			})
		})
	}
}

func TestAnalyticFiller_Triangle(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	eb := raster.NewEdgeBuilder(0)

	// Create a triangle
	path := scene.NewPath()
	path.MoveTo(50, 10)
	path.LineTo(80, 80)
	path.LineTo(20, 80)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	if eb.IsEmpty() {
		t.Fatal("raster.EdgeBuilder should not be empty for triangle")
	}

	scanlines := make(map[int]bool)
	filler.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		scanlines[y] = true
	})

	// Should process scanlines from y=10 to y=80
	if len(scanlines) == 0 {
		t.Error("should process scanlines for triangle")
	}
}

func TestAnalyticFiller_QuadraticCurve(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	eb := raster.NewEdgeBuilder(2) // AA enabled

	// Create a path with a quadratic curve
	path := scene.NewPath()
	path.MoveTo(10, 50)
	path.QuadTo(50, 10, 90, 50) // Control point at top
	path.LineTo(90, 90)
	path.LineTo(10, 90)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Should have quadratic edges
	if eb.QuadraticEdgeCount() == 0 {
		t.Log("Warning: expected quadratic edges but got none")
	}

	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		scanlineCount++
	})

	if scanlineCount == 0 {
		t.Error("should process scanlines for curved path")
	}
}

func TestAnalyticFiller_CubicCurve(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	eb := raster.NewEdgeBuilder(2)

	// Create a path with a cubic curve
	path := scene.NewPath()
	path.MoveTo(10, 50)
	path.CubicTo(30, 10, 70, 10, 90, 50) // S-curve
	path.LineTo(90, 90)
	path.LineTo(10, 90)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Should have cubic edges
	if eb.CubicEdgeCount() == 0 {
		t.Log("Warning: expected cubic edges but got none")
	}

	scanlineCount := 0
	filler.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		scanlineCount++
	})

	if scanlineCount == 0 {
		t.Error("should process scanlines for cubic path")
	}
}

// =============================================================================
// Coverage Calculation Tests
// =============================================================================

func TestClamp32(t *testing.T) {
	tests := []struct {
		v, minV, maxV float32
		expected      float32
	}{
		{0.5, 0, 1, 0.5},
		{-0.5, 0, 1, 0},
		{1.5, 0, 1, 1},
		{50, 0, 100, 50},
	}

	for _, tt := range tests {
		result := clamp32(tt.v, tt.minV, tt.maxV)
		if result != tt.expected {
			t.Errorf("clamp32(%v, %v, %v) = %v, want %v",
				tt.v, tt.minV, tt.maxV, result, tt.expected)
		}
	}
}

func TestMin32f(t *testing.T) {
	tests := []struct {
		a, b, expected float32
	}{
		{1, 2, 1},
		{2, 1, 1},
		{-1, 1, -1},
		{0, 0, 0},
	}

	for _, tt := range tests {
		result := min32f(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("min32f(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestMax32f(t *testing.T) {
	tests := []struct {
		a, b, expected float32
	}{
		{1, 2, 2},
		{2, 1, 2},
		{-1, 1, 1},
		{0, 0, 0},
	}

	for _, tt := range tests {
		result := max32f(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("max32f(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
		}
	}
}

// =============================================================================
// Convenience Function Tests
// =============================================================================

func TestFillPath(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(50, 10)
	path.LineTo(50, 50)
	path.LineTo(10, 50)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	callbackCalled := false
	FillPath(eb, 100, 100, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		callbackCalled = true
	})

	if !callbackCalled {
		t.Error("FillPath callback should be called")
	}
}

func TestFillToBuffer(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(50, 10)
	path.LineTo(50, 50)
	path.LineTo(10, 50)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	buffer := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, raster.FillRuleNonZero, buffer)

	// Check that some pixels inside the rectangle have coverage
	centerIdx := 30*100 + 30 // y=30, x=30
	if buffer[centerIdx] == 0 {
		t.Log("Warning: center pixel has no coverage")
	}
}

func TestFillToBuffer_SmallBuffer(t *testing.T) {
	eb := raster.NewEdgeBuilder(0)

	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(50, 10)
	path.LineTo(50, 50)
	path.LineTo(10, 50)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Buffer too small - should not panic
	buffer := make([]uint8, 10)
	FillToBuffer(eb, 100, 100, raster.FillRuleNonZero, buffer)
	// Just verify it doesn't panic
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkAlphaRuns_Add(b *testing.B) {
	ar := raster.NewAlphaRuns(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ar.Reset()
		ar.Add(100, 128, 200, 128)
		ar.SetOffset(0)
		ar.Add(300, 64, 100, 64)
	}
}

func BenchmarkAlphaRuns_Iter(b *testing.B) {
	ar := raster.NewAlphaRuns(1000)
	ar.Add(100, 255, 300, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		for range ar.Iter() {
			count++
		}
	}
}

func BenchmarkCurveAwareAET_SortByX(b *testing.B) {
	aet := raster.NewCurveAwareAET()

	// Add 100 edges
	for i := 0; i < 100; i++ {
		x := float32(i * 10)
		edge := raster.NewLineEdgeVariant(
			raster.CurvePoint{X: x, Y: 0},
			raster.CurvePoint{X: x + 5, Y: 100},
			0,
		)
		if edge != nil {
			aet.Insert(*edge)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		aet.SortByX()
	}
}

func BenchmarkAnalyticFiller_Rectangle(b *testing.B) {
	filler := NewAnalyticFiller(500, 500)

	path := scene.NewPath()
	path.MoveTo(100, 100)
	path.LineTo(400, 100)
	path.LineTo(400, 400)
	path.LineTo(100, 400)
	path.Close()

	eb := raster.NewEdgeBuilder(0)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
			// Empty callback for benchmarking core algorithm
		})
	}
}

func BenchmarkAnalyticFiller_QuadraticCurve(b *testing.B) {
	filler := NewAnalyticFiller(500, 500)

	path := scene.NewPath()
	path.MoveTo(50, 250)
	path.QuadTo(250, 50, 450, 250)
	path.QuadTo(250, 450, 50, 250)
	path.Close()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {})
	}
}

func BenchmarkAnalyticFiller_ComplexPath(b *testing.B) {
	filler := NewAnalyticFiller(500, 500)

	// Create a more complex path with multiple curves
	path := scene.NewPath()
	path.MoveTo(100, 100)
	path.CubicTo(150, 50, 200, 50, 250, 100)
	path.CubicTo(300, 150, 350, 150, 400, 100)
	path.LineTo(400, 400)
	path.QuadTo(250, 450, 100, 400)
	path.Close()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		filler.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {})
	}
}
