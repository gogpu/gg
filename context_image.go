package gg

import (
	"image"

	intImage "github.com/gogpu/gg/internal/image"
)

// ImageBuf is a public alias for internal ImageBuf.
// It represents a memory-efficient image buffer with support for multiple
// pixel formats and lazy premultiplication.
type ImageBuf = intImage.ImageBuf

// InterpolationMode defines how texture sampling is performed when drawing images.
type InterpolationMode = intImage.InterpolationMode

// Image interpolation modes.
const (
	// InterpNearest selects the closest pixel (no interpolation).
	// Fast but produces blocky results when scaling.
	InterpNearest = intImage.InterpNearest

	// InterpBilinear performs linear interpolation between 4 neighboring pixels.
	// Good balance between quality and performance.
	InterpBilinear = intImage.InterpBilinear

	// InterpBicubic performs cubic interpolation using a 4x4 pixel neighborhood.
	// Highest quality but slower than bilinear.
	InterpBicubic = intImage.InterpBicubic
)

// ImageFormat represents a pixel storage format.
type ImageFormat = intImage.Format

// Pixel formats.
const (
	// FormatGray8 is 8-bit grayscale (1 byte per pixel).
	FormatGray8 = intImage.FormatGray8

	// FormatGray16 is 16-bit grayscale (2 bytes per pixel).
	FormatGray16 = intImage.FormatGray16

	// FormatRGB8 is 24-bit RGB (3 bytes per pixel, no alpha).
	FormatRGB8 = intImage.FormatRGB8

	// FormatRGBA8 is 32-bit RGBA in sRGB color space (4 bytes per pixel).
	// This is the standard format for most operations.
	FormatRGBA8 = intImage.FormatRGBA8

	// FormatRGBAPremul is 32-bit RGBA with premultiplied alpha (4 bytes per pixel).
	// Used for correct alpha blending operations.
	FormatRGBAPremul = intImage.FormatRGBAPremul

	// FormatBGRA8 is 32-bit BGRA in sRGB color space (4 bytes per pixel).
	// Common on Windows and some GPU formats.
	FormatBGRA8 = intImage.FormatBGRA8

	// FormatBGRAPremul is 32-bit BGRA with premultiplied alpha (4 bytes per pixel).
	FormatBGRAPremul = intImage.FormatBGRAPremul
)

// BlendMode defines how source pixels are blended with destination pixels.
type BlendMode = intImage.BlendMode

// Blend modes.
const (
	// BlendNormal performs standard alpha blending (source over destination).
	BlendNormal = intImage.BlendNormal

	// BlendMultiply multiplies source and destination colors.
	// Result is always darker or equal. Formula: dst * src
	BlendMultiply = intImage.BlendMultiply

	// BlendScreen performs inverse multiply for lighter results.
	// Formula: 1 - (1-dst) * (1-src)
	BlendScreen = intImage.BlendScreen

	// BlendOverlay combines multiply and screen based on destination brightness.
	// Dark areas are multiplied, bright areas are screened.
	BlendOverlay = intImage.BlendOverlay
)

// DrawImageOptions specifies parameters for drawing an image.
type DrawImageOptions struct {
	// X, Y specify the top-left corner where the image will be drawn.
	X, Y float64

	// DstWidth and DstHeight specify the dimensions to scale the image to.
	// If zero, the source dimensions are used (possibly from SrcRect).
	DstWidth  float64
	DstHeight float64

	// SrcRect defines the source rectangle to sample from.
	// If nil, the entire source image is used.
	SrcRect *image.Rectangle

	// Interpolation specifies the interpolation mode for sampling.
	// Default is InterpBilinear.
	Interpolation InterpolationMode

	// Opacity controls the overall transparency of the source image (0.0 to 1.0).
	// 1.0 means fully opaque, 0.0 means fully transparent.
	// Default is 1.0.
	Opacity float64

	// BlendMode specifies how to blend source and destination pixels.
	// Default is BlendNormal.
	BlendMode BlendMode
}

// DrawImage draws an image at the specified position.
// The current transformation matrix is applied to the position and size.
//
// Example:
//
//	img, _ := gg.LoadImage("photo.png")
//	dc.DrawImage(img, 100, 100)
func (c *Context) DrawImage(img *ImageBuf, x, y float64) {
	c.DrawImageEx(img, DrawImageOptions{
		X:             x,
		Y:             y,
		Interpolation: InterpBilinear,
		Opacity:       1.0,
		BlendMode:     BlendNormal,
	})
}

