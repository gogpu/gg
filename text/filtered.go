package text

import "iter"

// UnicodeRange represents a contiguous range of code points.
type UnicodeRange struct {
	Start rune
	End   rune
}

// Contains reports whether the rune is in the range.
func (ur UnicodeRange) Contains(r rune) bool {
	return r >= ur.Start && r <= ur.End
}

// Common Unicode ranges for filtering faces.
var (
	// Latin Scripts
	RangeBasicLatin = UnicodeRange{0x0000, 0x007F} // ASCII
	RangeLatin1Sup  = UnicodeRange{0x0080, 0x00FF} // Latin-1 Supplement
	RangeLatinExtA  = UnicodeRange{0x0100, 0x017F} // Latin Extended-A
	RangeLatinExtB  = UnicodeRange{0x0180, 0x024F} // Latin Extended-B

	// Cyrillic Scripts
	RangeCyrillic = UnicodeRange{0x0400, 0x04FF} // Cyrillic

	// Greek Scripts
	RangeGreek = UnicodeRange{0x0370, 0x03FF} // Greek and Coptic

	// Middle Eastern Scripts
	RangeArabic = UnicodeRange{0x0600, 0x06FF} // Arabic
	RangeHebrew = UnicodeRange{0x0590, 0x05FF} // Hebrew

	// CJK Scripts
	RangeCJKUnified = UnicodeRange{0x4E00, 0x9FFF} // CJK Unified Ideographs
	RangeHiragana   = UnicodeRange{0x3040, 0x309F} // Hiragana
	RangeKatakana   = UnicodeRange{0x30A0, 0x30FF} // Katakana
	RangeHangul     = UnicodeRange{0xAC00, 0xD7AF} // Hangul Syllables

	// Emoji
	RangeEmoji        = UnicodeRange{0x1F600, 0x1F64F} // Emoticons
	RangeEmojiMisc    = UnicodeRange{0x1F300, 0x1F5FF} // Miscellaneous Symbols and Pictographs
	RangeEmojiSymbols = UnicodeRange{0x1F680, 0x1F6FF} // Transport and Map Symbols
	RangeEmojiFlags   = UnicodeRange{0x1F1E0, 0x1F1FF} // Regional Indicator Symbols (Flags)
)

// FilteredFace wraps a face and restricts it to specific Unicode ranges.
// Only glyphs in the specified ranges are considered available.
// FilteredFace is safe for concurrent use.
type FilteredFace struct {
	face   Face
	ranges []UnicodeRange
}

// NewFilteredFace creates a FilteredFace.
// Only glyphs in the specified ranges are considered available.
// If no ranges are specified, all glyphs are available (no filtering).
func NewFilteredFace(face Face, ranges ...UnicodeRange) *FilteredFace {
	return &FilteredFace{
		face:   face,
		ranges: ranges,
	}
}

// Metrics implements Face.Metrics.
func (f *FilteredFace) Metrics() Metrics {
	return f.face.Metrics()
}

// Advance implements Face.Advance.
// Only includes runes that are in the allowed ranges.
func (f *FilteredFace) Advance(text string) float64 {
	totalAdvance := 0.0

	for _, r := range text {
		if f.inRanges(r) {
			// Get glyph advance from the wrapped face
			glyphAdvance := 0.0
			for glyph := range f.face.Glyphs(string(r)) {
				glyphAdvance = glyph.Advance
				break // Only one glyph for a single rune
			}
			totalAdvance += glyphAdvance
		}
	}

	return totalAdvance
}

// HasGlyph implements Face.HasGlyph.
// Returns true only if the rune is in the allowed ranges and the wrapped face has it.
func (f *FilteredFace) HasGlyph(r rune) bool {
	if !f.inRanges(r) {
		return false
	}
	return f.face.HasGlyph(r)
}

// Glyphs implements Face.Glyphs.
// Only yields glyphs for runes in the allowed ranges.
func (f *FilteredFace) Glyphs(text string) iter.Seq[Glyph] {
	return func(yield func(Glyph) bool) {
		for glyph := range f.face.Glyphs(text) {
			if f.inRanges(glyph.Rune) {
				if !yield(glyph) {
					return
				}
			}
		}
	}
}

// AppendGlyphs implements Face.AppendGlyphs.
// Only appends glyphs for runes in the allowed ranges.
func (f *FilteredFace) AppendGlyphs(dst []Glyph, text string) []Glyph {
	for glyph := range f.Glyphs(text) {
		dst = append(dst, glyph)
	}
	return dst
}

// Direction implements Face.Direction.
func (f *FilteredFace) Direction() Direction {
	return f.face.Direction()
}

// Source implements Face.Source.
func (f *FilteredFace) Source() *FontSource {
	return f.face.Source()
}

// Size implements Face.Size.
func (f *FilteredFace) Size() float64 {
	return f.face.Size()
}

// private implements the Face interface.
func (f *FilteredFace) private() {}

// inRanges reports whether the rune is in any of the allowed ranges.
// If no ranges are specified, returns true (no filtering).
func (f *FilteredFace) inRanges(r rune) bool {
	if len(f.ranges) == 0 {
		return true
	}

	for _, ur := range f.ranges {
		if ur.Contains(r) {
			return true
		}
	}
	return false
}
