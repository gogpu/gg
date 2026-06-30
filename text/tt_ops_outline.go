// TrueType bytecode interpreter — outline manipulation instructions.
//
// Port of skrifa hint/engine/outline.rs (1418 LOC).
// THE CRITICAL FILE — the "big 10" point manipulation instructions:
// MDAP, MIAP, MDRP, MIRP, MSIRP, IUP, IP, ALIGNRP, ALIGNPTS, ISECT,
// SHP, SHC, SHZ, SHPIX, UTP, FLIPPT, FLIPRGON, FLIPRGOFF.
//
// Reference: skrifa/src/outline/glyf/hint/engine/outline.rs
package text

// ============================================================
// MDAP — Move Direct Absolute Point
// ============================================================

// opMdap implements MDAP[a] (0x2E-0x2F).
// Moves a point to its current position, optionally rounding.
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opMdap(opcode byte) error {
	pointIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	doRound := opcode&1 != 0
	z := e.zone(e.graphics.zp0)
	pt, err := z.point(pointIdx)
	if err != nil {
		if e.graphics.isPedantic {
			return err
		}
		e.graphics.rp0 = pointIdx
		e.graphics.rp1 = pointIdx
		return nil
	}
	curDist := e.graphics.project(pt[0], pt[1], 0, 0)
	var distance int32
	if doRound {
		distance = e.graphics.roundState.round(curDist) - curDist
	}
	if err := z.movePoint(&e.graphics, pointIdx, distance); err != nil {
		if e.graphics.isPedantic {
			return err
		}
	}
	// NOTE: movePoint now handles touching (skrifa pattern).
	e.graphics.rp0 = pointIdx
	e.graphics.rp1 = pointIdx
	return nil
}

// ============================================================
// MIAP — Move Indirect Absolute Point
// ============================================================

// opMiap implements MIAP[a] (0x3E-0x3F).
// Moves a point to a CVT value, optionally rounding.
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opMiap(opcode byte) error {
	cvtIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	pointIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	doRound := opcode&1 != 0
	// Read CVT value
	var cvtDist int32
	if cvtIdx >= 0 && cvtIdx < len(e.cvt) {
		cvtDist = e.cvt[cvtIdx]
	} else if e.graphics.isPedantic {
		return ttErrInvalidCvtIndex
	}
	z := e.zone(e.graphics.zp0)
	pt, err := z.point(pointIdx)
	if err != nil {
		if e.graphics.isPedantic {
			return err
		}
		e.graphics.rp0 = pointIdx
		e.graphics.rp1 = pointIdx
		return nil
	}
	curDist := e.graphics.project(pt[0], pt[1], 0, 0)
	distance := cvtDist - curDist
	if doRound {
		// Check cutin
		diff := cvtDist - curDist
		if diff < 0 {
			diff = -diff
		}
		if diff > e.graphics.retained.controlValueCutin {
			cvtDist = curDist
		}
		distance = e.graphics.roundState.round(cvtDist) - curDist
	}
	if err := z.movePoint(&e.graphics, pointIdx, distance); err != nil {
		if e.graphics.isPedantic {
			return err
		}
	}
	// NOTE: movePoint now handles touching (skrifa pattern).
	e.graphics.rp0 = pointIdx
	e.graphics.rp1 = pointIdx
	return nil
}

// ============================================================
// MDRP — Move Direct Relative Point (32 variants)
// ============================================================

