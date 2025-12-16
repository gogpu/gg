// Package blend implements Porter-Duff compositing operators and blend modes.
//
// All blend operations work with premultiplied alpha values in the range 0-255.
// This follows the WebGPU and modern graphics conventions for efficient compositing.
//
// References:
//   - Porter-Duff: "Compositing Digital Images" (1984)
//   - W3C Compositing and Blending Level 1: https://www.w3.org/TR/compositing-1/
package blend

// BlendMode represents a Porter-Duff compositing operation.
type BlendMode uint8

const (
	// Porter-Duff modes (standard compositing operators)
	BlendClear           BlendMode = iota // Result: 0 (clear destination)
	BlendSource                           // Result: S (replace with source)
	BlendDestination                      // Result: D (keep destination)
	BlendSourceOver                       // Result: S + D*(1-Sa) [default]
	BlendDestinationOver                  // Result: S*(1-Da) + D
	BlendSourceIn                         // Result: S*Da
	BlendDestinationIn                    // Result: D*Sa
	BlendSourceOut                        // Result: S*(1-Da)
	BlendDestinationOut                   // Result: D*(1-Sa)
	BlendSourceAtop                       // Result: S*Da + D*(1-Sa)
	BlendDestinationAtop                  // Result: S*(1-Da) + D*Sa
	BlendXor                              // Result: S*(1-Da) + D*(1-Sa)
	BlendPlus                             // Result: S + D (clamped to 255)
	BlendModulate                         // Result: S*D (multiply)
)

// BlendFunc is the signature for blend operations.
// All values are premultiplied alpha, 0-255.
// Parameters:
//   - sr, sg, sb, sa: source color (red, green, blue, alpha)
//   - dr, dg, db, da: destination color (red, green, blue, alpha)
//
// Returns: resulting color (r, g, b, a) after blending.
type BlendFunc func(sr, sg, sb, sa, dr, dg, db, da byte) (r, g, b, a byte)

// GetBlendFunc returns the blend function for the given mode.
// Returns blendSourceOver for unknown modes.
func GetBlendFunc(mode BlendMode) BlendFunc {
	switch mode {
	// Porter-Duff modes
	case BlendClear:
		return blendClear
	case BlendSource:
		return blendSource
	case BlendDestination:
		return blendDestination
	case BlendSourceOver:
		return blendSourceOver
	case BlendDestinationOver:
		return blendDestinationOver
	case BlendSourceIn:
		return blendSourceIn
	case BlendDestinationIn:
		return blendDestinationIn
	case BlendSourceOut:
		return blendSourceOut
	case BlendDestinationOut:
		return blendDestinationOut
	case BlendSourceAtop:
		return blendSourceAtop
	case BlendDestinationAtop:
		return blendDestinationAtop
	case BlendXor:
		return blendXor
	case BlendPlus:
		return blendPlus
	case BlendModulate:
		return blendModulate

	// Advanced separable blend modes
	case BlendMultiply:
		return blendMultiply
	case BlendScreen:
		return blendScreen
	case BlendOverlay:
		return blendOverlay
	case BlendDarken:
		return blendDarken
	case BlendLighten:
		return blendLighten
	case BlendColorDodge:
		return blendColorDodge
	case BlendColorBurn:
		return blendColorBurn
	case BlendHardLight:
		return blendHardLight
	case BlendSoftLight:
		return blendSoftLight
	case BlendDifference:
		return blendDifference
	case BlendExclusion:
		return blendExclusion

	// Non-separable blend modes (placeholder implementations)
	case BlendHue:
		return blendHue
	case BlendSaturation:
		return blendSaturation
	case BlendColor:
		return blendColor
	case BlendLuminosity:
		return blendLuminosity

	default:
		return blendSourceOver
	}
}

// Porter-Duff implementations (premultiplied alpha)

// blendClear clears the destination to transparent black.
func blendClear(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return 0, 0, 0, 0
}

// blendSource replaces destination with source.
func blendSource(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return sr, sg, sb, sa
}

// blendDestination keeps destination unchanged.
func blendDestination(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return dr, dg, db, da
}

// blendSourceOver composites source over destination (default blend mode).
// Formula: S + D * (1 - Sa)
func blendSourceOver(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invSa := 255 - sa
	return addDiv255(sr, mulDiv255(dr, invSa)),
		addDiv255(sg, mulDiv255(dg, invSa)),
		addDiv255(sb, mulDiv255(db, invSa)),
		addDiv255(sa, mulDiv255(da, invSa))
}

