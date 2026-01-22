// Package raster provides scanline rasterization for 2D paths.
// This file implements SuperBlitter for anti-aliased rendering via 4x supersampling.
// Based on tiny-skia's path_aa.rs (Android/Skia heritage).
package raster

// SupersampleShift controls supersampling level: 2 means 4x (1 << 2 = 4).
const SupersampleShift = 2

// SupersampleScale is the number of subpixels per pixel (4 for 2-bit shift).
const SupersampleScale = 1 << SupersampleShift

// SupersampleMask is used to extract subpixel coordinates.
const SupersampleMask = SupersampleScale - 1

// batchThreshold is the minimum run length to use SIMD batch processing.
// For runs shorter than this, scalar processing is more efficient.
const batchThreshold = 16

// Blitter is an interface for receiving horizontal spans.
type Blitter interface {
	// BlitH blits a horizontal span at supersampled coordinates.
	BlitH(x, y uint32, width int)
}

// AAPixmap extends Pixmap with alpha-blended pixel writing.
type AAPixmap interface {
	Pixmap
	// BlendPixelAlpha blends a color with the existing pixel using given alpha.
	// alpha is in range 0-255.
	BlendPixelAlpha(x, y int, c RGBA, alpha uint8)
}

// AAPixmapBatch is an optional interface for pixmaps that support batch AA blending.
// Pixmaps implementing this interface can benefit from SIMD-optimized processing
// for runs of 16+ pixels with the same coverage alpha.
type AAPixmapBatch interface {
	AAPixmap
	// BlendSpanAlpha blends a solid color over a horizontal span with constant alpha.
	// Parameters:
	//   - x, y: starting pixel coordinates
	//   - count: number of pixels to blend
	//   - r, g, b, a: source color (premultiplied alpha, 0-255)
	//   - alpha: coverage alpha (0-255)
	BlendSpanAlpha(x, y, count int, r, g, b, a, alpha uint8)
}

// SuperBlitter accumulates supersampled coverage and blits AA pixels.
type SuperBlitter struct {
	pixmap AAPixmap
	color  RGBA
	runs   *AlphaRuns

	// Current destination y coordinate (in pixel space).
	currIY int
	// Width of the region being blitted (in pixel space).
	width int
	// Left edge x coordinate (in pixel space).
	left int
	// Left edge x coordinate (in supersampled space).
	superLeft uint32

	// Current y in supersampled coordinates.
	currY int
	// Top boundary (in pixel space).
	top int

	// Offset hint for AlphaRuns.Add.
	offsetX int

	// Cached batch-capable pixmap (nil if pixmap doesn't support batch ops).
	batchPixmap AAPixmapBatch

	// Premultiplied color bytes for batch operations.
	colorR, colorG, colorB, colorA uint8
}

// NewSuperBlitter creates a new SuperBlitter for AA rendering.
// bounds defines the pixel-space bounding box of the path.
// clipLeft, clipTop, clipRight, clipBottom define the clipping region.
func NewSuperBlitter(
	pixmap AAPixmap,
	color RGBA,
	boundsLeft, boundsTop, boundsRight, boundsBottom int,
	clipLeft, clipTop, clipRight, clipBottom int,
) *SuperBlitter {
	// Intersect bounds with clip
	left := max(boundsLeft, clipLeft)
	top := max(boundsTop, clipTop)
	right := min(boundsRight, clipRight)
	bottom := min(boundsBottom, clipBottom)

	if left >= right || top >= bottom {
		return nil // clipped out
	}

	width := right - left
	if width <= 0 {
		return nil
	}

	// Check if pixmap supports batch operations
	batchPixmap, _ := pixmap.(AAPixmapBatch)

	// Pre-compute premultiplied color bytes for batch operations
	// Color is in float64 0-1 range, convert to uint8 0-255
	colorR := uint8(clamp255(color.R * 255))
	colorG := uint8(clamp255(color.G * 255))
	colorB := uint8(clamp255(color.B * 255))
	colorA := uint8(clamp255(color.A * 255))

	return &SuperBlitter{
		pixmap:      pixmap,
		color:       color,
		runs:        NewAlphaRuns(width),
		currIY:      top - 1,
		width:       width,
		left:        left,
		superLeft:   uint32(left << SupersampleShift), //nolint:gosec // left bounded by clip
		currY:       (top << SupersampleShift) - 1,
		top:         top,
		offsetX:     0,
		batchPixmap: batchPixmap,
		colorR:      colorR,
		colorG:      colorG,
		colorB:      colorB,
		colorA:      colorA,
	}
}

