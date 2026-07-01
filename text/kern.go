// kern table parser — fallback kerning for fonts without GPOS.
//
// Parses the legacy 'kern' table (format 0) which stores kerning as
// a flat list of (left, right) → value pairs. This is the pre-OpenType
// kerning mechanism, still present in many fonts alongside or instead
// of GPOS Lookup Type 2.
//
// The kern table is only used as a fallback when:
//   - No GPOS table is present, OR
//   - The GPOS table does not contain a 'kern' feature
//
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/kern
//
// This file is part of Phase 5 (ADR-048: Pure Go Font Stack).
package text

import (
	"encoding/binary"
	"sort"
)

// kernTable holds parsed kern pairs from the legacy kern table.
type kernTable struct {
	pairs []kernPair // sorted by (left<<16 | right) for binary search
}

// kernPair stores a single kerning pair.
type kernPair struct {
	left  uint16
	right uint16
	value int16 // kerning value in font units
}

// kernPairKey computes a sort key for binary search.
func kernPairKey(left, right uint16) uint32 {
	return uint32(left)<<16 | uint32(right)
}

// parseKern parses the legacy kern table.
// Supports kern table version 0 with format 0 subtables (simple pairs).
// Microsoft-style version 1 kern tables (used on macOS) are not supported.
func parseKern(data []byte) *kernTable {
	if len(data) < 4 {
		return nil
	}

	version := binary.BigEndian.Uint16(data[0:2])
	if version != 0 {
		// Version 1 (Apple) uses different header format — skip for now.
		return nil
	}

	nTables := int(binary.BigEndian.Uint16(data[2:4]))
	if nTables == 0 {
		return nil
	}

	var allPairs []kernPair
	offset := 4

	for t := range nTables {
		_ = t
		if offset+6 > len(data) {
			break
		}
		// Subtable header:
		//   uint16 version (0)
		//   uint16 length
		//   uint16 coverage
		subtableLength := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		coverage := binary.BigEndian.Uint16(data[offset+4 : offset+6])

		// Coverage bits:
		//   bit 0: 1 = horizontal, 0 = vertical (we only want horizontal)
		//   bit 1: 1 = minimum values, 0 = kerning values
		//   bit 2: 1 = cross-stream, 0 = normal
		//   bits 8-15: format (0 = format 0)
		format := coverage >> 8
		isHorizontal := coverage&0x01 != 0
		isMinimum := coverage&0x02 != 0

		subtableEnd := offset + subtableLength
		if subtableEnd > len(data) {
			break
		}

		if format == 0 && isHorizontal && !isMinimum {
			pairs := parseKernFormat0(data[offset+6 : subtableEnd])
			allPairs = append(allPairs, pairs...)
		}

		offset = subtableEnd
	}

	if len(allPairs) == 0 {
		return nil
	}

	// Sort pairs for binary search.
	sort.Slice(allPairs, func(i, j int) bool {
		ki := kernPairKey(allPairs[i].left, allPairs[i].right)
		kj := kernPairKey(allPairs[j].left, allPairs[j].right)
		return ki < kj
	})

	return &kernTable{pairs: allPairs}
}

// parseKernFormat0 parses a format 0 subtable (simple pairs).
func parseKernFormat0(data []byte) []kernPair {
	if len(data) < 8 {
		return nil
	}
	nPairs := int(binary.BigEndian.Uint16(data[0:2]))
	// Skip searchRange(2), entrySelector(2), rangeShift(2).
	recordStart := 8
	if len(data) < recordStart+nPairs*6 {
		return nil
	}

	pairs := make([]kernPair, nPairs)
	for i := range nPairs {
		off := recordStart + i*6
		pairs[i] = kernPair{
			left:  binary.BigEndian.Uint16(data[off : off+2]),
			right: binary.BigEndian.Uint16(data[off+2 : off+4]),
			value: int16(binary.BigEndian.Uint16(data[off+4 : off+6])),
		}
	}
	return pairs
}

// kernValue returns the kerning value (in font units) for a glyph pair.
// Returns 0 if no kerning is defined for this pair.
func (k *kernTable) kernValue(left, right uint16) int16 {
	if k == nil || len(k.pairs) == 0 {
		return 0
	}

	key := kernPairKey(left, right)
	idx := sort.Search(len(k.pairs), func(i int) bool {
		return kernPairKey(k.pairs[i].left, k.pairs[i].right) >= key
	})
	if idx < len(k.pairs) {
		p := k.pairs[idx]
		if p.left == left && p.right == right {
			return p.value
		}
	}
	return 0
}
