package gpu

import (
	"github.com/gogpu/gg/internal/raster"
	"github.com/gogpu/gg/scene"
)

// FineRasterizer performs fine rasterization with analytic anti-aliasing.
// It calculates exact sub-pixel coverage for each tile based on the segments
// that cross it and the fill rule.
type FineRasterizer struct {
	grid     *TileGrid
	segments *SegmentList
	backdrop []int32
	fillRule scene.FillStyle

	// Viewport dimensions
	width  uint16
	height uint16

	// Tile dimensions
	tileColumns uint16
	tileRows    uint16
}

// NewFineRasterizer creates a new fine rasterizer for the given dimensions.
func NewFineRasterizer(width, height uint16) *FineRasterizer {
	return &FineRasterizer{
		grid:        NewTileGrid(),
		width:       width,
		height:      height,
		tileColumns: (width + TileWidth - 1) / TileWidth,
		tileRows:    (height + TileHeight - 1) / TileHeight,
		fillRule:    scene.FillNonZero,
	}
}

// Reset clears the rasterizer state for reuse.
func (fr *FineRasterizer) Reset() {
	fr.grid.Reset()
	fr.segments = nil
	fr.backdrop = nil
}

// SetFillRule sets the fill rule for coverage calculation.
func (fr *FineRasterizer) SetFillRule(rule scene.FillStyle) {
	fr.fillRule = rule
	fr.grid.SetFillRule(rule)
}

// raster.FillRule returns the current fill rule.
func (fr *FineRasterizer) FillRule() scene.FillStyle {
	return fr.fillRule
}

// Rasterize performs fine rasterization on coarse tile entries.
// It calculates analytic anti-aliased coverage for each pixel.
func (fr *FineRasterizer) Rasterize(
	coarse *CoarseRasterizer,
	segments *SegmentList,
	backdrop []int32,
) {
	fr.segments = segments
	fr.backdrop = backdrop
	fr.grid.Reset()

	if segments == nil || len(coarse.Entries()) == 0 {
		return
	}

	entries := coarse.Entries()
	lines := segments.Segments()

	// Process tiles in sorted order, grouping by location
	coarse.SortEntries()

	var currentX, currentY uint16 = 0xFFFF, 0xFFFF
	var tileWinding [TileSize][TileSize]float32
	var accumulatedWinding [TileSize]float32

	for i, entry := range entries {
		// Check if we're at a new tile location
		if entry.X != currentX || entry.Y != currentY {
			// Finalize previous tile if any
			if currentX != 0xFFFF {
				fr.finalizeTile(currentX, currentY, &tileWinding, &accumulatedWinding)
			}

			// Start new tile
			currentX, currentY = entry.X, entry.Y
			fr.initTileWinding(&tileWinding, &accumulatedWinding, currentX, currentY)
		}

		// Process segment contribution to this tile
		if int(entry.LineIdx) < len(lines) {
			line := lines[entry.LineIdx]
			fr.processSegment(line, currentX, currentY, &tileWinding, &accumulatedWinding)
		}

		// Check if next entry is at different location (or this is last)
		isLast := i == len(entries)-1
		if isLast || entries[i+1].X != currentX || entries[i+1].Y != currentY {
			fr.finalizeTile(currentX, currentY, &tileWinding, &accumulatedWinding)
			if !isLast {
				currentX, currentY = 0xFFFF, 0xFFFF
			}
		}
	}
}

// initTileWinding initializes the winding arrays for a new tile.
func (fr *FineRasterizer) initTileWinding(
	tileWinding *[TileSize][TileSize]float32,
	accumulatedWinding *[TileSize]float32,
	tileX, tileY uint16,
) {
	// Get backdrop for this tile
	var backdropVal int32
	if fr.backdrop != nil {
		idx := int(tileY)*int(fr.tileColumns) + int(tileX)
		if idx >= 0 && idx < len(fr.backdrop) {
			backdropVal = fr.backdrop[idx]
		}
	}

	// Initialize accumulated winding from backdrop
	backdropF := float32(backdropVal)
	for y := 0; y < TileSize; y++ {
		accumulatedWinding[y] = backdropF
	}

	// Clear tile winding
	for y := 0; y < TileSize; y++ {
		for x := 0; x < TileSize; x++ {
			tileWinding[y][x] = backdropF
		}
	}
}

