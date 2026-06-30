// TrueType bytecode interpreter — hint instance (per-size cached state).
//
// Port of skrifa hint/instance.rs (275 LOC).
// HintInstance caches the results of running fpgm + prep programs for a
// given font size. This state is reused across all glyph hints at that size.
//
// Lifecycle matching skrifa:
//  1. newTTHintInstance() — allocate buffers, scale CVT, run fpgm + prep
//  2. hintGlyph() — run per-glyph bytecode using cached state
//  3. isEnabled() — check if prep disabled hinting
//
// Reference: skrifa/src/outline/glyf/hint/instance.rs
package text

// ttPhantomPointCount is the number of phantom points appended to each glyph.
// These 4 extra points encode hinted metrics (lsb, advance, tsb, vadvance).
//
// Reference: skrifa glyf/mod.rs:34
const ttPhantomPointCount = 4

// ttHintInstance holds cached state from running the font program (fpgm) and
// control value program (prep) at a specific font size. This matches skrifa's
// HintInstance struct.
//
// The instance is reused for all glyphs at the same ppem to avoid re-running
// the expensive fpgm/prep programs.
//
// Reference: skrifa hint/instance.rs:22-33
type ttHintInstance struct {
	// Cached from fpgm execution — function and instruction definitions.
	functions    []ttDefinition
	instructions []ttDefinition

	// Cached from prep execution — scaled CVT and storage.
	cvt     []int32 // scaled CVT values (26.6)
	storage []int32 // storage area

	// Retained graphics state from prep program.
	graphics ttRetainedGraphicsState

	// Twilight zone state after fpgm+prep.
	twilightScaled         [][2]int32
	twilightOriginalScaled [][2]int32
	twilightFlags          []ttPointFlags

	// Raw bytecodes — needed for glyph programs that CALL functions
	// defined in fpgm. The glyph engine must have access to all three
	// bytecodes (fpgm, prep, glyph) so that function calls can switch
	// to the correct bytecode stream.
	// Reference: skrifa hint/instance.rs:140-145
	fpgm []byte
	prep []byte

	// maxStack for value stack allocation.
	maxStack int

	// scale is the 16.16 fixed-point scale factor (ppem * 64 / upem).
	scale int32

	// ppem is the nominal pixels per em.
	ppem int32
}

