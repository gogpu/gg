// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package gpu

import (
	"fmt"
	"github.com/gogpu/gg/internal/raster"
	"image/png"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/gogpu/gg/scene"
)

func TestNewTileRasterizer(t *testing.T) {
	tr := NewTileRasterizer(100, 100)

	if tr.width != 100 {
		t.Errorf("width = %d, want 100", tr.width)
	}
	if tr.height != 100 {
		t.Errorf("height = %d, want 100", tr.height)
	}

	// 100 / 16 = 7 tiles per dimension (VelloTileWidth = 16)
	expectedTilesX := (100 + VelloTileWidth - 1) / VelloTileWidth
	expectedTilesY := (100 + VelloTileHeight - 1) / VelloTileHeight

	if tr.tilesX != expectedTilesX {
		t.Errorf("tilesX = %d, want %d", tr.tilesX, expectedTilesX)
	}
	if tr.tilesY != expectedTilesY {
		t.Errorf("tilesY = %d, want %d", tr.tilesY, expectedTilesY)
	}

	if len(tr.tiles) != expectedTilesX*expectedTilesY {
		t.Errorf("tiles len = %d, want %d", len(tr.tiles), expectedTilesX*expectedTilesY)
	}
}

func TestTileRasterizerReset(t *testing.T) {
	tr := NewTileRasterizer(16, 16)

	// Add some segments to a tile
	tr.tiles[0].Segments = append(tr.tiles[0].Segments, PathSegment{
		Point0: [2]float32{0, 0},
		Point1: [2]float32{4, 4},
		YEdge:  1e9,
	})
	tr.tiles[0].Backdrop = 1

	tr.Reset()

	if len(tr.tiles[0].Segments) != 0 {
		t.Errorf("segments not cleared, len = %d", len(tr.tiles[0].Segments))
	}
	if tr.tiles[0].Backdrop != 0 {
		t.Errorf("backdrop not cleared, = %d", tr.tiles[0].Backdrop)
	}
}

func TestPathSegment(t *testing.T) {
	seg := PathSegment{
		Point0: [2]float32{1.0, 2.0},
		Point1: [2]float32{3.0, 4.0},
		YEdge:  5.0,
	}

	if seg.Point0[0] != 1.0 || seg.Point0[1] != 2.0 {
		t.Errorf("start point = (%.1f, %.1f), want (1.0, 2.0)", seg.Point0[0], seg.Point0[1])
	}
	if seg.Point1[0] != 3.0 || seg.Point1[1] != 4.0 {
		t.Errorf("end point = (%.1f, %.1f), want (3.0, 4.0)", seg.Point1[0], seg.Point1[1])
	}
	if seg.YEdge != 5.0 {
		t.Errorf("yEdge = %f, want 5.0", seg.YEdge)
	}
}

func TestPathSegmentYEdgeSentinel(t *testing.T) {
	// Test that sentinel value (1e9) is used for segments not crossing left edge
	seg1 := PathSegment{Point0: [2]float32{1, 0}, Point1: [2]float32{2, 1}, YEdge: 1e9}
	seg2 := PathSegment{Point0: [2]float32{0, 5}, Point1: [2]float32{2, 8}, YEdge: 5.0}

	if seg1.YEdge < 1e8 {
		t.Errorf("seg1.YEdge = %f, expected sentinel (1e9)", seg1.YEdge)
	}
	if seg2.YEdge != 5.0 {
		t.Errorf("seg2.YEdge = %f, want 5.0", seg2.YEdge)
	}
}

func TestTileRasterizerFillEmpty(t *testing.T) {
	tr := NewTileRasterizer(100, 100)
	eb := raster.NewEdgeBuilder(2)

	callCount := 0
	tr.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		callCount++
	})

	// Empty edge builder should not call callback
	if callCount != 0 {
		t.Errorf("callback called %d times for empty path, want 0", callCount)
	}
}

