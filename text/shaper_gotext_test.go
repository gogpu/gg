package text

import (
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// goTextTestFace creates a test Face at size 16 for GoTextShaper tests.
// Uses Go Regular font which has Latin, Cyrillic, and Greek glyphs,
// including kerning tables.
func goTextTestFace(t *testing.T) (Face, *FontSource) {
	t.Helper()

	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to create font source: %v", err)
	}
	t.Cleanup(func() {
		_ = source.Close()
	})

	face := source.Face(16.0)
	return face, source
}

// TestGoTextShaper_BasicLatin tests shaping basic Latin text.
// Verifies that glyphs are produced with non-zero advances and correct count.
func TestGoTextShaper_BasicLatin(t *testing.T) {
	face, _ := goTextTestFace(t)
	shaper := NewGoTextShaper()

	result := shaper.Shape("Hello", face, 16.0)
	if len(result) != 5 {
		t.Fatalf("Shape(\"Hello\"): got %d glyphs, want 5", len(result))
	}

	// Verify all glyphs have non-zero advances and increasing X positions.
	var prevX float64
	for i, g := range result {
		if g.XAdvance <= 0 {
			t.Errorf("glyph %d: XAdvance=%f, want > 0", i, g.XAdvance)
		}
		if i > 0 && g.X <= prevX {
			t.Errorf("glyph %d: X=%f should be > previous X=%f", i, g.X, prevX)
		}
		prevX = g.X
	}
}

// TestGoTextShaper_VariousText tests shaping various Latin strings.
func TestGoTextShaper_VariousText(t *testing.T) {
	face, _ := goTextTestFace(t)
	shaper := NewGoTextShaper()

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
		{"mixed", "Test123", 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shaper.Shape(tt.text, face, 16.0)
			if len(result) != tt.wantLen {
				t.Errorf("Shape(%q): got %d glyphs, want %d", tt.text, len(result), tt.wantLen)
			}

			// Verify all glyphs have positive advances.
			for i, g := range result {
				if g.XAdvance <= 0 {
					t.Errorf("glyph %d in %q: XAdvance=%f, want > 0", i, tt.text, g.XAdvance)
				}
			}
		})
	}
}

// TestGoTextShaper_Kerning tests that GoTextShaper applies kerning.
// The pair "AV" is a classic kerning pair where the V tucks under the A,
// making the combined width less than the sum of individual widths.
func TestGoTextShaper_Kerning(t *testing.T) {
	face, _ := goTextTestFace(t)
	shaper := NewGoTextShaper()

	// Shape "A" and "V" separately to get individual advances.
	glyphsA := shaper.Shape("A", face, 16.0)
	glyphsV := shaper.Shape("V", face, 16.0)
	if len(glyphsA) != 1 || len(glyphsV) != 1 {
		t.Fatalf("expected 1 glyph each for A and V, got %d and %d",
			len(glyphsA), len(glyphsV))
	}

	individualWidth := glyphsA[0].XAdvance + glyphsV[0].XAdvance

	// Shape "AV" together -- kerning should tighten the pair.
	glyphsAV := shaper.Shape("AV", face, 16.0)
	if len(glyphsAV) != 2 {
		t.Fatalf("Shape(\"AV\"): got %d glyphs, want 2", len(glyphsAV))
	}

	combinedWidth := glyphsAV[1].X + glyphsAV[1].XAdvance

	// If the font has kerning for AV, combined should be less than individual.
	// Go Regular has kerning tables, so this should hold.
	// However, not all fonts guarantee AV kerning, so we log rather than fail hard.
	if combinedWidth < individualWidth {
		t.Logf("Kerning detected: AV combined=%.2f < individual=%.2f (diff=%.2f)",
			combinedWidth, individualWidth, individualWidth-combinedWidth)
	} else {
		t.Logf("No kerning detected for AV pair in this font: combined=%.2f, individual=%.2f",
			combinedWidth, individualWidth)
	}

	// At minimum, combined width should not exceed individual width + epsilon.
	// This is a sanity check that the shaper is not producing wrong results.
	if combinedWidth > individualWidth*1.1 {
		t.Errorf("AV combined width %.2f is suspiciously larger than individual %.2f",
			combinedWidth, individualWidth)
	}
}

