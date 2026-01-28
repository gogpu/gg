// Package raster provides scanline rasterization for 2D paths.
// This file implements the anti-aliased hairline rendering algorithm.
// Based on tiny-skia's hairline_aa.rs (Android/Skia heritage).
//
// The algorithm uses fixed-point arithmetic for precision:
// - FDot6: 26.6 fixed-point for pixel coordinates (64 subpixel positions)
// - FDot16: 16.16 fixed-point for slopes and interpolation
//
// The key insight is that for hairlines (stroke width <= 1px after transform),
// we can directly compute per-pixel coverage without expanding to a fill path.
// This produces smoother results for thin lines, especially dashed lines.
package raster

// HairlineLineCap specifies the shape of hairline endpoints.
type HairlineLineCap int

const (
	// HairlineCapButt specifies a flat line cap.
	HairlineCapButt HairlineLineCap = iota
	// HairlineCapRound specifies a rounded line cap.
	HairlineCapRound
	// HairlineCapSquare specifies a square line cap.
	HairlineCapSquare
)

// maxHairlineCoord is the maximum coordinate value that can be safely
// converted to FDot16 without overflow. Lines exceeding this are subdivided.
const maxHairlineCoord = 32767.0

// doAntiHairline draws an anti-aliased hairline between two FDot6 coordinates.
// This is the main entry point for the hairline algorithm.
func doAntiHairline(blitter HairlineBlitter, x0, y0, x1, y1 FDot6, coverage float64) {
	// Check for integer NaN (0x80000000) which can't be handled
	if anyBadInts(x0, y0, x1, y1) {
		return
	}

	// Subdivide very long lines to avoid overflow
	dx := Abs6(x1 - x0)
	dy := Abs6(y1 - y0)
	if dx > FDot6(511<<FDot6Shift) || dy > FDot6(511<<FDot6Shift) {
		// Subdivide: use separate shifts to avoid overflow
		hx := (x0 >> 1) + (x1 >> 1)
		hy := (y0 >> 1) + (y1 >> 1)
		doAntiHairline(blitter, x0, y0, hx, hy, coverage)
		doAntiHairline(blitter, hx, hy, x1, y1, coverage)
		return
	}

	if dx > dy {
		// Mostly horizontal
		doHorishHairline(blitter, x0, y0, x1, y1, coverage)
	} else if dy > 0 {
		// Mostly vertical
		doVertishHairline(blitter, x0, y0, x1, y1, coverage)
	}
	// Zero-length lines are skipped
}

// doHorishHairline draws a mostly-horizontal anti-aliased hairline.
// The algorithm iterates over x pixels and distributes coverage between
// two vertically adjacent pixels based on the y fractional position.
func doHorishHairline(blitter HairlineBlitter, x0, y0, x1, y1 FDot6, coverage float64) {
	// Ensure x0 < x1 (left-to-right)
	if x0 > x1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
	}

	istart := FDot6Floor(x0)
	istop := FDot6Ceil(x1)

	// Convert y0 to FDot16 for interpolation
	fy := FDot6ToFDot16(y0)

	// Check for pure horizontal line (special fast case)
	if y0 == y1 {
		// HLine case: constant y
		doHLineHairline(blitter, istart, istop, fy, coverage, x0, x1)
		return
	}

	// Calculate slope: dy/dx in FDot16
	slope := FDot16FastDiv(y1-y0, x1-x0)

	// Adjust fy for subpixel start position
	// (32 - (x0 & 63)) is the distance from x0 to the center of the first pixel
	adjustment := FDot16((int32(32-(x0&FDot6Mask)) * int32(slope)) >> FDot6Shift)
	fy += adjustment + FDot16Half // Add 0.5 to center the line

	// Calculate partial coverage for start and end pixels
	var scaleStart, scaleStop FDot6
	if istop-istart == 1 {
		// Start and end in same pixel
		scaleStart = x1 - x0
		scaleStop = 0
	} else {
		scaleStart = FDot6One - (x0 & FDot6Mask)
		scaleStop = x1 & FDot6Mask
	}

	// Global coverage scale (0-255)
	coverageScale := uint8(coverage * 255)

	// Draw first pixel (partial)
	if scaleStart < FDot6One && istart < istop {
		alpha := FDot6SmallScale(coverageScale, scaleStart)
		blitHairlinePixelY(blitter, istart, fy, alpha)
		fy += slope
		istart++
	}

	// Draw full pixels
	fullSpans := istop - istart
	if scaleStop > 0 {
		fullSpans--
	}
	for x := istart; x < istart+fullSpans; x++ {
		blitHairlinePixelY(blitter, x, fy, coverageScale)
		fy += slope
	}

	// Draw last pixel (partial)
	if scaleStop > 0 && istart+fullSpans < istop {
		alpha := FDot6SmallScale(coverageScale, scaleStop)
		blitHairlinePixelY(blitter, istop-1, fy, alpha)
	}
}

