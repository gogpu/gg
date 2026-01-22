package emoji

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// CBDT/CBLC table format errors.
var (
	// ErrNoCBLCTable indicates the font doesn't have a CBLC table.
	ErrNoCBLCTable = errors.New("emoji: font has no CBLC table")

	// ErrInvalidCBLCData indicates the CBLC table data is malformed.
	ErrInvalidCBLCData = errors.New("emoji: invalid CBLC table data")

	// ErrUnsupportedIndexFormat indicates an unsupported index subtable format.
	ErrUnsupportedIndexFormat = errors.New("emoji: unsupported index subtable format")

	// ErrUnsupportedImageFormat indicates an unsupported image format.
	ErrUnsupportedImageFormat = errors.New("emoji: unsupported image format")

	// ErrNoStrikeAvailable indicates no bitmap strike is available.
	ErrNoStrikeAvailable = errors.New("emoji: no bitmap strike available")
)

// CBLC/CBDT table versions.
const (
	cblcMajorVersion = 3
	cbdtMajorVersion = 3
)

// Index subtable formats.
const (
	indexFormat1 = 1 // Variable metrics, 32-bit offsets
	indexFormat2 = 2 // Constant metrics, no offset array
	indexFormat3 = 3 // Variable metrics, 16-bit offsets
	indexFormat4 = 4 // Variable metrics, sparse glyph IDs
	indexFormat5 = 5 // Constant metrics, sparse glyph IDs
)

// Image data formats (in CBDT).
const (
	imageFormat17 = 17 // Small metrics + PNG
	imageFormat18 = 18 // Big metrics + PNG
	imageFormat19 = 19 // Metrics in CBLC, PNG data only
)

// StrikeStrategy determines how to select a bitmap strike.
type StrikeStrategy int

const (
	// StrikeBestFit selects the smallest strike >= requested size, or largest if none.
	StrikeBestFit StrikeStrategy = iota

	// StrikeExact selects only an exact match.
	StrikeExact

	// StrikeLargest always selects the largest available strike.
	StrikeLargest
)

// String returns the string representation of the strike strategy.
func (s StrikeStrategy) String() string {
	switch s {
	case StrikeBestFit:
		return "BestFit"
	case StrikeExact:
		return "Exact"
	case StrikeLargest:
		return "Largest"
	default:
		return unknownStr
	}
}

// CBDTExtractor extracts bitmap glyphs from CBDT/CBLC tables.
// CBDT (Color Bitmap Data Table) stores the bitmap data.
// CBLC (Color Bitmap Location Table) stores the index/location data.
type CBDTExtractor struct {
	cbdtData []byte
	cblcData []byte

	// Parsed CBLC data.
	majorVersion uint16
	minorVersion uint16
	strikes      []bitmapStrike
}

// bitmapStrike represents one bitmap size entry from CBLC.
type bitmapStrike struct {
	// Location info.
	indexSubtableListOffset uint32
	indexSubtableListSize   uint32
	numberOfIndexSubtables  uint32

	// Glyph range.
	startGlyphIndex uint16
	endGlyphIndex   uint16

	// Size info.
	ppemX    uint8
	ppemY    uint8
	bitDepth uint8
	flags    int8

	// Line metrics.
	horiMetrics sbitLineMetrics
	vertMetrics sbitLineMetrics

	// Parsed index subtables (lazily populated).
	indexSubtables []indexSubtable
}

// sbitLineMetrics holds line metrics for a strike.
type sbitLineMetrics struct {
	ascender              int8
	descender             int8
	widthMax              uint8
	caretSlopeNumerator   int8
	caretSlopeDenominator int8
	caretOffset           int8
	minOriginSB           int8
	minAdvanceSB          int8
	maxBeforeBL           int8
	minAfterBL            int8
	pad1, pad2            int8
}

