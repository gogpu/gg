package parallel

import (
	"image/color"
	"testing"
)

// =============================================================================
// ParallelRasterizer Creation Tests
// =============================================================================

func TestParallelRasterizer_Create(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		wantNil       bool
		wantTiles     int
	}{
		{
			name:      "valid small canvas",
			width:     64,
			height:    64,
			wantNil:   false,
			wantTiles: 1,
		},
		{
			name:      "valid medium canvas",
			width:     128,
			height:    128,
			wantNil:   false,
			wantTiles: 4,
		},
		{
			name:      "valid HD canvas",
			width:     1920,
			height:    1080,
			wantNil:   false,
			wantTiles: 510, // 30 * 17
		},
		{
			name:      "zero width",
			width:     0,
			height:    100,
			wantNil:   true,
			wantTiles: 0,
		},
		{
			name:      "zero height",
			width:     100,
			height:    0,
			wantNil:   true,
			wantTiles: 0,
		},
		{
			name:      "negative width",
			width:     -10,
			height:    100,
			wantNil:   true,
			wantTiles: 0,
		},
		{
			name:      "negative height",
			width:     100,
			height:    -10,
			wantNil:   true,
			wantTiles: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := NewParallelRasterizer(tt.width, tt.height)

			if tt.wantNil {
				if pr != nil {
					t.Errorf("NewParallelRasterizer(%d, %d) = non-nil, want nil",
						tt.width, tt.height)
				}
				return
			}

			if pr == nil {
				t.Fatalf("NewParallelRasterizer(%d, %d) = nil, want non-nil",
					tt.width, tt.height)
			}

			defer pr.Close()

			if pr.Width() != tt.width {
				t.Errorf("Width() = %d, want %d", pr.Width(), tt.width)
			}
			if pr.Height() != tt.height {
				t.Errorf("Height() = %d, want %d", pr.Height(), tt.height)
			}
			if pr.TileCount() != tt.wantTiles {
				t.Errorf("TileCount() = %d, want %d", pr.TileCount(), tt.wantTiles)
			}
		})
	}
}

func TestParallelRasterizer_CreateWithWorkers(t *testing.T) {
	pr := NewParallelRasterizerWithWorkers(128, 128, 4)
	if pr == nil {
		t.Fatal("NewParallelRasterizerWithWorkers returned nil")
	}
	defer pr.Close()

	if pr.Width() != 128 || pr.Height() != 128 {
		t.Errorf("Dimensions = %dx%d, want 128x128", pr.Width(), pr.Height())
	}
}

func TestParallelRasterizer_CreateWithWorkers_Invalid(t *testing.T) {
	pr := NewParallelRasterizerWithWorkers(0, 100, 4)
	if pr != nil {
		t.Error("NewParallelRasterizerWithWorkers with invalid dimensions should return nil")
		pr.Close()
	}
}

// =============================================================================
// Clear Tests
// =============================================================================

func TestParallelRasterizer_Clear(t *testing.T) {
	pr := NewParallelRasterizer(128, 128)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	// Clear with red color
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	pr.Clear(red)

	// Verify all tiles are filled with red
	tiles := pr.Grid().AllTiles()
	for _, tile := range tiles {
		for i := 0; i < len(tile.Data); i += 4 {
			if tile.Data[i] != 255 || tile.Data[i+1] != 0 ||
				tile.Data[i+2] != 0 || tile.Data[i+3] != 255 {
				t.Errorf("Tile data mismatch at offset %d: got [%d,%d,%d,%d], want [255,0,0,255]",
					i, tile.Data[i], tile.Data[i+1], tile.Data[i+2], tile.Data[i+3])
				break
			}
		}
		if !tile.Dirty {
			t.Error("Tile should be marked dirty after Clear")
		}
	}
}

func TestParallelRasterizer_Clear_Different_Colors(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	tests := []struct {
		name  string
		color color.Color
		want  [4]byte
	}{
		{"white", color.White, [4]byte{255, 255, 255, 255}},
		{"black", color.Black, [4]byte{0, 0, 0, 255}},
		{"transparent", color.Transparent, [4]byte{0, 0, 0, 0}},
		{"green", color.RGBA{R: 0, G: 128, B: 0, A: 255}, [4]byte{0, 128, 0, 255}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr.Clear(tt.color)

			tile := pr.Grid().TileAt(0, 0)
			if tile.Data[0] != tt.want[0] || tile.Data[1] != tt.want[1] ||
				tile.Data[2] != tt.want[2] || tile.Data[3] != tt.want[3] {
				t.Errorf("First pixel = [%d,%d,%d,%d], want [%d,%d,%d,%d]",
					tile.Data[0], tile.Data[1], tile.Data[2], tile.Data[3],
					tt.want[0], tt.want[1], tt.want[2], tt.want[3])
			}
		})
	}
}

