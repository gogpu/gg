//go:build !nogpu

package gpu

import (
	"math"

	"github.com/gogpu/gg"
)

// fanFlattenTolerance is the maximum allowed deviation between a curve and its
// linear approximation, in pixels. Smaller values produce more triangles but
// smoother curves. 0.25 provides sub-pixel accuracy suitable for GPU rendering.
const fanFlattenTolerance = 0.25

// fanCoverPadding is the number of pixels added around the AABB when
// generating the cover quad. This padding ensures that anti-aliased edges
// at the path boundary are fully covered during the cover pass.
const fanCoverPadding = 1.0

// fanInitialVertexCapacity is the initial capacity of the vertex slice,
// measured in float32 values (not triangles). 6 floats = 1 triangle.
// 256 floats = ~42 triangles, a reasonable starting size for typical paths.
const fanInitialVertexCapacity = 256

// FanTessellator converts path data into triangle fan vertices for stencil fill.
//
// For each contour in the path, it picks the first vertex (v0) as the fan center
// and emits triangles (v0, vi, vi+1) for every subsequent edge. Cubic and
// quadratic Bezier curves are adaptively flattened to line segments before
// triangulation.
//
// This is an O(n) algorithm that works correctly for any path topology
// (concave, self-intersecting, paths with holes) because the stencil buffer
// handles winding number correctness during the stencil pass.
//
// The tessellator is designed to be reused across frames via Reset().
type FanTessellator struct {
	// vertices holds x, y pairs for fan triangles.
	// Every 6 consecutive floats represent one triangle (3 vertices x 2 coords).
	vertices []float32

	// bounds is the axis-aligned bounding box: [minX, minY, maxX, maxY].
	bounds [4]float32

	// hasBounds tracks whether any vertex has been added to the bounds.
	hasBounds bool
}

// NewFanTessellator creates a new tessellator with pre-allocated capacity.
func NewFanTessellator() *FanTessellator {
	return &FanTessellator{
		vertices: make([]float32, 0, fanInitialVertexCapacity),
	}
}

// Reset clears the tessellator state for reuse without releasing memory.
func (ft *FanTessellator) Reset() {
	ft.vertices = ft.vertices[:0]
	ft.bounds = [4]float32{}
	ft.hasBounds = false
}

// TessellatePath converts a sequence of path elements into triangle fan vertices.
//
// For each contour (started by MoveTo), the first vertex becomes the fan center.
// Subsequent line/curve segments are flattened and triangulated as fan triangles
// from the center vertex. Close elements generate a closing triangle back to the
// contour start.
//
// Returns the total number of vertices emitted. Each triangle uses 3 vertices
// (6 floats), so the triangle count is vertexCount/3.
func (ft *FanTessellator) TessellatePath(elements []gg.PathElement) int {
	if len(elements) == 0 {
		return 0
	}

	var (
		fanOriginX, fanOriginY float64 // First vertex of current contour (fan center)
		prevX, prevY           float64 // Previous vertex
		contourStarted         bool    // Whether we have a fan origin
	)

	for _, elem := range elements {
		switch e := elem.(type) {
		case gg.MoveTo:
			// Close previous contour implicitly if needed
			// (no closing triangle — path was not explicitly closed)
			fanOriginX, fanOriginY = e.Point.X, e.Point.Y
			prevX, prevY = e.Point.X, e.Point.Y
			contourStarted = true
			ft.updateBounds(float32(e.Point.X), float32(e.Point.Y))

		case gg.LineTo:
			if !contourStarted {
				continue
			}
			ft.updateBounds(float32(e.Point.X), float32(e.Point.Y))
			ft.emitFanTriangle(fanOriginX, fanOriginY, prevX, prevY, e.Point.X, e.Point.Y)
			prevX, prevY = e.Point.X, e.Point.Y

		case gg.QuadTo:
			if !contourStarted {
				continue
			}
			ft.flattenQuadFan(
				fanOriginX, fanOriginY,
				prevX, prevY,
				e.Control.X, e.Control.Y,
				e.Point.X, e.Point.Y,
				fanFlattenTolerance,
			)
			prevX, prevY = e.Point.X, e.Point.Y

		case gg.CubicTo:
			if !contourStarted {
				continue
			}
			ft.flattenCubicFan(
				fanOriginX, fanOriginY,
				prevX, prevY,
				e.Control1.X, e.Control1.Y,
				e.Control2.X, e.Control2.Y,
				e.Point.X, e.Point.Y,
				fanFlattenTolerance,
			)
			prevX, prevY = e.Point.X, e.Point.Y

		case gg.Close:
			if !contourStarted {
				continue
			}
			// Emit closing triangle if the last point differs from the fan origin
			if prevX != fanOriginX || prevY != fanOriginY {
				ft.emitFanTriangle(fanOriginX, fanOriginY, prevX, prevY, fanOriginX, fanOriginY)
			}
			prevX, prevY = fanOriginX, fanOriginY
			contourStarted = false
		}
	}

	return len(ft.vertices) / 2
}

// Vertices returns the raw vertex buffer data as x,y float32 pairs.
// Every 6 consecutive values represent one triangle (3 vertices).
func (ft *FanTessellator) Vertices() []float32 {
	return ft.vertices
}

// Bounds returns the axis-aligned bounding box of all tessellated vertices.
// Format: [minX, minY, maxX, maxY]. Returns zeroes if no vertices were emitted.
func (ft *FanTessellator) Bounds() [4]float32 {
	return ft.bounds
}

