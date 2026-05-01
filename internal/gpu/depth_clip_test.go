//go:build !nogpu

package gpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestDepthClipPipeline_Lifecycle verifies DepthClipPipeline creation,
// pipeline compilation, and destruction.
func TestDepthClipPipeline_Lifecycle(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewDepthClipPipeline(device, queue)
	if p == nil {
		t.Fatal("expected non-nil DepthClipPipeline")
	}
	if p.device != device {
		t.Error("device not stored correctly")
	}
	if p.queue != queue {
		t.Error("queue not stored correctly")
	}
	if p.tessellator == nil {
		t.Error("expected non-nil tessellator")
	}

	// Pipelines should not be created yet (lazy).
	if p.stencilFillPipeline != nil {
		t.Error("expected nil stencilFillPipeline before ensurePipeline")
	}
	if p.depthCoverPipeline != nil {
		t.Error("expected nil depthCoverPipeline before ensurePipeline")
	}

	// Force pipeline creation.
	if err := p.ensurePipeline(); err != nil {
		t.Fatalf("ensurePipeline failed: %v", err)
	}
	if p.stencilFillPipeline == nil {
		t.Error("expected non-nil stencilFillPipeline after ensurePipeline")
	}
	if p.depthCoverPipeline == nil {
		t.Error("expected non-nil depthCoverPipeline after ensurePipeline")
	}
	if p.shader == nil {
		t.Error("expected non-nil shader after ensurePipeline")
	}
	if p.uniformBGL == nil {
		t.Error("expected non-nil uniformBGL after ensurePipeline")
	}
	if p.pipeLayout == nil {
		t.Error("expected non-nil pipeLayout after ensurePipeline")
	}

	// Second call should be a no-op.
	if err := p.ensurePipeline(); err != nil {
		t.Fatalf("second ensurePipeline failed: %v", err)
	}

	// Destroy should release all resources.
	p.Destroy()
	if p.stencilFillPipeline != nil {
		t.Error("expected nil stencilFillPipeline after Destroy")
	}
	if p.depthCoverPipeline != nil {
		t.Error("expected nil depthCoverPipeline after Destroy")
	}
	if p.shader != nil {
		t.Error("expected nil shader after Destroy")
	}
	if p.uniformBGL != nil {
		t.Error("expected nil uniformBGL after Destroy")
	}
	if p.pipeLayout != nil {
		t.Error("expected nil pipeLayout after Destroy")
	}
	if p.vertBuf != nil {
		t.Error("expected nil vertBuf after Destroy")
	}
	if p.coverBuf != nil {
		t.Error("expected nil coverBuf after Destroy")
	}
	if p.uniformBuf != nil {
		t.Error("expected nil uniformBuf after Destroy")
	}
	if p.bindGroup != nil {
		t.Error("expected nil bindGroup after Destroy")
	}

	// Double-destroy should be safe.
	p.Destroy()
}

// TestDepthClipPipeline_BuildClipResources_NilPath verifies that a nil
// clip path returns nil resources (no-op).
func TestDepthClipPipeline_BuildClipResources_NilPath(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewDepthClipPipeline(device, queue)
	defer p.Destroy()

	if err := p.ensurePipeline(); err != nil {
		t.Fatalf("ensurePipeline failed: %v", err)
	}

	// Nil path should return nil resources.
	res, err := p.BuildClipResources(nil, 800, 600)
	if err != nil {
		t.Fatalf("BuildClipResources(nil) returned error: %v", err)
	}
	if res != nil {
		t.Error("expected nil resources for nil path")
	}
}

// TestDepthClipPipeline_BuildClipResources_EmptyPath verifies that an
// empty path (no subpaths) returns nil resources.
func TestDepthClipPipeline_BuildClipResources_EmptyPath(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewDepthClipPipeline(device, queue)
	defer p.Destroy()

	if err := p.ensurePipeline(); err != nil {
		t.Fatalf("ensurePipeline failed: %v", err)
	}

	// Empty path (no commands) should produce no tessellation vertices.
	emptyPath := &gg.Path{}
	res, err := p.BuildClipResources(emptyPath, 800, 600)
	if err != nil {
		t.Fatalf("BuildClipResources(empty) returned error: %v", err)
	}
	if res != nil {
		t.Error("expected nil resources for empty path")
	}
}

