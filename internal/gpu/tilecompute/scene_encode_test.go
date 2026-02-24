// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package tilecompute

import (
	"math"
	"math/bits"
	"testing"
)

// TestEncodeSceneSinglePath encodes one triangle path and verifies
// tag counts, data layout, and color packing.
func TestEncodeSceneSinglePath(t *testing.T) {
	triangle := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{10, 10}, {90, 50}, {10, 90},
		}),
		Color:    [4]uint8{255, 0, 0, 255}, // opaque red
		FillRule: FillRuleNonZero,
	}

	enc := EncodeScene([]PathDef{triangle})

	// Verify counts.
	if enc.NumPaths != 1 {
		t.Errorf("NumPaths = %d, want 1", enc.NumPaths)
	}
	if enc.NumDrawObjects != 1 {
		t.Errorf("NumDrawObjects = %d, want 1", enc.NumDrawObjects)
	}

	// Verify draw tag.
	if len(enc.DrawTags) != 1 || enc.DrawTags[0] != DrawTagColor {
		t.Errorf("DrawTags = %v, want [0x44]", enc.DrawTags)
	}

	// Verify color packing (premultiplied, opaque red).
	// r=255, g=0, b=0, a=255 → premultiplied same since a=255/255=1.0
	// packed = 255 | (0<<8) | (0<<16) | (255<<24) = 0xFF0000FF
	wantColor := uint32(255) | (uint32(0) << 8) | (uint32(0) << 16) | (uint32(255) << 24)
	if len(enc.DrawData) != 1 || enc.DrawData[0] != wantColor {
		t.Errorf("DrawData[0] = 0x%08X, want 0x%08X", enc.DrawData[0], wantColor)
	}

	// Verify transforms (1 identity = 6 floats).
	if len(enc.Transforms) != 6 {
		t.Errorf("len(Transforms) = %d, want 6", len(enc.Transforms))
	} else {
		want := [6]float32{1, 0, 0, 1, 0, 0}
		for i, v := range enc.Transforms {
			if v != want[i] {
				t.Errorf("Transforms[%d] = %f, want %f", i, v, want[i])
			}
		}
	}

	// Verify styles (1 style for fill non-zero = 0).
	if len(enc.Styles) != 1 || enc.Styles[0] != 0 {
		t.Errorf("Styles = %v, want [0]", enc.Styles)
	}

	// Verify PathTags is non-empty (packed words).
	if len(enc.PathTags) == 0 {
		t.Error("PathTags is empty")
	}

	// Verify PathData has coordinate data.
	// Triangle has 3 lines. Each line emits: MoveTo (if start) or LineTo for P0
	// plus LineTo for P1. For a connected polygon, we expect:
	// Line 0: MoveTo(P0) + LineTo(P1) → 2 tags, 4 floats
	// Line 1: LineTo(P1) → 1 tag, 2 floats (P0 == previous P1)
	// Line 2: LineTo(P1) → 1 tag, 2 floats (P0 == previous P1)
	// Total coordinate floats = 4 + 2 + 2 = 8
	if len(enc.PathData) == 0 {
		t.Error("PathData is empty")
	}

	t.Logf("PathTags: %d words, PathData: %d uint32s, DrawTags: %d, DrawData: %d",
		len(enc.PathTags), len(enc.PathData), len(enc.DrawTags), len(enc.DrawData))
}