// opMdrp implements MDRP[abcde] (0xC0-0xDF).
// Moves a point relative to rp0 based on the original distance.
// Opcode bits: set_rp0 | use_min_dist | round | dist_type(2)
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opMdrp(opcode byte) error {
	pointIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	flags := opcode - opMDRP00000
	setRP0 := flags&16 != 0
	useMinDist := flags&8 != 0
	doRound := flags&4 != 0

	z := e.zone(e.graphics.zp1)
	rp0z := e.zone(e.graphics.zp0)

	// Get original distance
	pt, err := z.originalPoint(pointIdx)
	if err != nil {
		if e.graphics.isPedantic {
			return err
		}
		return nil
	}
	rp0Pt, err := rp0z.originalPoint(e.graphics.rp0)
	if err != nil {
		if e.graphics.isPedantic {
			return err
		}
		return nil
	}
	origDist := e.graphics.dualProject(pt[0], pt[1], rp0Pt[0], rp0Pt[1])

	// Single width substitution
	distance := origDist
	if sw := e.graphics.retained.singleWidth; sw != 0 {
		diff := distance - sw
		if diff < 0 {
			diff = -diff
		}
		if diff < e.graphics.retained.singleWidthCutin {
			if distance >= 0 {
				distance = sw
			} else {
				distance = -sw
			}
		}
	}

	// Rounding
	if doRound {
		distance = e.graphics.roundState.round(distance)
	}

	// Minimum distance
	if useMinDist {
		minDist := e.graphics.retained.minDistance
		if origDist >= 0 {
			if distance < minDist {
				distance = minDist
			}
		} else {
			if distance > -minDist {
				distance = -minDist
			}
		}
	}

	// NOTE: backward compatibility is handled inside movePoint (skrifa pattern).
	// skrifa's op_mdrp does NOT check backward_compatibility.

	// Get current position and compute movement
	curPt, err := z.point(pointIdx)
	if err != nil {
		if e.graphics.isPedantic {
			return err
		}
		return nil
	}
	rp0CurPt, err := rp0z.point(e.graphics.rp0)
	if err != nil {
		if e.graphics.isPedantic {
			return err
		}
		return nil
	}
	curDist := e.graphics.project(curPt[0], curPt[1], rp0CurPt[0], rp0CurPt[1])
	if err := z.movePoint(&e.graphics, pointIdx, distance-curDist); err != nil {
		if e.graphics.isPedantic {
			return err
		}
	}

	e.graphics.rp1 = e.graphics.rp0
	e.graphics.rp2 = pointIdx
	if setRP0 {
		e.graphics.rp0 = pointIdx
	}
	return nil
}

// ============================================================
// MIRP — Move Indirect Relative Point (32 variants)
// ============================================================

// opMirp implements MIRP[abcde] (0xE0-0xFF).
// Like MDRP but uses CVT for the target distance.
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opMirp(opcode byte) error {
	cvtIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	pointIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	flags := opcode - opMIRP00000
	setRP0 := flags&16 != 0
	useMinDist := flags&8 != 0
	doRound := flags&4 != 0

	// Read CVT distance
	var cvtDist int32
	if cvtIdx >= 0 && cvtIdx < len(e.cvt) {
		cvtDist = e.cvt[cvtIdx]
	} else if e.graphics.isPedantic {
		return ttErrInvalidCvtIndex
	}

	// Auto flip
	if e.graphics.retained.autoFlip {
		z := e.zone(e.graphics.zp1)
		rp0z := e.zone(e.graphics.zp0)
		pt, e1 := z.originalPoint(pointIdx)
		rp0Pt, e2 := rp0z.originalPoint(e.graphics.rp0)
		if e1 == nil && e2 == nil {
			origDist := e.graphics.dualProject(pt[0], pt[1], rp0Pt[0], rp0Pt[1])
			if (origDist ^ cvtDist) < 0 {
				cvtDist = -cvtDist
			}
		}
	}

	// Single width substitution
	distance := cvtDist
	if sw := e.graphics.retained.singleWidth; sw != 0 {
		diff := distance - sw
		if diff < 0 {
			diff = -diff
		}
		if diff < e.graphics.retained.singleWidthCutin {
			if distance >= 0 {
				distance = sw
			} else {
				distance = -sw
			}
		}
	}

	// Rounding
	if doRound {
		// CVT cutin check
		z := e.zone(e.graphics.zp1)
		rp0z := e.zone(e.graphics.zp0)
		pt, e1 := z.originalPoint(pointIdx)
		rp0Pt, e2 := rp0z.originalPoint(e.graphics.rp0)
		if e1 == nil && e2 == nil {
			origDist := e.graphics.dualProject(pt[0], pt[1], rp0Pt[0], rp0Pt[1])
			diff := distance - origDist
			if diff < 0 {
				diff = -diff
			}
			if diff > e.graphics.retained.controlValueCutin {
				distance = origDist
			}
		}
		distance = e.graphics.roundState.round(distance)
	}

	// Minimum distance
	if useMinDist {
		minDist := e.graphics.retained.minDistance
		if cvtDist >= 0 {
			if distance < minDist {
				distance = minDist
			}
		} else {
			if distance > -minDist {
				distance = -minDist
			}
		}
	}

	// NOTE: backward compatibility is handled inside movePoint (skrifa pattern).
	// skrifa's op_mirp does NOT check backward_compatibility.

	// Apply movement
	z := e.zone(e.graphics.zp1)
	rp0z := e.zone(e.graphics.zp0)
	curPt, e1 := z.point(pointIdx)
	rp0CurPt, e2 := rp0z.point(e.graphics.rp0)
	if e1 != nil || e2 != nil {
		if e.graphics.isPedantic {
			if e1 != nil {
				return e1
			}
			return e2
		}
	} else {
		curDist := e.graphics.project(curPt[0], curPt[1], rp0CurPt[0], rp0CurPt[1])
		if err := z.movePoint(&e.graphics, pointIdx, distance-curDist); err != nil {
			if e.graphics.isPedantic {
				return err
			}
		}
	}

	e.graphics.rp1 = e.graphics.rp0
	e.graphics.rp2 = pointIdx
	if setRP0 {
		e.graphics.rp0 = pointIdx
	}
	return nil
}

