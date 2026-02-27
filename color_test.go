package gg

import (
	"image/color"
	"testing"
)

// Verify at compile time that RGBA implements color.Color.
var _ color.Color = RGBA{}

func TestRGBA_ColorInterface(t *testing.T) {
	tests := []struct {
		name string
		c    RGBA
		wantR, wantG, wantB, wantA uint32
	}{
		{
			name:  "opaque black",
			c:     Black,
			wantR: 0, wantG: 0, wantB: 0, wantA: 65535,
		},
		{
			name:  "opaque white",
			c:     White,
			wantR: 65535, wantG: 65535, wantB: 65535, wantA: 65535,
		},
		{
			name:  "opaque red",
			c:     Red,
			wantR: 65535, wantG: 0, wantB: 0, wantA: 65535,
		},
		{
			name:  "transparent",
			c:     RGBA{0, 0, 0, 0},
			wantR: 0, wantG: 0, wantB: 0, wantA: 0,
		},
		{
			name:  "50% alpha red",
			c:     RGBA{1, 0, 0, 0.5},
			wantR: 32767, wantG: 0, wantB: 0, wantA: 32767,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := tt.c.RGBA()
			// Allow ±1 tolerance for floating point
			if diff(r, tt.wantR) > 1 || diff(g, tt.wantG) > 1 || diff(b, tt.wantB) > 1 || diff(a, tt.wantA) > 1 {
				t.Errorf("RGBA() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wantR, tt.wantG, tt.wantB, tt.wantA)
			}
		})
	}
}

func TestRGBA_SetColor(t *testing.T) {
	dc := NewContext(10, 10)
	// This must compile — proves gg.RGBA implements color.Color
	dc.SetColor(Black)
	dc.SetColor(Red)
	dc.SetColor(Hex("#3498db"))
}

func TestRGBA_Roundtrip(t *testing.T) {
	// gg.RGBA → color.Color → FromColor → gg.RGBA
	original := RGBA{0.8, 0.3, 0.5, 0.9}
	r, g, b, a := original.RGBA()
	roundtripped := FromColor(color.NRGBA64{
		R: uint16(float64(r) / original.A),
		G: uint16(float64(g) / original.A),
		B: uint16(float64(b) / original.A),
		A: uint16(a),
	})
	const tolerance = 0.001
	if absDiff(original.R, roundtripped.R) > tolerance ||
		absDiff(original.G, roundtripped.G) > tolerance ||
		absDiff(original.B, roundtripped.B) > tolerance ||
		absDiff(original.A, roundtripped.A) > tolerance {
		t.Errorf("roundtrip: %v → %v", original, roundtripped)
	}
}

func diff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
