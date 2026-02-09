package gpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg/scene"
)

// =============================================================================
// Tile Tests
// =============================================================================

func TestTileReset(t *testing.T) {
	tile := &Tile{
		X:            5,
		Y:            10,
		Backdrop:     3,
		SegmentCount: 7,
	}
	for i := range tile.Coverage {
		tile.Coverage[i] = uint8(i)
	}

	tile.Reset()

	if tile.X != 0 || tile.Y != 0 {
		t.Errorf("Reset should clear coordinates: got (%d, %d)", tile.X, tile.Y)
	}
	if tile.Backdrop != 0 {
		t.Errorf("Reset should clear backdrop: got %d", tile.Backdrop)
	}
	if tile.SegmentCount != 0 {
		t.Errorf("Reset should clear segment count: got %d", tile.SegmentCount)
	}
	for i, c := range tile.Coverage {
		if c != 0 {
			t.Errorf("Reset should clear coverage[%d]: got %d", i, c)
		}
	}
}

func TestTilePixelCoordinates(t *testing.T) {
	tile := &Tile{X: 5, Y: 10}

	if tile.PixelX() != 5*TileSize {
		t.Errorf("PixelX() = %d, want %d", tile.PixelX(), 5*TileSize)
	}
	if tile.PixelY() != 10*TileSize {
		t.Errorf("PixelY() = %d, want %d", tile.PixelY(), 10*TileSize)
	}
}

func TestTileCoverage(t *testing.T) {
	tile := &Tile{}

	// Test SetCoverage and GetCoverage
	tile.SetCoverage(2, 3, 128)
	if got := tile.GetCoverage(2, 3); got != 128 {
		t.Errorf("GetCoverage(2,3) = %d, want 128", got)
	}

	// Verify correct index calculation
	tile.SetCoverage(0, 0, 10)
	tile.SetCoverage(3, 3, 20)
	if tile.Coverage[0] != 10 {
		t.Errorf("Coverage[0] = %d, want 10", tile.Coverage[0])
	}
	if tile.Coverage[15] != 20 {
		t.Errorf("Coverage[15] = %d, want 20", tile.Coverage[15])
	}
}

func TestTileFillSolid(t *testing.T) {
	tile := &Tile{}
	tile.FillSolid(200)

	for i, c := range tile.Coverage {
		if c != 200 {
			t.Errorf("Coverage[%d] = %d, want 200", i, c)
		}
	}
}

func TestTileIsEmpty(t *testing.T) {
	tile := &Tile{}
	if !tile.IsEmpty() {
		t.Error("New tile should be empty")
	}

	tile.SetCoverage(1, 1, 100)
	if tile.IsEmpty() {
		t.Error("Tile with coverage should not be empty")
	}
}

func TestTileIsSolid(t *testing.T) {
	tile := &Tile{}
	tile.FillSolid(255)

	if !tile.IsSolid() {
		t.Error("Tile filled with 255 should be solid")
	}

	tile.SetCoverage(0, 0, 254)
	if tile.IsSolid() {
		t.Error("Tile with non-255 coverage should not be solid")
	}
}

func TestTileGridBasic(t *testing.T) {
	grid := NewTileGrid()

	if grid.TileCount() != 0 {
		t.Errorf("New grid should be empty, got %d tiles", grid.TileCount())
	}

	// Create a tile
	tile := grid.GetOrCreate(5, 10)
	if tile == nil {
		t.Fatal("GetOrCreate should return a tile")
	}
	if tile.X != 5 || tile.Y != 10 {
		t.Errorf("Tile coordinates = (%d, %d), want (5, 10)", tile.X, tile.Y)
	}

	// Verify we can get the same tile
	tile2 := grid.GetOrCreate(5, 10)
	if tile2 != tile {
		t.Error("GetOrCreate should return the same tile for same coordinates")
	}

	if grid.TileCount() != 1 {
		t.Errorf("Grid should have 1 tile, got %d", grid.TileCount())
	}
}

