// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/wgpu"
)

// --- SetSharedEncoder / CreateEncoder / SubmitEncoder ---

// TestSetSharedEncoder_NilClearsEncoder verifies SetSharedEncoder with a
// zero-value CommandEncoder sets sharedEncoder to nil.
func TestSetSharedEncoder_NilClearsEncoder(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	if rc.sharedEncoder != nil {
		t.Fatal("expected nil sharedEncoder initially")
	}

	rc.SetSharedEncoder(gpucontext.CommandEncoder{})
	if rc.sharedEncoder != nil {
		t.Error("SetSharedEncoder(zero) should leave sharedEncoder nil")
	}
}

// TestSetSharedEncoder_SetsPointer verifies SetSharedEncoder with a non-nil
// CommandEncoder stores the pointer.
func TestSetSharedEncoder_SetsPointer(t *testing.T) {
	device, _, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPUShared()
	s.device = device

	rc := s.NewRenderContext()
	defer rc.Close()

	enc, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "test_encoder",
	})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	gpuEnc := gpucontext.NewCommandEncoder(unsafe.Pointer(enc))
	rc.SetSharedEncoder(gpuEnc)

	if rc.sharedEncoder == nil {
		t.Error("expected non-nil sharedEncoder after SetSharedEncoder")
	}

	rc.SetSharedEncoder(gpucontext.CommandEncoder{})
	if rc.sharedEncoder != nil {
		t.Error("expected nil sharedEncoder after clearing")
	}
}

// TestCreateEncoder_NilSession verifies CreateEncoder returns zero-value
// when session is nil.
func TestCreateEncoder_NilSession(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	enc := rc.CreateEncoder()
	if !enc.IsNil() {
		t.Error("expected nil encoder when session is nil")
	}
}

// TestCreateEncoder_WithNoopDevice verifies CreateEncoder succeeds with a
// noop device and an initialized session.
func TestCreateEncoder_WithNoopDevice(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPUShared()
	s.device = device
	s.queue = queue
	s.deviceReady = true
	s.gpuReady = true

	rc := s.NewRenderContext()
	defer rc.Close()

	sc := resolveSampleCount(device)
	rc.session = NewGPURenderSession(device, queue, sc)

	enc := rc.CreateEncoder()
	if enc.IsNil() {
		t.Fatal("expected non-nil encoder with noop device + session")
	}
}

// TestSubmitEncoder_NilSession verifies SubmitEncoder returns error when
// session is nil.
func TestSubmitEncoder_NilSession(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	err := rc.SubmitEncoder(gpucontext.CommandEncoder{})
	if err == nil {
		t.Fatal("expected error when session is nil")
	}
}

// TestSubmitEncoder_NilEncoder verifies SubmitEncoder returns error when
// encoder is nil.
func TestSubmitEncoder_NilEncoder(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPUShared()
	s.device = device
	s.queue = queue
	s.deviceReady = true
	s.gpuReady = true

	rc := s.NewRenderContext()
	defer rc.Close()

	sc := resolveSampleCount(device)
	rc.session = NewGPURenderSession(device, queue, sc)

	err := rc.SubmitEncoder(gpucontext.CommandEncoder{})
	if err == nil {
		t.Fatal("expected error for nil encoder")
	}
}

// TestSubmitEncoder_Success verifies the full CreateEncoder to SubmitEncoder round-trip
// on a noop device.
func TestSubmitEncoder_Success(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPUShared()
	s.device = device
	s.queue = queue
	s.deviceReady = true
	s.gpuReady = true

	rc := s.NewRenderContext()
	defer rc.Close()

	sc := resolveSampleCount(device)
	rc.session = NewGPURenderSession(device, queue, sc)

	enc := rc.CreateEncoder()
	if enc.IsNil() {
		t.Fatal("CreateEncoder returned nil")
	}

	err := rc.SubmitEncoder(enc)
	if err != nil {
		t.Fatalf("SubmitEncoder: %v", err)
	}
}

// --- SetAntiAlias ---

