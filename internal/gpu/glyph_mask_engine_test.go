//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

func TestSelectGlyphMaskLCD(t *testing.T) {
	tests := []struct {
		name     string
		fontSize float64
		matrix   gg.Matrix
		want     bool
	}{
		{
			name:     "small_identity",
			fontSize: 12,
			matrix:   gg.Identity(),
			want:     true,
		},
		{
			name:     "small_translation",
			fontSize: 16,
			matrix:   gg.Matrix{A: 1, B: 0, C: 50, D: 0, E: 1, F: 30},
			want:     true,
		},
		{
			name:     "threshold_48px",
			fontSize: 48,
			matrix:   gg.Identity(),
			want:     true,
		},
		{
			name:     "above_threshold",
			fontSize: 49,
			matrix:   gg.Identity(),
			want:     false,
		},
		{
			name:     "large_72px",
			fontSize: 72,
			matrix:   gg.Identity(),
			want:     false,
		},
		{
			name:     "rotated_small",
			fontSize: 12,
			matrix:   gg.Matrix{A: 0.707, B: -0.707, C: 0, D: 0.707, E: 0.707, F: 0},
			want:     false,
		},
		{
			name:     "skewed",
			fontSize: 14,
			matrix:   gg.Matrix{A: 1, B: 0.3, C: 0, D: 0, E: 1, F: 0},
			want:     false,
		},
		{
			name:     "uniform_scale_small",
			fontSize: 12,
			matrix:   gg.Matrix{A: 2, B: 0, C: 0, D: 0, E: 2, F: 0},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectGlyphMaskLCD(tt.fontSize, tt.matrix)
			if got != tt.want {
				t.Errorf("selectGlyphMaskLCD(%v, %v) = %v, want %v",
					tt.fontSize, tt.matrix, got, tt.want)
			}
		})
	}
}

func TestGlyphMaskEngine_SetLCDLayout(t *testing.T) {
	engine := NewGlyphMaskEngine()

	// Default should be LCDLayoutNone.
	if engine.LCDLayout() != text.LCDLayoutNone {
		t.Errorf("default LCD layout = %v, want LCDLayoutNone", engine.LCDLayout())
	}

	// Set to RGB.
	engine.SetLCDLayout(text.LCDLayoutRGB)
	if engine.LCDLayout() != text.LCDLayoutRGB {
		t.Errorf("after SetLCDLayout(RGB) = %v, want LCDLayoutRGB", engine.LCDLayout())
	}

	// Set to BGR.
	engine.SetLCDLayout(text.LCDLayoutBGR)
	if engine.LCDLayout() != text.LCDLayoutBGR {
		t.Errorf("after SetLCDLayout(BGR) = %v, want LCDLayoutBGR", engine.LCDLayout())
	}

	// Set back to None.
	engine.SetLCDLayout(text.LCDLayoutNone)
	if engine.LCDLayout() != text.LCDLayoutNone {
		t.Errorf("after SetLCDLayout(None) = %v, want LCDLayoutNone", engine.LCDLayout())
	}
}

func TestGlyphMaskEngine_SetLCDFilter(t *testing.T) {
	engine := NewGlyphMaskEngine()

	// Custom filter should not panic.
	custom := text.LCDFilter{Weights: [5]float32{0.1, 0.2, 0.4, 0.2, 0.1}}
	engine.SetLCDFilter(custom)
}

func TestSelectGlyphMaskHinting(t *testing.T) {
	tests := []struct {
		name     string
		fontSize float64
		matrix   gg.Matrix
		want     text.Hinting
	}{
		{
			name:     "small_identity",
			fontSize: 12,
			matrix:   gg.Identity(),
			want:     text.HintingFull,
		},
		{
			name:     "small_translation",
			fontSize: 16,
			matrix:   gg.Matrix{A: 1, B: 0, C: 50, D: 0, E: 1, F: 30},
			want:     text.HintingFull,
		},
		{
			name:     "threshold_48px",
			fontSize: 48,
			matrix:   gg.Identity(),
			want:     text.HintingFull,
		},
		{
			name:     "above_threshold",
			fontSize: 49,
			matrix:   gg.Identity(),
			want:     text.HintingNone,
		},
		{
			name:     "large_72px",
			fontSize: 72,
			matrix:   gg.Identity(),
			want:     text.HintingNone,
		},
		{
			name:     "rotated_small",
			fontSize: 12,
			matrix:   gg.Matrix{A: 0.707, B: -0.707, C: 0, D: 0.707, E: 0.707, F: 0},
			want:     text.HintingNone,
		},
		{
			name:     "skewed",
			fontSize: 14,
			matrix:   gg.Matrix{A: 1, B: 0.3, C: 0, D: 0, E: 1, F: 0},
			want:     text.HintingNone,
		},
		{
			name:     "uniform_scale_small",
			fontSize: 12,
			matrix:   gg.Matrix{A: 2, B: 0, C: 0, D: 0, E: 2, F: 0},
			want:     text.HintingFull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectGlyphMaskHinting(tt.fontSize, tt.matrix)
			if got != tt.want {
				t.Errorf("selectGlyphMaskHinting(%v, %v) = %v, want %v",
					tt.fontSize, tt.matrix, got, tt.want)
			}
		})
	}
}
