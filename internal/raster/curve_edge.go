// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

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

	// FirstY is the first scanline this edge covers (integer sub-pixel row, for AET).
	FirstY int32

	// LastY is the last scanline this edge covers (inclusive, integer sub-pixel row, for AET).
	LastY int32

	// UpperY is the precise upper Y endpoint in FDot16 (16.16 fixed-point).
	// Used by AnalyticFiller for sub-strip boundary computation (Skia AAA precision).
	// When zero, falls back to FirstY-based computation.
	UpperY FDot16

	// LowerY is the precise lower Y endpoint in FDot16 (16.16 fixed-point).
	// Used by AnalyticFiller for sub-strip boundary computation (Skia AAA precision).
	// When zero, falls back to LastY-based computation.
	LowerY FDot16

	// UpperX is the X position at UpperY in pixel-space SkFixed (16.16).
	// Computed using Skia's exact setLine() conversion chain to avoid rounding
	// errors from sub-pixel-to-pixel division. Only set for line edges created
	// by NewLineEdge (zero for curve sub-segments).
	// Used by computeEdgeX via Skia's goY(): X(Y) = UpperX + PixelDX*(Y-UpperY).
	UpperX int32

	// PixelDX is the slope in pixel-space SkFixed (16.16), matching Skia's fDX.
	PixelDX int32

	// PixelDY is Skia's fDY = abs(1/slope) for partialTriangleToAlpha.
	// Computed as abs(FDot6Div(dy_fdot6, dx_fdot6)) from ORIGINAL pixel-space
	// edge coordinates, matching SkAnalyticEdge::setLine line 197-199.
	// NOT derived from PixelDX (1/slope ≠ FDot6Div(dy,dx) due to integer rounding).
	PixelDY int32

	// Winding indicates direction: +1 for downward, -1 for upward.
	Winding int8
}

