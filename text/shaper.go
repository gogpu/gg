package text

import "sync"

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
	if multi, ok := face.(*MultiFace); ok {
		return flattenShapedRuns(shapeMultiFaceRuns(text, multi, shaper))
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
	if multi, ok := face.(*MultiFace); ok {
		return shapeMultiFaceRuns(text, multi, GetShaper())
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
	if multi == nil || shaper == nil {
		return nil
	}
	runs := multi.FontRuns(input)
	if len(runs) == 0 {
		return nil
	}

	shaped := make([]ShapedRun, 0, len(runs))
	var xOffset float64
	var runeOffset int
	for _, run := range runs {
		glyphs := annotateShapedGlyphs(shaper.Shape(run.Text, run.Face), run.Face)
		if len(glyphs) == 0 {
			runeOffset += runeCount(run.Text)
			xOffset += run.Face.Advance(run.Text)
			continue
		}

		r := newShapedRun(run.Face, glyphs, run.Face.Direction())
		for i := range glyphs {
			glyphs[i].X += xOffset
			glyphs[i].Cluster += runeOffset
		}
		r.Glyphs = glyphs
		shaped = append(shaped, r)
		xOffset += r.Advance
		runeOffset += runeCount(run.Text)
	}
	return shaped
}

func annotateShapedGlyphs(glyphs []ShapedGlyph, face Face) []ShapedGlyph {
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

func runeCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
