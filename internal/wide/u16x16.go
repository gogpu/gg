package wide

// U16x16 represents 16 uint16 values for SIMD-style operations.
// Designed for Go compiler auto-vectorization with fixed-size arrays.
// This type is ideal for processing alpha blending and color channel operations.
type U16x16 [16]uint16

// SplatU16 creates U16x16 with all elements set to n.
// This is useful for initializing constants or broadcasting a single value.
func SplatU16(n uint16) U16x16 {
	var result U16x16
	for i := range result {
		result[i] = n
	}
	return result
}

// Add performs element-wise addition.
// Returns a new U16x16 with v[i] + other[i] for each element.
func (v U16x16) Add(other U16x16) U16x16 {
	var result U16x16
	for i := range v {
		result[i] = v[i] + other[i]
	}
	return result
}

// Sub performs element-wise subtraction.
// Returns a new U16x16 with v[i] - other[i] for each element.
func (v U16x16) Sub(other U16x16) U16x16 {
	var result U16x16
	for i := range v {
		result[i] = v[i] - other[i]
	}
	return result
}

// Mul performs element-wise multiplication.
// Returns a new U16x16 with v[i] * other[i] for each element.
func (v U16x16) Mul(other U16x16) U16x16 {
	var result U16x16
	for i := range v {
		result[i] = v[i] * other[i]
	}
	return result
}

// Div255 divides each element by 255 using fast approximation.
// Uses the formula: (x + 1 + (x >> 8)) >> 8
// This is equivalent to (x * 257) >> 16 and provides accurate division by 255.
func (v U16x16) Div255() U16x16 {
	var result U16x16
	for i := range v {
		x := v[i]
		result[i] = (x + 1 + (x >> 8)) >> 8
	}
	return result
}

// Inv computes 255 - v for each element (inverse alpha).
// Useful for computing the complement of an alpha value.
func (v U16x16) Inv() U16x16 {
	var result U16x16
	for i := range v {
		result[i] = 255 - v[i]
	}
	return result
}

// MulDiv255 performs (v * other) / 255 for each element.
// Combines multiplication and division by 255 using fast approximation.
// This is the core operation for alpha blending: c_out = (c_src * alpha) / 255.
func (v U16x16) MulDiv255(other U16x16) U16x16 {
	var result U16x16
	for i := range v {
		x := uint32(v[i]) * uint32(other[i])
		// Fast division by 255: (x + 1 + (x >> 8)) >> 8
		// Intentional narrowing conversion - result always fits in uint16
		result[i] = uint16((x + 1 + (x >> 8)) >> 8) // #nosec G115
	}
	return result
}

// Clamp clamps each element to [0, maxVal].
// Any value greater than maxVal is set to maxVal.
func (v U16x16) Clamp(maxVal uint16) U16x16 {
	var result U16x16
	for i := range v {
		if v[i] > maxVal {
			result[i] = maxVal
		} else {
			result[i] = v[i]
		}
	}
	return result
}
