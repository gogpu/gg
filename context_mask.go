package gg

// SetMask sets an alpha mask for subsequent drawing operations.
// The mask modulates the alpha of all Fill and Stroke operations —
// each pixel's coverage is multiplied by the mask value at that pixel.
// A mask value of 255 means fully visible, 0 means fully transparent.
//
// When both a mask and a clip are active, they compose multiplicatively:
// finalCoverage = shapeCoverage * clipCoverage * maskCoverage / (255*255).
//
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

// AsMask creates a mask from the current unfilled path.
// The current path is rasterized (filled with white on a transparent background)
// and the alpha channel is extracted into a Mask. The fill rule from the context
// is used for rasterization. The path is NOT cleared after this operation.
//
// IMPORTANT: AsMask works with the current path, which is cleared by [Context.Fill].
// Call AsMask BEFORE Fill, or use [Context.FillPreserve] to keep the path.
//
// Correct usage patterns:
//
//	// Pattern 1: AsMask before Fill
//	dc.DrawCircle(50, 50, 30)
//	mask := dc.AsMask()    // captures the circle path as a mask
//	dc.Fill()              // fills the circle (clears the path)
//
//	// Pattern 2: FillPreserve + AsMask
//	dc.DrawCircle(50, 50, 30)
//	dc.FillPreserve()      // fills but keeps the path
//	mask := dc.AsMask()    // still has the circle path
//	dc.ClearPath()
//
//	// Pattern 3: Capture rendered output as mask
//	dc.DrawCircle(50, 50, 30)
//	dc.Fill()              // path is now cleared
//	mask := NewMaskFromAlpha(dc.Image()) // capture from rendered pixels
//
// Common mistake (returns empty mask):
//
//	dc.DrawCircle(50, 50, 30)
//	dc.Fill()              // clears the path!
//	mask := dc.AsMask()    // path is empty → mask is all zeros
func (c *Context) AsMask() *Mask {
	mask := NewMask(c.Width(), c.Height())

	// Create a temporary context for rasterizing the path
	temp := NewContext(c.Width(), c.Height())
	temp.path = c.path.Clone()
	temp.SetRGBA(1, 1, 1, 1)
	_ = temp.Fill() // Software renderer never fails

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
