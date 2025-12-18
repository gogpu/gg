package wgpu

import (
	"math"
	"sync"

	"github.com/gogpu/gg/scene"
)

// Tessellator converts paths to sparse strips for GPU rendering.
// It uses scanline conversion with the Active Edge Table algorithm
// to produce anti-aliased coverage strips.
type Tessellator struct {
	// Edge collection from flattened path
	edges *EdgeList

	// Active edge table for scanline processing
	aet *ActiveEdgeTable

	// Coverage accumulator for current scanline
	coverage []float32

	// Temporary buffer for anti-aliased coverage
	aaCoverage []uint8

	// Output buffer
	buffer *StripBuffer

	// Configuration
	fillRule  scene.FillStyle
	antiAlias bool

	// Curve flattening tolerance (in pixels)
	flattenTolerance float32

	// Working state
	pathStartX, pathStartY float32
	cursorX, cursorY       float32
	hasPath                bool
}

// NewTessellator creates a new tessellator.
func NewTessellator() *Tessellator {
	return &Tessellator{
		edges:            NewEdgeList(),
		aet:              NewActiveEdgeTable(),
		coverage:         make([]float32, 4096),
		aaCoverage:       make([]uint8, 4096),
		buffer:           NewStripBuffer(),
		fillRule:         scene.FillNonZero,
		antiAlias:        true,
		flattenTolerance: 0.25, // Quarter pixel tolerance
	}
}

// SetFillRule sets the fill rule for the tessellator.
func (t *Tessellator) SetFillRule(rule scene.FillStyle) {
	t.fillRule = rule
}

// SetAntiAlias enables or disables anti-aliasing.
func (t *Tessellator) SetAntiAlias(aa bool) {
	t.antiAlias = aa
}

// SetFlattenTolerance sets the curve flattening tolerance.
func (t *Tessellator) SetFlattenTolerance(tolerance float32) {
	t.flattenTolerance = tolerance
}

// Reset clears the tessellator for reuse.
func (t *Tessellator) Reset() {
	t.edges.Reset()
	t.aet.Reset()
	t.buffer.Reset()
	t.hasPath = false
}

// TessellatePath converts a path to strips.
// The path is transformed by the given affine transformation.
func (t *Tessellator) TessellatePath(path *scene.Path, transform scene.Affine) *StripBuffer {
	t.Reset()
	t.buffer.SetFillRule(t.fillRule)

	if path == nil || path.IsEmpty() {
		return t.buffer
	}

	// Step 1: Flatten path to edges
	t.flattenPath(path, transform)

	if t.edges.Len() == 0 {
		return t.buffer
	}

	// Step 2: Sort edges by yMin
	t.edges.SortByYMin()

	// Step 3: Scanline conversion
	t.scanlineConvert()

	return t.buffer
}

// flattenPath converts path curves to line segments and builds the edge list.
func (t *Tessellator) flattenPath(path *scene.Path, transform scene.Affine) {
	verbs := path.Verbs()
	points := path.Points()
	pointIdx := 0

	for _, verb := range verbs {
		switch verb {
		case scene.VerbMoveTo:
			t.flushSubpath()
			x, y := points[pointIdx], points[pointIdx+1]
			if !transform.IsIdentity() {
				x, y = transform.TransformPoint(x, y)
			}
			t.pathStartX, t.pathStartY = x, y
			t.cursorX, t.cursorY = x, y
			t.hasPath = true
			pointIdx += 2

		case scene.VerbLineTo:
			x, y := points[pointIdx], points[pointIdx+1]
			if !transform.IsIdentity() {
				x, y = transform.TransformPoint(x, y)
			}
			t.addLineEdge(t.cursorX, t.cursorY, x, y)
			t.cursorX, t.cursorY = x, y
			pointIdx += 2

		case scene.VerbQuadTo:
			cx, cy := points[pointIdx], points[pointIdx+1]
			x, y := points[pointIdx+2], points[pointIdx+3]
			if !transform.IsIdentity() {
				cx, cy = transform.TransformPoint(cx, cy)
				x, y = transform.TransformPoint(x, y)
			}
			t.flattenQuad(t.cursorX, t.cursorY, cx, cy, x, y)
			t.cursorX, t.cursorY = x, y
			pointIdx += 4

		case scene.VerbCubicTo:
			c1x, c1y := points[pointIdx], points[pointIdx+1]
			c2x, c2y := points[pointIdx+2], points[pointIdx+3]
			x, y := points[pointIdx+4], points[pointIdx+5]
			if !transform.IsIdentity() {
				c1x, c1y = transform.TransformPoint(c1x, c1y)
				c2x, c2y = transform.TransformPoint(c2x, c2y)
				x, y = transform.TransformPoint(x, y)
			}
			t.flattenCubic(t.cursorX, t.cursorY, c1x, c1y, c2x, c2y, x, y)
			t.cursorX, t.cursorY = x, y
			pointIdx += 6

		case scene.VerbClose:
			t.addLineEdge(t.cursorX, t.cursorY, t.pathStartX, t.pathStartY)
			t.cursorX, t.cursorY = t.pathStartX, t.pathStartY
		}
	}

	// Close any open path
	t.flushSubpath()
}

