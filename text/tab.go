package text

import "strings"

// DefaultTabWidth is the number of space characters per tab stop.
// Default is 8, matching CSS tab-size, Pango, and POSIX terminal conventions.
// Use SetTabWidth to change the default for all text rendering.
const DefaultTabWidth = 8

var globalTabWidth = DefaultTabWidth

// SetTabWidth sets the global tab width (number of space characters per tab).
// Affects all text rendering (bitmap, outline, GPU paths).
// Pass 0 or negative to reset to DefaultTabWidth.
func SetTabWidth(n int) {
	if n <= 0 {
		n = DefaultTabWidth
	}
	globalTabWidth = n
}

// TabWidth returns the current global tab width.
func TabWidth() int {
	return globalTabWidth
}

// spaceGIDAndAdvance returns the space character's glyph ID and advance width.
// Used by tab handling to produce advance-only glyphs with empty outlines.
func spaceGIDAndAdvance(parsed ParsedFont, size float64) (uint16, float64) {
	gid := parsed.GlyphIndex(' ')
	advance := parsed.GlyphAdvance(gid, size)
	return gid, advance
}

// tabAdvance computes the advance width for a tab character.
// Returns (spaceGID, tabAdvanceWidth).
func tabAdvance(parsed ParsedFont, size float64) (uint16, float64) {
	gid, spaceAdv := spaceGIDAndAdvance(parsed, size)
	return gid, spaceAdv * float64(globalTabWidth)
}

// expandTabs replaces tab characters with globalTabWidth spaces.
// Used by bitmap rendering (Draw) where font.Drawer maps \t to .notdef (tofu).
// For shaper/outline paths, use tabAdvance instead (preserves glyph-level semantics).
func expandTabs(s string) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", globalTabWidth))
}

// fixTabGlyphs post-processes shaped glyphs to fix tab characters.
// HarfBuzz may produce notdef (GID=0) for tabs — this replaces them
// with space GID and proper tab-stop advance.
func fixTabGlyphs(glyphs []ShapedGlyph, runes []rune, face Face) {
	if face == nil || face.Source() == nil {
		return
	}
	parsed := face.Source().Parsed()
	if parsed == nil {
		return
	}

	spaceGID, tabAdv := tabAdvance(parsed, face.Size())

	// Rebuild X positions from scratch to account for changed advances.
	var needsRebuild bool
	for i := range glyphs {
		cluster := glyphs[i].Cluster
		if cluster >= 0 && cluster < len(runes) && runes[cluster] == '\t' {
			glyphs[i].GID = GlyphID(spaceGID)
			glyphs[i].XAdvance = tabAdv
			needsRebuild = true
		}
	}

	if needsRebuild {
		x := 0.0
		for i := range glyphs {
			glyphs[i].X = x
			x += glyphs[i].XAdvance
		}
	}
}
