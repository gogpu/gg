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
// MultiFace and FilteredFace are rendered per source-owned rune so aliased
// output follows the same fallback policy as Draw without silently dropping
// the text.
func DrawAliased(dst draw.Image, text string, face Face, x, y float64, col color.Color) {
	if text == "" || face == nil {
		return
	}

	text = expandTabs(text)
	drawAliasedFace(dst, text, face, x, y, col)
}

func drawAliasedFace(dst draw.Image, text string, face Face, x, y float64, col color.Color) {
	sf, ok := face.(*sourceFace)
	if ok {
		drawAliasedSourceFace(dst, text, sf, x, y, col)
		return
	}

	switch face.(type) {
	case *MultiFace, *FilteredFace:
		drawAliasedComposite(dst, text, face, x, y, col)
	}
}

func drawAliasedSourceFace(dst draw.Image, text string, sf *sourceFace, x, y float64, col color.Color) {
	if sf == nil {
		return
	}

	if vars := sf.Variations(); len(vars) > 0 {
		drawGlyphsVariable(dst, sf, text, x, y, col, vars, rasterModeAliased)
		return
	}

	drawGlyphs(dst, sf, text, x, y, col, rasterizeAliasedGlyph)
}

// drawAliasedComposite walks one rune at a time to preserve the fallback
// face's advance contract. Each selected source face is then sent through the
// same aliased rasterizer as a standalone sourceFace.
func drawAliasedComposite(dst draw.Image, text string, face Face, x, y float64, col color.Color) {
	currentX := x
	for _, r := range text {
		runeText := string(r)
		if filtered, ok := face.(*FilteredFace); ok && !filtered.inRanges(r) {
			continue
		}

		owner := face
		switch f := face.(type) {
		case *MultiFace:
			owner = f.FaceForRune(r)
		case *FilteredFace:
			owner = f.face
		}
		if owner != nil {
			drawAliasedFace(dst, runeText, owner, currentX, y, col)
		}
		currentX += faceGlyphAdvance(face, runeText)
	}
}
