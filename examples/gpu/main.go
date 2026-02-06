// Package main demonstrates the GPU backend for gogpu/gg.
//
// This example shows how to:
//   - Create and initialize the native GPU backend directly
//   - Render a scene using GPU acceleration
//   - Handle fallback to software rendering
//
// The GPU backend uses WebGPU (via gogpu/wgpu) for hardware-accelerated
// 2D graphics rendering. When GPU is not available, it falls back to
// software rendering via scene.Renderer.
//
// Note: The GPU rendering pipeline is implemented but GPU operations
// currently run as stubs. When gogpu/wgpu implements texture readback
// and buffer mapping, actual GPU rendering will be enabled.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/native"
	"github.com/gogpu/gg/scene"
)

func main() {
	fmt.Println("GPU Backend Example")
	fmt.Println("===================")

	// Try GPU backend first, fall back to software
	nb := native.NewNativeBackend()
	if err := nb.Init(); err != nil {
		fmt.Printf("GPU backend unavailable: %v\n", err)
		fmt.Println("Falling back to software rendering...")
		runSoftware()
		return
	}
	defer nb.Close()

	fmt.Printf("Using backend: %s\n\n", nb.Name())

	// Create scene
	s := buildScene()

	// Create target pixmap
	const width, height = 800, 600
	pm := gg.NewPixmap(width, height)
	pm.Clear(gg.White)

	// Render scene
	fmt.Println("Rendering scene...")
	if err := nb.RenderScene(pm, s); err != nil {
		// Note: Some errors are expected until full GPU support is implemented
		fmt.Printf("RenderScene note: %v\n", err)
	}

	// Save result
	outputPath := "gpu_output.png"
	if err := pm.SavePNG(outputPath); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	fmt.Printf("\nSaved output to: %s\n", outputPath)
	fmt.Println("\nBackend Information:")
	fmt.Printf("  Name: %s\n", nb.Name())
	fmt.Println("  Type: GPU (WebGPU via gogpu/wgpu)")
	fmt.Println("  Status: Pipeline implemented, GPU ops as stubs")
}

// runSoftware demonstrates software rendering fallback.
func runSoftware() {
	s := buildScene()

	const width, height = 800, 600
	pm := gg.NewPixmap(width, height)
	pm.Clear(gg.White)

	sr := scene.NewRenderer(width, height)
	defer sr.Close()

	if err := sr.Render(pm, s); err != nil {
		log.Fatalf("Software render failed: %v", err)
	}

	outputPath := "gpu_output.png"
	if err := pm.SavePNG(outputPath); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	fmt.Printf("\nSaved output to: %s\n", outputPath)
	fmt.Println("\nBackend Information:")
	fmt.Println("  Name: software")
	fmt.Println("  Type: CPU (Software rendering)")
	fmt.Println("  Status: Fully functional")
}

// buildScene creates a demonstration scene with various shapes and layers.
func buildScene() *scene.Scene {
	builder := scene.NewSceneBuilder()

	// Background
	builder.FillRect(0, 0, 800, 600,
		scene.SolidBrush(gg.RGBA{R: 0.95, G: 0.95, B: 0.98, A: 1}))

	// Draw colored rectangles
	colors := []gg.RGBA{
		{R: 0.9, G: 0.3, B: 0.3, A: 1}, // Red
		{R: 0.3, G: 0.9, B: 0.3, A: 1}, // Green
		{R: 0.3, G: 0.3, B: 0.9, A: 1}, // Blue
		{R: 0.9, G: 0.9, B: 0.3, A: 1}, // Yellow
		{R: 0.9, G: 0.3, B: 0.9, A: 1}, // Magenta
	}

	for i, c := range colors {
		x := float32(50 + i*150)
		builder.FillRect(x, 50, 100, 100, scene.SolidBrush(c))
	}

	// Draw circles with semi-transparency
	for i := 0; i < 6; i++ {
		x := float32(100 + i*120)
		builder.FillCircle(x, 250, 50,
			scene.SolidBrush(gg.RGBA{R: 0.2, G: 0.6, B: 0.9, A: 0.6}))
	}

	// Layer with multiply blend mode
	builder.Layer(scene.BlendMultiply, 0.8, nil, func(lb *scene.SceneBuilder) {
		lb.FillRect(200, 350, 200, 150,
			scene.SolidBrush(gg.RGBA{R: 1, G: 0.8, B: 0.2, A: 1}))
	})

	// Layer with screen blend mode
	builder.Layer(scene.BlendScreen, 0.7, nil, func(lb *scene.SceneBuilder) {
		lb.FillCircle(500, 420, 100,
			scene.SolidBrush(gg.RGBA{R: 0.2, G: 0.8, B: 0.6, A: 1}))
	})

	// Stroked shapes
	builder.StrokeRect(50, 350, 100, 150,
		scene.SolidBrush(gg.RGBA{R: 0.2, G: 0.2, B: 0.2, A: 1}), 3)

	builder.StrokeCircle(700, 450, 75,
		scene.SolidBrush(gg.RGBA{R: 0.4, G: 0.2, B: 0.6, A: 1}), 4)

	// Complex path (star shape)
	starPath := createStarPath(700, 120, 50, 25, 5)
	builder.FillPath(starPath, scene.SolidBrush(gg.RGBA{R: 1, G: 0.7, B: 0.1, A: 1}))

	return builder.Build()
}

// createStarPath creates a star-shaped path.
func createStarPath(cx, cy, outerR, innerR float32, points int) *scene.Path {
	path := scene.NewPath()

	for i := 0; i < points*2; i++ {
		angle := float32(i) * 3.14159 / float32(points)
		angle -= 3.14159 / 2 // Start from top

		var r float32
		if i%2 == 0 {
			r = outerR
		} else {
			r = innerR
		}

		x := cx + r*cos32(angle)
		y := cy + r*sin32(angle)

		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}

	path.Close()
	return path
}

// cos32 returns the cosine of x (float32 version).
func cos32(x float32) float32 {
	// Simple Taylor series approximation for small angles
	// For production, use math.Cos with conversion
	x = mod32(x, 2*3.14159)
	if x > 3.14159 {
		x -= 2 * 3.14159
	}
	x2 := x * x
	return 1 - x2/2 + x2*x2/24 - x2*x2*x2/720
}

// sin32 returns the sine of x (float32 version).
func sin32(x float32) float32 {
	// Simple Taylor series approximation
	x = mod32(x, 2*3.14159)
	if x > 3.14159 {
		x -= 2 * 3.14159
	}
	x2 := x * x
	return x - x*x2/6 + x*x2*x2/120 - x*x2*x2*x2/5040
}

// mod32 returns x mod y (float32 version).
func mod32(x, y float32) float32 {
	for x >= y {
		x -= y
	}
	for x < 0 {
		x += y
	}
	return x
}

func init() {
	// Suppress log output to stderr for cleaner demo output
	// In production, you would want to configure proper logging
	log.SetOutput(os.Stdout)
}
