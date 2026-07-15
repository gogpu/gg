// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// --- shapeToPath all shape kinds ---

// TestShapeToPath_AllKinds verifies shapeToPath handles all DetectedShape kinds:
// Circle, Ellipse, Rect, RRect, and Unknown (nil). Covers the 35.7% to 100% gap.
func TestShapeToPath_AllKinds(t *testing.T) {
	tests := []struct {
		name    string
		shape   gg.DetectedShape
		wantNil bool
	}{
		{
			name:    "circle",
			shape:   gg.DetectedShape{Kind: gg.ShapeCircle, CenterX: 50, CenterY: 50, RadiusX: 20, RadiusY: 20},
			wantNil: false,
		},
		{
			name:    "ellipse",
			shape:   gg.DetectedShape{Kind: gg.ShapeEllipse, CenterX: 50, CenterY: 50, RadiusX: 30, RadiusY: 20},
			wantNil: false,
		},
		{
			name:    "rect",
			shape:   gg.DetectedShape{Kind: gg.ShapeRect, CenterX: 50, CenterY: 50, Width: 40, Height: 30},
			wantNil: false,
		},
		{
			name:    "rrect",
			shape:   gg.DetectedShape{Kind: gg.ShapeRRect, CenterX: 50, CenterY: 50, Width: 40, Height: 30, CornerRadius: 5},
			wantNil: false,
		},
		{
			name:    "unknown returns nil",
			shape:   gg.DetectedShape{Kind: gg.ShapeKind(99)},
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := shapeToPath(tt.shape)
			if (p == nil) != tt.wantNil {
				t.Errorf("shapeToPath(%s) nil=%v, want nil=%v", tt.name, p == nil, tt.wantNil)
			}
			if p != nil && p.NumVerbs() == 0 {
				t.Errorf("shapeToPath(%s) returned empty path (0 verbs)", tt.name)
			}
		})
	}
}

// --- dispatchRasterAtlasDraws routing ---

// TestDispatchRasterAtlasDraws_PixmapPath verifies the pixmap path (no View)
// in dispatchRasterAtlasDraws.
func TestDispatchRasterAtlasDraws_PixmapPath(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()

	data := make([]byte, 64*64*4)
	target := gg.GPURenderTarget{
		Data:   data,
		Width:  64,
		Height: 64,
		Stride: 256,
	}

	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 32, CenterY: 32, RadiusX: 15, RadiusY: 15,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	rc.pendingDraws = append(rc.pendingDraws, drawCommand{
		kind:  drawCmdFillShape,
		shape: shape,
		paint: *paint,
	})

	err := rc.dispatchRasterAtlasDraws(target)
	if err != nil {
		t.Fatalf("dispatchRasterAtlasDraws: %v", err)
	}

	if len(rc.pendingDraws) != 0 {
		t.Errorf("pendingDraws not cleared: got %d", len(rc.pendingDraws))
	}

	idx := (32*64 + 32) * 4
	if data[idx+3] == 0 {
		t.Error("pixel (32,32) alpha=0 — expected coverage at circle center")
	}
}

// TestDispatchRasterAtlasDraws_ViewPath verifies the View path (offscreen texture)
// in dispatchRasterAtlasDraws.
func TestDispatchRasterAtlasDraws_ViewPath(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPUShared()
	s.device = device
	s.queue = queue
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "test_offscreen",
		Size:          wgpu.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopyDst | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Label:         "test_offscreen_view",
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Dimension:     gputypes.TextureViewDimension2D,
		Aspect:        gputypes.TextureAspectAll,
		MipLevelCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	defer view.Release()
	defer tex.Release()

	data := make([]byte, 8*8*4)
	target := gg.GPURenderTarget{
		Data:       data,
		Width:      8,
		Height:     8,
		Stride:     32,
		View:       gpucontext.NewTextureView(unsafe.Pointer(view)),
		ViewWidth:  8,
		ViewHeight: 8,
	}

	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 4, CenterY: 4, RadiusX: 3, RadiusY: 3,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	rc.pendingDraws = append(rc.pendingDraws, drawCommand{
		kind:  drawCmdFillShape,
		shape: shape,
		paint: *paint,
	})

	err = rc.dispatchRasterAtlasDraws(target)
	if err != nil {
		t.Fatalf("dispatchRasterAtlasDraws (view path): %v", err)
	}

	if len(rc.pendingDraws) != 0 {
		t.Errorf("pendingDraws not cleared: got %d", len(rc.pendingDraws))
	}
}

