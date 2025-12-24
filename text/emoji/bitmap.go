package emoji

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/png"
)

// unknownStr is the string returned for unknown enum values.
const unknownStr = "Unknown"

// Bitmap table format errors.
var (
	// ErrNoSBIXTable indicates the font doesn't have an sbix table.
	ErrNoSBIXTable = errors.New("emoji: font has no sbix table")

	// ErrNoCBDTTable indicates the font doesn't have a CBDT table.
	ErrNoCBDTTable = errors.New("emoji: font has no CBDT table")

	// ErrInvalidSBIXData indicates the sbix table data is malformed.
	ErrInvalidSBIXData = errors.New("emoji: invalid sbix table data")

	// ErrInvalidCBDTData indicates the CBDT table data is malformed.
	ErrInvalidCBDTData = errors.New("emoji: invalid CBDT table data")

	// ErrGlyphNotInBitmap indicates the glyph has no bitmap data.
	ErrGlyphNotInBitmap = errors.New("emoji: glyph not found in bitmap table")

	// ErrUnsupportedBitmapFormat indicates an unsupported bitmap format.
	ErrUnsupportedBitmapFormat = errors.New("emoji: unsupported bitmap format")
)

// BitmapFormat indicates the format of embedded bitmap data.
type BitmapFormat int

const (
	// FormatPNG is PNG-compressed bitmap data.
	FormatPNG BitmapFormat = iota

	// FormatJPEG is JPEG-compressed bitmap data.
	FormatJPEG

	// FormatTIFF is TIFF-compressed bitmap data.
	FormatTIFF

	// FormatDUPE indicates this glyph references another glyph's bitmap.
	FormatDUPE

	// FormatRaw is uncompressed RGBA or grayscale bitmap data.
	FormatRaw
)

// bitmapFormatNames maps BitmapFormat to string names.
var bitmapFormatNames = [...]string{
	FormatPNG:  "PNG",
	FormatJPEG: "JPEG",
	FormatTIFF: "TIFF",
	FormatDUPE: "DUPE",
	FormatRaw:  "Raw",
}

// String returns the string name of the bitmap format.
func (f BitmapFormat) String() string {
	if int(f) < len(bitmapFormatNames) {
		return bitmapFormatNames[f]
	}
	return unknownStr
}

// BitmapGlyph represents a bitmap emoji from sbix/CBDT tables.
type BitmapGlyph struct {
	// GlyphID is the glyph ID this bitmap represents.
	GlyphID uint16

	// Data contains the raw bitmap data (PNG, JPEG, etc.).
	Data []byte

	// Format indicates how Data is encoded.
	Format BitmapFormat

	// Width is the bitmap width in pixels.
	Width int

	// Height is the bitmap height in pixels.
	Height int

	// OriginX is the horizontal offset from glyph origin.
	OriginX float32

	// OriginY is the vertical offset from glyph origin.
	OriginY float32

	// PPEM is the pixels-per-em for this bitmap size.
	PPEM uint16
}

// Decode decodes the bitmap data to an image.Image.
// Currently only supports PNG format.
// Returns nil if the format is not supported or data is invalid.
func (b *BitmapGlyph) Decode() (image.Image, error) {
	switch b.Format {
	case FormatPNG:
		return png.Decode(bytes.NewReader(b.Data))
	case FormatJPEG:
		// JPEG support would require image/jpeg import
		// For now, return an error indicating it's not yet implemented
		return nil, errors.New("emoji: JPEG bitmap decoding not yet implemented")
	default:
		return nil, ErrUnsupportedBitmapFormat
	}
}

// SBIXParser parses the sbix (Standard Bitmap Graphics) table.
// sbix is Apple's format for embedded bitmap graphics (emoji).
type SBIXParser struct {
	data      []byte
	numGlyphs uint16

	// Parsed strike information
	strikes []sbixStrike
}

// sbixStrike represents a bitmap strike (size) in sbix.
type sbixStrike struct {
	ppem      uint16
	ppi       uint16
	offset    uint32
	glyphData []uint32 // Offsets to glyph data
}

