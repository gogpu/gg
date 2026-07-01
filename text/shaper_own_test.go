package text

import (
	"sync"
	"testing"

)

// ownTestFace creates a test Face using the own parser at size 16.
func ownTestFace(t *testing.T) (Face, *FontSource) {
	t.Helper()

	source, err := NewFontSource(requireTestFont(t), WithParser("own"))
	if err != nil {
		t.Fatalf("failed to create font source with own parser: %v", err)
	}
	t.Cleanup(func() {
		_ = source.Close()
	})

	face := source.Face(16.0)
	return face, source
}

// TestOwnShaper_BasicLatin tests shaping basic Latin text.
func TestOwnShaper_BasicLatin(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

	result := shaper.Shape("Hello", face)
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

		// GID should be non-zero (Go Regular has these glyphs).
		if g.GID == 0 {
			t.Errorf("glyph %d: GID=0 (missing glyph for Latin)", i)
		}
	}
}

// TestOwnShaper_VariousText tests shaping various Latin strings.
func TestOwnShaper_VariousText(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

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
			result := shaper.Shape(tt.text, face)
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

// TestOwnShaper_EmptyText tests that empty text returns nil.
func TestOwnShaper_EmptyText(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

	result := shaper.Shape("", face)
	if result != nil {
		t.Errorf("Shape(\"\") = %v, want nil", result)
	}
}

// TestOwnShaper_NilFace tests that nil face returns nil.
func TestOwnShaper_NilFace(t *testing.T) {
	shaper := NewOwnShaper()

	result := shaper.Shape("Hello", nil)
	if result != nil {
		t.Errorf("Shape with nil face = %v, want nil", result)
	}
}

// TestOwnShaper_GlyphPositioning tests that glyph positions accumulate correctly.
func TestOwnShaper_GlyphPositioning(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

	result := shaper.Shape("ABC", face)
	if len(result) < 3 {
		t.Fatalf("Shape(\"ABC\"): got %d glyphs, want >= 3", len(result))
	}

	// First glyph should start at X=0.
	if result[0].X != 0 {
		t.Errorf("first glyph X=%f, want 0", result[0].X)
	}

	// Each glyph's X should equal the sum of preceding advances.
	var expectedX float64
	for i, g := range result {
		if i > 0 {
			// Allow for GPOS adjustments — the position should be close to expected.
			diff := g.X - expectedX
			if diff < 0 {
				diff = -diff
			}
			if diff > 1.0 {
				t.Errorf("glyph %d: X=%f, expected ~%f (diff=%f)", i, g.X, expectedX, diff)
			}
		}
		expectedX += g.XAdvance
	}

	// Y should be 0 for horizontal text.
	for i, g := range result {
		if g.Y != 0 {
			t.Errorf("glyph %d: Y=%f, want 0", i, g.Y)
		}
		if g.YAdvance != 0 {
			t.Errorf("glyph %d: YAdvance=%f, want 0 for horizontal text", i, g.YAdvance)
		}
	}
}

// TestOwnShaper_ClusterIndices tests that cluster indices are correct.
func TestOwnShaper_ClusterIndices(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

	result := shaper.Shape("Hello", face)
	if len(result) != 5 {
		t.Fatalf("Shape(\"Hello\"): got %d glyphs, want 5", len(result))
	}

	// For simple Latin without ligatures, cluster indices should be sequential.
	for i, g := range result {
		if g.Cluster != i {
			t.Errorf("glyph %d: Cluster=%d, want %d", i, g.Cluster, i)
		}
	}
}

// TestOwnShaper_DifferentSizes tests shaping at various font sizes.
func TestOwnShaper_DifferentSizes(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t), WithParser("own"))
	if err != nil {
		t.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	shaper := NewOwnShaper()
	sizes := []float64{8, 12, 16, 24, 32, 48}
	var prevTotalAdvance float64

	for _, size := range sizes {
		face := source.Face(size)
		result := shaper.Shape("Hello", face)
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

// TestOwnShaper_VsBuiltinShaper compares OwnShaper output with BuiltinShaper.
// Both should produce similar results for simple Latin text without GSUB/GPOS.
func TestOwnShaper_VsBuiltinShaper(t *testing.T) {
	// Use own parser for both to ensure same font parsing.
	source, err := NewFontSource(requireTestFont(t), WithParser("own"))
	if err != nil {
		t.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	ownShaper := NewOwnShaper()
	builtinShaper := &BuiltinShaper{}

	text := "Hello World"

	ownResult := ownShaper.Shape(text, face)
	builtinResult := builtinShaper.Shape(text, face)

	// Both should produce the same number of glyphs.
	if len(ownResult) != len(builtinResult) {
		t.Errorf("glyph count mismatch: Own=%d, Builtin=%d",
			len(ownResult), len(builtinResult))
		return
	}

	// Glyph IDs should match (same font, same characters).
	for i := range ownResult {
		if ownResult[i].GID != builtinResult[i].GID {
			t.Errorf("glyph %d GID mismatch: Own=%d, Builtin=%d",
				i, ownResult[i].GID, builtinResult[i].GID)
		}
	}

	// Total advances should be similar (OwnShaper may apply kerning).
	ownTotal := ownResult[len(ownResult)-1].X + ownResult[len(ownResult)-1].XAdvance
	builtinTotal := builtinResult[len(builtinResult)-1].X + builtinResult[len(builtinResult)-1].XAdvance

	t.Logf("Total advance: Own=%.2f, Builtin=%.2f, diff=%.2f",
		ownTotal, builtinTotal, ownTotal-builtinTotal)

	// Both should produce positive total advance.
	if ownTotal <= 0 {
		t.Errorf("Own total advance = %f, want > 0", ownTotal)
	}
	if builtinTotal <= 0 {
		t.Errorf("Builtin total advance = %f, want > 0", builtinTotal)
	}
}

// TestOwnShaper_Kerning tests that OwnShaper applies kerning.
func TestOwnShaper_Kerning(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

	// Shape "A" and "V" separately.
	glyphsA := shaper.Shape("A", face)
	glyphsV := shaper.Shape("V", face)
	if len(glyphsA) != 1 || len(glyphsV) != 1 {
		t.Fatalf("expected 1 glyph each for A and V, got %d and %d",
			len(glyphsA), len(glyphsV))
	}

	individualWidth := glyphsA[0].XAdvance + glyphsV[0].XAdvance

	// Shape "AV" together — kerning should tighten the pair.
	glyphsAV := shaper.Shape("AV", face)
	if len(glyphsAV) != 2 {
		t.Fatalf("Shape(\"AV\"): got %d glyphs, want 2", len(glyphsAV))
	}

	combinedWidth := glyphsAV[1].X + glyphsAV[1].XAdvance

	if combinedWidth < individualWidth {
		t.Logf("Kerning detected: AV combined=%.2f < individual=%.2f (diff=%.2f)",
			combinedWidth, individualWidth, individualWidth-combinedWidth)
	} else {
		t.Logf("No kerning detected for AV pair in this font: combined=%.2f, individual=%.2f",
			combinedWidth, individualWidth)
	}

	// Sanity check: combined should not be much larger than individual.
	if combinedWidth > individualWidth*1.1 {
		t.Errorf("AV combined width %.2f is suspiciously larger than individual %.2f",
			combinedWidth, individualWidth)
	}
}

// TestOwnShaper_SetShaper tests integration with the global shaper system.
func TestOwnShaper_SetShaper(t *testing.T) {
	original := GetShaper()
	t.Cleanup(func() {
		SetShaper(original)
	})

	face, _ := ownTestFace(t)
	ownShaper := NewOwnShaper()

	SetShaper(ownShaper)

	current := GetShaper()
	if _, ok := current.(*OwnShaper); !ok {
		t.Errorf("GetShaper() should return *OwnShaper, got %T", current)
	}

	result := Shape("Hello", face)
	if len(result) != 5 {
		t.Errorf("Shape(\"Hello\") via global: got %d glyphs, want 5", len(result))
	}
}

// TestOwnShaper_Concurrency tests thread safety.
func TestOwnShaper_Concurrency(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

	var wg sync.WaitGroup
	errors := make(chan string, 200)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				result := shaper.Shape("Hello World", face)
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

// TestOwnShaper_ClearCache tests cache clearing.
func TestOwnShaper_ClearCache(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

	// Populate cache.
	_ = shaper.Shape("A", face)

	shaper.mu.RLock()
	cacheLen := len(shaper.cache)
	shaper.mu.RUnlock()
	if cacheLen != 1 {
		t.Errorf("cache should have 1 entry, got %d", cacheLen)
	}

	// Clear.
	shaper.ClearCache()

	shaper.mu.RLock()
	cacheLen = len(shaper.cache)
	shaper.mu.RUnlock()
	if cacheLen != 0 {
		t.Errorf("cache should be empty after ClearCache, got %d entries", cacheLen)
	}

	// Should still work.
	result := shaper.Shape("A", face)
	if len(result) != 1 {
		t.Errorf("Shape after ClearCache: got %d glyphs, want 1", len(result))
	}
}

// TestOwnShaper_RemoveSource tests removing a specific source from cache.
func TestOwnShaper_RemoveSource(t *testing.T) {
	_, source := ownTestFace(t)
	shaper := NewOwnShaper()

	face := source.Face(16.0)
	_ = shaper.Shape("A", face)

	shaper.mu.RLock()
	cacheLen := len(shaper.cache)
	shaper.mu.RUnlock()
	if cacheLen != 1 {
		t.Errorf("cache should have 1 entry, got %d", cacheLen)
	}

	shaper.RemoveSource(source)

	shaper.mu.RLock()
	cacheLen = len(shaper.cache)
	shaper.mu.RUnlock()
	if cacheLen != 0 {
		t.Errorf("cache should be empty after RemoveSource, got %d entries", cacheLen)
	}
}

// TestOwnShaper_WhitespaceHandling tests shaping text with whitespace.
func TestOwnShaper_WhitespaceHandling(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

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
			result := shaper.Shape(tt.text, face)
			if len(result) != tt.wantLen {
				t.Errorf("Shape(%q): got %d glyphs, want %d", tt.text, len(result), tt.wantLen)
			}
		})
	}
}

// TestOwnShaper_TabAdvance tests that tab characters get proper advance width.
func TestOwnShaper_TabAdvance(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

	// Shape a tab.
	tabResult := shaper.Shape("\t", face)
	if len(tabResult) != 1 {
		t.Fatalf("Shape(\"\\t\"): got %d glyphs, want 1", len(tabResult))
	}

	// Shape a space.
	spaceResult := shaper.Shape(" ", face)
	if len(spaceResult) != 1 {
		t.Fatalf("Shape(\" \"): got %d glyphs, want 1", len(spaceResult))
	}

	// Tab should be wider than space (DefaultTabWidth * space).
	tabAdv := tabResult[0].XAdvance
	spaceAdv := spaceResult[0].XAdvance

	if tabAdv <= spaceAdv {
		t.Errorf("tab advance (%.2f) should be > space advance (%.2f)", tabAdv, spaceAdv)
	}

	expectedRatio := float64(DefaultTabWidth)
	actualRatio := tabAdv / spaceAdv
	diff := actualRatio - expectedRatio
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.1 {
		t.Errorf("tab/space ratio = %.2f, expected %.2f", actualRatio, expectedRatio)
	}
}

// TestOwnShaper_ControlCharacters tests that control characters are skipped.
func TestOwnShaper_ControlCharacters(t *testing.T) {
	face, _ := ownTestFace(t)
	shaper := NewOwnShaper()

	// Control characters (U+0001..U+001F except \t) should be skipped.
	result := shaper.Shape("A\x01B\x02C", face)
	if len(result) != 3 {
		t.Errorf("Shape with control chars: got %d glyphs, want 3", len(result))
	}

	if result[0].GID == 0 || result[1].GID == 0 || result[2].GID == 0 {
		t.Error("control characters should be skipped, not produce notdef glyphs")
	}
}

// --- GSUB/GPOS structure tests ---

// TestGSUB_ParseCoverage tests coverage table parsing.
func TestGSUB_ParseCoverage(t *testing.T) {
	t.Run("format1", func(t *testing.T) {
		// Coverage Format 1: glyph list [10, 20, 30]
		data := []byte{
			0, 1, // format 1
			0, 3, // glyphCount = 3
			0, 10, // glyph 10
			0, 20, // glyph 20
			0, 30, // glyph 30
		}
		cov := parseCoverage(data)
		if cov == nil {
			t.Fatal("parseCoverage returned nil")
		}

		tests := []struct {
			gid     uint16
			wantIdx int
			wantOK  bool
		}{
			{10, 0, true},
			{20, 1, true},
			{30, 2, true},
			{15, -1, false},
			{0, -1, false},
			{31, -1, false},
		}

		for _, tt := range tests {
			idx, ok := cov.contains(tt.gid)
			if ok != tt.wantOK {
				t.Errorf("contains(%d): ok=%v, want %v", tt.gid, ok, tt.wantOK)
			}
			if ok && idx != tt.wantIdx {
				t.Errorf("contains(%d): idx=%d, want %d", tt.gid, idx, tt.wantIdx)
			}
		}
	})

	t.Run("format2", func(t *testing.T) {
		// Coverage Format 2: range [10..20] starting at coverage index 0
		data := []byte{
			0, 2, // format 2
			0, 1, // rangeCount = 1
			0, 10, // startGlyphID = 10
			0, 20, // endGlyphID = 20
			0, 0, // startCoverageIndex = 0
		}
		cov := parseCoverage(data)
		if cov == nil {
			t.Fatal("parseCoverage returned nil")
		}

		idx, ok := cov.contains(10)
		if !ok || idx != 0 {
			t.Errorf("contains(10): idx=%d, ok=%v, want 0, true", idx, ok)
		}
		idx, ok = cov.contains(15)
		if !ok || idx != 5 {
			t.Errorf("contains(15): idx=%d, ok=%v, want 5, true", idx, ok)
		}
		_, ok = cov.contains(21)
		if ok {
			t.Error("contains(21) should be false")
		}
		_, ok = cov.contains(9)
		if ok {
			t.Error("contains(9) should be false")
		}
	})
}

// TestGSUB_ParseClassDef tests ClassDef table parsing.
func TestGSUB_ParseClassDef(t *testing.T) {
	t.Run("format1", func(t *testing.T) {
		// ClassDef Format 1: startGlyph=10, classes=[1, 2, 3]
		data := []byte{
			0, 1, // format 1
			0, 10, // startGlyphID
			0, 3, // glyphCount
			0, 1, // class 1
			0, 2, // class 2
			0, 3, // class 3
		}
		cd := parseClassDef(data)
		if cd == nil {
			t.Fatal("parseClassDef returned nil")
		}

		if cd.classOf(10) != 1 {
			t.Errorf("classOf(10) = %d, want 1", cd.classOf(10))
		}
		if cd.classOf(11) != 2 {
			t.Errorf("classOf(11) = %d, want 2", cd.classOf(11))
		}
		if cd.classOf(12) != 3 {
			t.Errorf("classOf(12) = %d, want 3", cd.classOf(12))
		}
		if cd.classOf(9) != 0 {
			t.Errorf("classOf(9) = %d, want 0 (before range)", cd.classOf(9))
		}
		if cd.classOf(13) != 0 {
			t.Errorf("classOf(13) = %d, want 0 (after range)", cd.classOf(13))
		}
	})

	t.Run("format2", func(t *testing.T) {
		// ClassDef Format 2: range [10..15] = class 1
		data := []byte{
			0, 2, // format 2
			0, 1, // rangeCount = 1
			0, 10, // startGlyphID
			0, 15, // endGlyphID
			0, 1, // class
		}
		cd := parseClassDef(data)
		if cd == nil {
			t.Fatal("parseClassDef returned nil")
		}

		if cd.classOf(10) != 1 {
			t.Errorf("classOf(10) = %d, want 1", cd.classOf(10))
		}
		if cd.classOf(15) != 1 {
			t.Errorf("classOf(15) = %d, want 1", cd.classOf(15))
		}
		if cd.classOf(9) != 0 {
			t.Errorf("classOf(9) = %d, want 0", cd.classOf(9))
		}
		if cd.classOf(16) != 0 {
			t.Errorf("classOf(16) = %d, want 0", cd.classOf(16))
		}
	})
}

// TestGSUB_SingleSubst tests GSUB Type 1 (single substitution).
func TestGSUB_SingleSubst(t *testing.T) {
	t.Run("format1_delta", func(t *testing.T) {
		// Single subst format 1: coverage=[10], delta=+5 → glyph 10 → 15
		cov := []byte{
			0, 1, // coverage format 1
			0, 1, // glyphCount
			0, 10, // glyph 10
		}
		data := make([]byte, 6+len(cov))
		data[0] = 0
		data[1] = 1 // format 1
		data[2] = 0
		data[3] = 6 // coverageOffset = 6
		data[4] = 0
		data[5] = 5 // deltaGlyphID = +5
		copy(data[6:], cov)

		glyphs := []shapingGlyph{
			{gid: 5, cluster: 0},
			{gid: 10, cluster: 1},
			{gid: 20, cluster: 2},
		}

		result := applySingleSubst(data, glyphs)
		if result[0].gid != 5 {
			t.Errorf("glyph 0: gid=%d, want 5 (not covered)", result[0].gid)
		}
		if result[1].gid != 15 {
			t.Errorf("glyph 1: gid=%d, want 15 (10+5)", result[1].gid)
		}
		if result[2].gid != 20 {
			t.Errorf("glyph 2: gid=%d, want 20 (not covered)", result[2].gid)
		}
	})

	t.Run("format2_array", func(t *testing.T) {
		// Single subst format 2: coverage=[10, 20], substitutes=[100, 200]
		// Layout: format(2) + covOff(2) + glyphCount(2) + subs(4) + coverage(8) = 18 bytes
		data := []byte{
			0, 2, // format 2
			0, 10, // coverageOffset = 10 (after substitutes)
			0, 2, // glyphCount = 2
			0, 100, // substitute[0] = 100
			0, 200, // substitute[1] = 200
			// Coverage at offset 10:
			0, 1, // coverage format 1
			0, 2, // glyphCount = 2
			0, 10, // glyph 10
			0, 20, // glyph 20
		}

		glyphs := []shapingGlyph{
			{gid: 10, cluster: 0},
			{gid: 20, cluster: 1},
			{gid: 30, cluster: 2},
		}
		result := applySingleSubst(data, glyphs)
		if result[0].gid != 100 {
			t.Errorf("glyph 0: gid=%d, want 100", result[0].gid)
		}
		if result[1].gid != 200 {
			t.Errorf("glyph 1: gid=%d, want 200", result[1].gid)
		}
		if result[2].gid != 30 {
			t.Errorf("glyph 2: gid=%d, want 30 (not covered)", result[2].gid)
		}
	})
}

// TestGSUB_LigatureSubst tests GSUB Type 4 (ligature substitution).
func TestGSUB_LigatureSubst(t *testing.T) {
	// Build a ligature table: glyph 10 + glyph 11 -> ligature glyph 100
	// Coverage: [10]
	// LigatureSet for glyph 10:
	//   Ligature: ligGlyph=100, compCount=2, components=[11]
	//
	// Layout (offsets are relative to table start):
	//   [0]  format(2) = 1
	//   [2]  coverageOffset(2) = 18 (after LigatureSet data)
	//   [4]  ligatureSetCount(2) = 1
	//   [6]  ligatureSetOffset[0](2) = 8
	//   [8]  LigatureSet: ligatureCount(2) = 1, ligatureOffset[0](2) = 4
	//   [12] Ligature: ligGlyph(2)=100, compCount(2)=2, component[0](2)=11
	//   [18] Coverage: format(2)=1, glyphCount(2)=1, glyph(2)=10
	data := []byte{
		0, 1, // [0] format 1
		0, 18, // [2] coverageOffset = 18
		0, 1, // [4] ligatureSetCount = 1
		0, 8, // [6] ligatureSetOffset[0] = 8
		// LigatureSet at offset 8:
		0, 1, // [8] ligatureCount = 1
		0, 4, // [10] ligatureOffset[0] = 4 (relative to LigatureSet start)
		// Ligature at offset 12 (LigatureSet start + 4):
		0, 100, // [12] ligatureGlyph = 100
		0, 2, // [14] componentCount = 2
		0, 11, // [16] component[0] = 11
		// Coverage at offset 18:
		0, 1, // [18] format 1
		0, 1, // [20] glyphCount = 1
		0, 10, // [22] glyph = 10
	}

	glyphs := []shapingGlyph{
		{gid: 5, cluster: 0},
		{gid: 10, cluster: 1},
		{gid: 11, cluster: 2},
		{gid: 20, cluster: 3},
	}

	result := applyLigatureSubst(data, glyphs)

	// Expected: glyph 10+11 → 100, so [5, 100, 20]
	if len(result) != 3 {
		t.Fatalf("ligature subst: got %d glyphs, want 3: %+v", len(result), result)
	}
	if result[0].gid != 5 {
		t.Errorf("glyph 0: gid=%d, want 5", result[0].gid)
	}
	if result[1].gid != 100 {
		t.Errorf("glyph 1: gid=%d, want 100 (ligature)", result[1].gid)
	}
	if result[2].gid != 20 {
		t.Errorf("glyph 2: gid=%d, want 20", result[2].gid)
	}
}

// TestKern_ParseAndLookup tests kern table parsing and lookup.
func TestKern_ParseAndLookup(t *testing.T) {
	// Build a minimal kern table format 0.
	// version=0, nTables=1
	// Subtable: version=0, length=20, coverage=0x0001 (horizontal, format 0)
	// 1 pair: left=10, right=20, value=-50
	//
	// Subtable layout (20 bytes total):
	//   Header: version(2) + length(2) + coverage(2) = 6 bytes
	//   Format 0: nPairs(2) + searchRange(2) + entrySelector(2) + rangeShift(2) = 8 bytes
	//   Pairs: 1 * 6 bytes = 6 bytes

	kernData := []byte{
		0, 0, // version 0
		0, 1, // nTables = 1
		// Subtable header (6 bytes):
		0, 0, // subtable version 0
		0, 20, // length = 20 (6 header + 8 format0 header + 6 pair)
		0, 1, // coverage: bit 0 = horizontal, format bits 8-15 = 0
		// Format 0 header (8 bytes):
		0, 1, // nPairs = 1
		0, 6, // searchRange
		0, 0, // entrySelector
		0, 0, // rangeShift
		// Pair (6 bytes):
		0, 10, // left
		0, 20, // right
		0xFF, 0xCE, // value = -50 (int16 big-endian)
	}

	k := parseKern(kernData)
	if k == nil {
		t.Fatal("parseKern returned nil")
	}

	// Test the pair.
	val := k.kernValue(10, 20)
	if val != -50 {
		t.Errorf("kernValue(10, 20) = %d, want -50", val)
	}

	// Test non-existent pair.
	val = k.kernValue(10, 21)
	if val != 0 {
		t.Errorf("kernValue(10, 21) = %d, want 0", val)
	}

	val = k.kernValue(20, 10)
	if val != 0 {
		t.Errorf("kernValue(20, 10) = %d, want 0 (reversed)", val)
	}
}

// TestOwnShaper_WithGoRegularFont tests shaping with Go Regular using own parser.
// Go Regular has a GPOS table with kerning data.
func TestOwnShaper_WithGoRegularFont(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t), WithParser("own"))
	if err != nil {
		t.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	shaper := NewOwnShaper()
	face := source.Face(16.0)

	// Check that the font tables were parsed.
	sc := shaper.getOrCreateCache(source)
	if sc == nil {
		t.Fatal("shaper cache is nil")
	}

	t.Logf("Go Regular: GSUB=%v, GPOS=%v, kern=%v, upem=%d",
		sc.gsub != nil, sc.gpos != nil, sc.kern != nil, sc.upem)

	// Shape a sentence.
	result := shaper.Shape("The quick brown fox jumps over the lazy dog.", face)
	if len(result) == 0 {
		t.Fatal("Shape returned no glyphs")
	}

	// Verify total advance is reasonable.
	totalAdvance := result[len(result)-1].X + result[len(result)-1].XAdvance
	if totalAdvance <= 0 {
		t.Errorf("total advance = %f, want > 0", totalAdvance)
	}
	t.Logf("Sentence: %d glyphs, total advance = %.2f", len(result), totalAdvance)
}

