package gg

import (
	"math"
	"testing"

	"github.com/gogpu/gg/text"
)

// findTestFont is defined in text_wrap_test.go

// TestRotatedTextNoHorizontalArtifacts is a regression test for issue #148.
// It renders rotated text at various angles and checks that no horizontal
// line artifacts extend far beyond the glyph bounding box.
//
// The artifact pattern: a row of non-white pixels extending from the text
// all the way to the right edge of the image. Normal anti-aliased text
// has localized alpha; artifacts have a continuous streak.
func TestRotatedTextNoHorizontalArtifacts(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	const w, h = 400, 200

	// Angles that triggered #148 (small rotations on curved glyphs).
	angles := []float64{1, 2, 3, 5, 7, 10, 15, 30, 45}

	for _, deg := range angles {
		t.Run(angleName(deg), func(t *testing.T) {
			dc := NewContext(w, h)
			dc.ClearWithColor(White)

			face := source.Face(16)
			dc.SetFont(face)
			dc.SetRGB(0, 0, 0)

			rad := deg * math.Pi / 180.0
			dc.Push()
			dc.RotateAbout(rad, float64(w)/2, float64(h)/2)
			dc.DrawStringAnchored("The quick brown fox", float64(w)/2, float64(h)/2, 0.5, 0.5)
			dc.Pop()

			// Scan for horizontal artifacts: rows with non-white pixels
			// extending far beyond expected text bounds.
			artifactRows := 0
			for y := 0; y < h; y++ {
				// Count longest continuous run of non-white pixels.
				run := 0
				maxRun := 0
				for x := 0; x < w; x++ {
					p := dc.pixmap.GetPixel(x, y)
					if p.R < 0.99 || p.G < 0.99 || p.B < 0.99 {
						run++
						if run > maxRun {
							maxRun = run
						}
					} else {
						run = 0
					}
				}
				// Normal text at 16px with rotation: individual glyphs ~12px wide.
				// A full sentence ~300px. An artifact would be a thin line spanning
				// nearly the full width. We flag runs > 350px as suspicious.
				if maxRun > 350 {
					artifactRows++
					t.Logf("y=%d: continuous non-white run of %d pixels", y, maxRun)
				}
			}

			if artifactRows > 0 {
				t.Errorf("angle=%.0f°: found %d rows with horizontal artifacts (regression of #148)", deg, artifactRows)
			}
		})
	}
}

// TestRotatedTextWithCurvedGlyphs specifically tests glyphs with
// many curves (e, o, b, p, d, g, q) that triggered #148.
func TestRotatedTextWithCurvedGlyphs(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	const w, h = 300, 100

	// These glyphs have counters (inner curves) that were the trigger.
	curvedTexts := []string{"eooo", "bbbppp", "dddggg", "qqq000"}

	for _, txt := range curvedTexts {
		t.Run(txt, func(t *testing.T) {
			dc := NewContext(w, h)
			dc.ClearWithColor(White)

			face := source.Face(16)
			dc.SetFont(face)
			dc.SetRGB(0, 0, 0)

			// 3 degrees — the most problematic angle from #148.
			dc.Push()
			dc.RotateAbout(3*math.Pi/180, float64(w)/2, float64(h)/2)
			dc.DrawStringAnchored(txt, float64(w)/2, float64(h)/2, 0.5, 0.5)
			dc.Pop()

			// Check right edge: should be clean white.
			nonWhiteRight := 0
			for y := 0; y < h; y++ {
				for x := w - 30; x < w; x++ {
					p := dc.pixmap.GetPixel(x, y)
					if p.R < 0.99 || p.G < 0.99 || p.B < 0.99 {
						nonWhiteRight++
					}
				}
			}

			// Right 30 pixels should be clean. At 3° rotation with
			// short text centered, no glyph should reach there.
			if nonWhiteRight > 5 {
				t.Errorf("%q at 3°: %d non-white pixels in right margin (artifact leak)", txt, nonWhiteRight)
			}
		})
	}
}