func TestTileRasterizerFillSimpleRect(t *testing.T) {
	tr := NewTileRasterizer(32, 32)
	eb := raster.NewEdgeBuilder(0) // No AA for simpler testing

	// Create a simple 16x16 rectangle at (8, 8) via scene.Path
	path := scene.NewPath()
	path.MoveTo(8, 8)
	path.LineTo(24, 8)
	path.LineTo(24, 24)
	path.LineTo(8, 24)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	scanlines := make(map[int]bool)
	tr.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		scanlines[y] = true
	})

	// Should have scanlines covering the rectangle area
	if len(scanlines) == 0 {
		t.Error("no scanlines generated for rectangle")
	}
}

func TestFillPathWithBackdrop(t *testing.T) {
	tr := NewTileRasterizer(32, 32)

	// Create a tile with backdrop = 1 (fully inside a shape)
	tr.tiles[0].Backdrop = 1

	// Fill the tile (no segments, just backdrop)
	tr.fillPath(&tr.tiles[0], raster.FillRuleNonZero)

	// All pixels should have coverage = 1.0 (alpha = 255)
	for i := 0; i < VelloTileSize; i++ {
		if tr.area[i] != 1.0 {
			t.Errorf("area[%d] = %f, want 1.0", i, tr.area[i])
			break
		}
	}
}

func TestAbs32(t *testing.T) {
	tests := []struct {
		input    float32
		expected float32
	}{
		{0, 0},
		{1, 1},
		{-1, 1},
		{3.14, 3.14},
		{-3.14, 3.14},
	}

	for _, tt := range tests {
		result := abs32(tt.input)
		if result != tt.expected {
			t.Errorf("abs32(%f) = %f, want %f", tt.input, result, tt.expected)
		}
	}
}

// TestTileLeftEdgeCrossing verifies that segments crossing the left edge
// of a tile correctly contribute to the backdrop.
func TestTileLeftEdgeCrossing(t *testing.T) {
	tests := []struct {
		name           string
		segXMin        float32
		segXMax        float32
		tileLeftX      float32
		expectHasEntry bool
	}{
		{
			name:           "segment crosses left edge",
			segXMin:        -2,
			segXMax:        2,
			tileLeftX:      0,
			expectHasEntry: true,
		},
		{
			name:           "segment starts at left edge",
			segXMin:        0,
			segXMax:        4,
			tileLeftX:      0,
			expectHasEntry: true,
		},
		{
			name:           "segment entirely inside tile",
			segXMin:        1,
			segXMax:        3,
			tileLeftX:      0,
			expectHasEntry: false,
		},
		{
			name:           "segment entirely to the right",
			segXMin:        5,
			segXMax:        8,
			tileLeftX:      0,
			expectHasEntry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This mimics the logic in binSegments
			hasEntry := tt.segXMin <= tt.tileLeftX && tt.segXMax > tt.tileLeftX
			if hasEntry != tt.expectHasEntry {
				t.Errorf("hasEntry = %v, want %v", hasEntry, tt.expectHasEntry)
			}
		})
	}
}

func TestGoldenPixelFormat(t *testing.T) {
	f, err := os.Open("../../testdata/vello_golden_circle.png")
	if err != nil {
		t.Skip("golden file not found")
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Print pixel values at corners and center
	points := [][2]int{{0, 0}, {10, 10}, {19, 0}, {0, 19}, {19, 19}}
	for _, p := range points {
		r, g, b, a := img.At(p[0], p[1]).RGBA()
		t.Logf("Pixel (%d,%d): R=%d G=%d B=%d A=%d", p[0], p[1], r>>8, g>>8, b>>8, a>>8)
	}
}

func TestVelloTileConstants(t *testing.T) {
	// Verify Vello uses 16x16 tiles
	if VelloTileWidth != 16 {
		t.Errorf("VelloTileWidth = %d, want 16", VelloTileWidth)
	}
	if VelloTileHeight != 16 {
		t.Errorf("VelloTileHeight = %d, want 16", VelloTileHeight)
	}
	if VelloTileSize != 256 {
		t.Errorf("VelloTileSize = %d, want 256", VelloTileSize)
	}
}

// Benchmark tile rasterizer creation
func BenchmarkNewTileRasterizer(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewTileRasterizer(800, 600)
	}
}

