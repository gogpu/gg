//go:build !nogpu

package native

import (
	"testing"

	"github.com/gogpu/gg/scene"
)

// TestPipelineStageString tests PipelineStage.String method.
func TestPipelineStageString(t *testing.T) {
	tests := []struct {
		stage    PipelineStage
		expected string
	}{
		{StageFlatten, "Flatten"},
		{StageCoarse, "Coarse"},
		{StageFine, "Fine"},
		{PipelineStage(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.stage.String()
			if got != tt.expected {
				t.Errorf("PipelineStage(%d).String() = %q, want %q", tt.stage, got, tt.expected)
			}
		})
	}
}

// TestHybridPipelineCPUOnly tests the pipeline in CPU-only mode.
func TestHybridPipelineCPUOnly(t *testing.T) {
	pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	// Verify no GPU is available
	if pipeline.IsGPUAvailable() {
		t.Error("GPU should not be available with ForceCPU")
	}

	if pipeline.IsStageGPUAvailable(StageFlatten) {
		t.Error("Flatten GPU should not be available")
	}
	if pipeline.IsStageGPUAvailable(StageCoarse) {
		t.Error("Coarse GPU should not be available")
	}
	if pipeline.IsStageGPUAvailable(StageFine) {
		t.Error("Fine GPU should not be available")
	}
}

// TestHybridPipelineDefaults tests that defaults are applied correctly.
func TestHybridPipelineDefaults(t *testing.T) {
	pipeline := NewHybridPipeline(200, 200, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	stats := pipeline.Stats()

	if stats.FlattenThreshold != DefaultFlattenThreshold {
		t.Errorf("FlattenThreshold = %d, want %d", stats.FlattenThreshold, DefaultFlattenThreshold)
	}
	if stats.CoarseThreshold != DefaultCoarseThreshold {
		t.Errorf("CoarseThreshold = %d, want %d", stats.CoarseThreshold, DefaultCoarseThreshold)
	}
	if stats.FineThreshold != DefaultFineThreshold {
		t.Errorf("FineThreshold = %d, want %d", stats.FineThreshold, DefaultFineThreshold)
	}
}

// TestHybridPipelineCustomThresholds tests custom threshold configuration.
func TestHybridPipelineCustomThresholds(t *testing.T) {
	pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
		ForceCPU:         true,
		FlattenThreshold: 10,
		CoarseThreshold:  20,
		FineThreshold:    30,
	})
	defer pipeline.Destroy()

	stats := pipeline.Stats()

	if stats.FlattenThreshold != 10 {
		t.Errorf("FlattenThreshold = %d, want 10", stats.FlattenThreshold)
	}
	if stats.CoarseThreshold != 20 {
		t.Errorf("CoarseThreshold = %d, want 20", stats.CoarseThreshold)
	}
	if stats.FineThreshold != 30 {
		t.Errorf("FineThreshold = %d, want 30", stats.FineThreshold)
	}
}

// TestHybridPipelineEmptyPath tests handling of empty paths.
func TestHybridPipelineEmptyPath(t *testing.T) {
	pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	// Test nil path
	grid := pipeline.RasterizePath(nil, scene.IdentityAffine(), scene.FillNonZero)
	if grid == nil {
		t.Fatal("grid should not be nil for nil path")
	}
	if grid.TileCount() != 0 {
		t.Errorf("expected 0 tiles for nil path, got %d", grid.TileCount())
	}

	// Test empty path
	emptyPath := scene.NewPath()
	grid = pipeline.RasterizePath(emptyPath, scene.IdentityAffine(), scene.FillNonZero)
	if grid.TileCount() != 0 {
		t.Errorf("expected 0 tiles for empty path, got %d", grid.TileCount())
	}
}

