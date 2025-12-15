package text

// SourceOption configures FontSource creation.
type SourceOption func(*sourceConfig)

// sourceConfig holds configuration for FontSource.
type sourceConfig struct {
	cacheLimit int
	parserName string
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
