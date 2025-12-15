package text

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Draw renders text to a destination image.
// Position (x, y) is the baseline origin.
// The face must be a *sourceFace from this package.
func Draw(dst draw.Image, text string, face Face, x, y float64, col color.Color) {
	if text == "" || face == nil {
		return
	}

	// Type assert to get source face
	sf, ok := face.(*sourceFace)
	if !ok {
		return
	}

	// Get the parsed font
	parsed := sf.source.Parsed()
	xparsed, ok := parsed.(*ximageParsedFont)
	if !ok {
		return
	}

	// Create opentype face
	opts := &opentype.FaceOptions{
		Size:    sf.size,
		DPI:     72,
		Hinting: mapHinting(sf.config.hinting),
	}

	otFace, err := opentype.NewFace(xparsed.font, opts)
	if err != nil {
		return
	}
	defer func() {
		_ = otFace.Close()
	}()

	// Create drawer
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: otFace,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)},
	}

	// Draw the text
	d.DrawString(text)
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

// mapHinting converts text.Hinting to font.Hinting.
func mapHinting(h Hinting) font.Hinting {
	switch h {
	case HintingNone:
		return font.HintingNone
	case HintingVertical:
		return font.HintingVertical
	case HintingFull:
		return font.HintingFull
	default:
		return font.HintingFull
	}
}
