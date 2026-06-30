// TrueType bytecode interpreter — value stack.
//
// Port of skrifa hint/value_stack.rs (388 LOC).
// Manages the TrueType interpreter's operand stack.
//
// Reference: skrifa/src/outline/glyf/hint/value_stack.rs
package text

// ttValueStack is the operand stack for the TrueType interpreter.
//
// Uses a pre-allocated slice as backing store. Stack grows upward.
// In non-pedantic mode, underflow returns 0 instead of error.
//
// Reference: skrifa hint/value_stack.rs:16-28
type ttValueStack struct {
	values   []int32
	top      int
	pedantic bool
}

// newTTValueStack creates a value stack with the given capacity.
// Reference: skrifa hint/value_stack.rs:23-29
func newTTValueStack(capacity int, pedantic bool) ttValueStack {
	return ttValueStack{
		values:   make([]int32, capacity),
		top:      0,
		pedantic: pedantic,
	}
}

// len returns the current depth of the stack.
// Reference: skrifa hint/value_stack.rs:33
func (s *ttValueStack) len() int {
	return s.top
}

// activeValues returns the active portion of the stack (for testing).
func (s *ttValueStack) activeValues() []int32 {
	return s.values[:s.top]
}

// push pushes a value onto the stack.
// Reference: skrifa hint/value_stack.rs:48-56
func (s *ttValueStack) push(value int32) error {
	if s.top >= len(s.values) {
		return ttErrValueStackOverflow
	}
	s.values[s.top] = value
	s.top++
	return nil
}

// pushN pushes multiple values from a byte slice as unsigned bytes.
// Used for PUSHB/NPUSHB instructions.
// Reference: skrifa hint/value_stack.rs:64-78
func (s *ttValueStack) pushBytes(data []byte) error {
	count := len(data)
	if s.top+count > len(s.values) {
		return ttErrValueStackOverflow
	}
	for i, b := range data {
		s.values[s.top+i] = int32(b)
	}
	s.top += count
	return nil
}

// pushWords pushes multiple 16-bit signed words onto the stack.
// Used for PUSHW/NPUSHW instructions.
// Reference: skrifa hint/value_stack.rs:64-78
func (s *ttValueStack) pushWords(data []int16) error {
	count := len(data)
	if s.top+count > len(s.values) {
		return ttErrValueStackOverflow
	}
	for i, w := range data {
		s.values[s.top+i] = int32(w)
	}
	s.top += count
	return nil
}

// peek returns the top value without removing it.
// Returns 0 if stack is empty.
// Reference: skrifa hint/value_stack.rs:80-86
func (s *ttValueStack) peek() (int32, bool) {
	if s.top > 0 {
		return s.values[s.top-1], true
	}
	return 0, false
}

// pop removes and returns the top value.
// In non-pedantic mode, underflow returns 0.
// Reference: skrifa hint/value_stack.rs:93-102
func (s *ttValueStack) pop() (int32, error) {
	if s.top > 0 {
		s.top--
		return s.values[s.top], nil
	}
	if s.pedantic {
		return 0, ttErrValueStackUnderflow
	}
	return 0, nil
}

// popN pops n values, returning only the last error (if any).
// Used for instructions that discard values.
func (s *ttValueStack) popN(n int) error {
	if s.top >= n {
		s.top -= n
		return nil
	}
	if s.pedantic {
		return ttErrValueStackUnderflow
	}
	s.top = 0
	return nil
}

// popUsize pops a value intended as a usize index.
// Reference: skrifa hint/value_stack.rs:111-113
func (s *ttValueStack) popUsize() (int, error) {
	v, err := s.pop()
	return int(v), err
}

// popCountChecked pops a value intended as a count.
// Negative values return error in pedantic mode, 0 otherwise.
// Reference: skrifa hint/value_stack.rs:119-126
func (s *ttValueStack) popCountChecked() (int, error) {
	v, err := s.pop()
	if err != nil {
		return 0, err
	}
	if v < 0 {
		if s.pedantic {
			return 0, ttErrInvalidStackValue
		}
		return 0, nil
	}
	return int(v), nil
}

// applyUnary pops one value, applies op, and pushes the result.
// Reference: skrifa hint/value_stack.rs:131-137
func (s *ttValueStack) applyUnary(op func(int32) (int32, error)) error {
	a, err := s.pop()
	if err != nil {
		return err
	}
	result, err := op(a)
	if err != nil {
		return err
	}
	return s.push(result)
}

// applyBinary pops b then a, applies op(a,b), pushes result.
// Reference: skrifa hint/value_stack.rs:142-149
func (s *ttValueStack) applyBinary(op func(int32, int32) (int32, error)) error {
	b, err := s.pop()
	if err != nil {
		return err
	}
	a, err := s.pop()
	if err != nil {
		return err
	}
	result, err := op(a, b)
	if err != nil {
		return err
	}
	return s.push(result)
}

// clear empties the stack.
// Reference: skrifa hint/value_stack.rs:157
func (s *ttValueStack) clear() {
	s.top = 0
}

// dup duplicates the top element.
// Reference: skrifa hint/value_stack.rs:165-173
func (s *ttValueStack) dup() error {
	v, ok := s.peek()
	if !ok {
		if s.pedantic {
			return ttErrValueStackUnderflow
		}
		return s.push(0)
	}
	return s.push(v)
}

// swap swaps the top two elements.
// Reference: skrifa hint/value_stack.rs:180-185
func (s *ttValueStack) swap() error {
	a, err := s.pop()
	if err != nil {
		return err
	}
	b, err := s.pop()
	if err != nil {
		return err
	}
	if err := s.push(a); err != nil {
		return err
	}
	return s.push(b)
}

// copyIndex implements CINDEX: copy the indexed element to top.
// Top of stack contains the 1-based index.
// Reference: skrifa hint/value_stack.rs:192-198
func (s *ttValueStack) copyIndex() error {
	if s.top < 1 {
		return ttErrValueStackUnderflow
	}
	topIx := s.top - 1
	index := int(s.values[topIx])
	elementIx := topIx - index
	if elementIx < 0 || elementIx >= s.top {
		return ttErrValueStackUnderflow
	}
	s.values[topIx] = s.values[elementIx]
	return nil
}

// moveIndex implements MINDEX: move the indexed element to top.
// Top of stack contains the 1-based index. Removes from original position.
// Reference: skrifa hint/value_stack.rs:205-214
func (s *ttValueStack) moveIndex() error {
	if s.top < 1 {
		return ttErrValueStackUnderflow
	}
	topIx := s.top - 1
	index := int(s.values[topIx])
	elementIx := topIx - index
	if elementIx < 0 || elementIx >= s.top {
		return ttErrValueStackUnderflow
	}
	if topIx < 1 {
		return ttErrValueStackUnderflow
	}
	newTopIx := topIx - 1
	value := s.values[elementIx]
	copy(s.values[elementIx:], s.values[elementIx+1:s.top])
	s.values[newTopIx] = value
	s.top--
	return nil
}

// roll rotates the top three elements: a,b,c -> b,c,a.
// Reference: skrifa hint/value_stack.rs:223-231
func (s *ttValueStack) roll() error {
	a, err := s.pop()
	if err != nil {
		return err
	}
	b, err := s.pop()
	if err != nil {
		return err
	}
	c, err := s.pop()
	if err != nil {
		return err
	}
	if err := s.push(b); err != nil {
		return err
	}
	if err := s.push(a); err != nil {
		return err
	}
	return s.push(c)
}
