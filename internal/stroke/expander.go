// Package stroke provides stroke expansion algorithms for converting stroked paths to filled outlines.
//
// This package implements CPU-side stroke expansion following tiny-skia and kurbo patterns.
// The algorithm converts a path with stroke style into a filled path suitable for GPU rasterization.
//
// Key algorithm insight: A stroke is converted to a FILL path where:
//   - The outer offset path goes forward
//   - The inner offset path is reversed
//   - Line caps connect the endpoints
//   - Line joins connect the segments
package stroke

import (
	"math"
)

// Point represents a 2D point (internal copy to avoid import cycle).
type Point struct {
	X, Y float64
}

// Vec2 returns the point as a vector from the origin.
func (p Point) Vec2() Vec2 {
	return Vec2(p)
}

// Add returns the sum of two points.
func (p Point) Add(v Vec2) Point {
	return Point{X: p.X + v.X, Y: p.Y + v.Y}
}

// Sub returns the difference between two points as a vector.
func (p Point) Sub(q Point) Vec2 {
	return Vec2{X: p.X - q.X, Y: p.Y - q.Y}
}

// Distance returns the distance between two points.
func (p Point) Distance(q Point) float64 {
	return p.Sub(q).Length()
}

// Lerp performs linear interpolation between two points.
func (p Point) Lerp(q Point, t float64) Point {
	return Point{
		X: p.X + (q.X-p.X)*t,
		Y: p.Y + (q.Y-p.Y)*t,
	}
}

// Vec2 represents a 2D vector.
type Vec2 struct {
	X, Y float64
}

// Add returns the sum of two vectors.
func (v Vec2) Add(w Vec2) Vec2 {
	return Vec2{X: v.X + w.X, Y: v.Y + w.Y}
}

// Sub returns the difference of two vectors.
func (v Vec2) Sub(w Vec2) Vec2 {
	return Vec2{X: v.X - w.X, Y: v.Y - w.Y}
}

// Scale returns the vector scaled by s.
func (v Vec2) Scale(s float64) Vec2 {
	return Vec2{X: v.X * s, Y: v.Y * s}
}

// Neg returns the negated vector.
func (v Vec2) Neg() Vec2 {
	return Vec2{X: -v.X, Y: -v.Y}
}

// Dot returns the dot product of two vectors.
func (v Vec2) Dot(w Vec2) float64 {
	return v.X*w.X + v.Y*w.Y
}

// Cross returns the 2D cross product (z-component of 3D cross).
func (v Vec2) Cross(w Vec2) float64 {
	return v.X*w.Y - v.Y*w.X
}

// Length returns the length of the vector.
func (v Vec2) Length() float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y)
}

// LengthSquared returns the squared length of the vector.
func (v Vec2) LengthSquared() float64 {
	return v.X*v.X + v.Y*v.Y
}

// Normalize returns a unit vector in the same direction.
func (v Vec2) Normalize() Vec2 {
	length := v.Length()
	if length < 1e-10 {
		return Vec2{X: 0, Y: 0}
	}
	return Vec2{X: v.X / length, Y: v.Y / length}
}

// Perp returns the perpendicular vector (rotated 90 degrees counter-clockwise).
func (v Vec2) Perp() Vec2 {
	return Vec2{X: -v.Y, Y: v.X}
}

// ToPoint converts the vector to a point.
func (v Vec2) ToPoint() Point {
	return Point(v)
}

// Angle returns the angle of the vector in radians.
func (v Vec2) Angle() float64 {
	return math.Atan2(v.Y, v.X)
}

// LineCap specifies the shape of line endpoints.
type LineCap int

const (
	// LineCapButt specifies a flat line cap.
	LineCapButt LineCap = iota
	// LineCapRound specifies a rounded line cap.
	LineCapRound
	// LineCapSquare specifies a square line cap.
	LineCapSquare
)

// LineJoin specifies the shape of line joins.
type LineJoin int

const (
	// LineJoinMiter specifies a sharp (mitered) join.
	LineJoinMiter LineJoin = iota
	// LineJoinRound specifies a rounded join.
	LineJoinRound
	// LineJoinBevel specifies a beveled join.
	LineJoinBevel
)

