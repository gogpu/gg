package text

import (
	"math"
	"testing"
)

func TestHinting_String(t *testing.T) {
	tests := []struct {
		h    Hinting
		want string
	}{
		{HintingNone, "None"},
		{HintingVertical, "Vertical"},
		{HintingFull, "Full"},
		{Hinting(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.h.String(); got != tt.want {
			t.Errorf("Hinting(%d).String() = %q, want %q", tt.h, got, tt.want)
		}
	}
}

func TestToFontHinting(t *testing.T) {
	// Verify our enum maps to x/image/font.Hinting correctly.
	if got := toFontHinting(HintingNone); got != 0 {
		t.Errorf("toFontHinting(HintingNone) = %d, want 0", got)
	}
	if got := toFontHinting(HintingVertical); got != 1 {
		t.Errorf("toFontHinting(HintingVertical) = %d, want 1", got)
	}
	if got := toFontHinting(HintingFull); got != 2 {
		t.Errorf("toFontHinting(HintingFull) = %d, want 2", got)
	}
}

func TestGridFitOutline_Nil(t *testing.T) {
	// Should not panic on nil.
	gridFitOutline(nil, HintingFull)
}

func TestGridFitOutline_Empty(t *testing.T) {
	outline := &GlyphOutline{Segments: nil}
	gridFitOutline(outline, HintingFull)
	if len(outline.Segments) != 0 {
		t.Error("empty outline should remain empty")
	}
}

func TestGridFitOutline_NoHinting(t *testing.T) {
	outline := makeTestSquare(0.3, 0.7) // off-grid Y values
	original := outline.Clone()
	gridFitOutline(outline, HintingNone)

	// With HintingNone, gridFitOutline returns early — no modification.
	// (The function is never called with HintingNone in production, but verify.)
	// Actually gridFitOutline is only called when hinting != None, but test the logic.
	for i, seg := range outline.Segments {
		for j := range seg.Points {
			if seg.Points[j] != original.Segments[i].Points[j] {
				t.Error("HintingNone should not modify outline coordinates")
				return
			}
		}
	}
}

func TestGridFitOutline_BaselineSnap(t *testing.T) {
	// Create an outline with Y-values near 0 (baseline).
	// Grid-fitting should snap them to exactly 0.
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 1.0, Y: 0.15}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 5.0, Y: 0.15}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 5.0, Y: -8.0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 1.0, Y: -8.0}}},
		},
		Bounds: Rect{MinX: 1, MinY: -8, MaxX: 5, MaxY: 0.15},
	}

	gridFitOutline(outline, HintingVertical)

	// Y=0.15 should snap to 0 (within snapThreshold=0.3 of baseline).
	if outline.Segments[0].Points[0].Y != 0 {
		t.Errorf("baseline Y snap: got %f, want 0", outline.Segments[0].Points[0].Y)
	}
	if outline.Segments[1].Points[0].Y != 0 {
		t.Errorf("baseline Y snap: got %f, want 0", outline.Segments[1].Points[0].Y)
	}
}

func TestGridFitOutline_HorizontalSegmentSnap(t *testing.T) {
	// Two points forming a near-horizontal line at Y≈5.1.
	// Should snap to Y=5.0.
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 5.05}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 4, Y: 5.15}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 4, Y: 10}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: 10}}},
		},
		Bounds: Rect{MinX: 0, MinY: 5.05, MaxX: 4, MaxY: 10},
	}

	gridFitOutline(outline, HintingVertical)

	// Y≈5.1 should snap to 5.0 (horizontal segment detection).
	snappedY0 := outline.Segments[0].Points[0].Y
	snappedY1 := outline.Segments[1].Points[0].Y
	if snappedY0 != snappedY1 {
		t.Errorf("horizontal segment Y values should be equal: %f vs %f", snappedY0, snappedY1)
	}
	if snappedY0 != 5.0 {
		t.Errorf("horizontal segment should snap to 5.0, got %f", snappedY0)
	}
}

func TestGridFitOutline_VerticalOnly(t *testing.T) {
	// With HintingVertical, X-coordinates should NOT be modified.
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 3.15, Y: 0.1}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 3.15, Y: -8.0}}},
		},
		Bounds: Rect{MinX: 3.15, MinY: -8, MaxX: 3.15, MaxY: 0.1},
	}

	gridFitOutline(outline, HintingVertical)

	// X should stay at 3.15 (not snapped to 3.0) with HintingVertical.
	if outline.Segments[0].Points[0].X != 3.15 {
		t.Errorf("HintingVertical should not snap X: got %f, want 3.15",
			outline.Segments[0].Points[0].X)
	}
}

func TestGridFitOutline_FullSnapsX(t *testing.T) {
	// With HintingFull, X-coordinates near integers should snap.
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 3.15, Y: 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 3.15, Y: -8.0}}},
		},
		Bounds: Rect{MinX: 3.15, MinY: -8, MaxX: 3.15, MaxY: 0},
	}

	gridFitOutline(outline, HintingFull)

	// X=3.15 should snap to 3.0 (within snapThreshold=0.3).
	if outline.Segments[0].Points[0].X != 3.0 {
		t.Errorf("HintingFull should snap X: got %f, want 3.0",
			outline.Segments[0].Points[0].X)
	}
}

