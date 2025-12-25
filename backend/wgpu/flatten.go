package wgpu

import (
	"math"

	"github.com/gogpu/gg/scene"
)

// FlattenTolerance is the default tolerance for curve flattening.
// Smaller values produce more accurate curves but more segments.
const FlattenTolerance = 0.25

// FlattenPath flattens a path to monotonic line segments.
// It converts Bezier curves to line segments and ensures all segments
// are monotonic in Y (Y0 <= Y1).
//
// Parameters:
//   - path: The input path to flatten
//   - transform: Affine transformation to apply to all points
//   - tolerance: Flattening tolerance (use FlattenTolerance for default)
//
// Returns a SegmentList containing all flattened line segments.
//
//nolint:dupl // Duplicated with FlattenPathTo for API convenience - avoids callback overhead
func FlattenPath(path *scene.Path, transform scene.Affine, tolerance float32) *SegmentList {
	segments := NewSegmentList()

	if path == nil || path.IsEmpty() {
		return segments
	}

	// Flattening state
	var curX, curY float32     // Current cursor position
	var startX, startY float32 // Subpath start position

	pointIdx := 0
	points := path.Points()
	verbs := path.Verbs()

	for _, verb := range verbs {
		switch verb {
		case scene.VerbMoveTo:
			// Close previous subpath if needed
			if curX != startX || curY != startY {
				addMonotonicLine(segments, curX, curY, startX, startY)
			}
			// Transform and set new position
			x, y := points[pointIdx], points[pointIdx+1]
			curX, curY = transform.TransformPoint(x, y)
			startX, startY = curX, curY
			pointIdx += 2

		case scene.VerbLineTo:
			x, y := points[pointIdx], points[pointIdx+1]
			nextX, nextY := transform.TransformPoint(x, y)
			addMonotonicLine(segments, curX, curY, nextX, nextY)
			curX, curY = nextX, nextY
			pointIdx += 2

		case scene.VerbQuadTo:
			// Control point and end point
			cx, cy := points[pointIdx], points[pointIdx+1]
			x, y := points[pointIdx+2], points[pointIdx+3]
			// Transform all points
			tcx, tcy := transform.TransformPoint(cx, cy)
			tx, ty := transform.TransformPoint(x, y)
			// Flatten quadratic curve
			flattenQuadratic(segments, curX, curY, tcx, tcy, tx, ty, tolerance)
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
			// Flatten cubic curve
			flattenCubic(segments, curX, curY, tc1x, tc1y, tc2x, tc2y, tx, ty, tolerance)
			curX, curY = tx, ty
			pointIdx += 6

		case scene.VerbClose:
			// Close the subpath
			if curX != startX || curY != startY {
				addMonotonicLine(segments, curX, curY, startX, startY)
			}
			curX, curY = startX, startY
		}
	}

	// Close final subpath if not explicitly closed
	if curX != startX || curY != startY {
		addMonotonicLine(segments, curX, curY, startX, startY)
	}

	return segments
}

// addMonotonicLine adds a line segment, splitting if non-monotonic in Y.
// A segment is monotonic if it always goes down (Y0 <= Y1).
func addMonotonicLine(segments *SegmentList, x0, y0, x1, y1 float32) {
	// Determine winding from original direction
	// +1 for downward (Y increases), -1 for upward (Y decreases)
	var winding int8 = 1
	if y1 < y0 {
		winding = -1
	}

	// Skip degenerate segments (points or horizontal lines)
	const epsilon = 1e-6
	dy := y1 - y0
	dx := x1 - x0
	if absf32(dy) < epsilon && absf32(dx) < epsilon {
		return
	}
	// Skip horizontal segments - they don't contribute to winding
	if absf32(dy) < epsilon {
		return
	}

	// Add the segment (constructor will ensure Y0 <= Y1)
	segments.AddLine(x0, y0, x1, y1, winding)
}

