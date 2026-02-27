package scene

import (
	"sync"

	"github.com/gogpu/gg/text"
)

// TextRenderer converts shaped text to scene paths for GPU rendering.
// It supports glyph outlines and handles positioning, transforms, and caching.
//
// TextRenderer is safe for concurrent use.
type TextRenderer struct {
	mu sync.RWMutex

	// extractor extracts glyph outlines from fonts
	extractor *text.OutlineExtractor

	// pathPool reuses Path objects to reduce allocations
	pathPool *PathPool

	// config holds renderer configuration
	config TextRendererConfig
}

// TextRendererConfig holds configuration for TextRenderer.
type TextRendererConfig struct {
	// FlipY inverts the Y-axis for rendering (default: true).
	// Fonts define Y as increasing upward, but screen coordinates
	// typically have Y increasing downward.
	FlipY bool

	// SubpixelPositioning enables fractional glyph positioning.
	// When false, glyphs are snapped to integer pixel positions.
	SubpixelPositioning bool

	// HintingEnabled enables font hinting for sharper rendering at small sizes.
	HintingEnabled bool
}

// DefaultTextRendererConfig returns the default configuration.
func DefaultTextRendererConfig() TextRendererConfig {
	return TextRendererConfig{
		FlipY:               true,
		SubpixelPositioning: true,
		HintingEnabled:      false,
	}
}

// NewTextRenderer creates a new text renderer with default configuration.
func NewTextRenderer() *TextRenderer {
	return NewTextRendererWithConfig(DefaultTextRendererConfig())
}

// NewTextRendererWithConfig creates a new text renderer with the given configuration.
func NewTextRendererWithConfig(config TextRendererConfig) *TextRenderer {
	return &TextRenderer{
		extractor: text.NewOutlineExtractor(),
		pathPool:  NewPathPool(),
		config:    config,
	}
}

// Config returns the current configuration.
func (r *TextRenderer) Config() TextRendererConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// SetConfig updates the configuration.
func (r *TextRenderer) SetConfig(config TextRendererConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config = config
}

// RenderedGlyph represents a glyph that has been converted to a path.
type RenderedGlyph struct {
	// Path is the vector path representing the glyph outline.
	// May be nil for non-outline glyphs (bitmap, empty).
	Path *Path

	// Bounds is the bounding box of the rendered glyph.
	Bounds Rect

	// Advance is the distance to the next glyph position.
	Advance float32

	// Position is the glyph position relative to the text origin.
	X, Y float32

	// GID is the glyph ID.
	GID text.GlyphID

	// Type indicates the glyph type.
	Type text.GlyphType

	// Cluster is the source character index.
	Cluster int
}

// RenderGlyph converts a single shaped glyph to a renderable path.
// The path is positioned at the glyph's location and scaled appropriately.
func (r *TextRenderer) RenderGlyph(glyph text.ShapedGlyph, face text.Face) (*RenderedGlyph, error) {
	r.mu.RLock()
	config := r.config
	r.mu.RUnlock()

	// Get the font source and size
	source := face.Source()
	if source == nil {
		return nil, &text.FontError{Reason: "face has no font source"}
	}
	size := face.Size()

	// Extract the outline
	outline, err := r.extractor.ExtractOutline(source.Parsed(), glyph.GID, size)
	if err != nil {
		return nil, err
	}

	// Create the rendered glyph
	rendered := &RenderedGlyph{
		Advance: float32(glyph.XAdvance),
		X:       float32(glyph.X),
		Y:       float32(glyph.Y),
		GID:     glyph.GID,
		Type:    text.GlyphTypeOutline,
		Cluster: glyph.Cluster,
	}

	// Handle empty outlines (like space)
	if outline == nil || outline.IsEmpty() {
		rendered.Path = nil
		rendered.Bounds = EmptyRect()
		return rendered, nil
	}

	// Convert outline to path with positioning
	path := r.outlineToPath(outline, float32(glyph.X), float32(glyph.Y), config)
	rendered.Path = path
	rendered.Bounds = path.Bounds()

	return rendered, nil
}

// RenderGlyphs converts multiple shaped glyphs to renderable paths.
// Returns a slice of rendered glyphs in the same order as input.
func (r *TextRenderer) RenderGlyphs(glyphs []text.ShapedGlyph, face text.Face) ([]*RenderedGlyph, error) {
	if len(glyphs) == 0 {
		return nil, nil
	}

	r.mu.RLock()
	config := r.config
	r.mu.RUnlock()

	// Get the font source and size
	source := face.Source()
	if source == nil {
		return nil, &text.FontError{Reason: "face has no font source"}
	}
	size := face.Size()
	parsed := source.Parsed()

	// Render all glyphs
	rendered := make([]*RenderedGlyph, len(glyphs))
	for i, glyph := range glyphs {
		rg := &RenderedGlyph{
			Advance: float32(glyph.XAdvance),
			X:       float32(glyph.X),
			Y:       float32(glyph.Y),
			GID:     glyph.GID,
			Type:    text.GlyphTypeOutline,
			Cluster: glyph.Cluster,
		}

		// Extract outline
		outline, err := r.extractor.ExtractOutline(parsed, glyph.GID, size)
		if err != nil {
			// Log error but continue - some glyphs may not have outlines
			rg.Path = nil
			rg.Bounds = EmptyRect()
			rendered[i] = rg
			continue
		}

		// Handle empty outlines
		if outline == nil || outline.IsEmpty() {
			rg.Path = nil
			rg.Bounds = EmptyRect()
			rendered[i] = rg
			continue
		}

		// Convert to path
		path := r.outlineToPath(outline, float32(glyph.X), float32(glyph.Y), config)
		rg.Path = path
		rg.Bounds = path.Bounds()
		rendered[i] = rg
	}

	return rendered, nil
}