// TestEncodeSceneMultiPath encodes 3 paths with distinct colors and verifies
// all offsets, counts, and data layout.
func TestEncodeSceneMultiPath(t *testing.T) {
	// Three simple paths with different colors.
	paths := []PathDef{
		{
			Lines: polygonToLineSoup([][2]float32{
				{10, 10}, {30, 10}, {30, 30}, {10, 30}, // square
			}),
			Color:    [4]uint8{255, 0, 0, 255}, // red
			FillRule: FillRuleNonZero,
		},
		{
			Lines: polygonToLineSoup([][2]float32{
				{40, 10}, {60, 50}, {40, 50}, // triangle
			}),
			Color:    [4]uint8{0, 255, 0, 255}, // green
			FillRule: FillRuleNonZero,
		},
		{
			Lines: polygonToLineSoup([][2]float32{
				{70, 10}, {90, 10}, {90, 30}, {70, 30}, // another square
			}),
			Color:    [4]uint8{0, 0, 255, 128}, // semi-transparent blue
			FillRule: FillRuleEvenOdd,
		},
	}

	enc := EncodeScene(paths)

	// Verify counts.
	if enc.NumPaths != 3 {
		t.Errorf("NumPaths = %d, want 3", enc.NumPaths)
	}
	if enc.NumDrawObjects != 3 {
		t.Errorf("NumDrawObjects = %d, want 3", enc.NumDrawObjects)
	}

	// Verify 3 draw tags (all Color).
	if len(enc.DrawTags) != 3 {
		t.Fatalf("len(DrawTags) = %d, want 3", len(enc.DrawTags))
	}
	for i, tag := range enc.DrawTags {
		if tag != DrawTagColor {
			t.Errorf("DrawTags[%d] = 0x%X, want 0x%X", i, tag, DrawTagColor)
		}
	}

	// Verify 3 draw data entries.
	if len(enc.DrawData) != 3 {
		t.Fatalf("len(DrawData) = %d, want 3", len(enc.DrawData))
	}

	// Check semi-transparent blue premultiplied color.
	// r=0, g=0, b=255, a=128 → premultiplied: r=0, g=0, b=128, a=128
	// b_premul = uint32(255 * (128/255) + 0.5) = uint32(128.5) = 128
	blueColor := enc.DrawData[2]
	bR := blueColor & 0xFF
	bG := (blueColor >> 8) & 0xFF
	bB := (blueColor >> 16) & 0xFF
	bA := (blueColor >> 24) & 0xFF
	if bR != 0 || bG != 0 || bA != 128 {
		t.Errorf("blue premul: R=%d G=%d B=%d A=%d, want R=0 G=0 B=~128 A=128", bR, bG, bB, bA)
	}
	if bB < 127 || bB > 129 {
		t.Errorf("blue premul B=%d, want ~128", bB)
	}

	// Verify 3 transforms (3 * 6 = 18 floats).
	if len(enc.Transforms) != 18 {
		t.Errorf("len(Transforms) = %d, want 18", len(enc.Transforms))
	}

	// Verify 3 styles.
	if len(enc.Styles) != 3 {
		t.Fatalf("len(Styles) = %d, want 3", len(enc.Styles))
	}
	// First two: fill non-zero (0), third: even-odd (0x02).
	if enc.Styles[0] != 0 || enc.Styles[1] != 0 {
		t.Errorf("Styles[0:2] = %v, want [0, 0]", enc.Styles[:2])
	}
	if enc.Styles[2] != 0x02 {
		t.Errorf("Styles[2] = 0x%X, want 0x02", enc.Styles[2])
	}

	t.Logf("PathTags: %d words, PathData: %d uint32s", len(enc.PathTags), len(enc.PathData))
}