// NewLineEdge creates a new line edge from two points.
// Returns (edge, true) on success, or (zero, false) if the edge is horizontal
// (no Y extent). Returning by value avoids a heap allocation per edge.
//
// Parameters:
//   - p0, p1: endpoints in pixel coordinates
//   - shift: AA shift (0 for no AA, 2 for 4x AA quality)
//
//nolint:gosec // G115: shift is bounded [0, MaxCoeffShift], conversions are safe
func NewLineEdge(p0, p1 CurvePoint, shift int) (LineEdge, bool) {
	// Convert to FDot6 with AA scaling (truncation, matching Skia's SkScalarToFDot6).
	// Scale = 1 << (shift + 6), e.g., 64 for no AA, 256 for 4x AA.
	scale := float32(int32(1) << uint(shift+FDot6Shift))
	x0 := int32(p0.X * scale)
	y0 := int32(p0.Y * scale)
	x1 := int32(p1.X * scale)
	y1 := int32(p1.Y * scale)

	// --- Skia AAA pixel-space fields (SkAnalyticEdge::setLine exact port) ---
	//
	// Skia's setLine() ALWAYS uses kDefaultAccuracy=2 (multiplier=4) for
	// pixel-space coordinates, regardless of the AA shift used for sub-pixel
	// edge construction. This ensures consistent edge ordering with quads/cubics.
	//
	// Conversion chain:
	//   x = SkFDot6ToFixed(SkScalarToFDot6(p.fX * 4)) >> 2
	//     = (int(p.fX * 4 * 64) << 10) >> 2  =  int(p.fX * 256) << 8
	//   y = SnapY(same formula for Y)
	//
	// All values are in SkFixed (16.16 pixel-space).
	const skiaAccuracy = 2                          // kDefaultAccuracy
	const skiaMultiplier = int32(1) << skiaAccuracy // 4
	// SkScalarToFDot6(p.X * multiplier) = int(p.X * multiplier * 64) = int(p.X * 256)
	skX0 := int32(p0.X * float32(skiaMultiplier) * 64.0)
	skY0 := int32(p0.Y * float32(skiaMultiplier) * 64.0)
	skX1 := int32(p1.X * float32(skiaMultiplier) * 64.0)
	skY1 := int32(p1.Y * float32(skiaMultiplier) * 64.0)
	// SkFDot6ToFixed(v) >> accuracy = (v << 10) >> 2 = v << 8
	pxX0 := leftShift(skX0, 10-skiaAccuracy)
	pxY0 := snapY(leftShift(skY0, 10-skiaAccuracy))
	pxX1 := leftShift(skX1, 10-skiaAccuracy)
	pxY1 := snapY(leftShift(skY1, 10-skiaAccuracy))

	winding := int8(1)
	if y0 > y1 {
		// Swap to ensure y0 <= y1 (edge goes downward)
		x0, x1 = x1, x0
		y0, y1 = y1, y0
		pxX0, pxX1 = pxX1, pxX0
		pxY0, pxY1 = pxY1, pxY0
		winding = -1
	}

	top := FDot6Round(y0)
	bottom := FDot6Round(y1)

	// Skip zero-height lines (horizontal)
	if top == bottom {
		return LineEdge{}, false
	}

	slope := FDot6Div(x1-x0, y1-y0)
	dy := computeDY(top, y0)

	// Skia pixel-space slope: SkFDot6Div(SkFixedToFDot6(pxX1-pxX0), SkFixedToFDot6(pxY1-pxY0))
	// SkFixedToFDot6(v) = v >> 10
	pxDx := (pxX1 - pxX0) >> 10
	pxDy := (pxY1 - pxY0) >> 10
	var pixelDX int32
	if pxDy == 0 {
		// Horizontal line in pixel space — should not happen (top != bottom),
		// but guard anyway.
		pixelDX = 0
	} else {
		pixelDX = FDot6Div(pxDx, pxDy)
	}

	// Skia's fDY = abs(1/slope) for partialTriangleToAlpha.
	// Computed as abs(FDot6Div(dy, dx)) from pixel-space FDot6 (NOT 1/slope).
	// Matches SkAnalyticEdge.cpp:197-199.
	var pixelDY int32
	if pxDx == 0 || pixelDX == 0 {
		pixelDY = 0x7FFFFFFF
	} else {
		absDx := pxDx
		if absDx < 0 {
			absDx = -absDx
		}
		absDy := pxDy
		if absDy < 0 {
			absDy = -absDy
		}
		pixelDY = FDot6Div(absDy, absDx)
		if pixelDY < 0 {
			pixelDY = 0x7FFFFFFF
		}
	}

	return LineEdge{
		Prev:    -1,
		Next:    -1,
		X:       FDot6ToFDot16(x0 + FDot16Mul(slope, dy)),
		DX:      slope,
		FirstY:  top,
		LastY:   bottom - 1,
		UpperY:  pxY0,
		LowerY:  pxY1,
		UpperX:  pxX0,
		PixelDX: pixelDX,
		PixelDY: pixelDY,
		Winding: winding,
	}, true
}

// snapY applies Skia's SnapY rounding (SkAnalyticEdge.h:52) with accuracy=2.
// Rounds FDot16 Y to nearest 1/4 pixel boundary.
func snapY(y FDot16) FDot16 {
	const accuracy = 2
	const half = int32(1) << (16 - accuracy - 1)
	const mask = ^(int32(1)<<(16-accuracy) - 1)
	return (y + half) & mask
}

// skFixedRoundToFixed rounds an FDot16 to the nearest integer, keeping it in FDot16.
// Equivalent to Skia's SkFixedRoundToFixed: (v + 0x8000) & ~0xFFFF.
func skFixedRoundToFixed(v FDot16) FDot16 {
	return (v + 0x8000) & ^FDot16(0xFFFF)
}

// minFixed returns the smaller of two FDot16 values.
func minFixed(a, b FDot16) FDot16 {
	if a < b {
		return a
	}
	return b
}

