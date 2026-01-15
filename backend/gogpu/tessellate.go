package gogpu

import (
	"math"

	"github.com/gogpu/gg"
)

// Vertex represents a triangle vertex for GPU rendering.
type Vertex struct {
	X, Y       float32 // Position in normalized device coordinates
	R, G, B, A float32 // Color (premultiplied alpha)
}

// TessellateFill converts a filled path to triangles using fan triangulation.
// Returns nil if path has fewer than 3 points.
// Coordinates are converted to NDC (-1 to 1) based on width/height.
func TessellateFill(path *gg.Path, paint *gg.Paint, width, height int) []Vertex {
	if path == nil || paint == nil || width <= 0 || height <= 0 {
		return nil
	}

	// Get flattened path points
	points := flattenPath(path)
	if len(points) < 3 {
		return nil
	}

	// Get color from paint (solid color at origin)
	c := paint.ColorAt(0, 0)
	// Premultiply alpha for GPU blending
	pc := c.Premultiply()
	r32 := float32(pc.R)
	g32 := float32(pc.G)
	b32 := float32(pc.B)
	a32 := float32(pc.A)

	// Fan triangulation: use first point as pivot
	// For N points, generates N-2 triangles
	vertices := make([]Vertex, 0, (len(points)-2)*3)

	p0 := toNDC(points[0], width, height)
	for i := 1; i < len(points)-1; i++ {
		p1 := toNDC(points[i], width, height)
		p2 := toNDC(points[i+1], width, height)

		// Add triangle vertices
		vertices = append(vertices,
			Vertex{p0.X, p0.Y, r32, g32, b32, a32},
			Vertex{p1.X, p1.Y, r32, g32, b32, a32},
			Vertex{p2.X, p2.Y, r32, g32, b32, a32},
		)
	}

	return vertices
}

// point2D is a simple 2D point for internal use.
type point2D struct {
	X, Y float32
}

// toNDC converts pixel coordinates to normalized device coordinates.
// NDC range: -1 (left/bottom) to +1 (right/top)
func toNDC(p point2D, width, height int) point2D {
	return point2D{
		X: (p.X/float32(width))*2.0 - 1.0,
		Y: 1.0 - (p.Y/float32(height))*2.0, // Flip Y axis
	}
}

// flattenPath converts a path with curves to a series of line segments.
// It iterates through path elements and flattens Bezier curves to polylines.
func flattenPath(path *gg.Path) []point2D {
	elements := path.Elements()
	if len(elements) == 0 {
		return nil
	}

	var points []point2D
	var current point2D
	var subpathStart point2D

	for _, elem := range elements {
		switch e := elem.(type) {
		case gg.MoveTo:
			current = point2D{X: float32(e.Point.X), Y: float32(e.Point.Y)}
			subpathStart = current
			points = append(points, current)

		case gg.LineTo:
			current = point2D{X: float32(e.Point.X), Y: float32(e.Point.Y)}
			points = append(points, current)

		case gg.QuadTo:
			// Flatten quadratic Bezier curve
			ctrl := point2D{X: float32(e.Control.X), Y: float32(e.Control.Y)}
			end := point2D{X: float32(e.Point.X), Y: float32(e.Point.Y)}
			flattenedQuad := flattenQuadratic(current, ctrl, end, defaultFlatness)
			// Skip the first point as it's the current point
			if len(flattenedQuad) > 1 {
				points = append(points, flattenedQuad[1:]...)
			}
			current = end

		case gg.CubicTo:
			// Flatten cubic Bezier curve
			ctrl1 := point2D{X: float32(e.Control1.X), Y: float32(e.Control1.Y)}
			ctrl2 := point2D{X: float32(e.Control2.X), Y: float32(e.Control2.Y)}
			end := point2D{X: float32(e.Point.X), Y: float32(e.Point.Y)}
			flattenedCubic := flattenCubic(current, ctrl1, ctrl2, end, defaultFlatness)
			// Skip the first point as it's the current point
			if len(flattenedCubic) > 1 {
				points = append(points, flattenedCubic[1:]...)
			}
			current = end

		case gg.Close:
			// Close the subpath by returning to start
			if current != subpathStart {
				points = append(points, subpathStart)
				current = subpathStart
			}
		}
	}

	return points
}