// ============================================================
// MSIRP — Move Stack Indirect Relative Point
// ============================================================

// opMsirp implements MSIRP[a] (0x3A-0x3B).
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opMsirp(opcode byte) error {
	distance, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	pointIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	// NOTE: backward compatibility is handled inside movePoint (skrifa pattern).
	// skrifa's op_msirp does NOT check backward_compatibility.
	z := e.zone(e.graphics.zp1)
	rp0z := e.zone(e.graphics.zp0)
	curPt, e1 := z.point(pointIdx)
	rp0CurPt, e2 := rp0z.point(e.graphics.rp0)
	if e1 != nil || e2 != nil {
		if e.graphics.isPedantic {
			if e1 != nil {
				return e1
			}
			return e2
		}
		return nil
	}
	curDist := e.graphics.project(curPt[0], curPt[1], rp0CurPt[0], rp0CurPt[1])
	if err := z.movePoint(&e.graphics, pointIdx, distance-curDist); err != nil {
		if e.graphics.isPedantic {
			return err
		}
	}
	e.graphics.rp1 = e.graphics.rp0
	e.graphics.rp2 = pointIdx
	if opcode&1 != 0 {
		e.graphics.rp0 = pointIdx
	}
	return nil
}

// ============================================================
// IUP — Interpolate Untouched Points
// ============================================================

// opIup implements IUP[a] (0x30-0x31).
// Interpolates all untouched points in a contour between touched ones.
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opIup(opcode byte) error {
	isX := opcode&1 != 0
	if isX {
		e.graphics.didIUPx = true
	} else {
		e.graphics.didIUPy = true
	}
	z := e.zone(ttZoneGlyph)
	if len(z.contours) == 0 || len(z.points) == 0 {
		return nil
	}
	contourStart := 0
	for _, contourEnd := range z.contours {
		end := int(contourEnd)
		if end >= len(z.points) {
			break
		}
		e.iupContour(z, contourStart, end, isX)
		contourStart = end + 1
	}
	return nil
}

