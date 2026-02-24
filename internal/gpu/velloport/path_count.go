// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Direct port of vello_shaders/src/cpu/path_count.rs
// Variable names match Rust originals for cross-reference.

package velloport

// pathCountMain is a direct port of path_count_main from path_count.rs.
// Stage 1: DDA tile walk + backdrop computation + segment counting.
func pathCountMain(
	bump *BumpAllocators,
	lines []LineSoup,
	paths []Path,
	tile []Tile,
	segCounts []SegmentCount,
) {
	for lineIx := uint32(0); lineIx < bump.Lines; lineIx++ {
		line := lines[lineIx]
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
		if dx+dy == 0.0 {
			continue
		}
		if dy == 0.0 && floor32(s0.y) == s0.y {
			continue
		}
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

		path := paths[line.PathIx]
		bbox := path.BBox
		bboxi := [4]int32{
			int32(bbox[0]),
			int32(bbox[1]),
			int32(bbox[2]),
			int32(bbox[3]),
		}
		xmin := min32(s0.x, s1.x)
		stride := bboxi[2] - bboxi[0]
		if s0.y >= float32(bboxi[3]) || s1.y < float32(bboxi[1]) || xmin >= float32(bboxi[2]) || stride == 0 {
			continue
		}

		// Clip line to bounding box in "i" space
		imin := uint32(0)
		if s0.y < float32(bboxi[1]) {
			iminf := round32((float32(bboxi[1])-y0+b-a)/(1.0-a)) - 1.0
			if y0+iminf-floor32(a*iminf+b) < float32(bboxi[1]) {
				iminf += 1.0
			}
			imin = uint32(iminf)
		}
		imax := count
		if s1.y > float32(bboxi[3]) {
			imaxf := round32((float32(bboxi[3])-y0+b-a)/(1.0-a)) - 1.0
			if y0+imaxf-floor32(a*imaxf+b) < float32(bboxi[3]) {
				imaxf += 1.0
			}
			imax = uint32(imaxf)
		}

		var delta int32
		if isDown {
			delta = -1
		} else {
			delta = 1
		}
		ymin := int32(0)
		ymax := int32(0)

		if max32(s0.x, s1.x) < float32(bboxi[0]) {
			ymin = int32(ceil32(s0.y))
			ymax = int32(ceil32(s1.y))
			imax = imin
		} else {
			var fudge float32
			if !isPositiveSlope {
				fudge = 1.0
			}
			if xmin < float32(bboxi[0]) {
				f := round32((sign*(float32(bboxi[0])-x0) - b + fudge) / a)
				if (x0+sign*floor32(a*f+b) < float32(bboxi[0])) == isPositiveSlope {
					f += 1.0
				}
				ynext := int32(y0 + f - floor32(a*f+b) + 1.0)
				if isPositiveSlope {
					if uint32(f) > imin {
						var yOff float32
						if y0 != s0.y {
							yOff = 1.0
						}
						ymin = int32(y0 + yOff)
						ymax = ynext
						imin = uint32(f)
					}
				} else if uint32(f) < imax {
					ymin = ynext
					ymax = int32(ceil32(s1.y))
					imax = uint32(f)
				}
			}
			if max32(s0.x, s1.x) > float32(bboxi[2]) {
				f := round32((sign*(float32(bboxi[2])-x0) - b + fudge) / a)
				if (x0+sign*floor32(a*f+b) < float32(bboxi[2])) == isPositiveSlope {
					f += 1.0
				}
				if isPositiveSlope {
					imax = minu32(imax, uint32(f))
				} else {
					imin = maxu32(imin, uint32(f))
				}
			}
		}

		imax = maxu32(imin, imax)
		ymin = maxi32(ymin, bboxi[1])
		ymax = mini32(ymax, bboxi[3])

		// Apply backdrop for left-overflow segments
		for y := ymin; y < ymax; y++ {
			base := int32(path.Tiles) + (y-bboxi[1])*stride
			tile[base].Backdrop += delta
		}

		// DDA walk
		lastZ := floor32(a*float32(imin-1) + b)
		segBase := bump.SegCounts
		bump.SegCounts += imax - imin

		for i := imin; i < imax; i++ {
			zf := a*float32(i) + b
			z := floor32(zf)
			y := int32(y0 + float32(i) - z)
			x := int32(x0 + sign*z)
			base := int32(path.Tiles) + (y-bboxi[1])*stride - bboxi[0]

			// top_edge: did segment enter from the top of this tile?
			var topEdge bool
			if i == 0 {
				topEdge = y0 == s0.y
			} else {
				topEdge = lastZ == z
			}
			if topEdge && x+1 < bboxi[2] {
				xBump := maxi32(x+1, bboxi[0])
				tile[base+xBump].Backdrop += delta
			}

			// Count segment in this tile
			segWithinSlice := tile[base+x].SegmentCountOrIx
			tile[base+x].SegmentCountOrIx++

			// Store SegmentCount
			counts := (segWithinSlice << 16) | i
			segCounts[segBase+i-imin] = SegmentCount{
				LineIx: lineIx,
				Counts: counts,
			}

			lastZ = z
		}
	}
}
