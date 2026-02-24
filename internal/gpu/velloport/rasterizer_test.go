// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package velloport

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

// goldenTest defines a test case matching an upstream Vello sparse strip snapshot.
type goldenTest struct {
	Name      string
	Width     int
	Height    int
	FillColor color.RGBA
	FillRule  FillRule
	// BuildPath uses EdgeBuilder for curves (circles). Horizontal lines get filtered!
	BuildPath func(eb *raster.EdgeBuilder)
	// BuildLines generates LineSoup directly for polygons — includes ALL lines
	// (including horizontal), matching Vello's flattener behavior exactly.
	BuildLines func() []LineSoup
}

func goldenTests() []goldenTest {
	return []goldenTest{
		{
			Name:      "filled_circle",
			Width:     100,
			Height:    100,
			FillColor: color.RGBA{R: 0, G: 255, B: 0, A: 255},
			FillRule:  FillRuleNonZero,
			BuildPath: func(eb *raster.EdgeBuilder) {
				path := scene.NewPath()
				path.Circle(50, 50, 45)
				eb.SetFlattenCurves(true)
				buildEdgesFromScenePath(eb, path)
			},
		},
		{
			Name:      "filled_triangle",
			Width:     100,
			Height:    100,
			FillColor: color.RGBA{R: 0, G: 255, B: 0, A: 255},
			FillRule:  FillRuleNonZero,
			// Direct LineSoup — no EdgeBuilder filtering
			BuildLines: func() []LineSoup {
				return polygonToLineSoup([][2]float32{
					{5, 5}, {95, 50}, {5, 95},
				})
			},
		},
		{
			Name:      "filling_nonzero_rule",
			Width:     100,
			Height:    100,
			FillColor: color.RGBA{R: 128, G: 0, B: 0, A: 255},
			FillRule:  FillRuleNonZero,
			// Direct LineSoup — includes horizontal (10,40)→(90,40)!
			BuildLines: func() []LineSoup {
				return polygonToLineSoup([][2]float32{
					{50, 10}, {75, 90}, {10, 40}, {90, 40}, {25, 90},
				})
			},
		},
		{
			Name:      "filling_evenodd_rule",
			Width:     100,
			Height:    100,
			FillColor: color.RGBA{R: 128, G: 0, B: 0, A: 255},
			FillRule:  FillRuleEvenOdd,
			// Direct LineSoup — includes horizontal (10,40)→(90,40)!
			BuildLines: func() []LineSoup {
				return polygonToLineSoup([][2]float32{
					{50, 10}, {75, 90}, {10, 40}, {90, 40}, {25, 90},
				})
			},
		},
	}
}

// polygonToLineSoup generates LineSoup from polygon vertices directly.
// Unlike EdgeBuilder, this includes ALL lines (even horizontal), matching
// how Vello's flattener emits lines before path_count filtering.
func polygonToLineSoup(vertices [][2]float32) []LineSoup {
	n := len(vertices)
	if n < 2 {
		return nil
	}
	lines := make([]LineSoup, 0, n)
	for i := 0; i < n; i++ {
		p0 := vertices[i]
		p1 := vertices[(i+1)%n]
		// Skip zero-length (degenerate) segments, but keep horizontal!
		if p0[0] == p1[0] && p0[1] == p1[1] {
			continue
		}
		lines = append(lines, LineSoup{PathIx: 0, P0: p0, P1: p1})
	}
	return lines
}

// scenePathAdapter wraps scene.Path to implement raster.PathLike.
type scenePathAdapter struct{ path *scene.Path }

func (a *scenePathAdapter) IsEmpty() bool     { return a.path.IsEmpty() }
func (a *scenePathAdapter) Points() []float32 { return a.path.Points() }
func (a *scenePathAdapter) Verbs() []raster.PathVerb {
	sv := a.path.Verbs()
	rv := make([]raster.PathVerb, len(sv))
	for i, v := range sv {
		rv[i] = raster.PathVerb(v)
	}
	return rv
}

// buildEdgesFromScenePath converts a scene.Path to edges in the EdgeBuilder.
func buildEdgesFromScenePath(eb *raster.EdgeBuilder, p *scene.Path) {
	eb.BuildFromPath(&scenePathAdapter{path: p}, raster.IdentityTransform{})
}

