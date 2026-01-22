package emoji

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestStrikeStrategy_String(t *testing.T) {
	tests := []struct {
		strategy StrikeStrategy
		want     string
	}{
		{StrikeBestFit, "BestFit"},
		{StrikeExact, "Exact"},
		{StrikeLargest, "Largest"},
		{StrikeStrategy(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.strategy.String()
			if got != tt.want {
				t.Errorf("StrikeStrategy(%d).String() = %q, want %q", tt.strategy, got, tt.want)
			}
		})
	}
}

func TestNewCBDTExtractor_NoData(t *testing.T) {
	tests := []struct {
		name     string
		cbdtData []byte
		cblcData []byte
		wantErr  error
	}{
		{
			name:     "no CBDT",
			cbdtData: nil,
			cblcData: makeMockCBLC(0),
			wantErr:  ErrNoCBDTTable,
		},
		{
			name:     "no CBLC",
			cbdtData: []byte{0, 3, 0, 0}, // CBDT header
			cblcData: nil,
			wantErr:  ErrNoCBLCTable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCBDTExtractor(tt.cbdtData, tt.cblcData)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("NewCBDTExtractor() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewCBDTExtractor_InvalidCBLC(t *testing.T) {
	tests := []struct {
		name     string
		cblcData []byte
	}{
		{
			name:     "too short",
			cblcData: []byte{0, 3, 0, 0},
		},
		{
			name:     "wrong version",
			cblcData: makeCBLCHeader(1, 0, 0), // Version 1.0 instead of 3.0
		},
	}

	cbdtData := []byte{0, 3, 0, 0} // Valid CBDT header.

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCBDTExtractor(cbdtData, tt.cblcData)
			if err == nil {
				t.Error("NewCBDTExtractor() expected error for invalid CBLC")
			}
		})
	}
}

func TestCBDTExtractor_NumStrikes(t *testing.T) {
	cblcData := makeMockCBLC(3)
	cbdtData := []byte{0, 3, 0, 0}

	e, err := NewCBDTExtractor(cbdtData, cblcData)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	if got := e.NumStrikes(); got != 3 {
		t.Errorf("NumStrikes() = %d, want 3", got)
	}
}

func TestCBDTExtractor_StrikePPEM(t *testing.T) {
	cblcData := makeMockCBLCWithPPEM([]uint8{16, 32, 64})
	cbdtData := []byte{0, 3, 0, 0}

	e, err := NewCBDTExtractor(cbdtData, cblcData)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	tests := []struct {
		index int
		want  uint16
	}{
		{0, 16},
		{1, 32},
		{2, 64},
		{-1, 0}, // Out of range
		{3, 0},  // Out of range
	}

	for _, tt := range tests {
		got := e.StrikePPEM(tt.index)
		if got != tt.want {
			t.Errorf("StrikePPEM(%d) = %d, want %d", tt.index, got, tt.want)
		}
	}
}

func TestCBDTExtractor_SelectStrike(t *testing.T) {
	cblcData := makeMockCBLCWithPPEM([]uint8{16, 32, 64, 128})
	cbdtData := []byte{0, 3, 0, 0}

	e, err := NewCBDTExtractor(cbdtData, cblcData)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	tests := []struct {
		name     string
		ppem     uint16
		strategy StrikeStrategy
		want     int
	}{
		// BestFit: smallest >= requested, or largest if none
		{"BestFit exact match", 32, StrikeBestFit, 1},
		{"BestFit round up", 20, StrikeBestFit, 1}, // 32 >= 20
		{"BestFit largest", 200, StrikeBestFit, 3}, // 128 is largest
		{"BestFit smallest", 8, StrikeBestFit, 0},  // 16 >= 8

		// Exact: only exact matches
		{"Exact match", 32, StrikeExact, 1},
		{"Exact no match", 24, StrikeExact, -1},
		{"Exact boundary", 64, StrikeExact, 2},

		// Largest: always largest
		{"Largest always", 16, StrikeLargest, 3},
		{"Largest any ppem", 0, StrikeLargest, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.SelectStrike(tt.ppem, tt.strategy)
			if got != tt.want {
				t.Errorf("SelectStrike(%d, %v) = %d, want %d", tt.ppem, tt.strategy, got, tt.want)
			}
		})
	}
}

