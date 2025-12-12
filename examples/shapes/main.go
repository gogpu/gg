package main

import (
	"log"
	"math"

	"github.com/gogpu/gg"
)

func main() {
	const (
		width  = 800
		height = 600
	)

	ctx := gg.NewContext(width, height)

	// White background
	ctx.ClearWithColor(gg.White)

	// Draw various shapes
	drawShapes(ctx)

	// Save result
	if err := ctx.SavePNG("shapes.png"); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	log.Println("Created shapes.png")
}

func drawShapes(ctx *gg.Context) {
	// Rectangle
	ctx.SetRGB(0.8, 0.2, 0.2)
	ctx.DrawRectangle(50, 50, 150, 100)
	ctx.Fill()

	// Rounded rectangle
	ctx.SetRGB(0.2, 0.8, 0.2)
	ctx.DrawRoundedRectangle(250, 50, 150, 100, 20)
	ctx.Fill()

	// Circle
	ctx.SetRGB(0.2, 0.2, 0.8)
	ctx.DrawCircle(500, 100, 60)
	ctx.Fill()

	// Ellipse
	ctx.SetRGB(0.8, 0.8, 0.2)
	ctx.DrawEllipse(650, 100, 80, 50)
	ctx.Fill()

	// Regular polygons
	ctx.SetRGB(1, 0.5, 0)
	ctx.DrawRegularPolygon(5, 100, 300, 50, -math.Pi/2) // Pentagon
	ctx.Fill()

	ctx.SetRGB(0.5, 0, 1)
	ctx.DrawRegularPolygon(6, 250, 300, 50, 0) // Hexagon
	ctx.Fill()

	ctx.SetRGB(0, 0.8, 0.8)
	ctx.DrawRegularPolygon(8, 400, 300, 50, 0) // Octagon
	ctx.Fill()

	// Lines
	ctx.SetRGB(0, 0, 0)
	ctx.SetLineWidth(3)
	ctx.DrawLine(50, 450, 750, 450)
	ctx.Stroke()

	// Arc
	ctx.SetRGB(0.8, 0, 0.8)
	ctx.SetLineWidth(5)
	ctx.DrawArc(650, 300, 60, 0, math.Pi*1.5)
	ctx.Stroke()

	// Transformed shapes
	ctx.Push()
	ctx.Translate(400, 500)
	ctx.Rotate(math.Pi / 4)
	ctx.SetRGB(0.2, 0.6, 0.8)
	ctx.DrawRectangle(-40, -40, 80, 80)
	ctx.Fill()
	ctx.Pop()
}
