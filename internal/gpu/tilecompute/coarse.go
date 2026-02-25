// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

// Coarse rasterization: tile allocation, path processing, and PTCL generation.
// This combines Vello pipeline stages 11-18 into a single sequential CPU pass.
//
// On GPU, these would be separate compute dispatches (tile_alloc, path_count_setup,
// path_count, backdrop, coarse). On CPU, we can process paths sequentially.
//
// Reference: vello_shaders/src/cpu/coarse.rs, tile_alloc.rs, backdrop.rs

package tilecompute

import "math"

// CoarseOutput holds the results of coarse rasterization.
type CoarseOutput struct {
	// TilePTCLs contains one PTCL per tile (row-major, widthInTiles * heightInTiles).
	TilePTCLs []*PTCL

	// Paths contains per-path metadata (bbox in tiles, tile offset).
	Paths []Path

	// Tiles contains per-tile segment counts/indices and backdrops.
	// This is a flat array; each path's tiles start at Paths[i].Tiles offset.
	Tiles []Tile

	// Segments contains clipped path segments from all paths.
	Segments []PathSegment

	// WidthInTiles is the tile grid width.
	WidthInTiles int

	// HeightInTiles is the tile grid height.
	HeightInTiles int

	// pathTotalSegs stores the total segment count per path (needed for
	// resolving the last tile's segment range in the inverted-index scheme).
	pathTotalSegs []uint32

	// pathSegBase stores the global segment base offset for each path.
	pathSegBase []uint32
}

// CoarseRasterize performs the full coarse pipeline:
//  1. Tile allocation (assign tiles to each draw object based on path bbox)
//  2. Path stages (pathCount + pathTiling + backdrop) per path
//  3. PTCL generation (per-tile command lists in scene order)
//
// This function combines stages 11-18 of the Vello pipeline into one CPU pass.
// On GPU, these would be separate compute dispatches.
//
// Parameters:
//   - scene: the packed scene encoding (draw tags, draw data, etc.)
//   - drawMonoids: exclusive prefix sum of DrawMonoids (one per draw object)
//   - info: extracted draw info buffer (packed RGBA colors at InfoOffset)
//   - lines: all LineSoup segments (PathIx indexes into paths)
//   - widthPx, heightPx: canvas size in pixels
func CoarseRasterize(
	scene *PackedScene,
	drawMonoids []DrawMonoid,
	info []uint32,
	lines []LineSoup,
	widthPx, heightPx int,
) *CoarseOutput {
	widthInTiles := (widthPx + TileWidth - 1) / TileWidth
	heightInTiles := (heightPx + TileHeight - 1) / TileHeight

	numDrawObjects := int(scene.Layout.NumDrawObjects)
	numPaths := int(scene.Layout.NumPaths)

	out := &CoarseOutput{
		WidthInTiles:  widthInTiles,
		HeightInTiles: heightInTiles,
		TilePTCLs:     make([]*PTCL, widthInTiles*heightInTiles),
		Paths:         make([]Path, numPaths),
		pathTotalSegs: make([]uint32, numPaths),
		pathSegBase:   make([]uint32, numPaths),
	}

	// Initialize PTCLs for all tiles.
	for i := range out.TilePTCLs {
		out.TilePTCLs[i] = NewPTCL()
	}

	if numDrawObjects == 0 || numPaths == 0 || len(lines) == 0 {
		for _, ptcl := range out.TilePTCLs {
			ptcl.WriteEnd()
		}
		return out
	}

	// --- Stage 1: Group lines by PathIx ---
	linesByPath := groupLinesByPath(lines, numPaths)

	// --- Stage 2: Tile allocation + path processing ---
	currentTileOffset := uint32(0)
	globalSegOffset := uint32(0)

	for pathIx := 0; pathIx < numPaths; pathIx++ {
		pathLines := linesByPath[pathIx]
		out.pathSegBase[pathIx] = globalSegOffset

		if len(pathLines) == 0 {
			out.Paths[pathIx] = Path{
				BBox:  [4]uint32{0, 0, 0, 0},
				Tiles: currentTileOffset,
			}
			continue
		}

		// Compute bounding box from lines.
		bbox := computeLineBBox(pathLines, widthPx, heightPx)
		bboxW := int(bbox[2] - bbox[0])
		bboxH := int(bbox[3] - bbox[1])
		tileCount := uint32(bboxW * bboxH)

		path := Path{
			BBox:  bbox,
			Tiles: currentTileOffset,
		}
		out.Paths[pathIx] = path

		if tileCount == 0 {
			continue
		}

		// Extend global tiles array.
		tilesStart := len(out.Tiles)
		out.Tiles = append(out.Tiles, make([]Tile, tileCount)...)
		currentTileOffset += tileCount

		// Run path stages and get segments.
		totalSegs, segments := runPathStages(
			pathLines,
			&out.Paths[pathIx],
			out.Tiles[tilesStart:tilesStart+int(tileCount)],
		)
		out.pathTotalSegs[pathIx] = totalSegs
		out.Segments = append(out.Segments, segments...)
		globalSegOffset += totalSegs
	}

	// --- Stage 3: PTCL generation ---
	generatePTCLs(out, scene, drawMonoids, info)

	// Terminate all PTCLs.
	for _, ptcl := range out.TilePTCLs {
		ptcl.WriteEnd()
	}

	return out
}