// --- dispatchFillShape unclipped SDF fallback ---

// TestDispatchFillShape_UnclippedSDFFallback verifies that when SDF accelerator
// fails for a shape, the fallback converts to path via shapeToPath and fills.
func TestDispatchFillShape_UnclippedSDFFallback(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()

	shape := gg.DetectedShape{
		Kind: gg.ShapeRect, CenterX: 50, CenterY: 50, Width: 40, Height: 30,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Green))

	rc.pendingDraws = append(rc.pendingDraws, drawCommand{
		kind:  drawCmdFillShape,
		shape: shape,
		paint: *paint,
	})

	pm := gg.NewPixmap(100, 100)
	sr := gg.NewSoftwareRenderer(100, 100)
	rc.dispatchDrawsToSoftware(pm, sr)

	data := pm.Data()
	nonzero := 0
	for i := 3; i < len(data); i += 4 {
		if data[i] != 0 {
			nonzero++
		}
	}
	if nonzero == 0 {
		t.Error("no pixels rendered — both SDF and path fallback failed for rect shape")
	}
}

// --- dispatchStrokeShape unclipped SDF paths ---

// TestDispatchStrokeShape_UnclippedSDF verifies the unclipped SDF stroke path
// where cpuFallback.StrokeShape succeeds.
func TestDispatchStrokeShape_UnclippedSDF(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()

	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 50, CenterY: 50, RadiusX: 20, RadiusY: 20,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	paint.SetStroke(gg.Stroke{Width: 4.0})

	rc.pendingDraws = append(rc.pendingDraws, drawCommand{
		kind:  drawCmdStrokeShape,
		shape: shape,
		paint: *paint,
	})

	pm := gg.NewPixmap(100, 100)
	sr := gg.NewSoftwareRenderer(100, 100)
	rc.dispatchDrawsToSoftware(pm, sr)

	data := pm.Data()
	nonzero := 0
	for i := 3; i < len(data); i += 4 {
		if data[i] != 0 {
			nonzero++
		}
	}
	if nonzero == 0 {
		t.Error("no pixels rendered — SDF stroke dispatch failed")
	}
}

// TestDispatchStrokeShape_UnclippedSDF_RectFallback verifies that when SDF
// cannot handle a shape stroke (e.g., rect), the path fallback via shapeToPath
// kicks in and produces pixels.
func TestDispatchStrokeShape_UnclippedSDF_RectFallback(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()

	shape := gg.DetectedShape{
		Kind: gg.ShapeRect, CenterX: 50, CenterY: 50, Width: 60, Height: 40,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Green))
	paint.SetStroke(gg.Stroke{Width: 3.0})

	rc.pendingDraws = append(rc.pendingDraws, drawCommand{
		kind:  drawCmdStrokeShape,
		shape: shape,
		paint: *paint,
	})

	pm := gg.NewPixmap(100, 100)
	sr := gg.NewSoftwareRenderer(100, 100)
	rc.dispatchDrawsToSoftware(pm, sr)

	data := pm.Data()
	nonzero := 0
	for i := 3; i < len(data); i += 4 {
		if data[i] != 0 {
			nonzero++
		}
	}
	if nonzero == 0 {
		t.Error("no pixels rendered — rect stroke fallback failed")
	}
}

// --- drawCmdStrokePath dispatch must force AnalyticFiller ---

