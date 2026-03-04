// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"testing"
)

// clipTestLinePath creates a PathLike with a single line from (x0,y0) to (x1,y1).
func clipTestLinePath(x0, y0, x1, y1 float32) PathLike {
	return &clipTestPathData{
		verbs:  []PathVerb{VerbMoveTo, VerbLineTo},
		points: []float32{x0, y0, x1, y1},
	}
}

// clipTestRectPath creates a PathLike for a rectangle at (x,y) with size (w,h).
func clipTestRectPath(x, y, w, h float32) PathLike {
	return &clipTestPathData{
		verbs: []PathVerb{
			VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo, VerbClose,
		},
		points: []float32{
			x, y,
			x + w, y,
			x + w, y + h,
			x, y + h,
		},
	}
}

type clipTestPathData struct {
	verbs  []PathVerb
	points []float32
}

func (p *clipTestPathData) IsEmpty() bool     { return len(p.verbs) == 0 }
func (p *clipTestPathData) Verbs() []PathVerb { return p.verbs }
func (p *clipTestPathData) Points() []float32 { return p.points }

func TestClipLineEntirelyInside(t *testing.T) {
	eb := NewEdgeBuilder(4) // aaShift=4 like SoftwareRenderer
	clip := Rect{MinX: 0, MinY: 0, MaxX: 800, MaxY: 600}
	eb.SetClipRect(&clip)

	eb.BuildFromPath(clipTestLinePath(100, 100, 200, 200), IdentityTransform{})

	if eb.IsEmpty() {
		t.Fatal("expected edges for line inside clip rect")
	}
	// BuildFromPath creates 2 edges: the line + the implicit close line
	if eb.LineEdgeCount() != 2 {
		t.Errorf("expected 2 line edges (line + close), got %d", eb.LineEdgeCount())
	}
}

func TestClipLineEntirelyOutsideRight(t *testing.T) {
	eb := NewEdgeBuilder(4)
	clip := Rect{MinX: 0, MinY: 0, MaxX: 800, MaxY: 600}
	eb.SetClipRect(&clip)

	// Line entirely to the right of clip → sentinel verticals.
	// But BuildFromPath auto-closes: line + close line = two opposite sentinels
	// at the same X → they cancel via combineVertical (correct: zero net winding).
	eb.BuildFromPath(clipTestLinePath(1000, 100, 1200, 200), IdentityTransform{})

	// Canceled sentinels = 0 edges. This is correct for an open path segment
	// that's entirely outside — it contributes zero winding.
	if !eb.IsEmpty() {
		// If edges remain, they must be sentinel verticals (DX=0)
		for edge := range eb.LineEdges() {
			if edge.DX != 0 {
				t.Errorf("sentinel should be vertical (DX=0), got DX=%d", edge.DX)
			}
		}
	}
}

func TestClipLineEntirelyAbove(t *testing.T) {
	eb := NewEdgeBuilder(4)
	clip := Rect{MinX: 0, MinY: 0, MaxX: 800, MaxY: 600}
	eb.SetClipRect(&clip)

	// Line entirely above clip → Y-clipped, no edges
	eb.BuildFromPath(clipTestLinePath(100, -200, 200, -100), IdentityTransform{})

	if !eb.IsEmpty() {
		t.Errorf("expected no edges for line entirely above clip, got %d", eb.EdgeCount())
	}
}

func TestClipLineEntirelyBelow(t *testing.T) {
	eb := NewEdgeBuilder(4)
	clip := Rect{MinX: 0, MinY: 0, MaxX: 800, MaxY: 600}
	eb.SetClipRect(&clip)

	// Line entirely below clip → Y-clipped, no edges
	eb.BuildFromPath(clipTestLinePath(100, 700, 200, 800), IdentityTransform{})

	if !eb.IsEmpty() {
		t.Errorf("expected no edges for line entirely below clip, got %d", eb.EdgeCount())
	}
}

