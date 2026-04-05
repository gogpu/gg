package gg

import "math"

// PathVerb represents a path construction command.
type PathVerb byte

const (
	// VerbMoveTo moves the current point without drawing. Consumes 2 coords (x, y).
	VerbMoveTo PathVerb = iota
	// VerbLineTo draws a line to the specified point. Consumes 2 coords (x, y).
	VerbLineTo
	// VerbQuadTo draws a quadratic Bezier curve. Consumes 4 coords (cx, cy, x, y).
	VerbQuadTo
	// VerbCubicTo draws a cubic Bezier curve. Consumes 6 coords (c1x, c1y, c2x, c2y, x, y).
	VerbCubicTo
	// VerbClose closes the current subpath. Consumes 0 coords.
	VerbClose
)

// String returns a human-readable name for the verb.
func (v PathVerb) String() string {
	switch v {
	case VerbMoveTo:
		return "MoveTo"
	case VerbLineTo:
		return "LineTo"
	case VerbQuadTo:
		return "QuadTo"
	case VerbCubicTo:
		return "CubicTo"
	case VerbClose:
		return "Close"
	default:
		return "Unknown"
	}
}

// verbCoordCount returns the number of float64 coordinates consumed by a verb.
func verbCoordCount(v PathVerb) int {
	switch v {
	case VerbMoveTo, VerbLineTo:
		return 2
	case VerbQuadTo:
		return 4
	case VerbCubicTo:
		return 6
	case VerbClose:
		return 0
	default:
		return 0
	}
}

// PathElement represents a single element in a path.
//
// Deprecated: Use Path.Iterate() or Path.Verbs()/Path.Coords() for zero-alloc path traversal.
// PathElement types (MoveTo, LineTo, QuadTo, CubicTo, Close structs) remain for backward
// compatibility and will be removed in a future version.
type PathElement interface {
	isPathElement()
}

// MoveToEl moves to a point without drawing.
//
// Deprecated: Use VerbMoveTo with Path.Iterate() instead.
type MoveToEl struct {
	Point Point
}

func (MoveToEl) isPathElement() {}

// LineToEl draws a line to a point.
//
// Deprecated: Use VerbLineTo with Path.Iterate() instead.
type LineToEl struct {
	Point Point
}

func (LineToEl) isPathElement() {}

// QuadToEl draws a quadratic Bezier curve.
//
// Deprecated: Use VerbQuadTo with Path.Iterate() instead.
type QuadToEl struct {
	Control Point
	Point   Point
}

func (QuadToEl) isPathElement() {}

// CubicToEl draws a cubic Bezier curve.
//
// Deprecated: Use VerbCubicTo with Path.Iterate() instead.
type CubicToEl struct {
	Control1 Point
	Control2 Point
	Point    Point
}

func (CubicToEl) isPathElement() {}

// CloseEl closes the current subpath.
//
// Deprecated: Use VerbClose with Path.Iterate() instead.
type CloseEl struct{}

func (CloseEl) isPathElement() {}

// Legacy type aliases for backward compatibility.
// These are the original type names used by consumers throughout the codebase.
// They will be removed in a future version along with PathElement.
type (
	// MoveTo is the legacy name for MoveToEl.
	//
	// Deprecated: Use VerbMoveTo with Path.Iterate() instead.
	MoveTo = MoveToEl

	// LineTo is the legacy name for LineToEl.
	//
	// Deprecated: Use VerbLineTo with Path.Iterate() instead.
	LineTo = LineToEl

	// QuadTo is the legacy name for QuadToEl.
	//
	// Deprecated: Use VerbQuadTo with Path.Iterate() instead.
	QuadTo = QuadToEl

	// CubicTo is the legacy name for CubicToEl.
	//
	// Deprecated: Use VerbCubicTo with Path.Iterate() instead.
	CubicTo = CubicToEl

	// Close is the legacy name for CloseEl.
	//
	// Deprecated: Use VerbClose with Path.Iterate() instead.
	Close = CloseEl
)

// Path represents a vector path using SOA (Structure of Arrays) layout.
//
// Internally, the path stores verbs and coordinates in separate contiguous slices
// for cache efficiency and zero per-verb heap allocations. This matches the
// enterprise standard used by Skia, Cairo, tiny-skia, Blend2D, and femtovg.
type Path struct {
	verbs   []PathVerb
	coords  []float64
	start   Point // Starting point of current subpath
	current Point // Current point
}

// NewPath creates a new empty path.
func NewPath() *Path {
	return &Path{
		verbs:  make([]PathVerb, 0, 16),
		coords: make([]float64, 0, 64),
	}
}

// MoveTo moves to a point without drawing.
func (p *Path) MoveTo(x, y float64) {
	p.verbs = append(p.verbs, VerbMoveTo)
	p.coords = append(p.coords, x, y)
	p.start = Pt(x, y)
	p.current = p.start
}

// LineTo draws a line to a point.
func (p *Path) LineTo(x, y float64) {
	p.verbs = append(p.verbs, VerbLineTo)
	p.coords = append(p.coords, x, y)
	p.current = Pt(x, y)
}

// QuadraticTo draws a quadratic Bezier curve.
func (p *Path) QuadraticTo(cx, cy, x, y float64) {
	p.verbs = append(p.verbs, VerbQuadTo)
	p.coords = append(p.coords, cx, cy, x, y)
	p.current = Pt(x, y)
}

// CubicTo draws a cubic Bezier curve.
func (p *Path) CubicTo(c1x, c1y, c2x, c2y, x, y float64) {
	p.verbs = append(p.verbs, VerbCubicTo)
	p.coords = append(p.coords, c1x, c1y, c2x, c2y, x, y)
	p.current = Pt(x, y)
}

