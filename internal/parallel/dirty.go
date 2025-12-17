package parallel

import (
	"math/bits"
	"sync/atomic"
)

// DirtyRegion tracks which tiles need redrawing using an atomic bitmap.
// It provides lock-free, thread-safe operations for concurrent access.
//
// The bitmap uses one bit per tile, packed into uint64 words (64 tiles per word).
// All methods are safe for concurrent use without external synchronization.
//
// This is designed for incremental rendering where only changed regions
// need to be redrawn, significantly improving performance for static content.
type DirtyRegion struct {
	// words is the atomic bitmap where each bit represents a tile's dirty state.
	// Bit index = ty * tilesX + tx
	// Word index = bit index / 64
	// Bit position = bit index % 64
	words []atomic.Uint64

	// tilesX is the number of tiles horizontally.
	tilesX int

	// tilesY is the number of tiles vertically.
	tilesY int
}

// NewDirtyRegion creates a new dirty region tracker for the given tile grid dimensions.
// All tiles start as clean (not dirty).
// Returns nil if dimensions are invalid (zero or negative).
func NewDirtyRegion(tilesX, tilesY int) *DirtyRegion {
	if tilesX <= 0 || tilesY <= 0 {
		return nil
	}

	totalTiles := tilesX * tilesY
	numWords := (totalTiles + 63) / 64 // Ceiling division

	return &DirtyRegion{
		words:  make([]atomic.Uint64, numWords),
		tilesX: tilesX,
		tilesY: tilesY,
	}
}

// Mark marks a single tile as dirty (needs redrawing).
// This is a lock-free O(1) operation using atomic OR.
// Does nothing if coordinates are out of bounds.
func (d *DirtyRegion) Mark(tx, ty int) {
	if tx < 0 || tx >= d.tilesX || ty < 0 || ty >= d.tilesY {
		return
	}
	idx := ty*d.tilesX + tx
	wordIdx := idx / 64
	bitIdx := idx & 63 // equivalent to idx % 64, always in [0, 63]
	d.words[wordIdx].Or(1 << bitIdx)
}

// MarkRect marks all tiles that intersect the given pixel rectangle as dirty.
// Coordinates are in pixel space, not tile space.
// Does nothing if the rectangle is invalid or completely outside the grid.
func (d *DirtyRegion) MarkRect(x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}

	// Convert pixel coordinates to tile coordinates
	// Note: TileWidth and TileHeight are defined in tile.go
	tx1 := x / TileWidth
	ty1 := y / TileHeight
	tx2 := (x + w - 1) / TileWidth
	ty2 := (y + h - 1) / TileHeight

	// Clamp to valid range
	if tx1 < 0 {
		tx1 = 0
	}
	if ty1 < 0 {
		ty1 = 0
	}
	if tx2 >= d.tilesX {
		tx2 = d.tilesX - 1
	}
	if ty2 >= d.tilesY {
		ty2 = d.tilesY - 1
	}

	// Check if rectangle is completely outside
	if tx1 > tx2 || ty1 > ty2 {
		return
	}

	// Mark all tiles in range
	for ty := ty1; ty <= ty2; ty++ {
		for tx := tx1; tx <= tx2; tx++ {
			d.Mark(tx, ty)
		}
	}
}

// MarkAll marks all tiles as dirty.
// This is useful when the entire content needs to be redrawn.
func (d *DirtyRegion) MarkAll() {
	totalTiles := d.tilesX * d.tilesY
	fullWords := totalTiles / 64
	remainder := totalTiles % 64

	// Set all bits in full words
	for i := 0; i < fullWords; i++ {
		d.words[i].Store(^uint64(0))
	}

	// Set remaining bits in the last partial word
	if remainder > 0 {
		mask := (uint64(1) << remainder) - 1
		d.words[fullWords].Store(mask)
	}
}

// Clear clears all dirty flags (marks all tiles as clean).
func (d *DirtyRegion) Clear() {
	for i := range d.words {
		d.words[i].Store(0)
	}
}