// IsVertical returns true if the edge has zero slope.
func (e *LineEdge) IsVertical() bool {
	return e.DX == 0
}

// update updates the line edge for a new line segment.
// Called by CubicEdge during stepping (legacy path without snapping).
// Returns true if a valid segment was produced.
//
// Note: UpperY/LowerY are NOT set here because the y0/y1 values from curve
// forward differencing are in the FDot6-scaled coordinate system (not pixel-
// space FDot16). Only NewLineEdge and updateLine set precise pixel-space fields.
// Curve segments are already subdivided finely, so FDot6-rounded Y is adequate.
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
	// Clear precise Y — curve segments use FDot6 system, not pixel-space FDot16.
	e.UpperY = 0
	e.LowerY = 0

	return true
}

// updateLine updates the line edge for a curve sub-segment with pre-computed slope.
// Port of Skia's SkAnalyticEdge::updateLine (SkAnalyticEdge.cpp:215-257).
//
// All parameters are in pixel-space FDot16 (SkFixed). The slope is pre-computed
// by QuadraticEdge.Update() from snapped coordinates to avoid redundant division.
// This method sets the precise pixel-space fields (UpperY, LowerY, UpperX,
// PixelDX, PixelDY) so the AnalyticFiller uses the high-precision path.
//
// Unlike update(), this does NOT swap y0>y1 because QuadraticEdge.Update()
// already ensures monotonic Y via snapping. For cubics (which don't chop at
// Y extrema), the caller must handle winding reversal — but cubics don't
// use this method.
func (e *LineEdge) updateLine(x0, y0, x1, y1, slope FDot16) bool {
	// Handle non-monotonic Y (cubics may produce y0 > y1).
	// QuadraticEdge guarantees monotonic after snapping, but be defensive.
	if y0 > y1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
		e.Winding = -e.Winding
	}

	// Check for zero-height segment in FDot6 precision.
	// Skia: SkFixedToFDot6(y1 - y0) == 0 means < 1/64 pixel height.
	// This is much less aggressive than the old update() which compared
	// FDot6Round(y0) == FDot6Round(y1), discarding segments < 1 whole pixel.
	dy := (y1 - y0) >> (FDot16Shift - FDot6Shift) // SkFixedToFDot6
	if dy == 0 {
		return false
	}

	dx := (x1 - x0) >> (FDot16Shift - FDot6Shift) // SkFixedToFDot6

	// Set pixel-space fields (Skia's fX, fDX, fUpperX, fUpperY, fLowerY, fDY).
	e.X = x0
	e.DX = slope
	e.UpperX = x0
	e.UpperY = y0
	e.LowerY = y1

	// Compute fDY = abs(1/slope) for partialTriangleToAlpha.
	// Matches Skia's updateLine (SkAnalyticEdge.cpp:250-254).
	absSlopeFDot6 := absInt32(slope >> (FDot16Shift - FDot6Shift))
	switch {
	case dx == 0 || slope == 0:
		e.PixelDY = 0x7FFFFFFF
	case absSlopeFDot6 > 0:
		// Use FDot6Div(dy, dx) for abs(1/slope), matching Skia's quick_div path.
		absDx := absInt32(dx)
		absDy := absInt32(dy)
		e.PixelDY = FDot6Div(absDy, absDx)
		if e.PixelDY < 0 {
			e.PixelDY = 0x7FFFFFFF
		}
	default:
		e.PixelDY = 0x7FFFFFFF
	}

	e.PixelDX = slope

	// Set sub-pixel fields for AET ordering.
	// FirstY/LastY are integer scanline indices derived from pixel-space Y.
	top := FDot16RoundToInt(y0)
	bottom := FDot16RoundToInt(y1)
	if top == bottom {
		// Even though FDot6 dy != 0, the pixel-rounded top/bottom may match.
		// This can happen for very short segments. Set FirstY=LastY so the
		// edge is valid in the AET but covers a single scanline.
		bottom = top + 1
	}
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

	// Forward difference coefficients in FDot16 (pixel-space after >>= kDefaultAccuracy).
	// qx, qy: current position
	// qdx, qdy: first derivative (changes each step)
	// qddx, qddy: second derivative (constant for quadratic)
	qx, qy     FDot16
	qdx, qdy   FDot16
	qddx, qddy FDot16

	// Exact endpoint for the final segment (pixel-space after >>= kDefaultAccuracy).
	// Using exact endpoint avoids cumulative rounding errors.
	qLastX, qLastY FDot16

	// snappedX, snappedY track the snapped coordinates between Update() calls.
	// Slope is computed from snapped→snapped coordinates, matching Skia's
	// SkAnalyticQuadraticEdge::fSnappedX/fSnappedY (SkAnalyticEdge.h:113).
	// This eliminates forward-diff accumulation errors from the slope computation.
	snappedX, snappedY FDot16
}

