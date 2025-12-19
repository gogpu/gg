package gg

import (
	"math"
	"sort"
)

// Curve types for 2D geometry operations.
// Based on kurbo patterns, adapted for Go idioms.

// Rect represents an axis-aligned rectangle.
// Min is the top-left corner (minimum coordinates).
// Max is the bottom-right corner (maximum coordinates).
type Rect struct {
	Min, Max Point
}

// NewRect creates a rectangle from two points.
// The points are normalized so Min <= Max.
func NewRect(p1, p2 Point) Rect {
	return Rect{
		Min: Point{X: math.Min(p1.X, p2.X), Y: math.Min(p1.Y, p2.Y)},
		Max: Point{X: math.Max(p1.X, p2.X), Y: math.Max(p1.Y, p2.Y)},
	}
}

// Width returns the width of the rectangle.
func (r Rect) Width() float64 {
	return r.Max.X - r.Min.X
}

// Height returns the height of the rectangle.
func (r Rect) Height() float64 {
	return r.Max.Y - r.Min.Y
}

// Union returns the smallest rectangle containing both r and other.
func (r Rect) Union(other Rect) Rect {
	return Rect{
		Min: Point{X: math.Min(r.Min.X, other.Min.X), Y: math.Min(r.Min.Y, other.Min.Y)},
		Max: Point{X: math.Max(r.Max.X, other.Max.X), Y: math.Max(r.Max.Y, other.Max.Y)},
	}
}

// Contains returns true if the point is inside the rectangle.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.Min.X && p.X <= r.Max.X && p.Y >= r.Min.Y && p.Y <= r.Max.Y
}

// -------------------------------------------------------------------
// Line
// -------------------------------------------------------------------

// Line represents a line segment from P0 to P1.
type Line struct {
	P0, P1 Point
}

// NewLine creates a new line segment.
func NewLine(p0, p1 Point) Line {
	return Line{P0: p0, P1: p1}
}

// Eval evaluates the line at parameter t (0 to 1).
// t=0 returns P0, t=1 returns P1.
func (l Line) Eval(t float64) Point {
	return l.P0.Lerp(l.P1, t)
}

// Start returns the starting point of the line.
func (l Line) Start() Point {
	return l.P0
}

// End returns the ending point of the line.
func (l Line) End() Point {
	return l.P1
}

// Subdivide splits the line at t=0.5 into two halves.
func (l Line) Subdivide() (Line, Line) {
	mid := l.Eval(0.5)
	return Line{P0: l.P0, P1: mid}, Line{P0: mid, P1: l.P1}
}

// Subsegment returns the portion of the line from t0 to t1.
func (l Line) Subsegment(t0, t1 float64) Line {
	return Line{
		P0: l.Eval(t0),
		P1: l.Eval(t1),
	}
}

// BoundingBox returns the axis-aligned bounding box of the line.
func (l Line) BoundingBox() Rect {
	return NewRect(l.P0, l.P1)
}

// Length returns the length of the line segment.
func (l Line) Length() float64 {
	return l.P0.Distance(l.P1)
}

// Midpoint returns the midpoint of the line segment.
func (l Line) Midpoint() Point {
	return l.Eval(0.5)
}

// Reversed returns a copy of the line with endpoints swapped.
func (l Line) Reversed() Line {
	return Line{P0: l.P1, P1: l.P0}
}

// -------------------------------------------------------------------
// QuadBez - Quadratic Bezier Curve
// -------------------------------------------------------------------

// QuadBez represents a quadratic Bezier curve with control points P0, P1, P2.
// P0 is the start point, P1 is the control point, P2 is the end point.
type QuadBez struct {
	P0, P1, P2 Point
}

// NewQuadBez creates a new quadratic Bezier curve.
func NewQuadBez(p0, p1, p2 Point) QuadBez {
	return QuadBez{P0: p0, P1: p1, P2: p2}
}

