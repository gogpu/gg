// Package main demonstrates image drawing capabilities in gg.
package main

import (
	"fmt"
	"image"
	"log"

	"github.com/gogpu/gg"
)

func main() {
	// Create a 800x600 canvas
	const width, height = 800, 600
	dc := gg.NewContext(width, height)

	// Fill background with light gray
	dc.SetRGB(0.95, 0.95, 0.95)
	dc.Clear()

	// Example 1: Create a simple test image programmatically
	testImg, err := createTestImage(100, 100)
	if err != nil {
		log.Fatalf("Failed to create test image: %v", err)
	}

	// Draw the image at different positions with different options
	drawExamples(dc, testImg)

	// Save the result
	if err := dc.SavePNG("examples/images/output.png"); err != nil {
		log.Fatalf("Failed to save image: %v", err)
	}

	fmt.Println("Image drawing example completed successfully!")
	fmt.Println("Output saved to: examples/images/output.png")
}

// createTestImage creates a simple colorful test pattern.
func createTestImage(width, height int) (*gg.ImageBuf, error) {
	img, err := gg.NewImageBuf(width, height, gg.FormatRGBA8)
	if err != nil {
		return nil, fmt.Errorf("create image buffer: %w", err)
	}

	// Create a gradient pattern
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := uint8(x * 255 / width)
			g := uint8(y * 255 / height)
			b := uint8((x + y) * 128 / (width + height))
			_ = img.SetRGBA(x, y, r, g, b, 255)
		}
	}

	return img, nil
}

// drawExamples demonstrates various image drawing techniques.
func drawExamples(dc *gg.Context, img *gg.ImageBuf) {
	// Title
	dc.SetRGB(0.2, 0.2, 0.2)
	drawText(dc, "gg Image Drawing Examples", 400, 30)

	// Example 1: Basic DrawImage
	drawText(dc, "1. Basic DrawImage", 100, 80)
	dc.DrawImage(img, 50, 100)

	// Example 2: Scaled image with bilinear interpolation
	drawText(dc, "2. Scaled (2x) - Bilinear", 100, 240)
	dc.DrawImageEx(img, gg.DrawImageOptions{
		X:             50,
		Y:             260,
		DstWidth:      200,
		DstHeight:     200,
		Interpolation: gg.InterpBilinear,
		Opacity:       1.0,
		BlendMode:     gg.BlendNormal,
	})

	// Example 3: With opacity
	drawText(dc, "3. Opacity (50%)", 350, 80)
	dc.DrawImageEx(img, gg.DrawImageOptions{
		X:             300,
		Y:             100,
		Interpolation: gg.InterpBilinear,
		Opacity:       0.5,
		BlendMode:     gg.BlendNormal,
	})

	// Example 4: Nearest neighbor interpolation (pixelated look)
	drawText(dc, "4. Scaled - Nearest", 550, 80)
	dc.DrawImageEx(img, gg.DrawImageOptions{
		X:             500,
		Y:             100,
		DstWidth:      150,
		DstHeight:     150,
		Interpolation: gg.InterpNearest,
		Opacity:       1.0,
		BlendMode:     gg.BlendNormal,
	})

	// Example 5: Source rectangle (draw only part of image)
	drawText(dc, "5. Source Rect (top-left quadrant)", 350, 240)
	srcRect := image.Rect(0, 0, 50, 50)
	dc.DrawImageEx(img, gg.DrawImageOptions{
		X:             300,
		Y:             260,
		DstWidth:      100,
		DstHeight:     100,
		SrcRect:       &srcRect,
		Interpolation: gg.InterpBilinear,
		Opacity:       1.0,
		BlendMode:     gg.BlendNormal,
	})

	// Example 6: Multiply blend mode
	drawText(dc, "6. Blend Mode: Multiply", 550, 240)
	// First draw a colored rectangle as background
	dc.SetRGB(0.8, 0.6, 0.4)
	dc.DrawRectangle(500, 260, 100, 100)
	dc.Fill()
	// Then draw image with multiply blend
	dc.DrawImageEx(img, gg.DrawImageOptions{
		X:             500,
		Y:             260,
		Interpolation: gg.InterpBilinear,
		Opacity:       1.0,
		BlendMode:     gg.BlendMultiply,
	})

	// Example 7: Rotated and scaled with transformation matrix
	drawText(dc, "7. Transform (rotate + scale)", 100, 490)
	dc.Push()
	dc.Translate(150, 540)
	dc.Rotate(0.3) // ~17 degrees
	dc.Scale(0.8, 0.8)
	dc.DrawImage(img, -50, -50)
	dc.Pop()

	// Example 8: Image pattern for fills
	drawText(dc, "8. Image Pattern Fill", 350, 490)
	pattern := dc.CreateImagePattern(img, 0, 0, 50, 50)
	dc.SetFillPattern(pattern)
	dc.DrawCircle(450, 540, 60)
	dc.Fill()
}

// drawText is a simple helper to draw text (would use actual text rendering in production).
// For this example, we just draw a rectangle as a placeholder since text rendering
// might not be available yet.
func drawText(dc *gg.Context, _ string, x, y float64) {
	// In a real application, you would use dc.DrawString or similar
	// For now, just draw a small marker to indicate where text would go
	dc.SetRGB(0.2, 0.2, 0.2)
	dc.DrawCircle(x-10, y, 3)
	dc.Fill()
}
