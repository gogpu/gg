// Package text provides GPU text rendering infrastructure.
//
// This file implements the OpenType ItemVariationStore (IVS) and
// DeltaSetIndexMap parsers. These are reusable components used by
// HVAR, VVAR, MVAR, GDEF, and COLR tables for variation deltas.
//
// Reference: skrifa (Google fontations) read-fonts/src/tables/variations.rs
// Spec: https://learn.microsoft.com/en-us/typography/opentype/spec/otvarcommonformats
package text

import (
	"encoding/binary"
	"fmt"
)

// itemVariationStore holds parsed ItemVariationStore data.
// It contains variation regions and per-subtable delta sets.
//
// Binary layout (spec):
//
//	uint16  format (must be 1)
//	Offset32 variationRegionListOffset
//	uint16  itemVariationDataCount
//	Offset32[itemVariationDataCount] itemVariationDataOffsets
type itemVariationStore struct {
	regions  []variationRegion
	itemData []itemVariationData
}

// variationRegion describes a region in the variation space.
// Each region has one set of axis coordinates per axis in the font.
type variationRegion struct {
	axes []regionAxisCoords
}

// regionAxisCoords defines the start/peak/end coordinates for one axis
// within a variation region. All values are F2.14 normalized coordinates
// stored as int16 (range -16384 to +16384, where 16384 = 1.0).
type regionAxisCoords struct {
	startCoord int16 // F2.14
	peakCoord  int16 // F2.14
	endCoord   int16 // F2.14
}

// itemVariationData holds the delta sets for one subtable of the IVS.
//
// Binary layout:
//
//	uint16  itemCount
//	uint16  wordDeltaCount (high bit = longWords flag)
//	uint16  regionIndexCount
//	uint16[regionIndexCount] regionIndexes
//	deltaSets[itemCount] (variable-size rows)
type itemVariationData struct {
	regionIndices []uint16
	deltaSets     [][]int32 // [item][region] deltas
}

// computeDelta computes the variation delta for the given outer/inner index
// and normalized variation coordinates. This is the core IVS operation.
//
// The computation uses 64-bit precision and rounds the result, matching
// skrifa's ItemVariationStore::compute_delta (variations.rs:1484-1513)
// and FreeType's tt_var_get_item_delta.
//
// Returns 0 if coords are empty or indices are out of range.
func (ivs *itemVariationStore) computeDelta(outer, inner uint16, coords []int16) int32 {
	if len(coords) == 0 {
		return 0
	}
	if int(outer) >= len(ivs.itemData) {
		return 0
	}
	data := &ivs.itemData[outer]
	if int(inner) >= len(data.deltaSets) {
		return 0
	}
	deltas := data.deltaSets[inner]

	var accum int64
	for i, regionIdx := range data.regionIndices {
		if int(regionIdx) >= len(ivs.regions) || i >= len(deltas) {
			continue
		}
		region := &ivs.regions[regionIdx]
		scalar := region.computeScalar(coords)
		if scalar != 0 {
			accum += int64(deltas[i]) * int64(scalar)
		}
	}
	// Round: (accum + 0x8000) >> 16, matching skrifa and FreeType.
	return int32((accum + 0x8000) >> 16)
}

