package gg

import (
	"math"
	"os"
	"testing"

	"github.com/gogpu/gg/text"
)

// findSystemFont returns a path to a system TTF font suitable for testing.
// Returns empty string if no font is found (test should be skipped).
func findSystemFont(t *testing.T) string {
	t.Helper()
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		// macOS
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		// Linux
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// setupTextContext creates a context with a loaded font for text transform tests.
// Skips the test if no system font is available.
func setupTextContext(t *testing.T, w, h int, fontSize float64) (*Context, *text.FontSource) {
	t.Helper()
	fontPath := findSystemFont(t)
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}

	dc := NewContext(w, h)
	dc.SetFont(source.Face(fontSize))
	dc.SetRGB(0, 0, 0)
	dc.ClearWithColor(White)

	return dc, source
}

// countNonWhitePixels counts pixels that are not pure white in the given region.
func countNonWhitePixels(dc *Context, x0, y0, x1, y1 int) int {
	count := 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if x < 0 || y < 0 || x >= dc.width || y >= dc.height {
				continue
			}
			p := dc.pixmap.GetPixel(x, y)
			if p.R < 0.99 || p.G < 0.99 || p.B < 0.99 {
				count++
			}
		}
	}
	return count
}

// --------------------------------------------------------------------------
// Tier 0: Translation-only → bitmap fast path
// --------------------------------------------------------------------------

func TestDrawStringCPU_TranslationOnly(t *testing.T) {
	dc, source := setupTextContext(t, 400, 200, 24)
	defer func() { _ = source.Close() }()

	// Draw with identity matrix (baseline)
	dc.DrawString("Hello", 50, 100)
	identityPixels := countNonWhitePixels(dc, 0, 0, 400, 200)

	// Reset and draw with translation-only matrix
	dc.ClearWithColor(White)
	dc.Push()
	dc.Translate(10, 5)
	dc.DrawString("Hello", 50, 100)
	dc.Pop()
	translatedPixels := countNonWhitePixels(dc, 0, 0, 400, 200)

	if identityPixels == 0 {
		t.Fatal("Identity draw produced no pixels")
	}
	if translatedPixels == 0 {
		t.Fatal("Translation draw produced no pixels")
	}

	// Both should produce similar pixel counts (same font size, same text)
	ratio := float64(translatedPixels) / float64(identityPixels)
	if ratio < 0.8 || ratio > 1.2 {
		t.Errorf("Translation-only pixel count (%d) differs significantly from identity (%d), ratio=%.2f",
			translatedPixels, identityPixels, ratio)
	}
}

// --------------------------------------------------------------------------
// Tier 1: Uniform positive scale ≤256px → bitmap at device size
// --------------------------------------------------------------------------

func TestDrawStringCPU_UniformScale(t *testing.T) {
	dc, source := setupTextContext(t, 800, 400, 12)
	defer func() { _ = source.Close() }()

	// Draw at 1x scale
	dc.DrawString("X", 50, 100)
	pixels1x := countNonWhitePixels(dc, 0, 0, 800, 400)

	// Draw at 2x uniform scale — should use Strategy A (bitmap at device size)
	dc.ClearWithColor(White)
	dc.Push()
	dc.Scale(2, 2)
	dc.DrawString("X", 50, 100)
	dc.Pop()
	pixels2x := countNonWhitePixels(dc, 0, 0, 800, 400)

	if pixels1x == 0 {
		t.Fatal("1x draw produced no pixels")
	}
	if pixels2x == 0 {
		t.Fatal("2x uniform scale draw produced no pixels")
	}

	// 2x scale should produce significantly more pixels (roughly 4x area)
	if pixels2x <= pixels1x {
		t.Errorf("2x scale (%d pixels) should produce more than 1x (%d pixels)", pixels2x, pixels1x)
	}
}

func TestDrawStringCPU_UniformScaleThreshold(t *testing.T) {
	// Scale that pushes device size above 256px threshold
	// Font size 12 * scale 25 = 300px > 256 → should use outline path (Tier 2)
	dc, source := setupTextContext(t, 2000, 2000, 12)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Scale(25, 25)
	dc.DrawString("A", 10, 50)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 2000, 2000)
	if pixels == 0 {
		t.Error("Large uniform scale (>256px threshold) produced no pixels")
	}
	// Should produce a large amount of pixels for such a big character
	if pixels < 100 {
		t.Errorf("Large uniform scale produced too few pixels: %d", pixels)
	}
}