func TestClipLineCrossingRightBoundary(t *testing.T) {
	eb := NewEdgeBuilder(4)
	clip := Rect{MinX: 0, MinY: 0, MaxX: 800, MaxY: 600}
	eb.SetClipRect(&clip)

	// Line crosses right boundary: starts inside, ends outside
	eb.BuildFromPath(clipTestLinePath(700, 100, 900, 300), IdentityTransform{})

	if eb.IsEmpty() {
		t.Fatal("expected edges for line crossing right boundary")
	}
	// Should have multiple edges: visible portion + sentinel + close line segments
	if eb.LineEdgeCount() < 2 {
		t.Errorf("expected at least 2 line edges, got %d", eb.LineEdgeCount())
	}
}

func TestClipNoClipRect(t *testing.T) {
	// Without clip rect, addLine should work as before
	eb := NewEdgeBuilder(4)

	eb.BuildFromPath(clipTestLinePath(5000, 100, 5200, 200), IdentityTransform{})

	// Without clipping, far-off-screen lines still produce edges
	// (FDot16 is now saturated instead of wrapping, so edges are at max coords)
	if eb.IsEmpty() {
		t.Error("without clip rect, edges should be created even for far coordinates")
	}
}

// TestClipRectPreservesWinding verifies that a rectangle half-visible
// fills correctly — the sentinel verticals at the clip boundary should
// preserve winding so only the visible half gets filled.
func TestClipRectPreservesWinding(t *testing.T) {
	eb := NewEdgeBuilder(4)
	clip := Rect{MinX: 0, MinY: 0, MaxX: 800, MaxY: 600}
	eb.SetClipRect(&clip)
	eb.SetFlattenCurves(true)

	// Rectangle from x=700 to x=900 — right half is outside
	eb.BuildFromPath(clipTestRectPath(700, 100, 200, 100), IdentityTransform{})

	if eb.IsEmpty() {
		t.Fatal("expected edges for partially visible rectangle")
	}

	// The rect has 4 sides:
	// - Top (700,100→900,100): horizontal → no edge (y0==y1)
	// - Right (900,100→900,200): outside → sentinel at x=800
	// - Bottom (900,200→700,200): horizontal → no edge
	// - Left (700,200→700,100): inside → normal vertical edge
	// The right sentinel and left edge give us 2 edges (may combine)
	if eb.LineEdgeCount() < 2 {
		t.Errorf("expected at least 2 edges for partial rect, got %d", eb.LineEdgeCount())
	}
}

// TestClipOverflowCoordinates is the key RAST-010 test: coordinates at x=5500
// with aaShift=4 would overflow FDot16 without clipping.
func TestClipOverflowCoordinates(t *testing.T) {
	eb := NewEdgeBuilder(4) // aaShift=4 → overflow at x>2048
	clip := Rect{MinX: -2, MinY: -2, MaxX: 802, MaxY: 602}
	eb.SetClipRect(&clip)
	eb.SetFlattenCurves(true)

	// Rectangle at x=5500 — entirely off-screen, would overflow without clipping
	eb.BuildFromPath(clipTestRectPath(5500, 100, 200, 100), IdentityTransform{})

	// All edges should be sentinel verticals at right boundary (802)
	// None should have X values corresponding to wrapped overflow (x≈1404)
	for edge := range eb.LineEdges() {
		// Convert FDot16 X to pixel: x = FDot16 / 65536
		xPixel := float32(edge.X) / 65536.0
		if xPixel > 0 && xPixel < 800 {
			t.Errorf("edge X=%.1f is inside visible area — overflow not prevented! "+
				"(FDot16=%d)", xPixel, edge.X)
		}
	}
}

