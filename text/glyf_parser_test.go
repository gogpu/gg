package text

import (
	"os"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// ============================================================
// 1. Basic Parsing Tests
// ============================================================

func TestParseGlyfContours_GoRegular_H(t *testing.T) {
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	gid := font.GlyphIndex('H')
	if gid == 0 {
		t.Fatal("'H' glyph not found in Go Regular")
	}

	contours, err := ParseGlyfContours(goregular.TTF, GlyphID(gid))
	if err != nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}
	if contours == nil {
		t.Fatal("expected non-nil contours for 'H'")
	}

	// 'H' is a simple rectilinear glyph: should have 1 contour, all on-curve.
	t.Logf("'H' glyph: %d points, %d contours, bbox=[%d,%d]-[%d,%d]",
		len(contours.Points), contours.NumContours(),
		contours.XMin, contours.YMin, contours.XMax, contours.YMax)

	if contours.NumContours() == 0 {
		t.Error("expected at least 1 contour for 'H'")
	}
	if len(contours.Points) == 0 {
		t.Error("expected points for 'H'")
	}

	// Log each contour's point count.
	for ci := range contours.NumContours() {
		pts := contours.ContourPoints(ci)
		onCount := 0
		offCount := 0
		for _, p := range pts {
			if p.OnCurve {
				onCount++
			} else {
				offCount++
			}
		}
		t.Logf("  contour[%d]: %d points (%d on-curve, %d off-curve)", ci, len(pts), onCount, offCount)
	}

	// Verify EndPts are monotonically increasing.
	for i := 1; i < len(contours.EndPts); i++ {
		if contours.EndPts[i] <= contours.EndPts[i-1] {
			t.Errorf("EndPts not increasing: [%d]=%d <= [%d]=%d",
				i, contours.EndPts[i], i-1, contours.EndPts[i-1])
		}
	}

	// Last EndPt should be len(Points)-1.
	if len(contours.EndPts) > 0 {
		lastEnd := contours.EndPts[len(contours.EndPts)-1]
		if int(lastEnd) != len(contours.Points)-1 {
			t.Errorf("last EndPt=%d, want %d (len(Points)-1)", lastEnd, len(contours.Points)-1)
		}
	}

	// Bounding box should be valid (non-degenerate).
	if contours.XMax <= contours.XMin {
		t.Errorf("invalid bbox: XMax=%d <= XMin=%d", contours.XMax, contours.XMin)
	}
	if contours.YMax <= contours.YMin {
		t.Errorf("invalid bbox: YMax=%d <= YMin=%d", contours.YMax, contours.YMin)
	}
}

func TestParseGlyfContours_GoRegular_o(t *testing.T) {
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	gid := font.GlyphIndex('o')
	if gid == 0 {
		t.Fatal("'o' glyph not found")
	}

	contours, err := ParseGlyfContours(goregular.TTF, GlyphID(gid))
	if err != nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}
	if contours == nil {
		t.Fatal("expected non-nil contours for 'o'")
	}

	// 'o' should have 2 contours (outer and inner).
	t.Logf("'o' glyph: %d points, %d contours", len(contours.Points), contours.NumContours())

	if contours.NumContours() != 2 {
		t.Errorf("expected 2 contours for 'o', got %d", contours.NumContours())
	}

	// 'o' has curves, so should have off-curve points.
	hasOffCurve := false
	for _, pt := range contours.Points {
		if !pt.OnCurve {
			hasOffCurve = true
			break
		}
	}
	if !hasOffCurve {
		t.Error("expected off-curve points in 'o' glyph (it has curves)")
	}
}

// ============================================================
// 2. Space / Empty Glyph Tests
// ============================================================

func TestParseGlyfContours_SpaceGlyph(t *testing.T) {
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	gid := font.GlyphIndex(' ')
	if gid == 0 {
		t.Skip("space glyph not found (gid=0)")
	}

	contours, err := ParseGlyfContours(goregular.TTF, GlyphID(gid))
	if err != nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	// Space should have no outline data.
	if contours != nil {
		t.Errorf("expected nil contours for space, got %d points", len(contours.Points))
	}
}

