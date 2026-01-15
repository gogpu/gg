package native

import (
	"sort"
)

// TileWidth is the width of a tile in pixels (matches TileSize).
const TileWidth = TileSize

// TileHeight is the height of a tile in pixels (matches TileSize).
const TileHeight = TileSize

// CoarseRasterizer performs coarse rasterization of line segments into tiles.
// It bins segments into the tiles they intersect and tracks winding information.
type CoarseRasterizer struct {
	grid     *TileGrid
	segments *SegmentList
	entries  []CoarseTileEntry

	// Viewport dimensions in pixels
	width  uint16
	height uint16

	// Tile dimensions
	tileColumns uint16
	tileRows    uint16
}

// CoarseTileEntry represents a tile with its associated line segment.
// This is used during the coarse rasterization phase before fine rasterization.
type CoarseTileEntry struct {
	X       uint16 // Tile X coordinate
	Y       uint16 // Tile Y coordinate
	LineIdx uint32 // Index into segment list
	Winding bool   // True if segment contributes winding at this tile
}

// NewCoarseRasterizer creates a new coarse rasterizer for the given dimensions.
func NewCoarseRasterizer(width, height uint16) *CoarseRasterizer {
	cr := &CoarseRasterizer{
		width:       width,
		height:      height,
		tileColumns: (width + TileWidth - 1) / TileWidth,
		tileRows:    (height + TileHeight - 1) / TileHeight,
		entries:     make([]CoarseTileEntry, 0, 256),
	}
	cr.grid = NewTileGrid()
	return cr
}

// Reset clears the rasterizer state for reuse.
func (cr *CoarseRasterizer) Reset() {
	cr.grid.Reset()
	cr.segments = nil
	cr.entries = cr.entries[:0]
}

// Rasterize performs coarse rasterization of segments into tiles.
// It determines which tiles each segment intersects and calculates winding.
func (cr *CoarseRasterizer) Rasterize(segments *SegmentList) {
	cr.segments = segments
	cr.entries = cr.entries[:0]

	if segments == nil || segments.Len() == 0 {
		return
	}

	if cr.width == 0 || cr.height == 0 {
		return
	}

	lines := segments.Segments()
	for lineIdx, line := range lines {
		//nolint:gosec // lineIdx is bounded by segments length which fits in uint32
		cr.rasterizeLine(line, uint32(lineIdx))
	}
}

// rasterizeLine determines which tiles a single line segment intersects.
// This implements the coarse binning phase from vello.
func (cr *CoarseRasterizer) rasterizeLine(line LineSegment, lineIdx uint32) {
	// Convert to tile coordinates
	p0x := line.X0 / float32(TileWidth)
	p0y := line.Y0 / float32(TileHeight)
	p1x := line.X1 / float32(TileWidth)
	p1y := line.Y1 / float32(TileHeight)

	// Determine left/right bounds
	lineLeftX := minf32(p0x, p1x)
	lineRightX := maxf32(p0x, p1x)

	// Cull lines fully to the right of viewport
	if lineLeftX > float32(cr.tileColumns) {
		return
	}

	// Determine top/bottom (lines are guaranteed monotonic Y: Y0 <= Y1)
	lineTopY := p0y
	lineTopX := p0x
	lineBottomY := p1y
	lineBottomX := p1x

	// Clamp to viewport rows
	yTopTiles := clampU16(int32(lineTopY), 0, int32(cr.tileRows))
	yBottomTiles := clampU16(int32(lineBottomY+0.999999), 0, int32(cr.tileRows))

	// Skip horizontal lines or lines fully outside viewport
	if yTopTiles >= yBottomTiles {
		return
	}

	// Get tile coordinates for endpoints
	p0TileX := int32(lineTopX)
	p0TileY := int32(lineTopY)
	p1TileX := int32(lineBottomX)
	p1TileY := int32(lineBottomY)

	// Check if both endpoints are in the same tile
	sameX := p0TileX == p1TileX
	sameY := p0TileY == p1TileY

	if sameX && sameY {
		// Line fully contained in single tile
		x := clampU16(int32(lineLeftX), 0, int32(cr.tileColumns-1))
		// Set winding if line crosses tile top edge
		winding := p0TileY >= int32(yTopTiles)
		cr.addEntry(x, yTopTiles, lineIdx, winding)
		return
	}

	// Handle vertical lines specially
	if lineLeftX == lineRightX {
		cr.rasterizeVerticalLine(lineLeftX, lineTopY, lineBottomY, lineIdx)
		return
	}

	// General sloped line
	cr.rasterizeSlopedLine(lineIdx, lineTopX, lineTopY, lineBottomX, lineBottomY, yTopTiles, yBottomTiles)
}

// addEntry adds a coarse tile entry.
func (cr *CoarseRasterizer) addEntry(x, y uint16, lineIdx uint32, winding bool) {
	cr.entries = append(cr.entries, CoarseTileEntry{
		X:       x,
		Y:       y,
		LineIdx: lineIdx,
		Winding: winding,
	})
}