func TestGridFitOutline_NoSnapFarFromGrid(t *testing.T) {
	// Y=0.5 is too far from any integer (>snapThreshold=0.3) for baseline snap.
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 1, Y: 0.5}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 5, Y: 0.5}}},
		},
		Bounds: Rect{MinX: 1, MinY: 0.5, MaxX: 5, MaxY: 0.5},
	}

	gridFitOutline(outline, HintingVertical)

	// Y=0.5 should NOT snap to 0 (too far from baseline).
	// But it MAY snap as a horizontal segment pair to round(0.5)=1.0.
	// The key is it should not snap to 0.
	if outline.Segments[0].Points[0].Y == 0 {
		t.Error("Y=0.5 should not snap to baseline (too far from 0)")
	}
}

func TestGridFitOutline_BoundsUpdated(t *testing.T) {
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 1, Y: 0.1}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 5, Y: 0.1}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 5, Y: -8.0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 1, Y: -8.0}}},
		},
		Bounds: Rect{MinX: 1, MinY: -8, MaxX: 5, MaxY: 0.1},
	}

	gridFitOutline(outline, HintingVertical)

	// Bounds should be updated after snapping Y=0.1 → 0.
	if outline.Bounds.MaxY != 0 {
		t.Errorf("bounds MaxY should be updated to 0 after snap, got %f", outline.Bounds.MaxY)
	}
}

func TestGridFitOutline_QuadCurve(t *testing.T) {
	// QuadTo control points should NOT be snapped for X in HintingVertical.
	// But Y baseline snap still applies.
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0.1}}},
			{Op: OutlineOpQuadTo, Points: [3]OutlinePoint{
				{X: 2.5, Y: -4.0}, // control
				{X: 5.0, Y: 0.1},  // end
			}},
		},
		Bounds: Rect{MinX: 0, MinY: -4, MaxX: 5, MaxY: 0.1},
	}

	gridFitOutline(outline, HintingVertical)

	// MoveTo Y=0.1 → 0 (baseline snap)
	if outline.Segments[0].Points[0].Y != 0 {
		t.Errorf("MoveTo baseline Y snap: got %f, want 0", outline.Segments[0].Points[0].Y)
	}
	// QuadTo end Y=0.1 → 0 (baseline snap)
	if outline.Segments[1].Points[1].Y != 0 {
		t.Errorf("QuadTo end baseline Y snap: got %f, want 0", outline.Segments[1].Points[1].Y)
	}
}

func TestExtractOutlineHinted_UnsupportedFont(t *testing.T) {
	e := NewOutlineExtractor()
	_, err := e.ExtractOutlineHinted(nil, 0, 12, HintingFull)
	if err == nil {
		t.Error("expected error for nil font")
	}
}

func TestSegPointCount(t *testing.T) {
	tests := []struct {
		op   OutlineOp
		want int
	}{
		{OutlineOpMoveTo, 1},
		{OutlineOpLineTo, 1},
		{OutlineOpQuadTo, 2},
		{OutlineOpCubicTo, 3},
		{OutlineOp(99), 0},
	}
	for _, tt := range tests {
		if got := segPointCount(tt.op); got != tt.want {
			t.Errorf("segPointCount(%v) = %d, want %d", tt.op, got, tt.want)
		}
	}
}

func TestSegEndY(t *testing.T) {
	tests := []struct {
		seg  OutlineSegment
		want float32
	}{
		{OutlineSegment{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{Y: 1.5}}}, 1.5},
		{OutlineSegment{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{Y: 2.5}}}, 2.5},
		{OutlineSegment{Op: OutlineOpQuadTo, Points: [3]OutlinePoint{{Y: 1}, {Y: 3.5}}}, 3.5},
		{OutlineSegment{Op: OutlineOpCubicTo, Points: [3]OutlinePoint{{Y: 1}, {Y: 2}, {Y: 4.5}}}, 4.5},
	}
	for _, tt := range tests {
		if got := segEndY(&tt.seg); got != tt.want {
			t.Errorf("segEndY(%v) = %f, want %f", tt.seg.Op, got, tt.want)
		}
	}
}

func TestAbs32f(t *testing.T) {
	if got := abs32f(-3.14); math.Abs(float64(got)-3.14) > 1e-6 {
		t.Errorf("abs32f(-3.14) = %f, want 3.14", got)
	}
	if got := abs32f(2.71); math.Abs(float64(got)-2.71) > 1e-6 {
		t.Errorf("abs32f(2.71) = %f, want 2.71", got)
	}
	if got := abs32f(0); got != 0 {
		t.Errorf("abs32f(0) = %f, want 0", got)
	}
}

// makeTestSquare creates a simple square outline for testing.
func makeTestSquare(y0, y1 float32) *GlyphOutline {
	return &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: y0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 5, Y: y0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 5, Y: y1}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 0, Y: y1}}},
		},
		Bounds: Rect{MinX: 0, MinY: float64(y0), MaxX: 5, MaxY: float64(y1)},
	}
}