// flushSubpath closes an implicit subpath if needed.
func (t *Tessellator) flushSubpath() {
	if t.hasPath {
		// If cursor isn't at start, close the path implicitly
		dx := t.cursorX - t.pathStartX
		dy := t.cursorY - t.pathStartY
		if dx*dx+dy*dy > epsilon*epsilon {
			t.addLineEdge(t.cursorX, t.cursorY, t.pathStartX, t.pathStartY)
		}
		t.hasPath = false
	}
}

// addLineEdge adds a line segment as an edge.
func (t *Tessellator) addLineEdge(x0, y0, x1, y1 float32) {
	t.edges.AddLine(x0, y0, x1, y1)
}

// flattenQuad flattens a quadratic Bezier curve to line segments.
func (t *Tessellator) flattenQuad(x0, y0, cx, cy, x1, y1 float32) {
	// Estimate number of subdivisions needed
	// Based on maximum deviation from the control point
	dx := x0 - 2*cx + x1
	dy := y0 - 2*cy + y1
	dd := float32(math.Sqrt(float64(dx*dx + dy*dy)))

	n := 1
	if dd > t.flattenTolerance {
		n = int(math.Ceil(math.Sqrt(float64(dd / t.flattenTolerance))))
	}
	if n > 100 {
		n = 100 // Sanity limit
	}

	// Subdivide
	prevX, prevY := x0, y0
	dt := 1.0 / float32(n)

	for i := 1; i <= n; i++ {
		tt := float32(i) * dt
		tt2 := tt * tt
		mt := 1 - tt
		mt2 := mt * mt

		x := mt2*x0 + 2*mt*tt*cx + tt2*x1
		y := mt2*y0 + 2*mt*tt*cy + tt2*y1

		t.addLineEdge(prevX, prevY, x, y)
		prevX, prevY = x, y
	}
}

// flattenCubic flattens a cubic Bezier curve to line segments.
func (t *Tessellator) flattenCubic(x0, y0, c1x, c1y, c2x, c2y, x1, y1 float32) {
	// Estimate flatness using control point deviation
	// Maximum deviation from line connecting endpoints
	dx1 := c1x - x0 - (x1-x0)/3
	dy1 := c1y - y0 - (y1-y0)/3
	dx2 := c2x - x0 - 2*(x1-x0)/3
	dy2 := c2y - y0 - 2*(y1-y0)/3

	dd := float32(math.Max(
		math.Sqrt(float64(dx1*dx1+dy1*dy1)),
		math.Sqrt(float64(dx2*dx2+dy2*dy2)),
	))

	n := 1
	if dd > t.flattenTolerance {
		n = int(math.Ceil(math.Pow(float64(dd/t.flattenTolerance), 1.0/3.0)))
	}
	if n > 100 {
		n = 100 // Sanity limit
	}

	// Subdivide
	prevX, prevY := x0, y0
	dt := 1.0 / float32(n)

	for i := 1; i <= n; i++ {
		tt := float32(i) * dt
		tt2 := tt * tt
		tt3 := tt2 * tt
		mt := 1 - tt
		mt2 := mt * mt
		mt3 := mt2 * mt

		x := mt3*x0 + 3*mt2*tt*c1x + 3*mt*tt2*c2x + tt3*x1
		y := mt3*y0 + 3*mt2*tt*c1y + 3*mt*tt2*c2y + tt3*y1

		t.addLineEdge(prevX, prevY, x, y)
		prevX, prevY = x, y
	}
}

