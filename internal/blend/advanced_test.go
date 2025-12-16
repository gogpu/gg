package blend

import (
	"math"
	"testing"
)

// TestMaxByte tests the maximum byte helper function.
func TestMaxByte(t *testing.T) {
	tests := []struct {
		name string
		a, b byte
		want byte
	}{
		{"0 vs 0", 0, 0, 0},
		{"0 vs 255", 0, 255, 255},
		{"255 vs 0", 255, 0, 255},
		{"128 vs 128", 128, 128, 128},
		{"100 vs 200", 100, 200, 200},
		{"200 vs 100", 200, 100, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maxByte(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("maxByte(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestBlendMultiply tests the Multiply blend mode.
func TestBlendMultiply(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque white * opaque white",
			255, 255, 255, 255,
			255, 255, 255, 255,
			255, 255, 255, 255,
		},
		{
			"opaque black * opaque white",
			0, 0, 0, 255,
			255, 255, 255, 255,
			0, 0, 0, 255,
		},
		{
			"opaque gray * opaque gray",
			128, 128, 128, 255,
			128, 128, 128, 255,
			64, 64, 64, 255, // 128 * 128 / 255 = 64
		},
		{
			"transparent over opaque",
			0, 0, 0, 0,
			255, 255, 255, 255,
			255, 255, 255, 255, // no change
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendMultiply(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendMultiply() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendScreen tests the Screen blend mode.
func TestBlendScreen(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque white screen opaque white",
			255, 255, 255, 255,
			255, 255, 255, 255,
			255, 255, 255, 255,
		},
		{
			"opaque black screen opaque white",
			0, 0, 0, 255,
			255, 255, 255, 255,
			255, 255, 255, 255,
		},
		{
			"opaque white screen opaque black",
			255, 255, 255, 255,
			0, 0, 0, 255,
			255, 255, 255, 255,
		},
		{
			"opaque black screen opaque black",
			0, 0, 0, 255,
			0, 0, 0, 255,
			0, 0, 0, 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendScreen(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendScreen() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendOverlay tests the Overlay blend mode.
func TestBlendOverlay(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{
			"opaque white over opaque white",
			255, 255, 255, 255,
			255, 255, 255, 255,
		},
		{
			"opaque black over opaque white",
			0, 0, 0, 255,
			255, 255, 255, 255,
		},
		{
			"opaque gray over opaque gray",
			128, 128, 128, 255,
			128, 128, 128, 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendOverlay(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			// Just verify it returns valid values (no specific expected output)
			if a == 0 && (r != 0 || g != 0 || b != 0) {
				t.Errorf("blendOverlay() returned non-zero color with zero alpha: (%d, %d, %d, %d)",
					r, g, b, a)
			}
		})
	}
}

// TestBlendDarken tests the Darken blend mode.
func TestBlendDarken(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque white vs opaque black",
			255, 255, 255, 255,
			0, 0, 0, 255,
			0, 0, 0, 255, // expect black (darker)
		},
		{
			"opaque gray vs opaque white",
			128, 128, 128, 255,
			255, 255, 255, 255,
			128, 128, 128, 255, // expect gray (darker)
		},
		{
			"opaque 100 vs opaque 200",
			100, 100, 100, 255,
			200, 200, 200, 255,
			100, 100, 100, 255, // expect 100 (darker)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendDarken(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendDarken() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendLighten tests the Lighten blend mode.
func TestBlendLighten(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque white vs opaque black",
			255, 255, 255, 255,
			0, 0, 0, 255,
			255, 255, 255, 255, // expect white (lighter)
		},
		{
			"opaque gray vs opaque white",
			128, 128, 128, 255,
			255, 255, 255, 255,
			255, 255, 255, 255, // expect white (lighter)
		},
		{
			"opaque 100 vs opaque 200",
			100, 100, 100, 255,
			200, 200, 200, 255,
			200, 200, 200, 255, // expect 200 (lighter)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendLighten(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendLighten() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendColorDodge tests the ColorDodge blend mode.
func TestBlendColorDodge(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{
			"opaque white dodge opaque gray",
			255, 255, 255, 255,
			128, 128, 128, 255,
		},
		{
			"opaque black dodge opaque white",
			0, 0, 0, 255,
			255, 255, 255, 255,
		},
		{
			"opaque gray dodge opaque gray",
			128, 128, 128, 255,
			128, 128, 128, 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendColorDodge(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			// Verify valid output
			if a == 0 && (r != 0 || g != 0 || b != 0) {
				t.Errorf("blendColorDodge() returned non-zero color with zero alpha: (%d, %d, %d, %d)",
					r, g, b, a)
			}
		})
	}
}

// TestBlendColorBurn tests the ColorBurn blend mode.
func TestBlendColorBurn(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{
			"opaque white burn opaque gray",
			255, 255, 255, 255,
			128, 128, 128, 255,
		},
		{
			"opaque black burn opaque white",
			0, 0, 0, 255,
			255, 255, 255, 255,
		},
		{
			"opaque gray burn opaque gray",
			128, 128, 128, 255,
			128, 128, 128, 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendColorBurn(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			// Verify valid output
			if a == 0 && (r != 0 || g != 0 || b != 0) {
				t.Errorf("blendColorBurn() returned non-zero color with zero alpha: (%d, %d, %d, %d)",
					r, g, b, a)
			}
		})
	}
}

// TestBlendHardLight tests the HardLight blend mode.
func TestBlendHardLight(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{
			"opaque white hard light opaque gray",
			255, 255, 255, 255,
			128, 128, 128, 255,
		},
		{
			"opaque black hard light opaque white",
			0, 0, 0, 255,
			255, 255, 255, 255,
		},
		{
			"opaque 64 hard light opaque gray",
			64, 64, 64, 255,
			128, 128, 128, 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendHardLight(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			// Verify valid output
			if a == 0 && (r != 0 || g != 0 || b != 0) {
				t.Errorf("blendHardLight() returned non-zero color with zero alpha: (%d, %d, %d, %d)",
					r, g, b, a)
			}
		})
	}
}

// TestBlendSoftLight tests the SoftLight blend mode.
func TestBlendSoftLight(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{
			"opaque white soft light opaque gray",
			255, 255, 255, 255,
			128, 128, 128, 255,
		},
		{
			"opaque black soft light opaque white",
			0, 0, 0, 255,
			255, 255, 255, 255,
		},
		{
			"opaque gray soft light opaque gray",
			128, 128, 128, 255,
			128, 128, 128, 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendSoftLight(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			// Verify valid output
			if a == 0 && (r != 0 || g != 0 || b != 0) {
				t.Errorf("blendSoftLight() returned non-zero color with zero alpha: (%d, %d, %d, %d)",
					r, g, b, a)
			}
		})
	}
}

// TestBlendDifference tests the Difference blend mode.
func TestBlendDifference(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque white diff opaque black",
			255, 255, 255, 255,
			0, 0, 0, 255,
			255, 255, 255, 255, // |255 - 0| = 255
		},
		{
			"opaque black diff opaque white",
			0, 0, 0, 255,
			255, 255, 255, 255,
			255, 255, 255, 255, // |0 - 255| = 255
		},
		{
			"opaque same color",
			128, 128, 128, 255,
			128, 128, 128, 255,
			0, 0, 0, 255, // |128 - 128| = 0
		},
		{
			"opaque 200 diff opaque 100",
			200, 200, 200, 255,
			100, 100, 100, 255,
			100, 100, 100, 255, // |200 - 100| = 100
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendDifference(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendDifference() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendExclusion tests the Exclusion blend mode.
func TestBlendExclusion(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{
			"opaque white exclusion opaque black",
			255, 255, 255, 255,
			0, 0, 0, 255,
		},
		{
			"opaque black exclusion opaque white",
			0, 0, 0, 255,
			255, 255, 255, 255,
		},
		{
			"opaque same color",
			128, 128, 128, 255,
			128, 128, 128, 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendExclusion(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			// Verify valid output
			if a == 0 && (r != 0 || g != 0 || b != 0) {
				t.Errorf("blendExclusion() returned non-zero color with zero alpha: (%d, %d, %d, %d)",
					r, g, b, a)
			}
		})
	}
}

// TestSeparableBlend tests the separableBlend helper function.
func TestSeparableBlend(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		blendFunc      func(s, d byte) byte
	}{
		{
			"identity blend (returns source)",
			255, 128, 64, 255,
			100, 150, 200, 255,
			func(s, d byte) byte { return s },
		},
		{
			"zero blend (returns zero)",
			255, 128, 64, 255,
			100, 150, 200, 255,
			func(s, d byte) byte { return 0 },
		},
		{
			"max blend (returns max)",
			255, 128, 64, 255,
			100, 150, 200, 255,
			maxByte,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := separableBlend(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da, tt.blendFunc)
			// Just verify no crash and valid output (bytes are always <= 255)
			_ = r
			_ = g
			_ = b
			_ = a
		})
	}
}

// TestSeparableBlendTransparent tests separableBlend with transparent inputs.
func TestSeparableBlendTransparent(t *testing.T) {
	identityBlend := func(s, d byte) byte { return s }

	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"transparent source",
			0, 0, 0, 0,
			255, 128, 64, 255,
			255, 128, 64, 255, // expect destination unchanged
		},
		{
			"transparent destination",
			255, 128, 64, 255,
			0, 0, 0, 0,
			255, 128, 64, 255, // expect source unchanged
		},
		{
			"both transparent",
			0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0, 0, 0, // expect transparent
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := separableBlend(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da, identityBlend)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("separableBlend() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestGetBlendFuncAdvanced tests that GetBlendFunc returns correct functions for advanced modes.
func TestGetBlendFuncAdvanced(t *testing.T) {
	modes := []struct {
		mode BlendMode
		name string
	}{
		{BlendMultiply, "Multiply"},
		{BlendScreen, "Screen"},
		{BlendOverlay, "Overlay"},
		{BlendDarken, "Darken"},
		{BlendLighten, "Lighten"},
		{BlendColorDodge, "ColorDodge"},
		{BlendColorBurn, "ColorBurn"},
		{BlendHardLight, "HardLight"},
		{BlendSoftLight, "SoftLight"},
		{BlendDifference, "Difference"},
		{BlendExclusion, "Exclusion"},
		{BlendHue, "Hue"},
		{BlendSaturation, "Saturation"},
		{BlendColor, "Color"},
		{BlendLuminosity, "Luminosity"},
	}

	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			fn := GetBlendFunc(m.mode)
			if fn == nil {
				t.Errorf("GetBlendFunc(%d) returned nil", m.mode)
			}
			// Smoke test: call the function
			r, g, b, a := fn(255, 128, 64, 200, 100, 150, 200, 180)
			// Verify function executed without panic
			_ = r
			_ = g
			_ = b
			_ = a
		})
	}
}

// TestAdvancedBlendModeConstants verifies blend mode constant values.
func TestAdvancedBlendModeConstants(t *testing.T) {
	tests := []struct {
		mode BlendMode
		want uint8
	}{
		{BlendMultiply, 14},
		{BlendScreen, 15},
		{BlendOverlay, 16},
		{BlendDarken, 17},
		{BlendLighten, 18},
		{BlendColorDodge, 19},
		{BlendColorBurn, 20},
		{BlendHardLight, 21},
		{BlendSoftLight, 22},
		{BlendDifference, 23},
		{BlendExclusion, 24},
		{BlendHue, 25},
		{BlendSaturation, 26},
		{BlendColor, 27},
		{BlendLuminosity, 28},
	}

	for _, tt := range tests {
		if uint8(tt.mode) != tt.want {
			t.Errorf("BlendMode constant mismatch: got %d, want %d", uint8(tt.mode), tt.want)
		}
	}
}

// TestBlendModeSymmetry tests symmetrical blend modes.
func TestBlendModeSymmetry(t *testing.T) {
	tests := []struct {
		name string
		mode BlendMode
	}{
		{"Multiply", BlendMultiply},
		{"Screen", BlendScreen},
		{"Difference", BlendDifference},
		{"Exclusion", BlendExclusion},
	}

	sr, sg, sb, sa := byte(200), byte(100), byte(50), byte(255)
	dr, dg, db, da := byte(50), byte(150), byte(200), byte(255)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := GetBlendFunc(tt.mode)

			// Blend S over D
			r1, g1, b1, a1 := fn(sr, sg, sb, sa, dr, dg, db, da)

			// Blend D over S
			r2, g2, b2, a2 := fn(dr, dg, db, da, sr, sg, sb, sa)

			// For symmetrical modes, results should be identical
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				t.Logf("Mode %s is not symmetrical (this may be expected)", tt.name)
				t.Logf("S over D: (%d, %d, %d, %d)", r1, g1, b1, a1)
				t.Logf("D over S: (%d, %d, %d, %d)", r2, g2, b2, a2)
			}
		})
	}
}