// groupLinesByPath groups LineSoup segments by their PathIx field.
func groupLinesByPath(lines []LineSoup, numPaths int) [][]LineSoup {
	result := make([][]LineSoup, numPaths)
	for i := range lines {
		pix := int(lines[i].PathIx)
		if pix < numPaths {
			result[pix] = append(result[pix], lines[i])
		}
	}
	return result
}

// computeLineBBox computes a bounding box in tile coordinates from lines.
// Returns [x0, y0, x1, y1] in tile coordinates, clamped to the canvas.
func computeLineBBox(lines []LineSoup, widthPx, heightPx int) [4]uint32 {
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

	// Clamp to canvas.
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > float32(widthPx) {
		maxX = float32(widthPx)
	}
	if maxY > float32(heightPx) {
		maxY = float32(heightPx)
	}

	// Convert to tile coordinates.
	bboxX0 := uint32(math.Floor(float64(minX / float32(TileWidth))))
	bboxY0 := uint32(math.Floor(float64(minY / float32(TileHeight))))
	bboxX1 := uint32(math.Ceil(float64(maxX / float32(TileWidth))))
	bboxY1 := uint32(math.Ceil(float64(maxY / float32(TileHeight))))

	// Clamp to grid limits.
	gridTilesX := uint32(math.Ceil(float64(widthPx) / float64(TileWidth)))
	gridTilesY := uint32(math.Ceil(float64(heightPx) / float64(TileHeight)))
	if bboxX1 > gridTilesX {
		bboxX1 = gridTilesX
	}
	if bboxY1 > gridTilesY {
		bboxY1 = gridTilesY
	}

	return [4]uint32{bboxX0, bboxY0, bboxX1, bboxY1}
}

