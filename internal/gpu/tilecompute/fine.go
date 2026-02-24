// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Direct port of fill_path from vello_shaders/src/cpu/fine.rs (lines 51-109).
// Variable names match Rust originals for cross-reference.

package tilecompute

// fineRasterizeTile processes a PTCL command stream for a single tile,
// producing RGBA pixel output. This is the CPU version of Vello's fine.wgsl main loop.
//
// The function walks the PTCL command stream, executing CmdFill (compute coverage),
// CmdSolid (full coverage), CmdColor (apply color with source-over), CmdBeginClip
// (push clip layer), and CmdEndClip (pop clip and composite). All compositing uses
// premultiplied alpha throughout.
//
// Parameters:
//   - ptcl: the per-tile command list to execute
//   - segments: global segment array (CmdFill.SegIndex indexes into this)
//   - bgColor: background color as premultiplied float32 RGBA [0,1]
//
// Returns per-pixel premultiplied RGBA as [TileWidth*TileHeight][4]float32.
//
//nolint:funlen // Direct port of Vello fine.wgsl PTCL dispatch loop; splitting would hurt cross-reference.
func fineRasterizeTile(ptcl *PTCL, segments []PathSegment, bgColor [4]float32) [TileWidth * TileHeight][4]float32 {
	const pixelCount = TileWidth * TileHeight

	// Initialize output with background color.
	var rgba [pixelCount][4]float32
	for i := range rgba {
		rgba[i] = bgColor
	}

	if ptcl == nil {
		return rgba
	}

	// Working area buffer for coverage values.
	var area [pixelCount]float32

	// Clip stack for CmdBeginClip / CmdEndClip pairs.
	var clipStack [][pixelCount][4]float32

	offset := 0
	for {
		tag, nextOffset := ptcl.ReadCmd(offset)
		offset = nextOffset

		switch tag {
		case CmdEnd:
			return rgba

		case CmdFill:
			data, next := ptcl.ReadFillData(offset)
			offset = next

			// Clear area for this fill.
			for i := range area {
				area[i] = 0
			}

			// Extract the segment slice from the global array.
			segStart := data.SegIndex
			segEnd := segStart + data.SegCount
			if segEnd > uint32(len(segments)) {
				segEnd = uint32(len(segments))
			}
			tileSegs := segments[segStart:segEnd]

			fillPath(area[:], tileSegs, data.Backdrop, data.EvenOdd)

		case CmdSolid:
			// Fully covered tile: all area = 1.0.
			for i := range area {
				area[i] = 1.0
			}

		case CmdColor:
			data, next := ptcl.ReadColorData(offset)
			offset = next

			// Unpack premultiplied RGBA from uint32.
			r := float32(data.RGBA&0xFF) / 255.0
			g := float32((data.RGBA>>8)&0xFF) / 255.0
			b := float32((data.RGBA>>16)&0xFF) / 255.0
			a := float32((data.RGBA>>24)&0xFF) / 255.0

			// Source-over compositing: for each pixel,
			// fg = color * area[i], then rgba[i] = rgba[i] * (1 - fg.a) + fg.
			for i := 0; i < pixelCount; i++ {
				cov := area[i]
				fgR := r * cov
				fgG := g * cov
				fgB := b * cov
				fgA := a * cov

				inv := 1.0 - fgA
				rgba[i][0] = rgba[i][0]*inv + fgR
				rgba[i][1] = rgba[i][1]*inv + fgG
				rgba[i][2] = rgba[i][2]*inv + fgB
				rgba[i][3] = rgba[i][3]*inv + fgA
			}

		case CmdBeginClip:
			// Push current rgba to clip stack and clear to transparent.
			saved := rgba
			clipStack = append(clipStack, saved)
			for i := range rgba {
				rgba[i] = [4]float32{0, 0, 0, 0}
			}

		case CmdEndClip:
			data, next := ptcl.ReadEndClipData(offset)
			offset = next

			// Pop saved state from clip stack.
			if len(clipStack) == 0 {
				// Malformed PTCL -- no matching BeginClip. Skip.
				continue
			}
			saved := clipStack[len(clipStack)-1]
			clipStack = clipStack[:len(clipStack)-1]

			alpha := data.Alpha
			_ = data.Blend // Only source-over (blend=0) is supported currently.

			// For each pixel: fg = rgba[i] * area[i] * alpha,
			// then rgba[i] = saved[i] * (1 - fg.a) + fg.
			for i := 0; i < pixelCount; i++ {
				scale := area[i] * alpha
				fgR := rgba[i][0] * scale
				fgG := rgba[i][1] * scale
				fgB := rgba[i][2] * scale
				fgA := rgba[i][3] * scale

				inv := 1.0 - fgA
				rgba[i][0] = saved[i][0]*inv + fgR
				rgba[i][1] = saved[i][1]*inv + fgG
				rgba[i][2] = saved[i][2]*inv + fgB
				rgba[i][3] = saved[i][3]*inv + fgA
			}

		default:
			// Unknown command -- skip to avoid infinite loop on malformed data.
			return rgba
		}
	}
}

