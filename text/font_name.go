// Name table parser — font family and full name extraction.
//
// Parses the OpenType 'name' table to extract human-readable font names.
// Supports Windows Unicode (platform 3, encoding 1) with UTF-16BE decoding
// and Mac Roman (platform 1, encoding 0) as a fallback.
//
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/name
// Reference: skrifa read-fonts/src/tables/name.rs
//
// This file is part of Phase 3a (ADR-048: Pure Go Font Stack).
package text

import (
	"encoding/binary"
	"unicode/utf16"
)

// nameIDs for the records we care about.
const (
	nameIDFamily   = 1 // Font Family name
	nameIDFullName = 4 // Full font name
)

// parseNameTable extracts the font family name (nameID 1) and full name
// (nameID 4) from a raw 'name' table.
//
// Layout:
//
//	uint16 format (0 or 1)
//	uint16 count
//	Offset16 stringOffset
//	NameRecord[count]:
//	    uint16 platformID
//	    uint16 encodingID
//	    uint16 languageID
//	    uint16 nameID
//	    uint16 length
//	    Offset16 stringOffset (relative to stringOffset above)
func parseNameTable(data []byte) (family, fullName string) {
	if len(data) < 6 {
		return "", ""
	}

	count := int(binary.BigEndian.Uint16(data[2:4]))
	storageOffset := int(binary.BigEndian.Uint16(data[4:6]))

	if len(data) < 6+count*12 {
		return "", ""
	}

	// Search for family name and full name.
	// Priority: Windows Unicode (3,1) > Unicode (0,*) > Mac Roman (1,0).
	type nameCandidate struct {
		value    string
		priority int
	}

	var familyCand, fullCand nameCandidate

	for i := range count {
		recOff := 6 + i*12
		platformID := binary.BigEndian.Uint16(data[recOff : recOff+2])
		encodingID := binary.BigEndian.Uint16(data[recOff+2 : recOff+4])
		// languageID at recOff+4 — we prefer English (0x0409 for Windows,
		// 0 for Mac/Unicode) but accept any if English isn't found.
		languageID := binary.BigEndian.Uint16(data[recOff+4 : recOff+6])
		nameID := binary.BigEndian.Uint16(data[recOff+6 : recOff+8])
		length := int(binary.BigEndian.Uint16(data[recOff+8 : recOff+10]))
		strOffset := int(binary.BigEndian.Uint16(data[recOff+10 : recOff+12]))

		// Only interested in family (1) and full name (4).
		if nameID != nameIDFamily && nameID != nameIDFullName {
			continue
		}

		// Calculate absolute offset within data.
		absOffset := storageOffset + strOffset
		if absOffset+length > len(data) {
			continue
		}

		strData := data[absOffset : absOffset+length]
		value, pri := decodeNameString(strData, platformID, encodingID, languageID)
		if value == "" {
			continue
		}

		cand := &familyCand
		if nameID == nameIDFullName {
			cand = &fullCand
		}

		if pri > cand.priority {
			cand.value = value
			cand.priority = pri
		}
	}

	return familyCand.value, fullCand.value
}

// decodeNameString decodes a name table string and returns its value plus
// a priority score (higher = better). Returns empty string if decoding fails.
func decodeNameString(data []byte, platformID, encodingID, languageID uint16) (string, int) {
	priority := 0

	switch platformID {
	case 3: // Windows
		if encodingID == 1 || encodingID == 10 {
			// Windows Unicode BMP or full repertoire — UTF-16BE.
			s := decodeUTF16BE(data)
			if s == "" {
				return "", 0
			}
			priority = 10
			// Prefer English.
			if languageID == 0x0409 {
				priority = 20
			}
			return s, priority
		}

	case 0: // Unicode
		// Unicode platform — also UTF-16BE.
		s := decodeUTF16BE(data)
		if s == "" {
			return "", 0
		}
		priority = 5
		if languageID == 0 {
			priority = 8
		}
		return s, priority

	case 1: // Macintosh
		if encodingID == 0 {
			// Mac Roman — direct byte → rune for ASCII range.
			s := decodeMacRoman(data)
			if s == "" {
				return "", 0
			}
			priority = 1
			if languageID == 0 { // English
				priority = 3
			}
			return s, priority
		}
	}

	return "", 0
}

// decodeUTF16BE decodes a UTF-16BE byte slice to a Go string.
func decodeUTF16BE(data []byte) string {
	if len(data) < 2 || len(data)%2 != 0 {
		return ""
	}

	u16 := make([]uint16, len(data)/2)
	for i := range u16 {
		u16[i] = binary.BigEndian.Uint16(data[i*2 : i*2+2])
	}

	runes := utf16.Decode(u16)
	return string(runes)
}

// decodeMacRoman decodes Mac Roman encoded bytes to a Go string.
// For simplicity, only handles the ASCII subset (0x20-0x7E) which covers
// most Western font names. Non-ASCII bytes are replaced with '?'.
func decodeMacRoman(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	runes := make([]rune, len(data))
	for i, b := range data {
		if b >= 0x20 && b <= 0x7E {
			runes[i] = rune(b)
		} else {
			runes[i] = '?'
		}
	}
	return string(runes)
}
