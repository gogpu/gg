package msdf

import (
	"math"

	"github.com/gogpu/gg/text"
)

// Contour represents a closed contour of edges.
// A glyph typically consists of one or more contours.
type Contour struct {
	// Edges is the list of edges that form this contour.
	Edges []Edge

	// Winding is the winding direction of the contour.
	// Positive = counter-clockwise (filled), Negative = clockwise (hole).
	Winding float64
}

// NewContour creates an empty contour.
func NewContour() *Contour {
	return &Contour{
		Edges: make([]Edge, 0),
	}
}

// AddEdge appends an edge to the contour.
func (c *Contour) AddEdge(e Edge) {
	c.Edges = append(c.Edges, e)
}

// Bounds returns the bounding box of all edges in the contour.
func (c *Contour) Bounds() Rect {
	if len(c.Edges) == 0 {
		return Rect{}
	}

	bounds := c.Edges[0].Bounds()
	for i := 1; i < len(c.Edges); i++ {
		bounds = bounds.Union(c.Edges[i].Bounds())
	}
	return bounds
}

// CalculateWinding calculates and stores the winding direction.
// Positive = CCW (outer contour), Negative = CW (inner/hole).
func (c *Contour) CalculateWinding() {
	// Calculate the signed area using the shoelace formula.
	// Sum up the cross products of consecutive edge endpoints.
	var area float64
	for i := range c.Edges {
		p0 := c.Edges[i].StartPoint()
		p1 := c.Edges[i].EndPoint()
		area += p0.Cross(p1)
	}
	c.Winding = area / 2
}

// IsClockwise returns true if the contour winds clockwise.
func (c *Contour) IsClockwise() bool {
	return c.Winding < 0
}

// Clone creates a deep copy of the contour.
func (c *Contour) Clone() *Contour {
	clone := &Contour{
		Edges:   make([]Edge, len(c.Edges)),
		Winding: c.Winding,
	}
	for i := range c.Edges {
		clone.Edges[i] = c.Edges[i].Clone()
	}
	return clone
}

// Shape represents a complete glyph shape consisting of contours.
type Shape struct {
	// Contours are the closed paths that make up the shape.
	Contours []*Contour

	// Bounds is the overall bounding box.
	Bounds Rect
}

// NewShape creates an empty shape.
func NewShape() *Shape {
	return &Shape{
		Contours: make([]*Contour, 0),
	}
}

// AddContour appends a contour to the shape.
func (s *Shape) AddContour(c *Contour) {
	s.Contours = append(s.Contours, c)
}

// CalculateBounds computes and stores the overall bounding box.
func (s *Shape) CalculateBounds() {
	if len(s.Contours) == 0 {
		s.Bounds = Rect{}
		return
	}

	s.Bounds = s.Contours[0].Bounds()
	for i := 1; i < len(s.Contours); i++ {
		s.Bounds = s.Bounds.Union(s.Contours[i].Bounds())
	}
}

// Validate checks that the shape is properly closed.
func (s *Shape) Validate() bool {
	for _, contour := range s.Contours {
		if len(contour.Edges) == 0 {
			continue
		}

		// Check that contour is closed (last endpoint == first startpoint)
		first := contour.Edges[0].StartPoint()
		last := contour.Edges[len(contour.Edges)-1].EndPoint()

		dx := math.Abs(first.X - last.X)
		dy := math.Abs(first.Y - last.Y)
		if dx > 1e-6 || dy > 1e-6 {
			return false
		}
	}
	return true
}

// EdgeCount returns the total number of edges across all contours.
func (s *Shape) EdgeCount() int {
	count := 0
	for _, c := range s.Contours {
		count += len(c.Edges)
	}
	return count
}

