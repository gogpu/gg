package scene

import "math"

// PathVerb represents a path construction command.
type PathVerb uint8

// Path verb constants.
const (
	// VerbMoveTo moves the current point without drawing.
	VerbMoveTo PathVerb = iota
	// VerbLineTo draws a line to the specified point.
	VerbLineTo
	// VerbQuadTo draws a quadratic Bezier curve.
	VerbQuadTo
	// VerbCubicTo draws a cubic Bezier curve.
	VerbCubicTo
	// VerbClose closes the current subpath.
	VerbClose
)

// unknownStr is the string returned for unknown enum values.
const unknownStr = "Unknown"

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
		return unknownStr
	}
}

// PointCount returns the number of points this verb consumes.
func (v PathVerb) PointCount() int {
	switch v {
	case VerbMoveTo, VerbLineTo:
		return 2 // x, y
	case VerbQuadTo:
		return 4 // cx, cy, x, y
	case VerbCubicTo:
		return 6 // c1x, c1y, c2x, c2y, x, y
	case VerbClose:
		return 0
	default:
		return 0
	}
}

// Path represents a vector path for encoding.
// It stores path commands (verbs) and coordinate data separately
// for efficient processing and encoding.
type Path struct {
	verbs  []PathVerb
	points []float32
	bounds Rect
	start  [2]float32 // Start of current subpath for Close
	cursor [2]float32 // Current position
}

// NewPath creates a new empty path.
func NewPath() *Path {
	return &Path{
		verbs:  make([]PathVerb, 0, 16),
		points: make([]float32, 0, 64),
		bounds: EmptyRect(),
	}
}

// Reset clears the path for reuse without deallocating memory.
func (p *Path) Reset() {
	p.verbs = p.verbs[:0]
	p.points = p.points[:0]
	p.bounds = EmptyRect()
	p.start = [2]float32{0, 0}
	p.cursor = [2]float32{0, 0}
}

// MoveTo begins a new subpath at the specified point.
func (p *Path) MoveTo(x, y float32) *Path {
	p.verbs = append(p.verbs, VerbMoveTo)
	p.points = append(p.points, x, y)
	p.bounds = p.bounds.UnionPoint(x, y)
	p.start = [2]float32{x, y}
	p.cursor = [2]float32{x, y}
	return p
}

// LineTo draws a line from the current point to (x, y).
func (p *Path) LineTo(x, y float32) *Path {
	p.verbs = append(p.verbs, VerbLineTo)
	p.points = append(p.points, x, y)
	p.bounds = p.bounds.UnionPoint(x, y)
	p.cursor = [2]float32{x, y}
	return p
}

// QuadTo draws a quadratic Bezier curve.
// The curve goes from the current point to (x, y) using (cx, cy) as control point.
func (p *Path) QuadTo(cx, cy, x, y float32) *Path {
	p.verbs = append(p.verbs, VerbQuadTo)
	p.points = append(p.points, cx, cy, x, y)
	p.bounds = p.bounds.UnionPoint(cx, cy)
	p.bounds = p.bounds.UnionPoint(x, y)
	// For accurate bounds, we should compute curve extrema,
	// but union with control points is a conservative approximation
	p.cursor = [2]float32{x, y}
	return p
}

// CubicTo draws a cubic Bezier curve.
// The curve goes from the current point to (x, y) using (c1x, c1y) and (c2x, c2y) as control points.
func (p *Path) CubicTo(c1x, c1y, c2x, c2y, x, y float32) *Path {
	p.verbs = append(p.verbs, VerbCubicTo)
	p.points = append(p.points, c1x, c1y, c2x, c2y, x, y)
	p.bounds = p.bounds.UnionPoint(c1x, c1y)
	p.bounds = p.bounds.UnionPoint(c2x, c2y)
	p.bounds = p.bounds.UnionPoint(x, y)
	// For accurate bounds, we should compute curve extrema,
	// but union with control points is a conservative approximation
	p.cursor = [2]float32{x, y}
	return p
}