// processSegment calculates the winding contribution of a segment to a tile.
// This is the core of the analytic anti-aliasing algorithm.
//
//nolint:dupl // Duplicated with processSegmentForStrip for performance - avoiding abstraction overhead in hot path
func (fr *FineRasterizer) processSegment(
	line LineSegment,
	tileX, tileY uint16,
	tileWinding *[TileSize][TileSize]float32,
	accumulatedWinding *[TileSize]float32,
) {
	// Convert tile coordinates to pixel coordinates
	tileLeftX := float32(tileX) * float32(TileWidth)
	tileTopY := float32(tileY) * float32(TileHeight)

	// Get line endpoints relative to tile
	p0x := line.X0 - tileLeftX
	p0y := line.Y0 - tileTopY
	p1x := line.X1 - tileLeftX
	p1y := line.Y1 - tileTopY

	// Skip horizontal segments
	if p0y == p1y {
		return
	}

	// Get sign from winding direction
	sign := float32(line.Winding)

	// Determine top/bottom points (line is monotonic, Y0 <= Y1)
	lineTopY := p0y
	lineTopX := p0x
	lineBottomY := p1y
	lineBottomX := p1x

	// Calculate slopes
	dy := lineBottomY - lineTopY
	dx := lineBottomX - lineTopX

	ySlope := dy / dx // dy/dx
	if dx == 0 {
		// Vertical line: use large value
		if lineBottomY > lineTopY {
			ySlope = 1e10
		} else {
			ySlope = -1e10
		}
	}
	xSlope := 1.0 / ySlope // dx/dy

	// Process each pixel row
	for yIdx := 0; yIdx < TileHeight; yIdx++ {
		pxTopY := float32(yIdx)
		pxBottomY := pxTopY + 1.0

		// Clamp line Y range to this pixel row
		yMin := maxf32(lineTopY, pxTopY)
		yMax := minf32(lineBottomY, pxBottomY)

		if yMin >= yMax {
			continue // Line doesn't cross this row
		}

		// Height of line segment in this row
		h := yMax - yMin

		// Accumulate winding contribution from left edge
		acc := float32(0)

		// Process each pixel column
		for xIdx := 0; xIdx < TileWidth; xIdx++ {
			pxLeftX := float32(xIdx)
			pxRightX := pxLeftX + 1.0

			// Calculate Y coordinates where line intersects pixel left and right edges
			// Using: y = lineTopY + (x - lineTopX) * ySlope
			// and:   x = lineTopX + (y - lineTopY) * xSlope
			linePxLeftY := lineTopY + (pxLeftX-lineTopX)*ySlope
			linePxRightY := lineTopY + (pxRightX-lineTopX)*ySlope

			// Clamp to pixel row bounds and line Y bounds
			linePxLeftY = clampf32(linePxLeftY, yMin, yMax)
			linePxRightY = clampf32(linePxRightY, yMin, yMax)

			// Calculate X coordinates at the clamped Y values
			linePxLeftYX := lineTopX + (linePxLeftY-lineTopY)*xSlope
			linePxRightYX := lineTopX + (linePxRightY-lineTopY)*xSlope

			// Height of line segment within this pixel's row
			pixelH := absf32(linePxRightY - linePxLeftY)

			// Trapezoidal area: the area enclosed between the line and pixel's right edge
			// This is 0.5 * height * (width1 + width2) where widths are distances from
			// line to right edge at top and bottom of segment within pixel
			area := 0.5 * pixelH * (2*pxRightX - linePxRightYX - linePxLeftYX)

			// Add area contribution plus accumulated winding from left
			tileWinding[yIdx][xIdx] += (area*sign + acc)
			acc += pixelH * sign
		}

		// Update accumulated winding for this row
		accumulatedWinding[yIdx] += h * sign
	}
}

