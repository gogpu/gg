// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package tilecompute

import (
	"math"
	"testing"
)

// rectLines creates LineSoup segments for a rectangle.
// The rectangle is wound counter-clockwise (consistent winding for non-zero fill).
func rectLines(x0, y0, x1, y1 float32) []LineSoup {
	return []LineSoup{
		{P0: [2]float32{x0, y0}, P1: [2]float32{x1, y0}}, // top
		{P0: [2]float32{x1, y0}, P1: [2]float32{x1, y1}}, // right
		{P0: [2]float32{x1, y1}, P1: [2]float32{x0, y1}}, // bottom
		{P0: [2]float32{x0, y1}, P1: [2]float32{x0, y0}}, // left
	}
}

// TestClipSceneIntegration verifies the full pipeline with clip elements:
// green background rect, then a clip that restricts red to a smaller area.
func TestClipSceneIntegration(t *testing.T) {
	const size = 64
	rast := NewRasterizer(size, size)

	elements := []SceneElement{
		// Draw green over entire canvas.
		{
			Type:     ElementDraw,
			Lines:    rectLines(0, 0, size, size),
			Color:    [4]uint8{0, 255, 0, 255},
			FillRule: FillRuleNonZero,
		},
		// BeginClip with a small rect in the center.
		{
			Type:      ElementBeginClip,
			Lines:     rectLines(16, 16, 48, 48),
			BlendMode: 0x8003, // CLIP_BLEND_MODE
			Alpha:     1.0,
		},
		// Draw red over entire canvas (but clipped to center).
		{
			Type:     ElementDraw,
			Lines:    rectLines(0, 0, size, size),
			Color:    [4]uint8{255, 0, 0, 255},
			FillRule: FillRuleNonZero,
		},
		// EndClip.
		{Type: ElementEndClip},
	}

	img := rast.RasterizeSceneDefPTCL([4]uint8{255, 255, 255, 255}, elements)

	// Check pixel inside clip region (32,32) — should be red.
	insideC := img.RGBAAt(32, 32)
	t.Logf("Inside clip (32,32): R=%d G=%d B=%d A=%d", insideC.R, insideC.G, insideC.B, insideC.A)
	if insideC.R < 200 {
		t.Errorf("Inside clip: red too low: got R=%d, want >200", insideC.R)
	}
	if insideC.G > 50 {
		t.Errorf("Inside clip: green too high: got G=%d, want <50", insideC.G)
	}

	// Check pixel outside clip region (4,4) — should be green.
	outsideC := img.RGBAAt(4, 4)
	t.Logf("Outside clip (4,4): R=%d G=%d B=%d A=%d", outsideC.R, outsideC.G, outsideC.B, outsideC.A)
	if outsideC.G < 200 {
		t.Errorf("Outside clip: green too low: got G=%d, want >200", outsideC.G)
	}
	if outsideC.R > 50 {
		t.Errorf("Outside clip: red too high: got R=%d, want <50", outsideC.R)
	}
}

// TestClipSceneNestedClip verifies nested clip layers.
func TestClipSceneNestedClip(t *testing.T) {
	const size = 64
	rast := NewRasterizer(size, size)

	elements := []SceneElement{
		// Draw blue background.
		{
			Type:     ElementDraw,
			Lines:    rectLines(0, 0, size, size),
			Color:    [4]uint8{0, 0, 255, 255},
			FillRule: FillRuleNonZero,
		},
		// Outer clip.
		{
			Type:      ElementBeginClip,
			Lines:     rectLines(8, 8, 56, 56),
			BlendMode: 0x8003,
			Alpha:     1.0,
		},
		// Draw green.
		{
			Type:     ElementDraw,
			Lines:    rectLines(0, 0, size, size),
			Color:    [4]uint8{0, 255, 0, 255},
			FillRule: FillRuleNonZero,
		},
		// Inner clip.
		{
			Type:      ElementBeginClip,
			Lines:     rectLines(20, 20, 44, 44),
			BlendMode: 0x8003,
			Alpha:     1.0,
		},
		// Draw red.
		{
			Type:     ElementDraw,
			Lines:    rectLines(0, 0, size, size),
			Color:    [4]uint8{255, 0, 0, 255},
			FillRule: FillRuleNonZero,
		},
		// End inner clip.
		{Type: ElementEndClip},
		// End outer clip.
		{Type: ElementEndClip},
	}

	img := rast.RasterizeSceneDefPTCL([4]uint8{255, 255, 255, 255}, elements)

	// Center (32,32) — inside both clips → red.
	center := img.RGBAAt(32, 32)
	t.Logf("Center (32,32): R=%d G=%d B=%d A=%d", center.R, center.G, center.B, center.A)
	if center.R < 200 {
		t.Errorf("Center: red too low: R=%d", center.R)
	}

	// Between clips (12,12) — inside outer clip only → green.
	between := img.RGBAAt(12, 12)
	t.Logf("Between clips (12,12): R=%d G=%d B=%d A=%d", between.R, between.G, between.B, between.A)
	if between.G < 200 {
		t.Errorf("Between clips: green too low: G=%d", between.G)
	}
	if between.R > 50 {
		t.Errorf("Between clips: red too high: R=%d", between.R)
	}

	// Outside all clips (2,2) — blue background.
	outside := img.RGBAAt(2, 2)
	t.Logf("Outside clips (2,2): R=%d G=%d B=%d A=%d", outside.R, outside.G, outside.B, outside.A)
	if outside.B < 200 {
		t.Errorf("Outside: blue too low: B=%d", outside.B)
	}
}

