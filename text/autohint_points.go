package text

// Point propagation for the auto-hinter.
//
// After edges are grid-fitted, their adjustments must be propagated to
// ALL outline points. This happens in three passes:
//
//  1. alignEdgePoints: Points directly on edges snap to edge.pos.
//  2. alignStrongPoints: Non-weak points between edges are interpolated
//     (equivalent to TrueType's IP instruction).
//  3. alignWeakPoints: Remaining points are interpolated within their
//     contour (equivalent to TrueType's IUP instruction).
//
// All coordinates are in 26.6 fixed-point (1 unit = 1/64 pixel).
//
// References:
//   - FreeType afhints.c:1324 af_glyph_hints_align_edge_points
//   - FreeType afhints.c:1399 af_glyph_hints_align_strong_points
//   - FreeType afhints.c:1673 af_glyph_hints_align_weak_points
//   - skrifa hint/outline.rs  align_edge_points, align_strong_points, align_weak_points

// alignEdgePoints adjusts all points that belong to edge segments.
//
// For Default script group, points snap directly to edge.pos.
// For CJK script group, points are shifted by the edge delta (edge.pos - edge.opos)
// unless HORIZONTAL_SNAP / VERTICAL_SNAP flags are set. This matches skrifa
// hint/outline.rs:42-44 which conditionally snaps for CJK:
//
//	let snap = group == ScriptGroup::Default
//	    || (axis.dim == Dim::H && scale.flags.contains(HORIZONTAL_SNAP))
//	    || (axis.dim == Dim::V && scale.flags.contains(VERTICAL_SNAP));
//
// For our default target (Normal, not LCD, not Mono), snap flags are not set,
// so CJK uses delta mode. This is critical for CJK hinting accuracy.
//
// CRITICAL: walks the contour linked list (point.next) from firstPt to lastPt,
// NOT an array index range. This matches FreeType afhints.c:1355 and
// skrifa hint/outline.rs:50-73. Using array index iteration is wrong because:
//   - When firstPt > lastPt (contour wrap-around), array iteration visits ZERO points
//   - Array iteration may include non-segment points that happen to be in the range
//
// See FreeType afhints.c:1324 and skrifa hint/outline.rs:31.
// See FreeType afcjk.c:2195 for CJK snap configuration.
func alignEdgePoints(pa *hintPointArray, segments []hintSegment, edges []*hintEdge, dim hintDimension, group scriptGroup) {
	touchFlag := pointFlagTouchedY
	if dim == dimHorizontal {
		touchFlag = pointFlagTouchedX
	}

	// CJK uses delta mode (shift by edge.pos - edge.opos) unless snap flags are set.
	// For our default target (Normal), snap flags are not set.
	// Default script group always snaps.
	snap := group == scriptGroupDefault

	for si := range segments {
		seg := &segments[si]
		if seg.edgeIdx < 0 || int(seg.edgeIdx) >= len(edges) {
			continue
		}
		edge := edges[seg.edgeIdx]
		delta := edge.pos - edge.opos

		// Walk the contour linked list from firstPt to lastPt.
		// This is the FreeType/skrifa pattern: seg->first → point->next → ... → seg->last.
		ptIdx := seg.firstPt
		if ptIdx < 0 || ptIdx >= len(pa.pts) {
			continue
		}
		for {
			pt := &pa.pts[ptIdx]
			alignEdgePointCoord(pt, dim, edge.pos, delta, snap)
			pt.flags |= touchFlag

			if ptIdx == seg.lastPt {
				break
			}
			ptIdx = pt.next
			// Safety: prevent infinite loops on malformed contours.
			if ptIdx == seg.firstPt {
				break
			}
		}
	}
}