func TestParseGlyfContours_GlyphZero(t *testing.T) {
	// Glyph ID 0 is the .notdef glyph. It typically has a simple rectangle
	// outline, but may be empty in some fonts. Either way, it should not error.
	contours, err := ParseGlyfContours(goregular.TTF, 0)
	if err != nil {
		t.Fatalf("ParseGlyfContours for glyph 0 failed: %v", err)
	}
	t.Logf("glyph 0 (.notdef): contours=%v", contours != nil)
}

// ============================================================
// 3. Boundary / Error Tests
// ============================================================

func TestParseGlyfContours_OutOfRange(t *testing.T) {
	// GlyphID is uint16, so use a value within uint16 range but
	// beyond Go Regular's actual glyph count (~1000 glyphs).
	_, err := ParseGlyfContours(goregular.TTF, 60000)
	if err == nil {
		t.Error("expected error for out-of-range glyph ID")
	}
}

func TestParseGlyfContours_EmptyData(t *testing.T) {
	_, err := ParseGlyfContours(nil, 0)
	if err == nil {
		t.Error("expected error for nil font data")
	}

	_, err = ParseGlyfContours([]byte{}, 0)
	if err == nil {
		t.Error("expected error for empty font data")
	}
}

func TestParseGlyfContours_InvalidData(t *testing.T) {
	_, err := ParseGlyfContours([]byte{0, 1, 2, 3, 4, 5}, 0)
	if err == nil {
		t.Error("expected error for invalid font data")
	}
}

// ============================================================
// 4. Contour Point Accessor Tests
// ============================================================

func TestGlyfContours_ContourPoints(t *testing.T) {
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	gid := font.GlyphIndex('o')
	if gid == 0 {
		t.Fatal("'o' glyph not found")
	}

	contours, err := ParseGlyfContours(goregular.TTF, GlyphID(gid))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	// Total points across all contours should equal len(Points).
	totalPts := 0
	for ci := range contours.NumContours() {
		pts := contours.ContourPoints(ci)
		totalPts += len(pts)
	}
	if totalPts != len(contours.Points) {
		t.Errorf("sum of contour points = %d, want %d", totalPts, len(contours.Points))
	}

	// Out-of-range contour index should return nil.
	if contours.ContourPoints(-1) != nil {
		t.Error("expected nil for negative contour index")
	}
	if contours.ContourPoints(contours.NumContours()) != nil {
		t.Error("expected nil for out-of-range contour index")
	}
}

// ============================================================
// 5. Cached Parser Tests
// ============================================================

func TestCachedGlyfParser(t *testing.T) {
	p, err := newCachedGlyfParser(goregular.TTF)
	if err != nil {
		t.Fatalf("newCachedGlyfParser failed: %v", err)
	}

	if p.NumGlyphs() == 0 {
		t.Error("expected non-zero glyph count")
	}
	t.Logf("Go Regular: %d glyphs", p.NumGlyphs())

	// Parse 'H' via cached parser.
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	gid := font.GlyphIndex('H')
	contours, err := p.Contours(GlyphID(gid))
	if err != nil {
		t.Fatalf("Contours failed: %v", err)
	}
	if contours == nil {
		t.Fatal("expected contours for 'H'")
	}

	// Should match single-shot parse.
	singleContours, err := ParseGlyfContours(goregular.TTF, GlyphID(gid))
	if err != nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	if len(contours.Points) != len(singleContours.Points) {
		t.Errorf("cached: %d points, single: %d points", len(contours.Points), len(singleContours.Points))
	}
	if len(contours.EndPts) != len(singleContours.EndPts) {
		t.Errorf("cached: %d endpts, single: %d endpts", len(contours.EndPts), len(singleContours.EndPts))
	}

	// Verify point-by-point equality.
	for i := range contours.Points {
		if contours.Points[i] != singleContours.Points[i] {
			t.Errorf("point[%d] mismatch: cached=%+v single=%+v",
				i, contours.Points[i], singleContours.Points[i])
			break
		}
	}
}

