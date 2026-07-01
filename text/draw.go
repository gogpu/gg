package text

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// Draw renders text to a destination image.
// Position (x, y) is the baseline origin.
// Supports sourceFace, MultiFace, and FilteredFace.
func Draw(dst draw.Image, text string, face Face, x, y float64, col color.Color) {
	if text == "" || face == nil {
		return
	}

	// Expand tabs to spaces for bitmap rendering.
	// font.Drawer maps \t to .notdef (tofu) because fonts lack a tab glyph.
	// Tab = globalTabWidth spaces (default: 8, matching CSS/Pango/POSIX).
	text = expandTabs(text)

	switch f := face.(type) {
	case *sourceFace:
		drawSourceFace(dst, text, f, x, y, col)
	case *MultiFace:
		drawMultiFace(dst, text, f, x, y, col)
	case *FilteredFace:
		drawFilteredFace(dst, text, f, x, y, col)
	}
}

// glyphRasterMode selects the rasterization coverage mode.
// Outline extraction (sfnt or go-text) and rasterization mode are orthogonal
// concerns (Skia pattern: SkFont::Edging is independent of font variations).
type glyphRasterMode int

const (
	rasterModeAA      glyphRasterMode = iota // 256-level analytic AA coverage
	rasterModeAliased                        // binary 0/255 coverage (Skia kAlias)
)

// glyphRasterizeFunc is the per-glyph rasterization callback used by drawGlyphs.
// It abstracts the difference between RasterizeHinted (256-level AA) and
// RasterizeAliased (binary coverage), allowing drawSourceFace and DrawAliased
// to share the glyph iteration and compositing loop.
type glyphRasterizeFunc func(
	rast *GlyphMaskRasterizer,
	pf ParsedFont,
	gid GlyphID,
	ppem float64,
	subpixelX, subpixelY float64,
	hinting Hinting,
) (*GlyphMaskResult, error)

