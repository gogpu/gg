// Example: SDF Accelerator Demo
//
// This example demonstrates the SDF (Signed Distance Field) accelerator
// for smooth circle and rounded rectangle rendering. It shows:
//   - Registering the CPU SDF accelerator
//   - Drawing filled and stroked circles with smooth anti-aliasing
//   - Drawing filled and stroked rounded rectangles
//   - Comparing SDF vs standard rasterizer quality
package main

import (
	"fmt"
	"log"

	"github.com/gogpu/gg"
)

func main() {
	// Register the CPU SDF accelerator.
	// This gives smoother circles and rounded rectangles than the default
	// area-based rasterizer by computing per-pixel signed distance fields.
	if err := gg.RegisterAccelerator(&gg.SDFAccelerator{}); err != nil {
		log.Fatalf("Failed to register SDF accelerator: %v", err)
	}

	const (
		width  = 512
		height = 512
	)

	dc := gg.NewContext(width, height)

	// Dark background
	dc.SetRGB(0.086, 0.129, 0.243)
	dc.Clear()

	// === Filled shapes ===

	// Large filled circle (red)
	dc.SetRGBA(0.9, 0.2, 0.2, 1.0)
	dc.DrawCircle(128, 128, 80)
	dc.Fill()

	// Medium filled circle (green)
	dc.SetRGBA(0.2, 0.8, 0.3, 1.0)
	dc.DrawCircle(384, 128, 60)
	dc.Fill()

	// Filled rounded rectangle (blue)
	dc.SetRGBA(0.2, 0.4, 0.9, 1.0)
	dc.DrawRoundedRectangle(28, 314, 200, 140, 20)
	dc.Fill()

	// Filled rectangle (orange)
	dc.SetRGBA(0.9, 0.6, 0.1, 1.0)
	dc.DrawRectangle(304, 324, 160, 120)
	dc.Fill()

	// === Stroked shapes ===

	// Stroked circle (cyan, 3px)
	dc.SetRGBA(0.0, 0.9, 0.9, 1.0)
	dc.SetLineWidth(3.0)
	dc.DrawCircle(256, 256, 50)
	dc.Stroke()

	// Stroked rounded rectangle (yellow, 2px)
	dc.SetRGBA(1.0, 0.9, 0.2, 1.0)
	dc.SetLineWidth(2.0)
	dc.DrawRoundedRectangle(166, 191, 180, 130, 15)
	dc.Stroke()

	// Save output
	if err := dc.SavePNG("sdf_output.png"); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	fmt.Println("SDF Accelerator Demo")
	fmt.Println("====================")
	fmt.Printf("Rendered %dx%d canvas with 6 shapes (4 filled + 2 stroked)\n", width, height)
	fmt.Println("Saved: sdf_output.png")
	fmt.Println()
	fmt.Println("The SDF accelerator produces smoother edges for circles and")
	fmt.Println("rounded rectangles compared to the default area-based rasterizer.")
}
