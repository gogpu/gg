package gg

import "math"

// Path operations for area calculation, winding number, containment testing,
// bounding box computation, flattening, and arc length measurement.

// Area returns the signed area enclosed by the path.
// Positive for clockwise paths, negative for counter-clockwise.
// Uses the shoelace formula extended for curves (Green's theorem).
// Only closed subpaths contribute to the area.
func (p *Path) Area() float64 {
	var area float64
	var current, start Point

	for _, elem := range p.elements {
		switch e := elem.(type) {
		case MoveTo:
			start = e.Point
			current = e.Point
		case LineTo:
			area += lineArea(current, e.Point)
			current = e.Point
		case QuadTo:
			area += quadArea(current, e.Control, e.Point)
			current = e.Point
		case CubicTo:
			area += cubicArea(current, e.Control1, e.Control2, e.Point)
			current = e.Point
		case Close:
			area += lineArea(current, start)
			current = start
		}
	}

	return area
}

// lineArea computes the contribution of a line segment to the signed area.
// Uses the shoelace formula: 0.5 * (x0*y1 - x1*y0)
func lineArea(p0, p1 Point) float64 {
	return 0.5 * (p0.X*p1.Y - p1.X*p0.Y)
}

// quadArea computes the contribution of a quadratic Bezier to the signed area.
// Integrates x*dy using the parametric form.
func quadArea(p0, p1, p2 Point) float64 {
	// For a quadratic Bezier B(t) = (1-t)^2*P0 + 2*(1-t)*t*P1 + t^2*P2
	// Area contribution = integral of x*dy from t=0 to t=1
	// After computing symbolically:
	// area = (x0*(2*y1 + y2) + x1*(y2 - y0) + x2*(-2*y1 - y0)) / 6
	// Simplified: area = (x0*(2*y1 + y2) + x1*(-y0 + y2) + x2*(-2*y1 - y0)) / 6
	return (p0.X*(2*p1.Y+p2.Y) + p1.X*(-p0.Y+p2.Y) + p2.X*(-2*p1.Y-p0.Y)) / 6.0
}

// cubicArea computes the contribution of a cubic Bezier to the signed area.
// Integrates x*dy using the parametric form and Green's theorem.
// Formula derived from: integral of x*dy for B(t) = (1-t)^3*P0 + 3*(1-t)^2*t*P1 + 3*(1-t)*t^2*P2 + t^3*P3
func cubicArea(p0, p1, p2, p3 Point) float64 {
	// The exact formula for the signed area contribution of a cubic Bezier:
	// Area = (3/20) * [ (x1-x0)*(y2-y0) - (x2-x0)*(y1-y0)
	//                 + (x2-x0)*(y3-y0) - (x3-x0)*(y2-y0)
	//                 + 2*((x1-x0)*(y3-y0) - (x3-x0)*(y1-y0))
	//                 + (x3-x0)*(y0+y3) - x0*(y3-y0) ]
	// Simplified using shoelace-like form:
	// = (x0*(6*y1-3*y3+3*y2) + x1*(3*y2-6*y0+3*y3) + x2*(3*y3-6*y0+3*y1) + x3*(-3*y2+6*y1-3*y0)) / 20
	//   + closing segment from p3 to p0

	// Simpler formulation using cross products:
	// Area = 3/20 * [(P1-P0) x (P2-P0) + (P2-P0) x (P3-P0) + 2*(P1-P0) x (P3-P0)]
	//        + (P3 x P0) / 2 [closing segment contribution]

	// Using the formula from the kurbo library:
	// area = (x0*(6*y1 + 3*y2 + y3) + 3*x1*(-2*y0 + y2 + y3) + 3*x2*(-y0 - y1 + 2*y3) + x3*(-y0 - 3*y1 - 6*y2)) / 20
	// Plus the closing line from p3 back to origin (included in total path area)

	// Direct formula for cubic bezier area contribution:
	return (p0.X*(6*p1.Y+3*p2.Y+p3.Y) +
		3*p1.X*(-2*p0.Y+p2.Y+p3.Y) +
		3*p2.X*(-p0.Y-p1.Y+2*p3.Y) +
		p3.X*(-p0.Y-3*p1.Y-6*p2.Y)) / 20.0
}