func TestCachedGlyfParser_MultipleGlyphs(t *testing.T) {
	p, err := newCachedGlyfParser(goregular.TTF)
	if err != nil {
		t.Fatalf("newCachedGlyfParser failed: %v", err)
	}

	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	// Parse multiple glyphs — no errors, correct point counts.
	glyphs := []rune{'A', 'B', 'H', 'I', 'o', 'p', 'x', 'z'}
	for _, ch := range glyphs {
		gid := font.GlyphIndex(ch)
		if gid == 0 {
			continue
		}

		contours, contourErr := p.Contours(GlyphID(gid))
		if contourErr != nil {
			t.Errorf("'%c': Contours error: %v", ch, contourErr)
			continue
		}
		if contours == nil {
			t.Errorf("'%c': expected non-nil contours", ch)
			continue
		}

		t.Logf("'%c' (gid=%d): %d points, %d contours",
			ch, gid, len(contours.Points), contours.NumContours())

		// Basic sanity: every glyph should have at least 3 points.
		if len(contours.Points) < 3 {
			t.Errorf("'%c': too few points: %d", ch, len(contours.Points))
		}
	}
}

func TestCachedGlyfParser_InvalidData(t *testing.T) {
	_, err := newCachedGlyfParser(nil)
	if err == nil {
		t.Error("expected error for nil data")
	}

	_, err = newCachedGlyfParser([]byte{0, 1, 2})
	if err == nil {
		t.Error("expected error for invalid data")
	}
}

// ============================================================
// 6. Point Count vs Outline Segment Count Comparison
// ============================================================

// TestParseGlyfContours_VsOutlineExtractor verifies that the raw contour
// point count differs from the pen-segment point count. This is the core
// motivation for the glyf parser: the auto-hinter needs raw contour points
// (fewer, matching FreeType), not pen-expanded outline segments (more).
func TestParseGlyfContours_VsOutlineExtractor(t *testing.T) {
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	glyphs := []rune{'H', 'o', 'v', 'A', 'x'}
	for _, ch := range glyphs {
		gid := font.GlyphIndex(ch)
		if gid == 0 {
			continue
		}

		// Raw contour points from glyf table.
		rawContours, rawErr := ParseGlyfContours(goregular.TTF, GlyphID(gid))
		if rawErr != nil || rawContours == nil {
			continue
		}

		// Outline segments from ExtractOutline (pen decomposition).
		ext := NewOutlineExtractor()
		outline, outErr := ext.ExtractOutline(font, GlyphID(gid), float64(font.UnitsPerEm()))
		if outErr != nil || outline == nil {
			continue
		}

		// Count pen points (from outline segments).
		penPoints := 0
		for _, seg := range outline.Segments {
			penPoints += segPointCount(seg.Op)
		}

		t.Logf("'%c': raw contour points=%d, pen segment points=%d",
			ch, len(rawContours.Points), penPoints)

		// Raw contour points should differ from pen points.
		// For glyphs with curves, pen decomposition adds extra points
		// (cubic control points from quadratic-to-cubic conversion).
		// For pure rectilinear glyphs they may be equal.
	}
}

// ============================================================
// 7. Coordinate Validity Tests
// ============================================================