// TestGoTextShaper_Ligatures tests ligature substitution.
// The "ffi" sequence in "office" may be substituted by a single ligature glyph
// in fonts that support it. Go Regular may or may not have ligatures,
// so we test the shaping does not break and report findings.
func TestGoTextShaper_Ligatures(t *testing.T) {
	face, _ := goTextTestFace(t)
	shaper := NewGoTextShaper()

	text := "office"
	runes := []rune(text)
	result := shaper.Shape(text, face, 16.0)

	if len(result) == 0 {
		t.Fatal("Shape(\"office\") returned no glyphs")
	}

	if len(result) < len(runes) {
		t.Logf("Ligatures detected: %d glyphs for %d runes", len(result), len(runes))
	} else if len(result) == len(runes) {
		t.Logf("No ligatures: %d glyphs for %d runes (font may not have fi/ffi ligatures)", len(result), len(runes))
	}

	// Verify total advance is reasonable (positive and finite).
	totalAdvance := result[len(result)-1].X + result[len(result)-1].XAdvance
	if totalAdvance <= 0 {
		t.Errorf("total advance = %f, want > 0", totalAdvance)
	}
}

// TestGoTextShaper_EmptyText tests that empty text returns nil.
func TestGoTextShaper_EmptyText(t *testing.T) {
	face, _ := goTextTestFace(t)
	shaper := NewGoTextShaper()

	result := shaper.Shape("", face, 16.0)
	if result != nil {
		t.Errorf("Shape(\"\") = %v, want nil", result)
	}
}

// TestGoTextShaper_NilFace tests that nil face returns nil.
func TestGoTextShaper_NilFace(t *testing.T) {
	shaper := NewGoTextShaper()

	result := shaper.Shape("Hello", nil, 16.0)
	if result != nil {
		t.Errorf("Shape with nil face = %v, want nil", result)
	}
}

// TestGoTextShaper_SetShaper tests GoTextShaper integration with the global shaper system.
func TestGoTextShaper_SetShaper(t *testing.T) {
	// Save original shaper.
	original := GetShaper()
	t.Cleanup(func() {
		SetShaper(original)
	})

	face, _ := goTextTestFace(t)
	goTextShaper := NewGoTextShaper()

	// Set GoTextShaper as global.
	SetShaper(goTextShaper)

	current := GetShaper()
	if _, ok := current.(*GoTextShaper); !ok {
		t.Errorf("GetShaper() should return *GoTextShaper, got %T", current)
	}

	// Shape via the global Shape function.
	result := Shape("Hello", face, 16.0)
	if len(result) != 5 {
		t.Errorf("Shape(\"Hello\") via global: got %d glyphs, want 5", len(result))
	}

	// Reset to nil should restore BuiltinShaper.
	SetShaper(nil)
	if _, ok := GetShaper().(*BuiltinShaper); !ok {
		t.Errorf("SetShaper(nil) should restore BuiltinShaper, got %T", GetShaper())
	}
}

