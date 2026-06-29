package text

// Segment detection and linking for the auto-hinter.
//
// A "segment" is a contiguous run of outline points that move in the
// same direction along the major axis. Segments represent the edges
// of stems (vertical bars in 'H', horizontal bar in 'T', etc.).
//
// After detection, segments are linked into stems by finding opposing
// segment pairs with sufficient overlap.
//
// References:
//   - FreeType aflatin.c:1557 af_latin_hints_compute_segments
//   - FreeType aflatin.c:2016 af_latin_hints_link_segments
//   - skrifa topo/segments.rs compute_segments, link_segments

// hintPoint represents an outline point for the auto-hinter.
// Each point has coordinates in both font units and 26.6 fixed-point:
//   - fx, fy = font units (used for direction classification, segment detection)
//   - ox, oy = original scaled coordinates in 26.6 fixed-point
//   - x, y   = current (possibly hinted) scaled coordinates in 26.6 fixed-point
//
// The u, v fields are temporaries used by align_weak_points (IUP).
//
// References:
//   - skrifa outline.rs: Point { fx, fy (i32 font units), ox, oy, x, y (i32, 26.6) }
//   - FreeType afhints.h:239: AF_PointRec
type hintPoint struct {
	// Font-unit coordinates (unscaled). Used for direction computation
	// and segment detection (matching skrifa's point.fx/fy = font units).
	// For the contour-based path, these are the raw glyf table coordinates.
	// For the legacy path, these are the scaled coordinates (no font units available).
	fx, fy float32

	// Current (possibly hinted) scaled coordinates, 26.6 fixed-point.
	// Matches skrifa Point.x/y: i32.
	x, y int32

	// Original scaled coordinates (pre-hinting), 26.6 fixed-point.
	// Matches skrifa Point.ox/oy: i32.
	ox, oy int32

	// Temporaries for IUP (align_weak_points).
	// u = current coordinate being interpolated, v = original reference.
	// Matches skrifa Point.u/v: i32.
	u, v int32

	// Incoming direction at this point (from prev non-near point to this).
	// See FreeType afhints.c:1194 — set by computePointProperties.
	inDir hintDirection

	// Outgoing direction at this point (from this to next non-near point).
	outDir hintDirection

	// Index of next/prev point in the same contour.
	next, prev int

	// Flags for point classification.
	flags hintPointFlags

	// Contour index this point belongs to.
	contour int
}

// hintDirection represents the direction of an outline segment.
type hintDirection int8

const (
	dirNone  hintDirection = 0
	dirUp    hintDirection = 1  // Positive Y (up in font coordinates)
	dirDown  hintDirection = -1 // Negative Y
	dirRight hintDirection = 2  // Positive X
	dirLeft  hintDirection = -2 // Negative X
)

// hintPointFlags classifies points for hinting.
type hintPointFlags uint8

const (
	pointFlagControl  hintPointFlags = 1 << iota // Off-curve control point
	pointFlagWeak                                // Weak point (near duplicate or no clear direction)
	pointFlagTouchedX                            // X already adjusted
	pointFlagTouchedY                            // Y already adjusted
)

// hintPointArray holds the flattened array of outline points with
// contour boundaries for the auto-hinter.
type hintPointArray struct {
	pts      []hintPoint
	contours []contourRange // start/end indices for each contour

	// nearLimit is the distance threshold for "near" point merging.
	// In font units: 20 * units_per_em / 2048 (skrifa outline/mod.rs:178).
	// In scaled pixels: 20/64 = 0.3125 (legacy path).
	nearLimit float32

	// scale is the ppem / unitsPerEm ratio. Used by edge computation to
	// bridge font-unit segment positions to scaled pixel coordinates.
	// Zero for the legacy path (which has no font unit / scaled split).
	scale float64

	// unitsPerEm is the font's design units per em. Stored directly to
	// avoid precision loss when recovering from nearLimit (integer truncation
	// in 20*UPM/2048 loses information). Zero for the legacy path.
	unitsPerEm int
}

// contourRange identifies a contour within the point array.
type contourRange struct {
	start, end int // inclusive indices into pts
}

// hintSegment represents a detected axis-aligned segment in an outline.
type hintSegment struct {
	// pos is the position along the major axis (average of min/max).
	pos float32

	// delta is half the width of the segment along the major axis.
	delta float32

	// minCoord, maxCoord are the extent along the minor axis.
	minCoord, maxCoord float32

	// height is maxCoord - minCoord (used for segment filtering).
	height float32

	// dir is the segment direction (+1 or -1 along major axis).
	dir hintDirection

	// flags for segment classification.
	flags uint32

	// linkIdx is the index of the linked segment (stem partner), or -1.
	linkIdx int16

	// serifIdx is the index of the serif segment, or -1.
	serifIdx int16

	// score for segment linking (lower is better).
	// Default: distance-based score with length demerits.
	// CJK: raw distance (int32 stored as float32).
	score float32

	// segLen is the overlap length stored alongside score for CJK linking.
	// CJK uses (dist*8 < score*9 && (dist*8 < score*7 || segLen < len))
	// to decide whether to accept a closer match.
	// Only meaningful for CJK; unused by Default linking.
	segLen int32

	// edgeIdx is the index of the edge this segment belongs to, or -1.
	edgeIdx int16

	// firstPt, lastPt are the first and last point indices in this segment.
	firstPt, lastPt int
}

// Edge flags matching FreeType AF_EDGE_*.
const (
	edgeFlagRound uint32 = 1 << iota
	edgeFlagSerif
	edgeFlagDone
	edgeFlagNeutral
)

