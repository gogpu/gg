// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"fmt"
	"math"
	"testing"
)

// TestCubicWaviness creates a circle using 4 standard cubic beziers and measures
// the forward-difference deviation from the true cubic bezier curve at each
// segment endpoint. This diagnoses the root cause of visible waviness in large
// circles rendered via CubicEdge forward differencing.
//
// The circle is R=150 centered at (250, 350). Standard kappa=0.5522847498
// approximation gives 4 quarter-arc cubics.
//
// For each cubic, we:
//  1. Create a CubicEdge with shift=2 (4x AA)
//  2. Step through ALL segments via the forward-diff engine
//  3. At each segment endpoint, compute the TRUE cubic bezier X for that Y
//  4. Report deviation |actual_x - true_x|
func TestCubicWaviness(t *testing.T) {
	const (
		cx    = float32(250.0) // circle center X
		cy    = float32(350.0) // circle center Y
		r     = float32(150.0) // radius
		kappa = float32(0.5522847498)
		shift = 2 // 4x AA (kDefaultAccuracy)
	)

	// 4 quarter-arc cubics for a circle.
	// Standard construction: control points offset by kappa*R along tangent.
	type cubicArc struct {
		name           string
		p0, p1, p2, p3 CurvePoint
	}

	arcs := []cubicArc{
		{
			name: "right->bottom (3 o'clock -> 6 o'clock)",
			p0:   CurvePoint{X: cx + r, Y: cy},
			p1:   CurvePoint{X: cx + r, Y: cy + r*kappa},
			p2:   CurvePoint{X: cx + r*kappa, Y: cy + r},
			p3:   CurvePoint{X: cx, Y: cy + r},
		},
		{
			name: "bottom->left (6 o'clock -> 9 o'clock)",
			p0:   CurvePoint{X: cx, Y: cy + r},
			p1:   CurvePoint{X: cx - r*kappa, Y: cy + r},
			p2:   CurvePoint{X: cx - r, Y: cy + r*kappa},
			p3:   CurvePoint{X: cx - r, Y: cy},
		},
		{
			name: "left->top (9 o'clock -> 12 o'clock)",
			p0:   CurvePoint{X: cx - r, Y: cy},
			p1:   CurvePoint{X: cx - r, Y: cy - r*kappa},
			p2:   CurvePoint{X: cx - r*kappa, Y: cy - r},
			p3:   CurvePoint{X: cx, Y: cy - r},
		},
		{
			name: "top->right (12 o'clock -> 3 o'clock)",
			p0:   CurvePoint{X: cx, Y: cy - r},
			p1:   CurvePoint{X: cx + r*kappa, Y: cy - r},
			p2:   CurvePoint{X: cx + r, Y: cy - r*kappa},
			p3:   CurvePoint{X: cx + r, Y: cy},
		},
	}

	for _, arc := range arcs {
		t.Run(arc.name, func(t *testing.T) {
			diagnoseCubicArc(t, arc.name, arc.p0, arc.p1, arc.p2, arc.p3, shift)
		})
	}
}

