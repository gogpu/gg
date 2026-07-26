package gg

// LineCap specifies the shape of line endpoints.
type LineCap int

const (
	// LineCapButt specifies a flat line cap.
	LineCapButt LineCap = iota
	// LineCapRound specifies a rounded line cap.
	LineCapRound
	// LineCapSquare specifies a square line cap.
	LineCapSquare
)

// LineJoin specifies the shape of line joins.
type LineJoin int

const (
	// LineJoinMiter specifies a sharp (mitered) join.
	LineJoinMiter LineJoin = iota
	// LineJoinRound specifies a rounded join.
	LineJoinRound
	// LineJoinBevel specifies a beveled join.
	LineJoinBevel
)

// FillRule specifies how to determine which areas are inside a path.
type FillRule int

const (
	// FillRuleNonZero uses the non-zero winding rule.
	FillRuleNonZero FillRule = iota
	// FillRuleEvenOdd uses the even-odd rule.
	FillRuleEvenOdd
)

// brushState holds the color/brush/pattern state for one side (fill or stroke)
// of the dual-brush Paint model. Each side independently tracks whether its
// color is stored as an inline solid color (zero allocation) or via a Brush/Pattern
// interface. This matches the Canvas 2D model where fill and stroke styles
// are independent (ADR-055).
type brushState struct {
	// solidColor stores the solid color inline (Skia fColor4f pattern).
	// When isSolid is true, this is the authoritative color source —
	// brush and pattern are nil, avoiding interface boxing allocations.
	solidColor RGBA

	// isSolid is true when the state represents a single solid color
	// stored in solidColor. When true, brush and pattern are nil.
	isSolid bool

	// brush is the fill or stroke brush (vello/peniko pattern).
	// When both brush and pattern are set, brush takes precedence.
	brush Brush

	// pattern is the fill or stroke pattern.
	//
	// Deprecated: Use brush instead. Pattern is maintained for backward compatibility.
	pattern Pattern
}

// setBrush sets the brush for this state.
// For solid colors, the color is stored inline (zero allocations).
// For non-solid brushes, it also updates the pattern field for backward compatibility.
func (s *brushState) setBrush(b Brush) {
	if sb, ok := b.(SolidBrush); ok {
		s.solidColor = sb.Color
		s.isSolid = true
		s.brush = nil
		s.pattern = nil
		return
	}
	s.brush = b
	s.pattern = PatternFromBrush(b)
	s.isSolid = false
}

// setPattern sets the pattern for this state.
// For solid patterns, stores the color inline (zero allocations).
// Also updates the brush field for consistency with ColorAt precedence.
func (s *brushState) setPattern(p Pattern) {
	if sp, ok := p.(*SolidPattern); ok {
		s.solidColor = sp.Color
		s.isSolid = true
		s.brush = nil
		s.pattern = nil
		return
	}
	s.pattern = p
	s.brush = BrushFromPattern(p)
	s.isSolid = false
}

// setSolidColor sets a solid color inline (zero allocation).
func (s *brushState) setSolidColor(c RGBA) {
	s.solidColor = c
	s.isSolid = true
	s.brush = nil
	s.pattern = nil
}

// colorAt returns the color at the given position.
// For solid colors, returns the inline color directly (no interface dispatch).
// For non-solid states, uses brush if set, otherwise falls back to pattern.
func (s *brushState) colorAt(x, y float64) RGBA {
	if s.isSolid {
		return s.solidColor
	}
	if s.brush != nil {
		return s.brush.ColorAt(x, y)
	}
	if s.pattern != nil {
		return s.pattern.ColorAt(x, y)
	}
	return Black
}

// solidColor returns the inline solid color and true if the state is a solid
// color. Returns (zero, false) for non-solid states (gradients, patterns).
func (s *brushState) getSolidColor() (RGBA, bool) {
	if s.isSolid {
		return s.solidColor, true
	}
	return RGBA{}, false
}

