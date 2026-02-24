package raster

import (
	"fmt"
	"math"
	"testing"
)

// TestVertexCoverage_PartialAlpha verifies that vertex pixels get correct
// partial coverage through the full pipeline (not maxValue=255).
//
// Regression test for: coverageToRuns used Add() with maxValue=255 for
// middle pixels in merged runs. Two adjacent vertex pixels with the same
// partial coverage (e.g. 44%) would get: first pixel=44% (correct),
// second pixel=100% (WRONG, got maxValue=255 instead of actual alpha).
func TestVertexCoverage_PartialAlpha(t *testing.T) {
	width, height := 400, 300

	af := NewAnalyticFiller(width, height)
	eb := NewEdgeBuilder(4)

	// Triangle with vertex at (200,100)
	path := &vertexTestPath{
		verbs:  []PathVerb{VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose},
		points: []float32{100, 220, 300, 220, 200, 100},
	}
	eb.BuildFromPath(path, IdentityTransform{})

	// Collect alpha for scanline y=100 (vertex row)
	alphas := make(map[int]uint8)
	af.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		if y == 100 {
			for x := 195; x <= 205; x++ {
				a := runs.GetAlpha(x)
				if a > 0 {
					alphas[x] = a
				}
			}
		}
	})

	// Vertex pixels should have partial coverage (~44%), NOT 100%
	for x, alpha := range alphas {
		if alpha > 200 {
			t.Errorf("pixel (%d,100) alpha=%d: vertex pixel should have partial coverage, not near-full", x, alpha)
		}
		if alpha < 50 || alpha > 170 {
			t.Errorf("pixel (%d,100) alpha=%d: expected partial coverage ~100-130 range", x, alpha)
		}
	}

	// Should have exactly 2 vertex pixels with partial coverage
	if len(alphas) < 1 || len(alphas) > 3 {
		t.Errorf("expected 1-3 vertex pixels with coverage, got %d: %v", len(alphas), alphas)
	}
}

// TestEdgePosition_NoDrift verifies that triangle edges don't drift over
// multiple scanlines (the AdvanceX bug).
//
// Regression test for: AdvanceX() advanced line.X by one sub-pixel step
// per pixel scanline, but computeSegmentCoverage already computes positions
// analytically. The accumulated drift caused edges to expand outward,
// progressively worsening toward the bottom of the shape.
func TestEdgePosition_NoDrift(t *testing.T) {
	width, height := 400, 300

	af := NewAnalyticFiller(width, height)
	eb := NewEdgeBuilder(4)

	// Triangle: vertex at (200,50), base at y=250
	// Left edge: (200,50)→(100,250), slope = -100/200 = -0.5 px/row
	// Right edge: (200,50)→(300,250), slope = +100/200 = +0.5 px/row
	path := &vertexTestPath{
		verbs:  []PathVerb{VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose},
		points: []float32{100, 250, 300, 250, 200, 50},
	}
	eb.BuildFromPath(path, IdentityTransform{})

	// Track leftmost and rightmost filled pixels per scanline
	type scanlineInfo struct {
		leftX, rightX int
	}
	scanlines := make(map[int]scanlineInfo)

	af.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		if y < 50 || y >= 250 {
			return
		}
		leftX := -1
		rightX := -1
		for x := 0; x < width; x++ {
			if runs.GetAlpha(x) > 128 { // >50% coverage = "inside"
				if leftX < 0 {
					leftX = x
				}
				rightX = x
			}
		}
		if leftX >= 0 {
			scanlines[y] = scanlineInfo{leftX, rightX}
		}
	})

	// Check several scanlines for correct edge positions
	// At y, exact left edge X = 200 - 0.5*(y-50)
	// At y, exact right edge X = 200 + 0.5*(y-50)
	testRows := []int{60, 100, 150, 200, 240}
	for _, y := range testRows {
		info, ok := scanlines[y]
		if !ok {
			t.Errorf("scanline y=%d: no filled pixels", y)
			continue
		}

		dy := float64(y - 50)
		expectedLeft := 200 - 0.5*dy
		expectedRight := 200 + 0.5*dy

		// Allow 2 pixel tolerance for AA edge transitions
		leftErr := math.Abs(float64(info.leftX) - expectedLeft)
		rightErr := math.Abs(float64(info.rightX) - expectedRight)

		if leftErr > 2 {
			t.Errorf("scanline y=%d: left edge at x=%d, expected ~%.0f (error=%.1f px)",
				y, info.leftX, expectedLeft, leftErr)
		}
		if rightErr > 2 {
			t.Errorf("scanline y=%d: right edge at x=%d, expected ~%.0f (error=%.1f px)",
				y, info.rightX, expectedRight, rightErr)
		}
	}

	// Check that edge error does NOT grow with distance from vertex
	// (This is the key symptom of the AdvanceX drift bug)
	if len(scanlines) > 10 {
		nearTop := scanlines[70]  // 20 rows below vertex
		nearBot := scanlines[230] // 180 rows below vertex

		expectedLeftTop := 200 - 0.5*20.0  // 190
		expectedLeftBot := 200 - 0.5*180.0 // 110

		errTop := math.Abs(float64(nearTop.leftX) - expectedLeftTop)
		errBot := math.Abs(float64(nearBot.leftX) - expectedLeftBot)

		// Error at bottom should NOT be significantly larger than at top
		if errBot > errTop+2 {
			t.Errorf("edge drift detected: error at y=70 is %.1f px, error at y=230 is %.1f px (should not grow)",
				errTop, errBot)
		}
	}
}

