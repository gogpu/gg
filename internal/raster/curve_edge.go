// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package raster

import (
	"math/bits"
)

// Forward Differencing Edge Types for Bezier Curve Rasterization
//
// This file implements QuadraticEdge and CubicEdge types that use forward
// differencing for O(1) per-step curve evaluation during scanline conversion.
//
// Forward Differencing Algorithm:
//
// For a polynomial p(t), instead of evaluating p(t) at each step (expensive),
// we compute the differences between consecutive values. For polynomials,
// the n-th difference is constant, allowing O(1) stepping.
//
// Quadratic Bezier p(t) = At^2 + Bt + C:
//   - First difference:  delta(t) = p(t + h) - p(t) = 2Ath + Ah^2 + Bh
//   - Second difference: delta2 = delta(t + h) - delta(t) = 2Ah^2  (constant!)
//   - Step: newx = oldx + dx; dx += ddx
//
// Cubic Bezier p(t) = At^3 + Bt^2 + Ct + D:
//   - Third difference is constant: dddx = 6Ah^3
//   - Step: newx = oldx + dx; dx += ddx; ddx += dddx
//
// Reference: tiny-skia/src/edge.rs, Skia's SkEdge.cpp

// MaxCoeffShift limits the number of subdivisions for a curve.
// We store 1<<shift in a signed byte (int8), so max value is 1<<6 = 64.
// This limits the number of line segments per curve.
const MaxCoeffShift = 6

// CurveEdger is the interface implemented by curve edges (quadratic, cubic).
// It allows polymorphic handling of different curve types in the AET.
type CurveEdger interface {
	// Update advances the curve to the next line segment.
	// Returns true if a valid segment was produced, false if done.
	Update() bool

	// Line returns the current line segment for AET processing.
	Line() *LineEdge

	// CurveCount returns the remaining number of segments.
	CurveCount() int8

	// Winding returns the winding direction (+1 or -1).
	Winding() int8
}

// CurvePoint represents a 2D point for curve edge construction.
// Using separate type to avoid coupling with scene.Point.
type CurvePoint struct {
	X, Y float32
}

// LineEdge represents a single line segment in the Active Edge Table.
// This is the base type that QuadraticEdge and CubicEdge use internally.
// Derived from tiny-skia's LineEdge.
type LineEdge struct {
	// Linked list pointers (indices into edge array).
	// Using Option<u32> pattern from Rust as nullable int32.
	Prev int32
	Next int32

	// X is the current X position in FDot16 (16.16 fixed-point).
	X FDot16

	// DX is the slope: change in X per scanline (in FDot16).
	DX FDot16

	// FirstY is the first scanline this edge covers.
	FirstY int32

	// LastY is the last scanline this edge covers (inclusive).
	LastY int32

	// Winding indicates direction: +1 for downward, -1 for upward.
	Winding int8
}

// NewLineEdge creates a new line edge from two points.
// Returns nil if the edge is horizontal (no Y extent).
//
// Parameters:
//   - p0, p1: endpoints in pixel coordinates
//   - shift: AA shift (0 for no AA, 2 for 4x AA quality)
//
//nolint:gosec // G115: shift is bounded [0, MaxCoeffShift], conversions are safe
func NewLineEdge(p0, p1 CurvePoint, shift int) *LineEdge {
	// Convert to FDot6 with AA scaling.
	// Scale = 1 << (shift + 6), e.g., 64 for no AA, 256 for 4x AA.
	scale := float32(int32(1) << uint(shift+FDot6Shift))
	x0 := int32(p0.X * scale)
	y0 := int32(p0.Y * scale)
	x1 := int32(p1.X * scale)
	y1 := int32(p1.Y * scale)

	winding := int8(1)
	if y0 > y1 {
		// Swap to ensure y0 <= y1 (edge goes downward)
		x0, x1 = x1, x0
		y0, y1 = y1, y0
		winding = -1
	}

	top := FDot6Round(y0)
	bottom := FDot6Round(y1)

	// Skip zero-height lines (horizontal)
	if top == bottom {
		return nil
	}

	slope := FDot6Div(x1-x0, y1-y0)
	dy := computeDY(top, y0)

	return &LineEdge{
		Prev:    -1, // No link
		Next:    -1, // No link
		X:       FDot6ToFDot16(x0 + FDot16Mul(slope, dy)),
		DX:      slope,
		FirstY:  top,
		LastY:   bottom - 1,
		Winding: winding,
	}
}

