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

// applyMaskToPixmapData applies DestinationIn blending to a pixmap using a mask.
// For each pixel: all premultiplied channels are scaled by mask.At(x,y) / 255.
// Pixels outside the mask bounds are cleared to transparent.
func applyMaskToPixmapData(pm *Pixmap, mask *Mask) {
	data := pm.Data()
	pw, ph := pm.Width(), pm.Height()
	mw, mh := mask.Width(), mask.Height()

	maxX := pw
	if mw < maxX {
		maxX = mw
	}
	maxY := ph
	if mh < maxY {
		maxY = mh
	}

	for y := 0; y < maxY; y++ {
		for x := 0; x < maxX; x++ {
			mv := uint16(mask.At(x, y))
			if mv == 255 {
				continue // fully visible, no change
			}
			idx := (y*pw + x) * 4
			if mv == 0 {
				// Fully masked out: clear pixel.
				data[idx+0] = 0
				data[idx+1] = 0
				data[idx+2] = 0
				data[idx+3] = 0
				continue
			}
			// DestinationIn: dst = dst * maskAlpha / 255
			data[idx+0] = uint8(uint16(data[idx+0]) * mv / 255)
			data[idx+1] = uint8(uint16(data[idx+1]) * mv / 255)
			data[idx+2] = uint8(uint16(data[idx+2]) * mv / 255)
			data[idx+3] = uint8(uint16(data[idx+3]) * mv / 255)
		}
	}

	// Clear pixels outside mask bounds.
	if mw < pw {
		for y := 0; y < maxY; y++ {
			for x := mw; x < pw; x++ {
				idx := (y*pw + x) * 4
				data[idx+0] = 0
				data[idx+1] = 0
				data[idx+2] = 0
				data[idx+3] = 0
			}
		}
	}
	if mh < ph {
		for y := mh; y < ph; y++ {
			for x := 0; x < pw; x++ {
				idx := (y*pw + x) * 4
				data[idx+0] = 0
				data[idx+1] = 0
				data[idx+2] = 0
				data[idx+3] = 0
			}
		}
	}
}

// NewLuminanceMask creates a mask from an image using the CSS Masking Level 1
// luminance formula: Y = 0.2126*R + 0.7152*G + 0.0722*B. The luminance
// value is used directly as the mask alpha (brighter = more visible).
//
// This matches tiny-skia MaskType::Luminance and Vello Mask::new_luminance().
func NewLuminanceMask(img image.Image) *Mask {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	mask := NewMask(w, h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			// r, g, b are 0-65535. Compute luminance using CSS formula.
			// Y = 0.2126*R + 0.7152*G + 0.0722*B
			lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			// Scale from 0-65535 to 0-255.
			// #nosec G115 -- safe: lum/257 is always in range [0, 255]
			mask.data[y*w+x] = uint8(lum/257.0 + 0.5)
		}
	}

	return mask
}

// NewMaskFromData creates a mask from a raw byte slice.
// The data must contain exactly width*height bytes, where each byte
// represents the mask alpha at that pixel (row-major order).
// Returns nil if the data length does not match width*height.
func NewMaskFromData(data []byte, width, height int) *Mask {
	if len(data) != width*height {
		return nil
	}
	m := &Mask{
		width:  width,
		height: height,
		data:   make([]uint8, width*height),
	}
	copy(m.data, data)
	return m
}
