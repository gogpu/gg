package wide

import (
	"testing"
)

func TestBatchState_LoadSrc(t *testing.T) {
	// Create test data: 16 pixels with known values
	src := make([]byte, 64) // 16 pixels * 4 bytes
	for i := 0; i < 16; i++ {
		src[i*4+0] = uint8(i * 10)   // R
		src[i*4+1] = uint8(i*10 + 1) // G
		src[i*4+2] = uint8(i*10 + 2) // B
		src[i*4+3] = uint8(i*10 + 3) // A
	}

	var batch BatchState
	batch.LoadSrc(src)

	// Verify each channel loaded correctly
	for i := 0; i < 16; i++ {
		if batch.SR[i] != uint16(i*10) {
			t.Errorf("SR[%d] = %d, want %d", i, batch.SR[i], i*10)
		}
		if batch.SG[i] != uint16(i*10+1) {
			t.Errorf("SG[%d] = %d, want %d", i, batch.SG[i], i*10+1)
		}
		if batch.SB[i] != uint16(i*10+2) {
			t.Errorf("SB[%d] = %d, want %d", i, batch.SB[i], i*10+2)
		}
		if batch.SA[i] != uint16(i*10+3) {
			t.Errorf("SA[%d] = %d, want %d", i, batch.SA[i], i*10+3)
		}
	}
}

func TestBatchState_LoadDst(t *testing.T) {
	// Create test data: 16 pixels with known values
	dst := make([]byte, 64) // 16 pixels * 4 bytes
	for i := 0; i < 16; i++ {
		dst[i*4+0] = uint8(100 + i) // R
		dst[i*4+1] = uint8(110 + i) // G
		dst[i*4+2] = uint8(120 + i) // B
		dst[i*4+3] = uint8(130 + i) // A
	}

	var batch BatchState
	batch.LoadDst(dst)

	// Verify each channel loaded correctly
	for i := 0; i < 16; i++ {
		if batch.DR[i] != uint16(100+i) {
			t.Errorf("DR[%d] = %d, want %d", i, batch.DR[i], 100+i)
		}
		if batch.DG[i] != uint16(110+i) {
			t.Errorf("DG[%d] = %d, want %d", i, batch.DG[i], 110+i)
		}
		if batch.DB[i] != uint16(120+i) {
			t.Errorf("DB[%d] = %d, want %d", i, batch.DB[i], 120+i)
		}
		if batch.DA[i] != uint16(130+i) {
			t.Errorf("DA[%d] = %d, want %d", i, batch.DA[i], 130+i)
		}
	}
}

func TestBatchState_StoreDst(t *testing.T) {
	var batch BatchState

	// Set up known values in destination channels
	for i := 0; i < 16; i++ {
		batch.DR[i] = uint16(50 + i)
		batch.DG[i] = uint16(60 + i)
		batch.DB[i] = uint16(70 + i)
		batch.DA[i] = uint16(80 + i)
	}

	dst := make([]byte, 64)
	batch.StoreDst(dst)

	// Verify each pixel stored correctly
	for i := 0; i < 16; i++ {
		if dst[i*4+0] != uint8(50+i) {
			t.Errorf("dst[%d+0] = %d, want %d", i*4, dst[i*4+0], 50+i)
		}
		if dst[i*4+1] != uint8(60+i) {
			t.Errorf("dst[%d+1] = %d, want %d", i*4, dst[i*4+1], 60+i)
		}
		if dst[i*4+2] != uint8(70+i) {
			t.Errorf("dst[%d+2] = %d, want %d", i*4, dst[i*4+2], 70+i)
		}
		if dst[i*4+3] != uint8(80+i) {
			t.Errorf("dst[%d+3] = %d, want %d", i*4, dst[i*4+3], 80+i)
		}
	}
}