// scanlineConvert processes all scanlines and generates strips.
func (t *Tessellator) scanlineConvert() {
	// Get bounds
	minX, minY, maxX, maxY := t.edges.Bounds()

	// Convert to pixel coordinates
	startY := int(math.Floor(float64(minY)))
	endY := int(math.Ceil(float64(maxY)))
	startX := int(math.Floor(float64(minX)))
	endX := int(math.Ceil(float64(maxX)))

	// Ensure coverage buffer is large enough
	width := endX - startX + 1
	if width > len(t.coverage) {
		t.coverage = make([]float32, width*2)
		t.aaCoverage = make([]uint8, width*2)
	}

	// Process each scanline
	edgeIdx := 0
	edges := t.edges.Edges()

	for y := startY; y < endY; y++ {
		yf := float32(y)
		yCenterF := yf + 0.5

		// Add new edges that start at or before this scanline
		for edgeIdx < len(edges) && edges[edgeIdx].yMin <= yCenterF {
			if edges[edgeIdx].yMax > yCenterF {
				t.aet.InsertEdge(&edges[edgeIdx], yCenterF)
			}
			edgeIdx++
		}

		// Skip if no active edges
		if t.aet.Len() == 0 {
			continue
		}

		// Update X positions and sort
		t.aet.UpdateX(yCenterF)
		t.aet.SortByX()

		// Calculate coverage for this scanline
		if t.antiAlias {
			t.calculateAAcoverage(y, startX, endX)
		} else {
			t.calculateCoverage(y, startX, endX)
		}

		// Extract strips from coverage
		t.extractStrips(y, startX, endX)

		// Remove edges that end at or before the next scanline
		t.aet.RemoveExpired(yf + 1)
	}
}

// calculateCoverage calculates non-anti-aliased coverage for a scanline.
func (t *Tessellator) calculateCoverage(_, startX, endX int) {
	width := endX - startX + 1

	// Clear coverage
	for i := 0; i < width; i++ {
		t.coverage[i] = 0
	}

	active := t.aet.Active()
	winding := 0

	// Process edge pairs
	for i := 0; i < len(active); i++ {
		ae := &active[i]
		winding += int(ae.Edge.winding)

		// Calculate coverage based on fill rule
		var covered bool
		if t.fillRule == scene.FillNonZero {
			covered = winding != 0
		} else { // EvenOdd
			covered = (winding & 1) != 0
		}

		if covered && i+1 < len(active) {
			// Fill from current edge to next edge
			x0 := int(ae.X) - startX
			x1 := int(active[i+1].X) - startX

			if x0 < 0 {
				x0 = 0
			}
			if x1 > width {
				x1 = width
			}

			for x := x0; x < x1; x++ {
				t.coverage[x] = 255
			}
		}
	}
}

// calculateAAcoverage calculates anti-aliased coverage for a scanline.
func (t *Tessellator) calculateAAcoverage(y, startX, endX int) {
	width := endX - startX + 1
	yCenterF := float32(y) + 0.5

	// Clear coverage
	for i := 0; i < width; i++ {
		t.coverage[i] = 0
	}

	active := t.aet.Active()
	winding := 0
	prevX := float32(startX)

	for i := 0; i < len(active); i++ {
		ae := &active[i]
		edgeX := ae.X

		// Apply winding from previous filled region
		if t.fillRule == scene.FillNonZero {
			if winding != 0 {
				t.fillCoverageRange(prevX, edgeX, startX, width)
			}
		} else { // EvenOdd
			if (winding & 1) != 0 {
				t.fillCoverageRange(prevX, edgeX, startX, width)
			}
		}

		// Add anti-aliased edge coverage
		t.addEdgeCoverage(ae.Edge, yCenterF, startX, width)

		winding += int(ae.Edge.winding)
		prevX = edgeX
	}

	// Fill to end if still inside
	if t.fillRule == scene.FillNonZero {
		if winding != 0 {
			t.fillCoverageRange(prevX, float32(endX), startX, width)
		}
	} else { // EvenOdd
		if (winding & 1) != 0 {
			t.fillCoverageRange(prevX, float32(endX), startX, width)
		}
	}
}

// fillCoverageRange fills coverage from x0 to x1 with full coverage (255).
func (t *Tessellator) fillCoverageRange(x0, x1 float32, startX, width int) {
	ix0 := int(math.Ceil(float64(x0))) - startX
	ix1 := int(math.Floor(float64(x1))) - startX

	if ix0 < 0 {
		ix0 = 0
	}
	if ix1 > width {
		ix1 = width
	}

	for x := ix0; x < ix1; x++ {
		t.coverage[x] = 255
	}
}

