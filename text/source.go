package text

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
)

// FontSource represents a loaded font file.
// One FontSource can create multiple Face instances at different sizes.
// FontSource is heavyweight and should be shared across the application.
//
// FontSource is safe for concurrent use.
// FontSource must not be copied after creation (enforced by copyCheck).
type FontSource struct {
	// addr is used for copy protection (Ebitengine pattern).
	// It must point to the FontSource itself.
	addr *FontSource

	// Font data
	data   []byte
	parsed ParsedFont // Abstracted font interface (pluggable backend)

	// Metadata
	name string

	// Mutex protects caches and internal state
	mu sync.RWMutex

	// Caches (to be implemented in TASK-044)
	// shapingCache  *Cache[shapingKey, []Glyph]
	// glyphCache    *Cache[glyphKey, *GlyphImage]
	// hasGlyphCache *runeToBoolMap

	// Configuration
	config sourceConfig
}

// NewFontSource creates a FontSource from font data (TTF or OTF).
// The data slice is copied internally and can be reused after this call.
//
// Options can be used to configure caching and parser backend.
func NewFontSource(data []byte, opts ...SourceOption) (*FontSource, error) {
	if len(data) == 0 {
		return nil, ErrEmptyFontData
	}

	// Apply options first to get parser name
	config := defaultSourceConfig()
	for _, opt := range opts {
		opt(&config)
	}

	// Get parser and parse the font.
	// If collection index is set (or data is a .ttc/.otc), use ParseIndex.
	parser := getParser(config.parserName)
	var parsed ParsedFont
	var err error
	if indexer, ok := parser.(interface {
		ParseIndex([]byte, int) (ParsedFont, error)
	}); ok {
		parsed, err = indexer.ParseIndex(data, config.collectionIndex)
	} else {
		parsed, err = parser.Parse(data)
	}
	if err != nil {
		return nil, err
	}

	// Copy the data
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	// Create FontSource
	s := &FontSource{
		data:   dataCopy,
		parsed: parsed,
		config: config,
	}
	s.addr = s // Self-reference for copy detection

	// Extract font name
	s.name = extractFontName(parsed)

	return s, nil
}

// NewFontSourceFromFile loads a FontSource from a font file path.
func NewFontSourceFromFile(path string, opts ...SourceOption) (*FontSource, error) {
	// #nosec G304 -- Font file path is provided by the user
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("text: failed to read font file: %w", err)
	}

	return NewFontSource(data, opts...)
}

// Face creates a Face at the specified size (in points).
// Multiple faces can be created from the same FontSource.
//
// Face is a lightweight object that shares caches with the FontSource.
// Panics if s is nil (e.g. when NewFontSourceFromFile error was ignored).
func (s *FontSource) Face(size float64, opts ...FaceOption) Face {
	if s == nil {
		panic("text: FontSource is nil — did you check the error from NewFontSourceFromFile?")
	}
	s.copyCheck()

	// Apply face options
	config := defaultFaceConfig()
	for _, opt := range opts {
		opt(&config)
	}

	// Create face
	// For now, this is a stub. Full implementation in TASK-043.
	return &sourceFace{
		source: s,
		size:   size,
		config: config,
	}
}

// Name returns the font name.
func (s *FontSource) Name() string {
	s.copyCheck()
	return s.name
}

// Parsed returns the parsed font for advanced operations.
// This is primarily used by Face implementations.
func (s *FontSource) Parsed() ParsedFont {
	s.copyCheck()
	return s.parsed
}

// Close releases resources associated with the FontSource.
// All faces created from this source become invalid after Close.
func (s *FontSource) Close() error {
	s.copyCheck()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear data
	s.data = nil
	s.parsed = nil

	// Clear caches (when implemented in TASK-044)

	return nil
}

// VariationAxis describes a design variation axis in a variable font.
// Each axis has a tag, display name, and valid range of values.
type VariationAxis struct {
	Tag     [4]byte // OpenType axis tag (e.g., "wght")
	Name    string  // Human-readable axis name (e.g., "Weight")
	Minimum float32 // Minimum design-space value
	Default float32 // Default design-space value
	Maximum float32 // Maximum design-space value
}

// NamedInstance describes a predefined variation instance in a variable font.
// Named instances represent specific points on the variation axes that the
// font designer has designated with a name (e.g., "Bold", "Light Condensed").
type NamedInstance struct {
	Name       string          // Instance name from the font's name table
	Variations []FontVariation // Axis values for this instance
}

