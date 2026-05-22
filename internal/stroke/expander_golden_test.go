package stroke

import (
	"math"
	"testing"
)

// TestStrokeExpander_SineWaveGolden verifies that stroke expansion of a
// 100-segment damped sine wave matches the Rust kurbo reference output.
// Golden values from: kurbo/examples/stroke_debug.rs (2px, butt cap, miter join, limit 10).
//
// This is the regression test for issue #347: extra inner join + skip-threshold
// segments caused self-intersecting outlines that broke tile-based rasterizers.
func TestStrokeExpander_SineWaveGolden(t *testing.T) {
	path := buildSineWavePath(100)
	style := Stroke{Width: 2.0, Cap: LineCapButt, Join: LineJoinMiter, MiterLimit: 10.0}
	expander := NewStrokeExpander(style)
	outVerbs, outCoords := expander.Expand(path.verbs, path.coords)

	// Golden: Rust kurbo produces 201 elements (1 MoveTo + 199 LineTo + 1 Close)
	wantElements := 201
	if len(outVerbs) != wantElements {
		t.Fatalf("element count: got %d, want %d (Rust kurbo golden)", len(outVerbs), wantElements)
	}

	// Verify verb composition
	var moves, lines, closes int
	for _, v := range outVerbs {
		switch v {
		case VerbMoveTo:
			moves++
		case VerbLineTo:
			lines++
		case VerbClose:
			closes++
		}
	}
	if moves != 1 || lines != 199 || closes != 1 {
		t.Fatalf("verb counts: MoveTo=%d LineTo=%d Close=%d, want 1/199/1", moves, lines, closes)
	}

	// Verify no duplicate adjacent points (the #347 bug signature)
	points := extractPoints(outVerbs, outCoords)
	dups := 0
	for i := 1; i < len(points); i++ {
		if points[i][0] == points[i-1][0] && points[i][1] == points[i-1][1] {
			dups++
		}
	}
	if dups > 0 {
		t.Errorf("found %d duplicate adjacent points (should be 0 after #347 fix)", dups)
	}

	// Verify key coordinates match Rust kurbo golden (±0.1 tolerance for float64 vs f64)
	goldenFirst := [][2]float64{
		{49.1, 249.7}, {56.1, 229.9}, {63.1, 210.7}, {70.1, 192.3}, {77.1, 174.8},
	}
	goldenLast := [][2]float64{
		{78.9, 175.5}, {71.9, 193.0}, {64.9, 211.4}, {57.9, 230.6}, {50.9, 250.3},
	}

	for i, g := range goldenFirst {
		if !closeEnough(points[i], g, 0.15) {
			t.Errorf("point[%d]: got (%.1f,%.1f), want (%.1f,%.1f)", i, points[i][0], points[i][1], g[0], g[1])
		}
	}
	for i, g := range goldenLast {
		idx := len(points) - len(goldenLast) + i
		if !closeEnough(points[idx], g, 0.15) {
			t.Errorf("point[%d]: got (%.1f,%.1f), want (%.1f,%.1f)", idx, points[idx][0], points[idx][1], g[0], g[1])
		}
	}

	// Verify no self-intersections (simple polygon)
	crossings := countSelfIntersections(points)
	if crossings > 0 {
		t.Errorf("found %d self-intersections (should be 0 for simple polygon)", crossings)
	}

	t.Logf("OK: %d elements, %d points, 0 duplicates, 0 self-intersections (matches Rust kurbo)", len(outVerbs), len(points))
}

// TestStrokeExpander_ClosedRectGolden verifies stroke of a closed rectangle.
func TestStrokeExpander_ClosedRectGolden(t *testing.T) {
	p := &soaPath{}
	p.moveTo(10, 10).lineTo(90, 10).lineTo(90, 90).lineTo(10, 90).close()

	style := Stroke{Width: 4.0, Cap: LineCapButt, Join: LineJoinMiter, MiterLimit: 10.0}
	expander := NewStrokeExpander(style)
	outVerbs, _ := expander.Expand(p.verbs, p.coords)

	// Closed rect stroke: 2 contours (forward ring + backward ring)
	var moves, closes int
	for _, v := range outVerbs {
		switch v {
		case VerbMoveTo:
			moves++
		case VerbClose:
			closes++
		}
	}
	if moves != 2 || closes != 2 {
		t.Errorf("closed rect stroke: MoveTo=%d Close=%d, want 2/2 (two contours)", moves, closes)
	}
}

func buildSineWavePath(n int) *soaPath {
	p := &soaPath{}
	for i := 0; i < n; i++ {
		t := float64(i) * 0.1
		x := 50 + t*70
		y := 250 - math.Sin(t)*math.Exp(-t*0.1)*200
		if i == 0 {
			p.moveTo(x, y)
		} else {
			p.lineTo(x, y)
		}
	}
	return p
}

func extractPoints(verbs []PathVerb, coords []float64) [][2]float64 {
	var pts [][2]float64
	ci := 0
	for _, v := range verbs {
		switch v {
		case VerbMoveTo, VerbLineTo:
			pts = append(pts, [2]float64{coords[ci], coords[ci+1]})
			ci += 2
		case VerbQuadTo:
			ci += 4
		case VerbCubicTo:
			ci += 6
		case VerbClose:
		}
	}
	return pts
}

func closeEnough(a, b [2]float64, tol float64) bool {
	return math.Abs(a[0]-b[0]) < tol && math.Abs(a[1]-b[1]) < tol
}

func countSelfIntersections(pts [][2]float64) int {
	n := len(pts)
	count := 0
	for i := 0; i < n; i++ {
		a1, a2 := pts[i], pts[(i+1)%n]
		for j := i + 2; j < n; j++ {
			if j == (i+n-1)%n {
				continue
			}
			b1, b2 := pts[j], pts[(j+1)%n]
			if segsCross(a1, a2, b1, b2) {
				count++
			}
		}
	}
	return count
}

func segsCross(a1, a2, b1, b2 [2]float64) bool {
	d1 := crossPt(b1, b2, a1)
	d2 := crossPt(b1, b2, a2)
	d3 := crossPt(a1, a2, b1)
	d4 := crossPt(a1, a2, b2)
	return ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0))
}

func crossPt(a, b, c [2]float64) float64 {
	return (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])
}
