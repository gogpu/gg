// Package text provides GPU text rendering infrastructure.
package text

import (
	"image/color"
	"sync"
)

// RenderParams holds parameters for glyph rendering.
type RenderParams struct {
	// Transform is an optional affine transformation to apply to all glyphs.
	// If nil, identity transform is used.
	Transform *AffineTransform

	// Color is the fill color for glyphs.
	Color color.RGBA

	// Opacity is the overall opacity [0, 1].
	// 1.0 means fully opaque, 0.0 means fully transparent.
	Opacity float64
}

// DefaultRenderParams returns default rendering parameters.
func DefaultRenderParams() RenderParams {
	return RenderParams{
		Transform: nil,
		Color:     color.RGBA{R: 0, G: 0, B: 0, A: 255},
		Opacity:   1.0,
	}
}

// WithColor returns a copy of params with the given color.
func (p RenderParams) WithColor(c color.RGBA) RenderParams {
	p.Color = c
	return p
}

// WithOpacity returns a copy of params with the given opacity.
func (p RenderParams) WithOpacity(opacity float64) RenderParams {
	p.Opacity = clampOpacity(opacity)
	return p
}

// WithTransform returns a copy of params with the given transform.
func (p RenderParams) WithTransform(t *AffineTransform) RenderParams {
	p.Transform = t
	return p
}

// GlyphRenderer converts shaped glyphs to outline paths.
// It uses the GlyphCache for efficient outline caching and
// OutlineExtractor for extracting glyph outlines from fonts.
//
// GlyphRenderer is safe for concurrent use.
type GlyphRenderer struct {
	cache     *GlyphCache
	extractor *OutlineExtractor

	mu sync.RWMutex
}

// NewGlyphRenderer creates a new glyph renderer with the global cache.
func NewGlyphRenderer() *GlyphRenderer {
	return &GlyphRenderer{
		cache:     GetGlobalGlyphCache(),
		extractor: NewOutlineExtractor(),
	}
}

// NewGlyphRendererWithCache creates a new glyph renderer with a custom cache.
func NewGlyphRendererWithCache(cache *GlyphCache) *GlyphRenderer {
	if cache == nil {
		cache = GetGlobalGlyphCache()
	}
	return &GlyphRenderer{
		cache:     cache,
		extractor: NewOutlineExtractor(),
	}
}

// Cache returns the glyph cache used by this renderer.
func (r *GlyphRenderer) Cache() *GlyphCache {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cache
}

// SetCache sets the glyph cache used by this renderer.
func (r *GlyphRenderer) SetCache(cache *GlyphCache) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cache == nil {
		cache = GetGlobalGlyphCache()
	}
	r.cache = cache
}

// RenderGlyph renders a single glyph to an outline.
// Returns the glyph outline with positioning applied.
//
// Parameters:
//   - glyph: The shaped glyph to render
//   - font: The parsed font to use for outline extraction
//   - size: Font size in pixels (ppem)
//   - params: Rendering parameters
//
// Returns the transformed outline, or nil if the glyph has no outline.
func (r *GlyphRenderer) RenderGlyph(
	glyph *ShapedGlyph,
	font ParsedFont,
	size float64,
	params RenderParams,
) *GlyphOutline {
	if glyph == nil || font == nil {
		return nil
	}

	// Get font ID for cache key
	fontID := computeFontID(font)

	// Build cache key
	key := OutlineCacheKey{
		FontID:  fontID,
		GID:     glyph.GID,
		Size:    sizeToInt16(size),
		Hinting: HintingNone,
	}

	// Get or create outline
	outline := r.cache.GetOrCreate(key, func() *GlyphOutline {
		o, err := r.extractor.ExtractOutline(font, glyph.GID, size)
		if err != nil {
			return nil
		}
		return o
	})

	// No outline means nothing to draw (e.g., space character)
	if outline == nil || outline.IsEmpty() {
		return nil
	}

	// Apply transformation if needed
	return r.transformOutline(outline, glyph, params)
}

// RenderGlyphs renders multiple glyphs to outlines.
// Returns a slice of outlines corresponding to each input glyph.
// Glyphs with no outline (e.g., spaces) will have nil entries.
func (r *GlyphRenderer) RenderGlyphs(
	glyphs []ShapedGlyph,
	font ParsedFont,
	size float64,
	params RenderParams,
) []*GlyphOutline {
	if len(glyphs) == 0 || font == nil {
		return nil
	}

	outlines := make([]*GlyphOutline, len(glyphs))
	for i := range glyphs {
		outlines[i] = r.RenderGlyph(&glyphs[i], font, size, params)
	}

	return outlines
}

