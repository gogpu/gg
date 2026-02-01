// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Package native provides CPU-based rendering with Vello-style analytic AA.
//
// This file implements tile-based rendering following the Vello architecture:
// - 4x4 pixel tiles (using TileSize from tile.go)
// - Backdrop accumulation for correct fill
// - Analytic coverage computation per pixel

package native

import "fmt"

// VelloSegment represents a line segment clipped to a tile.
// Coordinates are relative to tile origin (0 to TileSize).
type VelloSegment struct {
	X0, Y0 float32 // Start point (tile-relative)
	X1, Y1 float32 // End point (tile-relative)
	YEdge  float32 // Y coordinate where segment crosses left tile edge (x=0), or 1e9 if none
}

const noYEdge = float32(1e9)

// VelloTile represents a 4x4 pixel tile with its segments and backdrop.
type VelloTile struct {
	X, Y     int            // Tile position in tile coordinates
	Backdrop int            // Accumulated winding from tiles to the left
	Segments []VelloSegment // Segments intersecting this tile
}

// TileRasterizer implements Vello-style tile-based analytic AA.
type TileRasterizer struct {
	width, height  int
	tilesX, tilesY int
	tiles          []VelloTile
	alphaRuns      *AlphaRuns
	rowCoverage    []float32 // Coverage buffer for current row of tiles
}

// NewTileRasterizer creates a new tile-based rasterizer.
func NewTileRasterizer(width, height int) *TileRasterizer {
	tilesX := (width + TileSize - 1) / TileSize
	tilesY := (height + TileSize - 1) / TileSize

	return &TileRasterizer{
		width:       width,
		height:      height,
		tilesX:      tilesX,
		tilesY:      tilesY,
		tiles:       make([]VelloTile, tilesX*tilesY),
		alphaRuns:   NewAlphaRuns(width),
		rowCoverage: make([]float32, width),
	}
}

// Reset clears the rasterizer state.
func (tr *TileRasterizer) Reset() {
	for i := range tr.tiles {
		tr.tiles[i].Segments = tr.tiles[i].Segments[:0]
		tr.tiles[i].Backdrop = 0
	}
	tr.alphaRuns.Reset()
}

// debugTileRasterizer enables debug output.
var debugTileRasterizer = false
var debugTileY = -1
var debugVerbose = false

// SetDebugTileRasterizer enables debug output for tile rasterizer.
func SetDebugTileRasterizer(enable bool, y int) {
	debugTileRasterizer = enable
	debugTileY = y
}

// SetDebugTileRasterizerVerbose enables verbose debug output.
func SetDebugTileRasterizerVerbose(enable bool) {
	debugVerbose = enable
}

// Fill renders a path using tile-based analytic AA.
func (tr *TileRasterizer) Fill(
	eb *EdgeBuilder,
	fillRule FillRule,
	callback func(y int, runs *AlphaRuns),
) {
	if eb.IsEmpty() {
		return
	}

	// Reset tiles
	tr.Reset()

	// Get AA scale
	aaShift := eb.AAShift()
	//nolint:gosec // aaShift is bounded by EdgeBuilder
	aaScale := float32(int32(1) << uint(aaShift))

	// 1. Bin segments to tiles
	tr.binSegments(eb, aaScale)

	// 2. Compute backdrop for each tile row
	tr.computeBackdrops()

	// 3. Rasterize each tile row and emit scanlines
	for tileY := 0; tileY < tr.tilesY; tileY++ {
		tr.rasterizeTileRow(tileY, fillRule, callback)
	}
}