// Close closes the current subpath by drawing a line back to its start.
func (p *Path) Close() *Path {
	p.verbs = append(p.verbs, VerbClose)
	p.cursor = p.start
	return p
}

// Rectangle adds a rectangle path.
func (p *Path) Rectangle(x, y, w, h float32) *Path {
	return p.MoveTo(x, y).
		LineTo(x+w, y).
		LineTo(x+w, y+h).
		LineTo(x, y+h).
		Close()
}

// RoundedRectangle adds a rounded rectangle path.
func (p *Path) RoundedRectangle(x, y, w, h, r float32) *Path {
	// Clamp radius to half the minimum dimension
	maxR := min32(w, h) / 2
	if r > maxR {
		r = maxR
	}
	if r <= 0 {
		return p.Rectangle(x, y, w, h)
	}

	// Magic number for approximating circular arcs with cubic beziers
	// k = 4 * (sqrt(2) - 1) / 3 ≈ 0.5523
	k := float32(0.5522847498)
	kr := k * r

	// Start from top-left corner (after the rounded corner)
	p.MoveTo(x+r, y)

	// Top edge and top-right corner
	p.LineTo(x+w-r, y)
	p.CubicTo(x+w-r+kr, y, x+w, y+r-kr, x+w, y+r)

	// Right edge and bottom-right corner
	p.LineTo(x+w, y+h-r)
	p.CubicTo(x+w, y+h-r+kr, x+w-r+kr, y+h, x+w-r, y+h)

	// Bottom edge and bottom-left corner
	p.LineTo(x+r, y+h)
	p.CubicTo(x+r-kr, y+h, x, y+h-r+kr, x, y+h-r)

	// Left edge and top-left corner
	p.LineTo(x, y+r)
	p.CubicTo(x, y+r-kr, x+r-kr, y, x+r, y)

	return p.Close()
}

// Circle adds a circle path.
func (p *Path) Circle(cx, cy, r float32) *Path {
	return p.Ellipse(cx, cy, r, r)
}

// Ellipse adds an ellipse path.
func (p *Path) Ellipse(cx, cy, rx, ry float32) *Path {
	// Magic number for approximating circular arcs with cubic beziers
	k := float32(0.5522847498)
	kx := k * rx
	ky := k * ry

	// Start at the right edge
	p.MoveTo(cx+rx, cy)

	// Four quarter-circle arcs
	p.CubicTo(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry) // to bottom
	p.CubicTo(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy) // to left
	p.CubicTo(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry) // to top
	p.CubicTo(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy) // to right (start)

	return p.Close()
}

// Arc adds an arc path (portion of an ellipse).
// The arc is drawn from startAngle to endAngle (in radians).
// If sweepClockwise is true, the arc is drawn clockwise.
func (p *Path) Arc(cx, cy, rx, ry, startAngle, endAngle float32, sweepClockwise bool) *Path {
	// Normalize angles
	if sweepClockwise && endAngle < startAngle {
		endAngle += 2 * math.Pi
	} else if !sweepClockwise && startAngle < endAngle {
		startAngle += 2 * math.Pi
	}

	// Calculate start point
	startX := cx + rx*float32(math.Cos(float64(startAngle)))
	startY := cy + ry*float32(math.Sin(float64(startAngle)))
	p.MoveTo(startX, startY)

	// Calculate sweep angle
	sweep := endAngle - startAngle
	if !sweepClockwise {
		sweep = -sweep
	}

	// Split into quarter arcs (max 90 degrees each) for better approximation
	numArcs := int(math.Ceil(math.Abs(float64(sweep)) / (math.Pi / 2)))
	if numArcs < 1 {
		numArcs = 1
	}

	arcAngle := sweep / float32(numArcs)
	currentAngle := startAngle

	for i := 0; i < numArcs; i++ {
		nextAngle := currentAngle + arcAngle
		p.arcSegment(cx, cy, rx, ry, currentAngle, nextAngle)
		currentAngle = nextAngle
	}

	return p
}