// computeScalar computes the interpolation scalar for this region given
// normalized variation coordinates. Returns a Fixed 16.16 value where
// 0x10000 = 1.0.
//
// Matches skrifa VariationRegion::compute_scalar (variations.rs:1596-1617).
// Uses Fixed 16.16 arithmetic for precision parity with skrifa/FreeType.
func (r *variationRegion) computeScalar(coords []int16) int32 {
	// 1.0 in Fixed 16.16
	scalar := int32(0x10000)

	for i, axis := range r.axes {
		peak := int32(axis.peakCoord)
		if peak == 0 {
			continue
		}
		start := int32(axis.startCoord)
		end := int32(axis.endCoord)

		// Skip axes where the region definition is degenerate:
		// start > peak, peak > end, or region spans zero (start < 0 && end > 0).
		// This matches skrifa's active_region_axes filter (variations.rs:1645-1659).
		if start > peak || peak > end || (start < 0 && end > 0) {
			continue
		}

		// Get the coordinate for this axis (default 0 if not provided).
		coord := int32(0)
		if i < len(coords) {
			coord = int32(coords[i])
		}

		if coord < start || coord > end {
			return 0
		}
		if coord == peak {
			continue
		}

		// Interpolate using mul_div pattern from skrifa.
		// scalar = scalar * (coord - start) / (peak - start)  [coord < peak]
		// scalar = scalar * (end - coord) / (end - peak)      [coord > peak]
		if coord < peak {
			scalar = mulDiv(scalar, coord-start, peak-start)
		} else {
			scalar = mulDiv(scalar, end-coord, end-peak)
		}
	}
	return scalar
}

// mulDiv computes (a * b) / c with 64-bit intermediate precision.
// This matches skrifa's Fixed::mul_div.
func mulDiv(a, b, c int32) int32 {
	if c == 0 {
		return 0
	}
	return int32(int64(a) * int64(b) / int64(c))
}

// parseItemVariationStore parses an ItemVariationStore from raw bytes.
// The data starts at the IVS header (format field).
func parseItemVariationStore(data []byte) (*itemVariationStore, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("text: IVS data too short: %d bytes", len(data))
	}

	format := binary.BigEndian.Uint16(data[0:2])
	if format != 1 {
		return nil, fmt.Errorf("text: unsupported IVS format %d (expected 1)", format)
	}

	regionListOffset := binary.BigEndian.Uint32(data[2:6])
	dataCount := binary.BigEndian.Uint16(data[6:8])

	// Read item variation data offsets.
	offsetsStart := 8
	if len(data) < offsetsStart+int(dataCount)*4 {
		return nil, fmt.Errorf("text: IVS data too short for %d data offsets", dataCount)
	}

	// Parse variation region list.
	if int(regionListOffset) >= len(data) {
		return nil, fmt.Errorf("text: IVS region list offset %d out of range", regionListOffset)
	}
	regions, axisCount, err := parseVariationRegionList(data[regionListOffset:])
	if err != nil {
		return nil, fmt.Errorf("text: IVS region list: %w", err)
	}

	// Parse each ItemVariationData subtable.
	itemData := make([]itemVariationData, dataCount)
	for i := range dataCount {
		off := binary.BigEndian.Uint32(data[offsetsStart+int(i)*4:])
		if off == 0 {
			continue // Null offset — empty subtable.
		}
		if int(off) >= len(data) {
			return nil, fmt.Errorf("text: IVS data subtable %d offset %d out of range", i, off)
		}
		ivd, err := parseItemVariationData(data[off:], axisCount)
		if err != nil {
			return nil, fmt.Errorf("text: IVS data subtable %d: %w", i, err)
		}
		itemData[i] = ivd
	}

	return &itemVariationStore{
		regions:  regions,
		itemData: itemData,
	}, nil
}

// parseVariationRegionList parses the VariationRegionList table.
// Returns the regions and the axis count.
//
// Binary layout:
//
//	uint16  axisCount
//	uint16  regionCount
//	VariationRegion[regionCount] (each: axisCount * RegionAxisCoordinates)
//	RegionAxisCoordinates: int16 startCoord, int16 peakCoord, int16 endCoord (6 bytes)
func parseVariationRegionList(data []byte) ([]variationRegion, uint16, error) {
	if len(data) < 4 {
		return nil, 0, fmt.Errorf("region list too short: %d bytes", len(data))
	}

	axisCount := binary.BigEndian.Uint16(data[0:2])
	regionCount := binary.BigEndian.Uint16(data[2:4])

	regionSize := int(axisCount) * 6 // 6 bytes per RegionAxisCoordinates
	needed := 4 + int(regionCount)*regionSize
	if len(data) < needed {
		return nil, 0, fmt.Errorf("region list too short: need %d, have %d", needed, len(data))
	}

	regions := make([]variationRegion, regionCount)
	offset := 4
	for i := range regionCount {
		axes := make([]regionAxisCoords, axisCount)
		for j := range axisCount {
			axes[j] = regionAxisCoords{
				startCoord: int16(binary.BigEndian.Uint16(data[offset:])),
				peakCoord:  int16(binary.BigEndian.Uint16(data[offset+2:])),
				endCoord:   int16(binary.BigEndian.Uint16(data[offset+4:])),
			}
			offset += 6
		}
		regions[i] = variationRegion{axes: axes}
	}

	return regions, axisCount, nil
}