// binSegments distributes edge segments to tiles they intersect.
// Implements Vello path_tiling.wgsl algorithm with correct y_edge and backdrop.
//
//nolint:gocognit,gocyclo,cyclop,funlen // Direct port of Vello algorithm, complexity is inherent
func (tr *TileRasterizer) binSegments(eb *EdgeBuilder, aaScale float32) {
	// Track backdrop deltas separately - will accumulate in computeBackdrops
	backdropDeltas := make([]int, tr.tilesX*tr.tilesY)

	edgeIdx := 0
	for edge := range eb.AllEdges() {
		line := edge.AsLine()
		if line == nil {
			continue
		}

		// Convert to pixel coordinates
		x0 := FDot16ToFloat32(line.X) / aaScale
		y0 := float32(line.FirstY) / aaScale
		y1 := float32(line.LastY+1) / aaScale
		dx := FDot16ToFloat32(line.DX)
		x1 := x0 + dx*(y1-y0)

		if debugVerbose {
			fmt.Printf("Edge[%d]: (%.2f,%.2f)-(%.2f,%.2f) winding=%d\n",
				edgeIdx, x0, y0, x1, y1, line.Winding)
		}
		edgeIdx++

		// Winding direction: +1 for downward (y increasing), -1 for upward
		winding := int32(1)
		if line.Winding < 0 {
			winding = -1
		}

		// Find tile range
		xMin, xMax := x0, x1
		if xMin > xMax {
			xMin, xMax = xMax, xMin
		}
		yMin, yMax := y0, y1
		if yMin > yMax {
			yMin, yMax = yMax, yMin
		}

		tileXMin := int(xMin) / TileSize
		tileXMax := int(xMax) / TileSize
		tileYMin := int(yMin) / TileSize
		tileYMax := int(yMax) / TileSize

		// Clamp to valid range
		if tileXMin < 0 {
			tileXMin = 0
		}
		if tileXMax >= tr.tilesX {
			tileXMax = tr.tilesX - 1
		}
		if tileYMin < 0 {
			tileYMin = 0
		}
		if tileYMax >= tr.tilesY {
			tileYMax = tr.tilesY - 1
		}

		// Process each tile row
		for ty := tileYMin; ty <= tileYMax; ty++ {
			tileTopY := float32(ty * TileSize)

			// BACKDROP: Add delta when segment crosses TOP EDGE of tile row
			// This is the key fix - backdrop affects ALL tiles to the right
			if y0 < tileTopY && y1 >= tileTopY {
				// Segment crosses top of this tile row (going down)
				// Find X coordinate at the crossing
				t := (tileTopY - y0) / (y1 - y0)
				xCross := x0 + t*(x1-x0)
				tileCross := int(xCross) / TileSize
				if tileCross < 0 {
					tileCross = 0
				}
				// Add delta to all tiles RIGHT of crossing point
				for tx := tileCross; tx < tr.tilesX; tx++ {
					backdropDeltas[ty*tr.tilesX+tx] += int(winding)
				}
			} else if y1 < tileTopY && y0 >= tileTopY {
				// Segment crosses top of this tile row (going up)
				t := (tileTopY - y1) / (y0 - y1)
				xCross := x1 + t*(x0-x1)
				tileCross := int(xCross) / TileSize
				if tileCross < 0 {
					tileCross = 0
				}
				// Subtract delta (upward = negative winding contribution)
				for tx := tileCross; tx < tr.tilesX; tx++ {
					backdropDeltas[ty*tr.tilesX+tx] -= int(winding)
				}
			}

			// Add segment to each tile it intersects in this row
			for tx := tileXMin; tx <= tileXMax; tx++ {
				tileIdx := ty*tr.tilesX + tx
				tileX := float32(tx * TileSize)

				// Convert to tile-relative coordinates
				relX0 := x0 - tileX
				relY0 := y0 - tileTopY
				relX1 := x1 - tileX
				relY1 := y1 - tileTopY

				// y_edge: Y coordinate where segment TOUCHES x=0 (left tile edge)
				// KEY FIX: Only record when segment endpoint is AT x=0, not when crossing
				// This follows Vello path_tiling.wgsl logic exactly
				yEdge := noYEdge
				const epsilon = 1e-6

				// Apply Vello's numerical robustness logic
				//nolint:nestif,gocritic // Direct port of Vello path_tiling.wgsl
				if abs32(relX0) < epsilon {
					// Segment starts at (or very close to) left edge
					if abs32(relX1) < epsilon {
						// Vertical segment on left edge - shift slightly to avoid singularity
						relX0 = epsilon
					} else if abs32(relY0) < epsilon {
						// Starts at top-left corner - shift to avoid singularity
						relX0 = epsilon
					} else if relY0 > 0 && relY0 < float32(TileSize) {
						// Starts on left edge, not at corner - record y_edge
						yEdge = relY0
					}
				} else if abs32(relX1) < epsilon {
					// Segment ends at (or very close to) left edge
					if abs32(relY1) < epsilon {
						// Ends at top-left corner - shift to avoid singularity
						relX1 = epsilon
					} else if relY1 > 0 && relY1 < float32(TileSize) {
						// Ends on left edge, not at corner - record y_edge
						yEdge = relY1
					}
				}

				seg := VelloSegment{
					X0:    relX0,
					Y0:    relY0,
					X1:    relX1,
					Y1:    relY1,
					YEdge: yEdge,
				}
				tr.tiles[tileIdx].Segments = append(tr.tiles[tileIdx].Segments, seg)
				tr.tiles[tileIdx].X = tx
				tr.tiles[tileIdx].Y = ty
			}
		}
	}

	// Copy backdrop deltas to tiles
	for i := range tr.tiles {
		tr.tiles[i].Backdrop = backdropDeltas[i]
	}
}