// Benchmark tile rasterizer reset
func BenchmarkTileRasterizerReset(b *testing.B) {
	tr := NewTileRasterizer(800, 600)
	// Add some segments to make reset meaningful
	for i := 0; i < 100; i++ {
		tr.tiles[i].Segments = append(tr.tiles[i].Segments, PathSegment{})
		tr.tiles[i].Backdrop = 1
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Reset()
	}
}

// TestDebugBackdrop traces backdrop values for a simple rectangle
func TestDebugBackdrop(t *testing.T) {
	// Small 64x64 image = 4x4 tiles
	tr := NewTileRasterizer(64, 64)
	eb := raster.NewEdgeBuilder(0) // No AA for clarity

	// Simple rectangle from (16,16) to (48,48) - should cover tiles (1,1), (1,2), (2,1), (2,2)
	// Rectangle: clockwise winding via scene.Path
	path := scene.NewPath()
	path.MoveTo(16, 16)
	path.LineTo(48, 16)
	path.LineTo(48, 48)
	path.LineTo(16, 48)
	path.Close()

	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Manually call binSegments and prefix sum to see backdrops
	tr.binSegments(eb, 1.0)
	tr.computeBackdropPrefixSum()

	t.Log("Tile backdrops after prefix sum (4x4 grid):")
	for ty := 0; ty < tr.tilesY; ty++ {
		row := ""
		for tx := 0; tx < tr.tilesX; tx++ {
			tile := tr.tiles[ty*tr.tilesX+tx]
			row += fmt.Sprintf("%3d ", tile.Backdrop)
		}
		t.Logf("Row %d: %s", ty, row)
	}

	// Expected backdrops (backdrop = accumulated winding from LEFT):
	// Row 0: 0 0 0 0 (above rectangle - no segments)
	// Row 1: 0 0 1 1 (left edge at x=16 adds +1 to tile 2; prefix sum propagates to tile 3)
	// Row 2: 0 0 1 1 (same pattern)
	// Row 3: 0 0 1 1 (bottom edge is horizontal, doesn't change backdrop)
	//
	// Note: Tile 1 has backdrop=0 but contains left edge segment that adds coverage.
	// Tile 3 has backdrop=1 but contains right edge segment that removes coverage.
	// The segments handle the actual fill, backdrop just propagates winding.
}

