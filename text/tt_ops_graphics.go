// TrueType bytecode interpreter — graphics state instructions.
//
// Port of skrifa hint/engine/graphics.rs (1154 LOC).
// Implements vector setting, reference points, zone pointers, and
// round mode instructions.
//
// Reference: skrifa/src/outline/glyf/hint/engine/graphics.rs
package text

// ============================================================
// Vector Setting (SVTCA, SPVTCA, SFVTCA, SPVTL, SFVTL, SDPVTL,
//   SPVFS, SFVFS, GPV, GFV, SFVTPV)
// ============================================================

// opSvtca implements SVTCA/SPVTCA/SFVTCA (0x00-0x05).
// Sets projection and/or freedom vectors to the X or Y axis.
// Reference: skrifa hint/engine/graphics.rs
func (e *ttEngine) opSvtca(opcode byte) error {
	// opcodes 0x00-0x01: set both vectors
	// opcodes 0x02-0x03: set projection vector only
	// opcodes 0x04-0x05: set freedom vector only
	// odd opcodes: X axis, even opcodes: Y axis
	var vec [2]int32
	if opcode&1 != 0 {
		vec = [2]int32{0x4000, 0} // X axis
	} else {
		vec = [2]int32{0, 0x4000} // Y axis
	}
	switch {
	case opcode <= opSVTCA1:
		e.graphics.projVector = vec
		e.graphics.dualProjVector = vec
		e.graphics.freedomVector = vec
	case opcode <= opSPVTCA1:
		e.graphics.projVector = vec
		e.graphics.dualProjVector = vec
	default: // SFVTCA
		e.graphics.freedomVector = vec
	}
	e.graphics.updateProjectionState()
	return nil
}

// opSvtl implements SPVTL/SFVTL (0x06-0x09).
// Sets projection or freedom vector to a line between two points.
// Reference: skrifa hint/engine/graphics.rs
func (e *ttEngine) opSvtl(opcode byte) error {
	p2Idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	p1Idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	z1 := e.zone(e.graphics.zp1)
	z2 := e.zone(e.graphics.zp2)
	pt1, e1 := z1.point(p1Idx)
	pt2, e2 := z2.point(p2Idx)
	if e1 != nil || e2 != nil {
		if e.graphics.isPedantic {
			if e1 != nil {
				return e1
			}
			return e2
		}
		return nil
	}
	dx := pt1[0] - pt2[0]
	dy := pt1[1] - pt2[1]
	// Perpendicular rotation for odd opcodes (SPVTL1, SFVTL1)
	if opcode&1 != 0 {
		dx, dy = dy, -dx
	}
	vx, vy := ttNormalize14(dx, dy)
	vec := [2]int32{vx, vy}
	if opcode <= opSPVTL1 {
		e.graphics.projVector = vec
		e.graphics.dualProjVector = vec
	} else {
		e.graphics.freedomVector = vec
	}
	e.graphics.updateProjectionState()
	return nil
}

// opSdpvtl implements SDPVTL[a] (0x86-0x87).
// Sets dual projection vector to a line, using original positions.
// Reference: skrifa hint/engine/graphics.rs
func (e *ttEngine) opSdpvtl(opcode byte) error {
	p2Idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	p1Idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	z1 := e.zone(e.graphics.zp1)
	z2 := e.zone(e.graphics.zp2)
	// Current positions for projection vector
	pt1, e1 := z1.point(p1Idx)
	pt2, e2 := z2.point(p2Idx)
	if e1 != nil || e2 != nil {
		if e.graphics.isPedantic {
			if e1 != nil {
				return e1
			}
			return e2
		}
		return nil
	}
	dx := pt1[0] - pt2[0]
	dy := pt1[1] - pt2[1]
	if opcode&1 != 0 {
		dx, dy = dy, -dx
	}
	vx, vy := ttNormalize14(dx, dy)
	e.graphics.projVector = [2]int32{vx, vy}
	// Original positions for dual projection vector
	opt1, e1 := z1.originalPoint(p1Idx)
	opt2, e2 := z2.originalPoint(p2Idx)
	if e1 != nil || e2 != nil {
		if e.graphics.isPedantic {
			if e1 != nil {
				return e1
			}
			return e2
		}
		return nil
	}
	dx = opt1[0] - opt2[0]
	dy = opt1[1] - opt2[1]
	if opcode&1 != 0 {
		dx, dy = dy, -dx
	}
	dvx, dvy := ttNormalize14(dx, dy)
	e.graphics.dualProjVector = [2]int32{dvx, dvy}
	e.graphics.updateProjectionState()
	return nil
}

// opSpvfs implements SPVFS[] (0x0A).
// Sets projection vector from two 2.14 values on the stack.
// Reference: skrifa hint/engine/graphics.rs
func (e *ttEngine) opSpvfs() error {
	y, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	x, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	vx, vy := ttNormalize14(x, y)
	e.graphics.projVector = [2]int32{vx, vy}
	e.graphics.dualProjVector = [2]int32{vx, vy}
	e.graphics.updateProjectionState()
	return nil
}

// opSfvfs implements SFVFS[] (0x0B).
// Sets freedom vector from two 2.14 values on the stack.
// Reference: skrifa hint/engine/graphics.rs
func (e *ttEngine) opSfvfs() error {
	y, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	x, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	vx, vy := ttNormalize14(x, y)
	e.graphics.freedomVector = [2]int32{vx, vy}
	e.graphics.updateProjectionState()
	return nil
}

// opGpv implements GPV[] (0x0C).
// Pushes the projection vector (x, y) as 2.14 values.
// Reference: skrifa hint/engine/graphics.rs
func (e *ttEngine) opGpv() error {
	if err := e.valueStack.push(e.graphics.projVector[0]); err != nil {
		return err
	}
	return e.valueStack.push(e.graphics.projVector[1])
}

