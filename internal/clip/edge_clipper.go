package clip

import "math"

// EdgeClipper clips path edges against a rectangular clip region.
// Inspired by tiny-skia edge_clipper.rs and Skia's SkEdgeClipper.
type EdgeClipper struct {
	clip Rect
}

// NewEdgeClipper creates an edge clipper for the given bounds.
func NewEdgeClipper(clip Rect) *EdgeClipper {
	return &EdgeClipper{clip: clip}
}

// Clip returns the clip rectangle.
func (ec *EdgeClipper) Clip() Rect {
	return ec.clip
}

// Outcode constants for Cohen-Sutherland algorithm.
const (
	outcodeInside = 0
	outcodeLeft   = 1
	outcodeRight  = 2
	outcodeBottom = 4
	outcodeTop    = 8
)

// outcode computes the Cohen-Sutherland outcode for a point.
func (ec *EdgeClipper) outcode(p Point) int {
	code := outcodeInside

	if p.X < ec.clip.X {
		code |= outcodeLeft
	} else if p.X > ec.clip.Right() {
		code |= outcodeRight
	}

	if p.Y < ec.clip.Y {
		code |= outcodeTop
	} else if p.Y > ec.clip.Bottom() {
		code |= outcodeBottom
	}

	return code
}

// allInside returns true if all points are inside the clip region.
func (ec *EdgeClipper) allInside(points ...Point) bool {
	for _, p := range points {
		if ec.outcode(p) != 0 {
			return false
		}
	}
	return true
}

// boundsIntersect returns true if the given bounds intersect the clip region.
func (ec *EdgeClipper) boundsIntersect(b Rect) bool {
	return ec.clip.Intersects(b)
}

// ClipLine clips a line segment to the clip rectangle using Cohen-Sutherland algorithm.
// Returns nil if the line is entirely outside, or a slice with the clipped segment.
func (ec *EdgeClipper) ClipLine(p0, p1 Point) []LineSeg {
	code0 := ec.outcode(p0)
	code1 := ec.outcode(p1)

	for {
		if (code0 | code1) == 0 {
			// Both inside - trivially accept
			return []LineSeg{{P0: p0, P1: p1}}
		}
		if (code0 & code1) != 0 {
			// Both outside same region - trivially reject
			return nil
		}

		// One point outside, clip it
		codeOut := code0
		if codeOut == 0 {
			codeOut = code1
		}

		var p Point

		// Find intersection with clip boundary
		switch {
		case (codeOut & outcodeTop) != 0:
			// Point is above clip rect
			t := (ec.clip.Y - p0.Y) / (p1.Y - p0.Y)
			p.X = p0.X + t*(p1.X-p0.X)
			p.Y = ec.clip.Y
		case (codeOut & outcodeBottom) != 0:
			// Point is below clip rect
			t := (ec.clip.Bottom() - p0.Y) / (p1.Y - p0.Y)
			p.X = p0.X + t*(p1.X-p0.X)
			p.Y = ec.clip.Bottom()
		case (codeOut & outcodeRight) != 0:
			// Point is to the right
			t := (ec.clip.Right() - p0.X) / (p1.X - p0.X)
			p.Y = p0.Y + t*(p1.Y-p0.Y)
			p.X = ec.clip.Right()
		case (codeOut & outcodeLeft) != 0:
			// Point is to the left
			t := (ec.clip.X - p0.X) / (p1.X - p0.X)
			p.Y = p0.Y + t*(p1.Y-p0.Y)
			p.X = ec.clip.X
		}

		// Update the point that was outside
		if codeOut == code0 {
			p0 = p
			code0 = ec.outcode(p0)
		} else {
			p1 = p
			code1 = ec.outcode(p1)
		}
	}
}

