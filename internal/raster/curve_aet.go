// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package raster

import (
	"slices"
)

// CurveAwareAET is an Active Edge Table that handles all edge types.
//
// Unlike a simple line-based AET, this table can hold LineEdge, QuadraticEdge,
// and CubicEdge types. Curve edges are stepped through their segments during
// scanline traversal using forward differencing.
//
// The AET maintains edges sorted by their current X position, which is
// essential for correct scanline rasterization and winding number calculation.
//
// Usage:
//
//	aet := NewCurveAwareAET()
//	for y := yMin; y < yMax; y++ {
//	    aet.RemoveExpired(y)
//	    for edge := range eb.EdgesStartingAt(y) {
//	        aet.Insert(edge)
//	    }
//	    aet.StepCurves()
//	    aet.SortByX()
//	    // Process edges in X order
//	}
type CurveAwareAET struct {
	// edges holds all active edges.
	edges []aetEntry
}

// aetEntry wraps a CurveEdgeVariant with additional state for AET processing.
type aetEntry struct {
	edge CurveEdgeVariant

	// nextY is the Y coordinate where this edge needs to be re-evaluated.
	// For curve edges, this is when the current line segment ends.
	nextY int32
}

// NewCurveAwareAET creates a new curve-aware Active Edge Table.
func NewCurveAwareAET() *CurveAwareAET {
	return &CurveAwareAET{
		edges: make([]aetEntry, 0, 64),
	}
}

// Reset clears the AET for reuse.
func (aet *CurveAwareAET) Reset() {
	aet.edges = aet.edges[:0]
}

// Insert adds an edge to the AET.
// The edge's current line segment is used for initial positioning.
func (aet *CurveAwareAET) Insert(e CurveEdgeVariant) {
	line := e.AsLine()
	if line == nil {
		return
	}

	entry := aetEntry{
		edge:  e,
		nextY: line.LastY + 1,
	}

	aet.edges = append(aet.edges, entry)
}

// RemoveExpired removes edges that have ended before scanline y.
// An edge is expired when its LastY < y.
func (aet *CurveAwareAET) RemoveExpired(y int32) {
	// Use in-place filtering to avoid allocations
	n := 0
	for i := range aet.edges {
		line := aet.edges[i].edge.AsLine()
		if line == nil {
			continue
		}

		// Check if edge is still active
		// For curve edges, also check if there are more segments
		isActive := line.LastY >= y

		// For curve edges, even if current segment ended, there may be more
		if !isActive {
			switch aet.edges[i].edge.Type {
			case EdgeTypeQuadratic:
				isActive = aet.edges[i].edge.Quadratic.CurveCount() > 0
			case EdgeTypeCubic:
				// Cubic uses negative count, active while < 0
				isActive = aet.edges[i].edge.Cubic.CurveCount() < 0
			}
		}

		if isActive {
			aet.edges[n] = aet.edges[i]
			n++
		}
	}
	aet.edges = aet.edges[:n]
}

// RemoveExpiredSubpixel removes edges that have completely ended.
// Uses the edge's overall BottomY (not current segment) for expiration.
//
// This is used when coordinates are in sub-pixel space with AA scaling.
// An edge is expired when its BottomY <= ySubpixel.
func (aet *CurveAwareAET) RemoveExpiredSubpixel(ySubpixel int32) {
	// Use in-place filtering to avoid allocations
	n := 0
	for i := range aet.edges {
		edge := &aet.edges[i].edge

		// Get the curve's overall bottom Y (not current segment)
		bottomY := edge.BottomY()

		// Edge is active if it hasn't completely ended
		// Note: BottomY is exclusive (like LastY + 1)
		isActive := bottomY > ySubpixel

		if isActive {
			aet.edges[n] = aet.edges[i]
			n++
		}
	}
	aet.edges = aet.edges[:n]
}

// StepCurves advances curve edges to their next line segment.
// This should be called once per scanline after RemoveExpired.
//
// For each curve edge whose current segment has ended, Update() is called
// to compute the next segment. This is where forward differencing happens.
func (aet *CurveAwareAET) StepCurves() {
	for i := range aet.edges {
		entry := &aet.edges[i]
		line := entry.edge.AsLine()
		if line == nil {
			continue
		}

		// Skip if the current segment is still valid
		// (we'll step when we reach nextY)
		switch entry.edge.Type {
		case EdgeTypeQuadratic:
			// Check if we need to advance to next segment
			if entry.edge.Quadratic.CurveCount() > 0 {
				// Try to update to next segment
				if entry.edge.Quadratic.Update() {
					// Update nextY for new segment
					entry.nextY = line.LastY + 1
				}
			}

		case EdgeTypeCubic:
			// Cubic uses negative count
			if entry.edge.Cubic.CurveCount() < 0 {
				if entry.edge.Cubic.Update() {
					entry.nextY = line.LastY + 1
				}
			}
		}
	}
}

