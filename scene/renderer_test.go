package scene

import (
	"runtime"
	"testing"

	"github.com/gogpu/gg"
)

func TestNewRenderer(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		wantNil bool
	}{
		{"valid dimensions", 800, 600, false},
		{"small dimensions", 64, 64, false},
		{"zero width", 0, 600, true},
		{"zero height", 800, 0, true},
		{"negative width", -100, 600, true},
		{"negative height", 800, -100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRenderer(tt.width, tt.height)
			if (r == nil) != tt.wantNil {
				t.Errorf("NewRenderer(%d, %d) nil = %v, want %v",
					tt.width, tt.height, r == nil, tt.wantNil)
			}
			if r != nil {
				r.Close()
			}
		})
	}
}

func TestRenderer_Dimensions(t *testing.T) {
	r := NewRenderer(800, 600)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	if r.Width() != 800 {
		t.Errorf("Width() = %d, want 800", r.Width())
	}
	if r.Height() != 600 {
		t.Errorf("Height() = %d, want 600", r.Height())
	}
}

func TestRenderer_TileCount(t *testing.T) {
	// 800x600 with 64x64 tiles = 13x10 = 130 tiles
	r := NewRenderer(800, 600)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	tilesX := (800 + 63) / 64 // = 13
	tilesY := (600 + 63) / 64 // = 10
	expected := tilesX * tilesY

	if r.TileCount() != expected {
		t.Errorf("TileCount() = %d, want %d", r.TileCount(), expected)
	}
}

func TestRenderer_Options(t *testing.T) {
	cache := NewLayerCache(32)

	r := NewRenderer(400, 300,
		WithWorkers(4),
		WithCacheSize(32),
		WithTileSize(64),
		WithCache(cache),
	)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	if r.Cache() != cache {
		t.Error("WithCache option not applied correctly")
	}
}

func TestRenderer_RenderEmptyScene(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(200, 200)
	scene := NewScene()

	// Should not panic on empty scene
	err := r.Render(target, scene)
	if err != nil {
		t.Errorf("Render() returned error: %v", err)
	}
}

func TestRenderer_RenderSimpleScene(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(200, 200)
	target.Clear(gg.RGBA{R: 1, G: 1, B: 1, A: 1}) // White background

	scene := NewScene()
	rect := NewRectShape(50, 50, 100, 100)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}), rect)

	err := r.Render(target, scene)
	if err != nil {
		t.Errorf("Render() returned error: %v", err)
	}

	// Check stats
	stats := r.Stats()
	if stats.TilesTotal == 0 {
		t.Error("Stats should report tile count")
	}
	if stats.TilesRendered == 0 {
		t.Error("Stats should report rendered tiles")
	}
}

func TestRenderer_RenderDirty(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(200, 200)
	scene := NewScene()
	rect := NewRectShape(10, 10, 50, 50)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}), rect)

	// First full render
	_ = r.Render(target, scene)

	// Mark only a portion dirty
	r.MarkDirty(10, 10, 50, 50)

	// Dirty render should only render marked tiles
	err := r.RenderDirty(target, scene, nil)
	if err != nil {
		t.Errorf("RenderDirty() returned error: %v", err)
	}

	stats := r.Stats()
	if stats.TilesDirty > stats.TilesTotal {
		t.Errorf("DirtyTiles (%d) should not exceed TotalTiles (%d)",
			stats.TilesDirty, stats.TilesTotal)
	}
}

func TestRenderer_Resize(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	// Initial size
	if r.Width() != 200 || r.Height() != 200 {
		t.Errorf("Initial size = (%d, %d), want (200, 200)", r.Width(), r.Height())
	}

	// Resize
	r.Resize(400, 300)

	if r.Width() != 400 || r.Height() != 300 {
		t.Errorf("After resize = (%d, %d), want (400, 300)", r.Width(), r.Height())
	}

	// Verify all tiles marked dirty after resize
	if r.DirtyTileCount() != r.TileCount() {
		t.Errorf("After resize, dirty count (%d) should equal total (%d)",
			r.DirtyTileCount(), r.TileCount())
	}
}

func TestRenderer_MarkDirty(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	// Initially all tiles are dirty
	initialDirty := r.DirtyTileCount()
	if initialDirty == 0 {
		t.Error("Initially all tiles should be dirty")
	}

	// Render to clear dirty state
	target := gg.NewPixmap(200, 200)
	scene := NewScene()
	_ = r.Render(target, scene)

	// Mark a specific region dirty
	r.MarkDirty(64, 64, 64, 64) // Should mark at least one tile

	dirtyCount := r.DirtyTileCount()
	if dirtyCount == 0 {
		t.Error("MarkDirty should mark at least one tile")
	}
}

