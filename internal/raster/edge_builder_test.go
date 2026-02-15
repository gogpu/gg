// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package raster

import (
	"testing"
)

// TestEdgeBuilderEmpty tests the builder with no input.
func TestEdgeBuilderEmpty(t *testing.T) {
	eb := NewEdgeBuilder(2)

	if !eb.IsEmpty() {
		t.Error("new EdgeBuilder should be empty")
	}
	if eb.EdgeCount() != 0 {
		t.Errorf("EdgeCount = %d, want 0", eb.EdgeCount())
	}
	if eb.LineEdgeCount() != 0 {
		t.Errorf("LineEdgeCount = %d, want 0", eb.LineEdgeCount())
	}
	if eb.QuadraticEdgeCount() != 0 {
		t.Errorf("QuadraticEdgeCount = %d, want 0", eb.QuadraticEdgeCount())
	}
	if eb.CubicEdgeCount() != 0 {
		t.Errorf("CubicEdgeCount = %d, want 0", eb.CubicEdgeCount())
	}
	if eb.AAShift() != 2 {
		t.Errorf("AAShift = %d, want 2", eb.AAShift())
	}
}

// TestEdgeBuilderNilPath tests building from nil or empty paths.
func TestEdgeBuilderNilPath(t *testing.T) {
	eb := NewEdgeBuilder(2)

	// nil path should not panic
	eb.BuildFromPath(nil, IdentityTransform{})
	if !eb.IsEmpty() {
		t.Error("BuildFromPath(nil) should leave builder empty")
	}

	// empty path
	eb.BuildFromPath(&testPath{}, IdentityTransform{})
	if !eb.IsEmpty() {
		t.Error("BuildFromPath(empty) should leave builder empty")
	}
}

// TestEdgeBuilderTriangle tests building edges from a triangle path.
func TestEdgeBuilderTriangle(t *testing.T) {
	path := &testPath{
		verbs: []PathVerb{
			VerbMoveTo,
			VerbLineTo,
			VerbLineTo,
			VerbClose,
		},
		points: []float32{
			50, 10, // move to top
			10, 90, // line to bottom-left
			90, 90, // line to bottom-right
		},
	}

	eb := NewEdgeBuilder(0) // no AA
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	if eb.IsEmpty() {
		t.Fatal("EdgeBuilder should not be empty after triangle")
	}

	// Triangle has 3 sides, each becomes a line edge (or is removed if horizontal)
	// The bottom edge (10,90)-(90,90) is horizontal and should be skipped
	// So we expect 2 line edges, but edge building uses fixed-point rounding
	// which may create slightly more or fewer edges.
	if eb.EdgeCount() < 2 {
		t.Errorf("EdgeCount = %d, want at least 2 for triangle", eb.EdgeCount())
	}

	bounds := eb.Bounds()
	if bounds.IsEmpty() {
		t.Error("triangle bounds should not be empty")
	}
}

// TestEdgeBuilderRectangle tests building edges from a rectangle path.
func TestEdgeBuilderRectangle(t *testing.T) {
	path := &testPath{
		verbs: []PathVerb{
			VerbMoveTo,
			VerbLineTo,
			VerbLineTo,
			VerbLineTo,
			VerbClose,
		},
		points: []float32{
			10, 10, // top-left
			90, 10, // top-right
			90, 90, // bottom-right
			10, 90, // bottom-left
		},
	}

	eb := NewEdgeBuilder(2) // with AA
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	if eb.IsEmpty() {
		t.Fatal("EdgeBuilder should not be empty after rectangle")
	}

	// Rectangle has 4 sides, 2 horizontal (top/bottom) and 2 vertical
	// Horizontal edges are skipped, so we expect 2 vertical edges
	if eb.LineEdgeCount() < 2 {
		t.Errorf("LineEdgeCount = %d, want at least 2 for rectangle", eb.LineEdgeCount())
	}

	// Check bounds
	bounds := eb.Bounds()
	if bounds.MinX > 10.5 || bounds.MaxX < 89.5 {
		t.Errorf("bounds X range [%f, %f] doesn't contain rectangle", bounds.MinX, bounds.MaxX)
	}
	if bounds.MinY > 10.5 || bounds.MaxY < 89.5 {
		t.Errorf("bounds Y range [%f, %f] doesn't contain rectangle", bounds.MinY, bounds.MaxY)
	}
}

