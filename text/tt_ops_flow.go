// TrueType bytecode interpreter — control flow, definitions, CVT,
// storage, data, delta, and miscellaneous instructions.
//
// Port of skrifa hint/engine/{control_flow.rs, definition.rs, cvt.rs,
// storage.rs, data.rs, delta.rs, misc.rs, round.rs}.
//
// Reference: skrifa/src/outline/glyf/hint/engine/
package text

// ============================================================
// Control Flow (IF, ELSE, EIF, JMPR, JROT, JROF)
// Reference: skrifa hint/engine/control_flow.rs
// ============================================================

// opIf implements IF[] (0x58).
// If top of stack is false (0), skip to matching ELSE or EIF.
// Reference: skrifa hint/engine/control_flow.rs:28-49
func (e *ttEngine) opIf() error {
	cond, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	if cond == 0 {
		// Skip to matching ELSE or EIF, handling nested IF blocks.
		nestDepth := 1
		for nestDepth > 0 {
			opcode, err := e.decodeNextOpcode()
			if err != nil {
				return err
			}
			switch opcode {
			case opIF:
				nestDepth++
			case opELSE:
				if nestDepth == 1 {
					return nil // Enter else branch
				}
			case opEIF:
				nestDepth--
			}
		}
	}
	return nil
}

// opElse implements ELSE[] (0x1B).
// When encountered during execution (true branch), skip to matching EIF.
// Reference: skrifa hint/engine/control_flow.rs:61-72
func (e *ttEngine) opElse() error {
	nestDepth := 1
	for nestDepth > 0 {
		opcode, err := e.decodeNextOpcode()
		if err != nil {
			return err
		}
		switch opcode {
		case opIF:
			nestDepth++
		case opEIF:
			nestDepth--
		}
	}
	return nil
}

// opJmpr implements JMPR[] (0x1C).
// Unconditional relative jump.
// Reference: skrifa hint/engine/control_flow.rs:124-126
func (e *ttEngine) opJmpr() error {
	return e.doJump(true)
}

// opJrot implements JROT[] (0x78).
// Jump relative on true.
// Reference: skrifa hint/engine/control_flow.rs:105-108
func (e *ttEngine) opJrot() error {
	cond, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	return e.doJump(cond != 0)
}

// opJrof implements JROF[] (0x79).
// Jump relative on false.
// Reference: skrifa hint/engine/control_flow.rs:155-158
func (e *ttEngine) opJrof() error {
	cond, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	return e.doJump(cond == 0)
}

// doJump executes a conditional jump.
// Offset is relative to the jump instruction itself.
// Reference: skrifa hint/engine/control_flow.rs:163-182
func (e *ttEngine) doJump(test bool) error {
	offset, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	// Offset is relative to the instruction, decoder already advanced past
	// the opcode, so subtract 1.
	jumpOffset := offset - 1
	if test {
		if jumpOffset < 0 {
			if jumpOffset == -1 {
				return ttErrInvalidJump
			}
			if err := e.loopBudget.doingBackwardJump(); err != nil {
				return err
			}
		}
		e.program.decoder.pc += int(jumpOffset)
	}
	return nil
}

// decodeNextOpcode reads the next opcode, skipping inline operands of
// push instructions (for IF/ELSE scanning).
// Reference: skrifa hint/engine/control_flow.rs:184-191
func (e *ttEngine) decodeNextOpcode() (byte, error) {
	opcode, ok := e.program.decoder.nextByte()
	if !ok {
		return 0, ttErrUnexpectedEndOfBytecode
	}
	// Skip inline operands of push instructions
	e.program.decoder.skipInstructionOperands(opcode)
	return opcode, nil
}

// ============================================================
// Function/Instruction Definitions (FDEF, ENDF, CALL, LOOPCALL, IDEF)
// Reference: skrifa hint/engine/definition.rs
// ============================================================

// opFdef implements FDEF[] (0x2C).
// Reference: skrifa hint/engine/definition.rs
func (e *ttEngine) opFdef() error {
	key, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	return e.doDef(&e.definitions.functions, key)
}

