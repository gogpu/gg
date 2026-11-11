//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/gpu/tilecompute"
)

func thinRoundRectStroke() (*gg.Path, *gg.Paint) {
	p := gg.NewPath()
	p.RoundedRectangle(40, 40, 300, 120, 12) // Exact geometry from #369.
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	paint.SetStroke(gg.Stroke{Width: 1, Join: gg.LineJoinMiter, MiterLimit: 4})
	paint.FillRule = gg.FillRuleEvenOdd // Stroke must ignore the caller's fill rule.
	return p, paint
}

// Assert the entire safe interior, plus ink on all four edges. An empty
// image must not pass a test whose purpose is to keep a stroke hollow.
func assertHollowRoundRect(t *testing.T, target gg.GPURenderTarget) {
	t.Helper()
	for y := 54; y < 146; y++ {
		for x := 54; x < 326; x++ {
			if a := target.Data[y*target.Stride+x*4+3]; a != 0 {
				t.Fatalf("interior pixel (%d,%d) has alpha %d, want 0", x, y, a)
			}
		}
	}
	for _, center := range [][2]int{{190, 40}, {190, 160}, {40, 100}, {340, 100}} {
		ink := 0
		for y := center[1] - 2; y <= center[1]+2; y++ {
			for x := center[0] - 2; x <= center[0]+2; x++ {
				ink += int(target.Data[y*target.Stride+x*4+3])
			}
		}
		if ink == 0 {
			t.Errorf("stroke edge near (%d,%d) is empty", center[0], center[1])
		}
	}
}

// Always-run check of the exact outlines queued by the two GPU paths.
// Real stencil/compute dispatch is additionally tested below when available.
func TestStrokeRoundRect_QueuedOutlinesStayHollow(t *testing.T) {
	t.Run("stencil_outline", func(t *testing.T) {
		path, paint := thinRoundRectStroke()
		s := NewGPUShared()
		s.strategy = strategyRasterAtlas
		s.deviceReady = true
		rc := s.NewRenderContext()
		defer rc.Close()
		target := makeTestTarget(384, 208)
		if err := rc.StrokePath(target, path, paint); err != nil {
			t.Fatal(err)
		}
		if len(rc.pendingDraws) != 1 || rc.pendingDraws[0].stencilCmd == nil || rc.pendingDraws[0].convexPoints != nil {
			t.Fatal("round-rect stroke did not exclusively reach stencil-then-cover")
		}
		if rc.pendingDraws[0].stencilCmd.FillRule != gg.FillRuleNonZero {
			t.Fatal("stencil outline must use NonZero")
		}
		pm := gg.NewPixmap(target.Width, target.Height)
		rc.dispatchDrawsToSoftware(pm, gg.NewSoftwareRenderer(target.Width, target.Height))
		target.Data = pm.Data()
		assertHollowRoundRect(t, target)
	})
	t.Run("vello_outline", func(t *testing.T) {
		path, paint := thinRoundRectStroke()
		a := &VelloAccelerator{gpuReady: true, antiAlias: true}
		target := makeTestTarget(384, 208)
		if err := a.StrokePath(target, path, paint); err != nil {
			t.Fatal(err)
		}
		if len(a.pendingPaths) != 1 || a.pendingPaths[0].FillRule != tilecompute.FillRuleNonZero {
			t.Fatal("Vello outline must use NonZero")
		}
		img := tilecompute.NewRasterizer(target.Width, target.Height).RasterizeScenePTCL([4]uint8{}, a.pendingPaths)
		target.Data, target.Stride = img.Pix, img.Stride
		assertHollowRoundRect(t, target)
	})
}

// Included in the TestGoldenSoftware CI selection. Never substitutes a CPU
// render for a GPU result: unavailable pipelines are reported as skipped.
func TestGoldenSoftware_StrokeRoundRectStaysHollow(t *testing.T) {
	s := NewGPUShared()
	t.Cleanup(s.Close)
	s.mu.Lock()
	err := s.initGPU()
	s.mu.Unlock()
	if err != nil {
		t.Skipf("GPU unavailable: %v", err)
	}
	t.Run("stencil", func(t *testing.T) {
		if s.strategy == strategyRasterAtlas {
			t.Skip("GPU stencil pipeline unavailable on this adapter")
		}
		rc := s.NewRenderContext()
		t.Cleanup(rc.Close)
		rc.pipelineMode = gg.PipelineModeRenderPass
		rc.SetAntiAlias(true)
		path, paint := thinRoundRectStroke()
		target := makeTestTarget(384, 208)
		if err := rc.StrokePath(target, path, paint); err != nil {
			t.Fatal(err)
		}
		if len(rc.pendingDraws) != 1 || rc.pendingDraws[0].stencilCmd == nil {
			t.Fatal("stroke did not reach the stencil pipeline")
		}
		if err := rc.Flush(target); err != nil {
			t.Fatal(err)
		}
		assertHollowRoundRect(t, target)
	})
	t.Run("vello_compute", func(t *testing.T) {
		if !s.CanCompute() {
			t.Skip("GPU compute pipeline unavailable on this adapter")
		}
		a := s.velloAccel
		a.SetAntiAlias(true)
		path, paint := thinRoundRectStroke()
		target := makeTestTarget(384, 208)
		if err := a.StrokePath(target, path, paint); err != nil {
			t.Fatal(err)
		}
		if err := a.Flush(target); err != nil {
			t.Fatal(err)
		}
		assertHollowRoundRect(t, target)
	})
}