// newTTHintInstance creates a hint instance for the given font at the specified
// ppem. Runs fpgm and prep programs to initialize function definitions, CVT,
// and retained graphics state.
//
// This matches skrifa HintInstance::reconfigure.
//
// Reference: skrifa hint/instance.rs:36-81
//
//nolint:unparam // error return kept for API parity with skrifa HintInstance::reconfigure
func newTTHintInstance(font *ttFontProgram, ppem int32, target ttTarget) (*ttHintInstance, error) {
	// Compute scale: ppem * 64 / upem in 16.16 fixed-point.
	// This is 26.6 pixels per font unit, stored as 16.16.
	//
	// Uses rounded division to match skrifa exactly:
	//   Fixed::from_bits(ppem * 64) / Fixed::from_bits(upem)
	// which computes: ((a << 16) + (b >> 1)) / b
	//
	// Reference: skrifa glyf/mod.rs Scale26Dot6::new (line 387)
	// Reference: font-types/src/fixed.rs impl Div for Fixed (line 205)
	scale := int32(0)
	if font.unitsPerEm > 0 {
		a := uint64(ppem*64) << 16
		b := uint64(font.unitsPerEm)
		scale = int32((a + b/2) / b)
	}

	h := &ttHintInstance{
		scale:    scale,
		ppem:     ppem,
		maxStack: font.maxStack,
		fpgm:     font.fpgm,
		prep:     font.prep,
	}

	h.setup(font, scale)

	// Create twilight zone for fpgm/prep.
	twilightContours := []uint16{uint16(len(h.twilightScaled))} //nolint:gosec // bounded by maxTwilightPoints
	twilight := ttZone{
		unscaled: nil,
		original: h.twilightOriginalScaled,
		points:   h.twilightScaled,
		flags:    h.twilightFlags,
		contours: twilightContours,
	}
	glyph := ttZone{}

	// Create value stack.
	stack := newTTValueStack(h.maxStack, false)

	// Create initial graphics state.
	graphics := newTTRetainedGraphicsState(scale, ppem, target)

	// Create definition state (mutable for fpgm/prep).
	defs := ttDefinitionState{
		functions:    newTTDefinitionMap(len(h.functions)),
		instructions: newTTDefinitionMap(len(h.instructions)),
	}

	// Create program state with fpgm and prep bytecode.
	program := newTTProgramState(font.fpgm, font.prep, nil, ttProgramFont)

	// Create engine and run font program.
	engine := newTTEngine(
		&program, graphics, defs, h.cvt, h.storage, stack,
		twilight, glyph, 0, nil, false, len(h.cvt),
	)

	// Run font program (fpgm) — defines functions.
	// Non-pedantic: fpgm errors are common in production fonts (skrifa ignores them).
	// We proceed to prep regardless, matching skrifa instance.rs:125.
	// skrifa non-pedantic: fpgm errors ignored (instance.rs:125)
	_ = engine.runProgram(ttProgramFont, false)

	// Run control value program (prep) — sets cutins, instruct control, etc.
	// skrifa non-pedantic: prep errors ignored (instance.rs:131)
	_ = engine.runProgram(ttProgramControlValue, false)

	// Save retained graphics state.
	h.graphics = *engine.retainedGraphicsState()

	// Save function/instruction definitions.
	h.functions = make([]ttDefinition, len(engine.definitions.functions.defs))
	copy(h.functions, engine.definitions.functions.defs)
	h.instructions = make([]ttDefinition, len(engine.definitions.instructions.defs))
	copy(h.instructions, engine.definitions.instructions.defs)

	// Save CVT and storage (may have been modified by prep).
	h.cvt = make([]int32, len(engine.cvt))
	copy(h.cvt, engine.cvt)
	h.storage = make([]int32, len(engine.storage))
	copy(h.storage, engine.storage)

	// Save twilight zone state.
	h.twilightScaled = make([][2]int32, len(engine.graphics.zones[0].points))
	copy(h.twilightScaled, engine.graphics.zones[0].points)
	h.twilightOriginalScaled = make([][2]int32, len(engine.graphics.zones[0].original))
	copy(h.twilightOriginalScaled, engine.graphics.zones[0].original)
	h.twilightFlags = make([]ttPointFlags, len(engine.graphics.zones[0].flags))
	copy(h.twilightFlags, engine.graphics.zones[0].flags)

	return h, nil
}

// isEnabled returns true if hinting should be applied.
// The prep program can disable hinting by setting instruct control bit 0.
//
// Reference: skrifa hint/instance.rs:86-89
func (h *ttHintInstance) isEnabled() bool {
	return h.graphics.instructControl&1 == 0
}

// backwardCompatibility returns true if backward compatibility mode is active.
// This suppresses X-axis movements for ClearType-era fonts.
//
// Reference: skrifa hint/instance.rs:93-102
func (h *ttHintInstance) backwardCompatibility() bool {
	if h.graphics.target.preserveLinearMetrics() {
		return true
	}
	if h.graphics.target.isSmooth() {
		return (h.graphics.instructControl & 0x4) == 0
	}
	return false
}

