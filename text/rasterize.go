package text

import (
	"image"
	"math"
)

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

// RasterizeGlyph renders a glyph to an alpha mask using [GlyphMaskRasterizer].
//
// This function is primarily intended for future caching implementations
// and advanced use cases. For normal text drawing, use the Draw function instead.
//
// Parameters:
//   - parsed: The parsed font
//   - glyphID: The glyph index to rasterize
//   - ppem: Pixels per em (font size)
//
// Returns:
//   - *GlyphImage with the rasterized glyph, or nil if rasterization fails
func RasterizeGlyph(parsed ParsedFont, glyphID GlyphID, ppem float64) *GlyphImage {
	rast := NewGlyphMaskRasterizer()
	result, err := rast.RasterizeHinted(parsed, glyphID, ppem, 0, 0, HintingFull)
	if err != nil || result == nil {
		return nil
	}

	maskImg := &image.Alpha{
		Pix:    result.Mask,
		Stride: result.Width,
		Rect:   image.Rect(0, 0, result.Width, result.Height),
	}

	return &GlyphImage{
		Mask: maskImg,
		Bounds: image.Rect(
			int(math.Round(float64(result.BearingX))),
			-int(math.Round(float64(result.BearingY))),
			int(math.Round(float64(result.BearingX)))+result.Width,
			-int(math.Round(float64(result.BearingY)))+result.Height,
		),
		Advance: float64(result.Width), // simplified, true advance from font
	}
}