// DrawImageEx draws an image with advanced options.
// The current transformation matrix is applied to the position and size.
//
// Example:
//
//	dc.DrawImageEx(img, gg.DrawImageOptions{
//	    X:             100,
//	    Y:             100,
//	    DstWidth:      200,
//	    DstHeight:     150,
//	    Interpolation: gg.InterpBicubic,
//	    Opacity:       0.8,
//	    BlendMode:     gg.BlendNormal,
//	})
func (c *Context) DrawImageEx(img *ImageBuf, opts DrawImageOptions) {
	// Default values
	if opts.Interpolation == 0 {
		opts.Interpolation = InterpBilinear
	}
	if opts.Opacity == 0 {
		opts.Opacity = 1.0
	}

	// Get source dimensions
	srcWidth, srcHeight := img.Bounds()
	var srcRect intImage.Rect
	if opts.SrcRect != nil {
		srcRect = intImage.Rect{
			X:      opts.SrcRect.Min.X,
			Y:      opts.SrcRect.Min.Y,
			Width:  opts.SrcRect.Dx(),
			Height: opts.SrcRect.Dy(),
		}
	} else {
		srcRect = intImage.Rect{
			X:      0,
			Y:      0,
			Width:  srcWidth,
			Height: srcHeight,
		}
	}

	// Determine destination size
	dstWidth := opts.DstWidth
	dstHeight := opts.DstHeight
	if dstWidth == 0 {
		dstWidth = float64(srcRect.Width)
	}
	if dstHeight == 0 {
		dstHeight = float64(srcRect.Height)
	}

	// Transform destination rectangle corners
	topLeft := c.matrix.TransformPoint(Pt(opts.X, opts.Y))
	bottomRight := c.matrix.TransformPoint(Pt(opts.X+dstWidth, opts.Y+dstHeight))

	// Calculate transformed destination rectangle
	dstX := int(topLeft.X)
	dstY := int(topLeft.Y)
	dstW := int(bottomRight.X - topLeft.X)
	dstH := int(bottomRight.Y - topLeft.Y)

	// Create destination rectangle
	dstRect := intImage.Rect{
		X:      dstX,
		Y:      dstY,
		Width:  dstW,
		Height: dstH,
	}

	// Convert Pixmap to ImageBuf for drawing
	// This is a zero-copy operation - ImageBuf wraps the pixmap's data
	dstImg := c.pixmapToImageBuf(c.pixmap)

	// Prepare draw parameters
	params := intImage.DrawParams{
		SrcRect:   &srcRect,
		DstRect:   dstRect,
		Interp:    opts.Interpolation,
		Opacity:   opts.Opacity,
		BlendMode: opts.BlendMode,
	}

	// Draw the image directly into the pixmap via the ImageBuf wrapper
	intImage.DrawImage(dstImg, img, params)
	// No need to copy back - ImageBuf shares the pixmap's underlying data
}

// CreateImagePattern creates an image pattern from a rectangular region of an image.
// The pattern can be used with SetFillPattern or SetStrokePattern.
//
// Example:
//
//	img, _ := gg.LoadImage("texture.png")
//	pattern := dc.CreateImagePattern(img, 0, 0, 100, 100)
//	dc.SetFillPattern(pattern)
//	dc.DrawRectangle(0, 0, 400, 300)
//	dc.Fill()
func (c *Context) CreateImagePattern(img *ImageBuf, x, y, w, h int) Pattern {
	return &ImagePattern{
		image: img,
		x:     x,
		y:     y,
		w:     w,
		h:     h,
	}
}

// SetFillPattern sets the fill pattern.
// It also updates the Brush field for consistency with ColorAt precedence.
func (c *Context) SetFillPattern(pattern Pattern) {
	c.paint.Pattern = pattern
	c.paint.Brush = BrushFromPattern(pattern)
}

// SetStrokePattern sets the stroke pattern.
// It also updates the Brush field for consistency with ColorAt precedence.
func (c *Context) SetStrokePattern(pattern Pattern) {
	c.paint.Pattern = pattern
	c.paint.Brush = BrushFromPattern(pattern)
}

// ImagePattern represents an image-based pattern.
type ImagePattern struct {
	image *ImageBuf
	x, y  int
	w, h  int
}

// ColorAt implements the Pattern interface.
// It samples the image at the given coordinates using wrapping/tiling behavior.
func (p *ImagePattern) ColorAt(x, y float64) RGBA {
	// Get image bounds
	imgW, imgH := p.image.Bounds()

	// If pattern region is specified, use it
	patternW := p.w
	patternH := p.h
	if patternW == 0 {
		patternW = imgW
	}
	if patternH == 0 {
		patternH = imgH
	}

	// Wrap coordinates to pattern region (tiling)
	px := int(x) % patternW
	py := int(y) % patternH
	if px < 0 {
		px += patternW
	}
	if py < 0 {
		py += patternH
	}

	// Add pattern offset
	px += p.x
	py += p.y

	// Sample the image
	r, g, b, a := p.image.GetRGBA(px, py)
	return RGBA{
		R: float64(r) / 255.0,
		G: float64(g) / 255.0,
		B: float64(b) / 255.0,
		A: float64(a) / 255.0,
	}
}

// pixmapToImageBuf converts a Pixmap to an ImageBuf.
// This is a zero-copy operation that wraps the pixmap data.
func (c *Context) pixmapToImageBuf(pm *Pixmap) *ImageBuf {
	// Pixmap uses RGBA8 format
	stride := pm.Width() * 4
	img, _ := intImage.FromRaw(
		pm.Data(),
		pm.Width(),
		pm.Height(),
		intImage.FormatRGBA8,
		stride,
	)
	return img
}

// LoadImage loads an image from a file and returns an ImageBuf.
// Supported formats: PNG, JPEG, WebP.
func LoadImage(path string) (*ImageBuf, error) {
	return intImage.LoadImage(path)
}

// LoadWebP loads a WebP image from the given file path.
func LoadWebP(path string) (*ImageBuf, error) {
	return intImage.LoadWebP(path)
}

// NewImageBuf creates a new image buffer with the given dimensions and format.
func NewImageBuf(width, height int, format ImageFormat) (*ImageBuf, error) {
	return intImage.NewImageBuf(width, height, format)
}

// ImageBufFromImage creates an ImageBuf from a standard image.Image.
func ImageBufFromImage(img image.Image) *ImageBuf {
	return intImage.FromStdImage(img)
}
