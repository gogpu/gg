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

// --- Test helpers for unified draw queue ---

// collectGlyphMaskBatches extracts GlyphMaskBatch entries from pendingDraws.
func collectGlyphMaskBatches(rc *GPURenderContext) []GlyphMaskBatch {
	var out []GlyphMaskBatch
	for i := range rc.pendingDraws {
		if rc.pendingDraws[i].kind == drawCmdGlyphMaskText {
			out = append(out, rc.pendingDraws[i].textBatch.(GlyphMaskBatch))
		}
	}
	return out
}

// collectTextBatches extracts TextBatch entries from pendingDraws.
func collectTextBatches(rc *GPURenderContext) []TextBatch {
	var out []TextBatch
	for i := range rc.pendingDraws {
		if rc.pendingDraws[i].kind == drawCmdText {
			out = append(out, rc.pendingDraws[i].textBatch.(TextBatch))
		}
	}
	return out
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

	batches := collectGlyphMaskBatches(rc)
	if len(batches) != 1 {
		t.Fatalf("expected 1 coalesced batch, got %d", len(batches))
	}
	if len(batches[0].Quads) != 3 {
		t.Errorf("expected 3 quads in coalesced batch, got %d", len(batches[0].Quads))
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

	batches := collectGlyphMaskBatches(rc)
	if len(batches) != 2 {
		t.Fatalf("expected 2 separate batches, got %d", len(batches))
	}
}

func TestQueueGlyphMask_MixedSequence(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	white := [4]float32{1, 1, 1, 1}
	red := [4]float32{1, 0, 0, 1}

	// same, same, different, same, same -> should produce 3 batches
	input := []GlyphMaskBatch{
		{Transform: gg.Identity(), Color: white, AtlasPageIndex: 0, Quads: []GlyphMaskQuad{{X0: 0}}},
		{Transform: gg.Identity(), Color: white, AtlasPageIndex: 0, Quads: []GlyphMaskQuad{{X0: 10}}},
		{Transform: gg.Identity(), Color: red, AtlasPageIndex: 0, Quads: []GlyphMaskQuad{{X0: 20}}},
		{Transform: gg.Identity(), Color: white, AtlasPageIndex: 0, Quads: []GlyphMaskQuad{{X0: 30}}},
		{Transform: gg.Identity(), Color: white, AtlasPageIndex: 0, Quads: []GlyphMaskQuad{{X0: 40}}},
	}

	for _, b := range input {
		rc.QueueGlyphMask(target, b)
	}

	batches := collectGlyphMaskBatches(rc)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches (2 merged, 1 different, 2 merged), got %d", len(batches))
	}
	if len(batches[0].Quads) != 2 {
		t.Errorf("expected first batch to have 2 quads, got %d", len(batches[0].Quads))
	}
	if len(batches[1].Quads) != 1 {
		t.Errorf("expected second batch to have 1 quad, got %d", len(batches[1].Quads))
	}
	if len(batches[2].Quads) != 2 {
		t.Errorf("expected third batch to have 2 quads, got %d", len(batches[2].Quads))
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

	batches := collectTextBatches(rc)
	if len(batches) != 1 {
		t.Fatalf("expected 1 coalesced batch, got %d", len(batches))
	}
	if len(batches[0].Quads) != 3 {
		t.Errorf("expected 3 quads in coalesced batch, got %d", len(batches[0].Quads))
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

	batches := collectTextBatches(rc)
	if len(batches) != 2 {
		t.Fatalf("expected 2 separate batches, got %d", len(batches))
	}
}

func TestQueueText_MixedSequence(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	white := gg.RGBA{R: 1, G: 1, B: 1, A: 1}
	red := gg.RGBA{R: 1, G: 0, B: 0, A: 1}

	// same, same, different, same, same -> 3 batches
	input := []TextBatch{
		{Transform: gg.Identity(), Color: white, AtlasIndex: 0, PxRange: 4, AtlasSize: 1024, Quads: []TextQuad{{X0: 0}}},
		{Transform: gg.Identity(), Color: white, AtlasIndex: 0, PxRange: 4, AtlasSize: 1024, Quads: []TextQuad{{X0: 10}}},
		{Transform: gg.Identity(), Color: red, AtlasIndex: 0, PxRange: 4, AtlasSize: 1024, Quads: []TextQuad{{X0: 20}}},
		{Transform: gg.Identity(), Color: white, AtlasIndex: 0, PxRange: 4, AtlasSize: 1024, Quads: []TextQuad{{X0: 30}}},
		{Transform: gg.Identity(), Color: white, AtlasIndex: 0, PxRange: 4, AtlasSize: 1024, Quads: []TextQuad{{X0: 40}}},
	}

	for _, b := range input {
		rc.QueueText(target, b)
	}

	batches := collectTextBatches(rc)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches (2 merged, 1 different, 2 merged), got %d", len(batches))
	}
	if len(batches[0].Quads) != 2 {
		t.Errorf("expected first batch to have 2 quads, got %d", len(batches[0].Quads))
	}
	if len(batches[1].Quads) != 1 {
		t.Errorf("expected second batch to have 1 quad, got %d", len(batches[1].Quads))
	}
	if len(batches[2].Quads) != 2 {
		t.Errorf("expected third batch to have 2 quads, got %d", len(batches[2].Quads))
	}
}

// --- Scissor-boundary glyph batch isolation tests ---

// TestQueueGlyphMask_ScissorBoundary_SplitsBatch verifies that same-style glyph
// runs queued across clip boundaries are NOT merged into a single batch.
//
// After ADR-051 Phase 2 Step 3, clip state is stored per-draw in drawCommand.
// Clip changes between QueueGlyphMask calls produce drawCommands with different
// clip fields, which prevents coalescing. buildScissorGroupsFromDraws then
// creates separate ScissorGroups for each clip region.
func TestQueueGlyphMask_ScissorBoundary_SplitsBatch(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	white := [4]float32{1, 1, 1, 1}
	quad := func(x float32) GlyphMaskBatch {
		return GlyphMaskBatch{
			Transform:      gg.Identity(),
			Color:          white,
			IsLCD:          false,
			AtlasPageIndex: 0,
			Quads:          []GlyphMaskQuad{{X0: x, Y0: 0, X1: x + 10, Y1: 10}},
		}
	}

	// Simulate three sibling elements each clipped to their own rect.
	clip1 := [4]uint32{0, 0, 100, 30}
	clip2 := [4]uint32{0, 30, 100, 30}
	clip3 := [4]uint32{0, 60, 100, 30}

	// Button 1: set clip, queue text.
	rc.clipRect = &clip1
	rc.QueueGlyphMask(target, quad(0))

	// Button 2: change clip, queue text with identical style.
	c2 := clip2
	rc.clipRect = &c2
	rc.QueueGlyphMask(target, quad(10))

	// Button 3: change clip again, queue text with identical style.
	c3 := clip3
	rc.clipRect = &c3
	rc.QueueGlyphMask(target, quad(20))

	batches := collectGlyphMaskBatches(rc)
	if len(batches) != 3 {
		t.Fatalf("expected 3 separate glyph batches (one per clip region), got %d — "+
			"same-style runs across clip boundaries must not be merged",
			len(batches))
	}
	for i, b := range batches {
		if len(b.Quads) != 1 {
			t.Errorf("batch[%d]: expected 1 quad, got %d", i, len(b.Quads))
		}
	}
}

// TestQueueGlyphMask_ScissorBoundary_PreservesIntraGroupMerging verifies that
// within a single clip region, same-style consecutive runs still merge.
// This ensures the per-draw clip approach does not break the efficiency of
// coalescing runs that legitimately share a clip region.
func TestQueueGlyphMask_ScissorBoundary_PreservesIntraGroupMerging(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	white := [4]float32{1, 1, 1, 1}
	quad := func(x float32) GlyphMaskBatch {
		return GlyphMaskBatch{
			Transform:      gg.Identity(),
			Color:          white,
			IsLCD:          false,
			AtlasPageIndex: 0,
			Quads:          []GlyphMaskQuad{{X0: x}},
		}
	}

	clip1 := [4]uint32{0, 0, 100, 50}
	clip2 := [4]uint32{0, 50, 100, 50}

	// Region 1: two runs that should merge.
	rc.clipRect = &clip1
	rc.QueueGlyphMask(target, quad(0))
	rc.QueueGlyphMask(target, quad(10)) // same clip + same style -> merges

	// Region 2: two more runs that should merge with each other but not region 1.
	c2 := clip2
	rc.clipRect = &c2
	rc.QueueGlyphMask(target, quad(20))
	rc.QueueGlyphMask(target, quad(30)) // same clip + same style -> merges

	batches := collectGlyphMaskBatches(rc)
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches (one per clip region, each with 2 quads merged), got %d",
			len(batches))
	}
	if len(batches[0].Quads) != 2 {
		t.Errorf("region 1 batch: expected 2 merged quads, got %d",
			len(batches[0].Quads))
	}
	if len(batches[1].Quads) != 2 {
		t.Errorf("region 2 batch: expected 2 merged quads, got %d",
			len(batches[1].Quads))
	}
}

