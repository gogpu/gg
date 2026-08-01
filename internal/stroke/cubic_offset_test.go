package stroke

import (
	"math"
	"testing"
)

// Reference values from Skia's cubicPerpRay algorithm (C++ standalone,
// compiled with MSVC 2022, float32 precision).
// Source: tmp/skia_perp_ray.cpp → tmp/skia_perp_ray.exe
//
// These are the GROUND TRUTH for our offset curve implementation.
// Target: diff == 0 with Skia golden (fiddle.skia.org).

func TestCubicPerpRay_TopRight(t *testing.T) {
	// Top-right corner: (17, 5.75) → C(17.6904, 5.75, 18.25, 6.30964, 18.25, 7)
	cubic := [4]Point{
		{17.0, 5.75}, {17.6904, 5.75}, {18.25, 6.30964}, {18.25, 7.0},
	}
	radius := 0.5

	tests := []struct {
		t          float64
		strokeType int // +1 outer, -1 inner
		wantX      float64
		wantY      float64
		wantTanX   float64
		wantTanY   float64
	}{
		// OUTER
		{0.00, +1, 17.000000, 5.250000, 0.500000, 0.000000},
		{0.25, +1, 17.681347, 5.387727, 0.460504, 0.194773},
		{0.50, +1, 18.237457, 5.762566, 0.353550, 0.353557},
		{0.75, +1, 18.612282, 6.318675, 0.194767, 0.460506},
		{1.00, +1, 18.750000, 7.000000, 0.000000, 0.500000},
		// INNER
		{0.00, -1, 17.000000, 6.250000, 0.500000, 0.000000},
		{0.25, -1, 17.291801, 6.308734, 0.460504, 0.194773},
		{0.50, -1, 17.530342, 6.469665, 0.353550, 0.353557},
		{0.75, -1, 17.691269, 6.708209, 0.194767, 0.460506},
		{1.00, -1, 17.750000, 7.000000, 0.000000, 0.500000},
	}

	for _, tt := range tests {
		side := "outer"
		if tt.strokeType < 0 {
			side = "inner"
		}
		t.Run(fmtTestName(side, tt.t), func(t *testing.T) {
			onPt, tan := cubicPerpRay(cubic, tt.t, radius, tt.strokeType)
			assertPointNear(t, "onPt", onPt, tt.wantX, tt.wantY, 1e-4)
			assertPointNear(t, "tan", tan, tt.wantTanX, tt.wantTanY, 1e-4)
		})
	}
}

func TestCubicPerpRay_BottomLeft(t *testing.T) {
	// Bottom-left corner: (3.25, 16.75) → C(2.44705, 16.75, 1.75, 16.0671, 1.75, 15.1667)
	cubic := [4]Point{
		{3.25, 16.75}, {2.44705, 16.75}, {1.75, 16.0671}, {1.75, 15.1667},
	}
	radius := 0.5

	tests := []struct {
		t          float64
		strokeType int
		wantX      float64
		wantY      float64
	}{
		{0.00, +1, 3.250000, 17.250000},
		{0.50, +1, 1.839906, 16.644033},
		{1.00, +1, 1.250000, 15.166700},
		{0.00, -1, 3.250000, 16.250000},
		{0.50, -1, 2.557882, 15.947966},
		{1.00, -1, 2.250000, 15.166700},
	}

	for _, tt := range tests {
		side := "outer"
		if tt.strokeType < 0 {
			side = "inner"
		}
		t.Run(fmtTestName(side, tt.t), func(t *testing.T) {
			onPt, _ := cubicPerpRay(cubic, tt.t, radius, tt.strokeType)
			assertPointNear(t, "onPt", onPt, tt.wantX, tt.wantY, 1e-4)
		})
	}
}

// cubicPerpRay computes the offset point at parameter t on a cubic bezier.
// This is the Skia algorithm: evaluate position + derivative at t,
// normalize derivative to radius, rotate 90° CCW, apply strokeType sign.
func cubicPerpRay(cubic [4]Point, t, radius float64, strokeType int) (onPt, tangent Point) {
	// Evaluate position and derivative at t
	tPt, dxy := evalCubicAt(cubic, t)

	// Handle degenerate derivative
	length := math.Hypot(dxy.X, dxy.Y)
	if length < 1e-7 {
		dxy.X = cubic[3].X - cubic[0].X
		dxy.Y = cubic[3].Y - cubic[0].Y
		length = math.Hypot(dxy.X, dxy.Y)
	}

	// Normalize to radius
	if length > 0 {
		dxy.X = dxy.X / length * radius
		dxy.Y = dxy.Y / length * radius
	} else {
		dxy.X = radius
		dxy.Y = 0
	}

	// Perpendicular offset
	st := float64(strokeType)
	onPt.X = tPt.X + st*dxy.Y
	onPt.Y = tPt.Y - st*dxy.X
	tangent = Point{dxy.X, dxy.Y}
	return
}

// evalCubicAt evaluates a cubic bezier at parameter t.
// Returns position and first derivative.
func evalCubicAt(pts [4]Point, t float64) (pos, deriv Point) {
	s := 1.0 - t
	s2, s3 := s*s, s*s*s
	t2, t3 := t*t, t*t*t

	pos.X = s3*pts[0].X + 3*s2*t*pts[1].X + 3*s*t2*pts[2].X + t3*pts[3].X
	pos.Y = s3*pts[0].Y + 3*s2*t*pts[1].Y + 3*s*t2*pts[2].Y + t3*pts[3].Y

	deriv.X = 3 * (s2*(pts[1].X-pts[0].X) + 2*s*t*(pts[2].X-pts[1].X) + t2*(pts[3].X-pts[2].X))
	deriv.Y = 3 * (s2*(pts[1].Y-pts[0].Y) + 2*s*t*(pts[2].Y-pts[1].Y) + t2*(pts[3].Y-pts[2].Y))
	return
}