// doHLineHairline draws a pure horizontal hairline (y0 == y1).
func doHLineHairline(blitter HairlineBlitter, istart, istop int, fy FDot16, coverage float64, x0, x1 FDot6) {
	// Add 0.5 to center the line on the pixel
	// Without this, integer coordinates (like y=45) would have a=0,
	// causing all coverage to go to y-1 instead of being distributed
	fy += FDot16Half

	// Clamp fy to non-negative
	if fy < 0 {
		fy = 0
	}

	y := FDot16Floor(fy)
	a := i32ToAlpha(int32(fy >> 8))

	// Calculate partial coverage for start and end
	var scaleStart, scaleStop FDot6
	if istop-istart == 1 {
		scaleStart = x1 - x0
		scaleStop = 0
	} else {
		scaleStart = FDot6One - (x0 & FDot6Mask)
		scaleStop = x1 & FDot6Mask
	}

	coverageScale := uint8(coverage * 255)

	// First pixel
	if scaleStart > 0 && istart < istop {
		alpha := mulAlpha(FDot6SmallScale(a, scaleStart), coverageScale)
		if alpha > 0 {
			blitter.BlitH(istart, y, 1, alpha)
		}
		upperAlpha := mulAlpha(FDot6SmallScale(255-a, scaleStart), coverageScale)
		if upperAlpha > 0 && y > 0 {
			blitter.BlitH(istart, y-1, 1, upperAlpha)
		}
		istart++
	}

	// Middle pixels (full coverage)
	middleCount := istop - istart
	if scaleStop > 0 {
		middleCount--
	}
	if middleCount > 0 {
		lowerAlpha := mulAlpha(a, coverageScale)
		upperAlpha := mulAlpha(255-a, coverageScale)
		if lowerAlpha > 0 {
			blitter.BlitH(istart, y, middleCount, lowerAlpha)
		}
		if upperAlpha > 0 && y > 0 {
			blitter.BlitH(istart, y-1, middleCount, upperAlpha)
		}
	}

	// Last pixel
	if scaleStop > 0 && istart+middleCount < istop {
		alpha := mulAlpha(FDot6SmallScale(a, scaleStop), coverageScale)
		if alpha > 0 {
			blitter.BlitH(istop-1, y, 1, alpha)
		}
		upperAlpha := mulAlpha(FDot6SmallScale(255-a, scaleStop), coverageScale)
		if upperAlpha > 0 && y > 0 {
			blitter.BlitH(istop-1, y-1, 1, upperAlpha)
		}
	}
}

// doVertishHairline draws a mostly-vertical anti-aliased hairline.
// The algorithm iterates over y pixels and distributes coverage between
// two horizontally adjacent pixels based on the x fractional position.
func doVertishHairline(blitter HairlineBlitter, x0, y0, x1, y1 FDot6, coverage float64) {
	// Ensure y0 < y1 (top-to-bottom)
	if y0 > y1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
	}

	istart := FDot6Floor(y0)
	istop := FDot6Ceil(y1)

	// Convert x0 to FDot16 for interpolation
	fx := FDot6ToFDot16(x0)

	// Check for zero-length line
	if y0 == y1 {
		return // zero-length vertical line, nothing to draw
	}

	// Check for pure vertical line
	if x0 == x1 {
		// VLine case: constant x
		doVLineHairline(blitter, istart, istop, fx, coverage, y0, y1)
		return
	}

	// Calculate slope: dx/dy in FDot16
	slope := FDot16FastDiv(x1-x0, y1-y0)

	// Adjust fx for subpixel start position
	adjustment := FDot16((int32(32-(y0&FDot6Mask)) * int32(slope)) >> FDot6Shift)
	fx += adjustment + FDot16Half // Add 0.5 to center the line

	// Calculate partial coverage for start and end pixels
	var scaleStart, scaleStop FDot6
	if istop-istart == 1 {
		// Start and end in same pixel
		scaleStart = y1 - y0
		scaleStop = 0
	} else {
		scaleStart = FDot6One - (y0 & FDot6Mask)
		scaleStop = y1 & FDot6Mask
	}

	// Global coverage scale (0-255)
	coverageScale := uint8(coverage * 255)

	// Draw first pixel (partial)
	if scaleStart < FDot6One && istart < istop {
		alpha := FDot6SmallScale(coverageScale, scaleStart)
		blitHairlinePixelX(blitter, fx, istart, alpha)
		fx += slope
		istart++
	}

	// Draw full pixels
	fullSpans := istop - istart
	if scaleStop > 0 {
		fullSpans--
	}
	for y := istart; y < istart+fullSpans; y++ {
		blitHairlinePixelX(blitter, fx, y, coverageScale)
		fx += slope
	}

	// Draw last pixel (partial)
	if scaleStop > 0 && istart+fullSpans < istop {
		alpha := FDot6SmallScale(coverageScale, scaleStop)
		blitHairlinePixelX(blitter, fx, istop-1, alpha)
	}
}

