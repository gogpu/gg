// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

// Fixed-point number types for precise curve rasterization.
//
// This file provides fixed-point arithmetic for scanline conversion of
// Bezier curves. The algorithms are derived from tiny-skia/Skia which
// uses fixed-point for deterministic results across platforms.
//
// Why fixed-point?
// - Predictable overflow behavior
// - Exact intermediate results for curve stepping
// - Avoids floating-point accumulation errors in forward differencing
//
// Type Reference:
// - FDot6:  26.6 fixed-point (6 fractional bits) - for pixel subdivision
// - FDot16: 16.16 fixed-point (16 fractional bits) - for positions and slopes

// FDot6 is a 26.6 fixed-point number (6 fractional bits).
// Used for intermediate calculations in edge setup where sub-pixel
// precision (1/64 pixel) is sufficient.
//
// Range: approximately -33 million to +33 million with 1/64 precision.
type FDot6 = int32

// FDot16 is a 16.16 fixed-point number (16 fractional bits).
// Used for positions, slopes, and forward differencing coefficients
// where higher precision is needed.
//
// Range: approximately -32768 to +32768 with 1/65536 precision.
type FDot16 = int32

// Fixed-point constants for FDot6.
const (
	// FDot6One is 1.0 in FDot6 representation (2^6 = 64).
	FDot6One FDot6 = 64

	// FDot6Half is 0.5 in FDot6 representation (2^5 = 32).
	FDot6Half FDot6 = 32

	// FDot6Shift is the number of fractional bits in FDot6.
	FDot6Shift = 6

	// FDot6Mask is the mask for the fractional part of FDot6.
	FDot6Mask = FDot6One - 1
)

// Fixed-point constants for FDot16.
const (
	// FDot16One is 1.0 in FDot16 representation (2^16 = 65536).
	FDot16One FDot16 = 1 << 16

	// FDot16Half is 0.5 in FDot16 representation (2^15 = 32768).
	FDot16Half FDot16 = 1 << 15

	// FDot16Shift is the number of fractional bits in FDot16.
	FDot16Shift = 16

	// FDot16Mask is the mask for the fractional part of FDot16.
	FDot16Mask = FDot16One - 1
)

// fdot6 namespace contains functions for FDot6 operations.
// Named after tiny-skia's fdot6 module.

// FDot6FromInt converts an integer to FDot6.
// The integer must fit in 26 bits (approximately -33M to +33M).
func FDot6FromInt(n int32) FDot6 {
	return n << FDot6Shift
}

// FDot6FromFloat32 converts a float32 to FDot6.
// Values are scaled by 64 and truncated toward zero.
func FDot6FromFloat32(f float32) FDot6 {
	return int32(f * float32(FDot6One))
}

// FDot6FromFloat64 converts a float64 to FDot6.
func FDot6FromFloat64(f float64) FDot6 {
	return int32(f * float64(FDot6One))
}

// FDot6ToFloat32 converts an FDot6 to float32.
func FDot6ToFloat32(v FDot6) float32 {
	return float32(v) / float32(FDot6One)
}

// FDot6ToFloat64 converts an FDot6 to float64.
func FDot6ToFloat64(v FDot6) float64 {
	return float64(v) / float64(FDot6One)
}

// FDot6Floor returns the integer part (floor) of an FDot6.
func FDot6Floor(v FDot6) int32 {
	return v >> FDot6Shift
}

// FDot6Ceil returns the ceiling of an FDot6.
func FDot6Ceil(v FDot6) int32 {
	return (v + FDot6Mask) >> FDot6Shift
}

// FDot6Round returns the nearest integer to an FDot6.
func FDot6Round(v FDot6) int32 {
	return (v + FDot6Half) >> FDot6Shift
}

// FDot6ToFDot16 converts an FDot6 to FDot16.
// This is a left shift by 10 bits (16 - 6 = 10).
func FDot6ToFDot16(v FDot6) FDot16 {
	return leftShift(v, FDot16Shift-FDot6Shift)
}

// FDot6Div divides two FDot6 values and returns an FDot16.
// This is used for computing slopes (dx / dy).
// If the numerator fits in 16 bits, uses fast path.
func FDot6Div(a, b FDot6) FDot16 {
	if b == 0 {
		// Return max value for division by zero (vertical line)
		if a >= 0 {
			return 0x7FFFFFFF
		}
		return -0x7FFFFFFF
	}

	// Check if a fits in 16 bits for fast path
	//nolint:gosec // Intentional overflow check
	if a == int32(int16(a)) {
		return leftShift(a, FDot16Shift) / b
	}

	// Slow path: use 64-bit intermediate
	return FDot16Div(a, b)
}

// FDot6CanConvertToFDot16 returns true if the FDot6 value can be
// converted to FDot16 without overflow.
func FDot6CanConvertToFDot16(v FDot6) bool {
	const maxDot6 = 0x7FFFFFFF >> (FDot16Shift - FDot6Shift)
	return v >= -maxDot6 && v <= maxDot6
}

