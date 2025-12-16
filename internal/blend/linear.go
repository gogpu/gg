// Package blend implements linear color space blending operations.
//
// This file implements linear-space versions of all blend modes.
// Linear blending produces physically correct results by performing
// color math in linear RGB rather than sRGB space.
//
// Key principle: Alpha is ALWAYS linear - only RGB channels undergo gamma conversion.
//
// References:
//   - GPU Gems 3: "The Importance of Being Linear"
//   - W3C Compositing and Blending Level 1: https://www.w3.org/TR/compositing-1/
package blend

import (
	"github.com/gogpu/gg/internal/color"
)

// BlendModeLinear indicates if blending should be performed in linear space.
type BlendModeLinear struct {
	Mode   BlendMode
	Linear bool // If true, blend in linear space; if false, blend in sRGB
}

// GetBlendFuncLinear returns a blend function that optionally operates in linear space.
// If linear is true, the function will:
//  1. Convert src/dst RGB from sRGB to linear (alpha stays linear)
//  2. Perform blending in linear space
//  3. Convert result RGB from linear to sRGB (alpha stays linear)
//
// This produces physically correct blending without dark halos or incorrect color mixing.
func GetBlendFuncLinear(mode BlendMode, linear bool) BlendFunc {
	if !linear {
		// Standard sRGB blending
		return GetBlendFunc(mode)
	}

	// Linear space blending - wrap the original blend function
	baseFunc := GetBlendFunc(mode)
	return func(sr, sg, sb, sa, dr, dg, db, da byte) (r, g, b, a byte) {
		// Convert byte [0,255] to float32 [0,1]
		srcColor := color.ColorF32{
			R: float32(sr) / 255.0,
			G: float32(sg) / 255.0,
			B: float32(sb) / 255.0,
			A: float32(sa) / 255.0,
		}
		dstColor := color.ColorF32{
			R: float32(dr) / 255.0,
			G: float32(dg) / 255.0,
			B: float32(db) / 255.0,
			A: float32(da) / 255.0,
		}

		// Unpremultiply alpha (blend functions expect premultiplied, but we need
		// to unpremultiply to convert color space, then re-premultiply)
		if srcColor.A > 0 {
			srcColor.R /= srcColor.A
			srcColor.G /= srcColor.A
			srcColor.B /= srcColor.A
		}
		if dstColor.A > 0 {
			dstColor.R /= dstColor.A
			dstColor.G /= dstColor.A
			dstColor.B /= dstColor.A
		}

		// Convert RGB from sRGB to linear (alpha is already linear)
		srcLinear := color.SRGBToLinearColor(srcColor)
		dstLinear := color.SRGBToLinearColor(dstColor)

		// Re-premultiply in linear space
		srcLinear.R *= srcLinear.A
		srcLinear.G *= srcLinear.A
		srcLinear.B *= srcLinear.A
		dstLinear.R *= dstLinear.A
		dstLinear.G *= dstLinear.A
		dstLinear.B *= dstLinear.A

		// Convert to byte for blending
		srcBytes := color.F32ToU8(srcLinear)
		dstBytes := color.F32ToU8(dstLinear)

		// Perform blend in linear space
		resR, resG, resB, resA := baseFunc(
			srcBytes.R, srcBytes.G, srcBytes.B, srcBytes.A,
			dstBytes.R, dstBytes.G, dstBytes.B, dstBytes.A,
		)

		// Convert result back to float32
		resColor := color.ColorF32{
			R: float32(resR) / 255.0,
			G: float32(resG) / 255.0,
			B: float32(resB) / 255.0,
			A: float32(resA) / 255.0,
		}

		// Unpremultiply for color space conversion
		if resColor.A > 0 {
			resColor.R /= resColor.A
			resColor.G /= resColor.A
			resColor.B /= resColor.A
		}

		// Convert RGB from linear to sRGB (alpha stays linear)
		resSRGB := color.LinearToSRGBColor(resColor)

		// Re-premultiply in sRGB space
		resSRGB.R *= resSRGB.A
		resSRGB.G *= resSRGB.A
		resSRGB.B *= resSRGB.A

		// Convert back to byte
		final := color.F32ToU8(resSRGB)
		return final.R, final.G, final.B, final.A
	}
}

// BlendLinear performs blending in linear color space.
// This is a convenience function that wraps GetBlendFuncLinear.
func BlendLinear(src, dst color.ColorU8, mode BlendMode) color.ColorU8 {
	blendFunc := GetBlendFuncLinear(mode, true)
	r, g, b, a := blendFunc(src.R, src.G, src.B, src.A, dst.R, dst.G, dst.B, dst.A)
	return color.ColorU8{R: r, G: g, B: b, A: a}
}
