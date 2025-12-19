package gg

import "math"

// Polynomial root solvers for quadratic and cubic equations.
// These are used for curve operations like finding extrema and intersections.
//
// Based on algorithms from kurbo (https://github.com/linebender/kurbo)
// with adaptations for Go idioms.

// SolveQuadratic finds real roots of the quadratic equation ax^2 + bx + c = 0.
// Returns roots sorted in ascending order.
//
// The function is numerically robust:
// - If a is zero or nearly zero, treats as linear equation
// - If all coefficients are zero, returns a single 0.0
// - Handles edge cases with NaN and Inf gracefully
func SolveQuadratic(a, b, c float64) []float64 {
	// Scale coefficients to avoid overflow in discriminant calculation
	sc0 := c / a
	sc1 := b / a

	// Check if coefficients are valid (not Inf/NaN)
	if !isFinite(sc0) || !isFinite(sc1) {
		return solveQuadraticLinear(b, c)
	}

	// Normal case: valid quadratic
	return solveQuadraticNormal(sc0, sc1)
}

// solveQuadraticNormal handles the normal quadratic case with valid scaled coefficients.
func solveQuadraticNormal(sc0, sc1 float64) []float64 {
	arg := sc1*sc1 - 4.0*sc0

	if !isFinite(arg) {
		// Overflow in discriminant - use fallback
		return solveQuadraticOverflow(sc0, sc1)
	}

	if arg < 0.0 {
		// No real roots (complex roots)
		return nil
	}
	if arg == 0.0 {
		// One double root
		return []float64{-0.5 * sc1}
	}

	// Two distinct roots
	// Use numerically stable formula to avoid cancellation
	// See: https://math.stackexchange.com/questions/866331
	root1 := -0.5 * (sc1 + math.Copysign(math.Sqrt(arg), sc1))
	root2 := sc0 / root1

	if !isFinite(root2) {
		return []float64{root1}
	}

	// Return sorted
	if root1 > root2 {
		return []float64{root2, root1}
	}
	return []float64{root1, root2}
}

// solveQuadraticOverflow handles discriminant overflow.
func solveQuadraticOverflow(sc0, sc1 float64) []float64 {
	// Find one root using sc1*x + x^2 = 0, other as sc0/root1
	root1 := -sc1
	root2 := sc0 / root1

	if !isFinite(root2) {
		return []float64{root1}
	}

	if root1 > root2 {
		return []float64{root2, root1}
	}
	return []float64{root1, root2}
}

// solveQuadraticLinear handles the case when a is zero or very small.
func solveQuadraticLinear(b, c float64) []float64 {
	root := -c / b
	if isFinite(root) {
		return []float64{root}
	}

	// Degenerate case: all coefficients effectively zero
	if c == 0.0 && b == 0.0 {
		return []float64{0.0}
	}

	return nil
}

// SolveCubic finds real roots of the cubic equation ax^3 + bx^2 + cx + d = 0.
// Returns roots (not necessarily sorted).
//
// The implementation uses the method from:
// https://momentsingraphics.de/CubicRoots.html
// which is based on Jim Blinn's "How to Solve a Cubic Equation".
func SolveCubic(a, b, c, d float64) []float64 {
	// Handle degenerate case where a is zero
	aRecip := 1.0 / a
	const oneThird = 1.0 / 3.0

	scaledB := b * (oneThird * aRecip)
	scaledC := c * (oneThird * aRecip)
	scaledD := d * aRecip

	// Check if scaling resulted in non-finite values (a is zero or too small)
	if !isFinite(scaledB) || !isFinite(scaledC) || !isFinite(scaledD) {
		// Cubic coefficient is zero or nearly so - solve as quadratic
		return SolveQuadratic(b, c, d)
	}

	// Use scaled coefficients
	c0, c1, c2 := scaledD, scaledC, scaledB

	// (d0, d1, d2) is called "Delta" in the article
	d0 := (-c2)*c2 + c1
	d1 := (-c1)*c2 + c0
	d2 := c2*c0 - c1*c1

	// d is called "Discriminant"
	disc := 4.0*d0*d2 - d1*d1

	// de is called "Depressed.x", Depressed.y = d0
	de := (-2.0*c2)*d0 + d1

	if disc < 0.0 {
		// One real root
		sq := math.Sqrt(-0.25 * disc)
		r := -0.5 * de
		t1 := math.Cbrt(r+sq) + math.Cbrt(r-sq)
		return []float64{t1 - c2}
	} else if disc == 0.0 {
		// Two real roots (one is a double root)
		t1 := math.Copysign(math.Sqrt(-d0), de)
		return []float64{t1 - c2, -2.0*t1 - c2}
	}

	// Three distinct real roots
	th := math.Atan2(math.Sqrt(disc), -de) * oneThird
	thSin, thCos := math.Sincos(th)

	r0 := thCos
	ss3 := thSin * math.Sqrt(3.0)
	r1 := 0.5 * (-thCos + ss3)
	r2 := 0.5 * (-thCos - ss3)
	t := 2.0 * math.Sqrt(-d0)

	return []float64{
		t*r0 - c2,
		t*r1 - c2,
		t*r2 - c2,
	}
}

// SolveQuadraticInUnitInterval returns roots of ax^2 + bx + c = 0 that lie in [0, 1].
// This is useful for finding parameter values on Bezier curves.
func SolveQuadraticInUnitInterval(a, b, c float64) []float64 {
	roots := SolveQuadratic(a, b, c)
	return filterRootsToUnitInterval(roots)
}

// SolveCubicInUnitInterval returns roots of ax^3 + bx^2 + cx + d = 0 that lie in [0, 1].
// This is useful for finding parameter values on Bezier curves.
func SolveCubicInUnitInterval(a, b, c, d float64) []float64 {
	roots := SolveCubic(a, b, c, d)
	return filterRootsToUnitInterval(roots)
}

// filterRootsToUnitInterval filters roots to those in [0, 1].
// Uses a small epsilon to handle numerical precision issues at boundaries.
func filterRootsToUnitInterval(roots []float64) []float64 {
	if len(roots) == 0 {
		return nil
	}

	const eps = 1e-12
	result := make([]float64, 0, len(roots))
	for _, r := range roots {
		// Clamp values very close to boundaries
		if r >= -eps && r <= 1.0+eps {
			// Clamp to exact boundary values
			if r < 0.0 {
				r = 0.0
			} else if r > 1.0 {
				r = 1.0
			}
			result = append(result, r)
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// isFinite returns true if x is neither infinite nor NaN.
func isFinite(x float64) bool {
	return !math.IsInf(x, 0) && !math.IsNaN(x)
}
