// Example: Zero-Readback Compositor APIs (ADR-015/016)
//
// Demonstrates GPU-direct rendering via RenderDirect — zero CPU readback:
//
//   - FillRectCPU: CPU-only background (bypasses GPU SDF accelerator)
//   - GPU SDF circles: rendered directly to swapchain surface
//   - GPU GlyphMask text: pixel-perfect hinted text
//   - RenderDirect: single render pass, zero pixmap readback
//
// The FlushPixmap + DrawGPUTextureBase pipeline (for ui retained-mode) is
// demonstrated separately in ui/desktop.go where CPU pixmap contains static
// widget content and GPU shapes are animated overlay layers.
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
		WithTitle("Zero-Readback Compositor (ADR-015/016)").
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
		} else {
			_ = canvas.Resize(w, h)
		}

		_ = canvas.Draw(func(cc *gg.Context) {
			elapsed := time.Since(startTime).Seconds()

			// FillRectCPU: CPU-only background fill.
			// Bypasses GPU SDF accelerator — standard DrawRectangle+Fill
			// would queue an SDF shape, blocking the non-MSAA blit path.
			cc.FillRectCPU(0, 0, float64(w), float64(h), gg.RGBA{0.08, 0.08, 0.12, 1})

			// CPU grid lines (analytic AA rasterizer)
			cc.SetRGBA(0.15, 0.15, 0.22, 1)
			cc.SetLineWidth(1)
			for x := 0; x < w; x += 50 {
				cc.DrawLine(float64(x), 0, float64(x), float64(h))
				cc.Stroke()
			}
			for y := 0; y < h; y += 50 {
				cc.DrawLine(0, float64(y), float64(w), float64(y))
				cc.Stroke()
			}

			// GPU SDF circles (Tier 1 — rendered directly to surface)
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

			// GPU text (Tier 6 GlyphMask — pixel-perfect hinted)
			if fontFace != nil {
				cc.SetFont(fontFace)
				cc.SetRGBA(1, 1, 1, 0.9)
				cc.DrawString("Zero-Readback: RenderDirect (single GPU pass, no readback)", 20, 24)
				cc.SetRGBA(0.6, 0.8, 0.6, 0.8)
				cc.DrawString("FillRectCPU | GPU SDF circles | GPU GlyphMask text", 20, 48)
				cc.SetRGBA(0.5, 0.7, 1, 0.7)
				cc.DrawString(fmt.Sprintf("%.1fs elapsed", elapsed), 20, float64(h-16))
			}
		})

		// RenderDirect: single GPU render pass → swapchain surface.
		// Zero CPU readback, zero pixmap upload. All content GPU-rendered.
		_ = canvas.Render(dc.RenderTarget())
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
