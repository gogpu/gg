package gg

import (
	"os"
	"strings"
	"testing"

	"github.com/gogpu/gg/text"
)

// findTestFont returns a path to a system font suitable for testing.
// Returns empty string if no font is found.
func findTestFont() string {
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		// macOS
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		// Linux
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// --------------------------------------------------------------------------
// Align constants
// --------------------------------------------------------------------------

func TestAlignConstants(t *testing.T) {
	// Verify gg.AlignLeft/Center/Right match text package values
	if AlignLeft != text.AlignLeft {
		t.Errorf("AlignLeft mismatch: gg=%d text=%d", AlignLeft, text.AlignLeft)
	}
	if AlignCenter != text.AlignCenter {
		t.Errorf("AlignCenter mismatch: gg=%d text=%d", AlignCenter, text.AlignCenter)
	}
	if AlignRight != text.AlignRight {
		t.Errorf("AlignRight mismatch: gg=%d text=%d", AlignRight, text.AlignRight)
	}

	// Verify they can be used as Align type
	a := AlignCenter
	if a != text.AlignCenter {
		t.Errorf("Align type alias not working: got %d", a)
	}
}

// --------------------------------------------------------------------------
// WordWrap
// --------------------------------------------------------------------------

func TestWordWrap_NoFont(t *testing.T) {
	dc := NewContext(400, 200)
	defer dc.Close()

	// Without a font, WordWrap should return input as single element
	lines := dc.WordWrap("Hello world", 100)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line without font, got %d", len(lines))
	}
	if lines[0] != "Hello world" {
		t.Errorf("expected original string, got %q", lines[0])
	}
}

func TestWordWrap(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(800, 400)
	defer dc.Close()

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc.SetFont(source.Face(16.0))

	// Single word that fits
	lines := dc.WordWrap("Hello", 1000)
	if len(lines) != 1 {
		t.Errorf("short text: expected 1 line, got %d", len(lines))
	}

	// Long text that needs wrapping
	longText := "The quick brown fox jumps over the lazy dog and runs through the forest"
	lines = dc.WordWrap(longText, 200)
	if len(lines) < 2 {
		t.Errorf("long text with 200px width: expected multiple lines, got %d", len(lines))
	}

	// Verify all text is preserved
	joined := strings.Join(lines, " ")
	// Wrapped text may have trimmed whitespace at line boundaries
	if !strings.Contains(joined, "quick") || !strings.Contains(joined, "forest") {
		t.Errorf("wrapped text lost content: %q", joined)
	}

	// Very wide width — should not wrap
	lines = dc.WordWrap(longText, 10000)
	if len(lines) != 1 {
		t.Errorf("very wide width: expected 1 line, got %d", len(lines))
	}
}

func TestWordWrap_EmptyString(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(400, 200)
	defer dc.Close()

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc.SetFont(source.Face(16.0))

	lines := dc.WordWrap("", 200)
	if len(lines) != 1 {
		t.Fatalf("empty string: expected 1 line, got %d", len(lines))
	}
	if lines[0] != "" {
		t.Errorf("empty string: expected empty line, got %q", lines[0])
	}
}

// --------------------------------------------------------------------------
// MeasureMultilineString
// --------------------------------------------------------------------------

func TestMeasureMultilineString_NoFont(t *testing.T) {
	dc := NewContext(400, 200)
	defer dc.Close()

	w, h := dc.MeasureMultilineString("Hello\nWorld", 1.0)
	if w != 0 || h != 0 {
		t.Errorf("no font: expected (0, 0), got (%f, %f)", w, h)
	}
}

func TestMeasureMultilineString(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(400, 200)
	defer dc.Close()

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc.SetFont(source.Face(24.0))

	// Single line
	w1, h1 := dc.MeasureMultilineString("Hello", 1.0)
	if w1 <= 0 || h1 <= 0 {
		t.Errorf("single line: expected positive dims, got (%f, %f)", w1, h1)
	}

	// Two lines — height should be greater than single line
	w2, h2 := dc.MeasureMultilineString("Hello\nWorld", 1.0)
	if h2 <= h1 {
		t.Errorf("two lines height (%f) should be > single line height (%f)", h2, h1)
	}
	// Width should be >= single line width (both words measured)
	if w2 <= 0 {
		t.Errorf("two lines: expected positive width, got %f", w2)
	}

	// Three lines
	_, h3 := dc.MeasureMultilineString("A\nB\nC", 1.0)
	if h3 <= h2 {
		t.Errorf("three lines height (%f) should be > two lines height (%f)", h3, h2)
	}
}

func TestMeasureMultilineString_LineSpacing(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(400, 200)
	defer dc.Close()

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc.SetFont(source.Face(24.0))

	text2Lines := "Hello\nWorld"

	_, h10 := dc.MeasureMultilineString(text2Lines, 1.0)
	_, h15 := dc.MeasureMultilineString(text2Lines, 1.5)
	_, h20 := dc.MeasureMultilineString(text2Lines, 2.0)

	if h15 <= h10 {
		t.Errorf("1.5x spacing (%f) should be > 1.0x spacing (%f)", h15, h10)
	}
	if h20 <= h15 {
		t.Errorf("2.0x spacing (%f) should be > 1.5x spacing (%f)", h20, h15)
	}
}

