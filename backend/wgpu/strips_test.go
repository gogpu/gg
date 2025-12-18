package wgpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg/scene"
)

// =============================================================================
// Strip Tests
// =============================================================================

func TestStripClone(t *testing.T) {
	original := Strip{
		Y:        10,
		X:        20,
		Width:    5,
		Coverage: []uint8{255, 200, 150, 100, 50},
	}

	clone := original.Clone()

	// Verify values match
	if clone.Y != original.Y {
		t.Errorf("Y mismatch: got %d, want %d", clone.Y, original.Y)
	}
	if clone.X != original.X {
		t.Errorf("X mismatch: got %d, want %d", clone.X, original.X)
	}
	if clone.Width != original.Width {
		t.Errorf("Width mismatch: got %d, want %d", clone.Width, original.Width)
	}
	if len(clone.Coverage) != len(original.Coverage) {
		t.Errorf("Coverage length mismatch: got %d, want %d", len(clone.Coverage), len(original.Coverage))
	}

	// Verify it's a deep copy
	clone.Coverage[0] = 0
	if original.Coverage[0] == 0 {
		t.Error("Clone modified original coverage")
	}
}

func TestStripEnd(t *testing.T) {
	strip := Strip{X: 10, Width: 5}
	if strip.End() != 15 {
		t.Errorf("End() = %d, want 15", strip.End())
	}
}

// =============================================================================
// StripBuffer Tests
// =============================================================================

func TestStripBufferBasic(t *testing.T) {
	sb := NewStripBuffer()

	if !sb.IsEmpty() {
		t.Error("New buffer should be empty")
	}

	// Add a strip
	sb.AddStrip(10, 20, []uint8{255, 200, 150})

	if sb.IsEmpty() {
		t.Error("Buffer should not be empty after adding strip")
	}
	if sb.StripCount() != 1 {
		t.Errorf("StripCount() = %d, want 1", sb.StripCount())
	}
	if sb.TotalCoverage() != 3 {
		t.Errorf("TotalCoverage() = %d, want 3", sb.TotalCoverage())
	}
}

func TestStripBufferReset(t *testing.T) {
	sb := NewStripBuffer()
	sb.AddStrip(10, 20, []uint8{255, 200, 150})
	sb.AddStrip(11, 20, []uint8{255, 200, 150})

	sb.Reset()

	if !sb.IsEmpty() {
		t.Error("Buffer should be empty after reset")
	}
	if sb.StripCount() != 0 {
		t.Errorf("StripCount() = %d, want 0 after reset", sb.StripCount())
	}
}

func TestStripBufferBounds(t *testing.T) {
	sb := NewStripBuffer()
	sb.AddStrip(10, 20, []uint8{255, 255, 255, 255, 255})
	sb.AddStrip(15, 25, []uint8{255, 255, 255})

	bounds := sb.Bounds()

	if bounds.MinX != 20 {
		t.Errorf("MinX = %f, want 20", bounds.MinX)
	}
	if bounds.MinY != 10 {
		t.Errorf("MinY = %f, want 10", bounds.MinY)
	}
	if bounds.MaxX != 28 { // max of 20+5=25 and 25+3=28
		t.Errorf("MaxX = %f, want 28", bounds.MaxX)
	}
	if bounds.MaxY != 16 { // max of 10+1=11 and 15+1=16
		t.Errorf("MaxY = %f, want 16", bounds.MaxY)
	}
}

