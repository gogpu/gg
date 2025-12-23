package text

// unknownStr is the string returned for unknown enum values.
const unknownStr = "Unknown"

// Direction specifies text direction.
type Direction int

const (
	// DirectionLTR is left-to-right text (English, French, etc.)
	DirectionLTR Direction = iota
	// DirectionRTL is right-to-left text (Arabic, Hebrew)
	DirectionRTL
	// DirectionTTB is top-to-bottom text (traditional Chinese, Japanese)
	DirectionTTB
	// DirectionBTT is bottom-to-top text (rare)
	DirectionBTT
)

// String returns the string representation of the direction.
func (d Direction) String() string {
	switch d {
	case DirectionLTR:
		return "LTR"
	case DirectionRTL:
		return "RTL"
	case DirectionTTB:
		return "TTB"
	case DirectionBTT:
		return "BTT"
	default:
		return unknownStr
	}
}

// IsHorizontal returns true if the direction is horizontal (LTR or RTL).
func (d Direction) IsHorizontal() bool {
	return d == DirectionLTR || d == DirectionRTL
}

// IsVertical returns true if the direction is vertical (TTB or BTT).
func (d Direction) IsVertical() bool {
	return d == DirectionTTB || d == DirectionBTT
}

// Hinting specifies font hinting mode.
type Hinting int

const (
	// HintingNone disables hinting.
	HintingNone Hinting = iota
	// HintingVertical applies vertical hinting only.
	HintingVertical
	// HintingFull applies full hinting.
	HintingFull
)

// String returns the string representation of the hinting.
func (h Hinting) String() string {
	switch h {
	case HintingNone:
		return "None"
	case HintingVertical:
		return "Vertical"
	case HintingFull:
		return "Full"
	default:
		return unknownStr
	}
}

// Rect represents a rectangle for glyph bounds.
type Rect struct {
	// Min is the top-left corner
	MinX, MinY float64
	// Max is the bottom-right corner
	MaxX, MaxY float64
}

// Width returns the width of the rectangle.
func (r Rect) Width() float64 {
	return r.MaxX - r.MinX
}

// Height returns the height of the rectangle.
func (r Rect) Height() float64 {
	return r.MaxY - r.MinY
}

// Empty reports whether the rectangle is empty.
func (r Rect) Empty() bool {
	return r.MinX >= r.MaxX || r.MinY >= r.MaxY
}