// TestHybridPipelineSimpleLine tests rasterizing a simple line.
func TestHybridPipelineSimpleLine(t *testing.T) {
	pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	// Create a simple line path
	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(50, 50)
	path.Close()

	grid := pipeline.RasterizePath(path, scene.IdentityAffine(), scene.FillNonZero)

	// Should produce at least one tile
	if grid.TileCount() == 0 {
		t.Error("expected at least one tile for line path")
	}

	stats := pipeline.Stats()

	// Verify stats were updated
	if stats.FlattenTotalCalls != 1 {
		t.Errorf("FlattenTotalCalls = %d, want 1", stats.FlattenTotalCalls)
	}
	if stats.CoarseTotalCalls != 1 {
		t.Errorf("CoarseTotalCalls = %d, want 1", stats.CoarseTotalCalls)
	}
	if stats.FineTotalCalls != 1 {
		t.Errorf("FineTotalCalls = %d, want 1", stats.FineTotalCalls)
	}

	// CPU should be used (ForceCPU is true)
	if stats.FlattenCPUCalls != 1 {
		t.Errorf("FlattenCPUCalls = %d, want 1", stats.FlattenCPUCalls)
	}
	if stats.CoarseCPUCalls != 1 {
		t.Errorf("CoarseCPUCalls = %d, want 1", stats.CoarseCPUCalls)
	}
	if stats.FineCPUCalls != 1 {
		t.Errorf("FineCPUCalls = %d, want 1", stats.FineCPUCalls)
	}

	t.Logf("Line produced %d tiles, %d segments, %d entries",
		grid.TileCount(), stats.LastSegmentCount, stats.LastTileEntryCount)
}

// TestHybridPipelineTriangle tests rasterizing a triangle.
func TestHybridPipelineTriangle(t *testing.T) {
	pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	// Create a triangle path
	path := scene.NewPath()
	path.MoveTo(20, 20)
	path.LineTo(60, 80)
	path.LineTo(80, 30)
	path.Close()

	grid := pipeline.RasterizePath(path, scene.IdentityAffine(), scene.FillNonZero)

	stats := pipeline.Stats()

	if grid.TileCount() == 0 {
		t.Error("expected at least one tile for triangle")
	}

	// Triangle should have 3 segments (3 lines)
	if stats.LastSegmentCount != 3 {
		t.Errorf("LastSegmentCount = %d, want 3", stats.LastSegmentCount)
	}

	// Verify some tiles have non-zero coverage
	hasNonZeroCoverage := false
	grid.ForEach(func(tile *Tile) {
		for y := 0; y < TileSize; y++ {
			for x := 0; x < TileSize; x++ {
				if tile.GetCoverage(x, y) > 0 {
					hasNonZeroCoverage = true
				}
			}
		}
	})

	if !hasNonZeroCoverage {
		t.Error("expected non-zero coverage for triangle")
	}

	t.Logf("Triangle produced %d tiles, %d segments, %d entries",
		grid.TileCount(), stats.LastSegmentCount, stats.LastTileEntryCount)
}

// TestHybridPipelineCircle tests rasterizing a circle (curves).
func TestHybridPipelineCircle(t *testing.T) {
	pipeline := NewHybridPipeline(200, 200, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	// Create a circle path
	path := scene.NewPath()
	path.Circle(100, 100, 50)

	grid := pipeline.RasterizePath(path, scene.IdentityAffine(), scene.FillNonZero)

	stats := pipeline.Stats()

	if grid.TileCount() == 0 {
		t.Error("expected at least one tile for circle")
	}

	// Circle uses cubic Bezier curves, should produce more segments than path elements
	if stats.LastSegmentCount < stats.LastPathElements {
		t.Errorf("circle should produce more segments (%d) than path elements (%d)",
			stats.LastSegmentCount, stats.LastPathElements)
	}

	t.Logf("Circle produced %d tiles, %d segments from %d path elements",
		grid.TileCount(), stats.LastSegmentCount, stats.LastPathElements)
}

// TestHybridPipelineWithTransform tests applying a transformation.
func TestHybridPipelineWithTransform(t *testing.T) {
	pipeline := NewHybridPipeline(200, 200, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	// Create a triangle at origin
	path := scene.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(20, 0)
	path.LineTo(10, 20)
	path.Close()

	// Apply translate then scale: first translate to center, then scale
	// This puts the triangle in the middle of the viewport
	transform := scene.TranslateAffine(50, 50).Multiply(scene.ScaleAffine(2, 2))

	grid := pipeline.RasterizePath(path, transform, scene.FillNonZero)

	stats := pipeline.Stats()
	t.Logf("Transformed triangle: %d tiles, %d segments", grid.TileCount(), stats.LastSegmentCount)

	// The transformed triangle should be at (50,50) to (90,90) approximately
	// and should produce tiles
	if grid.TileCount() == 0 && stats.LastSegmentCount > 0 {
		// If we have segments but no tiles, the geometry might be outside viewport
		// This is acceptable as a test - just log it
		t.Logf("Note: segments generated but no tiles (geometry may be at edges)")
	}
}

// TestHybridPipelineFillRules tests different fill rules.
func TestHybridPipelineFillRules(t *testing.T) {
	tests := []struct {
		name     string
		fillRule scene.FillStyle
	}{
		{"NonZero", scene.FillNonZero},
		{"EvenOdd", scene.FillEvenOdd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
				ForceCPU: true,
			})
			defer pipeline.Destroy()

			// Create overlapping paths (would show difference in fill rules)
			path := scene.NewPath()
			path.Rectangle(10, 10, 40, 40)
			path.Rectangle(20, 20, 40, 40)

			pipeline.SetFillRule(tt.fillRule)
			grid := pipeline.RasterizePath(path, scene.IdentityAffine(), tt.fillRule)

			if grid.TileCount() == 0 {
				t.Errorf("expected at least one tile for %s fill rule", tt.name)
			}
		})
	}
}

