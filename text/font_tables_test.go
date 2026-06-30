// Tests for Pure Go font table parsers (Phase 3a, ADR-048).
//
// Cross-validates own parser output against ximageParsedFont output.
// Every test font's results must match between the two parsers.
//
// Test fonts (from testdata/):
//   - tthint_subset.ttf:   upem=1040, numGlyphs=3, 'A' at GID 1
//   - ahem.ttf:            simple test font, uniform advances
//   - cousine_hint_subset.ttf: Google Cousine monospace, TT hinted
//   - notoserifhebrew_autohint_metrics.ttf: Hebrew script
package text

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// loadTestFontData reads a font file from testdata/.
func loadTestFontData(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to load test font %s: %v", name, err)
	}
	return data
}

// parseWithOwn parses font data using the own parser.
func parseWithOwn(t *testing.T, data []byte) ParsedFont {
	t.Helper()
	parser := &ownParser{}
	parsed, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("own parser failed: %v", err)
	}
	return parsed
}

// parseWithXimage parses font data using the ximage parser.
func parseWithXimage(t *testing.T, data []byte) ParsedFont {
	t.Helper()
	parser := &ximageParser{}
	parsed, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("ximage parser failed: %v", err)
	}
	return parsed
}

// --- Table Directory ---

func TestParseFontTablesIndex(t *testing.T) {
	data := loadTestFontData(t, "tthint_subset.ttf")
	tables, err := parseFontTablesIndex(data, 0)
	if err != nil {
		t.Fatalf("parseFontTablesIndex failed: %v", err)
	}

	// Verify essential tables are present.
	requiredTables := []string{"head", "maxp", "cmap", "hhea", "hmtx", "name"}
	for _, tag := range requiredTables {
		if _, ok := tables[tag]; !ok {
			t.Errorf("missing required table: %s", tag)
		}
	}
}

func TestParseFontTablesIndex_InvalidIndex(t *testing.T) {
	data := loadTestFontData(t, "tthint_subset.ttf")
	// Standalone TTF — index is ignored, should parse fine.
	_, err := parseFontTablesIndex(data, 5)
	if err != nil {
		t.Fatalf("expected standalone TTF to ignore index, got error: %v", err)
	}
}

func TestParseFontTablesIndex_TruncatedData(t *testing.T) {
	_, err := parseFontTablesIndex([]byte{0, 1, 2, 3}, 0)
	if err == nil {
		t.Fatal("expected error for truncated data")
	}
}

// --- Cmap ---

func TestCmapParser(t *testing.T) {
	fonts := []struct {
		name     string
		char     rune
		wantNonZ bool // true if glyph must be present (non-.notdef)
	}{
		{"tthint_subset.ttf", 'A', true},
		{"ahem.ttf", 'A', true},
		// cousine_hint_subset is a SUBSET font — 'A' may not be present.
		// Cross-validation (own vs ximage agreement) is the primary check.
		{"cousine_hint_subset.ttf", 'A', false},
	}

	for _, tt := range fonts {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTestFontData(t, tt.name)
			own := parseWithOwn(t, data)
			ximg := parseWithXimage(t, data)

			ownGID := own.GlyphIndex(tt.char)
			ximgGID := ximg.GlyphIndex(tt.char)

			if ownGID != ximgGID {
				t.Errorf("GlyphIndex(%q): own=%d, ximage=%d", tt.char, ownGID, ximgGID)
			}
			if tt.wantNonZ && ownGID == 0 {
				t.Errorf("GlyphIndex(%q): got 0 (.notdef), expected a valid glyph", tt.char)
			}
		})
	}
}

func TestCmapParser_NotdefForMissing(t *testing.T) {
	data := loadTestFontData(t, "tthint_subset.ttf")
	own := parseWithOwn(t, data)

	// tthint_subset has only 3 glyphs — a Chinese character should return 0.
	gid := own.GlyphIndex('\u4E00')
	if gid != 0 {
		t.Errorf("expected .notdef (0) for unmapped rune, got %d", gid)
	}
}

func TestCmapParser_MultipleRunes(t *testing.T) {
	data := loadTestFontData(t, "cousine_hint_subset.ttf")
	own := parseWithOwn(t, data)
	ximg := parseWithXimage(t, data)

	testRunes := []rune{'A', 'B', 'Z', 'a', 'z', '0', '9', ' ', '!'}
	for _, r := range testRunes {
		ownGID := own.GlyphIndex(r)
		ximgGID := ximg.GlyphIndex(r)
		if ownGID != ximgGID {
			t.Errorf("GlyphIndex(%q U+%04X): own=%d, ximage=%d", r, r, ownGID, ximgGID)
		}
	}
}

