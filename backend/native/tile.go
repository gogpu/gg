package native

import (
	"sync"

	"github.com/gogpu/gg/scene"
)

// TileSize is the width and height of a tile in pixels.
// 4×4 is optimal for cache efficiency and SIMD processing.
const TileSize = 4

// TileShift is log2(TileSize) for efficient division.
const TileShift = 2

// TileMask is TileSize - 1 for efficient modulo.
const TileMask = TileSize - 1

// Tile represents a 4×4 pixel tile with coverage data.
// This is the core unit of the sparse strips algorithm.
//
// The coverage array stores anti-aliased alpha values for each pixel
// in row-major order: [row0: 0-3, row1: 4-7, row2: 8-11, row3: 12-15]
type Tile struct {
	// X, Y are tile coordinates (not pixel coordinates).
	// Pixel coordinates are (X * TileSize, Y * TileSize).
	X, Y int32

	// Coverage holds 4×4 = 16 anti-aliased coverage values (0-255).
	// Index = row * TileSize + col
	Coverage [TileSize * TileSize]uint8

	// Backdrop is the winding number entering this tile from the left.
	// Used for propagating fill state across tile boundaries.
	Backdrop int16

	// SegmentCount tracks how many segments cross this tile.
	// Used for optimization: tiles with 0 segments use backdrop only.
	SegmentCount int16
}

// Reset clears the tile for reuse.
func (t *Tile) Reset() {
	t.X = 0
	t.Y = 0
	t.Backdrop = 0
	t.SegmentCount = 0
	for i := range t.Coverage {
		t.Coverage[i] = 0
	}
}

// PixelX returns the pixel X coordinate of the tile's top-left corner.
func (t *Tile) PixelX() int32 {
	return t.X << TileShift
}

// PixelY returns the pixel Y coordinate of the tile's top-left corner.
func (t *Tile) PixelY() int32 {
	return t.Y << TileShift
}

// SetCoverage sets the coverage value for a pixel within the tile.
// px, py are pixel offsets within the tile (0-3).
func (t *Tile) SetCoverage(px, py int, value uint8) {
	t.Coverage[py*TileSize+px] = value
}

// GetCoverage returns the coverage value for a pixel within the tile.
func (t *Tile) GetCoverage(px, py int) uint8 {
	return t.Coverage[py*TileSize+px]
}

// FillSolid fills the entire tile with a single coverage value.
func (t *Tile) FillSolid(value uint8) {
	for i := range t.Coverage {
		t.Coverage[i] = value
	}
}

// IsEmpty returns true if all coverage values are zero.
func (t *Tile) IsEmpty() bool {
	for _, c := range t.Coverage {
		if c != 0 {
			return false
		}
	}
	return true
}

// IsSolid returns true if all coverage values are 255 (fully opaque).
func (t *Tile) IsSolid() bool {
	for _, c := range t.Coverage {
		if c != 255 {
			return false
		}
	}
	return true
}

// TileCoord represents tile coordinates for hashing.
type TileCoord struct {
	X, Y int32
}

// Key returns a unique key for the tile coordinate.
func (tc TileCoord) Key() uint64 {
	//nolint:gosec // Safe conversion for tile coordinates
	return uint64(tc.Y)<<32 | uint64(uint32(tc.X))
}

// TileGrid is a sparse collection of non-empty tiles.
// It uses a hashmap for O(1) tile lookup by coordinates.
type TileGrid struct {
	// tiles stores non-empty tiles by coordinate key.
	tiles map[uint64]*Tile

	// bounds tracks the bounding rectangle in tile coordinates.
	minX, minY, maxX, maxY int32

	// pool for tile reuse to reduce allocations.
	pool *TilePool

	// fillRule for coverage interpretation.
	fillRule scene.FillStyle
}

// NewTileGrid creates a new empty tile grid.
func NewTileGrid() *TileGrid {
	return &TileGrid{
		tiles:    make(map[uint64]*Tile, 256),
		pool:     globalTilePool,
		fillRule: scene.FillNonZero,
		minX:     1<<30 - 1,
		minY:     1<<30 - 1,
		maxX:     -(1<<30 - 1),
		maxY:     -(1<<30 - 1),
	}
}

// Reset clears the grid for reuse.
func (g *TileGrid) Reset() {
	// Return tiles to pool
	for _, tile := range g.tiles {
		g.pool.Put(tile)
	}
	// Clear map but keep capacity
	for k := range g.tiles {
		delete(g.tiles, k)
	}
	g.minX = 1<<30 - 1
	g.minY = 1<<30 - 1
	g.maxX = -(1<<30 - 1)
	g.maxY = -(1<<30 - 1)
}

// SetFillRule sets the fill rule for coverage calculation.
func (g *TileGrid) SetFillRule(rule scene.FillStyle) {
	g.fillRule = rule
}

// FillRule returns the current fill rule.
func (g *TileGrid) FillRule() scene.FillStyle {
	return g.fillRule
}

// GetOrCreate returns the tile at the given coordinates, creating if needed.
func (g *TileGrid) GetOrCreate(x, y int32) *Tile {
	key := TileCoord{X: x, Y: y}.Key()
	if tile, ok := g.tiles[key]; ok {
		return tile
	}

	tile := g.pool.Get()
	tile.X = x
	tile.Y = y
	g.tiles[key] = tile

	// Update bounds
	if x < g.minX {
		g.minX = x
	}
	if x > g.maxX {
		g.maxX = x
	}
	if y < g.minY {
		g.minY = y
	}
	if y > g.maxY {
		g.maxY = y
	}

	return tile
}

