package gg

import (
	"math"
	"testing"
)

const epsilon = 1e-10

func pointsEqual(p1, p2 Point, eps float64) bool {
	return math.Abs(p1.X-p2.X) < eps && math.Abs(p1.Y-p2.Y) < eps
}

// -------------------------------------------------------------------
// Rect Tests
// -------------------------------------------------------------------

func TestRect_NewRect(t *testing.T) {
	tests := []struct {
		name      string
		p1, p2    Point
		expectMin Point
		expectMax Point
	}{
		{
			name: "normal order",
			p1:   Pt(0, 0), p2: Pt(10, 10),
			expectMin: Pt(0, 0), expectMax: Pt(10, 10),
		},
		{
			name: "reversed order",
			p1:   Pt(10, 10), p2: Pt(0, 0),
			expectMin: Pt(0, 0), expectMax: Pt(10, 10),
		},
		{
			name: "mixed",
			p1:   Pt(5, 0), p2: Pt(0, 5),
			expectMin: Pt(0, 0), expectMax: Pt(5, 5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRect(tt.p1, tt.p2)
			if !pointsEqual(r.Min, tt.expectMin, epsilon) {
				t.Errorf("Min = %v, want %v", r.Min, tt.expectMin)
			}
			if !pointsEqual(r.Max, tt.expectMax, epsilon) {
				t.Errorf("Max = %v, want %v", r.Max, tt.expectMax)
			}
		})
	}
}

func TestRect_WidthHeight(t *testing.T) {
	r := NewRect(Pt(0, 0), Pt(10, 5))
	if r.Width() != 10 {
		t.Errorf("Width() = %v, want 10", r.Width())
	}
	if r.Height() != 5 {
		t.Errorf("Height() = %v, want 5", r.Height())
	}
}

func TestRect_Union(t *testing.T) {
	r1 := NewRect(Pt(0, 0), Pt(5, 5))
	r2 := NewRect(Pt(3, 3), Pt(10, 10))
	u := r1.Union(r2)

	if !pointsEqual(u.Min, Pt(0, 0), epsilon) {
		t.Errorf("Union Min = %v, want (0, 0)", u.Min)
	}
	if !pointsEqual(u.Max, Pt(10, 10), epsilon) {
		t.Errorf("Union Max = %v, want (10, 10)", u.Max)
	}
}

func TestRect_Contains(t *testing.T) {
	r := NewRect(Pt(0, 0), Pt(10, 10))

	tests := []struct {
		name   string
		p      Point
		expect bool
	}{
		{"inside", Pt(5, 5), true},
		{"corner", Pt(0, 0), true},
		{"edge", Pt(5, 0), true},
		{"outside", Pt(15, 5), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Contains(tt.p)
			if result != tt.expect {
				t.Errorf("Contains(%v) = %v, want %v", tt.p, result, tt.expect)
			}
		})
	}
}

// -------------------------------------------------------------------
// Line Tests
// -------------------------------------------------------------------

func TestLine_Eval(t *testing.T) {
	l := NewLine(Pt(0, 0), Pt(10, 10))

	tests := []struct {
		name   string
		t      float64
		expect Point
	}{
		{"t=0", 0, Pt(0, 0)},
		{"t=1", 1, Pt(10, 10)},
		{"t=0.5", 0.5, Pt(5, 5)},
		{"t=0.25", 0.25, Pt(2.5, 2.5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := l.Eval(tt.t)
			if !pointsEqual(result, tt.expect, epsilon) {
				t.Errorf("Eval(%v) = %v, want %v", tt.t, result, tt.expect)
			}
		})
	}
}

func TestLine_StartEnd(t *testing.T) {
	l := NewLine(Pt(1, 2), Pt(3, 4))

	if !pointsEqual(l.Start(), Pt(1, 2), epsilon) {
		t.Errorf("Start() = %v, want (1, 2)", l.Start())
	}
	if !pointsEqual(l.End(), Pt(3, 4), epsilon) {
		t.Errorf("End() = %v, want (3, 4)", l.End())
	}
}

func TestLine_Subdivide(t *testing.T) {
	l := NewLine(Pt(0, 0), Pt(10, 10))
	l1, l2 := l.Subdivide()

	if !pointsEqual(l1.P0, Pt(0, 0), epsilon) {
		t.Errorf("l1.P0 = %v, want (0, 0)", l1.P0)
	}
	if !pointsEqual(l1.P1, Pt(5, 5), epsilon) {
		t.Errorf("l1.P1 = %v, want (5, 5)", l1.P1)
	}
	if !pointsEqual(l2.P0, Pt(5, 5), epsilon) {
		t.Errorf("l2.P0 = %v, want (5, 5)", l2.P0)
	}
	if !pointsEqual(l2.P1, Pt(10, 10), epsilon) {
		t.Errorf("l2.P1 = %v, want (10, 10)", l2.P1)
	}
}

