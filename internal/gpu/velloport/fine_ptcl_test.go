// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package velloport

import (
	"math"
	"testing"
)

// TestFineRasterizeTileEmpty verifies that an empty PTCL (just CmdEnd)
// returns the background color for all pixels.
func TestFineRasterizeTileEmpty(t *testing.T) {
	ptcl := NewPTCL()
	ptcl.WriteEnd()

	bg := [4]float32{0.2, 0.4, 0.6, 1.0}
	result := fineRasterizeTile(ptcl, nil, bg)

	for i := 0; i < TileWidth*TileHeight; i++ {
		if result[i] != bg {
			t.Fatalf("pixel %d: got %v, want %v", i, result[i], bg)
		}
	}
}

// TestFineRasterizeTileSolidColor verifies that CmdSolid + CmdColor
// produces a solid fill covering the entire tile.
func TestFineRasterizeTileSolidColor(t *testing.T) {
	ptcl := NewPTCL()
	ptcl.WriteSolid()
	// Opaque red premultiplied: R=255, G=0, B=0, A=255
	ptcl.WriteColor(0xFF0000FF) // packed: R | G<<8 | B<<16 | A<<24
	ptcl.WriteEnd()

	bg := [4]float32{0, 0, 0, 1.0} // black background

	result := fineRasterizeTile(ptcl, nil, bg)

	// After CmdSolid, area = 1.0 everywhere.
	// CmdColor with opaque red (premul: r=1, g=0, b=0, a=1):
	// fg = color * area = (1,0,0,1) * 1 = (1,0,0,1)
	// source-over: bg * (1-1) + (1,0,0,1) = (1,0,0,1)
	for i := 0; i < TileWidth*TileHeight; i++ {
		r := result[i][0]
		g := result[i][1]
		b := result[i][2]
		a := result[i][3]
		if math.Abs(float64(r-1.0)) > 1e-4 || math.Abs(float64(g)) > 1e-4 ||
			math.Abs(float64(b)) > 1e-4 || math.Abs(float64(a-1.0)) > 1e-4 {
			t.Fatalf("pixel %d: got [%.4f, %.4f, %.4f, %.4f], want [1, 0, 0, 1]",
				i, r, g, b, a)
		}
	}
}

// TestFineRasterizeTileFillColor verifies CmdFill with known segments
// followed by CmdColor. Uses a triangle that covers part of the tile to verify
// that fillPath is correctly invoked through the PTCL command loop.
func TestFineRasterizeTileFillColor(t *testing.T) {
	// Create a small triangle inside the tile using tile-relative coordinates.
	// Triangle: (2,2) -> (14,8) -> (2,14).
	// The segments must be tile-relative for fillPath.
	// Using the winding number approach: downward-going left edge creates
	// backdrop=1 for interior tiles to the right.
	// For a single tile, we use backdrop=0 and segments that form a closed shape.
	//
	// Create line segments for a right-pointing triangle.
	// Line 1: (2,2) -> (14,8) — top edge going down-right
	// Line 2: (14,8) -> (2,14) — bottom edge going down-left
	// Line 3: (2,14) -> (2,2)  — left edge going up (closing)
	segments := []PathSegment{
		{Point0: [2]float32{2, 2}, Point1: [2]float32{14, 8}, YEdge: 1e9},
		{Point0: [2]float32{14, 8}, Point1: [2]float32{2, 14}, YEdge: 1e9},
		{Point0: [2]float32{2, 14}, Point1: [2]float32{2, 2}, YEdge: 1e9},
	}

	ptcl := NewPTCL()
	ptcl.WriteFill(3, false, 0, 0) // 3 segments, non-zero, segIndex=0, backdrop=0
	// Opaque green premultiplied: R=0, G=255, B=0, A=255
	ptcl.WriteColor(0xFF00FF00) // packed: R=0 | G=255<<8 | B=0<<16 | A=255<<24
	ptcl.WriteEnd()

	bg := [4]float32{1, 1, 1, 1} // white background
	result := fineRasterizeTile(ptcl, segments, bg)

	// Check a pixel inside the triangle (x=6, y=8, center-ish).
	// Should be heavily green since coverage > 0 inside the shape.
	insidePx := result[8*TileWidth+6]
	if insidePx[1] < 0.5 {
		t.Errorf("inside pixel (6,8): got G=%.4f, want > 0.5 (green-ish)", insidePx[1])
	}

	// Check a pixel outside the triangle (x=0, y=0, top-left corner).
	// Should remain white background since it is outside the shape.
	outsidePx := result[0*TileWidth+0]
	if outsidePx[0] < 0.9 || outsidePx[1] < 0.9 || outsidePx[2] < 0.9 {
		t.Errorf("outside pixel (0,0): got [%.4f, %.4f, %.4f], want ~[1,1,1] (white bg)",
			outsidePx[0], outsidePx[1], outsidePx[2])
	}

	// Check another pixel clearly outside (x=15, y=1).
	outsidePx2 := result[1*TileWidth+15]
	if outsidePx2[0] < 0.9 || outsidePx2[1] < 0.9 || outsidePx2[2] < 0.9 {
		t.Errorf("outside pixel (15,1): got [%.4f, %.4f, %.4f], want ~[1,1,1] (white bg)",
			outsidePx2[0], outsidePx2[1], outsidePx2[2])
	}
}

