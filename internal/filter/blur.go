package filter

import (
	"math"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// BlurFilter applies separable Gaussian blur to an image.
// The separable algorithm processes horizontal and vertical passes
// independently, achieving O(w*h*(rx+ry)) complexity instead of O(w*h*rx*ry).
type BlurFilter struct {
	// RadiusX is the horizontal blur radius in pixels.
	RadiusX float64

	// RadiusY is the vertical blur radius in pixels.
	RadiusY float64
}

// NewBlurFilter creates a new blur filter with equal radius in both directions.
func NewBlurFilter(radius float64) *BlurFilter {
	return &BlurFilter{
		RadiusX: radius,
		RadiusY: radius,
	}
}

// NewBlurFilterXY creates a new blur filter with different X and Y radii.
// This allows for anisotropic (directional) blur effects.
func NewBlurFilterXY(radiusX, radiusY float64) *BlurFilter {
	return &BlurFilter{
		RadiusX: radiusX,
		RadiusY: radiusY,
	}
}

// Apply applies the Gaussian blur to src and writes the result to dst.
// The operation uses a two-pass separable algorithm:
//  1. Horizontal pass: convolve each row with 1D kernel
//  2. Vertical pass: convolve each column with 1D kernel
func (f *BlurFilter) Apply(src, dst *gg.Pixmap, bounds scene.Rect) {
	if src == nil || dst == nil {
		return
	}

	// Handle zero radius (identity)
	if f.RadiusX <= 0 && f.RadiusY <= 0 {
		copyPixmapRegion(src, dst, bounds)
		return
	}

	// Get bounds in pixel coordinates
	minX := clampInt(int(bounds.MinX), 0, src.Width())
	maxX := clampInt(int(bounds.MaxX), 0, src.Width())
	minY := clampInt(int(bounds.MinY), 0, src.Height())
	maxY := clampInt(int(bounds.MaxY), 0, src.Height())

	if minX >= maxX || minY >= maxY {
		return
	}

	width := maxX - minX
	height := maxY - minY

	// Get temporary buffer from pool
	temp := getTempBuffer(width, height)
	defer putTempBuffer(temp)

	// Generate kernels
	kernelX := CachedGaussianKernel(f.RadiusX)
	kernelY := CachedGaussianKernel(f.RadiusY)

	// Pass 1: Horizontal blur (src -> temp)
	if f.RadiusX > 0 {
		blurHorizontal(src, temp, minX, minY, width, height, kernelX)
	} else {
		copyToTemp(src, temp, minX, minY, width, height)
	}

	// Pass 2: Vertical blur (temp -> dst)
	if f.RadiusY > 0 {
		blurVertical(temp, dst, minX, minY, width, height, kernelY)
	} else {
		copyFromTemp(temp, dst, minX, minY, width, height)
	}
}

// ExpandBounds returns the expanded bounds after blur application.
// Blur expands the output region by the kernel radius in all directions.
func (f *BlurFilter) ExpandBounds(input scene.Rect) scene.Rect {
	expandX := float32(math.Ceil(f.RadiusX * 3))
	expandY := float32(math.Ceil(f.RadiusY * 3))

	return scene.Rect{
		MinX: input.MinX - expandX,
		MinY: input.MinY - expandY,
		MaxX: input.MaxX + expandX,
		MaxY: input.MaxY + expandY,
	}
}

// blurHorizontal applies 1D horizontal convolution.
// Reads from src, writes to temp buffer.
func blurHorizontal(src *gg.Pixmap, temp []float32, minX, minY, width, height int, kernel []float32) {
	kernelSize := len(kernel)
	halfKernel := kernelSize / 2
	srcWidth := src.Width()
	srcData := src.Data()

	for y := 0; y < height; y++ {
		srcY := minY + y
		if srcY < 0 || srcY >= src.Height() {
			continue
		}

		for x := 0; x < width; x++ {
			srcX := minX + x

			var r, g, b, a float32

			for k := 0; k < kernelSize; k++ {
				kx := srcX + k - halfKernel

				// Clamp to source bounds (edge extension)
				if kx < 0 {
					kx = 0
				} else if kx >= srcWidth {
					kx = srcWidth - 1
				}

				srcIdx := (srcY*srcWidth + kx) * 4
				weight := kernel[k]

				r += float32(srcData[srcIdx+0]) * weight
				g += float32(srcData[srcIdx+1]) * weight
				b += float32(srcData[srcIdx+2]) * weight
				a += float32(srcData[srcIdx+3]) * weight
			}

			// Store in temp buffer (RGBA float32)
			tempIdx := (y*width + x) * 4
			temp[tempIdx+0] = r
			temp[tempIdx+1] = g
			temp[tempIdx+2] = b
			temp[tempIdx+3] = a
		}
	}
}

// blurVertical applies 1D vertical convolution.
// Reads from temp buffer, writes to dst.
func blurVertical(temp []float32, dst *gg.Pixmap, minX, minY, width, height int, kernel []float32) {
	kernelSize := len(kernel)
	halfKernel := kernelSize / 2
	dstData := dst.Data()
	dstWidth := dst.Width()

	for y := 0; y < height; y++ {
		dstY := minY + y
		if dstY < 0 || dstY >= dst.Height() {
			continue
		}

		for x := 0; x < width; x++ {
			dstX := minX + x
			if dstX < 0 || dstX >= dstWidth {
				continue
			}

			var r, g, b, a float32

			for k := 0; k < kernelSize; k++ {
				ky := y + k - halfKernel

				// Clamp to temp buffer bounds (edge extension)
				if ky < 0 {
					ky = 0
				} else if ky >= height {
					ky = height - 1
				}

				tempIdx := (ky*width + x) * 4
				weight := kernel[k]

				r += temp[tempIdx+0] * weight
				g += temp[tempIdx+1] * weight
				b += temp[tempIdx+2] * weight
				a += temp[tempIdx+3] * weight
			}

			// Write to destination (convert back to uint8)
			dstIdx := (dstY*dstWidth + dstX) * 4
			dstData[dstIdx+0] = clampUint8(r)
			dstData[dstIdx+1] = clampUint8(g)
			dstData[dstIdx+2] = clampUint8(b)
			dstData[dstIdx+3] = clampUint8(a)
		}
	}
}

// copyToTemp copies pixels from src to temp buffer.
func copyToTemp(src *gg.Pixmap, temp []float32, minX, minY, width, height int) {
	srcData := src.Data()
	srcWidth := src.Width()

	for y := 0; y < height; y++ {
		srcY := minY + y
		if srcY < 0 || srcY >= src.Height() {
			continue
		}

		for x := 0; x < width; x++ {
			srcX := minX + x
			if srcX < 0 || srcX >= srcWidth {
				continue
			}

			srcIdx := (srcY*srcWidth + srcX) * 4
			tempIdx := (y*width + x) * 4

			temp[tempIdx+0] = float32(srcData[srcIdx+0])
			temp[tempIdx+1] = float32(srcData[srcIdx+1])
			temp[tempIdx+2] = float32(srcData[srcIdx+2])
			temp[tempIdx+3] = float32(srcData[srcIdx+3])
		}
	}
}

// copyFromTemp copies pixels from temp buffer to dst.
func copyFromTemp(temp []float32, dst *gg.Pixmap, minX, minY, width, height int) {
	dstData := dst.Data()
	dstWidth := dst.Width()

	for y := 0; y < height; y++ {
		dstY := minY + y
		if dstY < 0 || dstY >= dst.Height() {
			continue
		}

		for x := 0; x < width; x++ {
			dstX := minX + x
			if dstX < 0 || dstX >= dstWidth {
				continue
			}

			tempIdx := (y*width + x) * 4
			dstIdx := (dstY*dstWidth + dstX) * 4

			dstData[dstIdx+0] = clampUint8(temp[tempIdx+0])
			dstData[dstIdx+1] = clampUint8(temp[tempIdx+1])
			dstData[dstIdx+2] = clampUint8(temp[tempIdx+2])
			dstData[dstIdx+3] = clampUint8(temp[tempIdx+3])
		}
	}
}

// copyPixmapRegion copies pixels from src to dst within bounds.
func copyPixmapRegion(src, dst *gg.Pixmap, bounds scene.Rect) {
	minX := clampInt(int(bounds.MinX), 0, src.Width())
	maxX := clampInt(int(bounds.MaxX), 0, src.Width())
	minY := clampInt(int(bounds.MinY), 0, src.Height())
	maxY := clampInt(int(bounds.MaxY), 0, src.Height())

	if maxX > dst.Width() {
		maxX = dst.Width()
	}
	if maxY > dst.Height() {
		maxY = dst.Height()
	}

	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			dst.SetPixel(x, y, src.GetPixel(x, y))
		}
	}
}

