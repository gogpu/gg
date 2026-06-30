package text

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"

	ot "github.com/go-text/typesetting/font/opentype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// ximageParser implements FontParser using golang.org/x/image/font/opentype.
type ximageParser struct{}

// Parse implements FontParser.Parse.
func (p *ximageParser) Parse(data []byte) (ParsedFont, error) {
	return p.ParseIndex(data, 0)
}

// ParseIndex parses a font at the given index within a collection.
// For single fonts (.ttf/.otf), index is ignored. For collections
// (.ttc/.otc), index selects which font to use (0 = first).
func (p *ximageParser) ParseIndex(data []byte, index int) (ParsedFont, error) {
	// Try single font first (most common case).
	f, err := opentype.Parse(data)
	if err == nil {
		return &ximageParsedFont{font: f, rawData: data}, nil
	}

	// Single parse failed — try as collection (.ttc/.otc).
	coll, collErr := opentype.ParseCollection(data)
	if collErr != nil {
		return nil, fmt.Errorf("text: failed to parse font: %w", err)
	}

	if index >= coll.NumFonts() {
		return nil, fmt.Errorf("text: collection index %d out of range (collection has %d fonts)", index, coll.NumFonts())
	}

	cf, cfErr := coll.Font(index)
	if cfErr != nil {
		return nil, fmt.Errorf("text: failed to get font %d from collection: %w", index, cfErr)
	}
	// For collections, store the entire collection data — ParseGlyfContours
	// handles individual glyph extraction from the collection-level glyf table.
	return &ximageParsedFont{font: cf, rawData: data}, nil
}

// ximageParsedFont implements ParsedFont using sfnt.Font.
type ximageParsedFont struct {
	font    *opentype.Font
	rawData []byte // raw font file bytes (for contour-based auto-hinter)

	// HVAR lazy loading (thread-safe).
	hvarOnce sync.Once
	hvar     *hvarTable // nil if HVAR not present or failed to parse
	fvarAxes []fvarAxis // parsed fvar axes for coordinate normalization

	// TT bytecode hint cache — lazy loading (thread-safe).
	// Provides cached fpgm/prep execution results per ppem.
	ttHintOnce  sync.Once
	ttHintCache *ttHintCache // nil if font has no TT instructions
}

// RawFontData implements RawFontDataProvider, returning the raw font file
// bytes. This enables the contour-based auto-hinter path for FreeType/skrifa
// coordinate parity.
func (f *ximageParsedFont) RawFontData() []byte {
	return f.rawData
}

// Name implements ParsedFont.Name.
func (f *ximageParsedFont) Name() string {
	if buf, err := f.font.Name(nil, sfnt.NameIDFamily); err == nil && buf != "" {
		return buf
	}
	return ""
}

// FullName implements ParsedFont.FullName.
func (f *ximageParsedFont) FullName() string {
	if buf, err := f.font.Name(nil, sfnt.NameIDFull); err == nil && buf != "" {
		return buf
	}
	return ""
}

// NumGlyphs implements ParsedFont.NumGlyphs.
func (f *ximageParsedFont) NumGlyphs() int {
	return f.font.NumGlyphs()
}

// UnitsPerEm implements ParsedFont.UnitsPerEm.
func (f *ximageParsedFont) UnitsPerEm() int {
	return int(f.font.UnitsPerEm())
}

// GlyphIndex implements ParsedFont.GlyphIndex.
func (f *ximageParsedFont) GlyphIndex(r rune) uint16 {
	idx, err := f.font.GlyphIndex(nil, r)
	if err != nil {
		return 0
	}
	return uint16(idx)
}

// GlyphAdvance implements ParsedFont.GlyphAdvance.
func (f *ximageParsedFont) GlyphAdvance(glyphIndex uint16, ppem float64) float64 {
	// Create buffer for operations
	var buf sfnt.Buffer

	// Get advance in font units
	advance, err := f.font.GlyphAdvance(&buf, sfnt.GlyphIndex(glyphIndex), fixed.Int26_6(ppem*64), font.HintingNone)
	if err != nil {
		return 0
	}

	return fixedToFloat64(advance)
}

// GlyphBounds implements ParsedFont.GlyphBounds.
func (f *ximageParsedFont) GlyphBounds(glyphIndex uint16, ppem float64) Rect {
	var buf sfnt.Buffer

	bounds, _, err := f.font.GlyphBounds(&buf, sfnt.GlyphIndex(glyphIndex), fixed.Int26_6(ppem*64), font.HintingFull)
	if err != nil {
		return Rect{}
	}

	return Rect{
		MinX: fixedToFloat64(bounds.Min.X),
		MinY: fixedToFloat64(bounds.Min.Y),
		MaxX: fixedToFloat64(bounds.Max.X),
		MaxY: fixedToFloat64(bounds.Max.Y),
	}
}

// Metrics implements ParsedFont.Metrics.
func (f *ximageParsedFont) Metrics(ppem float64) FontMetrics {
	var buf sfnt.Buffer

	metrics, err := f.font.Metrics(&buf, fixed.Int26_6(ppem*64), font.HintingFull)
	if err != nil {
		return FontMetrics{}
	}

	return FontMetrics{
		Ascent:    fixedToFloat64(metrics.Ascent),
		Descent:   fixedToFloat64(metrics.Descent),
		LineGap:   fixedToFloat64(metrics.Height) - fixedToFloat64(metrics.Ascent) + fixedToFloat64(metrics.Descent),
		XHeight:   fixedToFloat64(metrics.XHeight),
		CapHeight: fixedToFloat64(metrics.CapHeight),
	}
}

