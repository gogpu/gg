// Own font parser — Pure Go implementation of ParsedFont.
//
// ownParsedFont replaces ximageParsedFont with direct binary parsing
// of TrueType/OpenType tables. Zero external dependencies for font parsing.
//
// Implements:
//   - ParsedFont              (core interface)
//   - VariableAdvanceProvider  (HVAR-based variable font advance)
//   - RawFontDataProvider      (raw bytes for auto-hinter and TT interpreter)
//
// Lazy initialization: table directory is parsed eagerly (cheap), individual
// tables are parsed on first access via sync.Once (thread-safe).
//
// This file is part of Phase 3a (ADR-048: Pure Go Font Stack).
package text

import (
	"encoding/binary"
	"fmt"
	"sync"
)

// ownParser implements FontParser using pure Go binary parsing.
type ownParser struct{}

// Parse implements FontParser.Parse.
func (p *ownParser) Parse(data []byte) (ParsedFont, error) {
	return p.ParseIndex(data, 0)
}

// ParseIndex parses a font at the given index within a collection.
// For single fonts (.ttf/.otf), index is ignored. For collections
// (.ttc/.otc), index selects which font to use (0 = first).
func (p *ownParser) ParseIndex(data []byte, index int) (ParsedFont, error) {
	tables, err := parseFontTablesIndex(data, index)
	if err != nil {
		return nil, fmt.Errorf("text: own parser: %w", err)
	}

	// Parse head table eagerly — upem is required by many methods.
	headData, ok := tables["head"]
	if !ok {
		return nil, fmt.Errorf("text: own parser: missing head table")
	}
	upem, err := parseHeadUnitsPerEm(headData)
	if err != nil {
		return nil, fmt.Errorf("text: own parser: %w", err)
	}

	// Parse maxp numGlyphs eagerly.
	maxpData, ok := tables["maxp"]
	if !ok {
		return nil, fmt.Errorf("text: own parser: missing maxp table")
	}
	if len(maxpData) < 6 {
		return nil, fmt.Errorf("text: own parser: maxp table too short")
	}
	numGlyphs := int(binary.BigEndian.Uint16(maxpData[4:6]))

	font := &ownParsedFont{
		rawData:   data,
		tables:    tables,
		upem:      upem,
		numGlyphs: numGlyphs,
	}

	return font, nil
}

// ownParsedFont implements ParsedFont with direct binary parsing.
type ownParsedFont struct {
	rawData []byte            // raw font file bytes
	tables  map[string][]byte // raw table data keyed by tag

	// Eagerly parsed (cheap).
	upem      int
	numGlyphs int

	// Lazily parsed via sync.Once.

	cmapOnce sync.Once
	cmap     *cmapLookup // nil if cmap parsing failed

	hmtxOnce    sync.Once
	hmtxAdv     []uint16 // advance widths from hmtx
	hmtxLSB     []int16  // left side bearings from hmtx (unused currently, kept for GlyphBounds)
	numHMetrics int      // from hhea.numberOfHMetrics
	hmtxParsed  bool     // true if hmtx was parsed successfully

	nameOnce   sync.Once
	familyName string
	fullName   string

	metricsOnce sync.Once
	hhea        hheaMetrics // from hhea table
	os2         os2Metrics  // from OS/2 table
	hheaOK      bool        // true if hhea was parsed successfully
	os2OK       bool        // true if OS/2 was parsed successfully

	// fvar axes — parsed independently, needed by both HVAR and gvar.
	fvarOnce sync.Once
	fvarAxes []fvarAxis // parsed fvar axes for coordinate normalization

	// HVAR (reuse existing parser from hvar.go).
	hvarOnce sync.Once
	hvar     *hvarTable // nil if HVAR not present or failed to parse

	// TT hint cache — lazy loading (thread-safe).
	// Provides cached fpgm/prep execution results per ppem.
	ttHintOnce  sync.Once
	ttHintCache *ttHintCache // nil if font has no TT instructions

	// gvar/avar — lazy loading (thread-safe).
	// Provides variable font outline interpolation.
	gvarOnce sync.Once
	gvar     *gvarTable // nil if gvar not present or failed to parse
	avarOnce sync.Once
	avar     *avarTable // nil if avar not present
}