// rasterizeVerticalLine handles vertical line segments.
func (cr *CoarseRasterizer) rasterizeVerticalLine(x float32, topY, bottomY float32, lineIdx uint32) {
	xTile := clampU16(int32(x), 0, int32(cr.tileColumns-1))

	yTopTiles := clampU16(int32(topY), 0, int32(cr.tileRows))
	yBottomTiles := clampU16(int32(bottomY+0.999999), 0, int32(cr.tileRows))

	if yTopTiles >= yBottomTiles {
		return
	}

	// First tile - check if line starts above tile top
	isStartCulled := topY < 0
	if !isStartCulled {
		winding := float32(yTopTiles) >= topY
		cr.addEntry(xTile, yTopTiles, lineIdx, winding)
	}

	// Middle tiles - line crosses top and bottom
	yStart := yTopTiles
	if !isStartCulled {
		yStart++
	}
	yEndIdx := clampU16(int32(bottomY), 0, int32(cr.tileRows))

	for y := yStart; y < yEndIdx; y++ {
		cr.addEntry(xTile, y, lineIdx, true) // Winding always true for middle tiles
	}

	// Last tile if line doesn't end exactly on tile boundary
	bottomFloor := float32(int32(bottomY))
	if bottomY != bottomFloor && yEndIdx < cr.tileRows {
		cr.addEntry(xTile, yEndIdx, lineIdx, true)
	}
}

// rasterizeSlopedLine handles general sloped line segments.
func (cr *CoarseRasterizer) rasterizeSlopedLine(
	lineIdx uint32,
	lineTopX, lineTopY, lineBottomX, lineBottomY float32,
	yTopTiles, _ uint16, // yBottomTiles - reserved for future clipping
) {
	dx := lineBottomX - lineTopX
	dy := lineBottomY - lineTopY
	xSlope := dx / dy

	// Determine winding direction based on slope direction
	dxDir := lineBottomX >= lineTopX

	lineLeftX := minf32(lineTopX, lineBottomX)
	lineRightX := maxf32(lineTopX, lineBottomX)

	isStartCulled := lineTopY < 0

	// Process first row (if not culled)
	if !isStartCulled {
		y := float32(yTopTiles)
		rowBottomY := minf32(y+1.0, lineBottomY)
		winding := y >= lineTopY
		cr.processRow(lineIdx, lineTopY, y, rowBottomY, xSlope, lineTopX, lineLeftX, lineRightX, yTopTiles, winding, dxDir)
	}

	// Process middle rows
	yStartMiddle := yTopTiles
	if !isStartCulled {
		yStartMiddle++
	}
	yEndMiddle := clampU16(int32(lineBottomY), 0, int32(cr.tileRows))

	for y := yStartMiddle; y < yEndMiddle; y++ {
		yf := float32(y)
		rowBottomY := minf32(yf+1.0, lineBottomY)
		cr.processRow(lineIdx, lineTopY, yf, rowBottomY, xSlope, lineTopX, lineLeftX, lineRightX, y, true, dxDir)
	}

	// Process last row if line doesn't end on row boundary
	bottomFloor := float32(int32(lineBottomY))
	if lineBottomY != bottomFloor && yEndMiddle < cr.tileRows {
		if isStartCulled || yEndMiddle != yTopTiles {
			yf := float32(yEndMiddle)
			cr.processRow(lineIdx, lineTopY, yf, lineBottomY, xSlope, lineTopX, lineLeftX, lineRightX, yEndMiddle, true, dxDir)
		}
	}
}

// processRow processes a single row of tiles for a sloped line.
func (cr *CoarseRasterizer) processRow(
	lineIdx uint32,
	lineTopY float32, // Original line top Y for calculating X positions
	rowTopY, rowBottomY float32,
	xSlope, lineTopX float32,
	lineLeftX, lineRightX float32,
	yIdx uint16,
	winding bool,
	dxDir bool,
) {
	// Calculate X range for this row using line equation
	// x = lineTopX + (y - lineTopY) * xSlope
	rowTopX := lineTopX + (rowTopY-lineTopY)*xSlope
	rowBottomX := lineTopX + (rowBottomY-lineTopY)*xSlope

	// Clamp to line bounds
	rowLeftX := maxf32(minf32(rowTopX, rowBottomX), lineLeftX)
	rowRightX := minf32(maxf32(rowTopX, rowBottomX), lineRightX)

	xStart := clampU16(int32(rowLeftX), 0, int32(cr.tileColumns-1))
	xEnd := clampU16(int32(rowRightX), 0, int32(cr.tileColumns-1))

	if xStart > xEnd {
		return
	}

	// Single tile case
	if xStart == xEnd {
		cr.addEntry(xStart, yIdx, lineIdx, winding)
		return
	}

	// Multiple tiles
	// First tile gets winding based on direction
	if dxDir {
		// Going right: left tile gets winding
		cr.addEntry(xStart, yIdx, lineIdx, winding)
	} else {
		// Going left: right tile gets winding
		cr.addEntry(xStart, yIdx, lineIdx, false)
	}

	// Middle tiles (no winding)
	for x := xStart + 1; x < xEnd; x++ {
		cr.addEntry(x, yIdx, lineIdx, false)
	}

	// Last tile
	if dxDir {
		cr.addEntry(xEnd, yIdx, lineIdx, false)
	} else {
		cr.addEntry(xEnd, yIdx, lineIdx, winding)
	}
}

