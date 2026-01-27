// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"iter"
)

// AlphaRuns provides run-length-encoded (RLE) storage for coverage values.
//
// This is efficient for paths with long horizontal spans of constant coverage.
// Instead of storing per-pixel alpha values, it stores runs of consecutive
// pixels with the same alpha.
//
// The implementation follows tiny-skia's alpha_runs.rs pattern but with
// Go 1.25+ iterators for efficient traversal.
//
// Usage:
//
//	ar := NewAlphaRuns(width)
//	ar.Reset()
//	ar.Add(10, 128, 20, 128) // Add coverage from x=10, 20 pixels wide
//	for x, alpha := range ar.Iter() {
//	    // Process pixel at x with alpha value
//	}
type AlphaRuns struct {
	// runs stores the length of each run. Zero indicates end of runs.
	runs []uint16

	// alpha stores the alpha value for each run position.
	alpha []uint8

	// width is the scanline width.
	width int

	// offset tracks the current position for sequential Add calls.
	offset int
}

// AlphaRun represents a single run of coverage.
// Used for iteration output.
type AlphaRun struct {
	X     int   // Starting X position
	Alpha uint8 // Coverage value (0-255)
	Count int   // Run length
}

// NewAlphaRuns creates a new AlphaRuns buffer for the given width.
func NewAlphaRuns(width int) *AlphaRuns {
	if width <= 0 {
		width = 1
	}
	ar := &AlphaRuns{
		runs:  make([]uint16, width+1),
		alpha: make([]uint8, width+1),
		width: width,
	}
	ar.Reset()
	return ar
}

// Reset reinitializes the buffer for a new scanline.
// This is O(1) - it doesn't clear the entire buffer.
func (ar *AlphaRuns) Reset() {
	ar.offset = 0
	if ar.width > 65535 {
		ar.runs[0] = 65535
	} else {
		ar.runs[0] = uint16(ar.width) //nolint:gosec // bounded above
	}
	ar.runs[ar.width] = 0 // terminator
	ar.alpha[0] = 0
}

// IsEmpty returns true if the scanline contains only a single run of alpha 0.
func (ar *AlphaRuns) IsEmpty() bool {
	if ar.runs[0] == 0 {
		return true
	}
	// Check if single run with alpha 0 and next is terminator
	return ar.alpha[0] == 0 && ar.runs[ar.runs[0]] == 0
}

// Width returns the scanline width.
func (ar *AlphaRuns) Width() int {
	return ar.width
}

// catchOverflow converts 0-256 to 0-255 safely.
// Input value 256 maps to 255 (handles overflow from accumulation).
func catchOverflow(alpha uint16) uint8 {
	if alpha > 256 {
		alpha = 256
	}
	// (alpha - (alpha >> 8)) maps 256 -> 255
	result := alpha - (alpha >> 8)
	return uint8(result) //nolint:gosec // bounded by 255 after overflow correction
}

// Add inserts coverage into the buffer.
//
// Parameters:
//   - x: starting x coordinate
//   - startAlpha: alpha for first pixel (fractional left edge)
//   - middleCount: number of full-coverage pixels
//   - endAlpha: alpha for last pixel (fractional right edge)
//
// The method accumulates coverage - multiple Add calls can contribute
// to the same pixels. This is essential for correct winding rule handling.
func (ar *AlphaRuns) Add(x int, startAlpha uint8, middleCount int, endAlpha uint8) {
	if x < 0 || x >= ar.width {
		return
	}

	// Use max coverage (255) for middle pixels
	ar.addWithMaxValue(x, startAlpha, middleCount, endAlpha, 255)
}

// AddWithCoverage inserts coverage with a specified maximum value.
// This is useful when the coverage itself represents partial opacity.
func (ar *AlphaRuns) AddWithCoverage(x int, startAlpha uint8, middleCount int, endAlpha uint8, maxValue uint8) {
	if x < 0 || x >= ar.width {
		return
	}
	ar.addWithMaxValue(x, startAlpha, middleCount, endAlpha, maxValue)
}