func TestCBDTExtractor_SelectStrike_Empty(t *testing.T) {
	cblcData := makeMockCBLC(0)
	cbdtData := []byte{0, 3, 0, 0}

	e, err := NewCBDTExtractor(cbdtData, cblcData)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	got := e.SelectStrike(32, StrikeBestFit)
	if got != -1 {
		t.Errorf("SelectStrike() = %d, want -1 for empty strikes", got)
	}
}

func TestCBDTExtractor_AvailablePPEMs(t *testing.T) {
	cblcData := makeMockCBLCWithPPEM([]uint8{16, 32, 64})
	cbdtData := []byte{0, 3, 0, 0}

	e, err := NewCBDTExtractor(cbdtData, cblcData)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	ppems := e.AvailablePPEMs()
	if len(ppems) != 3 {
		t.Fatalf("AvailablePPEMs() len = %d, want 3", len(ppems))
	}

	want := []uint16{16, 32, 64}
	for i, p := range ppems {
		if p != want[i] {
			t.Errorf("AvailablePPEMs()[%d] = %d, want %d", i, p, want[i])
		}
	}
}

func TestCBDTExtractor_GetGlyph_NoStrike(t *testing.T) {
	cblcData := makeMockCBLC(0)
	cbdtData := []byte{0, 3, 0, 0}

	e, err := NewCBDTExtractor(cbdtData, cblcData)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	_, err = e.GetGlyph(100, 32)
	if !errors.Is(err, ErrNoStrikeAvailable) {
		t.Errorf("GetGlyph() error = %v, want ErrNoStrikeAvailable", err)
	}
}

func TestCBDTExtractor_GetGlyphAtStrike_OutOfRange(t *testing.T) {
	cblcData := makeMockCBLC(1)
	cbdtData := []byte{0, 3, 0, 0}

	e, err := NewCBDTExtractor(cbdtData, cblcData)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	_, err = e.GetGlyphAtStrike(100, -1)
	if !errors.Is(err, ErrNoStrikeAvailable) {
		t.Errorf("GetGlyphAtStrike() error = %v, want ErrNoStrikeAvailable", err)
	}

	_, err = e.GetGlyphAtStrike(100, 5)
	if !errors.Is(err, ErrNoStrikeAvailable) {
		t.Errorf("GetGlyphAtStrike() error = %v, want ErrNoStrikeAvailable", err)
	}
}

func TestParseBigGlyphMetrics(t *testing.T) {
	data := []byte{
		10,   // height
		12,   // width
		0xFE, // horiBearingX (-2)
		5,    // horiBearingY
		14,   // horiAdvance
		0xFF, // vertBearingX (-1)
		3,    // vertBearingY
		10,   // vertAdvance
	}

	var m bigGlyphMetrics
	parseBigGlyphMetrics(data, &m)

	if m.height != 10 {
		t.Errorf("height = %d, want 10", m.height)
	}
	if m.width != 12 {
		t.Errorf("width = %d, want 12", m.width)
	}
	if m.horiBearingX != -2 {
		t.Errorf("horiBearingX = %d, want -2", m.horiBearingX)
	}
	if m.horiBearingY != 5 {
		t.Errorf("horiBearingY = %d, want 5", m.horiBearingY)
	}
	if m.horiAdvance != 14 {
		t.Errorf("horiAdvance = %d, want 14", m.horiAdvance)
	}
}