// IsVertical returns true if the edge has zero slope.
func (e *LineEdge) IsVertical() bool {
	return e.DX == 0
}

// update updates the line edge for a new line segment.
// Called by QuadraticEdge and CubicEdge during stepping.
// Returns true if a valid segment was produced.
func (e *LineEdge) update(x0, y0, x1, y1 FDot16) bool {
	// Convert from FDot16 to FDot6 (shift right by 10)
	y0 >>= (FDot16Shift - FDot6Shift)
	y1 >>= (FDot16Shift - FDot6Shift)

	top := FDot6Round(y0)
	bottom := FDot6Round(y1)

	// Zero-height line?
	if top == bottom {
		return false
	}

	x0 >>= (FDot16Shift - FDot6Shift)
	x1 >>= (FDot16Shift - FDot6Shift)

	slope := FDot6Div(x1-x0, y1-y0)
	dy := computeDY(top, y0)

	e.X = FDot6ToFDot16(x0 + FDot16Mul(slope, dy))
	e.DX = slope
	e.FirstY = top
	e.LastY = bottom - 1

	return true
}

// QuadraticEdge represents a quadratic Bezier curve for scanline conversion.
// Uses forward differencing for O(1) per-step evaluation.
//
// A quadratic Bezier is defined by:
//
//	p(t) = (1-t)^2 * p0 + 2*t*(1-t) * p1 + t^2 * p2
//
// Rewritten in polynomial form:
//
//	p(t) = A*t^2 + B*t + C
//	where A = p0 - 2*p1 + p2, B = 2*(p1 - p0), C = p0
type QuadraticEdge struct {
	// TopY is the curve's overall top scanline (for AET insertion timing).
	// This is set once at creation and never changes, unlike line.FirstY
	// which changes as we step through curve segments.
	TopY int32

	// BottomY is the curve's overall bottom scanline.
	BottomY int32

	// line is the current line segment for AET compatibility.
	line LineEdge

	// curveCount is the remaining number of segments to generate.
	// Decrements from (1 << curveShift) down to 0.
	curveCount int8

	// curveShift determines the subdivision count (1 << curveShift segments).
	// Applied to all dx/ddx/ddy calculations.
	curveShift uint8

	// Forward difference coefficients in FDot16.
	// qx, qy: current position
	// qdx, qdy: first derivative (changes each step)
	// qddx, qddy: second derivative (constant for quadratic)
	qx, qy     FDot16
	qdx, qdy   FDot16
	qddx, qddy FDot16

	// Exact endpoint for the final segment.
	// Using exact endpoint avoids cumulative rounding errors.
	qLastX, qLastY FDot16
}

// NewQuadraticEdge creates a quadratic edge from control points.
// Returns nil if the curve has no vertical extent.
//
// Parameters:
//   - p0: start point
//   - p1: control point
//   - p2: end point
//   - shift: AA shift (0 for no AA, 2 for 4x AA quality)
func NewQuadraticEdge(p0, p1, p2 CurvePoint, shift int) *QuadraticEdge {
	// newQuadraticEdgeSetup already calls Update() to initialize the first segment
	return newQuadraticEdgeSetup(p0, p1, p2, shift)
}