// --- Scissor-boundary text batch isolation tests ---

// TestQueueText_ScissorBoundary_SplitsBatch verifies that same-style text
// runs queued across clip boundaries are NOT merged into a single batch.
//
// After ADR-051 Phase 2 Step 3, clip state is stored per-draw in drawCommand.
// Clip changes between QueueText calls produce drawCommands with different
// clip fields, which prevents coalescing.
func TestQueueText_ScissorBoundary_SplitsBatch(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	quad := func(x float32) TextBatch {
		return TextBatch{
			Transform:  gg.Identity(),
			Color:      gg.RGBA{R: 1, G: 1, B: 1, A: 1},
			AtlasIndex: 0,
			PxRange:    4.0,
			AtlasSize:  1024,
			Quads:      []TextQuad{{X0: x, Y0: 0, X1: x + 10, Y1: 10}},
		}
	}

	// Simulate three sibling elements each clipped to their own rect.
	clip1 := [4]uint32{0, 0, 100, 30}
	clip2 := [4]uint32{0, 30, 100, 30}
	clip3 := [4]uint32{0, 60, 100, 30}

	// Button 1: set clip, queue text.
	rc.clipRect = &clip1
	rc.QueueText(target, quad(0))

	// Button 2: change clip, queue text with identical style.
	c2 := clip2
	rc.clipRect = &c2
	rc.QueueText(target, quad(10))

	// Button 3: change clip again, queue text with identical style.
	c3 := clip3
	rc.clipRect = &c3
	rc.QueueText(target, quad(20))

	batches := collectTextBatches(rc)
	if len(batches) != 3 {
		t.Fatalf("expected 3 separate text batches (one per clip region), got %d — "+
			"same-style runs across clip boundaries must not be merged",
			len(batches))
	}
	for i, b := range batches {
		if len(b.Quads) != 1 {
			t.Errorf("batch[%d]: expected 1 quad, got %d", i, len(b.Quads))
		}
	}
}

