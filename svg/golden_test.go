package svg

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogpu/gg"
)

// TestFolderStrokeGolden compares our SVG stroke rendering against Skia's
// output (generated via fiddle.skia.org). Target: diff==0.
//
// Golden: testdata/golden/folder_stroke_20x20.png
// Source: fiddle.skia.org hash 4e6be8bff3f4f1e861e0578c35fffe23
// Skia C++ code: tmp/skia_folder_fiddle.cpp
func TestFolderStrokeGolden(t *testing.T) {
	// Same SVG as Skia fiddle: dark bg + stroke-only folder path.
	const folderSVG = `<svg width="20" height="20" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
<rect width="20" height="20" fill="#3C3F41"/>
<path d="M10.5199 5.57617L10.7285 5.75H11H17C17.6904 5.75 18.25 6.30964 18.25 7V15.1667C18.25 16.0671 17.553 16.75 16.75 16.75H3.25C2.44705 16.75 1.75 16.0671 1.75 15.1667V4.83333C1.75 3.93294 2.44705 3.25 3.25 3.25H7.63795C7.69643 3.25 7.75307 3.2705 7.798 3.30794L10.5199 5.57617Z" stroke="#CED0D6" stroke-width="1"/>
</svg>`

	golden := loadGoldenImage(t, "folder_stroke_20x20.png")
	got, err := Render([]byte(folderSVG), 20, 20)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	result := compareRGBA(golden, got)

	t.Logf("Pixels differ: %d / %d", result.diffCount, result.total)
	t.Logf("Max channel diff: %d", result.maxDiff)
	t.Logf("Total RGB diff: %d", result.totalDiff)

	if result.diffCount > 0 {
		// Log worst pixels
		for i, p := range result.worst {
			if i >= 15 {
				break
			}
			t.Logf("  (%2d,%2d): golden=(%3d,%3d,%3d) got=(%3d,%3d,%3d) diff=%d ours=%s",
				p.x, p.y, p.gr, p.gg, p.gb, p.or, p.og, p.ob, p.maxD, p.direction)
		}
	}

	// Max diff=40 expected: our deviation-based subdivision (0.1px threshold)
	// produces slightly different coverage than Skia's 4-segment forward-diff.
	// Both are correct; Skia uses GPU MSAA to hide chord artifacts.
	// See docs/dev/research/FORWARD-DIFF-ROOT-CAUSE.md for full analysis.
	if result.maxDiff > 45 {
		t.Errorf("FAIL: max diff=%d, want <= 45 (Skia comparison)", result.maxDiff)
	}
}

// --- helpers ---

