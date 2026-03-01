package gg

import (
	"math"
	"testing"
)

// TestPathArea_Quadratic tests Area() with paths containing quadratic curves.
func TestPathArea_Quadratic(t *testing.T) {
	tests := []struct {
		name      string
		buildPath func() *Path
		wantArea  float64
		tolerance float64
	}{
		{
			name: "closed quadratic curve",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(5, 10, 10, 0)
				p.Close()
				return p
			},
			// For a parabolic segment: area = 2/3 * base * height
			// base = 10, height ~= 5 (quadratic reaches midpoint of control)
			// actual area = 2/3 * 10 * 5 = 33.33
			wantArea:  33.33,
			tolerance: 1.0,
		},
		{
			name: "quadratic half-circle approximation",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(5, 8, 10, 0)
				p.Close()
				return p
			},
			wantArea:  26.67, // 2/3 * 10 * 4
			tolerance: 2.0,
		},
		{
			name: "mixed line and quadratic",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(10, 0)
				p.QuadraticTo(10, 10, 0, 10)
				p.Close()
				return p
			},
			wantArea:  66.67, // Approximate area of the shape
			tolerance: 10.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.buildPath()
			got := math.Abs(p.Area())
			if math.Abs(got-tt.wantArea) > tt.tolerance {
				t.Errorf("Area() = %v, want approximately %v (tolerance %v)", got, tt.wantArea, tt.tolerance)
			}
		})
	}
}

// TestPathWinding_Quadratic tests Winding() with quadratic curve paths.
func TestPathWinding_Quadratic(t *testing.T) {
	// Create a closed path with a quadratic curve that bulges up
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.QuadraticTo(10, 10, 0, 10)
	p.LineTo(0, 0)
	p.Close()

	tests := []struct {
		name     string
		point    Point
		isInside bool
	}{
		{"center inside", Pt(3, 3), true},
		{"far outside", Pt(20, 20), false},
		{"outside left", Pt(-5, 5), false},
		{"below path", Pt(5, -5), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := p.Winding(tt.point)
			gotInside := w != 0
			if gotInside != tt.isInside {
				t.Errorf("Winding(%v) = %d, inside=%v, want inside=%v", tt.point, w, gotInside, tt.isInside)
			}
		})
	}
}

// TestPathLength_Quadratic tests Length() with quadratic curve paths.
func TestPathLength_Quadratic(t *testing.T) {
	tests := []struct {
		name       string
		buildPath  func() *Path
		wantLength float64
		tolerance  float64
	}{
		{
			name: "straight-ish quadratic (control on chord)",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(5, 0, 10, 0) // Control on the line
				return p
			},
			wantLength: 10.0,
			tolerance:  0.1,
		},
		{
			name: "curved quadratic",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(5, 10, 10, 0)
				return p
			},
			// Arc length of parabola from (0,0) to (10,0) with control (5,10)
			// Should be longer than chord (10) but shorter than polygon (10+10=~22)
			wantLength: 15.0,
			tolerance:  3.0,
		},
		{
			name: "multiple quadratics",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(2, 5, 5, 0)
				p.QuadraticTo(8, -5, 10, 0)
				return p
			},
			wantLength: 16.0,
			tolerance:  4.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.buildPath()
			got := p.Length(0.001)
			if math.Abs(got-tt.wantLength) > tt.tolerance {
				t.Errorf("Length() = %v, want approximately %v (tolerance %v)", got, tt.wantLength, tt.tolerance)
			}
		})
	}
}

// TestPathLength_ZeroAccuracy tests that zero accuracy gets default.
func TestPathLength_ZeroAccuracy(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 0)

	// Should not panic and should return correct length
	got := p.Length(0) // zero accuracy → uses default
	if math.Abs(got-10.0) > 0.01 {
		t.Errorf("Length(0) = %v, want 10.0", got)
	}

	// Negative accuracy → uses default too
	got2 := p.Length(-1)
	if math.Abs(got2-10.0) > 0.01 {
		t.Errorf("Length(-1) = %v, want 10.0", got2)
	}
}

// TestFlattenCallback_ZeroTolerance tests that zero tolerance gets default.
func TestFlattenCallback_ZeroTolerance(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 0)

	var points []Point
	p.FlattenCallback(0, func(pt Point) {
		points = append(points, pt)
	})

	if len(points) < 2 {
		t.Errorf("FlattenCallback(0) returned %d points, want at least 2", len(points))
	}
}

// TestFlattenCallback_MultipleSubpaths tests flattening with multiple subpaths.
func TestFlattenCallback_MultipleSubpaths(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(5, 0)
	p.MoveTo(10, 10)
	p.LineTo(15, 10)

	var points []Point
	p.FlattenCallback(0.5, func(pt Point) {
		points = append(points, pt)
	})

	// Should have points from both subpaths
	if len(points) < 4 {
		t.Errorf("FlattenCallback() returned %d points for 2 subpaths, want at least 4", len(points))
	}
}

// TestFlattenCallback_ClosedPath tests flattening of a closed path.
func TestFlattenCallback_ClosedPath(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.LineTo(10, 10)
	p.Close()

	var points []Point
	p.FlattenCallback(0.5, func(pt Point) {
		points = append(points, pt)
	})

	if len(points) < 3 {
		t.Errorf("FlattenCallback() returned %d points for closed triangle, want at least 3", len(points))
	}

	// Last point should be back at start (close)
	last := points[len(points)-1]
	if last.Distance(Pt(0, 0)) > 0.01 {
		t.Errorf("Last point after Close = %v, want (0, 0)", last)
	}
}

