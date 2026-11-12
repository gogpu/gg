// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package text

import "testing"

// TestBuiltinShapersSetFace: the built-in shapers record the face each glyph
// came from, so Shape and ShapeRuns need not copy their result to annotate it
// (P3-TEXT-358).
func TestBuiltinShapersSetFace(t *testing.T) {
	src, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatal(err)
	}
	face := src.Face(16)
	for _, tc := range []struct {
		name   string
		shaper Shaper
	}{
		{"BuiltinShaper", &BuiltinShaper{}},
		{"OwnShaper", NewOwnShaper()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			glyphs := tc.shaper.Shape("Hello, world", face)
			if len(glyphs) == 0 {
				t.Fatal("no glyphs")
			}
			for i, g := range glyphs {
				if g.Face != face {
					t.Fatalf("glyph %d: Face = %v, want the shaped face", i, g.Face)
				}
			}
			if annotated := annotateShapedGlyphs(glyphs, face); &annotated[0] != &glyphs[0] {
				t.Fatal("annotateShapedGlyphs copied a result that already carries its face")
			}
		})
	}
}
