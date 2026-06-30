// Font table directory parser — Pure Go binary parsing.
//
// Provides indexed table directory access for TrueType/OpenType fonts
// and font collections (TTC/OTC). Reuses parseFontTables() from tt_font.go
// for single-font cases and adds collection index support.
//
// This file is part of Phase 3a (ADR-048: Pure Go Font Stack).
package text

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// parseFontTablesIndex parses the table directory of a font at the given
// index within the font data. For standalone fonts (TTF/OTF), index is
// ignored and the single font is parsed. For font collections (TTC/OTC),
// index selects which font to use (0 = first).
//
// This extends parseFontTables() from tt_font.go with collection index
// support, needed by ownParser.ParseIndex.
func parseFontTablesIndex(fontData []byte, index int) (map[string][]byte, error) {
	if len(fontData) < 12 {
		return nil, errors.New("font data too short")
	}

	sfVersion := binary.BigEndian.Uint32(fontData[0:4])

	// Not a collection — delegate to existing parser (ignores index).
	if sfVersion != tagTTCF {
		return parseFontTables(fontData)
	}

	// TTC header:
	//   Tag      ttcTag    ('ttcf')
	//   uint16   majorVersion
	//   uint16   minorVersion
	//   uint32   numFonts
	//   Offset32 offsets[numFonts]
	if len(fontData) < 12 {
		return nil, errors.New("TTC header too short")
	}
	numFonts := int(binary.BigEndian.Uint32(fontData[8:12]))
	if numFonts == 0 {
		return nil, errors.New("TTC has zero fonts")
	}
	if index < 0 || index >= numFonts {
		return nil, fmt.Errorf("TTC index %d out of range (collection has %d fonts)", index, numFonts)
	}

	offsetsEnd := 12 + numFonts*4
	if len(fontData) < offsetsEnd {
		return nil, errors.New("TTC offset array truncated")
	}

	fontOffset := int(binary.BigEndian.Uint32(fontData[12+index*4 : 12+index*4+4]))
	return parseTableDirectoryAt(fontData, fontOffset)
}

// parseTableDirectoryAt parses the table directory starting at the given
// byte offset within fontData. Used by parseFontTablesIndex for TTC support.
func parseTableDirectoryAt(fontData []byte, offset int) (map[string][]byte, error) {
	if offset+12 > len(fontData) {
		return nil, errors.New("font offset beyond data")
	}

	// Read sfnt header at offset.
	// Skip sfVersion (4 bytes).
	numTables := int(binary.BigEndian.Uint16(fontData[offset+4 : offset+6]))
	// Skip searchRange, entrySelector, rangeShift (6 bytes).

	recordsStart := offset + 12
	recordsEnd := recordsStart + numTables*16
	if recordsEnd > len(fontData) {
		return nil, errors.New("table records extend beyond font data")
	}

	tables := make(map[string][]byte, numTables)
	for i := range numTables {
		recOff := recordsStart + i*16
		tag := string(fontData[recOff : recOff+4])
		// Skip checksum at recOff+4.
		tableOffset := int(binary.BigEndian.Uint32(fontData[recOff+8 : recOff+12]))
		length := int(binary.BigEndian.Uint32(fontData[recOff+12 : recOff+16]))
		end := tableOffset + length
		if end > len(fontData) {
			continue // Skip tables extending beyond data.
		}
		tables[tag] = fontData[tableOffset:end]
	}

	return tables, nil
}

// tagTTCF is the TrueType Collection tag ('ttcf').
const tagTTCF = 0x74746366
