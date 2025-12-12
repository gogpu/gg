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

	ctx := gg.NewContext(*width, *height)

	// Gradient background (simulated with rectangles)
	drawGradientBackground(ctx, *width, *height)

	// Draw various demonstrations
	drawShapesDemo(ctx)
	drawTransformDemo(ctx)
	drawPathDemo(ctx)

	// Save result
	if err := ctx.SavePNG(*output); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	log.Printf("Demo saved to %s (%dx%d)\n", *output, *width, *height)
}

func drawGradientBackground(ctx *gg.Context, w, h int) {
	steps := 100
	for i := 0; i < steps; i++ {
		t := float64(i) / float64(steps)
		color := gg.RGB(0.1+t*0.4, 0.2+t*0.3, 0.4+t*0.2)
		ctx.SetColor(color.Color())
		y := float64(h) * t
		ctx.DrawRectangle(0, y, float64(w), float64(h)/float64(steps)+1)
		ctx.Fill()
	}
}

func drawShapesDemo(ctx *gg.Context) {
	// Circles
	ctx.SetRGBA(1, 0.3, 0.3, 0.8)
	ctx.DrawCircle(150, 150, 60)
	ctx.Fill()

	ctx.SetRGBA(0.3, 1, 0.3, 0.8)
	ctx.DrawCircle(200, 150, 60)
	ctx.Fill()

	ctx.SetRGBA(0.3, 0.3, 1, 0.8)
	ctx.DrawCircle(175, 200, 60)
	ctx.Fill()

	// Rectangles
	ctx.SetRGB(1, 0.8, 0)
	ctx.DrawRoundedRectangle(350, 100, 120, 80, 15)
	ctx.Fill()

	// Stroked shapes
	ctx.SetRGB(1, 1, 1)
	ctx.SetLineWidth(4)
	ctx.DrawRectangle(350, 100, 120, 80)
	ctx.Stroke()
}

func drawTransformDemo(ctx *gg.Context) {
	// Rotated squares
	centerX := 600.0
	centerY := 150.0

	for i := 0; i < 8; i++ {
		angle := float64(i) * math.Pi / 4
		ctx.Push()
		ctx.Translate(centerX, centerY)
		ctx.Rotate(angle)

		// Color based on rotation
		hue := float64(i) * 45
		color := gg.HSL(hue, 0.8, 0.6)
		ctx.SetColor(color.Color())

		ctx.DrawRectangle(-30, -30, 60, 60)
		ctx.Fill()
		ctx.Pop()
	}
}

func drawPathDemo(ctx *gg.Context) {
	// Complex path with curves
	ctx.Push()
	ctx.Translate(150, 400)

	ctx.SetRGB(1, 0.5, 0)
	ctx.MoveTo(0, 0)
	ctx.CubicTo(50, -50, 100, 50, 150, 0)
	ctx.CubicTo(200, -30, 250, 30, 300, 0)
	ctx.SetLineWidth(6)
	ctx.Stroke()

	// Polygon star
	ctx.Translate(400, 0)
	ctx.SetRGB(1, 1, 0)

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
			ctx.MoveTo(x, y)
		} else {
			ctx.LineTo(x, y)
		}
	}
	ctx.ClosePath()
	ctx.Fill()

	ctx.Pop()
}