// buildHintPointsFromContours converts raw TrueType contour points into a
// hintPointArray suitable for the auto-hinter. This is the FreeType/skrifa-
// correct path: it operates on the exact raw contour points (e.g., 32 for
// NotoSerifHebrew glyph 9), not the pen-derived outline segments (which
// expand to 42+ points due to curve decomposition).
//
// Critically, fx/fy store FONT UNITS (unscaled) while ox/oy/x/y store scaled
// pixel coordinates. This matches skrifa exactly:
//   - Direction computation uses fx/fy (font units) — skrifa outline/mod.rs
//   - Segment detection uses fx/fy (font units) — skrifa topo/segments.rs
//   - Edge hinting uses ox/oy/x/y (scaled) — skrifa hint/edges.rs
//
// Y-UP convention is maintained throughout (matching FreeType/skrifa).
//
// References:
//   - FreeType afhints.c:1080 af_glyph_hints_reload (loads points from FT_Outline)
//   - skrifa hint/outline.rs Outline::new (builds from raw contour points)
//   - skrifa outline.rs:194 Outline::scale (sets ox/oy from fx/fy * scale)
func buildHintPointsFromContours(contours *GlyfContours, scale float64, unitsPerEm int) hintPointArray {
	var result hintPointArray
	if contours == nil || len(contours.Points) == 0 {
		return result
	}

	numPoints := len(contours.Points)
	result.pts = make([]hintPoint, numPoints)
	result.scale = scale
	result.unitsPerEm = unitsPerEm

	// Near limit in font units: 20 * UPM / 2048.
	// Matches skrifa outline/mod.rs:178: let near_limit = 20 * self.units_per_em / 2048
	result.nearLimit = float32(20 * unitsPerEm / 2048)

	// Store both font-unit and scaled coordinates, matching skrifa's architecture:
	//   fx, fy = font units (for direction computation, segment detection)
	//   ox, oy / x, y = scaled to 26.6 fixed-point (for edge hinting, point alignment)
	//
	// Y-UP convention throughout (matching FreeType/skrifa).
	for i, cp := range contours.Points {
		// Font-unit coordinates (unscaled, Y-UP).
		// Matches skrifa point.fx/fy = font units.
		fxUnit := float32(cp.X)
		fyUnit := float32(cp.Y)

		// Scaled pixel coordinates in 26.6 fixed-point.
		// Matches skrifa Outline::scale: ox = fixed_mul(fx, x_scale).
		sx := f26dot6FromFloat(float64(cp.X) * scale)
		sy := f26dot6FromFloat(float64(cp.Y) * scale)

		result.pts[i] = hintPoint{
			fx:      fxUnit,
			fy:      fyUnit,
			x:       sx,
			y:       sy,
			ox:      sx,
			oy:      sy,
			contour: -1, // set below
		}

		if !cp.OnCurve {
			result.pts[i].flags |= pointFlagControl
		}
	}

	// Build contour ranges from EndPts.
	start := 0
	for ci, endPt := range contours.EndPts {
		end := int(endPt)
		if end >= numPoints {
			end = numPoints - 1
		}
		if start > end {
			start = end
		}

		result.contours = append(result.contours, contourRange{
			start: start,
			end:   end,
		})

		// Set contour index and next/prev links.
		for i := start; i <= end; i++ {
			result.pts[i].contour = ci
			if i < end {
				result.pts[i].next = i + 1
			} else {
				result.pts[i].next = start // wrap to contour start
			}
			if i > start {
				result.pts[i].prev = i - 1
			} else {
				result.pts[i].prev = end // wrap to contour end
			}
		}

		start = end + 1
	}

	// Compute directions and classify weak points.
	// Uses fx/fy (font units) and nearLimit (font units).
	computePointDirections(&result)

	return result
}

// buildHintPoints converts a GlyphOutline into a hintPointArray suitable
// for the auto-hinter. Points are in font-unit pixel space.
// The Y axis is NOT flipped — we work in font coordinates (Y-up).
func buildHintPoints(outline *GlyphOutline) hintPointArray {
	var result hintPointArray

	// First pass: collect all points and identify contours.
	contourStart := 0
	for i, seg := range outline.Segments {
		if seg.Op == OutlineOpMoveTo && i > 0 {
			// Close previous contour.
			if contourStart < len(result.pts) {
				result.contours = append(result.contours, contourRange{
					start: contourStart,
					end:   len(result.pts) - 1,
				})
			}
			contourStart = len(result.pts)
		}

		pointCount := segPointCount(seg.Op)
		for j := range pointCount {
			isControl := false
			switch seg.Op {
			case OutlineOpQuadTo:
				isControl = j == 0
			case OutlineOpCubicTo:
				isControl = j < 2
			}

			pt := hintPoint{
				fx:      seg.Points[j].X,
				fy:      seg.Points[j].Y,
				contour: len(result.contours),
			}
			pt.x = f26dot6FromFloat(float64(pt.fx))
			pt.y = f26dot6FromFloat(float64(pt.fy))
			pt.ox = pt.x
			pt.oy = pt.y

			if isControl {
				pt.flags |= pointFlagControl
			}

			result.pts = append(result.pts, pt)
		}
	}

	// Close last contour.
	if contourStart < len(result.pts) {
		result.contours = append(result.contours, contourRange{
			start: contourStart,
			end:   len(result.pts) - 1,
		})
	}

	// Set up next/prev links and compute directions.
	for ci, cr := range result.contours {
		for i := cr.start; i <= cr.end; i++ {
			result.pts[i].contour = ci
			if i < cr.end {
				result.pts[i].next = i + 1
			} else {
				result.pts[i].next = cr.start
			}
			if i > cr.start {
				result.pts[i].prev = i - 1
			} else {
				result.pts[i].prev = cr.end
			}
		}
	}

	// Compute outgoing directions.
	computePointDirections(&result)

	return result
}