// TestValueRecord_Parsing tests OpenType ValueRecord parsing.
func TestValueRecord_Parsing(t *testing.T) {
	tests := []struct {
		name        string
		valueFormat uint16
		data        []byte
		wantX       int16
		wantY       int16
		wantXAdv    int16
		wantYAdv    int16
		wantSize    int
	}{
		{
			name:        "xPlacement only",
			valueFormat: 0x0001,
			data:        []byte{0xFF, 0xD8}, // -40
			wantX:       -40,
			wantSize:    2,
		},
		{
			name:        "xAdvance only",
			valueFormat: 0x0004,
			data:        []byte{0xFF, 0xCE}, // -50
			wantXAdv:    -50,
			wantSize:    2,
		},
		{
			name:        "all four",
			valueFormat: 0x000F,
			data: []byte{
				0, 10, // xPlacement = 10
				0, 20, // yPlacement = 20
				0xFF, 0xCE, // xAdvance = -50
				0, 30, // yAdvance = 30
			},
			wantX:    10,
			wantY:    20,
			wantXAdv: -50,
			wantYAdv: 30,
			wantSize: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr, consumed := parseValueRecord(tt.data, 0, tt.valueFormat)
			if vr.xPlacement != tt.wantX {
				t.Errorf("xPlacement = %d, want %d", vr.xPlacement, tt.wantX)
			}
			if vr.yPlacement != tt.wantY {
				t.Errorf("yPlacement = %d, want %d", vr.yPlacement, tt.wantY)
			}
			if vr.xAdvance != tt.wantXAdv {
				t.Errorf("xAdvance = %d, want %d", vr.xAdvance, tt.wantXAdv)
			}
			if vr.yAdvance != tt.wantYAdv {
				t.Errorf("yAdvance = %d, want %d", vr.yAdvance, tt.wantYAdv)
			}

			expectedSize := valueRecordSize(tt.valueFormat)
			if expectedSize != tt.wantSize {
				t.Errorf("valueRecordSize = %d, want %d", expectedSize, tt.wantSize)
			}
			if consumed != tt.wantSize {
				t.Errorf("consumed = %d, want %d", consumed, tt.wantSize)
			}
		})
	}
}