// NewQuadraticEdge creates a quadratic edge from control points.
// Returns (edge, true) on success, or (zero, false) if the curve has no
// vertical extent or is degenerate. Returns by value to avoid heap allocation.
//
// Parameters:
//   - p0: start point
//   - p1: control point
//   - p2: end point
//   - shift: AA shift (0 for no AA, 2 for 4x AA quality)
//
//nolint:gosec // G115: shift values bounded by MaxCoeffShift (6), all conversions safe
func NewQuadraticEdge(p0, p1, p2 CurvePoint, shift int) (QuadraticEdge, bool) {
	// --- Phase 1: setQuadraticWithoutUpdate(pts, shift=kDefaultAccuracy=2) ---
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
		return QuadraticEdge{}, false
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

	// --- Phase 2: setQuadratic() coefficient >>= kDefaultAccuracy (Skia lines 429-436) ---
	// Convert from AA-scaled space to pixel-space by dividing by 4 (1 << accuracy).
	// This is the key step that was missing (Bug 2): without it, forward-diff
	// steps are 4× too large, producing only ~2 segments instead of ~8.
	const accuracy = 2 // kDefaultAccuracy
	qx >>= accuracy
	qy >>= accuracy
	qdx >>= accuracy
	qdy >>= accuracy
	qddx >>= accuracy
	qddy >>= accuracy
	qLastX >>= accuracy
	qLastY >>= accuracy

	// --- Phase 3: SnapY endpoints (Skia lines 437-438) ---
	qy = snapY(qy)
	qLastY = snapY(qLastY)

	edge := QuadraticEdge{
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
		// Phase 4: Initialize snapped coordinates (Skia lines 442-443)
		snappedX: qx,
		snappedY: qy,
	}

	// Initialize the first line segment by calling Update()
	// This sets up X, DX, FirstY, LastY, UpperY, LowerY for the first curve segment.
	if !edge.Update() {
		return QuadraticEdge{}, false // Degenerate curve
	}

	return edge, true
}

