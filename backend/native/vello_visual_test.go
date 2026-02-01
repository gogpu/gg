// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogpu/gg/scene"
)

// TestVelloVisualCircle renders a circle using the Vello tile rasterizer
// and saves the result as a PNG for visual inspection.
func TestVelloVisualCircle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping visual test in short mode")
	}

	width, height := 200, 200
	tr := NewTileRasterizer(width, height)
	eb := NewEdgeBuilder(2) // 4x AA
	eb.SetFlattenCurves(true)

	// Create a circle at center (100, 100) with radius 80
	cx, cy := float32(100), float32(100)
	radius := float32(80)

	// Approximate circle with bezier curves
	// Using 4 cubic beziers (standard circle approximation)
	const k = 0.5522847498 // Magic number for cubic bezier circle approximation

	path := scene.NewPath()
	path.MoveTo(cx+radius, cy)

	// Top-right quadrant
	path.CubicTo(
		cx+radius, cy-radius*k,
		cx+radius*k, cy-radius,
		cx, cy-radius,
	)
	// Top-left quadrant
	path.CubicTo(
		cx-radius*k, cy-radius,
		cx-radius, cy-radius*k,
		cx-radius, cy,
	)
	// Bottom-left quadrant
	path.CubicTo(
		cx-radius, cy+radius*k,
		cx-radius*k, cy+radius,
		cx, cy+radius,
	)
	// Bottom-right quadrant
	path.CubicTo(
		cx+radius*k, cy+radius,
		cx+radius, cy+radius*k,
		cx+radius, cy,
	)

	path.Close()
	eb.BuildFromScenePath(path, scene.IdentityAffine())

	// Render using tile rasterizer
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background white
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Render the circle with proper alpha blending
	tr.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				// Alpha blend blue (0, 100, 200) over white background
				alphaF := float32(alpha) / 255.0
				invAlpha := 1.0 - alphaF
				r := uint8(0*alphaF + 255*invAlpha)
				g := uint8(100*alphaF + 255*invAlpha)
				b := uint8(200*alphaF + 255*invAlpha)
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}
	})

	// Save to project tmp directory
	tmpDir := "../../tmp"
	_ = os.MkdirAll(tmpDir, 0o755)
	outPath := filepath.Join(tmpDir, "vello_circle.png")

	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("failed to create output file: %v", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		t.Fatalf("failed to encode PNG: %v", err)
	}

	t.Logf("Saved visual test to: %s", outPath)
}

// TestVelloVisualRectangle renders a simple rectangle for basic verification.
func TestVelloVisualRectangle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping visual test in short mode")
	}

	width, height := 200, 200
	tr := NewTileRasterizer(width, height)
	eb := NewEdgeBuilder(2) // 4x AA
	eb.SetFlattenCurves(true)

	// Create a rectangle ALIGNED to tile boundaries (multiples of 16)
	// This ensures backdrop propagation works correctly
	path := scene.NewPath()
	path.MoveTo(32, 32) // Tile boundary
	path.LineTo(160, 32)
	path.LineTo(160, 160)
	path.LineTo(32, 160)
	path.Close()
	eb.BuildFromScenePath(path, scene.IdentityAffine())

	// Render
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background white
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Render the rectangle with proper alpha blending
	tr.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				// Alpha blend green (0, 180, 100) over white background
				alphaF := float32(alpha) / 255.0
				invAlpha := 1.0 - alphaF
				r := uint8(0*alphaF + 255*invAlpha)
				g := uint8(180*alphaF + 255*invAlpha)
				b := uint8(100*alphaF + 255*invAlpha)
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}
	})

	// Save to tmp directory
	tmpDir := "../../tmp"
	_ = os.MkdirAll(tmpDir, 0o755)
	outPath := filepath.Join(tmpDir, "vello_rectangle.png")

	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("failed to create output file: %v", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		t.Fatalf("failed to encode PNG: %v", err)
	}

	t.Logf("Saved visual test to: %s", outPath)
}

