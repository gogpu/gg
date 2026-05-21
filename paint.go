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

// Paint represents the styling information for drawing.
type Paint struct {
	// solidColor stores the solid color inline (Skia fColor4f pattern).
	// When isSolid is true, this is the authoritative color source —
	// Brush and Pattern are nil, avoiding interface boxing allocations.
	solidColor RGBA

	// isSolid is true when the paint represents a single solid color
	// stored in solidColor. When true, Brush and Pattern are nil.
	isSolid bool

	// Pattern is the fill or stroke pattern.
	//
	// Deprecated: Use Brush instead. Pattern is maintained for backward compatibility.
	Pattern Pattern

	// Brush is the fill or stroke brush (vello/peniko pattern).
	// When both Brush and Pattern are set, Brush takes precedence.
	// Use SetBrush() to set the brush, which also updates Pattern for compatibility.
	Brush Brush

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
	// Set automatically by Context before rendering when a clip is active.
	ClipCoverage func(x, y float64) byte

	// MaskCoverage is a function that returns the alpha mask coverage (0-255)
	// at a given pixel coordinate. When non-nil, the renderer multiplies
	// pixel alpha by this coverage to apply the alpha mask.
	// Uses int coords because masks are pixel-aligned (no sub-pixel sampling).
	// Set automatically by Context before rendering when a mask is active.
	MaskCoverage func(x, y int) uint8
}

// NewPaint creates a new Paint with default values.
func NewPaint() *Paint {
	return &Paint{
		solidColor: Black,
		isSolid:    true,
		LineWidth:  1.0,
		LineCap:    LineCapButt,
		LineJoin:   LineJoinMiter,
		MiterLimit: 10.0,
		FillRule:   FillRuleNonZero,
		Antialias:  true,
	}
}

// Clone creates a copy of the Paint.
func (p *Paint) Clone() *Paint {
	clone := &Paint{
		solidColor: p.solidColor,
		isSolid:    p.isSolid,
		Pattern:    p.Pattern,
		Brush:      p.Brush,
		LineWidth:  p.LineWidth,
		LineCap:    p.LineCap,
		LineJoin:   p.LineJoin,
		MiterLimit: p.MiterLimit,
		FillRule:   p.FillRule,
		Antialias:  p.Antialias,
	}
	if p.Stroke != nil {
		strokeClone := p.Stroke.Clone()
		clone.Stroke = &strokeClone
	}
	return clone
}

// SetBrush sets the brush for this Paint.
// For solid colors, the color is stored inline (zero allocations).
// For non-solid brushes, it also updates the Pattern field for backward compatibility.
func (p *Paint) SetBrush(b Brush) {
	if sb, ok := b.(SolidBrush); ok {
		p.solidColor = sb.Color
		p.isSolid = true
		p.Brush = nil
		p.Pattern = nil
		return
	}
	p.Brush = b
	p.Pattern = PatternFromBrush(b)
	p.isSolid = false
}

// GetBrush returns the current brush.
// For solid colors, returns a SolidBrush value (no allocation).
// If Brush is nil and not solid, it returns a brush converted from Pattern.
func (p *Paint) GetBrush() Brush {
	if p.isSolid {
		return SolidBrush{Color: p.solidColor}
	}
	if p.Brush != nil {
		return p.Brush
	}
	if p.Pattern != nil {
		return BrushFromPattern(p.Pattern)
	}
	return SolidBrush{Color: Black}
}

// ColorAt returns the color at the given position.
// For solid colors, returns the inline color directly (no interface dispatch).
// For non-solid paints, uses Brush if set, otherwise falls back to Pattern.
func (p *Paint) ColorAt(x, y float64) RGBA {
	if p.isSolid {
		return p.solidColor
	}
	if p.Brush != nil {
		return p.Brush.ColorAt(x, y)
	}
	if p.Pattern != nil {
		return p.Pattern.ColorAt(x, y)
	}
	return Black
}

// SolidColor returns the inline solid color and true if the paint is a solid
// color. Returns (zero, false) for non-solid paints (gradients, patterns).
// This is the recommended way for external packages to check solid color
// without interface type assertions on Brush/Pattern.
func (p *Paint) SolidColor() (RGBA, bool) {
	if p.isSolid {
		return p.solidColor, true
	}
	return RGBA{}, false
}

// IsSolid reports whether the paint is a solid color stored inline.
func (p *Paint) IsSolid() bool {
	return p.isSolid
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
