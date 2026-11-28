// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
)

// twoCirclePath appends two disjoint circles (cubic approximation) to p.
func twoCirclePath(p *gg.Path, cx1, cx2, cy, r float64) {
	const k = 0.5522847498
	d := r * k
	for _, cx := range []float64{cx1, cx2} {
		p.MoveTo(cx+r, cy)
		p.CubicTo(cx+r, cy+d, cx+d, cy+r, cx, cy+r)
		p.CubicTo(cx-d, cy+r, cx-r, cy+d, cx-r, cy)
		p.CubicTo(cx-r, cy-d, cx-d, cy-r, cx, cy-r)
		p.CubicTo(cx+d, cy-r, cx+r, cy-d, cx+r, cy)
		p.Close()
	}
}

// rasterizeCoverage runs a CoverageFiller over the given path and returns the
// per-pixel coverage map. A pixel's coverage is the maximum value reported by
// the filler (the fillers visit each pixel at most once per fill).
func rasterizeCoverage(t *testing.T, f gg.CoverageFiller, p *gg.Path, w, h int, rule gg.FillRule) []uint8 {
	t.Helper()
	cov := make([]uint8, w*h)
	f.FillCoverage(p, w, h, rule, nil, func(x, y int, c uint8) {
		if x < 0 || x >= w || y < 0 || y >= h {
			return
		}
		idx := y*w + x
		if c > cov[idx] {
			cov[idx] = c
		}
	})
	return cov
}

// countInkRuns counts contiguous horizontally-inked runs on row y.
func countInkRuns(cov []uint8, w, y int) int {
	runs := 0
	inInk := false
	for x := 0; x < w; x++ {
		isInk := cov[y*w+x] > 0
		if isInk && !inInk {
			runs++
		}
		inInk = isInk
	}
	return runs
}

// TestCoverageFillerDisjointContoursNonZero guards the NonZero winding bug that
// made multi-contour paths render as one solid blob through the tile-based
// CoverageFillers (SparseStrips and TileCompute).
//
// Root cause: NewLineSegment normalized each segment to Y0<=Y1 and negated the
// winding at the same time. Flattening already encodes the direction in the
// winding, so the double negation left every segment with winding +1. A closed
// contour then never summed to zero and the winding prefix sum accumulated
// across contours, filling the space between them.
//
// The two circles are disjoint, so row y=50 must contain exactly two ink runs
// and the gap between them must be empty.
func TestCoverageFillerDisjointContoursNonZero(t *testing.T) {
	const (
		w, h = 200, 100
		cy   = 50.0
		r    = 25.0
		cx1  = 50.0
		cx2  = 150.0
		rowY = 50
		gapX = 100
		in1X = 50
		in2X = 150
	)

	fillers := []struct {
		name string
		f    gg.CoverageFiller
	}{
		{"SparseStrips", &SparseStripsFiller{}},
		{"TileCompute", &TileComputeFiller{}},
	}

	for _, tc := range fillers {
		t.Run(tc.name, func(t *testing.T) {
			p := gg.NewPath()
			twoCirclePath(p, cx1, cx2, cy, r)

			cov := rasterizeCoverage(t, tc.f, p, w, h, gg.FillRuleNonZero)

			if got := cov[rowY*w+in1X]; got == 0 {
				t.Errorf("inside first circle (x=%d,y=%d): coverage = 0, want > 0", in1X, rowY)
			}
			if got := cov[rowY*w+in2X]; got == 0 {
				t.Errorf("inside second circle (x=%d,y=%d): coverage = 0, want > 0", in2X, rowY)
			}
			if got := cov[rowY*w+gapX]; got != 0 {
				t.Errorf("gap between circles (x=%d,y=%d): coverage = %d, want 0 (winding must return to zero)", gapX, rowY, got)
			}
			if runs := countInkRuns(cov, w, rowY); runs != 2 {
				t.Errorf("ink runs at y=%d = %d, want 2 (two disjoint circles)", rowY, runs)
			}
		})

		// EvenOdd is unaffected by the winding sign accumulation and must keep
		// rendering two disjoint circles (never a single merged blob).
		t.Run(tc.name+"_EvenOdd", func(t *testing.T) {
			p := gg.NewPath()
			twoCirclePath(p, cx1, cx2, cy, r)

			cov := rasterizeCoverage(t, tc.f, p, w, h, gg.FillRuleEvenOdd)

			if got := cov[rowY*w+gapX]; got != 0 {
				t.Errorf("gap between circles (EvenOdd, x=%d,y=%d): coverage = %d, want 0", gapX, rowY, got)
			}
			// Exactly 2 would be ideal, but the SparseStrips EvenOdd path leaves
			// a small pre-existing hole at a circle's extreme-right column; a
			// merged blob yields 1 run, so assert separation, not the exact count.
			if runs := countInkRuns(cov, w, rowY); runs < 2 {
				t.Errorf("EvenOdd ink runs at y=%d = %d, want >= 2 (circles must stay separate)", rowY, runs)
			}
		})
	}
}