// finalizeTile converts winding values to coverage and stores in grid.
func (fr *FineRasterizer) finalizeTile(
	tileX, tileY uint16,
	tileWinding *[TileSize][TileSize]float32,
	_ *[TileSize]float32, // accumulatedWinding - reserved for future use
) {
	tile := fr.grid.GetOrCreate(int32(tileX), int32(tileY))

	// Convert winding to coverage based on fill rule
	for y := 0; y < TileSize; y++ {
		for x := 0; x < TileSize; x++ {
			winding := tileWinding[y][x]
			var coverage float32

			switch fr.fillRule {
			case scene.FillNonZero:
				// Non-zero: coverage = |winding|, clamped to [0, 1]
				coverage = absf32(winding)
				if coverage > 1.0 {
					coverage = 1.0
				}

			case scene.FillEvenOdd:
				// Even-odd: coverage based on fractional part of winding/2
				// Normalize: take absolute, mod 2, if > 1 then 2 - value
				absWinding := absf32(winding)
				// floor(winding + 0.5) to get nearest integer
				// then coverage = |winding - 2*floor((winding + 0.5) / 2)|
				im1 := float32(int32(absWinding*0.5 + 0.5))
				coverage = absf32(absWinding - 2.0*im1)
				if coverage > 1.0 {
					coverage = 1.0
				}
			}

			// Convert to 8-bit coverage
			alpha := uint8(coverage*255.0 + 0.5)
			tile.SetCoverage(x, y, alpha)
		}
	}
}

// Grid returns the tile grid with computed coverage.
func (fr *FineRasterizer) Grid() *TileGrid {
	return fr.grid
}

// RasterizeCurves processes curve edges directly without pre-flattening.
// This is the retained mode rendering path for scene graph rendering.
//
// Unlike Rasterize() which works with pre-flattened LineSegments,
// this method accepts curve edges from EdgeBuilder and steps through
// their segments during tile processing using forward differencing.
//
// Parameters:
//   - curveBins: Map of tile coordinates (as uint64 key) to curve edge bins (from BinCurveEdges)
//
// The output is stored in the FineRasterizer's TileGrid.
func (fr *FineRasterizer) RasterizeCurves(curveBins map[uint64]*CurveTileBin) {
	fr.grid.Reset()

	if len(curveBins) == 0 {
		return
	}

	// Process each tile with curve edges
	for key, bin := range curveBins {
		// Decode tile coordinates from key
		// Key format: Y in upper 32 bits, X in lower 32 bits
		tileX := int32(key & 0xFFFFFFFF) //nolint:gosec // Safe: extracting lower 32 bits
		tileY := int32(key >> 32)        //nolint:gosec // Safe: extracting upper 32 bits

		// Skip tiles outside viewport
		if tileX < 0 || tileX >= int32(fr.tileColumns) ||
			tileY < 0 || tileY >= int32(fr.tileRows) {
			continue
		}

		//nolint:gosec // tileX, tileY validated above
		fr.processTileWithCurves(uint16(tileX), uint16(tileY), bin.Edges, bin.Backdrop)
	}
}