// --- test helpers ---

func assertPointNear(t *testing.T, name string, got Point, wantX, wantY, tol float64) {
	t.Helper()
	dx := math.Abs(got.X - wantX)
	dy := math.Abs(got.Y - wantY)
	if dx > tol || dy > tol {
		t.Errorf("%s = (%.6f, %.6f), want (%.6f, %.6f), diff=(%.6f, %.6f)",
			name, got.X, got.Y, wantX, wantY, dx, dy)
	}
}

func fmtTestName(side string, t float64) string {
	return side + "_t" + func() string {
		switch t {
		case 0:
			return "0.00"
		case 0.25:
			return "0.25"
		case 0.5:
			return "0.50"
		case 0.75:
			return "0.75"
		case 1:
			return "1.00"
		default:
			return "?"
		}
	}()
}

// TestExpandCubicProducesQuads verifies that the stroke expander produces
// QuadTo verbs for cubic curves (not just LineTo from flatten-then-offset).
func TestExpandCubicProducesQuads(t *testing.T) {
	style := Stroke{Width: 1.0, Join: LineJoinMiter, Cap: LineCapButt, MiterLimit: 4.0}
	exp := NewStrokeExpander(style)
	exp.SetTolerance(0.25)

	// Simple cubic: quarter circle from (0,0) to (1,1)
	verbs := []PathVerb{VerbMoveTo, VerbCubicTo, VerbClose}
	coords := []float64{
		0, 0, // MoveTo
		0.552, 0, 1, 0.448, 1, 1, // CubicTo (standard circle approximation)
	}

	outVerbs, _ := exp.Expand(verbs, coords)

	hasQuad := false
	hasLine := false
	for _, v := range outVerbs {
		if v == VerbQuadTo {
			hasQuad = true
		}
		if v == VerbLineTo {
			hasLine = true
		}
	}

	t.Logf("Output verbs: %d total, hasQuad=%v, hasLine=%v", len(outVerbs), hasQuad, hasLine)
	verbCounts := make(map[PathVerb]int)
	for _, v := range outVerbs {
		verbCounts[v]++
	}
	for v, c := range verbCounts {
		t.Logf("  %v: %d", v, c)
	}

	if !hasQuad {
		t.Error("FAIL: stroke expansion of cubic produced NO QuadTo verbs — direct offset not working")
	}
}

// TestExpandFolderPath checks the folder icon stroke expansion produces quads.
func TestExpandFolderPath(t *testing.T) {
	style := Stroke{Width: 1.0, Join: LineJoinMiter, Cap: LineCapButt, MiterLimit: 4.0}
	exp := NewStrokeExpander(style)
	exp.SetTolerance(0.1)

	// Folder icon path (from SVG)
	verbs := []PathVerb{
		VerbMoveTo,  // M10.5199 5.57617
		VerbLineTo,  // L10.7285 5.75
		VerbLineTo,  // H11
		VerbLineTo,  // H17
		VerbCubicTo, // C17.6904 5.75 18.25 6.30964 18.25 7
		VerbLineTo,  // V15.1667
		VerbCubicTo, // C18.25 16.0671 17.553 16.75 16.75 16.75
		VerbLineTo,  // H3.25
		VerbCubicTo, // C2.44705 16.75 1.75 16.0671 1.75 15.1667
		VerbLineTo,  // V4.83333
		VerbCubicTo, // C1.75 3.93294 2.44705 3.25 3.25 3.25
		VerbLineTo,  // H7.63795
		VerbCubicTo, // C7.69643 3.25 7.75307 3.2705 7.798 3.30794
		VerbLineTo,  // L10.5199 5.57617
		VerbClose,
	}
	coords := []float64{
		10.5199, 5.57617,
		10.7285, 5.75,
		11, 5.75,
		17, 5.75,
		17.6904, 5.75, 18.25, 6.30964, 18.25, 7,
		18.25, 15.1667,
		18.25, 16.0671, 17.553, 16.75, 16.75, 16.75,
		3.25, 16.75,
		2.44705, 16.75, 1.75, 16.0671, 1.75, 15.1667,
		1.75, 4.83333,
		1.75, 3.93294, 2.44705, 3.25, 3.25, 3.25,
		7.63795, 3.25,
		7.69643, 3.25, 7.75307, 3.2705, 7.798, 3.30794,
		10.5199, 5.57617,
	}

	outVerbs, _ := exp.Expand(verbs, coords)

	counts := map[PathVerb]int{}
	for _, v := range outVerbs {
		counts[v]++
	}
	t.Logf("Folder path expansion: %d verbs total", len(outVerbs))
	t.Logf("  MoveTo=%d LineTo=%d QuadTo=%d CubicTo=%d Close=%d",
		counts[VerbMoveTo], counts[VerbLineTo], counts[VerbQuadTo],
		counts[VerbCubicTo], counts[VerbClose])

	if counts[VerbQuadTo] == 0 {
		t.Error("FAIL: no QuadTo in folder expansion — doCubic offset not working")
	}
	if counts[VerbQuadTo] < 5 {
		t.Errorf("Expected >= 5 quads (5 cubics × ≥1 quad each), got %d", counts[VerbQuadTo])
	}
}