// indexSubtable represents a parsed index subtable.
type indexSubtable struct {
	firstGlyphIndex uint16
	lastGlyphIndex  uint16
	indexFormat     uint16
	imageFormat     uint16
	imageDataOffset uint32

	// Format-specific data.
	offsets32    []uint32        // Format 1
	offsets16    []uint16        // Format 3
	imageSize    uint32          // Format 2, 5
	bigMetrics   *bigGlyphMetrics // Format 2, 5
	glyphPairs   []glyphIDOffsetPair // Format 4
	glyphIDs     []uint16        // Format 5
}

// glyphIDOffsetPair for format 4.
type glyphIDOffsetPair struct {
	glyphID    uint16
	sbitOffset uint16
}

// smallGlyphMetrics holds metrics for small metrics format (5 bytes).
type smallGlyphMetrics struct {
	height   uint8
	width    uint8
	bearingX int8
	bearingY int8
	advance  uint8
}

// bigGlyphMetrics holds metrics for both horizontal and vertical layouts (8 bytes).
type bigGlyphMetrics struct {
	height       uint8
	width        uint8
	horiBearingX int8
	horiBearingY int8
	horiAdvance  uint8
	vertBearingX int8
	vertBearingY int8
	vertAdvance  uint8
}

// NewCBDTExtractor creates a new CBDT/CBLC extractor.
// cbdtData is the raw CBDT table, cblcData is the raw CBLC table.
func NewCBDTExtractor(cbdtData, cblcData []byte) (*CBDTExtractor, error) {
	if len(cbdtData) == 0 {
		return nil, ErrNoCBDTTable
	}
	if len(cblcData) == 0 {
		return nil, ErrNoCBLCTable
	}

	e := &CBDTExtractor{
		cbdtData: cbdtData,
		cblcData: cblcData,
	}

	if err := e.parseCBLC(); err != nil {
		return nil, err
	}

	return e, nil
}

// parseCBLC parses the CBLC table header and bitmap size records.
func (e *CBDTExtractor) parseCBLC() error {
	data := e.cblcData
	if len(data) < 8 {
		return ErrInvalidCBLCData
	}

	e.majorVersion = binary.BigEndian.Uint16(data[0:2])
	e.minorVersion = binary.BigEndian.Uint16(data[2:4])

	if e.majorVersion != cblcMajorVersion {
		return fmt.Errorf("emoji: unsupported CBLC version %d.%d", e.majorVersion, e.minorVersion)
	}

	numSizes := binary.BigEndian.Uint32(data[4:8])

	// Parse BitmapSize records.
	// Each BitmapSize record is 48 bytes.
	const bitmapSizeRecordSize = 48
	recordsOffset := 8

	if recordsOffset+int(numSizes)*bitmapSizeRecordSize > len(data) {
		return ErrInvalidCBLCData
	}

	e.strikes = make([]bitmapStrike, numSizes)
	for i := uint32(0); i < numSizes; i++ {
		offset := recordsOffset + int(i)*bitmapSizeRecordSize
		if err := e.parseBitmapSizeRecord(data[offset:offset+bitmapSizeRecordSize], &e.strikes[i]); err != nil {
			return err
		}
	}

	return nil
}

// parseBitmapSizeRecord parses a single BitmapSize record.
func (e *CBDTExtractor) parseBitmapSizeRecord(data []byte, strike *bitmapStrike) error {
	if len(data) < 48 {
		return ErrInvalidCBLCData
	}

	strike.indexSubtableListOffset = binary.BigEndian.Uint32(data[0:4])
	strike.indexSubtableListSize = binary.BigEndian.Uint32(data[4:8])
	strike.numberOfIndexSubtables = binary.BigEndian.Uint32(data[8:12])
	// colorRef at offset 12-16 is unused.

	// Parse horizontal line metrics (12 bytes at offset 16).
	parseSbitLineMetrics(data[16:28], &strike.horiMetrics)

	// Parse vertical line metrics (12 bytes at offset 28).
	parseSbitLineMetrics(data[28:40], &strike.vertMetrics)

	strike.startGlyphIndex = binary.BigEndian.Uint16(data[40:42])
	strike.endGlyphIndex = binary.BigEndian.Uint16(data[42:44])
	strike.ppemX = data[44]
	strike.ppemY = data[45]
	strike.bitDepth = data[46]
	strike.flags = int8(data[47])

	return nil
}

