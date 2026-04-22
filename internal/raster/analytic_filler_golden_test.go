// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// tinySkiaColor holds premultiplied RGBA color matching tiny-skia's
// set_color_rgba8(50, 127, 150, 200) after premultiplication.
//
// Straight alpha: R=50, G=127, B=150, A=200
// Premultiplied:  R=50*200/255≈39, G=127*200/255≈99, B=150*200/255≈117, A=200
type tinySkiaColor struct {
	R, G, B, A uint8
}

// premultipliedColor returns the premultiplied RGBA for the tiny-skia test paint.
// tiny-skia set_color_rgba8(50, 127, 150, 200) stores straight alpha internally
// and premultiplies when rasterizing. The golden PNG stores premultiplied values.
// Uses tiny-skia's div255: (a*b + 128) / 255 (NOT truncation).
func premultipliedColor() tinySkiaColor {
	return tinySkiaColor{
		R: div255(50, 200),  // 39
		G: div255(127, 200), // 100
		B: div255(150, 200), // 118
		A: 200,
	}
}

func div255(a, b uint16) uint8 {
	return uint8((a*b + 128) / 255)
}

// renderWithAnalyticFiller rasterizes a path using AnalyticFiller and composites
// the result into a premultiplied RGBA image using the given paint color.
// The image has a transparent background (A=0 where no coverage).
//
// Parameters:
//   - width, height: canvas dimensions
//   - path: the path to fill (implements PathLike)
//   - fillRule: NonZero or EvenOdd
//   - paint: premultiplied RGBA paint color
//   - aaShift: anti-aliasing shift (0=none, 2=4x AA)
func renderWithAnalyticFiller(
	width, height int, //nolint:unparam // generic helper; all current tests use 100x100
	path PathLike,
	fillRule FillRule,
	paint tinySkiaColor,
	aaShift int,
) *image.RGBA {
	// Build edges from path
	eb := NewEdgeBuilder(aaShift)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	// Rasterize coverage
	coverageBuf := make([]uint8, width*height)
	FillToBuffer(eb, width, height, fillRule, coverageBuf)

	// Composite coverage with paint color into premultiplied RGBA image.
	// Each pixel: channel = paintChannel * coverage / 255
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cov := uint16(coverageBuf[y*width+x])
			if cov == 0 {
				continue
			}
			r := div255(uint16(paint.R), cov)
			g := div255(uint16(paint.G), cov)
			b := div255(uint16(paint.B), cov)
			a := div255(uint16(paint.A), cov)
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
	return img
}

// renderWithAnalyticFillerOnWhite rasterizes a path using AnalyticFiller and
// composites the result onto a WHITE background using source-over blending.
// All output pixels have A=255, making the image lossless through PNG round-trip
// (no un-premultiply/re-premultiply precision loss).
//
// This matches Skia Fiddle golden generation with canvas->clear(SK_ColorWHITE).
//
// Source-over compositing uses Skia's exact formula (SkAlphaMulQ):
//
//	scale = cov + 1                          (SkAlpha255To256)
//	srcR = (paintR * scale) >> 8             (SkAlphaMulQ)
//	srcA = (paintA * scale) >> 8
//	invScale = (255 - srcA) + 1              (SkAlpha255To256)
//	dstR = srcR + (255 * invScale) >> 8      (source-over on white)
//	dstA = 255
func renderWithAnalyticFillerOnWhite(
	width, height int,
	path PathLike,
	fillRule FillRule,
	paint tinySkiaColor,
	aaShift int,
) *image.RGBA {
	eb := NewEdgeBuilder(aaShift)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	coverageBuf := make([]uint8, width*height)
	FillToBuffer(eb, width, height, fillRule, coverageBuf)

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cov := uint32(coverageBuf[y*width+x])
			if cov == 0 {
				img.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
				continue
			}
			// Skia's SkAlphaMulQ: scale = cov + 1, result = (ch * scale) >> 8
			scale := cov + 1
			srcR := (uint32(paint.R) * scale) >> 8
			srcG := (uint32(paint.G) * scale) >> 8
			srcB := (uint32(paint.B) * scale) >> 8
			srcA := (uint32(paint.A) * scale) >> 8

			invScale := (255 - srcA) + 1
			r := uint8(srcR + (255*invScale)>>8)
			g := uint8(srcG + (255*invScale)>>8)
			b := uint8(srcB + (255*invScale)>>8)
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

// loadGoldenPNG loads a golden reference PNG from testdata/golden/.
// Returns nil and calls t.Fatal if the file cannot be loaded.
func loadGoldenPNG(t *testing.T, name string) *image.RGBA {
	t.Helper()

	path := filepath.Join(testdataGoldenDir(), name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open golden image %s: %v", path, err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("failed to decode golden image %s: %v", path, err)
	}

	// If already RGBA, use directly (preserves raw premultiplied bytes).
	// Do NOT use rgba.Set(x,y, img.At(x,y)) — the color.Color interface
	// round-trips through un-premultiply/re-premultiply, losing precision.
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	if nrgba, ok := img.(*image.NRGBA); ok {
		bounds := nrgba.Bounds()
		rgba := image.NewRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				c := nrgba.NRGBAAt(x, y)
				a16 := uint16(c.A)
				rgba.SetRGBA(x, y, color.RGBA{
					R: div255(uint16(c.R), a16),
					G: div255(uint16(c.G), a16),
					B: div255(uint16(c.B), a16),
					A: c.A,
				})
			}
		}
		return rgba
	}
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba
}

// goldenCompareResult holds the results of a pixel-by-pixel image comparison.
type goldenCompareResult struct {
	TotalPixels int         // Total number of pixels compared
	DiffCount   int         // Number of pixels that differ
	MaxDiff     int         // Maximum per-channel difference across all pixels
	DiffPct     float64     // Percentage of differing pixels
	DiffMap     *image.RGBA // Visual diff map (green=match, red=mismatch)
}

