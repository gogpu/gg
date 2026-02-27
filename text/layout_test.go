package text

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// layoutTestFace creates a test Face at size 16 for layout tests.
func layoutTestFace(t *testing.T) Face {
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

// TestLayoutText_Empty tests layout of empty string.
func TestLayoutText_Empty(t *testing.T) {
	face := layoutTestFace(t)

	layout := LayoutText("", face, 16.0, DefaultLayoutOptions())

	if layout == nil {
		t.Fatal("LayoutText returned nil for empty string")
	}
	if len(layout.Lines) != 0 {
		t.Errorf("expected 0 lines for empty string, got %d", len(layout.Lines))
	}
	if layout.Width != 0 {
		t.Errorf("expected 0 width for empty string, got %f", layout.Width)
	}
	if layout.Height != 0 {
		t.Errorf("expected 0 height for empty string, got %f", layout.Height)
	}
}

// TestLayoutText_NilFace tests layout with nil face.
func TestLayoutText_NilFace(t *testing.T) {
	layout := LayoutText("Hello", nil, 16.0, DefaultLayoutOptions())

	if layout == nil {
		t.Fatal("LayoutText returned nil for nil face")
	}
	if len(layout.Lines) != 0 {
		t.Errorf("expected 0 lines for nil face, got %d", len(layout.Lines))
	}
}

// TestLayoutText_SingleLine tests single line layout.
func TestLayoutText_SingleLine(t *testing.T) {
	face := layoutTestFace(t)
	text := "Hello World"

	layout := LayoutText(text, face, 16.0, DefaultLayoutOptions())

	if layout == nil {
		t.Fatal("LayoutText returned nil")
	}
	if len(layout.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(layout.Lines))
	}

	line := &layout.Lines[0]

	// Check that we have glyphs
	if len(line.Glyphs) != 11 {
		t.Errorf("expected 11 glyphs, got %d", len(line.Glyphs))
	}

	// Check that width is positive
	if line.Width <= 0 {
		t.Error("line width should be positive")
	}

	// Check that ascent and descent are set
	if line.Ascent <= 0 {
		t.Error("line ascent should be positive")
	}
	if line.Descent <= 0 {
		t.Error("line descent should be positive")
	}

	// Check layout dimensions
	if layout.Width != line.Width {
		t.Errorf("layout width %f should equal line width %f", layout.Width, line.Width)
	}
	if layout.Height <= 0 {
		t.Error("layout height should be positive")
	}
}

// TestLayoutText_MultiLine tests text with explicit line breaks.
func TestLayoutText_MultiLine(t *testing.T) {
	face := layoutTestFace(t)
	text := "Line One\nLine Two\nLine Three"

	layout := LayoutText(text, face, 16.0, DefaultLayoutOptions())

	if layout == nil {
		t.Fatal("LayoutText returned nil")
	}
	if len(layout.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(layout.Lines))
	}

	// Check that lines are stacked vertically
	var prevY float64
	for i, line := range layout.Lines {
		if i > 0 && line.Y <= prevY {
			t.Errorf("line %d Y (%f) should be greater than line %d Y (%f)",
				i, line.Y, i-1, prevY)
		}
		prevY = line.Y
	}

	// Check total height includes all lines
	if layout.Height <= layout.Lines[0].Height() {
		t.Error("layout height should include all lines")
	}
}

// TestLayoutText_WindowsLineEndings tests CRLF line endings.
func TestLayoutText_WindowsLineEndings(t *testing.T) {
	face := layoutTestFace(t)
	text := "Line One\r\nLine Two"

	layout := LayoutText(text, face, 16.0, DefaultLayoutOptions())

	if len(layout.Lines) != 2 {
		t.Errorf("expected 2 lines for CRLF text, got %d", len(layout.Lines))
	}
}

// TestLayoutText_MacLineEndings tests CR line endings.
func TestLayoutText_MacLineEndings(t *testing.T) {
	face := layoutTestFace(t)
	text := "Line One\rLine Two"

	layout := LayoutText(text, face, 16.0, DefaultLayoutOptions())

	if len(layout.Lines) != 2 {
		t.Errorf("expected 2 lines for CR text, got %d", len(layout.Lines))
	}
}