// newQuadraticEdgeSetup performs the setup for a quadratic edge.
// Separated from NewQuadraticEdge to match tiny-skia's new/new2 pattern.
//
//nolint:gosec // G115: shift values bounded by MaxCoeffShift (6), all conversions safe
func newQuadraticEdgeSetup(p0, p1, p2 CurvePoint, shift int) *QuadraticEdge {
	// Convert to FDot6 with AA scaling.
	scale := float32(int32(1) << uint(shift+FDot6Shift))
	x0 := int32(p0.X * scale)
	y0 := int32(p0.Y * scale)
	x1 := int32(p1.X * scale)
	y1 := int32(p1.Y * scale)
	x2 := int32(p2.X * scale)
	y2 := int32(p2.Y * scale)

	winding := int8(1)
	if y0 > y2 {
		// Swap to ensure y0 <= y2 (monotonic in Y)
		x0, x2 = x2, x0
		y0, y2 = y2, y0
		winding = -1
	}

	top := FDot6Round(y0)
	bottom := FDot6Round(y2)

	// Zero-height curve?
	if top == bottom {
		return nil
	}

	// Compute number of subdivisions needed (1 << shift).
	// Based on the "flatness" of the curve (deviation from chord).
	//
	// For quadratic: dx = 2*p1 - p0 - p2, dy similar
	// This measures the maximum distance from the control point to the chord.
	dx := (leftShift(x1, 1) - x0 - x2) >> 2
	dy := (leftShift(y1, 1) - y0 - y2) >> 2

	// Reuse shift variable for curve subdivision count
	curveShift := diffToShift(dx, dy, shift)
	if curveShift < 0 {
		curveShift = 0
	}

	// Need at least 1 subdivision for our bias trick
	if curveShift == 0 {
		curveShift = 1
	} else if curveShift > MaxCoeffShift {
		curveShift = MaxCoeffShift
	}

	curveCount := int8(1 << uint(curveShift))

	// Compute polynomial coefficients.
	// p(t) = A*t^2 + B*t + C where:
	//   A = p0 - 2*p1 + p2
	//   B = 2*(p1 - p0)
	//   C = p0
	//
	// To guard against overflow, we store A and B at 1/2 their actual value,
	// then apply 2x scale during Update(). Hence we store (shift - 1) in curveShift.
	coeffShift := curveShift - 1

	// A/2 in FDot16 (divided by 2 to avoid overflow)
	a := FDot6ToFixedDiv2(x0 - x1 - x1 + x2)
	// B/2 in FDot16
	b := FDot6ToFDot16(x1 - x0)

	qx := FDot6ToFDot16(x0)
	qdx := b + (a >> uint(curveShift)) // biased by shift
	var qddx FDot16
	if coeffShift >= 1 {
		qddx = a >> uint(coeffShift-1)
	} else {
		qddx = a << 1 // coeffShift == 0, multiply by 2
	}

	// Repeat for Y
	a = FDot6ToFixedDiv2(y0 - y1 - y1 + y2)
	b = FDot6ToFDot16(y1 - y0)

	qy := FDot6ToFDot16(y0)
	qdy := b + (a >> uint(curveShift))
	var qddy FDot16
	if coeffShift >= 1 {
		qddy = a >> uint(coeffShift-1)
	} else {
		qddy = a << 1
	}

	qLastX := FDot6ToFDot16(x2)
	qLastY := FDot6ToFDot16(y2)

	storedShift := coeffShift
	if storedShift < 0 {
		storedShift = 0
	}

	edge := &QuadraticEdge{
		TopY:    top,    // Curve's overall top Y (for AET insertion)
		BottomY: bottom, // Curve's overall bottom Y
		line: LineEdge{
			Prev:    -1,
			Next:    -1,
			X:       0,
			DX:      0,
			FirstY:  top, // Will be updated by Update()
			LastY:   bottom - 1,
			Winding: winding,
		},
		curveCount: curveCount,
		curveShift: uint8(storedShift),
		qx:         qx,
		qy:         qy,
		qdx:        qdx,
		qdy:        qdy,
		qddx:       qddx,
		qddy:       qddy,
		qLastX:     qLastX,
		qLastY:     qLastY,
	}

	// Initialize the first line segment by calling Update()
	// This sets up X, DX, FirstY, LastY for the first curve segment
	if !edge.Update() {
		return nil // Degenerate curve
	}

	return edge
}

// Update advances the quadratic curve to the next line segment.
// Returns true if a valid segment was produced.
//
// This is the core of the forward differencing algorithm:
//
//	newx = oldx + (dx >> shift)
//	dx += ddx  // Second derivative is constant!
func (q *QuadraticEdge) Update() bool {
	count := q.curveCount
	if count <= 0 {
		return false
	}

	oldx := q.qx
	oldy := q.qy
	dx := q.qdx
	dy := q.qdy
	shift := q.curveShift

	var newx, newy FDot16
	var success bool

	for {
		count--
		if count > 0 {
			// Forward difference step: O(1)!
			newx = oldx + (dx >> shift)
			dx += q.qddx
			newy = oldy + (dy >> shift)
			dy += q.qddy
		} else {
			// Last segment: use exact endpoint to avoid accumulation errors
			newx = q.qLastX
			newy = q.qLastY
		}

		success = q.line.update(oldx, oldy, newx, newy)
		oldx = newx
		oldy = newy

		if count == 0 || success {
			break
		}
	}

	// Save state for next Update() call
	q.qx = newx
	q.qy = newy
	q.qdx = dx
	q.qdy = dy
	q.curveCount = count

	return success
}

