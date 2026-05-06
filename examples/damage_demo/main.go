// Example: Four-level damage pipeline demo (ADR-021).
//
// Demonstrates real partial screen updates:
//   - Static elements: colored rectangles + text (drawn once, never redrawn)
//   - Animated element: single bouncing circle (only its area redrawn each frame)
//   - GOGPU_DEBUG_DAMAGE=1: green overlay shows which regions update
//
// Expected result: with debug overlay, only the bouncing circle area flashes green.
// Static rectangles stay clean after the first frame.
//
// Run:
//
//	GOGPU_GRAPHICS_API=software GOGPU_DEBUG_DAMAGE=1 go run ./examples/damage_demo
package main

import (
	"log"
	"math"
	"time"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
)

func main() {
	const width, height = 700, 500

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("Damage Pipeline Demo (ADR-021)").
		WithSize(width, height).
		WithContinuousRender(false))

	var (
		canvas    *ggcanvas.Canvas
		animToken *gogpu.AnimationToken
		startTime = time.Now()
		frameNum  int
		fpsFrames int
		lastFPS   time.Time
		currentFPS float64
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
			lastFPS = time.Now()
		} else {
			_ = canvas.Resize(w, h)
		}

		t := time.Since(startTime).Seconds()

		if err := canvas.Draw(func(cc *gg.Context) {
			cc.ResetFrameDamage()

			// STATIC elements — redraw every frame (immediate mode) but
			// reset damage after: these pixels don't change between frames.
			drawStaticBackground(cc, w, h)
			cc.ResetFrameDamage()

			// ANIMATED element — only this contributes to damage.
			// Single bouncing circle → small damage rect → green overlay only here.
			drawBouncingCircle(cc, w, h, t)
		}); err != nil {
			log.Printf("Draw: %v", err)
		}

		if err := canvas.Render(dc.RenderTarget()); err != nil {
			log.Printf("Render: %v", err)
		}

		frameNum++
		fpsFrames++
		if time.Since(lastFPS) >= time.Second {
			currentFPS = float64(fpsFrames) / time.Since(lastFPS).Seconds()
			damage := canvas.LastDamage()
			totalPx := w * h
			damagePx := damage.Dx() * damage.Dy()
			savings := 0.0
			if totalPx > 0 {
				savings = (1.0 - float64(damagePx)/float64(totalPx)) * 100
			}
			log.Printf("%.0f FPS | damage: %dx%d (%d px, %.0f%% saved) | total: %dx%d",
				currentFPS, damage.Dx(), damage.Dy(), damagePx, savings, w, h)
			fpsFrames = 0
			lastFPS = time.Now()
		}
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

func drawStaticBackground(cc *gg.Context, w, h int) {
	// Dark background
	cc.SetRGBA(0.08, 0.08, 0.12, 1)
	cc.Clear()

	// Static colored rectangles
	rects := []struct {
		x, y, w, h float64
		r, g, b    float64
	}{
		{20, 20, 150, 80, 0.8, 0.2, 0.2},
		{20, 120, 150, 80, 0.2, 0.8, 0.2},
		{20, 220, 150, 80, 0.2, 0.2, 0.8},
		{20, 320, 150, 80, 0.8, 0.8, 0.2},
		{float64(w) - 170, 20, 150, 80, 0.8, 0.2, 0.8},
		{float64(w) - 170, 120, 150, 80, 0.2, 0.8, 0.8},
		{float64(w) - 170, 220, 150, 80, 0.5, 0.5, 0.5},
		{float64(w) - 170, 320, 150, 80, 1.0, 0.5, 0.0},
	}
	for _, r := range rects {
		cc.SetRGBA(r.r, r.g, r.b, 0.8)
		cc.DrawRoundedRectangle(r.x, r.y, r.w, r.h, 8)
		cc.Fill()
	}

	// Static text
	cc.SetRGBA(1, 1, 1, 0.9)
	cc.DrawStringAnchored("STATIC REGION", float64(w)/2, 30, 0.5, 0.5)
	cc.DrawStringAnchored("These rectangles never change", float64(w)/2, float64(h)-30, 0.5, 0.5)
	cc.DrawStringAnchored("Only the bouncing circle triggers damage", float64(w)/2, float64(h)-10, 0.5, 0.5)
}

func drawBouncingCircle(cc *gg.Context, w, h int, t float64) {
	cx := float64(w)/2 + math.Sin(t*1.3)*150
	cy := float64(h)/2 + math.Cos(t*0.9)*100
	radius := 30 + math.Sin(t*2)*10

	cc.SetRGBA(1, 0.3, 0.1, 1)
	cc.DrawCircle(cx, cy, radius)
	cc.Fill()

	cc.SetRGBA(1, 1, 1, 0.9)
	cc.SetLineWidth(2)
	cc.DrawCircle(cx, cy, radius)
	cc.Stroke()
}