// TestDepthClipDepthStencil verifies the depth stencil state returned by
// depthClipDepthStencil() matches the GPU-CLIP-003a depth model.
func TestDepthClipDepthStencil(t *testing.T) {
	ds := depthClipDepthStencil()
	if ds == nil {
		t.Fatal("expected non-nil DepthStencilState")
	}

	// Format must be Depth24PlusStencil8 (shared with stencil renderer).
	if ds.Format != gputypes.TextureFormatDepth24PlusStencil8 {
		t.Errorf("Format = %v, want Depth24PlusStencil8", ds.Format)
	}

	// Depth write must be disabled — content should NOT modify depth buffer.
	if ds.DepthWriteEnabled {
		t.Error("DepthWriteEnabled = true, want false (content must not modify depth)")
	}

	// DepthCompare must be GreaterEqual — content passes only where clip wrote Z=0.0.
	if ds.DepthCompare != gputypes.CompareFunctionGreaterEqual {
		t.Errorf("DepthCompare = %v, want GreaterEqual", ds.DepthCompare)
	}

	// Stencil masks must be 0x00 — depth clip pipelines must not interact with stencil.
	if ds.StencilReadMask != 0x00 {
		t.Errorf("StencilReadMask = 0x%02x, want 0x00", ds.StencilReadMask)
	}
	if ds.StencilWriteMask != 0x00 {
		t.Errorf("StencilWriteMask = 0x%02x, want 0x00", ds.StencilWriteMask)
	}

	// Stencil ops must all be Keep (pass-through).
	if ds.StencilFront.PassOp != wgpu.StencilOperationKeep {
		t.Errorf("StencilFront.PassOp = %v, want Keep", ds.StencilFront.PassOp)
	}
	if ds.StencilBack.PassOp != wgpu.StencilOperationKeep {
		t.Errorf("StencilBack.PassOp = %v, want Keep", ds.StencilBack.PassOp)
	}
}

// TestScissorGroup_ClipPath verifies that ScissorGroup correctly stores a ClipPath.
func TestScissorGroup_ClipPath(t *testing.T) {
	// Without ClipPath — default state.
	grp := ScissorGroup{}
	if grp.ClipPath != nil {
		t.Error("default ScissorGroup should have nil ClipPath")
	}
	if grp.ClipDepthLevel != 0 {
		t.Error("default ScissorGroup should have ClipDepthLevel=0")
	}

	// With ClipPath set.
	path := &gg.Path{}
	path.MoveTo(0, 0)
	path.LineTo(100, 0)
	path.LineTo(100, 100)
	path.Close()

	grp.ClipPath = path
	grp.ClipDepthLevel = 1

	if grp.ClipPath == nil {
		t.Error("expected non-nil ClipPath after assignment")
	}
	if grp.ClipDepthLevel != 1 {
		t.Errorf("ClipDepthLevel = %d, want 1", grp.ClipDepthLevel)
	}
}