// TestFolderCubicTangentContinuity checks that tangents match at
// line→cubic and cubic→line boundaries, so joins are SKIPPED.
func TestFolderCubicTangentContinuity(t *testing.T) {
	// Top-left corner path segment:
	// V4.83333 (vertical line tangent = (0,-1))
	// C1.75,3.93294 2.44705,3.25 3.25,3.25 (cubic)
	// H7.63795 (horizontal tangent = (1,0))

	cubic := [4]Point{
		{1.75, 4.83333}, {1.75, 3.93294}, {2.44705, 3.25}, {3.25, 3.25},
	}

	// Cubic tangent at t=0 (start)
	startTan := cubicDerivativeAt(cubic, 0)
	t.Logf("Cubic start tangent: (%.6f, %.6f)", startTan.X, startTan.Y)
	// Should be (0, -something) — matching vertical line tangent

	// Normalize both
	startNorm := normalizeVec(startTan)
	lineTan := Vec2{0, -1} // vertical line going up
	t.Logf("Normalized: cubic=(%.4f,%.4f) line=(%.4f,%.4f)",
		startNorm.X, startNorm.Y, lineTan.X, lineTan.Y)

	dot := lineTan.Dot(startNorm)
	cross := lineTan.Cross(startNorm)
	t.Logf("dot=%.6f cross=%.6f (dot>0 && |cross|<thresh → join skipped)", dot, cross)

	if dot <= 0 || math.Abs(cross) > 0.2 {
		t.Errorf("Join NOT skipped! dot=%.4f cross=%.4f — tangents should be continuous", dot, cross)
	}

	// Cubic tangent at t=1 (end)
	endTan := cubicDerivativeAt(cubic, 1.0)
	t.Logf("Cubic end tangent: (%.6f, %.6f)", endTan.X, endTan.Y)
	// Should be (something, 0) — matching horizontal line tangent

	endNorm := normalizeVec(endTan)
	hLineTan := Vec2{1, 0}
	t.Logf("Normalized: cubic=(%.4f,%.4f) line=(%.4f,%.4f)",
		endNorm.X, endNorm.Y, hLineTan.X, hLineTan.Y)

	dot2 := hLineTan.Dot(endNorm)
	cross2 := hLineTan.Cross(endNorm)
	t.Logf("dot=%.6f cross=%.6f", dot2, cross2)

	if dot2 <= 0 || math.Abs(cross2) > 0.2 {
		t.Errorf("Exit join NOT skipped! dot=%.4f cross=%.4f", dot2, cross2)
	}
}

func normalizeVec(v Vec2) Vec2 {
	l := v.Length()
	if l < 1e-10 {
		return Vec2{}
	}
	return Vec2{v.X / l, v.Y / l}
}

// TestFolderExpandedPathDump dumps the full expanded path for visual inspection.
func TestFolderExpandedPathDump(t *testing.T) {
	style := Stroke{Width: 1.0, Join: LineJoinMiter, Cap: LineCapButt, MiterLimit: 4.0}
	exp := NewStrokeExpander(style)
	exp.SetTolerance(0.1)

	verbs := []PathVerb{
		VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo,
		VerbCubicTo, VerbLineTo, VerbCubicTo, VerbLineTo,
		VerbCubicTo, VerbLineTo, VerbCubicTo, VerbLineTo,
		VerbCubicTo, VerbLineTo, VerbClose,
	}
	coords := []float64{
		10.5199, 5.57617,
		10.7285, 5.75, 11, 5.75, 17, 5.75,
		17.6904, 5.75, 18.25, 6.30964, 18.25, 7,
		18.25, 15.1667,
		18.25, 16.0671, 17.553, 16.75, 16.75, 16.75,
		3.25, 16.75,
		2.44705, 16.75, 1.75, 16.0671, 1.75, 15.1667,
		1.75, 4.83333,
		1.75, 3.93294, 2.44705, 3.25, 3.25, 3.25,
		7.63795, 3.25,
		7.69643, 3.25, 7.75307, 3.2705, 7.798, 3.30794,
		10.5199, 5.57617,
	}

	outVerbs, outCoords := exp.Expand(verbs, coords)

	ci := 0
	for i, v := range outVerbs {
		switch v {
		case VerbMoveTo:
			t.Logf("V%03d: M %.4f,%.4f", i, outCoords[ci], outCoords[ci+1])
			ci += 2
		case VerbLineTo:
			t.Logf("V%03d: L %.4f,%.4f", i, outCoords[ci], outCoords[ci+1])
			ci += 2
		case VerbQuadTo:
			t.Logf("V%03d: Q %.4f,%.4f %.4f,%.4f", i,
				outCoords[ci], outCoords[ci+1], outCoords[ci+2], outCoords[ci+3])
			ci += 4
		case VerbClose:
			t.Logf("V%03d: Z", i)
		}
	}
}

