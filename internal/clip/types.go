// Package clip provides geometric clipping for paths and shapes.
package clip

import "math"

// Point represents a 2D point with float64 coordinates.
type Point struct {
	X, Y float64
}

// Pt creates a Point from x, y coordinates.
func Pt(x, y float64) Point {
	return Point{X: x, Y: y}
}

// Add returns the sum of two points.
func (p Point) Add(q Point) Point {
	return Point{X: p.X + q.X, Y: p.Y + q.Y}
}

// Sub returns the difference of two points.
func (p Point) Sub(q Point) Point {
	return Point{X: p.X - q.X, Y: p.Y - q.Y}
}

// Mul returns the point scaled by s.
func (p Point) Mul(s float64) Point {
	return Point{X: p.X * s, Y: p.Y * s}
}

// Lerp performs linear interpolation between p and q.
func (p Point) Lerp(q Point, t float64) Point {
	return Point{
		X: p.X + (q.X-p.X)*t,
		Y: p.Y + (q.Y-p.Y)*t,
	}
}

// Rect represents a rectangle with float64 coordinates.
type Rect struct {
	X, Y float64 // Top-left corner
	W, H float64 // Width and height
}

// NewRect creates a Rect from position and size.
func NewRect(x, y, w, h float64) Rect {
	return Rect{X: x, Y: y, W: w, H: h}
}

// Right returns the right edge x-coordinate.
func (r Rect) Right() float64 {
	return r.X + r.W
}

// Bottom returns the bottom edge y-coordinate.
func (r Rect) Bottom() float64 {
	return r.Y + r.H
}

// Contains returns true if the point is inside the rectangle.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.X <= r.Right() && p.Y >= r.Y && p.Y <= r.Bottom()
}

// Intersects returns true if two rectangles overlap.
func (r Rect) Intersects(other Rect) bool {
	return !(other.X > r.Right() || other.Right() < r.X ||
		other.Y > r.Bottom() || other.Bottom() < r.Y)
}

// Intersect returns the intersection of two rectangles.
// Returns an empty rectangle if they don't intersect.
func (r Rect) Intersect(other Rect) Rect {
	x0 := math.Max(r.X, other.X)
	y0 := math.Max(r.Y, other.Y)
	x1 := math.Min(r.Right(), other.Right())
	y1 := math.Min(r.Bottom(), other.Bottom())

	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// IsEmpty returns true if the rectangle has zero area.
func (r Rect) IsEmpty() bool {
	return r.W <= 0 || r.H <= 0
}

// LineSeg represents a line segment.
type LineSeg struct {
	P0, P1 Point
}

// QuadSeg represents a quadratic Bezier segment.
type QuadSeg struct {
	P0, P1, P2 Point
}

// CubicSeg represents a cubic Bezier segment.
type CubicSeg struct {
	P0, P1, P2, P3 Point
}

// Bounds returns the bounding box of a quadratic Bezier.
func (q QuadSeg) Bounds() Rect {
	minX := math.Min(q.P0.X, math.Min(q.P1.X, q.P2.X))
	maxX := math.Max(q.P0.X, math.Max(q.P1.X, q.P2.X))
	minY := math.Min(q.P0.Y, math.Min(q.P1.Y, q.P2.Y))
	maxY := math.Max(q.P0.Y, math.Max(q.P1.Y, q.P2.Y))
	return Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
}

// Bounds returns the bounding box of a cubic Bezier.
func (c CubicSeg) Bounds() Rect {
	minX := math.Min(c.P0.X, math.Min(c.P1.X, math.Min(c.P2.X, c.P3.X)))
	maxX := math.Max(c.P0.X, math.Max(c.P1.X, math.Max(c.P2.X, c.P3.X)))
	minY := math.Min(c.P0.Y, math.Min(c.P1.Y, math.Min(c.P2.Y, c.P3.Y)))
	maxY := math.Max(c.P0.Y, math.Max(c.P1.Y, math.Max(c.P2.Y, c.P3.Y)))
	return Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
}
