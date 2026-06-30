// TrueType bytecode interpreter — engine tests.
//
// Smoke tests for Phase A: validates core engine infrastructure,
// stack operations, program execution, and key opcodes.
//
// Reference: skrifa hint/engine/ test suite
package text

import (
	"errors"
	"testing"
)

// newTestEngine creates a minimal engine for testing.
// Matches skrifa's MockEngine pattern.
func newTestEngine() *ttEngine {
	retained := newTTRetainedGraphicsState(1<<16, 16, ttTargetSmooth)
	program := newTTProgramState(nil, nil, nil, ttProgramFont)
	defs := ttDefinitionState{
		functions:    newTTDefinitionMap(5),
		instructions: newTTDefinitionMap(5),
	}
	cvt := make([]int32, 10)
	storage := make([]int32, 10)
	stack := newTTValueStack(256, true)
	twilight := ttZone{
		unscaled: make([]int32, 8),
		original: make([][2]int32, 4),
		points:   make([][2]int32, 4),
		flags:    make([]ttPointFlags, 4),
		contours: []uint16{3},
	}
	glyph := ttZone{
		unscaled: make([]int32, 8),
		original: make([][2]int32, 4),
		points:   make([][2]int32, 4),
		flags:    make([]ttPointFlags, 4),
		contours: []uint16{3},
	}
	return newTTEngine(
		&program, retained, defs, cvt, storage, stack,
		twilight, glyph, 0, nil, false, len(cvt),
	)
}

// setFontCode sets the font program bytecode and resets the decoder.
func (e *ttEngine) setFontCode(code []byte) {
	e.program.bytecode[0] = code
	e.program.decoder = newTTDecoder(code)
	e.program.current = ttProgramFont
	e.program.initial = ttProgramFont
}

func TestValueStack_PushPop(t *testing.T) {
	s := newTTValueStack(8, true)
	tests := []struct {
		name    string
		pushVal int32
	}{
		{"zero", 0},
		{"positive", 42},
		{"negative", -100},
		{"max_int32", 0x7FFFFFFF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.clear()
			if err := s.push(tt.pushVal); err != nil {
				t.Fatalf("push: %v", err)
			}
			if s.len() != 1 {
				t.Errorf("len = %d, want 1", s.len())
			}
			got, err := s.pop()
			if err != nil {
				t.Fatalf("pop: %v", err)
			}
			if got != tt.pushVal {
				t.Errorf("pop = %d, want %d", got, tt.pushVal)
			}
		})
	}
}

func TestValueStack_Overflow(t *testing.T) {
	s := newTTValueStack(2, true)
	_ = s.push(1)
	_ = s.push(2)
	err := s.push(3)
	if !errors.Is(err, ttErrValueStackOverflow) {
		t.Errorf("expected overflow, got %v", err)
	}
}

func TestValueStack_Underflow(t *testing.T) {
	s := newTTValueStack(8, true)
	_, err := s.pop()
	if !errors.Is(err, ttErrValueStackUnderflow) {
		t.Errorf("expected underflow, got %v", err)
	}
}

func TestValueStack_NonPedanticUnderflow(t *testing.T) {
	s := newTTValueStack(8, false)
	v, err := s.pop()
	if err != nil {
		t.Fatalf("non-pedantic pop should not error: %v", err)
	}
	if v != 0 {
		t.Errorf("non-pedantic pop = %d, want 0", v)
	}
}

func TestValueStack_DupSwapRoll(t *testing.T) {
	s := newTTValueStack(16, true)
	// DUP
	_ = s.push(5)
	_ = s.dup()
	v, _ := s.pop()
	if v != 5 {
		t.Errorf("dup: got %d, want 5", v)
	}
	// SWAP
	s.clear()
	_ = s.push(10)
	_ = s.push(20)
	_ = s.swap()
	a, _ := s.pop()
	b, _ := s.pop()
	if a != 10 || b != 20 {
		t.Errorf("swap: got %d,%d want 10,20", a, b)
	}
	// ROLL
	s.clear()
	_ = s.push(1)
	_ = s.push(2)
	_ = s.push(3)
	_ = s.roll()
	c, _ := s.pop()
	d, _ := s.pop()
	f, _ := s.pop()
	if c != 1 || d != 3 || f != 2 {
		t.Errorf("roll: got %d,%d,%d want 1,3,2", c, d, f)
	}
}

