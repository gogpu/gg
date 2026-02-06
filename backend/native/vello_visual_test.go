// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"github.com/gogpu/gg/raster"
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
	eb := raster.NewEdgeBuilder(2) // 4x AA
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
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Render using tile rasterizer
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background white
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Render the circle with proper alpha blending
	tr.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha <= 0 {
				continue
			}
			// Alpha blend blue (0, 100, 200) over white background
			alphaF := float32(alpha) / 255.0
			invAlpha := 1.0 - alphaF
			r := uint8(0*alphaF + 255*invAlpha)
			g := uint8(100*alphaF + 255*invAlpha)
			b := uint8(200*alphaF + 255*invAlpha)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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
	eb := raster.NewEdgeBuilder(2) // 4x AA
	eb.SetFlattenCurves(true)

	// Create a rectangle ALIGNED to tile boundaries (multiples of 16)
	// This ensures backdrop propagation works correctly
	path := scene.NewPath()
	path.MoveTo(32, 32) // Tile boundary
	path.LineTo(160, 32)
	path.LineTo(160, 160)
	path.LineTo(32, 160)
	path.Close()
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Render
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background white
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Render the rectangle with proper alpha blending
	tr.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha <= 0 {
				continue
			}
			// Alpha blend green (0, 180, 100) over white background
			alphaF := float32(alpha) / 255.0
			invAlpha := 1.0 - alphaF
			r := uint8(0*alphaF + 255*invAlpha)
			g := uint8(180*alphaF + 255*invAlpha)
			b := uint8(100*alphaF + 255*invAlpha)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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
	eb := raster.NewEdgeBuilder(2) // 4x AA
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
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Render
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background white
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Render the diagonal with proper alpha blending
	tr.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha <= 0 {
				continue
			}
			// Alpha blend red (220, 50, 50) over white background
			alphaF := float32(alpha) / 255.0
			invAlpha := 1.0 - alphaF
			r := uint8(220*alphaF + 255*invAlpha)
			g := uint8(50*alphaF + 255*invAlpha)
			b := uint8(50*alphaF + 255*invAlpha)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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
	radius := float32(80)
	const k = 0.5522847498

	buildCircle := func(eb *raster.EdgeBuilder) {
		path := scene.NewPath()
		path.MoveTo(cx+radius, cy)
		path.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
		path.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
		path.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
		path.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
		path.Close()
		eb.SetFlattenCurves(true)
		BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
	}

	// Render with Vello tile rasterizer
	velloImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			velloImg.Set(x, y, color.White)
		}
	}

	tr := NewTileRasterizer(width, height)
	eb1 := raster.NewEdgeBuilder(2)
	buildCircle(eb1)
	tr.Fill(eb1, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha <= 0 {
				continue
			}
			// Alpha blend with white background
			a := float32(alpha) / 255.0
			r := uint8(float32(0)*a + 255*(1-a))
			g := uint8(float32(100)*a + 255*(1-a))
			b := uint8(float32(200)*a + 255*(1-a))
			velloImg.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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
	eb2 := raster.NewEdgeBuilder(2)
	buildCircle(eb2)
	af.Fill(eb2, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha <= 0 {
				continue
			}
			// Alpha blend with white background
			a := float32(alpha) / 255.0
			r := uint8(float32(0)*a + 255*(1-a))
			g := uint8(float32(100)*a + 255*(1-a))
			b := uint8(float32(200)*a + 255*(1-a))
			origImg.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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

// TestVelloCompareSquare compares Vello tile rasterizer output
// with the original analytic filler for a square shape.
func TestVelloCompareSquare(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping visual test in short mode")
	}

	width, height := 20, 20

	// Create a square path
	buildSquare := func(eb *raster.EdgeBuilder) {
		path := scene.NewPath()
		path.MoveTo(7, 7)
		path.LineTo(13, 7)
		path.LineTo(13, 13)
		path.LineTo(7, 13)
		path.Close()
		eb.SetFlattenCurves(true)
		BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
	}

	// Render with Vello tile rasterizer
	velloImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			velloImg.Set(x, y, color.White)
		}
	}

	tr := NewTileRasterizer(width, height)
	eb1 := raster.NewEdgeBuilder(2)
	buildSquare(eb1)
	tr.Fill(eb1, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha <= 0 {
				continue
			}
			// Alpha blend blue with white background
			a := float32(alpha) / 255.0
			r := uint8(255 * (1 - a))
			g := uint8(255 * (1 - a))
			b := uint8(255*a + 255*(1-a))
			velloImg.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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
	eb2 := raster.NewEdgeBuilder(2)
	buildSquare(eb2)
	af.Fill(eb2, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha <= 0 {
				continue
			}
			// Alpha blend blue with white background
			a := float32(alpha) / 255.0
			r := uint8(255 * (1 - a))
			g := uint8(255 * (1 - a))
			b := uint8(255*a + 255*(1-a))
			origImg.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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
				if diffCount <= 20 {
					// Calc alpha from blue component (b = 255*a + 255*(1-a) = 255, so use R or G)
					// r = 255 * (1 - a), so a = 1 - r/255
					vAlpha := 255 - int(v.R)
					oAlpha := 255 - int(o.R)
					t.Logf("Diff at (%d,%d): Vello α=%d, Orig α=%d, diff=%d",
						x, y, vAlpha, oAlpha, vAlpha-oAlpha)
				}
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

	saveImage(t, velloImg, filepath.Join(tmpDir, "square_vello.png"))
	saveImage(t, origImg, filepath.Join(tmpDir, "square_original.png"))
	saveImage(t, diffImg, filepath.Join(tmpDir, "square_diff.png"))

	t.Logf("Difference pixels: %d (%.2f%%)", diffCount, float64(diffCount)/float64(width*height)*100)
	t.Logf("Images saved to: %s", tmpDir)
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

// TestVelloGoldenComparison is deprecated.
// Use TestVelloAgainstGolden in golden_test.go instead.
// This test is kept for backward compatibility but skipped.
func TestVelloGoldenComparison(t *testing.T) {
	t.Skip("Deprecated: use TestVelloAgainstGolden instead (see golden_test.go)")
}

// TestVelloSmokeSquare replicates Vello's smoke test: filled_square
// Original: 20x20 image, blue square at center (10,10) size 6x6
func TestVelloSmokeSquare(t *testing.T) {
	width, height := 20, 20
	tr := NewTileRasterizer(width, height)
	eb := raster.NewEdgeBuilder(2) // 4x AA
	eb.SetFlattenCurves(true)

	// Rect from center (10,10) size (6,6) = corners at (7,7) to (13,13)
	path := scene.NewPath()
	path.MoveTo(7, 7)
	path.LineTo(13, 7)
	path.LineTo(13, 13)
	path.LineTo(7, 13)
	path.Close()
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// White background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Blue fill (matching Vello's palette::css::BLUE = #0000FF)
	tr.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 { //nolint:gocritic // nestingReduce: test rendering callback
				// Alpha blend blue with white background
				a := float32(alpha) / 255.0
				r := uint8(255 * (1 - a))
				g := uint8(255 * (1 - a))
				b := uint8(255*a + 255*(1-a))
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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
	eb := raster.NewEdgeBuilder(2) // 4x AA
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
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// White background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Blue fill
	tr.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 { //nolint:gocritic // nestingReduce: test rendering callback
				// Alpha blend blue with white background
				a := float32(alpha) / 255.0
				r := uint8(255 * (1 - a))
				g := uint8(255 * (1 - a))
				b := uint8(255*a + 255*(1-a))
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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

	buildDiagonal := func(eb *raster.EdgeBuilder) {
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
		BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
	}

	// Render with Vello tile rasterizer
	velloImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			velloImg.Set(x, y, color.White)
		}
	}

	tr := NewTileRasterizer(width, height)
	eb1 := raster.NewEdgeBuilder(2)
	buildDiagonal(eb1)
	tr.Fill(eb1, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 { //nolint:gocritic // nestingReduce: test rendering callback
				// Alpha blend red with white background
				a := float32(alpha) / 255.0
				r := uint8(220*a + 255*(1-a))
				g := uint8(50*a + 255*(1-a))
				b := uint8(50*a + 255*(1-a))
				velloImg.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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
	eb2 := raster.NewEdgeBuilder(2)
	buildDiagonal(eb2)
	af.Fill(eb2, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 { //nolint:gocritic // nestingReduce: test rendering callback
				// Alpha blend red with white background
				a := float32(alpha) / 255.0
				r := uint8(220*a + 255*(1-a))
				g := uint8(50*a + 255*(1-a))
				b := uint8(50*a + 255*(1-a))
				origImg.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
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

// TestEdgeDebug shows what edges are created for the diagonal polygon
func TestEdgeDebug(t *testing.T) {
	eb := raster.NewEdgeBuilder(2)

	thickness := float32(20)
	path := scene.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(10+thickness, 10)   // → (30, 10)
	path.LineTo(190, 190-thickness) // → (190, 170)
	path.LineTo(190, 190)
	path.LineTo(190-thickness, 190) // → (170, 190)
	path.LineTo(10, 10+thickness)   // → (10, 30)
	path.Close()
	eb.SetFlattenCurves(true)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	t.Log("Edges in polygon:")
	i := 0
	for edge := range eb.AllEdges() {
		line := edge.AsLine()
		x0 := float32(line.X) / 65536.0 // raster.FDot16 to float
		dx := float32(line.DX) / 65536.0
		y0 := float32(line.FirstY)
		y1 := float32(line.LastY)
		x1 := x0 + dx*(y1-y0)
		t.Logf("  Edge %d: (%.1f,%.1f) → (%.1f,%.1f)  DX=%.3f  Y range=[%d,%d]",
			i, x0, y0, x1, y1, dx, line.FirstY, line.LastY)
		i++
	}
}

// TestVelloTileDebugDiagonal shows tile structure for diagonal to debug backdrop issue
func TestVelloTileDebugDiagonal(t *testing.T) {
	width, height := 200, 200
	tr := NewTileRasterizer(width, height)
	eb := raster.NewEdgeBuilder(2)

	// Same diagonal as comparison test
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
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Process tiles
	tr.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {})

	// Show first 4x4 tiles
	t.Logf("Tile structure (first 4x4 tiles):")
	t.Logf("Note: Tile(x,y) covers pixels X=[%d*16, %d*16+15], Y=[y*16, y*16+15]", 0, 0)
	for ty := 0; ty < 4 && ty < tr.tilesY; ty++ {
		for tx := 0; tx < 4 && tx < tr.tilesX; tx++ {
			tile := &tr.tiles[ty*tr.tilesX+tx]
			if tile.Backdrop != 0 || len(tile.Segments) > 0 {
				t.Logf("  Tile(%d,%d) [pix X=%d-%d, Y=%d-%d]: Backdrop=%d, Segments=%d",
					tx, ty, tx*16, tx*16+15, ty*16, ty*16+15, tile.Backdrop, len(tile.Segments))
				for si, seg := range tile.Segments {
					t.Logf("    Seg[%d]: P0=(%.1f,%.1f) P1=(%.1f,%.1f) YEdge=%.1f",
						si, seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1], seg.YEdge)
				}
			}
		}
	}
}

// TestVelloCircleDebug shows tile structure for circle at bottom-left where artifacts occur
func TestVelloCircleDebug(t *testing.T) {
	width, height := 200, 200
	tr := NewTileRasterizer(width, height)
	eb := raster.NewEdgeBuilder(2)

	// Same circle as comparison test
	cx, cy := float32(100), float32(100)
	radius := float32(60)
	const k = 0.5522847498

	path := scene.NewPath()
	path.MoveTo(cx+radius, cy)
	path.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
	path.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
	path.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
	path.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
	path.Close()
	eb.SetFlattenCurves(true)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	// Process tiles
	tr.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {})

	// Circle: center (100,100), radius 60
	// Left edge at X=40, tiles around tx=2 (32-47)
	// Artifacts at Y≈123-133, tiles ty=7 (112-127) and ty=8 (128-143)
	t.Logf("Circle debug: center=(%.0f,%.0f), radius=%.0f", cx, cy, radius)
	t.Logf("Left edge at X=%.0f (tile X=2, pixels 32-47)", cx-radius)
	t.Log("Tiles around bottom-left of circle (where artifacts occur):")

	for ty := 6; ty <= 10 && ty < tr.tilesY; ty++ {
		for tx := 1; tx <= 4 && tx < tr.tilesX; tx++ {
			tile := &tr.tiles[ty*tr.tilesX+tx]
			if tile.Backdrop != 0 || len(tile.Segments) > 0 {
				t.Logf("  Tile(%d,%d) [X=%d-%d, Y=%d-%d]: Backdrop=%d, Segments=%d",
					tx, ty, tx*16, tx*16+15, ty*16, ty*16+15, tile.Backdrop, len(tile.Segments))
				for si, seg := range tile.Segments {
					dx := seg.Point1[0] - seg.Point0[0]
					dy := seg.Point1[1] - seg.Point0[1]
					exitsRight := seg.Point1[0] >= 15.9 && seg.Point1[0] <= 16.1
					exitsBottom := seg.Point1[1] >= 15.9 && seg.Point1[1] <= 16.1
					exitsLeft := seg.Point1[0] <= 0.1
					t.Logf("    Seg[%d]: (%.2f,%.2f)→(%.2f,%.2f) dx=%.2f dy=%.2f yEdge=%.2f exits[R=%v,B=%v,L=%v]",
						si, seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1],
						dx, dy, seg.YEdge, exitsRight, exitsBottom, exitsLeft)
				}
			}
		}
	}
}

// TestVelloCircleBottomDebug finds exact coordinates of difference pixels at circle bottom
func TestVelloCircleBottomDebug(t *testing.T) {
	width, height := 200, 200

	// Create Vello image
	tr := NewTileRasterizer(width, height)
	eb := raster.NewEdgeBuilder(2)

	cx, cy := float32(100), float32(100)
	radius := float32(60)
	const k = 0.5522847498

	path := scene.NewPath()
	path.MoveTo(cx+radius, cy)
	path.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
	path.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
	path.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
	path.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
	path.Close()
	eb.SetFlattenCurves(true)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	velloImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			velloImg.Set(x, y, color.White)
		}
	}

	tr.Fill(eb, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 { //nolint:gocritic // nestingReduce: test rendering callback
				a := float32(alpha) / 255.0
				r := uint8(float32(0)*a + 255*(1-a))
				g := uint8(float32(100)*a + 255*(1-a))
				b := uint8(float32(200)*a + 255*(1-a))
				velloImg.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}
	})

	// Create reference (AnalyticFiller) image
	origImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			origImg.Set(x, y, color.White)
		}
	}

	af := NewAnalyticFiller(width, height)
	eb2 := raster.NewEdgeBuilder(2)
	path2 := scene.NewPath()
	path2.MoveTo(cx+radius, cy)
	path2.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
	path2.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
	path2.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
	path2.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
	path2.Close()
	eb2.SetFlattenCurves(true)
	BuildEdgesFromScenePath(eb2, path2, scene.IdentityAffine())

	af.Fill(eb2, raster.FillRuleNonZero, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 { //nolint:gocritic // nestingReduce: test rendering callback
				a := float32(alpha) / 255.0
				r := uint8(float32(0)*a + 255*(1-a))
				g := uint8(float32(100)*a + 255*(1-a))
				b := uint8(float32(200)*a + 255*(1-a))
				origImg.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}
	})

	// Find all difference pixels
	t.Log("=== DIFFERENCE PIXELS (Vello vs AnalyticFiller) ===")
	t.Log("Circle: center=(100,100), radius=60")
	t.Log("Bottom of circle at Y=160")

	var diffPixels [][4]int // x, y, velloAlpha, origAlpha
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := velloImg.RGBAAt(x, y)
			o := origImg.RGBAAt(x, y)
			if v.R != o.R || v.G != o.G || v.B != o.B {
				// Compute approximate alpha from blue channel
				vAlpha := 255 - int(v.R)
				oAlpha := 255 - int(o.R)
				diffPixels = append(diffPixels, [4]int{x, y, vAlpha, oAlpha})
			}
		}
	}

	t.Logf("Total difference pixels: %d", len(diffPixels))
	for _, p := range diffPixels {
		tileX, tileY := p[0]/16, p[1]/16
		t.Logf("  (%3d, %3d) Tile(%d,%d) - Vello alpha≈%3d, Orig alpha≈%3d, diff=%+d",
			p[0], p[1], tileX, tileY, p[2], p[3], p[2]-p[3])
	}

	// Debug tiles around bottom of circle (Y=155-165, X=90-130)
	t.Log("\n=== TILES AROUND BOTTOM OF CIRCLE ===")
	for ty := 9; ty <= 10 && ty < tr.tilesY; ty++ {
		for tx := 5; tx <= 8 && tx < tr.tilesX; tx++ {
			tile := &tr.tiles[ty*tr.tilesX+tx]
			if tile.Backdrop != 0 || len(tile.Segments) > 0 {
				t.Logf("Tile(%d,%d) [X=%d-%d, Y=%d-%d]: Backdrop=%d, Segments=%d",
					tx, ty, tx*16, tx*16+15, ty*16, ty*16+15, tile.Backdrop, len(tile.Segments))
				for si, seg := range tile.Segments {
					dx := seg.Point1[0] - seg.Point0[0]
					dy := seg.Point1[1] - seg.Point0[1]
					t.Logf("  Seg[%d]: (%.2f,%.2f)→(%.2f,%.2f) dx=%.2f dy=%.2f yEdge=%.2f",
						si, seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1],
						dx, dy, seg.YEdge)
				}
			}
		}
	}
}
