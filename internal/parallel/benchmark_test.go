package parallel

import (
	"image/color"
	"testing"
)

// =============================================================================
// Component Benchmarks - TileGrid
// =============================================================================

// BenchmarkTileGrid_Create_HD benchmarks TileGrid creation for HD resolution.
func BenchmarkTileGrid_Create_HD(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		grid := NewTileGrid(1920, 1080)
		grid.Close()
	}
}

// BenchmarkTileGrid_Create_4K benchmarks TileGrid creation for 4K resolution.
func BenchmarkTileGrid_Create_4K(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		grid := NewTileGrid(3840, 2160)
		grid.Close()
	}
}

// BenchmarkTileGrid_TileAt_HD benchmarks tile lookup by tile coordinates.
func BenchmarkTileGrid_TileAt_HD(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = grid.TileAt(15, 8) // Middle of HD grid
	}
}

// BenchmarkTileGrid_TileAtPixel_HD benchmarks tile lookup by pixel coordinates.
func BenchmarkTileGrid_TileAtPixel_HD(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = grid.TileAtPixel(960, 540) // Center of screen
	}
}

// BenchmarkTileGrid_TilesInRect_SmallRect benchmarks finding tiles in a small rectangle.
func BenchmarkTileGrid_TilesInRect_SmallRect(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = grid.TilesInRect(100, 100, 64, 64) // Single tile
	}
}

// BenchmarkTileGrid_TilesInRect_MediumRect benchmarks finding tiles in a medium rectangle.
func BenchmarkTileGrid_TilesInRect_MediumRect(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = grid.TilesInRect(100, 100, 256, 256) // ~16 tiles
	}
}

// BenchmarkTileGrid_TilesInRect_LargeRect benchmarks finding tiles in a large rectangle.
func BenchmarkTileGrid_TilesInRect_LargeRect(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = grid.TilesInRect(0, 0, 960, 540) // Quarter screen
	}
}

// BenchmarkTileGrid_MarkDirty benchmarks marking a single tile dirty.
func BenchmarkTileGrid_MarkDirty(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		grid.MarkDirty(15, 8)
	}
}

// BenchmarkTileGrid_MarkRectDirty benchmarks marking a rectangle of tiles dirty.
func BenchmarkTileGrid_MarkRectDirty(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		grid.MarkRectDirty(100, 100, 256, 256)
	}
}

// =============================================================================
// Component Benchmarks - WorkerPool
// =============================================================================

// BenchmarkWorkerPool_Create benchmarks creating a worker pool.
func BenchmarkWorkerPool_Create(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pool := NewWorkerPool(0) // Use GOMAXPROCS
		pool.Close()
	}
}

