// Example: gg + gogpu clip demo
//
// This example demonstrates GPU-accelerated clipping using gg's ClipRect
// and ClipRoundRect APIs, rendered directly into a gogpu window via ggcanvas.
//
// Architecture:
//
//	gg.Context (draw + clip) → ggcanvas.Canvas → gogpu.Context (GPU) → Window
//
// Scene layout:
//
//	Left half:  rotating ring of colored circles (NO clip — fully visible)
//	Right half: animated shapes inside a pulsing clip region (clipped at edges)
//	Labels:     "No Clip" and "Clipped Region" above each section
//
// Rendering mode: event-driven with animation token.
// Uses ContinuousRender=false + StartAnimation() to render at VSync
// only while animation is active. Press Space to pause/resume.
//
// Requirements:
//   - gogpu v0.26.4+
//   - gg v0.40.0+
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // Register GPU accelerator (SDF + MSDF text)
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
)

func main() {
	const width, height = 800, 600

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("GoGPU + gg: Clip Demo").
		WithSize(width, height).
		WithContinuousRender(false)) // Event-driven: 0% CPU when paused

	// Load system font for text labels.
	fontSource := loadFontSource()
	var face20, face14 text.Face
	if fontSource != nil {
		face20 = fontSource.Face(20)
		face14 = fontSource.Face(14)
	}

	var canvas *ggcanvas.Canvas
	var animToken *gogpu.AnimationToken
	var frame int
	paused := false
	var animTime float64
	var lastDrawTime time.Time

	app.OnDraw(func(dc *gogpu.Context) {
		if frame == 0 {
			log.Printf("Backend: %s", dc.Backend())
			animToken = app.StartAnimation()
			log.Printf("Animation started (Space to pause/resume)")
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
				log.Fatalf("Failed to create canvas: %v", err)
			}
			log.Printf("Canvas created: %dx%d", w, h)
		}

		cw, ch := canvas.Size()
		if cw != w || ch != h {
			if err := canvas.Resize(w, h); err != nil {
				log.Printf("Resize error: %v", err)
			}
			cw, ch = w, h
		}

		elapsed := animTime
		if err := canvas.Draw(func(cc *gg.Context) {
			renderFrame(cc, elapsed, cw, ch, face20, face14, frame)
		}); err != nil {
			log.Printf("Draw error: %v", err)
		}

		if err := canvas.Render(dc.RenderTarget()); err != nil {
			log.Printf("Frame %d: Render error: %v", frame, err)
		}
		frame++

		if !paused {
			app.RequestRedraw()
		}
	})

	// Space toggles animation pause/resume.
	app.EventSource().OnKeyPress(func(key gpucontext.Key, _ gpucontext.Modifiers) {
		if key != gpucontext.KeySpace {
			return
		}
		paused = !paused
		if paused {
			if animToken != nil {
				animToken.Stop()
				animToken = nil
			}
			log.Printf("Paused (0%% CPU idle, press Space to resume)")
		} else {
			animToken = app.StartAnimation()
			app.RequestRedraw()
			log.Printf("Resumed")
		}
	})

	app.OnClose(func() {
		if animToken != nil {
			animToken.Stop()
		}
		gg.CloseAccelerator()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// renderFrame draws the animated clip demo scene.
func renderFrame(cc *gg.Context, elapsed float64, width, height int, face20, face14 text.Face, frame int) {
	// Dark background.
	cc.ClearWithColor(gg.RGBA{R: 0.10, G: 0.10, B: 0.12, A: 1})

	fw := float64(width)
	fh := float64(height)
	t := elapsed * 0.8

	// =============================================
	// LEFT HALF: Rotating ring of circles (no clip)
	// =============================================
	leftCX := fw / 4
	leftCY := fh/2 + 20

	for i := 0; i < 12; i++ {
		angle := float64(i)*math.Pi/6 + t
		x := leftCX + math.Cos(angle)*120
		y := leftCY + math.Sin(angle)*120

		hue := float64(i) / 12.0
		r, g, b := hsvToRGB(hue, 0.85, 1.0)
		cc.SetRGBA(r, g, b, 0.9)

		radius := 16 + 6*math.Sin(t*2+float64(i))
		cc.DrawCircle(x, y, radius)
		cc.Fill()
	}

	// Orbit guide circle.
	cc.SetRGBA(1, 1, 1, 0.15)
	cc.SetLineWidth(1)
	cc.DrawCircle(leftCX, leftCY, 120)
	cc.Stroke()

	// =============================================
	// RIGHT HALF: Animated shapes inside pulsing clip
	// =============================================
	rightCX := fw*3/4 - 10
	rightCY := fh/2 + 20

	// Pulsing clip dimensions — the clip rectangle slowly breathes.
	baseW := 220.0
	baseH := 180.0
	pulse := 20 * math.Sin(t*0.6)
	clipW := baseW + pulse
	clipH := baseH + pulse*0.7
	clipX := rightCX - clipW/2
	clipY := rightCY - clipH/2
	clipRadius := 16.0

	// Draw clip boundary outline (visible BEFORE clipping, so user sees the boundary).
	cc.SetRGBA(1, 1, 1, 0.3)
	cc.SetLineWidth(2)
	cc.DrawRoundedRectangle(clipX, clipY, clipW, clipH, clipRadius)
	cc.Stroke()

	// Apply round-rect clip.
	cc.Push()
	cc.ClipRoundRect(clipX, clipY, clipW, clipH, clipRadius)

	// --- Clipped content: bouncing circles ---
	type ball struct {
		speedX, speedY float64
		radius         float64
		hue            float64
	}
	balls := []ball{
		{1.2, 0.8, 28, 0.0},
		{-0.9, 1.1, 24, 0.15},
		{0.7, -1.3, 20, 0.30},
		{-1.1, -0.7, 32, 0.50},
		{1.4, 0.5, 18, 0.70},
		{-0.6, 1.4, 26, 0.85},
	}

	for i, b := range balls {
		// Deterministic bouncing: each ball travels a sine path.
		phase := float64(i) * 1.7
		bx := rightCX + 130*math.Sin(t*b.speedX+phase)
		by := rightCY + 100*math.Cos(t*b.speedY+phase*0.7)

		r, g, bv := hsvToRGB(b.hue, 0.85, 1.0)
		cc.SetRGBA(r, g, bv, 0.85)
		cc.DrawCircle(bx, by, b.radius)
		cc.Fill()
	}

	// --- Clipped content: rotating square ---
	sqAngle := t * 0.5
	sqSize := 60.0
	cc.SetRGBA(0.2, 0.5, 1.0, 0.5)
	drawRotatedSquare(cc, rightCX, rightCY, sqSize, sqAngle)
	cc.Fill()

	// --- Clipped content: wide horizontal bar (extends beyond clip) ---
	barY := rightCY + 40*math.Sin(t*0.9)
	cc.SetRGBA(1, 1, 1, 0.25)
	cc.DrawRoundedRectangle(rightCX-200, barY-8, 400, 16, 4)
	cc.Fill()

	// Remove clip.
	cc.Pop()

	// =============================================
	// TEXT LABELS
	// =============================================
	if face20 != nil {
		cc.SetFont(face20)
		cc.SetRGBA(1, 1, 1, 0.9)
		cc.DrawStringAnchored("No Clip", leftCX, 30, 0.5, 0)
		cc.DrawStringAnchored("Clipped Region", rightCX, 30, 0.5, 0)
	}

	// Frame counter + FPS in bottom-left.
	if face14 != nil {
		cc.SetFont(face14)
		cc.SetRGBA(0.6, 0.6, 0.6, 0.8)
		fps := 0.0
		if elapsed > 0 {
			fps = float64(frame) / elapsed
		}
		info := fmt.Sprintf("Frame %d | %.1fs | %.0f FPS", frame, elapsed, fps)
		cc.DrawString(info, 10, float64(height)-10)
	}
}

// drawRotatedSquare draws a square centered at (cx, cy) rotated by angle radians.
func drawRotatedSquare(cc *gg.Context, cx, cy, size, angle float64) {
	half := size / 2
	for i := 0; i < 4; i++ {
		a := angle + float64(i)*math.Pi/2 + math.Pi/4
		x := cx + half*math.Sqrt2*math.Cos(a)
		y := cy + half*math.Sqrt2*math.Sin(a)
		if i == 0 {
			cc.MoveTo(x, y)
		} else {
			cc.LineTo(x, y)
		}
	}
	cc.ClosePath()
}

// loadFontSource finds a system font and returns the font source.
func loadFontSource() *text.FontSource {
	fontPath := findSystemFont()
	if fontPath == "" {
		log.Println("No system font found. Text labels disabled.")
		return nil
	}
	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		log.Printf("Failed to load font %s: %v", fontPath, err)
		return nil
	}
	log.Printf("Loaded font: %s", source.Name())
	return source
}

// findSystemFont returns path to a TTF font.
func findSystemFont() string {
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\calibri.ttf",
		"C:\\Windows\\Fonts\\segoeui.ttf",
		// macOS
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		// Linux
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func hsvToRGB(h, s, v float64) (r, g, b float64) {
	if s == 0 {
		return v, v, v
	}
	h *= 6
	i := math.Floor(h)
	f := h - i
	p := v * (1 - s)
	q := v * (1 - s*f)
	tt := v * (1 - s*(1-f))
	switch int(i) % 6 {
	case 0:
		return v, tt, p
	case 1:
		return q, v, p
	case 2:
		return p, v, tt
	case 3:
		return p, q, v
	case 4:
		return tt, p, v
	default:
		return v, p, q
	}
}
