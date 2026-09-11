package gpu

import (
	"testing"

	"github.com/gogpu/gg"
)

// TestScissorGroups_KeepSubmissionOrderAcrossTiers verifies that draws of
// different render tiers under one clip stay in the order they were submitted.
//
// A ScissorGroup renders its tiers one after another — SDF shapes, convex
// paths, stencil paths, text, images — so grouping by clip alone put every
// convex fill of a clip before every stencil fill of it, whatever order they
// came in. A painter's-algorithm scene (a 3D surface drawn back to front)
// then had its non-convex faces and its grid strokes land on top of nearer
// convex faces. Groups must split where the tier changes.
func TestScissorGroups_KeepSubmissionOrderAcrossTiers(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()
	target := makeTestTarget(200, 200)

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	quad := func(x0, y0 float64) *gg.Path {
		p := gg.NewPath()
		p.MoveTo(x0, y0)
		p.LineTo(x0+40, y0)
		p.LineTo(x0+40, y0+40)
		p.LineTo(x0, y0+40)
		p.Close()
		return p
	}
	// A concave arrowhead: not convex, so it takes the stencil tier.
	concave := func() *gg.Path {
		p := gg.NewPath()
		p.MoveTo(60, 60)
		p.LineTo(140, 100)
		p.LineTo(60, 140)
		p.LineTo(90, 100)
		p.Close()
		return p
	}

	// Submitted far to near: convex, concave, convex.
	for _, p := range []*gg.Path{quad(10, 10), concave(), quad(120, 120)} {
		if err := rc.FillPath(target, p, paint); err != nil {
			t.Fatalf("FillPath: %v", err)
		}
	}
	if len(rc.pendingDraws) != 3 {
		t.Fatalf("expected 3 pending draws, got %d", len(rc.pendingDraws))
	}
	wantTiers := []renderTier{tierConvex, tierStencil, tierConvex}
	for i, want := range wantTiers {
		if got := drawTier(&rc.pendingDraws[i]); got != want {
			t.Fatalf("draw %d is in tier %d, want %d — the fixture no longer exercises two tiers", i, got, want)
		}
	}

	groups := rc.buildScissorGroupsFromDraws()
	if len(groups) != 3 {
		t.Fatalf("got %d scissor groups for convex, stencil, convex under one clip, want 3 "+
			"so that each renders in its turn", len(groups))
	}
	if len(groups[0].ConvexCommands) != 1 || len(groups[0].StencilPaths) != 0 {
		t.Errorf("group 0 holds %d convex and %d stencil draws, want the first convex fill alone",
			len(groups[0].ConvexCommands), len(groups[0].StencilPaths))
	}
	if len(groups[1].StencilPaths) != 1 || len(groups[1].ConvexCommands) != 0 {
		t.Errorf("group 1 holds %d convex and %d stencil draws, want the concave fill alone",
			len(groups[1].ConvexCommands), len(groups[1].StencilPaths))
	}
	if len(groups[2].ConvexCommands) != 1 || len(groups[2].StencilPaths) != 0 {
		t.Errorf("group 2 holds %d convex and %d stencil draws, want the last convex fill alone",
			len(groups[2].ConvexCommands), len(groups[2].StencilPaths))
	}
}

// TestScissorGroups_SameTierStaysOneGroup verifies that splitting on tier
// changes costs nothing where the tier does not change: a run of convex fills
// under one clip is still one group.
func TestScissorGroups_SameTierStaysOneGroup(t *testing.T) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	rc := s.NewRenderContext()
	defer rc.Close()
	target := makeTestTarget(200, 200)

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	for k := range 5 {
		p := gg.NewPath()
		x := 10 + 35*float64(k)
		p.MoveTo(x, 10)
		p.LineTo(x+30, 10)
		p.LineTo(x+30, 40)
		p.Close()
		if err := rc.FillPath(target, p, paint); err != nil {
			t.Fatalf("FillPath: %v", err)
		}
	}
	if groups := rc.buildScissorGroupsFromDraws(); len(groups) != 1 {
		t.Errorf("five convex fills under one clip made %d groups, want 1", len(groups))
	}
}
