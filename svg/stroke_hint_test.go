package svg

import (
	"image"
	"testing"
)

const toolTerminalSVG = `<svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
<path d="M3 5L7 8L3 11" stroke="#6C707E" stroke-linecap="round" stroke-linejoin="round"/>
<path d="M9 11H13" stroke="#6C707E" stroke-linecap="round"/>
</svg>`

// --- Rendering quality tests ---

// TestStrokeHintCrispness verifies that stroke hinting produces higher peak
// alpha (closer to 255) for thin stroked paths at small sizes.
func TestStrokeHintCrispness(t *testing.T) {
	// Simple horizontal line: M3 8 H13 — stroke-only, width=1, in 16x16 viewBox.
	// Without hinting: if line center is at y=8 (device px boundary), the 1px
	// stroke straddles pixels 7 and 8 with ~50% coverage each.
	// With hinting: snapped to y=8.5, stroke covers pixel 8 at ~100%.
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <path d="M3 8 H13" stroke="black" fill="none"/>
</svg>`

	// Render with hinting (default for 16x16).
	hinted, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render hinted: %v", err)
	}

	// Render without hinting (env var disables it).
	t.Setenv("GOGPU_SVG_NO_HINT", "1")
	unhinted, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render unhinted: %v", err)
	}

	hintedPeak := peakAlpha(hinted)
	unhintedPeak := peakAlpha(unhinted)

	t.Logf("Hinted peak alpha:   %d", hintedPeak)
	t.Logf("Unhinted peak alpha: %d", unhintedPeak)

	// The hinted version should have higher peak alpha because strokes
	// land on pixel centers instead of boundaries.
	if hintedPeak < unhintedPeak {
		t.Errorf("hinted peak alpha (%d) < unhinted peak alpha (%d); hinting should improve crispness",
			hintedPeak, unhintedPeak)
	}

	// The hinted version should have at least one nearly-opaque pixel (>= 200).
	if hintedPeak < 200 {
		t.Errorf("hinted peak alpha = %d, want >= 200 for a 1px stroke on pixel center", hintedPeak)
	}
}

// TestStrokeHintFewerPartialPixels verifies that hinting reduces the number
// of partially-covered pixels for thin strokes.
func TestStrokeHintFewerPartialPixels(t *testing.T) {
	// Horizontal line at y=8 — a pixel boundary in 16x16.
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <path d="M3 8 H13" stroke="black" fill="none"/>
</svg>`

	hinted, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render hinted: %v", err)
	}

	t.Setenv("GOGPU_SVG_NO_HINT", "1")
	unhinted, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render unhinted: %v", err)
	}

	hintedPartial := countPartialPixels(hinted)
	unhintedPartial := countPartialPixels(unhinted)

	t.Logf("Hinted partial pixels:   %d", hintedPartial)
	t.Logf("Unhinted partial pixels: %d", unhintedPartial)

	// Hinting should produce fewer or equal partial-coverage pixels because
	// strokes align to pixel grid, producing more fully-opaque pixels.
	if hintedPartial > unhintedPartial+5 {
		t.Errorf("hinted has more partial pixels (%d) than unhinted (%d); hinting should reduce partial coverage",
			hintedPartial, unhintedPartial)
	}
}

// TestStrokeHintLargeCanvasNoEffect verifies that hinting is NOT applied
// at large canvas sizes (above the 32-physical-pixel policy limit).
func TestStrokeHintLargeCanvasNoEffect(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <path d="M3 8 H13" stroke="black" fill="none"/>
</svg>`

	// Render at 64x64 — above the 48px threshold. Hinting should not activate.
	img1, err := Render([]byte(svg), 64, 64)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	t.Setenv("GOGPU_SVG_NO_HINT", "1")
	img2, err := Render([]byte(svg), 64, 64)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Both should produce identical results since hinting is inactive.
	if !imagesEqual(img1, img2) {
		t.Error("64x64 render differs with/without GOGPU_SVG_NO_HINT; hinting should not affect large canvases")
	}
}

// TestStrokeHintThickStrokeNoEffect verifies that hinting is NOT applied
// to thick strokes (device width > 1.5 pixels).
func TestStrokeHintThickStrokeNoEffect(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <path d="M3 8 H13" stroke="black" stroke-width="3" fill="none"/>
</svg>`

	// stroke-width=3 at 1:1 scale → device width 3 > 1.5 threshold.
	img1, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	t.Setenv("GOGPU_SVG_NO_HINT", "1")
	img2, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if !imagesEqual(img1, img2) {
		t.Error("thick stroke render differs with/without GOGPU_SVG_NO_HINT; hinting should not affect thick strokes")
	}
}

