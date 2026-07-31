package gg

// Mask gamma correction for glyph mask text rendering.
//
// Light text on dark backgrounds appears perceptually thinner than intended
// because linear alpha blending underestimates perceived coverage on low-
// luminance destinations. Skia compensates via SkMaskGamma (PreBlend LUTs
// keyed by text color and device gamma). Our approach applies the core
// contrast boost formula from Skia's apply_contrast() in the glyph mask
// fragment shader, using the text color's luminance to modulate the boost.
//
// The formula (Skia SkMaskGamma.cpp:74):
//
//	boosted = alpha + (1 - alpha) * contrast * alpha
//
// Properties:
//   - alpha=0 -> 0 (transparent unchanged)
//   - alpha=1 -> 1 (full coverage unchanged)
//   - Monotonically increasing in alpha for contrast in [0, 1]
//   - Output bounded to [0, 1] for inputs in [0, 1]
//
// The contrast factor is derived from the text color luminance:
//
//	contrast = luminance * maxContrast
//
// where luminance = 0.299*R + 0.587*G + 0.114*B (Rec.601) and maxContrast
// defaults to 0.5 (matching Skia's SK_GAMMA_CONTRAST).
//
// Effect: light text (high luminance) gets boosted edge coverage, making
// stems appear more solid on dark backgrounds. Dark text (low luminance)
// gets minimal boost, preserving the natural weight on light backgrounds.
//
// References:
//   - Skia SkMaskGamma.cpp:74 (apply_contrast)
//   - Skia SkTypes.h:89-93 (SK_GAMMA_CONTRAST = 0.5)
//   - Skia DistanceFieldAdjustTable.cpp:26-59 (GPU-side rationale)

// maskGammaMaxContrast is the maximum contrast factor applied to glyph mask
// coverage. Matches Skia's SK_GAMMA_CONTRAST default of 0.5.
//
// A value of 0.5 is a good compromise between stem darkening for light text
// and preserving natural weight for dark text.
const maskGammaMaxContrast = 0.5

// applyMaskGammaContrast applies the Skia mask gamma contrast boost to an
// alpha coverage value. The contrast parameter controls the strength of the
// boost (0 = no change, 1 = maximum boost).
//
// Formula: boosted = alpha + (1 - alpha) * contrast * alpha
//
// This boosts mid-range alpha values proportionally to the contrast factor
// while preserving the endpoints (0 and 1 are fixed points).
func applyMaskGammaContrast(alpha, contrast float64) float64 {
	return alpha + (1.0-alpha)*contrast*alpha
}

// maskGammaContrastForColor computes the contrast factor for mask gamma
// correction based on the text color's luminance. Light text gets higher
// contrast (more boost), dark text gets lower contrast (less boost).
//
// Parameters r, g, b are straight-alpha color components in [0, 1].
//
// Returns a contrast value in [0, maskGammaMaxContrast].
func maskGammaContrastForColor(r, g, b float64) float64 {
	// Rec.601 luminance (matches WGSL vec3(0.299, 0.587, 0.114)).
	luminance := 0.299*r + 0.587*g + 0.114*b
	return luminance * maskGammaMaxContrast
}
