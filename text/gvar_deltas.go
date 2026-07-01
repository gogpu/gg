// Package text provides GPU text rendering infrastructure.
//
// This file implements the packed point numbers and packed deltas
// decoders used by the gvar table, plus the IUP (Inferred Untouched
// Points) interpolation algorithm for sparse delta sets.
//
// Reference: skrifa (Google fontations)
//   - read-fonts/src/tables/variations.rs (PackedPointNumbers, PackedDeltas)
//   - skrifa/src/outline/glyf/deltas.rs (interpolate_deltas / Jiggler)
//
// Spec: https://learn.microsoft.com/en-us/typography/opentype/spec/otvarcommonformats
package text

// unpackPointNumbers reads packed point number indices from data.
//
// Returns the list of point indices and the number of bytes consumed.
// If count == 0, returns nil (meaning ALL points have deltas, i.e., dense).
//
// Packed format:
//   - First byte: if bit 7 clear, count = byte value. If bit 7 set,
//     count = (first_byte & 0x7F) << 8 | second_byte.
//   - If count == 0: all points (return nil).
//   - Then run-length encoded point number deltas:
//     Control byte: bit 7 = two-byte values, bits 6..0 = run_count - 1.
//     Values are cumulative deltas (each value is added to the last).
//
// Matches skrifa PackedPointNumbers (variations.rs:264-421).
func unpackPointNumbers(data []byte) ([]uint16, int) {
	if len(data) == 0 {
		return nil, 0
	}

	// Read count.
	var count int
	pos := 0
	firstByte := data[pos]
	pos++

	if firstByte == 0 {
		// count == 0 means all points.
		return nil, pos
	}

	if (firstByte & 0x80) != 0 {
		// Two-byte count.
		if pos >= len(data) {
			return nil, pos
		}
		count = int(firstByte&0x7F)<<8 | int(data[pos])
		pos++
		if count == 0 {
			return nil, pos
		}
	} else {
		count = int(firstByte)
	}

	// Read run-length encoded point indices.
	points := make([]uint16, 0, count)
	var lastVal uint16
	seen := 0

	for seen < count && pos < len(data) {
		// Read control byte.
		control := data[pos]
		pos++
		twoBytes := (control & 0x80) != 0
		runCount := int(control&0x7F) + 1

		for range runCount {
			if seen >= count {
				break
			}
			var delta uint16
			if twoBytes {
				if pos+2 > len(data) {
					break
				}
				delta = uint16(data[pos])<<8 | uint16(data[pos+1])
				pos += 2
			} else {
				if pos >= len(data) {
					break
				}
				delta = uint16(data[pos])
				pos++
			}
			lastVal += delta
			points = append(points, lastVal)
			seen++
		}
	}

	return points, pos
}

// unpackDeltas reads packed delta values from data.
//
// Returns the deltas and the number of bytes consumed from data.
//
// Packed format: runs of control byte + values.
// Control byte bits:
//
//	bit 7 (0x80): DELTAS_ARE_ZERO
//	bit 6 (0x40): DELTAS_ARE_WORDS
//	bits 5..0:    run_count - 1
//
// Value types (based on control bits 7,6):
//
//	(0,0) = int8 values
//	(0,1) = int16 values
//	(1,0) = zeros (no data bytes)
//	(1,1) = int32 values (VARC extension, rare)
//
// Matches skrifa PackedDeltas / DeltaRunIter (variations.rs:425-737).
func unpackDeltas(data []byte, count int) ([]int32, int) {
	deltas := make([]int32, count)
	pos := 0
	idx := 0

	for idx < count && pos < len(data) {
		control := data[pos]
		pos++

		runCount := int(control&0x3F) + 1
		areZero := (control & 0x80) != 0
		areWords := (control & 0x40) != 0

		for range runCount {
			if idx >= count {
				break
			}

			switch {
			case areZero && !areWords:
				// Zeros: no data.
				deltas[idx] = 0
			case !areZero && !areWords:
				// int8 values.
				if pos >= len(data) {
					return deltas, pos
				}
				deltas[idx] = int32(int8(data[pos]))
				pos++
			case !areZero && areWords:
				// int16 values.
				if pos+2 > len(data) {
					return deltas, pos
				}
				deltas[idx] = int32(int16(data[pos])<<8 | int16(data[pos+1]))
				pos += 2
			default:
				// int32 values (areZero && areWords).
				if pos+4 > len(data) {
					return deltas, pos
				}
				deltas[idx] = int32(data[pos])<<24 | int32(data[pos+1])<<16 |
					int32(data[pos+2])<<8 | int32(data[pos+3])
				pos += 4
			}
			idx++
		}
	}

	return deltas, pos
}