// opEndf implements ENDF[] (0x2D).
// Reference: skrifa hint/engine/definition.rs
func (e *ttEngine) opEndf() error {
	return e.program.leave()
}

// opCall implements CALL[] (0x2B).
// Reference: skrifa hint/engine/definition.rs
func (e *ttEngine) opCall() error {
	key, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	def, err := e.definitions.functions.get(key)
	if err != nil {
		return err
	}
	return e.program.enter(def, 1)
}

// opLoopcall implements LOOPCALL[] (0x2A).
// Reference: skrifa hint/engine/definition.rs
func (e *ttEngine) opLoopcall() error {
	key, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	count, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	if count <= 0 {
		return nil
	}
	if err := e.loopBudget.doingLoopCall(int(count)); err != nil {
		return err
	}
	def, err := e.definitions.functions.get(key)
	if err != nil {
		return err
	}
	return e.program.enter(def, count)
}

// opIdef implements IDEF[] (0x89).
// Reference: skrifa hint/engine/definition.rs
func (e *ttEngine) opIdef() error {
	key, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	return e.doDef(&e.definitions.instructions, key)
}

// doDef is the common code for FDEF and IDEF.
// Scans to matching ENDF and records the definition range.
// Reference: skrifa hint/engine/definition.rs:118-146 (do_def)
func (e *ttEngine) doDef(defs *ttDefinitionMap, key int32) error {
	if e.program.current == ttProgramGlyph {
		return ttErrDefinitionInGlyphProgram
	}
	idx, err := defs.allocate(key)
	if err != nil {
		return err
	}
	startPC := e.program.decoder.pc
	for {
		opcode, ok := e.program.decoder.nextByte()
		if !ok {
			return ttErrUnexpectedEndOfBytecode
		}
		e.program.decoder.skipInstructionOperands(opcode)
		if opcode == opENDF {
			break
		}
		if opcode == opFDEF || opcode == opIDEF {
			return ttErrNestedDefinition
		}
	}
	endPC := e.program.decoder.pc - 1
	if endPC-startPC > 65535 {
		return ttErrDefinitionTooLarge
	}
	d := &defs.defs[idx]
	d.start = int32(startPC)
	d.end = int32(endPC)
	d.prog = e.program.current
	d.isActive = true
	return nil
}

// opUnknown handles undefined opcodes — tries IDEF, else errors.
func (e *ttEngine) opUnknown(opcode byte) error {
	def, err := e.definitions.instructions.get(int32(opcode))
	if err != nil {
		if e.graphics.isPedantic {
			return ttErrUnhandledOpcode
		}
		return nil
	}
	return e.program.enter(def, 1)
}

// ============================================================
// CVT (RCVT, WCVTP, WCVTF)
// Reference: skrifa hint/engine/cvt.rs
// ============================================================

// opRcvt implements RCVT[] (0x45).
func (e *ttEngine) opRcvt() error {
	idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(e.cvt) {
		if e.graphics.isPedantic {
			return ttErrInvalidCvtIndex
		}
		return e.valueStack.push(0)
	}
	return e.valueStack.push(e.cvt[idx])
}

// opWcvtp implements WCVTP[] (0x44).
func (e *ttEngine) opWcvtp() error {
	value, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(e.cvt) {
		if e.graphics.isPedantic {
			return ttErrInvalidCvtIndex
		}
		return nil
	}
	e.cvt[idx] = value
	return nil
}

// opWcvtf implements WCVTF[] (0x70).
// Writes CVT in font units (converts to pixels using scale).
func (e *ttEngine) opWcvtf() error {
	value, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(e.cvt) {
		if e.graphics.isPedantic {
			return ttErrInvalidCvtIndex
		}
		return nil
	}
	e.cvt[idx] = ttMul16Dot16(value, e.graphics.retained.scale)
	return nil
}

// ============================================================
// Storage (RS, WS)
// Reference: skrifa hint/engine/storage.rs
// ============================================================

// opRs implements RS[] (0x43).
func (e *ttEngine) opRs() error {
	idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(e.storage) {
		if e.graphics.isPedantic {
			return ttErrInvalidStorageIndex
		}
		return e.valueStack.push(0)
	}
	return e.valueStack.push(e.storage[idx])
}