// processTileWithCurves computes coverage for a single tile using curve edges.
// This is the core of curve-aware fine rasterization.
func (fr *FineRasterizer) processTileWithCurves(
	tileX, tileY uint16,
	edges []raster.CurveEdgeVariant,
	backdrop int32,
) {
	if len(edges) == 0 {
		// No edges - fill with backdrop if non-zero
		if backdrop != 0 {
			tile := fr.grid.GetOrCreate(int32(tileX), int32(tileY))
			fr.fillTileWithBackdrop(tile, backdrop)
		}
		return
	}

	// Initialize winding arrays
	var tileWinding [TileSize][TileSize]float32
	var accumulatedWinding [TileSize]float32

	backdropF := float32(backdrop)
	for y := 0; y < TileSize; y++ {
		accumulatedWinding[y] = backdropF
		for x := 0; x < TileSize; x++ {
			tileWinding[y][x] = backdropF
		}
	}

	// Process each edge
	for i := range edges {
		edge := &edges[i]
		fr.processEdgeForTile(edge, tileX, tileY, &tileWinding, &accumulatedWinding)
	}

	// Convert winding to coverage and store in grid
	fr.finalizeTileFromCurves(tileX, tileY, &tileWinding)
}

// processEdgeForTile processes a single edge (line or curve) for a tile.
func (fr *FineRasterizer) processEdgeForTile(
	edge *raster.CurveEdgeVariant,
	tileX, tileY uint16,
	tileWinding *[TileSize][TileSize]float32,
	accumulatedWinding *[TileSize]float32,
) {
	switch edge.Type {
	case raster.EdgeTypeLine:
		if edge.Line != nil {
			fr.processLineEdgeForTile(edge.Line, tileX, tileY, tileWinding, accumulatedWinding)
		}

	case raster.EdgeTypeQuadratic:
		if edge.Quadratic != nil {
			fr.processQuadraticEdgeForTile(edge.Quadratic, tileX, tileY, tileWinding, accumulatedWinding)
		}

	case raster.EdgeTypeCubic:
		if edge.Cubic != nil {
			fr.processCubicEdgeForTile(edge.Cubic, tileX, tileY, tileWinding, accumulatedWinding)
		}
	}
}

// processLineEdgeForTile processes a simple line edge for a tile.
func (fr *FineRasterizer) processLineEdgeForTile(
	line *raster.LineEdge,
	tileX, tileY uint16,
	tileWinding *[TileSize][TileSize]float32,
	accumulatedWinding *[TileSize]float32,
) {
	// Convert raster.LineEdge to LineSegment for existing processSegment logic
	segment := fr.lineEdgeToSegment(line, tileX, tileY)
	if segment.Y0 != segment.Y1 { // Skip horizontal segments
		fr.processSegment(segment, tileX, tileY, tileWinding, accumulatedWinding)
	}
}

// processQuadraticEdgeForTile processes a quadratic curve edge for a tile.
// It steps through the curve segments using forward differencing.
func (fr *FineRasterizer) processQuadraticEdgeForTile(
	quad *raster.QuadraticEdge,
	tileX, tileY uint16,
	tileWinding *[TileSize][TileSize]float32,
	accumulatedWinding *[TileSize]float32,
) {
	// Process all segments from this quadratic curve
	for quad.CurveCount() > 0 {
		// Current line segment from the curve
		line := quad.Line()
		segment := fr.lineEdgeToSegment(line, tileX, tileY)
		if segment.Y0 != segment.Y1 {
			fr.processSegment(segment, tileX, tileY, tileWinding, accumulatedWinding)
		}

		// Advance to next segment using forward differencing
		if !quad.Update() {
			break
		}
	}

	// Process the final segment
	line := quad.Line()
	segment := fr.lineEdgeToSegment(line, tileX, tileY)
	if segment.Y0 != segment.Y1 {
		fr.processSegment(segment, tileX, tileY, tileWinding, accumulatedWinding)
	}
}