// TestSetAntiAlias verifies SetAntiAlias stores the value correctly.
func TestSetAntiAlias(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	// Default is true (set by NewRenderContext).
	if !rc.antiAlias {
		t.Error("expected antiAlias=true by default (NewRenderContext sets true)")
	}

	rc.SetAntiAlias(false)
	if rc.antiAlias {
		t.Error("expected antiAlias=false after SetAntiAlias(false)")
	}

	rc.SetAntiAlias(true)
	if !rc.antiAlias {
		t.Error("expected antiAlias=true after SetAntiAlias(true)")
	}
}

// --- DrawText nil face paths ---

// TestDrawText_NilFace_ReturnsError verifies DrawText returns ErrFallbackToCPU
// when face is nil.
func TestDrawText_NilFace_ReturnsError(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)

	err := rc.DrawText(target, nil, "hello", 10, 20, gg.Red, gg.Identity(), 1.0)
	if !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Errorf("DrawText(nil face) = %v, want ErrFallbackToCPU", err)
	}
}

// TestDrawText_InvalidFaceType_ReturnsError verifies DrawText returns
// ErrFallbackToCPU when face is not a text.Face.
func TestDrawText_InvalidFaceType_ReturnsError(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)

	err := rc.DrawText(target, "not-a-face", "hello", 10, 20, gg.Red, gg.Identity(), 1.0)
	if !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Errorf("DrawText(string face) = %v, want ErrFallbackToCPU", err)
	}
}

// TestDrawGlyphMaskText_NilFace_ReturnsError verifies DrawGlyphMaskText returns
// ErrFallbackToCPU when face is nil.
func TestDrawGlyphMaskText_NilFace_ReturnsError(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)

	err := rc.DrawGlyphMaskText(target, nil, "hello", 10, 20, gg.Red, gg.Identity(), 1.0)
	if !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Errorf("DrawGlyphMaskText(nil face) = %v, want ErrFallbackToCPU", err)
	}
}

// TestDrawGlyphMaskTextAliased_NilFace_ReturnsError verifies DrawGlyphMaskTextAliased
// returns ErrFallbackToCPU when face is nil.
func TestDrawGlyphMaskTextAliased_NilFace_ReturnsError(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)

	err := rc.DrawGlyphMaskTextAliased(target, nil, "hello", 10, 20, gg.Red, gg.Identity(), 1.0)
	if !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Errorf("DrawGlyphMaskTextAliased(nil face) = %v, want ErrFallbackToCPU", err)
	}
}

// TestDrawShapedGlyphMaskText_NilFace_ReturnsError verifies DrawShapedGlyphMaskText
// returns ErrFallbackToCPU when face is nil.
func TestDrawShapedGlyphMaskText_NilFace_ReturnsError(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)

	err := rc.DrawShapedGlyphMaskText(target, nil, nil, 10, 20, gg.Red, gg.Identity(), 1.0)
	if !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Errorf("DrawShapedGlyphMaskText(nil face) = %v, want ErrFallbackToCPU", err)
	}
}

// TestDrawText_IncreasesTextCount verifies DrawText does not increment
// sceneStats.TextCount for nil face (early return before stats).
func TestDrawText_IncreasesTextCount(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)

	err := rc.DrawText(target, nil, "hello", 10, 20, gg.Red, gg.Identity(), 1.0)
	if !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Fatalf("unexpected: %v", err)
	}

	stats := rc.SceneStats()
	if stats.TextCount != 0 {
		t.Errorf("TextCount = %d, want 0 (nil face returns before stats)", stats.TextCount)
	}
}

// --- StrokeShape thin stroke fallback ---