// iupContour interpolates untouched points within one contour.
func (e *ttEngine) iupContour(z *ttZone, start, end int, isX bool) {
	if start > end {
		return
	}
	// Find the first touched point
	firstTouched := -1
	for i := start; i <= end; i++ {
		if e.isPointTouched(z, i, isX) {
			firstTouched = i
			break
		}
	}
	if firstTouched < 0 {
		return // No touched points in this contour
	}
	// Walk around the contour interpolating between touched points
	cur := firstTouched
	for {
		// Find next touched point (wrapping around)
		next := cur + 1
		if next > end {
			next = start
		}
		for next != cur && !e.isPointTouched(z, next, isX) {
			next++
			if next > end {
				next = start
			}
		}
		if next == cur {
			break // Only one touched point
		}
		// Interpolate points between cur and next
		e.iupInterpolateRange(z, start, end, cur, next, isX)
		cur = next
		if cur == firstTouched {
			break
		}
	}
}

// iupInterpolateRange interpolates untouched points between two
// touched reference points.
func (e *ttEngine) iupInterpolateRange(z *ttZone, contourStart, contourEnd, ref1, ref2 int, isX bool) {
	axis := 0
	if !isX {
		axis = 1
	}
	ref1Orig := z.original[ref1][axis]
	ref1Cur := z.points[ref1][axis]
	ref2Orig := z.original[ref2][axis]
	ref2Cur := z.points[ref2][axis]

	// Walk from ref1 to ref2 (wrapping)
	i := ref1 + 1
	if i > contourEnd {
		i = contourStart
	}
	for i != ref2 {
		if !e.isPointTouched(z, i, isX) {
			origCoord := z.original[i][axis]
			// Determine which reference is "before" and "after"
			var newCoord int32
			if ref1Orig <= ref2Orig {
				newCoord = iupInterpolate(origCoord, ref1Orig, ref1Cur, ref2Orig, ref2Cur)
			} else {
				newCoord = iupInterpolate(origCoord, ref2Orig, ref2Cur, ref1Orig, ref1Cur)
			}
			z.points[i][axis] = newCoord
		}
		i++
		if i > contourEnd {
			i = contourStart
		}
	}
}

// iupInterpolate computes the interpolated coordinate.
// origBefore/origAfter must be in ascending order.
func iupInterpolate(origCoord, origBefore, curBefore, origAfter, curAfter int32) int32 {
	if origCoord <= origBefore {
		// Before the range — shift by same delta
		return curBefore + (origCoord - origBefore)
	}
	if origCoord >= origAfter {
		// After the range — shift by same delta
		return curAfter + (origCoord - origAfter)
	}
	// Within range — linear interpolation
	origRange := origAfter - origBefore
	if origRange == 0 {
		return curBefore
	}
	curRange := curAfter - curBefore
	return curBefore + ttMulDiv(origCoord-origBefore, curRange, origRange)
}

// ============================================================
// IP — Interpolate Point
// ============================================================

