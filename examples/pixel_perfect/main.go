package main

import (
	"log"

	"github.com/gogpu/gg"
)

func main() {
	dc := gg.NewContext(512, 256)
	dc.ClearWithColor(gg.White)

	// Left half: Anti-Aliased (default)
	drawShapes(dc, 0)

	// Right half: Pixel-Perfect (no AA)
	dc.SetAntiAlias(false)
	drawShapes(dc, 256)

	// Labels
	dc.SetAntiAlias(true)
	dc.SetRGB(0, 0, 0)
	dc.DrawStringAnchored("Anti-Aliased", 128, 240, 0.5, 0.5)
	dc.DrawStringAnchored("Pixel-Perfect", 384, 240, 0.5, 0.5)

	if err := dc.SavePNG("pixel_perfect.png"); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	log.Println("Created pixel_perfect.png — compare left (AA) vs right (no-AA)")
}

func drawShapes(dc *gg.Context, offsetX float64) {
	// Diagonal line
	dc.SetRGB(1, 0, 0)
	dc.SetLineWidth(1)
	dc.DrawLine(offsetX+20, 20, offsetX+100, 80)
	dc.Stroke()

	// Circle
	dc.SetRGB(0, 0, 1)
	dc.DrawCircle(offsetX+180, 60, 30)
	dc.Fill()

	// Rectangle (axis-aligned)
	dc.SetRGB(0, 0.6, 0)
	dc.DrawRectangle(offsetX+30, 110, 80, 50)
	dc.Fill()

	// Stroked circle
	dc.SetRGB(0.5, 0, 0.5)
	dc.SetLineWidth(2)
	dc.DrawCircle(offsetX+180, 150, 25)
	dc.Stroke()

	// Grid lines (1px)
	dc.SetRGB(0.3, 0.3, 0.3)
	dc.SetLineWidth(1)
	for i := 0; i < 5; i++ {
		y := float64(185 + i*10)
		dc.DrawLine(offsetX+30, y, offsetX+230, y)
		dc.Stroke()
	}
}