// getBrush returns the current brush.
// For solid colors, returns a SolidBrush value (no allocation).
// If brush is nil and not solid, it returns a brush converted from pattern.
func (s *brushState) getBrush() Brush {
	if s.isSolid {
		return SolidBrush{Color: s.solidColor}
	}
	if s.brush != nil {
		return s.brush
	}
	if s.pattern != nil {
		return BrushFromPattern(s.pattern)
	}
	return SolidBrush{Color: Black}
}

// Paint represents the styling information for drawing.
//
// Paint uses a dual-brush model (ADR-055) where fill and stroke operations
// have independent color/brush/pattern state. This matches the Canvas 2D
// specification where fillStyle and strokeStyle are separate properties.
//
// Legacy methods (SetBrush, ColorAt, etc.) operate on BOTH sides for backward
// compatibility. Use the explicit Fill/Stroke variants for independent control.
type Paint struct {
	// fill holds the color state for fill operations.
	fill brushState

	// stroke holds the color state for stroke operations.
	stroke brushState

	// LineWidth is the width of strokes.
	//
	// Deprecated: Use Stroke.Width instead. Maintained for backward compatibility.
	LineWidth float64

	// LineCap is the shape of line endpoints.
	//
	// Deprecated: Use Stroke.Cap instead. Maintained for backward compatibility.
	LineCap LineCap

	// LineJoin is the shape of line joins.
	//
	// Deprecated: Use Stroke.Join instead. Maintained for backward compatibility.
	LineJoin LineJoin

	// MiterLimit is the miter limit for sharp joins.
	//
	// Deprecated: Use Stroke.MiterLimit instead. Maintained for backward compatibility.
	MiterLimit float64

	// FillRule is the fill rule for paths
	FillRule FillRule

	// Antialias enables anti-aliasing
	Antialias bool

	// Stroke is the unified stroke style configuration.
	// This is the preferred way to configure stroke properties.
	// When Stroke is set, it takes precedence over the individual
	// LineWidth, LineCap, LineJoin, and MiterLimit fields.
	Stroke *Stroke

	// TransformScale is the scale factor from the current transform matrix.
	// Used internally by the renderer to determine effective stroke width.
	// Set automatically by Context.Stroke() before rendering.
	TransformScale float64

	// ClipCoverage is a function that returns the clip coverage (0-255)
	// at a given pixel coordinate. When non-nil, the renderer multiplies
	// pixel alpha by this coverage to apply the clip mask.
	//
	// Deprecated: Use ClipMask for pre-rasterized clip masks (0.5ns/pixel
	// array lookup vs 8ns/pixel closure call). ClipCoverage is maintained
	// as a legacy fallback during the transition period.
	// Set automatically by Context before rendering when a clip is active.
	ClipCoverage func(x, y float64) byte

	// ClipMask is a pre-rasterized alpha mask for per-pixel clip coverage.
	// When non-nil, the renderer multiplies pixel alpha by the mask value
	// at the pixel's position instead of calling the ClipCoverage closure.
	// This replaces per-pixel closure calls (~8ns) with array lookups (~0.5ns).
	//
	// The mask is stored as a flat uint8 array in row-major order, with
	// stride ClipMaskW. Values: 0 = fully clipped, 255 = fully visible.
	// ClipMaskX/ClipMaskY define the mask origin in device pixel coordinates.
	//
	// Set automatically by Context.applyClipToPaint() for non-rect clips.
	ClipMask  []uint8
	ClipMaskW int // mask width (stride in pixels)
	ClipMaskH int // mask height
	ClipMaskX int // mask origin X in device pixels
	ClipMaskY int // mask origin Y in device pixels

	// MaskCoverage is a function that returns the alpha mask coverage (0-255)
	// at a given pixel coordinate. When non-nil, the renderer multiplies
	// pixel alpha by this coverage to apply the alpha mask.
	// Uses int coords because masks are pixel-aligned (no sub-pixel sampling).
	// Set automatically by Context before rendering when a mask is active.
	MaskCoverage func(x, y int) uint8

	// blendMode controls per-operation compositing. Default is blendModeSourceOver
	// (standard alpha-over). Set via Context.SetBlendMode(). The SoftwareRenderer
	// dispatches to the appropriate blend function when this differs from SourceOver.
	blendMode paintBlendMode
}