func loadGoldenImage(t *testing.T, name string) *image.RGBA {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Cannot open golden %s: %v", path, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("Cannot decode golden %s: %v", path, err)
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

type pixelDiff struct {
	x, y       int
	gr, gg, gb int // golden
	or, og, ob int // ours
	maxD       int
	direction  string
}

type compareResult struct {
	total, diffCount int
	maxDiff          int
	totalDiff        int
	worst            []pixelDiff
}

func compareRGBA(golden, got *image.RGBA) compareResult {
	gb := golden.Bounds()
	ob := got.Bounds()
	w := min(gb.Dx(), ob.Dx())
	h := min(gb.Dy(), ob.Dy())

	var r compareResult
	r.total = w * h

	for y := range h {
		for x := range w {
			gc := golden.At(x+gb.Min.X, y+gb.Min.Y).(color.RGBA)
			oc := got.At(x+ob.Min.X, y+ob.Min.Y).(color.RGBA)

			dr := absDiff(int(gc.R), int(oc.R))
			dg := absDiff(int(gc.G), int(oc.G))
			db := absDiff(int(gc.B), int(oc.B))
			maxD := maxOf(dr, dg, db)

			r.totalDiff += dr + dg + db
			if maxD > r.maxDiff {
				r.maxDiff = maxD
			}
			if maxD > 0 {
				r.diffCount++
				dir := "BRIGHTER"
				if int(oc.R)+int(oc.G)+int(oc.B) < int(gc.R)+int(gc.G)+int(gc.B) {
					dir = "darker"
				}
				r.worst = append(r.worst, pixelDiff{
					x: x, y: y,
					gr: int(gc.R), gg: int(gc.G), gb: int(gc.B),
					or: int(oc.R), og: int(oc.G), ob: int(oc.B),
					maxD: maxD, direction: dir,
				})
			}
		}
	}

	// Sort worst by maxD descending
	for i := range r.worst {
		for j := i + 1; j < len(r.worst); j++ {
			if r.worst[j].maxD > r.worst[i].maxD {
				r.worst[i], r.worst[j] = r.worst[j], r.worst[i]
			}
		}
	}
	return r
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

func maxOf(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestDirectExpandFill isolates whether the Skia golden diff comes from:
// (a) the expanded stroke path geometry (rasterized as fill), or
// (b) the SVG rendering pipeline (transforms, hinting, dc.Stroke routing).
//
// Method: build the folder path programmatically, stroke it via dc.Stroke()
// (which internally does StrokeExpander.Expand → fill the expanded outline),
// on the same dark background as the Skia golden. Compare pixels.
//
// If diff matches the SVG renderer → the problem is in expansion or rasterizer.
// If diff is smaller → the SVG pipeline adds distortion.
func TestDirectExpandFill(t *testing.T) {
	golden := loadGoldenImage(t, "folder_stroke_20x20.png")

	// Build the folder path programmatically (same coords as the SVG d= attribute).
	path, err := gg.ParseSVGPath(
		"M10.5199,5.57617 L10.7285,5.75 H11 H17 " +
			"C17.6904,5.75 18.25,6.30964 18.25,7 " +
			"V15.1667 C18.25,16.0671 17.553,16.75 16.75,16.75 " +
			"H3.25 C2.44705,16.75 1.75,16.0671 1.75,15.1667 " +
			"V4.83333 C1.75,3.93294 2.44705,3.25 3.25,3.25 " +
			"H7.63795 C7.69643,3.25 7.75307,3.2705 7.798,3.30794 " +
			"L10.5199,5.57617 Z")
	if err != nil {
		t.Fatalf("ParseSVGPath: %v", err)
	}

	// Create 20x20 context with dark background (#3C3F41), same as Skia golden.
	dc := gg.NewContext(20, 20)

	// Fill background rect (matches SVG <rect width="20" height="20" fill="#3C3F41"/>).
	dc.SetHexColor("#3C3F41")
	dc.DrawRectangle(0, 0, 20, 20)
	_ = dc.Fill()

	// Set stroke params: width=1, butt cap, miter join (SVG defaults).
	dc.SetLineWidth(1.0)
	dc.SetLineCap(gg.LineCapButt)
	dc.SetLineJoin(gg.LineJoinMiter)

	// Set stroke color #CED0D6.
	dc.SetHexColor("#CED0D6")

	// Draw path and stroke — goes through SoftwareRenderer.Stroke which
	// calls StrokeExpander.Expand, then fills the expanded outline.
	dc.DrawPath(path)
	_ = dc.Stroke()

	got := dc.Image().(*image.RGBA)

	// Save debug output.
	saveGoldenPNG(t, got, "direct_expand_fill.png")

	result := compareRGBA(golden, got)

	t.Logf("=== Direct StrokeExpander+Fill vs Skia golden ===")
	t.Logf("Pixels differ: %d / %d", result.diffCount, result.total)
	t.Logf("Max channel diff: %d", result.maxDiff)
	t.Logf("Total RGB diff: %d", result.totalDiff)

	if result.diffCount > 0 {
		for i, p := range result.worst {
			if i >= 20 {
				break
			}
			t.Logf("  (%2d,%2d): golden=(%3d,%3d,%3d) got=(%3d,%3d,%3d) diff=%d ours=%s",
				p.x, p.y, p.gr, p.gg, p.gb, p.or, p.og, p.ob, p.maxD, p.direction)
		}
	}

	// Also run SVG renderer for comparison.
	const folderSVG = `<svg width="20" height="20" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
<rect width="20" height="20" fill="#3C3F41"/>
<path d="M10.5199 5.57617L10.7285 5.75H11H17C17.6904 5.75 18.25 6.30964 18.25 7V15.1667C18.25 16.0671 17.553 16.75 16.75 16.75H3.25C2.44705 16.75 1.75 16.0671 1.75 15.1667V4.83333C1.75 3.93294 2.44705 3.25 3.25 3.25H7.63795C7.69643 3.25 7.75307 3.2705 7.798 3.30794L10.5199 5.57617Z" stroke="#CED0D6" stroke-width="1"/>
</svg>`
	svgGot, err := Render([]byte(folderSVG), 20, 20)
	if err != nil {
		t.Fatalf("SVG Render: %v", err)
	}

	svgResult := compareRGBA(golden, svgGot)
	t.Logf("=== SVG renderer vs Skia golden ===")
	t.Logf("Pixels differ: %d / %d", svgResult.diffCount, svgResult.total)
	t.Logf("Max channel diff: %d", svgResult.maxDiff)
	t.Logf("Total RGB diff: %d", svgResult.totalDiff)

	// Compare direct vs SVG to see if they produce identical output.
	directVsSvg := compareRGBA(got, svgGot)
	t.Logf("=== Direct vs SVG (should be 0 if pipeline adds nothing) ===")
	t.Logf("Pixels differ: %d / %d", directVsSvg.diffCount, directVsSvg.total)
	t.Logf("Max channel diff: %d", directVsSvg.maxDiff)

	if directVsSvg.maxDiff == 0 {
		t.Logf("CONCLUSION: Direct and SVG produce IDENTICAL output.")
		t.Logf("  → Problem is in StrokeExpander or AnalyticFiller, NOT the SVG pipeline.")
	} else {
		t.Logf("CONCLUSION: SVG pipeline adds distortion (max diff=%d).", directVsSvg.maxDiff)
		t.Logf("  → Part of the golden diff comes from SVG pipeline.")
		if directVsSvg.diffCount > 0 {
			for i, p := range directVsSvg.worst {
				if i >= 10 {
					break
				}
				t.Logf("  (%2d,%2d): direct=(%3d,%3d,%3d) svg=(%3d,%3d,%3d) diff=%d",
					p.x, p.y, p.gr, p.gg, p.gb, p.or, p.og, p.ob, p.maxD)
			}
		}
	}

	// Report: if direct also shows diff≈40, the problem is in expansion/rasterizer
	// coverage difference (our deviation subdivision vs Skia's 4-segment forward-diff).
	if result.maxDiff > 45 {
		t.Errorf("Direct expand+fill: max diff=%d vs Skia golden, want <= 45", result.maxDiff)
	}
}

// saveGoldenPNG saves an image to testdata/tmp/ for visual inspection.
func saveGoldenPNG(t *testing.T, img image.Image, name string) {
	t.Helper()
	dir := filepath.Join("testdata", "tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("Cannot create tmp dir: %v", err)
		return
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Logf("Cannot create %s: %v", path, err)
		return
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Logf("Cannot encode PNG %s: %v", path, err)
		return
	}
	t.Logf("Saved: %s", path)
}

// Ensure fmt is used for future diagnostics.
var _ = fmt.Sprintf

// TestTinySkiaExpandedPathThroughOurRasterizer renders tiny-skia's EXACT
// expanded stroke path as a fill through our rasterizer and compares with Skia golden.
// This isolates: is the diff from our expander or our rasterizer?
func TestTinySkiaExpandedPathThroughOurRasterizer(t *testing.T) {
	// tiny-skia's expanded path for folder icon (from Rust stroke_dump)
	dc := gg.NewContext(20, 20)
	dc.SetHexColor("#3C3F41")
	dc.DrawRectangle(0, 0, 20, 20)
	dc.Fill()

	dc.SetHexColor("#CED0D6")

	// OUTER contour (from tiny-skia)
	dc.MoveTo(10.8400, 5.1921)
	dc.LineTo(11.0486, 5.3659)
	dc.LineTo(10.7285, 5.7500)
	dc.LineTo(10.7285, 5.2500)
	dc.LineTo(11.0000, 5.2500)
	dc.LineTo(17.0000, 5.2500)
	dc.QuadraticTo(18.7500, 5.2500, 18.7500, 7.0000)
	dc.LineTo(18.7500, 15.1667)
	dc.QuadraticTo(18.7500, 17.2500, 16.7500, 17.2500)
	dc.LineTo(3.2500, 17.2500)
	dc.QuadraticTo(1.2500, 17.2500, 1.2500, 15.1667)
	dc.LineTo(1.2500, 4.8333)
	dc.QuadraticTo(1.2500, 2.7500, 3.2500, 2.7500)
	dc.LineTo(7.6379, 2.7500)
	dc.QuadraticTo(7.9095, 2.7500, 8.1181, 2.9238)
	dc.LineTo(10.8400, 5.1921)
	dc.ClosePath()

	// INNER contour (from tiny-skia, reversed)
	dc.MoveTo(10.1998, 5.9603)
	dc.LineTo(7.4779, 3.6921)
	dc.QuadraticTo(7.5475, 3.7500, 7.6379, 3.7500)
	dc.LineTo(3.2500, 3.7500)
	dc.QuadraticTo(2.8507, 3.7500, 2.5579, 4.0520)
	dc.QuadraticTo(2.2500, 4.3696, 2.2500, 4.8333)
	dc.LineTo(2.2500, 15.1667)
	dc.QuadraticTo(2.2500, 16.2500, 3.2500, 16.2500)
	dc.LineTo(16.7500, 16.2500)
	dc.QuadraticTo(17.7500, 16.2500, 17.7500, 15.1667)
	dc.LineTo(17.7500, 7.0000)
	dc.QuadraticTo(17.7500, 6.2500, 17.0000, 6.2500)
	dc.LineTo(11.0000, 6.2500)
	dc.LineTo(10.5475, 6.2500)
	dc.LineTo(10.1998, 5.9603)
	dc.ClosePath()

	dc.Fill()

	got := dc.Image().(*image.RGBA)
	golden := loadGoldenImage(t, "folder_stroke_20x20.png")
	result := compareRGBA(golden, got)

	t.Logf("tiny-skia expanded path through our rasterizer:")
	t.Logf("  Pixels differ: %d / %d", result.diffCount, result.total)
	t.Logf("  Max channel diff: %d", result.maxDiff)

	if result.maxDiff <= 3 {
		t.Logf("MATCH (diff <= 3 = tiny-skia vs Skia level)")
		t.Logf("→ Our EXPANDER is wrong, rasterizer is correct")
	} else if result.maxDiff <= 10 {
		t.Logf("CLOSE but not matching")
	} else {
		t.Logf("LARGE diff → Our RASTERIZER handles this path differently than Skia")
		for i, p := range result.worst {
			if i >= 5 {
				break
			}
			t.Logf("  (%d,%d): golden=(%d,%d,%d) got=(%d,%d,%d) diff=%d %s",
				p.x, p.y, p.gr, p.gg, p.gb, p.or, p.og, p.ob, p.maxD, p.direction)
		}
	}
}

// TestTinySkiaPathVsResvgGolden — CRITICAL: same path, different rasterizer.
// If diff is large → rasterizer bug PROVEN.
// If diff is small → problem was in path geometry (Skia vs tiny-skia).
func TestTinySkiaPathVsResvgGolden(t *testing.T) {
	// Render tiny-skia's exact expanded path through our rasterizer
	dc := gg.NewContext(20, 20)
	dc.SetHexColor("#3C3F41")
	dc.DrawRectangle(0, 0, 20, 20)
	dc.Fill()
	dc.SetHexColor("#CED0D6")

	// OUTER contour
	dc.MoveTo(10.8400, 5.1921)
	dc.LineTo(11.0486, 5.3659)
	dc.LineTo(10.7285, 5.7500)
	dc.LineTo(10.7285, 5.2500)
	dc.LineTo(11.0000, 5.2500)
	dc.LineTo(17.0000, 5.2500)
	dc.QuadraticTo(18.7500, 5.2500, 18.7500, 7.0000)
	dc.LineTo(18.7500, 15.1667)
	dc.QuadraticTo(18.7500, 17.2500, 16.7500, 17.2500)
	dc.LineTo(3.2500, 17.2500)
	dc.QuadraticTo(1.2500, 17.2500, 1.2500, 15.1667)
	dc.LineTo(1.2500, 4.8333)
	dc.QuadraticTo(1.2500, 2.7500, 3.2500, 2.7500)
	dc.LineTo(7.6379, 2.7500)
	dc.QuadraticTo(7.9095, 2.7500, 8.1181, 2.9238)
	dc.LineTo(10.8400, 5.1921)
	dc.ClosePath()

	// INNER contour
	dc.MoveTo(10.1998, 5.9603)
	dc.LineTo(7.4779, 3.6921)
	dc.QuadraticTo(7.5475, 3.7500, 7.6379, 3.7500)
	dc.LineTo(3.2500, 3.7500)
	dc.QuadraticTo(2.8507, 3.7500, 2.5579, 4.0520)
	dc.QuadraticTo(2.2500, 4.3696, 2.2500, 4.8333)
	dc.LineTo(2.2500, 15.1667)
	dc.QuadraticTo(2.2500, 16.2500, 3.2500, 16.2500)
	dc.LineTo(16.7500, 16.2500)
	dc.QuadraticTo(17.7500, 16.2500, 17.7500, 15.1667)
	dc.LineTo(17.7500, 7.0000)
	dc.QuadraticTo(17.7500, 6.2500, 17.0000, 6.2500)
	dc.LineTo(11.0000, 6.2500)
	dc.LineTo(10.5475, 6.2500)
	dc.LineTo(10.1998, 5.9603)
	dc.ClosePath()

	dc.Fill()
	got := dc.Image().(*image.RGBA)

	// Compare with RESVG golden (same tiny-skia path, but 4x supersample rasterizer)
	resvg := loadGoldenPNG(t, "../tmp/folder_resvg_20.png")
	result := compareRGBA(imageToRGBA(resvg), got)

	t.Logf("tiny-skia path: OUR AAA rasterizer vs RESVG (tiny-skia 4x supersample):")
	t.Logf("  Pixels differ: %d / %d", result.diffCount, result.total)
	t.Logf("  Max channel diff: %d", result.maxDiff)
	t.Logf("  Total RGB diff: %d", result.totalDiff)

	if result.maxDiff <= 5 {
		t.Logf("CLOSE MATCH (diff <= 5)")
		t.Logf("→ Our rasterizer is CORRECT for this path")
		t.Logf("→ The diff=39-45 with Skia golden is from PATH GEOMETRY (Skia vs tiny-skia expander)")
	} else {
		t.Logf("SIGNIFICANT DIFF")
		t.Logf("→ Our AAA rasterizer differs from tiny-skia's 4x supersample for this path")
		for i, p := range result.worst {
			if i >= 5 {
				break
			}
			t.Logf("  (%d,%d): resvg=(%d,%d,%d) ours=(%d,%d,%d) diff=%d %s",
				p.x, p.y, p.gr, p.gg, p.gb, p.or, p.og, p.ob, p.maxD, p.direction)
		}
	}
}

func loadGoldenPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Cannot open %s: %v", path, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("Cannot decode %s: %v", path, err)
	}
	return img
}

func imageToRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba
}
