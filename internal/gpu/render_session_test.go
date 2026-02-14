//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestRenderSessionCreation(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	if s == nil {
		t.Fatal("expected non-nil session")
	}

	w, h := s.Size()
	if w != 0 || h != 0 {
		t.Errorf("expected size (0, 0) before EnsureTextures, got (%d, %d)", w, h)
	}

	s.Destroy()

	// Double-destroy should be safe.
	s.Destroy()
}

func TestRenderSessionTextures(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	err := s.EnsureTextures(800, 600)
	if err != nil {
		t.Fatalf("EnsureTextures failed: %v", err)
	}

	w, h := s.Size()
	if w != 800 || h != 600 {
		t.Errorf("expected size (800, 600), got (%d, %d)", w, h)
	}

	// Verify all textures exist.
	if s.textures.msaaTex == nil {
		t.Error("expected non-nil msaaTex")
	}
	if s.textures.msaaView == nil {
		t.Error("expected non-nil msaaView")
	}
	if s.textures.stencilTex == nil {
		t.Error("expected non-nil stencilTex")
	}
	if s.textures.stencilView == nil {
		t.Error("expected non-nil stencilView")
	}
	if s.textures.resolveTex == nil {
		t.Error("expected non-nil resolveTex")
	}
	if s.textures.resolveView == nil {
		t.Error("expected non-nil resolveView")
	}
}

func TestRenderSessionTexturesIdempotent(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	err := s.EnsureTextures(640, 480)
	if err != nil {
		t.Fatalf("first EnsureTextures failed: %v", err)
	}

	origMSAA := s.textures.msaaTex
	origStencil := s.textures.stencilTex
	origResolve := s.textures.resolveTex

	// Same dimensions should be a no-op.
	err = s.EnsureTextures(640, 480)
	if err != nil {
		t.Fatalf("second EnsureTextures failed: %v", err)
	}

	if s.textures.msaaTex != origMSAA {
		t.Error("MSAA texture was recreated unnecessarily")
	}
	if s.textures.stencilTex != origStencil {
		t.Error("stencil texture was recreated unnecessarily")
	}
	if s.textures.resolveTex != origResolve {
		t.Error("resolve texture was recreated unnecessarily")
	}
}

func TestRenderSessionTexturesResize(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	err := s.EnsureTextures(800, 600)
	if err != nil {
		t.Fatalf("initial EnsureTextures failed: %v", err)
	}

	err = s.EnsureTextures(1920, 1080)
	if err != nil {
		t.Fatalf("resize EnsureTextures failed: %v", err)
	}

	w, h := s.Size()
	if w != 1920 || h != 1080 {
		t.Errorf("expected (1920, 1080), got (%d, %d)", w, h)
	}

	if s.textures.msaaTex == nil || s.textures.stencilTex == nil || s.textures.resolveTex == nil {
		t.Error("expected non-nil textures after resize")
	}
}

func TestRenderSessionDestroyAndRecreate(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)

	err := s.EnsureTextures(256, 256)
	if err != nil {
		t.Fatalf("first EnsureTextures failed: %v", err)
	}

	s.Destroy()

	w, h := s.Size()
	if w != 0 || h != 0 {
		t.Errorf("expected (0, 0) after Destroy, got (%d, %d)", w, h)
	}

	err = s.EnsureTextures(512, 512)
	if err != nil {
		t.Fatalf("EnsureTextures after Destroy failed: %v", err)
	}
	defer s.Destroy()

	w, h = s.Size()
	if w != 512 || h != 512 {
		t.Errorf("expected (512, 512) after re-creation, got (%d, %d)", w, h)
	}
}

func TestRenderSessionEmpty(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	target := gg.GPURenderTarget{
		Width:  100,
		Height: 100,
		Data:   make([]uint8, 100*100*4),
		Stride: 100 * 4,
	}

	// Empty frame should return nil without creating textures.
	err := s.RenderFrame(target, nil, nil, nil)
	if err != nil {
		t.Fatalf("RenderFrame(nil, nil) failed: %v", err)
	}

	err = s.RenderFrame(target, []SDFRenderShape{}, nil, []StencilPathCommand{})
	if err != nil {
		t.Fatalf("RenderFrame([], []) failed: %v", err)
	}

	// Textures should not have been created for empty frames.
	if s.textures.msaaTex != nil {
		t.Error("expected nil msaaTex after empty RenderFrame")
	}
}

