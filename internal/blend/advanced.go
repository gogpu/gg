// Package blend implements advanced separable and non-separable blend modes.
//
// This file implements advanced blend modes beyond Porter-Duff compositing,
// following the W3C Compositing and Blending Level 1 specification.
//
// Separable blend modes operate on each color channel independently.
// Non-separable blend modes require color space conversions (HSL/HSV).
//
// References:
//   - W3C Compositing and Blending Level 1: https://www.w3.org/TR/compositing-1/
//   - PDF Blend Modes: Addendum (ISO 32000-1:2008)
package blend

import "math"

// Advanced separable blend modes (extend BlendMode enum)
const (
	// Separable blend modes
	BlendMultiply    BlendMode = iota + 14 // Result: S * D
	BlendScreen                             // Result: 1 - (1-S)*(1-D)
	BlendOverlay                            // HardLight with swapped layers
	BlendDarken                             // min(S, D)
	BlendLighten                            // max(S, D)
	BlendColorDodge                         // D / (1 - S)
	BlendColorBurn                          // 1 - (1 - D) / S
	BlendHardLight                          // Multiply or Screen depending on source
	BlendSoftLight                          // Soft version of HardLight
	BlendDifference                         // |S - D|
	BlendExclusion                          // S + D - 2*S*D

	// Non-separable blend modes (optional)
	BlendHue        // Hue of source, saturation and luminosity of backdrop
	BlendSaturation // Saturation of source, hue and luminosity of backdrop
	BlendColor      // Hue and saturation of source, luminosity of backdrop
	BlendLuminosity // Luminosity of source, hue and saturation of backdrop
)

// separableBlend is a helper function that applies a per-channel blend function.
// It handles the standard formula: Result = (1 - Sa) * D + (1 - Da) * S + Sa * Da * B(Sc, Dc)
// where B(Sc, Dc) is the blend function for unmultiplied source and destination channels.
//
// Parameters:
//   - sr, sg, sb, sa: source color (premultiplied alpha)
//   - dr, dg, db, da: destination color (premultiplied alpha)
//   - blendChan: per-channel blend function B(s, d) operating on unmultiplied values
//
// Returns: resulting color (r, g, b, a) after blending.
func separableBlend(sr, sg, sb, sa, dr, dg, db, da byte, blendChan func(s, d byte) byte) (byte, byte, byte, byte) {
	// Handle fully transparent cases
	if sa == 0 {
		return dr, dg, db, da
	}
	if da == 0 {
		return sr, sg, sb, sa
	}

	// Unpremultiply source and destination
	// For premultiplied colors: color = alpha * unmultiplied_color
	// So: unmultiplied = color / alpha
	var sur, sug, sub, dur, dug, dub byte
	if sa > 0 {
		sur = byte((uint16(sr) * 255) / uint16(sa))
		sug = byte((uint16(sg) * 255) / uint16(sa))
		sub = byte((uint16(sb) * 255) / uint16(sa))
	}
	if da > 0 {
		dur = byte((uint16(dr) * 255) / uint16(da))
		dug = byte((uint16(dg) * 255) / uint16(da))
		dub = byte((uint16(db) * 255) / uint16(da))
	}

	// Apply blend function to unmultiplied channels
	blendR := blendChan(sur, dur)
	blendG := blendChan(sug, dug)
	blendB := blendChan(sub, dub)

	// Standard formula: (1 - Sa) * D + (1 - Da) * S + Sa * Da * B(Sc, Dc)
	// Simplified for premultiplied: (1 - Sa) * D + (1 - Da) * S + Sa * Da * B
	invSa := 255 - sa
	invDa := 255 - da

	// Calculate final alpha: Sa + Da * (1 - Sa)
	finalA := addDiv255(sa, mulDiv255(da, invSa))

	// Calculate final color channels
	// Component = (1 - Sa) * Dc * Da + (1 - Da) * Sc * Sa + Sa * Da * B
	var finalR, finalG, finalB byte

	// (1 - Sa) * D + (1 - Da) * S
	finalR = addDiv255(mulDiv255(dr, invSa), mulDiv255(sr, invDa))
	finalG = addDiv255(mulDiv255(dg, invSa), mulDiv255(sg, invDa))
	finalB = addDiv255(mulDiv255(db, invSa), mulDiv255(sb, invDa))

	// + Sa * Da * B (blend result)
	saDa := mulDiv255(sa, da)
	finalR = addDiv255(finalR, mulDiv255(saDa, blendR))
	finalG = addDiv255(finalG, mulDiv255(saDa, blendG))
	finalB = addDiv255(finalB, mulDiv255(saDa, blendB))

	return finalR, finalG, finalB, finalA
}

// Advanced blend mode implementations

// blendMultiply multiplies source and destination colors.
// Formula: B(Cb, Cs) = Cb * Cs
func blendMultiply(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, mulDiv255)
}

// blendScreen produces a lighter result than multiply.
// Formula: B(Cb, Cs) = 1 - (1 - Cb) * (1 - Cs)
func blendScreen(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		// 1 - (1 - s) * (1 - d)
		invS := 255 - s
		invD := 255 - d
		return 255 - mulDiv255(invS, invD)
	})
}

