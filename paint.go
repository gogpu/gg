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

	// LineWidth is the width of strokes
	LineWidth float64

	// LineCap is the shape of line endpoints
	LineCap LineCap

	// LineJoin is the shape of line joins
	LineJoin LineJoin

	// MiterLimit is the miter limit for sharp joins
	MiterLimit float64

	// FillRule is the fill rule for paths
	FillRule FillRule

	// Antialias enables anti-aliasing
	Antialias bool
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
	return &Paint{
		Pattern:    p.Pattern,
		Brush:      p.Brush,
		LineWidth:  p.LineWidth,
		LineCap:    p.LineCap,
		LineJoin:   p.LineJoin,
		MiterLimit: p.MiterLimit,
		FillRule:   p.FillRule,
		Antialias:  p.Antialias,
	}
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
