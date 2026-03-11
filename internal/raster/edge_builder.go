// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"iter"
	"math"
	"slices"
)

// PathVerb represents a path construction command.
// Mirrors scene.PathVerb for core package independence.
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

// PathLike is the interface for path-like objects that can be processed by EdgeBuilder.
// This allows core package to work with any path implementation without importing scene.
type PathLike interface {
	// IsEmpty returns true if the path has no commands.
	IsEmpty() bool

	// Verbs returns the verb stream.
	Verbs() []PathVerb

	// Points returns the point data stream (pairs of float32 x,y coordinates).
	Points() []float32
}

// Transform is the interface for affine transformations.
// This allows core package to work with any transform implementation.
type Transform interface {
	// TransformPoint transforms a point (x, y) and returns the result.
	TransformPoint(x, y float32) (float32, float32)
}

// IdentityTransform is a no-op transform that returns points unchanged.
type IdentityTransform struct{}

// TransformPoint returns the point unchanged.
func (IdentityTransform) TransformPoint(x, y float32) (float32, float32) {
	return x, y
}

// Rect represents a bounding rectangle.
type Rect struct {
	MinX, MinY float32
	MaxX, MaxY float32
}

// EmptyRect returns an empty rectangle (inverted bounds for union operations).
func EmptyRect() Rect {
	return Rect{
		MinX: math.MaxFloat32,
		MinY: math.MaxFloat32,
		MaxX: -math.MaxFloat32,
		MaxY: -math.MaxFloat32,
	}
}

// IsEmpty returns true if the rectangle has no area.
func (r Rect) IsEmpty() bool {
	return r.MinX >= r.MaxX || r.MinY >= r.MaxY
}

// EdgeBuilder converts paths to typed edges for analytic anti-aliasing.
//
// By default, EdgeBuilder preserves curve information (QuadraticEdge, CubicEdge)
// which enables higher quality anti-aliasing by evaluating curve coverage analytically.
//
// Alternatively, with flattenCurves=true, all curves are converted to line segments
// at build time. This is simpler and more reliable for the AnalyticFiller.
//
// The builder ensures all edges are Y-monotonic by chopping curves at their
// Y extrema before creating edge objects.
//
// When a clip rect is set via SetClipRect, all edges are clipped to the rect
// before fixed-point conversion. This prevents FDot6→FDot16 integer overflow
// (RAST-010) by ensuring coordinates stay within safe range. Out-of-bounds
// portions are replaced with sentinel vertical lines at clip boundaries to
// preserve correct winding numbers for fill operations.
//
// Usage:
//
//	eb := NewEdgeBuilder(2) // 4x AA quality
//	eb.SetFlattenCurves(true) // Flatten curves to lines
//	eb.SetClipRect(&raster.Rect{MinX: -2, MinY: -2, MaxX: 802, MaxY: 602})
//	eb.BuildFromPath(path, IdentityTransform{})
//
//	for edge := range eb.AllEdges() {
//	    // Process edges sorted by top Y
//	}
//
// Reference: tiny-skia/src/edge_builder.rs
type EdgeBuilder struct {
	// Separate storage for different edge types
	lineEdges      []LineEdge
	quadraticEdges []*QuadraticEdge
	cubicEdges     []*CubicEdge
	velloLines     []VelloLine

	// aaShift controls AA quality (0=none, 2=4x equivalent)
	aaShift int

	// flattenCurves when true converts all curves to line segments
	flattenCurves bool

	// flattenTolerance overrides the default curve flattening tolerance (0.1 px).
	// When > 0, this value is used instead of the hardcoded constant.
	// On HiDPI displays, set to baseTol/deviceScale for finer subdivision.
	flattenTolerance float32

	// clipRect when non-nil clips all edges to this rectangle.
	// Prevents FDot6→FDot16 overflow for coordinates exceeding safe range.
	// Sentinel vertical lines are emitted at X boundaries to preserve winding.
	clipRect *Rect

	// bounds accumulates the bounding box of all edges
	bounds edgeBounds
}

// VelloLine stores a line segment with original float32 coordinates.
// Used by Vello tile rasterizer to avoid fixed-point quantization loss.
type VelloLine struct {
	P0     [2]float32 // Start point (pixel coords, normalized: P0.y <= P1.y)
	P1     [2]float32 // End point (pixel coords)
	IsDown bool       // true if original direction was downward (y0 < y1)
}

// edgeBounds tracks the bounding rectangle during edge building.
type edgeBounds struct {
	minX, minY float32
	maxX, maxY float32
	empty      bool
}