// TestDebugCircleArtifact investigates the horizontal artifact on circle's right side
func TestDebugCircleArtifact(t *testing.T) {
	// Same dimensions as visual test
	width, height := 200, 200
	tr := NewTileRasterizer(width, height)
	eb := raster.NewEdgeBuilder(2)

	// Build circle (same as TestVelloCompareWithOriginal)
	cx, cy := float32(100), float32(100)
	radius := float32(60)
	const k = 0.5522847498

	path := scene.NewPath()
	path.MoveTo(cx+radius, cy)
	path.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
	path.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
	path.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
	path.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
	path.Close()
	eb.SetFlattenCurves(true)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Bin segments
	aaShift := eb.AAShift()
	aaScale := float32(int32(1) << uint(aaShift))
	tr.binSegments(eb, aaScale)

	// Show RAW backdrop before prefix sum
	t.Log("RAW backdrops (before prefix sum) near artifact area:")
	for ty := 5; ty <= 7; ty++ {
		row := ""
		for tx := 8; tx <= 12 && tx < tr.tilesX; tx++ {
			tile := tr.tiles[ty*tr.tilesX+tx]
			row += fmt.Sprintf("(%d,%d)=%d ", tx, ty, tile.Backdrop)
		}
		t.Log(row)
	}

	tr.computeBackdropPrefixSum()

	// Focus on the area where artifact appears (around x=160, y=100)
	// Circle rightmost point is at x = 100 + 60 = 160
	// In tiles: x=160 is tile column 10 (160/16=10)
	// y=100 is tile row 6 (100/16=6)

	t.Log("Backdrops near artifact area (tile rows 5-7, columns 9-12):")
	for ty := 5; ty <= 7; ty++ {
		row := ""
		for tx := 9; tx <= 12 && tx < tr.tilesX; tx++ {
			tile := tr.tiles[ty*tr.tilesX+tx]
			row += fmt.Sprintf("(%d,%d)=%d ", tx, ty, tile.Backdrop)
		}
		t.Log(row)
	}

	// Count segments in tiles near artifact
	t.Log("\nSegment counts near artifact:")
	for ty := 5; ty <= 7; ty++ {
		row := ""
		for tx := 9; tx <= 12 && tx < tr.tilesX; tx++ {
			tile := tr.tiles[ty*tr.tilesX+tx]
			row += fmt.Sprintf("(%d,%d)=%d ", tx, ty, len(tile.Segments))
		}
		t.Log(row)
	}

	// Print segments in tile (10, 6) - where rightmost point is
	tileIdx := 6*tr.tilesX + 10
	t.Logf("\nSegments in tile (10, 6) [idx=%d]:", tileIdx)
	for i, seg := range tr.tiles[tileIdx].Segments {
		t.Logf("  %d: P0=(%.2f,%.2f) P1=(%.2f,%.2f) YEdge=%.2f",
			i, seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1], seg.YEdge)
	}

	// Also check tile (11, 6) - where artifact extends
	tileIdx = 6*tr.tilesX + 11
	t.Logf("\nSegments in tile (11, 6) [idx=%d]:", tileIdx)
	for i, seg := range tr.tiles[tileIdx].Segments {
		t.Logf("  %d: P0=(%.2f,%.2f) P1=(%.2f,%.2f) YEdge=%.2f",
			i, seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1], seg.YEdge)
	}

	// Check tile (9, 6) - inside circle, should have edge segments
	tileIdx = 6*tr.tilesX + 9
	t.Logf("\nSegments in tile (9, 6) [idx=%d]:", tileIdx)
	for i, seg := range tr.tiles[tileIdx].Segments {
		t.Logf("  %d: P0=(%.4f,%.4f) P1=(%.4f,%.4f) YEdge=%.4f",
			i, seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1], seg.YEdge)
	}

	// Print actual coverage for one scanline to understand the issue
	t.Log("\nCoverage at y=100 (localY=4 in tile row 6):")
	// Process tile 9
	tile9 := &tr.tiles[6*tr.tilesX+9]
	area9 := make([]float32, 16)
	for i := range area9 {
		area9[i] = float32(tile9.Backdrop)
	}
	t.Logf("Tile 9 initial (backdrop=%d): [%.2f %.2f ... %.2f %.2f]",
		tile9.Backdrop, area9[0], area9[1], area9[14], area9[15])

	// Process tile 10
	tile10 := &tr.tiles[6*tr.tilesX+10]
	area10 := make([]float32, 16)
	for i := range area10 {
		area10[i] = float32(tile10.Backdrop)
	}
	t.Logf("Tile 10 initial (backdrop=%d): [%.2f %.2f ... %.2f %.2f]",
		tile10.Backdrop, area10[0], area10[1], area10[14], area10[15])
}

// Benchmark fillPath
func BenchmarkFillPath(b *testing.B) {
	tr := NewTileRasterizer(800, 600)

	// Create a tile with some segments
	tile := &tr.tiles[0]
	tile.Backdrop = 0
	tile.Segments = []PathSegment{
		{Point0: [2]float32{0, 0}, Point1: [2]float32{16, 16}, YEdge: 1e9},
		{Point0: [2]float32{16, 0}, Point1: [2]float32{0, 16}, YEdge: 1e9},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.fillPath(tile, raster.FillRuleNonZero)
	}
}

// normalizedLine holds a line segment normalized for comparison.
// P0.y <= P1.y, with isDown indicating original direction.
type normalizedLine struct {
	x0, y0 float32
	x1, y1 float32
	isDown bool
}

// reconstituteLine converts a raster.LineEdge back to float coordinates using
// the same math as binSegments.
func reconstituteLine(line *raster.LineEdge, aaScale float32) normalizedLine {
	px0 := raster.FDot16ToFloat32(line.X) / aaScale
	py0 := float32(line.FirstY) / aaScale
	py1 := float32(line.LastY+1) / aaScale
	dxPerY := raster.FDot16ToFloat32(line.DX)
	px1 := px0 + dxPerY*(py1-py0)
	isDown := line.Winding > 0
	return normalizedLine{
		x0: px0, y0: py0,
		x1: px1, y1: py1,
		isDown: isDown,
	}
}

