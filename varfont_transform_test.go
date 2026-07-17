package gg

import (
	"os"
	"testing"

	"github.com/gogpu/gg/text"
)

// findVariableFont returns a path to a system variable font for testing.
func findVariableFont(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"C:\\Windows\\Fonts\\bahnschrift.ttf",
		"C:\\Windows\\Fonts\\CascadiaCode.ttf",
		"/System/Library/Fonts/SFNS.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-VF.ttf",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// setupVariableTextContext creates a context with a variable font at given weight.
func setupVariableTextContext(t *testing.T, weight float32) (*Context, *text.FontSource) {
	t.Helper()
	fontPath := findVariableFont(t)
	if fontPath == "" {
		t.Skip("No variable font available on this system")
	}
	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load variable font: %v", err)
	}
	if !source.IsVariable() {
		_ = source.Close()
		t.Skip("Font is not variable")
	}
	dc := NewContext(400, 200)
	face := source.Face(40, text.WithVariations(
		text.NewFontVariation("wght", weight),
	))
	dc.SetFont(face)
	dc.SetRGB(0, 0, 0)
	dc.ClearWithColor(White)
	return dc, source
}

// TestADR054_ShearVariableFont_BoldRendersPixels verifies that variable font
// wght=700 produces visible bold text under shear transform (the @tsl0922 bug).
func TestADR054_ShearVariableFont_BoldRendersPixels(t *testing.T) {
	dc, source := setupVariableTextContext(t, 700)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Shear(-0.3, 0)
	dc.DrawString("Test", 100, 100)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 400, 200)
	if pixels == 0 {
		t.Fatal("Variable font wght=700 under shear produced no pixels — gvar not applied")
	}
}

// TestADR054_ShearVariableFont_BoldVsRegular verifies that wght=700 produces
// MORE pixels than wght=400 under the same shear. This catches the original bug
// where gvar deltas were not applied — both weights rendered identically.
func TestADR054_ShearVariableFont_BoldVsRegular(t *testing.T) {
	fontPath := findVariableFont(t)
	if fontPath == "" {
		t.Skip("No variable font available on this system")
	}
	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load variable font: %v", err)
	}
	defer func() { _ = source.Close() }()
	if !source.IsVariable() {
		t.Skip("Font is not variable")
	}

	renderWithWeight := func(weight float32) int {
		dc := NewContext(400, 200)
		face := source.Face(40, text.WithVariations(
			text.NewFontVariation("wght", weight),
		))
		dc.SetFont(face)
		dc.SetRGB(0, 0, 0)
		dc.ClearWithColor(White)

		dc.Push()
		dc.Shear(-0.3, 0)
		dc.DrawString("Bold", 100, 100)
		dc.Pop()

		return countNonWhitePixels(dc, 0, 0, 400, 200)
	}

	regular := renderWithWeight(400)
	bold := renderWithWeight(700)

	if regular == 0 {
		t.Fatal("wght=400 under shear produced no pixels")
	}
	if bold == 0 {
		t.Fatal("wght=700 under shear produced no pixels")
	}
	if bold <= regular {
		t.Errorf("wght=700 (%d pixels) should produce more pixels than wght=400 (%d pixels) — gvar deltas not applied",
			bold, regular)
	}
}

// TestADR054_RotateVariableFont_BoldVsRegular verifies gvar applies under rotation too.
func TestADR054_RotateVariableFont_BoldVsRegular(t *testing.T) {
	fontPath := findVariableFont(t)
	if fontPath == "" {
		t.Skip("No variable font available on this system")
	}
	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load variable font: %v", err)
	}
	defer func() { _ = source.Close() }()
	if !source.IsVariable() {
		t.Skip("Font is not variable")
	}

	renderWithWeight := func(weight float32) int {
		dc := NewContext(400, 400)
		face := source.Face(30, text.WithVariations(
			text.NewFontVariation("wght", weight),
		))
		dc.SetFont(face)
		dc.SetRGB(0, 0, 0)
		dc.ClearWithColor(White)

		dc.Push()
		dc.RotateAbout(0.2, 200, 200)
		dc.DrawString("Rotated", 100, 200)
		dc.Pop()

		return countNonWhitePixels(dc, 0, 0, 400, 400)
	}

	regular := renderWithWeight(400)
	bold := renderWithWeight(700)

	if regular == 0 {
		t.Fatal("wght=400 under rotation produced no pixels")
	}
	if bold == 0 {
		t.Fatal("wght=700 under rotation produced no pixels")
	}
	if bold <= regular {
		t.Errorf("wght=700 (%d pixels) should produce more pixels than wght=400 (%d pixels) under rotation",
			bold, regular)
	}
}

// TestADR054_TextPath_VariableFont verifies TextPath() applies gvar deltas.
// Bold glyphs have different contour coordinates than regular — we compare raw
// path coordinates to detect variation application.
func TestADR054_TextPath_VariableFont(t *testing.T) {
	fontPath := findVariableFont(t)
	if fontPath == "" {
		t.Skip("No variable font available on this system")
	}
	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load variable font: %v", err)
	}
	defer func() { _ = source.Close() }()
	if !source.IsVariable() {
		t.Skip("Font is not variable")
	}

	pathForWeight := func(weight float32) *Path {
		dc := NewContext(400, 200)
		face := source.Face(40, text.WithVariations(
			text.NewFontVariation("wght", weight),
		))
		dc.SetFont(face)
		return dc.TextPath("A", 100, 100)
	}

	regularPath := pathForWeight(400)
	boldPath := pathForWeight(700)

	if regularPath == nil {
		t.Fatal("TextPath with wght=400 returned nil")
	}
	if boldPath == nil {
		t.Fatal("TextPath with wght=700 returned nil")
	}

	regularCoords := regularPath.Coords()
	boldCoords := boldPath.Coords()

	if len(regularCoords) == 0 {
		t.Fatal("Regular TextPath has no coordinates")
	}
	if len(boldCoords) == 0 {
		t.Fatal("Bold TextPath has no coordinates")
	}

	// Compare coordinate data — bold glyphs have different control points.
	coordsMatch := len(regularCoords) == len(boldCoords)
	if coordsMatch {
		for i := range regularCoords {
			if regularCoords[i] != boldCoords[i] {
				coordsMatch = false
				break
			}
		}
	}
	if coordsMatch {
		t.Error("TextPath wght=700 coordinates identical to wght=400 — gvar deltas not applied")
	}
}

// TestADR054_TranslationOnly_VariableFont verifies that axis-aligned transforms
// (bitmap path) still work correctly with variations — regression guard.
func TestADR054_TranslationOnly_VariableFont(t *testing.T) {
	dc, source := setupVariableTextContext(t, 700)
	defer func() { _ = source.Close() }()

	dc.DrawString("NoTransform", 50, 100)
	pixels := countNonWhitePixels(dc, 0, 0, 400, 200)
	if pixels == 0 {
		t.Fatal("Variable font wght=700 without transform produced no pixels")
	}
}
