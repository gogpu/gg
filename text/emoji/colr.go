package emoji

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/draw"
)

// COLR/CPAL table format errors.
var (
	// ErrNoCOLRTable indicates the font doesn't have a COLR table.
	ErrNoCOLRTable = errors.New("emoji: font has no COLR table")

	// ErrNoCPALTable indicates the font doesn't have a CPAL table.
	ErrNoCPALTable = errors.New("emoji: font has no CPAL table")

	// ErrInvalidCOLRData indicates the COLR table data is malformed.
	ErrInvalidCOLRData = errors.New("emoji: invalid COLR table data")

	// ErrInvalidCPALData indicates the CPAL table data is malformed.
	ErrInvalidCPALData = errors.New("emoji: invalid CPAL table data")

	// ErrGlyphNotInCOLR indicates the glyph is not a color glyph.
	ErrGlyphNotInCOLR = errors.New("emoji: glyph not found in COLR table")

	// ErrUnsupportedCOLRVersion indicates an unsupported COLR version.
	ErrUnsupportedCOLRVersion = errors.New("emoji: unsupported COLR version")
)

// COLRGlyph represents a color glyph from COLR table.
// It consists of multiple colored layers stacked on top of each other.
type COLRGlyph struct {
	// GlyphID is the original glyph ID for this color glyph.
	GlyphID uint16

	// Layers contains the color layers, bottom to top.
	Layers []ColorLayer

	// Bounds is the bounding box in font units.
	Bounds Rect

	// Version is the COLR table version (0 or 1).
	Version uint16
}

// ColorLayer represents one layer of a color glyph.
// Each layer is a glyph rendered in a specific color.
type ColorLayer struct {
	// GlyphID is the glyph to render for this layer.
	GlyphID uint16

	// PaletteIndex is the index into the CPAL color palette.
	// 0xFFFF indicates foreground color (use the text color).
	PaletteIndex uint16

	// Color is the resolved color from the palette.
	// This is set after calling ResolvePalette.
	Color Color
}

// IsForeground returns true if this layer uses the foreground text color.
func (l ColorLayer) IsForeground() bool {
	return l.PaletteIndex == 0xFFFF
}

// Color represents an RGBA color from CPAL palette.
type Color struct {
	R, G, B, A uint8
}

// RGBA implements color.Color interface.
func (c Color) RGBA() (r, g, b, a uint32) {
	return uint32(c.R) * 257, uint32(c.G) * 257, uint32(c.B) * 257, uint32(c.A) * 257
}

// ToRGBA returns the color as color.RGBA.
func (c Color) ToRGBA() color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}

// Rect represents a bounding rectangle.
type Rect struct {
	MinX, MinY float64
	MaxX, MaxY float64
}

// Width returns the width of the rectangle.
func (r Rect) Width() float64 {
	return r.MaxX - r.MinX
}

// Height returns the height of the rectangle.
func (r Rect) Height() float64 {
	return r.MaxY - r.MinY
}

// Empty returns true if the rectangle has zero area.
func (r Rect) Empty() bool {
	return r.MinX >= r.MaxX || r.MinY >= r.MaxY
}

// COLRParser parses COLR/CPAL tables from font data.
type COLRParser struct {
	colrData []byte
	cpalData []byte

	// Parsed COLR header
	version    uint16
	numGlyphs  uint16
	baseGlyphs []baseGlyphRecord
	layers     []layerRecord

	// Parsed CPAL palette
	palettes [][]Color
}

// baseGlyphRecord from COLRv0.
type baseGlyphRecord struct {
	glyphID     uint16
	firstLayer  uint16
	numLayers   uint16
}

// layerRecord from COLRv0.
type layerRecord struct {
	glyphID      uint16
	paletteIndex uint16
}

// NewCOLRParser creates a new COLR parser from table data.
// colrData is the raw COLR table, cpalData is the raw CPAL table.
func NewCOLRParser(colrData, cpalData []byte) (*COLRParser, error) {
	if len(colrData) == 0 {
		return nil, ErrNoCOLRTable
	}
	if len(cpalData) == 0 {
		return nil, ErrNoCPALTable
	}

	p := &COLRParser{
		colrData: colrData,
		cpalData: cpalData,
	}

	if err := p.parseCOLRHeader(); err != nil {
		return nil, err
	}

	if err := p.parseCPAL(); err != nil {
		return nil, err
	}

	return p, nil
}

