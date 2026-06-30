// TrueType bytecode interpreter — call stack.
//
// Port of skrifa hint/call_stack.rs (97 LOC).
// Tracks nested function/instruction definition calls.
//
// Reference: skrifa/src/outline/glyf/hint/call_stack.rs
package text

// ttCallStackMaxDepth is the maximum nesting depth for function calls.
// Matches FreeType's call stack depth of 32.
// Reference: skrifa hint/call_stack.rs:6
const ttCallStackMaxDepth = 32

// ttCallRecord is a record of an active function or instruction invocation.
// Reference: skrifa hint/call_stack.rs:13-19
type ttCallRecord struct {
	callerProgram ttProgramType
	returnPC      int
	currentCount  int32 // remaining loop iterations
	definition    ttDefinition
}

// ttCallStack tracks nested active function or instruction calls.
// Fixed-size array matching FreeType's 32-deep call stack.
// Reference: skrifa hint/call_stack.rs:22-25
type ttCallStack struct {
	records [ttCallStackMaxDepth]ttCallRecord
	top     int
}

// clear resets the call stack.
// Reference: skrifa hint/call_stack.rs:29-31
func (s *ttCallStack) clear() {
	s.top = 0
}

// push adds a call record to the stack.
// Reference: skrifa hint/call_stack.rs:33-40
func (s *ttCallStack) push(record ttCallRecord) error {
	if s.top >= ttCallStackMaxDepth {
		return ttErrCallStackOverflow
	}
	s.records[s.top] = record
	s.top++
	return nil
}

// peek returns the top record without removing it.
// Reference: skrifa hint/call_stack.rs:42-44
func (s *ttCallStack) peek() (ttCallRecord, bool) {
	if s.top <= 0 {
		return ttCallRecord{}, false
	}
	return s.records[s.top-1], true
}

// pop removes and returns the top record.
// Reference: skrifa hint/call_stack.rs:46-50
func (s *ttCallStack) pop() (ttCallRecord, error) {
	r, ok := s.peek()
	if !ok {
		return r, ttErrCallStackUnderflow
	}
	s.top--
	return r, nil
}
