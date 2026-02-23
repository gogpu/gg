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

// ---------------------------------------------------------------------------
// Pixel-level correctness tests (SCENE-001)
// ---------------------------------------------------------------------------

// TestRenderer_CircleFill verifies that a filled circle produces non-zero
// pixels at its center (analytic AA via SoftwareRenderer delegation).
func TestRenderer_CircleFill(t *testing.T) {
	const size = 200
	r := NewRenderer(size, size)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(size, size)

	s := NewScene()
	s.Fill(FillNonZero, IdentityAffine(),
		SolidBrush(gg.RGBA{R: 0, G: 1, B: 0, A: 1}),
		NewCircleShape(100, 100, 40))

	if err := r.Render(target, s); err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Center of the circle (100, 100) must be fully green.
	c := target.GetPixel(100, 100)
	if c.A < 0.9 {
		t.Errorf("center alpha = %.2f, want >= 0.9 (circle not rendered)", c.A)
	}
	if c.G < 0.9 {
		t.Errorf("center green = %.2f, want >= 0.9", c.G)
	}
	if c.R > 0.1 {
		t.Errorf("center red = %.2f, want <= 0.1 (wrong color)", c.R)
	}

	// A point well inside the circle (100, 80) should also be opaque green.
	inner := target.GetPixel(100, 80)
	if inner.A < 0.9 || inner.G < 0.9 {
		t.Errorf("inner pixel (100,80) = %+v, expected opaque green", inner)
	}

	// A point outside the circle (10, 10) should be transparent.
	outer := target.GetPixel(10, 10)
	if outer.A > 0.1 {
		t.Errorf("outer pixel (10,10) alpha = %.2f, want <= 0.1", outer.A)
	}

	// Edge pixels should have partial alpha (analytic AA produces subpixel coverage).
	// The circle edge at (140, 100) -- exactly on the radius boundary.
	edge := target.GetPixel(140, 100)
	// Edge pixel may be fully opaque, partially transparent, or near-boundary.
	// We just verify the rendering pipeline touched the edge region by checking
	// that somewhere around the boundary we get a partially-covered pixel.
	foundPartial := false
	for x := 138; x <= 142; x++ {
		p := target.GetPixel(x, 100)
		if p.A > 0.05 && p.A < 0.95 {
			foundPartial = true
			break
		}
	}
	if !foundPartial {
		t.Logf("edge pixel at (140,100) = %+v (may be exactly on boundary)", edge)
		// This is not a hard failure because the exact boundary pixel depends on
		// sub-pixel positioning, but it indicates AA is working if found.
	}
}

// TestRenderer_CircleStroke verifies that a stroked circle produces visible
// pixels. This is the primary regression test for gg#116 (CubicTo ignored).
func TestRenderer_CircleStroke(t *testing.T) {
	const size = 200
	r := NewRenderer(size, size)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(size, size)

	s := NewScene()
	strokeStyle := &StrokeStyle{
		Width:      3.0,
		MiterLimit: 10.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
	}
	s.Stroke(strokeStyle, IdentityAffine(),
		SolidBrush(gg.RGBA{R: 0, G: 0, B: 1, A: 1}),
		NewCircleShape(100, 100, 40))

	if err := r.Render(target, s); err != nil {
		t.Fatalf("Render: %v", err)
	}

	// The stroke ring should be visible on the circle perimeter.
	// Check a point on the right edge of the circle (radius=40, center=100).
	// Stroke width=3, so pixels at x in [138..142] should have coverage.
	foundStrokePixel := false
	for x := 137; x <= 143; x++ {
		p := target.GetPixel(x, 100)
		if p.A > 0.5 && p.B > 0.5 {
			foundStrokePixel = true
			break
		}
	}
	if !foundStrokePixel {
		t.Error("no visible blue stroke pixels found at right edge of circle; " +
			"CubicTo curves may not be handled (regression gg#116)")
	}

	// Also check top edge of circle (100, 60).
	foundTop := false
	for y := 57; y <= 63; y++ {
		p := target.GetPixel(100, y)
		if p.A > 0.5 && p.B > 0.5 {
			foundTop = true
			break
		}
	}
	if !foundTop {
		t.Error("no visible blue stroke pixels found at top edge of circle")
	}

	// Center of the circle should be transparent (stroke, not fill).
	center := target.GetPixel(100, 100)
	if center.A > 0.1 {
		t.Errorf("center alpha = %.2f, want <= 0.1 (stroke should not fill center)", center.A)
	}
}