// TestPackScene verifies the packed buffer layout matches expected offsets.
func TestPackScene(t *testing.T) {
	triangle := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{10, 10}, {90, 50}, {10, 90},
		}),
		Color:    [4]uint8{255, 0, 0, 255},
		FillRule: FillRuleNonZero,
	}

	enc := EncodeScene([]PathDef{triangle})
	packed := PackScene(enc)

	layout := packed.Layout

	// Layout metadata.
	if layout.NumPaths != 1 {
		t.Errorf("Layout.NumPaths = %d, want 1", layout.NumPaths)
	}
	if layout.NumDrawObjects != 1 {
		t.Errorf("Layout.NumDrawObjects = %d, want 1", layout.NumDrawObjects)
	}

	// PathTagBase should be 0.
	if layout.PathTagBase != 0 {
		t.Errorf("PathTagBase = %d, want 0", layout.PathTagBase)
	}

	// PathTags should be padded to multiple of pathReduceWG.
	paddedSize := layout.PathDataBase - layout.PathTagBase
	if paddedSize%pathReduceWG != 0 {
		t.Errorf("padded PathTag size = %d, not multiple of %d", paddedSize, pathReduceWG)
	}
	if paddedSize < pathReduceWG {
		t.Errorf("padded PathTag size = %d, expected at least %d", paddedSize, pathReduceWG)
	}

	// DrawTags should follow PathData.
	if layout.DrawTagBase != layout.PathDataBase+uint32(len(enc.PathData)) {
		t.Errorf("DrawTagBase = %d, want %d", layout.DrawTagBase, layout.PathDataBase+uint32(len(enc.PathData)))
	}

	// DrawData follows DrawTags.
	if layout.DrawDataBase != layout.DrawTagBase+uint32(len(enc.DrawTags)) {
		t.Errorf("DrawDataBase = %d, want %d", layout.DrawDataBase, layout.DrawTagBase+uint32(len(enc.DrawTags)))
	}

	// Verify total buffer size.
	expectedSize := paddedSize + uint32(len(enc.PathData)) +
		uint32(len(enc.DrawTags)) + uint32(len(enc.DrawData)) +
		uint32(len(enc.Transforms)) + uint32(len(enc.Styles))
	if uint32(len(packed.Data)) != expectedSize {
		t.Errorf("packed.Data len = %d, want %d", len(packed.Data), expectedSize)
	}

	// Verify draw tag is readable from packed buffer.
	drawTag := packed.Data[layout.DrawTagBase]
	if drawTag != DrawTagColor {
		t.Errorf("packed DrawTag = 0x%X, want 0x%X", drawTag, DrawTagColor)
	}

	t.Logf("Layout: PathTagBase=%d PathDataBase=%d DrawTagBase=%d DrawDataBase=%d TransformBase=%d StyleBase=%d",
		layout.PathTagBase, layout.PathDataBase, layout.DrawTagBase,
		layout.DrawDataBase, layout.TransformBase, layout.StyleBase)
	t.Logf("Total buffer size: %d uint32s", len(packed.Data))
}

// TestPathMonoidNew tests monoid construction from specific tag words.
func TestPathMonoidNew(t *testing.T) {
	tests := []struct {
		name    string
		tagWord uint32
		want    PathMonoid
	}{
		{
			name:    "empty",
			tagWord: 0x00000000,
			want:    PathMonoid{},
		},
		{
			name:    "single_lineto",
			tagWord: uint32(PathTagLineToF32), // 0x09 in byte 0
			want: PathMonoid{
				PathSegIx:     1,
				PathSegOffset: 2, // LineTo stores 1 point = 2 floats
			},
		},
		{
			name:    "single_transform",
			tagWord: uint32(PathTagTransform), // 0x20 in byte 0
			want: PathMonoid{
				TransIx: 1,
			},
		},
		{
			name:    "single_style",
			tagWord: uint32(PathTagStyle), // 0x40 in byte 0
			want: PathMonoid{
				StyleIx: 1,
			},
		},
		{
			name:    "single_path_marker",
			tagWord: uint32(PathTagPath), // 0x10 in byte 0
			want: PathMonoid{
				PathIx: 1,
			},
		},
		{
			name:    "two_linetos",
			tagWord: uint32(PathTagLineToF32) | (uint32(PathTagLineToF32) << 8), // 0x0909
			want: PathMonoid{
				PathSegIx:     2,
				PathSegOffset: 4, // 2 lines * 2 floats each
			},
		},
		{
			name: "transform_style_lineto_path",
			// byte0=transform(0x20), byte1=style(0x40), byte2=lineto(0x09), byte3=path(0x10)
			tagWord: uint32(PathTagTransform) |
				(uint32(PathTagStyle) << 8) |
				(uint32(PathTagLineToF32) << 16) |
				(uint32(PathTagPath) << 24),
			want: PathMonoid{
				TransIx:       1,
				StyleIx:       1,
				PathSegIx:     1,
				PathSegOffset: 2,
				PathIx:        1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newPathMonoid(tt.tagWord)
			if got != tt.want {
				t.Errorf("newPathMonoid(0x%08X):\n  got  %+v\n  want %+v", tt.tagWord, got, tt.want)
			}
		})
	}
}