// IsVariable reports whether the font is a variable font with at least one
// variation axis. Static fonts always return false.
func (s *FontSource) IsVariable() bool {
	s.copyCheck()
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data == nil {
		return false
	}

	axes, _ := s.parseFvarOwn()
	return len(axes) > 0
}

// VariationAxes returns the variation axes defined in the font.
// Returns nil for static (non-variable) fonts.
//
// Each axis describes a continuous design dimension (e.g., weight from 100 to 900)
// with its valid range and default value.
func (s *FontSource) VariationAxes() []VariationAxis {
	s.copyCheck()
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data == nil {
		return nil
	}

	axes, _ := s.parseFvarOwn()
	if len(axes) == 0 {
		return nil
	}

	result := make([]VariationAxis, len(axes))
	for i, axis := range axes {
		result[i] = VariationAxis{
			Tag:     axis.Tag,
			Name:    axisNameFromTag(axis.Tag),
			Minimum: axis.MinValue,
			Default: axis.DefaultValue,
			Maximum: axis.MaxValue,
		}
	}

	return result
}

// NamedInstances returns the predefined named instances in the font.
// Returns nil for static fonts or variable fonts without named instances.
//
// Named instances are specific axis configurations that the font designer
// has designated with a name, such as "Bold", "Light Condensed", etc.
func (s *FontSource) NamedInstances() []NamedInstance {
	s.copyCheck()
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data == nil {
		return nil
	}

	axes, instances := s.parseFvarOwn()
	if len(instances) == 0 {
		return nil
	}

	// Parse name table for instance names.
	tables, err := parseFontTables(s.data)
	if err != nil {
		return nil
	}
	nameData := tables["name"]

	var result []NamedInstance
	for _, inst := range instances {
		name := ""
		if nameData != nil && inst.subfamilyNameID != 0 {
			name = lookupNameTableEntry(nameData, inst.subfamilyNameID)
		}
		if name == "" {
			continue
		}

		var vars []FontVariation
		for j, coord := range inst.coordinates {
			if j < len(axes) {
				vars = append(vars, FontVariation{
					Tag:   axes[j].Tag,
					Value: coord,
				})
			}
		}

		result = append(result, NamedInstance{
			Name:       name,
			Variations: vars,
		})
	}

	return result
}

// ownFvarInstance holds a named instance parsed from the fvar table.
type ownFvarInstance struct {
	subfamilyNameID uint16
	coordinates     []float32
}

// parseFvarOwn parses the fvar table from raw font data using own binary parsing.
// Returns the axes and named instances, or nil on error.
// Caller must hold s.mu (at least RLock).
func (s *FontSource) parseFvarOwn() ([]fvarAxis, []ownFvarInstance) {
	tables, err := parseFontTables(s.data)
	if err != nil {
		return nil, nil
	}

	fvarData, ok := tables["fvar"]
	if !ok {
		return nil, nil
	}

	axes := parseFvarAxes(fvarData)
	if len(axes) == 0 {
		return nil, nil
	}

	instances := parseFvarInstances(fvarData, len(axes))
	return axes, instances
}