func TestMeasureMultilineString_CRLFNormalization(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(400, 200)
	defer dc.Close()

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc.SetFont(source.Face(16.0))

	// \n, \r\n, and \r should all produce the same result
	_, hLF := dc.MeasureMultilineString("A\nB\nC", 1.0)
	_, hCRLF := dc.MeasureMultilineString("A\r\nB\r\nC", 1.0)
	_, hCR := dc.MeasureMultilineString("A\rB\rC", 1.0)

	if hLF != hCRLF {
		t.Errorf("\\n height (%f) != \\r\\n height (%f)", hLF, hCRLF)
	}
	if hLF != hCR {
		t.Errorf("\\n height (%f) != \\r height (%f)", hLF, hCR)
	}
}

// --------------------------------------------------------------------------
// DrawStringWrapped
// --------------------------------------------------------------------------

func TestDrawStringWrapped_NoFont(t *testing.T) {
	dc := NewContext(400, 200)
	defer dc.Close()

	// Should not panic with no font set
	dc.DrawStringWrapped("Hello World", 0, 0, 0, 0, 200, 1.0, AlignLeft)
	dc.DrawStringWrapped("Hello World", 200, 100, 0.5, 0.5, 200, 1.5, AlignCenter)
	dc.DrawStringWrapped("Hello World", 400, 200, 1, 1, 200, 2.0, AlignRight)
}

func TestDrawStringWrapped_DrawsPixels(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(400, 400)
	defer dc.Close()
	dc.ClearWithColor(White)

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc.SetFont(source.Face(24.0))
	dc.SetRGB(0, 0, 0) // Black text

	longText := "The quick brown fox jumps over the lazy dog"
	dc.DrawStringWrapped(longText, 10, 30, 0, 0, 380, 1.5, AlignLeft)

	// Verify pixels were modified
	nonWhiteCount := 0
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			pixel := dc.pixmap.GetPixel(x, y)
			if pixel.R < 0.99 || pixel.G < 0.99 || pixel.B < 0.99 {
				nonWhiteCount++
			}
		}
	}

	if nonWhiteCount == 0 {
		t.Error("DrawStringWrapped produced no visible pixels")
	}
}

func TestDrawStringWrapped_Alignment(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(400, 600)
	defer dc.Close()
	dc.ClearWithColor(White)

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc.SetFont(source.Face(16.0))
	dc.SetRGB(0, 0, 0)

	shortText := "Hi"
	width := 300.0

	// Draw left-aligned and right-aligned text
	// Left-aligned: text starts near x=50
	dc.DrawStringWrapped(shortText, 50, 50, 0, 0, width, 1.0, AlignLeft)

	// Right-aligned: text ends near x=50+width
	dc.DrawStringWrapped(shortText, 50, 150, 0, 0, width, 1.0, AlignRight)

	// Check that left-aligned text has ink on the left side
	leftInk := false
	for y := 40; y < 80; y++ {
		for x := 40; x < 100; x++ {
			p := dc.pixmap.GetPixel(x, y)
			if p.R < 0.99 {
				leftInk = true
				break
			}
		}
		if leftInk {
			break
		}
	}

	// Check that right-aligned text has ink on the right side
	rightInk := false
	for y := 140; y < 180; y++ {
		for x := 300; x < 360; x++ {
			p := dc.pixmap.GetPixel(x, y)
			if p.R < 0.99 {
				rightInk = true
				break
			}
		}
		if rightInk {
			break
		}
	}

	if !leftInk {
		t.Error("Left-aligned text has no ink on the left side")
	}
	if !rightInk {
		t.Error("Right-aligned text has no ink on the right side")
	}
}

// --------------------------------------------------------------------------
// fontHeight
// --------------------------------------------------------------------------

func TestFontHeight(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(400, 200)
	defer dc.Close()

	// No font: fontHeight returns 0
	if fh := dc.fontHeight(); fh != 0 {
		t.Errorf("no font: expected fontHeight=0, got %f", fh)
	}

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc.SetFont(source.Face(24.0))

	fh := dc.fontHeight()
	if fh <= 0 {
		t.Errorf("with font: expected positive fontHeight, got %f", fh)
	}

	// fontHeight should match metrics.LineHeight()
	expected := dc.face.Metrics().LineHeight()
	if fh != expected {
		t.Errorf("fontHeight=%f != metrics.LineHeight()=%f", fh, expected)
	}
}

// --------------------------------------------------------------------------
// splitLines
// --------------------------------------------------------------------------

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected number of lines
	}{
		{"empty", "", 1},
		{"single line", "hello", 1},
		{"LF", "a\nb\nc", 3},
		{"CRLF", "a\r\nb\r\nc", 3},
		{"CR", "a\rb\rc", 3},
		{"mixed", "a\nb\r\nc\rd", 4},
		{"trailing newline", "hello\n", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := splitLines(tt.input)
			if len(lines) != tt.want {
				t.Errorf("splitLines(%q): got %d lines, want %d", tt.input, len(lines), tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// MeasureMultilineString + DrawStringWrapped consistency
// --------------------------------------------------------------------------

func TestDrawStringWrapped_HeightConsistency(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(400, 400)
	defer dc.Close()

	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() { _ = source.Close() }()

	dc.SetFont(source.Face(20.0))

	s := "Line one\nLine two\nLine three"
	lineSpacing := 1.5

	_, mh := dc.MeasureMultilineString(s, lineSpacing)

	// The measured height should be consistent with what DrawStringWrapped uses.
	// With anchor (0, 0) and (0, 1), the difference in y should equal the height.
	fh := dc.fontHeight()
	lines := splitLines(s)
	n := float64(len(lines))
	h := n*fh*lineSpacing - (lineSpacing-1)*fh

	if mh != h {
		t.Errorf("MeasureMultilineString height (%f) != computed height (%f)", mh, h)
	}
}