// runPathStages runs pathCount + segment allocation + pathTiling + backdrop
// for a single path. Modifies pathTiles in-place.
//
// Returns (totalSegments, clippedSegments).
func runPathStages(
	lines []LineSoup,
	path *Path,
	pathTiles []Tile,
) (uint32, []PathSegment) {
	bbox := path.BBox
	bboxW := int(bbox[2] - bbox[0])
	bboxH := int(bbox[3] - bbox[1])

	if bboxW*bboxH == 0 {
		return 0, nil
	}

	// Create local copies of lines with PathIx=0 (single path processing).
	localLines := make([]LineSoup, len(lines))
	for i, l := range lines {
		localLines[i] = LineSoup{PathIx: 0, P0: l.P0, P1: l.P1}
	}

	// Single-element paths array with Tiles=0 (tiles are the pathTiles slice).
	localPath := Path{BBox: bbox, Tiles: 0}
	localPaths := []Path{localPath}

	// Estimate max segCounts.
	maxSegCounts := uint32(0)
	for _, line := range localLines {
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
	bump := &BumpAllocators{Lines: uint32(len(localLines))}

	// Stage 1: pathCount.
	pathCountMain(bump, localLines, localPaths, pathTiles, segCounts)

	// Stage 2: Segment allocation (convert counts to indices).
	nextSegIx := uint32(0)
	for i := range pathTiles {
		nSegs := pathTiles[i].SegmentCountOrIx
		if nSegs != 0 {
			pathTiles[i].SegmentCountOrIx = ^nextSegIx
			nextSegIx += nSegs
		}
	}
	totalSegments := nextSegIx

	// Stage 3: pathTiling.
	segments := make([]PathSegment, totalSegments)
	pathTilingMain(bump, segCounts, localLines, localPaths, pathTiles, segments)

	// Stage 4: Backdrop prefix sum.
	for y := 0; y < bboxH; y++ {
		sum := int32(0)
		for x := 0; x < bboxW; x++ {
			idx := y*bboxW + x
			sum += pathTiles[idx].Backdrop
			pathTiles[idx].Backdrop = sum
		}
	}

	return totalSegments, segments
}

// drawParams holds resolved parameters for a single draw object during PTCL generation.
type drawParams struct {
	pathIx        int
	rgba          uint32
	evenOdd       bool
	globalSegBase uint32
	totalSegs     uint32
}

// generatePTCLs processes draw objects in scene order and generates per-tile PTCLs.
func generatePTCLs(
	out *CoarseOutput,
	scene *PackedScene,
	drawMonoids []DrawMonoid,
	info []uint32,
) {
	numDrawObjects := int(scene.Layout.NumDrawObjects)
	pathFillRules := extractPathFillRules(scene)

	for drawIx := 0; drawIx < numDrawObjects; drawIx++ {
		tag := scene.Data[scene.Layout.DrawTagBase+uint32(drawIx)]
		if tag != DrawTagColor {
			continue
		}

		dm := drawMonoids[drawIx]
		pathIx := int(dm.PathIx)
		if pathIx >= len(out.Paths) {
			continue
		}

		dp := resolveDrawParams(dm, pathIx, info, pathFillRules, out)
		emitDrawToTiles(out, dp)
	}
}

// resolveDrawParams extracts all parameters needed to emit PTCL commands for a draw.
func resolveDrawParams(
	dm DrawMonoid,
	pathIx int,
	info []uint32,
	pathFillRules []bool,
	out *CoarseOutput,
) drawParams {
	dp := drawParams{pathIx: pathIx}

	if int(dm.InfoOffset) < len(info) {
		dp.rgba = info[dm.InfoOffset]
	}
	if pathIx < len(pathFillRules) {
		dp.evenOdd = pathFillRules[pathIx]
	}
	if pathIx < len(out.pathSegBase) {
		dp.globalSegBase = out.pathSegBase[pathIx]
	}
	if pathIx < len(out.pathTotalSegs) {
		dp.totalSegs = out.pathTotalSegs[pathIx]
	}

	return dp
}

// emitDrawToTiles writes PTCL commands for one draw object across all tiles it covers.
func emitDrawToTiles(out *CoarseOutput, dp drawParams) {
	path := out.Paths[dp.pathIx]
	bbox := path.BBox
	bboxW := int(bbox[2] - bbox[0])
	bboxH := int(bbox[3] - bbox[1])
	if bboxW == 0 || bboxH == 0 {
		return
	}

	tilesStart := int(path.Tiles)

	for ty := 0; ty < bboxH; ty++ {
		for tx := 0; tx < bboxW; tx++ {
			localTileIdx := ty*bboxW + tx
			tileIdx := tilesStart + localTileIdx
			if tileIdx >= len(out.Tiles) {
				continue
			}

			globalTX := int(bbox[0]) + tx
			globalTY := int(bbox[1]) + ty
			if globalTX < 0 || globalTX >= out.WidthInTiles ||
				globalTY < 0 || globalTY >= out.HeightInTiles {
				continue
			}

			tile := out.Tiles[tileIdx]
			segCount, segStart := tileSegRange(
				tile, localTileIdx, bboxW*bboxH, out.Tiles[tilesStart:], dp.totalSegs,
			)

			globalTileIdx := globalTY*out.WidthInTiles + globalTX
			ptcl := out.TilePTCLs[globalTileIdx]

			switch {
			case segCount > 0:
				ptcl.WriteFill(segCount, dp.evenOdd, dp.globalSegBase+segStart, tile.Backdrop)
				ptcl.WriteColor(dp.rgba)
			case tile.Backdrop != 0:
				ptcl.WriteSolid()
				ptcl.WriteColor(dp.rgba)
			}
		}
	}
}

// tileSegRange computes the segment count and local start index for a tile
// within a path's tile array.
//
// After coarse allocation, tiles with segments have SegmentCountOrIx = ^segStart.
// The segment count is determined by finding the next tile's segStart or using
// totalSegments for the last tile.
func tileSegRange(
	tile Tile,
	localIdx int,
	tileCount int,
	pathTiles []Tile,
	totalSegments uint32,
) (count uint32, start uint32) {
	segStart := ^tile.SegmentCountOrIx
	if int32(segStart) < 0 {
		// No segments (SegmentCountOrIx was 0, ^0 = MaxUint32, int32 < 0).
		return 0, 0
	}

	// Find the end of this tile's segment range.
	segEnd := totalSegments
	for nextIdx := localIdx + 1; nextIdx < tileCount; nextIdx++ {
		nextStart := ^pathTiles[nextIdx].SegmentCountOrIx
		if int32(nextStart) >= 0 {
			segEnd = nextStart
			break
		}
	}

	if segEnd <= segStart {
		return 0, segStart
	}
	return segEnd - segStart, segStart
}

// extractPathFillRules determines the fill rule for each path from scene styles.
// Returns true for even-odd, false for non-zero.
func extractPathFillRules(scene *PackedScene) []bool {
	numPaths := int(scene.Layout.NumPaths)
	rules := make([]bool, numPaths)

	// In our simplified encoding, each path has exactly one style at StyleBase + pathIx.
	// Style flags: bit 1 = even-odd.
	styleCount := scene.Layout.TransformBase - scene.Layout.StyleBase
	for i := 0; i < numPaths && uint32(i) < styleCount; i++ {
		styleIdx := scene.Layout.StyleBase + uint32(i)
		if styleIdx < uint32(len(scene.Data)) {
			rules[i] = scene.Data[styleIdx]&0x02 != 0
		}
	}

	return rules
}