// TestQueueText_ScissorBoundary_PreservesIntraGroupMerging verifies that
// within a single clip region, same-style consecutive text runs still merge.
func TestQueueText_ScissorBoundary_PreservesIntraGroupMerging(t *testing.T) {
	rc := &GPURenderContext{}
	target := makeCoalesceTestTarget()

	quad := func(x float32) TextBatch {
		return TextBatch{
			Transform:  gg.Identity(),
			Color:      gg.RGBA{R: 1, G: 1, B: 1, A: 1},
			AtlasIndex: 0,
			PxRange:    4.0,
			AtlasSize:  1024,
			Quads:      []TextQuad{{X0: x}},
		}
	}

	clip1 := [4]uint32{0, 0, 100, 50}
	clip2 := [4]uint32{0, 50, 100, 50}

	// Region 1: two runs that should merge.
	rc.clipRect = &clip1
	rc.QueueText(target, quad(0))
	rc.QueueText(target, quad(10)) // same clip + same style -> merges

	// Region 2: two more runs that should merge with each other but not region 1.
	c2 := clip2
	rc.clipRect = &c2
	rc.QueueText(target, quad(20))
	rc.QueueText(target, quad(30)) // same clip + same style -> merges

	batches := collectTextBatches(rc)
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches (one per clip region, each with 2 quads merged), got %d",
			len(batches))
	}
	if len(batches[0].Quads) != 2 {
		t.Errorf("region 1 batch: expected 2 merged quads, got %d",
			len(batches[0].Quads))
	}
	if len(batches[1].Quads) != 2 {
		t.Errorf("region 2 batch: expected 2 merged quads, got %d",
			len(batches[1].Quads))
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
