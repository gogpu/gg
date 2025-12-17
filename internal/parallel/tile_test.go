package parallel

import (
	"sync"
	"testing"
)

// =============================================================================
// Tile Tests
// =============================================================================

func TestTile_Constants(t *testing.T) {
	// Verify constants match expected values
	if TileWidth != 64 {
		t.Errorf("TileWidth = %d, want 64", TileWidth)
	}
	if TileHeight != 64 {
		t.Errorf("TileHeight = %d, want 64", TileHeight)
	}
	if TilePixels != 64*64 {
		t.Errorf("TilePixels = %d, want %d", TilePixels, 64*64)
	}
	if TileBytes != 64*64*4 {
		t.Errorf("TileBytes = %d, want %d", TileBytes, 64*64*4)
	}
}

func TestTile_Bounds(t *testing.T) {
	tests := []struct {
		name         string
		tile         Tile
		wantX, wantY int
		wantW, wantH int
	}{
		{
			name:  "first tile",
			tile:  Tile{X: 0, Y: 0, Width: 64, Height: 64},
			wantX: 0, wantY: 0, wantW: 64, wantH: 64,
		},
		{
			name:  "second row first column",
			tile:  Tile{X: 0, Y: 1, Width: 64, Height: 64},
			wantX: 0, wantY: 64, wantW: 64, wantH: 64,
		},
		{
			name:  "edge tile",
			tile:  Tile{X: 2, Y: 3, Width: 32, Height: 16},
			wantX: 128, wantY: 192, wantW: 32, wantH: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, w, h := tt.tile.Bounds()
			if x != tt.wantX || y != tt.wantY || w != tt.wantW || h != tt.wantH {
				t.Errorf("Bounds() = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					x, y, w, h, tt.wantX, tt.wantY, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestTile_PixelOffset(t *testing.T) {
	tile := &Tile{X: 0, Y: 0, Width: 64, Height: 64, Data: make([]byte, TileBytes)}

	tests := []struct {
		name   string
		px, py int
		want   int
	}{
		{"top-left", 0, 0, 0},
		{"second pixel", 1, 0, 4},
		{"second row", 0, 1, 64 * 4},
		{"middle", 32, 32, (32*64 + 32) * 4},
		{"out of bounds negative x", -1, 0, -1},
		{"out of bounds negative y", 0, -1, -1},
		{"out of bounds x", 64, 0, -1},
		{"out of bounds y", 0, 64, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tile.PixelOffset(tt.px, tt.py)
			if got != tt.want {
				t.Errorf("PixelOffset(%d,%d) = %d, want %d", tt.px, tt.py, got, tt.want)
			}
		})
	}
}

func TestTile_CanvasPixelOffset(t *testing.T) {
	// Tile at position (1, 2) -> starts at canvas pixel (64, 128)
	tile := &Tile{X: 1, Y: 2, Width: 64, Height: 64, Data: make([]byte, TileBytes)}

	tests := []struct {
		name   string
		cx, cy int
		want   int
	}{
		{"tile origin", 64, 128, 0},
		{"inside tile", 65, 128, 4},
		{"inside tile row 2", 64, 129, 64 * 4},
		{"outside tile left", 63, 128, -1},
		{"outside tile top", 64, 127, -1},
		{"outside tile right", 128, 128, -1},
		{"outside tile bottom", 64, 192, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tile.CanvasPixelOffset(tt.cx, tt.cy)
			if got != tt.want {
				t.Errorf("CanvasPixelOffset(%d,%d) = %d, want %d", tt.cx, tt.cy, got, tt.want)
			}
		})
	}
}

func TestTile_Contains(t *testing.T) {
	tile := &Tile{X: 1, Y: 1, Width: 64, Height: 64}

	tests := []struct {
		name   string
		cx, cy int
		want   bool
	}{
		{"inside", 96, 96, true},
		{"top-left corner", 64, 64, true},
		{"bottom-right inside", 127, 127, true},
		{"outside left", 63, 96, false},
		{"outside right", 128, 96, false},
		{"outside top", 96, 63, false},
		{"outside bottom", 96, 128, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tile.Contains(tt.cx, tt.cy)
			if got != tt.want {
				t.Errorf("Contains(%d,%d) = %v, want %v", tt.cx, tt.cy, got, tt.want)
			}
		})
	}
}

func TestTile_Reset(t *testing.T) {
	tile := &Tile{
		X:      1,
		Y:      2,
		Width:  64,
		Height: 64,
		Dirty:  true,
		Data:   make([]byte, TileBytes),
	}

	// Fill with non-zero data
	for i := range tile.Data {
		tile.Data[i] = 0xFF
	}

	tile.Reset()

	if tile.Dirty {
		t.Error("Dirty not reset to false")
	}

	for i, b := range tile.Data {
		if b != 0 {
			t.Errorf("Data[%d] = %d, want 0", i, b)
			break
		}
	}
}

func TestTile_Stride(t *testing.T) {
	tests := []struct {
		width int
		want  int
	}{
		{64, 256},
		{32, 128},
		{16, 64},
	}

	for _, tt := range tests {
		tile := &Tile{Width: tt.width}
		got := tile.Stride()
		if got != tt.want {
			t.Errorf("Stride() with width=%d = %d, want %d", tt.width, got, tt.want)
		}
	}
}

func TestTile_ByteSize(t *testing.T) {
	tests := []struct {
		width, height int
		want          int
	}{
		{64, 64, 16384},
		{32, 64, 8192},
		{64, 32, 8192},
		{32, 16, 2048},
	}

	for _, tt := range tests {
		tile := &Tile{Width: tt.width, Height: tt.height}
		got := tile.ByteSize()
		if got != tt.want {
			t.Errorf("ByteSize() with %dx%d = %d, want %d", tt.width, tt.height, got, tt.want)
		}
	}
}

// =============================================================================
// TileGrid Tests
// =============================================================================

func TestTileGrid_Create(t *testing.T) {
	tests := []struct {
		name           string
		width, height  int
		wantTilesX     int
		wantTilesY     int
		wantTotalTiles int
	}{
		{
			name:  "exact multiple",
			width: 128, height: 128,
			wantTilesX: 2, wantTilesY: 2,
			wantTotalTiles: 4,
		},
		{
			name:  "single tile",
			width: 64, height: 64,
			wantTilesX: 1, wantTilesY: 1,
			wantTotalTiles: 1,
		},
		{
			name:  "partial width",
			width: 100, height: 64,
			wantTilesX: 2, wantTilesY: 1,
			wantTotalTiles: 2,
		},
		{
			name:  "partial height",
			width: 64, height: 100,
			wantTilesX: 1, wantTilesY: 2,
			wantTotalTiles: 2,
		},
		{
			name:  "large canvas",
			width: 1920, height: 1080,
			wantTilesX: 30, wantTilesY: 17,
			wantTotalTiles: 510,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grid := NewTileGrid(tt.width, tt.height)
			defer grid.Close()

			if grid.TilesX() != tt.wantTilesX {
				t.Errorf("TilesX() = %d, want %d", grid.TilesX(), tt.wantTilesX)
			}
			if grid.TilesY() != tt.wantTilesY {
				t.Errorf("TilesY() = %d, want %d", grid.TilesY(), tt.wantTilesY)
			}
			if grid.TileCount() != tt.wantTotalTiles {
				t.Errorf("TileCount() = %d, want %d", grid.TileCount(), tt.wantTotalTiles)
			}
			if grid.Width() != tt.width {
				t.Errorf("Width() = %d, want %d", grid.Width(), tt.width)
			}
			if grid.Height() != tt.height {
				t.Errorf("Height() = %d, want %d", grid.Height(), tt.height)
			}
		})
	}
}