// compareImages performs pixel-by-pixel comparison of two RGBA images.
// Returns the comparison result including a visual diff map.
//
// Diff map encoding:
//   - Green channel: match confidence (255 = exact match)
//   - Red channel: mismatch magnitude (brighter = bigger difference)
//   - Alpha: 255 for any pixel where either image has content
func compareImages(got, want *image.RGBA) goldenCompareResult {
	bounds := got.Bounds()
	wantBounds := want.Bounds()

	// Use intersection of bounds for comparison
	w := bounds.Dx()
	h := bounds.Dy()
	if wantBounds.Dx() < w {
		w = wantBounds.Dx()
	}
	if wantBounds.Dy() < h {
		h = wantBounds.Dy()
	}

	result := goldenCompareResult{
		TotalPixels: w * h,
		DiffMap:     image.NewRGBA(image.Rect(0, 0, w, h)),
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gc := got.RGBAAt(x+bounds.Min.X, y+bounds.Min.Y)
			wc := want.RGBAAt(x+wantBounds.Min.X, y+wantBounds.Min.Y)

			gr8 := gc.R
			gg8 := gc.G
			gb8 := gc.B
			ga8 := gc.A
			wr8 := wc.R
			wg8 := wc.G
			wb8 := wc.B
			wa8 := wc.A

			// Per-channel absolute differences
			dR := absDiffU8(gr8, wr8)
			dG := absDiffU8(gg8, wg8)
			dB := absDiffU8(gb8, wb8)
			dA := absDiffU8(ga8, wa8)

			maxChanDiff := maxU8(maxU8(dR, dG), maxU8(dB, dA))

			if maxChanDiff == 0 {
				// Exact match — show green if either image has content
				if ga8 > 0 || wa8 > 0 {
					result.DiffMap.SetRGBA(x, y, color.RGBA{R: 0, G: 128, B: 0, A: 255})
				}
				continue
			}

			result.DiffCount++
			if int(maxChanDiff) > result.MaxDiff {
				result.MaxDiff = int(maxChanDiff)
			}
			// Red channel = mismatch magnitude (scaled to be visible)
			diffVis := maxChanDiff
			if diffVis < 32 {
				diffVis = 32 // minimum visibility for small diffs
			}
			result.DiffMap.SetRGBA(x, y, color.RGBA{R: diffVis, G: 0, B: 0, A: 255})
		}
	}

	if result.TotalPixels > 0 {
		result.DiffPct = float64(result.DiffCount) / float64(result.TotalPixels) * 100.0
	}
	return result
}

// saveDiffMap writes a diff map image to the tmp/ directory for visual inspection.
func saveDiffMap(t *testing.T, img *image.RGBA, name string) {
	t.Helper()

	dir := filepath.Join(projectRoot(), "tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("warning: cannot create tmp dir: %v", err)
		return
	}

	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Logf("warning: cannot create diff image %s: %v", path, err)
		return
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		t.Logf("warning: cannot encode diff image: %v", err)
		return
	}
	t.Logf("diff map saved: %s", path)
}

// saveRendered writes a rendered image to the tmp/ directory for visual inspection.
func saveRendered(t *testing.T, img *image.RGBA, name string) {
	t.Helper()

	dir := filepath.Join(projectRoot(), "tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("warning: cannot create tmp dir: %v", err)
		return
	}

	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Logf("warning: cannot create rendered image %s: %v", path, err)
		return
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		t.Logf("warning: cannot encode rendered image: %v", err)
		return
	}
	t.Logf("rendered image saved: %s", path)
}

// logCompareResult reports comparison statistics using t.Logf (diagnostic, not assertions).
func logCompareResult(t *testing.T, testName string, result goldenCompareResult) {
	t.Helper()
	t.Logf("=== Golden comparison: %s ===", testName)
	t.Logf("  total pixels:    %d", result.TotalPixels)
	t.Logf("  differing pixels: %d (%.2f%%)", result.DiffCount, result.DiffPct)
	t.Logf("  max channel diff: %d", result.MaxDiff)
	if result.DiffCount == 0 {
		t.Logf("  result: EXACT MATCH")
	}
}

// --- Test Functions ---

// TestAnalyticFiller_TinySkiaPolygonGolden is the CRITICAL test for BUG-RAST-011.
//
// This test renders the same open polygon as tiny-skia's fill.rs "open_polygon" test:
// a pentagon with a near-horizontal bottom edge (dy = -0.022). This edge is the key
// stress test for accumulator drift in the AnalyticFiller.
//
// tiny-skia config: anti_alias=false, fill_rule=Winding, color=rgba8(50,127,150,200)
// Canvas: 100x100, transparent background.
func TestAnalyticFiller_TinySkiaPolygonGolden(t *testing.T) {
	// Open polygon (no Close verb) — matches tiny-skia's open_polygon test.
	// The path from tiny-skia tests/integration/fill.rs:
	//   pb.move_to(75.160671, 88.756136);
	//   pb.line_to(24.797274, 88.734053);  // near-horizontal! dy = -0.022
	//   pb.line_to( 9.255130, 40.828792);
	//   pb.line_to(50.012955, 11.243795);
	//   pb.line_to(90.744819, 40.864522);
	// Path is NOT closed — EdgeBuilder auto-closes (last→first).
	path := &testPath{
		verbs: []PathVerb{
			MoveTo,
			LineTo,
			LineTo,
			LineTo,
			LineTo,
		},
		points: []float32{
			75.160671, 88.756136,
			24.797274, 88.734053, // near-horizontal bottom edge
			9.255130, 40.828792,
			50.012955, 11.243795,
			90.744819, 40.864522,
		},
	}

	paint := premultipliedColor()
	got := renderWithAnalyticFiller(100, 100, path, FillRuleNonZero, paint, 0) // aaShift=0: no AA

	golden := loadGoldenPNG(t, "polygon.png")

	result := compareImages(got, golden)
	logCompareResult(t, "polygon (no AA, Winding, open)", result)

	saveRendered(t, got, "golden_rendered_polygon.png")
	saveDiffMap(t, result.DiffMap, "golden_diff_polygon.png")
}

