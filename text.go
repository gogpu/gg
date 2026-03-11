package gg

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/gogpu/gg/text"
)

// Align specifies text horizontal alignment.
// This is a type alias for text.Alignment, provided for fogleman/gg compatibility.
type Align = text.Alignment

// Alignment constants re-exported from the text package for convenience.
const (
	AlignLeft   = text.AlignLeft
	AlignCenter = text.AlignCenter
	AlignRight  = text.AlignRight
)

// SetFont sets the current font face for text drawing.
// The face should be created from a FontSource.
//
// Example:
//
//	source, _ := text.NewFontSourceFromFile("font.ttf")
//	face := source.Face(12.0)
//	ctx.SetFont(face)
func (c *Context) SetFont(face text.Face) {
	c.face = face
}

// Font returns the current font face.
// Returns nil if no font has been set.
func (c *Context) Font() text.Face {
	return c.face
}

// DrawString draws text at position (x, y) where y is the baseline.
// If no font has been set with SetFont, this function does nothing.
//
// If a GPU accelerator is registered and supports text rendering (implements
// GPUTextAccelerator), the text is rendered via the GPU MSDF pipeline.
// The CTM (Current Transform Matrix) is passed to the GPU so that Scale,
// Rotate, and Skew transforms affect text rendering, not just position.
// Otherwise, the CPU text pipeline is used with transform-aware rendering:
//   - Translation-only: bitmap fast path (zero quality loss)
//   - Uniform scale ≤256px: bitmap at device size (Strategy A, Skia pattern)
//   - Everything else: glyph outlines as vector paths (Strategy B, Vello pattern)
//
// The baseline is the line on which most letters sit. Characters with
// descenders (like 'g', 'j', 'p', 'q', 'y') extend below the baseline.
func (c *Context) DrawString(s string, x, y float64) {
	if c.face == nil {
		return
	}

	switch c.selectTextStrategy() {
	case TextModeMSDF:
		// Try GPU MSDF first; fall back to CPU if unavailable.
		if c.tryGPUText(s, x, y) {
			return
		}
		c.drawStringCPU(s, x, y)
	case TextModeVector:
		// Phase 3 will add GPU vector text via compute pipeline.
		// For now, use CPU outline rendering (Strategy B).
		c.flushGPUAccelerator()
		c.drawStringAsOutlines(s, x, y)
	case TextModeBitmap:
		// Skip GPU entirely, use CPU pipeline directly.
		c.flushGPUAccelerator()
		c.drawStringCPU(s, x, y)
	default: // TextModeAuto — current behavior
		if c.tryGPUText(s, x, y) {
			return
		}
		c.drawStringCPU(s, x, y)
	}
}

// tryGPUText attempts to render text via the GPU MSDF pipeline.
// The x, y coordinates are in user space (not pre-transformed by the CTM).
// The CTM is passed to the GPU pipeline so it can apply the full transform
// in the vertex shader, enabling correct scaling, rotation, and skew of text.
// Returns true if GPU text rendering was successful (queued for batch render).
func (c *Context) tryGPUText(s string, x, y float64) bool {
	a := Accelerator()
	if a == nil {
		return false
	}
	if !a.CanAccelerate(AccelText) {
		return false
	}
	ta, ok := a.(GPUTextAccelerator)
	if !ok {
		return false
	}
	col := FromColor(c.currentColor())
	target := c.gpuRenderTarget()
	err := ta.DrawText(target, c.face, s, x, y, col, c.matrix, c.deviceScale)
	return err == nil
}

// selectTextStrategy returns the effective text rendering strategy.
// When TextModeAuto, it returns TextModeAuto to preserve the current
// auto-selection behavior (try GPU MSDF first, then CPU).
// Future: smarter auto-selection based on transform, size, animation state.
func (c *Context) selectTextStrategy() TextMode {
	return c.textMode
}