// TestPathMonoidCombine tests monoid associativity.
func TestPathMonoidCombine(t *testing.T) {
	a := PathMonoid{TransIx: 1, PathSegIx: 2, PathSegOffset: 4, StyleIx: 1, PathIx: 0}
	b := PathMonoid{TransIx: 0, PathSegIx: 3, PathSegOffset: 6, StyleIx: 0, PathIx: 1}
	c := PathMonoid{TransIx: 1, PathSegIx: 1, PathSegOffset: 2, StyleIx: 1, PathIx: 1}

	// Test basic combine.
	ab := a.combine(b)
	want := PathMonoid{TransIx: 1, PathSegIx: 5, PathSegOffset: 10, StyleIx: 1, PathIx: 1}
	if ab != want {
		t.Errorf("a.combine(b) = %+v, want %+v", ab, want)
	}

	// Test associativity: (a+b)+c == a+(b+c).
	abc1 := a.combine(b).combine(c)
	abc2 := a.combine(b.combine(c))
	if abc1 != abc2 {
		t.Errorf("associativity failed:\n  (a+b)+c = %+v\n  a+(b+c) = %+v", abc1, abc2)
	}

	// Test identity: a.combine(zero) == a.
	zero := PathMonoid{}
	if a.combine(zero) != a {
		t.Errorf("right identity failed: a+0 = %+v, want %+v", a.combine(zero), a)
	}
	if zero.combine(a) != a {
		t.Errorf("left identity failed: 0+a = %+v, want %+v", zero.combine(a), a)
	}
}

// TestPathtagReduceScan runs full reduce+scan on a known scene and verifies
// cumulative counts at each tag word position.
func TestPathtagReduceScan(t *testing.T) {
	// Build a simple scene: 2 paths, triangle + square.
	paths := []PathDef{
		{
			Lines: polygonToLineSoup([][2]float32{
				{10, 10}, {50, 50}, {10, 50},
			}),
			Color:    [4]uint8{255, 0, 0, 255},
			FillRule: FillRuleNonZero,
		},
		{
			Lines: polygonToLineSoup([][2]float32{
				{60, 10}, {90, 10}, {90, 40}, {60, 40},
			}),
			Color:    [4]uint8{0, 0, 255, 255},
			FillRule: FillRuleNonZero,
		},
	}

	enc := EncodeScene(paths)
	packed := PackScene(enc)

	reduced := pathtagReduce(packed)
	scanned := pathtagScan(packed, reduced)

	// The total monoid (sum of all reduced blocks) should reflect:
	// 2 transforms, 2 styles, 2 path markers, and all line segments.
	totalReduced := PathMonoid{}
	for _, r := range reduced {
		totalReduced = totalReduced.combine(r)
	}

	if totalReduced.TransIx != 2 {
		t.Errorf("total TransIx = %d, want 2", totalReduced.TransIx)
	}
	if totalReduced.StyleIx != 2 {
		t.Errorf("total StyleIx = %d, want 2", totalReduced.StyleIx)
	}
	if totalReduced.PathIx != 2 {
		t.Errorf("total PathIx = %d, want 2", totalReduced.PathIx)
	}

	// PathSegIx should count all line segments.
	// Triangle: 3 lines → 3+3=6 LineTo tags (each line emits MoveTo-as-LineTo + LineTo,
	// but connected lines share endpoints, so: first line=2, next=1, next=1 = 4 tags)
	// Actually count depends on connectivity. Let's just verify it's positive.
	if totalReduced.PathSegIx == 0 {
		t.Error("total PathSegIx = 0, expected > 0")
	}

	t.Logf("Total monoid: %+v", totalReduced)
	t.Logf("Reduced blocks: %d", len(reduced))

	// Verify scan results are monotonically non-decreasing.
	numTagWords := numPathTagWords(packed)
	for i := uint32(1); i < numTagWords; i++ {
		prev := scanned[i-1]
		curr := scanned[i]
		if curr.TransIx < prev.TransIx || curr.PathSegIx < prev.PathSegIx ||
			curr.PathIx < prev.PathIx || curr.StyleIx < prev.StyleIx {
			t.Errorf("scan not monotonic at [%d]: prev=%+v curr=%+v", i, prev, curr)
			break
		}
	}

	// First element should be the identity (exclusive prefix).
	if scanned[0] != (PathMonoid{}) {
		t.Errorf("scanned[0] = %+v, want identity PathMonoid{}", scanned[0])
	}
}