// TestAnalyticFiller_TinySkiaFloatRectAAGolden tests a float-coordinate rectangle
// with AA enabled. This exercises sub-pixel positioning and AA edge coverage.
//
// tiny-skia config: anti_alias=true, color=rgba8(50,127,150,200)
// Rect: (10.3, 15.4) to (90.8, 86.0) — from Rect::from_xywh(10.3, 15.4, 80.5, 70.6)
// Canvas: 100x100, transparent background.
func TestAnalyticFiller_TinySkiaFloatRectAAGolden(t *testing.T) {
	// Build a closed rectangle path matching tiny-skia's Rect::from_xywh(10.3, 15.4, 80.5, 70.6).
	// That produces corners: (10.3, 15.4), (90.8, 15.4), (90.8, 86.0), (10.3, 86.0).
	path := &testPath{
		verbs: []PathVerb{
			MoveTo,
			LineTo,
			LineTo,
			LineTo,
			Close,
		},
		points: []float32{
			10.3, 15.4,
			90.8, 15.4,
			90.8, 86.0,
			10.3, 86.0,
		},
	}

	paint := premultipliedColor()
	got := renderWithAnalyticFiller(100, 100, path, FillRuleNonZero, paint, 2) // aaShift=2: 4x AA

	golden := loadGoldenPNG(t, "float-rect-aa.png")

	result := compareImages(got, golden)
	logCompareResult(t, "float-rect-aa (AA, Winding)", result)

	saveRendered(t, got, "golden_rendered_float_rect_aa.png")
	saveDiffMap(t, result.DiffMap, "golden_diff_float_rect_aa.png")
}

// TestAnalyticFiller_TinySkiaStarAAGolden tests a star polygon with AA and EvenOdd fill.
// The star has various edge angles, exercising the filler across a range of slopes.
//
// tiny-skia config: anti_alias=true, fill_rule=EvenOdd, color=rgba8(50,127,150,200)
// Canvas: 100x100, transparent background.
func TestAnalyticFiller_TinySkiaStarAAGolden(t *testing.T) {
	// Star path from tiny-skia tests/integration/fill.rs:
	//   pb.move_to(50.0,  7.5);
	//   pb.line_to(75.0, 87.5);
	//   pb.line_to(10.0, 37.5);
	//   pb.line_to(90.0, 37.5);
	//   pb.line_to(25.0, 87.5);
	// Path is implicitly closed.
	path := &testPath{
		verbs: []PathVerb{
			MoveTo,
			LineTo,
			LineTo,
			LineTo,
			LineTo,
			Close,
		},
		points: []float32{
			50.0, 7.5,
			75.0, 87.5,
			10.0, 37.5,
			90.0, 37.5,
			25.0, 87.5,
		},
	}

	paint := premultipliedColor()
	got := renderWithAnalyticFiller(100, 100, path, FillRuleEvenOdd, paint, 2) // aaShift=2: 4x AA

	golden := loadGoldenPNG(t, "star-aa.png")

	result := compareImages(got, golden)
	logCompareResult(t, "star-aa (AA, EvenOdd)", result)

	saveRendered(t, got, "golden_rendered_star_aa.png")
	saveDiffMap(t, result.DiffMap, "golden_diff_star_aa.png")
}

// --- Skia AAA Golden Comparison Tests ---
// These compare against golden images generated by Skia's AAA algorithm
// (fiddle.skia.org), which is the target algorithm we ported.

// TestAnalyticFiller_SkiaAAAPolygonGolden compares against Skia AAA output
// for the polygon test case (no AA, Winding fill).
// Uses white background to eliminate PNG premultiply round-trip precision loss.
func TestAnalyticFiller_SkiaAAAPolygonGolden(t *testing.T) {
	path := &testPath{
		verbs: []PathVerb{
			MoveTo,
			LineTo,
			LineTo,
			LineTo,
			LineTo,
		},
		points: []float32{
			75.160671, 88.756136,
			24.797274, 88.734053,
			9.255130, 40.828792,
			50.012955, 11.243795,
			90.744819, 40.864522,
		},
	}

	paint := premultipliedColor()
	got := renderWithAnalyticFillerOnWhite(100, 100, path, FillRuleNonZero, paint, 0)

	golden := loadGoldenPNG(t, "skia-aaa-polygon-white.png")

	result := compareImages(got, golden)
	logCompareResult(t, "skia-aaa-polygon (no AA, Winding, white bg)", result)

	saveRendered(t, got, "golden_rendered_skia_aaa_polygon.png")
	saveDiffMap(t, result.DiffMap, "golden_diff_skia_aaa_polygon.png")
}

// TestAnalyticFiller_SkiaAAAFloatRectGolden compares against Skia AAA output
// for the float-coordinate rectangle with AA.
// Uses white background to eliminate PNG premultiply round-trip precision loss.
func TestAnalyticFiller_SkiaAAAFloatRectGolden(t *testing.T) {
	path := &testPath{
		verbs: []PathVerb{
			MoveTo,
			LineTo,
			LineTo,
			LineTo,
			Close,
		},
		points: []float32{
			10.3, 15.4,
			90.8, 15.4,
			90.8, 86.0,
			10.3, 86.0,
		},
	}

	paint := premultipliedColor()
	got := renderWithAnalyticFillerOnWhite(100, 100, path, FillRuleNonZero, paint, 2)

	golden := loadGoldenPNG(t, "skia-aaa-float-rect-aa-white.png")

	result := compareImages(got, golden)
	logCompareResult(t, "skia-aaa-float-rect-aa (AA, Winding, white bg)", result)

	saveRendered(t, got, "golden_rendered_skia_aaa_float_rect.png")
	saveDiffMap(t, result.DiffMap, "golden_diff_skia_aaa_float_rect.png")

	// Golden was generated via Skia Fiddle drawPath, which dispatches rects to
	// aaa_walk_convex_edges with fixed_to_alpha (=(255*f+32768)>>16). Our walker
	// always uses blit_trapezoid_row with trapezoid_to_alpha (=area>>8). These
	// differ by 1 alpha for ~25% of area values (Skia source lines 535-575).
	// Max diff=1 is expected; diff=0 would require rect-specific code path.
	if result.MaxDiff > 1 {
		t.Errorf("REGRESSION: float rect max diff=%d, want <= 1", result.MaxDiff)
	}
	if result.DiffCount > 140 {
		t.Errorf("REGRESSION: float rect diff pixels=%d, want <= 140", result.DiffCount)
	}
}

