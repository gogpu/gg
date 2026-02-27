package text

import "sync"

// Shaper converts text to positioned glyphs.
// Implementations provide different levels of text shaping support:
//   - BuiltinShaper: Uses golang.org/x/image/font for Latin, Cyrillic, Greek, CJK
//   - HarfBuzz-compatible: Use SetShaper() with a go-text/typesetting implementation
type Shaper interface {
	// Shape converts text into positioned glyphs using the given face.
	// The font size is obtained from face.Size().
	// The returned ShapedGlyph slice is ready for GPU rendering.
	Shape(text string, face Face) []ShapedGlyph
}

var (
	shaperMu     sync.RWMutex
	globalShaper Shaper = &BuiltinShaper{}
)

// SetShaper sets the global shaper used by Shape().
// Pass nil to reset to the default BuiltinShaper.
//
// Example usage with a custom shaper:
//
//	text.SetShaper(myHarfBuzzShaper)
//	defer text.SetShaper(nil) // Reset to default
func SetShaper(s Shaper) {
	shaperMu.Lock()
	defer shaperMu.Unlock()
	if s == nil {
		s = &BuiltinShaper{}
	}
	globalShaper = s
}

// GetShaper returns the current global shaper.
func GetShaper() Shaper {
	shaperMu.RLock()
	defer shaperMu.RUnlock()
	return globalShaper
}

// Shape is a convenience function that uses the global shaper.
// It converts text to positioned glyphs using the given face.
// The font size is obtained from face.Size().
func Shape(text string, face Face) []ShapedGlyph {
	return GetShaper().Shape(text, face)
}