// --- ParsedFont interface ---

// Name implements ParsedFont.Name.
func (f *ownParsedFont) Name() string {
	f.ensureName()
	return f.familyName
}

// FullName implements ParsedFont.FullName.
func (f *ownParsedFont) FullName() string {
	f.ensureName()
	return f.fullName
}

// NumGlyphs implements ParsedFont.NumGlyphs.
func (f *ownParsedFont) NumGlyphs() int {
	return f.numGlyphs
}

// UnitsPerEm implements ParsedFont.UnitsPerEm.
func (f *ownParsedFont) UnitsPerEm() int {
	return f.upem
}

// GlyphIndex implements ParsedFont.GlyphIndex.
func (f *ownParsedFont) GlyphIndex(r rune) uint16 {
	f.ensureCmap()
	return f.cmap.glyphIndex(r)
}

// GlyphAdvance implements ParsedFont.GlyphAdvance.
// Returns the advance width in pixels: advanceFU * ppem / upem.
func (f *ownParsedFont) GlyphAdvance(glyphIndex uint16, ppem float64) float64 {
	f.ensureHmtx()
	if !f.hmtxParsed || f.upem == 0 {
		return 0
	}
	advFU := hmtxAdvance(f.hmtxAdv, f.numHMetrics, glyphIndex)
	return float64(advFU) * ppem / float64(f.upem)
}

// GlyphBounds implements ParsedFont.GlyphBounds.
// Returns the glyph bounding box scaled from font units to pixels.
//
// Uses the glyf table for TrueType outlines. CFF fonts are not yet
// supported by the own parser (returns zero rect).
func (f *ownParsedFont) GlyphBounds(glyphIndex uint16, ppem float64) Rect {
	glyfData, ok := f.tables["glyf"]
	if !ok || f.upem == 0 {
		return Rect{}
	}

	// Need loca to find the glyph offset.
	locaData, ok := f.tables["loca"]
	if !ok {
		return Rect{}
	}
	headData, ok := f.tables["head"]
	if !ok || len(headData) < 54 {
		return Rect{}
	}
	isLongLoca := binary.BigEndian.Uint16(headData[50:52]) != 0

	off, length := locateGlyph(locaData, int(glyphIndex), isLongLoca)
	if length == 0 {
		return Rect{} // empty glyph (space, etc.)
	}
	end := off + length
	if end > len(glyfData) {
		return Rect{}
	}

	data := glyfData[off:end]
	if len(data) < 10 {
		return Rect{}
	}

	// Glyph header:
	//   int16 numContours
	//   int16 xMin
	//   int16 yMin
	//   int16 xMax
	//   int16 yMax
	xMin := int16(binary.BigEndian.Uint16(data[2:4]))
	yMin := int16(binary.BigEndian.Uint16(data[4:6]))
	xMax := int16(binary.BigEndian.Uint16(data[6:8]))
	yMax := int16(binary.BigEndian.Uint16(data[8:10]))

	// The glyf header stores coordinates in Y-UP (font units).
	// sfnt.GlyphBounds returns Y-DOWN (Go image convention) by negating Y.
	// To match: negate Y and swap MinY/MaxY.
	scale := ppem / float64(f.upem)
	return Rect{
		MinX: float64(xMin) * scale,
		MinY: float64(-yMax) * scale, // Y-UP → Y-DOWN: negate and swap
		MaxX: float64(xMax) * scale,
		MaxY: float64(-yMin) * scale, // Y-UP → Y-DOWN: negate and swap
	}
}

// Metrics implements ParsedFont.Metrics.
func (f *ownParsedFont) Metrics(ppem float64) FontMetrics {
	f.ensureMetrics()
	if !f.hheaOK && !f.os2OK {
		return FontMetrics{}
	}
	return computeFontMetrics(f.hhea, f.os2, f.upem, ppem)
}

// --- RawFontDataProvider ---

// RawFontData implements RawFontDataProvider, returning the raw font file
// bytes. This enables the contour-based auto-hinter and TT bytecode
// interpreter paths.
func (f *ownParsedFont) RawFontData() []byte {
	return f.rawData
}