// parseSbitLineMetrics parses a SbitLineMetrics record.
func parseSbitLineMetrics(data []byte, m *sbitLineMetrics) {
	m.ascender = int8(data[0])
	m.descender = int8(data[1])
	m.widthMax = data[2]
	m.caretSlopeNumerator = int8(data[3])
	m.caretSlopeDenominator = int8(data[4])
	m.caretOffset = int8(data[5])
	m.minOriginSB = int8(data[6])
	m.minAdvanceSB = int8(data[7])
	m.maxBeforeBL = int8(data[8])
	m.minAfterBL = int8(data[9])
	m.pad1 = int8(data[10])
	m.pad2 = int8(data[11])
}

// NumStrikes returns the number of available bitmap strikes.
func (e *CBDTExtractor) NumStrikes() int {
	return len(e.strikes)
}

// StrikePPEM returns the PPEM for a strike at the given index.
// Returns 0 if the index is out of range.
func (e *CBDTExtractor) StrikePPEM(index int) uint16 {
	if index < 0 || index >= len(e.strikes) {
		return 0
	}
	return uint16(e.strikes[index].ppemX)
}

// StrikeBitDepth returns the bit depth for a strike at the given index.
// For color emoji, this is typically 32 (BGRA).
func (e *CBDTExtractor) StrikeBitDepth(index int) uint8 {
	if index < 0 || index >= len(e.strikes) {
		return 0
	}
	return e.strikes[index].bitDepth
}

// SelectStrike selects a bitmap strike based on the requested PPEM and strategy.
// Returns the strike index, or -1 if no suitable strike is found.
func (e *CBDTExtractor) SelectStrike(ppem uint16, strategy StrikeStrategy) int {
	if len(e.strikes) == 0 {
		return -1
	}

	switch strategy {
	case StrikeExact:
		for i := range e.strikes {
			if uint16(e.strikes[i].ppemX) == ppem {
				return i
			}
		}
		return -1

	case StrikeLargest:
		best := 0
		for i := 1; i < len(e.strikes); i++ {
			if e.strikes[i].ppemX > e.strikes[best].ppemX {
				best = i
			}
		}
		return best

	default:
		// Find smallest strike >= requested, or largest if none.
		bestLarger := -1
		bestLargerPPEM := uint8(255)

		largest := 0
		largestPPEM := e.strikes[0].ppemX

		// Clamp ppem to uint8 range for comparison.
		// #nosec G115 -- ppem clamped to 255 max, no overflow
		ppemClamped := uint8(min(int(ppem), 255))

		for i := range e.strikes {
			strikePPEM := e.strikes[i].ppemX

			// Track largest overall.
			if strikePPEM > largestPPEM {
				largest = i
				largestPPEM = strikePPEM
			}

			// Track smallest >= requested.
			if strikePPEM >= ppemClamped && strikePPEM < bestLargerPPEM {
				bestLarger = i
				bestLargerPPEM = strikePPEM
			}
		}

		if bestLarger >= 0 {
			return bestLarger
		}
		return largest
	}
}

// HasGlyph returns true if the glyph has bitmap data at any strike.
func (e *CBDTExtractor) HasGlyph(glyphID uint16) bool {
	for i := range e.strikes {
		if e.hasGlyphInStrike(glyphID, i) {
			return true
		}
	}
	return false
}

// HasGlyphInStrike returns true if the glyph has bitmap data at the given strike.
func (e *CBDTExtractor) HasGlyphInStrike(glyphID uint16, strikeIndex int) bool {
	if strikeIndex < 0 || strikeIndex >= len(e.strikes) {
		return false
	}
	return e.hasGlyphInStrike(glyphID, strikeIndex)
}

