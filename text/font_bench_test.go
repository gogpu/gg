// Font stack performance benchmarks — Pure Go font pipeline.
//
// Covers the full font pipeline from parsing through shaping:
//   - Font parsing: own parser vs x/image parser
//   - Cmap lookup: GlyphIndex for single rune and ASCII range
//   - Advance width: hmtx-based GlyphAdvance
//   - Font metrics: Metrics(ppem)
//   - Shaping: OwnShaper vs GoTextShaper (short/sentence/paragraph)
//   - TT bytecode hinting: instance creation, glyph hinting, cache
//   - Auto-hinter: outline hinting pipeline
//   - gvar interpolation: variable font delta computation
//   - Full pipeline: parse → shape (end-to-end)
//
// Run:
//
//	GOWORK=off go test -bench "BenchmarkFont" -benchmem -count 3 ./text/ -timeout 5m
package text

import (
	"encoding/binary"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// ---------------------------------------------------------------------------
// Setup helpers
// ---------------------------------------------------------------------------

// benchOwnParsed parses Go Regular with the own parser and returns the
// ParsedFont. Fails the benchmark on error.
func benchOwnParsed(b *testing.B) *ownParsedFont {
	b.Helper()
	p := &ownParser{}
	pf, err := p.Parse(goregular.TTF)
	if err != nil {
		b.Fatalf("own parser: %v", err)
	}
	return pf.(*ownParsedFont)
}

// benchXimageParsed parses Go Regular with the x/image parser and returns
// the ParsedFont. Fails the benchmark on error.
func benchXimageParsed(b *testing.B) *ximageParsedFont {
	b.Helper()
	p := &ximageParser{}
	pf, err := p.Parse(goregular.TTF)
	if err != nil {
		b.Fatalf("ximage parser: %v", err)
	}
	return pf.(*ximageParsedFont)
}

// benchOwnFace returns a Face at the given size using the own parser.
func benchOwnFace(b *testing.B, size float64) (Face, *FontSource) {
	b.Helper()
	source, err := NewFontSource(goregular.TTF, WithParser("own"))
	if err != nil {
		b.Fatalf("own FontSource: %v", err)
	}
	face := source.Face(size)
	return face, source
}

// benchXimageFace returns a Face at the given size using the ximage parser.
func benchXimageFace(b *testing.B, size float64) (Face, *FontSource) {
	b.Helper()
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("ximage FontSource: %v", err)
	}
	face := source.Face(size)
	return face, source
}

// longParagraph is a ~120 character string for paragraph-level shaping.
const longParagraph = "The quick brown fox jumps over the lazy dog. " +
	"Pack my box with five dozen liquor jugs. " +
	"How vexingly quick daft zebras jump!"

// ---------------------------------------------------------------------------
// 1. Font Parsing
// ---------------------------------------------------------------------------

func BenchmarkFontParse_Own(b *testing.B) {
	b.ReportAllocs()
	p := &ownParser{}
	b.ResetTimer()
	for range b.N {
		pf, err := p.Parse(goregular.TTF)
		if err != nil {
			b.Fatal(err)
		}
		_ = pf
	}
}

func BenchmarkFontParse_Ximage(b *testing.B) {
	b.ReportAllocs()
	p := &ximageParser{}
	b.ResetTimer()
	for range b.N {
		pf, err := p.Parse(goregular.TTF)
		if err != nil {
			b.Fatal(err)
		}
		_ = pf
	}
}

// ---------------------------------------------------------------------------
// 2. Cmap Lookup (GlyphIndex)
// ---------------------------------------------------------------------------

func BenchmarkFontCmap_GlyphIndex_Own(b *testing.B) {
	pf := benchOwnParsed(b)
	// Warm up lazy cmap.
	_ = pf.GlyphIndex('A')

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = pf.GlyphIndex('A')
	}
}

func BenchmarkFontCmap_GlyphIndex_Ximage(b *testing.B) {
	pf := benchXimageParsed(b)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = pf.GlyphIndex('A')
	}
}

func BenchmarkFontCmap_ASCII_Own(b *testing.B) {
	pf := benchOwnParsed(b)
	// Warm up lazy cmap.
	_ = pf.GlyphIndex(' ')

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for r := rune(0x20); r <= 0x7E; r++ {
			_ = pf.GlyphIndex(r)
		}
	}
}