// defaultFlatness is the maximum distance error allowed when flattening curves.
// Smaller values produce smoother curves but more vertices.
const defaultFlatness = 0.25

// flattenQuadratic flattens a quadratic Bezier curve to line segments.
// Uses adaptive subdivision based on flatness tolerance.
func flattenQuadratic(p0, p1, p2 point2D, flatness float32) []point2D {
	// Check if curve is flat enough
	if isQuadraticFlat(p0, p1, p2, flatness) {
		return []point2D{p0, p2}
	}

	// Subdivide using de Casteljau's algorithm
	q0 := midpoint(p0, p1)
	q1 := midpoint(p1, p2)
	r := midpoint(q0, q1)

	// Recursively flatten both halves
	left := flattenQuadratic(p0, q0, r, flatness)
	right := flattenQuadratic(r, q1, p2, flatness)

	// Combine results, avoiding duplicate midpoint
	return append(left[:len(left)-1], right...)
}

// flattenCubic flattens a cubic Bezier curve to line segments.
// Uses adaptive subdivision based on flatness tolerance.
func flattenCubic(p0, p1, p2, p3 point2D, flatness float32) []point2D {
	// Check if curve is flat enough
	if isCubicFlat(p0, p1, p2, p3, flatness) {
		return []point2D{p0, p3}
	}

	// Subdivide using de Casteljau's algorithm
	q0 := midpoint(p0, p1)
	q1 := midpoint(p1, p2)
	q2 := midpoint(p2, p3)
	r0 := midpoint(q0, q1)
	r1 := midpoint(q1, q2)
	s := midpoint(r0, r1)

	// Recursively flatten both halves
	left := flattenCubic(p0, q0, r0, s, flatness)
	right := flattenCubic(s, r1, q2, p3, flatness)

	// Combine results, avoiding duplicate midpoint
	return append(left[:len(left)-1], right...)
}

// isQuadraticFlat checks if a quadratic Bezier is flat enough.
// Uses the distance from control point to the line p0-p2.
func isQuadraticFlat(p0, p1, p2 point2D, flatness float32) bool {
	// Calculate distance from control point to line p0-p2
	d := pointToLineDistance(p1, p0, p2)
	return d <= flatness
}

// isCubicFlat checks if a cubic Bezier is flat enough.
// Uses the maximum distance from control points to the line p0-p3.
func isCubicFlat(p0, p1, p2, p3 point2D, flatness float32) bool {
	// Calculate distances from control points to line p0-p3
	d1 := pointToLineDistance(p1, p0, p3)
	d2 := pointToLineDistance(p2, p0, p3)
	maxD := d1
	if d2 > maxD {
		maxD = d2
	}
	return maxD <= flatness
}

// pointToLineDistance calculates the perpendicular distance from point p to line a-b.
func pointToLineDistance(p, a, b point2D) float32 {
	// Vector from a to b
	dx := b.X - a.X
	dy := b.Y - a.Y

	// Length squared of the line segment
	lenSq := dx*dx + dy*dy
	if lenSq < 1e-10 {
		// a and b are the same point, return distance to a
		pdx := p.X - a.X
		pdy := p.Y - a.Y
		return float32(math.Sqrt(float64(pdx*pdx + pdy*pdy)))
	}

	// Calculate perpendicular distance using cross product
	// |cross(b-a, p-a)| / |b-a|
	cross := dx*(p.Y-a.Y) - dy*(p.X-a.X)
	return float32(math.Abs(float64(cross)) / math.Sqrt(float64(lenSq)))
}

// midpoint returns the midpoint between two points.
func midpoint(a, b point2D) point2D {
	return point2D{
		X: (a.X + b.X) * 0.5,
		Y: (a.Y + b.Y) * 0.5,
	}
}