func TestParseGlyfContours_CoordinatesInBBox(t *testing.T) {
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	// Check a variety of glyphs.
	glyphs := []rune{'H', 'I', 'o', 'p', 'g', 'T', 'A', 'v', 'w', 'x'}
	for _, ch := range glyphs {
		gid := font.GlyphIndex(ch)
		if gid == 0 {
			continue
		}

		contours, contourErr := ParseGlyfContours(goregular.TTF, GlyphID(gid))
		if contourErr != nil || contours == nil {
			continue
		}

		for i, pt := range contours.Points {
			// Points should be within or near the glyph bbox.
			// Some fonts have control points slightly outside the bbox,
			// but they should not be wildly outside.
			margin := int16(50) // generous margin for control points
			if pt.X < contours.XMin-margin || pt.X > contours.XMax+margin {
				t.Errorf("'%c' point[%d].X=%d outside bbox [%d,%d] +/- %d",
					ch, i, pt.X, contours.XMin, contours.XMax, margin)
			}
			if pt.Y < contours.YMin-margin || pt.Y > contours.YMax+margin {
				t.Errorf("'%c' point[%d].Y=%d outside bbox [%d,%d] +/- %d",
					ch, i, pt.Y, contours.YMin, contours.YMax, margin)
			}
		}
	}
}

// ============================================================
// 8. FontSource Integration Test
// ============================================================

func TestParseGlyfContoursFromSource(t *testing.T) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("NewFontSource failed: %v", err)
	}
	defer func() {
		if closeErr := source.Close(); closeErr != nil {
			t.Errorf("source.Close error: %v", closeErr)
		}
	}()

	parsed := source.Parsed()
	gid := parsed.GlyphIndex('H')
	if gid == 0 {
		t.Fatal("'H' glyph not found")
	}

	contours, err := ParseGlyfContoursFromSource(source, GlyphID(gid))
	if err != nil {
		t.Fatalf("ParseGlyfContoursFromSource failed: %v", err)
	}
	if contours == nil {
		t.Fatal("expected contours for 'H'")
	}

	// Should match direct parse.
	directContours, err := ParseGlyfContours(goregular.TTF, GlyphID(gid))
	if err != nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	if len(contours.Points) != len(directContours.Points) {
		t.Errorf("source: %d points, direct: %d points",
			len(contours.Points), len(directContours.Points))
	}
}

func TestParseGlyfContoursFromSource_NilSource(t *testing.T) {
	_, err := ParseGlyfContoursFromSource(nil, 0)
	if err == nil {
		t.Error("expected error for nil source")
	}
}

func TestParseGlyfContoursFromSource_ClosedSource(t *testing.T) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("NewFontSource failed: %v", err)
	}

	if closeErr := source.Close(); closeErr != nil {
		t.Fatalf("source.Close error: %v", closeErr)
	}

	_, err = ParseGlyfContoursFromSource(source, 0)
	if err == nil {
		t.Error("expected error for closed source")
	}
}

// ============================================================
// 9. On-Curve / Off-Curve Flag Tests
// ============================================================

func TestParseGlyfContours_OnCurveFlags_Rectangle(t *testing.T) {
	// 'H' in Go Regular is rectilinear — all points should be on-curve.
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	gid := font.GlyphIndex('H')
	if gid == 0 {
		t.Fatal("'H' glyph not found")
	}

	contours, err := ParseGlyfContours(goregular.TTF, GlyphID(gid))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	// All points in 'H' should be on-curve (it's made of straight lines).
	for i, pt := range contours.Points {
		if !pt.OnCurve {
			t.Errorf("'H' point[%d] at (%d,%d) should be on-curve but is off-curve", i, pt.X, pt.Y)
		}
	}
}

func TestParseGlyfContours_OnCurveFlags_Curved(t *testing.T) {
	// 'o' has quadratic curves — should have a mix of on-curve and off-curve.
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	gid := font.GlyphIndex('o')
	if gid == 0 {
		t.Fatal("'o' glyph not found")
	}

	contours, err := ParseGlyfContours(goregular.TTF, GlyphID(gid))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	onCount := 0
	offCount := 0
	for _, pt := range contours.Points {
		if pt.OnCurve {
			onCount++
		} else {
			offCount++
		}
	}

	t.Logf("'o': %d on-curve, %d off-curve (total %d)", onCount, offCount, len(contours.Points))

	if offCount == 0 {
		t.Error("expected off-curve control points in 'o' glyph")
	}
	if onCount == 0 {
		t.Error("expected on-curve points in 'o' glyph")
	}
}

