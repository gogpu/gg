package svg

import (
	"image"
	"math"
	"testing"

	"github.com/gogpu/gg"
)

// --- snapViewBoxCoord unit tests ---

func TestSnapViewBoxCoord(t *testing.T) {
	tests := []struct {
		name  string
		v     float64
		scale float64
		want  float64
	}{
		// Scale 1:1 (16x16 viewBox → 16x16 canvas).
		// device = v * 1.0, snapped = floor(v) + 0.5
		{"integer at 1:1", 6.0, 1.0, 6.5},
		{"half at 1:1", 6.5, 1.0, 6.5},      // already at pixel center
		{"frac at 1:1", 6.3, 1.0, 6.5},      // rounds to pixel center
		{"frac high at 1:1", 6.8, 1.0, 6.5}, // floor(6.8)+0.5 = 6.5
		{"zero at 1:1", 0.0, 1.0, 0.5},

		// Scale 2:1 (16x16 viewBox → 32x32 canvas).
		// device = v * 2.0, snapped = floor(v*2) + 0.5
		// viewBox result = snapped / 2.0
		{"integer at 2:1", 6.0, 2.0, 6.25}, // device=12 → 12.5 → vb=6.25
		{"half at 2:1", 6.5, 2.0, 6.75},    // device=13 → 13.5 → vb=6.75
		{"frac at 2:1", 6.3, 2.0, 6.25},    // device=12.6 → 12.5 → vb=6.25
		{"3 at 2:1", 3.0, 2.0, 3.25},       // device=6 → 6.5 → vb=3.25

		// Scale 0.5:1 (32x32 viewBox → 16x16 canvas).
		{"integer at 0.5:1", 6.0, 0.5, 7.0}, // device=3 → 3.5 → vb=7.0
		{"frac at 0.5:1", 5.0, 0.5, 5.0},    // device=2.5 → 2.5 → vb=5.0

		// Zero scale edge case.
		{"zero scale", 6.3, 0.0, 6.3}, // unchanged
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapViewBoxCoord(tt.v, tt.scale)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("snapViewBoxCoord(%v, %v) = %v, want %v", tt.v, tt.scale, got, tt.want)
			}
		})
	}
}

// --- shouldHintStroke unit tests ---

func TestShouldHintStroke(t *testing.T) {
	tests := []struct {
		name        string
		hinting     bool
		strokeWidth float64
		scaleX      float64
		scaleY      float64
		want        bool
	}{
		{"hinting disabled", false, 1.0, 1.0, 1.0, false},
		{"thin stroke 1:1", true, 1.0, 1.0, 1.0, true},
		{"thin stroke 2:1", true, 0.5, 2.0, 2.0, true},   // device width = 1.0
		{"thick stroke 1:1", true, 2.0, 1.0, 1.0, false}, // device width = 2.0 > 1.5
		{"thick stroke 2:1", true, 1.0, 2.0, 2.0, false}, // device width = 2.0 > 1.5
		{"at threshold", true, 1.5, 1.0, 1.0, true},      // device width = 1.5 <= 1.5
		{"just over", true, 1.6, 1.0, 1.0, false},        // device width = 1.6 > 1.5
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &renderState{
				strokeHinting: tt.hinting,
				scaleX:        tt.scaleX,
				scaleY:        tt.scaleY,
			}
			a := &Attrs{StrokeWidth: tt.strokeWidth}
			got := shouldHintStroke(a, state)
			if got != tt.want {
				t.Errorf("shouldHintStroke(width=%v, scale=%v/%v, hinting=%v) = %v, want %v",
					tt.strokeWidth, tt.scaleX, tt.scaleY, tt.hinting, got, tt.want)
			}
		})
	}
}

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
// at large canvas sizes (> strokeHintMaxCanvasSize).
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
// to thick strokes (device width > strokeHintMaxWidth).
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

// TestHintSVGPath tests the path hinting function directly.
func TestHintSVGPath(t *testing.T) {
	// Build a simple path: M3,8 L13,8 (horizontal line at y=8 in viewBox).
	// With 1:1 scale, device coords = viewBox coords.
	// Hinting: M3.5,8.5 L13.5,8.5.
	src, err := parseTestPath("M3 8 L13 8")
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}

	hinted := hintSVGPath(src, 1.0, 1.0)

	// Verify hinted coords.
	coords := hinted.Coords()
	wantCoords := []float64{3.5, 8.5, 13.5, 8.5}
	if len(coords) != len(wantCoords) {
		t.Fatalf("hinted coords len = %d, want %d", len(coords), len(wantCoords))
	}
	for i, want := range wantCoords {
		if math.Abs(coords[i]-want) > 1e-9 {
			t.Errorf("coords[%d] = %v, want %v", i, coords[i], want)
		}
	}
}

// TestHintPoints tests the coordinate pair hinting helper.
func TestHintPoints(t *testing.T) {
	pts := []float64{3.0, 8.0, 13.0, 8.0, 8.0, 3.0}
	hinted := hintPoints(pts, 1.0, 1.0)

	want := []float64{3.5, 8.5, 13.5, 8.5, 8.5, 3.5}
	for i := range want {
		if math.Abs(hinted[i]-want[i]) > 1e-9 {
			t.Errorf("hintPoints[%d] = %v, want %v", i, hinted[i], want[i])
		}
	}

	// Verify original is not modified.
	if pts[0] != 3.0 {
		t.Error("hintPoints modified original slice")
	}
}

// --- Test helpers ---

func parseTestPath(d string) (*gg.Path, error) {
	return gg.ParseSVGPath(d)
}

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