// BenchmarkWorkerPool_ExecuteAll_10 benchmarks executing 10 work items.
func BenchmarkWorkerPool_ExecuteAll_10(b *testing.B) {
	pool := NewWorkerPool(0)
	defer pool.Close()

	work := make([]func(), 10)
	for i := range work {
		work[i] = func() {
			// Minimal work
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

// BenchmarkWorkerPool_ExecuteAll_100 benchmarks executing 100 work items.
func BenchmarkWorkerPool_ExecuteAll_100(b *testing.B) {
	pool := NewWorkerPool(0)
	defer pool.Close()

	work := make([]func(), 100)
	for i := range work {
		work[i] = func() {
			// Minimal work
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

// BenchmarkWorkerPool_ExecuteAll_1000 benchmarks executing 1000 work items.
func BenchmarkWorkerPool_ExecuteAll_1000(b *testing.B) {
	pool := NewWorkerPool(0)
	defer pool.Close()

	work := make([]func(), 1000)
	for i := range work {
		work[i] = func() {
			// Minimal work
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

// BenchmarkWorkerPool_ExecuteAll_WithWork benchmarks executing with actual workload.
func BenchmarkWorkerPool_ExecuteAll_WithWork(b *testing.B) {
	pool := NewWorkerPool(0)
	defer pool.Close()

	// Simulate tile-like work
	buffers := make([][]byte, 100)
	for i := range buffers {
		buffers[i] = make([]byte, TileBytes)
	}

	work := make([]func(), 100)
	for i := range work {
		buf := buffers[i]
		work[i] = func() {
			// Simulate clear operation
			clear(buf)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

// =============================================================================
// Component Benchmarks - TilePool
// =============================================================================

// BenchmarkTilePool_GetPut_FullTile benchmarks get/put cycle for full-size tiles.
func BenchmarkTilePool_GetPut_FullTile(b *testing.B) {
	pool := NewTilePool()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tile := pool.Get(TileWidth, TileHeight)
		pool.Put(tile)
	}
}

// BenchmarkTilePool_GetPut_EdgeTile benchmarks get/put cycle for edge tiles.
func BenchmarkTilePool_GetPut_EdgeTile(b *testing.B) {
	pool := NewTilePool()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tile := pool.Get(36, 36) // Common edge tile size
		pool.Put(tile)
	}
}

// BenchmarkTilePool_GetPut_Parallel benchmarks concurrent get/put operations.
func BenchmarkTilePool_GetPut_Parallel(b *testing.B) {
	pool := NewTilePool()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tile := pool.Get(TileWidth, TileHeight)
			pool.Put(tile)
		}
	})
}

// =============================================================================
// Component Benchmarks - DirtyRegion
// =============================================================================

// BenchmarkDirtyRegion_Mark_HD benchmarks marking a single tile dirty for HD resolution.
func BenchmarkDirtyRegion_Mark_HD(b *testing.B) {
	// HD resolution: 30x17 tiles
	dirty := NewDirtyRegion(30, 17)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dirty.Mark(15, 8) // Middle tile
	}
}

// BenchmarkDirtyRegion_MarkRect_Small benchmarks marking a small rectangle.
func BenchmarkDirtyRegion_MarkRect_Small(b *testing.B) {
	dirty := NewDirtyRegion(30, 17)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dirty.MarkRect(100, 100, 64, 64) // ~1 tile
	}
}

// BenchmarkDirtyRegion_MarkRect_Medium benchmarks marking a medium rectangle.
func BenchmarkDirtyRegion_MarkRect_Medium(b *testing.B) {
	dirty := NewDirtyRegion(30, 17)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dirty.MarkRect(100, 100, 256, 256) // ~16 tiles
	}
}

// BenchmarkDirtyRegion_MarkRect_Large benchmarks marking a large rectangle.
func BenchmarkDirtyRegion_MarkRect_Large(b *testing.B) {
	dirty := NewDirtyRegion(30, 17)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dirty.MarkRect(0, 0, 960, 540) // Quarter screen
	}
}

// BenchmarkDirtyRegion_GetDirtyTiles_AllDirty_HD benchmarks getting dirty tiles when all are dirty for HD.
func BenchmarkDirtyRegion_GetDirtyTiles_AllDirty_HD(b *testing.B) {
	dirty := NewDirtyRegion(30, 17)
	dirty.MarkAll()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dirty.MarkAll() // Reset for next iteration
		_ = dirty.GetAndClear()
	}
}

// BenchmarkDirtyRegion_GetDirtyTiles_PartialDirty_HD benchmarks getting dirty tiles when some are dirty for HD.
func BenchmarkDirtyRegion_GetDirtyTiles_PartialDirty_HD(b *testing.B) {
	dirty := NewDirtyRegion(30, 17)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Mark 10% of tiles dirty
		for j := 0; j < 51; j++ {
			dirty.Mark(j%30, j/30)
		}
		_ = dirty.GetAndClear()
	}
}

// BenchmarkDirtyRegion_GetAndClear_HD benchmarks atomic get and clear operation for HD.
func BenchmarkDirtyRegion_GetAndClear_HD(b *testing.B) {
	dirty := NewDirtyRegion(30, 17)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dirty.MarkAll()
		_ = dirty.GetAndClear()
	}
}

// BenchmarkDirtyRegion_Count_HD benchmarks counting dirty tiles for HD.
func BenchmarkDirtyRegion_Count_HD(b *testing.B) {
	dirty := NewDirtyRegion(30, 17)
	dirty.MarkAll()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = dirty.Count()
	}
}

// BenchmarkDirtyRegion_IsDirty_HD benchmarks checking if a tile is dirty for HD.
func BenchmarkDirtyRegion_IsDirty_HD(b *testing.B) {
	dirty := NewDirtyRegion(30, 17)
	dirty.MarkAll()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = dirty.IsDirty(15, 8)
	}
}

// BenchmarkDirtyRegion_Parallel benchmarks concurrent dirty region operations.
func BenchmarkDirtyRegion_Parallel(b *testing.B) {
	dirty := NewDirtyRegion(30, 17)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			dirty.Mark(i%30, (i/30)%17)
			_ = dirty.IsDirty(i%30, (i/30)%17)
			i++
		}
	})
}

// =============================================================================
// Component Benchmarks - ParallelRasterizer
// =============================================================================

