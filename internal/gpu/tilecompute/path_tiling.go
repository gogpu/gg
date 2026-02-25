// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

// Direct port of vello_shaders/src/cpu/path_tiling.rs
// Variable names match Rust originals for cross-reference.

package tilecompute

// pathTilingMain is a direct port of path_tiling_main from path_tiling.rs.
// Stage 2: Segment clipping to tile boundaries + yEdge computation.
func pathTilingMain(
	bump *BumpAllocators,
	segCounts []SegmentCount,
	lines []LineSoup,
	paths []Path,
	tiles []Tile,
	segments []PathSegment,
) {
	for segIx := uint32(0); segIx < bump.SegCounts; segIx++ {
		segCount := segCounts[segIx]
		line := lines[segCount.LineIx]
		counts := segCount.Counts
		segWithinSlice := counts >> 16
		segWithinLine := counts & 0xffff

		// Recompute DDA parameters (identical to path_count)
		p0 := vec2FromArray(line.P0)
		p1 := vec2FromArray(line.P1)
		isDown := p1.y >= p0.y
		var xy0, xy1 vec2
		if isDown {
			xy0, xy1 = p0, p1
		} else {
			xy0, xy1 = p1, p0
		}
		s0 := xy0.mul(tileScale)
		s1 := xy1.mul(tileScale)
		countX := span(s0.x, s1.x) - 1
		count := countX + span(s0.y, s1.y)

		dx := abs32(s1.x - s0.x)
		dy := s1.y - s0.y
		idxdy := 1.0 / (dx + dy)
		a := dx * idxdy
		isPositiveSlope := s1.x >= s0.x
		var sign float32
		if isPositiveSlope {
			sign = 1.0
		} else {
			sign = -1.0
		}
		xt0 := floor32(s0.x * sign)
		c := s0.x*sign - xt0
		y0 := floor32(s0.y)
		var ytop float32
		if s0.y == s1.y {
			ytop = ceil32(s0.y)
		} else {
			ytop = y0 + 1.0
		}
		b := min32((dy*c+dx*(ytop-s0.y))*idxdy, oneMinusULP)
		robustErr := floor32(a*float32(count-1)+b) - float32(countX)
		if robustErr != 0.0 {
			a -= copysign32(robustEpsilon, robustErr)
		}
		var x0 float32
		if isPositiveSlope {
			x0 = xt0 * sign
		} else {
			x0 = xt0*sign - 1.0
		}

		z := floor32(a*float32(segWithinLine) + b)
		// Match Rust path_tiling.rs line 57: split truncation (x0 as i32 + (sign*z) as i32)
		x := int32(x0) + int32(sign*z)
		y := int32(y0 + float32(segWithinLine) - z)

		path := paths[line.PathIx]
		bbox := path.BBox
		bboxi := [4]int32{
			int32(bbox[0]),
			int32(bbox[1]),
			int32(bbox[2]),
			int32(bbox[3]),
		}
		stride := bboxi[2] - bboxi[0]
		tileIx := int32(path.Tiles) + (y-bboxi[1])*stride + x - bboxi[0]
		tile := tiles[tileIx]
		segStart := ^tile.SegmentCountOrIx // bitwise NOT = !seg_ix in Rust
		if int32(segStart) < 0 {
			continue
		}

		tileXY := newVec2(float32(x)*float32(TileWidth), float32(y)*float32(TileHeight))
		tileXY1 := tileXY.add(newVec2(float32(TileWidth), float32(TileHeight)))

		// Top clipping (lines 78-96 of path_tiling.rs)
		// CRITICAL: xy0 is MUTABLE — top clip modifies it, bottom clip uses modified value
		if segWithinLine > 0 {
			zPrev := floor32(a*float32(segWithinLine-1) + b)
			if z == zPrev {
				// Top edge is clipped — entered from top
				xt := xy0.x + (xy1.x-xy0.x)*(tileXY.y-xy0.y)/(xy1.y-xy0.y)
				xt = clamp32(xt, tileXY.x+1e-3, tileXY1.x)
				xy0 = newVec2(xt, tileXY.y)
			} else {
				// Side edge is clipped — entered from left (pos slope) or right (neg slope)
				var xClip float32
				if isPositiveSlope {
					xClip = tileXY.x
				} else {
					xClip = tileXY1.x
				}
				yt := xy0.y + (xy1.y-xy0.y)*(xClip-xy0.x)/(xy1.x-xy0.x)
				yt = clamp32(yt, tileXY.y+1e-3, tileXY1.y)
				xy0 = newVec2(xClip, yt)
			}
		}

		// Bottom clipping (lines 97-115 of path_tiling.rs)
		// CRITICAL: Uses xy0 which was ALREADY MODIFIED by top clipping above!
		if segWithinLine < count-1 {
			zNext := floor32(a*float32(segWithinLine+1) + b)
			if z == zNext {
				// Bottom edge is clipped
				xt := xy0.x + (xy1.x-xy0.x)*(tileXY1.y-xy0.y)/(xy1.y-xy0.y)
				xt = clamp32(xt, tileXY.x+1e-3, tileXY1.x)
				xy1 = newVec2(xt, tileXY1.y)
			} else {
				// Side edge is clipped
				var xClip float32
				if isPositiveSlope {
					xClip = tileXY1.x
				} else {
					xClip = tileXY.x
				}
				yt := xy0.y + (xy1.y-xy0.y)*(xClip-xy0.x)/(xy1.x-xy0.x)
				yt = clamp32(yt, tileXY.y+1e-3, tileXY1.y)
				xy1 = newVec2(xClip, yt)
			}
		}

		// yEdge computation (lines 116-144 of path_tiling.rs)
		yEdge := float32(1e9)
		p0out := xy0.sub(tileXY) // Convert to tile-relative
		p1out := xy1.sub(tileXY)
		const epsilon float32 = 1e-6

		if p0out.x == 0.0 {
			if p1out.x == 0.0 {
				// Both on left edge
				p0out.x = epsilon
				if p0out.y == 0.0 {
					// Entire tile
					p1out.x = epsilon
					p1out.y = float32(TileHeight)
				} else {
					// Make segment disappear
					p1out.x = 2.0 * epsilon
					p1out.y = p0out.y
				}
			} else if p0out.y == 0.0 {
				// p0 at top-left corner
				p0out.x = epsilon
			} else {
				// p0 on left edge (not corner)
				yEdge = p0out.y
			}
		} else if p1out.x == 0.0 {
			if p1out.y == 0.0 {
				// p1 at top-left corner
				p1out.x = epsilon
			} else {
				// p1 on left edge (not corner)
				yEdge = p1out.y
			}
		}

		// Pixel boundary nudging (lines 145-150)
		if p0out.x == floor32(p0out.x) && p0out.x != 0.0 {
			p0out.x -= epsilon
		}
		if p1out.x == floor32(p1out.x) && p1out.x != 0.0 {
			p1out.x -= epsilon
		}

		// Restore original direction (line 151-153)
		if !isDown {
			p0out, p1out = p1out, p0out
		}

		segment := PathSegment{
			Point0: p0out.toArray(),
			Point1: p1out.toArray(),
			YEdge:  yEdge,
		}
		segments[segStart+segWithinSlice] = segment
	}
}
