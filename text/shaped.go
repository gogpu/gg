package text

// ShapedGlyph represents a positioned glyph ready for GPU rendering.
// Unlike Glyph which contains CPU rasterization data (Mask), ShapedGlyph
// is minimal and designed for efficient GPU text rendering pipelines.
type ShapedGlyph struct {
	// GID is the glyph index in the font.
	GID GlyphID

	// Cluster is the source character index in the original text.
	// Used for hit testing and cursor positioning.
	Cluster int

	// X is the horizontal position relative to the text origin.
	X float64

	// Y is the vertical position relative to the baseline.
	Y float64

	// XAdvance is the horizontal advance to the next glyph.
	XAdvance float64

	// YAdvance is the vertical advance (for vertical text).
	YAdvance float64
}
