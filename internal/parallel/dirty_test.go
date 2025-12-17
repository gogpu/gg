package parallel

import (
	"sync"
	"testing"
)

// =============================================================================
// DirtyRegion Basic Tests
// =============================================================================

func TestDirtyRegion_Create(t *testing.T) {
	tests := []struct {
		name   string
		tilesX int
		tilesY int
		wantOK bool
	}{
		{"valid small", 4, 4, true},
		{"valid large", 100, 100, true},
		{"valid non-square", 100, 10, true},
		{"valid single", 1, 1, true},
		{"invalid zero x", 0, 10, false},
		{"invalid zero y", 10, 0, false},
		{"invalid negative x", -1, 10, false},
		{"invalid negative y", 10, -1, false},
		{"invalid both zero", 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dr := NewDirtyRegion(tt.tilesX, tt.tilesY)
			gotOK := dr != nil

			if gotOK != tt.wantOK {
				t.Errorf("NewDirtyRegion(%d, %d) = %v, want nil=%v",
					tt.tilesX, tt.tilesY, gotOK, !tt.wantOK)
			}

			if dr == nil {
				return
			}

			// Validate dimensions
			if dr.TilesX() != tt.tilesX {
				t.Errorf("TilesX() = %d, want %d", dr.TilesX(), tt.tilesX)
			}
			if dr.TilesY() != tt.tilesY {
				t.Errorf("TilesY() = %d, want %d", dr.TilesY(), tt.tilesY)
			}
			if dr.TotalTiles() != tt.tilesX*tt.tilesY {
				t.Errorf("TotalTiles() = %d, want %d", dr.TotalTiles(), tt.tilesX*tt.tilesY)
			}
			// New region should be empty (all clean)
			if !dr.IsEmpty() {
				t.Error("New DirtyRegion should be empty")
			}
		})
	}
}

func TestDirtyRegion_Mark(t *testing.T) {
	dr := NewDirtyRegion(4, 4)

	// Mark a single tile
	dr.Mark(1, 2)

	if !dr.IsDirty(1, 2) {
		t.Error("Mark(1, 2) did not set dirty flag")
	}

	// Other tiles should not be dirty
	if dr.IsDirty(0, 0) {
		t.Error("Tile (0, 0) should not be dirty")
	}
	if dr.IsDirty(3, 3) {
		t.Error("Tile (3, 3) should not be dirty")
	}

	// Count should be 1
	if dr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", dr.Count())
	}
}

func TestDirtyRegion_MarkOutOfBounds(t *testing.T) {
	dr := NewDirtyRegion(4, 4)

	// These should not panic and should be no-ops
	dr.Mark(-1, 0)
	dr.Mark(0, -1)
	dr.Mark(4, 0)
	dr.Mark(0, 4)
	dr.Mark(100, 100)

	// Region should still be empty
	if !dr.IsEmpty() {
		t.Error("Out of bounds marks should not set any dirty flags")
	}
}

func TestDirtyRegion_MarkRect(t *testing.T) {
	tests := []struct {
		name      string
		tilesX    int
		tilesY    int
		x, y, w   int
		h         int
		wantCount int
	}{
		{
			name:   "single tile exact",
			tilesX: 4, tilesY: 4,
			x: 0, y: 0, w: TileWidth, h: TileHeight,
			wantCount: 1,
		},
		{
			name:   "two tiles horizontal",
			tilesX: 4, tilesY: 4,
			x: TileWidth / 2, y: 0, w: TileWidth, h: TileHeight / 2,
			wantCount: 2,
		},
		{
			name:   "four tiles",
			tilesX: 4, tilesY: 4,
			x: TileWidth / 2, y: TileHeight / 2, w: TileWidth, h: TileHeight,
			wantCount: 4,
		},
		{
			name:   "all tiles",
			tilesX: 4, tilesY: 4,
			x: 0, y: 0, w: 4 * TileWidth, h: 4 * TileHeight,
			wantCount: 16,
		},
		{
			name:   "zero width",
			tilesX: 4, tilesY: 4,
			x: 0, y: 0, w: 0, h: TileHeight,
			wantCount: 0,
		},
		{
			name:   "negative width",
			tilesX: 4, tilesY: 4,
			x: 0, y: 0, w: -10, h: TileHeight,
			wantCount: 0,
		},
		{
			name:   "partially outside left",
			tilesX: 4, tilesY: 4,
			x: -32, y: 0, w: TileWidth, h: TileHeight,
			wantCount: 1,
		},
		{
			name:   "completely outside",
			tilesX: 4, tilesY: 4,
			x: 1000, y: 0, w: TileWidth, h: TileHeight,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dr := NewDirtyRegion(tt.tilesX, tt.tilesY)
			dr.MarkRect(tt.x, tt.y, tt.w, tt.h)

			if dr.Count() != tt.wantCount {
				t.Errorf("MarkRect(%d,%d,%d,%d) Count() = %d, want %d",
					tt.x, tt.y, tt.w, tt.h, dr.Count(), tt.wantCount)
			}
		})
	}
}

