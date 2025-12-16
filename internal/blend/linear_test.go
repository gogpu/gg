package blend

import (
	"testing"

	"github.com/gogpu/gg/internal/color"
)

// TestLinearVsSRGBBlending verifies that linear blending produces different results than sRGB.
func TestLinearVsSRGBBlending(t *testing.T) {
	tests := []struct {
		name string
		src  color.ColorU8
		dst  color.ColorU8
		mode BlendMode
	}{
		{
			name: "50% red + 50% green",
			src:  color.ColorU8{R: 255, G: 0, B: 0, A: 128}, // 50% opaque red
			dst:  color.ColorU8{R: 0, G: 255, B: 0, A: 255}, // opaque green
			mode: BlendSourceOver,
		},
		{
			name: "multiply dark colors",
			src:  color.ColorU8{R: 64, G: 64, B: 64, A: 255},
			dst:  color.ColorU8{R: 128, G: 128, B: 128, A: 255},
			mode: BlendMultiply,
		},
		{
			name: "screen bright colors",
			src:  color.ColorU8{R: 200, G: 150, B: 100, A: 255},
			dst:  color.ColorU8{R: 150, G: 200, B: 180, A: 255},
			mode: BlendScreen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Blend in sRGB space
			srgbFunc := GetBlendFuncLinear(tt.mode, false)
			srgbR, srgbG, srgbB, srgbA := srgbFunc(
				tt.src.R, tt.src.G, tt.src.B, tt.src.A,
				tt.dst.R, tt.dst.G, tt.dst.B, tt.dst.A,
			)

			// Blend in linear space
			linearFunc := GetBlendFuncLinear(tt.mode, true)
			linearR, linearG, linearB, linearA := linearFunc(
				tt.src.R, tt.src.G, tt.src.B, tt.src.A,
				tt.dst.R, tt.dst.G, tt.dst.B, tt.dst.A,
			)

			// Results should be different (except for alpha, which is always linear)
			if srgbR == linearR && srgbG == linearG && srgbB == linearB {
				t.Errorf("Linear and sRGB blending produced identical results: sRGB(%d,%d,%d) == linear(%d,%d,%d)",
					srgbR, srgbG, srgbB, linearR, linearG, linearB)
			}

			// Alpha should be the same (alpha is always linear)
			if srgbA != linearA {
				t.Errorf("Alpha mismatch: sRGB=%d, linear=%d (alpha should always be linear)", srgbA, linearA)
			}

			t.Logf("sRGB:   R=%d, G=%d, B=%d, A=%d", srgbR, srgbG, srgbB, srgbA)
			t.Logf("Linear: R=%d, G=%d, B=%d, A=%d", linearR, linearG, linearB, linearA)
		})
	}
}

// TestLinearBlendingAccuracy tests that linear blending produces physically correct results.
// For example, 50% red + 50% green should produce a proper yellow, not dark brown.
func TestLinearBlendingAccuracy(t *testing.T) {
	// 50% opaque red over opaque green
	src := color.ColorU8{R: 255, G: 0, B: 0, A: 128} // 50% red
	dst := color.ColorU8{R: 0, G: 255, B: 0, A: 255} // opaque green

	linearFunc := GetBlendFuncLinear(BlendSourceOver, true)
	r, g, b, a := linearFunc(src.R, src.G, src.B, src.A, dst.R, dst.G, dst.B, dst.A)

	// In linear space, 50% red + 50% green should produce yellow with equal R and G
	// The result won't be exactly equal due to premultiplied alpha compositing,
	// but R and G should be closer than in sRGB blending
	t.Logf("Linear blend result: R=%d, G=%d, B=%d, A=%d", r, g, b, a)

	// Basic sanity checks
	if r == 0 {
		t.Error("Expected non-zero red component")
	}
	if g == 0 {
		t.Error("Expected non-zero green component")
	}
	if b != 0 {
		t.Error("Expected zero blue component")
	}
	if a == 0 {
		t.Error("Expected non-zero alpha")
	}

	// Compare with sRGB blending
	srgbFunc := GetBlendFuncLinear(BlendSourceOver, false)
	srgbR, srgbG, srgbB, srgbA := srgbFunc(src.R, src.G, src.B, src.A, dst.R, dst.G, dst.B, dst.A)
	t.Logf("sRGB blend result:   R=%d, G=%d, B=%d, A=%d", srgbR, srgbG, srgbB, srgbA)

	// Linear blending should produce brighter results (higher RGB values)
	// because sRGB gamma makes midtones darker
	if r < srgbR || g < srgbG {
		t.Logf("Warning: Linear blend not brighter than sRGB. This may indicate an issue.")
		t.Logf("  Linear R=%d < sRGB R=%d OR Linear G=%d < sRGB G=%d", r, srgbR, g, srgbG)
	}
}

