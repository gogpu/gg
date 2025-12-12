package gg

import "math"

// Point represents a 2D point or vector.
type Point struct {
	X, Y float64
}

// Pt is a convenience function to create a Point.
func Pt(x, y float64) Point {
	return Point{X: x, Y: y}
}

// Add returns the sum of two points (vector addition).
func (p Point) Add(q Point) Point {
	return Point{X: p.X + q.X, Y: p.Y + q.Y}
}

// Sub returns the difference of two points (vector subtraction).
func (p Point) Sub(q Point) Point {
	return Point{X: p.X - q.X, Y: p.Y - q.Y}
}

// Mul returns the point scaled by a scalar.
func (p Point) Mul(s float64) Point {
	return Point{X: p.X * s, Y: p.Y * s}
}

// Div returns the point divided by a scalar.
func (p Point) Div(s float64) Point {
	return Point{X: p.X / s, Y: p.Y / s}
}

// Dot returns the dot product of two vectors.
func (p Point) Dot(q Point) float64 {
	return p.X*q.X + p.Y*q.Y
}

// Cross returns the 2D cross product (scalar).
func (p Point) Cross(q Point) float64 {
	return p.X*q.Y - p.Y*q.X
}

// Length returns the length of the vector.
func (p Point) Length() float64 {
	return math.Sqrt(p.X*p.X + p.Y*p.Y)
}

// LengthSquared returns the squared length of the vector.
func (p Point) LengthSquared() float64 {
	return p.X*p.X + p.Y*p.Y
}

// Distance returns the distance between two points.
func (p Point) Distance(q Point) float64 {
	return p.Sub(q).Length()
}

// Normalize returns a unit vector in the same direction.
func (p Point) Normalize() Point {
	length := p.Length()
	if length == 0 {
		return Point{X: 0, Y: 0}
	}
	return Point{X: p.X / length, Y: p.Y / length}
}

// Rotate returns the point rotated by angle radians around the origin.
func (p Point) Rotate(angle float64) Point {
	cos := math.Cos(angle)
	sin := math.Sin(angle)
	return Point{
		X: p.X*cos - p.Y*sin,
		Y: p.X*sin + p.Y*cos,
	}
}

// Lerp performs linear interpolation between two points.
// t=0 returns p, t=1 returns q, intermediate values interpolate.
func (p Point) Lerp(q Point, t float64) Point {
	return Point{
		X: p.X + (q.X-p.X)*t,
		Y: p.Y + (q.Y-p.Y)*t,
	}
}
