package main

import (
	"log"

	"github.com/gogpu/gg"
)

func main() {
	ctx := gg.NewContext(400, 300)

	// White background
	ctx.ClearWithColor(gg.White)

	// Draw a placeholder message since text is not implemented in v0.1
	ctx.SetRGB(0.5, 0.5, 0.5)
	ctx.DrawRectangle(50, 125, 300, 50)
	ctx.Fill()

	// Note: Text rendering will be implemented in v0.2.0+
	// For now, this example just shows the structure
	// ctx.DrawString("Hello, World!", 200, 150)

	if err := ctx.SavePNG("text.png"); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	log.Println("Created text.png (text rendering coming in v0.2.0)")
}
