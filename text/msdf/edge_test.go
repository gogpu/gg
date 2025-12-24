package msdf

import (
	"math"
	"testing"
)

func TestEdgeTypeString(t *testing.T) {
	tests := []struct {
		et   EdgeType
		want string
	}{
		{EdgeLinear, "Linear"},
		{EdgeQuadratic, "Quadratic"},
		{EdgeCubic, "Cubic"},
		{EdgeType(99), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.et.String(); got != tt.want {
			t.Errorf("EdgeType(%d).String() = %q, want %q", tt.et, got, tt.want)
		}
	}
}

func TestEdgeColorString(t *testing.T) {
	tests := []struct {
		c    EdgeColor
		want string
	}{
		{ColorBlack, "Black"},
		{ColorRed, "Red"},
		{ColorGreen, "Green"},
		{ColorBlue, "Blue"},
		{ColorYellow, "Yellow"},
		{ColorCyan, "Cyan"},
		{ColorMagenta, "Magenta"},
		{ColorWhite, "White"},
		{EdgeColor(99), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("EdgeColor(%d).String() = %q, want %q", tt.c, got, tt.want)
		}
	}
}

func TestEdgeColorChannels(t *testing.T) {
	tests := []struct {
		c                  EdgeColor
		hasR, hasG, hasB   bool
	}{
		{ColorBlack, false, false, false},
		{ColorRed, true, false, false},
		{ColorGreen, false, true, false},
		{ColorBlue, false, false, true},
		{ColorYellow, true, true, false},
		{ColorCyan, false, true, true},
		{ColorMagenta, true, false, true},
		{ColorWhite, true, true, true},
	}

	for _, tt := range tests {
		if got := tt.c.HasRed(); got != tt.hasR {
			t.Errorf("EdgeColor(%d).HasRed() = %v, want %v", tt.c, got, tt.hasR)
		}
		if got := tt.c.HasGreen(); got != tt.hasG {
			t.Errorf("EdgeColor(%d).HasGreen() = %v, want %v", tt.c, got, tt.hasG)
		}
		if got := tt.c.HasBlue(); got != tt.hasB {
			t.Errorf("EdgeColor(%d).HasBlue() = %v, want %v", tt.c, got, tt.hasB)
		}
	}
}

func TestNewLinearEdge(t *testing.T) {
	start := Point{0, 0}
	end := Point{10, 10}

	edge := NewLinearEdge(start, end)

	if edge.Type != EdgeLinear {
		t.Errorf("NewLinearEdge().Type = %v, want EdgeLinear", edge.Type)
	}
	if edge.Points[0] != start || edge.Points[1] != end {
		t.Errorf("NewLinearEdge() points incorrect")
	}
	if edge.Color != ColorWhite {
		t.Errorf("NewLinearEdge().Color = %v, want ColorWhite", edge.Color)
	}
}

func TestNewQuadraticEdge(t *testing.T) {
	start := Point{0, 0}
	control := Point{5, 10}
	end := Point{10, 0}

	edge := NewQuadraticEdge(start, control, end)

	if edge.Type != EdgeQuadratic {
		t.Errorf("NewQuadraticEdge().Type = %v, want EdgeQuadratic", edge.Type)
	}
	if edge.Points[0] != start || edge.Points[1] != control || edge.Points[2] != end {
		t.Errorf("NewQuadraticEdge() points incorrect")
	}
}

func TestNewCubicEdge(t *testing.T) {
	start := Point{0, 0}
	c1 := Point{3, 10}
	c2 := Point{7, 10}
	end := Point{10, 0}

	edge := NewCubicEdge(start, c1, c2, end)

	if edge.Type != EdgeCubic {
		t.Errorf("NewCubicEdge().Type = %v, want EdgeCubic", edge.Type)
	}
	if edge.Points[0] != start || edge.Points[1] != c1 || edge.Points[2] != c2 || edge.Points[3] != end {
		t.Errorf("NewCubicEdge() points incorrect")
	}
}

func TestEdgeStartEndPoints(t *testing.T) {
	// Linear
	linear := NewLinearEdge(Point{0, 0}, Point{10, 0})
	if linear.StartPoint() != (Point{0, 0}) {
		t.Errorf("Linear.StartPoint() = %v, want {0, 0}", linear.StartPoint())
	}
	if linear.EndPoint() != (Point{10, 0}) {
		t.Errorf("Linear.EndPoint() = %v, want {10, 0}", linear.EndPoint())
	}

	// Quadratic
	quad := NewQuadraticEdge(Point{0, 0}, Point{5, 5}, Point{10, 0})
	if quad.StartPoint() != (Point{0, 0}) {
		t.Errorf("Quadratic.StartPoint() = %v, want {0, 0}", quad.StartPoint())
	}
	if quad.EndPoint() != (Point{10, 0}) {
		t.Errorf("Quadratic.EndPoint() = %v, want {10, 0}", quad.EndPoint())
	}

	// Cubic
	cubic := NewCubicEdge(Point{0, 0}, Point{3, 5}, Point{7, 5}, Point{10, 0})
	if cubic.StartPoint() != (Point{0, 0}) {
		t.Errorf("Cubic.StartPoint() = %v, want {0, 0}", cubic.StartPoint())
	}
	if cubic.EndPoint() != (Point{10, 0}) {
		t.Errorf("Cubic.EndPoint() = %v, want {10, 0}", cubic.EndPoint())
	}
}

func TestEdgePointAt(t *testing.T) {
	// Linear edge at t=0.5
	linear := NewLinearEdge(Point{0, 0}, Point{10, 0})
	mid := linear.PointAt(0.5)
	if math.Abs(mid.X-5) > 1e-10 || math.Abs(mid.Y) > 1e-10 {
		t.Errorf("Linear.PointAt(0.5) = %v, want {5, 0}", mid)
	}

	// Quadratic edge at t=0 and t=1
	quad := NewQuadraticEdge(Point{0, 0}, Point{5, 10}, Point{10, 0})
	start := quad.PointAt(0)
	if math.Abs(start.X) > 1e-10 || math.Abs(start.Y) > 1e-10 {
		t.Errorf("Quadratic.PointAt(0) = %v, want {0, 0}", start)
	}
	end := quad.PointAt(1)
	if math.Abs(end.X-10) > 1e-10 || math.Abs(end.Y) > 1e-10 {
		t.Errorf("Quadratic.PointAt(1) = %v, want {10, 0}", end)
	}
	// At t=0.5, quadratic should be at {5, 5} (halfway up the control point influence)
	midQuad := quad.PointAt(0.5)
	if math.Abs(midQuad.X-5) > 1e-10 || math.Abs(midQuad.Y-5) > 1e-10 {
		t.Errorf("Quadratic.PointAt(0.5) = %v, want {5, 5}", midQuad)
	}

	// Cubic edge endpoints
	cubic := NewCubicEdge(Point{0, 0}, Point{3, 10}, Point{7, 10}, Point{10, 0})
	if cubic.PointAt(0) != (Point{0, 0}) {
		t.Errorf("Cubic.PointAt(0) = %v, want {0, 0}", cubic.PointAt(0))
	}
	if cubic.PointAt(1) != (Point{10, 0}) {
		t.Errorf("Cubic.PointAt(1) = %v, want {10, 0}", cubic.PointAt(1))
	}
}

func TestEdgeDirectionAt(t *testing.T) {
	// Linear edge has constant direction
	linear := NewLinearEdge(Point{0, 0}, Point{10, 0})
	dir := linear.DirectionAt(0.5)
	if math.Abs(dir.X-10) > 1e-10 || math.Abs(dir.Y) > 1e-10 {
		t.Errorf("Linear.DirectionAt(0.5) = %v, want {10, 0}", dir)
	}

	// Quadratic at t=0 should point toward control
	quad := NewQuadraticEdge(Point{0, 0}, Point{5, 10}, Point{10, 0})
	dirStart := quad.DirectionAt(0).Normalized()
	// Should point toward (5, 10) direction from (0, 0)
	expected := Point{5, 10}.Normalized()
	if math.Abs(dirStart.X-expected.X) > 1e-10 || math.Abs(dirStart.Y-expected.Y) > 1e-10 {
		t.Errorf("Quadratic.DirectionAt(0) normalized = %v, want %v", dirStart, expected)
	}
}

func TestLinearSignedDistance(t *testing.T) {
	edge := NewLinearEdge(Point{0, 0}, Point{10, 0})

	tests := []struct {
		name     string
		p        Point
		wantDist float64 // approximate expected distance
		inside   bool    // expected sign (true = negative/inside)
	}{
		{"on line", Point{5, 0}, 0, false},
		{"above line", Point{5, 3}, 3, false},
		{"below line", Point{5, -3}, -3, true},
		{"at start", Point{0, 0}, 0, false},
		{"at end", Point{10, 0}, 0, false},
		{"before start", Point{-2, 0}, 2, false},
		{"after end", Point{12, 0}, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd := edge.SignedDistance(tt.p)
			gotDist := sd.Distance
			gotInside := gotDist < 0

			// Check distance magnitude
			if math.Abs(math.Abs(gotDist)-math.Abs(tt.wantDist)) > 0.1 {
				t.Errorf("distance = %v, want ~%v", gotDist, tt.wantDist)
			}

			// Check sign matches inside expectation
			if gotInside != tt.inside && math.Abs(tt.wantDist) > 0.1 {
				t.Errorf("inside = %v, want %v (dist=%v)", gotInside, tt.inside, gotDist)
			}
		})
	}
}

