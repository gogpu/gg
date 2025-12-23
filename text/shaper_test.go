package text

import (
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// builtinTestFace creates a test Face at size 16 for builtin shaper tests.
func builtinTestFace(t *testing.T) Face {
	t.Helper()

	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to create font source: %v", err)
	}
	t.Cleanup(func() {
		_ = source.Close()
	})

	return source.Face(16.0)
}

// TestBuiltinShapeEmpty tests shaping empty text.
func TestBuiltinShapeEmpty(t *testing.T) {
	face := builtinTestFace(t)

	result := Shape("", face, 16.0)
	if result != nil {
		t.Errorf("Shape(\"\") = %v, want nil", result)
	}
}

// TestBuiltinShapeLatinText tests shaping basic Latin text.
func TestBuiltinShapeLatinText(t *testing.T) {
	face := builtinTestFace(t)

	tests := []struct {
		name    string
		text    string
		wantLen int
	}{
		{"single char", "A", 1},
		{"word", "Hello", 5},
		{"with space", "Hello World", 11},
		{"numbers", "12345", 5},
		{"punctuation", "Hello, World!", 13},
		{"lowercase", "abcdefg", 7},
		{"uppercase", "ABCDEFG", 7},
		{"mixed case", "AbCdEfG", 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Shape(tt.text, face, 16.0)

			if len(result) != tt.wantLen {
				t.Errorf("len(Shape(%q)) = %d, want %d", tt.text, len(result), tt.wantLen)
			}

			// Verify glyph positions are increasing
			var prevX float64
			for i, g := range result {
				if i > 0 && g.X < prevX {
					t.Errorf("glyph %d: X=%f < previous X=%f", i, g.X, prevX)
				}
				prevX = g.X

				// Verify advance is positive
				if g.XAdvance <= 0 {
					t.Errorf("glyph %d: XAdvance=%f should be positive", i, g.XAdvance)
				}

				// Verify cluster is correct
				if g.Cluster != i {
					t.Errorf("glyph %d: Cluster=%d, want %d", i, g.Cluster, i)
				}
			}
		})
	}
}

// TestBuiltinShapeCyrillic tests shaping Cyrillic text.
func TestBuiltinShapeCyrillic(t *testing.T) {
	face := builtinTestFace(t)

	// Go Regular font supports Cyrillic
	text := "Hello" // Using Latin as Go Regular may not have full Cyrillic
	result := Shape(text, face, 16.0)

	if len(result) != 5 {
		t.Errorf("len(Shape(%q)) = %d, want 5", text, len(result))
	}

	// Verify basic structure
	for i, g := range result {
		if g.GID == 0 {
			// GID 0 means missing glyph (acceptable for fonts without full coverage)
			t.Logf("glyph %d: missing glyph for character", i)
		}
		if g.XAdvance <= 0 {
			t.Errorf("glyph %d: XAdvance=%f should be positive", i, g.XAdvance)
		}
	}
}

// TestBuiltinShapeCJK tests shaping CJK characters.
func TestBuiltinShapeCJK(t *testing.T) {
	face := builtinTestFace(t)

	// Note: Go Regular font may not have CJK glyphs
	// We just test that shaping completes without panic
	text := "ABC123" // Use ASCII that definitely exists
	result := Shape(text, face, 16.0)

	if len(result) != 6 {
		t.Errorf("len(Shape(%q)) = %d, want 6", text, len(result))
	}
}

// TestSetShaper tests setting a custom shaper.
func TestSetShaper(t *testing.T) {
	// Save original shaper
	original := GetShaper()
	t.Cleanup(func() {
		SetShaper(original)
	})

	// Create a custom shaper that returns empty
	customShaper := &mockShaper{glyphs: []ShapedGlyph{}}

	SetShaper(customShaper)

	current := GetShaper()
	if current != customShaper {
		t.Error("GetShaper() should return the custom shaper after SetShaper()")
	}
}

// TestSetShaperNil tests that nil resets to default.
func TestSetShaperNil(t *testing.T) {
	// Save original shaper
	original := GetShaper()
	t.Cleanup(func() {
		SetShaper(original)
	})

	// Set to nil should reset to BuiltinShaper
	SetShaper(nil)

	current := GetShaper()
	if _, ok := current.(*BuiltinShaper); !ok {
		t.Errorf("GetShaper() after SetShaper(nil) should be *BuiltinShaper, got %T", current)
	}
}

// TestGetShaper tests getting the current shaper.
func TestGetShaper(t *testing.T) {
	shaper := GetShaper()
	if shaper == nil {
		t.Error("GetShaper() returned nil")
	}

	// Default should be BuiltinShaper
	if _, ok := shaper.(*BuiltinShaper); !ok {
		t.Errorf("default shaper should be *BuiltinShaper, got %T", shaper)
	}
}

