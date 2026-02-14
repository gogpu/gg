//go:build !nogpu

// Package main demonstrates GPU-accelerated 2D rendering with gg.
//
// This example shows how to:
//   - Enable GPU acceleration via blank import of gg/gpu
//   - Draw shapes that automatically use the three-tier GPU pipeline:
//     Tier 1: SDF fragment shader (circles, rounded rects)
//     Tier 2a: Convex polygon fast-path (triangles, pentagons, etc.)
//     Tier 2b: Stencil-then-cover (arbitrary paths, stars, etc.)
//   - Fall back to CPU rendering transparently when GPU is unavailable
//
// The GPU backend uses WebGPU (via gogpu/wgpu) with Vulkan for
// hardware-accelerated rendering in a single unified render pass.
package main

import (
	"fmt"
	"log"
	"math"
	"os"

	"github.com/gogpu/gg"

	// Blank import enables GPU acceleration.
	// Without this import, all rendering uses CPU (software) rasterization.
	_ "github.com/gogpu/gg/gpu"
)

func main() {
	fmt.Println("GPU-Accelerated 2D Rendering Example")
	fmt.Println("=====================================")

	const (
		width  = 800
		height = 600
	)

	dc := gg.NewContext(width, height)

	// Light background.
	dc.SetRGBA(0.95, 0.95, 0.98, 1)
	dc.Clear()

	// --- Tier 1: SDF shapes (circles, rounded rects) ---
	// These use the SDF fragment shader for pixel-perfect edges.

	// Filled circles.
	colors := [][3]float64{
		{0.9, 0.3, 0.3}, // Red
		{0.3, 0.9, 0.3}, // Green
		{0.3, 0.3, 0.9}, // Blue
		{0.9, 0.9, 0.3}, // Yellow
		{0.9, 0.3, 0.9}, // Magenta
	}
	for i, c := range colors {
		x := 100 + float64(i)*150
		dc.SetRGB(c[0], c[1], c[2])
		dc.DrawCircle(x, 100, 50)
		if err := dc.Fill(); err != nil {
			log.Printf("Fill circle: %v", err)
		}
	}

	// Filled rounded rectangle.
	dc.SetRGBA(0.2, 0.4, 0.9, 0.8)
	dc.DrawRoundedRectangle(50, 200, 200, 120, 20)
	if err := dc.Fill(); err != nil {
		log.Printf("Fill rrect: %v", err)
	}

	// Stroked circle.
	dc.SetRGBA(0.0, 0.9, 0.9, 1.0)
	dc.SetLineWidth(3.0)
	dc.DrawCircle(400, 260, 60)
	if err := dc.Stroke(); err != nil {
		log.Printf("Stroke circle: %v", err)
	}

	// --- Tier 2a: Convex polygon fast-path ---
	// Convex polygons with only line segments use a single draw call (no stencil).

	// Triangle (convex).
	dc.SetRGBA(1.0, 0.6, 0.1, 0.9)
	dc.MoveTo(600, 180)
	dc.LineTo(700, 320)
	dc.LineTo(500, 320)
	dc.ClosePath()
	if err := dc.Fill(); err != nil {
		log.Printf("Fill triangle: %v", err)
	}

	// Pentagon (convex).
	dc.SetRGBA(0.2, 0.8, 0.4, 0.85)
	drawRegularPolygon(dc, 150, 430, 60, 5)
	if err := dc.Fill(); err != nil {
		log.Printf("Fill pentagon: %v", err)
	}

	// Hexagon (convex).
	dc.SetRGBA(0.8, 0.2, 0.6, 0.85)
	drawRegularPolygon(dc, 350, 430, 55, 6)
	if err := dc.Fill(); err != nil {
		log.Printf("Fill hexagon: %v", err)
	}

	// --- Tier 2b: Stencil-then-cover (arbitrary paths) ---
	// Non-convex paths and paths with curves use stencil buffer.

	// Star (non-convex).
	dc.SetRGBA(1, 0.7, 0.1, 1.0)
	drawStar(dc, 550, 430, 65, 30, 5)
	if err := dc.Fill(); err != nil {
		log.Printf("Fill star: %v", err)
	}

	// Curved path (uses cubic beziers → stencil-then-cover).
	dc.SetRGBA(0.4, 0.2, 0.8, 0.7)
	dc.MoveTo(650, 380)
	dc.CubicTo(750, 350, 750, 500, 650, 480)
	dc.CubicTo(550, 460, 550, 360, 650, 380)
	dc.ClosePath()
	if err := dc.Fill(); err != nil {
		log.Printf("Fill curved path: %v", err)
	}

	// Flush any pending GPU commands.
	if err := dc.FlushGPU(); err != nil {
		log.Printf("FlushGPU: %v", err)
	}

	// Save result.
	outputPath := "gpu_output.png"
	if err := dc.SavePNG(outputPath); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	fmt.Printf("\nSaved output to: %s\n", outputPath)

	// Report accelerator status.
	if a := gg.Accelerator(); a != nil {
		fmt.Printf("GPU Accelerator: %s\n", a.Name())
	} else {
		fmt.Println("GPU Accelerator: none (CPU rendering)")
	}

	fmt.Println("\nRendering tiers used:")
	fmt.Println("  Tier 1 (SDF):             circles, rounded rect")
	fmt.Println("  Tier 2a (Convex):          triangle, pentagon, hexagon")
	fmt.Println("  Tier 2b (Stencil+Cover):   star, curved path")
}

// drawRegularPolygon draws a regular polygon with n sides centered at (cx, cy).
func drawRegularPolygon(dc *gg.Context, cx, cy, radius float64, sides int) {
	for i := 0; i < sides; i++ {
		angle := float64(i)*2*math.Pi/float64(sides) - math.Pi/2
		x := cx + radius*math.Cos(angle)
		y := cy + radius*math.Sin(angle)
		if i == 0 {
			dc.MoveTo(x, y)
		} else {
			dc.LineTo(x, y)
		}
	}
	dc.ClosePath()
}

// drawStar draws a star with the given number of points.
func drawStar(dc *gg.Context, cx, cy, outerR, innerR float64, points int) {
	for i := 0; i < points*2; i++ {
		angle := float64(i)*math.Pi/float64(points) - math.Pi/2
		r := outerR
		if i%2 == 1 {
			r = innerR
		}
		x := cx + r*math.Cos(angle)
		y := cy + r*math.Sin(angle)
		if i == 0 {
			dc.MoveTo(x, y)
		} else {
			dc.LineTo(x, y)
		}
	}
	dc.ClosePath()
}

func init() {
	log.SetOutput(os.Stdout)
}