func TestTileGrid_CreateInvalid(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
	}{
		{"zero width", 0, 100},
		{"zero height", 100, 0},
		{"negative width", -10, 100},
		{"negative height", 100, -10},
		{"both zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grid := NewTileGrid(tt.width, tt.height)
			if grid.TileCount() != 0 {
				t.Errorf("TileCount() = %d, want 0 for invalid dimensions", grid.TileCount())
			}
		})
	}
}

func TestTileGrid_EdgeTiles(t *testing.T) {
	// Canvas 100x100 should have:
	// - Tile (0,0): 64x64
	// - Tile (1,0): 36x64
	// - Tile (0,1): 64x36
	// - Tile (1,1): 36x36
	grid := NewTileGrid(100, 100)
	defer grid.Close()

	tests := []struct {
		tx, ty int
		wantW  int
		wantH  int
	}{
		{0, 0, 64, 64},
		{1, 0, 36, 64},
		{0, 1, 64, 36},
		{1, 1, 36, 36},
	}

	for _, tt := range tests {
		tile := grid.TileAt(tt.tx, tt.ty)
		if tile == nil {
			t.Errorf("TileAt(%d,%d) = nil", tt.tx, tt.ty)
			continue
		}
		if tile.Width != tt.wantW || tile.Height != tt.wantH {
			t.Errorf("Tile(%d,%d) dimensions = %dx%d, want %dx%d",
				tt.tx, tt.ty, tile.Width, tile.Height, tt.wantW, tt.wantH)
		}
	}
}