// arcSegment adds a cubic bezier approximation of an arc segment.
func (p *Path) arcSegment(cx, cy, rx, ry, startAngle, endAngle float32) {
	// Bezier control point factor
	angle := endAngle - startAngle
	alpha := float32(math.Sin(float64(angle))) * (float32(math.Sqrt(float64(4+3*float32(math.Tan(float64(angle/2)))*float32(math.Tan(float64(angle/2)))))) - 1) / 3

	// Start point
	cos1 := float32(math.Cos(float64(startAngle)))
	sin1 := float32(math.Sin(float64(startAngle)))
	x1 := cx + rx*cos1
	y1 := cy + ry*sin1

	// End point
	cos2 := float32(math.Cos(float64(endAngle)))
	sin2 := float32(math.Sin(float64(endAngle)))
	x4 := cx + rx*cos2
	y4 := cy + ry*sin2

	// Control points
	x2 := x1 - alpha*rx*sin1
	y2 := y1 + alpha*ry*cos1
	x3 := x4 + alpha*rx*sin2
	y3 := y4 - alpha*ry*cos2

	p.CubicTo(x2, y2, x3, y3, x4, y4)
}

// Bounds returns the bounding rectangle of the path.
// Note: This is a conservative approximation that includes control points.
func (p *Path) Bounds() Rect {
	return p.bounds
}

// IsEmpty returns true if the path has no commands.
func (p *Path) IsEmpty() bool {
	return len(p.verbs) == 0
}

// Verbs returns the verb stream.
func (p *Path) Verbs() []PathVerb {
	return p.verbs
}

// Points returns the point data stream.
func (p *Path) Points() []float32 {
	return p.points
}

// VerbCount returns the number of verbs in the path.
func (p *Path) VerbCount() int {
	return len(p.verbs)
}

// PointCount returns the number of float32 values in the point stream.
func (p *Path) PointCount() int {
	return len(p.points)
}

// Transform returns a new path with all points transformed by the affine matrix.
func (p *Path) Transform(t Affine) *Path {
	result := NewPath()
	result.verbs = make([]PathVerb, len(p.verbs))
	copy(result.verbs, p.verbs)
	result.points = make([]float32, len(p.points))

	// Transform all points
	for i := 0; i < len(p.points); i += 2 {
		x, y := t.TransformPoint(p.points[i], p.points[i+1])
		result.points[i] = x
		result.points[i+1] = y
		result.bounds = result.bounds.UnionPoint(x, y)
	}

	// Transform start and cursor
	result.start[0], result.start[1] = t.TransformPoint(p.start[0], p.start[1])
	result.cursor[0], result.cursor[1] = t.TransformPoint(p.cursor[0], p.cursor[1])

	return result
}

// Clone creates a deep copy of the path.
func (p *Path) Clone() *Path {
	result := NewPath()
	result.verbs = make([]PathVerb, len(p.verbs))
	copy(result.verbs, p.verbs)
	result.points = make([]float32, len(p.points))
	copy(result.points, p.points)
	result.bounds = p.bounds
	result.start = p.start
	result.cursor = p.cursor
	return result
}

// subpathData holds data for a single subpath during reversal.
type subpathData struct {
	verbs  []PathVerb
	points []float32
	startX float32
	startY float32
	closed bool
}

