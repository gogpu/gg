// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package raster

import (
	"testing"
)

// TestAnalyticFiller_NewAndAccessors tests filler creation and accessor methods.
func TestAnalyticFiller_NewAndAccessors(t *testing.T) {
	filler := NewAnalyticFiller(200, 100)

	if filler.Width() != 200 {
		t.Errorf("Width = %d, want 200", filler.Width())
	}
	if filler.Height() != 100 {
		t.Errorf("Height = %d, want 100", filler.Height())
	}

	coverage := filler.Coverage()
	if len(coverage) != 200 {
		t.Errorf("Coverage len = %d, want 200", len(coverage))
	}

	ar := filler.AlphaRuns()
	if ar == nil {
		t.Error("AlphaRuns() returned nil")
	}
}

// TestAnalyticFiller_Reset tests resetting the filler.
func TestAnalyticFiller_Reset(t *testing.T) {
	filler := NewAnalyticFiller(100, 100)
	filler.Reset()

	// Should not panic and should remain in usable state
	if filler.Width() != 100 {
		t.Errorf("Width after reset = %d, want 100", filler.Width())
	}
}

// TestAnalyticFiller_EmptyPath tests filling an empty path.
func TestAnalyticFiller_EmptyPath(t *testing.T) {
	eb := NewEdgeBuilder(2)
	filler := NewAnalyticFiller(100, 100)

	called := false
	filler.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		called = true
	})

	if called {
		t.Error("callback should not be called for empty path")
	}
}

// TestAnalyticFiller_FillTriangle tests filling a triangle.
func TestAnalyticFiller_FillTriangle(t *testing.T) {
	path := &testPath{
		verbs: []PathVerb{
			VerbMoveTo,
			VerbLineTo,
			VerbLineTo,
			VerbClose,
		},
		points: []float32{
			50, 10,
			10, 90,
			90, 90,
		},
	}

	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	filler := NewAnalyticFiller(100, 100)

	var filledRows int
	filler.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		filledRows++
	})

	if filledRows == 0 {
		t.Error("triangle should produce at least one filled scanline")
	}
	// The triangle spans from y=10 to y=90, so expect ~80 scanlines
	if filledRows < 70 {
		t.Errorf("triangle should fill ~80 rows, got %d", filledRows)
	}
}

// TestAnalyticFiller_FillRuleEvenOdd tests even-odd fill rule.
func TestAnalyticFiller_FillRuleEvenOdd(t *testing.T) {
	path := &testPath{
		verbs: []PathVerb{
			VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose,
		},
		points: []float32{
			50, 10, 10, 90, 90, 90,
		},
	}

	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	filler := NewAnalyticFiller(100, 100)

	var filledRows int
	filler.Fill(eb, FillRuleEvenOdd, func(y int, runs *AlphaRuns) {
		filledRows++
	})

	if filledRows == 0 {
		t.Error("even-odd fill should produce scanlines")
	}
}

// TestFillPath tests the convenience function.
func TestFillPath(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose},
		points: []float32{50, 10, 10, 90, 90, 90},
	}

	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	var rows int
	FillPath(eb, 100, 100, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		rows++
	})

	if rows == 0 {
		t.Error("FillPath should produce scanlines")
	}
}

// TestFillToBuffer tests the buffer fill function.
func TestFillToBuffer(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose},
		points: []float32{50, 10, 10, 90, 90, 90},
	}

	width, height := 100, 100
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	buffer := make([]uint8, width*height)
	FillToBuffer(eb, width, height, FillRuleNonZero, buffer)

	// Interior pixel should have non-zero coverage
	center := 50*width + 50
	if buffer[center] == 0 {
		t.Error("center of triangle should have non-zero coverage")
	}

	// Corner should be empty
	if buffer[0] != 0 {
		t.Errorf("top-left corner coverage = %d, want 0", buffer[0])
	}
}

// TestFillToBuffer_SmallBuffer tests buffer size validation.
func TestFillToBuffer_SmallBuffer(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose},
		points: []float32{50, 10, 10, 90, 90, 90},
	}

	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	// Buffer too small - should not panic
	small := make([]uint8, 10)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, small)
}

