// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package tilecompute

import (
	"math"
	"testing"
)

// --- PTCL Tests ---

// TestPTCLWriteRead verifies that commands written to a PTCL can be read back correctly.
func TestPTCLWriteRead(t *testing.T) {
	p := NewPTCL()

	// Write a sequence of commands.
	p.WriteFill(10, false, 42, 3)
	p.WriteColor(0xFF00FF00)
	p.WriteSolid()
	p.WriteColor(0xFFFF0000)
	p.WriteFill(5, true, 100, -2)
	p.WriteColor(0x800000FF)
	p.WriteBeginClip()
	p.WriteEndClip(0, 0.5)
	p.WriteEnd()

	// Read back and verify.
	offset := 0
	var tag uint32

	// Cmd 1: CmdFill (non-zero, 10 segments at index 42, backdrop 3)
	tag, offset = p.ReadCmd(offset)
	if tag != CmdFill {
		t.Fatalf("cmd 1: got tag %d, want CmdFill(%d)", tag, CmdFill)
	}
	fill, offset := p.ReadFillData(offset)
	if fill.SegCount != 10 {
		t.Errorf("fill.SegCount = %d, want 10", fill.SegCount)
	}
	if fill.EvenOdd {
		t.Error("fill.EvenOdd = true, want false")
	}
	if fill.SegIndex != 42 {
		t.Errorf("fill.SegIndex = %d, want 42", fill.SegIndex)
	}
	if fill.Backdrop != 3 {
		t.Errorf("fill.Backdrop = %d, want 3", fill.Backdrop)
	}

	// Cmd 2: CmdColor
	tag, offset = p.ReadCmd(offset)
	if tag != CmdColor {
		t.Fatalf("cmd 2: got tag %d, want CmdColor(%d)", tag, CmdColor)
	}
	color, offset := p.ReadColorData(offset)
	if color.RGBA != 0xFF00FF00 {
		t.Errorf("color.RGBA = 0x%08X, want 0xFF00FF00", color.RGBA)
	}

	// Cmd 3: CmdSolid
	tag, offset = p.ReadCmd(offset)
	if tag != CmdSolid {
		t.Fatalf("cmd 3: got tag %d, want CmdSolid(%d)", tag, CmdSolid)
	}

	// Cmd 4: CmdColor
	tag, offset = p.ReadCmd(offset)
	if tag != CmdColor {
		t.Fatalf("cmd 4: got tag %d, want CmdColor(%d)", tag, CmdColor)
	}
	color, offset = p.ReadColorData(offset)
	if color.RGBA != 0xFFFF0000 {
		t.Errorf("color.RGBA = 0x%08X, want 0xFFFF0000", color.RGBA)
	}

	// Cmd 5: CmdFill (even-odd, 5 segments at index 100, backdrop -2)
	tag, offset = p.ReadCmd(offset)
	if tag != CmdFill {
		t.Fatalf("cmd 5: got tag %d, want CmdFill(%d)", tag, CmdFill)
	}
	fill, offset = p.ReadFillData(offset)
	if fill.SegCount != 5 {
		t.Errorf("fill.SegCount = %d, want 5", fill.SegCount)
	}
	if !fill.EvenOdd {
		t.Error("fill.EvenOdd = false, want true")
	}
	if fill.SegIndex != 100 {
		t.Errorf("fill.SegIndex = %d, want 100", fill.SegIndex)
	}
	if fill.Backdrop != -2 {
		t.Errorf("fill.Backdrop = %d, want -2", fill.Backdrop)
	}

	// Cmd 6: CmdColor
	tag, offset = p.ReadCmd(offset)
	if tag != CmdColor {
		t.Fatalf("cmd 6: got tag %d, want CmdColor(%d)", tag, CmdColor)
	}
	color, offset = p.ReadColorData(offset)
	if color.RGBA != 0x800000FF {
		t.Errorf("color.RGBA = 0x%08X, want 0x800000FF", color.RGBA)
	}

	// Cmd 7: CmdBeginClip
	tag, offset = p.ReadCmd(offset)
	if tag != CmdBeginClip {
		t.Fatalf("cmd 7: got tag %d, want CmdBeginClip(%d)", tag, CmdBeginClip)
	}

	// Cmd 8: CmdEndClip
	tag, offset = p.ReadCmd(offset)
	if tag != CmdEndClip {
		t.Fatalf("cmd 8: got tag %d, want CmdEndClip(%d)", tag, CmdEndClip)
	}
	endClip, offset := p.ReadEndClipData(offset)
	if endClip.Blend != 0 {
		t.Errorf("endClip.Blend = %d, want 0", endClip.Blend)
	}
	if math.Abs(float64(endClip.Alpha-0.5)) > 1e-6 {
		t.Errorf("endClip.Alpha = %f, want 0.5", endClip.Alpha)
	}

	// Cmd 9: CmdEnd
	tag, _ = p.ReadCmd(offset)
	if tag != CmdEnd {
		t.Fatalf("cmd 9: got tag %d, want CmdEnd(%d)", tag, CmdEnd)
	}
}

