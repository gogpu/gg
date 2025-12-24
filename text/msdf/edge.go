package msdf

import (
	"math"
)

// EdgeType classifies edge segments by their geometric type.
type EdgeType int

const (
	// EdgeLinear is a straight line segment between two points.
	EdgeLinear EdgeType = iota

	// EdgeQuadratic is a quadratic Bezier curve (one control point).
	EdgeQuadratic

	// EdgeCubic is a cubic Bezier curve (two control points).
	EdgeCubic
)

// String returns a string representation of the edge type.
func (t EdgeType) String() string {
	switch t {
	case EdgeLinear:
		return "Linear"
	case EdgeQuadratic:
		return "Quadratic"
	case EdgeCubic:
		return "Cubic"
	default:
		return "Unknown"
	}
}

// EdgeColor determines which RGB channels an edge contributes to.
// Different colors at corners preserve sharpness in MSDF.
type EdgeColor uint8

const (
	// ColorBlack means the edge contributes to no channels.
	ColorBlack EdgeColor = 0

	// ColorRed means the edge contributes to the red channel.
	ColorRed EdgeColor = 1 << iota

	// ColorGreen means the edge contributes to the green channel.
	ColorGreen

	// ColorBlue means the edge contributes to the blue channel.
	ColorBlue

	// ColorYellow combines red and green channels.
	ColorYellow = ColorRed | ColorGreen

	// ColorCyan combines green and blue channels.
	ColorCyan = ColorGreen | ColorBlue

	// ColorMagenta combines red and blue channels.
	ColorMagenta = ColorRed | ColorBlue

	// ColorWhite means the edge contributes to all channels.
	ColorWhite = ColorRed | ColorGreen | ColorBlue
)

// String returns a string representation of the edge color.
func (c EdgeColor) String() string {
	switch c {
	case ColorBlack:
		return "Black"
	case ColorRed:
		return "Red"
	case ColorGreen:
		return "Green"
	case ColorBlue:
		return "Blue"
	case ColorYellow:
		return "Yellow"
	case ColorCyan:
		return "Cyan"
	case ColorMagenta:
		return "Magenta"
	case ColorWhite:
		return "White"
	default:
		return "Unknown"
	}
}

// HasRed returns true if the color includes the red channel.
func (c EdgeColor) HasRed() bool { return c&ColorRed != 0 }

// HasGreen returns true if the color includes the green channel.
func (c EdgeColor) HasGreen() bool { return c&ColorGreen != 0 }

// HasBlue returns true if the color includes the blue channel.
func (c EdgeColor) HasBlue() bool { return c&ColorBlue != 0 }

// Edge represents a single edge segment for distance calculation.
// An edge can be linear, quadratic, or cubic Bezier.
type Edge struct {
	// Type is the geometric type of this edge.
	Type EdgeType

	// Points contains the control and end points for this edge.
	// Linear: P0 (start), P1 (end)
	// Quadratic: P0 (start), P1 (control), P2 (end)
	// Cubic: P0 (start), P1 (control1), P2 (control2), P3 (end)
	Points [4]Point

	// Color determines which channels this edge affects.
	Color EdgeColor
}

// NewLinearEdge creates a new linear edge from start to end.
func NewLinearEdge(start, end Point) Edge {
	return Edge{
		Type:   EdgeLinear,
		Points: [4]Point{start, end, {}, {}},
		Color:  ColorWhite,
	}
}

// NewQuadraticEdge creates a new quadratic Bezier edge.
func NewQuadraticEdge(start, control, end Point) Edge {
	return Edge{
		Type:   EdgeQuadratic,
		Points: [4]Point{start, control, end, {}},
		Color:  ColorWhite,
	}
}

// NewCubicEdge creates a new cubic Bezier edge.
func NewCubicEdge(start, control1, control2, end Point) Edge {
	return Edge{
		Type:   EdgeCubic,
		Points: [4]Point{start, control1, control2, end},
		Color:  ColorWhite,
	}
}

// StartPoint returns the starting point of the edge.
func (e *Edge) StartPoint() Point {
	return e.Points[0]
}

// EndPoint returns the ending point of the edge.
func (e *Edge) EndPoint() Point {
	switch e.Type {
	case EdgeLinear:
		return e.Points[1]
	case EdgeQuadratic:
		return e.Points[2]
	case EdgeCubic:
		return e.Points[3]
	default:
		return e.Points[0]
	}
}