// computePointProperties computes in/out directions and classifies weak points.
// This is a port of FreeType's af_glyph_hints_compute_point_properties
// (afhints.c:1080). It runs three classification passes:
//
//  1. Compute raw outgoing directions and merge "near" points (points very
//     close to their predecessor are marked weak, their vectors accumulated
//     into the preceding non-near point).
//  2. Mark points where in_dir == dirNone && out_dir == dirNone AND the two
//     vectors point into the same quadrant as weak ("smooth diagonal interior").
//  3. Mark remaining non-directional points as weak: control points, points
//     where in_dir == out_dir (mid-segment), and collinear points.
//
// Diagonal glyphs ('v','w','A','x','k','z') are primarily composed of
// same-quadrant diagonal points that MUST be classified as weak. Without
// this classification, alignStrongPoints will try to interpolate them
// between edges — producing catastrophic corruption.
//
// Uses fx/fy coordinates (which are font units for the contour path,
// or scaled pixels for the legacy path) and the pa.nearLimit threshold.
//
// See FreeType afhints.c:1080 af_glyph_hints_compute_point_properties
// See skrifa outline/mod.rs compute_point_properties
func computePointProperties(pa *hintPointArray) {
	nl := pa.nearLimit
	if nl == 0 {
		nl = defaultNearLimit
	}
	for _, cr := range pa.contours {
		if cr.end-cr.start < 2 {
			continue // Need at least 3 points.
		}
		computeDirectionsPass(pa.pts, cr, nl)
		classifySameQuadrantWeak(pa.pts, cr)
		classifyRemainingWeak(pa.pts, cr)
	}
}

// defaultNearLimit: if a vector is shorter than this, the next point is "near".
// FreeType uses 20 (in 26.6 fixed point = 20/64 ≈ 0.3125 pixels).
// This is used for the legacy path where fx/fy are in scaled pixels.
// The contour path uses pa.nearLimit (in font units) instead.
const defaultNearLimit float32 = 0.3125

// computeDirectionsPass is PASS 1 of weak point classification.
// Walks the contour accumulating vectors across "near" points.
// Near points get marked weak; non-near points get the accumulated direction.
// Also builds the simplified topology chain: u = next non-near point index,
// v = previous non-near point index. Passes 2 and 3 use this chain to skip
// over near/weak points when checking vector quadrants and flat corners.
//
// The nl parameter is the near limit (font units for contour path, scaled for legacy).
// See FreeType afhints.c:1100-1208.
// See skrifa outline/mod.rs compute_directions.
func computeDirectionsPass(pts []hintPoint, cr contourRange, nl float32) {
	// Backward walk: find the first non-near point.
	// Skrifa uses near_limit2 = 2*near_limit - 1 for this step.
	nl2 := 2*nl - 1
	firstIdx := cr.start
	foundStart := false
	idx := firstIdx
	prevIdx := pts[idx].prev
	for prevIdx != firstIdx {
		dx := pts[idx].fx - pts[prevIdx].fx
		dy := pts[idx].fy - pts[prevIdx].fy
		if absF32(dx)+absF32(dy) >= nl2 {
			foundStart = true
			break
		}
		idx = prevIdx
		prevIdx = pts[idx].prev
	}
	if !foundStart {
		// Check the last candidate.
		dx := pts[idx].fx - pts[prevIdx].fx
		dy := pts[idx].fy - pts[prevIdx].fy
		if absF32(dx)+absF32(dy) >= nl2 {
			foundStart = true
		}
	}
	if !foundStart {
		// All points are near — mark everything weak.
		for i := cr.start; i <= cr.end; i++ {
			pts[i].flags |= pointFlagWeak
		}
		return
	}
	firstIdx = idx

	// Initialize the u/v topology chain for the first non-near point.
	// u = next non-near index, v = previous non-near index.
	// Initially, firstIdx points to itself.
	pts[firstIdx].u = int32(firstIdx)
	pts[firstIdx].v = int32(firstIdx)

	// Forward walk: accumulate vectors, mark near points weak,
	// assign directions, and build the u/v chain.
	currIdx := firstIdx
	var outX, outY float32
	nextIdx := firstIdx

	for {
		pointIdx := nextIdx
		nextIdx = pts[pointIdx].next

		outX += pts[nextIdx].fx - pts[pointIdx].fx
		outY += pts[nextIdx].fy - pts[pointIdx].fy

		if absF32(outX)+absF32(outY) < nl {
			// Next point is "near" — mark it weak, accumulate further.
			pts[nextIdx].flags |= pointFlagWeak
			if nextIdx == firstIdx {
				break
			}
			continue
		}

		// Significant vector found: assign direction.
		d := directionCompute(outX, outY)
		pts[nextIdx].inDir = d
		pts[nextIdx].v = int32(currIdx) // v = prev non-near

		pts[currIdx].u = int32(nextIdx) // u = next non-near
		pts[currIdx].outDir = d

		// Set directions for all intermediate (near) points.
		for walkIdx := pts[currIdx].next; walkIdx != nextIdx; walkIdx = pts[walkIdx].next {
			pts[walkIdx].inDir = d
			pts[walkIdx].outDir = d
		}

		currIdx = nextIdx
		// Keep firstIdx's v updated to the latest non-near point.
		pts[currIdx].u = int32(firstIdx)
		pts[firstIdx].v = int32(currIdx)
		outX = 0
		outY = 0

		if nextIdx == firstIdx {
			break
		}
	}
}

