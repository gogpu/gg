// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"iter"
	"math"
	"slices"

	"github.com/gogpu/gg/scene"
)

// EdgeBuilder converts paths to typed edges for analytic anti-aliasing.
//
// Unlike the flatten.go approach that converts all curves to line segments,
// EdgeBuilder preserves curve information (QuadraticEdge, CubicEdge) which
// enables higher quality anti-aliasing by evaluating curve coverage analytically.
//
// The builder ensures all edges are Y-monotonic by chopping curves at their
// Y extrema before creating edge objects.
//
// Usage:
//
//	eb := NewEdgeBuilder(2) // 4x AA quality
//	eb.BuildFromScenePath(path, scene.IdentityAffine())
//
//	for edge := range eb.AllEdges() {
//	    // Process edges sorted by top Y
//	}
//
// Reference: tiny-skia/src/edge_builder.rs
type EdgeBuilder struct {
	// Separate storage for different edge types
	lineEdges      []LineEdge
	quadraticEdges []*QuadraticEdge
	cubicEdges     []*CubicEdge

	// aaShift controls AA quality (0=none, 2=4x equivalent)
	aaShift int

	// bounds accumulates the bounding box of all edges
	bounds edgeBounds
}

// edgeBounds tracks the bounding rectangle during edge building.
type edgeBounds struct {
	minX, minY float32
	maxX, maxY float32
	empty      bool
}

// newEmptyBounds returns an empty bounds (ready for union operations).
func newEmptyBounds() edgeBounds {
	return edgeBounds{
		minX:  math.MaxFloat32,
		minY:  math.MaxFloat32,
		maxX:  -math.MaxFloat32,
		maxY:  -math.MaxFloat32,
		empty: true,
	}
}

// unionPoint expands bounds to include a point.
func (b *edgeBounds) unionPoint(x, y float32) {
	if b.empty {
		b.minX, b.maxX = x, x
		b.minY, b.maxY = y, y
		b.empty = false
		return
	}
	if x < b.minX {
		b.minX = x
	}
	if x > b.maxX {
		b.maxX = x
	}
	if y < b.minY {
		b.minY = y
	}
	if y > b.maxY {
		b.maxY = y
	}
}

// NewEdgeBuilder creates a new edge builder with specified AA quality.
//
// Parameters:
//   - aaShift: anti-aliasing shift (0 = no AA, 2 = 4x AA quality)
//
// Higher shift values provide better AA quality but require more memory
// and computation.
func NewEdgeBuilder(aaShift int) *EdgeBuilder {
	return &EdgeBuilder{
		lineEdges:      make([]LineEdge, 0, 64),
		quadraticEdges: make([]*QuadraticEdge, 0, 16),
		cubicEdges:     make([]*CubicEdge, 0, 16),
		aaShift:        aaShift,
		bounds:         newEmptyBounds(),
	}
}

// Reset clears the builder for reuse without deallocating memory.
func (eb *EdgeBuilder) Reset() {
	eb.lineEdges = eb.lineEdges[:0]
	eb.quadraticEdges = eb.quadraticEdges[:0]
	eb.cubicEdges = eb.cubicEdges[:0]
	eb.bounds = newEmptyBounds()
}