// diagnoseCubicArc creates a CubicEdge and steps through all segments,
// comparing forward-diff output against the analytical cubic bezier truth.
func diagnoseCubicArc(t *testing.T, name string, p0, p1, p2, p3 CurvePoint, shift int) {
	t.Helper()

	// Create the cubic edge WITHOUT calling Update() so we can inspect initial state.
	// We use newCubicEdgeSetup, then manually call Update() in a loop.
	cubic, cubicOK := newCubicEdgeSetup(p0, p1, p2, p3, shift)
	if !cubicOK {
		t.Fatalf("newCubicEdgeSetup returned false for arc %s", name)
	}

	curveShift := int(cubic.curveShift)
	totalSegments := 1 << curveShift
	dshift := int(cubic.dshift)

	t.Logf("Arc: %s", name)
	t.Logf("  P0=(%.1f, %.1f) P1=(%.1f, %.1f) P2=(%.1f, %.1f) P3=(%.1f, %.1f)",
		p0.X, p0.Y, p1.X, p1.Y, p2.X, p2.Y, p3.X, p3.Y)
	t.Logf("  curveShift=%d (segments=%d), dshift=%d", curveShift, totalSegments, dshift)
	t.Logf("  Initial: cx=%d (%.4f px), cy=%d (%.4f px)",
		cubic.cx, FDot16ToFloat64(cubic.cx), cubic.cy, FDot16ToFloat64(cubic.cy))

	// Determine the actual control points used (may be swapped for winding).
	// The cubic uses FDot6-scaled coordinates internally. We need the original
	// control points for analytical evaluation.
	actualP0, actualP1, actualP2, actualP3 := p0, p1, p2, p3
	if cubic.line.Winding == -1 {
		// Points were swapped to ensure y0 <= y3
		actualP0, actualP3 = p3, p0
		actualP1, actualP2 = p2, p1
	}

	// Collect all segment endpoints by stepping through the forward-diff engine.
	type segPoint struct {
		x, y float64 // in pixel coordinates (from FDot16)
	}

	var points []segPoint
	// Add the starting point
	points = append(points, segPoint{
		x: FDot16ToFloat64(cubic.cx),
		y: FDot16ToFloat64(cubic.cy),
	})

	segIdx := 0
	for cubic.Update() {
		segIdx++
		// After Update(), the line segment has been set.
		// The cubic's cx/cy now contain the END of the current segment.
		points = append(points, segPoint{
			x: FDot16ToFloat64(cubic.cx),
			y: FDot16ToFloat64(cubic.cy),
		})
	}

	t.Logf("  Produced %d segments (%d points including start)", segIdx, len(points))

	// Now compute deviations. For each point, find the TRUE cubic bezier X at that Y.
	var maxDevX, maxDevY float64
	var sumDevX float64
	worstIdx := 0

	for i, pt := range points {
		// Find t such that B_y(t) = pt.y
		trueT := solveCubicForY(
			float64(actualP0.Y), float64(actualP1.Y),
			float64(actualP2.Y), float64(actualP3.Y),
			pt.y,
		)
		// Compute true X at that t
		trueX := evalCubic(
			float64(actualP0.X), float64(actualP1.X),
			float64(actualP2.X), float64(actualP3.X),
			trueT,
		)

		// Also compute true Y at that t (to check Y accuracy)
		trueY := evalCubic(
			float64(actualP0.Y), float64(actualP1.Y),
			float64(actualP2.Y), float64(actualP3.Y),
			trueT,
		)

		devX := math.Abs(pt.x - trueX)
		devY := math.Abs(pt.y - trueY)

		if devX > maxDevX {
			maxDevX = devX
			worstIdx = i
		}
		if devY > maxDevY {
			maxDevY = devY
		}
		sumDevX += devX

		if devX > 0.3 || i == 0 || i == len(points)-1 {
			t.Logf("  point[%2d]: actual=(%.4f, %.4f) true=(%.4f, %.4f) devX=%.4fpx devY=%.4fpx t=%.6f",
				i, pt.x, pt.y, trueX, trueY, devX, devY, trueT)
		}
	}

	meanDevX := float64(0)
	if len(points) > 0 {
		meanDevX = sumDevX / float64(len(points))
	}

	t.Logf("  SUMMARY: %d points, max_x_dev=%.4fpx, mean_x_dev=%.4fpx, max_y_dev=%.4fpx, worst_point=%d",
		len(points), maxDevX, meanDevX, maxDevY, worstIdx)

	// Report if waviness threshold exceeded
	if maxDevX > 0.5 {
		t.Errorf("  WAVINESS DETECTED: max X deviation %.4fpx > 0.5px threshold at point[%d]", maxDevX, worstIdx)
	}
}

// evalCubic evaluates the cubic bezier B(t) = (1-t)^3*p0 + 3*(1-t)^2*t*p1 + 3*(1-t)*t^2*p2 + t^3*p3.
func evalCubic(p0, p1, p2, p3, t float64) float64 {
	u := 1.0 - t
	return u*u*u*p0 + 3*u*u*t*p1 + 3*u*t*t*p2 + t*t*t*p3
}

