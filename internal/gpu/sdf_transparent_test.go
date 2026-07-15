//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
)

// TestFillShape_SkipsZeroAlpha verifies that FillShape via the draw queue
// skips shapes with zero alpha color at ScissorGroup build time (BUG-SDF-001).
// This prevents transparent fills from interfering with subsequent strokes
// via MSAA coverage weighting.
// Enterprise pattern: Skia nothingToDraw(), Cairo nothing_to_do().
func TestFillShape_SkipsZeroAlpha(t *testing.T) {
	shared := NewGPUShared()
	rc := shared.NewRenderContext()

	target := gg.GPURenderTarget{Width: 100, Height: 100, Data: make([]byte, 100*100*4), Stride: 400}

	// Transparent fill — queued but skipped at ScissorGroup build.
	transparentPaint := gg.NewPaint()
	transparentPaint.SetBrush(gg.Solid(gg.RGBA{R: 0, G: 0, B: 0, A: 0}))

	shape := gg.DetectedShape{
		Kind:         gg.ShapeRRect,
		CenterX:      50,
		CenterY:      50,
		Width:        80,
		Height:       60,
		CornerRadius: 8,
	}

	err := rc.FillShape(target, shape, transparentPaint)
	if err != nil {
		t.Fatalf("FillShape(transparent): %v", err)
	}
	// The draw command is queued, but the ScissorGroup builder will skip
	// zero-alpha shapes. Verify it's queued:
	if rc.PendingCount() != 1 {
		t.Errorf("expected 1 pending draw, got %d", rc.PendingCount())
	}

	// Build groups — zero-alpha should be filtered out.
	groups := rc.buildScissorGroupsFromDraws()
	if len(groups) == 0 {
		t.Fatal("expected at least 1 group")
	}
	if len(groups[0].SDFShapes) != 0 {
		t.Errorf("zero-alpha shape should be skipped in ScissorGroup, got %d SDFShapes", len(groups[0].SDFShapes))
	}
}

// TestFillShape_KeepsSemiTransparent verifies that shapes with partial
// alpha (e.g., 0.5) are NOT skipped — only fully transparent (alpha=0).
func TestFillShape_KeepsSemiTransparent(t *testing.T) {
	shared := NewGPUShared()
	rc := shared.NewRenderContext()

	target := gg.GPURenderTarget{Width: 100, Height: 100, Data: make([]byte, 100*100*4), Stride: 400}

	semiPaint := gg.NewPaint()
	semiPaint.SetBrush(gg.Solid(gg.RGBA{R: 1, G: 0, B: 0, A: 0.5}))

	shape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 30,
		RadiusY: 30,
	}

	err := rc.FillShape(target, shape, semiPaint)
	if err != nil {
		t.Fatalf("FillShape(semi-transparent): %v", err)
	}
	if rc.PendingCount() != 1 {
		t.Errorf("semi-transparent shape should be queued, got %d pending", rc.PendingCount())
	}

	// Build groups — semi-transparent should be kept.
	groups := rc.buildScissorGroupsFromDraws()
	if len(groups) == 0 {
		t.Fatal("expected at least 1 group")
	}
	if len(groups[0].SDFShapes) != 1 {
		t.Errorf("semi-transparent shape should be in ScissorGroup, got %d SDFShapes", len(groups[0].SDFShapes))
	}
}
