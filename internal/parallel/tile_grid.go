package parallel

// TileGrid manages a grid of tiles for parallel rendering.
//
// The grid divides a canvas into 64x64 pixel tiles. Edge tiles may have
// smaller dimensions when the canvas is not evenly divisible by the tile size.
// Tiles are stored in a flat slice for cache efficiency, accessed via
// index calculation: index = ty * tilesX + tx.
//
// Thread safety: TileGrid is NOT thread-safe. Use external synchronization
// for concurrent access, or use the provided WorkerPool.
type TileGrid struct {
	// tiles is a flat slice of all tiles (row-major order).
	tiles []*Tile

	// tilesX is the number of tiles horizontally.
	tilesX int

	// tilesY is the number of tiles vertically.
	tilesY int

	// width is the canvas width in pixels.
	width int

	// height is the canvas height in pixels.
	height int

	// pool is used for tile memory allocation.
	pool *TilePool
}

// NewTileGrid creates a new tile grid for the given canvas dimensions.
// The grid will contain enough tiles to cover the entire canvas.
// Edge tiles will have reduced dimensions if the canvas is not evenly
// divisible by the tile size (64x64).
func NewTileGrid(width, height int) *TileGrid {
	if width <= 0 || height <= 0 {
		return &TileGrid{
			tiles:  nil,
			tilesX: 0,
			tilesY: 0,
			width:  0,
			height: 0,
			pool:   NewTilePool(),
		}
	}

	tilesX := (width + TileWidth - 1) / TileWidth
	tilesY := (height + TileHeight - 1) / TileHeight

	g := &TileGrid{
		tiles:  make([]*Tile, tilesX*tilesY),
		tilesX: tilesX,
		tilesY: tilesY,
		width:  width,
		height: height,
		pool:   NewTilePool(),
	}

	g.allocateTiles()
	return g
}

// allocateTiles creates all tiles for the grid.
func (g *TileGrid) allocateTiles() {
	for ty := range g.tilesY {
		for tx := range g.tilesX {
			// Calculate actual tile dimensions (edge tiles may be smaller)
			tileW := TileWidth
			tileH := TileHeight

			// Right edge tile
			if (tx+1)*TileWidth > g.width {
				tileW = g.width - tx*TileWidth
			}
			// Bottom edge tile
			if (ty+1)*TileHeight > g.height {
				tileH = g.height - ty*TileHeight
			}

			tile := g.pool.Get(tileW, tileH)
			tile.X = tx
			tile.Y = ty
			tile.Dirty = true // New tiles start dirty

			g.tiles[ty*g.tilesX+tx] = tile
		}
	}
}

// Resize changes the grid dimensions, reallocating tiles as needed.
// If dimensions haven't changed, this is a no-op.
// All tiles will be marked dirty after resize.
func (g *TileGrid) Resize(width, height int) {
	if width <= 0 || height <= 0 {
		g.Close()
		g.tiles = nil
		g.tilesX = 0
		g.tilesY = 0
		g.width = 0
		g.height = 0
		return
	}

	newTilesX := (width + TileWidth - 1) / TileWidth
	newTilesY := (height + TileHeight - 1) / TileHeight

	// Check if dimensions actually changed
	if g.width == width && g.height == height {
		return
	}

	// Return old tiles to pool
	g.Close()

	// Update dimensions
	g.tilesX = newTilesX
	g.tilesY = newTilesY
	g.width = width
	g.height = height

	// Allocate new tiles
	g.tiles = make([]*Tile, g.tilesX*g.tilesY)
	g.allocateTiles()
}

// TileAt returns the tile at tile coordinates (tx, ty).
// Returns nil if coordinates are out of bounds.
func (g *TileGrid) TileAt(tx, ty int) *Tile {
	if tx < 0 || tx >= g.tilesX || ty < 0 || ty >= g.tilesY {
		return nil
	}
	return g.tiles[ty*g.tilesX+tx]
}

// TileAtPixel returns the tile containing the pixel at canvas coordinates (px, py).
// Returns nil if coordinates are out of bounds.
func (g *TileGrid) TileAtPixel(px, py int) *Tile {
	if px < 0 || px >= g.width || py < 0 || py >= g.height {
		return nil
	}
	tx := px / TileWidth
	ty := py / TileHeight
	return g.tiles[ty*g.tilesX+tx]
}

