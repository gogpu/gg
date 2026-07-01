// Package text provides GPU text rendering infrastructure.
//
// This file implements the OpenType avar (Axis Variations) table parser.
// The avar table provides piecewise linear remapping of normalized axis
// coordinates, allowing font designers to define non-linear relationships
// between user-facing axis values and internal design-space coordinates.
//
// Reference: skrifa (Google fontations)
//   - read-fonts/src/tables/avar.rs -- avar parser + SegmentMaps.apply()
//   - HarfBuzz hb-ot-var-avar-table.hh -- extended avar behavior
//
// Spec: https://learn.microsoft.com/en-us/typography/opentype/spec/avar
package text

import (
	"encoding/binary"
)

// avarTable holds parsed avar (Axis Variations) data.
//
// The avar table contains one SegmentMap per font axis, providing
// piecewise linear remapping of normalized coordinates.
//
// Binary layout:
//
//	uint16  majorVersion (must be 1)
//	uint16  minorVersion (0)
//	uint16  reserved
//	uint16  axisCount
//	SegmentMaps[axisCount] (variable-length)
type avarTable struct {
	segmentMaps [][]avarSegment // one segment map per axis
}

// avarSegment represents a single mapping point in a piecewise linear
// segment map. Both values are F2.14 normalized coordinates stored as
// int16 (range -16384 to +16384, where 16384 = 1.0).
type avarSegment struct {
	fromCoord int16 // F2.14 input coordinate
	toCoord   int16 // F2.14 output coordinate
}

// apply remaps normalized coordinates in-place using the avar segment maps.
// Each axis's coordinate is transformed through its corresponding piecewise
// linear segment map.
//
// Matches skrifa SegmentMaps::apply (avar.rs:10-126) which follows HarfBuzz's
// extended avar behavior for robustness.
func (a *avarTable) apply(coords []int16) {
	if a == nil {
		return
	}
	for i := range coords {
		if i >= len(a.segmentMaps) {
			break
		}
		coords[i] = avarApplySegmentMap(a.segmentMaps[i], coords[i])
	}
}

// avarApplySegmentMap applies a single axis segment map to a normalized
// coordinate. This implements piecewise linear interpolation with HarfBuzz-
// compatible edge case handling.
//
// Matches skrifa SegmentMaps::apply (avar.rs:10-126):
//   - len < 2: passthrough or single-mapping shift
//   - Exact match: return corresponding output
//   - Between two mappings: linear interpolation
//   - Outside range: extrapolate by shifting with nearest mapping delta
func avarApplySegmentMap(maps []avarSegment, coord int16) int16 {
	n := len(maps)

	if n < 2 {
		if n == 0 {
			return coord
		}
		return coord - maps[0].fromCoord + maps[0].toCoord
	}

	// Trim duplicate -1/+1 caps (CoreText quirks, skrifa avar.rs:37-50).
	start, end := avarTrimCaps(maps)

	// Look for exact match (skrifa avar.rs:55-95).
	if result, found := avarExactMatch(maps, coord, start, end); found {
		return result
	}

	// Not exact: find the segment for interpolation (skrifa avar.rs:98-126).
	return avarInterpolate(maps, coord, start, end)
}

// avarTrimCaps trims duplicate -1/+1 cap entries from the segment map bounds.
// Returns adjusted start and end indices.
func avarTrimCaps(maps []avarSegment) (start, end int) {
	const neg1 int16 = -16384 // F2.14 -1.0
	const pos1 int16 = 16384  // F2.14 +1.0

	start = 0
	end = len(maps)

	if maps[start].fromCoord == neg1 && maps[start].toCoord == neg1 &&
		maps[start+1].fromCoord == neg1 {
		start++
	}

	if maps[end-1].fromCoord == pos1 && maps[end-1].toCoord == pos1 &&
		maps[end-2].fromCoord == pos1 {
		end--
	}

	return start, end
}

