package image

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// I/O errors.
var (
	// ErrUnsupportedFormat is returned when the image format is not supported.
	ErrUnsupportedFormat = errors.New("image: unsupported format")

	// ErrEmptyData is returned when image data is empty.
	ErrEmptyData = errors.New("image: empty data")
)

// LoadPNG loads a PNG image from the given file path.
func LoadPNG(path string) (*ImageBuf, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("image: open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return DecodePNG(f)
}

// LoadJPEG loads a JPEG image from the given file path.
func LoadJPEG(path string) (*ImageBuf, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("image: open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return DecodeJPEG(f)
}

// LoadImage loads an image from the given file path, auto-detecting the format.
// Supported formats: PNG, JPEG.
func LoadImage(path string) (*ImageBuf, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return LoadPNG(path)
	case ".jpg", ".jpeg":
		return LoadJPEG(path)
	default:
		// Try to detect from content
		f, err := os.Open(filepath.Clean(path))
		if err != nil {
			return nil, fmt.Errorf("image: open file: %w", err)
		}
		defer func() { _ = f.Close() }()

		return Decode(f)
	}
}

// LoadImageFromBytes loads an image from a byte slice, auto-detecting the format.
func LoadImageFromBytes(data []byte) (*ImageBuf, error) {
	if len(data) == 0 {
		return nil, ErrEmptyData
	}
	return Decode(bytes.NewReader(data))
}

// Decode decodes an image from the given reader, auto-detecting the format.
func Decode(r io.Reader) (*ImageBuf, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("image: decode: %w", err)
	}

	_ = format // format string available if needed

	return FromStdImage(img), nil
}

// DecodePNG decodes a PNG image from the given reader.
func DecodePNG(r io.Reader) (*ImageBuf, error) {
	img, err := png.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("image: decode PNG: %w", err)
	}
	return FromStdImage(img), nil
}

// DecodeJPEG decodes a JPEG image from the given reader.
func DecodeJPEG(r io.Reader) (*ImageBuf, error) {
	img, err := jpeg.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("image: decode JPEG: %w", err)
	}
	return FromStdImage(img), nil
}

// SavePNG saves the image as a PNG file.
func (b *ImageBuf) SavePNG(path string) error {
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("image: create file: %w", err)
	}

	if err := b.EncodePNG(f); err != nil {
		_ = f.Close()
		return err
	}

	return f.Close()
}

// SaveJPEG saves the image as a JPEG file with the given quality (1-100).
func (b *ImageBuf) SaveJPEG(path string, quality int) error {
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("image: create file: %w", err)
	}

	if err := b.EncodeJPEG(f, quality); err != nil {
		_ = f.Close()
		return err
	}

	return f.Close()
}

// EncodePNG encodes the image as PNG to the given writer.
func (b *ImageBuf) EncodePNG(w io.Writer) error {
	img := b.ToStdImage()
	if err := png.Encode(w, img); err != nil {
		return fmt.Errorf("image: encode PNG: %w", err)
	}
	return nil
}

// EncodeJPEG encodes the image as JPEG to the given writer.
func (b *ImageBuf) EncodeJPEG(w io.Writer, quality int) error {
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}

	img := b.ToStdImage()
	if err := jpeg.Encode(w, img, &jpeg.Options{Quality: quality}); err != nil {
		return fmt.Errorf("image: encode JPEG: %w", err)
	}
	return nil
}

// FromStdImage creates an ImageBuf from a standard library image.Image.
// The resulting ImageBuf will be in RGBA8 format.
func FromStdImage(img image.Image) *ImageBuf {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	buf, _ := NewImageBuf(width, height, FormatRGBA8)

	// Fast path for RGBA images
	if rgba, ok := img.(*image.RGBA); ok {
		// Direct copy if stride matches
		if rgba.Stride == buf.Stride() {
			copy(buf.Data(), rgba.Pix)
			return buf
		}
		// Row-by-row copy for different strides
		for y := range height {
			srcStart := y * rgba.Stride
			copy(buf.RowBytes(y), rgba.Pix[srcStart:srcStart+width*4])
		}
		return buf
	}

	// Fast path for NRGBA images
	if nrgba, ok := img.(*image.NRGBA); ok {
		if nrgba.Stride == buf.Stride() {
			copy(buf.Data(), nrgba.Pix)
			return buf
		}
		for y := range height {
			srcStart := y * nrgba.Stride
			copy(buf.RowBytes(y), nrgba.Pix[srcStart:srcStart+width*4])
		}
		return buf
	}

	// Generic slow path for any image type
	for y := range height {
		for x := range width {
			c := img.At(bounds.Min.X+x, bounds.Min.Y+y)
			r, g, b, a := c.RGBA()
			// RGBA() returns 16-bit values, scale to 8-bit
			// Right shift by 8 guarantees result fits in uint8
			_ = buf.SetRGBA(x, y, byte(r>>8), byte(g>>8), byte(b>>8), byte(a>>8))
		}
	}

	return buf
}

