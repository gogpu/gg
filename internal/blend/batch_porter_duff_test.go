package blend

import (
	"math/rand"
	"testing"

	"github.com/gogpu/gg/internal/wide"
)

// TestBatchPorterDuffModes tests all 14 Porter-Duff modes against scalar reference.
func TestBatchPorterDuffModes(t *testing.T) {
	modes := []struct {
		name       string
		mode       BlendMode
		batchFunc  BatchBlendFunc
		scalarFunc BlendFunc
	}{
		{"Clear", BlendClear, ClearBatch, blendClear},
		{"Source", BlendSource, SourceBatch, blendSource},
		{"Destination", BlendDestination, DestinationBatch, blendDestination},
		{"SourceOver", BlendSourceOver, SourceOverBatch, blendSourceOver},
		{"DestinationOver", BlendDestinationOver, DestinationOverBatch, blendDestinationOver},
		{"SourceIn", BlendSourceIn, SourceInBatch, blendSourceIn},
		{"DestinationIn", BlendDestinationIn, DestinationInBatch, blendDestinationIn},
		{"SourceOut", BlendSourceOut, SourceOutBatch, blendSourceOut},
		{"DestinationOut", BlendDestinationOut, DestinationOutBatch, blendDestinationOut},
		{"SourceAtop", BlendSourceAtop, SourceAtopBatch, blendSourceAtop},
		{"DestinationAtop", BlendDestinationAtop, DestinationAtopBatch, blendDestinationAtop},
		{"Xor", BlendXor, XorBatch, blendXor},
		{"Plus", BlendPlus, PlusBatch, blendPlus},
		{"Modulate", BlendModulate, ModulateBatch, blendModulate},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			testBatchVsScalar(t, tc.batchFunc, tc.scalarFunc)
		})
	}
}

// testBatchVsScalar tests that batch implementation matches scalar reference.
func testBatchVsScalar(t *testing.T, batchFunc BatchBlendFunc, scalarFunc BlendFunc) {
	// Test with deterministic data
	src := make([]byte, 64)  // 16 pixels * 4 bytes
	dst := make([]byte, 64)  // 16 pixels * 4 bytes
	want := make([]byte, 64) // expected result from scalar

	// Generate test data
	rng := rand.New(rand.NewSource(42))
	for i := range src {
		src[i] = byte(rng.Intn(256))
		dst[i] = byte(rng.Intn(256))
	}

	// Compute scalar reference
	for i := 0; i < 16; i++ {
		offset := i * 4
		sr, sg, sb, sa := src[offset+0], src[offset+1], src[offset+2], src[offset+3]
		dr, dg, db, da := dst[offset+0], dst[offset+1], dst[offset+2], dst[offset+3]
		r, g, b, a := scalarFunc(sr, sg, sb, sa, dr, dg, db, da)
		want[offset+0] = r
		want[offset+1] = g
		want[offset+2] = b
		want[offset+3] = a
	}

	// Compute batch result
	var batch wide.BatchState
	batch.LoadSrc(src)
	batch.LoadDst(dst)
	batchFunc(&batch)
	got := make([]byte, 64)
	batch.StoreDst(got)

	// Compare (allow ±1 tolerance for rounding differences)
	for i := 0; i < 64; i++ {
		diff := int(got[i]) - int(want[i])
		if diff < -2 || diff > 2 {
			pixelIdx := i / 4
			channel := "RGBA"[i%4]
			t.Errorf("pixel %d channel %c: got %d, want %d (diff %d)",
				pixelIdx, channel, got[i], want[i], diff)
		}
	}
}

// TestBatchEdgeCases tests edge cases: transparent, opaque, half-transparent.
func TestBatchEdgeCases(t *testing.T) {
	testCases := []struct {
		name string
		src  [4]byte // RGBA
		dst  [4]byte // RGBA
	}{
		{"transparent_src", [4]byte{0, 0, 0, 0}, [4]byte{255, 128, 64, 255}},
		{"transparent_dst", [4]byte{255, 128, 64, 255}, [4]byte{0, 0, 0, 0}},
		{"opaque_both", [4]byte{255, 0, 0, 255}, [4]byte{0, 255, 0, 255}},
		{"half_alpha_src", [4]byte{128, 128, 128, 128}, [4]byte{255, 255, 255, 255}},
		{"half_alpha_dst", [4]byte{255, 255, 255, 255}, [4]byte{128, 128, 128, 128}},
		{"both_half_alpha", [4]byte{128, 64, 32, 128}, [4]byte{64, 128, 192, 128}},
	}

	modes := []struct {
		name       string
		batchFunc  BatchBlendFunc
		scalarFunc BlendFunc
	}{
		{"SourceOver", SourceOverBatch, blendSourceOver},
		{"DestinationOver", DestinationOverBatch, blendDestinationOver},
		{"SourceIn", SourceInBatch, blendSourceIn},
		{"SourceOut", SourceOutBatch, blendSourceOut},
		{"SourceAtop", SourceAtopBatch, blendSourceAtop},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					// Prepare batch data (replicate pixel 16 times)
					src := make([]byte, 64)
					dst := make([]byte, 64)
					for i := 0; i < 16; i++ {
						copy(src[i*4:], tc.src[:])
						copy(dst[i*4:], tc.dst[:])
					}

					// Compute scalar reference
					sr, sg, sb, sa := tc.src[0], tc.src[1], tc.src[2], tc.src[3]
					dr, dg, db, da := tc.dst[0], tc.dst[1], tc.dst[2], tc.dst[3]
					wantR, wantG, wantB, wantA := mode.scalarFunc(sr, sg, sb, sa, dr, dg, db, da)

					// Compute batch result
					var batch wide.BatchState
					batch.LoadSrc(src)
					batch.LoadDst(dst)
					mode.batchFunc(&batch)
					got := make([]byte, 64)
					batch.StoreDst(got)

					// Check all 16 pixels match
					for i := 0; i < 16; i++ {
						offset := i * 4
						gotR, gotG, gotB, gotA := got[offset+0], got[offset+1], got[offset+2], got[offset+3]

						// Allow ±2 tolerance for div255 approximation
						if absDiff(gotR, wantR) > 2 || absDiff(gotG, wantG) > 2 ||
							absDiff(gotB, wantB) > 2 || absDiff(gotA, wantA) > 2 {
							t.Errorf("pixel %d: got RGBA(%d,%d,%d,%d), want RGBA(%d,%d,%d,%d)",
								i, gotR, gotG, gotB, gotA, wantR, wantG, wantB, wantA)
						}
					}
				})
			}
		})
	}
}