// opWs implements WS[] (0x42).
func (e *ttEngine) opWs() error {
	value, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	idx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(e.storage) {
		if e.graphics.isPedantic {
			return ttErrInvalidStorageIndex
		}
		return nil
	}
	e.storage[idx] = value
	return nil
}

// ============================================================
// Data (MPPEM, MPS, GC, SCFS, MD, GETINFO, GETVARIATION, GETDATA)
// Reference: skrifa hint/engine/data.rs
// ============================================================

// opMppem implements MPPEM[] (0x4B).
func (e *ttEngine) opMppem() error {
	return e.valueStack.push(e.graphics.retained.ppem)
}

// opMps implements MPS[] (0x4C).
// Returns ppem (same as MPPEM in modern interpreters).
func (e *ttEngine) opMps() error {
	return e.valueStack.push(e.graphics.retained.ppem)
}

// opGc implements GC[a] (0x46-0x47).
// Gets coordinate projected along projection vector.
func (e *ttEngine) opGc(opcode byte) error {
	pointIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	z := e.zone(e.graphics.zp2)
	if opcode == opGC0 {
		// Current position
		pt, err := z.point(pointIdx)
		if err != nil {
			if e.graphics.isPedantic {
				return err
			}
			return e.valueStack.push(0)
		}
		coord := e.graphics.project(pt[0], pt[1], 0, 0)
		return e.valueStack.push(coord)
	}
	// Original position
	pt, err := z.originalPoint(pointIdx)
	if err != nil {
		if e.graphics.isPedantic {
			return err
		}
		return e.valueStack.push(0)
	}
	coord := e.graphics.dualProject(pt[0], pt[1], 0, 0)
	return e.valueStack.push(coord)
}

// opScfs implements SCFS[] (0x48).
// Sets coordinate from stack.
func (e *ttEngine) opScfs() error {
	value, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	pointIdx, err := e.valueStack.popUsize()
	if err != nil {
		return err
	}
	z := e.zone(e.graphics.zp2)
	pt, err := z.point(pointIdx)
	if err != nil {
		if e.graphics.isPedantic {
			return err
		}
		return nil
	}
	curDist := e.graphics.project(pt[0], pt[1], 0, 0)
	return z.movePoint(&e.graphics, pointIdx, value-curDist)
}

// opMd implements MD[a] (0x49-0x4A).
// Measures distance between two points.
func (e *ttEngine) opMd(opcode byte) error {
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
	if opcode == opMD0 {
		// Current positions
		pt1, e1 := z0.point(p1Idx)
		pt2, e2 := z1.point(p2Idx)
		if e1 != nil || e2 != nil {
			if e.graphics.isPedantic {
				if e1 != nil {
					return e1
				}
				return e2
			}
			return e.valueStack.push(0)
		}
		d := e.graphics.project(pt1[0], pt1[1], pt2[0], pt2[1])
		return e.valueStack.push(d)
	}
	// Original positions
	pt1, e1 := z0.originalPoint(p1Idx)
	pt2, e2 := z1.originalPoint(p2Idx)
	if e1 != nil || e2 != nil {
		if e.graphics.isPedantic {
			if e1 != nil {
				return e1
			}
			return e2
		}
		return e.valueStack.push(0)
	}
	d := e.graphics.dualProject(pt1[0], pt1[1], pt2[0], pt2[1])
	return e.valueStack.push(d)
}

