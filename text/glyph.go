package text

// GlyphID is a unique identifier for a glyph within a font.
// The glyph ID is assigned by the font file and is font-specific.
type GlyphID uint16

// Glyph represents a single shaped glyph with its position and metrics.
// This is the output of text shaping and is ready for rendering.
type Glyph struct {
	// Rune is the Unicode character this glyph represents.
	// For ligatures, this may be the first character of the ligature.
	Rune rune

	// GID is the glyph index in the font.
	GID GlyphID

	// X, Y are the position of the glyph relative to the text origin.
	// The origin is at the baseline of the first character.
	X, Y float64

	// OriginX, OriginY are the absolute position of the glyph's origin point.
	// This is where the glyph should be drawn from.
	OriginX float64
	OriginY float64

	// Advance is the horizontal advance width of the glyph.
	// This is how much the cursor moves after drawing this glyph.
	Advance float64

	// Bounds is the bounding box of the glyph.
	// This defines the area the glyph occupies.
	Bounds Rect

	// Index is the byte position in the original string where this glyph starts.
	Index int

	// Cluster is the character cluster index.
	// Multiple glyphs can belong to the same cluster (e.g., ligatures).
	Cluster int
}
