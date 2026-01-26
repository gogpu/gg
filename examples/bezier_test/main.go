// Package main demonstrates smooth bezier curve rendering.
// This example tests Issue #48 fix - analytic anti-aliasing for curves.
package main

import (
	"log"
	"math"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/backend/native"
)

func main() {
	const W, H = 800, 600

	// Create software renderer with analytic AA
	renderer := gg.NewSoftwareRenderer(W, H)
	filler := native.NewAnalyticFillerAdapter(W, H)
	renderer.SetAnalyticFiller(filler)
	log.Println("Analytic AA enabled!")

	// Create context with the analytic AA renderer
	dc := gg.NewContext(W, H, gg.WithRenderer(renderer))

	dc.ClearWithColor(gg.White)

	// ========================================
	// Section 1: Quadratic Bezier Curves
	// ========================================
	dc.SetRGB(0.8, 0.2, 0.2) // Red
	dc.SetLineWidth(3)

	// Simple quadratic curve
	dc.MoveTo(50, 100)
	dc.QuadraticTo(200, 20, 350, 100)
	dc.Stroke()

	// S-shaped quadratic curves
	dc.MoveTo(50, 150)
	dc.QuadraticTo(150, 80, 200, 150)
	dc.QuadraticTo(250, 220, 350, 150)
	dc.Stroke()

	// ========================================
	// Section 2: Cubic Bezier Curves
	// ========================================
	dc.SetRGB(0.2, 0.2, 0.8) // Blue
	dc.SetLineWidth(3)

	// Simple cubic curve
	dc.MoveTo(400, 100)
	dc.CubicTo(450, 20, 700, 20, 750, 100)
	dc.Stroke()

	// Complex cubic S-curve
	dc.MoveTo(400, 180)
	dc.CubicTo(500, 100, 600, 260, 750, 180)
	dc.Stroke()

	// ========================================
	// Section 3: Filled Curves (main test for Issue #48)
	// ========================================
	dc.SetRGB(0.2, 0.6, 0.2) // Green

	// Filled quadratic shape
	dc.MoveTo(100, 300)
	dc.QuadraticTo(200, 220, 300, 300)
	dc.QuadraticTo(200, 380, 100, 300)
	dc.Fill()

	// Filled cubic shape (leaf-like)
	dc.SetRGB(0.6, 0.2, 0.6) // Purple
	dc.MoveTo(450, 300)
	dc.CubicTo(550, 220, 650, 220, 700, 300)
	dc.CubicTo(650, 380, 550, 380, 450, 300)
	dc.Fill()

	// ========================================
	// Section 4: Circles at various sizes
	// ========================================
	// Small circles are good test for AA quality
	dc.SetRGB(0.1, 0.1, 0.1)

	radii := []float64{5, 10, 20, 40, 60}
	x := 80.0
	for _, r := range radii {
		dc.DrawCircle(x, 480, r)
		dc.Fill()
		x += r*2 + 30
	}

	// ========================================
	// Section 5: Spiral (stress test)
	// ========================================
	dc.SetRGB(0.9, 0.5, 0.1) // Orange
	dc.SetLineWidth(2)

	cx, cy := 600.0, 480.0
	dc.MoveTo(cx, cy)
	for i := 0; i < 720; i++ {
		angle := float64(i) * math.Pi / 180
		r := float64(i) / 10
		x := cx + r*math.Cos(angle)
		y := cy + r*math.Sin(angle)
		dc.LineTo(x, y)
	}
	dc.Stroke()

	// ========================================
	// Section 6: Thin strokes (AA stress test)
	// ========================================
	dc.SetRGB(0.3, 0.3, 0.3)

	widths := []float64{0.5, 1.0, 1.5, 2.0, 3.0}
	y := 550.0
	for i, w := range widths {
		dc.SetLineWidth(w)
		dc.MoveTo(50, y)
		dc.QuadraticTo(150, y-20, 250, y)
		dc.Stroke()
		y -= float64(i+1) * 2
	}

	// Save result
	if err := dc.SavePNG("bezier_test.png"); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	log.Println("Created bezier_test.png - check for smooth curves!")
}