// Stroke defines the style for stroke expansion.
type Stroke struct {
	Width      float64
	Cap        LineCap
	Join       LineJoin
	MiterLimit float64
}

// DefaultStroke returns a stroke with default settings.
func DefaultStroke() Stroke {
	return Stroke{
		Width:      1.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
	}
}

// PathElement represents an element in a path.
type PathElement interface {
	isPathElement()
}

// MoveTo moves to a point.
type MoveTo struct{ Point Point }

func (MoveTo) isPathElement() {}

// LineTo draws a line.
type LineTo struct{ Point Point }

func (LineTo) isPathElement() {}

// QuadTo draws a quadratic Bezier curve.
type QuadTo struct{ Control, Point Point }

func (QuadTo) isPathElement() {}

// CubicTo draws a cubic Bezier curve.
type CubicTo struct{ Control1, Control2, Point Point }

func (CubicTo) isPathElement() {}

// Close closes the path.
type Close struct{}

func (Close) isPathElement() {}

// StrokeExpander converts stroked paths to filled paths.
// This follows the kurbo stroke expansion algorithm.
type StrokeExpander struct {
	style Stroke

	// Tolerance for curve flattening and arc approximation.
	// Smaller values produce more accurate results but more segments.
	tolerance float64

	// Build state
	forward  *pathBuilder
	backward *pathBuilder
	output   *pathBuilder

	// Current segment state
	startPt   Point
	startNorm Vec2
	startTan  Vec2
	lastPt    Point
	lastTan   Vec2
	lastNorm  Vec2 // Normal at lastPt (scaled by radius), used for end cap

	// Join threshold for skipping small joins
	joinThresh float64
}

// NewStrokeExpander creates a new stroke expander with the given style.
func NewStrokeExpander(style Stroke) *StrokeExpander {
	return &StrokeExpander{
		style:     style,
		tolerance: 0.25, // Default tolerance
	}
}

// SetTolerance sets the curve flattening tolerance.
func (e *StrokeExpander) SetTolerance(tolerance float64) {
	if tolerance > 0 {
		e.tolerance = tolerance
	}
}

// Expand converts a stroked path to a fill path.
func (e *StrokeExpander) Expand(elements []PathElement) []PathElement {
	e.reset()

	for _, el := range elements {
		switch elem := el.(type) {
		case MoveTo:
			e.finish()
			e.startPt = elem.Point
			e.lastPt = elem.Point
		case LineTo:
			if elem.Point != e.lastPt {
				tangent := elem.Point.Sub(e.lastPt)
				e.doJoin(tangent)
				e.lastTan = tangent
				e.doLine(tangent, elem.Point)
			}
		case QuadTo:
			if elem.Control != e.lastPt || elem.Point != e.lastPt {
				// Flatten quadratic to lines for simplicity
				e.doQuad(elem.Control, elem.Point)
			}
		case CubicTo:
			if elem.Control1 != e.lastPt || elem.Control2 != e.lastPt || elem.Point != e.lastPt {
				// Flatten cubic to lines for simplicity
				e.doCubic(elem.Control1, elem.Control2, elem.Point)
			}
		case Close:
			if e.lastPt != e.startPt {
				tangent := e.startPt.Sub(e.lastPt)
				e.doJoin(tangent)
				e.lastTan = tangent
				e.doLine(tangent, e.startPt)
			}
			e.finishClosed()
		}
	}

	e.finish()
	return e.output.build()
}

// reset clears the expander state for a new expansion.
func (e *StrokeExpander) reset() {
	e.forward = newPathBuilder()
	e.backward = newPathBuilder()
	e.output = newPathBuilder()
	e.startPt = Point{}
	e.startNorm = Vec2{}
	e.startTan = Vec2{}
	e.lastPt = Point{}
	e.lastTan = Vec2{}
	e.lastNorm = Vec2{}
	e.joinThresh = 2.0 * e.tolerance / e.style.Width
}

