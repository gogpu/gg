// Cmap table parser — character to glyph ID mapping.
//
// Supports the three most common cmap subtable formats:
//   - Format 4:  Segment mapping to delta values (BMP characters)
//   - Format 6:  Trimmed table mapping (sequential range)
//   - Format 12: Segmented coverage (full Unicode, 32-bit code points)
//
// Selection priority: format 12 > format 4 > format 6.
// This matches Skia/FreeType/skrifa priority (prefer full Unicode coverage).
//
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/cmap
// Reference: skrifa read-fonts/src/tables/cmap.rs
//
// This file is part of Phase 3a (ADR-048: Pure Go Font Stack).
package text

import (
	"encoding/binary"
	"sort"
)

// cmapLookup provides character-to-glyph-ID mapping parsed from a cmap table.
type cmapLookup struct {
	// lookup is the resolved lookup function.
	// Dispatches to format4Lookup, format6Lookup, or format12Lookup.
	lookup func(r rune) uint16
}

// glyphIndex returns the glyph ID for the given rune.
// Returns 0 (.notdef) if the character is not mapped.
func (c *cmapLookup) glyphIndex(r rune) uint16 {
	if c == nil || c.lookup == nil {
		return 0
	}
	return c.lookup(r)
}

// parseCmapTable parses a cmap table and returns the best subtable as a
// cmapLookup. Prefers format 12 (full Unicode), then format 4 (BMP),
// then format 6 (trimmed range).
//
// Returns nil if no supported subtable is found (not an error — some
// fonts may have only unsupported formats).
func parseCmapTable(data []byte) *cmapLookup {
	if len(data) < 4 {
		return nil
	}

	// Cmap header:
	//   uint16 version (must be 0)
	//   uint16 numTables
	numTables := int(binary.BigEndian.Uint16(data[2:4]))
	if len(data) < 4+numTables*8 {
		return nil
	}

	// Scan encoding records to find the best subtable.
	// Priority: format 12 > format 4 > format 6.
	//
	// Encoding record:
	//   uint16 platformID
	//   uint16 encodingID
	//   Offset32 subtableOffset
	//
	// We look for Unicode subtables:
	//   (0, 3) Unicode BMP
	//   (0, 4) Unicode full repertoire (format 12)
	//   (3, 1) Windows Unicode BMP
	//   (3, 10) Windows Unicode full repertoire
	var bestOffset int
	var bestPriority int // higher = better

	for i := range numTables {
		recOff := 4 + i*8
		platformID := binary.BigEndian.Uint16(data[recOff : recOff+2])
		encodingID := binary.BigEndian.Uint16(data[recOff+2 : recOff+4])
		subtableOffset := int(binary.BigEndian.Uint32(data[recOff+4 : recOff+8]))

		if subtableOffset >= len(data) {
			continue
		}

		priority := cmapEncodingPriority(platformID, encodingID)
		if priority > bestPriority {
			bestPriority = priority
			bestOffset = subtableOffset
		}
	}

	if bestPriority == 0 {
		return nil
	}

	return parseCmapSubtable(data, bestOffset)
}

// cmapEncodingPriority returns the priority for a platform/encoding pair.
// Higher values indicate better coverage. Returns 0 for unsupported pairs.
func cmapEncodingPriority(platformID, encodingID uint16) int {
	switch {
	case platformID == 0 && encodingID == 4:
		return 6 // Unicode full repertoire (format 12)
	case platformID == 3 && encodingID == 10:
		return 5 // Windows Unicode full repertoire (format 12)
	case platformID == 0 && encodingID == 3:
		return 4 // Unicode BMP (format 4)
	case platformID == 3 && encodingID == 1:
		return 3 // Windows Unicode BMP (format 4)
	case platformID == 0 && (encodingID == 1 || encodingID == 2):
		return 2 // Unicode older variants
	case platformID == 1 && encodingID == 0:
		return 1 // Mac Roman (legacy)
	default:
		return 0
	}
}