// blendOverlay combines Multiply and Screen.
// Formula: B(Cb, Cs) = HardLight(Cs, Cb) (swapped parameters)
func blendOverlay(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		// if Cb <= 0.5: 2 * Cb * Cs
		// else: 1 - 2 * (1 - Cb) * (1 - Cs)
		if d <= 128 {
			return mulDiv255(2*d, s)
		}
		invD := 255 - d
		invS := 255 - s
		return 255 - mulDiv255(2*invD, invS)
	})
}

// blendDarken selects the darker of source and destination.
// Formula: B(Cb, Cs) = min(Cb, Cs)
func blendDarken(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, minByte)
}

// blendLighten selects the lighter of source and destination.
// Formula: B(Cb, Cs) = max(Cb, Cs)
func blendLighten(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, maxByte)
}

// blendColorDodge brightens the destination to reflect the source.
// Formula: B(Cb, Cs) = if Cs == 1: 1, else: min(1, Cb / (1 - Cs))
func blendColorDodge(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		if s == 255 {
			return 255
		}
		// Cb / (1 - Cs)
		invS := 255 - s
		result := (uint16(d) * 255) / uint16(invS)
		if result > 255 {
			return 255
		}
		return byte(result)
	})
}

// blendColorBurn darkens the destination to reflect the source.
// Formula: B(Cb, Cs) = if Cs == 0: 0, else: 1 - min(1, (1 - Cb) / Cs)
func blendColorBurn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		if s == 0 {
			return 0
		}
		// 1 - (1 - Cb) / Cs
		invD := 255 - d
		result := (uint16(invD) * 255) / uint16(s)
		if result > 255 {
			return 0
		}
		return 255 - byte(result)
	})
}

// blendHardLight combines Multiply and Screen based on source.
// Formula: B(Cb, Cs) = if Cs <= 0.5: Multiply(Cb, 2*Cs), else: Screen(Cb, 2*Cs - 1)
func blendHardLight(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		// if Cs <= 0.5: 2 * Cb * Cs
		// else: 1 - 2 * (1 - Cb) * (1 - Cs)
		if s <= 128 {
			return mulDiv255(2*s, d)
		}
		invS := 255 - s
		invD := 255 - d
		return 255 - mulDiv255(2*invS, invD)
	})
}

// blendSoftLight is a softer version of HardLight.
// Formula: B(Cb, Cs) = complex formula based on Cb and Cs
func blendSoftLight(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		// Convert to float for precise calculation
		sf := float64(s) / 255.0
		df := float64(d) / 255.0

		var result float64
		if sf <= 0.5 {
			// B(Cb, Cs) = Cb - (1 - 2*Cs) * Cb * (1 - Cb)
			result = df - (1-2*sf)*df*(1-df)
		} else {
			// B(Cb, Cs) = Cb + (2*Cs - 1) * (D(Cb) - Cb)
			// where D(x) = if x <= 0.25: ((16*x - 12)*x + 4)*x, else: sqrt(x)
			var dx float64
			if df <= 0.25 {
				dx = ((16*df-12)*df+4)*df
			} else {
				dx = math.Sqrt(df)
			}
			result = df + (2*sf-1)*(dx-df)
		}

		// Clamp to [0, 1] and convert back to byte
		if result < 0 {
			return 0
		}
		if result > 1 {
			return 255
		}
		return byte(result * 255)
	})
}

// blendDifference produces the absolute difference between source and destination.
// Formula: B(Cb, Cs) = |Cb - Cs|
func blendDifference(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		if s > d {
			return s - d
		}
		return d - s
	})
}

// blendExclusion is similar to Difference but with lower contrast.
// Formula: B(Cb, Cs) = Cb + Cs - 2 * Cb * Cs
func blendExclusion(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlend(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		// Cb + Cs - 2 * Cb * Cs
		sum := uint16(s) + uint16(d)
		product := mulDiv255(s, d)
		diff := sum - 2*uint16(product)
		if diff > 255 {
			return 255
		}
		return byte(diff)
	})
}

// Non-separable blend modes (optional, not implemented in v0.3.0)

// blendHue uses the hue of the source with saturation and luminosity of destination.
// TODO: Implement for future version (requires RGB to HSL conversion)
func blendHue(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	// Placeholder: returns source over for now
	return blendSourceOver(sr, sg, sb, sa, dr, dg, db, da)
}

// blendSaturation uses saturation of source with hue and luminosity of destination.
// TODO: Implement for future version (requires RGB to HSL conversion)
func blendSaturation(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	// Placeholder: returns source over for now
	return blendSourceOver(sr, sg, sb, sa, dr, dg, db, da)
}

// blendColor uses hue and saturation of source with luminosity of destination.
// TODO: Implement for future version (requires RGB to HSL conversion)
func blendColor(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	// Placeholder: returns source over for now
	return blendSourceOver(sr, sg, sb, sa, dr, dg, db, da)
}

// blendLuminosity uses luminosity of source with hue and saturation of destination.
// TODO: Implement for future version (requires RGB to HSL conversion)
func blendLuminosity(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	// Placeholder: returns source over for now
	return blendSourceOver(sr, sg, sb, sa, dr, dg, db, da)
}

// Utility functions

// maxByte returns the larger of two bytes.
func maxByte(a, b byte) byte {
	if a > b {
		return a
	}
	return b
}
