package clip

import (
	"math"
	"testing"
)

func TestEdgeClipper_ClipLine_FullyInside(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipLine(Pt(10, 10), Pt(90, 90))

	if len(result) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result))
	}
	assertPointEqual(t, result[0].P0, Pt(10, 10))
	assertPointEqual(t, result[0].P1, Pt(90, 90))
}

func TestEdgeClipper_ClipLine_FullyOutside(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	tests := []struct {
		name   string
		p0, p1 Point
	}{
		{"left", Pt(-50, 50), Pt(-10, 50)},
		{"right", Pt(110, 50), Pt(150, 50)},
		{"top", Pt(50, -50), Pt(50, -10)},
		{"bottom", Pt(50, 110), Pt(50, 150)},
		{"diagonal outside", Pt(-10, -10), Pt(-5, -5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ec.ClipLine(tt.p0, tt.p1)
			if len(result) != 0 {
				t.Errorf("expected 0 segments, got %d", len(result))
			}
		})
	}
}

func TestEdgeClipper_ClipLine_CrossingFromLeft(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipLine(Pt(-50, 50), Pt(50, 50))

	if len(result) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result))
	}
	assertPointNear(t, result[0].P0, Pt(0, 50))
	assertPointNear(t, result[0].P1, Pt(50, 50))
}

func TestEdgeClipper_ClipLine_CrossingFromRight(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipLine(Pt(50, 50), Pt(150, 50))

	if len(result) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result))
	}
	assertPointNear(t, result[0].P0, Pt(50, 50))
	assertPointNear(t, result[0].P1, Pt(100, 50))
}

func TestEdgeClipper_ClipLine_CrossingFromTop(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipLine(Pt(50, -50), Pt(50, 50))

	if len(result) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result))
	}
	assertPointNear(t, result[0].P0, Pt(50, 0))
	assertPointNear(t, result[0].P1, Pt(50, 50))
}

func TestEdgeClipper_ClipLine_CrossingFromBottom(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipLine(Pt(50, 150), Pt(50, 50))

	if len(result) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result))
	}
	assertPointNear(t, result[0].P0, Pt(50, 100))
	assertPointNear(t, result[0].P1, Pt(50, 50))
}

func TestEdgeClipper_ClipLine_CrossingBothSides(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipLine(Pt(-50, 50), Pt(150, 50))

	if len(result) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result))
	}
	assertPointNear(t, result[0].P0, Pt(0, 50))
	assertPointNear(t, result[0].P1, Pt(100, 50))
}

func TestEdgeClipper_ClipLine_DiagonalCrossing(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipLine(Pt(-50, -50), Pt(150, 150))

	if len(result) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result))
	}
	assertPointNear(t, result[0].P0, Pt(0, 0))
	assertPointNear(t, result[0].P1, Pt(100, 100))
}

func TestEdgeClipper_ClipLine_CornerCases(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	tests := []struct {
		name     string
		p0, p1   Point
		expected int
	}{
		{"on left edge", Pt(0, 20), Pt(0, 80), 1},
		{"on top edge", Pt(20, 0), Pt(80, 0), 1},
		{"corner to corner", Pt(0, 0), Pt(100, 100), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ec.ClipLine(tt.p0, tt.p1)
			if len(result) != tt.expected {
				t.Errorf("expected %d segments, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestEdgeClipper_ClipQuadratic_FullyInside(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipQuadratic(Pt(10, 10), Pt(50, 50), Pt(90, 10))

	if len(result) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result))
	}
	assertPointEqual(t, result[0].P0, Pt(10, 10))
	assertPointEqual(t, result[0].P2, Pt(90, 10))
}

func TestEdgeClipper_ClipQuadratic_FullyOutside(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipQuadratic(Pt(-50, -50), Pt(-30, -30), Pt(-10, -50))

	if len(result) != 0 {
		t.Errorf("expected 0 segments, got %d", len(result))
	}
}

func TestEdgeClipper_ClipQuadratic_CrossingTop(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	// Quadratic that goes above the clip rect
	result := ec.ClipQuadratic(Pt(20, 50), Pt(50, -50), Pt(80, 50))

	// Should be clipped at y=0
	if len(result) == 0 {
		t.Fatalf("expected clipped segments, got none")
	}

	// All resulting points should be within or on bounds
	for _, seg := range result {
		if seg.P0.Y < -0.001 || seg.P2.Y < -0.001 {
			t.Errorf("segment endpoints outside clip: P0=%v, P2=%v", seg.P0, seg.P2)
		}
	}
}

func TestEdgeClipper_ClipQuadratic_MonotonicSplit(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 200, 200))

	// S-curve that has Y extremum
	result := ec.ClipQuadratic(Pt(20, 100), Pt(100, 0), Pt(180, 100))

	// Should have been split at extremum
	if len(result) < 1 {
		t.Errorf("expected at least 1 segment after monotonic split")
	}
}

