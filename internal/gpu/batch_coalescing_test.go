//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
)

// --- GlyphMaskBatch.CanMerge tests ---

func TestGlyphMaskBatch_CanMerge_SameProperties(t *testing.T) {
	a := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
		Quads:          []GlyphMaskQuad{{X0: 0, Y0: 0, X1: 10, Y1: 10}},
	}
	b := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
		Quads:          []GlyphMaskQuad{{X0: 20, Y0: 0, X1: 30, Y1: 10}},
	}
	if !a.CanMerge(b) {
		t.Error("expected CanMerge=true for identical properties")
	}
}

func TestGlyphMaskBatch_CanMerge_DifferentColor(t *testing.T) {
	a := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 0, 0, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
	}
	b := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{0, 1, 0, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
	}
	if a.CanMerge(b) {
		t.Error("expected CanMerge=false for different colors")
	}
}

func TestGlyphMaskBatch_CanMerge_DifferentTransform(t *testing.T) {
	a := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
	}
	b := GlyphMaskBatch{
		Transform:      gg.Matrix{A: 2, B: 0, C: 0, D: 0, E: 2, F: 0},
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
	}
	if a.CanMerge(b) {
		t.Error("expected CanMerge=false for different transforms")
	}
}

func TestGlyphMaskBatch_CanMerge_DifferentLCD(t *testing.T) {
	a := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
	}
	b := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          true,
		AtlasPageIndex: 0,
	}
	if a.CanMerge(b) {
		t.Error("expected CanMerge=false for different IsLCD")
	}
}

func TestGlyphMaskBatch_CanMerge_DifferentAtlasPage(t *testing.T) {
	a := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
	}
	b := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          false,
		AtlasPageIndex: 1,
	}
	if a.CanMerge(b) {
		t.Error("expected CanMerge=false for different atlas page")
	}
}

// --- TextBatch.CanMerge tests ---

func TestTextBatch_CanMerge_SameProperties(t *testing.T) {
	a := TextBatch{
		Transform:  gg.Identity(),
		Color:      gg.RGBA{R: 1, G: 1, B: 1, A: 1},
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  1024,
		Quads:      []TextQuad{{X0: 0, Y0: 0, X1: 10, Y1: 10}},
	}
	b := TextBatch{
		Transform:  gg.Identity(),
		Color:      gg.RGBA{R: 1, G: 1, B: 1, A: 1},
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  1024,
		Quads:      []TextQuad{{X0: 20, Y0: 0, X1: 30, Y1: 10}},
	}
	if !a.CanMerge(b) {
		t.Error("expected CanMerge=true for identical properties")
	}
}

func TestTextBatch_CanMerge_DifferentColor(t *testing.T) {
	a := TextBatch{
		Transform:  gg.Identity(),
		Color:      gg.RGBA{R: 1, G: 0, B: 0, A: 1},
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  1024,
	}
	b := TextBatch{
		Transform:  gg.Identity(),
		Color:      gg.RGBA{R: 0, G: 1, B: 0, A: 1},
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  1024,
	}
	if a.CanMerge(b) {
		t.Error("expected CanMerge=false for different colors")
	}
}

func TestTextBatch_CanMerge_DifferentAtlas(t *testing.T) {
	a := TextBatch{
		Transform:  gg.Identity(),
		Color:      gg.RGBA{R: 1, G: 1, B: 1, A: 1},
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  1024,
	}
	b := TextBatch{
		Transform:  gg.Identity(),
		Color:      gg.RGBA{R: 1, G: 1, B: 1, A: 1},
		AtlasIndex: 1,
		PxRange:    4.0,
		AtlasSize:  1024,
	}
	if a.CanMerge(b) {
		t.Error("expected CanMerge=false for different atlas index")
	}
}

func TestTextBatch_CanMerge_DifferentTransform(t *testing.T) {
	a := TextBatch{
		Transform:  gg.Identity(),
		Color:      gg.RGBA{R: 1, G: 1, B: 1, A: 1},
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  1024,
	}
	b := TextBatch{
		Transform:  gg.Matrix{A: 1, B: 0, C: 100, D: 0, E: 1, F: 50},
		Color:      gg.RGBA{R: 1, G: 1, B: 1, A: 1},
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  1024,
	}
	if a.CanMerge(b) {
		t.Error("expected CanMerge=false for different transforms")
	}
}

