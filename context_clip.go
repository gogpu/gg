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

	// Convert gg.PathElement to clip.PathElement
	elements := convertPathElements(c.path.Elements())

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

	// Convert gg.PathElement to clip.PathElement
	elements := convertPathElements(c.path.Elements())

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

	// Transform the rectangle corners
	p1 := c.matrix.TransformPoint(Pt(x, y))
	p2 := c.matrix.TransformPoint(Pt(x+w, y+h))

	// Create clip rectangle in device coordinates
	rect := clip.NewRect(
		math.Min(p1.X, p2.X),
		math.Min(p1.Y, p2.Y),
		math.Abs(p2.X-p1.X),
		math.Abs(p2.Y-p1.Y),
	)

	c.clipStack.PushRect(rect)
}

// ResetClip removes all clipping regions, restoring the full canvas as drawable.
func (c *Context) ResetClip() {
	if c.clipStack == nil {
		return
	}

	// Reset to canvas bounds
	bounds := clip.NewRect(0, 0, float64(c.width), float64(c.height))
	c.clipStack.Reset(bounds)
}

// initClipStack initializes the clip stack with canvas bounds.
func (c *Context) initClipStack() {
	bounds := clip.NewRect(0, 0, float64(c.width), float64(c.height))
	c.clipStack = clip.NewClipStack(bounds)
}

// convertPathElements converts gg.PathElement slice to clip.PathElement slice.
func convertPathElements(elements []PathElement) []clip.PathElement {
	result := make([]clip.PathElement, len(elements))
	for i, elem := range elements {
		switch e := elem.(type) {
		case MoveTo:
			result[i] = clip.MoveTo{Point: clip.Pt(e.Point.X, e.Point.Y)}
		case LineTo:
			result[i] = clip.LineTo{Point: clip.Pt(e.Point.X, e.Point.Y)}
		case QuadTo:
			result[i] = clip.QuadTo{
				Control: clip.Pt(e.Control.X, e.Control.Y),
				Point:   clip.Pt(e.Point.X, e.Point.Y),
			}
		case CubicTo:
			result[i] = clip.CubicTo{
				Control1: clip.Pt(e.Control1.X, e.Control1.Y),
				Control2: clip.Pt(e.Control2.X, e.Control2.Y),
				Point:    clip.Pt(e.Point.X, e.Point.Y),
			}
		case Close:
			result[i] = clip.Close{}
		}
	}
	return result
}