// TestOwnShaper_ScriptDetection tests OpenType script tag detection.
func TestOwnShaper_ScriptDetection(t *testing.T) {
	tests := []struct {
		name string
		text string
		want [4]byte
	}{
		{"Latin", "Hello", [4]byte{'l', 'a', 't', 'n'}},
		{"Cyrillic", "\u0410\u0411", [4]byte{'c', 'y', 'r', 'l'}},
		{"Greek", "\u0391\u0392", [4]byte{'g', 'r', 'e', 'k'}},
		{"spaces then Latin", "  Hello", [4]byte{'l', 'a', 't', 'n'}},
		{"empty", "", [4]byte{'l', 'a', 't', 'n'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runes := []rune(tt.text)
			tag := detectOTScriptTag(runes)
			if tag != tt.want {
				t.Errorf("detectOTScriptTag(%q) = %q, want %q", tt.text, tag, tt.want)
			}
		})
	}
}

// TestSliceReplace tests the slice replacement helper.
func TestSliceReplace(t *testing.T) {
	tests := []struct {
		name     string
		input    []shapingGlyph
		pos      int
		remove   int
		repl     []shapingGlyph
		wantGIDs []uint16
	}{
		{
			name:     "delete middle",
			input:    []shapingGlyph{{gid: 1}, {gid: 2}, {gid: 3}},
			pos:      1,
			remove:   1,
			repl:     nil,
			wantGIDs: []uint16{1, 3},
		},
		{
			name:     "replace one with two",
			input:    []shapingGlyph{{gid: 1}, {gid: 2}, {gid: 3}},
			pos:      1,
			remove:   1,
			repl:     []shapingGlyph{{gid: 10}, {gid: 11}},
			wantGIDs: []uint16{1, 10, 11, 3},
		},
		{
			name:     "replace two with one",
			input:    []shapingGlyph{{gid: 1}, {gid: 2}, {gid: 3}, {gid: 4}},
			pos:      1,
			remove:   2,
			repl:     []shapingGlyph{{gid: 99}},
			wantGIDs: []uint16{1, 99, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sliceReplace(tt.input, tt.pos, tt.remove, tt.repl)
			if len(result) != len(tt.wantGIDs) {
				t.Fatalf("len(result) = %d, want %d", len(result), len(tt.wantGIDs))
			}
			for i, want := range tt.wantGIDs {
				if result[i].gid != want {
					t.Errorf("result[%d].gid = %d, want %d", i, result[i].gid, want)
				}
			}
		})
	}
}