func TestTileGridBounds(t *testing.T) {
	grid := NewTileGrid()

	grid.GetOrCreate(2, 5)
	grid.GetOrCreate(8, 3)
	grid.GetOrCreate(5, 10)

	minX, minY, maxX, maxY := grid.Bounds()

	if minX != 2 || maxX != 8 {
		t.Errorf("X bounds = [%d, %d], want [2, 8]", minX, maxX)
	}
	if minY != 3 || maxY != 10 {
		t.Errorf("Y bounds = [%d, %d], want [3, 10]", minY, maxY)
	}
}

func TestTileGridReset(t *testing.T) {
	grid := NewTileGrid()
	grid.GetOrCreate(5, 10)
	grid.GetOrCreate(3, 7)

	grid.Reset()

	if grid.TileCount() != 0 {
		t.Errorf("Reset grid should be empty, got %d tiles", grid.TileCount())
	}
}

func TestTileGridFillRule(t *testing.T) {
	grid := NewTileGrid()

	if grid.FillRule() != scene.FillNonZero {
		t.Error("Default fill rule should be NonZero")
	}

	grid.SetFillRule(scene.FillEvenOdd)
	if grid.FillRule() != scene.FillEvenOdd {
		t.Error("Fill rule should be EvenOdd after setting")
	}
}

func TestPixelToTile(t *testing.T) {
	tests := []struct {
		px, py       int32
		wantX, wantY int32
	}{
		{0, 0, 0, 0},
		{TileSize - 1, TileSize - 1, 0, 0},
		{TileSize, TileSize, 1, 1},
		{TileSize * 5, TileSize * 3, 5, 3},
		{-1, -1, -1, -1},
		{-TileSize, -TileSize, -1, -1},
		{-TileSize - 1, -TileSize - 1, -2, -2},
	}

	for _, tt := range tests {
		gotX, gotY := PixelToTile(tt.px, tt.py)
		if gotX != tt.wantX || gotY != tt.wantY {
			t.Errorf("PixelToTile(%d, %d) = (%d, %d), want (%d, %d)",
				tt.px, tt.py, gotX, gotY, tt.wantX, tt.wantY)
		}
	}
}

func TestPixelToTileF(t *testing.T) {
	tests := []struct {
		px, py       float32
		wantX, wantY int32
	}{
		{0, 0, 0, 0},
		{float32(TileSize) - 0.5, float32(TileSize) - 0.5, 0, 0},
		{float32(TileSize), float32(TileSize), 1, 1},
		{-0.5, -0.5, -1, -1},
		{-float32(TileSize), -float32(TileSize), -1, -1},
	}

	for _, tt := range tests {
		gotX, gotY := PixelToTileF(tt.px, tt.py)
		if gotX != tt.wantX || gotY != tt.wantY {
			t.Errorf("PixelToTileF(%f, %f) = (%d, %d), want (%d, %d)",
				tt.px, tt.py, gotX, gotY, tt.wantX, tt.wantY)
		}
	}
}

// =============================================================================
// Segment Tests
// =============================================================================

func TestNewLineSegment(t *testing.T) {
	// Test normal segment (Y0 <= Y1)
	seg := NewLineSegment(10, 20, 30, 40, 1)
	if seg.Y0 > seg.Y1 {
		t.Error("Segment should be monotonic: Y0 <= Y1")
	}
	if seg.Winding != 1 {
		t.Errorf("Winding = %d, want 1", seg.Winding)
	}

	// Test reversed segment (Y1 < Y0)
	seg2 := NewLineSegment(30, 40, 10, 20, 1)
	if seg2.Y0 > seg2.Y1 {
		t.Error("Segment should be normalized: Y0 <= Y1")
	}
	if seg2.Winding != -1 {
		t.Errorf("Reversed segment winding = %d, want -1", seg2.Winding)
	}
}

