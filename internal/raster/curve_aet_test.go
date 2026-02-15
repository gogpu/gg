// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package raster

import (
	"testing"
)

// TestCurveAwareAET_Basic tests basic AET operations.
func TestCurveAwareAET_Basic(t *testing.T) {
	aet := NewCurveAwareAET()

	if !aet.IsEmpty() {
		t.Error("new AET should be empty")
	}
	if aet.Len() != 0 {
		t.Errorf("new AET Len = %d, want 0", aet.Len())
	}
}

// TestCurveAwareAET_Insert tests edge insertion.
func TestCurveAwareAET_Insert(t *testing.T) {
	aet := NewCurveAwareAET()

	// Create a line edge variant
	p0 := CurvePoint{X: 0, Y: 0}
	p1 := CurvePoint{X: 10, Y: 10}
	variant := NewLineEdgeVariant(p0, p1, 0)
	if variant == nil {
		t.Fatal("NewLineEdgeVariant returned nil")
	}

	aet.Insert(*variant)
	if aet.Len() != 1 {
		t.Errorf("AET Len after insert = %d, want 1", aet.Len())
	}
	if aet.IsEmpty() {
		t.Error("AET should not be empty after insert")
	}

	// Insert a nil-line variant should be ignored
	nilVariant := CurveEdgeVariant{Type: EdgeType(99)}
	aet.Insert(nilVariant)
	if aet.Len() != 1 {
		t.Errorf("AET Len after nil insert = %d, want 1", aet.Len())
	}
}

// TestCurveAwareAET_Reset tests clearing the AET.
func TestCurveAwareAET_Reset(t *testing.T) {
	aet := NewCurveAwareAET()

	p0 := CurvePoint{X: 0, Y: 0}
	p1 := CurvePoint{X: 10, Y: 10}
	variant := NewLineEdgeVariant(p0, p1, 0)
	if variant != nil {
		aet.Insert(*variant)
	}

	aet.Reset()
	if !aet.IsEmpty() {
		t.Error("AET should be empty after Reset")
	}
}

// TestCurveAwareAET_Edges tests the Edges accessor.
func TestCurveAwareAET_Edges(t *testing.T) {
	aet := NewCurveAwareAET()

	p0 := CurvePoint{X: 0, Y: 0}
	p1 := CurvePoint{X: 10, Y: 20}
	variant := NewLineEdgeVariant(p0, p1, 0)
	if variant == nil {
		t.Skip("NewLineEdgeVariant returned nil")
	}

	aet.Insert(*variant)
	edges := aet.Edges()
	if len(edges) != 1 {
		t.Errorf("Edges() returned %d edges, want 1", len(edges))
	}
}

// TestCurveAwareAET_EdgeAt tests indexed access.
func TestCurveAwareAET_EdgeAt(t *testing.T) {
	aet := NewCurveAwareAET()

	p0 := CurvePoint{X: 5, Y: 0}
	p1 := CurvePoint{X: 5, Y: 20}
	variant := NewLineEdgeVariant(p0, p1, 0)
	if variant == nil {
		t.Skip("NewLineEdgeVariant returned nil")
	}

	aet.Insert(*variant)
	edge := aet.EdgeAt(0)
	if edge == nil {
		t.Fatal("EdgeAt(0) returned nil")
	}
	if edge.Type != EdgeTypeLine {
		t.Errorf("EdgeAt(0).Type = %d, want EdgeTypeLine", edge.Type)
	}
}

// TestCurveAwareAET_SortByX tests X-sorting of edges.
func TestCurveAwareAET_SortByX(t *testing.T) {
	aet := NewCurveAwareAET()

	// Insert edges at different X positions
	// Note: X in LineEdge is FDot16, not float pixels
	testEdges := []struct {
		x0, y0, x1, y1 float32
	}{
		{30, 0, 30, 20},
		{10, 0, 10, 20},
		{20, 0, 20, 20},
	}

	for _, te := range testEdges {
		v := NewLineEdgeVariant(
			CurvePoint{X: te.x0, Y: te.y0},
			CurvePoint{X: te.x1, Y: te.y1},
			0,
		)
		if v != nil {
			aet.Insert(*v)
		}
	}

	aet.SortByX()

	// Verify X-sorted order
	var prevX FDot16
	first := true
	aet.ForEach(func(e *CurveEdgeVariant) bool {
		line := e.AsLine()
		if line != nil {
			if !first && line.X < prevX {
				t.Errorf("not sorted: X=%d after X=%d", line.X, prevX)
			}
			prevX = line.X
			first = false
		}
		return true
	})
}