// opGetinfo implements GETINFO[] (0x88).
// Reference: skrifa hint/engine/misc.rs
func (e *ttEngine) opGetinfo() error {
	selector, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	result := int32(0)
	// Bit 0: version (we report version 40 = ClearType compatible)
	if selector&1 != 0 {
		result = 40
	}
	// Bit 1: glyph rotated
	if selector&2 != 0 && e.graphics.retained.isRotated {
		result |= 1 << 8
	}
	// Bit 2: glyph stretched
	if selector&4 != 0 && e.graphics.retained.isStretched {
		result |= 1 << 9
	}
	// Bit 5: ClearType enabled
	if selector&32 != 0 && e.graphics.retained.target.isSmooth() {
		result |= 1 << 12
	}
	// Bit 6: compatible width ClearType enabled
	if selector&64 != 0 && e.graphics.retained.target.preserveLinearMetrics() {
		result |= 1 << 13
	}
	// Bit 8: subpixel positioned (smooth targets)
	if selector&256 != 0 && e.graphics.retained.target.isSmooth() {
		result |= 1 << 15
	}
	// Bit 9: symmetric smoothing (smooth targets)
	if selector&512 != 0 && e.graphics.retained.target.isSmooth() {
		result |= 1 << 16
	}
	// Bit 10: gray ClearType (smooth targets)
	if selector&1024 != 0 && e.graphics.retained.target == ttTargetSmooth {
		result |= 1 << 17
	}
	return e.valueStack.push(result)
}

// opGetvariation implements GETVARIATION[] (0x91).
// Pushes the normalized variation coordinates onto the stack.
func (e *ttEngine) opGetvariation() error {
	if e.axisCount == 0 {
		return nil
	}
	for i := 0; i < e.axisCount; i++ {
		var coord int32
		if i < len(e.coords) {
			// 2.14 to 16.16
			coord = int32(e.coords[i]) << 2
		}
		if err := e.valueStack.push(coord); err != nil {
			return err
		}
	}
	return nil
}

// opGetdata implements GETDATA[] (0x92).
// Returns 17 (magic number per FreeType).
func (e *ttEngine) opGetdata() error {
	return e.valueStack.push(17)
}

// ============================================================
// Delta Exceptions (DELTAP1/2/3, DELTAC1/2/3)
// Reference: skrifa hint/engine/delta.rs
// ============================================================

// opDeltap implements DELTAP1/2/3 (0x5D, 0x71, 0x72).
// Reference: skrifa hint/engine/delta.rs
func (e *ttEngine) opDeltap(opcode byte) error {
	count, err := e.valueStack.popCountChecked()
	if err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		arg, err := e.valueStack.pop()
		if err != nil {
			return err
		}
		pointIdx, err := e.valueStack.popUsize()
		if err != nil {
			return err
		}
		// Compute the ppem range for this delta opcode
		var deltaBase int32
		switch opcode {
		case opDELTAP1:
			deltaBase = int32(e.graphics.retained.deltaBase)
		case opDELTAP2:
			deltaBase = int32(e.graphics.retained.deltaBase) + 16
		case opDELTAP3:
			deltaBase = int32(e.graphics.retained.deltaBase) + 32
		default:
			continue
		}
		ppemIdx := (arg >> 4) & 0xF
		targetPpem := deltaBase + ppemIdx
		if targetPpem != e.graphics.retained.ppem {
			continue
		}
		// Check backward compatibility suppression
		if e.graphics.backwardCompatibility {
			if e.graphics.didIUPx && e.graphics.didIUPy {
				continue
			}
		}
		// Compute the movement
		mag := arg & 0xF
		if mag >= 8 {
			mag -= 7
		} else {
			mag = -(8 - mag)
		}
		mag <<= 6 - int32(e.graphics.retained.deltaShift)
		z := e.zone(e.graphics.zp0)
		if err := z.movePoint(&e.graphics, pointIdx, mag); err != nil {
			if e.graphics.isPedantic {
				return err
			}
		}
	}
	return nil
}

// opDeltac implements DELTAC1/2/3 (0x73-0x75).
// Reference: skrifa hint/engine/delta.rs
func (e *ttEngine) opDeltac(opcode byte) error {
	count, err := e.valueStack.popCountChecked()
	if err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		arg, err := e.valueStack.pop()
		if err != nil {
			return err
		}
		cvtIdx, err := e.valueStack.popUsize()
		if err != nil {
			return err
		}
		var deltaBase int32
		switch opcode {
		case opDELTAC1:
			deltaBase = int32(e.graphics.retained.deltaBase)
		case opDELTAC2:
			deltaBase = int32(e.graphics.retained.deltaBase) + 16
		case opDELTAC3:
			deltaBase = int32(e.graphics.retained.deltaBase) + 32
		default:
			continue
		}
		ppemIdx := (arg >> 4) & 0xF
		targetPpem := deltaBase + ppemIdx
		if targetPpem != e.graphics.retained.ppem {
			continue
		}
		mag := arg & 0xF
		if mag >= 8 {
			mag -= 7
		} else {
			mag = -(8 - mag)
		}
		mag <<= 6 - int32(e.graphics.retained.deltaShift)
		if cvtIdx >= 0 && cvtIdx < len(e.cvt) {
			e.cvt[cvtIdx] += mag
		} else if e.graphics.isPedantic {
			return ttErrInvalidCvtIndex
		}
	}
	return nil
}

