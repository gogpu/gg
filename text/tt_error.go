// TrueType bytecode interpreter — error definitions.
//
// Port of skrifa hint/error.rs.
// Reference: skrifa/src/outline/glyf/hint/error.rs
package text

import "fmt"

// ttHintErrorKind describes the category of error encountered during
// TrueType bytecode interpretation.
//
// Matches skrifa HintErrorKind.
// Reference: skrifa hint/error.rs:10-33
type ttHintErrorKind int

const (
	ttErrUnexpectedEndOfBytecode ttHintErrorKind = iota
	ttErrUnhandledOpcode
	ttErrDefinitionInGlyphProgram
	ttErrNestedDefinition
	ttErrDefinitionTooLarge
	ttErrTooManyDefinitions
	ttErrInvalidDefinition
	ttErrValueStackOverflow
	ttErrValueStackUnderflow
	ttErrCallStackOverflow
	ttErrCallStackUnderflow
	ttErrInvalidStackValue
	ttErrInvalidPointIndex
	ttErrInvalidPointRange
	ttErrInvalidContourIndex
	ttErrInvalidCvtIndex
	ttErrInvalidStorageIndex
	ttErrDivideByZero
	ttErrInvalidZoneIndex
	ttErrNegativeLoopCounter
	ttErrInvalidJump
	ttErrExceededExecutionBudget
)

// Error implements the error interface for ttHintErrorKind.
func (k ttHintErrorKind) Error() string {
	switch k {
	case ttErrUnexpectedEndOfBytecode:
		return "tt: unexpected end of bytecode"
	case ttErrUnhandledOpcode:
		return "tt: unhandled opcode"
	case ttErrDefinitionInGlyphProgram:
		return "tt: function or instruction definition in glyph program"
	case ttErrNestedDefinition:
		return "tt: nested function or instruction definition"
	case ttErrDefinitionTooLarge:
		return "tt: definition exceeded maximum size of 64k"
	case ttErrTooManyDefinitions:
		return "tt: too many definitions"
	case ttErrInvalidDefinition:
		return "tt: invalid definition"
	case ttErrValueStackOverflow:
		return "tt: value stack overflow"
	case ttErrValueStackUnderflow:
		return "tt: value stack underflow"
	case ttErrCallStackOverflow:
		return "tt: call stack overflow"
	case ttErrCallStackUnderflow:
		return "tt: call stack underflow"
	case ttErrInvalidStackValue:
		return "tt: invalid stack value"
	case ttErrInvalidPointIndex:
		return "tt: point index out of bounds"
	case ttErrInvalidPointRange:
		return "tt: point range out of bounds"
	case ttErrInvalidContourIndex:
		return "tt: contour index out of bounds"
	case ttErrInvalidCvtIndex:
		return "tt: cvt index out of bounds"
	case ttErrInvalidStorageIndex:
		return "tt: storage area index out of bounds"
	case ttErrDivideByZero:
		return "tt: divide by zero"
	case ttErrInvalidZoneIndex:
		return "tt: invalid zone index (only 0 or 1 permitted)"
	case ttErrNegativeLoopCounter:
		return "tt: negative loop counter"
	case ttErrInvalidJump:
		return "tt: invalid jump target"
	case ttErrExceededExecutionBudget:
		return "tt: exceeded execution budget"
	default:
		return "tt: unknown error"
	}
}

// ttHintError is a hinting error with additional context about where
// the error occurred.
//
// Matches skrifa HintError.
// Reference: skrifa hint/error.rs:94-122
type ttHintError struct {
	program ttProgramType
	pc      int
	opcode  int // -1 if unknown
	kind    ttHintErrorKind
	detail  int // extra info (e.g. opcode value, index)
}

// Error implements the error interface.
func (e *ttHintError) Error() string {
	progName := "unknown"
	switch e.program {
	case ttProgramFont:
		progName = "fpgm"
	case ttProgramControlValue:
		progName = "prep"
	case ttProgramGlyph:
		progName = "glyf"
	}
	if e.opcode >= 0 {
		return fmt.Sprintf("tt: %s@%d:op=0x%02X: %s", progName, e.pc, e.opcode, e.kind)
	}
	return fmt.Sprintf("tt: %s@%d: %s", progName, e.pc, e.kind)
}

// Unwrap returns the underlying error kind for errors.Is/As.
func (e *ttHintError) Unwrap() error {
	return e.kind
}