// ClipQuadratic clips a quadratic Bezier curve to the clip region.
// The curve is first chopped at extrema to ensure monotonicity, then clipped.
func (ec *EdgeClipper) ClipQuadratic(p0, p1, p2 Point) []QuadSeg {
	// Fast path: entirely inside
	if ec.allInside(p0, p1, p2) {
		return []QuadSeg{{P0: p0, P1: p1, P2: p2}}
	}

	// Check if entirely outside
	bounds := QuadSeg{P0: p0, P1: p1, P2: p2}.Bounds()
	if !ec.boundsIntersect(bounds) {
		return nil
	}

	// Chop at Y extrema to ensure Y-monotonicity
	var result []QuadSeg
	ec.chopQuadAtYExtrema(p0, p1, p2, func(q0, q1, q2 Point) {
		// Then chop at X extrema for X-monotonicity
		ec.chopQuadAtXExtrema(q0, q1, q2, func(r0, r1, r2 Point) {
			// Clip the monotonic quad
			clipped := ec.clipMonoQuad(r0, r1, r2)
			result = append(result, clipped...)
		})
	})

	return result
}

// ClipCubic clips a cubic Bezier curve to the clip region.
func (ec *EdgeClipper) ClipCubic(p0, p1, p2, p3 Point) []CubicSeg {
	// Fast path: entirely inside
	if ec.allInside(p0, p1, p2, p3) {
		return []CubicSeg{{P0: p0, P1: p1, P2: p2, P3: p3}}
	}

	// Check if entirely outside
	bounds := CubicSeg{P0: p0, P1: p1, P2: p2, P3: p3}.Bounds()
	if !ec.boundsIntersect(bounds) {
		return nil
	}

	// Chop at Y extrema
	var result []CubicSeg
	ec.chopCubicAtYExtrema(p0, p1, p2, p3, func(c0, c1, c2, c3 Point) {
		// Chop at X extrema
		ec.chopCubicAtXExtrema(c0, c1, c2, c3, func(d0, d1, d2, d3 Point) {
			// Clip monotonic cubic
			clipped := ec.clipMonoCubic(d0, d1, d2, d3)
			result = append(result, clipped...)
		})
	})

	return result
}

// isNotMonotonic checks if values a, b, c are not monotonic (have an extremum).
func isNotMonotonic(a, b, c float64) bool {
	ab := a - b
	bc := b - c
	if ab < 0 {
		bc = -bc
	}
	return ab == 0 || bc < 0
}

// chopQuadAtYExtrema chops a quadratic at its Y extremum for Y-monotonicity.
func (ec *EdgeClipper) chopQuadAtYExtrema(p0, p1, p2 Point, emit func(Point, Point, Point)) {
	a := p0.Y
	b := p1.Y
	c := p2.Y

	if !isNotMonotonic(a, b, c) {
		emit(p0, p1, p2)
		return
	}

	// Find t where derivative = 0
	// dy/dt = 2(1-t)(p1-p0) + 2t(p2-p1) = 0
	// Solving: t = (p0-p1) / (p0 - 2*p1 + p2)
	denom := a - 2*b + c
	if math.Abs(denom) < 1e-10 {
		emit(p0, p1, p2)
		return
	}

	t := (a - b) / denom
	if t <= 0 || t >= 1 {
		emit(p0, p1, p2)
		return
	}

	// Split at t using de Casteljau
	q0, q1, q2, q3, q4 := chopQuadAt(p0, p1, p2, t)
	emit(q0, q1, q2)
	emit(q2, q3, q4)
}

// chopQuadAtXExtrema chops a quadratic at its X extremum for X-monotonicity.
func (ec *EdgeClipper) chopQuadAtXExtrema(p0, p1, p2 Point, emit func(Point, Point, Point)) {
	a := p0.X
	b := p1.X
	c := p2.X

	if !isNotMonotonic(a, b, c) {
		emit(p0, p1, p2)
		return
	}

	denom := a - 2*b + c
	if math.Abs(denom) < 1e-10 {
		emit(p0, p1, p2)
		return
	}

	t := (a - b) / denom
	if t <= 0 || t >= 1 {
		emit(p0, p1, p2)
		return
	}

	q0, q1, q2, q3, q4 := chopQuadAt(p0, p1, p2, t)
	emit(q0, q1, q2)
	emit(q2, q3, q4)
}

