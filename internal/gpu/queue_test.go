// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
)

// --- QueueImageDraw ---

// TestQueueImageDraw_AppendsCommand verifies QueueImageDraw builds an
// ImageDrawCommand and appends it to pendingDraws (ADR-051 Phase 2 Step 4).
func TestQueueImageDraw_AppendsCommand(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)
	pixels := make([]byte, 16*16*4)
	for i := 0; i < len(pixels); i += 4 {
		pixels[i] = 255
		pixels[i+3] = 255
	}

	before := len(rc.pendingDraws)
	rc.QueueImageDraw(target, pixels, 42, 16, 16, 64,
		10, 20, 32, 32, 1.0, 100, 100,
		0, 0, 1, 1)

	if len(rc.pendingDraws) != before+1 {
		t.Errorf("QueueImageDraw: expected %d pendingDraws, got %d", before+1, len(rc.pendingDraws))
	}

	draw := rc.pendingDraws[before]
	if draw.kind != drawCmdImage {
		t.Fatalf("expected drawCmdImage, got %d", draw.kind)
	}
	cmd := draw.imageCmd.(ImageDrawCommand)
	if cmd.GenerationID != 42 {
		t.Errorf("GenerationID = %d, want 42", cmd.GenerationID)
	}
	if cmd.ImgWidth != 16 || cmd.ImgHeight != 16 {
		t.Errorf("dimensions = %dx%d, want 16x16", cmd.ImgWidth, cmd.ImgHeight)
	}
	if cmd.DstX != 10 || cmd.DstY != 20 {
		t.Errorf("DstX,DstY = (%f,%f), want (10,20)", cmd.DstX, cmd.DstY)
	}
	if cmd.Opacity != 1.0 {
		t.Errorf("Opacity = %f, want 1.0", cmd.Opacity)
	}
	if !rc.hasPendingTarget {
		t.Error("QueueImageDraw: hasPendingTarget should be true")
	}
}

// --- QueueGPUTextureDraw ---

// TestQueueGPUTextureDraw_AppendsCommand verifies QueueGPUTextureDraw appends
// to pendingDraws (ADR-051 Phase 2 Step 5).
func TestQueueGPUTextureDraw_AppendsCommand(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)

	before := len(rc.pendingDraws)
	rc.QueueGPUTextureDraw(target, gpucontext.TextureView{},
		10, 20, 50, 50, 0.8, 100, 100)

	if len(rc.pendingDraws) != before+1 {
		t.Errorf("expected %d pendingDraws, got %d", before+1, len(rc.pendingDraws))
	}

	draw := rc.pendingDraws[before]
	if draw.kind != drawCmdGPUTexture {
		t.Fatalf("expected drawCmdGPUTexture, got %d", draw.kind)
	}
	cmd := draw.gpuTexCmd.(GPUTextureDrawCommand)
	if cmd.DstX != 10 || cmd.DstY != 20 {
		t.Errorf("DstX,DstY = (%f,%f), want (10,20)", cmd.DstX, cmd.DstY)
	}
	if cmd.Opacity != 0.8 {
		t.Errorf("Opacity = %f, want 0.8", cmd.Opacity)
	}
}

// --- QueueText / QueueGlyphMask clip-boundary isolation ---

// TestQueueText_ClipChangePreventsCoalescing verifies that text batch coalescing
// is blocked when the clip state changes between QueueText calls (ADR-051
// per-draw clip prevents merge across clip boundaries).
func TestQueueText_ClipChangePreventsCoalescing(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)

	batch1 := TextBatch{
		Quads:      make([]TextQuad, 1),
		Color:      gg.Red,
		Transform:  gg.Identity(),
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  512,
	}
	batch2 := TextBatch{
		Quads:      make([]TextQuad, 1),
		Color:      gg.Red,
		Transform:  gg.Identity(),
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  512,
	}

	rc.QueueText(target, batch1)
	textDraws := collectTextDraws(rc)
	if len(textDraws) != 1 {
		t.Fatalf("expected 1 text draw after first QueueText, got %d", len(textDraws))
	}

	// Change clip — should prevent merge with the previous batch.
	clip := [4]uint32{10, 10, 80, 80}
	rc.clipRect = &clip
	rc.QueueText(target, batch2)
	textDraws = collectTextDraws(rc)
	if len(textDraws) != 2 {
		t.Errorf("expected 2 text draws (clip change prevents merge), got %d", len(textDraws))
	}
}

// TestQueueGlyphMask_ClipChangePreventsCoalescing verifies glyph mask batch
// coalescing is blocked when clip state changes between calls.
func TestQueueGlyphMask_ClipChangePreventsCoalescing(t *testing.T) {
	s := NewGPUShared()
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(100, 100)

	batch1 := GlyphMaskBatch{
		Quads:          make([]GlyphMaskQuad, 1),
		Color:          [4]float32{1, 0, 0, 1},
		Transform:      gg.Identity(),
		AtlasPageIndex: 0,
	}
	batch2 := GlyphMaskBatch{
		Quads:          make([]GlyphMaskQuad, 1),
		Color:          [4]float32{1, 0, 0, 1},
		Transform:      gg.Identity(),
		AtlasPageIndex: 0,
	}

	rc.QueueGlyphMask(target, batch1)
	glyphDraws := collectGlyphMaskDraws(rc)
	if len(glyphDraws) != 1 {
		t.Fatalf("expected 1 glyph draw, got %d", len(glyphDraws))
	}

	// Change clip — should prevent merge with the previous batch.
	clip := [4]uint32{10, 10, 80, 80}
	rc.clipRect = &clip
	rc.QueueGlyphMask(target, batch2)
	glyphDraws = collectGlyphMaskDraws(rc)
	if len(glyphDraws) != 2 {
		t.Errorf("expected 2 glyph draws (clip change prevents merge), got %d", len(glyphDraws))
	}
}

// collectTextDraws counts drawCmdText entries in pendingDraws.
func collectTextDraws(rc *GPURenderContext) []drawCommand {
	var out []drawCommand
	for i := range rc.pendingDraws {
		if rc.pendingDraws[i].kind == drawCmdText {
			out = append(out, rc.pendingDraws[i])
		}
	}
	return out
}

// collectGlyphMaskDraws counts drawCmdGlyphMaskText entries in pendingDraws.
func collectGlyphMaskDraws(rc *GPURenderContext) []drawCommand {
	var out []drawCommand
	for i := range rc.pendingDraws {
		if rc.pendingDraws[i].kind == drawCmdGlyphMaskText {
			out = append(out, rc.pendingDraws[i])
		}
	}
	return out
}