// TestClipLargeCanvasNoOverflow verifies that a 3000px canvas with geometry
// at the right edge doesn't produce overflow artifacts.
// At aaShift=4, x>2048 overflows FDot16. With saturating FDot6ToFDot16,
// the overflow is clamped (no wrap-around to visible area).
func TestClipLargeCanvasNoOverflow(t *testing.T) {
	eb := NewEdgeBuilder(4) // aaShift=4
	clip := Rect{MinX: -2, MinY: -2, MaxX: 3002, MaxY: 1002}
	eb.SetClipRect(&clip)
	eb.SetFlattenCurves(true)

	// Rect at x=2900 — near right edge of 3000px canvas, inside clip.
	// At aaShift=4, x=2900 exceeds safe FDot16 range (2048) but
	// saturating FDot6ToFDot16 prevents wrap-around.
	eb.BuildFromPath(clipTestRectPath(2900, 100, 50, 50), IdentityTransform{})

	if eb.IsEmpty() {
		t.Fatal("expected edges for rect near canvas edge")
	}

	// With saturating arithmetic, edges should NOT wrap to negative/wrong positions.
	// They may be clamped to int32 max, but NOT wrapped to small positive values
	// that would appear as artifacts in the visible area.
	for edge := range eb.LineEdges() {
		xPixel := float32(edge.X) / 65536.0
		// The key check: x should NOT wrap to a small visible value (like 1404).
		// Clamped values at int32 max ≈ 32767 are OK (far off-screen).
		if xPixel > 0 && xPixel < 2000 {
			t.Errorf("edge X=%.1f wrapped to visible area — overflow! "+
				"(FDot16=%d)", xPixel, edge.X)
		}
	}
}

// TestClipSaturatingFDot16 verifies that FDot6ToFDot16 no longer wraps.
func TestClipSaturatingFDot16(t *testing.T) {
	// x=5500 at aaShift=4: FDot6 = 5500 * 1024 = 5,632,000
	// Without saturation: 5,632,000 << 10 = 5,767,168,000 → wraps to 1,472,200,704 → 1404.0
	// With saturation: clamped to 0x7FFFFFFF = 2,147,483,647 → 32767.99
	fdot6 := FDot6(5500 * 1024) // 5,632,000
	fdot16 := FDot6ToFDot16(fdot6)

	// Must NOT wrap to a small value
	if fdot16 < 0 || FDot16ToFloat32(fdot16) < 2048 {
		t.Errorf("FDot6ToFDot16 wrapped! got FDot16=%d (%.1f px), "+
			"expected clamped to max", fdot16, FDot16ToFloat32(fdot16))
	}

	// Should be clamped to max int32
	if fdot16 != 0x7FFFFFFF {
		t.Errorf("expected clamped to 0x7FFFFFFF, got %d", fdot16)
	}
}

// TestClipSaturatingFDot16Negative verifies saturation for negative overflow.
func TestClipSaturatingFDot16Negative(t *testing.T) {
	fdot6 := FDot6(-5500 * 1024)
	fdot16 := FDot6ToFDot16(fdot6)

	if fdot16 > 0 || FDot16ToFloat32(fdot16) > -2048 {
		t.Errorf("FDot6ToFDot16 wrapped for negative! got FDot16=%d (%.1f px)",
			fdot16, FDot16ToFloat32(fdot16))
	}

	if fdot16 != -0x7FFFFFFF {
		t.Errorf("expected clamped to -0x7FFFFFFF, got %d", fdot16)
	}
}

// TestClipSafeFDot16Values verifies normal values still convert correctly.
// FDot6 = pixel * 64 (standard, without AA scaling).
// FDot6ToFDot16 shifts left by 10 → FDot16 = pixel * 65536.
// FDot16ToFloat32 divides by 65536 → back to pixel.
func TestClipSafeFDot16Values(t *testing.T) {
	tests := []struct {
		pixel float32
	}{
		{0},
		{100},
		{500},
		{1000},
		{2000},
	}

	for _, tc := range tests {
		fdot6 := FDot6(tc.pixel * 64) // Standard FDot6 (26.6 format)
		fdot16 := FDot6ToFDot16(fdot6)
		back := FDot16ToFloat32(fdot16)

		if back != tc.pixel {
			t.Errorf("pixel=%.0f: expected round-trip %.0f, got %.1f (FDot6=%d, FDot16=%d)",
				tc.pixel, tc.pixel, back, fdot6, fdot16)
		}
	}
}