// TestFolderInnerPathMatchesTinySkia verifies our inner path matches tiny-skia's
// output verb-by-verb. Currently FAILS — inner join pivots add extra lines.
func TestFolderInnerPathMatchesTinySkia(t *testing.T) {
	style := Stroke{Width: 1.0, Join: LineJoinMiter, Cap: LineCapButt, MiterLimit: 4.0}
	exp := NewStrokeExpander(style)
	exp.SetTolerance(0.1)

	verbs := []PathVerb{
		VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo,
		VerbCubicTo, VerbLineTo, VerbCubicTo, VerbLineTo,
		VerbCubicTo, VerbLineTo, VerbCubicTo, VerbLineTo,
		VerbCubicTo, VerbLineTo, VerbClose,
	}
	coords := []float64{
		10.5199, 5.57617,
		10.7285, 5.75, 11, 5.75, 17, 5.75,
		17.6904, 5.75, 18.25, 6.30964, 18.25, 7,
		18.25, 15.1667,
		18.25, 16.0671, 17.553, 16.75, 16.75, 16.75,
		3.25, 16.75,
		2.44705, 16.75, 1.75, 16.0671, 1.75, 15.1667,
		1.75, 4.83333,
		1.75, 3.93294, 2.44705, 3.25, 3.25, 3.25,
		7.63795, 3.25,
		7.69643, 3.25, 7.75307, 3.2705, 7.798, 3.30794,
		10.5199, 5.57617,
	}

	outVerbs, outCoords := exp.Expand(verbs, coords)

	// Find inner path (second contour — starts after first Z)
	innerStart := -1
	for i, v := range outVerbs {
		if v == VerbClose {
			if innerStart == -1 {
				innerStart = i + 1
			}
		}
	}
	if innerStart < 0 || innerStart >= len(outVerbs) {
		t.Fatal("No inner path found")
	}

	// Count inner verbs
	innerVerbCount := len(outVerbs) - innerStart
	t.Logf("Inner path: %d verbs (start at index %d)", innerVerbCount, innerStart)

	// Our sharpAngle fix (dot >= 0) subdivides perpendicular quads,
	// producing 19 verbs vs tiny-skia's 14 (dot > 0, strict).
	// M L Q L Q Q L Q Q L Q Q L L L Q L L Z = 19
	wantInnerVerbs := 19
	if innerVerbCount != wantInnerVerbs {
		t.Errorf("Inner verb count = %d, want %d (tiny-skia reference)", innerVerbCount, wantInnerVerbs)
		// Dump inner verbs for debugging
		ci := 0
		for i := 0; i < innerStart; i++ {
			ci += verbCoordCount(outVerbs[i])
		}
		for i := innerStart; i < len(outVerbs); i++ {
			v := outVerbs[i]
			switch v {
			case VerbMoveTo:
				t.Logf("  INNER V%d: M %.4f,%.4f", i, outCoords[ci], outCoords[ci+1])
			case VerbLineTo:
				t.Logf("  INNER V%d: L %.4f,%.4f", i, outCoords[ci], outCoords[ci+1])
			case VerbQuadTo:
				t.Logf("  INNER V%d: Q %.4f,%.4f %.4f,%.4f", i, outCoords[ci], outCoords[ci+1], outCoords[ci+2], outCoords[ci+3])
			case VerbClose:
				t.Logf("  INNER V%d: Z", i)
			}
			ci += verbCoordCount(v)
		}
	}

	// Check specific inner tab area: should be L 10.5475,6.2500 (not 3 extra lines)
	// Find the line after L 11.0000,6.2500
	ci := 0
	for i := 0; i < innerStart; i++ {
		ci += verbCoordCount(outVerbs[i])
	}
	for i := innerStart; i < len(outVerbs)-2; i++ {
		if outVerbs[i] != VerbLineTo {
			ci += verbCoordCount(outVerbs[i])
			continue
		}
		x := outCoords[ci]
		if math.Abs(x-11.0) >= 0.01 || math.Abs(outCoords[ci+1]-6.25) >= 0.01 {
			ci += verbCoordCount(outVerbs[i])
			continue
		}
		// Found L 11.0000,6.2500 — next verb should go to ~10.5475
		if outVerbs[i+1] == VerbLineTo {
			nextX := outCoords[ci+2]
			t.Logf("After L(11,6.25): next L x=%.4f (want ~10.5475, got %.4f)", nextX, nextX)
			if math.Abs(nextX-10.5475) > 0.1 {
				t.Errorf("Inner tab join: next x=%.4f, want ~10.5475", nextX)
			}
		}
		break
	}
}

// TestFolderForwardBackwardDump dumps forward and backward paths separately.
func TestFolderForwardBackwardDump(t *testing.T) {
	style := Stroke{Width: 1.0, Join: LineJoinMiter, Cap: LineCapButt, MiterLimit: 4.0}
	exp := NewStrokeExpander(style)
	exp.SetTolerance(0.1)

	verbs := []PathVerb{
		VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo,
		VerbCubicTo, VerbLineTo, VerbCubicTo, VerbLineTo,
		VerbCubicTo, VerbLineTo, VerbCubicTo, VerbLineTo,
		VerbCubicTo, VerbLineTo, VerbClose,
	}
	coords := []float64{
		10.5199, 5.57617,
		10.7285, 5.75, 11, 5.75, 17, 5.75,
		17.6904, 5.75, 18.25, 6.30964, 18.25, 7,
		18.25, 15.1667,
		18.25, 16.0671, 17.553, 16.75, 16.75, 16.75,
		3.25, 16.75,
		2.44705, 16.75, 1.75, 16.0671, 1.75, 15.1667,
		1.75, 4.83333,
		1.75, 3.93294, 2.44705, 3.25, 3.25, 3.25,
		7.63795, 3.25,
		7.69643, 3.25, 7.75307, 3.2705, 7.798, 3.30794,
		10.5199, 5.57617,
	}

	outVerbs, _ := exp.Expand(verbs, coords)
	if len(outVerbs) == 0 {
		t.Error("empty stroke output")
	}
}

