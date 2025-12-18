package wgpu

import (
	"math"
)

// Edge represents a line segment for scanline conversion.
// Edges are derived from path segments (lines, curves flattened to lines)
// and used by the Active Edge Table algorithm.
type Edge struct {
	// yMin is the minimum Y coordinate (top of edge)
	yMin float32

	// yMax is the maximum Y coordinate (bottom of edge)
	yMax float32

	// xAtYMin is the X coordinate at yMin
	xAtYMin float32

	// dxdy is the inverse slope: change in X per unit Y
	dxdy float32

	// winding indicates the direction: +1 for downward, -1 for upward
	winding int8
}

// NewEdge creates a new edge from two points.
// Returns nil if the edge is horizontal (no Y extent).
func NewEdge(x0, y0, x1, y1 float32) *Edge {
	// Ensure y0 <= y1 (edge goes downward in normalized form)
	if y0 > y1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
	}

	// Skip horizontal edges (no Y extent)
	dy := y1 - y0
	if dy < epsilon {
		return nil
	}

	dx := x1 - x0
	dxdy := dx / dy

	// Winding is always +1 for edges created this way since we normalize direction
	var winding int8 = 1

	return &Edge{
		yMin:    y0,
		yMax:    y1,
		xAtYMin: x0,
		dxdy:    dxdy,
		winding: winding,
	}
}

// NewEdgeWithWinding creates a new edge with explicit winding.
func NewEdgeWithWinding(x0, y0, x1, y1 float32, winding int8) *Edge {
	// Normalize so yMin <= yMax
	if y0 > y1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
		winding = -winding // Reverse winding when we flip
	}

	dy := y1 - y0
	if dy < epsilon {
		return nil
	}

	dx := x1 - x0
	dxdy := dx / dy

	return &Edge{
		yMin:    y0,
		yMax:    y1,
		xAtYMin: x0,
		dxdy:    dxdy,
		winding: winding,
	}
}

// epsilon is a small value for floating point comparison.
const epsilon = 1e-6

// XAtY calculates the X coordinate at a given Y value.
// This is the core calculation for scanline intersection.
func (e *Edge) XAtY(y float32) float32 {
	return e.xAtYMin + (y-e.yMin)*e.dxdy
}

// IsActiveAt returns true if the edge is active at the given Y coordinate.
// An edge is active when yMin <= y < yMax.
func (e *Edge) IsActiveAt(y float32) bool {
	return y >= e.yMin && y < e.yMax
}

// ContainsY returns true if Y is within the edge's Y range (inclusive).
func (e *Edge) ContainsY(y float32) bool {
	return y >= e.yMin && y <= e.yMax
}

// Height returns the vertical extent of the edge.
func (e *Edge) Height() float32 {
	return e.yMax - e.yMin
}

// EdgeList is a collection of edges with utility methods.
type EdgeList struct {
	edges []Edge
}

// NewEdgeList creates a new empty edge list.
func NewEdgeList() *EdgeList {
	return &EdgeList{
		edges: make([]Edge, 0, 64),
	}
}

// Reset clears the edge list for reuse.
func (el *EdgeList) Reset() {
	el.edges = el.edges[:0]
}

// Add adds an edge to the list.
func (el *EdgeList) Add(e *Edge) {
	if e != nil {
		el.edges = append(el.edges, *e)
	}
}

// AddLine adds a line segment as an edge.
func (el *EdgeList) AddLine(x0, y0, x1, y1 float32) {
	// Determine winding based on direction
	var winding int8 = 1
	if y0 > y1 {
		winding = -1
	}

	edge := NewEdgeWithWinding(x0, y0, x1, y1, winding)
	if edge != nil {
		el.edges = append(el.edges, *edge)
	}
}

// Len returns the number of edges.
func (el *EdgeList) Len() int {
	return len(el.edges)
}

// Edges returns the underlying slice.
func (el *EdgeList) Edges() []Edge {
	return el.edges
}

