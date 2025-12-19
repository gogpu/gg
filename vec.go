package gg

import "math"

// Vec2 represents a 2D displacement vector.
// Unlike Point which represents a position, Vec2 represents a direction and magnitude.
// This semantic distinction helps make code clearer when working with curve geometry.
type Vec2 struct {
	X, Y float64
}

// V2 is a convenience function to create a Vec2.
func V2(x, y float64) Vec2 {
	return Vec2{X: x, Y: y}
}

// Add returns the sum of two vectors.
func (v Vec2) Add(w Vec2) Vec2 {
	return Vec2{X: v.X + w.X, Y: v.Y + w.Y}
}

// Sub returns the difference of two vectors.
func (v Vec2) Sub(w Vec2) Vec2 {
	return Vec2{X: v.X - w.X, Y: v.Y - w.Y}
}

// Mul returns the vector scaled by a scalar.
func (v Vec2) Mul(s float64) Vec2 {
	return Vec2{X: v.X * s, Y: v.Y * s}
}

// Div returns the vector divided by a scalar.
func (v Vec2) Div(s float64) Vec2 {
	return Vec2{X: v.X / s, Y: v.Y / s}
}

// Neg returns the negation of the vector.
func (v Vec2) Neg() Vec2 {
	return Vec2{X: -v.X, Y: -v.Y}
}

// Dot returns the dot product of two vectors.
func (v Vec2) Dot(w Vec2) float64 {
	return v.X*w.X + v.Y*w.Y
}

// Cross returns the 2D cross product (scalar).
// This is the z-component of the 3D cross product with z=0.
// Useful for determining the sign of the angle between vectors.
func (v Vec2) Cross(w Vec2) float64 {
	return v.X*w.Y - v.Y*w.X
}

// Length returns the length (magnitude) of the vector.
func (v Vec2) Length() float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y)
}

// LengthSq returns the squared length of the vector.
// This is faster than Length() when you only need to compare magnitudes.
func (v Vec2) LengthSq() float64 {
	return v.X*v.X + v.Y*v.Y
}

// Normalize returns a unit vector in the same direction.
// Returns zero vector if the original vector has zero length.
func (v Vec2) Normalize() Vec2 {
	length := v.Length()
	if length == 0 {
		return Vec2{}
	}
	return Vec2{X: v.X / length, Y: v.Y / length}
}

// Lerp performs linear interpolation between two vectors.
// t=0 returns v, t=1 returns w, intermediate values interpolate.
func (v Vec2) Lerp(w Vec2, t float64) Vec2 {
	return Vec2{
		X: v.X + (w.X-v.X)*t,
		Y: v.Y + (w.Y-v.Y)*t,
	}
}

// Rotate returns the vector rotated by angle radians.
func (v Vec2) Rotate(angle float64) Vec2 {
	cos := math.Cos(angle)
	sin := math.Sin(angle)
	return Vec2{
		X: v.X*cos - v.Y*sin,
		Y: v.X*sin + v.Y*cos,
	}
}

// Perp returns the perpendicular vector (rotated 90 degrees counter-clockwise).
func (v Vec2) Perp() Vec2 {
	return Vec2{X: -v.Y, Y: v.X}
}

// Atan2 returns the angle of the vector in radians.
func (v Vec2) Atan2() float64 {
	return math.Atan2(v.Y, v.X)
}

// Angle returns the angle between two vectors in radians.
func (v Vec2) Angle(w Vec2) float64 {
	return math.Atan2(v.Cross(w), v.Dot(w))
}

// IsZero returns true if the vector is the zero vector.
func (v Vec2) IsZero() bool {
	return v.X == 0 && v.Y == 0
}

// Approx returns true if two vectors are approximately equal within epsilon.
func (v Vec2) Approx(w Vec2, epsilon float64) bool {
	return math.Abs(v.X-w.X) < epsilon && math.Abs(v.Y-w.Y) < epsilon
}

// ToPoint converts Vec2 to Point.
// Useful when you need to treat a displacement as a position.
func (v Vec2) ToPoint() Point {
	return Point(v)
}

// PointToVec2 converts Point to Vec2.
// Useful when you need to treat a position as a displacement.
func PointToVec2(p Point) Vec2 {
	return Vec2(p)
}