// RenderRun renders a shaped run to outlines.
func (r *GlyphRenderer) RenderRun(run *ShapedRun, params RenderParams) []*GlyphOutline {
	if run == nil || len(run.Glyphs) == 0 || run.Face == nil {
		return nil
	}

	font := run.Face.Source().Parsed()
	if font == nil {
		return nil
	}

	return r.RenderGlyphs(run.Glyphs, font, run.Size, params)
}

// RenderLayout renders a complete layout to outlines.
// Returns a 2D slice where [line][glyph] contains the outlines.
func (r *GlyphRenderer) RenderLayout(layout *Layout, params RenderParams) [][]*GlyphOutline {
	if layout == nil || len(layout.Lines) == 0 {
		return nil
	}

	result := make([][]*GlyphOutline, len(layout.Lines))
	for i := range layout.Lines {
		line := &layout.Lines[i]
		lineOutlines := make([]*GlyphOutline, 0, len(line.Glyphs))

		// Apply line Y offset to transform
		lineParams := params
		if params.Transform != nil {
			lineTranslate := TranslateTransform(0, float32(line.Y))
			lineParams.Transform = params.Transform.Multiply(lineTranslate)
		} else {
			lineParams.Transform = TranslateTransform(0, float32(line.Y))
		}

		// Render runs in this line
		for j := range line.Runs {
			run := &line.Runs[j]
			runOutlines := r.RenderRun(run, lineParams)
			lineOutlines = append(lineOutlines, runOutlines...)
		}

		result[i] = lineOutlines
	}

	return result
}

// transformOutline applies glyph positioning and user transform to an outline.
func (r *GlyphRenderer) transformOutline(
	outline *GlyphOutline,
	glyph *ShapedGlyph,
	params RenderParams,
) *GlyphOutline {
	if outline == nil {
		return nil
	}

	// Calculate the combined transform:
	// 1. Y-flip (fonts have Y up, screen has Y down)
	// 2. Translation to glyph position
	// 3. User transform (if any)

	// Create glyph positioning transform
	// y' = -y (flip), then translate
	glyphTransform := &AffineTransform{
		A:  1,
		B:  0,
		C:  0,
		D:  -1, // Y-flip
		Tx: float32(glyph.X),
		Ty: float32(glyph.Y),
	}

	// Combine with user transform
	var finalTransform *AffineTransform
	if params.Transform != nil {
		finalTransform = params.Transform.Multiply(glyphTransform)
	} else {
		finalTransform = glyphTransform
	}

	// Transform the outline
	return outline.Transform(finalTransform)
}

// computeFontID generates a unique ID for a font based on its properties.
// This uses FNV-1a hash of font name and number of glyphs.
func computeFontID(font ParsedFont) uint64 {
	const (
		fnvOffset = 14695981039346656037
		fnvPrime  = 1099511628211
	)

	hash := uint64(fnvOffset)

	// Hash the font name
	name := font.Name()
	for i := 0; i < len(name); i++ {
		hash ^= uint64(name[i])
		hash *= fnvPrime
	}

	// Hash the full name
	fullName := font.FullName()
	for i := 0; i < len(fullName); i++ {
		hash ^= uint64(fullName[i])
		hash *= fnvPrime
	}

	// Hash number of glyphs (always non-negative)
	hash ^= uint64(font.NumGlyphs()) //nolint:gosec // NumGlyphs is always non-negative
	hash *= fnvPrime

	// Hash units per em (always non-negative)
	hash ^= uint64(font.UnitsPerEm()) //nolint:gosec // UnitsPerEm is always non-negative
	hash *= fnvPrime

	return hash
}

// sizeToInt16 converts a float64 size to int16 for cache key.
// Sizes are clamped to the valid int16 range.
func sizeToInt16(size float64) int16 {
	if size < 0 {
		size = 0
	}
	if size > 32767 {
		size = 32767
	}
	return int16(size) //#nosec G115 -- bounds checked above
}

// clampOpacity clamps opacity to [0, 1] range.
func clampOpacity(opacity float64) float64 {
	if opacity < 0 {
		return 0
	}
	if opacity > 1 {
		return 1
	}
	return opacity
}

// RotateTransform creates a rotation transformation (angle in radians).
func RotateTransform(angle float32) *AffineTransform {
	sin := float32(sinf64(float64(angle)))
	cos := float32(cosf64(float64(angle)))
	return &AffineTransform{
		A: cos, B: -sin,
		C: sin, D: cos,
		Tx: 0, Ty: 0,
	}
}

// ScaleTransformXY creates a non-uniform scaling transformation.
func ScaleTransformXY(sx, sy float32) *AffineTransform {
	return &AffineTransform{A: sx, D: sy}
}

// sinf64 returns the sine of x.
func sinf64(x float64) float64 {
	if x < 0.001 && x > -0.001 {
		return x
	}
	return sineApprox(x)
}

