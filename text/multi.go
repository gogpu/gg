package text

import (
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

// FontRun is a contiguous source-font run in a fallback face.
//
// Start and End are byte offsets into the input passed to FontRuns. Offset is
// the advance of all preceding runs and can be used as the run's origin. The
// IsCJK flag lets GPU consumers keep script-specific atlas and hinting policy
// while still emitting one ordered batch for the complete string.
type FontRun struct {
	Face   Face
	Text   string
	Start  int
	End    int
	Offset float64
	IsCJK  bool
}

// NewMultiFace creates a MultiFace from faces.
// All faces must have the same direction.
// Returns error if faces is empty or directions don't match.
func NewMultiFace(faces ...Face) (*MultiFace, error) {
	if len(faces) == 0 {
		return nil, ErrEmptyFaces
	}

	// Check that all faces have the same direction
	direction := faces[0].Direction()
	for i, face := range faces[1:] {
		if face.Direction() != direction {
			return nil, &DirectionMismatchError{
				Index:    i + 1,
				Got:      face.Direction(),
				Expected: direction,
			}
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

// Faces returns the fallback chain in priority order.
//
// The returned slice is a copy and can be modified by the caller without
// changing this MultiFace.
func (m *MultiFace) Faces() []Face {
	faces := make([]Face, len(m.faces))
	copy(faces, m.faces)
	return faces
}

// FaceForRune returns the first source face that contains r. Composite faces
// are resolved recursively so callers always receive a face that can expose a
// FontSource when one exists. If no face contains r, the first face is used as
// the replacement-glyph fallback, matching Glyphs and AppendGlyphs.
func (m *MultiFace) FaceForRune(r rune) Face {
	for _, face := range m.faces {
		if face != nil && face.HasGlyph(r) {
			return resolveFallbackFace(face, r)
		}
	}
	if len(m.faces) == 0 {
		return nil
	}
	return resolveFallbackFace(m.faces[0], r)
}

// FontRuns splits text into contiguous runs that share a source face and
// script class. Runs retain their original byte ranges and cumulative x
// offsets, allowing GPU consumers to rasterize each source independently and
// append quads without dropping shaped positions or leaving the GPU path.
func (m *MultiFace) FontRuns(text string) []FontRun {
	if text == "" || len(m.faces) == 0 {
		return nil
	}

	var runs []FontRun
	start := 0
	var current Face
	var currentCJK bool
	var offset float64

	flush := func(end int) {
		if current == nil || start >= end {
			return
		}
		runText := text[start:end]
		runs = append(runs, FontRun{
			Face:   current,
			Text:   runText,
			Start:  start,
			End:    end,
			Offset: offset,
			IsCJK:  currentCJK,
		})
		offset += current.Advance(runText)
	}

	for byteIndex, r := range text {
		face := m.FaceForRune(r)
		isCJK := IsCJKRune(r)
		if current == nil {
			current = face
			currentCJK = isCJK
			start = byteIndex
			continue
		}
		if face != current || isCJK != currentCJK {
			flush(byteIndex)
			start = byteIndex
			current = face
			currentCJK = isCJK
		}
	}
	flush(len(text))
	return runs
}

// ShapeRuns is the method form of [ShapeRuns] for callers that already hold a
// MultiFace. Every returned ShapedRun carries the source Face used to produce
// its glyph IDs.
func (m *MultiFace) ShapeRuns(text string) []ShapedRun {
	if m == nil || text == "" {
		return nil
	}
	return shapeMultiFaceRuns(text, m, GetShaper())
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

// Features implements Face.Features.
// Returns features from the first face.
func (m *MultiFace) Features() []FontFeature {
	return m.faces[0].Features()
}

// Language implements Face.Language.
// Returns the language from the first face.
func (m *MultiFace) Language() string {
	return m.faces[0].Language()
}

// Variations implements Face.Variations.
// Returns variations from the first face.
func (m *MultiFace) Variations() []FontVariation {
	return m.faces[0].Variations()
}

// private implements the Face interface.
func (m *MultiFace) private() {}

// faceForRune returns the first face that has the glyph for the rune.
// If no face has the glyph, returns the first face as fallback.
func (m *MultiFace) faceForRune(r rune) Face {
	return m.FaceForRune(r)
}

// resolveFallbackFace unwraps nested composite faces while retaining any
// filtering decision that selected the face in the first place.
func resolveFallbackFace(face Face, r rune) Face {
	switch f := face.(type) {
	case *MultiFace:
		return f.FaceForRune(r)
	case *FilteredFace:
		if nested, ok := f.face.(*MultiFace); ok && nested.HasGlyph(r) {
			return resolveFallbackFace(nested, r)
		}
		// Keep the filter wrapper: GPU consumers can use its FontSource while
		// preserving the caller's range policy for glyph iteration.
		return f
	}
	return face
}