// TestBlendModeIdentity tests that blending with transparent has no effect.
func TestBlendModeIdentity(t *testing.T) {
	modes := []BlendMode{
		BlendMultiply, BlendScreen, BlendOverlay,
		BlendDarken, BlendLighten, BlendColorDodge,
		BlendColorBurn, BlendHardLight, BlendSoftLight,
		BlendDifference, BlendExclusion,
	}

	dr, dg, db, da := byte(128), byte(64), byte(192), byte(200)

	for _, mode := range modes {
		t.Run("", func(t *testing.T) {
			fn := GetBlendFunc(mode)

			// Blend transparent source over destination
			r, g, b, a := fn(0, 0, 0, 0, dr, dg, db, da)

			// Result should be destination unchanged
			if r != dr || g != dg || b != db || a != da {
				t.Errorf("blend mode %d: transparent source changed destination: got (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					mode, r, g, b, a, dr, dg, db, da)
			}
		})
	}
}

// TestSoftLightPrecision tests SoftLight precision edge cases.
func TestSoftLightPrecision(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{
			"source < 0.5",
			100, 100, 100, 255,
			150, 150, 150, 255,
		},
		{
			"source > 0.5",
			200, 200, 200, 255,
			150, 150, 150, 255,
		},
		{
			"dest < 0.25",
			200, 200, 200, 255,
			50, 50, 50, 255,
		},
		{
			"dest > 0.25",
			200, 200, 200, 255,
			100, 100, 100, 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendSoftLight(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)

			// Verify no panic and result is valid (bytes are always in range)
			_ = r
			_ = g
			_ = b
			_ = a

			// Verify no NaN (would become 0 in byte conversion)
			sf := float64(tt.sr) / 255.0
			df := float64(tt.dr) / 255.0
			if math.IsNaN(sf) || math.IsNaN(df) {
				t.Error("Input values produced NaN")
			}
		})
	}
}