// Get returns the tile at coordinates, or nil if not present.
func (g *TileGrid) Get(x, y int32) *Tile {
	key := TileCoord{X: x, Y: y}.Key()
	return g.tiles[key]
}

// Has returns true if a tile exists at the coordinates.
func (g *TileGrid) Has(x, y int32) bool {
	key := TileCoord{X: x, Y: y}.Key()
	_, ok := g.tiles[key]
	return ok
}

// TileCount returns the number of non-empty tiles.
func (g *TileGrid) TileCount() int {
	return len(g.tiles)
}

// Bounds returns the bounding rectangle in tile coordinates.
func (g *TileGrid) Bounds() (minX, minY, maxX, maxY int32) {
	return g.minX, g.minY, g.maxX, g.maxY
}

// PixelBounds returns the bounding rectangle in pixel coordinates.
func (g *TileGrid) PixelBounds() scene.Rect {
	if len(g.tiles) == 0 {
		return scene.EmptyRect()
	}
	return scene.Rect{
		MinX: float32(g.minX << TileShift),
		MinY: float32(g.minY << TileShift),
		MaxX: float32((g.maxX + 1) << TileShift),
		MaxY: float32((g.maxY + 1) << TileShift),
	}
}

// ForEach iterates over all tiles in unspecified order.
func (g *TileGrid) ForEach(fn func(*Tile)) {
	for _, tile := range g.tiles {
		fn(tile)
	}
}

// ForEachSorted iterates over tiles sorted by Y, then X.
// This is important for correct backdrop propagation.
func (g *TileGrid) ForEachSorted(fn func(*Tile)) {
	if len(g.tiles) == 0 {
		return
	}

	// Collect and sort tile coords
	coords := make([]TileCoord, 0, len(g.tiles))
	for _, tile := range g.tiles {
		coords = append(coords, TileCoord{X: tile.X, Y: tile.Y})
	}

	// Sort by Y, then X (insertion sort for typically small grids)
	for i := 1; i < len(coords); i++ {
		j := i
		for j > 0 && (coords[j].Y < coords[j-1].Y ||
			(coords[j].Y == coords[j-1].Y && coords[j].X < coords[j-1].X)) {
			coords[j], coords[j-1] = coords[j-1], coords[j]
			j--
		}
	}

	// Iterate in sorted order
	for _, coord := range coords {
		tile := g.tiles[coord.Key()]
		fn(tile)
	}
}

// ForEachInRow iterates over tiles in a specific row, sorted by X.
func (g *TileGrid) ForEachInRow(y int32, fn func(*Tile)) {
	// Collect tiles in this row
	var rowTiles []*Tile
	for _, tile := range g.tiles {
		if tile.Y == y {
			rowTiles = append(rowTiles, tile)
		}
	}

	// Sort by X
	for i := 1; i < len(rowTiles); i++ {
		j := i
		for j > 0 && rowTiles[j].X < rowTiles[j-1].X {
			rowTiles[j], rowTiles[j-1] = rowTiles[j-1], rowTiles[j]
			j--
		}
	}

	for _, tile := range rowTiles {
		fn(tile)
	}
}

// TilePool manages a pool of reusable tiles.
type TilePool struct {
	mu   sync.Mutex
	pool []*Tile
}

// globalTilePool is the default tile pool.
var globalTilePool = NewTilePool()

// NewTilePool creates a new tile pool.
func NewTilePool() *TilePool {
	return &TilePool{
		pool: make([]*Tile, 0, 256),
	}
}

// Get retrieves a tile from the pool or creates a new one.
func (p *TilePool) Get() *Tile {
	p.mu.Lock()
	if len(p.pool) > 0 {
		tile := p.pool[len(p.pool)-1]
		p.pool = p.pool[:len(p.pool)-1]
		p.mu.Unlock()
		tile.Reset()
		return tile
	}
	p.mu.Unlock()
	return &Tile{}
}

// Put returns a tile to the pool.
func (p *TilePool) Put(tile *Tile) {
	if tile == nil {
		return
	}
	p.mu.Lock()
	// Limit pool size to prevent unbounded growth
	if len(p.pool) < 4096 {
		p.pool = append(p.pool, tile)
	}
	p.mu.Unlock()
}

// PixelToTile converts pixel coordinates to tile coordinates.
// Uses arithmetic shift which provides floor division for negative numbers.
func PixelToTile(px, py int32) (tx, ty int32) {
	// Arithmetic right shift gives floor division for signed integers
	tx = px >> TileShift
	ty = py >> TileShift
	return
}

// PixelToTileF converts float pixel coordinates to tile coordinates.
// Uses floor semantics for correct handling of negative coordinates.
func PixelToTileF(px, py float32) (tx, ty int32) {
	// Floor the pixel coordinates first, then shift
	// For positive: int32 truncation is correct
	// For negative: need floor, not truncation
	floorPx := int32(px)
	if px < 0 && float32(floorPx) != px {
		floorPx--
	}
	floorPy := int32(py)
	if py < 0 && float32(floorPy) != py {
		floorPy--
	}
	tx = floorPx >> TileShift
	ty = floorPy >> TileShift
	return
}
