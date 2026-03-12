package text

// LCDLayout describes the physical subpixel arrangement on the display.
// Most LCD monitors use horizontal RGB stripe ordering, where each pixel
// consists of three vertical subpixel columns (red, green, blue) from
// left to right. ClearType-style rendering exploits this to triple the
// effective horizontal resolution for text.
type LCDLayout int

const (
	// LCDLayoutNone disables subpixel rendering (grayscale fallback).
	LCDLayoutNone LCDLayout = iota

	// LCDLayoutRGB is horizontal RGB ordering (most common: Windows, most monitors).
	// Physical subpixels left-to-right: Red, Green, Blue.
	LCDLayoutRGB

	// LCDLayoutBGR is horizontal BGR ordering (rare, some monitors).
	// Physical subpixels left-to-right: Blue, Green, Red.
	LCDLayoutBGR
)

// String returns the string representation of the LCD layout.
func (l LCDLayout) String() string {
	switch l {
	case LCDLayoutNone:
		return noneStr
	case LCDLayoutRGB:
		return "RGB"
	case LCDLayoutBGR:
		return "BGR"
	default:
		return unknownStr
	}
}

// LCDFilter applies a 5-tap FIR (Finite Impulse Response) filter to a
// 3x-oversampled coverage buffer to reduce color fringing in ClearType
// subpixel rendering.
//
// The filter convolves each subpixel column with 5 weights, distributing
// energy from adjacent subpixels to reduce chromatic artifacts at glyph
// edges. This is the same approach used by FreeType's LCD filtering.
//
// The default weights [0.08, 0.24, 0.36, 0.24, 0.08] (sum = 1.0) provide
// a good balance between sharpness and fringe reduction ("light" filter).
type LCDFilter struct {
	// Weights are the 5-tap FIR filter coefficients, centered on the
	// current subpixel column. Weights[2] is the center tap.
	Weights [5]float32
}

// DefaultLCDFilter returns the FreeType-compatible "light" LCD filter.
// Weights: [0.08, 0.24, 0.36, 0.24, 0.08] (sum = 1.0).
// This provides good fringe reduction without excessive blurring.
func DefaultLCDFilter() LCDFilter {
	return LCDFilter{Weights: [5]float32{0.08, 0.24, 0.36, 0.24, 0.08}}
}

// Apply runs the 5-tap horizontal FIR filter on a 3x-oversampled R8 buffer,
// producing per-channel RGB coverage values.
//
// Parameters:
//   - dst: output buffer, must be at least width*3 bytes (RGB triplets)
//   - src: input buffer of 3*width alpha samples (3x horizontal oversampling)
//   - width: number of output pixels (dst has width*3 bytes, src has 3*width bytes)
//
// For each output pixel at position i, the three subpixel columns are at
// src indices i*3+0 (red), i*3+1 (green), i*3+2 (blue). The 5-tap filter
// is centered on each subpixel column independently. Out-of-bounds samples
// are treated as zero.
func (f *LCDFilter) Apply(dst []byte, src []byte, width int) {
	srcLen := 3 * width
	if len(dst) < width*3 || len(src) < srcLen {
		return
	}

	for i := range width {
		// For each output pixel, filter 3 subpixel columns independently.
		for ch := range 3 {
			center := i*3 + ch // index in the 3x-wide source buffer

			var acc float32
			for tap := range 5 {
				srcIdx := center + tap - 2 // 5-tap centered: -2, -1, 0, +1, +2
				if srcIdx >= 0 && srcIdx < srcLen {
					acc += f.Weights[tap] * float32(src[srcIdx])
				}
			}

			// Clamp to [0, 255]
			v := int(acc + 0.5)
			if v < 0 {
				v = 0
			}
			if v > 255 {
				v = 255
			}
			dst[i*3+ch] = byte(v) //nolint:gosec // v is clamped to [0,255]
		}
	}
}

// LCDMaskResult holds the output of LCD subpixel rasterization.
type LCDMaskResult struct {
	// Mask is the RGB coverage buffer (3 bytes per pixel: R, G, B).
	// Row-major, width x height pixels, 3*width bytes per row.
	Mask []byte

	// Width and Height of the mask in pixels (NOT the 3x oversampled width).
	Width, Height int

	// BearingX is the horizontal offset from the glyph origin to the left
	// edge of the mask bounding box, in pixels.
	BearingX float32

	// BearingY is the vertical offset from the baseline to the top edge
	// of the mask bounding box, in pixels. Positive = above baseline.
	BearingY float32
}