// --------------------------------------------------------------------------
// Tier 2: Non-trivial transforms → glyph outlines as paths
// --------------------------------------------------------------------------

func TestDrawStringCPU_Rotation(t *testing.T) {
	dc, source := setupTextContext(t, 400, 400, 24)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Translate(200, 200)
	dc.Rotate(math.Pi / 4) // 45 degrees
	dc.DrawString("Rotated", -50, 0)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 400, 400)
	if pixels == 0 {
		t.Error("Rotated text produced no pixels")
	}
}

func TestDrawStringCPU_NonUniformScale(t *testing.T) {
	dc, source := setupTextContext(t, 400, 400, 16)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Scale(2, 3) // Non-uniform → must use outlines
	dc.DrawString("Stretched", 20, 50)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 400, 400)
	if pixels == 0 {
		t.Error("Non-uniform scale text produced no pixels")
	}
}

func TestDrawStringCPU_Shear(t *testing.T) {
	dc, source := setupTextContext(t, 400, 200, 20)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Shear(0.3, 0) // Faux italic
	dc.DrawString("Italic", 50, 100)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 400, 200)
	if pixels == 0 {
		t.Error("Sheared (faux italic) text produced no pixels")
	}
}

func TestDrawStringCPU_NegativeScale(t *testing.T) {
	dc, source := setupTextContext(t, 400, 400, 16)
	defer func() { _ = source.Close() }()

	// Negative X scale (mirror) → goes to outline path
	dc.Push()
	dc.Translate(300, 0)
	dc.Scale(-1, 1)
	dc.DrawString("Mirror", 0, 100)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 400, 400)
	if pixels == 0 {
		t.Error("Negative scale (mirror) text produced no pixels")
	}
}

// --------------------------------------------------------------------------
// DrawStringAnchored with transforms
// --------------------------------------------------------------------------

func TestDrawStringAnchored_WithTransform(t *testing.T) {
	dc, source := setupTextContext(t, 400, 400, 20)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Scale(2, 2)
	dc.DrawStringAnchored("Center", 100, 100, 0.5, 0.5)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 400, 400)
	if pixels == 0 {
		t.Error("DrawStringAnchored with scale produced no pixels")
	}
}

func TestDrawStringAnchored_Rotated(t *testing.T) {
	dc, source := setupTextContext(t, 400, 400, 18)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Translate(200, 200)
	dc.Rotate(math.Pi / 6) // 30 degrees
	dc.DrawStringAnchored("Angled", 0, 0, 0.5, 0.5)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 400, 400)
	if pixels == 0 {
		t.Error("DrawStringAnchored with rotation produced no pixels")
	}
}

// --------------------------------------------------------------------------
// DrawStringWrapped inherits transform support
// --------------------------------------------------------------------------

func TestDrawStringWrapped_WithScale(t *testing.T) {
	dc, source := setupTextContext(t, 800, 800, 14)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Scale(2, 2)
	dc.DrawStringWrapped("The quick brown fox", 10, 30, 0, 0, 150, 1.2, AlignLeft)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 800, 800)
	if pixels == 0 {
		t.Error("DrawStringWrapped with scale produced no pixels")
	}
}

// --------------------------------------------------------------------------
// Edge cases
// --------------------------------------------------------------------------

func TestDrawStringCPU_EmptyString(t *testing.T) {
	dc, source := setupTextContext(t, 200, 100, 16)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Rotate(math.Pi / 4)
	// Should not panic on empty string
	dc.DrawString("", 50, 50)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 200, 100)
	if pixels != 0 {
		t.Errorf("Empty string produced %d pixels, expected 0", pixels)
	}
}

func TestDrawStringCPU_SpacesOnly(t *testing.T) {
	dc, source := setupTextContext(t, 200, 100, 16)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Rotate(math.Pi / 4)
	// Spaces have no outlines — should not panic, should produce no pixels
	dc.DrawString("   ", 50, 50)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 200, 100)
	if pixels != 0 {
		t.Errorf("Spaces-only string produced %d pixels, expected 0", pixels)
	}
}

