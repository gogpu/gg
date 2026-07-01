// TrueType bytecode interpreter — graphics state.
//
// Port of skrifa hint/graphics.rs (316 LOC).
// Manages the interpreter's graphics state including projection/freedom
// vectors, reference points, zone pointers, and retained state.
//
// Reference: skrifa/src/outline/glyf/hint/graphics.rs
package text

// ttCoordAxis describes which axis a measurement or movement applies to.
// Reference: skrifa hint/graphics.rs:12-18
type ttCoordAxis uint8

const (
	ttCoordBoth ttCoordAxis = iota
	ttCoordX
	ttCoordY
)

// ttTarget describes the hinting target (rendering mode).
type ttTarget uint8

const (
	ttTargetNormal ttTarget = iota
	ttTargetSmooth
	ttTargetLCD
	ttTargetLCDV
)

// isSmooth returns true for smooth (anti-aliased) targets.
func (t ttTarget) isSmooth() bool {
	return t >= ttTargetSmooth
}

// preserveLinearMetrics returns true if the target preserves linear
// metrics (LCD subpixel rendering).
func (t ttTarget) preserveLinearMetrics() bool {
	return t == ttTargetLCD || t == ttTargetLCDV
}

// isVerticalLCD returns true if the target uses vertical LCD subpixels.
// Reference: skrifa hint.rs Target::is_vertical_lcd
func (t ttTarget) isVerticalLCD() bool {
	return t == ttTargetLCDV
}

// isGrayscaleClearType returns true for smooth targets that use grayscale
// rendering (not LCD subpixel). This corresponds to SmoothMode::Normal
// in skrifa — the default smooth mode without LCD optimization.
// Reference: skrifa hint.rs:496-501
func (t ttTarget) isGrayscaleClearType() bool {
	return t == ttTargetSmooth
}

// ttRetainedGraphicsState holds the persistent portion of the graphics state
// that survives between interpreter runs (set by CV program, persists for
// all glyph programs).
//
// Reference: skrifa hint/graphics.rs:200-265
type ttRetainedGraphicsState struct {
	// autoFlip controls whether CVT entry signs are flipped to match distances.
	autoFlip bool
	// controlValueCutin limits CVT regularization (default: 68 = 17/16 pixels in 26.6).
	controlValueCutin int32
	// deltaBase is the base ppem for DELTAP/DELTAC (default: 9).
	deltaBase uint16
	// deltaShift determines step size for DELTAP/DELTAC (default: 3).
	deltaShift uint16
	// instructControl controls instruction execution behavior.
	instructControl uint8
	// minDistance is the smallest distance after rounding (default: 64 = 1px in 26.6).
	minDistance int32
	// scanControl enables dropout control for the current glyph.
	scanControl bool
	// scanType is associated with scanControl.
	scanType int32
	// singleWidthCutin is the threshold for single-width substitution.
	singleWidthCutin int32
	// singleWidth is the value used when within single-width cutin.
	singleWidth int32
	// target is the hinting target (Normal/Smooth/LCD/LCDV).
	target ttTarget
	// scale is the font-units-to-26.6 scale factor (16.16 fixed-point).
	scale int32
	// ppem is the nominal pixels per em.
	ppem int32
	// isRotated is true if a rotation is applied.
	isRotated bool
	// isStretched is true if a non-uniform scale is applied.
	isStretched bool
}

// newTTRetainedGraphicsState creates a retained state with the given
// scale, ppem, and target, all other fields at defaults.
// Reference: skrifa hint/graphics.rs:268-276
func newTTRetainedGraphicsState(scale, ppem int32, target ttTarget) ttRetainedGraphicsState {
	s := defaultRetainedGraphicsState()
	s.scale = scale
	s.ppem = ppem
	s.target = target
	return s
}