// ============================================================
// Miscellaneous (SANGW, SCANCTRL, SCANTYPE, INSTCTRL, FLIPON/OFF,
//   SDB, SDS, SLOOP, SMD, SCVTCI, SSWCI, SSW)
// Reference: skrifa hint/engine/misc.rs + graphics.rs
// ============================================================

// opSangw implements SANGW[] (0x7E) — deprecated no-op that pops 1 value.
func (e *ttEngine) opSangw() error {
	_, err := e.valueStack.pop()
	return err
}

// opScanctrl implements SCANCTRL[] (0x85).
func (e *ttEngine) opScanctrl() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	e.graphics.retained.scanControl = v != 0
	return nil
}

// opScantype implements SCANTYPE[] (0x8D).
func (e *ttEngine) opScantype() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	e.graphics.retained.scanType = v
	return nil
}

// opInstctrl implements INSTCTRL[] (0x8E).
// Reference: skrifa hint/engine/misc.rs
func (e *ttEngine) opInstctrl() error {
	selector, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	value, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	// Only font/CV programs can use INSTCTRL
	if e.program.current == ttProgramGlyph {
		return nil
	}
	switch selector {
	case 1:
		if value != 0 {
			e.graphics.retained.instructControl |= 1
		} else {
			e.graphics.retained.instructControl &^= 1
		}
	case 2:
		if value != 0 {
			e.graphics.retained.instructControl |= 2
		} else {
			e.graphics.retained.instructControl &^= 2
		}
	case 3:
		if e.graphics.retained.target.isSmooth() {
			if value != 0 {
				e.graphics.retained.instructControl |= 4
			} else {
				e.graphics.retained.instructControl &^= 4
			}
		}
	}
	return nil
}

// opFlipon implements FLIPON[] (0x4D).
func (e *ttEngine) opFlipon() error {
	e.graphics.retained.autoFlip = true
	return nil
}

// opFlipoff implements FLIPOFF[] (0x4E).
func (e *ttEngine) opFlipoff() error {
	e.graphics.retained.autoFlip = false
	return nil
}

// opSdb implements SDB[] (0x5E).
func (e *ttEngine) opSdb() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	e.graphics.retained.deltaBase = uint16(v) //nolint:gosec // value is naturally a u16
	return nil
}

// opSds implements SDS[] (0x5F).
func (e *ttEngine) opSds() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	e.graphics.retained.deltaShift = uint16(v) //nolint:gosec // value is naturally a u16
	return nil
}

// opSloop implements SLOOP[] (0x17).
func (e *ttEngine) opSloop() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	if v < 0 {
		if e.graphics.isPedantic {
			return ttErrNegativeLoopCounter
		}
		v = 1
	}
	e.graphics.loopCounter = v
	return nil
}

// opSmd implements SMD[] (0x1A).
func (e *ttEngine) opSmd() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	e.graphics.retained.minDistance = v
	return nil
}

// opScvtci implements SCVTCI[] (0x1D).
func (e *ttEngine) opScvtci() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	e.graphics.retained.controlValueCutin = v
	return nil
}

// opSswci implements SSWCI[] (0x1E).
func (e *ttEngine) opSswci() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	e.graphics.retained.singleWidthCutin = v
	return nil
}

// opSsw implements SSW[] (0x1F).
func (e *ttEngine) opSsw() error {
	v, err := e.valueStack.pop()
	if err != nil {
		return err
	}
	// Convert from font units to 26.6 pixels
	e.graphics.retained.singleWidth = ttMul16Dot16(v, e.graphics.retained.scale)
	return nil
}