// TestVelloVisualDiagonalLine tests a diagonal line to check for dark bands.
func TestVelloVisualDiagonalLine(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping visual test in short mode")
	}

	width, height := 200, 200
	tr := NewTileRasterizer(width, height)
	eb := NewEdgeBuilder(2) // 4x AA
	eb.SetFlattenCurves(true)

	// Create a thick diagonal line (as a polygon)
	// From top-left to bottom-right with thickness
	thickness := float32(20)

	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(10+thickness, 10)
	path.LineTo(190, 190-thickness)
	path.LineTo(190, 190)
	path.LineTo(190-thickness, 190)
	path.LineTo(10, 10+thickness)
	path.Close()
	eb.BuildFromScenePath(path, scene.IdentityAffine())

	// Render
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background white
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Render the diagonal with proper alpha blending
	tr.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				// Alpha blend red (220, 50, 50) over white background
				alphaF := float32(alpha) / 255.0
				invAlpha := 1.0 - alphaF
				r := uint8(220*alphaF + 255*invAlpha)
				g := uint8(50*alphaF + 255*invAlpha)
				b := uint8(50*alphaF + 255*invAlpha)
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}
	})

	// Save to tmp directory
	tmpDir := "../../tmp"
	_ = os.MkdirAll(tmpDir, 0o755)
	outPath := filepath.Join(tmpDir, "vello_diagonal.png")

	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("failed to create output file: %v", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		t.Fatalf("failed to encode PNG: %v", err)
	}

	t.Logf("Saved visual test to: %s", outPath)
}

// TestVelloCompareWithOriginal compares Vello tile rasterizer output
// with the original analytic filler to detect differences.
func TestVelloCompareWithOriginal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping visual test in short mode")
	}

	width, height := 200, 200

	// Create a circle path
	cx, cy := float32(100), float32(100)
	radius := float32(60)
	const k = 0.5522847498

	buildCircle := func(eb *EdgeBuilder) {
		path := scene.NewPath()
		path.MoveTo(cx+radius, cy)
		path.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
		path.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
		path.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
		path.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
		path.Close()
		eb.SetFlattenCurves(true)
		eb.BuildFromScenePath(path, scene.IdentityAffine())
	}

	// Render with Vello tile rasterizer
	velloImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			velloImg.Set(x, y, color.White)
		}
	}

	tr := NewTileRasterizer(width, height)
	eb1 := NewEdgeBuilder(2)
	buildCircle(eb1)
	tr.Fill(eb1, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				c := color.RGBA{R: 0, G: 100, B: 200, A: alpha}
				velloImg.Set(x, y, c)
			}
		}
	})

	// Render with original analytic filler
	origImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			origImg.Set(x, y, color.White)
		}
	}

	af := NewAnalyticFiller(width, height)
	eb2 := NewEdgeBuilder(2)
	buildCircle(eb2)
	af.Fill(eb2, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				c := color.RGBA{R: 0, G: 100, B: 200, A: alpha}
				origImg.Set(x, y, c)
			}
		}
	})

	// Create difference image
	diffImg := image.NewRGBA(image.Rect(0, 0, width, height))
	var diffCount int

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := velloImg.RGBAAt(x, y)
			o := origImg.RGBAAt(x, y)

			if v.R != o.R || v.G != o.G || v.B != o.B || v.A != o.A {
				diffCount++
				// Highlight difference in red
				diffImg.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else {
				// Grayscale the matching pixels
				gray := uint8((uint32(v.R) + uint32(v.G) + uint32(v.B)) / 3)
				diffImg.Set(x, y, color.RGBA{R: gray, G: gray, B: gray, A: 255})
			}
		}
	}

	// Save all three images
	tmpDir := "../../tmp"
	_ = os.MkdirAll(tmpDir, 0o755)

	saveImage(t, velloImg, filepath.Join(tmpDir, "compare_vello.png"))
	saveImage(t, origImg, filepath.Join(tmpDir, "compare_original.png"))
	saveImage(t, diffImg, filepath.Join(tmpDir, "compare_diff.png"))

	t.Logf("Difference pixels: %d (%.2f%%)", diffCount, float64(diffCount)/float64(width*height)*100)
	t.Logf("Images saved to: %s", tmpDir)

	// Note: Some difference is expected due to different algorithms
	// Dark bands would show as significant difference at tile boundaries
}

func saveImage(t *testing.T, img image.Image, path string) {
	t.Helper()
	// Scale up 3x for better visibility of artifacts
	scaled := scaleImage(img, 3)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, scaled); err != nil {
		t.Fatalf("failed to encode %s: %v", path, err)
	}
}