// classifySameQuadrantWeak is PASS 2 of weak point classification.
// If both in and out vectors have dirNone AND both vectors point into the
// same quadrant (same sign for both X and Y components), mark as weak.
// This catches diagonal interior points (e.g., along the strokes of 'v', 'w', 'A').
//
// Uses the u/v topology chain (set in pass 1) to look up the next/prev
// non-near points, skipping over near/weak points. When a point is marked
// weak, its neighbors' u/v are updated to bypass it.
//
// See FreeType afhints.c:1220-1253.
// See skrifa outline/mod.rs simplify_topology.
func classifySameQuadrantWeak(pts []hintPoint, cr contourRange) {
	for i := cr.start; i <= cr.end; i++ {
		pt := &pts[i]
		if (pt.flags & pointFlagWeak) != 0 {
			continue
		}
		if pt.inDir != dirNone || pt.outDir != dirNone {
			continue
		}

		// Use u/v topology chain for next/prev non-near points.
		uIdx := int(pt.u) // next non-near
		vIdx := int(pt.v) // prev non-near

		inX := pt.fx - pts[vIdx].fx
		inY := pt.fy - pts[vIdx].fy
		outXq := pts[uIdx].fx - pt.fx
		outYq := pts[uIdx].fy - pt.fy

		// FreeType: (in_x ^ out_x) >= 0 && (in_y ^ out_y) >= 0
		if sameSign(inX, outXq) && sameSign(inY, outYq) {
			pt.flags |= pointFlagWeak
			// Update u/v chain: bypass this point.
			pts[vIdx].u = int32(uIdx)
			pts[uIdx].v = int32(vIdx)
		}
	}
}

// classifyRemainingWeak is PASS 3 of weak point classification.
// - Control points are always weak.
// - Points where in_dir == out_dir != dirNone (mid-segment) are weak.
// - Points where in_dir == -out_dir (spike) are weak.
// - Points where in_dir == out_dir == dirNone and are "flat" are weak.
//
// Uses the u/v topology chain (set in pass 1, updated in pass 2) to look
// up the next/prev non-near non-weak points for flat corner detection.
// When a point is marked weak, its neighbors' u/v are updated to bypass it.
//
// See FreeType afhints.c:1262-1308.
// See skrifa outline/mod.rs check_remaining_weak_points.
func classifyRemainingWeak(pts []hintPoint, cr contourRange) {
	for i := cr.start; i <= cr.end; i++ {
		pt := &pts[i]
		if (pt.flags & pointFlagWeak) != 0 {
			continue
		}

		if (pt.flags & pointFlagControl) != 0 {
			pt.flags |= pointFlagWeak
			continue
		}

		if pt.outDir == pt.inDir {
			if pt.outDir != dirNone {
				// Mid-segment point — weak.
				pt.flags |= pointFlagWeak
				continue
			}

			// Both dirNone but not same-quadrant. Check "flat corner".
			// Use u/v topology chain for the simplified neighbor lookup.
			uIdx := int(pt.u) // next non-weak
			vIdx := int(pt.v) // prev non-weak

			inX := pt.fx - pts[vIdx].fx
			inY := pt.fy - pts[vIdx].fy
			outXf := pts[uIdx].fx - pt.fx
			outYf := pts[uIdx].fy - pt.fy

			if isCornerFlat(inX, inY, outXf, outYf) {
				pt.flags |= pointFlagWeak
				// Update u/v chain: bypass this point.
				pts[vIdx].u = int32(uIdx)
				pts[uIdx].v = int32(vIdx)
			}
		} else if pt.inDir == -pt.outDir {
			// Spike — weak.
			pt.flags |= pointFlagWeak
		}
	}
}

// directionCompute determines the cardinal direction of a vector.
// Matches FreeType af_direction_compute (afhints.c:750):
// if the "long" arm is not at least 14x the "short" arm, return dirNone.
// The factor 14 corresponds to approximately 4.1 degrees.
func directionCompute(dx, dy float32) hintDirection {
	dir, ll, ss := classifyVector(dx, dy)
	if ss < 0 {
		ss = -ss
	}
	if ll <= 14*ss {
		return dirNone
	}
	return dir
}

// classifyVector classifies a vector into a cardinal direction and returns
// the long and short arm lengths. Matches FreeType afhints.c:758-787.
func classifyVector(dx, dy float32) (dir hintDirection, ll, ss float32) {
	if dy >= dx {
		if dy >= -dx {
			return dirUp, dy, dx
		}
		return dirLeft, -dx, dy
	}
	if dy >= -dx {
		return dirRight, dx, dy
	}
	return dirDown, -dy, dx
}

// sameSign returns true if both values have the same sign (or either is zero).
// Matches FreeType's (in_x ^ out_x) >= 0 check for floats.
func sameSign(a, b float32) bool {
	if a >= 0 {
		return b >= 0
	}
	return b <= 0
}

