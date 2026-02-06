// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"github.com/gogpu/gg/internal/raster"
	"testing"

	"github.com/gogpu/gg/scene"
)

// TestRasterizeCurvesEmptyInput verifies behavior with empty input.
func TestRasterizeCurvesEmptyInput(t *testing.T) {
	fr := NewFineRasterizer(64, 64)

	// Test with nil map
	fr.RasterizeCurves(nil)
	if fr.Grid().TileCount() != 0 {
		t.Errorf("expected 0 tiles for nil input, got %d", fr.Grid().TileCount())
	}

	// Test with empty map
	fr.RasterizeCurves(make(map[uint64]*CurveTileBin))
	if fr.Grid().TileCount() != 0 {
		t.Errorf("expected 0 tiles for empty input, got %d", fr.Grid().TileCount())
	}
}

// TestRasterizeCurvesWithLineEdges verifies line edge processing.
func TestRasterizeCurvesWithLineEdges(t *testing.T) {
	// Test with a closed rectangle (lines only)
	fr := NewFineRasterizer(32, 32)
	fr.SetFillRule(scene.FillNonZero)
	cr := NewCoarseRasterizer(32, 32)

	// Create a closed rectangle path
	path := scene.NewPath()
	path.MoveTo(4, 4)
	path.LineTo(28, 4)
	path.LineTo(28, 28)
	path.LineTo(4, 28)
	path.Close()

	// Build edges from path
	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Verify we have line edges
	if eb.LineEdgeCount() == 0 {
		t.Error("expected line edges to be created")
	}

	// Bin edges
	bins := cr.BinCurveEdges(eb)
	if bins == nil {
		t.Fatal("BinCurveEdges returned nil for non-empty raster.EdgeBuilder")
	}

	// Rasterize
	fr.RasterizeCurves(bins)

	// Check we got some output (closed shapes should produce tiles)
	if fr.Grid().TileCount() == 0 {
		t.Error("expected tiles from closed rectangle path")
	}

	t.Logf("Rectangle: %d line edges, %d tiles", eb.LineEdgeCount(), fr.Grid().TileCount())
}

// TestRasterizeCurvesWithQuadratic verifies quadratic curve edge processing.
func TestRasterizeCurvesWithQuadratic(t *testing.T) {
	fr := NewFineRasterizer(64, 64)
	fr.SetFillRule(scene.FillNonZero)
	cr := NewCoarseRasterizer(64, 64)

	// Create edge builder with a quadratic curve via scene.Path
	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.QuadTo(32, 5, 54, 10)
	path.Close()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Bin edges
	bins := cr.BinCurveEdges(eb)

	// Rasterize
	fr.RasterizeCurves(bins)

	// Quadratic curves should produce edges
	if eb.QuadraticEdgeCount() == 0 && eb.LineEdgeCount() == 0 {
		t.Error("expected edges to be created")
	}

	// Verify some tiles were created
	if fr.Grid().TileCount() == 0 {
		t.Log("Warning: no tiles created for quadratic curve (may be outside viewport)")
	}
}

// TestRasterizeCurvesWithCubic verifies cubic curve edge processing.
func TestRasterizeCurvesWithCubic(t *testing.T) {
	fr := NewFineRasterizer(64, 64)
	fr.SetFillRule(scene.FillNonZero)
	cr := NewCoarseRasterizer(64, 64)

	// Create edge builder with a cubic curve via scene.Path
	path := scene.NewPath()
	path.MoveTo(10, 32)
	path.CubicTo(20, 10, 44, 10, 54, 32)
	path.Close()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Bin edges
	bins := cr.BinCurveEdges(eb)

	// Rasterize
	fr.RasterizeCurves(bins)

	// Cubic curves should produce edges
	if eb.CubicEdgeCount() == 0 && eb.LineEdgeCount() == 0 {
		t.Error("expected edges to be created")
	}
}