// parseCOLRHeader parses the COLR table header and records.
func (p *COLRParser) parseCOLRHeader() error {
	data := p.colrData
	if len(data) < 14 {
		return ErrInvalidCOLRData
	}

	p.version = binary.BigEndian.Uint16(data[0:2])

	// We support COLRv0 and basic COLRv1
	if p.version > 1 {
		return ErrUnsupportedCOLRVersion
	}

	p.numGlyphs = binary.BigEndian.Uint16(data[2:4])
	baseGlyphOffset := binary.BigEndian.Uint32(data[4:8])
	layerRecordOffset := binary.BigEndian.Uint32(data[8:12])
	numLayers := binary.BigEndian.Uint16(data[12:14])

	// Parse base glyph records
	if err := p.parseBaseGlyphs(baseGlyphOffset); err != nil {
		return err
	}

	// Parse layer records
	return p.parseLayers(layerRecordOffset, numLayers)
}

// parseBaseGlyphs parses the base glyph records.
func (p *COLRParser) parseBaseGlyphs(offset uint32) error {
	data := p.colrData
	recordSize := 6 // glyphID (2) + firstLayer (2) + numLayers (2)

	for i := uint16(0); i < p.numGlyphs; i++ {
		pos := int(offset) + int(i)*recordSize
		if pos+recordSize > len(data) {
			return ErrInvalidCOLRData
		}

		record := baseGlyphRecord{
			glyphID:    binary.BigEndian.Uint16(data[pos : pos+2]),
			firstLayer: binary.BigEndian.Uint16(data[pos+2 : pos+4]),
			numLayers:  binary.BigEndian.Uint16(data[pos+4 : pos+6]),
		}
		p.baseGlyphs = append(p.baseGlyphs, record)
	}

	return nil
}

// parseLayers parses the layer records.
func (p *COLRParser) parseLayers(offset uint32, numLayers uint16) error {
	data := p.colrData
	recordSize := 4 // glyphID (2) + paletteIndex (2)

	for i := uint16(0); i < numLayers; i++ {
		pos := int(offset) + int(i)*recordSize
		if pos+recordSize > len(data) {
			return ErrInvalidCOLRData
		}

		record := layerRecord{
			glyphID:      binary.BigEndian.Uint16(data[pos : pos+2]),
			paletteIndex: binary.BigEndian.Uint16(data[pos+2 : pos+4]),
		}
		p.layers = append(p.layers, record)
	}

	return nil
}

// parseCPAL parses the CPAL (Color Palette) table.
func (p *COLRParser) parseCPAL() error {
	data := p.cpalData
	if len(data) < 12 {
		return ErrInvalidCPALData
	}

	version := binary.BigEndian.Uint16(data[0:2])
	if version > 1 {
		// We handle v0 and v1 the same for basic color extraction
		_ = version
	}

	numEntries := binary.BigEndian.Uint16(data[2:4])
	numPalettes := binary.BigEndian.Uint16(data[4:6])
	// numColorRecords := binary.BigEndian.Uint16(data[6:8]) // unused for now
	colorRecordsOffset := binary.BigEndian.Uint32(data[8:12])

	// Parse palette offsets
	if 12+int(numPalettes)*2 > len(data) {
		return ErrInvalidCPALData
	}

	paletteOffsets := make([]uint16, numPalettes)
	for i := uint16(0); i < numPalettes; i++ {
		pos := 12 + int(i)*2
		paletteOffsets[i] = binary.BigEndian.Uint16(data[pos : pos+2])
	}

	// Parse color records for each palette
	p.palettes = make([][]Color, numPalettes)
	for i := uint16(0); i < numPalettes; i++ {
		palette := make([]Color, numEntries)
		for j := uint16(0); j < numEntries; j++ {
			colorIndex := paletteOffsets[i] + j
			pos := int(colorRecordsOffset) + int(colorIndex)*4
			if pos+4 > len(data) {
				return ErrInvalidCPALData
			}

			// CPAL stores colors as BGRA
			palette[j] = Color{
				B: data[pos],
				G: data[pos+1],
				R: data[pos+2],
				A: data[pos+3],
			}
		}
		p.palettes[i] = palette
	}

	return nil
}

