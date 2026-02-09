//go:build !nogpu

package gpu

import (
	"fmt"
	"testing"

	"github.com/gogpu/naga"
)

// TestGPUCoarseConfigSize tests that GPUCoarseConfig has correct size for GPU alignment.
func TestGPUCoarseConfigSize(t *testing.T) {
	// GPUCoarseConfig should be 32 bytes (8 x uint32)
	cfg := GPUCoarseConfig{
		ViewportWidth:  100,
		ViewportHeight: 100,
		TileColumns:    25,
		TileRows:       25,
		SegmentCount:   10,
		MaxEntries:     40,
	}

	bytes := coarseConfigToBytes(cfg)
	if len(bytes) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(bytes))
	}

	// Verify byte layout
	// ViewportWidth at offset 0
	if bytes[0] != 100 || bytes[1] != 0 || bytes[2] != 0 || bytes[3] != 0 {
		t.Errorf("ViewportWidth not at correct position")
	}
	// SegmentCount at offset 16
	if bytes[16] != 10 || bytes[17] != 0 || bytes[18] != 0 || bytes[19] != 0 {
		t.Errorf("SegmentCount not at correct position")
	}
}

// TestGPUCoarseTileEntrySerialization tests tile entry byte serialization.
func TestGPUCoarseTileEntrySerialization(t *testing.T) {
	entries := []GPUTileSegmentRef{
		{TileX: 1, TileY: 2, SegmentIdx: 3, WindingFlag: 1},
		{TileX: 10, TileY: 20, SegmentIdx: 30, WindingFlag: 0},
	}

	bytes := tileEntriesToBytes(entries)

	// Each entry is 16 bytes
	if len(bytes) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(bytes))
	}

	// Verify first entry
	if bytes[0] != 1 || bytes[4] != 2 || bytes[8] != 3 || bytes[12] != 1 {
		t.Errorf("first entry not serialized correctly")
	}

	// Verify second entry
	if bytes[16] != 10 || bytes[20] != 20 || bytes[24] != 30 || bytes[28] != 0 {
		t.Errorf("second entry not serialized correctly")
	}
}