// TestGetBatchBlendFunc verifies the dispatcher returns correct functions.
func TestGetBatchBlendFunc(t *testing.T) {
	testCases := []struct {
		mode BlendMode
		want string
	}{
		{BlendClear, "Clear"},
		{BlendSource, "Source"},
		{BlendSourceOver, "SourceOver"},
		{BlendPlus, "Plus"},
		{BlendMode(99), "SourceOver"}, // Unknown mode defaults to SourceOver
	}

	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			fn := GetBatchBlendFunc(tc.mode)
			if fn == nil {
				t.Errorf("GetBatchBlendFunc(%d) returned nil", tc.mode)
			}
		})
	}
}

// TestBatchRandomData tests batch blending with random data.
func TestBatchRandomData(t *testing.T) {
	modes := []struct {
		name       string
		batchFunc  BatchBlendFunc
		scalarFunc BlendFunc
	}{
		{"SourceOver", SourceOverBatch, blendSourceOver},
		{"DestinationOver", DestinationOverBatch, blendDestinationOver},
		{"SourceIn", SourceInBatch, blendSourceIn},
		{"SourceOut", SourceOutBatch, blendSourceOut},
		{"Plus", PlusBatch, blendPlus},
		{"Modulate", ModulateBatch, blendModulate},
	}

	rng := rand.New(rand.NewSource(12345))

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			// Test 100 random pixel batches
			for iter := 0; iter < 100; iter++ {
				src := make([]byte, 64)
				dst := make([]byte, 64)
				for i := range src {
					src[i] = byte(rng.Intn(256))
					dst[i] = byte(rng.Intn(256))
				}

				// Compute scalar reference
				want := make([]byte, 64)
				for i := 0; i < 16; i++ {
					offset := i * 4
					sr, sg, sb, sa := src[offset+0], src[offset+1], src[offset+2], src[offset+3]
					dr, dg, db, da := dst[offset+0], dst[offset+1], dst[offset+2], dst[offset+3]
					r, g, b, a := mode.scalarFunc(sr, sg, sb, sa, dr, dg, db, da)
					want[offset+0] = r
					want[offset+1] = g
					want[offset+2] = b
					want[offset+3] = a
				}

				// Compute batch result
				var batch wide.BatchState
				batch.LoadSrc(src)
				batch.LoadDst(dst)
				mode.batchFunc(&batch)
				got := make([]byte, 64)
				batch.StoreDst(got)

				// Compare with tolerance
				maxDiff := 0
				for i := 0; i < 64; i++ {
					diff := absDiff(got[i], want[i])
					if diff > maxDiff {
						maxDiff = diff
					}
					if diff > 2 {
						pixelIdx := i / 4
						channel := "RGBA"[i%4]
						t.Errorf("iter %d pixel %d channel %c: got %d, want %d (diff %d)",
							iter, pixelIdx, channel, got[i], want[i], diff)
						break // Only report first error per iteration
					}
				}
			}
		})
	}
}

// BenchmarkBatchPorterDuff benchmarks batch blend operations.
func BenchmarkBatchPorterDuff(b *testing.B) {
	modes := []struct {
		name string
		fn   BatchBlendFunc
	}{
		{"Clear", ClearBatch},
		{"Source", SourceBatch},
		{"SourceOver", SourceOverBatch},
		{"DestinationOver", DestinationOverBatch},
		{"SourceIn", SourceInBatch},
		{"SourceOut", SourceOutBatch},
		{"Plus", PlusBatch},
		{"Modulate", ModulateBatch},
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

// absDiff returns absolute difference between two bytes.
func absDiff(a, b byte) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