// flattenQuadratic flattens a quadratic Bezier curve to line segments.
// Uses recursive subdivision based on flatness criterion.
func flattenQuadratic(segments *SegmentList, x0, y0, cx, cy, x1, y1 float32, tol float32) {
	// Check if flat enough using the midpoint deviation
	// For a quadratic, max deviation is at t=0.5
	midX := 0.25*x0 + 0.5*cx + 0.25*x1
	midY := 0.25*y0 + 0.5*cy + 0.25*y1
	chordMidX := 0.5 * (x0 + x1)
	chordMidY := 0.5 * (y0 + y1)

	dx := midX - chordMidX
	dy := midY - chordMidY
	distSq := dx*dx + dy*dy

	tolSq := tol * tol
	if distSq <= tolSq {
		// Flat enough, add as line
		addMonotonicLine(segments, x0, y0, x1, y1)
		return
	}

	// Check for Y-monotonicity and split at extrema
	// For quadratic: extremum when dy/dt = 0
	// dy/dt = 2(1-t)(cy-y0) + 2t(y1-cy) = 0
	// Solving: t = (y0-cy) / (y0 - 2*cy + y1)
	denom := y0 - 2*cy + y1
	if absf32(denom) > flattenEpsilon {
		t := (y0 - cy) / denom
		if t > flattenEpsilon && t < 1.0-flattenEpsilon {
			// Split at extremum
			splitQuadraticAt(segments, x0, y0, cx, cy, x1, y1, t, tol)
			return
		}
	}

	// Subdivide at midpoint
	subdivideQuadratic(segments, x0, y0, cx, cy, x1, y1, tol)
}

// splitQuadraticAt splits a quadratic at parameter t and flattens both parts.
func splitQuadraticAt(segments *SegmentList, x0, y0, cx, cy, x1, y1, t float32, tol float32) {
	// De Casteljau subdivision
	// First level
	ax := lerp(x0, cx, t)
	ay := lerp(y0, cy, t)
	bx := lerp(cx, x1, t)
	by := lerp(cy, y1, t)

	// Second level - the split point
	mx := lerp(ax, bx, t)
	my := lerp(ay, by, t)

	// Flatten both halves
	flattenQuadratic(segments, x0, y0, ax, ay, mx, my, tol)
	flattenQuadratic(segments, mx, my, bx, by, x1, y1, tol)
}

// subdivideQuadratic subdivides a quadratic at t=0.5 and flattens both parts.
func subdivideQuadratic(segments *SegmentList, x0, y0, cx, cy, x1, y1, tol float32) {
	// De Casteljau at t=0.5
	ax := 0.5 * (x0 + cx)
	ay := 0.5 * (y0 + cy)
	bx := 0.5 * (cx + x1)
	by := 0.5 * (cy + y1)
	mx := 0.5 * (ax + bx)
	my := 0.5 * (ay + by)

	// Flatten both halves
	flattenQuadratic(segments, x0, y0, ax, ay, mx, my, tol)
	flattenQuadratic(segments, mx, my, bx, by, x1, y1, tol)
}