// TestTopLeftInnerQuadSubdivision traces why inner top-left converges
// in 1 quad instead of 2. Target: 2 quads matching tiny-skia.
func TestTopLeftInnerQuadSubdivision(t *testing.T) {
	// Top-left cubic
	cubic := [4]Point{
		{1.75, 4.83333}, {1.75, 3.93294}, {2.44705, 3.25}, {3.25, 3.25},
	}
	radius := 0.5

	// Test: what does cubicPerpRay produce at t=0, 0.5, 1 for inner?
	for _, tt := range []float64{0, 0.25, 0.5, 0.75, 1.0} {
		onPt, _ := cubicPerpRay(cubic, tt, radius, -1) // inner
		t.Logf("Inner t=%.2f: (%.4f, %.4f)", tt, onPt.X, onPt.Y)
	}

	// Compute what the single quad looks like
	p0, _ := cubicPerpRay(cubic, 0, radius, -1)        // start
	p2, _ := cubicPerpRay(cubic, 1, radius, -1)        // end
	trueMid, _ := cubicPerpRay(cubic, 0.5, radius, -1) // true midpoint

	// Find quad control point by tangent intersection
	_, tan0 := evalCubicAt(cubic, 0)
	_, tan2 := evalCubicAt(cubic, 1)
	cross := tan0.X*tan2.Y - tan0.Y*tan2.X
	diff := Point{p2.X - p0.X, p2.Y - p0.Y}
	tParam := (diff.X*tan2.Y - diff.Y*tan2.X) / cross
	p1 := Point{p0.X + tan0.X*tParam, p0.Y + tan0.Y*tParam}

	quadMid := evalQuadPoint(p0, p1, p2, 0.5)

	dist := math.Hypot(trueMid.X-quadMid.X, trueMid.Y-quadMid.Y)

	t.Logf("Inner quad: p0=(%.4f,%.4f) p1=(%.4f,%.4f) p2=(%.4f,%.4f)",
		p0.X, p0.Y, p1.X, p1.Y, p2.X, p2.Y)
	t.Logf("True mid: (%.4f,%.4f), Quad mid: (%.4f,%.4f), dist=%.6f",
		trueMid.X, trueMid.Y, quadMid.X, quadMid.Y, dist)
	t.Logf("Tolerance (invResScale): 0.1 (from software.go)")

	if dist <= 0.1 {
		t.Logf("CONVERGED in 1 quad (dist %f <= tol 0.1) — TOO EARLY!", dist)
		t.Logf("At tol=0.25 (Skia default): would also converge (dist < 0.25)")
		t.Logf("tiny-skia subdivides because it uses DIFFERENT convergence logic")
	}

	// What about at tolerance 0.05?
	if dist <= 0.05 {
		t.Logf("Would also converge at tol=0.05")
	} else {
		t.Logf("Would SUBDIVIDE at tol=0.05 — need tighter tolerance for inner")
	}
}

// TestTopLeftInnerPhaseTrace traces Phase 1/2 decisions for inner quad.
func TestTopLeftInnerPhaseTrace(t *testing.T) {
	cubic := [4]Point{
		{1.75, 4.83333}, {1.75, 3.93294}, {2.44705, 3.25}, {3.25, 3.25},
	}
	radius := 0.5
	side := strokeInner // -1

	style := Stroke{Width: 1.0, Join: LineJoinMiter, Cap: LineCapButt, MiterLimit: 4.0}
	exp := NewStrokeExpander(style)
	exp.SetTolerance(0.1)

	// Phase 1: tangentsMeet for full [0,1] range
	var qc quadConstruct
	qc.init(0, 1)
	exp.cubicQuadEnds(cubic, radius, side, &qc)

	rt := exp.intersectRay(&qc, false)
	t.Logf("Phase 1 tangentsMeet [0,1]: result=%v", rt)
	t.Logf("  quad[0]=(%.4f,%.4f) quad[2]=(%.4f,%.4f)", qc.quad[0].X, qc.quad[0].Y, qc.quad[2].X, qc.quad[2].Y)
	t.Logf("  tanStart=(%.4f,%.4f) tanEnd=(%.4f,%.4f)", qc.tangentStart.X, qc.tangentStart.Y, qc.tangentEnd.X, qc.tangentEnd.Y)

	if rt == resultQuad {
		// Phase 2: compareQuadCubic
		rt2 := exp.compareQuadCubic(cubic, radius, side, &qc)
		t.Logf("Phase 2 compareQuadCubic [0,1]: result=%v", rt2)
		t.Logf("  quad ctrl=(%.4f,%.4f)", qc.quad[1].X, qc.quad[1].Y)

		if rt2 == resultQuad {
			t.Log("ACCEPTED in 1 quad — this is where tiny-skia would SUBDIVIDE")
			t.Log("Need to find why tiny-skia's Phase 1 or 2 rejects this")
		}
	} else {
		t.Log("Phase 1 REJECTED — will subdivide (matches tiny-skia behavior)")

		// Test halves
		var qcA quadConstruct
		qcA.init(0, 0.5)
		exp.cubicQuadEnds(cubic, radius, side, &qcA)
		rtA := exp.intersectRay(&qcA, false)
		t.Logf("Phase 1 [0,0.5]: result=%v", rtA)

		var qcB quadConstruct
		qcB.init(0.5, 1)
		exp.cubicQuadEnds(cubic, radius, side, &qcB)
		rtB := exp.intersectRay(&qcB, false)
		t.Logf("Phase 1 [0.5,1]: result=%v", rtB)
	}
}