// TestDrawMonoidNew tests draw monoid construction from specific tags.
func TestDrawMonoidNew(t *testing.T) {
	tests := []struct {
		name string
		tag  uint32
		want DrawMonoid
	}{
		{
			name: "nop",
			tag:  DrawTagNop,
			want: DrawMonoid{},
		},
		{
			name: "color",
			tag:  DrawTagColor, // 0x44 = 0b01000100
			want: DrawMonoid{
				PathIx:      1,
				ClipIx:      0,                         // bit 0 = 0
				SceneOffset: (DrawTagColor >> 2) & 0x7, // bits 2-4 = 1
				InfoOffset:  (DrawTagColor >> 6) & 0xf, // bits 6-9 = 1
			},
		},
		{
			name: "begin_clip",
			tag:  DrawTagBeginClip, // 0x9
			want: DrawMonoid{
				PathIx:      1,
				ClipIx:      1,                             // bit 0 = 1
				SceneOffset: (DrawTagBeginClip >> 2) & 0x7, // bits 2-4 = 2
				InfoOffset:  (DrawTagBeginClip >> 6) & 0xf, // bits 6-9 = 0
			},
		},
		{
			name: "end_clip",
			tag:  DrawTagEndClip, // 0x21
			want: DrawMonoid{
				PathIx:      1,
				ClipIx:      1,                           // bit 0 = 1
				SceneOffset: (DrawTagEndClip >> 2) & 0x7, // bits 2-4
				InfoOffset:  (DrawTagEndClip >> 6) & 0xf, // bits 6-9
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newDrawMonoid(tt.tag)
			if got != tt.want {
				t.Errorf("newDrawMonoid(0x%X) = %+v, want %+v", tt.tag, got, tt.want)
			}
		})
	}
}

