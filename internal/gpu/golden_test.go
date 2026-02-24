//go:build !nogpu

// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package gpu

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogpu/gg/internal/raster"
	"github.com/gogpu/gg/scene"
)

// Vello upstream golden tests.
//
// These tests compare our TileRasterizer output against reference images
// from the upstream Vello repository (linebender/vello). Since our
// TileRasterizer is a port of Vello's CPU fine rasterizer, the correct
// ground truth is Vello's own output — not our AnalyticFiller.
//
// Reference images: testdata/golden/vello-upstream/
// Source: vello_tests/snapshots/ and sparse_strips/vello_sparse_tests/snapshots/

// VelloGoldenTest defines a test case with parameters matching an upstream
// Vello snapshot test exactly.
type VelloGoldenTest struct {
	Name      string       // Test name matching upstream snapshot filename
	Width     int          // Canvas width in pixels
	Height    int          // Canvas height in pixels
	FillColor color.RGBA   // Fill color (premultiplied over white bg)
	FillRule  raster.FillRule
	BuildPath func(eb *raster.EdgeBuilder) // Path builder
	Threshold float64 // Max acceptable different-pixel percentage
}

// VelloUpstreamTests returns test cases matching upstream Vello sparse strip
// snapshot tests. Parameters extracted from:
//
//	sparse_strips/vello_sparse_tests/tests/basic.rs
//
// Note: Vello smoke tests (vello_tests/snapshots/smoke/) use a different
// rendering pipeline (GPU compute) with BLACK background and are not directly
// comparable to our TileRasterizer (CPU fine rasterizer port). Sparse strip
// tests use WHITE background and match our implementation approach.
func VelloUpstreamTests() []VelloGoldenTest {
	return []VelloGoldenTest{
		{
			Name:      "filled_circle",
			Width:     100,
			Height:    100,
			FillColor: color.RGBA{R: 0, G: 255, B: 0, A: 255}, // css::LIME
			FillRule:  raster.FillRuleNonZero,
			Threshold: 5.0, // Curve approximation differences
			BuildPath: func(eb *raster.EdgeBuilder) {
				// Circle::new((50., 50.), 45.)
				path := scene.NewPath()
				path.Circle(50, 50, 45)
				eb.SetFlattenCurves(true)
				BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
			},
		},
		{
			Name:      "filled_triangle",
			Width:     100,
			Height:    100,
			FillColor: color.RGBA{R: 0, G: 255, B: 0, A: 255}, // css::LIME
			FillRule:  raster.FillRuleNonZero,
			Threshold: 5.0, // AA edge differences from curve flattening tolerance
			BuildPath: func(eb *raster.EdgeBuilder) {
				// move_to(5, 5), line_to(95, 50), line_to(5, 95), close
				path := scene.NewPath()
				path.MoveTo(5, 5)
				path.LineTo(95, 50)
				path.LineTo(5, 95)
				path.Close()
				eb.SetFlattenCurves(true)
				BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
			},
		},
		{
			Name:      "filling_nonzero_rule",
			Width:     100,
			Height:    100,
			FillColor: color.RGBA{R: 128, G: 0, B: 0, A: 255}, // css::MAROON
			FillRule:  raster.FillRuleNonZero,
			Threshold: 10.0, // Self-intersecting star: winding accumulation differs at edges
			BuildPath: func(eb *raster.EdgeBuilder) {
				// crossed_line_star: 5-point star
				path := scene.NewPath()
				path.MoveTo(50, 10)
				path.LineTo(75, 90)
				path.LineTo(10, 40)
				path.LineTo(90, 40)
				path.LineTo(25, 90)
				path.LineTo(50, 10)
				path.Close()
				eb.SetFlattenCurves(true)
				BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
			},
		},
		{
			Name:      "filling_evenodd_rule",
			Width:     100,
			Height:    100,
			FillColor: color.RGBA{R: 128, G: 0, B: 0, A: 255}, // css::MAROON
			FillRule:  raster.FillRuleEvenOdd,
			Threshold: 10.0, // Self-intersecting star: EvenOdd winding differs at edges
			BuildPath: func(eb *raster.EdgeBuilder) {
				// Same crossed_line_star, but EvenOdd fill
				path := scene.NewPath()
				path.MoveTo(50, 10)
				path.LineTo(75, 90)
				path.LineTo(10, 40)
				path.LineTo(90, 40)
				path.LineTo(25, 90)
				path.LineTo(50, 10)
				path.Close()
				eb.SetFlattenCurves(true)
				BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())
			},
		},
	}
}

// upstreamGoldenDir returns the path to Vello upstream reference images.
func upstreamGoldenDir() string {
	return filepath.Join("..", "..", "testdata", "golden", "vello-upstream")
}

// upstreamGoldenPath returns the path for a specific upstream golden file.
func upstreamGoldenPath(name string) string {
	return filepath.Join(upstreamGoldenDir(), name+".png")
}

