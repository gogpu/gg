// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

// Command compute_pipeline is a visual demo comparing the Vello compute pipeline
// GPU output against the CPU reference implementation.
//
// It renders a rich scene through both paths and produces a triptych image
// (CPU | GPU | Diff) for visual inspection.
//
// Output:
//
//	tmp/compute_cpu.png         — CPU reference
//	tmp/compute_gpu.png         — GPU compute output
//	tmp/compute_comparison.png  — Side-by-side triptych with diff
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/gogpu/gg/internal/gpu"
	"github.com/gogpu/gg/internal/gpu/tilecompute"
)

const (
	canvasWidth  = 400
	canvasHeight = 300

	diffThreshold = 1.0 // Maximum acceptable diff percentage.
)

var bgColor = [4]uint8{255, 255, 255, 255} // White background.

func main() {
	fmt.Println("Vello Compute Pipeline Demo")
	fmt.Println("============================")
	fmt.Println()

	// Enable debug logging to see GPU initialization details.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// Full demo scene — 6 paths with triangles, circles, rects, star, transparency.
	paths := buildDemoScene()

	tilesX := (canvasWidth + tilecompute.TileWidth - 1) / tilecompute.TileWidth
	tilesY := (canvasHeight + tilecompute.TileHeight - 1) / tilecompute.TileHeight
	fmt.Printf("Scene: %d path(s)\n", len(paths))
	fmt.Printf("Canvas: %dx%d (%dx%d tiles)\n\n", canvasWidth, canvasHeight, tilesX, tilesY)

	// CPU render.
	cpuStart := time.Now()
	rast := tilecompute.NewRasterizer(canvasWidth, canvasHeight)
	cpuImg := rast.RasterizeScenePTCL(bgColor, paths)
	cpuDur := time.Since(cpuStart)
	fmt.Printf("CPU (tilecompute.RasterizeScenePTCL)... %v \u2713\n", cpuDur.Round(100*time.Microsecond))

	// GPU render.
	gpuImg, gpuDur, gpuErr := renderGPU(paths)
	if gpuErr != nil {
		fmt.Printf("GPU (VelloAccelerator.RenderSceneCompute)... SKIP (%v)\n", gpuErr)
	} else {
		fmt.Printf("GPU (VelloAccelerator.RenderSceneCompute)... %v \u2713\n", gpuDur.Round(100*time.Microsecond))
	}
	fmt.Println()

	// Ensure tmp directory exists.
	if err := os.MkdirAll("tmp", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot create tmp/: %v\n", err)
		os.Exit(1)
	}

	// Save CPU image.
	if err := savePNG(cpuImg, "tmp/compute_cpu.png"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: save CPU image: %v\n", err)
		os.Exit(1)
	}

	// Compare and save if GPU is available.
	if gpuImg == nil {
		fmt.Println("Output:")
		fmt.Println("  CPU:        tmp/compute_cpu.png")
		fmt.Println("  GPU:        (skipped - no GPU)")
		return
	}

	if err := savePNG(gpuImg, "tmp/compute_gpu.png"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: save GPU image: %v\n", err)
		os.Exit(1)
	}

	diffPercent, diffCount := comparePixels(cpuImg, gpuImg)
	totalPixels := canvasWidth * canvasHeight
	status := "PASS"
	if diffPercent > diffThreshold {
		status = "FAIL"
	}

	fmt.Println("Comparison:")
	fmt.Printf("  Pixel diff: %d / %d (%.2f%%)\n", diffCount, totalPixels, diffPercent)
	fmt.Printf("  Status: %s (threshold: %.1f%%)\n", status, diffThreshold)

	// Dump specific diagnostic pixels.
	type probe struct {
		x, y int
		desc string
	}
	probes := []probe{
		{230, 80, "blue rect interior"},
		{250, 160, "blue rect + purple circle"},
		{250, 200, "purple circle + white rect"},
		{260, 220, "purple + white overlap"},
		{250, 250, "purple circle only"},
		{100, 200, "star EvenOdd"},
		{80, 160, "triangle interior"},
		{340, 100, "red circle interior"},
	}
	fmt.Println("  Diagnostic pixels:")
	for _, p := range probes {
		ca := cpuImg.RGBAAt(p.x, p.y)
		cb := gpuImg.RGBAAt(p.x, p.y)
		match := "OK"
		if ca.R != cb.R || ca.G != cb.G || ca.B != cb.B || ca.A != cb.A {
			match = "DIFF"
		}
		fmt.Printf("    (%3d,%3d) %-35s CPU=(%3d,%3d,%3d,%3d) GPU=(%3d,%3d,%3d,%3d) %s\n",
			p.x, p.y, p.desc, ca.R, ca.G, ca.B, ca.A, cb.R, cb.G, cb.B, cb.A, match)
	}
	fmt.Println()

	triptych := buildTriptych(cpuImg, gpuImg)
	if err := savePNG(triptych, "tmp/compute_comparison.png"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: save comparison: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Output:")
	fmt.Println("  CPU:        tmp/compute_cpu.png")
	fmt.Println("  GPU:        tmp/compute_gpu.png")
	fmt.Println("  Comparison: tmp/compute_comparison.png")

	if status == "FAIL" {
		os.Exit(1)
	}
}

// buildDemoScene creates a rich scene exercising all compute pipeline features.
func buildDemoScene() []tilecompute.PathDef {
	return []tilecompute.PathDef{
		// 1. Green triangle (lines) - NonZero fill.
		{
			Lines:    triangleLines(60, 30, 160, 120, 20, 250),
			Color:    [4]uint8{0, 200, 0, 255},
			FillRule: tilecompute.FillRuleNonZero,
		},
		// 2. Blue rectangle (lines) - axis-aligned fill.
		{
			Lines:    rectLines(180, 40, 280, 140),
			Color:    [4]uint8{30, 60, 220, 255},
			FillRule: tilecompute.FillRuleNonZero,
		},
		// 3. Red circle (cubics) - curve flattening.
		{
			Lines:    tilecompute.FlattenFill(circleCubics(340, 100, 50)),
			Color:    [4]uint8{220, 30, 30, 255},
			FillRule: tilecompute.FillRuleNonZero,
		},
		// 4. Yellow 5-point star (lines) - EvenOdd fill rule.
		{
			Lines:    starLines(100, 200, 50),
			Color:    [4]uint8{230, 200, 0, 255},
			FillRule: tilecompute.FillRuleEvenOdd,
		},
		// 5. Purple semi-transparent large circle - overlapping compositing.
		{
			Lines:    tilecompute.FlattenFill(circleCubics(250, 200, 80)),
			Color:    [4]uint8{140, 40, 200, 128},
			FillRule: tilecompute.FillRuleNonZero,
		},
		// 6. White 80% small rectangle - overlapping on top.
		{
			Lines:    rectLines(220, 170, 290, 230),
			Color:    [4]uint8{255, 255, 255, 204},
			FillRule: tilecompute.FillRuleNonZero,
		},
	}
}

// renderGPU renders the scene through the Vello compute pipeline on the GPU.
// Returns nil image if GPU is unavailable.
func renderGPU(paths []tilecompute.PathDef) (*image.RGBA, time.Duration, error) {
	accel := &gpu.VelloAccelerator{}

	// Enable debug logging so GPU init errors are visible.
	debugLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	accel.SetLogger(debugLogger)

	if err := accel.InitStandalone(); err != nil {
		return nil, 0, fmt.Errorf("GPU init: %w", err)
	}
	defer accel.Close()

	if !accel.CanCompute() {
		return nil, 0, fmt.Errorf("compute pipeline not available (check stderr for details)")
	}

	start := time.Now()
	img, err := accel.RenderSceneCompute(canvasWidth, canvasHeight, bgColor, paths)
	dur := time.Since(start)
	if err != nil {
		return nil, 0, fmt.Errorf("render: %w", err)
	}
	return img, dur, nil
}

// comparePixels returns the percentage and count of pixels that differ between
// two images of the same dimensions.
func comparePixels(a, b *image.RGBA) (percent float64, count int) {
	bounds := a.Bounds()
	total := bounds.Dx() * bounds.Dy()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			ca := a.RGBAAt(x, y)
			cb := b.RGBAAt(x, y)
			if ca.R != cb.R || ca.G != cb.G || ca.B != cb.B || ca.A != cb.A {
				count++
			}
		}
	}
	percent = float64(count) / float64(total) * 100
	return
}