// doJoin handles joining the current segment to the previous one.
func (e *StrokeExpander) doJoin(tan0 Vec2) {
	scale := 0.5 * e.style.Width / tan0.Length()
	norm := tan0.Perp().Scale(scale)
	p0 := e.lastPt

	if e.forward.isEmpty() {
		e.startFirstSegment(p0, norm, tan0)
		return
	}
	e.joinWithPrevious(p0, norm, tan0)
}

// startFirstSegment initializes the forward and backward paths for the first segment.
func (e *StrokeExpander) startFirstSegment(p0 Point, norm, tan0 Vec2) {
	e.forward.moveTo(p0.Add(norm.Neg()))
	e.backward.moveTo(p0.Add(norm))
	e.startTan = tan0
	e.startNorm = norm
}

// joinWithPrevious handles joining with the previous segment.
func (e *StrokeExpander) joinWithPrevious(p0 Point, norm, tan0 Vec2) {
	ab := e.lastTan
	cd := tan0
	cross := ab.Cross(cd)
	dot := ab.Dot(cd)
	hypot := math.Hypot(cross, dot)

	// Skip join if angle change is insignificant, but still connect paths
	// to maintain continuity. Without the lineTo calls, the forward/backward
	// paths have a gap at circle cardinal points where tangents are identical.
	if dot > 0.0 && math.Abs(cross) < hypot*e.joinThresh {
		e.forward.lineTo(p0.Add(norm.Neg()))
		e.backward.lineTo(p0.Add(norm))
		return
	}

	switch e.style.Join {
	case LineJoinBevel:
		e.applyBevelJoin(p0, norm)
	case LineJoinMiter:
		e.applyMiterJoin(p0, norm, ab, cd, cross, dot, hypot)
	case LineJoinRound:
		e.applyRoundJoin(p0, norm, cross, dot)
	}
}

// applyBevelJoin applies a bevel join at the given point.
func (e *StrokeExpander) applyBevelJoin(p0 Point, norm Vec2) {
	e.forward.lineTo(p0.Add(norm.Neg()))
	e.backward.lineTo(p0.Add(norm))
}

// applyMiterJoin applies a miter join at the given point.
func (e *StrokeExpander) applyMiterJoin(p0 Point, norm, ab, cd Vec2, cross, dot, hypot float64) {
	miterLimitSq := e.style.MiterLimit * e.style.MiterLimit
	if 2.0*hypot < (hypot+dot)*miterLimitSq {
		e.computeMiterPoint(p0, norm, ab, cd, cross)
	}
	e.forward.lineTo(p0.Add(norm.Neg()))
	e.backward.lineTo(p0.Add(norm))
}

// computeMiterPoint computes and applies the miter point.
func (e *StrokeExpander) computeMiterPoint(p0 Point, norm, ab, cd Vec2, cross float64) {
	lastScale := 0.5 * e.style.Width / ab.Length()
	lastNorm := ab.Perp().Scale(lastScale)

	if cross > 0.0 {
		// Join on forward path
		fpLast := p0.Add(lastNorm.Neg())
		fpThis := p0.Add(norm.Neg())
		h := ab.Cross(fpThis.Sub(fpLast.Vec2().ToPoint())) / cross
		miterPt := fpThis.Add(cd.Scale(-h))
		e.forward.lineTo(miterPt)
		e.backward.lineTo(p0)
	} else if cross < 0.0 {
		// Join on backward path
		fpLast := p0.Add(lastNorm)
		fpThis := p0.Add(norm)
		h := ab.Cross(fpThis.Sub(fpLast.Vec2().ToPoint())) / cross
		miterPt := fpThis.Add(cd.Scale(-h))
		e.backward.lineTo(miterPt)
		e.forward.lineTo(p0)
	}
}

// applyRoundJoin applies a round join at the given point.
// The arc goes from the previous segment's normal (lastNorm) to the current normal (norm).
func (e *StrokeExpander) applyRoundJoin(p0 Point, norm Vec2, cross, dot float64) {
	// Compute lastNorm from lastTan (same pattern as computeMiterPoint)
	lastScale := 0.5 * e.style.Width / e.lastTan.Length()
	lastNorm := e.lastTan.Perp().Scale(lastScale)

	angle := math.Atan2(cross, dot)
	if angle > 0.0 {
		e.backward.lineTo(p0.Add(norm))
		e.roundJoin(e.forward, p0, lastNorm.Neg(), angle)
	} else {
		e.forward.lineTo(p0.Add(norm.Neg()))
		e.roundJoinRev(e.backward, p0, lastNorm, -angle)
	}
}