// TestFineRasterizeTileMultiShape verifies that two shapes are composited
// correctly in the PTCL command stream.
func TestFineRasterizeTileMultiShape(t *testing.T) {
	// Shape 1: Solid red fill.
	// Shape 2: Solid semi-transparent blue over it.

	ptcl := NewPTCL()
	// Shape 1: solid red
	ptcl.WriteSolid()
	ptcl.WriteColor(0xFF0000FF) // R=255, G=0, B=0, A=255 premul

	// Shape 2: solid semi-transparent blue (A=128 => premul B=128, A=128)
	ptcl.WriteSolid()
	// Blue premultiplied: R=0, G=0, B=128, A=128 -> packed: 0x80800000
	ptcl.WriteColor(0x80800000) // R=0 | G=0<<8 | B=128<<16 | A=128<<24
	ptcl.WriteEnd()

	bg := [4]float32{0, 0, 0, 1.0}
	result := fineRasterizeTile(ptcl, nil, bg)

	// After shape 1 (solid red): pixel = (1, 0, 0, 1)
	// After shape 2 (50% blue): fg = (0, 0, 128/255, 128/255) * 1.0
	//   fgA = 128/255 ~= 0.502
	//   inv = 1 - 0.502 = 0.498
	//   r = 1.0 * 0.498 + 0 = 0.498
	//   b = 0 * 0.498 + 128/255 = 0.502
	//   a = 1.0 * 0.498 + 0.502 = 1.0
	px := result[0]
	if math.Abs(float64(px[0]-0.498)) > 0.02 {
		t.Errorf("pixel R: got %.4f, want ~0.498", px[0])
	}
	if math.Abs(float64(px[2]-0.502)) > 0.02 {
		t.Errorf("pixel B: got %.4f, want ~0.502", px[2])
	}
	if math.Abs(float64(px[3]-1.0)) > 0.02 {
		t.Errorf("pixel A: got %.4f, want ~1.0", px[3])
	}
}

// TestFineRasterizeTileClip verifies CmdBeginClip + CmdSolid + CmdColor + CmdEndClip.
func TestFineRasterizeTileClip(t *testing.T) {
	ptcl := NewPTCL()

	// Set coverage for the clip mask to 1.0 (solid).
	ptcl.WriteSolid()

	// Begin clip layer.
	ptcl.WriteBeginClip()

	// Inside clip: draw solid green.
	ptcl.WriteSolid()
	ptcl.WriteColor(0xFF00FF00) // opaque green premul

	// End clip: alpha=0.5, blend=0 (source-over).
	ptcl.WriteEndClip(0, 0.5)
	ptcl.WriteEnd()

	bg := [4]float32{1, 1, 1, 1} // white background

	result := fineRasterizeTile(ptcl, nil, bg)

	// After BeginClip: saved = white, rgba = transparent.
	// Inside clip: CmdSolid (area=1), CmdColor green -> rgba = (0, 1, 0, 1).
	// EndClip: area=1 (from previous CmdSolid before BeginClip), alpha=0.5
	//   fg = rgba * area * alpha = (0, 1, 0, 1) * 1.0 * 0.5 = (0, 0.5, 0, 0.5)
	//   result = saved * (1 - 0.5) + fg = (1,1,1,1)*0.5 + (0,0.5,0,0.5) = (0.5, 1.0, 0.5, 1.0)
	px := result[0]
	if math.Abs(float64(px[0]-0.5)) > 0.02 {
		t.Errorf("pixel R: got %.4f, want ~0.5", px[0])
	}
	if math.Abs(float64(px[1]-1.0)) > 0.02 {
		t.Errorf("pixel G: got %.4f, want ~1.0", px[1])
	}
	if math.Abs(float64(px[2]-0.5)) > 0.02 {
		t.Errorf("pixel B: got %.4f, want ~0.5", px[2])
	}
	if math.Abs(float64(px[3]-1.0)) > 0.02 {
		t.Errorf("pixel A: got %.4f, want ~1.0", px[3])
	}
}

