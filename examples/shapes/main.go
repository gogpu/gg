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

	dc := gg.NewContext(width, height)

	// White background
	dc.ClearWithColor(gg.White)

	// Draw various shapes
	drawShapes(dc)

	// Save result
	if err := dc.SavePNG("shapes.png"); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	log.Println("Created shapes.png")
}

func drawShapes(dc *gg.Context) {
	// Rectangle
	dc.SetRGB(0.8, 0.2, 0.2)
	dc.DrawRectangle(50, 50, 150, 100)
	dc.Fill()

	// Rounded rectangle
	dc.SetRGB(0.2, 0.8, 0.2)
	dc.DrawRoundedRectangle(250, 50, 150, 100, 20)
	dc.Fill()

	// Circle
	dc.SetRGB(0.2, 0.2, 0.8)
	dc.DrawCircle(500, 100, 60)
	dc.Fill()

	// Ellipse
	dc.SetRGB(0.8, 0.8, 0.2)
	dc.DrawEllipse(650, 100, 80, 50)
	dc.Fill()

	// Regular polygons
	dc.SetRGB(1, 0.5, 0)
	dc.DrawRegularPolygon(5, 100, 300, 50, -math.Pi/2) // Pentagon
	dc.Fill()

	dc.SetRGB(0.5, 0, 1)
	dc.DrawRegularPolygon(6, 250, 300, 50, 0) // Hexagon
	dc.Fill()

	dc.SetRGB(0, 0.8, 0.8)
	dc.DrawRegularPolygon(8, 400, 300, 50, 0) // Octagon
	dc.Fill()

	// Lines
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(3)
	dc.DrawLine(50, 450, 750, 450)
	dc.Stroke()

	// Arc
	dc.SetRGB(0.8, 0, 0.8)
	dc.SetLineWidth(5)
	dc.DrawArc(650, 300, 60, 0, math.Pi*1.5)
	dc.Stroke()

	// Transformed shapes
	dc.Push()
	dc.Translate(400, 500)
	dc.Rotate(math.Pi / 4)
	dc.SetRGB(0.2, 0.6, 0.8)
	dc.DrawRectangle(-40, -40, 80, 80)
	dc.Fill()
	dc.Pop()
}
