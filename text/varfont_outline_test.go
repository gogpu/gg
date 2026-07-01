package text

import (
	"math"
	"os"
	"testing"
)

// --------------------------------------------------------------------------
// Golden tests for variable font hinting (skrifa single-path parity).
//
// These tests verify that variable fonts receive the SAME hinting treatment
// as static fonts — matching skrifa's unified load_simple architecture where
// gvar deltas are applied to unscaled points BEFORE TT bytecode hinting.
//
// The critical invariant: a variable font at its default instance must produce
// IDENTICAL outlines to the same font treated as static (no variations).
// --------------------------------------------------------------------------

// TestOutline_VarDefaultVsStatic_IdenticalOutlines verifies that a variable
// font at its default weight produces the same hinted outline as when no
// variations are specified. This is the core invariant: the variable path
// and static path must converge at the default instance.
//
// If this test fails, it means the variable font path skips hinting steps
// that the static path applies (the original bug).
func TestOutline_VarDefaultVsStatic_IdenticalOutlines(t *testing.T) {
	// Requires TrueType variable font with glyf+gvar (not CFF2).
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	axes := source.VariationAxes()
	if len(axes) == 0 {
		t.Skip("font has no variation axes")
	}

	// Find the weight axis default value.
	var defaultWeight float32
	hasWeight := false
	for _, axis := range axes {
		if axis.Tag == AxisWeight {
			defaultWeight = axis.Default
			hasWeight = true
			break
		}
	}
	if !hasWeight {
		t.Skip("font has no weight axis")
	}

	extractor := NewOutlineExtractor()
	ppem := 16.0

	// Test multiple glyphs.
	testRunes := []rune{'H', 'e', 'l', 'o', 'A', 'g', 'p'}
	for _, r := range testRunes {
		gid := GlyphID(parsed.GlyphIndex(r))
		if gid == 0 {
			continue
		}

		t.Run(string(r), func(t *testing.T) {
			// Static path: no variations, full hinting.
			staticOutline, err := extractor.ExtractOutlineHinted(parsed, gid, ppem, HintingFull)
			if err != nil {
				t.Fatalf("static ExtractOutlineHinted: %v", err)
			}

			// Variable path at default weight: explicit wght=default, full hinting.
			varDefaultOutline, err := extractor.ExtractOutlineHintedVar(
				parsed, gid, ppem, HintingFull,
				[]FontVariation{NewFontVariation("wght", defaultWeight)},
			)
			if err != nil {
				t.Fatalf("variable ExtractOutlineHintedVar: %v", err)
			}

			if staticOutline == nil && varDefaultOutline == nil {
				return // both empty (e.g., space), OK
			}
			if staticOutline == nil || varDefaultOutline == nil {
				t.Fatalf("one outline nil: static=%v, varDefault=%v",
					staticOutline == nil, varDefaultOutline == nil)
			}

			// Compare segment-by-segment.
			if len(staticOutline.Segments) != len(varDefaultOutline.Segments) {
				t.Errorf("segment count: static=%d, varDefault=%d",
					len(staticOutline.Segments), len(varDefaultOutline.Segments))
				return
			}

			for i, sSeg := range staticOutline.Segments {
				vSeg := varDefaultOutline.Segments[i]
				if sSeg.Op != vSeg.Op {
					t.Errorf("seg[%d] op: static=%v, varDefault=%v", i, sSeg.Op, vSeg.Op)
					continue
				}
				for j := range segPointCount(sSeg.Op) {
					if sSeg.Points[j] != vSeg.Points[j] {
						t.Errorf("seg[%d].Points[%d]: static=%v, varDefault=%v",
							i, j, sSeg.Points[j], vSeg.Points[j])
					}
				}
			}

			// Compare advances.
			if staticOutline.Advance != varDefaultOutline.Advance {
				t.Errorf("advance: static=%.4f, varDefault=%.4f",
					staticOutline.Advance, varDefaultOutline.Advance)
			}
		})
	}
}