// --- Hmtx / Advance ---

func TestHmtxAdvance(t *testing.T) {
	fonts := []struct {
		name string
		ppem float64
	}{
		{"tthint_subset.ttf", 24},
		{"ahem.ttf", 24},
		{"cousine_hint_subset.ttf", 16},
	}

	for _, tt := range fonts {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTestFontData(t, tt.name)
			own := parseWithOwn(t, data)
			ximg := parseWithXimage(t, data)

			gid := own.GlyphIndex('A')
			if gid == 0 {
				t.Skip("font does not contain 'A'")
			}

			ownAdv := own.GlyphAdvance(gid, tt.ppem)
			ximgAdv := ximg.GlyphAdvance(gid, tt.ppem)

			if math.Abs(ownAdv-ximgAdv) > 0.01 {
				t.Errorf("GlyphAdvance('A', ppem=%g): own=%g, ximage=%g, diff=%g",
					tt.ppem, ownAdv, ximgAdv, ownAdv-ximgAdv)
			}
		})
	}
}

func TestHmtxAdvance_KnownValues(t *testing.T) {
	// tthint_subset.ttf: upem=1040, 'A' advance = 880 font units (known).
	// At ppem=1040 (1:1), advance should be 880.0.
	data := loadTestFontData(t, "tthint_subset.ttf")
	own := parseWithOwn(t, data)

	gid := own.GlyphIndex('A')
	if gid == 0 {
		t.Fatal("tthint_subset: 'A' not found")
	}

	upem := float64(own.UnitsPerEm())
	adv := own.GlyphAdvance(gid, upem)

	// At ppem == upem, advance = advanceFU exactly.
	expected := 880.0
	if math.Abs(adv-expected) > 0.01 {
		t.Errorf("GlyphAdvance('A', ppem=upem=%g): got %g, want %g", upem, adv, expected)
	}
}

// --- Name ---

func TestNameParser(t *testing.T) {
	fonts := []struct {
		name string
	}{
		{"tthint_subset.ttf"},
		{"ahem.ttf"},
		{"cousine_hint_subset.ttf"},
	}

	for _, tt := range fonts {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTestFontData(t, tt.name)
			own := parseWithOwn(t, data)
			ximg := parseWithXimage(t, data)

			ownName := own.Name()
			ximgName := ximg.Name()

			if ownName != ximgName {
				t.Errorf("Name(): own=%q, ximage=%q", ownName, ximgName)
			}
			if ownName == "" {
				t.Error("Name(): returned empty string")
			}
		})
	}
}

func TestFullNameParser(t *testing.T) {
	fonts := []struct {
		name string
	}{
		{"cousine_hint_subset.ttf"},
	}

	for _, tt := range fonts {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTestFontData(t, tt.name)
			own := parseWithOwn(t, data)
			ximg := parseWithXimage(t, data)

			ownFull := own.FullName()
			ximgFull := ximg.FullName()

			if ownFull != ximgFull {
				t.Errorf("FullName(): own=%q, ximage=%q", ownFull, ximgFull)
			}
		})
	}
}

// --- Metrics (hhea + OS/2) ---

func TestFontMetrics(t *testing.T) {
	fonts := []struct {
		name string
		ppem float64
	}{
		{"tthint_subset.ttf", 24},
		{"ahem.ttf", 16},
		{"cousine_hint_subset.ttf", 12},
	}

	for _, tt := range fonts {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTestFontData(t, tt.name)
			own := parseWithOwn(t, data)

			ownM := own.Metrics(tt.ppem)

			// Our parser returns unhinted, linearly-scaled metrics.
			// ximage uses HintingFull which grid-fits values.
			// We verify our parser returns sane values independently.

			if ownM.Ascent <= 0 {
				t.Errorf("Ascent should be positive, got %g", ownM.Ascent)
			}
			// Descent from OS/2 sTypoDescender is negative (font spec).
			// face.go handles the sign flip: negative→positive for public Metrics.
			if ownM.Descent >= 0 {
				t.Errorf("Descent should be negative (OS/2 sTypoDescender), got %g", ownM.Descent)
			}
		})
	}
}

