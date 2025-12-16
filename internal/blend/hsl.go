// Package blend implements HSL-based non-separable blend modes.
//
// This file implements the non-separable blend modes (Hue, Saturation, Color, Luminosity)
// per W3C Compositing and Blending Level 1 specification.
//
// These modes require color space conversion and operate on the entire RGB triplet
// rather than individual channels.
//
// References:
//   - W3C Compositing and Blending Level 1: https://www.w3.org/TR/compositing-1/
//   - Section 8: Non-separable blend modes
package blend

import "math"

// Lum returns the luminance of a color using BT.601 coefficients.
// Formula: Lum(r, g, b) = 0.30*r + 0.59*g + 0.11*b
//
// Parameters are normalized float32 values in [0, 1].
func Lum(r, g, b float32) float32 {
	return 0.30*r + 0.59*g + 0.11*b
}

// Sat returns the saturation (max - min) of a color.
// Formula: Sat(r, g, b) = max(r, g, b) - min(r, g, b)
//
// Parameters are normalized float32 values in [0, 1].
func Sat(r, g, b float32) float32 {
	return max3(r, g, b) - min3(r, g, b)
}

// ClipColor clips color components to [0,1] while preserving luminance.
// This implements the W3C spec ClipColor algorithm.
//
// If any component is outside [0,1], the color is scaled towards the luminance
// to bring it back into range while maintaining the relative relationships.
func ClipColor(r, g, b float32) (float32, float32, float32) {
	l := Lum(r, g, b)
	n := min3(r, g, b)
	x := max3(r, g, b)

	// If minimum component is negative, scale towards luminance
	if n < 0 {
		r = l + (r-l)*l/(l-n)
		g = l + (g-l)*l/(l-n)
		b = l + (b-l)*l/(l-n)
	}

	// If maximum component exceeds 1, scale towards luminance
	if x > 1 {
		r = l + (r-l)*(1-l)/(x-l)
		g = l + (g-l)*(1-l)/(x-l)
		b = l + (b-l)*(1-l)/(x-l)
	}

	return r, g, b
}

// SetLum sets the luminance of a color while preserving saturation and hue.
// This implements the W3C spec SetLum algorithm.
//
// The algorithm adjusts the color's luminance to the target value l,
// then clips the result to [0,1] while maintaining relative relationships.
func SetLum(r, g, b, l float32) (float32, float32, float32) {
	d := l - Lum(r, g, b)
	r += d
	g += d
	b += d
	return ClipColor(r, g, b)
}

// SetSat sets the saturation of a color while preserving hue and luminosity relationships.
// This implements the W3C spec SetSat algorithm.
//
// The algorithm works by identifying min, mid, max components and scaling them
// to achieve the target saturation while preserving their relative ordering.
func SetSat(r, g, b, s float32) (float32, float32, float32) {
	// Find which component is which (min, mid, max)
	minPtr, midPtr, maxPtr := sortRGB(&r, &g, &b)

	minVal := *minPtr
	midVal := *midPtr
	maxVal := *maxPtr

	// Apply SetSat per W3C spec
	if maxVal > minVal {
		// Non-grayscale: scale to new saturation
		*midPtr = ((midVal - minVal) * s) / (maxVal - minVal)
		*maxPtr = s
		*minPtr = 0
	} else {
		// Grayscale: all components equal, can't meaningfully set saturation
		// Keep the luminosity by returning the original equal values
		// Saturation remains 0
		*minPtr = minVal
		*midPtr = midVal
		*maxPtr = maxVal
	}

	return r, g, b
}

// sortRGB returns pointers to r, g, b sorted by value (minPtr, midPtr, maxPtr).
func sortRGB(r, g, b *float32) (minPtr, midPtr, maxPtr *float32) {
	switch {
	case *r <= *g && *g <= *b:
		// r <= g <= b
		return r, g, b
	case *r <= *b && *b <= *g:
		// r <= b < g
		return r, b, g
	case *b <= *r && *r <= *g:
		// b < r <= g
		return b, r, g
	case *g <= *r && *r <= *b:
		// g < r <= b
		return g, r, b
	case *g <= *b && *b <= *r:
		// g <= b < r
		return g, b, r
	default:
		// b < g < r
		return b, g, r
	}
}

// hslBlendHue uses the hue of the source with saturation and luminosity of the backdrop.
// Formula: SetLum(SetSat(Cs, Sat(Cb)), Lum(Cb))
//
// This creates a color with the hue of the source and the saturation and luminosity
// of the backdrop.
func hslBlendHue(sr, sg, sb, dr, dg, db float32) (float32, float32, float32) {
	// SetSat(Cs, Sat(Cb))
	satB := Sat(dr, dg, db)
	r, g, b := SetSat(sr, sg, sb, satB)

	// SetLum(result, Lum(Cb))
	lumB := Lum(dr, dg, db)
	return SetLum(r, g, b, lumB)
}