// BenchmarkParallelRasterizer_Clear_HD benchmarks Clear operation for HD resolution.
func BenchmarkParallelRasterizer_Clear_HD(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// BenchmarkParallelRasterizer_Clear_4K benchmarks Clear operation for 4K resolution.
func BenchmarkParallelRasterizer_Clear_4K(b *testing.B) {
	pr := NewParallelRasterizer(3840, 2160)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// BenchmarkParallelRasterizer_FillRect_Small_HD benchmarks small rectangle fill on HD.
func BenchmarkParallelRasterizer_FillRect_Small_HD(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.FillRect(100, 100, 64, 64, red) // Single tile
	}
}

// BenchmarkParallelRasterizer_FillRect_Medium_HD benchmarks medium rectangle fill on HD.
func BenchmarkParallelRasterizer_FillRect_Medium_HD(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.FillRect(100, 100, 256, 256, red) // ~16 tiles
	}
}

// BenchmarkParallelRasterizer_FillRect_Large_HD benchmarks large rectangle fill on HD.
func BenchmarkParallelRasterizer_FillRect_Large_HD(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.FillRect(0, 0, 960, 540, red) // Quarter screen
	}
}

// BenchmarkParallelRasterizer_FillRect_FullScreen_HD benchmarks full screen fill on HD.
func BenchmarkParallelRasterizer_FillRect_FullScreen_HD(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.FillRect(0, 0, 1920, 1080, red)
	}
}

// BenchmarkParallelRasterizer_Composite_HD benchmarks Composite operation for HD.
func BenchmarkParallelRasterizer_Composite_HD(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	pr.Clear(color.White)

	stride := 1920 * 4
	dst := make([]byte, 1080*stride)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Composite(dst, stride)
	}
}

// BenchmarkParallelRasterizer_Composite_4K benchmarks Composite operation for 4K.
func BenchmarkParallelRasterizer_Composite_4K(b *testing.B) {
	pr := NewParallelRasterizer(3840, 2160)
	defer pr.Close()

	pr.Clear(color.White)

	stride := 3840 * 4
	dst := make([]byte, 2160*stride)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Composite(dst, stride)
	}
}

// BenchmarkParallelRasterizer_CompositeDirty_10Percent benchmarks compositing 10% dirty tiles.
func BenchmarkParallelRasterizer_CompositeDirty_10Percent(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	pr.Clear(color.White)

	stride := 1920 * 4
	dst := make([]byte, 1080*stride)

	// Mark 10% of tiles dirty
	tiles := pr.Grid().AllTiles()
	dirtyCount := len(tiles) / 10

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.ClearDirty()
		for j := 0; j < dirtyCount; j++ {
			tiles[j].Dirty = true
		}
		pr.CompositeDirty(dst, stride)
	}
}

// BenchmarkParallelRasterizer_CompositeDirty_50Percent benchmarks compositing 50% dirty tiles.
func BenchmarkParallelRasterizer_CompositeDirty_50Percent(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	pr.Clear(color.White)

	stride := 1920 * 4
	dst := make([]byte, 1080*stride)

	// Mark 50% of tiles dirty
	tiles := pr.Grid().AllTiles()
	dirtyCount := len(tiles) / 2

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.ClearDirty()
		for j := 0; j < dirtyCount; j++ {
			tiles[j].Dirty = true
		}
		pr.CompositeDirty(dst, stride)
	}
}

// =============================================================================
// Hot Path Allocation Tests
// =============================================================================

// BenchmarkHotPath_TileGridLookup verifies tile lookup has zero allocations.
func BenchmarkHotPath_TileGridLookup(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = grid.TileAt(15, 8)
		_ = grid.TileAtPixel(960, 540)
	}
}

// BenchmarkHotPath_DirtyRegionMark verifies dirty marking has zero allocations.
func BenchmarkHotPath_DirtyRegionMark(b *testing.B) {
	dirty := NewDirtyRegion(30, 17)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dirty.Mark(15, 8)
		_ = dirty.IsDirty(15, 8)
	}
}

// BenchmarkHotPath_TilePoolReuse verifies tile pool reuse has zero allocations after warmup.
func BenchmarkHotPath_TilePoolReuse(b *testing.B) {
	pool := NewTilePool()

	// Warmup - fill the pool
	tiles := make([]*Tile, 100)
	for i := range tiles {
		tiles[i] = pool.Get(TileWidth, TileHeight)
	}
	for _, t := range tiles {
		pool.Put(t)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tile := pool.Get(TileWidth, TileHeight)
		pool.Put(tile)
	}
}