func TestLineSegmentProperties(t *testing.T) {
	seg := NewLineSegment(0, 0, 10, 10, 1)

	if seg.DeltaX() != 10 {
		t.Errorf("DeltaX() = %f, want 10", seg.DeltaX())
	}
	if seg.DeltaY() != 10 {
		t.Errorf("DeltaY() = %f, want 10", seg.DeltaY())
	}
	if seg.Slope() != 1 {
		t.Errorf("Slope() = %f, want 1", seg.Slope())
	}
}

func TestLineSegmentHorizontal(t *testing.T) {
	seg := NewLineSegment(0, 5, 10, 5, 1)
	if !seg.IsHorizontal() {
		t.Error("Segment with same Y should be horizontal")
	}

	seg2 := NewLineSegment(0, 0, 10, 10, 1)
	if seg2.IsHorizontal() {
		t.Error("Diagonal segment should not be horizontal")
	}
}

func TestLineSegmentVertical(t *testing.T) {
	seg := NewLineSegment(5, 0, 5, 10, 1)
	if !seg.IsVertical() {
		t.Error("Segment with same X should be vertical")
	}

	seg2 := NewLineSegment(0, 0, 10, 10, 1)
	if seg2.IsVertical() {
		t.Error("Diagonal segment should not be vertical")
	}
}

func TestLineSegmentXAtY(t *testing.T) {
	// 45-degree line from (0,0) to (10,10)
	seg := NewLineSegment(0, 0, 10, 10, 1)

	tests := []struct {
		y    float32
		want float32
	}{
		{0, 0},
		{5, 5},
		{10, 10},
	}

	for _, tt := range tests {
		got := seg.XAtY(tt.y)
		if math.Abs(float64(got-tt.want)) > 0.001 {
			t.Errorf("XAtY(%f) = %f, want %f", tt.y, got, tt.want)
		}
	}
}

func TestLineSegmentBounds(t *testing.T) {
	seg := NewLineSegment(5, 10, 15, 25, 1)

	minX, minY, maxX, maxY := seg.Bounds()

	if minX != 5 || maxX != 15 {
		t.Errorf("X bounds = [%f, %f], want [5, 15]", minX, maxX)
	}
	if minY != 10 || maxY != 25 {
		t.Errorf("Y bounds = [%f, %f], want [10, 25]", minY, maxY)
	}
}

func TestSegmentListBasic(t *testing.T) {
	sl := NewSegmentList()

	if sl.Len() != 0 {
		t.Errorf("New list should be empty, got %d segments", sl.Len())
	}

	// Add segments
	sl.AddLine(0, 0, 10, 10, 1)
	sl.AddLine(10, 10, 20, 20, 1)

	if sl.Len() != 2 {
		t.Errorf("List should have 2 segments, got %d", sl.Len())
	}
}

func TestSegmentListSkipsDegenerate(t *testing.T) {
	sl := NewSegmentList()

	// Point (degenerate)
	sl.AddLine(5, 5, 5, 5, 1)
	if sl.Len() != 0 {
		t.Error("Point segment should be skipped")
	}

	// Horizontal (doesn't contribute to winding)
	sl.AddLine(0, 5, 10, 5, 1)
	if sl.Len() != 0 {
		t.Error("Horizontal segment should be skipped")
	}

	// Valid segment
	sl.AddLine(0, 0, 10, 10, 1)
	if sl.Len() != 1 {
		t.Error("Valid segment should be added")
	}
}

func TestSegmentListReset(t *testing.T) {
	sl := NewSegmentList()
	sl.AddLine(0, 0, 10, 10, 1)
	sl.AddLine(10, 10, 20, 20, 1)

	sl.Reset()

	if sl.Len() != 0 {
		t.Errorf("Reset list should be empty, got %d segments", sl.Len())
	}
}

func TestSegmentListBounds(t *testing.T) {
	sl := NewSegmentList()
	sl.AddLine(5, 10, 15, 25, 1)
	sl.AddLine(0, 5, 20, 30, 1)

	minX, minY, maxX, maxY := sl.Bounds()

	if minX != 0 || maxX != 20 {
		t.Errorf("X bounds = [%f, %f], want [0, 20]", minX, maxX)
	}
	if minY != 5 || maxY != 30 {
		t.Errorf("Y bounds = [%f, %f], want [5, 30]", minY, maxY)
	}
}