func TestTileGrid_TileAt(t *testing.T) {
	grid := NewTileGrid(128, 128)
	defer grid.Close()

	tests := []struct {
		name   string
		tx, ty int
		wantOK bool
	}{
		{"valid (0,0)", 0, 0, true},
		{"valid (1,1)", 1, 1, true},
		{"out of bounds x", 2, 0, false},
		{"out of bounds y", 0, 2, false},
		{"negative x", -1, 0, false},
		{"negative y", 0, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tile := grid.TileAt(tt.tx, tt.ty)
			gotOK := tile != nil
			if gotOK != tt.wantOK {
				t.Errorf("TileAt(%d,%d) returned tile=%v, wantOK=%v", tt.tx, tt.ty, gotOK, tt.wantOK)
			}
		})
	}
}

func TestTileGrid_TileAtPixel(t *testing.T) {
	grid := NewTileGrid(200, 200)
	defer grid.Close()

	tests := []struct {
		name   string
		px, py int
		wantTX int
		wantTY int
		wantOK bool
	}{
		{"origin", 0, 0, 0, 0, true},
		{"inside first tile", 32, 32, 0, 0, true},
		{"second tile x", 64, 0, 1, 0, true},
		{"second tile y", 0, 64, 0, 1, true},
		{"third tile x", 128, 0, 2, 0, true},
		{"edge of canvas", 199, 199, 3, 3, true},
		{"out of bounds x", 200, 0, 0, 0, false},
		{"out of bounds y", 0, 200, 0, 0, false},
		{"negative x", -1, 0, 0, 0, false},
		{"negative y", 0, -1, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tile := grid.TileAtPixel(tt.px, tt.py)
			if tt.wantOK && tile == nil {
				t.Errorf("TileAtPixel(%d,%d) = nil, want tile", tt.px, tt.py)
				return
			}
			if tt.wantOK && (tile.X != tt.wantTX || tile.Y != tt.wantTY) {
				t.Errorf("TileAtPixel(%d,%d) = tile(%d,%d), want tile(%d,%d)",
					tt.px, tt.py, tile.X, tile.Y, tt.wantTX, tt.wantTY)
			}
			if !tt.wantOK && tile != nil {
				t.Errorf("TileAtPixel(%d,%d) = tile, want nil", tt.px, tt.py)
			}
		})
	}
}

