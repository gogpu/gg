// TrueType bytecode interpreter — fixed-point math helpers.
//
// Port of skrifa hint/math.rs (324 LOC).
// These are specific to TrueType hinting and operate on i32 values
// in 26.6, 16.16, or 2.14 fixed-point formats.
//
// Reference: skrifa/src/outline/glyf/hint/math.rs
package text

// ttFloor26Dot6 rounds a 26.6 value down (toward negative infinity).
// Reference: skrifa hint/math.rs:9
func ttFloor26Dot6(x int32) int32 {
	return x & ^63
}

// ttRound26Dot6 rounds a 26.6 value to nearest integer.
// Reference: skrifa hint/math.rs:13
func ttRound26Dot6(x int32) int32 {
	return ttFloor26Dot6(x + 32)
}

// ttCeil26Dot6 rounds a 26.6 value up (toward positive infinity).
// Reference: skrifa hint/math.rs:17
func ttCeil26Dot6(x int32) int32 {
	return ttFloor26Dot6(x + 63)
}

// ttFloorPad floors x to a multiple of n (n must be a power of 2).
// Reference: skrifa hint/math.rs:21
func ttFloorPad(x, n int32) int32 {
	return x & ^(n - 1)
}

// ttRoundPad rounds x to the nearest multiple of n (n must be power of 2).
// Reference: skrifa hint/math.rs:25
func ttRoundPad(x, n int32) int32 {
	return ttFloorPad(x+n/2, n)
}

// ttMul16Dot16 multiplies two 16.16 fixed-point values.
// Uses the same rounding as skrifa's Fixed::mul:
//
//	ab + 0x8000 - sign_correction  (sign_correction = 1 if product is negative)
//
// This produces a correctly-rounded result for both positive and negative products.
// Without the sign correction, negative products round away from zero instead of
// toward the nearest integer.
//
// Reference: font-types/src/fixed.rs:189-192 (impl Mul for Fixed)
func ttMul16Dot16(a, b int32) int32 {
	ab := int64(a) * int64(b)
	sign := int64(0)
	if ab < 0 {
		sign = 1
	}
	return int32((ab + 0x8000 - sign) >> 16)
}

// ttDiv16Dot16 divides two 16.16 fixed-point values with rounding.
//
// Uses the same rounded division as skrifa's Fixed::div:
//
//	sign = (a < 0) ^ (b < 0)
//	q = (abs(a) << 16 + abs(b)/2) / abs(b)
//	result = sign ? -q : q
//
// The rounding (adding half-divisor) is critical for IUP interpolation
// where even 1 LSB difference in the scale factor produces visible
// coordinate differences on untouched points.
//
// Reference: skrifa hint/math.rs:34 → font-types/src/fixed.rs:195-207
func ttDiv16Dot16(a, b int32) int32 {
	sign := (a < 0) != (b < 0)
	au := int64(a)
	if au < 0 {
		au = -au
	}
	bu := int64(b)
	if bu < 0 {
		bu = -bu
	}
	var q int32
	if bu == 0 {
		q = 0x7FFFFFFF
	} else {
		q = int32(((au << 16) + (bu >> 1)) / bu)
	}
	if sign {
		return -q
	}
	return q
}

// ttMulDiv computes a * b / c with 64-bit intermediate precision and rounding.
// Reference: skrifa hint/math.rs:39
func ttMulDiv(a, b, c int32) int32 {
	if c == 0 {
		if (a >= 0) == (b >= 0) {
			return 0x7FFFFFFF
		}
		return -0x7FFFFFFF
	}
	ab := int64(a) * int64(b)
	// Round toward nearest
	if (ab >= 0) == (c > 0) {
		return int32((ab + int64(c)/2) / int64(c))
	}
	return int32((ab - int64(c)/2) / int64(c))
}

// ttMulDivNoRound computes a * b / c without rounding.
// Matches FreeType FT_MulDiv_NoRound.
// Reference: skrifa hint/math.rs:48-72
func ttMulDivNoRound(a, b, c int32) int32 {
	s := int32(1)
	ua, ub, uc := a, b, c
	if ua < 0 {
		ua = -ua
		s = -1
	}
	if ub < 0 {
		ub = -ub
		s = -s
	}
	if uc < 0 {
		uc = -uc
		s = -s
	}
	var d int64
	if uc > 0 {
		d = (int64(ua) * int64(ub)) / int64(uc)
	} else {
		d = 0x7FFFFFFF
	}
	if s < 0 {
		return -int32(d)
	}
	return int32(d)
}

