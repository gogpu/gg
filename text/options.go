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

// NewFontFeature creates a FontFeature from a 4-character OpenType tag string
// and a value. The tag must be exactly 4 ASCII characters (e.g., "tnum", "liga",
// "smcp"). Panics if the tag is not exactly 4 bytes.
//
// Example:
//
//	tnum := text.NewFontFeature("tnum", 1)  // enable tabular nums
//	liga := text.NewFontFeature("liga", 0)  // disable ligatures
func NewFontFeature(tag string, value uint32) FontFeature {
	if len(tag) != 4 {
		panic("text.NewFontFeature: tag must be exactly 4 bytes, got " + tag)
	}
	return FontFeature{
		Tag:   [4]byte{tag[0], tag[1], tag[2], tag[3]},
		Value: value,
	}
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

	// NoDLigatures disables discretionary ligatures.
	// Some fonts (e.g. Times New Roman) place fi/fl ligatures under 'dlig'
	// rather than 'liga'. The shaper enables 'dlig' by default for
	// compatibility. Use this constant to disable discretionary ligatures
	// when strict HarfBuzz-compatible behavior is needed.
	NoDLigatures = FontFeature{Tag: [4]byte{'d', 'l', 'i', 'g'}, Value: 0}

	// Kerning enables kerning (pair-wise glyph spacing adjustment).
	// Kerning is enabled by default in most OpenType fonts; this constant
	// is useful for explicitly requesting it when combined with other features.
	Kerning = FontFeature{Tag: [4]byte{'k', 'e', 'r', 'n'}, Value: 1}

	// NoKerning disables kerning.
	NoKerning = FontFeature{Tag: [4]byte{'k', 'e', 'r', 'n'}, Value: 0}

	// SmallCaps enables small capitals substitution.
	// Lowercase letters are replaced with small capital forms.
	SmallCaps = FontFeature{Tag: [4]byte{'s', 'm', 'c', 'p'}, Value: 1}

	// OldstyleNums enables oldstyle (text) figures.
	// Digits have varying heights and descenders (3, 4, 5, 7, 9 descend),
	// matching the visual rhythm of body text.
	OldstyleNums = FontFeature{Tag: [4]byte{'o', 'n', 'u', 'm'}, Value: 1}
)

// FontVariation sets a font variation axis value for variable fonts.
// Variable fonts define design-space axes (weight, width, slant, etc.)
// that allow continuous interpolation between styles.
//
// See https://learn.microsoft.com/en-us/typography/opentype/spec/fvar
type FontVariation struct {
	Tag   [4]byte // OpenType axis tag (e.g., "wght", "wdth", "slnt")
	Value float32 // Design-space value (e.g., 700 for Bold weight)
}

// NewFontVariation creates a FontVariation from a 4-character axis tag string
// and a design-space value. The tag must be exactly 4 ASCII characters
// (e.g., "wght", "wdth", "slnt"). Panics if the tag is not exactly 4 bytes.
//
// Example:
//
//	bold := text.NewFontVariation("wght", 700)
//	wide := text.NewFontVariation("wdth", 125)
func NewFontVariation(tag string, value float32) FontVariation {
	if len(tag) != 4 {
		panic("text.NewFontVariation: tag must be exactly 4 bytes, got " + tag)
	}
	return FontVariation{
		Tag:   [4]byte{tag[0], tag[1], tag[2], tag[3]},
		Value: value,
	}
}

// Standard registered axis tag constants.
// These are the five axes defined in the OpenType specification.
// Custom fonts may define additional axes.
//
// See https://learn.microsoft.com/en-us/typography/opentype/spec/dvaraxistag
var (
	// AxisWeight is the "wght" axis — controls stroke thickness.
	// Typical range: 100 (Thin) to 900 (Black). Default: 400 (Regular).
	AxisWeight = [4]byte{'w', 'g', 'h', 't'}

	// AxisWidth is the "wdth" axis — controls horizontal character width.
	// Typical range: 75 (Condensed) to 125 (Expanded). Default: 100 (Normal).
	AxisWidth = [4]byte{'w', 'd', 't', 'h'}

	// AxisItalic is the "ital" axis — selects italic style.
	// Values: 0 (Upright) or 1 (Italic). Default: 0.
	AxisItalic = [4]byte{'i', 't', 'a', 'l'}

	// AxisSlant is the "slnt" axis — controls oblique angle in degrees.
	// Typical range: -90 to 90. Default: 0.
	AxisSlant = [4]byte{'s', 'l', 'n', 't'}

	// AxisOpticalSize is the "opsz" axis — adjusts for display size.
	// Typical range: 6 to 144 (points). Default: varies by font.
	AxisOpticalSize = [4]byte{'o', 'p', 's', 'z'}
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
// The default is "own" which uses Pure Go binary parsing (ADR-048).
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
	direction  Direction
	hinting    Hinting
	language   string
	features   []FontFeature   // OpenType features (tnum, liga, etc.)
	variations []FontVariation // Font variation axes (wght, wdth, etc.)
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
// Features are applied during shaping when using [OwnShaper] (the default).
// The [BuiltinShaper] ignores features since it does not perform
// OpenType shaping.
//
// Note: Features affect shaped output via [OwnShaper] only. Methods like
// [Face.Advance] and [Face.Glyphs] use raw glyph metrics without shaping.
//
// Example — enable tabular figures for aligned numeric columns:
//
//	face := source.Face(12, text.WithFeatures(text.TabularNums))
func WithFeatures(features ...FontFeature) FaceOption {
	return func(c *faceConfig) {
		c.features = features
	}
}

// WithVariations sets font variation axis values for the face.
// Variations are applied to both shaping and rendering via the own parser,
// which handles gvar/HVAR/avar interpolation for TrueType variable fonts.
// Static fonts ignore variations.
//
// Variable fonts define continuous design axes (weight, width, slant, etc.)
// that allow smooth interpolation between styles without separate font files.
// Use [FontSource.IsVariable] to check if a font supports variations, and
// [FontSource.VariationAxes] to discover available axes and their ranges.
//
// Example — set weight to Bold (700) and width to Condensed (75):
//
//	face := source.Face(16, text.WithVariations(
//	    text.NewFontVariation("wght", 700),
//	    text.NewFontVariation("wdth", 75),
//	))
func WithVariations(variations ...FontVariation) FaceOption {
	return func(c *faceConfig) {
		c.variations = variations
	}
}
