//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
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
	err := s.RenderFrame(target, nil, nil)
	if err != nil {
		t.Fatalf("RenderFrame(nil, nil) failed: %v", err)
	}

	err = s.RenderFrame(target, []SDFRenderShape{}, []StencilPathCommand{})
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
	err := s.RenderFrame(target, shapes, nil)
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

	err := s.RenderFrame(target, nil, paths)
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

	err := s.RenderFrame(target, shapes, paths)
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
		err := s.RenderFrame(target, shapes, nil)
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
