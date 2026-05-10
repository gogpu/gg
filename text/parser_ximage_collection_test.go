package text

import (
	"os"
	"runtime"
	"testing"
)

func TestXimageParser_ParseIndex_SingleFont(t *testing.T) {
	p := &ximageParser{}

	data := loadSingleFontData(t)
	parsed, err := p.ParseIndex(data, 0)
	if err != nil {
		t.Fatalf("ParseIndex single font: %v", err)
	}
	if parsed.NumGlyphs() == 0 {
		t.Error("parsed font has 0 glyphs")
	}
}

func TestXimageParser_ParseIndex_Collection(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("TTC test requires Windows system fonts")
	}

	data, err := os.ReadFile("C:/Windows/Fonts/msyh.ttc")
	if err != nil {
		t.Skipf("msyh.ttc not found: %v", err)
	}

	p := &ximageParser{}

	// Index 0 — first font in collection.
	parsed, err := p.ParseIndex(data, 0)
	if err != nil {
		t.Fatalf("ParseIndex(0): %v", err)
	}
	name := parsed.Name()
	if name == "" {
		t.Error("font 0 has empty name")
	}
	t.Logf("Font 0: %s (%d glyphs)", name, parsed.NumGlyphs())

	// Index 1 — second font (if exists).
	parsed1, err := p.ParseIndex(data, 1)
	if err != nil {
		t.Logf("Font 1 not available: %v", err)
	} else {
		t.Logf("Font 1: %s (%d glyphs)", parsed1.Name(), parsed1.NumGlyphs())
	}
}

func TestXimageParser_ParseIndex_OutOfRange(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("TTC test requires Windows system fonts")
	}

	data, err := os.ReadFile("C:/Windows/Fonts/msyh.ttc")
	if err != nil {
		t.Skipf("msyh.ttc not found: %v", err)
	}

	p := &ximageParser{}
	_, err = p.ParseIndex(data, 999)
	if err == nil {
		t.Error("expected error for out-of-range index")
	}
	t.Logf("Expected error: %v", err)
}

func TestNewFontSourceFromFile_TTC(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("TTC test requires Windows system fonts")
	}

	// Default (index 0).
	source, err := NewFontSourceFromFile("C:/Windows/Fonts/msyh.ttc")
	if err != nil {
		t.Skipf("msyh.ttc: %v", err)
	}

	face := source.Face(14)
	if face == nil {
		t.Fatal("Face is nil")
	}
	t.Logf("Font: %s, face size=14", source.Name())

	// CJK glyph should exist.
	idx := source.Parsed().GlyphIndex('中')
	if idx == 0 {
		t.Error("GlyphIndex('中') = 0, CJK glyph missing")
	}
	t.Logf("GlyphIndex('中') = %d", idx)
}

func TestNewFontSourceFromFile_TTC_WithIndex(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("TTC test requires Windows system fonts")
	}

	// Explicit index 1.
	source, err := NewFontSourceFromFile("C:/Windows/Fonts/msyh.ttc", WithCollectionIndex(1))
	if err != nil {
		t.Skipf("msyh.ttc index 1: %v", err)
	}
	t.Logf("Font index 1: %s (%d glyphs)", source.Name(), source.Parsed().NumGlyphs())
}

func loadSingleFontData(t *testing.T) []byte {
	t.Helper()
	paths := []string{
		"../testdata/Roboto-Regular.ttf",
		"testdata/Roboto-Regular.ttf",
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			return data
		}
	}
	t.Skip("no test font found")
	return nil
}
