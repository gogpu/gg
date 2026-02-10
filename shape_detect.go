package gg

import "math"

// ShapeKind identifies detected shapes for GPU SDF acceleration.
type ShapeKind int

const (
	// ShapeUnknown indicates the path is too complex for shape detection.
	ShapeUnknown ShapeKind = iota

	// ShapeCircle indicates a circular path.
	ShapeCircle

	// ShapeEllipse indicates an elliptical path.
	ShapeEllipse

	// ShapeRect indicates an axis-aligned rectangular path.
	ShapeRect

	// ShapeRRect indicates a rounded rectangle path.
	ShapeRRect
)

// DetectedShape holds parameters of a recognized geometric shape.
// The Kind field indicates which parameters are meaningful.
type DetectedShape struct {
	Kind         ShapeKind
	CenterX      float64 // Center X coordinate.
	CenterY      float64 // Center Y coordinate.
	RadiusX      float64 // X radius. For circle: RadiusX == RadiusY.
	RadiusY      float64 // Y radius. For circle: RadiusX == RadiusY.
	Width        float64 // Total width for rect/rrect.
	Height       float64 // Total height for rect/rrect.
	CornerRadius float64 // Corner radius for rrect only.
}

// kappa is the cubic Bezier control point distance for circle approximation.
// Equal to 4/3 * (sqrt(2) - 1).
const kappa = 0.5522847498307936

// shapeDetectTolerance is the maximum allowed error for shape detection.
const shapeDetectTolerance = 1e-3

// DetectShape analyzes a Path and returns the identified shape if recognized.
// Returns a DetectedShape with Kind == ShapeUnknown if the path cannot be
// identified as a simple geometric primitive.
func DetectShape(path *Path) DetectedShape {
	if path == nil {
		return DetectedShape{Kind: ShapeUnknown}
	}

	elems := path.Elements()
	if len(elems) == 0 {
		return DetectedShape{Kind: ShapeUnknown}
	}

	// Try circle/ellipse: MoveTo + 4xCubicTo + Close = 6 elements
	if len(elems) == 6 {
		if shape, ok := detectCircleOrEllipse(elems); ok {
			return shape
		}
	}

	// Try rect: MoveTo + 3xLineTo + Close = 5 elements
	if len(elems) == 5 {
		if shape, ok := detectRect(elems); ok {
			return shape
		}
	}

	// Try rrect: MoveTo + (CubicTo + LineTo)*4 + Close = 10 elements
	// Or variation with arcs: more elements
	if len(elems) >= 9 {
		if shape, ok := detectRRect(elems); ok {
			return shape
		}
	}

	return DetectedShape{Kind: ShapeUnknown}
}

// detectCircleOrEllipse checks if 6 elements form a circle or ellipse.
// Expected pattern: MoveTo, CubicTo, CubicTo, CubicTo, CubicTo, Close.
func detectCircleOrEllipse(elems []PathElement) (DetectedShape, bool) {
	move, ok := elems[0].(MoveTo)
	if !ok {
		return DetectedShape{}, false
	}

	cubics := make([]CubicTo, 0, 4)
	for i := 1; i <= 4; i++ {
		c, ok := elems[i].(CubicTo)
		if !ok {
			return DetectedShape{}, false
		}
		cubics = append(cubics, c)
	}

	if _, ok := elems[5].(Close); !ok {
		return DetectedShape{}, false
	}

	// The start point is move.Point.
	// For a circle centered at (cx, cy) with radius r:
	// Start = (cx+r, cy), i.e. rightmost point
	//
	// Quadrant 1 (right to bottom): end = (cx, cy+r)
	// Quadrant 2 (bottom to left):  end = (cx-r, cy)
	// Quadrant 3 (left to top):     end = (cx, cy-r)
	// Quadrant 4 (top to right):    end = (cx+r, cy) = start (closed)

	// Extract the 4 endpoints: start, and each cubic's endpoint.
	pts := [5]Point{
		move.Point,
		cubics[0].Point,
		cubics[1].Point,
		cubics[2].Point,
		cubics[3].Point,
	}

	// The path must close: endpoint of last cubic == start.
	if !pointsClose(pts[4], pts[0]) {
		return DetectedShape{}, false
	}

	// Calculate center from opposing points.
	// pts[0] and pts[2] are diametrically opposite (right and left).
	// pts[1] and pts[3] are diametrically opposite (bottom and top).
	cx := (pts[0].X + pts[2].X) / 2
	cy := (pts[0].Y + pts[2].Y) / 2

	// Also check second pair of opposing points gives same center.
	cx2 := (pts[1].X + pts[3].X) / 2
	cy2 := (pts[1].Y + pts[3].Y) / 2

	if math.Abs(cx-cx2) > shapeDetectTolerance || math.Abs(cy-cy2) > shapeDetectTolerance {
		return DetectedShape{}, false
	}

	// Compute radii.
	rx := math.Abs(pts[0].X - cx)
	ry := math.Abs(pts[1].Y - cy)

	if rx < shapeDetectTolerance || ry < shapeDetectTolerance {
		return DetectedShape{}, false
	}

	// Verify control points match the kappa-based circle/ellipse approximation.
	// For each quadrant, the control points should be at kappa * radius from the endpoints.
	if !verifyEllipseControlPoints(cubics, cx, cy, rx, ry) {
		return DetectedShape{}, false
	}

	if math.Abs(rx-ry) < shapeDetectTolerance {
		// Circle
		r := (rx + ry) / 2
		return DetectedShape{
			Kind:    ShapeCircle,
			CenterX: cx,
			CenterY: cy,
			RadiusX: r,
			RadiusY: r,
		}, true
	}

	// Ellipse
	return DetectedShape{
		Kind:    ShapeEllipse,
		CenterX: cx,
		CenterY: cy,
		RadiusX: rx,
		RadiusY: ry,
	}, true
}

