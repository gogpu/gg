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
// The font size is obtained from face.Size().
//
// The shaping is simple left-to-right positioning without:
//   - Ligature substitution (fi, fl, etc.)
//   - Kerning pairs
//   - Contextual alternates
//   - Right-to-left reordering
//
// For these features, use a full shaper like go-text/typesetting.
func (s *BuiltinShaper) Shape(text string, face Face) []ShapedGlyph {
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

	size := face.Size()
	runes := []rune(text)
	result := make([]ShapedGlyph, 0, len(runes))

	var x float64

	for cluster, r := range runes {
		// Skip control characters (U+0000..U+001F) — no visual representation.
		// Tab (U+0009) is handled below with proper advance width.
		if r < 0x20 && r != '\t' {
			continue
		}

		var gid uint16
		var advance float64

		if r == '\t' {
			// Tab: use space GID (empty outline) with tab-stop advance.
			// Space GID has no contours → correctly skipped by outline renderer.
			gid, advance = tabAdvance(parsed, size)
		} else {
			gid = parsed.GlyphIndex(r)
			advance = parsed.GlyphAdvance(gid, size)
		}

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
