package text

// FontFeature enables or disables an OpenType font feature.
// Features are identified by a 4-byte tag (e.g., "tnum" for tabular figures)
// and controlled by a uint32 value (1 = enable, 0 = disable).
//
// See https://learn.microsoft.com/en-us/typography/opentype/spec/featurelist
type FontFeature struct {
	Tag   [4]byte // e.g., [4]byte{'t','n','u','m'}
	Value uint32  // 1 = enable, 0 = disable
}

// Predefined font feature constants for common use cases.
var (
	// TabularNums enables tabular (monospaced) digit widths.
	// This ensures that digits like 1 and 0 occupy the same horizontal space,
	// which is essential for aligned numeric columns (e.g., axis tick labels).
	TabularNums = FontFeature{Tag: [4]byte{'t', 'n', 'u', 'm'}, Value: 1}

	// ProportionalNums explicitly requests proportional (variable-width) digit widths.
	// This is the default for most fonts, but can be used to override a face
	// that was configured with TabularNums.
	ProportionalNums = FontFeature{Tag: [4]byte{'p', 'n', 'u', 'm'}, Value: 1}

	// NoLigatures disables standard ligatures (fi, fl, ffi, etc.).
	NoLigatures = FontFeature{Tag: [4]byte{'l', 'i', 'g', 'a'}, Value: 0}
)

// SourceOption configures FontSource creation.
type SourceOption func(*sourceConfig)

// sourceConfig holds configuration for FontSource.
type sourceConfig struct {
	cacheLimit      int
	parserName      string
	collectionIndex int // Font index within .ttc/.otc collection (default 0)
}

// defaultSourceConfig returns the default source configuration.
func defaultSourceConfig() sourceConfig {
	return sourceConfig{
		cacheLimit: 512,               // Default cache limit
		parserName: defaultParserName, // Default parser (ximage)
	}
}

// WithCacheLimit sets the maximum number of cached glyphs.
// A value of 0 disables the cache limit.
func WithCacheLimit(n int) SourceOption {
	return func(c *sourceConfig) {
		c.cacheLimit = n
	}
}

// WithCollectionIndex selects a font within a TrueType/OpenType collection
// (.ttc/.otc). Index 0 is the first font (default). Ignored for single fonts.
//
// Example: msyh.ttc contains Microsoft YaHei (0) and Microsoft YaHei UI (1).
func WithCollectionIndex(index int) SourceOption {
	return func(c *sourceConfig) {
		c.collectionIndex = index
	}
}

// WithParser specifies the font parser backend.
// The default is "ximage" which uses golang.org/x/image/font/opentype.
//
// Custom parsers can be registered with RegisterParser.
// This allows using alternative font parsing libraries or
// a pure Go implementation in the future.
func WithParser(name string) SourceOption {
	return func(c *sourceConfig) {
		c.parserName = name
	}
}

// FaceOption configures Face creation.
type FaceOption func(*faceConfig)

// faceConfig holds configuration for Face.
type faceConfig struct {
	direction Direction
	hinting   Hinting
	language  string
	features  []FontFeature // OpenType features (tnum, liga, etc.)
}

// defaultFaceConfig returns the default face configuration.
func defaultFaceConfig() faceConfig {
	return faceConfig{
		direction: DirectionLTR,
		hinting:   HintingFull,
		language:  "en",
	}
}

// WithDirection sets the text direction for the face.
func WithDirection(d Direction) FaceOption {
	return func(c *faceConfig) {
		c.direction = d
	}
}

// WithHinting sets the hinting mode for the face.
func WithHinting(h Hinting) FaceOption {
	return func(c *faceConfig) {
		c.hinting = h
	}
}

// WithLanguage sets the language tag for the face (e.g., "en", "ja", "ar").
func WithLanguage(lang string) FaceOption {
	return func(c *faceConfig) {
		c.language = lang
	}
}

// WithFeatures sets OpenType font features for the face.
// Features are applied during shaping when using [GoTextShaper].
// The [BuiltinShaper] ignores features since it does not perform
// OpenType shaping.
//
// Example — enable tabular figures for aligned numeric columns:
//
//	face := source.Face(12, text.WithFeatures(text.TabularNums))
func WithFeatures(features ...FontFeature) FaceOption {
	return func(c *faceConfig) {
		c.features = features
	}
}
