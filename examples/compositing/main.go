// Example: Boundary Texture Compositing Test
//
// Demonstrates the UI RepaintBoundary pattern using gg compositor APIs:
//
//  1. Create offscreen GPU texture (like RepaintBoundary)
//  2. Render shapes into offscreen texture via FlushGPUWithView
//  3. Composite offscreen texture onto surface via DrawGPUTexture
//  4. Direct shapes on surface (background, animated circle)
//  5. Text rendering (DrawString)
//  6. Image rendering (DrawImage with procedural test image)
//
// This example catches "invisible boundary textures on software backend"
// bugs without requiring the full UI framework.
//
// Expected: all elements visible on every backend (GPU and software).
// If boundary textures appear as black boxes, software dispatching is broken.
//
// Press Escape to quit.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"time"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // Register GPU accelerator (SDF + MSAA + MSDF text)
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
)

const (
	winWidth  = 800
	winHeight = 500

	offscreenW = 200
	offscreenH = 150
)

func main() {
	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("gg: Boundary Compositing Test").
		WithSize(winWidth, winHeight).
		WithContinuousRender(true))

	fontFace := loadFont()

	// Create a procedural test image (checkerboard with colored quadrants).
	testImage := createTestImage(64, 64)

	var (
		canvas   *ggcanvas.Canvas
		animTime float64
		lastDraw time.Time

		// Offscreen textures (created and rendered once, composited every frame).
		offscreen1View     gpucontext.TextureView
		offscreen1Release  func()
		offscreen2View     gpucontext.TextureView
		offscreen2Release  func()
		offscreenRendered  bool

		// FPS tracking.
		fpsFrames   int
		lastFPSTick time.Time
		currentFPS  float64

		frame int
	)

	app.OnDraw(func(dc *gogpu.Context) {
		w, h := dc.Width(), dc.Height()
		if w <= 0 || h <= 0 {
			return
		}

		// Log backend on first frame.
		if frame == 0 {
			log.Printf("Backend: %s", dc.Backend())
			log.Printf("Window:  %dx%d", w, h)
		}

		// --- Canvas setup ---
		if canvas == nil {
			canvas = initCanvas(app, w, h)
			if canvas == nil {
				return
			}
		} else if cw, ch := canvas.Size(); cw != w || ch != h {
			if err := canvas.Resize(w, h); err != nil {
				log.Printf("Resize error: %v", err)
			}
		}

		// --- Animation time ---
		now := time.Now()
		if !lastDraw.IsZero() {
			dt := now.Sub(lastDraw).Seconds()
			if dt > 0.1 {
				dt = 1.0 / 60.0 // Clamp after stalls.
			}
			animTime += dt
		}
		lastDraw = now

		// --- FPS counter ---
		fpsFrames++
		if time.Since(lastFPSTick) >= time.Second {
			currentFPS = float64(fpsFrames) / time.Since(lastFPSTick).Seconds()
			fpsFrames = 0
			lastFPSTick = time.Now()
		}

		// --- Render ---
		elapsed := animTime
		fps := currentFPS
		rendered := offscreenRendered
		err := canvas.Draw(func(cc *gg.Context) {
			renderFrame(cc, elapsed, w, h, fontFace, testImage,
				&offscreen1View, &offscreen1Release,
				&offscreen2View, &offscreen2Release,
				frame, fps, rendered)
		})
		if !offscreenRendered {
			offscreenRendered = true
		}
		if err != nil {
			log.Printf("Frame %d: Draw error: %v", frame, err)
		}

		if err := canvas.Render(dc.RenderTarget()); err != nil {
			log.Printf("Frame %d: Render error: %v", frame, err)
			return
		}

		frame++
		app.RequestRedraw()
	})

	app.OnClose(func() {
		// Release offscreen textures before GPU device is destroyed.
		if offscreen1Release != nil {
			offscreen1Release()
		}
		if offscreen2Release != nil {
			offscreen2Release()
		}
		gg.CloseAccelerator()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// initCanvas creates the ggcanvas.Canvas from the app's GPU context provider.
// Returns nil if the provider is not yet available.
func initCanvas(app *gogpu.App, w, h int) *ggcanvas.Canvas {
	provider := app.GPUContextProvider()
	if provider == nil {
		return nil
	}
	c, err := ggcanvas.New(provider, w, h)
	if err != nil {
		log.Fatalf("Failed to create canvas: %v", err)
	}
	log.Printf("Canvas created: %dx%d", w, h)
	return c
}

// renderFrame draws a single frame with all compositing elements.
func renderFrame(
	cc *gg.Context,
	elapsed float64,
	w, h int,
	fontFace text.Face,
	testImage *gg.ImageBuf,
	offView1 *gpucontext.TextureView,
	offRelease1 *func(),
	offView2 *gpucontext.TextureView,
	offRelease2 *func(),
	frame int,
	fps float64,
	offscreenRendered bool,
) {
	fw, fh := float64(w), float64(h)

	// ── 1. Dark background (CPU fill) ──
	cc.FillRectCPU(0, 0, fw, fh, gg.RGBA{R: 0.06, G: 0.06, B: 0.09, A: 1})

	// Subtle grid for visual reference.
	cc.SetRGBA(0.12, 0.12, 0.18, 1)
	cc.SetLineWidth(0.5)
	for x := 0; x < w; x += 40 {
		cc.DrawLine(float64(x), 0, float64(x), fh)
		_ = cc.Stroke()
	}
	for y := 0; y < h; y += 40 {
		cc.DrawLine(0, float64(y), fw, float64(y))
		_ = cc.Stroke()
	}

	// ── 2. Animated circle (GPU SDF, Tier 1) ──
	circleX := fw*0.2 + math.Cos(elapsed*1.2)*80
	circleY := fh*0.5 + math.Sin(elapsed*0.9)*60
	radius := 30 + 10*math.Sin(elapsed*2.5)
	r, g, b := hsvToRGB(math.Mod(elapsed*0.15, 1.0), 0.8, 1.0)
	cc.SetRGBA(r, g, b, 0.9)
	cc.DrawCircle(circleX, circleY, radius)
	_ = cc.Fill()

	// Stroked orbit ring.
	cc.SetRGBA(1, 1, 1, 0.15)
	cc.SetLineWidth(1)
	cc.DrawCircle(fw*0.2, fh*0.5, 100)
	_ = cc.Stroke()

	// ── 3. Text (GPU Tier 4/6 or CPU fallback) ──
	if fontFace != nil {
		cc.SetFont(fontFace)
		cc.SetRGBA(1, 1, 1, 1)
		cc.DrawString("Boundary Compositing Test", 20, 30)

		cc.SetRGBA(0.7, 0.8, 0.7, 0.9)
		cc.DrawString("Animated circle (SDF) | Offscreen textures | DrawImage | Text", 20, 52)
	}

	// ── 4-5. Offscreen textures — render once, composite every frame ──
	// Render only on first frame to avoid per-frame GPU resource creation
	// which causes OOM on software backend (SPIR-V interpreter).
	if !offscreenRendered {
		renderOffscreen(cc, offView1, offRelease1, elapsed, 0)
		renderOffscreen(cc, offView2, offRelease2, elapsed, 1)
	}

	// ── 6. Composite offscreen textures onto main surface ──
	compositeOffscreen(cc, *offView1, 420, 60, fontFace, "Boundary 1")
	compositeOffscreen(cc, *offView2, 420, 260, fontFace, "Boundary 2")

	// ── 7. DrawImage — procedural test image ──
	if testImage != nil {
		cc.DrawImage(testImage, 20, fh-100)

		if fontFace != nil {
			cc.SetFont(fontFace)
			cc.SetRGBA(0.6, 0.6, 0.7, 0.8)
			cc.DrawString("DrawImage (64x64)", 20, fh-104)
		}
	}

	// ── 8. FPS counter ──
	if fontFace != nil {
		cc.SetFont(fontFace)
		cc.SetRGBA(0.5, 0.7, 1, 0.8)
		fpsText := fmt.Sprintf("Frame %d | %.1fs | %.0f FPS", frame, elapsed, fps)
		cc.DrawString(fpsText, fw-260, fh-12)
	}
}

// renderOffscreen creates an offscreen texture (once) and renders shapes into
// it via FlushGPUWithView. This is the RepaintBoundary pattern:
// the offscreen is a cached layer that could be reused across frames.
func renderOffscreen(
	cc *gg.Context,
	viewPtr *gpucontext.TextureView,
	releasePtr *func(),
	elapsed float64,
	variant int,
) {
	// Create offscreen texture once (persist across frames).
	if viewPtr.IsNil() {
		view, release := cc.CreateOffscreenTexture(offscreenW, offscreenH)
		if release == nil {
			// GPU not available — will draw placeholder in compositeOffscreen.
			return
		}
		*viewPtr = view
		*releasePtr = release
		log.Printf("Offscreen texture %d created: %dx%d", variant+1, offscreenW, offscreenH)
	}

	// Begin a new GPU frame for the offscreen target.
	cc.BeginGPUFrame()

	// Draw content into offscreen texture.
	// Variant 0: warm-toned shapes. Variant 1: cool-toned shapes.
	drawOffscreenContent(cc, elapsed, variant)

	// Flush to the offscreen texture view.
	err := cc.FlushGPUWithView(*viewPtr, uint32(offscreenW), uint32(offscreenH)) //nolint:gosec // offscreen fits uint32
	if err != nil {
		// Expected on software backend — FlushGPUWithView needs GPU.
		// compositeOffscreen will show placeholder text.
		return
	}
}

// drawOffscreenContent renders colored shapes into the current target.
func drawOffscreenContent(cc *gg.Context, elapsed float64, variant int) {
	fw, fh := float64(offscreenW), float64(offscreenH)

	// Background.
	if variant == 0 {
		cc.SetRGBA(0.15, 0.08, 0.05, 1) // Warm dark.
	} else {
		cc.SetRGBA(0.05, 0.08, 0.15, 1) // Cool dark.
	}
	cc.DrawRectangle(0, 0, fw, fh)
	_ = cc.Fill()

	// Border.
	if variant == 0 {
		cc.SetRGBA(0.8, 0.4, 0.1, 0.8)
	} else {
		cc.SetRGBA(0.1, 0.4, 0.8, 0.8)
	}
	cc.SetLineWidth(2)
	cc.DrawRectangle(1, 1, fw-2, fh-2)
	_ = cc.Stroke()

	// Animated circles inside the boundary.
	for i := range 5 {
		angle := float64(i)*math.Pi*2/5 + elapsed*(0.8+float64(variant)*0.3)
		cx := fw/2 + math.Cos(angle)*40
		cy := fh/2 + math.Sin(angle)*30

		var hueBase float64
		if variant == 0 {
			hueBase = 0.05 // Orange-red.
		} else {
			hueBase = 0.55 // Blue-cyan.
		}
		hue := hueBase + float64(i)*0.04
		r, g, b := hsvToRGB(hue, 0.9, 1.0)
		cc.SetRGBA(r, g, b, 0.85)
		cc.DrawCircle(cx, cy, 12+4*math.Sin(elapsed*2+float64(i)))
		_ = cc.Fill()
	}

	// Center shape: rounded rect for variant 0, regular circle for variant 1.
	if variant == 0 {
		cc.SetRGBA(1, 0.7, 0.2, 0.6)
		cc.DrawRoundedRectangle(fw/2-25, fh/2-20, 50, 40, 8)
		_ = cc.Fill()
	} else {
		cc.SetRGBA(0.2, 0.7, 1, 0.6)
		cc.DrawCircle(fw/2, fh/2, 25)
		_ = cc.Fill()
	}
}

// compositeOffscreen draws the offscreen texture onto the main surface,
// or a placeholder if the texture is not available.
func compositeOffscreen(
	cc *gg.Context,
	view gpucontext.TextureView,
	x, y float64,
	fontFace text.Face,
	label string,
) {
	if !view.IsNil() {
		// GPU path: composite offscreen texture as overlay.
		cc.DrawGPUTexture(view, x, y, offscreenW, offscreenH)
	} else {
		// Software fallback: draw placeholder rectangle.
		cc.SetRGBA(0.2, 0.2, 0.25, 1)
		cc.DrawRoundedRectangle(x, y, float64(offscreenW), float64(offscreenH), 4)
		_ = cc.Fill()

		cc.SetRGBA(0.6, 0.4, 0.3, 0.9)
		cc.SetLineWidth(1)
		cc.DrawRoundedRectangle(x, y, float64(offscreenW), float64(offscreenH), 4)
		_ = cc.Stroke()

		if fontFace != nil {
			cc.SetFont(fontFace)
			cc.SetRGBA(0.8, 0.6, 0.4, 0.9)
			cc.DrawString("Offscreen N/A", x+40, y+float64(offscreenH)/2+4)
		}
	}

	// Label above the boundary texture.
	if fontFace != nil {
		cc.SetFont(fontFace)
		cc.SetRGBA(0.7, 0.7, 0.8, 0.8)
		cc.DrawString(label, x, y-6)
	}
}

// createTestImage generates a 4-quadrant checkerboard test image.
func createTestImage(w, h int) *gg.ImageBuf {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	halfW, halfH := w/2, h/2

	for y := range h {
		for x := range w {
			var c color.RGBA
			checker := ((x/8)+(y/8))%2 == 0

			switch {
			case x < halfW && y < halfH: // Top-left: red/dark
				if checker {
					c = color.RGBA{R: 220, G: 60, B: 60, A: 255}
				} else {
					c = color.RGBA{R: 80, G: 20, B: 20, A: 255}
				}
			case x >= halfW && y < halfH: // Top-right: green/dark
				if checker {
					c = color.RGBA{R: 60, G: 200, B: 60, A: 255}
				} else {
					c = color.RGBA{R: 20, G: 70, B: 20, A: 255}
				}
			case x < halfW && y >= halfH: // Bottom-left: blue/dark
				if checker {
					c = color.RGBA{R: 60, G: 60, B: 220, A: 255}
				} else {
					c = color.RGBA{R: 20, G: 20, B: 80, A: 255}
				}
			default: // Bottom-right: yellow/dark
				if checker {
					c = color.RGBA{R: 220, G: 200, B: 60, A: 255}
				} else {
					c = color.RGBA{R: 80, G: 70, B: 20, A: 255}
				}
			}
			img.SetRGBA(x, y, c)
		}
	}

	return gg.ImageBufFromImage(img)
}

// loadFont finds and loads a system TTF font.
// Returns nil if no font is available (text rendering will be skipped).
func loadFont() text.Face {
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
		if _, err := os.Stat(path); err != nil {
			continue
		}
		source, err := text.NewFontSourceFromFile(path)
		if err != nil {
			continue
		}
		log.Printf("Loaded font: %s", source.Name())
		return source.Face(14)
	}

	log.Println("No system font found — text rendering disabled.")
	return nil
}

// hsvToRGB converts HSV (all 0-1) to RGB (all 0-1).
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