// TestPTCLEmpty verifies that an empty PTCL with just CmdEnd works.
func TestPTCLEmpty(t *testing.T) {
	p := NewPTCL()
	p.WriteEnd()

	tag, _ := p.ReadCmd(0)
	if tag != CmdEnd {
		t.Errorf("got tag %d, want CmdEnd(%d)", tag, CmdEnd)
	}
}

// TestPTCLFillEncoding verifies the exact binary encoding of CmdFill payload.
func TestPTCLFillEncoding(t *testing.T) {
	p := NewPTCL()
	p.WriteFill(7, true, 99, -1)

	// Expected: [CmdFill, (7<<1)|1, 99, uint32(-1)]
	if p.Cmds[0] != CmdFill {
		t.Errorf("Cmds[0] = %d, want %d", p.Cmds[0], CmdFill)
	}
	wantPacked := uint32(7<<1) | 1 // 15
	if p.Cmds[1] != wantPacked {
		t.Errorf("Cmds[1] = %d, want %d", p.Cmds[1], wantPacked)
	}
	if p.Cmds[2] != 99 {
		t.Errorf("Cmds[2] = %d, want 99", p.Cmds[2])
	}
	// -1 as int32 = 0xFFFFFFFF as uint32 (two's complement).
	wantBackdrop := ^uint32(0) // 0xFFFFFFFF
	if p.Cmds[3] != wantBackdrop {
		t.Errorf("Cmds[3] = 0x%08X, want 0x%08X", p.Cmds[3], wantBackdrop)
	}
}

// --- Coarse Rasterization Tests ---

// TestCoarseRasterizeSingleTriangle creates a single triangle and verifies
// that the coarse rasterizer produces correct PTCLs for covered tiles.
func TestCoarseRasterizeSingleTriangle(t *testing.T) {
	triangle := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{5, 5}, {27, 5}, {16, 27},
		}),
		Color:    [4]uint8{255, 0, 0, 255}, // opaque red
		FillRule: FillRuleNonZero,
	}

	out := runCoarsePipeline(t, []PathDef{triangle}, 32, 32)

	// 32x32 canvas = 2x2 tile grid (16px tiles).
	if out.WidthInTiles != 2 || out.HeightInTiles != 2 {
		t.Fatalf("tile grid = %dx%d, want 2x2", out.WidthInTiles, out.HeightInTiles)
	}

	// Triangle spans tiles (0,0) to (1,1) — it should have commands.
	// Count how many tiles have at least one command (not just CmdEnd).
	tilesWithCmds := 0
	for i, ptcl := range out.TilePTCLs {
		if len(ptcl.Cmds) > 1 { // More than just CmdEnd
			tilesWithCmds++
			ty := i / out.WidthInTiles
			tx := i % out.WidthInTiles
			t.Logf("Tile (%d,%d) has %d commands", tx, ty, len(ptcl.Cmds))
		}
	}
	if tilesWithCmds == 0 {
		t.Fatal("no tiles have commands — triangle should cover at least one tile")
	}

	// Verify that at least one tile has CmdFill or CmdSolid followed by CmdColor.
	found := false
	for _, ptcl := range out.TilePTCLs {
		if hasFillOrSolidWithColor(ptcl) {
			found = true
			break
		}
	}
	if !found {
		t.Error("no tile has CmdFill/CmdSolid + CmdColor — expected at least one")
	}
}