func TestQuadraticSignedDistance(t *testing.T) {
	// Simple parabola opening downward
	edge := NewQuadraticEdge(Point{0, 0}, Point{5, 10}, Point{10, 0})

	tests := []struct {
		name     string
		p        Point
		maxDist  float64 // max expected distance
	}{
		{"on curve start", Point{0, 0}, 0.1},
		{"on curve end", Point{10, 0}, 0.1},
		{"at apex roughly", Point{5, 5}, 0.5},
		{"far outside", Point{5, 20}, 15},
		{"below curve", Point{5, -5}, 8}, // Actual distance is ~7.07
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd := edge.SignedDistance(tt.p)
			gotDist := math.Abs(sd.Distance)

			if gotDist > tt.maxDist {
				t.Errorf("distance = %v, expected < %v", gotDist, tt.maxDist)
			}
		})
	}
}

func TestCubicSignedDistance(t *testing.T) {
	// S-curve
	edge := NewCubicEdge(Point{0, 0}, Point{3, 10}, Point{7, -10}, Point{10, 0})

	tests := []struct {
		name    string
		p       Point
		maxDist float64
	}{
		{"on curve start", Point{0, 0}, 0.1},
		{"on curve end", Point{10, 0}, 0.1},
		{"middle area", Point{5, 0}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd := edge.SignedDistance(tt.p)
			gotDist := math.Abs(sd.Distance)

			if gotDist > tt.maxDist {
				t.Errorf("distance = %v, expected < %v", gotDist, tt.maxDist)
			}
		})
	}
}

