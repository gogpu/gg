package text

import (
	"iter"
	"unicode/utf8"
)

// Face represents a font face at a specific size.
// This is a lightweight object that can be created from a FontSource.
// Face is safe for concurrent use.
type Face interface {
	// Metrics returns the font metrics at this face's size.
	Metrics() Metrics

	// Advance returns the total advance width of the text in pixels.
	// This is the sum of all glyph advances.
	Advance(text string) float64

	// HasGlyph reports whether the font has a glyph for the given rune.
	HasGlyph(r rune) bool

	// Glyphs returns an iterator over all glyphs in the text.
	// The glyphs are positioned relative to the origin (0, 0).
	// Uses Go 1.25+ iter.Seq for zero-allocation iteration.
	Glyphs(text string) iter.Seq[Glyph]

	// AppendGlyphs appends glyphs for the text to dst and returns the extended slice.
	// This is useful for building glyph slices without allocation.
	AppendGlyphs(dst []Glyph, text string) []Glyph

	// Direction returns the text direction for this face.
	Direction() Direction

	// Source returns the FontSource this face was created from.
	Source() *FontSource

	// Size returns the size of this face in points.
	Size() float64

	// Features returns the OpenType font features configured for this face.
	// Features are set via [WithFeatures] when creating the face.
	Features() []FontFeature

	// Language returns the BCP 47 language tag for this face (e.g., "en", "ja", "ar").
	// The language affects OpenType shaping: script-specific ligatures, localized
	// forms, and language-dependent glyph selection.
	// Language is set via [WithLanguage] when creating the face; defaults to "en".
	Language() string

	// Variations returns the font variation axis values configured for this face.
	// Variations are set via [WithVariations] when creating the face.
	// Returns nil for faces created without variations.
	Variations() []FontVariation

	// private prevents external implementation
	private()
}

// sourceFace is the internal implementation of Face.
type sourceFace struct {
	source *FontSource
	size   float64
	config faceConfig
}

// Metrics implements Face.Metrics.
func (f *sourceFace) Metrics() Metrics {
	parsed := f.source.Parsed()
	fontMetrics := parsed.Metrics(f.size)

	// FontMetrics.Descent is negative (below baseline)
	// Metrics.Descent is positive (absolute distance from baseline)
	descent := fontMetrics.Descent
	if descent < 0 {
		descent = -descent
	}

	return Metrics{
		Ascent:    fontMetrics.Ascent,
		Descent:   descent,
		LineGap:   fontMetrics.LineGap,
		XHeight:   fontMetrics.XHeight,
		CapHeight: fontMetrics.CapHeight,
	}
}

// Advance implements Face.Advance.
func (f *sourceFace) Advance(text string) float64 {
	parsed := f.source.Parsed()
	totalAdvance := 0.0

	// Check for variable advance provider (HVAR) when variations are set.
	var varProvider VariableAdvanceProvider
	if len(f.config.variations) > 0 {
		varProvider, _ = parsed.(VariableAdvanceProvider)
	}

	for _, r := range text {
		if r < 0x20 && r != '\t' {
			continue
		}
		var advance float64
		if r == '\t' {
			_, advance = tabAdvance(parsed, f.size)
		} else {
			gid := parsed.GlyphIndex(r)
			if varProvider != nil {
				advance = varProvider.GlyphAdvanceVar(gid, f.size, f.config.variations)
			} else {
				advance = parsed.GlyphAdvance(gid, f.size)
			}
		}
		totalAdvance += advance
	}

	return totalAdvance
}

// HasGlyph implements Face.HasGlyph.
func (f *sourceFace) HasGlyph(r rune) bool {
	parsed := f.source.Parsed()
	gid := parsed.GlyphIndex(r)
	return gid != 0
}

// Glyphs implements Face.Glyphs.
func (f *sourceFace) Glyphs(text string) iter.Seq[Glyph] {
	return func(yield func(Glyph) bool) {
		parsed := f.source.Parsed()
		x := 0.0
		byteIndex := 0

		// Check for variable advance provider (HVAR) when variations are set.
		var varProvider VariableAdvanceProvider
		if len(f.config.variations) > 0 {
			varProvider, _ = parsed.(VariableAdvanceProvider)
		}

		for i, r := range text {
			// Skip non-tab control characters.
			if r < 0x20 && r != '\t' {
				byteIndex += utf8.RuneLen(r)
				continue
			}

			var gid uint16
			var advance float64
			var bounds Rect

			if r == '\t' {
				// Tab: use space GID (empty outline) with tab-stop advance.
				gid, advance = tabAdvance(parsed, f.size)
				// Space bounds are empty — no visual rendering.
			} else {
				gid = parsed.GlyphIndex(r)
				if varProvider != nil {
					advance = varProvider.GlyphAdvanceVar(gid, f.size, f.config.variations)
				} else {
					advance = parsed.GlyphAdvance(gid, f.size)
				}
				bounds = parsed.GlyphBounds(gid, f.size)
			}

			glyph := Glyph{
				Rune:    r,
				GID:     GlyphID(gid),
				X:       x,
				Y:       0,
				OriginX: x,
				OriginY: 0,
				Advance: advance,
				Bounds:  bounds,
				Index:   byteIndex,
				Cluster: i,
			}

			if !yield(glyph) {
				return
			}

			x += advance
			byteIndex += utf8.RuneLen(r)
		}
	}
}

// AppendGlyphs implements Face.AppendGlyphs.
func (f *sourceFace) AppendGlyphs(dst []Glyph, text string) []Glyph {
	parsed := f.source.Parsed()
	x := 0.0
	byteIndex := 0

	// Check for variable advance provider (HVAR) when variations are set.
	var varProvider VariableAdvanceProvider
	if len(f.config.variations) > 0 {
		varProvider, _ = parsed.(VariableAdvanceProvider)
	}

	for i, r := range text {
		// Skip non-tab control characters.
		if r < 0x20 && r != '\t' {
			byteIndex += utf8.RuneLen(r)
			continue
		}

		var gid uint16
		var advance float64
		var bounds Rect

		if r == '\t' {
			gid, advance = tabAdvance(parsed, f.size)
		} else {
			gid = parsed.GlyphIndex(r)
			if varProvider != nil {
				advance = varProvider.GlyphAdvanceVar(gid, f.size, f.config.variations)
			} else {
				advance = parsed.GlyphAdvance(gid, f.size)
			}
			bounds = parsed.GlyphBounds(gid, f.size)
		}

		glyph := Glyph{
			Rune:    r,
			GID:     GlyphID(gid),
			X:       x,
			Y:       0,
			OriginX: x,
			OriginY: 0,
			Advance: advance,
			Bounds:  bounds,
			Index:   byteIndex,
			Cluster: i,
		}

		dst = append(dst, glyph)
		x += advance
		byteIndex += utf8.RuneLen(r)
	}

	return dst
}

// Direction implements Face.Direction.
func (f *sourceFace) Direction() Direction {
	return f.config.direction
}

// Source implements Face.Source.
func (f *sourceFace) Source() *FontSource {
	return f.source
}

// Size implements Face.Size.
func (f *sourceFace) Size() float64 {
	return f.size
}

// Features implements Face.Features.
func (f *sourceFace) Features() []FontFeature {
	return f.config.features
}

// Language implements Face.Language.
func (f *sourceFace) Language() string {
	return f.config.language
}

// Variations implements Face.Variations.
func (f *sourceFace) Variations() []FontVariation {
	return f.config.variations
}

// private implements the Face interface.
func (f *sourceFace) private() {}