// TestAnalyticFiller_SkiaAAAStarGolden compares against Skia AAA output
// for the star with AA. NOTE: Skia golden uses Winding fill (not EvenOdd).
// Uses white background to eliminate PNG premultiply round-trip precision loss.
func TestAnalyticFiller_SkiaAAAStarGolden(t *testing.T) {
	path := &testPath{
		verbs: []PathVerb{
			MoveTo,
			LineTo,
			LineTo,
			LineTo,
			LineTo,
			Close,
		},
		points: []float32{
			50.0, 7.5,
			75.0, 87.5,
			10.0, 37.5,
			90.0, 37.5,
			25.0, 87.5,
		},
	}

	paint := premultipliedColor()
	// Skia AAA star golden uses Winding fill, not EvenOdd
	got := renderWithAnalyticFillerOnWhite(100, 100, path, FillRuleNonZero, paint, 2)

	golden := loadGoldenPNG(t, "skia-aaa-star-aa-white.png")

	result := compareImages(got, golden)
	logCompareResult(t, "skia-aaa-star-aa (AA, Winding, white bg)", result)

	saveRendered(t, got, "golden_rendered_skia_aaa_star.png")
	saveDiffMap(t, result.DiffMap, "golden_diff_skia_aaa_star.png")

	// Coverage is pixel-perfect (diff=0 vs Skia-exact C++ walker, see StarCoverageVsCpp).
	// RGB diff=58 is from Skia blitter-level optimizations NOT in our rasterizer:
	//   56px: dual compositing formula (SkBlendARGB32 vs SkAlphaMulQ, diff=1)
	//   2px:  snapAlpha (coverage 248-255 snapped to 255, diff=2)
	// These are compositing-layer concerns, not rasterization errors.
	if result.MaxDiff > 2 {
		t.Errorf("REGRESSION: star max diff=%d, want <= 2", result.MaxDiff)
	}
	if result.DiffCount > 58 {
		t.Errorf("REGRESSION: star diff pixels=%d, want <= 58", result.DiffCount)
	}
}

func TestAnalyticFiller_StarCoverageDiag(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)
	t.Logf("Star interior (50,30): cov=%d (want 255)", buf[30*100+50])
	t.Logf("Star interior (50,50): cov=%d (want 255)", buf[50*100+50])
	t.Logf("Star interior (40,40): cov=%d (want 255)", buf[40*100+40])

	// Crossing vertex pixels — these are where diff=1/2 remains
	// C++ ground truth: pixel 34 at y=56 with 4 sub-strips = 128, golden = 129
	t.Logf("Star vertex (34,56): cov=%d (C++ 4-sub=128, golden=129)", buf[56*100+34])
	t.Logf("Star vertex (65,56): cov=%d (C++ 4-sub=128, golden=129)", buf[56*100+65])
	t.Logf("Star vertex (31,68): cov=%d (golden wants ~252)", buf[68*100+31])
	t.Logf("Star vertex (68,68): cov=%d (golden wants ~252)", buf[68*100+68])
	// Non-crossing edge pixels
	t.Logf("Star edge (49,7): cov=%d", buf[7*100+49])
	t.Logf("Star edge (10,37): cov=%d (golden wants specific value)", buf[37*100+10])
	t.Logf("Star edge (49,7): cov=%d", buf[7*100+49])
	t.Logf("Star edge (49,8): cov=%d", buf[8*100+49])
}

