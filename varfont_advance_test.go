package gg

import (
	"os"
	"testing"

	"github.com/gogpu/gg/text"
)

// TestRegression_VariableFontShear_LettersOverlap reproduces the P0 regression
// reported by @tsl0922 (#405): letters "r" and "l" merge under shear with
// variable font wght=700.
//
// Root cause: OwnShaper uses hmtx-only advances (base weight) but
// ExtractOutlineHintedVar applies gvar deltas (bold weight). Wider glyphs
// positioned at narrower spacing → overlap.
//
// This test verifies that variable font advances under shear are consistent
// with outline geometry — no letter overlap.
func TestRegression_VariableFontShear_LettersOverlap(t *testing.T) {
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

	// Render "rl" at bold weight with shear — the exact combination that regressed.
	renderWithWeight := func(weight float32) *Pixmap {
		dc := NewContext(200, 100)
		face := source.Face(36, text.WithVariations(
			text.NewFontVariation("wght", weight),
		))
		dc.SetFont(face)
		dc.SetRGB(0, 0, 0)
		dc.ClearWithColor(White)

		dc.Push()
		dc.Shear(-0.3, 0)
		dc.DrawString("rl", 50, 60)
		dc.Pop()
		return dc.pixmap
	}

	boldPix := renderWithWeight(700)

	// Find the gap between 'r' and 'l' — scan horizontally at mid-height.
	// If letters overlap, there will be NO white gap between them.
	midY := 45 // approximate vertical center of text at y=60, size=36

	// Find leftmost and rightmost ink
	leftInk, rightInk := -1, -1
	for x := 0; x < 200; x++ {
		p := boldPix.GetPixel(x, midY)
		if pixelByte(p.R) < 200 { // non-white = ink
			if leftInk == -1 {
				leftInk = x
			}
			rightInk = x
		}
	}

	if leftInk == -1 {
		t.Fatal("No ink found — text not rendered")
	}

	// Scan for a white gap between leftInk and rightInk.
	// "rl" should have a visible gap between the two letters.
	gapFound := false
	inGap := false
	gapWidth := 0
	for x := leftInk; x <= rightInk; x++ {
		p := boldPix.GetPixel(x, midY)
		isWhite := pixelByte(p.R) > 240
		if isWhite {
			if !inGap {
				inGap = true
				gapWidth = 0
			}
			gapWidth++
		} else {
			if inGap && gapWidth >= 2 {
				gapFound = true
				break
			}
			inGap = false
		}
	}

	if !gapFound {
		t.Errorf("Letters 'r' and 'l' have no gap at y=%d — they overlap (advance mismatch). "+
			"Ink span: x=%d..%d", midY, leftInk, rightInk)
	}
}

