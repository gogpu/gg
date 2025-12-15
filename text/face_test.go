package text

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// loadTestFont loads a test font for testing.
func loadTestFont(t *testing.T) *FontSource {
	t.Helper()

	// Use embedded Go font
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}

	return source
}

// TestFaceMetrics tests Face.Metrics.
func TestFaceMetrics(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		if err := source.Close(); err != nil {
			t.Errorf("failed to close font source: %v", err)
		}
	}()

	tests := []struct {
		name string
		size float64
	}{
		{"size 12", 12.0},
		{"size 16", 16.0},
		{"size 24", 24.0},
		{"size 48", 48.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			face := source.Face(tt.size)

			metrics := face.Metrics()

			// Verify metrics are non-zero
			if metrics.Ascent <= 0 {
				t.Errorf("Ascent should be positive, got %f", metrics.Ascent)
			}
			if metrics.Descent <= 0 {
				t.Errorf("Descent should be positive, got %f", metrics.Descent)
			}
			if metrics.LineGap < 0 {
				t.Errorf("LineGap should be non-negative, got %f", metrics.LineGap)
			}

			// Verify LineHeight is the sum
			expectedLineHeight := metrics.Ascent + metrics.Descent + metrics.LineGap
			if metrics.LineHeight() != expectedLineHeight {
				t.Errorf("LineHeight() = %f, want %f", metrics.LineHeight(), expectedLineHeight)
			}

			// Metrics should scale with size
			if tt.size == 24.0 {
				face12 := source.Face(12.0)
				metrics12 := face12.Metrics()

				// At 24pt, metrics should be approximately 2x of 12pt
				ratio := metrics.Ascent / metrics12.Ascent
				if ratio < 1.8 || ratio > 2.2 {
					t.Errorf("Metrics scaling incorrect: ratio = %f, want ~2.0", ratio)
				}
			}
		})
	}
}

// TestFaceAdvance tests Face.Advance.
func TestFaceAdvance(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)

	tests := []struct {
		name string
		text string
	}{
		{"empty string", ""},
		{"single char", "A"},
		{"word", "Hello"},
		{"sentence", "The quick brown fox"},
		{"unicode", "Hello 世界"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			advance := face.Advance(tt.text)

			if tt.text == "" {
				if advance != 0 {
					t.Errorf("Advance() = %f, want 0 for empty string", advance)
				}
				return
			}

			// Advance should be positive for non-empty text
			if advance <= 0 {
				t.Errorf("Advance() = %f, want positive value for %q", advance, tt.text)
			}

			// Advance should grow with text length
			if len(tt.text) > 1 {
				singleAdvance := face.Advance(string(tt.text[0]))
				if advance <= singleAdvance {
					t.Errorf("Advance(%q) = %f should be > Advance(%q) = %f",
						tt.text, advance, string(tt.text[0]), singleAdvance)
				}
			}
		})
	}
}

// TestFaceAdvanceVsGlyphs tests that Advance matches sum of glyph advances.
func TestFaceAdvanceVsGlyphs(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)

	tests := []string{
		"Hello",
		"World",
		"Hello World",
		"ABC123",
	}

	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			advance := face.Advance(text)

			// Sum glyph advances
			glyphAdvanceSum := 0.0
			for glyph := range face.Glyphs(text) {
				glyphAdvanceSum += glyph.Advance
			}

			// Should match (with small floating point tolerance)
			diff := advance - glyphAdvanceSum
			if diff < -0.01 || diff > 0.01 {
				t.Errorf("Advance() = %f, sum of glyph advances = %f, diff = %f",
					advance, glyphAdvanceSum, diff)
			}
		})
	}
}

// TestFaceHasGlyph tests Face.HasGlyph.
func TestFaceHasGlyph(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)

	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"ASCII letter", 'A', true},
		{"ASCII digit", '5', true},
		{"space", ' ', true},
		{"period", '.', true},
		{"common punctuation", '!', true},
		// Note: goregular may not have all Unicode characters
		{"basic latin", 'Z', true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := face.HasGlyph(tt.r)
			if got != tt.want {
				t.Errorf("HasGlyph(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

// TestFaceGlyphs tests Face.Glyphs iterator.
func TestFaceGlyphs(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)

	tests := []struct {
		name      string
		text      string
		wantCount int
	}{
		{"empty", "", 0},
		{"single char", "A", 1},
		{"word", "Hello", 5},
		{"sentence", "Hi there", 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := 0
			prevX := 0.0

			for glyph := range face.Glyphs(tt.text) {
				count++

				// Verify glyph fields
				if glyph.Rune == 0 && tt.text != "" {
					t.Error("Glyph.Rune is zero")
				}

				// X should increase (LTR text)
				if count > 1 && glyph.X <= prevX {
					t.Errorf("Glyph.X not increasing: prev=%f, current=%f", prevX, glyph.X)
				}
				prevX = glyph.X

				// Advance should be positive for most glyphs
				if glyph.Advance < 0 {
					t.Errorf("Glyph.Advance is negative: %f", glyph.Advance)
				}

				// OriginX should match X for simple shaping
				if glyph.OriginX != glyph.X {
					t.Errorf("Glyph.OriginX (%f) != Glyph.X (%f)", glyph.OriginX, glyph.X)
				}

				// Cluster should be valid
				if glyph.Cluster < 0 {
					t.Errorf("Glyph.Cluster is negative: %d", glyph.Cluster)
				}
			}

			if count != tt.wantCount {
				t.Errorf("Glyphs() returned %d glyphs, want %d", count, tt.wantCount)
			}
		})
	}
}

// TestFaceGlyphsEarlyExit tests that Glyphs iterator respects early exit.
func TestFaceGlyphsEarlyExit(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	text := "Hello World"

	count := 0
	for range face.Glyphs(text) {
		count++
		if count == 3 {
			break
		}
	}

	if count != 3 {
		t.Errorf("Expected to break after 3 iterations, got %d", count)
	}
}

// TestFaceAppendGlyphs tests Face.AppendGlyphs.
func TestFaceAppendGlyphs(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)

	tests := []struct {
		name    string
		initial []Glyph
		text    string
		wantLen int
		wantCap int // expected capacity (at least)
	}{
		{
			name:    "nil slice",
			initial: nil,
			text:    "Hi",
			wantLen: 2,
			wantCap: 2,
		},
		{
			name:    "empty slice",
			initial: []Glyph{},
			text:    "ABC",
			wantLen: 3,
			wantCap: 3,
		},
		{
			name:    "with capacity",
			initial: make([]Glyph, 0, 10),
			text:    "Test",
			wantLen: 4,
			wantCap: 10,
		},
		{
			name: "append to existing",
			initial: []Glyph{
				{Rune: 'X', GID: 1, X: 0, Advance: 10},
			},
			text:    "AB",
			wantLen: 3,
			wantCap: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := face.AppendGlyphs(tt.initial, tt.text)

			if len(result) != tt.wantLen {
				t.Errorf("len(result) = %d, want %d", len(result), tt.wantLen)
			}

			if cap(result) < tt.wantCap {
				t.Errorf("cap(result) = %d, want at least %d", cap(result), tt.wantCap)
			}

			// If we had initial glyphs, verify they're still there
			if len(tt.initial) > 0 {
				if result[0].Rune != tt.initial[0].Rune {
					t.Errorf("Initial glyph was modified")
				}
			}
		})
	}
}