// Line returns the current line segment for AET processing.
func (q *QuadraticEdge) Line() *LineEdge {
	return &q.line
}

// CurveCount returns the remaining number of segments.
func (q *QuadraticEdge) CurveCount() int8 {
	return q.curveCount
}

// Winding returns the winding direction.
func (q *QuadraticEdge) Winding() int8 {
	return q.line.Winding
}

// CubicEdge represents a cubic Bezier curve for scanline conversion.
// Uses forward differencing for O(1) per-step evaluation.
//
// A cubic Bezier is defined by:
//
//	p(t) = (1-t)^3 * p0 + 3*t*(1-t)^2 * p1 + 3*t^2*(1-t) * p2 + t^3 * p3
//
// Rewritten in polynomial form:
//
//	p(t) = A*t^3 + B*t^2 + C*t + D
//	where:
//	A = -p0 + 3*p1 - 3*p2 + p3
//	B = 3*p0 - 6*p1 + 3*p2
//	C = -3*p0 + 3*p1
//	D = p0
type CubicEdge struct {
	// TopY is the curve's overall top scanline (for AET insertion timing).
	// This is set once at creation and never changes, unlike line.FirstY
	// which changes as we step through curve segments.
	TopY int32

	// BottomY is the curve's overall bottom scanline.
	BottomY int32

	// line is the current line segment for AET compatibility.
	line LineEdge

	// curveCount is the remaining number of segments.
	// For cubics, counts DOWN from 0 to -(1 << curveShift).
	// Using negative counting matches tiny-skia behavior.
	curveCount int8

	// curveShift determines subdivision count.
	// Applied to ddx/ddy calculations.
	curveShift uint8

	// dshift is an additional shift applied to dx/dy.
	// Needed because cubic has one more derivative level.
	dshift uint8

	// Forward difference coefficients in FDot16.
	// cx, cy: current position
	// cdx, cdy: first derivative
	// cddx, cddy: second derivative
	// cdddx, cdddy: third derivative (constant for cubic!)
	cx, cy       FDot16
	cdx, cdy     FDot16
	cddx, cddy   FDot16
	cdddx, cdddy FDot16

	// Exact endpoint for the final segment.
	cLastX, cLastY FDot16
}

// NewCubicEdge creates a cubic edge from control points.
// Returns nil if the curve has no vertical extent.
//
// Parameters:
//   - p0: start point
//   - p1: first control point
//   - p2: second control point
//   - p3: end point
//   - shift: AA shift (0 for no AA, 2 for 4x AA quality)
func NewCubicEdge(p0, p1, p2, p3 CurvePoint, shift int) *CubicEdge {
	cubic := newCubicEdgeSetup(p0, p1, p2, p3, shift, true)
	if cubic == nil {
		return nil
	}
	if cubic.Update() {
		return cubic
	}
	return nil
}