// gvarIUPInterpolate performs IUP (Inferred Untouched Points) interpolation
// to fill in missing deltas for points that were not explicitly included
// in a sparse tuple variation.
//
// Parameters:
//   - sparseDeltas: the scaled delta values for explicitly listed points
//   - pointIndices: which points have explicit deltas
//   - totalPoints: total number of points including phantoms
//   - contourEnds: end-point index per contour
//   - outlinePoints: original unscaled outline points as [x, y]
//   - axis: 0 for X, 1 for Y
//
// Returns a slice of length totalPoints with interpolated deltas.
//
// The algorithm processes each contour independently:
//  1. Find the first point in the contour that has an explicit delta.
//  2. Walk forward to find consecutive reference points.
//  3. For gaps between reference points, interpolate linearly using
//     the coordinate positions of the two bounding reference points.
//  4. For points outside both reference points, use the nearest
//     reference delta (clamping, not extrapolating).
//  5. If only one reference point exists in the contour, shift all
//     points by that single delta.
//
// Matches skrifa deltas.rs:interpolate_deltas (Jiggler pattern).
func gvarIUPInterpolate(
	sparseDeltas []int32,
	pointIndices []uint16,
	totalPoints int,
	contourEnds []uint16,
	outlinePoints [][2]int32,
	axis int,
) []int32 {
	result := make([]int32, totalPoints)

	// Build a lookup: which points have explicit deltas?
	hasDelta := make([]bool, totalPoints)
	for i, idx := range pointIndices {
		if int(idx) < totalPoints && i < len(sparseDeltas) {
			result[idx] = sparseDeltas[i]
			hasDelta[idx] = true
		}
	}

	// Process each contour.
	contourStart := 0
	for _, endIdx := range contourEnds {
		end := int(endIdx)
		if end >= totalPoints {
			break
		}

		// Find first point with a delta in this contour.
		firstDelta := -1
		for p := contourStart; p <= end; p++ {
			if hasDelta[p] {
				firstDelta = p
				break
			}
		}

		if firstDelta < 0 {
			// No deltas in this contour — skip.
			contourStart = end + 1
			continue
		}

		// Walk through the contour finding reference point pairs.
		curDelta := firstDelta
		nextP := curDelta + 1

		for nextP <= end {
			if hasDelta[nextP] {
				// Interpolate the gap between curDelta and nextP.
				if nextP > curDelta+1 {
					gvarIUPInterpolateRange(result, outlinePoints, axis,
						curDelta+1, nextP-1, curDelta, nextP)
				}
				curDelta = nextP
			}
			nextP++
		}

		// Handle wrapping: if only one delta point, shift everything.
		if curDelta == firstDelta {
			gvarIUPShiftRange(result, curDelta, contourStart, end)
		} else {
			// Handle the gap from curDelta to end of contour, wrapping
			// to firstDelta.
			if curDelta < end {
				gvarIUPInterpolateRange(result, outlinePoints, axis,
					curDelta+1, end, curDelta, firstDelta)
			}
			if firstDelta > contourStart {
				gvarIUPInterpolateRange(result, outlinePoints, axis,
					contourStart, firstDelta-1, curDelta, firstDelta)
			}
		}

		contourStart = end + 1
	}

	// Phantom points (beyond contours) get their deltas directly from
	// sparseDeltas via the hasDelta lookup above. No IUP needed since
	// phantom points don't belong to any contour.

	return result
}

// iupShiftRange shifts all points in [start, end] by the delta of refIdx.
//
// Matches skrifa Jiggler::shift (deltas.rs:217-233).
func gvarIUPShiftRange(result []int32, refIdx, start, end int) {
	delta := result[refIdx]
	if delta == 0 {
		return
	}
	for p := start; p <= end; p++ {
		if p != refIdx {
			result[p] += delta
		}
	}
}

// iupInterpolateRange interpolates deltas for points in [rangeStart, rangeEnd]
// using two reference points ref1 and ref2.
//
// Matches skrifa Jiggler::interpolate (deltas.rs:241-289).
func gvarIUPInterpolateRange(
	result []int32,
	outlinePoints [][2]int32,
	axis int,
	rangeStart, rangeEnd int,
	ref1, ref2 int,
) {
	if rangeStart > rangeEnd {
		return
	}

	// Ensure ref1 has the smaller coordinate.
	r1, r2 := ref1, ref2
	if ref1 < len(outlinePoints) && ref2 < len(outlinePoints) {
		if outlinePoints[ref1][axis] > outlinePoints[ref2][axis] {
			r1, r2 = ref2, ref1
		}
	} else {
		return
	}

	in1 := outlinePoints[r1][axis]
	in2 := outlinePoints[r2][axis]
	out1 := in1 + result[r1]
	out2 := in2 + result[r2]

	// If coordinates are equal but deltas differ, inferred delta is 0.
	// If coordinates and deltas are both equal, apply the shared delta.
	if in1 == in2 && out1 != out2 {
		return
	}

	d1 := out1 - in1 // delta of ref1
	d2 := out2 - in2 // delta of ref2

	for p := rangeStart; p <= rangeEnd; p++ {
		if p >= len(outlinePoints) {
			break
		}
		coord := outlinePoints[p][axis]

		var newDelta int32
		switch {
		case coord <= in1:
			newDelta = d1
		case coord >= in2:
			newDelta = d2
		default:
			// Linear interpolation: out1 + (coord - in1) * (out2 - out1) / (in2 - in1)
			if in2 != in1 {
				newDelta = d1 + int32(int64(out2-out1)*int64(coord-in1)/int64(in2-in1))
			}
		}
		result[p] = newDelta
	}
}
