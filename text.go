package gg

// Text rendering is not implemented in v0.1.0.
// Future versions will add text support using:
// - TrueType font parsing
// - Glyph rasterization
// - Text layout and shaping

// DrawString is a placeholder for future text rendering.
func (c *Context) DrawString(s string, x, y float64) {
	// TODO: Implement in v0.2.0+
	// Will require font loading and glyph rendering
}

// DrawStringAnchored is a placeholder for future text rendering with anchoring.
func (c *Context) DrawStringAnchored(s string, x, y, ax, ay float64) {
	// TODO: Implement in v0.2.0+
}

// MeasureString is a placeholder for future text measurement.
func (c *Context) MeasureString(s string) (w, h float64) {
	// TODO: Implement in v0.2.0+
	return 0, 0
}

// LoadFontFace is a placeholder for future font loading.
func (c *Context) LoadFontFace(path string, points float64) error {
	// TODO: Implement in v0.2.0+
	return nil
}