func TestRenderSessionSDFOnly(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	target := gg.GPURenderTarget{
		Width:  200,
		Height: 200,
		Data:   make([]uint8, 200*200*4),
		Stride: 200 * 4,
	}

	shapes := []SDFRenderShape{
		{
			Kind: 0, CenterX: 100, CenterY: 100,
			Param1: 50, Param2: 50,
			ColorR: 1, ColorG: 0, ColorB: 0, ColorA: 1,
		},
	}

	// This tests the full pipeline creation and render pass encoding
	// with the noop device.
	err := s.RenderFrame(target, shapes, nil, nil)
	if err != nil {
		t.Fatalf("RenderFrame with SDF shapes failed: %v", err)
	}

	// Verify textures were created.
	w, h := s.Size()
	if w != 200 || h != 200 {
		t.Errorf("expected size (200, 200), got (%d, %d)", w, h)
	}

	// Verify pipelines were created.
	if s.sdfPipeline == nil {
		t.Error("expected non-nil SDF pipeline after render")
	}
	if s.sdfPipeline.pipelineWithStencil == nil {
		t.Error("expected non-nil pipelineWithStencil after render")
	}
}

func TestRenderSessionStencilOnly(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	target := gg.GPURenderTarget{
		Width:  200,
		Height: 200,
		Data:   make([]uint8, 200*200*4),
		Stride: 200 * 4,
	}

	// Triangle fan for a simple triangle path.
	paths := []StencilPathCommand{
		{
			Vertices: []float32{
				50, 50, 150, 50, 150, 150, // triangle 1
			},
			CoverQuad: [12]float32{
				49, 49, 151, 49, 151, 151,
				49, 49, 151, 151, 49, 151,
			},
			Color:    [4]float32{0, 1, 0, 1},
			FillRule: gg.FillRuleNonZero,
		},
	}

	err := s.RenderFrame(target, nil, nil, paths)
	if err != nil {
		t.Fatalf("RenderFrame with stencil paths failed: %v", err)
	}

	if s.stencilRenderer == nil {
		t.Error("expected non-nil stencil renderer after render")
	}
}

func TestRenderSessionMixed(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	target := gg.GPURenderTarget{
		Width:  400,
		Height: 300,
		Data:   make([]uint8, 400*300*4),
		Stride: 400 * 4,
	}

	// SDF shapes (circles + rrects).
	shapes := []SDFRenderShape{
		{
			Kind: 0, CenterX: 100, CenterY: 100,
			Param1: 40, Param2: 40,
			ColorR: 1, ColorG: 0, ColorB: 0, ColorA: 1,
		},
		{
			Kind: 1, CenterX: 300, CenterY: 100,
			Param1: 50, Param2: 30, Param3: 8,
			ColorR: 0, ColorG: 0, ColorB: 1, ColorA: 1,
		},
	}

	// Stencil paths (arbitrary shape).
	paths := []StencilPathCommand{
		{
			Vertices: []float32{
				200, 200, 250, 200, 250, 250,
				200, 200, 250, 250, 200, 250,
			},
			CoverQuad: [12]float32{
				199, 199, 251, 199, 251, 251,
				199, 199, 251, 251, 199, 251,
			},
			Color:    [4]float32{0, 0.5, 0, 0.5},
			FillRule: gg.FillRuleEvenOdd,
		},
	}

	err := s.RenderFrame(target, shapes, nil, paths)
	if err != nil {
		t.Fatalf("RenderFrame with mixed content failed: %v", err)
	}

	// Both pipeline types should be initialized.
	if s.sdfPipeline == nil || s.sdfPipeline.pipelineWithStencil == nil {
		t.Error("expected SDF pipelines after mixed render")
	}
	if s.stencilRenderer == nil || s.stencilRenderer.nonZeroStencilPipeline == nil {
		t.Error("expected stencil pipelines after mixed render")
	}
}

