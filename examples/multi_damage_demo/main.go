// Multi-rect damage demo (ADR-028).
//
// Two animated elements at opposite corners — demonstrates per-draw dynamic
// scissor. With single-rect union damage, the entire diagonal is re-rendered.
// With multi-rect, only two small regions update.
//
// Run with debug overlay to see damage rects:
//
//	GOGPU_DEBUG_DAMAGE=1 go run ./examples/multi_damage_demo
//
// Expected: two green flash regions at opposite corners, NOT a diagonal stripe.
package main

import (
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"time"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
)

func main() {
	const width, height = 800, 600

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("Multi-Rect Damage Demo (ADR-028)").
		WithSize(width, height).
		WithContinuousRender(false))

	var (
		canvas       *ggcanvas.Canvas
		animToken    *gogpu.AnimationToken
		animTime     float64
		lastDrawTime time.Time
		frameNum     int
		warmupDone   bool
	)

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
			var err error
			canvas, err = ggcanvas.New(provider, w, h)
			if err != nil {
				log.Fatalf("ggcanvas.New: %v", err)
			}
			animToken = app.StartAnimation()
		} else {
			_ = canvas.Resize(w, h)
		}

		now := time.Now()
		if !lastDrawTime.IsZero() {
			dt := now.Sub(lastDrawTime).Seconds()
			if dt > 0.1 {
				dt = 1.0 / 60.0
			}
			animTime += dt
		}
		lastDrawTime = now
		t := animTime

		// Draw everything into canvas.
		if err := canvas.Draw(func(cc *gg.Context) {
			cc.ResetFrameDamage()
			if fp := findFont(); fp != "" {
				_ = cc.LoadFontFace(fp, 14)
			}

			// Static background — drawn every frame but damage reset after.
			drawBackground(cc, w, h)
			cc.ResetFrameDamage()

			// Two animated elements at OPPOSITE CORNERS.
			drawTopLeftSpinner(cc, t)
			drawBottomRightPulse(cc, w, h, t)

			// HUD
			drawHUD(cc, w, h, frameNum, t)
		}); err != nil {
			log.Printf("Draw: %v", err)
		}

		// First frame: full render (LoadOpClear). After warmup: multi-rect damage.
		sv := dc.RenderTarget().SurfaceView()
		sw, sh := dc.RenderTarget().SurfaceSize()

		if !warmupDone || frameNum < 3 {
			// Full render for warmup frames (populate swapchain buffers).
			if err := canvas.Render(dc.RenderTarget()); err != nil {
				log.Printf("Render: %v", err)
			}
			warmupDone = true
		} else {
			// Multi-rect damage: two small scissors at opposite corners.
			rects := []image.Rectangle{
				image.Rect(20, 20, 120, 120),                                 // top-left spinner
				image.Rect(int(sw)-120, int(sh)-120, int(sw)-20, int(sh)-20), // bottom-right pulse
			}
			canvas.MarkDirty()
			if err := canvas.RenderDirectWithDamageRects(sv, sw, sh, rects); err != nil {
				log.Printf("RenderDirectWithDamageRects: %v", err)
				// Fallback to full render.
				_ = canvas.Render(dc.RenderTarget())
			}
		}

		frameNum++
		if frameNum%60 == 0 {
			log.Printf("Frame %d | multi-rect damage: top-left 100×100 + bottom-right 100×100 = 20K px (vs union %dx%d = %dK px)",
				frameNum, sw-40, sh-40, int(sw-40)*int(sh-40)/1000)
		}
		app.RequestRedraw()
	})

	app.EventSource().OnKeyPress(func(_ gpucontext.Key, _ gpucontext.Modifiers) {})
	app.OnClose(func() {
		if animToken != nil {
			animToken.Stop()
		}
	})

	if err := app.Run(); err != nil {
		log.Fatalf("app.Run: %v", err)
	}
}

func drawBackground(cc *gg.Context, w, h int) {
	cc.SetRGBA(0.05, 0.05, 0.1, 1)
	cc.DrawRectangle(0, 0, float64(w), float64(h))
	_ = cc.Fill()

	cc.SetRGBA(1, 1, 1, 0.3)
	cc.DrawStringAnchored("Multi-Rect Damage Demo (ADR-028)", float64(w)/2, float64(h)/2-20, 0.5, 0.5)
	cc.DrawStringAnchored("Two animated regions at opposite corners", float64(w)/2, float64(h)/2+5, 0.5, 0.5)
	cc.DrawStringAnchored("With GOGPU_DEBUG_DAMAGE=1: only two green squares, not diagonal", float64(w)/2, float64(h)/2+25, 0.5, 0.5)
}

func drawTopLeftSpinner(cc *gg.Context, t float64) {
	cx, cy := 70.0, 70.0
	r := 30.0

	cc.SetRGBA(0.2, 0.8, 1.0, 1)
	angle := t * 3
	for i := 0; i < 3; i++ {
		a := angle + float64(i)*2.094 // 120° apart
		x := cx + r*math.Cos(a)
		y := cy + r*math.Sin(a)
		cc.DrawCircle(x, y, 8)
		_ = cc.Fill()
	}

	cc.SetRGBA(1, 1, 1, 0.8)
	cc.DrawStringAnchored(fmt.Sprintf("%.0f°", math.Mod(angle*57.3, 360)), cx, cy+45, 0.5, 0.5)
}

func drawBottomRightPulse(cc *gg.Context, w, h int, t float64) {
	cx := float64(w) - 70
	cy := float64(h) - 70
	r := 20 + 15*math.Sin(t*4)

	cc.SetRGBA(1.0, 0.3, 0.5, 1)
	cc.DrawCircle(cx, cy, r)
	_ = cc.Fill()

	cc.SetRGBA(1, 1, 1, 0.7)
	cc.SetLineWidth(2)
	cc.DrawCircle(cx, cy, r+5)
	_ = cc.Stroke()
}

func drawHUD(cc *gg.Context, w, h, frame int, t float64) {
	s := fmt.Sprintf("Frame %d | %.1fs", frame, t)
	cc.SetRGBA(0, 0, 0, 0.7)
	cc.DrawRoundedRectangle(6, float64(h)-22, 180, 18, 4)
	_ = cc.Fill()
	cc.SetRGBA(0.7, 1, 0.7, 1)
	cc.DrawString(s, 10, float64(h)-8)
}

func findFont() string {
	for _, p := range []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\segoeui.ttf",
		"/Library/Fonts/Arial.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