func TestDirtyRegion_MarkAll(t *testing.T) {
	tests := []struct {
		name   string
		tilesX int
		tilesY int
	}{
		{"small grid", 4, 4},
		{"non-multiple of 64", 10, 7}, // 70 tiles, partial word
		{"large grid", 100, 100},
		{"single tile", 1, 1},
		{"exactly 64 tiles", 8, 8},
		{"exactly 128 tiles", 16, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dr := NewDirtyRegion(tt.tilesX, tt.tilesY)
			dr.MarkAll()

			expectedCount := tt.tilesX * tt.tilesY
			if dr.Count() != expectedCount {
				t.Errorf("After MarkAll() Count() = %d, want %d", dr.Count(), expectedCount)
			}

			if dr.IsEmpty() {
				t.Error("After MarkAll() IsEmpty() should be false")
			}

			// Verify all tiles are marked
			for ty := 0; ty < tt.tilesY; ty++ {
				for tx := 0; tx < tt.tilesX; tx++ {
					if !dr.IsDirty(tx, ty) {
						t.Errorf("Tile (%d, %d) should be dirty after MarkAll()", tx, ty)
					}
				}
			}
		})
	}
}

func TestDirtyRegion_Clear(t *testing.T) {
	dr := NewDirtyRegion(4, 4)

	// Mark all dirty
	dr.MarkAll()
	if dr.IsEmpty() {
		t.Error("Should not be empty after MarkAll()")
	}

	// Clear all
	dr.Clear()
	if !dr.IsEmpty() {
		t.Error("Should be empty after Clear()")
	}
	if dr.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after Clear()", dr.Count())
	}
}

func TestDirtyRegion_IsDirty(t *testing.T) {
	dr := NewDirtyRegion(4, 4)

	// Check out of bounds returns false
	if dr.IsDirty(-1, 0) {
		t.Error("IsDirty(-1, 0) should return false")
	}
	if dr.IsDirty(0, -1) {
		t.Error("IsDirty(0, -1) should return false")
	}
	if dr.IsDirty(4, 0) {
		t.Error("IsDirty(4, 0) should return false")
	}
	if dr.IsDirty(0, 4) {
		t.Error("IsDirty(0, 4) should return false")
	}

	// Mark and check
	dr.Mark(2, 3)
	if !dr.IsDirty(2, 3) {
		t.Error("IsDirty(2, 3) should return true after Mark")
	}
}

func TestDirtyRegion_IsEmpty(t *testing.T) {
	dr := NewDirtyRegion(4, 4)

	if !dr.IsEmpty() {
		t.Error("New region should be empty")
	}

	dr.Mark(0, 0)
	if dr.IsEmpty() {
		t.Error("Region with marked tile should not be empty")
	}

	dr.Clear()
	if !dr.IsEmpty() {
		t.Error("Region after Clear should be empty")
	}
}

