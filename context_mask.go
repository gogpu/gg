package gg

// SetMask sets an alpha mask for subsequent drawing operations.
// The mask modulates the alpha of all drawing operations.
// Pass nil to clear the mask.
func (c *Context) SetMask(mask *Mask) {
	c.mask = mask
}

// GetMask returns the current mask, or nil if no mask is set.
func (c *Context) GetMask() *Mask {
	return c.mask
}

// InvertMask inverts the current mask.
// Has no effect if no mask is set.
func (c *Context) InvertMask() {
	if c.mask != nil {
		c.mask.Invert()
	}
}

// ClearMask removes the current mask.
func (c *Context) ClearMask() {
	c.mask = nil
}

// AsMask creates a mask from the current path.
// The mask is filled according to the current fill rule.
// The path is NOT cleared after this operation.
func (c *Context) AsMask() *Mask {
	mask := NewMask(c.Width(), c.Height())

	// Create a temporary context for rasterizing the path
	temp := NewContext(c.Width(), c.Height())
	temp.path = c.path.Clone()
	temp.SetRGBA(1, 1, 1, 1)
	temp.Fill()

	// Extract alpha channel from the rendered path
	img := temp.Image()
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			// a is 0-65535, shift by 8 to get 0-255
			// #nosec G115 -- safe: a>>8 is always in range [0, 255]
			mask.Set(x-bounds.Min.X, y-bounds.Min.Y, uint8(a>>8))
		}
	}

	return mask
}
