// Package raster provides scanline rasterization for 2D paths.
// This file implements line cap handling for hairline rendering.
package raster

import "math"

// capExtensionRound is the extension for round caps (approximately PI/8 pixels).
// This is an approximation that provides visually pleasing results.
const capExtensionRound = 0.39

// capExtensionSquare is the extension for square caps (half pixel).
const capExtensionSquare = 0.5

// extendHairlineForCap extends line endpoints based on cap style.
// This is applied to the fixed-point coordinates for precise control.
//
// Parameters:
//   - x0, y0: Start point in FDot6 (modified in place)
//   - x1, y1: End point in FDot6 (modified in place)
//   - lineCap: Line cap style
//   - extendStart: Whether to extend the start point
//   - extendEnd: Whether to extend the end point
func extendHairlineForCap(x0, y0, x1, y1 *FDot6, lineCap HairlineLineCap, extendStart, extendEnd bool) {
	if lineCap == HairlineCapButt {
		return // No extension for butt caps
	}

	// Calculate direction vector in float64 for precision
	dx := FDot6ToFloat(*x1 - *x0)
	dy := FDot6ToFloat(*y1 - *y0)
	length := math.Hypot(dx, dy)

	if length < 1e-10 {
		return // Zero-length line, can't determine direction
	}

	// Normalize direction
	dx /= length
	dy /= length

	// Determine extension amount
	var extend float64
	switch lineCap {
	case HairlineCapRound:
		extend = capExtensionRound
	case HairlineCapSquare:
		extend = capExtensionSquare
	default:
		return
	}

	// Convert extension to FDot6
	extFDot6 := FloatToFDot6(extend)
	dxFDot6 := FDot6(float64(extFDot6) * dx)
	dyFDot6 := FDot6(float64(extFDot6) * dy)

	// Extend endpoints
	if extendStart {
		*x0 -= dxFDot6
		*y0 -= dyFDot6
	}
	if extendEnd {
		*x1 += dxFDot6
		*y1 += dyFDot6
	}
}
