// TrueType bytecode interpreter — stack management and push instructions.
//
// Port of skrifa hint/engine/stack.rs (225 LOC).
// Implements: DUP, POP, CLEAR, SWAP, DEPTH, CINDEX, MINDEX, ROLL,
// NPUSHB, NPUSHW, PUSHB, PUSHW.
//
// Reference: skrifa/src/outline/glyf/hint/engine/stack.rs
package text

// opDup implements DUP[] (0x20).
// Reference: skrifa hint/engine/stack.rs:24-26
func (e *ttEngine) opDup() error {
	return e.valueStack.dup()
}

// opPop implements POP[] (0x21).
// Reference: skrifa hint/engine/stack.rs:38-40
func (e *ttEngine) opPop() error {
	_, err := e.valueStack.pop()
	return err
}

// opClear implements CLEAR[] (0x22).
// Reference: skrifa hint/engine/stack.rs:53-56
func (e *ttEngine) opClear() error {
	e.valueStack.clear()
	return nil
}

// opSwap implements SWAP[] (0x23).
// Reference: skrifa hint/engine/stack.rs:70-72
func (e *ttEngine) opSwap() error {
	return e.valueStack.swap()
}

// opDepth implements DEPTH[] (0x24).
// Reference: skrifa hint/engine/stack.rs:84-87
func (e *ttEngine) opDepth() error {
	n := e.valueStack.len()
	return e.valueStack.push(int32(n))
}

// opCindex implements CINDEX[] (0x25).
// Reference: skrifa hint/engine/stack.rs:100-102
func (e *ttEngine) opCindex() error {
	return e.valueStack.copyIndex()
}

// opMindex implements MINDEX[] (0x26).
// Reference: skrifa hint/engine/stack.rs:115-117
func (e *ttEngine) opMindex() error {
	return e.valueStack.moveIndex()
}

// opRoll implements ROLL[] (0x8A).
// Reference: skrifa hint/engine/stack.rs:133-135
func (e *ttEngine) opRoll() error {
	return e.valueStack.roll()
}

// opNpushb implements NPUSHB[] (0x40).
// Reads n (1 byte) then n unsigned bytes from the instruction stream
// and pushes them onto the stack.
// Reference: skrifa hint/engine/stack.rs:171-173
func (e *ttEngine) opNpushb() error {
	n, ok := e.program.decoder.nextByte()
	if !ok {
		return ttErrUnexpectedEndOfBytecode
	}
	return e.opPushBytes(int(n))
}

// opNpushw implements NPUSHW[] (0x41).
// Reads n (1 byte) then n signed 16-bit words from the instruction stream
// and pushes them onto the stack.
// Reference: skrifa hint/engine/stack.rs:171-173
func (e *ttEngine) opNpushw() error {
	n, ok := e.program.decoder.nextByte()
	if !ok {
		return ttErrUnexpectedEndOfBytecode
	}
	return e.opPushWords(int(n))
}

// opPushBytes reads count unsigned bytes from the instruction stream
// and pushes them as int32 values.
func (e *ttEngine) opPushBytes(count int) error {
	for i := 0; i < count; i++ {
		b, ok := e.program.decoder.nextByte()
		if !ok {
			return ttErrUnexpectedEndOfBytecode
		}
		if err := e.valueStack.push(int32(b)); err != nil {
			return err
		}
	}
	return nil
}

// opPushWords reads count signed 16-bit words (big-endian) from the
// instruction stream and pushes them as int32 values.
func (e *ttEngine) opPushWords(count int) error {
	for i := 0; i < count; i++ {
		w, ok := e.program.decoder.nextWord()
		if !ok {
			return ttErrUnexpectedEndOfBytecode
		}
		if err := e.valueStack.push(int32(w)); err != nil {
			return err
		}
	}
	return nil
}