// NewSBIXParser creates a new sbix parser.
// numGlyphs should be from the maxp table.
func NewSBIXParser(data []byte, numGlyphs uint16) (*SBIXParser, error) {
	if len(data) == 0 {
		return nil, ErrNoSBIXTable
	}
	if len(data) < 8 {
		return nil, ErrInvalidSBIXData
	}

	p := &SBIXParser{
		data:      data,
		numGlyphs: numGlyphs,
	}

	if err := p.parse(); err != nil {
		return nil, err
	}

	return p, nil
}

// parse parses the sbix table structure.
func (p *SBIXParser) parse() error {
	data := p.data

	// sbix header
	version := binary.BigEndian.Uint16(data[0:2])
	if version != 1 {
		return ErrInvalidSBIXData
	}

	// flags := binary.BigEndian.Uint16(data[2:4]) // unused
	numStrikes := binary.BigEndian.Uint32(data[4:8])

	// Parse strike offsets
	if int(8+numStrikes*4) > len(data) {
		return ErrInvalidSBIXData
	}

	p.strikes = make([]sbixStrike, numStrikes)
	for i := uint32(0); i < numStrikes; i++ {
		offset := binary.BigEndian.Uint32(data[8+i*4 : 12+i*4])
		if err := p.parseStrike(i, offset); err != nil {
			return err
		}
	}

	return nil
}

// parseStrike parses a single strike record.
func (p *SBIXParser) parseStrike(index uint32, offset uint32) error {
	data := p.data
	if int(offset)+4 > len(data) {
		return ErrInvalidSBIXData
	}

	strike := &p.strikes[index]
	strike.offset = offset
	strike.ppem = binary.BigEndian.Uint16(data[offset : offset+2])
	strike.ppi = binary.BigEndian.Uint16(data[offset+2 : offset+4])

	// Parse glyph data offsets
	glyphOffsetStart := offset + 4
	numOffsets := int(p.numGlyphs) + 1 // One extra for length calculation
	if int(glyphOffsetStart)+numOffsets*4 > len(data) {
		return ErrInvalidSBIXData
	}

	strike.glyphData = make([]uint32, numOffsets)
	for i := 0; i < numOffsets; i++ {
		pos := int(glyphOffsetStart) + i*4
		strike.glyphData[i] = binary.BigEndian.Uint32(data[pos : pos+4])
	}

	return nil
}

// NumStrikes returns the number of bitmap strikes (sizes).
func (p *SBIXParser) NumStrikes() int {
	return len(p.strikes)
}

// StrikePPEM returns the ppem for a strike index.
func (p *SBIXParser) StrikePPEM(strikeIndex int) uint16 {
	if strikeIndex < 0 || strikeIndex >= len(p.strikes) {
		return 0
	}
	return p.strikes[strikeIndex].ppem
}

// HasGlyph returns true if the glyph has bitmap data at the given strike.
func (p *SBIXParser) HasGlyph(glyphID, strikeIndex int) bool {
	if strikeIndex < 0 || strikeIndex >= len(p.strikes) {
		return false
	}
	if glyphID < 0 || glyphID >= int(p.numGlyphs) {
		return false
	}

	strike := &p.strikes[strikeIndex]
	glyphStart := strike.glyphData[glyphID]
	glyphEnd := strike.glyphData[glyphID+1]

	return glyphEnd > glyphStart
}