// alignStrongPoints interpolates non-weak, untouched points between edges.
// This is equivalent to the TrueType IP (Interpolate Point) instruction.
// All coordinates in 26.6 fixed-point.
//
// For each untouched, non-weak point:
//   - If before the first edge → shift by first edge's delta
//   - If after the last edge → shift by last edge's delta
//   - If between two edges → linearly interpolate based on edge movement
//
// Matches skrifa hint/outline.rs:81 which uses:
//   - point.fy/fx (i32, font units) for fpos comparison
//   - point.oy/ox (i32, 26.6 scaled) for position computation
//   - fixed_mul/fixed_div (16.16) for scale interpolation
//
// See FreeType afhints.c:1399 and skrifa hint/outline.rs:81.
func alignStrongPoints(pa *hintPointArray, edges []*hintEdge, dim hintDimension) {
	if len(edges) == 0 {
		return
	}

	touchFlag := pointFlagTouchedY
	if dim == dimHorizontal {
		touchFlag = pointFlagTouchedX
	}

	for i := range pa.pts {
		pt := &pa.pts[i]

		// Skip already touched or weak points.
		if (pt.flags & (touchFlag | pointFlagWeak)) != 0 {
			continue
		}

		u := pt.fontU(dim)  // Font-unit coordinate (float32, matches edge.fpos)
		ou := pt.origU(dim) // Original scaled, 26.6

		// Is the point before the first edge?
		firstEdge := edges[0]
		if u <= firstEdge.fpos {
			storePoint26dot6(pt, dim, firstEdge.pos-(firstEdge.opos-ou))
			pt.flags |= touchFlag
			continue
		}

		// Is the point after the last edge?
		lastEdge := edges[len(edges)-1]
		if u >= lastEdge.fpos {
			storePoint26dot6(pt, dim, lastEdge.pos+(ou-lastEdge.opos))
			pt.flags |= touchFlag
			continue
		}

		// Find enclosing edges and interpolate.
		// Linear search for small edge counts (most common case).
		// Note: this is critical for matching FreeType in cases where we have
		// more than one edge with the same fpos. Linear and binary searches
		// can produce different results.
		for j := 0; j < len(edges)-1; j++ {
			before := edges[j]
			after := edges[j+1]

			if u == before.fpos {
				storePoint26dot6(pt, dim, before.pos)
				pt.flags |= touchFlag
				break
			}
			if u == after.fpos {
				storePoint26dot6(pt, dim, after.pos)
				pt.flags |= touchFlag
				break
			}

			if u > before.fpos && u < after.fpos { //nolint:nestif // FreeType aflatin.c port
				// Interpolate using 16.16 fixed-point scale.
				// Matches skrifa: scale = fixed_div(after.pos - before.pos, after.fpos - before.fpos)
				// then: result = before.pos + fixed_mul(u - before.fpos, scale)
				denom := after.fpos - before.fpos
				if denom == 0 {
					storePoint26dot6(pt, dim, before.pos)
				} else {
					// Use cached scale if available, otherwise compute and cache.
					scale := before.scale
					if scale == 0 {
						scale = fixedDiv26dot6(after.pos-before.pos, int32(denom))
						before.scale = scale
						edges[j].scale = scale
					}
					fposInt := int32(u - before.fpos) // font unit delta as integer
					storePoint26dot6(pt, dim, before.pos+fixedMul26dot6(fposInt, scale))
				}
				pt.flags |= touchFlag
				break
			}
		}
	}
}

// storePoint26dot6 sets the adjusted coordinate for a point (26.6).
func storePoint26dot6(pt *hintPoint, dim hintDimension, val int32) {
	if dim == dimHorizontal {
		pt.x = val
	} else {
		pt.y = val
	}
}

// alignEdgePointCoord applies edge position to a point coordinate.
// In snap mode (Default group), sets the coordinate to edge.pos.
// In delta mode (CJK group), shifts the coordinate by delta (edge.pos - edge.opos).
func alignEdgePointCoord(pt *hintPoint, dim hintDimension, pos, delta int32, snap bool) {
	switch {
	case snap:
		storePoint26dot6(pt, dim, pos)
	case dim == dimHorizontal:
		pt.x += delta
	default:
		pt.y += delta
	}
}