// TestEdgeBuilderQuadCurve tests building with quadratic curves.
func TestEdgeBuilderQuadCurve(t *testing.T) {
	// Simple quadratic from (10,10) control (50,50) to (90,10) — arch shape
	path := &testPath{
		verbs: []PathVerb{
			VerbMoveTo,
			VerbQuadTo,
			VerbClose,
		},
		points: []float32{
			10, 10, // start
			50, 90, // control
			90, 10, // end
		},
	}

	t.Run("flattened", func(t *testing.T) {
		eb := NewEdgeBuilder(2)
		eb.SetFlattenCurves(true)
		eb.BuildFromPath(path, IdentityTransform{})

		if eb.IsEmpty() {
			t.Fatal("EdgeBuilder should not be empty after quad curve")
		}
		// When flattened, should produce multiple line edges
		if eb.LineEdgeCount() < 3 {
			t.Errorf("LineEdgeCount = %d, want at least 3 for flattened quad", eb.LineEdgeCount())
		}
		if !eb.FlattenCurves() {
			t.Error("FlattenCurves() should return true")
		}
	})

	t.Run("native curves", func(t *testing.T) {
		eb := NewEdgeBuilder(2)
		eb.SetFlattenCurves(false)
		eb.BuildFromPath(path, IdentityTransform{})

		if eb.IsEmpty() {
			t.Fatal("EdgeBuilder should not be empty after quad curve")
		}
		// When not flattened, should produce quadratic edges
		if eb.QuadraticEdgeCount() < 1 {
			t.Errorf("QuadraticEdgeCount = %d, want at least 1", eb.QuadraticEdgeCount())
		}
	})
}

// TestEdgeBuilderCubicCurve tests building with cubic curves.
func TestEdgeBuilderCubicCurve(t *testing.T) {
	// S-curve
	path := &testPath{
		verbs: []PathVerb{
			VerbMoveTo,
			VerbCubicTo,
			VerbClose,
		},
		points: []float32{
			10, 10, // start
			30, 80, // control 1
			70, 20, // control 2
			90, 90, // end
		},
	}

	t.Run("flattened", func(t *testing.T) {
		eb := NewEdgeBuilder(2)
		eb.SetFlattenCurves(true)
		eb.BuildFromPath(path, IdentityTransform{})

		if eb.IsEmpty() {
			t.Fatal("EdgeBuilder should not be empty after cubic curve")
		}
		if eb.LineEdgeCount() < 3 {
			t.Errorf("LineEdgeCount = %d, want at least 3 for flattened cubic", eb.LineEdgeCount())
		}
	})

	t.Run("native curves", func(t *testing.T) {
		eb := NewEdgeBuilder(2)
		eb.SetFlattenCurves(false)
		eb.BuildFromPath(path, IdentityTransform{})

		if eb.IsEmpty() {
			t.Fatal("EdgeBuilder should not be empty after cubic curve")
		}
	})
}

// TestEdgeBuilderVelloLines tests that VelloLines are populated when flattening.
func TestEdgeBuilderVelloLines(t *testing.T) {
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

	eb := NewEdgeBuilder(0)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	velloLines := eb.VelloLines()
	if len(velloLines) == 0 {
		t.Error("VelloLines should be populated when flattenCurves is true")
	}

	// Each VelloLine should have P0.y <= P1.y (normalized)
	for i, vl := range velloLines {
		if vl.P0[1] > vl.P1[1] {
			t.Errorf("VelloLine[%d]: P0.y=%f > P1.y=%f (should be normalized)", i, vl.P0[1], vl.P1[1])
		}
	}
}