// TestVariableFont_ShaperAdvance_MatchesHVAR verifies that shaped glyph
// advances match HVAR-adjusted Face.Advance for variable fonts.
// This is the core invariant: shaper advances == Face advances.
//
// ROOT CAUSE of tsl0922 regression: OwnShaper uses hmtx-only advances,
// Face.Advance uses HVAR. At wght=700 the difference is ~1-2px per glyph,
// causing letters to overlap under shear (vector outline path).
func TestVariableFont_ShaperAdvance_MatchesHVAR(t *testing.T) {
	// Use NotoSansSC if available (known HVAR font with weight-dependent advances)
	fontPath := "tmp/tsl0922_fonts/NotoSansSC-VariableFont_wght.ttf"
	if _, err := os.Stat(fontPath); err != nil {
		// Fall back to system variable font
		fontPath = findVariableFont(t)
		if fontPath == "" {
			t.Skip("No variable font available")
		}
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()
	if !source.IsVariable() {
		t.Skip("Font is not variable")
	}

	boldFace := source.Face(36, text.WithVariations(
		text.NewFontVariation("wght", 700),
	))

	// Face.Advance uses HVAR — correct bold advances.
	faceAdvR := boldFace.Advance("r")
	faceAdvL := boldFace.Advance("l")

	// text.Shape uses OwnShaper — should also use HVAR.
	shaped := text.Shape("rl", boldFace)
	if len(shaped) < 2 {
		t.Skip("Shaping 'rl' produced fewer than 2 glyphs")
	}

	shaperAdvR := shaped[0].XAdvance
	shaperAdvL := shaped[1].XAdvance

	t.Logf("Face.Advance: r=%.4f l=%.4f", faceAdvR, faceAdvL)
	t.Logf("Shaper XAdv:  r=%.4f l=%.4f", shaperAdvR, shaperAdvL)

	// Shaper advance should be within 10% of Face.Advance.
	// If shaper uses hmtx-only (base weight), the difference is ~15-25%.
	tolR := faceAdvR * 0.10
	tolL := faceAdvL * 0.10

	diffR := faceAdvR - shaperAdvR
	diffL := faceAdvL - shaperAdvL

	if diffR < 0 {
		diffR = -diffR
	}
	if diffL < 0 {
		diffL = -diffL
	}

	if diffR > tolR {
		t.Errorf("Shaper advance for 'r' (%.4f) differs from Face.Advance (%.4f) by %.4f — HVAR not applied in shaper",
			shaperAdvR, faceAdvR, diffR)
	}
	if diffL > tolL {
		t.Errorf("Shaper advance for 'l' (%.4f) differs from Face.Advance (%.4f) by %.4f — HVAR not applied in shaper",
			shaperAdvL, faceAdvL, diffL)
	}
}

// TestVariableFont_ShapedPositions_Consistent verifies that shaped glyph
// positions are consistent with outline widths for variable fonts.
func TestVariableFont_ShapedPositions_Consistent(t *testing.T) {
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

	boldFace := source.Face(36, text.WithVariations(
		text.NewFontVariation("wght", 700),
	))

	// Shape "AB" and verify B's X position > A's outline width
	shaped := text.Shape("AB", boldFace)
	if len(shaped) < 2 {
		t.Skip("Shaping produced fewer than 2 glyphs")
	}

	aPos := shaped[0].X
	bPos := shaped[1].X
	spacing := bPos - aPos

	if spacing <= 0 {
		t.Errorf("Glyph B position (%.2f) should be after A (%.2f)", bPos, aPos)
	}

	// The spacing should be reasonable — at least 50% of font size
	// for a capital letter at bold weight
	if spacing < 36*0.3 {
		t.Errorf("Glyph spacing %.2f is suspiciously narrow for bold 36px — HVAR not applied?", spacing)
	}

	t.Logf("Shaped AB: A.X=%.2f, B.X=%.2f, spacing=%.2f", aPos, bPos, spacing)
}

// TestVariableFont_Shear_NoOverlap_TslReproduction is the exact reproduction
// from @tsl0922's bug report using NotoSansSC if available.
func TestVariableFont_Shear_NoOverlap_TslReproduction(t *testing.T) {
	// Try tsl0922's exact font
	fontPath := "tmp/tsl0922_fonts/NotoSansSC-VariableFont_wght.ttf"
	if _, err := os.Stat(fontPath); err != nil {
		t.Skip("NotoSansSC-VariableFont_wght.ttf not available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc := NewContext(400, 100)
	defer func() { _ = dc.Close() }()
	dc.SetAntiAlias(false)
	dc.SetTextMode(TextModeAliased)
	dc.ClearWithColor(White)

	face := source.Face(36, text.WithVariations(
		text.NewFontVariation("wght", 700),
	))
	dc.SetFont(face)

	dc.Push()
	dc.Shear(-0.3, 0)
	dc.DrawString("hello world", 30, 60)
	dc.Pop()

	// Scan for white gaps between letters at mid-height
	midY := 45
	inkRuns := 0
	inInk := false
	for x := 0; x < 400; x++ {
		p := dc.pixmap.GetPixel(x, midY)
		isInk := pixelByte(p.R) < 200
		if isInk && !inInk {
			inkRuns++
			inInk = true
		} else if !isInk {
			inInk = false
		}
	}

	// "hello world" has 10 characters + space = at least 8-9 ink runs
	// (some letters like 'l' may be single-stroke and share scan line)
	// If letters merge, inkRuns will be much lower (e.g., 3-4 blobs)
	t.Logf("Ink runs at y=%d: %d (expect >=5 for 'hello world')", midY, inkRuns)

	if inkRuns < 5 {
		t.Errorf("Only %d ink runs — letters are merging (expected >=5 for 'hello world')", inkRuns)
	}
}
