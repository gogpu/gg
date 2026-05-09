// Example: LCD ClearType subpixel text rendering
//
// Demonstrates LCD subpixel text via GPU Tier 6 glyph mask pipeline.
// ClearType is auto-detected from the OS via PlatformProvider (ADR-024).
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // GPU accelerator: text goes through Tier 6 glyph mask
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
)

func main() {
	fontSource := loadFont()
	if fontSource != nil {
		log.Printf("Font loaded: %s", fontSource.Name())
	} else {
		log.Println("WARNING: No font loaded")
	}

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("LCD ClearType Text — ADR-024").
		WithSize(700, 400).
		WithContinuousRender(false))

	var canvas *ggcanvas.Canvas

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

		if err := canvas.Draw(func(cc *gg.Context) {
			// White background via GPU fill (CPU Clear() is invisible in GPU-direct mode).
			cc.SetRGBA(0.97, 0.97, 0.97, 1)
			cc.DrawRectangle(0, 0, float64(w), float64(h))
			_ = cc.Fill()

			if fontSource == nil {
				return
			}

			cc.SetRGB(0.1, 0.1, 0.1)
			sizes := []float64{12, 14, 16, 20, 24, 32}
			y := 40.0
			for _, size := range sizes {
				cc.SetFont(fontSource.Face(size))
				cc.DrawString(fmt.Sprintf("%.0fpx: The quick brown fox jumps over the lazy dog", size), 20, y)
				y += size*1.5 + 6
			}
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

func loadFont() *text.FontSource {
	paths := []string{
		`C:\Windows\Fonts\segoeui.ttf`,
		`C:\Windows\Fonts\arial.ttf`,
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			src, err := text.NewFontSourceFromFile(p)
			if err == nil {
				return src
			}
		}
	}
	return nil
}