// RenderRun converts a shaped run to renderable glyphs.
func (r *TextRenderer) RenderRun(run *text.ShapedRun) ([]*RenderedGlyph, error) {
	if run == nil || len(run.Glyphs) == 0 {
		return nil, nil
	}
	return r.RenderGlyphs(run.Glyphs, run.Face)
}

// RenderText shapes and renders text in one operation.
// Uses the global shaper for text shaping.
func (r *TextRenderer) RenderText(str string, face text.Face) ([]*RenderedGlyph, error) {
	if str == "" {
		return nil, nil
	}

	// Shape the text
	shaped := text.Shape(str, face)
	if len(shaped) == 0 {
		return nil, nil
	}

	return r.RenderGlyphs(shaped, face)
}

// RenderToScene renders shaped glyphs directly into a scene.
// This is the primary method for GPU text rendering.
func (r *TextRenderer) RenderToScene(s *Scene, glyphs []text.ShapedGlyph, face text.Face, brush Brush) error {
	rendered, err := r.RenderGlyphs(glyphs, face)
	if err != nil {
		return err
	}

	for _, rg := range rendered {
		if rg.Path != nil && !rg.Path.IsEmpty() {
			s.Fill(FillNonZero, IdentityAffine(), brush, NewPathShape(rg.Path))
		}
	}

	return nil
}

// RenderTextToScene shapes and renders text directly into a scene.
func (r *TextRenderer) RenderTextToScene(s *Scene, str string, face text.Face, x, y float32, brush Brush) error {
	if str == "" {
		return nil
	}

	// Shape the text
	shaped := text.Shape(str, face)
	if len(shaped) == 0 {
		return nil
	}

	r.mu.RLock()
	config := r.config
	r.mu.RUnlock()

	// Get font info
	source := face.Source()
	if source == nil {
		return &text.FontError{Reason: "face has no font source"}
	}
	size := face.Size()
	parsed := source.Parsed()

	// Render each glyph
	for _, glyph := range shaped {
		outline, err := r.extractor.ExtractOutline(parsed, glyph.GID, size)
		if err != nil || outline == nil || outline.IsEmpty() {
			continue
		}

		// Position glyph relative to origin (x, y)
		posX := x + float32(glyph.X)
		posY := y + float32(glyph.Y)

		path := r.outlineToPath(outline, posX, posY, config)
		if path != nil && !path.IsEmpty() {
			s.Fill(FillNonZero, IdentityAffine(), brush, NewPathShape(path))
		}
	}

	return nil
}

// outlineToPath converts a text.GlyphOutline to a scene.Path with positioning.
func (r *TextRenderer) outlineToPath(outline *text.GlyphOutline, x, y float32, config TextRendererConfig) *Path {
	if outline == nil || outline.SegmentCount() == 0 {
		return nil
	}

	path := r.pathPool.Get()

	// Y-axis handling: font Y increases upward, screen Y increases downward
	yMul := float32(1.0)
	if config.FlipY {
		yMul = -1.0
	}

	segments := outline.Segments
	for _, seg := range segments {
		switch seg.Op {
		case text.OutlineOpMoveTo:
			px := x + seg.Points[0].X
			py := y + seg.Points[0].Y*yMul
			path.MoveTo(px, py)

		case text.OutlineOpLineTo:
			px := x + seg.Points[0].X
			py := y + seg.Points[0].Y*yMul
			path.LineTo(px, py)

		case text.OutlineOpQuadTo:
			cx := x + seg.Points[0].X
			cy := y + seg.Points[0].Y*yMul
			px := x + seg.Points[1].X
			py := y + seg.Points[1].Y*yMul
			path.QuadTo(cx, cy, px, py)

		case text.OutlineOpCubicTo:
			c1x := x + seg.Points[0].X
			c1y := y + seg.Points[0].Y*yMul
			c2x := x + seg.Points[1].X
			c2y := y + seg.Points[1].Y*yMul
			px := x + seg.Points[2].X
			py := y + seg.Points[2].Y*yMul
			path.CubicTo(c1x, c1y, c2x, c2y, px, py)
		}
	}

	return path
}

