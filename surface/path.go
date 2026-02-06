// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package surface

import (
	"math"

	"github.com/gogpu/gg/internal/raster"
)

// Path represents a vector path for drawing operations.
//
// Path is the surface-level path type that wraps raster.PathLike.
// It provides a convenient builder API for constructing paths.
//
// Example:
//
//	p := surface.NewPath()
//	p.MoveTo(100, 100)
//	p.LineTo(200, 100)
//	p.LineTo(150, 200)
//	p.Close()
//
//	surface.Fill(p, style)
type Path struct {
	verbs  []raster.PathVerb
	points []float32
	startX float32
	startY float32
	curX   float32
	curY   float32
}

// NewPath creates a new empty path.
func NewPath() *Path {
	return &Path{
		verbs:  make([]raster.PathVerb, 0, 16),
		points: make([]float32, 0, 64),
	}
}

// MoveTo starts a new subpath at the given point.
func (p *Path) MoveTo(x, y float64) {
	p.verbs = append(p.verbs, raster.VerbMoveTo)
	p.points = append(p.points, float32(x), float32(y))
	p.startX, p.startY = float32(x), float32(y)
	p.curX, p.curY = float32(x), float32(y)
}

// LineTo adds a line from the current point to (x, y).
func (p *Path) LineTo(x, y float64) {
	if len(p.verbs) == 0 {
		p.MoveTo(x, y)
		return
	}
	p.verbs = append(p.verbs, raster.VerbLineTo)
	p.points = append(p.points, float32(x), float32(y))
	p.curX, p.curY = float32(x), float32(y)
}

// QuadTo adds a quadratic Bezier curve from the current point.
// (cx, cy) is the control point, (x, y) is the endpoint.
func (p *Path) QuadTo(cx, cy, x, y float64) {
	if len(p.verbs) == 0 {
		p.MoveTo(cx, cy)
	}
	p.verbs = append(p.verbs, raster.VerbQuadTo)
	p.points = append(p.points, float32(cx), float32(cy), float32(x), float32(y))
	p.curX, p.curY = float32(x), float32(y)
}

// CubicTo adds a cubic Bezier curve from the current point.
// (c1x, c1y) and (c2x, c2y) are control points, (x, y) is the endpoint.
func (p *Path) CubicTo(c1x, c1y, c2x, c2y, x, y float64) {
	if len(p.verbs) == 0 {
		p.MoveTo(c1x, c1y)
	}
	p.verbs = append(p.verbs, raster.VerbCubicTo)
	p.points = append(p.points,
		float32(c1x), float32(c1y),
		float32(c2x), float32(c2y),
		float32(x), float32(y))
	p.curX, p.curY = float32(x), float32(y)
}

// Close closes the current subpath by connecting to the start point.
func (p *Path) Close() {
	if len(p.verbs) == 0 {
		return
	}
	p.verbs = append(p.verbs, raster.VerbClose)
	p.curX, p.curY = p.startX, p.startY
}

// Clear removes all elements from the path.
func (p *Path) Clear() {
	p.verbs = p.verbs[:0]
	p.points = p.points[:0]
	p.startX, p.startY = 0, 0
	p.curX, p.curY = 0, 0
}

// IsEmpty returns true if the path has no elements.
func (p *Path) IsEmpty() bool {
	return len(p.verbs) == 0
}

// Verbs returns the verb slice for raster.PathLike interface.
func (p *Path) Verbs() []raster.PathVerb {
	return p.verbs
}

// Points returns the points slice for raster.PathLike interface.
func (p *Path) Points() []float32 {
	return p.points
}

// Verify Path implements raster.PathLike.
var _ raster.PathLike = (*Path)(nil)

// Clone creates a deep copy of the path.
func (p *Path) Clone() *Path {
	clone := &Path{
		verbs:  make([]raster.PathVerb, len(p.verbs)),
		points: make([]float32, len(p.points)),
		startX: p.startX,
		startY: p.startY,
		curX:   p.curX,
		curY:   p.curY,
	}
	copy(clone.verbs, p.verbs)
	copy(clone.points, p.points)
	return clone
}

// CurrentPoint returns the current point.
func (p *Path) CurrentPoint() Point {
	return Point{X: float64(p.curX), Y: float64(p.curY)}
}

// Rectangle adds a rectangle to the path.
func (p *Path) Rectangle(x, y, w, h float64) {
	p.MoveTo(x, y)
	p.LineTo(x+w, y)
	p.LineTo(x+w, y+h)
	p.LineTo(x, y+h)
	p.Close()
}