// TestRasterizeScenePTCLSinglePath runs the full PTCL pipeline for a single
// triangle path and compares with RasterizeScene.
func TestRasterizeScenePTCLSinglePath(t *testing.T) {
	triangle := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{5, 5}, {27, 5}, {16, 27},
		}),
		Color:    [4]uint8{255, 0, 0, 255},
		FillRule: FillRuleNonZero,
	}

	bg := [4]uint8{255, 255, 255, 255}
	r := NewRasterizer(32, 32)

	imgOld := r.RasterizeScene(bg, []PathDef{triangle})
	imgNew := r.RasterizeScenePTCL(bg, []PathDef{triangle})

	// Compare pixel-by-pixel with tolerance of 1/255.
	diffCount := 0
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c1 := imgOld.RGBAAt(x, y)
			c2 := imgNew.RGBAAt(x, y)
			if absDiffU8(c1.R, c2.R) > 1 || absDiffU8(c1.G, c2.G) > 1 ||
				absDiffU8(c1.B, c2.B) > 1 || absDiffU8(c1.A, c2.A) > 1 {
				diffCount++
				if diffCount <= 5 {
					t.Logf("diff at (%d,%d): old=(%d,%d,%d,%d) new=(%d,%d,%d,%d)",
						x, y, c1.R, c1.G, c1.B, c1.A, c2.R, c2.G, c2.B, c2.A)
				}
			}
		}
	}
	if diffCount > 0 {
		t.Errorf("single path: %d pixels differ (>1/255 per channel) out of %d",
			diffCount, 32*32)
	}
}

// TestRasterizeScenePTCLMultiPath tests two overlapping colored paths and verifies
// compositing matches RasterizeScene.
func TestRasterizeScenePTCLMultiPath(t *testing.T) {
	// Red square covering most of the canvas.
	redSquare := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{2, 2}, {30, 2}, {30, 30}, {2, 30},
		}),
		Color:    [4]uint8{255, 0, 0, 255},
		FillRule: FillRuleNonZero,
	}

	// Semi-transparent blue triangle overlapping.
	blueTriangle := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{5, 5}, {27, 16}, {5, 27},
		}),
		Color:    [4]uint8{0, 0, 255, 180},
		FillRule: FillRuleNonZero,
	}

	bg := [4]uint8{200, 200, 200, 255}
	r := NewRasterizer(32, 32)

	imgOld := r.RasterizeScene(bg, []PathDef{redSquare, blueTriangle})
	imgNew := r.RasterizeScenePTCL(bg, []PathDef{redSquare, blueTriangle})

	diffCount := 0
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c1 := imgOld.RGBAAt(x, y)
			c2 := imgNew.RGBAAt(x, y)
			if absDiffU8(c1.R, c2.R) > 1 || absDiffU8(c1.G, c2.G) > 1 ||
				absDiffU8(c1.B, c2.B) > 1 || absDiffU8(c1.A, c2.A) > 1 {
				diffCount++
				if diffCount <= 5 {
					t.Logf("diff at (%d,%d): old=(%d,%d,%d,%d) new=(%d,%d,%d,%d)",
						x, y, c1.R, c1.G, c1.B, c1.A, c2.R, c2.G, c2.B, c2.A)
				}
			}
		}
	}
	if diffCount > 0 {
		t.Errorf("multi path: %d pixels differ (>1/255 per channel) out of %d",
			diffCount, 32*32)
	}
}