// TestEdgeBuilderReset tests builder reuse via Reset.
func TestEdgeBuilderReset(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{VerbMoveTo, VerbLineTo, VerbClose},
		points: []float32{0, 0, 10, 10},
	}

	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	if eb.IsEmpty() {
		t.Fatal("should have edges before reset")
	}

	eb.Reset()
	if !eb.IsEmpty() {
		t.Error("should be empty after reset")
	}
	if eb.EdgeCount() != 0 {
		t.Errorf("EdgeCount after reset = %d, want 0", eb.EdgeCount())
	}
	bounds := eb.Bounds()
	if !bounds.IsEmpty() {
		t.Error("bounds should be empty after reset")
	}
}

// TestEdgeBuilderAllEdges tests the sorted edge iterator.
func TestEdgeBuilderAllEdges(t *testing.T) {
	// Two separate line segments that start at different Y
	path := &testPath{
		verbs: []PathVerb{
			VerbMoveTo, VerbLineTo,
			VerbMoveTo, VerbLineTo,
		},
		points: []float32{
			0, 50, 10, 100, // starts at y=50
			0, 10, 10, 40, // starts at y=10
		},
	}

	eb := NewEdgeBuilder(0)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	// AllEdges should yield edges sorted by top Y
	var topYs []int32
	for edge := range eb.AllEdges() {
		line := edge.AsLine()
		if line != nil {
			topYs = append(topYs, line.FirstY)
		}
	}

	for i := 1; i < len(topYs); i++ {
		if topYs[i] < topYs[i-1] {
			t.Errorf("AllEdges not sorted: topY[%d]=%d < topY[%d]=%d",
				i, topYs[i], i-1, topYs[i-1])
		}
	}
}

// TestEdgeBuilderLineEdgesIterator tests the line edges iterator.
func TestEdgeBuilderLineEdgesIterator(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose},
		points: []float32{50, 10, 10, 90, 90, 90},
	}

	eb := NewEdgeBuilder(0)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	count := 0
	for edge := range eb.LineEdges() {
		if edge == nil {
			t.Error("nil edge from LineEdges iterator")
		}
		count++
	}
	if count != eb.LineEdgeCount() {
		t.Errorf("LineEdges iterator yielded %d, LineEdgeCount = %d", count, eb.LineEdgeCount())
	}
}

// TestEdgeBuilderEmptyRect tests the EmptyRect helper.
func TestEdgeBuilderEmptyRect(t *testing.T) {
	r := EmptyRect()
	if !r.IsEmpty() {
		t.Error("EmptyRect() should be empty")
	}
}

