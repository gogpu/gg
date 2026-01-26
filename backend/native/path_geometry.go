// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"math"
)

// Path Geometry Functions for Y-Monotonic Curve Processing
//
// This file implements algorithms for splitting Bezier curves at Y extrema
// to ensure each resulting curve segment is monotonic in Y. This is required
// for scanline rasterization where we need curves that always go either
// up or down (never changing direction).
//
// For a curve to be Y-monotonic: dy/dt must not change sign.
//
// Reference: tiny-skia/src/path_geometry.rs

// GeomPoint represents a 2D point for geometry calculations.
// Using a local type to avoid coupling with scene.Point and CurvePoint.
type GeomPoint struct {
	X, Y float32
}

// ZeroGeomPoint returns the origin point.
func ZeroGeomPoint() GeomPoint {
	return GeomPoint{0, 0}
}

// ChopQuadAtYExtrema chops a quadratic Bezier at its Y extremum (if any).
//
// A quadratic Bezier can have at most one Y extremum. If the curve is not
// monotonic in Y, this function splits it at the extremum point.
//
// Parameters:
//   - src: 3 control points [p0, p1, p2]
//   - dst: output array, must have capacity for 5 points
//
// Returns:
//   - number of chops (0 or 1)
//   - 0 means dst[0..3] contains the original (already monotonic)
//   - 1 means dst[0..3] and dst[2..5] are two monotonic quads
//
// The control points are structured as:
//   - dst[0] = first quad start
//   - dst[1] = first quad control
//   - dst[2] = first quad end = second quad start (shared)
//   - dst[3] = second quad control
//   - dst[4] = second quad end
func ChopQuadAtYExtrema(src [3]GeomPoint, dst *[5]GeomPoint) int {
	a := src[0].Y
	b := src[1].Y
	c := src[2].Y

	if isNotMonotonic(a, b, c) { //nolint:nestif // Logic is interconnected, splitting would reduce clarity
		// Find t where derivative is zero: dy/dt = 0
		// For quadratic: dy/dt = 2(1-t)(b-a) + 2t(c-b)
		// Setting to 0: (a-b) = t(a - 2b + c)
		// t = (a-b) / (a - 2b + c)
		t := validUnitDivide(a-b, a-2*b+c)
		if t > 0 && t < 1 {
			chopQuadAt(src, t, dst)

			// Ensure monotonicity by clamping control points to their segment's Y range.
			// This handles numerical precision issues without distorting the curve shape.
			//
			// First segment: dst[0] -> dst[1] -> dst[2]
			// Control point dst[1].Y should be between dst[0].Y and dst[2].Y
			minY1 := minF32(dst[0].Y, dst[2].Y)
			maxY1 := maxF32(dst[0].Y, dst[2].Y)
			if dst[1].Y < minY1 {
				dst[1].Y = minY1
			} else if dst[1].Y > maxY1 {
				dst[1].Y = maxY1
			}

			// Second segment: dst[2] -> dst[3] -> dst[4]
			// Control point dst[3].Y should be between dst[2].Y and dst[4].Y
			minY2 := minF32(dst[2].Y, dst[4].Y)
			maxY2 := maxF32(dst[2].Y, dst[4].Y)
			if dst[3].Y < minY2 {
				dst[3].Y = minY2
			} else if dst[3].Y > maxY2 {
				dst[3].Y = maxY2
			}

			return 1
		}

		// If we can't compute valid t (underflow), force monotonicity
		// by adjusting the control point
		if absF32(a-b) < absF32(b-c) {
			b = a
		} else {
			b = c
		}
	}

	// Already monotonic or forced monotonic - copy with adjusted b
	dst[0] = GeomPoint{src[0].X, a}
	dst[1] = GeomPoint{src[1].X, b}
	dst[2] = GeomPoint{src[2].X, c}
	return 0
}

