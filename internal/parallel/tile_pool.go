package parallel

import "sync"

// TilePool provides efficient reuse of Tile instances via sync.Pool.
//
// The pool reduces GC pressure by reusing tile memory. When a tile is
// returned to the pool, its data is cleared and ready for reuse.
//
// Thread safety: TilePool is safe for concurrent use.
type TilePool struct {
	// pools holds separate sync.Pool instances for each tile size.
	// Key format: (width << 16) | height
	pools sync.Map

	// fullTilePool is the dedicated pool for full-size tiles (64x64).
	// This is the most common case, so we optimize for it.
	fullTilePool sync.Pool
}

// NewTilePool creates a new tile pool.
func NewTilePool() *TilePool {
	p := &TilePool{}

	// Pre-configure the full tile pool
	p.fullTilePool.New = func() any {
		return &Tile{
			Width:  TileWidth,
			Height: TileHeight,
			Data:   make([]byte, TileBytes),
		}
	}

	return p
}

// Get retrieves a tile from the pool or creates a new one.
// The tile is guaranteed to have the specified dimensions.
// The tile data is zeroed and ready for use.
func (p *TilePool) Get(width, height int) *Tile {
	if width <= 0 || height <= 0 {
		return nil
	}

	// Fast path for full-size tiles
	if width == TileWidth && height == TileHeight {
		tile := p.fullTilePool.Get().(*Tile)
		tile.Reset()
		tile.X = 0
		tile.Y = 0
		tile.Dirty = false
		return tile
	}

	// Slow path for edge tiles (different sizes)
	key := poolKey(width, height)
	pool := p.getOrCreatePool(key, width, height)

	tile := pool.Get().(*Tile)
	tile.Reset()
	tile.X = 0
	tile.Y = 0
	tile.Width = width
	tile.Height = height
	return tile
}

// Put returns a tile to the pool for reuse.
// The tile data will be cleared.
// If tile is nil, this is a no-op.
func (p *TilePool) Put(tile *Tile) {
	if tile == nil {
		return
	}

	// Clear data before returning to pool
	tile.Reset()

	// Fast path for full-size tiles
	if tile.Width == TileWidth && tile.Height == TileHeight {
		p.fullTilePool.Put(tile)
		return
	}

	// Slow path for edge tiles
	key := poolKey(tile.Width, tile.Height)
	if pool, ok := p.pools.Load(key); ok {
		pool.(*sync.Pool).Put(tile)
	}
	// If pool doesn't exist, let GC reclaim the tile
}

// poolKey creates a unique key for a tile size.
// Width and height are clamped to 16-bit values to prevent overflow.
func poolKey(width, height int) uint32 {
	// Clamp to uint16 range to prevent overflow (max 65535)
	w := width
	h := height
	if w > 0xFFFF {
		w = 0xFFFF
	}
	if h > 0xFFFF {
		h = 0xFFFF
	}
	return uint32(w)<<16 | uint32(h) //nolint:gosec // values are clamped above
}

// getOrCreatePool gets or creates a sync.Pool for the given dimensions.
func (p *TilePool) getOrCreatePool(key uint32, width, height int) *sync.Pool {
	if pool, ok := p.pools.Load(key); ok {
		return pool.(*sync.Pool)
	}

	// Create new pool for this size
	newPool := &sync.Pool{
		New: func() any {
			dataSize := width * height * 4
			return &Tile{
				Width:  width,
				Height: height,
				Data:   make([]byte, dataSize),
			}
		},
	}

	// Try to store; if another goroutine beat us, use theirs
	actual, _ := p.pools.LoadOrStore(key, newPool)
	return actual.(*sync.Pool)
}

// defaultPool is the package-level tile pool for convenient usage.
var defaultPool = NewTilePool()

// GetTile retrieves a tile from the default pool.
func GetTile(width, height int) *Tile {
	return defaultPool.Get(width, height)
}

// PutTile returns a tile to the default pool.
func PutTile(tile *Tile) {
	defaultPool.Put(tile)
}
