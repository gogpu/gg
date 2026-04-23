// Example: Scene rendering with GPU acceleration
//
// Demonstrates the GPU auto-select path in scene.Renderer.
// When `import _ "gg/gpu"` registers the GPU accelerator,
// scene.Renderer automatically routes through GPUSceneRenderer
// instead of the CPU tile-parallel path.
//
// The same scene code works identically with or without GPU —
// remove the gpu import to fall back to CPU rendering.
//
// Output: scene_gpu_output.png (512x512)
package main

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // GPU acceleration for scene rendering
	"github.com/gogpu/gg/scene"
)

func main() {
	const width, height = 512, 512

	renderer := scene.NewRenderer(width, height)
	if renderer == nil {
		log.Fatal("Failed to create renderer")
	}
	defer renderer.Close()

	target := gg.NewPixmap(width, height)
	target.Clear(gg.RGBA{R: 0.08, G: 0.08, B: 0.12, A: 1})

	s := buildScene(width, height)

	start := time.Now()
	if err := renderer.Render(target, s); err != nil {
		log.Fatalf("Render: %v", err)
	}
	elapsed := time.Since(start)

	stats := renderer.Stats()
	if stats.TilesRendered == 0 {
		fmt.Println("Rendered via: GPU (0 CPU tiles used)")
	} else {
		fmt.Printf("Rendered via: CPU (%d tiles)\n", stats.TilesRendered)
	}
	fmt.Printf("Time: %v\n", elapsed)

	if err := target.SavePNG("scene_gpu_output.png"); err != nil {
		log.Fatalf("SavePNG: %v", err)
	}
	fmt.Println("Saved: scene_gpu_output.png")
}

func buildScene(w, h int) *scene.Scene {
	b := scene.NewSceneBuilder()
	cx, cy := float32(w)/2, float32(h)/2

	// Background gradient (two overlapping rects)
	b.FillRect(0, 0, float32(w), float32(h),
		scene.SolidBrush(gg.RGBA{R: 0.1, G: 0.1, B: 0.18, A: 1}))

	// Orbiting circles (like gogpu_integration demo)
	n := 12
	radius := float32(140)
	for i := 0; i < n; i++ {
		angle := float64(i) * 2 * math.Pi / float64(n)
		x := cx + radius*float32(math.Cos(angle))
		y := cy + radius*float32(math.Sin(angle))
		r := 12 + float32(i)*3

		hue := float64(i) / float64(n)
		color := hueToRGB(hue)
		b.FillCircle(x, y, r, scene.SolidBrush(color))
	}

	// Center circle with stroke
	b.FillCircle(cx, cy, 60, scene.SolidBrush(gg.RGBA{R: 0.2, G: 0.3, B: 0.8, A: 0.8}))
	b.StrokeCircle(cx, cy, 60, scene.SolidBrush(gg.RGBA{R: 0.4, G: 0.5, B: 1, A: 1}), 2)

	// Orbit ring
	b.StrokeCircle(cx, cy, radius, scene.SolidBrush(gg.RGBA{R: 0.3, G: 0.3, B: 0.4, A: 0.5}), 1)

	// Corner shapes
	b.FillRect(20, 20, 80, 80, scene.SolidBrush(gg.RGBA{R: 0.9, G: 0.3, B: 0.3, A: 0.7}))
	b.Fill(scene.NewRoundedRectShape(float32(w)-100, 20, 80, 80, 12),
		scene.SolidBrush(gg.RGBA{R: 0.3, G: 0.9, B: 0.3, A: 0.7}))
	b.Fill(scene.NewRoundedRectShape(20, float32(h)-100, 80, 80, 12),
		scene.SolidBrush(gg.RGBA{R: 0.3, G: 0.3, B: 0.9, A: 0.7}))
	b.FillRect(float32(w)-100, float32(h)-100, 80, 80,
		scene.SolidBrush(gg.RGBA{R: 0.9, G: 0.9, B: 0.3, A: 0.7}))

	// Layer with blend mode
	b.Layer(scene.BlendScreen, 0.6, nil, func(lb *scene.SceneBuilder) {
		lb.FillCircle(cx-80, cy+80, 50,
			scene.SolidBrush(gg.RGBA{R: 1, G: 0.4, B: 0.2, A: 1}))
	})

	return b.Build()
}

func hueToRGB(h float64) gg.RGBA {
	h = h - math.Floor(h)
	s, v := 1.0, 1.0
	i := int(h * 6)
	f := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	case 5:
		r, g, b = v, p, q
	}
	return gg.RGBA{R: r, G: g, B: b, A: 1}
}