// hasGlyphInStrike is the internal implementation.
func (e *CBDTExtractor) hasGlyphInStrike(glyphID uint16, strikeIndex int) bool {
	strike := &e.strikes[strikeIndex]

	// Quick range check.
	if glyphID < strike.startGlyphIndex || glyphID > strike.endGlyphIndex {
		return false
	}

	// Ensure index subtables are parsed.
	if err := e.parseIndexSubtables(strikeIndex); err != nil {
		return false
	}

	// Search index subtables.
	for i := range strike.indexSubtables {
		ist := &strike.indexSubtables[i]
		if glyphID >= ist.firstGlyphIndex && glyphID <= ist.lastGlyphIndex {
			return true
		}
	}

	return false
}

// parseIndexSubtables parses the index subtables for a strike (lazily).
func (e *CBDTExtractor) parseIndexSubtables(strikeIndex int) error {
	strike := &e.strikes[strikeIndex]

	// Already parsed?
	if strike.indexSubtables != nil {
		return nil
	}

	data := e.cblcData
	listOffset := int(strike.indexSubtableListOffset)

	if listOffset+int(strike.numberOfIndexSubtables)*8 > len(data) {
		return ErrInvalidCBLCData
	}

	strike.indexSubtables = make([]indexSubtable, strike.numberOfIndexSubtables)

	// Parse IndexSubtableArray (records pointing to actual subtables).
	for i := uint32(0); i < strike.numberOfIndexSubtables; i++ {
		recordOffset := listOffset + int(i)*8

		ist := &strike.indexSubtables[i]
		ist.firstGlyphIndex = binary.BigEndian.Uint16(data[recordOffset : recordOffset+2])
		ist.lastGlyphIndex = binary.BigEndian.Uint16(data[recordOffset+2 : recordOffset+4])
		additionalOffset := binary.BigEndian.Uint32(data[recordOffset+4 : recordOffset+8])

		// Parse the actual index subtable.
		subtableOffset := listOffset + int(additionalOffset)
		if err := e.parseIndexSubtable(subtableOffset, ist); err != nil {
			return err
		}
	}

	return nil
}

