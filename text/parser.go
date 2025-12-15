package text

// FontParser is an interface for font parsing backends.
// This abstraction allows swapping the font parsing library
// (e.g., golang.org/x/image/font/opentype vs a pure Go implementation).
//
// The default implementation uses golang.org/x/image/font/opentype.
type FontParser interface {
	// Parse parses font data (TTF or OTF) and returns a ParsedFont.
	Parse(data []byte) (ParsedFont, error)
}

// ParsedFont represents a parsed font file.
// This interface abstracts the underlying font representation.
type ParsedFont interface {
	// Name returns the font family name.
	// Returns empty string if not available.
	Name() string

	// FullName returns the full font name.
	// Returns empty string if not available.
	FullName() string

	// NumGlyphs returns the number of glyphs in the font.
	NumGlyphs() int

	// UnitsPerEm returns the units per em for the font.
	UnitsPerEm() int

	// GlyphIndex returns the glyph index for a rune.
	// Returns 0 if the glyph is not found.
	GlyphIndex(r rune) uint16

	// GlyphAdvance returns the advance width for a glyph at the given size (in points).
	// The ppem (pixels per em) is derived from size and DPI.
	GlyphAdvance(glyphIndex uint16, ppem float64) float64

	// GlyphBounds returns the bounding box for a glyph at the given size.
	GlyphBounds(glyphIndex uint16, ppem float64) Rect

	// Metrics returns the font metrics at the given size.
	Metrics(ppem float64) FontMetrics
}

// FontMetrics holds font-level metrics at a specific size.
type FontMetrics struct {
	// Ascent is the distance from the baseline to the top of the font (positive).
	Ascent float64

	// Descent is the distance from the baseline to the bottom of the font (negative).
	Descent float64

	// LineGap is the recommended line gap between lines.
	LineGap float64

	// XHeight is the height of lowercase letters (like 'x').
	XHeight float64

	// CapHeight is the height of uppercase letters.
	CapHeight float64
}

// Height returns the total line height (ascent - descent + line gap).
func (m FontMetrics) Height() float64 {
	return m.Ascent - m.Descent + m.LineGap
}

// parserRegistry holds registered font parsers.
// The default parser is "ximage" (golang.org/x/image).
var parserRegistry = map[string]FontParser{
	"ximage": &ximageParser{},
}

// defaultParserName is the name of the default parser.
const defaultParserName = "ximage"

// RegisterParser registers a custom font parser.
// This allows users to provide their own parsing implementation.
func RegisterParser(name string, parser FontParser) {
	parserRegistry[name] = parser
}

// getParser returns the parser by name, or the default if not found.
func getParser(name string) FontParser {
	if p, ok := parserRegistry[name]; ok {
		return p
	}
	return parserRegistry[defaultParserName]
}
