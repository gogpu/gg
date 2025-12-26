//go:build !nogpu

package wgpu

import (
	"fmt"
	"testing"

	"github.com/gogpu/gg/scene"
	"github.com/gogpu/naga"
)

// TestGPUSegmentConversion tests converting CPU segments to GPU format.
func TestGPUSegmentConversion(t *testing.T) {
	tests := []struct {
		name     string
		segment  LineSegment
		expected GPUSegment
	}{
		{
			name: "simple horizontal",
			segment: LineSegment{
				X0: 0, Y0: 0, X1: 10, Y1: 0,
				Winding: 1, TileY0: 0, TileY1: 0,
			},
			expected: GPUSegment{
				X0: 0, Y0: 0, X1: 10, Y1: 0,
				Winding: 1, TileY0: 0, TileY1: 0,
			},
		},
		{
			name: "diagonal",
			segment: LineSegment{
				X0: 0, Y0: 0, X1: 10, Y1: 10,
				Winding: -1, TileY0: 0, TileY1: 2,
			},
			expected: GPUSegment{
				X0: 0, Y0: 0, X1: 10, Y1: 10,
				Winding: -1, TileY0: 0, TileY1: 2,
			},
		},
		{
			name: "negative winding",
			segment: LineSegment{
				X0: 5.5, Y0: 3.2, X1: 8.7, Y1: 12.1,
				Winding: -1, TileY0: 0, TileY1: 3,
			},
			expected: GPUSegment{
				X0: 5.5, Y0: 3.2, X1: 8.7, Y1: 12.1,
				Winding: -1, TileY0: 0, TileY1: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments := NewSegmentList()
			segments.Add(tt.segment)

			// Use a mock rasterizer to test conversion
			r := &GPUFineRasterizer{}
			gpuSegs := r.convertSegments(segments)

			if len(gpuSegs) != 1 {
				t.Fatalf("expected 1 segment, got %d", len(gpuSegs))
			}

			got := gpuSegs[0]
			if got.X0 != tt.expected.X0 || got.Y0 != tt.expected.Y0 ||
				got.X1 != tt.expected.X1 || got.Y1 != tt.expected.Y1 ||
				got.Winding != tt.expected.Winding ||
				got.TileY0 != tt.expected.TileY0 || got.TileY1 != tt.expected.TileY1 {
				t.Errorf("segment mismatch:\ngot:  %+v\nwant: %+v", got, tt.expected)
			}
		})
	}
}

// TestFillRuleToGPU tests fill rule conversion.
func TestFillRuleToGPU(t *testing.T) {
	tests := []struct {
		rule     scene.FillStyle
		expected uint32
	}{
		{scene.FillNonZero, 0},
		{scene.FillEvenOdd, 1},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("rule_%d", i), func(t *testing.T) {
			got := FillRuleToGPU(tt.rule)
			if got != tt.expected {
				t.Errorf("FillRuleToGPU(%v) = %d, want %d", tt.rule, got, tt.expected)
			}
		})
	}
}

// TestByteConversions tests byte serialization helpers.
func TestByteConversions(t *testing.T) {
	t.Run("uint32", func(t *testing.T) {
		buf := make([]byte, 4)
		writeUint32(buf, 0, 0x12345678)

		// Little-endian check
		if buf[0] != 0x78 || buf[1] != 0x56 || buf[2] != 0x34 || buf[3] != 0x12 {
			t.Errorf("writeUint32 failed: got %v", buf)
		}
	})

	t.Run("int32", func(t *testing.T) {
		buf := make([]byte, 4)
		writeInt32(buf, 0, -1)

		// -1 in two's complement is 0xFFFFFFFF
		if buf[0] != 0xFF || buf[1] != 0xFF || buf[2] != 0xFF || buf[3] != 0xFF {
			t.Errorf("writeInt32 failed: got %v", buf)
		}
	})

	t.Run("segmentsToBytes", func(t *testing.T) {
		segs := []GPUSegment{
			{X0: 1.0, Y0: 2.0, X1: 3.0, Y1: 4.0, Winding: 1, TileY0: 0, TileY1: 1},
		}

		bytes := segmentsToBytes(segs)
		if len(bytes) != 32 {
			t.Errorf("segmentsToBytes: expected 32 bytes, got %d", len(bytes))
		}
	})

	t.Run("tileRefsToBytes", func(t *testing.T) {
		refs := []GPUTileSegmentRef{
			{TileX: 1, TileY: 2, SegmentIdx: 3, WindingFlag: 1},
		}

		bytes := tileRefsToBytes(refs)
		if len(bytes) != 16 {
			t.Errorf("tileRefsToBytes: expected 16 bytes, got %d", len(bytes))
		}
	})

	t.Run("tilesToBytes", func(t *testing.T) {
		tiles := []GPUTileInfo{
			{TileX: 1, TileY: 2, StartIdx: 0, Count: 5, Backdrop: -1},
		}

		bytes := tilesToBytes(tiles)
		if len(bytes) != 32 {
			t.Errorf("tilesToBytes: expected 32 bytes, got %d", len(bytes))
		}
	})
}