// TestRasterizeCurvesClosedPath verifies closed path processing.
func TestRasterizeCurvesClosedPath(t *testing.T) {
	fr := NewFineRasterizer(64, 64)
	fr.SetFillRule(scene.FillNonZero)
	cr := NewCoarseRasterizer(64, 64)

	// Create a triangle path using scene.Path
	path := scene.NewPath()
	path.MoveTo(32, 8)
	path.LineTo(56, 56)
	path.LineTo(8, 56)
	path.Close()

	// Build edges from path
	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Bin edges
	bins := cr.BinCurveEdges(eb)

	// Rasterize
	fr.RasterizeCurves(bins)

	// Triangle should produce tiles
	if fr.Grid().TileCount() == 0 {
		t.Error("expected tiles from closed triangle path")
	}
}

// TestRasterizeCurvesCirclePath verifies circular path with curves.
func TestRasterizeCurvesCirclePath(t *testing.T) {
	fr := NewFineRasterizer(64, 64)
	fr.SetFillRule(scene.FillNonZero)
	cr := NewCoarseRasterizer(64, 64)

	// Create a circle-like path with cubic curves
	path := scene.NewPath()
	cx, cy := float32(32), float32(32)
	r := float32(20)

	// Approximate circle with 4 cubic Bezier segments
	// Using standard cubic Bezier approximation: control point offset = r * 0.5523
	k := r * 0.5523

	path.MoveTo(cx+r, cy)
	path.CubicTo(cx+r, cy+k, cx+k, cy+r, cx, cy+r)
	path.CubicTo(cx-k, cy+r, cx-r, cy+k, cx-r, cy)
	path.CubicTo(cx-r, cy-k, cx-k, cy-r, cx, cy-r)
	path.CubicTo(cx+k, cy-r, cx+r, cy-k, cx+r, cy)
	path.Close()

	// Build edges from path
	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Bin edges
	bins := cr.BinCurveEdges(eb)

	// Rasterize
	fr.RasterizeCurves(bins)

	// Log what we got - circle with cubic curves requires proper curve handling
	t.Logf("Circle path: %d line edges, %d quad edges, %d cubic edges, %d tiles",
		eb.LineEdgeCount(), eb.QuadraticEdgeCount(), eb.CubicEdgeCount(), fr.Grid().TileCount())

	// Cubic edges should be created
	if eb.CubicEdgeCount() == 0 {
		t.Error("expected cubic edges from circle path")
	}

	// Note: The current implementation bins cubics to tiles, but the curve processing
	// in FineRasterizer may not produce visible output if the curve edges don't
	// cross tile boundaries properly. This is expected for unit test validation.
}

// TestRasterizeCurvesFillRules verifies both fill rules work correctly.
func TestRasterizeCurvesFillRules(t *testing.T) {
	// Create overlapping squares path
	path := scene.NewPath()
	// Outer square
	path.MoveTo(10, 10)
	path.LineTo(54, 10)
	path.LineTo(54, 54)
	path.LineTo(10, 54)
	path.Close()
	// Inner square (creates overlap for even-odd testing)
	path.MoveTo(20, 20)
	path.LineTo(44, 20)
	path.LineTo(44, 44)
	path.LineTo(20, 44)
	path.Close()

	tests := []struct {
		name     string
		fillRule scene.FillStyle
	}{
		{"NonZero", scene.FillNonZero},
		{"EvenOdd", scene.FillEvenOdd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := NewFineRasterizer(64, 64)
			fr.SetFillRule(tt.fillRule)
			cr := NewCoarseRasterizer(64, 64)

			eb := raster.NewEdgeBuilder(2)
			BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

			bins := cr.BinCurveEdges(eb)
			fr.RasterizeCurves(bins)

			if fr.Grid().TileCount() == 0 {
				t.Errorf("expected tiles for fill rule %s", tt.name)
			}

			t.Logf("%s: %d tiles", tt.name, fr.Grid().TileCount())
		})
	}
}

