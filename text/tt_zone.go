// TrueType bytecode interpreter — glyph zones.
//
// Port of skrifa hint/zone.rs (835 LOC).
// Manages twilight and glyph zone point arrays for the interpreter.
//
// Reference: skrifa/src/outline/glyf/hint/zone.rs
package text

// ttZonePointer selects either the twilight or glyph zone.
// Reference: skrifa hint/zone.rs:19-25
type ttZonePointer uint8

const (
	ttZoneTwilight ttZonePointer = 0
	ttZoneGlyph    ttZonePointer = 1
)

// ttZonePointerFromInt32 converts an int32 to a zone pointer, validating range.
func ttZonePointerFromInt32(v int32) (ttZonePointer, error) {
	switch v {
	case 0:
		return ttZoneTwilight, nil
	case 1:
		return ttZoneGlyph, nil
	default:
		return 0, ttErrInvalidZoneIndex
	}
}

// ttPointFlags tracks per-point state during hinting.
// Reference: skrifa zone.rs uses PointFlags from read_fonts
type ttPointFlags uint8

const (
	ttPointFlagOnCurve  ttPointFlags = 1 << 0
	ttPointFlagTouchedX ttPointFlags = 1 << 1
	ttPointFlagTouchedY ttPointFlags = 1 << 2
)

// ttZone is a glyph zone containing point arrays for hinting.
//
// Each zone has three sets of points:
//   - unscaled: original font-unit points
//   - original: scaled but unhinted points (snapshot before hinting)
//   - points: current (hinted) points — modified by instructions
//
// Reference: skrifa hint/zone.rs:48-58
type ttZone struct {
	unscaled []int32        // pairs of (x, y) in font units
	original [][2]int32     // scaled points (x, y) in 26.6
	points   [][2]int32     // current hinted points (x, y) in 26.6
	flags    []ttPointFlags // per-point flags
	contours []uint16       // end-of-contour point indices
}

// pointCount returns the number of points in this zone.
func (z *ttZone) pointCount() int {
	return len(z.points)
}

// point returns the hinted point at the given index.
// Reference: skrifa hint/zone.rs:78-83
func (z *ttZone) point(index int) ([2]int32, error) {
	if index < 0 || index >= len(z.points) {
		return [2]int32{}, ttErrInvalidPointIndex
	}
	return z.points[index], nil
}

// setPoint sets the hinted point at the given index.
func (z *ttZone) setPoint(index int, x, y int32) error {
	if index < 0 || index >= len(z.points) {
		return ttErrInvalidPointIndex
	}
	z.points[index] = [2]int32{x, y}
	return nil
}

// originalPoint returns the original (pre-hinting) point at the given index.
// Reference: skrifa hint/zone.rs:89-94
func (z *ttZone) originalPoint(index int) ([2]int32, error) {
	if index < 0 || index >= len(z.original) {
		return [2]int32{}, ttErrInvalidPointIndex
	}
	return z.original[index], nil
}

// setOriginalPoint sets the original point at the given index.
func (z *ttZone) setOriginalPoint(index int, x, y int32) error {
	if index < 0 || index >= len(z.original) {
		return ttErrInvalidPointIndex
	}
	z.original[index] = [2]int32{x, y}
	return nil
}

// unscaledPoint returns the unscaled point at the given index as (x, y).
// If the index is out of range, returns (0, 0).
// Reference: skrifa hint/zone.rs:100-107
func (z *ttZone) unscaledPoint(index int) (int32, int32) {
	i := index * 2
	if i < 0 || i+1 >= len(z.unscaled) {
		return 0, 0
	}
	return z.unscaled[i], z.unscaled[i+1]
}

// unscaledCoord returns a single coordinate from the unscaled array.
// axis: 0 = x, 1 = y.
// If the index is out of range, returns 0.
func (z *ttZone) unscaledCoord(index, axis int) int32 {
	i := index*2 + axis
	if i < 0 || i >= len(z.unscaled) {
		return 0
	}
	return z.unscaled[i]
}

// touchX marks a point as touched in the X direction.
func (z *ttZone) touchX(index int) {
	if index >= 0 && index < len(z.flags) {
		z.flags[index] |= ttPointFlagTouchedX
	}
}