// TestFaceAppendGlyphsMatchesGlyphs tests that AppendGlyphs matches Glyphs iterator.
func TestFaceAppendGlyphsMatchesGlyphs(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	text := "Hello World"

	// Get glyphs via AppendGlyphs
	appendResult := face.AppendGlyphs(nil, text)

	// Get glyphs via iterator
	iterResult := make([]Glyph, 0, len(text))
	for glyph := range face.Glyphs(text) {
		iterResult = append(iterResult, glyph)
	}

	// Should have same length
	if len(appendResult) != len(iterResult) {
		t.Fatalf("AppendGlyphs returned %d glyphs, Glyphs returned %d",
			len(appendResult), len(iterResult))
	}

	// Compare each glyph
	for i := range appendResult {
		a := appendResult[i]
		b := iterResult[i]

		if a.Rune != b.Rune {
			t.Errorf("Glyph %d: Rune mismatch: %q vs %q", i, a.Rune, b.Rune)
		}
		if a.GID != b.GID {
			t.Errorf("Glyph %d: GID mismatch: %d vs %d", i, a.GID, b.GID)
		}
		if a.X != b.X {
			t.Errorf("Glyph %d: X mismatch: %f vs %f", i, a.X, b.X)
		}
		if a.Advance != b.Advance {
			t.Errorf("Glyph %d: Advance mismatch: %f vs %f", i, a.Advance, b.Advance)
		}
	}
}

// TestFaceDirection tests Face.Direction.
func TestFaceDirection(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	tests := []struct {
		name    string
		option  FaceOption
		wantDir Direction
	}{
		{"default", nil, DirectionLTR},
		{"explicit LTR", WithDirection(DirectionLTR), DirectionLTR},
		{"RTL", WithDirection(DirectionRTL), DirectionRTL},
		{"TTB", WithDirection(DirectionTTB), DirectionTTB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var face Face
			if tt.option == nil {
				face = source.Face(16.0)
			} else {
				face = source.Face(16.0, tt.option)
			}

			if got := face.Direction(); got != tt.wantDir {
				t.Errorf("Direction() = %v, want %v", got, tt.wantDir)
			}
		})
	}
}

// TestFaceSource tests Face.Source.
func TestFaceSource(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)

	if got := face.Source(); got != source {
		t.Errorf("Source() returned different source: %p vs %p", got, source)
	}
}

// TestFaceSize tests Face.Size.
func TestFaceSize(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	tests := []float64{12.0, 16.0, 24.0, 48.0, 72.0}

	for _, size := range tests {
		t.Run("", func(t *testing.T) {
			face := source.Face(size)

			if got := face.Size(); got != size {
				t.Errorf("Size() = %f, want %f", got, size)
			}
		})
	}
}

// TestFaceMultipleFaces tests creating multiple faces from one source.
func TestFaceMultipleFaces(t *testing.T) {
	source := loadTestFont(t)
	defer func() {
		_ = source.Close()
	}()

	// Create multiple faces with different sizes
	face12 := source.Face(12.0)
	face16 := source.Face(16.0)
	face24 := source.Face(24.0)

	// All should have correct sizes
	if face12.Size() != 12.0 {
		t.Errorf("face12.Size() = %f, want 12.0", face12.Size())
	}
	if face16.Size() != 16.0 {
		t.Errorf("face16.Size() = %f, want 16.0", face16.Size())
	}
	if face24.Size() != 24.0 {
		t.Errorf("face24.Size() = %f, want 24.0", face24.Size())
	}

	// All should share the same source
	if face12.Source() != source {
		t.Error("face12 has different source")
	}
	if face16.Source() != source {
		t.Error("face16 has different source")
	}
	if face24.Source() != source {
		t.Error("face24 has different source")
	}

	// Metrics should scale
	metrics12 := face12.Metrics()
	metrics24 := face24.Metrics()

	ratio := metrics24.Ascent / metrics12.Ascent
	if ratio < 1.8 || ratio > 2.2 {
		t.Errorf("Metrics scaling incorrect: ratio = %f, want ~2.0", ratio)
	}
}