// PointAt evaluates the edge at parameter t in [0, 1].
func (e *Edge) PointAt(t float64) Point {
	switch e.Type {
	case EdgeLinear:
		return e.Points[0].Lerp(e.Points[1], t)
	case EdgeQuadratic:
		return evaluateQuadratic(e.Points[0], e.Points[1], e.Points[2], t)
	case EdgeCubic:
		return evaluateCubic(e.Points[0], e.Points[1], e.Points[2], e.Points[3], t)
	default:
		return e.Points[0]
	}
}

// DirectionAt returns the tangent direction at parameter t.
func (e *Edge) DirectionAt(t float64) Point {
	switch e.Type {
	case EdgeLinear:
		return e.Points[1].Sub(e.Points[0])
	case EdgeQuadratic:
		return quadraticDerivative(e.Points[0], e.Points[1], e.Points[2], t)
	case EdgeCubic:
		return cubicDerivative(e.Points[0], e.Points[1], e.Points[2], e.Points[3], t)
	default:
		return Point{1, 0}
	}
}

// SignedDistance calculates the signed distance from point p to this edge.
// Returns the closest signed distance and the parameter t at that point.
func (e *Edge) SignedDistance(p Point) SignedDistance {
	switch e.Type {
	case EdgeLinear:
		return linearSignedDistance(e.Points[0], e.Points[1], p)
	case EdgeQuadratic:
		return quadraticSignedDistance(e.Points[0], e.Points[1], e.Points[2], p)
	case EdgeCubic:
		return cubicSignedDistance(e.Points[0], e.Points[1], e.Points[2], e.Points[3], p)
	default:
		return Infinite()
	}
}

// Bounds returns the bounding box of the edge.
func (e *Edge) Bounds() Rect {
	switch e.Type {
	case EdgeLinear:
		return linearBounds(e.Points[0], e.Points[1])
	case EdgeQuadratic:
		return quadraticBounds(e.Points[0], e.Points[1], e.Points[2])
	case EdgeCubic:
		return cubicBounds(e.Points[0], e.Points[1], e.Points[2], e.Points[3])
	default:
		return Rect{}
	}
}

// Clone creates a deep copy of the edge.
func (e *Edge) Clone() Edge {
	return Edge{
		Type:   e.Type,
		Points: e.Points,
		Color:  e.Color,
	}
}

// evaluateQuadratic evaluates a quadratic Bezier curve at parameter t.
func evaluateQuadratic(p0, p1, p2 Point, t float64) Point {
	u := 1 - t
	// B(t) = (1-t)^2*P0 + 2*(1-t)*t*P1 + t^2*P2
	return Point{
		u*u*p0.X + 2*u*t*p1.X + t*t*p2.X,
		u*u*p0.Y + 2*u*t*p1.Y + t*t*p2.Y,
	}
}

// evaluateCubic evaluates a cubic Bezier curve at parameter t.
func evaluateCubic(p0, p1, p2, p3 Point, t float64) Point {
	u := 1 - t
	u2 := u * u
	t2 := t * t
	// B(t) = (1-t)^3*P0 + 3*(1-t)^2*t*P1 + 3*(1-t)*t^2*P2 + t^3*P3
	return Point{
		u*u2*p0.X + 3*u2*t*p1.X + 3*u*t2*p2.X + t*t2*p3.X,
		u*u2*p0.Y + 3*u2*t*p1.Y + 3*u*t2*p2.Y + t*t2*p3.Y,
	}
}

// quadraticDerivative returns the derivative of a quadratic Bezier at t.
func quadraticDerivative(p0, p1, p2 Point, t float64) Point {
	u := 1 - t
	// B'(t) = 2*(1-t)*(P1-P0) + 2*t*(P2-P1)
	return Point{
		2*u*(p1.X-p0.X) + 2*t*(p2.X-p1.X),
		2*u*(p1.Y-p0.Y) + 2*t*(p2.Y-p1.Y),
	}
}

// cubicDerivative returns the derivative of a cubic Bezier at t.
func cubicDerivative(p0, p1, p2, p3 Point, t float64) Point {
	u := 1 - t
	// B'(t) = 3*(1-t)^2*(P1-P0) + 6*(1-t)*t*(P2-P1) + 3*t^2*(P3-P2)
	return Point{
		3*u*u*(p1.X-p0.X) + 6*u*t*(p2.X-p1.X) + 3*t*t*(p3.X-p2.X),
		3*u*u*(p1.Y-p0.Y) + 6*u*t*(p2.Y-p1.Y) + 3*t*t*(p3.Y-p2.Y),
	}
}