// BuildFromScenePath processes a scene.Path and creates typed edges.
//
// This is the main entry point for path processing. It:
//  1. Iterates through path verbs
//  2. Applies transform to all points
//  3. Chops curves at Y extrema for monotonicity
//  4. Creates appropriate edge types
//
// Parameters:
//   - path: the path to process
//   - transform: affine transformation to apply to all points
func (eb *EdgeBuilder) BuildFromScenePath(path *scene.Path, transform scene.Affine) {
	if path == nil || path.IsEmpty() {
		return
	}

	// State for path traversal
	var curX, curY float32     // Current position
	var startX, startY float32 // Subpath start for Close

	pointIdx := 0
	points := path.Points()
	verbs := path.Verbs()

	for _, verb := range verbs {
		switch verb {
		case scene.VerbMoveTo:
			// Close previous subpath if not at start
			if curX != startX || curY != startY {
				eb.addLine(curX, curY, startX, startY)
			}

			// Transform and update position
			x, y := points[pointIdx], points[pointIdx+1]
			curX, curY = transform.TransformPoint(x, y)
			startX, startY = curX, curY
			pointIdx += 2

		case scene.VerbLineTo:
			x, y := points[pointIdx], points[pointIdx+1]
			nextX, nextY := transform.TransformPoint(x, y)
			eb.addLine(curX, curY, nextX, nextY)
			curX, curY = nextX, nextY
			pointIdx += 2

		case scene.VerbQuadTo:
			// Control point and end point
			cx, cy := points[pointIdx], points[pointIdx+1]
			x, y := points[pointIdx+2], points[pointIdx+3]

			// Transform all points
			tcx, tcy := transform.TransformPoint(cx, cy)
			tx, ty := transform.TransformPoint(x, y)

			eb.addQuad(curX, curY, tcx, tcy, tx, ty)
			curX, curY = tx, ty
			pointIdx += 4

		case scene.VerbCubicTo:
			// Two control points and end point
			c1x, c1y := points[pointIdx], points[pointIdx+1]
			c2x, c2y := points[pointIdx+2], points[pointIdx+3]
			x, y := points[pointIdx+4], points[pointIdx+5]

			// Transform all points
			tc1x, tc1y := transform.TransformPoint(c1x, c1y)
			tc2x, tc2y := transform.TransformPoint(c2x, c2y)
			tx, ty := transform.TransformPoint(x, y)

			eb.addCubic(curX, curY, tc1x, tc1y, tc2x, tc2y, tx, ty)
			curX, curY = tx, ty
			pointIdx += 6

		case scene.VerbClose:
			// Close the subpath
			if curX != startX || curY != startY {
				eb.addLine(curX, curY, startX, startY)
			}
			curX, curY = startX, startY
		}
	}

	// Close final subpath if not explicitly closed
	if curX != startX || curY != startY {
		eb.addLine(curX, curY, startX, startY)
	}
}

// addLine adds a line edge, handling vertical edge combining.
func (eb *EdgeBuilder) addLine(x0, y0, x1, y1 float32) {
	// Update bounds
	eb.bounds.unionPoint(x0, y0)
	eb.bounds.unionPoint(x1, y1)

	// Create line edge
	p0 := CurvePoint{X: x0, Y: y0}
	p1 := CurvePoint{X: x1, Y: y1}

	edge := NewLineEdge(p0, p1, eb.aaShift)
	if edge == nil {
		return // Horizontal or degenerate
	}

	// Try to combine with previous vertical edge
	if edge.IsVertical() && len(eb.lineEdges) > 0 {
		last := &eb.lineEdges[len(eb.lineEdges)-1]
		combine := combineVertical(edge, last)
		switch combine {
		case combineTotal:
			// Edges cancel out - remove the last edge
			eb.lineEdges = eb.lineEdges[:len(eb.lineEdges)-1]
			return
		case combinePartial:
			// Last edge was modified - don't add new edge
			return
		case combineNo:
			// No combination - fall through to add
		}
	}

	eb.lineEdges = append(eb.lineEdges, *edge)
}

// addQuad adds quadratic curve edges, chopping at Y extrema if needed.
func (eb *EdgeBuilder) addQuad(x0, y0, cx, cy, x1, y1 float32) {
	// Update bounds (conservative - includes control point)
	eb.bounds.unionPoint(x0, y0)
	eb.bounds.unionPoint(cx, cy)
	eb.bounds.unionPoint(x1, y1)

	// Check if curve needs to be chopped at Y extrema
	src := [3]GeomPoint{
		{X: x0, Y: y0},
		{X: cx, Y: cy},
		{X: x1, Y: y1},
	}

	var dst [5]GeomPoint
	numChops := ChopQuadAtYExtrema(src, &dst)

	// Add each monotonic segment
	for i := 0; i <= numChops; i++ {
		p0 := CurvePoint{X: dst[i*2].X, Y: dst[i*2].Y}
		p1 := CurvePoint{X: dst[i*2+1].X, Y: dst[i*2+1].Y}
		p2 := CurvePoint{X: dst[i*2+2].X, Y: dst[i*2+2].Y}

		edge := NewQuadraticEdge(p0, p1, p2, eb.aaShift)
		if edge != nil {
			eb.quadraticEdges = append(eb.quadraticEdges, edge)
		}
	}
}