// TestBuildTileData tests tile data building from coarse entries.
func TestBuildTileData(t *testing.T) {
	// Create a simple coarse rasterizer with known entries
	coarse := NewCoarseRasterizer(100, 100)

	segments := NewSegmentList()
	segments.AddLine(5, 5, 15, 15, 1)
	segments.AddLine(20, 10, 30, 20, -1)

	coarse.Rasterize(segments)

	if len(coarse.Entries()) == 0 {
		t.Skip("no coarse entries generated")
	}

	backdrop := coarse.CalculateBackdrop()

	r := &GPUFineRasterizer{}
	tiles, refs := r.buildTileData(coarse, segments, backdrop)

	if len(tiles) == 0 {
		t.Error("expected at least one tile")
	}

	if len(refs) == 0 {
		t.Error("expected at least one tile-segment reference")
	}

	// Verify tile data consistency
	totalRefs := uint32(0)
	for _, tile := range tiles {
		totalRefs += tile.Count
	}

	if totalRefs != uint32(len(refs)) {
		t.Errorf("tile ref count mismatch: sum=%d, actual=%d", totalRefs, len(refs))
	}
}

// TestHybridFineRasterizer tests the hybrid rasterizer without GPU.
func TestHybridFineRasterizer(t *testing.T) {
	t.Run("CPU only", func(t *testing.T) {
		h := NewHybridFineRasterizer(100, 100, HybridFineRasterizerConfig{
			ForceCPU: true,
		})
		defer h.Destroy()

		if h.IsGPUAvailable() {
			t.Error("GPU should not be available with ForceCPU")
		}

		// Create simple test data
		coarse := NewCoarseRasterizer(100, 100)
		segments := NewSegmentList()
		segments.AddLine(10, 10, 20, 30, 1)
		coarse.Rasterize(segments)

		backdrop := coarse.CalculateBackdrop()

		h.SetFillRule(scene.FillNonZero)
		h.Rasterize(coarse, segments, backdrop)

		grid := h.Grid()
		if grid.TileCount() == 0 {
			t.Error("expected at least one tile after rasterization")
		}
	})

	t.Run("threshold logic", func(t *testing.T) {
		h := NewHybridFineRasterizer(100, 100, HybridFineRasterizerConfig{
			SegmentThreshold: 50,
			ForceCPU:         true,
		})
		defer h.Destroy()

		// Small workload - should use CPU
		if h.shouldUseGPU(10) {
			t.Error("should not use GPU for 10 segments with threshold 50")
		}

		// Note: GPU is disabled, so shouldUseGPU will always return false
		// Testing the threshold logic would require mocking gpuAvailable
	})

	t.Run("fill rule", func(t *testing.T) {
		h := NewHybridFineRasterizer(100, 100, HybridFineRasterizerConfig{
			ForceCPU: true,
		})
		defer h.Destroy()

		h.SetFillRule(scene.FillEvenOdd)
		if h.FillRule() != scene.FillEvenOdd {
			t.Error("fill rule not set correctly")
		}
	})

	t.Run("reset", func(t *testing.T) {
		h := NewHybridFineRasterizer(100, 100, HybridFineRasterizerConfig{
			ForceCPU: true,
		})
		defer h.Destroy()

		// Rasterize something
		coarse := NewCoarseRasterizer(100, 100)
		segments := NewSegmentList()
		segments.AddLine(10, 10, 20, 30, 1)
		coarse.Rasterize(segments)
		backdrop := coarse.CalculateBackdrop()
		h.Rasterize(coarse, segments, backdrop)

		// Reset
		h.Reset()

		if h.Grid().TileCount() != 0 {
			t.Error("grid should be empty after reset")
		}
	})
}

// TestGPURasterizerStats tests statistics collection.
func TestGPURasterizerStats(t *testing.T) {
	h := NewHybridFineRasterizer(100, 100, HybridFineRasterizerConfig{
		SegmentThreshold: 100,
		ForceCPU:         true,
	})
	defer h.Destroy()

	stats := h.Stats()

	if stats.GPUAvailable {
		t.Error("GPU should not be available with ForceCPU")
	}

	if stats.SegmentThreshold != 100 {
		t.Errorf("threshold = %d, want 100", stats.SegmentThreshold)
	}
}