// Close closes the current subpath by drawing a line to the start point.
func (p *Path) Close() {
	p.verbs = append(p.verbs, VerbClose)
	p.current = p.start
}

// Clear removes all elements from the path, releasing the underlying storage.
func (p *Path) Clear() {
	p.verbs = p.verbs[:0]
	p.coords = p.coords[:0]
	p.start = Point{}
	p.current = Point{}
}

// Reset clears the path for reuse, keeping allocated capacity.
// This is identical to Clear in the current implementation.
func (p *Path) Reset() {
	p.verbs = p.verbs[:0]
	p.coords = p.coords[:0]
	p.start = Point{}
	p.current = Point{}
}

// Append adds all elements from other to this path.
// The current point and subpath start are updated to match other's state.
func (p *Path) Append(other *Path) {
	if other == nil || len(other.verbs) == 0 {
		return
	}
	p.verbs = append(p.verbs, other.verbs...)
	p.coords = append(p.coords, other.coords...)
	p.current = other.current
	p.start = other.start
}

// Iterate calls fn for each verb in the path with the corresponding coordinate slice.
// This is the primary zero-allocation iteration API.
//
// The coords slice passed to fn is a sub-slice of the path's coordinate buffer:
//   - VerbMoveTo:  coords has 2 elements (x, y)
//   - VerbLineTo:  coords has 2 elements (x, y)
//   - VerbQuadTo:  coords has 4 elements (cx, cy, x, y)
//   - VerbCubicTo: coords has 6 elements (c1x, c1y, c2x, c2y, x, y)
//   - VerbClose:   coords has 0 elements (nil)
func (p *Path) Iterate(fn func(verb PathVerb, coords []float64)) {
	ci := 0
	for _, v := range p.verbs {
		n := verbCoordCount(v)
		if n > 0 {
			fn(v, p.coords[ci:ci+n])
		} else {
			fn(v, nil)
		}
		ci += n
	}
}

// Verbs returns the verb stream. The returned slice must not be modified.
func (p *Path) Verbs() []PathVerb {
	return p.verbs
}

// Coords returns the coordinate stream. The returned slice must not be modified.
func (p *Path) Coords() []float64 {
	return p.coords
}

// NumVerbs returns the number of verbs in the path.
func (p *Path) NumVerbs() int {
	return len(p.verbs)
}

// Elements returns the path elements as an interface slice.
//
// Deprecated: Use Iterate() for zero-alloc path traversal. This method allocates
// one PathElement per verb for backward compatibility.
func (p *Path) Elements() []PathElement {
	elems := make([]PathElement, 0, len(p.verbs))
	ci := 0
	for _, v := range p.verbs {
		switch v {
		case VerbMoveTo:
			elems = append(elems, MoveTo{Point: Pt(p.coords[ci], p.coords[ci+1])})
			ci += 2
		case VerbLineTo:
			elems = append(elems, LineTo{Point: Pt(p.coords[ci], p.coords[ci+1])})
			ci += 2
		case VerbQuadTo:
			elems = append(elems, QuadTo{
				Control: Pt(p.coords[ci], p.coords[ci+1]),
				Point:   Pt(p.coords[ci+2], p.coords[ci+3]),
			})
			ci += 4
		case VerbCubicTo:
			elems = append(elems, CubicTo{
				Control1: Pt(p.coords[ci], p.coords[ci+1]),
				Control2: Pt(p.coords[ci+2], p.coords[ci+3]),
				Point:    Pt(p.coords[ci+4], p.coords[ci+5]),
			})
			ci += 6
		case VerbClose:
			elems = append(elems, Close{})
		}
	}
	return elems
}

// CurrentPoint returns the current point.
func (p *Path) CurrentPoint() Point {
	return p.current
}

// HasCurrentPoint returns true if the path has a current point.
// A path has a current point after MoveTo, LineTo, or any curve operation.
func (p *Path) HasCurrentPoint() bool {
	return len(p.verbs) > 0
}

// isEmpty returns true if the path has no elements.
func (p *Path) isEmpty() bool {
	return len(p.verbs) == 0
}

// Transform applies a transformation matrix to all points in the path.
func (p *Path) Transform(m Matrix) *Path {
	result := NewPath()
	p.Iterate(func(verb PathVerb, coords []float64) {
		switch verb {
		case VerbMoveTo:
			pt := m.TransformPoint(Pt(coords[0], coords[1]))
			result.MoveTo(pt.X, pt.Y)
		case VerbLineTo:
			pt := m.TransformPoint(Pt(coords[0], coords[1]))
			result.LineTo(pt.X, pt.Y)
		case VerbQuadTo:
			ctrl := m.TransformPoint(Pt(coords[0], coords[1]))
			pt := m.TransformPoint(Pt(coords[2], coords[3]))
			result.QuadraticTo(ctrl.X, ctrl.Y, pt.X, pt.Y)
		case VerbCubicTo:
			ctrl1 := m.TransformPoint(Pt(coords[0], coords[1]))
			ctrl2 := m.TransformPoint(Pt(coords[2], coords[3]))
			pt := m.TransformPoint(Pt(coords[4], coords[5]))
			result.CubicTo(ctrl1.X, ctrl1.Y, ctrl2.X, ctrl2.Y, pt.X, pt.Y)
		case VerbClose:
			result.Close()
		}
	})
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

// arcSegment adds a single arc segment (<=90 degrees).
func (p *Path) arcSegment(cx, cy, r, a1, a2 float64) {
	// Calculate control points for cubic Bezier approximation
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
	result := &Path{
		verbs:   make([]PathVerb, len(p.verbs)),
		coords:  make([]float64, len(p.coords)),
		start:   p.start,
		current: p.current,
	}
	copy(result.verbs, p.verbs)
	copy(result.coords, p.coords)
	return result
}