func TestParseSmallGlyphMetrics(t *testing.T) {
	data := []byte{
		8,    // height
		10,   // width
		0xFD, // bearingX (-3)
		6,    // bearingY
		12,   // advance
	}

	var m smallGlyphMetrics
	parseSmallGlyphMetrics(data, &m)

	if m.height != 8 {
		t.Errorf("height = %d, want 8", m.height)
	}
	if m.width != 10 {
		t.Errorf("width = %d, want 10", m.width)
	}
	if m.bearingX != -3 {
		t.Errorf("bearingX = %d, want -3", m.bearingX)
	}
	if m.bearingY != 6 {
		t.Errorf("bearingY = %d, want 6", m.bearingY)
	}
	if m.advance != 12 {
		t.Errorf("advance = %d, want 12", m.advance)
	}
}

func TestCBDTExtractor_StrikeBitDepth(t *testing.T) {
	cblcData := makeMockCBLCWithPPEM([]uint8{32})
	cbdtData := []byte{0, 3, 0, 0}

	e, err := NewCBDTExtractor(cbdtData, cblcData)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	// Our mock sets bitDepth to 32.
	if got := e.StrikeBitDepth(0); got != 32 {
		t.Errorf("StrikeBitDepth(0) = %d, want 32", got)
	}

	if got := e.StrikeBitDepth(-1); got != 0 {
		t.Errorf("StrikeBitDepth(-1) = %d, want 0", got)
	}
}

// Helper functions to create mock CBLC data.

func makeCBLCHeader(majorVersion, minorVersion uint16, numSizes uint32) []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint16(data[0:2], majorVersion)
	binary.BigEndian.PutUint16(data[2:4], minorVersion)
	binary.BigEndian.PutUint32(data[4:8], numSizes)
	return data
}

func makeMockCBLC(numStrikes int) []byte {
	return makeMockCBLCWithPPEM(make([]uint8, numStrikes))
}

func makeMockCBLCWithPPEM(ppems []uint8) []byte {
	numStrikes := len(ppems)
	const bitmapSizeRecordSize = 48

	// Header (8 bytes) + BitmapSize records (48 bytes each).
	totalSize := 8 + numStrikes*bitmapSizeRecordSize
	data := make([]byte, totalSize)

	// Header.
	binary.BigEndian.PutUint16(data[0:2], 3) // majorVersion
	binary.BigEndian.PutUint16(data[2:4], 0) // minorVersion
	binary.BigEndian.PutUint32(data[4:8], uint32(numStrikes))

	// BitmapSize records.
	for i := 0; i < numStrikes; i++ {
		offset := 8 + i*bitmapSizeRecordSize

		// indexSubtableListOffset - point past all bitmap size records.
		binary.BigEndian.PutUint32(data[offset:offset+4], uint32(totalSize))

		// indexSubtableListSize
		binary.BigEndian.PutUint32(data[offset+4:offset+8], 0)

		// numberOfIndexSubtables
		binary.BigEndian.PutUint32(data[offset+8:offset+12], 0)

		// colorRef (unused)
		binary.BigEndian.PutUint32(data[offset+12:offset+16], 0)

		// horiMetrics (12 bytes) - zeros are fine.
		// vertMetrics (12 bytes) - zeros are fine.

		// startGlyphIndex
		binary.BigEndian.PutUint16(data[offset+40:offset+42], 0)

		// endGlyphIndex
		binary.BigEndian.PutUint16(data[offset+42:offset+44], 0)

		// ppemX
		if i < len(ppems) {
			data[offset+44] = ppems[i]
		}

		// ppemY
		if i < len(ppems) {
			data[offset+45] = ppems[i]
		}

		// bitDepth (32 for color)
		data[offset+46] = 32

		// flags
		data[offset+47] = 0x01 // Horizontal
	}

	return data
}