// TestAnalyticFiller_StarY56Debug traces the full sub-strip processing at y=56
// to diagnose the 2-pixel outlier (diff=31) at (34,56) and (65,56).
//
// At y=56, the star has 4 edges crossing in close pairs:
//   - e4: x≈34.69 (w=-1) and e1: x≈34.70 (w=-1) — within 0.01px
//   - e3: x≈65.30 (w=+1) and e0: x≈65.31 (w=+1) — within 0.01px
//
// With NonZero fill, paired walk produces ONE trapezoid from e4(34.69) to e0(65.31).
// Our coverage at pixel 34 = 80, Skia gives 129. Delta = 49 coverage units.
func TestAnalyticFiller_StarY56Debug(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}

	const aaShift = 2
	eb := NewEdgeBuilder(aaShift)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	filler := NewAnalyticFiller(100, 100)

	sortedBuf := eb.sortedEdgesSlice()
	filler.edgeBuf = make([]CurveEdgeVariant, len(sortedBuf))
	for i := range sortedBuf {
		filler.edgeBuf[i] = sortedBuf[i].variant
	}
	filler.edgeIdx = 0
	// Initialize persistent edge states (required by processScanlineAAA).
	filler.edgeStates = make([]edgeYState, len(filler.edgeBuf))

	//nolint:gosec // aaShift bounded
	aaScale := int32(1) << aaShift

	// Process scanlines 0..55 to advance AET to y=56
	for y := 0; y < 56; y++ {
		filler.processScanlineAAA(y, aaScale, filler.edgeBuf, FillRuleNonZero,
			func(_ int, _ *AlphaRuns) {})
	}

	// Now process y=56 and inspect everything
	t.Logf("=== Processing y=56 ===")
	t.Logf("AET has %d edges before processing", filler.aet.Len())

	// Clear coverage
	for i := range filler.coverage {
		filler.coverage[i] = 0
	}

	ySubpixel := int32(56) * aaScale
	ySubpixelNext := ySubpixel + aaScale
	filler.aet.RemoveExpiredSubpixel(ySubpixel)

	for filler.edgeIdx < len(filler.edgeBuf) {
		edge := filler.edgeBuf[filler.edgeIdx]
		topY := edge.TopY()
		if topY >= ySubpixelNext {
			break
		}
		filler.aet.Insert(edge)
		filler.edgeIdx++
	}

	t.Logf("AET has %d edges after insertion", filler.aet.Len())

	// Log all AET edges
	for i := 0; i < filler.aet.Len(); i++ {
		edge := filler.aet.EdgeAt(i)
		line := edge.AsLine()
		if line == nil {
			continue
		}
		t.Logf("  AET[%d]: type=%d winding=%+d firstY=%d lastY=%d X=%d DX=%d upperY=%d lowerY=%d",
			i, edge.Type, line.Winding, line.FirstY, line.LastY, line.X, line.DX,
			line.UpperY, line.LowerY)
	}

	// Collect sub-strip boundaries
	yFixed := intToSkFixed(56)
	yFixedEnd := intToSkFixed(57)
	stripYs := filler.collectStripBoundariesFixed(yFixed, yFixedEnd, aaScale)

	t.Logf("Sub-strip boundaries (SkFixed): %v", stripYs)
	for i, sy := range stripYs {
		t.Logf("  stripY[%d] = %d (%.6f pixels)", i, sy, float64(sy)/65536.0)
	}

	// Process each sub-strip with tracing
	for si := 0; si < len(stripYs)-1; si++ {
		stripTop := stripYs[si]
		stripBot := stripYs[si+1]
		if stripBot <= stripTop {
			continue
		}

		yDiff := stripBot - stripTop
		fullAlpha := fixedToAlpha(yDiff)
		t.Logf("\n--- Sub-strip %d: [%d, %d) = [%.4f, %.4f) fullAlpha=%d ---",
			si, stripTop, stripBot, float64(stripTop)/65536.0, float64(stripBot)/65536.0, fullAlpha)

		if fullAlpha == 0 {
			continue
		}

		// Resolve edges
		n := filler.aet.Len()
		resolved := make([]edgeLineState, 0, n)
		for i := 0; i < n; i++ {
			edge := filler.aet.EdgeAt(i)
			ls := filler.resolveEdgeLineFixed(edge, aaScale, stripTop, stripBot, fullAlpha)
			if ls.valid {
				resolved = append(resolved, ls)
			}
		}

		sortEdgesByTopX(resolved)

		for j, ls := range resolved {
			midX := (int64(ls.topX) + int64(ls.botX)) / 2
			t.Logf("  edge[%d]: topX=%d(%.4f) botX=%d(%.4f) midX=%.4f winding=%+d dy=%d fullAlpha=%d",
				j, ls.topX, float64(ls.topX)/65536.0,
				ls.botX, float64(ls.botX)/65536.0,
				float64(midX)/65536.0,
				ls.winding, ls.dy, ls.fullAlpha)
		}

		// Paired walk
		winding := int32(0)
		inInterval := false
		var leftLS edgeLineState

		for j := range resolved {
			ls := resolved[j]
			winding += int32(ls.winding)
			prevIn := inInterval
			inInterval = winding != 0

			isLeft := inInterval && !prevIn
			isRight := !inInterval && prevIn

			if isRight {
				t.Logf("  TRAPEZOID: left=(%.4f,%.4f) right=(%.4f,%.4f) alpha=%d",
					float64(leftLS.topX)/65536.0, float64(leftLS.botX)/65536.0,
					float64(ls.topX)/65536.0, float64(ls.botX)/65536.0,
					min(leftLS.fullAlpha, ls.fullAlpha))
			}
			if isLeft {
				leftLS = ls
			}
		}
	}

	// Now actually process it and check coverage
	for i := range filler.coverage {
		filler.coverage[i] = 0
	}
	for si := 0; si < len(stripYs)-1; si++ {
		stripTop := stripYs[si]
		stripBot := stripYs[si+1]
		if stripBot <= stripTop {
			continue
		}
		filler.processSubStripFixed(aaScale, stripTop, stripBot, FillRuleNonZero)
	}

	t.Logf("\n=== Final coverage at key pixels ===")
	for _, x := range []int{33, 34, 35, 36, 64, 65, 66, 67} {
		t.Logf("  cov[%d] = %d", x, filler.coverage[x])
	}

	// Load golden and compare
	golden := loadGoldenPNG(t, "skia-aaa-star-aa-white.png")
	paint := premultipliedColor()
	for _, x := range []int{34, 65} {
		cov := filler.coverage[x]
		wc := golden.RGBAAt(x, 56)

		// Compute what color our coverage produces
		scale := uint32(cov) + 1
		srcR := (uint32(paint.R) * scale) >> 8
		srcA := (200 * scale) >> 8
		invScale := (255 - srcA) + 1
		gotR := uint8(srcR + (255*invScale)>>8)

		t.Logf("  pixel(%d,56): cov=%d → R=%d, golden R=%d (diff=%d)",
			x, cov, gotR, wc.R, absDiffU8(gotR, wc.R))
	}
}

func TestAnalyticFiller_StarY68Debug(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}
	const aaShift = 2
	eb := NewEdgeBuilder(aaShift)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	filler := NewAnalyticFiller(100, 100)
	sortedBuf := eb.sortedEdgesSlice()
	filler.edgeBuf = make([]CurveEdgeVariant, len(sortedBuf))
	for i := range sortedBuf {
		filler.edgeBuf[i] = sortedBuf[i].variant
	}
	// Initialize persistent edge states (required by processScanlineAAA).
	filler.edgeStates = make([]edgeYState, len(filler.edgeBuf))
	//nolint:gosec // bounded
	aaScale := int32(1) << aaShift

	// Process up to y=68
	for y := 0; y < 68; y++ {
		filler.processScanlineAAA(y, aaScale, filler.edgeBuf, FillRuleNonZero,
			func(_ int, _ *AlphaRuns) {})
	}

	// Collect strip boundaries at y=68
	yFixed := intToSkFixed(68)
	yFixedEnd := intToSkFixed(69)
	stripYs := filler.collectStripBoundariesFixed(yFixed, yFixedEnd, aaScale)
	t.Logf("Y=68 sub-strip boundaries: %v", stripYs)
	for i, sy := range stripYs {
		t.Logf("  stripY[%d] = %d (%.4f pixels)", i, sy, float64(sy)/65536.0)
	}

	// Process y=68 and check coverage
	for i := range filler.coverage {
		filler.coverage[i] = 0
	}
	filler.processScanlineAAA(68, aaScale, filler.edgeBuf, FillRuleNonZero,
		func(_ int, _ *AlphaRuns) {})

	t.Logf("Coverage at problem pixels:")
	for _, x := range []int{30, 31, 32, 49, 50, 51, 68, 69, 70} {
		t.Logf("  cov[%d] = %d", x, filler.coverage[x])
	}
}

