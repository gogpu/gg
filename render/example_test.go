// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package render_test

import (
	"fmt"
	"image/color"

	"github.com/gogpu/gg/render"
)

// ExampleNewGPURenderer demonstrates creating a GPU renderer with a DeviceHandle.
//
// In real usage, the DeviceHandle would come from the host application
// (e.g., gogpu.App.GPUContextProvider()). For testing without a GPU,
// use NullDeviceHandle.
func ExampleNewGPURenderer() {
	// Create renderer with null device (for testing without GPU)
	renderer, err := render.NewGPURenderer(render.NullDeviceHandle{})
	if err != nil {
		fmt.Println("failed to create renderer:", err)
		return
	}

	// Create a CPU target for software fallback rendering
	target := render.NewPixmapTarget(100, 100)

	// Create a scene with a red rectangle
	scene := render.NewScene()
	scene.Clear(color.White)
	scene.SetFillColor(color.RGBA{R: 255, G: 0, B: 0, A: 255})
	scene.Rectangle(10, 10, 80, 80)
	scene.Fill()

	// Render the scene (falls back to software in Phase 1)
	if err := renderer.Render(target, scene); err != nil {
		fmt.Println("render failed:", err)
		return
	}

	fmt.Println("rendered successfully")
	// Output: rendered successfully
}

// ExampleNewSoftwareRenderer demonstrates CPU-based software rendering.
func ExampleNewSoftwareRenderer() {
	// Create software renderer (no GPU required)
	renderer := render.NewSoftwareRenderer()

	// Create a CPU-backed render target
	target := render.NewPixmapTarget(200, 200)

	// Build a scene
	scene := render.NewScene()
	scene.Clear(color.White)
	scene.SetFillColor(color.RGBA{R: 0, G: 0, B: 255, A: 255})
	scene.Circle(100, 100, 50)
	scene.Fill()

	// Render the scene
	if err := renderer.Render(target, scene); err != nil {
		fmt.Println("render failed:", err)
		return
	}

	// Access the rendered image
	img := target.Image()
	fmt.Printf("rendered %dx%d image\n", img.Bounds().Dx(), img.Bounds().Dy())
	// Output: rendered 200x200 image
}

// ExampleScene demonstrates building a scene with various drawing commands.
func ExampleScene() {
	scene := render.NewScene()

	// Clear background
	scene.Clear(color.White)

	// Draw a red triangle
	scene.SetFillColor(color.RGBA{R: 255, G: 0, B: 0, A: 255})
	scene.MoveTo(100, 50)
	scene.LineTo(150, 150)
	scene.LineTo(50, 150)
	scene.ClosePath()
	scene.Fill()

	// Draw a blue circle
	scene.SetFillColor(color.RGBA{R: 0, G: 0, B: 255, A: 255})
	scene.Circle(100, 100, 30)
	scene.Fill()

	// Draw a green rectangle outline
	scene.SetStrokeColor(color.RGBA{R: 0, G: 255, B: 0, A: 255})
	scene.SetStrokeWidth(2.0)
	scene.Rectangle(20, 20, 60, 40)
	scene.Stroke()

	fmt.Printf("scene has %d commands\n", scene.CommandCount())
	// Output: scene has 4 commands
}

// ExampleNewPixmapTarget demonstrates creating and using a CPU render target.
func ExampleNewPixmapTarget() {
	// Create a 400x300 pixel render target
	target := render.NewPixmapTarget(400, 300)

	fmt.Printf("target size: %dx%d\n", target.Width(), target.Height())
	fmt.Printf("stride: %d bytes per row\n", target.Stride())
	fmt.Printf("pixels: %d bytes total\n", len(target.Pixels()))
	// Output:
	// target size: 400x300
	// stride: 1600 bytes per row
	// pixels: 480000 bytes total
}

// ExamplePixmapTarget_Clear demonstrates clearing a target with a color.
func ExamplePixmapTarget_Clear() {
	target := render.NewPixmapTarget(100, 100)

	// Clear to red
	target.Clear(color.RGBA{R: 255, G: 0, B: 0, A: 255})

	// Check a pixel
	pixel := target.GetPixel(50, 50).(color.RGBA)
	fmt.Printf("pixel at (50,50): R=%d, G=%d, B=%d, A=%d\n",
		pixel.R, pixel.G, pixel.B, pixel.A)
	// Output: pixel at (50,50): R=255, G=0, B=0, A=255
}

// ExampleNullDeviceHandle demonstrates the null device for testing.
func ExampleNullDeviceHandle() {
	handle := render.NullDeviceHandle{}

	// NullDeviceHandle returns nil for all GPU resources
	fmt.Printf("device: %v\n", handle.Device())
	fmt.Printf("queue: %v\n", handle.Queue())
	fmt.Printf("adapter: %v\n", handle.Adapter())
	// Output:
	// device: <nil>
	// queue: <nil>
	// adapter: <nil>
}

// ExampleGPURenderer_Capabilities demonstrates querying renderer capabilities.
func ExampleGPURenderer_Capabilities() {
	renderer, err := render.NewGPURenderer(render.NullDeviceHandle{})
	if err != nil {
		fmt.Println("failed:", err)
		return
	}

	caps := renderer.Capabilities()
	fmt.Printf("GPU renderer: %v\n", caps.IsGPU)
	fmt.Printf("supports antialiasing: %v\n", caps.SupportsAntialiasing)
	// Output:
	// GPU renderer: true
	// supports antialiasing: true
}

// ExampleSoftwareRenderer_Capabilities demonstrates querying software renderer capabilities.
func ExampleSoftwareRenderer_Capabilities() {
	renderer := render.NewSoftwareRenderer()

	caps := renderer.Capabilities()
	fmt.Printf("GPU renderer: %v\n", caps.IsGPU)
	fmt.Printf("supports antialiasing: %v\n", caps.SupportsAntialiasing)
	// Output:
	// GPU renderer: false
	// supports antialiasing: true
}

// ExampleScene_damage demonstrates damage tracking for efficient redraws.
func ExampleScene_damage() {
	scene := render.NewScene()

	// Initially no dirty regions
	fmt.Printf("has dirty: %v\n", scene.HasDirtyRegions())

	// Invalidate a region
	scene.Invalidate(render.DirtyRect{X: 10, Y: 10, Width: 100, Height: 50})
	fmt.Printf("has dirty: %v\n", scene.HasDirtyRegions())
	fmt.Printf("dirty rects: %d\n", len(scene.DirtyRects()))

	// Clear dirty state after rendering
	scene.ClearDirty()
	fmt.Printf("has dirty after clear: %v\n", scene.HasDirtyRegions())
	// Output:
	// has dirty: false
	// has dirty: true
	// dirty rects: 1
	// has dirty after clear: false
}

// ExamplePixmapTarget_Image demonstrates accessing the underlying image.
func ExamplePixmapTarget_Image() {
	target := render.NewPixmapTarget(100, 100)

	// Render something
	renderer := render.NewSoftwareRenderer()
	scene := render.NewScene()
	scene.Clear(color.RGBA{R: 128, G: 128, B: 128, A: 255})

	_ = renderer.Render(target, scene)

	// Get the image for further processing (e.g., saving to file)
	img := target.Image()
	bounds := img.Bounds()
	fmt.Printf("image bounds: %v\n", bounds)
	// Output: image bounds: (0,0)-(100,100)
}