// renderWithTileRasterizer renders a scene using our TileRasterizer and
// composites the result onto a white background with the given fill color.
func renderWithTileRasterizer(tc VelloGoldenTest) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, tc.Width, tc.Height))

	// White background (matching Vello default)
	for y := 0; y < tc.Height; y++ {
		for x := 0; x < tc.Width; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	tr := NewTileRasterizer(tc.Width, tc.Height)
	eb := raster.NewEdgeBuilder(2) // 4x AA
	tc.BuildPath(eb)

	fc := tc.FillColor
	tr.Fill(eb, tc.FillRule, func(y int, runs *raster.AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha <= 0 {
				continue
			}
			// Alpha-composite fill color over white background
			a := float32(alpha) / 255.0
			r := uint8(float32(fc.R)*a + 255*(1-a))
			g := uint8(float32(fc.G)*a + 255*(1-a))
			b := uint8(float32(fc.B)*a + 255*(1-a))
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	})

	return img
}

// compareImages returns the percentage of different pixels between two images.
func compareImages(img1, img2 *image.RGBA) (diffPercent float64, diffCount int) {
	bounds := img1.Bounds()
	totalPixels := bounds.Dx() * bounds.Dy()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c1 := img1.RGBAAt(x, y)
			c2 := img2.RGBAAt(x, y)
			if c1.R != c2.R || c1.G != c2.G || c1.B != c2.B || c1.A != c2.A {
				diffCount++
			}
		}
	}

	diffPercent = float64(diffCount) / float64(totalPixels) * 100
	return
}

// loadUpstreamGolden loads a Vello upstream reference PNG and converts to RGBA.
func loadUpstreamGolden(t *testing.T, name string) *image.RGBA {
	t.Helper()
	path := upstreamGoldenPath(name)
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("upstream golden file not found: %v", err)
		return nil
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("failed to decode %s: %v", path, err)
	}

	// Convert to RGBA
	rgba := image.NewRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba
}

// saveDiffImage saves a visual diff image highlighting pixel differences.
func saveDiffImage(t *testing.T, name string, ours, reference *image.RGBA) {
	t.Helper()
	tmpDir := filepath.Join("..", "..", "tmp")
	_ = os.MkdirAll(tmpDir, 0o755)

	diffImg := image.NewRGBA(ours.Bounds())
	for y := ours.Bounds().Min.Y; y < ours.Bounds().Max.Y; y++ {
		for x := ours.Bounds().Min.X; x < ours.Bounds().Max.X; x++ {
			v := ours.RGBAAt(x, y)
			g := reference.RGBAAt(x, y)
			if v.R != g.R || v.G != g.G || v.B != g.B {
				diffImg.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else {
				gray := uint8((uint32(v.R) + uint32(v.G) + uint32(v.B)) / 3)
				diffImg.Set(x, y, color.RGBA{R: gray, G: gray, B: gray, A: 255})
			}
		}
	}

	diffPath := filepath.Join(tmpDir, "vello_upstream_diff_"+name+".png")
	if f, err := os.Create(diffPath); err == nil {
		_ = png.Encode(f, diffImg)
		f.Close()
		t.Logf("Diff image saved: %s", diffPath)
	}

	// Also save our output for comparison
	oursPath := filepath.Join(tmpDir, "vello_upstream_ours_"+name+".png")
	if f, err := os.Create(oursPath); err == nil {
		_ = png.Encode(f, ours)
		f.Close()
		t.Logf("Our output saved: %s", oursPath)
	}
}

// TestVelloAgainstUpstream compares TileRasterizer output against Vello
// upstream reference images. This validates that our Vello port produces
// output matching the original Rust implementation.
func TestVelloAgainstUpstream(t *testing.T) {
	tests := VelloUpstreamTests()

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			reference := loadUpstreamGolden(t, tc.Name)
			if reference == nil {
				return
			}

			// Verify dimensions match
			if reference.Bounds().Dx() != tc.Width || reference.Bounds().Dy() != tc.Height {
				t.Fatalf("upstream image %dx%d does not match expected %dx%d",
					reference.Bounds().Dx(), reference.Bounds().Dy(), tc.Width, tc.Height)
			}

			ours := renderWithTileRasterizer(tc)

			diffPercent, diffCount := compareImages(ours, reference)

			t.Logf("Scene: %s, Size: %dx%d, Diff: %d pixels (%.2f%%), Threshold: %.2f%%",
				tc.Name, tc.Width, tc.Height, diffCount, diffPercent, tc.Threshold)

			if diffPercent > tc.Threshold {
				saveDiffImage(t, tc.Name, ours, reference)
				t.Errorf("FAIL: %.2f%% pixel difference exceeds threshold %.2f%% "+
					"(our TileRasterizer vs Vello upstream reference)", diffPercent, tc.Threshold)
			}
		})
	}
}