// TestDispatchStrokePath_ForcesAnalyticFiller verifies that pre-expanded stroke
// paths dispatched via the draw queue produce pixel-correct output matching
// SoftwareRenderer.Stroke() (which forces RasterizerAnalytic internally).
//
// Background: stroke expansion produces multi-contour fill paths (inner + outer
// outline). SoftwareRenderer.Stroke() forces RasterizerAnalytic because
// SparseStripsFiller does not correctly handle multi-contour winding. The draw
// queue dispatch (drawCmdStrokePath) was calling sr.Fill() with default Auto
// mode, routing expanded strokes through SparseStripsFiller and causing
// visible artifacts (white/gray residual marks inside stroke rings).
//
// The test draws a large stroked circle (radius=120, lineWidth=1.5) which
// produces a path with many verbs (well above the SparseStrips threshold).
// If the dispatch incorrectly routes to SparseStripsFiller, the center pixel
// will have non-zero coverage (artifact). With correct AnalyticFiller routing,
// the center pixel remains zero (only the stroke ring has coverage).
func TestDispatchStrokePath_ForcesAnalyticFiller(t *testing.T) {
	// Register CoverageFiller so auto-selection can route to SparseStrips.
	prevFiller := gg.GetCoverageFiller()
	gg.RegisterCoverageFiller(&AdaptiveFiller{})
	defer func() {
		if prevFiller != nil {
			gg.RegisterCoverageFiller(prevFiller)
		}
	}()

	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()

	const size = 300
	const cx, cy = 150.0, 150.0
	const radius = 120.0
	const lineWidth = 1.5

	// Build a circle path (same as DrawCircle → Ellipse).
	circlePath := gg.NewPath()
	circlePath.Ellipse(cx, cy, radius, radius)
	circlePath.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.White))
	paint.SetStroke(gg.Stroke{Width: lineWidth})

	// Queue as stroke path — preTessellateStroke expands stroke geometry.
	cmd := drawCommand{
		kind:  drawCmdStrokePath,
		path:  circlePath.Clone(),
		paint: *paint,
	}
	rc.preTessellateStroke(&cmd)

	// Verify pre-tessellation produced a multi-contour path with many verbs.
	if cmd.path == nil {
		t.Fatal("preTessellateStroke produced nil path")
	}
	if cmd.path.NumVerbs() < 64 {
		t.Skipf("expanded stroke has only %d verbs (below SparseStrips threshold); "+
			"test requires enough verbs to trigger SparseStripsFiller routing", cmd.path.NumVerbs())
	}

	// Dispatch to software (this is the code path under test).
	rc.pendingDraws = append(rc.pendingDraws, cmd)
	pm := gg.NewPixmap(size, size)
	sr := gg.NewSoftwareRenderer(size, size)
	rc.dispatchDrawsToSoftware(pm, sr)

	// Verify: center pixel (150,150) must have zero alpha.
	// The stroked circle ring is at radius=120±0.75 from center.
	// Center pixel is 120px away from the ring — no coverage expected.
	data := pm.Data()
	centerIdx := (int(cy)*size + int(cx)) * 4
	centerAlpha := data[centerIdx+3]
	if centerAlpha != 0 {
		t.Errorf("stroke dispatch artifact: center pixel (%d,%d) alpha=%d, want 0 "+
			"(SparseStripsFiller multi-contour winding bug)", int(cx), int(cy), centerAlpha)
	}

	// Also check a point well inside the ring (30px from center = 90px from ring).
	innerIdx := (int(cy)*size + int(cx) + 30) * 4
	innerAlpha := data[innerIdx+3]
	if innerAlpha != 0 {
		t.Errorf("stroke dispatch artifact: inner pixel (%d,%d) alpha=%d, want 0",
			int(cx)+30, int(cy), innerAlpha)
	}

	// Verify some pixels ON the stroke ring do have coverage.
	ringIdx := (int(cy)*size + int(cx+radius)) * 4
	ringAlpha := data[ringIdx+3]
	if ringAlpha == 0 {
		t.Error("no pixels on stroke ring — stroke dispatch produced nothing")
	}
}

