package parallel

import (
	"image/color"
	"runtime"
)

// ParallelRasterizer renders to tiles in parallel using WorkerPool.
//
// It divides the canvas into 64x64 pixel tiles that can be rendered
// independently and in parallel. This is the primary entry point for
// parallel rendering operations.
//
// Thread safety: ParallelRasterizer methods are safe for concurrent use
// after initialization. The internal WorkerPool handles synchronization.
type ParallelRasterizer struct {
	grid   *TileGrid
	pool   *WorkerPool
	width  int
	height int
}

// NewParallelRasterizer creates a new parallel rasterizer for the given dimensions.
// It initializes a TileGrid and WorkerPool with GOMAXPROCS workers.
// Returns nil if width or height is <= 0.
func NewParallelRasterizer(width, height int) *ParallelRasterizer {
	if width <= 0 || height <= 0 {
		return nil
	}

	return &ParallelRasterizer{
		grid:   NewTileGrid(width, height),
		pool:   NewWorkerPool(runtime.GOMAXPROCS(0)),
		width:  width,
		height: height,
	}
}

// NewParallelRasterizerWithWorkers creates a parallel rasterizer with a specific worker count.
// If workers <= 0, GOMAXPROCS is used.
// Returns nil if width or height is <= 0.
func NewParallelRasterizerWithWorkers(width, height, workers int) *ParallelRasterizer {
	if width <= 0 || height <= 0 {
		return nil
	}

	return &ParallelRasterizer{
		grid:   NewTileGrid(width, height),
		pool:   NewWorkerPool(workers),
		width:  width,
		height: height,
	}
}

// Width returns the canvas width in pixels.
func (pr *ParallelRasterizer) Width() int {
	return pr.width
}

// Height returns the canvas height in pixels.
func (pr *ParallelRasterizer) Height() int {
	return pr.height
}

// TileCount returns the total number of tiles.
func (pr *ParallelRasterizer) TileCount() int {
	return pr.grid.TileCount()
}

// Grid returns the underlying TileGrid for advanced usage.
// Use with caution as direct grid manipulation bypasses parallel safety.
func (pr *ParallelRasterizer) Grid() *TileGrid {
	return pr.grid
}

// Resize changes the canvas dimensions, reallocating tiles as needed.
// All tiles will be marked dirty after resize.
// If dimensions are unchanged, this is a no-op.
func (pr *ParallelRasterizer) Resize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}

	if pr.width == width && pr.height == height {
		return
	}

	pr.grid.Resize(width, height)
	pr.width = width
	pr.height = height
}

// Clear fills all tiles with the specified color in parallel.
func (pr *ParallelRasterizer) Clear(c color.Color) {
	tiles := pr.grid.AllTiles()
	if len(tiles) == 0 {
		return
	}

	// Convert color to RGBA bytes once
	rgba := colorToRGBA(c)

	// Create work for each tile
	work := make([]func(), len(tiles))
	for i, tile := range tiles {
		t := tile
		work[i] = func() {
			pr.clearTile(t, rgba)
		}
	}

	pr.pool.ExecuteAll(work)
}

// clearTile fills a single tile with a solid color.
func (pr *ParallelRasterizer) clearTile(t *Tile, rgba [4]byte) {
	data := t.Data
	stride := t.Width * 4

	// Fill first row
	for x := 0; x < t.Width; x++ {
		offset := x * 4
		data[offset] = rgba[0]
		data[offset+1] = rgba[1]
		data[offset+2] = rgba[2]
		data[offset+3] = rgba[3]
	}

	// Copy first row to all other rows
	firstRow := data[:stride]
	for y := 1; y < t.Height; y++ {
		rowStart := y * stride
		copy(data[rowStart:rowStart+stride], firstRow)
	}

	t.Dirty = true
}

// FillRect fills a rectangle with the specified color across affected tiles in parallel.
// Coordinates are in canvas pixel space. The rectangle is clipped to canvas bounds.
func (pr *ParallelRasterizer) FillRect(x, y, w, h int, c color.Color) {
	// Early exit for empty or invalid rectangles
	if w <= 0 || h <= 0 {
		return
	}

	// Clamp to canvas bounds
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > pr.width {
		w = pr.width - x
	}
	if y+h > pr.height {
		h = pr.height - y
	}

	// Check if rectangle is completely outside canvas
	if w <= 0 || h <= 0 {
		return
	}

	// Get affected tiles
	tiles := pr.grid.TilesInRect(x, y, w, h)
	if len(tiles) == 0 {
		return
	}

	// Convert color to RGBA bytes once
	rgba := colorToRGBA(c)

	// Create work for each tile
	work := make([]func(), len(tiles))
	for i, tile := range tiles {
		t := tile
		work[i] = func() {
			pr.fillRectInTile(t, x, y, w, h, rgba)
		}
	}

	pr.pool.ExecuteAll(work)
}

