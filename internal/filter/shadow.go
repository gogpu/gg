package filter

import (
	"math"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// DropShadowFilter creates a drop shadow effect beneath an image.
// The filter extracts the alpha channel, blurs it, colorizes it,
// and composites it under the original image with an offset.
type DropShadowFilter struct {
	// OffsetX is the horizontal shadow offset in pixels.
	OffsetX float64

	// OffsetY is the vertical shadow offset in pixels.
	OffsetY float64

	// BlurRadius is the shadow blur radius in pixels.
	BlurRadius float64

	// Color is the shadow color (typically black with partial alpha).
	Color gg.RGBA
}

// NewDropShadowFilter creates a new drop shadow filter.
// Common usage: NewDropShadowFilter(3, 3, 5, gg.RGBA2(0, 0, 0, 0.5))
func NewDropShadowFilter(offsetX, offsetY, blurRadius float64, color gg.RGBA) *DropShadowFilter {
	return &DropShadowFilter{
		OffsetX:    offsetX,
		OffsetY:    offsetY,
		BlurRadius: blurRadius,
		Color:      color,
	}
}

// NewSimpleDropShadow creates a drop shadow with default black color at 50% opacity.
func NewSimpleDropShadow(offsetX, offsetY, blurRadius float64) *DropShadowFilter {
	return &DropShadowFilter{
		OffsetX:    offsetX,
		OffsetY:    offsetY,
		BlurRadius: blurRadius,
		Color:      gg.RGBA2(0, 0, 0, 0.5),
	}
}

// Apply applies the drop shadow filter.
// The algorithm:
//  1. Extract alpha channel from source
//  2. Apply Gaussian blur to alpha
//  3. Colorize with shadow color
//  4. Offset the shadow
//  5. Composite shadow under original
func (f *DropShadowFilter) Apply(src, dst *gg.Pixmap, bounds scene.Rect) {
	if src == nil || dst == nil {
		return
	}

	// Calculate expanded bounds for shadow
	expandedBounds := f.ExpandBounds(bounds)

	// Get integer coordinates
	minX := clampInt(int(expandedBounds.MinX), 0, dst.Width())
	maxX := clampInt(int(expandedBounds.MaxX), 0, dst.Width())
	minY := clampInt(int(expandedBounds.MinY), 0, dst.Height())
	maxY := clampInt(int(expandedBounds.MaxY), 0, dst.Height())

	if minX >= maxX || minY >= maxY {
		return
	}

	width := maxX - minX
	height := maxY - minY

	// Step 1: Extract alpha channel from source (with offset applied)
	alphaBuffer := getAlphaBuffer(width, height)
	defer putAlphaBuffer(alphaBuffer)

	extractAlpha(src, alphaBuffer, minX, minY, width, height, int(f.OffsetX), int(f.OffsetY))

	// Step 2: Blur the alpha channel if blur radius > 0
	if f.BlurRadius > 0 {
		blurredAlpha := getAlphaBuffer(width, height)
		defer putAlphaBuffer(blurredAlpha)

		blurAlphaChannel(alphaBuffer, blurredAlpha, width, height, f.BlurRadius)
		copy(alphaBuffer, blurredAlpha)
	}

	// Step 3 & 4 & 5: Colorize shadow and composite under original
	compositeShadow(src, dst, alphaBuffer, minX, minY, width, height, f.Color)
}

// ExpandBounds returns the expanded bounds after shadow application.
// Shadow expands by offset + blur radius in all directions.
func (f *DropShadowFilter) ExpandBounds(input scene.Rect) scene.Rect {
	blurExpand := float32(math.Ceil(f.BlurRadius * 3))

	// Calculate expansion in each direction
	expandLeft := blurExpand
	expandRight := blurExpand
	expandTop := blurExpand
	expandBottom := blurExpand

	// Add offset expansion
	if f.OffsetX < 0 {
		expandLeft += float32(-f.OffsetX)
	} else {
		expandRight += float32(f.OffsetX)
	}

	if f.OffsetY < 0 {
		expandTop += float32(-f.OffsetY)
	} else {
		expandBottom += float32(f.OffsetY)
	}

	return scene.Rect{
		MinX: input.MinX - expandLeft,
		MinY: input.MinY - expandTop,
		MaxX: input.MaxX + expandRight,
		MaxY: input.MaxY + expandBottom,
	}
}

// extractAlpha extracts the alpha channel from source to a float32 buffer.
// The offset is applied during extraction.
func extractAlpha(src *gg.Pixmap, alpha []float32, minX, minY, width, height, offsetX, offsetY int) {
	srcWidth := src.Width()
	srcHeight := src.Height()
	srcData := src.Data()

	for y := 0; y < height; y++ {
		// Source Y with offset subtracted (shadow is drawn offset FROM source)
		srcY := minY + y - offsetY

		for x := 0; x < width; x++ {
			srcX := minX + x - offsetX

			alphaIdx := y*width + x

			// Check bounds
			if srcX < 0 || srcX >= srcWidth || srcY < 0 || srcY >= srcHeight {
				alpha[alphaIdx] = 0
				continue
			}

			srcIdx := (srcY*srcWidth + srcX) * 4
			alpha[alphaIdx] = float32(srcData[srcIdx+3]) / 255.0
		}
	}
}

// blurAlphaChannel applies Gaussian blur to a single-channel alpha buffer.
func blurAlphaChannel(src, dst []float32, width, height int, radius float64) {
	kernel := CachedGaussianKernel(radius)
	kernelSize := len(kernel)
	halfKernel := kernelSize / 2

	// Temporary buffer for horizontal pass
	temp := getAlphaBuffer(width * height)
	defer putAlphaBuffer(temp)

	// Horizontal pass
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var sum float32

			for k := 0; k < kernelSize; k++ {
				kx := x + k - halfKernel

				// Edge extension
				if kx < 0 {
					kx = 0
				} else if kx >= width {
					kx = width - 1
				}

				sum += src[y*width+kx] * kernel[k]
			}

			temp[y*width+x] = sum
		}
	}

	// Vertical pass
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var sum float32

			for k := 0; k < kernelSize; k++ {
				ky := y + k - halfKernel

				// Edge extension
				if ky < 0 {
					ky = 0
				} else if ky >= height {
					ky = height - 1
				}

				sum += temp[ky*width+x] * kernel[k]
			}

			dst[y*width+x] = sum
		}
	}
}