// =============================================================================
// FillRect Tests
// =============================================================================

func TestParallelRasterizer_FillRect(t *testing.T) {
	pr := NewParallelRasterizer(128, 128)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	// Clear to black first
	pr.Clear(color.Black)

	// Fill a red rectangle
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	pr.FillRect(10, 10, 50, 50, red)

	// Verify pixels inside the rectangle are red
	tile := pr.Grid().TileAt(0, 0)
	offset := tile.PixelOffset(15, 15)
	if offset < 0 {
		t.Fatal("Invalid pixel offset")
	}

	if tile.Data[offset] != 255 || tile.Data[offset+1] != 0 ||
		tile.Data[offset+2] != 0 || tile.Data[offset+3] != 255 {
		t.Errorf("Pixel inside rect = [%d,%d,%d,%d], want [255,0,0,255]",
			tile.Data[offset], tile.Data[offset+1], tile.Data[offset+2], tile.Data[offset+3])
	}

	// Verify pixel outside is still black
	offset = tile.PixelOffset(5, 5)
	if tile.Data[offset] != 0 || tile.Data[offset+3] != 255 {
		t.Errorf("Pixel outside rect should be black")
	}
}

func TestParallelRasterizer_FillRect_SingleTile(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	pr.Clear(color.Black)

	blue := color.RGBA{R: 0, G: 0, B: 255, A: 255}
	pr.FillRect(10, 10, 20, 20, blue)

	tile := pr.Grid().TileAt(0, 0)

	// Check pixel at (15, 15) - inside the rect
	offset := tile.PixelOffset(15, 15)
	if tile.Data[offset+2] != 255 {
		t.Errorf("Expected blue pixel inside rect, got B=%d", tile.Data[offset+2])
	}

	// Check pixel at (5, 5) - outside the rect
	offset = tile.PixelOffset(5, 5)
	if tile.Data[offset+2] != 0 {
		t.Errorf("Expected black pixel outside rect, got B=%d", tile.Data[offset+2])
	}
}

func TestParallelRasterizer_FillRect_MultipleTiles(t *testing.T) {
	pr := NewParallelRasterizer(128, 128) // 2x2 tiles
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	pr.Clear(color.Black)

	// Fill rect spanning all 4 tiles
	green := color.RGBA{R: 0, G: 255, B: 0, A: 255}
	pr.FillRect(32, 32, 64, 64, green)

	// Verify pixels in each tile
	testCases := []struct {
		tileX, tileY   int
		pixelX, pixelY int
		wantGreen      bool
	}{
		{0, 0, 40, 40, true},  // Inside rect in tile (0,0)
		{1, 0, 0, 40, true},   // Inside rect in tile (1,0)
		{0, 1, 40, 0, true},   // Inside rect in tile (0,1)
		{1, 1, 0, 0, true},    // Inside rect in tile (1,1)
		{0, 0, 10, 10, false}, // Outside rect
	}

	for _, tc := range testCases {
		tile := pr.Grid().TileAt(tc.tileX, tc.tileY)
		if tile == nil {
			t.Errorf("Tile (%d,%d) is nil", tc.tileX, tc.tileY)
			continue
		}

		offset := tile.PixelOffset(tc.pixelX, tc.pixelY)
		if offset < 0 {
			continue
		}

		isGreen := tile.Data[offset+1] == 255
		if isGreen != tc.wantGreen {
			t.Errorf("Tile(%d,%d) pixel(%d,%d) green=%v, want green=%v",
				tc.tileX, tc.tileY, tc.pixelX, tc.pixelY, isGreen, tc.wantGreen)
		}
	}
}

func TestParallelRasterizer_FillRect_EdgeTiles(t *testing.T) {
	pr := NewParallelRasterizer(100, 100) // Non-multiple of 64
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	pr.Clear(color.Black)

	// Fill rect that includes edge tiles
	yellow := color.RGBA{R: 255, G: 255, B: 0, A: 255}
	pr.FillRect(60, 60, 35, 35, yellow)

	// Verify edge tile (1,1) which is 36x36 pixels
	tile := pr.Grid().TileAt(1, 1)
	if tile == nil {
		t.Fatal("Edge tile (1,1) is nil")
	}

	if tile.Width != 36 || tile.Height != 36 {
		t.Errorf("Edge tile dimensions = %dx%d, want 36x36", tile.Width, tile.Height)
	}

	// Check pixel inside rect on edge tile
	offset := tile.PixelOffset(5, 5)
	if offset >= 0 {
		if tile.Data[offset] != 255 || tile.Data[offset+1] != 255 {
			t.Errorf("Edge tile pixel should be yellow")
		}
	}
}