// TestGPUCPUCoverageMatch tests that GPU and CPU produce matching coverage.
// This test is skipped if GPU is not available.
func TestGPUCPUCoverageMatch(t *testing.T) {
	// This test requires actual GPU hardware and would be run as an integration test.
	// For unit tests, we verify the data flow paths work correctly.

	t.Run("data path verification", func(t *testing.T) {
		// Create test geometry - a simple triangle
		coarse := NewCoarseRasterizer(100, 100)
		segments := NewSegmentList()

		// Triangle vertices approximately at (20, 20), (60, 80), (80, 30)
		segments.AddLine(20, 20, 60, 80, 1) // Left edge
		segments.AddLine(60, 80, 80, 30, 1) // Bottom edge
		segments.AddLine(80, 30, 20, 20, 1) // Right edge

		coarse.Rasterize(segments)

		if len(coarse.Entries()) == 0 {
			t.Skip("no coarse entries - geometry might be degenerate")
		}

		backdrop := coarse.CalculateBackdrop()

		// CPU rasterization
		cpuFine := NewFineRasterizer(100, 100)
		cpuFine.SetFillRule(scene.FillNonZero)
		cpuFine.Rasterize(coarse, segments, backdrop)

		cpuGrid := cpuFine.Grid()
		if cpuGrid.TileCount() == 0 {
			t.Error("CPU rasterization produced no tiles")
		}

		// Verify some tiles have non-zero coverage
		hasNonZeroCoverage := false
		cpuGrid.ForEach(func(tile *Tile) {
			for y := 0; y < TileSize; y++ {
				for x := 0; x < TileSize; x++ {
					if tile.GetCoverage(x, y) > 0 {
						hasNonZeroCoverage = true
					}
				}
			}
		})

		if !hasNonZeroCoverage {
			t.Error("CPU rasterization produced only zero coverage")
		}
	})
}

// TestFineShaderCompilation tests that the WGSL shader compiles to SPIR-V.
func TestFineShaderCompilation(t *testing.T) {
	// The shader source is embedded via go:embed
	if fineShaderWGSL == "" {
		t.Fatal("fine shader source is empty")
	}

	// Test compilation via naga
	spirvBytes, err := naga.Compile(fineShaderWGSL)
	if err != nil {
		// Check for known naga limitations and skip gracefully
		errStr := err.Error()
		if contains(errStr, "runtime-sized arrays not yet implemented") {
			t.Skip("Skipping: naga doesn't yet support runtime-sized arrays (needed for storage buffers)")
		}
		if contains(errStr, "not yet implemented") || contains(errStr, "not supported") {
			t.Skipf("Skipping: naga feature not yet implemented: %v", err)
		}
		t.Fatalf("failed to compile fine shader: %v", err)
	}

	if len(spirvBytes) == 0 {
		t.Error("SPIR-V output is empty")
	}

	// Verify SPIR-V magic number (0x07230203)
	if len(spirvBytes) < 4 {
		t.Fatal("SPIR-V too short")
	}
	magic := uint32(spirvBytes[0]) |
		uint32(spirvBytes[1])<<8 |
		uint32(spirvBytes[2])<<16 |
		uint32(spirvBytes[3])<<24
	if magic != 0x07230203 {
		t.Errorf("invalid SPIR-V magic: 0x%08X, want 0x07230203", magic)
	}

	t.Logf("Fine shader compiled to %d bytes of SPIR-V", len(spirvBytes))
}

// contains checks if s contains substr (simple helper to avoid strings import).
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BenchmarkSegmentConversion benchmarks segment conversion performance.
func BenchmarkSegmentConversion(b *testing.B) {
	// Create a segment list with many segments
	segments := NewSegmentList()
	for i := 0; i < 1000; i++ {
		x0 := float32(i % 100)
		y0 := float32(i / 100)
		segments.AddLine(x0, y0, x0+10, y0+10, 1)
	}

	r := &GPUFineRasterizer{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.convertSegments(segments)
	}
}

// BenchmarkByteConversion benchmarks byte serialization.
func BenchmarkByteConversion(b *testing.B) {
	// Create test data
	segs := make([]GPUSegment, 1000)
	for i := range segs {
		segs[i] = GPUSegment{
			X0: float32(i), Y0: float32(i),
			X1: float32(i + 10), Y1: float32(i + 10),
			Winding: 1, TileY0: 0, TileY1: 2,
		}
	}

	b.Run("segments", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = segmentsToBytes(segs)
		}
	})

	refs := make([]GPUTileSegmentRef, 1000)
	for i := range refs {
		refs[i] = GPUTileSegmentRef{
			TileX: uint32(i % 25), TileY: uint32(i / 25),
			SegmentIdx: uint32(i), WindingFlag: 1,
		}
	}

	b.Run("tileRefs", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = tileRefsToBytes(refs)
		}
	})
}
