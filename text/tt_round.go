// TrueType bytecode interpreter — round state.
//
// Port of skrifa hint/round.rs (240 LOC).
// Implements all TrueType rounding modes: Grid, HalfGrid, DoubleGrid,
// DownToGrid, UpToGrid, Off, Super, Super45.
//
// Reference: skrifa/src/outline/glyf/hint/round.rs
package text

// ttRoundMode selects the rounding strategy.
// Reference: skrifa hint/round.rs:6-48
type ttRoundMode uint8

const (
	// ttRoundGrid rounds to nearest grid line. Set by RTG.
	ttRoundGrid ttRoundMode = iota
	// ttRoundHalfGrid rounds to nearest half grid line. Set by RTHG.
	ttRoundHalfGrid
	// ttRoundDoubleGrid rounds to nearest half or integer pixel. Set by RTDG.
	ttRoundDoubleGrid
	// ttRoundDownToGrid rounds down to nearest integer. Set by RDTG.
	ttRoundDownToGrid
	// ttRoundUpToGrid rounds up to nearest integer. Set by RUTG.
	ttRoundUpToGrid
	// ttRoundOff disables rounding. Set by ROFF.
	ttRoundOff
	// ttRoundSuper uses custom period/phase/threshold. Set by SROUND.
	ttRoundSuper
	// ttRoundSuper45 like Super but with sqrt(2)/2 period. Set by S45ROUND.
	ttRoundSuper45
)

// ttRoundState controls rounding behavior.
// Reference: skrifa hint/round.rs:53-59
type ttRoundState struct {
	mode      ttRoundMode
	threshold int32
	phase     int32
	period    int32
}

// defaultRoundState returns the default round state (Grid mode, period=64).
// Reference: skrifa hint/round.rs:61-69
func defaultRoundState() ttRoundState {
	return ttRoundState{
		mode:      ttRoundGrid,
		threshold: 0,
		phase:     0,
		period:    64,
	}
}

// round applies the current rounding mode to a 26.6 distance value.
// Reference: skrifa hint/round.rs:73-165
func (rs *ttRoundState) round(distance int32) int32 {
	switch rs.mode {
	case ttRoundGrid:
		// Reference: skrifa hint/round.rs:87-93
		if distance >= 0 {
			r := ttRound26Dot6(distance)
			if r < 0 {
				return 0
			}
			return r
		}
		r := -ttRound26Dot6(-distance)
		if r > 0 {
			return 0
		}
		return r

	case ttRoundHalfGrid:
		// Reference: skrifa hint/round.rs:79-85
		if distance >= 0 {
			r := ttFloor26Dot6(distance) + 32
			if r < 0 {
				return 0
			}
			return r
		}
		r := -(ttFloor26Dot6(-distance) + 32)
		if r > 0 {
			return 0
		}
		return r

	case ttRoundDoubleGrid:
		// Reference: skrifa hint/round.rs:95-101
		if distance >= 0 {
			r := ttRoundPad(distance, 32)
			if r < 0 {
				return 0
			}
			return r
		}
		r := -ttRoundPad(-distance, 32)
		if r > 0 {
			return 0
		}
		return r

	case ttRoundDownToGrid:
		// Reference: skrifa hint/round.rs:103-109
		if distance >= 0 {
			r := ttFloor26Dot6(distance)
			if r < 0 {
				return 0
			}
			return r
		}
		r := -ttFloor26Dot6(-distance)
		if r > 0 {
			return 0
		}
		return r

	case ttRoundUpToGrid:
		// Reference: skrifa hint/round.rs:111-117
		if distance >= 0 {
			r := ttCeil26Dot6(distance)
			if r < 0 {
				return 0
			}
			return r
		}
		r := -ttCeil26Dot6(-distance)
		if r > 0 {
			return 0
		}
		return r

	case ttRoundSuper:
		// Reference: skrifa hint/round.rs:119-137
		if distance >= 0 {
			val := ((distance + (rs.threshold - rs.phase)) & -rs.period) + rs.phase
			if val < 0 {
				return rs.phase
			}
			return val
		}
		val := -(((rs.threshold - rs.phase) - distance) & -rs.period) - rs.phase
		if val > 0 {
			return -rs.phase
		}
		return val

	case ttRoundSuper45:
		// Reference: skrifa hint/round.rs:139-159
		if distance >= 0 {
			val := (((distance + (rs.threshold - rs.phase)) / rs.period) *
				rs.period) + rs.phase
			if val < 0 {
				return rs.phase
			}
			return val
		}
		val := -((((rs.threshold - rs.phase) - distance) / rs.period) *
			rs.period) - rs.phase
		if val > 0 {
			return -rs.phase
		}
		return val

	case ttRoundOff:
		// Reference: skrifa hint/round.rs:161
		return distance

	default:
		return distance
	}
}
