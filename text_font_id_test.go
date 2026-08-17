package gg

import (
	"testing"

	"github.com/gogpu/gg/text"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestComputeTextFontID_DifferentFacesInSameFamily(t *testing.T) {
	regular, err := text.NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("load Go Regular: %v", err)
	}
	bold, err := text.NewFontSource(gobold.TTF)
	if err != nil {
		t.Fatalf("load Go Bold: %v", err)
	}

	if regular.Name() != bold.Name() {
		t.Skipf("fonts have different family names (%q vs %q), test not applicable", regular.Name(), bold.Name())
	}
	if regular.Parsed().NumGlyphs() != bold.Parsed().NumGlyphs() {
		t.Skipf("fonts have different glyph counts (%d vs %d), test not applicable",
			regular.Parsed().NumGlyphs(), bold.Parsed().NumGlyphs())
	}

	regularID := computeTextFontID(regular)
	boldID := computeTextFontID(bold)
	if regularID == boldID {
		t.Errorf("Go Regular and Go Bold must have different FontIDs to avoid atlas cache collision\n"+
			"  Go Regular: name=%q fullName=%q numGlyphs=%d fontID=%d\n"+
			"  Go Bold:    name=%q fullName=%q numGlyphs=%d fontID=%d",
			regular.Name(), regular.Parsed().FullName(), regular.Parsed().NumGlyphs(), regularID,
			bold.Name(), bold.Parsed().FullName(), bold.Parsed().NumGlyphs(), boldID)
	}
}