// --- QueueGlyphMask coalescing tests ---

func TestQueueGlyphMask_Coalescing_SameStyle(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	batch1 := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
		Quads:          []GlyphMaskQuad{{X0: 0, Y0: 0, X1: 10, Y1: 10}},
	}
	batch2 := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
		Quads:          []GlyphMaskQuad{{X0: 20, Y0: 0, X1: 30, Y1: 10}},
	}
	batch3 := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 1, 1, 1},
		IsLCD:          false,
		AtlasPageIndex: 0,
		Quads:          []GlyphMaskQuad{{X0: 40, Y0: 0, X1: 50, Y1: 10}},
	}

	rc.QueueGlyphMask(target, batch1)
	rc.QueueGlyphMask(target, batch2)
	rc.QueueGlyphMask(target, batch3)

	if len(rc.pendingGlyphMaskBatches) != 1 {
		t.Fatalf("expected 1 coalesced batch, got %d", len(rc.pendingGlyphMaskBatches))
	}
	if len(rc.pendingGlyphMaskBatches[0].Quads) != 3 {
		t.Errorf("expected 3 quads in coalesced batch, got %d", len(rc.pendingGlyphMaskBatches[0].Quads))
	}
}

func TestQueueGlyphMask_NoCoalescing_DifferentColor(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	batch1 := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{1, 0, 0, 1},
		AtlasPageIndex: 0,
		Quads:          []GlyphMaskQuad{{X0: 0, Y0: 0, X1: 10, Y1: 10}},
	}
	batch2 := GlyphMaskBatch{
		Transform:      gg.Identity(),
		Color:          [4]float32{0, 1, 0, 1},
		AtlasPageIndex: 0,
		Quads:          []GlyphMaskQuad{{X0: 20, Y0: 0, X1: 30, Y1: 10}},
	}

	rc.QueueGlyphMask(target, batch1)
	rc.QueueGlyphMask(target, batch2)

	if len(rc.pendingGlyphMaskBatches) != 2 {
		t.Fatalf("expected 2 separate batches, got %d", len(rc.pendingGlyphMaskBatches))
	}
}

func TestQueueGlyphMask_MixedSequence(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	white := [4]float32{1, 1, 1, 1}
	red := [4]float32{1, 0, 0, 1}

	// same, same, different, same, same → should produce 3 batches
	batches := []GlyphMaskBatch{
		{Transform: gg.Identity(), Color: white, AtlasPageIndex: 0, Quads: []GlyphMaskQuad{{X0: 0}}},
		{Transform: gg.Identity(), Color: white, AtlasPageIndex: 0, Quads: []GlyphMaskQuad{{X0: 10}}},
		{Transform: gg.Identity(), Color: red, AtlasPageIndex: 0, Quads: []GlyphMaskQuad{{X0: 20}}},
		{Transform: gg.Identity(), Color: white, AtlasPageIndex: 0, Quads: []GlyphMaskQuad{{X0: 30}}},
		{Transform: gg.Identity(), Color: white, AtlasPageIndex: 0, Quads: []GlyphMaskQuad{{X0: 40}}},
	}

	for _, b := range batches {
		rc.QueueGlyphMask(target, b)
	}

	if len(rc.pendingGlyphMaskBatches) != 3 {
		t.Fatalf("expected 3 batches (2 merged, 1 different, 2 merged), got %d", len(rc.pendingGlyphMaskBatches))
	}
	if len(rc.pendingGlyphMaskBatches[0].Quads) != 2 {
		t.Errorf("expected first batch to have 2 quads, got %d", len(rc.pendingGlyphMaskBatches[0].Quads))
	}
	if len(rc.pendingGlyphMaskBatches[1].Quads) != 1 {
		t.Errorf("expected second batch to have 1 quad, got %d", len(rc.pendingGlyphMaskBatches[1].Quads))
	}
	if len(rc.pendingGlyphMaskBatches[2].Quads) != 2 {
		t.Errorf("expected third batch to have 2 quads, got %d", len(rc.pendingGlyphMaskBatches[2].Quads))
	}
}

// --- QueueText coalescing tests ---