// newEmptyBounds returns an empty bounds (ready for union operations).
func newEmptyBounds() edgeBounds {
	return edgeBounds{
		minX:  math.MaxFloat32,
		minY:  math.MaxFloat32,
		maxX:  -math.MaxFloat32,
		maxY:  -math.MaxFloat32,
		empty: true,
	}
}

// unionPoint expands bounds to include a point.
func (b *edgeBounds) unionPoint(x, y float32) {
	if b.empty {
		b.minX, b.maxX = x, x
		b.minY, b.maxY = y, y
		b.empty = false
		return
	}
	if x < b.minX {
		b.minX = x
	}
	if x > b.maxX {
		b.maxX = x
	}
	if y < b.minY {
		b.minY = y
	}
	if y > b.maxY {
		b.maxY = y
	}
}

// NewEdgeBuilder creates a new edge builder with specified AA quality.
//
// Parameters:
//   - aaShift: anti-aliasing shift (0 = no AA, 2 = 4x AA quality)
//
// Higher shift values provide better AA quality but require more memory
// and computation.
func NewEdgeBuilder(aaShift int) *EdgeBuilder {
	return &EdgeBuilder{
		lineEdges:      make([]LineEdge, 0, 64),
		quadraticEdges: make([]*QuadraticEdge, 0, 16),
		cubicEdges:     make([]*CubicEdge, 0, 16),
		velloLines:     make([]VelloLine, 0, 64),
		aaShift:        aaShift,
		bounds:         newEmptyBounds(),
	}
}

// Reset clears the builder for reuse without deallocating memory.
func (eb *EdgeBuilder) Reset() {
	eb.lineEdges = eb.lineEdges[:0]
	eb.quadraticEdges = eb.quadraticEdges[:0]
	eb.cubicEdges = eb.cubicEdges[:0]
	eb.velloLines = eb.velloLines[:0]
	eb.bounds = newEmptyBounds()
}

// BuildFromPath processes a PathLike and creates typed edges.
//
// This is the main entry point for path processing. It:
//  1. Iterates through path verbs
//  2. Applies transform to all points
//  3. Chops curves at Y extrema for monotonicity
//  4. Creates appropriate edge types
//
// Parameters:
//   - path: the path to process (implements PathLike interface)
//   - transform: transformation to apply to all points
func (eb *EdgeBuilder) BuildFromPath(path PathLike, transform Transform) {
	if path == nil || path.IsEmpty() {
		return
	}

	// State for path traversal
	var curX, curY float32     // Current position
	var startX, startY float32 // Subpath start for Close

	pointIdx := 0
	points := path.Points()
	verbs := path.Verbs()

	for _, verb := range verbs {
		switch verb {
		case VerbMoveTo:
			// Close previous subpath if not at start
			if curX != startX || curY != startY {
				eb.addLine(curX, curY, startX, startY)
			}

			// Transform and update position
			x, y := points[pointIdx], points[pointIdx+1]
			curX, curY = transform.TransformPoint(x, y)
			startX, startY = curX, curY
			pointIdx += 2

		case VerbLineTo:
			x, y := points[pointIdx], points[pointIdx+1]
			nextX, nextY := transform.TransformPoint(x, y)
			eb.addLine(curX, curY, nextX, nextY)
			curX, curY = nextX, nextY
			pointIdx += 2

		case VerbQuadTo:
			// Control point and end point
			cx, cy := points[pointIdx], points[pointIdx+1]
			x, y := points[pointIdx+2], points[pointIdx+3]

			// Transform all points
			tcx, tcy := transform.TransformPoint(cx, cy)
			tx, ty := transform.TransformPoint(x, y)

			eb.addQuad(curX, curY, tcx, tcy, tx, ty)
			curX, curY = tx, ty
			pointIdx += 4

		case VerbCubicTo:
			// Two control points and end point
			c1x, c1y := points[pointIdx], points[pointIdx+1]
			c2x, c2y := points[pointIdx+2], points[pointIdx+3]
			x, y := points[pointIdx+4], points[pointIdx+5]

			// Transform all points
			tc1x, tc1y := transform.TransformPoint(c1x, c1y)
			tc2x, tc2y := transform.TransformPoint(c2x, c2y)
			tx, ty := transform.TransformPoint(x, y)

			eb.addCubic(curX, curY, tc1x, tc1y, tc2x, tc2y, tx, ty)
			curX, curY = tx, ty
			pointIdx += 6

		case VerbClose:
			// Close the subpath
			if curX != startX || curY != startY {
				eb.addLine(curX, curY, startX, startY)
			}
			curX, curY = startX, startY
		}
	}

	// Close final subpath if not explicitly closed
	if curX != startX || curY != startY {
		eb.addLine(curX, curY, startX, startY)
	}
}

