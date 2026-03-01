package text

import (
	"testing"
)

func TestDefaultTabWidth(t *testing.T) {
	if DefaultTabWidth != 8 {
		t.Errorf("DefaultTabWidth = %d, want 8", DefaultTabWidth)
	}
}

func TestTabWidth(t *testing.T) {
	// Save and restore.
	orig := globalTabWidth
	defer func() { globalTabWidth = orig }()

	if TabWidth() != DefaultTabWidth {
		t.Errorf("TabWidth() = %d, want %d", TabWidth(), DefaultTabWidth)
	}

	SetTabWidth(4)
	if TabWidth() != 4 {
		t.Errorf("after SetTabWidth(4): TabWidth() = %d, want 4", TabWidth())
	}

	// Zero resets to default.
	SetTabWidth(0)
	if TabWidth() != DefaultTabWidth {
		t.Errorf("after SetTabWidth(0): TabWidth() = %d, want %d", TabWidth(), DefaultTabWidth)
	}

	// Negative resets to default.
	SetTabWidth(4)
	SetTabWidth(-1)
	if TabWidth() != DefaultTabWidth {
		t.Errorf("after SetTabWidth(-1): TabWidth() = %d, want %d", TabWidth(), DefaultTabWidth)
	}
}

func TestExpandTabs(t *testing.T) {
	orig := globalTabWidth
	defer func() { globalTabWidth = orig }()

	SetTabWidth(4)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no tabs", "hello", "hello"},
		{"one tab", "a\tb", "a    b"},
		{"leading tab", "\thello", "    hello"},
		{"trailing tab", "hello\t", "hello    "},
		{"two tabs", "a\t\tb", "a        b"},
		{"empty", "", ""},
		{"only tab", "\t", "    "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandTabs(tt.in)
			if got != tt.want {
				t.Errorf("expandTabs(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExpandTabsWidthChange(t *testing.T) {
	orig := globalTabWidth
	defer func() { globalTabWidth = orig }()

	SetTabWidth(2)
	got := expandTabs("a\tb")
	if got != "a  b" {
		t.Errorf("tabWidth=2: expandTabs(%q) = %q, want %q", "a\tb", got, "a  b")
	}

	SetTabWidth(8)
	got = expandTabs("a\tb")
	if got != "a        b" {
		t.Errorf("tabWidth=8: expandTabs(%q) = %q, want %q", "a\tb", got, "a        b")
	}
}

func TestTabAdvance(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	size := 16.0

	orig := globalTabWidth
	defer func() { globalTabWidth = orig }()

	// Tab advance = globalTabWidth × space advance.
	spaceGID := parsed.GlyphIndex(' ')
	spaceAdv := parsed.GlyphAdvance(spaceGID, size)

	SetTabWidth(8)
	gid, adv := tabAdvance(parsed, size)
	if gid != spaceGID {
		t.Errorf("tabAdvance GID = %d, want space GID %d", gid, spaceGID)
	}
	want := spaceAdv * 8
	if adv != want {
		t.Errorf("tabAdvance advance = %f, want %f (8 × space)", adv, want)
	}

	SetTabWidth(4)
	_, adv = tabAdvance(parsed, size)
	want = spaceAdv * 4
	if adv != want {
		t.Errorf("tabWidth=4: advance = %f, want %f", adv, want)
	}
}

func TestFaceAdvanceWithTab(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16.0)

	// "A\tB" should be wider than "AB".
	advAB := face.Advance("AB")
	advATabB := face.Advance("A\tB")

	if advATabB <= advAB {
		t.Errorf("Advance(\"A\\tB\") = %f should be > Advance(\"AB\") = %f", advATabB, advAB)
	}

	// Tab advance should equal space advance × globalTabWidth.
	advA := face.Advance("A")
	advB := face.Advance("B")
	advTab := face.Advance("\t")
	expected := advA + advTab + advB
	if advATabB != expected {
		t.Errorf("Advance(\"A\\tB\") = %f, want %f (A + tab + B)", advATabB, expected)
	}
}

func TestFaceGlyphsWithTab(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16.0)

	glyphs := make([]Glyph, 0, 3)
	for g := range face.Glyphs("A\tB") {
		glyphs = append(glyphs, g)
	}

	if len(glyphs) != 3 {
		t.Fatalf("Glyphs(\"A\\tB\") returned %d glyphs, want 3", len(glyphs))
	}

	// Tab glyph should have '\t' rune.
	if glyphs[1].Rune != '\t' {
		t.Errorf("glyphs[1].Rune = %q, want '\\t'", glyphs[1].Rune)
	}

	// Tab glyph should use space GID (not 0 / .notdef).
	if glyphs[1].GID == 0 {
		t.Error("tab glyph GID = 0 (.notdef), want space GID")
	}

	// Tab advance should be positive.
	if glyphs[1].Advance <= 0 {
		t.Errorf("tab glyph advance = %f, want > 0", glyphs[1].Advance)
	}

	// B should start after A + tab advance.
	if glyphs[2].X <= glyphs[0].X+glyphs[0].Advance {
		t.Errorf("B.X = %f should be > A.X + A.Advance = %f", glyphs[2].X, glyphs[0].X+glyphs[0].Advance)
	}
}

func TestFaceGlyphsSkipsControlChars(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16.0)

	// Control characters (except \t) should be skipped.
	var count int
	for range face.Glyphs("A\x01\x02B") {
		count++
	}
	if count != 2 {
		t.Errorf("Glyphs with control chars: got %d glyphs, want 2 (A, B)", count)
	}
}

func TestFixTabGlyphs(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16.0)
	runes := []rune{'H', '\t', 'W'}

	// Simulate HarfBuzz output with notdef (GID=0) for tab.
	glyphs := []ShapedGlyph{
		{GID: 42, Cluster: 0, X: 0, XAdvance: 10},
		{GID: 0, Cluster: 1, X: 10, XAdvance: 0}, // notdef for tab
		{GID: 50, Cluster: 2, X: 10, XAdvance: 10},
	}

	fixTabGlyphs(glyphs, runes, face)

	// Tab GID should now be space GID, not 0.
	if glyphs[1].GID == 0 {
		t.Error("fixTabGlyphs: tab GID still 0 (.notdef)")
	}

	// Tab advance should be positive.
	if glyphs[1].XAdvance <= 0 {
		t.Errorf("fixTabGlyphs: tab XAdvance = %f, want > 0", glyphs[1].XAdvance)
	}

	// W should be repositioned after tab.
	expectedX := glyphs[0].XAdvance + glyphs[1].XAdvance
	if glyphs[2].X != expectedX {
		t.Errorf("fixTabGlyphs: W.X = %f, want %f", glyphs[2].X, expectedX)
	}
}