// Winding returns the winding number of a point relative to the path.
// 0 = outside, non-zero = inside (for non-zero fill rule).
// Uses ray casting with a horizontal ray to the right.
func (p *Path) Winding(pt Point) int {
	var winding int
	var current, start Point

	for _, elem := range p.elements {
		switch e := elem.(type) {
		case MoveTo:
			start = e.Point
			current = e.Point
		case LineTo:
			winding += lineWinding(current, e.Point, pt)
			current = e.Point
		case QuadTo:
			winding += quadWinding(current, e.Control, e.Point, pt)
			current = e.Point
		case CubicTo:
			winding += cubicWinding(current, e.Control1, e.Control2, e.Point, pt)
			current = e.Point
		case Close:
			winding += lineWinding(current, start, pt)
			current = start
		}
	}

	return winding
}

// lineWinding computes the winding contribution of a line segment.
func lineWinding(p0, p1, pt Point) int {
	if p0.Y <= pt.Y && p1.Y > pt.Y {
		// Upward crossing
		if isLeft(p0, p1, pt) > 0 {
			return 1
		}
	} else if p0.Y > pt.Y && p1.Y <= pt.Y {
		// Downward crossing
		if isLeft(p0, p1, pt) < 0 {
			return -1
		}
	}
	return 0
}

// isLeft returns positive if pt is left of line p0-p1, negative if right, 0 if on.
func isLeft(p0, p1, pt Point) float64 {
	return (p1.X-p0.X)*(pt.Y-p0.Y) - (pt.X-p0.X)*(p1.Y-p0.Y)
}

// quadWinding computes the winding contribution of a quadratic Bezier.
func quadWinding(p0, p1, p2, pt Point) int {
	// Early exit if point is outside the vertical range
	minY := math.Min(math.Min(p0.Y, p1.Y), p2.Y)
	maxY := math.Max(math.Max(p0.Y, p1.Y), p2.Y)
	if pt.Y < minY || pt.Y > maxY {
		return 0
	}

	// Early exit if point is to the right of the curve
	maxX := math.Max(math.Max(p0.X, p1.X), p2.X)
	if pt.X > maxX {
		return 0
	}

	// Flatten the curve and sum line winding contributions
	return flattenQuadWinding(p0, p1, p2, pt)
}

// flattenQuadWinding computes winding by adaptively flattening the quadratic.
func flattenQuadWinding(p0, p1, p2, pt Point) int {
	q := NewQuadBez(p0, p1, p2)

	// Use adaptive subdivision based on flatness
	const tolerance = 0.1
	var winding int
	flattenQuadWindingRecursive(q, pt, tolerance, &winding)
	return winding
}

// flattenQuadWindingRecursive recursively subdivides and accumulates winding.
func flattenQuadWindingRecursive(q QuadBez, pt Point, tolerance float64, winding *int) {
	// Flatness test: distance from control point to chord
	mid := q.P0.Lerp(q.P2, 0.5)
	dist := q.P1.Sub(mid).Length()

	if dist <= tolerance {
		// Flat enough - use line approximation
		*winding += lineWinding(q.P0, q.P2, pt)
		return
	}

	// Subdivide and recurse
	q1, q2 := q.Subdivide()
	flattenQuadWindingRecursive(q1, pt, tolerance, winding)
	flattenQuadWindingRecursive(q2, pt, tolerance, winding)
}

// cubicWinding computes the winding contribution of a cubic Bezier.
func cubicWinding(p0, p1, p2, p3, pt Point) int {
	// Early exit if point is outside the vertical range
	minY := math.Min(math.Min(p0.Y, p1.Y), math.Min(p2.Y, p3.Y))
	maxY := math.Max(math.Max(p0.Y, p1.Y), math.Max(p2.Y, p3.Y))
	if pt.Y < minY || pt.Y > maxY {
		return 0
	}

	// Early exit if point is to the right of the curve
	maxX := math.Max(math.Max(p0.X, p1.X), math.Max(p2.X, p3.X))
	if pt.X > maxX {
		return 0
	}

	// Flatten the curve and sum line winding contributions
	return flattenCubicWinding(p0, p1, p2, p3, pt)
}

// flattenCubicWinding computes winding by adaptively flattening the cubic.
func flattenCubicWinding(p0, p1, p2, p3, pt Point) int {
	c := NewCubicBez(p0, p1, p2, p3)

	const tolerance = 0.1
	var winding int
	flattenCubicWindingRecursive(c, pt, tolerance, &winding)
	return winding
}