// NewPaint creates a new Paint with default values.
// Both fill and stroke sides are initialized to solid Black.
func NewPaint() *Paint {
	defaultBrush := brushState{solidColor: Black, isSolid: true}
	return &Paint{
		fill:       defaultBrush,
		stroke:     defaultBrush,
		LineWidth:  1.0,
		LineCap:    LineCapButt,
		LineJoin:   LineJoinMiter,
		MiterLimit: 10.0,
		FillRule:   FillRuleNonZero,
		Antialias:  true,
		blendMode:  blendModeSourceOver,
	}
}

// Clone creates a copy of the Paint.
// Both fill and stroke brush states are copied independently.
func (p *Paint) Clone() *Paint {
	clone := &Paint{
		fill:       p.fill,
		stroke:     p.stroke,
		LineWidth:  p.LineWidth,
		LineCap:    p.LineCap,
		LineJoin:   p.LineJoin,
		MiterLimit: p.MiterLimit,
		FillRule:   p.FillRule,
		Antialias:  p.Antialias,
		blendMode:  p.blendMode,
	}
	if p.Stroke != nil {
		strokeClone := p.Stroke.Clone()
		clone.Stroke = &strokeClone
	}
	return clone
}

// SetBrush sets the brush for BOTH fill and stroke (backward compatibility).
// For solid colors, the color is stored inline (zero allocations).
// For non-solid brushes, it also updates the Pattern field for backward compatibility.
func (p *Paint) SetBrush(b Brush) {
	p.fill.setBrush(b)
	p.stroke.setBrush(b)
}

// SetFillBrush sets the brush for fill operations only.
// This does not affect the stroke brush.
func (p *Paint) SetFillBrush(b Brush) {
	p.fill.setBrush(b)
}

// SetStrokeBrush sets the brush for stroke operations only.
// This does not affect the fill brush.
func (p *Paint) SetStrokeBrush(b Brush) {
	p.stroke.setBrush(b)
}

// GetBrush returns the current fill brush (backward compatibility alias).
// For solid colors, returns a SolidBrush value (no allocation).
//
// Deprecated: Use FillBrush() or StrokeBrush() for explicit side selection.
func (p *Paint) GetBrush() Brush {
	return p.fill.getBrush()
}

// FillBrush returns the current fill brush.
func (p *Paint) FillBrush() Brush {
	return p.fill.getBrush()
}

// StrokeBrush returns the current stroke brush.
func (p *Paint) StrokeBrush() Brush {
	return p.stroke.getBrush()
}

// ColorAt returns the fill color at the given position (backward compatibility).
// For solid colors, returns the inline color directly (no interface dispatch).
//
// Deprecated: Use FillColorAt() or StrokeColorAt() for explicit side selection.
func (p *Paint) ColorAt(x, y float64) RGBA {
	return p.fill.colorAt(x, y)
}

// FillColorAt returns the fill color at the given position.
func (p *Paint) FillColorAt(x, y float64) RGBA {
	return p.fill.colorAt(x, y)
}

// StrokeColorAt returns the stroke color at the given position.
func (p *Paint) StrokeColorAt(x, y float64) RGBA {
	return p.stroke.colorAt(x, y)
}

// SolidColor returns the inline fill solid color and true if the fill is a solid
// color. Returns (zero, false) for non-solid paints (gradients, patterns).
//
// Deprecated: Use FillSolidColor() or StrokeSolidColor() for explicit side selection.
func (p *Paint) SolidColor() (RGBA, bool) {
	return p.fill.getSolidColor()
}

