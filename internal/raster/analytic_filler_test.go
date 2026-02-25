// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"math"
	"testing"
)

// testPath implements PathLike for testing.
type testPath struct {
	verbs  []PathVerb
	points []float32
}

func (p *testPath) IsEmpty() bool     { return len(p.verbs) == 0 }
func (p *testPath) Verbs() []PathVerb { return p.verbs }
func (p *testPath) Points() []float32 { return p.points }

// makeCirclePath creates a circle path using 4 cubic Bezier curves.
// This matches the standard SVG/Canvas circle approximation.
func makeCirclePath(cx, cy, r float64) *testPath {
	const kappa = 0.5522847498 // 4*(sqrt(2)-1)/3
	k := r * kappa

	p := &testPath{}

	// Start at right (3 o'clock)
	startX, startY := cx+r, cy
	p.verbs = append(p.verbs, VerbMoveTo)
	p.points = append(p.points, float32(startX), float32(startY))

	// Right to bottom
	p.verbs = append(p.verbs, VerbCubicTo)
	p.points = append(p.points,
		float32(cx+r), float32(cy+k),
		float32(cx+k), float32(cy+r),
		float32(cx), float32(cy+r))

	// Bottom to left
	p.verbs = append(p.verbs, VerbCubicTo)
	p.points = append(p.points,
		float32(cx-k), float32(cy+r),
		float32(cx-r), float32(cy+k),
		float32(cx-r), float32(cy))

	// Left to top
	p.verbs = append(p.verbs, VerbCubicTo)
	p.points = append(p.points,
		float32(cx-r), float32(cy-k),
		float32(cx-k), float32(cy-r),
		float32(cx), float32(cy-r))

	// Top to right
	p.verbs = append(p.verbs, VerbCubicTo)
	p.points = append(p.points,
		float32(cx+k), float32(cy-r),
		float32(cx+r), float32(cy-k),
		float32(cx+r), float32(cy))

	p.verbs = append(p.verbs, VerbClose)

	return p
}

// TestOffCanvasCircleClipping verifies that circles partially or fully
// off-screen are clipped correctly without producing horizontal bands.
//
// This is a regression test for the X-bounds clipping bug where edges
// extending beyond the canvas in X caused incorrect winding accumulation,
// producing full-width horizontal color bands during window resize.
func TestOffCanvasCircleClipping(t *testing.T) {
	tests := []struct {
		name         string
		canvasW      int
		canvasH      int
		circleX      float64 // circle center X
		circleY      float64 // circle center Y
		circleR      float64 // circle radius
		checkY       int     // scanline to check
		expectFilled bool    // should the center of this scanline be filled?
	}{
		{
			name:         "circle fully on-screen",
			canvasW:      200,
			canvasH:      200,
			circleX:      100,
			circleY:      100,
			circleR:      50,
			checkY:       100,
			expectFilled: true,
		},
		{
			name:         "circle partially off-screen left",
			canvasW:      100,
			canvasH:      200,
			circleX:      -30,
			circleY:      100,
			circleR:      50,
			checkY:       100,
			expectFilled: true, // center of circle is at x=-30, but right edge at x=20 is visible
		},
		{
			name:         "circle partially off-screen right",
			canvasW:      100,
			canvasH:      200,
			circleX:      130,
			circleY:      100,
			circleR:      50,
			checkY:       100,
			expectFilled: true, // left edge at x=80 is visible
		},
		{
			name:         "circle center on-screen but extends both sides",
			canvasW:      100,
			canvasH:      200,
			circleX:      50,
			circleY:      100,
			circleR:      150,
			checkY:       100,
			expectFilled: true, // canvas is entirely inside the circle
		},
		{
			name:         "circle fully off-screen left",
			canvasW:      100,
			canvasH:      200,
			circleX:      -100,
			circleY:      100,
			circleR:      50,
			checkY:       100,
			expectFilled: false,
		},
		{
			name:         "circle fully off-screen right",
			canvasW:      100,
			canvasH:      200,
			circleX:      200,
			circleY:      100,
			circleR:      50,
			checkY:       100,
			expectFilled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := makeCirclePath(tt.circleX, tt.circleY, tt.circleR)

			eb := NewEdgeBuilder(2) // aaShift=2 (4x AA)
			eb.BuildFromPath(path, IdentityTransform{})

			filler := NewAnalyticFiller(tt.canvasW, tt.canvasH)

			// Collect scanline coverage
			var scanlineCoverage []float32
			filler.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
				if y == tt.checkY {
					// Read coverage values from alpha runs
					scanlineCoverage = make([]float32, tt.canvasW)
					for x := 0; x < tt.canvasW; x++ {
						scanlineCoverage[x] = float32(runs.GetAlpha(x)) / 255.0
					}
				}
			})

			if scanlineCoverage == nil {
				t.Fatalf("scanline %d was never processed", tt.checkY)
			}

			// Check center pixel of the scanline
			centerX := tt.canvasW / 2

			// For "no horizontal bands" check: verify that coverage transitions
			// are smooth, not abrupt full-width fills
			if tt.expectFilled {
				// The circle should cover the center of the visible scanline
				// (for partially off-screen circles, the visible portion should be filled)
				pixelInCircle := isPixelInCircle(centerX, tt.checkY, tt.circleX, tt.circleY, tt.circleR)
				if pixelInCircle && scanlineCoverage[centerX] < 0.5 {
					t.Errorf("center pixel (%d, %d) should be filled (coverage=%.3f), circle at (%.0f, %.0f) r=%.0f",
						centerX, tt.checkY, scanlineCoverage[centerX], tt.circleX, tt.circleY, tt.circleR)
				}
			} else {
				// Circle is entirely off-screen, ALL pixels should have zero or near-zero coverage
				for x := 0; x < tt.canvasW; x++ {
					if scanlineCoverage[x] > 0.01 {
						t.Errorf("pixel (%d, %d) should be empty (coverage=%.3f) but circle is fully off-screen",
							x, tt.checkY, scanlineCoverage[x])
						break
					}
				}
			}

			// KEY REGRESSION CHECK: no full-width horizontal bands.
			// The bug manifested as ALL pixels on a scanline having identical
			// non-zero coverage, even pixels far from the circle.
			// Check that pixels far from the circle have zero coverage.
			if tt.circleX > 0 && tt.circleX < float64(tt.canvasW) {
				// Circle center is on-screen — check that pixels far outside have low coverage
				for x := 0; x < tt.canvasW; x++ {
					dist := math.Sqrt(float64(x-int(tt.circleX))*float64(x-int(tt.circleX)) +
						float64(tt.checkY-int(tt.circleY))*float64(tt.checkY-int(tt.circleY)))
					if dist > tt.circleR+2 && scanlineCoverage[x] > 0.1 {
						t.Errorf("pixel (%d, %d) is %.1f px from circle edge but has coverage=%.3f (should be ~0); possible horizontal band artifact",
							x, tt.checkY, dist-tt.circleR, scanlineCoverage[x])
						break
					}
				}
			}
		})
	}
}