// flattenCubic flattens a cubic Bezier curve to line segments.
// Uses recursive subdivision based on flatness criterion.
func flattenCubic(segments *SegmentList, x0, y0, c1x, c1y, c2x, c2y, x1, y1 float32, tol float32) {
	// Check if flat enough using control point deviation from chord
	// For a cubic, check both control points' distance from the chord
	ux := 3*c1x - 2*x0 - x1
	uy := 3*c1y - 2*y0 - y1
	vx := 3*c2x - x0 - 2*x1
	vy := 3*c2y - y0 - 2*y1

	distSq := maxf32(ux*ux+uy*uy, vx*vx+vy*vy)

	tolSq := 16 * tol * tol // Factor of 16 for cubic approximation
	if distSq <= tolSq {
		// Flat enough, add as line
		addMonotonicLine(segments, x0, y0, x1, y1)
		return
	}

	// Check for Y-monotonicity and split at extrema
	// For cubic: extremum when dy/dt = 0
	// dy/dt = 3(1-t)^2(c1y-y0) + 6(1-t)t(c2y-c1y) + 3t^2(y1-c2y)
	// This is a quadratic in t
	a := y0 - 3*c1y + 3*c2y - y1
	b := 2 * (c1y - 2*c2y + y1)
	c := c2y - y1

	extrema := findQuadraticRoots(a, b, c)
	//nolint:nestif // Nested structure reflects algorithm logic for root filtering
	if len(extrema) > 0 {
		// Sort and filter valid roots
		var validRoots []float32
		for _, t := range extrema {
			if t > flattenEpsilon && t < 1.0-flattenEpsilon {
				validRoots = append(validRoots, t)
			}
		}
		if len(validRoots) > 0 {
			// Sort roots
			if len(validRoots) == 2 && validRoots[0] > validRoots[1] {
				validRoots[0], validRoots[1] = validRoots[1], validRoots[0]
			}
			// Split at first extremum
			splitCubicAt(segments, x0, y0, c1x, c1y, c2x, c2y, x1, y1, validRoots[0], tol)
			return
		}
	}

	// Subdivide at midpoint
	subdivideCubic(segments, x0, y0, c1x, c1y, c2x, c2y, x1, y1, tol)
}

// splitCubicAt splits a cubic at parameter t and flattens both parts.
func splitCubicAt(segments *SegmentList, x0, y0, c1x, c1y, c2x, c2y, x1, y1, t float32, tol float32) {
	// De Casteljau subdivision for cubic
	// First level
	ax := lerp(x0, c1x, t)
	ay := lerp(y0, c1y, t)
	bx := lerp(c1x, c2x, t)
	by := lerp(c1y, c2y, t)
	cx := lerp(c2x, x1, t)
	cy := lerp(c2y, y1, t)

	// Second level
	dx := lerp(ax, bx, t)
	dy := lerp(ay, by, t)
	ex := lerp(bx, cx, t)
	ey := lerp(by, cy, t)

	// Third level - the split point
	mx := lerp(dx, ex, t)
	my := lerp(dy, ey, t)

	// Flatten both halves
	flattenCubic(segments, x0, y0, ax, ay, dx, dy, mx, my, tol)
	flattenCubic(segments, mx, my, ex, ey, cx, cy, x1, y1, tol)
}

// subdivideCubic subdivides a cubic at t=0.5 and flattens both parts.
func subdivideCubic(segments *SegmentList, x0, y0, c1x, c1y, c2x, c2y, x1, y1, tol float32) {
	// De Casteljau at t=0.5
	ax := 0.5 * (x0 + c1x)
	ay := 0.5 * (y0 + c1y)
	bx := 0.5 * (c1x + c2x)
	by := 0.5 * (c1y + c2y)
	cx := 0.5 * (c2x + x1)
	cy := 0.5 * (c2y + y1)

	dx := 0.5 * (ax + bx)
	dy := 0.5 * (ay + by)
	ex := 0.5 * (bx + cx)
	ey := 0.5 * (by + cy)

	mx := 0.5 * (dx + ex)
	my := 0.5 * (dy + ey)

	// Flatten both halves
	flattenCubic(segments, x0, y0, ax, ay, dx, dy, mx, my, tol)
	flattenCubic(segments, mx, my, ex, ey, cx, cy, x1, y1, tol)
}

// flattenEpsilon is a small value for floating point comparisons in flattening.
const flattenEpsilon = 1e-6