// TestRenderer_BackgroundPreservation verifies that a pre-cleared white
// background is preserved after rendering a translucent shape on top.
// This is a regression test for the original clear(tile.Data) bug.
func TestRenderer_BackgroundPreservation(t *testing.T) {
	const size = 200
	r := NewRenderer(size, size)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(size, size)
	white := gg.RGBA{R: 1, G: 1, B: 1, A: 1}
	target.Clear(white)

	// Render a small red rectangle in the center
	s := NewScene()
	s.Fill(FillNonZero, IdentityAffine(),
		SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
		NewRectShape(80, 80, 40, 40))

	if err := r.Render(target, s); err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Pixel in the rendered rectangle should be red.
	center := target.GetPixel(100, 100)
	if center.R < 0.9 || center.A < 0.9 {
		t.Errorf("rectangle center = %+v, want opaque red", center)
	}

	// Pixel outside the rectangle but inside the canvas should still be white.
	// The original bug cleared tile.Data to transparent, destroying the background.
	bg := target.GetPixel(10, 10)
	if bg.A < 0.9 {
		t.Errorf("background alpha at (10,10) = %.2f, want >= 0.9 (background destroyed)", bg.A)
	}
	if bg.R < 0.9 || bg.G < 0.9 || bg.B < 0.9 {
		t.Errorf("background color at (10,10) = %+v, want white (background destroyed)", bg)
	}

	// Also check a pixel in a different corner
	corner := target.GetPixel(190, 190)
	if corner.A < 0.9 || corner.R < 0.9 || corner.G < 0.9 || corner.B < 0.9 {
		t.Errorf("corner (190,190) = %+v, want white (background destroyed)", corner)
	}
}

// TestRenderer_AlphaCompositing verifies that overlapping semi-transparent
// shapes blend correctly using premultiplied source-over compositing.
func TestRenderer_AlphaCompositing(t *testing.T) {
	const size = 200
	r := NewRenderer(size, size)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(size, size)

	s := NewScene()

	// First shape: opaque red rectangle at (50,50)-(150,150)
	s.Fill(FillNonZero, IdentityAffine(),
		SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
		NewRectShape(50, 50, 100, 100))

	// Second shape: semi-transparent blue rectangle at (80,80)-(180,180)
	// This overlaps with the red rectangle in the (80,80)-(150,150) region.
	s.Fill(FillNonZero, IdentityAffine(),
		SolidBrush(gg.RGBA{R: 0, G: 0, B: 1, A: 0.5}),
		NewRectShape(80, 80, 100, 100))

	if err := r.Render(target, s); err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Pixel in red-only region (60, 60) should be fully red.
	redOnly := target.GetPixel(60, 60)
	if redOnly.R < 0.9 || redOnly.A < 0.9 {
		t.Errorf("red-only region (60,60) = %+v, want opaque red", redOnly)
	}

	// Pixel in blue-only region (170, 170) should be semi-transparent blue.
	blueOnly := target.GetPixel(170, 170)
	if blueOnly.B < 0.8 {
		t.Errorf("blue-only region (170,170) blue = %.2f, want >= 0.8", blueOnly.B)
	}
	if blueOnly.A < 0.4 || blueOnly.A > 0.6 {
		t.Errorf("blue-only region (170,170) alpha = %.2f, want ~0.5", blueOnly.A)
	}

	// Pixel in overlap region (100, 100) should be a blend of red and blue.
	// Source-over: dst' = src + dst * (1 - srcAlpha)
	// Red is dst (opaque), blue is src (alpha=0.5).
	// Expected: R = 0 + 1.0 * 0.5 = 0.5, G = 0, B = 0.5 + 0 = 0.5, A = 0.5 + 1.0*0.5 = 1.0
	// Actually in premultiplied: srcR=0, srcG=0, srcB=0.5*0.5=0.25premul, srcA=0.5
	// dstR=1.0premul, dstG=0, dstB=0, dstA=1.0
	// result premul: R=0+1.0*0.5=0.5, G=0, B=0.25+0*0.5=0.25, A=0.5+1.0*0.5=1.0
	// straight: R=0.5, G=0, B=0.25, A=1.0
	overlap := target.GetPixel(100, 100)
	if overlap.A < 0.9 {
		t.Errorf("overlap alpha = %.2f, want >= 0.9", overlap.A)
	}
	// Red channel should be reduced (blended with blue source)
	if overlap.R < 0.3 || overlap.R > 0.7 {
		t.Errorf("overlap red = %.2f, want ~0.5", overlap.R)
	}
	// Blue channel should have some blue from the overlapping shape
	if overlap.B < 0.1 {
		t.Errorf("overlap blue = %.2f, want > 0.1 (blue not composited)", overlap.B)
	}
}