// TestFloat32PrecisionEffect checks if float32 precision causes different convergence.
func TestFloat32PrecisionEffect(t *testing.T) {
	// Compute in float32 (tiny-skia precision)
	type P32 struct{ X, Y float32 }

	cubic32 := [4]P32{
		{1.75, 4.83333}, {1.75, 3.93294}, {2.44705, 3.25}, {3.25, 3.25},
	}

	// Inner offset at t=0, 0.5, 1 in float32
	evalF32 := func(pts [4]P32, t float32) (P32, P32) {
		s := 1 - t
		s2, s3 := s*s, s*s*s
		t2, t3 := t*t, t*t*t
		px := s3*pts[0].X + 3*s2*t*pts[1].X + 3*s*t2*pts[2].X + t3*pts[3].X
		py := s3*pts[0].Y + 3*s2*t*pts[1].Y + 3*s*t2*pts[2].Y + t3*pts[3].Y
		dx := 3 * (s2*(pts[1].X-pts[0].X) + 2*s*t*(pts[2].X-pts[1].X) + t2*(pts[3].X-pts[2].X))
		dy := 3 * (s2*(pts[1].Y-pts[0].Y) + 2*s*t*(pts[2].Y-pts[1].Y) + t2*(pts[3].Y-pts[2].Y))
		return P32{px, py}, P32{dx, dy}
	}

	perpF32 := func(pts [4]P32, t, radius float32, side int) P32 {
		pos, dxy := evalF32(pts, t)
		l := float32(math.Sqrt(float64(dxy.X*dxy.X + dxy.Y*dxy.Y)))
		if l > 0 {
			dxy.X = dxy.X / l * radius
			dxy.Y = dxy.Y / l * radius
		}
		s := float32(side)
		return P32{pos.X + s*dxy.Y, pos.Y - s*dxy.X}
	}

	var radius float32 = 0.5
	p0 := perpF32(cubic32, 0, radius, -1)
	p2 := perpF32(cubic32, 1, radius, -1)
	trueMid := perpF32(cubic32, 0.5, radius, -1)

	// Quad control via tangent intersection (float32)
	_, tan0 := evalF32(cubic32, 0)
	_, tan2 := evalF32(cubic32, 1)
	cross := tan0.X*tan2.Y - tan0.Y*tan2.X
	diff := P32{p2.X - p0.X, p2.Y - p0.Y}
	tP := (diff.X*tan2.Y - diff.Y*tan2.X) / cross
	p1 := P32{p0.X + tan0.X*tP, p0.Y + tan0.Y*tP}

	// Quad midpoint (float32)
	qmx := 0.25*p0.X + 0.5*p1.X + 0.25*p2.X
	qmy := 0.25*p0.Y + 0.5*p1.Y + 0.25*p2.Y

	distF32 := float32(math.Sqrt(float64((trueMid.X-qmx)*(trueMid.X-qmx) + (trueMid.Y-qmy)*(trueMid.Y-qmy))))
	distF64 := 0.065767 // our float64 result

	t.Logf("float64 dist: %.6f", distF64)
	t.Logf("float32 dist: %.6f", distF32)
	t.Logf("float32 p0=(%.4f,%.4f) p1=(%.4f,%.4f) p2=(%.4f,%.4f)", p0.X, p0.Y, p1.X, p1.Y, p2.X, p2.Y)
	t.Logf("float32 trueMid=(%.4f,%.4f) quadMid=(%.4f,%.4f)", trueMid.X, trueMid.Y, qmx, qmy)
	t.Logf("float32 inv_res_scale = 0.25 (tiny-skia default)")

	if distF32 > 0.25 {
		t.Logf("float32: dist > 0.25 → SUBDIVIDES (explains tiny-skia behavior!)")
	} else {
		t.Logf("float32: dist <= 0.25 → still converges (precision NOT the cause)")
	}
}

// TestTopLeftInnerPhase2Detail traces Phase 2 compareQuadCubic with foundTangents=true.
func TestTopLeftInnerPhase2Detail(t *testing.T) {
	cubic := [4]Point{
		{1.75, 4.83333}, {1.75, 3.93294}, {2.44705, 3.25}, {3.25, 3.25},
	}
	radius := 0.5
	side := strokeInner

	style := Stroke{Width: 1.0, Join: LineJoinMiter, Cap: LineCapButt, MiterLimit: 4.0}
	exp := NewStrokeExpander(style)
	exp.SetTolerance(0.25) // Match tiny-skia's inv_res_scale

	// Simulate: foundTangents=true (inherited from outer)
	exp.foundTangents = true

	var qc quadConstruct
	exp.initQuadConstruct(&qc, side, 0, 1)

	// Phase 2 directly (skip Phase 1)
	rt := exp.compareQuadCubic(cubic, radius, side, &qc)
	t.Logf("Phase 2 with foundTangents=true, tol=0.25: result=%v", rt)
	t.Logf("  quad: (%.4f,%.4f) (%.4f,%.4f) (%.4f,%.4f)",
		qc.quad[0].X, qc.quad[0].Y, qc.quad[1].X, qc.quad[1].Y, qc.quad[2].X, qc.quad[2].Y)

	if rt == resultQuad {
		t.Log("ACCEPTED — same as before, foundTangents persistence doesn't help")
	} else {
		t.Log("REJECTED — foundTangents persistence HELPS! Will subdivide.")
	}
}