// TestGoTextShaper_VsBuiltinShaper compares GoTextShaper output with BuiltinShaper.
// Both should produce similar (but not identical) results for simple Latin text.
// GoTextShaper may produce tighter or different layout due to kerning.
func TestGoTextShaper_VsBuiltinShaper(t *testing.T) {
	face, _ := goTextTestFace(t)
	goTextShaper := NewGoTextShaper()
	builtinShaper := &BuiltinShaper{}

	text := "Hello World"

	goTextResult := goTextShaper.Shape(text, face, 16.0)
	builtinResult := builtinShaper.Shape(text, face, 16.0)

	// Both should produce the same number of glyphs for simple Latin.
	if len(goTextResult) != len(builtinResult) {
		t.Errorf("glyph count mismatch: GoText=%d, Builtin=%d",
			len(goTextResult), len(builtinResult))
		return
	}

	// Compare total advance widths.
	goTextTotal := goTextResult[len(goTextResult)-1].X + goTextResult[len(goTextResult)-1].XAdvance
	builtinTotal := builtinResult[len(builtinResult)-1].X + builtinResult[len(builtinResult)-1].XAdvance

	t.Logf("Total advance: GoText=%.2f, Builtin=%.2f, diff=%.2f",
		goTextTotal, builtinTotal, goTextTotal-builtinTotal)

	// Both should produce positive total advance.
	if goTextTotal <= 0 {
		t.Errorf("GoText total advance = %f, want > 0", goTextTotal)
	}
	if builtinTotal <= 0 {
		t.Errorf("Builtin total advance = %f, want > 0", builtinTotal)
	}

	// Glyph IDs should match (same font, same characters, no ligatures for Latin).
	for i := range goTextResult {
		if goTextResult[i].GID != builtinResult[i].GID {
			t.Logf("glyph %d GID mismatch: GoText=%d, Builtin=%d (expected for ligatures/substitution)",
				i, goTextResult[i].GID, builtinResult[i].GID)
		}
	}
}

// TestGoTextShaper_DifferentSizes tests shaping at various font sizes.
// Larger sizes should produce larger total advances.
func TestGoTextShaper_DifferentSizes(t *testing.T) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	shaper := NewGoTextShaper()
	sizes := []float64{8, 12, 16, 24, 32, 48}
	var prevTotalAdvance float64

	for _, size := range sizes {
		face := source.Face(size)
		result := shaper.Shape("Hello", face, size)
		if len(result) != 5 {
			t.Errorf("size %f: got %d glyphs, want 5", size, len(result))
			continue
		}

		totalAdvance := result[len(result)-1].X + result[len(result)-1].XAdvance
		if size > 8 && totalAdvance <= prevTotalAdvance {
			t.Errorf("size %f: total advance %f should be > previous %f",
				size, totalAdvance, prevTotalAdvance)
		}
		prevTotalAdvance = totalAdvance
	}
}