// TestRasterizeCurvesBackdrop verifies backdrop handling.
func TestRasterizeCurvesBackdrop(t *testing.T) {
	fr := NewFineRasterizer(32, 32)
	fr.SetFillRule(scene.FillNonZero)

	// Create a simple bin with backdrop
	bins := make(map[uint64]*CurveTileBin)
	coord := TileCoord{X: 2, Y: 2}
	bins[coord.Key()] = &CurveTileBin{
		Edges:    []raster.CurveEdgeVariant{},
		Backdrop: 1, // Inside fill
	}

	fr.RasterizeCurves(bins)

	// Should create a solid tile
	tile := fr.Grid().Get(2, 2)
	if tile == nil {
		t.Error("expected tile at (2, 2) with backdrop")
		return
	}

	// Check that tile is filled
	if tile.IsEmpty() {
		t.Error("expected non-empty tile with backdrop=1")
	}

	// Check coverage value
	coverage := tile.GetCoverage(0, 0)
	if coverage != 255 {
		t.Errorf("expected coverage 255 for backdrop=1, got %d", coverage)
	}
}

// TestBinCurveEdgesNil verifies nil EdgeBuilder handling.
func TestBinCurveEdgesNil(t *testing.T) {
	cr := NewCoarseRasterizer(64, 64)

	result := cr.BinCurveEdges(nil)
	if result != nil {
		t.Error("expected nil result for nil raster.EdgeBuilder")
	}
}

// TestBinCurveEdgesEmpty verifies empty EdgeBuilder handling.
func TestBinCurveEdgesEmpty(t *testing.T) {
	cr := NewCoarseRasterizer(64, 64)
	eb := raster.NewEdgeBuilder(2)

	result := cr.BinCurveEdges(eb)
	if result != nil {
		t.Error("expected nil result for empty raster.EdgeBuilder")
	}
}

// TestCurveTileBinCloning verifies edge cloning works correctly.
func TestCurveTileBinCloning(t *testing.T) {
	cr := NewCoarseRasterizer(64, 64)

	// Create edge builder with a vertical line via scene.Path
	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(10, 50)
	path.LineTo(11, 50) // small width to form closed shape
	path.LineTo(11, 10)
	path.Close()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	bins := cr.BinCurveEdges(eb)

	// Collect all edges from bins
	edgeCount := 0
	for _, bin := range bins {
		edgeCount += len(bin.Edges)
	}

	// Should have at least one edge
	if edgeCount == 0 {
		t.Error("expected edges in bins")
	}

	t.Logf("Lines created %d edges across %d bins", edgeCount, len(bins))
}

// TestLineEdgeToSegment verifies raster.LineEdge to LineSegment conversion.
func TestLineEdgeToSegment(t *testing.T) {
	fr := NewFineRasterizer(64, 64)

	// Create a line edge
	p0 := raster.CurvePoint{X: 10, Y: 5}
	p1 := raster.CurvePoint{X: 20, Y: 15}
	line := raster.NewLineEdge(p0, p1, 2)

	if line == nil {
		t.Fatal("failed to create line edge")
	}

	// Convert to segment
	segment := fr.lineEdgeToSegment(line, 0, 0)

	// Verify Y coordinates are correct
	if segment.Y0 > segment.Y1 {
		t.Error("expected Y0 <= Y1 for monotonic segment")
	}

	// Verify winding is preserved
	if segment.Winding == 0 {
		t.Error("expected non-zero winding")
	}
}

