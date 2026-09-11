package gpu

import (
	"testing"

	"github.com/gogpu/gg"
)

// TestStrokePath_OutlineNeverTakesConvexFastPath verifies that an expanded
// stroke outline goes to stencil-then-cover, not to the convex fast path.
//
// The convex path draws its polygon as one triangle fan with no stencil, so a
// concave polygon that is accepted anyway renders as its hull. The outline of
// an open stroke with a corner in it is accepted: the inner join pivots
// through the centerline point in a small loop that turns the same way as the
// outer corners, so every cross product has one sign. Filled NonZero, an L —
// one corner of a step chart — came out as a solid wedge between its arms.
//
// The software dispatch fills cmd.path and never sees convexPoints, which is
// why the tier is asserted here rather than compared in pixels.
func TestStrokePath_OutlineNeverTakesConvexFastPath(t *testing.T) {
	polylines := map[string][][2]float64{
		"L":         {{517, 188}, {517, 46}, {639, 46}},
		"step":      {{214, 34}, {214, 388}, {305, 388}, {305, 388}, {376, 388}},
		"collinear": {{33, 34}, {154, 34}, {154, 34}, {214, 34}},
	}
	for name, pts := range polylines {
		t.Run(name, func(t *testing.T) {
			path := gg.NewPath()
			path.MoveTo(pts[0][0], pts[0][1])
			for _, p := range pts[1:] {
				path.LineTo(p[0], p[1])
			}
			paint := gg.NewPaint()
			paint.SetBrush(gg.Solid(gg.Red))
			paint.SetStroke(gg.Stroke{Width: 1.75, Cap: gg.LineCapButt, Join: gg.LineJoinMiter, MiterLimit: 4})

			cmd := drawCommand{kind: drawCmdStrokePath, path: path, paint: *paint}
			rc := &GPURenderContext{}
			rc.preTessellateStroke(&cmd)

			if cmd.convexPoints != nil {
				t.Error("the stroke outline took the convex fast path, which fills it as its hull")
			}
			if cmd.stencilCmd == nil {
				t.Fatal("the stroke outline did not reach stencil-then-cover")
			}
			if cmd.stencilCmd.FillRule != gg.FillRuleNonZero {
				t.Errorf("stencil fill rule = %v, want NonZero", cmd.stencilCmd.FillRule)
			}
		})
	}
}
