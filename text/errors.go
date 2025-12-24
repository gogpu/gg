package text

import "errors"

// Sentinel errors for text package.
var (
	// ErrEmptyFontData is returned when font data is empty.
	ErrEmptyFontData = errors.New("text: empty font data")

	// ErrEmptyFaces is returned when no faces are provided to MultiFace.
	ErrEmptyFaces = errors.New("text: faces cannot be empty")
)

// DirectionMismatchError is returned when faces have different directions.
type DirectionMismatchError struct {
	Index    int
	Got      Direction
	Expected Direction
}

func (e *DirectionMismatchError) Error() string {
	return "text: face direction mismatch"
}