// solveCubicForY finds t in [0,1] such that B_y(t) = targetY.
// Uses Newton's method with a bisection fallback.
func solveCubicForY(y0, y1, y2, y3, targetY float64) float64 {
	// Bisection first pass to get a good initial guess
	lo, hi := 0.0, 1.0
	for i := 0; i < 20; i++ {
		mid := (lo + hi) / 2.0
		ym := evalCubic(y0, y1, y2, y3, mid)
		if ym < targetY {
			lo = mid
		} else {
			hi = mid
		}
	}
	t := (lo + hi) / 2.0

	// Newton refinement (derivative of cubic bezier)
	for i := 0; i < 10; i++ {
		yt := evalCubic(y0, y1, y2, y3, t)
		// B'(t) = 3*(1-t)^2*(y1-y0) + 6*(1-t)*t*(y2-y1) + 3*t^2*(y3-y2)
		u := 1.0 - t
		dyt := 3*u*u*(y1-y0) + 6*u*t*(y2-y1) + 3*t*t*(y3-y2)
		if math.Abs(dyt) < 1e-12 {
			break
		}
		newT := t - (yt-targetY)/dyt
		if newT < 0 {
			newT = 0
		}
		if newT > 1 {
			newT = 1
		}
		t = newT
	}

	return t
}

