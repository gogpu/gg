package blend

import (
	"testing"

	"github.com/gogpu/gg/internal/wide"
)

// BenchmarkSourceOver_Scalar_1000px benchmarks scalar SourceOver blending.
// This is the v0.4.0 baseline for comparison.
func BenchmarkSourceOver_Scalar_1000px(b *testing.B) {
	const n = 1000
	src := make([]byte, n*4)
	dst := make([]byte, n*4)

	// Fill with semi-transparent source
	for i := 0; i < n*4; i += 4 {
		src[i+0] = 200 // R
		src[i+1] = 100 // G
		src[i+2] = 50  // B
		src[i+3] = 128 // A (50% transparent)

		dst[i+0] = 50  // R
		dst[i+1] = 100 // G
		dst[i+2] = 200 // B
		dst[i+3] = 255 // A (opaque)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Scalar version (v0.4.0 baseline)
		for j := 0; j < n; j++ {
			offset := j * 4
			dst[offset+0], dst[offset+1], dst[offset+2], dst[offset+3] = blendSourceOver(
				src[offset+0], src[offset+1], src[offset+2], src[offset+3],
				dst[offset+0], dst[offset+1], dst[offset+2], dst[offset+3],
			)
		}
	}
	b.SetBytes(n * 4) // 4 bytes per pixel
}

// BenchmarkSourceOver_Batch_1000px benchmarks batch SourceOver blending.
// This is the v0.5.0 optimized version using SIMD-friendly wide types.
func BenchmarkSourceOver_Batch_1000px(b *testing.B) {
	const n = 1000
	src := make([]byte, n*4)
	dst := make([]byte, n*4)

	// Fill with semi-transparent source
	for i := 0; i < n*4; i += 4 {
		src[i+0] = 200 // R
		src[i+1] = 100 // G
		src[i+2] = 50  // B
		src[i+3] = 128 // A (50% transparent)

		dst[i+0] = 50  // R
		dst[i+1] = 100 // G
		dst[i+2] = 200 // B
		dst[i+3] = 255 // A (opaque)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		BlendBatch(dst, src, n, BlendSourceOver)
	}
	b.SetBytes(n * 4)
}

// BenchmarkSourceOver_1Mpx benchmarks SourceOver on 1 megapixel (typical HD frame).
func BenchmarkSourceOver_1Mpx(b *testing.B) {
	const n = 1_000_000
	src := make([]byte, n*4)
	dst := make([]byte, n*4)

	// Fill with semi-transparent colors
	for i := 0; i < n*4; i += 4 {
		src[i+0] = 200
		src[i+1] = 100
		src[i+2] = 50
		src[i+3] = 128

		dst[i+0] = 50
		dst[i+1] = 100
		dst[i+2] = 200
		dst[i+3] = 255
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		BlendBatch(dst, src, n, BlendSourceOver)
	}
	b.SetBytes(n * 4)
}

// BenchmarkAllBlendModes_16px benchmarks all blend modes on a single batch.
// Validates that batch operations are consistently fast across all modes.
func BenchmarkAllBlendModes_16px(b *testing.B) {
	modes := []struct {
		name string
		mode BlendMode
	}{
		{"Clear", BlendClear},
		{"Source", BlendSource},
		{"Destination", BlendDestination},
		{"SourceOver", BlendSourceOver},
		{"DestinationOver", BlendDestinationOver},
		{"SourceIn", BlendSourceIn},
		{"DestinationIn", BlendDestinationIn},
		{"SourceOut", BlendSourceOut},
		{"DestinationOut", BlendDestinationOut},
		{"SourceAtop", BlendSourceAtop},
		{"DestinationAtop", BlendDestinationAtop},
		{"Xor", BlendXor},
		{"Plus", BlendPlus},
		{"Modulate", BlendModulate},
		{"Screen", BlendScreen},
		{"Overlay", BlendOverlay},
		{"Darken", BlendDarken},
		{"Lighten", BlendLighten},
		{"ColorDodge", BlendColorDodge},
		{"ColorBurn", BlendColorBurn},
		{"HardLight", BlendHardLight},
		{"SoftLight", BlendSoftLight},
		{"Difference", BlendDifference},
		{"Exclusion", BlendExclusion},
	}

	for _, mode := range modes {
		b.Run(mode.name, func(b *testing.B) {
			const n = 16
			src := make([]byte, n*4)
			dst := make([]byte, n*4)

			// Fill with varied colors
			for i := 0; i < n*4; i++ {
				src[i] = byte((i * 7) % 256)
				dst[i] = byte((i * 13) % 256)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				BlendBatch(dst, src, n, mode.mode)
			}
			b.SetBytes(n * 4)
		})
	}
}