// Test with a more complete CBLC/CBDT structure for format 17.
func TestCBDTExtractor_Format17_Integration(t *testing.T) {
	// Create a minimal but complete CBLC/CBDT pair.
	cblc, cbdt := makeTestCBLCCBDTFormat17()

	e, err := NewCBDTExtractor(cbdt, cblc)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	// Check glyph availability.
	if !e.HasGlyph(100) {
		t.Error("HasGlyph(100) = false, want true")
	}

	if e.HasGlyph(200) {
		t.Error("HasGlyph(200) = true, want false")
	}

	// Extract glyph.
	glyph, err := e.GetGlyph(100, 32)
	if err != nil {
		t.Fatalf("GetGlyph(100, 32) error = %v", err)
	}

	if glyph.GlyphID != 100 {
		t.Errorf("glyph.GlyphID = %d, want 100", glyph.GlyphID)
	}

	if glyph.Format != FormatPNG {
		t.Errorf("glyph.Format = %v, want FormatPNG", glyph.Format)
	}

	if glyph.Width != 16 || glyph.Height != 16 {
		t.Errorf("glyph size = %dx%d, want 16x16", glyph.Width, glyph.Height)
	}

	// Check PNG data starts with PNG signature.
	if len(glyph.Data) < 8 {
		t.Fatalf("glyph.Data too short: %d bytes", len(glyph.Data))
	}

	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i, b := range pngSig {
		if glyph.Data[i] != b {
			t.Errorf("glyph.Data[%d] = 0x%02X, want 0x%02X", i, glyph.Data[i], b)
		}
	}
}

// makeTestCBLCCBDTFormat17 creates a test CBLC/CBDT pair with format 1/17.
func makeTestCBLCCBDTFormat17() (cblc, cbdt []byte) {
	// PNG signature for test data.
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, // IHDR length
		0x49, 0x48, 0x44, 0x52, // "IHDR"
		0x00, 0x00, 0x00, 0x10, // width = 16
		0x00, 0x00, 0x00, 0x10, // height = 16
		0x08, 0x06, 0x00, 0x00, 0x00, // bit depth, color type, etc.
		0x1F, 0xF3, 0xFF, 0x61, // CRC
	}

	// Build CBDT first.
	// CBDT header (4 bytes) + glyph data for format 17.
	// Format 17: SmallGlyphMetrics (5) + dataLen (4) + PNG data.
	cbdtHeader := make([]byte, 4)
	binary.BigEndian.PutUint16(cbdtHeader[0:2], 3) // majorVersion
	binary.BigEndian.PutUint16(cbdtHeader[2:4], 0) // minorVersion

	glyphDataOffset := uint32(4) // After CBDT header.

	// SmallGlyphMetrics.
	smallMetrics := []byte{
		16, // height
		16, // width
		0,  // bearingX
		16, // bearingY
		18, // advance
	}

	// dataLen.
	dataLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(dataLenBytes, uint32(len(pngData)))

	// Combine CBDT.
	cbdt = cbdtHeader
	cbdt = append(cbdt, smallMetrics...)
	cbdt = append(cbdt, dataLenBytes...)
	cbdt = append(cbdt, pngData...)

	glyphDataSize := uint32(5 + 4 + len(pngData))

	// Build CBLC.
	// CBLC header (8 bytes) + BitmapSize record (48 bytes) + IndexSubtableArray (8 bytes) + IndexSubtable format 1.

	const bitmapSizeRecordSize = 48
	indexSubtableListOffset := uint32(8 + bitmapSizeRecordSize)

	// IndexSubtableArray entry (8 bytes).
	indexSubtableArrayEntry := make([]byte, 8)
	binary.BigEndian.PutUint16(indexSubtableArrayEntry[0:2], 100) // firstGlyphIndex
	binary.BigEndian.PutUint16(indexSubtableArrayEntry[2:4], 100) // lastGlyphIndex
	binary.BigEndian.PutUint32(indexSubtableArrayEntry[4:8], 8)   // offset from indexSubtableList

	// IndexSubtable format 1 header (8 bytes) + 2 offsets (8 bytes).
	indexSubtableFormat1 := make([]byte, 16)
	binary.BigEndian.PutUint16(indexSubtableFormat1[0:2], 1)               // indexFormat
	binary.BigEndian.PutUint16(indexSubtableFormat1[2:4], 17)              // imageFormat
	binary.BigEndian.PutUint32(indexSubtableFormat1[4:8], glyphDataOffset) // imageDataOffset
	binary.BigEndian.PutUint32(indexSubtableFormat1[8:12], 0)              // offset[0]
	binary.BigEndian.PutUint32(indexSubtableFormat1[12:16], glyphDataSize) // offset[1]

	indexSubtableListSize := uint32(len(indexSubtableArrayEntry) + len(indexSubtableFormat1))

	// BitmapSize record.
	bitmapSizeRecord := make([]byte, bitmapSizeRecordSize)
	binary.BigEndian.PutUint32(bitmapSizeRecord[0:4], indexSubtableListOffset)
	binary.BigEndian.PutUint32(bitmapSizeRecord[4:8], indexSubtableListSize)
	binary.BigEndian.PutUint32(bitmapSizeRecord[8:12], 1)  // numberOfIndexSubtables
	binary.BigEndian.PutUint32(bitmapSizeRecord[12:16], 0) // colorRef (unused)
	// horiMetrics and vertMetrics (24 bytes) - zeros
	binary.BigEndian.PutUint16(bitmapSizeRecord[40:42], 100) // startGlyphIndex
	binary.BigEndian.PutUint16(bitmapSizeRecord[42:44], 100) // endGlyphIndex
	bitmapSizeRecord[44] = 32                                // ppemX
	bitmapSizeRecord[45] = 32                                // ppemY
	bitmapSizeRecord[46] = 32                                // bitDepth
	bitmapSizeRecord[47] = 0x01                              // flags

	// CBLC header.
	cblcHeader := make([]byte, 8)
	binary.BigEndian.PutUint16(cblcHeader[0:2], 3) // majorVersion
	binary.BigEndian.PutUint16(cblcHeader[2:4], 0) // minorVersion
	binary.BigEndian.PutUint32(cblcHeader[4:8], 1) // numSizes

	// Combine CBLC.
	cblc = cblcHeader
	cblc = append(cblc, bitmapSizeRecord...)
	cblc = append(cblc, indexSubtableArrayEntry...)
	cblc = append(cblc, indexSubtableFormat1...)

	return cblc, cbdt
}