// TestCombineVertical tests vertical edge combination logic.
func TestCombineVertical(t *testing.T) {
	t.Run("non-vertical edges", func(t *testing.T) {
		edge := &LineEdge{X: FDot16One, DX: 100, FirstY: 0, LastY: 10, Winding: 1}
		last := &LineEdge{X: FDot16One, DX: 0, FirstY: 0, LastY: 10, Winding: 1}
		if combineVertical(edge, last) != combineNo {
			t.Error("non-vertical edges should not combine")
		}
	})

	t.Run("different X positions", func(t *testing.T) {
		edge := &LineEdge{X: FDot16One, DX: 0, FirstY: 0, LastY: 10, Winding: 1}
		last := &LineEdge{X: 2 * FDot16One, DX: 0, FirstY: 0, LastY: 10, Winding: 1}
		if combineVertical(edge, last) != combineNo {
			t.Error("edges at different X should not combine")
		}
	})

	t.Run("same winding extend below", func(t *testing.T) {
		last := &LineEdge{X: FDot16One, DX: 0, FirstY: 5, LastY: 10, Winding: 1}
		edge := &LineEdge{X: FDot16One, DX: 0, FirstY: 11, LastY: 20, Winding: 1}
		// edge.FirstY == last.LastY + 1, extend last downward
		result := combineVertical(edge, last)
		if result != combinePartial {
			t.Errorf("expected combinePartial, got %d", result)
		}
		if last.LastY != 20 {
			t.Errorf("last.LastY = %d, want 20", last.LastY)
		}
	})

	t.Run("same winding extend above", func(t *testing.T) {
		last := &LineEdge{X: FDot16One, DX: 0, FirstY: 10, LastY: 20, Winding: 1}
		edge := &LineEdge{X: FDot16One, DX: 0, FirstY: 0, LastY: 9, Winding: 1}
		// edge.LastY + 1 == last.FirstY, extend last upward
		result := combineVertical(edge, last)
		if result != combinePartial {
			t.Errorf("expected combinePartial, got %d", result)
		}
		if last.FirstY != 0 {
			t.Errorf("last.FirstY = %d, want 0", last.FirstY)
		}
	})

	t.Run("opposite winding total cancel", func(t *testing.T) {
		last := &LineEdge{X: FDot16One, DX: 0, FirstY: 5, LastY: 15, Winding: 1}
		edge := &LineEdge{X: FDot16One, DX: 0, FirstY: 5, LastY: 15, Winding: -1}
		result := combineVertical(edge, last)
		if result != combineTotal {
			t.Errorf("expected combineTotal, got %d", result)
		}
	})

	t.Run("opposite winding partial from top edge shorter", func(t *testing.T) {
		last := &LineEdge{X: FDot16One, DX: 0, FirstY: 5, LastY: 15, Winding: 1}
		edge := &LineEdge{X: FDot16One, DX: 0, FirstY: 5, LastY: 10, Winding: -1}
		result := combineVertical(edge, last)
		if result != combinePartial {
			t.Errorf("expected combinePartial, got %d", result)
		}
		if last.FirstY != 11 {
			t.Errorf("last.FirstY = %d, want 11", last.FirstY)
		}
	})

	t.Run("opposite winding partial from top edge longer", func(t *testing.T) {
		last := &LineEdge{X: FDot16One, DX: 0, FirstY: 5, LastY: 10, Winding: 1}
		edge := &LineEdge{X: FDot16One, DX: 0, FirstY: 5, LastY: 15, Winding: -1}
		result := combineVertical(edge, last)
		if result != combinePartial {
			t.Errorf("expected combinePartial, got %d", result)
		}
	})

	t.Run("opposite winding partial from bottom same end", func(t *testing.T) {
		last := &LineEdge{X: FDot16One, DX: 0, FirstY: 5, LastY: 15, Winding: 1}
		edge := &LineEdge{X: FDot16One, DX: 0, FirstY: 10, LastY: 15, Winding: -1}
		result := combineVertical(edge, last)
		if result != combinePartial {
			t.Errorf("expected combinePartial, got %d", result)
		}
		if last.LastY != 9 {
			t.Errorf("last.LastY = %d, want 9", last.LastY)
		}
	})

	t.Run("opposite winding partial from bottom edge starts before last", func(t *testing.T) {
		last := &LineEdge{X: FDot16One, DX: 0, FirstY: 10, LastY: 20, Winding: 1}
		edge := &LineEdge{X: FDot16One, DX: 0, FirstY: 5, LastY: 20, Winding: -1}
		result := combineVertical(edge, last)
		if result != combinePartial {
			t.Errorf("expected combinePartial, got %d", result)
		}
	})

	t.Run("same winding non-adjacent", func(t *testing.T) {
		last := &LineEdge{X: FDot16One, DX: 0, FirstY: 0, LastY: 5, Winding: 1}
		edge := &LineEdge{X: FDot16One, DX: 0, FirstY: 10, LastY: 20, Winding: 1}
		result := combineVertical(edge, last)
		if result != combineNo {
			t.Errorf("expected combineNo for non-adjacent edges, got %d", result)
		}
	})
}