// doVLineHairline draws a pure vertical hairline (x0 == x1).
func doVLineHairline(blitter HairlineBlitter, istart, istop int, fx FDot16, coverage float64, y0, y1 FDot6) {
	// Add 0.5 to center the line on the pixel
	// Without this, integer coordinates (like x=67) would have a=0,
	// causing all coverage to go to x-1 instead of being distributed
	fx += FDot16Half

	// Clamp fx to non-negative
	if fx < 0 {
		fx = 0
	}

	x := FDot16Floor(fx)
	a := i32ToAlpha(int32(fx >> 8))

	// Calculate partial coverage for start and end
	var scaleStart, scaleStop FDot6
	if istop-istart == 1 {
		scaleStart = y1 - y0
		scaleStop = 0
	} else {
		scaleStart = FDot6One - (y0 & FDot6Mask)
		scaleStop = y1 & FDot6Mask
	}

	coverageScale := uint8(coverage * 255)

	// First pixel
	if scaleStart > 0 && istart < istop {
		alpha := mulAlpha(FDot6SmallScale(a, scaleStart), coverageScale)
		if alpha > 0 {
			blitter.BlitV(x, istart, 1, alpha)
		}
		leftAlpha := mulAlpha(FDot6SmallScale(255-a, scaleStart), coverageScale)
		if leftAlpha > 0 && x > 0 {
			blitter.BlitV(x-1, istart, 1, leftAlpha)
		}
		istart++
	}

	// Middle pixels (full coverage)
	middleCount := istop - istart
	if scaleStop > 0 {
		middleCount--
	}
	if middleCount > 0 {
		rightAlpha := mulAlpha(a, coverageScale)
		leftAlpha := mulAlpha(255-a, coverageScale)
		if rightAlpha > 0 {
			blitter.BlitV(x, istart, middleCount, rightAlpha)
		}
		if leftAlpha > 0 && x > 0 {
			blitter.BlitV(x-1, istart, middleCount, leftAlpha)
		}
	}

	// Last pixel
	if scaleStop > 0 && istart+middleCount < istop {
		alpha := mulAlpha(FDot6SmallScale(a, scaleStop), coverageScale)
		if alpha > 0 {
			blitter.BlitV(x, istop-1, 1, alpha)
		}
		leftAlpha := mulAlpha(FDot6SmallScale(255-a, scaleStop), coverageScale)
		if leftAlpha > 0 && x > 0 {
			blitter.BlitV(x-1, istop-1, 1, leftAlpha)
		}
	}
}

// blitHairlinePixelY blits a pixel with Y fractional coverage.
// Used for mostly-horizontal lines where the fractional Y position
// determines how coverage is split between two vertically adjacent pixels.
func blitHairlinePixelY(blitter HairlineBlitter, x int, fy FDot16, alpha uint8) {
	if alpha == 0 {
		return
	}

	// Clamp fy to non-negative
	if fy < 0 {
		fy = 0
	}

	y := FDot16Floor(fy)
	frac := i32ToAlpha(int32(fy >> 8)) // Extract 8-bit fraction

	// Distribute alpha between two pixels based on Y fraction
	alphaLower := mulAlpha(alpha, frac)
	alphaUpper := mulAlpha(alpha, 255-frac)

	blitter.BlitAntiV2(x, y-1, alphaUpper, alphaLower)
}