// TilesInRect returns all tiles that intersect the given rectangle.
// Coordinates are in canvas pixel space.
// Returns nil if the rectangle is completely outside the canvas.
func (g *TileGrid) TilesInRect(x, y, w, h int) []*Tile {
	if w <= 0 || h <= 0 {
		return nil
	}

	// Clamp rectangle to canvas bounds
	x1 := max(x, 0)
	y1 := max(y, 0)
	x2 := min(x+w, g.width)
	y2 := min(y+h, g.height)

	if x1 >= x2 || y1 >= y2 {
		return nil
	}

	// Convert to tile coordinates
	tx1 := x1 / TileWidth
	ty1 := y1 / TileHeight
	tx2 := (x2 - 1) / TileWidth
	ty2 := (y2 - 1) / TileHeight

	// Collect tiles
	result := make([]*Tile, 0, (tx2-tx1+1)*(ty2-ty1+1))
	for ty := ty1; ty <= ty2; ty++ {
		for tx := tx1; tx <= tx2; tx++ {
			if tile := g.TileAt(tx, ty); tile != nil {
				result = append(result, tile)
			}
		}
	}

	return result
}

// MarkDirty marks the tile at tile coordinates (tx, ty) as dirty.
// Does nothing if coordinates are out of bounds.
func (g *TileGrid) MarkDirty(tx, ty int) {
	if tile := g.TileAt(tx, ty); tile != nil {
		tile.Dirty = true
	}
}

// MarkRectDirty marks all tiles intersecting the pixel rectangle as dirty.
// Coordinates are in canvas pixel space.
func (g *TileGrid) MarkRectDirty(x, y, w, h int) {
	tiles := g.TilesInRect(x, y, w, h)
	for _, tile := range tiles {
		tile.Dirty = true
	}
}

// MarkAllDirty marks all tiles as dirty.
func (g *TileGrid) MarkAllDirty() {
	for _, tile := range g.tiles {
		if tile != nil {
			tile.Dirty = true
		}
	}
}

// DirtyTiles returns all tiles that are marked as dirty.
// The returned slice is newly allocated and can be safely modified.
func (g *TileGrid) DirtyTiles() []*Tile {
	result := make([]*Tile, 0, len(g.tiles))
	for _, tile := range g.tiles {
		if tile != nil && tile.Dirty {
			result = append(result, tile)
		}
	}
	return result
}

// ClearDirty resets the dirty flag on all tiles.
func (g *TileGrid) ClearDirty() {
	for _, tile := range g.tiles {
		if tile != nil {
			tile.Dirty = false
		}
	}
}

// TileCount returns the total number of tiles in the grid.
func (g *TileGrid) TileCount() int {
	return len(g.tiles)
}

// TilesX returns the number of tiles horizontally.
func (g *TileGrid) TilesX() int {
	return g.tilesX
}

// TilesY returns the number of tiles vertically.
func (g *TileGrid) TilesY() int {
	return g.tilesY
}

// Width returns the canvas width in pixels.
func (g *TileGrid) Width() int {
	return g.width
}

// Height returns the canvas height in pixels.
func (g *TileGrid) Height() int {
	return g.height
}

// AllTiles returns all tiles in the grid.
// The returned slice should not be modified.
func (g *TileGrid) AllTiles() []*Tile {
	return g.tiles
}

// Close releases all tiles back to the pool.
// The grid should not be used after calling Close.
func (g *TileGrid) Close() {
	for i, tile := range g.tiles {
		if tile != nil {
			g.pool.Put(tile)
			g.tiles[i] = nil
		}
	}
}

// ForEach calls fn for each tile in the grid.
// Tiles are visited in row-major order (left-to-right, top-to-bottom).
func (g *TileGrid) ForEach(fn func(tile *Tile)) {
	for _, tile := range g.tiles {
		if tile != nil {
			fn(tile)
		}
	}
}

// ForEachDirty calls fn for each dirty tile in the grid.
func (g *TileGrid) ForEachDirty(fn func(tile *Tile)) {
	for _, tile := range g.tiles {
		if tile != nil && tile.Dirty {
			fn(tile)
		}
	}
}
