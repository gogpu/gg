package gg

// Painter generates colors for rendering operations.
// For simple use cases, implement Pattern instead — it auto-wraps via PainterFromPaint.
// For maximum performance, implement Painter directly with span-based color generation.
type Painter interface {
	// PaintSpan fills dest with colors for pixels starting at (x, y) for length pixels.
	PaintSpan(dest []RGBA, x, y, length int)
}

// SolidPainter fills all pixels with a single color (fastest path).
type SolidPainter struct {
	Color RGBA
}

// PaintSpan fills the destination buffer with the solid color.
func (p *SolidPainter) PaintSpan(dest []RGBA, _, _ int, length int) {
	for i := 0; i < length && i < len(dest); i++ {
		dest[i] = p.Color
	}
}

// FuncPainter wraps a ColorAt function as a Painter (per-pixel sampling).
type FuncPainter struct {
	Fn func(x, y float64) RGBA
}

// PaintSpan samples the color function at each pixel center.
func (p *FuncPainter) PaintSpan(dest []RGBA, x, y, length int) {
	fy := float64(y) + 0.5
	for i := 0; i < length && i < len(dest); i++ {
		dest[i] = p.Fn(float64(x+i)+0.5, fy)
	}
}

// PainterFromPaint creates the appropriate Painter for the fill side of a Paint.
// Solid paints return SolidPainter (fast). Non-solid paints return FuncPainter
// that samples the fill brush per pixel.
func PainterFromPaint(paint *Paint) Painter {
	return painterFromBrushState(&paint.fill)
}

// PainterFromPaintStroke creates the appropriate Painter for the stroke side of a Paint.
func PainterFromPaintStroke(paint *Paint) Painter {
	return painterFromBrushState(&paint.stroke)
}

// painterFromBrushState creates the appropriate Painter for a brushState.
func painterFromBrushState(s *brushState) Painter {
	// Fast path: inline solid color (no interface dispatch).
	if s.isSolid {
		return &SolidPainter{Color: s.solidColor}
	}
	// Check brush first (takes precedence)
	if s.brush != nil {
		if sb, ok := s.brush.(SolidBrush); ok {
			return &SolidPainter{Color: sb.Color}
		}
		// Check if the Brush itself implements Painter (power-user opt-in)
		if p, ok := s.brush.(Painter); ok {
			return p
		}
		return &FuncPainter{Fn: s.brush.ColorAt}
	}
	// Fall back to pattern
	if s.pattern != nil {
		if sp, ok := s.pattern.(*SolidPattern); ok {
			return &SolidPainter{Color: sp.Color}
		}
		return &FuncPainter{Fn: s.pattern.ColorAt}
	}
	return &SolidPainter{Color: Black}
}