// flattenCubicWindingRecursive recursively subdivides and accumulates winding.
func flattenCubicWindingRecursive(c CubicBez, pt Point, tolerance float64, winding *int) {
	// Flatness test: max distance from control points to chord
	flatness := cubicFlatness(c)

	if flatness <= tolerance {
		// Flat enough - use line approximation
		*winding += lineWinding(c.P0, c.P3, pt)
		return
	}

	// Subdivide and recurse
	c1, c2 := c.Subdivide()
	flattenCubicWindingRecursive(c1, pt, tolerance, winding)
	flattenCubicWindingRecursive(c2, pt, tolerance, winding)
}

// cubicFlatness returns the maximum distance from control points to the chord.
func cubicFlatness(c CubicBez) float64 {
	// Distance from P1 and P2 to the line P0-P3
	ux := 3.0*c.P1.X - 2.0*c.P0.X - c.P3.X
	uy := 3.0*c.P1.Y - 2.0*c.P0.Y - c.P3.Y
	vx := 3.0*c.P2.X - c.P0.X - 2.0*c.P3.X
	vy := 3.0*c.P2.Y - c.P0.Y - 2.0*c.P3.Y

	return math.Max(ux*ux+uy*uy, vx*vx+vy*vy)
}

// Contains tests if a point is inside the path using the non-zero fill rule.
func (p *Path) Contains(pt Point) bool {
	return p.Winding(pt) != 0
}

// BoundingBox returns the tight axis-aligned bounding box of the path.
// Uses curve extrema for accuracy.
func (p *Path) BoundingBox() Rect {
	if len(p.elements) == 0 {
		return Rect{}
	}

	// Initialize with extreme values
	bbox := Rect{
		Min: Point{X: math.MaxFloat64, Y: math.MaxFloat64},
		Max: Point{X: -math.MaxFloat64, Y: -math.MaxFloat64},
	}

	var current Point

	for _, elem := range p.elements {
		switch e := elem.(type) {
		case MoveTo:
			bbox = expandBBox(bbox, e.Point)
			current = e.Point
		case LineTo:
			bbox = expandBBox(bbox, e.Point)
			current = e.Point
		case QuadTo:
			bbox = bbox.Union(quadBBox(current, e.Control, e.Point))
			current = e.Point
		case CubicTo:
			bbox = bbox.Union(cubicBBox(current, e.Control1, e.Control2, e.Point))
			current = e.Point
		case Close:
			// Close doesn't add new points
		}
	}

	// Handle empty path case
	if bbox.Min.X == math.MaxFloat64 {
		return Rect{}
	}

	return bbox
}

// expandBBox expands the bounding box to include the point.
func expandBBox(bbox Rect, pt Point) Rect {
	return Rect{
		Min: Point{X: math.Min(bbox.Min.X, pt.X), Y: math.Min(bbox.Min.Y, pt.Y)},
		Max: Point{X: math.Max(bbox.Max.X, pt.X), Y: math.Max(bbox.Max.Y, pt.Y)},
	}
}

// quadBBox returns the tight bounding box of a quadratic Bezier.
func quadBBox(p0, p1, p2 Point) Rect {
	q := NewQuadBez(p0, p1, p2)
	return q.BoundingBox()
}

// cubicBBox returns the tight bounding box of a cubic Bezier.
func cubicBBox(p0, p1, p2, p3 Point) Rect {
	c := NewCubicBez(p0, p1, p2, p3)
	return c.BoundingBox()
}

// Flatten converts all curves to line segments with given tolerance.
// tolerance is the maximum distance from the curve.
func (p *Path) Flatten(tolerance float64) []Point {
	if len(p.elements) == 0 {
		return nil
	}

	points := make([]Point, 0, len(p.elements)*4)
	p.FlattenCallback(tolerance, func(pt Point) {
		points = append(points, pt)
	})
	return points
}