// BenchmarkBatchState_LoadStore benchmarks the overhead of batch data movement.
func BenchmarkBatchState_LoadStore(b *testing.B) {
	src := make([]byte, 64) // 16 pixels
	dst := make([]byte, 64)
	for i := range src {
		src[i] = byte(i)
		dst[i] = byte(255 - i)
	}

	var batch wide.BatchState

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		batch.LoadSrc(src)
		batch.LoadDst(dst)
		// Simulate minimal work
		batch.DR = batch.SR
		batch.DG = batch.SG
		batch.DB = batch.SB
		batch.DA = batch.SA
		batch.StoreDst(dst)
	}
	b.SetBytes(64 * 2) // Read + Write
}

// BenchmarkDiv255_Comparison compares different div255 implementations.
func BenchmarkDiv255_Comparison(b *testing.B) {
	b.Run("Fast_Single", func(b *testing.B) {
		x := uint16(32768)
		var result uint16
		for i := 0; i < b.N; i++ {
			result = div255(x)
		}
		_ = result
	})

	b.Run("Exact_Single", func(b *testing.B) {
		x := uint16(32768)
		var result uint16
		for i := 0; i < b.N; i++ {
			result = div255Exact(x)
		}
		_ = result
	})

	b.Run("Division_Single", func(b *testing.B) {
		x := uint16(32768)
		var result uint16
		for i := 0; i < b.N; i++ {
			result = x / 255
		}
		_ = result
	})

	b.Run("Batch_U16x16", func(b *testing.B) {
		a := wide.SplatU16(32768)
		for i := 0; i < b.N; i++ {
			_ = a.Div255()
		}
	})
}

// BenchmarkMulDiv255_Comparison compares different multiply-divide implementations.
func BenchmarkMulDiv255_Comparison(b *testing.B) {
	b.Run("Fast_Single", func(b *testing.B) {
		a, c := byte(200), byte(150)
		var result byte
		for i := 0; i < b.N; i++ {
			result = mulDiv255(a, c)
		}
		_ = result
	})

	b.Run("Exact_Single", func(b *testing.B) {
		a, c := byte(200), byte(150)
		var result byte
		for i := 0; i < b.N; i++ {
			result = mulDiv255Exact(a, c)
		}
		_ = result
	})

	b.Run("Old_Division", func(b *testing.B) {
		a, c := byte(200), byte(150)
		var result byte
		for i := 0; i < b.N; i++ {
			result = byte((uint16(a)*uint16(c) + 127) / 255)
		}
		_ = result
	})

	b.Run("Batch_U16x16", func(b *testing.B) {
		a := wide.SplatU16(200)
		c := wide.SplatU16(150)
		for i := 0; i < b.N; i++ {
			_ = a.MulDiv255(c)
		}
	})
}