func BenchmarkFontCmap_ASCII_Ximage(b *testing.B) {
	pf := benchXimageParsed(b)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for r := rune(0x20); r <= 0x7E; r++ {
			_ = pf.GlyphIndex(r)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Advance Width (hmtx)
// ---------------------------------------------------------------------------

func BenchmarkFontHmtx_GlyphAdvance_Own(b *testing.B) {
	pf := benchOwnParsed(b)
	gid := pf.GlyphIndex('A')

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = pf.GlyphAdvance(gid, 16.0)
	}
}

func BenchmarkFontHmtx_GlyphAdvance_Ximage(b *testing.B) {
	pf := benchXimageParsed(b)
	gid := pf.GlyphIndex('A')

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = pf.GlyphAdvance(gid, 16.0)
	}
}

// ---------------------------------------------------------------------------
// 4. Font Metrics
// ---------------------------------------------------------------------------

func BenchmarkFontMetrics_Own(b *testing.B) {
	pf := benchOwnParsed(b)
	// Warm up lazy metrics.
	_ = pf.Metrics(16.0)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = pf.Metrics(16.0)
	}
}

func BenchmarkFontMetrics_Ximage(b *testing.B) {
	pf := benchXimageParsed(b)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = pf.Metrics(16.0)
	}
}

// ---------------------------------------------------------------------------
// 5. Shaping: OwnShaper vs GoTextShaper
// ---------------------------------------------------------------------------

func BenchmarkFontShape_Own(b *testing.B) {
	face, source := benchOwnFace(b, 16.0)
	defer func() { _ = source.Close() }()

	shaper := NewOwnShaper()
	// Warm up cache.
	_ = shaper.Shape("A", face)

	texts := []struct {
		name string
		text string
	}{
		{"Short_5", "Hello"},
		{"Sentence_19", "The quick brown fox"},
		{"Paragraph_120", longParagraph},
	}

	for _, tc := range texts {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = shaper.Shape(tc.text, face)
			}
		})
	}
}

func BenchmarkFontShape_GoText(b *testing.B) {
	face, source := benchXimageFace(b, 16.0)
	defer func() { _ = source.Close() }()

	shaper := NewGoTextShaper()
	// Warm up.
	_ = shaper.Shape("A", face)

	texts := []struct {
		name string
		text string
	}{
		{"Short_5", "Hello"},
		{"Sentence_19", "The quick brown fox"},
		{"Paragraph_120", longParagraph},
	}

	for _, tc := range texts {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = shaper.Shape(tc.text, face)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. TT Bytecode Interpreter
// ---------------------------------------------------------------------------

func BenchmarkFontTTHint_GlyphOutline(b *testing.B) {
	data, err := os.ReadFile("testdata/tthint_subset.ttf")
	if err != nil {
		b.Skipf("test font not available: %v", err)
	}

	cache := newTTHintCache(data)
	if cache == nil {
		b.Skip("font has no TT instructions")
	}

	// Parse to get glyph ID for 'A'.
	p := &ownParser{}
	pf, err := p.Parse(data)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	gid := pf.GlyphIndex('A')
	if gid == 0 {
		b.Fatal("glyph 'A' not found in test font")
	}

	// Warm up the instance cache at ppem 16.
	_, _ = cache.hintGlyphOutline(gid, 16)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = cache.hintGlyphOutline(gid, 16)
	}
}

func BenchmarkFontTTHint_MultiSize(b *testing.B) {
	data, err := os.ReadFile("testdata/tthint_subset.ttf")
	if err != nil {
		b.Skipf("test font not available: %v", err)
	}

	cache := newTTHintCache(data)
	if cache == nil {
		b.Skip("font has no TT instructions")
	}

	p := &ownParser{}
	pf, err := p.Parse(data)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	gid := pf.GlyphIndex('A')
	if gid == 0 {
		b.Fatal("glyph 'A' not found")
	}

	sizes := []int32{8, 12, 16, 24, 48}

	// Warm up all sizes.
	for _, ppem := range sizes {
		_, _ = cache.hintGlyphOutline(gid, ppem)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, ppem := range sizes {
			_, _ = cache.hintGlyphOutline(gid, ppem)
		}
	}
}

func BenchmarkFontTTHint_InstanceCreation(b *testing.B) {
	data, err := os.ReadFile("testdata/tthint_subset.ttf")
	if err != nil {
		b.Skipf("test font not available: %v", err)
	}

	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		b.Skip("no TT font program")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		inst, err := newTTHintInstance(fp, 16, ttTargetSmooth)
		if err != nil {
			b.Fatal(err)
		}
		_ = inst
	}
}

