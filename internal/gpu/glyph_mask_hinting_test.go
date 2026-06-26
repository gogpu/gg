//go:build !nogpu

package gpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// TestGlyphMaskFullHintingForLatin guards the hinting choice: small axis-aligned
// Latin text uses FULL hinting so stems are grid-fit and crisp. layoutGlyphs
// then places fully-hinted glyphs on integer device pixels (so the grid-fit
// stems are not displaced and faded) using rounded advances (so spacing stays
// even). Skewed text disables hinting.
func TestGlyphMaskFullHintingForLatin(t *testing.T) {
	if h := selectGlyphMaskHinting(13, gg.Identity(), false, 1.0); h != text.HintingFull {
		t.Fatalf("small Latin text hinting = %v, want HintingFull", h)
	}
	if h := selectGlyphMaskHinting(13, gg.Matrix{A: 1, B: 0.3, D: 0.3, E: 1}, false, 1.0); h != text.HintingNone {
		t.Fatalf("skewed text hinting = %v, want HintingNone", h)
	}
}

// TestGlyphMaskEvenSpacing guards against the two failure modes seen while
// fixing the faded glyph-mask text:
//
//  1. Fully-hinted glyphs must land on integer device pixels (else the grid-fit
//     stems are displaced and render faded). So every quad's left edge is an
//     integer.
//  2. Spacing must stay even: rounding each glyph's ABSOLUTE position
//     independently makes adjacent advances jitter by ±1px and opens visible
//     gaps inside words ("anyway" -> "an yway"). Using rounded ADVANCES instead
//     makes every like-advance identical. The test lays out a word and asserts
//     each glyph-to-glyph advance equals the rounded shaped advance (no jitter).
func TestGlyphMaskEvenSpacing(t *testing.T) {
	face := reproFont(t)
	var glyphs []text.ShapedGlyph
	for g := range face.Glyphs("anyway") {
		glyphs = append(glyphs, text.ShapedGlyph{GID: g.GID, X: g.X, Y: g.Y})
	}
	if len(glyphs) < 3 {
		t.Skip("font produced too few glyphs")
	}

	eng := NewGlyphMaskEngine()
	advances := func(baseX float64) []float64 {
		b, err := eng.LayoutShapedGlyphs(face, glyphs, baseX, 20, gg.RGBA{A: 1}, gg.Identity(), 1.0, false)
		if err != nil {
			t.Fatalf("LayoutShapedGlyphs: %v", err)
		}
		if len(b.Quads) != len(glyphs) {
			t.Skipf("got %d quads for %d glyphs (some empty)", len(b.Quads), len(glyphs))
		}
		out := make([]float64, 0, len(b.Quads))
		for i := range b.Quads {
			// Fully hinted glyphs must land on integer device pixels, or the
			// grid-fit stems are displaced and render faded.
			if d := b.Quads[i].X0 - float32(math.Round(float64(b.Quads[i].X0))); math.Abs(float64(d)) > 0.01 {
				t.Errorf("quad[%d].X0 = %.3f not integer-aligned", i, b.Quads[i].X0)
			}
			if i+1 < len(b.Quads) {
				out = append(out, float64(b.Quads[i+1].X0-b.Quads[i].X0))
			}
		}
		return out
	}

	// Even spacing means the internal advances depend only on the glyphs, not
	// on the word's sub-pixel start position. Rounding absolute positions (the
	// bug) makes them jitter with the base fraction and opens gaps in words;
	// rounding advances makes them identical regardless of base.
	a := advances(100.0)
	for _, base := range []float64{100.25, 100.5, 100.75} {
		b := advances(base)
		for i := range a {
			if math.Abs(a[i]-b[i]) > 0.01 {
				t.Fatalf("advance[%d] changed with sub-pixel base: %.2f at x=100.00 vs %.2f at x=%.2f — spacing jitters with position (opens gaps in words)",
					i, a[i], b[i], base)
			}
		}
	}
}
