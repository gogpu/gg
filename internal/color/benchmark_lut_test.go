package color

import (
	"math"
	"testing"
)

// BenchmarkSRGBToLinear_MathPow benchmarks the slow math.Pow implementation.
// This is the v0.4.0 baseline (~40 ns/op).
func BenchmarkSRGBToLinear_MathPow(b *testing.B) {
	s := uint8(128)
	var result float32
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = SRGBToLinearSlow(s)
	}
	_ = result
}

// BenchmarkSRGBToLinear_LUT benchmarks the fast LUT implementation.
// Target: ~0.2 ns/op (200x faster than math.Pow).
func BenchmarkSRGBToLinear_LUT(b *testing.B) {
	s := uint8(128)
	var result float32
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = SRGBToLinearFast(s)
	}
	_ = result
}

// BenchmarkLinearToSRGB_MathPow benchmarks the slow math.Pow implementation.
// This is the v0.4.0 baseline (~35 ns/op).
func BenchmarkLinearToSRGB_MathPow(b *testing.B) {
	l := float32(0.5)
	var result uint8
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = LinearToSRGBSlow(l)
	}
	_ = result
}

// BenchmarkLinearToSRGB_LUT benchmarks the fast LUT implementation.
// Target: ~0.2 ns/op (175x faster than math.Pow).
func BenchmarkLinearToSRGB_LUT(b *testing.B) {
	l := float32(0.5)
	var result uint8
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = LinearToSRGBFast(l)
	}
	_ = result
}

// BenchmarkColorConversion_1000px benchmarks full color space conversion pipeline.
// This simulates converting 1000 pixels from sRGB to linear, processing, and back.
func BenchmarkColorConversion_1000px(b *testing.B) {
	const n = 1000
	srgbPixels := make([]byte, n*4)
	linearPixels := make([]float32, n*4)

	// Fill with typical sRGB data
	for i := 0; i < n*4; i++ {
		srgbPixels[i] = byte((i * 7) % 256)
	}

	b.Run("MathPow_Pipeline", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// sRGB → Linear (using math.Pow)
			for j := 0; j < n*4; j++ {
				linearPixels[j] = SRGBToLinearSlow(srgbPixels[j])
			}

			// Simulate processing (just multiply by 0.8)
			for j := 0; j < n*4; j++ {
				linearPixels[j] *= 0.8
			}

			// Linear → sRGB (using math.Pow)
			for j := 0; j < n*4; j++ {
				srgbPixels[j] = LinearToSRGBSlow(linearPixels[j])
			}
		}
		b.SetBytes(n * 4 * 2) // Read + Write
	})

	b.Run("LUT_Pipeline", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// sRGB → Linear (using LUT)
			for j := 0; j < n*4; j++ {
				linearPixels[j] = SRGBToLinearFast(srgbPixels[j])
			}

			// Simulate processing (just multiply by 0.8)
			for j := 0; j < n*4; j++ {
				linearPixels[j] *= 0.8
			}

			// Linear → sRGB (using LUT)
			for j := 0; j < n*4; j++ {
				srgbPixels[j] = LinearToSRGBFast(linearPixels[j])
			}
		}
		b.SetBytes(n * 4 * 2)
	})
}

// BenchmarkSRGBToLinear_AllValues benchmarks conversion of all 256 sRGB values.
// This simulates a gradient or image with varied brightness.
func BenchmarkSRGBToLinear_AllValues(b *testing.B) {
	b.Run("MathPow", func(b *testing.B) {
		var result float32
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for s := 0; s < 256; s++ {
				result = SRGBToLinearSlow(uint8(s))
			}
		}
		_ = result
		b.SetBytes(256)
	})

	b.Run("LUT", func(b *testing.B) {
		var result float32
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for s := 0; s < 256; s++ {
				result = SRGBToLinearFast(uint8(s))
			}
		}
		_ = result
		b.SetBytes(256)
	})
}

// BenchmarkLinearToSRGB_Range benchmarks conversion across the linear range.
func BenchmarkLinearToSRGB_Range(b *testing.B) {
	// Create 256 evenly spaced linear values from 0.0 to 1.0
	linearValues := make([]float32, 256)
	for i := 0; i < 256; i++ {
		linearValues[i] = float32(i) / 255.0
	}

	b.Run("MathPow", func(b *testing.B) {
		var result uint8
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, l := range linearValues {
				result = LinearToSRGBSlow(l)
			}
		}
		_ = result
		b.SetBytes(256)
	})

	b.Run("LUT", func(b *testing.B) {
		var result uint8
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, l := range linearValues {
				result = LinearToSRGBFast(l)
			}
		}
		_ = result
		b.SetBytes(256)
	})
}

// BenchmarkGammaCorrection benchmarks typical gamma correction workflow.
func BenchmarkGammaCorrection(b *testing.B) {
	const n = 1000
	pixels := make([]byte, n*3) // RGB only

	// Fill with typical image data
	for i := range pixels {
		pixels[i] = byte((i * 13) % 256)
	}

	b.Run("Direct_Pow", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for j := 0; j < n*3; j++ {
				// Direct gamma correction with math.Pow
				normalized := float64(pixels[j]) / 255.0
				gamma := math.Pow(normalized, 2.2)
				pixels[j] = uint8(gamma * 255.0)
			}
		}
		b.SetBytes(n * 3)
	})

	b.Run("LUT_sRGB", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for j := 0; j < n*3; j++ {
				// Use LUT for correct sRGB conversion
				linear := SRGBToLinearFast(pixels[j])
				pixels[j] = LinearToSRGBFast(linear)
			}
		}
		b.SetBytes(n * 3)
	})
}