// ChopCubicAtYExtrema chops a cubic Bezier at its Y extrema (if any).
//
// A cubic Bezier can have up to 2 Y extrema. This function splits the curve
// at all extrema to produce 1-3 monotonic cubic segments.
//
// Parameters:
//   - src: 4 control points [p0, p1, p2, p3]
//   - dst: output array, must have capacity for 10 points
//
// Returns:
//   - number of chops (0, 1, or 2)
//   - 0 means dst[0..4] contains the original (already monotonic)
//   - 1 means dst[0..4] and dst[3..7] are two monotonic cubics
//   - 2 means dst[0..4], dst[3..7], and dst[6..10] are three monotonic cubics
func ChopCubicAtYExtrema(src [4]GeomPoint, dst *[10]GeomPoint) int {
	// Find t values where dy/dt = 0
	// Cubic derivative: dy/dt = 3(1-t)^2(p1-p0) + 6(1-t)t(p2-p1) + 3t^2(p3-p2)
	// This is a quadratic in t: At^2 + Bt + C = 0
	// A = 3(-p0 + 3p1 - 3p2 + p3) / 3 = -a + 3b - 3c + d
	// B = 6(p0 - 2p1 + p2) / 3 = 2(a - 2b + c)
	// C = 3(p1 - p0) / 3 = b - a
	//
	// Simplified (dividing by 3):
	// A = d - a + 3(b - c)
	// B = 2(a - 2b + c)
	// C = b - a
	a := src[0].Y
	b := src[1].Y
	c := src[2].Y
	d := src[3].Y

	tValues := findCubicExtrema(a, b, c, d)
	numChops := len(tValues)

	chopCubicAt(src, tValues, dst)

	// Ensure monotonicity by clamping control points to their segment's Y range.
	// This handles numerical precision issues without distorting the curve shape.
	//
	// For cubic, each segment has 4 points: p0, p1, p2, p3
	// Control points p1 and p2 should be clamped to [min(p0.Y, p3.Y), max(p0.Y, p3.Y)]

	// Helper to clamp cubic control points
	clampCubicControlPoints := func(startIdx int) {
		p0Y := dst[startIdx].Y
		p3Y := dst[startIdx+3].Y
		minY := minF32(p0Y, p3Y)
		maxY := maxF32(p0Y, p3Y)

		// Clamp p1 (startIdx+1)
		if dst[startIdx+1].Y < minY {
			dst[startIdx+1].Y = minY
		} else if dst[startIdx+1].Y > maxY {
			dst[startIdx+1].Y = maxY
		}

		// Clamp p2 (startIdx+2)
		if dst[startIdx+2].Y < minY {
			dst[startIdx+2].Y = minY
		} else if dst[startIdx+2].Y > maxY {
			dst[startIdx+2].Y = maxY
		}
	}

	// Always clamp first segment (dst[0..4])
	clampCubicControlPoints(0)

	if numChops >= 1 {
		// Clamp second segment (dst[3..7])
		clampCubicControlPoints(3)
	}
	if numChops >= 2 {
		// Clamp third segment (dst[6..10])
		clampCubicControlPoints(6)
	}

	return numChops
}

// isNotMonotonic returns true if the three values do NOT form a monotonic sequence.
// A sequence is monotonic if it's either non-decreasing or non-increasing throughout.
func isNotMonotonic(a, b, c float32) bool {
	ab := a - b
	bc := b - c

	// If ab and bc have the same sign (or one is zero), it's monotonic
	// Not monotonic when: ab > 0 and bc < 0, or ab < 0 and bc > 0

	// Normalize bc to have same sign expectation as ab
	if ab < 0 {
		bc = -bc
	}

	// Not monotonic if ab is zero (horizontal then changes) or bc is negative
	return ab == 0 || bc < 0
}

// validUnitDivide performs division and returns the result only if it's in (0, 1).
// Returns 0 if the result is not a valid unit value.
func validUnitDivide(numer, denom float32) float32 {
	if denom == 0 {
		return 0
	}

	t := numer / denom

	// Check for valid range (0, 1) exclusive
	if t > 0 && t < 1 {
		// Additional validation for numerical stability
		if math.IsNaN(float64(t)) || math.IsInf(float64(t), 0) {
			return 0
		}
		return t
	}

	return 0
}

// chopQuadAt splits a quadratic Bezier at parameter t using De Casteljau's algorithm.
//
// The algorithm:
//  1. Linearly interpolate between adjacent points to get intermediate points
//  2. Linearly interpolate between those to get the split point
//
// Result: Two quadratics that share the split point (dst[2]).
func chopQuadAt(src [3]GeomPoint, t float32, dst *[5]GeomPoint) {
	// First level interpolation
	ab := lerpPoint(src[0], src[1], t)
	bc := lerpPoint(src[1], src[2], t)

	// Second level - the split point
	abbc := lerpPoint(ab, bc, t)

	// First quadratic: src[0], ab, abbc
	dst[0] = src[0]
	dst[1] = ab
	dst[2] = abbc

	// Second quadratic: abbc, bc, src[2]
	dst[3] = bc
	dst[4] = src[2]
}

// chopCubicAt splits a cubic Bezier at the given t values.
//
// Parameters:
//   - src: 4 control points
//   - tValues: t values to split at (must be sorted, 0-2 values)
//   - dst: output points
func chopCubicAt(src [4]GeomPoint, tValues []float32, dst *[10]GeomPoint) {
	if len(tValues) == 0 {
		// No chops - copy original
		dst[0] = src[0]
		dst[1] = src[1]
		dst[2] = src[2]
		dst[3] = src[3]
		return
	}

	// First chop
	t := tValues[0]
	chopCubicAtSingle(src, t, dst)

	if len(tValues) == 1 {
		return
	}

	// Second chop - need to renormalize t value
	// The second t was relative to the original curve
	// After the first chop, we need to find the equivalent t in the remaining curve
	t2 := tValues[1]

	// Renormalize: t2_new = (t2 - t) / (1 - t)
	newT := validUnitDivide(t2-t, 1-t)
	if newT <= 0 {
		// Can't renormalize - create degenerate segment
		dst[7] = src[3]
		dst[8] = src[3]
		dst[9] = src[3]
		return
	}

	// Chop the second half (dst[3..7]) at the renormalized t
	remaining := [4]GeomPoint{dst[3], dst[4], dst[5], dst[6]}
	var secondHalf [10]GeomPoint
	chopCubicAtSingle(remaining, newT, &secondHalf)

	// Copy the result back
	// dst[0..4] stays the same (first segment)
	// dst[3..7] becomes secondHalf[0..4] (middle segment)
	// dst[6..10] becomes secondHalf[3..7] (last segment)
	dst[4] = secondHalf[1]
	dst[5] = secondHalf[2]
	dst[6] = secondHalf[3]
	dst[7] = secondHalf[4]
	dst[8] = secondHalf[5]
	dst[9] = secondHalf[6]
}