// addLine adds a line edge, clipping to clipRect if set.
// When clipRect is active, delegates to clipAndAddLine which may emit
// sentinel vertical lines at clip boundaries to preserve winding.
func (eb *EdgeBuilder) addLine(x0, y0, x1, y1 float32) {
	if eb.clipRect != nil {
		eb.clipAndAddLine(x0, y0, x1, y1)
		return
	}
	eb.addLineUnclipped(x0, y0, x1, y1)
}

// addLineUnclipped adds a line edge without clipping.
// This is the original addLine logic, called directly by clipAndAddLine
// after coordinates have been clipped to safe range.
func (eb *EdgeBuilder) addLineUnclipped(x0, y0, x1, y1 float32) {
	// Update bounds
	eb.bounds.unionPoint(x0, y0)
	eb.bounds.unionPoint(x1, y1)

	// Store original float coordinates for Vello pipeline
	// (before fixed-point quantization in NewLineEdge)
	if eb.flattenCurves && y0 != y1 {
		isDown := y0 < y1
		p0 := [2]float32{x0, y0}
		p1 := [2]float32{x1, y1}
		if !isDown {
			p0, p1 = p1, p0
		}
		eb.velloLines = append(eb.velloLines, VelloLine{
			P0: p0, P1: p1, IsDown: isDown,
		})
	}

	// Create line edge
	p0 := CurvePoint{X: x0, Y: y0}
	p1 := CurvePoint{X: x1, Y: y1}

	edge := NewLineEdge(p0, p1, eb.aaShift)
	if edge == nil {
		return // Horizontal or degenerate
	}

	// Try to combine with previous vertical edge
	if edge.IsVertical() && len(eb.lineEdges) > 0 {
		last := &eb.lineEdges[len(eb.lineEdges)-1]
		combine := combineVertical(edge, last)
		switch combine {
		case combineTotal:
			// Edges cancel out - remove the last edge
			eb.lineEdges = eb.lineEdges[:len(eb.lineEdges)-1]
			return
		case combinePartial:
			// Last edge was modified - don't add new edge
			return
		case combineNo:
			// No combination - fall through to add
		}
	}

	eb.lineEdges = append(eb.lineEdges, *edge)
}

// clipAndAddLine clips a line to clipRect and emits clipped segments.
//
// Algorithm (Skia-style Y-then-X clipping):
//
// Phase 1: Y-clip — discard portions above/below clip rect.
// No sentinel verticals needed for Y because edges are Y-sorted and
// winding doesn't bleed vertically.
//
// Phase 2: X-clip with sentinel verticals — replace portions outside
// left/right boundaries with vertical lines at the boundary. This
// preserves the winding contribution of clipped edges.
//
// Reference: Skia SkEdgeClipper, tiny-skia edge_clipper.rs
func (eb *EdgeBuilder) clipAndAddLine(x0, y0, x1, y1 float32) {
	cr := eb.clipRect

	// Ensure y0 <= y1 for consistent clipping (track original direction)
	downward := true
	if y0 > y1 {
		x0, y0, x1, y1 = x1, y1, x0, y0
		downward = false
	}

	// Phase 1: Y-clip
	// Entirely above or below → discard
	if y1 <= cr.MinY || y0 >= cr.MaxY {
		return
	}

	// Clip to top Y boundary
	if y0 < cr.MinY {
		// Interpolate X at top boundary
		t := (cr.MinY - y0) / (y1 - y0)
		x0 += t * (x1 - x0)
		y0 = cr.MinY
	}

	// Clip to bottom Y boundary
	if y1 > cr.MaxY {
		t := (cr.MaxY - y0) / (y1 - y0)
		x1 = x0 + t*(x1-x0)
		y1 = cr.MaxY
	}

	// After Y-clip, check for degenerate
	if y0 >= y1 {
		return
	}

	// Phase 2: X-clip with sentinel verticals
	// Restore original direction for emit
	if !downward {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}

	eb.clipLineX(x0, y0, x1, y1)
}

