// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Direct port of fill_path from vello_shaders/src/cpu/fine.rs (lines 51-109).
// Variable names match Rust originals for cross-reference.

package velloport

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