// Update advances the quadratic curve to the next line segment.
// Returns true if a valid segment was produced.
//
// Port of Skia's SkAnalyticQuadraticEdge::updateQuadratic (SkAnalyticEdge.cpp:448-510).
//
// Key differences from the old implementation:
//   - Uses SnapY and snapped coordinates for slope computation (Bug 3 fix)
//   - Computes slope from snapped→snapped coordinates, not raw forward-diff output
//   - Calls updateLine() which sets pixel-space fields (UpperY, LowerY, PixelDX, etc.)
//   - Has separate path for "large dy" vs "small dy" snapping (Skia lines 467-480)
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
	var newSnappedX, newSnappedY FDot16
	var success bool

	for {
		var slope FDot16
		count--

		if count > 0 {
			// Forward difference step: O(1)!
			newx = oldx + (dx >> shift)
			newy = oldy + (dy >> shift)

			// Skia's two-branch snapping strategy (SkAnalyticEdge.cpp:467-480):
			// When dy is large enough AND dx/dy isn't too extreme, snap more
			// aggressively using SkFixedRoundToFixed. Otherwise use SnapY.
			absDyShifted := absInt32(dy >> shift)
			if absDyShifted >= FDot16One*2 &&
				(int64(absInt32(dy))<<6) > int64(absInt32(dx)) {
				// Large-dy path
				diffY := (newy - q.snappedY) >> (FDot16Shift - FDot6Shift)
				if diffY != 0 {
					diffX := (newx - q.snappedX) >> (FDot16Shift - FDot6Shift)
					slope = FDot6Div(diffX, diffY)
				} else {
					slope = 0x7FFFFFFF
				}
				newSnappedY = minFixed(q.qLastY, skFixedRoundToFixed(newy))
				newSnappedX = newx - skFixedMul(slope, newy-newSnappedY)
			} else {
				// Small-dy path
				newSnappedY = minFixed(q.qLastY, snapY(newy))
				newSnappedX = newx
				diffY := (newSnappedY - q.snappedY) >> (FDot16Shift - FDot6Shift)
				if diffY != 0 {
					diffX := (newx - q.snappedX) >> (FDot16Shift - FDot6Shift)
					slope = FDot6Div(diffX, diffY)
				} else {
					slope = 0x7FFFFFFF
				}
			}

			dx += q.qddx
			dy += q.qddy
		} else {
			// Last segment: use exact endpoint to avoid accumulation errors.
			newx = q.qLastX
			newy = q.qLastY
			newSnappedX = newx
			newSnappedY = newy
			diffY := (newy - q.snappedY) >> (FDot16Shift - FDot6Shift)
			if diffY != 0 {
				diffX := (newx - q.snappedX) >> (FDot16Shift - FDot6Shift)
				slope = FDot6Div(diffX, diffY)
			} else {
				slope = 0x7FFFFFFF
			}
		}

		if slope < 0x7FFFFFFF {
			success = q.line.updateLine(q.snappedX, q.snappedY, newSnappedX, newSnappedY, slope)
		}

		q.snappedX = newSnappedX
		q.snappedY = newSnappedY
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

	// Exact endpoint for the final segment (pixel-space after >>= kDefaultAccuracy).
	cLastX, cLastY FDot16

	// snappedY tracks the snapped Y coordinate between Update() calls.
	// Slope is computed from snapped Y coordinates, matching Skia's
	// SkAnalyticCubicEdge::fSnappedY (SkAnalyticEdge.h:134).
	// Cubics don't track snappedX (unlike quads) — slope uses raw oldx.
	snappedY FDot16
}

// NewCubicEdge creates a cubic edge from control points.
// Returns (edge, true) on success, or (zero, false) if the curve has no
// vertical extent or is degenerate. Returns by value to avoid heap allocation.
//
// Parameters:
//   - p0: start point
//   - p1: first control point
//   - p2: second control point
//   - p3: end point
//   - shift: AA shift (0 for no AA, 2 for 4x AA quality)
//
//nolint:gosec // G115: shift values bounded by MaxCoeffShift (6), all conversions safe
func NewCubicEdge(p0, p1, p2, p3 CurvePoint, shift int) (CubicEdge, bool) {
	edge, ok := newCubicEdgeSetup(p0, p1, p2, p3, shift, true)
	if !ok {
		return CubicEdge{}, false
	}
	if edge.Update() {
		return edge, true
	}
	return CubicEdge{}, false
}