// defaultRetainedGraphicsState returns the default retained state.
// Reference: skrifa hint/graphics.rs:278-301
func defaultRetainedGraphicsState() ttRetainedGraphicsState {
	return ttRetainedGraphicsState{
		autoFlip:          true,
		controlValueCutin: 68, // 17/16 pixels in 26.6: (17*64)/16 = 68
		deltaBase:         9,
		deltaShift:        3,
		instructControl:   0,
		minDistance:       64, // 1 pixel in 26.6
		scanControl:       false,
		scanType:          0,
		singleWidthCutin:  0,
		singleWidth:       0,
		target:            ttTargetNormal,
		scale:             0,
		ppem:              0,
		isRotated:         false,
		isStretched:       false,
	}
}

// ttGraphicsState is the full graphics state for the interpreter.
// Reference: skrifa hint/graphics.rs:24-116
type ttGraphicsState struct {
	retained ttRetainedGraphicsState

	// Projection/freedom vectors (2.14 fixed-point).
	projVector     [2]int32 // projection vector (x, y)
	projAxis       ttCoordAxis
	dualProjVector [2]int32 // dual projection vector
	dualProjAxis   ttCoordAxis
	freedomVector  [2]int32 // freedom vector
	freedomAxis    ttCoordAxis

	// Dot product of freedom and projection vectors (2.14).
	fdotp int32

	// Round state.
	roundState ttRoundState

	// Reference points.
	rp0 int
	rp1 int
	rp2 int

	// Loop counter (default 1).
	loopCounter int32

	// Zone pointers.
	zp0 ttZonePointer
	zp1 ttZonePointer
	zp2 ttZonePointer

	// Zone data.
	zones [2]ttZone // [0]=twilight, [1]=glyph

	// Composite glyph flag.
	isComposite bool

	// Backward compatibility mode.
	// When true, suppresses certain outline modifications for ClearType compat.
	// Reference: skrifa hint/graphics.rs:93-106
	backwardCompatibility bool

	// Pedantic mode enables strict error checking.
	isPedantic bool

	// IUP execution tracking.
	didIUPx bool
	didIUPy bool
}

// defaultGraphicsState returns a graphics state with default values.
// All vectors default to the X axis (0x4000, 0) in 2.14 format.
// Reference: skrifa hint/graphics.rs:163-192
func defaultGraphicsState() ttGraphicsState {
	return ttGraphicsState{
		retained:              defaultRetainedGraphicsState(),
		projVector:            [2]int32{0x4000, 0},
		projAxis:              ttCoordBoth,
		dualProjVector:        [2]int32{0x4000, 0},
		dualProjAxis:          ttCoordBoth,
		freedomVector:         [2]int32{0x4000, 0},
		freedomAxis:           ttCoordBoth,
		fdotp:                 0x4000,
		roundState:            defaultRoundState(),
		rp0:                   0,
		rp1:                   0,
		rp2:                   0,
		loopCounter:           1,
		zp0:                   ttZoneGlyph,
		zp1:                   ttZoneGlyph,
		zp2:                   ttZoneGlyph,
		isComposite:           false,
		backwardCompatibility: true,
		isPedantic:            false,
		didIUPx:               false,
		didIUPy:               false,
	}
}

// unscaledToPixels returns the scale factor for converting unscaled points
// to pixels. For composite glyphs, unscaled points are already scaled
// so we return the identity (1.0 in 16.16).
// Reference: skrifa hint/graphics.rs:119-125
func (gs *ttGraphicsState) unscaledToPixels() int32 {
	if gs.isComposite {
		return 1 << 16
	}
	return gs.retained.scale
}

// reset resets the non-retained portions of the graphics state.
// Retains the retained state and zone data.
// Reference: skrifa hint/graphics.rs:132-146
func (gs *ttGraphicsState) reset() {
	retained := gs.retained
	zones := gs.zones
	isComposite := gs.isComposite
	*gs = defaultGraphicsState()
	gs.retained = retained
	gs.zones = zones
	gs.isComposite = isComposite
	gs.updateProjectionState()
}