// TestLayoutText_EmptyLines tests empty lines.
func TestLayoutText_EmptyLines(t *testing.T) {
	face := layoutTestFace(t)
	text := "Line One\n\nLine Three"

	layout := LayoutText(text, face, 16.0, DefaultLayoutOptions())

	if len(layout.Lines) != 3 {
		t.Errorf("expected 3 lines (including empty), got %d", len(layout.Lines))
	}

	// Empty line should have height but no glyphs
	emptyLine := &layout.Lines[1]
	if len(emptyLine.Glyphs) != 0 {
		t.Errorf("empty line should have 0 glyphs, got %d", len(emptyLine.Glyphs))
	}
	if emptyLine.Ascent <= 0 {
		t.Error("empty line should have positive ascent for line height")
	}
}

// TestLayoutText_Alignment tests horizontal alignment.
func TestLayoutText_Alignment(t *testing.T) {
	face := layoutTestFace(t)
	text := "Test"

	tests := []struct {
		name      string
		alignment Alignment
		maxWidth  float64
		checkFn   func(t *testing.T, line *Line, maxWidth float64)
	}{
		{
			name:      "AlignLeft",
			alignment: AlignLeft,
			maxWidth:  200,
			checkFn: func(t *testing.T, line *Line, _ float64) {
				if len(line.Glyphs) > 0 && line.Glyphs[0].X != 0 {
					t.Errorf("left-aligned first glyph should be at X=0, got %f", line.Glyphs[0].X)
				}
			},
		},
		{
			name:      "AlignCenter",
			alignment: AlignCenter,
			maxWidth:  200,
			checkFn: func(t *testing.T, line *Line, maxWidth float64) {
				if len(line.Glyphs) == 0 {
					return
				}
				expectedOffset := (maxWidth - line.Width) / 2
				if line.Glyphs[0].X < expectedOffset-1 || line.Glyphs[0].X > expectedOffset+1 {
					t.Errorf("center-aligned first glyph should be near X=%f, got %f",
						expectedOffset, line.Glyphs[0].X)
				}
			},
		},
		{
			name:      "AlignRight",
			alignment: AlignRight,
			maxWidth:  200,
			checkFn: func(t *testing.T, line *Line, maxWidth float64) {
				if len(line.Glyphs) == 0 {
					return
				}
				lastGlyph := &line.Glyphs[len(line.Glyphs)-1]
				expectedEnd := maxWidth
				actualEnd := lastGlyph.X + lastGlyph.XAdvance
				// Use line.Glyphs[0].X to calculate actual offset
				expectedOffset := maxWidth - line.Width
				if line.Glyphs[0].X < expectedOffset-1 || line.Glyphs[0].X > expectedOffset+1 {
					t.Errorf("right-aligned should end at %f, got %f (offset %f)",
						expectedEnd, actualEnd, line.Glyphs[0].X)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := LayoutOptions{
				MaxWidth:    tt.maxWidth,
				LineSpacing: 1.0,
				Alignment:   tt.alignment,
				Direction:   DirectionLTR,
			}

			layout := LayoutText(text, face, 16.0, opts)

			if len(layout.Lines) != 1 {
				t.Fatalf("expected 1 line, got %d", len(layout.Lines))
			}

			tt.checkFn(t, &layout.Lines[0], tt.maxWidth)
		})
	}
}

// TestLayoutText_AlignmentString tests Alignment.String method.
func TestLayoutText_AlignmentString(t *testing.T) {
	tests := []struct {
		alignment Alignment
		want      string
	}{
		{AlignLeft, "Left"},
		{AlignCenter, "Center"},
		{AlignRight, "Right"},
		{AlignJustify, "Justify"},
		{Alignment(99), unknownStr},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.alignment.String()
			if got != tt.want {
				t.Errorf("Alignment(%d).String() = %q, want %q", tt.alignment, got, tt.want)
			}
		})
	}
}

