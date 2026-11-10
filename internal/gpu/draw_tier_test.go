//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
)

type drawTierCase struct {
	name string
	cmd  drawCommand
	want renderTier
}

func drawTierCases() []drawTierCase {
	paint := *gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	paint.SetStroke(gg.Stroke{Width: 3})
	shape := gg.DetectedShape{Kind: gg.ShapeRect, CenterX: 20, CenterY: 20, Width: 20, Height: 20}
	points := []gg.Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 0, Y: 20}}
	stencil := &StencilPathCommand{Vertices: []float32{0, 0, 20, 0, 0, 20}}
	cases := make([]drawTierCase, 0, 17)
	cases = append(cases,
		drawTierCase{"text", drawCommand{kind: drawCmdText, textBatch: TextBatch{}}, tierText},
		drawTierCase{"glyph_mask", drawCommand{kind: drawCmdGlyphMaskText, textBatch: GlyphMaskBatch{}}, tierGlyphMask},
		drawTierCase{"image", drawCommand{kind: drawCmdImage, imageCmd: ImageDrawCommand{}}, tierImage},
		drawTierCase{"gpu_texture", drawCommand{kind: drawCmdGPUTexture, gpuTexCmd: GPUTextureDrawCommand{}}, tierGPUTexture},
		drawTierCase{"base_layer", drawCommand{kind: drawCmdBaseLayer, gpuTexCmd: GPUTextureDrawCommand{}}, tierNone},
	)
	for _, kind := range []drawCommandKind{drawCmdFillPath, drawCmdStrokePath} {
		name := "fill_path"
		if kind == drawCmdStrokePath {
			name = "stroke_path"
		}
		cases = append(cases,
			drawTierCase{name + "/convex", drawCommand{kind: kind, paint: paint, convexPoints: points}, tierConvex},
			drawTierCase{name + "/stencil", drawCommand{kind: kind, paint: paint, stencilCmd: stencil}, tierStencil},
			drawTierCase{name + "/empty", drawCommand{kind: kind, paint: paint}, tierNone},
		)
	}
	for _, kind := range []drawCommandKind{drawCmdFillShape, drawCmdStrokeShape} {
		name := "fill_shape"
		if kind == drawCmdStrokeShape {
			name = "stroke_shape"
		}
		// Different fill/stroke alpha also catches reading the wrong brush.
		transparent := paint
		opaque := paint
		if kind == drawCmdFillShape {
			transparent.SetFillBrush(gg.Solid(gg.Transparent))
			opaque.SetStrokeBrush(gg.Solid(gg.Transparent))
		} else {
			transparent.SetStrokeBrush(gg.Solid(gg.Transparent))
			opaque.SetFillBrush(gg.Solid(gg.Transparent))
		}
		cases = append(cases,
			drawTierCase{name + "/opaque", drawCommand{kind: kind, paint: opaque, shape: shape}, tierSDF},
			drawTierCase{name + "/transparent", drawCommand{kind: kind, paint: transparent, shape: shape}, tierNone},
			drawTierCase{name + "/unsupported", drawCommand{kind: kind, paint: paint, shape: gg.DetectedShape{Kind: gg.ShapeUnknown}}, tierNone},
		)
	}
	return cases
}

// Compare the exact destination list, not just whether the group is empty:
// moving a draw between two nonempty tiers can also break painter's order.
func TestDrawTierMatchesScissorGroup(t *testing.T) {
	var rc GPURenderContext
	seen := make(map[drawCommandKind]bool)
	for _, tc := range drawTierCases() {
		seen[tc.cmd.kind] = true
		t.Run(tc.name, func(t *testing.T) {
			g := rc.drawsToScissorGroup([]drawCommand{tc.cmd})
			got := [8]int{
				tierSDF: len(g.SDFShapes), tierConvex: len(g.ConvexCommands),
				tierStencil: len(g.StencilPaths), tierText: len(g.TextBatches),
				tierGlyphMask: len(g.GlyphMaskBatches), tierImage: len(g.ImageCommands),
				tierGPUTexture: len(g.GPUTextureCommands),
			}
			var want [8]int
			if tc.want != tierNone {
				want[tc.want] = 1
			}
			if got != want {
				t.Errorf("destination list counts = %v, want %v", got, want)
			}
			if tier := drawTier(&tc.cmd); tier != tc.want {
				t.Errorf("drawTier = %d, want %d", tier, tc.want)
			}
		})
	}
	// Adding a command kind must also add a representative fixture here.
	for kind := drawCommandKind(0); kind < drawCmdCount; kind++ {
		if !seen[kind] {
			t.Errorf("draw command kind %d has no tier consistency fixture", kind)
		}
	}
}

func TestScissorGroups_IgnoredDrawsDoNotSplitRun(t *testing.T) {
	convex := drawCommand{kind: drawCmdFillPath, convexPoints: []gg.Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 0, Y: 20}}}
	for _, tc := range drawTierCases() {
		if tc.want != tierNone {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			// Leading, intervening and trailing ignored draws must all preserve
			// a single run. The leading draw also exercises tierNone adoption.
			rc := GPURenderContext{pendingDraws: []drawCommand{tc.cmd, convex, tc.cmd, convex, tc.cmd}}
			groups := rc.buildScissorGroupsFromDraws()
			if len(groups) != 1 {
				t.Fatalf("ignored draws split a convex run into %d groups, want 1", len(groups))
			}
			if len(groups[0].ConvexCommands) != 2 {
				t.Errorf("got %d convex commands, want 2", len(groups[0].ConvexCommands))
			}
		})
	}
}