func TestRenderSessionMultipleFrames(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	target := gg.GPURenderTarget{
		Width:  200,
		Height: 200,
		Data:   make([]uint8, 200*200*4),
		Stride: 200 * 4,
	}

	shapes := []SDFRenderShape{
		{
			Kind: 0, CenterX: 100, CenterY: 100,
			Param1: 30, Param2: 30, ColorA: 1,
		},
	}

	// Multiple frames should reuse textures and pipelines.
	for i := 0; i < 3; i++ {
		err := s.RenderFrame(target, shapes, nil, nil)
		if err != nil {
			t.Fatalf("frame %d failed: %v", i, err)
		}
	}

	w, h := s.Size()
	if w != 200 || h != 200 {
		t.Errorf("expected consistent size, got (%d, %d)", w, h)
	}
}

func TestRenderSessionPipelineSetters(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	// Initially nil.
	if s.SDFPipeline() != nil {
		t.Error("expected nil SDF pipeline initially")
	}
	if s.StencilRendererRef() != nil {
		t.Error("expected nil stencil renderer initially")
	}

	// Set external pipelines.
	sdfP := NewSDFRenderPipeline(device, queue)
	defer sdfP.Destroy()
	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	s.SetSDFPipeline(sdfP)
	s.SetStencilRenderer(sr)

	if s.SDFPipeline() != sdfP {
		t.Error("SetSDFPipeline did not set correctly")
	}
	if s.StencilRendererRef() != sr {
		t.Error("SetStencilRenderer did not set correctly")
	}
}

func TestStencilPathCommandFields(t *testing.T) {
	cmd := StencilPathCommand{
		Vertices: []float32{0, 0, 100, 0, 100, 100},
		CoverQuad: [12]float32{
			-1, -1, 101, -1, 101, 101,
			-1, -1, 101, 101, -1, 101,
		},
		Color:    [4]float32{1.0, 0.5, 0.25, 0.75},
		FillRule: gg.FillRuleEvenOdd,
	}

	if len(cmd.Vertices) != 6 {
		t.Errorf("expected 6 vertex floats, got %d", len(cmd.Vertices))
	}
	if cmd.Color[0] != 1.0 || cmd.Color[3] != 0.75 {
		t.Errorf("unexpected color: %v", cmd.Color)
	}
	if cmd.FillRule != gg.FillRuleEvenOdd {
		t.Errorf("expected EvenOdd fill rule, got %v", cmd.FillRule)
	}
}

func TestSDFRenderPipelineWithStencil(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)
	defer p.Destroy()

	err := p.ensurePipelineWithStencil()
	if err != nil {
		t.Fatalf("ensurePipelineWithStencil failed: %v", err)
	}

	// Both pipeline variants should exist.
	if p.pipeline == nil {
		t.Error("expected non-nil base pipeline")
	}
	if p.pipelineWithStencil == nil {
		t.Error("expected non-nil pipelineWithStencil")
	}

	// Calling again should be idempotent.
	origPipeline := p.pipelineWithStencil
	err = p.ensurePipelineWithStencil()
	if err != nil {
		t.Fatalf("second ensurePipelineWithStencil failed: %v", err)
	}
	if p.pipelineWithStencil != origPipeline {
		t.Error("pipelineWithStencil was recreated unnecessarily")
	}
}

func TestSDFRenderPipelineDestroyWithStencilVariant(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)

	err := p.ensurePipelineWithStencil()
	if err != nil {
		t.Fatalf("ensurePipelineWithStencil failed: %v", err)
	}

	p.Destroy()

	if p.pipeline != nil {
		t.Error("expected nil pipeline after Destroy")
	}
	if p.pipelineWithStencil != nil {
		t.Error("expected nil pipelineWithStencil after Destroy")
	}
}