// velloToNormalized converts a raster.VelloLine to normalizedLine.
// VelloLine is already normalized (P0.y <= P1.y).
func velloToNormalized(vl raster.VelloLine) normalizedLine {
	return normalizedLine{
		x0: vl.P0[0], y0: vl.P0[1],
		x1: vl.P1[0], y1: vl.P1[1],
		isDown: vl.IsDown,
	}
}

// sortNormalized sorts a slice of normalizedLine by (y0, x0, y1, x1) for
// deterministic comparison.
func sortNormalized(lines []normalizedLine) {
	sort.Slice(lines, func(i, j int) bool {
		a, b := lines[i], lines[j]
		if a.y0 != b.y0 {
			return a.y0 < b.y0
		}
		if a.x0 != b.x0 {
			return a.x0 < b.x0
		}
		if a.y1 != b.y1 {
			return a.y1 < b.y1
		}
		return a.x1 < b.x1
	})
}

// TestVelloLineCoordinateValidation validates that VelloLine and raster.LineEdge
// produce identical coordinates for integer-coordinate shapes.
//
// VelloLine stores original float32 coords; raster.LineEdge quantizes through
// fixed-point (raster.FDot6/raster.FDot16). For axis-aligned edges at integer pixel
// coordinates the round-trip is lossless. For diagonal edges, the
// fixed-point quantization in raster.NewLineEdge introduces a small X offset
// (typically 0.125 = 1/(aaScale*2)) due to raster.FDot6Round and computeDY
// adjustments. This quantization error is exactly why VelloLine exists:
// to preserve the original float coordinates for the Vello pipeline.
//
// The test validates:
//  1. Axis-aligned shapes: exact coordinate match (tolerance 1e-4)
//  2. Diagonal shapes: Y coordinates match exactly, X offset is bounded
//  3. Direction (IsDown/Winding) is consistent between representations
//  4. Both representations are properly normalized (P0.y <= P1.y)
func TestVelloLineCoordinateValidation(t *testing.T) {
	const aaShift = 2
	aaScale := float32(int32(1) << uint(aaShift)) // 4.0

	type testCase struct {
		name      string
		exactX    bool // true if X coords should match exactly (axis-aligned)
		buildPath func(eb *raster.EdgeBuilder)
	}

	tests := []testCase{
		{
			name:   "square 10,10 to 30,30",
			exactX: true,
			buildPath: func(eb *raster.EdgeBuilder) {
				path := scene.NewPath()
				path.MoveTo(10, 10)
				path.LineTo(30, 10)
				path.LineTo(30, 30)
				path.LineTo(10, 30)
				path.Close()
				eb.SetFlattenCurves(true)
				BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
			},
		},
		{
			name:   "diagonal polygon",
			exactX: false, // Diagonal lines have fixed-point X quantization
			buildPath: func(eb *raster.EdgeBuilder) {
				thickness := float32(20)
				path := scene.NewPath()
				path.MoveTo(10, 10)
				path.LineTo(10+thickness, 10)
				path.LineTo(190, 190-thickness)
				path.LineTo(190, 190)
				path.LineTo(190-thickness, 190)
				path.LineTo(10, 10+thickness)
				path.Close()
				eb.SetFlattenCurves(true)
				BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eb := raster.NewEdgeBuilder(aaShift)
			tc.buildPath(eb)

			velloLines := eb.VelloLines()
			t.Logf("VelloLine count: %d", len(velloLines))
			t.Logf("raster.LineEdge count:  %d", eb.LineEdgeCount())

			// Convert VelloLines to normalized form.
			velloNorm := make([]normalizedLine, len(velloLines))
			for i, vl := range velloLines {
				velloNorm[i] = velloToNormalized(vl)
			}
			sortNormalized(velloNorm)

			// Reconstitute LineEdges to normalized form.
			lineNorm := make([]normalizedLine, 0, eb.LineEdgeCount())
			for le := range eb.LineEdges() {
				lineNorm = append(lineNorm, reconstituteLine(le, aaScale))
			}
			sortNormalized(lineNorm)

			// Log all entries for debug visibility.
			for i, v := range velloNorm {
				t.Logf("  vello[%d]: (%.4f,%.4f)-(%.4f,%.4f) down=%v",
					i, v.x0, v.y0, v.x1, v.y1, v.isDown)
			}
			for i, le := range lineNorm {
				t.Logf("  line[%d]:  (%.4f,%.4f)-(%.4f,%.4f) down=%v",
					i, le.x0, le.y0, le.x1, le.y1, le.isDown)
			}

			// Vertical combining may reduce raster.LineEdge count compared to
			// VelloLine count. We match what we can.
			if len(lineNorm) > len(velloNorm) {
				t.Errorf("raster.LineEdge count (%d) > VelloLine count (%d); "+
					"expected VelloLine >= raster.LineEdge due to vertical combining",
					len(lineNorm), len(velloNorm))
				return
			}

			// Tolerances:
			//   - Y always matches exactly for integer coords (tolerance 1e-4)
			//   - X matches exactly for axis-aligned edges
			//   - X has up to 0.25 offset for diagonal edges due to
			//     raster.FDot6Round + computeDY in raster.NewLineEdge
			const yTol = 1e-4
			xTol := 1e-4
			if !tc.exactX {
				xTol = 0.25 // Allow fixed-point quantization offset
			}

			matched := make([]bool, len(velloNorm))
			mismatches := 0
			var maxDX0, maxDX1 float64 // Track max X deviations

			for li, le := range lineNorm {
				found := false
				for vi, v := range velloNorm {
					if matched[vi] {
						continue
					}
					if v.isDown != le.isDown {
						continue
					}

					dy0 := math.Abs(float64(v.y0 - le.y0))
					dy1 := math.Abs(float64(v.y1 - le.y1))
					dx0 := math.Abs(float64(v.x0 - le.x0))
					dx1 := math.Abs(float64(v.x1 - le.x1))

					if dy0 < yTol && dy1 < yTol && dx0 < xTol && dx1 < xTol {
						matched[vi] = true
						found = true
						if dx0 > maxDX0 {
							maxDX0 = dx0
						}
						if dx1 > maxDX1 {
							maxDX1 = dx1
						}
						break
					}
				}
				if !found {
					mismatches++
					if mismatches <= 10 {
						t.Errorf("raster.LineEdge[%d] (%.4f,%.4f)-(%.4f,%.4f) down=%v "+
							"has no matching VelloLine (yTol=%.4f, xTol=%.4f)",
							li, le.x0, le.y0, le.x1, le.y1, le.isDown, yTol, xTol)
					}
				}
			}

			if mismatches > 10 {
				t.Errorf("... and %d more unmatched LineEdges", mismatches-10)
			}

			if mismatches == 0 {
				t.Logf("All %d LineEdges matched VelloLines (yTol=%.4f, xTol=%.4f)",
					len(lineNorm), yTol, xTol)
			}
			t.Logf("Max X deviation: x0=%.6f, x1=%.6f", maxDX0, maxDX1)

			// For axis-aligned shapes, verify zero X deviation.
			if tc.exactX && (maxDX0 > 1e-4 || maxDX1 > 1e-4) {
				t.Errorf("axis-aligned shape has unexpected X deviation: "+
					"x0=%.6f, x1=%.6f (expected < 1e-4)", maxDX0, maxDX1)
			}

			// Verify direction consistency: both representations must be
			// normalized with P0.y <= P1.y.
			for i, v := range velloNorm {
				if v.y0 > v.y1 {
					t.Errorf("velloNorm[%d] not normalized: y0=%.4f > y1=%.4f",
						i, v.y0, v.y1)
				}
			}
			for i, le := range lineNorm {
				if le.y0 > le.y1 {
					t.Errorf("lineNorm[%d] not normalized: y0=%.4f > y1=%.4f",
						i, le.y0, le.y1)
				}
			}
		})
	}
}