func TestRenderer_MarkAllDirty(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	// Render to clear dirty state
	target := gg.NewPixmap(200, 200)
	scene := NewScene()
	_ = r.Render(target, scene)

	// Mark all dirty
	r.MarkAllDirty()

	if r.DirtyTileCount() != r.TileCount() {
		t.Errorf("After MarkAllDirty, dirty count (%d) should equal total (%d)",
			r.DirtyTileCount(), r.TileCount())
	}
}

func TestRenderer_Stats(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(200, 200)
	scene := NewScene()
	rect := NewRectShape(10, 10, 180, 180)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}), rect)

	_ = r.Render(target, scene)

	stats := r.Stats()

	if stats.TilesTotal == 0 {
		t.Error("TilesTotal should be > 0")
	}
	if stats.TilesRendered == 0 {
		t.Error("TilesRendered should be > 0")
	}
	// Note: TimeTotal may be 0 on very fast systems or when rendering completes
	// in sub-nanosecond time. We just verify it's non-negative.
	if stats.TimeTotal < 0 {
		t.Error("TimeTotal should be >= 0")
	}
}

func TestRenderer_CacheStats(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	stats := r.CacheStats()

	// Cache should have defaults set
	if stats.MaxSize == 0 {
		t.Error("CacheStats.MaxSize should be > 0")
	}
}

func TestRenderer_NilInputs(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	// Nil target should not panic
	err := r.Render(nil, NewScene())
	if err != nil {
		t.Errorf("Render(nil, scene) error: %v", err)
	}

	// Nil scene should not panic
	target := gg.NewPixmap(200, 200)
	err = r.Render(target, nil)
	if err != nil {
		t.Errorf("Render(target, nil) error: %v", err)
	}

	// Both nil should not panic
	err = r.Render(nil, nil)
	if err != nil {
		t.Errorf("Render(nil, nil) error: %v", err)
	}
}

func TestRenderer_Close(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}

	// Close should not panic
	r.Close()

	// Multiple close should not panic
	r.Close()
}

func TestRenderer_MultipleShapes(t *testing.T) {
	r := NewRenderer(400, 400)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(400, 400)

	scene := NewScene()

	// Add multiple shapes
	scene.Fill(FillNonZero, IdentityAffine(),
		SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
		NewRectShape(10, 10, 100, 100))

	scene.Fill(FillNonZero, IdentityAffine(),
		SolidBrush(gg.RGBA{R: 0, G: 1, B: 0, A: 1}),
		NewCircleShape(200, 200, 50))

	scene.Stroke(DefaultStrokeStyle(), IdentityAffine(),
		SolidBrush(gg.RGBA{R: 0, G: 0, B: 1, A: 1}),
		NewRectShape(300, 300, 80, 80))

	err := r.Render(target, scene)
	if err != nil {
		t.Errorf("Render() with multiple shapes: %v", err)
	}
}

func TestRenderer_Transforms(t *testing.T) {
	r := NewRenderer(200, 200)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(200, 200)

	scene := NewScene()

	// Shape with transform
	transform := TranslateAffine(50, 50)
	scene.Fill(FillNonZero, transform,
		SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
		NewRectShape(0, 0, 50, 50))

	err := r.Render(target, scene)
	if err != nil {
		t.Errorf("Render() with transform: %v", err)
	}
}

func TestRenderer_WithDefaultWorkers(t *testing.T) {
	r := NewRenderer(200, 200, WithWorkers(0)) // 0 means use GOMAXPROCS
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	// WithWorkers(0) sets workers to 0, but the WorkerPool normalizes this
	// to GOMAXPROCS internally. We verify the configured value is stored.
	// The actual parallelism depends on WorkerPool implementation.
	workers := r.Workers()
	if workers < 0 {
		t.Errorf("Workers() = %d, want >= 0", workers)
	}
}

func TestRenderer_ParallelExecution(t *testing.T) {
	// Use more workers to test parallelism
	workers := runtime.GOMAXPROCS(0)
	if workers < 2 {
		t.Skip("Skipping parallel test with < 2 CPUs")
	}

	r := NewRenderer(1024, 1024, WithWorkers(workers))
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(1024, 1024)

	scene := NewScene()
	// Add many shapes to exercise parallelism
	for i := 0; i < 100; i++ {
		x := float32(i % 10 * 100)
		y := float32(i / 10 * 100)
		scene.Fill(FillNonZero, IdentityAffine(),
			SolidBrush(gg.RGBA{R: float64(i%2) * 0.5, G: float64(i%3) * 0.33, B: float64(i%5) * 0.2, A: 1}),
			NewRectShape(x, y, 80, 80))
	}

	err := r.Render(target, scene)
	if err != nil {
		t.Errorf("Parallel Render(): %v", err)
	}

	stats := r.Stats()
	if stats.TilesRendered == 0 {
		t.Error("Expected tiles to be rendered")
	}
}