// chopQuadAt subdivides a quadratic Bezier at parameter t using de Casteljau.
// Returns the two resulting quadratics as (q0, q1, q2) and (q2, q3, q4).
func chopQuadAt(p0, p1, p2 Point, t float64) (Point, Point, Point, Point, Point) {
	// de Casteljau subdivision
	q0 := p0
	q1 := p0.Lerp(p1, t)
	tmp := p1.Lerp(p2, t)
	q2 := q1.Lerp(tmp, t)
	q3 := tmp
	q4 := p2
	return q0, q1, q2, q3, q4
}

// clipMonoQuad clips a monotonic quadratic Bezier to the clip bounds.
func (ec *EdgeClipper) clipMonoQuad(p0, p1, p2 Point) []QuadSeg {
	// If all points inside, return as-is
	if ec.allInside(p0, p1, p2) {
		return []QuadSeg{{P0: p0, P1: p1, P2: p2}}
	}

	// Check bounds
	bounds := QuadSeg{P0: p0, P1: p1, P2: p2}.Bounds()
	if !ec.boundsIntersect(bounds) {
		return nil
	}

	// For monotonic curves, we can clip by finding intersection t values
	// and subdividing. This is a simplified approach that works well for
	// most cases. A full implementation would precisely clip at boundaries.

	// Find t parameters where curve crosses clip boundaries
	var tValues []float64

	// Check intersections with all four clip edges
	tValues = append(tValues, ec.quadIntersectY(p0, p1, p2, ec.clip.Y)...)
	tValues = append(tValues, ec.quadIntersectY(p0, p1, p2, ec.clip.Bottom())...)
	tValues = append(tValues, ec.quadIntersectX(p0, p1, p2, ec.clip.X)...)
	tValues = append(tValues, ec.quadIntersectX(p0, p1, p2, ec.clip.Right())...)

	// Filter to valid range (0, 1) and sort
	tValues = filterAndSort(tValues)

	// If no intersections, check if endpoints are inside
	if len(tValues) == 0 {
		if ec.clip.Contains(p0) || ec.clip.Contains(p2) {
			return []QuadSeg{{P0: p0, P1: p1, P2: p2}}
		}
		return nil
	}

	// Split at intersection points and keep segments inside clip
	var result []QuadSeg
	tValues = append([]float64{0}, tValues...)
	tValues = append(tValues, 1)

	for i := 0; i < len(tValues)-1; i++ {
		t0 := tValues[i]
		t1 := tValues[i+1]

		// Get the subsegment
		sub := subdivideQuadRange(p0, p1, p2, t0, t1)

		// Check if midpoint is inside clip
		midT := (t0 + t1) / 2
		mid := evalQuad(p0, p1, p2, midT)
		if ec.clip.Contains(mid) {
			result = append(result, sub)
		}
	}

	return result
}

// quadIntersectY finds t values where the quadratic intersects a horizontal line y=yVal.
func (ec *EdgeClipper) quadIntersectY(p0, p1, p2 Point, yVal float64) []float64 {
	// Quadratic: y(t) = (1-t)^2*p0.Y + 2*(1-t)*t*p1.Y + t^2*p2.Y
	// Rearranged: at^2 + bt + c = 0
	a := p0.Y - 2*p1.Y + p2.Y
	b := 2 * (p1.Y - p0.Y)
	c := p0.Y - yVal

	return solveQuadratic(a, b, c)
}

// quadIntersectX finds t values where the quadratic intersects a vertical line x=xVal.
func (ec *EdgeClipper) quadIntersectX(p0, p1, p2 Point, xVal float64) []float64 {
	a := p0.X - 2*p1.X + p2.X
	b := 2 * (p1.X - p0.X)
	c := p0.X - xVal

	return solveQuadratic(a, b, c)
}