// hslBlendSaturation uses the saturation of the source with hue and luminosity of the backdrop.
// Formula: SetLum(SetSat(Cb, Sat(Cs)), Lum(Cb))
//
// This creates a color with the saturation of the source and the hue and luminosity
// of the backdrop.
func hslBlendSaturation(sr, sg, sb, dr, dg, db float32) (float32, float32, float32) {
	// SetSat(Cb, Sat(Cs))
	satS := Sat(sr, sg, sb)
	r, g, b := SetSat(dr, dg, db, satS)

	// SetLum(result, Lum(Cb))
	lumB := Lum(dr, dg, db)
	return SetLum(r, g, b, lumB)
}

// hslBlendColor uses the hue and saturation of the source with luminosity of the backdrop.
// Formula: SetLum(Cs, Lum(Cb))
//
// This creates a color with the hue and saturation of the source and the luminosity
// of the backdrop.
func hslBlendColor(sr, sg, sb, dr, dg, db float32) (float32, float32, float32) {
	lumB := Lum(dr, dg, db)
	return SetLum(sr, sg, sb, lumB)
}

// hslBlendLuminosity uses the luminosity of the source with hue and saturation of the backdrop.
// Formula: SetLum(Cb, Lum(Cs))
//
// This creates a color with the luminosity of the source and the hue and saturation
// of the backdrop.
func hslBlendLuminosity(sr, sg, sb, dr, dg, db float32) (float32, float32, float32) {
	lumS := Lum(sr, sg, sb)
	return SetLum(dr, dg, db, lumS)
}

// Utility functions

// min3 returns the minimum of three float32 values.
func min3(a, b, c float32) float32 {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// max3 returns the maximum of three float32 values.
func max3(a, b, c float32) float32 {
	if a > b {
		if a > c {
			return a
		}
		return c
	}
	if b > c {
		return b
	}
	return c
}

// Byte-based wrapper functions for integration with existing blend system

// blendHue wraps hslBlendHue for byte-based blend operations.
func blendHue(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return nonSeparableBlend(sr, sg, sb, sa, dr, dg, db, da, hslBlendHue)
}

// blendSaturation wraps hslBlendSaturation for byte-based blend operations.
func blendSaturation(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return nonSeparableBlend(sr, sg, sb, sa, dr, dg, db, da, hslBlendSaturation)
}

// blendColor wraps hslBlendColor for byte-based blend operations.
func blendColor(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return nonSeparableBlend(sr, sg, sb, sa, dr, dg, db, da, hslBlendColor)
}

// blendLuminosity wraps hslBlendLuminosity for byte-based blend operations.
func blendLuminosity(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return nonSeparableBlend(sr, sg, sb, sa, dr, dg, db, da, hslBlendLuminosity)
}

// nonSeparableBlend is a helper that applies non-separable blend modes.
// It handles premultiplied alpha and applies the standard compositing formula:
// Result = (1 - Sa) * D + (1 - Da) * S + Sa * Da * B(Cs, Cb)
//
// where B(Cs, Cb) is the non-separable blend function.
func nonSeparableBlend(
	sr, sg, sb, sa, dr, dg, db, da byte,
	blendFunc func(sr, sg, sb, dr, dg, db float32) (float32, float32, float32),
) (byte, byte, byte, byte) {
	// Handle fully transparent cases
	if sa == 0 {
		return dr, dg, db, da
	}
	if da == 0 {
		return sr, sg, sb, sa
	}

	// Unpremultiply and convert to float [0, 1]
	var sur, sug, sub, dur, dug, dub float32
	if sa > 0 {
		sur = float32(sr) / float32(sa)
		sug = float32(sg) / float32(sa)
		sub = float32(sb) / float32(sa)
	}
	if da > 0 {
		dur = float32(dr) / float32(da)
		dug = float32(dg) / float32(da)
		dub = float32(db) / float32(da)
	}

	// Apply non-separable blend function
	blendR, blendG, blendB := blendFunc(sur, sug, sub, dur, dug, dub)

	// Convert back to premultiplied alpha with compositing formula
	// Result = (1 - Sa) * D + (1 - Da) * S + Sa * Da * B
	invSa := 255 - sa
	invDa := 255 - da
	saf := float32(sa) / 255.0
	daf := float32(da) / 255.0

	// Calculate final alpha: Sa + Da * (1 - Sa)
	finalA := addDiv255(sa, mulDiv255(da, invSa))

	// Calculate final color channels
	// (1 - Sa) * D + (1 - Da) * S
	finalR := addDiv255(mulDiv255(dr, invSa), mulDiv255(sr, invDa))
	finalG := addDiv255(mulDiv255(dg, invSa), mulDiv255(sg, invDa))
	finalB := addDiv255(mulDiv255(db, invSa), mulDiv255(sb, invDa))

	// + Sa * Da * B (blend result)
	saDa := saf * daf
	blendContribR := byte(math.Round(float64(blendR * saDa * 255.0)))
	blendContribG := byte(math.Round(float64(blendG * saDa * 255.0)))
	blendContribB := byte(math.Round(float64(blendB * saDa * 255.0)))

	finalR = addDiv255(finalR, blendContribR)
	finalG = addDiv255(finalG, blendContribG)
	finalB = addDiv255(finalB, blendContribB)

	return finalR, finalG, finalB, finalA
}