func TestFontMetrics_ScalingCorrectness(t *testing.T) {
	// Verify that metrics scale linearly with ppem.
	data := loadTestFontData(t, "tthint_subset.ttf")
	own := parseWithOwn(t, data)

	m12 := own.Metrics(12)
	m24 := own.Metrics(24)

	// At 2x ppem, all metrics should be exactly 2x (linear scaling).
	if math.Abs(m24.Ascent-m12.Ascent*2) > 0.001 {
		t.Errorf("Ascent not linear: @12=%g, @24=%g (want %g)",
			m12.Ascent, m24.Ascent, m12.Ascent*2)
	}
	if math.Abs(m24.Descent-m12.Descent*2) > 0.001 {
		t.Errorf("Descent not linear: @12=%g, @24=%g (want %g)",
			m12.Descent, m24.Descent, m12.Descent*2)
	}
}

func TestFontMetrics_FaceIntegration(t *testing.T) {
	// Verify that face.Metrics() produces correct results when using own parser.
	// face.go flips negative descent to positive.
	data := loadTestFontData(t, "tthint_subset.ttf")
	source, err := NewFontSource(data, WithParser("own"))
	if err != nil {
		t.Fatalf("NewFontSource failed: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(24)
	m := face.Metrics()

	if m.Ascent <= 0 {
		t.Errorf("Face Ascent should be positive, got %g", m.Ascent)
	}
	if m.Descent <= 0 {
		t.Errorf("Face Descent should be positive (absolute), got %g", m.Descent)
	}
	if m.LineHeight() <= 0 {
		t.Errorf("Face LineHeight should be positive, got %g", m.LineHeight())
	}
}

// --- UnitsPerEm and NumGlyphs ---

func TestUnitsPerEmAndNumGlyphs(t *testing.T) {
	fonts := []struct {
		name      string
		wantUpem  int
		wantGlyph int // minimum glyphs
	}{
		{"tthint_subset.ttf", 1040, 3},
		{"ahem.ttf", 1000, 2},
	}

	for _, tt := range fonts {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTestFontData(t, tt.name)
			own := parseWithOwn(t, data)
			ximg := parseWithXimage(t, data)

			if own.UnitsPerEm() != ximg.UnitsPerEm() {
				t.Errorf("UnitsPerEm: own=%d, ximage=%d", own.UnitsPerEm(), ximg.UnitsPerEm())
			}
			if own.UnitsPerEm() != tt.wantUpem {
				t.Errorf("UnitsPerEm: got %d, want %d", own.UnitsPerEm(), tt.wantUpem)
			}
			if own.NumGlyphs() != ximg.NumGlyphs() {
				t.Errorf("NumGlyphs: own=%d, ximage=%d", own.NumGlyphs(), ximg.NumGlyphs())
			}
			if own.NumGlyphs() < tt.wantGlyph {
				t.Errorf("NumGlyphs: got %d, want >= %d", own.NumGlyphs(), tt.wantGlyph)
			}
		})
	}
}

// --- GlyphBounds ---

func TestGlyphBounds(t *testing.T) {
	data := loadTestFontData(t, "tthint_subset.ttf")
	own := parseWithOwn(t, data)

	gid := own.GlyphIndex('A')
	if gid == 0 {
		t.Fatal("'A' not found in tthint_subset")
	}

	bounds := own.GlyphBounds(gid, 24)

	// Bounds should be non-zero for a visible glyph.
	if bounds.MaxX <= bounds.MinX {
		t.Errorf("GlyphBounds: zero width (MinX=%g, MaxX=%g)", bounds.MinX, bounds.MaxX)
	}
	if bounds.MaxY <= bounds.MinY {
		t.Errorf("GlyphBounds: zero height (MinY=%g, MaxY=%g)", bounds.MinY, bounds.MaxY)
	}
}

func TestGlyphBounds_EmptyGlyph(t *testing.T) {
	data := loadTestFontData(t, "cousine_hint_subset.ttf")
	own := parseWithOwn(t, data)

	// Space should have zero bounds.
	gid := own.GlyphIndex(' ')
	bounds := own.GlyphBounds(gid, 24)
	if bounds.MaxX != 0 || bounds.MaxY != 0 {
		t.Logf("space glyph bounds: %+v (may be non-zero in some fonts)", bounds)
	}
}

// --- RawFontDataProvider ---