// linearSignedDistance calculates signed distance from point p to line segment a-b.
func linearSignedDistance(a, b, p Point) SignedDistance {
	ab := b.Sub(a)
	ap := p.Sub(a)

	// Project p onto line ab
	abLenSq := ab.LengthSquared()
	if abLenSq == 0 {
		// Degenerate line - both points are the same
		dist := ap.Length()
		return NewSignedDistance(dist, 0)
	}

	t := ap.Dot(ab) / abLenSq

	// Clamp t to [0, 1] for segment distance
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	// Find closest point on segment
	closest := a.Add(ab.Mul(t))
	diff := p.Sub(closest)
	dist := diff.Length()

	// Determine sign using cross product
	cross := ab.Cross(ap)
	if cross < 0 {
		dist = -dist
	}

	// Calculate orthogonality for tie-breaking
	var dot float64
	switch t {
	case 0, 1:
		dot = math.Abs(ab.Normalized().Dot(diff.Normalized()))
	default:
		dot = 0
	}

	return NewSignedDistance(dist, dot)
}

// quadraticSignedDistance calculates signed distance from point p to quadratic Bezier.
func quadraticSignedDistance(p0, p1, p2, p Point) SignedDistance {
	// Convert to polynomial form and find closest point
	// This uses the analytical solution by finding roots of the distance derivative.

	// Transform so p is at origin
	qa := p0.Sub(p)
	qb := p1.Sub(p)
	qc := p2.Sub(p)

	// Coefficients of the Bezier curve: B(t) = a*t^2 + b*t + c
	a := qa.Sub(qb.Mul(2)).Add(qc)
	b := qb.Sub(qa).Mul(2)
	c := qa

	// The distance squared is a quartic polynomial in t.
	// Its derivative is cubic. Solve for roots.
	// d(dist^2)/dt = 0 leads to a cubic equation.

	// Coefficients of the cubic
	c3 := 2 * a.Dot(a)
	c2 := 3 * a.Dot(b)
	c1 := 2*a.Dot(c) + b.Dot(b)
	c0 := b.Dot(c)

	// Find roots of c3*t^3 + c2*t^2 + c1*t + c0 = 0
	roots := solveCubic(c3, c2, c1, c0)

	// Check endpoints and all real roots in [0, 1]
	minDist := Infinite()

	checkPoint := func(t float64) {
		if t < 0 || t > 1 {
			return
		}
		pt := evaluateQuadratic(p0, p1, p2, t)
		diff := p.Sub(pt)
		dist := diff.Length()

		// Determine sign
		tangent := quadraticDerivative(p0, p1, p2, t)
		cross := tangent.Cross(diff)
		if cross < 0 {
			dist = -dist
		}

		// Calculate pseudo-distance
		var dot float64
		if t == 0 || t == 1 {
			dot = math.Abs(tangent.Normalized().Dot(diff.Normalized()))
		}

		sd := NewSignedDistance(dist, dot)
		if sd.IsCloserThan(minDist) {
			minDist = sd
		}
	}

	// Check endpoints
	checkPoint(0)
	checkPoint(1)

	// Check roots
	for _, t := range roots {
		checkPoint(t)
	}

	return minDist
}

// cubicSignedDistance calculates signed distance from point p to cubic Bezier.
func cubicSignedDistance(p0, p1, p2, p3, p Point) SignedDistance {
	// For cubic Bezier, the distance derivative is a quintic polynomial.
	// We use Newton's method with multiple starting points for robustness.

	minDist := Infinite()

	checkPoint := func(t float64) {
		if t < 0 || t > 1 {
			return
		}
		pt := evaluateCubic(p0, p1, p2, p3, t)
		diff := p.Sub(pt)
		dist := diff.Length()

		// Determine sign
		tangent := cubicDerivative(p0, p1, p2, p3, t)
		cross := tangent.Cross(diff)
		if cross < 0 {
			dist = -dist
		}

		// Calculate pseudo-distance
		var dot float64
		if t == 0 || t == 1 {
			dot = math.Abs(tangent.Normalized().Dot(diff.Normalized()))
		}

		sd := NewSignedDistance(dist, dot)
		if sd.IsCloserThan(minDist) {
			minDist = sd
		}
	}

	// Check endpoints
	checkPoint(0)
	checkPoint(1)

	// Use subdivision approach with Newton refinement
	// This is more robust than pure Newton's method
	const numSamples = 8
	for i := 0; i <= numSamples; i++ {
		t := float64(i) / float64(numSamples)
		// Refine with Newton's method
		t = newtonRefineCubic(p0, p1, p2, p3, p, t)
		checkPoint(t)
	}

	return minDist
}