// doLine extends both paths with a line segment.
func (e *StrokeExpander) doLine(tangent Vec2, p1 Point) {
	scale := 0.5 * e.style.Width / tangent.Length()
	norm := tangent.Perp().Scale(scale)

	e.forward.lineTo(p1.Add(norm.Neg()))
	e.backward.lineTo(p1.Add(norm))
	e.lastPt = p1
	e.lastNorm = norm // Save normal for end cap (tiny-skia pattern)
}

// doQuad handles a quadratic Bezier curve by flattening it.
func (e *StrokeExpander) doQuad(control, end Point) {
	// Flatten quadratic to lines
	points := e.flattenQuad(e.lastPt, control, end)
	for i := 1; i < len(points); i++ {
		tangent := points[i].Sub(points[i-1])
		if tangent.LengthSquared() > 1e-10 {
			e.doJoin(tangent)
			e.lastTan = tangent
			e.doLine(tangent, points[i])
		}
	}
}

// doCubic handles a cubic Bezier curve by flattening it.
func (e *StrokeExpander) doCubic(c1, c2, end Point) {
	// Flatten cubic to lines
	points := e.flattenCubic(e.lastPt, c1, c2, end)
	for i := 1; i < len(points); i++ {
		tangent := points[i].Sub(points[i-1])
		if tangent.LengthSquared() > 1e-10 {
			e.doJoin(tangent)
			e.lastTan = tangent
			e.doLine(tangent, points[i])
		}
	}
}

// finish completes an open subpath with end caps.
func (e *StrokeExpander) finish() {
	if e.forward.isEmpty() {
		return
	}

	// Copy forward path to output
	e.output.appendPath(e.forward)

	// Apply end cap using saved normal from last line segment.
	// This follows the tiny-skia pattern: use prev_normal instead of
	// computing from points, which would give incorrect cap direction.
	// Note: lastNorm points toward backward path, but applyCap expects
	// the normal pointing toward forward path (from where we're drawing),
	// so we negate it.
	if len(e.backward.elements) > 0 {
		e.applyCap(e.style.Cap, e.lastPt, e.lastNorm.Neg(), false)
	}

	// Append reversed backward path
	e.appendReversed(e.backward)

	// Apply start cap and close
	e.applyCap(e.style.Cap, e.startPt, e.startNorm, true)

	// Clear for next subpath
	e.forward = newPathBuilder()
	e.backward = newPathBuilder()
}

// finishClosed completes a closed subpath.
func (e *StrokeExpander) finishClosed() {
	if e.forward.isEmpty() {
		return
	}

	// Join back to start
	e.doJoin(e.startTan)

	// Copy forward path and close
	e.output.appendPath(e.forward)
	e.output.close()

	// Handle backward path separately
	backElems := e.backward.elements
	if len(backElems) > 0 {
		lastPt := getEndPoint(backElems[len(backElems)-1])
		e.output.moveTo(lastPt)
	}
	e.appendReversed(e.backward)
	e.output.close()

	// Clear for next subpath
	e.forward = newPathBuilder()
	e.backward = newPathBuilder()
}

// applyCap applies a line cap at the given position.
func (e *StrokeExpander) applyCap(capStyle LineCap, center Point, norm Vec2, closePath bool) {
	switch capStyle {
	case LineCapButt:
		if closePath {
			e.output.close()
		} else {
			// Line to the other side
			returnPt := center.Add(norm.Neg())
			e.output.lineTo(returnPt)
		}

	case LineCapRound:
		e.roundCap(e.output, center, norm)
		if closePath {
			e.output.close()
		}

	case LineCapSquare:
		e.squareCap(e.output, center, norm, closePath)
	}
}

// roundCap adds a rounded cap.
func (e *StrokeExpander) roundCap(out *pathBuilder, center Point, norm Vec2) {
	e.roundJoin(out, center, norm, math.Pi)
}