// opIp implements IP[] (0x39).
// Moves each point so that its relationship to rp1 and rp2 is the same
// as it was in the original uninstructed outline.
// Reference: skrifa hint/engine/outline.rs op_ip
func (e *ttEngine) opIP() error {
	gs := &e.graphics
	loop := gs.loopCounter
	gs.loopCounter = 1

	// Bounds-check reference points; if invalid and not pedantic, drain stack and return.
	rp1z := e.zone(gs.zp0)
	rp2z := e.zone(gs.zp1)
	if gs.rp1 < 0 || gs.rp1 >= rp1z.pointCount() || gs.rp2 < 0 || gs.rp2 >= rp2z.pointCount() {
		if gs.isPedantic {
			return ttErrInvalidPointIndex
		}
		return e.valueStack.popN(int(loop))
	}

	// In twilight zone, use original points; otherwise use unscaled points
	// treated as 26.6 fixed-point (skrifa: unscaled().map(F26Dot6::from_bits)).
	inTwilight := gs.zp0 == ttZoneTwilight || gs.zp1 == ttZoneTwilight || gs.zp2 == ttZoneTwilight

	// Compute original base (rp1 position in original/unscaled space).
	var orusBase [2]int32
	if inTwilight {
		pt, err := rp1z.originalPoint(gs.rp1)
		if err != nil {
			if gs.isPedantic {
				return err
			}
			return e.valueStack.popN(int(loop))
		}
		orusBase = pt
	} else {
		x, y := rp1z.unscaledPoint(gs.rp1)
		orusBase = [2]int32{x, y}
	}

	// Compute original range: distance from rp1 to rp2 in original space.
	var oldRange int32
	if inTwilight {
		pt, err := rp2z.originalPoint(gs.rp2)
		if err != nil {
			if gs.isPedantic {
				return err
			}
			return e.valueStack.popN(int(loop))
		}
		oldRange = gs.dualProject(pt[0], pt[1], orusBase[0], orusBase[1])
	} else {
		x, y := rp2z.unscaledPoint(gs.rp2)
		oldRange = gs.dualProject(x, y, orusBase[0], orusBase[1])
	}

	// Compute current base and range in hinted space.
	curBase, _ := rp1z.point(gs.rp1)
	rp2Cur, _ := rp2z.point(gs.rp2)
	curRange := gs.project(rp2Cur[0], rp2Cur[1], curBase[0], curBase[1])

	z := e.zone(gs.zp2)
	for i := int32(0); i < loop; i++ {
		pointIdx, err := e.valueStack.popUsize()
		if err != nil {
			return err
		}
		// Bounds-check each point.
		if pointIdx < 0 || pointIdx >= z.pointCount() {
			if gs.isPedantic {
				return ttErrInvalidPointIndex
			}
			continue
		}

		// Compute original distance of point from rp1.
		var origDist int32
		if inTwilight {
			pt, err := z.originalPoint(pointIdx)
			if err != nil {
				if gs.isPedantic {
					return err
				}
				continue
			}
			origDist = gs.dualProject(pt[0], pt[1], orusBase[0], orusBase[1])
		} else {
			x, y := z.unscaledPoint(pointIdx)
			origDist = gs.dualProject(x, y, orusBase[0], orusBase[1])
		}

		// Compute current distance of point from rp1 in hinted space.
		curPt, _ := z.point(pointIdx)
		curDist := gs.project(curPt[0], curPt[1], curBase[0], curBase[1])

		// Compute new distance: scale original distance by (curRange / oldRange).
		var newDist int32
		if origDist != 0 {
			if oldRange != 0 {
				newDist = ttMulDiv(origDist, curRange, oldRange)
			} else {
				newDist = origDist
			}
		}

		if err := z.movePoint(gs, pointIdx, newDist-curDist); err != nil {
			return err
		}
		// NOTE: movePoint now handles touching (skrifa pattern).
	}
	return nil
}

// ============================================================
// ALIGNRP — Align to Reference Point
// ============================================================

// opAlignrp implements ALIGNRP[] (0x3C).
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opAlignrp() error {
	loop := e.graphics.loopCounter
	e.graphics.loopCounter = 1
	for i := int32(0); i < loop; i++ {
		pointIdx, err := e.valueStack.popUsize()
		if err != nil {
			return err
		}
		// NOTE: backward compatibility handled inside movePoint (skrifa pattern).
		// skrifa's op_alignrp does NOT check backward_compatibility.
		z := e.zone(e.graphics.zp1)
		rp0z := e.zone(e.graphics.zp0)
		curPt, e1 := z.point(pointIdx)
		rp0Pt, e2 := rp0z.point(e.graphics.rp0)
		if e1 != nil || e2 != nil {
			if e.graphics.isPedantic {
				if e1 != nil {
					return e1
				}
				return e2
			}
			continue
		}
		dist := e.graphics.project(curPt[0], curPt[1], rp0Pt[0], rp0Pt[1])
		if err := z.movePoint(&e.graphics, pointIdx, -dist); err != nil {
			return err
		}
	}
	return nil
}

// ============================================================
// ALIGNPTS — Align Two Points
// ============================================================