func TestLine_Subsegment(t *testing.T) {
	l := NewLine(Pt(0, 0), Pt(10, 0))
	sub := l.Subsegment(0.25, 0.75)

	if !pointsEqual(sub.P0, Pt(2.5, 0), epsilon) {
		t.Errorf("Subsegment P0 = %v, want (2.5, 0)", sub.P0)
	}
	if !pointsEqual(sub.P1, Pt(7.5, 0), epsilon) {
		t.Errorf("Subsegment P1 = %v, want (7.5, 0)", sub.P1)
	}
}

func TestLine_BoundingBox(t *testing.T) {
	l := NewLine(Pt(5, 3), Pt(2, 8))
	bbox := l.BoundingBox()

	if !pointsEqual(bbox.Min, Pt(2, 3), epsilon) {
		t.Errorf("BoundingBox Min = %v, want (2, 3)", bbox.Min)
	}
	if !pointsEqual(bbox.Max, Pt(5, 8), epsilon) {
		t.Errorf("BoundingBox Max = %v, want (5, 8)", bbox.Max)
	}
}

func TestLine_Length(t *testing.T) {
	l := NewLine(Pt(0, 0), Pt(3, 4))
	if math.Abs(l.Length()-5) > epsilon {
		t.Errorf("Length() = %v, want 5", l.Length())
	}
}

func TestLine_Midpoint(t *testing.T) {
	l := NewLine(Pt(0, 0), Pt(10, 10))
	mid := l.Midpoint()
	if !pointsEqual(mid, Pt(5, 5), epsilon) {
		t.Errorf("Midpoint() = %v, want (5, 5)", mid)
	}
}

func TestLine_Reversed(t *testing.T) {
	l := NewLine(Pt(1, 2), Pt(3, 4))
	r := l.Reversed()

	if !pointsEqual(r.P0, l.P1, epsilon) {
		t.Errorf("Reversed P0 = %v, want %v", r.P0, l.P1)
	}
	if !pointsEqual(r.P1, l.P0, epsilon) {
		t.Errorf("Reversed P1 = %v, want %v", r.P1, l.P0)
	}
}

// -------------------------------------------------------------------
// QuadBez Tests
// -------------------------------------------------------------------

func TestQuadBez_Eval(t *testing.T) {
	// A parabola-like curve
	q := NewQuadBez(Pt(0, 0), Pt(5, 10), Pt(10, 0))

	tests := []struct {
		name   string
		t      float64
		expect Point
	}{
		{"t=0", 0, Pt(0, 0)},
		{"t=1", 1, Pt(10, 0)},
		{"t=0.5", 0.5, Pt(5, 5)}, // Midpoint should be at (5, 5)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := q.Eval(tt.t)
			if !pointsEqual(result, tt.expect, epsilon) {
				t.Errorf("Eval(%v) = %v, want %v", tt.t, result, tt.expect)
			}
		})
	}
}

func TestQuadBez_StartEnd(t *testing.T) {
	q := NewQuadBez(Pt(1, 2), Pt(3, 4), Pt(5, 6))

	if !pointsEqual(q.Start(), Pt(1, 2), epsilon) {
		t.Errorf("Start() = %v, want (1, 2)", q.Start())
	}
	if !pointsEqual(q.End(), Pt(5, 6), epsilon) {
		t.Errorf("End() = %v, want (5, 6)", q.End())
	}
}

func TestQuadBez_Subdivide(t *testing.T) {
	q := NewQuadBez(Pt(0, 0), Pt(5, 10), Pt(10, 0))
	q1, q2 := q.Subdivide()

	// The midpoint should be shared
	if !pointsEqual(q1.P2, q2.P0, epsilon) {
		t.Errorf("Subdivision junction: q1.P2=%v != q2.P0=%v", q1.P2, q2.P0)
	}

	// Endpoints should match original
	if !pointsEqual(q1.P0, q.P0, epsilon) {
		t.Errorf("q1.P0 = %v, want %v", q1.P0, q.P0)
	}
	if !pointsEqual(q2.P2, q.P2, epsilon) {
		t.Errorf("q2.P2 = %v, want %v", q2.P2, q.P2)
	}

	// Verify points on subdivided curves match original
	for i := 0; i <= 10; i++ {
		tt := float64(i) / 10.0
		original := q.Eval(tt)

		var subdivided Point
		if tt <= 0.5 {
			subdivided = q1.Eval(tt * 2)
		} else {
			subdivided = q2.Eval((tt - 0.5) * 2)
		}

		if !pointsEqual(original, subdivided, 1e-9) {
			t.Errorf("Mismatch at t=%v: original=%v, subdivided=%v", tt, original, subdivided)
		}
	}
}

