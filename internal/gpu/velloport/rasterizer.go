// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Rasterizer ties together all Vello CPU pipeline stages into a single API.
// This is NOT part of the Vello source — it's our integration layer.

package velloport

import "math"

// Rasterizer runs the complete Vello CPU rasterization pipeline.
type Rasterizer struct {
	width, height int
}

// NewRasterizer creates a new rasterizer for the given canvas size.
func NewRasterizer(width, height int) *Rasterizer {
	return &Rasterizer{width: width, height: height}
}

// Rasterize runs the full pipeline and returns per-pixel alpha values [0.0, 1.0].
// The result is a flat array of width*height float32 values in row-major order.
func (r *Rasterizer) Rasterize(lines []LineSoup, fillRule FillRule) []float32 {
	if len(lines) == 0 {
		return make([]float32, r.width*r.height)
	}

	// Step 0: Compute bounding box in tile coordinates
	path, tilesX, tilesY := r.computePath(lines)

	tileCount := tilesX * tilesY
	if tileCount == 0 {
		return make([]float32, r.width*r.height)
	}

	// Allocate tiles
	tiles := make([]Tile, tileCount)

	// Step 1: pathCount — DDA walk, backdrop, segment counting
	// We need to estimate max segCounts size.
	// Each line produces at most span_x + span_y segments.
	maxSegCounts := uint32(0)
	for _, line := range lines {
		p0 := vec2FromArray(line.P0)
		p1 := vec2FromArray(line.P1)
		var xy0, xy1 vec2
		if p1.y >= p0.y {
			xy0, xy1 = p0, p1
		} else {
			xy0, xy1 = p1, p0
		}
		s0 := xy0.mul(tileScale)
		s1 := xy1.mul(tileScale)
		countX := span(s0.x, s1.x) - 1
		count := countX + span(s0.y, s1.y)
		maxSegCounts += count
	}

	segCounts := make([]SegmentCount, maxSegCounts)
	bump := &BumpAllocators{
		Lines: uint32(len(lines)),
	}
	paths := []Path{path}

	pathCountMain(bump, lines, paths, tiles, segCounts)

	// Step 2: Coarse allocation — convert counts to indices
	// (Port of coarse.rs segment allocation, lines 79-83)
	nextSegIx := uint32(0)
	for i := range tiles {
		nSegs := tiles[i].SegmentCountOrIx
		if nSegs != 0 {
			tiles[i].SegmentCountOrIx = ^nextSegIx // !seg_ix in Rust
			nextSegIx += nSegs
		}
	}
	totalSegments := nextSegIx

	// Step 3: pathTiling — segment clipping + yEdge
	segments := make([]PathSegment, totalSegments)
	pathTilingMain(bump, segCounts, lines, paths, tiles, segments)

	// Step 4: Backdrop prefix sum
	// (Port of backdrop.rs)
	bboxW := int(path.BBox[2] - path.BBox[0])
	bboxH := int(path.BBox[3] - path.BBox[1])
	base := int(path.Tiles)
	for y := 0; y < bboxH; y++ {
		sum := int32(0)
		for x := 0; x < bboxW; x++ {
			idx := base + y*bboxW + x
			sum += tiles[idx].Backdrop
			tiles[idx].Backdrop = sum
		}
	}

	// Step 5: Fine rasterization — per tile fill_path
	result := make([]float32, r.width*r.height)
	area := make([]float32, TileWidth*TileHeight)

	bboxX0 := int(path.BBox[0])
	bboxY0 := int(path.BBox[1])

	for ty := 0; ty < tilesY; ty++ {
		for tx := 0; tx < tilesX; tx++ {
			tileIdx := base + ty*bboxW + tx
			tile := tiles[tileIdx]

			// Gather segments for this tile
			var tileSegments []PathSegment
			segStart := ^tile.SegmentCountOrIx
			if int32(segStart) >= 0 {
				// Count how many segments this tile has by looking at
				// the next tile's segStart (or totalSegments)
				segEnd := totalSegments
				// Find next tile that has segments
				for nextIdx := tileIdx + 1; nextIdx < len(tiles); nextIdx++ {
					nextStart := ^tiles[nextIdx].SegmentCountOrIx
					if int32(nextStart) >= 0 {
						segEnd = nextStart
						break
					}
				}
				if segStart < segEnd {
					tileSegments = segments[segStart:segEnd]
				}
			}

			// Clear area
			for i := range area {
				area[i] = 0
			}

			// Fill
			fillPath(area, tileSegments, tile.Backdrop, fillRule == FillRuleEvenOdd)

			// Write to result
			globalTileX := (bboxX0 + tx) * TileWidth
			globalTileY := (bboxY0 + ty) * TileHeight
			for ly := 0; ly < TileHeight; ly++ {
				py := globalTileY + ly
				if py >= r.height {
					break
				}
				for lx := 0; lx < TileWidth; lx++ {
					px := globalTileX + lx
					if px >= r.width {
						break
					}
					result[py*r.width+px] = area[ly*TileWidth+lx]
				}
			}
		}
	}

	return result
}

// computePath computes the Path struct and tile grid dimensions from lines.
func (r *Rasterizer) computePath(lines []LineSoup) (Path, int, int) {
	// Compute pixel bounding box from all lines
	minX := float32(math.MaxFloat32)
	minY := float32(math.MaxFloat32)
	maxX := float32(-math.MaxFloat32)
	maxY := float32(-math.MaxFloat32)

	for _, line := range lines {
		for _, p := range [][2]float32{line.P0, line.P1} {
			if p[0] < minX {
				minX = p[0]
			}
			if p[0] > maxX {
				maxX = p[0]
			}
			if p[1] < minY {
				minY = p[1]
			}
			if p[1] > maxY {
				maxY = p[1]
			}
		}
	}

	// Clamp to canvas
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > float32(r.width) {
		maxX = float32(r.width)
	}
	if maxY > float32(r.height) {
		maxY = float32(r.height)
	}

	// Convert to tile coordinates
	bboxX0 := uint32(math.Floor(float64(minX / float32(TileWidth))))
	bboxY0 := uint32(math.Floor(float64(minY / float32(TileHeight))))
	bboxX1 := uint32(math.Ceil(float64(maxX / float32(TileWidth))))
	bboxY1 := uint32(math.Ceil(float64(maxY / float32(TileHeight))))

	// Clamp to grid limits
	gridTilesX := uint32(math.Ceil(float64(r.width) / float64(TileWidth)))
	gridTilesY := uint32(math.Ceil(float64(r.height) / float64(TileHeight)))
	if bboxX1 > gridTilesX {
		bboxX1 = gridTilesX
	}
	if bboxY1 > gridTilesY {
		bboxY1 = gridTilesY
	}

	tilesX := int(bboxX1 - bboxX0)
	tilesY := int(bboxY1 - bboxY0)

	path := Path{
		BBox:  [4]uint32{bboxX0, bboxY0, bboxX1, bboxY1},
		Tiles: 0,
	}

	return path, tilesX, tilesY
}

// LineSoupFromVelloLine converts a pre-sorted VelloLine (P0.Y <= P1.Y, IsDown flag)
// back to original LineSoup direction (unsorted, as Vello's flattener would emit).
func LineSoupFromVelloLine(p0, p1 [2]float32, isDown bool) LineSoup {
	if isDown {
		// Original direction was downward (p0.y < p1.y) — already in correct order
		return LineSoup{PathIx: 0, P0: p0, P1: p1}
	}
	// Original direction was upward (p0 was below p1) — swap back
	return LineSoup{PathIx: 0, P0: p1, P1: p0}
}
