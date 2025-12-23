package text

// ShapedRun is a sequence of shaped glyphs with uniform style.
// Used by the Layout Engine for multi-line and multi-style text rendering.
type ShapedRun struct {
	// Glyphs is the sequence of positioned glyphs.
	Glyphs []ShapedGlyph

	// Advance is the total advance of all glyphs.
	// For horizontal text, this is width; for vertical, height.
	Advance float64

	// Ascent is the maximum ascent above the baseline.
	Ascent float64

	// Descent is the maximum descent below the baseline (positive value).
	Descent float64

	// Direction is the text direction for this run.
	Direction Direction

	// Face is the font face used for this run.
	Face Face

	// Size is the font size in pixels.
	Size float64
}

// Width returns the total width of the run.
// For horizontal text, this equals Advance.
// For vertical text, this is based on glyph widths.
func (r *ShapedRun) Width() float64 {
	if r.Direction.IsHorizontal() {
		return r.Advance
	}
	// For vertical text, width is based on max glyph width
	// This is a simplified implementation
	return r.Ascent + r.Descent
}

// Height returns the total height of the run.
// For horizontal text, this is Ascent + Descent.
// For vertical text, this equals Advance.
func (r *ShapedRun) Height() float64 {
	if r.Direction.IsVertical() {
		return r.Advance
	}
	return r.Ascent + r.Descent
}

// LineHeight returns the recommended line height for this run.
func (r *ShapedRun) LineHeight() float64 {
	return r.Ascent + r.Descent
}

// Bounds returns the bounding rectangle of the run.
// The origin is at the baseline start.
func (r *ShapedRun) Bounds() (x, y, width, height float64) {
	if r.Direction.IsHorizontal() {
		return 0, -r.Ascent, r.Advance, r.Ascent + r.Descent
	}
	return -r.Ascent, 0, r.Ascent + r.Descent, r.Advance
}