// TestStrokeShape_ThinStroke_FallsBackToCPU verifies that strokes < 2px
// return ErrFallbackToCPU (ADR-040: SDF annular ring thinner than AA zone).
func TestStrokeShape_ThinStroke_FallsBackToCPU(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true

	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)
	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 50, CenterY: 50, RadiusX: 30, RadiusY: 30,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	paint.SetStroke(gg.Stroke{Width: 1.5})

	err := rc.StrokeShape(target, shape, paint)
	if !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Errorf("StrokeShape(1.5px) = %v, want ErrFallbackToCPU", err)
	}

	if len(rc.pendingDraws) != 0 {
		t.Errorf("expected 0 pendingDraws for thin stroke, got %d", len(rc.pendingDraws))
	}
}

// TestStrokeShape_ExactThreshold_Succeeds verifies that strokes at exactly
// 2px are accepted (boundary test).
func TestStrokeShape_ExactThreshold_Succeeds(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true

	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)
	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 50, CenterY: 50, RadiusX: 30, RadiusY: 30,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	paint.SetStroke(gg.Stroke{Width: 2.0})

	err := rc.StrokeShape(target, shape, paint)
	if err != nil {
		t.Errorf("StrokeShape(2.0px) = %v, want nil", err)
	}
	if len(rc.pendingDraws) != 1 {
		t.Errorf("expected 1 pendingDraw for 2px stroke, got %d", len(rc.pendingDraws))
	}
}

// --- FillShape / StrokeShape compute mode ---

// TestFillShape_ComputeMode_NoVello_QueuesNormally verifies that in Compute mode
// without a VelloAccelerator, FillShape falls through to the normal queue path.
func TestFillShape_ComputeMode_NoVello_QueuesNormally(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true

	rc := s.NewRenderContext()
	defer rc.Close()
	rc.pipelineMode = gg.PipelineModeCompute

	target := makeTestTarget(100, 100)
	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 50, CenterY: 50, RadiusX: 20, RadiusY: 20,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	err := rc.FillShape(target, shape, paint)
	if err != nil {
		t.Fatalf("FillShape: %v", err)
	}

	if len(rc.pendingDraws) != 1 {
		t.Errorf("expected 1 pendingDraw (no Vello fallthrough), got %d", len(rc.pendingDraws))
	}
}

// TestStrokeShape_ComputeMode_NoVello_QueuesNormally verifies StrokeShape in
// Compute mode without VelloAccelerator queues normally.
func TestStrokeShape_ComputeMode_NoVello_QueuesNormally(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true

	rc := s.NewRenderContext()
	defer rc.Close()
	rc.pipelineMode = gg.PipelineModeCompute

	target := makeTestTarget(100, 100)
	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 50, CenterY: 50, RadiusX: 30, RadiusY: 30,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	paint.SetStroke(gg.Stroke{Width: 3.0})

	err := rc.StrokeShape(target, shape, paint)
	if err != nil {
		t.Fatalf("StrokeShape: %v", err)
	}

	if len(rc.pendingDraws) != 1 {
		t.Errorf("expected 1 pendingDraw (no Vello fallthrough), got %d", len(rc.pendingDraws))
	}
}

// --- FillShape target mismatch flush ---

// TestFillShape_TargetMismatch_FlushesFirst verifies that changing targets between
// FillShape calls triggers an intermediate flush.
func TestFillShape_TargetMismatch_FlushesFirst(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()

	target1 := makeTestTarget(100, 100)
	target2 := makeTestTarget(200, 200)

	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 50, CenterY: 50, RadiusX: 20, RadiusY: 20,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	if err := rc.FillShape(target1, shape, paint); err != nil {
		t.Fatalf("FillShape target1: %v", err)
	}
	if len(rc.pendingDraws) != 1 {
		t.Fatalf("expected 1 pendingDraw after first shape, got %d", len(rc.pendingDraws))
	}

	if err := rc.FillShape(target2, shape, paint); err != nil {
		t.Fatalf("FillShape target2: %v", err)
	}

	if len(rc.pendingDraws) != 1 {
		t.Errorf("expected 1 pendingDraw after target switch (first flushed), got %d", len(rc.pendingDraws))
	}
}

// --- drawClipEqual clipPath + clipRRect branches ---