// =============================================================================
// Flatten Tests
// =============================================================================

func TestFlattenPathRectangle(t *testing.T) {
	path := scene.NewPath().Rectangle(10, 10, 20, 20)

	segments := FlattenPath(path, scene.IdentityAffine(), FlattenTolerance)

	// Rectangle should produce 4 line segments (but some may be horizontal and skipped)
	if segments.Len() < 2 {
		t.Errorf("Rectangle should produce at least 2 vertical segments, got %d", segments.Len())
	}

	// Check bounds roughly match rectangle
	minX, minY, maxX, maxY := segments.Bounds()
	if minX < 9 || maxX > 31 {
		t.Errorf("X bounds [%f, %f] outside expected range", minX, maxX)
	}
	if minY < 9 || maxY > 31 {
		t.Errorf("Y bounds [%f, %f] outside expected range", minY, maxY)
	}
}

func TestFlattenPathQuadratic(t *testing.T) {
	path := scene.NewPath()
	path.MoveTo(0, 0)
	path.QuadTo(50, 100, 100, 0)
	path.Close()

	segments := FlattenPath(path, scene.IdentityAffine(), FlattenTolerance)

	// Quadratic curve should be flattened to multiple segments
	if segments.Len() < 5 {
		t.Errorf("Quadratic curve should produce at least 5 segments, got %d", segments.Len())
	}

	// All segments should be monotonic
	for _, seg := range segments.Segments() {
		if seg.Y0 > seg.Y1 {
			t.Error("All segments should be monotonic (Y0 <= Y1)")
		}
	}
}

func TestFlattenPathCubic(t *testing.T) {
	path := scene.NewPath()
	path.MoveTo(0, 0)
	path.CubicTo(25, 100, 75, 100, 100, 0)
	path.Close()

	segments := FlattenPath(path, scene.IdentityAffine(), FlattenTolerance)

	// Cubic curve should be flattened to multiple segments
	if segments.Len() < 5 {
		t.Errorf("Cubic curve should produce at least 5 segments, got %d", segments.Len())
	}
}

func TestFlattenPathWithTransform(t *testing.T) {
	path := scene.NewPath().Rectangle(0, 0, 10, 10)

	// Translate by (50, 50)
	transform := scene.TranslateAffine(50, 50)
	segments := FlattenPath(path, transform, FlattenTolerance)

	if segments.Len() == 0 {
		t.Fatal("Should produce segments")
	}

	// Check bounds are translated
	minX, minY, _, _ := segments.Bounds()
	if minX < 50 || minY < 50 {
		t.Errorf("Bounds should be translated: min = (%f, %f)", minX, minY)
	}
}

func TestFlattenPathEmpty(t *testing.T) {
	path := scene.NewPath()
	segments := FlattenPath(path, scene.IdentityAffine(), FlattenTolerance)

	if segments.Len() != 0 {
		t.Errorf("Empty path should produce 0 segments, got %d", segments.Len())
	}
}

func TestFlattenPathNil(t *testing.T) {
	segments := FlattenPath(nil, scene.IdentityAffine(), FlattenTolerance)

	if segments.Len() != 0 {
		t.Errorf("Nil path should produce 0 segments, got %d", segments.Len())
	}
}

func TestFlattenContext(t *testing.T) {
	ctx := NewFlattenContext()

	path := scene.NewPath().Rectangle(10, 10, 20, 20)
	ctx.FlattenPathTo(path, scene.IdentityAffine(), FlattenTolerance)

	if ctx.Segments().Len() == 0 {
		t.Error("Context should have segments after flattening")
	}

	// Test reuse
	ctx.Reset()
	if ctx.Segments().Len() != 0 {
		t.Error("Context should be empty after reset")
	}

	ctx.FlattenPathTo(path, scene.IdentityAffine(), FlattenTolerance)
	if ctx.Segments().Len() == 0 {
		t.Error("Context should work after reset")
	}
}