// solveQuadratic solves at^2 + bt + c = 0, returning roots in [0, 1].
func solveQuadratic(a, b, c float64) []float64 {
	const epsilon = 1e-10

	if math.Abs(a) < epsilon {
		// Linear case: bt + c = 0
		if math.Abs(b) < epsilon {
			return nil
		}
		t := -c / b
		if t > 0 && t < 1 {
			return []float64{t}
		}
		return nil
	}

	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		return nil
	}

	sqrtD := math.Sqrt(discriminant)
	t1 := (-b - sqrtD) / (2 * a)
	t2 := (-b + sqrtD) / (2 * a)

	var result []float64
	if t1 > epsilon && t1 < 1-epsilon {
		result = append(result, t1)
	}
	if t2 > epsilon && t2 < 1-epsilon && math.Abs(t2-t1) > epsilon {
		result = append(result, t2)
	}

	return result
}

// evalQuad evaluates a quadratic Bezier at parameter t.
func evalQuad(p0, p1, p2 Point, t float64) Point {
	s := 1 - t
	return Point{
		X: s*s*p0.X + 2*s*t*p1.X + t*t*p2.X,
		Y: s*s*p0.Y + 2*s*t*p1.Y + t*t*p2.Y,
	}
}

// subdivideQuadRange extracts a portion of a quadratic between t0 and t1.
func subdivideQuadRange(p0, p1, p2 Point, t0, t1 float64) QuadSeg {
	// Evaluate endpoints and midpoint
	newP0 := evalQuad(p0, p1, p2, t0)
	newP2 := evalQuad(p0, p1, p2, t1)

	// For the control point, we need to compute it properly
	// using the derivative at t0
	dt := t1 - t0

	// Derivative at t0
	d0 := Point{
		X: 2 * ((1-t0)*(p1.X-p0.X) + t0*(p2.X-p1.X)),
		Y: 2 * ((1-t0)*(p1.Y-p0.Y) + t0*(p2.Y-p1.Y)),
	}

	// Control point is: newP0 + d0 * dt / 2
	newP1 := Point{
		X: newP0.X + d0.X*dt/2,
		Y: newP0.Y + d0.Y*dt/2,
	}

	return QuadSeg{P0: newP0, P1: newP1, P2: newP2}
}

// chopCubicAtYExtrema chops a cubic at its Y extrema.
func (ec *EdgeClipper) chopCubicAtYExtrema(p0, p1, p2, p3 Point, emit func(Point, Point, Point, Point)) {
	// Derivative of cubic is quadratic
	// Find roots of: 3*(p1-p0) + 6*t*(p2-2*p1+p0) + 3*t^2*(p3-3*p2+3*p1-p0)
	// Simplified coefficients for Y:
	a := -p0.Y + 3*p1.Y - 3*p2.Y + p3.Y
	b := 2 * (p0.Y - 2*p1.Y + p2.Y)
	c := p1.Y - p0.Y
	chopCubicAtExtremaRoots(p0, p1, p2, p3, a, b, c, emit)
}

// chopCubicAtXExtrema chops a cubic at its X extrema.
func (ec *EdgeClipper) chopCubicAtXExtrema(p0, p1, p2, p3 Point, emit func(Point, Point, Point, Point)) {
	a := -p0.X + 3*p1.X - 3*p2.X + p3.X
	b := 2 * (p0.X - 2*p1.X + p2.X)
	c := p1.X - p0.X
	chopCubicAtExtremaRoots(p0, p1, p2, p3, a, b, c, emit)
}