func TestDirtyRegion_Count(t *testing.T) {
	dr := NewDirtyRegion(10, 10) // 100 tiles

	if dr.Count() != 0 {
		t.Errorf("New region Count() = %d, want 0", dr.Count())
	}

	// Mark specific tiles
	dr.Mark(0, 0)
	dr.Mark(5, 5)
	dr.Mark(9, 9)
	if dr.Count() != 3 {
		t.Errorf("After marking 3 tiles Count() = %d, want 3", dr.Count())
	}

	// Mark same tile again (should be idempotent)
	dr.Mark(0, 0)
	if dr.Count() != 3 {
		t.Errorf("After re-marking same tile Count() = %d, want 3", dr.Count())
	}

	// Mark all
	dr.MarkAll()
	if dr.Count() != 100 {
		t.Errorf("After MarkAll() Count() = %d, want 100", dr.Count())
	}
}

func TestDirtyRegion_GetAndClear(t *testing.T) {
	dr := NewDirtyRegion(4, 4)

	// Mark specific tiles
	dr.Mark(0, 0)
	dr.Mark(1, 1)
	dr.Mark(3, 3)

	dirty := dr.GetAndClear()

	// Check we got 3 tiles
	if len(dirty) != 3 {
		t.Errorf("GetAndClear() returned %d tiles, want 3", len(dirty))
	}

	// Region should now be empty
	if !dr.IsEmpty() {
		t.Error("Region should be empty after GetAndClear()")
	}

	// Verify we got the right coordinates
	expectedTiles := map[[2]int]bool{
		{0, 0}: true,
		{1, 1}: true,
		{3, 3}: true,
	}

	for _, coord := range dirty {
		if !expectedTiles[coord] {
			t.Errorf("Unexpected tile returned: (%d, %d)", coord[0], coord[1])
		}
		delete(expectedTiles, coord)
	}

	if len(expectedTiles) > 0 {
		t.Errorf("Missing tiles: %v", expectedTiles)
	}
}

func TestDirtyRegion_ForEachDirty(t *testing.T) {
	dr := NewDirtyRegion(4, 4)

	// Mark specific tiles
	dr.Mark(0, 0)
	dr.Mark(2, 1)
	dr.Mark(3, 3)

	visited := make(map[[2]int]bool)
	dr.ForEachDirty(func(tx, ty int) {
		visited[[2]int{tx, ty}] = true
	})

	if len(visited) != 3 {
		t.Errorf("ForEachDirty visited %d tiles, want 3", len(visited))
	}

	// Region should NOT be cleared (unlike GetAndClear)
	if dr.IsEmpty() {
		t.Error("ForEachDirty should not clear the region")
	}

	// Test nil function doesn't panic
	dr.ForEachDirty(nil)
}