// clamp255 clamps a float64 value to the range [0, 255].
func clamp255(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// BlitH implements Blitter interface for receiving supersampled spans.
func (sb *SuperBlitter) BlitH(x, y uint32, width int) {
	if width <= 0 {
		return
	}

	iy := int(y >> SupersampleShift)

	// Adjust x relative to superLeft
	if x < sb.superLeft {
		// Handle spans that start before our region
		diff := int(sb.superLeft - x)
		if diff >= width {
			return // entire span is outside
		}
		width -= diff
		x = sb.superLeft
	}
	x -= sb.superLeft

	// Reset offset when moving to new supersampled row
	if sb.currY != int(y) {
		sb.offsetX = 0
		sb.currY = int(y)
	}

	// Flush when moving to new pixel row
	if iy != sb.currIY {
		sb.Flush()
		sb.currIY = iy
	}

	start := x
	stop := x + uint32(width) //nolint:gosec // width is bounded by pixmap dimensions

	// Calculate partial coverage for start and end pixels
	fb := start & SupersampleMask // fractional part of start
	fe := stop & SupersampleMask  // fractional part of end
	n := int(stop>>SupersampleShift) - int(start>>SupersampleShift) - 1

	if n < 0 {
		// Start and end in same pixel
		fb = fe - fb
		n = 0
		fe = 0
	} else {
		if fb == 0 {
			n++
		} else {
			fb = SupersampleScale - fb
		}
	}

	// Calculate max alpha contribution based on y position
	// This accounts for the fact that only certain scanlines contribute
	// Result is bounded to 0-63 for SupersampleShift=2
	//nolint:gosec // bounded calculation, max result is 63
	maxValue := uint8((1 << (8 - SupersampleShift)) - (((y & SupersampleMask) + 1) >> SupersampleShift))

	sb.offsetX = sb.runs.Add(
		int(x>>SupersampleShift),
		coverageToPartialAlpha(fb),
		n,
		coverageToPartialAlpha(fe),
		maxValue,
		sb.offsetX,
	)
}

// Flush writes the accumulated coverage to the pixmap.
func (sb *SuperBlitter) Flush() {
	if sb.currIY < sb.top {
		return
	}

	if sb.runs.IsEmpty() {
		return
	}

	// Blit the accumulated alpha runs
	sb.blitAntiH(sb.left, sb.currIY)

	// Reset for next scanline
	sb.runs.Reset(sb.width)
	sb.offsetX = 0
	sb.currIY = sb.top - 1
}

// blitAntiH writes a row of anti-aliased pixels using the accumulated runs.
// For runs of 16+ pixels with the same alpha, SIMD batch processing is used.
func (sb *SuperBlitter) blitAntiH(x, y int) {
	runs := sb.runs.Runs()
	alpha := sb.runs.Alpha()

	// Check if we can use batch processing
	if sb.batchPixmap != nil {
		sb.blitAntiHBatch(x, y, runs, alpha)
		return
	}

	// Scalar fallback for pixmaps without batch support
	sb.blitAntiHScalar(x, y, runs, alpha)
}

// blitAntiHScalar writes pixels one at a time (fallback path).
func (sb *SuperBlitter) blitAntiHScalar(x, y int, runs []uint16, alpha []uint8) {
	i := 0
	for runs[i] > 0 {
		runLen := int(runs[i])
		a := alpha[i]

		if a > 0 {
			// Write pixels with this alpha value
			for j := 0; j < runLen; j++ {
				sb.pixmap.BlendPixelAlpha(x+i+j, y, sb.color, a)
			}
		}

		i += runLen
		if i >= len(runs) {
			break
		}
	}
}

// blitAntiHBatch writes pixels using SIMD batch processing when possible.
// Falls back to scalar for short runs.
func (sb *SuperBlitter) blitAntiHBatch(x, y int, runs []uint16, alpha []uint8) {
	i := 0
	for runs[i] > 0 {
		runLen := int(runs[i])
		a := alpha[i]

		if a > 0 {
			pixelX := x + i

			// For long runs, use batch processing
			if runLen >= batchThreshold {
				sb.batchPixmap.BlendSpanAlpha(
					pixelX, y, runLen,
					sb.colorR, sb.colorG, sb.colorB, sb.colorA, a,
				)
			} else {
				// Short runs: use scalar processing
				for j := 0; j < runLen; j++ {
					sb.pixmap.BlendPixelAlpha(pixelX+j, y, sb.color, a)
				}
			}
		}

		i += runLen
		if i >= len(runs) {
			break
		}
	}
}

// coverageToPartialAlpha converts fractional coverage to alpha contribution.
// The coverage is accumulated by AlphaRuns which handles clamping 256->255.
func coverageToPartialAlpha(coverage uint32) uint8 {
	// Scale coverage from 0-4 to alpha contribution
	// For SupersampleShift=2, coverage is 0-4, shifted by 4 bits -> 0-64
	aa := coverage << (8 - 2*SupersampleShift)
	return uint8(aa) //nolint:gosec // bounded by coverage 0-4 -> max 64
}
