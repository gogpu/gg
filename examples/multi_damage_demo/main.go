// Multi-rect damage demo (ADR-028).
//
// Two animated elements at opposite corners — demonstrates per-draw dynamic
// scissor. With single-rect union damage, the entire diagonal is re-rendered.
// With multi-rect, only two small regions update.
//
// Run with debug overlay to see damage rects:
//
//	GOGPU_DEBUG_DAMAGE=overlay go run ./examples/multi_damage_demo
//
// Expected: two green flash regions at opposite corners + HUD, NOT a diagonal stripe.
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
)

const (
	winWidth  = 800
	winHeight = 600
)

func main() {
	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("Multi-Rect Damage Demo (ADR-028)").
		WithSize(winWidth, winHeight).
		WithContinuousRender(true))

	var fontFace text.Face
	if fp := findFont(); fp != "" {
		if src, err := text.NewFontSourceFromFile(fp); err == nil {
			fontFace = src.Face(14)
		}
	}

	var (
		canvas       *ggcanvas.Canvas
		animToken    *gogpu.AnimationToken
		animTime     float64
		lastDrawTime time.Time
		frameNum     int
		warmupDone   bool
		currentFPS   float64
		fpsAccum     int
		fpsTimer     time.Time
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
			fpsTimer = time.Now()
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

		fpsAccum++
		if elapsed := now.Sub(fpsTimer).Seconds(); elapsed >= 1.0 {
			currentFPS = float64(fpsAccum) / elapsed
			fpsAccum = 0
			fpsTimer = now
		}

		t := animTime

		if err := canvas.Draw(func(cc *gg.Context) {
			if fontFace != nil {
				cc.SetFont(fontFace)
			}

			// Static background — drawn every frame (immediate mode).
			// ResetFrameDamage before AND after: these pixels don't change.
			cc.ResetFrameDamage()
			drawBackground(cc, w, h)
			cc.ResetFrameDamage()

			// Animated elements — each generates damage rects for the overlay.
			// Single path per element → one clean damage rect per region.
			drawTopLeftSpinner(cc, t)
			drawBottomRightPulse(cc, w, h, t)
			drawHUD(cc, w, h, frameNum, t, currentFPS)
		}); err != nil {
			log.Printf("Draw: %v", err)
		}

		if !warmupDone || frameNum < 3 {
			if err := canvas.Render(dc.RenderTarget()); err != nil {
				log.Printf("Render: %v", err)
			}
			warmupDone = true
		} else {
			if err := canvas.Render(dc.RenderTarget()); err != nil {
				log.Printf("Render: %v", err)
			}
		}

		frameNum++
		if frameNum%60 == 0 {
			log.Printf("Frame %d | %.0f FPS | damage rects active", frameNum, currentFPS)
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

func drawBackground(cc *gg.Context, w, h int) {
	cc.SetRGBA(0.05, 0.05, 0.1, 1)
	cc.DrawRectangle(0, 0, float64(w), float64(h))
	_ = cc.Fill()

	cc.SetRGBA(1, 1, 1, 0.3)
	cc.DrawStringAnchored("Multi-Rect Damage Demo (ADR-028)", float64(w)/2, float64(h)/2-20, 0.5, 0.5)
	cc.DrawStringAnchored("Two animated regions at opposite corners", float64(w)/2, float64(h)/2+5, 0.5, 0.5)
	cc.DrawStringAnchored("With GOGPU_DEBUG_DAMAGE=overlay: green squares show damage", float64(w)/2, float64(h)/2+25, 0.5, 0.5)
}

func drawTopLeftSpinner(cc *gg.Context, t float64) {
	cx, cy := 70.0, 70.0
	r := 30.0
	angle := t * 3

	// All dots in one path → one Fill → one damage rect.
	cc.SetRGBA(0.2, 0.8, 1.0, 1)
	for i := 0; i < 3; i++ {
		a := angle + float64(i)*2.094
		x := cx + r*math.Cos(a)
		y := cy + r*math.Sin(a)
		cc.DrawCircle(x, y, 8)
	}
	_ = cc.Fill()

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

func drawHUD(cc *gg.Context, w, h, frame int, t, fps float64) {
	s := fmt.Sprintf("Frame %d | %.1fs | %.0f FPS", frame, t, fps)
	cc.SetRGBA(0, 0, 0, 0.7)
	cc.DrawRoundedRectangle(6, float64(h)-22, 200, 18, 4)
	_ = cc.Fill()
	cc.SetRGBA(0.7, 1, 0.7, 1)
	cc.DrawString(s, 10, float64(h)-8)
}

func findFont() string {
	for _, p := range []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\segoeui.ttf",
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