// newCubicEdgeSetup performs the setup for a cubic edge.
//
//nolint:gosec // G115: shift values bounded by MaxCoeffShift (6), all conversions safe
func newCubicEdgeSetup(p0, p1, p2, p3 CurvePoint, shift int, sortY bool) *CubicEdge {
	// Convert to FDot6 with AA scaling.
	scale := float32(int32(1) << uint(shift+FDot6Shift))
	x0 := int32(p0.X * scale)
	y0 := int32(p0.Y * scale)
	x1 := int32(p1.X * scale)
	y1 := int32(p1.Y * scale)
	x2 := int32(p2.X * scale)
	y2 := int32(p2.Y * scale)
	x3 := int32(p3.X * scale)
	y3 := int32(p3.Y * scale)

	winding := int8(1)
	if sortY && y0 > y3 {
		// Swap to ensure y0 <= y3 (monotonic in Y)
		x0, x3 = x3, x0
		x1, x2 = x2, x1
		y0, y3 = y3, y0
		y1, y2 = y2, y1
		winding = -1
	}

	top := FDot6Round(y0)
	bot := FDot6Round(y3)

	// Zero-height curve?
	if sortY && top == bot {
		return nil
	}

	// Compute number of subdivisions needed.
	// For cubic, we can't use center-of-curve vs center-of-baseline
	// because the max deviation might not be at the center.
	// Instead, check both off-curve control points.
	dx := cubicDeltaFromLine(x0, x1, x2, x3)
	dy := cubicDeltaFromLine(y0, y1, y2, y3)

	// Add 1 to shift (by observation from Skia)
	curveShift := diffToShift(dx, dy, 2) + 1
	if curveShift < 1 {
		curveShift = 1
	}
	if curveShift > MaxCoeffShift {
		curveShift = MaxCoeffShift
	}

	// Compute up/down shifts for coefficient scaling.
	// Our input data is shifted by (shift + 6) = 8 to 10 bits.
	// We compute coefficients with 3x multiplier, so max safe upshift is 6.
	upShift := 6
	downShift := curveShift + upShift - 10
	if downShift < 0 {
		downShift = 0
		upShift = 10 - curveShift
	}

	// Curve count is NEGATIVE for cubic (counts up to 0).
	// This matches tiny-skia behavior.
	curveCount := int8(leftShift(-1, curveShift))
	dshift := uint8(downShift)

	// Compute forward differencing coefficients.
	// For cubic p(t) = At^3 + Bt^2 + Ct + D:
	//   C = 3*(p1 - p0)
	//   B = 3*(p0 - 2*p1 + p2) = 3*(p2 - 2*p1 + p0)
	//   A = p3 - 3*p2 + 3*p1 - p0 = p3 + 3*(p1 - p2) - p0
	b := FDot6UpShift(3*(x1-x0), upShift)
	c := FDot6UpShift(3*(x0-x1-x1+x2), upShift)
	d := FDot6UpShift(x3+3*(x1-x2)-x0, upShift)

	cx := FDot6ToFDot16(x0)
	cdx := b + (c >> uint(curveShift)) + (d >> uint(2*curveShift))
	cddx := 2*c + ((3 * d) >> uint(curveShift-1))
	cdddx := (3 * d) >> uint(curveShift-1)

	// Repeat for Y
	b = FDot6UpShift(3*(y1-y0), upShift)
	c = FDot6UpShift(3*(y0-y1-y1+y2), upShift)
	d = FDot6UpShift(y3+3*(y1-y2)-y0, upShift)

	cy := FDot6ToFDot16(y0)
	cdy := b + (c >> uint(curveShift)) + (d >> uint(2*curveShift))
	cddy := 2*c + ((3 * d) >> uint(curveShift-1))
	cdddy := (3 * d) >> uint(curveShift-1)

	cLastX := FDot6ToFDot16(x3)
	cLastY := FDot6ToFDot16(y3)

	return &CubicEdge{
		TopY:    top, // Curve's overall top Y (for AET insertion)
		BottomY: bot, // Curve's overall bottom Y
		line: LineEdge{
			Prev:    -1,
			Next:    -1,
			X:       0,
			DX:      0,
			FirstY:  top, // Will be updated by Update()
			LastY:   bot - 1,
			Winding: winding,
		},
		curveCount: curveCount,
		curveShift: uint8(curveShift),
		dshift:     dshift,
		cx:         cx,
		cy:         cy,
		cdx:        cdx,
		cdy:        cdy,
		cddx:       cddx,
		cddy:       cddy,
		cdddx:      cdddx,
		cdddy:      cdddy,
		cLastX:     cLastX,
		cLastY:     cLastY,
	}
}