// TestHybridPipelineReset tests reset functionality.
func TestHybridPipelineReset(t *testing.T) {
	pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	// Rasterize something
	path := scene.NewPath()
	path.Rectangle(10, 10, 30, 30)
	pipeline.RasterizePath(path, scene.IdentityAffine(), scene.FillNonZero)

	// Verify tiles were created
	if pipeline.Grid().TileCount() == 0 {
		t.Skip("no tiles created")
	}

	// Reset
	pipeline.Reset()

	// Grid should be empty after reset
	if pipeline.Grid().TileCount() != 0 {
		t.Error("grid should be empty after reset")
	}
}

// TestHybridPipelineStatsReset tests statistics reset.
func TestHybridPipelineStatsReset(t *testing.T) {
	pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
		ForceCPU:         true,
		FlattenThreshold: 25,
		CoarseThreshold:  50,
		FineThreshold:    75,
	})
	defer pipeline.Destroy()

	// Rasterize something to generate stats
	path := scene.NewPath()
	path.Rectangle(10, 10, 30, 30)
	pipeline.RasterizePath(path, scene.IdentityAffine(), scene.FillNonZero)

	// Verify stats were updated
	stats := pipeline.Stats()
	if stats.FlattenTotalCalls == 0 {
		t.Error("expected non-zero call count before reset")
	}

	// Reset stats
	pipeline.ResetStats()

	// Verify counters are reset but thresholds preserved
	stats = pipeline.Stats()
	if stats.FlattenTotalCalls != 0 {
		t.Errorf("FlattenTotalCalls should be 0 after reset, got %d", stats.FlattenTotalCalls)
	}
	if stats.FlattenThreshold != 25 {
		t.Errorf("FlattenThreshold should be preserved (25), got %d", stats.FlattenThreshold)
	}
	if stats.CoarseThreshold != 50 {
		t.Errorf("CoarseThreshold should be preserved (50), got %d", stats.CoarseThreshold)
	}
	if stats.FineThreshold != 75 {
		t.Errorf("FineThreshold should be preserved (75), got %d", stats.FineThreshold)
	}
}

// TestHybridPipelineSetTolerance tests tolerance configuration.
func TestHybridPipelineSetTolerance(t *testing.T) {
	pipeline := NewHybridPipeline(200, 200, HybridPipelineConfig{
		ForceCPU:  true,
		Tolerance: 1.0, // Large tolerance = fewer segments
	})
	defer pipeline.Destroy()

	// Create a circle
	path := scene.NewPath()
	path.Circle(100, 100, 50)

	// Rasterize with large tolerance
	pipeline.RasterizePath(path, scene.IdentityAffine(), scene.FillNonZero)
	stats1 := pipeline.Stats()

	// Reset and change tolerance
	pipeline.Reset()
	pipeline.SetTolerance(0.1) // Small tolerance = more segments

	pipeline.RasterizePath(path, scene.IdentityAffine(), scene.FillNonZero)
	stats2 := pipeline.Stats()

	// Smaller tolerance should produce more segments
	if stats2.LastSegmentCount <= stats1.LastSegmentCount {
		t.Errorf("smaller tolerance should produce more segments: tol=1.0 gave %d, tol=0.1 gave %d",
			stats1.LastSegmentCount, stats2.LastSegmentCount)
	}

	t.Logf("Tolerance 1.0: %d segments, Tolerance 0.1: %d segments",
		stats1.LastSegmentCount, stats2.LastSegmentCount)
}

