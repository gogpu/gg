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

	"github.com/gogpu/gg/internal/gpu/tilecompute"
	"github.com/gogpu/gg/internal/raster"
	"github.com/gogpu/gg/scene"
)

// Vello sparse strips golden tests.
//
// These tests compare our TileRasterizer output against reference images
// from Vello's sparse strips CPU rasterizer (vello_common/src/strip.rs).
// Note: sparse strips uses a DIFFERENT algorithm from vello_shaders/src/cpu/
// (which our tilecompute package ports). Higher thresholds are expected.
//
// Reference images: testdata/golden/vello-sparse-strips/
// Source: sparse_strips/vello_sparse_tests/snapshots/

// VelloGoldenTest defines a test case with parameters matching an upstream
// Vello snapshot test exactly.
type VelloGoldenTest struct {
	Name      string     // Test name matching upstream snapshot filename
	Width     int        // Canvas width in pixels
	Height    int        // Canvas height in pixels
	FillColor color.RGBA // Fill color (premultiplied over white bg)
	FillRule  raster.FillRule
	BuildPath func(eb *raster.EdgeBuilder) // Path builder
	Threshold float64                      // Max acceptable different-pixel percentage
}

// VelloUpstreamTests returns test cases matching Vello sparse strip snapshot
// tests. Parameters extracted from:
//
//	sparse_strips/vello_sparse_tests/tests/basic.rs
//
// Known differences (TileRasterizer vs sparse strips):
//   - Circle: ~5% — curve flattening + backdrop bugs
//   - Triangle: ~5% — backdrop overflow (green rectangle artifact)
//   - Star NZ/EO: ~10% — missing horizontal line + backdrop bugs
//
// For more accurate comparison, see tilecompute package (1:1 port of
// vello_shaders/src/cpu/) which achieves 0.9-3.8% against these same images.
func VelloUpstreamTests() []VelloGoldenTest {
	return []VelloGoldenTest{
		{
			Name:      "filled_circle",
			Width:     100,
			Height:    100,
			FillColor: color.RGBA{R: 0, G: 255, B: 0, A: 255}, // css::LIME
			FillRule:  raster.FillRuleNonZero,
			Threshold: 5.0, // Curve flattening tolerance differences (kurbo vs EdgeBuilder)
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
			Threshold: 5.0, // Known: vertex artifact at (5,5) from backdrop/yEdge
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
			Threshold: 10.0, // Known: horizontal band at y=40 from coincident vertex handling
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
			Threshold: 10.0, // Known: horizontal band at y=40 + interior differences
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

// upstreamGoldenDir returns the path to Vello sparse strips reference images.
func upstreamGoldenDir() string {
	return filepath.Join("..", "..", "testdata", "golden", "vello-sparse-strips")
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

			// Always save output + diff for visual inspection
			if os.Getenv("SAVE_DIFFS") == "1" || diffPercent > tc.Threshold {
				saveDiffImage(t, tc.Name, ours, reference)
			}

			if diffPercent > tc.Threshold {
				t.Errorf("FAIL: %.2f%% pixel difference exceeds threshold %.2f%% "+
					"(our TileRasterizer vs Vello upstream reference)", diffPercent, tc.Threshold)
			}
		})
	}
}

// =============================================================================
// Vello Compute Pipeline Golden Tests (GPU vs CPU)
// =============================================================================
//
// These tests compare GPU compute pipeline output (VelloAccelerator) against
// CPU reference output (tilecompute.RasterizeScenePTCL). Both implementations
// share the same algorithm, so pixel-identical results are expected (within
// floating-point rounding tolerance).
//
// Tests are skipped when no GPU is available (e.g., in CI without hardware).

// computeGoldenTest defines a GPU vs CPU golden test case.
type computeGoldenTest struct {
	Name      string
	Width     int
	Height    int
	BgColor   [4]uint8
	Paths     []tilecompute.PathDef
	Threshold float64 // Maximum acceptable percent of different pixels.
}