// verifyEllipseControlPoints validates that cubic Bezier control points
// match the standard kappa-based ellipse approximation.
func verifyEllipseControlPoints(cubics []CubicTo, cx, cy, rx, ry float64) bool {
	kx := rx * kappa
	ky := ry * kappa

	// Quadrant 1: (cx+rx, cy) -> (cx, cy+ry)
	// CP1 = (cx+rx, cy+ky), CP2 = (cx+kx, cy+ry)
	if !checkCP(cubics[0].Control1, cx+rx, cy+ky) ||
		!checkCP(cubics[0].Control2, cx+kx, cy+ry) {
		return false
	}

	// Quadrant 2: (cx, cy+ry) -> (cx-rx, cy)
	// CP1 = (cx-kx, cy+ry), CP2 = (cx-rx, cy+ky)
	if !checkCP(cubics[1].Control1, cx-kx, cy+ry) ||
		!checkCP(cubics[1].Control2, cx-rx, cy+ky) {
		return false
	}

	// Quadrant 3: (cx-rx, cy) -> (cx, cy-ry)
	// CP1 = (cx-rx, cy-ky), CP2 = (cx-kx, cy-ry)
	if !checkCP(cubics[2].Control1, cx-rx, cy-ky) ||
		!checkCP(cubics[2].Control2, cx-kx, cy-ry) {
		return false
	}

	// Quadrant 4: (cx, cy-ry) -> (cx+rx, cy)
	// CP1 = (cx+kx, cy-ry), CP2 = (cx+rx, cy-ky)
	if !checkCP(cubics[3].Control1, cx+kx, cy-ry) ||
		!checkCP(cubics[3].Control2, cx+rx, cy-ky) {
		return false
	}

	return true
}

// checkCP verifies a control point is close to expected coordinates.
func checkCP(pt Point, ex, ey float64) bool {
	return math.Abs(pt.X-ex) < shapeDetectTolerance && math.Abs(pt.Y-ey) < shapeDetectTolerance
}

// detectRect checks if 5 elements form an axis-aligned rectangle.
// Expected pattern: MoveTo, LineTo, LineTo, LineTo, Close.
func detectRect(elems []PathElement) (DetectedShape, bool) {
	move, ok := elems[0].(MoveTo)
	if !ok {
		return DetectedShape{}, false
	}

	lines := make([]LineTo, 0, 3)
	for i := 1; i <= 3; i++ {
		l, ok := elems[i].(LineTo)
		if !ok {
			return DetectedShape{}, false
		}
		lines = append(lines, l)
	}

	if _, ok := elems[4].(Close); !ok {
		return DetectedShape{}, false
	}

	// Extract 4 corners.
	corners := [4]Point{
		move.Point,
		lines[0].Point,
		lines[1].Point,
		lines[2].Point,
	}

	// Verify axis-aligned: each consecutive pair of points must share
	// either X or Y coordinate.
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		dx := math.Abs(corners[i].X - corners[j].X)
		dy := math.Abs(corners[i].Y - corners[j].Y)
		if dx > shapeDetectTolerance && dy > shapeDetectTolerance {
			// Neither horizontal nor vertical.
			return DetectedShape{}, false
		}
	}

	// Find bounding box.
	minX, maxX := corners[0].X, corners[0].X
	minY, maxY := corners[0].Y, corners[0].Y
	for _, c := range corners[1:] {
		minX = math.Min(minX, c.X)
		maxX = math.Max(maxX, c.X)
		minY = math.Min(minY, c.Y)
		maxY = math.Max(maxY, c.Y)
	}

	w := maxX - minX
	h := maxY - minY

	if w < shapeDetectTolerance || h < shapeDetectTolerance {
		return DetectedShape{}, false
	}

	return DetectedShape{
		Kind:    ShapeRect,
		CenterX: (minX + maxX) / 2,
		CenterY: (minY + maxY) / 2,
		Width:   w,
		Height:  h,
	}, true
}

