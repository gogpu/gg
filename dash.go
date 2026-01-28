package gg

import "math"

// Dash defines a dash pattern for stroking.
// A dash pattern consists of alternating dash and gap lengths.
// For example, [5, 3] creates a pattern of 5 units dash, 3 units gap.
type Dash struct {
	// Array contains alternating dash/gap lengths.
	// If the array has an odd number of elements, it is logically duplicated
	// to create an even-length pattern (e.g., [5] becomes [5, 5]).
	Array []float64

	// Offset is the starting offset into the pattern.
	// The stroke begins at this point in the pattern cycle.
	Offset float64
}

// NewDash creates a dash pattern from alternating dash/gap lengths.
// If an odd number of elements is provided, the pattern is conceptually
// duplicated to create an even-length pattern.
//
// Examples:
//
//	NewDash(5, 3)       // 5 units dash, 3 units gap
//	NewDash(10, 5, 2, 5) // 10 dash, 5 gap, 2 dash, 5 gap
//	NewDash(5)          // equivalent to [5, 5]
//
// Returns nil if no lengths are provided or all lengths are zero.
func NewDash(lengths ...float64) *Dash {
	if len(lengths) == 0 {
		return nil
	}

	// Check if all values are zero or negative
	allZeroOrNeg := true
	for _, l := range lengths {
		if l > 0 {
			allZeroOrNeg = false
			break
		}
	}
	if allZeroOrNeg {
		return nil
	}

	// Take absolute values for any negative lengths
	normalized := make([]float64, len(lengths))
	for i, l := range lengths {
		normalized[i] = math.Abs(l)
	}

	return &Dash{
		Array:  normalized,
		Offset: 0,
	}
}

// WithOffset returns a new Dash with the given offset.
// The offset determines where in the pattern the stroke begins.
func (d *Dash) WithOffset(offset float64) *Dash {
	if d == nil {
		return nil
	}
	return &Dash{
		Array:  d.Array,
		Offset: offset,
	}
}

// PatternLength returns the total length of one complete pattern cycle.
// For odd-length arrays, this includes the duplicated pattern.
func (d *Dash) PatternLength() float64 {
	if d == nil || len(d.Array) == 0 {
		return 0
	}

	var total float64
	for _, l := range d.Array {
		total += l
	}

	// If odd number of elements, pattern is duplicated
	if len(d.Array)%2 != 0 {
		total *= 2
	}

	return total
}

// IsDashed returns true if this represents a dashed line (not solid).
// Returns false for nil Dash or empty/all-zero arrays.
func (d *Dash) IsDashed() bool {
	if d == nil || len(d.Array) == 0 {
		return false
	}

	// Check if any dash has positive length
	for _, l := range d.Array {
		if l > 0 {
			return true
		}
	}
	return false
}

// Clone creates a deep copy of the Dash.
func (d *Dash) Clone() *Dash {
	if d == nil {
		return nil
	}

	arrayCopy := make([]float64, len(d.Array))
	copy(arrayCopy, d.Array)

	return &Dash{
		Array:  arrayCopy,
		Offset: d.Offset,
	}
}

// NormalizedOffset returns the offset normalized to be within one pattern cycle.
// This is useful for calculating where in the pattern a stroke should begin.
func (d *Dash) NormalizedOffset() float64 {
	if d == nil {
		return 0
	}

	patternLen := d.PatternLength()
	if patternLen <= 0 {
		return 0
	}

	offset := math.Mod(d.Offset, patternLen)
	if offset < 0 {
		offset += patternLen
	}
	return offset
}

// Scale returns a new Dash with all lengths multiplied by the given factor.
// This is used to scale dash patterns when a transform is applied to the path.
// Per Cairo/Skia convention, dash lengths are in user-space units, so they
// must be scaled along with the coordinate transform.
func (d *Dash) Scale(factor float64) *Dash {
	if d == nil || factor <= 0 {
		return d
	}

	scaledArray := make([]float64, len(d.Array))
	for i, l := range d.Array {
		scaledArray[i] = l * factor
	}

	return &Dash{
		Array:  scaledArray,
		Offset: d.Offset * factor,
	}
}

// effectiveArray returns the array with odd-length arrays duplicated.
// This is used internally for pattern iteration.
func (d *Dash) effectiveArray() []float64 {
	if d == nil || len(d.Array) == 0 {
		return nil
	}

	if len(d.Array)%2 == 0 {
		return d.Array
	}

	// Duplicate for odd-length arrays
	result := make([]float64, len(d.Array)*2)
	copy(result, d.Array)
	copy(result[len(d.Array):], d.Array)
	return result
}