// TestStrokeHintEnvVarDisables verifies the GOGPU_SVG_NO_HINT opt-out.
func TestStrokeHintEnvVarDisables(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <path d="M3 8 H13" stroke="black" fill="none"/>
</svg>`

	// With env var set, stroke hinting should be disabled.
	t.Setenv("GOGPU_SVG_NO_HINT", "1")
	if !strokeHintingDisabled() {
		t.Error("strokeHintingDisabled() should return true when GOGPU_SVG_NO_HINT is set")
	}

	img, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNonEmpty(t, img, "unhinted horizontal line")
}

// TestStrokeHintVerticalLine tests hinting on a vertical line.
func TestStrokeHintVerticalLine(t *testing.T) {
	// Vertical line at x=8 — a pixel boundary. Hinting should snap to x=8.5.
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <path d="M8 3 V13" stroke="black" fill="none"/>
</svg>`

	hinted, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render hinted: %v", err)
	}

	t.Setenv("GOGPU_SVG_NO_HINT", "1")
	unhinted, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render unhinted: %v", err)
	}

	hintedPeak := peakAlpha(hinted)
	unhintedPeak := peakAlpha(unhinted)

	t.Logf("Hinted peak alpha:   %d", hintedPeak)
	t.Logf("Unhinted peak alpha: %d", unhintedPeak)

	if hintedPeak < unhintedPeak {
		t.Errorf("hinted peak (%d) < unhinted peak (%d) for vertical line", hintedPeak, unhintedPeak)
	}
}

// TestStrokeHintLineElement tests hinting on an SVG <line> element.
func TestStrokeHintLineElement(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <line x1="3" y1="8" x2="13" y2="8" stroke="black"/>
</svg>`

	hinted, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render hinted: %v", err)
	}

	t.Setenv("GOGPU_SVG_NO_HINT", "1")
	unhinted, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render unhinted: %v", err)
	}

	hintedPeak := peakAlpha(hinted)
	unhintedPeak := peakAlpha(unhinted)

	t.Logf("Hinted peak alpha:   %d", hintedPeak)
	t.Logf("Unhinted peak alpha: %d", unhintedPeak)

	if hintedPeak < unhintedPeak {
		t.Errorf("hinted peak (%d) < unhinted peak (%d) for <line> element", hintedPeak, unhintedPeak)
	}
	if hintedPeak < 200 {
		t.Errorf("hinted peak alpha = %d, want >= 200 for a hinted 1px horizontal line", hintedPeak)
	}
}

// TestStrokeHintExistingIconsNoRegression verifies that existing icon
// rendering still works correctly with hinting enabled.
func TestStrokeHintExistingIconsNoRegression(t *testing.T) {
	icons := []struct {
		name string
		svg  string
	}{
		{"close", closeIconSVG},
		{"search", searchIconSVG},
		{"refresh", refreshIconSVG},
		{"back", backIconSVG},
		{"execute", executeIconSVG},
		{"commit", commitIconSVG},
		{"problems", problemsIconSVG},
	}
	for _, icon := range icons {
		t.Run(icon.name, func(t *testing.T) {
			for _, sz := range []int{16, 32} {
				img, err := Render([]byte(icon.svg), sz, sz)
				if err != nil {
					t.Errorf("Render %s %dx%d: %v", icon.name, sz, sz, err)
					continue
				}
				assertNonEmpty(t, img, icon.name)
			}
		})
	}
}

// TestStrokeHintTerminalIcon tests the ToolTerminal icon that originally
// motivated this feature. Its strokes should be visibly crisper with hinting.
func TestStrokeHintTerminalIcon(t *testing.T) {
	hinted, err := Render([]byte(toolTerminalSVG), 16, 16)
	if err != nil {
		t.Fatalf("Render hinted: %v", err)
	}

	t.Setenv("GOGPU_SVG_NO_HINT", "1")
	unhinted, err := Render([]byte(toolTerminalSVG), 16, 16)
	if err != nil {
		t.Fatalf("Render unhinted: %v", err)
	}

	hintedPeak := peakAlpha(hinted)
	unhintedPeak := peakAlpha(unhinted)
	hintedOpaque := countOpaquePixels(hinted)
	unhintedOpaque := countOpaquePixels(unhinted)

	t.Logf("Terminal icon 16x16:")
	t.Logf("  Hinted:   peak=%d, opaque=%d", hintedPeak, hintedOpaque)
	t.Logf("  Unhinted: peak=%d, opaque=%d", unhintedPeak, unhintedOpaque)

	// Hinting should improve at least one metric — peak alpha or opaque count.
	if hintedPeak < unhintedPeak && hintedOpaque < unhintedOpaque {
		t.Errorf("hinting made terminal icon worse: peak %d→%d, opaque %d→%d",
			unhintedPeak, hintedPeak, unhintedOpaque, hintedOpaque)
	}
}

// --- Test helpers ---

// peakAlpha returns the highest alpha value in the image (0-255).
func peakAlpha(img *image.RGBA) uint8 {
	bounds := img.Bounds()
	var peak uint8
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			a8 := uint8(a >> 8)
			if a8 > peak {
				peak = a8
			}
		}
	}
	return peak
}

// countPartialPixels counts pixels with alpha > 0 and < 255.
func countPartialPixels(img *image.RGBA) int {
	bounds := img.Bounds()
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			a8 := a >> 8
			if a8 > 0 && a8 < 255 {
				count++
			}
		}
	}
	return count
}

// countOpaquePixels counts pixels with alpha == 255.
func countOpaquePixels(img *image.RGBA) int {
	bounds := img.Bounds()
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a>>8 == 255 {
				count++
			}
		}
	}
	return count
}

// imagesEqual checks if two images have identical pixel data.
func imagesEqual(a, b *image.RGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}
