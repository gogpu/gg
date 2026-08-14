package text

import (
	"sync"
	"unicode/utf8"
)

// Shaper converts text to positioned glyphs.
// Implementations provide different levels of text shaping support:
//   - OwnShaper: Pure Go shaper with GSUB/GPOS support (default, ADR-048)
//   - BuiltinShaper: Simple LTR shaper for Latin, Cyrillic, Greek, CJK (no GSUB/GPOS)
//   - BuiltinShaper: Simple LTR shaper for scripts without GSUB/GPOS (legacy)
type Shaper interface {
	// Shape converts text into positioned glyphs using the given face.
	// The font size is obtained from face.Size().
	// The returned ShapedGlyph slice is ready for GPU rendering.
	Shape(text string, face Face) []ShapedGlyph
}

// defaultShaper is initialized to OwnShaper in shaper_own.go init().
// This variable is set before any concurrent access (during init).
var defaultOwnShaper = NewOwnShaper()

var (
	shaperMu     sync.RWMutex
	globalShaper Shaper = defaultOwnShaper
)

// SetShaper sets the global shaper used by Shape().
// Pass nil to reset to the default OwnShaper (Pure Go GSUB/GPOS).
//
// Example usage with a custom shaper:
//
//	text.SetShaper(myCustomShaper)
//	defer text.SetShaper(nil) // Reset to default
func SetShaper(s Shaper) {
	shaperMu.Lock()
	defer shaperMu.Unlock()
	if s == nil {
		s = defaultOwnShaper
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
	if text == "" || face == nil {
		return nil
	}

	shaper := GetShaper()
	if face.Source() == nil {
		if runsFace, ok := face.(interface{ FontRuns(string) []FontRun }); ok {
			return flattenShapedRuns(shapeFontRuns(text, runsFace.FontRuns(text), shaper))
		}
	}

	return annotateShapedGlyphs(shaper.Shape(text, face), face)
}

// ShapeRuns shapes a fallback face while retaining source-face identity for
// every run. It is the source-aware counterpart to Shape and is useful to
// renderers that need to rasterize glyph IDs from more than one font.
func ShapeRuns(text string, face Face) []ShapedRun {
	if text == "" || face == nil {
		return nil
	}
	if face.Source() == nil {
		if runsFace, ok := face.(interface{ FontRuns(string) []FontRun }); ok {
			return shapeFontRuns(text, runsFace.FontRuns(text), GetShaper())
		}
	}
	glyphs := annotateShapedGlyphs(GetShaper().Shape(text, face), face)
	if len(glyphs) == 0 {
		return nil
	}
	return []ShapedRun{newShapedRun(face, glyphs, face.Direction())}
}

// shapeMultiFaceRuns shapes each contiguous source run with the active
// shaper. Keeping shaping inside each run preserves GSUB/GPOS output and
// avoids interpreting a glyph ID from one font in another font's namespace.
func shapeMultiFaceRuns(input string, multi *MultiFace, shaper Shaper) []ShapedRun {
	if multi == nil {
		return nil
	}
	return shapeFontRuns(input, multi.FontRuns(input), shaper)
}

// shapeFontRuns shapes source-aware fallback runs produced by a composite
// face. Run.Start is a byte offset, while ShapedGlyph.Cluster is a rune
// index, so convert each run's start explicitly before applying the offset.
func shapeFontRuns(input string, runs []FontRun, shaper Shaper) []ShapedRun {
	if shaper == nil {
		return nil
	}
	if len(runs) == 0 {
		return nil
	}

	shaped := make([]ShapedRun, 0, len(runs))
	var xOffset float64
	for _, run := range runs {
		runeOffset := utf8.RuneCountInString(input[:run.Start])
		rawGlyphs := shaper.Shape(run.Text, run.Face)
		if len(rawGlyphs) == 0 {
			xOffset += run.Face.Advance(run.Text)
			continue
		}
		// Position and cluster offsets below are run-specific. Copy even when
		// the custom shaper supplied Face values because Shaper implementations
		// may legally reuse a result buffer across calls.
		glyphs := make([]ShapedGlyph, len(rawGlyphs))
		copy(glyphs, rawGlyphs)
		glyphs = annotateShapedGlyphs(glyphs, run.Face)

		r := newShapedRun(run.Face, glyphs, run.Face.Direction())
		for i := range glyphs {
			glyphs[i].X += xOffset
			glyphs[i].Cluster += runeOffset
		}
		r.Glyphs = glyphs
		shaped = append(shaped, r)
		xOffset += r.Advance
	}
	return shaped
}

func annotateShapedGlyphs(glyphs []ShapedGlyph, face Face) []ShapedGlyph {
	// Shapers are allowed to reuse their result buffer. Never annotate that
	// buffer in place: doing so leaks the first caller's source face into later
	// calls (and can make a fallback glyph be rasterized through the wrong font).
	// Keep the common case allocation-free when the shaper already supplied
	// source identity for every glyph.
	needsCopy := false
	for i := range glyphs {
		if glyphs[i].Face == nil {
			needsCopy = true
			break
		}
	}
	if needsCopy {
		annotated := make([]ShapedGlyph, len(glyphs))
		copy(annotated, glyphs)
		glyphs = annotated
	}
	for i := range glyphs {
		if glyphs[i].Face == nil {
			glyphs[i].Face = face
		}
	}
	return glyphs
}

func flattenShapedRuns(runs []ShapedRun) []ShapedGlyph {
	if len(runs) == 0 {
		return nil
	}
	var glyphs []ShapedGlyph
	for i := range runs {
		glyphs = append(glyphs, runs[i].Glyphs...)
	}
	return glyphs
}

func newShapedRun(face Face, glyphs []ShapedGlyph, direction Direction) ShapedRun {
	advance := 0.0
	if len(glyphs) > 0 {
		last := glyphs[len(glyphs)-1]
		advance = last.X + last.XAdvance
	}
	metrics := face.Metrics()
	return ShapedRun{
		Glyphs:    glyphs,
		Advance:   advance,
		Ascent:    metrics.Ascent,
		Descent:   metrics.Descent,
		Direction: direction,
		Face:      face,
		Size:      face.Size(),
	}
}