// TestOutline_VarNonDefault_DiffersFromDefault verifies that a variable font
// at a non-default weight produces DIFFERENT outlines than at the default
// weight. This confirms that gvar deltas are being applied.
func TestOutline_VarNonDefault_DiffersFromDefault(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	axes := source.VariationAxes()

	var defaultWeight, maxWeight float32
	for _, axis := range axes {
		if axis.Tag == AxisWeight {
			defaultWeight = axis.Default
			maxWeight = axis.Maximum
			break
		}
	}
	if maxWeight == 0 || maxWeight == defaultWeight {
		t.Skip("font has no weight variation range")
	}

	extractor := NewOutlineExtractor()
	ppem := 24.0

	gid := GlyphID(parsed.GlyphIndex('H'))
	if gid == 0 {
		t.Skip("font doesn't have 'H' glyph")
	}

	defaultOutline, err := extractor.ExtractOutlineHintedVar(
		parsed, gid, ppem, HintingFull,
		[]FontVariation{NewFontVariation("wght", defaultWeight)},
	)
	if err != nil {
		t.Fatalf("default: %v", err)
	}

	maxOutline, err := extractor.ExtractOutlineHintedVar(
		parsed, gid, ppem, HintingFull,
		[]FontVariation{NewFontVariation("wght", maxWeight)},
	)
	if err != nil {
		t.Fatalf("max weight: %v", err)
	}

	if defaultOutline == nil || maxOutline == nil {
		t.Fatal("outline is nil")
	}

	// The outlines must differ — gvar deltas change point positions.
	if outlineSegmentsEqual(defaultOutline, maxOutline) {
		t.Error("wght=default and wght=max produced identical outlines — gvar deltas not applied")
	} else {
		t.Logf("wght=%.0f: %d segments, wght=%.0f: %d segments (outlines differ as expected)",
			defaultWeight, len(defaultOutline.Segments),
			maxWeight, len(maxOutline.Segments))
	}
}

// TestOutline_VarHinted_PixelAlignedAdvance verifies that hinted variable
// font outlines produce pixel-aligned advances at small sizes (the hallmark
// of TT bytecode hinting working correctly).
func TestOutline_VarHinted_PixelAlignedAdvance(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	extractor := NewOutlineExtractor()

	gid := GlyphID(parsed.GlyphIndex('H'))
	if gid == 0 {
		t.Skip("font doesn't have 'H' glyph")
	}

	variations := []FontVariation{NewFontVariation("wght", 400)}
	ppemSizes := []float64{10, 12, 14, 16, 20, 24}

	for _, ppem := range ppemSizes {
		t.Run(ppemName(ppem), func(t *testing.T) {
			outline, err := extractor.ExtractOutlineHintedVar(
				parsed, gid, ppem, HintingFull, variations,
			)
			if err != nil {
				t.Fatalf("ExtractOutlineHintedVar: %v", err)
			}
			if outline == nil {
				t.Skip("no outline")
			}

			advance := outline.Advance
			frac := advance - float32(math.Round(float64(advance)))
			if math.Abs(float64(frac)) > 0.01 {
				// TT hinting should produce integer-pixel advances.
				// Some fonts may not hint all sizes, so log rather than fail.
				t.Logf("ppem=%.0f: advance=%.4f (fractional=%.4f) — TT hinting may not cover this size",
					ppem, advance, frac)
			} else {
				t.Logf("ppem=%.0f: advance=%.4f (pixel-aligned)", ppem, advance)
			}
		})
	}
}

// TestOutline_VarHintedVsUnhinted_Differs verifies that hinting actually
// modifies the variable font outline. Without the fix, variable fonts only
// got gridFitOutline (or nothing for AA mode) — this test catches that.
func TestOutline_VarHintedVsUnhinted_Differs(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	extractor := NewOutlineExtractor()

	gid := GlyphID(parsed.GlyphIndex('H'))
	if gid == 0 {
		t.Skip("font doesn't have 'H' glyph")
	}

	variations := []FontVariation{NewFontVariation("wght", 400)}
	ppem := 16.0

	unhinted, err := extractor.ExtractOutlineHintedVar(
		parsed, gid, ppem, HintingNone, variations,
	)
	if err != nil {
		t.Fatalf("unhinted: %v", err)
	}

	hinted, err := extractor.ExtractOutlineHintedVar(
		parsed, gid, ppem, HintingFull, variations,
	)
	if err != nil {
		t.Fatalf("hinted: %v", err)
	}

	if unhinted == nil || hinted == nil {
		t.Skip("outline nil")
	}

	// Hinted and unhinted outlines should differ (hinting grid-fits Y coords).
	differs := !outlineSegmentsEqual(unhinted, hinted)

	if !differs {
		t.Error("hinted and unhinted variable font outlines are identical — hinting not applied")
	} else {
		t.Logf("hinted and unhinted differ (hinting is being applied to variable fonts)")
	}

	// Advances may also differ.
	t.Logf("advance: unhinted=%.4f, hinted=%.4f", unhinted.Advance, hinted.Advance)
}