// Update advances the cubic curve to the next line segment.
// Returns true if a valid segment was produced.
//
// Forward differencing for cubic:
//
//	newx = oldx + (dx >> dshift)
//	dx += ddx >> ddshift
//	ddx += dddx  // Third derivative is constant!
func (c *CubicEdge) Update() bool {
	count := c.curveCount
	// Cubic uses negative count, increments toward 0
	if count >= 0 {
		return false
	}

	oldx := c.cx
	oldy := c.cy
	ddshift := c.curveShift
	dshift := c.dshift

	var newx, newy FDot16
	var success bool

	for {
		count++
		if count < 0 {
			// Forward difference step: O(1)!
			newx = oldx + (c.cdx >> dshift)
			c.cdx += c.cddx >> ddshift
			c.cddx += c.cdddx

			newy = oldy + (c.cdy >> dshift)
			c.cdy += c.cddy >> ddshift
			c.cddy += c.cdddy
		} else {
			// Last segment: use exact endpoint
			newx = c.cLastX
			newy = c.cLastY
		}

		// Pin newy to prevent going backwards (numerical precision issue)
		if newy < oldy {
			newy = oldy
		}

		success = c.line.update(oldx, oldy, newx, newy)
		oldx = newx
		oldy = newy

		if count == 0 || success {
			break
		}
	}

	// Save state
	c.cx = newx
	c.cy = newy
	c.curveCount = count

	return success
}

// Line returns the current line segment for AET processing.
func (c *CubicEdge) Line() *LineEdge {
	return &c.line
}

// CurveCount returns the remaining number of segments.
// For cubic, this is negative (counts up to 0).
func (c *CubicEdge) CurveCount() int8 {
	return c.curveCount
}

// Winding returns the winding direction.
func (c *CubicEdge) Winding() int8 {
	return c.line.Winding
}

// Helper functions for curve edge computation.

// computeDY calculates the fractional Y offset for the first scanline.
// This correctly favors the lower-pixel when y0 is on a 1/2 pixel boundary.
func computeDY(top int32, y0 FDot6) FDot6 {
	// (top << 6) + 32 - y0
	// This gives us the distance from y0 to the center of the first scanline.
	return leftShift(top, FDot6Shift) + FDot6Half - y0
}

// diffToShift determines the number of subdivision bits needed for a curve
// based on the maximum distance from control points to the chord.
//
// Each subdivision (shift value) cuts the error by 1/4.
// The shift is chosen to achieve sub-pixel accuracy.
//
// Parameters:
//   - dx, dy: deviation in FDot6
//   - shiftAA: anti-aliasing shift (0 or 2 typically)
//
// Returns: shift value (0 to MaxCoeffShift)
//
//nolint:gosec // G115: shiftAA bounded [0, 2], dist bounded by coordinate range
func diffToShift(dx, dy FDot6, shiftAA int) int {
	// Cheap approximation of distance: max + min/2
	dist := cheapDistance(dx, dy)

	// Shift down dist (currently in FDot6).
	// Down by 3 gives ~1/8 pixel accuracy (heuristic from Skia).
	// When shiftAA > 0, we're using AA and everything is scaled up,
	// so we can lower the accuracy requirement.
	dist = (dist + (1 << uint(2+shiftAA))) >> uint(3+shiftAA)

	// Each subdivision (shift value) cuts error by 1/4.
	// Find how many times we need to divide by 4 to get dist < 1.
	// This is equivalent to (log2(dist) + 1) / 2 = (32 - leading_zeros) / 2.
	if dist <= 0 {
		return 0
	}
	return (32 - bits.LeadingZeros32(uint32(dist))) >> 1
}

// cheapDistance approximates the distance sqrt(dx*dx + dy*dy)
// using the cheaper formula: max(|dx|, |dy|) + min(|dx|, |dy|) / 2.
// This is accurate to within ~12%.
func cheapDistance(dx, dy FDot6) FDot6 {
	dx = absInt32(dx)
	dy = absInt32(dy)

	if dx > dy {
		return dx + (dy >> 1)
	}
	return dy + (dx >> 1)
}

// cubicDeltaFromLine computes the maximum deviation of a cubic's
// control points from the baseline (p0 to p3).
//
// Evaluates the curve at t=1/3 and t=2/3 and finds the max
// distance from the corresponding baseline points.
//
// Uses 16/512 to approximate 1/27 (each cubic evaluated at these points
// involves denominators of 27).
func cubicDeltaFromLine(a, b, c, d FDot6) FDot6 {
	// f(1/3) = (8a + 12b + 6c + d) / 27
	// f(2/3) = (a + 6b + 12c + 8d) / 27
	//
	// Deviation from line:
	// f(1/3) - (2a + d)/3 ≈ (8a - 15b + 6c + d) / 27
	// f(2/3) - (a + 2d)/3 ≈ (a + 6b - 15c + 8d) / 27
	//
	// Use 19/512 ≈ 1/27 for approximation.
	oneThird := ((a*8 - b*15 + 6*c + d) * 19) >> 9
	twoThird := ((a + 6*b - c*15 + d*8) * 19) >> 9

	return maxInt32(absInt32(oneThird), absInt32(twoThird))
}

