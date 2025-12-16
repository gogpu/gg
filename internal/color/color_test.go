package color

import (
	"math"
	"testing"
)

// TestSRGBToLinearEdgeCases tests edge cases for sRGB to linear conversion.
func TestSRGBToLinearEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input float32
		want  float32
	}{
		{"black", 0.0, 0.0},
		{"white", 1.0, 1.0},
		{"threshold", 0.04045, 0.04045 / 12.92},
		{"just above threshold", 0.04046, float32(math.Pow((0.04046+0.055)/1.055, 2.4))},
		{"mid gray", 0.5, float32(math.Pow((0.5+0.055)/1.055, 2.4))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SRGBToLinear(tt.input)
			if !floatNear(got, tt.want, 1e-6) {
				t.Errorf("SRGBToLinear(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestLinearToSRGBEdgeCases tests edge cases for linear to sRGB conversion.
func TestLinearToSRGBEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input float32
		want  float32
	}{
		{"black", 0.0, 0.0},
		{"white", 1.0, 1.0},
		{"threshold", 0.0031308, 0.0031308 * 12.92},
		{"just above threshold", 0.0031309, 1.055*float32(math.Pow(0.0031309, 1.0/2.4)) - 0.055},
		{"mid gray linear", 0.21404, float32(1.055*math.Pow(0.21404, 1.0/2.4) - 0.055)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LinearToSRGB(tt.input)
			if !floatNear(got, tt.want, 1e-6) {
				t.Errorf("LinearToSRGB(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestRoundTripSRGBLinear tests round-trip conversion accuracy.
// Maximum error should be less than 1/255 to preserve 8-bit precision.
func TestRoundTripSRGBLinear(t *testing.T) {
	const maxError = 1.0 / 255.0

	// Test all 8-bit values
	for i := 0; i <= 255; i++ {
		srgb := float32(i) / 255.0
		linear := SRGBToLinear(srgb)
		roundTrip := LinearToSRGB(linear)

		diff := float32(math.Abs(float64(roundTrip - srgb)))
		if diff > maxError {
			t.Errorf("Round-trip error for %d/255: got %v, want %v, diff %v (max %v)",
				i, roundTrip, srgb, diff, maxError)
		}
	}
}

// TestRoundTripLinearSRGB tests reverse round-trip conversion accuracy.
func TestRoundTripLinearSRGB(t *testing.T) {
	const maxError = 1.0 / 255.0

	// Test 256 evenly spaced linear values
	for i := 0; i <= 255; i++ {
		linear := float32(i) / 255.0
		srgb := LinearToSRGB(linear)
		roundTrip := SRGBToLinear(srgb)

		diff := float32(math.Abs(float64(roundTrip - linear)))
		if diff > maxError {
			t.Errorf("Reverse round-trip error for %d/255: got %v, want %v, diff %v (max %v)",
				i, roundTrip, linear, diff, maxError)
		}
	}
}

// TestSRGBToLinearColor tests full color conversion to linear space.
func TestSRGBToLinearColor(t *testing.T) {
	tests := []struct {
		name  string
		input ColorF32
		want  ColorF32
	}{
		{
			name:  "opaque white",
			input: ColorF32{R: 1.0, G: 1.0, B: 1.0, A: 1.0},
			want:  ColorF32{R: 1.0, G: 1.0, B: 1.0, A: 1.0},
		},
		{
			name:  "opaque black",
			input: ColorF32{R: 0.0, G: 0.0, B: 0.0, A: 1.0},
			want:  ColorF32{R: 0.0, G: 0.0, B: 0.0, A: 1.0},
		},
		{
			name:  "semi-transparent red",
			input: ColorF32{R: 1.0, G: 0.0, B: 0.0, A: 0.5},
			want:  ColorF32{R: 1.0, G: 0.0, B: 0.0, A: 0.5}, // Alpha unchanged
		},
		{
			name:  "mid gray with alpha",
			input: ColorF32{R: 0.5, G: 0.5, B: 0.5, A: 0.75},
			want: ColorF32{
				R: SRGBToLinear(0.5),
				G: SRGBToLinear(0.5),
				B: SRGBToLinear(0.5),
				A: 0.75, // Alpha unchanged
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SRGBToLinearColor(tt.input)
			if !colorF32Near(got, tt.want, 1e-6) {
				t.Errorf("SRGBToLinearColor(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestLinearToSRGBColor tests full color conversion to sRGB space.
func TestLinearToSRGBColor(t *testing.T) {
	tests := []struct {
		name  string
		input ColorF32
		want  ColorF32
	}{
		{
			name:  "opaque white",
			input: ColorF32{R: 1.0, G: 1.0, B: 1.0, A: 1.0},
			want:  ColorF32{R: 1.0, G: 1.0, B: 1.0, A: 1.0},
		},
		{
			name:  "opaque black",
			input: ColorF32{R: 0.0, G: 0.0, B: 0.0, A: 1.0},
			want:  ColorF32{R: 0.0, G: 0.0, B: 0.0, A: 1.0},
		},
		{
			name:  "semi-transparent green",
			input: ColorF32{R: 0.0, G: 1.0, B: 0.0, A: 0.3},
			want:  ColorF32{R: 0.0, G: 1.0, B: 0.0, A: 0.3}, // Alpha unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LinearToSRGBColor(tt.input)
			if !colorF32Near(got, tt.want, 1e-6) {
				t.Errorf("LinearToSRGBColor(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestAlphaPreserved ensures alpha is never gamma-encoded.
func TestAlphaPreserved(t *testing.T) {
	input := ColorF32{R: 0.5, G: 0.5, B: 0.5, A: 0.5}

	// Convert to linear
	linear := SRGBToLinearColor(input)
	if linear.A != input.A {
		t.Errorf("SRGBToLinearColor changed alpha: got %v, want %v", linear.A, input.A)
	}

	// Convert to sRGB
	srgb := LinearToSRGBColor(linear)
	if srgb.A != input.A {
		t.Errorf("LinearToSRGBColor changed alpha: got %v, want %v", srgb.A, input.A)
	}
}

// TestU8ToF32 tests uint8 to float32 conversion.
func TestU8ToF32(t *testing.T) {
	tests := []struct {
		name  string
		input ColorU8
		want  ColorF32
	}{
		{
			name:  "black",
			input: ColorU8{R: 0, G: 0, B: 0, A: 0},
			want:  ColorF32{R: 0.0, G: 0.0, B: 0.0, A: 0.0},
		},
		{
			name:  "white",
			input: ColorU8{R: 255, G: 255, B: 255, A: 255},
			want:  ColorF32{R: 1.0, G: 1.0, B: 1.0, A: 1.0},
		},
		{
			name:  "mid values",
			input: ColorU8{R: 128, G: 64, B: 192, A: 255},
			want:  ColorF32{R: 128.0 / 255.0, G: 64.0 / 255.0, B: 192.0 / 255.0, A: 1.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := U8ToF32(tt.input)
			if !colorF32Near(got, tt.want, 1e-6) {
				t.Errorf("U8ToF32(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestF32ToU8 tests float32 to uint8 conversion.
func TestF32ToU8(t *testing.T) {
	tests := []struct {
		name  string
		input ColorF32
		want  ColorU8
	}{
		{
			name:  "black",
			input: ColorF32{R: 0.0, G: 0.0, B: 0.0, A: 0.0},
			want:  ColorU8{R: 0, G: 0, B: 0, A: 0},
		},
		{
			name:  "white",
			input: ColorF32{R: 1.0, G: 1.0, B: 1.0, A: 1.0},
			want:  ColorU8{R: 255, G: 255, B: 255, A: 255},
		},
		{
			name:  "mid values with rounding",
			input: ColorF32{R: 0.5, G: 0.25, B: 0.75, A: 1.0},
			want:  ColorU8{R: 128, G: 64, B: 191, A: 255}, // 0.5*255=127.5→128, 0.25*255=63.75→64, 0.75*255=191.25→191
		},
		{
			name:  "clamping below 0",
			input: ColorF32{R: -0.1, G: 0.0, B: 0.0, A: 0.0},
			want:  ColorU8{R: 0, G: 0, B: 0, A: 0},
		},
		{
			name:  "clamping above 1",
			input: ColorF32{R: 1.5, G: 1.0, B: 1.0, A: 1.0},
			want:  ColorU8{R: 255, G: 255, B: 255, A: 255},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := F32ToU8(tt.input)
			if got != tt.want {
				t.Errorf("F32ToU8(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestRoundTripU8F32 tests round-trip conversion between ColorU8 and ColorF32.
func TestRoundTripU8F32(t *testing.T) {
	// Test all possible uint8 values
	for r := 0; r <= 255; r++ {
		for g := 0; g <= 255; g += 51 { // Sample every 51 to reduce test time
			for b := 0; b <= 255; b += 51 {
				for a := 0; a <= 255; a += 51 {
					original := ColorU8{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
					f32 := U8ToF32(original)
					roundTrip := F32ToU8(f32)

					if roundTrip != original {
						t.Errorf("Round-trip U8→F32→U8 failed: %v → %v → %v",
							original, f32, roundTrip)
					}
				}
			}
		}
	}
}

// TestF32ToU8Rounding tests correct rounding behavior.
func TestF32ToU8Rounding(t *testing.T) {
	tests := []struct {
		name  string
		input float32
		want  uint8
	}{
		{"0.0", 0.0, 0},
		{"1.0", 1.0, 255},
		{"0.5 rounds to 128", 0.5, 128},               // 0.5 * 255 = 127.5 → 128
		{"127/255 rounds to 127", 127.0 / 255.0, 127}, // Exact value
		{"128/255 rounds to 128", 128.0 / 255.0, 128}, // Exact value
		{"just below 0.5", 127.0 / 255.0, 127},
		{"just above 0.5", 128.0 / 255.0, 128},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color := ColorF32{R: tt.input, G: 0, B: 0, A: 0}
			got := F32ToU8(color)
			if got.R != tt.want {
				t.Errorf("F32ToU8(R=%v).R = %v, want %v", tt.input, got.R, tt.want)
			}
		})
	}
}

// floatNear checks if two float32 values are within epsilon of each other.
func floatNear(a, b, epsilon float32) bool {
	return math.Abs(float64(a-b)) < float64(epsilon)
}

// colorF32Near checks if two ColorF32 values are within epsilon of each other.
func colorF32Near(a, b ColorF32, epsilon float32) bool {
	return floatNear(a.R, b.R, epsilon) &&
		floatNear(a.G, b.G, epsilon) &&
		floatNear(a.B, b.B, epsilon) &&
		floatNear(a.A, b.A, epsilon)
}
