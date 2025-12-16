package color

import "math"

// SRGBToLinear converts an sRGB component to linear (EOTF - Electro-Optical Transfer Function).
// Formula: if s <= 0.04045: s/12.92; else: pow((s+0.055)/1.055, 2.4)
// Input and output are in range [0,1].
func SRGBToLinear(s float32) float32 {
	if s <= 0.04045 {
		return s / 12.92
	}
	return float32(math.Pow(float64((s+0.055)/1.055), 2.4))
}

// LinearToSRGB converts a linear component to sRGB (OETF - Opto-Electronic Transfer Function).
// Formula: if l <= 0.0031308: l*12.92; else: 1.055*pow(l, 1/2.4)-0.055
// Input and output are in range [0,1].
func LinearToSRGB(l float32) float32 {
	if l <= 0.0031308 {
		return l * 12.92
	}
	return 1.055*float32(math.Pow(float64(l), 1.0/2.4)) - 0.055
}

// SRGBToLinearColor converts a full color from sRGB to linear space.
// Only RGB components are converted; alpha remains linear (never gamma-encoded).
func SRGBToLinearColor(c ColorF32) ColorF32 {
	return ColorF32{
		R: SRGBToLinear(c.R),
		G: SRGBToLinear(c.G),
		B: SRGBToLinear(c.B),
		A: c.A, // Alpha is always linear
	}
}

// LinearToSRGBColor converts a full color from linear to sRGB space.
// Only RGB components are converted; alpha remains linear (never gamma-encoded).
func LinearToSRGBColor(c ColorF32) ColorF32 {
	return ColorF32{
		R: LinearToSRGB(c.R),
		G: LinearToSRGB(c.G),
		B: LinearToSRGB(c.B),
		A: c.A, // Alpha is always linear
	}
}

// U8ToF32 converts ColorU8 to ColorF32.
// Each uint8 component [0,255] is mapped to float32 [0,1].
func U8ToF32(c ColorU8) ColorF32 {
	return ColorF32{
		R: float32(c.R) / 255.0,
		G: float32(c.G) / 255.0,
		B: float32(c.B) / 255.0,
		A: float32(c.A) / 255.0,
	}
}

// F32ToU8 converts ColorF32 to ColorU8.
// Each float32 component [0,1] is mapped to uint8 [0,255] with rounding.
func F32ToU8(c ColorF32) ColorU8 {
	return ColorU8{
		R: clampAndRound(c.R),
		G: clampAndRound(c.G),
		B: clampAndRound(c.B),
		A: clampAndRound(c.A),
	}
}

// clampAndRound clamps a float32 to [0,1] and converts to uint8 with rounding.
func clampAndRound(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	// Round to nearest integer
	return uint8(v*255.0 + 0.5)
}
