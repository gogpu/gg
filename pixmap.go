package gg

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
)

// Compile-time interface checks.
var (
	_ image.Image = (*Pixmap)(nil)
	_ draw.Image  = (*Pixmap)(nil)
)

// Pixmap represents a rectangular pixel buffer.
// It implements both image.Image (read-only) and draw.Image (read-write)
// interfaces, making it compatible with Go's standard image ecosystem
// including text rendering via golang.org/x/image/font.
type Pixmap struct {
	width  int
	height int
	data   []uint8 // RGBA format, 4 bytes per pixel
}

// NewPixmap creates a new pixmap with the given dimensions.
func NewPixmap(width, height int) *Pixmap {
	return &Pixmap{
		width:  width,
		height: height,
		data:   make([]uint8, width*height*4),
	}
}

// Width returns the width of the pixmap.
func (p *Pixmap) Width() int {
	return p.width
}

// Height returns the height of the pixmap.
func (p *Pixmap) Height() int {
	return p.height
}

// Data returns the raw pixel data (RGBA format).
func (p *Pixmap) Data() []uint8 {
	return p.data
}

// SetPixel sets the color of a single pixel.
func (p *Pixmap) SetPixel(x, y int, c RGBA) {
	if x < 0 || x >= p.width || y < 0 || y >= p.height {
		return
	}
	i := (y*p.width + x) * 4
	p.data[i+0] = uint8(clamp255(c.R * 255))
	p.data[i+1] = uint8(clamp255(c.G * 255))
	p.data[i+2] = uint8(clamp255(c.B * 255))
	p.data[i+3] = uint8(clamp255(c.A * 255))
}

// GetPixel returns the color of a single pixel.
func (p *Pixmap) GetPixel(x, y int) RGBA {
	if x < 0 || x >= p.width || y < 0 || y >= p.height {
		return Transparent
	}
	i := (y*p.width + x) * 4
	return RGBA{
		R: float64(p.data[i+0]) / 255,
		G: float64(p.data[i+1]) / 255,
		B: float64(p.data[i+2]) / 255,
		A: float64(p.data[i+3]) / 255,
	}
}

// Clear fills the entire pixmap with a color.
func (p *Pixmap) Clear(c RGBA) {
	r := uint8(clamp255(c.R * 255))
	g := uint8(clamp255(c.G * 255))
	b := uint8(clamp255(c.B * 255))
	a := uint8(clamp255(c.A * 255))

	for i := 0; i < len(p.data); i += 4 {
		p.data[i+0] = r
		p.data[i+1] = g
		p.data[i+2] = b
		p.data[i+3] = a
	}
}

// ToImage converts the pixmap to an image.RGBA.
func (p *Pixmap) ToImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, p.width, p.height))
	copy(img.Pix, p.data)
	return img
}

// FromImage creates a pixmap from an image.
func FromImage(img image.Image) *Pixmap {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pm := NewPixmap(width, height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := img.At(bounds.Min.X+x, bounds.Min.Y+y)
			pm.SetPixel(x, y, FromColor(c))
		}
	}

	return pm
}

// SavePNG saves the pixmap to a PNG file.
func (p *Pixmap) SavePNG(path string) error {
	f, err := os.Create(path) //nolint:gosec // path is user-provided intentionally
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	img := p.ToImage()
	return png.Encode(f, img)
}

// At implements the image.Image interface.
func (p *Pixmap) At(x, y int) color.Color {
	return p.GetPixel(x, y).Color()
}

// Set implements the draw.Image interface.
// This allows Pixmap to be used as a destination for image drawing operations,
// including text rendering via golang.org/x/image/font.
func (p *Pixmap) Set(x, y int, c color.Color) {
	p.SetPixel(x, y, FromColor(c))
}

// Bounds implements the image.Image interface.
func (p *Pixmap) Bounds() image.Rectangle {
	return image.Rect(0, 0, p.width, p.height)
}

// ColorModel implements the image.Image interface.
func (p *Pixmap) ColorModel() color.Model {
	return color.NRGBAModel
}

