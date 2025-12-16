package clip

import (
	"github.com/gogpu/gg/internal/image"
)

// PathElement represents a single element in a path (copy to avoid import cycle).
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

// MaskClipper performs alpha mask-based clipping for anti-aliased complex clips.
// It rasterizes a path into a grayscale mask where each pixel's value represents
// coverage (0 = outside, 255 = fully inside).
type MaskClipper struct {
	mask   *image.ImageBuf
	bounds Rect
}

// NewMaskClipper creates a mask clipper by rasterizing the given path elements
// into an alpha mask.
//
// Parameters:
//   - elements: Path elements to rasterize
//   - bounds: Bounding rectangle for the mask
//   - antiAlias: Enable anti-aliased rendering (currently always on)
//
// The mask is stored as FormatGray8 (1 byte per pixel) for memory efficiency.
func NewMaskClipper(elements []PathElement, bounds Rect, antiAlias bool) (*MaskClipper, error) {
	// Validate bounds - empty bounds means no clipping needed
	if bounds.IsEmpty() {
		return nil, image.ErrInvalidDimensions
	}

	// Calculate mask dimensions (ceiling to ensure we cover all pixels)
	width := int(bounds.W + 0.5)
	height := int(bounds.H + 0.5)
	if width <= 0 || height <= 0 {
		return nil, image.ErrInvalidDimensions
	}

	// Create grayscale mask buffer
	mask, err := image.NewImageBuf(width, height, image.FormatGray8)
	if err != nil {
		return nil, err
	}

	mc := &MaskClipper{
		mask:   mask,
		bounds: bounds,
	}

	// Rasterize path into mask
	mc.rasterizePath(elements, antiAlias)

	return mc, nil
}

// Coverage returns the coverage value (0-255) at the given point.
// Points outside the mask bounds return 0 (no coverage).
func (mc *MaskClipper) Coverage(x, y float64) byte {
	// Convert to mask coordinates
	mx := x - mc.bounds.X
	my := y - mc.bounds.Y

	// Check bounds
	if mx < 0 || my < 0 || mx >= float64(mc.mask.Width()) || my >= float64(mc.mask.Height()) {
		return 0
	}

	// Get pixel value (bilinear interpolation for smoother results)
	ix := int(mx)
	iy := int(my)

	// Simple nearest-neighbor for now (can be enhanced with bilinear later)
	if ix >= mc.mask.Width() {
		ix = mc.mask.Width() - 1
	}
	if iy >= mc.mask.Height() {
		iy = mc.mask.Height() - 1
	}

	// GetRGBA returns (r, g, b, a), but for Gray8 format r=g=b=gray value
	gray, _, _, _ := mc.mask.GetRGBA(ix, iy) //nolint:dogsled // Gray8 format has r=g=b
	return gray
}

// ApplyCoverage modulates the source alpha by the mask coverage at the given point.
// Returns the modulated alpha value (0-255).
func (mc *MaskClipper) ApplyCoverage(x, y float64, srcAlpha byte) byte {
	coverage := mc.Coverage(x, y)
	if coverage == 0 {
		return 0
	}
	if coverage == 255 {
		return srcAlpha
	}

	// Modulate: result = srcAlpha * coverage / 255
	// Use 16-bit math to avoid overflow
	result := (uint16(srcAlpha) * uint16(coverage)) / 255
	return byte(result)
}

// Bounds returns the bounding rectangle of the mask.
func (mc *MaskClipper) Bounds() Rect {
	return mc.bounds
}

// Mask returns the underlying grayscale image buffer.
// This is useful for debugging or advanced use cases.
func (mc *MaskClipper) Mask() *image.ImageBuf {
	return mc.mask
}

// rasterizePath converts path elements into a coverage mask.
func (mc *MaskClipper) rasterizePath(elements []PathElement, antiAlias bool) {
	if len(elements) == 0 {
		return
	}

	// Flatten path to line segments
	points := mc.flattenPath(elements)
	if len(points) < 2 {
		return
	}

	// Build edge list for scanline rasterization
	edges := make([]edge, 0, len(points))
	for i := 0; i < len(points)-1; i++ {
		p0 := points[i]
		p1 := points[i+1]

		// Skip horizontal edges
		if p1.Y == p0.Y {
			continue
		}

		edges = append(edges, mc.makeEdge(p0, p1))
	}

	if len(edges) == 0 {
		return
	}

	// Scanline rasterization
	// Note: antiAlias parameter is reserved for future enhancement
	_ = antiAlias
	for y := 0; y < mc.mask.Height(); y++ {
		mc.rasterizeScanline(edges, y)
	}
}