func TestQuadBez_Extrema(t *testing.T) {
	// y = x^2 style parabola, extremum in y at t=0.5
	q := NewQuadBez(Pt(-1, 1), Pt(0, -1), Pt(1, 1))
	extrema := q.Extrema()

	if len(extrema) != 1 {
		t.Errorf("Expected 1 extremum, got %d: %v", len(extrema), extrema)
		return
	}
	if math.Abs(extrema[0]-0.5) > epsilon {
		t.Errorf("Extremum at %v, want 0.5", extrema[0])
	}
}

func TestQuadBez_BoundingBox(t *testing.T) {
	q := NewQuadBez(Pt(0, 0), Pt(5, 10), Pt(10, 0))
	bbox := q.BoundingBox()

	// Should include endpoints
	if !bbox.Contains(q.P0) || !bbox.Contains(q.P2) {
		t.Error("BoundingBox should contain endpoints")
	}

	// Should include extremum point (at t=0.5, y=5)
	extremumPt := q.Eval(0.5)
	if !bbox.Contains(extremumPt) {
		t.Errorf("BoundingBox should contain extremum point %v", extremumPt)
	}
}

func TestQuadBez_Raise(t *testing.T) {
	q := NewQuadBez(Pt(0, 0), Pt(5, 10), Pt(10, 0))
	c := q.Raise()

	// Endpoints should match
	if !pointsEqual(c.P0, q.P0, epsilon) {
		t.Errorf("Raised P0 = %v, want %v", c.P0, q.P0)
	}
	if !pointsEqual(c.P3, q.P2, epsilon) {
		t.Errorf("Raised P3 = %v, want %v", c.P3, q.P2)
	}

	// Verify curves match at sample points
	for i := 0; i <= 10; i++ {
		tt := float64(i) / 10.0
		qp := q.Eval(tt)
		cp := c.Eval(tt)
		if !pointsEqual(qp, cp, 1e-9) {
			t.Errorf("Mismatch at t=%v: quad=%v, cubic=%v", tt, qp, cp)
		}
	}
}

// -------------------------------------------------------------------
// CubicBez Tests
// -------------------------------------------------------------------

func TestCubicBez_Eval(t *testing.T) {
	// y = x^2 approximation
	c := NewCubicBez(Pt(0, 0), Pt(1.0/3.0, 0), Pt(2.0/3.0, 1.0/3.0), Pt(1, 1))

	tests := []struct {
		name   string
		t      float64
		expect Point
	}{
		{"t=0", 0, Pt(0, 0)},
		{"t=1", 1, Pt(1, 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Eval(tt.t)
			if !pointsEqual(result, tt.expect, epsilon) {
				t.Errorf("Eval(%v) = %v, want %v", tt.t, result, tt.expect)
			}
		})
	}
}

func TestCubicBez_StartEnd(t *testing.T) {
	c := NewCubicBez(Pt(1, 2), Pt(3, 4), Pt(5, 6), Pt(7, 8))

	if !pointsEqual(c.Start(), Pt(1, 2), epsilon) {
		t.Errorf("Start() = %v, want (1, 2)", c.Start())
	}
	if !pointsEqual(c.End(), Pt(7, 8), epsilon) {
		t.Errorf("End() = %v, want (7, 8)", c.End())
	}
}

func TestCubicBez_Subdivide(t *testing.T) {
	c := NewCubicBez(Pt(0, 0), Pt(0, 10), Pt(10, 10), Pt(10, 0))
	c1, c2 := c.Subdivide()

	// Junction should match
	if !pointsEqual(c1.P3, c2.P0, epsilon) {
		t.Errorf("Subdivision junction: c1.P3=%v != c2.P0=%v", c1.P3, c2.P0)
	}

	// Endpoints should match original
	if !pointsEqual(c1.P0, c.P0, epsilon) {
		t.Errorf("c1.P0 = %v, want %v", c1.P0, c.P0)
	}
	if !pointsEqual(c2.P3, c.P3, epsilon) {
		t.Errorf("c2.P3 = %v, want %v", c2.P3, c.P3)
	}

	// Verify points on subdivided curves match original
	for i := 0; i <= 10; i++ {
		tt := float64(i) / 10.0
		original := c.Eval(tt)

		var subdivided Point
		if tt <= 0.5 {
			subdivided = c1.Eval(tt * 2)
		} else {
			subdivided = c2.Eval((tt - 0.5) * 2)
		}

		if !pointsEqual(original, subdivided, 1e-9) {
			t.Errorf("Mismatch at t=%v: original=%v, subdivided=%v", tt, original, subdivided)
		}
	}
}

