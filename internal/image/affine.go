// Package image provides image buffer management for gogpu/gg.
package image

import (
	"math"
)

// Affine represents a 2D affine transformation matrix.
//
// The transformation is represented as a 3x3 matrix:
//   | a  b  c |
//   | d  e  f |
//   | 0  0  1 |
//
// This allows for translation, rotation, scaling, and shearing operations.
type Affine struct {
	a, b, c float64 // First row: x' = ax + by + c
	d, e, f float64 // Second row: y' = dx + ey + f
}

// Identity returns the identity transformation (no change).
func Identity() Affine {
	return Affine{
		a: 1, b: 0, c: 0,
		d: 0, e: 1, f: 0,
	}
}

// Translate returns a translation transformation that shifts points by (tx, ty).
func Translate(tx, ty float64) Affine {
	return Affine{
		a: 1, b: 0, c: tx,
		d: 0, e: 1, f: ty,
	}
}

// Scale returns a scaling transformation that scales by (sx, sy) around the origin.
// Use negative values to flip the image.
func Scale(sx, sy float64) Affine {
	return Affine{
		a: sx, b: 0, c: 0,
		d: 0, e: sy, f: 0,
	}
}

// Rotate returns a rotation transformation that rotates by angle (in radians) around the origin.
// Positive angles rotate counter-clockwise.
func Rotate(angle float64) Affine {
	cos := math.Cos(angle)
	sin := math.Sin(angle)
	return Affine{
		a: cos, b: -sin, c: 0,
		d: sin, e: cos, f: 0,
	}
}

// Shear returns a shearing transformation that shears by (sx, sy).
// sx controls horizontal shear (skew along x-axis).
// sy controls vertical shear (skew along y-axis).
func Shear(sx, sy float64) Affine {
	return Affine{
		a: 1, b: sx, c: 0,
		d: sy, e: 1, f: 0,
	}
}

// Multiply returns the result of multiplying this affine transform by another.
// The result applies 'other' first, then 'this'.
// This is equivalent to matrix multiplication: this * other.
func (a Affine) Multiply(other Affine) Affine {
	return Affine{
		a: a.a*other.a + a.b*other.d,
		b: a.a*other.b + a.b*other.e,
		c: a.a*other.c + a.b*other.f + a.c,
		d: a.d*other.a + a.e*other.d,
		e: a.d*other.b + a.e*other.e,
		f: a.d*other.c + a.e*other.f + a.f,
	}
}

// Invert returns the inverse transformation.
// Returns false if the matrix is singular (non-invertible).
func (a Affine) Invert() (Affine, bool) {
	// Compute determinant
	det := a.a*a.e - a.b*a.d

	// Check if matrix is singular
	if math.Abs(det) < 1e-10 {
		return Affine{}, false
	}

	invDet := 1.0 / det

	return Affine{
		a: a.e * invDet,
		b: -a.b * invDet,
		c: (a.b*a.f - a.c*a.e) * invDet,
		d: -a.d * invDet,
		e: a.a * invDet,
		f: (a.c*a.d - a.a*a.f) * invDet,
	}, true
}

// TransformPoint applies the affine transformation to point (x, y).
// Returns the transformed coordinates (x', y').
func (a Affine) TransformPoint(x, y float64) (float64, float64) {
	return a.a*x + a.b*y + a.c, a.d*x + a.e*y + a.f
}

// RotateAt returns a rotation transformation that rotates by angle (in radians)
// around the point (cx, cy).
func RotateAt(angle, cx, cy float64) Affine {
	// Translate to origin, rotate, translate back
	return Translate(cx, cy).Multiply(Rotate(angle)).Multiply(Translate(-cx, -cy))
}

// ScaleAt returns a scaling transformation that scales by (sx, sy)
// around the point (cx, cy).
func ScaleAt(sx, sy, cx, cy float64) Affine {
	// Translate to origin, scale, translate back
	return Translate(cx, cy).Multiply(Scale(sx, sy)).Multiply(Translate(-cx, -cy))
}
