// Example: LCD ClearType subpixel text rendering
//
// Demonstrates LCD subpixel text via GPU Tier 6 glyph mask pipeline.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
)

func main() {
	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("LCD ClearType Text Demo").
		WithSize(700, 500))

	fontSource := loadFont()
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
				log.Fatalf("ggcanvas: %v", err)
			}
		}

		cw, ch := canvas.Size()
		if cw != w || ch != h {
			_ = canvas.Resize(w, h)
		}

		_ = canvas.Draw(func(cc *gg.Context) {
			// White background
			cc.SetRGB(1, 1, 1)
			cc.DrawRectangle(0, 0, float64(w), float64(h))
			_ = cc.Fill()

			// Enable LCD subpixel rendering
			cc.SetLCDLayout(gg.LCDLayoutRGB)

			cc.SetRGB(0, 0, 0)

			if fontSource != nil {
				log.Println("font loaded, drawing text with LCD")
				sizes := []float64{12, 16, 20, 24, 32}
				y := 40.0
				for _, size := range sizes {
					face := fontSource.Face(size)
					cc.SetFont(face)
					cc.DrawString(fmt.Sprintf("%.0fpx: The quick brown fox jumps", size), 20, y)
					y += size*1.5 + 4
				}
			} else {
				log.Println("NO font - drawing shapes only")
				cc.DrawCircle(200, 200, 50)
				_ = cc.Fill()
			}
		})

		if err := canvas.Render(dc.RenderTarget()); err != nil {
			log.Printf("render: %v", err)
		}
	})

	app.OnClose(func() {
		gg.CloseAccelerator()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func loadFont() *text.FontSource {
	paths := []string{
		`C:/Windows/Fonts/segoeui.ttf`,
		`C:WindowsFontsrial.ttf`,
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
