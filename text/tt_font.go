// TrueType bytecode interpreter — font program data.
//
// Port of skrifa glyf/mod.rs Outlines struct (font-level fields).
// Loaded once per font file: fpgm, prep, CVT, and maxp limits.
// These are immutable after loading and shared across all sizes.
//
// Reference: skrifa/src/outline/glyf/mod.rs:38-58 (Outlines struct)
package text

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ttFontProgram holds TrueType bytecode program data loaded once per font.
// This corresponds to the font-level fields of skrifa's Outlines struct.
//
// Reference: skrifa glyf/mod.rs:38-58
type ttFontProgram struct {
	// fpgm is the font program bytecode (fpgm table).
	// Executed once when the font is first used at a given size.
	fpgm []byte

	// prep is the control value program bytecode (prep table).
	// Executed once per size, after fpgm.
	prep []byte

	// cvt contains raw CVT values in font units (from cvt table).
	// These are scaled per-size during HintInstance setup.
	cvt []int32

	// maxStack is the maximum stack depth (from maxp.maxStackElements).
	maxStack int

	// maxStorage is the storage area size (from maxp.maxStorage).
	maxStorage int

	// maxTwilight is the number of twilight zone points (from maxp.maxTwilightPoints).
	maxTwilight int

	// maxFunctionDefs is the max function definitions (from maxp.maxFunctionDefs).
	maxFunctionDefs int

	// maxInstructionDefs is the max instruction definitions (from maxp.maxInstructionDefs).
	maxInstructionDefs int

	// unitsPerEm is the font's design units per em (from head.unitsPerEm).
	unitsPerEm int

	// numGlyphs is the total number of glyphs (from maxp.numGlyphs).
	numGlyphs int

	// os2Ascender is sTypoAscender from the OS/2 table (font units).
	// Used for vertical phantom point computation.
	// Reference: skrifa glyf/mod.rs:55 (os2_vmetrics)
	os2Ascender int16

	// os2Descender is sTypoDescender from the OS/2 table (font units).
	// Typically negative.
	os2Descender int16
}

// loadTTFontProgram loads font-level TrueType bytecode data from raw font bytes.
// Parses fpgm, prep, cvt, maxp, head, and hmtx tables.
// Returns nil, nil if the font has no TrueType instructions (CFF fonts, etc.).
//
// Reference: skrifa glyf/mod.rs:60-110 (Outlines::new)
//
//nolint:nilnil // nil result = "no TrueType instructions", not an error
func loadTTFontProgram(fontData []byte) (*ttFontProgram, error) {
	tables, err := parseFontTables(fontData)
	if err != nil {
		return nil, fmt.Errorf("tt: font program: %w", err)
	}

	// Check if font has a glyf table (TrueType outlines).
	// CFF fonts don't have glyf and use a different hinting system.
	if _, ok := tables["glyf"]; !ok {
		return nil, nil
	}

	// Parse maxp table for limits.
	maxpData, ok := tables["maxp"]
	if !ok {
		return nil, fmt.Errorf("tt: font program: missing maxp table")
	}
	maxp, err := parseMaxpLimits(maxpData)
	if err != nil {
		return nil, fmt.Errorf("tt: font program: %w", err)
	}

	// Parse head table for unitsPerEm.
	headData, ok := tables["head"]
	if !ok {
		return nil, fmt.Errorf("tt: font program: missing head table")
	}
	upem, err := parseHeadUnitsPerEm(headData)
	if err != nil {
		return nil, fmt.Errorf("tt: font program: %w", err)
	}

	fp := &ttFontProgram{
		maxStack:           maxp.maxStackElements,
		maxStorage:         maxp.maxStorage,
		maxTwilight:        maxp.maxTwilightPoints,
		maxFunctionDefs:    maxp.maxFunctionDefs,
		maxInstructionDefs: maxp.maxInstructionDefs,
		unitsPerEm:         upem,
		numGlyphs:          maxp.numGlyphs,
	}

	// Parse OS/2 table for vertical metrics (sTypoAscender, sTypoDescender).
	// These are used for vertical phantom point computation.
	// Reference: skrifa glyf/mod.rs:101-103 (os2_vmetrics)
	if os2Data, ok := tables["OS/2"]; ok && len(os2Data) >= 72 {
		fp.os2Ascender = int16(binary.BigEndian.Uint16(os2Data[68:70]))
		fp.os2Descender = int16(binary.BigEndian.Uint16(os2Data[70:72]))
	}

	// Load optional tables (fpgm, prep, cvt may be absent).
	if fpgmData, ok := tables["fpgm"]; ok {
		fp.fpgm = fpgmData
	}
	if prepData, ok := tables["prep"]; ok {
		fp.prep = prepData
	}
	if cvtData, ok := tables["cvt "]; ok {
		fp.cvt = parseCVT(cvtData)
	}

	// Validate: if no instructions exist at all, return nil.
	if len(fp.fpgm) == 0 && len(fp.prep) == 0 {
		return nil, nil
	}

	return fp, nil
}

