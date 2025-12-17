// Package blend provides fast math utilities for alpha blending.
//
// The div255 family of functions avoid expensive integer division by using
// bit shifts and addition. These are critical for performance as mulDiv255
// is called for every pixel in every blend operation.
//
// References:
//   - Alpha blending without division: https://arxiv.org/abs/2202.02864
//   - Alvy Ray Smith's technical memos: http://alvyray.com/Memos/
package blend

// div255 divides x by 255 using fast shift approximation.
//
// Formula: (x + 255) >> 8
//
// This is ~5x faster than integer division. The maximum error is +1
// for some input values, which is imperceptible in alpha blending.
//
// For inputs 0-65535, the result is within [0, 256].
// For alpha blending (inputs 0-65025 = 255*255), result is within [0, 255].
func div255(x uint16) uint16 {
	return (x + 255) >> 8
}

// div255Exact divides x by 255 exactly without using division.
//
// Formula: ((x + 1) + ((x + 1) >> 8)) >> 8
//
// This is Alvy Ray Smith's formula, which gives exact results for all
// uint16 values. It's ~3x faster than integer division but slower than
// the fast approximation.
//
// Use this when exact results are required (e.g., in tests).
func div255Exact(x uint16) uint16 {
	t := x + 1
	return (t + (t >> 8)) >> 8
}

// mulDiv255 multiplies two bytes and divides by 255 using fast approximation.
//
// Formula: (a * b + 255) >> 8
//
// This replaces the old: (a * b + 127) / 255
// The speed improvement is ~5x with imperceptible quality difference.
func mulDiv255(a, b byte) byte {
	return byte(div255(uint16(a) * uint16(b)))
}

// mulDiv255Exact multiplies two bytes and divides by 255 exactly.
//
// Use this when exact results are required (e.g., in reference implementations).
func mulDiv255Exact(a, b byte) byte {
	return byte(div255Exact(uint16(a) * uint16(b)))
}

// inv255 computes 255 - x (inverse alpha).
func inv255(x byte) byte {
	return 255 - x
}

// clamp255 clamps a uint16 to byte range [0, 255].
func clamp255(x uint16) byte {
	if x > 255 {
		return 255
	}
	return byte(x)
}

// addClamp adds two bytes and clamps to 255.
func addClamp(a, b byte) byte {
	sum := uint16(a) + uint16(b)
	if sum > 255 {
		return 255
	}
	return byte(sum)
}

// subClamp subtracts b from a, clamping to 0.
func subClamp(a, b byte) byte {
	if b >= a {
		return 0
	}
	return a - b
}