func TestValueStack_CopyMoveIndex(t *testing.T) {
	s := newTTValueStack(16, true)
	_ = s.push(100) // index 0
	_ = s.push(200) // index 1
	_ = s.push(300) // index 2
	_ = s.push(2)   // CINDEX: copy element 2 positions down

	if err := s.copyIndex(); err != nil {
		t.Fatalf("copyIndex: %v", err)
	}
	v, _ := s.pop()
	if v != 200 {
		t.Errorf("copyIndex = %d, want 200", v)
	}
}

func TestCallStack_PushPop(t *testing.T) {
	cs := ttCallStack{}
	record := ttCallRecord{
		callerProgram: ttProgramFont,
		returnPC:      42,
		currentCount:  1,
	}
	if err := cs.push(record); err != nil {
		t.Fatalf("push: %v", err)
	}
	got, err := cs.pop()
	if err != nil {
		t.Fatalf("pop: %v", err)
	}
	if got.returnPC != 42 {
		t.Errorf("returnPC = %d, want 42", got.returnPC)
	}
}

func TestCallStack_Overflow(t *testing.T) {
	cs := ttCallStack{}
	for i := 0; i < ttCallStackMaxDepth; i++ {
		if err := cs.push(ttCallRecord{}); err != nil {
			t.Fatalf("push %d: %v", i, err)
		}
	}
	err := cs.push(ttCallRecord{})
	if !errors.Is(err, ttErrCallStackOverflow) {
		t.Errorf("expected overflow, got %v", err)
	}
}

func TestRoundState_AllModes(t *testing.T) {
	tests := []struct {
		mode     ttRoundMode
		input    int32
		expected int32
	}{
		{ttRoundGrid, 96, 128},      // 1.5 px -> 2 px (round half up)
		{ttRoundGrid, 32, 64},       // 0.5 px -> 1 px (round half up)
		{ttRoundGrid, 33, 64},       // 0.515 px -> 1 px
		{ttRoundHalfGrid, 96, 96},   // 1.5 px -> 1.5 px
		{ttRoundHalfGrid, 64, 96},   // 1.0 px -> 1.5 px
		{ttRoundDoubleGrid, 48, 64}, // 0.75 px -> 1.0 px (nearest half-pixel)
		{ttRoundDownToGrid, 96, 64}, // 1.5 px -> 1 px
		{ttRoundUpToGrid, 65, 128},  // 1.015 px -> 2 px
		{ttRoundOff, 42, 42},        // No rounding
	}
	for _, tt := range tests {
		rs := ttRoundState{mode: tt.mode, period: 64}
		got := rs.round(tt.input)
		if got != tt.expected {
			t.Errorf("round(%d) mode=%d: got %d, want %d", tt.input, tt.mode, got, tt.expected)
		}
	}
}