// resetRetained resets the retained state to defaults while preserving
// scale, ppem, and target.
// Reference: skrifa hint/graphics.rs:149-160
func (gs *ttGraphicsState) resetRetained() {
	scale := gs.retained.scale
	ppem := gs.retained.ppem
	target := gs.retained.target
	gs.retained = defaultRetainedGraphicsState()
	gs.retained.scale = scale
	gs.retained.ppem = ppem
	gs.retained.target = target
}

// updateProjectionState updates cached state derived from projection vectors.
// This must be called after any vector modification.
// Reference: skrifa hint/projection.rs:7-51
func (gs *ttGraphicsState) updateProjectionState() {
	const one = 0x4000 // 1.0 in 2.14

	// Compute fdotp (dot product of freedom and projection vectors)
	switch {
	case gs.freedomVector[0] == one:
		gs.fdotp = gs.projVector[0]
	case gs.freedomVector[1] == one:
		gs.fdotp = gs.projVector[1]
	default:
		px, py := gs.projVector[0], gs.projVector[1]
		fx, fy := gs.freedomVector[0], gs.freedomVector[1]
		gs.fdotp = (px*fx + py*fy) >> 14
	}

	// Determine projection axis optimization
	gs.projAxis = ttCoordBoth
	if gs.projVector[0] == one {
		gs.projAxis = ttCoordX
	} else if gs.projVector[1] == one {
		gs.projAxis = ttCoordY
	}

	// Determine dual projection axis optimization
	gs.dualProjAxis = ttCoordBoth
	if gs.dualProjVector[0] == one {
		gs.dualProjAxis = ttCoordX
	} else if gs.dualProjVector[1] == one {
		gs.dualProjAxis = ttCoordY
	}

	// Determine freedom axis optimization
	gs.freedomAxis = ttCoordBoth
	if gs.fdotp == one {
		if gs.freedomVector[0] == one {
			gs.freedomAxis = ttCoordX
		} else if gs.freedomVector[1] == one {
			gs.freedomAxis = ttCoordY
		}
	}

	// At small sizes, fdotp can become too small causing overflow/spikes.
	if gs.fdotp > -0x400 && gs.fdotp < 0x400 {
		gs.fdotp = one
	}
}

// project computes the projection of vector (v1 - v2) along the
// current projection vector.
// Reference: skrifa hint/projection.rs:56-74
func (gs *ttGraphicsState) project(v1x, v1y, v2x, v2y int32) int32 {
	switch gs.projAxis {
	case ttCoordX:
		return v1x - v2x
	case ttCoordY:
		return v1y - v2y
	default:
		dx := v1x - v2x
		dy := v1y - v2y
		return ttDot14(dx, dy, gs.projVector[0], gs.projVector[1])
	}
}

// dualProject computes the projection of (v1 - v2) along the
// current dual projection vector.
// Reference: skrifa hint/projection.rs:79-97
func (gs *ttGraphicsState) dualProject(v1x, v1y, v2x, v2y int32) int32 {
	switch gs.dualProjAxis {
	case ttCoordX:
		return v1x - v2x
	case ttCoordY:
		return v1y - v2y
	default:
		dx := v1x - v2x
		dy := v1y - v2y
		return ttDot14(dx, dy, gs.dualProjVector[0], gs.dualProjVector[1])
	}
}

// dualProjectUnscaled computes the projection of (v1 - v2) along the
// current dual projection vector for unscaled (font-unit) points.
// This is the same operation as dualProject but takes raw int32 coordinates
// instead of 26.6 fixed-point coordinates.
// Reference: skrifa hint/projection.rs:99-115
func (gs *ttGraphicsState) dualProjectUnscaled(v1x, v1y, v2x, v2y int32) int32 {
	switch gs.dualProjAxis {
	case ttCoordX:
		return v1x - v2x
	case ttCoordY:
		return v1y - v2y
	default:
		dx := v1x - v2x
		dy := v1y - v2y
		return ttDot14(dx, dy, gs.dualProjVector[0], gs.dualProjVector[1])
	}
}