// processCubicEdgeForTile processes a cubic curve edge for a tile.
// It steps through the curve segments using forward differencing.
func (fr *FineRasterizer) processCubicEdgeForTile(
	cubic *raster.CubicEdge,
	tileX, tileY uint16,
	tileWinding *[TileSize][TileSize]float32,
	accumulatedWinding *[TileSize]float32,
) {
	// Cubic uses negative count, active while < 0
	for cubic.CurveCount() < 0 {
		// Current line segment from the curve
		line := cubic.Line()
		segment := fr.lineEdgeToSegment(line, tileX, tileY)
		if segment.Y0 != segment.Y1 {
			fr.processSegment(segment, tileX, tileY, tileWinding, accumulatedWinding)
		}

		// Advance to next segment using forward differencing
		if !cubic.Update() {
			break
		}
	}

	// Process the final segment
	line := cubic.Line()
	segment := fr.lineEdgeToSegment(line, tileX, tileY)
	if segment.Y0 != segment.Y1 {
		fr.processSegment(segment, tileX, tileY, tileWinding, accumulatedWinding)
	}
}

// lineEdgeToSegment converts a raster.LineEdge to a LineSegment for processSegment.
func (fr *FineRasterizer) lineEdgeToSegment(line *raster.LineEdge, _, _ uint16) LineSegment {
	// Convert raster.FDot16 coordinates to float32 pixel coordinates
	x0 := raster.FDot16ToFloat32(line.X)
	y0 := float32(line.FirstY)
	y1 := float32(line.LastY + 1)

	// Calculate X at the end using slope
	dy := y1 - y0
	dx := raster.FDot16ToFloat32(line.DX)
	x1 := x0 + dx*dy

	return LineSegment{
		X0:      x0,
		Y0:      y0,
		X1:      x1,
		Y1:      y1,
		Winding: line.Winding,
	}
}

// fillTileWithBackdrop fills a tile with solid coverage based on backdrop winding.
func (fr *FineRasterizer) fillTileWithBackdrop(tile *Tile, backdrop int32) {
	var coverage float32

	switch fr.fillRule {
	case scene.FillNonZero:
		coverage = absf32(float32(backdrop))
		if coverage > 1.0 {
			coverage = 1.0
		}
	case scene.FillEvenOdd:
		absBackdrop := backdrop
		if absBackdrop < 0 {
			absBackdrop = -absBackdrop
		}
		if absBackdrop%2 != 0 {
			coverage = 1.0
		}
	}

	if coverage > 0 {
		alpha := uint8(coverage*255.0 + 0.5)
		tile.FillSolid(alpha)
	}
}

// finalizeTileFromCurves converts winding values to coverage and stores in grid.
func (fr *FineRasterizer) finalizeTileFromCurves(
	tileX, tileY uint16,
	tileWinding *[TileSize][TileSize]float32,
) {
	tile := fr.grid.GetOrCreate(int32(tileX), int32(tileY))

	// Convert winding to coverage based on fill rule
	for y := 0; y < TileSize; y++ {
		for x := 0; x < TileSize; x++ {
			winding := tileWinding[y][x]
			var coverage float32

			switch fr.fillRule {
			case scene.FillNonZero:
				// Non-zero: coverage = |winding|, clamped to [0, 1]
				coverage = absf32(winding)
				if coverage > 1.0 {
					coverage = 1.0
				}

			case scene.FillEvenOdd:
				// Even-odd: coverage based on fractional part of winding/2
				absWinding := absf32(winding)
				im1 := float32(int32(absWinding*0.5 + 0.5))
				coverage = absf32(absWinding - 2.0*im1)
				if coverage > 1.0 {
					coverage = 1.0
				}
			}

			// Convert to 8-bit coverage
			alpha := uint8(coverage*255.0 + 0.5)
			tile.SetCoverage(x, y, alpha)
		}
	}
}

