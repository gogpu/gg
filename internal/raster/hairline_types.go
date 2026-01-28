// Package raster provides scanline rasterization for 2D paths.
// This file implements fixed-point types for anti-aliased hairline rendering.
// Based on tiny-skia's fixed_point.rs (Android/Skia heritage).
package raster

// FDot6 is a 26.6 fixed-point type for pixel coordinates.
// The 6-bit fractional part provides 64 subpixel positions per pixel.
type FDot6 int32

// FDot16 is a 16.16 fixed-point type for slopes and interpolation.
// Used for precise calculations that need more fractional bits.
type FDot16 int32

// Fixed-point constants for FDot6 (26.6 format).
const (
	// FDot6Shift is the number of fractional bits in FDot6.
	FDot6Shift = 6
	// FDot6One represents 1.0 in FDot6 format (64).
	FDot6One FDot6 = 1 << FDot6Shift
	// FDot6Mask is used to extract the fractional part.
	FDot6Mask = FDot6One - 1
)

// Fixed-point constants for FDot16 (16.16 format).
const (
	// FDot16Shift is the number of fractional bits in FDot16.
	FDot16Shift = 16
	// FDot16One represents 1.0 in FDot16 format (65536).
	FDot16One FDot16 = 1 << FDot16Shift
	// FDot16Half represents 0.5 in FDot16 format.
	FDot16Half FDot16 = FDot16One / 2
)

// FloatToFDot6 converts a float64 to FDot6 fixed-point.
func FloatToFDot6(f float64) FDot6 {
	return FDot6(f * float64(FDot6One))
}

// FloatToFDot16 converts a float64 to FDot16 fixed-point.
func FloatToFDot16(f float64) FDot16 {
	return FDot16(f * float64(FDot16One))
}

// FDot6ToFloat converts FDot6 to float64.
func FDot6ToFloat(f FDot6) float64 {
	return float64(f) / float64(FDot6One)
}

// FDot16ToFloat converts FDot16 to float64.
func FDot16ToFloat(f FDot16) float64 {
	return float64(f) / float64(FDot16One)
}

// FDot6Floor returns the floor of an FDot6 value as an integer.
func FDot6Floor(f FDot6) int {
	return int(f >> FDot6Shift)
}

// FDot6Ceil returns the ceiling of an FDot6 value as an integer.
func FDot6Ceil(f FDot6) int {
	return int((f + FDot6Mask) >> FDot6Shift)
}

// FDot6Round returns the rounded value of an FDot6 as an integer.
func FDot6Round(f FDot6) int {
	return int((f + FDot6One/2) >> FDot6Shift)
}

// FDot6ToFDot16 converts FDot6 to FDot16 (shifts left by 10 bits).
func FDot6ToFDot16(f FDot6) FDot16 {
	return FDot16(f) << (FDot16Shift - FDot6Shift)
}

// FDot16Floor returns the floor of an FDot16 value as an integer.
func FDot16Floor(f FDot16) int {
	return int(f >> FDot16Shift)
}

// FDot16Ceil returns the ceiling of an FDot16 value as an integer.
func FDot16Ceil(f FDot16) int {
	return int((f + FDot16One - 1) >> FDot16Shift)
}

// FDot16FastDiv computes (a << 16) / b for fixed-point division.
// Used for computing slopes. Requires b != 0.
// The result is always valid FDot16 since inputs are bounded by design.
//
//nolint:gosec // Result is bounded by input constraints
func FDot16FastDiv(a, b FDot6) FDot16 {
	if b == 0 {
		return 0
	}
	return FDot16((int64(a) << FDot16Shift) / int64(b))
}

// FDot6SmallScale scales a uint8 value by an FDot6 value in range [0, 64].
// This is used to compute partial pixel coverage.
// Result is always in [0, 255] since: max(uint8)*64/64 = 255.
//
//nolint:gosec // Result is bounded: (255 * 64) >> 6 = 255
func FDot6SmallScale(value uint8, dot6 FDot6) uint8 {
	// dot6 should be in range [0, 64]
	return uint8((int32(value) * int32(dot6)) >> FDot6Shift)
}

// Abs6 returns the absolute value of an FDot6.
func Abs6(f FDot6) FDot6 {
	if f < 0 {
		return -f
	}
	return f
}

// Abs16 returns the absolute value of an FDot16.
func Abs16(f FDot16) FDot16 {
	if f < 0 {
		return -f
	}
	return f
}

// Min6 returns the minimum of two FDot6 values.
func Min6(a, b FDot6) FDot6 {
	if a < b {
		return a
	}
	return b
}

// Max6 returns the maximum of two FDot6 values.
func Max6(a, b FDot6) FDot6 {
	if a > b {
		return a
	}
	return b
}

// Contribution64 returns the fractional part of an FDot6 ordinate,
// but returns 64 for exact integer positions instead of 0.
// This is used for calculating partial pixel coverage at endpoints.
func Contribution64(ordinate FDot6) FDot6 {
	// We want multiples of 64 to return 64, not 0.
	// ((ordinate - 1) & 63) + 1 achieves this:
	// - ordinate=0: ((-1) & 63) + 1 = 63 + 1 = 64 (but 0 is special, typically not used)
	// - ordinate=64: ((63) & 63) + 1 = 63 + 1 = 64
	// - ordinate=32: ((31) & 63) + 1 = 31 + 1 = 32
	result := ((ordinate - 1) & FDot6Mask) + 1
	return result
}