// RoundedRectangle adds a rectangle with rounded corners.
func (p *Path) RoundedRectangle(x, y, w, h, r float64) {
	maxR := math.Min(w, h) / 2
	if r > maxR {
		r = maxR
	}

	const k = 0.5522847498307936 // Bezier circle approximation constant
	ctl := r * k

	p.MoveTo(x+r, y)
	p.LineTo(x+w-r, y)
	p.CubicTo(x+w-r+ctl, y, x+w, y+r-ctl, x+w, y+r)
	p.LineTo(x+w, y+h-r)
	p.CubicTo(x+w, y+h-r+ctl, x+w-r+ctl, y+h, x+w-r, y+h)
	p.LineTo(x+r, y+h)
	p.CubicTo(x+r-ctl, y+h, x, y+h-r+ctl, x, y+h-r)
	p.LineTo(x, y+r)
	p.CubicTo(x, y+r-ctl, x+r-ctl, y, x+r, y)
	p.Close()
}

// Circle adds a circle to the path.
func (p *Path) Circle(cx, cy, r float64) {
	const k = 0.5522847498307936
	offset := r * k

	p.MoveTo(cx+r, cy)
	p.CubicTo(cx+r, cy+offset, cx+offset, cy+r, cx, cy+r)
	p.CubicTo(cx-offset, cy+r, cx-r, cy+offset, cx-r, cy)
	p.CubicTo(cx-r, cy-offset, cx-offset, cy-r, cx, cy-r)
	p.CubicTo(cx+offset, cy-r, cx+r, cy-offset, cx+r, cy)
	p.Close()
}

// Ellipse adds an ellipse to the path.
func (p *Path) Ellipse(cx, cy, rx, ry float64) {
	const k = 0.5522847498307936
	ox := rx * k
	oy := ry * k

	p.MoveTo(cx+rx, cy)
	p.CubicTo(cx+rx, cy+oy, cx+ox, cy+ry, cx, cy+ry)
	p.CubicTo(cx-ox, cy+ry, cx-rx, cy+oy, cx-rx, cy)
	p.CubicTo(cx-rx, cy-oy, cx-ox, cy-ry, cx, cy-ry)
	p.CubicTo(cx+ox, cy-ry, cx+rx, cy-oy, cx+rx, cy)
	p.Close()
}

// Arc adds a circular arc to the path.
// The arc goes from angle1 to angle2 (in radians) around (cx, cy).
func (p *Path) Arc(cx, cy, r, angle1, angle2 float64) {
	const twoPi = 2 * math.Pi
	for angle2 < angle1 {
		angle2 += twoPi
	}

	const maxAngle = math.Pi / 2
	numSegments := int(math.Ceil((angle2 - angle1) / maxAngle))
	angleStep := (angle2 - angle1) / float64(numSegments)

	for i := 0; i < numSegments; i++ {
		a1 := angle1 + float64(i)*angleStep
		a2 := a1 + angleStep
		p.arcSegment(cx, cy, r, a1, a2)
	}
}

// arcSegment adds a single arc segment (up to 90 degrees).
func (p *Path) arcSegment(cx, cy, r, a1, a2 float64) {
	alpha := math.Sin(a2-a1) * (math.Sqrt(4+3*math.Tan((a2-a1)/2)*math.Tan((a2-a1)/2)) - 1) / 3

	cos1, sin1 := math.Cos(a1), math.Sin(a1)
	cos2, sin2 := math.Cos(a2), math.Sin(a2)

	x1 := cx + r*cos1
	y1 := cy + r*sin1
	x2 := cx + r*cos2
	y2 := cy + r*sin2

	c1x := x1 - alpha*r*sin1
	c1y := y1 + alpha*r*cos1
	c2x := x2 + alpha*r*sin2
	c2y := y2 - alpha*r*cos2

	if len(p.verbs) == 0 {
		p.MoveTo(x1, y1)
	}
	p.CubicTo(c1x, c1y, c2x, c2y, x2, y2)
}

// Bounds returns the axis-aligned bounding box of the path.
// Returns an empty rectangle if the path is empty.
func (p *Path) Bounds() (minX, minY, maxX, maxY float64) {
	if len(p.points) == 0 {
		return 0, 0, 0, 0
	}

	minX = float64(p.points[0])
	maxX = minX
	minY = float64(p.points[1])
	maxY = minY

	for i := 2; i < len(p.points); i += 2 {
		x := float64(p.points[i])
		y := float64(p.points[i+1])
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	return minX, minY, maxX, maxY
}
