package text

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// TestWrapModeString tests WrapMode.String method.
func TestWrapModeString(t *testing.T) {
	tests := []struct {
		mode WrapMode
		want string
	}{
		{WrapWordChar, "WordChar"},
		{WrapNone, "None"},
		{WrapWord, "Word"},
		{WrapChar, "Char"},
		{WrapMode(99), unknownStr},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("WrapMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

// TestClassifyRune tests rune classification for line breaking.
func TestClassifyRune(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want BreakClass
	}{
		{"space", ' ', breakSpace},
		{"tab", '\t', breakSpace},
		{"zero-width space", '\u200B', breakZero},
		{"open paren", '(', breakOpen},
		{"close paren", ')', breakClose},
		{"open bracket", '[', breakOpen},
		{"close bracket", ']', breakClose},
		{"open brace", '{', breakOpen},
		{"close brace", '}', breakClose},
		{"left double quote", '\u201C', breakOpen},
		{"right double quote", '\u201D', breakClose},
		{"hyphen", '-', breakHyphen},
		{"en dash", '\u2013', breakHyphen},
		{"em dash", '\u2014', breakHyphen},
		{"CJK ideograph", '\u4E00', breakIdeographic},
		{"hiragana", '\u3042', breakIdeographic},
		{"katakana", '\u30A2', breakIdeographic},
		{"hangul", '\uAC00', breakIdeographic},
		{"latin a", 'a', breakOther},
		{"digit 1", '1', breakOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRune(tt.r)
			if got != tt.want {
				t.Errorf("classifyRune(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

// TestIsCJKRune tests CJK character detection.
func TestIsCJKRune(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"CJK ideograph start", '\u4E00', true},
		{"CJK ideograph end", '\u9FFF', true},
		{"CJK Extension A", '\u3400', true},
		{"Hiragana A", '\u3042', true},
		{"Katakana A", '\u30A2', true},
		{"Hangul syllable", '\uAC00', true},
		{"Fullwidth A", '\uFF21', true},
		{"Latin A", 'A', false},
		{"Space", ' ', false},
		{"Digit", '1', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCJKRune(tt.r)
			if got != tt.want {
				t.Errorf("isCJKRune(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

// TestFindBreakOpportunities tests break opportunity detection.
func TestFindBreakOpportunities(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		mode    WrapMode
		wantLen int
	}{
		{"empty", "", WrapWord, 0},
		{"single char", "a", WrapWord, 1},
		{"two chars", "ab", WrapWord, 2},
		{"word with space", "a b", WrapWord, 3},
		{"CJK", "\u4E00\u4E01", WrapWord, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			breaks := findBreakOpportunities(tt.text, tt.mode)
			if len(breaks) != tt.wantLen {
				t.Errorf("findBreakOpportunities(%q, %v) returned %d breaks, want %d",
					tt.text, tt.mode, len(breaks), tt.wantLen)
			}
			// First character should never have a break before it
			if len(breaks) > 0 && breaks[0] != BreakNo {
				t.Error("first character should have BreakNo")
			}
		})
	}
}

// TestFindBreakOpportunities_WrapNone tests that WrapNone disables all breaks.
func TestFindBreakOpportunities_WrapNone(t *testing.T) {
	text := "hello world"
	breaks := findBreakOpportunities(text, WrapNone)

	for i, b := range breaks {
		if b != BreakNo {
			t.Errorf("WrapNone: break at position %d should be BreakNo, got %v", i, b)
		}
	}
}

// TestFindBreakOpportunities_WrapWord tests word boundary breaks.
func TestFindBreakOpportunities_WrapWord(t *testing.T) {
	text := "hello world"
	breaks := findBreakOpportunities(text, WrapWord)

	// Should have break opportunity after space (position 6, before 'w')
	if len(breaks) < 7 {
		t.Fatalf("expected at least 7 break positions, got %d", len(breaks))
	}

	// Position 0: 'h' - no break
	if breaks[0] != BreakNo {
		t.Error("position 0 should be BreakNo")
	}

	// Position 6: 'w' - should have break before it (after space)
	if breaks[6] != BreakAllowed {
		t.Errorf("position 6 (after space) should be BreakAllowed, got %v", breaks[6])
	}
}

// TestFindBreakOpportunities_WrapChar tests character boundary breaks.
func TestFindBreakOpportunities_WrapChar(t *testing.T) {
	text := "abc"
	breaks := findBreakOpportunities(text, WrapChar)

	// WrapChar allows breaks everywhere except position 0
	if breaks[0] != BreakNo {
		t.Error("position 0 should be BreakNo")
	}
	for i := 1; i < len(breaks); i++ {
		if breaks[i] != BreakAllowed {
			t.Errorf("position %d should be BreakAllowed in WrapChar mode", i)
		}
	}
}

// TestFindBreakOpportunities_CJK tests CJK character breaks.
func TestFindBreakOpportunities_CJK(t *testing.T) {
	// CJK text: "中文" (Chinese characters)
	text := "\u4E2D\u6587"
	breaks := findBreakOpportunities(text, WrapWord)

	if len(breaks) != 2 {
		t.Fatalf("expected 2 break positions, got %d", len(breaks))
	}

	// Break allowed before second CJK character
	if breaks[1] != BreakAllowed {
		t.Errorf("break before second CJK char should be allowed, got %v", breaks[1])
	}
}

// TestFindBreakOpportunities_Punctuation tests punctuation handling.
func TestFindBreakOpportunities_Punctuation(t *testing.T) {
	// Test that we don't break before closing punctuation
	text := "(abc)"
	breaks := findBreakOpportunities(text, WrapWord)

	// Position 4: ')' - no break before closing paren
	if len(breaks) > 4 && breaks[4] != BreakNo {
		t.Errorf("no break allowed before closing paren, got %v", breaks[4])
	}
}

// TestWrapTextInfo tests wrapTextInfo creation.
func TestWrapTextInfo(t *testing.T) {
	text := "hello"
	info := newWrapTextInfo(text, WrapWord)

	if info.text != text {
		t.Errorf("text = %q, want %q", info.text, text)
	}
	if len(info.runes) != 5 {
		t.Errorf("runes length = %d, want 5", len(info.runes))
	}
	if len(info.breaks) != 5 {
		t.Errorf("breaks length = %d, want 5", len(info.breaks))
	}
	if len(info.byteOffsets) != 6 {
		t.Errorf("byteOffsets length = %d, want 6", len(info.byteOffsets))
	}
}

// TestWrapTextInfo_UTF8 tests wrapTextInfo with multi-byte characters.
func TestWrapTextInfo_UTF8(t *testing.T) {
	text := "héllo" // 'é' is 2 bytes in UTF-8
	info := newWrapTextInfo(text, WrapWord)

	if len(info.runes) != 5 {
		t.Errorf("runes length = %d, want 5", len(info.runes))
	}

	// Check byte offsets
	// h=0, é=1, l=3, l=4, o=5, end=6
	expectedOffsets := []int{0, 1, 3, 4, 5, 6}
	for i, expected := range expectedOffsets {
		if info.byteOffsets[i] != expected {
			t.Errorf("byteOffsets[%d] = %d, want %d", i, info.byteOffsets[i], expected)
		}
	}
}

// TestWrapTextInfo_CanBreakAt tests break opportunity checking.
func TestWrapTextInfo_CanBreakAt(t *testing.T) {
	info := newWrapTextInfo("a b", WrapWord)

	// Position 0: no break
	if info.canBreakAt(0) {
		t.Error("canBreakAt(0) should be false")
	}

	// Position 2: after space, break allowed
	if !info.canBreakAt(2) {
		t.Error("canBreakAt(2) should be true (after space)")
	}

	// Out of bounds
	if info.canBreakAt(-1) {
		t.Error("canBreakAt(-1) should be false")
	}
	if info.canBreakAt(100) {
		t.Error("canBreakAt(100) should be false")
	}
}

// TestWrapTextInfo_Substring tests substring extraction.
func TestWrapTextInfo_Substring(t *testing.T) {
	info := newWrapTextInfo("hello world", WrapWord)

	// "hello"
	sub := info.substring(0, 5)
	if sub != "hello" {
		t.Errorf("substring(0, 5) = %q, want %q", sub, "hello")
	}

	// "world"
	sub = info.substring(6, 11)
	if sub != "world" {
		t.Errorf("substring(6, 11) = %q, want %q", sub, "world")
	}
}

// wrapTestFace creates a test Face for wrap tests.
func wrapTestFace(t *testing.T) Face {
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

// TestWrapText_Empty tests wrapping empty text.
func TestWrapText_Empty(t *testing.T) {
	face := wrapTestFace(t)
	results := WrapText("", face, 16.0, 100, WrapWord)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Text != "" {
		t.Errorf("expected empty text, got %q", results[0].Text)
	}
}

// TestWrapText_NoWrap tests that WrapNone doesn't wrap.
func TestWrapText_NoWrap(t *testing.T) {
	face := wrapTestFace(t)
	text := "The quick brown fox jumps over the lazy dog"

	results := WrapText(text, face, 16.0, 100, WrapNone)

	if len(results) != 1 {
		t.Errorf("WrapNone should produce 1 line, got %d", len(results))
	}
	if results[0].Text != text {
		t.Errorf("text should be unchanged, got %q", results[0].Text)
	}
}

// TestWrapText_Word tests word-based wrapping.
func TestWrapText_Word(t *testing.T) {
	face := wrapTestFace(t)
	text := "hello world test"

	// Use narrow width to force wrapping
	results := WrapText(text, face, 16.0, 80, WrapWord)

	if len(results) < 2 {
		t.Errorf("WrapWord should produce multiple lines, got %d", len(results))
	}

	// Verify text is preserved
	var combined strings.Builder
	for i, r := range results {
		if i > 0 {
			combined.WriteString(" ")
		}
		combined.WriteString(strings.TrimSpace(r.Text))
	}
	if strings.TrimSpace(combined.String()) != text {
		t.Errorf("combined text = %q, want %q", combined.String(), text)
	}
}

// TestWrapText_Char tests character-based wrapping.
func TestWrapText_Char(t *testing.T) {
	face := wrapTestFace(t)
	text := "abcdefghij"

	// Use very narrow width
	results := WrapText(text, face, 16.0, 30, WrapChar)

	if len(results) < 2 {
		t.Errorf("WrapChar with narrow width should produce multiple lines, got %d", len(results))
	}
}

// TestWrapText_WordChar tests WordChar fallback behavior.
func TestWrapText_WordChar(t *testing.T) {
	face := wrapTestFace(t)
	// Single long word that exceeds maxWidth
	text := "supercalifragilisticexpialidocious"

	results := WrapText(text, face, 16.0, 100, WrapWordChar)

	// Should break the long word since no word boundaries exist
	if len(results) < 2 {
		t.Errorf("WrapWordChar should break long words, got %d lines", len(results))
	}
}

// TestWrapText_CJK tests CJK text wrapping.
func TestWrapText_CJK(t *testing.T) {
	face := wrapTestFace(t)
	// Chinese text
	text := "\u4E2D\u6587\u6D4B\u8BD5\u6587\u672C"

	results := WrapText(text, face, 16.0, 50, WrapWord)

	// CJK should allow breaking between any characters
	if len(results) < 2 {
		t.Errorf("CJK text should wrap, got %d lines", len(results))
	}
}

// TestMeasureText tests text measurement.
func TestMeasureText(t *testing.T) {
	face := wrapTestFace(t)

	tests := []struct {
		text     string
		wantZero bool
	}{
		{"", true},
		{"a", false},
		{"hello", false},
		{"   ", false}, // Spaces have width
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			width := MeasureText(tt.text, face, 16.0)
			if tt.wantZero && width != 0 {
				t.Errorf("MeasureText(%q) = %f, want 0", tt.text, width)
			}
			if !tt.wantZero && width <= 0 {
				t.Errorf("MeasureText(%q) = %f, want > 0", tt.text, width)
			}
		})
	}
}

// TestMeasureText_NilFace tests MeasureText with nil face.
func TestMeasureText_NilFace(t *testing.T) {
	width := MeasureText("hello", nil, 16.0)
	if width != 0 {
		t.Errorf("MeasureText with nil face should return 0, got %f", width)
	}
}

// TestMeasureText_Proportional tests that longer text has greater width.
func TestMeasureText_Proportional(t *testing.T) {
	face := wrapTestFace(t)

	short := MeasureText("abc", face, 16.0)
	long := MeasureText("abcdefg", face, 16.0)

	if long <= short {
		t.Errorf("longer text width (%f) should be > shorter text width (%f)", long, short)
	}
}

// TestLayoutText_WrapMode tests layout with different wrap modes.
func TestLayoutText_WrapMode(t *testing.T) {
	face := wrapTestFace(t)
	text := "The quick brown fox"

	tests := []struct {
		name     string
		mode     WrapMode
		maxWidth float64
		minLines int
	}{
		{"WrapNone", WrapNone, 100, 1},
		{"WrapWord narrow", WrapWord, 50, 2},
		{"WrapChar narrow", WrapChar, 50, 2},
		{"WrapWordChar narrow", WrapWordChar, 50, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := LayoutOptions{
				MaxWidth:    tt.maxWidth,
				LineSpacing: 1.0,
				Alignment:   AlignLeft,
				Direction:   DirectionLTR,
				WrapMode:    tt.mode,
			}

			layout := LayoutText(text, face, 16.0, opts)

			if len(layout.Lines) < tt.minLines {
				t.Errorf("expected at least %d lines with %s, got %d",
					tt.minLines, tt.name, len(layout.Lines))
			}
		})
	}
}

// TestLayoutText_WrapModeNone tests that WrapNone prevents wrapping.
func TestLayoutText_WrapModeNone(t *testing.T) {
	face := wrapTestFace(t)
	text := "The quick brown fox jumps over the lazy dog"

	opts := LayoutOptions{
		MaxWidth:    50, // Very narrow
		LineSpacing: 1.0,
		Alignment:   AlignLeft,
		Direction:   DirectionLTR,
		WrapMode:    WrapNone,
	}

	layout := LayoutText(text, face, 16.0, opts)

	if len(layout.Lines) != 1 {
		t.Errorf("WrapNone should produce 1 line regardless of MaxWidth, got %d", len(layout.Lines))
	}
}

// BenchmarkFindBreakOpportunities benchmarks break opportunity detection.
func BenchmarkFindBreakOpportunities(b *testing.B) {
	text := "The quick brown fox jumps over the lazy dog."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = findBreakOpportunities(text, WrapWord)
	}
}

// BenchmarkFindBreakOpportunities_Long benchmarks with long text.
func BenchmarkFindBreakOpportunities_Long(b *testing.B) {
	var builder strings.Builder
	for i := 0; i < 100; i++ {
		builder.WriteString("The quick brown fox jumps over the lazy dog. ")
	}
	text := builder.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = findBreakOpportunities(text, WrapWord)
	}
}

// BenchmarkWrapText benchmarks text wrapping.
func BenchmarkWrapText(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	text := "The quick brown fox jumps over the lazy dog."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WrapText(text, face, 16.0, 200, WrapWord)
	}
}

// BenchmarkMeasureText benchmarks text measurement.
func BenchmarkMeasureText(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	text := "The quick brown fox"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MeasureText(text, face, 16.0)
	}
}

// BenchmarkClassifyRune benchmarks rune classification.
func BenchmarkClassifyRune(b *testing.B) {
	runes := []rune("The quick brown fox jumps over the lazy dog.")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, r := range runes {
			_ = classifyRune(r)
		}
	}
}
