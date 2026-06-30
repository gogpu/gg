// TrueType bytecode interpreter — program state.
//
// Port of skrifa hint/program.rs (163 LOC).
// Manages bytecode storage, instruction decoding, and the call stack
// for the three program types (Font, ControlValue, Glyph).
//
// Reference: skrifa/src/outline/glyf/hint/program.rs
package text

// ttProgramType describes the source of a piece of bytecode.
// Reference: skrifa hint/program.rs:13-24
type ttProgramType uint8

const (
	// ttProgramFont is the font program (fpgm table).
	// Initializes function and instruction tables.
	ttProgramFont ttProgramType = 0
	// ttProgramControlValue is the CV program (prep table).
	// Initializes CVT and storage based on font size.
	ttProgramControlValue ttProgramType = 1
	// ttProgramGlyph is a per-glyph program (glyf table).
	ttProgramGlyph ttProgramType = 2
)

// ttProgramState manages active programs and instruction decoding.
// Reference: skrifa hint/program.rs:27-38
type ttProgramState struct {
	// bytecode for each of the three program types.
	bytecode [3][]byte
	// initial program when execution begins.
	initial ttProgramType
	// current active program.
	current ttProgramType
	// decoder for reading instructions from bytecode.
	decoder ttDecoder
	// callStack tracks nested function/instruction calls.
	callStack ttCallStack
}

// newTTProgramState creates a program state for the given bytecodes.
// Reference: skrifa hint/program.rs:41-55
func newTTProgramState(fontCode, cvCode, glyphCode []byte, initial ttProgramType) ttProgramState {
	return ttProgramState{
		bytecode:  [3][]byte{fontCode, cvCode, glyphCode},
		initial:   initial,
		current:   initial,
		decoder:   newTTDecoder(getBytecodeSlice([3][]byte{fontCode, cvCode, glyphCode}, initial)),
		callStack: ttCallStack{},
	}
}

// getBytecodeSlice returns the bytecode for the given program type.
func getBytecodeSlice(bytecode [3][]byte, program ttProgramType) []byte {
	switch program {
	case ttProgramFont:
		return bytecode[0]
	case ttProgramControlValue:
		return bytecode[1]
	case ttProgramGlyph:
		return bytecode[2]
	default:
		return nil
	}
}

// resetProgram resets the state for execution of the given program.
// Reference: skrifa hint/program.rs:58-63
func (ps *ttProgramState) resetProgram(program ttProgramType) {
	ps.initial = program
	ps.current = program
	ps.decoder = newTTDecoder(getBytecodeSlice(ps.bytecode, program))
	ps.callStack.clear()
}

// enter jumps to the code in the given definition and sets it up for
// execution count times (for LOOPCALL).
// Reference: skrifa hint/program.rs:67-79
func (ps *ttProgramState) enter(def ttDefinition, count int32) error {
	program := def.program()
	pc := def.start
	err := ps.callStack.push(ttCallRecord{
		callerProgram: ps.current,
		returnPC:      ps.decoder.pc,
		currentCount:  count,
		definition:    def,
	})
	if err != nil {
		return err
	}
	ps.current = program
	ps.decoder = newTTDecoder(getBytecodeSlice(ps.bytecode, program))
	ps.decoder.pc = int(pc)
	return nil
}

// leave exits the current function definition.
// If loop count > 1, restarts from the beginning of the definition.
// Otherwise, resumes at the caller.
// Reference: skrifa hint/program.rs:87-101
func (ps *ttProgramState) leave() error {
	record, err := ps.callStack.pop()
	if err != nil {
		return err
	}
	if record.currentCount > 1 {
		// Loop call with iterations remaining.
		record.currentCount--
		ps.decoder.pc = int(record.definition.start)
		return ps.callStack.push(record)
	}
	// Return to caller.
	ps.current = record.callerProgram
	ps.decoder.bytecode = getBytecodeSlice(ps.bytecode, record.callerProgram)
	ps.decoder.pc = record.returnPC
	return nil
}

// ttDecoder reads TrueType bytecode instructions.
// Simple sequential decoder — opcodes are single bytes, operands
// follow inline.
type ttDecoder struct {
	bytecode []byte
	pc       int
}

// newTTDecoder creates a decoder for the given bytecode.
func newTTDecoder(bytecode []byte) ttDecoder {
	return ttDecoder{
		bytecode: bytecode,
		pc:       0,
	}
}

// done returns true if there are no more instructions to decode.
func (d *ttDecoder) done() bool {
	return d.pc >= len(d.bytecode)
}

// nextByte reads a single byte from the instruction stream.
func (d *ttDecoder) nextByte() (byte, bool) {
	if d.pc >= len(d.bytecode) {
		return 0, false
	}
	b := d.bytecode[d.pc]
	d.pc++
	return b, true
}

// nextWord reads a signed 16-bit word (big-endian) from the stream.
func (d *ttDecoder) nextWord() (int16, bool) {
	if d.pc+1 >= len(d.bytecode) {
		return 0, false
	}
	hi := d.bytecode[d.pc]
	lo := d.bytecode[d.pc+1]
	d.pc += 2
	return int16(uint16(hi)<<8 | uint16(lo)), true
}

// skipBytes advances the decoder by n bytes.
func (d *ttDecoder) skipBytes(n int) {
	d.pc += n
}

// skipInstruction advances past the current instruction's inline
// operands (for IF/ELSE scanning that must skip PUSH instructions).
func (d *ttDecoder) skipInstructionOperands(opcode byte) {
	switch {
	case opcode == 0x40: // NPUSHB
		if n, ok := d.nextByte(); ok {
			d.skipBytes(int(n))
		}
	case opcode == 0x41: // NPUSHW
		if n, ok := d.nextByte(); ok {
			d.skipBytes(int(n) * 2)
		}
	case opcode >= 0xB0 && opcode <= 0xB7: // PUSHB[n]
		count := int(opcode-0xB0) + 1
		d.skipBytes(count)
	case opcode >= 0xB8 && opcode <= 0xBF: // PUSHW[n]
		count := int(opcode-0xB8) + 1
		d.skipBytes(count * 2)
	}
}