// TestClipSceneAlpha verifies that BeginClip alpha modulates content.
func TestClipSceneAlpha(t *testing.T) {
	const size = 32
	rast := NewRasterizer(size, size)

	elements := []SceneElement{
		// Clip with 50% alpha.
		{
			Type:      ElementBeginClip,
			Lines:     rectLines(0, 0, size, size),
			BlendMode: 0x8003,
			Alpha:     0.5,
		},
		// Draw opaque red inside clip.
		{
			Type:     ElementDraw,
			Lines:    rectLines(0, 0, size, size),
			Color:    [4]uint8{255, 0, 0, 255},
			FillRule: FillRuleNonZero,
		},
		{Type: ElementEndClip},
	}

	img := rast.RasterizeSceneDefPTCL([4]uint8{255, 255, 255, 255}, elements)

	// Center should be red at ~50% over white background.
	// fg = red * 0.5 = (0.5, 0, 0, 0.5), result = white*(1-0.5) + (0.5,0,0,0.5) = (1.0, 0.5, 0.5, 1.0)
	// In straight alpha uint8: R=255, G=128, B=128, A=255
	px := img.RGBAAt(16, 16)
	t.Logf("Alpha clip (16,16): R=%d G=%d B=%d A=%d", px.R, px.G, px.B, px.A)

	if px.A < 240 {
		t.Errorf("Alpha too low: A=%d, want ~255", px.A)
	}
	if px.R < 200 {
		t.Errorf("Red too low: R=%d, want ~255", px.R)
	}
	// Green and blue should be around 128 (50% blend with white).
	if math.Abs(float64(px.G)-128) > 30 {
		t.Errorf("Green not ~128: G=%d", px.G)
	}
	if math.Abs(float64(px.B)-128) > 30 {
		t.Errorf("Blue not ~128: B=%d", px.B)
	}
}

// TestClipLeafScan verifies that clipLeafScan correctly fixes up draw monoids.
func TestClipLeafScan(t *testing.T) {
	// Simulate: [0]=BeginClip(path=2), [1]=EndClip(^3)
	clipInps := []ClipInp{
		{Ix: 1, PathIx: 2},  // BeginClip at draw idx 1, path 2
		{Ix: 3, PathIx: ^3}, // EndClip at draw idx 3
	}

	drawMonoids := make([]DrawMonoid, 5)
	drawMonoids[1] = DrawMonoid{PathIx: 2, SceneOffset: 42}

	clipLeafScan(clipInps, drawMonoids)

	// After fixup, draw monoid at index 3 should have BeginClip's path_ix and scene_offset.
	if drawMonoids[3].PathIx != 2 {
		t.Errorf("EndClip PathIx = %d, want 2", drawMonoids[3].PathIx)
	}
	if drawMonoids[3].SceneOffset != 42 {
		t.Errorf("EndClip SceneOffset = %d, want 42", drawMonoids[3].SceneOffset)
	}
}