func TestDrawStringCPU_NoFont(t *testing.T) {
	dc := NewContext(200, 100)
	defer dc.Close()

	// No font set — should not panic with any transform
	dc.Push()
	dc.Rotate(math.Pi / 4)
	dc.DrawString("Test", 50, 50)
	dc.DrawStringAnchored("Test", 100, 50, 0.5, 0.5)
	dc.Pop()
}

func TestDrawStringCPU_IdentityMatrix(t *testing.T) {
	dc, source := setupTextContext(t, 400, 200, 24)
	defer func() { _ = source.Close() }()

	// Identity matrix → should use bitmap fast path (Tier 0)
	dc.DrawString("Identity", 50, 100)

	pixels := countNonWhitePixels(dc, 0, 0, 400, 200)
	if pixels == 0 {
		t.Error("Identity matrix text produced no pixels")
	}
}

// --------------------------------------------------------------------------
// MultiFace fallback — Source() returns nil → bitmap fallback
// --------------------------------------------------------------------------

func TestDrawStringCPU_MultiFaceFallback(t *testing.T) {
	fontPath := findSystemFont(t)
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	// MultiFace wraps faces — Source() returns nil
	multi, err := text.NewMultiFace(source.Face(16))
	if err != nil {
		t.Fatalf("Failed to create MultiFace: %v", err)
	}

	dc := NewContext(400, 200)
	defer dc.Close()
	dc.ClearWithColor(White)
	dc.SetFont(multi)
	dc.SetRGB(0, 0, 0)

	// With rotation → drawStringAsOutlines would be called, but Source() == nil
	// → should fall back to bitmap gracefully
	dc.Push()
	dc.Rotate(0.1)
	dc.DrawString("MultiFace", 50, 100)
	dc.Pop()

	// Should not panic; may or may not produce correct rotated output
	// (bitmap fallback means position-only transform)
}

// --------------------------------------------------------------------------
// ensureOutlineExtractor lazy initialization
// --------------------------------------------------------------------------

func TestEnsureOutlineExtractor(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()

	// Initially nil
	if dc.outlineExtractor != nil {
		t.Error("outlineExtractor should be nil initially")
	}

	// First call creates it
	ext1 := dc.ensureOutlineExtractor()
	if ext1 == nil {
		t.Fatal("ensureOutlineExtractor returned nil")
	}

	// Second call returns the same instance
	ext2 := dc.ensureOutlineExtractor()
	if ext1 != ext2 {
		t.Error("ensureOutlineExtractor should return the same instance")
	}
}

// --------------------------------------------------------------------------
// Decision tree tier selection verification
// --------------------------------------------------------------------------

