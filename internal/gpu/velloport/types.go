// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package velloport

// Tile dimensions (matching Vello TILE_WIDTH/TILE_HEIGHT).
const (
	TileWidth  = 16
	TileHeight = 16
	tileScale  = 1.0 / 16.0
)

// LineSoup matches vello_encoding::LineSoup.
// Lines are stored in their ORIGINAL direction (NOT pre-sorted by Y).
type LineSoup struct {
	PathIx uint32
	P0     [2]float32 // Start point in pixel coordinates
	P1     [2]float32 // End point in pixel coordinates
}

// Path matches vello_encoding::Path.
type Path struct {
	BBox  [4]uint32 // Bounding box in TILE coordinates [x0, y0, x1, y1]
	Tiles uint32    // Offset into the Tile array
}

// Tile matches vello_encoding::Tile.
type Tile struct {
	Backdrop         int32
	SegmentCountOrIx uint32 // Count in path_count, inverted index after coarse
}

// SegmentCount matches vello_encoding::SegmentCount.
// Bridge between path_count (stage 1) and path_tiling (stage 2).
type SegmentCount struct {
	LineIx uint32 // Index into LineSoup array
	Counts uint32 // (seg_within_slice << 16) | seg_within_line
}

// PathSegment matches vello_encoding::PathSegment.
// Coordinates are tile-relative.
type PathSegment struct {
	Point0 [2]float32 // Tile-relative start point
	Point1 [2]float32 // Tile-relative end point
	YEdge  float32    // Y where segment touches x=0 (tile-relative), or 1e9
}

// BumpAllocators matches vello_encoding::BumpAllocators (subset).
type BumpAllocators struct {
	Lines     uint32
	SegCounts uint32
	Segments  uint32
}

// FillRule for winding number computation.
type FillRule int

const (
	FillRuleNonZero FillRule = iota
	FillRuleEvenOdd
)