// chopCubicAtExtremaRoots is a helper that chops a cubic at roots of at^2 + bt + c = 0.
func chopCubicAtExtremaRoots(p0, p1, p2, p3 Point, a, b, c float64, emit func(Point, Point, Point, Point)) {
	roots := solveQuadratic(a, b, c)

	if len(roots) == 0 {
		emit(p0, p1, p2, p3)
		return
	}

	roots = filterAndSort(roots)

	current := CubicSeg{P0: p0, P1: p1, P2: p2, P3: p3}
	lastT := 0.0

	for _, t := range roots {
		localT := (t - lastT) / (1 - lastT)
		if localT <= 0 || localT >= 1 {
			continue
		}

		left, right := chopCubicAt(current.P0, current.P1, current.P2, current.P3, localT)
		emit(left.P0, left.P1, left.P2, left.P3)
		current = right
		lastT = t
	}

	emit(current.P0, current.P1, current.P2, current.P3)
}

// chopCubicAt subdivides a cubic Bezier at parameter t using de Casteljau.
func chopCubicAt(p0, p1, p2, p3 Point, t float64) (CubicSeg, CubicSeg) {
	// de Casteljau subdivision
	q0 := p0
	q1 := p0.Lerp(p1, t)
	mid1 := p1.Lerp(p2, t)
	q5 := p2.Lerp(p3, t)
	q2 := q1.Lerp(mid1, t)
	q4 := mid1.Lerp(q5, t)
	q3 := q2.Lerp(q4, t)
	q6 := p3

	return CubicSeg{P0: q0, P1: q1, P2: q2, P3: q3},
		CubicSeg{P0: q3, P1: q4, P2: q5, P3: q6}
}

// clipMonoCubic clips a monotonic cubic to the clip bounds.
func (ec *EdgeClipper) clipMonoCubic(p0, p1, p2, p3 Point) []CubicSeg {
	if ec.allInside(p0, p1, p2, p3) {
		return []CubicSeg{{P0: p0, P1: p1, P2: p2, P3: p3}}
	}

	bounds := CubicSeg{P0: p0, P1: p1, P2: p2, P3: p3}.Bounds()
	if !ec.boundsIntersect(bounds) {
		return nil
	}

	// Find intersection t values
	var tValues []float64
	tValues = append(tValues, ec.cubicIntersectY(p0, p1, p2, p3, ec.clip.Y)...)
	tValues = append(tValues, ec.cubicIntersectY(p0, p1, p2, p3, ec.clip.Bottom())...)
	tValues = append(tValues, ec.cubicIntersectX(p0, p1, p2, p3, ec.clip.X)...)
	tValues = append(tValues, ec.cubicIntersectX(p0, p1, p2, p3, ec.clip.Right())...)

	tValues = filterAndSort(tValues)

	if len(tValues) == 0 {
		if ec.clip.Contains(p0) || ec.clip.Contains(p3) {
			return []CubicSeg{{P0: p0, P1: p1, P2: p2, P3: p3}}
		}
		return nil
	}

	var result []CubicSeg
	tValues = append([]float64{0}, tValues...)
	tValues = append(tValues, 1)

	for i := 0; i < len(tValues)-1; i++ {
		t0 := tValues[i]
		t1 := tValues[i+1]

		sub := subdivideCubicRange(p0, p1, p2, p3, t0, t1)

		midT := (t0 + t1) / 2
		mid := evalCubic(p0, p1, p2, p3, midT)
		if ec.clip.Contains(mid) {
			result = append(result, sub)
		}
	}

	return result
}

// cubicIntersectY finds t values where cubic intersects horizontal line y=yVal.
func (ec *EdgeClipper) cubicIntersectY(p0, p1, p2, p3 Point, yVal float64) []float64 {
	// Cubic: y(t) = (1-t)^3*p0 + 3*(1-t)^2*t*p1 + 3*(1-t)*t^2*p2 + t^3*p3
	a := -p0.Y + 3*p1.Y - 3*p2.Y + p3.Y
	b := 3*p0.Y - 6*p1.Y + 3*p2.Y
	c := -3*p0.Y + 3*p1.Y
	d := p0.Y - yVal

	return solveCubic(a, b, c, d)
}