func TestTileGrid_TilesInRect(t *testing.T) {
	grid := NewTileGrid(256, 256) // 4x4 tiles
	defer grid.Close()

	tests := []struct {
		name       string
		x, y, w, h int
		wantCount  int
	}{
		{"single tile", 0, 0, 32, 32, 1},
		{"full tile", 0, 0, 64, 64, 1},
		{"span two tiles x", 32, 0, 64, 32, 2},
		{"span two tiles y", 0, 32, 32, 64, 2},
		{"four tiles", 32, 32, 64, 64, 4},
		{"all tiles", 0, 0, 256, 256, 16},
		{"empty rect", 0, 0, 0, 0, 0},
		{"negative size", 0, 0, -10, 10, 0},
		{"outside canvas", 256, 0, 64, 64, 0},
		{"partial overlap", 200, 200, 100, 100, 1}, // Clamped to canvas bounds, only tile (3,3)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tiles := grid.TilesInRect(tt.x, tt.y, tt.w, tt.h)
			if len(tiles) != tt.wantCount {
				t.Errorf("TilesInRect(%d,%d,%d,%d) returned %d tiles, want %d",
					tt.x, tt.y, tt.w, tt.h, len(tiles), tt.wantCount)
			}
		})
	}
}

func TestTileGrid_MarkDirty(t *testing.T) {
	grid := NewTileGrid(128, 128)
	defer grid.Close()

	// Clear all dirty flags first
	grid.ClearDirty()

	// Mark one tile dirty
	grid.MarkDirty(1, 1)

	tile := grid.TileAt(1, 1)
	if !tile.Dirty {
		t.Error("MarkDirty did not set dirty flag")
	}

	// Other tiles should not be dirty
	if grid.TileAt(0, 0).Dirty {
		t.Error("MarkDirty affected wrong tile (0,0)")
	}

	// Mark out of bounds should not panic
	grid.MarkDirty(100, 100)
}

func TestTileGrid_MarkRectDirty(t *testing.T) {
	grid := NewTileGrid(256, 256) // 4x4 tiles
	defer grid.Close()

	// Clear all dirty flags
	grid.ClearDirty()

	// Mark a rectangle spanning 4 tiles
	grid.MarkRectDirty(32, 32, 64, 64)

	dirtyTiles := grid.DirtyTiles()
	if len(dirtyTiles) != 4 {
		t.Errorf("DirtyTiles() returned %d tiles, want 4", len(dirtyTiles))
	}
}

func TestTileGrid_DirtyTiles(t *testing.T) {
	grid := NewTileGrid(128, 128) // 2x2 tiles
	defer grid.Close()

	// All tiles start dirty
	dirty := grid.DirtyTiles()
	if len(dirty) != 4 {
		t.Errorf("Initial DirtyTiles() = %d, want 4", len(dirty))
	}

	// Clear all
	grid.ClearDirty()
	dirty = grid.DirtyTiles()
	if len(dirty) != 0 {
		t.Errorf("After ClearDirty() DirtyTiles() = %d, want 0", len(dirty))
	}

	// Mark some dirty
	grid.MarkDirty(0, 0)
	grid.MarkDirty(1, 1)
	dirty = grid.DirtyTiles()
	if len(dirty) != 2 {
		t.Errorf("After marking 2 dirty DirtyTiles() = %d, want 2", len(dirty))
	}
}

func TestTileGrid_Resize(t *testing.T) {
	grid := NewTileGrid(128, 128)

	// Verify initial state
	if grid.TileCount() != 4 {
		t.Errorf("Initial TileCount() = %d, want 4", grid.TileCount())
	}

	// Resize to larger
	grid.Resize(256, 256)
	if grid.TileCount() != 16 {
		t.Errorf("After resize to 256x256 TileCount() = %d, want 16", grid.TileCount())
	}
	if grid.Width() != 256 || grid.Height() != 256 {
		t.Errorf("Dimensions = %dx%d, want 256x256", grid.Width(), grid.Height())
	}

	// All tiles should be dirty after resize
	dirty := grid.DirtyTiles()
	if len(dirty) != 16 {
		t.Errorf("After resize DirtyTiles() = %d, want 16", len(dirty))
	}

	// Resize to smaller
	grid.Resize(64, 64)
	if grid.TileCount() != 1 {
		t.Errorf("After resize to 64x64 TileCount() = %d, want 1", grid.TileCount())
	}

	// Resize to same size should be no-op
	grid.ClearDirty()
	grid.Resize(64, 64)
	dirty = grid.DirtyTiles()
	if len(dirty) != 0 {
		t.Errorf("Same size resize made tiles dirty: %d", len(dirty))
	}

	// Resize to invalid
	grid.Resize(0, 0)
	if grid.TileCount() != 0 {
		t.Errorf("After resize to 0x0 TileCount() = %d, want 0", grid.TileCount())
	}

	grid.Close()
}

