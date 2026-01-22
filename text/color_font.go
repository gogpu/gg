package text

import (
	"github.com/gogpu/gg/text/emoji"
)

// ColorFont is an optional interface that ParsedFont implementations
// can satisfy to provide color glyph support (emoji, icons).
//
// This interface is separate from ParsedFont to avoid breaking
// existing implementations while adding color support.
type ColorFont interface {
	// HasColorTables returns true if the font has any color tables
	// (CBDT/CBLC, sbix, COLR/CPAL, or SVG).
	HasColorTables() bool

	// GlyphType returns the rendering type for a glyph.
	// This determines which rendering path to use.
	GlyphType(glyphID uint16) GlyphType

	// BitmapGlyph extracts a bitmap glyph at the requested PPEM.
	// Returns nil and error if the glyph is not a bitmap glyph.
	// Uses StrikeBestFit strategy by default.
	BitmapGlyph(glyphID uint16, ppem uint16) (*emoji.BitmapGlyph, error)

	// COLRGlyph returns the COLR glyph data for layered rendering.
	// Returns nil and error if the glyph is not a COLR glyph.
	COLRGlyph(glyphID uint16, paletteIndex int) (*emoji.COLRGlyph, error)
}

// ColorFontInfo provides information about a font's color capabilities.
type ColorFontInfo struct {
	// HasCBDT is true if the font has CBDT/CBLC tables (Google format).
	HasCBDT bool

	// HasSbix is true if the font has sbix table (Apple format).
	HasSbix bool

	// HasCOLR is true if the font has COLR/CPAL tables (Microsoft format).
	HasCOLR bool

	// HasSVG is true if the font has SVG table (Mozilla format).
	HasSVG bool

	// COLRVersion is the COLR table version (0 or 1), if HasCOLR is true.
	COLRVersion uint16

	// BitmapStrikes lists available bitmap sizes (PPEM values).
	BitmapStrikes []uint16
}

// PreferredColorFormat returns the recommended color format for a font.
// Priority: CBDT > sbix > COLR > SVG (based on compatibility research).
func (info ColorFontInfo) PreferredColorFormat() string {
	if info.HasCBDT {
		return "CBDT"
	}
	if info.HasSbix {
		return "sbix"
	}
	if info.HasCOLR {
		return "COLR"
	}
	if info.HasSVG {
		return "SVG"
	}
	return ""
}

// HasAnyColorTable returns true if any color table is present.
func (info ColorFontInfo) HasAnyColorTable() bool {
	return info.HasCBDT || info.HasSbix || info.HasCOLR || info.HasSVG
}

// DetectGlyphType determines the glyph type for rendering.
// This is a helper function that checks for color tables.
//
// Priority order (based on Rust library research):
//  1. COLR (vector, scalable without quality loss)
//  2. Bitmap (CBDT/sbix - PNG based, widely compatible)
//  3. Outline (fallback for non-color glyphs)
//
// Note: In practice, CBDT is often more compatible than COLR
// (see cosmic-text #2546 - COLRv1 renders blank on some systems).
func DetectGlyphType(font ParsedFont, glyphID uint16) GlyphType {
	// Check if font implements ColorFont interface.
	colorFont, ok := font.(ColorFont)
	if !ok {
		return GlyphTypeOutline
	}

	return colorFont.GlyphType(glyphID)
}

// GetBitmapGlyph extracts a bitmap glyph from a font.
// Returns nil and error if the font doesn't support bitmap glyphs
// or if the glyph is not available as bitmap.
func GetBitmapGlyph(font ParsedFont, glyphID uint16, ppem uint16) (*emoji.BitmapGlyph, error) {
	colorFont, ok := font.(ColorFont)
	if !ok {
		return nil, emoji.ErrGlyphNotInBitmap
	}

	return colorFont.BitmapGlyph(glyphID, ppem)
}

// GetCOLRGlyph extracts a COLR glyph from a font.
// Returns nil and error if the font doesn't support COLR glyphs
// or if the glyph is not available as COLR.
func GetCOLRGlyph(font ParsedFont, glyphID uint16, paletteIndex int) (*emoji.COLRGlyph, error) {
	colorFont, ok := font.(ColorFont)
	if !ok {
		return nil, emoji.ErrGlyphNotInCOLR
	}

	return colorFont.COLRGlyph(glyphID, paletteIndex)
}
