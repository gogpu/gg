// Package path provides internal path processing utilities.
package path

import "math"

// Point represents a 2D point (internal copy to avoid import cycle).
type Point struct {
	X, Y float64
}

// Tolerance is the maximum distance from the curve for flattening.
const Tolerance = 0.1

// PathElement represents an element in a path.
type PathElement interface {
	isPathElement()
}

// MoveTo moves to a point.
type MoveTo struct{ Point Point }

func (MoveTo) isPathElement() {}

// LineTo draws a line.
type LineTo struct{ Point Point }

func (LineTo) isPathElement() {}

// QuadTo draws a quadratic curve.
type QuadTo struct{ Control, Point Point }

func (QuadTo) isPathElement() {}

// CubicTo draws a cubic curve.
type CubicTo struct{ Control1, Control2, Point Point }

func (CubicTo) isPathElement() {}

// Close closes the path.
type Close struct{}

func (Close) isPathElement() {}

// Flatten converts a path with curves into a path with only straight lines.
func Flatten(elements []PathElement) []Point {
	var points []Point
	var current Point

	for _, elem := range elements {
		switch e := elem.(type) {
		case MoveTo:
			current = e.Point
			points = append(points, current)

		case LineTo:
			current = e.Point
			points = append(points, current)

		case QuadTo:
			// Flatten quadratic Bezier curve
			quad := flattenQuadratic(current, e.Control, e.Point, Tolerance)
			points = append(points, quad...)
			current = e.Point

		case CubicTo:
			// Flatten cubic Bezier curve
			cubic := flattenCubic(current, e.Control1, e.Control2, e.Point, Tolerance)
			points = append(points, cubic...)
			current = e.Point

		case Close:
			// Close returns to the start of the subpath
			if len(points) > 0 {
				points = append(points, points[0])
			}
		}
	}

	return points
}

// Helper methods for Point
func (p Point) Lerp(q Point, t float64) Point {
	return Point{
		X: p.X + (q.X-p.X)*t,
		Y: p.Y + (q.Y-p.Y)*t,
	}
}

func (p Point) Sub(q Point) Point {
	return Point{X: p.X - q.X, Y: p.Y - q.Y}
}

func (p Point) Add(q Point) Point {
	return Point{X: p.X + q.X, Y: p.Y + q.Y}
}

func (p Point) Mul(s float64) Point {
	return Point{X: p.X * s, Y: p.Y * s}
}

func (p Point) Dot(q Point) float64 {
	return p.X*q.X + p.Y*q.Y
}

func (p Point) Length() float64 {
	return math.Sqrt(p.X*p.X + p.Y*p.Y)
}

func (p Point) Distance(q Point) float64 {
	return p.Sub(q).Length()
}

// flattenQuadratic flattens a quadratic Bezier curve into line segments.
func flattenQuadratic(p0, p1, p2 Point, tolerance float64) []Point {
	var points []Point
	flattenQuadraticRec(p0, p1, p2, tolerance, &points)
	return points
}

// flattenQuadraticRec recursively subdivides a quadratic Bezier curve.
func flattenQuadraticRec(p0, p1, p2 Point, tolerance float64, points *[]Point) {
	// Calculate the distance from the control point to the line p0-p2
	dist := distanceToLine(p1, p0, p2)

	if dist < tolerance {
		// Curve is flat enough, add the endpoint
		*points = append(*points, p2)
		return
	}

	// Subdivide the curve
	q0 := p0.Lerp(p1, 0.5)
	q1 := p1.Lerp(p2, 0.5)
	q2 := q0.Lerp(q1, 0.5)

	flattenQuadraticRec(p0, q0, q2, tolerance, points)
	flattenQuadraticRec(q2, q1, p2, tolerance, points)
}

// flattenCubic flattens a cubic Bezier curve into line segments.
func flattenCubic(p0, p1, p2, p3 Point, tolerance float64) []Point {
	var points []Point
	flattenCubicRec(p0, p1, p2, p3, tolerance, &points)
	return points
}

// flattenCubicRec recursively subdivides a cubic Bezier curve.
func flattenCubicRec(p0, p1, p2, p3 Point, tolerance float64, points *[]Point) {
	// Calculate the distance from control points to the line p0-p3
	d1 := distanceToLine(p1, p0, p3)
	d2 := distanceToLine(p2, p0, p3)
	dist := math.Max(d1, d2)

	if dist < tolerance {
		// Curve is flat enough, add the endpoint
		*points = append(*points, p3)
		return
	}

	// Subdivide the curve using de Casteljau's algorithm
	q0 := p0.Lerp(p1, 0.5)
	q1 := p1.Lerp(p2, 0.5)
	q2 := p2.Lerp(p3, 0.5)
	r0 := q0.Lerp(q1, 0.5)
	r1 := q1.Lerp(q2, 0.5)
	s := r0.Lerp(r1, 0.5)

	flattenCubicRec(p0, q0, r0, s, tolerance, points)
	flattenCubicRec(s, r1, q2, p3, tolerance, points)
}

// distanceToLine calculates the perpendicular distance from point p to line segment (a, b).
func distanceToLine(p, a, b Point) float64 {
	// Vector from a to b
	ab := b.Sub(a)
	abLen := ab.Length()

	if abLen < 1e-10 {
		// Line segment is a point
		return p.Distance(a)
	}

	// Project p onto the line
	ap := p.Sub(a)
	t := ap.Dot(ab) / (abLen * abLen)

	if t < 0 {
		// Closest point is a
		return p.Distance(a)
	}
	if t > 1 {
		// Closest point is b
		return p.Distance(b)
	}

	// Closest point is on the line segment
	closest := a.Add(ab.Mul(t))
	return p.Distance(closest)
}