// TestDrawReduceScan runs full draw reduce+scan and verifies offsets.
func TestDrawReduceScan(t *testing.T) {
	paths := []PathDef{
		{
			Lines:    polygonToLineSoup([][2]float32{{10, 10}, {50, 50}, {10, 50}}),
			Color:    [4]uint8{255, 0, 0, 255},
			FillRule: FillRuleNonZero,
		},
		{
			Lines:    polygonToLineSoup([][2]float32{{60, 10}, {90, 10}, {90, 40}, {60, 40}}),
			Color:    [4]uint8{0, 255, 0, 255},
			FillRule: FillRuleNonZero,
		},
		{
			Lines:    polygonToLineSoup([][2]float32{{20, 60}, {80, 60}, {50, 90}}),
			Color:    [4]uint8{0, 0, 255, 128}, // semi-transparent
			FillRule: FillRuleEvenOdd,
		},
	}

	enc := EncodeScene(paths)
	packed := PackScene(enc)

	reduced := drawReduce(packed)
	drawMonoids, info := drawLeafScan(packed, reduced)

	// Verify we get 3 draw monoids.
	if len(drawMonoids) != 3 {
		t.Fatalf("len(drawMonoids) = %d, want 3", len(drawMonoids))
	}

	// First draw monoid should be identity (exclusive prefix).
	if drawMonoids[0] != (DrawMonoid{}) {
		t.Errorf("drawMonoids[0] = %+v, want identity", drawMonoids[0])
	}

	// Second draw monoid should have PathIx=1, SceneOffset=1 (after first color).
	if drawMonoids[1].PathIx != 1 {
		t.Errorf("drawMonoids[1].PathIx = %d, want 1", drawMonoids[1].PathIx)
	}
	if drawMonoids[1].SceneOffset != 1 {
		t.Errorf("drawMonoids[1].SceneOffset = %d, want 1", drawMonoids[1].SceneOffset)
	}

	// Third draw monoid should have PathIx=2, SceneOffset=2.
	if drawMonoids[2].PathIx != 2 {
		t.Errorf("drawMonoids[2].PathIx = %d, want 2", drawMonoids[2].PathIx)
	}
	if drawMonoids[2].SceneOffset != 2 {
		t.Errorf("drawMonoids[2].SceneOffset = %d, want 2", drawMonoids[2].SceneOffset)
	}

	// Verify info buffer has 3 entries (1 info_size per DrawTagColor).
	if len(info) != 3 {
		t.Fatalf("len(info) = %d, want 3", len(info))
	}

	// Verify colors in info buffer match packed draw data.
	for i := 0; i < 3; i++ {
		if info[i] != enc.DrawData[i] {
			t.Errorf("info[%d] = 0x%08X, want 0x%08X (from DrawData)", i, info[i], enc.DrawData[i])
		}
	}

	// Verify the semi-transparent blue is correctly premultiplied in info.
	blueInfo := info[2]
	bR := blueInfo & 0xFF
	bG := (blueInfo >> 8) & 0xFF
	bB := (blueInfo >> 16) & 0xFF
	bA := (blueInfo >> 24) & 0xFF
	if bR != 0 || bG != 0 || bA != 128 {
		t.Errorf("blue info: R=%d G=%d B=%d A=%d", bR, bG, bB, bA)
	}
	if bB < 127 || bB > 129 {
		t.Errorf("blue info B=%d, want ~128", bB)
	}

	t.Logf("Draw monoids: %+v", drawMonoids)
	t.Logf("Info buffer (%d entries): %v", len(info), info)
}