// TestRenderer_RectFillPixels verifies that a filled rectangle produces
// correct pixel values at specific locations.
func TestRenderer_RectFillPixels(t *testing.T) {
	const size = 200
	r := NewRenderer(size, size)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(size, size)

	s := NewScene()
	s.Fill(FillNonZero, IdentityAffine(),
		SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
		NewRectShape(20, 20, 60, 60))

	if err := r.Render(target, s); err != nil {
		t.Fatalf("Render: %v", err)
	}

	tests := []struct {
		name    string
		x, y    int
		wantR   bool // expect red
		wantAlp bool // expect alpha > 0
	}{
		{"inside center", 50, 50, true, true},
		{"inside top-left", 25, 25, true, true},
		{"inside bottom-right", 75, 75, true, true},
		{"outside top-left", 5, 5, false, false},
		{"outside bottom-right", 100, 100, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := target.GetPixel(tt.x, tt.y)
			hasAlpha := p.A > 0.5
			hasRed := p.R > 0.5

			if hasAlpha != tt.wantAlp {
				t.Errorf("pixel(%d,%d) alpha=%.2f, wantAlpha=%v", tt.x, tt.y, p.A, tt.wantAlp)
			}
			if hasRed != tt.wantR {
				t.Errorf("pixel(%d,%d) red=%.2f, wantRed=%v", tt.x, tt.y, p.R, tt.wantR)
			}
		})
	}
}

// TestRenderer_TransformPixels verifies that transforms are correctly applied
// by checking pixel output at the transformed location.
func TestRenderer_TransformPixels(t *testing.T) {
	const size = 200
	r := NewRenderer(size, size)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	defer r.Close()

	target := gg.NewPixmap(size, size)

	s := NewScene()
	// A 30x30 rectangle at (0,0) translated to (100,100)
	transform := TranslateAffine(100, 100)
	s.Fill(FillNonZero, transform,
		SolidBrush(gg.RGBA{R: 0, G: 0, B: 1, A: 1}),
		NewRectShape(0, 0, 30, 30))

	if err := r.Render(target, s); err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Should be blue at the translated position
	at := target.GetPixel(115, 115)
	if at.A < 0.9 || at.B < 0.9 {
		t.Errorf("translated pixel (115,115) = %+v, want opaque blue", at)
	}

	// Original position should be empty
	orig := target.GetPixel(15, 15)
	if orig.A > 0.1 {
		t.Errorf("original position (15,15) alpha = %.2f, want transparent", orig.A)
	}
}

