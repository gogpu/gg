package text

// ShapedGlyph represents a positioned glyph ready for GPU rendering.
// Unlike Glyph which contains CPU rasterization data (Mask), ShapedGlyph
// is minimal and designed for efficient GPU text rendering pipelines.
type ShapedGlyph struct {
	// GID is the glyph index in the font.
	GID GlyphID

	// Face identifies the source face that owns GID. It is set by Shape and
	// source-aware fallback runs, and is nil for legacy/custom shaped data
	// where the Face argument supplied to the renderer owns every glyph.
	//
	// Glyph IDs are local to a font, so retaining this identity is required
	// when a shaped run contains glyphs from more than one fallback face.
	Face Face

	// Cluster is the source character index in the original text.
	// Used for hit testing and cursor positioning.
	Cluster int

	// IsCJK indicates that this glyph belongs to a CJK script (Han, Hiragana,
	// Katakana, Hangul). Used for script-aware rendering: CJK glyphs use
	// reduced hinting and bypass bucket quantization (ADR-027).
	IsCJK bool

	// X is the horizontal position relative to the text origin.
	X float64

	// Y is the vertical position relative to the baseline.
	Y float64

	// XAdvance is the horizontal advance to the next glyph.
	XAdvance float64

	// YAdvance is the vertical advance (for vertical text).
	YAdvance float64
}
