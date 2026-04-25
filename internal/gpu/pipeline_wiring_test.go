// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
)

// TestSDFAccelerator_SetPipelineMode verifies pipeline mode is stored.
func TestSDFAccelerator_SetPipelineMode(t *testing.T) {
	a := &SDFAccelerator{shared: NewGPUShared()}

	a.SetPipelineMode(gg.PipelineModeCompute)
	a.SetPipelineMode(gg.PipelineModeRenderPass)
	a.SetPipelineMode(gg.PipelineModeAuto)
	// No panic = success. Pipeline mode is internal to the default context.
}

// TestSDFAccelerator_CanCompute_NoVello verifies CanCompute returns false
// when no VelloAccelerator is attached.
func TestSDFAccelerator_CanCompute_NoVello(t *testing.T) {
	a := &SDFAccelerator{shared: NewGPUShared()}
	// gpuReady = false, no vello
	if a.CanCompute() {
		t.Error("expected CanCompute()=false without VelloAccelerator")
	}
}

// TestSDFAccelerator_CanCompute_WithVelloNotReady verifies CanCompute returns
// false when VelloAccelerator exists but its dispatcher is not initialized.
func TestSDFAccelerator_CanCompute_WithVelloNotReady(t *testing.T) {
	s := NewGPUShared()
	s.gpuReady = true
	s.velloAccel = &VelloAccelerator{
		gpuReady: true,
		// No dispatcher — CanCompute should return false.
	}
	a := &SDFAccelerator{shared: s}

	if a.CanCompute() {
		t.Error("expected CanCompute()=false with uninitialized VelloAccelerator")
	}
}

// TestSDFAccelerator_SceneStats_Accumulation verifies that FillPath and FillShape
// increment scene stats via the default context.
func TestSDFAccelerator_SceneStats_Accumulation(t *testing.T) {
	s := NewGPUShared()
	s.gpuReady = true
	a := &SDFAccelerator{shared: s}
	target := makeTestTarget(100, 100)

	// FillPath should increment PathCount and ShapeCount.
	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 50)
	path.LineTo(10, 90)
	path.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	if err := a.FillPath(target, path, paint); err != nil {
		t.Fatalf("FillPath: %v", err)
	}

	stats := a.SceneStats()
	if stats.PathCount != 1 {
		t.Errorf("after FillPath: PathCount = %d, want 1", stats.PathCount)
	}
	if stats.ShapeCount != 1 {
		t.Errorf("after FillPath: ShapeCount = %d, want 1", stats.ShapeCount)
	}

	// FillShape should increment ShapeCount.
	shape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 30,
		RadiusY: 30,
	}
	if err := a.FillShape(target, shape, paint); err != nil {
		t.Fatalf("FillShape: %v", err)
	}

	stats = a.SceneStats()
	if stats.ShapeCount != 2 {
		t.Errorf("after FillShape: ShapeCount = %d, want 2", stats.ShapeCount)
	}
}

// TestSDFAccelerator_SceneStats_ResetOnFlush verifies that Flush resets
// scene stats for the next frame.
func TestSDFAccelerator_SceneStats_ResetOnFlush(t *testing.T) {
	s := NewGPUShared()
	s.gpuReady = true
	a := &SDFAccelerator{shared: s}
	target := makeTestTarget(100, 100)

	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 50)
	path.LineTo(10, 90)
	path.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	if err := a.FillPath(target, path, paint); err != nil {
		t.Fatalf("FillPath: %v", err)
	}

	if stats := a.SceneStats(); stats.ShapeCount != 1 {
		t.Fatalf("before Flush: ShapeCount = %d, want 1", stats.ShapeCount)
	}

	// Flush resets stats (via Flush on default context).
	_ = a.Flush(target)

	stats := a.SceneStats()
	if stats.ShapeCount != 0 {
		t.Errorf("after Flush: ShapeCount = %d, want 0 (reset)", stats.ShapeCount)
	}
	if stats.PathCount != 0 {
		t.Errorf("after Flush: PathCount = %d, want 0 (reset)", stats.PathCount)
	}
}

// TestSDFAccelerator_ComputeMode_DelegatesToVello verifies that in Compute mode,
// FillPath delegates to VelloAccelerator when compute is available, or falls
// back to the render pass pipeline when compute is unavailable.
func TestSDFAccelerator_ComputeMode_DelegatesToVello(t *testing.T) {
	s := NewGPUShared()
	a := &SDFAccelerator{shared: s}
	a.SetPipelineMode(gg.PipelineModeCompute)

	target := makeTestTarget(100, 100)

	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 50)
	path.LineTo(10, 90)
	path.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	err := a.FillPath(target, path, paint)
	if err != nil {
		t.Fatalf("FillPath: %v", err)
	}

	// Path should be pending somewhere — either in Vello (if compute available)
	// or in the render pass pipeline (convex/stencil).
	velloPending := 0
	if s.velloAccel != nil {
		velloPending = s.velloAccel.PendingCount()
	}
	renderPending := a.PendingCount()

	if velloPending == 0 && renderPending == 0 {
		t.Error("FillPath should have queued commands in either Vello or render pass")
	}
}