// newtonRefineCubic refines a parameter t using Newton's method.
func newtonRefineCubic(p0, p1, p2, p3, p Point, t float64) float64 {
	const maxIter = 8
	const epsilon = 1e-10

	for i := 0; i < maxIter; i++ {
		pt := evaluateCubic(p0, p1, p2, p3, t)
		diff := pt.Sub(p)

		d1 := cubicDerivative(p0, p1, p2, p3, t)
		d2 := cubicSecondDerivative(p0, p1, p2, p3, t)

		// f(t) = diff.Dot(d1) (derivative of distance squared)
		// f'(t) = d1.Dot(d1) + diff.Dot(d2)
		f := diff.Dot(d1)
		fp := d1.Dot(d1) + diff.Dot(d2)

		if math.Abs(fp) < epsilon {
			break
		}

		dt := -f / fp
		if math.Abs(dt) < epsilon {
			break
		}

		t += dt

		// Clamp to valid range
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
	}

	return t
}

// cubicSecondDerivative returns the second derivative of a cubic Bezier at t.
func cubicSecondDerivative(p0, p1, p2, p3 Point, t float64) Point {
	// B''(t) = 6*(1-t)*(P2-2*P1+P0) + 6*t*(P3-2*P2+P1)
	a := p2.Sub(p1.Mul(2)).Add(p0)
	b := p3.Sub(p2.Mul(2)).Add(p1)
	u := 1 - t
	return a.Mul(6 * u).Add(b.Mul(6 * t))
}

// solveCubic solves a*x^3 + b*x^2 + c*x + d = 0.
// Returns real roots in [0, 1].
func solveCubic(a, b, c, d float64) []float64 {
	// Handle degenerate case: quadratic
	if math.Abs(a) < 1e-14 {
		return solveQuadratic(b, c, d)
	}

	return solveCubicCardano(a, b, c, d)
}

// solveCubicCardano uses Cardano's method to solve a cubic equation.
func solveCubicCardano(a, b, c, d float64) []float64 {
	var roots []float64

	// Normalize coefficients
	b /= a
	c /= a
	d /= a

	// Cardano's method: depress the cubic
	p := c - b*b/3
	q := d - b*c/3 + 2*b*b*b/27
	discriminant := q*q/4 + p*p*p/27

	switch {
	case discriminant > 1e-14:
		// One real root
		roots = solveCubicOneRoot(q, discriminant, b)
	case discriminant < -1e-14:
		// Three real roots
		roots = solveCubicThreeRoots(p, q, b)
	default:
		// Triple root or repeated roots
		roots = solveCubicRepeatedRoots(q, b)
	}

	return roots
}

// solveCubicOneRoot handles the case with one real root.
func solveCubicOneRoot(q, discriminant, b float64) []float64 {
	var roots []float64
	sqrtD := math.Sqrt(discriminant)
	u := cbrt(-q/2 + sqrtD)
	v := cbrt(-q/2 - sqrtD)
	root := u + v - b/3
	if root >= 0 && root <= 1 {
		roots = append(roots, root)
	}
	return roots
}

// solveCubicThreeRoots handles the case with three real roots.
func solveCubicThreeRoots(p, q, b float64) []float64 {
	var roots []float64
	r := math.Sqrt(-p * p * p / 27)
	phi := math.Acos(-q / (2 * r))
	cubeRootR := math.Pow(r, 1.0/3.0)

	for k := 0; k < 3; k++ {
		root := 2*cubeRootR*math.Cos((phi+float64(2*k)*math.Pi)/3) - b/3
		if root >= 0 && root <= 1 {
			roots = append(roots, root)
		}
	}
	return roots
}

// solveCubicRepeatedRoots handles the case with repeated roots.
func solveCubicRepeatedRoots(q, b float64) []float64 {
	var roots []float64
	u := cbrt(-q / 2)
	root1 := 2*u - b/3
	root2 := -u - b/3

	if root1 >= 0 && root1 <= 1 {
		roots = append(roots, root1)
	}
	if root2 >= 0 && root2 <= 1 && math.Abs(root1-root2) > 1e-10 {
		roots = append(roots, root2)
	}
	return roots
}

