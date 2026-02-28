// Package main demonstrates the CPU hybrid text transform pipeline (TEXT-002).
//
// It renders a 3x3 grid of "Hello gg!" text under different affine transforms:
// identity, translate, uniform scale, downscale, non-uniform scale, rotation,
// 45-degree rotation, faux-italic shear, and combined scale+rotate.
//
// The three CPU rendering tiers are exercised automatically:
//
//	Tier 0 (bitmap fast path)   - identity, translate
//	Tier 1 (scaled bitmap)      - uniform scale (0.7, 2.0)
//	Tier 2 (glyph outlines)     - rotation, shear, non-uniform scale, combined
//
// Output: text_transform.png (900x700)
package main

import (
	"fmt"
	"log"
	"math"
	"os"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

const (
	canvasW = 900
	canvasH = 700

	cols = 3
	rows = 3

	cellW = 280.0
	cellH = 210.0

	// Padding from canvas edges to the grid area.
	gridLeft = 25.0
	gridTop  = 70.0

	// Inset within each cell for the text drawing origin.
	textOffX = 30.0
	textOffY = 100.0
)

// cell describes one grid entry: a human-readable label and a function that
// applies the desired transform on the drawing context before text is drawn.
type cell struct {
	label     string
	transform func(dc *gg.Context)
}

func main() {
	fontPath := findSystemFont()
	if fontPath == "" {
		log.Println("No system font found. Skipping text_transform example.")
		return
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		log.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc := gg.NewContext(canvasW, canvasH)
	dc.ClearWithColor(gg.White)

	faceTitle := source.Face(28)
	faceLabel := source.Face(12)
	faceText := source.Face(24)

	// -- Title --
	dc.SetFont(faceTitle)
	dc.SetRGB(0.1, 0.1, 0.1)
	dc.DrawString("Text Transform Pipeline (v0.32.1)", gridLeft, 45)

	// -- Grid cells --
	cells := [rows][cols]cell{
		{
			{"Identity", func(_ *gg.Context) {}},
			{"Translate(100, 50)", func(dc *gg.Context) { dc.Translate(100, 50) }},
			{"Scale(2, 2)", func(dc *gg.Context) { dc.Scale(2, 2) }},
		},
		{
			{"Scale(0.7, 0.7)", func(dc *gg.Context) { dc.Scale(0.7, 0.7) }},
			{"Scale(3, 1) non-uniform", func(dc *gg.Context) { dc.Scale(3, 1) }},
			{fmt.Sprintf("Rotate(%s/6 = 30%s)", "\u03c0", "\u00b0"), func(dc *gg.Context) { dc.Rotate(math.Pi / 6) }},
		},
		{
			{fmt.Sprintf("Rotate(%s/4 = 45%s)", "\u03c0", "\u00b0"), func(dc *gg.Context) { dc.Rotate(math.Pi / 4) }},
			{"Shear (faux italic)", func(dc *gg.Context) { dc.Shear(-0.3, 0) }},
			{fmt.Sprintf("Scale(2)+Rotate(%s/8)", "\u03c0"), func(dc *gg.Context) {
				dc.Scale(2, 2)
				dc.Rotate(math.Pi / 8)
			}},
		},
	}

	for row := range rows {
		for col := range cols {
			c := cells[row][col]

			// Cell top-left corner in canvas space.
			cx := gridLeft + float64(col)*cellW + float64(col)*10
			cy := gridTop + float64(row)*cellH + float64(row)*5

			// Light gray cell background.
			dc.SetRGB(0.95, 0.95, 0.95)
			dc.DrawRectangle(cx, cy, cellW, cellH)
			dc.Fill()

			// Thin border.
			dc.SetRGB(0.8, 0.8, 0.8)
			dc.SetLineWidth(1)
			dc.DrawRectangle(cx, cy, cellW, cellH)
			dc.Stroke()

			// Label at top of cell.
			dc.SetFont(faceLabel)
			dc.SetRGB(0.45, 0.45, 0.45)
			dc.DrawString(c.label, cx+8, cy+16)

			// Clip to cell bounds so transformed text doesn't overflow.
			dc.Push()
			dc.ClipRect(cx, cy, cellW, cellH)

			// Draw text with transform.
			dc.Translate(cx+textOffX, cy+textOffY)
			c.transform(dc)

			// Small red crosshair at the drawing origin (before text).
			dc.SetRGB(0.9, 0.2, 0.2)
			dc.SetLineWidth(0.5)
			dc.DrawLine(-6, 0, 6, 0)
			dc.Stroke()
			dc.DrawLine(0, -6, 0, 6)
			dc.Stroke()

			// The transformed text.
			dc.SetFont(faceText)
			dc.SetRGB(0.12, 0.12, 0.12)
			dc.DrawString("Hello gg!", 0, 0)

			dc.Pop()
		}
	}

	if err := dc.SavePNG("text_transform.png"); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	log.Printf("Created text_transform.png (%dx%d) using font: %s",
		canvasW, canvasH, source.Name())
}

// findSystemFont returns path to a TTF font (TTC collections not supported).
func findSystemFont() string {
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\calibri.ttf",
		"C:\\Windows\\Fonts\\segoeui.ttf",
		// macOS
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