func TestStripBufferPackForGPU(t *testing.T) {
	sb := NewStripBuffer()
	sb.AddStrip(10, 20, []uint8{255, 200})
	sb.AddStrip(11, 22, []uint8{150, 100, 50})

	headers, coverage := sb.PackForGPU()

	// Verify headers
	if len(headers) != 2 {
		t.Fatalf("headers length = %d, want 2", len(headers))
	}

	// First header
	if headers[0].Y != 10 || headers[0].X != 20 || headers[0].Width != 2 || headers[0].Offset != 0 {
		t.Errorf("headers[0] = %+v, want Y=10, X=20, Width=2, Offset=0", headers[0])
	}

	// Second header
	if headers[1].Y != 11 || headers[1].X != 22 || headers[1].Width != 3 || headers[1].Offset != 2 {
		t.Errorf("headers[1] = %+v, want Y=11, X=22, Width=3, Offset=2", headers[1])
	}

	// Verify coverage
	expected := []uint8{255, 200, 150, 100, 50}
	if len(coverage) != len(expected) {
		t.Fatalf("coverage length = %d, want %d", len(coverage), len(expected))
	}
	for i, v := range expected {
		if coverage[i] != v {
			t.Errorf("coverage[%d] = %d, want %d", i, coverage[i], v)
		}
	}
}

func TestStripBufferPackForGPUInto(t *testing.T) {
	sb := NewStripBuffer()
	sb.AddStrip(10, 20, []uint8{255, 200})
	sb.AddStrip(11, 22, []uint8{150})

	headers := make([]GPUStripHeader, 10)
	coverage := make([]uint8, 10)

	n := sb.PackForGPUInto(headers, coverage)
	if n != 2 {
		t.Errorf("PackForGPUInto returned %d, want 2", n)
	}

	// Test with too small buffers
	smallHeaders := make([]GPUStripHeader, 1)
	if sb.PackForGPUInto(smallHeaders, coverage) != -1 {
		t.Error("Should return -1 for too small headers buffer")
	}

	smallCoverage := make([]uint8, 1)
	if sb.PackForGPUInto(headers, smallCoverage) != -1 {
		t.Error("Should return -1 for too small coverage buffer")
	}
}

func TestStripBufferMergeAdjacent(t *testing.T) {
	sb := NewStripBuffer()
	sb.AddStrip(10, 20, []uint8{255, 200})
	sb.AddStrip(10, 22, []uint8{150, 100}) // Adjacent on same row
	sb.AddStrip(11, 20, []uint8{255})      // Different row

	sb.MergeAdjacent()

	if sb.StripCount() != 2 {
		t.Errorf("StripCount() = %d, want 2 after merge", sb.StripCount())
	}

	// Find the merged strip on row 10
	var row10Strip *Strip
	for i := range sb.Strips() {
		if sb.Strips()[i].Y == 10 {
			row10Strip = &sb.Strips()[i]
			break
		}
	}

	if row10Strip == nil {
		t.Fatal("Could not find strip on row 10")
	}

	if row10Strip.Width != 4 {
		t.Errorf("Merged strip width = %d, want 4", row10Strip.Width)
	}
	if len(row10Strip.Coverage) != 4 {
		t.Errorf("Merged coverage length = %d, want 4", len(row10Strip.Coverage))
	}

	expected := []uint8{255, 200, 150, 100}
	for i, v := range expected {
		if row10Strip.Coverage[i] != v {
			t.Errorf("Merged coverage[%d] = %d, want %d", i, row10Strip.Coverage[i], v)
		}
	}
}

func TestStripBufferClone(t *testing.T) {
	sb := NewStripBuffer()
	sb.SetFillRule(scene.FillEvenOdd)
	sb.AddStrip(10, 20, []uint8{255, 200})

	clone := sb.Clone()

	if clone.FillRule() != scene.FillEvenOdd {
		t.Error("Clone should preserve fill rule")
	}
	if clone.StripCount() != 1 {
		t.Errorf("Clone strip count = %d, want 1", clone.StripCount())
	}

	// Verify deep copy
	clone.Reset()
	if sb.StripCount() == 0 {
		t.Error("Clone reset affected original")
	}
}

// =============================================================================
// Edge Tests
// =============================================================================

