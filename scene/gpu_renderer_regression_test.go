package scene

import (
	"testing"

	gg "github.com/gogpu/gg"
)

// --- Regression: BUG-GG-GPU-SCENE-RENDERER-TEXT-001 ---

func TestGPUSceneRenderer_FillAppliesCTM(t *testing.T) {
	dc := gg.NewContext(200, 200)
	dc.Translate(50, 50)

	scene := NewScene()
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
		NewRectShape(0, 0, 10, 10))

	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(scene); err != nil {
		t.Fatalf("RenderScene: %v", err)
	}

	pm := dc.ResizeTarget()

	// With translate(50,50), rect at (0,0,10,10) → rendered at (50,50)-(60,60).
	px := pm.GetPixel(55, 55)
	if px.A == 0 {
		t.Error("REGRESSION: pixel at (55,55) transparent — CTM not applied to fill path")
	}

	// Pixel at (5,5) should be empty (rect translated away).
	px2 := pm.GetPixel(5, 5)
	if px2.A > 0 {
		t.Error("pixel at (5,5) should be transparent (rect translated to 50,50)")
	}
}

func TestGPUSceneRenderer_StrokeAppliesCTM(t *testing.T) {
	dc := gg.NewContext(200, 200)
	dc.Translate(100, 100)

	scene := NewScene()
	style := &StrokeStyle{Width: 3}
	scene.Stroke(style, IdentityAffine(), SolidBrush(gg.RGBA{R: 0, G: 1, B: 0, A: 1}),
		NewRectShape(0, 0, 20, 20))

	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(scene); err != nil {
		t.Fatalf("RenderScene: %v", err)
	}

	pm := dc.ResizeTarget()
	px := pm.GetPixel(100, 100) // top-left corner of stroked rect
	if px.A == 0 {
		t.Error("REGRESSION: pixel at (100,100) transparent — CTM not applied to stroke")
	}
}

func TestGPUSceneRenderer_TransformPreservesParentCTM(t *testing.T) {
	dc := gg.NewContext(200, 200)
	dc.Translate(20, 20) // parent transform

	scene := NewScene()
	scene.Fill(FillNonZero, TranslateAffine(30, 30), SolidBrush(gg.RGBA{R: 0, G: 0, B: 1, A: 1}),
		NewRectShape(0, 0, 10, 10))

	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(scene); err != nil {
		t.Fatalf("RenderScene: %v", err)
	}

	// Parent(20,20) + scene(30,30) = rect at (50,50).
	pm := dc.ResizeTarget()
	px := pm.GetPixel(55, 55)
	if px.A == 0 {
		t.Error("pixel at (55,55) transparent — parent+scene transforms not composed")
	}
}

func TestGPUSceneRenderer_FillRoundRect(t *testing.T) {
	dc := gg.NewContext(200, 200)

	scene := NewScene()
	rect := Rect{MinX: 10, MinY: 10, MaxX: 60, MaxY: 60}
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.RGBA{R: 1, G: 1, B: 0, A: 1}),
		NewRoundRectShape(rect, 5, 5))

	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(scene); err != nil {
		t.Fatalf("RenderScene: %v", err)
	}

	pm := dc.ResizeTarget()
	px := pm.GetPixel(35, 35) // center of rounded rect
	if px.A == 0 {
		t.Error("REGRESSION: FillRoundRect not rendered — TagFillRoundRect handler missing")
	}
}

