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

	// Load system fonts for Tier 4 (MSDF text rendering).
	fontSource := loadFontSource()
	var fontFace text.Face
	var face28, face18, face14 text.Face
	if fontSource != nil {
		fontFace = fontSource.Face(20)
		face28 = fontSource.Face(28)
		face18 = fontSource.Face(18)
		face14 = fontSource.Face(14)
	}

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
		faces := [4]text.Face{fontFace, face28, face18, face14}
		if err := canvas.Draw(func(cc *gg.Context) {
			renderFrame(cc, elapsed, cw, ch, faces, frame)
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
func renderFrame(cc *gg.Context, elapsed float64, width, height int, faces [4]text.Face, frame int) {
	face28, face18, face14 := faces[1], faces[2], faces[3]

	cc.SetRGBA(0, 0, 0, 0)
	cc.Clear()

	t := elapsed * 0.8
	cx, cy := float64(width)/2, float64(height)/2

	// --- Tier 1: SDF shapes (circles, rounded rects) ---
	// Animated ring of circles (left half).
	for i := 0; i < 12; i++ {
		angle := float64(i)*math.Pi/6 + t
		x := cx/2 + math.Cos(angle)*120
		y := cy + math.Sin(angle)*120

		hue := float64(i) / 12.0
		r, g, b := hsvToRGB(hue, 0.85, 1.0)
		cc.SetRGBA(r, g, b, 0.9)

		radius := 16 + 6*math.Sin(t*2+float64(i))
		cc.DrawCircle(x, y, radius)
		cc.Fill()
	}

	// Stroked circle in center of left half.
	cc.SetRGBA(1, 1, 1, 0.3)
	cc.SetLineWidth(1.5)
	cc.DrawCircle(cx/2, cy, 120)
	cc.Stroke()

	// --- Tier 4: MSDF text comparison panel (right half) ---
	// White background panel for text samples.
	panelX := float64(width)/2 + 20
	panelY := 30.0
	panelW := float64(width)/2 - 50
	panelH := float64(height) - 60

	cc.SetRGBA(0.12, 0.12, 0.14, 1)
	cc.DrawRoundedRectangle(panelX, panelY, panelW, panelH, 12)
	cc.Fill()

	// --- Text samples at different sizes and colors (matching UI example) ---
	if face28 != nil {
		y := panelY + 50
		x := panelX + 24

		// 28px bold, white — title
		cc.SetFont(face28)
		cc.SetRGBA(1, 1, 1, 1)
		cc.DrawString("Widget Demo Title", x, y)
		y += 40

		// 18px bold, light gray — section headers
		cc.SetFont(face18)
		cc.SetRGBA(0.85, 0.85, 0.85, 1)
		cc.DrawString("Checkboxes", x, y)
		y += 28

		// 14px regular, white — checkbox labels
		cc.SetFont(face14)
		cc.SetRGBA(1, 1, 1, 1)
		cc.DrawString("Enable notifications", x, y)
		y += 22
		cc.DrawString("Dark mode", x, y)
		y += 22
		cc.DrawString("Disabled checkbox", x, y)
		y += 32

		// 18px bold, light gray — Radio Buttons header
		cc.SetFont(face18)
		cc.SetRGBA(0.85, 0.85, 0.85, 1)
		cc.DrawString("Radio Buttons", x, y)
		y += 28

		// 14px regular, white — radio labels
		cc.SetFont(face14)
		cc.SetRGBA(1, 1, 1, 1)
		cc.DrawString("Small", x, y)
		y += 22
		cc.DrawString("Medium", x, y)
		y += 22
		cc.DrawString("Large", x, y)
		y += 32

		// 14px light gray — "Horizontal Radio"
		cc.SetRGBA(0.7, 0.7, 0.7, 1)
		cc.DrawString("Horizontal Radio", x, y)
		y += 22
		cc.DrawString("Light    Dark    System", x, y)
		y += 32

		// Additional test: even smaller sizes
		cc.SetRGBA(1, 1, 1, 1)
		cc.DrawString("The quick brown fox jumps over the lazy dog", x, y)
	}

	// Title (28px) and frame counter (14px).
	if face28 != nil {
		cc.SetFont(face28)
		cc.SetRGBA(1, 1, 1, 1)
		cc.DrawStringAnchored("Four-Tier GPU Rendering", cx/2, 30, 0.5, 0)
	}
	if face14 != nil {
		cc.SetFont(face14)
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

// loadFontSource finds a system font and returns the font source.
// Returns nil if no font is available (text rendering will be skipped).
func loadFontSource() *text.FontSource {
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
	return source
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