// clipLineX handles the X-clipping phase, emitting sentinel verticals
// at clip boundaries for out-of-bounds portions.
func (eb *EdgeBuilder) clipLineX(x0, y0, x1, y1 float32) {
	cr := eb.clipRect
	left := cr.MinX
	right := cr.MaxX

	// Sort by Y for consistent boundary analysis
	var topX, topY, botX, botY float32
	if y0 <= y1 {
		topX, topY, botX, botY = x0, y0, x1, y1
	} else {
		topX, topY, botX, botY = x1, y1, x0, y0
	}

	// Fast paths for common cases
	if topX >= left && topX <= right && botX >= left && botX <= right {
		eb.addLineUnclipped(x0, y0, x1, y1) // Entirely inside
		return
	}
	if topX <= left && botX <= left {
		eb.addLineUnclipped(left, y0, left, y1) // Entirely left → sentinel
		return
	}
	if topX >= right && botX >= right {
		eb.addLineUnclipped(right, y0, right, y1) // Entirely right → sentinel
		return
	}

	// Line crosses one or both X boundaries — split and emit segments.
	preserveDir := y0 <= y1

	// Find intersection t values
	splits := eb.findXSplits(topX, topY, botX, botY, left, right)

	// Build and emit segments
	eb.emitClippedSegments(splits, topX, topY, botX, botY, left, right, preserveDir)
}

// findXSplits finds t values where a line (in top→bottom order) crosses
// the left and right clip boundaries. Returns sorted t values (max 2).
func (eb *EdgeBuilder) findXSplits(
	topX, _, botX, _ float32, left, right float32,
) [2]struct {
	t     float32
	x     float32
	valid bool
} {
	var splits [2]struct {
		t     float32
		x     float32
		valid bool
	}
	count := 0

	dx := botX - topX
	if dx == 0 {
		return splits
	}

	for _, boundary := range [2]float32{left, right} {
		tVal := (boundary - topX) / dx
		if tVal > 0 && tVal < 1 {
			splits[count] = struct {
				t     float32
				x     float32
				valid bool
			}{t: tVal, x: boundary, valid: true}
			count++
		}
	}

	// Sort by t if we have 2 splits
	if count == 2 && splits[0].t > splits[1].t {
		splits[0], splits[1] = splits[1], splits[0]
	}

	return splits
}

// emitClippedSegments emits line segments between split points.
// Inside segments are emitted as-is; outside segments become sentinel verticals.
func (eb *EdgeBuilder) emitClippedSegments(
	splits [2]struct {
		t     float32
		x     float32
		valid bool
	},
	topX, topY, botX, botY, left, right float32,
	preserveDir bool,
) {
	prevX, prevY := topX, topY

	for _, sp := range splits {
		if !sp.valid {
			break
		}
		yAt := topY + sp.t*(botY-topY)
		eb.emitSegment(prevX, prevY, sp.x, yAt, left, right, preserveDir)
		prevX, prevY = sp.x, yAt
	}
	eb.emitSegment(prevX, prevY, botX, botY, left, right, preserveDir)
}

// emitSegment emits a single line segment, converting outside portions to sentinels.
func (eb *EdgeBuilder) emitSegment(
	sx0, sy0, sx1, sy1, left, right float32, preserveDir bool,
) {
	midX := (sx0 + sx1) * 0.5
	var ex0, ey0, ex1, ey1 float32

	switch {
	case midX < left:
		ex0, ey0, ex1, ey1 = left, sy0, left, sy1
	case midX > right:
		ex0, ey0, ex1, ey1 = right, sy0, right, sy1
	default:
		ex0, ey0, ex1, ey1 = sx0, sy0, sx1, sy1
	}

	if !preserveDir {
		ex0, ey0, ex1, ey1 = ex1, ey1, ex0, ey0
	}
	eb.addLineUnclipped(ex0, ey0, ex1, ey1)
}

// curveBBoxInsideClip returns true if all given coordinates (control points
// and endpoints) are inside the clip rect. Used as a fast check for curves:
// if the convex hull (approximated by control point bbox) is inside clip,
// the curve itself is safe and doesn't need force-flattening.
func (eb *EdgeBuilder) curveBBoxInsideClip(coords ...float32) bool {
	cr := eb.clipRect
	for i := 0; i < len(coords); i += 2 {
		x, y := coords[i], coords[i+1]
		if x < cr.MinX || x > cr.MaxX || y < cr.MinY || y > cr.MaxY {
			return false
		}
	}
	return true
}

// SetFlattenCurves enables or disables curve flattening mode.
// When enabled, all curves are converted to line segments at build time.
// This is simpler and more reliable for AnalyticFiller.
func (eb *EdgeBuilder) SetFlattenCurves(flatten bool) {
	eb.flattenCurves = flatten
}

// FlattenCurves returns whether curve flattening is enabled.
func (eb *EdgeBuilder) FlattenCurves() bool {
	return eb.flattenCurves
}

// SetFlattenTolerance sets the curve flattening tolerance in pixels.
// When > 0, this overrides the default 0.1 px tolerance used for
// converting curves to line segments. Smaller values produce more
// segments (smoother curves).
//
// On HiDPI displays, set to baseTol/deviceScale for finer subdivision:
//
//	eb.SetFlattenTolerance(0.1 / deviceScale) // e.g., 0.05 for 2x Retina
//
// Set to 0 to use the default tolerance.
func (eb *EdgeBuilder) SetFlattenTolerance(tol float32) {
	if tol < 0 {
		tol = 0
	}
	eb.flattenTolerance = tol
}