func TestNewEdge(t *testing.T) {
	tests := []struct {
		name    string
		x0, y0  float32
		x1, y1  float32
		wantNil bool
	}{
		{"normal downward", 0, 0, 10, 10, false},
		{"normal upward", 10, 10, 0, 0, false},
		{"horizontal", 0, 5, 10, 5, true},
		{"nearly horizontal", 0, 5, 10, 5.0000001, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := NewEdge(tt.x0, tt.y0, tt.x1, tt.y1)
			if (edge == nil) != tt.wantNil {
				t.Errorf("NewEdge() nil = %v, want %v", edge == nil, tt.wantNil)
			}
		})
	}
}

func TestEdgeXAtY(t *testing.T) {
	// 45-degree line from (0,0) to (10,10)
	edge := NewEdgeWithWinding(0, 0, 10, 10, 1)
	if edge == nil {
		t.Fatal("Edge should not be nil")
	}

	tests := []struct {
		y    float32
		want float32
	}{
		{0, 0},
		{5, 5},
		{10, 10},
		{2.5, 2.5},
	}

	for _, tt := range tests {
		got := edge.XAtY(tt.y)
		if math.Abs(float64(got-tt.want)) > 0.001 {
			t.Errorf("XAtY(%f) = %f, want %f", tt.y, got, tt.want)
		}
	}
}

func TestEdgeIsActiveAt(t *testing.T) {
	edge := NewEdgeWithWinding(0, 0, 10, 10, 1)
	if edge == nil {
		t.Fatal("Edge should not be nil")
	}

	tests := []struct {
		y    float32
		want bool
	}{
		{-1, false},
		{0, true},
		{5, true},
		{9.999, true},
		{10, false},
		{11, false},
	}

	for _, tt := range tests {
		got := edge.IsActiveAt(tt.y)
		if got != tt.want {
			t.Errorf("IsActiveAt(%f) = %v, want %v", tt.y, got, tt.want)
		}
	}
}

func TestEdgeListSortByYMin(t *testing.T) {
	el := NewEdgeList()
	el.AddLine(0, 10, 10, 20)
	el.AddLine(0, 0, 10, 10)
	el.AddLine(0, 5, 10, 15)

	el.SortByYMin()

	edges := el.Edges()
	if edges[0].yMin > edges[1].yMin || edges[1].yMin > edges[2].yMin {
		t.Error("Edges not sorted by yMin")
	}
}

func TestEdgeListBounds(t *testing.T) {
	el := NewEdgeList()
	el.AddLine(5, 10, 15, 20)
	el.AddLine(0, 5, 20, 25)

	minX, minY, maxX, maxY := el.Bounds()

	if minX != 0 {
		t.Errorf("minX = %f, want 0", minX)
	}
	if minY != 5 {
		t.Errorf("minY = %f, want 5", minY)
	}
	if maxX != 20 {
		t.Errorf("maxX = %f, want 20", maxX)
	}
	if maxY != 25 {
		t.Errorf("maxY = %f, want 25", maxY)
	}
}

// =============================================================================
// Tessellator Tests
// =============================================================================

func TestTessellatorRectangle(t *testing.T) {
	tess := NewTessellator()

	path := scene.NewPath().Rectangle(10, 10, 20, 20)
	buffer := tess.TessellatePath(path, scene.IdentityAffine())

	if buffer.IsEmpty() {
		t.Error("Buffer should not be empty for rectangle")
	}

	// A 20x20 rectangle should produce approximately 20 strips
	if buffer.StripCount() < 15 || buffer.StripCount() > 25 {
		t.Errorf("Strip count = %d, expected around 20", buffer.StripCount())
	}

	// Check bounds (allow 1 pixel tolerance for anti-aliasing)
	bounds := buffer.Bounds()
	if bounds.MinX < 9 || bounds.MaxX > 31 {
		t.Errorf("X bounds [%f, %f] outside expected [9, 31]", bounds.MinX, bounds.MaxX)
	}
	if bounds.MinY < 9 || bounds.MaxY > 31 {
		t.Errorf("Y bounds [%f, %f] outside expected [9, 31]", bounds.MinY, bounds.MaxY)
	}
}

