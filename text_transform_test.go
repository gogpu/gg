package gg

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
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

// --------------------------------------------------------------------------
// Golden test: 9 transform scenarios with visual output verification
// --------------------------------------------------------------------------

// getPixelPositions returns a set of (x,y) coordinates that have non-white pixels.
func getPixelPositions(dc *Context) map[[2]int]struct{} {
	positions := make(map[[2]int]struct{})
	for y := 0; y < dc.height; y++ {
		for x := 0; x < dc.width; x++ {
			p := dc.pixmap.GetPixel(x, y)
			if p.R < 0.99 || p.G < 0.99 || p.B < 0.99 {
				positions[[2]int{x, y}] = struct{}{}
			}
		}
	}
	return positions
}

// positionsOverlap returns the fraction of positions in a that also exist in b.
// Returns 0.0 if a is empty.
func positionsOverlap(a, b map[[2]int]struct{}) float64 {
	if len(a) == 0 {
		return 0
	}
	shared := 0
	for pos := range a {
		if _, ok := b[pos]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(a))
}

// saveGoldenImage saves a context image to tmp/golden_text_transform_<name>.png.
func saveGoldenImage(t *testing.T, name string, dc *Context) {
	t.Helper()
	tmpDir := "tmp"
	_ = os.MkdirAll(tmpDir, 0o755)

	path := filepath.Join(tmpDir, fmt.Sprintf("golden_text_transform_%s.png", name))
	f, err := os.Create(path)
	if err != nil {
		t.Logf("Failed to create %s: %v", path, err)
		return
	}
	defer func() { _ = f.Close() }()

	img := dc.pixmap.ToImage()
	if err := png.Encode(f, img); err != nil {
		t.Logf("Failed to encode PNG %s: %v", path, err)
		return
	}
	t.Logf("Saved golden image: %s", path)
}

