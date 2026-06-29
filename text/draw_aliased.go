package text

import (
	"image/color"
	"image/draw"
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

	if vars := sf.Variations(); len(vars) > 0 {
		drawGlyphsVariable(dst, sf, text, x, y, col, vars, rasterModeAliased)
		return
	}

	drawGlyphs(dst, sf, text, x, y, col, rasterizeAliasedGlyph)
}