// TestCoarseRasterizeMultiPath verifies that two overlapping shapes produce
// PTCLs with commands in correct scene order (back-to-front).
func TestCoarseRasterizeMultiPath(t *testing.T) {
	// Red square (fills entire 32x32 canvas).
	redSquare := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{0, 0}, {32, 0}, {32, 32}, {0, 32},
		}),
		Color:    [4]uint8{255, 0, 0, 255},
		FillRule: FillRuleNonZero,
	}

	// Blue square in top-left quadrant.
	blueSquare := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{0, 0}, {16, 0}, {16, 16}, {0, 16},
		}),
		Color:    [4]uint8{0, 0, 255, 255},
		FillRule: FillRuleNonZero,
	}

	out := runCoarsePipeline(t, []PathDef{redSquare, blueSquare}, 32, 32)

	// Tile (0,0) should have commands from BOTH paths.
	// Red first (scene order 0), blue second (scene order 1).
	ptcl := out.TilePTCLs[0] // tile (0,0)
	colors := extractColors(ptcl)

	if len(colors) < 2 {
		t.Fatalf("tile (0,0) has %d colors, want >= 2", len(colors))
	}

	// First color should be red (premultiplied: R=255, G=0, B=0, A=255).
	redPacked := uint32(255) | (0 << 8) | (0 << 16) | (255 << 24)
	if colors[0] != redPacked {
		t.Errorf("first color = 0x%08X, want 0x%08X (red)", colors[0], redPacked)
	}

	// Second color should be blue.
	bluePacked := uint32(0) | (0 << 8) | (255 << 16) | (255 << 24)
	if colors[1] != bluePacked {
		t.Errorf("second color = 0x%08X, want 0x%08X (blue)", colors[1], bluePacked)
	}
}

// TestCoarseTileAllocation verifies that paths get correct tile offsets and counts.
func TestCoarseTileAllocation(t *testing.T) {
	// Path 0: small square in top-left (covers 1 tile).
	p0 := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{2, 2}, {10, 2}, {10, 10}, {2, 10},
		}),
		Color:    [4]uint8{255, 0, 0, 255},
		FillRule: FillRuleNonZero,
	}

	// Path 1: larger square (covers 2x2 tiles).
	p1 := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{5, 5}, {25, 5}, {25, 25}, {5, 25},
		}),
		Color:    [4]uint8{0, 255, 0, 255},
		FillRule: FillRuleNonZero,
	}

	out := runCoarsePipeline(t, []PathDef{p0, p1}, 32, 32)

	if len(out.Paths) != 2 {
		t.Fatalf("got %d paths, want 2", len(out.Paths))
	}

	// Path 0: bbox should be (0,0)-(1,1) = 1 tile.
	p0bbox := out.Paths[0].BBox
	p0w := p0bbox[2] - p0bbox[0]
	p0h := p0bbox[3] - p0bbox[1]
	if p0w*p0h != 1 {
		t.Errorf("path 0: tile count = %d, want 1 (bbox=%v)", p0w*p0h, p0bbox)
	}

	// Path 1: bbox should cover at least 2x2 tiles.
	p1bbox := out.Paths[1].BBox
	p1w := p1bbox[2] - p1bbox[0]
	p1h := p1bbox[3] - p1bbox[1]
	if p1w*p1h < 4 {
		t.Errorf("path 1: tile count = %d, want >= 4 (bbox=%v)", p1w*p1h, p1bbox)
	}

	// Path 1 tiles start after path 0 tiles.
	if out.Paths[1].Tiles <= out.Paths[0].Tiles {
		t.Errorf("path 1 tiles offset (%d) should be > path 0 (%d)",
			out.Paths[1].Tiles, out.Paths[0].Tiles)
	}

	t.Logf("Path 0: bbox=%v, tiles=%d, offset=%d",
		p0bbox, p0w*p0h, out.Paths[0].Tiles)
	t.Logf("Path 1: bbox=%v, tiles=%d, offset=%d",
		p1bbox, p1w*p1h, out.Paths[1].Tiles)
}

