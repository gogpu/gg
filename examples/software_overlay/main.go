// Minimal test: DrawGPUTexture on software backend.
//
// Creates a bright red offscreen texture, composites it at (100, 100).
// If visible = compositing works. If invisible = bug in readback/present.
//
// Run: GOGPU_GRAPHICS_API=software go run .
package main

import (
	"log"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
)

func main() {
	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("Software Overlay Test").
		WithSize(400, 300))

	var (
		canvas      *ggcanvas.Canvas
		offView     gpucontext.TextureView
		offRelease  func()
		offRendered bool
		frame       int
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
				log.Fatal(err)
			}
			log.Printf("Backend: %s, canvas %dx%d", dc.Backend(), w, h)
		}

		err := canvas.Draw(func(cc *gg.Context) {
			fw, fh := float64(w), float64(h)

			// 1. Blue background (CPU)
			cc.FillRectCPU(0, 0, fw, fh, gg.RGBA{R: 0, G: 0, B: 0.3, A: 1})

			// 2. White rectangle (CPU shape)
			cc.SetRGBA(1, 1, 1, 1)
			cc.DrawRectangle(10, 10, 100, 20)
			_ = cc.Fill()

			// 3. Create + render offscreen (once)
			if !offRendered {
				if offView.IsNil() {
					view, release := cc.CreateOffscreenTexture(150, 100)
					if release != nil {
						offView = view
						offRelease = release
						log.Println("Offscreen texture created")
					}
				}
				if !offView.IsNil() {
					cc.BeginGPUFrame()
					cc.SetRGBA(1, 0, 0, 1)
					cc.DrawRectangle(0, 0, 150, 100)
					_ = cc.Fill()
					cc.SetRGBA(0, 1, 0, 1)
					cc.DrawCircle(75, 50, 30)
					_ = cc.Fill()

					if err := cc.FlushGPUWithView(offView, 150, 100); err != nil {
						log.Printf("FlushGPUWithView: %v", err)
					} else {
						log.Println("Offscreen rendered OK")
					}
				}
				offRendered = true
			}

			// 4. Composite offscreen at (100, 100)
			if !offView.IsNil() {
				cc.DrawGPUTexture(offView, 100, 100, 150, 100)
			}

			// 5. Yellow marker at expected overlay position
			cc.SetRGBA(1, 1, 0, 1)
			cc.SetLineWidth(2)
			cc.DrawRectangle(99, 99, 152, 102)
			_ = cc.Stroke()
		})
		if err != nil {
			log.Printf("Draw: %v", err)
		}

		if err := canvas.Render(dc.RenderTarget()); err != nil {
			log.Printf("Render: %v", err)
		}

		frame++
		if frame < 5 {
			app.RequestRedraw()
		}
	})

	app.OnClose(func() {
		if offRelease != nil {
			offRelease()
		}
		gg.CloseAccelerator()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