// detectRRect checks if elements form a rounded rectangle.
// The RoundedRectangle method on Path produces:
// MoveTo, LineTo, [arc cubics], LineTo, [arc cubics], LineTo, [arc cubics], LineTo, [arc cubics], Close.
// Each arc is a single CubicTo for a 90-degree arc.
func detectRRect(elems []PathElement) (DetectedShape, bool) {
	// Expect: MoveTo + (LineTo + CubicTo) * 4 + Close = 1 + 8 + 1 = 10
	// But path.RoundedRectangle uses Arc which may produce different structure.
	// The Arc method for exactly 90 degrees produces exactly 1 CubicTo per corner.
	//
	// Actual pattern from Path.RoundedRectangle:
	// MoveTo(x+r, y)
	// LineTo(x+w-r, y)        -- top edge
	// CubicTo(...)             -- top-right corner arc
	// LineTo(x+w, y+h-r)      -- right edge
	// CubicTo(...)             -- bottom-right corner arc
	// LineTo(x+r, y+h)        -- bottom edge
	// CubicTo(...)             -- bottom-left corner arc
	// LineTo(x, y+r)          -- left edge
	// CubicTo(...)             -- top-left corner arc
	// Close
	//
	// Total = 10 elements.

	if len(elems) != 10 {
		return DetectedShape{}, false
	}

	move, ok := elems[0].(MoveTo)
	if !ok {
		return DetectedShape{}, false
	}

	// Verify pattern: LineTo, CubicTo alternating, then Close.
	var linePoints [4]Point
	var cubicArcs [4]CubicTo

	for i := 0; i < 4; i++ {
		baseIdx := 1 + i*2
		lt, ok := elems[baseIdx].(LineTo)
		if !ok {
			return DetectedShape{}, false
		}
		linePoints[i] = lt.Point

		ct, ok := elems[baseIdx+1].(CubicTo)
		if !ok {
			return DetectedShape{}, false
		}
		cubicArcs[i] = ct
	}

	if _, ok := elems[9].(Close); !ok {
		return DetectedShape{}, false
	}

	// Extract geometry from the known structure.
	// move.Point = (x+r, y) -- start of top edge
	// linePoints[0] = (x+w-r, y) -- end of top edge
	// cubicArcs[0].Point = endpoint of top-right corner
	// linePoints[1] = end of right edge
	// etc.

	// Top edge: from move.Point to linePoints[0] -- both Y should be equal (top).
	topY := move.Point.Y
	if math.Abs(linePoints[0].Y-topY) > shapeDetectTolerance {
		return DetectedShape{}, false
	}

	// After top-right arc, we're at the top of the right edge.
	// Right edge: cubicArcs[0].Point to linePoints[1] -- both X should be equal (right).
	rightX := cubicArcs[0].Point.X
	if math.Abs(linePoints[1].X-rightX) > shapeDetectTolerance {
		return DetectedShape{}, false
	}

	// Bottom edge: cubicArcs[1].Point to linePoints[2] -- both Y should be equal (bottom).
	bottomY := cubicArcs[1].Point.Y
	if math.Abs(linePoints[2].Y-bottomY) > shapeDetectTolerance {
		return DetectedShape{}, false
	}

	// Left edge: cubicArcs[2].Point to linePoints[3] -- both X should be equal (left).
	leftX := cubicArcs[2].Point.X
	if math.Abs(linePoints[3].X-leftX) > shapeDetectTolerance {
		return DetectedShape{}, false
	}

	// Compute dimensions.
	w := rightX - leftX
	h := bottomY - topY
	if w < shapeDetectTolerance || h < shapeDetectTolerance {
		return DetectedShape{}, false
	}

	// Compute corner radius from the top edge.
	// move.Point.X = leftX + r, linePoints[0].X = rightX - r
	r1 := move.Point.X - leftX
	r2 := rightX - linePoints[0].X
	if r1 < 0 || r2 < 0 {
		return DetectedShape{}, false
	}
	if math.Abs(r1-r2) > shapeDetectTolerance {
		return DetectedShape{}, false
	}
	cornerR := (r1 + r2) / 2

	// Verify the corner radius from the right edge as well.
	r3 := cubicArcs[0].Point.Y - topY
	r4 := bottomY - linePoints[1].Y
	if math.Abs(r3-cornerR) > shapeDetectTolerance || math.Abs(r4-cornerR) > shapeDetectTolerance {
		return DetectedShape{}, false
	}

	return DetectedShape{
		Kind:         ShapeRRect,
		CenterX:      (leftX + rightX) / 2,
		CenterY:      (topY + bottomY) / 2,
		Width:        w,
		Height:       h,
		CornerRadius: cornerR,
	}, true
}

// pointsClose checks if two points are within tolerance.
func pointsClose(a, b Point) bool {
	return math.Abs(a.X-b.X) < shapeDetectTolerance && math.Abs(a.Y-b.Y) < shapeDetectTolerance
}
