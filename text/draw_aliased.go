package text

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// DrawAliased renders text to a destination image using binary (non-anti-aliased)
// coverage. Every pixel in the output is either fully transparent or fully opaque
// (alpha 0 or 255). This matches Skia's SkFont::Edging::kAlias behavior.
//
// The function uses GlyphMaskRasterizer.RasterizeAliased internally, which routes
// through NoAAFiller (integer scanline, binary coverage) instead of AnalyticFiller.
//
// Position (x, y) is the baseline origin (same semantics as Draw).
// Supports sourceFace only. For MultiFace and FilteredFace, this is a no-op —
// callers should fall back to Draw() for complex font stacks.
func DrawAliased(dst draw.Image, text string, face Face, x, y float64, col color.Color) {
	if text == "" || face == nil {
		return
	}

	sf, ok := face.(*sourceFace)
	if !ok {
		return
	}

	text = expandTabs(text)

	parsed := sf.source.Parsed()
	ppem := sf.size
	hinting := sf.config.hinting

	rast := NewGlyphMaskRasterizer()
	src := image.NewUniform(col)

	for glyph := range sf.Glyphs(text) {
		if glyph.GID == 0 {
			continue
		}

		// glyph.X is the accumulated horizontal position (includes all prior advances).
		glyphX := x + glyph.X
		glyphY := y + glyph.Y

		intX := math.Floor(glyphX)
		intY := math.Floor(glyphY)
		subpixelX := glyphX - intX
		subpixelY := glyphY - intY

		result, err := rast.RasterizeAliased(parsed, glyph.GID, ppem, subpixelX, subpixelY, hinting)
		if err != nil || result == nil {
			continue
		}

		maskImg := &image.Alpha{
			Pix:    result.Mask,
			Stride: result.Width,
			Rect:   image.Rect(0, 0, result.Width, result.Height),
		}

		dstX := int(intX) + int(math.Round(float64(result.BearingX)))
		dstY := int(intY) - int(math.Round(float64(result.BearingY)))

		destRect := image.Rect(dstX, dstY, dstX+result.Width, dstY+result.Height)
		draw.DrawMask(dst, destRect, src, image.Point{}, maskImg, image.Point{}, draw.Over)
	}
}