// roundJoin adds a round join arc.
func (e *StrokeExpander) roundJoin(out *pathBuilder, center Point, norm Vec2, angle float64) {
	// Approximate arc with cubic Beziers
	// For a 90-degree arc, we use the standard k = 0.5522847498
	numSegments := int(math.Ceil(math.Abs(angle) / (math.Pi / 2)))
	if numSegments < 1 {
		numSegments = 1
	}

	angleStep := angle / float64(numSegments)
	currentAngle := norm.Angle()
	radius := norm.Length()

	for i := 0; i < numSegments; i++ {
		a0 := currentAngle
		a1 := currentAngle + angleStep
		e.arcSegment(out, center, radius, a0, a1)
		currentAngle = a1
	}
}

// roundJoinRev adds a round join arc in reverse direction.
func (e *StrokeExpander) roundJoinRev(out *pathBuilder, center Point, norm Vec2, angle float64) {
	e.roundJoin(out, center, norm.Neg(), angle)
}

// arcSegment adds a single arc segment (up to 90 degrees) using cubic Bezier.
func (e *StrokeExpander) arcSegment(out *pathBuilder, center Point, radius, a0, a1 float64) {
	// Calculate control points for cubic Bezier approximation of arc
	// Using formula from "Drawing an elliptical arc using polylines, quadratic or cubic Bezier curves"
	da := a1 - a0
	alpha := math.Sin(da) * (math.Sqrt(4+3*math.Tan(da/2)*math.Tan(da/2)) - 1) / 3

	cos0, sin0 := math.Cos(a0), math.Sin(a0)
	cos1, sin1 := math.Cos(a1), math.Sin(a1)

	p1 := Point{X: center.X + radius*cos0, Y: center.Y + radius*sin0}
	p2 := Point{X: center.X + radius*cos1, Y: center.Y + radius*sin1}

	c1 := Point{X: p1.X - alpha*radius*sin0, Y: p1.Y + alpha*radius*cos0}
	c2 := Point{X: p2.X + alpha*radius*sin1, Y: p2.Y - alpha*radius*cos1}

	out.cubicTo(c1, c2, p2)
}

// squareCap adds a square cap.
func (e *StrokeExpander) squareCap(out *pathBuilder, center Point, norm Vec2, closePath bool) {
	// Create affine transform: norm.x, norm.y, -norm.y, norm.x, center.x, center.y
	// Apply to square corners at (+1, +1), (-1, +1), (-1, 0)
	p1 := e.transformPoint(center, norm, Point{X: 1, Y: 1})
	p2 := e.transformPoint(center, norm, Point{X: -1, Y: 1})

	out.lineTo(p1)
	out.lineTo(p2)

	if closePath {
		out.close()
	} else {
		p3 := e.transformPoint(center, norm, Point{X: -1, Y: 0})
		out.lineTo(p3)
	}
}

// transformPoint applies the affine transform: [norm.x, norm.y, -norm.y, norm.x, center.x, center.y].
func (e *StrokeExpander) transformPoint(center Point, norm Vec2, p Point) Point {
	return Point{
		X: norm.X*p.X - norm.Y*p.Y + center.X,
		Y: norm.Y*p.X + norm.X*p.Y + center.Y,
	}
}

// appendReversed appends the backward path in reverse order.
func (e *StrokeExpander) appendReversed(pb *pathBuilder) {
	elems := pb.elements
	for i := len(elems) - 1; i >= 1; i-- {
		endPt := getEndPoint(elems[i-1])
		switch el := elems[i].(type) {
		case LineTo:
			e.output.lineTo(endPt)
		case QuadTo:
			e.output.quadTo(el.Control, endPt)
		case CubicTo:
			e.output.cubicTo(el.Control2, el.Control1, endPt)
		}
	}
}

// flattenQuad flattens a quadratic Bezier curve to line segments.
func (e *StrokeExpander) flattenQuad(p0, p1, p2 Point) []Point {
	points := []Point{p0}
	e.flattenQuadRec(p0, p1, p2, &points)
	return points
}

