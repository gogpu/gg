//go:build !nogpu

package gpu

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

func TestGlyphMaskEngineLayoutShapedGlyphsAliasedUsesBinaryCoverage(t *testing.T) {
	source, err := text.NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("NewFontSource: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	face := source.Face(24)
	var glyphs []text.ShapedGlyph
	for glyph := range face.Glyphs("O") {
		glyphs = append(glyphs, text.ShapedGlyph{GID: glyph.GID, X: glyph.X, Y: glyph.Y})
	}

	engine := NewGlyphMaskEngine()
	engine.SetLCDLayout(text.LCDLayoutRGB)
	batch, err := engine.LayoutShapedGlyphsAliased(
		face, glyphs, 0, 30, gg.RGBA{A: 1}, gg.Identity(), 1, false,
	)
	if err != nil {
		t.Fatalf("LayoutShapedGlyphsAliased: %v", err)
	}
	if len(batch.Quads) == 0 {
		t.Fatal("LayoutShapedGlyphsAliased produced no quads")
	}
	if batch.IsLCD {
		t.Error("aliased shaped glyphs used LCD coverage")
	}

	atlas, _, _ := engine.Atlas().PageR8Data(0)
	hasInk := false
	for _, alpha := range atlas {
		if alpha != 0 && alpha != 255 {
			t.Fatalf("aliased shaped atlas contains intermediate alpha %d", alpha)
		}
		if alpha == 255 {
			hasInk = true
		}
	}
	if !hasInk {
		t.Fatal("aliased shaped atlas contains no glyph pixels")
	}
}

func TestSelectGlyphMaskLCD(t *testing.T) {
	// ADR-060 / BUG-TEXT-001: selectGlyphMaskLCD always returns false for
	// the GPU pipeline because standard SrcOver blend cannot do per-channel
	// alpha compositing. LCD disabled until dual-source blending (FEAT-TEXT-002).
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
			want:     false,
		},
		{
			name:     "small_translation",
			fontSize: 16,
			matrix:   gg.Matrix{A: 1, B: 0, C: 50, D: 0, E: 1, F: 30},
			want:     false,
		},
		{
			name:     "threshold_48px",
			fontSize: 48,
			matrix:   gg.Identity(),
			want:     false,
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
			want:     false,
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

	key := text.MakeGlyphMaskKey(1, 1, 12, 0, 0)
	if _, err := engine.Atlas().Put(key, []byte{255}, 1, 1, 0, 0); err != nil {
		t.Fatalf("populate atlas: %v", err)
	}

	// Reapplying the same filter preserves compatible cached masks.
	engine.SetLCDFilter(text.DefaultLCDFilter())
	if entries := glyphMaskAtlasEntryCount(engine); entries != 1 {
		t.Fatalf("same filter cleared atlas: entries = %d, want 1", entries)
	}

	// A changed filter invalidates masks that may have used its weights.
	custom := text.LCDFilter{Weights: [5]float32{0.1, 0.2, 0.4, 0.2, 0.1}}
	engine.SetLCDFilter(custom)
	if entries := glyphMaskAtlasEntryCount(engine); entries != 0 {
		t.Fatalf("changed filter retained atlas: entries = %d, want 0", entries)
	}
}

func glyphMaskAtlasEntryCount(engine *GlyphMaskEngine) int {
	hits, misses, entries, pages := engine.Atlas().Stats()
	_ = hits
	_ = misses
	_ = pages
	return entries
}

func TestSelectGlyphMaskHinting(t *testing.T) {
	tests := []struct {
		name        string
		fontSize    float64
		matrix      gg.Matrix
		isCJK       bool
		deviceScale float64
		want        text.Hinting
	}{
		{
			name: "latin_small_identity", fontSize: 12, matrix: gg.Identity(),
			want: text.HintingFull,
		},
		{
			name: "latin_small_translation", fontSize: 16,
			matrix: gg.Matrix{A: 1, B: 0, C: 50, D: 0, E: 1, F: 30},
			want:   text.HintingFull,
		},
		{
			name: "latin_threshold_48px", fontSize: 48, matrix: gg.Identity(),
			want: text.HintingFull,
		},
		{
			name: "latin_above_threshold", fontSize: 49, matrix: gg.Identity(),
			want: text.HintingNone,
		},
		{
			name: "latin_large_72px", fontSize: 72, matrix: gg.Identity(),
			want: text.HintingNone,
		},
		{
			name: "rotated_small", fontSize: 12,
			matrix: gg.Matrix{A: 0.707, B: -0.707, C: 0, D: 0.707, E: 0.707, F: 0},
			want:   text.HintingNone,
		},
		{
			name: "skewed", fontSize: 14,
			matrix: gg.Matrix{A: 1, B: 0.3, C: 0, D: 0, E: 1, F: 0},
			want:   text.HintingNone,
		},
		{
			name: "latin_uniform_scale", fontSize: 12,
			matrix: gg.Matrix{A: 2, B: 0, C: 0, D: 0, E: 2, F: 0},
			want:   text.HintingFull,
		},
		// ADR-027: CJK script-aware hinting
		{
			name: "cjk_small_1x_vertical_only", fontSize: 14, matrix: gg.Identity(),
			isCJK: true, deviceScale: 1.0,
			want: text.HintingVertical,
		},
		{
			name: "cjk_small_2x_none", fontSize: 14, matrix: gg.Identity(),
			isCJK: true, deviceScale: 2.0,
			want: text.HintingNone,
		},
		{
			name: "cjk_large_none", fontSize: 72, matrix: gg.Identity(),
			isCJK: true, deviceScale: 1.0,
			want: text.HintingNone,
		},
		{
			name: "cjk_rotated_none", fontSize: 14,
			matrix: gg.Matrix{A: 0.707, B: -0.707, C: 0, D: 0.707, E: 0.707, F: 0},
			isCJK:  true, deviceScale: 1.0,
			want: text.HintingNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := tt.deviceScale
			if ds == 0 {
				ds = 1.0
			}
			got := selectGlyphMaskHinting(tt.fontSize, tt.matrix, tt.isCJK, ds)
			if got != tt.want {
				t.Errorf("selectGlyphMaskHinting(%v, %v, cjk=%v, scale=%v) = %v, want %v",
					tt.fontSize, tt.matrix, tt.isCJK, ds, got, tt.want)
			}
		})
	}
}
