package blend

import (
	"testing"

	"github.com/gogpu/gg/internal/wide"
)

// TestBatchAdvancedBasic tests that advanced batch blend modes execute without errors.
// These are approximations that work with premultiplied alpha directly,
// unlike scalar versions which unpremultiply first. They're tested for basic
// functionality rather than exact scalar equivalence.
func TestBatchAdvancedBasic(t *testing.T) {
	modes := []struct {
		name string
		fn   BatchBlendFunc
	}{
		{"Multiply", MultiplyBatch},
		{"Screen", ScreenBatch},
		{"Darken", DarkenBatch},
		{"Lighten", LightenBatch},
		{"Difference", DifferenceBatch},
		{"Exclusion", ExclusionBatch},
		{"Overlay", OverlayBatch},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			// Test with various pixel values
			testCases := []struct {
				name string
				src  [4]byte
				dst  [4]byte
			}{
				{"opaque_white", [4]byte{255, 255, 255, 255}, [4]byte{255, 255, 255, 255}},
				{"opaque_black", [4]byte{0, 0, 0, 255}, [4]byte{255, 255, 255, 255}},
				{"half_alpha", [4]byte{128, 128, 128, 128}, [4]byte{128, 128, 128, 128}},
				{"transparent", [4]byte{0, 0, 0, 0}, [4]byte{255, 255, 255, 255}},
			}

			for _, test := range testCases {
				t.Run(test.name, func(t *testing.T) {
					src := make([]byte, 64)
					dst := make([]byte, 64)

					// Fill with test data
					for i := 0; i < 16; i++ {
						copy(src[i*4:], test.src[:])
						copy(dst[i*4:], test.dst[:])
					}

					// Execute batch blend
					var batch wide.BatchState
					batch.LoadSrc(src)
					batch.LoadDst(dst)
					tc.fn(&batch)
					// Just verify it did not panic - batch blend operations modify BatchState in-place
				})
			}
		})
	}
}

// BenchmarkBatchAdvanced benchmarks batch advanced blend operations.
func BenchmarkBatchAdvanced(b *testing.B) {
	modes := []struct {
		name string
		fn   BatchBlendFunc
	}{
		{"Multiply", MultiplyBatch},
		{"Screen", ScreenBatch},
		{"Darken", DarkenBatch},
		{"Lighten", LightenBatch},
		{"Difference", DifferenceBatch},
		{"Exclusion", ExclusionBatch},
		{"Overlay", OverlayBatch},
	}

	for _, mode := range modes {
		b.Run(mode.name, func(b *testing.B) {
			// Setup test data
			src := make([]byte, 64)
			dst := make([]byte, 64)
			for i := range src {
				src[i] = byte((i * 7) % 256)
				dst[i] = byte((i * 13) % 256)
			}

			var batch wide.BatchState
			batch.LoadSrc(src)
			batch.LoadDst(dst)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				mode.fn(&batch)
			}

			// Prevent optimization
			batch.StoreDst(dst)
		})
	}
}