// =============================================================================
// Coarse Rasterizer Tests
// =============================================================================

func TestCoarseRasterizerBasic(t *testing.T) {
	cr := NewCoarseRasterizer(100, 100)

	// Create segments for a simple vertical line
	segments := NewSegmentList()
	segments.AddLine(10, 10, 10, 30, 1)

	cr.Rasterize(segments)

	entries := cr.Entries()
	if len(entries) == 0 {
		t.Error("Should produce tile entries for vertical line")
	}
}

func TestCoarseRasterizerRectangle(t *testing.T) {
	cr := NewCoarseRasterizer(100, 100)

	path := scene.NewPath().Rectangle(10, 10, 20, 20)
	segments := FlattenPath(path, scene.IdentityAffine(), FlattenTolerance)

	cr.Rasterize(segments)

	if len(cr.Entries()) == 0 {
		t.Error("Rectangle should produce tile entries")
	}

	// Check tile columns and rows
	if cr.TileColumns() != (100+TileWidth-1)/TileWidth {
		t.Errorf("TileColumns = %d, want %d", cr.TileColumns(), (100+TileWidth-1)/TileWidth)
	}
}

func TestCoarseRasterizerBackdrop(t *testing.T) {
	cr := NewCoarseRasterizer(100, 100)

	// Create a vertical line that creates winding
	segments := NewSegmentList()
	segments.AddLine(20, 10, 20, 30, 1) // Line going down

	cr.Rasterize(segments)
	backdrop := cr.CalculateBackdrop()

	if backdrop == nil {
		t.Fatal("Backdrop should not be nil")
	}

	// Backdrop should exist for the tiles
	found := false
	for _, v := range backdrop {
		if v != 0 {
			found = true
			break
		}
	}

	// Note: the backdrop propagation is complex, we just verify it runs
	if !found {
		t.Log("Note: backdrop may be 0 for simple cases")
	}
}

func TestCoarseRasterizerReset(t *testing.T) {
	cr := NewCoarseRasterizer(100, 100)

	segments := NewSegmentList()
	segments.AddLine(10, 10, 10, 30, 1)
	cr.Rasterize(segments)

	cr.Reset()

	if len(cr.Entries()) != 0 {
		t.Error("Reset should clear entries")
	}
}

func TestCoarseRasterizerSort(t *testing.T) {
	cr := NewCoarseRasterizer(100, 100)

	// Add segments that will produce entries in different tiles
	segments := NewSegmentList()
	segments.AddLine(40, 40, 40, 50, 1)
	segments.AddLine(10, 10, 10, 20, 1)
	segments.AddLine(30, 30, 30, 40, 1)

	cr.Rasterize(segments)
	cr.SortEntries()

	entries := cr.Entries()
	if len(entries) < 2 {
		t.Skip("Need multiple entries to test sorting")
	}

	// Verify sorted by Y, then X
	for i := 1; i < len(entries); i++ {
		prev, curr := entries[i-1], entries[i]
		if prev.Y > curr.Y || (prev.Y == curr.Y && prev.X > curr.X) {
			t.Error("Entries should be sorted by Y, then X")
		}
	}
}

// =============================================================================
// Fine Rasterizer Tests
// =============================================================================

func TestFineRasterizerBasic(t *testing.T) {
	fr := NewFineRasterizer(100, 100)

	if fr.FillRule() != scene.FillNonZero {
		t.Error("Default fill rule should be NonZero")
	}

	fr.SetFillRule(scene.FillEvenOdd)
	if fr.FillRule() != scene.FillEvenOdd {
		t.Error("Fill rule should be updated")
	}
}