// CoverQuad returns 6 vertices (2 triangles) forming a rectangle that covers
// the entire path bounding box plus fanCoverPadding pixels of padding.
// The padding ensures anti-aliased edges at the boundary are fully rendered.
//
// Triangle layout (counter-clockwise):
//
//	Triangle 1: (minX, minY), (maxX, minY), (maxX, maxY)
//	Triangle 2: (minX, minY), (maxX, maxY), (minX, maxY)
func (ft *FanTessellator) CoverQuad() [12]float32 {
	minX := ft.bounds[0] - fanCoverPadding
	minY := ft.bounds[1] - fanCoverPadding
	maxX := ft.bounds[2] + fanCoverPadding
	maxY := ft.bounds[3] + fanCoverPadding

	return [12]float32{
		// Triangle 1
		minX, minY, maxX, minY, maxX, maxY,
		// Triangle 2
		minX, minY, maxX, maxY, minX, maxY,
	}
}

// TriangleCount returns the number of triangles in the tessellated output.
func (ft *FanTessellator) TriangleCount() int {
	return len(ft.vertices) / 6
}

// emitFanTriangle appends a single triangle (v0, v1, v2) to the vertex buffer.
// Degenerate triangles (zero area) are skipped.
func (ft *FanTessellator) emitFanTriangle(v0x, v0y, v1x, v1y, v2x, v2y float64) {
	// Skip degenerate triangles using cross product (2x area)
	ax, ay := v1x-v0x, v1y-v0y
	bx, by := v2x-v0x, v2y-v0y
	cross := ax*by - ay*bx
	if cross == 0 {
		return
	}

	ft.vertices = append(ft.vertices,
		float32(v0x), float32(v0y),
		float32(v1x), float32(v1y),
		float32(v2x), float32(v2y),
	)
}

// updateBounds expands the AABB to include the given point.
func (ft *FanTessellator) updateBounds(x, y float32) {
	if !ft.hasBounds {
		ft.bounds = [4]float32{x, y, x, y}
		ft.hasBounds = true
		return
	}
	if x < ft.bounds[0] {
		ft.bounds[0] = x
	}
	if y < ft.bounds[1] {
		ft.bounds[1] = y
	}
	if x > ft.bounds[2] {
		ft.bounds[2] = x
	}
	if y > ft.bounds[3] {
		ft.bounds[3] = y
	}
}

// flattenQuadFan flattens a quadratic Bezier curve and emits fan triangles.
// Uses recursive de Casteljau subdivision with the specified tolerance.
func (ft *FanTessellator) flattenQuadFan(
	fanX, fanY, x0, y0, cx, cy, x1, y1, tol float64,
) {
	// Check flatness: deviation of control point from chord midpoint
	midX := 0.25*x0 + 0.5*cx + 0.25*x1
	midY := 0.25*y0 + 0.5*cy + 0.25*y1
	chordMidX := 0.5 * (x0 + x1)
	chordMidY := 0.5 * (y0 + y1)

	dx := midX - chordMidX
	dy := midY - chordMidY
	distSq := dx*dx + dy*dy

	if distSq <= tol*tol {
		// Flat enough: emit single fan triangle
		ft.updateBounds(float32(x1), float32(y1))
		ft.emitFanTriangle(fanX, fanY, x0, y0, x1, y1)
		return
	}

	// De Casteljau subdivision at t=0.5
	ax := 0.5 * (x0 + cx)
	ay := 0.5 * (y0 + cy)
	bx := 0.5 * (cx + x1)
	by := 0.5 * (cy + y1)
	mx := 0.5 * (ax + bx)
	my := 0.5 * (ay + by)

	ft.flattenQuadFan(fanX, fanY, x0, y0, ax, ay, mx, my, tol)
	ft.flattenQuadFan(fanX, fanY, mx, my, bx, by, x1, y1, tol)
}

// flattenCubicFan flattens a cubic Bezier curve and emits fan triangles.
// Uses the standard cubic flatness test: both control points must be within
// tolerance of the chord line. The factor of 16 accounts for the cubic
// approximation error bound.
func (ft *FanTessellator) flattenCubicFan(
	fanX, fanY, x0, y0, c1x, c1y, c2x, c2y, x1, y1, tol float64,
) {
	// Cubic flatness test: check control point deviation from chord
	ux := 3*c1x - 2*x0 - x1
	uy := 3*c1y - 2*y0 - y1
	vx := 3*c2x - x0 - 2*x1
	vy := 3*c2y - y0 - 2*y1

	distSq := math.Max(ux*ux+uy*uy, vx*vx+vy*vy)

	// Factor of 16 for cubic approximation error bound
	if distSq <= 16*tol*tol {
		// Flat enough: emit single fan triangle
		ft.updateBounds(float32(x1), float32(y1))
		ft.emitFanTriangle(fanX, fanY, x0, y0, x1, y1)
		return
	}

	// De Casteljau subdivision at t=0.5
	ab1x := 0.5 * (x0 + c1x)
	ab1y := 0.5 * (y0 + c1y)
	ab2x := 0.5 * (c1x + c2x)
	ab2y := 0.5 * (c1y + c2y)
	ab3x := 0.5 * (c2x + x1)
	ab3y := 0.5 * (c2y + y1)

	bc1x := 0.5 * (ab1x + ab2x)
	bc1y := 0.5 * (ab1y + ab2y)
	bc2x := 0.5 * (ab2x + ab3x)
	bc2y := 0.5 * (ab2y + ab3y)

	mx := 0.5 * (bc1x + bc2x)
	my := 0.5 * (bc1y + bc2y)

	ft.flattenCubicFan(fanX, fanY, x0, y0, ab1x, ab1y, bc1x, bc1y, mx, my, tol)
	ft.flattenCubicFan(fanX, fanY, mx, my, bc2x, bc2y, ab3x, ab3y, x1, y1, tol)
}