// FillSolidColor returns the inline fill solid color and true if the fill is
// solid. Returns (zero, false) for non-solid fills (gradients, patterns).
func (p *Paint) FillSolidColor() (RGBA, bool) {
	return p.fill.getSolidColor()
}

// StrokeSolidColor returns the inline stroke solid color and true if the stroke
// is solid. Returns (zero, false) for non-solid strokes (gradients, patterns).
func (p *Paint) StrokeSolidColor() (RGBA, bool) {
	return p.stroke.getSolidColor()
}

// IsSolid reports whether the fill paint is a solid color stored inline.
//
// Deprecated: Use IsFillSolid() or IsStrokeSolid() for explicit side selection.
func (p *Paint) IsSolid() bool {
	return p.fill.isSolid
}

// IsFillSolid reports whether the fill paint is a solid color stored inline.
func (p *Paint) IsFillSolid() bool {
	return p.fill.isSolid
}

// IsStrokeSolid reports whether the stroke paint is a solid color stored inline.
func (p *Paint) IsStrokeSolid() bool {
	return p.stroke.isSolid
}

// colorAtForMode returns a closure that reads from the fill or stroke side
// depending on strokeMode. Used by SoftwareRenderer to avoid threading a
// mode parameter through every blend function.
func (p *Paint) colorAtForMode(strokeMode bool) func(x, y float64) RGBA {
	if strokeMode {
		return p.stroke.colorAt
	}
	return p.fill.colorAt
}

// GetStroke returns the effective stroke style.
// If Stroke is set, returns a copy of it.
// Otherwise, constructs a Stroke from the legacy fields.
func (p *Paint) GetStroke() Stroke {
	if p.Stroke != nil {
		return p.Stroke.Clone()
	}
	return Stroke{
		Width:      p.LineWidth,
		Cap:        p.LineCap,
		Join:       p.LineJoin,
		MiterLimit: p.MiterLimit,
		Dash:       nil,
	}
}

// SetStroke sets the stroke style.
// This also updates the legacy fields for backward compatibility.
func (p *Paint) SetStroke(s Stroke) {
	strokeCopy := s.Clone()
	p.Stroke = &strokeCopy

	// Update legacy fields for backward compatibility
	p.LineWidth = s.Width
	p.LineCap = s.Cap
	p.LineJoin = s.Join
	p.MiterLimit = s.MiterLimit
}

// EffectiveLineWidth returns the effective line width.
// If Stroke is set, uses Stroke.Width; otherwise uses LineWidth.
func (p *Paint) EffectiveLineWidth() float64 {
	if p.Stroke != nil {
		return p.Stroke.Width
	}
	return p.LineWidth
}

// EffectiveLineCap returns the effective line cap.
// If Stroke is set, uses Stroke.Cap; otherwise uses LineCap.
func (p *Paint) EffectiveLineCap() LineCap {
	if p.Stroke != nil {
		return p.Stroke.Cap
	}
	return p.LineCap
}

// EffectiveLineJoin returns the effective line join.
// If Stroke is set, uses Stroke.Join; otherwise uses LineJoin.
func (p *Paint) EffectiveLineJoin() LineJoin {
	if p.Stroke != nil {
		return p.Stroke.Join
	}
	return p.LineJoin
}

// EffectiveMiterLimit returns the effective miter limit.
// If Stroke is set, uses Stroke.MiterLimit; otherwise uses MiterLimit.
func (p *Paint) EffectiveMiterLimit() float64 {
	if p.Stroke != nil {
		return p.Stroke.MiterLimit
	}
	return p.MiterLimit
}

// EffectiveDash returns the effective dash pattern.
// Returns nil if no dash is set (solid line).
func (p *Paint) EffectiveDash() *Dash {
	if p.Stroke != nil && p.Stroke.Dash != nil {
		return p.Stroke.Dash.Clone()
	}
	return nil
}

// IsDashed returns true if the current stroke uses a dash pattern.
func (p *Paint) IsDashed() bool {
	return p.Stroke != nil && p.Stroke.IsDashed()
}