// parseIndexSubtable parses a single index subtable.
func (e *CBDTExtractor) parseIndexSubtable(offset int, ist *indexSubtable) error {
	data := e.cblcData

	if offset+8 > len(data) {
		return ErrInvalidCBLCData
	}

	// IndexSubHeader (common to all formats).
	ist.indexFormat = binary.BigEndian.Uint16(data[offset : offset+2])
	ist.imageFormat = binary.BigEndian.Uint16(data[offset+2 : offset+4])
	ist.imageDataOffset = binary.BigEndian.Uint32(data[offset+4 : offset+8])

	headerEnd := offset + 8
	numGlyphs := int(ist.lastGlyphIndex) - int(ist.firstGlyphIndex) + 1

	switch ist.indexFormat {
	case indexFormat1:
		// Variable metrics, 32-bit offsets.
		numOffsets := numGlyphs + 1
		if headerEnd+numOffsets*4 > len(data) {
			return ErrInvalidCBLCData
		}
		ist.offsets32 = make([]uint32, numOffsets)
		for i := 0; i < numOffsets; i++ {
			pos := headerEnd + i*4
			ist.offsets32[i] = binary.BigEndian.Uint32(data[pos : pos+4])
		}

	case indexFormat2:
		// Constant metrics, no offset array.
		if headerEnd+4+8 > len(data) {
			return ErrInvalidCBLCData
		}
		ist.imageSize = binary.BigEndian.Uint32(data[headerEnd : headerEnd+4])
		ist.bigMetrics = &bigGlyphMetrics{}
		parseBigGlyphMetrics(data[headerEnd+4:headerEnd+12], ist.bigMetrics)

	case indexFormat3:
		// Variable metrics, 16-bit offsets.
		numOffsets := numGlyphs + 1
		if headerEnd+numOffsets*2 > len(data) {
			return ErrInvalidCBLCData
		}
		ist.offsets16 = make([]uint16, numOffsets)
		for i := 0; i < numOffsets; i++ {
			pos := headerEnd + i*2
			ist.offsets16[i] = binary.BigEndian.Uint16(data[pos : pos+2])
		}

	case indexFormat4:
		// Variable metrics, sparse glyph IDs.
		if headerEnd+4 > len(data) {
			return ErrInvalidCBLCData
		}
		numGlyphsInTable := binary.BigEndian.Uint32(data[headerEnd : headerEnd+4])
		numPairs := int(numGlyphsInTable) + 1 // Extra for end marker.

		pairsOffset := headerEnd + 4
		if pairsOffset+numPairs*4 > len(data) {
			return ErrInvalidCBLCData
		}

		ist.glyphPairs = make([]glyphIDOffsetPair, numPairs)
		for i := 0; i < numPairs; i++ {
			pos := pairsOffset + i*4
			ist.glyphPairs[i].glyphID = binary.BigEndian.Uint16(data[pos : pos+2])
			ist.glyphPairs[i].sbitOffset = binary.BigEndian.Uint16(data[pos+2 : pos+4])
		}

	case indexFormat5:
		// Constant metrics, sparse glyph IDs.
		if headerEnd+4+8+4 > len(data) {
			return ErrInvalidCBLCData
		}
		ist.imageSize = binary.BigEndian.Uint32(data[headerEnd : headerEnd+4])
		ist.bigMetrics = &bigGlyphMetrics{}
		parseBigGlyphMetrics(data[headerEnd+4:headerEnd+12], ist.bigMetrics)

		numGlyphsInTable := binary.BigEndian.Uint32(data[headerEnd+12 : headerEnd+16])
		glyphIDsOffset := headerEnd + 16

		if glyphIDsOffset+int(numGlyphsInTable)*2 > len(data) {
			return ErrInvalidCBLCData
		}

		ist.glyphIDs = make([]uint16, numGlyphsInTable)
		for i := uint32(0); i < numGlyphsInTable; i++ {
			pos := glyphIDsOffset + int(i)*2
			ist.glyphIDs[i] = binary.BigEndian.Uint16(data[pos : pos+2])
		}

	default:
		return ErrUnsupportedIndexFormat
	}

	return nil
}

// parseBigGlyphMetrics parses BigGlyphMetrics from 8 bytes.
func parseBigGlyphMetrics(data []byte, m *bigGlyphMetrics) {
	m.height = data[0]
	m.width = data[1]
	m.horiBearingX = int8(data[2])
	m.horiBearingY = int8(data[3])
	m.horiAdvance = data[4]
	m.vertBearingX = int8(data[5])
	m.vertBearingY = int8(data[6])
	m.vertAdvance = data[7]
}

// parseSmallGlyphMetrics parses SmallGlyphMetrics from 5 bytes.
func parseSmallGlyphMetrics(data []byte, m *smallGlyphMetrics) {
	m.height = data[0]
	m.width = data[1]
	m.bearingX = int8(data[2])
	m.bearingY = int8(data[3])
	m.advance = data[4]
}

// GetGlyph extracts a bitmap glyph at the specified PPEM.
// Uses StrikeBestFit strategy to select the strike.
func (e *CBDTExtractor) GetGlyph(glyphID uint16, ppem uint16) (*BitmapGlyph, error) {
	return e.GetGlyphWithStrategy(glyphID, ppem, StrikeBestFit)
}

// GetGlyphWithStrategy extracts a bitmap glyph using the specified strategy.
func (e *CBDTExtractor) GetGlyphWithStrategy(glyphID uint16, ppem uint16, strategy StrikeStrategy) (*BitmapGlyph, error) {
	strikeIndex := e.SelectStrike(ppem, strategy)
	if strikeIndex < 0 {
		return nil, ErrNoStrikeAvailable
	}

	return e.GetGlyphAtStrike(glyphID, strikeIndex)
}

