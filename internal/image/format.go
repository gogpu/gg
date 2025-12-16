// Package image provides image buffer management for gogpu/gg.
//
// This package implements enterprise-grade image handling with support for
// multiple pixel formats, lazy premultiplication, and memory-efficient operations.
package image

// Format represents a pixel storage format.
type Format uint8

const (
	// FormatGray8 is 8-bit grayscale (1 byte per pixel).
	FormatGray8 Format = iota

	// FormatGray16 is 16-bit grayscale (2 bytes per pixel).
	FormatGray16

	// FormatRGB8 is 24-bit RGB (3 bytes per pixel, no alpha).
	FormatRGB8

	// FormatRGBA8 is 32-bit RGBA in sRGB color space (4 bytes per pixel).
	// This is the standard format for most operations.
	FormatRGBA8

	// FormatRGBAPremul is 32-bit RGBA with premultiplied alpha (4 bytes per pixel).
	// Used for correct alpha blending operations.
	FormatRGBAPremul

	// FormatBGRA8 is 32-bit BGRA in sRGB color space (4 bytes per pixel).
	// Common on Windows and some GPU formats.
	FormatBGRA8

	// FormatBGRAPremul is 32-bit BGRA with premultiplied alpha (4 bytes per pixel).
	FormatBGRAPremul

	// formatCount is the number of formats (for internal use).
	formatCount
)

// FormatInfo contains metadata about a pixel format.
type FormatInfo struct {
	// BytesPerPixel is the number of bytes per pixel.
	BytesPerPixel int

	// Channels is the number of color channels.
	Channels int

	// HasAlpha indicates if the format has an alpha channel.
	HasAlpha bool

	// IsPremultiplied indicates if alpha is premultiplied.
	IsPremultiplied bool

	// IsGrayscale indicates if this is a grayscale format.
	IsGrayscale bool

	// BitsPerChannel is the number of bits per color channel.
	BitsPerChannel int
}

// formatInfoTable contains metadata for each format.
var formatInfoTable = [formatCount]FormatInfo{
	FormatGray8: {
		BytesPerPixel:   1,
		Channels:        1,
		HasAlpha:        false,
		IsPremultiplied: false,
		IsGrayscale:     true,
		BitsPerChannel:  8,
	},
	FormatGray16: {
		BytesPerPixel:   2,
		Channels:        1,
		HasAlpha:        false,
		IsPremultiplied: false,
		IsGrayscale:     true,
		BitsPerChannel:  16,
	},
	FormatRGB8: {
		BytesPerPixel:   3,
		Channels:        3,
		HasAlpha:        false,
		IsPremultiplied: false,
		IsGrayscale:     false,
		BitsPerChannel:  8,
	},
	FormatRGBA8: {
		BytesPerPixel:   4,
		Channels:        4,
		HasAlpha:        true,
		IsPremultiplied: false,
		IsGrayscale:     false,
		BitsPerChannel:  8,
	},
	FormatRGBAPremul: {
		BytesPerPixel:   4,
		Channels:        4,
		HasAlpha:        true,
		IsPremultiplied: true,
		IsGrayscale:     false,
		BitsPerChannel:  8,
	},
	FormatBGRA8: {
		BytesPerPixel:   4,
		Channels:        4,
		HasAlpha:        true,
		IsPremultiplied: false,
		IsGrayscale:     false,
		BitsPerChannel:  8,
	},
	FormatBGRAPremul: {
		BytesPerPixel:   4,
		Channels:        4,
		HasAlpha:        true,
		IsPremultiplied: true,
		IsGrayscale:     false,
		BitsPerChannel:  8,
	},
}

// Info returns the FormatInfo for this format.
func (f Format) Info() FormatInfo {
	if f >= formatCount {
		return FormatInfo{}
	}
	return formatInfoTable[f]
}

// BytesPerPixel returns the number of bytes per pixel for this format.
func (f Format) BytesPerPixel() int {
	return f.Info().BytesPerPixel
}

// Channels returns the number of color channels.
func (f Format) Channels() int {
	return f.Info().Channels
}

// HasAlpha returns true if this format has an alpha channel.
func (f Format) HasAlpha() bool {
	return f.Info().HasAlpha
}

// IsPremultiplied returns true if alpha is premultiplied.
func (f Format) IsPremultiplied() bool {
	return f.Info().IsPremultiplied
}

// IsGrayscale returns true if this is a grayscale format.
func (f Format) IsGrayscale() bool {
	return f.Info().IsGrayscale
}

// BitsPerChannel returns the number of bits per color channel.
func (f Format) BitsPerChannel() int {
	return f.Info().BitsPerChannel
}

// String returns a string representation of the format.
func (f Format) String() string {
	switch f {
	case FormatGray8:
		return "Gray8"
	case FormatGray16:
		return "Gray16"
	case FormatRGB8:
		return "RGB8"
	case FormatRGBA8:
		return "RGBA8"
	case FormatRGBAPremul:
		return "RGBAPremul"
	case FormatBGRA8:
		return "BGRA8"
	case FormatBGRAPremul:
		return "BGRAPremul"
	default:
		return "Unknown"
	}
}

// IsValid returns true if the format is a valid known format.
func (f Format) IsValid() bool {
	return f < formatCount
}

// RowBytes calculates the number of bytes needed for a row of the given width.
func (f Format) RowBytes(width int) int {
	return width * f.BytesPerPixel()
}

// ImageBytes calculates the total number of bytes needed for an image.
func (f Format) ImageBytes(width, height int) int {
	return f.RowBytes(width) * height
}

// PremultipliedVersion returns the premultiplied version of this format.
// Returns the same format if already premultiplied or has no alpha.
func (f Format) PremultipliedVersion() Format {
	switch f {
	case FormatRGBA8:
		return FormatRGBAPremul
	case FormatBGRA8:
		return FormatBGRAPremul
	default:
		return f
	}
}

// UnpremultipliedVersion returns the non-premultiplied version of this format.
// Returns the same format if already non-premultiplied or has no alpha.
func (f Format) UnpremultipliedVersion() Format {
	switch f {
	case FormatRGBAPremul:
		return FormatRGBA8
	case FormatBGRAPremul:
		return FormatBGRA8
	default:
		return f
	}
}