// opAlignpts implements ALIGNPTS[] (0x27).
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opAlignpts() error {
	p2Idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	p1Idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	z0 := e.zone(e.graphics.zp0)
	z1 := e.zone(e.graphics.zp1)
	pt1, e1 := z0.point(p1Idx)
	pt2, e2 := z1.point(p2Idx)
	if e1 != nil || e2 != nil {
		if e.graphics.isPedantic {
			if e1 != nil {
				return e1
			}
			return e2
		}
		return nil
	}
	dist := e.graphics.project(pt1[0], pt1[1], pt2[0], pt2[1])
	if err := z0.movePoint(&e.graphics, p1Idx, -dist/2); err != nil {
		return err
	}
	if err := z1.movePoint(&e.graphics, p2Idx, dist/2); err != nil {
		return err
	}
	// NOTE: movePoint now handles touching (skrifa pattern).
	return nil
}

// ============================================================
// ISECT — Move Point to Intersection
// ============================================================

// opIsect implements ISECT[] (0x0F).
// Moves a point to the intersection of lines (a0→a1) and (b0→b1).
// Uses FreeType/skrifa determinant-based intersection with grazing angle rejection.
// Reference: skrifa hint/engine/outline.rs op_isect
func (e *ttEngine) opIsect() error {
	b1Idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	b0Idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	a1Idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	a0Idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	pointIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}

	// Line A: zp1 zone, Line B: zp0 zone, Target point: zp2 zone.
	// (skrifa uses zp1 for a-points, zp0 for b-points)
	za := e.zone(e.graphics.zp1)
	zb := e.zone(e.graphics.zp0)

	pa0, ea0 := za.point(a0Idx)
	pa1, ea1 := za.point(a1Idx)
	pb0, eb0 := zb.point(b0Idx)
	pb1, eb1 := zb.point(b1Idx)
	if ea0 != nil || ea1 != nil || eb0 != nil || eb1 != nil {
		if e.graphics.isPedantic {
			for _, e := range []error{ea0, ea1, eb0, eb1} {
				if e != nil {
					return e
				}
			}
		}
		return nil
	}

	// Direction vectors for each line.
	dax := pa1[0] - pa0[0]
	day := pa1[1] - pa0[1]
	dbx := pb1[0] - pb0[0]
	dby := pb1[1] - pb0[1]

	// Vector from a0 to b0.
	dx := pb0[0] - pa0[0]
	dy := pb0[1] - pa0[1]

	// Cross product (discriminant) and dot product of direction vectors.
	// discriminant = da × (-db) = dax*(-dby) + day*dbx
	// dotproduct   = da · db   = dax*dbx + day*dby
	discriminant := ttMulDiv(dax, -dby, 0x40) + ttMulDiv(day, dbx, 0x40)
	dotproduct := ttMulDiv(dax, dbx, 0x40) + ttMulDiv(day, dby, 0x40)

	// FreeType/skrifa grazing angle rejection:
	// Reject if |sin(angle)| / |cos(angle)| < 1/19 (angle < ~3 degrees).
	// This avoids numerical instability for near-parallel lines.
	z := e.zone(e.graphics.zp2)

	absDiscriminant := discriminant
	if absDiscriminant < 0 {
		absDiscriminant = -absDiscriminant
	}
	absDotproduct := dotproduct
	if absDotproduct < 0 {
		absDotproduct = -absDotproduct
	}

	if absDiscriminant*19 > absDotproduct {
		// Intersection is well-defined — compute it.
		v := ttMulDiv(dx, -dby, 0x40) + ttMulDiv(dy, dbx, 0x40)
		ix := ttMulDiv(v, dax, discriminant)
		iy := ttMulDiv(v, day, discriminant)
		if err := z.setPoint(pointIdx, pa0[0]+ix, pa0[1]+iy); err != nil {
			return err
		}
	} else {
		// Lines are nearly parallel — use midpoint of a0 and b0.
		if err := z.setPoint(pointIdx, (pa0[0]+pa1[0]+pb0[0]+pb1[0])/4, (pa0[1]+pa1[1]+pb0[1]+pb1[1])/4); err != nil {
			return err
		}
	}

	// ISECT always touches in both X and Y (skrifa: CoordAxis::Both).
	z.touchX(pointIdx)
	z.touchY(pointIdx)
	return nil
}

