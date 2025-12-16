// Package image provides image buffer management for gogpu/gg.
//
// This package implements enterprise-grade image handling with support for
// multiple pixel formats, lazy premultiplication, and memory-efficient operations.
package image

import (
	"errors"
	"sync"
)

// Common errors for image operations.
var (
	// ErrInvalidDimensions is returned when width or height is non-positive.
	ErrInvalidDimensions = errors.New("image: invalid dimensions")

	// ErrInvalidFormat is returned when the format is not recognized.
	ErrInvalidFormat = errors.New("image: invalid format")

	// ErrInvalidStride is returned when stride is less than minimum required.
	ErrInvalidStride = errors.New("image: stride too small for width")

	// ErrDataTooSmall is returned when provided data is smaller than required.
	ErrDataTooSmall = errors.New("image: data buffer too small")

	// ErrOutOfBounds is returned when pixel coordinates are outside image bounds.
	ErrOutOfBounds = errors.New("image: coordinates out of bounds")
)

// ImageBuf is a memory-efficient image buffer with lazy premultiplication.
//
// ImageBuf stores pixel data in a contiguous byte slice with optional stride
// for memory alignment. It supports lazy premultiplication - the premultiplied
// version is computed only when needed and cached for reuse.
//
// Thread safety: ImageBuf is safe for concurrent read access. Write operations
// (Set*, Clear, InvalidatePremulCache) require external synchronization.
type ImageBuf struct {
	data   []byte
	width  int
	height int
	stride int
	format Format

	// Lazy premultiplication cache
	premulMu    sync.RWMutex
	premulReady bool
	premulData  []byte
}

// NewImageBuf creates a new image buffer with the given dimensions and format.
// Returns an error if dimensions are invalid or format is unknown.
func NewImageBuf(width, height int, format Format) (*ImageBuf, error) {
	if width <= 0 || height <= 0 {
		return nil, ErrInvalidDimensions
	}
	if !format.IsValid() {
		return nil, ErrInvalidFormat
	}

	stride := format.RowBytes(width)
	data := make([]byte, stride*height)

	return &ImageBuf{
		data:   data,
		width:  width,
		height: height,
		stride: stride,
		format: format,
	}, nil
}

// NewImageBufWithStride creates a new image buffer with custom stride for alignment.
// Stride must be at least format.RowBytes(width).
func NewImageBufWithStride(width, height int, format Format, stride int) (*ImageBuf, error) {
	if width <= 0 || height <= 0 {
		return nil, ErrInvalidDimensions
	}
	if !format.IsValid() {
		return nil, ErrInvalidFormat
	}

	minStride := format.RowBytes(width)
	if stride < minStride {
		return nil, ErrInvalidStride
	}

	data := make([]byte, stride*height)

	return &ImageBuf{
		data:   data,
		width:  width,
		height: height,
		stride: stride,
		format: format,
	}, nil
}

// FromRaw creates an ImageBuf from existing data without copying.
// The caller must ensure data remains valid for the lifetime of the ImageBuf.
// Stride must be at least format.RowBytes(width).
func FromRaw(data []byte, width, height int, format Format, stride int) (*ImageBuf, error) {
	if width <= 0 || height <= 0 {
		return nil, ErrInvalidDimensions
	}
	if !format.IsValid() {
		return nil, ErrInvalidFormat
	}

	minStride := format.RowBytes(width)
	if stride < minStride {
		return nil, ErrInvalidStride
	}

	requiredSize := stride * height
	if len(data) < requiredSize {
		return nil, ErrDataTooSmall
	}

	return &ImageBuf{
		data:   data[:requiredSize],
		width:  width,
		height: height,
		stride: stride,
		format: format,
	}, nil
}

// Clone creates a deep copy of the image buffer.
func (b *ImageBuf) Clone() *ImageBuf {
	newData := make([]byte, len(b.data))
	copy(newData, b.data)

	return &ImageBuf{
		data:   newData,
		width:  b.width,
		height: b.height,
		stride: b.stride,
		format: b.format,
		// premul cache is not copied - will be recomputed if needed
	}
}

// Width returns the image width in pixels.
func (b *ImageBuf) Width() int {
	return b.width
}

// Height returns the image height in pixels.
func (b *ImageBuf) Height() int {
	return b.height
}

// Stride returns the number of bytes per row (including padding).
func (b *ImageBuf) Stride() int {
	return b.stride
}

// Format returns the pixel format.
func (b *ImageBuf) Format() Format {
	return b.format
}

// Bounds returns the image dimensions as (width, height).
func (b *ImageBuf) Bounds() (int, int) {
	return b.width, b.height
}