func TestDirtyRegion_Resize(t *testing.T) {
	dr := NewDirtyRegion(4, 4)
	dr.Mark(1, 1)

	// Resize creates a NEW region
	newDR := dr.Resize(8, 8)

	if newDR == nil {
		t.Fatal("Resize returned nil")
	}

	// New region should be all dirty
	if newDR.TilesX() != 8 || newDR.TilesY() != 8 {
		t.Errorf("New region dimensions = %dx%d, want 8x8", newDR.TilesX(), newDR.TilesY())
	}

	if newDR.Count() != 64 {
		t.Errorf("New region Count() = %d, want 64 (all dirty)", newDR.Count())
	}

	// Original should be unchanged
	if dr.Count() != 1 {
		t.Errorf("Original region Count() = %d, want 1 (unchanged)", dr.Count())
	}

	// Resize to invalid returns nil
	invalidDR := dr.Resize(0, 0)
	if invalidDR != nil {
		t.Error("Resize to invalid dimensions should return nil")
	}
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestDirtyRegion_LargeGrid(t *testing.T) {
	// 100x100 = 10000 tiles = 157 words (156 full + 1 partial)
	dr := NewDirtyRegion(100, 100)

	if dr.TotalTiles() != 10000 {
		t.Errorf("TotalTiles() = %d, want 10000", dr.TotalTiles())
	}

	// Test marking corners
	dr.Mark(0, 0)
	dr.Mark(99, 0)
	dr.Mark(0, 99)
	dr.Mark(99, 99)

	if dr.Count() != 4 {
		t.Errorf("After marking 4 corners Count() = %d, want 4", dr.Count())
	}

	// Test MarkAll
	dr.MarkAll()
	if dr.Count() != 10000 {
		t.Errorf("After MarkAll() Count() = %d, want 10000", dr.Count())
	}

	// Test GetAndClear returns all
	dirty := dr.GetAndClear()
	if len(dirty) != 10000 {
		t.Errorf("GetAndClear() returned %d tiles, want 10000", len(dirty))
	}
}

func TestDirtyRegion_SingleTile(t *testing.T) {
	dr := NewDirtyRegion(1, 1)

	if dr.TotalTiles() != 1 {
		t.Errorf("TotalTiles() = %d, want 1", dr.TotalTiles())
	}

	dr.Mark(0, 0)
	if !dr.IsDirty(0, 0) {
		t.Error("Single tile should be dirty")
	}

	dr.Clear()
	if dr.IsDirty(0, 0) {
		t.Error("Single tile should be clean after Clear")
	}
}

func TestDirtyRegion_NonSquare(t *testing.T) {
	// 100x10 = 1000 tiles
	dr := NewDirtyRegion(100, 10)

	if dr.TotalTiles() != 1000 {
		t.Errorf("TotalTiles() = %d, want 1000", dr.TotalTiles())
	}

	// Mark corners
	dr.Mark(0, 0)
	dr.Mark(99, 0)
	dr.Mark(0, 9)
	dr.Mark(99, 9)

	if dr.Count() != 4 {
		t.Errorf("Count() = %d, want 4", dr.Count())
	}

	// Verify each corner
	if !dr.IsDirty(0, 0) || !dr.IsDirty(99, 0) || !dr.IsDirty(0, 9) || !dr.IsDirty(99, 9) {
		t.Error("Not all corners are marked dirty")
	}
}

// =============================================================================
// Concurrency Tests
// =============================================================================

func TestDirtyRegion_ConcurrentMark(t *testing.T) {
	dr := NewDirtyRegion(100, 100)
	numGoroutines := 20
	marksPerGoroutine := 500

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < marksPerGoroutine; i++ {
				tx := (id*marksPerGoroutine + i) % 100
				ty := (id*marksPerGoroutine + i) / 100 % 100
				dr.Mark(tx, ty)
			}
		}(g)
	}

	wg.Wait()

	// Should have some dirty tiles (exact count depends on overlap)
	count := dr.Count()
	if count == 0 {
		t.Error("Expected some dirty tiles after concurrent marks")
	}

	t.Logf("Concurrent mark: %d dirty tiles", count)
}

func TestDirtyRegion_ConcurrentMarkAndClear(t *testing.T) {
	dr := NewDirtyRegion(50, 50)
	numIterations := 1000

	var wg sync.WaitGroup
	wg.Add(3)

	// Marker goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < numIterations; i++ {
			dr.Mark(i%50, i%50)
		}
	}()

	// Reader goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < numIterations; i++ {
			_ = dr.IsDirty(i%50, i%50)
			_ = dr.Count()
			_ = dr.IsEmpty()
		}
	}()

	// GetAndClear goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < numIterations/10; i++ {
			_ = dr.GetAndClear()
		}
	}()

	wg.Wait()
	// No race conditions = test passes
}

func TestDirtyRegion_ConcurrentGetAndClear(t *testing.T) {
	dr := NewDirtyRegion(10, 10)
	numGoroutines := 10

	// Mark all tiles
	dr.MarkAll()

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	totalCollected := make(chan int, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			dirty := dr.GetAndClear()
			totalCollected <- len(dirty)
		}()
	}

	wg.Wait()
	close(totalCollected)

	// Sum all collected tiles
	total := 0
	for count := range totalCollected {
		total += count
	}

	// Each tile should be returned exactly once across all goroutines
	if total != 100 {
		t.Errorf("Total tiles collected = %d, want 100 (no duplicates, no misses)", total)
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkDirtyRegion_Mark(b *testing.B) {
	dr := NewDirtyRegion(100, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dr.Mark(i%100, (i/100)%100)
	}
}

func BenchmarkDirtyRegion_MarkParallel(b *testing.B) {
	dr := NewDirtyRegion(100, 100)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			dr.Mark(i%100, (i/100)%100)
			i++
		}
	})
}