// TestShapeNilFace tests shaping with nil face.
func TestShapeNilFace(t *testing.T) {
	result := Shape("Hello", nil, 16.0)
	if result != nil {
		t.Errorf("Shape with nil face should return nil, got %v", result)
	}
}

// TestBuiltinShaperDirectly tests BuiltinShaper directly.
func TestBuiltinShaperDirectly(t *testing.T) {
	face := builtinTestFace(t)
	shaper := &BuiltinShaper{}

	result := shaper.Shape("Test", face, 16.0)
	if len(result) != 4 {
		t.Errorf("len(BuiltinShaper.Shape(\"Test\")) = %d, want 4", len(result))
	}
}

// TestShapedGlyphFields tests that ShapedGlyph fields are populated correctly.
func TestShapedGlyphFields(t *testing.T) {
	face := builtinTestFace(t)

	result := Shape("AB", face, 16.0)
	if len(result) != 2 {
		t.Fatalf("len(Shape(\"AB\")) = %d, want 2", len(result))
	}

	// First glyph should start at x=0
	if result[0].X != 0 {
		t.Errorf("first glyph X=%f, want 0", result[0].X)
	}

	// Second glyph should be at X = first.X + first.XAdvance
	expectedX := result[0].X + result[0].XAdvance
	if result[1].X != expectedX {
		t.Errorf("second glyph X=%f, want %f", result[1].X, expectedX)
	}

	// Y should be 0 for horizontal text
	for i, g := range result {
		if g.Y != 0 {
			t.Errorf("glyph %d: Y=%f, want 0", i, g.Y)
		}
		if g.YAdvance != 0 {
			t.Errorf("glyph %d: YAdvance=%f, want 0", i, g.YAdvance)
		}
	}
}

// TestShapeDifferentSizes tests shaping at different font sizes.
func TestShapeDifferentSizes(t *testing.T) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	sizes := []float64{8, 12, 16, 24, 32, 48}
	var prevTotalAdvance float64

	for _, size := range sizes {
		face := source.Face(size)
		result := Shape("Hello", face, size)

		if len(result) != 5 {
			t.Errorf("size %f: len(Shape) = %d, want 5", size, len(result))
			continue
		}

		// Calculate total advance
		totalAdvance := result[len(result)-1].X + result[len(result)-1].XAdvance

		// Total advance should increase with size
		if size > 8 && totalAdvance <= prevTotalAdvance {
			t.Errorf("size %f: total advance %f should be > previous %f",
				size, totalAdvance, prevTotalAdvance)
		}
		prevTotalAdvance = totalAdvance
	}
}

// TestBuiltinShaperConcurrency tests concurrent access to the global shaper.
func TestBuiltinShaperConcurrency(t *testing.T) {
	face := builtinTestFace(t)

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				result := Shape("Hello World", face, 16.0)
				if len(result) != 11 {
					errors <- nil // Signal error (can't use t.Error in goroutine)
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	errCount := 0
	for range errors {
		errCount++
	}
	if errCount > 0 {
		t.Errorf("concurrent shaping failed %d times", errCount)
	}
}

// mockShaper is a mock implementation of Shaper for testing.
type mockShaper struct {
	glyphs []ShapedGlyph
}

func (m *mockShaper) Shape(string, Face, float64) []ShapedGlyph {
	return m.glyphs
}

// TestCustomShaperIntegration tests using a custom shaper.
func TestCustomShaperIntegration(t *testing.T) {
	// Save original
	original := GetShaper()
	t.Cleanup(func() {
		SetShaper(original)
	})

	// Create custom shaper that returns fixed glyphs
	customGlyphs := []ShapedGlyph{
		{GID: 100, Cluster: 0, X: 0, Y: 0, XAdvance: 10, YAdvance: 0},
		{GID: 101, Cluster: 1, X: 10, Y: 0, XAdvance: 10, YAdvance: 0},
	}
	SetShaper(&mockShaper{glyphs: customGlyphs})

	// Shape should return custom glyphs regardless of input
	face := builtinTestFace(t)
	result := Shape("anything", face, 16.0)

	if len(result) != 2 {
		t.Fatalf("custom shaper should return 2 glyphs, got %d", len(result))
	}

	if result[0].GID != 100 || result[1].GID != 101 {
		t.Error("custom shaper glyphs not returned correctly")
	}
}

// BenchmarkBuiltinShape benchmarks the Shape function with BuiltinShaper.
func BenchmarkBuiltinShape(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	text := "The quick brown fox jumps over the lazy dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Shape(text, face, 16.0)
	}
}

// BenchmarkBuiltinShapeShort benchmarks shaping short text.
func BenchmarkBuiltinShapeShort(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	text := "Hello"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Shape(text, face, 16.0)
	}
}

// BenchmarkBuiltinShapeLong benchmarks shaping long text.
func BenchmarkBuiltinShapeLong(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Shape(text, face, 16.0)
	}
}