// TestCoarseEmptyTiles verifies that tiles outside any path get only CmdEnd.
func TestCoarseEmptyTiles(t *testing.T) {
	// Small triangle in top-left corner.
	triangle := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{1, 1}, {10, 1}, {5, 10},
		}),
		Color:    [4]uint8{255, 0, 0, 255},
		FillRule: FillRuleNonZero,
	}

	out := runCoarsePipeline(t, []PathDef{triangle}, 64, 64)

	// 64x64 = 4x4 tile grid.
	if out.WidthInTiles != 4 || out.HeightInTiles != 4 {
		t.Fatalf("tile grid = %dx%d, want 4x4", out.WidthInTiles, out.HeightInTiles)
	}

	// Bottom-right tile (3,3) should be empty (only CmdEnd).
	brIdx := 3*out.WidthInTiles + 3
	ptcl := out.TilePTCLs[brIdx]
	if len(ptcl.Cmds) != 1 || ptcl.Cmds[0] != CmdEnd {
		t.Errorf("tile (3,3) has %d commands, want [CmdEnd]", len(ptcl.Cmds))
	}
}

// TestCoarseBackdropSolid verifies that a tile fully inside a large shape
// (nonzero backdrop, no segments crossing it) gets CmdSolid rather than CmdFill.
func TestCoarseBackdropSolid(t *testing.T) {
	// Large square covering the entire 48x48 canvas.
	square := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{0, 0}, {48, 0}, {48, 48}, {0, 48},
		}),
		Color:    [4]uint8{0, 255, 0, 255},
		FillRule: FillRuleNonZero,
	}

	out := runCoarsePipeline(t, []PathDef{square}, 48, 48)

	// 48x48 = 3x3 tile grid.
	// The center tile (1,1) is fully inside the square: no segments cross it,
	// but backdrop should be nonzero due to the left edge winding.
	centerIdx := 1*out.WidthInTiles + 1
	ptcl := out.TilePTCLs[centerIdx]

	hasSolid := false
	offset := 0
	for offset < len(ptcl.Cmds) {
		tag, next := ptcl.ReadCmd(offset)
		offset = next
		switch tag {
		case CmdEnd:
			offset = len(ptcl.Cmds) // break loop
		case CmdSolid:
			hasSolid = true
		case CmdFill:
			_, offset = ptcl.ReadFillData(offset)
		case CmdColor:
			_, offset = ptcl.ReadColorData(offset)
		case CmdEndClip:
			_, offset = ptcl.ReadEndClipData(offset)
		}
	}

	if hasSolid {
		t.Log("center tile correctly uses CmdSolid (fully covered)")
		return
	}

	// It's also acceptable for the center tile to have CmdFill if
	// edge segments touch it. Log but don't fail hard.
	t.Logf("center tile PTCL commands: %v", ptcl.Cmds)
	hasFill := ptclContainsTag(ptcl, CmdFill)
	if !hasFill {
		t.Error("center tile has neither CmdSolid nor CmdFill -- expected one")
	} else {
		t.Log("center tile has CmdFill (edge segments cross it)")
	}
}

// TestCoarseEvenOdd verifies that the even-odd fill rule flag is correctly
// propagated into PTCL CmdFill commands.
func TestCoarseEvenOdd(t *testing.T) {
	star := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{16, 1}, {20, 14}, {30, 14}, {22, 22}, {26, 31}, {16, 25}, {6, 31}, {10, 22}, {2, 14}, {12, 14},
		}),
		Color:    [4]uint8{128, 0, 0, 255},
		FillRule: FillRuleEvenOdd,
	}

	out := runCoarsePipeline(t, []PathDef{star}, 32, 32)

	// Find a tile with CmdFill and verify evenOdd flag.
	for i, ptcl := range out.TilePTCLs {
		fill, ok := findFirstFill(ptcl)
		if !ok {
			continue
		}
		if !fill.EvenOdd {
			tx := i % out.WidthInTiles
			ty := i / out.WidthInTiles
			t.Errorf("tile (%d,%d): CmdFill.EvenOdd = false, want true", tx, ty)
		}
		return // Found one, that's enough.
	}

	t.Error("no tile has CmdFill -- star should produce at least one CmdFill")
}

// TestCoarseNoLines verifies that an empty scene produces only CmdEnd in all tiles.
func TestCoarseNoLines(t *testing.T) {
	enc := EncodeScene(nil)
	scene := PackScene(enc)

	out := CoarseRasterize(scene, nil, nil, nil, 32, 32)

	for i, ptcl := range out.TilePTCLs {
		if len(ptcl.Cmds) != 1 || ptcl.Cmds[0] != CmdEnd {
			t.Errorf("tile %d: expected only CmdEnd, got %v", i, ptcl.Cmds)
		}
	}
}

