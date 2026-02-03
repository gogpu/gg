package recording

import (
	"github.com/gogpu/gg"
)

// Brush represents a fill/stroke style for recording commands.
// This is a sealed interface - only types in this package implement it.
//
// Unlike gg.Brush which is designed for immediate-mode rendering with ColorAt,
// recording.Brush stores the brush definition for later playback to different
// backends (raster, PDF, SVG).
type Brush interface {
	// brushMarker is an unexported method that seals this interface.
	brushMarker()
}

// SolidBrush is a solid color brush.
type SolidBrush struct {
	Color gg.RGBA
}

func (SolidBrush) brushMarker() {}

// NewSolidBrush creates a solid color brush.
func NewSolidBrush(color gg.RGBA) SolidBrush {
	return SolidBrush{Color: color}
}

// LinearGradientBrush is a linear gradient brush.
// Colors transition linearly from Start to End.
type LinearGradientBrush struct {
	Start  gg.Point       // Start point of the gradient
	End    gg.Point       // End point of the gradient
	Stops  []GradientStop // Color stops defining the gradient
	Extend ExtendMode     // How gradient extends beyond bounds
}

func (LinearGradientBrush) brushMarker() {}

// NewLinearGradientBrush creates a new linear gradient brush.
func NewLinearGradientBrush(x0, y0, x1, y1 float64) *LinearGradientBrush {
	return &LinearGradientBrush{
		Start:  gg.Point{X: x0, Y: y0},
		End:    gg.Point{X: x1, Y: y1},
		Stops:  nil,
		Extend: ExtendPad,
	}
}

// AddColorStop adds a color stop at the specified offset.
// Offset should be in the range [0, 1].
// Returns the gradient for method chaining.
func (g *LinearGradientBrush) AddColorStop(offset float64, color gg.RGBA) *LinearGradientBrush {
	g.Stops = append(g.Stops, GradientStop{Offset: offset, Color: color})
	return g
}

// SetExtend sets the extend mode for the gradient.
// Returns the gradient for method chaining.
func (g *LinearGradientBrush) SetExtend(mode ExtendMode) *LinearGradientBrush {
	g.Extend = mode
	return g
}

// RadialGradientBrush is a radial gradient brush.
// Colors radiate from Focus within a circle defined by Center and EndRadius.
type RadialGradientBrush struct {
	Center      gg.Point       // Center of the gradient circle
	Focus       gg.Point       // Focal point (can differ from center)
	StartRadius float64        // Inner radius where gradient begins (t=0)
	EndRadius   float64        // Outer radius where gradient ends (t=1)
	Stops       []GradientStop // Color stops defining the gradient
	Extend      ExtendMode     // How gradient extends beyond bounds
}

func (RadialGradientBrush) brushMarker() {}

// NewRadialGradientBrush creates a new radial gradient brush.
// The gradient transitions from startRadius to endRadius around (cx, cy).
// Focus defaults to center.
func NewRadialGradientBrush(cx, cy, startRadius, endRadius float64) *RadialGradientBrush {
	center := gg.Point{X: cx, Y: cy}
	return &RadialGradientBrush{
		Center:      center,
		Focus:       center, // Default focus at center
		StartRadius: startRadius,
		EndRadius:   endRadius,
		Stops:       nil,
		Extend:      ExtendPad,
	}
}

// SetFocus sets the focal point of the gradient.
// Returns the gradient for method chaining.
func (g *RadialGradientBrush) SetFocus(fx, fy float64) *RadialGradientBrush {
	g.Focus = gg.Point{X: fx, Y: fy}
	return g
}

// AddColorStop adds a color stop at the specified offset.
// Offset should be in the range [0, 1].
// Returns the gradient for method chaining.
func (g *RadialGradientBrush) AddColorStop(offset float64, color gg.RGBA) *RadialGradientBrush {
	g.Stops = append(g.Stops, GradientStop{Offset: offset, Color: color})
	return g
}

// SetExtend sets the extend mode for the gradient.
// Returns the gradient for method chaining.
func (g *RadialGradientBrush) SetExtend(mode ExtendMode) *RadialGradientBrush {
	g.Extend = mode
	return g
}

// SweepGradientBrush is an angular (conic) gradient brush.
// Colors sweep from StartAngle to EndAngle around Center.
type SweepGradientBrush struct {
	Center     gg.Point       // Center of the sweep
	StartAngle float64        // Start angle in radians
	EndAngle   float64        // End angle in radians
	Stops      []GradientStop // Color stops defining the gradient
	Extend     ExtendMode     // How gradient extends beyond bounds
}

func (SweepGradientBrush) brushMarker() {}

// NewSweepGradientBrush creates a new sweep (conic) gradient brush.
// The gradient sweeps a full 360 degrees by default.
func NewSweepGradientBrush(cx, cy, startAngle float64) *SweepGradientBrush {
	const twoPi = 6.283185307179586 // 2 * math.Pi
	return &SweepGradientBrush{
		Center:     gg.Point{X: cx, Y: cy},
		StartAngle: startAngle,
		EndAngle:   startAngle + twoPi,
		Stops:      nil,
		Extend:     ExtendPad,
	}
}

