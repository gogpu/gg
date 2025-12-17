package scene

import "math"

// Shape is the interface for geometric shapes that can be converted to paths.
// All shapes must be able to provide a path representation and their bounds.
type Shape interface {
	// ToPath converts the shape to a Path for encoding.
	ToPath() *Path

	// Bounds returns the bounding rectangle of the shape.
	Bounds() Rect
}

// RectShape represents an axis-aligned rectangle.
type RectShape struct {
	X, Y          float32 // Top-left corner
	Width, Height float32 // Dimensions
}

// NewRectShape creates a new rectangle shape.
func NewRectShape(x, y, width, height float32) *RectShape {
	return &RectShape{X: x, Y: y, Width: width, Height: height}
}

// ToPath converts the rectangle to a Path.
func (r *RectShape) ToPath() *Path {
	return NewPath().Rectangle(r.X, r.Y, r.Width, r.Height)
}

// Bounds returns the bounding rectangle.
func (r *RectShape) Bounds() Rect {
	return Rect{
		MinX: r.X,
		MinY: r.Y,
		MaxX: r.X + r.Width,
		MaxY: r.Y + r.Height,
	}
}

// Contains returns true if the point (px, py) is inside the rectangle.
func (r *RectShape) Contains(px, py float32) bool {
	return px >= r.X && px <= r.X+r.Width &&
		py >= r.Y && py <= r.Y+r.Height
}

// RoundedRectShape represents a rectangle with rounded corners.
type RoundedRectShape struct {
	X, Y          float32 // Top-left corner
	Width, Height float32 // Dimensions
	Radius        float32 // Corner radius (same for all corners)
}

// NewRoundedRectShape creates a new rounded rectangle shape.
func NewRoundedRectShape(x, y, width, height, radius float32) *RoundedRectShape {
	return &RoundedRectShape{
		X: x, Y: y, Width: width, Height: height, Radius: radius,
	}
}

// ToPath converts the rounded rectangle to a Path.
func (r *RoundedRectShape) ToPath() *Path {
	return NewPath().RoundedRectangle(r.X, r.Y, r.Width, r.Height, r.Radius)
}

// Bounds returns the bounding rectangle.
func (r *RoundedRectShape) Bounds() Rect {
	return Rect{
		MinX: r.X,
		MinY: r.Y,
		MaxX: r.X + r.Width,
		MaxY: r.Y + r.Height,
	}
}

// CircleShape represents a circle.
type CircleShape struct {
	CX, CY float32 // Center
	R      float32 // Radius
}

// NewCircleShape creates a new circle shape.
func NewCircleShape(cx, cy, r float32) *CircleShape {
	return &CircleShape{CX: cx, CY: cy, R: r}
}

// ToPath converts the circle to a Path.
func (c *CircleShape) ToPath() *Path {
	return NewPath().Circle(c.CX, c.CY, c.R)
}

// Bounds returns the bounding rectangle.
func (c *CircleShape) Bounds() Rect {
	return Rect{
		MinX: c.CX - c.R,
		MinY: c.CY - c.R,
		MaxX: c.CX + c.R,
		MaxY: c.CY + c.R,
	}
}

// Contains returns true if the point (px, py) is inside the circle.
func (c *CircleShape) Contains(px, py float32) bool {
	dx := px - c.CX
	dy := py - c.CY
	return dx*dx+dy*dy <= c.R*c.R
}

// EllipseShape represents an axis-aligned ellipse.
type EllipseShape struct {
	CX, CY float32 // Center
	RX, RY float32 // Radii
}

// NewEllipseShape creates a new ellipse shape.
func NewEllipseShape(cx, cy, rx, ry float32) *EllipseShape {
	return &EllipseShape{CX: cx, CY: cy, RX: rx, RY: ry}
}

// ToPath converts the ellipse to a Path.
func (e *EllipseShape) ToPath() *Path {
	return NewPath().Ellipse(e.CX, e.CY, e.RX, e.RY)
}

// Bounds returns the bounding rectangle.
func (e *EllipseShape) Bounds() Rect {
	return Rect{
		MinX: e.CX - e.RX,
		MinY: e.CY - e.RY,
		MaxX: e.CX + e.RX,
		MaxY: e.CY + e.RY,
	}
}