// GetGlyph extracts bitmap data for a glyph at the given strike.
func (p *SBIXParser) GetGlyph(glyphID, strikeIndex int) (*BitmapGlyph, error) {
	if strikeIndex < 0 || strikeIndex >= len(p.strikes) {
		return nil, ErrGlyphNotInBitmap
	}
	if glyphID < 0 || glyphID >= int(p.numGlyphs) {
		return nil, ErrGlyphNotInBitmap
	}

	strike := &p.strikes[strikeIndex]
	glyphStart := strike.glyphData[glyphID]
	glyphEnd := strike.glyphData[glyphID+1]

	if glyphEnd <= glyphStart {
		return nil, ErrGlyphNotInBitmap
	}

	// Glyph data format:
	// originOffsetX: int16
	// originOffsetY: int16
	// graphicType: 4 bytes (tag)
	// data: remaining bytes

	dataOffset := strike.offset + glyphStart
	if int(dataOffset)+8 > len(p.data) {
		return nil, ErrInvalidSBIXData
	}

	// #nosec G115 -- uint16 to int16 reinterpretation is intentional for font data
	originX := int16(binary.BigEndian.Uint16(p.data[dataOffset : dataOffset+2]))
	// #nosec G115 -- uint16 to int16 reinterpretation is intentional for font data
	originY := int16(binary.BigEndian.Uint16(p.data[dataOffset+2 : dataOffset+4]))
	graphicType := string(p.data[dataOffset+4 : dataOffset+8])

	// Determine format from graphic type tag
	format, err := parseGraphicType(graphicType)
	if err != nil {
		return nil, err
	}

	// Extract image data
	imageDataOffset := dataOffset + 8
	imageDataEnd := strike.offset + glyphEnd
	if int(imageDataEnd) > len(p.data) {
		return nil, ErrInvalidSBIXData
	}

	imageData := p.data[imageDataOffset:imageDataEnd]

	bitmap := &BitmapGlyph{
		GlyphID: uint16(glyphID), //#nosec G115 -- bounds checked above
		Data:    imageData,
		Format:  format,
		OriginX: float32(originX),
		OriginY: float32(originY),
		PPEM:    strike.ppem,
	}

	// Try to decode PNG to get dimensions
	if format == FormatPNG && len(imageData) > 0 {
		if img, decodeErr := png.Decode(bytes.NewReader(imageData)); decodeErr == nil {
			bounds := img.Bounds()
			bitmap.Width = bounds.Dx()
			bitmap.Height = bounds.Dy()
		}
	}

	return bitmap, nil
}

// BestStrikeForPPEM returns the strike index best matching the requested ppem.
func (p *SBIXParser) BestStrikeForPPEM(ppem uint16) int {
	if len(p.strikes) == 0 {
		return -1
	}

	bestIdx := 0
	bestDiff := absDiff(p.strikes[0].ppem, ppem)

	for i := 1; i < len(p.strikes); i++ {
		diff := absDiff(p.strikes[i].ppem, ppem)
		// Prefer larger or equal size
		if diff < bestDiff || (diff == bestDiff && p.strikes[i].ppem > p.strikes[bestIdx].ppem) {
			bestIdx = i
			bestDiff = diff
		}
	}

	return bestIdx
}

// parseGraphicType converts a 4-byte graphic type tag to BitmapFormat.
func parseGraphicType(tag string) (BitmapFormat, error) {
	switch tag {
	case "png ":
		return FormatPNG, nil
	case "jpg ":
		return FormatJPEG, nil
	case "tiff":
		return FormatTIFF, nil
	case "dupe":
		return FormatDUPE, nil
	default:
		return FormatRaw, ErrUnsupportedBitmapFormat
	}
}

// absDiff returns the absolute difference between two uint16 values.
func absDiff(a, b uint16) uint16 {
	if a > b {
		return a - b
	}
	return b - a
}

// CBDTParser parses the CBDT (Color Bitmap Data) table.
// CBDT is Google's format for embedded bitmap graphics (emoji).
// It works together with CBLC (Color Bitmap Location) table.
type CBDTParser struct {
	cbdtData []byte
	cblcData []byte
}

// NewCBDTParser creates a new CBDT parser.
func NewCBDTParser(cbdtData, cblcData []byte) (*CBDTParser, error) {
	if len(cbdtData) == 0 {
		return nil, ErrNoCBDTTable
	}
	if len(cblcData) == 0 {
		return nil, errors.New("emoji: font has no CBLC table")
	}

	return &CBDTParser{
		cbdtData: cbdtData,
		cblcData: cblcData,
	}, nil
}

// Note: Full CBDT parsing is complex and involves multiple index formats.
// This is a placeholder for future implementation.
// For now, the COLR approach or sbix is more commonly used for color emoji.

// HasTable returns true if CBDT/CBLC tables are present.
func (p *CBDTParser) HasTable() bool {
	return len(p.cbdtData) > 0 && len(p.cblcData) > 0
}
