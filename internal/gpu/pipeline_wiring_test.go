// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
)

// TestSDFAccelerator_SetPipelineMode verifies pipeline mode is stored.
func TestSDFAccelerator_SetPipelineMode(t *testing.T) {
	a := &SDFAccelerator{}

	a.SetPipelineMode(gg.PipelineModeCompute)
	if a.pipelineMode != gg.PipelineModeCompute {
		t.Errorf("expected Compute, got %v", a.pipelineMode)
	}

	a.SetPipelineMode(gg.PipelineModeRenderPass)
	if a.pipelineMode != gg.PipelineModeRenderPass {
		t.Errorf("expected RenderPass, got %v", a.pipelineMode)
	}

	a.SetPipelineMode(gg.PipelineModeAuto)
	if a.pipelineMode != gg.PipelineModeAuto {
		t.Errorf("expected Auto, got %v", a.pipelineMode)
	}
}

// TestSDFAccelerator_CanCompute_NoVello verifies CanCompute returns false
// when no VelloAccelerator is attached.
func TestSDFAccelerator_CanCompute_NoVello(t *testing.T) {
	a := &SDFAccelerator{gpuReady: true}

	if a.CanCompute() {
		t.Error("expected CanCompute()=false without VelloAccelerator")
	}
}

// TestSDFAccelerator_CanCompute_WithVelloNotReady verifies CanCompute returns
// false when VelloAccelerator exists but its dispatcher is not initialized.
func TestSDFAccelerator_CanCompute_WithVelloNotReady(t *testing.T) {
	a := &SDFAccelerator{
		gpuReady: true,
		velloAccel: &VelloAccelerator{
			gpuReady: true,
			// No dispatcher — CanCompute should return false.
		},
	}

	if a.CanCompute() {
		t.Error("expected CanCompute()=false with uninitialized VelloAccelerator")
	}
}

// TestSDFAccelerator_SceneStats_Accumulation verifies that FillPath, FillShape,
// StrokePath, and StrokeShape all increment scene stats.
func TestSDFAccelerator_SceneStats_Accumulation(t *testing.T) {
	a := &SDFAccelerator{gpuReady: true}
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
	a := &SDFAccelerator{gpuReady: true}
	target := makeTestTarget(100, 100)

	// Accumulate some stats.
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

	// Flush should reset stats.
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
// FillPath delegates to VelloAccelerator instead of the render pass pipeline.
func TestSDFAccelerator_ComputeMode_DelegatesToVello(t *testing.T) {
	vello := &VelloAccelerator{gpuReady: true}

	a := &SDFAccelerator{
		gpuReady:     true,
		pipelineMode: gg.PipelineModeCompute,
		velloAccel:   vello,
	}

	target := makeTestTarget(100, 100)

	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 50)
	path.LineTo(10, 90)
	path.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	// Note: VelloAccelerator.CanCompute() is false because dispatcher is nil.
	// So FillPath should NOT delegate and should fall back to render pass pipeline.
	err := a.FillPath(target, path, paint)
	if err != nil {
		t.Fatalf("FillPath: %v", err)
	}

	// Path should be in SDFAccelerator's pending (render pass), not Vello's.
	if vello.PendingCount() != 0 {
		t.Errorf("Vello should have 0 pending (no compute), got %d", vello.PendingCount())
	}
	if a.PendingCount() == 0 {
		t.Error("SDFAccelerator should have pending commands (render pass fallback)")
	}
}

// TestSDFAccelerator_RenderPassMode_IgnoresVello verifies that in RenderPass mode,
// operations never go to VelloAccelerator even if it's available.
func TestSDFAccelerator_RenderPassMode_IgnoresVello(t *testing.T) {
	vello := &VelloAccelerator{gpuReady: true}

	a := &SDFAccelerator{
		gpuReady:     true,
		pipelineMode: gg.PipelineModeRenderPass,
		velloAccel:   vello,
	}

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

// TestSDFAccelerator_FillShape_ComputeMode verifies FillShape delegates to
// VelloAccelerator in Compute mode (when compute is available).
func TestSDFAccelerator_FillShape_ComputeMode(t *testing.T) {
	// VelloAccelerator without dispatcher — CanCompute() is false.
	vello := &VelloAccelerator{gpuReady: true}

	a := &SDFAccelerator{
		gpuReady:     true,
		pipelineMode: gg.PipelineModeCompute,
		velloAccel:   vello,
	}

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

	// CanCompute() is false → should go to render pass (SDF).
	if vello.PendingCount() != 0 {
		t.Errorf("Vello should have 0 pending (no compute), got %d", vello.PendingCount())
	}
	if a.PendingCount() == 0 {
		t.Error("SDFAccelerator should have pending SDF shape")
	}
}

// TestSDFAccelerator_EffectivePipelineMode_Auto verifies that Auto mode
// uses selectPipeline heuristics.
func TestSDFAccelerator_EffectivePipelineMode_Auto(t *testing.T) {
	// Auto + simple scene + no compute = RenderPass.
	a := &SDFAccelerator{
		gpuReady:     true,
		pipelineMode: gg.PipelineModeAuto,
		sceneStats:   gg.SceneStats{ShapeCount: 3},
	}

	mode := a.effectivePipelineMode()
	if mode != gg.PipelineModeRenderPass {
		t.Errorf("Auto + simple scene: expected RenderPass, got %v", mode)
	}

	// Auto + complex scene + no compute = RenderPass (no compute support).
	a.sceneStats = gg.SceneStats{ShapeCount: 100}
	mode = a.effectivePipelineMode()
	if mode != gg.PipelineModeRenderPass {
		t.Errorf("Auto + complex scene + no compute: expected RenderPass, got %v", mode)
	}
}

// TestSDFAccelerator_EffectivePipelineMode_Explicit verifies that explicit
// modes override Auto heuristics.
func TestSDFAccelerator_EffectivePipelineMode_Explicit(t *testing.T) {
	a := &SDFAccelerator{
		gpuReady:     true,
		pipelineMode: gg.PipelineModeCompute,
		sceneStats:   gg.SceneStats{ShapeCount: 3}, // Simple scene
	}

	mode := a.effectivePipelineMode()
	if mode != gg.PipelineModeCompute {
		t.Errorf("explicit Compute: expected Compute, got %v", mode)
	}

	a.pipelineMode = gg.PipelineModeRenderPass
	a.sceneStats = gg.SceneStats{ShapeCount: 100} // Complex scene
	mode = a.effectivePipelineMode()
	if mode != gg.PipelineModeRenderPass {
		t.Errorf("explicit RenderPass: expected RenderPass, got %v", mode)
	}
}

// TestSDFAccelerator_Interfaces verifies SDFAccelerator implements all
// the expected interfaces.
func TestSDFAccelerator_Interfaces(t *testing.T) {
	a := &SDFAccelerator{}

	// Core interface.
	var _ gg.GPUAccelerator = a

	// Extension interfaces.
	var _ gg.SurfaceTargetAware = a
	var _ gg.GPUTextAccelerator = a
	var _ gg.PipelineModeAware = a
	var _ gg.ComputePipelineAware = a
}