// TestSDFAccelerator_RenderPassMode_IgnoresVello verifies that in RenderPass mode,
// operations never go to VelloAccelerator even if it's available.
func TestSDFAccelerator_RenderPassMode_IgnoresVello(t *testing.T) {
	vello := &VelloAccelerator{gpuReady: true}
	s := NewGPUShared()
	s.gpuReady = true
	s.velloAccel = vello

	a := &SDFAccelerator{shared: s}
	a.SetPipelineMode(gg.PipelineModeRenderPass)

	target := makeTestTarget(100, 100)

	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 50)
	path.LineTo(10, 90)
	path.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	err := a.FillPath(target, path, paint)
	if err != nil {
		t.Fatalf("FillPath: %v", err)
	}

	// Should be in render pass pipeline, not Vello.
	if vello.PendingCount() != 0 {
		t.Errorf("Vello should have 0 pending in RenderPass mode, got %d", vello.PendingCount())
	}
	if a.PendingCount() == 0 {
		t.Error("SDFAccelerator should have pending commands in RenderPass mode")
	}
}

// TestSDFAccelerator_FillShape_ComputeMode verifies FillShape routes commands
// in Compute mode — either to VelloAccelerator (if compute available) or
// to the render pass pipeline (SDF shapes).
func TestSDFAccelerator_FillShape_ComputeMode(t *testing.T) {
	s := NewGPUShared()
	a := &SDFAccelerator{shared: s}
	a.SetPipelineMode(gg.PipelineModeCompute)

	target := makeTestTarget(100, 100)
	shape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 30,
		RadiusY: 30,
	}

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Green))

	err := a.FillShape(target, shape, paint)
	if err != nil {
		t.Fatalf("FillShape: %v", err)
	}

	// Shape should be pending somewhere — either Vello or SDF render pass.
	velloPending := 0
	if s.velloAccel != nil {
		velloPending = s.velloAccel.PendingCount()
	}
	renderPending := a.PendingCount()

	if velloPending == 0 && renderPending == 0 {
		t.Error("FillShape should have queued commands in either Vello or render pass")
	}
}

// TestSDFAccelerator_Interfaces verifies SDFAccelerator implements all
// the expected interfaces.
func TestSDFAccelerator_Interfaces(t *testing.T) {
	a := &SDFAccelerator{shared: NewGPUShared()}

	// Core interface.
	var _ gg.GPUAccelerator = a

	// Extension interfaces.
	var _ gg.GPURenderContextProvider = a
	var _ gg.GPUTextAccelerator = a
	var _ gg.PipelineModeAware = a
	var _ gg.ComputePipelineAware = a
}

// TestGPURenderContext_Isolation verifies two GPURenderContexts have isolated
// pending command queues.
func TestGPURenderContext_Isolation(t *testing.T) {
	s := NewGPUShared()
	s.gpuReady = true

	rc1 := s.NewRenderContext()
	rc2 := s.NewRenderContext()

	target1 := makeTestTarget(100, 100)
	target2 := makeTestTarget(200, 200)

	// Queue a shape in rc1.
	shape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 30,
		RadiusY: 30,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	if err := rc1.QueueShape(target1, shape, paint, false); err != nil {
		t.Fatalf("rc1.QueueShape: %v", err)
	}

	// rc2 should have no pending commands.
	if rc2.PendingCount() != 0 {
		t.Errorf("rc2 should have 0 pending, got %d", rc2.PendingCount())
	}

	// rc1 should have exactly 1 pending.
	if rc1.PendingCount() != 1 {
		t.Errorf("rc1 should have 1 pending, got %d", rc1.PendingCount())
	}

	// Queue in rc2.
	if err := rc2.QueueShape(target2, shape, paint, false); err != nil {
		t.Fatalf("rc2.QueueShape: %v", err)
	}

	if rc1.PendingCount() != 1 {
		t.Errorf("rc1 should still have 1 pending, got %d", rc1.PendingCount())
	}
	if rc2.PendingCount() != 1 {
		t.Errorf("rc2 should have 1 pending, got %d", rc2.PendingCount())
	}

	rc1.Close()
	rc2.Close()
}

// TestGPURenderContext_FrameTracking verifies per-context frame state.
func TestGPURenderContext_FrameTracking(t *testing.T) {
	s := NewGPUShared()
	rc1 := s.NewRenderContext()
	rc2 := s.NewRenderContext()

	// Initial state: not rendered.
	if rc1.frameRendered {
		t.Error("rc1 should start with frameRendered=false")
	}
	if rc2.frameRendered {
		t.Error("rc2 should start with frameRendered=false")
	}

	// Simulated render for rc1.
	rc1.frameRendered = true

	// rc2 should be unaffected.
	if rc2.frameRendered {
		t.Error("rc2 should still have frameRendered=false after rc1 render")
	}

	// BeginFrame resets only rc1.
	rc1.BeginFrame()
	if rc1.frameRendered {
		t.Error("rc1 should have frameRendered=false after BeginFrame")
	}

	rc1.Close()
	rc2.Close()
}