// GetGlyphAtStrike extracts a bitmap glyph at the specified strike index.
func (e *CBDTExtractor) GetGlyphAtStrike(glyphID uint16, strikeIndex int) (*BitmapGlyph, error) {
	if strikeIndex < 0 || strikeIndex >= len(e.strikes) {
		return nil, ErrNoStrikeAvailable
	}

	strike := &e.strikes[strikeIndex]

	// Quick range check.
	if glyphID < strike.startGlyphIndex || glyphID > strike.endGlyphIndex {
		return nil, ErrGlyphNotInBitmap
	}

	// Ensure index subtables are parsed.
	if err := e.parseIndexSubtables(strikeIndex); err != nil {
		return nil, err
	}

	// Find the index subtable containing this glyph.
	for i := range strike.indexSubtables {
		ist := &strike.indexSubtables[i]
		if glyphID >= ist.firstGlyphIndex && glyphID <= ist.lastGlyphIndex {
			return e.extractGlyphFromSubtable(glyphID, ist, strike)
		}
	}

	return nil, ErrGlyphNotInBitmap
}

// extractGlyphFromSubtable extracts glyph data from a specific index subtable.
func (e *CBDTExtractor) extractGlyphFromSubtable(glyphID uint16, ist *indexSubtable, strike *bitmapStrike) (*BitmapGlyph, error) {
	// Calculate the data offset and size based on index format.
	dataOffset, dataSize, metrics, err := e.calculateGlyphLocation(glyphID, ist)
	if err != nil {
		return nil, err
	}

	if dataSize == 0 {
		return nil, ErrGlyphNotInBitmap
	}

	// Extract the image data from CBDT.
	return e.extractImageData(glyphID, dataOffset, dataSize, ist.imageFormat, metrics, strike)
}

// calculateGlyphLocation calculates the offset and size of glyph data in CBDT.
func (e *CBDTExtractor) calculateGlyphLocation(glyphID uint16, ist *indexSubtable) (offset uint32, size uint32, metrics *bigGlyphMetrics, err error) {
	glyphIndex := int(glyphID) - int(ist.firstGlyphIndex)

	switch ist.indexFormat {
	case indexFormat1:
		if glyphIndex < 0 || glyphIndex >= len(ist.offsets32)-1 {
			return 0, 0, nil, ErrGlyphNotInBitmap
		}
		offset = ist.imageDataOffset + ist.offsets32[glyphIndex]
		size = ist.offsets32[glyphIndex+1] - ist.offsets32[glyphIndex]

	case indexFormat2:
		if glyphIndex < 0 || glyphIndex >= int(ist.lastGlyphIndex-ist.firstGlyphIndex)+1 {
			return 0, 0, nil, ErrGlyphNotInBitmap
		}
		// #nosec G115 -- glyphIndex bounds checked above, overflow not possible
		offset = ist.imageDataOffset + uint32(glyphIndex)*ist.imageSize
		size = ist.imageSize
		metrics = ist.bigMetrics

	case indexFormat3:
		if glyphIndex < 0 || glyphIndex >= len(ist.offsets16)-1 {
			return 0, 0, nil, ErrGlyphNotInBitmap
		}
		offset = ist.imageDataOffset + uint32(ist.offsets16[glyphIndex])
		size = uint32(ist.offsets16[glyphIndex+1] - ist.offsets16[glyphIndex])

	case indexFormat4:
		// Binary search for glyph ID.
		found := false
		for i := 0; i < len(ist.glyphPairs)-1; i++ {
			if ist.glyphPairs[i].glyphID == glyphID {
				offset = ist.imageDataOffset + uint32(ist.glyphPairs[i].sbitOffset)
				size = uint32(ist.glyphPairs[i+1].sbitOffset - ist.glyphPairs[i].sbitOffset)
				found = true
				break
			}
		}
		if !found {
			return 0, 0, nil, ErrGlyphNotInBitmap
		}

	case indexFormat5:
		// Search for glyph ID in array.
		found := false
		for i, gid := range ist.glyphIDs {
			if gid != glyphID {
				continue
			}
			// #nosec G115 -- i is index into small array, overflow not possible
			offset = ist.imageDataOffset + uint32(i)*ist.imageSize
			size = ist.imageSize
			metrics = ist.bigMetrics
			found = true
			break
		}
		if !found {
			return 0, 0, nil, ErrGlyphNotInBitmap
		}

	default:
		return 0, 0, nil, ErrUnsupportedIndexFormat
	}

	return offset, size, metrics, nil
}