// TestCubicCoefficientPrecision directly inspects the forward-diff coefficients
// for a large circle arc and compares with exact floating-point computation.
//
// This test checks whether the fixed-point coefficient computation loses
// significant precision during the FDot6 -> FDot16 -> >>accuracy chain.
func TestCubicCoefficientPrecision(t *testing.T) {
	const (
		cx    = float32(250.0)
		cy    = float32(350.0)
		r     = float32(150.0)
		kappa = float32(0.5522847498)
		shift = 2
	)

	// First quarter arc: right -> bottom
	p0 := CurvePoint{X: cx + r, Y: cy}
	p1 := CurvePoint{X: cx + r, Y: cy + r*kappa}
	p2 := CurvePoint{X: cx + r*kappa, Y: cy + r}
	p3 := CurvePoint{X: cx, Y: cy + r}

	cubic, cubicOK := newCubicEdgeSetup(p0, p1, p2, p3, shift)
	if !cubicOK {
		t.Fatal("newCubicEdgeSetup returned false")
	}

	curveShift := int(cubic.curveShift)
	upShift := 6
	downShift := curveShift + upShift - 10
	if downShift < 0 {
		downShift = 0
		upShift = 10 - curveShift
	}

	t.Logf("curveShift=%d, upShift=%d, downShift=%d", curveShift, upShift, downShift)
	t.Logf("totalSegments=%d", 1<<curveShift)

	// Compute coefficients in float64 for comparison
	scale := float64(int32(1) << uint(shift+FDot6Shift)) // 256 for shift=2
	fx0 := float64(p0.X) * scale
	fx1 := float64(p1.X) * scale
	fx2 := float64(p2.X) * scale
	fx3 := float64(p3.X) * scale

	// Winding: check if y0 > y3 (would swap)
	fy0 := float64(p0.Y) * scale
	fy3 := float64(p3.Y) * scale
	if fy0 > fy3 {
		t.Logf("Points swapped for winding (y0 > y3)")
		fx0, fx3 = fx3, fx0
		fx1, fx2 = fx2, fx1
	}

	// Exact float64 coefficients (before truncation to int32)
	bF := 3.0 * (fx1 - fx0) * float64(int32(1)<<upShift)
	cF := 3.0 * (fx0 - fx1 - fx1 + fx2) * float64(int32(1)<<upShift)
	dF := (fx3 + 3.0*(fx1-fx2) - fx0) * float64(int32(1)<<upShift)

	t.Logf("Float64 B=%.2f, C=%.2f, D=%.2f", bF, cF, dF)

	// Integer coefficients (as computed by our code)
	ix0 := int32(p0.X * float32(int32(1)<<uint(shift+FDot6Shift)))
	ix1 := int32(p1.X * float32(int32(1)<<uint(shift+FDot6Shift)))
	ix2 := int32(p2.X * float32(int32(1)<<uint(shift+FDot6Shift)))
	ix3 := int32(p3.X * float32(int32(1)<<uint(shift+FDot6Shift)))

	if ix0 > ix3 {
		ix0, ix3 = ix3, ix0
		ix1, ix2 = ix2, ix1
	}

	bI := FDot6UpShift(3*(ix1-ix0), upShift)
	cI := FDot6UpShift(3*(ix0-ix1-ix1+ix2), upShift)
	dI := FDot6UpShift(ix3+3*(ix1-ix2)-ix0, upShift)

	t.Logf("Int32   B=%d, C=%d, D=%d", bI, cI, dI)
	t.Logf("B error: %.6f, C error: %.6f, D error: %.6f",
		float64(bI)-bF, float64(cI)-cF, float64(dI)-dF)

	// Forward-diff first derivative (CDx)
	cdxF := bF + cF/float64(int32(1)<<curveShift) + dF/float64(int64(1)<<(2*curveShift))
	cdxI := bI + (cI >> uint(curveShift)) + (dI >> uint(2*curveShift))
	t.Logf("CDx: float=%.4f, int=%d, error=%.4f", cdxF, cdxI, float64(cdxI)-cdxF)

	// Second derivative (CDDx)
	cddxF := 2*cF + 3*dF/float64(int32(1)<<(curveShift-1))
	cddxI := 2*cI + ((3 * dI) >> uint(curveShift-1))
	t.Logf("CDDx: float=%.4f, int=%d, error=%.4f", cddxF, cddxI, float64(cddxI)-cddxF)

	// Third derivative (CDDDx)
	cdddxF := 3 * dF / float64(int32(1)<<(curveShift-1))
	cdddxI := (3 * dI) >> uint(curveShift-1)
	t.Logf("CDDDx: float=%.4f, int=%d, error=%.4f", cdddxF, cdddxI, float64(cdddxI)-cdddxF)

	// After >>= accuracy (divide by 4)
	const accuracy = 2
	t.Logf("After >>%d: CDx=%d (%.4f px), CDDx=%d (%.4f px), CDDDx=%d (%.4f px)",
		accuracy,
		cdxI>>accuracy, FDot16ToFloat64(cdxI>>accuracy),
		cddxI>>accuracy, FDot16ToFloat64(cddxI>>accuracy),
		cdddxI>>accuracy, FDot16ToFloat64(cdddxI>>accuracy))

	// Simulate the forward-diff stepping and accumulate error
	t.Logf("\n--- Simulated stepping (X only) ---")
	x := cubic.cx
	cdx := cubic.cdx
	cddx := cubic.cddx
	cdddx := cubic.cdddx

	// Also compute via float64 stepping
	xF := float64(cubic.cx)
	cdxFStep := float64(cubic.cdx)
	cddxFStep := float64(cubic.cddx)
	cdddxFStep := float64(cubic.cdddx)

	segments := 1 << curveShift
	dshiftVal := int(cubic.dshift)
	ddshiftVal := int(cubic.curveShift)

	var maxAccumErr float64
	for i := 0; i < segments; i++ {
		// Integer stepping (matches CubicEdge.Update)
		newx := x + (cdx >> dshiftVal)
		cdx += cddx >> ddshiftVal
		cddx += cdddx

		// Float stepping (same formulas, no truncation)
		newxF := xF + cdxFStep/float64(int32(1)<<dshiftVal)
		cdxFStep += cddxFStep / float64(int32(1)<<ddshiftVal)
		cddxFStep += cdddxFStep

		accumErr := math.Abs(float64(newx) - newxF)
		if accumErr > maxAccumErr {
			maxAccumErr = accumErr
		}

		if i < 5 || i >= segments-3 || accumErr > float64(FDot16One)/4 {
			t.Logf("  step[%2d]: int_x=%d (%.4f px), float_x=%.1f (%.4f px), accum_err=%.1f (%.4f px)",
				i, newx, FDot16ToFloat64(newx), newxF, newxF/float64(FDot16One), accumErr, accumErr/float64(FDot16One))
		}

		x = newx
		xF = newxF
	}

	t.Logf("Max accumulation error: %.1f units (%.4f px)", maxAccumErr, maxAccumErr/float64(FDot16One))
}

