// Command ggdemo demonstrates the gg 2D graphics library.
package main

import (
	"flag"
	"log"
	"math"

	"github.com/gogpu/gg"
)

func main() {
	var (
		width  = flag.Int("width", 800, "image width")
		height = flag.Int("height", 600, "image height")
		output = flag.String("output", "demo.png", "output file")
	)
	flag.Parse()

	dc := gg.NewContext(*width, *height)

	// Gradient background (simulated with rectangles)
	drawGradientBackground(dc, *width, *height)

	// Draw various demonstrations
	drawShapesDemo(dc)
	drawTransformDemo(dc)
	drawPathDemo(dc)

	// Save result
	if err := dc.SavePNG(*output); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	log.Printf("Demo saved to %s (%dx%d)\n", *output, *width, *height)
}

func drawGradientBackground(dc *gg.Context, w, h int) {
	steps := 100
	for i := 0; i < steps; i++ {
		t := float64(i) / float64(steps)
		c := gg.RGB(0.1+t*0.4, 0.2+t*0.3, 0.4+t*0.2)
		dc.SetColor(c)
		y := float64(h) * t
		dc.DrawRectangle(0, y, float64(w), float64(h)/float64(steps)+1)
		_ = dc.Fill()
	}
}

func drawShapesDemo(dc *gg.Context) {
	// Circles
	dc.SetRGBA(1, 0.3, 0.3, 0.8)
	dc.DrawCircle(150, 150, 60)
	_ = dc.Fill()

	dc.SetRGBA(0.3, 1, 0.3, 0.8)
	dc.DrawCircle(200, 150, 60)
	_ = dc.Fill()

	dc.SetRGBA(0.3, 0.3, 1, 0.8)
	dc.DrawCircle(175, 200, 60)
	_ = dc.Fill()

	// Rectangles
	dc.SetRGB(1, 0.8, 0)
	dc.DrawRoundedRectangle(350, 100, 120, 80, 15)
	_ = dc.Fill()

	// Stroked shapes
	dc.SetRGB(1, 1, 1)
	dc.SetLineWidth(4)
	dc.DrawRectangle(350, 100, 120, 80)
	_ = dc.Stroke()
}

func drawTransformDemo(dc *gg.Context) {
	// Rotated squares
	centerX := 600.0
	centerY := 150.0

	for i := 0; i < 8; i++ {
		angle := float64(i) * math.Pi / 4
		dc.Push()
		dc.Translate(centerX, centerY)
		dc.Rotate(angle)

		// Color based on rotation
		hue := float64(i) * 45
		c := gg.HSL(hue, 0.8, 0.6)
		dc.SetColor(c)

		dc.DrawRectangle(-30, -30, 60, 60)
		_ = dc.Fill()
		dc.Pop()
	}
}

func drawPathDemo(dc *gg.Context) {
	// Complex path with curves
	dc.Push()
	dc.Translate(150, 400)

	dc.SetRGB(1, 0.5, 0)
	dc.MoveTo(0, 0)
	dc.CubicTo(50, -50, 100, 50, 150, 0)
	dc.CubicTo(200, -30, 250, 30, 300, 0)
	dc.SetLineWidth(6)
	_ = dc.Stroke()

	// Polygon star
	dc.Translate(400, 0)
	dc.SetRGB(1, 1, 0)

	points := 5
	outerR := 60.0
	innerR := 30.0

	for i := 0; i < points*2; i++ {
		angle := float64(i) * math.Pi / float64(points)
		r := outerR
		if i%2 == 1 {
			r = innerR
		}
		x := r * math.Cos(angle-math.Pi/2)
		y := r * math.Sin(angle-math.Pi/2)

		if i == 0 {
			dc.MoveTo(x, y)
		} else {
			dc.LineTo(x, y)
		}
	}
	dc.ClosePath()
	_ = dc.Fill()

	dc.Pop()
}