func TestParallelRasterizer_FillRect_OutOfBounds(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	pr.Clear(color.Black)
	pr.ClearDirty()

	// Fill rect completely outside canvas - should be no-op
	pr.FillRect(100, 100, 50, 50, color.White)

	// Verify nothing changed (no dirty tiles)
	if pr.DirtyTileCount() != 0 {
		t.Error("FillRect outside canvas should not dirty any tiles")
	}
}

func TestParallelRasterizer_FillRect_ZeroSize(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	pr.Clear(color.Black)
	pr.ClearDirty()

	// Zero width
	pr.FillRect(10, 10, 0, 50, color.White)
	if pr.DirtyTileCount() != 0 {
		t.Error("FillRect with zero width should not dirty tiles")
	}

	// Zero height
	pr.FillRect(10, 10, 50, 0, color.White)
	if pr.DirtyTileCount() != 0 {
		t.Error("FillRect with zero height should not dirty tiles")
	}
}

func TestParallelRasterizer_FillRect_Negative(t *testing.T) {
	pr := NewParallelRasterizer(128, 128)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	pr.Clear(color.Black)

	// Negative coordinates that still overlap canvas
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	pr.FillRect(-10, -10, 30, 30, red)

	// Should fill visible portion (0,0) to (20,20)
	tile := pr.Grid().TileAt(0, 0)
	offset := tile.PixelOffset(10, 10)
	if tile.Data[offset] != 255 {
		t.Error("Negative offset rect should still fill visible portion")
	}

	// Pixel outside the clamped rect
	offset = tile.PixelOffset(25, 25)
	if tile.Data[offset] != 0 {
		t.Error("Pixel outside clamped rect should be black")
	}
}

func TestParallelRasterizer_FillRect_NegativeSize(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	pr.Clear(color.Black)
	pr.ClearDirty()

	// Negative width
	pr.FillRect(10, 10, -10, 20, color.White)
	if pr.DirtyTileCount() != 0 {
		t.Error("FillRect with negative width should not dirty tiles")
	}

	// Negative height
	pr.FillRect(10, 10, 20, -10, color.White)
	if pr.DirtyTileCount() != 0 {
		t.Error("FillRect with negative height should not dirty tiles")
	}
}

func TestParallelRasterizer_FillRect_PartialOverlap(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	pr.Clear(color.Black)

	// Rect extends beyond canvas
	cyan := color.RGBA{R: 0, G: 255, B: 255, A: 255}
	pr.FillRect(50, 50, 100, 100, cyan)

	// Should fill from (50,50) to (64,64) - clipped to canvas
	tile := pr.Grid().TileAt(0, 0)

	// Inside the clipped rect
	offset := tile.PixelOffset(55, 55)
	if tile.Data[offset+1] != 255 || tile.Data[offset+2] != 255 {
		t.Error("Expected cyan pixel in overlapping region")
	}

	// Outside the rect
	offset = tile.PixelOffset(40, 40)
	if tile.Data[offset+1] != 0 || tile.Data[offset+2] != 0 {
		t.Error("Expected black pixel outside rect")
	}
}

// =============================================================================
// FillTiles Tests
// =============================================================================

func TestParallelRasterizer_FillTiles(t *testing.T) {
	pr := NewParallelRasterizer(128, 128)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	pr.Clear(color.Black)

	tiles := pr.Grid().AllTiles()
	callCount := 0

	pr.FillTiles(tiles, func(tile *Tile) {
		callCount++
		// Fill first pixel with red
		tile.Data[0] = 255
		tile.Data[1] = 0
		tile.Data[2] = 0
		tile.Data[3] = 255
	})

	// Note: We can't check callCount directly due to parallel execution,
	// but we can verify all tiles were modified
	for _, tile := range tiles {
		if tile.Data[0] != 255 {
			t.Error("FillTiles did not modify all tiles")
			break
		}
	}
}

func TestParallelRasterizer_FillTiles_Empty(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	// Should not panic with empty tiles
	pr.FillTiles(nil, func(tile *Tile) {})
	pr.FillTiles([]*Tile{}, func(tile *Tile) {})
}

func TestParallelRasterizer_FillTiles_NilFunc(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	tiles := pr.Grid().AllTiles()

	// Should not panic with nil function
	pr.FillTiles(tiles, nil)
}

// =============================================================================
// Composite Tests
// =============================================================================