// cubicIntersectX finds t values where cubic intersects vertical line x=xVal.
func (ec *EdgeClipper) cubicIntersectX(p0, p1, p2, p3 Point, xVal float64) []float64 {
	a := -p0.X + 3*p1.X - 3*p2.X + p3.X
	b := 3*p0.X - 6*p1.X + 3*p2.X
	c := -3*p0.X + 3*p1.X
	d := p0.X - xVal

	return solveCubic(a, b, c, d)
}

// solveCubic solves at^3 + bt^2 + ct + d = 0, returning roots in (0, 1).
func solveCubic(a, b, c, d float64) []float64 {
	const epsilon = 1e-10

	if math.Abs(a) < epsilon {
		// Reduce to quadratic
		return solveQuadratic(b, c, d)
	}

	// Normalize: t^3 + pt^2 + qt + r = 0
	p := b / a
	q := c / a
	r := d / a

	// Cardano's method
	// Substitute t = u - p/3
	// u^3 + (q - p^2/3)u + (2p^3/27 - pq/3 + r) = 0
	// u^3 + Au + B = 0
	A := q - p*p/3
	B := 2*p*p*p/27 - p*q/3 + r

	// Discriminant
	disc := B*B/4 + A*A*A/27

	var roots []float64

	switch {
	case disc > epsilon:
		// One real root
		sqrtDisc := math.Sqrt(disc)
		u := cbrt(-B/2+sqrtDisc) + cbrt(-B/2-sqrtDisc)
		t := u - p/3
		if t > epsilon && t < 1-epsilon {
			roots = append(roots, t)
		}
	case disc < -epsilon:
		// Three real roots (Vieta's substitution)
		phi := math.Acos(-B / 2 / math.Sqrt(-A*A*A/27))
		sqrtA := 2 * math.Sqrt(-A/3)
		for k := 0; k < 3; k++ {
			t := sqrtA*math.Cos((phi+float64(k)*2*math.Pi)/3) - p/3
			if t > epsilon && t < 1-epsilon {
				roots = append(roots, t)
			}
		}
	default:
		// Two real roots (one repeated)
		u := cbrt(-B / 2)
		t1 := 2*u - p/3
		t2 := -u - p/3
		if t1 > epsilon && t1 < 1-epsilon {
			roots = append(roots, t1)
		}
		if t2 > epsilon && t2 < 1-epsilon && math.Abs(t2-t1) > epsilon {
			roots = append(roots, t2)
		}
	}

	return roots
}

// cbrt computes the real cube root.
func cbrt(x float64) float64 {
	if x < 0 {
		return -math.Pow(-x, 1.0/3.0)
	}
	return math.Pow(x, 1.0/3.0)
}

// evalCubic evaluates a cubic Bezier at parameter t.
func evalCubic(p0, p1, p2, p3 Point, t float64) Point {
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

// subdivideCubicRange extracts a portion of a cubic between t0 and t1.
func subdivideCubicRange(p0, p1, p2, p3 Point, t0, t1 float64) CubicSeg {
	// Split at t0
	_, right := chopCubicAt(p0, p1, p2, p3, t0)

	// Remap t1 to the right segment
	newT := (t1 - t0) / (1 - t0)
	if newT >= 1 {
		return right
	}

	// Split again
	left, _ := chopCubicAt(right.P0, right.P1, right.P2, right.P3, newT)
	return left
}

// filterAndSort filters t values to (0, 1) and sorts them.
func filterAndSort(values []float64) []float64 {
	const epsilon = 1e-10
	var result []float64

	for _, v := range values {
		if v > epsilon && v < 1-epsilon {
			result = append(result, v)
		}
	}

	// Simple sort (usually 0-3 elements)
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j] < result[i] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	// Remove duplicates
	if len(result) <= 1 {
		return result
	}

	unique := []float64{result[0]}
	for i := 1; i < len(result); i++ {
		if result[i]-unique[len(unique)-1] > epsilon {
			unique = append(unique, result[i])
		}
	}

	return unique
}