// Data returns the raw pixel data slice.
// Modifying this data will affect the image; call InvalidatePremulCache()
// after modifications if premultiplied data may have been cached.
func (b *ImageBuf) Data() []byte {
	return b.data
}

// RowBytes returns a slice of the pixel data for row y.
// Returns nil if y is out of bounds.
func (b *ImageBuf) RowBytes(y int) []byte {
	if y < 0 || y >= b.height {
		return nil
	}
	start := y * b.stride
	end := start + b.format.RowBytes(b.width)
	return b.data[start:end]
}

// PixelOffset returns the byte offset of pixel (x, y) in the data slice.
// Returns -1 if coordinates are out of bounds.
func (b *ImageBuf) PixelOffset(x, y int) int {
	if x < 0 || x >= b.width || y < 0 || y >= b.height {
		return -1
	}
	return y*b.stride + x*b.format.BytesPerPixel()
}

// PixelBytes returns a slice of the raw bytes for pixel (x, y).
// Returns nil if coordinates are out of bounds.
func (b *ImageBuf) PixelBytes(x, y int) []byte {
	offset := b.PixelOffset(x, y)
	if offset < 0 {
		return nil
	}
	bpp := b.format.BytesPerPixel()
	return b.data[offset : offset+bpp]
}

// SetPixelBytes sets the raw bytes for pixel (x, y).
// Returns ErrOutOfBounds if coordinates are outside image bounds.
// Automatically invalidates the premultiplication cache.
func (b *ImageBuf) SetPixelBytes(x, y int, pixel []byte) error {
	offset := b.PixelOffset(x, y)
	if offset < 0 {
		return ErrOutOfBounds
	}
	bpp := b.format.BytesPerPixel()
	copy(b.data[offset:offset+bpp], pixel)
	b.InvalidatePremulCache()
	return nil
}

// GetRGBA returns the color at (x, y) as (r, g, b, a) in 0-255 range.
// For grayscale formats, r=g=b=gray and a=255.
// For formats without alpha, a=255.
// Returns (0,0,0,0) if coordinates are out of bounds.
func (b *ImageBuf) GetRGBA(x, y int) (r, g, bl, a uint8) {
	pixel := b.PixelBytes(x, y)
	if pixel == nil {
		return 0, 0, 0, 0
	}

	switch b.format {
	case FormatGray8:
		v := pixel[0]
		return v, v, v, 255
	case FormatGray16:
		v := pixel[1] // High byte (big endian for display purposes)
		return v, v, v, 255
	case FormatRGB8:
		return pixel[0], pixel[1], pixel[2], 255
	case FormatRGBA8, FormatRGBAPremul:
		return pixel[0], pixel[1], pixel[2], pixel[3]
	case FormatBGRA8, FormatBGRAPremul:
		return pixel[2], pixel[1], pixel[0], pixel[3]
	default:
		return 0, 0, 0, 0
	}
}

// SetRGBA sets the color at (x, y) from (r, g, b, a) in 0-255 range.
// For grayscale formats, uses standard luminance weights.
// Returns ErrOutOfBounds if coordinates are outside image bounds.
func (b *ImageBuf) SetRGBA(x, y int, r, g, bl, a uint8) error {
	offset := b.PixelOffset(x, y)
	if offset < 0 {
		return ErrOutOfBounds
	}

	switch b.format {
	case FormatGray8:
		// Standard luminance: 0.299*R + 0.587*G + 0.114*B
		gray := (int(r)*299 + int(g)*587 + int(bl)*114) / 1000
		b.data[offset] = byte(gray)
	case FormatGray16:
		gray := (int(r)*299 + int(g)*587 + int(bl)*114) / 1000
		b.data[offset] = byte(gray)
		b.data[offset+1] = byte(gray)
	case FormatRGB8:
		b.data[offset] = r
		b.data[offset+1] = g
		b.data[offset+2] = bl
	case FormatRGBA8, FormatRGBAPremul:
		b.data[offset] = r
		b.data[offset+1] = g
		b.data[offset+2] = bl
		b.data[offset+3] = a
	case FormatBGRA8, FormatBGRAPremul:
		b.data[offset] = bl
		b.data[offset+1] = g
		b.data[offset+2] = r
		b.data[offset+3] = a
	}

	b.InvalidatePremulCache()
	return nil
}

// Clear sets all pixels to zero (transparent black for RGBA formats).
func (b *ImageBuf) Clear() {
	clear(b.data)
	b.InvalidatePremulCache()
}