func TestRawFontDataProvider(t *testing.T) {
	data := loadTestFontData(t, "tthint_subset.ttf")
	own := parseWithOwn(t, data)

	provider, ok := own.(RawFontDataProvider)
	if !ok {
		t.Fatal("ownParsedFont does not implement RawFontDataProvider")
	}

	raw := provider.RawFontData()
	if len(raw) == 0 {
		t.Fatal("RawFontData() returned empty slice")
	}
	if len(raw) != len(data) {
		t.Errorf("RawFontData() length: got %d, want %d", len(raw), len(data))
	}
}

// --- VariableAdvanceProvider ---

func TestVariableAdvanceProvider(t *testing.T) {
	data := loadTestFontData(t, "tthint_subset.ttf")
	own := parseWithOwn(t, data)

	_, ok := own.(VariableAdvanceProvider)
	if !ok {
		t.Fatal("ownParsedFont does not implement VariableAdvanceProvider")
	}
}

// --- TT Hint Cache ---

func TestOwnParsedFont_TTHintCache(t *testing.T) {
	data := loadTestFontData(t, "tthint_subset.ttf")
	parser := &ownParser{}
	parsed, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("own parser failed: %v", err)
	}

	ownFont, ok := parsed.(*ownParsedFont)
	if !ok {
		t.Fatal("expected *ownParsedFont")
	}

	// tthint_subset has TT instructions — cache should be non-nil.
	cache := ownFont.loadTTHintCache()
	if cache == nil {
		t.Fatal("expected non-nil TT hint cache for tthint_subset.ttf")
	}

	// Hinted advance should be available for GID 1 ('A') at ppem 24.
	adv, ok := cache.hintedAdvanceWidth(1, 24)
	if !ok {
		t.Error("hintedAdvanceWidth returned false for GID 1 at ppem 24")
	}
	if adv <= 0 {
		t.Errorf("hintedAdvanceWidth: got %g, want positive", adv)
	}
}

// --- Parser Registration ---

func TestOwnParserRegistered(t *testing.T) {
	parser := getParser("own")
	if parser == nil {
		t.Fatal("'own' parser not registered")
	}

	data := loadTestFontData(t, "cousine_hint_subset.ttf")
	parsed, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("own parser.Parse failed: %v", err)
	}

	if parsed.Name() == "" {
		t.Error("parsed font has empty name")
	}
}