// TestLayoutText_MaxWidth tests line wrapping at MaxWidth.
func TestLayoutText_MaxWidth(t *testing.T) {
	face := layoutTestFace(t)

	// Long text that should wrap
	text := "The quick brown fox jumps over the lazy dog"

	opts := LayoutOptions{
		MaxWidth:    100, // Narrow width to force wrapping
		LineSpacing: 1.0,
		Alignment:   AlignLeft,
		Direction:   DirectionLTR,
	}

	layout := LayoutText(text, face, 16.0, opts)

	if len(layout.Lines) <= 1 {
		t.Errorf("expected multiple lines with MaxWidth=100, got %d", len(layout.Lines))
	}

	// Each line should not exceed MaxWidth (with some tolerance)
	for i, line := range layout.Lines {
		if line.Width > opts.MaxWidth*1.5 {
			t.Errorf("line %d width %f exceeds MaxWidth %f significantly",
				i, line.Width, opts.MaxWidth)
		}
	}
}

// TestLayoutText_NoWrap tests that wrapping is disabled when MaxWidth is 0.
func TestLayoutText_NoWrap(t *testing.T) {
	face := layoutTestFace(t)
	text := "The quick brown fox jumps over the lazy dog"

	opts := LayoutOptions{
		MaxWidth:    0, // No wrapping
		LineSpacing: 1.0,
		Alignment:   AlignLeft,
		Direction:   DirectionLTR,
	}

	layout := LayoutText(text, face, 16.0, opts)

	if len(layout.Lines) != 1 {
		t.Errorf("expected 1 line without wrapping, got %d", len(layout.Lines))
	}
}

// TestLayoutText_Bidi tests bidirectional text.
func TestLayoutText_Bidi(t *testing.T) {
	face := layoutTestFace(t)

	tests := []struct {
		name string
		text string
		dir  Direction
	}{
		{"Latin only", "Hello World", DirectionLTR},
		{"With Arabic", "Hello مرحبا World", DirectionLTR},
		{"With Hebrew", "Hello שלום World", DirectionLTR},
		{"RTL base", "مرحبا Hello", DirectionRTL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := LayoutOptions{
				MaxWidth:    0,
				LineSpacing: 1.0,
				Alignment:   AlignLeft,
				Direction:   tt.dir,
			}

			layout := LayoutText(tt.text, face, 16.0, opts)

			if layout == nil {
				t.Fatal("LayoutText returned nil")
			}
			if len(layout.Lines) != 1 {
				t.Errorf("expected 1 line, got %d", len(layout.Lines))
			}

			// Verify glyphs exist
			if len(layout.Lines[0].Glyphs) == 0 {
				t.Error("expected glyphs in line")
			}
		})
	}
}

// TestLayoutText_LineSpacing tests custom line spacing.
func TestLayoutText_LineSpacing(t *testing.T) {
	face := layoutTestFace(t)
	text := "Line One\nLine Two"

	// Layout with default spacing
	opts1 := LayoutOptions{
		MaxWidth:    0,
		LineSpacing: 1.0,
		Alignment:   AlignLeft,
	}
	layout1 := LayoutText(text, face, 16.0, opts1)

	// Layout with 1.5x spacing
	opts2 := LayoutOptions{
		MaxWidth:    0,
		LineSpacing: 1.5,
		Alignment:   AlignLeft,
	}
	layout2 := LayoutText(text, face, 16.0, opts2)

	if layout2.Height <= layout1.Height {
		t.Errorf("1.5x line spacing height (%f) should be greater than 1.0x (%f)",
			layout2.Height, layout1.Height)
	}
}

// TestLayoutText_ZeroLineSpacing tests that zero line spacing defaults to 1.0.
func TestLayoutText_ZeroLineSpacing(t *testing.T) {
	face := layoutTestFace(t)
	text := "Line One\nLine Two"

	optsZero := LayoutOptions{
		MaxWidth:    0,
		LineSpacing: 0, // Should default to 1.0
		Alignment:   AlignLeft,
	}

	optsDefault := DefaultLayoutOptions()

	layoutZero := LayoutText(text, face, 16.0, optsZero)
	layoutDefault := LayoutText(text, face, 16.0, optsDefault)

	// Heights should be equal since 0 defaults to 1.0
	if layoutZero.Height != layoutDefault.Height {
		t.Errorf("zero line spacing height (%f) should equal default (%f)",
			layoutZero.Height, layoutDefault.Height)
	}
}

