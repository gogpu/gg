package gg

import "math"

// PathElement represents a single element in a path.
type PathElement interface {
	isPathElement()
}

// MoveTo moves to a point without drawing.
type MoveTo struct {
	Point Point
}

func (MoveTo) isPathElement() {}

// LineTo draws a line to a point.
type LineTo struct {
	Point Point
}

func (LineTo) isPathElement() {}

// QuadTo draws a quadratic Bezier curve.
type QuadTo struct {
	Control Point
	Point   Point
}

func (QuadTo) isPathElement() {}

// CubicTo draws a cubic Bezier curve.
type CubicTo struct {
	Control1 Point
	Control2 Point
	Point    Point
}

func (CubicTo) isPathElement() {}

// Close closes the current subpath.
type Close struct{}

func (Close) isPathElement() {}

// Path represents a vector path.
type Path struct {
	elements []PathElement
	start    Point // Starting point of current subpath
	current  Point // Current point
}

// NewPath creates a new empty path.
func NewPath() *Path {
	return &Path{
		elements: make([]PathElement, 0, 16),
	}
}

// MoveTo moves to a point without drawing.
func (p *Path) MoveTo(x, y float64) {
	pt := Pt(x, y)
	p.elements = append(p.elements, MoveTo{Point: pt})
	p.start = pt
	p.current = pt
}

// LineTo draws a line to a point.
func (p *Path) LineTo(x, y float64) {
	pt := Pt(x, y)
	p.elements = append(p.elements, LineTo{Point: pt})
	p.current = pt
}

// QuadraticTo draws a quadratic Bezier curve.
func (p *Path) QuadraticTo(cx, cy, x, y float64) {
	ctrl := Pt(cx, cy)
	pt := Pt(x, y)
	p.elements = append(p.elements, QuadTo{Control: ctrl, Point: pt})
	p.current = pt
}

// CubicTo draws a cubic Bezier curve.
func (p *Path) CubicTo(c1x, c1y, c2x, c2y, x, y float64) {
	ctrl1 := Pt(c1x, c1y)
	ctrl2 := Pt(c2x, c2y)
	pt := Pt(x, y)
	p.elements = append(p.elements, CubicTo{
		Control1: ctrl1,
		Control2: ctrl2,
		Point:    pt,
	})
	p.current = pt
}

// Close closes the current subpath by drawing a line to the start point.
func (p *Path) Close() {
	p.elements = append(p.elements, Close{})
	p.current = p.start
}

// Clear removes all elements from the path.
func (p *Path) Clear() {
	p.elements = p.elements[:0]
	p.start = Point{}
	p.current = Point{}
}

// Elements returns the path elements.
func (p *Path) Elements() []PathElement {
	return p.elements
}

// CurrentPoint returns the current point.
func (p *Path) CurrentPoint() Point {
	return p.current
}

// HasCurrentPoint returns true if the path has a current point.
// A path has a current point after MoveTo, LineTo, or any curve operation.
func (p *Path) HasCurrentPoint() bool {
	return len(p.elements) > 0
}

// Transform applies a transformation matrix to all points in the path.
func (p *Path) Transform(m Matrix) *Path {
	result := NewPath()
	for _, elem := range p.elements {
		switch e := elem.(type) {
		case MoveTo:
			pt := m.TransformPoint(e.Point)
			result.MoveTo(pt.X, pt.Y)
		case LineTo:
			pt := m.TransformPoint(e.Point)
			result.LineTo(pt.X, pt.Y)
		case QuadTo:
			ctrl := m.TransformPoint(e.Control)
			pt := m.TransformPoint(e.Point)
			result.QuadraticTo(ctrl.X, ctrl.Y, pt.X, pt.Y)
		case CubicTo:
			ctrl1 := m.TransformPoint(e.Control1)
			ctrl2 := m.TransformPoint(e.Control2)
			pt := m.TransformPoint(e.Point)
			result.CubicTo(ctrl1.X, ctrl1.Y, ctrl2.X, ctrl2.Y, pt.X, pt.Y)
		case Close:
			result.Close()
		}
	}
	return result
}

// Rectangle adds a rectangle to the path.
func (p *Path) Rectangle(x, y, w, h float64) {
	p.MoveTo(x, y)
	p.LineTo(x+w, y)
	p.LineTo(x+w, y+h)
	p.LineTo(x, y+h)
	p.Close()
}

// Circle adds a circle to the path using cubic Bezier curves.
func (p *Path) Circle(cx, cy, r float64) {
	// Magic constant for circle approximation with cubic Beziers
	const k = 0.5522847498307936 // 4/3 * (sqrt(2) - 1)
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
// The arc is drawn from angle1 to angle2 (in radians) around center (cx, cy).
func (p *Path) Arc(cx, cy, r, angle1, angle2 float64) {
	// Normalize angles
	const twoPi = 2 * math.Pi
	for angle2 < angle1 {
		angle2 += twoPi
	}

	// Split into multiple cubic Bezier curves
	// Maximum 90 degrees per segment
	const maxAngle = math.Pi / 2
	numSegments := int(math.Ceil((angle2 - angle1) / maxAngle))
	angleStep := (angle2 - angle1) / float64(numSegments)

	for i := 0; i < numSegments; i++ {
		a1 := angle1 + float64(i)*angleStep
		a2 := a1 + angleStep
		p.arcSegment(cx, cy, r, a1, a2)
	}
}

// arcSegment adds a single arc segment (≤90 degrees).
func (p *Path) arcSegment(cx, cy, r, a1, a2 float64) {
	// Calculate control points for cubic Bezier approximation
	// Using the formula from "Drawing an elliptical arc using polylines, quadratic or cubic Bezier curves"
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

	if len(p.elements) == 0 {
		p.MoveTo(x1, y1)
	}
	p.CubicTo(c1x, c1y, c2x, c2y, x2, y2)
}

// RoundedRectangle adds a rectangle with rounded corners.
func (p *Path) RoundedRectangle(x, y, w, h, r float64) {
	// Clamp radius to half of the smaller dimension
	maxR := math.Min(w, h) / 2
	if r > maxR {
		r = maxR
	}

	p.MoveTo(x+r, y)
	p.LineTo(x+w-r, y)
	p.Arc(x+w-r, y+r, r, -math.Pi/2, 0)
	p.LineTo(x+w, y+h-r)
	p.Arc(x+w-r, y+h-r, r, 0, math.Pi/2)
	p.LineTo(x+r, y+h)
	p.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi)
	p.LineTo(x, y+r)
	p.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2)
	p.Close()
}

// Clone creates a deep copy of the path.
func (p *Path) Clone() *Path {
	result := NewPath()
	result.elements = make([]PathElement, len(p.elements))
	copy(result.elements, p.elements)
	result.start = p.start
	result.current = p.current
	return result
}