// isCornerFlat determines if an in-vector and out-vector form a "flat" corner
// (i.e., one vector dominates the other so the point is not a true corner).
//
// Uses the FreeType/skrifa JIT formula (ft_corner_is_flat from ftcalc.c:1026).
// A corner is flat if the triangle inequality gap is small:
//
//	(d_in + d_out - d_sum) < (d_sum >> 4)
//
// where d_in, d_out, d_sum are fast approximations of the vector lengths
// using hypot(x,y) = max(|x|,|y|) + 3/8 * min(|x|,|y|).
//
// This correctly identifies points where one vector dominates (e.g., a tiny
// out-vector vs a large in-vector), marking them as non-corners. The old
// cross/dot formula was wrong for near-perpendicular vectors with very
// different magnitudes (e.g., contour 3 of CJK glyphs).
//
// See skrifa outline.rs:477 is_corner_flat_jit.
// See FreeType ftcalc.c:1026 ft_corner_is_flat.
func isCornerFlat(inX, inY, outX, outY float32) bool {
	// Convert to int32 for integer arithmetic matching skrifa exactly.
	// Our float32 values are always exact integers (font unit coordinates).
	ix, iy := int32(inX), int32(inY)
	ox, oy := int32(outX), int32(outY)
	ax := ix + ox
	ay := iy + oy
	dIn := fastHypot(ix, iy)
	dOut := fastHypot(ox, oy)
	dSum := fastHypot(ax, ay)
	return (dIn + dOut - dSum) < (dSum >> 4)
}

// fastHypot computes an approximate vector length: max(|x|,|y|) + 3/8 * min(|x|,|y|).
// Matches FreeType/skrifa's inline hypot in ft_corner_is_flat.
func fastHypot(x, y int32) int32 {
	if x < 0 {
		x = -x
	}
	if y < 0 {
		y = -y
	}
	if x > y {
		return x + ((3 * y) >> 3)
	}
	return y + ((3 * x) >> 3)
}

// computePointDirections is the simplified direction computation entry point.
// It delegates to computePointProperties which implements the full FreeType
// 3-pass weak point classification algorithm.
func computePointDirections(pa *hintPointArray) {
	computePointProperties(pa)
}

// computeSegments scans outline contours and finds axis-aligned segments.
// For the horizontal dimension (dimHorizontal), we detect vertical segments
// (runs of points moving in dirUp or dirDown). For dimVertical, horizontal
// segments (dirLeft or dirRight).
//
// This is a faithful port of skrifa topo/segments.rs build_segments, which
// follows FreeType aflatin.c:1588. Key design:
//   - Coordinates used for pos/min/max come from fx/fy (font units for contour
//     path, scaled pixels for legacy path).
//   - When the contour starts mid-segment, we back up to find the true start.
//   - A circular walk with a "passed" flag handles wrap-around segments.
//   - The current point's coordinate is included in min/max BEFORE checking
//     whether the segment ends (this is why skrifa includes off-curve endpoints).
//
// See FreeType aflatin.c:1557 af_latin_hints_compute_segments.
// See skrifa topo/segments.rs build_segments.
//
//nolint:gocognit,gocyclo,cyclop,nestif,funlen,maintidx // FreeType aflatin.c port — algorithmic complexity is inherent
func computeSegments(pa *hintPointArray, dim hintDimension) []hintSegment {
	var segments []hintSegment

	// isSameAxis returns true if the direction is along the major axis.
	// Matches skrifa Direction::is_same_axis.
	isSameAxis := func(d hintDirection) bool {
		if dim == dimHorizontal {
			return d == dirUp || d == dirDown
		}
		return d == dirLeft || d == dirRight
	}

	// pointU returns the position along the major axis (font units).
	// Horizontal dim: pos = X (fx), coord = Y (fy).
	// Vertical dim: pos = Y (fy), coord = X (fx).
	pointU := func(idx int) float32 {
		if dim == dimHorizontal {
			return pa.pts[idx].fx
		}
		return pa.pts[idx].fy
	}
	pointV := func(idx int) float32 {
		if dim == dimHorizontal {
			return pa.pts[idx].fy
		}
		return pa.pts[idx].fx
	}

	for _, cr := range pa.contours {
		contourLen := cr.end - cr.start + 1
		if contourLen < 3 {
			continue
		}

		// Check if the contour starts on an edge and if so, back up to
		// find the starting point. This handles segments that wrap around
		// the contour boundary (e.g., seg with first=31, last=0).
		// See skrifa topo/segments.rs:316-330.
		pointIdx := cr.start
		lastIdx := cr.end
		if isSameAxis(pa.pts[pointIdx].outDir) && isSameAxis(pa.pts[lastIdx].outDir) {
			lastIdx = pointIdx
			for {
				pointIdx = pa.pts[pointIdx].prev
				if !isSameAxis(pa.pts[pointIdx].outDir) {
					pointIdx = pa.pts[pointIdx].next
					break
				}
				if pointIdx == lastIdx {
					break
				}
			}
		}
		lastIdx = pointIdx

		// Flat threshold for round segment detection.
		// A segment is "round" if either end is off-curve and the on-curve
		// span is small. Matches skrifa: flat_threshold = units_per_em / 14.
		// See FreeType aflatin.c:1588.
		flatThreshold := float32(32000) // effectively disabled if no UPM
		if pa.unitsPerEm > 0 {
			// Use stored UPM directly — avoids precision loss from round-tripping
			// through nearLimit (which uses integer division 20*UPM/2048).
			flatThreshold = float32(pa.unitsPerEm / 14)
		} else if pa.scale > 0 {
			// Legacy fallback: estimate UPM from nearLimit.
			upm := float32(pa.nearLimit * 2048.0 / 20.0)
			flatThreshold = upm / 14.0
		}

		// Segment state tracking (matching skrifa State struct).
		onEdge := false
		passed := false
		var segDir hintDirection
		var segFirstIdx int
		var minPos, maxPos, minCoord, maxCoord float32
		var minFlags, maxFlags hintPointFlags
		var minOnCoord, maxOnCoord float32

		const maxScoreF = float32(32000)
		const minScoreF = float32(-32000)

		// Walk the contour circularly.
		for {
			if onEdge {
				// Update position and coordinate bounds with current point.
				// CRITICAL: this happens BEFORE the termination check, so the
				// ending point's coordinates are included (skrifa lines 338-349).
				u := pointU(pointIdx)
				v := pointV(pointIdx)
				if u < minPos {
					minPos = u
				}
				if u > maxPos {
					maxPos = u
				}
				if v < minCoord {
					minCoord = v
					minFlags = pa.pts[pointIdx].flags
				}
				if v > maxCoord {
					maxCoord = v
					maxFlags = pa.pts[pointIdx].flags
				}
				// Track on-curve coordinate extent (for round detection).
				if (pa.pts[pointIdx].flags & pointFlagControl) == 0 {
					if v < minOnCoord {
						minOnCoord = v
					}
					if v > maxOnCoord {
						maxOnCoord = v
					}
				}

				// Check if the segment ends here.
				// Skrifa: point.out_dir != segment_dir || point_ix == last_ix
				if pa.pts[pointIdx].outDir != segDir || pointIdx == lastIdx {
					// Finalize the segment. Note: skrifa has complex merging
					// logic for segments that share an endpoint (prev_segment_ix).
					// For now we use the simpler "just create the segment" path
					// which matches the non-merging case.
					// Position and delta use integer right-shift to match
					// skrifa's (min_pos + max_pos) >> 1 arithmetic.
					// Using float division would give 481.5 → rounds to 482,
					// but skrifa truncates to 481. The >> 1 is equivalent to
					// floor((a+b)/2) for non-negative sums.
					iMinPos := int(minPos)
					iMaxPos := int(maxPos)
					var segFlags uint32
					// A segment is round if either end point is a control
					// (off-curve) and the on-curve span is within the flat
					// threshold. Matches skrifa State::apply_to_segment.
					minIsControl := (minFlags & pointFlagControl) != 0
					maxIsControl := (maxFlags & pointFlagControl) != 0
					if (minIsControl || maxIsControl) && (maxOnCoord-minOnCoord) < flatThreshold {
						segFlags |= edgeFlagRound
					}
					seg := hintSegment{
						pos:      float32((iMinPos + iMaxPos) >> 1),
						delta:    float32((iMaxPos - iMinPos) >> 1),
						minCoord: minCoord,
						maxCoord: maxCoord,
						height:   maxCoord - minCoord,
						dir:      segDir,
						flags:    segFlags,
						linkIdx:  -1,
						serifIdx: -1,
						edgeIdx:  -1,
						score:    32000,
						firstPt:  segFirstIdx,
						lastPt:   pointIdx,
					}
					segments = append(segments, seg)
					onEdge = false
				}
			}

			// Check if we've completed the circular walk.
			if pointIdx == lastIdx {
				if passed {
					break
				}
				passed = true
			}

			// Try to start a new segment.
			if !onEdge && isSameAxis(pa.pts[pointIdx].outDir) {
				if len(segments) > 1000 {
					return nil
				}
				segDir = pa.pts[pointIdx].outDir
				segFirstIdx = pointIdx
				u := pointU(pointIdx)
				v := pointV(pointIdx)
				minPos = u
				maxPos = u
				minCoord = v
				maxCoord = v
				minFlags = pa.pts[pointIdx].flags
				maxFlags = pa.pts[pointIdx].flags
				if (pa.pts[pointIdx].flags & pointFlagControl) != 0 {
					minOnCoord = maxScoreF
					maxOnCoord = minScoreF
				} else {
					minOnCoord = v
					maxOnCoord = v
				}
				onEdge = true
			}

			// Advance to next point in contour.
			pointIdx = pa.pts[pointIdx].next
		}
	}

	return segments
}

