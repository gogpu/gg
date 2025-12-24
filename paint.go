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
}

// NewPaint creates a new Paint with default values.
func NewPaint() *Paint {
	return &Paint{
		Pattern:    NewSolidPattern(Black),
		Brush:      Solid(Black),
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
// It also updates the Pattern field for backward compatibility.
func (p *Paint) SetBrush(b Brush) {
	p.Brush = b
	p.Pattern = PatternFromBrush(b)
}

// GetBrush returns the current brush.
// If Brush is nil, it returns a brush converted from Pattern.
func (p *Paint) GetBrush() Brush {
	if p.Brush != nil {
		return p.Brush
	}
	if p.Pattern != nil {
		return BrushFromPattern(p.Pattern)
	}
	return Solid(Black)
}

// ColorAt returns the color at the given position.
// It uses Brush if set, otherwise falls back to Pattern.
func (p *Paint) ColorAt(x, y float64) RGBA {
	if p.Brush != nil {
		return p.Brush.ColorAt(x, y)
	}
	if p.Pattern != nil {
		return p.Pattern.ColorAt(x, y)
	}
	return Black
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
