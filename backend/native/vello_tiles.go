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
	velloTileScale  = 1.0 / 16.0
)

// Vello constants for numerical robustness
const (
	velloOneMinusULP   float32 = 0.99999994
	velloRobustEpsilon float32 = 2e-7
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

	// Get path bounds for correct backdrop application
	// Backdrop should only apply to scanlines >= path's minY
	bounds := eb.Bounds()
	pathMinY := int(math.Floor(float64(bounds.MinY)))
	pathMaxY := int(math.Ceil(float64(bounds.MaxY)))
	if pathMinY < 0 {
		pathMinY = 0
	}
	if pathMaxY > tr.height {
		pathMaxY = tr.height
	}

	// 1. Bin segments to tiles (port of path_count.rs + path_tiling.rs)
	tr.binSegments(eb, aaScale)

	// 2. Prefix sum for backdrop (port of backdrop.rs)
	tr.computeBackdropPrefixSum()

	// 3. Rasterize row by row
	for tileY := 0; tileY < tr.tilesY; tileY++ {
		// Process each scanline within this tile row
		for localY := 0; localY < VelloTileHeight; localY++ {
			pixelY := tileY*VelloTileHeight + localY
			if pixelY >= tr.height || pixelY >= pathMaxY {
				break
			}
			// Skip scanlines above path's minimum Y
			if pixelY < pathMinY {
				continue
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

// velloSpan computes the number of tiles a segment spans.
// Direct port of util.rs span function.
func velloSpan(a, b float32) int {
	maxVal := a
	if b > maxVal {
		maxVal = b
	}
	minVal := a
	if b < minVal {
		minVal = b
	}
	result := float32(math.Ceil(float64(maxVal))) - float32(math.Floor(float64(minVal)))
	if result < 1.0 {
		result = 1.0
	}
	return int(result)
}

// binSegments distributes edge segments to tiles.
// Direct port of path_count.rs (backdrop) + path_tiling.rs (segments with y_edge).
//
//nolint:gocognit,gocyclo,cyclop,funlen,maintidx // Direct port of Vello algorithm
func (tr *TileRasterizer) binSegments(eb *EdgeBuilder, aaScale float32) {
	const epsilon float32 = 1e-6
	const noYEdge float32 = 1e9

	// Get path's bounding box in tile coordinates.
	// In Vello, path.bbox determines where backdrop is added.
	// Note: eb.Bounds() returns pixel coordinates, NOT sub-pixel!
	bounds := eb.Bounds()
	pathBboxMinX := int(math.Floor(float64(bounds.MinX * velloTileScale)))
	pathBboxMinY := int(math.Floor(float64(bounds.MinY * velloTileScale)))
	pathBboxMaxX := int(math.Ceil(float64(bounds.MaxX * velloTileScale)))
	pathBboxMaxY := int(math.Ceil(float64(bounds.MaxY * velloTileScale)))
	if pathBboxMinX < 0 {
		pathBboxMinX = 0
	}
	if pathBboxMinY < 0 {
		pathBboxMinY = 0
	}
	if pathBboxMaxX > tr.tilesX {
		pathBboxMaxX = tr.tilesX
	}
	if pathBboxMaxY > tr.tilesY {
		pathBboxMaxY = tr.tilesY
	}

	for edge := range eb.AllEdges() {
		line := edge.AsLine()
		if line == nil {
			continue
		}

		// Convert to pixel coordinates
		px0 := FDot16ToFloat32(line.X) / aaScale
		py0 := float32(line.FirstY) / aaScale
		py1 := float32(line.LastY+1) / aaScale
		dxPerY := FDot16ToFloat32(line.DX)
		px1 := px0 + dxPerY*(py1-py0)

		// Winding > 0 means segment originally went DOWN
		isDown := line.Winding > 0
		delta := 1
		if isDown {
			delta = -1
		}

		// Normalize so y0 <= y1 (already done by EdgeBuilder, but use local vars)
		x0, y0, x1, y1 := px0, py0, px1, py1

		// Scale to tile coordinates
		s0x := x0 * velloTileScale
		s0y := y0 * velloTileScale
		s1x := x1 * velloTileScale
		s1y := y1 * velloTileScale

		// DDA setup from path_count.rs
		countX := velloSpan(s0x, s1x) - 1
		count := countX + velloSpan(s0y, s1y)

		dx := float32(math.Abs(float64(s1x - s0x)))
		dy := s1y - s0y

		if dx+dy == 0.0 {
			continue
		}
		if dy == 0.0 && float32(math.Floor(float64(s0y))) == s0y {
			continue
		}

		idxdy := 1.0 / (dx + dy)
		a := dx * idxdy
		isPositiveSlope := s1x >= s0x
		sign := float32(1.0)
		if !isPositiveSlope {
			sign = -1.0
		}
		xt0 := float32(math.Floor(float64(s0x * sign)))
		c := s0x*sign - xt0
		tileY0 := float32(math.Floor(float64(s0y)))
		ytop := tileY0 + 1.0
		if s0y == s1y {
			ytop = float32(math.Ceil(float64(s0y)))
		}
		b := (dy*c + dx*(ytop-s0y)) * idxdy
		if b > velloOneMinusULP {
			b = velloOneMinusULP
		}

		// Robustness correction
		robustErr := float32(math.Floor(float64(a*float32(count-1)+b))) - float32(countX)
		if robustErr != 0.0 {
			if robustErr > 0 {
				a -= velloRobustEpsilon
			} else {
				a += velloRobustEpsilon
			}
		}

		tileX0 := xt0 * sign
		if !isPositiveSlope {
			tileX0 -= 1.0
		}

		// Use path's bounding box in tiles (computed once at start)
		bboxMinX := pathBboxMinX
		bboxMinY := pathBboxMinY
		bboxMaxX := pathBboxMaxX
		bboxMaxY := pathBboxMaxY

		xmin := s0x
		if s1x < xmin {
			xmin = s1x
		}
		xmax := s0x
		if s1x > xmax {
			xmax = s1x
		}

		// Skip if entirely outside
		if s0y >= float32(bboxMaxY) || s1y < float32(bboxMinY) || xmin >= float32(bboxMaxX) {
			continue
		}

		// Compute iteration bounds
		imin := 0
		imax := count

		// Clip to bounding box
		if s0y < float32(bboxMinY) {
			iminf := ((float32(bboxMinY) - tileY0 + b - a) / (1.0 - a))
			iminf = float32(math.Round(float64(iminf))) - 1.0
			if tileY0+iminf-float32(math.Floor(float64(a*iminf+b))) < float32(bboxMinY) {
				iminf += 1.0
			}
			imin = int(iminf)
		}
		if s1y > float32(bboxMaxY) {
			imaxf := ((float32(bboxMaxY) - tileY0 + b - a) / (1.0 - a))
			imaxf = float32(math.Round(float64(imaxf))) - 1.0
			if tileY0+imaxf-float32(math.Floor(float64(a*imaxf+b))) < float32(bboxMaxY) {
				imaxf += 1.0
			}
			imax = int(imaxf)
		}

		// Handle segments entirely to the left
		ymin := 0
		ymax := 0
		if xmax < float32(bboxMinX) {
			ymin = int(math.Ceil(float64(s0y)))
			ymax = int(math.Ceil(float64(s1y)))
			imax = imin
		} else {
			fudge := float32(0.0)
			if !isPositiveSlope {
				fudge = 1.0
			}
			if xmin < float32(bboxMinX) {
				f := (sign*(float32(bboxMinX)-tileX0) - b + fudge) / a
				f = float32(math.Round(float64(f)))
				cond := tileX0+sign*float32(math.Floor(float64(a*f+b))) < float32(bboxMinX)
				if cond == isPositiveSlope {
					f += 1.0
				}
				ynext := int(tileY0 + f - float32(math.Floor(float64(a*f+b))) + 1.0)
				if isPositiveSlope {
					if int(f) > imin {
						ymin = int(tileY0)
						if tileY0 != s0y {
							ymin = int(tileY0 + 1.0)
						}
						ymax = ynext
						imin = int(f)
					}
				} else if int(f) < imax {
					ymin = ynext
					ymax = int(math.Ceil(float64(s1y)))
					imax = int(f)
				}
			}
			if xmax > float32(bboxMaxX) {
				f := (sign*(float32(bboxMaxX)-tileX0) - b + fudge) / a
				f = float32(math.Round(float64(f)))
				cond := tileX0+sign*float32(math.Floor(float64(a*f+b))) < float32(bboxMaxX)
				if cond == isPositiveSlope {
					f += 1.0
				}
				if isPositiveSlope {
					if int(f) < imax {
						imax = int(f)
					}
				} else {
					if int(f) > imin {
						imin = int(f)
					}
				}
			}
		}

		if imax < imin {
			imax = imin
		}
		if ymin < bboxMinY {
			ymin = bboxMinY
		}
		if ymax > bboxMaxY {
			ymax = bboxMaxY
		}

		// Add backdrop for segments crossing into bounding box from left
		// In Vello: base = path.tiles + (y - bbox[1]) * stride, then tile[base].backdrop += delta
		// This adds backdrop to the FIRST column of the BBOX, not column 0 of the grid
		for y := ymin; y < ymax; y++ {
			if y >= 0 && y < tr.tilesY && bboxMinX >= 0 && bboxMinX < tr.tilesX {
				tr.tiles[y*tr.tilesX+bboxMinX].Backdrop += delta
			}
		}

		// DDA walk to bin segments and update backdrop
		lastZ := float32(math.Floor(float64(a*float32(imin-1) + b)))
		for i := imin; i < imax; i++ {
			zf := a*float32(i) + b
			z := float32(math.Floor(float64(zf)))
			tileY := int(tileY0 + float32(i) - z)
			tileX := int(tileX0 + sign*z)

			// Skip if outside bounds
			if tileY < 0 || tileY >= tr.tilesY || tileX < 0 || tileX >= tr.tilesX {
				lastZ = z
				continue
			}

			// top_edge detection from path_count.rs
			// top_edge is true when segment enters tile from the top edge.
			// For i==0: check if segment starts at tile boundary (y0 == s0y)
			// For i>0: check if segment enters tile from top (lastZ == z)
			topEdge := false
			if i == 0 {
				topEdge = (tileY0 == s0y)
			} else {
				topEdge = (lastZ == z)
			}

			// Add backdrop to tile x+1 when crossing from top
			if topEdge && tileX+1 < tr.tilesX {
				xBump := tileX + 1
				if xBump < bboxMinX {
					xBump = bboxMinX
				}
				if xBump < tr.tilesX {
					tr.tiles[tileY*tr.tilesX+xBump].Backdrop += delta
				}
			}

			// FIX: When a PERFECTLY VERTICAL edge (dx = 0) starts INSIDE a tile (i==0 && !topEdge)
			// and is in the leftmost column, tiles to the right need fill for rows >= startY.
			// Only apply for truly vertical edges (dx exactly 0) to avoid artifacts on curved shapes.
			edgeDx := x1 - x0
			isVertical := edgeDx == 0 // Only truly vertical edges
			if i == 0 && !topEdge && tileX == bboxMinX && isVertical && tileX+1 < tr.tilesX {
				// Segment start Y in tile coords
				segStartY := (s0y - float32(tileY)) * VelloTileHeight
				if segStartY < 0 {
					segStartY = 0
				}
				// Add minimal segment at x=0 with y_edge to fill interior
				xBump := tileX + 1
				if xBump < tr.tilesX {
					tileIdx := tileY*tr.tilesX + xBump
					syntheticYEdge := segStartY
					// Add segment with positive dx (going right) to add fill via y_edge
					tr.tiles[tileIdx].Segments = append(tr.tiles[tileIdx].Segments, PathSegment{
						Point0: [2]float32{0, segStartY},
						Point1: [2]float32{epsilon, segStartY},
						YEdge:  syntheticYEdge,
					})
				}
			}

			// Now add the segment to this tile using path_tiling.rs logic
			tr.addSegmentToTile(x0, y0, x1, y1, tileX, tileY, isDown, i, imin, imax, a, b, tileY0, sign, isPositiveSlope, epsilon, noYEdge)

			lastZ = z
		}
	}
}

// addSegmentToTile clips and adds a segment to a specific tile.
// Port of path_tiling.rs segment clipping and y_edge calculation.
//
//nolint:gocognit,gocyclo,cyclop,funlen // Direct port of Vello algorithm
func (tr *TileRasterizer) addSegmentToTile(
	x0, y0, x1, y1 float32,
	tileX, tileY int,
	isDown bool,
	i, imin, imax int,
	a, b, tileY0f, sign float32,
	isPositiveSlope bool,
	epsilon, noYEdge float32,
) {
	const tileW = float32(VelloTileWidth)
	const tileH = float32(VelloTileHeight)

	tileLeftX := float32(tileX) * tileW
	tileTopY := float32(tileY) * tileH
	tileRightX := tileLeftX + tileW
	tileBotY := tileTopY + tileH

	// Start with original segment
	xy0x, xy0y := x0, y0
	xy1x, xy1y := x1, y1

	// Clip to tile boundaries (from path_tiling.rs)
	if i > imin {
		zPrev := float32(math.Floor(float64(a*float32(i-1) + b)))
		z := float32(math.Floor(float64(a*float32(i) + b)))
		if z == zPrev {
			// Top edge is clipped
			if y1 != y0 {
				t := (tileTopY - y0) / (y1 - y0)
				xt := x0 + (x1-x0)*t
				if xt < tileLeftX+1e-3 {
					xt = tileLeftX + 1e-3
				}
				if xt > tileRightX {
					xt = tileRightX
				}
				xy0x, xy0y = xt, tileTopY
			}
		} else {
			// Left or right edge is clipped
			var xClip float32
			if isPositiveSlope {
				xClip = tileLeftX
			} else {
				xClip = tileRightX
			}
			if x1 != x0 {
				t := (xClip - x0) / (x1 - x0)
				yt := y0 + (y1-y0)*t
				if yt < tileTopY+1e-3 {
					yt = tileTopY + 1e-3
				}
				if yt > tileBotY {
					yt = tileBotY
				}
				xy0x, xy0y = xClip, yt
			}
		}
	}

	if i < imax-1 {
		zNext := float32(math.Floor(float64(a*float32(i+1) + b)))
		z := float32(math.Floor(float64(a*float32(i) + b)))
		if z == zNext {
			// Bottom edge is clipped
			if y1 != y0 {
				t := (tileBotY - y0) / (y1 - y0)
				xt := x0 + (x1-x0)*t
				if xt < tileLeftX+1e-3 {
					xt = tileLeftX + 1e-3
				}
				if xt > tileRightX {
					xt = tileRightX
				}
				xy1x, xy1y = xt, tileBotY
			}
		} else {
			// Left or right edge is clipped
			var xClip float32
			if isPositiveSlope {
				xClip = tileRightX
			} else {
				xClip = tileLeftX
			}
			if x1 != x0 {
				t := (xClip - x0) / (x1 - x0)
				yt := y0 + (y1-y0)*t
				if yt < tileTopY+1e-3 {
					yt = tileTopY + 1e-3
				}
				if yt > tileBotY {
					yt = tileBotY
				}
				xy1x, xy1y = xClip, yt
			}
		}
	}

	// Convert to tile-relative coordinates
	p0x := xy0x - tileLeftX
	p0y := xy0y - tileTopY
	p1x := xy1x - tileLeftX
	p1y := xy1y - tileTopY

	// Apply numerical robustness and compute y_edge (from path_tiling.rs)
	yEdge := noYEdge

	// FIX DISABLED FOR TESTING - was causing over-fill on circle
	// Original fix: for segments entering from TOP and exiting through RIGHT,
	// set y_edge to provide fill compensation.
	// Problem: this fills ALL pixels in the row, including exterior pixels to the left.
	//
	// exitsRight := p1x >= tileW-epsilon && p1x <= tileW+epsilon
	// if i == 0 && p0x > 0 && p0y > 0 && exitsRight {
	// 	yEdge = p0y
	// }

	//nolint:nestif,gocritic // Direct port
	if p0x == 0.0 {
		if p1x == 0.0 {
			p0x = epsilon
			if p0y == 0.0 {
				p1x = epsilon
				p1y = tileH
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

	// Restore original direction - segments are stored with original winding
	if !isDown {
		p0x, p1x = p1x, p0x
		p0y, p1y = p1y, p0y
	}

	// Add segment to tile
	tileIdx := tileY*tr.tilesX + tileX
	tr.tiles[tileIdx].Segments = append(tr.tiles[tileIdx].Segments, PathSegment{
		Point0: [2]float32{p0x, p0y},
		Point1: [2]float32{p1x, p1y},
		YEdge:  yEdge,
	})
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
			segX0 := startX + t0*delta[0]
			segX1 := startX + t1*delta[0]

			xmin0 := min32f(segX0, segX1)
			xmax0 := max32f(segX0, segX1)

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

				segX0 := seg.Point0[0] + t0*deltaX
				segX1 := seg.Point0[0] + t1*deltaX

				xmin0 := min32f(segX0, segX1)
				xmax0 := max32f(segX0, segX1)

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
