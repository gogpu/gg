// Example: Animated scene rendering with GPU acceleration in a gogpu window.
//
// Demonstrates GPUSceneRenderer: scene commands decode into gg.Context GPU
// calls (SDF shapes → per-context GPURenderContext → GPU render pass).
//
// The scene is built each frame (retained encoding), then rendered through
// the canvas gg.Context. GPU accelerator handles shapes via SDF pipeline;
// unsupported ops fall back to CPU automatically.
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/scene"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
)

func main() {
	const width, height = 600, 600

	loadFont()

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("Scene GPU Rendering — ARCH-GG-001").
		WithSize(width, height).
		WithContinuousRender(false))

	var (
		canvas       *ggcanvas.Canvas
		animTime     float64
		lastDrawTime time.Time
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
		s := buildAnimatedScene(w, h, t)

		if err := canvas.Draw(func(cc *gg.Context) {
			cc.SetRGBA(0.06, 0.06, 0.1, 1)
			cc.Clear()

			// Scene → GPU: decode scene commands through this Context's GPU pipeline.
			gpuR := scene.NewGPUSceneRenderer(cc)
			_ = gpuR.RenderScene(s)
		}); err != nil {
			log.Printf("Draw: %v", err)
		}

		if err := canvas.Render(dc.RenderTarget()); err != nil {
			log.Printf("Render: %v", err)
		}

		app.RequestRedraw()
	})

	app.OnClose(func() {})

	if err := app.Run(); err != nil {
		log.Fatalf("app.Run: %v", err)
	}
}

var fontSource *text.FontSource

func loadFont() {
	candidates := []string{
		`C:\Windows\Fonts\segoeui.ttf`,
		`C:\Windows\Fonts\arial.ttf`,
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			src, err := text.NewFontSourceFromFile(p)
			if err == nil {
				fontSource = src
				return
			}
		}
	}
}

func buildAnimatedScene(w, h int, t float64) *scene.Scene {
	b := scene.NewSceneBuilder()
	cx, cy := float32(w)/2, float32(h)/2

	n := 12
	radius := float32(180)
	for i := 0; i < n; i++ {
		angle := float64(i)*2*math.Pi/float64(n) + t*0.5
		x := cx + radius*float32(math.Cos(angle))
		y := cy + radius*float32(math.Sin(angle))
		r := 14 + float32(i)*2.5
		hue := math.Mod(float64(i)/float64(n)+t*0.1, 1.0)
		b.FillCircle(x, y, r, scene.SolidBrush(hueToRGBA(hue)))
	}

	b.StrokeCircle(cx, cy, radius,
		scene.SolidBrush(gg.RGBA{R: 0.3, G: 0.3, B: 0.4, A: 0.4}), 1)
	b.FillCircle(cx, cy, 50,
		scene.SolidBrush(gg.RGBA{R: 0.15, G: 0.2, B: 0.5, A: 0.9}))
	b.StrokeCircle(cx, cy, 50,
		scene.SolidBrush(gg.RGBA{R: 0.3, G: 0.4, B: 0.8, A: 1}), 2)

	s := b.Build()
	if fontSource != nil {
		white := scene.SolidBrush(gg.RGBA{R: 1, G: 1, B: 1, A: 1})
		light := scene.SolidBrush(gg.RGBA{R: 0.7, G: 0.7, B: 0.8, A: 1})

		face13 := fontSource.Face(13)
		face16 := fontSource.Face(16)

		_ = s.DrawText("Scene Text — TagText (ADR-022)", face16, 16, float32(h)-50, white)
		_ = s.DrawText("Hinted · Atlas-batched · DPI-aware", face13, 16, float32(h)-30, light)
		_ = s.DrawText(fmt.Sprintf("t = %.1fs", t), face13, float32(w)-90, float32(h)-30, light)
	}
	return s
}

func hueToRGBA(h float64) gg.RGBA {
	h = h - math.Floor(h)
	i := int(h * 6)
	f := h*6 - float64(i)
	q := 1 - f
	t := f
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = 1, t, 0
	case 1:
		r, g, b = q, 1, 0
	case 2:
		r, g, b = 0, 1, t
	case 3:
		r, g, b = 0, q, 1
	case 4:
		r, g, b = t, 0, 1
	case 5:
		r, g, b = 1, 0, q
	}
	return gg.RGBA{R: r, G: g, B: b, A: 1}
}