// scaleImage scales an image by the given factor using nearest neighbor.
func scaleImage(img image.Image, scale int) *image.RGBA {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	scaled := image.NewRGBA(image.Rect(0, 0, w*scale, h*scale))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.At(bounds.Min.X+x, bounds.Min.Y+y)
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					scaled.Set(x*scale+dx, y*scale+dy, c)
				}
			}
		}
	}
	return scaled
}

// TestVelloGoldenComparison compares our output with Vello's golden files
func TestVelloGoldenComparison(t *testing.T) {
	tests := []struct {
		name       string
		goldenPath string
		threshold  float64 // per-shape threshold (percent)
		buildPath  func(eb *EdgeBuilder)
	}{
		{
			name:       "square",
			goldenPath: "../../testdata/vello_golden_square.png",
			threshold:  1.0, // Squares should be exact
			buildPath: func(eb *EdgeBuilder) {
				// Rect from center (10,10) size (6,6) = corners at (7,7) to (13,13)
				path := scene.NewPath()
				path.MoveTo(7, 7)
				path.LineTo(13, 7)
				path.LineTo(13, 13)
				path.LineTo(7, 13)
				path.Close()
				eb.BuildFromScenePath(path, scene.IdentityAffine())
			},
		},
		{
			name:       "circle",
			goldenPath: "../../testdata/vello_golden_circle.png",
			threshold:  15.0, // Circles have edge AA differences due to curve flattening
			buildPath: func(eb *EdgeBuilder) {
				// Circle at (10, 10) radius 7
				cx, cy := float32(10), float32(10)
				radius := float32(7)
				const k = 0.5522847498
				path := scene.NewPath()
				path.MoveTo(cx+radius, cy)
				path.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
				path.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
				path.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
				path.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
				path.Close()
				eb.BuildFromScenePath(path, scene.IdentityAffine())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height := 20, 20
			tr := NewTileRasterizer(width, height)
			eb := NewEdgeBuilder(2)
			eb.SetFlattenCurves(true)
			tt.buildPath(eb)

			// Render with BLACK background (like Vello's golden files)
			rendered := image.NewRGBA(image.Rect(0, 0, width, height))
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					rendered.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 255})
				}
			}
			tr.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
				for x, alpha := range runs.Iter() {
					if alpha > 0 {
						// Alpha blend blue onto black background
						c := color.RGBA{R: 0, G: 0, B: 255, A: alpha}
						rendered.Set(x, y, c)
					}
				}
			})

			// Load golden
			goldenFile, err := os.Open(tt.goldenPath)
			if err != nil {
				t.Fatalf("failed to open golden: %v", err)
			}
			defer goldenFile.Close()
			golden, err := png.Decode(goldenFile)
			if err != nil {
				t.Fatalf("failed to decode golden: %v", err)
			}

			// Compare
			var diffCount, totalPixels int
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					totalPixels++
					r1, g1, b1, a1 := rendered.At(x, y).RGBA()
					r2, g2, b2, a2 := golden.At(x, y).RGBA()
					if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
						diffCount++
					}
				}
			}

			diffPct := float64(diffCount) / float64(totalPixels) * 100
			t.Logf("Golden comparison: %d different pixels (%.2f%%)", diffCount, diffPct)

			// Save our output for visual inspection
			tmpDir := "../../tmp"
			saveImage(t, rendered, filepath.Join(tmpDir, "golden_compare_"+tt.name+".png"))

			// Use per-shape threshold
			if diffPct > tt.threshold {
				t.Errorf("Too many different pixels: %.2f%% (threshold %.1f%%)", diffPct, tt.threshold)
			}
		})
	}
}

// TestVelloSmokeSquare replicates Vello's smoke test: filled_square
// Original: 20x20 image, blue square at center (10,10) size 6x6
func TestVelloSmokeSquare(t *testing.T) {
	width, height := 20, 20
	tr := NewTileRasterizer(width, height)
	eb := NewEdgeBuilder(2) // 4x AA
	eb.SetFlattenCurves(true)

	// Rect from center (10,10) size (6,6) = corners at (7,7) to (13,13)
	path := scene.NewPath()
	path.MoveTo(7, 7)
	path.LineTo(13, 7)
	path.LineTo(13, 13)
	path.LineTo(7, 13)
	path.Close()
	eb.BuildFromScenePath(path, scene.IdentityAffine())

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// White background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Blue fill (matching Vello's palette::css::BLUE = #0000FF)
	tr.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				c := color.RGBA{R: 0, G: 0, B: 255, A: alpha}
				img.Set(x, y, c)
			}
		}
	})

	tmpDir := "../../tmp"
	saveImage(t, img, filepath.Join(tmpDir, "smoke_square.png"))
	t.Logf("Saved smoke square to: %s/smoke_square.png", tmpDir)
}