// --- VariableAdvanceProvider ---

// GlyphAdvanceVar implements VariableAdvanceProvider.
// Returns the advance width in pixels adjusted by HVAR deltas for the
// given font variations.
func (f *ownParsedFont) GlyphAdvanceVar(glyphIndex uint16, ppem float64, variations []FontVariation) float64 {
	f.loadHVAR()

	// Get base advance.
	baseAdvance := f.GlyphAdvance(glyphIndex, ppem)

	if f.hvar == nil || len(f.fvarAxes) == 0 || len(variations) == 0 {
		return baseAdvance
	}

	// Normalize variation coordinates.
	coords := normalizeCoords(f.fvarAxes, variations)

	// Get HVAR delta (in font units).
	delta := f.hvar.advanceDelta(glyphIndex, coords)
	if delta == 0 {
		return baseAdvance
	}

	// Scale delta from font units to pixels.
	if f.upem == 0 {
		return baseAdvance
	}
	scaledDelta := float64(delta) * ppem / float64(f.upem)
	return baseAdvance + scaledDelta
}

// --- Lazy initialization helpers ---

// ensureCmap lazily parses the cmap table.
func (f *ownParsedFont) ensureCmap() {
	f.cmapOnce.Do(func() {
		cmapData, ok := f.tables["cmap"]
		if !ok {
			return
		}
		f.cmap = parseCmapTable(cmapData)
	})
}

// ensureHmtx lazily parses the hhea and hmtx tables for advance widths.
func (f *ownParsedFont) ensureHmtx() {
	f.hmtxOnce.Do(func() {
		hheaData, ok := f.tables["hhea"]
		if !ok {
			return
		}
		hhea, ok := parseHheaTable(hheaData)
		if !ok || hhea.numberOfHMetrics == 0 {
			return
		}

		hmtxData, ok := f.tables["hmtx"]
		if !ok {
			return
		}

		advances, lsbs, err := parseHmtx(hmtxData, hhea.numberOfHMetrics, f.numGlyphs)
		if err != nil {
			return
		}

		f.hmtxAdv = advances
		f.hmtxLSB = lsbs
		f.numHMetrics = hhea.numberOfHMetrics
		f.hmtxParsed = true
	})
}

// ensureName lazily parses the name table.
func (f *ownParsedFont) ensureName() {
	f.nameOnce.Do(func() {
		nameData, ok := f.tables["name"]
		if !ok {
			return
		}
		f.familyName, f.fullName = parseNameTable(nameData)
	})
}

// ensureMetrics lazily parses hhea and OS/2 tables for font-level metrics.
func (f *ownParsedFont) ensureMetrics() {
	f.metricsOnce.Do(func() {
		if hheaData, ok := f.tables["hhea"]; ok {
			f.hhea, f.hheaOK = parseHheaTable(hheaData)
		}
		if os2Data, ok := f.tables["OS/2"]; ok {
			f.os2, f.os2OK = parseOS2Table(os2Data)
		}
	})
}

// loadTTHintCache lazily initializes the TT bytecode hint cache.
// Thread-safe via sync.Once. Returns nil if the font has no TT instructions.
func (f *ownParsedFont) loadTTHintCache() *ttHintCache {
	f.ttHintOnce.Do(func() {
		if f.rawData == nil {
			return
		}
		f.ttHintCache = newTTHintCache(f.rawData)
	})
	return f.ttHintCache
}

// loadFvar lazily parses the fvar table to extract axis definitions.
// fvar axes are needed by both HVAR (advance deltas) and gvar (outline deltas),
// so they are parsed independently from either table.
//
// A font with gvar but no HVAR (e.g., Apple SFNS.ttf) still needs fvarAxes
// for normalizeCoords to produce the correct-length coordinate array.
func (f *ownParsedFont) loadFvar() {
	f.fvarOnce.Do(func() {
		fvarRaw, ok := f.tables["fvar"]
		if !ok {
			return
		}
		f.fvarAxes = parseFvarAxes(fvarRaw)
	})
}