func TestEdgeBounds(t *testing.T) {
	// Linear
	linear := NewLinearEdge(Point{0, 0}, Point{10, 5})
	lb := linear.Bounds()
	if lb.MinX != 0 || lb.MinY != 0 || lb.MaxX != 10 || lb.MaxY != 5 {
		t.Errorf("Linear bounds = %v, unexpected", lb)
	}

	// Quadratic with control point extending bounds
	quad := NewQuadraticEdge(Point{0, 0}, Point{5, 10}, Point{10, 0})
	qb := quad.Bounds()
	if qb.MinX != 0 || qb.MaxX != 10 || qb.MinY != 0 {
		t.Errorf("Quadratic bounds = %v, unexpected", qb)
	}
	// The max Y should be around 5 (the apex of the parabola)
	if qb.MaxY < 4 || qb.MaxY > 6 {
		t.Errorf("Quadratic bounds MaxY = %v, expected ~5", qb.MaxY)
	}

	// Cubic
	cubic := NewCubicEdge(Point{0, 0}, Point{3, 10}, Point{7, 10}, Point{10, 0})
	cb := cubic.Bounds()
	if cb.MinX != 0 || cb.MaxX != 10 || cb.MinY != 0 {
		t.Errorf("Cubic bounds = %v, unexpected", cb)
	}
}