// chopCubicAtSingle splits a cubic at a single t value using De Casteljau.
// Writes 7 points to dst (two cubics sharing one point).
func chopCubicAtSingle(src [4]GeomPoint, t float32, dst *[10]GeomPoint) {
	// First level
	ab := lerpPoint(src[0], src[1], t)
	bc := lerpPoint(src[1], src[2], t)
	cd := lerpPoint(src[2], src[3], t)

	// Second level
	abbc := lerpPoint(ab, bc, t)
	bccd := lerpPoint(bc, cd, t)

	// Third level - split point
	abbcbccd := lerpPoint(abbc, bccd, t)

	// First cubic: src[0], ab, abbc, abbcbccd
	dst[0] = src[0]
	dst[1] = ab
	dst[2] = abbc
	dst[3] = abbcbccd

	// Second cubic: abbcbccd, bccd, cd, src[3]
	dst[4] = bccd
	dst[5] = cd
	dst[6] = src[3]
}

// findCubicExtrema finds t values where dy/dt = 0 for a cubic.
// Returns 0, 1, or 2 t values in sorted order, all in (0, 1).
func findCubicExtrema(a, b, c, d float32) []float32 {
	// Cubic derivative: dy/dt = At^2 + Bt + C where
	// A = d - a + 3(b - c)
	// B = 2(a - 2b + c)
	// C = b - a
	//
	// We divide by 3 to simplify (doesn't change roots):
	// na = d - a + 3(b - c)
	// nb = 2(a - 2b + c)
	// nc = b - a
	na := d - a + 3*(b-c)
	nb := 2 * (a - 2*b + c)
	nc := b - a

	return findUnitQuadRoots(na, nb, nc)
}

// findUnitQuadRoots finds roots of at^2 + bt + c = 0 that are in (0, 1).
// Returns roots in sorted order.
func findUnitQuadRoots(a, b, c float32) []float32 {
	const epsilon = 1e-7

	// Handle degenerate cases
	if absF32(a) < epsilon {
		// Linear equation: bt + c = 0
		if absF32(b) < epsilon {
			return nil
		}
		t := -c / b
		if t > 0 && t < 1 {
			return []float32{t}
		}
		return nil
	}

	// Quadratic formula: t = (-b +/- sqrt(b^2 - 4ac)) / 2a
	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		return nil
	}

	sqrtD := float32(math.Sqrt(float64(discriminant)))
	inv2a := 1.0 / (2 * a)

	t1 := (-b - sqrtD) * inv2a
	t2 := (-b + sqrtD) * inv2a

	// Ensure t1 <= t2
	if t1 > t2 {
		t1, t2 = t2, t1
	}

	var roots []float32

	if t1 > epsilon && t1 < 1-epsilon {
		roots = append(roots, t1)
	}
	if t2 > epsilon && t2 < 1-epsilon && absF32(t2-t1) > epsilon {
		roots = append(roots, t2)
	}

	return roots
}

// lerpPoint performs linear interpolation between two points.
func lerpPoint(a, b GeomPoint, t float32) GeomPoint {
	return GeomPoint{
		X: a.X + t*(b.X-a.X),
		Y: a.Y + t*(b.Y-a.Y),
	}
}

// absF32 returns the absolute value of x.
func absF32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// minF32 returns the minimum of a and b.
func minF32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// maxF32 returns the maximum of a and b.
func maxF32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// QuadIsYMonotonic returns true if the quadratic is monotonic in Y.
// A quadratic is Y-monotonic if its control point's Y is between the endpoints' Y values.
// This is more permissive than isNotMonotonic to handle curves flattened at extrema.
func QuadIsYMonotonic(p0, p1, p2 GeomPoint) bool {
	// After chopping at Y extrema, the curve should have consistent Y direction
	// or the control point should be at the extremum (flat region is OK)
	minY := minF32(p0.Y, p2.Y)
	maxY := maxF32(p0.Y, p2.Y)
	// Control point should be within or at the endpoint Y range
	return p1.Y >= minY && p1.Y <= maxY
}

// CubicIsYMonotonic returns true if the cubic is monotonic in Y.
// A cubic is Y-monotonic if it has no Y extrema in (0, 1).
func CubicIsYMonotonic(p0, p1, p2, p3 GeomPoint) bool {
	extrema := findCubicExtrema(p0.Y, p1.Y, p2.Y, p3.Y)
	return len(extrema) == 0
}