// createMockSurfaceView creates a texture and view that simulates a window
// surface for testing surface rendering mode. The caller must destroy the
// texture and view when done.
func createMockSurfaceView(t *testing.T, device hal.Device, w, h uint32) (hal.Texture, hal.TextureView) {
	t.Helper()
	tex, err := device.CreateTexture(&hal.TextureDescriptor{
		Label:         "mock_surface",
		Size:          hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("create mock surface texture: %v", err)
	}
	view, err := device.CreateTextureView(tex, &hal.TextureViewDescriptor{
		Label: "mock_surface_view",
	})
	if err != nil {
		device.DestroyTexture(tex)
		t.Fatalf("create mock surface view: %v", err)
	}
	return tex, view
}

func TestRenderSessionSurfaceMode(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	// Initially in offscreen mode.
	if s.RenderMode() != RenderModeOffscreen {
		t.Errorf("expected offscreen mode initially, got %d", s.RenderMode())
	}

	// Set surface target.
	tex, view := createMockSurfaceView(t, device, 800, 600)
	defer device.DestroyTextureView(view)
	defer device.DestroyTexture(tex)

	s.SetSurfaceTarget(view, 800, 600)

	if s.RenderMode() != RenderModeSurface {
		t.Errorf("expected surface mode after SetSurfaceTarget, got %d", s.RenderMode())
	}

	// Render a frame with SDF shapes in surface mode.
	target := gg.GPURenderTarget{
		Width:  800,
		Height: 600,
		Data:   make([]uint8, 800*600*4),
		Stride: 800 * 4,
	}
	shapes := []SDFRenderShape{
		{
			Kind: 0, CenterX: 400, CenterY: 300,
			Param1: 100, Param2: 100,
			ColorR: 1, ColorG: 0, ColorB: 0, ColorA: 1,
		},
	}

	err := s.RenderFrame(target, shapes, nil, nil)
	if err != nil {
		t.Fatalf("surface mode RenderFrame failed: %v", err)
	}

	// Verify textures were created (MSAA + stencil but NOT resolve).
	if s.textures.msaaTex == nil {
		t.Error("expected non-nil msaaTex in surface mode")
	}
	if s.textures.stencilTex == nil {
		t.Error("expected non-nil stencilTex in surface mode")
	}
	// Resolve texture should be nil -- surface view is the resolve target.
	if s.textures.resolveTex != nil {
		t.Error("expected nil resolveTex in surface mode (surface is resolve target)")
	}
}

func TestRenderSessionSurfaceModeReset(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	// Enter surface mode.
	tex, view := createMockSurfaceView(t, device, 640, 480)
	defer device.DestroyTextureView(view)
	defer device.DestroyTexture(tex)

	s.SetSurfaceTarget(view, 640, 480)
	if s.RenderMode() != RenderModeSurface {
		t.Fatal("expected surface mode")
	}

	// Create textures in surface mode.
	err := s.EnsureTextures(640, 480)
	if err != nil {
		t.Fatalf("EnsureTextures in surface mode failed: %v", err)
	}
	if s.textures.resolveTex != nil {
		t.Error("expected nil resolveTex in surface mode")
	}

	// Reset to offscreen mode.
	s.SetSurfaceTarget(nil, 0, 0)
	if s.RenderMode() != RenderModeOffscreen {
		t.Fatal("expected offscreen mode after reset")
	}

	// Textures should have been invalidated by mode change.
	if s.textures.msaaTex != nil {
		t.Error("expected textures to be invalidated after mode change")
	}

	// Re-create textures in offscreen mode (resolve texture should be created).
	err = s.EnsureTextures(640, 480)
	if err != nil {
		t.Fatalf("EnsureTextures in offscreen mode failed: %v", err)
	}
	if s.textures.resolveTex == nil {
		t.Error("expected non-nil resolveTex in offscreen mode")
	}
	if s.textures.resolveView == nil {
		t.Error("expected non-nil resolveView in offscreen mode")
	}
}

func TestRenderSessionSurfaceModeTextures(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	tex, view := createMockSurfaceView(t, device, 1024, 768)
	defer device.DestroyTextureView(view)
	defer device.DestroyTexture(tex)

	s.SetSurfaceTarget(view, 1024, 768)

	err := s.EnsureTextures(1024, 768)
	if err != nil {
		t.Fatalf("EnsureTextures failed: %v", err)
	}

	// MSAA and stencil must exist.
	if s.textures.msaaTex == nil {
		t.Error("expected non-nil msaaTex")
	}
	if s.textures.msaaView == nil {
		t.Error("expected non-nil msaaView")
	}
	if s.textures.stencilTex == nil {
		t.Error("expected non-nil stencilTex")
	}
	if s.textures.stencilView == nil {
		t.Error("expected non-nil stencilView")
	}

	// Resolve texture must NOT exist (surface is the resolve target).
	if s.textures.resolveTex != nil {
		t.Error("expected nil resolveTex in surface mode")
	}
	if s.textures.resolveView != nil {
		t.Error("expected nil resolveView in surface mode")
	}

	// Dimensions should be set correctly.
	w, h := s.Size()
	if w != 1024 || h != 768 {
		t.Errorf("expected size (1024, 768), got (%d, %d)", w, h)
	}
}

func TestRenderSessionSurfaceModeResize(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	tex1, view1 := createMockSurfaceView(t, device, 800, 600)
	defer device.DestroyTextureView(view1)
	defer device.DestroyTexture(tex1)

	s.SetSurfaceTarget(view1, 800, 600)

	err := s.EnsureTextures(800, 600)
	if err != nil {
		t.Fatalf("initial EnsureTextures failed: %v", err)
	}

	w, h := s.Size()
	if w != 800 || h != 600 {
		t.Errorf("expected initial size (800, 600), got (%d, %d)", w, h)
	}

	// Simulate window resize: new surface view with different dimensions.
	tex2, view2 := createMockSurfaceView(t, device, 1920, 1080)
	defer device.DestroyTextureView(view2)
	defer device.DestroyTexture(tex2)

	s.SetSurfaceTarget(view2, 1920, 1080)

	// After SetSurfaceTarget with different size, textures should be
	// invalidated (destroyed). Verify they are nil before EnsureTextures.
	if s.textures.msaaTex != nil {
		t.Error("expected nil msaaTex after SetSurfaceTarget with new size")
	}

	// Recreate textures at the new size.
	err = s.EnsureTextures(1920, 1080)
	if err != nil {
		t.Fatalf("resize EnsureTextures failed: %v", err)
	}

	// MSAA texture should exist at the new dimensions.
	if s.textures.msaaTex == nil {
		t.Error("expected non-nil msaaTex after resize")
	}

	w, h = s.Size()
	if w != 1920 || h != 1080 {
		t.Errorf("expected (1920, 1080), got (%d, %d)", w, h)
	}

	// Resolve should still be nil in surface mode.
	if s.textures.resolveTex != nil {
		t.Error("expected nil resolveTex in surface mode after resize")
	}
}

func TestRenderSessionSurfaceModeStencilPaths(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	tex, view := createMockSurfaceView(t, device, 400, 300)
	defer device.DestroyTextureView(view)
	defer device.DestroyTexture(tex)

	s.SetSurfaceTarget(view, 400, 300)

	target := gg.GPURenderTarget{
		Width:  400,
		Height: 300,
		Data:   make([]uint8, 400*300*4),
		Stride: 400 * 4,
	}

	// Mixed SDF + stencil paths in surface mode.
	shapes := []SDFRenderShape{
		{Kind: 0, CenterX: 100, CenterY: 100, Param1: 40, Param2: 40, ColorA: 1},
	}
	paths := []StencilPathCommand{
		{
			Vertices:  []float32{200, 200, 250, 200, 250, 250},
			CoverQuad: [12]float32{199, 199, 251, 199, 251, 251, 199, 199, 251, 251, 199, 251},
			Color:     [4]float32{0, 1, 0, 1},
			FillRule:  gg.FillRuleNonZero,
		},
	}

	err := s.RenderFrame(target, shapes, nil, paths)
	if err != nil {
		t.Fatalf("surface mode mixed render failed: %v", err)
	}

	// Stencil texture must exist for stencil-then-cover.
	if s.textures.stencilTex == nil {
		t.Error("expected non-nil stencilTex for stencil paths")
	}
}

func TestRenderSessionDestroyClearsSurface(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)

	tex, view := createMockSurfaceView(t, device, 640, 480)
	defer device.DestroyTextureView(view)
	defer device.DestroyTexture(tex)

	s.SetSurfaceTarget(view, 640, 480)
	if s.RenderMode() != RenderModeSurface {
		t.Fatal("expected surface mode")
	}

	s.Destroy()

	// After Destroy, should be back in offscreen mode.
	if s.RenderMode() != RenderModeOffscreen {
		t.Error("expected offscreen mode after Destroy")
	}
}

func TestStencilRendererRecordPath(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	// Create pipelines first.
	err := sr.createPipelines()
	if err != nil {
		t.Fatalf("createPipelines failed: %v", err)
	}

	// Verify pipelines exist for RecordPath.
	if sr.nonZeroStencilPipeline == nil {
		t.Error("expected non-nil nonZeroStencilPipeline")
	}
	if sr.evenOddStencilPipeline == nil {
		t.Error("expected non-nil evenOddStencilPipeline")
	}
	if sr.nonZeroCoverPipeline == nil {
		t.Error("expected non-nil nonZeroCoverPipeline")
	}
}
