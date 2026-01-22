package raster

// Point represents a 2D point (internal copy to avoid import cycle).
type Point struct {
	X, Y float64
}

// Edge represents a line segment for scanline rasterization.
type Edge struct {
	x0, y0 float64 // Start point
	x1, y1 float64 // End point
	dx     float64 // dx/dy slope
	dir    int     // Direction: +1 or -1
}

// NewEdge creates a new edge from two points.
func NewEdge(p0, p1 Point) Edge {
	// Determine direction BEFORE swap (for non-zero winding rule)
	dir := 1
	if p0.Y > p1.Y {
		dir = -1
		p0, p1 = p1, p0 // Swap to ensure y0 < y1
	}

	dy := p1.Y - p0.Y
	var dx float64
	if dy != 0 {
		dx = (p1.X - p0.X) / dy
	}

	return Edge{
		x0:  p0.X,
		y0:  p0.Y,
		x1:  p1.X,
		y1:  p1.Y,
		dx:  dx,
		dir: dir,
	}
}

// XAtY calculates the x coordinate at the given y coordinate.
func (e *Edge) XAtY(y float64) float64 {
	if e.y1 == e.y0 {
		return e.x0
	}
	t := (y - e.y0) / (e.y1 - e.y0)
	return e.x0 + (e.x1-e.x0)*t
}

// ActiveEdgeTable represents edges active at a scanline.
type ActiveEdgeTable struct {
	edges []ActiveEdge
}

// ActiveEdge is an edge being processed by the rasterizer.
type ActiveEdge struct {
	x      float64 // Current x position
	dx     float64 // Change in x per scanline
	yMax   float64 // Maximum y (when edge becomes inactive)
	dir    int     // Direction for winding
	active bool    // Whether this edge is active
}

// NewActiveEdgeTable creates a new active edge table.
func NewActiveEdgeTable() *ActiveEdgeTable {
	return &ActiveEdgeTable{
		edges: make([]ActiveEdge, 0, 32),
	}
}

// Add adds an edge to the active edge table.
func (aet *ActiveEdgeTable) Add(edge Edge) {
	aet.edges = append(aet.edges, ActiveEdge{
		x:      edge.x0,
		dx:     edge.dx,
		yMax:   edge.y1,
		dir:    edge.dir,
		active: true,
	})
}

// AddAtY adds an edge to the active edge table with x computed for the given y.
func (aet *ActiveEdgeTable) AddAtY(edge Edge, y float64) {
	// Calculate x position at this y coordinate
	x := edge.XAtY(y)
	aet.edges = append(aet.edges, ActiveEdge{
		x:      x,
		dx:     edge.dx,
		yMax:   edge.y1,
		dir:    edge.dir,
		active: true,
	})
}

// Remove removes inactive edges for the given scanline.
func (aet *ActiveEdgeTable) Remove(y float64) {
	// Mark inactive edges
	for i := range aet.edges {
		if aet.edges[i].active && y >= aet.edges[i].yMax {
			aet.edges[i].active = false
		}
	}

	// Compact the slice
	j := 0
	for i := range aet.edges {
		if aet.edges[i].active {
			aet.edges[j] = aet.edges[i]
			j++
		}
	}
	aet.edges = aet.edges[:j]
}

// Update updates x positions for the next scanline.
func (aet *ActiveEdgeTable) Update() {
	for i := range aet.edges {
		aet.edges[i].x += aet.edges[i].dx
	}
}

// Sort sorts edges by x coordinate (insertion sort for small lists).
func (aet *ActiveEdgeTable) Sort() {
	for i := 1; i < len(aet.edges); i++ {
		key := aet.edges[i]
		j := i - 1
		for j >= 0 && aet.edges[j].x > key.x {
			aet.edges[j+1] = aet.edges[j]
			j--
		}
		aet.edges[j+1] = key
	}
}

// Edges returns the active edges.
func (aet *ActiveEdgeTable) Edges() []ActiveEdge {
	return aet.edges
}

// Clear clears all edges.
func (aet *ActiveEdgeTable) Clear() {
	aet.edges = aet.edges[:0]
}