// ttMul14 multiplies a 26.6 value by a 2.14 value.
// Matches FreeType TT_MulFix14.
// Reference: skrifa hint/math.rs:77-81
func ttMul14(a, b int32) int32 {
	v := int64(a) * int64(b)
	v += 0x2000 + (v >> 63)
	return int32(v >> 14)
}

// ttNormalize14 normalizes a 2D vector to 2.14 fixed-point unit length.
// Uses Wrapping arithmetic matching FreeType FT_Vector_NormLen.
// Reference: skrifa hint/math.rs:86-162
func ttNormalize14(x, y int32) (int32, int32) {
	sx, sy := int32(1), int32(1)
	ux, uy := uint32(x), uint32(y)
	rx, ry := int32(0), int32(0)

	if x < 0 {
		ux = uint32(-x) // wrapping negate
		sx = -1
	}
	if y < 0 {
		uy = uint32(-y)
		sy = -1
	}

	// Degenerate: one axis is zero
	if ux == 0 {
		rx = x / 4
		if uy > 0 {
			ry = int32(uint32(sy*0x10000) / 4) //nolint:gosec // wrapping arithmetic matching skrifa
		}
		return rx, ry
	}
	if uy == 0 {
		ry = y / 4
		if ux > 0 {
			rx = int32(uint32(sx*0x10000) / 4) //nolint:gosec // wrapping arithmetic matching skrifa
		}
		return rx, ry
	}

	// Approximate length
	var lenVal uint32
	if ux > uy {
		lenVal = ux + (uy >> 1)
	} else {
		lenVal = uy + (ux >> 1)
	}

	// Compute shift to normalize
	lz := clz32(lenVal)
	shift := int32(lz) - 15
	check := uint32(0xAAAAAAAA) >> lz
	if lenVal >= check {
		shift--
	}

	if shift > 0 {
		s := uint(shift) //nolint:gosec // shift is guaranteed positive here
		ux <<= s
		uy <<= s
		if ux > uy {
			lenVal = ux + (uy >> 1)
		} else {
			lenVal = uy + (ux >> 1)
		}
	} else {
		s := uint(-shift) //nolint:gosec // -shift is guaranteed non-negative here
		ux >>= s
		uy >>= s
		lenVal >>= s
	}

	b := 0x10000 - int32(lenVal) //nolint:gosec // wrapping arithmetic matching skrifa
	ix := int32(ux)              //nolint:gosec // wrapping arithmetic matching skrifa
	iy := int32(uy)              //nolint:gosec // wrapping arithmetic matching skrifa

	var u, v uint32
	for {
		u = uint32(ix + ((ix * b) >> 16)) //nolint:gosec // wrapping arithmetic matching skrifa
		v = uint32(iy + ((iy * b) >> 16)) //nolint:gosec // wrapping arithmetic matching skrifa

		sum := u*u + v*v
		z := -int32(sum) / 0x200 //nolint:gosec // wrapping arithmetic matching skrifa
		z = z * ((int32(0x10000) + b) >> 8) / 0x10000
		b += z
		if z <= 0 {
			break
		}
	}

	rx = int32(u) * sx / 4 //nolint:gosec // wrapping arithmetic matching skrifa
	ry = int32(v) * sy / 4 //nolint:gosec // wrapping arithmetic matching skrifa
	return rx, ry
}

// ttDot14 computes the dot product of two 2.14 vectors.
// Reference: skrifa hint/projection.rs:119-125
func ttDot14(ax, ay, bx, by int32) int32 {
	v := int64(ax)*int64(bx) + int64(ay)*int64(by)
	v += 0x2000 + (v >> 63)
	return int32(v >> 14)
}

// clz32 counts leading zeros in a uint32 (equivalent to bits.LeadingZeros32).
func clz32(x uint32) uint32 {
	if x == 0 {
		return 32
	}
	n := uint32(0)
	if x <= 0x0000FFFF {
		n += 16
		x <<= 16
	}
	if x <= 0x00FFFFFF {
		n += 8
		x <<= 8
	}
	if x <= 0x0FFFFFFF {
		n += 4
		x <<= 4
	}
	if x <= 0x3FFFFFFF {
		n += 2
		x <<= 2
	}
	if x <= 0x7FFFFFFF {
		n++
	}
	return n
}