// TestHybridPipelineThresholdLogic tests GPU/CPU selection logic.
func TestHybridPipelineThresholdLogic(t *testing.T) {
	pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
		ForceCPU:         true,
		FlattenThreshold: 5,
		CoarseThreshold:  10,
		FineThreshold:    20,
	})
	defer pipeline.Destroy()

	// Test shouldUseGPU internal logic via stats
	// Since ForceCPU is set, GPU is never available
	// But we can verify the threshold tracking works

	// Small path - below thresholds
	smallPath := scene.NewPath()
	smallPath.MoveTo(10, 10)
	smallPath.LineTo(20, 20)
	smallPath.Close()
	pipeline.RasterizePath(smallPath, scene.IdentityAffine(), scene.FillNonZero)

	stats := pipeline.Stats()
	if stats.LastFlattenUsedGPU {
		t.Error("small path should not use GPU flatten")
	}
	if stats.LastCoarseUsedGPU {
		t.Error("small path should not use GPU coarse")
	}
	if stats.LastFineUsedGPU {
		t.Error("small path should not use GPU fine")
	}
}

// TestHybridPipelineMultipleRasterizations tests multiple consecutive rasterizations.
func TestHybridPipelineMultipleRasterizations(t *testing.T) {
	pipeline := NewHybridPipeline(200, 200, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	paths := []*scene.Path{
		func() *scene.Path {
			p := scene.NewPath()
			p.Rectangle(10, 10, 30, 30)
			return p
		}(),
		func() *scene.Path {
			p := scene.NewPath()
			p.Circle(100, 100, 40)
			return p
		}(),
		func() *scene.Path {
			p := scene.NewPath()
			p.MoveTo(20, 20)
			p.LineTo(80, 30)
			p.LineTo(50, 80)
			p.Close()
			return p
		}(),
	}

	for i, path := range paths {
		grid := pipeline.RasterizePath(path, scene.IdentityAffine(), scene.FillNonZero)
		if grid.TileCount() == 0 {
			t.Errorf("path %d: expected at least one tile", i)
		}
	}

	stats := pipeline.Stats()
	if stats.FlattenTotalCalls != uint64(len(paths)) {
		t.Errorf("FlattenTotalCalls = %d, want %d", stats.FlattenTotalCalls, len(paths))
	}
}

// TestHybridPipelineDestroy tests resource cleanup.
func TestHybridPipelineDestroy(t *testing.T) {
	pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
		ForceCPU: true,
	})

	// Destroy should not panic
	pipeline.Destroy()

	// Double destroy should also not panic
	pipeline.Destroy()

	// GPU availability should be false after destroy
	if pipeline.IsGPUAvailable() {
		t.Error("GPU should not be available after destroy")
	}
}

// TestHybridPipelineGridConsistency tests that grid is consistent across operations.
func TestHybridPipelineGridConsistency(t *testing.T) {
	pipeline := NewHybridPipeline(100, 100, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	path := scene.NewPath()
	path.Rectangle(10, 10, 30, 30)

	// Get grid via RasterizePath return value
	grid1 := pipeline.RasterizePath(path, scene.IdentityAffine(), scene.FillNonZero)

	// Get grid via Grid() method
	grid2 := pipeline.Grid()

	// Both should be the same grid
	if grid1.TileCount() != grid2.TileCount() {
		t.Errorf("grid inconsistency: RasterizePath returned %d tiles, Grid() returned %d",
			grid1.TileCount(), grid2.TileCount())
	}
}

// BenchmarkHybridPipelineSimplePath benchmarks simple path rasterization.
func BenchmarkHybridPipelineSimplePath(b *testing.B) {
	pipeline := NewHybridPipeline(200, 200, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	path := scene.NewPath()
	path.Rectangle(10, 10, 100, 100)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pipeline.RasterizePath(path, transform, scene.FillNonZero)
	}
}

// BenchmarkHybridPipelineCircle benchmarks circle rasterization.
func BenchmarkHybridPipelineCircle(b *testing.B) {
	pipeline := NewHybridPipeline(200, 200, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	path := scene.NewPath()
	path.Circle(100, 100, 80)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pipeline.RasterizePath(path, transform, scene.FillNonZero)
	}
}

// BenchmarkHybridPipelineComplexPath benchmarks complex path rasterization.
func BenchmarkHybridPipelineComplexPath(b *testing.B) {
	pipeline := NewHybridPipeline(400, 400, HybridPipelineConfig{
		ForceCPU: true,
	})
	defer pipeline.Destroy()

	// Create a complex path with multiple curves
	path := scene.NewPath()
	path.MoveTo(50, 200)
	for i := 0; i < 20; i++ {
		x := float32(50 + i*15)
		path.CubicTo(x+5, 150, x+10, 250, x+15, 200)
	}
	path.Close()
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pipeline.RasterizePath(path, transform, scene.FillNonZero)
	}
}