// computeGoldenTests returns test cases for GPU compute vs CPU comparison.
func computeGoldenTests() []computeGoldenTest {
	return []computeGoldenTest{
		{
			Name:    "compute_green_triangle",
			Width:   100,
			Height:  100,
			BgColor: [4]uint8{255, 255, 255, 255},
			Paths: []tilecompute.PathDef{
				{
					Lines:    computeTriangleLines(5, 5, 95, 50, 5, 95),
					Color:    [4]uint8{0, 255, 0, 255},
					FillRule: tilecompute.FillRuleNonZero,
				},
			},
			Threshold: 0.0, // GPU and CPU must be pixel-identical for straight lines.
		},
		{
			Name:    "compute_blue_square",
			Width:   64,
			Height:  64,
			BgColor: [4]uint8{0, 0, 0, 255},
			Paths: []tilecompute.PathDef{
				{
					Lines:    computeSquareLines(10, 10, 54, 54),
					Color:    [4]uint8{0, 0, 255, 255},
					FillRule: tilecompute.FillRuleNonZero,
				},
			},
			Threshold: 0.0, // Axis-aligned rectangle: exact match expected.
		},
		{
			Name:    "compute_red_circle",
			Width:   100,
			Height:  100,
			BgColor: [4]uint8{255, 255, 255, 255},
			Paths: []tilecompute.PathDef{
				{
					Lines:    tilecompute.FlattenFill(computeCircleCubics(50, 50, 40)),
					Color:    [4]uint8{255, 0, 0, 255},
					FillRule: tilecompute.FillRuleNonZero,
				},
			},
			Threshold: 0.5, // Floating-point rounding differences in curve segments.
		},
		{
			Name:    "compute_star_nonzero",
			Width:   100,
			Height:  100,
			BgColor: [4]uint8{255, 255, 255, 255},
			Paths: []tilecompute.PathDef{
				{
					Lines:    computeStarLines(),
					Color:    [4]uint8{128, 0, 0, 255},
					FillRule: tilecompute.FillRuleNonZero,
				},
			},
			Threshold: 0.5, // Complex self-intersections: minor rounding diffs.
		},
		{
			Name:    "compute_star_evenodd",
			Width:   100,
			Height:  100,
			BgColor: [4]uint8{255, 255, 255, 255},
			Paths: []tilecompute.PathDef{
				{
					Lines:    computeStarLines(),
					Color:    [4]uint8{128, 0, 0, 255},
					FillRule: tilecompute.FillRuleEvenOdd,
				},
			},
			Threshold: 0.5, // Even-odd fill with self-intersections.
		},
		{
			Name:    "compute_multipath",
			Width:   100,
			Height:  100,
			BgColor: [4]uint8{255, 255, 255, 255},
			Paths: []tilecompute.PathDef{
				{
					Lines:    tilecompute.FlattenFill(computeCircleCubics(30, 50, 20)),
					Color:    [4]uint8{255, 0, 0, 255},
					FillRule: tilecompute.FillRuleNonZero,
				},
				{
					Lines:    tilecompute.FlattenFill(computeCircleCubics(70, 50, 20)),
					Color:    [4]uint8{0, 0, 255, 255},
					FillRule: tilecompute.FillRuleNonZero,
				},
			},
			Threshold: 0.5, // Two non-overlapping circles.
		},
		{
			Name:    "compute_overlapping_semitransparent",
			Width:   80,
			Height:  80,
			BgColor: [4]uint8{255, 255, 255, 255},
			Paths: []tilecompute.PathDef{
				{
					Lines:    computeSquareLines(10, 10, 50, 50),
					Color:    [4]uint8{0, 0, 255, 255},
					FillRule: tilecompute.FillRuleNonZero,
				},
				{
					Lines:    tilecompute.FlattenFill(computeCircleCubics(40, 40, 25)),
					Color:    [4]uint8{255, 0, 0, 128}, // 50% opacity red over blue.
					FillRule: tilecompute.FillRuleNonZero,
				},
			},
			Threshold: 1.0, // Blending of semitransparent layers: rounding tolerance.
		},
	}
}

