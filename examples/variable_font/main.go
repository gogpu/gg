// Example: Variable Font rendering with different weights and named instances.
//
// Demonstrates the WithVariations() API for OpenType variable fonts.
// Uses Bahnschrift (Windows), SF Pro (macOS), or DejaVu Sans (Linux).
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

func main() {
	source := loadVariableFont()
	if source == nil {
		log.Fatal("no variable font found on this system")
	}

	fmt.Printf("Font: %s (variable: %v)\n", source.Name(), source.IsVariable())
	for _, axis := range source.VariationAxes() {
		fmt.Printf("  %s: %.0f – %.0f (default %.0f)\n",
			string(axis.Tag[:]), axis.Minimum, axis.Maximum, axis.Default)
	}

	dc := gg.NewContext(800, 500)
	dc.SetRGB(1, 1, 1)
	dc.DrawRectangle(0, 0, 800, 500)
	dc.Fill()

	dc.SetRGB(0, 0, 0)
	y := 50.0

	// Render at multiple weights from the same font file.
	weights := []float32{300, 400, 500, 600, 700}
	for _, w := range weights {
		face := source.Face(28, text.WithVariations(
			text.NewFontVariation("wght", w),
		))
		dc.SetFont(face)
		label := fmt.Sprintf("wght=%.0f: The quick brown fox jumps over the lazy dog", w)
		dc.DrawString(label, 30, y)
		y += 45
	}

	y += 20

	// Render named instances (font-designer presets).
	instances := source.NamedInstances()
	if len(instances) > 0 {
		dc.SetFont(source.Face(16))
		dc.SetRGBA(0.5, 0.5, 0.5, 1)
		dc.DrawString("Named Instances:", 30, y)
		y += 30

		dc.SetRGB(0, 0, 0)
		for _, inst := range instances {
			face := source.Face(24, text.WithVariations(inst.Variations...))
			dc.SetFont(face)
			dc.DrawString(fmt.Sprintf("%s: Hello, World!", inst.Name), 30, y)
			y += 35
		}
	}

	out := "variable_font.png"
	if err := dc.SavePNG(out); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Saved: %s\n", out)
}

func loadVariableFont() *text.FontSource {
	candidates := []string{
		// Windows — Bahnschrift is a variable font (wght, wdth axes)
		`C:\Windows\Fonts\bahnschrift.ttf`,
		// macOS — SF Pro is variable (requires download from Apple)
		"/Library/Fonts/SF-Pro.ttf",
		"/System/Library/Fonts/SFPro.ttf",
		// Linux — check for common variable fonts
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		source, err := text.NewFontSourceFromFile(path)
		if err != nil {
			continue
		}
		if source.IsVariable() {
			return source
		}
	}
	return nil
}