// addCubic adds cubic curve edges, chopping at Y extrema if needed.
func (eb *EdgeBuilder) addCubic(x0, y0, c1x, c1y, c2x, c2y, x1, y1 float32) {
	// Update bounds (conservative - includes control points)
	eb.bounds.unionPoint(x0, y0)
	eb.bounds.unionPoint(c1x, c1y)
	eb.bounds.unionPoint(c2x, c2y)
	eb.bounds.unionPoint(x1, y1)

	// Check if curve needs to be chopped at Y extrema
	src := [4]GeomPoint{
		{X: x0, Y: y0},
		{X: c1x, Y: c1y},
		{X: c2x, Y: c2y},
		{X: x1, Y: y1},
	}

	var dst [10]GeomPoint
	numChops := ChopCubicAtYExtrema(src, &dst)

	// Add each monotonic segment
	for i := 0; i <= numChops; i++ {
		p0 := CurvePoint{X: dst[i*3].X, Y: dst[i*3].Y}
		p1 := CurvePoint{X: dst[i*3+1].X, Y: dst[i*3+1].Y}
		p2 := CurvePoint{X: dst[i*3+2].X, Y: dst[i*3+2].Y}
		p3 := CurvePoint{X: dst[i*3+3].X, Y: dst[i*3+3].Y}

		edge := NewCubicEdge(p0, p1, p2, p3, eb.aaShift)
		if edge != nil {
			eb.cubicEdges = append(eb.cubicEdges, edge)
		}
	}
}

// combineResult represents the result of trying to combine vertical edges.
type combineResult int

const (
	combineNo      combineResult = iota // No combination possible
	combinePartial                      // Partial combination - last edge modified
	combineTotal                        // Total cancellation - remove last edge
)

// combineVertical attempts to combine two vertical edges.
// This optimization reduces edge count for paths with coincident vertical segments.
func combineVertical(edge, last *LineEdge) combineResult {
	// Both must be vertical and at the same X
	if last.DX != 0 || edge.X != last.X {
		return combineNo
	}

	// Same winding - try to extend
	if edge.Winding == last.Winding {
		if edge.LastY+1 == last.FirstY {
			last.FirstY = edge.FirstY
			return combinePartial
		}
		if edge.FirstY == last.LastY+1 {
			last.LastY = edge.LastY
			return combinePartial
		}
		return combineNo
	}

	// Opposite winding - try to cancel or reduce
	if edge.FirstY == last.FirstY {
		if edge.LastY == last.LastY {
			return combineTotal // Exact cancellation
		}
		if edge.LastY < last.LastY {
			last.FirstY = edge.LastY + 1
			return combinePartial
		}
		// edge.LastY > last.LastY
		last.FirstY = last.LastY + 1
		last.LastY = edge.LastY
		last.Winding = edge.Winding
		return combinePartial
	}

	if edge.LastY == last.LastY {
		if edge.FirstY > last.FirstY {
			last.LastY = edge.FirstY - 1
			return combinePartial
		}
		// edge.FirstY < last.FirstY
		last.LastY = last.FirstY - 1
		last.FirstY = edge.FirstY
		last.Winding = edge.Winding
		return combinePartial
	}

	return combineNo
}

// Bounds returns the bounding rectangle of all edges.
func (eb *EdgeBuilder) Bounds() scene.Rect {
	if eb.bounds.empty {
		return scene.EmptyRect()
	}
	return scene.Rect{
		MinX: eb.bounds.minX,
		MinY: eb.bounds.minY,
		MaxX: eb.bounds.maxX,
		MaxY: eb.bounds.maxY,
	}
}