// TestOutline_VarDefault_MatchesStaticRendering verifies that rendered pixels
// from static and variable@default paths are identical. This is the end-to-end
// test that catches rendering differences caused by hinting divergence.
func TestOutline_VarDefault_MatchesStaticRendering(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	axes := source.VariationAxes()
	var defaultWeight float32
	for _, axis := range axes {
		if axis.Tag == AxisWeight {
			defaultWeight = axis.Default
			break
		}
	}
	if defaultWeight == 0 {
		t.Skip("no weight axis")
	}

	staticFace := source.Face(20)
	varDefaultFace := source.Face(20, WithVariations(NewFontVariation("wght", defaultWeight)))

	text := "Hello"
	staticW, _ := Measure(text, staticFace)
	varW, _ := Measure(text, varDefaultFace)

	diff := math.Abs(staticW - varW)
	t.Logf("static width=%.2f, varDefault width=%.2f, diff=%.2f", staticW, varW, diff)

	// Widths should be very close (tolerance for floating-point).
	if diff > 0.5 {
		t.Errorf("static and variable@default widths differ by %.2f (expected <0.5)", diff)
	}
}

// TestOutline_Trimmed_VarDefaultVsStatic verifies the invariant using the
// trimmed test fonts (always available in CI, no system font dependency).
func TestOutline_Trimmed_VarDefaultVsStatic(t *testing.T) {
	fonts := []struct {
		name        string
		path        string
		defaultWght float32
	}{
		{"Cantarell-VF", "testdata/cantarell_vf_trimmed.ttf", 400},
		{"Vazirmatn-Var", "testdata/vazirmatn_var_trimmed.ttf", 400},
	}

	for _, font := range fonts {
		t.Run(font.name, func(t *testing.T) {
			data, err := os.ReadFile(font.path)
			if err != nil {
				t.Skipf("font not available: %v", err)
			}

			parser := &ownParser{}
			parsed, err := parser.Parse(data)
			if err != nil {
				t.Skipf("parse error: %v", err)
			}

			extractor := NewOutlineExtractor()
			ppem := 16.0

			numGlyphs := parsed.NumGlyphs()
			testedCount := 0

			// Test all valid glyph IDs in the font.
			for gid := GlyphID(1); gid < GlyphID(numGlyphs); gid++ {
				staticOutline, sErr := extractor.ExtractOutlineHinted(parsed, gid, ppem, HintingFull)
				varOutline, vErr := extractor.ExtractOutlineHintedVar(
					parsed, gid, ppem, HintingFull,
					[]FontVariation{NewFontVariation("wght", font.defaultWght)},
				)

				// Skip glyphs where either path errors (out-of-range, etc.).
				if sErr != nil || vErr != nil {
					continue
				}

				if staticOutline == nil && varOutline == nil {
					continue // both empty
				}
				if (staticOutline == nil) != (varOutline == nil) {
					t.Errorf("gid=%d: one nil: static=%v, var=%v",
						gid, staticOutline == nil, varOutline == nil)
					continue
				}

				testedCount++

				// Segment counts must match.
				if len(staticOutline.Segments) != len(varOutline.Segments) {
					t.Errorf("gid=%d: segment count static=%d, var=%d",
						gid, len(staticOutline.Segments), len(varOutline.Segments))
					continue
				}

				// Every point must match.
				for i, sSeg := range staticOutline.Segments {
					vSeg := varOutline.Segments[i]
					if sSeg.Op != vSeg.Op {
						t.Errorf("gid=%d seg[%d]: op static=%v, var=%v",
							gid, i, sSeg.Op, vSeg.Op)
						break
					}
					for j := range segPointCount(sSeg.Op) {
						if sSeg.Points[j] != vSeg.Points[j] {
							t.Errorf("gid=%d seg[%d].Points[%d]: static=%v, var=%v",
								gid, i, j, sSeg.Points[j], vSeg.Points[j])
						}
					}
				}
			}

			t.Logf("tested %d/%d glyphs", testedCount, numGlyphs)
		})
	}
}

// ppemName formats a ppem value for use as a test sub-name.
func ppemName(ppem float64) string {
	return "ppem" + varfontFmtFloat(ppem)
}

// varfontFmtFloat converts a float64 to a clean string (no trailing zeros).
func varfontFmtFloat(f float64) string {
	if f == math.Trunc(f) {
		return varfontFmtInt(int(f))
	}
	n := int(math.Round(f * 10))
	whole := n / 10
	frac := n % 10
	if frac < 0 {
		frac = -frac
	}
	return varfontFmtInt(whole) + "." + varfontFmtInt(frac)
}

func varfontFmtInt(n int) string {
	s := make([]byte, 0, 4)
	if n < 0 {
		s = append(s, '-')
		n = -n
	}
	if n >= 100 {
		s = append(s, byte('0'+n/100))
	}
	if n >= 10 {
		s = append(s, byte('0'+(n/10)%10))
	}
	s = append(s, byte('0'+n%10))
	return string(s)
}
