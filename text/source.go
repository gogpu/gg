package text

import (
	"bytes"
	"fmt"
	"os"
	"sync"

	gotextFont "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
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

	fvarTable, _ := s.parseFvar()
	return len(fvarTable.Axis) > 0
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

	fvarTable, nameTable := s.parseFvar()
	if len(fvarTable.Axis) == 0 {
		return nil
	}

	axes := make([]VariationAxis, len(fvarTable.Axis))
	for i, axis := range fvarTable.Axis {
		tag := axis.Tag
		axes[i] = VariationAxis{
			Tag:     tagToBytes(tag),
			Name:    axisName(tag, axis, nameTable),
			Minimum: axis.Minimum,
			Default: axis.Default,
			Maximum: axis.Maximum,
		}
	}

	return axes
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

	fvarTable, nameTable := s.parseFvar()
	if len(fvarTable.Instances) == 0 {
		return nil
	}

	var instances []NamedInstance
	for _, inst := range fvarTable.Instances {
		name := ""
		if nameTable != nil && inst.SubfamilyNameID != 0 {
			name = nameTable.Name(tables.NameID(inst.SubfamilyNameID))
		}
		if name == "" {
			continue
		}

		var vars []FontVariation
		for j, coord := range inst.Coordinates {
			if j < len(fvarTable.Axis) {
				vars = append(vars, FontVariation{
					Tag:   tagToBytes(fvarTable.Axis[j].Tag),
					Value: coord,
				})
			}
		}

		instances = append(instances, NamedInstance{
			Name:       name,
			Variations: vars,
		})
	}

	return instances
}

// parseFvar parses the fvar and name tables from the raw font data.
// Returns zero values on error (non-variable font or parse failure).
// Caller must hold s.mu (at least RLock).
func (s *FontSource) parseFvar() (tables.FvarRecords, *tables.Name) {
	reader := bytes.NewReader(s.data)
	ld, err := ot.NewLoader(reader)
	if err != nil {
		return tables.FvarRecords{}, nil
	}

	// Parse fvar table.
	raw, err := ld.RawTable(ot.MustNewTag("fvar"))
	if err != nil {
		return tables.FvarRecords{}, nil
	}

	fvar, _, err := tables.ParseFvar(raw)
	if err != nil {
		return tables.FvarRecords{}, nil
	}

	// Parse name table for axis/instance names.
	var nameTable *tables.Name
	nameRaw, err := ld.RawTable(ot.MustNewTag("name"))
	if err == nil {
		nt, _, ntErr := tables.ParseName(nameRaw)
		if ntErr == nil {
			nameTable = &nt
		}
	}

	return fvar.FvarRecords, nameTable
}

// tagToBytes converts an OpenType tag (uint32) to [4]byte.
func tagToBytes(tag gotextFont.Tag) [4]byte {
	return [4]byte{
		byte(tag >> 24),
		byte(tag >> 16),
		byte(tag >> 8),
		byte(tag),
	}
}

// axisName returns a human-readable name for a variation axis.
// It first tries the font's name table, then falls back to well-known axis names.
func axisName(tag gotextFont.Tag, _ tables.VariationAxisRecord, nameTable *tables.Name) string {
	// The strid field on VariationAxisRecord is unexported in go-text,
	// so we cannot look up per-axis names from the name table.
	// Use well-known registered axis names as the primary source.
	_ = nameTable // Future: if strid becomes exported, look up from name table.

	switch tag {
	case ot.MustNewTag("wght"):
		return "Weight"
	case ot.MustNewTag("wdth"):
		return "Width"
	case ot.MustNewTag("ital"):
		return "Italic"
	case ot.MustNewTag("slnt"):
		return "Slant"
	case ot.MustNewTag("opsz"):
		return "Optical Size"
	default:
		// Return the tag as a readable 4-char string.
		b := tagToBytes(tag)
		return string(b[:])
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