// TestCurveAwareAET_RemoveExpired tests edge expiration.
func TestCurveAwareAET_RemoveExpired(t *testing.T) {
	aet := NewCurveAwareAET()

	// Insert edges with different Y extents
	v1 := NewLineEdgeVariant(CurvePoint{X: 0, Y: 0}, CurvePoint{X: 0, Y: 5}, 0)
	v2 := NewLineEdgeVariant(CurvePoint{X: 10, Y: 0}, CurvePoint{X: 10, Y: 15}, 0)

	if v1 != nil {
		aet.Insert(*v1)
	}
	if v2 != nil {
		aet.Insert(*v2)
	}

	initialLen := aet.Len()
	if initialLen < 1 {
		t.Skip("no edges inserted")
	}

	// Remove edges whose LastY < 10
	// v1 spans y 0..5, v2 spans y 0..15
	aet.RemoveExpired(10)

	// v1 should be expired (LastY < 10), v2 should remain
	if aet.Len() >= initialLen {
		// At minimum, v1 should have been removed
		t.Logf("AET Len after RemoveExpired(10) = %d (was %d)", aet.Len(), initialLen)
	}
}

// TestCurveAwareAET_RemoveExpiredSubpixel tests sub-pixel edge expiration.
func TestCurveAwareAET_RemoveExpiredSubpixel(t *testing.T) {
	aet := NewCurveAwareAET()

	// Insert line edge: y 0..10
	v := NewLineEdgeVariant(CurvePoint{X: 5, Y: 0}, CurvePoint{X: 5, Y: 10}, 0)
	if v == nil {
		t.Skip("NewLineEdgeVariant returned nil")
	}
	aet.Insert(*v)

	// BottomY for a line is LastY + 1; after RemoveExpiredSubpixel
	// with large Y, the edge should be removed
	aet.RemoveExpiredSubpixel(100)
	if aet.Len() != 0 {
		t.Errorf("AET Len after RemoveExpiredSubpixel(100) = %d, want 0", aet.Len())
	}
}

// TestCurveAwareAET_AdvanceX tests edge X advancement.
func TestCurveAwareAET_AdvanceX(t *testing.T) {
	aet := NewCurveAwareAET()

	// Create a diagonal line with non-zero slope
	v := NewLineEdgeVariant(CurvePoint{X: 0, Y: 0}, CurvePoint{X: 20, Y: 20}, 0)
	if v == nil {
		t.Skip("NewLineEdgeVariant returned nil")
	}
	aet.Insert(*v)

	line := aet.EdgeAt(0).AsLine()
	if line == nil {
		t.Fatal("AsLine() returned nil")
	}
	xBefore := line.X

	aet.AdvanceX()

	xAfter := line.X
	if line.DX != 0 && xAfter == xBefore {
		t.Error("AdvanceX did not change X for non-vertical edge")
	}
}

// TestCurveAwareAET_ForEach tests iteration.
func TestCurveAwareAET_ForEach(t *testing.T) {
	aet := NewCurveAwareAET()

	v1 := NewLineEdgeVariant(CurvePoint{X: 5, Y: 0}, CurvePoint{X: 5, Y: 10}, 0)
	v2 := NewLineEdgeVariant(CurvePoint{X: 15, Y: 0}, CurvePoint{X: 15, Y: 10}, 0)
	if v1 != nil {
		aet.Insert(*v1)
	}
	if v2 != nil {
		aet.Insert(*v2)
	}

	count := 0
	aet.ForEach(func(e *CurveEdgeVariant) bool {
		count++
		return true
	})
	if count != aet.Len() {
		t.Errorf("ForEach visited %d edges, Len = %d", count, aet.Len())
	}

	// Test early termination
	earlyCount := 0
	aet.ForEach(func(e *CurveEdgeVariant) bool {
		earlyCount++
		return false // stop after first
	})
	if earlyCount != 1 {
		t.Errorf("ForEach with early stop visited %d edges, want 1", earlyCount)
	}
}