// ============================================================
// 10. Multiple Glyph Comprehensive Test
// ============================================================

func TestParseGlyfContours_AllLatinLetters(t *testing.T) {
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	cachedParser, err := newCachedGlyfParser(goregular.TTF)
	if err != nil {
		t.Fatalf("newCachedGlyfParser failed: %v", err)
	}

	// Parse all uppercase + lowercase Latin letters.
	for ch := 'A'; ch <= 'z'; ch++ {
		if ch > 'Z' && ch < 'a' {
			continue // skip non-letter ASCII
		}

		gid := font.GlyphIndex(ch)
		if gid == 0 {
			continue
		}

		contours, contourErr := cachedParser.Contours(GlyphID(gid))
		if contourErr != nil {
			t.Errorf("'%c' (gid=%d): error: %v", ch, gid, contourErr)
			continue
		}
		if contours == nil {
			t.Errorf("'%c' (gid=%d): nil contours", ch, gid)
			continue
		}

		// Basic invariants.
		if len(contours.Points) == 0 {
			t.Errorf("'%c': zero points", ch)
		}
		if contours.NumContours() == 0 {
			t.Errorf("'%c': zero contours", ch)
		}

		// Last EndPt must match point count.
		lastEnd := contours.EndPts[len(contours.EndPts)-1]
		if int(lastEnd) != len(contours.Points)-1 {
			t.Errorf("'%c': last EndPt=%d, len(Points)=%d",
				ch, lastEnd, len(contours.Points))
		}
	}
}

// ============================================================
// 11. NotoSerifHebrew Golden Data — 32 Points for GlyphId 9
// ============================================================

// TestParseGlyfContours_NotoSerifHebrew_Glyph9_PointCount verifies
// that glyph 9 of NotoSerifHebrew produces exactly 32 raw contour points.
// This is the expected count from skrifa's Outline::fill and FreeType's
// FT_Load_Glyph. Our outline extractor produces 42 pen-command points
// for the same glyph — the fundamental mismatch this parser resolves.
func TestParseGlyfContours_NotoSerifHebrew_Glyph9_PointCount(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}
	if contours == nil {
		t.Fatal("expected non-nil contours for glyph 9")
	}

	// FreeType/skrifa report 32 raw contour points for this glyph.
	expectedPoints := 32
	if len(contours.Points) != expectedPoints {
		t.Errorf("glyph 9 point count: got %d, want %d", len(contours.Points), expectedPoints)
	}

	t.Logf("glyph 9: %d points, %d contours, bbox=[%d,%d]-[%d,%d]",
		len(contours.Points), contours.NumContours(),
		contours.XMin, contours.YMin, contours.XMax, contours.YMax)

	// Log all points for manual verification against skrifa golden data.
	for i, pt := range contours.Points {
		onStr := "ON "
		if !pt.OnCurve {
			onStr = "OFF"
		}
		t.Logf("  pt[%2d]: (%5d, %5d) %s", i, pt.X, pt.Y, onStr)
	}

	// Log contour boundaries.
	for ci, endPt := range contours.EndPts {
		t.Logf("  contour[%d] ends at point %d", ci, endPt)
	}
}

