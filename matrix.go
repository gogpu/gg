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

// ScaleFactor returns the maximum scale factor of the transformation.
// This is useful for determining effective stroke width after transform.
// For a pure scale matrix Scale(sx, sy), returns max(sx, sy).
// For rotation/shear, returns the maximum singular value.
func (m Matrix) ScaleFactor() float64 {
	// Calculate the two singular values of the 2x2 part of the matrix.
	// For the matrix [A B; D E], singular values are sqrt of eigenvalues of A^T*A.
	// This gives us the maximum stretch factor in any direction.
	sx := math.Sqrt(m.A*m.A + m.D*m.D)
	sy := math.Sqrt(m.B*m.B + m.E*m.E)
	if sx > sy {
		return sx
	}
	return sy
}

// IsTranslationOnly reports whether the matrix is identity or pure translation
// (no scale, rotation, or skew). The 2x2 linear portion must be the identity.
//
// This is equivalent to IsTranslation but named for clarity in the text
// rendering pipeline where the distinction between "translation only" and
// "scale only" determines the rasterization algorithm.
func (m Matrix) IsTranslationOnly() bool {
	return m.A == 1 && m.B == 0 && m.D == 0 && m.E == 1
}

// IsScaleOnly reports whether the matrix has only scale (and possibly translation),
// with no rotation or skew. The off-diagonal elements of the 2x2 linear portion
// must be zero.
//
// Note that this returns true for identity and pure translation matrices as well,
// since those are special cases of scale (with scale factors of 1).
func (m Matrix) IsScaleOnly() bool {
	return m.B == 0 && m.D == 0
}

// MaxScaleFactor returns the maximum axis scale factor of the transformation.
// This is the largest singular value of the 2x2 linear portion of the matrix,
// representing the maximum stretch in any direction.
//
// For a pure scale matrix Scale(sx, sy), returns max(|sx|, |sy|).
// For a rotation matrix, returns 1.0 (rotation preserves lengths).
// For general matrices (with rotation and/or skew), computes the spectral norm
// via the eigenvalues of M^T * M.
//
// This matches the approach used by Skia (SkMatrix::getMaxScale) and
// Cairo (_cairo_matrix_compute_basis_scale_factors).
//
// Returns 0 if the matrix is degenerate (zero area).
func (m Matrix) MaxScaleFactor() float64 {
	// For scale-only matrices (no rotation/skew), use the fast path.
	if m.B == 0 && m.D == 0 {
		sx := math.Abs(m.A)
		sy := math.Abs(m.E)
		if sx > sy {
			return sx
		}
		return sy
	}

	// General case: compute max singular value via eigenvalues of M^T * M.
	//
	// For the 2x2 matrix [A B; D E]:
	//   M^T * M = [A*A+D*D  A*B+D*E]
	//             [A*B+D*E  B*B+E*E]
	//
	// The eigenvalues of a symmetric 2x2 matrix [p q; q r] are:
	//   lambda = (p + r +/- sqrt((p - r)^2 + 4*q^2)) / 2
	//
	// The max singular value = sqrt(max eigenvalue).
	p := m.A*m.A + m.D*m.D
	r := m.B*m.B + m.E*m.E
	q := m.A*m.B + m.D*m.E

	sum := p + r
	diff := p - r
	disc := math.Sqrt(diff*diff + 4*q*q)

	// Max eigenvalue of M^T * M.
	maxEigen := (sum + disc) / 2

	if maxEigen <= 0 {
		return 0
	}
	return math.Sqrt(maxEigen)
}
