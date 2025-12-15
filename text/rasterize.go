package text

import (
	"image"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
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

// RasterizeGlyph renders a glyph to an alpha mask.
// Uses golang.org/x/image/font for rasterization.
//
// This function is primarily intended for future caching implementations
// and advanced use cases. For normal text drawing, use the Draw function instead.
//
// Parameters:
//   - parsed: The parsed font (must be *ximageParsedFont)
//   - glyphID: The glyph index to rasterize
//   - ppem: Pixels per em (font size)
//
// Returns:
//   - *GlyphImage with the rasterized glyph, or nil if rasterization fails
func RasterizeGlyph(parsed ParsedFont, glyphID GlyphID, ppem float64) *GlyphImage {
	// Type assert to ximage parsed font
	xparsed, ok := parsed.(*ximageParsedFont)
	if !ok {
		return nil
	}

	// Create opentype face
	opts := &opentype.FaceOptions{
		Size:    ppem,
		DPI:     72,
		Hinting: font.HintingFull,
	}

	otFace, err := opentype.NewFace(xparsed.font, opts)
	if err != nil {
		return nil
	}
	defer func() {
		_ = otFace.Close()
	}()

	// Get glyph bounds
	bounds, advance, ok := otFace.GlyphBounds(rune(glyphID))
	if !ok {
		return nil
	}

	// Convert fixed.Int26_6 to pixels
	minX := int(bounds.Min.X) >> 6
	minY := int(bounds.Min.Y) >> 6
	maxX := int(bounds.Max.X+63) >> 6
	maxY := int(bounds.Max.Y+63) >> 6

	// Create bounds rectangle
	rect := image.Rect(minX, minY, maxX, maxY)

	// Create alpha mask
	mask := image.NewAlpha(rect)

	// Draw glyph to mask
	drawer := &font.Drawer{
		Dst:  mask,
		Src:  image.White,
		Face: otFace,
		Dot:  fixed.Point26_6{X: 0, Y: 0},
	}

	// Draw the glyph
	drawer.Dot = fixed.Point26_6{X: -bounds.Min.X, Y: -bounds.Min.Y}
	drawer.DrawString(string(rune(glyphID)))

	return &GlyphImage{
		Mask:    mask,
		Bounds:  rect,
		Advance: fixedToFloat64(advance),
	}
}
