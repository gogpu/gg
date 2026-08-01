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

// PathVerb represents a path construction command.
// Values match gg.PathVerb for zero-cost conversion.
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

// verbCoordCount returns the number of float64 coordinates consumed by a verb.
func verbCoordCount(v PathVerb) int {
	switch v {
	case VerbMoveTo, VerbLineTo:
		return 2
	case VerbQuadTo:
		return 4
	case VerbCubicTo:
		return 6
	default:
		return 0
	}
}

// StrokeExpander converts stroked paths to filled paths.
// This follows the tiny-skia/Skia stroke expansion algorithm.
//
// StrokeExpander is designed for reuse: call Expand() multiple times on the same
// instance. Internal buffers are retained and reused between calls to minimize
// heap allocations.
type StrokeExpander struct {
	style Stroke

	// Tolerance for curve flattening and arc approximation.
	// Smaller values produce more accurate results but more segments.
	tolerance float64

	// Build state — embedded structs, reused between Expand() calls.
	forward  pathBuilder
	backward pathBuilder
	output   pathBuilder

	// Current segment state
	startPt   Point
	startNorm Vec2
	startTan  Vec2
	lastPt    Point
	lastTan   Vec2
	lastNorm  Vec2 // Normal at lastPt (scaled by radius), used for end cap

	// Join threshold for skipping small joins
	joinThresh float64

	// hadInnerJoin is true if handleInnerJoin was called during the last Expand.
	// When false, the expanded path has no inner-pivot V-shapes and can be
	// rendered with NonZero fill rule (avoids StencilOperationInvert).
	hadInnerJoin bool

	// prevIsLine tracks whether the previous segment was a line (not a curve).
	// Skia/tiny-skia use this to call setLastPoint instead of lineTo in joins,
	// which avoids adding extra vertices at line→line transitions.
	prevIsLine bool

	// currIsLine tracks whether the current segment being joined to is a line.
	// tiny-skia miter_joiner:1551 skips final outer lineTo when curr_is_line,
	// because doLine will write the correct position anyway.
	currIsLine bool

	// foundTangents persists between outer/inner cubicStroke calls (Skia pattern).
	// Reset in doCubic before the outer+inner pair, NOT between them.
	// This causes inner stroke to skip Phase 1 when outer already found tangents,
	// leading to stricter Phase 2 convergence → more subdivision on inner curves.
	foundTangents bool

	// Reusable buffer for curve flattening (flattenQuad/flattenCubic).
	// Retained between calls to avoid per-curve allocations.
	flattenBuf []Point
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

// Expand converts a stroked path (given as SOA verb+coords) to a filled path.
// Returns the expanded path as (verbs, coords) slices.
func (e *StrokeExpander) Expand(verbs []PathVerb, coords []float64) ([]PathVerb, []float64) {
	e.reset()

	ci := 0
	for _, v := range verbs {
		switch v {
		case VerbMoveTo:
			pt := Point{X: coords[ci], Y: coords[ci+1]}
			e.finish()
			e.startPt = pt
			e.lastPt = pt
			ci += 2
		case VerbLineTo:
			pt := Point{X: coords[ci], Y: coords[ci+1]}
			if pt != e.lastPt {
				tangent := pt.Sub(e.lastPt)
				e.currIsLine = true
				e.doJoin(tangent)
				e.lastTan = tangent
				e.doLine(tangent, pt)
			}
			ci += 2
		case VerbQuadTo:
			ctrl := Point{X: coords[ci], Y: coords[ci+1]}
			pt := Point{X: coords[ci+2], Y: coords[ci+3]}
			if ctrl != e.lastPt || pt != e.lastPt {
				e.currIsLine = false
				e.doQuad(ctrl, pt)
			}
			ci += 4
		case VerbCubicTo:
			c1 := Point{X: coords[ci], Y: coords[ci+1]}
			c2 := Point{X: coords[ci+2], Y: coords[ci+3]}
			pt := Point{X: coords[ci+4], Y: coords[ci+5]}
			if c1 != e.lastPt || c2 != e.lastPt || pt != e.lastPt {
				e.currIsLine = false
				e.doCubic(c1, c2, pt)
			}
			ci += 6
		case VerbClose:
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
	return e.output.verbs, e.output.coords
}

// HadInnerJoin reports whether handleInnerJoin was called during the last Expand.
// When false, all joins were skipped (smooth path) and the expansion produces
// no inner-pivot V-shapes, making it safe to use NonZero fill rule.
func (e *StrokeExpander) HadInnerJoin() bool {
	return e.hadInnerJoin
}

// reset clears the expander state for a new expansion.
// Buffers are truncated but retain their backing arrays for reuse.
func (e *StrokeExpander) reset() {
	e.forward.reset()
	e.backward.reset()
	e.output.reset()
	e.startPt = Point{}
	e.startNorm = Vec2{}
	e.startTan = Vec2{}
	e.lastPt = Point{}
	e.lastTan = Vec2{}
	e.lastNorm = Vec2{}
	e.joinThresh = 2.0 * e.tolerance / e.style.Width
	e.hadInnerJoin = false
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
//
// The key insight (from Skia/tiny-skia/Cairo) is that the two sides of a join
// must be treated asymmetrically:
//   - Outer (convex) side: receives join decoration (miter/bevel/round)
//   - Inner (concave) side: routes through the pivot point to prevent self-intersection
//
// The cross product of consecutive tangents determines which side is which:
//   - cross > 0 (left turn): forward is outer, backward is inner
//   - cross < 0 (right turn): backward is outer, forward is inner
func (e *StrokeExpander) joinWithPrevious(p0 Point, norm, tan0 Vec2) {
	ab := e.lastTan
	cd := tan0
	cross := ab.Cross(cd)
	dot := ab.Dot(cd)
	hypot := math.Hypot(cross, dot)

	// Skip join if angle change is insignificant (kurbo stroke.rs:428).
	// Rust kurbo emits nothing here — the paths continue without explicit
	// connecting segments. The connection happens implicitly from the next
	// doLine() which adds lineTo for both forward and backward paths.
	if dot > 0.0 && math.Abs(cross) < hypot*e.joinThresh {
		return
	}

	// Compute the previous segment's normal (needed for miter point and round arc).
	lastScale := 0.5 * e.style.Width / ab.Length()
	lastNorm := ab.Perp().Scale(lastScale)

	switch {
	case cross > 0.0:
		// Left turn: forward path is outer (convex), backward is inner (concave).
		e.applyOuterJoin(&e.forward, p0, lastNorm.Neg(), norm.Neg(), ab, cd, dot, hypot)
		e.handleInnerJoin(&e.backward, p0, norm)
	case cross < 0.0:
		// Right turn: backward path is outer (convex), forward is inner (concave).
		e.applyOuterJoin(&e.backward, p0, lastNorm, norm, ab, cd, dot, hypot)
		e.handleInnerJoin(&e.forward, p0, norm.Neg())
	default:
		// Exactly parallel (cross == 0). This includes near-180-degree reversals
		// (dot < 0) and exactly collinear (dot > 0). Just connect both sides.
		e.forward.lineTo(p0.Add(norm.Neg()))
		e.backward.lineTo(p0.Add(norm))
	}
}

// handleInnerJoin handles the concave (inner) side of a join.
//
// Two-step routing (Skia HandleInnerJoin in SkStrokerPriv.cpp:72-84):
//  1. lineTo(pivot) — route through the center to prevent self-intersection
//  2. lineTo(pivot + afterNorm) — place at correct normal offset for next segment
//
// afterNorm points toward the inner path's side (already oriented by the caller:
// cross>0 passes norm toward backward, cross<0 passes norm.Neg() toward forward).
// Without step 2, the inner path "jumps" diagonally from pivot to the next
// doLine() position, creating visible teeth on thick strokes (#354).
func (e *StrokeExpander) handleInnerJoin(path *pathBuilder, pivot Point, afterNorm Vec2) {
	e.hadInnerJoin = true
	path.lineTo(pivot)
	path.lineTo(pivot.Add(afterNorm))
}

// applyOuterJoin applies the requested join type to the outer (convex) side of a join.
// The inner side is handled separately by handleInnerJoin.
//
// Parameters:
//   - outerPath: the path builder for the outer (convex) side
//   - p0: the join vertex (pivot point)
//   - lastNorm: normal of the previous segment (pointing away from center, toward this outer path)
//   - norm: normal of the current segment (pointing away from center, toward this outer path)
//   - ab, cd: tangent vectors of previous and current segments
//   - dot: dot product of tangents
//   - hypot: hypot(cross, dot)
func (e *StrokeExpander) applyOuterJoin(
	outerPath *pathBuilder, p0 Point, lastNorm, norm, ab, cd Vec2,
	dot, hypot float64,
) {
	switch e.style.Join {
	case LineJoinBevel:
		outerPath.lineTo(p0.Add(norm))
	case LineJoinMiter:
		e.applyOuterMiterJoin(outerPath, p0, lastNorm, norm, ab, cd, dot, hypot)
	case LineJoinRound:
		e.applyOuterRoundJoin(outerPath, p0, lastNorm, norm)
	}
}

// applyOuterMiterJoin applies a miter join on the outer (convex) side.
// If the miter limit is exceeded, falls back to bevel.
func (e *StrokeExpander) applyOuterMiterJoin(
	outerPath *pathBuilder, p0 Point, lastNorm, norm, ab, cd Vec2,
	dot, hypot float64,
) {
	miterLimitSq := e.style.MiterLimit * e.style.MiterLimit
	if 2.0*hypot < (hypot+dot)*miterLimitSq {
		// Compute miter point: intersection of the two offset lines.
		// Use signed cross product ab×cd (not |cross|) so the parametric
		// factor h has the correct sign for both left turns (cross>0,
		// negated normals) and right turns (cross<0, positive normals).
		fpLast := p0.Add(lastNorm)
		fpThis := p0.Add(norm)
		signedCross := ab.Cross(cd)
		h := ab.Cross(fpThis.Sub(fpLast.Vec2().ToPoint())) / signedCross
		miterPt := fpThis.Add(cd.Scale(-h))
		// Skia/tiny-skia: when previous segment was a line, replace last point
		// instead of adding new vertex (set_last_point pattern).
		if e.prevIsLine {
			outerPath.setLastPoint(miterPt)
		} else {
			outerPath.lineTo(miterPt)
		}
	}
	// tiny-skia miter_joiner:1551: skip final lineTo when current segment is
	// a line — doLine will write the correct position anyway.
	if !e.currIsLine {
		outerPath.lineTo(p0.Add(norm))
	}
}

// applyOuterRoundJoin applies a round join arc on the outer (convex) side.
// The arc sweeps from lastNorm to norm around the pivot point p0.
func (e *StrokeExpander) applyOuterRoundJoin(outerPath *pathBuilder, p0 Point, lastNorm, norm Vec2) {
	// Compute the sweep angle between the two normals.
	// Both normals point outward on the same (convex) side, so the angle
	// between them is the exterior angle at the join.
	crossN := lastNorm.Cross(norm)
	dotN := lastNorm.Dot(norm)
	angle := math.Atan2(crossN, dotN)

	if math.Abs(angle) < 1e-6 {
		// Normals are nearly identical; just connect with a line.
		outerPath.lineTo(p0.Add(norm))
		return
	}

	e.roundJoin(outerPath, p0, lastNorm, angle)
}

// doLine extends both paths with a line segment.
func (e *StrokeExpander) doLine(tangent Vec2, p1 Point) {
	scale := 0.5 * e.style.Width / tangent.Length()
	norm := tangent.Perp().Scale(scale)

	e.forward.lineTo(p1.Add(norm.Neg()))
	e.backward.lineTo(p1.Add(norm))
	e.lastPt = p1
	e.lastNorm = norm
	e.prevIsLine = true
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

// doCubic handles a cubic Bezier curve using Skia's direct offset-curve algorithm.
//
// Instead of flatten-then-offset (which creates spurious joins at every flattened
// vertex), this directly approximates the offset curve with quadratic Beziers using
// Skia's two-phase recursive approach:
//
//	Phase 1 (tangentsMeet): cheap check — do tangent rays converge? (limit=15)
//	Phase 2 (compareQuadCubic): full quality check — is quad close to offset? (limit=24)
//
// The cubic is first split at inflection points to ensure monotonic curvature
// within each segment. Cusps get a filled circle (Skia SkFindCubicCusp pattern).
//
// Reference: Skia SkStroke.cpp cubicTo() (lines 1314-1374)
func (e *StrokeExpander) doCubic(c1, c2, end Point) {
	cubic := [4]Point{e.lastPt, c1, c2, end}
	radius := 0.5 * e.style.Width

	// Degenerate check: all points equal → treat as zero-length line.
	ab := cubic[1].Sub(cubic[0])
	cd := cubic[3].Sub(cubic[2])
	degAB := ab.LengthSquared() < 1e-14
	degCD := cd.LengthSquared() < 1e-14
	bc := cubic[2].Sub(cubic[1])
	degBC := bc.LengthSquared() < 1e-14
	if degAB && degBC && degCD {
		return // point degenerate
	}

	// Entry join: connect previous segment to start tangent.
	// Use Skia's tangent priority: try P1-P0, fallback to P2-P0.
	startTan := ab
	if degAB {
		startTan = bc
		if degBC {
			startTan = cd
		}
	}
	if startTan.LengthSquared() > 1e-14 {
		e.doJoin(startTan)
		e.lastTan = startTan
	}

	// Split at inflection points (Skia SkFindCubicInflections).
	tValues := findCubicInflections(cubic)
	lastT := 0.0
	for _, tVal := range tValues {
		e.cubicStrokeSegment(cubic, radius, lastT, tVal)
		lastT = tVal
	}
	e.cubicStrokeSegment(cubic, radius, lastT, 1.0)

	// Cusp handling: if cubic has a cusp, add a filled circle there.
	// (Skia SkFindCubicCusp + fCusper.addCircle)
	if cusp := findCubicCusp(cubic); cusp > 0 {
		pos, _ := evalCubicAtF64(cubic, cusp)
		e.addCuspCircle(pos, radius)
	}

	// Update end state for next segment.
	endTan := cd
	if degCD {
		endTan = Vec2{cubic[3].X - cubic[1].X, cubic[3].Y - cubic[1].Y}
		if endTan.LengthSquared() < 1e-14 {
			endTan = Vec2{cubic[3].X - cubic[0].X, cubic[3].Y - cubic[0].Y}
		}
	}
	e.lastPt = end
	e.lastTan = endTan

	// Update lastNorm for end cap.
	if endTan.LengthSquared() > 1e-14 {
		scale := radius / endTan.Length()
		e.lastNorm = endTan.Perp().Scale(scale)
	}
	e.prevIsLine = false
}

// cubicStrokeSegment strokes a portion of a cubic [tStart, tEnd] on both sides.
// Called once per inflection-free segment.
func (e *StrokeExpander) cubicStrokeSegment(cubic [4]Point, radius, tStart, tEnd float64) {
	// Reset foundTangents before outer+inner pair (Skia pattern: persists between sides)
	e.foundTangents = false

	// Outer side → forward path
	var qc quadConstruct
	e.initQuadConstruct(&qc, strokeOuter, tStart, tEnd)
	e.cubicStroke(cubic, radius, strokeOuter, &qc, &e.forward)

	// Inner side → backward path
	e.initQuadConstruct(&qc, strokeInner, tStart, tEnd)
	e.cubicStroke(cubic, radius, strokeInner, &qc, &e.backward)
}

// strokeSide indicates which side of the curve is being stroked.
type strokeSide int

const (
	strokeOuter strokeSide = 1  // outer (convex) side
	strokeInner strokeSide = -1 // inner (concave) side
)

// quadConstruct holds the state of a quad stroke approximation under construction.
// Direct port of Skia SkQuadConstruct (SkStroke.cpp:130-168).
type quadConstruct struct {
	quad             [3]Point // the stroked quad parallel to the original curve
	tangentStart     Vec2     // tangent at quad[0]
	tangentEnd       Vec2     // tangent at quad[2]
	startT           float64  // parameter range on original curve
	midT             float64
	endT             float64
	startSet, endSet bool // endpoint caching flags
	oppositeTangents bool // coincident tangents with opposite directions
}

// init resets the quadConstruct for a parameter range.
// Returns false if start and end are too close to have a unique midpoint.
func (qc *quadConstruct) init(start, end float64) bool {
	qc.startT = start
	qc.midT = (start + end) * 0.5
	qc.endT = end
	qc.startSet = false
	qc.endSet = false
	return qc.startT < qc.midT && qc.midT < qc.endT
}

// initWithStart creates a child for the first half of parent's parameter range.
func (qc *quadConstruct) initWithStart(parent *quadConstruct) bool {
	if !qc.init(parent.startT, parent.midT) {
		return false
	}
	qc.quad[0] = parent.quad[0]
	qc.tangentStart = parent.tangentStart
	qc.startSet = true
	return true
}

// initWithEnd creates a child for the second half of parent's parameter range.
func (qc *quadConstruct) initWithEnd(parent *quadConstruct) bool {
	if !qc.init(parent.midT, parent.endT) {
		return false
	}
	qc.quad[2] = parent.quad[2]
	qc.tangentEnd = parent.tangentEnd
	qc.endSet = true
	return true
}

// initQuadConstruct initializes a quadConstruct for a given side and parameter range.
func (e *StrokeExpander) initQuadConstruct(qc *quadConstruct, side strokeSide, tStart, tEnd float64) {
	qc.init(tStart, tEnd)
	_ = side // side is used in cubicPerpRay calls
}

// resultType is the outcome of a stroke approximation check.
type resultType int

const (
	resultSplit      resultType = iota // caller should subdivide
	resultDegenerate                   // caller should add a line
	resultQuad                         // caller should add a quad
)

// cubicStroke recursively approximates the offset curve of a cubic with quads.
// This is a direct port of Skia's SkPathStroker::cubicStroke() (SkStroke.cpp:1177-1243).
//
// Two-phase approach:
//
//	Phase 1 (fFoundTangents=false): Use tangentsMeet() — cheap check whether the
//	  tangent rays at start/end converge. Recursion limit = 15.
//	Phase 2 (fFoundTangents=true): Use compareQuadCubic() — full quality check:
//	  compute quad control point, project ray from curve midpoint, measure error.
//	  Recursion limit = 24.
func (e *StrokeExpander) cubicStroke(
	cubic [4]Point, radius float64, side strokeSide,
	qc *quadConstruct, path *pathBuilder,
) {
	// Reset foundTangents per cubic_stroke call (tiny-skia init_quad line 984).
	e.foundTangents = false
	e.cubicStrokeRec(cubic, radius, side, qc, path, false, 0)
}

func (e *StrokeExpander) cubicStrokeRec(
	cubic [4]Point, radius float64, side strokeSide,
	qc *quadConstruct, path *pathBuilder,
	foundTangents bool, depth int,
) {
	// Phase 1: tangentsMeet — cheap convergence check.
	foundTangents = e.cubicStrokePhase1(cubic, radius, side, qc, path, foundTangents)
	if foundTangents {
		// Phase 2: compareQuadCubic — full quality check.
		if e.cubicStrokePhase2(cubic, radius, side, qc, path) {
			return
		}
	}

	// Bail on non-finite coordinates.
	if !isFinitePoint(qc.quad[2]) {
		return
	}

	// Recursion limit: 15 for phase 1, 24 for phase 2.
	limit := 15
	if foundTangents {
		limit = 24
	}
	if depth >= limit {
		e.addDegenerateLine(qc, path)
		return
	}

	// Subdivide: first half, then second half.
	var half quadConstruct
	if !half.initWithStart(qc) {
		e.addDegenerateLine(qc, path)
		return
	}
	e.cubicStrokeRec(cubic, radius, side, &half, path, foundTangents, depth+1)

	if !half.initWithEnd(qc) {
		e.addDegenerateLine(qc, path)
		return
	}
	e.cubicStrokeRec(cubic, radius, side, &half, path, foundTangents, depth+1)
}

// cubicStrokePhase1 checks if tangent rays converge. Returns updated foundTangents.
// If the segment is degenerate and mid-on-line, emits a line and returns false
// (which causes cubicStrokeRec to fall through to subdivision).
func (e *StrokeExpander) cubicStrokePhase1(
	cubic [4]Point, radius float64, side strokeSide,
	qc *quadConstruct, path *pathBuilder, foundTangents bool,
) bool {
	if foundTangents {
		return true
	}
	rt := e.tangentsMeet(cubic, radius, side, qc)
	if rt == resultQuad {
		e.foundTangents = true
		return true // tangents meet, proceed to phase 2
	}
	isDegen := rt == resultDegenerate || pointsWithinDist(qc.quad[0], qc.quad[2], e.invResScale())
	if isDegen && e.cubicMidOnLine(cubic, radius, side, qc) {
		e.addDegenerateLine(qc, path)
	}
	return false
}

// cubicStrokePhase2 performs the full quality check. Returns true if the quad
// was accepted (emitted as quadTo or degenerate line), false if subdivision needed.
func (e *StrokeExpander) cubicStrokePhase2(
	cubic [4]Point, radius float64, side strokeSide,
	qc *quadConstruct, path *pathBuilder,
) bool {
	rt := e.compareQuadCubic(cubic, radius, side, qc)
	switch rt {
	case resultQuad:
		path.quadTo(qc.quad[1], qc.quad[2])
		return true
	case resultDegenerate:
		if !qc.oppositeTangents {
			e.addDegenerateLine(qc, path)
			return true
		}
	}
	return false
}

// isFinitePoint returns true if both coordinates are finite (not NaN or Inf).
func isFinitePoint(p Point) bool {
	return !math.IsNaN(p.X) && !math.IsNaN(p.Y) &&
		!math.IsInf(p.X, 0) && !math.IsInf(p.Y, 0)
}

// invResScale returns Skia's invResScale = 1 / (resScale * 4).
// At default resScale=1, this is 0.25.
func (e *StrokeExpander) invResScale() float64 {
	tol := e.tolerance
	if tol <= 0 {
		tol = 0.25
	}
	return tol
}

// invResScaleSquared returns invResScale^2 for distance comparisons.
func (e *StrokeExpander) invResScaleSquared() float64 {
	irs := e.invResScale()
	return irs * irs
}

// addDegenerateLine emits a line to the endpoint of the quad construct.
func (e *StrokeExpander) addDegenerateLine(qc *quadConstruct, path *pathBuilder) {
	path.lineTo(qc.quad[2])
}

// cubicPerpRayOnPt computes the offset point and tangent at parameter t on a cubic.
// Direct port of Skia SkPathStroker::cubicPerpRay() (SkStroke.cpp:875-901).
func cubicPerpRayOnPt(cubic [4]Point, t, radius float64, side strokeSide) (Point, Vec2) {
	tPt, dxy := evalCubicAtF64(cubic, t)

	if dxy.LengthSquared() < 1e-14 {
		dxy = cubicDegenerateTangent(cubic, t)
	}

	// Normalize dxy to radius length.
	length := math.Hypot(dxy.X, dxy.Y)
	if length > 0 {
		dxy.X = dxy.X / length * radius
		dxy.Y = dxy.Y / length * radius
	} else {
		dxy = Vec2{radius, 0}
	}

	// Perpendicular: rotate 90deg CCW, apply side sign.
	// Skia: onPt = tPt + axisFlip * dxy.perp()
	// axisFlip = +1 for outer, -1 for inner
	s := float64(side)
	onPt := Point{tPt.X + s*dxy.Y, tPt.Y - s*dxy.X}
	return onPt, dxy
}

// cubicDegenerateTangent computes a tangent direction for a cubic with a zero
// derivative at parameter t. Extracted from cubicPerpRayOnPt to reduce nesting.
func cubicDegenerateTangent(cubic [4]Point, t float64) Vec2 {
	var dxy Vec2
	switch {
	case t < 1e-7:
		// Near start: try P2-P0
		dxy = Vec2{cubic[2].X - cubic[0].X, cubic[2].Y - cubic[0].Y}
	case t > 1.0-1e-7:
		// Near end: try P3-P1
		dxy = Vec2{cubic[3].X - cubic[1].X, cubic[3].Y - cubic[1].Y}
	default:
		// At inflection/cusp: chop at t, use chopped tangent
		chopped := chopCubicAt(cubic, t)
		dxy = Vec2{chopped[3].X - chopped[2].X, chopped[3].Y - chopped[2].Y}
		if dxy.LengthSquared() < 1e-14 {
			dxy = Vec2{chopped[3].X - chopped[1].X, chopped[3].Y - chopped[1].Y}
		}
	}
	if dxy.LengthSquared() < 1e-14 {
		dxy = Vec2{cubic[3].X - cubic[0].X, cubic[3].Y - cubic[0].Y}
	}
	return dxy
}

// cubicQuadEnds fills in the start and end points of the quad construct.
// Port of Skia SkPathStroker::cubicQuadEnds() (SkStroke.cpp:904-917).
func (e *StrokeExpander) cubicQuadEnds(cubic [4]Point, radius float64, side strokeSide, qc *quadConstruct) {
	if !qc.startSet {
		qc.quad[0], qc.tangentStart = cubicPerpRayOnPt(cubic, qc.startT, radius, side)
		// Round to f32 precision to match Skia/tiny-skia convergence behavior.
		// f64 perpendicular vectors produce dot=0 exactly in sharpAngle,
		// while f32 rounding makes dot slightly positive → different subdivision.
		qc.quad[0].X = float64(float32(qc.quad[0].X))
		qc.quad[0].Y = float64(float32(qc.quad[0].Y))
		qc.startSet = true
	}
	if !qc.endSet {
		qc.quad[2], qc.tangentEnd = cubicPerpRayOnPt(cubic, qc.endT, radius, side)
		qc.quad[2].X = float64(float32(qc.quad[2].X))
		qc.quad[2].Y = float64(float32(qc.quad[2].Y))
		qc.endSet = true
	}
}

// tangentsMeet checks whether the tangent rays at start/end of the quad converge.
// Phase 1 of Skia's two-phase approach. Returns resultQuad when tangents do meet.
// Port of Skia SkPathStroker::tangentsMeet() (SkStroke.cpp:993-997).
func (e *StrokeExpander) tangentsMeet(cubic [4]Point, radius float64, side strokeSide, qc *quadConstruct) resultType {
	e.cubicQuadEnds(cubic, radius, side, qc)
	return e.intersectRay(qc, false)
}

// intersectRay finds the intersection of the stroke tangent rays to construct a quad control point.
// Returns resultDegenerate (line), resultQuad (ok), or resultSplit (subdivide).
//
// When computeCtrlPt is true, the quad control point is computed and stored in qc.quad[1].
//
// Port of Skia SkPathStroker::intersectRay() (SkStroke.cpp:939-990).
func (e *StrokeExpander) intersectRay(qc *quadConstruct, computeCtrlPt bool) resultType {
	start := qc.quad[0]
	end := qc.quad[2]
	aLen := qc.tangentStart
	bLen := qc.tangentEnd

	denom := aLen.Cross(bLen)
	if denom == 0 || math.IsNaN(denom) || math.IsInf(denom, 0) {
		qc.oppositeTangents = aLen.Dot(bLen) < 0
		return resultDegenerate
	}
	qc.oppositeTangents = false

	ab0 := start.Sub(end)
	numerA := bLen.Cross(ab0)
	numerB := aLen.Cross(ab0)

	if (numerA >= 0) == (numerB >= 0) {
		// Control point outside quad ends. Check if endpoints are close enough
		// to the opposite tangent line for a straight line approximation.
		dist1 := ptToTangentLine(start, end, qc.tangentEnd)
		dist2 := ptToTangentLine(end, start, qc.tangentStart)
		if math.Max(dist1, dist2) <= e.invResScaleSquared() {
			return resultDegenerate
		}
		return resultSplit
	}

	// Check for numerically unstable division.
	numerA /= denom
	if !(numerA > numerA-1) { // catches NaN and very large values
		qc.oppositeTangents = aLen.Dot(bLen) < 0
		return resultDegenerate
	}

	if computeCtrlPt {
		qc.quad[1] = start.Add(qc.tangentStart.Scale(numerA))
	}
	return resultQuad
}

// compareQuadCubic determines whether a quad approximation is close enough to the
// offset curve. Full quality check (Phase 2).
// Port of Skia SkPathStroker::compareQuadCubic() (SkStroke.cpp:1105-1119).
func (e *StrokeExpander) compareQuadCubic(cubic [4]Point, radius float64, side strokeSide, qc *quadConstruct) resultType {
	e.cubicQuadEnds(cubic, radius, side, qc)
	rt := e.intersectRay(qc, true)
	if rt != resultQuad {
		return rt
	}

	// Project a ray from the offset curve midpoint through the cubic midpoint.
	// Skia: cubicPerpRay(cubic, midT, &ray[1], &ray[0], nullptr)
	// ray[0] = offset point (onPt), ray[1] = point on cubic (tPt)
	var ray [2]Point
	tPt, dxy := evalCubicAtF64(cubic, qc.midT)
	// Handle degenerate derivative (same as cubicPerpRayOnPt)
	if dxy.LengthSquared() < 1e-14 {
		dxy = Vec2{cubic[3].X - cubic[0].X, cubic[3].Y - cubic[0].Y}
	}
	length := math.Hypot(dxy.X, dxy.Y)
	if length > 0 {
		dxy.X = dxy.X / length * radius
		dxy.Y = dxy.Y / length * radius
	} else {
		dxy = Vec2{radius, 0}
	}
	s := float64(side)
	ray[0] = Point{tPt.X + s*dxy.Y, tPt.Y - s*dxy.X} // offset point
	ray[1] = tPt                                     // point on cubic

	return e.strokeCloseEnough(qc.quad, ray, qc)
}

// strokeCloseEnough checks whether a quad stroke approximation is close enough.
// Port of Skia SkPathStroker::strokeCloseEnough() (SkStroke.cpp:1056-1103).
//
// ray[0] = offset point (on the approximated stroke curve)
// ray[1] = point on the original cubic (for ray direction)
func (e *StrokeExpander) strokeCloseEnough(stroke [3]Point, ray [2]Point, qc *quadConstruct) resultType {
	irs := e.invResScale()

	// Evaluate quad at t=0.5
	strokeMid := evalQuadPoint(stroke[0], stroke[1], stroke[2], 0.5)

	// Quick check: is the offset point close to the quad midpoint?
	if pointsWithinDist(ray[0], strokeMid, irs) {
		if sharpAngle(qc.quad) {
			return resultSplit
		}
		return resultQuad
	}

	// Quick reject: is the offset point even within the quad bounds?
	if !ptInQuadBounds(stroke, ray[0], irs) {
		return resultSplit
	}

	// Full check: intersect the ray line (from offset to cubic point) with the quad.
	// Skia uses the line from ray[0] (offset) to ray[1] (cubic) to find where
	// the perpendicular ray crosses the quad stroke.
	rootCount, root := intersectQuadRayLine(ray, stroke)
	if rootCount != 1 {
		return resultSplit
	}

	quadPt := evalQuadPoint(stroke[0], stroke[1], stroke[2], root)
	// Adaptive tolerance: tighter near t=0.5, relaxed at endpoints.
	// Skia: error = invResScale * (1 - |root - 0.5| * 2)
	adaptiveError := irs * (1.0 - math.Abs(root-0.5)*2.0)
	if pointsWithinDist(ray[0], quadPt, adaptiveError) {
		if sharpAngle(qc.quad) {
			return resultSplit
		}
		return resultQuad
	}
	return resultSplit
}

// cubicMidOnLine checks whether the offset curve midpoint lies on the line
// connecting the quad endpoints. Used for degenerate segments.
// Port of Skia SkPathStroker::cubicMidOnLine() (SkStroke.cpp:1170-1175).
func (e *StrokeExpander) cubicMidOnLine(cubic [4]Point, radius float64, side strokeSide, qc *quadConstruct) bool {
	strokeMid, _ := cubicPerpRayOnPt(cubic, qc.midT, radius, side)
	dist := ptToLine(strokeMid, qc.quad[0], qc.quad[2])
	return dist < e.invResScaleSquared()
}

// --- Geometry helpers ported from Skia ---

// pointsWithinDist checks if two points are within the given distance.
func pointsWithinDist(a, b Point, limit float64) bool {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return dx*dx+dy*dy <= limit*limit
}

// ptToLine returns the squared distance from pt to the line segment (lineStart, lineEnd).
// Port of Skia pt_to_line() (SkStroke.cpp:551-563).
func ptToLine(pt, lineStart, lineEnd Point) float64 {
	dxy := lineEnd.Sub(lineStart)
	ab0 := pt.Sub(lineStart)
	numer := dxy.Dot(ab0)
	denom := dxy.Dot(dxy)
	if denom < 1e-14 {
		dx := pt.X - lineStart.X
		dy := pt.Y - lineStart.Y
		return dx*dx + dy*dy
	}
	t := numer / denom
	if t >= 0 && t <= 1 {
		hit := Point{
			X: lineStart.X*(1-t) + lineEnd.X*t,
			Y: lineStart.Y*(1-t) + lineEnd.Y*t,
		}
		dx := hit.X - pt.X
		dy := hit.Y - pt.Y
		return dx*dx + dy*dy
	}
	dx := pt.X - lineStart.X
	dy := pt.Y - lineStart.Y
	return dx*dx + dy*dy
}

// ptToTangentLine returns the squared distance from pt to a tangent line
// starting at lineStart in the direction of tangent.
// Port of Skia pt_to_tangent_line() (SkStroke.cpp:566-580).
func ptToTangentLine(pt, lineStart Point, tangent Vec2) float64 {
	ab0 := pt.Sub(lineStart)
	numer := tangent.Dot(ab0)
	denom := tangent.Dot(tangent)
	if denom < 1e-14 {
		dx := pt.X - lineStart.X
		dy := pt.Y - lineStart.Y
		return dx*dx + dy*dy
	}
	t := numer / denom
	if t >= 0 && t <= 1 {
		hit := Point{
			X: lineStart.X + tangent.X*t,
			Y: lineStart.Y + tangent.Y*t,
		}
		dx := hit.X - pt.X
		dy := hit.Y - pt.Y
		return dx*dx + dy*dy
	}
	dx := pt.X - lineStart.X
	dy := pt.Y - lineStart.Y
	return dx*dx + dy*dy
}

// sharpAngle returns true if the quad has a sharp angle at the control point.
// Port of Skia sharp_angle() (SkStroke.cpp:1039-1054).
//
// Uses float32 arithmetic internally to match Skia/tiny-skia behavior.
// In float64, perpendicular vectors produce dot=0 exactly. In float32,
// cubicPerpRay endpoint rounding makes dot slightly positive at ~90° angles,
// causing subdivision. This precision-matching is required for Skia golden parity.
func sharpAngle(quad [3]Point) bool {
	// Compute in float32 to match Skia/tiny-skia precision
	sx := float32(quad[1].X - quad[0].X)
	sy := float32(quad[1].Y - quad[0].Y)
	lx := float32(quad[1].X - quad[2].X)
	ly := float32(quad[1].Y - quad[2].Y)

	sLen := sx*sx + sy*sy
	lLen := lx*lx + ly*ly
	if sLen > lLen {
		sx, lx = lx, sx
		sy, ly = ly, sy
		lLen = sLen
	}

	// set_length(lLen): normalize then scale by lLen (squared length, not sqrt)
	l := float32(math.Sqrt(float64(sx*sx + sy*sy)))
	if l < 1e-7 {
		return false
	}
	nx := sx / l * lLen
	ny := sy / l * lLen
	dot := nx*lx + ny*ly
	// Use >= 0 instead of > 0: perpendicular quads (dot=0, 90° angle) ARE
	// geometrically sharp for inner stroke corners. Skia/tiny-skia's f32
	// rounding makes dot slightly positive at 90°, achieving this accidentally.
	// Using >= 0 is mathematically correct AND matches Skia visual output.
	return dot >= 0
}

// ptInQuadBounds checks if a point is within the bounding box of a quad,
// expanded by invResScale tolerance.
// Port of Skia SkPathStroker::ptInQuadBounds() (SkStroke.cpp:1015-1033).
func ptInQuadBounds(quad [3]Point, pt Point, invResScale float64) bool {
	xMin := math.Min(math.Min(quad[0].X, quad[1].X), quad[2].X)
	if pt.X+invResScale < xMin {
		return false
	}
	xMax := math.Max(math.Max(quad[0].X, quad[1].X), quad[2].X)
	if pt.X-invResScale > xMax {
		return false
	}
	yMin := math.Min(math.Min(quad[0].Y, quad[1].Y), quad[2].Y)
	if pt.Y+invResScale < yMin {
		return false
	}
	yMax := math.Max(math.Max(quad[0].Y, quad[1].Y), quad[2].Y)
	return pt.Y-invResScale <= yMax
}

// intersectQuadRayLine intersects a line (line[0] → line[1]) with a quad curve.
// Returns the number of roots found (0, 1, or 2) and the single root value.
// Port of Skia intersect_quad_ray() (SkStroke.cpp:1000-1012).
func intersectQuadRayLine(line [2]Point, quad [3]Point) (count int, root float64) {
	vec := Vec2{line[1].X - line[0].X, line[1].Y - line[0].Y}
	var r [3]float64
	for i := 0; i < 3; i++ {
		qi := Vec2{quad[i].X - line[0].X, quad[i].Y - line[0].Y}
		r[i] = vec.Cross(qi)
	}
	a := r[2]
	b := r[1]
	c := r[0]
	a += c - 2*b // a - 2b + c
	b -= c       // -(b - c)

	roots := findUnitQuadRoots(a, 2*b, c)
	if len(roots) == 1 {
		return 1, roots[0]
	}
	return len(roots), 0
}

// findUnitQuadRoots finds the real roots of At^2 + Bt + C = 0 in [0, 1].
// Port of Skia SkFindUnitQuadRoots() (SkGeometry.cpp:95-127).
func findUnitQuadRoots(a, b, c float64) []float64 {
	if a == 0 {
		if b == 0 {
			return nil
		}
		r := -c / b
		if r > 0 && r < 1 {
			return []float64{r}
		}
		return nil
	}

	dr := b*b - 4*a*c
	if dr < 0 {
		return nil
	}
	dr = math.Sqrt(dr)

	var q float64
	if b < 0 {
		q = -(b - dr) / 2
	} else {
		q = -(b + dr) / 2
	}

	var roots []float64
	r1 := q / a
	if r1 > 0 && r1 < 1 {
		roots = append(roots, r1)
	}
	r2 := c / q
	if r2 > 0 && r2 < 1 && r2 != r1 {
		roots = append(roots, r2)
	}
	if len(roots) == 2 && roots[0] > roots[1] {
		roots[0], roots[1] = roots[1], roots[0]
	}
	return roots
}

// findCubicInflections finds the inflection points of a cubic.
// Port of Skia SkFindCubicInflections() (SkGeometry.cpp:741-753).
// Returns t values in (0, 1) sorted ascending.
func findCubicInflections(cubic [4]Point) []float64 {
	ax := cubic[1].X - cubic[0].X
	ay := cubic[1].Y - cubic[0].Y
	bx := cubic[2].X - 2*cubic[1].X + cubic[0].X
	by := cubic[2].Y - 2*cubic[1].Y + cubic[0].Y
	cx := cubic[3].X + 3*(cubic[1].X-cubic[2].X) - cubic[0].X
	cy := cubic[3].Y + 3*(cubic[1].Y-cubic[2].Y) - cubic[0].Y

	return findUnitQuadRoots(
		bx*cy-by*cx,
		ax*cy-ay*cx,
		ax*by-ay*bx,
	)
}

// findCubicCusp finds a cusp point on a cubic, or returns -1 if none.
// Port of Skia SkFindCubicCusp() (SkGeometry.cpp:1112-1148).
func findCubicCusp(cubic [4]Point) float64 {
	// Skip when adjacent control point matches end point.
	if cubic[0] == cubic[1] || cubic[2] == cubic[3] {
		return -1
	}

	// Check if line segments from control/end points cross.
	if onSameSide(cubic, 0, 2) || onSameSide(cubic, 2, 0) {
		return -1
	}

	// Find max curvature points and check for zero derivative.
	maxCurvatures := findCubicMaxCurvature(cubic)
	precision := calcCubicPrecision(cubic)
	for _, testT := range maxCurvatures {
		if testT <= 0 || testT >= 1 {
			continue
		}
		_, dPt := evalCubicAtF64(cubic, testT)
		dPtMag := dPt.LengthSquared()
		if dPtMag < precision {
			return testT
		}
	}
	return -1
}

// onSameSide checks if cubic[offset] and cubic[offset+1] are on the same side
// of the line formed by cubic[otherOffset] and cubic[otherOffset+1].
func onSameSide(cubic [4]Point, offset, otherOffset int) bool {
	p0 := cubic[otherOffset]
	p1 := cubic[otherOffset+1]
	line := p1.Sub(p0)
	a := cubic[offset].Sub(p0)
	b := cubic[offset+1].Sub(p0)
	crossA := line.Cross(a)
	crossB := line.Cross(b)
	return crossA*crossB >= 0
}

// findCubicMaxCurvature finds t values where curvature is maximized.
// Simplified: returns roots of the derivative of curvature numerator.
func findCubicMaxCurvature(cubic [4]Point) []float64 {
	// Coefficients of the cubic's derivative: 3[(P1-P0) + 2t(P2-2P1+P0) + t^2(P3-3P2+3P1-P0)]
	// The curvature numerator is |B'(t) x B''(t)|.
	// B'(t) = a + bt + ct^2 where:
	ax := cubic[1].X - cubic[0].X
	ay := cubic[1].Y - cubic[0].Y
	bx := cubic[2].X - 2*cubic[1].X + cubic[0].X
	by := cubic[2].Y - 2*cubic[1].Y + cubic[0].Y
	cx := cubic[3].X - 3*cubic[2].X + 3*cubic[1].X - cubic[0].X
	cy := cubic[3].Y - 3*cubic[2].Y + 3*cubic[1].Y - cubic[0].Y

	// B''(t) = 2b + 2ct (divided by 3 but doesn't affect roots)
	// Derivative of |B' x B''| leads to a degree-3 polynomial.
	// For simplicity, sample at several points and find local maxima.
	// This matches Skia's SkFindCubicMaxCurvature which solves a cubic.
	// We use the analytical approach: the curvature numerator cross product
	// B'xB'' = (a+bt+ct^2) x (2b+2ct) = ... yields a polynomial in t.
	// d/dt(B'xB'') = 0 → quadratic equation.

	// B' x B'' = 2(axby - aybx) + 2t(axcy - aycx) + 2t^2(bxcy - bycx)
	// This is 2(A + Bt + Ct^2) where:
	//   A = axby - aybx
	//   B = axcy - aycx
	//   C = bxcy - bycx
	// d/dt = 2(B + 2Ct) = 0 → t = -B/(2C)
	// But for max curvature we need d/dt(|B'xB''| / |B'|^3) which is more complex.
	// Skia uses the full approach. For our purposes, we use a simplified version.

	_ = ax*by - ay*bx // bigA — not needed for root finding, only for sign
	bigB := ax*cy - ay*cx
	bigC := bx*cy - by*cx

	// d/dt(cross) = B + 2Ct = 0
	var results []float64
	if math.Abs(bigC) > 1e-14 {
		t := -bigB / (2 * bigC)
		if t > 0 && t < 1 {
			results = append(results, t)
		}
	}

	// Also check where |B'| is minimized (could indicate cusp).
	// d/dt(|B'|^2) = 2(B'.B'') = 0
	// B'.B'' = (a+bt+ct^2).(2b+2ct)
	//        = 2(a.b + t(a.c+b.b) + t^2(b.c) + ... )
	// This gives another quadratic. For cusps, the derivative magnitude minimum
	// is more reliable.
	dotAB := ax*bx + ay*by
	dotAC := ax*cx + ay*cy
	dotBB := bx*bx + by*by
	dotBC := bx*cx + by*cy
	dotCC := cx*cx + cy*cy

	// d/dt(|B'|^2) = 2[(a.b) + t(2*b.b + a.c) + t^2(3*b.c) + t^3(2*c.c)]
	// We solve the cubic 2c.c*t^3 + 3b.c*t^2 + (2b.b+a.c)*t + a.b = 0
	// For simplicity, use findCubicRealRoots for the full cubic.
	cubicRoots := solveCubicForMaxCurvature(
		2*dotCC, 3*dotBC, 2*dotBB+dotAC, dotAB,
	)
	for _, r := range cubicRoots {
		if r > 0 && r < 1 {
			results = append(results, r)
		}
	}

	return results
}

// solveCubicForMaxCurvature finds real roots of at^3 + bt^2 + ct + d = 0 in (0,1).
func solveCubicForMaxCurvature(a, b, c, d float64) []float64 {
	if math.Abs(a) < 1e-14 {
		// Degenerate to quadratic
		return findUnitQuadRoots(b, c, d)
	}

	// Normalize
	b /= a
	c /= a
	d /= a

	p := c - b*b/3
	q := 2*b*b*b/27 - b*c/3 + d
	disc := q*q/4 + p*p*p/27

	if disc > 0 {
		return solveCubicOneRoot(q, disc, b)
	}
	return solveCubicThreeRoots(p, q, b)
}

// solveCubicOneRoot handles the case where the depressed cubic has one real root.
func solveCubicOneRoot(q, disc, b float64) []float64 {
	sqrtDisc := math.Sqrt(disc)
	u := math.Cbrt(-q/2 + sqrtDisc)
	v := math.Cbrt(-q/2 - sqrtDisc)
	t := u + v - b/3
	if t > 0 && t < 1 {
		return []float64{t}
	}
	return nil
}

// solveCubicThreeRoots handles the case where the depressed cubic has three real roots.
func solveCubicThreeRoots(p, q, b float64) []float64 {
	r := math.Sqrt(-p * p * p / 27)
	if r < 1e-14 {
		return nil
	}
	theta := math.Acos(math.Max(-1, math.Min(1, -q/(2*r))))
	m := 2 * math.Cbrt(r)
	var roots []float64
	for k := 0; k < 3; k++ {
		t := m*math.Cos((theta+2*math.Pi*float64(k))/3) - b/3
		if t > 0 && t < 1 {
			roots = append(roots, t)
		}
	}
	return roots
}

// calcCubicPrecision computes a precision threshold for cusp detection.
// Port of Skia calc_cubic_precision() (SkGeometry.cpp).
func calcCubicPrecision(cubic [4]Point) float64 {
	maxX := math.Max(math.Max(math.Abs(cubic[0].X), math.Abs(cubic[1].X)),
		math.Max(math.Abs(cubic[2].X), math.Abs(cubic[3].X)))
	maxY := math.Max(math.Max(math.Abs(cubic[0].Y), math.Abs(cubic[1].Y)),
		math.Max(math.Abs(cubic[2].Y), math.Abs(cubic[3].Y)))
	maxCoord := math.Max(maxX, maxY)
	// Skia uses a relative precision; for float64 we use a tighter value.
	return maxCoord * maxCoord * 1e-8
}

// chopCubicAt splits a cubic at parameter t using de Casteljau's algorithm.
// Returns [7]Point: first cubic [0..3], second cubic [3..6].
func chopCubicAt(cubic [4]Point, t float64) [7]Point {
	s := 1.0 - t
	ab := Point{cubic[0].X*s + cubic[1].X*t, cubic[0].Y*s + cubic[1].Y*t}
	bc := Point{cubic[1].X*s + cubic[2].X*t, cubic[1].Y*s + cubic[2].Y*t}
	cd := Point{cubic[2].X*s + cubic[3].X*t, cubic[2].Y*s + cubic[3].Y*t}
	abc := Point{ab.X*s + bc.X*t, ab.Y*s + bc.Y*t}
	bcd := Point{bc.X*s + cd.X*t, bc.Y*s + cd.Y*t}
	abcd := Point{abc.X*s + bcd.X*t, abc.Y*s + bcd.Y*t}
	return [7]Point{cubic[0], ab, abc, abcd, bcd, cd, cubic[3]}
}

// addCuspCircle adds a filled circle at a cusp point.
// This matches Skia's fCusper.addCircle() pattern.
func (e *StrokeExpander) addCuspCircle(center Point, radius float64) {
	// Approximate a full circle with 4 cubic arcs (standard kappa=0.5522847498).
	const k = 0.5522847498
	kr := k * radius

	// Start at right (center + radius, 0), go counter-clockwise.
	p0 := Point{center.X + radius, center.Y}
	e.output.moveTo(p0)
	e.output.cubicTo(
		Point{center.X + radius, center.Y + kr},
		Point{center.X + kr, center.Y + radius},
		Point{center.X, center.Y + radius},
	)
	e.output.cubicTo(
		Point{center.X - kr, center.Y + radius},
		Point{center.X - radius, center.Y + kr},
		Point{center.X - radius, center.Y},
	)
	e.output.cubicTo(
		Point{center.X - radius, center.Y - kr},
		Point{center.X - kr, center.Y - radius},
		Point{center.X, center.Y - radius},
	)
	e.output.cubicTo(
		Point{center.X + kr, center.Y - radius},
		Point{center.X + radius, center.Y - kr},
		p0,
	)
	e.output.close()
}

// cubicDerivativeAt returns the derivative of a cubic at parameter t.
func cubicDerivativeAt(pts [4]Point, t float64) Vec2 {
	s := 1.0 - t
	s2 := s * s
	t2 := t * t
	return Vec2{
		X: 3 * (s2*(pts[1].X-pts[0].X) + 2*s*t*(pts[2].X-pts[1].X) + t2*(pts[3].X-pts[2].X)),
		Y: 3 * (s2*(pts[1].Y-pts[0].Y) + 2*s*t*(pts[2].Y-pts[1].Y) + t2*(pts[3].Y-pts[2].Y)),
	}
}

// evalCubicAtF64 evaluates a cubic bezier at parameter t (float64).
func evalCubicAtF64(pts [4]Point, t float64) (Point, Vec2) {
	s := 1.0 - t
	s2, s3 := s*s, s*s*s
	t2, t3 := t*t, t*t*t
	pos := Point{
		X: s3*pts[0].X + 3*s2*t*pts[1].X + 3*s*t2*pts[2].X + t3*pts[3].X,
		Y: s3*pts[0].Y + 3*s2*t*pts[1].Y + 3*s*t2*pts[2].Y + t3*pts[3].Y,
	}
	deriv := Vec2{
		X: 3 * (s2*(pts[1].X-pts[0].X) + 2*s*t*(pts[2].X-pts[1].X) + t2*(pts[3].X-pts[2].X)),
		Y: 3 * (s2*(pts[1].Y-pts[0].Y) + 2*s*t*(pts[2].Y-pts[1].Y) + t2*(pts[3].Y-pts[2].Y)),
	}
	return pos, deriv
}

// evalQuadPoint evaluates a quadratic bezier at parameter t.
func evalQuadPoint(p0, p1, p2 Point, t float64) Point {
	s := 1.0 - t
	return Point{
		X: s*s*p0.X + 2*s*t*p1.X + t*t*p2.X,
		Y: s*s*p0.Y + 2*s*t*p1.Y + t*t*p2.Y,
	}
}

// finish completes an open subpath with end caps.
func (e *StrokeExpander) finish() {
	if e.forward.isEmpty() {
		return
	}

	// Copy forward path to output
	e.output.appendPath(&e.forward)

	// Apply end cap using saved normal from last line segment.
	// This follows the tiny-skia pattern: use prev_normal instead of
	// computing from points, which would give incorrect cap direction.
	// Note: lastNorm points toward backward path, but applyCap expects
	// the normal pointing toward forward path (from where we're drawing),
	// so we negate it.
	if len(e.backward.verbs) > 0 {
		e.applyCap(e.style.Cap, e.lastPt, e.lastNorm.Neg(), false)
	}

	// Append reversed backward path
	e.appendReversed(&e.backward)

	// Apply start cap and close
	e.applyCap(e.style.Cap, e.startPt, e.startNorm, true)

	// Clear for next subpath (truncate, keep backing arrays)
	e.forward.reset()
	e.backward.reset()
}

// finishClosed completes a closed subpath.
func (e *StrokeExpander) finishClosed() {
	if e.forward.isEmpty() {
		return
	}

	// Join back to start
	e.doJoin(e.startTan)

	// Debug: capture before output (remove after golden fix)
	// Copy forward path and close
	e.output.appendPath(&e.forward)
	e.output.close()

	// Handle backward path separately
	if len(e.backward.verbs) > 0 {
		lastPt := e.backward.endPointOfLastVerb()
		e.output.moveTo(lastPt)
	}
	e.appendReversed(&e.backward)
	e.output.close()

	// Clear for next subpath (truncate, keep backing arrays)
	e.forward.reset()
	e.backward.reset()
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
		e.roundCap(center, norm)
		if closePath {
			e.output.close()
		}

	case LineCapSquare:
		e.squareCap(&e.output, center, norm, closePath)
	}
}

// roundCap adds a rounded cap using the output path builder.
func (e *StrokeExpander) roundCap(center Point, norm Vec2) {
	e.roundJoin(&e.output, center, norm, math.Pi)
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
	nv := len(pb.verbs)
	if nv <= 1 {
		return
	}
	// Build coord offsets for each verb
	offsets := make([]int, nv+1)
	off := 0
	for j, v := range pb.verbs {
		offsets[j] = off
		off += verbCoordCount(v)
	}
	offsets[nv] = off

	for i := nv - 1; i >= 1; i-- {
		// endPt = endpoint of verb[i-1]
		prevOff := offsets[i-1]
		prevN := verbCoordCount(pb.verbs[i-1])
		var endPt Point
		if prevN >= 2 {
			endPt = Point{X: pb.coords[prevOff+prevN-2], Y: pb.coords[prevOff+prevN-1]}
		}

		curOff := offsets[i]
		switch pb.verbs[i] {
		case VerbLineTo:
			e.output.lineTo(endPt)
		case VerbQuadTo:
			ctrl := Point{X: pb.coords[curOff], Y: pb.coords[curOff+1]}
			e.output.quadTo(ctrl, endPt)
		case VerbCubicTo:
			// Reverse: swap control1 and control2
			ctrl2 := Point{X: pb.coords[curOff+2], Y: pb.coords[curOff+3]}
			ctrl1 := Point{X: pb.coords[curOff], Y: pb.coords[curOff+1]}
			e.output.cubicTo(ctrl2, ctrl1, endPt)
		}
	}
}

// flattenQuad flattens a quadratic Bezier curve to line segments.
// Uses the reusable flattenBuf to avoid per-curve allocations.
func (e *StrokeExpander) flattenQuad(p0, p1, p2 Point) []Point {
	e.flattenBuf = append(e.flattenBuf[:0], p0)
	e.flattenQuadRec(p0, p1, p2, 0)
	return e.flattenBuf
}

func (e *StrokeExpander) flattenQuadRec(p0, p1, p2 Point, depth int) {
	// Max recursion depth to prevent stack overflow (e.g. NaN coordinates)
	if depth > 10 {
		e.flattenBuf = append(e.flattenBuf, p2)
		return
	}

	// Check if curve is flat enough
	dist := distanceToLine(p1, p0, p2)
	if dist < e.tolerance {
		e.flattenBuf = append(e.flattenBuf, p2)
		return
	}

	// Subdivide
	q0 := p0.Lerp(p1, 0.5)
	q1 := p1.Lerp(p2, 0.5)
	q2 := q0.Lerp(q1, 0.5)

	e.flattenQuadRec(p0, q0, q2, depth+1)
	e.flattenQuadRec(q2, q1, p2, depth+1)
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

// pathBuilder is a helper for building paths using SOA (verb+coords) layout.
type pathBuilder struct {
	verbs   []PathVerb
	coords  []float64
	current Point
}

// reset clears the path builder for reuse, retaining the backing arrays.
func (b *pathBuilder) reset() {
	b.verbs = b.verbs[:0]
	b.coords = b.coords[:0]
	b.current = Point{}
}

func (b *pathBuilder) isEmpty() bool {
	return len(b.verbs) == 0
}

func (b *pathBuilder) moveTo(p Point) {
	b.verbs = append(b.verbs, VerbMoveTo)
	b.coords = append(b.coords, p.X, p.Y)
	b.current = p
}

func (b *pathBuilder) lineTo(p Point) {
	b.verbs = append(b.verbs, VerbLineTo)
	b.coords = append(b.coords, p.X, p.Y)
	b.current = p
}

// setLastPoint replaces the endpoint of the last verb without adding a new verb.
// This is Skia/tiny-skia's pattern for joins when prev_is_line: instead of adding
// a new lineTo (which creates an extra vertex), it adjusts the existing endpoint.
func (b *pathBuilder) setLastPoint(p Point) {
	if len(b.coords) >= 2 {
		b.coords[len(b.coords)-2] = p.X
		b.coords[len(b.coords)-1] = p.Y
		b.current = p
	}
}

func (b *pathBuilder) quadTo(c, p Point) {
	b.verbs = append(b.verbs, VerbQuadTo)
	b.coords = append(b.coords, c.X, c.Y, p.X, p.Y)
	b.current = p
}

func (b *pathBuilder) cubicTo(c1, c2, p Point) {
	b.verbs = append(b.verbs, VerbCubicTo)
	b.coords = append(b.coords, c1.X, c1.Y, c2.X, c2.Y, p.X, p.Y)
	b.current = p
}

func (b *pathBuilder) close() {
	b.verbs = append(b.verbs, VerbClose)
}

func (b *pathBuilder) appendPath(other *pathBuilder) {
	b.verbs = append(b.verbs, other.verbs...)
	b.coords = append(b.coords, other.coords...)
}

// endPointOfLastVerb returns the endpoint of the last verb in the path.
func (b *pathBuilder) endPointOfLastVerb() Point {
	if len(b.verbs) == 0 {
		return Point{}
	}
	lastVerb := b.verbs[len(b.verbs)-1]
	n := verbCoordCount(lastVerb)
	if n >= 2 {
		cl := len(b.coords)
		return Point{X: b.coords[cl-2], Y: b.coords[cl-1]}
	}
	return Point{}
}
