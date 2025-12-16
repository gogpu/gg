// Package image provides image buffer management for gogpu/gg.
package image

import "math"

// InterpolationMode defines how texture sampling is performed.
type InterpolationMode uint8

const (
	// InterpNearest selects the closest pixel (no interpolation).
	// Fast but produces blocky results when scaling.
	InterpNearest InterpolationMode = iota

	// InterpBilinear performs linear interpolation between 4 neighboring pixels.
	// Good balance between quality and performance.
	InterpBilinear

	// InterpBicubic performs cubic interpolation using a 4x4 pixel neighborhood.
	// Highest quality but slower than bilinear.
	InterpBicubic
)

// String returns a string representation of the interpolation mode.
func (m InterpolationMode) String() string {
	switch m {
	case InterpNearest:
		return "Nearest"
	case InterpBilinear:
		return "Bilinear"
	case InterpBicubic:
		return "Bicubic"
	default:
		return "Unknown"
	}
}

// Sample samples the image at normalized coordinates (u, v) using the specified interpolation mode.
// u and v are in the range [0.0, 1.0] where (0,0) is top-left and (1,1) is bottom-right.
// Out-of-bounds coordinates are clamped to the edge.
func Sample(img *ImageBuf, u, v float64, mode InterpolationMode) (r, g, b, a byte) {
	switch mode {
	case InterpNearest:
		return SampleNearest(img, u, v)
	case InterpBilinear:
		return SampleBilinear(img, u, v)
	case InterpBicubic:
		return SampleBicubic(img, u, v)
	default:
		return 0, 0, 0, 0
	}
}

// SampleNearest performs nearest-neighbor sampling at normalized coordinates (u, v).
// This is the fastest sampling method but produces blocky results when scaling.
func SampleNearest(img *ImageBuf, u, v float64) (r, g, b, a byte) {
	w, h := img.Bounds()

	// Convert normalized coords to pixel coords
	// Floor is used to select the pixel containing the coordinate
	x := int(math.Floor(u * float64(w)))
	y := int(math.Floor(v * float64(h)))

	// Clamp to edge
	x = clamp(x, 0, w-1)
	y = clamp(y, 0, h-1)

	return img.GetRGBA(x, y)
}

// SampleBilinear performs bilinear interpolation at normalized coordinates (u, v).
// Interpolates between 4 neighboring pixels using linear weights.
func SampleBilinear(img *ImageBuf, u, v float64) (r, g, b, a byte) {
	w, h := img.Bounds()

	// Convert normalized coords to continuous pixel coords
	fx := u*float64(w) - 0.5
	fy := v*float64(h) - 0.5

	// Get integer coordinates and fractional parts
	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	tx := fx - float64(x0)
	ty := fy - float64(y0)

	x1 := x0 + 1
	y1 := y0 + 1

	// Clamp coordinates to image bounds
	x0 = clamp(x0, 0, w-1)
	y0 = clamp(y0, 0, h-1)
	x1 = clamp(x1, 0, w-1)
	y1 = clamp(y1, 0, h-1)

	// Get 4 corner pixels
	r00, g00, b00, a00 := img.GetRGBA(x0, y0)
	r10, g10, b10, a10 := img.GetRGBA(x1, y0)
	r01, g01, b01, a01 := img.GetRGBA(x0, y1)
	r11, g11, b11, a11 := img.GetRGBA(x1, y1)

	// Bilinear interpolation
	r = byte(lerp2D(float64(r00), float64(r10), float64(r01), float64(r11), tx, ty))
	g = byte(lerp2D(float64(g00), float64(g10), float64(g01), float64(g11), tx, ty))
	b = byte(lerp2D(float64(b00), float64(b10), float64(b01), float64(b11), tx, ty))
	a = byte(lerp2D(float64(a00), float64(a10), float64(a01), float64(a11), tx, ty))

	return r, g, b, a
}