func TestOwnParserViaFontSource(t *testing.T) {
	// Use tthint_subset which is known to contain 'A'.
	data := loadTestFontData(t, "tthint_subset.ttf")
	source, err := NewFontSource(data, WithParser("own"))
	if err != nil {
		t.Fatalf("NewFontSource with own parser failed: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	if source.Name() == "" {
		t.Error("FontSource name is empty")
	}

	// Create a face and test basic functionality.
	face := source.Face(24)
	m := face.Metrics()
	if m.Ascent <= 0 {
		t.Errorf("Ascent should be positive, got %g", m.Ascent)
	}

	advance := face.Advance("A")
	if advance <= 0 {
		t.Errorf("Advance('A') should be positive, got %g", advance)
	}

	if !face.HasGlyph('A') {
		t.Error("HasGlyph('A') should be true")
	}
}

func TestOwnParserViaFontSource_Cousine(t *testing.T) {
	data := loadTestFontData(t, "cousine_hint_subset.ttf")
	source, err := NewFontSource(data, WithParser("own"))
	if err != nil {
		t.Fatalf("NewFontSource with own parser failed: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	if source.Name() == "" {
		t.Error("FontSource name is empty")
	}

	face := source.Face(16)
	m := face.Metrics()
	if m.Ascent <= 0 {
		t.Errorf("Ascent should be positive, got %g", m.Ascent)
	}
}

// --- Cross-validation: own vs ximage for all methods ---

func TestCrossValidation(t *testing.T) {
	fonts := []string{
		"tthint_subset.ttf",
		"ahem.ttf",
		"cousine_hint_subset.ttf",
	}

	for _, fontName := range fonts {
		t.Run(fontName, func(t *testing.T) {
			data := loadTestFontData(t, fontName)
			own := parseWithOwn(t, data)
			ximg := parseWithXimage(t, data)

			// UnitsPerEm — must match exactly.
			if own.UnitsPerEm() != ximg.UnitsPerEm() {
				t.Errorf("UnitsPerEm: own=%d, ximage=%d",
					own.UnitsPerEm(), ximg.UnitsPerEm())
			}

			// NumGlyphs — must match exactly.
			if own.NumGlyphs() != ximg.NumGlyphs() {
				t.Errorf("NumGlyphs: own=%d, ximage=%d",
					own.NumGlyphs(), ximg.NumGlyphs())
			}

			// Name — must match exactly.
			if own.Name() != ximg.Name() {
				t.Errorf("Name: own=%q, ximage=%q", own.Name(), ximg.Name())
			}

			// GlyphIndex for ASCII printable range.
			for r := rune(0x20); r <= 0x7E; r++ {
				ownGID := own.GlyphIndex(r)
				ximgGID := ximg.GlyphIndex(r)
				if ownGID != ximgGID {
					t.Errorf("GlyphIndex(%q U+%04X): own=%d, ximage=%d",
						r, r, ownGID, ximgGID)
				}
			}

			// GlyphAdvance for 'A' at multiple sizes.
			gidA := own.GlyphIndex('A')
			if gidA != 0 {
				for _, ppem := range []float64{12, 16, 24, 48} {
					ownAdv := own.GlyphAdvance(gidA, ppem)
					ximgAdv := ximg.GlyphAdvance(gidA, ppem)
					if math.Abs(ownAdv-ximgAdv) > 0.01 {
						t.Errorf("GlyphAdvance(GID=%d, ppem=%g): own=%g, ximage=%g",
							gidA, ppem, ownAdv, ximgAdv)
					}
				}
			}
		})
	}
}

// --- Edge Cases ---

func TestOwnParser_EmptyData(t *testing.T) {
	parser := &ownParser{}
	_, err := parser.Parse([]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestOwnParser_InvalidData(t *testing.T) {
	parser := &ownParser{}
	_, err := parser.Parse([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0})
	if err == nil {
		t.Error("expected error for invalid data")
	}
}

func TestCmapGlyphIndex_BeyondBMP(t *testing.T) {
	// Cross-validate Hebrew font between own and ximage parsers.
	data := loadTestFontData(t, "notoserifhebrew_autohint_metrics.ttf")
	own := parseWithOwn(t, data)
	ximg := parseWithXimage(t, data)

	// Hebrew character — check both parsers agree.
	ownGID := own.GlyphIndex('\u05D0') // Alef
	ximgGID := ximg.GlyphIndex('\u05D0')
	if ownGID != ximgGID {
		t.Errorf("GlyphIndex(Alef): own=%d, ximage=%d", ownGID, ximgGID)
	}
	if ownGID == 0 {
		t.Log("Hebrew Alef not found in NotoSerifHebrew (might be subset)")
	}

	// Cross-validate Name().
	if own.Name() != ximg.Name() {
		t.Errorf("Name(): own=%q, ximage=%q", own.Name(), ximg.Name())
	}

	// Cross-validate NumGlyphs and UnitsPerEm.
	if own.NumGlyphs() != ximg.NumGlyphs() {
		t.Errorf("NumGlyphs: own=%d, ximage=%d", own.NumGlyphs(), ximg.NumGlyphs())
	}
	if own.UnitsPerEm() != ximg.UnitsPerEm() {
		t.Errorf("UnitsPerEm: own=%d, ximage=%d", own.UnitsPerEm(), ximg.UnitsPerEm())
	}
}

func TestCmapGlyphIndex_NotoSerifShaping(t *testing.T) {
	// Cross-validate NotoSerif shaping font (Latin script).
	data := loadTestFontData(t, "notoserif_autohint_shaping.ttf")
	own := parseWithOwn(t, data)
	ximg := parseWithXimage(t, data)

	// Test several Latin characters for agreement.
	for _, r := range []rune{'A', 'a', 'B', 'Z', '0', ' '} {
		ownGID := own.GlyphIndex(r)
		ximgGID := ximg.GlyphIndex(r)
		if ownGID != ximgGID {
			t.Errorf("GlyphIndex(%q U+%04X): own=%d, ximage=%d", r, r, ownGID, ximgGID)
		}
	}

	// Cross-validate advance for a known glyph.
	gid := own.GlyphIndex('A')
	if gid != 0 {
		ownAdv := own.GlyphAdvance(gid, 24)
		ximgAdv := ximg.GlyphAdvance(gid, 24)
		if math.Abs(ownAdv-ximgAdv) > 0.01 {
			t.Errorf("GlyphAdvance('A', 24): own=%g, ximage=%g", ownAdv, ximgAdv)
		}
	}
}