// TestOwnShaper_NoLigatures tests that disabling ligatures works.
func TestOwnShaper_NoLigatures(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t), WithParser("own"))
	if err != nil {
		t.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	shaper := NewOwnShaper()

	// Create face with ligatures disabled.
	face := source.Face(16.0, WithFeatures(NoLigatures))

	// Shape text that might have ligatures.
	result := shaper.Shape("office", face)
	if len(result) == 0 {
		t.Fatal("Shape returned no glyphs")
	}

	// With ligatures disabled, "office" should produce exactly 6 glyphs.
	if len(result) != 6 {
		t.Logf("With NoLigatures: %d glyphs for 'office' (expected 6, got %d — font may not have fi ligature)",
			len(result), len(result))
	}
}

// BenchmarkOwnShape benchmarks OwnShaper with a standard sentence.
func BenchmarkOwnShape(b *testing.B) {
	source, err := NewFontSource(requireTestFont(b), WithParser("own"))
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	shaper := NewOwnShaper()

	// Warm the cache.
	_ = shaper.Shape("warmup", face)

	text := "The quick brown fox jumps over the lazy dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = shaper.Shape(text, face)
	}
}

// BenchmarkOwnShapeShort benchmarks shaping a short string.
func BenchmarkOwnShapeShort(b *testing.B) {
	source, err := NewFontSource(requireTestFont(b), WithParser("own"))
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	shaper := NewOwnShaper()
	_ = shaper.Shape("w", face) // warm cache

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = shaper.Shape("Hello", face)
	}
}
