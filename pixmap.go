package gg

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// Pixmap represents a rectangular pixel buffer.
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

// Bounds implements the image.Image interface.
func (p *Pixmap) Bounds() image.Rectangle {
	return image.Rect(0, 0, p.width, p.height)
}

// ColorModel implements the image.Image interface.
func (p *Pixmap) ColorModel() color.Model {
	return color.NRGBAModel
}
