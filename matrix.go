package gg

import "math"

// Matrix represents a 2D affine transformation matrix.
// It uses a 2x3 matrix in row-major order:
//
//	| a  b  c |
//	| d  e  f |
//
// This represents the transformation:
//
//	x' = a*x + b*y + c
//	y' = d*x + e*y + f
type Matrix struct {
	A, B, C float64
	D, E, F float64
}

// Identity returns the identity transformation matrix.
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
func Scale(x, y float64) Matrix {
	return Matrix{
		A: x, B: 0, C: 0,
		D: 0, E: y, F: 0,
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
func (m Matrix) TransformPoint(p Point) Point {
	return Point{
		X: m.A*p.X + m.B*p.Y + m.C,
		Y: m.D*p.X + m.E*p.Y + m.F,
	}
}

// TransformVector applies the transformation to a vector (no translation).
func (m Matrix) TransformVector(p Point) Point {
	return Point{
		X: m.A*p.X + m.B*p.Y,
		Y: m.D*p.X + m.E*p.Y,
	}
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
	return m.A == 1 && m.B == 0 && m.C == 0 &&
		m.D == 0 && m.E == 1 && m.F == 0
}

// IsTranslation returns true if the matrix is only a translation.
func (m Matrix) IsTranslation() bool {
	return m.A == 1 && m.B == 0 && m.D == 0 && m.E == 1
}