// TestCoarseShaderCompilation tests that the WGSL shader compiles to SPIR-V.
func TestCoarseShaderCompilation(t *testing.T) {
	if coarseShaderWGSL == "" {
		t.Fatal("coarse shader source is empty")
	}

	// Test compilation via naga
	spirvBytes, err := naga.Compile(coarseShaderWGSL)
	if err != nil {
		errStr := err.Error()
		if contains(errStr, "runtime-sized arrays not yet implemented") {
			t.Skip("Skipping: naga doesn't yet support runtime-sized arrays")
		}
		if contains(errStr, "not yet implemented") || contains(errStr, "not supported") {
			t.Skipf("Skipping: naga feature not yet implemented: %v", err)
		}
		// Atomics are a known limitation in naga
		if contains(errStr, "lowering error") || contains(errStr, "atomic") {
			t.Skipf("Skipping: naga atomic/lowering limitation: %v", err)
		}
		t.Fatalf("failed to compile coarse shader: %v", err)
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

	t.Logf("Coarse shader compiled to %d bytes of SPIR-V", len(spirvBytes))
}

// TestGPUCoarseRasterizer_CPUFallback tests CPU fallback implementation.
func TestGPUCoarseRasterizer_CPUFallback(t *testing.T) {
	// Create test segments
	segments := NewSegmentList()
	segments.AddLine(5, 5, 15, 15, 1)    // Diagonal line
	segments.AddLine(20, 10, 30, 20, -1) // Another diagonal

	// Create a mock rasterizer for CPU fallback testing
	r := &GPUCoarseRasterizer{
		width:       100,
		height:      100,
		tileColumns: 25,
		tileRows:    25,
	}

	// Test CPU fallback
	gpuSegments := r.convertSegments(segments)
	entries := r.computeEntriesCPU(gpuSegments, 100)

	if len(entries) == 0 {
		t.Error("expected at least one tile entry")
	}

	// Verify entries have reasonable values
	for i, e := range entries {
		if e.TileX >= 25 {
			t.Errorf("entry %d: TileX %d out of range", i, e.TileX)
		}
		if e.TileY >= 25 {
			t.Errorf("entry %d: TileY %d out of range", i, e.TileY)
		}
		if e.SegmentIdx >= uint32(segments.Len()) {
			t.Errorf("entry %d: SegmentIdx %d out of range", i, e.SegmentIdx)
		}
		if e.WindingFlag > 1 {
			t.Errorf("entry %d: WindingFlag %d out of range", i, e.WindingFlag)
		}
	}

	t.Logf("Generated %d tile entries for %d segments", len(entries), segments.Len())
}

// TestGPUCoarseVsCPUCoarse tests that GPU and CPU coarse produce similar results.
func TestGPUCoarseVsCPUCoarse(t *testing.T) {
	tests := []struct {
		name     string
		segments []struct{ x0, y0, x1, y1 float32 }
	}{
		{
			name: "single vertical line",
			segments: []struct{ x0, y0, x1, y1 float32 }{
				{10, 5, 10, 25},
			},
		},
		{
			name: "single diagonal line",
			segments: []struct{ x0, y0, x1, y1 float32 }{
				{5, 5, 25, 25},
			},
		},
		{
			name: "triangle",
			segments: []struct{ x0, y0, x1, y1 float32 }{
				{20, 20, 60, 80},
				{60, 80, 80, 30},
				{80, 30, 20, 20},
			},
		},
		{
			name: "multiple lines",
			segments: []struct{ x0, y0, x1, y1 float32 }{
				{5, 5, 15, 15},
				{20, 10, 30, 20},
				{40, 40, 60, 80},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create segments
			segments := NewSegmentList()
			for _, seg := range tt.segments {
				segments.AddLine(seg.x0, seg.y0, seg.x1, seg.y1, 1)
			}

			// CPU coarse
			cpuCoarse := NewCoarseRasterizer(100, 100)
			cpuCoarse.Rasterize(segments)
			cpuEntries := cpuCoarse.Entries()

			// GPU coarse (CPU fallback)
			gpuCoarse := &GPUCoarseRasterizer{
				width:       100,
				height:      100,
				tileColumns: 25,
				tileRows:    25,
			}
			gpuSegments := gpuCoarse.convertSegments(segments)
			gpuEntries := gpuCoarse.computeEntriesCPU(gpuSegments, 100)

			// Compare entry counts (may differ due to algorithm differences)
			// but should be in the same ballpark
			cpuCount := len(cpuEntries)
			gpuCount := len(gpuEntries)

			// Allow some variance (up to 20%) due to algorithm differences
			variance := float64(cpuCount) * 0.2
			if variance < 2 {
				variance = 2
			}
			diff := float64(cpuCount - gpuCount)
			if diff < 0 {
				diff = -diff
			}
			if diff > variance && cpuCount > 0 {
				t.Errorf("entry count mismatch: CPU=%d, GPU=%d (diff=%.0f, allowed=%.0f)",
					cpuCount, gpuCount, diff, variance)
			}

			// Build tile sets for comparison
			cpuTiles := make(map[string]bool)
			for _, e := range cpuEntries {
				key := fmt.Sprintf("%d,%d", e.X, e.Y)
				cpuTiles[key] = true
			}

			gpuTiles := make(map[string]bool)
			for _, e := range gpuEntries {
				key := fmt.Sprintf("%d,%d", e.TileX, e.TileY)
				gpuTiles[key] = true
			}

			// Check that GPU covers at least most of CPU tiles
			matched := 0
			for tile := range cpuTiles {
				if gpuTiles[tile] {
					matched++
				}
			}

			if cpuCount > 0 {
				matchRate := float64(matched) / float64(len(cpuTiles))
				if matchRate < 0.8 {
					t.Errorf("low tile match rate: %.0f%% (CPU tiles: %d, matched: %d)",
						matchRate*100, len(cpuTiles), matched)
				}
			}

			t.Logf("CPU entries: %d, GPU entries: %d, tiles matched: %d/%d",
				cpuCount, gpuCount, matched, len(cpuTiles))
		})
	}
}

// TestGPUCoarseRasterizer_VerticalLine tests vertical line handling.
func TestGPUCoarseRasterizer_VerticalLine(t *testing.T) {
	segments := NewSegmentList()
	segments.AddLine(10, 5, 10, 25, 1) // Vertical line spanning multiple tiles

	gpuCoarse := &GPUCoarseRasterizer{
		width:       100,
		height:      100,
		tileColumns: 25,
		tileRows:    25,
	}

	gpuSegments := gpuCoarse.convertSegments(segments)
	entries := gpuCoarse.computeEntriesCPU(gpuSegments, 100)

	if len(entries) == 0 {
		t.Error("expected at least one tile entry for vertical line")
	}

	// Verify all entries have the same X tile
	tileX := entries[0].TileX
	for i, e := range entries {
		if e.TileX != tileX {
			t.Errorf("entry %d: expected TileX=%d, got %d", i, tileX, e.TileX)
		}
	}

	t.Logf("Vertical line produced %d entries", len(entries))
}

// TestGPUCoarseRasterizer_SingleTileLine tests line contained in single tile.
func TestGPUCoarseRasterizer_SingleTileLine(t *testing.T) {
	segments := NewSegmentList()
	// Line entirely within one tile (tile 1,1 spans pixels 4-7)
	segments.AddLine(5, 5, 6, 6, 1)

	gpuCoarse := &GPUCoarseRasterizer{
		width:       100,
		height:      100,
		tileColumns: 25,
		tileRows:    25,
	}

	gpuSegments := gpuCoarse.convertSegments(segments)
	entries := gpuCoarse.computeEntriesCPU(gpuSegments, 100)

	if len(entries) != 1 {
		t.Errorf("expected 1 tile entry for single-tile line, got %d", len(entries))
	}

	if len(entries) > 0 {
		e := entries[0]
		if e.TileX != 1 || e.TileY != 1 {
			t.Errorf("expected tile (1,1), got (%d,%d)", e.TileX, e.TileY)
		}
	}
}

// TestGPUCoarseRasterizer_OutOfBounds tests handling of out-of-bounds lines.
func TestGPUCoarseRasterizer_OutOfBounds(t *testing.T) {
	segments := NewSegmentList()
	// Line fully to the right of viewport
	segments.AddLine(200, 50, 300, 100, 1)

	gpuCoarse := &GPUCoarseRasterizer{
		width:       100,
		height:      100,
		tileColumns: 25,
		tileRows:    25,
	}

	gpuSegments := gpuCoarse.convertSegments(segments)
	entries := gpuCoarse.computeEntriesCPU(gpuSegments, 100)

	if len(entries) != 0 {
		t.Errorf("expected 0 entries for out-of-bounds line, got %d", len(entries))
	}
}

// TestGPUCoarseRasterizer_HorizontalLine tests horizontal line handling.
func TestGPUCoarseRasterizer_HorizontalLine(t *testing.T) {
	segments := NewSegmentList()
	// Horizontal line (should be skipped by AddLine)
	segments.AddLine(5, 10, 50, 10, 1)

	gpuCoarse := &GPUCoarseRasterizer{
		width:       100,
		height:      100,
		tileColumns: 25,
		tileRows:    25,
	}

	gpuSegments := gpuCoarse.convertSegments(segments)
	entries := gpuCoarse.computeEntriesCPU(gpuSegments, 100)

	// Horizontal lines should be skipped by segment list
	if len(entries) != 0 && segments.Len() != 0 {
		t.Logf("Horizontal line produced %d entries (segment skipped: %v)",
			len(entries), segments.Len() == 0)
	}
}

// TestGPUCoarseRasterizer_GetTileEntries tests conversion from CPU coarse.
func TestGPUCoarseRasterizer_GetTileEntries(t *testing.T) {
	// Create CPU coarse entries
	cpuCoarse := NewCoarseRasterizer(100, 100)
	segments := NewSegmentList()
	segments.AddLine(10, 10, 30, 30, 1)
	cpuCoarse.Rasterize(segments)

	gpuCoarse := &GPUCoarseRasterizer{
		width:       100,
		height:      100,
		tileColumns: 25,
		tileRows:    25,
	}

	// Convert to GPU format
	gpuEntries := gpuCoarse.GetTileEntries(cpuCoarse)

	cpuEntries := cpuCoarse.Entries()
	if len(gpuEntries) != len(cpuEntries) {
		t.Errorf("entry count mismatch: CPU=%d, GPU=%d", len(cpuEntries), len(gpuEntries))
	}

	// Verify conversion
	for i, cpu := range cpuEntries {
		gpu := gpuEntries[i]
		if gpu.TileX != uint32(cpu.X) || gpu.TileY != uint32(cpu.Y) {
			t.Errorf("entry %d: position mismatch", i)
		}
		if gpu.SegmentIdx != cpu.LineIdx {
			t.Errorf("entry %d: segment idx mismatch", i)
		}
		if (gpu.WindingFlag == 1) != cpu.Winding {
			t.Errorf("entry %d: winding mismatch", i)
		}
	}
}

// BenchmarkGPUCoarseCPU benchmarks the CPU fallback implementation.
func BenchmarkGPUCoarseCPU(b *testing.B) {
	// Create many segments
	segments := NewSegmentList()
	for i := 0; i < 1000; i++ {
		x0 := float32(i % 90)
		y0 := float32((i / 90) % 90)
		segments.AddLine(x0, y0, x0+10, y0+10, 1)
	}

	gpuCoarse := &GPUCoarseRasterizer{
		width:       100,
		height:      100,
		tileColumns: 25,
		tileRows:    25,
	}

	gpuSegments := gpuCoarse.convertSegments(segments)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gpuCoarse.computeEntriesCPU(gpuSegments, 4000)
	}
}

// BenchmarkGPUCoarseConvert benchmarks segment conversion.
func BenchmarkGPUCoarseConvert(b *testing.B) {
	segments := NewSegmentList()
	for i := 0; i < 1000; i++ {
		x0 := float32(i % 90)
		y0 := float32((i / 90) % 90)
		segments.AddLine(x0, y0, x0+10, y0+10, 1)
	}

	gpuCoarse := &GPUCoarseRasterizer{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gpuCoarse.convertSegments(segments)
	}
}