func TestParallelRasterizer_Composite(t *testing.T) {
	pr := NewParallelRasterizer(128, 128)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	// Fill with a test pattern
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	pr.Clear(red)

	// Composite to buffer
	stride := 128 * 4
	dst := make([]byte, 128*128*4)
	pr.Composite(dst, stride)

	// Verify output
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			offset := y*stride + x*4
			if dst[offset] != 255 || dst[offset+1] != 0 ||
				dst[offset+2] != 0 || dst[offset+3] != 255 {
				t.Errorf("Composite pixel (%d,%d) = [%d,%d,%d,%d], want red",
					x, y, dst[offset], dst[offset+1], dst[offset+2], dst[offset+3])
				return
			}
		}
	}
}

func TestParallelRasterizer_Composite_EdgeTiles(t *testing.T) {
	pr := NewParallelRasterizer(100, 100) // Non-multiple of 64
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	blue := color.RGBA{R: 0, G: 0, B: 255, A: 255}
	pr.Clear(blue)

	stride := 100 * 4
	dst := make([]byte, 100*100*4)
	pr.Composite(dst, stride)

	// Check corner pixels
	corners := []struct {
		x, y int
	}{
		{0, 0},
		{99, 0},
		{0, 99},
		{99, 99},
	}

	for _, c := range corners {
		offset := c.y*stride + c.x*4
		if dst[offset+2] != 255 {
			t.Errorf("Corner pixel (%d,%d) B=%d, want 255", c.x, c.y, dst[offset+2])
		}
	}
}

func TestParallelRasterizer_Composite_SmallBuffer(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	pr.Clear(color.White)

	// Buffer too small - should be no-op
	smallDst := make([]byte, 10)
	pr.Composite(smallDst, 64*4)

	// Verify buffer unchanged
	for _, b := range smallDst {
		if b != 0 {
			t.Error("Small buffer should not be modified")
			break
		}
	}
}

func TestParallelRasterizer_CompositeDirty(t *testing.T) {
	pr := NewParallelRasterizer(128, 128)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	// Fill with black
	pr.Clear(color.Black)
	pr.ClearDirty()

	// Fill one tile with white
	tile := pr.Grid().TileAt(0, 0)
	for i := 0; i < len(tile.Data); i += 4 {
		tile.Data[i] = 255
		tile.Data[i+1] = 255
		tile.Data[i+2] = 255
		tile.Data[i+3] = 255
	}
	tile.Dirty = true

	stride := 128 * 4
	dst := make([]byte, 128*128*4)

	// Only dirty tiles should be composited
	pr.CompositeDirty(dst, stride)

	// First tile (0,0) should be white
	if dst[0] != 255 {
		t.Error("Dirty tile (0,0) should be composited")
	}

	// Other tiles should be zero (not composited)
	offset := 64 * 4 // First pixel of tile (1,0)
	if dst[offset] != 0 {
		t.Error("Non-dirty tile should not be composited")
	}
}

// =============================================================================
// Resize Tests
// =============================================================================

func TestParallelRasterizer_Resize(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	// Initial state
	if pr.TileCount() != 1 {
		t.Errorf("Initial TileCount() = %d, want 1", pr.TileCount())
	}

	// Resize to larger
	pr.Resize(128, 128)

	if pr.Width() != 128 || pr.Height() != 128 {
		t.Errorf("After resize: dimensions = %dx%d, want 128x128", pr.Width(), pr.Height())
	}
	if pr.TileCount() != 4 {
		t.Errorf("After resize: TileCount() = %d, want 4", pr.TileCount())
	}

	// Resize to same - should be no-op
	pr.ClearDirty()
	pr.Resize(128, 128)
	if pr.DirtyTileCount() != 0 {
		t.Error("Resize to same dimensions should not dirty tiles")
	}

	// Resize to smaller
	pr.Resize(50, 50)
	if pr.TileCount() != 1 {
		t.Errorf("After shrink: TileCount() = %d, want 1", pr.TileCount())
	}
}

func TestParallelRasterizer_Resize_Invalid(t *testing.T) {
	pr := NewParallelRasterizer(64, 64)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	// Invalid resize should be no-op
	pr.Resize(0, 100)
	if pr.Width() != 64 || pr.Height() != 64 {
		t.Error("Invalid resize should not change dimensions")
	}

	pr.Resize(-10, 100)
	if pr.Width() != 64 {
		t.Error("Negative resize should not change dimensions")
	}
}

// =============================================================================
// Dirty Tracking Tests
// =============================================================================

