package gg

import (
	"github.com/gogpu/gg/text"
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
// The baseline is the line on which most letters sit. Characters with
// descenders (like 'g', 'j', 'p', 'q', 'y') extend below the baseline.
func (c *Context) DrawString(s string, x, y float64) {
	if c.face == nil {
		return
	}
	// Flush pending GPU shapes so they don't overwrite text.
	c.flushGPUAccelerator()
	text.Draw(c.pixmap, s, c.face, x, y, c.currentColor())
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

	// Measure the text
	w, h := text.Measure(s, c.face)

	// Calculate offset based on anchor
	x -= w * ax
	y += h * ay // Note: y is baseline, so we adjust upward for top alignment

	// Flush pending GPU shapes so they don't overwrite text.
	c.flushGPUAccelerator()
	text.Draw(c.pixmap, s, c.face, x, y, c.currentColor())
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
