package main

import (
	"log"

	"github.com/gogpu/gg"
)

func main() {
	// Create a 512x512 context
	dc := gg.NewContext(512, 512)

	// Clear with white background
	dc.ClearWithColor(gg.White)

	// Draw a red circle
	dc.SetRGB(1, 0, 0)
	dc.DrawCircle(256, 256, 100)
	dc.Fill()

	// Draw a blue rectangle
	dc.SetRGB(0, 0, 1)
	dc.DrawRectangle(100, 100, 150, 100)
	dc.Fill()

	// Draw a green stroked circle
	dc.SetRGB(0, 1, 0)
	dc.SetLineWidth(5)
	dc.DrawCircle(400, 150, 50)
	dc.Stroke()

	// Save to PNG
	if err := dc.SavePNG("output.png"); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	log.Println("Successfully created output.png")
}
