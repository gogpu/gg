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
	clipVerbs, clipCoords := ConvertPathToClipVerbs(devicePath)

	// Push the path as a clip region
	_ = c.clipStack.PushPath(clipVerbs, clipCoords, true) // anti-aliased by default

	// Store the device-space path for GPU depth clipping (GPU-CLIP-003a).
	// The GPU DepthClipPipeline fan-tessellates this path at draw time.
	c.gpuClipPath = devicePath.Clone()

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
	clipVerbs, clipCoords := ConvertPathToClipVerbs(devicePath)

	// Push the path as a clip region
	_ = c.clipStack.PushPath(clipVerbs, clipCoords, true) // anti-aliased by default

	// Store the device-space path for GPU depth clipping (GPU-CLIP-003a).
	c.gpuClipPath = devicePath.Clone()
	// Path is preserved
}

// ClipRect sets a rectangular clipping region.
// This is a faster alternative to creating a rectangular path and calling Clip().
// The clip region is intersected with any existing clip regions.
func (c *Context) ClipRect(x, y, w, h float64) {
	if c.clipStack == nil {
		c.initClipStack()
	}

	tm := c.totalMatrix()
	// A scissor rectangle can represent the transformed shape only when the
	// linear part of the transform keeps the rectangle axis-aligned.  The old
	// implementation transformed just two opposite corners and took their
	// bounding box, which collapses a square at 45 degrees and clips the wrong
	// pixels for any rotation or shear.  Preserve the cheap rectangle path for
	// translations/scales, and rasterize the exact device-space quadrilateral
	// otherwise.
	if !isAxisAlignedTransform(tm) {
		devicePath := transformedRectPath(tm, x, y, w, h)
		c.pushDevicePathClip(devicePath)
		return
	}

	// Transform the rectangle corners to device coordinates.
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
// corners are rounded with the given radius. The radius is clamped to half the
// minimum absolute dimension. If radius is zero, this is equivalent to
// ClipRect.
//
// On GPU, axis-aligned clips use a two-level strategy:
//   - Scissor rect (hardware, free) for the bounding box
//   - Analytic SDF in the fragment shader for the rounded corners
//
// Transformed clips use the depth clip path so the rotated/sheared shape is
// preserved exactly.
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

	tm := c.totalMatrix()
	// A rounded rectangle remains an axis-aligned rounded rectangle only when
	// the transform has no rotation/shear and applies the same magnitude of
	// scale on both axes. Under a general affine transform its quarter-circles
	// become rotated/elliptical curves, so use a transformed path rather than
	// approximating the shape with its bounding box and a scalar radius.
	if !isUniformAxisAlignedTransform(tm) {
		pathX, pathY, pathW, pathH := normalizeRect(x, y, w, h)
		userPath := NewPath()
		userPath.RoundedRectangle(pathX, pathY, pathW, pathH, radius)
		c.pushDevicePathClip(userPath.Transform(tm))
		return
	}

	// Transform the rectangle corners to device coordinates.
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

// isAxisAlignedTransform reports whether the linear part of m keeps the axes
// axis-aligned, possibly swapping them. Use exact comparisons: treating a
// small shear as zero can produce a large clipping error when combined with
// large coordinates or a very small axis scale.
func isAxisAlignedTransform(m Matrix) bool {
	return (m.B == 0 && m.D == 0) || (m.A == 0 && m.E == 0)
}

// isUniformAxisAlignedTransform reports whether m keeps rounded rectangles
// axis-aligned without changing circular corners into ellipses. Reflections
// are included; only the magnitudes of the two axis scales must match.
func isUniformAxisAlignedTransform(m Matrix) bool {
	switch {
	case m.B == 0 && m.D == 0:
		return math.Abs(m.A) == math.Abs(m.E)
	case m.A == 0 && m.E == 0:
		return math.Abs(m.B) == math.Abs(m.D)
	default:
		return false
	}
}

// normalizeRect rewrites a rectangle with negative dimensions using its
// geometric top-left corner and positive extents. This matches ClipRect's
// min/abs handling and keeps transformed rounded-rectangle paths well formed.
func normalizeRect(x, y, w, h float64) (float64, float64, float64, float64) {
	if w < 0 {
		x += w
		w = -w
	}
	if h < 0 {
		y += h
		h = -h
	}
	return x, y, w, h
}

// transformedRectPath builds a closed rectangle in device coordinates.
func transformedRectPath(tm Matrix, x, y, w, h float64) *Path {
	p := NewPath()
	p1 := tm.TransformPoint(Pt(x, y))
	p2 := tm.TransformPoint(Pt(x+w, y))
	p3 := tm.TransformPoint(Pt(x+w, y+h))
	p4 := tm.TransformPoint(Pt(x, y+h))
	p.MoveTo(p1.X, p1.Y)
	p.LineTo(p2.X, p2.Y)
	p.LineTo(p3.X, p3.Y)
	p.LineTo(p4.X, p4.Y)
	p.Close()
	return p
}

// pushDevicePathClip adds an already device-space path to the clip stack and
// keeps a copy for the GPU depth-clip path when a GPU backend is active.
func (c *Context) pushDevicePathClip(devicePath *Path) {
	clipVerbs, clipCoords := ConvertPathToClipVerbs(devicePath)
	_ = c.clipStack.PushPath(clipVerbs, clipCoords, true)
	c.gpuClipPath = devicePath.Clone()
}

// ResetClip removes all clipping regions, restoring the full canvas as drawable.
func (c *Context) ResetClip() {
	if c.clipStack == nil {
		return
	}

	// Reset to physical pixel bounds (clip stack operates in device-space).
	bounds := clip.NewRect(0, 0, float64(c.pixmap.Width()), float64(c.pixmap.Height()))
	c.clipStack.Reset(bounds)
	c.gpuClipPath = nil
}

// initClipStack initializes the clip stack with canvas bounds in device-space.
func (c *Context) initClipStack() {
	bounds := clip.NewRect(0, 0, float64(c.pixmap.Width()), float64(c.pixmap.Height()))
	c.clipStack = clip.NewClipStack(bounds)
}

// ConvertPathToClipVerbs converts a gg.Path to clip.PathVerb + coords slices.
// Both PathVerb types have identical byte values, so this is a simple cast.
// Exported for use by internal/gpu CPU clip dispatch (ADR-052).
func ConvertPathToClipVerbs(p *Path) ([]clip.PathVerb, []float64) {
	verbs := p.Verbs()
	result := make([]clip.PathVerb, len(verbs))
	for i, v := range verbs {
		result[i] = clip.PathVerb(v)
	}
	return result, p.Coords()
}