// blendDestinationOver composites destination over source.
// Formula: S * (1 - Da) + D
func blendDestinationOver(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invDa := 255 - da
	return addDiv255(mulDiv255(sr, invDa), dr),
		addDiv255(mulDiv255(sg, invDa), dg),
		addDiv255(mulDiv255(sb, invDa), db),
		addDiv255(mulDiv255(sa, invDa), da)
}

// blendSourceIn shows source where destination is opaque.
// Formula: S * Da
func blendSourceIn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return mulDiv255(sr, da), mulDiv255(sg, da), mulDiv255(sb, da), mulDiv255(sa, da)
}

// blendDestinationIn shows destination where source is opaque.
// Formula: D * Sa
func blendDestinationIn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return mulDiv255(dr, sa), mulDiv255(dg, sa), mulDiv255(db, sa), mulDiv255(da, sa)
}

// blendSourceOut shows source where destination is transparent.
// Formula: S * (1 - Da)
func blendSourceOut(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invDa := 255 - da
	return mulDiv255(sr, invDa), mulDiv255(sg, invDa), mulDiv255(sb, invDa), mulDiv255(sa, invDa)
}

// blendDestinationOut shows destination where source is transparent.
// Formula: D * (1 - Sa)
func blendDestinationOut(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invSa := 255 - sa
	return mulDiv255(dr, invSa), mulDiv255(dg, invSa), mulDiv255(db, invSa), mulDiv255(da, invSa)
}

// blendSourceAtop composites source over destination, preserving destination alpha.
// Formula: S * Da + D * (1 - Sa)
func blendSourceAtop(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invSa := 255 - sa
	return addDiv255(mulDiv255(sr, da), mulDiv255(dr, invSa)),
		addDiv255(mulDiv255(sg, da), mulDiv255(dg, invSa)),
		addDiv255(mulDiv255(sb, da), mulDiv255(db, invSa)),
		da // Alpha unchanged (destination alpha)
}

// blendDestinationAtop composites destination over source, preserving source alpha.
// Formula: S * (1 - Da) + D * Sa
func blendDestinationAtop(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invDa := 255 - da
	return addDiv255(mulDiv255(sr, invDa), mulDiv255(dr, sa)),
		addDiv255(mulDiv255(sg, invDa), mulDiv255(dg, sa)),
		addDiv255(mulDiv255(sb, invDa), mulDiv255(db, sa)),
		sa // Alpha = source alpha
}

// blendXor shows source and destination where they don't overlap.
// Formula: S * (1 - Da) + D * (1 - Sa)
func blendXor(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invDa := 255 - da
	invSa := 255 - sa
	return addDiv255(mulDiv255(sr, invDa), mulDiv255(dr, invSa)),
		addDiv255(mulDiv255(sg, invDa), mulDiv255(dg, invSa)),
		addDiv255(mulDiv255(sb, invDa), mulDiv255(db, invSa)),
		addDiv255(mulDiv255(sa, invDa), mulDiv255(da, invSa))
}

// blendPlus adds source and destination colors (clamped to 255).
// Formula: min(S + D, 255)
func blendPlus(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return clampAdd(sr, dr), clampAdd(sg, dg), clampAdd(sb, db), clampAdd(sa, da)
}

// blendModulate multiplies source and destination colors.
// Formula: S * D / 255
func blendModulate(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return mulDiv255(sr, dr), mulDiv255(sg, dg), mulDiv255(sb, db), mulDiv255(sa, da)
}

// Utility functions

// mulDiv255 multiplies two byte values and divides by 255 with proper rounding.
// Formula: (a * b + 127) / 255
// The +127 provides correct rounding (equivalent to adding 0.5 before truncation).
func mulDiv255(a, b byte) byte {
	return byte((uint16(a)*uint16(b) + 127) / 255)
}

// addDiv255 adds two byte values with clamping to 255.
func addDiv255(a, b byte) byte {
	sum := uint16(a) + uint16(b)
	if sum > 255 {
		return 255
	}
	return byte(sum)
}

// minByte returns the smaller of two bytes.
func minByte(a, b byte) byte {
	if a < b {
		return a
	}
	return b
}

// clampAdd adds two byte values with clamping to 255.
// This is needed for blendPlus to avoid byte overflow.
func clampAdd(a, b byte) byte {
	sum := uint16(a) + uint16(b)
	if sum > 255 {
		return 255
	}
	return byte(sum)
}
