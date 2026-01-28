// Package raster provides scanline rasterization for 2D paths.
// This file implements blitters for anti-aliased hairline rendering.
// Based on tiny-skia's blitter.rs (Android/Skia heritage).
package raster

// HairlineBlitter blits anti-aliased hairline pixels.
// This interface is optimized for hairline rendering where pixels
// are written in specific patterns (horizontal, vertical, or diagonal).
type HairlineBlitter interface {
	// BlitH blits a horizontal span at the given coordinates with coverage alpha.
	// The span covers [x, x+width) at row y.
	BlitH(x, y, width int, alpha uint8)

	// BlitV blits a vertical span at the given coordinates with coverage alpha.
	// The span covers [y, y+height) at column x.
	BlitV(x, y, height int, alpha uint8)

	// BlitAntiH2 blits two horizontal pixels with different alpha values.
	// Used for anti-aliased line drawing where coverage varies.
	BlitAntiH2(x, y int, alpha0, alpha1 uint8)

	// BlitAntiV2 blits two vertical pixels with different alpha values.
	// Used for anti-aliased line drawing where coverage varies.
	BlitAntiV2(x, y int, alpha0, alpha1 uint8)
}

// HairlinePixmap extends the Pixmap interface with methods needed for hairline blitting.
type HairlinePixmap interface {
	Width() int
	Height() int
	BlendPixelAlpha(x, y int, c RGBA, alpha uint8)
}

// RGBAHairlineBlitter implements HairlineBlitter for RGBA pixmaps.
// It handles bounds checking and alpha blending.
type RGBAHairlineBlitter struct {
	pixmap HairlinePixmap
	color  RGBA
	width  int
	height int
}

// NewRGBAHairlineBlitter creates a new hairline blitter for the given pixmap and color.
func NewRGBAHairlineBlitter(pixmap HairlinePixmap, color RGBA) *RGBAHairlineBlitter {
	return &RGBAHairlineBlitter{
		pixmap: pixmap,
		color:  color,
		width:  pixmap.Width(),
		height: pixmap.Height(),
	}
}

// BlitH blits a horizontal span with the given alpha coverage.
func (b *RGBAHairlineBlitter) BlitH(x, y, width int, alpha uint8) {
	if alpha == 0 || y < 0 || y >= b.height || width <= 0 {
		return
	}

	// Clamp x bounds
	if x < 0 {
		width += x
		x = 0
	}
	if x+width > b.width {
		width = b.width - x
	}
	if width <= 0 {
		return
	}

	for i := 0; i < width; i++ {
		b.pixmap.BlendPixelAlpha(x+i, y, b.color, alpha)
	}
}

// BlitV blits a vertical span with the given alpha coverage.
func (b *RGBAHairlineBlitter) BlitV(x, y, height int, alpha uint8) {
	if alpha == 0 || x < 0 || x >= b.width || height <= 0 {
		return
	}

	// Clamp y bounds
	if y < 0 {
		height += y
		y = 0
	}
	if y+height > b.height {
		height = b.height - y
	}
	if height <= 0 {
		return
	}

	for i := 0; i < height; i++ {
		b.pixmap.BlendPixelAlpha(x, y+i, b.color, alpha)
	}
}

// BlitAntiH2 blits two horizontal pixels with different alpha values.
// This is the core operation for mostly-vertical lines where coverage
// is distributed between two adjacent horizontal pixels.
func (b *RGBAHairlineBlitter) BlitAntiH2(x, y int, alpha0, alpha1 uint8) {
	if y < 0 || y >= b.height {
		return
	}

	// First pixel
	if x >= 0 && x < b.width && alpha0 > 0 {
		b.pixmap.BlendPixelAlpha(x, y, b.color, alpha0)
	}

	// Second pixel
	x1 := x + 1
	if x1 >= 0 && x1 < b.width && alpha1 > 0 {
		b.pixmap.BlendPixelAlpha(x1, y, b.color, alpha1)
	}
}

// BlitAntiV2 blits two vertical pixels with different alpha values.
// This is the core operation for mostly-horizontal lines where coverage
// is distributed between two adjacent vertical pixels.
func (b *RGBAHairlineBlitter) BlitAntiV2(x, y int, alpha0, alpha1 uint8) {
	if x < 0 || x >= b.width {
		return
	}

	// First pixel (upper)
	if y >= 0 && y < b.height && alpha0 > 0 {
		b.pixmap.BlendPixelAlpha(x, y, b.color, alpha0)
	}

	// Second pixel (lower)
	y1 := y + 1
	if y1 >= 0 && y1 < b.height && alpha1 > 0 {
		b.pixmap.BlendPixelAlpha(x, y1, b.color, alpha1)
	}
}