// Contains returns true if the point (px, py) is inside the ellipse.
func (e *EllipseShape) Contains(px, py float32) bool {
	if e.RX == 0 || e.RY == 0 {
		return false
	}
	dx := (px - e.CX) / e.RX
	dy := (py - e.CY) / e.RY
	return dx*dx+dy*dy <= 1
}

// LineShape represents a line segment.
type LineShape struct {
	X1, Y1 float32 // Start point
	X2, Y2 float32 // End point
}

// NewLineShape creates a new line shape.
func NewLineShape(x1, y1, x2, y2 float32) *LineShape {
	return &LineShape{X1: x1, Y1: y1, X2: x2, Y2: y2}
}

// ToPath converts the line to a Path.
func (l *LineShape) ToPath() *Path {
	return NewPath().MoveTo(l.X1, l.Y1).LineTo(l.X2, l.Y2)
}

// Bounds returns the bounding rectangle.
func (l *LineShape) Bounds() Rect {
	return Rect{
		MinX: min32(l.X1, l.X2),
		MinY: min32(l.Y1, l.Y2),
		MaxX: max32(l.X1, l.X2),
		MaxY: max32(l.Y1, l.Y2),
	}
}

// Length returns the length of the line segment.
func (l *LineShape) Length() float32 {
	dx := l.X2 - l.X1
	dy := l.Y2 - l.Y1
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// PathShape wraps a Path as a Shape.
type PathShape struct {
	path *Path
}

// NewPathShape creates a new path shape.
func NewPathShape(path *Path) *PathShape {
	return &PathShape{path: path}
}

// ToPath returns the underlying path.
func (ps *PathShape) ToPath() *Path {
	return ps.path
}

// Bounds returns the bounding rectangle of the path.
func (ps *PathShape) Bounds() Rect {
	if ps.path == nil {
		return EmptyRect()
	}
	return ps.path.Bounds()
}

// PolygonShape represents a closed polygon.
type PolygonShape struct {
	points []float32 // x1, y1, x2, y2, ...
}

// NewPolygonShape creates a new polygon shape from a list of points.
// Points should be provided as x, y pairs.
func NewPolygonShape(points ...float32) *PolygonShape {
	if len(points)%2 != 0 {
		// Ignore the last point if odd number
		points = points[:len(points)-1]
	}
	ps := &PolygonShape{
		points: make([]float32, len(points)),
	}
	copy(ps.points, points)
	return ps
}

// ToPath converts the polygon to a Path.
func (p *PolygonShape) ToPath() *Path {
	if len(p.points) < 4 { // Need at least 2 points (4 floats)
		return NewPath()
	}

	path := NewPath()
	path.MoveTo(p.points[0], p.points[1])

	for i := 2; i < len(p.points); i += 2 {
		path.LineTo(p.points[i], p.points[i+1])
	}

	return path.Close()
}

// Bounds returns the bounding rectangle.
func (p *PolygonShape) Bounds() Rect {
	if len(p.points) < 2 {
		return EmptyRect()
	}

	bounds := Rect{
		MinX: p.points[0],
		MinY: p.points[1],
		MaxX: p.points[0],
		MaxY: p.points[1],
	}

	for i := 2; i < len(p.points); i += 2 {
		bounds = bounds.UnionPoint(p.points[i], p.points[i+1])
	}

	return bounds
}

// PointCount returns the number of vertices in the polygon.
func (p *PolygonShape) PointCount() int {
	return len(p.points) / 2
}

// Point returns the i-th vertex of the polygon.
func (p *PolygonShape) Point(i int) (x, y float32, ok bool) {
	idx := i * 2
	if idx < 0 || idx+1 >= len(p.points) {
		return 0, 0, false
	}
	return p.points[idx], p.points[idx+1], true
}

// RegularPolygonShape represents a regular polygon (all sides equal length).
type RegularPolygonShape struct {
	CX, CY   float32 // Center
	R        float32 // Radius (distance from center to vertices)
	Sides    int     // Number of sides
	Rotation float32 // Rotation angle in radians
}

// NewRegularPolygonShape creates a new regular polygon shape.
func NewRegularPolygonShape(cx, cy, r float32, sides int, rotation float32) *RegularPolygonShape {
	if sides < 3 {
		sides = 3
	}
	return &RegularPolygonShape{
		CX: cx, CY: cy, R: r, Sides: sides, Rotation: rotation,
	}
}

// ToPath converts the regular polygon to a Path.
func (rp *RegularPolygonShape) ToPath() *Path {
	path := NewPath()

	angleStep := 2 * math.Pi / float64(rp.Sides)
	startAngle := float64(rp.Rotation) - math.Pi/2 // Start from top

	for i := 0; i < rp.Sides; i++ {
		angle := startAngle + angleStep*float64(i)
		x := rp.CX + rp.R*float32(math.Cos(angle))
		y := rp.CY + rp.R*float32(math.Sin(angle))

		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}

	return path.Close()
}

// Bounds returns the bounding rectangle.
func (rp *RegularPolygonShape) Bounds() Rect {
	// For simplicity, use the circumscribed circle bounds
	// More accurate bounds would require computing all vertices
	return Rect{
		MinX: rp.CX - rp.R,
		MinY: rp.CY - rp.R,
		MaxX: rp.CX + rp.R,
		MaxY: rp.CY + rp.R,
	}
}

// StarShape represents a star shape.
type StarShape struct {
	CX, CY      float32 // Center
	OuterRadius float32 // Outer radius (points)
	InnerRadius float32 // Inner radius (valleys)
	Points      int     // Number of points
	Rotation    float32 // Rotation angle in radians
}

// NewStarShape creates a new star shape.
func NewStarShape(cx, cy, outerRadius, innerRadius float32, points int, rotation float32) *StarShape {
	if points < 3 {
		points = 5 // Default to 5-pointed star
	}
	return &StarShape{
		CX: cx, CY: cy,
		OuterRadius: outerRadius, InnerRadius: innerRadius,
		Points: points, Rotation: rotation,
	}
}

// ToPath converts the star to a Path.
func (s *StarShape) ToPath() *Path {
	path := NewPath()

	// Each star point alternates between outer and inner vertices
	angleStep := math.Pi / float64(s.Points)
	startAngle := float64(s.Rotation) - math.Pi/2 // Start from top

	for i := 0; i < s.Points*2; i++ {
		angle := startAngle + angleStep*float64(i)
		var r float32
		if i%2 == 0 {
			r = s.OuterRadius
		} else {
			r = s.InnerRadius
		}

		x := s.CX + r*float32(math.Cos(angle))
		y := s.CY + r*float32(math.Sin(angle))

		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}

	return path.Close()
}

// Bounds returns the bounding rectangle.
func (s *StarShape) Bounds() Rect {
	// Use outer radius for bounds (conservative)
	return Rect{
		MinX: s.CX - s.OuterRadius,
		MinY: s.CY - s.OuterRadius,
		MaxX: s.CX + s.OuterRadius,
		MaxY: s.CY + s.OuterRadius,
	}
}

// ArcShape represents an arc (portion of an ellipse outline).
type ArcShape struct {
	CX, CY         float32 // Center
	RX, RY         float32 // Radii
	StartAngle     float32 // Start angle in radians
	EndAngle       float32 // End angle in radians
	SweepClockwise bool    // Direction
}

// NewArcShape creates a new arc shape.
func NewArcShape(cx, cy, rx, ry, startAngle, endAngle float32, sweepClockwise bool) *ArcShape {
	return &ArcShape{
		CX: cx, CY: cy, RX: rx, RY: ry,
		StartAngle: startAngle, EndAngle: endAngle,
		SweepClockwise: sweepClockwise,
	}
}

// ToPath converts the arc to a Path.
func (a *ArcShape) ToPath() *Path {
	return NewPath().Arc(a.CX, a.CY, a.RX, a.RY, a.StartAngle, a.EndAngle, a.SweepClockwise)
}

// Bounds returns a conservative bounding rectangle.
func (a *ArcShape) Bounds() Rect {
	// Conservative: use the full ellipse bounds
	// More accurate bounds would compute arc extrema
	return Rect{
		MinX: a.CX - a.RX,
		MinY: a.CY - a.RY,
		MaxX: a.CX + a.RX,
		MaxY: a.CY + a.RY,
	}
}

// PieShape represents a pie slice (wedge).
type PieShape struct {
	CX, CY         float32 // Center
	R              float32 // Radius
	StartAngle     float32 // Start angle in radians
	EndAngle       float32 // End angle in radians
	SweepClockwise bool    // Direction
}

// NewPieShape creates a new pie shape.
func NewPieShape(cx, cy, r, startAngle, endAngle float32, sweepClockwise bool) *PieShape {
	return &PieShape{
		CX: cx, CY: cy, R: r,
		StartAngle: startAngle, EndAngle: endAngle,
		SweepClockwise: sweepClockwise,
	}
}

// ToPath converts the pie to a Path.
func (p *PieShape) ToPath() *Path {
	path := NewPath()

	// Start at center
	path.MoveTo(p.CX, p.CY)

	// Line to start of arc
	startX := p.CX + p.R*float32(math.Cos(float64(p.StartAngle)))
	startY := p.CY + p.R*float32(math.Sin(float64(p.StartAngle)))
	path.LineTo(startX, startY)

	// Add arc using the Path's Arc method (but we need the interior arc)
	// We'll manually add the arc segments
	arc := NewPath()
	arc.Arc(p.CX, p.CY, p.R, p.R, p.StartAngle, p.EndAngle, p.SweepClockwise)

	// Copy arc verbs and points (skip the initial MoveTo)
	for i, verb := range arc.verbs {
		if i == 0 && verb == VerbMoveTo {
			continue // Skip initial MoveTo
		}
		path.verbs = append(path.verbs, verb)
	}
	// Copy points (skip first 2 for the MoveTo)
	if len(arc.points) > 2 {
		path.points = append(path.points, arc.points[2:]...)
	}

	return path.Close()
}

// Bounds returns a conservative bounding rectangle.
func (p *PieShape) Bounds() Rect {
	// Conservative: use the full circle bounds
	return Rect{
		MinX: p.CX - p.R,
		MinY: p.CY - p.R,
		MaxX: p.CX + p.R,
		MaxY: p.CY + p.R,
	}
}

// TransformShape wraps a shape with a transformation.
type TransformShape struct {
	shape     Shape
	transform Affine
}

// NewTransformShape creates a transformed shape.
func NewTransformShape(shape Shape, transform Affine) *TransformShape {
	return &TransformShape{shape: shape, transform: transform}
}

// ToPath converts the shape to a transformed Path.
func (ts *TransformShape) ToPath() *Path {
	if ts.shape == nil {
		return NewPath()
	}
	return ts.shape.ToPath().Transform(ts.transform)
}

// Bounds returns the transformed bounding rectangle.
// Note: This is a conservative approximation.
func (ts *TransformShape) Bounds() Rect {
	if ts.shape == nil {
		return EmptyRect()
	}

	b := ts.shape.Bounds()
	if b.IsEmpty() {
		return b
	}

	// Transform all four corners and compute new bounds
	corners := [][2]float32{
		{b.MinX, b.MinY},
		{b.MaxX, b.MinY},
		{b.MaxX, b.MaxY},
		{b.MinX, b.MaxY},
	}

	result := EmptyRect()
	for _, c := range corners {
		x, y := ts.transform.TransformPoint(c[0], c[1])
		result = result.UnionPoint(x, y)
	}

	return result
}

// CompositeShape combines multiple shapes into one.
type CompositeShape struct {
	shapes []Shape
}

// NewCompositeShape creates a new composite shape.
func NewCompositeShape(shapes ...Shape) *CompositeShape {
	return &CompositeShape{shapes: shapes}
}

// AddShape adds a shape to the composite.
func (cs *CompositeShape) AddShape(shape Shape) {
	cs.shapes = append(cs.shapes, shape)
}

// ToPath converts all shapes to a single Path.
func (cs *CompositeShape) ToPath() *Path {
	result := NewPath()

	for _, shape := range cs.shapes {
		if shape == nil {
			continue
		}
		p := shape.ToPath()
		if p == nil || p.IsEmpty() {
			continue
		}

		// Append the path data
		result.verbs = append(result.verbs, p.verbs...)
		result.points = append(result.points, p.points...)
		result.bounds = result.bounds.Union(p.bounds)
	}

	return result
}

// Bounds returns the union of all shape bounds.
func (cs *CompositeShape) Bounds() Rect {
	result := EmptyRect()
	for _, shape := range cs.shapes {
		if shape != nil {
			result = result.Union(shape.Bounds())
		}
	}
	return result
}

// ShapeCount returns the number of shapes in the composite.
func (cs *CompositeShape) ShapeCount() int {
	return len(cs.shapes)
}