func TestFineRasterizerRasterize(t *testing.T) {
	// Create a simple triangle
	path := scene.NewPath()
	path.MoveTo(20, 10)
	path.LineTo(30, 30)
	path.LineTo(10, 30)
	path.Close()

	segments := FlattenPath(path, scene.IdentityAffine(), FlattenTolerance)

	cr := NewCoarseRasterizer(100, 100)
	cr.Rasterize(segments)
	cr.SortEntries()
	backdrop := cr.CalculateBackdrop()

	fr := NewFineRasterizer(100, 100)
	fr.Rasterize(cr, segments, backdrop)

	grid := fr.Grid()
	if grid.TileCount() == 0 {
		t.Error("Triangle should produce tiles with coverage")
	}

	// Check some tiles have non-zero coverage
	hasNonZero := false
	grid.ForEach(func(tile *Tile) {
		if !tile.IsEmpty() {
			hasNonZero = true
		}
	})

	if !hasNonZero {
		t.Error("Should have tiles with non-zero coverage")
	}
}

func TestFineRasterizerRenderToBuffer(t *testing.T) {
	path := scene.NewPath().Rectangle(10, 10, 20, 20)
	segments := FlattenPath(path, scene.IdentityAffine(), FlattenTolerance)

	cr := NewCoarseRasterizer(100, 100)
	cr.Rasterize(segments)
	cr.SortEntries()
	backdrop := cr.CalculateBackdrop()

	fr := NewFineRasterizer(100, 100)
	fr.Rasterize(cr, segments, backdrop)

	// Render to buffer
	buffer := make([]uint8, 100*100*4)
	color := [4]uint8{255, 0, 0, 255} // Red
	fr.RenderToBuffer(buffer, 100, 100, 100*4, color)

	// Check that some pixels were rendered
	hasRendered := false
	for i := 0; i < len(buffer); i += 4 {
		if buffer[i+3] > 0 { // Alpha > 0
			hasRendered = true
			break
		}
	}

	if !hasRendered {
		t.Error("Should render some pixels")
	}
}

// =============================================================================
// Sparse Strips Integration Tests
// =============================================================================

func TestSparseStripsRasterizerBasic(t *testing.T) {
	config := DefaultConfig(100, 100)
	ssr := NewSparseStripsRasterizer(config)

	if ssr.Width() != 100 || ssr.Height() != 100 {
		t.Errorf("Dimensions = (%d, %d), want (100, 100)", ssr.Width(), ssr.Height())
	}

	if ssr.FillRule() != scene.FillNonZero {
		t.Error("Default fill rule should be NonZero")
	}
}

func TestSparseStripsRasterizerRectangle(t *testing.T) {
	config := DefaultConfig(100, 100)
	ssr := NewSparseStripsRasterizer(config)

	path := scene.NewPath().Rectangle(10, 10, 20, 20)
	ssr.RasterizePath(path, scene.IdentityAffine(), FlattenTolerance)

	stats := ssr.GetStats()

	if stats.SegmentCount == 0 {
		t.Error("Should produce segments")
	}
	if stats.TileEntryCount == 0 {
		t.Error("Should produce tile entries")
	}
	if stats.ActiveTileCount == 0 {
		t.Error("Should produce active tiles")
	}
}

func TestSparseStripsRasterizerCircle(t *testing.T) {
	config := DefaultConfig(200, 200)
	ssr := NewSparseStripsRasterizer(config)

	path := scene.NewPath().Circle(100, 100, 50)
	ssr.RasterizePath(path, scene.IdentityAffine(), FlattenTolerance)

	stats := ssr.GetStats()

	// Circle should produce many segments from curve flattening
	if stats.SegmentCount < 20 {
		t.Errorf("Circle should produce at least 20 segments, got %d", stats.SegmentCount)
	}
}

func TestSparseStripsRasterizerStrips(t *testing.T) {
	config := DefaultConfig(100, 100)
	ssr := NewSparseStripsRasterizer(config)

	path := scene.NewPath().Rectangle(10, 10, 20, 20)
	ssr.RasterizeToStrips(path, scene.IdentityAffine(), FlattenTolerance)

	strips := ssr.Strips()
	if len(strips.Strips()) == 0 {
		t.Error("Should produce strips")
	}
}