// TestLayoutTextSimple tests the convenience function.
func TestLayoutTextSimple(t *testing.T) {
	face := layoutTestFace(t)
	text := "Hello World"

	layout := LayoutTextSimple(text, face, 16.0)

	if layout == nil {
		t.Fatal("LayoutTextSimple returned nil")
	}
	if len(layout.Lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(layout.Lines))
	}
}

// TestDefaultLayoutOptions tests default options.
func TestDefaultLayoutOptions(t *testing.T) {
	opts := DefaultLayoutOptions()

	if opts.MaxWidth != 0 {
		t.Errorf("default MaxWidth should be 0, got %f", opts.MaxWidth)
	}
	if opts.LineSpacing != 1.0 {
		t.Errorf("default LineSpacing should be 1.0, got %f", opts.LineSpacing)
	}
	if opts.Alignment != AlignLeft {
		t.Errorf("default Alignment should be AlignLeft, got %v", opts.Alignment)
	}
	if opts.Direction != DirectionLTR {
		t.Errorf("default Direction should be DirectionLTR, got %v", opts.Direction)
	}
}

// TestLine_Height tests Line.Height method.
func TestLine_Height(t *testing.T) {
	line := Line{
		Ascent:  10.0,
		Descent: 5.0,
	}

	height := line.Height()
	expected := 15.0

	if height != expected {
		t.Errorf("Line.Height() = %f, want %f", height, expected)
	}
}

// TestLayoutText_GlyphPositions tests that glyph positions are increasing.
func TestLayoutText_GlyphPositions(t *testing.T) {
	face := layoutTestFace(t)
	text := "ABCDEFG"

	layout := LayoutText(text, face, 16.0, DefaultLayoutOptions())

	if len(layout.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(layout.Lines))
	}

	glyphs := layout.Lines[0].Glyphs
	if len(glyphs) != 7 {
		t.Fatalf("expected 7 glyphs, got %d", len(glyphs))
	}

	// Positions should be increasing
	var prevX float64
	for i, g := range glyphs {
		if i > 0 && g.X <= prevX {
			t.Errorf("glyph %d X (%f) should be greater than glyph %d X (%f)",
				i, g.X, i-1, prevX)
		}
		prevX = g.X
	}
}

// TestLayoutText_Runs tests that runs are created correctly.
func TestLayoutText_Runs(t *testing.T) {
	face := layoutTestFace(t)
	text := "Hello World"

	layout := LayoutText(text, face, 16.0, DefaultLayoutOptions())

	if len(layout.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(layout.Lines))
	}

	line := &layout.Lines[0]

	// Should have at least one run
	if len(line.Runs) == 0 {
		t.Error("expected at least one run")
	}

	// Total glyphs in runs should match line glyphs
	totalRunGlyphs := 0
	for _, run := range line.Runs {
		totalRunGlyphs += len(run.Glyphs)
	}

	if totalRunGlyphs != len(line.Glyphs) {
		t.Errorf("run glyphs (%d) should match line glyphs (%d)",
			totalRunGlyphs, len(line.Glyphs))
	}
}

// TestLayoutText_LongText tests layout of long text.
func TestLayoutText_LongText(t *testing.T) {
	face := layoutTestFace(t)

	// Generate long text
	var builder strings.Builder
	for i := 0; i < 100; i++ {
		builder.WriteString("The quick brown fox jumps over the lazy dog. ")
	}
	text := builder.String()

	opts := LayoutOptions{
		MaxWidth:    500,
		LineSpacing: 1.0,
		Alignment:   AlignLeft,
	}

	layout := LayoutText(text, face, 16.0, opts)

	if layout == nil {
		t.Fatal("LayoutText returned nil")
	}
	if len(layout.Lines) <= 1 {
		t.Error("expected multiple lines for long text")
	}

	// Verify all lines have reasonable width
	for i, line := range layout.Lines {
		if line.Width > opts.MaxWidth*2 {
			t.Errorf("line %d width %f is unreasonably large", i, line.Width)
		}
	}
}

