package gg

import (
	"os"
	"testing"

	"github.com/gogpu/gg/text"
)

// TestTextIntegration tests the integration of text drawing with Context.
func TestTextIntegration(t *testing.T) {
	// Only TTF files are supported (not TTC font collections)
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		// macOS - Supplemental fonts are TTF
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		// Linux
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	}

	var fontPath string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			fontPath = path
			break
		}
	}

	if fontPath == "" {
		t.Skip("No system font available for integration test")
	}

	// Create context
	dc := NewContext(400, 200)
	dc.SetRGB(1, 1, 1) // White background
	dc.Clear()

	// Load font
	err := dc.LoadFontFace(fontPath, 24.0)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}

	// Set text color
	dc.SetRGB(0, 0, 0) // Black text

	// Draw string
	dc.DrawString("Hello, World!", 50, 100)

	// Verify font is set
	if dc.Font() == nil {
		t.Error("Expected font to be set")
	}

	// Measure string
	w, h := dc.MeasureString("Hello, World!")
	if w <= 0 || h <= 0 {
		t.Errorf("Expected positive dimensions, got (%f, %f)", w, h)
	}

	// Draw anchored string
	dc.DrawStringAnchored("Centered", 200, 150, 0.5, 0.5)

	// Save (optional for visual verification)
	// _ = dc.SavePNG("test_output.png")
}

// TestTextNewAPI tests the new API using FontSource and SetFont.
func TestTextNewAPI(t *testing.T) {
	// Only TTF files are supported (not TTC font collections)
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		// macOS - Supplemental fonts are TTF
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		// Linux
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	}

	var fontPath string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			fontPath = path
			break
		}
	}

	if fontPath == "" {
		t.Skip("No system font available for integration test")
	}

	// Create context
	dc := NewContext(400, 200)

	// Load font using new API
	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	// Create face
	face := source.Face(18.0)
	dc.SetFont(face)

	// Verify face is set
	if dc.Font() == nil {
		t.Error("Expected font to be set")
	}

	if dc.Font().Size() != 18.0 {
		t.Errorf("Expected size 18.0, got %f", dc.Font().Size())
	}

	// Draw text
	dc.SetRGB(0, 0, 0)
	dc.DrawString("New API Test", 10, 50)

	// Measure
	w, h := dc.MeasureString("New API Test")
	if w <= 0 || h <= 0 {
		t.Errorf("Expected positive dimensions, got (%f, %f)", w, h)
	}
}

// TestTextDrawsPixels verifies that text drawing actually modifies pixels.
// This is a regression test for issue #11 where text was drawn to a copy
// of the pixmap instead of the actual pixmap.
func TestTextDrawsPixels(t *testing.T) {
	// Find a system font
	candidates := []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	}

	var fontPath string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			fontPath = path
			break
		}
	}

	if fontPath == "" {
		t.Skip("No system font available")
	}

	// Create context with white background
	dc := NewContext(200, 100)
	dc.ClearWithColor(White)

	// Verify background is white
	initialPixel := dc.pixmap.GetPixel(100, 50)
	if initialPixel.R != 1 || initialPixel.G != 1 || initialPixel.B != 1 {
		t.Fatalf("Expected white background, got %+v", initialPixel)
	}

	// Load font and draw black text
	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc.SetFont(source.Face(48)) // Large font for easy detection
	dc.SetRGB(0, 0, 0)          // Black text
	dc.DrawString("X", 80, 70)  // Draw near center

	// Count non-white pixels in the area where text should be
	nonWhiteCount := 0
	for y := 20; y < 80; y++ {
		for x := 70; x < 130; x++ {
			pixel := dc.pixmap.GetPixel(x, y)
			// Check if pixel is not pure white (text was drawn)
			if pixel.R < 0.99 || pixel.G < 0.99 || pixel.B < 0.99 {
				nonWhiteCount++
			}
		}
	}

	// With a 48pt "X", we expect significant number of non-white pixels
	if nonWhiteCount == 0 {
		t.Errorf("Text drawing produced no visible pixels! Expected text to modify pixmap. (issue #11 regression)")
	}

	t.Logf("Text drew %d non-white pixels", nonWhiteCount)
}

// TestTextNoFont tests behavior when no font is set.
func TestTextNoFont(t *testing.T) {
	dc := NewContext(200, 100)

	// DrawString with no font (should not panic)
	dc.DrawString("Test", 10, 50)

	// DrawStringAnchored with no font (should not panic)
	dc.DrawStringAnchored("Test", 100, 50, 0.5, 0.5)

	// MeasureString with no font (should return 0, 0)
	w, h := dc.MeasureString("Test")
	if w != 0 || h != 0 {
		t.Errorf("Expected (0, 0) with no font, got (%f, %f)", w, h)
	}
}
