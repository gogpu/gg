package text

import (
	"testing"
)

func TestUnicodeRangeContains(t *testing.T) {
	tests := []struct {
		name     string
		ur       UnicodeRange
		rune     rune
		expected bool
	}{
		{"start boundary", UnicodeRange{0x0000, 0x007F}, 0x0000, true},
		{"end boundary", UnicodeRange{0x0000, 0x007F}, 0x007F, true},
		{"inside range", UnicodeRange{0x0000, 0x007F}, 0x0041, true},
		{"before range", UnicodeRange{0x0000, 0x007F}, 0xFFFF, false},
		{"after range", UnicodeRange{0x0100, 0x017F}, 0x0050, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ur.Contains(tt.rune)
			if result != tt.expected {
				t.Errorf("Contains(%U): expected %v, got %v", tt.rune, tt.expected, result)
			}
		})
	}
}

func TestCommonUnicodeRanges(t *testing.T) {
	tests := []struct {
		name  string
		ur    UnicodeRange
		rune  rune
		check bool
	}{
		{"BasicLatin A", RangeBasicLatin, 'A', true},
		{"BasicLatin z", RangeBasicLatin, 'z', true},
		{"BasicLatin beyond", RangeBasicLatin, 0x0080, false},
		{"Cyrillic А", RangeCyrillic, 'А', true},
		{"Cyrillic я", RangeCyrillic, 'я', true},
		{"Greek α", RangeGreek, 'α', true},
		{"CJK 中", RangeCJKUnified, '中', true},
		{"Hiragana あ", RangeHiragana, 'あ', true},
		{"Katakana ア", RangeKatakana, 'ア', true},
		{"Emoji smile", RangeEmoji, '😀', true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ur.Contains(tt.rune)
			if result != tt.check {
				t.Errorf("%s.Contains(%U): expected %v, got %v", tt.name, tt.rune, tt.check, result)
			}
		})
	}
}

func TestNewFilteredFace(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{
		'a': 6, 'b': 7, 'А': 8, 'я': 9,
	})

	t.Run("no ranges", func(t *testing.T) {
		ff := NewFilteredFace(face)
		if ff == nil {
			t.Fatal("NewFilteredFace returned nil")
		}
		if len(ff.ranges) != 0 {
			t.Errorf("expected 0 ranges, got %d", len(ff.ranges))
		}
	})

	t.Run("with ranges", func(t *testing.T) {
		ff := NewFilteredFace(face, RangeBasicLatin, RangeCyrillic)
		if ff == nil {
			t.Fatal("NewFilteredFace returned nil")
		}
		if len(ff.ranges) != 2 {
			t.Errorf("expected 2 ranges, got %d", len(ff.ranges))
		}
	})
}

func TestFilteredFaceHasGlyph(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{
		'a': 6, 'b': 7, 'А': 8, 'я': 9, '中': 10,
	})

	// Filter to only BasicLatin
	ff := NewFilteredFace(face, RangeBasicLatin)

	tests := []struct {
		rune     rune
		expected bool
	}{
		{'a', true},  // In BasicLatin and face has it
		{'b', true},  // In BasicLatin and face has it
		{'А', false}, // Not in BasicLatin (Cyrillic)
		{'я', false}, // Not in BasicLatin (Cyrillic)
		{'中', false}, // Not in BasicLatin (CJK)
		{'z', false}, // In BasicLatin but face doesn't have it
	}

	for _, tt := range tests {
		result := ff.HasGlyph(tt.rune)
		if result != tt.expected {
			t.Errorf("HasGlyph(%q): expected %v, got %v", tt.rune, tt.expected, result)
		}
	}
}

func TestFilteredFaceNoRanges(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{
		'a': 6, 'А': 8,
	})

	// No filtering
	ff := NewFilteredFace(face)

	tests := []struct {
		rune     rune
		expected bool
	}{
		{'a', true},  // Face has it
		{'А', true},  // Face has it
		{'x', false}, // Face doesn't have it
	}

	for _, tt := range tests {
		result := ff.HasGlyph(tt.rune)
		if result != tt.expected {
			t.Errorf("HasGlyph(%q): expected %v, got %v", tt.rune, tt.expected, result)
		}
	}
}

func TestFilteredFaceMultipleRanges(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{
		'a': 6, 'А': 8, '中': 10,
	})

	// Filter to BasicLatin and Cyrillic
	ff := NewFilteredFace(face, RangeBasicLatin, RangeCyrillic)

	tests := []struct {
		rune     rune
		expected bool
	}{
		{'a', true},  // In BasicLatin
		{'А', true},  // In Cyrillic
		{'中', false}, // Not in either range
	}

	for _, tt := range tests {
		result := ff.HasGlyph(tt.rune)
		if result != tt.expected {
			t.Errorf("HasGlyph(%q): expected %v, got %v", tt.rune, tt.expected, result)
		}
	}
}