func TestTileGrid_ForEach(t *testing.T) {
	grid := NewTileGrid(128, 128) // 2x2 tiles
	defer grid.Close()

	count := 0
	grid.ForEach(func(tile *Tile) {
		count++
	})

	if count != 4 {
		t.Errorf("ForEach visited %d tiles, want 4", count)
	}
}

func TestTileGrid_ForEachDirty(t *testing.T) {
	grid := NewTileGrid(128, 128) // 2x2 tiles
	defer grid.Close()

	grid.ClearDirty()
	grid.MarkDirty(0, 0)
	grid.MarkDirty(1, 1)

	count := 0
	grid.ForEachDirty(func(tile *Tile) {
		count++
		if !tile.Dirty {
			t.Error("ForEachDirty called with non-dirty tile")
		}
	})

	if count != 2 {
		t.Errorf("ForEachDirty visited %d tiles, want 2", count)
	}
}

func TestTileGrid_MarkAllDirty(t *testing.T) {
	grid := NewTileGrid(128, 128) // 2x2 tiles
	defer grid.Close()

	grid.ClearDirty()
	if len(grid.DirtyTiles()) != 0 {
		t.Error("ClearDirty did not clear all tiles")
	}

	grid.MarkAllDirty()
	dirty := grid.DirtyTiles()
	if len(dirty) != 4 {
		t.Errorf("MarkAllDirty: DirtyTiles() = %d, want 4", len(dirty))
	}
}

// =============================================================================
// TilePool Tests
// =============================================================================

func TestTilePool_GetPut(t *testing.T) {
	pool := NewTilePool()

	// Get a full-size tile
	tile := pool.Get(TileWidth, TileHeight)
	if tile == nil {
		t.Fatal("Get returned nil")
	}
	if tile.Width != TileWidth || tile.Height != TileHeight {
		t.Errorf("Tile dimensions = %dx%d, want %dx%d", tile.Width, tile.Height, TileWidth, TileHeight)
	}
	if len(tile.Data) != TileBytes {
		t.Errorf("Tile data size = %d, want %d", len(tile.Data), TileBytes)
	}

	// Return to pool
	pool.Put(tile)

	// Get again - should reuse
	tile2 := pool.Get(TileWidth, TileHeight)
	if tile2 == nil {
		t.Fatal("Second Get returned nil")
	}
}

func TestTilePool_GetEdgeTile(t *testing.T) {
	pool := NewTilePool()

	// Get an edge tile (smaller dimensions)
	tile := pool.Get(32, 16)
	if tile == nil {
		t.Fatal("Get returned nil for edge tile")
	}
	if tile.Width != 32 || tile.Height != 16 {
		t.Errorf("Edge tile dimensions = %dx%d, want 32x16", tile.Width, tile.Height)
	}
	if len(tile.Data) != 32*16*4 {
		t.Errorf("Edge tile data size = %d, want %d", len(tile.Data), 32*16*4)
	}

	// Return to pool
	pool.Put(tile)

	// Get again with same dimensions
	tile2 := pool.Get(32, 16)
	if tile2 == nil {
		t.Fatal("Second Get returned nil for edge tile")
	}
	if tile2.Width != 32 || tile2.Height != 16 {
		t.Errorf("Reused edge tile dimensions = %dx%d, want 32x16", tile2.Width, tile2.Height)
	}
}