// FlattenTolerance returns the current curve flattening tolerance.
// Returns 0 if using the default (0.1 px).
func (eb *EdgeBuilder) FlattenTolerance() float32 {
	return eb.flattenTolerance
}

// effectiveFlattenTolerance returns the tolerance to use for flattening.
// Returns the custom tolerance if set, otherwise the default 0.1 px.
func (eb *EdgeBuilder) effectiveFlattenTolerance() float32 {
	if eb.flattenTolerance > 0 {
		return eb.flattenTolerance
	}
	return 0.1 // default: tight tolerance for smooth curves
}

// SetClipRect sets the clipping rectangle for edge building.
// When set, all edges are clipped to this rect before fixed-point conversion,
// preventing FDot6→FDot16 integer overflow (RAST-010). Out-of-bounds line
// portions are replaced with sentinel vertical lines at clip boundaries
// to preserve correct winding numbers.
//
// Pass nil to disable clipping (default).
// The clip rect is preserved across Reset() calls.
func (eb *EdgeBuilder) SetClipRect(r *Rect) {
	eb.clipRect = r
}

// ClipRect returns the current clip rectangle, or nil if not set.
func (eb *EdgeBuilder) ClipRect() *Rect {
	return eb.clipRect
}

// VelloLines returns the stored float-coordinate lines.
// Only populated when flattenCurves is true.
func (eb *EdgeBuilder) VelloLines() []VelloLine {
	return eb.velloLines
}

// addQuad adds quadratic curve edges, chopping at Y extrema if needed.
func (eb *EdgeBuilder) addQuad(x0, y0, cx, cy, x1, y1 float32) {
	// If flattenCurves is enabled, convert curve to line segments.
	// Line clipping (clipAndAddLine) handles overflow prevention.
	if eb.flattenCurves {
		eb.flattenQuadToLines(x0, y0, cx, cy, x1, y1)
		return
	}

	// Safety guard: when clip rect is set and not flattening, force-flatten
	// curves that extend beyond clip bounds to ensure line clipping catches them.
	// This prevents FDot6→FDot16 overflow in NewQuadraticEdge. (RAST-010)
	if eb.clipRect != nil && !eb.curveBBoxInsideClip(x0, y0, cx, cy, x1, y1) {
		eb.flattenQuadToLines(x0, y0, cx, cy, x1, y1)
		return
	}

	// Check if curve needs to be chopped at Y extrema
	src := [3]GeomPoint{
		{X: x0, Y: y0},
		{X: cx, Y: cy},
		{X: x1, Y: y1},
	}

	var dst [5]GeomPoint
	numChops := ChopQuadAtYExtrema(src, &dst)

	// Update bounds with endpoints (which lie on the curve).
	// After chopping, dst[0], dst[2], dst[4] are points ON the curve.
	// Control points (dst[1], dst[3]) do NOT lie on the curve.
	eb.bounds.unionPoint(dst[0].X, dst[0].Y)
	eb.bounds.unionPoint(dst[2].X, dst[2].Y) // Shared point (or endpoint if no chop)
	if numChops > 0 {
		eb.bounds.unionPoint(dst[4].X, dst[4].Y)
	}

	// Add each monotonic segment
	for i := 0; i <= numChops; i++ {
		p0 := CurvePoint{X: dst[i*2].X, Y: dst[i*2].Y}
		p1 := CurvePoint{X: dst[i*2+1].X, Y: dst[i*2+1].Y}
		p2 := CurvePoint{X: dst[i*2+2].X, Y: dst[i*2+2].Y}

		edge := NewQuadraticEdge(p0, p1, p2, eb.aaShift)
		if edge != nil {
			eb.quadraticEdges = append(eb.quadraticEdges, edge)
		}
	}
}

// flattenQuadToLines converts a quadratic bezier to line segments.
// Uses adaptive subdivision based on flatness tolerance.
func (eb *EdgeBuilder) flattenQuadToLines(x0, y0, cx, cy, x1, y1 float32) {
	// Flatness tolerance: max deviation from straight line.
	// Default 0.1 px produces smooth curves even at small radii.
	// On HiDPI, effectiveFlattenTolerance() returns baseTol/deviceScale
	// for finer subdivision at physical pixel resolution.
	tolerance := eb.effectiveFlattenTolerance()

	eb.flattenQuadRecursive(x0, y0, cx, cy, x1, y1, tolerance, 0)
}