// solveQuadratic solves a*x^2 + b*x + c = 0.
// Returns real roots in [0, 1].
func solveQuadratic(a, b, c float64) []float64 {
	// Handle degenerate case: linear
	if math.Abs(a) < 1e-14 {
		return solveLinear(b, c)
	}

	return solveQuadraticFull(a, b, c)
}

// solveLinear solves b*x + c = 0.
func solveLinear(b, c float64) []float64 {
	var roots []float64
	if math.Abs(b) >= 1e-14 {
		root := -c / b
		if root >= 0 && root <= 1 {
			roots = append(roots, root)
		}
	}
	return roots
}

// solveQuadraticFull solves a non-degenerate quadratic equation.
func solveQuadraticFull(a, b, c float64) []float64 {
	var roots []float64
	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		return roots
	}

	sqrtD := math.Sqrt(discriminant)
	root1 := (-b + sqrtD) / (2 * a)
	root2 := (-b - sqrtD) / (2 * a)

	if root1 >= 0 && root1 <= 1 {
		roots = append(roots, root1)
	}
	if root2 >= 0 && root2 <= 1 && math.Abs(root1-root2) > 1e-10 {
		roots = append(roots, root2)
	}
	return roots
}

// cbrt returns the cube root of x (handles negative values).
func cbrt(x float64) float64 {
	if x < 0 {
		return -math.Pow(-x, 1.0/3.0)
	}
	return math.Pow(x, 1.0/3.0)
}

// linearBounds returns the bounding box of a line segment.
func linearBounds(a, b Point) Rect {
	return Rect{
		MinX: min(a.X, b.X),
		MinY: min(a.Y, b.Y),
		MaxX: max(a.X, b.X),
		MaxY: max(a.Y, b.Y),
	}
}

// quadraticBounds returns the bounding box of a quadratic Bezier.
func quadraticBounds(p0, p1, p2 Point) Rect {
	bounds := linearBounds(p0, p2)

	// Find extrema in x
	// B'(t) = 2*(1-t)*(p1-p0) + 2*t*(p2-p1) = 0
	// t = (p0-p1)/(p0-2*p1+p2)
	dx := p0.X - 2*p1.X + p2.X
	if math.Abs(dx) > 1e-10 {
		t := (p0.X - p1.X) / dx
		if t > 0 && t < 1 {
			x := evaluateQuadratic(p0, p1, p2, t).X
			bounds.MinX = min(bounds.MinX, x)
			bounds.MaxX = max(bounds.MaxX, x)
		}
	}

	// Find extrema in y
	dy := p0.Y - 2*p1.Y + p2.Y
	if math.Abs(dy) > 1e-10 {
		t := (p0.Y - p1.Y) / dy
		if t > 0 && t < 1 {
			y := evaluateQuadratic(p0, p1, p2, t).Y
			bounds.MinY = min(bounds.MinY, y)
			bounds.MaxY = max(bounds.MaxY, y)
		}
	}

	return bounds
}

// cubicBounds returns the bounding box of a cubic Bezier.
func cubicBounds(p0, p1, p2, p3 Point) Rect {
	bounds := linearBounds(p0, p3)

	// Find extrema using derivative roots
	// B'(t) = 3*(1-t)^2*(p1-p0) + 6*(1-t)*t*(p2-p1) + 3*t^2*(p3-p2)
	// This is a quadratic in t.

	// X extrema
	ax := -p0.X + 3*p1.X - 3*p2.X + p3.X
	bx := 2*p0.X - 4*p1.X + 2*p2.X
	cx := -p0.X + p1.X

	for _, t := range solveQuadratic(ax, bx, cx) {
		if t > 0 && t < 1 {
			x := evaluateCubic(p0, p1, p2, p3, t).X
			bounds.MinX = min(bounds.MinX, x)
			bounds.MaxX = max(bounds.MaxX, x)
		}
	}

	// Y extrema
	ay := -p0.Y + 3*p1.Y - 3*p2.Y + p3.Y
	by := 2*p0.Y - 4*p1.Y + 2*p2.Y
	cy := -p0.Y + p1.Y

	for _, t := range solveQuadratic(ay, by, cy) {
		if t > 0 && t < 1 {
			y := evaluateCubic(p0, p1, p2, p3, t).Y
			bounds.MinY = min(bounds.MinY, y)
			bounds.MaxY = max(bounds.MaxY, y)
		}
	}

	return bounds
}