// ============================================================
// SHP, SHC, SHZ — Shift Point/Contour/Zone
// ============================================================

// opShp implements SHP[a] (0x32-0x33).
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opShp(opcode byte) error {
	loop := e.graphics.loopCounter
	e.graphics.loopCounter = 1
	var rpIdx int
	var rpZone *ttZone
	if opcode&1 != 0 {
		rpIdx = e.graphics.rp1
		rpZone = e.zone(e.graphics.zp0)
	} else {
		rpIdx = e.graphics.rp2
		rpZone = e.zone(e.graphics.zp1)
	}
	rpPt, e1 := rpZone.point(rpIdx)
	rpOrig, e2 := rpZone.originalPoint(rpIdx)
	if e1 != nil || e2 != nil {
		// Drain loop values from stack
		return e.valueStack.popN(int(loop))
	}
	displacement := e.graphics.project(rpPt[0], rpPt[1], rpOrig[0], rpOrig[1])
	z := e.zone(e.graphics.zp2)
	for i := int32(0); i < loop; i++ {
		pointIdx, err := e.valueStack.popUsize()
		if err != nil {
			return err
		}
		// NOTE: backward compatibility handled inside movePoint (skrifa pattern).
		// skrifa's op_shp uses move_zp2_point which embeds backward compat.
		if err := z.movePoint(&e.graphics, pointIdx, displacement); err != nil {
			return err
		}
	}
	return nil
}

// opShc implements SHC[a] (0x34-0x35).
// Shifts all points in a contour.
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opShc(opcode byte) error {
	contourIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	// NOTE: backward compatibility handled inside movePoint (skrifa pattern).
	var rpIdx int
	var rpZone *ttZone
	if opcode&1 != 0 {
		rpIdx = e.graphics.rp1
		rpZone = e.zone(e.graphics.zp0)
	} else {
		rpIdx = e.graphics.rp2
		rpZone = e.zone(e.graphics.zp1)
	}
	rpPt, rpErr := rpZone.point(rpIdx)
	rpOrig, rpOrigErr := rpZone.originalPoint(rpIdx)
	if rpErr != nil || rpOrigErr != nil {
		if e.graphics.isPedantic {
			if rpErr != nil {
				return rpErr
			}
			return rpOrigErr
		}
		return nil
	}
	displacement := e.graphics.project(rpPt[0], rpPt[1], rpOrig[0], rpOrig[1])
	z := e.zone(e.graphics.zp2)
	end, err := z.contourEnd(contourIdx)
	if err != nil {
		if e.graphics.isPedantic {
			return err
		}
		return nil
	}
	start := 0
	if contourIdx > 0 {
		prev, prevErr := z.contourEnd(contourIdx - 1)
		if prevErr == nil {
			start = prev + 1
		}
	}
	for i := start; i <= end && i < len(z.points); i++ {
		if err := z.movePoint(&e.graphics, i, displacement); err != nil {
			return err
		}
	}
	return nil
}

// opShz implements SHZ[a] (0x36-0x37).
// Shifts all points in a zone.
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opShz(opcode byte) error {
	zoneIdx, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	// NOTE: backward compatibility handled inside movePoint (skrifa pattern).
	zp, err := ttZonePointerFromInt32(zoneIdx)
	if err != nil {
		if e.graphics.isPedantic {
			return err
		}
		return nil
	}
	var rpIdx int
	var rpZone *ttZone
	if opcode&1 != 0 {
		rpIdx = e.graphics.rp1
		rpZone = e.zone(e.graphics.zp0)
	} else {
		rpIdx = e.graphics.rp2
		rpZone = e.zone(e.graphics.zp1)
	}
	rpPt, rpErr := rpZone.point(rpIdx)
	rpOrig, rpOrigErr := rpZone.originalPoint(rpIdx)
	if rpErr != nil || rpOrigErr != nil {
		if e.graphics.isPedantic {
			if rpErr != nil {
				return rpErr
			}
			return rpOrigErr
		}
		return nil
	}
	displacement := e.graphics.project(rpPt[0], rpPt[1], rpOrig[0], rpOrig[1])
	z := e.zone(zp)
	for i := 0; i < len(z.points); i++ {
		if err := z.movePoint(&e.graphics, i, displacement); err != nil {
			return err
		}
	}
	return nil
}

