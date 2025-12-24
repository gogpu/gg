package gg

import "math"

// SweepGradientBrush represents an angular (conic) color transition around a center point.
// Colors sweep from StartAngle to EndAngle. Also known as a conic gradient.
// It implements the Brush interface and supports multiple color stops,
// proper sRGB color interpolation, and configurable extend modes.
//
// SweepGradientBrush follows the vello/peniko gradient model, providing
// professional-quality angular gradients for effects like color wheels,
// pie charts, and radar displays.
//
// Example:
//
//	// Color wheel
//	wheel := gg.NewSweepGradientBrush(50, 50, 0).
//	    AddColorStop(0, gg.Red).
//	    AddColorStop(0.166, gg.Yellow).
//	    AddColorStop(0.333, gg.Green).
//	    AddColorStop(0.5, gg.Cyan).
//	    AddColorStop(0.666, gg.Blue).
//	    AddColorStop(0.833, gg.Magenta).
//	    AddColorStop(1, gg.Red)
type SweepGradientBrush struct {
	Center     Point       // Center of the sweep
	StartAngle float64     // Start angle in radians
	EndAngle   float64     // End angle in radians (if 0, defaults to StartAngle + 2*Pi)
	Stops      []ColorStop // Color stops defining the gradient
	Extend     ExtendMode  // How gradient extends beyond bounds
}

// NewSweepGradientBrush creates a new sweep (conic) gradient centered at (cx, cy).
// startAngle is the angle where the gradient begins (in radians).
// The gradient sweeps a full 360 degrees by default.
func NewSweepGradientBrush(cx, cy, startAngle float64) *SweepGradientBrush {
	return &SweepGradientBrush{
		Center:     Point{X: cx, Y: cy},
		StartAngle: startAngle,
		EndAngle:   startAngle + 2*math.Pi, // Full rotation by default
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
func (g *SweepGradientBrush) AddColorStop(offset float64, c RGBA) *SweepGradientBrush {
	g.Stops = append(g.Stops, ColorStop{Offset: offset, Color: c})
	return g
}

// SetExtend sets the extend mode for the gradient.
// Returns the gradient for method chaining.
func (g *SweepGradientBrush) SetExtend(mode ExtendMode) *SweepGradientBrush {
	g.Extend = mode
	return g
}

// brushMarker implements the Brush interface marker.
func (SweepGradientBrush) brushMarker() {}

// ColorAt returns the color at the given point.
// Implements the Pattern and Brush interfaces.
func (g *SweepGradientBrush) ColorAt(x, y float64) RGBA {
	// Handle point at center (undefined angle)
	dx := x - g.Center.X
	dy := y - g.Center.Y
	if dx == 0 && dy == 0 {
		return firstStopColor(g.Stops)
	}

	// Calculate angle from center to point
	// atan2 returns angle in range [-Pi, Pi]
	angle := math.Atan2(dy, dx)

	// Normalize angle relative to start angle
	t := g.angleToT(angle)

	return colorAtOffset(g.Stops, t, g.Extend)
}

// angleToT converts an angle to a gradient parameter t in [0, 1].
func (g *SweepGradientBrush) angleToT(angle float64) float64 {
	sweepRange := g.EndAngle - g.StartAngle

	// Handle zero sweep (degenerate case)
	if sweepRange == 0 {
		return 0
	}

	// Normalize angle to be relative to start angle
	relativeAngle := angle - g.StartAngle

	// Wrap to [0, 2*Pi) for positive sweep or (-2*Pi, 0] for negative sweep
	relativeAngle = normalizeAngle(relativeAngle, sweepRange)

	// Map to t in [0, 1]
	t := relativeAngle / sweepRange

	return t
}

// normalizeAngle normalizes an angle relative to a sweep direction.
func normalizeAngle(angle float64, sweepRange float64) float64 {
	twoPi := 2 * math.Pi

	if sweepRange > 0 {
		// Positive sweep: normalize to [0, 2*Pi)
		for angle < 0 {
			angle += twoPi
		}
		for angle >= twoPi {
			angle -= twoPi
		}
	} else {
		// Negative sweep: normalize to (-2*Pi, 0]
		for angle > 0 {
			angle -= twoPi
		}
		for angle <= -twoPi {
			angle += twoPi
		}
	}

	return angle
}