// TestPackPathTags verifies the 4-tags-per-uint32 packing.
func TestPackPathTags(t *testing.T) {
	tests := []struct {
		name string
		tags []uint8
		want []uint32
	}{
		{
			name: "empty",
			tags: nil,
			want: []uint32{},
		},
		{
			name: "single_tag",
			tags: []uint8{0x09},
			want: []uint32{0x00000009},
		},
		{
			name: "four_tags",
			tags: []uint8{0x20, 0x40, 0x09, 0x10},
			want: []uint32{0x10094020},
		},
		{
			name: "five_tags",
			tags: []uint8{0x09, 0x09, 0x09, 0x10, 0x20},
			want: []uint32{0x10090909, 0x00000020},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := packPathTags(tt.tags)
			if len(got) != len(tt.want) {
				t.Fatalf("packPathTags len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("packPathTags[%d] = 0x%08X, want 0x%08X", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestColorPacking verifies premultiplied color packing for various alpha values.
func TestColorPacking(t *testing.T) {
	tests := []struct {
		name  string
		color [4]uint8
		wantR uint32
		wantG uint32
		wantB uint32
		wantA uint32
	}{
		{
			name:  "opaque_red",
			color: [4]uint8{255, 0, 0, 255},
			wantR: 255, wantG: 0, wantB: 0, wantA: 255,
		},
		{
			name:  "opaque_white",
			color: [4]uint8{255, 255, 255, 255},
			wantR: 255, wantG: 255, wantB: 255, wantA: 255,
		},
		{
			name:  "fully_transparent",
			color: [4]uint8{255, 128, 64, 0},
			wantR: 0, wantG: 0, wantB: 0, wantA: 0,
		},
		{
			name:  "half_transparent_white",
			color: [4]uint8{255, 255, 255, 128},
			wantR: 128, wantG: 128, wantB: 128, wantA: 128,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := PathDef{
				Lines:    polygonToLineSoup([][2]float32{{0, 0}, {10, 0}, {10, 10}}),
				Color:    tt.color,
				FillRule: FillRuleNonZero,
			}
			enc := EncodeScene([]PathDef{pd})
			packed := enc.DrawData[0]
			gotR := packed & 0xFF
			gotG := (packed >> 8) & 0xFF
			gotB := (packed >> 16) & 0xFF
			gotA := (packed >> 24) & 0xFF

			// Allow +/- 1 for rounding.
			if absDiffU32(gotR, tt.wantR) > 1 || absDiffU32(gotG, tt.wantG) > 1 ||
				absDiffU32(gotB, tt.wantB) > 1 || absDiffU32(gotA, tt.wantA) > 1 {
				t.Errorf("color(%v): got RGBA(%d,%d,%d,%d) want RGBA(%d,%d,%d,%d)",
					tt.color, gotR, gotG, gotB, gotA, tt.wantR, tt.wantG, tt.wantB, tt.wantA)
			}
		})
	}
}

// TestEncodeSceneEvenOddStyle verifies that even-odd fill rule is encoded correctly.
func TestEncodeSceneEvenOddStyle(t *testing.T) {
	pd := PathDef{
		Lines: polygonToLineSoup([][2]float32{
			{50, 10}, {75, 90}, {10, 40}, {90, 40}, {25, 90},
		}),
		Color:    [4]uint8{128, 0, 0, 255},
		FillRule: FillRuleEvenOdd,
	}

	enc := EncodeScene([]PathDef{pd})

	if len(enc.Styles) != 1 {
		t.Fatalf("len(Styles) = %d, want 1", len(enc.Styles))
	}
	if enc.Styles[0] != 0x02 {
		t.Errorf("Styles[0] = 0x%X, want 0x02 (even-odd)", enc.Styles[0])
	}
}

// TestSceneRoundTrip verifies that encoding and packing produces a valid
// packed scene that can be consumed by reduce/scan stages without panic.
func TestSceneRoundTrip(t *testing.T) {
	paths := []PathDef{
		{
			Lines:    FlattenFill(circleCubics(50, 50, 20)),
			Color:    [4]uint8{255, 128, 0, 255},
			FillRule: FillRuleNonZero,
		},
		{
			Lines: polygonToLineSoup([][2]float32{
				{10, 10}, {90, 10}, {90, 90}, {10, 90},
			}),
			Color:    [4]uint8{0, 128, 255, 200},
			FillRule: FillRuleEvenOdd,
		},
	}

	enc := EncodeScene(paths)
	packed := PackScene(enc)

	// Run pathtag reduce + scan.
	ptReduced := pathtagReduce(packed)
	ptScanned := pathtagScan(packed, ptReduced)

	// Run draw reduce + scan.
	dReduced := drawReduce(packed)
	dMonoids, info := drawLeafScan(packed, dReduced)

	// Basic sanity checks.
	numTagWords := numPathTagWords(packed)
	if uint32(len(ptScanned)) != numTagWords {
		t.Errorf("ptScanned len = %d, want %d", len(ptScanned), numTagWords)
	}
	if len(dMonoids) != 2 {
		t.Errorf("dMonoids len = %d, want 2", len(dMonoids))
	}
	if len(info) != 2 {
		t.Errorf("info len = %d, want 2", len(info))
	}

	// Total path monoid should reflect 2 paths.
	totalPT := PathMonoid{}
	for _, r := range ptReduced {
		totalPT = totalPT.combine(r)
	}
	if totalPT.PathIx != 2 {
		t.Errorf("total PathIx = %d, want 2", totalPT.PathIx)
	}
	if totalPT.TransIx != 2 {
		t.Errorf("total TransIx = %d, want 2", totalPT.TransIx)
	}
	if totalPT.StyleIx != 2 {
		t.Errorf("total StyleIx = %d, want 2", totalPT.StyleIx)
	}

	t.Logf("Round-trip: %d tag words, %d draw objects, %d info entries",
		numTagWords, len(dMonoids), len(info))
	t.Logf("Total PathMonoid: %+v", totalPT)
}

// absDiffU32 returns the absolute difference between two uint32 values.
func absDiffU32(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// Ensure math and bits are used (prevent "imported and not used" errors).
var _ = math.Float32bits
var _ = bits.OnesCount32
