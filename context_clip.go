package gg

import (
	"math"

	"github.com/gogpu/gg/internal/clip"
)

// Clip sets the current path as the clipping region and clears the path.
// Subsequent drawing operations will be clipped to this region.
// The clip region is intersected with any existing clip regions.
func (c *Context) Clip() {
	if c.clipStack == nil {
		c.initClipStack()
	}

	// Path elements are in user-space; clip stack operates in device-space.
	// Transform through deviceMatrix to get device coordinates.
	devicePath := c.deviceSpacePath()
	elements := convertPathToClipElements(devicePath)

	// Push the path as a clip region
	_ = c.clipStack.PushPath(elements, true) // anti-aliased by default

	// Clear the path
	c.path.Clear()
}

// ClipPreserve sets the current path as the clipping region but keeps the path.
// This is like Clip() but doesn't clear the path, allowing you to both clip
// and then fill/stroke the same path.
func (c *Context) ClipPreserve() {
	if c.clipStack == nil {
		c.initClipStack()
	}

	// Path elements are in user-space; clip stack operates in device-space.
	devicePath := c.deviceSpacePath()
	elements := convertPathToClipElements(devicePath)

	// Push the path as a clip region
	_ = c.clipStack.PushPath(elements, true) // anti-aliased by default
	// Path is preserved
}

// ClipRect sets a rectangular clipping region.
// This is a faster alternative to creating a rectangular path and calling Clip().
// The clip region is intersected with any existing clip regions.
func (c *Context) ClipRect(x, y, w, h float64) {
	if c.clipStack == nil {
		c.initClipStack()
	}

	// Transform the rectangle corners to device coordinates.
	tm := c.totalMatrix()
	p1 := tm.TransformPoint(Pt(x, y))
	p2 := tm.TransformPoint(Pt(x+w, y+h))

	// Create clip rectangle in device coordinates
	rect := clip.NewRect(
		math.Min(p1.X, p2.X),
		math.Min(p1.Y, p2.Y),
		math.Abs(p2.X-p1.X),
		math.Abs(p2.Y-p1.Y),
	)

	c.clipStack.PushRect(rect)
}

// ClipRoundRect sets a rounded rectangle clipping region.
// The rectangle is defined by (x, y, w, h) in user coordinates and the
// corners are rounded with the given radius. The radius is clamped to
// min(w, h)/2. If radius is zero, this is equivalent to ClipRect.
//
// On GPU, this uses a two-level clip strategy:
//   - Scissor rect (hardware, free) for the bounding box
//   - Analytic SDF in the fragment shader for the rounded corners
//
// On CPU, the SDF is evaluated per-pixel during coverage computation.
func (c *Context) ClipRoundRect(x, y, w, h, radius float64) {
	if radius <= 0 {
		c.ClipRect(x, y, w, h)
		return
	}

	if c.clipStack == nil {
		c.initClipStack()
	}

	// Transform the rectangle corners to device coordinates.
	tm := c.totalMatrix()
	p1 := tm.TransformPoint(Pt(x, y))
	p2 := tm.TransformPoint(Pt(x+w, y+h))

	// Create clip rectangle in device coordinates.
	devX := math.Min(p1.X, p2.X)
	devY := math.Min(p1.Y, p2.Y)
	devW := math.Abs(p2.X - p1.X)
	devH := math.Abs(p2.Y - p1.Y)

	// Scale radius by the total transform scale factor.
	scaledRadius := radius * tm.ScaleFactor()

	// Clamp to half the smaller dimension.
	maxRadius := math.Min(devW, devH) / 2
	if scaledRadius > maxRadius {
		scaledRadius = maxRadius
	}

	rect := clip.NewRect(devX, devY, devW, devH)
	c.clipStack.PushRRect(rect, scaledRadius)
}

// ResetClip removes all clipping regions, restoring the full canvas as drawable.
func (c *Context) ResetClip() {
	if c.clipStack == nil {
		return
	}

	// Reset to physical pixel bounds (clip stack operates in device-space).
	bounds := clip.NewRect(0, 0, float64(c.pixmap.Width()), float64(c.pixmap.Height()))
	c.clipStack.Reset(bounds)
}

// initClipStack initializes the clip stack with canvas bounds in device-space.
func (c *Context) initClipStack() {
	bounds := clip.NewRect(0, 0, float64(c.pixmap.Width()), float64(c.pixmap.Height()))
	c.clipStack = clip.NewClipStack(bounds)
}

// convertPathToClipElements converts a gg.Path to a clip.PathElement slice
// using zero-alloc verb/coord iteration.
func convertPathToClipElements(p *Path) []clip.PathElement {
	result := make([]clip.PathElement, 0, p.NumVerbs())
	p.Iterate(func(verb PathVerb, coords []float64) {
		switch verb {
		case VerbMoveTo:
			result = append(result, clip.MoveTo{Point: clip.Pt(coords[0], coords[1])})
		case VerbLineTo:
			result = append(result, clip.LineTo{Point: clip.Pt(coords[0], coords[1])})
		case VerbQuadTo:
			result = append(result, clip.QuadTo{
				Control: clip.Pt(coords[0], coords[1]),
				Point:   clip.Pt(coords[2], coords[3]),
			})
		case VerbCubicTo:
			result = append(result, clip.CubicTo{
				Control1: clip.Pt(coords[0], coords[1]),
				Control2: clip.Pt(coords[2], coords[3]),
				Point:    clip.Pt(coords[4], coords[5]),
			})
		case VerbClose:
			result = append(result, clip.Close{})
		}
	})
	return result
}