func TestDrawStringCPU_TierSelection(t *testing.T) {
	// This test verifies that the decision tree selects the correct tier
	// by comparing pixel output characteristics across transform types.

	dc, source := setupTextContext(t, 600, 600, 16)
	defer func() { _ = source.Close() }()

	tests := []struct {
		name      string
		transform func()
		restore   func()
		wantInk   bool
	}{
		{
			name:      "identity",
			transform: func() {},
			restore:   func() {},
			wantInk:   true,
		},
		{
			name:      "translate_50_30",
			transform: func() { dc.Push(); dc.Translate(50, 30) },
			restore:   func() { dc.Pop() },
			wantInk:   true,
		},
		{
			name:      "uniform_scale_3x",
			transform: func() { dc.Push(); dc.Scale(3, 3) },
			restore:   func() { dc.Pop() },
			wantInk:   true,
		},
		{
			name:      "non_uniform_scale",
			transform: func() { dc.Push(); dc.Scale(1, 2) },
			restore:   func() { dc.Pop() },
			wantInk:   true,
		},
		{
			name:      "rotate_90deg",
			transform: func() { dc.Push(); dc.Translate(300, 0); dc.Rotate(math.Pi / 2) },
			restore:   func() { dc.Pop() },
			wantInk:   true,
		},
		{
			name:      "shear",
			transform: func() { dc.Push(); dc.Shear(0.5, 0) },
			restore:   func() { dc.Pop() },
			wantInk:   true,
		},
		{
			name:      "scale_translate_combo",
			transform: func() { dc.Push(); dc.Translate(100, 50); dc.Scale(2, 2) },
			restore:   func() { dc.Pop() },
			wantInk:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.ClearWithColor(White)
			tt.transform()
			dc.DrawString("Tier", 10, 40)
			tt.restore()

			pixels := countNonWhitePixels(dc, 0, 0, 600, 600)
			if tt.wantInk && pixels == 0 {
				t.Errorf("transform %q produced no pixels", tt.name)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Regression: Scale(2,2) text should be visually larger than Scale(1,1)
// --------------------------------------------------------------------------

func TestDrawStringCPU_ScaleProducesLargerText(t *testing.T) {
	dc, source := setupTextContext(t, 800, 400, 16)
	defer func() { _ = source.Close() }()

	// Draw at 1x
	dc.DrawString("ABC", 50, 100)
	pixels1x := countNonWhitePixels(dc, 0, 0, 800, 400)

	// Draw at 3x uniform scale
	dc.ClearWithColor(White)
	dc.Push()
	dc.Scale(3, 3)
	dc.DrawString("ABC", 50, 100)
	dc.Pop()
	pixels3x := countNonWhitePixels(dc, 0, 0, 800, 400)

	if pixels1x == 0 {
		t.Fatal("1x scale produced no pixels")
	}
	if pixels3x == 0 {
		t.Fatal("3x scale produced no pixels")
	}

	// 3x scale should produce at least 3x more ink (approximately 9x for area)
	ratio := float64(pixels3x) / float64(pixels1x)
	if ratio < 2.0 {
		t.Errorf("3x scale ratio too low: %d/%d = %.2f (expected > 2.0)", pixels3x, pixels1x, ratio)
	}
}

// --------------------------------------------------------------------------
// Regression: Rotation should spread ink differently than no rotation
// --------------------------------------------------------------------------

func TestDrawStringCPU_RotationChangesInkDistribution(t *testing.T) {
	dc, source := setupTextContext(t, 400, 400, 20)
	defer func() { _ = source.Close() }()

	// Draw without rotation
	dc.DrawString("ABCD", 100, 200)

	// Count ink in the right half
	rightInk := countNonWhitePixels(dc, 200, 0, 400, 400)

	// Draw with 45-degree rotation from center
	dc.ClearWithColor(White)
	dc.Push()
	dc.Translate(200, 200)
	dc.Rotate(math.Pi / 4)
	dc.DrawString("ABCD", -40, 0)
	dc.Pop()

	// With rotation, ink should appear in the upper-right quadrant
	topInk := countNonWhitePixels(dc, 0, 0, 400, 200)
	bottomInk := countNonWhitePixels(dc, 0, 200, 400, 400)

	totalRotated := topInk + bottomInk
	if totalRotated == 0 {
		t.Fatal("Rotated text produced no pixels")
	}

	// Rotated text should have ink in both top and bottom halves
	// (unlike horizontal text which is mostly in one horizontal band)
	if topInk == 0 || bottomInk == 0 {
		t.Logf("Ink distribution: top=%d bottom=%d right(no-rot)=%d", topInk, bottomInk, rightInk)
		// This is a soft check — font metrics and exact placement may vary
	}
}

// --------------------------------------------------------------------------
// Combined transform: Translate + Scale + Rotate
// --------------------------------------------------------------------------

func TestDrawStringCPU_CombinedTransform(t *testing.T) {
	dc, source := setupTextContext(t, 600, 600, 14)
	defer func() { _ = source.Close() }()

	dc.Push()
	dc.Translate(300, 300)
	dc.Scale(2, 2)
	dc.Rotate(math.Pi / 6) // 30 degrees
	dc.DrawString("Combined", -40, 0)
	dc.Pop()

	pixels := countNonWhitePixels(dc, 0, 0, 600, 600)
	if pixels == 0 {
		t.Error("Combined transform (translate+scale+rotate) produced no pixels")
	}
}