// ToCompositePath combines multiple rendered glyphs into a single path.
// This is useful for text that will be rendered as a single unit.
func (r *TextRenderer) ToCompositePath(glyphs []*RenderedGlyph) *Path {
	if len(glyphs) == 0 {
		return nil
	}

	composite := NewPath()
	for _, rg := range glyphs {
		if rg.Path != nil && !rg.Path.IsEmpty() {
			appendPath(composite, rg.Path)
		}
	}

	return composite
}

// appendPath appends one path to another.
func appendPath(dst, src *Path) {
	if src == nil || src.IsEmpty() {
		return
	}

	srcVerbs := src.Verbs()
	srcPoints := src.Points()
	pointIdx := 0

	for _, verb := range srcVerbs {
		switch verb {
		case VerbMoveTo:
			dst.MoveTo(srcPoints[pointIdx], srcPoints[pointIdx+1])
			pointIdx += 2
		case VerbLineTo:
			dst.LineTo(srcPoints[pointIdx], srcPoints[pointIdx+1])
			pointIdx += 2
		case VerbQuadTo:
			dst.QuadTo(srcPoints[pointIdx], srcPoints[pointIdx+1],
				srcPoints[pointIdx+2], srcPoints[pointIdx+3])
			pointIdx += 4
		case VerbCubicTo:
			dst.CubicTo(srcPoints[pointIdx], srcPoints[pointIdx+1],
				srcPoints[pointIdx+2], srcPoints[pointIdx+3],
				srcPoints[pointIdx+4], srcPoints[pointIdx+5])
			pointIdx += 6
		case VerbClose:
			dst.Close()
		}
	}
}

// TextBounds computes the bounding box for rendered text.
func TextBounds(glyphs []*RenderedGlyph) Rect {
	if len(glyphs) == 0 {
		return EmptyRect()
	}

	bounds := EmptyRect()
	for _, rg := range glyphs {
		if rg.Path != nil {
			bounds = bounds.Union(rg.Bounds)
		}
	}

	return bounds
}

// TextAdvance returns the total advance width of rendered glyphs.
func TextAdvance(glyphs []*RenderedGlyph) float32 {
	if len(glyphs) == 0 {
		return 0
	}

	// The advance is the position of the last glyph plus its advance
	last := glyphs[len(glyphs)-1]
	return last.X + last.Advance
}

// TextRendererPool manages a pool of TextRenderers for concurrent use.
type TextRendererPool struct {
	pool sync.Pool
}

// NewTextRendererPool creates a new pool of text renderers.
func NewTextRendererPool() *TextRendererPool {
	return &TextRendererPool{
		pool: sync.Pool{
			New: func() any {
				return NewTextRenderer()
			},
		},
	}
}

// Get retrieves a renderer from the pool.
func (p *TextRendererPool) Get() *TextRenderer {
	return p.pool.Get().(*TextRenderer)
}

// Put returns a renderer to the pool.
func (p *TextRendererPool) Put(r *TextRenderer) {
	if r != nil {
		p.pool.Put(r)
	}
}

// DrawText is a convenience method on Scene to draw text directly.
// It creates a temporary TextRenderer, renders the text, and adds it to the scene.
func (s *Scene) DrawText(str string, face text.Face, x, y float32, brush Brush) error {
	renderer := NewTextRenderer()
	return renderer.RenderTextToScene(s, str, face, x, y, brush)
}

// DrawGlyphs draws pre-shaped glyphs directly to the scene.
func (s *Scene) DrawGlyphs(glyphs []text.ShapedGlyph, face text.Face, brush Brush) error {
	renderer := NewTextRenderer()
	return renderer.RenderToScene(s, glyphs, face, brush)
}

// TextShape represents a shaped text string as a scene Shape.
// This allows text to be used with fill/stroke operations.
type TextShape struct {
	path *Path
}

// NewTextShape creates a TextShape from text.
// The text is shaped and converted to a path at the specified position.
func NewTextShape(str string, face text.Face, x, y float32) (*TextShape, error) {
	if str == "" {
		return &TextShape{path: NewPath()}, nil
	}

	renderer := NewTextRenderer()
	rendered, err := renderer.RenderText(str, face)
	if err != nil {
		return nil, err
	}

	// Combine all glyph paths with offset
	composite := NewPath()
	for _, rg := range rendered {
		if rg.Path != nil && !rg.Path.IsEmpty() {
			// Transform path to final position
			transformed := rg.Path.Transform(TranslateAffine(x, y))
			appendPath(composite, transformed)
		}
	}

	return &TextShape{path: composite}, nil
}

// ToPath returns the path representation of the text.
func (ts *TextShape) ToPath() *Path {
	return ts.path
}

// Bounds returns the bounding rectangle of the text.
func (ts *TextShape) Bounds() Rect {
	if ts.path == nil {
		return EmptyRect()
	}
	return ts.path.Bounds()
}