// flattenQuadRecursive recursively subdivides a quadratic curve until flat enough.
func (eb *EdgeBuilder) flattenQuadRecursive(x0, y0, cx, cy, x1, y1, tolerance float32, depth int) {
	// Max recursion depth to prevent stack overflow
	if depth > 10 {
		eb.addLine(x0, y0, x1, y1)
		return
	}

	// Compute flatness: distance from control point to line (x0,y0)-(x1,y1)
	// Using the formula: d = |cross(P1-P0, P2-P0)| / |P2-P0|
	dx := x1 - x0
	dy := y1 - y0
	dcx := cx - x0
	dcy := cy - y0

	// Cross product magnitude (2D)
	cross := dcx*dy - dcy*dx

	// Length of baseline squared
	lenSq := dx*dx + dy*dy

	// Flatness metric: cross^2 / lenSq (avoids sqrt)
	// If flat enough, emit line segment
	if lenSq < 1e-6 || cross*cross/lenSq < tolerance*tolerance {
		eb.addLine(x0, y0, x1, y1)
		return
	}

	// Subdivide at t=0.5 using de Casteljau
	// Q0 = (P0 + P1) / 2
	// Q1 = (P1 + P2) / 2
	// R0 = (Q0 + Q1) / 2
	q0x := (x0 + cx) * 0.5
	q0y := (y0 + cy) * 0.5
	q1x := (cx + x1) * 0.5
	q1y := (cy + y1) * 0.5
	r0x := (q0x + q1x) * 0.5
	r0y := (q0y + q1y) * 0.5

	// Recurse on both halves
	eb.flattenQuadRecursive(x0, y0, q0x, q0y, r0x, r0y, tolerance, depth+1)
	eb.flattenQuadRecursive(r0x, r0y, q1x, q1y, x1, y1, tolerance, depth+1)
}

// addCubic adds cubic curve edges, chopping at Y extrema if needed.
func (eb *EdgeBuilder) addCubic(x0, y0, c1x, c1y, c2x, c2y, x1, y1 float32) {
	// If flattenCurves is enabled, convert curve to line segments.
	// Line clipping (clipAndAddLine) handles overflow prevention.
	if eb.flattenCurves {
		eb.flattenCubicToLines(x0, y0, c1x, c1y, c2x, c2y, x1, y1)
		return
	}

	// Safety guard: when clip rect is set and not flattening, force-flatten
	// curves that extend beyond clip bounds to ensure line clipping catches them.
	// This prevents FDot6→FDot16 overflow in NewCubicEdge. (RAST-010)
	if eb.clipRect != nil && !eb.curveBBoxInsideClip(x0, y0, c1x, c1y, c2x, c2y, x1, y1) {
		eb.flattenCubicToLines(x0, y0, c1x, c1y, c2x, c2y, x1, y1)
		return
	}

	// Check if curve needs to be chopped at Y extrema
	src := [4]GeomPoint{
		{X: x0, Y: y0},
		{X: c1x, Y: c1y},
		{X: c2x, Y: c2y},
		{X: x1, Y: y1},
	}

	var dst [10]GeomPoint
	numChops := ChopCubicAtYExtrema(src, &dst)

	// Update bounds with endpoints (which lie on the curve).
	// After chopping, dst[0], dst[3], dst[6], dst[9] are points ON the curve.
	// Control points do NOT lie on the curve.
	eb.bounds.unionPoint(dst[0].X, dst[0].Y)
	eb.bounds.unionPoint(dst[3].X, dst[3].Y) // First shared point (or endpoint if no chop)
	if numChops >= 1 {
		eb.bounds.unionPoint(dst[6].X, dst[6].Y) // Second shared point
	}
	if numChops >= 2 {
		eb.bounds.unionPoint(dst[9].X, dst[9].Y) // Final endpoint
	}

	// Add each monotonic segment
	for i := 0; i <= numChops; i++ {
		p0 := CurvePoint{X: dst[i*3].X, Y: dst[i*3].Y}
		p1 := CurvePoint{X: dst[i*3+1].X, Y: dst[i*3+1].Y}
		p2 := CurvePoint{X: dst[i*3+2].X, Y: dst[i*3+2].Y}
		p3 := CurvePoint{X: dst[i*3+3].X, Y: dst[i*3+3].Y}

		edge := NewCubicEdge(p0, p1, p2, p3, eb.aaShift)
		if edge != nil {
			eb.cubicEdges = append(eb.cubicEdges, edge)
		}
	}
}

