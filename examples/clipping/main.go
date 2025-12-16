// Package main demonstrates the clipping API in gogpu/gg.
// This example shows how to use Clip(), ClipPreserve(), ClipRect(), and ResetClip()
// to create various clipping effects.
package main

import (
	"log"
	"math"

	"github.com/gogpu/gg"
)

func main() {
	// Create a 800x800 canvas
	const (
		width  = 800
		height = 800
	)
	dc := gg.NewContext(width, height)

	// White background
	dc.SetRGB(1, 1, 1)
	dc.Clear()

	// Example 1: Circular clip with pattern fill
	example1CircularClip(dc)

	// Example 2: Rectangular clip with ClipRect
	example2RectClip(dc)

	// Example 3: ClipPreserve - clip and then stroke the same path
	example3ClipPreserve(dc)

	// Example 4: Multiple intersecting clips with Push/Pop
	example4NestedClips(dc)

	// Example 5: Complex path clipping
	example5ComplexClip(dc)

	// Save the result
	err := dc.SavePNG("examples/clipping/output.png")
	if err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}
	log.Println("Saved: examples/clipping/output.png")
}

// example1CircularClip demonstrates basic circular clipping.
func example1CircularClip(dc *gg.Context) {
	dc.Push()

	// Create a circular clip region
	dc.DrawCircle(150, 150, 80)
	dc.Clip() // Path is cleared after Clip()

	// Draw a gradient pattern inside the clip
	for i := 0; i < 200; i++ {
		hue := float64(i) / 200.0
		dc.SetRGB(hue, 0.7, 0.9)
		dc.DrawRectangle(50+float64(i), 50, 2, 200)
		dc.Fill()
	}

	dc.Pop()

	// Label
	dc.SetRGB(0, 0, 0)
	dc.DrawString("1. Circular Clip", 100, 260)
}

// example2RectClip demonstrates ClipRect for fast rectangular clipping.
func example2RectClip(dc *gg.Context) {
	dc.Push()

	// Use ClipRect for efficient rectangular clipping
	dc.ClipRect(300, 50, 160, 160)

	// Draw a colorful pattern
	for y := 0; y < 200; y += 10 {
		for x := 0; x < 200; x += 10 {
			dc.SetRGB(
				float64(x)/200.0,
				float64(y)/200.0,
				0.5,
			)
			dc.DrawRectangle(float64(250+x), float64(y), 10, 10)
			dc.Fill()
		}
	}

	dc.Pop()

	// Draw border around clip region
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(2)
	dc.DrawRectangle(300, 50, 160, 160)
	dc.Stroke()

	// Label
	dc.DrawString("2. ClipRect", 340, 260)
}

// example3ClipPreserve demonstrates ClipPreserve which keeps the path after clipping.
func example3ClipPreserve(dc *gg.Context) {
	dc.Push()

	// Create a star path
	drawStar(dc, 150, 450, 70, 5)

	// Use ClipPreserve to both clip and keep the path
	dc.ClipPreserve()

	// Fill with gradient inside the clip
	for i := 0; i < 200; i++ {
		dc.SetRGBA(
			0.8,
			0.3,
			float64(i)/200.0,
			1,
		)
		dc.DrawRectangle(50, 350+float64(i), 200, 2)
		dc.Fill()
	}

	// Now stroke the same path (it was preserved)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(3)
	dc.Stroke()

	dc.Pop()

	// Label
	dc.SetRGB(0, 0, 0)
	dc.DrawString("3. ClipPreserve", 100, 560)
}

// example4NestedClips demonstrates nested clipping with Push/Pop.
func example4NestedClips(dc *gg.Context) {
	dc.Push()

	// First clip: Large rectangle
	dc.ClipRect(300, 350, 160, 160)

	// Draw background pattern
	for y := 0; y < 200; y += 5 {
		dc.SetRGBA(0.2, 0.6, 0.9, 0.8)
		dc.DrawRectangle(250, 300+float64(y), 250, 3)
		dc.Fill()
	}

	// Push state and add second clip
	dc.Push()

	// Second clip: Circle (intersects with rectangle)
	dc.DrawCircle(380, 430, 60)
	dc.Clip()

	// Draw different pattern in intersection
	for i := 0; i < 50; i++ {
		dc.SetRGBA(0.9, 0.3, 0.2, 0.7)
		dc.DrawCircle(380, 430, float64(60-i))
		dc.Stroke()
	}

	// Pop to restore first clip only
	dc.Pop()

	// Draw rectangle border
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(2)
	dc.DrawRectangle(300, 350, 160, 160)
	dc.Stroke()

	dc.Pop()

	// Label
	dc.DrawString("4. Nested Clips", 330, 560)
}

// example5ComplexClip demonstrates clipping with a complex curved path.
func example5ComplexClip(dc *gg.Context) {
	dc.Push()

	// Create a complex curved shape
	dc.MoveTo(600, 120)
	dc.CubicTo(650, 50, 700, 50, 750, 120)
	dc.CubicTo(700, 180, 650, 180, 600, 120)
	dc.ClosePath()

	// Clip to this shape
	dc.Clip()

	// Fill with diagonal lines
	dc.SetLineWidth(2)
	for i := -100; i < 300; i += 8 {
		dc.SetRGB(0.2, 0.7, 0.4)
		dc.DrawLine(float64(550+i), 50, float64(650+i), 200)
		dc.Stroke()
	}

	dc.Pop()

	// Draw the shape outline
	dc.MoveTo(600, 120)
	dc.CubicTo(650, 50, 700, 50, 750, 120)
	dc.CubicTo(700, 180, 650, 180, 600, 120)
	dc.ClosePath()
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(3)
	dc.Stroke()

	// Label
	dc.DrawString("5. Complex Path", 640, 260)
}

// example6 demonstrates ResetClip
func example6ResetClip(dc *gg.Context) {
	dc.Push()

	// Set a clip
	dc.ClipRect(550, 350, 160, 160)

	// Draw something
	dc.SetRGB(0.9, 0.7, 0.3)
	dc.DrawRectangle(500, 300, 250, 250)
	dc.Fill()

	// Reset clip to draw outside
	dc.ResetClip()

	// Now we can draw anywhere
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(2)
	dc.DrawRectangle(550, 350, 160, 160)
	dc.Stroke()

	dc.Pop()

	// Label
	dc.DrawString("6. ResetClip", 590, 560)
}

// drawStar draws a star shape centered at (cx, cy) with given radius and points.
func drawStar(dc *gg.Context, cx, cy, r float64, points int) {
	angle := 2 * math.Pi / float64(points)
	halfAngle := angle / 2

	for i := 0; i < points*2; i++ {
		a := float64(i) * halfAngle
		radius := r
		if i%2 == 1 {
			radius = r * 0.4 // Inner radius
		}
		x := cx + radius*math.Cos(a-math.Pi/2)
		y := cy + radius*math.Sin(a-math.Pi/2)

		if i == 0 {
			dc.MoveTo(x, y)
		} else {
			dc.LineTo(x, y)
		}
	}
	dc.ClosePath()
}