// hasFontProgram returns true if the font has a fpgm table.
func (fp *ttFontProgram) hasFontProgram() bool {
	return len(fp.fpgm) > 0
}

// hasPrepProgram returns true if the font has a prep table.
func (fp *ttFontProgram) hasPrepProgram() bool {
	return len(fp.prep) > 0
}

// maxpLimits holds the relevant fields from the maxp table.
type maxpLimits struct {
	numGlyphs          int
	maxTwilightPoints  int
	maxStorage         int
	maxFunctionDefs    int
	maxInstructionDefs int
	maxStackElements   int
}

// parseMaxpLimits extracts TT hinting limits from a raw maxp table.
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/maxp
func parseMaxpLimits(data []byte) (maxpLimits, error) {
	if len(data) < 6 {
		return maxpLimits{}, errors.New("maxp table too short")
	}
	r := bytes.NewReader(data)
	var version uint32
	if err := binary.Read(r, binary.BigEndian, &version); err != nil {
		return maxpLimits{}, fmt.Errorf("maxp: read version: %w", err)
	}

	var numGlyphs uint16
	if err := binary.Read(r, binary.BigEndian, &numGlyphs); err != nil {
		return maxpLimits{}, fmt.Errorf("maxp: read numGlyphs: %w", err)
	}

	ml := maxpLimits{numGlyphs: int(numGlyphs)}

	// Version 1.0 (0x00010000) has the full TrueType maxp fields.
	// Version 0.5 (0x00005000) is CFF-only and lacks these fields.
	if version == 0x00010000 && len(data) >= 32 {
		r = bytes.NewReader(data[6:])
		var fields struct {
			MaxPoints             uint16
			MaxContours           uint16
			MaxCompositePoints    uint16
			MaxCompositeContours  uint16
			MaxZones              uint16
			MaxTwilightPoints     uint16
			MaxStorage            uint16
			MaxFunctionDefs       uint16
			MaxInstructionDefs    uint16
			MaxStackElements      uint16
			MaxSizeOfInstructions uint16
		}
		if err := binary.Read(r, binary.BigEndian, &fields); err != nil {
			return maxpLimits{}, fmt.Errorf("maxp: read TT fields: %w", err)
		}
		ml.maxTwilightPoints = int(fields.MaxTwilightPoints)
		ml.maxStorage = int(fields.MaxStorage)
		ml.maxFunctionDefs = int(fields.MaxFunctionDefs)
		ml.maxInstructionDefs = int(fields.MaxInstructionDefs)
		ml.maxStackElements = int(fields.MaxStackElements)
	}

	// Ensure reasonable minimums.
	if ml.maxStackElements < 256 {
		ml.maxStackElements = 256
	}

	return ml, nil
}

