package wide

import "math"

// F32x8 represents 8 float32 values for SIMD-style operations.
// Designed for Go compiler auto-vectorization with fixed-size arrays.
// This type is ideal for floating-point operations like gradients and filters.
type F32x8 [8]float32

// SplatF32 creates F32x8 with all elements set to n.
// This is useful for initializing constants or broadcasting a single value.
func SplatF32(n float32) F32x8 {
	var result F32x8
	for i := range result {
		result[i] = n
	}
	return result
}

// Add performs element-wise addition.
// Returns a new F32x8 with v[i] + other[i] for each element.
func (v F32x8) Add(other F32x8) F32x8 {
	var result F32x8
	for i := range v {
		result[i] = v[i] + other[i]
	}
	return result
}

// Sub performs element-wise subtraction.
// Returns a new F32x8 with v[i] - other[i] for each element.
func (v F32x8) Sub(other F32x8) F32x8 {
	var result F32x8
	for i := range v {
		result[i] = v[i] - other[i]
	}
	return result
}

// Mul performs element-wise multiplication.
// Returns a new F32x8 with v[i] * other[i] for each element.
func (v F32x8) Mul(other F32x8) F32x8 {
	var result F32x8
	for i := range v {
		result[i] = v[i] * other[i]
	}
	return result
}

// Div performs element-wise division.
// Returns a new F32x8 with v[i] / other[i] for each element.
// Note: Division by zero results in +Inf, -Inf, or NaN according to IEEE 754.
func (v F32x8) Div(other F32x8) F32x8 {
	var result F32x8
	for i := range v {
		result[i] = v[i] / other[i]
	}
	return result
}

// Sqrt computes square root of each element.
// Returns a new F32x8 with sqrt(v[i]) for each element.
// Negative values result in NaN according to IEEE 754.
func (v F32x8) Sqrt() F32x8 {
	var result F32x8
	for i := range v {
		result[i] = float32(math.Sqrt(float64(v[i])))
	}
	return result
}

// Clamp clamps each element to [minVal, maxVal].
// Any value less than minVal is set to minVal, any value greater than maxVal is set to maxVal.
func (v F32x8) Clamp(minVal, maxVal float32) F32x8 {
	var result F32x8
	for i := range v {
		switch {
		case v[i] < minVal:
			result[i] = minVal
		case v[i] > maxVal:
			result[i] = maxVal
		default:
			result[i] = v[i]
		}
	}
	return result
}

// Lerp performs linear interpolation: v + (other - v) * t.
// When t=0, returns v; when t=1, returns other.
// t is per-element interpolation factor.
func (v F32x8) Lerp(other F32x8, t F32x8) F32x8 {
	var result F32x8
	for i := range v {
		result[i] = v[i] + (other[i]-v[i])*t[i]
	}
	return result
}

// Min performs element-wise minimum.
// Returns a new F32x8 with min(v[i], other[i]) for each element.
func (v F32x8) Min(other F32x8) F32x8 {
	var result F32x8
	for i := range v {
		if v[i] < other[i] {
			result[i] = v[i]
		} else {
			result[i] = other[i]
		}
	}
	return result
}

// Max performs element-wise maximum.
// Returns a new F32x8 with max(v[i], other[i]) for each element.
func (v F32x8) Max(other F32x8) F32x8 {
	var result F32x8
	for i := range v {
		if v[i] > other[i] {
			result[i] = v[i]
		} else {
			result[i] = other[i]
		}
	}
	return result
}