// parseCmapSubtable dispatches to the appropriate format parser based on the
// format field at the subtable offset.
func parseCmapSubtable(data []byte, offset int) *cmapLookup {
	if offset+2 > len(data) {
		return nil
	}
	format := binary.BigEndian.Uint16(data[offset : offset+2])
	switch format {
	case 4:
		return parseCmapFormat4(data[offset:])
	case 6:
		return parseCmapFormat6(data[offset:])
	case 12:
		return parseCmapFormat12(data[offset:])
	default:
		return nil
	}
}

// --- Format 4: Segment mapping to delta values ---

// cmapFormat4 holds the parsed data for a cmap format 4 subtable.
// Format 4 covers BMP characters (U+0000 to U+FFFF) using segments.
type cmapFormat4 struct {
	endCode        []uint16
	startCode      []uint16
	idDelta        []int16
	idRangeOffset  []uint16
	glyphIDArray   []uint16
	segCount       int
	rangeOffsetPos int // byte offset of idRangeOffset[] within subtable data
}

// parseCmapFormat4 parses a cmap format 4 subtable.
//
// Layout:
//
//	uint16 format (4)
//	uint16 length
//	uint16 language
//	uint16 segCountX2
//	uint16 searchRange
//	uint16 entrySelector
//	uint16 rangeShift
//	uint16 endCode[segCount]
//	uint16 reservedPad
//	uint16 startCode[segCount]
//	int16  idDelta[segCount]
//	uint16 idRangeOffset[segCount]
//	uint16 glyphIdArray[variable]
func parseCmapFormat4(data []byte) *cmapLookup {
	if len(data) < 14 {
		return nil
	}

	segCountX2 := int(binary.BigEndian.Uint16(data[6:8]))
	segCount := segCountX2 / 2
	if segCount == 0 {
		return nil
	}

	// Calculate expected minimum size.
	// Header (14) + endCode (segCount*2) + pad (2) + startCode (segCount*2) +
	// idDelta (segCount*2) + idRangeOffset (segCount*2)
	headerSize := 14
	arraysSize := segCount * 2 * 4 // 4 arrays
	padSize := 2
	minSize := headerSize + arraysSize + padSize
	if len(data) < minSize {
		return nil
	}

	endCodeOff := headerSize
	startCodeOff := endCodeOff + segCount*2 + padSize
	idDeltaOff := startCodeOff + segCount*2
	idRangeOffsetOff := idDeltaOff + segCount*2
	glyphArrayOff := idRangeOffsetOff + segCount*2

	tbl := &cmapFormat4{
		endCode:        make([]uint16, segCount),
		startCode:      make([]uint16, segCount),
		idDelta:        make([]int16, segCount),
		idRangeOffset:  make([]uint16, segCount),
		segCount:       segCount,
		rangeOffsetPos: idRangeOffsetOff,
	}

	for i := range segCount {
		tbl.endCode[i] = binary.BigEndian.Uint16(data[endCodeOff+i*2 : endCodeOff+i*2+2])
		tbl.startCode[i] = binary.BigEndian.Uint16(data[startCodeOff+i*2 : startCodeOff+i*2+2])
		tbl.idDelta[i] = int16(binary.BigEndian.Uint16(data[idDeltaOff+i*2 : idDeltaOff+i*2+2]))
		tbl.idRangeOffset[i] = binary.BigEndian.Uint16(data[idRangeOffsetOff+i*2 : idRangeOffsetOff+i*2+2])
	}

	// Parse glyphIdArray (remainder of the subtable).
	if glyphArrayOff < len(data) {
		glyphArrayLen := (len(data) - glyphArrayOff) / 2
		tbl.glyphIDArray = make([]uint16, glyphArrayLen)
		for i := range glyphArrayLen {
			tbl.glyphIDArray[i] = binary.BigEndian.Uint16(data[glyphArrayOff+i*2 : glyphArrayOff+i*2+2])
		}
	}

	// Capture data slice for idRangeOffset-based lookups.
	capturedData := data

	return &cmapLookup{
		lookup: func(r rune) uint16 {
			if r < 0 || r > 0xFFFF {
				return 0
			}
			code := uint16(r)
			return format4Lookup(tbl, capturedData, code)
		},
	}
}