// TestCubicVsSkiaCoefficients compares our coefficient computation against
// a hand-computed Skia-equivalent computation for a specific circle arc.
// This validates that each step in newCubicEdgeSetup matches Skia's
// setCubicWithoutUpdate exactly.
func TestCubicVsSkiaCoefficients(t *testing.T) {
	const (
		cx    = float32(250.0)
		cy    = float32(350.0)
		r     = float32(150.0)
		kappa = float32(0.5522847498)
		shift = 2
	)

	// Right -> Bottom arc
	p0 := CurvePoint{X: cx + r, Y: cy}
	p1 := CurvePoint{X: cx + r, Y: cy + r*kappa}
	p2 := CurvePoint{X: cx + r*kappa, Y: cy + r}
	p3 := CurvePoint{X: cx, Y: cy + r}

	// Step 1: Convert to FDot6 with AA scaling (matches Skia line 572-581)
	scale := float32(int32(1) << uint(shift+FDot6Shift)) // 256 for shift=2
	x0 := int32(p0.X * scale)
	y0 := int32(p0.Y * scale)
	x1 := int32(p1.X * scale)
	y1 := int32(p1.Y * scale)
	x2 := int32(p2.X * scale)
	y2 := int32(p2.Y * scale)
	x3 := int32(p3.X * scale)
	y3 := int32(p3.Y * scale)

	t.Logf("FDot6 scaled: x0=%d y0=%d x1=%d y1=%d x2=%d y2=%d x3=%d y3=%d",
		x0, y0, x1, y1, x2, y2, x3, y3)

	// Step 2: Compute curveShift (matches Skia lines 602-616)
	dx := cubicDeltaFromLine(x0, x1, x2, x3)
	dy := cubicDeltaFromLine(y0, y1, y2, y3)
	curveShift := diffToShift(dx, dy, 2) + 1
	if curveShift < 1 {
		curveShift = 1
	}
	if curveShift > MaxCoeffShift {
		curveShift = MaxCoeffShift
	}

	t.Logf("cubicDelta: dx=%d, dy=%d, curveShift=%d (segments=%d)", dx, dy, curveShift, 1<<curveShift)

	// Step 3: Compute upShift/downShift (matches Skia lines 621-627)
	upShift := 6
	downShift := curveShift + upShift - 10
	if downShift < 0 {
		downShift = 0
		upShift = 10 - curveShift
	}

	t.Logf("upShift=%d, downShift=%d", upShift, downShift)

	// Step 4: Compute coefficients (matches Skia lines 635-651)
	b := FDot6UpShift(3*(x1-x0), upShift)
	c := FDot6UpShift(3*(x0-x1-x1+x2), upShift)
	d := FDot6UpShift(x3+3*(x1-x2)-x0, upShift)

	t.Logf("Raw coefficients X: B=%d, C=%d, D=%d", b, c, d)

	fCx := FDot6ToFDot16(x0)
	fCDx := b + (c >> uint(curveShift)) + (d >> uint(2*curveShift))
	fCDDx := 2*c + ((3 * d) >> uint(curveShift-1))
	fCDDDx := (3 * d) >> uint(curveShift-1)

	t.Logf("Before >>accuracy: Cx=%d, CDx=%d, CDDx=%d, CDDDx=%d", fCx, fCDx, fCDDx, fCDDDx)

	// Step 5: >>= accuracy (matches Skia lines 521-528 / our 882-894)
	const accuracy = 2
	fCx >>= accuracy
	fCDx >>= accuracy
	fCDDx >>= accuracy
	fCDDDx >>= accuracy

	t.Logf("After >>accuracy: Cx=%d (%.4f px), CDx=%d, CDDx=%d, CDDDx=%d",
		fCx, FDot16ToFloat64(fCx), fCDx, fCDDx, fCDDDx)

	// Step 6: Compare with what newCubicEdgeSetup actually produces
	cubic, cubicOK := newCubicEdgeSetup(p0, p1, p2, p3, shift)
	if !cubicOK {
		t.Fatal("newCubicEdgeSetup returned false")
	}

	// Check each coefficient
	checkCoeff := func(name string, expected, actual int32) {
		if expected != actual {
			t.Errorf("%s: expected %d, got %d (diff=%d)", name, expected, actual, actual-expected)
		} else {
			t.Logf("%s: %d (match)", name, actual)
		}
	}

	checkCoeff("cx", fCx, cubic.cx)
	checkCoeff("cdx", fCDx, cubic.cdx)
	checkCoeff("cddx", fCDDx, cubic.cddx)
	checkCoeff("cdddx", fCDDDx, cubic.cdddx)
	checkCoeff("curveShift", int32(curveShift), int32(cubic.curveShift))
	checkCoeff("dshift", int32(downShift), int32(cubic.dshift))
}

