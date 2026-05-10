//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

func TestGPUTextEngine_CJKAtlasCreated(t *testing.T) {
	engine := NewGPUTextEngine()
	if engine.atlasManager == nil {
		t.Fatal("Latin atlas manager is nil")
	}
	if engine.cjkAtlasManager == nil {
		t.Fatal("CJK atlas manager is nil")
	}
	if engine.msdfSize != 64 {
		t.Errorf("Latin MSDF size = %d, want 64", engine.msdfSize)
	}
	if engine.msdfSizeCJK != 128 {
		t.Errorf("CJK MSDF size = %d, want 128", engine.msdfSizeCJK)
	}
}

func TestGPUTextEngine_CJKAtlasConfig(t *testing.T) {
	engine := NewGPUTextEngine()

	latinCfg := engine.atlasManager.Config()
	cjkCfg := engine.cjkAtlasManager.Config()

	if latinCfg.GlyphSize != 64 {
		t.Errorf("Latin glyph size = %d, want 64", latinCfg.GlyphSize)
	}
	if cjkCfg.GlyphSize != 128 {
		t.Errorf("CJK glyph size = %d, want 128", cjkCfg.GlyphSize)
	}
	if latinCfg.Size != 1024 {
		t.Errorf("Latin atlas size = %d, want 1024", latinCfg.Size)
	}
	if cjkCfg.Size != 2048 {
		t.Errorf("CJK atlas size = %d, want 2048", cjkCfg.Size)
	}
}

func TestGPUTextEngine_CJKAtlasOffset(t *testing.T) {
	if cjkAtlasOffset < 100 {
		t.Errorf("cjkAtlasOffset = %d, should be ≥100 to avoid index collision", cjkAtlasOffset)
	}
}

func TestGPUTextEngine_DirtyAtlasesEmpty(t *testing.T) {
	engine := NewGPUTextEngine()
	dirty := engine.DirtyAtlases()
	if len(dirty) != 0 {
		t.Errorf("new engine should have 0 dirty atlases, got %d", len(dirty))
	}
}

func TestGPUTextEngine_AtlasRGBADataNil(t *testing.T) {
	engine := NewGPUTextEngine()

	data, w, h := engine.AtlasRGBAData(0)
	if data != nil || w != 0 || h != 0 {
		t.Error("empty Latin atlas should return nil")
	}

	data, w, h = engine.AtlasRGBAData(cjkAtlasOffset)
	if data != nil || w != 0 || h != 0 {
		t.Error("empty CJK atlas should return nil")
	}
}

func TestGPUTextEngine_MarkCleanBothAtlases(t *testing.T) {
	engine := NewGPUTextEngine()
	// Should not panic for either atlas type.
	engine.MarkClean(0)
	engine.MarkClean(cjkAtlasOffset)
}

// --- IsCJKRune coverage for text rendering decisions ---

func TestIsCJKRune_TextRendering(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"Han", '中', true},
		{"Han_complex", '龍', true},
		{"Hiragana", 'あ', true},
		{"Katakana", 'ア', true},
		{"Hangul", '한', true},
		{"Fullwidth", 'Ａ', true},
		{"Latin", 'A', false},
		{"Digit", '1', false},
		{"Emoji", '😀', false},
		{"Arabic", 'ع', false},
		{"Cyrillic", 'Д', false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := text.IsCJKRune(tt.r); got != tt.want {
				t.Errorf("IsCJKRune(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

func TestSelectGlyphMaskHinting_CJKEnterprise(t *testing.T) {
	// Enterprise validation: CJK hinting matches platform behaviors.
	tests := []struct {
		name        string
		isCJK       bool
		deviceScale float64
		want        text.Hinting
		reason      string
	}{
		{
			name: "cjk_1x_vertical", isCJK: true, deviceScale: 1.0,
			want:   text.HintingVertical,
			reason: "FreeType afcjk: Y-direction only for CJK",
		},
		{
			name: "cjk_1.5x_vertical", isCJK: true, deviceScale: 1.5,
			want:   text.HintingVertical,
			reason: "150% scale: still benefits from vertical hinting",
		},
		{
			name: "cjk_2x_none", isCJK: true, deviceScale: 2.0,
			want:   text.HintingNone,
			reason: "macOS Core Text: ignores hinting on Retina",
		},
		{
			name: "cjk_3x_none", isCJK: true, deviceScale: 3.0,
			want:   text.HintingNone,
			reason: "HiDPI: pixel density makes hinting unnecessary",
		},
		{
			name: "latin_1x_full", isCJK: false, deviceScale: 1.0,
			want:   text.HintingFull,
			reason: "Latin: full grid-fitting for crisp stems",
		},
		{
			name: "latin_2x_full", isCJK: false, deviceScale: 2.0,
			want:   text.HintingFull,
			reason: "Latin on Retina: still benefits from full hinting",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectGlyphMaskHinting(14, identityMatrix(), tt.isCJK, tt.deviceScale)
			if got != tt.want {
				t.Errorf("%s: got %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

func identityMatrix() gg.Matrix {
	return gg.Identity()
}