// SetEndAngle sets the end angle of the sweep.
// Returns the gradient for method chaining.
func (g *SweepGradientBrush) SetEndAngle(endAngle float64) *SweepGradientBrush {
	g.EndAngle = endAngle
	return g
}

// AddColorStop adds a color stop at the specified offset.
// Offset should be in the range [0, 1].
// Returns the gradient for method chaining.
func (g *SweepGradientBrush) AddColorStop(offset float64, color gg.RGBA) *SweepGradientBrush {
	g.Stops = append(g.Stops, GradientStop{Offset: offset, Color: color})
	return g
}

// SetExtend sets the extend mode for the gradient.
// Returns the gradient for method chaining.
func (g *SweepGradientBrush) SetExtend(mode ExtendMode) *SweepGradientBrush {
	g.Extend = mode
	return g
}

// PatternBrush is an image pattern brush.
// The pattern can be repeated, reflected, or clamped.
type PatternBrush struct {
	Image     ImageRef   // Reference to the pattern image in the pool
	Repeat    RepeatMode // How the pattern repeats
	Transform gg.Matrix  // Transform applied to the pattern
}

func (PatternBrush) brushMarker() {}

// NewPatternBrush creates a new pattern brush with the given image reference.
func NewPatternBrush(imageRef ImageRef) *PatternBrush {
	return &PatternBrush{
		Image:     imageRef,
		Repeat:    RepeatBoth,
		Transform: gg.Identity(),
	}
}

// SetRepeat sets the repeat mode for the pattern.
// Returns the brush for method chaining.
func (b *PatternBrush) SetRepeat(mode RepeatMode) *PatternBrush {
	b.Repeat = mode
	return b
}

// SetTransform sets the transform matrix for the pattern.
// Returns the brush for method chaining.
func (b *PatternBrush) SetTransform(m gg.Matrix) *PatternBrush {
	b.Transform = m
	return b
}

// GradientStop defines a color stop in a gradient.
type GradientStop struct {
	Offset float64 // Position in gradient, 0.0 to 1.0
	Color  gg.RGBA // Color at this position
}

// ExtendMode defines how gradients extend beyond their defined bounds.
type ExtendMode int

const (
	// ExtendPad extends edge colors beyond bounds (default behavior).
	ExtendPad ExtendMode = iota
	// ExtendRepeat repeats the gradient pattern.
	ExtendRepeat
	// ExtendReflect mirrors the gradient pattern.
	ExtendReflect
)

// RepeatMode defines how patterns repeat.
type RepeatMode int

const (
	// RepeatBoth repeats in both X and Y directions.
	RepeatBoth RepeatMode = iota
	// RepeatX repeats only in X direction.
	RepeatX
	// RepeatY repeats only in Y direction.
	RepeatY
	// RepeatNone does not repeat (clamps to edge).
	RepeatNone
)

// BrushFromGG converts a gg.Brush to a recording.Brush.
// This extracts the brush definition for storage in a recording.
func BrushFromGG(b gg.Brush) Brush {
	switch brush := b.(type) {
	case gg.SolidBrush:
		return SolidBrush{Color: brush.Color}
	case *gg.LinearGradientBrush:
		return linearGradientFromGG(brush)
	case *gg.RadialGradientBrush:
		return radialGradientFromGG(brush)
	case *gg.SweepGradientBrush:
		return sweepGradientFromGG(brush)
	default:
		// For unknown brush types, sample at origin and use as solid color
		// This is a fallback that preserves color but loses patterns
		color := b.ColorAt(0, 0)
		return SolidBrush{Color: color}
	}
}

// linearGradientFromGG converts a gg.LinearGradientBrush to recording.LinearGradientBrush.
func linearGradientFromGG(g *gg.LinearGradientBrush) *LinearGradientBrush {
	stops := make([]GradientStop, len(g.Stops))
	for i, stop := range g.Stops {
		stops[i] = GradientStop{Offset: stop.Offset, Color: stop.Color}
	}
	return &LinearGradientBrush{
		Start:  g.Start,
		End:    g.End,
		Stops:  stops,
		Extend: ExtendMode(g.Extend),
	}
}

// radialGradientFromGG converts a gg.RadialGradientBrush to recording.RadialGradientBrush.
func radialGradientFromGG(g *gg.RadialGradientBrush) *RadialGradientBrush {
	stops := make([]GradientStop, len(g.Stops))
	for i, stop := range g.Stops {
		stops[i] = GradientStop{Offset: stop.Offset, Color: stop.Color}
	}
	return &RadialGradientBrush{
		Center:      g.Center,
		Focus:       g.Focus,
		StartRadius: g.StartRadius,
		EndRadius:   g.EndRadius,
		Stops:       stops,
		Extend:      ExtendMode(g.Extend),
	}
}

// sweepGradientFromGG converts a gg.SweepGradientBrush to recording.SweepGradientBrush.
func sweepGradientFromGG(g *gg.SweepGradientBrush) *SweepGradientBrush {
	stops := make([]GradientStop, len(g.Stops))
	for i, stop := range g.Stops {
		stops[i] = GradientStop{Offset: stop.Offset, Color: stop.Color}
	}
	return &SweepGradientBrush{
		Center:     g.Center,
		StartAngle: g.StartAngle,
		EndAngle:   g.EndAngle,
		Stops:      stops,
		Extend:     ExtendMode(g.Extend),
	}
}