// addEdgeCoverage adds anti-aliased coverage for an edge crossing.
func (t *Tessellator) addEdgeCoverage(edge *Edge, y float32, startX, width int) {
	// Calculate exact X position at scanline center
	edgeX := edge.XAtY(y)

	// Get pixel index
	pixelX := int(math.Floor(float64(edgeX))) - startX
	if pixelX < 0 || pixelX >= width {
		return
	}

	// Calculate sub-pixel coverage
	frac := edgeX - float32(math.Floor(float64(edgeX)))

	// The coverage represents how much of the pixel is inside the shape
	// For a left-to-right edge (positive winding), right part is covered
	// For a right-to-left edge (negative winding), left part is covered
	var coverage float32
	if edge.winding > 0 {
		// Entering shape: coverage increases from left to right
		coverage = (1 - frac) * 255
	} else {
		// Exiting shape: coverage decreases from left to right
		coverage = frac * 255
	}

	// Blend with existing coverage
	t.coverage[pixelX] = float32(math.Max(float64(t.coverage[pixelX]), float64(coverage)))
}

// extractStrips extracts non-zero coverage spans as strips.
func (t *Tessellator) extractStrips(y, startX, endX int) {
	width := endX - startX + 1

	// Convert float coverage to uint8
	for i := 0; i < width; i++ {
		cov := t.coverage[i]
		if cov > 255 {
			cov = 255
		}
		if cov < 0 {
			cov = 0
		}
		t.aaCoverage[i] = uint8(cov)
	}

	// Find contiguous non-zero spans
	spanStart := -1
	for x := 0; x < width; x++ {
		if t.aaCoverage[x] > 0 {
			if spanStart < 0 {
				spanStart = x
			}
		} else if spanStart >= 0 {
			// End of span
			t.emitStrip(y, spanStart+startX, t.aaCoverage[spanStart:x])
			spanStart = -1
		}
	}

	// Emit final span if we ended inside one
	if spanStart >= 0 {
		// Find actual end (might have trailing zeros)
		spanEnd := width
		for spanEnd > spanStart && t.aaCoverage[spanEnd-1] == 0 {
			spanEnd--
		}
		if spanEnd > spanStart {
			t.emitStrip(y, spanStart+startX, t.aaCoverage[spanStart:spanEnd])
		}
	}
}

// emitStrip adds a strip to the output buffer.
func (t *Tessellator) emitStrip(y, x int, coverage []uint8) {
	if len(coverage) == 0 {
		return
	}
	t.buffer.AddStrip(y, x, coverage)
}

// TessellateRect is a convenience method for tessellating a rectangle.
func (t *Tessellator) TessellateRect(x, y, w, h float32) *StripBuffer {
	path := scene.NewPath().Rectangle(x, y, w, h)
	return t.TessellatePath(path, scene.IdentityAffine())
}

// TessellateCircle is a convenience method for tessellating a circle.
func (t *Tessellator) TessellateCircle(cx, cy, r float32) *StripBuffer {
	path := scene.NewPath().Circle(cx, cy, r)
	return t.TessellatePath(path, scene.IdentityAffine())
}

// TessellatorPool manages a pool of reusable tessellators.
// It is safe for concurrent use.
type TessellatorPool struct {
	mu   sync.Mutex
	pool []*Tessellator
}

// NewTessellatorPool creates a new tessellator pool.
func NewTessellatorPool() *TessellatorPool {
	return &TessellatorPool{
		pool: make([]*Tessellator, 0, 4),
	}
}

// Get retrieves a tessellator from the pool or creates a new one.
func (tp *TessellatorPool) Get() *Tessellator {
	tp.mu.Lock()
	if len(tp.pool) > 0 {
		t := tp.pool[len(tp.pool)-1]
		tp.pool = tp.pool[:len(tp.pool)-1]
		tp.mu.Unlock()
		t.Reset()
		return t
	}
	tp.mu.Unlock()
	return NewTessellator()
}

// Put returns a tessellator to the pool.
func (tp *TessellatorPool) Put(t *Tessellator) {
	if t == nil {
		return
	}
	t.Reset()
	tp.mu.Lock()
	tp.pool = append(tp.pool, t)
	tp.mu.Unlock()
}