// Grid returns the tile grid after rasterization.
func (cr *CoarseRasterizer) Grid() *TileGrid {
	return cr.grid
}

// Entries returns the coarse tile entries.
func (cr *CoarseRasterizer) Entries() []CoarseTileEntry {
	return cr.entries
}

// Segments returns the segment list.
func (cr *CoarseRasterizer) Segments() *SegmentList {
	return cr.segments
}

// SortEntries sorts the entries for efficient rendering.
// Tiles are sorted by Y, then X, then line index.
func (cr *CoarseRasterizer) SortEntries() {
	sort.Slice(cr.entries, func(i, j int) bool {
		ei, ej := cr.entries[i], cr.entries[j]
		if ei.Y != ej.Y {
			return ei.Y < ej.Y
		}
		if ei.X != ej.X {
			return ei.X < ej.X
		}
		return ei.LineIdx < ej.LineIdx
	})
}

// TileColumns returns the number of tile columns.
func (cr *CoarseRasterizer) TileColumns() uint16 {
	return cr.tileColumns
}

// TileRows returns the number of tile rows.
func (cr *CoarseRasterizer) TileRows() uint16 {
	return cr.tileRows
}

// CalculateBackdrop calculates the backdrop winding for fine rasterization.
// Returns a slice of backdrop values indexed by [y * columns + x].
func (cr *CoarseRasterizer) CalculateBackdrop() []int32 {
	if cr.segments == nil || len(cr.entries) == 0 {
		return nil
	}

	backdrop := make([]int32, int(cr.tileColumns)*int(cr.tileRows))

	// Ensure entries are sorted
	cr.SortEntries()

	lines := cr.segments.Segments()

	// Process each row
	currentY := uint16(0xFFFF)
	rowWinding := int32(0)

	for _, entry := range cr.entries {
		// New row?
		if entry.Y != currentY {
			currentY = entry.Y
			rowWinding = 0
		}

		// Get backdrop for this tile position
		idx := int(entry.Y)*int(cr.tileColumns) + int(entry.X)
		if idx < len(backdrop) {
			backdrop[idx] = rowWinding
		}

		// Update winding based on segment direction
		if entry.Winding && int(entry.LineIdx) < len(lines) {
			line := lines[entry.LineIdx]
			rowWinding += int32(line.Winding)
		}
	}

	return backdrop
}

// clampU16 clamps value to [minVal, maxVal] range and returns as uint16.
//
//nolint:gosec,unparam // Integer overflow is acceptable; minVal kept for API flexibility
func clampU16(val, minVal, maxVal int32) uint16 {
	if val < minVal {
		return uint16(minVal)
	}
	if val > maxVal {
		return uint16(maxVal)
	}
	return uint16(val)
}

// CoarseTileIterator provides iteration over coarse tiles in sorted order.
type CoarseTileIterator struct {
	rasterizer *CoarseRasterizer
	index      int
	sorted     bool
}

// NewIterator creates an iterator for the coarse tile entries.
func (cr *CoarseRasterizer) NewIterator() *CoarseTileIterator {
	if !cr.isSorted() {
		cr.SortEntries()
	}
	return &CoarseTileIterator{
		rasterizer: cr,
		index:      0,
		sorted:     true,
	}
}

// isSorted returns true if entries are sorted (simple heuristic).
func (cr *CoarseRasterizer) isSorted() bool {
	if len(cr.entries) < 2 {
		return true
	}
	// Check a few entries as a heuristic
	for i := 1; i < len(cr.entries) && i < 10; i++ {
		ei, ej := cr.entries[i-1], cr.entries[i]
		if ei.Y > ej.Y || (ei.Y == ej.Y && ei.X > ej.X) {
			return false
		}
	}
	return true
}

// Next returns the next tile entry or nil if done.
func (it *CoarseTileIterator) Next() *CoarseTileEntry {
	if it.index >= len(it.rasterizer.entries) {
		return nil
	}
	entry := &it.rasterizer.entries[it.index]
	it.index++
	return entry
}

// HasNext returns true if there are more entries.
func (it *CoarseTileIterator) HasNext() bool {
	return it.index < len(it.rasterizer.entries)
}

// Reset resets the iterator to the beginning.
func (it *CoarseTileIterator) Reset() {
	it.index = 0
}

// EntriesAtLocation returns all entries at the given tile location.
func (cr *CoarseRasterizer) EntriesAtLocation(x, y uint16) []CoarseTileEntry {
	var result []CoarseTileEntry
	for _, e := range cr.entries {
		if e.X == x && e.Y == y {
			result = append(result, e)
		}
	}
	return result
}