// cosf64 returns the cosine of x.
func cosf64(x float64) float64 {
	if x < 0.001 && x > -0.001 {
		return 1.0
	}
	return cosineApprox(x)
}

// sineApprox approximates sin(x) using Taylor series.
func sineApprox(x float64) float64 {
	// Normalize to [-pi, pi]
	const twoPi = 6.283185307179586
	const pi = 3.141592653589793

	for x > pi {
		x -= twoPi
	}
	for x < -pi {
		x += twoPi
	}

	// Taylor series: sin(x) = x - x^3/3! + x^5/5! - x^7/7! + ...
	x2 := x * x
	x3 := x2 * x
	x5 := x3 * x2
	x7 := x5 * x2
	x9 := x7 * x2

	return x - x3/6.0 + x5/120.0 - x7/5040.0 + x9/362880.0
}

// cosineApprox approximates cos(x) using Taylor series.
func cosineApprox(x float64) float64 {
	// Normalize to [-pi, pi]
	const twoPi = 6.283185307179586
	const pi = 3.141592653589793

	for x > pi {
		x -= twoPi
	}
	for x < -pi {
		x += twoPi
	}

	// Taylor series: cos(x) = 1 - x^2/2! + x^4/4! - x^6/6! + x^8/8! - ...
	x2 := x * x
	x4 := x2 * x2
	x6 := x4 * x2
	x8 := x6 * x2

	return 1.0 - x2/2.0 + x4/24.0 - x6/720.0 + x8/40320.0
}

// TextRenderer provides a high-level API for text rendering.
// It combines shaping and glyph outline extraction.
//
// Note: For rendering to a scene.Scene, use scene.TextRenderer instead,
// which provides direct scene integration.
type TextRenderer struct {
	glyphRenderer *GlyphRenderer
	defaultFace   Face
	defaultSize   float64
	defaultColor  color.RGBA
}

// NewTextRenderer creates a new text renderer.
func NewTextRenderer() *TextRenderer {
	return &TextRenderer{
		glyphRenderer: NewGlyphRenderer(),
		defaultSize:   16.0,
		defaultColor:  color.RGBA{R: 0, G: 0, B: 0, A: 255},
	}
}

// SetDefaultFace sets the default font face for rendering.
func (tr *TextRenderer) SetDefaultFace(face Face) {
	tr.defaultFace = face
}

// SetDefaultSize sets the default font size.
func (tr *TextRenderer) SetDefaultSize(size float64) {
	if size > 0 {
		tr.defaultSize = size
	}
}

// SetDefaultColor sets the default text color.
func (tr *TextRenderer) SetDefaultColor(c color.RGBA) {
	tr.defaultColor = c
}

// ShapeAndRender shapes text and returns the glyph outlines.
func (tr *TextRenderer) ShapeAndRender(text string) ([]*GlyphOutline, error) {
	if tr.defaultFace == nil {
		return nil, ErrUnsupportedFontType
	}

	// Shape the text
	glyphs := Shape(text, tr.defaultFace, tr.defaultSize)
	if len(glyphs) == 0 {
		return nil, nil
	}

	// Get the parsed font
	font := tr.defaultFace.Source().Parsed()
	if font == nil {
		return nil, ErrUnsupportedFontType
	}

	params := RenderParams{
		Color:   tr.defaultColor,
		Opacity: 1.0,
	}

	return tr.glyphRenderer.RenderGlyphs(glyphs, font, tr.defaultSize, params), nil
}

// ShapeAndRenderAt shapes text and returns outlines at the specified position.
func (tr *TextRenderer) ShapeAndRenderAt(text string, x, y float64) ([]*GlyphOutline, error) {
	if tr.defaultFace == nil {
		return nil, ErrUnsupportedFontType
	}

	// Shape the text
	glyphs := Shape(text, tr.defaultFace, tr.defaultSize)
	if len(glyphs) == 0 {
		return nil, nil
	}

	// Get the parsed font
	font := tr.defaultFace.Source().Parsed()
	if font == nil {
		return nil, ErrUnsupportedFontType
	}

	params := RenderParams{
		Transform: TranslateTransform(float32(x), float32(y)),
		Color:     tr.defaultColor,
		Opacity:   1.0,
	}

	return tr.glyphRenderer.RenderGlyphs(glyphs, font, tr.defaultSize, params), nil
}

// GlyphRenderer returns the underlying glyph renderer.
func (tr *TextRenderer) GlyphRenderer() *GlyphRenderer {
	return tr.glyphRenderer
}

// globalTextRenderer is a shared text renderer instance.
var globalTextRenderer = NewTextRenderer()

// GetGlobalTextRenderer returns the global shared text renderer.
func GetGlobalTextRenderer() *TextRenderer {
	return globalTextRenderer
}
