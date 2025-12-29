package text

import (
	"image"
	"image/color"
	"os"
	"testing"
)

func TestDraw(t *testing.T) {
	fontPath := testFontPath(t)

	// Load font
	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	// Create face
	face := source.Face(12.0)

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, 200, 50))

	// Draw text
	Draw(dst, "Hello, World!", face, 10, 30, color.Black)

	// Verify that some pixels were modified (basic smoke test)
	modified := false
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				modified = true
				break
			}
		}
		if modified {
			break
		}
	}

	if !modified {
		t.Error("Expected Draw to modify the destination image")
	}
}

func TestDrawEmpty(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)
	dst := image.NewRGBA(image.Rect(0, 0, 100, 50))

	// Draw empty string (should not panic)
	Draw(dst, "", face, 10, 30, color.Black)
}

func TestDrawNilFace(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 100, 50))

	// Draw with nil face (should not panic)
	Draw(dst, "Hello", nil, 10, 30, color.Black)
}

func TestMeasure(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)

	tests := []struct {
		name string
		text string
	}{
		{"Simple", "Hello"},
		{"With spaces", "Hello World"},
		{"Long text", "The quick brown fox jumps over the lazy dog"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := Measure(tt.text, face)

			if w <= 0 {
				t.Errorf("Expected positive width, got %f", w)
			}

			if h <= 0 {
				t.Errorf("Expected positive height, got %f", h)
			}

			// Width should increase with text length
			if len(tt.text) > 5 {
				shortW, _ := Measure(tt.text[:5], face)
				if w <= shortW {
					t.Errorf("Expected width to increase with text length: %f vs %f", w, shortW)
				}
			}
		})
	}
}

func TestMeasureEmpty(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)

	w, h := Measure("", face)

	if w != 0 {
		t.Errorf("Expected width 0 for empty string, got %f", w)
	}

	// Height might be non-zero (line height), which is acceptable
	if h < 0 {
		t.Errorf("Expected non-negative height, got %f", h)
	}
}

func TestMeasureNilFace(t *testing.T) {
	w, h := Measure("Hello", nil)

	if w != 0 || h != 0 {
		t.Errorf("Expected (0, 0) for nil face, got (%f, %f)", w, h)
	}
}

func TestDrawColor(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(24.0)
	dst := image.NewRGBA(image.Rect(0, 0, 200, 50))

	// Draw with red color
	Draw(dst, "Test", face, 10, 30, color.RGBA{R: 255, A: 255})

	// Find a colored pixel
	foundRed := false
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			// Check if we have a red-ish pixel (r > 0, g ≈ 0, b ≈ 0)
			if r > 0 && g < 100 && b < 100 && a > 0 {
				foundRed = true
				break
			}
		}
		if foundRed {
			break
		}
	}

	if !foundRed {
		t.Error("Expected to find red pixels in the drawn text")
	}
}

func TestMeasureConsistency(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)

	// Measure the same text multiple times
	text := "Consistency Test"
	w1, h1 := Measure(text, face)
	w2, h2 := Measure(text, face)

	if w1 != w2 {
		t.Errorf("Width not consistent: %f vs %f", w1, w2)
	}

	if h1 != h2 {
		t.Errorf("Height not consistent: %f vs %f", h1, h2)
	}
}

func TestDrawMultiFace(t *testing.T) {
	fontPath := testFontPath(t)

	// Load font
	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	// Create faces for MultiFace
	face1 := source.Face(12.0)
	face2 := source.Face(14.0)

	// Create MultiFace
	multiFace, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("Failed to create MultiFace: %v", err)
	}

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, 200, 50))

	// Draw text using MultiFace - this was the bug in Issue #34
	Draw(dst, "Hello, World!", multiFace, 10, 30, color.Black)

	// Verify that some pixels were modified
	modified := false
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				modified = true
				break
			}
		}
		if modified {
			break
		}
	}

	if !modified {
		t.Error("Expected Draw with MultiFace to modify the destination image (Issue #34 regression)")
	}
}

func TestDrawFilteredFace(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)

	// Create FilteredFace with ASCII range
	filteredFace := NewFilteredFace(face, UnicodeRange{Start: 0x0020, End: 0x007F})

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, 200, 50))

	// Draw text using FilteredFace
	Draw(dst, "Hello", filteredFace, 10, 30, color.Black)

	// Verify that some pixels were modified
	modified := false
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				modified = true
				break
			}
		}
		if modified {
			break
		}
	}

	if !modified {
		t.Error("Expected Draw with FilteredFace to modify the destination image")
	}
}

func TestDrawMultiFaceWithFilteredFaces(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	// Create base faces
	face1 := source.Face(12.0)
	face2 := source.Face(12.0)

	// Create filtered faces for different ranges
	latinFace := NewFilteredFace(face1, UnicodeRange{Start: 0x0000, End: 0x024F}) // Latin
	extendedFace := NewFilteredFace(face2, UnicodeRange{Start: 0x0250, End: 0xFFFF})

	// Create MultiFace from filtered faces
	multiFace, err := NewMultiFace(latinFace, extendedFace)
	if err != nil {
		t.Fatalf("Failed to create MultiFace: %v", err)
	}

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, 300, 50))

	// Draw text - this tests the full composite face rendering chain
	Draw(dst, "Hello World 123", multiFace, 10, 30, color.Black)

	// Verify that some pixels were modified
	modified := false
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				modified = true
				break
			}
		}
		if modified {
			break
		}
	}

	if !modified {
		t.Error("Expected Draw with MultiFace containing FilteredFaces to modify the destination image")
	}
}

func TestMeasureMultiFace(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face1 := source.Face(12.0)
	face2 := source.Face(14.0)

	multiFace, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("Failed to create MultiFace: %v", err)
	}

	w, h := Measure("Hello", multiFace)

	if w <= 0 {
		t.Errorf("Expected positive width for MultiFace, got %f", w)
	}

	if h <= 0 {
		t.Errorf("Expected positive height for MultiFace, got %f", h)
	}
}

func TestMeasureFilteredFace(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)
	filteredFace := NewFilteredFace(face, UnicodeRange{Start: 0x0020, End: 0x007F})

	w, h := Measure("Hello", filteredFace)

	if w <= 0 {
		t.Errorf("Expected positive width for FilteredFace, got %f", w)
	}

	if h <= 0 {
		t.Errorf("Expected positive height for FilteredFace, got %f", h)
	}
}

func BenchmarkDraw(b *testing.B) {
	// Try to get a font, skip if not available
	candidates := []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}

	var fontPath string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			fontPath = path
			break
		}
	}

	if fontPath == "" {
		b.Skip("No font available for benchmarking")
	}

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		b.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)
	dst := image.NewRGBA(image.Rect(0, 0, 400, 100))
	text := "The quick brown fox jumps over the lazy dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Draw(dst, text, face, 10, 50, color.Black)
	}
}

func BenchmarkMeasure(b *testing.B) {
	candidates := []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}

	var fontPath string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			fontPath = path
			break
		}
	}

	if fontPath == "" {
		b.Skip("No font available for benchmarking")
	}

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		b.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)
	text := "The quick brown fox jumps over the lazy dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Measure(text, face)
	}
}
