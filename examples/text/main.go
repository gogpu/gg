package main

import (
	"log"
	"os"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

func main() {
	// Find a system font
	fontPath := findSystemFont()
	if fontPath == "" {
		log.Println("No system font found. Skipping text example.")
		return
	}

	// Load font source (heavyweight, share across app)
	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		log.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	// Create context
	dc := gg.NewContext(800, 400)
	dc.ClearWithColor(gg.White)

	// Create faces at different sizes
	face48 := source.Face(48)
	face24 := source.Face(24)
	face16 := source.Face(16)

	// Draw title
	dc.SetFont(face48)
	dc.SetRGB(0.1, 0.1, 0.1)
	dc.DrawString("Hello, GoGPU!", 50, 80)

	// Draw subtitle
	dc.SetFont(face24)
	dc.SetRGB(0.3, 0.3, 0.3)
	dc.DrawString("Text rendering with TrueType fonts", 50, 130)

	// Draw aligned text examples
	dc.SetFont(face16)
	dc.SetRGB(0.5, 0.5, 0.5)
	dc.DrawString("Left aligned (default)", 50, 180)

	// Center aligned
	dc.SetRGB(0.2, 0.4, 0.8)
	dc.DrawStringAnchored("Center aligned", 400, 220, 0.5, 0.5)

	// Right aligned
	dc.SetRGB(0.8, 0.2, 0.2)
	dc.DrawStringAnchored("Right aligned", 750, 260, 1.0, 0.5)

	// Measure and draw with bounding box
	dc.SetFont(face24)
	testText := "Measured text"
	w, h := dc.MeasureString(testText)
	dc.SetRGB(0.9, 0.9, 0.9)
	dc.DrawRectangle(50, 290, w, h)
	dc.Fill()
	dc.SetRGB(0.1, 0.5, 0.1)
	dc.DrawString(testText, 50, 290+h*0.8) // Adjust for baseline

	// Display font info
	dc.SetFont(face16)
	dc.SetRGB(0.4, 0.4, 0.4)
	dc.DrawString("Font: "+source.Name(), 50, 370)

	// Save to PNG
	if err := dc.SavePNG("text.png"); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	log.Printf("Created text.png using font: %s", source.Name())
}

// findSystemFont returns path to a TTF font (TTC collections not supported).
func findSystemFont() string {
	// Only TTF files are supported (not TTC font collections)
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\calibri.ttf",
		"C:\\Windows\\Fonts\\segoeui.ttf",
		// macOS - Supplemental fonts are TTF
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf",
		"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		// Linux
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}
