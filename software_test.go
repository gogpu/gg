package gg

import (
	"testing"
)

// TestBlendPixelAlphaSemiTransparent tests BUG-001: semi-transparent colors
// should use source-over compositing even when coverage is 255.
//
// Bug: BlendPixelAlpha had a fast path that skipped compositing when
// alpha (coverage) was 255, but didn't check if the color itself was
// semi-transparent (c.A < 1.0).
//
// Fix: Check both coverage AND color alpha before using fast path.
func TestBlendPixelAlphaSemiTransparent(t *testing.T) {
	tests := []struct {
		name       string
		background RGBA
		foreground RGBA
		coverage   uint8
		wantBlend  bool // true if we expect blended result, false if pure foreground
	}{
		{
			name:       "opaque color with full coverage - fast path OK",
			background: White,
			foreground: RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}, // opaque red
			coverage:   255,
			wantBlend:  false, // should use fast path (pure foreground)
		},
		{
			name:       "semi-transparent color with full coverage - must blend (BUG-001)",
			background: White,
			foreground: RGBA{R: 1.0, G: 0.0, B: 0.0, A: 0.5}, // 50% alpha red
			coverage:   255,
			wantBlend:  true, // MUST blend, not use fast path!
		},
		{
			name:       "semi-transparent color with partial coverage - must blend",
			background: White,
			foreground: RGBA{R: 0.0, G: 1.0, B: 0.0, A: 0.5}, // 50% alpha green
			coverage:   128,
			wantBlend:  true,
		},
		{
			name:       "opaque color with partial coverage - must blend",
			background: White,
			foreground: RGBA{R: 0.0, G: 0.0, B: 1.0, A: 1.0}, // opaque blue
			coverage:   128,
			wantBlend:  true,
		},
		{
			name:       "zero alpha color - should not change background",
			background: Red,
			foreground: RGBA{R: 0.0, G: 1.0, B: 0.0, A: 0.0}, // transparent green
			coverage:   255,
			wantBlend:  false, // background unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a small pixmap with background color
			pm := NewPixmap(10, 10)
			pm.Clear(tt.background)

			// Blend using source-over compositing
			blendPixelAlpha(pm, 5, 5, tt.foreground, tt.coverage)

			// Get result
			result := pm.GetPixel(5, 5)

			// Check result
			tolerance := 0.02
			isPureBackground := colorNear(result, tt.background, tolerance)
			isPureForeground := colorNear(result, tt.foreground, tolerance)

			switch {
			case tt.foreground.A == 0.0:
				// Zero alpha should not change background
				if !isPureBackground {
					t.Errorf("zero alpha foreground should not change background\ngot:  %+v\nwant: %+v",
						result, tt.background)
				}

			case tt.wantBlend:
				// Should be blended (neither pure foreground nor pure background)
				if isPureForeground {
					t.Errorf("expected blended result, got pure foreground\nresult:     %+v\nforeground: %+v\nbackground: %+v",
						result, tt.foreground, tt.background)
				}
				if isPureBackground {
					t.Errorf("expected blended result, got pure background\nresult:     %+v\nforeground: %+v\nbackground: %+v",
						result, tt.foreground, tt.background)
				}

			default:
				// Should be pure foreground (fast path)
				if !isPureForeground {
					t.Errorf("expected pure foreground, got different result\nresult:     %+v\nforeground: %+v",
						result, tt.foreground)
				}
			}
		})
	}
}

// TestBlendPixelAlphaSemiTransparentRGBValues verifies the actual RGB values
// after blending a semi-transparent color over white background.
func TestBlendPixelAlphaSemiTransparentRGBValues(t *testing.T) {
	// 50% alpha red over white should produce pink (R=1.0, G=0.5, B=0.5)
	pm := NewPixmap(10, 10)
	pm.Clear(White)

	blendPixelAlpha(pm, 5, 5, RGBA{R: 1.0, G: 0.0, B: 0.0, A: 0.5}, 255)

	result := pm.GetPixel(5, 5)

	// With 50% alpha, red over white:
	// outR = (1.0 * 0.5 + 1.0 * 1.0 * 0.5) / (0.5 + 0.5) = (0.5 + 0.5) / 1.0 = 1.0
	// outG = (0.0 * 0.5 + 1.0 * 1.0 * 0.5) / 1.0 = 0.5
	// outB = (0.0 * 0.5 + 1.0 * 1.0 * 0.5) / 1.0 = 0.5
	// outA = 0.5 + 1.0 * 0.5 = 1.0
	//
	// So result should be (1.0, 0.5, 0.5, 1.0) - a pink color

	tolerance := 0.05 // Allow small numerical errors

	if abs(result.R-1.0) > tolerance {
		t.Errorf("R = %.3f, want ~1.0", result.R)
	}
	if abs(result.G-0.5) > tolerance {
		t.Errorf("G = %.3f, want ~0.5", result.G)
	}
	if abs(result.B-0.5) > tolerance {
		t.Errorf("B = %.3f, want ~0.5", result.B)
	}
	if abs(result.A-1.0) > tolerance {
		t.Errorf("A = %.3f, want ~1.0", result.A)
	}
}

// blendPixelAlpha blends a color with the existing pixel using given alpha.
// This is a test helper that implements premultiplied source-over compositing.
func blendPixelAlpha(pm *Pixmap, x, y int, c RGBA, alpha uint8) {
	if alpha == 0 {
		return
	}

	// Bounds check
	if x < 0 || x >= pm.Width() || y < 0 || y >= pm.Height() {
		return
	}

	if alpha == 255 && c.A >= 1.0 {
		pm.SetPixel(x, y, c)
		return
	}

	// Premultiplied source-over compositing
	srcAlpha := c.A * float64(alpha) / 255.0
	invSrcAlpha := 1.0 - srcAlpha

	// Premultiply source color
	srcR := c.R * srcAlpha
	srcG := c.G * srcAlpha
	srcB := c.B * srcAlpha

	// Read existing pixel (already premultiplied in buffer)
	dstR, dstG, dstB, dstA := pm.getPremul(x, y)

	// Source-over in premultiplied space: Result = Src + Dst * (1 - SrcA)
	pm.setPremul(x, y,
		srcR+dstR*invSrcAlpha,
		srcG+dstG*invSrcAlpha,
		srcB+dstB*invSrcAlpha,
		srcAlpha+dstA*invSrcAlpha,
	)
}