// TestDispatchStrokePath_MatchesAnalyticFiller verifies that the draw queue
// dispatch for stroke paths produces the SAME output as a direct sr.Fill()
// with forced RasterizerAnalytic on the same pre-expanded path.
//
// This isolates the filler selection: both paths use the same expanded stroke
// geometry — the only difference is whether the dispatch correctly forces
// AnalyticFiller or incorrectly allows SparseStripsFiller.
//
// The test registers the real AdaptiveFiller (SparseStrips) so that
// SoftwareRenderer.Fill() with RasterizerAuto routes complex paths through
// SparseStripsFiller. Without this registration, all paths always go through
// AnalyticFiller and the bug is invisible.
func TestDispatchStrokePath_MatchesAnalyticFiller(t *testing.T) {
	// Save and restore the previous filler to avoid test pollution.
	prevFiller := gg.GetCoverageFiller()
	gg.RegisterCoverageFiller(&AdaptiveFiller{})
	defer func() {
		if prevFiller != nil {
			gg.RegisterCoverageFiller(prevFiller)
		}
	}()

	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()

	const size = 200
	const cx, cy = 100.0, 100.0
	const radius = 80.0
	const lineWidth = 1.5

	circlePath := gg.NewPath()
	circlePath.Ellipse(cx, cy, radius, radius)
	circlePath.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.White))
	paint.SetStroke(gg.Stroke{Width: lineWidth})

	// Pre-tessellate: expand stroke to fill path (same code path for both).
	cmd := drawCommand{
		kind:  drawCmdStrokePath,
		path:  circlePath.Clone(),
		paint: *paint,
	}
	rc.preTessellateStroke(&cmd)
	if cmd.path == nil {
		t.Fatal("preTessellateStroke produced nil path")
	}
	t.Logf("expanded stroke path: %d verbs", cmd.path.NumVerbs())

	expandedPath := cmd.path.Clone() // save a copy for reference rendering

	// Path 1: draw queue dispatch (code under test).
	rc.pendingDraws = append(rc.pendingDraws, cmd)
	pmQueue := gg.NewPixmap(size, size)
	srQueue := gg.NewSoftwareRenderer(size, size)
	rc.dispatchDrawsToSoftware(pmQueue, srQueue)

	// Path 2: direct sr.Fill() with forced AnalyticFiller on same expanded path.
	pmRef := gg.NewPixmap(size, size)
	srRef := gg.NewSoftwareRenderer(size, size)
	srRef.SetRasterizerMode(gg.RasterizerAnalytic)
	refPaint := cmd.paint // same paint (EvenOdd fill rule set by preTessellateStroke)
	if err := srRef.Fill(pmRef, expandedPath, &refPaint); err != nil {
		t.Fatalf("reference Fill: %v", err)
	}

	// Compare: pixels must be identical (same geometry, same filler = same output).
	dataQueue := pmQueue.Data()
	dataRef := pmRef.Data()
	maxDiff := 0
	diffPixels := 0
	for i := 3; i < len(dataQueue); i += 4 {
		d := int(dataQueue[i]) - int(dataRef[i])
		if d < 0 {
			d = -d
		}
		if d > maxDiff {
			maxDiff = d
		}
		if d > 0 {
			diffPixels++
		}
	}

	if diffPixels > 0 {
		t.Errorf("stroke dispatch vs forced AnalyticFiller: %d pixels differ (maxDiff=%d); "+
			"draw queue dispatch likely routed expanded stroke to SparseStripsFiller "+
			"instead of forcing AnalyticFiller for multi-contour winding",
			diffPixels, maxDiff)
	}
}

// --- flushVello ---

// TestFlushVello_NoVelloAccel_Noop verifies flushVello is a no-op when
// VelloAccelerator is nil.
func TestFlushVello_NoVelloAccel_Noop(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)
	err := rc.flushVello(target)
	if err != nil {
		t.Fatalf("flushVello: %v", err)
	}
}