// blitHairlinePixelX blits a pixel with X fractional coverage.
// Used for mostly-vertical lines where the fractional X position
// determines how coverage is split between two horizontally adjacent pixels.
func blitHairlinePixelX(blitter HairlineBlitter, fx FDot16, y int, alpha uint8) {
	if alpha == 0 {
		return
	}

	// Clamp fx to non-negative
	if fx < 0 {
		fx = 0
	}

	x := FDot16Floor(fx)
	frac := i32ToAlpha(int32(fx >> 8)) // Extract 8-bit fraction

	// Distribute alpha between two pixels based on X fraction
	alphaRight := mulAlpha(alpha, frac)
	alphaLeft := mulAlpha(alpha, 255-frac)

	blitter.BlitAntiH2(x-1, y, alphaLeft, alphaRight)
}

// anyBadInts checks if any of the values are "bad" (NaN equivalent in fixed-point).
// Returns true if any value is the minimum int32 (can't be negated).
func anyBadInts(a, b, c, d FDot6) bool {
	const minInt32 = FDot6(-1 << 31)
	return a == minInt32 || b == minInt32 || c == minInt32 || d == minInt32
}

// i32ToAlpha extracts the lower 8 bits as an alpha value.
// The mask ensures the result is always in [0, 255].
//
//nolint:gosec // Mask guarantees value fits in uint8
func i32ToAlpha(a int32) uint8 {
	return uint8(a & 0xFF)
}

// mulAlpha multiplies two alpha values (0-255) and returns the result.
// The computation is (a * b) >> 8 which is always in [0, 255].
//
//nolint:gosec // Product of two uint8 values shifted right by 8 always fits in uint8
func mulAlpha(a, b uint8) uint8 {
	return uint8((int(a) * int(b)) >> 8)
}

// HairlinePoint represents a point for hairline rendering.
type HairlinePoint struct {
	X, Y float64
}

// StrokeHairlineAA draws an anti-aliased hairline path.
// The path is a sequence of points representing line segments.
// Coverage controls the global opacity (0.0 to 1.0).
//
//nolint:gosec // Bounds checked at start of function
func StrokeHairlineAA(blitter HairlineBlitter, points []HairlinePoint, lineCap HairlineLineCap, coverage float64) {
	numPoints := len(points)
	if numPoints < 2 {
		return
	}

	for i := 0; i < numPoints-1; i++ {
		// Safe: i < numPoints-1, so i+1 < numPoints
		p0 := points[i]
		p1 := points[i+1]

		// Clip to safe range
		if !clipHairlineToSafeRange(&p0, &p1) {
			continue
		}

		// Convert to fixed-point
		fx0 := FloatToFDot6(p0.X)
		fy0 := FloatToFDot6(p0.Y)
		fx1 := FloatToFDot6(p1.X)
		fy1 := FloatToFDot6(p1.Y)

		// Draw with caps for first and last segment
		isFirst := (i == 0)
		isLast := (i == numPoints-2)

		// Apply cap extension if needed
		if lineCap != HairlineCapButt {
			extendHairlineForCap(&fx0, &fy0, &fx1, &fy1, lineCap, isFirst, isLast)
		}

		doAntiHairline(blitter, fx0, fy0, fx1, fy1, coverage)
	}
}

// clipHairlineToSafeRange clips a line segment to the safe coordinate range.
// Returns false if the segment is completely outside the range.
func clipHairlineToSafeRange(p0, p1 *HairlinePoint) bool {
	// Use a bounds slightly inside the maximum to ensure we can safely
	// perform fixed-point arithmetic without overflow
	const bound = maxHairlineCoord - 1.0

	// Quick reject if both endpoints are outside
	if (p0.X < -bound && p1.X < -bound) || (p0.X > bound && p1.X > bound) {
		return false
	}
	if (p0.Y < -bound && p1.Y < -bound) || (p0.Y > bound && p1.Y > bound) {
		return false
	}

	// Clip to bounds using Cohen-Sutherland-like algorithm
	// For simplicity, we just clamp coordinates (this can cause visual artifacts
	// for lines that cross the boundary, but the boundary is far enough that
	// this shouldn't matter in practice)
	p0.X = clampToHairlineBounds(p0.X)
	p0.Y = clampToHairlineBounds(p0.Y)
	p1.X = clampToHairlineBounds(p1.X)
	p1.Y = clampToHairlineBounds(p1.Y)

	return true
}

// clampToHairlineBounds clamps a float64 value to the safe hairline range.
func clampToHairlineBounds(v float64) float64 {
	const bound = maxHairlineCoord - 1.0
	if v < -bound {
		return -bound
	}
	if v > bound {
		return bound
	}
	return v
}
