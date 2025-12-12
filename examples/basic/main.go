package main

import (
	"log"

	"github.com/gogpu/gg"
)

func main() {
	// Create a 512x512 context
	ctx := gg.NewContext(512, 512)

	// Clear with white background
	ctx.ClearWithColor(gg.White)

	// Draw a red circle
	ctx.SetRGB(1, 0, 0)
	ctx.DrawCircle(256, 256, 100)
	ctx.Fill()

	// Draw a blue rectangle
	ctx.SetRGB(0, 0, 1)
	ctx.DrawRectangle(100, 100, 150, 100)
	ctx.Fill()

	// Draw a green stroked circle
	ctx.SetRGB(0, 1, 0)
	ctx.SetLineWidth(5)
	ctx.DrawCircle(400, 150, 50)
	ctx.Stroke()

	// Save to PNG
	if err := ctx.SavePNG("output.png"); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	log.Println("Successfully created output.png")
}