// adjustSegmentHeights slightly increases segment heights when neighboring
// points extend beyond the segment boundaries. This improves serif detection
// by making stem segments appear taller when their adjacent points contribute
// to the visual extent.
//
// For each segment, look at the point before firstPt and after lastPt:
//   - If the neighbor extends the segment's coordinate range, add half the
//     extension to the height.
//
// This is called after build_segments (computeSegments) and before link_segments.
//
// See skrifa topo/segments.rs adjust_segment_heights.
// See FreeType aflatin.c:1933 af_latin_hints_adjust_segment_heights.
//
//nolint:nestif // FreeType aflatin.c port — directional conditional structure
func adjustSegmentHeights(pa *hintPointArray, segments []hintSegment, dim hintDimension) {
	pointV := func(idx int) float32 {
		if dim == dimHorizontal {
			return pa.pts[idx].fy
		}
		return pa.pts[idx].fx
	}

	for i := range segments {
		seg := &segments[i]
		firstIdx := seg.firstPt
		lastIdx := seg.lastPt

		if firstIdx < 0 || firstIdx >= len(pa.pts) || lastIdx < 0 || lastIdx >= len(pa.pts) {
			continue
		}

		firstV := pointV(firstIdx)
		lastV := pointV(lastIdx)
		prevV := pointV(pa.pts[firstIdx].prev)
		nextV := pointV(pa.pts[lastIdx].next)

		// FreeType/skrifa use integer right-shift (>> 1) for the half-extension.
		// We must match: truncating integer division, not float division.
		halfInt := func(a, b float32) float32 {
			return float32(int32(a-b) >> 1)
		}

		if firstV < lastV {
			if prevV < firstV {
				seg.height += halfInt(firstV, prevV)
			}
			if nextV > lastV {
				seg.height += halfInt(nextV, lastV)
			}
		} else {
			if prevV > firstV {
				seg.height += halfInt(prevV, firstV)
			}
			if nextV < lastV {
				seg.height += halfInt(lastV, nextV)
			}
		}
	}
}

