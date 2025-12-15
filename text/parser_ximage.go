package text

import (
	"fmt"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// ximageParser implements FontParser using golang.org/x/image/font/opentype.
type ximageParser struct{}

// Parse implements FontParser.Parse.
func (p *ximageParser) Parse(data []byte) (ParsedFont, error) {
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("text: failed to parse font: %w", err)
	}
	return &ximageParsedFont{font: f}, nil
}

// ximageParsedFont implements ParsedFont using sfnt.Font.
type ximageParsedFont struct {
	font *opentype.Font
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
	advance, err := f.font.GlyphAdvance(&buf, sfnt.GlyphIndex(glyphIndex), fixed.Int26_6(ppem*64), font.HintingFull)
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

// fixedToFloat64 converts fixed.Int26_6 to float64.
func fixedToFloat64(x fixed.Int26_6) float64 {
	return float64(x) / 64.0
}
