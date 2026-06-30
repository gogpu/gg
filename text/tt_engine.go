// TrueType bytecode interpreter — main engine.
//
// Port of skrifa hint/engine/mod.rs (269 LOC).
// The Engine is a stack-based virtual machine that executes font hinting
// instructions, manipulating glyph outline points to snap them to the
// pixel grid.
//
// Reference: skrifa/src/outline/glyf/hint/engine/mod.rs
package text

import "errors"

// ttMaxRunInstructions is the maximum number of instructions executed
// in a single run. Prevents infinite loops.
// Reference: skrifa hint/engine/dispatch.rs:9
const ttMaxRunInstructions = 1_000_000

// ttEngine is the TrueType bytecode interpreter.
// Reference: skrifa hint/engine/mod.rs:39-49
type ttEngine struct {
	program     ttProgramState
	graphics    ttGraphicsState
	definitions ttDefinitionState
	cvt         []int32 // Control Value Table (26.6 values)
	storage     []int32 // Storage area
	valueStack  ttValueStack
	loopBudget  ttLoopBudget
	axisCount   int
	coords      []int16 // 2.14 normalized variation coords
}

// newTTEngine creates a new interpreter engine.
// Reference: skrifa hint/engine/mod.rs:51-89
//
//nolint:unparam // axisCount, coords kept for skrifa parity; needed for variable fonts (gvar)
func newTTEngine(
	program *ttProgramState,
	retained ttRetainedGraphicsState,
	definitions ttDefinitionState,
	cvt []int32,
	storage []int32,
	valueStack ttValueStack,
	twilight ttZone,
	glyph ttZone,
	axisCount int,
	coords []int16,
	isComposite bool,
	cvtLen int,
) *ttEngine {
	pointCount := 0
	if len(glyph.points) > 0 {
		pointCount = len(glyph.points)
	}

	gs := defaultGraphicsState()
	gs.retained = retained
	gs.zones = [2]ttZone{twilight, glyph}
	gs.isComposite = isComposite
	gs.updateProjectionState()

	return &ttEngine{
		program:     *program,
		graphics:    gs,
		definitions: definitions,
		cvt:         cvt,
		storage:     storage,
		valueStack:  valueStack,
		loopBudget:  newTTLoopBudget(pointCount, cvtLen),
		axisCount:   axisCount,
		coords:      coords,
	}
}

// backwardCompatibility returns whether backward compatibility mode is active.
// Reference: skrifa hint/engine/mod.rs:91-93
func (e *ttEngine) backwardCompatibility() bool {
	return e.graphics.backwardCompatibility
}

// retainedGraphicsState returns the persistent graphics state.
// Reference: skrifa hint/engine/mod.rs:95-97
func (e *ttEngine) retainedGraphicsState() *ttRetainedGraphicsState {
	return &e.graphics.retained
}

// runProgram resets state for the given program and executes all instructions.
// Reference: skrifa hint/engine/dispatch.rs:14-17
//
//nolint:unparam // isPedantic kept for skrifa parity; production uses non-pedantic
func (e *ttEngine) runProgram(program ttProgramType, isPedantic bool) error {
	e.resetForProgram(program, isPedantic)
	return e.run()
}

// resetForProgram sets internal state for running the specified program.
// Reference: skrifa hint/engine/dispatch.rs:20-52
func (e *ttEngine) resetForProgram(program ttProgramType, isPedantic bool) {
	e.program.resetProgram(program)
	e.graphics.reset()
	e.graphics.isPedantic = isPedantic
	e.loopBudget.reset()

	switch program {
	case ttProgramFont:
		e.definitions.functions.reset()
		e.definitions.instructions.reset()
	case ttProgramControlValue:
		e.graphics.backwardCompatibility = false
	case ttProgramGlyph:
		// Instruct control bit 1: reset retained graphics state.
		if e.graphics.retained.instructControl&2 != 0 {
			e.graphics.resetRetained()
		}
		// Set backward compatibility mode.
		switch {
		case e.graphics.retained.target.preserveLinearMetrics():
			e.graphics.backwardCompatibility = true
		case e.graphics.retained.target.isSmooth():
			e.graphics.backwardCompatibility = (e.graphics.retained.instructControl & 0x4) == 0
		default:
			e.graphics.backwardCompatibility = false
		}
	}
}

// run decodes and dispatches all instructions until completion or error.
// Reference: skrifa hint/engine/dispatch.rs:55-72
func (e *ttEngine) run() error {
	count := 0
	for !e.program.decoder.done() {
		opcode, ok := e.program.decoder.nextByte()
		if !ok {
			break
		}
		pc := e.program.decoder.pc - 1

		if err := e.dispatch(opcode); err != nil {
			var kind ttHintErrorKind
			if !errors.As(err, &kind) {
				kind = ttErrUnhandledOpcode
			}
			return &ttHintError{
				program: e.program.current,
				pc:      pc,
				opcode:  int(opcode),
				kind:    kind,
			}
		}

		count++
		if count > ttMaxRunInstructions {
			return &ttHintError{
				program: e.program.current,
				pc:      pc,
				opcode:  int(opcode),
				kind:    ttErrExceededExecutionBudget,
			}
		}
	}
	return nil
}

// zone returns a pointer to the zone selected by the given zone pointer.
func (e *ttEngine) zone(zp ttZonePointer) *ttZone {
	return &e.graphics.zones[zp]
}

// ttLoopBudget tracks execution budgets to limit execution time.
// Reference: skrifa hint/engine/mod.rs:101-153
type ttLoopBudget struct {
	limit         int
	backwardJumps int
	loopCalls     int
}

// newTTLoopBudget computes the execution budget based on point/CVT counts.
// Reference: skrifa hint/engine/mod.rs:112-129
func newTTLoopBudget(pointCount, cvtLen int) ttLoopBudget {
	var limit int
	if pointCount > 0 {
		limit = max(pointCount*10, 50) + max(cvtLen/10, 50)
	} else {
		limit = 300 + 22*cvtLen
	}
	return ttLoopBudget{
		limit:         limit,
		backwardJumps: 0,
		loopCalls:     0,
	}
}

// reset clears the budget counters.
func (lb *ttLoopBudget) reset() {
	lb.backwardJumps = 0
	lb.loopCalls = 0
}

// doingBackwardJump checks and increments the backward jump counter.
func (lb *ttLoopBudget) doingBackwardJump() error {
	lb.backwardJumps++
	if lb.backwardJumps > lb.limit {
		return ttErrExceededExecutionBudget
	}
	return nil
}

// doingLoopCall checks and increments the loop call counter.
func (lb *ttLoopBudget) doingLoopCall(count int) error {
	lb.loopCalls += count
	if lb.loopCalls > lb.limit {
		return ttErrExceededExecutionBudget
	}
	return nil
}