// ============================================================
// SHPIX — Shift Point by Pixel Amount
// ============================================================

// opShpix implements SHPIX[] (0x38).
// Reference: skrifa hint/engine/outline.rs:221-244
func (e *ttEngine) opShpix() error {
	gs := &e.graphics
	inTwilight := gs.zp0 == ttZoneTwilight || gs.zp1 == ttZoneTwilight || gs.zp2 == ttZoneTwilight
	distance, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	loop := gs.loopCounter
	gs.loopCounter = 1
	didIUP := gs.didIUPx && gs.didIUPy
	z := e.zone(gs.zp2)
	for i := int32(0); i < loop; i++ {
		pointIdx, err := e.valueStack.popUsize()
		if err != nil {
			return err
		}
		if gs.backwardCompatibility {
			// In backward compat mode, SHPIX has its own gating logic:
			// only move if in twilight zone, or if IUP hasn't been done
			// and either (composite with Y freedom) or (point is Y-touched).
			// Reference: skrifa hint/engine/outline.rs:232-239
			if inTwilight ||
				(!didIUP &&
					((gs.isComposite && gs.freedomVector[1] != 0) ||
						z.isTouchedY(pointIdx))) {
				if err := z.movePoint(gs, pointIdx, distance); err != nil {
					return err
				}
			}
		} else {
			if err := z.movePoint(gs, pointIdx, distance); err != nil {
				return err
			}
		}
	}
	return nil
}

// ============================================================
// UTP — Untouch Point
// ============================================================

// opUtp implements UTP[] (0x29).
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opUtp() error {
	pointIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	z := e.zone(e.graphics.zp0)
	z.untouch(pointIdx)
	return nil
}

// ============================================================
// FLIPPT, FLIPRGON, FLIPRGOFF
// ============================================================

// opFlippt implements FLIPPT[] (0x80).
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opFlippt() error {
	loop := e.graphics.loopCounter
	e.graphics.loopCounter = 1
	if e.graphics.backwardCompatibility && e.graphics.didIUPx && e.graphics.didIUPy {
		return e.valueStack.popN(int(loop))
	}
	z := e.zone(e.graphics.zp0)
	for i := int32(0); i < loop; i++ {
		pointIdx, err := e.valueStack.popUsize()
		if err != nil {
			return err
		}
		z.flipOnCurve(pointIdx)
	}
	return nil
}

// opFliprgon implements FLIPRGON[] (0x81).
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opFliprgon() error {
	hi, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	lo, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	z := e.zone(e.graphics.zp0)
	for i := lo; i <= hi && i < len(z.flags); i++ {
		z.setOnCurve(i, true)
	}
	return nil
}

// opFliprgoff implements FLIPRGOFF[] (0x82).
// Reference: skrifa hint/engine/outline.rs
func (e *ttEngine) opFliprgoff() error {
	hi, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	lo, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	z := e.zone(e.graphics.zp0)
	for i := lo; i <= hi && i < len(z.flags); i++ {
		z.setOnCurve(i, false)
	}
	return nil
}

// ============================================================
// Helper: check if point is touched along an axis
// ============================================================

// isPointTouched checks if a point has been touched along the specified axis.
func (e *ttEngine) isPointTouched(z *ttZone, index int, isX bool) bool {
	if isX {
		return z.isTouchedX(index)
	}
	return z.isTouchedY(index)
}
