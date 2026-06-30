// TrueType bytecode interpreter — arithmetic and math instructions.
//
// Port of skrifa hint/engine/arith.rs (244 LOC).
// Implements: ADD, SUB, DIV, MUL, ABS, NEG, FLOOR, CEILING, MAX, MIN.
//
// Reference: skrifa/src/outline/glyf/hint/engine/arith.rs
package text

// opAdd implements ADD[] (0x60).
// Reference: skrifa hint/engine/arith.rs:20-22
func (e *ttEngine) opAdd() error {
	return e.valueStack.applyBinary(func(a, b int32) (int32, error) {
		return a + b, nil
	})
}

// opSub implements SUB[] (0x61).
// Reference: skrifa hint/engine/arith.rs:34-36
func (e *ttEngine) opSub() error {
	return e.valueStack.applyBinary(func(a, b int32) (int32, error) {
		return a - b, nil
	})
}

// opDiv implements DIV[] (0x62).
// Division in 26.6: a * 64 / b (since both are 26.6).
// Reference: skrifa hint/engine/arith.rs:49-57
func (e *ttEngine) opDiv() error {
	return e.valueStack.applyBinary(func(a, b int32) (int32, error) {
		if b == 0 {
			return 0, ttErrDivideByZero
		}
		return ttMulDivNoRound(a, 64, b), nil
	})
}

// opMul implements MUL[] (0x63).
// Multiply in 26.6: a * b / 64.
// Reference: skrifa hint/engine/arith.rs:69-72
func (e *ttEngine) opMul() error {
	return e.valueStack.applyBinary(func(a, b int32) (int32, error) {
		return ttMulDiv(a, b, 64), nil
	})
}

// opAbs implements ABS[] (0x64).
// Reference: skrifa hint/engine/arith.rs:83-85
func (e *ttEngine) opAbs() error {
	return e.valueStack.applyUnary(func(n int32) (int32, error) {
		if n < 0 {
			return -n, nil
		}
		return n, nil
	})
}

// opNeg implements NEG[] (0x65).
// Reference: skrifa hint/engine/arith.rs:97-99
func (e *ttEngine) opNeg() error {
	return e.valueStack.applyUnary(func(n int32) (int32, error) {
		return -n, nil
	})
}

// opFloor implements FLOOR[] (0x66).
// Reference: skrifa hint/engine/arith.rs:111-113
func (e *ttEngine) opFloor() error {
	return e.valueStack.applyUnary(func(n int32) (int32, error) {
		return ttFloor26Dot6(n), nil
	})
}

// opCeiling implements CEILING[] (0x67).
// Reference: skrifa hint/engine/arith.rs:124-126
func (e *ttEngine) opCeiling() error {
	return e.valueStack.applyUnary(func(n int32) (int32, error) {
		return ttCeil26Dot6(n), nil
	})
}

// opMax implements MAX[] (0x8B).
// Reference: skrifa hint/engine/arith.rs:137-139
func (e *ttEngine) opMax() error {
	return e.valueStack.applyBinary(func(a, b int32) (int32, error) {
		if a > b {
			return a, nil
		}
		return b, nil
	})
}

// opMin implements MIN[] (0x8C).
// Reference: skrifa hint/engine/arith.rs:151-153
func (e *ttEngine) opMin() error {
	return e.valueStack.applyBinary(func(a, b int32) (int32, error) {
		if a < b {
			return a, nil
		}
		return b, nil
	})
}

// opRound implements ROUND[ab] (0x68-0x6B).
// Reference: skrifa hint/engine/round.rs
func (e *ttEngine) opRound() error {
	rs := e.graphics.roundState
	return e.valueStack.applyUnary(func(n int32) (int32, error) {
		return rs.round(n), nil
	})
}