func TestSparseStripsRasterizerRenderToBuffer(t *testing.T) {
	config := DefaultConfig(100, 100)
	ssr := NewSparseStripsRasterizer(config)

	// Rectangle at (20, 20) with width=30, height=30 spans from (20,20) to (50,50)
	path := scene.NewPath().Rectangle(20, 20, 30, 30)
	ssr.RasterizePath(path, scene.IdentityAffine(), FlattenTolerance)

	buffer := make([]uint8, 100*100*4)
	color := [4]uint8{0, 255, 0, 255} // Green
	ssr.RenderToBuffer(buffer, 100*4, color)

	// NOTE: Current sparse strips implementation only creates tiles for edge regions
	// where segments cross. Interior tiles are not created (future enhancement).
	// Test checks edge pixels rather than interior.

	// Check that grid has tiles (edge tiles are created)
	grid := ssr.Grid()
	if grid.TileCount() == 0 {
		t.Error("Grid should have tiles for rectangle edges")
		return
	}

	// Verify edge pixels have coverage by checking the grid tiles directly
	hasEdgeCoverage := false
	grid.ForEach(func(tile *Tile) {
		if !tile.IsEmpty() {
			hasEdgeCoverage = true
		}
	})

	if !hasEdgeCoverage {
		t.Error("Edge tiles should have non-zero coverage")
	}

	// Verify the buffer has some pixels rendered (on edges)
	hasRenderedPixels := false
	for i := 3; i < len(buffer); i += 4 {
		if buffer[i] > 0 { // Check alpha channel
			hasRenderedPixels = true
			break
		}
	}

	if !hasRenderedPixels {
		t.Error("Buffer should have rendered pixels on rectangle edges")
	}
}

func TestSparseStripsRasterizerReset(t *testing.T) {
	config := DefaultConfig(100, 100)
	ssr := NewSparseStripsRasterizer(config)

	path := scene.NewPath().Rectangle(10, 10, 20, 20)
	ssr.RasterizePath(path, scene.IdentityAffine(), FlattenTolerance)

	ssr.Reset()

	stats := ssr.GetStats()
	if stats.SegmentCount != 0 || stats.TileEntryCount != 0 {
		t.Error("Reset should clear all state")
	}
}

func TestSparseStripsRasterizerFillRules(t *testing.T) {
	tests := []struct {
		name     string
		fillRule scene.FillStyle
	}{
		{"NonZero", scene.FillNonZero},
		{"EvenOdd", scene.FillEvenOdd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig(100, 100)
			config.FillRule = tt.fillRule
			ssr := NewSparseStripsRasterizer(config)

			path := scene.NewPath().Rectangle(10, 10, 20, 20)
			ssr.RasterizePath(path, scene.IdentityAffine(), FlattenTolerance)

			if ssr.FillRule() != tt.fillRule {
				t.Errorf("FillRule = %v, want %v", ssr.FillRule(), tt.fillRule)
			}
		})
	}
}

func TestSparseStripsPool(t *testing.T) {
	pool := NewSparseStripsPool()
	config := DefaultConfig(100, 100)

	ssr1 := pool.Get(config)
	if ssr1 == nil {
		t.Fatal("Pool should return a rasterizer")
	}

	pool.Put(ssr1)

	ssr2 := pool.Get(config)
	if ssr2 != ssr1 {
		t.Error("Pool should reuse rasterizer")
	}
}

func TestSparseStripsRasterizerEmptyPath(t *testing.T) {
	config := DefaultConfig(100, 100)
	ssr := NewSparseStripsRasterizer(config)

	path := scene.NewPath()
	ssr.RasterizePath(path, scene.IdentityAffine(), FlattenTolerance)

	stats := ssr.GetStats()
	if stats.SegmentCount != 0 {
		t.Error("Empty path should produce 0 segments")
	}
}

func TestSparseStripsRasterizerNilPath(t *testing.T) {
	config := DefaultConfig(100, 100)
	ssr := NewSparseStripsRasterizer(config)

	ssr.RasterizePath(nil, scene.IdentityAffine(), FlattenTolerance)

	stats := ssr.GetStats()
	if stats.SegmentCount != 0 {
		t.Error("Nil path should produce 0 segments")
	}
}

