package text

// GlyphType indicates how to render a glyph.
type GlyphType uint8

const (
	// GlyphTypeOutline is a vector path glyph (default).
	// Rendered via sparse strips or MSDF.
	GlyphTypeOutline GlyphType = iota

	// GlyphTypeBitmap is an embedded bitmap glyph.
	// Found in sbix (Apple) or CBDT/CBLC (Google) tables.
	// Used for color emoji.
	GlyphTypeBitmap

	// GlyphTypeCOLR is a color layers glyph.
	// Uses COLRv0 or COLRv1 tables for layered color glyphs.
	GlyphTypeCOLR

	// GlyphTypeSVG is an SVG document glyph.
	// Found in SVG table, used for complex color glyphs.
	GlyphTypeSVG
)

// String returns the string representation of the glyph type.
func (t GlyphType) String() string {
	switch t {
	case GlyphTypeOutline:
		return "Outline"
	case GlyphTypeBitmap:
		return "Bitmap"
	case GlyphTypeCOLR:
		return "COLR"
	case GlyphTypeSVG:
		return "SVG"
	default:
		return unknownStr
	}
}

// GlyphFlags provides additional glyph information for rendering.
type GlyphFlags uint8

const (
	// GlyphFlagLigature indicates this glyph is the first in a ligature.
	// The following glyphs with zero advance are part of the same ligature.
	GlyphFlagLigature GlyphFlags = 1 << iota

	// GlyphFlagMark indicates this glyph is a combining mark.
	// Marks are positioned relative to their base glyph.
	GlyphFlagMark

	// GlyphFlagSafeToBreak indicates this is a safe line break point.
	// Used by the layout engine for word wrapping.
	GlyphFlagSafeToBreak

	// GlyphFlagClusterStart indicates this glyph starts a new cluster.
	// Clusters are groups of glyphs that map to one or more characters.
	GlyphFlagClusterStart
)

// Has returns true if the flags contain the specified flag.
func (f GlyphFlags) Has(flag GlyphFlags) bool {
	return f&flag != 0
}

// String returns a human-readable representation of the flags.
func (f GlyphFlags) String() string {
	if f == 0 {
		return noneStr
	}

	var result string
	if f.Has(GlyphFlagLigature) {
		result += "Ligature|"
	}
	if f.Has(GlyphFlagMark) {
		result += "Mark|"
	}
	if f.Has(GlyphFlagSafeToBreak) {
		result += "SafeToBreak|"
	}
	if f.Has(GlyphFlagClusterStart) {
		result += "ClusterStart|"
	}

	// Remove trailing pipe
	if result != "" {
		result = result[:len(result)-1]
	}
	return result
}
