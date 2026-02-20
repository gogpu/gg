// Example: gg + gogpu integration via ggcanvas
//
// This example demonstrates rendering 2D graphics with gg
// directly into a gogpu window using the ggcanvas integration package.
//
// Architecture:
//
//	gg.Context (draw) → ggcanvas.Canvas → gogpu.Context (GPU) → Window
//
// The example showcases all four GPU rendering tiers:
//
//	Tier 1 (SDF):           circles, rounded rectangles
//	Tier 2a (Convex):       triangle, pentagon, hexagon
//	Tier 2b (Stencil+Cover): star shape, curved paths
//	Tier 4 (MSDF text):     title text, FPS counter
//
// Rendering mode: event-driven with animation token.
// Uses ContinuousRender=false + StartAnimation() to render at VSync
// only while animation is active. Press Space to pause/resume.
//
// Requirements:
//   - gogpu v0.18.1+
//   - gg v0.28.1+
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // Register GPU accelerator (SDF + MSAA 4x + MSDF text)
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
)

func main() {
	const width, height = 800, 600

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("GoGPU + gg: Four-Tier GPU Rendering").
		WithSize(width, height).
		WithContinuousRender(false)) // Event-driven: 0% CPU when paused

	// Load system font for Tier 4 (MSDF text rendering).
	fontFace := loadFont()

	var canvas *ggcanvas.Canvas
	var animToken *gogpu.AnimationToken
	var frame int
	paused := false
	startTime := time.Now()

	app.OnDraw(func(dc *gogpu.Context) {
		if frame == 0 {
			log.Printf("Backend: %s", dc.Backend())
			// Start animation — renders at VSync while token is alive.
			animToken = app.StartAnimation()
			log.Printf("Animation started (Space to pause/resume)")
		}

		w, h := dc.Width(), dc.Height()
		if w <= 0 || h <= 0 {
			return
		}

		// No dc.Clear() needed — gg renders directly to surface.
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

		elapsed := time.Since(startTime).Seconds()
		if err := canvas.Draw(func(cc *gg.Context) {
			renderFrame(cc, elapsed, cw, ch, fontFace, frame)
		}); err != nil {
			log.Printf("Draw error: %v", err)
		}

		// Render directly to surface (zero-copy, no readback).
		sv := dc.SurfaceView()
		sw, sh := dc.SurfaceSize()
		if err := canvas.RenderDirect(sv, sw, sh); err != nil {
			log.Printf("Frame %d: RenderDirect error: %v", frame, err)
		}
		frame++
	})

	// Space toggles animation pause/resume — demonstrates three-state model.
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
			log.Printf("Resumed")
		}
	})

	// GPU resources are automatically cleaned up on shutdown:
	// - ggcanvas.Canvas auto-registers with App's ResourceTracker
	// - App.Run() calls tracker.CloseAll() before Renderer.Destroy()
	// - OnClose is still available for additional cleanup (e.g., accelerator)
	app.OnClose(func() {
		if animToken != nil {
			animToken.Stop()
		}
		// Close accelerator: drains GPU queue and destroys session
		// resources (persistent buffers, textures) while the device is alive.
		gg.CloseAccelerator()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// renderFrame draws animated 2D graphics demonstrating all four GPU tiers.
func renderFrame(cc *gg.Context, elapsed float64, width, height int, face text.Face, frame int) {
	cc.SetRGBA(0, 0, 0, 0)
	cc.Clear()

	t := elapsed * 0.8
	cx, cy := float64(width)/2, float64(height)/2

	// --- Tier 1: SDF shapes (circles, rounded rects) ---
	// Animated ring of circles.
	for i := 0; i < 12; i++ {
		angle := float64(i)*math.Pi/6 + t
		x := cx + math.Cos(angle)*160
		y := cy + math.Sin(angle)*160

		hue := float64(i) / 12.0
		r, g, b := hsvToRGB(hue, 0.85, 1.0)
		cc.SetRGBA(r, g, b, 0.9)

		radius := 22 + 8*math.Sin(t*2+float64(i))
		cc.DrawCircle(x, y, radius)
		cc.Fill()
	}

	// Stroked circle in center.
	cc.SetRGBA(1, 1, 1, 0.3)
	cc.SetLineWidth(1.5)
	cc.DrawCircle(cx, cy, 160)
	cc.Stroke()

	// Rounded rectangle indicator.
	rrW, rrH := 120.0, 50.0
	cc.SetRGBA(0.2, 0.6, 1.0, 0.7)
	cc.DrawRoundedRectangle(cx-rrW/2, cy-rrH/2, rrW, rrH, 12)
	cc.Fill()

	// --- Tier 2a: Convex polygons (no stencil needed) ---
	// Rotating triangle.
	triAngle := t * 1.5
	triCx, triCy := cx-200, cy+150
	drawRotatedPolygon(cc, triCx, triCy, 40, 3, triAngle)
	cc.SetRGBA(1.0, 0.6, 0.1, 0.85)
	cc.Fill()

	// Rotating pentagon.
	pentCx, pentCy := cx, cy+150
	drawRotatedPolygon(cc, pentCx, pentCy, 35, 5, -t*1.2)
	cc.SetRGBA(0.2, 0.9, 0.4, 0.85)
	cc.Fill()

	// Rotating hexagon.
	hexCx, hexCy := cx+200, cy+150
	drawRotatedPolygon(cc, hexCx, hexCy, 35, 6, t*0.9)
	cc.SetRGBA(0.9, 0.2, 0.6, 0.85)
	cc.Fill()

	// --- Tier 2b: Stencil-then-cover (non-convex / curves) ---
	// Rotating star (non-convex).
	starCx, starCy := cx, cy-160
	drawRotatedStar(cc, starCx, starCy, 45, 20, 5, t*0.7)
	cc.SetRGBA(1.0, 0.85, 0.2, 0.95)
	cc.Fill()

	// Pulsing curved shape.
	pulse := 0.8 + 0.2*math.Sin(t*3)
	curveCx, curveCy := cx+220, cy-100
	r := 40.0 * pulse
	cc.MoveTo(curveCx, curveCy-r)
	cc.CubicTo(curveCx+r*1.5, curveCy-r, curveCx+r*1.5, curveCy+r, curveCx, curveCy+r)
	cc.CubicTo(curveCx-r*1.5, curveCy+r, curveCx-r*1.5, curveCy-r, curveCx, curveCy-r)
	cc.ClosePath()
	cc.SetRGBA(0.5, 0.2, 0.9, 0.7)
	cc.Fill()

	// --- Tier 4: MSDF text (GPU-accelerated text rendering) ---
	if face != nil {
		// Title text.
		cc.SetFont(face)
		cc.SetRGBA(1, 1, 1, 0.95)
		cc.DrawStringAnchored("Four-Tier GPU Rendering", cx, 30, 0.5, 0)

		// Frame counter in the bottom-right corner.
		cc.SetRGBA(0.7, 0.7, 0.7, 0.8)
		fpsText := fmt.Sprintf("Frame %d | %.1fs", frame, elapsed)
		cc.DrawString(fpsText, 10, float64(height)-10)
	}
}

// drawRotatedPolygon draws a regular polygon rotated by angle radians.
func drawRotatedPolygon(cc *gg.Context, cx, cy, radius float64, sides int, angle float64) {
	for i := 0; i < sides; i++ {
		a := float64(i)*2*math.Pi/float64(sides) + angle - math.Pi/2
		x := cx + radius*math.Cos(a)
		y := cy + radius*math.Sin(a)
		if i == 0 {
			cc.MoveTo(x, y)
		} else {
			cc.LineTo(x, y)
		}
	}
	cc.ClosePath()
}

// drawRotatedStar draws a star rotated by angle radians.
func drawRotatedStar(cc *gg.Context, cx, cy, outerR, innerR float64, points int, angle float64) {
	for i := 0; i < points*2; i++ {
		a := float64(i)*math.Pi/float64(points) + angle - math.Pi/2
		r := outerR
		if i%2 == 1 {
			r = innerR
		}
		x := cx + r*math.Cos(a)
		y := cy + r*math.Sin(a)
		if i == 0 {
			cc.MoveTo(x, y)
		} else {
			cc.LineTo(x, y)
		}
	}
	cc.ClosePath()
}

// loadFont finds a system font and creates a Face for text rendering.
// Returns nil if no font is available (text rendering will be skipped).
func loadFont() text.Face {
	fontPath := findSystemFont()
	if fontPath == "" {
		log.Println("No system font found. Tier 4 (MSDF text) disabled.")
		return nil
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		log.Printf("Failed to load font %s: %v", fontPath, err)
		return nil
	}

	log.Printf("Loaded font: %s", source.Name())
	return source.Face(20)
}

// findSystemFont returns path to a TTF font (TTC collections not supported).
func findSystemFont() string {
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\calibri.ttf",
		"C:\\Windows\\Fonts\\segoeui.ttf",
		// macOS
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf",
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
	t := v * (1 - s*(1-f))
	switch int(i) % 6 {
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
