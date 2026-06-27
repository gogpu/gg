//go:build !nogpu

package gpu

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// glyphMaskTestFont returns a usable TTF face or skips. Mirrors the candidate
// list used by the text package tests.
func glyphMaskTestFont(t *testing.T, size float64) text.Face {
	t.Helper()
	candidates := []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"C:\\Windows\\Fonts\\arial.ttf",
		filepath.Join("testdata", "test.ttf"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			src, err := text.NewFontSourceFromFile(p)
			if err != nil {
				continue
			}
			return src.Face(size)
		}
	}
	t.Skip("no TTF font available")
	return nil
}

// TestGlyphMaskDeviceGridAlignment guards the subpixel-space fix: at
// deviceScale != 1 each glyph mask quad must land on the device pixel grid.
//
// The glyph mask is rasterized at integer device-pixel resolution and the GPU
// samples the atlas with a Nearest filter. If a quad's left edge does not fall
// on a device pixel boundary, nearest sampling drops/duplicates texel columns
// per glyph, so each glyph renders a pixel wider or narrower than its neighbor
// — visible as uneven horizontal spacing (letters overlap/gap). Before the fix
// the subpixel offset was computed in user space; that only aligns the grid at
// integer deviceScale. The real-world trigger is FRACTIONAL deviceScale — e.g.
// a Linux desktop at 125%/150% reports Xft.dpi/96 = 1.25 or 1.5 — where the
// user-space fraction leaves every quad off-grid.
func TestGlyphMaskDeviceGridAlignment(t *testing.T) {
	face := glyphMaskTestFont(t, 16)

	var glyphs []text.ShapedGlyph
	for g := range face.Glyphs("Hamburglyphs") {
		glyphs = append(glyphs, text.ShapedGlyph{GID: g.GID, X: g.X, Y: g.Y})
	}

	// 1.25 and 1.5 are the common Linux fractional-scale values that triggered
	// the original bug; 2.0 covers integer HiDPI.
	for _, deviceScale := range []float64{1.25, 1.5, 2.0} {
		// Non-integer origin exercises the subpixel path.
		const originX = 10.3
		engine := NewGlyphMaskEngine()
		batch, err := engine.LayoutShapedGlyphs(
			face, glyphs, originX, 40, gg.RGBA{A: 1},
			gg.Identity(), deviceScale, false,
		)
		if err != nil {
			t.Fatalf("deviceScale=%g: LayoutShapedGlyphs: %v", deviceScale, err)
		}
		if len(batch.Quads) == 0 {
			t.Fatalf("deviceScale=%g: no quads produced", deviceScale)
		}

		for i, q := range batch.Quads {
			dev := float64(q.X0) * deviceScale
			if off := math.Abs(dev - math.Round(dev)); off > 1e-4 {
				t.Errorf("deviceScale=%g quad %d left edge %.4f -> device %.4f is %.4f off the pixel grid",
					deviceScale, i, q.X0, dev, off)
			}
		}
	}
}