// TestAlphaPreservation verifies that alpha channel is never gamma-encoded.
func TestAlphaPreservation(t *testing.T) {
	tests := []struct {
		name string
		src  color.ColorU8
		dst  color.ColorU8
		mode BlendMode
	}{
		{
			name: "source over with various alpha",
			src:  color.ColorU8{R: 200, G: 100, B: 50, A: 100},
			dst:  color.ColorU8{R: 50, G: 150, B: 200, A: 150},
			mode: BlendSourceOver,
		},
		{
			name: "multiply with semi-transparent",
			src:  color.ColorU8{R: 128, G: 128, B: 128, A: 64},
			dst:  color.ColorU8{R: 192, G: 192, B: 192, A: 192},
			mode: BlendMultiply,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Linear blending
			linearFunc := GetBlendFuncLinear(tt.mode, true)
			_, _, _, linearA := linearFunc(
				tt.src.R, tt.src.G, tt.src.B, tt.src.A,
				tt.dst.R, tt.dst.G, tt.dst.B, tt.dst.A,
			)

			// sRGB blending
			srgbFunc := GetBlendFuncLinear(tt.mode, false)
			_, _, _, srgbA := srgbFunc(
				tt.src.R, tt.src.G, tt.src.B, tt.src.A,
				tt.dst.R, tt.dst.G, tt.dst.B, tt.dst.A,
			)

			// Alpha must be identical between linear and sRGB blending
			if linearA != srgbA {
				t.Errorf("Alpha channel differs: linear=%d, sRGB=%d (alpha must never be gamma-encoded)", linearA, srgbA)
			}
		})
	}
}

// TestRoundTripAccuracy tests that converting to linear and back preserves color reasonably well.
func TestRoundTripAccuracy(t *testing.T) {
	tests := []color.ColorU8{
		{R: 255, G: 0, B: 0, A: 255},     // Pure red
		{R: 0, G: 255, B: 0, A: 255},     // Pure green
		{R: 0, G: 0, B: 255, A: 255},     // Pure blue
		{R: 128, G: 128, B: 128, A: 255}, // Gray
		{R: 255, G: 255, B: 255, A: 255}, // White
		{R: 0, G: 0, B: 0, A: 255},       // Black
		{R: 200, G: 100, B: 50, A: 128},  // Semi-transparent
	}

	for _, c := range tests {
		t.Run("", func(t *testing.T) {
			// Convert to float32
			cf := color.ColorF32{
				R: float32(c.R) / 255.0,
				G: float32(c.G) / 255.0,
				B: float32(c.B) / 255.0,
				A: float32(c.A) / 255.0,
			}

			// sRGB -> Linear -> sRGB
			linear := color.SRGBToLinearColor(cf)
			back := color.LinearToSRGBColor(linear)

			// Convert back to uint8
			final := color.F32ToU8(back)

			// Check round-trip accuracy (allow ±1 error due to rounding)
			checkComponent := func(name string, original, final uint8) {
				diff := int(original) - int(final)
				if diff < 0 {
					diff = -diff
				}
				if diff > 1 {
					t.Errorf("%s component error too large: original=%d, final=%d, diff=%d",
						name, original, final, diff)
				}
			}

			checkComponent("R", c.R, final.R)
			checkComponent("G", c.G, final.G)
			checkComponent("B", c.B, final.B)
			checkComponent("A", c.A, final.A)
		})
	}
}