// TestVelloSmokeCircle replicates Vello's smoke test: filled_circle
// Original: 20x20 image, blue circle at center (10,10) radius 7
func TestVelloSmokeCircle(t *testing.T) {
	width, height := 20, 20
	tr := NewTileRasterizer(width, height)
	eb := NewEdgeBuilder(2) // 4x AA
	eb.SetFlattenCurves(true)

	// Circle at (10, 10) radius 7
	cx, cy := float32(10), float32(10)
	radius := float32(7)
	const k = 0.5522847498

	path := scene.NewPath()
	path.MoveTo(cx+radius, cy)
	path.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
	path.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
	path.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
	path.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
	path.Close()
	eb.BuildFromScenePath(path, scene.IdentityAffine())

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// White background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Blue fill
	tr.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				c := color.RGBA{R: 0, G: 0, B: 255, A: alpha}
				img.Set(x, y, c)
			}
		}
	})

	tmpDir := "../../tmp"
	saveImage(t, img, filepath.Join(tmpDir, "smoke_circle.png"))
	t.Logf("Saved smoke circle to: %s/smoke_circle.png", tmpDir)
}

// TestVelloCompareDiagonal compares diagonal line rendering between
// Vello tile rasterizer and the original analytic filler.
func TestVelloCompareDiagonal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping visual test in short mode")
	}

	width, height := 200, 200

	buildDiagonal := func(eb *EdgeBuilder) {
		thickness := float32(20)
		path := scene.NewPath()
		path.MoveTo(10, 10)
		path.LineTo(10+thickness, 10)
		path.LineTo(190, 190-thickness)
		path.LineTo(190, 190)
		path.LineTo(190-thickness, 190)
		path.LineTo(10, 10+thickness)
		path.Close()
		eb.SetFlattenCurves(true)
		eb.BuildFromScenePath(path, scene.IdentityAffine())
	}

	// Render with Vello tile rasterizer
	velloImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			velloImg.Set(x, y, color.White)
		}
	}

	tr := NewTileRasterizer(width, height)
	eb1 := NewEdgeBuilder(2)
	buildDiagonal(eb1)
	tr.Fill(eb1, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				c := color.RGBA{R: 220, G: 50, B: 50, A: alpha}
				velloImg.Set(x, y, c)
			}
		}
	})

	// Render with original analytic filler
	origImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			origImg.Set(x, y, color.White)
		}
	}

	af := NewAnalyticFiller(width, height)
	eb2 := NewEdgeBuilder(2)
	buildDiagonal(eb2)
	af.Fill(eb2, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				c := color.RGBA{R: 220, G: 50, B: 50, A: alpha}
				origImg.Set(x, y, c)
			}
		}
	})

	// Create difference image
	diffImg := image.NewRGBA(image.Rect(0, 0, width, height))
	var diffCount int

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := velloImg.RGBAAt(x, y)
			o := origImg.RGBAAt(x, y)

			if v.R != o.R || v.G != o.G || v.B != o.B || v.A != o.A {
				diffCount++
				diffImg.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else {
				gray := uint8((uint32(v.R) + uint32(v.G) + uint32(v.B)) / 3)
				diffImg.Set(x, y, color.RGBA{R: gray, G: gray, B: gray, A: 255})
			}
		}
	}

	tmpDir := "../../tmp"
	_ = os.MkdirAll(tmpDir, 0o755)

	saveImage(t, velloImg, filepath.Join(tmpDir, "diag_vello.png"))
	saveImage(t, origImg, filepath.Join(tmpDir, "diag_original.png"))
	saveImage(t, diffImg, filepath.Join(tmpDir, "diag_diff.png"))

	t.Logf("Diagonal difference pixels: %d (%.2f%%)", diffCount, float64(diffCount)/float64(width*height)*100)
	t.Logf("Images saved to: %s", tmpDir)
}
