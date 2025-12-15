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
	ctx := NewContext(400, 200)
	ctx.SetRGB(1, 1, 1) // White background
	ctx.Clear()

	// Load font
	err := ctx.LoadFontFace(fontPath, 24.0)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}

	// Set text color
	ctx.SetRGB(0, 0, 0) // Black text

	// Draw string
	ctx.DrawString("Hello, World!", 50, 100)

	// Verify font is set
	if ctx.Font() == nil {
		t.Error("Expected font to be set")
	}

	// Measure string
	w, h := ctx.MeasureString("Hello, World!")
	if w <= 0 || h <= 0 {
		t.Errorf("Expected positive dimensions, got (%f, %f)", w, h)
	}

	// Draw anchored string
	ctx.DrawStringAnchored("Centered", 200, 150, 0.5, 0.5)

	// Save (optional for visual verification)
	// _ = ctx.SavePNG("test_output.png")
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
	ctx := NewContext(400, 200)

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
	ctx.SetFont(face)

	// Verify face is set
	if ctx.Font() == nil {
		t.Error("Expected font to be set")
	}

	if ctx.Font().Size() != 18.0 {
		t.Errorf("Expected size 18.0, got %f", ctx.Font().Size())
	}

	// Draw text
	ctx.SetRGB(0, 0, 0)
	ctx.DrawString("New API Test", 10, 50)

	// Measure
	w, h := ctx.MeasureString("New API Test")
	if w <= 0 || h <= 0 {
		t.Errorf("Expected positive dimensions, got (%f, %f)", w, h)
	}
}

// TestTextNoFont tests behavior when no font is set.
func TestTextNoFont(t *testing.T) {
	ctx := NewContext(200, 100)

	// DrawString with no font (should not panic)
	ctx.DrawString("Test", 10, 50)

	// DrawStringAnchored with no font (should not panic)
	ctx.DrawStringAnchored("Test", 100, 50, 0.5, 0.5)

	// MeasureString with no font (should return 0, 0)
	w, h := ctx.MeasureString("Test")
	if w != 0 || h != 0 {
		t.Errorf("Expected (0, 0) with no font, got (%f, %f)", w, h)
	}
}