func TestCBDTExtractor_HasGlyphInStrike(t *testing.T) {
	cblc, cbdt := makeTestCBLCCBDTFormat17()

	e, err := NewCBDTExtractor(cbdt, cblc)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	if !e.HasGlyphInStrike(100, 0) {
		t.Error("HasGlyphInStrike(100, 0) = false, want true")
	}

	if e.HasGlyphInStrike(100, -1) {
		t.Error("HasGlyphInStrike(100, -1) = true, want false")
	}

	if e.HasGlyphInStrike(100, 5) {
		t.Error("HasGlyphInStrike(100, 5) = true, want false")
	}

	if e.HasGlyphInStrike(50, 0) {
		t.Error("HasGlyphInStrike(50, 0) = true, want false")
	}
}

func TestCBDTExtractor_GetGlyphWithStrategy(t *testing.T) {
	cblc, cbdt := makeTestCBLCCBDTFormat17()

	e, err := NewCBDTExtractor(cbdt, cblc)
	if err != nil {
		t.Fatalf("NewCBDTExtractor() error = %v", err)
	}

	// Test with exact match.
	glyph, err := e.GetGlyphWithStrategy(100, 32, StrikeExact)
	if err != nil {
		t.Fatalf("GetGlyphWithStrategy() error = %v", err)
	}

	if glyph.GlyphID != 100 {
		t.Errorf("glyph.GlyphID = %d, want 100", glyph.GlyphID)
	}

	// Test with no exact match.
	_, err = e.GetGlyphWithStrategy(100, 64, StrikeExact)
	if !errors.Is(err, ErrNoStrikeAvailable) {
		t.Errorf("GetGlyphWithStrategy() error = %v, want ErrNoStrikeAvailable", err)
	}
}
