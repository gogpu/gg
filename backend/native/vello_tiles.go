// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Package native provides CPU-based rendering with Vello-style analytic AA.
//
// This file is a 1:1 port of Vello's CPU fine rasterizer from:
// - vello_shaders/src/cpu/fine.rs (fill_path function)
// - vello_shaders/src/cpu/path_tiling.rs (segment binning with y_edge)
// - vello_shaders/src/cpu/path_count.rs (backdrop computation)
// - vello_shaders/src/cpu/backdrop.rs (backdrop prefix sum)

package native

import "math"

// TileWidth and TileHeight match Vello's tile dimensions.
// Using 16x16 tiles as in the original Vello implementation.
const (
	VelloTileWidth  = 16
	VelloTileHeight = 16
	VelloTileSize   = VelloTileWidth * VelloTileHeight
)

// PathSegment is a direct port of Vello's PathSegment struct.
// Coordinates are relative to tile origin (0..TileWidth, 0..TileHeight).
type PathSegment struct {
	Point0 [2]float32 // Start point (tile-relative)
	Point1 [2]float32 // End point (tile-relative)
	YEdge  float32    // Y coordinate where segment touches x=0 (left edge), or 1e9 if none
}

// VelloTile represents a tile with its segments and backdrop.
type VelloTile struct {
	Backdrop int           // Winding number from tiles to the left
	Segments []PathSegment // Segments in this tile
}

// TileRasterizer implements Vello-style tile-based analytic AA.
type TileRasterizer struct {
	width, height  int
	tilesX, tilesY int
	tiles          []VelloTile
	area           []float32  // Per-tile pixel area buffer (TileSize elements)
	rowCoverage    []float32  // Full scanline coverage buffer
	alphaRuns      *AlphaRuns // Output alpha runs
}

// NewTileRasterizer creates a new Vello-style tile rasterizer.
func NewTileRasterizer(width, height int) *TileRasterizer {
	tilesX := (width + VelloTileWidth - 1) / VelloTileWidth
	tilesY := (height + VelloTileHeight - 1) / VelloTileHeight

	return &TileRasterizer{
		width:       width,
		height:      height,
		tilesX:      tilesX,
		tilesY:      tilesY,
		tiles:       make([]VelloTile, tilesX*tilesY),
		area:        make([]float32, VelloTileSize),
		rowCoverage: make([]float32, width),
		alphaRuns:   NewAlphaRuns(width),
	}
}

// Reset clears the rasterizer for reuse.
func (tr *TileRasterizer) Reset() {
	for i := range tr.tiles {
		tr.tiles[i].Segments = tr.tiles[i].Segments[:0]
		tr.tiles[i].Backdrop = 0
	}
}

// Fill renders a path using Vello's tile-based algorithm.
func (tr *TileRasterizer) Fill(
	eb *EdgeBuilder,
	fillRule FillRule,
	callback func(y int, runs *AlphaRuns),
) {
	if eb.IsEmpty() {
		return
	}

	tr.Reset()

	// Get AA scale
	aaShift := eb.AAShift()
	//nolint:gosec // aaShift is bounded
	aaScale := float32(int32(1) << uint(aaShift))

	// 1. Bin segments to tiles (port of path_count.rs + path_tiling.rs)
	tr.binSegments(eb, aaScale)

	// 2. Prefix sum for backdrop (port of backdrop.rs)
	tr.computeBackdropPrefixSum()

	// 3. Rasterize row by row
	for tileY := 0; tileY < tr.tilesY; tileY++ {
		// Process each scanline within this tile row
		for localY := 0; localY < VelloTileHeight; localY++ {
			pixelY := tileY*VelloTileHeight + localY
			if pixelY >= tr.height {
				break
			}

			// Clear row coverage
			for i := range tr.rowCoverage {
				tr.rowCoverage[i] = 0
			}

			// Process all tiles in this row
			for tileX := 0; tileX < tr.tilesX; tileX++ {
				tileIdx := tileY*tr.tilesX + tileX
				tile := &tr.tiles[tileIdx]

				// Fill this tile's area buffer for this scanline
				tr.fillTileScanline(tile, localY, fillRule)

				// Copy to row coverage
				baseX := tileX * VelloTileWidth
				for i := 0; i < VelloTileWidth && baseX+i < tr.width; i++ {
					tr.rowCoverage[baseX+i] = tr.area[i]
				}
			}

			// Convert to alpha runs and emit
			tr.emitScanline(pixelY, callback)
		}
	}
}

