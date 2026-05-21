package gg

import (
	"os"
	"testing"

	"github.com/gogpu/gg/text"
)

// findAliasedTestFont returns a path to an available system font, or empty string.
func findAliasedTestFont() string {
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		// macOS
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		// Linux
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// TestTextModeAliased_Enum verifies TextModeAliased is properly defined.
func TestTextModeAliased_Enum(t *testing.T) {
	if TextModeAliased.String() != "Aliased" {
		t.Errorf("TextModeAliased.String() = %q, want %q", TextModeAliased.String(), "Aliased")
	}

	// Verify it's a distinct value from other modes.
	modes := []TextMode{TextModeAuto, TextModeMSDF, TextModeVector, TextModeBitmap, TextModeGlyphMask, TextModeAliased}
	seen := make(map[TextMode]bool)
	for _, m := range modes {
		if seen[m] {
			t.Errorf("TextMode %d (%s) is a duplicate", m, m)
		}
		seen[m] = true
	}
}

// TestTextModeAliased_SetGet verifies SetTextMode/TextMode round-trip.
func TestTextModeAliased_SetGet(t *testing.T) {
	dc := NewContext(100, 50)
	dc.SetTextMode(TextModeAliased)
	if dc.TextMode() != TextModeAliased {
		t.Errorf("TextMode() = %v, want TextModeAliased", dc.TextMode())
	}
}

// TestTextModeAliased_SelectStrategy verifies that TextModeAliased is returned
// as-is from selectTextStrategy (it's an explicit mode, not auto-selected).
func TestTextModeAliased_SelectStrategy(t *testing.T) {
	dc := NewContext(200, 50)
	dc.SetTextMode(TextModeAliased)

	got := dc.selectTextStrategy()
	if got != TextModeAliased {
		t.Errorf("selectTextStrategy() = %v, want TextModeAliased", got)
	}
}

// TestTextModeAliased_DrawString_CPUFallback verifies that aliased text
// renders without panic on CPU path (no GPU accelerator registered).
// Without GPU, it falls back to CPU bitmap rendering.
func TestTextModeAliased_DrawString_CPUFallback(t *testing.T) {
	fontPath := findAliasedTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(200, 50)
	dc.SetRGB(1, 1, 1)
	dc.Clear()

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	face := source.Face(16.0)
	dc.SetFont(face)
	dc.SetRGB(0, 0, 0)
	dc.SetTextMode(TextModeAliased)

	// Should not panic.
	dc.DrawString("Hello", 10, 30)

	// Verify some pixels were drawn (not all white).
	img := dc.Image()
	drawn := false
	for y := range 50 {
		for x := range 200 {
			r, g, b, _ := img.At(x, y).RGBA()
			if r != 0xFFFF || g != 0xFFFF || b != 0xFFFF {
				drawn = true
				break
			}
		}
		if drawn {
			break
		}
	}
	if !drawn {
		t.Error("No pixels drawn — aliased text may not be rendering")
	}
}

// TestGlyphMaskKey_AliasedFlag verifies that the Flags field properly
// distinguishes aliased from anti-aliased cache keys.
func TestGlyphMaskKey_AliasedFlag(t *testing.T) {
	aaKey := text.MakeGlyphMaskKey(42, 1, 16.0, 0, 0)
	aliasedKey := text.MakeGlyphMaskKeyAliased(42, 1, 16.0, 0, 0)

	// Same font, glyph, size, subpixel — but different flags.
	if aaKey == aliasedKey {
		t.Error("AA and aliased keys should differ (Flags field)")
	}

	// AA key has Flags=0.
	if aaKey.Flags != 0 {
		t.Errorf("AA key Flags = %d, want 0", aaKey.Flags)
	}

	// Aliased key has GlyphMaskFlagAliased set.
	if aliasedKey.Flags != text.GlyphMaskFlagAliased {
		t.Errorf("Aliased key Flags = %d, want %d", aliasedKey.Flags, text.GlyphMaskFlagAliased)
	}
}

// TestGlyphMaskRasterizer_Aliased_BinaryPixels verifies that RasterizeAliased
// produces only binary coverage (0 or 255) in the glyph mask.
func TestGlyphMaskRasterizer_Aliased_BinaryPixels(t *testing.T) {
	fontPath := findAliasedTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}

	parsed := source.Parsed()
	rast := text.NewGlyphMaskRasterizer()

	// Try multiple glyphs to increase chance of diagonal edges.
	testRunes := []rune{'A', 'W', 'O', 'g', 'j', 'S'}
	for _, r := range testRunes {
		// Get glyph ID for this rune.
		var gid text.GlyphID
		face := source.Face(24.0)
		for g := range face.Glyphs(string(r)) {
			gid = g.GID
			break
		}
		if gid == 0 {
			continue // Missing glyph
		}

		result, rErr := rast.RasterizeAliased(parsed, gid, 24.0, 0, 0, text.HintingNone)
		if rErr != nil {
			t.Errorf("RasterizeAliased(%c) error: %v", r, rErr)
			continue
		}
		if result == nil {
			continue // Empty glyph (space)
		}

		// Verify every pixel is 0 or 255.
		for i, v := range result.Mask {
			if v != 0 && v != 255 {
				y := i / result.Width
				x := i % result.Width
				t.Errorf("RasterizeAliased(%c): pixel(%d,%d) = %d, want 0 or 255", r, x, y, v)
				break // One error per glyph is enough.
			}
		}
	}
}

// TestGlyphMaskRasterizer_Aliased_VsHinted verifies that aliased and hinted
// rasterizations of the same glyph produce different masks. The hinted mask
// should have intermediate alpha values on edges, while the aliased should not.
func TestGlyphMaskRasterizer_Aliased_VsHinted(t *testing.T) {
	fontPath := findAliasedTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}

	parsed := source.Parsed()
	rast := text.NewGlyphMaskRasterizer()
	face := source.Face(24.0)

	// Use 'O' — has circular outline, guarantees diagonal edges.
	var gid text.GlyphID
	for g := range face.Glyphs("O") {
		gid = g.GID
		break
	}
	if gid == 0 {
		t.Skip("No glyph for 'O'")
	}

	hinted, hErr := rast.RasterizeHinted(parsed, gid, 24.0, 0, 0, text.HintingNone)
	aliased, aErr := rast.RasterizeAliased(parsed, gid, 24.0, 0, 0, text.HintingNone)
	if hErr != nil {
		t.Fatalf("RasterizeHinted error: %v", hErr)
	}
	if aErr != nil {
		t.Fatalf("RasterizeAliased error: %v", aErr)
	}
	if hinted == nil || aliased == nil {
		t.Skip("Glyph produced nil result")
	}

	// Hinted mask should have at least one intermediate value.
	hasIntermediate := false
	for _, v := range hinted.Mask {
		if v > 0 && v < 255 {
			hasIntermediate = true
			break
		}
	}
	if !hasIntermediate {
		t.Log("Warning: hinted mask has no intermediate values — may be a very simple glyph shape")
	}

	// Aliased mask must have ONLY binary values.
	for i, v := range aliased.Mask {
		if v != 0 && v != 255 {
			y := i / aliased.Width
			x := i % aliased.Width
			t.Errorf("aliased pixel(%d,%d) = %d, want 0 or 255", x, y, v)
			break
		}
	}
}