// Fill sets all pixels to the given RGBA color.
func (b *ImageBuf) Fill(r, g, bl, a uint8) {
	for y := range b.height {
		for x := range b.width {
			_ = b.SetRGBA(x, y, r, g, bl, a)
		}
	}
}

// InvalidatePremulCache marks the premultiplication cache as stale.
// Call this after modifying pixel data directly via Data() or RowBytes().
func (b *ImageBuf) InvalidatePremulCache() {
	b.premulMu.Lock()
	b.premulReady = false
	b.premulMu.Unlock()
}

// PremultipliedData returns the image data with premultiplied alpha.
// For formats already premultiplied or without alpha, returns the original data.
// The result is cached for efficiency; call InvalidatePremulCache() if the
// original data has been modified.
func (b *ImageBuf) PremultipliedData() []byte {
	// Fast path: format already premultiplied or has no alpha
	if b.format.IsPremultiplied() || !b.format.HasAlpha() {
		return b.data
	}

	// Check cache
	b.premulMu.RLock()
	if b.premulReady {
		data := b.premulData
		b.premulMu.RUnlock()
		return data
	}
	b.premulMu.RUnlock()

	// Compute premultiplied data
	b.premulMu.Lock()
	defer b.premulMu.Unlock()

	// Double-check after acquiring write lock
	if b.premulReady {
		return b.premulData
	}

	// Allocate or reuse premul buffer
	if len(b.premulData) != len(b.data) {
		b.premulData = make([]byte, len(b.data))
	}

	b.computePremultiplied()
	b.premulReady = true

	return b.premulData
}

// computePremultiplied fills premulData with premultiplied version.
// Caller must hold premulMu write lock.
func (b *ImageBuf) computePremultiplied() {
	bpp := b.format.BytesPerPixel()

	for y := range b.height {
		srcRow := y * b.stride
		for x := range b.width {
			offset := srcRow + x*bpp
			b.premulPixel(offset)
		}
	}
}

// premulPixel premultiplies a single pixel.
func (b *ImageBuf) premulPixel(offset int) {
	switch b.format {
	case FormatRGBA8:
		r := uint16(b.data[offset])
		g := uint16(b.data[offset+1])
		bl := uint16(b.data[offset+2])
		a := uint16(b.data[offset+3])

		// Premultiply: channel = channel * alpha / 255
		b.premulData[offset] = byte((r*a + 127) / 255)
		b.premulData[offset+1] = byte((g*a + 127) / 255)
		b.premulData[offset+2] = byte((bl*a + 127) / 255)
		b.premulData[offset+3] = byte(a)

	case FormatBGRA8:
		bl := uint16(b.data[offset])
		g := uint16(b.data[offset+1])
		r := uint16(b.data[offset+2])
		a := uint16(b.data[offset+3])

		b.premulData[offset] = byte((bl*a + 127) / 255)
		b.premulData[offset+1] = byte((g*a + 127) / 255)
		b.premulData[offset+2] = byte((r*a + 127) / 255)
		b.premulData[offset+3] = byte(a)
	}
}

// IsPremulCached returns true if premultiplied data is currently cached.
func (b *ImageBuf) IsPremulCached() bool {
	b.premulMu.RLock()
	ready := b.premulReady
	b.premulMu.RUnlock()
	return ready
}

// SubImage returns a view into a rectangular region of the image.
// The returned ImageBuf shares the underlying data with the original.
// Modifications to the sub-image affect the original and vice versa.
// Returns nil if the bounds are invalid or outside the image.
func (b *ImageBuf) SubImage(x, y, width, height int) *ImageBuf {
	// Validate bounds
	if x < 0 || y < 0 || width <= 0 || height <= 0 {
		return nil
	}
	if x+width > b.width || y+height > b.height {
		return nil
	}

	// Calculate new data slice starting at (x, y)
	offset := y*b.stride + x*b.format.BytesPerPixel()
	// Total bytes needed: (height-1)*stride + width*bpp
	endOffset := (y+height-1)*b.stride + (x+width)*b.format.BytesPerPixel()

	return &ImageBuf{
		data:   b.data[offset:endOffset],
		width:  width,
		height: height,
		stride: b.stride, // Keep original stride for proper row access
		format: b.format,
		// premul cache is not shared
	}
}

// ByteSize returns the total size of the image data in bytes.
func (b *ImageBuf) ByteSize() int {
	return len(b.data)
}

// IsEmpty returns true if the image has zero dimensions.
func (b *ImageBuf) IsEmpty() bool {
	return b.width == 0 || b.height == 0
}
