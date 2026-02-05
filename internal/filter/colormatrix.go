package filter

import (
	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// ColorMatrixFilter applies a 4x5 color transformation matrix to an image.
// The transformation is:
//
//	[R']   [a00 a01 a02 a03 a04]   [R]
//	[G'] = [a10 a11 a12 a13 a14] * [G]
//	[B']   [a20 a21 a22 a23 a24]   [B]
//	[A']   [a30 a31 a32 a33 a34]   [A]
//	                               [1]
//
// The fifth column provides bias/offset values.
// Color values are in [0, 255] range during transformation,
// then clamped back to valid range.
type ColorMatrixFilter struct {
	// Matrix is the 4x5 transformation matrix in row-major order.
	// [0-4] = row 0 (R), [5-9] = row 1 (G), [10-14] = row 2 (B), [15-19] = row 3 (A)
	Matrix [20]float32
}

// NewColorMatrixFilter creates a color matrix filter with the given matrix.
func NewColorMatrixFilter(matrix [20]float32) *ColorMatrixFilter {
	return &ColorMatrixFilter{Matrix: matrix}
}

// NewIdentityColorMatrix creates a color matrix filter that passes through unchanged.
func NewIdentityColorMatrix() *ColorMatrixFilter {
	return &ColorMatrixFilter{
		Matrix: [20]float32{
			1, 0, 0, 0, 0, // R
			0, 1, 0, 0, 0, // G
			0, 0, 1, 0, 0, // B
			0, 0, 0, 1, 0, // A
		},
	}
}

// NewBrightnessFilter creates a filter that adjusts brightness.
// factor: 0.0 = black, 1.0 = unchanged, 2.0 = twice as bright
func NewBrightnessFilter(factor float32) *ColorMatrixFilter {
	return &ColorMatrixFilter{
		Matrix: [20]float32{
			factor, 0, 0, 0, 0,
			0, factor, 0, 0, 0,
			0, 0, factor, 0, 0,
			0, 0, 0, 1, 0,
		},
	}
}

// NewContrastFilter creates a filter that adjusts contrast.
// factor: 0.0 = gray, 1.0 = unchanged, 2.0 = high contrast
func NewContrastFilter(factor float32) *ColorMatrixFilter {
	// Contrast adjustment: (color - 0.5) * factor + 0.5
	// In matrix form with 0-255 range: (color - 128) * factor + 128
	offset := 128 * (1 - factor)
	return &ColorMatrixFilter{
		Matrix: [20]float32{
			factor, 0, 0, 0, offset,
			0, factor, 0, 0, offset,
			0, 0, factor, 0, offset,
			0, 0, 0, 1, 0,
		},
	}
}

// NewSaturationFilter creates a filter that adjusts color saturation.
// factor: 0.0 = grayscale, 1.0 = unchanged, 2.0 = oversaturated
func NewSaturationFilter(factor float32) *ColorMatrixFilter {
	// Luminance weights (Rec. 709)
	const (
		lumR = 0.2126
		lumG = 0.7152
		lumB = 0.0722
	)

	// Saturation matrix blends between luminance (0) and identity (1)
	invFactor := 1 - factor

	return &ColorMatrixFilter{
		Matrix: [20]float32{
			lumR*invFactor + factor, lumG * invFactor, lumB * invFactor, 0, 0,
			lumR * invFactor, lumG*invFactor + factor, lumB * invFactor, 0, 0,
			lumR * invFactor, lumG * invFactor, lumB*invFactor + factor, 0, 0,
			0, 0, 0, 1, 0,
		},
	}
}

// NewGrayscaleFilter creates a filter that converts to grayscale.
// Uses Rec. 709 luminance weights.
func NewGrayscaleFilter() *ColorMatrixFilter {
	return NewSaturationFilter(0)
}

// NewSepiaFilter creates a filter that applies sepia tone effect.
func NewSepiaFilter() *ColorMatrixFilter {
	return &ColorMatrixFilter{
		Matrix: [20]float32{
			0.393, 0.769, 0.189, 0, 0,
			0.349, 0.686, 0.168, 0, 0,
			0.272, 0.534, 0.131, 0, 0,
			0, 0, 0, 1, 0,
		},
	}
}

// NewInvertFilter creates a filter that inverts colors.
func NewInvertFilter() *ColorMatrixFilter {
	return &ColorMatrixFilter{
		Matrix: [20]float32{
			-1, 0, 0, 0, 255,
			0, -1, 0, 0, 255,
			0, 0, -1, 0, 255,
			0, 0, 0, 1, 0,
		},
	}
}

// NewHueRotateFilter creates a filter that rotates hue by the given angle (in degrees).
func NewHueRotateFilter(degrees float32) *ColorMatrixFilter {
	// Convert to radians
	const degToRad = 0.017453292519943295 // pi/180
	rad := degrees * degToRad

	cos := float32(cosf(float64(rad)))
	sin := float32(sinf(float64(rad)))

	// Hue rotation matrix (approximation)
	// Based on rotating in YIQ color space
	const (
		lumR = 0.213
		lumG = 0.715
		lumB = 0.072
	)

	return &ColorMatrixFilter{
		Matrix: [20]float32{
			lumR + cos*(1-lumR) + sin*(-lumR), lumG + cos*(-lumG) + sin*(-lumG), lumB + cos*(-lumB) + sin*(1-lumB), 0, 0,
			lumR + cos*(-lumR) + sin*(0.143), lumG + cos*(1-lumG) + sin*(0.140), lumB + cos*(-lumB) + sin*(-0.283), 0, 0,
			lumR + cos*(-lumR) + sin*(-(1 - lumR)), lumG + cos*(-lumG) + sin*(lumG), lumB + cos*(1-lumB) + sin*(lumB), 0, 0,
			0, 0, 0, 1, 0,
		},
	}
}

// NewOpacityFilter creates a filter that multiplies alpha by the given factor.
// factor: 0.0 = fully transparent, 1.0 = unchanged
func NewOpacityFilter(factor float32) *ColorMatrixFilter {
	return &ColorMatrixFilter{
		Matrix: [20]float32{
			1, 0, 0, 0, 0,
			0, 1, 0, 0, 0,
			0, 0, 1, 0, 0,
			0, 0, 0, factor, 0,
		},
	}
}

// NewColorTintFilter creates a filter that tints the image with a color.
// The tint is blended with the original based on the color's alpha.
func NewColorTintFilter(tint gg.RGBA) *ColorMatrixFilter {
	// Blend factor from tint alpha
	f := float32(tint.A)
	invF := 1 - f

	// Tint color values (0-255 range for matrix)
	tR := float32(tint.R * 255)
	tG := float32(tint.G * 255)
	tB := float32(tint.B * 255)

	return &ColorMatrixFilter{
		Matrix: [20]float32{
			invF, 0, 0, 0, tR * f,
			0, invF, 0, 0, tG * f,
			0, 0, invF, 0, tB * f,
			0, 0, 0, 1, 0,
		},
	}
}

// Apply applies the color matrix transformation to the image.
func (f *ColorMatrixFilter) Apply(src, dst *gg.Pixmap, bounds scene.Rect) {
	if src == nil || dst == nil {
		return
	}

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

	if minX >= maxX || minY >= maxY {
		return
	}

	srcData := src.Data()
	dstData := dst.Data()
	srcWidth := src.Width()
	dstWidth := dst.Width()

	m := &f.Matrix

	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			srcIdx := (y*srcWidth + x) * 4
			dstIdx := (y*dstWidth + x) * 4

			// Read premultiplied RGBA bytes
			pr := float32(srcData[srcIdx+0])
			pg := float32(srcData[srcIdx+1])
			pb := float32(srcData[srcIdx+2])
			a := float32(srcData[srcIdx+3])

			// Un-premultiply RGB to straight-alpha [0-255] for matrix transform.
			// The matrix coefficients assume straight-alpha color values.
			var r, g, b float32
			if a > 0 {
				r = pr * 255 / a
				g = pg * 255 / a
				b = pb * 255 / a
			}

			// Apply matrix transformation (in straight-alpha space)
			newR := m[0]*r + m[1]*g + m[2]*b + m[3]*a + m[4]
			newG := m[5]*r + m[6]*g + m[7]*b + m[8]*a + m[9]
			newB := m[10]*r + m[11]*g + m[12]*b + m[13]*a + m[14]
			newA := m[15]*r + m[16]*g + m[17]*b + m[18]*a + m[19]

			// Re-premultiply for storage
			if newA > 0 {
				factor := newA / 255
				newR *= factor
				newG *= factor
				newB *= factor
			} else {
				newR, newG, newB = 0, 0, 0
			}

			// Clamp and write premultiplied result to destination
			dstData[dstIdx+0] = clampUint8(newR)
			dstData[dstIdx+1] = clampUint8(newG)
			dstData[dstIdx+2] = clampUint8(newB)
			dstData[dstIdx+3] = clampUint8(newA)
		}
	}
}