// parseFvarInstances parses named instances from raw fvar table data.
// Each instance record is 4 + axisCount*4 bytes (+ optional 2 bytes for postScriptNameID).
//
// fvar table layout after axes:
//
//	uint16 instanceCount
//	uint16 instanceSize
//	InstanceRecord[instanceCount]
//
// InstanceRecord:
//
//	uint16 subfamilyNameID
//	uint16 flags
//	Fixed  coordinates[axisCount] (4 bytes each, 16.16)
//	[uint16 postScriptNameID] (optional, present if instanceSize > minSize)
func parseFvarInstances(data []byte, axisCount int) []ownFvarInstance {
	if len(data) < 14 {
		return nil
	}

	axisArrayOffset := binary.BigEndian.Uint16(data[4:6])
	numAxes := binary.BigEndian.Uint16(data[8:10])
	axisSize := binary.BigEndian.Uint16(data[10:12])
	instanceCount := binary.BigEndian.Uint16(data[12:14])

	if instanceCount == 0 || axisSize < 20 {
		return nil
	}

	// Instance size: if the table has an instanceSize field at offset 14.
	instanceSize := 4 + axisCount*4 // minimum: subfamilyNameID + flags + coordinates
	if len(data) >= 16 {
		// Some fonts store instanceSize at offset 14 (after instanceCount).
		// But per spec, it's part of the fvar header at a fixed position.
		// The field at offset [4] is axesArrayOffset, [8] axisCount, [10] axisSize,
		// [12] instanceCount, [14] instanceSize.
		is := int(binary.BigEndian.Uint16(data[14:16]))
		if is >= instanceSize {
			instanceSize = is
		}
	}

	// Instances start after axes.
	instanceStart := int(axisArrayOffset) + int(numAxes)*int(axisSize)
	if instanceStart+int(instanceCount)*instanceSize > len(data) {
		return nil
	}

	instances := make([]ownFvarInstance, 0, instanceCount)
	for i := range int(instanceCount) {
		off := instanceStart + i*instanceSize
		if off+4+axisCount*4 > len(data) {
			break
		}

		inst := ownFvarInstance{
			subfamilyNameID: binary.BigEndian.Uint16(data[off : off+2]),
			coordinates:     make([]float32, axisCount),
		}

		for j := range axisCount {
			coordOff := off + 4 + j*4
			inst.coordinates[j] = fixed1616ToFloat32(data[coordOff:])
		}

		instances = append(instances, inst)
	}

	return instances
}

// lookupNameTableEntry finds a name entry by nameID in a raw name table.
// Returns the first match found (preferring platform 3 = Windows, encoding 1 = Unicode BMP).
func lookupNameTableEntry(nameData []byte, nameID uint16) string {
	if len(nameData) < 6 {
		return ""
	}

	count := binary.BigEndian.Uint16(nameData[2:4])
	storageOff := binary.BigEndian.Uint16(nameData[4:6])

	for i := range int(count) {
		recOff := 6 + i*12
		if recOff+12 > len(nameData) {
			break
		}

		recNameID := binary.BigEndian.Uint16(nameData[recOff+6 : recOff+8])
		if recNameID != nameID {
			continue
		}

		platformID := binary.BigEndian.Uint16(nameData[recOff : recOff+2])
		encodingID := binary.BigEndian.Uint16(nameData[recOff+2 : recOff+4])
		length := binary.BigEndian.Uint16(nameData[recOff+8 : recOff+10])
		offset := binary.BigEndian.Uint16(nameData[recOff+10 : recOff+12])

		strStart := int(storageOff) + int(offset)
		strEnd := strStart + int(length)
		if strEnd > len(nameData) {
			continue
		}

		raw := nameData[strStart:strEnd]

		// Platform 3 (Windows), Encoding 1 (Unicode BMP): UTF-16BE.
		if platformID == 3 && encodingID == 1 {
			return decodeUTF16BE(raw)
		}
		// Platform 1 (Macintosh), Encoding 0 (Roman): ASCII/Latin-1.
		if platformID == 1 && encodingID == 0 {
			return string(raw)
		}
		// Platform 0 (Unicode): UTF-16BE.
		if platformID == 0 {
			return decodeUTF16BE(raw)
		}
	}

	return ""
}

// axisNameFromTag returns a human-readable name for a variation axis tag.
// Uses well-known registered axis names, falling back to the tag string.
func axisNameFromTag(tag [4]byte) string {
	switch tag {
	case [4]byte{'w', 'g', 'h', 't'}:
		return "Weight"
	case [4]byte{'w', 'd', 't', 'h'}:
		return "Width"
	case [4]byte{'i', 't', 'a', 'l'}:
		return "Italic"
	case [4]byte{'s', 'l', 'n', 't'}:
		return "Slant"
	case [4]byte{'o', 'p', 's', 'z'}:
		return "Optical Size"
	default:
		return string(tag[:])
	}
}

// copyCheck panics if FontSource was copied by value.
// This is the Ebitengine pattern for preventing accidental copies.
func (s *FontSource) copyCheck() {
	if s.addr != s {
		panic("text: FontSource must not be copied by value")
	}
}

// extractFontName extracts the font family name from the parsed font.
func extractFontName(parsed ParsedFont) string {
	// Try to get the family name
	if name := parsed.Name(); name != "" {
		return name
	}

	// Try full name as fallback
	if fullName := parsed.FullName(); fullName != "" {
		return fullName
	}

	// Fallback
	return "Unknown Font"
}