// premulToStraightU8 converts a premultiplied float32 RGBA pixel to straight alpha uint8.
func premulToStraightU8(pm [4]float32) [4]uint8 {
	a := pm[3]
	if a <= 0 {
		return [4]uint8{}
	}
	// Clamp alpha to [0,1].
	if a > 1.0 {
		a = 1.0
	}
	r := pm[0] / a
	if r > 1.0 {
		r = 1.0
	}
	g := pm[1] / a
	if g > 1.0 {
		g = 1.0
	}
	b := pm[2] / a
	if b > 1.0 {
		b = 1.0
	}
	return [4]uint8{
		uint8(r*255.0 + 0.5),
		uint8(g*255.0 + 0.5),
		uint8(b*255.0 + 0.5),
		uint8(a*255.0 + 0.5),
	}
}

// fillPath is a direct port of fine.rs fill_path.
// Computes per-pixel area values for a tile using its segments.
//
// area: output array of TileWidth*TileHeight float32 values
// segments: PathSegments for this tile (tile-relative coordinates)
// backdrop: accumulated winding number from tiles to the left
// evenOdd: true for even-odd fill rule, false for non-zero
func fillPath(area []float32, segments []PathSegment, backdrop int32, evenOdd bool) {
	// Initialize area with backdrop
	backdropF := float32(backdrop)
	for i := range area {
		area[i] = backdropF
	}

	for _, segment := range segments {
		delta := [2]float32{
			segment.Point1[0] - segment.Point0[0],
			segment.Point1[1] - segment.Point0[1],
		}
		for yi := 0; yi < TileHeight; yi++ {
			// fine.rs line 64: let y = segment.point0[1] - (y_tile + yi as f32);
			// Since our segments are tile-relative and we process locally, y_tile = 0
			y := segment.Point0[1] - float32(yi)
			y0 := clamp32(y, 0.0, 1.0)
			y1 := clamp32(y+delta[1], 0.0, 1.0)
			dy := y0 - y1

			// fine.rs line 68-69: y_edge = signum(delta.x) * clamp(y_tile + yi - y_edge + 1.0, 0, 1)
			// With y_tile = 0 (tile-relative): y_edge_contrib = signum(delta.x) * clamp(yi - segment.y_edge + 1, 0, 1)
			yEdge := signum32(delta[0]) * clamp32(float32(yi)-segment.YEdge+1.0, 0.0, 1.0)

			if dy != 0.0 {
				vecYRecip := 1.0 / delta[1]
				t0 := (y0 - y) * vecYRecip
				t1 := (y1 - y) * vecYRecip
				startx := segment.Point0[0] // x_tile = 0 for tile-relative
				x0 := startx + t0*delta[0]
				x1 := startx + t1*delta[0]
				xmin0 := min32(x0, x1)
				xmax0 := max32(x0, x1)
				for i := 0; i < TileWidth; i++ {
					iF := float32(i)
					xmin := min32(xmin0-iF, 1.0) - 1.0e-6
					xmax := xmax0 - iF
					b := min32(xmax, 1.0)
					c := max32(b, 0.0)
					d := max32(xmin, 0.0)
					a := (b + 0.5*(d*d-c*c) - xmin) / (xmax - xmin)
					area[yi*TileWidth+i] += yEdge + a*dy
				}
			} else if yEdge != 0.0 {
				for i := 0; i < TileWidth; i++ {
					area[yi*TileWidth+i] += yEdge
				}
			}
		}
	}

	// Apply fill rule
	if evenOdd {
		for i := range area {
			// fine.rs line 99: *a = (*a - 2.0 * (0.5 * *a).round()).abs()
			area[i] = abs32(area[i] - 2.0*round32(0.5*area[i]))
		}
	} else {
		for i := range area {
			// fine.rs line 105: *a = a.abs().min(1.0)
			area[i] = min32(abs32(area[i]), 1.0)
		}
	}
}