// SampleBicubic performs bicubic interpolation at normalized coordinates (u, v).
// Uses Catmull-Rom splines with a 4x4 pixel neighborhood for smooth results.
func SampleBicubic(img *ImageBuf, u, v float64) (r, g, b, a byte) {
	w, h := img.Bounds()

	// Convert normalized coords to continuous pixel coords
	fx := u*float64(w) - 0.5
	fy := v*float64(h) - 0.5

	// Get integer coordinates and fractional parts
	x := int(math.Floor(fx))
	y := int(math.Floor(fy))
	tx := fx - float64(x)
	ty := fy - float64(y)

	// Sample 4x4 neighborhood
	var rVals, gVals, bVals, aVals [4][4]float64

	for dy := -1; dy <= 2; dy++ {
		for dx := -1; dx <= 2; dx++ {
			px := clamp(x+dx, 0, w-1)
			py := clamp(y+dy, 0, h-1)

			pr, pg, pb, pa := img.GetRGBA(px, py)
			rVals[dy+1][dx+1] = float64(pr)
			gVals[dy+1][dx+1] = float64(pg)
			bVals[dy+1][dx+1] = float64(pb)
			aVals[dy+1][dx+1] = float64(pa)
		}
	}

	// Bicubic interpolation using Catmull-Rom
	r = byte(clampFloat(bicubicInterp(rVals, tx, ty), 0, 255))
	g = byte(clampFloat(bicubicInterp(gVals, tx, ty), 0, 255))
	b = byte(clampFloat(bicubicInterp(bVals, tx, ty), 0, 255))
	a = byte(clampFloat(bicubicInterp(aVals, tx, ty), 0, 255))

	return r, g, b, a
}

// clamp clamps an integer value to [minVal, maxVal].
//
//nolint:unparam // minVal is always 0 currently, but function is general-purpose
func clamp(val, minVal, maxVal int) int {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

// clampFloat clamps a float64 value to [minVal, maxVal].
//
//nolint:unparam // minVal is always 0 currently, but function is general-purpose
func clampFloat(val, minVal, maxVal float64) float64 {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

// lerp performs linear interpolation between a and b.
func lerp(a, b, t float64) float64 {
	return a*(1-t) + b*t
}

// lerp2D performs bilinear interpolation on a 2x2 grid.
func lerp2D(v00, v10, v01, v11, tx, ty float64) float64 {
	v0 := lerp(v00, v10, tx)
	v1 := lerp(v01, v11, tx)
	return lerp(v0, v1, ty)
}

// cubicWeight computes the Catmull-Rom cubic weight for distance t.
func cubicWeight(t float64) float64 {
	// Catmull-Rom spline (Mitchell-Netravali with B=0, C=0.5):
	// |t| < 1: (1.5|t|³ - 2.5|t|² + 1)
	// 1 ≤ |t| < 2: (-0.5|t|³ + 2.5|t|² - 4|t| + 2)
	// |t| ≥ 2: 0
	absT := math.Abs(t)
	if absT < 1 {
		return 1.5*absT*absT*absT - 2.5*absT*absT + 1.0
	}
	if absT < 2 {
		return -0.5*absT*absT*absT + 2.5*absT*absT - 4.0*absT + 2.0
	}
	return 0
}

// bicubicInterp performs bicubic interpolation on a 4x4 grid using Catmull-Rom weights.
func bicubicInterp(vals [4][4]float64, tx, ty float64) float64 {
	// Compute weights for x and y
	wx := [4]float64{
		cubicWeight(tx + 1),
		cubicWeight(tx),
		cubicWeight(tx - 1),
		cubicWeight(tx - 2),
	}
	wy := [4]float64{
		cubicWeight(ty + 1),
		cubicWeight(ty),
		cubicWeight(ty - 1),
		cubicWeight(ty - 2),
	}

	// Weighted sum
	var result float64
	for i := range 4 {
		for j := range 4 {
			//nolint:gosec // G602: False positive - arrays are fixed size [4][4] and loop is bounded by 4
			result += vals[i][j] * wx[j] * wy[i]
		}
	}

	return result
}