// flattenCubicToLines converts a cubic bezier to line segments.
// Uses adaptive subdivision based on flatness tolerance.
func (eb *EdgeBuilder) flattenCubicToLines(x0, y0, c1x, c1y, c2x, c2y, x1, y1 float32) {
	// Flatness tolerance: max deviation from straight line.
	// Default 0.1 px produces smooth curves even at small radii.
	// On HiDPI, effectiveFlattenTolerance() returns baseTol/deviceScale
	// for finer subdivision at physical pixel resolution.
	tolerance := eb.effectiveFlattenTolerance()

	eb.flattenCubicRecursive(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance, 0)
}

// flattenCubicRecursive recursively subdivides a cubic curve until flat enough.
func (eb *EdgeBuilder) flattenCubicRecursive(x0, y0, c1x, c1y, c2x, c2y, x1, y1, tolerance float32, depth int) {
	// Max recursion depth to prevent stack overflow
	if depth > 10 {
		eb.addLine(x0, y0, x1, y1)
		return
	}

	// Compute flatness: max distance from control points to line (x0,y0)-(x1,y1)
	dx := x1 - x0
	dy := y1 - y0
	lenSq := dx*dx + dy*dy

	if lenSq < 1e-6 {
		eb.addLine(x0, y0, x1, y1)
		return
	}

	// Distance from c1 to line
	dc1x := c1x - x0
	dc1y := c1y - y0
	cross1 := dc1x*dy - dc1y*dx

	// Distance from c2 to line
	dc2x := c2x - x0
	dc2y := c2y - y0
	cross2 := dc2x*dy - dc2y*dx

	// Use max flatness
	maxCross := cross1
	if cross1 < 0 {
		maxCross = -cross1
	}
	if cross2 > maxCross {
		maxCross = cross2
	}
	if cross2 < -maxCross {
		maxCross = -cross2
	}

	// If flat enough, emit line segment
	if maxCross*maxCross/lenSq < tolerance*tolerance {
		eb.addLine(x0, y0, x1, y1)
		return
	}

	// Subdivide at t=0.5 using de Casteljau
	// Level 1
	m01x := (x0 + c1x) * 0.5
	m01y := (y0 + c1y) * 0.5
	m12x := (c1x + c2x) * 0.5
	m12y := (c1y + c2y) * 0.5
	m23x := (c2x + x1) * 0.5
	m23y := (c2y + y1) * 0.5
	// Level 2
	m012x := (m01x + m12x) * 0.5
	m012y := (m01y + m12y) * 0.5
	m123x := (m12x + m23x) * 0.5
	m123y := (m12y + m23y) * 0.5
	// Level 3 - midpoint
	mx := (m012x + m123x) * 0.5
	my := (m012y + m123y) * 0.5

	// Recurse on both halves
	eb.flattenCubicRecursive(x0, y0, m01x, m01y, m012x, m012y, mx, my, tolerance, depth+1)
	eb.flattenCubicRecursive(mx, my, m123x, m123y, m23x, m23y, x1, y1, tolerance, depth+1)
}

// combineResult represents the result of trying to combine vertical edges.
type combineResult int

const (
	combineNo      combineResult = iota // No combination possible
	combinePartial                      // Partial combination - last edge modified
	combineTotal                        // Total cancellation - remove last edge
)

// combineVertical attempts to combine two vertical edges.
// This optimization reduces edge count for paths with coincident vertical segments.
func combineVertical(edge, last *LineEdge) combineResult {
	// Both must be vertical and at the same X
	if last.DX != 0 || edge.X != last.X {
		return combineNo
	}

	// Same winding - try to extend
	if edge.Winding == last.Winding {
		if edge.LastY+1 == last.FirstY {
			last.FirstY = edge.FirstY
			return combinePartial
		}
		if edge.FirstY == last.LastY+1 {
			last.LastY = edge.LastY
			return combinePartial
		}
		return combineNo
	}

	// Opposite winding - try to cancel or reduce
	if edge.FirstY == last.FirstY {
		if edge.LastY == last.LastY {
			return combineTotal // Exact cancellation
		}
		if edge.LastY < last.LastY {
			last.FirstY = edge.LastY + 1
			return combinePartial
		}
		// edge.LastY > last.LastY
		last.FirstY = last.LastY + 1
		last.LastY = edge.LastY
		last.Winding = edge.Winding
		return combinePartial
	}

	if edge.LastY == last.LastY {
		if edge.FirstY > last.FirstY {
			last.LastY = edge.FirstY - 1
			return combinePartial
		}
		// edge.FirstY < last.FirstY
		last.LastY = last.FirstY - 1
		last.FirstY = edge.FirstY
		last.Winding = edge.Winding
		return combinePartial
	}

	return combineNo
}