func TestGPUSceneRenderer_MultipleTransformsNoStackCorruption(t *testing.T) {
	// Regression: BUG-002 — transformDepth counted ALL TagTransforms
	// but only 1 push was active. Cleanup popped N times → corrupted parent stack.
	dc := gg.NewContext(200, 200)
	dc.Push() // parent push — must survive RenderScene

	scene := NewScene()
	// 3 items with different transforms (like ListView items)
	scene.Fill(FillNonZero, TranslateAffine(10, 10), SolidBrush(gg.RGBA{R: 1, A: 1}),
		NewRectShape(0, 0, 5, 5))
	scene.Fill(FillNonZero, TranslateAffine(10, 30), SolidBrush(gg.RGBA{G: 1, A: 1}),
		NewRectShape(0, 0, 5, 5))
	scene.Fill(FillNonZero, TranslateAffine(10, 50), SolidBrush(gg.RGBA{B: 1, A: 1}),
		NewRectShape(0, 0, 5, 5))

	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(scene); err != nil {
		t.Fatalf("RenderScene: %v", err)
	}

	// Parent Pop must not panic — if stack corrupted, this panics or
	// restores wrong state.
	dc.Pop()

	// Verify items rendered at different positions (not all at first position).
	pm := dc.ResizeTarget()
	px1 := pm.GetPixel(12, 12) // item 1 at (10,10)
	px2 := pm.GetPixel(12, 32) // item 2 at (10,30)
	px3 := pm.GetPixel(12, 52) // item 3 at (10,50)

	if px1.A == 0 {
		t.Error("item 1 at (12,12) not rendered")
	}
	if px2.A == 0 {
		t.Error("item 2 at (12,32) not rendered")
	}
	if px3.A == 0 {
		t.Error("item 3 at (12,52) not rendered")
	}
}

// --- Regression: BUG-GG-GPU-SCENE-CLIP-001 ---
// TagTransform inside a BeginClip/EndClip region must NOT pop the clip's
// Push(). The old code used Push/Pop for transforms which corrupted the clip
// stack when a TagTransform appeared between BeginClip and EndClip.
// Fix: transforms use dc.SetTransform() (direct matrix replacement) instead
// of Push/Pop, reserving Push/Pop exclusively for clip/layer boundaries.

func TestGPUSceneRenderer_ClipAppliedToContent(t *testing.T) {
	// Build a scene with a clip rect and content that extends outside it.
	// Verify that the content is actually clipped.
	dc := gg.NewContext(100, 100)

	s := NewScene()
	// Clip to rect (10,10)-(90,90)
	clipShape := NewRectShape(10, 10, 80, 80)
	s.PushClip(clipShape)

	// Fill the entire canvas (0,0,100,100) with red — should be clipped to (10,10)-(90,90).
	s.Fill(FillNonZero, IdentityAffine(),
		SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
		NewRectShape(0, 0, 100, 100))

	s.PopClip()

	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(s); err != nil {
		t.Fatalf("RenderScene: %v", err)
	}

	pm := dc.ResizeTarget()

	// Inside clip: pixel at (50,50) should be red.
	inside := pm.GetPixel(50, 50)
	if inside.R < 0.5 || inside.A < 0.5 {
		t.Errorf("pixel at (50,50) inside clip: R=%.2f A=%.2f, want red", inside.R, inside.A)
	}

	// Outside clip: pixel at (5,5) should be empty (transparent/black).
	outside := pm.GetPixel(5, 5)
	if outside.A > 0.1 {
		t.Errorf("pixel at (5,5) outside clip: A=%.2f, want transparent (clip not applied)", outside.A)
	}
}

func TestGPUSceneRenderer_ClipWithTransformInside(t *testing.T) {
	// Regression test: a transform change INSIDE a clip region must not
	// destroy the clip. This is the exact pattern used by ui ScrollView:
	// parent sets a clip for the viewport, then each child widget has its
	// own transform (scroll offset).
	dc := gg.NewContext(100, 100)

	s := NewScene()
	// Clip to upper half: (0,0)-(100,50).
	s.PushClip(NewRectShape(0, 0, 100, 50))

	// Fill a rect at (10,10,30,80) with a different transform.
	// The rect extends from y=10 to y=90, but only y=0..50 should be visible.
	s.Fill(FillNonZero, TranslateAffine(10, 10),
		SolidBrush(gg.RGBA{R: 0, G: 1, B: 0, A: 1}),
		NewRectShape(0, 0, 30, 80))

	s.PopClip()

	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(s); err != nil {
		t.Fatalf("RenderScene: %v", err)
	}

	pm := dc.ResizeTarget()

	// Inside clip and content: (20,25) should be green.
	pxInside := pm.GetPixel(20, 25)
	if pxInside.G < 0.5 || pxInside.A < 0.5 {
		t.Errorf("pixel at (20,25) inside clip+content: G=%.2f A=%.2f, want green",
			pxInside.G, pxInside.A)
	}

	// Below clip (y=70): should be empty even though rect extends there.
	pxBelow := pm.GetPixel(20, 70)
	if pxBelow.A > 0.1 {
		t.Errorf("pixel at (20,70) below clip: A=%.2f, want transparent (clip not applied)",
			pxBelow.A)
	}
}