// FlattenCallback calls fn for each point in the flattened path.
// More efficient than Flatten() as it avoids allocation.
func (p *Path) FlattenCallback(tolerance float64, fn func(pt Point)) {
	if tolerance <= 0 {
		tolerance = 0.1 // Default tolerance
	}

	var current, start Point
	var started bool

	for _, elem := range p.elements {
		switch e := elem.(type) {
		case MoveTo:
			if started {
				fn(current) // Emit last point of previous subpath
			}
			fn(e.Point)
			start = e.Point
			current = e.Point
			started = true
		case LineTo:
			fn(e.Point)
			current = e.Point
		case QuadTo:
			flattenQuad(current, e.Control, e.Point, tolerance, fn)
			current = e.Point
		case CubicTo:
			flattenCubic(current, e.Control1, e.Control2, e.Point, tolerance, fn)
			current = e.Point
		case Close:
			if current != start {
				fn(start)
			}
			current = start
		}
	}
}

// flattenQuad flattens a quadratic Bezier curve.
func flattenQuad(p0, p1, p2 Point, tolerance float64, fn func(pt Point)) {
	q := NewQuadBez(p0, p1, p2)
	flattenQuadRecursive(q, tolerance*tolerance, fn)
}

// flattenQuadRecursive recursively subdivides the quadratic.
func flattenQuadRecursive(q QuadBez, toleranceSq float64, fn func(pt Point)) {
	// Flatness test: distance from control point to chord midpoint
	mid := q.P0.Lerp(q.P2, 0.5)
	dist := q.P1.Sub(mid)
	if dist.LengthSquared() <= toleranceSq {
		fn(q.P2)
		return
	}

	// Subdivide
	q1, q2 := q.Subdivide()
	flattenQuadRecursive(q1, toleranceSq, fn)
	flattenQuadRecursive(q2, toleranceSq, fn)
}

// flattenCubic flattens a cubic Bezier curve.
func flattenCubic(p0, p1, p2, p3 Point, tolerance float64, fn func(pt Point)) {
	c := NewCubicBez(p0, p1, p2, p3)
	flattenCubicRecursive(c, tolerance*tolerance, fn)
}

// flattenCubicRecursive recursively subdivides the cubic.
func flattenCubicRecursive(c CubicBez, toleranceSq float64, fn func(pt Point)) {
	// Flatness test using the standard cubic flatness metric
	flatness := cubicFlatness(c)

	if flatness <= toleranceSq*16 { // Adjust for the metric scale
		fn(c.P3)
		return
	}

	// Subdivide
	c1, c2 := c.Subdivide()
	flattenCubicRecursive(c1, toleranceSq, fn)
	flattenCubicRecursive(c2, toleranceSq, fn)
}

// Reversed returns a new path with reversed direction.
// Each subpath is reversed independently.
func (p *Path) Reversed() *Path {
	if len(p.elements) == 0 {
		return NewPath()
	}

	// Collect subpaths
	subpaths := p.collectSubpaths()

	// Reverse each subpath and build new path
	result := NewPath()
	for _, sp := range subpaths {
		reverseSubpath(sp, result)
	}

	return result
}

// subpath represents a single subpath with its elements and closure state.
type subpath struct {
	elements []PathElement
	closed   bool
}

// collectSubpaths splits the path into separate subpaths.
func (p *Path) collectSubpaths() []subpath {
	var subpaths []subpath
	var current subpath

	for _, elem := range p.elements {
		switch elem.(type) {
		case MoveTo:
			// Start a new subpath
			if len(current.elements) > 0 {
				subpaths = append(subpaths, current)
			}
			current = subpath{elements: []PathElement{elem}}
		case Close:
			current.closed = true
			subpaths = append(subpaths, current)
			current = subpath{}
		default:
			current.elements = append(current.elements, elem)
		}
	}

	// Add final subpath if not closed
	if len(current.elements) > 0 {
		subpaths = append(subpaths, current)
	}

	return subpaths
}

// reverseSubpath reverses a single subpath and appends to result.
func reverseSubpath(sp subpath, result *Path) {
	if len(sp.elements) == 0 {
		return
	}

	// Get the endpoint of the subpath
	endPoint := getSubpathEndpoint(sp)

	// Start from the endpoint
	result.MoveTo(endPoint.X, endPoint.Y)

	// Reverse elements
	for i := len(sp.elements) - 1; i >= 0; i-- {
		elem := sp.elements[i]
		prevPoint := getElementStartPoint(sp, i)

		switch e := elem.(type) {
		case MoveTo:
			// MoveTo becomes the end point
			continue
		case LineTo:
			result.LineTo(prevPoint.X, prevPoint.Y)
		case QuadTo:
			// Reverse quadratic: swap start and end, keep control
			result.QuadraticTo(e.Control.X, e.Control.Y, prevPoint.X, prevPoint.Y)
		case CubicTo:
			// Reverse cubic: swap start and end, swap control points
			result.CubicTo(e.Control2.X, e.Control2.Y, e.Control1.X, e.Control1.Y, prevPoint.X, prevPoint.Y)
		}
	}

	if sp.closed {
		result.Close()
	}
}