// TestCubicCircleRadiusDeviation measures the radial deviation of forward-diff
// points from the ideal circle. For a circle of radius R, each point should be
// at distance R from center. The deviation from R is the "waviness" metric.
func TestCubicCircleRadiusDeviation(t *testing.T) {
	const (
		cx    = float32(250.0)
		cy    = float32(350.0)
		r     = float32(150.0)
		kappa = float32(0.5522847498)
		shift = 2
	)

	// Collect ALL forward-diff points from ALL 4 quarter arcs
	type arcDef struct {
		name           string
		p0, p1, p2, p3 CurvePoint
	}

	arcs := []arcDef{
		{"right->bottom", CurvePoint{cx + r, cy}, CurvePoint{cx + r, cy + r*kappa}, CurvePoint{cx + r*kappa, cy + r}, CurvePoint{cx, cy + r}},
		{"bottom->left", CurvePoint{cx, cy + r}, CurvePoint{cx - r*kappa, cy + r}, CurvePoint{cx - r, cy + r*kappa}, CurvePoint{cx - r, cy}},
		{"left->top", CurvePoint{cx - r, cy}, CurvePoint{cx - r, cy - r*kappa}, CurvePoint{cx - r*kappa, cy - r}, CurvePoint{cx, cy - r}},
		{"top->right", CurvePoint{cx, cy - r}, CurvePoint{cx + r*kappa, cy - r}, CurvePoint{cx + r, cy - r*kappa}, CurvePoint{cx + r, cy}},
	}

	var allMaxDev float64
	var allMaxArc string

	for _, arc := range arcs {
		cubic, cubicOK := newCubicEdgeSetup(arc.p0, arc.p1, arc.p2, arc.p3, shift)
		if !cubicOK {
			t.Logf("  %s: degenerate (zero height)", arc.name)
			continue
		}

		// Collect points
		var maxRadDev float64
		segCount := 0

		// Check starting point
		px := FDot16ToFloat64(cubic.cx)
		py := FDot16ToFloat64(cubic.cy)
		dist := math.Sqrt((px-float64(cx))*(px-float64(cx)) + (py-float64(cy))*(py-float64(cy)))
		dev := math.Abs(dist - float64(r))
		if dev > maxRadDev {
			maxRadDev = dev
		}

		for cubic.Update() {
			segCount++
			px = FDot16ToFloat64(cubic.cx)
			py = FDot16ToFloat64(cubic.cy)
			dist = math.Sqrt((px-float64(cx))*(px-float64(cx)) + (py-float64(cy))*(py-float64(cy)))
			dev = math.Abs(dist - float64(r))
			if dev > maxRadDev {
				maxRadDev = dev
			}
		}

		t.Logf("  %s: %d segments, max radial deviation = %.4f px", arc.name, segCount, maxRadDev)

		if maxRadDev > allMaxDev {
			allMaxDev = maxRadDev
			allMaxArc = arc.name
		}
	}

	t.Logf("Overall max radial deviation: %.4f px (in %s)", allMaxDev, allMaxArc)

	// The cubic bezier approximation itself has inherent error ~0.027% for kappa=0.5523.
	// For R=150, that's about 0.04px. Forward-diff should not add more than ~0.2px.
	// If total > 0.5px, it indicates a precision problem.
	if allMaxDev > 0.5 {
		t.Errorf("Radial deviation %.4fpx exceeds 0.5px threshold — forward-diff precision issue", allMaxDev)
	}

	// Also note the kappa approximation error for reference
	kappaErr := float64(r) * 0.00027 // ~0.027% for standard kappa
	t.Logf("Reference: cubic kappa approximation error = %.4f px (expected baseline)", kappaErr)
}