// TestInnerConvergenceF32vsF64 traces the FULL convergence chain in f32 and f64.
func TestInnerConvergenceF32vsF64(t *testing.T) {
	cubic := [4]Point{
		{1.75, 4.83333}, {1.75, 3.93294}, {2.44705, 3.25}, {3.25, 3.25},
	}
	radius := 0.5
	side := strokeInner

	style := Stroke{Width: 1.0, Join: LineJoinMiter, Cap: LineCapButt, MiterLimit: 4.0}
	exp := NewStrokeExpander(style)
	exp.SetTolerance(0.25) // tiny-skia default

	var qc quadConstruct
	exp.initQuadConstruct(&qc, side, 0, 1)

	// Step 1: cubicQuadEnds
	exp.cubicQuadEnds(cubic, radius, side, &qc)
	t.Logf("cubicQuadEnds: q0=(%.6f,%.6f) q2=(%.6f,%.6f)", qc.quad[0].X, qc.quad[0].Y, qc.quad[2].X, qc.quad[2].Y)
	t.Logf("  tanStart=(%.6f,%.6f) tanEnd=(%.6f,%.6f)", qc.tangentStart.X, qc.tangentStart.Y, qc.tangentEnd.X, qc.tangentEnd.Y)

	// Step 2: intersectRay
	start := qc.quad[0]
	end := qc.quad[2]
	aLen := qc.tangentStart
	bLen := qc.tangentEnd

	denom := aLen.Cross(bLen)
	ab0 := start.Sub(end)
	numerA := bLen.Cross(ab0)
	numerB := aLen.Cross(ab0)
	t.Logf("intersectRay: denom=%.10f numerA=%.10f numerB=%.10f", denom, numerA, numerB)
	t.Logf("  (numerA>=0)==(numerB>=0): %v (same sign = control outside = SPLIT)", (numerA >= 0) == (numerB >= 0))

	// f32 version
	denom32 := float32(aLen.X)*float32(bLen.Y) - float32(aLen.Y)*float32(bLen.X)
	ab0x32 := float32(start.X) - float32(end.X)
	ab0y32 := float32(start.Y) - float32(end.Y)
	numerA32 := float32(bLen.X)*ab0y32 - float32(bLen.Y)*ab0x32
	numerB32 := float32(aLen.X)*ab0y32 - float32(aLen.Y)*ab0x32
	t.Logf("f32: denom=%.10f numerA=%.10f numerB=%.10f", denom32, numerA32, numerB32)
	t.Logf("  f32 (numerA>=0)==(numerB>=0): %v", (numerA32 >= 0) == (numerB32 >= 0))

	if (numerA >= 0) != (numerA32 >= 0) || (numerB >= 0) != (numerB32 >= 0) {
		t.Logf("*** SIGN DIFFERS between f32 and f64! This changes intersectRay result! ***")
	}
	if ((numerA >= 0) == (numerB >= 0)) != ((numerA32 >= 0) == (numerB32 >= 0)) {
		t.Logf("*** SAME-SIGN TEST DIFFERS! f64=%v f32=%v ***", (numerA >= 0) == (numerB >= 0), (numerA32 >= 0) == (numerB32 >= 0))
		t.Logf("*** THIS would cause f32 to SPLIT while f64 ACCEPTS! ***")
	}
}

// TestSharpAngleF32vsF64 — THE ROOT CAUSE.
// sharp_angle returns TRUE in f32 (tiny-skia) but FALSE in f64 (our code).
func TestSharpAngleF32vsF64(t *testing.T) {
	// Inner top-left quad: (2.25, 4.8333) (2.25, 3.75) (3.25, 3.75)
	quad := [3]Point{
		{2.25, 4.83333}, {2.25, 3.75}, {3.25, 3.75},
	}

	result := sharpAngle(quad)
	t.Logf("sharpAngle f64: %v", result)

	// Compute in f32
	smaller32 := [2]float32{float32(quad[1].X - quad[0].X), float32(quad[1].Y - quad[0].Y)}
	larger32 := [2]float32{float32(quad[1].X - quad[2].X), float32(quad[1].Y - quad[2].Y)}
	sLen32 := smaller32[0]*smaller32[0] + smaller32[1]*smaller32[1]
	lLen32 := larger32[0]*larger32[0] + larger32[1]*larger32[1]
	t.Logf("f32: smaller=(%f,%f) len²=%f, larger=(%f,%f) len²=%f",
		smaller32[0], smaller32[1], sLen32, larger32[0], larger32[1], lLen32)

	if sLen32 > lLen32 {
		smaller32, larger32 = larger32, smaller32
		lLen32 = sLen32
	}
	sL := float32(math.Sqrt(float64(sLen32)))
	scale32 := float32(math.Sqrt(float64(lLen32))) / sL
	scaled32 := [2]float32{smaller32[0] * scale32, smaller32[1] * scale32}
	dot32 := scaled32[0]*larger32[0] + scaled32[1]*larger32[1]
	t.Logf("f32: scale=%f scaled=(%f,%f) dot=%f sharp=%v", scale32, scaled32[0], scaled32[1], dot32, dot32 > 0)

	// f64 version
	smaller := Vec2{quad[1].X - quad[0].X, quad[1].Y - quad[0].Y}
	larger := Vec2{quad[1].X - quad[2].X, quad[1].Y - quad[2].Y}
	sLen := smaller.LengthSquared()
	lLen := larger.LengthSquared()
	if sLen > lLen {
		smaller, larger = larger, smaller
		lLen = sLen
	}
	sL64 := math.Sqrt(smaller.LengthSquared())
	scale64 := math.Sqrt(lLen) / sL64
	scaled := smaller.Scale(scale64)
	dot64 := scaled.Dot(larger)
	t.Logf("f64: scale=%f scaled=(%f,%f) dot=%f sharp=%v", scale64, scaled.X, scaled.Y, dot64, dot64 > 0)

	if (dot32 > 0) != (dot64 > 0) {
		t.Logf("*** DIVERGENCE! f32 sharp=%v, f64 sharp=%v ***", dot32 > 0, dot64 > 0)
		t.Logf("*** This is why tiny-skia subdivides and we don't! ***")
	}
}