// TestEncodeSceneDefBasic verifies EncodeSceneDef encodes clip elements correctly.
func TestEncodeSceneDefBasic(t *testing.T) {
	elements := []SceneElement{
		{
			Type:     ElementDraw,
			Lines:    rectLines(0, 0, 16, 16),
			Color:    [4]uint8{255, 0, 0, 255},
			FillRule: FillRuleNonZero,
		},
		{
			Type:      ElementBeginClip,
			Lines:     rectLines(4, 4, 12, 12),
			BlendMode: 0x8003,
			Alpha:     1.0,
		},
		{Type: ElementEndClip},
	}

	enc := EncodeSceneDef(elements)

	// 3 draw objects (Draw, BeginClip, EndClip).
	if enc.NumDrawObjects != 3 {
		t.Errorf("NumDrawObjects = %d, want 3", enc.NumDrawObjects)
	}

	// 3 paths (Draw path, BeginClip path, EndClip dummy path).
	if enc.NumPaths != 3 {
		t.Errorf("NumPaths = %d, want 3", enc.NumPaths)
	}

	// 2 clips (BeginClip + EndClip).
	if enc.NumClips != 2 {
		t.Errorf("NumClips = %d, want 2", enc.NumClips)
	}

	// Draw tags should be [DrawTagColor, DrawTagBeginClip, DrawTagEndClip].
	if len(enc.DrawTags) != 3 {
		t.Fatalf("DrawTags len = %d, want 3", len(enc.DrawTags))
	}
	if enc.DrawTags[0] != DrawTagColor {
		t.Errorf("DrawTags[0] = 0x%x, want 0x%x (Color)", enc.DrawTags[0], DrawTagColor)
	}
	if enc.DrawTags[1] != DrawTagBeginClip {
		t.Errorf("DrawTags[1] = 0x%x, want 0x%x (BeginClip)", enc.DrawTags[1], DrawTagBeginClip)
	}
	if enc.DrawTags[2] != DrawTagEndClip {
		t.Errorf("DrawTags[2] = 0x%x, want 0x%x (EndClip)", enc.DrawTags[2], DrawTagEndClip)
	}

	// Draw data: Color(1 u32) + BeginClip(2 u32: blend + alpha) + EndClip(0 u32) = 3 u32.
	if len(enc.DrawData) != 3 {
		t.Errorf("DrawData len = %d, want 3", len(enc.DrawData))
	}

	// Verify BeginClip draw data.
	if enc.DrawData[1] != 0x8003 {
		t.Errorf("BeginClip blend = 0x%x, want 0x8003", enc.DrawData[1])
	}
	if math.Float32frombits(enc.DrawData[2]) != 1.0 {
		t.Errorf("BeginClip alpha = %f, want 1.0", math.Float32frombits(enc.DrawData[2]))
	}
}

// TestClipSceneDefPTCLMatchesPathDef verifies that a scene without clips
// produces the same result whether using PathDef or SceneElement.
func TestClipSceneDefPTCLMatchesPathDef(t *testing.T) {
	const size = 32
	rast := NewRasterizer(size, size)
	bg := [4]uint8{255, 255, 255, 255}

	// PathDef version.
	paths := []PathDef{
		{Lines: rectLines(0, 0, size, size), Color: [4]uint8{255, 0, 0, 255}, FillRule: FillRuleNonZero},
		{Lines: rectLines(8, 8, 24, 24), Color: [4]uint8{0, 0, 255, 128}, FillRule: FillRuleNonZero},
	}
	imgPD := rast.RasterizeScenePTCL(bg, paths)

	// SceneElement version (same, no clips).
	elements := []SceneElement{
		{Type: ElementDraw, Lines: rectLines(0, 0, size, size), Color: [4]uint8{255, 0, 0, 255}, FillRule: FillRuleNonZero},
		{Type: ElementDraw, Lines: rectLines(8, 8, 24, 24), Color: [4]uint8{0, 0, 255, 128}, FillRule: FillRuleNonZero},
	}
	imgSE := rast.RasterizeSceneDefPTCL(bg, elements)

	// Compare every pixel.
	maxDiff := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			pd := imgPD.RGBAAt(x, y)
			se := imgSE.RGBAAt(x, y)
			for _, d := range []int{
				abs(int(pd.R) - int(se.R)),
				abs(int(pd.G) - int(se.G)),
				abs(int(pd.B) - int(se.B)),
				abs(int(pd.A) - int(se.A)),
			} {
				if d > maxDiff {
					maxDiff = d
				}
			}
		}
	}

	t.Logf("Max pixel diff between PathDef and SceneElement: %d", maxDiff)
	if maxDiff > 1 {
		t.Errorf("PathDef vs SceneElement mismatch: max diff = %d (want <=1)", maxDiff)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