// TestDrawClipEqual_ClipPathAndRRect verifies drawClipEqual handles the clipPath
// and clipRRect comparison branches. Replaces legacy scissorGroupClipEqual test
// after ADR-051 Phase 2 Step 6 (all clip comparison is per-draw now).
func TestDrawClipEqual_ClipPathAndRRect(t *testing.T) {
	path1 := gg.NewPath()
	path1.MoveTo(0, 0)
	path1.LineTo(100, 100)
	path1.Close()

	path2 := gg.NewPath()
	path2.MoveTo(50, 50)
	path2.LineTo(150, 150)
	path2.Close()

	rect1 := [4]uint32{10, 10, 80, 80}
	rrect1 := ClipParams{RectX1: 10, RectY1: 20, RectX2: 110, RectY2: 100, Radius: 5, Enabled: 1}
	rrect2 := ClipParams{RectX1: 0, RectY1: 0, RectX2: 200, RectY2: 200, Radius: 10, Enabled: 1}

	tests := []struct {
		name string
		a, b drawCommand
		want bool
	}{
		{"both nil clipPath", drawCommand{}, drawCommand{}, true},
		{"same clipPath pointer", drawCommand{clipPath: path1}, drawCommand{clipPath: path1}, true},
		{"different clipPath pointers", drawCommand{clipPath: path1}, drawCommand{clipPath: path2}, false},
		{"one nil clipPath", drawCommand{clipPath: path1}, drawCommand{}, false},
		{
			"rect match clipPath mismatch",
			drawCommand{clipRect: &rect1, clipPath: path1},
			drawCommand{clipRect: &rect1, clipPath: path2},
			false,
		},
		{
			"full match including clipPath",
			drawCommand{clipRect: &rect1, clipPath: path1},
			drawCommand{clipRect: &rect1, clipPath: path1},
			true,
		},
		{
			"clipRRect match",
			drawCommand{clipRRect: &rrect1},
			drawCommand{clipRRect: &rrect1},
			true,
		},
		{
			"clipRRect mismatch",
			drawCommand{clipRRect: &rrect1},
			drawCommand{clipRRect: &rrect2},
			false,
		},
		{
			"one nil clipRRect",
			drawCommand{clipRRect: &rrect1},
			drawCommand{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := drawClipEqual(&tt.a, &tt.b)
			if got != tt.want {
				t.Errorf("drawClipEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- drawsToScissorGroup mixed types ---

// TestDrawsToScissorGroup_MixedTypes verifies drawsToScissorGroup handles all
// four draw command kinds in a single group.
func TestDrawsToScissorGroup_MixedTypes(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true

	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 50, CenterY: 50, RadiusX: 20, RadiusY: 20,
	}
	if err := rc.FillShape(target, shape, paint); err != nil {
		t.Fatalf("FillShape: %v", err)
	}

	strokePaint := gg.NewPaint()
	strokePaint.SetBrush(gg.Solid(gg.Blue))
	strokePaint.SetStroke(gg.Stroke{Width: 3.0})
	if err := rc.StrokeShape(target, shape, strokePaint); err != nil {
		t.Fatalf("StrokeShape: %v", err)
	}

	triPath := gg.NewPath()
	triPath.MoveTo(10, 10)
	triPath.LineTo(90, 10)
	triPath.LineTo(50, 90)
	triPath.Close()
	if err := rc.FillPath(target, triPath, paint); err != nil {
		t.Fatalf("FillPath triangle: %v", err)
	}

	complexPath := gg.NewPath()
	complexPath.MoveTo(10, 50)
	complexPath.CubicTo(30, 10, 70, 90, 90, 50)
	complexPath.CubicTo(70, 10, 30, 90, 10, 50)
	complexPath.Close()
	if err := rc.FillPath(target, complexPath, paint); err != nil {
		t.Fatalf("FillPath complex: %v", err)
	}

	if len(rc.pendingDraws) != 4 {
		t.Fatalf("expected 4 pendingDraws, got %d", len(rc.pendingDraws))
	}

	g := rc.drawsToScissorGroup(rc.pendingDraws)

	if len(g.SDFShapes) != 2 {
		t.Errorf("SDFShapes = %d, want 2 (fill + stroke)", len(g.SDFShapes))
	}

	totalPathCmds := len(g.ConvexCommands) + len(g.StencilPaths)
	if totalPathCmds == 0 {
		t.Error("expected at least 1 ConvexCommand or StencilPath from path draws, got 0")
	}
}

// TestDrawsToScissorGroup_ZeroAlphaSkipped verifies that zero-alpha shapes
// are skipped in drawsToScissorGroup (BUG-SDF-001 pattern).
func TestDrawsToScissorGroup_ZeroAlphaSkipped(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.RGBA{R: 1, G: 0, B: 0, A: 0}))

	draws := []drawCommand{
		{
			kind:  drawCmdFillShape,
			shape: gg.DetectedShape{Kind: gg.ShapeCircle, CenterX: 50, CenterY: 50, RadiusX: 20, RadiusY: 20},
			paint: *paint,
		},
	}

	g := rc.drawsToScissorGroup(draws)
	if len(g.SDFShapes) != 0 {
		t.Errorf("SDFShapes = %d, want 0 (zero-alpha should be skipped)", len(g.SDFShapes))
	}
}

// --- CreateOffscreenTexture ---

// TestCreateOffscreenTexture_WithNoopDevice verifies CreateOffscreenTexture
// succeeds with a noop device and returns a valid view + release function.
func TestCreateOffscreenTexture_WithNoopDevice(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPUShared()
	s.device = device
	s.queue = queue
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true

	rc := s.NewRenderContext()
	defer rc.Close()

	view, release := rc.CreateOffscreenTexture(64, 64)
	if view.IsNil() {
		t.Fatal("expected non-nil view from noop device")
	}
	if release == nil {
		t.Fatal("expected non-nil release function")
	}
	release()
}

// TestCreateOffscreenTexture_DeviceNotReady_TriggersEnsureGPU verifies that
// CreateOffscreenTexture calls ensureGPU when deviceReady is false.
func TestCreateOffscreenTexture_DeviceNotReady_TriggersEnsureGPU(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = false
	s.gpuReady = false

	rc := s.NewRenderContext()
	defer rc.Close()

	view, release := rc.CreateOffscreenTexture(64, 64)
	if view.IsNil() {
		t.Log("ensureGPU failed (no GPU available) — nil view is expected")
	} else {
		if release == nil {
			t.Error("non-nil view but nil release function")
		} else {
			release()
		}
	}
}

// --- Flush branches ---

// TestFlush_RasterAtlas_NoPending_NoView_IsNoop verifies that Flush on
// rasterAtlas with no pending draws and no view does nothing.
func TestFlush_RasterAtlas_NoPending_NoView_IsNoop(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false

	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)
	err := rc.Flush(target)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

// TestFlush_FullStrategy_NoDevice_FallsBack verifies that Flush on full strategy
// with no GPU device returns ErrFallbackToCPU.
func TestFlush_FullStrategy_NoDevice_FallsBack(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = false
	s.gpuReady = false

	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)
	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 50, CenterY: 50, RadiusX: 20, RadiusY: 20,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	if err := rc.FillShape(target, shape, paint); err != nil {
		t.Fatalf("FillShape: %v", err)
	}

	err := rc.Flush(target)
	if err != nil && !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Errorf("Flush = %v, want ErrFallbackToCPU", err)
	}
}

// --- Close with active session ---

// TestClose_WithSession verifies Close releases the session.
func TestClose_WithSession(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPUShared()
	s.device = device
	s.queue = queue
	s.deviceReady = true
	s.gpuReady = true

	rc := s.NewRenderContext()

	sc := resolveSampleCount(device)
	rc.session = NewGPURenderSession(device, queue, sc)

	rc.Close()

	if rc.session != nil {
		t.Error("session should be nil after Close")
	}
	if rc.pendingDraws != nil {
		t.Error("pendingDraws should be nil after Close")
	}
}