// DrawStringAnchored draws text with an anchor point.
// The anchor point is specified by ax and ay, which are in the range [0, 1].
//
//	(0, 0) = top-left
//	(0.5, 0.5) = center
//	(1, 1) = bottom-right
//
// The text is positioned so that the anchor point is at (x, y).
func (c *Context) DrawStringAnchored(s string, x, y, ax, ay float64) {
	if c.face == nil {
		return
	}

	// Measure the text and calculate offset based on anchor.
	// The anchor maps linearly within the text bounding box:
	//   ay=0 → y is the top of the text (baseline = y + ascent)
	//   ay=0.5 → y is the vertical center (baseline = y + ascent - h/2)
	//   ay=1 → y is the bottom (baseline = y + ascent - h)
	// Formula: baseline = y + ascent - ay * h
	// where h = ascent + descent (visual bounding box, no lineGap).
	w, _ := text.Measure(s, c.face)
	metrics := c.face.Metrics()
	h := metrics.Ascent + metrics.Descent
	x -= w * ax
	y = y + metrics.Ascent - ay*h

	// Delegate to DrawString which handles TextMode routing.
	c.DrawString(s, x, y)
}

// MeasureString returns the dimensions of text in pixels.
// Returns (width, height) where:
//   - width is the horizontal advance of the text
//   - height is the line height (ascent + descent + line gap)
//
// If no font has been set, returns (0, 0).
func (c *Context) MeasureString(s string) (w, h float64) {
	if c.face == nil {
		return 0, 0
	}
	return text.Measure(s, c.face)
}

// LoadFontFace loads a font from a file and sets it as the current font.
// The size is specified in points.
//
// Deprecated: Use text.NewFontSourceFromFile and SetFont instead.
// This method is provided for convenience and backward compatibility.
//
// Example (new way):
//
//	source, err := text.NewFontSourceFromFile("font.ttf")
//	if err != nil {
//	    return err
//	}
//	face := source.Face(12.0)
//	ctx.SetFont(face)
func (c *Context) LoadFontFace(path string, points float64) error {
	source, err := text.NewFontSourceFromFile(path)
	if err != nil {
		return err
	}
	c.face = source.Face(points)
	return nil
}

// WordWrap wraps text to fit within the given width using word boundaries.
// Returns a slice of strings, one per wrapped line.
// If no font face is set, returns the input string as a single-element slice.
//
// This method is compatible with fogleman/gg's WordWrap.
func (c *Context) WordWrap(s string, w float64) []string {
	if c.face == nil {
		return []string{s}
	}
	results := text.WrapText(s, c.face, w, text.WrapWord)
	lines := make([]string, len(results))
	for i, r := range results {
		lines[i] = r.Text
	}
	return lines
}

// MeasureMultilineString measures text that may contain newlines.
// The lineSpacing parameter is a multiplier for the font's natural line height
// (1.0 = normal spacing, 1.5 = 50% extra space between lines).
// Returns (width, height) where width is the maximum line width and height
// is the total height of all lines with the given line spacing.
// If no font face is set, returns (0, 0).
//
// This method is compatible with fogleman/gg's MeasureMultilineString.
func (c *Context) MeasureMultilineString(s string, lineSpacing float64) (width, height float64) {
	if c.face == nil {
		return 0, 0
	}
	lines := splitLines(s)
	metrics := c.face.Metrics()
	fh := metrics.LineHeight()
	for _, line := range lines {
		lw, _ := text.Measure(line, c.face)
		if lw > width {
			width = lw
		}
	}
	// Visual height: ascent above first baseline + (n-1) inter-line gaps + descent below last baseline.
	n := float64(len(lines))
	height = (n-1)*fh*lineSpacing + metrics.Ascent + metrics.Descent
	return
}

// DrawStringWrapped wraps text to the given width and draws it with alignment.
// The text is positioned relative to (x, y) using the anchor (ax, ay):
//
//	(0, 0) = top-left of the text block is at (x, y)
//	(0.5, 0.5) = center of the text block is at (x, y)
//	(1, 1) = bottom-right of the text block is at (x, y)
//
// The lineSpacing parameter multiplies the font's natural line height
// (1.0 = normal, 1.5 = 50% extra space between lines).
// The align parameter controls horizontal alignment within the wrapped width.
// If no font face is set, this method does nothing.
//
// This method is compatible with fogleman/gg's DrawStringWrapped.
func (c *Context) DrawStringWrapped(s string, x, y, ax, ay, width, lineSpacing float64, align Align) {
	if c.face == nil {
		return
	}
	lines := c.WordWrap(s, width)
	if len(lines) == 0 {
		return
	}

	metrics := c.face.Metrics()
	fh := metrics.LineHeight()

	// Visual height of the text block:
	// - (n-1) inter-line gaps of fh*lineSpacing
	// - ascent above first baseline + descent below last baseline
	n := float64(len(lines))
	h := (n-1)*fh*lineSpacing + metrics.Ascent + metrics.Descent

	// Adjust starting position by anchor (bounding-box model):
	//   ay=0 → y is the top of the block (first baseline = y + ascent)
	//   ay=0.5 → y is the vertical center
	//   ay=1 → y is the bottom of the block
	// Formula: first_baseline = y + ascent - ay * h
	x -= ax * width
	y = y + metrics.Ascent - ay*h

	// Adjust x base for alignment
	switch align {
	case text.AlignCenter:
		x += width / 2
	case text.AlignRight:
		x += width
	}

	for _, line := range lines {
		drawX := x
		switch align {
		case text.AlignCenter:
			lw, _ := c.MeasureString(line)
			drawX = x - lw/2
		case text.AlignRight:
			lw, _ := c.MeasureString(line)
			drawX = x - lw
		}
		c.DrawString(line, drawX, y)
		y += fh * lineSpacing
	}
}