func (e *StrokeExpander) flattenQuadRec(p0, p1, p2 Point, points *[]Point) {
	// Check if curve is flat enough
	dist := distanceToLine(p1, p0, p2)
	if dist < e.tolerance {
		*points = append(*points, p2)
		return
	}

	// Subdivide
	q0 := p0.Lerp(p1, 0.5)
	q1 := p1.Lerp(p2, 0.5)
	q2 := q0.Lerp(q1, 0.5)

	e.flattenQuadRec(p0, q0, q2, points)
	e.flattenQuadRec(q2, q1, p2, points)
}

// flattenCubic flattens a cubic Bezier curve to line segments.
func (e *StrokeExpander) flattenCubic(p0, p1, p2, p3 Point) []Point {
	points := []Point{p0}
	e.flattenCubicRec(p0, p1, p2, p3, &points)
	return points
}

func (e *StrokeExpander) flattenCubicRec(p0, p1, p2, p3 Point, points *[]Point) {
	// Check if curve is flat enough
	d1 := distanceToLine(p1, p0, p3)
	d2 := distanceToLine(p2, p0, p3)
	dist := math.Max(d1, d2)

	if dist < e.tolerance {
		*points = append(*points, p3)
		return
	}

	// Subdivide using de Casteljau's algorithm
	q0 := p0.Lerp(p1, 0.5)
	q1 := p1.Lerp(p2, 0.5)
	q2 := p2.Lerp(p3, 0.5)
	r0 := q0.Lerp(q1, 0.5)
	r1 := q1.Lerp(q2, 0.5)
	s := r0.Lerp(r1, 0.5)

	e.flattenCubicRec(p0, q0, r0, s, points)
	e.flattenCubicRec(s, r1, q2, p3, points)
}

// distanceToLine calculates the perpendicular distance from point p to line segment (a, b).
func distanceToLine(p, a, b Point) float64 {
	ab := b.Sub(a)
	abLen := ab.Length()

	if abLen < 1e-10 {
		return p.Distance(a)
	}

	// Project p onto the line
	ap := p.Sub(a)
	t := ap.Dot(ab) / (abLen * abLen)

	if t < 0 {
		return p.Distance(a)
	}
	if t > 1 {
		return p.Distance(b)
	}

	closest := a.Add(ab.Scale(t))
	return p.Distance(closest)
}

// getEndPoint returns the endpoint of a path element.
func getEndPoint(el PathElement) Point {
	switch e := el.(type) {
	case MoveTo:
		return e.Point
	case LineTo:
		return e.Point
	case QuadTo:
		return e.Point
	case CubicTo:
		return e.Point
	default:
		return Point{}
	}
}

// pathBuilder is a helper for building paths.
type pathBuilder struct {
	elements []PathElement
	current  Point
}

func newPathBuilder() *pathBuilder {
	return &pathBuilder{
		elements: make([]PathElement, 0, 64),
	}
}

func (b *pathBuilder) isEmpty() bool {
	return len(b.elements) == 0
}

func (b *pathBuilder) moveTo(p Point) {
	b.elements = append(b.elements, MoveTo{Point: p})
	b.current = p
}

func (b *pathBuilder) lineTo(p Point) {
	b.elements = append(b.elements, LineTo{Point: p})
	b.current = p
}

func (b *pathBuilder) quadTo(c, p Point) {
	b.elements = append(b.elements, QuadTo{Control: c, Point: p})
	b.current = p
}

func (b *pathBuilder) cubicTo(c1, c2, p Point) {
	b.elements = append(b.elements, CubicTo{Control1: c1, Control2: c2, Point: p})
	b.current = p
}

func (b *pathBuilder) close() {
	b.elements = append(b.elements, Close{})
}

func (b *pathBuilder) appendPath(other *pathBuilder) {
	for i, el := range other.elements {
		if i == 0 {
			if _, ok := el.(MoveTo); ok {
				// Copy MoveTo as-is
				b.elements = append(b.elements, el)
				continue
			}
		}
		b.elements = append(b.elements, el)
	}
}

func (b *pathBuilder) build() []PathElement {
	return b.elements
}
