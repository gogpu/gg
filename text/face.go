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

// advanceResolver is the single source of truth for glyph advances.
//
// All Face methods (Advance, Glyphs, AppendGlyphs) use this to ensure
// measurement matches rendering. Priority: TT hinted > HVAR > raw hmtx.
// This matches drawGlyphs' hintedOrRawAdvance() and Skia's SkFont pattern
// where a single hinting flag controls all metric sources (#479).
type advanceResolver struct {
	parsed     ParsedFont
	varProv    VariableAdvanceProvider
	ttCache    *ttHintCache
	variations []FontVariation
	size       float64
}

func newAdvanceResolver(f *sourceFace) advanceResolver {
	parsed := f.source.Parsed()

	var varProv VariableAdvanceProvider
	if len(f.config.variations) > 0 {
		varProv, _ = parsed.(VariableAdvanceProvider)
	}

	var ttCache *ttHintCache
	if f.config.hinting != HintingNone {
		if ownFont, ok := parsed.(*ownParsedFont); ok {
			ttCache = ownFont.loadTTHintCache()
		}
	}

	return advanceResolver{
		parsed:     parsed,
		varProv:    varProv,
		ttCache:    ttCache,
		variations: f.config.variations,
		size:       f.size,
	}
}

// advance returns the advance for gid from the single source of truth.
// advance returns the advance for gid from the single source of truth.
//
// Priority matches FreeType ttgload.c:964-977:
//   - Variable fonts: HVAR > raw (TT hint cache has no gvar deltas)
//   - Static fonts: TT hinted > raw
func (ar *advanceResolver) advance(gid uint16) float64 {
	if ar.varProv != nil {
		// Variable font: HVAR takes priority. TT hint cache uses default-weight
		// phantom points (no gvar deltas) — wrong for non-default instances.
		return ar.varProv.GlyphAdvanceVar(gid, ar.size, ar.variations)
	}
	if ar.ttCache != nil {
		// Static font: TT-hinted advance matches drawGlyphs positioning.
		if adv, ok := ar.ttCache.hintedAdvanceWidth(gid, int32(ar.size)); ok {
			return adv
		}
	}
	return ar.parsed.GlyphAdvance(gid, ar.size)
}

// Advance implements Face.Advance.
//
// Uses the single source of truth for advances: TT hinted when bytecode
// hinting is active, HVAR when variable, raw hmtx otherwise.
// Matches drawGlyphs positioning to prevent cursor drift (#479).
func (f *sourceFace) Advance(text string) float64 {
	ar := newAdvanceResolver(f)
	total := 0.0
	for _, r := range text {
		if r < 0x20 && r != '\t' {
			continue
		}
		if r == '\t' {
			_, adv := tabAdvance(ar.parsed, f.size)
			total += adv
		} else {
			total += ar.advance(ar.parsed.GlyphIndex(r))
		}
	}
	return total
}

// HasGlyph implements Face.HasGlyph.
func (f *sourceFace) HasGlyph(r rune) bool {
	parsed := f.source.Parsed()
	gid := parsed.GlyphIndex(r)
	return gid != 0
}

// Glyphs implements Face.Glyphs.
//
// Advances come from the single source of truth (advanceResolver):
// TT hinted when hinting active, HVAR when variable, raw otherwise.
// Matches Advance() and drawGlyphs positioning (#479, Skia pattern).
func (f *sourceFace) Glyphs(text string) iter.Seq[Glyph] {
	return func(yield func(Glyph) bool) {
		ar := newAdvanceResolver(f)
		x := 0.0
		byteIndex := 0

		for i, r := range text {
			if r < 0x20 && r != '\t' {
				byteIndex += utf8.RuneLen(r)
				continue
			}

			var gid uint16
			var adv float64
			var bounds Rect

			if r == '\t' {
				gid, adv = tabAdvance(ar.parsed, f.size)
			} else {
				gid = ar.parsed.GlyphIndex(r)
				adv = ar.advance(gid)
				bounds = ar.parsed.GlyphBounds(gid, f.size)
			}

			glyph := Glyph{
				Rune:    r,
				GID:     GlyphID(gid),
				X:       x,
				Y:       0,
				OriginX: x,
				OriginY: 0,
				Advance: adv,
				Bounds:  bounds,
				Index:   byteIndex,
				Cluster: i,
			}

			if !yield(glyph) {
				return
			}

			x += adv
			byteIndex += utf8.RuneLen(r)
		}
	}
}

// AppendGlyphs implements Face.AppendGlyphs.
func (f *sourceFace) AppendGlyphs(dst []Glyph, text string) []Glyph {
	ar := newAdvanceResolver(f)
	x := 0.0
	byteIndex := 0

	for i, r := range text {
		if r < 0x20 && r != '\t' {
			byteIndex += utf8.RuneLen(r)
			continue
		}

		var gid uint16
		var adv float64
		var bounds Rect

		if r == '\t' {
			gid, adv = tabAdvance(ar.parsed, f.size)
		} else {
			gid = ar.parsed.GlyphIndex(r)
			adv = ar.advance(gid)
			bounds = ar.parsed.GlyphBounds(gid, f.size)
		}

		glyph := Glyph{
			Rune:    r,
			GID:     GlyphID(gid),
			X:       x,
			Y:       0,
			OriginX: x,
			OriginY: 0,
			Advance: adv,
			Bounds:  bounds,
			Index:   byteIndex,
			Cluster: i,
		}

		dst = append(dst, glyph)
		x += adv
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
