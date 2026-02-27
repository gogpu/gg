package text

import (
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

	// Get parser and parse the font
	parser := getParser(config.parserName)
	parsed, err := parser.Parse(data)
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