// SortByYMin sorts edges by their minimum Y coordinate.
func (el *EdgeList) SortByYMin() {
	// Insertion sort (usually nearly sorted already)
	for i := 1; i < len(el.edges); i++ {
		j := i
		for j > 0 && el.edges[j].yMin < el.edges[j-1].yMin {
			el.edges[j], el.edges[j-1] = el.edges[j-1], el.edges[j]
			j--
		}
	}
}

// Bounds returns the bounding rectangle of all edges.
func (el *EdgeList) Bounds() (minX, minY, maxX, maxY float32) {
	if len(el.edges) == 0 {
		return 0, 0, 0, 0
	}

	minX = float32(math.MaxFloat32)
	minY = float32(math.MaxFloat32)
	maxX = float32(-math.MaxFloat32)
	maxY = float32(-math.MaxFloat32)

	for i := range el.edges {
		e := &el.edges[i]

		// Y bounds
		if e.yMin < minY {
			minY = e.yMin
		}
		if e.yMax > maxY {
			maxY = e.yMax
		}

		// X bounds (check both endpoints)
		x0 := e.xAtYMin
		x1 := e.XAtY(e.yMax)

		if x0 < minX {
			minX = x0
		}
		if x0 > maxX {
			maxX = x0
		}
		if x1 < minX {
			minX = x1
		}
		if x1 > maxX {
			maxX = x1
		}
	}

	return minX, minY, maxX, maxY
}

// ActiveEdgeTable manages active edges during scanline conversion.
type ActiveEdgeTable struct {
	edges []ActiveEdge
}

// ActiveEdge holds an edge with its current X position.
type ActiveEdge struct {
	Edge *Edge
	X    float32 // Current X position at current scanline
}

// NewActiveEdgeTable creates a new active edge table.
func NewActiveEdgeTable() *ActiveEdgeTable {
	return &ActiveEdgeTable{
		edges: make([]ActiveEdge, 0, 32),
	}
}

// Reset clears the active edge table.
func (aet *ActiveEdgeTable) Reset() {
	aet.edges = aet.edges[:0]
}

// InsertEdge adds an edge to the active list.
func (aet *ActiveEdgeTable) InsertEdge(e *Edge, y float32) {
	ae := ActiveEdge{
		Edge: e,
		X:    e.XAtY(y),
	}

	// Insert in sorted order by X
	i := len(aet.edges)
	aet.edges = append(aet.edges, ae)
	for i > 0 && aet.edges[i-1].X > ae.X {
		aet.edges[i] = aet.edges[i-1]
		i--
	}
	aet.edges[i] = ae
}

// RemoveExpired removes edges that end at or before the given Y.
func (aet *ActiveEdgeTable) RemoveExpired(y float32) {
	j := 0
	for i := 0; i < len(aet.edges); i++ {
		if aet.edges[i].Edge.yMax > y {
			aet.edges[j] = aet.edges[i]
			j++
		}
	}
	aet.edges = aet.edges[:j]
}

// UpdateX updates X positions for all active edges at the new Y.
func (aet *ActiveEdgeTable) UpdateX(y float32) {
	for i := range aet.edges {
		aet.edges[i].X = aet.edges[i].Edge.XAtY(y)
	}
}

// SortByX sorts active edges by their current X position.
func (aet *ActiveEdgeTable) SortByX() {
	// Insertion sort (usually nearly sorted)
	for i := 1; i < len(aet.edges); i++ {
		j := i
		for j > 0 && aet.edges[j].X < aet.edges[j-1].X {
			aet.edges[j], aet.edges[j-1] = aet.edges[j-1], aet.edges[j]
			j--
		}
	}
}

// Active returns the list of active edges for iteration.
func (aet *ActiveEdgeTable) Active() []ActiveEdge {
	return aet.edges
}

// Len returns the number of active edges.
func (aet *ActiveEdgeTable) Len() int {
	return len(aet.edges)
}