func TestFilteredFaceAdvance(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{
		'a': 6, 'b': 7, 'А': 8,
	})

	// Filter to BasicLatin only
	ff := NewFilteredFace(face, RangeBasicLatin)

	tests := []struct {
		text     string
		expected float64
	}{
		{"ab", 13}, // Both in range: 6 + 7
		{"aА", 6},  // Only 'a' in range
		{"Аb", 7},  // Only 'b' in range
		{"АБ", 0},  // Neither in range
		{"a", 6},   // Single char in range
		{"", 0},    // Empty string
	}

	for _, tt := range tests {
		result := ff.Advance(tt.text)
		if result != tt.expected {
			t.Errorf("Advance(%q): expected %f, got %f", tt.text, tt.expected, result)
		}
	}
}

func TestFilteredFaceGlyphs(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{
		'a': 6, 'b': 7, 'А': 8,
	})

	// Filter to BasicLatin only
	ff := NewFilteredFace(face, RangeBasicLatin)

	text := "aАb" // Only 'a' and 'b' should be yielded
	glyphs := make([]Glyph, 0, 3)
	for glyph := range ff.Glyphs(text) {
		glyphs = append(glyphs, glyph)
	}

	if len(glyphs) != 2 {
		t.Fatalf("expected 2 glyphs, got %d", len(glyphs))
	}

	if glyphs[0].Rune != 'a' {
		t.Errorf("glyph 0: expected 'a', got %q", glyphs[0].Rune)
	}
	if glyphs[1].Rune != 'b' {
		t.Errorf("glyph 1: expected 'b', got %q", glyphs[1].Rune)
	}
}

func TestFilteredFaceAppendGlyphs(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{
		'a': 6, 'b': 7, 'А': 8,
	})

	// Filter to BasicLatin only
	ff := NewFilteredFace(face, RangeBasicLatin)

	text := "aАb"
	glyphs := ff.AppendGlyphs(nil, text)

	if len(glyphs) != 2 {
		t.Fatalf("expected 2 glyphs, got %d", len(glyphs))
	}

	if glyphs[0].Rune != 'a' || glyphs[1].Rune != 'b' {
		t.Errorf("unexpected runes: %q, %q", glyphs[0].Rune, glyphs[1].Rune)
	}
}

func TestFilteredFaceMetrics(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6})
	ff := NewFilteredFace(face, RangeBasicLatin)

	metrics := ff.Metrics()
	expected := face.Metrics()

	if metrics.Ascent != expected.Ascent {
		t.Errorf("Ascent: expected %f, got %f", expected.Ascent, metrics.Ascent)
	}
	if metrics.Descent != expected.Descent {
		t.Errorf("Descent: expected %f, got %f", expected.Descent, metrics.Descent)
	}
}

func TestFilteredFaceDirection(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6})
	ff := NewFilteredFace(face, RangeBasicLatin)

	if ff.Direction() != DirectionLTR {
		t.Errorf("expected DirectionLTR, got %v", ff.Direction())
	}
}

func TestFilteredFaceSource(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6})
	ff := NewFilteredFace(face, RangeBasicLatin)

	if ff.Source() != nil {
		t.Errorf("expected nil Source (delegated to wrapped face), got %v", ff.Source())
	}
}

func TestFilteredFaceSize(t *testing.T) {
	face := newMockFace(12, DirectionLTR, map[rune]float64{'a': 6})
	ff := NewFilteredFace(face, RangeBasicLatin)

	if ff.Size() != 12 {
		t.Errorf("expected size 12, got %f", ff.Size())
	}
}

func TestFilteredFaceWithMultiFace(t *testing.T) {
	// Create a MultiFace with Latin and Cyrillic coverage
	latinFace := newMockFace(12, DirectionLTR, map[rune]float64{
		'a': 6, 'b': 7,
	})
	cyrillicFace := newMockFace(12, DirectionLTR, map[rune]float64{
		'А': 8, 'Б': 9,
	})

	mf, err := NewMultiFace(latinFace, cyrillicFace)
	if err != nil {
		t.Fatalf("NewMultiFace failed: %v", err)
	}

	// Filter to only Latin
	ff := NewFilteredFace(mf, RangeBasicLatin)

	// Should only render Latin characters
	text := "aАbБ"
	glyphs := make([]Glyph, 0, 4)
	for glyph := range ff.Glyphs(text) {
		glyphs = append(glyphs, glyph)
	}

	if len(glyphs) != 2 {
		t.Fatalf("expected 2 glyphs (a, b), got %d", len(glyphs))
	}

	if glyphs[0].Rune != 'a' || glyphs[1].Rune != 'b' {
		t.Errorf("unexpected runes: %q, %q", glyphs[0].Rune, glyphs[1].Rune)
	}
}
