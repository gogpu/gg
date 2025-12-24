package gg

import "math"

// RadialGradientBrush represents a radial color transition.
// Colors radiate from a focal point within a circle defined by center and end radius.
// It implements the Brush interface and supports multiple color stops,
// proper sRGB color interpolation, and configurable extend modes.
//
// RadialGradientBrush follows the vello/peniko gradient model, supporting
// both simple radial gradients (focus at center) and focal gradients
// (focus offset from center for spotlight effects).
//
// Example:
//
//	// Simple radial gradient
//	gradient := gg.NewRadialGradientBrush(50, 50, 0, 50).
//	    AddColorStop(0, gg.White).
//	    AddColorStop(1, gg.Black)
//
//	// Focal gradient (spotlight effect)
//	spotlight := gg.NewRadialGradientBrush(50, 50, 0, 50).
//	    SetFocus(30, 30).
//	    AddColorStop(0, gg.White).
//	    AddColorStop(1, gg.Black)
type RadialGradientBrush struct {
	Center      Point       // Center of the gradient circle
	Focus       Point       // Focal point (can differ from center)
	StartRadius float64     // Inner radius where gradient begins (t=0)
	EndRadius   float64     // Outer radius where gradient ends (t=1)
	Stops       []ColorStop // Color stops defining the gradient
	Extend      ExtendMode  // How gradient extends beyond bounds
}

// NewRadialGradientBrush creates a new radial gradient.
// The gradient transitions from startRadius to endRadius around (cx, cy).
// Focus defaults to center.
func NewRadialGradientBrush(cx, cy, startRadius, endRadius float64) *RadialGradientBrush {
	center := Point{X: cx, Y: cy}
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
// A focal point different from center creates an asymmetric gradient.
// Returns the gradient for method chaining.
func (g *RadialGradientBrush) SetFocus(fx, fy float64) *RadialGradientBrush {
	g.Focus = Point{X: fx, Y: fy}
	return g
}

// AddColorStop adds a color stop at the specified offset.
// Offset should be in the range [0, 1].
// Returns the gradient for method chaining.
func (g *RadialGradientBrush) AddColorStop(offset float64, c RGBA) *RadialGradientBrush {
	g.Stops = append(g.Stops, ColorStop{Offset: offset, Color: c})
	return g
}

// SetExtend sets the extend mode for the gradient.
// Returns the gradient for method chaining.
func (g *RadialGradientBrush) SetExtend(mode ExtendMode) *RadialGradientBrush {
	g.Extend = mode
	return g
}

// brushMarker implements the Brush interface marker.
func (RadialGradientBrush) brushMarker() {}

// ColorAt returns the color at the given point.
// Implements the Pattern and Brush interfaces.
func (g *RadialGradientBrush) ColorAt(x, y float64) RGBA {
	// Handle degenerate gradient (zero radius difference)
	radiusDiff := g.EndRadius - g.StartRadius
	if radiusDiff == 0 {
		return firstStopColor(g.Stops)
	}

	t := g.computeT(x, y)
	return colorAtOffset(g.Stops, t, g.Extend)
}

// computeT calculates the gradient parameter t for a point.
// For simple case (focus == center): t = (distance - startRadius) / (endRadius - startRadius)
// For focal gradient: uses ray-circle intersection.
func (g *RadialGradientBrush) computeT(x, y float64) float64 {
	// Simple case: focus at center
	if g.Focus.X == g.Center.X && g.Focus.Y == g.Center.Y {
		return g.computeTSimple(x, y)
	}

	// Complex case: focal gradient
	return g.computeTFocal(x, y)
}

// computeTSimple calculates t for the simple case where focus equals center.
func (g *RadialGradientBrush) computeTSimple(x, y float64) float64 {
	dx := x - g.Center.X
	dy := y - g.Center.Y
	distance := math.Sqrt(dx*dx + dy*dy)

	radiusDiff := g.EndRadius - g.StartRadius
	if radiusDiff == 0 {
		return 0
	}

	return (distance - g.StartRadius) / radiusDiff
}

// computeTFocal calculates t for focal gradients (focus != center).
// This solves a ray-circle intersection problem.
func (g *RadialGradientBrush) computeTFocal(x, y float64) float64 {
	// Direction from focus to point
	dx := x - g.Focus.X
	dy := y - g.Focus.Y

	// Vector from focus to center
	fx := g.Center.X - g.Focus.X
	fy := g.Center.Y - g.Focus.Y

	// Solve quadratic for ray-circle intersection
	// Ray: P(t) = Focus + t * (Point - Focus)
	// Circle: |P - Center|^2 = EndRadius^2
	//
	// |Focus + t*(dx,dy) - Center|^2 = EndRadius^2
	// |t*(dx,dy) - (fx,fy)|^2 = EndRadius^2
	// t^2*(dx^2+dy^2) - 2t*(dx*fx+dy*fy) + (fx^2+fy^2) - EndRadius^2 = 0

	a := dx*dx + dy*dy
	b := -2 * (dx*fx + dy*fy)
	c := fx*fx + fy*fy - g.EndRadius*g.EndRadius

	// Handle degenerate case (point at focus)
	if a == 0 {
		return 0
	}

	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		// Point is outside the gradient circle
		return 1
	}

	// We want the positive root (forward along ray)
	sqrtD := math.Sqrt(discriminant)
	t1 := (-b - sqrtD) / (2 * a)
	t2 := (-b + sqrtD) / (2 * a)

	// Choose the appropriate root
	// t > 0 means the intersection is in the direction from focus to point
	var t float64
	switch {
	case t1 > 0 && t2 > 0:
		t = math.Min(t1, t2)
	case t1 > 0:
		t = t1
	case t2 > 0:
		t = t2
	default:
		return 0
	}

	// The gradient parameter is the ratio of actual distance to intersection distance
	pointDist := math.Sqrt(a) // Distance from focus to point
	intersectDist := t * pointDist

	if intersectDist == 0 {
		return 0
	}

	// Map to gradient space, accounting for start radius
	gradientT := pointDist / intersectDist
	return gradientT
}