// ToStdImage converts the ImageBuf to a standard library image.Image.
// Returns *image.RGBA for most formats, *image.Gray for grayscale.
func (b *ImageBuf) ToStdImage() image.Image {
	rect := image.Rect(0, 0, b.width, b.height)

	switch b.format {
	case FormatGray8:
		gray := image.NewGray(rect)
		for y := range b.height {
			row := b.RowBytes(y)
			copy(gray.Pix[y*gray.Stride:], row)
		}
		return gray

	case FormatGray16:
		gray16 := image.NewGray16(rect)
		for y := range b.height {
			row := b.RowBytes(y)
			dstStart := y * gray16.Stride
			for x := range b.width {
				// Gray16 in image package is big-endian
				gray16.Pix[dstStart+x*2] = row[x*2+1] // high byte
				gray16.Pix[dstStart+x*2+1] = row[x*2] // low byte
			}
		}
		return gray16

	case FormatRGBA8:
		// RGBA8 is non-premultiplied, use NRGBA
		nrgba := image.NewNRGBA(rect)
		if b.stride == nrgba.Stride {
			copy(nrgba.Pix, b.data)
		} else {
			for y := range b.height {
				copy(nrgba.Pix[y*nrgba.Stride:], b.RowBytes(y))
			}
		}
		return nrgba

	case FormatRGBAPremul:
		// RGBAPremul is premultiplied, use RGBA
		rgba := image.NewRGBA(rect)
		if b.stride == rgba.Stride {
			copy(rgba.Pix, b.data)
		} else {
			for y := range b.height {
				copy(rgba.Pix[y*rgba.Stride:], b.RowBytes(y))
			}
		}
		return rgba

	case FormatBGRA8:
		// Convert BGRA to NRGBA (non-premultiplied)
		nrgba := image.NewNRGBA(rect)
		for y := range b.height {
			row := b.RowBytes(y)
			dstStart := y * nrgba.Stride
			for x := range b.width {
				srcOff := x * 4
				dstOff := dstStart + x*4
				nrgba.Pix[dstOff] = row[srcOff+2]   // R <- B
				nrgba.Pix[dstOff+1] = row[srcOff+1] // G <- G
				nrgba.Pix[dstOff+2] = row[srcOff]   // B <- R
				nrgba.Pix[dstOff+3] = row[srcOff+3] // A <- A
			}
		}
		return nrgba

	case FormatBGRAPremul:
		// Convert BGRA to RGBA (premultiplied)
		rgba := image.NewRGBA(rect)
		for y := range b.height {
			row := b.RowBytes(y)
			dstStart := y * rgba.Stride
			for x := range b.width {
				srcOff := x * 4
				dstOff := dstStart + x*4
				rgba.Pix[dstOff] = row[srcOff+2]   // R <- B
				rgba.Pix[dstOff+1] = row[srcOff+1] // G <- G
				rgba.Pix[dstOff+2] = row[srcOff]   // B <- R
				rgba.Pix[dstOff+3] = row[srcOff+3] // A <- A
			}
		}
		return rgba

	case FormatRGB8:
		// Expand to NRGBA (opaque)
		nrgba := image.NewNRGBA(rect)
		for y := range b.height {
			row := b.RowBytes(y)
			dstStart := y * nrgba.Stride
			for x := range b.width {
				srcOff := x * 3
				dstOff := dstStart + x*4
				nrgba.Pix[dstOff] = row[srcOff]
				nrgba.Pix[dstOff+1] = row[srcOff+1]
				nrgba.Pix[dstOff+2] = row[srcOff+2]
				nrgba.Pix[dstOff+3] = 255 // Opaque
			}
		}
		return nrgba

	default:
		// Fallback using GetRGBA - assume non-premultiplied
		nrgba := image.NewNRGBA(rect)
		for y := range b.height {
			for x := range b.width {
				r, g, bl, a := b.GetRGBA(x, y)
				off := y*nrgba.Stride + x*4
				nrgba.Pix[off] = r
				nrgba.Pix[off+1] = g
				nrgba.Pix[off+2] = bl
				nrgba.Pix[off+3] = a
			}
		}
		return nrgba
	}
}

// EncodeToBytes encodes the image to PNG format and returns the bytes.
func (b *ImageBuf) EncodeToBytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := b.EncodePNG(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodeToJPEGBytes encodes the image to JPEG format and returns the bytes.
func (b *ImageBuf) EncodeToJPEGBytes(quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := b.EncodeJPEG(&buf, quality); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