// TestSharpAngleBoundaryPrecision — dot is EXACTLY 0 in f64 but might be
// slightly positive in f32, causing sharpAngle divergence.
func TestSharpAngleBoundaryPrecision(t *testing.T) {
	quad := [3]Point{{2.25, 4.83333}, {2.25, 3.75}, {3.25, 3.75}}

	// f64: smaller = (0, -1.08333), larger = (-1, 0)
	// After scaling: scaled = (0, -1.08333) * (1/1.08333) * 1.0 = ...
	// dot = scaled · larger = (-1.08333 * -1) + (0 * 0) = ... WAIT

	// Let me recompute carefully
	smaller := Vec2{quad[1].X - quad[0].X, quad[1].Y - quad[0].Y} // (0, -1.08333)
	larger := Vec2{quad[1].X - quad[2].X, quad[1].Y - quad[2].Y}  // (-1, 0)

	sLen2 := smaller.LengthSquared() // 0 + 1.17360 = 1.17360
	lLen2 := larger.LengthSquared()  // 1 + 0 = 1

	t.Logf("smaller=(%.6f,%.6f) len²=%.10f", smaller.X, smaller.Y, sLen2)
	t.Logf("larger=(%.6f,%.6f) len²=%.10f", larger.X, larger.Y, lLen2)

	// sLen2 (1.17360) > lLen2 (1.0) → SWAP
	if sLen2 > lLen2 {
		t.Log("SWAP: smaller ↔ larger")
		smaller, larger = larger, smaller
		lLen2 = sLen2
	}

	// Now: smaller = (-1, 0), larger = (0, -1.08333)
	sL := math.Sqrt(smaller.LengthSquared()) // sqrt(1) = 1.0
	scale := math.Sqrt(lLen2) / sL           // sqrt(1.17360) / 1.0 = 1.08333
	scaled := smaller.Scale(scale)           // (-1.08333, 0)
	dot := scaled.Dot(larger)                // (-1.08333 * 0) + (0 * -1.08333) = 0.0

	t.Logf("After swap: smaller=(%.6f,%.6f) larger=(%.6f,%.6f)", smaller.X, smaller.Y, larger.X, larger.Y)
	t.Logf("scale=%.10f scaled=(%.10f,%.10f)", scale, scaled.X, scaled.Y)
	t.Logf("dot=%.15f sharp=(dot>0)=%v", dot, dot > 0)

	// f32 version — EXACT same steps
	s32 := [2]float32{float32(quad[1].X - quad[0].X), float32(quad[1].Y - quad[0].Y)}
	l32 := [2]float32{float32(quad[1].X - quad[2].X), float32(quad[1].Y - quad[2].Y)}
	sLen32 := s32[0]*s32[0] + s32[1]*s32[1]
	lLen32 := l32[0]*l32[0] + l32[1]*l32[1]
	if sLen32 > lLen32 {
		s32, l32 = l32, s32
		lLen32 = sLen32
	}
	sL32 := float32(math.Sqrt(float64(s32[0]*s32[0] + s32[1]*s32[1])))
	scale32 := float32(math.Sqrt(float64(lLen32))) / sL32
	scaled32 := [2]float32{s32[0] * scale32, s32[1] * scale32}
	dot32 := scaled32[0]*l32[0] + scaled32[1]*l32[1]

	t.Logf("f32 dot=%.15f sharp=%v", dot32, dot32 > 0)

	if dot > 0 != (dot32 > 0) {
		t.Logf("*** F32/F64 DIVERGENCE at dot boundary! f64=%.15f f32=%.15f ***", dot, dot32)
	}
}

func TestSharpAngleF32SetLength(t *testing.T) {
	// Reproduce tiny-skia's exact f32 computation
	q0 := [2]float32{2.25, float32(4.83333)}
	q1 := [2]float32{2.25, 3.75}
	q2 := [2]float32{3.25, 3.75}

	smaller := [2]float32{q1[0] - q0[0], q1[1] - q0[1]}
	larger := [2]float32{q1[0] - q2[0], q1[1] - q2[1]}
	sLen := smaller[0]*smaller[0] + smaller[1]*smaller[1]
	lLen := larger[0]*larger[0] + larger[1]*larger[1]

	if sLen > lLen {
		smaller, larger = larger, smaller
		lLen = sLen
	}

	// set_length(lLen) = normalize then scale by lLen
	// normalize: smaller / |smaller|
	smallerL := float32(math.Sqrt(float64(smaller[0]*smaller[0] + smaller[1]*smaller[1])))
	if smallerL < 1e-7 {
		t.Fatal("zero length")
	}
	nx := smaller[0] / smallerL
	ny := smaller[1] / smallerL
	// scale by lLen (squared length of larger, NOT sqrt)
	sx := nx * lLen
	sy := ny * lLen

	dot := sx*larger[0] + sy*larger[1]
	t.Logf("f32 set_length: smaller=(%e,%e) norm=(%e,%e) scaled=(%e,%e)",
		smaller[0], smaller[1], nx, ny, sx, sy)
	t.Logf("f32 larger=(%e,%e)", larger[0], larger[1])
	t.Logf("f32 dot = %e (%.20f) sharp=%v", dot, dot, dot > 0)
}