// TestTabRenderingNoTofu verifies tab characters render as whitespace,
// not as tofu boxes (.notdef rectangles). Regression test for TEXT-008.
func TestTabRenderingNoTofu(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc := NewContext(600, 60)
	dc.ClearWithColor(White)

	face := source.Face(20)
	dc.SetFont(face)
	dc.SetRGB(0, 0, 0)

	// Draw text with tab.
	dc.DrawString("A\tB", 10, 40)

	// The tab region (between A and B) should have no dark pixels.
	// With tofu, the tab position would have a rectangle outline.
	// Measure A advance to find tab region.
	advA := face.Advance("A")
	tabStart := int(10 + advA + 2) // 2px margin
	tabEnd := int(10 + face.Advance("A\t") - 2)

	if tabEnd <= tabStart {
		t.Skip("tab region too small to test")
	}

	darkPixels := 0
	for y := 10; y < 50; y++ {
		for x := tabStart; x < tabEnd; x++ {
			if x >= 0 && x < 600 && y >= 0 && y < 60 {
				p := dc.pixmap.GetPixel(x, y)
				if p.R < 0.5 && p.G < 0.5 && p.B < 0.5 {
					darkPixels++
				}
			}
		}
	}

	if darkPixels > 0 {
		t.Errorf("found %d dark pixels in tab region [%d..%d] — tofu box? (TEXT-008 regression)", darkPixels, tabStart, tabEnd)
	}
}

// TestRotatedTabNoTofu verifies tabs work in outline rendering path too.
func TestRotatedTabNoTofu(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc := NewContext(400, 100)
	dc.ClearWithColor(White)

	face := source.Face(16)
	dc.SetFont(face)
	dc.SetRGB(0, 0, 0)

	// Rotation forces outline path (Tier 2).
	dc.Push()
	dc.RotateAbout(0.3, 200, 50)
	dc.DrawStringAnchored("Col1\tCol2\tCol3", 200, 50, 0.5, 0.5)
	dc.Pop()

	// Count total non-white pixels. With tofu, there would be many extra
	// dark pixels forming rectangles. Without tofu, only glyph outlines.
	nonWhite := 0
	for y := 0; y < 100; y++ {
		for x := 0; x < 400; x++ {
			p := dc.pixmap.GetPixel(x, y)
			if p.R < 0.95 || p.G < 0.95 || p.B < 0.95 {
				nonWhite++
			}
		}
	}

	// Render same text without tabs for comparison.
	dc2 := NewContext(400, 100)
	dc2.ClearWithColor(White)
	dc2.SetFont(face)
	dc2.SetRGB(0, 0, 0)
	dc2.Push()
	dc2.RotateAbout(0.3, 200, 50)
	dc2.DrawStringAnchored("Col1 Col2 Col3", 200, 50, 0.5, 0.5)
	dc2.Pop()

	nonWhiteRef := 0
	for y := 0; y < 100; y++ {
		for x := 0; x < 400; x++ {
			p := dc2.pixmap.GetPixel(x, y)
			if p.R < 0.95 || p.G < 0.95 || p.B < 0.95 {
				nonWhiteRef++
			}
		}
	}

	// Tab version should have similar pixel count to space version.
	// Tofu would add ~200+ pixels per box. Allow 2x ratio as margin.
	ratio := float64(nonWhite) / float64(nonWhiteRef+1)
	t.Logf("tab=%d space=%d ratio=%.2f", nonWhite, nonWhiteRef, ratio)

	if ratio > 2.0 {
		t.Errorf("tab version has %.1fx more pixels than space version — possible tofu boxes", ratio)
	}
}

func angleName(deg float64) string {
	d := int(deg)
	s := ""
	if d < 10 {
		s = "0"
	}
	n := d
	if n == 0 {
		return s + "0deg"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return s + digits + "deg"
}
