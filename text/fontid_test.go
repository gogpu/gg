package text

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestComputeFontID_Nil(t *testing.T) {
	if id := ComputeFontID(nil); id != 0 {
		t.Errorf("ComputeFontID(nil) = %d, want 0", id)
	}
}

func TestComputeFontID_Deterministic(t *testing.T) {
	src, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("NewFontSource: %v", err)
	}
	id1 := ComputeFontID(src)
	id2 := ComputeFontID(src)
	if id1 != id2 {
		t.Errorf("same source produced different IDs: %d vs %d", id1, id2)
	}
	if id1 == 0 {
		t.Error("non-nil source produced zero ID")
	}
}

func TestComputeFontID_DistinguishesRegularAndBold(t *testing.T) {
	regular, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("load Go Regular: %v", err)
	}
	bold, err := NewFontSource(gobold.TTF)
	if err != nil {
		t.Fatalf("load Go Bold: %v", err)
	}

	if regular.Name() != bold.Name() {
		t.Skipf("fonts have different family names (%q vs %q)", regular.Name(), bold.Name())
	}
	if regular.Parsed().NumGlyphs() != bold.Parsed().NumGlyphs() {
		t.Skipf("fonts have different glyph counts (%d vs %d)",
			regular.Parsed().NumGlyphs(), bold.Parsed().NumGlyphs())
	}

	regularID := ComputeFontID(regular)
	boldID := ComputeFontID(bold)
	if regularID == boldID {
		t.Errorf("Go Regular and Go Bold must have different FontIDs\n"+
			"  Regular: fullName=%q id=%d\n"+
			"  Bold:    fullName=%q id=%d",
			regular.Parsed().FullName(), regularID,
			bold.Parsed().FullName(), boldID)
	}
}