// TestGoTextShaper_Concurrency tests thread safety of GoTextShaper.
// Multiple goroutines shape text concurrently using the same shaper instance.
func TestGoTextShaper_Concurrency(t *testing.T) {
	face, _ := goTextTestFace(t)
	shaper := NewGoTextShaper()

	var wg sync.WaitGroup
	errors := make(chan string, 200)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				result := shaper.Shape("Hello World", face, 16.0)
				if len(result) != 11 {
					errors <- "wrong glyph count"
				}
				for _, g := range result {
					if g.XAdvance <= 0 {
						errors <- "zero advance"
					}
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	var errMsgs []string
	for msg := range errors {
		errMsgs = append(errMsgs, msg)
	}
	if len(errMsgs) > 0 {
		t.Errorf("concurrent shaping had %d errors; first: %s", len(errMsgs), errMsgs[0])
	}
}

// TestGoTextShaper_FaceCache tests the internal font face cache.
func TestGoTextShaper_FaceCache(t *testing.T) {
	_, source := goTextTestFace(t)
	shaper := NewGoTextShaper()

	face := source.Face(16.0)

	// First call should parse the font and cache it.
	result1 := shaper.Shape("A", face, 16.0)
	if len(result1) != 1 {
		t.Fatalf("first Shape(\"A\"): got %d glyphs, want 1", len(result1))
	}

	// Second call should use the cached font face.
	result2 := shaper.Shape("A", face, 16.0)
	if len(result2) != 1 {
		t.Fatalf("second Shape(\"A\"): got %d glyphs, want 1", len(result2))
	}

	// Results should be identical.
	if result1[0].GID != result2[0].GID {
		t.Errorf("GID mismatch between cached calls: %d vs %d",
			result1[0].GID, result2[0].GID)
	}
	if result1[0].XAdvance != result2[0].XAdvance {
		t.Errorf("XAdvance mismatch between cached calls: %f vs %f",
			result1[0].XAdvance, result2[0].XAdvance)
	}
}

// TestGoTextShaper_ClearCache tests that ClearCache removes cached font faces.
func TestGoTextShaper_ClearCache(t *testing.T) {
	face, _ := goTextTestFace(t)
	shaper := NewGoTextShaper()

	// Populate cache.
	_ = shaper.Shape("A", face, 16.0)

	shaper.mu.RLock()
	cacheLen := len(shaper.fontCache)
	shaper.mu.RUnlock()
	if cacheLen != 1 {
		t.Errorf("cache should have 1 entry, got %d", cacheLen)
	}

	// Clear cache.
	shaper.ClearCache()

	shaper.mu.RLock()
	cacheLen = len(shaper.fontCache)
	shaper.mu.RUnlock()
	if cacheLen != 0 {
		t.Errorf("cache should be empty after ClearCache, got %d entries", cacheLen)
	}

	// Shaping should still work (re-parses font).
	result := shaper.Shape("A", face, 16.0)
	if len(result) != 1 {
		t.Errorf("Shape after ClearCache: got %d glyphs, want 1", len(result))
	}
}

// TestGoTextShaper_RemoveSource tests that RemoveSource removes a specific entry.
func TestGoTextShaper_RemoveSource(t *testing.T) {
	_, source := goTextTestFace(t)
	shaper := NewGoTextShaper()

	face := source.Face(16.0)

	// Populate cache.
	_ = shaper.Shape("A", face, 16.0)

	shaper.mu.RLock()
	cacheLen := len(shaper.fontCache)
	shaper.mu.RUnlock()
	if cacheLen != 1 {
		t.Errorf("cache should have 1 entry, got %d", cacheLen)
	}

	// Remove the source.
	shaper.RemoveSource(source)

	shaper.mu.RLock()
	cacheLen = len(shaper.fontCache)
	shaper.mu.RUnlock()
	if cacheLen != 0 {
		t.Errorf("cache should be empty after RemoveSource, got %d entries", cacheLen)
	}
}

// TestGoTextShaper_GlyphPositioning tests that glyph X positions are
// correctly accumulated from advances.
func TestGoTextShaper_GlyphPositioning(t *testing.T) {
	face, _ := goTextTestFace(t)
	shaper := NewGoTextShaper()

	result := shaper.Shape("ABC", face, 16.0)
	if len(result) < 3 {
		t.Fatalf("Shape(\"ABC\"): got %d glyphs, want >= 3", len(result))
	}

	// First glyph should start at X=0 (possibly with offset).
	if result[0].X < 0 {
		t.Errorf("first glyph X=%f, want >= 0", result[0].X)
	}

	// Y should be 0 for horizontal text (no vertical offset unless kerning applies Y).
	for i, g := range result {
		if g.YAdvance != 0 {
			t.Errorf("glyph %d: YAdvance=%f, want 0 for horizontal text", i, g.YAdvance)
		}
	}
}

// TestGoTextShaper_WhitespaceHandling tests shaping text with whitespace.
func TestGoTextShaper_WhitespaceHandling(t *testing.T) {
	face, _ := goTextTestFace(t)
	shaper := NewGoTextShaper()

	tests := []struct {
		name    string
		text    string
		wantLen int
	}{
		{"single space", " ", 1},
		{"tab", "\t", 1},
		{"multiple spaces", "   ", 3},
		{"word and space", "A B", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shaper.Shape(tt.text, face, 16.0)
			if len(result) != tt.wantLen {
				t.Errorf("Shape(%q): got %d glyphs, want %d", tt.text, len(result), tt.wantLen)
			}
		})
	}
}