func BenchmarkFontTTHint_CacheHitPath(b *testing.B) {
	data, err := os.ReadFile("testdata/tthint_subset.ttf")
	if err != nil {
		b.Skipf("test font not available: %v", err)
	}

	cache := newTTHintCache(data)
	if cache == nil {
		b.Skip("font has no TT instructions")
	}

	// Warm up ppem=16 instance.
	_, _ = cache.getInstance(16)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = cache.getInstance(16)
	}
}

// ---------------------------------------------------------------------------
// 7. Auto-Hinter
// ---------------------------------------------------------------------------

func BenchmarkFontAutoHint_Glyph(b *testing.B) {
	p := &ownParser{}
	pf, err := p.Parse(goregular.TTF)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}

	gid := pf.GlyphIndex('H')
	if gid == 0 {
		b.Fatal("glyph 'H' not found")
	}

	// Extract outline once.
	ext := NewOutlineExtractor()
	outline, err := ext.ExtractOutlineHinted(pf, GlyphID(gid), 16.0, HintingNone)
	if err != nil || outline == nil {
		b.Fatalf("no outline for 'H': %v", err)
	}

	// Warm up auto-hint metrics cache.
	_ = autoHintOutline(outline, pf, 16.0, HintingFull)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		// Clone outline since autoHintOutline modifies it in-place.
		o := cloneOutline(outline)
		_ = autoHintOutline(o, pf, 16.0, HintingFull)
	}
}

// cloneOutline creates a deep copy of a GlyphOutline for benchmark reuse.
func cloneOutline(src *GlyphOutline) *GlyphOutline {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Segments = make([]OutlineSegment, len(src.Segments))
	copy(dst.Segments, src.Segments)
	return &dst
}

// ---------------------------------------------------------------------------
// 8. gvar Interpolation (variable font)
// ---------------------------------------------------------------------------

func BenchmarkFontGvar_ParseTable(b *testing.B) {
	data, err := os.ReadFile("testdata/vazirmatn_var_trimmed.ttf")
	if err != nil {
		b.Skipf("variable font not available: %v", err)
	}

	tables, err := parseFontTablesIndex(data, 0)
	if err != nil {
		b.Fatalf("parse tables: %v", err)
	}

	gvarData, ok := tables["gvar"]
	if !ok {
		b.Skip("font has no gvar table")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		gv, err := parseGvar(gvarData)
		if err != nil {
			b.Fatal(err)
		}
		_ = gv
	}
}

func BenchmarkFontGvar_VariationDeltas(b *testing.B) {
	data, err := os.ReadFile("testdata/vazirmatn_var_trimmed.ttf")
	if err != nil {
		b.Skipf("variable font not available: %v", err)
	}

	tables, err := parseFontTablesIndex(data, 0)
	if err != nil {
		b.Fatalf("parse tables: %v", err)
	}

	gvarData, ok := tables["gvar"]
	if !ok {
		b.Skip("font has no gvar table")
	}

	gvar, err := parseGvar(gvarData)
	if err != nil {
		b.Fatalf("parseGvar: %v", err)
	}

	// We need a glyph with variation data. Find the first non-empty glyph.
	var testGID uint16
	for i := range len(gvar.glyphOffsets) - 1 {
		if gvar.glyphOffsets[i+1] > gvar.glyphOffsets[i] {
			testGID = uint16(i)
			break
		}
	}

	// Parse glyf for contour data.
	glyfData, ok := tables["glyf"]
	locaData, locaOK := tables["loca"]
	headData, headOK := tables["head"]
	if !ok || !locaOK || !headOK || len(headData) < 54 {
		b.Skip("missing glyf/loca/head tables")
	}
	isLongLoca := binary.BigEndian.Uint16(headData[50:52]) != 0

	off, length := locateGlyph(locaData, int(testGID), isLongLoca)
	if length == 0 {
		b.Skip("test glyph has no outline")
	}
	end := off + length
	if end > len(glyfData) {
		b.Skip("glyph data out of bounds")
	}

	gd := glyfData[off:end]
	if len(gd) < 10 {
		b.Skip("glyph data too short")
	}

	numContours := int(int16(binary.BigEndian.Uint16(gd[0:2])))
	if numContours <= 0 {
		b.Skip("composite or empty glyph")
	}

	// Parse contour endpoints and points (simplified: use contour count
	// to estimate point count).
	contourEndsStart := 10
	contourEndsEnd := contourEndsStart + numContours*2
	if contourEndsEnd > len(gd) {
		b.Skip("contour endpoints truncated")
	}

	contourEnds := make([]uint16, numContours)
	for i := range numContours {
		contourEnds[i] = binary.BigEndian.Uint16(gd[contourEndsStart+i*2 : contourEndsStart+i*2+2])
	}

	numPoints := int(contourEnds[numContours-1]) + 1

	// Build placeholder outline points (actual coordinates not needed for
	// delta computation — the gvar decoder applies deltas independently).
	outlinePoints := make([][2]int32, numPoints)

	// Normalized coordinates: wght axis at 1.0 (F2.14 = 0x4000).
	coords := []int16{0x4000}

	// Warm up.
	_, _ = gvar.glyphVariationDeltas(testGID, coords, numPoints, contourEnds, outlinePoints)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = gvar.glyphVariationDeltas(testGID, coords, numPoints, contourEnds, outlinePoints)
	}
}