// compositeShadow colorizes and composites shadow under the original image.
// Note: offset is already applied during alpha extraction.
func compositeShadow(src, dst *gg.Pixmap, shadowAlpha []float32,
	minX, minY, width, height int, color gg.RGBA) {
	srcWidth := src.Width()
	srcHeight := src.Height()
	srcData := src.Data()
	dstWidth := dst.Width()
	dstData := dst.Data()

	// Precompute shadow color components (premultiplied)
	shadowR := uint8(clamp255f(color.R * 255))
	shadowG := uint8(clamp255f(color.G * 255))
	shadowB := uint8(clamp255f(color.B * 255))
	shadowBaseA := float32(color.A)

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

			// Get shadow alpha
			shadowA := shadowAlpha[y*width+x] * shadowBaseA

			// Get source pixel (at non-offset position)
			srcX := dstX
			srcY := dstY

			var srcR, srcG, srcB, srcA uint8
			if srcX >= 0 && srcX < srcWidth && srcY >= 0 && srcY < srcHeight {
				srcIdx := (srcY*srcWidth + srcX) * 4
				srcR = srcData[srcIdx+0]
				srcG = srcData[srcIdx+1]
				srcB = srcData[srcIdx+2]
				srcA = srcData[srcIdx+3]
			}

			// Composite: shadow under source
			// Using source-over: Result = Src + Dst * (1 - SrcA)
			// Here we do: shadow first, then source over shadow

			// Shadow layer (premultiplied)
			sR := float32(shadowR) * shadowA
			sG := float32(shadowG) * shadowA
			sB := float32(shadowB) * shadowA
			sA := shadowA * 255

			// Source over shadow
			srcAlphaF := float32(srcA) / 255.0
			invSrcA := 1.0 - srcAlphaF

			finalR := float32(srcR) + sR*invSrcA
			finalG := float32(srcG) + sG*invSrcA
			finalB := float32(srcB) + sB*invSrcA
			finalA := float32(srcA) + sA*invSrcA

			// Write to destination
			dstIdx := (dstY*dstWidth + dstX) * 4
			dstData[dstIdx+0] = clampUint8(finalR)
			dstData[dstIdx+1] = clampUint8(finalG)
			dstData[dstIdx+2] = clampUint8(finalB)
			dstData[dstIdx+3] = clampUint8(finalA)
		}
	}
}

func getAlphaBuffer(size ...int) []float32 {
	totalSize := 1
	for _, s := range size {
		totalSize *= s
	}

	// For small buffers, just allocate
	if totalSize <= 0 {
		return nil
	}

	return make([]float32, totalSize)
}

func putAlphaBuffer(_ []float32) {
	// Simple implementation: let GC handle it
	// For production, would implement proper pooling
}

// clamp255f clamps a float64 to [0, 255] range.
func clamp255f(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 255 {
		return 255
	}
	return x
}