// avarExactMatch handles exact match and duplicate "from" entries.
// Returns the mapped value and true if found, otherwise false.
func avarExactMatch(maps []avarSegment, coord int16, start, end int) (int16, bool) {
	i := start
	for i < end {
		if coord == maps[i].fromCoord {
			break
		}
		i++
	}

	if i >= end {
		return 0, false
	}

	// Found at least one exact match. Check for consecutive equals.
	j := i
	for j+1 < end && coord == maps[j+1].fromCoord {
		j++
	}

	// Spec-compliant: exactly one match.
	if i == j {
		return maps[i].toCoord, true
	}

	// Exactly three -> return the middle one.
	if i+2 == j {
		return maps[i+1].toCoord, true
	}

	// Multiple matches: HarfBuzz tiebreaking.
	return avarResolveDuplicates(maps, coord, i, j), true
}

// avarResolveDuplicates resolves multiple identical "from" entries,
// matching HarfBuzz behavior.
func avarResolveDuplicates(maps []avarSegment, coord int16, i, j int) int16 {
	if coord < 0 {
		return maps[j].toCoord
	}
	if coord > 0 {
		return maps[i].toCoord
	}
	// coord == 0: choose the one with smaller |to|.
	ti := maps[i].toCoord
	tj := maps[j].toCoord
	if absI16(ti) < absI16(tj) {
		return ti
	}
	return tj
}

// avarInterpolate finds the bracketing segment and interpolates.
func avarInterpolate(maps []avarSegment, coord int16, start, end int) int16 {
	k := start
	for k < end {
		if coord < maps[k].fromCoord {
			break
		}
		k++
	}

	if k == start {
		return coord - maps[start].fromCoord + maps[start].toCoord
	}
	if k == end {
		return coord - maps[end-1].fromCoord + maps[end-1].toCoord
	}

	bf := int32(maps[k-1].fromCoord)
	bt := int32(maps[k-1].toCoord)
	af := int32(maps[k].fromCoord)
	at := int32(maps[k].toCoord)

	denom := af - bf
	if denom == 0 {
		return maps[k-1].toCoord
	}

	result := bt + (at-bt)*(int32(coord)-bf)/denom
	return int16(result)
}

// parseAvar parses an avar table from raw bytes.
// Returns nil if the data is too short or the version is unsupported.
//
// Binary layout:
//
//	uint16 majorVersion
//	uint16 minorVersion
//	uint16 reserved
//	uint16 axisCount
//	SegmentMaps[axisCount]:
//	  uint16 positionMapCount
//	  AxisValueMap[positionMapCount]:
//	    int16 fromCoordinate (F2.14)
//	    int16 toCoordinate   (F2.14)
func parseAvar(data []byte) *avarTable {
	if len(data) < 8 {
		return nil
	}

	major := binary.BigEndian.Uint16(data[0:2])
	if major != 1 {
		return nil
	}
	// minor at data[2:4] -- accept any minor version.
	// reserved at data[4:6] -- skip.
	axisCount := int(binary.BigEndian.Uint16(data[6:8]))

	if axisCount == 0 {
		return &avarTable{}
	}

	segmentMaps := make([][]avarSegment, 0, axisCount)
	offset := 8

	for range axisCount {
		if offset+2 > len(data) {
			return nil
		}
		mapCount := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2

		needed := mapCount * 4 // 2 bytes fromCoord + 2 bytes toCoord
		if offset+needed > len(data) {
			return nil
		}

		segments := make([]avarSegment, mapCount)
		for j := range mapCount {
			segments[j] = avarSegment{
				fromCoord: int16(binary.BigEndian.Uint16(data[offset:])),
				toCoord:   int16(binary.BigEndian.Uint16(data[offset+2:])),
			}
			offset += 4
		}
		segmentMaps = append(segmentMaps, segments)
	}

	return &avarTable{segmentMaps: segmentMaps}
}

// absI16 returns the absolute value of an int16.
func absI16(x int16) int16 {
	if x < 0 {
		return -x
	}
	return x
}