// TestOffCanvasEdgeBandRegression specifically tests the scenario that caused
// the horizontal banding bug: a circle centered at (width/2, height/2) with
// radius > width/2, rendered on a narrow canvas.
func TestOffCanvasEdgeBandRegression(t *testing.T) {
	// Simulate narrow window: 100px wide, circle radius 150
	// This is the exact scenario from the bug report
	width, height := 100, 300
	cx, cy, r := float64(width)/2, float64(height)/2, 150.0

	path := makeCirclePath(cx, cy, r)
	eb := NewEdgeBuilder(2)
	eb.BuildFromPath(path, IdentityTransform{})

	filler := NewAnalyticFiller(width, height)

	bandDetected := false
	filler.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		// Skip scanlines outside circle's Y range
		if float64(y) < cy-r || float64(y) > cy+r {
			return
		}

		// At this Y, compute expected circle X range
		dy := float64(y) - cy
		discriminant := r*r - dy*dy
		if discriminant <= 0 {
			return
		}
		halfChord := math.Sqrt(discriminant)

		// Only check scanlines where the circle is wide enough that
		// anti-aliasing margin won't cause false positives.
		// Near the top/bottom of the circle, edges are very steep and
		// AA can extend several pixels beyond the geometric boundary.
		if halfChord < 20 {
			return
		}

		expectedLeft := cx - halfChord
		expectedRight := cx + halfChord

		// Use a generous margin (5px) to avoid false positives from AA
		margin := 5.0

		// Check for band: pixels far outside circle should have zero coverage
		outsideCount := 0
		outsideFilledCount := 0
		for x := 0; x < width; x++ {
			alpha := runs.GetAlpha(x)
			xf := float64(x) + 0.5 // pixel center

			if xf < expectedLeft-margin || xf > expectedRight+margin {
				outsideCount++
				if alpha > 25 { // > 10% coverage
					outsideFilledCount++
				}
			}
		}

		// If more than 50% of outside pixels are filled, it's a band
		if outsideCount > 0 && outsideFilledCount*2 > outsideCount {
			t.Errorf("scanline %d: horizontal band detected — %d/%d pixels outside circle have coverage (expectedLeft=%.1f, expectedRight=%.1f)",
				y, outsideFilledCount, outsideCount, expectedLeft, expectedRight)
			bandDetected = true
		}
	})

	if bandDetected {
		t.Error("REGRESSION: horizontal banding artifact detected during off-canvas circle clipping")
	}
}

func isPixelInCircle(px, py int, cx, cy, r float64) bool {
	dx := float64(px) + 0.5 - cx
	dy := float64(py) + 0.5 - cy
	return dx*dx+dy*dy < r*r
}
