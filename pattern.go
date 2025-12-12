package gg

// Pattern represents a fill or stroke pattern.
type Pattern interface {
	// ColorAt returns the color at the given point.
	ColorAt(x, y float64) RGBA
}

// SolidPattern represents a solid color pattern.
type SolidPattern struct {
	Color RGBA
}

// NewSolidPattern creates a solid color pattern.
func NewSolidPattern(color RGBA) *SolidPattern {
	return &SolidPattern{Color: color}
}

// ColorAt implements Pattern.
func (p *SolidPattern) ColorAt(x, y float64) RGBA {
	return p.Color
}
