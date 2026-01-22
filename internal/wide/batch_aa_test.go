package wide

import (
	"testing"
)

// TestBlendBatchAA verifies the batch AA blending matches scalar behavior.
func TestBlendBatchAA(t *testing.T) {
	tests := []struct {
		name  string
		alpha uint8
	}{
		{"zero alpha", 0},
		{"quarter alpha", 64},
		{"half alpha", 128},
		{"three quarter alpha", 192},
		{"full alpha", 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create source and destination with known values
			var batch BatchState

			// Source: solid red with full alpha
			for i := 0; i < 16; i++ {
				batch.SR[i] = 255
				batch.SG[i] = 0
				batch.SB[i] = 0
				batch.SA[i] = 255
			}

			// Destination: solid blue with full alpha
			for i := 0; i < 16; i++ {
				batch.DR[i] = 0
				batch.DG[i] = 0
				batch.DB[i] = 255
				batch.DA[i] = 255
			}

			// Also compute scalar result for comparison
			scalarR := make([]uint16, 16)
			scalarG := make([]uint16, 16)
			scalarB := make([]uint16, 16)
			scalarA := make([]uint16, 16)

			for i := 0; i < 16; i++ {
				sr := uint16(255)
				sg := uint16(0)
				sb := uint16(0)
				sa := uint16(255)
				dr := uint16(0)
				dg := uint16(0)
				db := uint16(255)
				da := uint16(255)

				// Scale source by alpha
				scaledR := mulDiv255Scalar(uint8(sr), tt.alpha)
				scaledG := mulDiv255Scalar(uint8(sg), tt.alpha)
				scaledB := mulDiv255Scalar(uint8(sb), tt.alpha)
				scaledA := mulDiv255Scalar(uint8(sa), tt.alpha)

				// SourceOver: Result = scaledS + D * (255 - scaledA) / 255
				invScaledA := 255 - scaledA
				scalarR[i] = clamp255U16(uint16(scaledR) + uint16(mulDiv255Scalar(uint8(dr), invScaledA)))
				scalarG[i] = clamp255U16(uint16(scaledG) + uint16(mulDiv255Scalar(uint8(dg), invScaledA)))
				scalarB[i] = clamp255U16(uint16(scaledB) + uint16(mulDiv255Scalar(uint8(db), invScaledA)))
				scalarA[i] = clamp255U16(uint16(scaledA) + uint16(mulDiv255Scalar(uint8(da), invScaledA)))
			}

			// Apply batch operation
			BlendBatchAA(&batch, tt.alpha)

			// Compare results
			for i := 0; i < 16; i++ {
				// Allow small differences due to rounding
				if diff(batch.DR[i], scalarR[i]) > 1 {
					t.Errorf("R[%d] = %d, want %d (diff > 1)", i, batch.DR[i], scalarR[i])
				}
				if diff(batch.DG[i], scalarG[i]) > 1 {
					t.Errorf("G[%d] = %d, want %d (diff > 1)", i, batch.DG[i], scalarG[i])
				}
				if diff(batch.DB[i], scalarB[i]) > 1 {
					t.Errorf("B[%d] = %d, want %d (diff > 1)", i, batch.DB[i], scalarB[i])
				}
				if diff(batch.DA[i], scalarA[i]) > 1 {
					t.Errorf("A[%d] = %d, want %d (diff > 1)", i, batch.DA[i], scalarA[i])
				}
			}
		})
	}
}