// ExpandBounds returns the input bounds unchanged (color matrix doesn't expand).
func (f *ColorMatrixFilter) ExpandBounds(input scene.Rect) scene.Rect {
	return input
}

// Multiply returns a new filter that is the product of this filter and another.
// The result applies this filter first, then the other.
func (f *ColorMatrixFilter) Multiply(other *ColorMatrixFilter) *ColorMatrixFilter {
	a := &f.Matrix
	b := &other.Matrix

	result := &ColorMatrixFilter{}
	r := &result.Matrix

	// Matrix multiplication for 4x5 * 4x5 (treating 5th column as constant)
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			sum := float32(0)
			for k := 0; k < 4; k++ {
				sum += a[row*5+k] * b[k*5+col]
			}
			r[row*5+col] = sum
		}
		// Offset column (5th)
		r[row*5+4] = a[row*5+0]*b[4] + a[row*5+1]*b[9] +
			a[row*5+2]*b[14] + a[row*5+3]*b[19] + a[row*5+4]
	}

	return result
}

// Math helpers (avoiding math import for simple ops)
func cosf(x float64) float64 {
	// Taylor series approximation for small angles
	// For accuracy, use math.Cos in production
	x = modTwoPi(x)
	x2 := x * x
	return 1 - x2/2 + x2*x2/24 - x2*x2*x2/720
}

func sinf(x float64) float64 {
	// Taylor series approximation
	x = modTwoPi(x)
	x2 := x * x
	return x - x*x2/6 + x*x2*x2/120 - x*x2*x2*x2/5040
}

func modTwoPi(x float64) float64 {
	const twoPi = 6.283185307179586
	for x >= twoPi {
		x -= twoPi
	}
	for x < 0 {
		x += twoPi
	}
	return x
}
