package recording

import "math"

// Matrix represents a 2D affine transformation matrix.
// It uses a 2x3 matrix in row-major order:
//
//	| A  B  C |
//	| D  E  F |
//
// This represents the transformation:
//
//	x' = A*x + B*y + C
//	y' = D*x + E*y + F
//
// The Matrix type is designed to be compatible with gg.Matrix
// for seamless integration with the gg graphics library.
type Matrix struct {
	A, B, C float64
	D, E, F float64
}

// Identity returns the identity transformation matrix.
// The identity matrix performs no transformation.
func Identity() Matrix {
	return Matrix{
		A: 1, B: 0, C: 0,
		D: 0, E: 1, F: 0,
	}
}

// Translate creates a translation matrix.
func Translate(x, y float64) Matrix {
	return Matrix{
		A: 1, B: 0, C: x,
		D: 0, E: 1, F: y,
	}
}

// Scale creates a scaling matrix.
func Scale(sx, sy float64) Matrix {
	return Matrix{
		A: sx, B: 0, C: 0,
		D: 0, E: sy, F: 0,
	}
}

// Rotate creates a rotation matrix (angle in radians).
func Rotate(angle float64) Matrix {
	cos := math.Cos(angle)
	sin := math.Sin(angle)
	return Matrix{
		A: cos, B: -sin, C: 0,
		D: sin, E: cos, F: 0,
	}
}

// Shear creates a shear matrix.
func Shear(x, y float64) Matrix {
	return Matrix{
		A: 1, B: x, C: 0,
		D: y, E: 1, F: 0,
	}
}

// Multiply multiplies two matrices (m * other).
// This applies the transformation of `other` after `m`.
func (m Matrix) Multiply(other Matrix) Matrix {
	return Matrix{
		A: m.A*other.A + m.B*other.D,
		B: m.A*other.B + m.B*other.E,
		C: m.A*other.C + m.B*other.F + m.C,
		D: m.D*other.A + m.E*other.D,
		E: m.D*other.B + m.E*other.E,
		F: m.D*other.C + m.E*other.F + m.F,
	}
}

// TransformPoint applies the transformation to a point.
func (m Matrix) TransformPoint(x, y float64) (float64, float64) {
	return m.A*x + m.B*y + m.C, m.D*x + m.E*y + m.F
}

// TransformVector applies the transformation to a vector (no translation).
func (m Matrix) TransformVector(x, y float64) (float64, float64) {
	return m.A*x + m.B*y, m.D*x + m.E*y
}

// Invert returns the inverse matrix.
// Returns the identity matrix if the matrix is not invertible.
func (m Matrix) Invert() Matrix {
	det := m.A*m.E - m.B*m.D
	if math.Abs(det) < 1e-10 {
		return Identity()
	}

	invDet := 1.0 / det
	return Matrix{
		A: m.E * invDet,
		B: -m.B * invDet,
		C: (m.B*m.F - m.C*m.E) * invDet,
		D: -m.D * invDet,
		E: m.A * invDet,
		F: (m.C*m.D - m.A*m.F) * invDet,
	}
}

// IsIdentity returns true if the matrix is the identity matrix.
func (m Matrix) IsIdentity() bool {
	const eps = 1e-10
	return math.Abs(m.A-1) < eps && math.Abs(m.B) < eps && math.Abs(m.C) < eps &&
		math.Abs(m.D) < eps && math.Abs(m.E-1) < eps && math.Abs(m.F) < eps
}

// IsTranslation returns true if the matrix is only a translation.
func (m Matrix) IsTranslation() bool {
	const eps = 1e-10
	return math.Abs(m.A-1) < eps && math.Abs(m.B) < eps &&
		math.Abs(m.D) < eps && math.Abs(m.E-1) < eps
}