// Bounds returns the bounding rectangle of all edges.
func (eb *EdgeBuilder) Bounds() Rect {
	if eb.bounds.empty {
		return EmptyRect()
	}
	return Rect{
		MinX: eb.bounds.minX,
		MinY: eb.bounds.minY,
		MaxX: eb.bounds.maxX,
		MaxY: eb.bounds.maxY,
	}
}

// IsEmpty returns true if no edges have been added.
func (eb *EdgeBuilder) IsEmpty() bool {
	return len(eb.lineEdges) == 0 &&
		len(eb.quadraticEdges) == 0 &&
		len(eb.cubicEdges) == 0 &&
		len(eb.velloLines) == 0
}

// EdgeCount returns the total number of edges.
func (eb *EdgeBuilder) EdgeCount() int {
	return len(eb.lineEdges) + len(eb.quadraticEdges) + len(eb.cubicEdges)
}

// LineEdgeCount returns the number of line edges.
func (eb *EdgeBuilder) LineEdgeCount() int {
	return len(eb.lineEdges)
}

// QuadraticEdgeCount returns the number of quadratic edges.
func (eb *EdgeBuilder) QuadraticEdgeCount() int {
	return len(eb.quadraticEdges)
}

// CubicEdgeCount returns the number of cubic edges.
func (eb *EdgeBuilder) CubicEdgeCount() int {
	return len(eb.cubicEdges)
}

// sortableEdge pairs an edge with its top Y for sorting.
type sortableEdge struct {
	topY    int32
	variant CurveEdgeVariant
}

// AllEdges returns an iterator over all edges sorted by top Y coordinate.
//
// This uses Go 1.25+ iter.Seq for efficient iteration. Edges are yielded
// in scanline order (top to bottom), which is required for Active Edge Table
// processing.
//
// Usage:
//
//	for edge := range eb.AllEdges() {
//	    line := edge.AsLine()
//	    // Process edge.Line().FirstY, etc.
//	}
func (eb *EdgeBuilder) AllEdges() iter.Seq[CurveEdgeVariant] {
	return func(yield func(CurveEdgeVariant) bool) {
		// Collect all edges with their top Y for sorting
		edges := make([]sortableEdge, 0, eb.EdgeCount())

		// Add line edges
		for i := range eb.lineEdges {
			edges = append(edges, sortableEdge{
				topY: eb.lineEdges[i].FirstY,
				variant: CurveEdgeVariant{
					Type: EdgeTypeLine,
					Line: &eb.lineEdges[i],
				},
			})
		}

		// Add quadratic edges (use TopY for sorting, not current segment's FirstY)
		for _, quad := range eb.quadraticEdges {
			edges = append(edges, sortableEdge{
				topY: quad.TopY,
				variant: CurveEdgeVariant{
					Type:      EdgeTypeQuadratic,
					Quadratic: quad,
				},
			})
		}

		// Add cubic edges (use TopY for sorting, not current segment's FirstY)
		for _, cubic := range eb.cubicEdges {
			edges = append(edges, sortableEdge{
				topY: cubic.TopY,
				variant: CurveEdgeVariant{
					Type:  EdgeTypeCubic,
					Cubic: cubic,
				},
			})
		}

		// Sort by top Y (stable sort preserves insertion order for equal Y)
		slices.SortStableFunc(edges, func(a, b sortableEdge) int {
			if a.topY < b.topY {
				return -1
			}
			if a.topY > b.topY {
				return 1
			}
			return 0
		})

		// Yield edges in sorted order
		for _, e := range edges {
			if !yield(e.variant) {
				return
			}
		}
	}
}

// LineEdges returns an iterator over line edges only.
func (eb *EdgeBuilder) LineEdges() iter.Seq[*LineEdge] {
	return func(yield func(*LineEdge) bool) {
		for i := range eb.lineEdges {
			if !yield(&eb.lineEdges[i]) {
				return
			}
		}
	}
}

// QuadraticEdges returns an iterator over quadratic edges only.
func (eb *EdgeBuilder) QuadraticEdges() iter.Seq[*QuadraticEdge] {
	return func(yield func(*QuadraticEdge) bool) {
		for _, edge := range eb.quadraticEdges {
			if !yield(edge) {
				return
			}
		}
	}
}

// CubicEdges returns an iterator over cubic edges only.
func (eb *EdgeBuilder) CubicEdges() iter.Seq[*CubicEdge] {
	return func(yield func(*CubicEdge) bool) {
		for _, edge := range eb.cubicEdges {
			if !yield(edge) {
				return
			}
		}
	}
}

// AAShift returns the anti-aliasing shift value.
func (eb *EdgeBuilder) AAShift() int {
	return eb.aaShift
}