// addWithMaxValue is the internal implementation.
func (ar *AlphaRuns) addWithMaxValue(x int, startAlpha uint8, middleCount int, endAlpha uint8, maxValue uint8) {
	runsOffset := ar.offset
	alphaOffset := ar.offset
	lastAlphaOffset := ar.offset
	x -= ar.offset

	if startAlpha != 0 {
		ar.breakRun(runsOffset, x, 1)

		// Handle potential overflow when adding alpha
		tmp := uint16(ar.alpha[alphaOffset+x]) + uint16(startAlpha)
		ar.alpha[alphaOffset+x] = catchOverflow(tmp)

		runsOffset += x + 1
		alphaOffset += x + 1
		x = 0
	}

	if middleCount > 0 {
		ar.breakRun(runsOffset, x, middleCount)
		alphaOffset += x
		runsOffset += x
		x = 0

		remaining := middleCount
		for remaining > 0 {
			a := catchOverflow(uint16(ar.alpha[alphaOffset]) + uint16(maxValue))
			ar.alpha[alphaOffset] = a

			n := int(ar.runs[runsOffset])
			if n <= 0 {
				break
			}
			if n > remaining {
				n = remaining
			}
			alphaOffset += n
			runsOffset += n
			remaining -= n
		}

		lastAlphaOffset = alphaOffset
	}

	if endAlpha != 0 {
		ar.breakRun(runsOffset, x, 1)
		alphaOffset += x
		ar.alpha[alphaOffset] = catchOverflow(uint16(ar.alpha[alphaOffset]) + uint16(endAlpha))
		lastAlphaOffset = alphaOffset
	}

	ar.offset = lastAlphaOffset
}

// breakRun splits runs at positions x and x+count.
// This allows Add() to modify sub-ranges of existing runs.
func (ar *AlphaRuns) breakRun(runsOffset, x, count int) {
	if count <= 0 {
		return
	}

	origX := x

	// First break: find and split at position x
	ro := runsOffset
	ao := runsOffset
	for x > 0 {
		n := int(ar.runs[ro])
		if n <= 0 {
			return
		}

		if x < n {
			// Split the run at position x
			ar.alpha[ao+x] = ar.alpha[ao]
			ar.runs[ro] = uint16(x)       //nolint:gosec // x < n and n fits in uint16
			ar.runs[ro+x] = uint16(n - x) //nolint:gosec // n-x is positive and bounded
			break
		}
		ro += n
		ao += n
		x -= n
	}

	// Second break: find and split at position x+count
	ro = runsOffset + origX
	ao = runsOffset + origX
	x = count

	for {
		n := int(ar.runs[ro])
		if n <= 0 {
			break
		}

		if x < n {
			// Split the run at position x
			ar.alpha[ao+x] = ar.alpha[ao]
			ar.runs[ro] = uint16(x)       //nolint:gosec // x < n and n fits in uint16
			ar.runs[ro+x] = uint16(n - x) //nolint:gosec // n-x is positive and bounded
			break
		}

		x -= n
		if x == 0 {
			break
		}

		ro += n
		ao += n
	}
}

// Iter returns an iterator over all runs with non-zero alpha.
// Each iteration yields the X position and alpha value.
// This uses Go 1.25+ iter.Seq2 for efficient iteration.
func (ar *AlphaRuns) Iter() iter.Seq2[int, uint8] {
	return func(yield func(int, uint8) bool) {
		x := 0
		for x < ar.width {
			n := int(ar.runs[x])
			if n <= 0 {
				break
			}
			alpha := ar.alpha[x]
			if alpha > 0 {
				// Yield each pixel in the run
				for i := 0; i < n && x+i < ar.width; i++ {
					if !yield(x+i, alpha) {
						return
					}
				}
			}
			x += n
		}
	}
}

// IterRuns returns an iterator over runs (not individual pixels).
// More efficient when processing entire runs at once.
func (ar *AlphaRuns) IterRuns() iter.Seq[AlphaRun] {
	return func(yield func(AlphaRun) bool) {
		x := 0
		for x < ar.width {
			n := int(ar.runs[x])
			if n <= 0 {
				break
			}
			run := AlphaRun{
				X:     x,
				Alpha: ar.alpha[x],
				Count: n,
			}
			if !yield(run) {
				return
			}
			x += n
		}
	}
}

// GetAlpha returns the alpha value at position x.
// Returns 0 if x is out of bounds.
func (ar *AlphaRuns) GetAlpha(x int) uint8 {
	if x < 0 || x >= ar.width {
		return 0
	}

	// Find the run containing x
	pos := 0
	for pos < ar.width {
		n := int(ar.runs[pos])
		if n <= 0 {
			break
		}
		if x < pos+n {
			return ar.alpha[pos]
		}
		pos += n
	}
	return 0
}

// Clear sets all alpha values to zero and resets to a single run.
func (ar *AlphaRuns) Clear() {
	ar.Reset()
}

// CopyTo copies the coverage values to a destination slice.
// The destination must have at least ar.width elements.
func (ar *AlphaRuns) CopyTo(dst []uint8) {
	if len(dst) < ar.width {
		return
	}

	x := 0
	for x < ar.width {
		n := int(ar.runs[x])
		if n <= 0 {
			break
		}
		alpha := ar.alpha[x]
		for i := 0; i < n && x+i < ar.width; i++ {
			dst[x+i] = alpha
		}
		x += n
	}
}

// SetOffset sets the offset for the next Add call.
// Use 0 when starting a new scanline.
func (ar *AlphaRuns) SetOffset(offset int) {
	ar.offset = offset
}
