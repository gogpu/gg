//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg/internal/raster"
)

// TestGPUAnalyticFiller_MergedRunAlpha verifies that merged runs in
// coverageToRuns preserve the actual alpha value (not maxValue=255).
//
// Regression test for: coverageToRuns used Add() with maxValue=255 for
// middle pixels. Runs of partial-coverage pixels would get alpha=255
// instead of the actual coverage value.
func TestGPUAnalyticFiller_MergedRunAlpha(t *testing.T) {
	af := NewAnalyticFiller(10, 1)

	// Simulate pixels 3,4,5 all having 50% coverage
	for i := range af.coverage {
		af.coverage[i] = 0
	}
	af.coverage[3] = 0.5
	af.coverage[4] = 0.5
	af.coverage[5] = 0.5

	af.coverageToRuns()

	for x := 3; x <= 5; x++ {
		alpha := af.alphaRuns.GetAlpha(x)
		if alpha > 200 {
			t.Errorf("pixel %d: alpha=%d, expected ~127 (50%%). Bug: merged run used maxValue=255", x, alpha)
		}
		expected := uint8(127)
		diff := int(alpha) - int(expected)
		if diff < 0 {
			diff = -diff
		}
		if diff > 2 {
			t.Errorf("pixel %d: alpha=%d, expected %d (50%% coverage)", x, alpha, expected)
		}
	}

	if af.alphaRuns.GetAlpha(2) != 0 {
		t.Errorf("pixel 2 should have alpha=0, got %d", af.alphaRuns.GetAlpha(2))
	}
	if af.alphaRuns.GetAlpha(6) != 0 {
		t.Errorf("pixel 6 should have alpha=0, got %d", af.alphaRuns.GetAlpha(6))
	}
}

// TestGPUAnalyticFillerVello_MergedRunAlpha verifies the same fix in the
// Vello-style analytic filler. Tests the alpha run emission logic by
// preparing area buffer directly and invoking the fill rule + run emission.
func TestGPUAnalyticFillerVello_MergedRunAlpha(t *testing.T) {
	af := NewAnalyticFillerVello(10, 1)

	// Prepare area buffer with 50% coverage for pixels 3,4,5
	// then invoke processScanlineVello with empty segments —
	// it clears area, so we test via Fill with a real path instead.
	//
	// Alternative: test the run emission by calling Fill with a wide shape
	// that produces partial-coverage runs.
	eb := raster.NewEdgeBuilder(4)

	// Horizontal band: top=0, bottom=0.5 — gives 50% coverage across full width
	path := &gpuTestPath{
		verbs:  []raster.PathVerb{raster.VerbMoveTo, raster.VerbLineTo, raster.VerbLineTo, raster.VerbLineTo, raster.VerbClose},
		points: []float32{2, 0, 8, 0, 8, 0.5, 2, 0.5},
	}
	eb.BuildFromPath(path, raster.IdentityTransform{})

	alphas := make(map[int]uint8)
	af.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		if y == 0 {
			for x := 2; x <= 8; x++ {
				a := runs.GetAlpha(x)
				if a > 0 {
					alphas[x] = a
				}
			}
		}
	})

	// Interior pixels (3-7) should all have the same partial alpha
	// If the Add() bug exists, first pixel would be correct but rest would be 255
	for x := 3; x <= 7; x++ {
		alpha, ok := alphas[x]
		if !ok {
			continue
		}
		if alpha > 200 {
			t.Errorf("pixel %d: alpha=%d, expected partial coverage. Bug: merged run used maxValue=255", x, alpha)
		}
	}

	// Check consistency: all interior pixels should have similar alpha
	checkAlphaConsistency(t, alphas)
}

func checkAlphaConsistency(t *testing.T, alphas map[int]uint8) {
	t.Helper()
	if len(alphas) < 3 {
		return
	}
	var first uint8
	for x := 3; x <= 7; x++ {
		a, ok := alphas[x]
		if !ok {
			continue
		}
		if first == 0 {
			first = a
			continue
		}
		diff := int(a) - int(first)
		if diff < 0 {
			diff = -diff
		}
		if diff > 5 {
			t.Errorf("pixel %d: alpha=%d differs from first pixel alpha=%d by %d (should be consistent)", x, a, first, diff)
		}
	}
}

// gpuTestPath implements raster.PathLike for testing.
type gpuTestPath struct {
	verbs  []raster.PathVerb
	points []float32
}

func (p *gpuTestPath) Verbs() []raster.PathVerb { return p.verbs }
func (p *gpuTestPath) Points() []float32        { return p.points }
func (p *gpuTestPath) IsEmpty() bool            { return len(p.verbs) == 0 }
func (p *gpuTestPath) PointCount() int          { return len(p.points) / 2 }