// BenchmarkBlendInLinearSpace benchmarks physically correct alpha blending.
// Blending should be done in linear space for accurate color mixing.
func BenchmarkBlendInLinearSpace(b *testing.B) {
	const n = 100
	srcSRGB := make([]byte, n*4)
	dstSRGB := make([]byte, n*4)

	// Fill with semi-transparent colors
	for i := 0; i < n*4; i += 4 {
		srcSRGB[i+0] = 200 // R
		srcSRGB[i+1] = 100 // G
		srcSRGB[i+2] = 50  // B
		srcSRGB[i+3] = 128 // A (50% transparent)

		dstSRGB[i+0] = 50  // R
		dstSRGB[i+1] = 100 // G
		dstSRGB[i+2] = 200 // B
		dstSRGB[i+3] = 255 // A (opaque)
	}

	b.Run("sRGB_Space_Incorrect", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Blend directly in sRGB (incorrect but fast)
			for j := 0; j < n; j++ {
				offset := j * 4
				sr := uint16(srcSRGB[offset+0])
				sg := uint16(srcSRGB[offset+1])
				sb := uint16(srcSRGB[offset+2])
				sa := uint16(srcSRGB[offset+3])
				dr := uint16(dstSRGB[offset+0])
				dg := uint16(dstSRGB[offset+1])
				db := uint16(dstSRGB[offset+2])

				alpha := sa
				invAlpha := 255 - sa

				// Simple alpha blend in sRGB
				dr = (sr*alpha + dr*invAlpha) / 255
				dg = (sg*alpha + dg*invAlpha) / 255
				db = (sb*alpha + db*invAlpha) / 255

				dstSRGB[offset+0] = uint8(dr)
				dstSRGB[offset+1] = uint8(dg)
				dstSRGB[offset+2] = uint8(db)
			}
		}
		b.SetBytes(n * 4)
	})

	b.Run("Linear_Space_Correct_LUT", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Blend in linear space (correct, using LUT)
			for j := 0; j < n; j++ {
				offset := j * 4

				// Convert to linear
				srLin := SRGBToLinearFast(srcSRGB[offset+0])
				sgLin := SRGBToLinearFast(srcSRGB[offset+1])
				sbLin := SRGBToLinearFast(srcSRGB[offset+2])
				sa := float32(srcSRGB[offset+3]) / 255.0

				drLin := SRGBToLinearFast(dstSRGB[offset+0])
				dgLin := SRGBToLinearFast(dstSRGB[offset+1])
				dbLin := SRGBToLinearFast(dstSRGB[offset+2])

				// Blend in linear space
				invAlpha := 1.0 - sa
				drLin = srLin*sa + drLin*invAlpha
				dgLin = sgLin*sa + dgLin*invAlpha
				dbLin = sbLin*sa + dbLin*invAlpha

				// Convert back to sRGB
				dstSRGB[offset+0] = LinearToSRGBFast(drLin)
				dstSRGB[offset+1] = LinearToSRGBFast(dgLin)
				dstSRGB[offset+2] = LinearToSRGBFast(dbLin)
			}
		}
		b.SetBytes(n * 4)
	})
}

// BenchmarkLUTMemoryAccess benchmarks different access patterns.
func BenchmarkLUTMemoryAccess(b *testing.B) {
	b.Run("Sequential_Forward", func(b *testing.B) {
		var result float32
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for s := uint8(0); s < 255; s++ {
				result = SRGBToLinearFast(s)
			}
		}
		_ = result
	})

	b.Run("Sequential_Backward", func(b *testing.B) {
		var result float32
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for s := uint8(255); s > 0; s-- {
				result = SRGBToLinearFast(s)
			}
		}
		_ = result
	})

	b.Run("Random_Pattern", func(b *testing.B) {
		// Simulate random memory access (typical for image processing)
		pattern := [16]uint8{128, 64, 192, 32, 224, 16, 240, 8, 248, 4, 252, 2, 254, 1, 255, 0}
		var result float32
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, s := range pattern {
				result = SRGBToLinearFast(s)
			}
		}
		_ = result
	})
}

// BenchmarkLUTPrecision benchmarks precision requirements.
func BenchmarkLUTPrecision(b *testing.B) {
	b.Run("8bit_Input_256_Entries", func(b *testing.B) {
		// Current implementation: 256 entries for sRGB to Linear
		var result float32
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for s := uint8(0); s < 255; s++ {
				result = SRGBToLinearFast(s)
			}
		}
		_ = result
	})

	b.Run("12bit_Output_4096_Entries", func(b *testing.B) {
		// Current implementation: 4096 entries for Linear to sRGB
		var result uint8
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for j := 0; j < 256; j++ {
				l := float32(j) / 255.0
				result = LinearToSRGBFast(l)
			}
		}
		_ = result
	})
}