func TestBatchState_RoundTrip(t *testing.T) {
	// Create original data
	original := make([]byte, 64)
	for i := 0; i < 64; i++ {
		original[i] = uint8(i)
	}

	// Load, then store
	var batch BatchState
	batch.LoadSrc(original)

	result := make([]byte, 64)
	// Copy source channels to destination channels
	batch.DR = batch.SR
	batch.DG = batch.SG
	batch.DB = batch.SB
	batch.DA = batch.SA
	batch.StoreDst(result)

	// Verify round-trip preserves data
	for i := 0; i < 64; i++ {
		if result[i] != original[i] {
			t.Errorf("result[%d] = %d, want %d", i, result[i], original[i])
		}
	}
}

func TestBatchState_EdgeCases(t *testing.T) {
	t.Run("all zeros", func(t *testing.T) {
		src := make([]byte, 64)
		var batch BatchState
		batch.LoadSrc(src)

		for i := 0; i < 16; i++ {
			if batch.SR[i] != 0 || batch.SG[i] != 0 || batch.SB[i] != 0 || batch.SA[i] != 0 {
				t.Errorf("expected all zeros, got SR[%d]=%d, SG[%d]=%d, SB[%d]=%d, SA[%d]=%d",
					i, batch.SR[i], i, batch.SG[i], i, batch.SB[i], i, batch.SA[i])
			}
		}
	})

	t.Run("all max", func(t *testing.T) {
		src := make([]byte, 64)
		for i := range src {
			src[i] = 255
		}
		var batch BatchState
		batch.LoadSrc(src)

		for i := 0; i < 16; i++ {
			if batch.SR[i] != 255 || batch.SG[i] != 255 || batch.SB[i] != 255 || batch.SA[i] != 255 {
				t.Errorf("expected all 255, got SR[%d]=%d, SG[%d]=%d, SB[%d]=%d, SA[%d]=%d",
					i, batch.SR[i], i, batch.SG[i], i, batch.SB[i], i, batch.SA[i])
			}
		}
	})

	t.Run("alternating pattern", func(t *testing.T) {
		src := make([]byte, 64)
		for i := range src {
			if i%2 == 0 {
				src[i] = 0
			} else {
				src[i] = 255
			}
		}
		var batch BatchState
		batch.LoadSrc(src)

		for i := 0; i < 16; i++ {
			expectedR := uint16(0)
			expectedG := uint16(255)
			expectedB := uint16(0)
			expectedA := uint16(255)

			if batch.SR[i] != expectedR {
				t.Errorf("SR[%d] = %d, want %d", i, batch.SR[i], expectedR)
			}
			if batch.SG[i] != expectedG {
				t.Errorf("SG[%d] = %d, want %d", i, batch.SG[i], expectedG)
			}
			if batch.SB[i] != expectedB {
				t.Errorf("SB[%d] = %d, want %d", i, batch.SB[i], expectedB)
			}
			if batch.SA[i] != expectedA {
				t.Errorf("SA[%d] = %d, want %d", i, batch.SA[i], expectedA)
			}
		}
	})
}

func TestBatchState_Truncation(t *testing.T) {
	// Test that values > 255 are correctly truncated to uint8
	var batch BatchState

	// Set up values that will be truncated
	for i := 0; i < 16; i++ {
		batch.DR[i] = 300 // > 255, should truncate to 44 (300 & 0xFF)
		batch.DG[i] = 256 // Should truncate to 0
		batch.DB[i] = 511 // Should truncate to 255
		batch.DA[i] = 255 // Exact fit
	}

	dst := make([]byte, 64)
	batch.StoreDst(dst)

	for i := 0; i < 16; i++ {
		if dst[i*4+0] != 44 { // 300 & 0xFF = 44
			t.Errorf("dst[%d+0] = %d, want 44 (300 truncated)", i*4, dst[i*4+0])
		}
		if dst[i*4+1] != 0 { // 256 & 0xFF = 0
			t.Errorf("dst[%d+1] = %d, want 0 (256 truncated)", i*4, dst[i*4+1])
		}
		if dst[i*4+2] != 255 { // 511 & 0xFF = 255
			t.Errorf("dst[%d+2] = %d, want 255 (511 truncated)", i*4, dst[i*4+2])
		}
		if dst[i*4+3] != 255 {
			t.Errorf("dst[%d+3] = %d, want 255", i*4, dst[i*4+3])
		}
	}
}
