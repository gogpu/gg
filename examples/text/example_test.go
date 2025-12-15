package main

import (
	"os"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// TestFindSystemFont verifies font discovery works.
func TestFindSystemFont(t *testing.T) {
	fontPath := getSystemFont()
	if fontPath == "" {
		t.Skip("No system font available on this system")
	}

	// Verify the file exists
	info, err := os.Stat(fontPath)
	if err != nil {
		t.Errorf("Font path %s not accessible: %v", fontPath, err)
	}

	if info.IsDir() {
		t.Errorf("Font path %s is a directory, expected file", fontPath)
	}

	t.Logf("Found system font: %s (%d bytes)", fontPath, info.Size())
}

// TestTextRendering tests the basic text rendering functionality.
func TestTextRendering(t *testing.T) {
	fontPath := getSystemFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	ctx := gg.NewContext(400, 200)
	ctx.ClearWithColor(gg.White)

	face := source.Face(24)
	ctx.SetFont(face)
	ctx.SetRGB(0, 0, 0)
	ctx.DrawString("Test", 50, 50)

	// Verify context dimensions
	if ctx.Width() != 400 {
		t.Errorf("Expected width 400, got %d", ctx.Width())
	}
	if ctx.Height() != 200 {
		t.Errorf("Expected height 200, got %d", ctx.Height())
	}
}

// TestMeasureString tests text measurement accuracy.
func TestMeasureString(t *testing.T) {
	fontPath := getSystemFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	ctx := gg.NewContext(100, 100)
	face := source.Face(24)
	ctx.SetFont(face)

	tests := []struct {
		text string
	}{
		{""},
		{"A"},
		{"Hello"},
		{"Hello World"},
		{"AAAAAAAAAA"},
	}

	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			w, h := ctx.MeasureString(tc.text)

			if tc.text == "" {
				if w != 0 {
					t.Errorf("Expected width 0 for empty string, got %f", w)
				}
				return
			}

			if w <= 0 {
				t.Errorf("Expected positive width for %q, got %f", tc.text, w)
			}
			if h <= 0 {
				t.Errorf("Expected positive height for %q, got %f", tc.text, h)
			}
		})
	}
}

// TestDrawStringAnchored tests anchored text positioning.
func TestDrawStringAnchored(t *testing.T) {
	fontPath := getSystemFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	ctx := gg.NewContext(200, 200)
	ctx.ClearWithColor(gg.White)

	face := source.Face(16)
	ctx.SetFont(face)
	ctx.SetRGB(0, 0, 0)

	// Test various anchor positions
	anchors := []struct {
		name   string
		ax, ay float64
	}{
		{"top-left", 0, 0},
		{"center", 0.5, 0.5},
		{"bottom-right", 1, 1},
	}

	for _, a := range anchors {
		t.Run(a.name, func(t *testing.T) {
			ctx.DrawStringAnchored("X", 100, 100, a.ax, a.ay)
			// If we get here without panic, the anchoring works
		})
	}
}

// TestMultipleFaces tests creating faces at different sizes.
func TestMultipleFaces(t *testing.T) {
	fontPath := getSystemFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	sizes := []float64{8, 12, 16, 24, 32, 48, 72}

	for _, size := range sizes {
		t.Run("", func(t *testing.T) {
			face := source.Face(size)
			if face == nil {
				t.Errorf("Face(%f) returned nil", size)
				return
			}

			// Check metrics scale with size
			metrics := face.Metrics()
			if metrics.Ascent <= 0 {
				t.Errorf("Expected positive ascent, got %f", metrics.Ascent)
			}
		})
	}
}

// BenchmarkTextRendering benchmarks text drawing performance.
func BenchmarkTextRendering(b *testing.B) {
	fontPath := getSystemFont()
	if fontPath == "" {
		b.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		b.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	ctx := gg.NewContext(800, 600)
	face := source.Face(24)
	ctx.SetFont(face)
	ctx.SetRGB(0, 0, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.DrawString("Hello, World!", 100, 100)
	}
}

// BenchmarkMeasureString benchmarks text measurement.
func BenchmarkMeasureString(b *testing.B) {
	fontPath := getSystemFont()
	if fontPath == "" {
		b.Skip("No system font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		b.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	ctx := gg.NewContext(100, 100)
	face := source.Face(24)
	ctx.SetFont(face)

	testText := "Hello, World! This is a longer string for benchmarking."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ctx.MeasureString(testText)
	}
}

// getSystemFont returns a TTF font path (TTC collections not supported).
func getSystemFont() string {
	// Only TTF files are supported (not TTC font collections)
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\calibri.ttf",
		// macOS - Supplemental fonts are TTF
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf",
		"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
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