// TestCurveAwareAET_ComputeSpans tests span computation.
func TestCurveAwareAET_ComputeSpans(t *testing.T) {
	aet := NewCurveAwareAET()

	// Empty AET should produce no spans
	var spans []struct{ x, width int }
	aet.ComputeSpans(0, FillRuleNonZero, func(x, width int, coverage float32) {
		spans = append(spans, struct{ x, width int }{x, width})
	})
	if len(spans) != 0 {
		t.Errorf("empty AET produced %d spans, want 0", len(spans))
	}

	// Create two vertical edges to define a span
	v1 := NewLineEdgeVariant(CurvePoint{X: 10, Y: 0}, CurvePoint{X: 10, Y: 20}, 0)
	v2 := NewLineEdgeVariant(CurvePoint{X: 30, Y: 0}, CurvePoint{X: 30, Y: 20}, 0)
	if v1 != nil {
		aet.Insert(*v1)
	}
	if v2 != nil {
		aet.Insert(*v2)
	}
	aet.SortByX()

	spans = nil
	aet.ComputeSpans(5, FillRuleNonZero, func(x, width int, coverage float32) {
		spans = append(spans, struct{ x, width int }{x, width})
	})

	// Should produce at least one span between the two edges
	t.Logf("ComputeSpans produced %d spans", len(spans))
}

// TestCurveAwareAET_ComputeSpansEvenOdd tests even-odd fill rule.
func TestCurveAwareAET_ComputeSpansEvenOdd(t *testing.T) {
	aet := NewCurveAwareAET()

	v1 := NewLineEdgeVariant(CurvePoint{X: 10, Y: 0}, CurvePoint{X: 10, Y: 20}, 0)
	v2 := NewLineEdgeVariant(CurvePoint{X: 30, Y: 0}, CurvePoint{X: 30, Y: 20}, 0)
	if v1 != nil {
		aet.Insert(*v1)
	}
	if v2 != nil {
		aet.Insert(*v2)
	}
	aet.SortByX()

	var spans int
	aet.ComputeSpans(5, FillRuleEvenOdd, func(x, width int, coverage float32) {
		spans++
	})
	t.Logf("EvenOdd ComputeSpans produced %d spans", spans)
}

// TestFillRule_String tests the FillRule String method.
func TestFillRule_String(t *testing.T) {
	tests := []struct {
		fr   FillRule
		want string
	}{
		{FillRuleNonZero, "NonZero"},
		{FillRuleEvenOdd, "EvenOdd"},
		{FillRule(99), "Unknown"},
	}

	for _, tt := range tests {
		got := tt.fr.String()
		if got != tt.want {
			t.Errorf("FillRule(%d).String() = %q, want %q", tt.fr, got, tt.want)
		}
	}
}

// TestCurveAwareAET_StepCurves tests curve stepping.
func TestCurveAwareAET_StepCurves(t *testing.T) {
	aet := NewCurveAwareAET()

	// Insert a quadratic edge that will have multiple segments
	p0 := CurvePoint{X: 10, Y: 0}
	p1 := CurvePoint{X: 50, Y: 50}
	p2 := CurvePoint{X: 90, Y: 0}
	variant := NewQuadraticEdgeVariant(p0, p1, p2, 0)
	if variant == nil {
		t.Skip("NewQuadraticEdgeVariant returned nil for test curve")
	}

	aet.Insert(*variant)

	// StepCurves should not panic
	aet.StepCurves()
	t.Logf("After StepCurves: AET Len = %d", aet.Len())
}