func TestGPUSceneRenderer_ClipWithMultipleChildTransforms(t *testing.T) {
	// Simulate a ScrollView with multiple child widgets, each with its own
	// transform. Tests that the clip survives across multiple TagTransform
	// changes inside the clip region.
	dc := gg.NewContext(200, 200)

	s := NewScene()
	// Clip to (10,10)-(190,100) — upper viewport area.
	s.PushClip(NewRectShape(10, 10, 180, 90))

	// Child 1 at offset (20,20): small rect.
	s.Fill(FillNonZero, TranslateAffine(20, 20),
		SolidBrush(gg.RGBA{R: 1, A: 1}),
		NewRectShape(0, 0, 40, 40))

	// Child 2 at offset (20,70): overlaps clip bottom edge.
	// Rect from y=70 to y=130, but clip ends at y=100.
	s.Fill(FillNonZero, TranslateAffine(20, 70),
		SolidBrush(gg.RGBA{B: 1, A: 1}),
		NewRectShape(0, 0, 40, 60))

	s.PopClip()

	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(s); err != nil {
		t.Fatalf("RenderScene: %v", err)
	}

	pm := dc.ResizeTarget()

	// Child 1 center (40,40) — inside clip → visible.
	px1 := pm.GetPixel(40, 40)
	if px1.R < 0.5 || px1.A < 0.5 {
		t.Errorf("child 1 at (40,40): R=%.2f A=%.2f, want red", px1.R, px1.A)
	}

	// Child 2 top (40,80) — inside clip → visible.
	px2 := pm.GetPixel(40, 80)
	if px2.B < 0.5 || px2.A < 0.5 {
		t.Errorf("child 2 at (40,80): B=%.2f A=%.2f, want blue", px2.B, px2.A)
	}

	// Child 2 bottom (40,120) — BELOW clip → must be clipped away.
	px3 := pm.GetPixel(40, 120)
	if px3.A > 0.1 {
		t.Errorf("child 2 at (40,120) below clip: A=%.2f, want transparent", px3.A)
	}

	// Outside clip area entirely (5,5) → must be empty.
	px4 := pm.GetPixel(5, 5)
	if px4.A > 0.1 {
		t.Errorf("outside clip at (5,5): A=%.2f, want transparent", px4.A)
	}
}

func TestGPUSceneRenderer_TransformRestoredAfterRender(t *testing.T) {
	// Verify that RenderScene restores the context's transform to what it
	// was before the call, including when the scene contains transforms.
	dc := gg.NewContext(100, 100)
	dc.Translate(42, 17) // arbitrary parent transform

	before := dc.GetTransform()

	s := NewScene()
	s.Fill(FillNonZero, TranslateAffine(10, 10),
		SolidBrush(gg.RGBA{R: 1, A: 1}),
		NewRectShape(0, 0, 5, 5))

	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(s); err != nil {
		t.Fatalf("RenderScene: %v", err)
	}

	after := dc.GetTransform()
	if before != after {
		t.Errorf("transform changed after RenderScene: before=%v, after=%v", before, after)
	}
}

func TestGPUSceneRenderer_NilScene(t *testing.T) {
	dc := gg.NewContext(100, 100)
	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(nil); err != nil {
		t.Errorf("RenderScene(nil) = %v, want nil", err)
	}
}

func TestGPUSceneRenderer_EmptyScene(t *testing.T) {
	dc := gg.NewContext(100, 100)
	renderer := NewGPUSceneRenderer(dc)
	if err := renderer.RenderScene(NewScene()); err != nil {
		t.Errorf("RenderScene(empty) = %v, want nil", err)
	}
}