// computeBackdropPrefixSum applies prefix sum to backdrop values.
// Direct port of backdrop.rs - for each row, sum backdrops from left to right.
func (tr *TileRasterizer) computeBackdropPrefixSum() {
	for ty := 0; ty < tr.tilesY; ty++ {
		sum := 0
		for tx := 0; tx < tr.tilesX; tx++ {
			idx := ty*tr.tilesX + tx
			sum += tr.tiles[idx].Backdrop
			tr.tiles[idx].Backdrop = sum
		}
	}
}

// binSegments distributes edge segments to tiles.
// Port of path_count.rs (backdrop) + path_tiling.rs (segments with y_edge).
//
//nolint:gocognit,gocyclo,cyclop,funlen,maintidx // Direct port of Vello algorithm
func (tr *TileRasterizer) binSegments(eb *EdgeBuilder, aaScale float32) {
	const epsilon float32 = 1e-6
	const noYEdge float32 = 1e9

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

		// Direct port from path_count.rs line 85:
		// let delta = if is_down { -1 } else { 1 };
		// is_down means original segment goes down (increasing Y)
		// EdgeBuilder normalizes edges so FirstY <= LastY, but stores original direction in Winding
		// Winding > 0 means segment originally went DOWN
		// Winding < 0 means segment originally went UP
		isDown := line.Winding > 0
		delta := 1
		if isDown {
			delta = -1
		}

		// Edges are already normalized by EdgeBuilder (y0 < y1)
		// No swap needed

		// Find tile range
		xMin, xMax := x0, x1
		if xMin > xMax {
			xMin, xMax = xMax, xMin
		}

		tileYMin := int(y0) / VelloTileHeight
		tileYMax := int(y1) / VelloTileHeight
		tileXMin := int(xMin) / VelloTileWidth
		tileXMax := int(xMax) / VelloTileWidth

		// Clamp to valid range
		if tileYMin < 0 {
			tileYMin = 0
		}
		if tileYMax >= tr.tilesY {
			tileYMax = tr.tilesY - 1
		}
		if tileXMin < 0 {
			tileXMin = 0
		}
		if tileXMax >= tr.tilesX {
			tileXMax = tr.tilesX - 1
		}

		// Port of path_count.rs lines 127-130:
		// If segment is entirely to the left of image, add backdrop to first tile of each row
		if xMax < 0 {
			yMinTile := int(float32(math.Ceil(float64(y0)))) / VelloTileHeight
			yMaxTile := int(float32(math.Ceil(float64(y1)))) / VelloTileHeight
			for ty := yMinTile; ty < yMaxTile && ty < tr.tilesY; ty++ {
				if ty >= 0 {
					tr.tiles[ty*tr.tilesX].Backdrop += delta
				}
			}
		}

		// Process each tile the segment might intersect
		for ty := tileYMin; ty <= tileYMax; ty++ {
			tileTopY := float32(ty * VelloTileHeight)
			tileBotY := float32((ty + 1) * VelloTileHeight)

			// Port of path_count.rs lines 140-144:
			// top_edge: segment enters tile row from top
			// When this happens, add backdrop to the tile to the RIGHT of crossing point
			//
			// A segment enters from top if:
			// - First row: segment starts at or above tile top
			// - Other rows: segment came from above (always true if segment spans multiple rows)
			var xAtTop float32
			if y0 <= tileTopY {
				// Segment starts above or at tile top - compute X at tileTopY
				if y0 == tileTopY {
					xAtTop = x0
				} else {
					t := (tileTopY - y0) / (y1 - y0)
					xAtTop = x0 + t*(x1-x0)
				}
				// Add backdrop to tile to the right of crossing point
				txCross := int(xAtTop) / VelloTileWidth
				if txCross >= 0 && txCross+1 < tr.tilesX {
					tr.tiles[ty*tr.tilesX+txCross+1].Backdrop += delta
				}
			}

			// Clip segment to tile's Y range
			segY0 := y0
			segY1 := y1
			segX0 := x0
			segX1 := x1

			if segY0 < tileTopY {
				t := (tileTopY - y0) / (y1 - y0)
				segX0 = x0 + t*(x1-x0)
				segY0 = tileTopY
			}
			if segY1 > tileBotY {
				t := (tileBotY - y0) / (y1 - y0)
				segX1 = x0 + t*(x1-x0)
				segY1 = tileBotY
			}

			// Skip if segment doesn't intersect this tile row
			if segY0 >= segY1 {
				continue
			}

			for tx := tileXMin; tx <= tileXMax; tx++ {
				tileLeftX := float32(tx * VelloTileWidth)
				tileRightX := float32((tx + 1) * VelloTileWidth)

				// Check if segment intersects this tile's X range
				segXMin := min32f(segX0, segX1)
				segXMax := max32f(segX0, segX1)
				if segXMax < tileLeftX || segXMin > tileRightX {
					continue
				}

				// Clip segment to tile's X range (critical for y_edge!)
				// This follows Vello's path_tiling.rs clipping logic
				clipX0, clipY0 := segX0, segY0
				clipX1, clipY1 := segX1, segY1
				isPositiveSlope := segX1 >= segX0

				// Clip start point if outside tile
				if clipX0 < tileLeftX {
					// Segment enters from left
					t := (tileLeftX - segX0) / (segX1 - segX0)
					clipX0 = tileLeftX
					clipY0 = segY0 + t*(segY1-segY0)
				} else if clipX0 > tileRightX {
					// Segment enters from right
					t := (tileRightX - segX0) / (segX1 - segX0)
					clipX0 = tileRightX
					clipY0 = segY0 + t*(segY1-segY0)
				}

				// Clip end point if outside tile
				if clipX1 < tileLeftX {
					// Segment exits from left
					t := (tileLeftX - segX0) / (segX1 - segX0)
					clipX1 = tileLeftX
					clipY1 = segY0 + t*(segY1-segY0)
				} else if clipX1 > tileRightX {
					// Segment exits from right
					t := (tileRightX - segX0) / (segX1 - segX0)
					clipX1 = tileRightX
					clipY1 = segY0 + t*(segY1-segY0)
				}

				// Skip if segment becomes degenerate
				if clipY0 >= clipY1 || (clipX0 == clipX1 && clipY0 == clipY1) {
					continue
				}

				// Convert to tile-relative coordinates
				p0x := clipX0 - tileLeftX
				p0y := clipY0 - tileTopY
				p1x := clipX1 - tileLeftX
				p1y := clipY1 - tileTopY

				// Compute y_edge: Y coordinate where segment touches left tile edge (x=0)
				// Direct port of path_tiling.rs lines 116-144
				// y_edge is set when segment STARTS or ENDS at x=0 (left tile edge)
				yEdge := noYEdge
				_ = isPositiveSlope // Used for understanding, may be needed later

				//nolint:nestif,gocritic // Direct port
				if p0x == 0.0 {
					if p1x == 0.0 {
						// Both endpoints on left edge - make segment disappear
						p0x = epsilon
						if p0y == 0.0 {
							p1x = epsilon
							p1y = float32(VelloTileHeight)
						} else {
							p1x = 2.0 * epsilon
							p1y = p0y
						}
					} else if p0y == 0.0 {
						p0x = epsilon
					} else {
						yEdge = p0y
					}
				} else if p1x == 0.0 {
					if p1y == 0.0 {
						p1x = epsilon
					} else {
						yEdge = p1y
					}
				}

				// Handle pixel boundary
				if p0x == float32(int(p0x)) && p0x != 0.0 {
					p0x -= epsilon
				}
				if p1x == float32(int(p1x)) && p1x != 0.0 {
					p1x -= epsilon
				}

				// Restore original direction if needed
				if !isDown {
					p0x, p1x = p1x, p0x
					p0y, p1y = p1y, p0y
				}

				// Add segment to tile
				tileIdx := ty*tr.tilesX + tx
				tr.tiles[tileIdx].Segments = append(tr.tiles[tileIdx].Segments, PathSegment{
					Point0: [2]float32{p0x, p0y},
					Point1: [2]float32{p1x, p1y},
					YEdge:  yEdge,
				})
			}
		}
	}
}