// Eval evaluates the curve at parameter t (0 to 1) using de Casteljau's algorithm.
func (q QuadBez) Eval(t float64) Point {
	mt := 1.0 - t
	// (1-t)^2 * P0 + 2(1-t)t * P1 + t^2 * P2
	return Point{
		X: mt*mt*q.P0.X + 2*mt*t*q.P1.X + t*t*q.P2.X,
		Y: mt*mt*q.P0.Y + 2*mt*t*q.P1.Y + t*t*q.P2.Y,
	}
}

// Start returns the starting point of the curve.
func (q QuadBez) Start() Point {
	return q.P0
}

// End returns the ending point of the curve.
func (q QuadBez) End() Point {
	return q.P2
}

// Subdivide splits the curve at t=0.5 into two halves using de Casteljau.
func (q QuadBez) Subdivide() (QuadBez, QuadBez) {
	mid := q.Eval(0.5)
	return QuadBez{
			P0: q.P0,
			P1: q.P0.Lerp(q.P1, 0.5),
			P2: mid,
		}, QuadBez{
			P0: mid,
			P1: q.P1.Lerp(q.P2, 0.5),
			P2: q.P2,
		}
}

// Subsegment returns the portion of the curve from t0 to t1.
func (q QuadBez) Subsegment(t0, t1 float64) QuadBez {
	p0 := q.Eval(t0)
	p2 := q.Eval(t1)

	// Calculate the control point for the subsegment
	// Using the property that the tangent at any point is the lerp of
	// the control polygon edges
	d0 := q.P1.Sub(q.P0)
	d1 := q.P2.Sub(q.P1)
	dt := (t1 - t0)

	// Tangent direction at t0, scaled for the new segment
	tanDir := Point{
		X: d0.X + t0*(d1.X-d0.X),
		Y: d0.Y + t0*(d1.Y-d0.Y),
	}
	p1 := Point{
		X: p0.X + dt*tanDir.X,
		Y: p0.Y + dt*tanDir.Y,
	}

	return QuadBez{P0: p0, P1: p1, P2: p2}
}

// Extrema returns parameter values where the derivative is zero (extrema points).
// Used for computing tight bounding boxes.
func (q QuadBez) Extrema() []float64 {
	var result []float64

	// For a quadratic Bezier, the derivative is linear:
	// B'(t) = 2[(P1-P0) + t(P2-2P1+P0)]
	// Setting to zero: t = (P0-P1) / (P0-2P1+P2)

	d0 := q.P1.Sub(q.P0)
	d1 := q.P2.Sub(q.P1)
	dd := Point{X: d1.X - d0.X, Y: d1.Y - d0.Y}

	// X extrema
	if dd.X != 0 {
		t := -d0.X / dd.X
		if t > 0 && t < 1 {
			result = append(result, t)
		}
	}

	// Y extrema
	if dd.Y != 0 {
		t := -d0.Y / dd.Y
		if t > 0 && t < 1 {
			result = append(result, t)
		}
	}

	sort.Float64s(result)
	return result
}

// BoundingBox returns the tight axis-aligned bounding box of the curve.
func (q QuadBez) BoundingBox() Rect {
	// Start with endpoints
	bbox := NewRect(q.P0, q.P2)

	// Include extrema points
	for _, t := range q.Extrema() {
		p := q.Eval(t)
		bbox = bbox.Union(NewRect(p, p))
	}

	return bbox
}

// Raise elevates the quadratic to a cubic Bezier curve.
// Returns an exact cubic representation of this quadratic.
func (q QuadBez) Raise() CubicBez {
	// For a quadratic Q with points (P0, P1, P2), the cubic representation is:
	// C0 = P0
	// C1 = P0 + 2/3 * (P1 - P0) = (P0 + 2*P1) / 3
	// C2 = P2 + 2/3 * (P1 - P2) = (2*P1 + P2) / 3
	// C3 = P2
	return CubicBez{
		P0: q.P0,
		P1: Point{
			X: q.P0.X + (2.0/3.0)*(q.P1.X-q.P0.X),
			Y: q.P0.Y + (2.0/3.0)*(q.P1.Y-q.P0.Y),
		},
		P2: Point{
			X: q.P2.X + (2.0/3.0)*(q.P1.X-q.P2.X),
			Y: q.P2.Y + (2.0/3.0)*(q.P1.Y-q.P2.Y),
		},
		P3: q.P2,
	}
}