// TestVelloComputeGolden compares GPU compute pipeline output against the CPU
// reference implementation (tilecompute.RasterizeScenePTCL). Both run the same
// 8-stage Vello algorithm, so output should be pixel-identical or very close.
func TestVelloComputeGolden(t *testing.T) {
	// Initialize GPU.
	accel := &VelloAccelerator{}
	if err := accel.initGPU(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer accel.Close()

	if !accel.CanCompute() {
		t.Skip("compute pipeline not available")
	}

	tests := computeGoldenTests()

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			// CPU reference via RasterizeScenePTCL.
			rast := tilecompute.NewRasterizer(tc.Width, tc.Height)
			cpuImg := rast.RasterizeScenePTCL(tc.BgColor, tc.Paths)

			// GPU compute.
			gpuImg, err := accel.RenderSceneCompute(
				tc.Width, tc.Height, tc.BgColor, tc.Paths,
			)
			if err != nil {
				t.Fatalf("GPU render failed: %v", err)
			}

			// Compare.
			diffPercent, diffCount := compareImages(gpuImg, cpuImg)
			t.Logf("GPU vs CPU: %d diff pixels (%.2f%%), threshold: %.2f%%",
				diffCount, diffPercent, tc.Threshold)

			// Save images for inspection.
			saveComputeDiffImage(t, tc.Name, gpuImg, cpuImg)

			if diffPercent > tc.Threshold {
				t.Errorf("GPU-CPU diff %.2f%% exceeds threshold %.2f%%",
					diffPercent, tc.Threshold)
			}
		})
	}
}

// TestVelloComputeSmoke is a minimal smoke test that validates the compute
// pipeline can render a single triangle and produce non-zero output.
func TestVelloComputeSmoke(t *testing.T) {
	accel := &VelloAccelerator{}
	if err := accel.initGPU(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer accel.Close()

	if !accel.CanCompute() {
		t.Skip("compute pipeline not available")
	}

	paths := []tilecompute.PathDef{
		{
			Lines:    computeTriangleLines(2, 2, 18, 10, 2, 18),
			Color:    [4]uint8{0, 255, 0, 255},
			FillRule: tilecompute.FillRuleNonZero,
		},
	}

	gpuImg, err := accel.RenderSceneCompute(20, 20, [4]uint8{255, 255, 255, 255}, paths)
	if err != nil {
		t.Fatalf("GPU render failed: %v", err)
	}

	// Verify the image is not all-white (triangle should have green pixels).
	nonWhiteCount := 0
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			c := gpuImg.RGBAAt(x, y)
			if c.R != 255 || c.G != 255 || c.B != 255 {
				nonWhiteCount++
			}
		}
	}

	t.Logf("Non-white pixels: %d / 400", nonWhiteCount)
	if nonWhiteCount == 0 {
		t.Errorf("smoke test: GPU produced an all-white image; expected green triangle pixels")
	}
}

// =============================================================================
// Compute test helpers
// =============================================================================

// computeTriangleLines creates a triangle as LineSoup segments.
func computeTriangleLines(x0, y0, x1, y1, x2, y2 float32) []tilecompute.LineSoup {
	return []tilecompute.LineSoup{
		{PathIx: 0, P0: [2]float32{x0, y0}, P1: [2]float32{x1, y1}},
		{PathIx: 0, P0: [2]float32{x1, y1}, P1: [2]float32{x2, y2}},
		{PathIx: 0, P0: [2]float32{x2, y2}, P1: [2]float32{x0, y0}},
	}
}

// computeSquareLines creates an axis-aligned rectangle as LineSoup segments.
func computeSquareLines(x0, y0, x1, y1 float32) []tilecompute.LineSoup {
	return []tilecompute.LineSoup{
		{PathIx: 0, P0: [2]float32{x0, y0}, P1: [2]float32{x1, y0}},
		{PathIx: 0, P0: [2]float32{x1, y0}, P1: [2]float32{x1, y1}},
		{PathIx: 0, P0: [2]float32{x1, y1}, P1: [2]float32{x0, y1}},
		{PathIx: 0, P0: [2]float32{x0, y1}, P1: [2]float32{x0, y0}},
	}
}