// FromOutline converts a GlyphOutline to a Shape with colored edges.
// This is the main entry point for MSDF generation.
func FromOutline(outline *text.GlyphOutline) *Shape {
	if outline == nil || len(outline.Segments) == 0 {
		return NewShape()
	}

	shape := NewShape()
	var currentContour *Contour
	var currentPos Point

	for _, seg := range outline.Segments {
		switch seg.Op {
		case text.OutlineOpMoveTo:
			// Start a new contour
			if currentContour != nil && len(currentContour.Edges) > 0 {
				currentContour.CalculateWinding()
				shape.AddContour(currentContour)
			}
			currentContour = NewContour()
			currentPos = Point{
				X: float64(seg.Points[0].X),
				Y: float64(seg.Points[0].Y),
			}

		case text.OutlineOpLineTo:
			if currentContour == nil {
				currentContour = NewContour()
			}
			endPoint := Point{
				X: float64(seg.Points[0].X),
				Y: float64(seg.Points[0].Y),
			}
			// Skip degenerate lines
			if endPoint.Sub(currentPos).LengthSquared() > 1e-12 {
				edge := NewLinearEdge(currentPos, endPoint)
				currentContour.AddEdge(edge)
			}
			currentPos = endPoint

		case text.OutlineOpQuadTo:
			if currentContour == nil {
				currentContour = NewContour()
			}
			controlPoint := Point{
				X: float64(seg.Points[0].X),
				Y: float64(seg.Points[0].Y),
			}
			endPoint := Point{
				X: float64(seg.Points[1].X),
				Y: float64(seg.Points[1].Y),
			}
			edge := NewQuadraticEdge(currentPos, controlPoint, endPoint)
			currentContour.AddEdge(edge)
			currentPos = endPoint

		case text.OutlineOpCubicTo:
			if currentContour == nil {
				currentContour = NewContour()
			}
			control1 := Point{
				X: float64(seg.Points[0].X),
				Y: float64(seg.Points[0].Y),
			}
			control2 := Point{
				X: float64(seg.Points[1].X),
				Y: float64(seg.Points[1].Y),
			}
			endPoint := Point{
				X: float64(seg.Points[2].X),
				Y: float64(seg.Points[2].Y),
			}
			edge := NewCubicEdge(currentPos, control1, control2, endPoint)
			currentContour.AddEdge(edge)
			currentPos = endPoint
		}
	}

	// Add the last contour
	if currentContour != nil && len(currentContour.Edges) > 0 {
		currentContour.CalculateWinding()
		shape.AddContour(currentContour)
	}

	shape.CalculateBounds()
	return shape
}

// AssignColors assigns edge colors to preserve corners.
// This is the key MSDF innovation - corners get different colors
// so that the median operation can preserve them.
func AssignColors(shape *Shape, angleThreshold float64) {
	for _, contour := range shape.Contours {
		if len(contour.Edges) == 0 {
			continue
		}

		assignContourColors(contour, angleThreshold)
	}
}

// assignContourColors assigns colors to edges in a single contour.
func assignContourColors(contour *Contour, angleThreshold float64) {
	n := len(contour.Edges)
	if n == 0 {
		return
	}

	if n == 1 {
		// Single edge gets all colors
		contour.Edges[0].Color = ColorWhite
		return
	}

	// Detect corners (sharp angle changes)
	corners := make([]int, 0)
	for i := 0; i < n; i++ {
		// Get the direction at the end of this edge and start of next
		prevEdge := &contour.Edges[i]
		nextEdge := &contour.Edges[(i+1)%n]

		// Direction leaving this edge
		dirOut := prevEdge.DirectionAt(1).Normalized()
		// Direction entering next edge
		dirIn := nextEdge.DirectionAt(0).Normalized()

		// Angle between them
		angle := AngleBetween(dirOut, dirIn)

		if angle > angleThreshold {
			corners = append(corners, i)
		}
	}

	if len(corners) == 0 {
		// No corners - use simple alternating colors
		for i := range contour.Edges {
			contour.Edges[i].Color = ColorWhite
		}
		return
	}

	// Assign colors based on corners
	// Each segment between corners gets a different color
	colors := []EdgeColor{ColorCyan, ColorMagenta, ColorYellow}

	colorIdx := 0
	for i := 0; i < len(corners); i++ {
		start := corners[i]
		end := corners[(i+1)%len(corners)]

		// Assign color to edges from start+1 to end (inclusive)
		color := colors[colorIdx%len(colors)]
		colorIdx++

		if end <= start {
			end += n
		}

		for j := start + 1; j <= end; j++ {
			contour.Edges[j%n].Color = color
		}
	}

	// Edges at corners should use the XOR of adjacent colors
	for _, cornerIdx := range corners {
		prevColor := contour.Edges[cornerIdx].Color
		nextColor := contour.Edges[(cornerIdx+1)%n].Color

		// Corner gets both adjacent colors (to preserve sharpness)
		if prevColor == nextColor {
			// Same color on both sides - use white
			contour.Edges[cornerIdx].Color = ColorWhite
		} else {
			// Use the union of both colors
			contour.Edges[cornerIdx].Color = prevColor | nextColor
		}
	}
}

// SwitchColor returns the next color in the cycle.
// Used for edge coloring algorithm.
func SwitchColor(current EdgeColor, seed int) EdgeColor {
	colors := []EdgeColor{ColorCyan, ColorMagenta, ColorYellow}
	for i, c := range colors {
		if c == current {
			return colors[(i+1+seed)%len(colors)]
		}
	}
	return colors[seed%len(colors)]
}

// EdgeSelectorFunc selects edges based on color.
type EdgeSelectorFunc func(color EdgeColor) bool

// SelectRed returns true if the color includes red.
func SelectRed(color EdgeColor) bool {
	return color.HasRed()
}

// SelectGreen returns true if the color includes green.
func SelectGreen(color EdgeColor) bool {
	return color.HasGreen()
}

// SelectBlue returns true if the color includes blue.
func SelectBlue(color EdgeColor) bool {
	return color.HasBlue()
}