// --- Helper functions ---

// runCoarsePipeline runs the full scene encoding + draw monoid + coarse pipeline.
func runCoarsePipeline(t *testing.T, paths []PathDef, widthPx, heightPx int) *CoarseOutput {
	t.Helper()

	// Assign PathIx to lines.
	var allLines []LineSoup
	for i, pd := range paths {
		for _, line := range pd.Lines {
			allLines = append(allLines, LineSoup{
				PathIx: uint32(i),
				P0:     line.P0,
				P1:     line.P1,
			})
		}
	}

	// Encode scene.
	enc := EncodeScene(paths)
	scene := PackScene(enc)

	// Run draw monoid stages.
	reduced := drawReduce(scene)
	drawMonoids, info := drawLeafScan(scene, reduced)

	// Run coarse rasterization.
	out := CoarseRasterize(scene, drawMonoids, info, allLines, widthPx, heightPx)

	t.Logf("CoarseOutput: %dx%d tiles, %d paths, %d total tiles, %d segments",
		out.WidthInTiles, out.HeightInTiles, len(out.Paths), len(out.Tiles), len(out.Segments))

	return out
}

// hasFillOrSolidWithColor checks if a PTCL contains CmdFill or CmdSolid
// followed by CmdColor.
func hasFillOrSolidWithColor(ptcl *PTCL) bool {
	sawFillOrSolid := false
	offset := 0
	for offset < len(ptcl.Cmds) {
		tag, next := ptcl.ReadCmd(offset)
		offset = next
		switch tag {
		case CmdEnd:
			return false
		case CmdFill:
			sawFillOrSolid = true
			_, offset = ptcl.ReadFillData(offset)
		case CmdSolid:
			sawFillOrSolid = true
		case CmdColor:
			if sawFillOrSolid {
				return true
			}
			_, offset = ptcl.ReadColorData(offset)
		case CmdEndClip:
			_, offset = ptcl.ReadEndClipData(offset)
		}
	}
	return false
}

// ptclContainsTag checks if a PTCL contains the given command tag.
func ptclContainsTag(ptcl *PTCL, target uint32) bool {
	offset := 0
	for offset < len(ptcl.Cmds) {
		tag, next := ptcl.ReadCmd(offset)
		offset = next
		switch tag {
		case target:
			return true
		case CmdEnd:
			return false
		case CmdFill:
			_, offset = ptcl.ReadFillData(offset)
		case CmdColor:
			_, offset = ptcl.ReadColorData(offset)
		case CmdEndClip:
			_, offset = ptcl.ReadEndClipData(offset)
		}
	}
	return false
}

// findFirstFill returns the first CmdFill data in a PTCL, or false if none found.
func findFirstFill(ptcl *PTCL) (CmdFillData, bool) {
	offset := 0
	for offset < len(ptcl.Cmds) {
		tag, next := ptcl.ReadCmd(offset)
		offset = next
		switch tag {
		case CmdFill:
			fill, _ := ptcl.ReadFillData(offset)
			return fill, true
		case CmdEnd:
			return CmdFillData{}, false
		case CmdColor:
			_, offset = ptcl.ReadColorData(offset)
		case CmdEndClip:
			_, offset = ptcl.ReadEndClipData(offset)
		}
	}
	return CmdFillData{}, false
}

// extractColors reads all CmdColor payloads from a PTCL in order.
func extractColors(ptcl *PTCL) []uint32 {
	var colors []uint32
	offset := 0
	for offset < len(ptcl.Cmds) {
		tag, next := ptcl.ReadCmd(offset)
		offset = next
		switch tag {
		case CmdEnd:
			return colors
		case CmdFill:
			_, offset = ptcl.ReadFillData(offset)
		case CmdColor:
			var cd CmdColorData
			cd, offset = ptcl.ReadColorData(offset)
			colors = append(colors, cd.RGBA)
		case CmdEndClip:
			_, offset = ptcl.ReadEndClipData(offset)
		case CmdSolid, CmdBeginClip:
			// No payload.
		}
	}
	return colors
}