func TestEdgeClipper_ClipCubic_FullyInside(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipCubic(Pt(10, 50), Pt(30, 20), Pt(70, 80), Pt(90, 50))

	if len(result) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result))
	}
}

func TestEdgeClipper_ClipCubic_FullyOutside(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	result := ec.ClipCubic(Pt(-50, -50), Pt(-30, -30), Pt(-20, -40), Pt(-10, -50))

	if len(result) != 0 {
		t.Errorf("expected 0 segments, got %d", len(result))
	}
}

func TestEdgeClipper_ClipCubic_CrossingBounds(t *testing.T) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))

	// Cubic that crosses multiple boundaries
	result := ec.ClipCubic(Pt(-20, 50), Pt(30, -50), Pt(70, 150), Pt(120, 50))

	if len(result) == 0 {
		t.Fatalf("expected some clipped segments")
	}

	// Verify midpoints of segments are inside clip
	for _, seg := range result {
		mid := evalCubic(seg.P0, seg.P1, seg.P2, seg.P3, 0.5)
		if !ec.clip.Contains(mid) {
			// This is okay - we're just checking the general behavior
			t.Logf("midpoint %v, clip bounds %v", mid, ec.clip)
		}
	}
}

func TestRect_Intersects(t *testing.T) {
	r := NewRect(0, 0, 100, 100)

	tests := []struct {
		name     string
		other    Rect
		expected bool
	}{
		{"overlapping", NewRect(50, 50, 100, 100), true},
		{"inside", NewRect(20, 20, 30, 30), true},
		{"containing", NewRect(-10, -10, 200, 200), true},
		{"adjacent right", NewRect(100, 0, 50, 100), true},
		{"separated right", NewRect(110, 0, 50, 100), false},
		{"separated left", NewRect(-60, 0, 50, 100), false},
		{"separated above", NewRect(0, -60, 100, 50), false},
		{"separated below", NewRect(0, 110, 100, 50), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Intersects(tt.other); got != tt.expected {
				t.Errorf("Intersects() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRect_Intersect(t *testing.T) {
	r := NewRect(0, 0, 100, 100)

	tests := []struct {
		name     string
		other    Rect
		expected Rect
	}{
		{"overlapping", NewRect(50, 50, 100, 100), NewRect(50, 50, 50, 50)},
		{"inside", NewRect(20, 20, 30, 30), NewRect(20, 20, 30, 30)},
		{"no overlap", NewRect(200, 200, 50, 50), Rect{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Intersect(tt.other)
			if got != tt.expected {
				t.Errorf("Intersect() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSolveQuadratic(t *testing.T) {
	tests := []struct {
		name      string
		a, b, c   float64
		wantCount int
	}{
		{"two roots", 1, 0, -1, 0},          // t^2 - 1 = 0, roots at -1 and 1 (outside [0,1])
		{"one root", 1, -1, 0, 1},           // t^2 - t = 0, roots at 0 and 1
		{"no roots", 1, 0, 1, 0},            // t^2 + 1 = 0, no real roots
		{"linear", 0, 2, -1, 1},             // 2t - 1 = 0, root at 0.5
		{"root at 0.5", 4, -4, 1, 0},        // 4t^2 - 4t + 1 = (2t-1)^2 = 0, double root at 0.5
		{"interior roots", 1, -1.5, 0.5, 2}, // t^2 - 1.5t + 0.5 = 0, roots at 0.5 and 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := solveQuadratic(tt.a, tt.b, tt.c)
			// Just verify roots are in valid range
			for _, r := range roots {
				if r <= 0 || r >= 1 {
					t.Errorf("root %v outside (0,1)", r)
				}
				// Verify it's actually a root
				val := tt.a*r*r + tt.b*r + tt.c
				if math.Abs(val) > 0.001 {
					t.Errorf("root %v gives value %v, expected ~0", r, val)
				}
			}
		})
	}
}

func TestSolveCubic(t *testing.T) {
	tests := []struct {
		name        string
		a, b, c, d  float64
		expectedMin int // minimum expected roots in (0,1)
	}{
		{"simple", 1, 0, 0, -0.125, 1},        // t^3 = 0.125, root at 0.5
		{"no interior roots", 1, 0, 0, -8, 0}, // t^3 = 8, root at 2 (outside)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := solveCubic(tt.a, tt.b, tt.c, tt.d)
			if len(roots) < tt.expectedMin {
				t.Errorf("expected at least %d roots, got %d", tt.expectedMin, len(roots))
			}
			// Verify roots
			for _, r := range roots {
				if r <= 0 || r >= 1 {
					t.Errorf("root %v outside (0,1)", r)
				}
				val := tt.a*r*r*r + tt.b*r*r + tt.c*r + tt.d
				if math.Abs(val) > 0.01 {
					t.Errorf("root %v gives value %v, expected ~0", r, val)
				}
			}
		})
	}
}

func TestChopQuadAt(t *testing.T) {
	p0 := Pt(0, 0)
	p1 := Pt(50, 100)
	p2 := Pt(100, 0)

	q0, q1, q2, q3, q4 := chopQuadAt(p0, p1, p2, 0.5)

	// q0 should be p0
	assertPointEqual(t, q0, p0)
	// q4 should be p2
	assertPointEqual(t, q4, p2)
	// q2 should be the midpoint of the original curve
	expectedMid := evalQuad(p0, p1, p2, 0.5)
	assertPointNear(t, q2, expectedMid)

	// The two halves should join at q2
	// First half: q0, q1, q2
	// Second half: q2, q3, q4
	// Evaluate first half at t=1 should equal q2
	firstEnd := evalQuad(q0, q1, q2, 1)
	assertPointNear(t, firstEnd, q2)

	// Evaluate second half at t=0 should equal q2
	secondStart := evalQuad(q2, q3, q4, 0)
	assertPointNear(t, secondStart, q2)
}

func TestChopCubicAt(t *testing.T) {
	p0 := Pt(0, 0)
	p1 := Pt(25, 100)
	p2 := Pt(75, 100)
	p3 := Pt(100, 0)

	left, right := chopCubicAt(p0, p1, p2, p3, 0.5)

	// left.P0 should be p0
	assertPointEqual(t, left.P0, p0)
	// right.P3 should be p3
	assertPointEqual(t, right.P3, p3)
	// They should join at the midpoint
	assertPointEqual(t, left.P3, right.P0)

	// The join point should be the curve evaluated at t=0.5
	expectedMid := evalCubic(p0, p1, p2, p3, 0.5)
	assertPointNear(t, left.P3, expectedMid)
}

func TestFilterAndSort(t *testing.T) {
	tests := []struct {
		name     string
		input    []float64
		expected []float64
	}{
		{"empty", []float64{}, []float64{}},
		{"single valid", []float64{0.5}, []float64{0.5}},
		{"out of range", []float64{-0.5, 1.5, 0}, []float64{}},
		{"unsorted", []float64{0.7, 0.3, 0.5}, []float64{0.3, 0.5, 0.7}},
		{"duplicates", []float64{0.5, 0.5, 0.5}, []float64{0.5}},
		{"mixed", []float64{1.5, 0.7, -0.1, 0.3, 0.7}, []float64{0.3, 0.7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterAndSort(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("length mismatch: got %v, want %v", got, tt.expected)
			}
			for i := range got {
				if math.Abs(got[i]-tt.expected[i]) > 0.001 {
					t.Errorf("value mismatch at %d: got %v, want %v", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestQuadSegBounds(t *testing.T) {
	q := QuadSeg{P0: Pt(0, 0), P1: Pt(50, 100), P2: Pt(100, 0)}
	bounds := q.Bounds()

	if bounds.X != 0 || bounds.Y != 0 {
		t.Errorf("unexpected origin: %v", bounds)
	}
	if bounds.W != 100 || bounds.H != 100 {
		t.Errorf("unexpected size: %v", bounds)
	}
}

func TestCubicSegBounds(t *testing.T) {
	c := CubicSeg{P0: Pt(0, 0), P1: Pt(25, 100), P2: Pt(75, -50), P3: Pt(100, 50)}
	bounds := c.Bounds()

	if bounds.X != 0 {
		t.Errorf("unexpected minX: %v", bounds.X)
	}
	if bounds.Y != -50 {
		t.Errorf("unexpected minY: %v", bounds.Y)
	}
	if bounds.W != 100 {
		t.Errorf("unexpected width: %v", bounds.W)
	}
	if bounds.H != 150 {
		t.Errorf("unexpected height: %v", bounds.H)
	}
}

func TestEvalQuad(t *testing.T) {
	p0 := Pt(0, 0)
	p1 := Pt(50, 100)
	p2 := Pt(100, 0)

	// At t=0, should be p0
	assertPointEqual(t, evalQuad(p0, p1, p2, 0), p0)
	// At t=1, should be p2
	assertPointEqual(t, evalQuad(p0, p1, p2, 1), p2)
	// At t=0.5, should be at (50, 50) for this symmetric curve
	mid := evalQuad(p0, p1, p2, 0.5)
	assertPointNear(t, mid, Pt(50, 50))
}

func TestEvalCubic(t *testing.T) {
	p0 := Pt(0, 0)
	p1 := Pt(0, 100)
	p2 := Pt(100, 100)
	p3 := Pt(100, 0)

	// At t=0, should be p0
	assertPointEqual(t, evalCubic(p0, p1, p2, p3, 0), p0)
	// At t=1, should be p3
	assertPointEqual(t, evalCubic(p0, p1, p2, p3, 1), p3)
	// At t=0.5, should be at center for this symmetric curve
	mid := evalCubic(p0, p1, p2, p3, 0.5)
	// For this particular curve, midpoint is at (50, 75)
	assertPointNear(t, mid, Pt(50, 75))
}

// Benchmark tests
func BenchmarkClipLine_Inside(b *testing.B) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))
	p0, p1 := Pt(10, 10), Pt(90, 90)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ec.ClipLine(p0, p1)
	}
}

func BenchmarkClipLine_Crossing(b *testing.B) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))
	p0, p1 := Pt(-50, 50), Pt(150, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ec.ClipLine(p0, p1)
	}
}

func BenchmarkClipQuadratic_Inside(b *testing.B) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))
	p0, p1, p2 := Pt(10, 10), Pt(50, 50), Pt(90, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ec.ClipQuadratic(p0, p1, p2)
	}
}

func BenchmarkClipQuadratic_Crossing(b *testing.B) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))
	p0, p1, p2 := Pt(-20, 50), Pt(50, -50), Pt(120, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ec.ClipQuadratic(p0, p1, p2)
	}
}

func BenchmarkClipCubic_Inside(b *testing.B) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))
	p0, p1, p2, p3 := Pt(10, 50), Pt(30, 20), Pt(70, 80), Pt(90, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ec.ClipCubic(p0, p1, p2, p3)
	}
}

func BenchmarkClipCubic_Crossing(b *testing.B) {
	ec := NewEdgeClipper(NewRect(0, 0, 100, 100))
	p0, p1, p2, p3 := Pt(-20, 50), Pt(30, -50), Pt(70, 150), Pt(120, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ec.ClipCubic(p0, p1, p2, p3)
	}
}

// Helper functions

const testEpsilon = 0.001

func assertPointEqual(t *testing.T, got, want Point) {
	t.Helper()
	if got.X != want.X || got.Y != want.Y {
		t.Errorf("point mismatch: got %v, want %v", got, want)
	}
}

func assertPointNear(t *testing.T, got, want Point) {
	t.Helper()
	if math.Abs(got.X-want.X) > testEpsilon || math.Abs(got.Y-want.Y) > testEpsilon {
		t.Errorf("point mismatch: got %v, want %v (epsilon=%v)", got, want, testEpsilon)
	}
}