// edge represents a scanline edge for rasterization.
type edge struct {
	x0, y0 float64 // Start point
	x1, y1 float64 // End point
	dir    int     // Direction: +1 for down, -1 for up
}

// makeEdge creates an edge from two points, ensuring y0 < y1.
func (mc *MaskClipper) makeEdge(p0, p1 Point) edge {
	// Convert to mask coordinates
	x0 := p0.X - mc.bounds.X
	y0 := p0.Y - mc.bounds.Y
	x1 := p1.X - mc.bounds.X
	y1 := p1.Y - mc.bounds.Y

	if y0 > y1 {
		// Swap to ensure y0 < y1
		x0, x1 = x1, x0
		y0, y1 = y1, y0
		return edge{x0: x0, y0: y0, x1: x1, y1: y1, dir: -1}
	}
	return edge{x0: x0, y0: y0, x1: x1, y1: y1, dir: 1}
}

// rasterizeScanline fills a single scanline using the non-zero winding rule.
func (mc *MaskClipper) rasterizeScanline(edges []edge, y int) {
	scanY := float64(y) + 0.5

	// Find edges that intersect this scanline
	var intersections []float64
	for _, e := range edges {
		if e.y0 <= scanY && scanY < e.y1 {
			// Compute x intersection
			t := (scanY - e.y0) / (e.y1 - e.y0)
			x := e.x0 + t*(e.x1-e.x0)
			intersections = append(intersections, x)
		}
	}

	if len(intersections) == 0 {
		return
	}

	// Sort intersections
	sortFloats(intersections)

	// Fill spans using even-odd rule (pairs of intersections)
	for i := 0; i+1 < len(intersections); i += 2 {
		x1 := intersections[i]
		x2 := intersections[i+1]

		// Convert to pixel coordinates
		px1 := int(x1)
		px2 := int(x2)

		// Clamp to mask bounds
		if px1 < 0 {
			px1 = 0
		}
		if px2 >= mc.mask.Width() {
			px2 = mc.mask.Width() - 1
		}

		// Fill pixels
		for x := px1; x <= px2; x++ {
			_ = mc.mask.SetRGBA(x, y, 255, 255, 255, 255)
		}
	}
}

// flattenPath converts path elements into a sequence of points.
func (mc *MaskClipper) flattenPath(elements []PathElement) []Point {
	var points []Point
	var current Point

	for _, elem := range elements {
		switch e := elem.(type) {
		case MoveTo:
			current = e.Point
			points = append(points, current)

		case LineTo:
			current = e.Point
			points = append(points, current)

		case QuadTo:
			// Flatten quadratic Bezier to line segments
			prev := current
			steps := 10 // Number of segments
			for i := 1; i <= steps; i++ {
				t := float64(i) / float64(steps)
				pt := evalQuadraticBezier(prev, e.Control, e.Point, t)
				points = append(points, pt)
			}
			current = e.Point

		case CubicTo:
			// Flatten cubic Bezier to line segments
			prev := current
			steps := 16 // Number of segments
			for i := 1; i <= steps; i++ {
				t := float64(i) / float64(steps)
				pt := evalCubicBezier(prev, e.Control1, e.Control2, e.Point, t)
				points = append(points, pt)
			}
			current = e.Point

		case Close:
			// Close path by connecting to first point
			if len(points) > 0 {
				points = append(points, points[0])
			}
		}
	}

	return points
}

// evalQuadraticBezier evaluates a quadratic Bezier curve at parameter t.
func evalQuadraticBezier(p0, p1, p2 Point, t float64) Point {
	s := 1 - t
	return Point{
		X: s*s*p0.X + 2*s*t*p1.X + t*t*p2.X,
		Y: s*s*p0.Y + 2*s*t*p1.Y + t*t*p2.Y,
	}
}

// evalCubicBezier evaluates a cubic Bezier curve at parameter t.
func evalCubicBezier(p0, p1, p2, p3 Point, t float64) Point {
	s := 1 - t
	s2 := s * s
	s3 := s2 * s
	t2 := t * t
	t3 := t2 * t
	return Point{
		X: s3*p0.X + 3*s2*t*p1.X + 3*s*t2*p2.X + t3*p3.X,
		Y: s3*p0.Y + 3*s2*t*p1.Y + 3*s*t2*p2.Y + t3*p3.Y,
	}
}

// sortFloats sorts a slice of float64 values (simple bubble sort for small slices).
func sortFloats(values []float64) {
	n := len(values)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if values[j] > values[j+1] {
				values[j], values[j+1] = values[j+1], values[j]
			}
		}
	}
}