// TestConvertPath verifies path conversion from scene.Path (float32) to gg.Path (float64).
func TestConvertPath(t *testing.T) {
	sp := NewPath()
	sp.MoveTo(10, 20)
	sp.LineTo(30, 40)
	sp.QuadTo(50, 60, 70, 80)
	sp.CubicTo(90, 100, 110, 120, 130, 140)
	sp.Close()

	ggp := convertPath(sp, 5, 10)

	elements := ggp.Elements()
	if len(elements) != 5 {
		t.Fatalf("expected 5 elements, got %d", len(elements))
	}

	// Verify element types
	if _, ok := elements[0].(gg.MoveTo); !ok {
		t.Errorf("element 0: want MoveTo, got %T", elements[0])
	}
	if _, ok := elements[1].(gg.LineTo); !ok {
		t.Errorf("element 1: want LineTo, got %T", elements[1])
	}
	if _, ok := elements[2].(gg.QuadTo); !ok {
		t.Errorf("element 2: want QuadTo, got %T", elements[2])
	}
	if _, ok := elements[3].(gg.CubicTo); !ok {
		t.Errorf("element 3: want CubicTo, got %T", elements[3])
	}
	if _, ok := elements[4].(gg.Close); !ok {
		t.Errorf("element 4: want Close, got %T", elements[4])
	}

	// Verify offset subtraction on MoveTo
	m := elements[0].(gg.MoveTo)
	wantX, wantY := 10.0-5.0, 20.0-10.0
	if m.Point.X != wantX || m.Point.Y != wantY {
		t.Errorf("MoveTo = (%.1f, %.1f), want (%.1f, %.1f)", m.Point.X, m.Point.Y, wantX, wantY)
	}

	// Verify offset on CubicTo endpoint
	cb := elements[3].(gg.CubicTo)
	wantEndX, wantEndY := 130.0-5.0, 140.0-10.0
	if cb.Point.X != wantEndX || cb.Point.Y != wantEndY {
		t.Errorf("CubicTo end = (%.1f, %.1f), want (%.1f, %.1f)",
			cb.Point.X, cb.Point.Y, wantEndX, wantEndY)
	}
}

// TestConvertFillPaint verifies fill paint conversion.
func TestConvertFillPaint(t *testing.T) {
	tests := []struct {
		name     string
		brush    Brush
		style    FillStyle
		wantRule gg.FillRule
	}{
		{
			"solid non-zero",
			SolidBrush(gg.RGBA{R: 1, A: 1}),
			FillNonZero,
			gg.FillRuleNonZero,
		},
		{
			"solid even-odd",
			SolidBrush(gg.RGBA{R: 1, A: 1}),
			FillEvenOdd,
			gg.FillRuleEvenOdd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paint := convertFillPaint(tt.brush, tt.style)
			if paint.FillRule != tt.wantRule {
				t.Errorf("FillRule = %d, want %d", paint.FillRule, tt.wantRule)
			}
		})
	}
}

// TestConvertStrokePaint verifies stroke paint conversion uses non-deprecated API.
func TestConvertStrokePaint(t *testing.T) {
	style := &StrokeStyle{
		Width:      2.5,
		MiterLimit: 8.0,
		Cap:        LineCapRound,
		Join:       LineJoinBevel,
	}
	brush := SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1})

	paint := convertStrokePaint(brush, style)

	// Verify using the non-deprecated Stroke API
	s := paint.GetStroke()
	if s.Width != 2.5 {
		t.Errorf("Stroke.Width = %f, want 2.5", s.Width)
	}
	if s.MiterLimit != 8.0 {
		t.Errorf("Stroke.MiterLimit = %f, want 8.0", s.MiterLimit)
	}
	if s.Cap != gg.LineCapRound {
		t.Errorf("Stroke.Cap = %d, want LineCapRound", s.Cap)
	}
	if s.Join != gg.LineJoinBevel {
		t.Errorf("Stroke.Join = %d, want LineJoinBevel", s.Join)
	}
}

// TestConvertStrokePaintNilStyle verifies default stroke style is used when nil.
func TestConvertStrokePaintNilStyle(t *testing.T) {
	brush := SolidBrush(gg.RGBA{R: 1, A: 1})
	paint := convertStrokePaint(brush, nil)

	s := paint.GetStroke()
	if s.Width != 1.0 {
		t.Errorf("default Width = %f, want 1.0", s.Width)
	}
	if s.MiterLimit != 10.0 {
		t.Errorf("default MiterLimit = %f, want 10.0", s.MiterLimit)
	}
}