// TestCoverageToRuns_MergedRunAlpha verifies that merged runs in coverageToRuns
// preserve the actual alpha value (not maxValue=255).
func TestCoverageToRuns_MergedRunAlpha(t *testing.T) {
	af := NewAnalyticFiller(10, 1)

	// Simulate a scanline where pixels 3,4,5 all have 50% coverage
	for i := range af.coverage {
		af.coverage[i] = 0
	}
	af.coverage[3] = 0.5
	af.coverage[4] = 0.5 // same as pixel 3 — will be merged into a run
	af.coverage[5] = 0.5

	af.coverageToRuns()

	// All three pixels should have alpha=127 (50%), not 255
	for x := 3; x <= 5; x++ {
		alpha := af.alphaRuns.GetAlpha(x)
		if alpha > 200 {
			t.Errorf("pixel %d: alpha=%d, expected ~127 (50%%). Bug: merged run used maxValue=255", x, alpha)
		}
		expected := uint8(127) // 0.5 * 255
		diff := int(alpha) - int(expected)
		if diff < 0 {
			diff = -diff
		}
		if diff > 2 {
			t.Errorf("pixel %d: alpha=%d, expected %d (50%% coverage)", x, alpha, expected)
		}
	}

	// Surrounding pixels should be 0
	if af.alphaRuns.GetAlpha(2) != 0 {
		t.Errorf("pixel 2 should have alpha=0, got %d", af.alphaRuns.GetAlpha(2))
	}
	if af.alphaRuns.GetAlpha(6) != 0 {
		t.Errorf("pixel 6 should have alpha=0, got %d", af.alphaRuns.GetAlpha(6))
	}
}

// TestInteriorPixels_FullCoverage verifies that interior pixels still
// get 100% coverage (the fix must not break fully-covered regions).
func TestInteriorPixels_FullCoverage(t *testing.T) {
	width, height := 100, 100

	af := NewAnalyticFiller(width, height)
	eb := NewEdgeBuilder(4)

	// Large rectangle covering most of the canvas
	path := &vertexTestPath{
		verbs: []PathVerb{VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo, VerbClose},
		points: []float32{
			10, 10, 90, 10, 90, 90, 10, 90,
		},
	}
	eb.BuildFromPath(path, IdentityTransform{})

	af.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		if y >= 20 && y <= 80 {
			// Interior pixels should be fully covered
			for x := 20; x <= 80; x++ {
				alpha := runs.GetAlpha(x)
				if alpha < 250 {
					t.Errorf("interior pixel (%d,%d): alpha=%d, expected 255", x, y, alpha)
					return // don't spam
				}
			}
		}
	})
}

// vertexTestPath implements PathLike for testing.
type vertexTestPath struct {
	verbs  []PathVerb
	points []float32
}

func (p *vertexTestPath) Verbs() []PathVerb { return p.verbs }
func (p *vertexTestPath) Points() []float32 { return p.points }
func (p *vertexTestPath) IsEmpty() bool     { return len(p.verbs) == 0 }
func (p *vertexTestPath) PointCount() int   { return len(p.points) / 2 }

// unused but needed for go vet
var _ = fmt.Println