// format4Lookup performs a binary search in a format 4 cmap subtable.
func format4Lookup(tbl *cmapFormat4, data []byte, code uint16) uint16 {
	// Binary search for the segment containing code.
	lo, hi := 0, tbl.segCount
	for lo < hi {
		mid := (lo + hi) / 2
		if tbl.endCode[mid] < code {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo >= tbl.segCount {
		return 0
	}
	if code < tbl.startCode[lo] {
		return 0
	}

	if tbl.idRangeOffset[lo] == 0 {
		return uint16(int16(code) + tbl.idDelta[lo])
	}

	// idRangeOffset-based lookup.
	// The offset is relative to the position of idRangeOffset[lo] itself.
	// glyphIndex = *(idRangeOffset[lo]/2 + (code - startCode[lo]) + &idRangeOffset[lo])
	// In byte terms: byte position of idRangeOffset[lo] + idRangeOffset[lo] + (code - startCode[lo])*2
	bytePos := tbl.rangeOffsetPos + lo*2 + int(tbl.idRangeOffset[lo]) + int(code-tbl.startCode[lo])*2
	if bytePos+2 > len(data) {
		return 0
	}
	gid := binary.BigEndian.Uint16(data[bytePos : bytePos+2])
	if gid == 0 {
		return 0
	}
	return uint16(int16(gid) + tbl.idDelta[lo])
}

// --- Format 6: Trimmed table mapping ---

// parseCmapFormat6 parses a cmap format 6 subtable.
//
// Layout:
//
//	uint16 format (6)
//	uint16 length
//	uint16 language
//	uint16 firstCode
//	uint16 entryCount
//	uint16 glyphIdArray[entryCount]
func parseCmapFormat6(data []byte) *cmapLookup {
	if len(data) < 10 {
		return nil
	}

	firstCode := int(binary.BigEndian.Uint16(data[6:8]))
	entryCount := int(binary.BigEndian.Uint16(data[8:10]))

	if len(data) < 10+entryCount*2 {
		return nil
	}

	glyphs := make([]uint16, entryCount)
	for i := range entryCount {
		glyphs[i] = binary.BigEndian.Uint16(data[10+i*2 : 10+i*2+2])
	}

	return &cmapLookup{
		lookup: func(r rune) uint16 {
			code := int(r)
			idx := code - firstCode
			if idx < 0 || idx >= len(glyphs) {
				return 0
			}
			return glyphs[idx]
		},
	}
}

// --- Format 12: Segmented coverage (full Unicode) ---

// cmapFormat12Group is a single group in a format 12 cmap subtable.
type cmapFormat12Group struct {
	startCharCode uint32
	endCharCode   uint32
	startGlyphID  uint32
}

// parseCmapFormat12 parses a cmap format 12 subtable.
//
// Layout:
//
//	uint16  format (12)
//	uint16  reserved
//	uint32  length
//	uint32  language
//	uint32  numGroups
//	Group[numGroups]:
//	    uint32 startCharCode
//	    uint32 endCharCode
//	    uint32 startGlyphID
func parseCmapFormat12(data []byte) *cmapLookup {
	if len(data) < 16 {
		return nil
	}

	numGroups := int(binary.BigEndian.Uint32(data[12:16]))
	if len(data) < 16+numGroups*12 {
		return nil
	}

	groups := make([]cmapFormat12Group, numGroups)
	for i := range numGroups {
		off := 16 + i*12
		groups[i] = cmapFormat12Group{
			startCharCode: binary.BigEndian.Uint32(data[off : off+4]),
			endCharCode:   binary.BigEndian.Uint32(data[off+4 : off+8]),
			startGlyphID:  binary.BigEndian.Uint32(data[off+8 : off+12]),
		}
	}

	return &cmapLookup{
		lookup: func(r rune) uint16 {
			code := uint32(r)
			// Binary search for the group containing code.
			idx := sort.Search(len(groups), func(i int) bool {
				return groups[i].endCharCode >= code
			})
			if idx >= len(groups) {
				return 0
			}
			g := groups[idx]
			if code < g.startCharCode || code > g.endCharCode {
				return 0
			}
			gid := g.startGlyphID + (code - g.startCharCode)
			if gid > 0xFFFF {
				return 0 // GID overflow (uint16 limit)
			}
			return uint16(gid)
		},
	}
}