// TestPathReversed_QuadraticClosed tests reversing a closed path with quadratics.
func TestPathReversed_QuadraticClosed(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.QuadraticTo(5, 10, 10, 0)
	p.Close()

	rev := p.Reversed()
	revElems := rev.Elements()

	if len(revElems) == 0 {
		t.Fatal("Reversed path has no elements")
	}

	// Should also be closed
	_, isClosed := revElems[len(revElems)-1].(Close)
	if !isClosed {
		t.Error("Reversed closed path should also be closed")
	}

	// Area should have same magnitude but opposite sign
	origArea := p.Area()
	revArea := rev.Area()
	if math.Abs(origArea+revArea) > 1.0 {
		t.Errorf("Reversed area should negate: orig=%v, reversed=%v", origArea, revArea)
	}
}

// TestPathContains_QuadraticShape tests Contains with a quadratic curve boundary.
func TestPathContains_QuadraticShape(t *testing.T) {
	// Create a shape with quadratic curves
	p := NewPath()
	p.MoveTo(0, 0)
	p.QuadraticTo(5, 10, 10, 0)
	p.Close()

	tests := []struct {
		name string
		pt   Point
		want bool
	}{
		{"inside bottom center", Pt(5, 1), true},
		{"outside above curve", Pt(5, 8), false},
		{"outside left", Pt(-2, 0), false},
		{"outside right", Pt(12, 0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Contains(tt.pt)
			if got != tt.want {
				t.Errorf("Contains(%v) = %v, want %v", tt.pt, got, tt.want)
			}
		})
	}
}

// TestPathBoundingBox_QuadraticBulge tests BoundingBox with quadratic that bulges.
func TestPathBoundingBox_QuadraticBulge(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.QuadraticTo(5, 10, 10, 0) // Control at (5, 10) causes upward bulge

	bbox := p.BoundingBox()

	// Quadratic reaches max Y at midpoint between chord and control
	// Max Y should be around 5 (midpoint of 0 and 10)
	if bbox.Max.Y < 4 || bbox.Max.Y > 6 {
		t.Errorf("BoundingBox max Y = %v, expected ~5.0 for quadratic bulge", bbox.Max.Y)
	}

	if bbox.Min.X > 0.01 || bbox.Max.X < 9.99 {
		t.Errorf("BoundingBox X range = [%v, %v], expected [0, 10]", bbox.Min.X, bbox.Max.X)
	}
}

// TestLineArea tests the lineArea helper function.
func TestLineArea(t *testing.T) {
	tests := []struct {
		name string
		p0   Point
		p1   Point
		want float64
	}{
		{"horizontal right", Pt(0, 0), Pt(1, 0), 0},
		{"diagonal", Pt(0, 0), Pt(1, 1), 0},
		{"triangle contrib", Pt(1, 0), Pt(0, 1), 0.5},
		{"reverse", Pt(0, 1), Pt(1, 0), -0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lineArea(tt.p0, tt.p1)
			if math.Abs(got-tt.want) > 1e-10 {
				t.Errorf("lineArea(%v, %v) = %v, want %v", tt.p0, tt.p1, got, tt.want)
			}
		})
	}
}

// TestIsLeft tests the isLeft helper.
func TestIsLeft(t *testing.T) {
	tests := []struct {
		name string
		p0   Point
		p1   Point
		pt   Point
		sign int // 1 = left, -1 = right, 0 = on line
	}{
		{"point left of line", Pt(0, 0), Pt(10, 0), Pt(5, 5), 1},
		{"point right of line", Pt(0, 0), Pt(10, 0), Pt(5, -5), -1},
		{"point on line", Pt(0, 0), Pt(10, 0), Pt(5, 0), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLeft(tt.p0, tt.p1, tt.pt)
			switch tt.sign {
			case 1:
				if got <= 0 {
					t.Errorf("isLeft = %v, want positive (left)", got)
				}
			case -1:
				if got >= 0 {
					t.Errorf("isLeft = %v, want negative (right)", got)
				}
			case 0:
				if got != 0 {
					t.Errorf("isLeft = %v, want 0 (on line)", got)
				}
			}
		})
	}
}

// TestCubicFlatness tests the cubicFlatness helper.
func TestCubicFlatness(t *testing.T) {
	// A perfectly straight cubic should have zero (or very small) flatness
	straight := NewCubicBez(Pt(0, 0), Pt(1, 0), Pt(2, 0), Pt(3, 0))
	flatness := cubicFlatness(straight)
	if flatness > 1e-10 {
		t.Errorf("Straight cubic flatness = %v, want ~0", flatness)
	}

	// A curved cubic should have non-zero flatness
	curved := NewCubicBez(Pt(0, 0), Pt(0, 10), Pt(10, 10), Pt(10, 0))
	flatness = cubicFlatness(curved)
	if flatness < 1.0 {
		t.Errorf("Curved cubic flatness = %v, want > 1.0", flatness)
	}
}

// TestLineWinding tests the lineWinding helper function.
func TestLineWinding(t *testing.T) {
	tests := []struct {
		name string
		p0   Point
		p1   Point
		pt   Point
		want int
	}{
		{"upward crossing left", Pt(0, 0), Pt(0, 10), Pt(-1, 5), 1},
		{"no crossing horizontal", Pt(0, 0), Pt(10, 0), Pt(5, 5), 0},
		{"downward crossing right", Pt(0, 10), Pt(0, 0), Pt(-1, 5), -1},
		{"point above segment", Pt(0, 0), Pt(0, 5), Pt(-1, 10), 0},
		{"point below segment", Pt(0, 5), Pt(0, 10), Pt(-1, 0), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lineWinding(tt.p0, tt.p1, tt.pt)
			if got != tt.want {
				t.Errorf("lineWinding(%v, %v, %v) = %d, want %d", tt.p0, tt.p1, tt.pt, got, tt.want)
			}
		})
	}
}