// buildTriptych creates a side-by-side image: CPU | GPU | Diff.
func buildTriptych(cpuImg, gpuImg *image.RGBA) *image.RGBA {
	triptych := image.NewRGBA(image.Rect(0, 0, canvasWidth*3, canvasHeight))

	// Panel 1: CPU reference.
	draw.Draw(triptych, image.Rect(0, 0, canvasWidth, canvasHeight), cpuImg, image.Point{}, draw.Src)

	// Panel 2: GPU compute.
	draw.Draw(triptych, image.Rect(canvasWidth, 0, canvasWidth*2, canvasHeight), gpuImg, image.Point{}, draw.Src)

	// Panel 3: Diff visualization.
	for y := 0; y < canvasHeight; y++ {
		for x := 0; x < canvasWidth; x++ {
			ca := cpuImg.RGBAAt(x, y)
			cb := gpuImg.RGBAAt(x, y)
			if ca.R != cb.R || ca.G != cb.G || ca.B != cb.B || ca.A != cb.A {
				// Different pixel: bright red.
				triptych.SetRGBA(canvasWidth*2+x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else {
				// Matching pixel: grayscale.
				gray := uint8((uint32(ca.R) + uint32(ca.G) + uint32(ca.B)) / 3)
				triptych.SetRGBA(canvasWidth*2+x, y, color.RGBA{R: gray, G: gray, B: gray, A: 255})
			}
		}
	}

	return triptych
}

// savePNG writes an RGBA image to a PNG file.
func savePNG(img *image.RGBA, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// --- Shape builders ---

func triangleLines(x0, y0, x1, y1, x2, y2 float32) []tilecompute.LineSoup {
	return []tilecompute.LineSoup{
		{PathIx: 0, P0: [2]float32{x0, y0}, P1: [2]float32{x1, y1}},
		{PathIx: 0, P0: [2]float32{x1, y1}, P1: [2]float32{x2, y2}},
		{PathIx: 0, P0: [2]float32{x2, y2}, P1: [2]float32{x0, y0}},
	}
}

func rectLines(x0, y0, x1, y1 float32) []tilecompute.LineSoup {
	return []tilecompute.LineSoup{
		{PathIx: 0, P0: [2]float32{x0, y0}, P1: [2]float32{x1, y0}},
		{PathIx: 0, P0: [2]float32{x1, y0}, P1: [2]float32{x1, y1}},
		{PathIx: 0, P0: [2]float32{x1, y1}, P1: [2]float32{x0, y1}},
		{PathIx: 0, P0: [2]float32{x0, y1}, P1: [2]float32{x0, y0}},
	}
}

func circleCubics(cx, cy, r float32) []tilecompute.CubicBezier {
	const kappa float32 = 0.5522847498
	k := r * kappa
	return []tilecompute.CubicBezier{
		{P0: [2]float32{cx + r, cy}, P1: [2]float32{cx + r, cy + k}, P2: [2]float32{cx + k, cy + r}, P3: [2]float32{cx, cy + r}},
		{P0: [2]float32{cx, cy + r}, P1: [2]float32{cx - k, cy + r}, P2: [2]float32{cx - r, cy + k}, P3: [2]float32{cx - r, cy}},
		{P0: [2]float32{cx - r, cy}, P1: [2]float32{cx - r, cy - k}, P2: [2]float32{cx - k, cy - r}, P3: [2]float32{cx, cy - r}},
		{P0: [2]float32{cx, cy - r}, P1: [2]float32{cx + k, cy - r}, P2: [2]float32{cx + r, cy - k}, P3: [2]float32{cx + r, cy}},
	}
}

func starLines(cx, cy, r float32) []tilecompute.LineSoup {
	// 5-point star vertices (tip at top).
	var vertices [5][2]float32
	for i := 0; i < 5; i++ {
		angle := -math.Pi/2 + float64(i)*2*math.Pi/5
		vertices[i] = [2]float32{
			cx + r*float32(math.Cos(angle)),
			cy + r*float32(math.Sin(angle)),
		}
	}

	// Star order: 0 -> 2 -> 4 -> 1 -> 3 -> 0 (skip one vertex).
	order := [6]int{0, 2, 4, 1, 3, 0}
	lines := make([]tilecompute.LineSoup, 5)
	for i := 0; i < 5; i++ {
		lines[i] = tilecompute.LineSoup{
			PathIx: 0,
			P0:     vertices[order[i]],
			P1:     vertices[order[i+1]],
		}
	}
	return lines
}