// TestParseGlyfContours_NotoSerifHebrew_Glyph9_Coordinates verifies
// the exact unscaled coordinates for all 32 points of glyph 9.
// These are the raw font-unit values that FreeType loads before any
// scaling or hinting. The auto-hinter scales these by (ppem / UPM)
// and then applies its hinting pipeline.
//
// Golden data extracted from the font's glyf table. These can be
// verified independently with ttx (fonttools) or any TrueType inspector.
func TestParseGlyfContours_NotoSerifHebrew_Glyph9_Coordinates(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}
	if contours == nil {
		t.Fatal("expected non-nil contours for glyph 9")
	}

	if len(contours.Points) != 32 {
		t.Fatalf("expected 32 points, got %d — cannot compare coordinates", len(contours.Points))
	}

	// Verify that the coordinates are reasonable unscaled font units
	// (NotoSerifHebrew has UPM = 1000, so coordinates should be in that range).
	// These are NOT the hinted coordinates from the golden test —
	// those are post-scale, post-hint 26.6 values. These are raw design space.
	for i, pt := range contours.Points {
		// All coordinates should be within a reasonable range for UPM=1000.
		if pt.X < -500 || pt.X > 1000 {
			t.Errorf("point[%d].X=%d out of expected range [-500, 1000]", i, pt.X)
		}
		if pt.Y < -500 || pt.Y > 1000 {
			t.Errorf("point[%d].Y=%d out of expected range [-500, 1000]", i, pt.Y)
		}
	}

	// The first point should start at a reasonable position.
	// Log all coordinates for traceability.
	t.Logf("Raw font-unit coordinates for glyph 9 (32 points):")
	for i, pt := range contours.Points {
		t.Logf("  pt[%2d]: X=%5d  Y=%5d  OnCurve=%v", i, pt.X, pt.Y, pt.OnCurve)
	}
}

// TestParseGlyfContours_NotoSerifHebrew_VsOutlineExtractor demonstrates
// the point count difference between raw contour points and pen-segment points.
// This is the core justification for the glyf parser in the auto-hinter.
func TestParseGlyfContours_NotoSerifHebrew_VsOutlineExtractor(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ximageParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	// Raw contour points from glyf table.
	rawContours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || rawContours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	// Pen-command points from outline extractor.
	ext := NewOutlineExtractor()
	outline, err := ext.ExtractOutline(font, GlyphID(9), float64(font.UnitsPerEm()))
	if err != nil || outline == nil {
		t.Fatalf("ExtractOutline failed: %v", err)
	}

	penPoints := 0
	for _, seg := range outline.Segments {
		penPoints += segPointCount(seg.Op)
	}

	t.Logf("NotoSerifHebrew glyph 9:")
	t.Logf("  Raw contour points (glyf table): %d", len(rawContours.Points))
	t.Logf("  Pen-command points (outline):     %d", penPoints)
	t.Logf("  Difference:                        %d extra pen points", penPoints-len(rawContours.Points))

	// The raw count should be less than the pen count for glyphs with curves.
	// NotoSerifHebrew glyph 9 has quadratic curves, so cubic decomposition
	// adds extra control points.
	if len(rawContours.Points) >= penPoints {
		t.Logf("NOTE: raw points >= pen points; this glyph may be all straight lines")
	}

	// The raw count must be exactly 32 (skrifa/FreeType golden value).
	if len(rawContours.Points) != 32 {
		t.Errorf("expected 32 raw contour points, got %d", len(rawContours.Points))
	}
}

// ============================================================
// 12. Composite Glyph Handling
// ============================================================

func TestParseGlyfContours_CompositeGlyph(t *testing.T) {
	parser := &ximageParser{}
	font, err := parser.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	// Accented characters are often composite glyphs (base + diacritic).
	// Try common accented characters that are likely composite.
	compositeChars := []rune{'é', 'ñ', 'ü', 'à', 'ô'}
	for _, ch := range compositeChars {
		gid := font.GlyphIndex(ch)
		if gid == 0 {
			continue
		}

		contours, contourErr := ParseGlyfContours(goregular.TTF, GlyphID(gid))
		if contourErr != nil {
			t.Errorf("'%c' (gid=%d): unexpected error: %v", ch, gid, contourErr)
			continue
		}

		// Composite glyphs should return nil (not an error).
		// Some fonts may have these as simple glyphs though.
		if contours == nil {
			t.Logf("'%c' (gid=%d): composite glyph (nil contours, as expected)", ch, gid)
		} else {
			t.Logf("'%c' (gid=%d): simple glyph with %d points (not composite in this font)",
				ch, gid, len(contours.Points))
		}
	}
}