// loadTTHintCache lazily initializes the TT bytecode hint cache.
// Thread-safe via sync.Once. Returns nil if the font has no TT instructions.
func (f *ximageParsedFont) loadTTHintCache() *ttHintCache {
	f.ttHintOnce.Do(func() {
		if f.rawData == nil {
			return
		}
		f.ttHintCache = newTTHintCache(f.rawData)
	})
	return f.ttHintCache
}

// loadHVAR lazily parses the HVAR table and fvar axes from the raw font data.
// Thread-safe via sync.Once. Silently ignores errors (HVAR is optional).
func (f *ximageParsedFont) loadHVAR() {
	f.hvarOnce.Do(func() {
		if f.rawData == nil {
			return
		}
		loader, err := ot.NewLoader(bytes.NewReader(f.rawData))
		if err != nil {
			return
		}

		// Parse HVAR table.
		hvarRaw, err := loader.RawTable(ot.MustNewTag("HVAR"))
		if err != nil {
			return
		}
		hvar, err := parseHVAR(hvarRaw)
		if err != nil {
			return
		}
		f.hvar = hvar

		// Parse fvar axes for coordinate normalization.
		fvarRaw, err := loader.RawTable(ot.MustNewTag("fvar"))
		if err != nil {
			return
		}
		f.fvarAxes = parseFvarAxes(fvarRaw)
	})
}

// GlyphAdvanceVar implements VariableAdvanceProvider.
// Returns the advance width in pixels for a glyph, adjusted by HVAR deltas
// for the given font variations.
//
// Matches skrifa GlyphMetrics::advance_width (metrics.rs:291-311):
//
//	advance = hmtx_advance + hvar_delta(gid, normalizedCoords)
//	result = advance * ppem / unitsPerEm
func (f *ximageParsedFont) GlyphAdvanceVar(glyphIndex uint16, ppem float64, variations []FontVariation) float64 {
	f.loadHVAR()

	// Get base advance (same as GlyphAdvance).
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

	// Scale delta from font units to pixels: delta * ppem / unitsPerEm.
	upm := float64(f.font.UnitsPerEm())
	if upm == 0 {
		return baseAdvance
	}
	// The delta from advanceDelta is in font units (integer).
	// skrifa truncates to i32, then scales: Fixed::from_i32(delta).to_f64() → float.
	// Then: self.fixed_scale.apply(advance) where fixed_scale = ppem / upem.
	// But since baseAdvance already has the scale applied (ppem/upem factor via sfnt),
	// we need to add the scaled delta separately.
	scaledDelta := float64(delta) * ppem / upm

	return baseAdvance + scaledDelta
}

// parseFvarAxes extracts axis definitions from raw fvar table data.
// Uses direct binary parsing to avoid go-text dependency for this simple task.
//
// fvar table layout:
//
//	uint16  majorVersion (must be 1)
//	uint16  minorVersion (must be 0)
//	Offset16 axisArrayOffset
//	uint16  reserved
//	uint16  axisCount
//	uint16  axisSize (must be 20)
//	... (instance data follows)
//
// Each axis record (20 bytes):
//
//	Tag     axisTag (4 bytes)
//	Fixed   minValue (4 bytes, 16.16)
//	Fixed   defaultValue (4 bytes, 16.16)
//	Fixed   maxValue (4 bytes, 16.16)
//	uint16  flags
//	uint16  axisNameID
func parseFvarAxes(data []byte) []fvarAxis {
	if len(data) < 16 {
		return nil
	}

	// major := binary.BigEndian.Uint16(data[0:2])
	// minor := binary.BigEndian.Uint16(data[2:4])
	axisArrayOffset := binary.BigEndian.Uint16(data[4:6])
	// reserved := binary.BigEndian.Uint16(data[6:8])
	axisCount := binary.BigEndian.Uint16(data[8:10])
	axisSize := binary.BigEndian.Uint16(data[10:12])

	if axisSize < 20 || axisCount == 0 {
		return nil
	}

	start := int(axisArrayOffset)
	if start+int(axisCount)*int(axisSize) > len(data) {
		return nil
	}

	axes := make([]fvarAxis, axisCount)
	for i := range axisCount {
		off := start + int(i)*int(axisSize)
		axes[i] = fvarAxis{
			Tag:          [4]byte{data[off], data[off+1], data[off+2], data[off+3]},
			MinValue:     fixed1616ToFloat32(data[off+4:]),
			DefaultValue: fixed1616ToFloat32(data[off+8:]),
			MaxValue:     fixed1616ToFloat32(data[off+12:]),
		}
	}
	return axes
}

// fixed1616ToFloat32 reads a big-endian Fixed 16.16 value and converts to float32.
func fixed1616ToFloat32(data []byte) float32 {
	raw := int32(binary.BigEndian.Uint32(data[:4]))
	return float32(raw) / 65536.0
}

// fixedToFloat64 converts fixed.Int26_6 to float64.
func fixedToFloat64(x fixed.Int26_6) float64 {
	return float64(x) / 64.0
}
