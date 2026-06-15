//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestBuildGlyphMaskDrawCalls_QuadOffsetOnNilBindGroup is a regression test
// for BUG-GLYPHMASK-001: when a bind group is nil, buildGlyphMaskDrawCalls
// must still advance quadOffset so subsequent batches get correct indexOffset.
func TestBuildGlyphMaskDrawCalls_QuadOffsetOnNilBindGroup(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	// Create 3 batches: batch 0 (5 quads), batch 1 (3 quads), batch 2 (4 quads).
	batches := []GlyphMaskBatch{
		{Quads: make([]GlyphMaskQuad, 5), Transform: gg.Identity(), Color: [4]float32{1, 1, 1, 1}},
		{Quads: make([]GlyphMaskQuad, 3), Transform: gg.Identity(), Color: [4]float32{1, 0, 0, 1}},
		{Quads: make([]GlyphMaskQuad, 4), Transform: gg.Identity(), Color: [4]float32{0, 1, 0, 1}},
	}

	// Allocate uniform buffer pool for 3 batches.
	s.ensureGlyphMaskBatchPools(len(batches))

	// Set bind groups: batch 0 = nil (simulates failed atlas sync),
	// batch 1 and 2 = valid bind groups.
	mockBG := createMockBindGroup(t, device, s)
	s.glyphMaskBindGroups[0] = nil // nil → batch skipped
	s.glyphMaskBindGroups[1] = mockBG
	s.glyphMaskBindGroups[2] = mockBG

	s.frameW = 800
	s.frameH = 600

	drawCalls, err := s.buildGlyphMaskDrawCalls(batches, 800, 600)
	if err != nil {
		t.Fatalf("buildGlyphMaskDrawCalls failed: %v", err)
	}

	// Batch 0 skipped (nil bind group) → 2 draw calls from batches 1 and 2.
	if len(drawCalls) != 2 {
		t.Fatalf("expected 2 draw calls, got %d", len(drawCalls))
	}

	// Batch 1: should start at quadOffset=5 (batch 0's 5 quads skipped but counted).
	// indexOffset = 5 * 6 = 30
	if drawCalls[0].indexOffset != 30 {
		t.Errorf("draw call 0: indexOffset = %d, want 30 (5 skipped quads × 6 indices)",
			drawCalls[0].indexOffset)
	}
	if drawCalls[0].indexCount != 18 { // 3 quads × 6
		t.Errorf("draw call 0: indexCount = %d, want 18", drawCalls[0].indexCount)
	}

	// Batch 2: should start at quadOffset=8 (5 + 3).
	// indexOffset = 8 * 6 = 48
	if drawCalls[1].indexOffset != 48 {
		t.Errorf("draw call 1: indexOffset = %d, want 48 (8 quads × 6 indices)",
			drawCalls[1].indexOffset)
	}
	if drawCalls[1].indexCount != 24 { // 4 quads × 6
		t.Errorf("draw call 1: indexCount = %d, want 24", drawCalls[1].indexCount)
	}
}

// TestBuildGlyphMaskDrawCalls_AllBindGroupsValid verifies correct offsets
// when no batches are skipped (normal operation).
func TestBuildGlyphMaskDrawCalls_AllBindGroupsValid(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	batches := []GlyphMaskBatch{
		{Quads: make([]GlyphMaskQuad, 2), Transform: gg.Identity(), Color: [4]float32{1, 1, 1, 1}},
		{Quads: make([]GlyphMaskQuad, 3), Transform: gg.Identity(), Color: [4]float32{1, 1, 1, 1}},
	}

	s.ensureGlyphMaskBatchPools(len(batches))
	mockBG := createMockBindGroup(t, device, s)
	s.glyphMaskBindGroups[0] = mockBG
	s.glyphMaskBindGroups[1] = mockBG

	s.frameW = 800
	s.frameH = 600

	drawCalls, err := s.buildGlyphMaskDrawCalls(batches, 800, 600)
	if err != nil {
		t.Fatalf("buildGlyphMaskDrawCalls failed: %v", err)
	}

	if len(drawCalls) != 2 {
		t.Fatalf("expected 2 draw calls, got %d", len(drawCalls))
	}

	// Batch 0: offset=0, count=12
	if drawCalls[0].indexOffset != 0 {
		t.Errorf("draw call 0: indexOffset = %d, want 0", drawCalls[0].indexOffset)
	}
	if drawCalls[0].indexCount != 12 {
		t.Errorf("draw call 0: indexCount = %d, want 12", drawCalls[0].indexCount)
	}

	// Batch 1: offset=12, count=18
	if drawCalls[1].indexOffset != 12 {
		t.Errorf("draw call 1: indexOffset = %d, want 12", drawCalls[1].indexOffset)
	}
	if drawCalls[1].indexCount != 18 {
		t.Errorf("draw call 1: indexCount = %d, want 18", drawCalls[1].indexCount)
	}
}

// TestBuildGlyphMaskDrawCalls_EmptyBatchSkipped verifies that batches with
// zero quads are skipped without affecting quadOffset.
func TestBuildGlyphMaskDrawCalls_EmptyBatchSkipped(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	batches := []GlyphMaskBatch{
		{Quads: make([]GlyphMaskQuad, 3), Transform: gg.Identity(), Color: [4]float32{1, 1, 1, 1}},
		{Quads: nil, Transform: gg.Identity(), Color: [4]float32{1, 1, 1, 1}},
		{Quads: make([]GlyphMaskQuad, 2), Transform: gg.Identity(), Color: [4]float32{1, 1, 1, 1}},
	}

	s.ensureGlyphMaskBatchPools(len(batches))
	mockBG := createMockBindGroup(t, device, s)
	s.glyphMaskBindGroups[0] = mockBG
	s.glyphMaskBindGroups[1] = mockBG
	s.glyphMaskBindGroups[2] = mockBG

	s.frameW = 800
	s.frameH = 600

	drawCalls, err := s.buildGlyphMaskDrawCalls(batches, 800, 600)
	if err != nil {
		t.Fatalf("buildGlyphMaskDrawCalls failed: %v", err)
	}

	if len(drawCalls) != 2 {
		t.Fatalf("expected 2 draw calls (empty batch skipped), got %d", len(drawCalls))
	}

	if drawCalls[0].indexOffset != 0 {
		t.Errorf("draw call 0: indexOffset = %d, want 0", drawCalls[0].indexOffset)
	}
	// Batch 1 empty (0 quads) → no offset advance.
	// Batch 2: offset = 3 * 6 = 18
	if drawCalls[1].indexOffset != 18 {
		t.Errorf("draw call 1: indexOffset = %d, want 18", drawCalls[1].indexOffset)
	}
}

// createMockBindGroup creates a minimal bind group for testing.
func createMockBindGroup(t *testing.T, device *wgpu.Device, s *GPURenderSession) *wgpu.BindGroup {
	t.Helper()

	if err := s.ensureGlyphMaskPipeline(false); err != nil {
		t.Fatalf("ensureGlyphMaskPipeline failed: %v", err)
	}

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "test_atlas",
		Size:          wgpu.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatR8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture failed: %v", err)
	}
	t.Cleanup(func() { tex.Release() })

	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Format:        gputypes.TextureFormatR8Unorm,
		Dimension:     gputypes.TextureViewDimension2D,
		MipLevelCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTextureView failed: %v", err)
	}
	t.Cleanup(func() { view.Release() })

	s.SetGlyphMaskAtlasView(0, view, false)

	bg := s.glyphMaskBindGroups[0]
	if bg == nil {
		t.Fatal("bind group not created by SetGlyphMaskAtlasView")
	}
	return bg
}