// floatBuffer wraps a slice for sync.Pool to avoid allocation warnings.
type floatBuffer struct {
	data []float32
}

// Temporary buffer pool for blur operations.
var tempBufferPool = sync.Pool{
	New: func() interface{} {
		// Start with a reasonable default size
		return &floatBuffer{data: make([]float32, 1024*1024*4)} // ~16MB for 1024x1024 RGBA
	},
}

// getTempBuffer retrieves a temporary buffer from the pool.
// The buffer is guaranteed to have at least width*height*4 elements.
func getTempBuffer(width, height int) []float32 {
	size := width * height * 4
	wrapper := tempBufferPool.Get().(*floatBuffer)

	if len(wrapper.data) < size {
		// Need larger buffer - return old one and allocate new
		tempBufferPool.Put(wrapper)
		return make([]float32, size)
	}

	// Clear the portion we'll use
	for i := 0; i < size; i++ {
		wrapper.data[i] = 0
	}

	return wrapper.data[:size]
}

// putTempBuffer returns a temporary buffer to the pool.
func putTempBuffer(buf []float32) {
	// Only pool reasonably-sized buffers
	if cap(buf) <= 16*1024*1024 { // 64MB max
		tempBufferPool.Put(&floatBuffer{data: buf[:cap(buf)]})
	}
}

// clampInt clamps an integer to [minVal, maxVal).
func clampInt(v, minVal, maxVal int) int {
	if v < minVal {
		return minVal
	}
	if v >= maxVal {
		return maxVal
	}
	return v
}

// clampUint8 clamps a float32 to [0, 255] and converts to uint8.
func clampUint8(v float32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5) // Round to nearest
}
