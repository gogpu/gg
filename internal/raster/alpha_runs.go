// Package raster provides scanline rasterization for 2D paths.
// This file implements AlphaRuns for RLE-encoded alpha (coverage) values.
// Based on tiny-skia's alpha_runs.rs (Android/Skia heritage).
package raster

// AlphaRuns stores run-length-encoded alpha (supersampling coverage) values.
// Sparseness allows independent composition of several paths into the same buffer.
type AlphaRuns struct {
	// runs stores the length of each run. A zero value indicates end of runs.
	// Value represents the number of pixels with the same alpha.
	runs []uint16
	// alpha stores the alpha value for each run.
	alpha []uint8
}

// NewAlphaRuns creates a new AlphaRuns buffer for the given width.
func NewAlphaRuns(width int) *AlphaRuns {
	if width <= 0 {
		width = 1
	}
	ar := &AlphaRuns{
		runs:  make([]uint16, width+1),
		alpha: make([]uint8, width+1),
	}
	ar.Reset(width)
	return ar
}

// CatchOverflow converts 0-256 to 0-255 safely.
// Input value 256 maps to 255 (handles overflow from accumulation).
func CatchOverflow(alpha uint16) uint8 {
	if alpha > 256 {
		alpha = 256
	}
	// (alpha - (alpha >> 8)) maps 256 -> 255
	result := alpha - (alpha >> 8)
	return uint8(result) //nolint:gosec // bounded by 255 after overflow correction
}

// IsEmpty returns true if the scanline contains only a single run of alpha 0.
func (ar *AlphaRuns) IsEmpty() bool {
	if ar.runs[0] == 0 {
		return true
	}
	// Check if single run with alpha 0 and next is terminator
	return ar.alpha[0] == 0 && ar.runs[ar.runs[0]] == 0
}

// Reset reinitializes the buffer for a new scanline.
func (ar *AlphaRuns) Reset(width int) {
	if width <= 0 {
		width = 1
	}
	if width > 65535 {
		width = 65535
	}
	ar.runs[0] = uint16(width) //nolint:gosec // bounded to 65535 above
	ar.runs[width] = 0         // terminator
	ar.alpha[0] = 0
}

// Add inserts a run into the buffer.
// Parameters:
//   - x: starting x coordinate
//   - startAlpha: alpha for first pixel (if non-zero)
//   - middleCount: number of full-coverage pixels
//   - stopAlpha: alpha for last pixel (if non-zero)
//   - maxValue: maximum alpha value for middle pixels
//   - offsetX: hint for where to start searching in runs array
//
// Returns the new offsetX value for the next call on the same scanline.
func (ar *AlphaRuns) Add(x int, startAlpha uint8, middleCount int, stopAlpha uint8, maxValue uint8, offsetX int) int {
	if x < 0 {
		return offsetX
	}

	runsOffset := offsetX
	alphaOffset := offsetX
	lastAlphaOffset := offsetX
	x -= offsetX

	if startAlpha != 0 {
		ar.breakRun(runsOffset, x, 1)

		// Handle potential overflow when adding alpha
		tmp := uint16(ar.alpha[alphaOffset+x]) + uint16(startAlpha)
		ar.alpha[alphaOffset+x] = CatchOverflow(tmp)

		runsOffset += x + 1
		alphaOffset += x + 1
		x = 0
	}

	if middleCount > 0 {
		ar.breakRun(runsOffset, x, middleCount)
		alphaOffset += x
		runsOffset += x
		x = 0

		for middleCount > 0 {
			a := CatchOverflow(uint16(ar.alpha[alphaOffset]) + uint16(maxValue))
			ar.alpha[alphaOffset] = a

			n := int(ar.runs[runsOffset])
			if n <= 0 {
				break
			}
			if n > middleCount {
				n = middleCount
			}
			alphaOffset += n
			runsOffset += n
			middleCount -= n
		}

		lastAlphaOffset = alphaOffset
	}

	if stopAlpha != 0 {
		ar.breakRun(runsOffset, x, 1)
		alphaOffset += x
		ar.alpha[alphaOffset] += stopAlpha
		lastAlphaOffset = alphaOffset
	}

	return lastAlphaOffset
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

// Runs returns the runs slice.
func (ar *AlphaRuns) Runs() []uint16 {
	return ar.runs
}

// Alpha returns the alpha slice.
func (ar *AlphaRuns) Alpha() []uint8 {
	return ar.alpha
}