// -------------------------------------------------------------------
// CubicBez - Cubic Bezier Curve
// -------------------------------------------------------------------

// CubicBez represents a cubic Bezier curve with control points P0, P1, P2, P3.
// P0 is the start point, P1 and P2 are control points, P3 is the end point.
type CubicBez struct {
	P0, P1, P2, P3 Point
}

// NewCubicBez creates a new cubic Bezier curve.
func NewCubicBez(p0, p1, p2, p3 Point) CubicBez {
	return CubicBez{P0: p0, P1: p1, P2: p2, P3: p3}
}

// Eval evaluates the curve at parameter t (0 to 1) using de Casteljau's algorithm.
func (c CubicBez) Eval(t float64) Point {
	mt := 1.0 - t
	mt2 := mt * mt
	mt3 := mt2 * mt
	t2 := t * t
	t3 := t2 * t

	// (1-t)^3 * P0 + 3(1-t)^2*t * P1 + 3(1-t)*t^2 * P2 + t^3 * P3
	return Point{
		X: mt3*c.P0.X + 3*mt2*t*c.P1.X + 3*mt*t2*c.P2.X + t3*c.P3.X,
		Y: mt3*c.P0.Y + 3*mt2*t*c.P1.Y + 3*mt*t2*c.P2.Y + t3*c.P3.Y,
	}
}

// Start returns the starting point of the curve.
func (c CubicBez) Start() Point {
	return c.P0
}

// End returns the ending point of the curve.
func (c CubicBez) End() Point {
	return c.P3
}

// Subdivide splits the curve at t=0.5 into two halves using de Casteljau.
func (c CubicBez) Subdivide() (CubicBez, CubicBez) {
	// De Casteljau subdivision at t=0.5
	p01 := c.P0.Lerp(c.P1, 0.5)
	p12 := c.P1.Lerp(c.P2, 0.5)
	p23 := c.P2.Lerp(c.P3, 0.5)
	p012 := p01.Lerp(p12, 0.5)
	p123 := p12.Lerp(p23, 0.5)
	mid := p012.Lerp(p123, 0.5)

	return CubicBez{P0: c.P0, P1: p01, P2: p012, P3: mid},
		CubicBez{P0: mid, P1: p123, P2: p23, P3: c.P3}
}

// Subsegment returns the portion of the curve from t0 to t1.
func (c CubicBez) Subsegment(t0, t1 float64) CubicBez {
	p0 := c.Eval(t0)
	p3 := c.Eval(t1)

	// Calculate control points using derivative at endpoints
	// The derivative at t is: 3[(P1-P0)(1-t)^2 + 2(P2-P1)(1-t)t + (P3-P2)t^2]
	d0 := c.P1.Sub(c.P0)
	d1 := c.P2.Sub(c.P1)
	d2 := c.P3.Sub(c.P2)

	scale := (t1 - t0) / 3.0

	// Derivative at t0
	mt0 := 1.0 - t0
	deriv0 := Point{
		X: 3 * (d0.X*mt0*mt0 + 2*d1.X*mt0*t0 + d2.X*t0*t0),
		Y: 3 * (d0.Y*mt0*mt0 + 2*d1.Y*mt0*t0 + d2.Y*t0*t0),
	}
	p1 := Point{
		X: p0.X + scale*deriv0.X,
		Y: p0.Y + scale*deriv0.Y,
	}

	// Derivative at t1
	mt1 := 1.0 - t1
	deriv1 := Point{
		X: 3 * (d0.X*mt1*mt1 + 2*d1.X*mt1*t1 + d2.X*t1*t1),
		Y: 3 * (d0.Y*mt1*mt1 + 2*d1.Y*mt1*t1 + d2.Y*t1*t1),
	}
	p2 := Point{
		X: p3.X - scale*deriv1.X,
		Y: p3.Y - scale*deriv1.Y,
	}

	return CubicBez{P0: p0, P1: p1, P2: p2, P3: p3}
}