// FDot6SmallScale scales a byte value by an FDot6 factor (0 to 64).
// Used for alpha blending calculations.
func FDot6SmallScale(value uint8, dot6 FDot6) uint8 {
	//nolint:gosec // Safe: result fits in uint8 when dot6 <= 64
	return uint8((int32(value) * dot6) >> FDot6Shift)
}

// fdot16 namespace contains functions for FDot16 operations.

// FDot16FromFloat32 converts a float32 to FDot16.
func FDot16FromFloat32(f float32) FDot16 {
	return saturateInt32(int64(f * float32(FDot16One)))
}

// FDot16FromFloat64 converts a float64 to FDot16.
func FDot16FromFloat64(f float64) FDot16 {
	return saturateInt32(int64(f * float64(FDot16One)))
}

// FDot16ToFloat32 converts an FDot16 to float32.
func FDot16ToFloat32(v FDot16) float32 {
	return float32(v) / float32(FDot16One)
}

// FDot16ToFloat64 converts an FDot16 to float64.
func FDot16ToFloat64(v FDot16) float64 {
	return float64(v) / float64(FDot16One)
}

// FDot16FloorToInt returns the integer part (floor) of an FDot16.
func FDot16FloorToInt(v FDot16) int32 {
	return v >> FDot16Shift
}

// FDot16CeilToInt returns the ceiling of an FDot16.
func FDot16CeilToInt(v FDot16) int32 {
	return (v + FDot16One - 1) >> FDot16Shift
}

// FDot16RoundToInt returns the nearest integer to an FDot16.
func FDot16RoundToInt(v FDot16) int32 {
	return (v + FDot16Half) >> FDot16Shift
}

// FDot16Mul multiplies two FDot16 values.
// Uses 64-bit intermediate to avoid overflow, then truncates.
func FDot16Mul(a, b FDot16) FDot16 {
	//nolint:gosec // Result clamped to int32 range
	return int32((int64(a) * int64(b)) >> FDot16Shift)
}

// FDot16Div divides two FDot6/FDot16 values and returns an FDot16.
// Uses 64-bit intermediate to avoid overflow.
func FDot16Div(numer, denom int32) FDot16 {
	if denom == 0 {
		if numer >= 0 {
			return 0x7FFFFFFF
		}
		return -0x7FFFFFFF
	}
	v := leftShift64(int64(numer), FDot16Shift) / int64(denom)
	return saturateInt32(v)
}

// FDot16FastDiv divides two FDot6 values using fast 32-bit math.
// Only valid when the numerator fits in 16 bits.
func FDot16FastDiv(a, b FDot6) FDot16 {
	if b == 0 {
		if a >= 0 {
			return 0x7FFFFFFF
		}
		return -0x7FFFFFFF
	}
	return leftShift(a, FDot16Shift) / b
}

// FDot8 is a 24.8 fixed-point number (8 fractional bits).
// Used for anti-aliased coverage values.
type FDot8 = int32

// FDot8FromFDot16 converts an FDot16 to FDot8 with rounding.
func FDot8FromFDot16(v FDot16) FDot8 {
	return (v + 0x80) >> 8
}

// Helper functions for fixed-point math.

// leftShift performs a left shift with sign preservation.
// Equivalent to Rust's left_shift function in tiny-skia.
func leftShift(v int32, shift int) int32 {
	if shift < 0 {
		return v >> uint(-shift)
	}
	return v << uint(shift)
}

// leftShift64 performs a left shift on int64 with sign preservation.
func leftShift64(v int64, shift int) int64 {
	if shift < 0 {
		return v >> uint(-shift)
	}
	return v << uint(shift)
}

// saturateInt32 clamps an int64 to int32 range.
func saturateInt32(v int64) int32 {
	const maxInt32 = 0x7FFFFFFF
	const minInt32 = -0x80000000

	if v > maxInt32 {
		return maxInt32
	}
	if v < minInt32 {
		return minInt32
	}
	return int32(v)
}

// absInt32 returns the absolute value of an int32.
func absInt32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// maxInt32 returns the maximum of two int32 values.
func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}


// FDot6ToFixedDiv2 converts an FDot6 to FDot16 divided by 2.
// This is used in quadratic edge setup to avoid overflow.
// The result is (value / 2) in FDot16 representation.
func FDot6ToFixedDiv2(v FDot6) FDot16 {
	// We want FDot6ToFDot16(value >> 1), but we don't want to lose
	// the LSB of value, so we perform a modified shift.
	// Shift by (16 - 6 - 1) = 9 instead of 10.
	return leftShift(v, FDot16Shift-FDot6Shift-1)
}

// FDot6UpShift performs a left shift on an FDot6 value.
// Used in cubic edge coefficient computation.
func FDot6UpShift(v FDot6, upShift int) int32 {
	return leftShift(v, upShift)
}