// fillTileScanline computes coverage for one scanline within a tile.
// Direct port of fine.rs fill_path function for a single row.
func (tr *TileRasterizer) fillTileScanline(tile *VelloTile, localY int, fillRule FillRule) {
	// Initialize area with backdrop (only for first VelloTileWidth elements)
	backdropF := float32(tile.Backdrop)
	for i := 0; i < VelloTileWidth; i++ {
		tr.area[i] = backdropF
	}

	yf := float32(localY)

	// Process each segment
	for _, seg := range tile.Segments {
		delta := [2]float32{
			seg.Point1[0] - seg.Point0[0],
			seg.Point1[1] - seg.Point0[1],
		}

		// y relative to segment start within this row
		y := seg.Point0[1] - yf
		y0 := clamp32(y, 0, 1)
		y1 := clamp32(y+delta[1], 0, 1)
		dy := y0 - y1

		// y_edge contribution: signum(delta.x) * clamp(yi - y_edge + 1, 0, 1)
		var yEdge float32
		if delta[0] > 0 {
			yEdge = clamp32(yf-seg.YEdge+1.0, 0, 1)
		} else if delta[0] < 0 {
			yEdge = -clamp32(yf-seg.YEdge+1.0, 0, 1)
		}

		if dy != 0 {
			// Segment crosses this row - compute coverage
			vecYRecip := 1.0 / delta[1]
			t0 := (y0 - y) * vecYRecip
			t1 := (y1 - y) * vecYRecip

			// X positions at intersection points
			startX := seg.Point0[0]
			x0 := startX + t0*delta[0]
			x1 := startX + t1*delta[0]

			xmin0 := min32f(x0, x1)
			xmax0 := max32f(x0, x1)

			// Process each pixel in row
			for i := 0; i < VelloTileWidth; i++ {
				iF := float32(i)

				// Coverage formula from Vello
				xmin := min32f(xmin0-iF, 1.0) - 1.0e-6
				xmax := xmax0 - iF

				b := min32f(xmax, 1.0)
				c := max32f(b, 0.0)
				d := max32f(xmin, 0.0)

				// Trapezoidal area calculation
				denom := xmax - xmin
				var a float32
				if denom != 0 {
					a = (b + 0.5*(d*d-c*c) - xmin) / denom
				}

				// KEY: y_edge is added together with a*dy
				tr.area[i] += yEdge + a*dy
			}
		} else if yEdge != 0 {
			// No Y delta but segment crosses left edge - just add y_edge
			for i := 0; i < VelloTileWidth; i++ {
				tr.area[i] += yEdge
			}
		}
	}

	// Apply fill rule
	if fillRule == FillRuleEvenOdd {
		for i := 0; i < VelloTileWidth; i++ {
			a := tr.area[i]
			im := float32(int32(0.5*a + 0.5))
			tr.area[i] = abs32(a - 2.0*im)
		}
	} else {
		// Non-zero: clamp(abs(a), 0, 1)
		for i := 0; i < VelloTileWidth; i++ {
			a := tr.area[i]
			if a < 0 {
				a = -a
			}
			if a > 1 {
				a = 1
			}
			tr.area[i] = a
		}
	}
}

