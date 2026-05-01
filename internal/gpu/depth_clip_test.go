//go:build !nogpu

package gpu

import (
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

	// Pipeline should not be created yet (lazy).
	if p.pipeline != nil {
		t.Error("expected nil pipeline before ensurePipeline")
	}

	// Force pipeline creation.
	if err := p.ensurePipeline(); err != nil {
		t.Fatalf("ensurePipeline failed: %v", err)
	}
	if p.pipeline == nil {
		t.Error("expected non-nil pipeline after ensurePipeline")
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
	if p.pipeline != nil {
		t.Error("expected nil pipeline after Destroy")
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