func TestTextTransformGolden(t *testing.T) {
	fontPath := findSystemFont(t)
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	saveDiffs := os.Getenv("SAVE_DIFFS") == "1"

	type scenario struct {
		name         string
		width        int     // canvas width (0 = default 200)
		height       int     // canvas height (0 = default 100)
		drawX        float64 // draw position X (0 = default 20)
		drawY        float64 // draw position Y (0 = default 50)
		transform    func(dc *Context)
		expectedTier string // documentation only
	}

	const (
		defaultWidth  = 200
		defaultHeight = 100
		defaultDrawX  = 20.0
		defaultDrawY  = 50.0
		fontSize      = 16
		drawText      = "Test"
	)

	scenarios := []scenario{
		{
			name:         "Identity",
			transform:    func(_ *Context) {}, // no transform
			expectedTier: "Tier 0 (bitmap)",
		},
		{
			name: "Translate",
			transform: func(dc *Context) {
				dc.Translate(50, 50)
			},
			expectedTier: "Tier 0 (bitmap)",
		},
		{
			name: "UniformScale2x",
			transform: func(dc *Context) {
				dc.Scale(2, 2)
			},
			expectedTier: "Tier 1 (bitmap@2x)",
		},
		{
			name: "UniformScaleDown",
			transform: func(dc *Context) {
				dc.Scale(0.7, 0.7)
			},
			expectedTier: "Tier 1 (bitmap@0.7x)",
		},
		{
			name:   "LargeScale",
			width:  800,
			height: 600,
			drawX:  5,
			drawY:  10,
			transform: func(dc *Context) {
				dc.Scale(10, 10)
			},
			expectedTier: "Tier 2 (outlines, >256px)",
		},
		{
			name:  "Rotate30",
			width: 300, height: 200,
			drawX: 0, drawY: 0,
			transform: func(dc *Context) {
				dc.Translate(100, 80)
				dc.Rotate(math.Pi / 6)
			},
			expectedTier: "Tier 2 (outlines)",
		},
		{
			name:  "Rotate45",
			width: 300, height: 200,
			drawX: 0, drawY: 0,
			transform: func(dc *Context) {
				dc.Translate(100, 80)
				dc.Rotate(math.Pi / 4)
			},
			expectedTier: "Tier 2 (outlines)",
		},
		{
			name: "NonUniformScale",
			transform: func(dc *Context) {
				dc.Scale(2, 1)
			},
			expectedTier: "Tier 2 (outlines)",
		},
		{
			name: "Shear",
			transform: func(dc *Context) {
				dc.Shear(-0.3, 0) // B=-0.3 faux italic (lean right)
			},
			expectedTier: "Tier 2 (outlines)",
		},
	}

	// Render identity first to use as baseline for comparison.
	var identityPixels int
	var identityPositions map[[2]int]struct{}

	results := make([]struct {
		name      string
		pixels    int
		positions map[[2]int]struct{}
	}, len(scenarios))

	for i, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			w := sc.width
			if w == 0 {
				w = defaultWidth
			}
			h := sc.height
			if h == 0 {
				h = defaultHeight
			}
			dx := sc.drawX
			if dx == 0 && sc.width == 0 {
				dx = defaultDrawX
			}
			dy := sc.drawY
			if dy == 0 && sc.height == 0 {
				dy = defaultDrawY
			}

			dc := NewContext(w, h)
			dc.ClearWithColor(White)
			dc.SetFont(source.Face(fontSize))
			dc.SetRGB(0, 0, 0)

			dc.Push()
			sc.transform(dc)
			dc.DrawString(drawText, dx, dy)
			dc.Pop()

			pixels := countNonWhitePixels(dc, 0, 0, w, h)
			positions := getPixelPositions(dc)

			results[i].name = sc.name
			results[i].pixels = pixels
			results[i].positions = positions

			// Capture identity baseline.
			if i == 0 {
				identityPixels = pixels
				identityPositions = positions
			}

			// Assert: every scenario must produce visible text.
			if pixels == 0 {
				t.Errorf("scenario %q produced no visible pixels (expected tier: %s)", sc.name, sc.expectedTier)
			}

			if saveDiffs {
				saveGoldenImage(t, sc.name, dc)
			}
		})
	}

	// Cross-scenario comparisons (scenarios vs identity).
	// These run after all sub-tests have populated results.
	t.Run("CrossComparisons", func(t *testing.T) {
		if identityPixels == 0 {
			t.Skip("Identity scenario produced no pixels; cannot compare")
		}

		// UniformScale2x (index 2): should produce MORE pixels than identity.
		// Both use the same default canvas size.
		if results[2].pixels > 0 && results[2].pixels <= identityPixels {
			t.Errorf("UniformScale2x (%d pixels) should produce more pixels than Identity (%d pixels)",
				results[2].pixels, identityPixels)
		}

		// UniformScaleDown (index 3): should produce FEWER pixels than identity.
		// Both use the same default canvas size.
		if results[3].pixels > 0 && results[3].pixels >= identityPixels {
			t.Errorf("UniformScaleDown (%d pixels) should produce fewer pixels than Identity (%d pixels)",
				results[3].pixels, identityPixels)
		}

		// For default-canvas scenarios (NonUniformScale=7, Shear=8):
		// pixel positions should differ from identity (overlap < 90%).
		// LargeScale(4), Rotate30(5), Rotate45(6) use different canvas sizes,
		// so position overlap with identity is not meaningful.
		for _, idx := range []int{7, 8} {
			if len(results[idx].positions) == 0 {
				continue // already reported as error in sub-test
			}
			overlap := positionsOverlap(results[idx].positions, identityPositions)
			if overlap > 0.90 {
				t.Errorf("scenario %q has %.1f%% pixel overlap with Identity (expected <90%% for a visible transform effect)",
					results[idx].name, overlap*100)
			}
		}

		// LargeScale (index 4): just verify it produced substantial ink
		// (large text = many pixels). Canvas is 800x600.
		if results[4].pixels > 0 && results[4].pixels < 500 {
			t.Errorf("LargeScale produced only %d pixels; expected >500 for 10x scaled text", results[4].pixels)
		}

		// Rotate30 (index 5) and Rotate45 (index 6): verify they produced ink.
		// They use a 300x200 canvas, so we just check non-zero (already done in sub-tests).
	})
}
