package path

import (
	"math"
	"testing"
)

// --- Flatten tests ---

func TestFlattenEmpty(t *testing.T) {
	pts := Flatten(nil)
	if len(pts) != 0 {
		t.Errorf("Flatten(nil) returned %d points, want 0", len(pts))
	}
}

func TestFlattenMoveTo(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{10, 20}},
	}
	pts := Flatten(elems)
	if len(pts) != 1 {
		t.Fatalf("Flatten(MoveTo) returned %d points, want 1", len(pts))
	}
	if pts[0].X != 10 || pts[0].Y != 20 {
		t.Errorf("point = (%f,%f), want (10,20)", pts[0].X, pts[0].Y)
	}
}

func TestFlattenLine(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{0, 0}},
		LineTo{Point{100, 0}},
		LineTo{Point{100, 100}},
	}
	pts := Flatten(elems)
	if len(pts) != 3 {
		t.Fatalf("Flatten(lines) returned %d points, want 3", len(pts))
	}
	if pts[2].X != 100 || pts[2].Y != 100 {
		t.Errorf("last point = (%f,%f), want (100,100)", pts[2].X, pts[2].Y)
	}
}

func TestFlattenClose(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{0, 0}},
		LineTo{Point{100, 0}},
		LineTo{Point{100, 100}},
		Close{},
	}
	pts := Flatten(elems)
	// Close adds the first point again
	last := pts[len(pts)-1]
	if last.X != 0 || last.Y != 0 {
		t.Errorf("after Close, last point = (%f,%f), want (0,0)", last.X, last.Y)
	}
}

func TestFlattenQuadratic(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{0, 0}},
		QuadTo{Control: Point{50, 100}, Point: Point{100, 0}},
	}
	pts := Flatten(elems)
	// Should produce multiple points for the curve
	if len(pts) < 3 {
		t.Errorf("Flatten(quad) returned %d points, want >= 3", len(pts))
	}
	// Last point should be endpoint
	last := pts[len(pts)-1]
	if math.Abs(last.X-100) > 0.5 || math.Abs(last.Y-0) > 0.5 {
		t.Errorf("quad endpoint = (%f,%f), want ~(100,0)", last.X, last.Y)
	}
}

func TestFlattenCubic(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{0, 0}},
		CubicTo{
			Control1: Point{33, 100},
			Control2: Point{66, -100},
			Point:    Point{100, 0},
		},
	}
	pts := Flatten(elems)
	if len(pts) < 3 {
		t.Errorf("Flatten(cubic) returned %d points, want >= 3", len(pts))
	}
	last := pts[len(pts)-1]
	if math.Abs(last.X-100) > 0.5 || math.Abs(last.Y-0) > 0.5 {
		t.Errorf("cubic endpoint = (%f,%f), want ~(100,0)", last.X, last.Y)
	}
}

func TestFlattenMixed(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{0, 0}},
		LineTo{Point{50, 0}},
		QuadTo{Control: Point{75, 50}, Point: Point{100, 0}},
		CubicTo{
			Control1: Point{133, 50},
			Control2: Point{166, -50},
			Point:    Point{200, 0},
		},
		Close{},
	}
	pts := Flatten(elems)
	if len(pts) < 5 {
		t.Errorf("Flatten(mixed) returned %d points, want >= 5", len(pts))
	}
}

// --- Point helper tests ---

func TestPointLerp(t *testing.T) {
	a := Point{0, 0}
	b := Point{100, 200}

	mid := a.Lerp(b, 0.5)
	if mid.X != 50 || mid.Y != 100 {
		t.Errorf("Lerp(0.5) = (%f,%f), want (50,100)", mid.X, mid.Y)
	}

	start := a.Lerp(b, 0)
	if start.X != 0 || start.Y != 0 {
		t.Errorf("Lerp(0) = (%f,%f), want (0,0)", start.X, start.Y)
	}

	end := a.Lerp(b, 1)
	if end.X != 100 || end.Y != 200 {
		t.Errorf("Lerp(1) = (%f,%f), want (100,200)", end.X, end.Y)
	}
}

func TestPointSub(t *testing.T) {
	a := Point{100, 200}
	b := Point{30, 50}
	r := a.Sub(b)
	if r.X != 70 || r.Y != 150 {
		t.Errorf("Sub = (%f,%f), want (70,150)", r.X, r.Y)
	}
}

func TestPointAdd(t *testing.T) {
	a := Point{10, 20}
	b := Point{30, 40}
	r := a.Add(b)
	if r.X != 40 || r.Y != 60 {
		t.Errorf("Add = (%f,%f), want (40,60)", r.X, r.Y)
	}
}

