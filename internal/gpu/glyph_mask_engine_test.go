//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

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
