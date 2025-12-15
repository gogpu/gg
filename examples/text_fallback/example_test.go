package main

import (
	"os"
	"runtime"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// TestFindMainFont verifies main font discovery works.
func TestFindMainFont(t *testing.T) {
	fontPath := getMainFont()
	if fontPath == "" {
		t.Skip("No main font available on this system")
	}

	info, err := os.Stat(fontPath)
	if err != nil {
		t.Errorf("Main font path %s not accessible: %v", fontPath, err)
	}

	if info.IsDir() {
		t.Errorf("Main font path %s is a directory", fontPath)
	}

	t.Logf("Found main font: %s", fontPath)
}

// TestFindEmojiFont checks emoji font discovery.
func TestFindEmojiFont(t *testing.T) {
	fontPath := getEmojiFont()
	if fontPath == "" {
		t.Log("No emoji font available (this is expected on some systems)")
		return
	}

	info, err := os.Stat(fontPath)
	if err != nil {
		t.Errorf("Emoji font path %s not accessible: %v", fontPath, err)
	}

	if info.IsDir() {
		t.Errorf("Emoji font path %s is a directory", fontPath)
	}

	t.Logf("Found emoji font: %s", fontPath)
}

// TestFilteredFace tests creating filtered faces.
func TestFilteredFace(t *testing.T) {
	fontPath := getMainFont()
	if fontPath == "" {
		t.Skip("No main font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(24)

	tests := []struct {
		name   string
		ranges []text.UnicodeRange
	}{
		{"ASCII only", []text.UnicodeRange{text.RangeBasicLatin}},
		{"Latin Extended", []text.UnicodeRange{text.RangeBasicLatin, text.RangeLatinExtA}},
		{"Cyrillic", []text.UnicodeRange{text.RangeCyrillic}},
		{"Emoji", []text.UnicodeRange{text.RangeEmoji}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filtered := text.NewFilteredFace(face, tc.ranges...)
			if filtered == nil {
				t.Error("NewFilteredFace returned nil")
				return
			}

			// Verify it implements Face
			_ = filtered.Metrics()
		})
	}
}

// TestMultiFace tests creating MultiFace for font fallback.
func TestMultiFace(t *testing.T) {
	fontPath := getMainFont()
	if fontPath == "" {
		t.Skip("No main font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	// Create two faces at different sizes (simulating fallback)
	face1 := source.Face(24)
	face2 := source.Face(24)

	multiFace, err := text.NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	// Test basic operations
	metrics := multiFace.Metrics()
	if metrics.Ascent <= 0 {
		t.Errorf("Expected positive ascent, got %f", metrics.Ascent)
	}

	advance := multiFace.Advance("Hello")
	if advance <= 0 {
		t.Errorf("Expected positive advance, got %f", advance)
	}

	// Test HasGlyph (ASCII should be available)
	if !multiFace.HasGlyph('A') {
		t.Error("Expected HasGlyph('A') to return true")
	}
}

// TestMultiFaceRendering tests drawing with MultiFace.
func TestMultiFaceRendering(t *testing.T) {
	fontPath := getMainFont()
	if fontPath == "" {
		t.Skip("No main font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(24)
	multiFace, err := text.NewMultiFace(face)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	ctx := gg.NewContext(400, 200)
	ctx.ClearWithColor(gg.White)
	ctx.SetFont(multiFace)
	ctx.SetRGB(0, 0, 0)

	// Should not panic
	ctx.DrawString("Hello World", 50, 100)

	w, h := ctx.MeasureString("Hello World")
	if w <= 0 || h <= 0 {
		t.Errorf("Expected positive dimensions, got w=%f, h=%f", w, h)
	}
}

// TestFilteredFaceRendering tests drawing with FilteredFace.
func TestFilteredFaceRendering(t *testing.T) {
	fontPath := getMainFont()
	if fontPath == "" {
		t.Skip("No main font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(24)
	filtered := text.NewFilteredFace(face, text.RangeBasicLatin)

	ctx := gg.NewContext(400, 200)
	ctx.ClearWithColor(gg.White)
	ctx.SetFont(filtered)
	ctx.SetRGB(0, 0, 0)

	// ASCII text should render fine
	ctx.DrawString("Hello ASCII", 50, 100)
}

// BenchmarkMultiFaceRendering benchmarks MultiFace drawing.
func BenchmarkMultiFaceRendering(b *testing.B) {
	fontPath := getMainFont()
	if fontPath == "" {
		b.Skip("No main font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		b.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(24)
	multiFace, _ := text.NewMultiFace(face)

	ctx := gg.NewContext(800, 600)
	ctx.SetFont(multiFace)
	ctx.SetRGB(0, 0, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.DrawString("Hello World!", 100, 100)
	}
}

// BenchmarkFilteredFaceRendering benchmarks FilteredFace drawing.
func BenchmarkFilteredFaceRendering(b *testing.B) {
	fontPath := getMainFont()
	if fontPath == "" {
		b.Skip("No main font available")
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		b.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(24)
	filtered := text.NewFilteredFace(face, text.RangeBasicLatin)

	ctx := gg.NewContext(800, 600)
	ctx.SetFont(filtered)
	ctx.SetRGB(0, 0, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.DrawString("Hello ASCII World!", 100, 100)
	}
}

// getMainFont is duplicated to make test self-contained.
func getMainFont() string {
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			"C:\\Windows\\Fonts\\arial.ttf",
			"C:\\Windows\\Fonts\\calibri.ttf",
		}
	case "darwin":
		candidates = []string{
			"/System/Library/Fonts/Helvetica.ttc",
			"/Library/Fonts/Arial.ttf",
		}
	default:
		candidates = []string{
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/TTF/DejaVuSans.ttf",
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// getEmojiFont is duplicated to make test self-contained.
func getEmojiFont() string {
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			"C:\\Windows\\Fonts\\seguiemj.ttf",
			"C:\\Windows\\Fonts\\seguisym.ttf",
		}
	case "darwin":
		candidates = []string{
			"/System/Library/Fonts/Apple Color Emoji.ttc",
		}
	default:
		candidates = []string{
			"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
			"/usr/share/fonts/noto-emoji/NotoColorEmoji.ttf",
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}