func TestPointMul(t *testing.T) {
	a := Point{10, 20}
	r := a.Mul(3)
	if r.X != 30 || r.Y != 60 {
		t.Errorf("Mul(3) = (%f,%f), want (30,60)", r.X, r.Y)
	}
}

func TestPointDot(t *testing.T) {
	a := Point{1, 0}
	b := Point{0, 1}
	if a.Dot(b) != 0 {
		t.Errorf("perpendicular Dot = %f, want 0", a.Dot(b))
	}

	c := Point{3, 4}
	d := Point{3, 4}
	if c.Dot(d) != 25 {
		t.Errorf("parallel Dot = %f, want 25", c.Dot(d))
	}
}

func TestPointLength(t *testing.T) {
	p := Point{3, 4}
	if math.Abs(p.Length()-5) > 0.001 {
		t.Errorf("Length() = %f, want 5", p.Length())
	}

	zero := Point{0, 0}
	if zero.Length() != 0 {
		t.Errorf("zero Length() = %f, want 0", zero.Length())
	}
}

func TestPointDistance(t *testing.T) {
	a := Point{0, 0}
	b := Point{3, 4}
	if math.Abs(a.Distance(b)-5) > 0.001 {
		t.Errorf("Distance = %f, want 5", a.Distance(b))
	}
}

// --- distanceToLine tests ---

func TestDistanceToLine(t *testing.T) {
	tests := []struct {
		name    string
		p, a, b Point
		want    float64
	}{
		{
			name: "point on line",
			p:    Point{50, 0}, a: Point{0, 0}, b: Point{100, 0},
			want: 0,
		},
		{
			name: "point above horizontal line",
			p:    Point{50, 10}, a: Point{0, 0}, b: Point{100, 0},
			want: 10,
		},
		{
			name: "point before line start",
			p:    Point{-10, 0}, a: Point{0, 0}, b: Point{100, 0},
			want: 10,
		},
		{
			name: "point after line end",
			p:    Point{110, 0}, a: Point{0, 0}, b: Point{100, 0},
			want: 10,
		},
		{
			name: "degenerate line (point)",
			p:    Point{3, 4}, a: Point{0, 0}, b: Point{0, 0},
			want: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := distanceToLine(tt.p, tt.a, tt.b)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("distanceToLine() = %f, want %f", got, tt.want)
			}
		})
	}
}

// --- EdgeIter additional tests ---

func TestEdgeIterQuadCurve(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{0, 0}},
		QuadTo{Control: Point{50, 100}, Point: Point{100, 0}},
		Close{},
	}
	edges := CollectEdges(elems)
	if len(edges) < 2 {
		t.Errorf("quad path: expected >= 2 edges, got %d", len(edges))
	}
}

func TestEdgeIterCubicCurve(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{0, 0}},
		CubicTo{
			Control1: Point{33, 100},
			Control2: Point{66, -100},
			Point:    Point{100, 0},
		},
		Close{},
	}
	edges := CollectEdges(elems)
	if len(edges) < 2 {
		t.Errorf("cubic path: expected >= 2 edges, got %d", len(edges))
	}
}

func TestEdgeIterEmptyPath(t *testing.T) {
	edges := CollectEdges(nil)
	if len(edges) != 0 {
		t.Errorf("empty path: expected 0 edges, got %d", len(edges))
	}
}

func TestEdgeIterOnlyMoveTo(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{10, 20}},
	}
	edges := CollectEdges(elems)
	if len(edges) != 0 {
		t.Errorf("only MoveTo: expected 0 edges, got %d", len(edges))
	}
}

func TestEdgeIterMultipleMoveTos(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{0, 0}},
		LineTo{Point{100, 0}},
		MoveTo{Point{200, 200}},
		LineTo{Point{300, 200}},
	}
	edges := CollectEdges(elems)
	// Should have: line 0->100, close 100->0, line 200->300, close 300->200
	if len(edges) < 2 {
		t.Errorf("two subpaths: expected >= 2 edges, got %d", len(edges))
	}
}

func TestEdgeIterZeroLengthLine(t *testing.T) {
	elems := []PathElement{
		MoveTo{Point{10, 10}},
		LineTo{Point{10, 10}}, // zero-length
		LineTo{Point{20, 20}},
		Close{},
	}
	edges := CollectEdges(elems)
	// Zero-length edges should be skipped
	for _, e := range edges {
		if e.P0 == e.P1 {
			t.Error("found zero-length edge")
		}
	}
}