// loadHVAR lazily parses the HVAR table.
// Ensures fvar axes are also parsed (needed for coordinate normalization).
func (f *ownParsedFont) loadHVAR() {
	f.loadFvar()
	f.hvarOnce.Do(func() {
		hvarRaw, ok := f.tables["HVAR"]
		if !ok {
			return
		}
		hvar, err := parseHVAR(hvarRaw)
		if err != nil {
			return
		}
		f.hvar = hvar
	})
}

// loadGvar lazily parses the gvar table.
func (f *ownParsedFont) loadGvar() {
	f.gvarOnce.Do(func() {
		gvarRaw, ok := f.tables["gvar"]
		if !ok {
			return
		}
		gvar, err := parseGvar(gvarRaw)
		if err != nil {
			return
		}
		f.gvar = gvar
	})
}

// loadAvar lazily parses the avar table.
func (f *ownParsedFont) loadAvar() {
	f.avarOnce.Do(func() {
		avarRaw, ok := f.tables["avar"]
		if !ok {
			return
		}
		f.avar = parseAvar(avarRaw)
	})
}

// applyVariations computes gvar deltas and applies them to the given
// outline points. Points are modified in-place.
//
// Parameters:
//   - glyphID: the glyph to look up in gvar
//   - points: outline points as [x, y] pairs (modified in-place)
//   - contourEnds: end-of-contour point indices
//   - variations: user-space variation settings (e.g., wght=700)
//
// The function normalizes coordinates, applies avar remapping, then
// computes gvar deltas and adds them to the points.
func (f *ownParsedFont) applyVariations(
	glyphID uint16,
	points [][2]int32,
	contourEnds []uint16,
	variations []FontVariation,
) {
	if len(variations) == 0 {
		return
	}

	f.loadFvar()
	if len(f.fvarAxes) == 0 {
		return
	}

	f.loadGvar()
	if f.gvar == nil {
		return
	}

	// Normalize variation coordinates.
	coords := normalizeCoords(f.fvarAxes, variations)

	// Apply avar remapping.
	f.loadAvar()
	f.avar.apply(coords)

	// Optimization: skip gvar delta computation when all normalized coords
	// are zero (default instance). This is the common case when the user
	// specifies e.g. wght=400 on a font where 400 is the default weight.
	// Avoids allocation + IUP computation for a zero-delta result.
	allZero := true
	for _, c := range coords {
		if c != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return
	}

	// Total outline points (without phantom points).
	numPoints := len(points) - 4
	if numPoints < 0 {
		return
	}

	// Compute gvar deltas.
	dx, dy := f.gvar.glyphVariationDeltas(glyphID, coords, numPoints, contourEnds, points)
	if dx == nil || dy == nil {
		return
	}

	// Apply deltas to points.
	for i := range points {
		if i < len(dx) {
			points[i][0] += dx[i]
		}
		if i < len(dy) {
			points[i][1] += dy[i]
		}
	}
}

// locateGlyph returns the byte offset and length of a glyph within the glyf
// table, using the loca table for lookup.
func locateGlyph(locaData []byte, glyphIndex int, isLong bool) (offset, length int) {
	if isLong {
		// Long format: uint32 offsets.
		pos := glyphIndex * 4
		nextPos := pos + 4
		if nextPos+4 > len(locaData) {
			return 0, 0
		}
		start := int(binary.BigEndian.Uint32(locaData[pos : pos+4]))
		end := int(binary.BigEndian.Uint32(locaData[nextPos : nextPos+4]))
		if end > start {
			return start, end - start
		}
		return 0, 0
	}

	// Short format: uint16 offsets * 2.
	pos := glyphIndex * 2
	nextPos := pos + 2
	if nextPos+2 > len(locaData) {
		return 0, 0
	}
	start := int(binary.BigEndian.Uint16(locaData[pos:pos+2])) * 2
	end := int(binary.BigEndian.Uint16(locaData[nextPos:nextPos+2])) * 2
	if end > start {
		return start, end - start
	}
	return 0, 0
}

func init() {
	// Register the own parser alongside ximage.
	// Users can select it with WithParser("own").
	RegisterParser("own", &ownParser{})
}