// parseItemVariationData parses one ItemVariationData subtable.
//
// Binary layout:
//
//	uint16  itemCount
//	uint16  wordDeltaCount (bit 15 = longWords)
//	uint16  regionIndexCount
//	uint16[regionIndexCount] regionIndexes
//	deltaSets[itemCount] (packed rows of deltas)
//
// Delta row encoding depends on wordDeltaCount and longWords flag:
//   - longWords=false: first wordDeltaCount deltas as int16, rest as int8
//   - longWords=true:  first wordDeltaCount deltas as int32, rest as int16
//
// See skrifa ItemVariationData::delta_set (variations.rs:1662-1699)
func parseItemVariationData(data []byte, _ uint16) (itemVariationData, error) {
	if len(data) < 6 {
		return itemVariationData{}, fmt.Errorf("item variation data too short: %d bytes", len(data))
	}

	itemCount := binary.BigEndian.Uint16(data[0:2])
	wordDeltaCountRaw := binary.BigEndian.Uint16(data[2:4])
	regionIndexCount := binary.BigEndian.Uint16(data[4:6])

	longWords := (wordDeltaCountRaw & 0x8000) != 0
	wordDeltaCount := wordDeltaCountRaw & 0x7FFF

	// Parse region indices.
	riStart := 6
	riEnd := riStart + int(regionIndexCount)*2
	if len(data) < riEnd {
		return itemVariationData{}, fmt.Errorf("data too short for %d region indices", regionIndexCount)
	}

	regionIndices := make([]uint16, regionIndexCount)
	for i := range regionIndexCount {
		regionIndices[i] = binary.BigEndian.Uint16(data[riStart+int(i)*2:])
	}

	// Compute bytes per delta row.
	// Matches skrifa ItemVariationData::delta_row_len (variations.rs:1692-1698).
	var wordSize, smallSize int
	if longWords {
		wordSize = 4
		smallSize = 2
	} else {
		wordSize = 2
		smallSize = 1
	}
	longDeltaCount := int(wordDeltaCount)
	shortDeltaCount := int(regionIndexCount) - longDeltaCount
	if shortDeltaCount < 0 {
		shortDeltaCount = 0
	}
	bytesPerRow := longDeltaCount*wordSize + shortDeltaCount*smallSize

	// Parse delta sets.
	deltaSetsStart := riEnd
	deltaSets := make([][]int32, itemCount)
	for i := range itemCount {
		rowStart := deltaSetsStart + int(i)*bytesPerRow
		if rowStart+bytesPerRow > len(data) {
			return itemVariationData{}, fmt.Errorf("data too short for delta row %d", i)
		}
		row := make([]int32, regionIndexCount)
		pos := rowStart

		for j := range regionIndexCount {
			isWordDelta := j < wordDeltaCount
			switch {
			case !isWordDelta && longWords:
				// Long words mode, non-word delta → int16
				row[j] = int32(int16(binary.BigEndian.Uint16(data[pos:])))
				pos += 2
			case !isWordDelta && !longWords:
				// Short words mode, non-word delta → int8
				row[j] = int32(int8(data[pos]))
				pos++
			case isWordDelta && longWords:
				// Long words mode, word delta → int32
				row[j] = int32(binary.BigEndian.Uint32(data[pos:]))
				pos += 4
			default:
				// Short words mode, word delta → int16
				row[j] = int32(int16(binary.BigEndian.Uint16(data[pos:])))
				pos += 2
			}
		}
		deltaSets[i] = row
	}

	return itemVariationData{
		regionIndices: regionIndices,
		deltaSets:     deltaSets,
	}, nil
}