func TestMath_Floor26Dot6(t *testing.T) {
	tests := []struct {
		input int32
		want  int32
	}{
		{0, 0},
		{64, 64},    // 1.0
		{96, 64},    // 1.5 -> 1.0
		{-96, -128}, // -1.5 -> -2.0
		{127, 64},   // 1.984 -> 1.0
		{-1, -64},   // -0.015 -> -1.0
	}
	for _, tt := range tests {
		got := ttFloor26Dot6(tt.input)
		if got != tt.want {
			t.Errorf("floor(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestMath_MulDiv(t *testing.T) {
	// 100 * 200 / 10 = 2000
	got := ttMulDiv(100, 200, 10)
	if got != 2000 {
		t.Errorf("mulDiv(100,200,10) = %d, want 2000", got)
	}
	// Division by zero -> max value
	got = ttMulDiv(1, 1, 0)
	if got != 0x7FFFFFFF {
		t.Errorf("mulDiv(1,1,0) = %d, want maxint", got)
	}
}

func TestMath_Mul14(t *testing.T) {
	// 1.0 * 1.0 in 2.14 (0x4000) = 1.0
	got := ttMul14(64, 0x4000) // 1px in 26.6 * 1.0 in 2.14
	if got != 64 {
		t.Errorf("mul14(64, 0x4000) = %d, want 64", got)
	}
}

func TestMath_Normalize14(t *testing.T) {
	// Normalize (1, 0) -> should be close to (0x4000, 0)
	x, y := ttNormalize14(100, 0)
	if x <= 0 || y != 0 {
		t.Errorf("normalize14(100, 0) = (%d, %d), want (positive, 0)", x, y)
	}
}

func TestMath_Dot14(t *testing.T) {
	// Dot product of (1,0) . (1,0) in 2.14 = 1.0 (0x4000)
	got := ttDot14(0x4000, 0, 0x4000, 0)
	if got != 0x4000 {
		t.Errorf("dot14 = %d, want 0x4000", got)
	}
}

func TestDecoder_NextByteWord(t *testing.T) {
	data := []byte{0x42, 0x01, 0x00}
	d := newTTDecoder(data)
	b, ok := d.nextByte()
	if !ok || b != 0x42 {
		t.Errorf("nextByte = %d, %v", b, ok)
	}
	w, ok := d.nextWord()
	if !ok || w != 256 { // 0x0100 = 256
		t.Errorf("nextWord = %d, %v", w, ok)
	}
	if !d.done() {
		t.Error("expected done after consuming all bytes")
	}
}

func TestZone_PointAccess(t *testing.T) {
	z := ttZone{
		original: [][2]int32{{10, 20}, {30, 40}},
		points:   [][2]int32{{10, 20}, {30, 40}},
		flags:    []ttPointFlags{0, 0},
		contours: []uint16{1},
	}
	pt, err := z.point(0)
	if err != nil {
		t.Fatalf("point(0): %v", err)
	}
	if pt != [2]int32{10, 20} {
		t.Errorf("point(0) = %v, want [10,20]", pt)
	}
	// Out of bounds
	_, err = z.point(5)
	if !errors.Is(err, ttErrInvalidPointIndex) {
		t.Errorf("expected invalid point index, got %v", err)
	}
	// Touch
	z.touchX(0)
	if !z.isTouchedX(0) {
		t.Error("expected touchedX after touchX")
	}
	z.untouch(0)
	if z.isTouchedX(0) {
		t.Error("expected untouched after untouch")
	}
	// On-curve
	z.setOnCurve(0, true)
	if !z.isOnCurve(0) {
		t.Error("expected on-curve")
	}
	z.flipOnCurve(0)
	if z.isOnCurve(0) {
		t.Error("expected off-curve after flip")
	}
	// Contour end
	end, err := z.contourEnd(0)
	if err != nil || end != 1 {
		t.Errorf("contourEnd(0) = %d, %v", end, err)
	}
}

func TestZone_MovePoint(t *testing.T) {
	t.Run("X axis no backward compat", func(t *testing.T) {
		z := ttZone{
			points: [][2]int32{{100, 200}},
			flags:  []ttPointFlags{0},
		}
		gs := defaultGraphicsState()
		gs.backwardCompatibility = false
		gs.freedomVector = [2]int32{0x4000, 0}
		gs.updateProjectionState()
		if err := z.movePoint(&gs, 0, 64); err != nil {
			t.Fatalf("movePoint: %v", err)
		}
		if z.points[0][0] != 164 {
			t.Errorf("x = %d, want 164", z.points[0][0])
		}
		if !z.isTouchedX(0) {
			t.Error("expected X-touched after movePoint")
		}
	})
	t.Run("X axis backward compat suppresses move", func(t *testing.T) {
		z := ttZone{
			points: [][2]int32{{100, 200}},
			flags:  []ttPointFlags{0},
		}
		gs := defaultGraphicsState()
		gs.backwardCompatibility = true
		gs.freedomVector = [2]int32{0x4000, 0}
		gs.updateProjectionState()
		if err := z.movePoint(&gs, 0, 64); err != nil {
			t.Fatalf("movePoint: %v", err)
		}
		// In backward compat mode, X movement is suppressed but point is touched.
		if z.points[0][0] != 100 {
			t.Errorf("x = %d, want 100 (suppressed)", z.points[0][0])
		}
		if !z.isTouchedX(0) {
			t.Error("expected X-touched even when movement suppressed")
		}
	})
	t.Run("Y axis backward compat allows move", func(t *testing.T) {
		z := ttZone{
			points: [][2]int32{{100, 200}},
			flags:  []ttPointFlags{0},
		}
		gs := defaultGraphicsState()
		gs.backwardCompatibility = true
		gs.freedomVector = [2]int32{0, 0x4000}
		gs.updateProjectionState()
		if err := z.movePoint(&gs, 0, 64); err != nil {
			t.Fatalf("movePoint: %v", err)
		}
		// Y movement proceeds normally in backward compat (until IUP done).
		if z.points[0][1] != 264 {
			t.Errorf("y = %d, want 264", z.points[0][1])
		}
		if !z.isTouchedY(0) {
			t.Error("expected Y-touched after movePoint")
		}
	})
	t.Run("Y axis backward compat after IUP suppresses", func(t *testing.T) {
		z := ttZone{
			points: [][2]int32{{100, 200}},
			flags:  []ttPointFlags{0},
		}
		gs := defaultGraphicsState()
		gs.backwardCompatibility = true
		gs.didIUPx = true
		gs.didIUPy = true
		gs.freedomVector = [2]int32{0, 0x4000}
		gs.updateProjectionState()
		if err := z.movePoint(&gs, 0, 64); err != nil {
			t.Fatalf("movePoint: %v", err)
		}
		// After IUP done on both axes, Y movement also suppressed.
		if z.points[0][1] != 200 {
			t.Errorf("y = %d, want 200 (suppressed after IUP)", z.points[0][1])
		}
		if !z.isTouchedY(0) {
			t.Error("expected Y-touched even when movement suppressed")
		}
	})
}

func TestGraphicsState_Project(t *testing.T) {
	gs := defaultGraphicsState()
	// Default proj vector is X axis
	d := gs.project(200, 100, 100, 50)
	if d != 100 { // X difference
		t.Errorf("project(X axis) = %d, want 100", d)
	}
	// Set to Y axis
	gs.projVector = [2]int32{0, 0x4000}
	gs.updateProjectionState()
	d = gs.project(200, 100, 100, 50)
	if d != 50 { // Y difference
		t.Errorf("project(Y axis) = %d, want 50", d)
	}
}

func TestEngine_PushBPopProgram(t *testing.T) {
	e := newTestEngine()
	// PUSHB[0] 42, POP
	code := []byte{opPUSHB000, 42, opPOP}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if e.valueStack.len() != 0 {
		t.Errorf("stack len = %d, want 0", e.valueStack.len())
	}
}

func TestEngine_PushBDup(t *testing.T) {
	e := newTestEngine()
	// PUSHB[0] 10, DUP -> stack: [10, 10]
	code := []byte{opPUSHB000, 10, opDUP}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if e.valueStack.len() != 2 {
		t.Errorf("stack len = %d, want 2", e.valueStack.len())
	}
	v, _ := e.valueStack.pop()
	if v != 10 {
		t.Errorf("dup result = %d, want 10", v)
	}
}

func TestEngine_Arithmetic(t *testing.T) {
	e := newTestEngine()
	// PUSHB[1] 100 50, ADD -> 150
	code := []byte{opPUSHB000 + 1, 100, 50, opADD}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 150 {
		t.Errorf("100+50 = %d, want 150", v)
	}
}

func TestEngine_Comparison(t *testing.T) {
	e := newTestEngine()
	// PUSHB[1] 10 20, LT -> 1 (10 < 20)
	code := []byte{opPUSHB000 + 1, 10, 20, opLT}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 1 {
		t.Errorf("10 < 20 = %d, want 1", v)
	}
}

func TestEngine_IfElse(t *testing.T) {
	e := newTestEngine()
	// PUSHB[0] 0, IF, PUSHB[0] 99, ELSE, PUSHB[0] 42, EIF
	// Condition is false -> should skip to ELSE and push 42
	code := []byte{
		opPUSHB000, 0, // push false
		opIF,
		opPUSHB000, 99, // true branch (skipped)
		opELSE,
		opPUSHB000, 42, // false branch
		opEIF,
	}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 42 {
		t.Errorf("IF false branch = %d, want 42", v)
	}
}

func TestEngine_IfTrue(t *testing.T) {
	e := newTestEngine()
	// PUSHB[0] 1, IF, PUSHB[0] 99, ELSE, PUSHB[0] 42, EIF
	// Condition is true -> should push 99, then skip ELSE
	code := []byte{
		opPUSHB000, 1, // push true
		opIF,
		opPUSHB000, 99, // true branch
		opELSE,
		opPUSHB000, 42, // false branch (skipped)
		opEIF,
	}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 99 {
		t.Errorf("IF true branch = %d, want 99", v)
	}
}

func TestEngine_FdefCall(t *testing.T) {
	e := newTestEngine()
	// FDEF 0: ADD 2 to top of stack. Then call it with value 10.
	code := []byte{
		opPUSHB000, 0, // function key = 0
		opFDEF,
		opPUSHB000, 2, // push 2
		opADD, // add
		opENDF,
	}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run (define): %v", err)
	}
	// Now call function 0 with 10 on stack
	if err := e.valueStack.push(10); err != nil {
		t.Fatalf("push: %v", err)
	}
	if err := e.valueStack.push(0); err != nil { // function key
		t.Fatalf("push: %v", err)
	}
	if err := e.opCall(); err != nil {
		t.Fatalf("opCall: %v", err)
	}
	if err := e.run(); err != nil {
		t.Fatalf("run (call): %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 12 {
		t.Errorf("FDEF call = %d, want 12", v)
	}
}

func TestEngine_CVT(t *testing.T) {
	e := newTestEngine()
	e.cvt[3] = 128 // 2 pixels in 26.6
	// RCVT 3 -> should push 128
	code := []byte{opPUSHB000, 3, opRCVT}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 128 {
		t.Errorf("RCVT = %d, want 128", v)
	}
}

func TestEngine_Storage(t *testing.T) {
	e := newTestEngine()
	// WS: store 42 at index 1
	// RS: read from index 1
	code := []byte{
		opPUSHB000 + 1, 1, 42, // push index=1, value=42
		opWS,
		opPUSHB000, 1, // push index=1
		opRS,
	}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 42 {
		t.Errorf("RS = %d, want 42", v)
	}
}

func TestEngine_VectorSetting(t *testing.T) {
	e := newTestEngine()
	// SVTCA[0] -> Y axis
	code := []byte{opSVTCA0}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if e.graphics.projVector != [2]int32{0, 0x4000} {
		t.Errorf("projVector = %v, want Y axis", e.graphics.projVector)
	}
	// SVTCA[1] -> X axis
	code = []byte{opSVTCA1}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if e.graphics.projVector != [2]int32{0x4000, 0} {
		t.Errorf("projVector = %v, want X axis", e.graphics.projVector)
	}
}

func TestEngine_RoundModes(t *testing.T) {
	e := newTestEngine()
	// RTG, RTHG, RTDG, RDTG, RUTG, ROFF
	tests := []struct {
		opcode byte
		mode   ttRoundMode
	}{
		{opRTG, ttRoundGrid},
		{opRTHG, ttRoundHalfGrid},
		{opRTDG, ttRoundDoubleGrid},
		{opRDTG, ttRoundDownToGrid},
		{opRUTG, ttRoundUpToGrid},
		{opROFF, ttRoundOff},
	}
	for _, tt := range tests {
		code := []byte{tt.opcode}
		e.setFontCode(code)
		if err := e.run(); err != nil {
			t.Fatalf("run opcode 0x%02X: %v", tt.opcode, err)
		}
		if e.graphics.roundState.mode != tt.mode {
			t.Errorf("opcode 0x%02X: mode = %d, want %d", tt.opcode, e.graphics.roundState.mode, tt.mode)
		}
	}
}

func TestEngine_MPPEM(t *testing.T) {
	e := newTestEngine()
	code := []byte{opMPPEM}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 16 { // ppem set in newTestEngine
		t.Errorf("MPPEM = %d, want 16", v)
	}
}

func TestEngine_GETINFO(t *testing.T) {
	e := newTestEngine()
	// Selector bit 0 = get version
	code := []byte{opPUSHB000, 1, opGETINFO}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 40 { // version 40 = ClearType compatible
		t.Errorf("GETINFO(1) = %d, want 40", v)
	}
}

func TestEngine_GETDATA(t *testing.T) {
	e := newTestEngine()
	code := []byte{opGETDATA}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 17 { // magic number
		t.Errorf("GETDATA = %d, want 17", v)
	}
}

func TestEngine_LogicalOps(t *testing.T) {
	tests := []struct {
		name   string
		code   []byte
		expect int32
	}{
		{"AND(1,1)", []byte{opPUSHB000 + 1, 1, 1, opAND}, 1},
		{"AND(1,0)", []byte{opPUSHB000 + 1, 1, 0, opAND}, 0},
		{"OR(0,1)", []byte{opPUSHB000 + 1, 0, 1, opOR}, 1},
		{"OR(0,0)", []byte{opPUSHB000 + 1, 0, 0, opOR}, 0},
		{"NOT(0)", []byte{opPUSHB000, 0, opNOT}, 1},
		{"NOT(1)", []byte{opPUSHB000, 1, opNOT}, 0},
		{"EQ(5,5)", []byte{opPUSHB000 + 1, 5, 5, opEQ}, 1},
		{"NEQ(5,3)", []byte{opPUSHB000 + 1, 5, 3, opNEQ}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine()
			e.setFontCode(tt.code)
			if err := e.run(); err != nil {
				t.Fatalf("run: %v", err)
			}
			v, _ := e.valueStack.pop()
			if v != tt.expect {
				t.Errorf("got %d, want %d", v, tt.expect)
			}
		})
	}
}

func TestEngine_MaxInstructions(t *testing.T) {
	// Verify the loop budget mechanism works.
	lb := newTTLoopBudget(10, 5)
	// Also verify the engine creates a proper budget.
	e := newTestEngine()
	if e.loopBudget.limit <= 0 {
		t.Fatalf("engine loop budget limit = %d, want >0", e.loopBudget.limit)
	}
	for i := 0; i < lb.limit; i++ {
		if err := lb.doingBackwardJump(); err != nil {
			t.Fatalf("backward jump %d: %v", i, err)
		}
	}
	if err := lb.doingBackwardJump(); err == nil {
		t.Error("expected budget exceeded error")
	}
}

func TestEngine_ExceededExecutionBudget(t *testing.T) {
	// Verify that ttErrExceededExecutionBudget is a proper error.
	var err error = ttErrExceededExecutionBudget
	if err.Error() != "tt: exceeded execution budget" {
		t.Errorf("error message = %q", err.Error())
	}
}

func TestHintError_Format(t *testing.T) {
	he := &ttHintError{
		program: ttProgramFont,
		pc:      42,
		opcode:  0x2C,
		kind:    ttErrNestedDefinition,
	}
	s := he.Error()
	if s != "tt: fpgm@42:op=0x2C: tt: nested function or instruction definition" {
		t.Errorf("error = %q", s)
	}
	var kind ttHintErrorKind
	if !errors.As(he, &kind) {
		t.Error("expected errors.As to work with ttHintErrorKind")
	}
}

func TestDefinitionMap_AllocateGet(t *testing.T) {
	dm := newTTDefinitionMap(8)
	idx, err := dm.allocate(2)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	dm.defs[idx].start = 10
	dm.defs[idx].end = 20
	dm.defs[idx].isActive = true

	def, err := dm.get(2)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if def.start != 10 || def.end != 20 {
		t.Errorf("def = %+v", def)
	}
	// Reset
	dm.reset()
	_, err = dm.get(2)
	if err == nil {
		t.Error("expected error after reset")
	}
}

func TestEngine_BackwardCompatibility(t *testing.T) {
	e := newTestEngine()
	// Default state has backwardCompatibility=true (ClearType compat).
	if !e.backwardCompatibility() {
		t.Error("backward compatibility should be true by default")
	}
	// retainedGraphicsState accessor
	rgs := e.retainedGraphicsState()
	if rgs == nil {
		t.Fatal("retainedGraphicsState returned nil")
	}
}

func TestZone_UnscaledPoint(t *testing.T) {
	z := ttZone{
		unscaled: []int32{100, 200, 300, 400},
	}
	x, y := z.unscaledPoint(0)
	if x != 100 || y != 200 {
		t.Errorf("unscaled(0) = %d,%d want 100,200", x, y)
	}
	x, y = z.unscaledPoint(5) // out of range
	if x != 0 || y != 0 {
		t.Errorf("unscaled(5) = %d,%d want 0,0", x, y)
	}
}

func TestEngine_NPushW(t *testing.T) {
	e := newTestEngine()
	// NPUSHW: push 2 words: 0x0100 (256) and 0xFF00 (-256)
	code := []byte{
		opNPUSHW, 2,
		0x01, 0x00, // +256
		0xFF, 0x00, // -256 (signed)
	}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	v2, _ := e.valueStack.pop()
	v1, _ := e.valueStack.pop()
	if v1 != 256 || v2 != -256 {
		t.Errorf("NPUSHW: got %d, %d, want 256, -256", v1, v2)
	}
}

func TestRetainedGraphicsState_Defaults(t *testing.T) {
	s := defaultRetainedGraphicsState()
	if !s.autoFlip {
		t.Error("autoFlip should default to true")
	}
	if s.controlValueCutin != 68 {
		t.Errorf("controlValueCutin = %d, want 68", s.controlValueCutin)
	}
	if s.deltaBase != 9 {
		t.Errorf("deltaBase = %d, want 9", s.deltaBase)
	}
	if s.deltaShift != 3 {
		t.Errorf("deltaShift = %d, want 3", s.deltaShift)
	}
	if s.minDistance != 64 {
		t.Errorf("minDistance = %d, want 64", s.minDistance)
	}
}

func TestEngine_ScanCtrl(t *testing.T) {
	e := newTestEngine()
	code := []byte{opPUSHB000, 1, opSCANCTRL}
	e.setFontCode(code)
	if err := e.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !e.graphics.retained.scanControl {
		t.Error("scanControl should be true")
	}
}

// TestEngine_RunProgram tests the full program execution lifecycle.
func TestEngine_RunProgram(t *testing.T) {
	retained := newTTRetainedGraphicsState(1<<16, 16, ttTargetSmooth)
	fontCode := []byte{opPUSHB000, 42}
	program := newTTProgramState(fontCode, nil, nil, ttProgramFont)
	defs := ttDefinitionState{
		functions:    newTTDefinitionMap(5),
		instructions: newTTDefinitionMap(5),
	}
	e := newTTEngine(
		&program, retained, defs, make([]int32, 5), make([]int32, 5),
		newTTValueStack(256, true),
		ttZone{}, ttZone{}, 0, nil, false, 5,
	)
	if err := e.runProgram(ttProgramFont, false); err != nil {
		t.Fatalf("runProgram: %v", err)
	}
	v, _ := e.valueStack.pop()
	if v != 42 {
		t.Errorf("got %d, want 42", v)
	}
}

// TestGraphicsState_ResetMethods tests reset and resetRetained.
func TestGraphicsState_ResetMethods(t *testing.T) {
	gs := defaultGraphicsState()
	gs.rp0 = 5
	gs.loopCounter = 10
	gs.retained.deltaBase = 99
	gs.reset()
	if gs.rp0 != 0 {
		t.Errorf("after reset: rp0 = %d, want 0", gs.rp0)
	}
	if gs.loopCounter != 1 {
		t.Errorf("after reset: loopCounter = %d, want 1", gs.loopCounter)
	}
	// retained fields are preserved
	if gs.retained.deltaBase != 99 {
		t.Errorf("after reset: deltaBase = %d, want 99", gs.retained.deltaBase)
	}
	// resetRetained resets deltaBase but preserves scale/ppem/target
	gs.retained.scale = 2 << 16
	gs.retained.ppem = 16
	gs.retained.target = ttTargetLCD
	gs.resetRetained()
	if gs.retained.deltaBase != 9 { // default
		t.Errorf("after resetRetained: deltaBase = %d, want 9", gs.retained.deltaBase)
	}
	if gs.retained.scale != 2<<16 {
		t.Errorf("after resetRetained: scale lost")
	}
}

func TestMath_Div16Dot16(t *testing.T) {
	// 2.0 / 1.0 in 16.16 = 2.0
	got := ttDiv16Dot16(2<<16, 1<<16)
	if got != 2<<16 {
		t.Errorf("div(2,1) = %d, want %d", got, 2<<16)
	}
	// Division by zero
	got = ttDiv16Dot16(1, 0)
	if got != 0x7FFFFFFF {
		t.Errorf("div(1,0) = %d, want maxint", got)
	}
}

func TestCallStack_Clear(t *testing.T) {
	cs := ttCallStack{}
	_ = cs.push(ttCallRecord{returnPC: 1})
	_ = cs.push(ttCallRecord{returnPC: 2})
	cs.clear()
	_, ok := cs.peek()
	if ok {
		t.Error("stack should be empty after clear")
	}
}

func TestDefinitionMap_Readonly(t *testing.T) {
	defs := []ttDefinition{{key: 0, isActive: true, start: 10, end: 20}}
	dm := newTTDefinitionMapReadonly(defs)
	// Should not be able to allocate
	_, err := dm.allocate(5)
	if !errors.Is(err, ttErrDefinitionInGlyphProgram) {
		t.Errorf("readonly allocate = %v, want error", err)
	}
	// Reset on readonly should be no-op
	dm.reset()
	if !dm.defs[0].isActive {
		t.Error("readonly reset should not clear defs")
	}
}

func TestLoopBudget_Reset(t *testing.T) {
	lb := newTTLoopBudget(10, 5)
	_ = lb.doingBackwardJump()
	_ = lb.doingLoopCall(5)
	lb.reset()
	if lb.backwardJumps != 0 || lb.loopCalls != 0 {
		t.Errorf("after reset: jumps=%d calls=%d, want 0,0", lb.backwardJumps, lb.loopCalls)
	}
}

func TestProgramState_ResetProgram(t *testing.T) {
	ps := newTTProgramState([]byte{1, 2}, []byte{3, 4}, []byte{5}, ttProgramFont)
	ps.decoder.pc = 2 // advance
	ps.resetProgram(ttProgramControlValue)
	if ps.current != ttProgramControlValue {
		t.Errorf("current = %d, want ControlValue", ps.current)
	}
	if ps.decoder.pc != 0 {
		t.Errorf("pc = %d, want 0", ps.decoder.pc)
	}
}

func TestValueStack_PushBytesPushWords(t *testing.T) {
	s := newTTValueStack(16, true)
	if err := s.pushBytes([]byte{10, 20, 30}); err != nil {
		t.Fatalf("pushBytes: %v", err)
	}
	if s.len() != 3 {
		t.Errorf("len = %d, want 3", s.len())
	}
	v, _ := s.pop()
	if v != 30 {
		t.Errorf("last byte = %d, want 30", v)
	}
	s.clear()
	if err := s.pushWords([]int16{-100, 200}); err != nil {
		t.Fatalf("pushWords: %v", err)
	}
	v, _ = s.pop()
	if v != 200 {
		t.Errorf("last word = %d, want 200", v)
	}
	v, _ = s.pop()
	if v != -100 {
		t.Errorf("first word = %d, want -100", v)
	}
}

func TestValueStack_ActiveValues(t *testing.T) {
	s := newTTValueStack(16, true)
	_ = s.push(1)
	_ = s.push(2)
	vals := s.activeValues()
	if len(vals) != 2 || vals[0] != 1 || vals[1] != 2 {
		t.Errorf("activeValues = %v, want [1,2]", vals)
	}
}

func TestZone_SetPointSetOriginal(t *testing.T) {
	z := ttZone{
		original: make([][2]int32, 2),
		points:   make([][2]int32, 2),
		flags:    make([]ttPointFlags, 2),
	}
	if err := z.setPoint(0, 100, 200); err != nil {
		t.Fatalf("setPoint: %v", err)
	}
	pt, _ := z.point(0)
	if pt != [2]int32{100, 200} {
		t.Errorf("point = %v, want [100,200]", pt)
	}
	if err := z.setOriginalPoint(1, 300, 400); err != nil {
		t.Fatalf("setOriginalPoint: %v", err)
	}
	opt, _ := z.originalPoint(1)
	if opt != [2]int32{300, 400} {
		t.Errorf("originalPoint = %v, want [300,400]", opt)
	}
	if z.pointCount() != 2 {
		t.Errorf("pointCount = %d, want 2", z.pointCount())
	}
}

func TestHintError_Detail(t *testing.T) {
	he := &ttHintError{
		program: ttProgramGlyph,
		pc:      10,
		opcode:  -1,
		kind:    ttErrInvalidPointIndex,
		detail:  42,
	}
	if he.detail != 42 {
		t.Errorf("detail = %d, want 42", he.detail)
	}
	// Verify no-opcode formatting
	s := he.Error()
	if s != "tt: glyf@10: tt: point index out of bounds" {
		t.Errorf("error = %q", s)
	}
}

func TestGraphicsState_UnscaledToPixels(t *testing.T) {
	gs := defaultGraphicsState()
	gs.retained.scale = 2 << 16
	// Non-composite: returns scale
	if gs.unscaledToPixels() != 2<<16 {
		t.Errorf("non-composite scale = %d", gs.unscaledToPixels())
	}
	// Composite: returns 1.0 (identity)
	gs.isComposite = true
	if gs.unscaledToPixels() != 1<<16 {
		t.Errorf("composite scale = %d", gs.unscaledToPixels())
	}
}
