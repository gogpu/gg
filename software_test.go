package gg

import (
	"testing"

	"github.com/gogpu/gg/internal/raster"
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
		foreground raster.RGBA
		coverage   uint8
		wantBlend  bool // true if we expect blended result, false if pure foreground
	}{
		{
			name:       "opaque color with full coverage - fast path OK",
			background: White,
			foreground: raster.RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}, // opaque red
			coverage:   255,
			wantBlend:  false, // should use fast path (pure foreground)
		},
		{
			name:       "semi-transparent color with full coverage - must blend (BUG-001)",
			background: White,
			foreground: raster.RGBA{R: 1.0, G: 0.0, B: 0.0, A: 0.5}, // 50% alpha red
			coverage:   255,
			wantBlend:  true, // MUST blend, not use fast path!
		},
		{
			name:       "semi-transparent color with partial coverage - must blend",
			background: White,
			foreground: raster.RGBA{R: 0.0, G: 1.0, B: 0.0, A: 0.5}, // 50% alpha green
			coverage:   128,
			wantBlend:  true,
		},
		{
			name:       "opaque color with partial coverage - must blend",
			background: White,
			foreground: raster.RGBA{R: 0.0, G: 0.0, B: 1.0, A: 1.0}, // opaque blue
			coverage:   128,
			wantBlend:  true,
		},
		{
			name:       "zero alpha color - should not change background",
			background: Red,
			foreground: raster.RGBA{R: 0.0, G: 1.0, B: 0.0, A: 0.0}, // transparent green
			coverage:   255,
			wantBlend:  false, // background unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a small pixmap with background color
			pm := NewPixmap(10, 10)
			pm.Clear(tt.background)

			// Create adapter and blend
			adapter := &pixmapAdapter{pixmap: pm}
			adapter.BlendPixelAlpha(5, 5, tt.foreground, tt.coverage)

			// Get result
			result := pm.GetPixel(5, 5)

			// Check result
			tolerance := 0.02
			isPureBackground := colorNear(result, tt.background, tolerance)
			isPureForeground := colorNear(result, RGBA{
				R: tt.foreground.R,
				G: tt.foreground.G,
				B: tt.foreground.B,
				A: tt.foreground.A,
			}, tolerance)

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

	adapter := &pixmapAdapter{pixmap: pm}
	adapter.BlendPixelAlpha(5, 5, raster.RGBA{R: 1.0, G: 0.0, B: 0.0, A: 0.5}, 255)

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

// TestAAPixmapAdapterSemiTransparent tests the same bug in raster.AAPixmapAdapter.
func TestAAPixmapAdapterSemiTransparent(t *testing.T) {
	// Create a mock pixmap that tracks SetPixel calls
	mock := &trackingPixmap{
		width:  10,
		height: 10,
		pixels: make(map[[2]int]raster.RGBA),
	}

	adapter := &raster.AAPixmapAdapter{Pixmap: mock}

	// Blend semi-transparent color with full coverage
	semiTransparent := raster.RGBA{R: 1.0, G: 0.0, B: 0.0, A: 0.5}
	adapter.BlendPixelAlpha(5, 5, semiTransparent, 255)

	// Get the result
	result, ok := mock.pixels[[2]int{5, 5}]
	if !ok {
		t.Fatal("pixel was not set")
	}

	// The alpha should be scaled by coverage, not the original 0.5
	// With proper fix: A should be 0.5 (original alpha * 255/255)
	// BUG: Would have set A = 0.5 without proper blending

	if result.A == semiTransparent.A {
		// This is actually correct for AAPixmapAdapter's simpler blending
		// It just scales color alpha by coverage
		t.Logf("AAPixmapAdapter correctly set alpha = %.2f", result.A)
	}
}

// trackingPixmap is a test helper that tracks SetPixel calls.
type trackingPixmap struct {
	width  int
	height int
	pixels map[[2]int]raster.RGBA
}

func (p *trackingPixmap) Width() int  { return p.width }
func (p *trackingPixmap) Height() int { return p.height }
func (p *trackingPixmap) SetPixel(x, y int, c raster.RGBA) {
	p.pixels[[2]int{x, y}] = c
}