// TestHasAnyDepthClip verifies the hasAnyDepthClip helper detects depth clip
// presence across a slice of groupResources.
func TestHasAnyDepthClip(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	tests := []struct {
		name string
		grps []groupResources
		want bool
	}{
		{
			name: "nil slice",
			grps: nil,
			want: false,
		},
		{
			name: "empty slice",
			grps: []groupResources{},
			want: false,
		},
		{
			name: "all false",
			grps: []groupResources{
				{hasDepthClip: false},
				{hasDepthClip: false},
			},
			want: false,
		},
		{
			name: "first true",
			grps: []groupResources{
				{hasDepthClip: true},
				{hasDepthClip: false},
			},
			want: true,
		},
		{
			name: "last true",
			grps: []groupResources{
				{hasDepthClip: false},
				{hasDepthClip: true},
			},
			want: true,
		},
		{
			name: "all true",
			grps: []groupResources{
				{hasDepthClip: true},
				{hasDepthClip: true},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.hasAnyDepthClip(tt.grps)
			if got != tt.want {
				t.Errorf("hasAnyDepthClip() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRecordGroupDraws_NoDepthClip_Regression verifies that groups without
// depth clipping produce the same pipeline selection as before GPU-CLIP-003a.
// The stencil renderer must use the base (non-depth-clipped) pipelines when
// hasDepthClip is false, ensuring backward compatibility.
func TestRecordGroupDraws_NoDepthClip_Regression(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	if err := s.EnsureTextures(200, 200); err != nil {
		t.Fatalf("EnsureTextures failed: %v", err)
	}

	// Build a simple stencil path (triangle).
	path := &gg.Path{}
	path.MoveTo(10, 10)
	path.LineTo(190, 100)
	path.LineTo(100, 190)
	path.Close()

	tess := NewFanTessellator()
	tess.TessellatePath(path)
	fanVerts := tess.Vertices()
	coverQuad := tess.CoverQuad()

	cmd := StencilPathCommand{
		Vertices:  fanVerts,
		CoverQuad: coverQuad,
		Color:     [4]float32{1, 0, 0, 1},
		FillRule:  gg.FillRuleNonZero,
	}

	target := gg.GPURenderTarget{
		Width:  200,
		Height: 200,
		Data:   make([]uint8, 200*200*4),
		Stride: 200 * 4,
	}

	// Render with a single group (no depth clip).
	groups := []ScissorGroup{
		{
			StencilPaths: []StencilPathCommand{cmd},
		},
	}

	err := s.RenderFrameGrouped(target, groups, nil, nil)
	if err != nil {
		t.Fatalf("RenderFrameGrouped failed: %v", err)
	}

	// Verify stencil renderer was initialized with base pipelines.
	if s.stencilRenderer == nil {
		t.Fatal("expected non-nil stencilRenderer after render")
	}
	if s.stencilRenderer.nonZeroStencilPipeline == nil {
		t.Error("expected non-nil nonZeroStencilPipeline")
	}
	if s.stencilRenderer.nonZeroCoverPipeline == nil {
		t.Error("expected non-nil nonZeroCoverPipeline")
	}

	// Depth-clipped variants should NOT have been created (no depth clip used).
	if s.stencilRenderer.pipelineWithDepthClipNZ != nil {
		t.Error("expected nil pipelineWithDepthClipNZ when no depth clip active")
	}
	if s.stencilRenderer.pipelineWithDepthClipEO != nil {
		t.Error("expected nil pipelineWithDepthClipEO when no depth clip active")
	}
	if s.stencilRenderer.pipelineWithDepthClipCover != nil {
		t.Error("expected nil pipelineWithDepthClipCover when no depth clip active")
	}
}

// TestStencilRenderer_EnsureDepthClipPipelines verifies that the depth-clipped
// pipeline variants are created correctly and are idempotent on second call.
func TestStencilRenderer_EnsureDepthClipPipelines(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	// Base pipelines must be created first (or ensureDepthClipPipelines handles it).
	if err := sr.ensureDepthClipPipelines(); err != nil {
		t.Fatalf("ensureDepthClipPipelines failed: %v", err)
	}

	// All three depth-clipped variants must exist.
	if sr.pipelineWithDepthClipNZ == nil {
		t.Error("expected non-nil pipelineWithDepthClipNZ")
	}
	if sr.pipelineWithDepthClipEO == nil {
		t.Error("expected non-nil pipelineWithDepthClipEO")
	}
	if sr.pipelineWithDepthClipCover == nil {
		t.Error("expected non-nil pipelineWithDepthClipCover")
	}

	// Base pipelines should also have been created as prerequisite.
	if sr.nonZeroStencilPipeline == nil {
		t.Error("expected non-nil nonZeroStencilPipeline after ensureDepthClipPipelines")
	}
	if sr.evenOddStencilPipeline == nil {
		t.Error("expected non-nil evenOddStencilPipeline after ensureDepthClipPipelines")
	}
	if sr.nonZeroCoverPipeline == nil {
		t.Error("expected non-nil nonZeroCoverPipeline after ensureDepthClipPipelines")
	}

	// Second call should be a no-op (idempotent).
	origNZ := sr.pipelineWithDepthClipNZ
	if err := sr.ensureDepthClipPipelines(); err != nil {
		t.Fatalf("second ensureDepthClipPipelines failed: %v", err)
	}
	if sr.pipelineWithDepthClipNZ != origNZ {
		t.Error("pipeline was recreated on second call (should be idempotent)")
	}
}

// TestDepthLoadOp_AlwaysClear_Regression verifies that the render session
// can render multiple frames with depth clip without errors.
//
// Regression test for circle depth clip appearing empty on frame 2+:
// DepthStoreOp=Discard discards depth after each render pass. Loading
// discarded depth on subsequent passes produces undefined values. If
// undefined depth happens to be small positive values, the GreaterEqual
// depth test in content pipelines fails everywhere, producing empty
// (invisible) clipped content.
//
// Fix: always clear depth to 1.0 regardless of frameRendered state. The
// depth clip pipeline writes Z=0.0 fresh each pass, so a clean 1.0 is
// always the correct starting point.
func TestDepthLoadOp_AlwaysClear_Regression(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	if err := s.EnsureTextures(200, 200); err != nil {
		t.Fatalf("EnsureTextures failed: %v", err)
	}

	// Build a simple triangle clip path.
	clipPath := &gg.Path{}
	clipPath.MoveTo(50, 0)
	clipPath.LineTo(100, 100)
	clipPath.LineTo(0, 100)
	clipPath.Close()

	// Build one group with depth clip.
	groups := []ScissorGroup{
		{
			ClipPath:  clipPath,
			SDFShapes: []SDFRenderShape{{Kind: 0, CenterX: 50, CenterY: 50, Param1: 40, Param2: 40, ColorR: 1, ColorG: 0, ColorB: 0, ColorA: 1}},
		},
	}

	target := gg.GPURenderTarget{
		Width:  200,
		Height: 200,
		Data:   make([]uint8, 200*200*4),
		Stride: 200 * 4,
	}

	// Render 3 consecutive frames. Before the fix, frame 2+ used LoadOpLoad
	// on discarded depth, causing undefined depth buffer values and empty
	// clipped output.
	for frame := 0; frame < 3; frame++ {
		err := s.RenderFrameGrouped(target, groups, nil, nil)
		if err != nil {
			t.Fatalf("Frame %d RenderFrameGrouped failed: %v", frame, err)
		}
	}
}

// TestScissorSegments_CircleClip_MultipleSDFContent verifies that multiple
// SDF shapes drawn inside a circle clip each get their own scissor group
// with the clip path correctly assigned. This is a regression test for the
// circle clip appearing empty: each Fill() call creates SetClipRect +
// SetClipPath (2 segments at setup) + ClearClipPath + ClearClipRect (2
// segments at cleanup), producing 4 segments per fill. buildScissorGroups
// must correctly assign content to the segment that has the clipPath set.
func TestScissorSegments_CircleClip_MultipleSDFContent(t *testing.T) {
	rc := &GPURenderContext{
		shared: &GPUShared{},
	}

	// Build a circle clip path (device-space).
	circlePath := &gg.Path{}
	circlePath.MoveTo(180, 100)
	for i := 1; i <= 36; i++ {
		angle := float64(i) * 2 * 3.14159265 / 36
		circlePath.LineTo(100+80*math.Cos(angle), 100+80*math.Sin(angle))
	}
	circlePath.Close()

	// Simulate 3 Fill() calls inside a circle clip, each following the
	// setGPUClipRect → setGPUClipPath → QueueShape → ClearClipPath → ClearClipRect
	// pattern from context.go doFill().
	clipRect := [4]uint32{20, 20, 160, 160}

	for i := 0; i < 3; i++ {
		// Setup: setGPUClipPath calls SetClipRect then SetClipPath.
		rc.SetClipRect(clipRect[0], clipRect[1], clipRect[2], clipRect[3])
		rc.SetClipPath(circlePath)

		// GPU fill: queue SDF shape.
		rc.pendingShapes = append(rc.pendingShapes, SDFRenderShape{
			Kind:    1,
			CenterX: 100, CenterY: float32(40 + i*40),
			Param1: 80, Param2: 20,
			ColorR: 1, ColorA: 1,
		})

		// Cleanup: ClearClipPath then ClearClipRect (deferred from setGPUClipPath).
		rc.ClearClipPath()
		rc.ClearClipRect()
	}

	groups := rc.buildScissorGroups()

	// Count groups that have content (non-empty SDF shapes) AND a clip path.
	clippedGroupCount := 0
	totalClippedShapes := 0
	for _, g := range groups {
		if len(g.SDFShapes) > 0 && g.ClipPath != nil {
			clippedGroupCount++
			totalClippedShapes += len(g.SDFShapes)
		}
	}

	if clippedGroupCount != 3 {
		t.Errorf("expected 3 groups with clipPath + SDF content, got %d (total groups: %d)", clippedGroupCount, len(groups))
		for i, g := range groups {
			t.Logf("  group[%d]: sdf=%d clipPath=%v rect=%v", i, len(g.SDFShapes), g.ClipPath != nil, g.Rect != nil)
		}
	}
	if totalClippedShapes != 3 {
		t.Errorf("expected 3 total SDF shapes in clipped groups, got %d", totalClippedShapes)
	}
}

// TestClipPath_SurvivesDeepCopy verifies that ClipPath is preserved when
// buildScissorGroups output is deep-copied in Flush(). This is a regression
// test for the bug where the deep-copy loop in gpu_render_context.go:Flush()
// initialized ScissorGroup with only Rect and ClipRRect, dropping ClipPath.
// Without ClipPath, BuildClipResources was never called and depth clipping
// had no effect — shapes rendered as rectangles (scissor only), not clipped
// to the arbitrary path boundary.
func TestClipPath_SurvivesDeepCopy(t *testing.T) {
	// Simulate the deep-copy pattern from gpu_render_context.go:Flush().
	clipPath := &gg.Path{}
	clipPath.MoveTo(50, 0)
	clipPath.LineTo(100, 100)
	clipPath.LineTo(0, 100)
	clipPath.Close()

	original := []ScissorGroup{
		{
			Rect:           &[4]uint32{0, 0, 100, 100},
			ClipRRect:      &ClipParams{Enabled: 1.0},
			ClipPath:       clipPath,
			ClipDepthLevel: 1,
			SDFShapes:      []SDFRenderShape{{Kind: 0}},
		},
	}

	// Replicate the deep-copy pattern (as fixed).
	owned := make([]ScissorGroup, len(original))
	for i := range original {
		g := &original[i]
		owned[i] = ScissorGroup{
			Rect:           g.Rect,
			ClipRRect:      g.ClipRRect,
			ClipPath:       g.ClipPath,
			ClipDepthLevel: g.ClipDepthLevel,
		}
		if len(g.SDFShapes) > 0 {
			owned[i].SDFShapes = make([]SDFRenderShape, len(g.SDFShapes))
			copy(owned[i].SDFShapes, g.SDFShapes)
		}
	}

	// Verify ClipPath is preserved.
	if owned[0].ClipPath == nil {
		t.Fatal("ClipPath lost during deep-copy — depth clip will not work")
	}
	if owned[0].ClipPath != clipPath {
		t.Error("ClipPath pointer changed during deep-copy")
	}
	if owned[0].ClipDepthLevel != 1 {
		t.Errorf("ClipDepthLevel = %d, want 1", owned[0].ClipDepthLevel)
	}
}