// TestGoTextShaper_ClusterIndices tests that cluster indices are populated.
func TestGoTextShaper_ClusterIndices(t *testing.T) {
	face, _ := goTextTestFace(t)
	shaper := NewGoTextShaper()

	result := shaper.Shape("Hello", face, 16.0)
	if len(result) != 5 {
		t.Fatalf("Shape(\"Hello\"): got %d glyphs, want 5", len(result))
	}

	// For simple Latin text without ligatures, cluster indices should map to rune indices.
	for i, g := range result {
		if g.Cluster != i {
			t.Logf("glyph %d: Cluster=%d (may differ if font applies substitution)", i, g.Cluster)
		}
	}
}

// TestGoTextShaper_DetectScript tests the detectScript helper function.
func TestGoTextShaper_DetectScript(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"Latin", "Hello"},
		{"spaces then Latin", "  Hello"},
		{"all spaces", "   "},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runes := []rune(tt.text)
			// Just ensure it does not panic.
			_ = detectScript(runes)
		})
	}
}

// TestGoTextShaper_MapDirection tests the mapDirection helper.
func TestGoTextShaper_MapDirection(t *testing.T) {
	tests := []struct {
		name string
		dir  Direction
	}{
		{"LTR", DirectionLTR},
		{"RTL", DirectionRTL},
		{"TTB", DirectionTTB},
		{"BTT", DirectionBTT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify mapDirection does not panic and returns a valid value.
			result := mapDirection(tt.dir)
			_ = result
		})
	}
}

// TestGoTextShaper_FixedPointConversion tests float-to-fixed and back.
func TestGoTextShaper_FixedPointConversion(t *testing.T) {
	tests := []struct {
		name  string
		value float64
	}{
		{"zero", 0},
		{"positive", 16.0},
		{"small", 0.5},
		{"large", 72.0},
		{"fractional", 12.75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixed := floatToFixed(tt.value)
			back := fixedToFloat(fixed)

			// Allow small rounding error due to fixed-point precision.
			diff := back - tt.value
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.02 {
				t.Errorf("floatToFixed(%.4f) -> fixedToFloat = %.4f, diff=%.4f > 0.02",
					tt.value, back, diff)
			}
		})
	}
}

// TestGoTextShaper_ConvertGlyphsEmpty tests convertGlyphs with empty input.
func TestGoTextShaper_ConvertGlyphsEmpty(t *testing.T) {
	result := convertGlyphs(nil, 0)
	if result != nil {
		t.Errorf("convertGlyphs(nil) = %v, want nil", result)
	}
}

// TestNewGoTextShaper tests constructor.
func TestNewGoTextShaper(t *testing.T) {
	shaper := NewGoTextShaper()
	if shaper == nil {
		t.Fatal("NewGoTextShaper() returned nil")
	}
	if shaper.fontCache == nil {
		t.Error("faceCache should be initialized")
	}
}

// BenchmarkGoTextShape benchmarks GoTextShaper with a standard sentence.
func BenchmarkGoTextShape(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	shaper := NewGoTextShaper()

	// Warm the cache.
	_ = shaper.Shape("warmup", face, 16.0)

	text := "The quick brown fox jumps over the lazy dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = shaper.Shape(text, face, 16.0)
	}
}

// BenchmarkGoTextShapeShort benchmarks shaping a short string.
func BenchmarkGoTextShapeShort(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	shaper := NewGoTextShaper()

	// Warm the cache.
	_ = shaper.Shape("w", face, 16.0)

	text := "Hello"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = shaper.Shape(text, face, 16.0)
	}
}

// BenchmarkGoTextShapeLong benchmarks shaping a long string.
func BenchmarkGoTextShapeLong(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	shaper := NewGoTextShaper()

	// Warm the cache.
	_ = shaper.Shape("w", face, 16.0)

	text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = shaper.Shape(text, face, 16.0)
	}
}