// EdgeType represents the type of an edge for polymorphic handling.
type EdgeType int

const (
	// EdgeTypeLine represents a simple line edge.
	EdgeTypeLine EdgeType = iota

	// EdgeTypeQuadratic represents a quadratic Bezier edge.
	EdgeTypeQuadratic

	// EdgeTypeCubic represents a cubic Bezier edge.
	EdgeTypeCubic
)

// CurveEdgeVariant wraps different edge types for uniform handling in the AET.
// This is Go's equivalent of Rust's enum Edge { Line, Quadratic, Cubic }.
type CurveEdgeVariant struct {
	Type      EdgeType
	Line      *LineEdge
	Quadratic *QuadraticEdge
	Cubic     *CubicEdge
}

// AsLine returns the LineEdge for this edge, regardless of type.
// All edge types contain a LineEdge for AET compatibility.
func (e *CurveEdgeVariant) AsLine() *LineEdge {
	switch e.Type {
	case EdgeTypeLine:
		return e.Line
	case EdgeTypeQuadratic:
		return &e.Quadratic.line
	case EdgeTypeCubic:
		return &e.Cubic.line
	default:
		return nil
	}
}

// TopY returns the curve's overall top Y coordinate (for AET insertion timing).
// For line edges, this is the same as FirstY.
// For curve edges, this is the curve's original top Y before stepping.
func (e *CurveEdgeVariant) TopY() int32 {
	switch e.Type {
	case EdgeTypeLine:
		return e.Line.FirstY
	case EdgeTypeQuadratic:
		return e.Quadratic.TopY
	case EdgeTypeCubic:
		return e.Cubic.TopY
	default:
		return 0
	}
}

// BottomY returns the curve's overall bottom Y coordinate.
// For line edges, this is the same as LastY + 1.
// For curve edges, this is the curve's original bottom Y.
func (e *CurveEdgeVariant) BottomY() int32 {
	switch e.Type {
	case EdgeTypeLine:
		return e.Line.LastY + 1
	case EdgeTypeQuadratic:
		return e.Quadratic.BottomY
	case EdgeTypeCubic:
		return e.Cubic.BottomY
	default:
		return 0
	}
}

// Update advances a curve edge to the next line segment.
// Returns true if a valid segment was produced.
// For line edges, always returns false (no more segments).
func (e *CurveEdgeVariant) Update() bool {
	switch e.Type {
	case EdgeTypeQuadratic:
		return e.Quadratic.Update()
	case EdgeTypeCubic:
		return e.Cubic.Update()
	default:
		return false
	}
}

// NewLineEdgeVariant creates a CurveEdgeVariant for a line.
func NewLineEdgeVariant(p0, p1 CurvePoint, shift int) *CurveEdgeVariant {
	line := NewLineEdge(p0, p1, shift)
	if line == nil {
		return nil
	}
	return &CurveEdgeVariant{
		Type: EdgeTypeLine,
		Line: line,
	}
}

// NewQuadraticEdgeVariant creates a CurveEdgeVariant for a quadratic.
func NewQuadraticEdgeVariant(p0, p1, p2 CurvePoint, shift int) *CurveEdgeVariant {
	quad := NewQuadraticEdge(p0, p1, p2, shift)
	if quad == nil {
		return nil
	}
	return &CurveEdgeVariant{
		Type:      EdgeTypeQuadratic,
		Quadratic: quad,
	}
}

// NewCubicEdgeVariant creates a CurveEdgeVariant for a cubic.
func NewCubicEdgeVariant(p0, p1, p2, p3 CurvePoint, shift int) *CurveEdgeVariant {
	cubic := NewCubicEdge(p0, p1, p2, p3, shift)
	if cubic == nil {
		return nil
	}
	return &CurveEdgeVariant{
		Type:  EdgeTypeCubic,
		Cubic: cubic,
	}
}