// computeBackdrops is now a no-op since binSegments directly computes
// cumulative backdrop values by adding to all tiles to the right of crossings.
// Kept for API compatibility and potential future optimizations.
func (tr *TileRasterizer) computeBackdrops() {
	// Backdrops are already cumulative from binSegments - nothing to do
	// Debug output if needed
	if debugVerbose {
		for ty := 0; ty < tr.tilesY; ty++ {
			for tx := 0; tx < tr.tilesX; tx++ {
				tileIdx := ty*tr.tilesX + tx
				if tr.tiles[tileIdx].Backdrop != 0 {
					fmt.Printf("Tile(%d,%d) backdrop=%d\n", tx, ty, tr.tiles[tileIdx].Backdrop)
				}
			}
		}
	}
}

// rasterizeTileRow processes one row of tiles and emits scanlines.
func (tr *TileRasterizer) rasterizeTileRow(tileY int, fillRule FillRule, callback func(y int, runs *AlphaRuns)) {
	// Process each scanline within the tile row
	for localY := 0; localY < TileSize; localY++ {
		pixelY := tileY*TileSize + localY
		if pixelY >= tr.height {
			break
		}

		// Clear coverage buffer
		for i := range tr.rowCoverage {
			tr.rowCoverage[i] = 0
		}

		// Process each tile in the row
		for tileX := 0; tileX < tr.tilesX; tileX++ {
			tileIdx := tileY*tr.tilesX + tileX
			tile := &tr.tiles[tileIdx]

			// Rasterize this tile for this scanline
			tr.rasterizeTileScanline(tile, localY, tileX)
		}

		// Convert coverage to alpha runs
		tr.coverageToRuns(pixelY, fillRule, callback)
	}
}