// linkSegments dispatches to Default or CJK segment linking based on script group.
//
// See skrifa topo/segments.rs link_segments (dispatcher).
func linkSegments(segments []hintSegment, axis *scaledAxisMetrics, group scriptGroup) {
	if group == scriptGroupCJK {
		linkSegmentsCJK(segments, axis)
	} else {
		linkSegmentsDefault(segments, axis)
	}
}

// linkSegmentsDefault links opposing segments into stems (Default/Latin algorithm).
// For each segment with the major direction, find the best opposing segment
// based on overlap and distance.
//
// All segment coordinates (pos, minCoord, maxCoord) are in font units.
// The scoring thresholds are also in font units, matching skrifa's
// link_segments_default which receives unscaled max_width.
//
// See FreeType aflatin.c:2016 af_latin_hints_link_segments.
// See skrifa topo/segments.rs link_segments_default.
//
//nolint:gocognit,gocyclo,cyclop // FreeType aflatin.c port — algorithmic complexity is inherent
func linkSegmentsDefault(segments []hintSegment, axis *scaledAxisMetrics) {
	if len(segments) < 2 {
		return
	}

	// UPM for derived constants. Use stored value if available (contour path),
	// fall back to estimate from scale (legacy path).
	upm := axis.unitsPerEm
	if upm <= 0 && axis.scale > 0 {
		// Legacy fallback: estimate from standardWidth if available.
		upm = 1000 // reasonable default
	}
	if upm <= 0 {
		upm = 1000
	}

	// Heuristic thresholds in font units (matching skrifa link_segments_default).
	// derived_constant(UPM, value) = value * UPM / 2048
	lenThreshold := int32(8 * upm / 2048)
	if lenThreshold < 1 {
		lenThreshold = 1
	}
	lenScore := int32(6000 * upm / 2048)
	if lenScore < 1 {
		lenScore = 1
	}
	distScore := int32(3000)

	// Max width in font units for distance scoring.
	// Skrifa passes unscaled max_width from the unscaled axis metrics.
	// Use stored maxWidth (in font units) if available; fall back to computing
	// from scaled widths.
	var maxWidth int32
	if axis.maxWidth > 0 {
		maxWidth = axis.maxWidth
	} else if len(axis.widths) > 0 && axis.scale > 0 {
		maxWidth = int32(float64(axis.widths[len(axis.widths)-1].scaled) / axis.scale)
	}

	// Compare each segment to all others (O(n^2)).
	// Skrifa only iterates seg1 in the major direction, but FreeType iterates all.
	// We follow FreeType/skrifa default: seg1 must have opposing dir to seg2.
	for i := range segments {
		seg1 := &segments[i]
		pos1 := int32(seg1.pos)

		for j := range segments {
			seg2 := &segments[j]
			pos2 := int32(seg2.pos)

			// Must have opposing directions and seg2 must be to the right/above.
			if seg1.dir+seg2.dir != 0 || pos2 <= pos1 {
				continue
			}

			// Compute overlap along the minor axis (font units).
			overlapMin := int32(seg1.minCoord)
			if int32(seg2.minCoord) > overlapMin {
				overlapMin = int32(seg2.minCoord)
			}
			overlapMax := int32(seg1.maxCoord)
			if int32(seg2.maxCoord) < overlapMax {
				overlapMax = int32(seg2.maxCoord)
			}

			overlap := overlapMax - overlapMin
			if overlap < lenThreshold {
				continue
			}

			// Compute score: lower is better (all in font units / integer).
			// Matches skrifa link_segments_default score computation.
			dist := pos2 - pos1

			var distDemerit int32
			if maxWidth != 0 {
				// Distance demerits are based on multiples of max_width.
				// delta = (dist << 10) / max_width - (1 << 10)
				delta := (dist<<10)/maxWidth - (1 << 10)
				if delta > 10000 {
					distDemerit = 32000
				} else if delta > 0 {
					distDemerit = delta * delta / distScore
				}
			} else {
				distDemerit = dist
			}

			score := float32(distDemerit) + float32(lenScore)/float32(overlap)

			if score < seg1.score {
				seg1.score = score
				seg1.linkIdx = int16(j)
			}
			if score < seg2.score {
				seg2.score = score
				seg2.linkIdx = int16(i)
			}
		}
	}

	// Identify serifs: if seg1->seg2 but seg2->seg3 (not seg1),
	// then seg1 is a serif of seg3.
	for i := range segments {
		seg := &segments[i]
		if seg.linkIdx < 0 {
			continue
		}
		link := &segments[seg.linkIdx]
		if link.linkIdx != int16(i) {
			seg.serifIdx = link.linkIdx
			seg.linkIdx = -1
		}
	}
}