// IsDirty returns true if the tile at (tx, ty) is marked as dirty.
// Returns false for out-of-bounds coordinates.
func (d *DirtyRegion) IsDirty(tx, ty int) bool {
	if tx < 0 || tx >= d.tilesX || ty < 0 || ty >= d.tilesY {
		return false
	}
	idx := ty*d.tilesX + tx
	wordIdx := idx / 64
	bitIdx := idx & 63 // equivalent to idx % 64, always in [0, 63]
	return d.words[wordIdx].Load()&(1<<bitIdx) != 0
}

// IsEmpty returns true if no tiles are marked as dirty.
func (d *DirtyRegion) IsEmpty() bool {
	for i := range d.words {
		if d.words[i].Load() != 0 {
			return false
		}
	}
	return true
}

// Count returns the number of tiles marked as dirty.
func (d *DirtyRegion) Count() int {
	count := 0
	totalTiles := d.tilesX * d.tilesY
	fullWords := totalTiles / 64

	// Count bits in full words
	for i := 0; i < fullWords; i++ {
		count += bits.OnesCount64(d.words[i].Load())
	}

	// Count bits in partial word (if any)
	if fullWords < len(d.words) {
		remainder := totalTiles % 64
		mask := (uint64(1) << remainder) - 1
		count += bits.OnesCount64(d.words[fullWords].Load() & mask)
	}

	return count
}

// GetAndClear atomically retrieves all dirty tile coordinates and clears them.
// Returns a slice of [2]int{tx, ty} for each dirty tile.
// This is the primary method for processing dirty tiles during rendering.
func (d *DirtyRegion) GetAndClear() [][2]int {
	var dirty [][2]int
	totalTiles := d.tilesX * d.tilesY

	for wordIdx := range d.words {
		// Atomically swap the word with 0 to get and clear
		word := d.words[wordIdx].Swap(0)
		if word == 0 {
			continue
		}

		// Extract all set bits
		for word != 0 {
			// Find position of lowest set bit
			bitIdx := bits.TrailingZeros64(word)

			// Calculate tile index
			tileIdx := wordIdx*64 + bitIdx
			if tileIdx >= totalTiles {
				// Beyond valid tiles (in partial last word)
				break
			}

			// Convert to tile coordinates
			tx := tileIdx % d.tilesX
			ty := tileIdx / d.tilesX
			dirty = append(dirty, [2]int{tx, ty})

			// Clear the processed bit
			word &^= 1 << bitIdx
		}
	}

	return dirty
}

// ForEachDirty calls fn for each dirty tile without clearing the dirty flags.
// Tiles are visited in row-major order (left-to-right, top-to-bottom).
func (d *DirtyRegion) ForEachDirty(fn func(tx, ty int)) {
	if fn == nil {
		return
	}

	totalTiles := d.tilesX * d.tilesY

	for wordIdx := range d.words {
		word := d.words[wordIdx].Load()
		if word == 0 {
			continue
		}

		// Extract all set bits
		for word != 0 {
			// Find position of lowest set bit
			bitIdx := bits.TrailingZeros64(word)

			// Calculate tile index
			tileIdx := wordIdx*64 + bitIdx
			if tileIdx >= totalTiles {
				// Beyond valid tiles (in partial last word)
				break
			}

			// Convert to tile coordinates
			tx := tileIdx % d.tilesX
			ty := tileIdx / d.tilesX
			fn(tx, ty)

			// Clear the processed bit (local copy only)
			word &^= 1 << bitIdx
		}
	}
}

// Resize creates a new dirty region with the specified dimensions.
// All tiles in the new region are marked as dirty.
// Returns nil if dimensions are invalid.
//
// Note: This is not an in-place resize; it returns a new DirtyRegion.
// For thread-safe usage, the caller should atomically swap the pointer.
func (d *DirtyRegion) Resize(tilesX, tilesY int) *DirtyRegion {
	newRegion := NewDirtyRegion(tilesX, tilesY)
	if newRegion != nil {
		newRegion.MarkAll()
	}
	return newRegion
}

// TilesX returns the number of tiles horizontally.
func (d *DirtyRegion) TilesX() int {
	return d.tilesX
}

// TilesY returns the number of tiles vertically.
func (d *DirtyRegion) TilesY() int {
	return d.tilesY
}

// TotalTiles returns the total number of tiles in the region.
func (d *DirtyRegion) TotalTiles() int {
	return d.tilesX * d.tilesY
}