// TestCubicForwardDiffAccumulation directly simulates forward-diff stepping
// in float64 vs int32 to isolate truncation error accumulation.
// No CubicEdge creation — just raw coefficient stepping.
func TestCubicForwardDiffAccumulation(t *testing.T) {
	const (
		cx    = float32(250.0)
		cy    = float32(350.0)
		r     = float32(150.0)
		kappa = float32(0.5522847498)
		shift = 2
	)

	p0 := CurvePoint{X: cx + r, Y: cy}
	p1 := CurvePoint{X: cx + r, Y: cy + r*kappa}
	p2 := CurvePoint{X: cx + r*kappa, Y: cy + r}
	p3 := CurvePoint{X: cx, Y: cy + r}

	cubic, cubicOK := newCubicEdgeSetup(p0, p1, p2, p3, shift)
	if !cubicOK {
		t.Fatal("newCubicEdgeSetup returned false")
	}

	// Raw stepping in both int32 and float64
	ix := cubic.cx
	idx := cubic.cdx
	iddx := cubic.cddx
	idddx := cubic.cdddx

	fx := float64(cubic.cx)
	fdx := float64(cubic.cdx)
	fddx := float64(cubic.cddx)
	fdddx := float64(cubic.cdddx)

	segments := 1 << int(cubic.curveShift)
	dsh := int(cubic.dshift)
	ddsh := int(cubic.curveShift)

	t.Logf("Stepping %d segments, dshift=%d, ddshift=%d", segments, dsh, ddsh)
	t.Logf("Initial: ix=%d, idx=%d, iddx=%d, idddx=%d", ix, idx, iddx, idddx)

	var maxErr float64
	for i := 0; i < segments; i++ {
		// Integer step
		ix += idx >> dsh
		idx += iddx >> ddsh
		iddx += idddx

		// Float step
		fx += fdx / float64(int32(1)<<dsh)
		fdx += fddx / float64(int32(1)<<ddsh)
		fddx += fdddx

		err := math.Abs(float64(ix) - fx)
		errPx := err / float64(FDot16One)
		if err > maxErr {
			maxErr = err
		}

		if i < 5 || i >= segments-3 || errPx > 0.1 {
			t.Logf("  step[%2d]: int=%d (%.4fpx), float=%.1f (%.4fpx), err=%.4fpx",
				i, ix, FDot16ToFloat64(ix), fx, fx/float64(FDot16One), errPx)
		}
	}

	maxErrPx := maxErr / float64(FDot16One)
	t.Logf("Max int vs float accumulation error: %.4f px", maxErrPx)

	// Pure integer truncation in forward-diff should accumulate at most
	// O(N) * 0.5 LSB per step. For 32 segments and 16.16 fixed point,
	// that's about 32 * 0.5 / 65536 = 0.00024 px. Should be tiny.
	if maxErrPx > 0.01 {
		t.Errorf("Integer truncation accumulation %.4fpx exceeds 0.01px — check >>= logic", maxErrPx)
	}
}