// TestRasterizeScenePTCLMatchesExisting is the CRITICAL test that runs both
// RasterizeScene() and RasterizeScenePTCL() on the same input and verifies
// they produce matching results (within 1/255 tolerance per channel).
func TestRasterizeScenePTCLMatchesExisting(t *testing.T) {
	tests := []struct {
		name  string
		bg    [4]uint8
		paths []PathDef
		w, h  int
	}{
		{
			name: "two_opaque_squares",
			bg:   [4]uint8{255, 255, 255, 255},
			w:    32, h: 32,
			paths: []PathDef{
				{
					Lines: polygonToLineSoup([][2]float32{
						{0, 0}, {20, 0}, {20, 20}, {0, 20},
					}),
					Color:    [4]uint8{255, 0, 0, 255},
					FillRule: FillRuleNonZero,
				},
				{
					Lines: polygonToLineSoup([][2]float32{
						{10, 10}, {30, 10}, {30, 30}, {10, 30},
					}),
					Color:    [4]uint8{0, 0, 255, 255},
					FillRule: FillRuleNonZero,
				},
			},
		},
		{
			name: "three_overlapping_semitransparent",
			bg:   [4]uint8{128, 128, 128, 255},
			w:    32, h: 32,
			paths: []PathDef{
				{
					Lines: polygonToLineSoup([][2]float32{
						{2, 2}, {28, 2}, {28, 20}, {2, 20},
					}),
					Color:    [4]uint8{255, 0, 0, 200},
					FillRule: FillRuleNonZero,
				},
				{
					Lines: polygonToLineSoup([][2]float32{
						{5, 10}, {27, 10}, {27, 28}, {5, 28},
					}),
					Color:    [4]uint8{0, 255, 0, 150},
					FillRule: FillRuleNonZero,
				},
				{
					Lines: polygonToLineSoup([][2]float32{
						{8, 5}, {24, 5}, {24, 25}, {8, 25},
					}),
					Color:    [4]uint8{0, 0, 255, 100},
					FillRule: FillRuleNonZero,
				},
			},
		},
		{
			name: "triangle_and_square",
			bg:   [4]uint8{0, 0, 0, 255},
			w:    32, h: 32,
			paths: []PathDef{
				{
					Lines: polygonToLineSoup([][2]float32{
						{3, 3}, {29, 3}, {29, 29}, {3, 29},
					}),
					Color:    [4]uint8{0, 128, 255, 255},
					FillRule: FillRuleNonZero,
				},
				{
					Lines: polygonToLineSoup([][2]float32{
						{5, 25}, {16, 5}, {27, 25},
					}),
					Color:    [4]uint8{255, 200, 0, 180},
					FillRule: FillRuleNonZero,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRasterizer(tc.w, tc.h)
			imgOld := r.RasterizeScene(tc.bg, tc.paths)
			imgNew := r.RasterizeScenePTCL(tc.bg, tc.paths)

			diffCount := 0
			maxDiff := uint8(0)
			for y := 0; y < tc.h; y++ {
				for x := 0; x < tc.w; x++ {
					c1 := imgOld.RGBAAt(x, y)
					c2 := imgNew.RGBAAt(x, y)
					dR := absDiffU8(c1.R, c2.R)
					dG := absDiffU8(c1.G, c2.G)
					dB := absDiffU8(c1.B, c2.B)
					dA := absDiffU8(c1.A, c2.A)

					mx := dR
					if dG > mx {
						mx = dG
					}
					if dB > mx {
						mx = dB
					}
					if dA > mx {
						mx = dA
					}
					if mx > maxDiff {
						maxDiff = mx
					}

					if mx > 1 {
						diffCount++
						if diffCount <= 3 {
							t.Logf("diff at (%d,%d): old=(%d,%d,%d,%d) new=(%d,%d,%d,%d) delta=(%d,%d,%d,%d)",
								x, y, c1.R, c1.G, c1.B, c1.A, c2.R, c2.G, c2.B, c2.A, dR, dG, dB, dA)
						}
					}
				}
			}

			total := tc.w * tc.h
			t.Logf("scene %q: %d/%d pixels differ (>1), maxDiff=%d", tc.name, diffCount, total, maxDiff)

			if diffCount > 0 {
				t.Errorf("PTCL pipeline does not match existing pipeline: %d pixels differ (>1/255)", diffCount)
			}
		})
	}
}

// TestFineRasterizeTileNilPTCL verifies that a nil PTCL returns background color.
func TestFineRasterizeTileNilPTCL(t *testing.T) {
	bg := [4]float32{0.5, 0.5, 0.5, 1.0}
	result := fineRasterizeTile(nil, nil, bg)

	for i := 0; i < TileWidth*TileHeight; i++ {
		if result[i] != bg {
			t.Fatalf("pixel %d: got %v, want %v", i, result[i], bg)
		}
	}
}

// absDiffU8 returns the absolute difference between two uint8 values.
func absDiffU8(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