// extractImageData extracts and parses the image data from CBDT.
func (e *CBDTExtractor) extractImageData(glyphID uint16, offset, size uint32, imageFormat uint16, sharedMetrics *bigGlyphMetrics, strike *bitmapStrike) (*BitmapGlyph, error) {
	data := e.cbdtData

	if int(offset+size) > len(data) {
		return nil, ErrInvalidCBDTData
	}

	imageData := data[offset : offset+size]

	glyph := &BitmapGlyph{
		GlyphID: glyphID,
		PPEM:    uint16(strike.ppemX),
	}

	switch imageFormat {
	case imageFormat17:
		// SmallGlyphMetrics (5 bytes) + dataLen (4 bytes) + PNG data.
		if len(imageData) < 9 {
			return nil, ErrInvalidCBDTData
		}

		var sm smallGlyphMetrics
		parseSmallGlyphMetrics(imageData[0:5], &sm)

		dataLen := binary.BigEndian.Uint32(imageData[5:9])
		if int(9+dataLen) > len(imageData) {
			return nil, ErrInvalidCBDTData
		}

		glyph.Width = int(sm.width)
		glyph.Height = int(sm.height)
		glyph.OriginX = float32(sm.bearingX)
		glyph.OriginY = float32(sm.bearingY)
		glyph.Data = imageData[9 : 9+dataLen]
		glyph.Format = FormatPNG

	case imageFormat18:
		// BigGlyphMetrics (8 bytes) + dataLen (4 bytes) + PNG data.
		if len(imageData) < 12 {
			return nil, ErrInvalidCBDTData
		}

		var bm bigGlyphMetrics
		parseBigGlyphMetrics(imageData[0:8], &bm)

		dataLen := binary.BigEndian.Uint32(imageData[8:12])
		if int(12+dataLen) > len(imageData) {
			return nil, ErrInvalidCBDTData
		}

		glyph.Width = int(bm.width)
		glyph.Height = int(bm.height)
		glyph.OriginX = float32(bm.horiBearingX)
		glyph.OriginY = float32(bm.horiBearingY)
		glyph.Data = imageData[12 : 12+dataLen]
		glyph.Format = FormatPNG

	case imageFormat19:
		// Metrics from CBLC, only dataLen (4 bytes) + PNG data in CBDT.
		if len(imageData) < 4 {
			return nil, ErrInvalidCBDTData
		}

		dataLen := binary.BigEndian.Uint32(imageData[0:4])
		if int(4+dataLen) > len(imageData) {
			return nil, ErrInvalidCBDTData
		}

		// Use shared metrics from index subtable.
		if sharedMetrics != nil {
			glyph.Width = int(sharedMetrics.width)
			glyph.Height = int(sharedMetrics.height)
			glyph.OriginX = float32(sharedMetrics.horiBearingX)
			glyph.OriginY = float32(sharedMetrics.horiBearingY)
		}

		glyph.Data = imageData[4 : 4+dataLen]
		glyph.Format = FormatPNG

	default:
		return nil, ErrUnsupportedImageFormat
	}

	return glyph, nil
}

// AvailablePPEMs returns a list of available PPEM sizes.
func (e *CBDTExtractor) AvailablePPEMs() []uint16 {
	ppems := make([]uint16, len(e.strikes))
	for i := range e.strikes {
		ppems[i] = uint16(e.strikes[i].ppemX)
	}
	return ppems
}
