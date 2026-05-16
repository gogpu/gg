// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"testing"
)

// buildRectPath creates a simple rectangle path for testing.
func buildRectPath(x, y, w, h float32) PathLike {
	return &testPath{
		verbs: []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{
			x, y, // moveTo
			x + w, y, // lineTo
			x + w, y + h, // lineTo
			x, y + h, // lineTo
		},
	}
}

// buildCirclePath creates a circle approximation using 4 quadratic beziers.
func buildCirclePath(cx, cy, r float32) PathLike {
	// Approximate circle with 8 line segments.
	const n = 8
	verbs := make([]PathVerb, 0, n+2)
	points := make([]float32, 0, (n+1)*2)

	verbs = append(verbs, MoveTo)
	points = append(points, cx+r, cy)

	for i := 1; i <= n; i++ {
		angle := float32(i) * 2.0 * 3.14159265 / float32(n)
		x := cx + r*cos32(angle)
		y := cy + r*sin32(angle)
		verbs = append(verbs, LineTo)
		points = append(points, x, y)
	}
	verbs = append(verbs, Close)

	return &testPath{verbs: verbs, points: points}
}

func cos32(x float32) float32 {
	// Simple Taylor series for testing (accurate enough for coarse circle).
	x2 := x * x
	return 1.0 - x2/2.0 + x2*x2/24.0 - x2*x2*x2/720.0
}

func sin32(x float32) float32 {
	x2 := x * x
	return x - x*x2/6.0 + x*x2*x2/120.0 - x*x2*x2*x2/5040.0
}

// testPath is already defined in analytic_filler_test.go within this package.

func TestNoAAFiller_AxisAlignedRect(t *testing.T) {
	// A 10x10 rectangle at (5,5) should fill pixels [5,14] x [5,14]
	// with ALL pixels having full coverage (255). NO gray edge pixels.
	const w, h = 20, 20
	filler := NewNoAAFiller(w, h)
	eb := NewEdgeBuilder(0) // aaShift=0 for non-AA
	eb.SetFlattenCurves(true)

	path := buildRectPath(5, 5, 10, 10)
	eb.BuildFromPath(path, IdentityTransform{})

	// Collect all blitted spans.
	type span struct{ y, left, width int }
	var spans []span
	filler.Fill(eb, FillRuleNonZero, func(y, left, width int) {
		spans = append(spans, span{y, left, width})
	})

	// Verify: should have 10 scanlines (y=5..14), each spanning [5, 15).
	if len(spans) != 10 {
		t.Fatalf("expected 10 spans, got %d", len(spans))
	}
	for i, s := range spans {
		expectedY := 5 + i
		if s.y != expectedY {
			t.Errorf("span[%d]: y=%d, expected %d", i, s.y, expectedY)
		}
		if s.left != 5 {
			t.Errorf("span[%d]: left=%d, expected 5", i, s.left)
		}
		if s.width != 10 {
			t.Errorf("span[%d]: width=%d, expected 10", i, s.width)
		}
	}
}

func TestNoAAFiller_NoCoverageOutside(t *testing.T) {
	// Verify that a rectangle produces NO spans outside its bounds.
	const w, h = 50, 50
	filler := NewNoAAFiller(w, h)
	eb := NewEdgeBuilder(0)
	eb.SetFlattenCurves(true)

	path := buildRectPath(10, 10, 5, 5)
	eb.BuildFromPath(path, IdentityTransform{})

	filler.Fill(eb, FillRuleNonZero, func(y, left, width int) {
		if y < 10 || y > 14 {
			t.Errorf("span at y=%d is outside rect bounds [10, 14]", y)
		}
		if left < 10 || left+width > 15 {
			t.Errorf("span at y=%d: [%d, %d) exceeds rect bounds [10, 15)", y, left, left+width)
		}
	})
}

func TestNoAAFiller_CircleBinaryCoverage(t *testing.T) {
	// A circle should produce spans with only full coverage.
	// No gray/fractional pixels — that's the point of no-AA.
	const w, h = 100, 100
	filler := NewNoAAFiller(w, h)
	eb := NewEdgeBuilder(0)
	eb.SetFlattenCurves(true)

	path := buildCirclePath(50, 50, 20)
	eb.BuildFromPath(path, IdentityTransform{})

	spanCount := 0
	filler.Fill(eb, FillRuleNonZero, func(y, left, width int) {
		spanCount++
		if width <= 0 {
			t.Errorf("span at y=%d has zero/negative width %d", y, width)
		}
		if left < 0 || left+width > w {
			t.Errorf("span at y=%d: [%d, %d) exceeds canvas bounds [0, %d)", y, left, left+width, w)
		}
	})

	if spanCount == 0 {
		t.Error("circle produced no spans")
	}
}

