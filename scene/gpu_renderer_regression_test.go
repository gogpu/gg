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