// ---------------------------------------------------------------------------
// 9. Full Pipeline
// ---------------------------------------------------------------------------

func BenchmarkFontFullPipeline_ParseAndShape(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		// Parse.
		source, err := NewFontSource(goregular.TTF, WithParser("own"))
		if err != nil {
			b.Fatal(err)
		}
		face := source.Face(16.0)

		// Shape.
		shaper := NewOwnShaper()
		result := shaper.Shape("Hello, World!", face)
		_ = result

		_ = source.Close()
	}
}

func BenchmarkFontFullPipeline_ShapeOnly(b *testing.B) {
	// Parse once, shape many times.
	source, err := NewFontSource(goregular.TTF, WithParser("own"))
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16.0)
	shaper := NewOwnShaper()
	// Warm up.
	_ = shaper.Shape("A", face)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = shaper.Shape("Hello, World!", face)
	}
}

// ---------------------------------------------------------------------------
// 10. Glyph Bounds
// ---------------------------------------------------------------------------

func BenchmarkFontGlyphBounds_Own(b *testing.B) {
	pf := benchOwnParsed(b)
	gid := pf.GlyphIndex('A')

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = pf.GlyphBounds(gid, 16.0)
	}
}

func BenchmarkFontGlyphBounds_Ximage(b *testing.B) {
	pf := benchXimageParsed(b)
	gid := pf.GlyphIndex('A')

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = pf.GlyphBounds(gid, 16.0)
	}
}

// ---------------------------------------------------------------------------
// 11. Name Table
// ---------------------------------------------------------------------------

func BenchmarkFontName_Own(b *testing.B) {
	pf := benchOwnParsed(b)
	// Warm up lazy name parsing.
	_ = pf.Name()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = pf.Name()
	}
}

func BenchmarkFontName_Ximage(b *testing.B) {
	pf := benchXimageParsed(b)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = pf.Name()
	}
}

// ---------------------------------------------------------------------------
// 12. Outline Extraction
// ---------------------------------------------------------------------------

func BenchmarkFontOutlineExtract(b *testing.B) {
	pf := benchOwnParsed(b)
	gid := GlyphID(pf.GlyphIndex('A'))

	ext := NewOutlineExtractor()
	// Warm up.
	_, _ = ext.ExtractOutlineHinted(pf, gid, 16.0, HintingNone)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = ext.ExtractOutlineHinted(pf, gid, 16.0, HintingNone)
	}
}

// ---------------------------------------------------------------------------
// 13. ParseGlyfContours (raw contour extraction)
// ---------------------------------------------------------------------------

func BenchmarkFontParseGlyfContours(b *testing.B) {
	pf := benchOwnParsed(b)
	gid := GlyphID(pf.GlyphIndex('H'))

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, err := ParseGlyfContours(goregular.TTF, gid)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Ensure binary import is used (for gvar benchmark).
var _ = binary.BigEndian
