//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestComputeFontID_DifferentFontsInSameFamily(t *testing.T) {
	srcRegular, err := text.NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to load GoRegular: %v", err)
	}
	srcBold, err := text.NewFontSource(gobold.TTF)
	if err != nil {
		t.Fatalf("failed to load GoBold: %v", err)
	}

	// Precondition: both fonts share the same family name and glyph count.
	if srcRegular.Name() != srcBold.Name() {
		t.Skipf("fonts have different family names (%q vs %q), test not applicable",
			srcRegular.Name(), srcBold.Name())
	}
	if srcRegular.Parsed().NumGlyphs() != srcBold.Parsed().NumGlyphs() {
		t.Skipf("fonts have different glyph counts (%d vs %d), test not applicable",
			srcRegular.Parsed().NumGlyphs(), srcBold.Parsed().NumGlyphs())
	}

	idRegular := computeFontID(srcRegular)
	idBold := computeFontID(srcBold)

	if idRegular == idBold {
		t.Errorf("GoRegular and GoBold must have different FontIDs to avoid atlas cache collision\n"+
			"  GoRegular: name=%q fullName=%q numGlyphs=%d fontID=%d\n"+
			"  GoBold:    name=%q fullName=%q numGlyphs=%d fontID=%d",
			srcRegular.Name(), srcRegular.Parsed().FullName(), srcRegular.Parsed().NumGlyphs(), idRegular,
			srcBold.Name(), srcBold.Parsed().FullName(), srcBold.Parsed().NumGlyphs(), idBold)
	}
}

func TestComputeFontID_NilSource(t *testing.T) {
	id := computeFontID(nil)
	if id != 0 {
		t.Errorf("nil source should return 0, got %d", id)
	}
}

func TestComputeFontID_SameFontStableID(t *testing.T) {
	src, err := text.NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to load GoRegular: %v", err)
	}

	id1 := computeFontID(src)
	id2 := computeFontID(src)
	if id1 != id2 {
		t.Errorf("same font source should produce stable ID: %d != %d", id1, id2)
	}
}

func TestGPUTextEngineMultiFaceKeepsFallbackGlyphsBatched(t *testing.T) {
	source, err := text.NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("NewFontSource: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	latin := text.NewFilteredFace(source.Face(20), text.RangeBasicLatin)
	cyrillic := text.NewFilteredFace(source.Face(20), text.RangeCyrillic)
	face, err := text.NewMultiFace(latin, cyrillic)
	if err != nil {
		t.Fatalf("NewMultiFace: %v", err)
	}

	engine := NewGPUTextEngine()
	batch, err := engine.LayoutText(face, "AБ", 0, 24, gg.RGBA{A: 1}, gg.Identity(), 1)
	if err != nil {
		t.Fatalf("LayoutText(MultiFace): %v", err)
	}
	if len(batch.Quads) < 2 {
		t.Fatalf("LayoutText(MultiFace) emitted %d quads, want at least 2", len(batch.Quads))
	}

	filtered := text.NewFilteredFace(face, text.RangeBasicLatin)
	filteredBatch, err := engine.LayoutText(filtered, "AБ", 0, 24, gg.RGBA{A: 1}, gg.Identity(), 1)
	if err != nil {
		t.Fatalf("LayoutText(FilteredFace(MultiFace)): %v", err)
	}
	if len(filteredBatch.Quads) == 0 {
		t.Fatal("LayoutText(FilteredFace(MultiFace)) dropped the allowed run")
	}
}