// findQuadraticRoots finds real roots of ax^2 + bx + c = 0.
func findQuadraticRoots(a, b, c float32) []float32 {
	if absf32(a) < flattenEpsilon {
		// Linear equation bx + c = 0
		if absf32(b) < flattenEpsilon {
			return nil
		}
		return []float32{-c / b}
	}

	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		return nil
	}

	sqrtD := float32(math.Sqrt(float64(discriminant)))
	inv2a := 1.0 / (2 * a)

	if discriminant < flattenEpsilon {
		// Single root
		return []float32{-b * inv2a}
	}

	// Two roots
	r1 := (-b - sqrtD) * inv2a
	r2 := (-b + sqrtD) * inv2a
	return []float32{r1, r2}
}

// lerp performs linear interpolation between a and b at parameter t.
func lerp(a, b, t float32) float32 {
	return a + t*(b-a)
}

// absf32 returns the absolute value of x.
func absf32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// maxf32 returns the maximum of a and b.
func maxf32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// minf32 returns the minimum of a and b.
func minf32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// FlattenContext provides reusable state for path flattening.
// Use this to reduce allocations when flattening multiple paths.
type FlattenContext struct {
	segments *SegmentList
}

// NewFlattenContext creates a new flattening context.
func NewFlattenContext() *FlattenContext {
	return &FlattenContext{
		segments: NewSegmentList(),
	}
}

// Reset clears the context for reuse.
func (ctx *FlattenContext) Reset() {
	ctx.segments.Reset()
}

// Segments returns the flattened segments.
func (ctx *FlattenContext) Segments() *SegmentList {
	return ctx.segments
}

// FlattenPathTo flattens a path into the context's segment list.
//
// This avoids allocating a new SegmentList for each path.
//
//nolint:dupl // Duplicated with FlattenPath for API convenience - avoids callback overhead
func (ctx *FlattenContext) FlattenPathTo(path *scene.Path, transform scene.Affine, tolerance float32) {
	if path == nil || path.IsEmpty() {
		return
	}

	segments := ctx.segments

	// Flattening state
	var curX, curY float32
	var startX, startY float32

	pointIdx := 0
	points := path.Points()
	verbs := path.Verbs()

	for _, verb := range verbs {
		switch verb {
		case scene.VerbMoveTo:
			if curX != startX || curY != startY {
				addMonotonicLine(segments, curX, curY, startX, startY)
			}
			x, y := points[pointIdx], points[pointIdx+1]
			curX, curY = transform.TransformPoint(x, y)
			startX, startY = curX, curY
			pointIdx += 2

		case scene.VerbLineTo:
			x, y := points[pointIdx], points[pointIdx+1]
			nextX, nextY := transform.TransformPoint(x, y)
			addMonotonicLine(segments, curX, curY, nextX, nextY)
			curX, curY = nextX, nextY
			pointIdx += 2

		case scene.VerbQuadTo:
			cx, cy := points[pointIdx], points[pointIdx+1]
			x, y := points[pointIdx+2], points[pointIdx+3]
			tcx, tcy := transform.TransformPoint(cx, cy)
			tx, ty := transform.TransformPoint(x, y)
			flattenQuadratic(segments, curX, curY, tcx, tcy, tx, ty, tolerance)
			curX, curY = tx, ty
			pointIdx += 4

		case scene.VerbCubicTo:
			c1x, c1y := points[pointIdx], points[pointIdx+1]
			c2x, c2y := points[pointIdx+2], points[pointIdx+3]
			x, y := points[pointIdx+4], points[pointIdx+5]
			tc1x, tc1y := transform.TransformPoint(c1x, c1y)
			tc2x, tc2y := transform.TransformPoint(c2x, c2y)
			tx, ty := transform.TransformPoint(x, y)
			flattenCubic(segments, curX, curY, tc1x, tc1y, tc2x, tc2y, tx, ty, tolerance)
			curX, curY = tx, ty
			pointIdx += 6

		case scene.VerbClose:
			if curX != startX || curY != startY {
				addMonotonicLine(segments, curX, curY, startX, startY)
			}
			curX, curY = startX, startY
		}
	}

	if curX != startX || curY != startY {
		addMonotonicLine(segments, curX, curY, startX, startY)
	}
}