// drawStringCPU selects the optimal CPU text rendering strategy based on the CTM.
// Three-tier decision tree modeled after Skia (QR decomposition, 256px threshold)
// and Cairo (three-matrix model):
//
//   - Tier 0: Translation-only → bitmap fast path (no quality loss)
//   - Tier 1: Uniform positive scale ≤256px → bitmap at device size (Strategy A)
//   - Tier 2: Everything else → glyph outlines as vector paths (Strategy B)
func (c *Context) drawStringCPU(s string, x, y float64) {
	m := c.matrix

	// Tier 0: Translation-only → bitmap fast path (no quality loss).
	if m.IsTranslationOnly() {
		c.drawStringBitmap(s, x, y)
		return
	}

	// Tier 1: Uniform positive scale ≤256px → bitmap at device size (Strategy A).
	// Skia threshold: kSkSideTooBigForAtlas = 256.
	if m.B == 0 && m.D == 0 && m.A == m.E && m.A > 0 {
		deviceSize := c.face.Size() * m.A
		if deviceSize > 0 && deviceSize <= 256 {
			c.drawStringScaled(s, x, y, deviceSize)
			return
		}
	}

	// Tier 2: Everything else → glyph outlines as paths (Strategy B, Vello pattern).
	c.drawStringAsOutlines(s, x, y)
}

// drawStringBitmap renders text via the bitmap rasterizer at the transformed position.
// This is the fast path for identity/translation-only CTMs where no quality loss occurs.
func (c *Context) drawStringBitmap(s string, x, y float64) {
	p := c.matrix.TransformPoint(Pt(x, y))
	c.flushGPUAccelerator()
	text.Draw(c.pixmap, s, c.face, p.X, p.Y, c.currentColor())
}

// drawStringScaled renders text via bitmap rasterization at the device pixel size.
// Strategy A: Create a face at the scaled size, render at the transformed position.
// Falls back to drawStringBitmap if the face doesn't have a FontSource (e.g. MultiFace).
func (c *Context) drawStringScaled(s string, x, y float64, deviceSize float64) {
	source := c.face.Source()
	if source == nil {
		c.drawStringBitmap(s, x, y) // MultiFace fallback
		return
	}
	deviceFace := source.Face(deviceSize)
	p := c.matrix.TransformPoint(Pt(x, y))
	c.flushGPUAccelerator()
	text.Draw(c.pixmap, s, deviceFace, p.X, p.Y, c.currentColor())
}