// TestClamp32 tests the float32 clamping helper.
func TestClamp32(t *testing.T) {
	tests := []struct {
		v, min, max float32
		want        float32
	}{
		{0.5, 0, 1, 0.5},
		{-0.5, 0, 1, 0},
		{1.5, 0, 1, 1},
		{0, 0, 0, 0},
	}

	for _, tt := range tests {
		got := clamp32(tt.v, tt.min, tt.max)
		if got != tt.want {
			t.Errorf("clamp32(%f, %f, %f) = %f, want %f", tt.v, tt.min, tt.max, got, tt.want)
		}
	}
}

// TestAnalyticFiller_ClipBounds tests that fill respects canvas bounds.
func TestAnalyticFiller_ClipBounds(t *testing.T) {
	// Path extends beyond canvas
	path := &testPath{
		verbs:  []PathVerb{VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose},
		points: []float32{50, -20, -20, 120, 120, 120}, // extends beyond 100x100
	}

	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	filler := NewAnalyticFiller(100, 100)

	maxY := -1
	minY := 999
	filler.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		if y < 0 || y >= 100 {
			t.Errorf("callback Y=%d is out of canvas bounds [0, 100)", y)
		}
		if y > maxY {
			maxY = y
		}
		if y < minY {
			minY = y
		}
	})

	if minY < 0 {
		t.Errorf("minimum Y = %d, should be >= 0", minY)
	}
	if maxY >= 100 {
		t.Errorf("maximum Y = %d, should be < 100", maxY)
	}
}

// TestScenePathAdapter tests the scene path adapter.
func TestScenePathAdapter(t *testing.T) {
	adapter := NewScenePathAdapter(false,
		[]PathVerb{VerbMoveTo, VerbLineTo, VerbClose},
		[]float32{0, 0, 100, 100},
	)

	if adapter.IsEmpty() {
		t.Error("adapter should not be empty")
	}
	if len(adapter.Verbs()) != 3 {
		t.Errorf("Verbs len = %d, want 3", len(adapter.Verbs()))
	}
	if len(adapter.Points()) != 4 {
		t.Errorf("Points len = %d, want 4", len(adapter.Points()))
	}

	// Test empty adapter
	empty := NewScenePathAdapter(true, nil, nil)
	if !empty.IsEmpty() {
		t.Error("empty adapter should be empty")
	}
}

// TestAnalyticFiller_CurveEdges tests filling with curve edges (not flattened).
func TestAnalyticFiller_CurveEdges(t *testing.T) {
	// Circle path using cubic curves
	path := makeCirclePath(50, 50, 30)

	eb := NewEdgeBuilder(2)
	// Don't flatten - use native curve edges
	eb.SetFlattenCurves(false)
	eb.BuildFromPath(path, IdentityTransform{})

	filler := NewAnalyticFiller(100, 100)

	var filledRows int
	filler.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		filledRows++
	})

	if filledRows == 0 {
		t.Error("circle with curve edges should produce filled scanlines")
	}
	t.Logf("Circle with curve edges: %d filled rows", filledRows)
}

// BenchmarkAnalyticFiller_Triangle benchmarks triangle filling.
func BenchmarkAnalyticFiller_Triangle(b *testing.B) {
	path := &testPath{
		verbs:  []PathVerb{VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose},
		points: []float32{50, 10, 10, 90, 90, 90},
	}

	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	filler := NewAnalyticFiller(100, 100)
	nopCallback := func(y int, runs *AlphaRuns) {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		eb.Reset()
		eb.BuildFromPath(path, IdentityTransform{})
		filler.Fill(eb, FillRuleNonZero, nopCallback)
	}
}

// BenchmarkAnalyticFiller_Circle benchmarks circle filling.
func BenchmarkAnalyticFiller_Circle(b *testing.B) {
	path := makeCirclePath(100, 100, 80)

	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	filler := NewAnalyticFiller(200, 200)
	nopCallback := func(y int, runs *AlphaRuns) {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filler.Reset()
		eb.Reset()
		eb.BuildFromPath(path, IdentityTransform{})
		filler.Fill(eb, FillRuleNonZero, nopCallback)
	}
}
