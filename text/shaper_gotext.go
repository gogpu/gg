package text

import (
	"bytes"
	"sync"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// GoTextShaper provides HarfBuzz-level text shaping using go-text/typesetting.
// It supports advanced OpenType features including:
//   - Ligature substitution (fi, fl, ffi, etc.)
//   - Kerning pairs (AV, To, etc.)
//   - Contextual alternates
//   - Right-to-left text (Arabic, Hebrew)
//   - Complex scripts (Devanagari, Thai, etc.)
//
// GoTextShaper is an opt-in replacement for BuiltinShaper. To use it:
//
//	shaper := text.NewGoTextShaper()
//	text.SetShaper(shaper)
//	defer text.SetShaper(nil) // Reset to default BuiltinShaper
//
// GoTextShaper is safe for concurrent use. It caches parsed font.Font objects
// (which are thread-safe) and creates lightweight font.Face instances per
// Shape() call (font.Face is NOT safe for concurrent use). The HarfbuzzShaper
// instances are pooled via sync.Pool since they also are not concurrent-safe.
type GoTextShaper struct {
	// shaperPool pools HarfbuzzShaper instances for concurrent use.
	// HarfbuzzShaper has internal mutable state (buffer) and is NOT safe
	// for concurrent use, but reusing across sequential calls is efficient.
	shaperPool sync.Pool

	// mu protects the font cache.
	mu sync.RWMutex

	// fontCache maps FontSource pointers to parsed go-text Font objects.
	// font.Font is read-only and safe for concurrent use, unlike font.Face.
	// This avoids re-parsing the font data on every Shape() call.
	fontCache map[*FontSource]*font.Font
}

// NewGoTextShaper creates a new GoTextShaper backed by go-text/typesetting's
// HarfBuzz implementation.
func NewGoTextShaper() *GoTextShaper {
	return &GoTextShaper{
		shaperPool: sync.Pool{
			New: func() any {
				return &shaping.HarfbuzzShaper{}
			},
		},
		fontCache: make(map[*FontSource]*font.Font),
	}
}

// Shape implements the Shaper interface.
// It converts text into positioned glyphs using HarfBuzz shaping via go-text/typesetting.
// The font size is obtained from face.Size().
// This produces higher-quality output than BuiltinShaper for text that benefits
// from kerning, ligatures, or complex script shaping.
func (s *GoTextShaper) Shape(text string, face Face) []ShapedGlyph {
	if text == "" || face == nil {
		return nil
	}

	source := face.Source()
	if source == nil {
		return nil
	}

	// Get or create the cached go-text Font for this source.
	goTextFont, err := s.getOrCreateFont(source)
	if err != nil {
		// Fall back: return nil on font parsing error.
		// In production, users should validate their fonts upfront.
		return nil
	}

	// Create a lightweight font.Face for this shaping call.
	// font.Face is NOT safe for concurrent use, so each Shape() call
	// gets its own instance. font.NewFace is cheap — it wraps the
	// thread-safe *Font and initializes glyph caches.
	goTextFace := font.NewFace(goTextFont)

	size := face.Size()
	runes := []rune(text)

	// Convert our Direction to go-text's di.Direction.
	dir := mapDirection(face.Direction())

	// Detect script from the first non-space rune.
	script := detectScript(runes)

	// Build shaping input.
	input := shaping.Input{
		Text:      runes,
		RunStart:  0,
		RunEnd:    len(runes),
		Direction: dir,
		Face:      goTextFace,
		Size:      floatToFixed(size),
		Script:    script,
		Language:  language.NewLanguage("en"),
	}

	// Get a HarfbuzzShaper from the pool (not concurrent-safe, so each
	// goroutine needs its own instance).
	hbShaper := s.shaperPool.Get().(*shaping.HarfbuzzShaper)
	output := hbShaper.Shape(input)
	s.shaperPool.Put(hbShaper)

	// Convert go-text glyphs to our ShapedGlyph format.
	return convertGlyphs(output.Glyphs, dir)
}

// getOrCreateFont returns a cached go-text font.Font for the given source,
// or parses the font data and caches the Font (not Face).
// font.Font is read-only and safe for concurrent use.
func (s *GoTextShaper) getOrCreateFont(source *FontSource) (*font.Font, error) {
	// Fast path: check cache with read lock.
	s.mu.RLock()
	if f, ok := s.fontCache[source]; ok {
		s.mu.RUnlock()
		return f, nil
	}
	s.mu.RUnlock()

	// Slow path: parse font and update cache with write lock.
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock.
	if f, ok := s.fontCache[source]; ok {
		return f, nil
	}

	// Parse font data using go-text/typesetting.
	// ParseTTF returns a *Face which embeds the thread-safe *Font.
	reader := bytes.NewReader(source.data)
	goTextFace, err := font.ParseTTF(reader)
	if err != nil {
		return nil, err
	}

	// Cache the Font (thread-safe), not the Face.
	s.fontCache[source] = goTextFace.Font
	return goTextFace.Font, nil
}

// ClearCache removes all cached parsed fonts.
// Call this if you no longer need previously loaded fonts and want to free memory.
func (s *GoTextShaper) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fontCache = make(map[*FontSource]*font.Font)
}

// RemoveSource removes the cached parsed font for a specific FontSource.
// This is useful when a FontSource is closed.
func (s *GoTextShaper) RemoveSource(source *FontSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.fontCache, source)
}

// mapDirection converts our text.Direction to go-text's di.Direction.
func mapDirection(d Direction) di.Direction {
	switch d {
	case DirectionRTL:
		return di.DirectionRTL
	case DirectionTTB:
		return di.DirectionTTB
	case DirectionBTT:
		return di.DirectionBTT
	default:
		return di.DirectionLTR
	}
}

// detectScript inspects the runes and returns the script of the first
// non-space character. This is a simple heuristic; for mixed-script text,
// users should split runs by script before shaping.
func detectScript(runes []rune) language.Script {
	for _, r := range runes {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		return language.LookupScript(r)
	}
	return language.Latin
}

// floatToFixed converts a float64 font size to fixed.Int26_6.
// The fixed-point representation uses 6 fractional bits, so we multiply by 64.
func floatToFixed(size float64) fixed.Int26_6 {
	return fixed.Int26_6(size * 64)
}

// fixedToFloat converts a fixed.Int26_6 value to float64.
func fixedToFloat(v fixed.Int26_6) float64 {
	return float64(v) / 64.0
}

// convertGlyphs converts go-text/typesetting output glyphs to our ShapedGlyph slice.
func convertGlyphs(glyphs []shaping.Glyph, dir di.Direction) []ShapedGlyph {
	if len(glyphs) == 0 {
		return nil
	}

	result := make([]ShapedGlyph, len(glyphs))

	var x, y float64

	for i, g := range glyphs {
		// XOffset and YOffset represent fine-grained positioning adjustments
		// applied on top of the current pen position.
		xOff := fixedToFloat(g.XOffset)
		yOff := fixedToFloat(g.YOffset)

		result[i] = ShapedGlyph{
			GID:     GlyphID(uint16(g.GlyphID)), //nolint:gosec // GlyphID is uint16 by design; overflow is handled by font subsetting
			Cluster: g.TextIndex(),
			X:       x + xOff,
			Y:       y + yOff,
		}

		// Advance the pen position.
		if dir.IsVertical() {
			adv := fixedToFloat(g.Advance)
			result[i].YAdvance = adv
			y += adv
		} else {
			adv := fixedToFloat(g.Advance)
			result[i].XAdvance = adv
			x += adv
		}
	}

	return result
}