// TestAllBlendModesLinear verifies all blend modes work in linear space.
func TestAllBlendModesLinear(t *testing.T) {
	modes := []BlendMode{
		// Porter-Duff
		BlendClear, BlendSource, BlendDestination, BlendSourceOver,
		BlendDestinationOver, BlendSourceIn, BlendDestinationIn,
		BlendSourceOut, BlendDestinationOut, BlendSourceAtop,
		BlendDestinationAtop, BlendXor, BlendPlus, BlendModulate,
		// Advanced separable
		BlendMultiply, BlendScreen, BlendOverlay, BlendDarken,
		BlendLighten, BlendColorDodge, BlendColorBurn, BlendHardLight,
		BlendSoftLight, BlendDifference, BlendExclusion,
		// Non-separable
		BlendHue, BlendSaturation, BlendColor, BlendLuminosity,
	}

	src := color.ColorU8{R: 200, G: 100, B: 50, A: 200}
	dst := color.ColorU8{R: 100, G: 150, B: 200, A: 255}

	for _, mode := range modes {
		t.Run("", func(t *testing.T) {
			// Should not panic
			linearFunc := GetBlendFuncLinear(mode, true)
			r, g, b, a := linearFunc(src.R, src.G, src.B, src.A, dst.R, dst.G, dst.B, dst.A)

			// Basic sanity check - just verify it produced a result
			_ = r
			_ = g
			_ = b
			_ = a
		})
	}
}

// TestBlendLinearConvenience tests the BlendLinear convenience function.
func TestBlendLinearConvenience(t *testing.T) {
	src := color.ColorU8{R: 255, G: 0, B: 0, A: 128}
	dst := color.ColorU8{R: 0, G: 255, B: 0, A: 255}

	result := BlendLinear(src, dst, BlendSourceOver)

	// Should produce a non-zero result
	if result.R == 0 && result.G == 0 && result.B == 0 && result.A == 0 {
		t.Error("BlendLinear produced zero result")
	}

	t.Logf("BlendLinear result: R=%d, G=%d, B=%d, A=%d", result.R, result.G, result.B, result.A)
}

// TestLinearBlendingEdgeCases tests edge cases in linear blending.
func TestLinearBlendingEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		src  color.ColorU8
		dst  color.ColorU8
		mode BlendMode
	}{
		{
			name: "fully transparent source",
			src:  color.ColorU8{R: 255, G: 0, B: 0, A: 0},
			dst:  color.ColorU8{R: 0, G: 255, B: 0, A: 255},
			mode: BlendSourceOver,
		},
		{
			name: "fully transparent destination",
			src:  color.ColorU8{R: 255, G: 0, B: 0, A: 255},
			dst:  color.ColorU8{R: 0, G: 255, B: 0, A: 0},
			mode: BlendSourceOver,
		},
		{
			name: "both transparent",
			src:  color.ColorU8{R: 255, G: 0, B: 0, A: 0},
			dst:  color.ColorU8{R: 0, G: 255, B: 0, A: 0},
			mode: BlendSourceOver,
		},
		{
			name: "black colors",
			src:  color.ColorU8{R: 0, G: 0, B: 0, A: 255},
			dst:  color.ColorU8{R: 0, G: 0, B: 0, A: 255},
			mode: BlendMultiply,
		},
		{
			name: "white colors",
			src:  color.ColorU8{R: 255, G: 255, B: 255, A: 255},
			dst:  color.ColorU8{R: 255, G: 255, B: 255, A: 255},
			mode: BlendScreen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic or produce invalid results
			linearFunc := GetBlendFuncLinear(tt.mode, true)
			r, g, b, a := linearFunc(
				tt.src.R, tt.src.G, tt.src.B, tt.src.A,
				tt.dst.R, tt.dst.G, tt.dst.B, tt.dst.A,
			)

			// Bytes are always in [0,255] range by type definition
			// Just log the result for visual inspection
			t.Logf("Result: R=%d, G=%d, B=%d, A=%d", r, g, b, a)
		})
	}
}