// hintGlyph runs the per-glyph bytecode interpreter on the provided outline.
// After execution, the outline's points and phantom points are updated with
// hinted positions.
//
// The outline must have:
//   - points/original/flags allocated for numPoints + 4 phantom points
//   - contours set to the glyph's contour endpoints
//   - bytecode from the glyph's instructions
//   - phantom points initialized and appended to the end of points
//
// Reference: skrifa hint/instance.rs:104-177
func (h *ttHintInstance) hintGlyph(outline *ttGlyphOutline) error {
	numPhysicalPoints := len(outline.points) // includes phantom points

	// Copy twilight zone state (each glyph gets a fresh copy).
	twilightCount := len(h.twilightScaled)
	twilightScaled := make([][2]int32, twilightCount)
	copy(twilightScaled, h.twilightScaled)
	twilightOriginalScaled := make([][2]int32, twilightCount)
	copy(twilightOriginalScaled, h.twilightOriginalScaled)
	twilightFlags := make([]ttPointFlags, twilightCount)
	copy(twilightFlags, h.twilightFlags)
	twilightContours := []uint16{uint16(twilightCount)} //nolint:gosec // bounded by maxTwilightPoints

	twilight := ttZone{
		unscaled: nil,
		original: twilightOriginalScaled,
		points:   twilightScaled,
		flags:    twilightFlags,
		contours: twilightContours,
	}

	// Build glyph zone with a COPY of outline points. The engine modifies
	// zone points in-place (MDAP, MIRP, SHP, etc.), so we must work on a
	// copy to preserve the original if the glyph program fails. On success,
	// hinted points are copied back to outline.points.
	//
	// Reference: skrifa hint/instance.rs creates a fresh points buffer per glyph.
	glyphPoints := make([][2]int32, len(outline.points))
	copy(glyphPoints, outline.points)

	glyph := ttZone{
		unscaled: outline.unscaled,
		original: outline.original,
		points:   glyphPoints,
		flags:    outline.flags,
		contours: outline.contours,
	}

	// Allocate scratch buffers for CVT and storage (copy-on-write pattern).
	cvt := make([]int32, len(h.cvt))
	copy(cvt, h.cvt)
	storage := make([]int32, len(h.storage))
	copy(storage, h.storage)

	// Create value stack.
	stack := newTTValueStack(h.maxStack, false)

	// Create definition state (read-only for glyph programs).
	defs := ttDefinitionState{
		functions:    newTTDefinitionMapReadonly(h.functions),
		instructions: newTTDefinitionMapReadonly(h.instructions),
	}

	// Create program state with ALL bytecodes — the glyph program may CALL
	// functions defined in fpgm. Without fpgm bytecode, CALL instructions
	// would switch to an empty decoder and silently fail.
	// Reference: skrifa hint/instance.rs:140-145 passes outlines.fpgm + outlines.prep + outline.bytecode
	program := newTTProgramState(h.fpgm, h.prep, outline.bytecode, ttProgramGlyph)

	// Create and run engine.
	engine := newTTEngine(
		&program, h.graphics, defs, cvt, storage, stack,
		twilight, glyph, 0, nil, outline.isComposite, len(h.cvt),
	)

	if err := engine.runProgram(ttProgramGlyph, false); err != nil {
		// Non-pedantic: glyph program errors are common in production fonts.
		// Original outline.points are untouched (engine worked on glyphPoints copy).
		// Return the error so the caller can fall back to auto-hinter.
		return err
	}

	// Copy hinted points from zone copy back to outline.
	copy(outline.points, engine.graphics.zones[1].points)

	// Extract phantom points.
	// If backward compatibility mode is disabled, capture modified phantom points.
	// Reference: skrifa hint/instance.rs:168-175
	if !engine.backwardCompatibility() {
		phantomStart := numPhysicalPoints - ttPhantomPointCount
		if phantomStart >= 0 && phantomStart+ttPhantomPointCount <= len(outline.points) {
			for i := range ttPhantomPointCount {
				outline.phantoms[i] = outline.points[phantomStart+i]
			}
		}
	}

	return nil
}

// setup initializes buffers and scales the CVT.
// Matches skrifa hint/instance.rs:180-235.
//
// Reference: skrifa hint/instance.rs:180-235
func (h *ttHintInstance) setup(font *ttFontProgram, scale int32) {
	// Allocate function/instruction definition buffers.
	h.functions = make([]ttDefinition, font.maxFunctionDefs)
	h.instructions = make([]ttDefinition, font.maxInstructionDefs)

	// Scale CVT values.
	// CVT values are font units (int16 from table) → convert to 26.6, then scale.
	//
	// Uses rounded 16.16 multiply (Fixed::mul) matching skrifa exactly.
	// CVT values are in 26.6 (font_units * 64), scale is adjusted: scale >> 6.
	//
	// Reference: skrifa hint/instance.rs:236-242
	// Reference: FreeType ttobjs.c:996
	h.cvt = make([]int32, len(font.cvt))
	scaleFrac := scale >> 6 // scale >> 6 = Fixed scale for 26.6 CVT values
	for i, v := range font.cvt {
		v26dot6 := v * 64
		h.cvt[i] = ttMul16Dot16(v26dot6, scaleFrac)
	}

	// Allocate storage area.
	h.storage = make([]int32, font.maxStorage)

	// Allocate twilight zone points.
	maxTwilight := font.maxTwilight
	h.twilightScaled = make([][2]int32, maxTwilight)
	h.twilightOriginalScaled = make([][2]int32, maxTwilight)
	h.twilightFlags = make([]ttPointFlags, maxTwilight)

	// Reset graphics state.
	h.graphics = defaultRetainedGraphicsState()
}