// IsEmpty returns true if no edges have been added.
func (eb *EdgeBuilder) IsEmpty() bool {
	return len(eb.lineEdges) == 0 &&
		len(eb.quadraticEdges) == 0 &&
		len(eb.cubicEdges) == 0
}

// EdgeCount returns the total number of edges.
func (eb *EdgeBuilder) EdgeCount() int {
	return len(eb.lineEdges) + len(eb.quadraticEdges) + len(eb.cubicEdges)
}

// LineEdgeCount returns the number of line edges.
func (eb *EdgeBuilder) LineEdgeCount() int {
	return len(eb.lineEdges)
}

// QuadraticEdgeCount returns the number of quadratic edges.
func (eb *EdgeBuilder) QuadraticEdgeCount() int {
	return len(eb.quadraticEdges)
}

// CubicEdgeCount returns the number of cubic edges.
func (eb *EdgeBuilder) CubicEdgeCount() int {
	return len(eb.cubicEdges)
}

// sortableEdge pairs an edge with its top Y for sorting.
type sortableEdge struct {
	topY    int32
	variant CurveEdgeVariant
}

// AllEdges returns an iterator over all edges sorted by top Y coordinate.
//
// This uses Go 1.25+ iter.Seq for efficient iteration. Edges are yielded
// in scanline order (top to bottom), which is required for Active Edge Table
// processing.
//
// Usage:
//
//	for edge := range eb.AllEdges() {
//	    line := edge.AsLine()
//	    // Process edge.Line().FirstY, etc.
//	}
func (eb *EdgeBuilder) AllEdges() iter.Seq[CurveEdgeVariant] {
	return func(yield func(CurveEdgeVariant) bool) {
		// Collect all edges with their top Y for sorting
		edges := make([]sortableEdge, 0, eb.EdgeCount())

		// Add line edges
		for i := range eb.lineEdges {
			edges = append(edges, sortableEdge{
				topY: eb.lineEdges[i].FirstY,
				variant: CurveEdgeVariant{
					Type: EdgeTypeLine,
					Line: &eb.lineEdges[i],
				},
			})
		}

		// Add quadratic edges
		for _, quad := range eb.quadraticEdges {
			edges = append(edges, sortableEdge{
				topY: quad.line.FirstY,
				variant: CurveEdgeVariant{
					Type:      EdgeTypeQuadratic,
					Quadratic: quad,
				},
			})
		}

		// Add cubic edges
		for _, cubic := range eb.cubicEdges {
			edges = append(edges, sortableEdge{
				topY: cubic.line.FirstY,
				variant: CurveEdgeVariant{
					Type:  EdgeTypeCubic,
					Cubic: cubic,
				},
			})
		}

		// Sort by top Y (stable sort preserves insertion order for equal Y)
		slices.SortStableFunc(edges, func(a, b sortableEdge) int {
			if a.topY < b.topY {
				return -1
			}
			if a.topY > b.topY {
				return 1
			}
			return 0
		})

		// Yield edges in sorted order
		for _, e := range edges {
			if !yield(e.variant) {
				return
			}
		}
	}
}

// LineEdges returns an iterator over line edges only.
func (eb *EdgeBuilder) LineEdges() iter.Seq[*LineEdge] {
	return func(yield func(*LineEdge) bool) {
		for i := range eb.lineEdges {
			if !yield(&eb.lineEdges[i]) {
				return
			}
		}
	}
}

// QuadraticEdges returns an iterator over quadratic edges only.
func (eb *EdgeBuilder) QuadraticEdges() iter.Seq[*QuadraticEdge] {
	return func(yield func(*QuadraticEdge) bool) {
		for _, edge := range eb.quadraticEdges {
			if !yield(edge) {
				return
			}
		}
	}
}

// CubicEdges returns an iterator over cubic edges only.
func (eb *EdgeBuilder) CubicEdges() iter.Seq[*CubicEdge] {
	return func(yield func(*CubicEdge) bool) {
		for _, edge := range eb.cubicEdges {
			if !yield(edge) {
				return
			}
		}
	}
}

// AAShift returns the anti-aliasing shift value.
func (eb *EdgeBuilder) AAShift() int {
	return eb.aaShift
}