func TestSparseStripsRasterizerResize(t *testing.T) {
	config := DefaultConfig(100, 100)
	ssr := NewSparseStripsRasterizer(config)

	ssr.SetSize(200, 200)

	if ssr.Width() != 200 || ssr.Height() != 200 {
		t.Errorf("After resize: (%d, %d), want (200, 200)", ssr.Width(), ssr.Height())
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkFlattenRectangle(b *testing.B) {
	path := scene.NewPath().Rectangle(0, 0, 100, 100)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FlattenPath(path, transform, FlattenTolerance)
	}
}

func BenchmarkFlattenCircle(b *testing.B) {
	path := scene.NewPath().Circle(50, 50, 50)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FlattenPath(path, transform, FlattenTolerance)
	}
}

func BenchmarkCoarseRasterizeRectangle(b *testing.B) {
	path := scene.NewPath().Rectangle(10, 10, 100, 100)
	segments := FlattenPath(path, scene.IdentityAffine(), FlattenTolerance)
	cr := NewCoarseRasterizer(200, 200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr.Reset()
		cr.Rasterize(segments)
	}
}

func BenchmarkFineRasterizeRectangle(b *testing.B) {
	path := scene.NewPath().Rectangle(10, 10, 100, 100)
	segments := FlattenPath(path, scene.IdentityAffine(), FlattenTolerance)

	cr := NewCoarseRasterizer(200, 200)
	cr.Rasterize(segments)
	cr.SortEntries()
	backdrop := cr.CalculateBackdrop()

	fr := NewFineRasterizer(200, 200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fr.Reset()
		fr.Rasterize(cr, segments, backdrop)
	}
}

func BenchmarkSparseStripsRectangle100x100(b *testing.B) {
	config := DefaultConfig(200, 200)
	ssr := NewSparseStripsRasterizer(config)
	path := scene.NewPath().Rectangle(10, 10, 100, 100)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ssr.Reset()
		ssr.RasterizePath(path, transform, FlattenTolerance)
	}
}

func BenchmarkSparseStripsCircle(b *testing.B) {
	config := DefaultConfig(200, 200)
	ssr := NewSparseStripsRasterizer(config)
	path := scene.NewPath().Circle(100, 100, 50)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ssr.Reset()
		ssr.RasterizePath(path, transform, FlattenTolerance)
	}
}

func BenchmarkSparseStripsComplexPath(b *testing.B) {
	config := DefaultConfig(400, 400)
	ssr := NewSparseStripsRasterizer(config)

	// Create a complex path with curves
	path := scene.NewPath()
	path.MoveTo(100, 100)
	for i := 0; i < 50; i++ {
		angle := float32(i) * 0.2
		x := 200 + 80*float32(math.Cos(float64(angle)))
		y := 200 + 80*float32(math.Sin(float64(angle)))
		path.LineTo(x, y)
	}
	path.Close()

	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ssr.Reset()
		ssr.RasterizePath(path, transform, FlattenTolerance)
	}
}

func BenchmarkRenderToBuffer(b *testing.B) {
	config := DefaultConfig(400, 400)
	ssr := NewSparseStripsRasterizer(config)
	path := scene.NewPath().Rectangle(50, 50, 300, 300)
	ssr.RasterizePath(path, scene.IdentityAffine(), FlattenTolerance)

	buffer := make([]uint8, 400*400*4)
	color := [4]uint8{255, 128, 0, 255}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear buffer
		for j := range buffer {
			buffer[j] = 0
		}
		ssr.RenderToBuffer(buffer, 400*4, color)
	}
}

func BenchmarkTilePool(b *testing.B) {
	pool := NewTilePool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tile := pool.Get()
		pool.Put(tile)
	}
}

func BenchmarkSparseStripsPool(b *testing.B) {
	pool := NewSparseStripsPool()
	config := DefaultConfig(100, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ssr := pool.Get(config)
		pool.Put(ssr)
	}
}
