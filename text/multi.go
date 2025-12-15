package text

import (
	"fmt"
	"iter"
	"unicode/utf8"
)

// MultiFace combines multiple faces with fallback.
// When rendering, it uses the first face that has the glyph.
// MultiFace is safe for concurrent use.
type MultiFace struct {
	faces     []Face
	direction Direction
}

// NewMultiFace creates a MultiFace from faces.
// All faces must have the same direction.
// Returns error if faces is empty or directions don't match.
func NewMultiFace(faces ...Face) (*MultiFace, error) {
	if len(faces) == 0 {
		return nil, fmt.Errorf("NewMultiFace: faces cannot be empty")
	}

	// Check that all faces have the same direction
	direction := faces[0].Direction()
	for i, face := range faces[1:] {
		if face.Direction() != direction {
			return nil, fmt.Errorf("NewMultiFace: face %d has direction %v, expected %v",
				i+1, face.Direction(), direction)
		}
	}

	return &MultiFace{
		faces:     faces,
		direction: direction,
	}, nil
}

// Metrics implements Face.Metrics.
// Returns metrics from the first face.
func (m *MultiFace) Metrics() Metrics {
	return m.faces[0].Metrics()
}

// Advance implements Face.Advance.
// Calculates total advance using the appropriate face for each rune.
func (m *MultiFace) Advance(text string) float64 {
	totalAdvance := 0.0

	for _, r := range text {
		face := m.faceForRune(r)
		// Get glyph advance from the selected face
		// We can't call Advance on the face with the full text,
		// so we need to calculate per-rune
		glyphAdvance := 0.0
		for glyph := range face.Glyphs(string(r)) {
			glyphAdvance = glyph.Advance
			break // Only one glyph for a single rune
		}
		totalAdvance += glyphAdvance
	}

	return totalAdvance
}

// HasGlyph implements Face.HasGlyph.
// Returns true if any face has the glyph.
func (m *MultiFace) HasGlyph(r rune) bool {
	for _, face := range m.faces {
		if face.HasGlyph(r) {
			return true
		}
	}
	return false
}

// Glyphs implements Face.Glyphs.
// Returns an iterator over all glyphs, using the appropriate face for each rune.
func (m *MultiFace) Glyphs(text string) iter.Seq[Glyph] {
	return func(yield func(Glyph) bool) {
		x := 0.0
		byteIndex := 0

		for i, r := range text {
			face := m.faceForRune(r)

			// Get the glyph from the selected face
			for glyph := range face.Glyphs(string(r)) {
				// Update position and index to match the full text
				glyph.X = x
				glyph.OriginX = x
				glyph.Index = byteIndex
				glyph.Cluster = i

				if !yield(glyph) {
					return
				}

				x += glyph.Advance
			}

			byteIndex += utf8.RuneLen(r)
		}
	}
}

// AppendGlyphs implements Face.AppendGlyphs.
// Appends glyphs using the appropriate face for each rune.
func (m *MultiFace) AppendGlyphs(dst []Glyph, text string) []Glyph {
	x := 0.0
	byteIndex := 0

	for i, r := range text {
		face := m.faceForRune(r)

		// Get the glyph from the selected face
		for glyph := range face.Glyphs(string(r)) {
			// Update position and index to match the full text
			glyph.X = x
			glyph.OriginX = x
			glyph.Index = byteIndex
			glyph.Cluster = i

			dst = append(dst, glyph)
			x += glyph.Advance
		}

		byteIndex += utf8.RuneLen(r)
	}

	return dst
}

// Direction implements Face.Direction.
func (m *MultiFace) Direction() Direction {
	return m.direction
}

// Source implements Face.Source.
// Returns nil since MultiFace is a composite face.
func (m *MultiFace) Source() *FontSource {
	return nil
}

// Size implements Face.Size.
// Returns the size from the first face.
func (m *MultiFace) Size() float64 {
	return m.faces[0].Size()
}

// private implements the Face interface.
func (m *MultiFace) private() {}

// faceForRune returns the first face that has the glyph for the rune.
// If no face has the glyph, returns the first face as fallback.
func (m *MultiFace) faceForRune(r rune) Face {
	for _, face := range m.faces {
		if face.HasGlyph(r) {
			return face
		}
	}
	// Fallback to first face if no face has the glyph
	return m.faces[0]
}