// emitScanline converts row coverage to alpha runs and calls callback.
func (tr *TileRasterizer) emitScanline(pixelY int, callback func(y int, runs *AlphaRuns)) {
	tr.alphaRuns.Reset()

	var runStart int
	var currentAlpha uint8

	for i := 0; i < tr.width; i++ {
		coverage := tr.rowCoverage[i]
		alpha := uint8(clamp32(coverage, 0, 1) * 255.0)

		if i == 0 {
			currentAlpha = alpha
			runStart = 0
		} else if alpha != currentAlpha {
			if currentAlpha > 0 {
				runLen := i - runStart
				tr.alphaRuns.Add(runStart, currentAlpha, runLen-1, 0)
			}
			currentAlpha = alpha
			runStart = i
		}
	}

	// Emit final run
	if currentAlpha > 0 {
		runLen := tr.width - runStart
		if runLen > 0 {
			tr.alphaRuns.Add(runStart, currentAlpha, runLen-1, 0)
		}
	}

	callback(pixelY, tr.alphaRuns)
}

// fillPath is kept for compatibility with tests.
//
//nolint:gocognit // Direct port of Vello algorithm, complexity is inherent
func (tr *TileRasterizer) fillPath(tile *VelloTile, fillRule FillRule) {
	// Initialize full area buffer with backdrop
	backdropF := float32(tile.Backdrop)
	for i := range tr.area {
		tr.area[i] = backdropF
	}

	// Process each segment for all rows
	for _, seg := range tile.Segments {
		deltaX := seg.Point1[0] - seg.Point0[0]
		deltaY := seg.Point1[1] - seg.Point0[1]

		for yi := 0; yi < VelloTileHeight; yi++ {
			y := seg.Point0[1] - float32(yi)
			y0 := clamp32(y, 0, 1)
			y1 := clamp32(y+deltaY, 0, 1)
			dy := y0 - y1

			var yEdge float32
			if deltaX > 0 {
				yEdge = clamp32(float32(yi)-seg.YEdge+1.0, 0, 1)
			} else if deltaX < 0 {
				yEdge = -clamp32(float32(yi)-seg.YEdge+1.0, 0, 1)
			}

			if dy != 0 {
				vecYRecip := 1.0 / deltaY
				t0 := (y0 - y) * vecYRecip
				t1 := (y1 - y) * vecYRecip

				x0 := seg.Point0[0] + t0*deltaX
				x1 := seg.Point0[0] + t1*deltaX

				xmin0 := min32f(x0, x1)
				xmax0 := max32f(x0, x1)

				for i := 0; i < VelloTileWidth; i++ {
					iF := float32(i)
					xmin := min32f(xmin0-iF, 1.0) - 1.0e-6
					xmax := xmax0 - iF

					b := min32f(xmax, 1.0)
					c := max32f(b, 0.0)
					d := max32f(xmin, 0.0)

					denom := xmax - xmin
					var a float32
					if denom != 0 {
						a = (b + 0.5*(d*d-c*c) - xmin) / denom
					}

					tr.area[yi*VelloTileWidth+i] += yEdge + a*dy
				}
			} else if yEdge != 0 {
				for i := 0; i < VelloTileWidth; i++ {
					tr.area[yi*VelloTileWidth+i] += yEdge
				}
			}
		}
	}

	// Apply fill rule
	if fillRule == FillRuleEvenOdd {
		for i := range tr.area {
			a := tr.area[i]
			im := float32(int32(0.5*a + 0.5))
			tr.area[i] = abs32(a - 2.0*im)
		}
	} else {
		for i := range tr.area {
			a := tr.area[i]
			if a < 0 {
				a = -a
			}
			if a > 1 {
				a = 1
			}
			tr.area[i] = a
		}
	}
}

// abs32 returns absolute value of float32.
func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