// BenchmarkAlphaBlendComplete benchmarks full alpha blending operation.
func BenchmarkAlphaBlendComplete(b *testing.B) {
	b.Run("Scalar_16px", func(b *testing.B) {
		src := make([]byte, 64)
		dst := make([]byte, 64)
		for i := range src {
			src[i] = byte((i * 7) % 256)
			dst[i] = byte((i * 13) % 256)
		}

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for j := 0; j < 16; j++ {
				offset := j * 4
				sr := uint16(src[offset+0])
				sg := uint16(src[offset+1])
				sb := uint16(src[offset+2])
				sa := uint16(src[offset+3])
				dr := uint16(dst[offset+0])
				dg := uint16(dst[offset+1])
				db := uint16(dst[offset+2])

				invAlpha := 255 - sa
				dr = sr + uint16((uint32(dr)*uint32(invAlpha)+1+((uint32(dr)*uint32(invAlpha))>>8))>>8)
				dg = sg + uint16((uint32(dg)*uint32(invAlpha)+1+((uint32(dg)*uint32(invAlpha))>>8))>>8)
				db = sb + uint16((uint32(db)*uint32(invAlpha)+1+((uint32(db)*uint32(invAlpha))>>8))>>8)

				dst[offset+0] = byte(dr)
				dst[offset+1] = byte(dg)
				dst[offset+2] = byte(db)
				dst[offset+3] = 255
			}
		}
		b.SetBytes(64)
	})

	b.Run("Batch_16px", func(b *testing.B) {
		src := make([]byte, 64)
		dst := make([]byte, 64)
		for i := range src {
			src[i] = byte((i * 7) % 256)
			dst[i] = byte((i * 13) % 256)
		}

		var batch wide.BatchState

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			batch.LoadSrc(src)
			batch.LoadDst(dst)

			invAlpha := batch.SA.Inv()
			batch.DR = batch.SR.Add(batch.DR.MulDiv255(invAlpha))
			batch.DG = batch.SG.Add(batch.DG.MulDiv255(invAlpha))
			batch.DB = batch.SB.Add(batch.DB.MulDiv255(invAlpha))
			batch.DA = wide.SplatU16(255)

			batch.StoreDst(dst)
		}
		b.SetBytes(64)
	})
}

// BenchmarkBatchVsScalar_SizeComparison benchmarks batch vs scalar at different sizes.
func BenchmarkBatchVsScalar_SizeComparison(b *testing.B) {
	sizes := []struct {
		name   string
		pixels int
	}{
		{"16px", 16},
		{"100px", 100},
		{"1000px", 1000},
		{"10000px", 10000},
	}

	for _, size := range sizes {
		// Scalar version
		b.Run("Scalar_"+size.name, func(b *testing.B) {
			src := make([]byte, size.pixels*4)
			dst := make([]byte, size.pixels*4)
			for i := range src {
				src[i] = byte((i * 7) % 256)
				dst[i] = byte((i * 13) % 256)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for j := 0; j < size.pixels; j++ {
					offset := j * 4
					dst[offset+0], dst[offset+1], dst[offset+2], dst[offset+3] = blendSourceOver(
						src[offset+0], src[offset+1], src[offset+2], src[offset+3],
						dst[offset+0], dst[offset+1], dst[offset+2], dst[offset+3],
					)
				}
			}
			b.SetBytes(int64(size.pixels * 4))
		})

		// Batch version
		b.Run("Batch_"+size.name, func(b *testing.B) {
			src := make([]byte, size.pixels*4)
			dst := make([]byte, size.pixels*4)
			for i := range src {
				src[i] = byte((i * 7) % 256)
				dst[i] = byte((i * 13) % 256)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				BlendBatch(dst, src, size.pixels, BlendSourceOver)
			}
			b.SetBytes(int64(size.pixels * 4))
		})
	}
}

// BenchmarkMemoryAllocations verifies zero-allocation blending.
func BenchmarkMemoryAllocations(b *testing.B) {
	const n = 1000
	src := make([]byte, n*4)
	dst := make([]byte, n*4)

	for i := range src {
		src[i] = byte((i * 7) % 256)
		dst[i] = byte((i * 13) % 256)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		BlendBatch(dst, src, n, BlendSourceOver)
	}
	b.SetBytes(n * 4)

	// This benchmark MUST show 0 allocs/op to pass v0.5.0 quality bar
}
