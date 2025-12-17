// Package main demonstrates the scene graph (retained mode) API for gogpu/gg.
//
// This example shows how to:
//   - Build a scene using SceneBuilder fluent API
//   - Use layers with blend modes and alpha
//   - Apply transforms
//   - Render the scene to a Pixmap
//   - Save the result to PNG
//
// The scene graph approach is more efficient for static or slowly-changing
// content as it can cache rendered layers and only re-render dirty regions.
package main

import (
	"fmt"
	"log"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

func main() {
	// Create a 512x512 output
	const width, height = 512, 512

	// Create a renderer for our canvas size
	renderer := scene.NewRenderer(width, height,
		scene.WithWorkers(0),    // Use default (GOMAXPROCS)
		scene.WithCacheSize(32), // 32MB cache
	)
	if renderer == nil {
		log.Fatal("Failed to create renderer")
	}
	defer renderer.Close()

	// Create target pixmap
	target := gg.NewPixmap(width, height)
	target.Clear(gg.RGBA{R: 1, G: 1, B: 1, A: 1}) // White background

	// Build the scene using SceneBuilder fluent API
	sceneGraph := buildScene()

	// Render the scene
	if err := renderer.Render(target, sceneGraph); err != nil {
		log.Fatalf("Render failed: %v", err)
	}

	// Print render statistics
	stats := renderer.Stats()
	fmt.Printf("Render Statistics:\n")
	fmt.Printf("  Tiles Total:    %d\n", stats.TilesTotal)
	fmt.Printf("  Tiles Rendered: %d\n", stats.TilesRendered)
	fmt.Printf("  Time Total:     %v\n", stats.TimeTotal)
	fmt.Printf("  Time Raster:    %v\n", stats.TimeRaster)
	fmt.Printf("  Time Composite: %v\n", stats.TimeComposite)

	// Save to PNG
	if err := target.SavePNG("scene_output.png"); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	fmt.Println("\nSuccessfully created scene_output.png")

	// Demonstrate incremental rendering
	demonstrateIncrementalRender(renderer, target, sceneGraph)
}

// buildScene creates a demonstration scene with various shapes and layers.
func buildScene() *scene.Scene {
	builder := scene.NewSceneBuilder()

	// Background rectangle
	builder.FillRect(0, 0, 512, 512,
		scene.SolidBrush(gg.RGBA{R: 0.95, G: 0.95, B: 1.0, A: 1}))

	// Draw a grid of colored rectangles
	colors := []gg.RGBA{
		{R: 0.9, G: 0.3, B: 0.3, A: 1}, // Red
		{R: 0.3, G: 0.9, B: 0.3, A: 1}, // Green
		{R: 0.3, G: 0.3, B: 0.9, A: 1}, // Blue
		{R: 0.9, G: 0.9, B: 0.3, A: 1}, // Yellow
	}

	for i := 0; i < 4; i++ {
		x := float32(50 + i*110)
		builder.FillRect(x, 50, 80, 80, scene.SolidBrush(colors[i]))
	}

	// Draw circles with transforms
	builder.WithTransform(scene.TranslateAffine(256, 256), func(b *scene.SceneBuilder) {
		// Centered circles
		b.FillCircle(0, 0, 100, scene.SolidBrush(gg.RGBA{R: 0.8, G: 0.4, B: 0.8, A: 0.7}))
		b.StrokeCircle(0, 0, 100, scene.SolidBrush(gg.RGBA{R: 0.4, G: 0.2, B: 0.4, A: 1}), 3)
	})

	// Layer with multiply blend mode
	builder.Layer(scene.BlendMultiply, 0.8, nil, func(lb *scene.SceneBuilder) {
		lb.FillRect(200, 180, 150, 150,
			scene.SolidBrush(gg.RGBA{R: 1, G: 0.8, B: 0.2, A: 1}))
	})

	// Layer with screen blend mode
	builder.Layer(scene.BlendScreen, 0.6, nil, func(lb *scene.SceneBuilder) {
		lb.FillCircle(280, 280, 80,
			scene.SolidBrush(gg.RGBA{R: 0.2, G: 0.6, B: 1, A: 1}))
	})

	// Draw some stroked shapes
	builder.StrokeRect(30, 400, 100, 80,
		scene.SolidBrush(gg.RGBA{R: 0.2, G: 0.2, B: 0.2, A: 1}), 2)

	builder.DrawLine(150, 440, 350, 440,
		scene.SolidBrush(gg.RGBA{R: 0.5, G: 0.5, B: 0.5, A: 1}), 4)

	// Rounded rectangle
	builder.Fill(
		scene.NewRoundedRectShape(380, 400, 100, 80, 15),
		scene.SolidBrush(gg.RGBA{R: 0.3, G: 0.7, B: 0.5, A: 1}))

	// Build and return the scene
	return builder.Build()
}

// demonstrateIncrementalRender shows how dirty region tracking works.
func demonstrateIncrementalRender(renderer *scene.Renderer, target *gg.Pixmap, sceneGraph *scene.Scene) {
	fmt.Println("\nDemonstrating incremental rendering...")

	// Full render first
	renderer.MarkAllDirty()
	_ = renderer.Render(target, sceneGraph)
	fullStats := renderer.Stats()

	// Now mark only a small region dirty and render again
	renderer.MarkDirty(100, 100, 64, 64) // Mark one tile

	_ = renderer.RenderDirty(target, sceneGraph, nil)
	dirtyStats := renderer.Stats()

	fmt.Printf("\nFull Render:  %d tiles in %v\n",
		fullStats.TilesRendered, fullStats.TimeTotal)
	fmt.Printf("Dirty Render: %d tiles in %v\n",
		dirtyStats.TilesRendered, dirtyStats.TimeTotal)

	if dirtyStats.TilesRendered < fullStats.TilesRendered {
		fmt.Printf("Incremental rendering processed %.1f%% fewer tiles\n",
			100*(1-float64(dirtyStats.TilesRendered)/float64(fullStats.TilesRendered)))
	}
}
