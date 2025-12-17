// Package color provides fast color space conversion using lookup tables.
//
// The lookup tables (LUT) provide O(1) sRGB ↔ Linear conversions,
// replacing expensive math.Pow calls with simple array lookups.
// This is critical for performance in alpha blending operations.
//
// sRGB is the standard color space for images and displays, but blending
// operations should be performed in linear space for physically correct results.
//
// References:
//   - sRGB specification: https://www.w3.org/Graphics/Color/sRGB
//   - GPU Gems 3, Chapter 24: https://developer.nvidia.com/gpugems/gpugems3/part-iv-image-effects/chapter-24-importance-being-linear
package color

import "math"

// sRGBToLinearLUT provides O(1) sRGB to Linear conversion.
// Pre-computed 256 entries, 1KB memory cost.
// Converts sRGB byte [0-255] → Linear float32 [0.0-1.0].
var sRGBToLinearLUT [256]float32

// linearToSRGBLUT provides O(1) Linear to sRGB conversion.
// Uses 4096 entries for 12-bit precision (sufficient for 8-bit sRGB).
// Converts Linear float32 [0.0-1.0] → sRGB byte [0-255].
var linearToSRGBLUT [4096]uint8

func init() {
	// Build sRGB → Linear table
	for i := 0; i < 256; i++ {
		s := float64(i) / 255.0
		var linear float64
		if s <= 0.04045 {
			linear = s / 12.92
		} else {
			linear = math.Pow((s+0.055)/1.055, 2.4)
		}
		sRGBToLinearLUT[i] = float32(linear)
	}

	// Build Linear → sRGB table
	for i := 0; i < 4096; i++ {
		linear := float64(i) / 4095.0
		var s float64
		if linear <= 0.0031308 {
			s = linear * 12.92
		} else {
			s = 1.055*math.Pow(linear, 1.0/2.4) - 0.055
		}
		// Clamp and convert to byte
		srgb := int(s*255.0 + 0.5)
		if srgb < 0 {
			srgb = 0
		}
		if srgb > 255 {
			srgb = 255
		}
		//nolint:gosec // G115: srgb is clamped to [0,255] range
		linearToSRGBLUT[i] = uint8(srgb)
	}
}

// SRGBToLinearFast converts sRGB byte to linear float32 using lookup table.
//
// This is ~20x faster than computing with math.Pow for each pixel.
// Used in blend operations that require linear color space.
//
// Example:
//
//	r := SRGBToLinearFast(128) // ~0.2159 (not 0.5!)
func SRGBToLinearFast(s uint8) float32 {
	return sRGBToLinearLUT[s]
}

// LinearToSRGBFast converts linear float32 to sRGB byte using lookup table.
//
// Input is clamped to [0.0, 1.0] range automatically.
// Uses 12-bit precision (4096 entries) which is more than sufficient
// for 8-bit sRGB output.
//
// Example:
//
//	s := LinearToSRGBFast(0.5) // 188 (not 128!)
func LinearToSRGBFast(l float32) uint8 {
	// Clamp to [0.0, 1.0]
	if l < 0 {
		l = 0
	}
	if l > 1 {
		l = 1
	}
	// Map to [0, 4095] range
	index := int(l*4095.0 + 0.5)
	if index > 4095 {
		index = 4095
	}
	return linearToSRGBLUT[index]
}

// SRGBToLinearSlow converts sRGB byte to linear float32 using math.Pow.
//
// This is the reference implementation, ~20x slower than the LUT version.
// Used for testing and verification only.
func SRGBToLinearSlow(s uint8) float32 {
	sf := float64(s) / 255.0
	var linear float64
	if sf <= 0.04045 {
		linear = sf / 12.92
	} else {
		linear = math.Pow((sf+0.055)/1.055, 2.4)
	}
	return float32(linear)
}

// LinearToSRGBSlow converts linear float32 to sRGB byte using math.Pow.
//
// This is the reference implementation, ~15x slower than the LUT version.
// Used for testing and verification only.
func LinearToSRGBSlow(l float32) uint8 {
	lf := float64(l)
	// Clamp to [0.0, 1.0]
	if lf < 0 {
		lf = 0
	}
	if lf > 1 {
		lf = 1
	}
	var s float64
	if lf <= 0.0031308 {
		s = lf * 12.92
	} else {
		s = 1.055*math.Pow(lf, 1.0/2.4) - 0.055
	}
	// Convert to byte with rounding
	srgb := int(s*255.0 + 0.5)
	if srgb < 0 {
		srgb = 0
	}
	if srgb > 255 {
		srgb = 255
	}
	//nolint:gosec // G115: srgb is clamped to [0,255] range
	return uint8(srgb)
}