// opGfv implements GFV[] (0x0D).
// Pushes the freedom vector (x, y) as 2.14 values.
// Reference: skrifa hint/engine/graphics.rs
func (e *ttEngine) opGfv() error {
	if err := e.valueStack.push(e.graphics.freedomVector[0]); err != nil {
		return err
	}
	return e.valueStack.push(e.graphics.freedomVector[1])
}

// opSfvtpv implements SFVTPV[] (0x0E).
// Sets the freedom vector equal to the projection vector.
// Reference: skrifa hint/engine/graphics.rs
func (e *ttEngine) opSfvtpv() error {
	e.graphics.freedomVector = e.graphics.projVector
	e.graphics.updateProjectionState()
	return nil
}

// ============================================================
// Reference Points (SRP0, SRP1, SRP2)
// ============================================================

// opSrp0 implements SRP0[] (0x10).
func (e *ttEngine) opSrp0() error {
	v, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	e.graphics.rp0 = v
	return nil
}

// opSrp1 implements SRP1[] (0x11).
func (e *ttEngine) opSrp1() error {
	v, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	e.graphics.rp1 = v
	return nil
}

// opSrp2 implements SRP2[] (0x12).
func (e *ttEngine) opSrp2() error {
	v, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	e.graphics.rp2 = v
	return nil
}

// ============================================================
// Zone Pointers (SZP0, SZP1, SZP2, SZPS)
// ============================================================

// opSzp0 implements SZP0[] (0x13).
func (e *ttEngine) opSzp0() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	zp, err := ttZonePointerFromInt32(v)
	if err != nil {
		return err
	}
	e.graphics.zp0 = zp
	return nil
}

// opSzp1 implements SZP1[] (0x14).
func (e *ttEngine) opSzp1() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	zp, err := ttZonePointerFromInt32(v)
	if err != nil {
		return err
	}
	e.graphics.zp1 = zp
	return nil
}

// opSzp2 implements SZP2[] (0x15).
func (e *ttEngine) opSzp2() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	zp, err := ttZonePointerFromInt32(v)
	if err != nil {
		return err
	}
	e.graphics.zp2 = zp
	return nil
}

// opSzps implements SZPS[] (0x16).
// Sets all three zone pointers to the same value.
func (e *ttEngine) opSzps() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	zp, err := ttZonePointerFromInt32(v)
	if err != nil {
		return err
	}
	e.graphics.zp0 = zp
	e.graphics.zp1 = zp
	e.graphics.zp2 = zp
	return nil
}

// ============================================================
// Rounding Mode Instructions
// ============================================================

// opRtg implements RTG[] (0x18).
func (e *ttEngine) opRtg() error {
	e.graphics.roundState.mode = ttRoundGrid
	return nil
}

// opRthg implements RTHG[] (0x19).
func (e *ttEngine) opRthg() error {
	e.graphics.roundState.mode = ttRoundHalfGrid
	return nil
}

// opRtdg implements RTDG[] (0x3D).
func (e *ttEngine) opRtdg() error {
	e.graphics.roundState.mode = ttRoundDoubleGrid
	return nil
}

// opRutg implements RUTG[] (0x7C).
func (e *ttEngine) opRutg() error {
	e.graphics.roundState.mode = ttRoundUpToGrid
	return nil
}

// opRdtg implements RDTG[] (0x7D).
func (e *ttEngine) opRdtg() error {
	e.graphics.roundState.mode = ttRoundDownToGrid
	return nil
}

// opRoff implements ROFF[] (0x7A).
func (e *ttEngine) opRoff() error {
	e.graphics.roundState.mode = ttRoundOff
	return nil
}

// opSround implements SROUND[] (0x76).
// Reference: skrifa hint/engine/round.rs
func (e *ttEngine) opSround() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	e.graphics.roundState.mode = ttRoundSuper
	e.setSuperRoundParams(v, 1)
	return nil
}

// opS45round implements S45ROUND[] (0x77).
// Reference: skrifa hint/engine/round.rs
func (e *ttEngine) opS45round() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	e.graphics.roundState.mode = ttRoundSuper45
	e.setSuperRoundParams(v, 46) // sqrt(2)/2 * 64 ≈ 46
	return nil
}

// setSuperRoundParams extracts period, phase, and threshold from the
// SROUND/S45ROUND operand.
// Reference: skrifa hint/engine/round.rs
func (e *ttEngine) setSuperRoundParams(operand int32, gridPeriod int32) {
	// Period: bits 7-6
	switch (operand >> 6) & 3 {
	case 0:
		e.graphics.roundState.period = gridPeriod / 2
	case 1:
		e.graphics.roundState.period = gridPeriod
	case 2:
		e.graphics.roundState.period = gridPeriod * 2
	default:
		// Reserved, keep current
	}
	// Clamp period
	if e.graphics.roundState.period == 0 {
		e.graphics.roundState.period = 1
	}
	// Phase: bits 5-4
	switch (operand >> 4) & 3 {
	case 0:
		e.graphics.roundState.phase = 0
	case 1:
		e.graphics.roundState.phase = e.graphics.roundState.period / 4
	case 2:
		e.graphics.roundState.phase = e.graphics.roundState.period / 2
	case 3:
		e.graphics.roundState.phase = e.graphics.roundState.period * 3 / 4
	}
	// Threshold: bits 3-0
	threshold := operand & 0xF
	if threshold == 0 {
		e.graphics.roundState.threshold = e.graphics.roundState.period - 1
	} else {
		e.graphics.roundState.threshold = ((threshold - 4) * e.graphics.roundState.period) / 8
	}
}