func BenchmarkDirtyRegion_IsDirty(b *testing.B) {
	dr := NewDirtyRegion(100, 100)
	dr.MarkAll()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = dr.IsDirty(i%100, (i/100)%100)
	}
}

func BenchmarkDirtyRegion_GetAndClear(b *testing.B) {
	b.Run("sparse_10pct", func(b *testing.B) {
		dr := NewDirtyRegion(100, 100)
		// Mark ~10% of tiles
		for i := 0; i < 1000; i++ {
			dr.Mark(i%100, i/100)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			dr.MarkAll() // Reset for next iteration
			_ = dr.GetAndClear()
		}
	})

	b.Run("dense_all", func(b *testing.B) {
		dr := NewDirtyRegion(100, 100)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			dr.MarkAll()
			_ = dr.GetAndClear()
		}
	})

	b.Run("empty", func(b *testing.B) {
		dr := NewDirtyRegion(100, 100)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = dr.GetAndClear()
		}
	})
}

func BenchmarkDirtyRegion_Count(b *testing.B) {
	b.Run("empty", func(b *testing.B) {
		dr := NewDirtyRegion(100, 100)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = dr.Count()
		}
	})

	b.Run("sparse", func(b *testing.B) {
		dr := NewDirtyRegion(100, 100)
		for i := 0; i < 100; i++ {
			dr.Mark(i, i%100)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = dr.Count()
		}
	})

	b.Run("all_dirty", func(b *testing.B) {
		dr := NewDirtyRegion(100, 100)
		dr.MarkAll()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = dr.Count()
		}
	})
}

func BenchmarkDirtyRegion_MarkAll(b *testing.B) {
	dr := NewDirtyRegion(100, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dr.MarkAll()
	}
}

func BenchmarkDirtyRegion_Clear(b *testing.B) {
	dr := NewDirtyRegion(100, 100)
	dr.MarkAll()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dr.Clear()
	}
}

func BenchmarkDirtyRegion_IsEmpty(b *testing.B) {
	b.Run("empty", func(b *testing.B) {
		dr := NewDirtyRegion(100, 100)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = dr.IsEmpty()
		}
	})

	b.Run("not_empty", func(b *testing.B) {
		dr := NewDirtyRegion(100, 100)
		dr.Mark(50, 50)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = dr.IsEmpty()
		}
	})
}

func BenchmarkDirtyRegion_ForEachDirty(b *testing.B) {
	b.Run("sparse_100", func(b *testing.B) {
		dr := NewDirtyRegion(100, 100)
		for i := 0; i < 100; i++ {
			dr.Mark(i, i%100)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			dr.ForEachDirty(func(tx, ty int) {
				_ = tx + ty
			})
		}
	})

	b.Run("all_dirty", func(b *testing.B) {
		dr := NewDirtyRegion(30, 30) // 900 tiles for reasonable benchmark time
		dr.MarkAll()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			dr.ForEachDirty(func(tx, ty int) {
				_ = tx + ty
			})
		}
	})
}

func BenchmarkDirtyRegion_MarkRect(b *testing.B) {
	dr := NewDirtyRegion(100, 100)

	b.Run("small_rect", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			dr.Clear()
			dr.MarkRect(32, 32, TileWidth, TileHeight) // ~1 tile
		}
	})

	b.Run("medium_rect", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			dr.Clear()
			dr.MarkRect(0, 0, TileWidth*5, TileHeight*5) // ~25 tiles
		}
	})

	b.Run("large_rect", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			dr.Clear()
			dr.MarkRect(0, 0, TileWidth*50, TileHeight*50) // ~2500 tiles
		}
	})
}