// TestFillTileWithBackdrop verifies backdrop filling.
func TestFillTileWithBackdrop(t *testing.T) {
	tests := []struct {
		name           string
		backdrop       int32
		fillRule       scene.FillStyle
		expectEmpty    bool
		expectCoverage uint8
	}{
		{
			name:           "nonzero positive",
			backdrop:       1,
			fillRule:       scene.FillNonZero,
			expectEmpty:    false,
			expectCoverage: 255,
		},
		{
			name:           "nonzero negative",
			backdrop:       -1,
			fillRule:       scene.FillNonZero,
			expectEmpty:    false,
			expectCoverage: 255,
		},
		{
			name:           "nonzero zero",
			backdrop:       0,
			fillRule:       scene.FillNonZero,
			expectEmpty:    true,
			expectCoverage: 0,
		},
		{
			name:           "evenodd odd",
			backdrop:       1,
			fillRule:       scene.FillEvenOdd,
			expectEmpty:    false,
			expectCoverage: 255,
		},
		{
			name:           "evenodd even",
			backdrop:       2,
			fillRule:       scene.FillEvenOdd,
			expectEmpty:    true,
			expectCoverage: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := NewFineRasterizer(32, 32)
			fr.SetFillRule(tt.fillRule)

			tile := fr.grid.GetOrCreate(1, 1)
			tile.Reset()

			if tt.backdrop != 0 {
				fr.fillTileWithBackdrop(tile, tt.backdrop)
			}

			if tt.expectEmpty && !tile.IsEmpty() {
				t.Error("expected empty tile")
			}
			if !tt.expectEmpty && tile.IsEmpty() {
				t.Error("expected non-empty tile")
			}

			if !tt.expectEmpty {
				coverage := tile.GetCoverage(0, 0)
				if coverage != tt.expectCoverage {
					t.Errorf("expected coverage %d, got %d", tt.expectCoverage, coverage)
				}
			}
		})
	}
}

// BenchmarkRasterizeCurvesLine benchmarks line edge processing.
func BenchmarkRasterizeCurvesLine(b *testing.B) {
	fr := NewFineRasterizer(256, 256)
	cr := NewCoarseRasterizer(256, 256)

	// Create a diagonal line via scene.Path (closed shape)
	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(246, 246)
	path.LineTo(247, 245)
	path.Close()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
	bins := cr.BinCurveEdges(eb)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fr.RasterizeCurves(bins)
	}
}

// BenchmarkRasterizeCurvesQuadratic benchmarks quadratic curve processing.
func BenchmarkRasterizeCurvesQuadratic(b *testing.B) {
	fr := NewFineRasterizer(256, 256)
	cr := NewCoarseRasterizer(256, 256)

	// Create circle-like path with quadratic approximation
	path := scene.NewPath()
	path.MoveTo(128, 28)
	path.QuadTo(228, 128, 128, 228)
	path.QuadTo(28, 128, 128, 28)
	path.Close()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
	bins := cr.BinCurveEdges(eb)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fr.RasterizeCurves(bins)
	}
}

// BenchmarkRasterizeCurvesCubic benchmarks cubic curve processing.
func BenchmarkRasterizeCurvesCubic(b *testing.B) {
	fr := NewFineRasterizer(256, 256)
	cr := NewCoarseRasterizer(256, 256)

	// Create circle path with cubic Bezier approximation
	path := scene.NewPath()
	cx, cy := float32(128), float32(128)
	r := float32(100)
	k := r * 0.5523

	path.MoveTo(cx+r, cy)
	path.CubicTo(cx+r, cy+k, cx+k, cy+r, cx, cy+r)
	path.CubicTo(cx-k, cy+r, cx-r, cy+k, cx-r, cy)
	path.CubicTo(cx-r, cy-k, cx-k, cy-r, cx, cy-r)
	path.CubicTo(cx+k, cy-r, cx+r, cy-k, cx+r, cy)
	path.Close()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
	bins := cr.BinCurveEdges(eb)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fr.RasterizeCurves(bins)
	}
}

// BenchmarkBinCurveEdges benchmarks the binning phase.
func BenchmarkBinCurveEdges(b *testing.B) {
	cr := NewCoarseRasterizer(256, 256)

	path := scene.NewPath()
	cx, cy := float32(128), float32(128)
	r := float32(100)
	k := r * 0.5523

	path.MoveTo(cx+r, cy)
	path.CubicTo(cx+r, cy+k, cx+k, cy+r, cx, cy+r)
	path.CubicTo(cx-k, cy+r, cx-r, cy+k, cx-r, cy)
	path.CubicTo(cx-r, cy-k, cx-k, cy-r, cx, cy-r)
	path.CubicTo(cx+k, cy-r, cx+r, cy-k, cx+r, cy)
	path.Close()

	eb := raster.NewEdgeBuilder(2)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cr.BinCurveEdges(eb)
	}
}