// TestLayoutText_SingleCharacter tests layout of single character.
func TestLayoutText_SingleCharacter(t *testing.T) {
	face := layoutTestFace(t)

	layout := LayoutText("A", face, 16.0, DefaultLayoutOptions())

	if len(layout.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(layout.Lines))
	}
	if len(layout.Lines[0].Glyphs) != 1 {
		t.Errorf("expected 1 glyph, got %d", len(layout.Lines[0].Glyphs))
	}
}

// TestLayoutText_Whitespace tests layout of whitespace-only text.
func TestLayoutText_Whitespace(t *testing.T) {
	face := layoutTestFace(t)

	layout := LayoutText("   ", face, 16.0, DefaultLayoutOptions())

	if layout == nil {
		t.Fatal("LayoutText returned nil")
	}
	if len(layout.Lines) != 1 {
		t.Errorf("expected 1 line for whitespace, got %d", len(layout.Lines))
	}
	// Whitespace should have glyphs (space characters)
	if len(layout.Lines[0].Glyphs) != 3 {
		t.Errorf("expected 3 glyphs for 3 spaces, got %d", len(layout.Lines[0].Glyphs))
	}
}

// TestIsWordBreakRune tests word break detection.
func TestIsWordBreakRune(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'a', false},
		{'A', false},
		{'1', false},
		{'.', false},
		// CJK characters can break anywhere
		{'\u4E00', true}, // CJK ideograph
		{'\u3042', true}, // Hiragana
		{'\u30A2', true}, // Katakana
		{'\uAC00', true}, // Hangul
	}

	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			got := isWordBreakRune(tt.r)
			if got != tt.want {
				t.Errorf("isWordBreakRune(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

// TestIsCJK tests CJK character detection.
func TestIsCJK(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"CJK ideograph", '\u4E00', true},
		{"CJK ideograph end", '\u9FFF', true},
		{"CJK Extension A", '\u3400', true},
		{"Hiragana A", '\u3042', true},
		{"Hiragana N", '\u3093', true},
		{"Katakana A", '\u30A2', true},
		{"Katakana N", '\u30F3', true},
		{"Hangul syllable", '\uAC00', true},
		{"Latin A", 'A', false},
		{"Latin a", 'a', false},
		{"Space", ' ', false},
		{"Digit", '1', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCJK(tt.r)
			if got != tt.want {
				t.Errorf("isCJK(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

// TestLayoutText_DifferentSizes tests layout at different font sizes.
func TestLayoutText_DifferentSizes(t *testing.T) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	text := "Hello World"
	sizes := []float64{8, 12, 16, 24, 32, 48}
	var prevWidth, prevHeight float64

	for _, size := range sizes {
		face := source.Face(size)
		layout := LayoutText(text, face, size, DefaultLayoutOptions())

		if len(layout.Lines) != 1 {
			t.Errorf("size %f: expected 1 line, got %d", size, len(layout.Lines))
			continue
		}

		// Width and height should increase with size
		if size > 8 {
			if layout.Width <= prevWidth {
				t.Errorf("size %f: width %f should be > %f", size, layout.Width, prevWidth)
			}
			if layout.Height <= prevHeight {
				t.Errorf("size %f: height %f should be > %f", size, layout.Height, prevHeight)
			}
		}

		prevWidth = layout.Width
		prevHeight = layout.Height
	}
}

// TestSplitParagraphs tests paragraph splitting.
func TestSplitParagraphs(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 1},
		{"single line", "Hello", 1},
		{"two lines LF", "Line1\nLine2", 2},
		{"two lines CRLF", "Line1\r\nLine2", 2},
		{"two lines CR", "Line1\rLine2", 2},
		{"three lines", "Line1\nLine2\nLine3", 3},
		{"empty middle", "Line1\n\nLine3", 3},
		{"trailing newline", "Line1\n", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitParagraphs(tt.text)
			if len(got) != tt.want {
				t.Errorf("splitParagraphs(%q) = %d paragraphs, want %d", tt.text, len(got), tt.want)
			}
		})
	}
}

// BenchmarkLayoutText benchmarks the LayoutText function.
func BenchmarkLayoutText(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	text := "The quick brown fox jumps over the lazy dog."
	opts := DefaultLayoutOptions()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = LayoutText(text, face, 16.0, opts)
	}
}