func TestCubicBez_Subsegment(t *testing.T) {
	c := NewCubicBez(Pt(0, 0), Pt(0, 10), Pt(10, 10), Pt(10, 0))
	sub := c.Subsegment(0.25, 0.75)

	// Verify endpoints of subsegment
	expectedStart := c.Eval(0.25)
	expectedEnd := c.Eval(0.75)

	if !pointsEqual(sub.P0, expectedStart, 1e-9) {
		t.Errorf("Subsegment start = %v, want %v", sub.P0, expectedStart)
	}
	if !pointsEqual(sub.P3, expectedEnd, 1e-9) {
		t.Errorf("Subsegment end = %v, want %v", sub.P3, expectedEnd)
	}

	// Verify points along subsegment match original
	for i := 0; i <= 10; i++ {
		tSub := float64(i) / 10.0
		tOrig := 0.25 + tSub*0.5

		subPt := sub.Eval(tSub)
		origPt := c.Eval(tOrig)

		if !pointsEqual(subPt, origPt, 1e-8) {
			t.Errorf("Mismatch at tSub=%v: sub=%v, orig=%v", tSub, subPt, origPt)
		}
	}
}

func TestCubicBez_Extrema(t *testing.T) {
	// Curve with clear internal extremum
	c := NewCubicBez(Pt(0, 0), Pt(0, 1), Pt(1, 1), Pt(1, 0))
	extrema := c.Extrema()

	// Should have at least 1 extremum
	if len(extrema) < 1 {
		t.Errorf("Expected at least 1 extremum, got %d", len(extrema))
	}

	// All extrema should be in [0, 1]
	for _, e := range extrema {
		if e < 0 || e > 1 {
			t.Errorf("Extremum %v not in [0, 1]", e)
		}
	}

	// For this particular curve, y extremum should be around t=0.5
	hasMiddleExtremum := false
	for _, e := range extrema {
		if e > 0.3 && e < 0.7 {
			hasMiddleExtremum = true
			break
		}
	}
	if !hasMiddleExtremum {
		t.Errorf("Expected extremum near t=0.5, got %v", extrema)
	}
}

func TestCubicBez_BoundingBox(t *testing.T) {
	c := NewCubicBez(Pt(0, 0), Pt(0, 10), Pt(10, 10), Pt(10, 0))
	bbox := c.BoundingBox()

	// Should contain endpoints
	if !bbox.Contains(c.P0) || !bbox.Contains(c.P3) {
		t.Error("BoundingBox should contain endpoints")
	}

	// Test points along curve should be contained
	for i := 0; i <= 100; i++ {
		tt := float64(i) / 100.0
		p := c.Eval(tt)
		if !bbox.Contains(p) {
			t.Errorf("BoundingBox should contain point at t=%v: %v", tt, p)
		}
	}
}

func TestCubicBez_Inflections(t *testing.T) {
	// S-curve with inflection
	c := NewCubicBez(Pt(0, 0), Pt(0.8, 1), Pt(0.2, 1), Pt(1, 0))
	inflections := c.Inflections()

	// S-curve should have 2 inflection points
	if len(inflections) != 2 {
		t.Errorf("Expected 2 inflections, got %d: %v", len(inflections), inflections)
	}
}

func TestCubicBez_Deriv(t *testing.T) {
	c := NewCubicBez(Pt(0, 0), Pt(1, 0), Pt(2, 1), Pt(3, 1))
	deriv := c.Deriv()

	// Check derivative is a quadratic
	// At t=0, tangent should be 3*(P1-P0) = 3*(1,0) = (3,0)
	d0 := deriv.Eval(0)
	expected0 := Pt(3*(c.P1.X-c.P0.X), 3*(c.P1.Y-c.P0.Y))
	if !pointsEqual(d0, expected0, epsilon) {
		t.Errorf("Deriv at t=0: got %v, want %v", d0, expected0)
	}
}

func TestCubicBez_Tangent(t *testing.T) {
	c := NewCubicBez(Pt(0, 0), Pt(10, 0), Pt(10, 10), Pt(0, 10))

	// At t=0, tangent should point in direction of P1-P0
	tan0 := c.Tangent(0)
	if tan0.X <= 0 {
		t.Errorf("Tangent at t=0 should point right, got %v", tan0)
	}

	// At t=1, tangent should point in direction of P3-P2
	tan1 := c.Tangent(1)
	if tan1.X >= 0 {
		t.Errorf("Tangent at t=1 should point left, got %v", tan1)
	}
}

func TestCubicBez_Normal(t *testing.T) {
	c := NewCubicBez(Pt(0, 0), Pt(10, 0), Pt(20, 0), Pt(30, 0))

	// For horizontal line, normal should be vertical
	n := c.Normal(0.5)
	if math.Abs(n.X) > epsilon || math.Abs(math.Abs(n.Y)-1) > epsilon {
		t.Errorf("Normal for horizontal line should be (0, +/-1), got %v", n)
	}
}