// TestCubicWavinessComparison compares forward-diff output against geometric
// flattening (converting cubic to line segments). This quantifies the visual
// difference between the two approaches.
func TestCubicWavinessComparison(t *testing.T) {
	const (
		cx    = float32(250.0)
		cy    = float32(350.0)
		r     = float32(150.0)
		kappa = float32(0.5522847498)
		shift = 2
	)

	// Right -> bottom arc
	p0 := CurvePoint{X: cx + r, Y: cy}
	p1 := CurvePoint{X: cx + r, Y: cy + r*kappa}
	p2 := CurvePoint{X: cx + r*kappa, Y: cy + r}
	p3 := CurvePoint{X: cx, Y: cy + r}

	// Forward-diff points
	cubic, cubicOK := newCubicEdgeSetup(p0, p1, p2, p3, shift)
	if !cubicOK {
		t.Fatal("newCubicEdgeSetup returned false")
	}

	type pt struct{ x, y float64 }
	var fdPoints []pt
	fdPoints = append(fdPoints, pt{FDot16ToFloat64(cubic.cx), FDot16ToFloat64(cubic.cy)})
	for cubic.Update() {
		fdPoints = append(fdPoints, pt{FDot16ToFloat64(cubic.cx), FDot16ToFloat64(cubic.cy)})
	}

	// Float64 forward-diff points (no truncation)
	cubic2, _ := newCubicEdgeSetup(p0, p1, p2, p3, shift)
	fx := float64(cubic2.cx)
	fy := float64(cubic2.cy)
	fdx := float64(cubic2.cdx)
	fdy := float64(cubic2.cdy)
	fddx := float64(cubic2.cddx)
	fddy := float64(cubic2.cddy)
	fdddx := float64(cubic2.cdddx)
	fdddy := float64(cubic2.cdddy)
	dsh := int(cubic2.dshift)
	ddsh := int(cubic2.curveShift)
	segments := 1 << int(cubic2.curveShift)

	var floatPoints []pt
	floatPoints = append(floatPoints, pt{fx / float64(FDot16One), fy / float64(FDot16One)})
	for i := 0; i < segments; i++ {
		fx += fdx / float64(int32(1)<<dsh)
		fdx += fddx / float64(int32(1)<<ddsh)
		fddx += fdddx

		fy += fdy / float64(int32(1)<<dsh)
		fdy += fddy / float64(int32(1)<<ddsh)
		fddy += fdddy

		floatPoints = append(floatPoints, pt{fx / float64(FDot16One), fy / float64(FDot16One)})
	}

	// Compare
	maxDev := 0.0
	n := len(fdPoints)
	if len(floatPoints) < n {
		n = len(floatPoints)
	}
	for i := 0; i < n; i++ {
		devX := math.Abs(fdPoints[i].x - floatPoints[i].x)
		devY := math.Abs(fdPoints[i].y - floatPoints[i].y)
		dev := math.Max(devX, devY)
		if dev > maxDev {
			maxDev = dev
		}
	}

	t.Logf("Forward-diff: %d int32 points, %d float64 points", len(fdPoints), len(floatPoints))
	t.Logf("Max deviation int32 vs float64: %.6f px", maxDev)

	// Also report radial deviation for float64 points (to separate
	// cubic-approximation error from integer-truncation error)
	var maxFloatRadDev float64
	for _, p := range floatPoints {
		dist := math.Sqrt((p.x-float64(cx))*(p.x-float64(cx)) + (p.y-float64(cy))*(p.y-float64(cy)))
		dev := math.Abs(dist - float64(r))
		if dev > maxFloatRadDev {
			maxFloatRadDev = dev
		}
	}
	t.Logf("Float64 max radial deviation from circle: %.4f px (= cubic kappa error only)", maxFloatRadDev)

	var maxIntRadDev float64
	for _, p := range fdPoints {
		dist := math.Sqrt((p.x-float64(cx))*(p.x-float64(cx)) + (p.y-float64(cy))*(p.y-float64(cy)))
		dev := math.Abs(dist - float64(r))
		if dev > maxIntRadDev {
			maxIntRadDev = dev
		}
	}
	t.Logf("Int32 max radial deviation from circle: %.4f px (= kappa + truncation)", maxIntRadDev)

	addedError := maxIntRadDev - maxFloatRadDev
	t.Logf("Error added by integer truncation: %.4f px", addedError)

	// Print summary
	fmt.Printf("\n=== WAVINESS DIAGNOSIS ===\n")
	fmt.Printf("Cubic kappa approximation error: %.4f px\n", maxFloatRadDev)
	fmt.Printf("Integer truncation added error:  %.4f px\n", addedError)
	fmt.Printf("Total radial deviation:          %.4f px\n", maxIntRadDev)
	fmt.Printf("Segments: %d (curveShift=%d)\n", segments, cubic.curveShift)
	if maxIntRadDev > 0.5 {
		fmt.Printf("VERDICT: WAVINESS — total deviation exceeds 0.5px threshold\n")
	} else {
		fmt.Printf("VERDICT: OK — within 0.5px threshold\n")
	}
}
