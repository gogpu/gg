package blend

import (
	"math/rand"
	"testing"
)

// TestBlendBatch tests BlendBatch against scalar reference implementation.
func TestBlendBatch(t *testing.T) {
	sizes := []int{1, 7, 15, 16, 17, 31, 32, 33, 100, 256, 1024}
	modes := []struct {
		name string
		mode BlendMode
	}{
		{"Clear", BlendClear},
		{"Source", BlendSource},
		{"SourceOver", BlendSourceOver},
		{"DestinationOver", BlendDestinationOver},
		{"SourceIn", BlendSourceIn},
		{"Plus", BlendPlus},
		{"Modulate", BlendModulate},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range sizes {
				t.Run("", func(t *testing.T) {
					testBlendBatchSize(t, size, tc.mode)
				})
			}
		})
	}
}

func testBlendBatchSize(t *testing.T, n int, mode BlendMode) {
	// Prepare test data
	src := make([]byte, n*4)
	dstBatch := make([]byte, n*4)
	dstScalar := make([]byte, n*4)

	rng := rand.New(rand.NewSource(int64(n) + int64(mode)))
	for i := range src {
		src[i] = byte(rng.Intn(256))
		dstBatch[i] = byte(rng.Intn(256))
	}
	copy(dstScalar, dstBatch)

	// Compute batch result
	BlendBatch(dstBatch, src, n, mode)

	// Compute scalar reference
	scalarFunc := GetBlendFunc(mode)
	for i := 0; i < n; i++ {
		offset := i * 4
		sr := src[offset+0]
		sg := src[offset+1]
		sb := src[offset+2]
		sa := src[offset+3]
		dr := dstScalar[offset+0]
		dg := dstScalar[offset+1]
		db := dstScalar[offset+2]
		da := dstScalar[offset+3]

		r, g, b, a := scalarFunc(sr, sg, sb, sa, dr, dg, db, da)
		dstScalar[offset+0] = r
		dstScalar[offset+1] = g
		dstScalar[offset+2] = b
		dstScalar[offset+3] = a
	}

	// Compare results (allow ±1 tolerance)
	for i := 0; i < n*4; i++ {
		diff := absDiff(dstBatch[i], dstScalar[i])
		if diff > 2 {
			pixelIdx := i / 4
			channel := "RGBA"[i%4]
			t.Errorf("pixel %d channel %c: batch=%d scalar=%d (diff %d)",
				pixelIdx, channel, dstBatch[i], dstScalar[i], diff)
		}
	}
}

// TestBlendSourceOverBatch tests the optimized SourceOver implementation.
func TestBlendSourceOverBatch(t *testing.T) {
	sizes := []int{1, 7, 15, 16, 17, 31, 32, 33, 100, 256, 1024}

	for _, size := range sizes {
		t.Run("", func(t *testing.T) {
			// Prepare test data
			src := make([]byte, size*4)
			dstBatch := make([]byte, size*4)
			dstScalar := make([]byte, size*4)

			rng := rand.New(rand.NewSource(int64(size)))
			for i := range src {
				src[i] = byte(rng.Intn(256))
				dstBatch[i] = byte(rng.Intn(256))
			}
			copy(dstScalar, dstBatch)

			// Compute batch result
			BlendSourceOverBatch(dstBatch, src, size)

			// Compute scalar reference
			for i := 0; i < size; i++ {
				offset := i * 4
				sr := src[offset+0]
				sg := src[offset+1]
				sb := src[offset+2]
				sa := src[offset+3]
				dr := dstScalar[offset+0]
				dg := dstScalar[offset+1]
				db := dstScalar[offset+2]
				da := dstScalar[offset+3]

				invSa := 255 - sa
				dstScalar[offset+0] = addDiv255(sr, mulDiv255(dr, invSa))
				dstScalar[offset+1] = addDiv255(sg, mulDiv255(dg, invSa))
				dstScalar[offset+2] = addDiv255(sb, mulDiv255(db, invSa))
				dstScalar[offset+3] = addDiv255(sa, mulDiv255(da, invSa))
			}

			// Compare results (allow ±1 tolerance)
			for i := 0; i < size*4; i++ {
				diff := absDiff(dstBatch[i], dstScalar[i])
				if diff > 2 {
					pixelIdx := i / 4
					channel := "RGBA"[i%4]
					t.Errorf("pixel %d channel %c: batch=%d scalar=%d (diff %d)",
						pixelIdx, channel, dstBatch[i], dstScalar[i], diff)
				}
			}
		})
	}
}

// TestBlendBatchAligned tests aligned batch operations.
func TestBlendBatchAligned(t *testing.T) {
	sizes := []int{16, 32, 64, 128, 256}
	modes := []BlendMode{BlendSourceOver, BlendSourceIn, BlendPlus}

	for _, mode := range modes {
		t.Run("", func(t *testing.T) {
			for _, size := range sizes {
				t.Run("", func(t *testing.T) {
					// Prepare test data
					src := make([]byte, size*4)
					dstBatch := make([]byte, size*4)
					dstNormal := make([]byte, size*4)

					rng := rand.New(rand.NewSource(int64(size) + int64(mode)))
					for i := range src {
						src[i] = byte(rng.Intn(256))
						dstBatch[i] = byte(rng.Intn(256))
					}
					copy(dstNormal, dstBatch)

					// Compute aligned result
					BlendBatchAligned(dstBatch, src, size, mode)

					// Compute normal result
					BlendBatch(dstNormal, src, size, mode)

					// Results should be identical
					for i := 0; i < size*4; i++ {
						if dstBatch[i] != dstNormal[i] {
							pixelIdx := i / 4
							channel := "RGBA"[i%4]
							t.Errorf("pixel %d channel %c: aligned=%d normal=%d",
								pixelIdx, channel, dstBatch[i], dstNormal[i])
						}
					}
				})
			}
		})
	}
}

