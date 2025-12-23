package text

// BuiltinShaper provides text shaping using golang.org/x/image/font.
// It supports Latin, Cyrillic, Greek, CJK, and other scripts that don't
// require complex text shaping (ligatures, contextual forms, etc.).
//
// For complex scripts like Arabic, Hebrew, or Indic languages that require
// advanced shaping features (GSUB/GPOS tables), use SetShaper() with a
// HarfBuzz-compatible implementation such as go-text/typesetting.
//
// BuiltinShaper is stateless and safe for concurrent use.
type BuiltinShaper struct{}

// Shape implements the Shaper interface.
// It converts text to positioned glyphs using the font's glyph metrics.
//
// The shaping is simple left-to-right positioning without:
//   - Ligature substitution (fi, fl, etc.)
//   - Kerning pairs
//   - Contextual alternates
//   - Right-to-left reordering
//
// For these features, use a full shaper like go-text/typesetting.
func (s *BuiltinShaper) Shape(text string, face Face, size float64) []ShapedGlyph {
	if text == "" || face == nil {
		return nil
	}

	source := face.Source()
	if source == nil {
		return nil
	}

	parsed := source.Parsed()
	if parsed == nil {
		return nil
	}

	runes := []rune(text)
	result := make([]ShapedGlyph, 0, len(runes))

	var x float64

	for cluster, r := range runes {
		// Get glyph index from font
		gid := parsed.GlyphIndex(r)

		// Get advance width for this glyph at the given size
		advance := parsed.GlyphAdvance(gid, size)

		result = append(result, ShapedGlyph{
			GID:      GlyphID(gid),
			Cluster:  cluster,
			X:        x,
			Y:        0,
			XAdvance: advance,
			YAdvance: 0,
		})

		x += advance
	}

	return result
}