// fillRectInTile fills the intersection of rect and tile.
func (pr *ParallelRasterizer) fillRectInTile(t *Tile, rectX, rectY, rectW, rectH int, rgba [4]byte) {
	// Calculate tile bounds in canvas space
	tileX, tileY, tileW, tileH := t.Bounds()

	// Calculate intersection with tile bounds
	x1 := max(rectX, tileX)
	y1 := max(rectY, tileY)
	x2 := min(rectX+rectW, tileX+tileW)
	y2 := min(rectY+rectH, tileY+tileH)

	if x1 >= x2 || y1 >= y2 {
		return
	}

	// Convert to tile-local coordinates
	localX := x1 - tileX
	localY := y1 - tileY
	localW := x2 - x1
	localH := y2 - y1

	// Fill in tile's data buffer
	for row := localY; row < localY+localH; row++ {
		rowStart := row * t.Width * 4
		for col := localX; col < localX+localW; col++ {
			offset := rowStart + col*4
			t.Data[offset] = rgba[0]
			t.Data[offset+1] = rgba[1]
			t.Data[offset+2] = rgba[2]
			t.Data[offset+3] = rgba[3]
		}
	}

	t.Dirty = true
}

// FillTiles executes a function on each tile in parallel.
// The function receives each tile and should only modify that tile's Data.
// This is the generic parallel fill mechanism for custom operations.
func (pr *ParallelRasterizer) FillTiles(tiles []*Tile, fn func(t *Tile)) {
	if len(tiles) == 0 || fn == nil {
		return
	}

	work := make([]func(), len(tiles))
	for i, tile := range tiles {
		t := tile
		work[i] = func() {
			fn(t)
		}
	}

	pr.pool.ExecuteAll(work)
}

// Composite copies all tile data to a destination buffer in row-major RGBA order.
// The dst buffer must be at least width * height * 4 bytes.
// The stride is the number of bytes per row in dst (typically width * 4).
// This operation is performed in parallel across tiles.
func (pr *ParallelRasterizer) Composite(dst []byte, stride int) {
	if len(dst) < pr.height*stride {
		return
	}

	tiles := pr.grid.AllTiles()
	if len(tiles) == 0 {
		return
	}

	work := make([]func(), len(tiles))
	for i, tile := range tiles {
		t := tile
		work[i] = func() {
			pr.compositeTile(t, dst, stride)
		}
	}

	pr.pool.ExecuteAll(work)
}

// compositeTile copies a single tile's data to the destination buffer.
func (pr *ParallelRasterizer) compositeTile(t *Tile, dst []byte, dstStride int) {
	// Calculate tile position in canvas space
	tileX, tileY, _, _ := t.Bounds()

	srcStride := t.Width * 4

	for row := 0; row < t.Height; row++ {
		canvasY := tileY + row

		// Skip if outside canvas bounds
		if canvasY >= pr.height {
			break
		}

		dstOffset := canvasY*dstStride + tileX*4
		srcOffset := row * srcStride

		// Calculate how many bytes to copy (handle edge tiles)
		copyLen := srcStride
		if tileX*4+copyLen > dstStride {
			copyLen = dstStride - tileX*4
		}
		if copyLen <= 0 {
			continue
		}

		copy(dst[dstOffset:dstOffset+copyLen], t.Data[srcOffset:srcOffset+copyLen])
	}
}

// CompositeDirty copies only dirty tiles to a destination buffer.
// This is useful for incremental rendering where only changed regions need updating.
func (pr *ParallelRasterizer) CompositeDirty(dst []byte, stride int) {
	if len(dst) < pr.height*stride {
		return
	}

	tiles := pr.grid.DirtyTiles()
	if len(tiles) == 0 {
		return
	}

	work := make([]func(), len(tiles))
	for i, tile := range tiles {
		t := tile
		work[i] = func() {
			pr.compositeTile(t, dst, stride)
		}
	}

	pr.pool.ExecuteAll(work)
}

// MarkAllDirty marks all tiles for redraw.
func (pr *ParallelRasterizer) MarkAllDirty() {
	pr.grid.MarkAllDirty()
}

// MarkRectDirty marks all tiles intersecting the rectangle as dirty.
func (pr *ParallelRasterizer) MarkRectDirty(x, y, w, h int) {
	pr.grid.MarkRectDirty(x, y, w, h)
}

// ClearDirty resets the dirty flag on all tiles.
func (pr *ParallelRasterizer) ClearDirty() {
	pr.grid.ClearDirty()
}

// DirtyTileCount returns the number of tiles marked as dirty.
func (pr *ParallelRasterizer) DirtyTileCount() int {
	return len(pr.grid.DirtyTiles())
}

// Close releases all resources.
// The rasterizer should not be used after Close is called.
func (pr *ParallelRasterizer) Close() {
	pr.pool.Close()
	pr.grid.Close()
}

// colorToRGBA converts a color.Color to RGBA bytes.
func colorToRGBA(c color.Color) [4]byte {
	r, g, b, a := c.RGBA()
	return [4]byte{
		byte(r >> 8),
		byte(g >> 8),
		byte(b >> 8),
		byte(a >> 8),
	}
}