// sparseStripsGoldenPath returns path to Vello sparse strips reference images.
// Note: these are from a DIFFERENT rasterizer (vello_common/strip.rs) than what
// velloport ports (vello_shaders/src/cpu/). Expected diff ~2-3% is algorithmic.
func sparseStripsGoldenPath(name string) string {
	return filepath.Join("..", "..", "..", "testdata", "golden", "vello-sparse-strips", name+".png")
}

func loadGolden(t *testing.T, name string) *image.RGBA {
	t.Helper()
	path := sparseStripsGoldenPath(name)
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
	rgba := image.NewRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba
}

func renderWithVelloPort(tc goldenTest) *image.RGBA {
	var lines []LineSoup

	if tc.BuildLines != nil {
		// Direct LineSoup generation — includes ALL lines (horizontal etc.)
		lines = tc.BuildLines()
	} else {
		// EdgeBuilder path — for curves that need flattening
		eb := raster.NewEdgeBuilder(2) // 4x AA
		tc.BuildPath(eb)
		vlines := eb.VelloLines()
		lines = make([]LineSoup, len(vlines))
		for i, vl := range vlines {
			lines[i] = LineSoupFromVelloLine(vl.P0, vl.P1, vl.IsDown)
		}
	}

	// Rasterize
	r := NewRasterizer(tc.Width, tc.Height)
	alphas := r.Rasterize(lines, tc.FillRule)

	// Composite onto white background
	img := image.NewRGBA(image.Rect(0, 0, tc.Width, tc.Height))
	fc := tc.FillColor
	for y := 0; y < tc.Height; y++ {
		for x := 0; x < tc.Width; x++ {
			alpha := alphas[y*tc.Width+x]
			if alpha <= 0 {
				img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
				continue
			}
			if alpha > 1.0 {
				alpha = 1.0
			}
			// Alpha-composite fill color over white background
			a := alpha
			cr := uint8(float32(fc.R)*a + 255*(1-a))
			cg := uint8(float32(fc.G)*a + 255*(1-a))
			cb := uint8(float32(fc.B)*a + 255*(1-a))
			img.Set(x, y, color.RGBA{R: cr, G: cg, B: cb, A: 255})
		}
	}
	return img
}

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

func saveDiffImage(t *testing.T, name string, ours, reference *image.RGBA) {
	t.Helper()
	tmpDir := filepath.Join("..", "..", "..", "tmp")
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

	diffPath := filepath.Join(tmpDir, "velloport_diff_"+name+".png")
	if f, err := os.Create(diffPath); err == nil {
		_ = png.Encode(f, diffImg)
		f.Close()
		t.Logf("Diff image saved: %s", diffPath)
	}

	oursPath := filepath.Join(tmpDir, "velloport_ours_"+name+".png")
	if f, err := os.Create(oursPath); err == nil {
		_ = png.Encode(f, ours)
		f.Close()
		t.Logf("Our output saved: %s", oursPath)
	}
}

func TestVelloPortAgainstUpstream(t *testing.T) {
	tests := goldenTests()

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			reference := loadGolden(t, tc.Name)
			if reference == nil {
				return
			}

			if reference.Bounds().Dx() != tc.Width || reference.Bounds().Dy() != tc.Height {
				t.Fatalf("upstream image %dx%d does not match expected %dx%d",
					reference.Bounds().Dx(), reference.Bounds().Dy(), tc.Width, tc.Height)
			}

			ours := renderWithVelloPort(tc)

			diffPercent, diffCount := compareImages(ours, reference)

			t.Logf("Scene: %s, Size: %dx%d, Diff: %d pixels (%.2f%%)",
				tc.Name, tc.Width, tc.Height, diffCount, diffPercent)

			// Always save diffs for inspection
			saveDiffImage(t, tc.Name, ours, reference)

			// Golden images are from sparse strips (different algorithm).
			// Expected ~2-3% diff is cross-algorithm, not a bug.
			// Goal: < 1% when we get GPU pipeline golden images.
			threshold := 5.0
			if diffPercent > threshold {
				t.Errorf("FAIL: %.2f%% pixel difference exceeds threshold %.2f%%", diffPercent, threshold)
			}
		})
	}
}