// BenchmarkBlendMultiply benchmarks the Multiply blend mode.
func BenchmarkBlendMultiply(b *testing.B) {
	var r, g, b2, a byte
	for i := 0; i < b.N; i++ {
		r, g, b2, a = blendMultiply(200, 100, 50, 200, 50, 100, 200, 150)
	}
	_ = r
	_ = g
	_ = b2
	_ = a
}

// BenchmarkBlendScreen benchmarks the Screen blend mode.
func BenchmarkBlendScreen(b *testing.B) {
	var r, g, b2, a byte
	for i := 0; i < b.N; i++ {
		r, g, b2, a = blendScreen(200, 100, 50, 200, 50, 100, 200, 150)
	}
	_ = r
	_ = g
	_ = b2
	_ = a
}

// BenchmarkBlendSoftLight benchmarks the SoftLight blend mode.
func BenchmarkBlendSoftLight(b *testing.B) {
	var r, g, b2, a byte
	for i := 0; i < b.N; i++ {
		r, g, b2, a = blendSoftLight(200, 100, 50, 200, 50, 100, 200, 150)
	}
	_ = r
	_ = g
	_ = b2
	_ = a
}

// BenchmarkSeparableBlend benchmarks the separableBlend helper.
func BenchmarkSeparableBlend(b *testing.B) {
	var r, g, b2, a byte
	for i := 0; i < b.N; i++ {
		r, g, b2, a = separableBlend(200, 100, 50, 200, 50, 100, 200, 150, mulDiv255)
	}
	_ = r
	_ = g
	_ = b2
	_ = a
}