// TestBlendSolidColorSpanAA tests the span blending function.
func TestBlendSolidColorSpanAA(t *testing.T) {
	tests := []struct {
		name       string
		count      int
		r, g, b, a uint8
		alpha      uint8
	}{
		{"small span", 8, 255, 0, 0, 255, 128},
		{"batch aligned span", 16, 255, 0, 0, 255, 128},
		{"large span", 48, 0, 255, 0, 255, 192},
		{"partial alpha source", 32, 128, 128, 128, 128, 128},
		{"zero coverage", 16, 255, 0, 0, 255, 0},
		{"full coverage", 16, 255, 0, 0, 255, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create destination buffer (blue background)
			dst := make([]byte, tt.count*4)
			for i := 0; i < tt.count; i++ {
				dst[i*4+0] = 0   // R
				dst[i*4+1] = 0   // G
				dst[i*4+2] = 255 // B
				dst[i*4+3] = 255 // A
			}

			// Create reference buffer and apply scalar blending
			ref := make([]byte, tt.count*4)
			copy(ref, dst)
			blendSpanScalar(ref, tt.count, tt.r, tt.g, tt.b, tt.a, tt.alpha)

			// Apply batch blending
			BlendSolidColorSpanAA(dst, tt.count, tt.r, tt.g, tt.b, tt.a, tt.alpha)

			// Compare results
			for i := 0; i < tt.count*4; i++ {
				if diffByte(dst[i], ref[i]) > 1 {
					pixel := i / 4
					channel := []string{"R", "G", "B", "A"}[i%4]
					t.Errorf("pixel %d %s = %d, want %d", pixel, channel, dst[i], ref[i])
				}
			}
		})
	}
}

// TestBlendBatchAAZeroAlpha verifies zero alpha leaves destination unchanged.
func TestBlendBatchAAZeroAlpha(t *testing.T) {
	var batch BatchState

	// Set destination to known values
	for i := 0; i < 16; i++ {
		batch.DR[i] = 100
		batch.DG[i] = 150
		batch.DB[i] = 200
		batch.DA[i] = 255
	}

	// Store original values
	origR := batch.DR
	origG := batch.DG
	origB := batch.DB
	origA := batch.DA

	// Set source to different values
	for i := 0; i < 16; i++ {
		batch.SR[i] = 255
		batch.SG[i] = 0
		batch.SB[i] = 0
		batch.SA[i] = 255
	}

	// Apply with zero alpha
	BlendBatchAA(&batch, 0)

	// Verify destination unchanged
	if batch.DR != origR {
		t.Error("DR modified with zero alpha")
	}
	if batch.DG != origG {
		t.Error("DG modified with zero alpha")
	}
	if batch.DB != origB {
		t.Error("DB modified with zero alpha")
	}
	if batch.DA != origA {
		t.Error("DA modified with zero alpha")
	}
}

// TestBlendBatchAAFullAlpha verifies full alpha produces SourceOver result.
func TestBlendBatchAAFullAlpha(t *testing.T) {
	// Test with full alpha (255) - should produce same result as SourceOverBatchAA
	var batch1, batch2 BatchState

	// Same source and destination for both
	for i := 0; i < 16; i++ {
		batch1.SR[i] = 200
		batch1.SG[i] = 100
		batch1.SB[i] = 50
		batch1.SA[i] = 180

		batch1.DR[i] = 50
		batch1.DG[i] = 100
		batch1.DB[i] = 200
		batch1.DA[i] = 220

		batch2.SR[i] = batch1.SR[i]
		batch2.SG[i] = batch1.SG[i]
		batch2.SB[i] = batch1.SB[i]
		batch2.SA[i] = batch1.SA[i]

		batch2.DR[i] = batch1.DR[i]
		batch2.DG[i] = batch1.DG[i]
		batch2.DB[i] = batch1.DB[i]
		batch2.DA[i] = batch1.DA[i]
	}

	// Apply BlendBatchAA with full alpha
	BlendBatchAA(&batch1, 255)

	// Apply SourceOverBatchAA directly
	SourceOverBatchAA(&batch2)

	// Results should match
	for i := 0; i < 16; i++ {
		if batch1.DR[i] != batch2.DR[i] {
			t.Errorf("DR[%d] = %d, SourceOver = %d", i, batch1.DR[i], batch2.DR[i])
		}
		if batch1.DG[i] != batch2.DG[i] {
			t.Errorf("DG[%d] = %d, SourceOver = %d", i, batch1.DG[i], batch2.DG[i])
		}
		if batch1.DB[i] != batch2.DB[i] {
			t.Errorf("DB[%d] = %d, SourceOver = %d", i, batch1.DB[i], batch2.DB[i])
		}
		if batch1.DA[i] != batch2.DA[i] {
			t.Errorf("DA[%d] = %d, SourceOver = %d", i, batch1.DA[i], batch2.DA[i])
		}
	}
}