// Extrema returns parameter values where the derivative is zero (extrema points).
// For a cubic Bezier, there can be up to 4 extrema (2 for x, 2 for y).
func (c CubicBez) Extrema() []float64 {
	// Pre-allocate for max 4 extrema (2 for x, 2 for y)
	result := make([]float64, 0, 4)

	// The derivative is a quadratic: B'(t) = a*t^2 + b*t + c
	// Where the coefficients come from differentiating the Bernstein form
	d0 := c.P1.Sub(c.P0)
	d1 := c.P2.Sub(c.P1)
	d2 := c.P3.Sub(c.P2)

	// X extrema: solve d0.X - 2*d1.X + d2.X = 0
	ax := d0.X - 2*d1.X + d2.X
	bx := 2 * (d1.X - d0.X)
	cx := d0.X

	result = append(result, SolveQuadraticInUnitInterval(ax, bx, cx)...)

	// Y extrema
	ay := d0.Y - 2*d1.Y + d2.Y
	by := 2 * (d1.Y - d0.Y)
	cy := d0.Y

	result = append(result, SolveQuadraticInUnitInterval(ay, by, cy)...)

	sort.Float64s(result)
	return result
}

// BoundingBox returns the tight axis-aligned bounding box of the curve.
func (c CubicBez) BoundingBox() Rect {
	// Start with endpoints
	bbox := NewRect(c.P0, c.P3)

	// Include extrema points
	for _, t := range c.Extrema() {
		p := c.Eval(t)
		bbox = bbox.Union(NewRect(p, p))
	}

	return bbox
}

// Inflections returns the parameter values of inflection points.
// An inflection point is where the curvature changes sign.
// A cubic can have 0, 1, or 2 inflection points.
func (c CubicBez) Inflections() []float64 {
	// See https://www.caffeineowl.com/graphics/2d/vectorial/cubic-inflexion.html
	a := c.P1.Sub(c.P0)
	b := c.P2.Sub(c.P1).Sub(a)
	cc := c.P3.Sub(c.P0).Sub(c.P2.Sub(c.P1).Mul(3))

	// Cross products for the quadratic equation
	// Solves: crossBC * t^2 + crossAC * t + crossAB = 0
	crossAB := a.Cross(b)
	crossAC := a.Cross(cc)
	crossBC := b.Cross(cc)

	// Note: SolveQuadratic expects a*t^2 + b*t + c = 0
	roots := SolveQuadratic(crossBC, crossAC, crossAB)

	var result []float64
	for _, t := range roots {
		if t >= 0 && t <= 1 {
			result = append(result, t)
		}
	}

	sort.Float64s(result)
	return result
}

// Deriv returns the derivative curve (a quadratic Bezier).
// The derivative gives the tangent direction at any point.
func (c CubicBez) Deriv() QuadBez {
	return QuadBez{
		P0: Point{X: 3 * (c.P1.X - c.P0.X), Y: 3 * (c.P1.Y - c.P0.Y)},
		P1: Point{X: 3 * (c.P2.X - c.P1.X), Y: 3 * (c.P2.Y - c.P1.Y)},
		P2: Point{X: 3 * (c.P3.X - c.P2.X), Y: 3 * (c.P3.Y - c.P2.Y)},
	}
}

// Tangent returns the tangent vector at parameter t.
func (c CubicBez) Tangent(t float64) Vec2 {
	deriv := c.Deriv()
	p := deriv.Eval(t)
	return Vec2(p)
}

// Normal returns the normal vector (perpendicular to tangent) at parameter t.
func (c CubicBez) Normal(t float64) Vec2 {
	tan := c.Tangent(t)
	return tan.Perp().Normalize()
}
