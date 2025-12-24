package gg

import "image"

// Mask represents an alpha mask for compositing operations.
// Values range from 0 (fully transparent) to 255 (fully opaque).
type Mask struct {
	width  int
	height int
	data   []uint8
}

// NewMask creates a new empty mask with the given dimensions.
// All values are initialized to 0 (fully transparent).
func NewMask(width, height int) *Mask {
	return &Mask{
		width:  width,
		height: height,
		data:   make([]uint8, width*height),
	}
}

// NewMaskFromAlpha creates a mask from an image's alpha channel.
func NewMaskFromAlpha(img image.Image) *Mask {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	mask := NewMask(w, h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			_, _, _, a := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			// a is 0-65535, shift by 8 to get 0-255
			// #nosec G115 -- safe: a>>8 is always in range [0, 255]
			mask.data[y*w+x] = uint8(a >> 8)
		}
	}

	return mask
}

// Bounds returns the mask dimensions as an image.Rectangle.
func (m *Mask) Bounds() image.Rectangle {
	return image.Rect(0, 0, m.width, m.height)
}

// Width returns the mask width.
func (m *Mask) Width() int { return m.width }

// Height returns the mask height.
func (m *Mask) Height() int { return m.height }

// At returns the mask value at (x, y).
// Returns 0 for coordinates outside the mask bounds.
func (m *Mask) At(x, y int) uint8 {
	if x < 0 || x >= m.width || y < 0 || y >= m.height {
		return 0
	}
	return m.data[y*m.width+x]
}

// Set sets the mask value at (x, y).
// Coordinates outside the mask bounds are ignored.
func (m *Mask) Set(x, y int, value uint8) {
	if x < 0 || x >= m.width || y < 0 || y >= m.height {
		return
	}
	m.data[y*m.width+x] = value
}

// Fill fills the entire mask with a value.
func (m *Mask) Fill(value uint8) {
	for i := range m.data {
		m.data[i] = value
	}
}

// Invert inverts all mask values (255 - value).
func (m *Mask) Invert() {
	for i := range m.data {
		m.data[i] = 255 - m.data[i]
	}
}

// Clear clears the mask (sets all values to 0).
func (m *Mask) Clear() {
	for i := range m.data {
		m.data[i] = 0
	}
}

// Clone creates a copy of the mask.
func (m *Mask) Clone() *Mask {
	clone := NewMask(m.width, m.height)
	copy(clone.data, m.data)
	return clone
}

// Data returns the underlying mask data slice.
// This is useful for advanced operations.
func (m *Mask) Data() []uint8 {
	return m.data
}
