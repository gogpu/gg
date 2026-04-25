// Example: Blit-Only Compositor Path (ADR-016, non-MSAA)
//
// Tests the non-MSAA blit-only fast path in isolation. NO SDF shapes,
// NO GPU text — only CPU-drawn content uploaded via FlushPixmap and
// composited via DrawGPUTextureBase. isBlitOnly() should return true,
// triggering the 1x render pass (no 4x MSAA overhead).
//
// This is the path ui/desktop.go uses when all animated content is in
// RepaintBoundary GPU textures and the main compositor only blits
// the CPU pixmap + overlay quads.
//
// Expected: smooth animation, low GPU (<3%), non-MSAA render pass.
//
// Press Escape to quit.
package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"time"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gogpu"
)

func main() {
	const width, height = 600, 400

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("Blit-Only Path (ADR-016, non-MSAA)").
		WithSize(width, height).
		WithContinuousRender(false))

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

		// ── CPU-ONLY content (NO GPU SDF, NO GPU text) ──

		gg.BeginAcceleratorFrame()
		cc.BeginGPUFrame()

		cc.FillRectCPU(0, 0, float64(w), float64(h), gg.RGBA{0.05, 0.07, 0.12, 1})

		for i := range 12 {
			t := elapsed*0.8 + float64(i)*math.Pi/6
			cx := float64(w)/2 + math.Cos(t)*float64(w)*0.35
			cy := float64(h)/2 + math.Sin(t)*float64(h)*0.35
			radius := 8 + 6*math.Sin(elapsed*2+float64(i))
			r := uint8(128 + 127*math.Sin(t*0.5))
			g := uint8(128 + 127*math.Sin(t*0.5+2))
			b := uint8(128 + 127*math.Sin(t*0.5+4))
			fillCircleCPU(cc.ResizeTarget(), int(cx), int(cy), int(radius), color.RGBA{r, g, b, 200})
		}

		gridColor := gg.RGBA{0.12, 0.12, 0.2, 0.5}
		for x := 0; x < w; x += 40 {
			cc.FillRectCPU(float64(x), 0, 1, float64(h), gridColor)
		}
		for y := 0; y < h; y += 40 {
			cc.FillRectCPU(0, float64(y), float64(w), 1, gridColor)
		}

		cc.FillRectCPU(10, 10, 350, 20, gg.RGBA{0, 0, 0, 0.7})
		drawLabel(cc, "BLIT-ONLY: CPU pixmap, non-MSAA 1x render pass", 15, 12)
		drawLabel(cc, fmt.Sprintf("%.1fs | isBlitOnly=true expected", elapsed), 15, float64(h-18))

		// ── Upload + Composite (blit-only path) ──

		canvas.MarkDirty()
		if _, err := canvas.FlushPixmap(); err != nil {
			log.Printf("FlushPixmap: %v", err)
			return
		}
		if err := canvas.EnsureGPUTexture(dc.RenderTarget()); err != nil {
			log.Printf("EnsureGPUTexture: %v", err)
			return
		}

		view := canvas.PixmapTextureView()
		sv := dc.RenderTarget().SurfaceView()
		if view.IsNil() || sv.IsNil() {
			return
		}

		cc.DrawGPUTextureBase(view, 0, 0, w, h)

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

func fillCircleCPU(pm *gg.Pixmap, cx, cy, r int, c color.RGBA) {
	pr := uint8(uint16(c.R) * uint16(c.A) / 255)
	pg := uint8(uint16(c.G) * uint16(c.A) / 255)
	pb := uint8(uint16(c.B) * uint16(c.A) / 255)
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				pm.SetPixelPremul(cx+dx, cy+dy, pr, pg, pb, c.A)
			}
		}
	}
}

func drawLabel(cc *gg.Context, text string, x, y float64) {
	cc.FillRectCPU(x, y, float64(len(text))*6.5, 14, gg.RGBA{0.3, 0.6, 0.3, 0.8})
}