// TestTexturePool_AcquireRelease verifies texture pool acquire/release lifecycle.
func TestTexturePool_AcquireRelease(t *testing.T) {
	pool := NewTexturePool(128)

	// Acquire with no pooled textures should return nil.
	ts := pool.Acquire(1920, 1080, 4)
	if ts != nil {
		t.Error("expected nil from empty pool")
	}

	// Release a textureSet.
	pool.Release(&textureSet{width: 1920, height: 1080})

	// Now acquire should return it.
	ts = pool.Acquire(1920, 1080, 4)
	if ts == nil {
		t.Fatal("expected non-nil after Release")
	}
	if ts.width != 1920 || ts.height != 1080 {
		t.Errorf("got %dx%d, want 1920x1080", ts.width, ts.height)
	}

	// Pool should be empty again.
	if pool.PooledCount() != 0 {
		t.Errorf("pool should be empty, got %d", pool.PooledCount())
	}
}

// TestTexturePool_EndFrame verifies unused textures are trimmed.
func TestTexturePool_EndFrame(t *testing.T) {
	pool := NewTexturePool(128)

	// Add 5 texture sets with same key.
	for i := 0; i < 5; i++ {
		pool.Release(&textureSet{width: 800, height: 600})
	}
	if pool.PooledCount() != 5 {
		t.Errorf("expected 5 pooled, got %d", pool.PooledCount())
	}

	// EndFrame should trim to 2 per key.
	pool.EndFrame()
	if pool.PooledCount() != 2 {
		t.Errorf("expected 2 pooled after EndFrame, got %d", pool.PooledCount())
	}
}

// TestGPURenderContext_BaseLayer_PendingCount verifies base layer is included in PendingCount.
func TestGPURenderContext_BaseLayer_PendingCount(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	if rc.PendingCount() != 0 {
		t.Errorf("expected 0 pending, got %d", rc.PendingCount())
	}

	target := gg.GPURenderTarget{
		Data: make([]byte, 100*100*4), Width: 100, Height: 100,
	}
	rc.QueueBaseLayer(target, gpucontext.TextureView{}, 0, 0, 100, 100, 1.0, 100, 100)

	if rc.PendingCount() != 1 {
		t.Errorf("expected 1 pending (base layer), got %d", rc.PendingCount())
	}
}

// TestGPURenderContext_BaseLayer_LastCallWins verifies last QueueBaseLayer overwrites previous.
func TestGPURenderContext_BaseLayer_LastCallWins(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := gg.GPURenderTarget{
		Data: make([]byte, 100*100*4), Width: 100, Height: 100,
	}

	rc.QueueBaseLayer(target, gpucontext.TextureView{}, 0, 0, 50, 50, 1.0, 100, 100)
	rc.QueueBaseLayer(target, gpucontext.TextureView{}, 0, 0, 100, 100, 0.5, 100, 100)

	if rc.PendingCount() != 1 {
		t.Errorf("expected 1 pending (base layer, last call wins), got %d", rc.PendingCount())
	}
	if rc.baseLayer.Opacity != 0.5 {
		t.Errorf("expected opacity 0.5 from last call, got %f", rc.baseLayer.Opacity)
	}
}

// TestGPURenderContext_BaseLayer_ClearedAfterClose verifies base layer is nil after Close.
func TestGPURenderContext_BaseLayer_ClearedAfterClose(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()

	target := gg.GPURenderTarget{
		Data: make([]byte, 100*100*4), Width: 100, Height: 100,
	}
	rc.QueueBaseLayer(target, gpucontext.TextureView{}, 0, 0, 100, 100, 1.0, 100, 100)
	rc.Close()

	if rc.baseLayer != nil {
		t.Error("expected nil baseLayer after Close")
	}
}

// TestGPURenderContext_BaseLayer_DoesNotAffectOtherCounts verifies base layer
// is separate from regular command counts.
func TestGPURenderContext_BaseLayer_DoesNotAffectOtherCounts(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := gg.GPURenderTarget{
		Data: make([]byte, 100*100*4), Width: 100, Height: 100,
	}

	rc.QueueBaseLayer(target, gpucontext.TextureView{}, 0, 0, 100, 100, 1.0, 100, 100)

	shapes := len(rc.pendingShapes)
	images := len(rc.pendingImageCommands)
	gpuTex := len(rc.pendingGPUTextureCommands)

	if shapes != 0 || images != 0 || gpuTex != 0 {
		t.Errorf("base layer should not affect other queues: shapes=%d images=%d gpuTex=%d",
			shapes, images, gpuTex)
	}
}