// getSubpathEndpoint returns the endpoint of a subpath.
func getSubpathEndpoint(sp subpath) Point {
	if len(sp.elements) == 0 {
		return Point{}
	}

	// Find the last non-MoveTo element
	for i := len(sp.elements) - 1; i >= 0; i-- {
		switch e := sp.elements[i].(type) {
		case MoveTo:
			return e.Point
		case LineTo:
			return e.Point
		case QuadTo:
			return e.Point
		case CubicTo:
			return e.Point
		}
	}

	// Fallback to MoveTo
	if m, ok := sp.elements[0].(MoveTo); ok {
		return m.Point
	}
	return Point{}
}

// getElementStartPoint returns the start point of element at index i.
func getElementStartPoint(sp subpath, i int) Point {
	if i == 0 {
		// First element (MoveTo)
		if m, ok := sp.elements[0].(MoveTo); ok {
			return m.Point
		}
		return Point{}
	}

	// Get endpoint of previous element
	switch e := sp.elements[i-1].(type) {
	case MoveTo:
		return e.Point
	case LineTo:
		return e.Point
	case QuadTo:
		return e.Point
	case CubicTo:
		return e.Point
	}
	return Point{}
}

// Length returns the total arc length of the path.
// accuracy controls the precision of the approximation (smaller = more accurate).
func (p *Path) Length(accuracy float64) float64 {
	if accuracy <= 0 {
		accuracy = 0.001 // Default accuracy
	}

	var length float64
	var current Point

	for _, elem := range p.elements {
		switch e := elem.(type) {
		case MoveTo:
			current = e.Point
		case LineTo:
			length += current.Distance(e.Point)
			current = e.Point
		case QuadTo:
			length += quadLength(current, e.Control, e.Point, accuracy)
			current = e.Point
		case CubicTo:
			length += cubicLength(current, e.Control1, e.Control2, e.Point, accuracy)
			current = e.Point
		case Close:
			// Close doesn't add length (already computed if there's a closing line)
		}
	}

	return length
}

// quadLength computes the arc length of a quadratic Bezier.
// Uses adaptive subdivision.
func quadLength(p0, p1, p2 Point, accuracy float64) float64 {
	q := NewQuadBez(p0, p1, p2)
	return quadLengthRecursive(q, accuracy*accuracy)
}

// quadLengthRecursive recursively computes quadratic arc length.
func quadLengthRecursive(q QuadBez, accuracySq float64) float64 {
	// Compute chord length and control polygon length
	chord := q.P0.Distance(q.P2)
	polygon := q.P0.Distance(q.P1) + q.P1.Distance(q.P2)

	// If they're close enough, use the average
	diff := polygon - chord
	if diff*diff <= accuracySq {
		return (chord + polygon) / 2
	}

	// Subdivide
	q1, q2 := q.Subdivide()
	return quadLengthRecursive(q1, accuracySq) + quadLengthRecursive(q2, accuracySq)
}

// cubicLength computes the arc length of a cubic Bezier.
// Uses adaptive subdivision.
func cubicLength(p0, p1, p2, p3 Point, accuracy float64) float64 {
	c := NewCubicBez(p0, p1, p2, p3)
	return cubicLengthRecursive(c, accuracy*accuracy)
}

// cubicLengthRecursive recursively computes cubic arc length.
func cubicLengthRecursive(c CubicBez, accuracySq float64) float64 {
	// Compute chord length and control polygon length
	chord := c.P0.Distance(c.P3)
	polygon := c.P0.Distance(c.P1) + c.P1.Distance(c.P2) + c.P2.Distance(c.P3)

	// If they're close enough, use the average
	diff := polygon - chord
	if diff*diff <= accuracySq {
		return (chord + polygon) / 2
	}

	// Subdivide
	c1, c2 := c.Subdivide()
	return cubicLengthRecursive(c1, accuracySq) + cubicLengthRecursive(c2, accuracySq)
}
