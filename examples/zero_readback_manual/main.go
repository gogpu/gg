// Example: Manual Zero-Readback Pipeline (ADR-015/016)
//
// This example uses the MANUAL zero-readback pipeline — NO FlushGPU readback:
//
//	Phase 1: CPU content → pixmap (FillRectCPU, DrawLine, DrawString)
//	Phase 2: FlushPixmap → upload pixmap to GPU texture (no readback!)
//	Phase 3: DrawGPUTextureBase → pixmap as compositor base layer
//	         GPU SDF circles → overlay shapes
//	         FlushGPUWithView → single pass to swapchain
//
// Compare with examples/zero_readback/ which uses the standard RenderDirect path.
// This example tests the manual pipeline that ui/desktop.go will use for
// retained-mode compositing (ADR-006).
//
// Press Escape to quit.
package main

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
)

func main() {
	const width, height = 600, 400

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("Manual Zero-Readback Pipeline").
		WithSize(width, height).
		WithContinuousRender(false))

	var fontFace text.Face
	if src, err := text.NewFontSourceFromFile("C:/Windows/Fonts/arial.ttf"); err == nil {
		fontFace = src.Face(14)
	}

	var canvas *ggcanvas.Canvas
	var animToken *gogpu.AnimationToken
	startTime := time.Now()

	app.OnDraw(func(dc *gogpu.Context) {
		w, h := dc.Width(), dc.Height()
		if w <= 0 || h <= 0 {
			return
		}

		if canvas == nil {
			provider := app.GPUContextProvider()
			if provider == nil {
				return
			}
			canvas, _ = ggcanvas.New(provider, w, h)
			if canvas == nil {
				return
			}
			animToken = app.StartAnimation()
		}

		cc := canvas.Context()
		elapsed := time.Since(startTime).Seconds()

		// ── Phase 1: CPU content → pixmap ──

		gg.BeginAcceleratorFrame()
		cc.BeginGPUFrame()

		cc.FillRectCPU(0, 0, float64(w), float64(h), gg.RGBA{0.06, 0.06, 0.1, 1})

		cc.SetRGBA(0.12, 0.12, 0.18, 1)
		cc.SetLineWidth(1)
		for x := 0; x < w; x += 50 {
			cc.DrawLine(float64(x), 0, float64(x), float64(h))
			cc.Stroke()
		}
		for y := 0; y < h; y += 50 {
			cc.DrawLine(0, float64(y), float64(w), float64(y))
			cc.Stroke()
		}

		if fontFace != nil {
			cc.SetFont(fontFace)
			cc.SetRGBA(1, 1, 1, 0.9)
			cc.DrawString("MANUAL Zero-Readback Pipeline (FlushPixmap + DrawGPUTextureBase)", 20, 24)
			cc.SetRGBA(0.4, 0.9, 0.4, 0.8)
			cc.DrawString("No FlushGPU readback — pixmap uploaded directly", 20, 48)
			cc.SetRGBA(0.4, 0.7, 1, 0.7)
			cc.DrawString(fmt.Sprintf("%.1fs | wgpu runtime.AddCleanup prevents resource leak", elapsed), 20, float64(h-16))
		}

		// ── Phase 2: Upload pixmap (NO GPU readback) ──

		canvas.MarkDirty()
		if _, err := canvas.FlushPixmap(); err != nil {
			log.Printf("FlushPixmap: %v", err)
			return
		}
		if err := canvas.EnsureGPUTexture(dc.RenderTarget()); err != nil {
			log.Printf("EnsureGPUTexture: %v", err)
			return
		}

		// ── Phase 3: Single-pass compositor ──

		view := canvas.PixmapTextureView()
		sv := dc.RenderTarget().SurfaceView()
		if view.IsNil() || sv.IsNil() {
			return
		}

		cc.DrawGPUTextureBase(view, 0, 0, w, h)

		for i := range 8 {
			t := elapsed + float64(i)*0.4
			cx := float64(w)/2 + math.Cos(t*1.3)*float64(w)*0.35
			cy := float64(h)/2 + math.Sin(t*1.1)*float64(h)*0.3
			radius := 15 + 20*math.Sin(t*2)
			r := 0.5 + 0.5*math.Sin(t*0.7)
			g := 0.3 + 0.3*math.Sin(t*0.7+2.1)
			b := 0.5 + 0.5*math.Sin(t*0.7+4.2)
			cc.SetRGBA(r, g, b, 0.85)
			cc.DrawCircle(cx, cy, radius)
			cc.Fill()
		}

		sw, sh := dc.SurfaceSize()
		if err := cc.FlushGPUWithView(sv, sw, sh); err != nil {
			log.Printf("FlushGPUWithView: %v", err)
		}
	})

	app.OnClose(func() {
		if animToken != nil {
			animToken.Stop()
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