// SortByX sorts all edges by their current X position.
// This is essential for correct scanline processing.
func (aet *CurveAwareAET) SortByX() {
	slices.SortFunc(aet.edges, func(a, b aetEntry) int {
		lineA := a.edge.AsLine()
		lineB := b.edge.AsLine()

		if lineA == nil || lineB == nil {
			return 0
		}

		// Primary sort by X position
		if lineA.X < lineB.X {
			return -1
		}
		if lineA.X > lineB.X {
			return 1
		}

		// Secondary sort by slope (DX) for stability
		if lineA.DX < lineB.DX {
			return -1
		}
		if lineA.DX > lineB.DX {
			return 1
		}

		return 0
	})
}

// Len returns the number of active edges.
func (aet *CurveAwareAET) Len() int {
	return len(aet.edges)
}

// IsEmpty returns true if there are no active edges.
func (aet *CurveAwareAET) IsEmpty() bool {
	return len(aet.edges) == 0
}

// Edges returns the slice of edge variants for processing.
// The edges are in X-sorted order after SortByX() is called.
func (aet *CurveAwareAET) Edges() []CurveEdgeVariant {
	result := make([]CurveEdgeVariant, len(aet.edges))
	for i := range aet.edges {
		result[i] = aet.edges[i].edge
	}
	return result
}

// EdgeAt returns the edge at index i.
// Panics if i is out of bounds.
func (aet *CurveAwareAET) EdgeAt(i int) *CurveEdgeVariant {
	return &aet.edges[i].edge
}

// ForEach calls fn for each edge in the AET.
// Iteration stops if fn returns false.
func (aet *CurveAwareAET) ForEach(fn func(edge *CurveEdgeVariant) bool) {
	for i := range aet.edges {
		if !fn(&aet.edges[i].edge) {
			return
		}
	}
}

// AdvanceX advances all edges to the next scanline.
// This updates the X position based on the slope (DX).
func (aet *CurveAwareAET) AdvanceX() {
	for i := range aet.edges {
		line := aet.edges[i].edge.AsLine()
		if line != nil {
			line.X += line.DX
		}
	}
}

// EdgeRange represents a range of edges for a scanline span.
type EdgeRange struct {
	StartX    int32   // Left X of the span
	EndX      int32   // Right X of the span
	Winding   int32   // Accumulated winding number
	Coverage  float32 // Accumulated coverage
	EdgeCount int     // Number of edges in this range
}

// ComputeSpans computes the horizontal spans for the current scanline.
// This implements the core of scanline rasterization:
//
//  1. For each pair of edges, compute the winding contribution
//  2. Apply fill rule to determine coverage
//  3. Return spans with their coverage values
//
// The callback is called for each span with x, width, and coverage.
func (aet *CurveAwareAET) ComputeSpans(_ int32, fillRule FillRule, callback func(x, width int, coverage float32)) {
	if len(aet.edges) == 0 {
		return
	}

	// Process edges in X order
	winding := int32(0)

	for i := 0; i < len(aet.edges); i++ {
		line := aet.edges[i].edge.AsLine()
		if line == nil {
			continue
		}

		// Update winding
		winding += int32(line.Winding)

		// Determine coverage based on fill rule
		coverage := aet.computeCoverageForWinding(winding, fillRule)

		// Emit span if we have coverage and a next edge
		aet.emitSpanIfNeeded(i, line, coverage, callback)
	}
}

// computeCoverageForWinding determines coverage from winding number.
func (aet *CurveAwareAET) computeCoverageForWinding(winding int32, fillRule FillRule) float32 {
	switch fillRule {
	case FillRuleNonZero:
		return aet.nonZeroCoverage(winding)
	case FillRuleEvenOdd:
		if (winding & 1) != 0 {
			return 1.0
		}
		return 0
	default:
		return 0
	}
}

// nonZeroCoverage computes coverage for non-zero fill rule.
func (aet *CurveAwareAET) nonZeroCoverage(winding int32) float32 {
	if winding == 0 {
		return 0
	}
	w := winding
	if w < 0 {
		w = -w
	}
	if w > 1 {
		return 1.0
	}
	return float32(w)
}

// emitSpanIfNeeded emits a span callback if conditions are met.
func (aet *CurveAwareAET) emitSpanIfNeeded(i int, line *LineEdge, coverage float32, callback func(x, width int, coverage float32)) {
	if coverage <= 0 || i+1 >= len(aet.edges) {
		return
	}

	nextLine := aet.edges[i+1].edge.AsLine()
	if nextLine == nil {
		return
	}

	x := FDot16FloorToInt(line.X)
	nextX := FDot16FloorToInt(nextLine.X)
	width := nextX - x
	if width > 0 {
		callback(int(x), int(width), coverage)
	}
}

// FillRule determines how overlapping paths are filled.
type FillRule int

const (
	// FillRuleNonZero fills regions where the winding number is non-zero.
	// This is the default fill rule and handles self-intersecting paths well.
	FillRuleNonZero FillRule = iota

	// FillRuleEvenOdd fills regions where the winding number is odd.
	// This creates a checkerboard pattern for self-intersecting paths.
	FillRuleEvenOdd
)

// String returns the string representation of the fill rule.
func (fr FillRule) String() string {
	switch fr {
	case FillRuleNonZero:
		return "NonZero"
	case FillRuleEvenOdd:
		return "EvenOdd"
	default:
		return "Unknown"
	}
}