// deltaSetIndexMap maps glyph IDs (or other indices) to (outer, inner)
// pairs for looking up deltas in an ItemVariationStore.
//
// When nil, the identity mapping is used: outer=0, inner=glyphID.
//
// Binary layout:
//
//	uint8   format (0 or 1)
//	uint8   entryFormat (packed: bits[5:4]=entrySize-1, bits[3:0]=innerBits-1)
//	uint16  mapCount (format 0) or uint32 mapCount (format 1)
//	entries[mapCount] (1-4 bytes each, depending on entryFormat)
type deltaSetIndexMap struct {
	entries   []uint32
	innerBits uint8
}

// get returns the (outer, inner) delta set index for a glyph ID.
// If the map is nil, the identity mapping (outer=0, inner=glyphID) is used.
//
// If glyphID >= len(entries), the last entry is used (per spec).
// See skrifa DeltaSetIndexMap::get (variations.rs:1441-1478).
func (m *deltaSetIndexMap) get(glyphID uint16) (outer, inner uint16) {
	if m == nil {
		return 0, glyphID
	}
	if len(m.entries) == 0 {
		return 0, glyphID
	}
	idx := int(glyphID)
	if idx >= len(m.entries) {
		idx = len(m.entries) - 1
	}
	entry := m.entries[idx]
	inner = uint16(entry & ((1 << m.innerBits) - 1))
	outer = uint16(entry >> m.innerBits)
	return
}

// parseDeltaSetIndexMap parses a DeltaSetIndexMap from raw bytes.
// Returns nil if the data represents a null/empty mapping.
func parseDeltaSetIndexMap(data []byte) (*deltaSetIndexMap, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("delta set index map too short: %d bytes", len(data))
	}

	format := data[0]
	entryFormat := data[1]

	// Extract entry size (bits [5:4] + 1) and inner bit count (bits [3:0] + 1).
	// Matches skrifa EntryFormat (generated_variations.rs:581-591).
	entrySize := ((entryFormat >> 4) & 0x03) + 1
	innerBits := (entryFormat & 0x0F) + 1

	var mapCount uint32
	var dataStart int
	switch format {
	case 0:
		if len(data) < 4 {
			return nil, fmt.Errorf("format 0 map too short")
		}
		mapCount = uint32(binary.BigEndian.Uint16(data[2:4]))
		dataStart = 4
	case 1:
		if len(data) < 6 {
			return nil, fmt.Errorf("format 1 map too short")
		}
		mapCount = binary.BigEndian.Uint32(data[2:6])
		dataStart = 6
	default:
		return nil, fmt.Errorf("unsupported delta set index map format %d", format)
	}

	if mapCount == 0 {
		return nil, nil //nolint:nilnil // null mapping is valid per spec
	}

	needed := dataStart + int(mapCount)*int(entrySize)
	if len(data) < needed {
		return nil, fmt.Errorf("map data too short: need %d, have %d", needed, len(data))
	}

	entries := make([]uint32, mapCount)
	for i := range mapCount {
		offset := dataStart + int(i)*int(entrySize)
		switch entrySize {
		case 1:
			entries[i] = uint32(data[offset])
		case 2:
			entries[i] = uint32(binary.BigEndian.Uint16(data[offset:]))
		case 3:
			entries[i] = uint32(data[offset])<<16 | uint32(data[offset+1])<<8 | uint32(data[offset+2])
		case 4:
			entries[i] = binary.BigEndian.Uint32(data[offset:])
		}
	}

	return &deltaSetIndexMap{
		entries:   entries,
		innerBits: innerBits,
	}, nil
}
