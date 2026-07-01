package text

import "image"

// GlyphImage represents a rasterized glyph.
// This contains the alpha mask and positioning information.
type GlyphImage struct {
	// Mask is the alpha mask (grayscale image).
	// This represents the glyph's shape.
	Mask *image.Alpha

	// Bounds relative to glyph origin.
	// The origin is typically on the baseline at the left edge.
	Bounds image.Rectangle

	// Advance width in pixels.
	// This is how far the cursor should move after drawing this glyph.
	Advance float64
}

// RasterizeGlyph renders a glyph to an alpha mask.
// Uses the ximage parser (golang.org/x/image/font) for rasterization.
//
// This function is primarily intended for future caching implementations
// and advanced use cases. For normal text drawing, use the Draw function instead.
//
// Parameters:
//   - parsed: The parsed font (must be *ximageParsedFont from the "ximage" parser)
//   - glyphID: The glyph index to rasterize
//   - ppem: Pixels per em (font size)
//
// Returns:
//   - *GlyphImage with the rasterized glyph, or nil if rasterization fails
//     or the font type is not ximageParsedFont
func RasterizeGlyph(parsed ParsedFont, glyphID GlyphID, ppem float64) *GlyphImage {
	return rasterizeGlyphXimage(parsed, glyphID, ppem)
}