func TestAnalyticFiller_CoverageDiag(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{10.3, 15.4, 90.8, 15.4, 90.8, 86.0, 10.3, 86.0},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)

	t.Logf("Interior (50,50): cov=%d (want 255)", buf[50*100+50])
	t.Logf("Interior (50,40): cov=%d (want 255)", buf[40*100+50])
	t.Logf("Top edge (11,15): cov=%d (want ~153 for y=15.4, h=0.6)", buf[15*100+11])
	t.Logf("Top edge (11,16): cov=%d (want 255)", buf[16*100+11])
	t.Logf("Left edge (10,50): cov=%d (want ~179 for x=10.3, w=0.7)", buf[50*100+10])
	t.Logf("Right edge (90,50): cov=%d (want ~204 for x=90.8, w=0.8)", buf[50*100+90])
	t.Logf("Corner (10,15): cov=%d", buf[15*100+10])
}

// --- Utility functions ---

func absDiffU8(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func maxU8(a, b uint8) uint8 {
	if a > b {
		return a
	}
	return b
}

// thisFileDir returns the directory containing this test file via runtime.Caller.
func thisFileDir() string {
	//nolint:dogsled // runtime.Caller returns 4 values; we only need the filename
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}

// testdataGoldenDir returns the path to testdata/golden/ relative to this test file.
func testdataGoldenDir() string {
	return filepath.Join(thisFileDir(), "testdata", "golden")
}

// projectRoot returns the project root (gg/) by walking up from this test file.
func projectRoot() string {
	// This file is at internal/raster/analytic_filler_golden_test.go
	// Project root is 2 levels up.
	return filepath.Join(thisFileDir(), "..", "..")
}

// reportPixelSamples logs a few sample pixels from both images for quick comparison.
// Useful for understanding the nature of differences when debugging.
func reportPixelSamples(t *testing.T, got, want *image.RGBA, samplePoints [][2]int) {
	t.Helper()
	for _, pt := range samplePoints {
		x, y := pt[0], pt[1]
		gc := got.RGBAAt(x, y)
		wc := want.RGBAAt(x, y)
		t.Logf("  pixel(%d,%d): got=(%3d,%3d,%3d,%3d) want=(%3d,%3d,%3d,%3d)",
			x, y,
			gc.R, gc.G, gc.B, gc.A,
			wc.R, wc.G, wc.B, wc.A,
		)
	}
}

// TestAnalyticFiller_TinySkiaPolygonGoldenSamples extends the polygon test with
// sample pixel inspection along the near-horizontal bottom edge where BUG-RAST-011
// causes coverage bleed.
func TestAnalyticFiller_TinySkiaPolygonGoldenSamples(t *testing.T) {
	path := &testPath{
		verbs: []PathVerb{
			MoveTo,
			LineTo,
			LineTo,
			LineTo,
			LineTo,
		},
		points: []float32{
			75.160671, 88.756136,
			24.797274, 88.734053,
			9.255130, 40.828792,
			50.012955, 11.243795,
			90.744819, 40.864522,
		},
	}

	paint := premultipliedColor()
	got := renderWithAnalyticFiller(100, 100, path, FillRuleNonZero, paint, 0)
	golden := loadGoldenPNG(t, "polygon.png")

	// Sample pixels along the near-horizontal bottom edge (y≈88-89)
	// and a row below it (y=90) where coverage should be zero.
	t.Logf("=== Pixel samples along near-horizontal edge (BUG-RAST-011 area) ===")
	samples := [][2]int{
		{25, 87}, {35, 87}, {45, 87}, {55, 87}, {65, 87}, {75, 87}, // just above edge
		{25, 88}, {35, 88}, {45, 88}, {55, 88}, {65, 88}, {75, 88}, // edge scanline
		{25, 89}, {35, 89}, {45, 89}, {55, 89}, {65, 89}, {75, 89}, // just below edge
		{25, 90}, {35, 90}, {45, 90}, {55, 90}, {65, 90}, {75, 90}, // should be clear
		{50, 50}, // interior (sanity check)
	}
	reportPixelSamples(t, got, golden, samples)

	// Count non-zero pixels below y=89 in both images
	gotBelow := 0
	goldenBelow := 0
	for y := 90; y < 100; y++ {
		for x := 0; x < 100; x++ {
			_, _, _, gA := got.At(x, y).RGBA()
			_, _, _, wA := golden.At(x, y).RGBA()
			if gA > 0 {
				gotBelow++
			}
			if wA > 0 {
				goldenBelow++
			}
		}
	}
	t.Logf("  non-zero pixels below y=89: got=%d golden=%d (excess=%d)",
		gotBelow, goldenBelow, gotBelow-goldenBelow)

	// Summarize overall comparison
	result := compareImages(got, golden)
	logCompareResult(t, "polygon samples", result)
	t.Logf("  note: this test is diagnostic — differences indicate BUG-RAST-011")
}

func TestAnalyticFiller_StarPixel12_39(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	buf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)
	cov := buf[39*100+12]
	t.Logf("pixel(12,39): coverage=%d (golden=108)", cov)
	if cov != 108 {
		t.Logf("DIFF: our=%d golden=108 diff=%d", cov, int(cov)-108)
	}
}

func TestAnalyticFiller_TraceY39(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	// Get edges and compute X positions at y=39
	sorted := eb.sortedEdgesSlice()
	aaScale := int32(4)
	yFixed := int32(39) << 16 // y=39 in SkFixed
	yFixedEnd := int32(40) << 16

	t.Logf("Edges active at y=39:")
	for i, se := range sorted {
		e := se.variant
		line := e.AsLine()
		if line == nil {
			continue
		}
		topY := e.TopY()
		botY := e.BottomY()
		ySubTop := int32(39) * aaScale
		ySubBot := int32(40) * aaScale
		if topY > ySubBot || botY < ySubTop {
			continue
		}

		hasPrecise := line.UpperY != 0 || line.LowerY != 0
		if hasPrecise {
			topX, botX := computeEdgeX(line, aaScale, hasPrecise, yFixed, yFixedEnd)
			t.Logf("  edge[%d]: topX=%.6f botX=%.6f UpperX=%d PixelDX=%d UpperY=%.4f w=%+d",
				i, float64(topX)/65536.0, float64(botX)/65536.0,
				line.UpperX, line.PixelDX, float64(line.UpperY)/65536.0, line.Winding)
		}
	}

	// Compare with C++: ul=11.949982 ll=13.249969
	t.Logf("C++ reference: ul=11.949982 ll=13.249969")
}