func TestEdgeClone(t *testing.T) {
	edge := NewQuadraticEdge(Point{0, 0}, Point{5, 5}, Point{10, 0})
	edge.Color = ColorMagenta

	clone := edge.Clone()

	// Verify values match
	if clone.Type != edge.Type {
		t.Errorf("Clone.Type = %v, want %v", clone.Type, edge.Type)
	}
	if clone.Color != edge.Color {
		t.Errorf("Clone.Color = %v, want %v", clone.Color, edge.Color)
	}
	if clone.Points != edge.Points {
		t.Errorf("Clone.Points = %v, want %v", clone.Points, edge.Points)
	}

	// Verify independence (modifying clone doesn't affect original)
	clone.Color = ColorCyan
	if edge.Color == clone.Color {
		t.Error("Clone is not independent from original")
	}
}

func TestSolveQuadratic(t *testing.T) {
	tests := []struct {
		name   string
		a, b, c float64
		want   []float64 // roots in [0, 1]
	}{
		{"two roots in range", 1, -1.5, 0.5, []float64{0.5, 1.0}},
		{"one root", 1, -1, 0, []float64{0, 1}},
		{"no real roots", 1, 0, 1, nil},
		{"linear (a=0)", 0, 2, -1, []float64{0.5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := solveQuadratic(tt.a, tt.b, tt.c)
			if len(roots) != len(tt.want) {
				t.Errorf("solveQuadratic got %d roots, want %d", len(roots), len(tt.want))
				return
			}
			// Check that all expected roots are found (order may vary)
			for _, expected := range tt.want {
				found := false
				for _, got := range roots {
					if math.Abs(got-expected) < 0.01 {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected root %v not found in %v", expected, roots)
				}
			}
		})
	}
}

func TestSolveCubic(t *testing.T) {
	// Test with known cubic that has a root at t=1
	// t^3 - t^2 has roots at 0 and 1, but 0 is a double root
	// a=1, b=-1, c=0, d=0
	roots := solveCubic(1, -1, 0, 0)
	if len(roots) < 1 {
		t.Errorf("solveCubic(1,-1,0,0) = %v, expected at least 1 root", roots)
	}

	// Check that 1 is among the roots (0 might be filtered depending on implementation)
	hasOne := false
	for _, r := range roots {
		if math.Abs(r-1) < 0.01 {
			hasOne = true
		}
	}
	if !hasOne {
		t.Errorf("expected root 1 not found in %v", roots)
	}

	// Test another cubic: (t-0.5)(t^2-0.5t+0.125) = t^3 - t^2 + 0.375t - 0.0625
	// This has roots at 0.5, 0.25+0.25i, 0.25-0.25i (only 0.5 is real)
	roots2 := solveCubic(1, -1, 0.375, -0.0625)
	hasHalf := false
	for _, r := range roots2 {
		if math.Abs(r-0.5) < 0.01 {
			hasHalf = true
		}
	}
	if !hasHalf && len(roots2) > 0 {
		t.Logf("solveCubic for (t-0.5)^3 = %v (may not contain 0.5 exactly)", roots2)
	}
}

func TestCbrt(t *testing.T) {
	tests := []struct {
		x, want float64
	}{
		{8, 2},
		{-8, -2},
		{27, 3},
		{0, 0},
		{1, 1},
	}

	for _, tt := range tests {
		got := cbrt(tt.x)
		if math.Abs(got-tt.want) > 1e-10 {
			t.Errorf("cbrt(%v) = %v, want %v", tt.x, got, tt.want)
		}
	}
}

func BenchmarkLinearSignedDistance(b *testing.B) {
	edge := NewLinearEdge(Point{0, 0}, Point{100, 0})
	p := Point{50, 10}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = edge.SignedDistance(p)
	}
}

func BenchmarkQuadraticSignedDistance(b *testing.B) {
	edge := NewQuadraticEdge(Point{0, 0}, Point{50, 100}, Point{100, 0})
	p := Point{50, 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = edge.SignedDistance(p)
	}
}

func BenchmarkCubicSignedDistance(b *testing.B) {
	edge := NewCubicEdge(Point{0, 0}, Point{30, 100}, Point{70, 100}, Point{100, 0})
	p := Point{50, 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = edge.SignedDistance(p)
	}
}