// linkSegmentsCJK links opposing segments into stems for CJK script group.
// Key differences from Default:
//   - Score = raw distance (not distance_demerit + len_score/len)
//   - Match acceptance uses (dist*8 < seg.score*9) && (dist*8 < seg.score*7 || seg.segLen < len)
//   - Serif detection uses dist_threshold = fixed_div(64*3, scale) and complex containment checks
//
// See FreeType afcjk.c:848 af_cjk_hints_link_segments.
// See skrifa topo/segments.rs link_segments_cjk.
//
//nolint:gocognit,gocyclo,cyclop,funlen // FreeType afcjk.c port — algorithmic complexity is inherent
func linkSegmentsCJK(segments []hintSegment, axis *scaledAxisMetrics) {
	if len(segments) < 2 {
		return
	}

	upm := axis.unitsPerEm
	if upm <= 0 {
		upm = 1000
	}

	// Heuristic value to set up a minimum for overlapping.
	// derived_constant(UPM, 8) = 8 * UPM / 2048
	lenThreshold := int32(8 * upm / 2048)
	if lenThreshold < 1 {
		lenThreshold = 1
	}

	// dist_threshold for serif detection: fixed_div(64*3, scale)
	// scale is the axis scale in 16.16 fixed-point.
	// fixed_div(a, b) = (a << 16) / b
	var distThreshold int32
	if axis.scale16dot16 > 0 {
		distThreshold = int32((int64(64*3) << 16) / int64(axis.scale16dot16))
	} else {
		distThreshold = 32000 // effectively disabled
	}

	// Compare each segment to the others (O(n^2)).
	// CJK: seg1 must have the major direction.
	// Skrifa: if seg1.dir != axis.major_dir { continue }
	// See skrifa topo/segments.rs:168 link_segments_cjk.
	for ix1 := range segments {
		seg1 := &segments[ix1]
		if seg1.dir != axis.majorDir {
			continue
		}
		pos1 := int32(seg1.pos)

		for ix2 := range segments {
			if ix1 == ix2 {
				continue
			}
			seg2 := &segments[ix2]
			// Must have opposing directions.
			if seg1.dir+seg2.dir != 0 {
				continue
			}
			pos2 := int32(seg2.pos)
			dist := pos2 - pos1
			if dist < 0 {
				continue
			}

			// Compute overlap along the minor axis.
			overlapMin := int32(seg1.minCoord)
			if int32(seg2.minCoord) > overlapMin {
				overlapMin = int32(seg2.minCoord)
			}
			overlapMax := int32(seg1.maxCoord)
			if int32(seg2.maxCoord) < overlapMax {
				overlapMax = int32(seg2.maxCoord)
			}
			overlapLen := overlapMax - overlapMin
			if overlapLen < lenThreshold {
				continue
			}

			// CJK acceptance check for seg1.
			// Accept if: (dist*8 < seg.score*9) && (dist*8 < seg.score*7 || seg.segLen < overlapLen)
			checkAndUpdate := func(seg *hintSegment, linkIdx int) {
				score := int32(seg.score)
				if (dist*8 < score*9) && (dist*8 < score*7 || seg.segLen < overlapLen) {
					seg.score = float32(dist)
					seg.segLen = overlapLen
					seg.linkIdx = int16(linkIdx)
				}
			}
			checkAndUpdate(seg1, ix2)

			// Reload seg2 pointer since seg1 may have been updated.
			seg2 = &segments[ix2]
			checkAndUpdate(seg2, ix1)
		}
	}

	// CJK serif detection.
	// See skrifa topo/segments.rs:213-276 (link_segments_cjk serif pass).
	for ix1 := range segments {
		seg1 := segments[ix1]
		if int32(seg1.score) >= distThreshold {
			continue
		}
		if seg1.linkIdx < 0 {
			continue
		}
		link1Idx := int(seg1.linkIdx)
		link1 := segments[link1Idx]
		if link1.linkIdx != int16(ix1) || link1.pos <= seg1.pos {
			continue
		}

		for ix2 := range segments {
			seg2 := segments[ix2]
			if seg2.pos > seg1.pos || ix1 == ix2 {
				continue
			}
			if seg2.linkIdx < 0 {
				continue
			}
			link2Idx := int(seg2.linkIdx)
			link2 := segments[link2Idx]
			if link2.linkIdx != int16(ix2) || link2.pos < link1.pos {
				continue
			}
			if seg1.pos == seg2.pos && link1.pos == link2.pos {
				continue
			}
			if int32(seg2.score) <= int32(seg1.score) || int32(seg1.score)*4 <= int32(seg2.score) {
				continue
			}

			if seg1.segLen >= seg2.segLen*3 {
				// Remap links: segments linked to seg2 get serif to link1,
				// segments linked to link2 get serif to seg1.
				for si := range segments {
					s := &segments[si]
					if s.linkIdx == int16(ix2) {
						s.linkIdx = -1
						s.serifIdx = int16(link1Idx)
					} else if s.linkIdx == int16(link2Idx) {
						s.linkIdx = -1
						s.serifIdx = int16(ix1)
					}
				}
			} else {
				segments[ix1].linkIdx = -1
				segments[link1Idx].linkIdx = -1
				break
			}
		}
	}

	// Final serif pass: unreciprocated links become serifs.
	for ix1 := range segments {
		seg1 := segments[ix1]
		if seg1.linkIdx < 0 {
			continue
		}
		seg2 := segments[seg1.linkIdx]
		if seg2.linkIdx != int16(ix1) {
			segments[ix1].linkIdx = -1
			if int32(seg2.score) < distThreshold || int32(seg1.score) < int32(seg2.score)*4 {
				segments[ix1].serifIdx = seg2.linkIdx
			}
		}
	}
}

// fontU returns the font-unit coordinate along the major axis for a given dimension.
// This is used by alignStrongPoints for the "fpos" lookup (matching edge.fpos
// which is also in font units for the contour path).
func (p *hintPoint) fontU(dim hintDimension) float32 {
	if dim == dimHorizontal {
		return p.fx
	}
	return p.fy
}

// origU returns the original scaled coordinate (26.6) along the major axis.
// This is used by alignStrongPoints for interpolation in scaled space.
func (p *hintPoint) origU(dim hintDimension) int32 {
	if dim == dimHorizontal {
		return p.ox
	}
	return p.oy
}