func TestAnalyticFiller_TraceBlitAaaY39(t *testing.T) {
	// Exact SkFixed values from C++ full_walk trace at y=39:
	// blit_aaa: ul=783154 ur=917504 ll=868350 lr=917504
	ul := int32(783154)
	ur := int32(917504)
	ll := int32(868350)
	lr := int32(917504)

	af := NewAnalyticFiller(100, 100)
	for i := range af.coverage {
		af.coverage[i] = 0
	}

	af.blitAaaTrapezoidRow(ul, ur, ll, lr, 50412, 0x7FFFFFFF, 255)

	t.Logf("pixel 11: alpha=%d (C++=0)", af.coverage[11])
	t.Logf("pixel 12: alpha=%d (C++=108)", af.coverage[12])
	t.Logf("pixel 13: alpha=%d (C++=249)", af.coverage[13])

	if af.coverage[12] != 108 {
		t.Errorf("pixel 12: got=%d want=108 (C++ reference)", af.coverage[12])
	}
}

func TestAnalyticFiller_TraceFullY39(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	// Run filler and capture coverage at y=39
	af := NewAnalyticFiller(100, 100)
	var y39cov [100]uint8
	af.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		if y == 39 {
			for x := 0; x < 100; x++ {
				y39cov[x] = af.coverage[x]
			}
		}
	})

	t.Logf("Full filler pixel 12: cov=%d (want 108)", y39cov[12])
	t.Logf("Full filler pixel 13: cov=%d (want 249)", y39cov[13])
	// Direct call gave 108. Full filler might differ because of edge X accumulation.
}

func TestAnalyticFiller_TraceFxAtY39(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	af := NewAnalyticFiller(100, 100)
	af.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		if y == 39 {
			t.Logf("y=39 coverage[12]=%d (want 108)", af.coverage[12])
			for i, st := range af.edgeStates {
				if st.valid && st.winding != 0 {
					t.Logf("  edgeState[%d]: fX=%d (%.6f) fDX=%d w=%+d",
						i, st.fX, float64(st.fX)/65536.0, st.fDX, st.winding)
				}
			}
			t.Logf("  C++ left edge at y=39: fX=783155 (11.949982)")
		}
	})
}

func TestAnalyticFiller_CountBlitY39(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	af := NewAnalyticFiller(100, 100)
	af.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		if y == 39 {
			// Check how many sub-strips were processed
			yFixed := intToSkFixed(39)
			yFixedEnd := intToSkFixed(40)
			stripYs := af.collectStripBoundariesFixed(yFixed, yFixedEnd, 4)
			t.Logf("y=39: %d sub-strips", len(stripYs)-1)
			for i := 0; i < len(stripYs); i++ {
				t.Logf("  boundary[%d]=%d (%.4f)", i, stripYs[i], float64(stripYs[i])/65536.0)
			}
		}
	})
}

// TestAnalyticFiller_StarEdgesY9 traces which edges our Go code uses at y=9.
func TestAnalyticFiller_StarEdgesY9(t *testing.T) {
	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}
	const aaShift = 2
	eb := NewEdgeBuilder(aaShift)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})

	sorted := eb.sortedEdgesSlice()
	aaScale := int32(1) << aaShift

	t.Logf("Total edges: %d", len(sorted))
	for i, se := range sorted {
		e := se.variant
		line := e.AsLine()
		if line == nil {
			continue
		}
		topYsub := e.TopY()
		botYsub := e.BottomY()
		topYpx := float64(topYsub) / float64(aaScale)
		botYpx := float64(botYsub) / float64(aaScale)
		t.Logf("  edge[%d]: topY=%.2f botY=%.2f w=%+d UpperX=%d(%.4f) UpperY=%d(%.4f) PixelDX=%d(%.6f) PixelDY=%d",
			i, topYpx, botYpx, line.Winding,
			line.UpperX, float64(line.UpperX)/65536,
			line.UpperY, float64(line.UpperY)/65536,
			line.PixelDX, float64(line.PixelDX)/65536,
			line.PixelDY)
	}

	// Check edges active at y=9 (sub-pixel rows 36..39)
	t.Logf("\nEdges active at y=9 (sub-rows %d..%d):", 9*int(aaScale), 10*int(aaScale)-1)
	for i, se := range sorted {
		e := se.variant
		if e.TopY() > 10*aaScale || e.BottomY() < 9*aaScale {
			continue
		}
		line := e.AsLine()
		if line == nil {
			continue
		}
		t.Logf("  ACTIVE edge[%d]: topY=%.2f botY=%.2f w=%+d PixelDX=%d(%.6f)",
			i, float64(e.TopY())/float64(aaScale), float64(e.BottomY())/float64(aaScale),
			line.Winding, line.PixelDX, float64(line.PixelDX)/65536)

		// Compute X at y=9 using our formula
		hasPrecise := line.UpperY != 0 || line.LowerY != 0
		if hasPrecise {
			yFixed := int32(9) << 16
			yFixedEnd := int32(10) << 16
			topX := line.UpperX + skFixedMul(line.PixelDX, yFixed-line.UpperY)
			botX := line.UpperX + skFixedMul(line.PixelDX, yFixedEnd-line.UpperY)
			t.Logf("    from-origin X: topX=%d(%.6f) botX=%d(%.6f)",
				topX, float64(topX)/65536, botX, float64(botX)/65536)
		}
	}
}