// computeCircleCubics returns a circle as 4 cubic Bezier curves.
func computeCircleCubics(cx, cy, r float32) []tilecompute.CubicBezier {
	const kappa float32 = 0.5522847498
	k := r * kappa
	return []tilecompute.CubicBezier{
		{P0: [2]float32{cx + r, cy}, P1: [2]float32{cx + r, cy + k}, P2: [2]float32{cx + k, cy + r}, P3: [2]float32{cx, cy + r}},
		{P0: [2]float32{cx, cy + r}, P1: [2]float32{cx - k, cy + r}, P2: [2]float32{cx - r, cy + k}, P3: [2]float32{cx - r, cy}},
		{P0: [2]float32{cx - r, cy}, P1: [2]float32{cx - r, cy - k}, P2: [2]float32{cx - k, cy - r}, P3: [2]float32{cx, cy - r}},
		{P0: [2]float32{cx, cy - r}, P1: [2]float32{cx + k, cy - r}, P2: [2]float32{cx + r, cy - k}, P3: [2]float32{cx + r, cy}},
	}
}

// computeStarLines creates a 5-point star (crossed line star) as LineSoup.
func computeStarLines() []tilecompute.LineSoup {
	vertices := [][2]float32{
		{50, 10}, {75, 90}, {10, 40}, {90, 40}, {25, 90},
	}
	n := len(vertices)
	lines := make([]tilecompute.LineSoup, 0, n)
	for i := 0; i < n; i++ {
		p0 := vertices[i]
		p1 := vertices[(i+1)%n]
		lines = append(lines, tilecompute.LineSoup{
			PathIx: 0,
			P0:     p0,
			P1:     p1,
		})
	}
	return lines
}

// saveComputeDiffImage saves GPU output, CPU reference, and diff images
// to the tmp directory for visual inspection.
func saveComputeDiffImage(t *testing.T, name string, gpuImg, cpuImg *image.RGBA) {
	t.Helper()
	tmpDir := filepath.Join("..", "..", "tmp")
	_ = os.MkdirAll(tmpDir, 0o755)

	// Save GPU output.
	gpuPath := filepath.Join(tmpDir, "compute_gpu_"+name+".png")
	if f, err := os.Create(gpuPath); err == nil {
		_ = png.Encode(f, gpuImg)
		_ = f.Close()
		t.Logf("GPU output saved: %s", gpuPath)
	}

	// Save CPU reference.
	cpuPath := filepath.Join(tmpDir, "compute_cpu_"+name+".png")
	if f, err := os.Create(cpuPath); err == nil {
		_ = png.Encode(f, cpuImg)
		_ = f.Close()
		t.Logf("CPU reference saved: %s", cpuPath)
	}

	// Save diff image.
	diffImg := image.NewRGBA(gpuImg.Bounds())
	for y := gpuImg.Bounds().Min.Y; y < gpuImg.Bounds().Max.Y; y++ {
		for x := gpuImg.Bounds().Min.X; x < gpuImg.Bounds().Max.X; x++ {
			v := gpuImg.RGBAAt(x, y)
			g := cpuImg.RGBAAt(x, y)
			if v.R != g.R || v.G != g.G || v.B != g.B || v.A != g.A {
				diffImg.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else {
				gray := uint8((uint32(v.R) + uint32(v.G) + uint32(v.B)) / 3)
				diffImg.Set(x, y, color.RGBA{R: gray, G: gray, B: gray, A: 255})
			}
		}
	}

	diffPath := filepath.Join(tmpDir, "compute_diff_"+name+".png")
	if f, err := os.Create(diffPath); err == nil {
		_ = png.Encode(f, diffImg)
		_ = f.Close()
		t.Logf("Diff image saved: %s", diffPath)
	}
}
