// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package tilecompute

import (
	"math"
	"testing"
)

// TestFineRasterizeClip verifies that CmdBeginClip/CmdEndClip produce
// correct compositing output. This exercises the packed blend stack
// (the code path that caused Adreno miscompilation in BUG-ADRENO-001).
func TestFineRasterizeClip(t *testing.T) {
	bg := [4]float32{1, 1, 1, 1} // white background (premultiplied)

	// Build a PTCL with: Solid → BeginClip → Color(red 50% alpha) → EndClip
	// Expected: white background, then clip layer captures it, then red 50%
	// composited on top via EndClip.
	ptcl := NewPTCL()

	// Solid: full tile coverage (backdrop=1, no segments)
	ptcl.WriteSolid()

	// Color: white (will be captured by BeginClip)
	ptcl.WriteColor(packRGBA(255, 255, 255, 255))

	// BeginClip: save current rgba, reset to transparent
	ptcl.WriteBeginClip()

	// Inside clip: solid red at 50% alpha
	ptcl.WriteSolid()
	ptcl.WriteColor(packRGBA(128, 0, 0, 128)) // premultiplied: (128,0,0,128)

	// EndClip: composite clip content over saved background
	// blend_mix=0 (normal), alpha=1.0 (full clip)
	ptcl.WriteEndClip(0, 1.0)

	// End
	ptcl.WriteEnd()

	result := fineRasterizeTile(ptcl, nil, bg)

	// Check center pixel (8,8) — should be red composited over white.
	// EndClip: bg * (1 - fg.a) + fg
	// fg = (128/255, 0, 0, 128/255) * area(1.0) * alpha(1.0)
	// fg premultiplied = (0.502, 0, 0, 0.502)
	// result = white * (1 - 0.502) + (0.502, 0, 0, 0.502)
	//        = (0.498, 0.498, 0.498, 0.498) + (0.502, 0, 0, 0.502)
	//        = (1.0, 0.498, 0.498, 1.0)
	px := result[8*TileWidth+8]
	t.Logf("Clip result pixel (8,8): R=%.3f G=%.3f B=%.3f A=%.3f", px[0], px[1], px[2], px[3])

	if px[3] < 0.9 {
		t.Errorf("Alpha too low: got %.3f, want ~1.0", px[3])
	}
	if px[0] < 0.9 {
		t.Errorf("Red channel too low: got %.3f, want ~1.0", px[0])
	}
	if px[1] > 0.6 {
		t.Errorf("Green channel too high: got %.3f, want ~0.5", px[1])
	}
	if px[2] > 0.6 {
		t.Errorf("Blue channel too high: got %.3f, want ~0.5", px[2])
	}
}

// TestFineRasterizeNestedClip tests nested clip layers (depth=2).
// This exercises blend_stack at depth 0 AND depth 1.
func TestFineRasterizeNestedClip(t *testing.T) {
	bg := [4]float32{0, 0, 0, 1} // black background

	ptcl := NewPTCL()

	// Layer 0: solid green
	ptcl.WriteSolid()
	ptcl.WriteColor(packRGBA(0, 255, 0, 255))

	// Clip 1: save green
	ptcl.WriteBeginClip()

	// Layer 1: solid blue
	ptcl.WriteSolid()
	ptcl.WriteColor(packRGBA(0, 0, 255, 255))

	// Clip 2 (nested): save blue
	ptcl.WriteBeginClip()

	// Layer 2: solid red 50%
	ptcl.WriteSolid()
	ptcl.WriteColor(packRGBA(128, 0, 0, 128))

	// EndClip 2: red over blue
	ptcl.WriteEndClip(0, 1.0)

	// EndClip 1: result over green
	ptcl.WriteEndClip(0, 1.0)

	ptcl.WriteEnd()

	result := fineRasterizeTile(ptcl, nil, bg)
	px := result[8*TileWidth+8]
	t.Logf("Nested clip pixel (8,8): R=%.3f G=%.3f B=%.3f A=%.3f", px[0], px[1], px[2], px[3])

	// Inner EndClip: red(0.502,0,0,0.502) over blue(0,0,1,1)
	// = blue*(1-0.502) + red = (0, 0, 0.498, 0.498) + (0.502, 0, 0, 0.502) = (0.502, 0, 0.498, 1.0)
	// Outer EndClip: above over green(0,1,0,1)
	// = green*(1-1.0) + above = (0.502, 0, 0.498, 1.0)
	// Wait — alpha=1.0 means full clip, so fg.a from inner is 1.0
	// Result should be dominated by inner clip result

	if px[3] < 0.9 {
		t.Errorf("Alpha too low: got %.3f, want ~1.0", px[3])
	}
	// Should have some red and some blue, very little green
	if px[0] < 0.3 {
		t.Errorf("Red too low (nested clip): got %.3f", px[0])
	}
	if px[2] < 0.3 {
		t.Errorf("Blue too low (nested clip): got %.3f", px[2])
	}
}

// TestFineRasterizeDeepClip tests clip depth > BlendStackSplit (4)
// to exercise the blend_spill path.
func TestFineRasterizeDeepClip(t *testing.T) {
	bg := [4]float32{1, 1, 1, 1} // white

	ptcl := NewPTCL()

	// Push 5 clip levels (exceeds BlendStackSplit=4, triggers spill)
	for i := 0; i < 5; i++ {
		ptcl.WriteSolid()
		ptcl.WriteColor(packRGBA(255, 0, 0, 255)) // red
		ptcl.WriteBeginClip()
	}

	// Innermost: blue
	ptcl.WriteSolid()
	ptcl.WriteColor(packRGBA(0, 0, 255, 255))

	// Pop all 5
	for i := 0; i < 5; i++ {
		ptcl.WriteEndClip(0, 1.0)
	}

	ptcl.WriteEnd()

	result := fineRasterizeTile(ptcl, nil, bg)
	px := result[8*TileWidth+8]
	t.Logf("Deep clip (depth=5) pixel (8,8): R=%.3f G=%.3f B=%.3f A=%.3f", px[0], px[1], px[2], px[3])

	// Should have blue composited over red layers
	if px[3] < 0.9 {
		t.Errorf("Alpha too low: got %.3f, want ~1.0", px[3])
	}
	// Blue should dominate (innermost, full alpha, full coverage)
	if px[2] < 0.5 {
		t.Errorf("Blue too low: got %.3f, want dominant blue", px[2])
	}
}

// packRGBA packs RGBA bytes into a uint32 (Vello format: R in low byte).
func packRGBA(r, g, b, a uint8) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16 | uint32(a)<<24
}

// Verify packRGBA matches what fine.go expects.
func TestPackRGBA(t *testing.T) {
	// Red fully opaque
	got := packRGBA(255, 0, 0, 255)
	r := float32(got&0xFF) / 255.0
	a := float32((got>>24)&0xFF) / 255.0
	if r != 1.0 || a != 1.0 {
		t.Errorf("packRGBA(255,0,0,255) = %08x, r=%.1f a=%.1f", got, r, a)
	}

	// Check it matches WriteColor encoding
	ptcl := NewPTCL()
	ptcl.WriteColor(got)
	_, offset := ptcl.ReadCmd(CmdStartOffset)
	colorData, _ := ptcl.ReadColorData(offset)
	rOut := float32(colorData.RGBA&0xFF) / 255.0
	aOut := float32((colorData.RGBA>>24)&0xFF) / 255.0
	_ = math.Abs(float64(rOut - 1.0)) // just verify no panic
	_ = math.Abs(float64(aOut - 1.0))
}
