package gg

import (
	"math"
	"testing"
)

// TestMaskGamma_ApplyContrast verifies the core contrast boost formula
// (Skia SkMaskGamma apply_contrast pattern):
//
//	boosted = alpha + (1 - alpha) * contrast * alpha
//
// Properties:
//   - alpha=0 -> boosted=0 (transparent stays transparent)
//   - alpha=1 -> boosted=1 (full coverage stays full)
//   - 0 < alpha < 1 -> boosted >= alpha (coverage only increases)
//   - contrast=0 -> boosted=alpha (no change)
func TestMaskGamma_ApplyContrast(t *testing.T) {
	tests := []struct {
		name     string
		alpha    float64
		contrast float64
		want     float64
	}{
		// Zero alpha is unchanged regardless of contrast.
		{"zero_alpha_zero_contrast", 0.0, 0.0, 0.0},
		{"zero_alpha_half_contrast", 0.0, 0.5, 0.0},
		{"zero_alpha_full_contrast", 0.0, 1.0, 0.0},

		// Full coverage is unchanged regardless of contrast.
		{"full_alpha_zero_contrast", 1.0, 0.0, 1.0},
		{"full_alpha_half_contrast", 1.0, 0.5, 1.0},
		{"full_alpha_full_contrast", 1.0, 1.0, 1.0},

		// Zero contrast means no change.
		{"mid_alpha_zero_contrast", 0.5, 0.0, 0.5},
		{"quarter_alpha_zero_contrast", 0.25, 0.0, 0.25},

		// Mid-range alpha with Skia default contrast (0.5).
		// boosted = 0.5 + (1-0.5)*0.5*0.5 = 0.5 + 0.125 = 0.625
		{"mid_alpha_skia_contrast", 0.5, 0.5, 0.625},

		// Quarter alpha: boosted = 0.25 + 0.75*0.5*0.25 = 0.25 + 0.09375 = 0.34375
		{"quarter_alpha_skia_contrast", 0.25, 0.5, 0.34375},

		// Three-quarter alpha: boosted = 0.75 + 0.25*0.5*0.75 = 0.75 + 0.09375 = 0.84375
		{"three_quarter_alpha_skia_contrast", 0.75, 0.5, 0.84375},

		// Full contrast (1.0): boosted = a + (1-a)*1*a = a + a - a^2 = 2a - a^2
		// For a=0.5: 1.0 - 0.25 = 0.75
		{"mid_alpha_full_contrast", 0.5, 1.0, 0.75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyMaskGammaContrast(tt.alpha, tt.contrast)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("applyMaskGammaContrast(%v, %v) = %v, want %v",
					tt.alpha, tt.contrast, got, tt.want)
			}
		})
	}
}

// TestMaskGamma_LuminanceContrast verifies that text color luminance
// determines the contrast factor:
//   - Light text (high luminance) -> high contrast -> more boost
//   - Dark text (low luminance) -> low contrast -> less boost
//   - Black text -> zero contrast -> no boost at all
func TestMaskGamma_LuminanceContrast(t *testing.T) {
	tests := []struct {
		name       string
		r, g, b    float64 // text color (straight, 0-1)
		wantConMin float64 // minimum expected contrast
		wantConMax float64 // maximum expected contrast
	}{
		// Black text: luminance ~0 -> contrast ~0.
		{"black_text", 0.0, 0.0, 0.0, 0.0, 0.01},
		// White text: luminance ~1 -> contrast = maxContrast (0.5).
		{"white_text", 1.0, 1.0, 1.0, 0.49, 0.51},
		// Mid-gray: luminance ~0.5 -> contrast ~0.25.
		{"mid_gray_text", 0.5, 0.5, 0.5, 0.20, 0.30},
		// Light gray (#C0C0C0 = 0.75): luminance ~0.75 -> contrast ~0.375.
		{"light_gray_text", 0.75, 0.75, 0.75, 0.35, 0.40},
		// Dark gray (#333333 = 0.2): luminance ~0.2 -> contrast ~0.1.
		{"dark_gray_text", 0.2, 0.2, 0.2, 0.08, 0.12},
		// Pure red: luminance = 0.299 -> contrast ~0.15.
		{"red_text", 1.0, 0.0, 0.0, 0.13, 0.17},
		// Pure green: luminance = 0.587 -> contrast ~0.29.
		{"green_text", 0.0, 1.0, 0.0, 0.27, 0.32},
		// Pure blue: luminance = 0.114 -> contrast ~0.057.
		{"blue_text", 0.0, 0.0, 1.0, 0.04, 0.08},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contrast := maskGammaContrastForColor(tt.r, tt.g, tt.b)
			if contrast < tt.wantConMin || contrast > tt.wantConMax {
				t.Errorf("maskGammaContrastForColor(%v,%v,%v) = %v, want [%v, %v]",
					tt.r, tt.g, tt.b, contrast, tt.wantConMin, tt.wantConMax)
			}
		})
	}
}