func TestNoAAFiller_EvenOddFillRule(t *testing.T) {
	// Two concentric rectangles with EvenOdd should produce a frame (hole in center).
	const w, h = 30, 30
	filler := NewNoAAFiller(w, h)
	eb := NewEdgeBuilder(0)
	eb.SetFlattenCurves(true)

	// Outer rect: 5,5 to 25,25
	// Inner rect: 10,10 to 20,20 (wound same direction — EvenOdd makes hole)
	outerInner := &testPath{
		verbs: []PathVerb{
			MoveTo, LineTo, LineTo, LineTo, Close,
			MoveTo, LineTo, LineTo, LineTo, Close,
		},
		points: []float32{
			5, 5, 25, 5, 25, 25, 5, 25, // outer
			10, 10, 20, 10, 20, 20, 10, 20, // inner
		},
	}
	eb.BuildFromPath(outerInner, IdentityTransform{})

	// Track which pixels are filled.
	filled := make([][]bool, h)
	for i := range filled {
		filled[i] = make([]bool, w)
	}

	filler.Fill(eb, FillRuleEvenOdd, func(y, left, width int) {
		for x := left; x < left+width; x++ {
			if x >= 0 && x < w && y >= 0 && y < h {
				filled[y][x] = true
			}
		}
	})

	// Verify outer area is filled.
	for y := 5; y < 10; y++ {
		for x := 5; x < 25; x++ {
			if !filled[y][x] {
				t.Errorf("pixel (%d,%d) should be filled (outer frame)", x, y)
				return
			}
		}
	}

	// Verify inner area is NOT filled (EvenOdd hole).
	for y := 10; y < 20; y++ {
		for x := 10; x < 20; x++ {
			if filled[y][x] {
				t.Errorf("pixel (%d,%d) should NOT be filled (EvenOdd hole)", x, y)
				return
			}
		}
	}
}

func TestNoAAFiller_NonZeroFillRule(t *testing.T) {
	// Same concentric rects with NonZero — inner should be filled (same winding).
	const w, h = 30, 30
	filler := NewNoAAFiller(w, h)
	eb := NewEdgeBuilder(0)
	eb.SetFlattenCurves(true)

	outerInner := &testPath{
		verbs: []PathVerb{
			MoveTo, LineTo, LineTo, LineTo, Close,
			MoveTo, LineTo, LineTo, LineTo, Close,
		},
		points: []float32{
			5, 5, 25, 5, 25, 25, 5, 25,
			10, 10, 20, 10, 20, 20, 10, 20,
		},
	}
	eb.BuildFromPath(outerInner, IdentityTransform{})

	filled := make([][]bool, h)
	for i := range filled {
		filled[i] = make([]bool, w)
	}

	filler.Fill(eb, FillRuleNonZero, func(y, left, width int) {
		for x := left; x < left+width; x++ {
			if x >= 0 && x < w && y >= 0 && y < h {
				filled[y][x] = true
			}
		}
	})

	// With NonZero and same-direction winding, inner should be filled.
	for y := 10; y < 20; y++ {
		for x := 10; x < 20; x++ {
			if !filled[y][x] {
				t.Errorf("pixel (%d,%d) should be filled (NonZero, same winding)", x, y)
				return
			}
		}
	}
}

func TestNoAAFiller_EmptyPath(t *testing.T) {
	filler := NewNoAAFiller(100, 100)
	eb := NewEdgeBuilder(0)

	// Empty path — no edges.
	filler.Fill(eb, FillRuleNonZero, func(y, left, width int) {
		t.Error("should not produce spans for empty path")
	})
}

func TestNoAAFiller_PathOutsideBounds(t *testing.T) {
	// Rectangle entirely outside canvas bounds.
	filler := NewNoAAFiller(10, 10)
	eb := NewEdgeBuilder(0)
	eb.SetFlattenCurves(true)

	path := buildRectPath(20, 20, 10, 10) // completely outside [0,10)
	eb.BuildFromPath(path, IdentityTransform{})

	filler.Fill(eb, FillRuleNonZero, func(y, left, width int) {
		t.Errorf("should not produce spans for path outside bounds: y=%d left=%d w=%d", y, left, width)
	})
}

func TestNoAAFiller_SinglePixelRect(t *testing.T) {
	// A 1x1 pixel rectangle should produce exactly 1 span.
	filler := NewNoAAFiller(10, 10)
	eb := NewEdgeBuilder(0)
	eb.SetFlattenCurves(true)

	path := buildRectPath(3, 3, 1, 1)
	eb.BuildFromPath(path, IdentityTransform{})

	spans := 0
	filler.Fill(eb, FillRuleNonZero, func(y, left, width int) {
		spans++
		if y != 3 || left != 3 || width != 1 {
			t.Errorf("expected span (y=3, left=3, w=1), got (y=%d, left=%d, w=%d)", y, left, width)
		}
	})
	if spans != 1 {
		t.Errorf("expected 1 span, got %d", spans)
	}
}

func TestFixedRoundToInt(t *testing.T) {
	tests := []struct {
		name  string
		input FDot16
		want  int
	}{
		{"zero", 0, 0},
		{"one", FDot16One, 1},
		{"half_rounds_up", FDot16Half, 1},
		{"just_below_half", FDot16Half - 1, 0},
		{"negative_one", -FDot16One, -1},
		{"ten", 10 * FDot16One, 10},
		{"quarter", FDot16One / 4, 0},
		{"three_quarters", FDot16One * 3 / 4, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixedRoundToInt(tt.input)
			if got != tt.want {
				t.Errorf("fixedRoundToInt(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
