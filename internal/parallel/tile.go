// Package parallel provides tile-based parallel rendering infrastructure for gogpu/gg.
//
// This package implements a tile-based rendering system where the canvas is divided
// into 64x64 pixel tiles that can be rendered independently in parallel. Key features:
//
//   - 64x64 tiles optimized for L1 cache (16KB per tile in RGBA)
//   - Tile pooling for memory reuse via sync.Pool
//   - Dirty region tracking for incremental rendering
//   - Lock-free operations where possible
//
// Thread safety: TileGrid operations are NOT thread-safe by default.
// Use external synchronization or the provided WorkerPool for parallel access.
package parallel

// Tile size constants optimized for cache efficiency and work distribution.
const (
	// TileWidth is the width of a tile in pixels.
	// 64 pixels is optimal for work distribution (matches vello/tiny-skia).
	TileWidth = 64

	// TileHeight is the height of a tile in pixels.
	// 64 pixels ensures 16KB per tile (fits L1 cache).
	TileHeight = 64

	// TilePixels is the total number of pixels in a full tile.
	TilePixels = TileWidth * TileHeight

	// TileBytes is the size of a full tile in bytes (RGBA = 4 bytes per pixel).
	TileBytes = TilePixels * 4
)

// Tile represents a rectangular region for parallel processing.
//
// Each tile contains its own pixel buffer that can be rendered independently.
// Edge tiles may have smaller actual dimensions when the canvas is not
// evenly divisible by the tile size.
type Tile struct {
	// X is the tile column index (0-based).
	X int

	// Y is the tile row index (0-based).
	Y int

	// Width is the actual width in pixels (may be < TileWidth for edge tiles).
	Width int

	// Height is the actual height in pixels (may be < TileHeight for edge tiles).
	Height int

	// Dirty indicates whether this tile needs redrawing.
	Dirty bool

	// Data contains the RGBA pixel data owned by this tile.
	// Length is Width * Height * 4 bytes.
	Data []byte
}

// Reset clears the tile data for reuse.
// This zeros all pixel data and resets the dirty flag.
func (t *Tile) Reset() {
	clear(t.Data)
	t.Dirty = false
}

// Bounds returns the pixel bounds of this tile in canvas space.
// Returns (x, y, width, height) where x,y is the top-left corner.
func (t *Tile) Bounds() (x, y, w, h int) {
	return t.X * TileWidth, t.Y * TileHeight, t.Width, t.Height
}

// PixelOffset returns the byte offset into Data for the given pixel.
// Coordinates px, py are relative to the tile (0,0 is top-left of tile).
// Returns -1 if coordinates are out of bounds.
func (t *Tile) PixelOffset(px, py int) int {
	if px < 0 || px >= t.Width || py < 0 || py >= t.Height {
		return -1
	}
	return (py*t.Width + px) * 4
}

// CanvasPixelOffset returns the byte offset for a canvas-space pixel.
// Coordinates cx, cy are in canvas space.
// Returns -1 if the pixel is not within this tile.
func (t *Tile) CanvasPixelOffset(cx, cy int) int {
	// Convert canvas coordinates to tile-local coordinates
	tileX := t.X * TileWidth
	tileY := t.Y * TileHeight

	px := cx - tileX
	py := cy - tileY

	return t.PixelOffset(px, py)
}

// Contains returns true if the canvas-space pixel (cx, cy) is within this tile.
func (t *Tile) Contains(cx, cy int) bool {
	tileX := t.X * TileWidth
	tileY := t.Y * TileHeight

	return cx >= tileX && cx < tileX+t.Width &&
		cy >= tileY && cy < tileY+t.Height
}

// Stride returns the row stride in bytes.
func (t *Tile) Stride() int {
	return t.Width * 4
}

// ByteSize returns the total size of the tile data in bytes.
func (t *Tile) ByteSize() int {
	return t.Width * t.Height * 4
}