// drawGlyphs is the shared per-glyph rendering loop for drawSourceFace and
// DrawAliased. Each glyph is individually rasterized via the provided callback,
// then composited at its precise subpixel position using draw.DrawMask.
//
// The Glyphs() iterator returns fractional X positions from HintingNone
// advances (ADR-039), while the rasterize callback controls outline
// rasterization (AA vs aliased).
func drawGlyphs(
	dst draw.Image,
	sf *sourceFace,
	text string,
	x, y float64,
	col color.Color,
	rasterize glyphRasterizeFunc,
) {
	if vars := sf.Variations(); len(vars) > 0 {
		drawGlyphsVariable(dst, sf, text, x, y, col, vars, rasterModeAA)
		return
	}

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

		result, err := rasterize(rast, parsed, glyph.GID, ppem, subpixelX, subpixelY, hinting)
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

// drawGlyphsVariable renders text using go-text/typesetting for outline extraction,
// which supports variable font tables (gvar/HVAR). This path is used when
// the face has font variations configured via WithVariations.
//
// The mode parameter selects coverage computation (Skia pattern: outline source
// and rasterization mode are orthogonal — variable fonts don't affect AA choice).
func drawGlyphsVariable(
	dst draw.Image,
	sf *sourceFace,
	text string,
	x, y float64,
	col color.Color,
	variations []FontVariation,
	mode glyphRasterMode,
) {
	source := sf.source
	gtFont, err := GetGoTextFont(source)
	if err != nil {
		return
	}

	parsed := source.Parsed()
	ppem := sf.size
	rast := NewGlyphMaskRasterizer()
	src := image.NewUniform(col)

	// Hinting only for aliased mode (Skia pattern: FT_LOAD_TARGET_MONO for kAlias,
	// FT_LOAD_TARGET_NORMAL for kAntiAlias). AA rendering benefits from smooth
	// unhinted outlines; aliased needs grid-fitting for crisp stems.
	applyHinting := mode == rasterModeAliased && sf.config.hinting != HintingNone

	advanceX := 0.0
	for _, r := range text {
		if r < 0x20 && r != '\t' {
			continue
		}

		gid := GlyphID(parsed.GlyphIndex(r))
		if gid == 0 {
			advanceX += goTextGlyphAdvance(gtFont, gid, ppem, variations)
			continue
		}

		glyphX := x + advanceX
		glyphY := y

		intX := math.Floor(glyphX)
		intY := math.Floor(glyphY)
		subpixelX := glyphX - intX
		subpixelY := glyphY - intY

		outline := ExtractOutlineGoText(gtFont, gid, ppem, variations)
		if applyHinting && outline != nil && !outline.IsEmpty() {
			gridFitOutline(outline, sf.config.hinting)
		}
		if outline == nil || outline.IsEmpty() {
			if outline != nil {
				advanceX += float64(outline.Advance)
			}
			continue
		}

		var result *GlyphMaskResult
		var rErr error
		switch mode {
		case rasterModeAliased:
			result, rErr = rast.RasterizeOutlineAliased(outline, subpixelX, subpixelY)
		default:
			result, rErr = rast.RasterizeOutline(outline, subpixelX, subpixelY)
		}
		if rErr != nil || result == nil {
			advanceX += float64(outline.Advance)
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

		advanceX += float64(outline.Advance)
	}
}

// rasterizeHintedGlyph rasterizes a glyph with 256-level analytic AA coverage.
func rasterizeHintedGlyph(
	rast *GlyphMaskRasterizer,
	pf ParsedFont,
	gid GlyphID,
	ppem float64,
	subpixelX, subpixelY float64,
	hinting Hinting,
) (*GlyphMaskResult, error) {
	return rast.RasterizeHinted(pf, gid, ppem, subpixelX, subpixelY, hinting)
}

// rasterizeAliasedGlyph rasterizes a glyph with binary (0 or 255) coverage.
func rasterizeAliasedGlyph(
	rast *GlyphMaskRasterizer,
	pf ParsedFont,
	gid GlyphID,
	ppem float64,
	subpixelX, subpixelY float64,
	hinting Hinting,
) (*GlyphMaskResult, error) {
	return rast.RasterizeAliased(pf, gid, ppem, subpixelX, subpixelY, hinting)
}

// drawSourceFace renders text using per-glyph rasterization with fractional
// advances. Each glyph is individually rasterized via GlyphMaskRasterizer
// (256-level analytic AA with hinting), then composited at its precise
// subpixel position.
//
// This replaces the previous font.Drawer approach which used integer-rounded
// advances internally, causing letters to merge at small sizes (e.g., "Te"
// at 12px). The Glyphs() iterator now returns fractional X positions from
// HintingNone advances (ADR-039), while outline rasterization still uses
// the face's configured hinting for crisp stems.
func drawSourceFace(dst draw.Image, text string, sf *sourceFace, x, y float64, col color.Color) {
	drawGlyphs(dst, sf, text, x, y, col, rasterizeHintedGlyph)
}

// drawMultiFace renders text using a MultiFace, selecting the appropriate font for each rune.
func drawMultiFace(dst draw.Image, text string, mf *MultiFace, x, y float64, col color.Color) {
	currentX := x

	// Tabs already expanded to spaces by Draw() via expandTabs().
	for _, r := range text {
		runeStr := string(r)

		// Find the face that has this glyph
		var faceToUse Face
		for _, f := range mf.faces {
			if f.HasGlyph(r) {
				faceToUse = f
				break
			}
		}

		// Fallback to first face if no face has the glyph
		if faceToUse == nil {
			faceToUse = mf.faces[0]
		}

		// Get advance for this rune
		advance := 0.0
		for glyph := range faceToUse.Glyphs(runeStr) {
			advance = glyph.Advance
			break
		}

		// Render based on face type
		switch f := faceToUse.(type) {
		case *sourceFace:
			drawSourceFace(dst, runeStr, f, currentX, y, col)
		case *FilteredFace:
			drawFilteredFace(dst, runeStr, f, currentX, y, col)
		case *MultiFace:
			// Nested MultiFace (rare but possible)
			drawMultiFace(dst, runeStr, f, currentX, y, col)
		}

		currentX += advance
	}
}

// drawFilteredFace renders text using a FilteredFace.
func drawFilteredFace(dst draw.Image, text string, ff *FilteredFace, x, y float64, col color.Color) {
	// FilteredFace wraps another face - extract and use it
	// Only render runes that pass the filter
	currentX := x

	// Tabs already expanded to spaces by Draw() via expandTabs().
	for _, r := range text {
		if !ff.inRanges(r) {
			continue // Skip filtered runes
		}

		runeStr := string(r)

		// Get advance for this rune
		advance := 0.0
		for glyph := range ff.face.Glyphs(runeStr) {
			advance = glyph.Advance
			break
		}

		// Render using the underlying face
		switch f := ff.face.(type) {
		case *sourceFace:
			drawSourceFace(dst, runeStr, f, currentX, y, col)
		case *FilteredFace:
			drawFilteredFace(dst, runeStr, f, currentX, y, col)
		case *MultiFace:
			drawMultiFace(dst, runeStr, f, currentX, y, col)
		}

		currentX += advance
	}
}

// Measure returns the dimensions of text.
// Width is the horizontal advance, height is the font's line height.
func Measure(text string, face Face) (width, height float64) {
	if text == "" || face == nil {
		return 0, 0
	}

	// Get advance width
	width = face.Advance(text)

	// Get line height from metrics
	metrics := face.Metrics()
	height = metrics.LineHeight()

	return width, height
}

// DrawOptions provides advanced options for text drawing.
// Reserved for future enhancements.
type DrawOptions struct {
	// Color for the text (default: black)
	Color color.Color
}