func TestTessellatorCircle(t *testing.T) {
	tess := NewTessellator()

	buffer := tess.TessellateCircle(50, 50, 20)

	if buffer.IsEmpty() {
		t.Error("Buffer should not be empty for circle")
	}

	// A circle with radius 20 should produce approximately 40 strips
	if buffer.StripCount() < 30 || buffer.StripCount() > 50 {
		t.Errorf("Strip count = %d, expected around 40", buffer.StripCount())
	}
}

func TestTessellatorTransform(t *testing.T) {
	tess := NewTessellator()

	path := scene.NewPath().Rectangle(0, 0, 10, 10)

	// Translate by (50, 50)
	transform := scene.TranslateAffine(50, 50)
	buffer := tess.TessellatePath(path, transform)

	if buffer.IsEmpty() {
		t.Error("Buffer should not be empty")
	}

	bounds := buffer.Bounds()
	if bounds.MinX < 50 || bounds.MinY < 50 {
		t.Errorf("Bounds should be translated: got min (%f, %f)", bounds.MinX, bounds.MinY)
	}
}

func TestTessellatorFillRules(t *testing.T) {
	// Create a path with overlapping regions
	path := scene.NewPath()
	path.Rectangle(0, 0, 20, 20)
	path.Rectangle(5, 5, 10, 10) // Inner rectangle

	// Test NonZero
	tessNonZero := NewTessellator()
	tessNonZero.SetFillRule(scene.FillNonZero)
	bufferNonZero := tessNonZero.TessellatePath(path, scene.IdentityAffine())

	// Test EvenOdd
	tessEvenOdd := NewTessellator()
	tessEvenOdd.SetFillRule(scene.FillEvenOdd)
	bufferEvenOdd := tessEvenOdd.TessellatePath(path, scene.IdentityAffine())

	// Both should produce strips
	if bufferNonZero.IsEmpty() {
		t.Error("NonZero buffer should not be empty")
	}
	if bufferEvenOdd.IsEmpty() {
		t.Error("EvenOdd buffer should not be empty")
	}

	// The fill rules should produce different results for overlapping paths
	// (Though in this simple test, the visual difference might not be measurable)
	if bufferNonZero.FillRule() != scene.FillNonZero {
		t.Error("NonZero buffer should have NonZero fill rule")
	}
	if bufferEvenOdd.FillRule() != scene.FillEvenOdd {
		t.Error("EvenOdd buffer should have EvenOdd fill rule")
	}
}

func TestTessellatorQuadCurve(t *testing.T) {
	tess := NewTessellator()

	path := scene.NewPath()
	path.MoveTo(0, 0)
	path.QuadTo(50, 100, 100, 0) // Parabola-like curve
	path.Close()

	buffer := tess.TessellatePath(path, scene.IdentityAffine())

	if buffer.IsEmpty() {
		t.Error("Buffer should not be empty for quad curve")
	}

	// Should produce strips for the curve area
	if buffer.StripCount() < 10 {
		t.Errorf("Strip count = %d, expected more for quad curve", buffer.StripCount())
	}
}

func TestTessellatorCubicCurve(t *testing.T) {
	tess := NewTessellator()

	path := scene.NewPath()
	path.MoveTo(0, 0)
	path.CubicTo(25, 100, 75, 100, 100, 0) // S-curve
	path.Close()

	buffer := tess.TessellatePath(path, scene.IdentityAffine())

	if buffer.IsEmpty() {
		t.Error("Buffer should not be empty for cubic curve")
	}

	if buffer.StripCount() < 10 {
		t.Errorf("Strip count = %d, expected more for cubic curve", buffer.StripCount())
	}
}

