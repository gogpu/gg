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

// PainterFromPaint creates the appropriate Painter for a Paint.
// Solid paints return SolidPainter (fast). Non-solid paints return FuncPainter
// that samples paint.ColorAt per pixel.
func PainterFromPaint(paint *Paint) Painter {
	// Check Brush first (takes precedence)
	if paint.Brush != nil {
		if sb, ok := paint.Brush.(SolidBrush); ok {
			return &SolidPainter{Color: sb.Color}
		}
		// Check if the Brush itself implements Painter (power-user opt-in)
		if p, ok := paint.Brush.(Painter); ok {
			return p
		}
		return &FuncPainter{Fn: paint.Brush.ColorAt}
	}
	// Fall back to Pattern
	if paint.Pattern != nil {
		if sp, ok := paint.Pattern.(*SolidPattern); ok {
			return &SolidPainter{Color: sp.Color}
		}
		return &FuncPainter{Fn: paint.Pattern.ColorAt}
	}
	return &SolidPainter{Color: Black}
}
