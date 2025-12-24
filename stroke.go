package gg

// Stroke defines the style for stroking paths.
// It encapsulates all stroke-related properties in a single struct,
// following the tiny-skia/kurbo pattern for unified stroke configuration.
type Stroke struct {
	// Width is the line width in pixels. Default: 1.0
	Width float64

	// Cap is the shape of line endpoints. Default: LineCapButt
	Cap LineCap

	// Join is the shape of line joins. Default: LineJoinMiter
	Join LineJoin

	// MiterLimit is the limit for miter joins before they become bevels.
	// Default: 4.0 (common default, matches SVG)
	MiterLimit float64

	// Dash is the dash pattern for the stroke.
	// nil means a solid line (no dashing).
	Dash *Dash
}

// DefaultStroke returns a Stroke with default settings.
// This creates a solid 1-pixel line with butt caps and miter joins.
func DefaultStroke() Stroke {
	return Stroke{
		Width:      1.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
		Dash:       nil,
	}
}

// WithWidth returns a copy of the Stroke with the given width.
func (s Stroke) WithWidth(w float64) Stroke {
	s.Width = w
	return s
}

// WithCap returns a copy of the Stroke with the given line cap style.
func (s Stroke) WithCap(lineCap LineCap) Stroke {
	s.Cap = lineCap
	return s
}

// WithJoin returns a copy of the Stroke with the given line join style.
func (s Stroke) WithJoin(join LineJoin) Stroke {
	s.Join = join
	return s
}

// WithMiterLimit returns a copy of the Stroke with the given miter limit.
// The miter limit controls when miter joins are converted to bevel joins.
// A value of 1.0 effectively disables miter joins.
func (s Stroke) WithMiterLimit(limit float64) Stroke {
	s.MiterLimit = limit
	return s
}

// WithDash returns a copy of the Stroke with the given dash pattern.
// Pass nil to remove dashing and return to solid lines.
func (s Stroke) WithDash(dash *Dash) Stroke {
	if dash == nil {
		s.Dash = nil
	} else {
		s.Dash = dash.Clone()
	}
	return s
}

// WithDashPattern returns a copy of the Stroke with a dash pattern
// created from the given lengths.
//
// Example:
//
//	stroke.WithDashPattern(5, 3) // 5 units dash, 3 units gap
func (s Stroke) WithDashPattern(lengths ...float64) Stroke {
	s.Dash = NewDash(lengths...)
	return s
}

// WithDashOffset returns a copy of the Stroke with the dash offset set.
// If there is no dash pattern, this has no effect.
func (s Stroke) WithDashOffset(offset float64) Stroke {
	if s.Dash != nil {
		s.Dash = s.Dash.WithOffset(offset)
	}
	return s
}

// IsDashed returns true if this stroke has a dash pattern.
func (s Stroke) IsDashed() bool {
	return s.Dash != nil && s.Dash.IsDashed()
}

// Clone creates a deep copy of the Stroke.
func (s Stroke) Clone() Stroke {
	result := s
	if s.Dash != nil {
		result.Dash = s.Dash.Clone()
	}
	return result
}

// Thin returns a thin stroke (0.5 pixels).
func Thin() Stroke {
	return DefaultStroke().WithWidth(0.5)
}

// Thick returns a thick stroke (3 pixels).
func Thick() Stroke {
	return DefaultStroke().WithWidth(3.0)
}

// Bold returns a bold stroke (5 pixels).
func Bold() Stroke {
	return DefaultStroke().WithWidth(5.0)
}

// RoundStroke returns a stroke with round caps and joins.
func RoundStroke() Stroke {
	return DefaultStroke().WithCap(LineCapRound).WithJoin(LineJoinRound)
}

// SquareStroke returns a stroke with square caps.
func SquareStroke() Stroke {
	return DefaultStroke().WithCap(LineCapSquare)
}

// DashedStroke returns a dashed stroke with the given pattern.
func DashedStroke(lengths ...float64) Stroke {
	return DefaultStroke().WithDashPattern(lengths...)
}

// DottedStroke returns a dotted stroke.
// Uses round caps with equal dash and gap (1, 2 pattern with 2px width).
func DottedStroke() Stroke {
	return Stroke{
		Width:      2.0,
		Cap:        LineCapRound,
		Join:       LineJoinRound,
		MiterLimit: 4.0,
		Dash:       NewDash(0.1, 4),
	}
}
