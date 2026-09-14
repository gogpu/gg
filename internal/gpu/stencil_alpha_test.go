//go:build !nogpu

package gpu

import (
	"fmt"
	"testing"

	"github.com/gogpu/gg"
)

// Exercise the color all the way from the paint, through tessellation, to
// the cover shader's uniform. This requires no device; opaque colors alone
// would not detect the second alpha multiplication that darkened violins.
func TestStencilCoverPreservesPremultipliedColor(t *testing.T) {
	for _, kind := range []drawCommandKind{drawCmdFillPath, drawCmdStrokePath} {
		for _, alpha := range []float64{0, 0.125, 0.35, 0.5, 0.75, 1} {
			t.Run(fmt.Sprintf("kind=%d/alpha=%g", kind, alpha), func(t *testing.T) {
				path := gg.NewPath()
				path.MoveTo(8, 8)
				path.LineTo(56, 8)
				path.LineTo(40, 24)
				path.LineTo(56, 56)
				path.LineTo(8, 56)
				path.Close()
				fill := gg.RGBA{R: 0.2, G: 0.5, B: 0.8, A: alpha}
				stroke := gg.RGBA{R: 0.9, G: 0.4, B: 0.1, A: alpha}
				paint := gg.NewPaint()
				paint.SetFillBrush(gg.Solid(fill))
				paint.SetStrokeBrush(gg.Solid(stroke))
				paint.SetStroke(gg.Stroke{Width: 4, Cap: gg.LineCapButt, Join: gg.LineJoinMiter, MiterLimit: 10})
				cmd := drawCommand{kind: kind, path: path, paint: *paint}
				rc := &GPURenderContext{}
				source := fill
				if kind == drawCmdStrokePath {
					source = stroke
					rc.preTessellateStroke(&cmd)
				} else {
					rc.preTessellateFill(&cmd)
				}
				if cmd.stencilCmd == nil {
					t.Fatal("the path did not reach stencil-then-cover")
				}
				uniform := makeCoverUniform(64, 64, cmd.stencilCmd.Color)
				want := [4]float32{float32(source.R * alpha), float32(source.G * alpha), float32(source.B * alpha), float32(alpha)}
				for i, expected := range want {
					got := decodeFloat32(uniform[16+i*4 : 20+i*4])
					if testAbs32(got-expected) > 1e-6 {
						t.Errorf("cover channel %d = %g, want %g (one alpha multiplication)", i, got, expected)
					}
				}
			})
		}
	}
}