func TestQueueText_Coalescing_SameStyle(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	style := TextBatch{
		Transform:  gg.Identity(),
		Color:      gg.RGBA{R: 1, G: 1, B: 1, A: 1},
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  1024,
	}

	batch1 := style
	batch1.Quads = []TextQuad{{X0: 0, Y0: 0, X1: 10, Y1: 10}}
	batch2 := style
	batch2.Quads = []TextQuad{{X0: 20, Y0: 0, X1: 30, Y1: 10}}
	batch3 := style
	batch3.Quads = []TextQuad{{X0: 40, Y0: 0, X1: 50, Y1: 10}}

	rc.QueueText(target, batch1)
	rc.QueueText(target, batch2)
	rc.QueueText(target, batch3)

	if len(rc.pendingTextBatches) != 1 {
		t.Fatalf("expected 1 coalesced batch, got %d", len(rc.pendingTextBatches))
	}
	if len(rc.pendingTextBatches[0].Quads) != 3 {
		t.Errorf("expected 3 quads in coalesced batch, got %d", len(rc.pendingTextBatches[0].Quads))
	}
}

func TestQueueText_NoCoalescing_DifferentColor(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	batch1 := TextBatch{
		Transform:  gg.Identity(),
		Color:      gg.RGBA{R: 1, G: 0, B: 0, A: 1},
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  1024,
		Quads:      []TextQuad{{X0: 0, Y0: 0, X1: 10, Y1: 10}},
	}
	batch2 := TextBatch{
		Transform:  gg.Identity(),
		Color:      gg.RGBA{R: 0, G: 1, B: 0, A: 1},
		AtlasIndex: 0,
		PxRange:    4.0,
		AtlasSize:  1024,
		Quads:      []TextQuad{{X0: 20, Y0: 0, X1: 30, Y1: 10}},
	}

	rc.QueueText(target, batch1)
	rc.QueueText(target, batch2)

	if len(rc.pendingTextBatches) != 2 {
		t.Fatalf("expected 2 separate batches, got %d", len(rc.pendingTextBatches))
	}
}

func TestQueueText_MixedSequence(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	white := gg.RGBA{R: 1, G: 1, B: 1, A: 1}
	red := gg.RGBA{R: 1, G: 0, B: 0, A: 1}

	// same, same, different, same, same → 3 batches
	batches := []TextBatch{
		{Transform: gg.Identity(), Color: white, AtlasIndex: 0, PxRange: 4, AtlasSize: 1024, Quads: []TextQuad{{X0: 0}}},
		{Transform: gg.Identity(), Color: white, AtlasIndex: 0, PxRange: 4, AtlasSize: 1024, Quads: []TextQuad{{X0: 10}}},
		{Transform: gg.Identity(), Color: red, AtlasIndex: 0, PxRange: 4, AtlasSize: 1024, Quads: []TextQuad{{X0: 20}}},
		{Transform: gg.Identity(), Color: white, AtlasIndex: 0, PxRange: 4, AtlasSize: 1024, Quads: []TextQuad{{X0: 30}}},
		{Transform: gg.Identity(), Color: white, AtlasIndex: 0, PxRange: 4, AtlasSize: 1024, Quads: []TextQuad{{X0: 40}}},
	}

	for _, b := range batches {
		rc.QueueText(target, b)
	}

	if len(rc.pendingTextBatches) != 3 {
		t.Fatalf("expected 3 batches (2 merged, 1 different, 2 merged), got %d", len(rc.pendingTextBatches))
	}
	if len(rc.pendingTextBatches[0].Quads) != 2 {
		t.Errorf("expected first batch to have 2 quads, got %d", len(rc.pendingTextBatches[0].Quads))
	}
	if len(rc.pendingTextBatches[1].Quads) != 1 {
		t.Errorf("expected second batch to have 1 quad, got %d", len(rc.pendingTextBatches[1].Quads))
	}
	if len(rc.pendingTextBatches[2].Quads) != 2 {
		t.Errorf("expected third batch to have 2 quads, got %d", len(rc.pendingTextBatches[2].Quads))
	}
}

// makeCoalesceTestTarget creates a minimal GPURenderTarget for coalescing tests.
// Uses a non-nil Data slice so sameTarget compares via data pointer identity.
func makeCoalesceTestTarget() gg.GPURenderTarget {
	data := make([]byte, 4) // minimal non-nil data for sameTarget comparison
	return gg.GPURenderTarget{
		Width:  100,
		Height: 100,
		Data:   data,
		Stride: 400,
	}
}