// touchY marks a point as touched in the Y direction.
func (z *ttZone) touchY(index int) {
	if index >= 0 && index < len(z.flags) {
		z.flags[index] |= ttPointFlagTouchedY
	}
}

// untouch clears both touch flags for a point.
func (z *ttZone) untouch(index int) {
	if index >= 0 && index < len(z.flags) {
		z.flags[index] &^= ttPointFlagTouchedX | ttPointFlagTouchedY
	}
}

// isTouchedX returns true if the point was touched in X.
func (z *ttZone) isTouchedX(index int) bool {
	if index < 0 || index >= len(z.flags) {
		return false
	}
	return z.flags[index]&ttPointFlagTouchedX != 0
}

// isTouchedY returns true if the point was touched in Y.
func (z *ttZone) isTouchedY(index int) bool {
	if index < 0 || index >= len(z.flags) {
		return false
	}
	return z.flags[index]&ttPointFlagTouchedY != 0
}

// isOnCurve returns true if the point is on-curve.
func (z *ttZone) isOnCurve(index int) bool {
	if index < 0 || index >= len(z.flags) {
		return false
	}
	return z.flags[index]&ttPointFlagOnCurve != 0
}

// setOnCurve sets the on-curve flag for a point.
func (z *ttZone) setOnCurve(index int, onCurve bool) {
	if index >= 0 && index < len(z.flags) {
		if onCurve {
			z.flags[index] |= ttPointFlagOnCurve
		} else {
			z.flags[index] &^= ttPointFlagOnCurve
		}
	}
}

// flipOnCurve toggles the on-curve flag for a point.
func (z *ttZone) flipOnCurve(index int) {
	if index >= 0 && index < len(z.flags) {
		z.flags[index] ^= ttPointFlagOnCurve
	}
}

// contourEnd returns the last point index for the given contour.
func (z *ttZone) contourEnd(contourIndex int) (int, error) {
	if contourIndex < 0 || contourIndex >= len(z.contours) {
		return 0, ttErrInvalidContourIndex
	}
	return int(z.contours[contourIndex]), nil
}

// movePoint moves a point along the freedom vector by distance d.
// The distance d is projected along the freedom vector and scaled
// by 1/fdotp to account for the angle between projection and freedom
// vectors.
//
// In backward compatibility mode (ClearType):
//   - X adjustments are suppressed (point.x NOT moved, but still touched)
//   - Y adjustments are suppressed ONLY after IUP has been done on both axes
//
// This matches skrifa zone.rs:417-468 where backward compat is enforced at the
// move level, NOT at the instruction level. Individual instructions (MDRP, MIRP,
// etc.) must NOT check backward compat themselves.
//
// Reference: skrifa hint/zone.rs:417-468
func (z *ttZone) movePoint(gs *ttGraphicsState, index int, distance int32) error {
	if index < 0 || index >= len(z.points) {
		if gs.isPedantic {
			return ttErrInvalidPointIndex
		}
		return nil
	}

	backCompat := gs.backwardCompatibility
	backCompatAndDidIUP := backCompat && gs.didIUPx && gs.didIUPy

	switch gs.freedomAxis {
	case ttCoordX:
		if !backCompat {
			z.points[index][0] += distance
		}
		z.touchX(index)
	case ttCoordY:
		if !backCompatAndDidIUP {
			z.points[index][1] += distance
		}
		z.touchY(index)
	default:
		// Non-axis-aligned freedom vector: decompose distance along each axis
		// using mul_div(distance, fv_component, fdotp) = distance * fv / fdotp.
		// This matches skrifa zone.rs:448-464 exactly.
		fv := gs.freedomVector
		fdotp := gs.fdotp
		if fv[0] != 0 {
			if !backCompat {
				z.points[index][0] += ttMulDiv(distance, fv[0], fdotp)
			}
			z.touchX(index)
		}
		if fv[1] != 0 {
			if !backCompatAndDidIUP {
				z.points[index][1] += ttMulDiv(distance, fv[1], fdotp)
			}
			z.touchY(index)
		}
	}
	return nil
}