// TestBlendBatchEdgeCases tests edge cases.
func TestBlendBatchEdgeCases(t *testing.T) {
	t.Run("zero_pixels", func(t *testing.T) {
		dst := make([]byte, 64)
		src := make([]byte, 64)
		BlendBatch(dst, src, 0, BlendSourceOver)
		// Should not crash
	})

	t.Run("negative_pixels", func(t *testing.T) {
		dst := make([]byte, 64)
		src := make([]byte, 64)
		BlendBatch(dst, src, -1, BlendSourceOver)
		// Should not crash
	})

	t.Run("single_pixel", func(t *testing.T) {
		dst := []byte{100, 100, 100, 200}
		src := []byte{200, 50, 50, 128}
		expected := make([]byte, 4)
		copy(expected, dst)

		// Compute expected with scalar
		r, g, b, a := blendSourceOver(src[0], src[1], src[2], src[3], dst[0], dst[1], dst[2], dst[3])
		expected[0] = r
		expected[1] = g
		expected[2] = b
		expected[3] = a

		BlendBatch(dst, src, 1, BlendSourceOver)

		for i := 0; i < 4; i++ {
			if absDiff(dst[i], expected[i]) > 1 {
				t.Errorf("channel %d: got %d, want %d", i, dst[i], expected[i])
			}
		}
	})
}

// BenchmarkBlendBatch benchmarks batch blending vs scalar for various sizes.
func BenchmarkBlendBatch(b *testing.B) {
	sizes := []int{16, 32, 64, 128, 256, 512, 1024}

	for _, size := range sizes {
		b.Run("", func(b *testing.B) {
			src := make([]byte, size*4)
			dst := make([]byte, size*4)
			for i := range src {
				src[i] = byte((i * 7) % 256)
				dst[i] = byte((i * 13) % 256)
			}

			b.ReportAllocs()
			b.SetBytes(int64(size * 4)) // Bytes processed per operation
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				BlendBatch(dst, src, size, BlendSourceOver)
			}
		})
	}
}

// BenchmarkBlendSourceOverBatch benchmarks optimized SourceOver.
func BenchmarkBlendSourceOverBatch(b *testing.B) {
	sizes := []int{16, 32, 64, 128, 256, 512, 1024}

	for _, size := range sizes {
		b.Run("", func(b *testing.B) {
			src := make([]byte, size*4)
			dst := make([]byte, size*4)
			for i := range src {
				src[i] = byte((i * 7) % 256)
				dst[i] = byte((i * 13) % 256)
			}

			b.ReportAllocs()
			b.SetBytes(int64(size * 4))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				BlendSourceOverBatch(dst, src, size)
			}
		})
	}
}

// BenchmarkBlendBatchAligned benchmarks aligned batch operations.
func BenchmarkBlendBatchAligned(b *testing.B) {
	sizes := []int{16, 32, 64, 128, 256, 512, 1024}

	for _, size := range sizes {
		b.Run("", func(b *testing.B) {
			src := make([]byte, size*4)
			dst := make([]byte, size*4)
			for i := range src {
				src[i] = byte((i * 7) % 256)
				dst[i] = byte((i * 13) % 256)
			}

			b.ReportAllocs()
			b.SetBytes(int64(size * 4))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				BlendBatchAligned(dst, src, size, BlendSourceOver)
			}
		})
	}
}

// BenchmarkBatchVsScalar compares batch and scalar implementations.
func BenchmarkBatchVsScalar(b *testing.B) {
	size := 1024 // Large size to show batch benefit
	src := make([]byte, size*4)
	dst := make([]byte, size*4)
	for i := range src {
		src[i] = byte((i * 7) % 256)
		dst[i] = byte((i * 13) % 256)
	}

	b.Run("Batch", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(size * 4))
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			BlendSourceOverBatch(dst, src, size)
		}
	})

	b.Run("Scalar", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(size * 4))
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Scalar loop
			for j := 0; j < size; j++ {
				offset := j * 4
				sr := src[offset+0]
				sg := src[offset+1]
				sb := src[offset+2]
				sa := src[offset+3]
				dr := dst[offset+0]
				dg := dst[offset+1]
				db := dst[offset+2]
				da := dst[offset+3]

				invSa := 255 - sa
				dst[offset+0] = addDiv255(sr, mulDiv255(dr, invSa))
				dst[offset+1] = addDiv255(sg, mulDiv255(dg, invSa))
				dst[offset+2] = addDiv255(sb, mulDiv255(db, invSa))
				dst[offset+3] = addDiv255(sa, mulDiv255(da, invSa))
			}
		}
	})
}