// TestAnalyticFiller_StarBlitY9 calls blitTrapezoidRow directly with C++ values
// to isolate whether the issue is in edge X computation or trapezoid alpha.
func TestAnalyticFiller_StarBlitY9(t *testing.T) {
	af := NewAnalyticFiller(100, 100)

	// C++ trace at y=9: ul=3246080 ur=3307520 ll=3225600 lr=3328000 fA=255
	// lDY=209715 rDY=209715
	for i := range af.coverage {
		af.coverage[i] = 0
	}
	af.blitTrapezoidRow(3246080, 3307520, 3225600, 3328000, 209715, 209715, 255)
	t.Logf("With C++ exact values:")
	for x := 48; x <= 52; x++ {
		t.Logf("  pixel %d: cov=%d (C++ want: 49=%d, 50=%d)", x, af.coverage[x], 160, 160)
	}

	// Now with our from-origin values (which should be the same!)
	// Edge1 (left, w=-1): UpperX=3276800, UpperY=491520, PixelDX=-20480
	// At y=9 (589824): topX = 3276800 + SkFixedMul(-20480, 589824-491520)
	//                       = 3276800 + SkFixedMul(-20480, 98304)
	topX_L := int32(3276800) + skFixedMul(-20480, 98304)
	botX_L := int32(3276800) + skFixedMul(-20480, 163840) // y=10 - UpperY
	// Edge0 (right, w=+1): UpperX=3276800, PixelDX=+20480
	topX_R := int32(3276800) + skFixedMul(20480, 98304)
	botX_R := int32(3276800) + skFixedMul(20480, 163840)
	t.Logf("\nFrom-origin values: ul=%d ur=%d ll=%d lr=%d", topX_L, topX_R, botX_L, botX_R)
	t.Logf("  C++ values:       ul=%d ur=%d ll=%d lr=%d", 3246080, 3307520, 3225600, 3328000)

	for i := range af.coverage {
		af.coverage[i] = 0
	}
	af.blitTrapezoidRow(topX_L, topX_R, botX_L, botX_R, 209715, 209715, 255)
	t.Logf("With from-origin values:")
	for x := 48; x <= 52; x++ {
		t.Logf("  pixel %d: cov=%d", x, af.coverage[x])
	}
}

// TestAnalyticFiller_StarCoverageVsCpp compares our coverage buffer byte-for-byte
// against C++ full_walk.exe output (Skia AAA ground truth).
func TestAnalyticFiller_StarCoverageVsCpp(t *testing.T) {
	cppFile := filepath.Join(projectRoot(), "tmp", "skia_coverage", "star_coverage_skia_exact.bin")
	cppData, err := os.ReadFile(cppFile)
	if err != nil {
		t.Skipf("C++ coverage not found: %v (run full_walk.exe first)", err)
	}
	if len(cppData) != 10000 {
		t.Fatalf("unexpected C++ coverage size: %d, want 10000", len(cppData))
	}

	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, LineTo, Close},
		points: []float32{50.0, 7.5, 75.0, 87.5, 10.0, 37.5, 90.0, 37.5, 25.0, 87.5},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	goBuf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, goBuf)

	diffCount := 0
	maxDiff := 0
	type covDiff struct {
		x, y    int
		goCov   int
		cppCov  int
		absDiff int
	}
	var diffs []covDiff

	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			g := int(goBuf[y*100+x])
			c := int(cppData[y*100+x])
			d := g - c
			if d < 0 {
				d = -d
			}
			if d > 0 {
				diffCount++
				if d > maxDiff {
					maxDiff = d
				}
				diffs = append(diffs, covDiff{x, y, g, c, d})
			}
		}
	}

	t.Logf("Coverage comparison Go vs C++: %d diff pixels, max diff=%d", diffCount, maxDiff)

	shown := 0
	for _, d := range diffs {
		if shown >= 50 {
			break
		}
		t.Logf("  (%2d,%2d): go=%3d cpp=%3d diff=%+d", d.x, d.y, d.goCov, d.cppCov, d.goCov-d.cppCov)
		shown++
	}
	if len(diffs) > 50 {
		t.Logf("  ... and %d more", len(diffs)-50)
	}

	// Coverage must be pixel-perfect vs Skia-exact C++ walker.
	// C++ tool uses verbatim Skia source (SkScan_AAAPath.cpp aaa_walk_edges).
	if diffCount > 0 {
		t.Errorf("REGRESSION: coverage diff=%d (max=%d), want diff=0 (pixel-perfect)", diffCount, maxDiff)
	}
}

// TestAnalyticFiller_RectCoverageVsCpp compares float rect coverage byte-for-byte
// against C++ Skia-exact tool output.
func TestAnalyticFiller_RectCoverageVsCpp(t *testing.T) {
	cppFile := filepath.Join(projectRoot(), "tmp", "skia_coverage", "rect_coverage_skia_exact.bin")
	cppData, err := os.ReadFile(cppFile)
	if err != nil {
		t.Skipf("C++ rect coverage not found: %v (run skia_exact.exe first)", err)
	}
	if len(cppData) != 10000 {
		t.Fatalf("unexpected C++ coverage size: %d, want 10000", len(cppData))
	}

	path := &testPath{
		verbs:  []PathVerb{MoveTo, LineTo, LineTo, LineTo, Close},
		points: []float32{10.3, 15.4, 90.8, 15.4, 90.8, 86.0, 10.3, 86.0},
	}
	eb := NewEdgeBuilder(2)
	eb.SetFlattenCurves(true)
	eb.BuildFromPath(path, IdentityTransform{})
	goBuf := make([]uint8, 100*100)
	FillToBuffer(eb, 100, 100, FillRuleNonZero, goBuf)

	diffCount := 0
	maxDiff := 0
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			g := int(goBuf[y*100+x])
			c := int(cppData[y*100+x])
			d := g - c
			if d < 0 {
				d = -d
			}
			if d > 0 {
				diffCount++
				if d > maxDiff {
					maxDiff = d
				}
				if diffCount <= 10 {
					t.Logf("  (%2d,%2d): go=%3d cpp=%3d diff=%+d", x, y, g, c, g-c)
				}
			}
		}
	}

	t.Logf("Rect coverage comparison Go vs C++: %d diff pixels, max diff=%d", diffCount, maxDiff)

	if diffCount > 0 {
		t.Errorf("REGRESSION: rect coverage diff=%d (max=%d), want diff=0", diffCount, maxDiff)
	}
}