// ScaleFactor returns the maximum scale factor of the transformation.
// This is useful for determining effective stroke width after transform.
func (m Matrix) ScaleFactor() float64 {
	// Calculate the two singular values of the 2x2 part.
	sx := math.Sqrt(m.A*m.A + m.D*m.D)
	sy := math.Sqrt(m.B*m.B + m.E*m.E)
	if sx > sy {
		return sx
	}
	return sy
}

// Determinant returns the determinant of the 2x2 part of the matrix.
// A determinant of zero means the matrix is not invertible.
// A negative determinant means the transformation flips orientation.
func (m Matrix) Determinant() float64 {
	return m.A*m.E - m.B*m.D
}

// Translation returns the translation components of the matrix.
func (m Matrix) Translation() (x, y float64) {
	return m.C, m.F
}

// Rect represents an axis-aligned rectangle.
// Min is the top-left corner (minimum coordinates).
// Max is the bottom-right corner (maximum coordinates).
type Rect struct {
	MinX, MinY float64
	MaxX, MaxY float64
}

// NewRect creates a rectangle from position and size.
func NewRect(x, y, width, height float64) Rect {
	return Rect{
		MinX: x,
		MinY: y,
		MaxX: x + width,
		MaxY: y + height,
	}
}

// NewRectFromPoints creates a rectangle from two corner points.
// The points are normalized so Min <= Max.
func NewRectFromPoints(x1, y1, x2, y2 float64) Rect {
	return Rect{
		MinX: math.Min(x1, x2),
		MinY: math.Min(y1, y2),
		MaxX: math.Max(x1, x2),
		MaxY: math.Max(y1, y2),
	}
}

// X returns the left edge of the rectangle.
func (r Rect) X() float64 {
	return r.MinX
}

// Y returns the top edge of the rectangle.
func (r Rect) Y() float64 {
	return r.MinY
}

// Width returns the width of the rectangle.
func (r Rect) Width() float64 {
	return r.MaxX - r.MinX
}

// Height returns the height of the rectangle.
func (r Rect) Height() float64 {
	return r.MaxY - r.MinY
}

// IsEmpty returns true if the rectangle has zero or negative area.
func (r Rect) IsEmpty() bool {
	return r.MaxX <= r.MinX || r.MaxY <= r.MinY
}

// Contains returns true if the point is inside the rectangle.
func (r Rect) Contains(x, y float64) bool {
	return x >= r.MinX && x <= r.MaxX && y >= r.MinY && y <= r.MaxY
}

// Union returns the smallest rectangle containing both r and other.
func (r Rect) Union(other Rect) Rect {
	return Rect{
		MinX: math.Min(r.MinX, other.MinX),
		MinY: math.Min(r.MinY, other.MinY),
		MaxX: math.Max(r.MaxX, other.MaxX),
		MaxY: math.Max(r.MaxY, other.MaxY),
	}
}

// Intersect returns the intersection of r and other.
// Returns an empty rectangle if they don't intersect.
func (r Rect) Intersect(other Rect) Rect {
	result := Rect{
		MinX: math.Max(r.MinX, other.MinX),
		MinY: math.Max(r.MinY, other.MinY),
		MaxX: math.Min(r.MaxX, other.MaxX),
		MaxY: math.Min(r.MaxY, other.MaxY),
	}
	if result.IsEmpty() {
		return Rect{}
	}
	return result
}

// Inset returns a new rectangle inset by the given amounts.
// Positive values shrink the rectangle, negative values expand it.
func (r Rect) Inset(dx, dy float64) Rect {
	return Rect{
		MinX: r.MinX + dx,
		MinY: r.MinY + dy,
		MaxX: r.MaxX - dx,
		MaxY: r.MaxY - dy,
	}
}

// Offset returns a new rectangle offset by the given amounts.
func (r Rect) Offset(dx, dy float64) Rect {
	return Rect{
		MinX: r.MinX + dx,
		MinY: r.MinY + dy,
		MaxX: r.MaxX + dx,
		MaxY: r.MaxY + dy,
	}
}