// clampf32 clamps value to [minVal, maxVal] range.
func clampf32(val, minVal, maxVal float32) float32 {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

// RenderToBuffer renders the tile grid to a pixel buffer.
// The buffer is in RGBA format with the given stride (bytes per row).
func (fr *FineRasterizer) RenderToBuffer(
	buffer []uint8,
	width, height int,
	stride int,
	color [4]uint8,
) {
	fr.grid.ForEach(func(tile *Tile) {
		baseX := int(tile.PixelX())
		baseY := int(tile.PixelY())

		for py := 0; py < TileSize; py++ {
			pixelY := baseY + py
			if pixelY < 0 || pixelY >= height {
				continue
			}

			rowOffset := pixelY * stride

			for px := 0; px < TileSize; px++ {
				pixelX := baseX + px
				if pixelX < 0 || pixelX >= width {
					continue
				}

				coverage := tile.GetCoverage(px, py)
				if coverage == 0 {
					continue
				}

				idx := rowOffset + pixelX*4

				// Premultiplied alpha blending
				srcA := uint16(coverage) * uint16(color[3]) / 255
				invA := 255 - srcA

				if idx+3 < len(buffer) {
					//nolint:gosec // Result is bounded 0-255
					buffer[idx+0] = uint8((uint16(color[0])*srcA + uint16(buffer[idx+0])*invA) / 255)
					//nolint:gosec // Result is bounded 0-255
					buffer[idx+1] = uint8((uint16(color[1])*srcA + uint16(buffer[idx+1])*invA) / 255)
					//nolint:gosec // Result is bounded 0-255
					buffer[idx+2] = uint8((uint16(color[2])*srcA + uint16(buffer[idx+2])*invA) / 255)
					//nolint:gosec // Result is bounded 0-255
					buffer[idx+3] = uint8(srcA + uint16(buffer[idx+3])*invA/255)
				}
			}
		}
	})
}

// SparseStrip represents a sparse strip for efficient rendering.
// A strip is a horizontal run of tiles with the same Y coordinate.
// This is different from the legacy Strip type in strips.go.
type SparseStrip struct {
	X        uint16 // X coordinate in pixels
	Y        uint16 // Y coordinate in pixels
	AlphaIdx uint32 // Index into alpha buffer
	FillGap  bool   // Whether to fill gap before this strip
}

// StripRenderer renders tiles as sparse strips.
type StripRenderer struct {
	strips    []SparseStrip
	alphas    []uint8
	fillRule  scene.FillStyle
	aliasMode bool // If true, use hard edges instead of anti-aliasing
}

// NewStripRenderer creates a new strip renderer.
func NewStripRenderer() *StripRenderer {
	return &StripRenderer{
		strips:   make([]SparseStrip, 0, 256),
		alphas:   make([]uint8, 0, 4096),
		fillRule: scene.FillNonZero,
	}
}

// Reset clears the renderer state for reuse.
func (sr *StripRenderer) Reset() {
	sr.strips = sr.strips[:0]
	sr.alphas = sr.alphas[:0]
}

// SetFillRule sets the fill rule.
func (sr *StripRenderer) SetFillRule(rule scene.FillStyle) {
	sr.fillRule = rule
}

// SetAliasMode enables or disables aliased (non-anti-aliased) rendering.
func (sr *StripRenderer) SetAliasMode(enabled bool) {
	sr.aliasMode = enabled
}

// Strips returns the strips.
func (sr *StripRenderer) Strips() []SparseStrip {
	return sr.strips
}

// Alphas returns the alpha buffer.
func (sr *StripRenderer) Alphas() []uint8 {
	return sr.alphas
}

// RenderTiles converts tiles to strips for efficient rendering.
//
//nolint:gocognit,gocyclo,cyclop,nestif // Complexity is inherent to rendering algorithm - function is cohesive
func (sr *StripRenderer) RenderTiles(
	coarse *CoarseRasterizer,
	segments *SegmentList,
	backdrop []int32,
) {
	sr.Reset()

	if segments == nil || len(coarse.Entries()) == 0 {
		return
	}

	entries := coarse.Entries()
	lines := segments.Segments()
	coarse.SortEntries()

	var currentX, currentY uint16 = 0xFFFF, 0xFFFF
	var tileWinding [TileSize][TileSize]float32
	var accumulatedWinding [TileSize]float32
	var windingDelta int32
	var prevTile CoarseTileEntry

	// shouldFill determines if a gap should be filled based on winding
	shouldFill := func(winding int32) bool {
		switch sr.fillRule {
		case scene.FillNonZero:
			return winding != 0
		case scene.FillEvenOdd:
			return winding%2 != 0
		default:
			return winding != 0
		}
	}

	for i, entry := range entries {
		// Check for new tile location
		sameLocation := entry.X == currentX && entry.Y == currentY
		prevLocation := (i > 0) && entry.Y == prevTile.Y && entry.X == prevTile.X+1

		if !sameLocation {
			// Finalize previous tile
			if currentX != 0xFFFF {
				sr.finalizeTileToStrip(currentX, currentY, &tileWinding, &accumulatedWinding)
			}

			// Handle strip boundaries
			if currentX != 0xFFFF && !prevLocation && prevTile.X != 0xFFFF {
				// End current strip and potentially start new one
				if prevTile.Y != entry.Y {
					// New row - emit sentinel and reset winding
					if windingDelta != 0 || i == len(entries)-1 {
						sr.emitSentinel(prevTile.Y, shouldFill(windingDelta))
					}
					windingDelta = 0
					accumulatedWinding = [TileSize]float32{}
				}

				// Start new strip
				sr.startStrip(entry.X, entry.Y, shouldFill(windingDelta))
			} else if currentX == 0xFFFF {
				// First strip
				sr.startStrip(entry.X, entry.Y, false)
			}

			// Initialize new tile
			currentX, currentY = entry.X, entry.Y
			sr.initTileWindingForStrip(&tileWinding, &accumulatedWinding, backdrop, currentX, currentY, int(coarse.TileColumns()))
			prevTile = entry
		}

		// Process segment contribution
		if int(entry.LineIdx) < len(lines) {
			line := lines[entry.LineIdx]
			sr.processSegmentForStrip(line, currentX, currentY, &tileWinding, &accumulatedWinding)

			// Update winding delta
			if entry.Winding {
				windingDelta += int32(line.Winding)
			}
		}
	}

	// Finalize last tile
	if currentX != 0xFFFF {
		sr.finalizeTileToStrip(currentX, currentY, &tileWinding, &accumulatedWinding)
		// Emit final sentinel
		sr.emitSentinel(currentY, shouldFill(windingDelta))
	}
}

// startStrip begins a new strip at the given location.
func (sr *StripRenderer) startStrip(x, y uint16, fillGap bool) {
	sr.strips = append(sr.strips, SparseStrip{
		X:        x * TileWidth,
		Y:        y * TileHeight,
		AlphaIdx: uint32(len(sr.alphas)), //nolint:gosec // alphas length bounded by viewport
		FillGap:  fillGap,
	})
}

// emitSentinel emits a sentinel strip marking end of row.
func (sr *StripRenderer) emitSentinel(y uint16, fillGap bool) {
	sr.strips = append(sr.strips, SparseStrip{
		X:        0xFFFF,
		Y:        y * TileHeight,
		AlphaIdx: uint32(len(sr.alphas)), //nolint:gosec // alphas length bounded by viewport
		FillGap:  fillGap,
	})
}

// initTileWindingForStrip initializes winding for strip rendering.
func (sr *StripRenderer) initTileWindingForStrip(
	tileWinding *[TileSize][TileSize]float32,
	accumulatedWinding *[TileSize]float32,
	backdrop []int32,
	tileX, tileY uint16,
	tileColumns int,
) {
	var backdropVal int32
	if backdrop != nil {
		idx := int(tileY)*tileColumns + int(tileX)
		if idx >= 0 && idx < len(backdrop) {
			backdropVal = backdrop[idx]
		}
	}

	backdropF := float32(backdropVal)
	for y := 0; y < TileSize; y++ {
		accumulatedWinding[y] = backdropF
		for x := 0; x < TileSize; x++ {
			tileWinding[y][x] = backdropF
		}
	}
}

// processSegmentForStrip processes a segment for strip rendering.
//
//nolint:dupl // Duplicated with processSegment for performance - avoiding abstraction overhead in hot path
func (sr *StripRenderer) processSegmentForStrip(
	line LineSegment,
	tileX, tileY uint16,
	tileWinding *[TileSize][TileSize]float32,
	accumulatedWinding *[TileSize]float32,
) {
	tileLeftX := float32(tileX) * float32(TileWidth)
	tileTopY := float32(tileY) * float32(TileHeight)

	p0x := line.X0 - tileLeftX
	p0y := line.Y0 - tileTopY
	p1x := line.X1 - tileLeftX
	p1y := line.Y1 - tileTopY

	if p0y == p1y {
		return
	}

	sign := float32(line.Winding)

	lineTopY := p0y
	lineTopX := p0x
	lineBottomY := p1y
	lineBottomX := p1x

	dy := lineBottomY - lineTopY
	dx := lineBottomX - lineTopX

	var ySlope float32
	if dx == 0 {
		if lineBottomY > lineTopY {
			ySlope = 1e10
		} else {
			ySlope = -1e10
		}
	} else {
		ySlope = dy / dx
	}
	xSlope := 1.0 / ySlope

	for yIdx := 0; yIdx < TileHeight; yIdx++ {
		pxTopY := float32(yIdx)
		pxBottomY := pxTopY + 1.0

		yMin := maxf32(lineTopY, pxTopY)
		yMax := minf32(lineBottomY, pxBottomY)

		if yMin >= yMax {
			continue
		}

		h := yMax - yMin
		acc := float32(0)

		for xIdx := 0; xIdx < TileWidth; xIdx++ {
			pxLeftX := float32(xIdx)
			pxRightX := pxLeftX + 1.0

			linePxLeftY := lineTopY + (pxLeftX-lineTopX)*ySlope
			linePxRightY := lineTopY + (pxRightX-lineTopX)*ySlope

			linePxLeftY = clampf32(linePxLeftY, yMin, yMax)
			linePxRightY = clampf32(linePxRightY, yMin, yMax)

			linePxLeftYX := lineTopX + (linePxLeftY-lineTopY)*xSlope
			linePxRightYX := lineTopX + (linePxRightY-lineTopY)*xSlope

			pixelH := absf32(linePxRightY - linePxLeftY)
			area := 0.5 * pixelH * (2*pxRightX - linePxRightYX - linePxLeftYX)

			tileWinding[yIdx][xIdx] += (area*sign + acc)
			acc += pixelH * sign
		}

		accumulatedWinding[yIdx] += h * sign
	}
}

// finalizeTileToStrip converts winding to alpha and appends to buffer.
func (sr *StripRenderer) finalizeTileToStrip(
	_, _ uint16, // tileX, tileY - unused, kept for API consistency
	tileWinding *[TileSize][TileSize]float32,
	_ *[TileSize]float32, // accumulatedWinding - reserved for future use
) {
	// Convert winding to alpha for each column
	for x := 0; x < TileWidth; x++ {
		for y := 0; y < TileHeight; y++ {
			winding := tileWinding[y][x]
			var coverage float32

			switch sr.fillRule {
			case scene.FillNonZero:
				coverage = absf32(winding)
				if coverage > 1.0 {
					coverage = 1.0
				}
			case scene.FillEvenOdd:
				absWinding := absf32(winding)
				im1 := float32(int32(absWinding*0.5 + 0.5))
				coverage = absf32(absWinding - 2.0*im1)
				if coverage > 1.0 {
					coverage = 1.0
				}
			}

			// Apply aliasing threshold if enabled
			if sr.aliasMode {
				if coverage >= 0.5 {
					coverage = 1.0
				} else {
					coverage = 0.0
				}
			}

			alpha := uint8(coverage*255.0 + 0.5)
			sr.alphas = append(sr.alphas, alpha)
		}
	}
}
