// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"testing"
)

// TestFillToBufferNoAA_BinaryPixels verifies that FillToBufferNoAA produces
// only binary coverage values: 0 (outside) or 255 (inside). No intermediate
// alpha values are permitted — this is the defining property of aliased
// rasterization.
func TestFillToBufferNoAA_BinaryPixels(t *testing.T) {
	const width, height = 100, 100

	// Triangle path: clearly has diagonal edges where AA would produce gray.
	path := &testPath{
		verbs: []PathVerb{MoveTo, LineTo, LineTo, Close},
		points: []float32{
			50, 10, // top
			10, 90, // bottom-left
			90, 90, // bottom-right
		},
	}

	eb := NewEdgeBuilder(0) // aaShift=0 for NoAA
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	buffer := make([]byte, width*height)
	FillToBufferNoAA(eb, width, height, FillRuleNonZero, buffer)

	// Every pixel must be 0 or 255 — no intermediate values.
	nonBinaryCount := 0
	for i, v := range buffer {
		if v != 0 && v != 255 {
			nonBinaryCount++
			if nonBinaryCount <= 5 {
				y := i / width
				x := i % width
				t.Errorf("pixel(%d,%d) = %d, want 0 or 255", x, y, v)
			}
		}
	}
	if nonBinaryCount > 0 {
		t.Errorf("found %d non-binary pixels (expected all 0 or 255)", nonBinaryCount)
	}

	// Interior pixels must be filled (255).
	if buffer[50*width+50] != 255 {
		t.Errorf("interior pixel (50,50) = %d, want 255", buffer[50*width+50])
	}
	if buffer[60*width+50] != 255 {
		t.Errorf("interior pixel (50,60) = %d, want 255", buffer[60*width+50])
	}

	// Exterior pixels must be empty (0).
	if buffer[0] != 0 {
		t.Errorf("exterior pixel (0,0) = %d, want 0", buffer[0])
	}
	if buffer[5*width+5] != 0 {
		t.Errorf("exterior pixel (5,5) = %d, want 0", buffer[5*width+5])
	}
}

// TestFillToBufferNoAA_Rectangle verifies a simple axis-aligned rectangle.
func TestFillToBufferNoAA_Rectangle(t *testing.T) {
	const width, height = 50, 50

	path := &testPath{
		verbs: []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{
			10, 10,
			40, 10,
			40, 40,
			10, 40,
		},
	}

	eb := NewEdgeBuilder(0)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	buffer := make([]byte, width*height)
	FillToBufferNoAA(eb, width, height, FillRuleNonZero, buffer)

	// All pixels inside [10,39] x [10,39] should be 255.
	for y := 10; y < 40; y++ {
		for x := 10; x < 40; x++ {
			if buffer[y*width+x] != 255 {
				t.Errorf("interior pixel(%d,%d) = %d, want 255", x, y, buffer[y*width+x])
			}
		}
	}

	// Pixels outside should be 0.
	for x := range width {
		if buffer[0*width+x] != 0 {
			t.Errorf("exterior pixel(%d,0) = %d, want 0", x, buffer[x])
		}
	}
}

// TestFillToBufferNoAA_EmptyPath verifies no crash on empty path.
func TestFillToBufferNoAA_EmptyPath(t *testing.T) {
	buffer := make([]byte, 100)
	eb := NewEdgeBuilder(0)
	// Don't build any path.
	FillToBufferNoAA(eb, 10, 10, FillRuleNonZero, buffer)

	// All zeros.
	for i, v := range buffer {
		if v != 0 {
			t.Errorf("buffer[%d] = %d, want 0 (empty path)", i, v)
		}
	}
}

// TestFillToBufferNoAA_SmallBuffer verifies no crash when buffer is too small.
func TestFillToBufferNoAA_SmallBuffer(t *testing.T) {
	eb := NewEdgeBuilder(0)
	buffer := make([]byte, 5) // Too small for 10x10.
	// Should return without panic.
	FillToBufferNoAA(eb, 10, 10, FillRuleNonZero, buffer)
}

// TestFillToBufferNoAA_VsFillToBuffer_Diagonal compares aliased vs AA output.
// The aliased buffer must have only 0/255, while the AA buffer will have
// intermediate values on diagonal edges.
func TestFillToBufferNoAA_VsFillToBuffer_Diagonal(t *testing.T) {
	const width, height = 100, 100

	// Diamond shape — all diagonal edges, guarantees AA fringe in FillToBuffer.
	path := &testPath{
		verbs: []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{
			50, 10,
			90, 50,
			50, 90,
			10, 50,
		},
	}

	// Aliased
	ebNoAA := NewEdgeBuilder(0)
	ebNoAA.SetFlattenCurves(true)
	ebNoAA.BuildFromPath(path, IdentityTransform{})
	noaaBuf := make([]byte, width*height)
	FillToBufferNoAA(ebNoAA, width, height, FillRuleNonZero, noaaBuf)

	// Anti-aliased
	ebAA := NewEdgeBuilder(2)
	ebAA.SetFlattenCurves(true)
	ebAA.BuildFromPath(path, IdentityTransform{})
	aaBuf := make([]byte, width*height)
	FillToBuffer(ebAA, width, height, FillRuleNonZero, aaBuf)

	// Verify aliased buffer has ONLY binary pixels.
	for i, v := range noaaBuf {
		if v != 0 && v != 255 {
			y := i / width
			x := i % width
			t.Fatalf("noAA pixel(%d,%d) = %d, want 0 or 255", x, y, v)
		}
	}

	// Verify AA buffer has at least SOME intermediate values on diagonal edges.
	hasIntermediate := false
	for _, v := range aaBuf {
		if v > 0 && v < 255 {
			hasIntermediate = true
			break
		}
	}
	if !hasIntermediate {
		t.Error("AA buffer has no intermediate coverage values — expected some on diagonal edges")
	}
}