// BenchmarkBlendBatchAA benchmarks the batch AA blending.
func BenchmarkBlendBatchAA(b *testing.B) {
	var batch BatchState

	// Initialize with typical values
	for i := 0; i < 16; i++ {
		batch.SR[i] = 255
		batch.SG[i] = 128
		batch.SB[i] = 64
		batch.SA[i] = 200
		batch.DR[i] = 100
		batch.DG[i] = 150
		batch.DB[i] = 200
		batch.DA[i] = 255
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BlendBatchAA(&batch, 128)
	}
}

// BenchmarkBlendSolidColorSpanAA benchmarks span blending at various sizes.
func BenchmarkBlendSolidColorSpanAA(b *testing.B) {
	sizes := []int{16, 64, 256, 1024}

	for _, size := range sizes {
		b.Run(sprintSize(size), func(b *testing.B) {
			dst := make([]byte, size*4)
			for i := 0; i < size*4; i++ {
				dst[i] = 128
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				BlendSolidColorSpanAA(dst, size, 255, 128, 64, 200, 128)
			}
		})
	}
}

// BenchmarkBlendSolidColorSpanAAvsScalar compares batch vs scalar performance.
func BenchmarkBlendSolidColorSpanAAvsScalar(b *testing.B) {
	size := 256
	dst := make([]byte, size*4)
	for i := 0; i < size*4; i++ {
		dst[i] = 128
	}

	b.Run("Batch", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			BlendSolidColorSpanAA(dst, size, 255, 128, 64, 200, 128)
		}
	})

	b.Run("Scalar", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			blendSpanScalar(dst, size, 255, 128, 64, 200, 128)
		}
	})
}

// Helper functions

func mulDiv255Scalar(a, b uint8) uint8 {
	x := uint16(a) * uint16(b)
	return uint8((x + 255) >> 8)
}

func clamp255U16(x uint16) uint16 {
	if x > 255 {
		return 255
	}
	return x
}

func diff(a, b uint16) uint16 {
	if a > b {
		return a - b
	}
	return b - a
}

func diffByte(a, b byte) byte {
	if a > b {
		return a - b
	}
	return b - a
}

func blendSpanScalar(dst []byte, count int, r, g, b, a, alpha uint8) {
	if alpha == 0 {
		return
	}

	scaledR := mulDiv255Scalar(r, alpha)
	scaledG := mulDiv255Scalar(g, alpha)
	scaledB := mulDiv255Scalar(b, alpha)
	scaledA := mulDiv255Scalar(a, alpha)
	invScaledA := 255 - scaledA

	for i := 0; i < count; i++ {
		offset := i * 4
		dr := dst[offset+0]
		dg := dst[offset+1]
		db := dst[offset+2]
		da := dst[offset+3]

		dst[offset+0] = clamp255Byte(uint16(scaledR) + uint16(mulDiv255Scalar(dr, invScaledA)))
		dst[offset+1] = clamp255Byte(uint16(scaledG) + uint16(mulDiv255Scalar(dg, invScaledA)))
		dst[offset+2] = clamp255Byte(uint16(scaledB) + uint16(mulDiv255Scalar(db, invScaledA)))
		dst[offset+3] = clamp255Byte(uint16(scaledA) + uint16(mulDiv255Scalar(da, invScaledA)))
	}
}

func clamp255Byte(x uint16) byte {
	if x > 255 {
		return 255
	}
	return byte(x) // #nosec G115 - bounded by 255
}

func sprintSize(n int) string {
	if n >= 1024 {
		return sprintInt(n/1024) + "K"
	}
	return sprintInt(n)
}

func sprintInt(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	// Reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