// drawStringAsOutlines renders text by converting glyph vector outlines to a Path,
// transforming by the CTM, and filling with the SoftwareRenderer.
// Strategy B (Vello pattern): handles rotation, non-uniform scale, shear, mirroring,
// and extreme scales that exceed the bitmap threshold.
//
// Design: all glyphs are composed into ONE path for a single efficient fill call.
// Outlines are built in user space, then path.Transform(CTM) converts to device space.
// Y-flip is applied because font outlines use Y-up (PostScript/TrueType convention)
// while screen coordinates use Y-down.
func (c *Context) drawStringAsOutlines(s string, x, y float64) {
	source := c.face.Source()
	if source == nil {
		c.drawStringBitmap(s, x, y) // MultiFace fallback
		return
	}

	extractor := c.ensureOutlineExtractor()
	parsed := source.Parsed()
	fontSize := c.face.Size()

	// Use glyph cache to avoid repeated outline extraction.
	cache := c.ensureGlyphCache()
	fontID := computeTextFontID(source)
	var sizeKey int16
	switch {
	case fontSize < 0:
		sizeKey = 0
	case fontSize > 32767:
		sizeKey = 32767
	default:
		sizeKey = int16(fontSize) //nolint:gosec // bounds checked above
	}

	path := NewPath()
	hasContour := false

	for glyph := range c.face.Glyphs(s) {
		cacheKey := text.OutlineCacheKey{
			FontID:  fontID,
			GID:     glyph.GID,
			Size:    sizeKey,
			Hinting: text.HintingNone,
		}
		outline := cache.GetOrCreate(cacheKey, func() *text.GlyphOutline {
			o, err := extractor.ExtractOutline(parsed, glyph.GID, fontSize)
			if err != nil || o == nil || o.IsEmpty() {
				return nil
			}
			return o
		})
		if outline == nil {
			continue // space/missing glyph — advance handled by Glyphs iterator
		}

		gx := x + glyph.X

		for _, seg := range outline.Segments {
			// sfnt.LoadGlyph returns Y-down coordinates (screen convention):
			// Y=0 at baseline, Y<0 above baseline, Y>0 below baseline.
			// So we ADD outlineY to baseline (no flip needed).
			switch seg.Op {
			case text.OutlineOpMoveTo:
				if hasContour {
					path.Close()
				}
				path.MoveTo(gx+float64(seg.Points[0].X), y+float64(seg.Points[0].Y))
				hasContour = true
			case text.OutlineOpLineTo:
				path.LineTo(gx+float64(seg.Points[0].X), y+float64(seg.Points[0].Y))
			case text.OutlineOpQuadTo:
				path.QuadraticTo(
					gx+float64(seg.Points[0].X), y+float64(seg.Points[0].Y),
					gx+float64(seg.Points[1].X), y+float64(seg.Points[1].Y))
			case text.OutlineOpCubicTo:
				path.CubicTo(
					gx+float64(seg.Points[0].X), y+float64(seg.Points[0].Y),
					gx+float64(seg.Points[1].X), y+float64(seg.Points[1].Y),
					gx+float64(seg.Points[2].X), y+float64(seg.Points[2].Y))
			}
		}
	}
	if hasContour {
		path.Close()
	}
	if path.isEmpty() {
		return
	}

	devicePath := path.Transform(c.matrix)

	c.flushGPUAccelerator()

	// Apply rasterizer mode for text (same as doFill).
	// Without this, text always uses the default rasterizer,
	// ignoring SetRasterizerMode().
	if sr, ok := c.renderer.(*SoftwareRenderer); ok {
		sr.rasterizerMode = c.rasterizerMode
		defer func() { sr.rasterizerMode = RasterizerAuto }()
	}

	textPaint := *c.paint // shallow copy
	textPaint.FillRule = FillRuleNonZero
	_ = c.renderer.Fill(c.pixmap, devicePath, &textPaint)
}

// ensureOutlineExtractor lazily initializes the outline extractor.
func (c *Context) ensureOutlineExtractor() *text.OutlineExtractor {
	if c.outlineExtractor == nil {
		c.outlineExtractor = text.NewOutlineExtractor()
	}
	return c.outlineExtractor
}

// ensureGlyphCache lazily initializes the glyph cache reference.
// Uses the global shared cache to benefit from cross-Context reuse.
func (c *Context) ensureGlyphCache() *text.GlyphCache {
	if c.glyphCache == nil {
		c.glyphCache = text.GetGlobalGlyphCache()
	}
	return c.glyphCache
}

// computeTextFontID generates a stable hash identifier for a font source.
// Uses FNV-1a hash of font name and glyph count as a lightweight fingerprint.
// Same algorithm as internal/gpu/gpu_text.go:computeFontID.
func computeTextFontID(source *text.FontSource) uint64 {
	if source == nil {
		return 0
	}
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s:%d", source.Name(), source.Parsed().NumGlyphs())
	return h.Sum64()
}

// fontHeight returns the font's natural line height (ascent + descent + line gap).
func (c *Context) fontHeight() float64 {
	if c.face == nil {
		return 0
	}
	return c.face.Metrics().LineHeight()
}

// splitLines splits text by line breaks, normalizing \r\n and \r to \n.
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}