// HasGlyph returns true if the glyph ID is a color glyph.
func (p *COLRParser) HasGlyph(glyphID uint16) bool {
	_, found := p.findBaseGlyph(glyphID)
	return found
}

// GetGlyph returns the COLRGlyph for the given glyph ID.
// Returns ErrGlyphNotInCOLR if the glyph is not a color glyph.
func (p *COLRParser) GetGlyph(glyphID uint16, paletteIndex int) (*COLRGlyph, error) {
	record, found := p.findBaseGlyph(glyphID)
	if !found {
		return nil, ErrGlyphNotInCOLR
	}

	glyph := &COLRGlyph{
		GlyphID: glyphID,
		Layers:  make([]ColorLayer, record.numLayers),
		Version: p.version,
	}

	// Extract layers
	for i := uint16(0); i < record.numLayers; i++ {
		layerIdx := record.firstLayer + i
		if int(layerIdx) >= len(p.layers) {
			return nil, ErrInvalidCOLRData
		}

		layer := p.layers[layerIdx]
		glyph.Layers[i] = ColorLayer{
			GlyphID:      layer.glyphID,
			PaletteIndex: layer.paletteIndex,
		}

		// Resolve color from palette
		if !glyph.Layers[i].IsForeground() {
			if paletteIndex < len(p.palettes) && int(layer.paletteIndex) < len(p.palettes[paletteIndex]) {
				glyph.Layers[i].Color = p.palettes[paletteIndex][layer.paletteIndex]
			}
		}
	}

	return glyph, nil
}

// findBaseGlyph finds the base glyph record for a glyph ID.
// Uses binary search since glyphs are sorted.
func (p *COLRParser) findBaseGlyph(glyphID uint16) (baseGlyphRecord, bool) {
	// Binary search
	lo, hi := 0, len(p.baseGlyphs)
	for lo < hi {
		mid := (lo + hi) / 2
		if p.baseGlyphs[mid].glyphID < glyphID {
			lo = mid + 1
		} else {
			hi = mid
		}
	}

	if lo < len(p.baseGlyphs) && p.baseGlyphs[lo].glyphID == glyphID {
		return p.baseGlyphs[lo], true
	}

	return baseGlyphRecord{}, false
}

// NumPalettes returns the number of color palettes.
func (p *COLRParser) NumPalettes() int {
	return len(p.palettes)
}

// PaletteColors returns the colors in a palette.
func (p *COLRParser) PaletteColors(paletteIndex int) []Color {
	if paletteIndex < 0 || paletteIndex >= len(p.palettes) {
		return nil
	}
	return p.palettes[paletteIndex]
}

// RenderCOLRToImage renders a COLR glyph to an RGBA image.
// This is a simplified renderer that composites layers.
// For production use, each layer glyph should be rasterized
// at the given size and composited with its color.
//
// Parameters:
//   - glyph: The COLR glyph to render
//   - renderLayer: Function to render a single layer glyph to an alpha mask
//   - width, height: Size of the output image
//   - foreground: Color to use for foreground (text) layers
func RenderCOLRToImage(
	glyph *COLRGlyph,
	renderLayer func(glyphID uint16) *image.Alpha,
	width, height int,
	foreground color.RGBA,
) *image.RGBA {
	if glyph == nil || len(glyph.Layers) == 0 {
		return nil
	}

	result := image.NewRGBA(image.Rect(0, 0, width, height))

	for _, layer := range glyph.Layers {
		// Render the layer glyph to an alpha mask
		mask := renderLayer(layer.GlyphID)
		if mask == nil {
			continue
		}

		// Determine layer color
		var layerColor color.RGBA
		if layer.IsForeground() {
			layerColor = foreground
		} else {
			layerColor = layer.Color.ToRGBA()
		}

		// Create a uniform color image
		colorImg := image.NewUniform(layerColor)

		// Composite layer onto result
		draw.DrawMask(result, result.Bounds(), colorImg, image.Point{}, mask, image.Point{}, draw.Over)
	}

	return result
}

// GlyphRenderer is a function type for rendering a glyph ID to an alpha mask.
type GlyphRenderer func(glyphID uint16, size float64) *image.Alpha