// TestMaskGamma_FullPipeline_LightOnDark verifies that light text on a dark
// background gets a coverage boost. This is the key integration test.
//
// Render light gray text (#C0C0C0) on dark bg. With mask gamma, edge pixels
// (alpha ~0.5) should be boosted to ~0.59 (contrast ~0.375 for lum 0.75).
func TestMaskGamma_FullPipeline_LightOnDark(t *testing.T) {
	// Light gray text: r=g=b=0.75
	alpha := 0.5
	r, g, b := 0.75, 0.75, 0.75

	contrast := maskGammaContrastForColor(r, g, b)
	boosted := applyMaskGammaContrast(alpha, contrast)

	// Must be strictly greater than original alpha.
	if boosted <= alpha {
		t.Errorf("light text on dark bg: boosted=%v should be > alpha=%v", boosted, alpha)
	}

	// Boost should be meaningful (> 10% relative increase for mid-alpha).
	relativeBoost := (boosted - alpha) / alpha
	if relativeBoost < 0.10 {
		t.Errorf("light text: relative boost = %.1f%%, want >= 10%%", relativeBoost*100)
	}

	t.Logf("light text (lum=0.75): contrast=%.3f, alpha=%.3f -> boosted=%.3f (%.1f%% boost)",
		contrast, alpha, boosted, relativeBoost*100)
}

// TestMaskGamma_FullPipeline_DarkOnLight verifies that dark text on a light
// background gets minimal or no coverage boost.
func TestMaskGamma_FullPipeline_DarkOnLight(t *testing.T) {
	// Dark text: r=g=b=0.2 (#333333)
	alpha := 0.5
	r, g, b := 0.2, 0.2, 0.2

	contrast := maskGammaContrastForColor(r, g, b)
	boosted := applyMaskGammaContrast(alpha, contrast)

	// Boost should be minimal (< 6% relative increase for dark text).
	// For r=g=b=0.2: luminance=0.2, contrast=0.1, boost at alpha=0.5 is 5.0%.
	relativeBoost := (boosted - alpha) / alpha
	if relativeBoost > 0.06 {
		t.Errorf("dark text on light bg: relative boost = %.1f%%, want < 6%%", relativeBoost*100)
	}

	t.Logf("dark text (lum=0.2): contrast=%.3f, alpha=%.3f -> boosted=%.3f (%.1f%% boost)",
		contrast, alpha, boosted, relativeBoost*100)
}

// TestMaskGamma_FullPipeline_BlackText_NoBoost verifies that pure black text
// gets zero boost (contrast=0).
func TestMaskGamma_FullPipeline_BlackText_NoBoost(t *testing.T) {
	alpha := 0.5

	contrast := maskGammaContrastForColor(0, 0, 0)
	boosted := applyMaskGammaContrast(alpha, contrast)

	if math.Abs(boosted-alpha) > 1e-9 {
		t.Errorf("black text: boosted=%v, want exactly %v (zero contrast)", boosted, alpha)
	}
}

// TestMaskGamma_Monotonicity verifies that the boost function is monotonically
// increasing in alpha for any valid contrast.
func TestMaskGamma_Monotonicity(t *testing.T) {
	contrasts := []float64{0.0, 0.1, 0.25, 0.5, 1.0}

	for _, c := range contrasts {
		prev := 0.0
		for i := 0; i <= 100; i++ {
			alpha := float64(i) / 100.0
			boosted := applyMaskGammaContrast(alpha, c)
			if boosted < prev-1e-12 {
				t.Errorf("contrast=%v: non-monotonic at alpha=%v: boosted=%v < prev=%v",
					c, alpha, boosted, prev)
			}
			prev = boosted
		}
	}
}

// TestMaskGamma_BoundedOutput verifies that output is always in [0, 1].
func TestMaskGamma_BoundedOutput(t *testing.T) {
	for ci := 0; ci <= 10; ci++ {
		c := float64(ci) / 10.0
		for ai := 0; ai <= 100; ai++ {
			a := float64(ai) / 100.0
			boosted := applyMaskGammaContrast(a, c)
			if boosted < 0 || boosted > 1.0 {
				t.Errorf("out of bounds: applyMaskGammaContrast(%v, %v) = %v", a, c, boosted)
			}
		}
	}
}

// BenchmarkApplyMaskGammaContrast measures the overhead of the contrast
// boost calculation (should be negligible per-glyph).
func BenchmarkApplyMaskGammaContrast(b *testing.B) {
	for i := 0; i < b.N; i++ {
		alpha := float64(i%256) / 255.0
		_ = applyMaskGammaContrast(alpha, 0.5)
	}
}