func TestTessellatorEmptyPath(t *testing.T) {
	tess := NewTessellator()

	path := scene.NewPath()
	buffer := tess.TessellatePath(path, scene.IdentityAffine())

	if !buffer.IsEmpty() {
		t.Error("Buffer should be empty for empty path")
	}
}

func TestTessellatorNilPath(t *testing.T) {
	tess := NewTessellator()

	buffer := tess.TessellatePath(nil, scene.IdentityAffine())

	if !buffer.IsEmpty() {
		t.Error("Buffer should be empty for nil path")
	}
}

func TestTessellatorReuse(t *testing.T) {
	tess := NewTessellator()

	// First tessellation
	path1 := scene.NewPath().Rectangle(0, 0, 10, 10)
	buffer1 := tess.TessellatePath(path1, scene.IdentityAffine())
	count1 := buffer1.StripCount()

	// Second tessellation (should reset and work correctly)
	path2 := scene.NewPath().Rectangle(0, 0, 20, 20)
	buffer2 := tess.TessellatePath(path2, scene.IdentityAffine())
	count2 := buffer2.StripCount()

	// Second should have more strips (larger rectangle)
	if count2 <= count1 {
		t.Errorf("Second tessellation should have more strips: %d <= %d", count2, count1)
	}
}

func TestTessellatorPool(t *testing.T) {
	pool := NewTessellatorPool()

	// Get tessellators
	t1 := pool.Get()
	t2 := pool.Get()

	if t1 == t2 {
		t.Error("Should get different tessellators")
	}

	// Return to pool
	pool.Put(t1)

	// Get again - should reuse
	t3 := pool.Get()
	if t3 != t1 {
		t.Error("Should reuse tessellator from pool")
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkTessellateRectangle100x100(b *testing.B) {
	tess := NewTessellator()
	path := scene.NewPath().Rectangle(0, 0, 100, 100)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tess.TessellatePath(path, transform)
	}
}

func BenchmarkTessellateRectangle1000x1000(b *testing.B) {
	tess := NewTessellator()
	path := scene.NewPath().Rectangle(0, 0, 1000, 1000)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tess.TessellatePath(path, transform)
	}
}

func BenchmarkTessellateCircle(b *testing.B) {
	tess := NewTessellator()
	path := scene.NewPath().Circle(50, 50, 50)
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tess.TessellatePath(path, transform)
	}
}

func BenchmarkTessellateComplexPath(b *testing.B) {
	tess := NewTessellator()

	// Create a complex path with multiple curves
	path := scene.NewPath()
	path.MoveTo(0, 0)
	for i := 0; i < 100; i++ {
		angle := float32(i) * 0.1
		x := 100 + 50*float32(math.Cos(float64(angle)))
		y := 100 + 50*float32(math.Sin(float64(angle)))
		path.LineTo(x, y)
	}
	path.Close()

	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tess.TessellatePath(path, transform)
	}
}

func BenchmarkStripBufferPackForGPU(b *testing.B) {
	sb := NewStripBuffer()

	// Add 100 strips
	for i := 0; i < 100; i++ {
		coverage := make([]uint8, 50)
		for j := range coverage {
			coverage[j] = uint8((i + j) % 256)
		}
		sb.AddStrip(i, i*10, coverage)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sb.PackForGPU()
	}
}

func BenchmarkStripBufferMergeAdjacent(b *testing.B) {
	// Prepare data outside the benchmark loop
	strips := make([]struct {
		y, x     int
		coverage []uint8
	}, 100)

	for i := 0; i < 100; i++ {
		coverage := make([]uint8, 10)
		for j := range coverage {
			coverage[j] = 255
		}
		strips[i] = struct {
			y, x     int
			coverage []uint8
		}{i / 10, (i % 10) * 10, coverage}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sb := NewStripBuffer()
		for _, s := range strips {
			sb.AddStrip(s.y, s.x, s.coverage)
		}
		sb.MergeAdjacent()
	}
}
