package gg

import (
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
// Otherwise, the software freetype renderer is used as fallback.
//
// The baseline is the line on which most letters sit. Characters with
// descenders (like 'g', 'j', 'p', 'q', 'y') extend below the baseline.
func (c *Context) DrawString(s string, x, y float64) {
	if c.face == nil {
		return
	}

	// Try GPU text rendering first with user-space coordinates.
	// The GPU pipeline receives the CTM and applies it in the vertex shader,
	// so positions are passed in user space (not pre-transformed).
	if c.tryGPUText(s, x, y) {
		return
	}

	// CPU fallback: apply CTM to position only.
	p := c.matrix.TransformPoint(Pt(x, y))

	// Flush pending GPU shapes so they don't overwrite text.
	c.flushGPUAccelerator()
	text.Draw(c.pixmap, s, c.face, p.X, p.Y, c.currentColor())
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
	err := ta.DrawText(target, c.face, s, x, y, col, c.matrix)
	return err == nil
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
	w, h := text.Measure(s, c.face)
	x -= w * ax
	y += h * ay // Note: y is baseline, so we adjust upward for top alignment

	// Try GPU text rendering first with user-space coordinates.
	// The CTM is applied in the vertex shader.
	if c.tryGPUText(s, x, y) {
		return
	}

	// CPU fallback: apply CTM to position only.
	p := c.matrix.TransformPoint(Pt(x, y))

	// Flush pending GPU shapes so they don't overwrite text.
	c.flushGPUAccelerator()
	text.Draw(c.pixmap, s, c.face, p.X, p.Y, c.currentColor())
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
	fh := c.fontHeight()
	for _, line := range lines {
		lw, _ := text.Measure(line, c.face)
		if lw > width {
			width = lw
		}
	}
	// Height formula: n lines with (n-1) inter-line gaps of fh*lineSpacing,
	// plus one line height for the last line.
	// h = fh * ((n-1)*lineSpacing + 1)
	n := float64(len(lines))
	height = n*fh*lineSpacing - (lineSpacing-1)*fh
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

	fh := c.fontHeight()

	// Total height (same formula as MeasureMultilineString)
	n := float64(len(lines))
	h := n*fh*lineSpacing - (lineSpacing-1)*fh

	// Adjust starting position by anchor
	x -= ax * width
	y -= ay * h

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