// rasterizeTileScanline computes coverage for one scanline within a tile.
// Implements Vello's fine rasterizer algorithm with analytic coverage.
func (tr *TileRasterizer) rasterizeTileScanline(tile *VelloTile, localY int, tileX int) {
	yf := float32(localY)
	baseX := tileX * TileSize
	pixelY := tile.Y*TileSize + localY

	// Initialize with backdrop (accumulated winding from tiles to the left)
	backdrop := float32(tile.Backdrop)
	for i := 0; i < TileSize && baseX+i < tr.width; i++ {
		tr.rowCoverage[baseX+i] = backdrop
	}

	if debugTileRasterizer && (debugTileY < 0 || pixelY == debugTileY) && len(tile.Segments) > 0 {
		fmt.Printf("Tile(%d,%d) Y=%d: backdrop=%d, %d segments\n",
			tile.X, tile.Y, pixelY, tile.Backdrop, len(tile.Segments))
		for i, seg := range tile.Segments {
			fmt.Printf("  seg[%d]: (%.2f,%.2f)-(%.2f,%.2f) yEdge=%.2f\n",
				i, seg.X0, seg.Y0, seg.X1, seg.Y1, seg.YEdge)
		}
	}

	// Process each segment
	for _, seg := range tile.Segments {
		delta := [2]float32{seg.X1 - seg.X0, seg.Y1 - seg.Y0}

		// Y relative to this scanline within tile
		relY := seg.Y0 - yf

		// Clamp Y range to [0, 1] (one pixel row)
		y0 := clamp32(relY, 0, 1)
		y1 := clamp32(relY+delta[1], 0, 1)
		dy := y0 - y1

		// y_edge contribution for winding propagation
		// This is applied to ALL pixels, not just those where segment has passed
		// Formula: sign(delta.x) * clamp(y - y_edge + 1, 0, 1)
		var yEdgeContrib float32
		if seg.YEdge < noYEdge { // Only if segment crosses left tile edge
			if delta[0] > 0 {
				yEdgeContrib = clamp32(yf-seg.YEdge+1.0, 0, 1)
			} else if delta[0] < 0 {
				yEdgeContrib = -clamp32(yf-seg.YEdge+1.0, 0, 1)
			}
			// Apply y_edge contribution to ALL pixels in tile
			for i := 0; i < TileSize && baseX+i < tr.width; i++ {
				tr.rowCoverage[baseX+i] += yEdgeContrib
			}
		}

		if dy != 0 {
			// Calculate intersection parameters
			vecYRecip := 1.0 / delta[1]
			t0 := (y0 - relY) * vecYRecip
			t1 := (y1 - relY) * vecYRecip

			x0 := seg.X0 + t0*delta[0]
			x1 := seg.X0 + t1*delta[0]

			xmin0 := min32f(x0, x1)
			xmax0 := max32f(x0, x1)

			// Process each pixel in tile
			for i := 0; i < TileSize && baseX+i < tr.width; i++ {
				iF := float32(i)

				xmin := min32f(xmin0-iF, 1.0) - 1.0e-6
				xmax := xmax0 - iF

				// Vello coverage formula: computes fraction of pixel to the left of segment
				b := min32f(xmax, 1.0)
				c := max32f(b, 0.0)
				d := max32f(xmin, 0.0)

				denom := xmax - xmin
				var a float32
				if denom > 1e-6 || denom < -1e-6 {
					a = (b + 0.5*(d*d-c*c) - xmin) / denom
				}

				tr.rowCoverage[baseX+i] += a * dy
			}
		}
	}
}

// coverageToRuns converts coverage buffer to alpha runs.
func (tr *TileRasterizer) coverageToRuns(y int, fillRule FillRule, callback func(y int, runs *AlphaRuns)) {
	tr.alphaRuns.Reset()

	var currentAlpha uint8
	runStart := 0

	for i := 0; i < tr.width; i++ {
		w := tr.rowCoverage[i]

		var coverage float32
		switch fillRule {
		case FillRuleNonZero:
			if w < 0 {
				w = -w
			}
			coverage = clamp32(w, 0, 1)
		case FillRuleEvenOdd:
			if w < 0 {
				w = -w
			}
			// Even-odd: abs(w - 2 * round(0.5 * w))
			im1 := float32(int32(w*0.5 + 0.5))
			coverage = clamp32(abs32(w-2.0*im1), 0, 1)
		}

		alpha := uint8(coverage * 255.0)

		if i == 0 {
			currentAlpha = alpha
			runStart = 0
			continue
		}

		if alpha != currentAlpha {
			if currentAlpha > 0 {
				runLen := i - runStart
				tr.alphaRuns.Add(runStart, currentAlpha, runLen-1, 0)
			}
			currentAlpha = alpha
			runStart = i
		}
	}

	if currentAlpha > 0 {
		runLen := tr.width - runStart
		tr.alphaRuns.Add(runStart, currentAlpha, runLen-1, 0)
	}

	callback(y, tr.alphaRuns)
}

// abs32 returns absolute value of float32.
func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