// parseHeadUnitsPerEm extracts unitsPerEm from a raw head table.
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/head
func parseHeadUnitsPerEm(data []byte) (int, error) {
	// unitsPerEm is at offset 18 in the head table (uint16).
	if len(data) < 20 {
		return 0, errors.New("head table too short for unitsPerEm")
	}
	upem := int(binary.BigEndian.Uint16(data[18:20]))
	if upem == 0 {
		return 0, errors.New("head: unitsPerEm is zero")
	}
	return upem, nil
}

// parseCVT parses the cvt (Control Value Table) from raw bytes.
// Each entry is a big-endian int16 (FWord), stored as int32.
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/cvt
func parseCVT(data []byte) []int32 {
	count := len(data) / 2
	if count == 0 {
		return nil
	}
	cvt := make([]int32, count)
	for i := range count {
		cvt[i] = int32(int16(binary.BigEndian.Uint16(data[i*2 : i*2+2])))
	}
	return cvt
}

// parseFontTables parses the OpenType/TrueType table directory and returns
// a map of table tag to raw table data.
//
// Supports both standalone fonts (TTF/OTF) and the first font in a
// collection (TTC). This is a lightweight parser that only reads the
// table directory — individual tables are sliced from the raw data.
func parseFontTables(fontData []byte) (map[string][]byte, error) {
	if len(fontData) < 12 {
		return nil, errors.New("font data too short")
	}
	r := bytes.NewReader(fontData)

	// Check for TTC (tag "ttcf").
	var sfntVersion uint32
	if err := binary.Read(r, binary.BigEndian, &sfntVersion); err != nil {
		return nil, fmt.Errorf("read sfnt version: %w", err)
	}

	offset := int64(0)
	if sfntVersion == 0x74746366 { // "ttcf"
		// TTC header: skip version (4 bytes), read numFonts, take first offset.
		if len(fontData) < 16 {
			return nil, errors.New("TTC header too short")
		}
		// Skip to numFonts at offset 8.
		if _, err := r.Seek(8, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek to TTC numFonts: %w", err)
		}
		var numFonts uint32
		if err := binary.Read(r, binary.BigEndian, &numFonts); err != nil {
			return nil, fmt.Errorf("read TTC numFonts: %w", err)
		}
		if numFonts == 0 {
			return nil, errors.New("TTC has zero fonts")
		}
		var firstOffset uint32
		if err := binary.Read(r, binary.BigEndian, &firstOffset); err != nil {
			return nil, fmt.Errorf("read TTC first offset: %w", err)
		}
		offset = int64(firstOffset)
		if _, err := r.Seek(offset, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek to first font: %w", err)
		}
		// Re-read sfnt version at the font offset.
		if err := binary.Read(r, binary.BigEndian, &sfntVersion); err != nil {
			return nil, fmt.Errorf("read sfnt version at font: %w", err)
		}
	}

	// Read table count.
	var numTables uint16
	if err := binary.Read(r, binary.BigEndian, &numTables); err != nil {
		return nil, fmt.Errorf("read numTables: %w", err)
	}
	// Skip searchRange, entrySelector, rangeShift (6 bytes).
	if _, err := r.Seek(6, io.SeekCurrent); err != nil {
		return nil, fmt.Errorf("skip sfnt header fields: %w", err)
	}

	tables := make(map[string][]byte, numTables)
	for range int(numTables) {
		var tag [4]byte
		if _, err := io.ReadFull(r, tag[:]); err != nil {
			return nil, fmt.Errorf("read table tag: %w", err)
		}
		var checksum, tableOffset, length uint32
		if err := binary.Read(r, binary.BigEndian, &checksum); err != nil {
			return nil, fmt.Errorf("read table checksum: %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &tableOffset); err != nil {
			return nil, fmt.Errorf("read table offset: %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			return nil, fmt.Errorf("read table length: %w", err)
		}
		end := int64(tableOffset) + int64(length)
		if end > int64(len(fontData)) {
			continue // Skip tables that extend beyond font data.
		}
		tables[string(tag[:])] = fontData[tableOffset:end]
	}
	return tables, nil
}