// alignWeakPoints interpolates remaining untouched (weak) points.
// This is equivalent to the TrueType IUP (Interpolate Untouched Points)
// instruction. It works per-contour. All coordinates in 26.6.
//
// For each contour:
//  1. Find touched points
//  2. For each span of untouched points between two touched points,
//     interpolate based on the touched points' movement
//  3. Handle wrap-around at contour boundaries
//
// Matches skrifa hint/outline.rs:204 exactly:
//   - Copies x→u, ox→v (or y→u, oy→v) before processing
//   - IUP operates on u/v fields
//   - Writes u back to x (or y) after processing
//
// See FreeType afhints.c:1673 and skrifa hint/outline.rs:204.
//
//nolint:gocognit,gocyclo,cyclop // FreeType afhints.c port — algorithmic complexity is inherent
func alignWeakPoints(pa *hintPointArray, dim hintDimension) {
	touchFlag := pointFlagTouchedY
	if dim == dimHorizontal {
		touchFlag = pointFlagTouchedX
	}

	// Copy current coordinates into u/v for interpolation.
	// Matches skrifa: point.u = point.x, point.v = point.ox (horizontal)
	//            or:  point.u = point.y, point.v = point.oy (vertical)
	for i := range pa.pts {
		pt := &pa.pts[i]
		if dim == dimHorizontal {
			pt.u = pt.x
			pt.v = pt.ox
		} else {
			pt.u = pt.y
			pt.v = pt.oy
		}
	}

	for _, cr := range pa.contours {
		pts := pa.pts[cr.start : cr.end+1]
		if len(pts) < 2 {
			continue
		}

		// Find first touched point in contour.
		firstTouched := -1
		for i, pt := range pts {
			if (pt.flags & touchFlag) != 0 {
				firstTouched = i
				break
			}
		}

		if firstTouched < 0 {
			continue // No touched points in this contour.
		}

		lastIdx := len(pts) - 1
		pointIdx := firstTouched
		var lastTouched int

		// Walk through touched points, interpolating gaps.
		for {
			// Skip consecutive touched points.
			for pointIdx < lastIdx && (pts[pointIdx+1].flags&touchFlag) != 0 {
				pointIdx++
			}
			lastTouched = pointIdx

			// Find next touched point.
			pointIdx++
			nextTouched := -1
			for pointIdx <= lastIdx {
				if (pts[pointIdx].flags & touchFlag) != 0 {
					nextTouched = pointIdx
					break
				}
				pointIdx++
			}

			if nextTouched < 0 {
				break // No more touched points.
			}

			// Interpolate points between lastTouched and nextTouched.
			if lastTouched+1 < nextTouched {
				iupInterpolate26dot6(pts, lastTouched+1, nextTouched-1, lastTouched, nextTouched)
			}
		}

		if lastTouched == firstTouched {
			// Only one touched point: shift everything.
			iupShift26dot6(pts, 0, lastIdx, firstTouched)
		} else {
			// Handle wrap-around.
			if lastTouched < lastIdx {
				iupInterpolate26dot6(pts, lastTouched+1, lastIdx, lastTouched, firstTouched)
			}
			if firstTouched > 0 {
				iupInterpolate26dot6(pts, 0, firstTouched-1, lastTouched, firstTouched)
			}
		}
	}

	// Write back interpolated values from u to x/y.
	// Matches skrifa: point.x = point.u (horizontal) or point.y = point.u (vertical).
	for i := range pa.pts {
		pt := &pa.pts[i]
		if dim == dimHorizontal {
			pt.x = pt.u
		} else {
			pt.y = pt.u
		}
	}
}

// iupShift26dot6 shifts all untouched points by the same delta as the reference point.
// Operates on u/v fields (26.6 fixed-point).
//
// See FreeType afhints.c:1578 and skrifa hint/outline.rs:312.
func iupShift26dot6(pts []hintPoint, p1, p2, refIdx int) {
	ref := &pts[refIdx]
	delta := ref.u - ref.v
	if delta == 0 {
		return
	}

	for i := p1; i <= p2; i++ {
		if i == refIdx {
			continue
		}
		pts[i].u = pts[i].v + delta
	}
}

// iupInterpolate26dot6 interpolates untouched points between two reference points.
// Operates on u/v fields (26.6 fixed-point).
//
// See FreeType afhints.c:1605 and skrifa hint/outline.rs:335.
//
//nolint:nestif // FreeType afhints.c port — algorithmic complexity is inherent
func iupInterpolate26dot6(pts []hintPoint, p1, p2, ref1, ref2 int) {
	if p1 > p2 {
		return
	}

	u1, v1 := pts[ref1].u, pts[ref1].v
	u2, v2 := pts[ref2].u, pts[ref2].v

	// Ensure v1 <= v2 for consistent interpolation.
	if v1 > v2 {
		u1, u2 = u2, u1
		v1, v2 = v2, v1
	}

	d1 := u1 - v1
	d2 := u2 - v2

	if u1 == u2 || v1 == v2 {
		// Degenerate case: shift or clamp.
		for i := p1; i <= p2; i++ {
			v := pts[i].v
			if v <= v1 { //nolint:gocritic // FreeType afhints.c port — value range if-else chain
				pts[i].u = v + d1
			} else if v >= v2 {
				pts[i].u = v + d2
			} else {
				pts[i].u = u1
			}
		}
	} else {
		// Linear interpolation using 16.16 fixed-point scale.
		scale := fixedDiv26dot6(u2-u1, v2-v1)
		for i := p1; i <= p2; i++ {
			v := pts[i].v
			if v <= v1 { //nolint:gocritic // FreeType afhints.c port — value range if-else chain
				pts[i].u = v + d1
			} else if v >= v2 {
				pts[i].u = v + d2
			} else {
				pts[i].u = u1 + fixedMul26dot6(v-v1, scale)
			}
		}
	}
}
