// GPU-CLIP-003a validation: arbitrary path clipping via depth buffer.
//
// Three sections:
//   - Left: Circle clip (arbitrary path) with colored rectangles inside
//   - Center: Star clip (complex path) with concentric circles inside
//   - Right: No clip (reference)
//
// If GPU depth clip works: shapes are clipped to circle/star boundaries.
// If GPU depth clip fails: shapes extend beyond boundaries (CPU fallback).
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
	const width, height = 900, 500

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("GPU-CLIP-003a: Arbitrary Path Clip Test").
		WithSize(width, height).
		WithContinuousRender(false))

	log.Println("=== REBUILD MARKER 2026-05-01 22:30 ===")
	fontSource := loadFontSource()
	var face16, face12 text.Face
	if fontSource != nil {
		face16 = fontSource.Face(16)
		face12 = fontSource.Face(12)
	}

	var canvas *ggcanvas.Canvas
	var animToken *gogpu.AnimationToken
	var animTime float64
	var lastDrawTime time.Time
	frame := 0

	app.OnDraw(func(dc *gogpu.Context) {
		if frame == 0 {
			log.Printf("Backend: %s", dc.Backend())
			animToken = app.StartAnimation()
			_ = animToken
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

		w, h := dc.Size()
		if canvas == nil {
			provider := app.GPUContextProvider()
			if provider == nil {
				return
			}
			var err error
			canvas, err = ggcanvas.New(provider, w, h)
			if err != nil {
				log.Printf("Canvas error: %v", err)
				return
			}
		}

		cw, ch := w, h
		t := animTime

		if err := canvas.Draw(func(cc *gg.Context) {
			renderFrame(cc, t, float64(cw), float64(ch), face16, face12, frame)
		}); err != nil {
			log.Printf("Draw error: %v", err)
		}

		if err := canvas.Render(dc.RenderTarget()); err != nil {
			log.Printf("Render error: %v", err)
		}
		frame++
		app.RequestRedraw()
	})

	app.OnResize(func(w, h int) {
		if canvas != nil {
			canvas.Resize(w, h)
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func renderFrame(cc *gg.Context, t, w, h float64, face16, face12 text.Face, frame int) {
	cc.SetRGBA(0.1, 0.1, 0.15, 1)
	cc.DrawRectangle(0, 0, w, h)
	_ = cc.Fill()

	// === Section 1: Circle clip (arbitrary path) ===
	cx1 := w * 0.2
	cy1 := h * 0.55
	radius := 80.0 + 10*math.Sin(t*0.8)

	cc.SetRGBA(1, 1, 1, 0.3)
	cc.SetLineWidth(1)
	cc.DrawCircle(cx1, cy1, radius)
	cc.Stroke()

	cc.Push()
	cc.DrawCircle(cx1, cy1, radius)
	cc.Clip()

	// Static rectangles — clip circle animates over them.
	colors := [][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0.5, 1}, {1, 1, 0}, {1, 0, 1}}
	baseR := 90.0 // fixed size for content
	for i, col := range colors {
		y := cy1 - baseR + float64(i)*baseR*2/5
		cc.SetRGBA(col[0], col[1], col[2], 0.8)
		cc.DrawRectangle(cx1-baseR-10, y, baseR*2+20, baseR*2/5)
		_ = cc.Fill()
	}
	cc.Pop()

	// === Section 2: Star clip (complex arbitrary path) ===
	cx2 := w * 0.5
	cy2 := h * 0.55
	starR := 90.0

	cc.SetRGBA(1, 1, 1, 0.3)
	cc.SetLineWidth(1)
	drawStar(cc, cx2, cy2, starR, starR*0.4, 5, t*0.3)
	cc.Stroke()

	cc.Push()
	drawStar(cc, cx2, cy2, starR, starR*0.4, 5, t*0.3)
	cc.Clip()

	for i := 0; i < 8; i++ {
		r := starR * float64(8-i) / 8
		hue := math.Mod(float64(i)*0.125+t*0.1, 1.0)
		cr, cg, cb := hsvToRGB(hue, 0.8, 0.9)
		cc.SetRGBA(cr, cg, cb, 0.7)
		cc.DrawCircle(cx2, cy2, r)
		_ = cc.Fill()
	}
	cc.Pop()

	// === Section 3: No clip (reference) ===
	cx3 := w * 0.8
	cy3 := h * 0.55

	cc.SetRGBA(1, 1, 1, 0.3)
	cc.SetLineWidth(1)
	cc.DrawCircle(cx3, cy3, 80)
	cc.Stroke()

	for i, col := range colors {
		y := cy3 - 80 + float64(i)*160/5
		cc.SetRGBA(col[0], col[1], col[2], 0.8)
		cc.DrawRectangle(cx3-100, y, 200, 32)
		_ = cc.Fill()
	}

	// === Labels ===
	if face16 != nil {
		cc.SetFont(face16)
		cc.SetRGBA(1, 1, 1, 0.9)
		cc.DrawStringAnchored("Circle Clip (GPU depth)", cx1, 30, 0.5, 0)
		cc.DrawStringAnchored("Star Clip (GPU depth)", cx2, 30, 0.5, 0)
		cc.DrawStringAnchored("No Clip (reference)", cx3, 30, 0.5, 0)
	}
	if face12 != nil {
		cc.SetFont(face12)
		cc.SetRGBA(0.6, 0.6, 0.6, 0.8)
		cc.DrawString(fmt.Sprintf("Frame %d | GPU-CLIP-003a depth-based arbitrary path clipping", frame), 10, h-10)
	}
}

func drawStar(dc *gg.Context, cx, cy, outerR, innerR float64, points int, angle float64) {
	n := points * 2
	for i := 0; i < n; i++ {
		a := angle + float64(i)*math.Pi*2/float64(n) - math.Pi/2
		r := outerR
		if i%2 == 1 {
			r = innerR
		}
		x := cx + r*math.Cos(a)
		y := cy + r*math.Sin(a)
		if i == 0 {
			dc.MoveTo(x, y)
		} else {
			dc.LineTo(x, y)
		}
	}
	dc.ClosePath()
}

func hsvToRGB(h, s, v float64) (float64, float64, float64) {
	i := int(h * 6)
	f := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	switch i % 6 {
	case 0:
		return v, t, p
	case 1:
		return q, v, p
	case 2:
		return p, v, t
	case 3:
		return p, q, v
	case 4:
		return t, p, v
	default:
		return v, p, q
	}
}

func loadFontSource() *text.FontSource {
	paths := []string{
		"C:/Windows/Fonts/arial.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
	}
	for _, p := range paths {
		src, err := text.NewFontSourceFromFile(p)
		if err == nil {
			return src
		}
	}
	return nil
}