func TestTilePool_GetInvalid(t *testing.T) {
	pool := NewTilePool()

	tests := []struct {
		name          string
		width, height int
	}{
		{"zero width", 0, 64},
		{"zero height", 64, 0},
		{"negative width", -1, 64},
		{"negative height", 64, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tile := pool.Get(tt.width, tt.height)
			if tile != nil {
				t.Errorf("Get(%d,%d) returned tile, want nil", tt.width, tt.height)
			}
		})
	}
}

func TestTilePool_PutNil(t *testing.T) {
	pool := NewTilePool()
	// Should not panic
	pool.Put(nil)
}

func TestTilePool_Concurrent(t *testing.T) {
	pool := NewTilePool()
	numGoroutines := 20
	numOps := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOps; j++ {
				// Alternate between full tiles and edge tiles
				var tile *Tile
				if j%2 == 0 {
					tile = pool.Get(TileWidth, TileHeight)
				} else {
					w := 16 + (id % 48)
					h := 16 + (j % 48)
					tile = pool.Get(w, h)
				}

				if tile == nil {
					t.Errorf("goroutine %d: Get returned nil", id)
					continue
				}

				// Simulate some work
				if len(tile.Data) > 0 {
					tile.Data[0] = byte(id)
				}

				pool.Put(tile)
			}
		}(i)
	}

	wg.Wait()
}

func TestTilePool_DataCleared(t *testing.T) {
	pool := NewTilePool()

	// Get tile and fill with data
	tile := pool.Get(TileWidth, TileHeight)
	for i := range tile.Data {
		tile.Data[i] = 0xFF
	}

	// Return to pool
	pool.Put(tile)

	// Get again - data should be cleared
	tile2 := pool.Get(TileWidth, TileHeight)
	for i, b := range tile2.Data {
		if b != 0 {
			t.Errorf("Data[%d] = %d, want 0 (tile not cleared)", i, b)
			break
		}
	}
}

func TestDefaultPoolFunctions(t *testing.T) {
	// Test package-level convenience functions
	tile := GetTile(TileWidth, TileHeight)
	if tile == nil {
		t.Fatal("GetTile returned nil")
	}

	PutTile(tile)

	// Should not panic
	PutTile(nil)
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkTileGrid_Create(b *testing.B) {
	b.Run("128x128", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			grid := NewTileGrid(128, 128)
			grid.Close()
		}
	})

	b.Run("1920x1080", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			grid := NewTileGrid(1920, 1080)
			grid.Close()
		}
	})

	b.Run("4K", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			grid := NewTileGrid(3840, 2160)
			grid.Close()
		}
	})
}

func BenchmarkTileGrid_TileAtPixel(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = grid.TileAtPixel(960, 540)
	}
}

func BenchmarkTileGrid_TilesInRect(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.Run("small_rect", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = grid.TilesInRect(100, 100, 64, 64)
		}
	})

	b.Run("medium_rect", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = grid.TilesInRect(100, 100, 256, 256)
		}
	})

	b.Run("large_rect", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = grid.TilesInRect(0, 0, 960, 540)
		}
	})
}

func BenchmarkTileGrid_DirtyTiles(b *testing.B) {
	grid := NewTileGrid(1920, 1080)
	defer grid.Close()

	b.Run("all_dirty", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = grid.DirtyTiles()
		}
	})

	b.Run("none_dirty", func(b *testing.B) {
		grid.ClearDirty()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = grid.DirtyTiles()
		}
	})
}

func BenchmarkTilePool_GetPut(b *testing.B) {
	pool := NewTilePool()

	b.Run("full_tile", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			tile := pool.Get(TileWidth, TileHeight)
			pool.Put(tile)
		}
	})

	b.Run("edge_tile", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			tile := pool.Get(32, 16)
			pool.Put(tile)
		}
	})
}

func BenchmarkTilePool_Concurrent(b *testing.B) {
	pool := NewTilePool()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tile := pool.Get(TileWidth, TileHeight)
			pool.Put(tile)
		}
	})
}

func BenchmarkTile_PixelOffset(b *testing.B) {
	tile := &Tile{Width: TileWidth, Height: TileHeight, Data: make([]byte, TileBytes)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tile.PixelOffset(32, 32)
	}
}