// newCubicEdgeSetup performs the setup for a cubic edge.
// Returns (edge, true) on success, or (zero, false) if degenerate.
//
//nolint:gosec // G115: shift values bounded by MaxCoeffShift (6), all conversions safe
func newCubicEdgeSetup(p0, p1, p2, p3 CurvePoint, shift int, sortY bool) (CubicEdge, bool) {
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
		return CubicEdge{}, false
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

	// --- Phase 2: setCubic() coefficient >>= kDefaultAccuracy (Skia lines 521-528) ---
	// Convert from AA-scaled space to pixel-space by dividing by 4 (1 << accuracy).
	// Same pattern as QuadraticEdge: without this, forward-diff steps are 4x too large.
	const accuracy = 2 // kDefaultAccuracy
	cx >>= accuracy
	cy >>= accuracy
	cdx >>= accuracy
	cdy >>= accuracy
	cddx >>= accuracy
	cddy >>= accuracy
	cdddx >>= accuracy
	cdddy >>= accuracy
	cLastX >>= accuracy
	cLastY >>= accuracy

	// --- Phase 3: SnapY endpoints (Skia lines 529-532) ---
	cy = snapY(cy)
	cLastY = snapY(cLastY)

	edge := CubicEdge{
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
		// Phase 4: Initialize snapped coordinate (Skia line 531)
		snappedY: cy,
	}
	return edge, true
}

// Update advances the cubic curve to the next line segment.
// Returns true if a valid segment was produced.
//
// Port of Skia's SkAnalyticCubicEdge::updateCubic (SkAnalyticEdge.cpp:659-715).
//
// Key differences from the old implementation:
//   - Uses SnapY and snappedY for slope computation (same fix pattern as QuadraticEdge)
//   - Calls updateLine() which sets pixel-space fields (UpperY, LowerY, PixelDX, etc.)
//   - Monotonic pin: newy pinned to oldy to prevent going backwards
//   - Slope computed from (newx - oldx) and (newSnappedY - snappedY) in FDot6
//   - Cubics don't use snappedX (unlike quads) — slope uses raw oldx
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

		// Pin newy to prevent going backwards (monotonic, Skia line 688)
		if newy < oldy {
			newy = oldy
		}

		// Snap newy to 1/4 pixel grid (Skia line 690)
		newSnappedY := snapY(newy)

		// Pin snapped Y to endpoint (Skia lines 691-694):
		// If the snapped Y overshoots the final Y, clamp it and force last segment.
		if c.cLastY < newSnappedY {
			newSnappedY = c.cLastY
			count = 0
		}

		// Compute slope from raw X delta and snapped Y delta (Skia lines 696-699).
		// Cubics use oldx (not snappedX) for X — only Y is snapped.
		var slope FDot16
		dyFDot6 := (newSnappedY - c.snappedY) >> (FDot16Shift - FDot6Shift)
		if dyFDot6 == 0 {
			slope = 0x7FFFFFFF
		} else {
			dxFDot6 := (newx - oldx) >> (FDot16Shift - FDot6Shift)
			slope = FDot6Div(dxFDot6, dyFDot6)
		}

		// Call updateLine with snapped coordinates and pre-computed slope.
		// This sets pixel-space fields (UpperY, LowerY, PixelDX, PixelDY).
		if slope < 0x7FFFFFFF {
			success = c.line.updateLine(oldx, c.snappedY, newx, newSnappedY, slope)
		}

		c.snappedY = newSnappedY
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
	line, ok := NewLineEdge(p0, p1, shift)
	if !ok {
		return nil
	}
	return &CurveEdgeVariant{
		Type: EdgeTypeLine,
		Line: &line,
	}
}

// NewQuadraticEdgeVariant creates a CurveEdgeVariant for a quadratic.
func NewQuadraticEdgeVariant(p0, p1, p2 CurvePoint, shift int) *CurveEdgeVariant {
	quad, ok := NewQuadraticEdge(p0, p1, p2, shift)
	if !ok {
		return nil
	}
	return &CurveEdgeVariant{
		Type:      EdgeTypeQuadratic,
		Quadratic: &quad,
	}
}

// NewCubicEdgeVariant creates a CurveEdgeVariant for a cubic.
func NewCubicEdgeVariant(p0, p1, p2, p3 CurvePoint, shift int) *CurveEdgeVariant {
	cubic, ok := NewCubicEdge(p0, p1, p2, p3, shift)
	if !ok {
		return nil
	}
	return &CurveEdgeVariant{
		Type:  EdgeTypeCubic,
		Cubic: &cubic,
	}
}
