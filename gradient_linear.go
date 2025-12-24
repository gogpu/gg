package gg

// LinearGradientBrush represents a linear color transition between two points.
// It implements the Brush interface and supports multiple color stops,
// proper sRGB color interpolation, and configurable extend modes.
//
// LinearGradientBrush follows the vello/peniko gradient model, providing
// professional-quality gradients for 2D graphics.
//
// Example:
//
//	gradient := gg.NewLinearGradientBrush(0, 0, 100, 0).
//	    AddColorStop(0, gg.Red).
//	    AddColorStop(0.5, gg.Yellow).
//	    AddColorStop(1, gg.Blue)
//	ctx.SetFillBrush(gradient)
type LinearGradientBrush struct {
	Start  Point       // Start point of the gradient
	End    Point       // End point of the gradient
	Stops  []ColorStop // Color stops defining the gradient
	Extend ExtendMode  // How gradient extends beyond bounds
}

// NewLinearGradientBrush creates a new linear gradient from (x0, y0) to (x1, y1).
func NewLinearGradientBrush(x0, y0, x1, y1 float64) *LinearGradientBrush {
	return &LinearGradientBrush{
		Start:  Point{X: x0, Y: y0},
		End:    Point{X: x1, Y: y1},
		Stops:  nil,
		Extend: ExtendPad,
	}
}

// AddColorStop adds a color stop at the specified offset.
// Offset should be in the range [0, 1].
// Returns the gradient for method chaining.
func (g *LinearGradientBrush) AddColorStop(offset float64, c RGBA) *LinearGradientBrush {
	g.Stops = append(g.Stops, ColorStop{Offset: offset, Color: c})
	return g
}

// SetExtend sets the extend mode for the gradient.
// Returns the gradient for method chaining.
func (g *LinearGradientBrush) SetExtend(mode ExtendMode) *LinearGradientBrush {
	g.Extend = mode
	return g
}

// brushMarker implements the Brush interface marker.
func (LinearGradientBrush) brushMarker() {}

// ColorAt returns the color at the given point.
// Implements the Pattern and Brush interfaces.
func (g *LinearGradientBrush) ColorAt(x, y float64) RGBA {
	// Handle zero-length gradient (start == end)
	dx := g.End.X - g.Start.X
	dy := g.End.Y - g.Start.Y
	lengthSq := dx*dx + dy*dy

	if lengthSq == 0 {
		return firstStopColor(g.Stops)
	}

	// Project point onto the gradient line
	// t = dot(P - Start, End - Start) / |End - Start|^2
	px := x - g.Start.X
	py := y - g.Start.Y
	t := (px*dx + py*dy) / lengthSq

	return colorAtOffset(g.Stops, t, g.Extend)
}

// firstStopColor returns the first stop's color or Transparent if empty.
func firstStopColor(stops []ColorStop) RGBA {
	if len(stops) == 0 {
		return Transparent
	}
	// Find stop with minimum offset
	sorted := sortStops(stops)
	return sorted[0].Color
}