// BenchmarkLayoutText_MultiLine benchmarks multi-line layout.
func BenchmarkLayoutText_MultiLine(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	text := "Line One\nLine Two\nLine Three\nLine Four\nLine Five"
	opts := DefaultLayoutOptions()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = LayoutText(text, face, 16.0, opts)
	}
}

// BenchmarkLayoutText_Wrapped benchmarks line wrapping.
func BenchmarkLayoutText_Wrapped(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatalf("failed to create font source: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)
	text := "The quick brown fox jumps over the lazy dog. " +
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit."
	opts := LayoutOptions{
		MaxWidth:    200,
		LineSpacing: 1.0,
		Alignment:   AlignLeft,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = LayoutText(text, face, 16.0, opts)
	}
}

// TestLayoutText_WrappedLineYPositions verifies that wrapped lines within a
// single paragraph get sequential (increasing) Y positions, not all the same Y.
// Regression test for #138: all wrapped lines had identical Y = line.Ascent.
func TestLayoutText_WrappedLineYPositions(t *testing.T) {
	face := layoutTestFace(t)

	// Long text that will wrap into multiple lines at narrow width
	text := "The quick brown fox jumps over the lazy dog and keeps running around the park"

	opts := LayoutOptions{
		MaxWidth:    100,
		LineSpacing: 1.0,
		Alignment:   AlignLeft,
		Direction:   DirectionLTR,
	}

	layout := LayoutText(text, face, 16.0, opts)

	if len(layout.Lines) < 3 {
		t.Fatalf("expected at least 3 wrapped lines, got %d", len(layout.Lines))
	}

	// Each line Y must be strictly greater than previous
	for i := 1; i < len(layout.Lines); i++ {
		if layout.Lines[i].Y <= layout.Lines[i-1].Y {
			t.Errorf("line %d Y=%f should be > line %d Y=%f",
				i, layout.Lines[i].Y, i-1, layout.Lines[i-1].Y)
		}
	}

	// Height must account for all lines, not just the first
	expectedMinHeight := layout.Lines[len(layout.Lines)-1].Y
	if layout.Height < expectedMinHeight {
		t.Errorf("layout.Height=%f too small, last line Y=%f",
			layout.Height, expectedMinHeight)
	}
}

// TestLayoutText_WrappedMultiParagraphY verifies Y positions across both
// wrapped lines within a paragraph and across paragraph boundaries.
func TestLayoutText_WrappedMultiParagraphY(t *testing.T) {
	face := layoutTestFace(t)

	// Two paragraphs, each will wrap
	text := "The quick brown fox jumps over the lazy dog\nThe lazy dog sleeps under the old oak tree"

	opts := LayoutOptions{
		MaxWidth:    120,
		LineSpacing: 1.0,
		Alignment:   AlignLeft,
		Direction:   DirectionLTR,
	}

	layout := LayoutText(text, face, 16.0, opts)

	if len(layout.Lines) < 4 {
		t.Fatalf("expected at least 4 lines (2 paragraphs wrapped), got %d", len(layout.Lines))
	}

	// ALL lines must have strictly increasing Y
	for i := 1; i < len(layout.Lines); i++ {
		if layout.Lines[i].Y <= layout.Lines[i-1].Y {
			t.Errorf("line %d Y=%f should be > line %d Y=%f",
				i, layout.Lines[i].Y, i-1, layout.Lines[i-1].Y)
		}
	}
}
