package text

import (
	"iter"
	"testing"
)

// mockFace is a simple mock implementation for testing
type mockFace struct {
	metrics   Metrics
	size      float64
	direction Direction
	glyphs    map[rune]float64 // rune -> advance
}

func newMockFace(size float64, direction Direction, glyphs map[rune]float64) *mockFace {
	return &mockFace{
		metrics: Metrics{
			Ascent:    size * 0.8,
			Descent:   size * 0.2,
			LineGap:   size * 0.1,
			XHeight:   size * 0.5,
			CapHeight: size * 0.7,
		},
		size:      size,
		direction: direction,
		glyphs:    glyphs,
	}
}

func (m *mockFace) Metrics() Metrics            { return m.metrics }
func (m *mockFace) Direction() Direction        { return m.direction }
func (m *mockFace) Source() *FontSource         { return nil }
func (m *mockFace) Size() float64               { return m.size }
func (m *mockFace) private()                    {}
func (m *mockFace) HasGlyph(r rune) bool        { _, ok := m.glyphs[r]; return ok }
func (m *mockFace) Advance(text string) float64 { panic("not implemented") }
func (m *mockFace) Glyphs(text string) iter.Seq[Glyph] {
	return func(yield func(Glyph) bool) {
		x := 0.0
		for _, r := range text {
			adv, ok := m.glyphs[r]
			if !ok {
				adv = m.size // Default advance for missing glyphs
			}
			glyph := Glyph{
				Rune:    r,
				X:       x,
				OriginX: x,
				Advance: adv,
			}
			if !yield(glyph) {
				return
			}
			x += adv
		}
	}
}
func (m *mockFace) AppendGlyphs(dst []Glyph, text string) []Glyph {
	for glyph := range m.Glyphs(text) {
		dst = append(dst, glyph)
	}
	return dst
}

func TestNewMultiFace(t *testing.T) {
	face1 := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6, 'b': 7})
	face2 := newMockFace(12, DirectionLTR, map[rune]float64{'c': 8, 'd': 9})

	t.Run("valid faces", func(t *testing.T) {
		mf, err := NewMultiFace(face1, face2)
		if err != nil {
			t.Fatalf("NewMultiFace failed: %v", err)
		}
		if mf == nil {
			t.Fatal("NewMultiFace returned nil")
		}
		if len(mf.faces) != 2 {
			t.Errorf("expected 2 faces, got %d", len(mf.faces))
		}
		if mf.direction != DirectionLTR {
			t.Errorf("expected direction LTR, got %v", mf.direction)
		}
	})

	t.Run("empty faces", func(t *testing.T) {
		_, err := NewMultiFace()
		if err == nil {
			t.Fatal("expected error for empty faces")
		}
	})

	t.Run("mismatched directions", func(t *testing.T) {
		face3 := newMockFace(12, DirectionRTL, map[rune]float64{'x': 10})
		_, err := NewMultiFace(face1, face3)
		if err == nil {
			t.Fatal("expected error for mismatched directions")
		}
	})
}

func TestMultiFaceMetrics(t *testing.T) {
	face1 := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6})
	face2 := newMockFace(14, DirectionLTR, map[rune]float64{'b': 7})

	mf, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	metrics := mf.Metrics()
	expected := face1.Metrics()

	if metrics.Ascent != expected.Ascent {
		t.Errorf("Ascent: expected %f, got %f", expected.Ascent, metrics.Ascent)
	}
	if metrics.Descent != expected.Descent {
		t.Errorf("Descent: expected %f, got %f", expected.Descent, metrics.Descent)
	}
}

func TestMultiFaceHasGlyph(t *testing.T) {
	face1 := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6, 'b': 7})
	face2 := newMockFace(12, DirectionLTR, map[rune]float64{'c': 8, 'd': 9})

	mf, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	tests := []struct {
		rune     rune
		expected bool
	}{
		{'a', true},  // In face1
		{'b', true},  // In face1
		{'c', true},  // In face2
		{'d', true},  // In face2
		{'x', false}, // Not in any face
	}

	for _, tt := range tests {
		result := mf.HasGlyph(tt.rune)
		if result != tt.expected {
			t.Errorf("HasGlyph(%q): expected %v, got %v", tt.rune, tt.expected, result)
		}
	}
}

func TestMultiFaceAdvance(t *testing.T) {
	face1 := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6, 'b': 7})
	face2 := newMockFace(12, DirectionLTR, map[rune]float64{'c': 8, 'd': 9})

	mf, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	// "ab" should use face1: 6 + 7 = 13
	// "cd" should use face2: 8 + 9 = 17
	// "abcd" should use both: 6 + 7 + 8 + 9 = 30
	tests := []struct {
		text     string
		expected float64
	}{
		{"ab", 13},
		{"cd", 17},
		{"abcd", 30},
		{"a", 6},
		{"c", 8},
	}

	for _, tt := range tests {
		result := mf.Advance(tt.text)
		if result != tt.expected {
			t.Errorf("Advance(%q): expected %f, got %f", tt.text, tt.expected, result)
		}
	}
}

func TestMultiFaceGlyphs(t *testing.T) {
	face1 := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6, 'b': 7})
	face2 := newMockFace(12, DirectionLTR, map[rune]float64{'c': 8})

	mf, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	text := "abc"
	glyphs := make([]Glyph, 0, 3)
	for glyph := range mf.Glyphs(text) {
		glyphs = append(glyphs, glyph)
	}

	if len(glyphs) != 3 {
		t.Fatalf("expected 3 glyphs, got %d", len(glyphs))
	}

	// Check that positions are cumulative
	expectedX := []float64{0, 6, 13}
	expectedAdvance := []float64{6, 7, 8}

	for i, glyph := range glyphs {
		if glyph.X != expectedX[i] {
			t.Errorf("glyph %d: expected X=%f, got %f", i, expectedX[i], glyph.X)
		}
		if glyph.Advance != expectedAdvance[i] {
			t.Errorf("glyph %d: expected Advance=%f, got %f", i, expectedAdvance[i], glyph.Advance)
		}
	}
}

func TestMultiFaceAppendGlyphs(t *testing.T) {
	face1 := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6})
	face2 := newMockFace(12, DirectionLTR, map[rune]float64{'b': 7})

	mf, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	text := "ab"
	glyphs := mf.AppendGlyphs(nil, text)

	if len(glyphs) != 2 {
		t.Fatalf("expected 2 glyphs, got %d", len(glyphs))
	}

	if glyphs[0].Rune != 'a' || glyphs[1].Rune != 'b' {
		t.Errorf("unexpected runes: %q, %q", glyphs[0].Rune, glyphs[1].Rune)
	}
}

func TestMultiFaceDirection(t *testing.T) {
	face1 := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6})
	face2 := newMockFace(12, DirectionLTR, map[rune]float64{'b': 7})

	mf, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	if mf.Direction() != DirectionLTR {
		t.Errorf("expected DirectionLTR, got %v", mf.Direction())
	}
}

func TestMultiFaceSource(t *testing.T) {
	face1 := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6})

	mf, err := NewMultiFace(face1)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	if mf.Source() != nil {
		t.Errorf("expected nil Source for composite face, got %v", mf.Source())
	}
}

func TestMultiFaceSize(t *testing.T) {
	face1 := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6})
	face2 := newMockFace(14, DirectionLTR, map[rune]float64{'b': 7})

	mf, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	if mf.Size() != 12 {
		t.Errorf("expected size 12, got %f", mf.Size())
	}
}