// FillSpan fills a horizontal span of pixels with a solid color (no blending).
// This is optimized for batch operations when the span is >= 16 pixels.
// The span is from x1 (inclusive) to x2 (exclusive) on row y.
func (p *Pixmap) FillSpan(x1, x2, y int, c RGBA) {
	// Bounds checking
	if y < 0 || y >= p.height {
		return
	}
	if x1 >= x2 {
		return
	}
	if x1 < 0 {
		x1 = 0
	}
	if x2 > p.width {
		x2 = p.width
	}
	if x1 >= x2 {
		return
	}

	// Convert color to bytes once
	r := uint8(clamp255(c.R * 255))
	g := uint8(clamp255(c.G * 255))
	b := uint8(clamp255(c.B * 255))
	a := uint8(clamp255(c.A * 255))

	// Calculate start position in data buffer
	startIdx := (y*p.width + x1) * 4
	length := x2 - x1

	// For short spans (< 16 pixels), use simple loop
	if length < 16 {
		for i := 0; i < length; i++ {
			idx := startIdx + i*4
			p.data[idx+0] = r
			p.data[idx+1] = g
			p.data[idx+2] = b
			p.data[idx+3] = a
		}
		return
	}

	// For longer spans, fill first pixel then copy in batches
	// First pixel
	p.data[startIdx+0] = r
	p.data[startIdx+1] = g
	p.data[startIdx+2] = b
	p.data[startIdx+3] = a

	// Double the pattern until we have at least 16 pixels
	filled := 1
	for filled < 16 && filled < length {
		copyLen := filled
		if filled+copyLen > length {
			copyLen = length - filled
		}
		copy(p.data[startIdx+filled*4:], p.data[startIdx:startIdx+copyLen*4])
		filled += copyLen
	}

	// Copy the 16-pixel pattern to fill the rest
	if filled < length {
		patternSize := filled * 4
		for offset := filled * 4; offset < length*4; {
			copyLen := patternSize
			if offset+copyLen > length*4 {
				copyLen = length*4 - offset
			}
			copy(p.data[startIdx+offset:], p.data[startIdx:startIdx+copyLen])
			offset += copyLen
		}
	}
}

// FillSpanBlend fills a horizontal span with blending.
// This uses batch blending operations for spans >= 16 pixels.
func (p *Pixmap) FillSpanBlend(x1, x2, y int, c RGBA) {
	// Bounds checking
	if y < 0 || y >= p.height {
		return
	}
	if x1 >= x2 {
		return
	}
	if x1 < 0 {
		x1 = 0
	}
	if x2 > p.width {
		x2 = p.width
	}
	if x1 >= x2 {
		return
	}

	// If alpha is 1.0 (fully opaque), use direct fill (no blending needed)
	if c.A >= 0.9999 {
		p.FillSpan(x1, x2, y, c)
		return
	}

	// Convert color to premultiplied RGBA bytes
	r := uint8(clamp255(c.R * c.A * 255))
	g := uint8(clamp255(c.G * c.A * 255))
	b := uint8(clamp255(c.B * c.A * 255))
	a := uint8(clamp255(c.A * 255))

	length := x2 - x1
	startIdx := (y*p.width + x1) * 4

	// For short spans, use scalar blending
	if length < 16 {
		for i := 0; i < length; i++ {
			idx := startIdx + i*4
			dr := p.data[idx+0]
			dg := p.data[idx+1]
			db := p.data[idx+2]
			da := p.data[idx+3]

			// Source-over blending: Result = S + D * (1 - Sa)
			invSa := 255 - a
			p.data[idx+0] = r + uint8((uint32(dr)*uint32(invSa)+127)/255) //nolint:gosec // bounded by 255
			p.data[idx+1] = g + uint8((uint32(dg)*uint32(invSa)+127)/255) //nolint:gosec // bounded by 255
			p.data[idx+2] = b + uint8((uint32(db)*uint32(invSa)+127)/255) //nolint:gosec // bounded by 255
			p.data[idx+3] = a + uint8((uint32(da)*uint32(invSa)+127)/255) //nolint:gosec // bounded by 255
		}
		return
	}

	// For longer spans, use the same scalar blending
	// TODO: Integrate batch blending without import cycle
	invSa := 255 - a
	for i := 0; i < length; i++ {
		idx := startIdx + i*4
		dr := p.data[idx+0]
		dg := p.data[idx+1]
		db := p.data[idx+2]
		da := p.data[idx+3]

		p.data[idx+0] = r + uint8((uint32(dr)*uint32(invSa)+127)/255) //nolint:gosec // bounded by 255
		p.data[idx+1] = g + uint8((uint32(dg)*uint32(invSa)+127)/255) //nolint:gosec // bounded by 255
		p.data[idx+2] = b + uint8((uint32(db)*uint32(invSa)+127)/255) //nolint:gosec // bounded by 255
		p.data[idx+3] = a + uint8((uint32(da)*uint32(invSa)+127)/255) //nolint:gosec // bounded by 255
	}
}