// Reverse returns a new path with the direction reversed.
// This is useful for creating cut-out shapes.
func (p *Path) Reverse() *Path {
	if p.IsEmpty() {
		return NewPath()
	}

	result := NewPath()

	// Collect subpaths
	var subpaths []subpathData
	var current subpathData
	pointIdx := 0

	for _, verb := range p.verbs {
		switch verb {
		case VerbMoveTo:
			if len(current.verbs) > 0 {
				subpaths = append(subpaths, current)
			}
			current = subpathData{
				verbs:  []PathVerb{verb},
				points: []float32{p.points[pointIdx], p.points[pointIdx+1]},
				startX: p.points[pointIdx],
				startY: p.points[pointIdx+1],
			}
			pointIdx += 2
		case VerbLineTo:
			current.verbs = append(current.verbs, verb)
			current.points = append(current.points, p.points[pointIdx], p.points[pointIdx+1])
			pointIdx += 2
		case VerbQuadTo:
			current.verbs = append(current.verbs, verb)
			current.points = append(current.points, p.points[pointIdx:pointIdx+4]...)
			pointIdx += 4
		case VerbCubicTo:
			current.verbs = append(current.verbs, verb)
			current.points = append(current.points, p.points[pointIdx:pointIdx+6]...)
			pointIdx += 6
		case VerbClose:
			current.verbs = append(current.verbs, verb)
			current.closed = true
		}
	}
	if len(current.verbs) > 0 {
		subpaths = append(subpaths, current)
	}

	// Reverse each subpath
	for _, sp := range subpaths {
		reverseSubpath(result, sp)
	}

	return result
}

// reverseSubpath reverses a single subpath and appends to result.
func reverseSubpath(result *Path, sp subpathData) {
	if len(sp.verbs) == 0 {
		return
	}

	// Find the end point (where we start the reversed path)
	lastX, lastY := sp.startX, sp.startY
	pointIdx := 0
	if sp.verbs[0] == VerbMoveTo {
		pointIdx = 2
	}

	for i := 1; i < len(sp.verbs); i++ {
		verb := sp.verbs[i]
		switch verb {
		case VerbLineTo:
			lastX, lastY = sp.points[pointIdx], sp.points[pointIdx+1]
			pointIdx += 2
		case VerbQuadTo:
			lastX, lastY = sp.points[pointIdx+2], sp.points[pointIdx+3]
			pointIdx += 4
		case VerbCubicTo:
			lastX, lastY = sp.points[pointIdx+4], sp.points[pointIdx+5]
			pointIdx += 6
		case VerbClose:
			lastX, lastY = sp.startX, sp.startY
		}
	}

	// Start reversed path from the end point
	result.MoveTo(lastX, lastY)

	// Walk backwards through the verbs
	pointIdx = len(sp.points)
	prevX, prevY := lastX, lastY

	for i := len(sp.verbs) - 1; i >= 1; i-- {
		verb := sp.verbs[i]
		switch verb {
		case VerbClose:
			// Skip close, will add at the end if needed
		case VerbLineTo:
			pointIdx -= 2
			result.LineTo(prevX, prevY)
			prevX, prevY = sp.points[pointIdx], sp.points[pointIdx+1]
		case VerbQuadTo:
			pointIdx -= 4
			// Reverse: swap control point order
			result.QuadTo(sp.points[pointIdx], sp.points[pointIdx+1], prevX, prevY)
			prevX, prevY = sp.points[pointIdx+2], sp.points[pointIdx+3]
		case VerbCubicTo:
			pointIdx -= 6
			// Reverse: swap control point order
			result.CubicTo(sp.points[pointIdx+2], sp.points[pointIdx+3],
				sp.points[pointIdx], sp.points[pointIdx+1], prevX, prevY)
			prevX, prevY = sp.points[pointIdx+4], sp.points[pointIdx+5]
		}
	}

	// Add final line to start point if we have one
	if len(sp.verbs) > 1 {
		result.LineTo(sp.startX, sp.startY)
	}

	if sp.closed {
		result.Close()
	}
}

// PathPool manages a pool of reusable Path objects.
type PathPool struct {
	paths []*Path
}

// NewPathPool creates a new path pool.
func NewPathPool() *PathPool {
	return &PathPool{
		paths: make([]*Path, 0, 8),
	}
}

// Get retrieves a path from the pool or creates a new one.
func (pp *PathPool) Get() *Path {
	if len(pp.paths) > 0 {
		p := pp.paths[len(pp.paths)-1]
		pp.paths = pp.paths[:len(pp.paths)-1]
		p.Reset()
		return p
	}
	return NewPath()
}

// Put returns a path to the pool for reuse.
func (pp *PathPool) Put(p *Path) {
	if p == nil {
		return
	}
	pp.paths = append(pp.paths, p)
}