func TestParallelRasterizer_DirtyTracking(t *testing.T) {
	pr := NewParallelRasterizer(128, 128)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	// All tiles start dirty after creation
	if pr.DirtyTileCount() != 4 {
		t.Errorf("Initial dirty count = %d, want 4", pr.DirtyTileCount())
	}

	// Clear dirty
	pr.ClearDirty()
	if pr.DirtyTileCount() != 0 {
		t.Error("ClearDirty should reset all dirty flags")
	}

	// Mark rect dirty
	pr.MarkRectDirty(32, 32, 64, 64)
	if pr.DirtyTileCount() != 4 {
		t.Errorf("MarkRectDirty should dirty 4 tiles, got %d", pr.DirtyTileCount())
	}

	// Clear and mark all dirty
	pr.ClearDirty()
	pr.MarkAllDirty()
	if pr.DirtyTileCount() != 4 {
		t.Errorf("MarkAllDirty should dirty all tiles, got %d", pr.DirtyTileCount())
	}
}

// =============================================================================
// Grid Access Tests
// =============================================================================

func TestParallelRasterizer_Grid(t *testing.T) {
	pr := NewParallelRasterizer(128, 128)
	if pr == nil {
		t.Fatal("Failed to create rasterizer")
	}
	defer pr.Close()

	grid := pr.Grid()
	if grid == nil {
		t.Fatal("Grid() returned nil")
	}

	if grid.TileCount() != pr.TileCount() {
		t.Error("Grid TileCount mismatch")
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkParallelRasterizer_Clear(b *testing.B) {
	sizes := []struct {
		name          string
		width, height int
	}{
		{"64x64", 64, 64},
		{"256x256", 256, 256},
		{"1920x1080", 1920, 1080},
		{"4K", 3840, 2160},
	}

	white := color.White

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			pr := NewParallelRasterizer(size.width, size.height)
			defer pr.Close()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				pr.Clear(white)
			}
		})
	}
}

func BenchmarkParallelRasterizer_FillRect_Small(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pr.FillRect(100, 100, 50, 50, red)
	}
}

func BenchmarkParallelRasterizer_FillRect_Large(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pr.FillRect(0, 0, 960, 540, red)
	}
}

func BenchmarkParallelRasterizer_FillRect_FullScreen(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pr.FillRect(0, 0, 1920, 1080, red)
	}
}

func BenchmarkParallelRasterizer_Composite(b *testing.B) {
	sizes := []struct {
		name          string
		width, height int
	}{
		{"256x256", 256, 256},
		{"1920x1080", 1920, 1080},
		{"4K", 3840, 2160},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			pr := NewParallelRasterizer(size.width, size.height)
			defer pr.Close()

			pr.Clear(color.White)

			stride := size.width * 4
			dst := make([]byte, size.height*stride)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				pr.Composite(dst, stride)
			}
		})
	}
}

func BenchmarkParallelRasterizer_CompositeDirty(b *testing.B) {
	pr := NewParallelRasterizer(1920, 1080)
	defer pr.Close()

	pr.Clear(color.White)
	pr.ClearDirty()

	// Mark only 10% of tiles dirty
	tiles := pr.Grid().AllTiles()
	for i := 0; i < len(tiles)/10; i++ {
		tiles[i].Dirty = true
	}

	stride := 1920 * 4
	dst := make([]byte, 1080*stride)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pr.CompositeDirty(dst, stride)
	}
}

func BenchmarkParallelRasterizer_Create(b *testing.B) {
	b.Run("HD", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			pr := NewParallelRasterizer(1920, 1080)
			pr.Close()
		}
	})

	b.Run("4K", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			pr := NewParallelRasterizer(3840, 2160)
			pr.Close()
		}
	})
}

// BenchmarkParallelVsSequential compares parallel vs sequential clear.
func BenchmarkParallelVsSequential(b *testing.B) {
	width, height := 1920, 1080
	white := color.White

	b.Run("Parallel", func(b *testing.B) {
		pr := NewParallelRasterizer(width, height)
		defer pr.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pr.Clear(white)
		}
	})

	b.Run("Sequential", func(b *testing.B) {
		grid := NewTileGrid(width, height)
		defer grid.Close()

		rgba := colorToRGBA(white)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, tile := range grid.AllTiles() {
				// Sequential fill
				data := tile.Data
				stride := tile.Width * 4
				for x := 0; x < tile.Width; x++ {
					offset := x * 4
					data[offset] = rgba[0]
					data[offset+1] = rgba[1]
					data[offset+2] = rgba[2]
					data[offset+3] = rgba[3]
				}
				firstRow := data[:stride]
				for y := 1; y < tile.Height; y++ {
					rowStart := y * stride
					copy(data[rowStart:rowStart+stride], firstRow)
				}
			}
		}
	})
}
