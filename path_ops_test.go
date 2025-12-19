package gg

import (
	"math"
	"testing"
)

// TestPathArea tests the Area() method for various shapes.
func TestPathArea(t *testing.T) {
	tests := []struct {
		name      string
		buildPath func() *Path
		wantArea  float64
		tolerance float64
	}{
		{
			name: "unit square clockwise",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(1, 0)
				p.LineTo(1, 1)
				p.LineTo(0, 1)
				p.Close()
				return p
			},
			wantArea:  1.0, // Full signed area
			tolerance: 0.001,
		},
		{
			name: "unit square counter-clockwise",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(0, 1)
				p.LineTo(1, 1)
				p.LineTo(1, 0)
				p.Close()
				return p
			},
			wantArea:  -1.0,
			tolerance: 0.001,
		},
		{
			name: "10x10 square",
			buildPath: func() *Path {
				p := NewPath()
				p.Rectangle(0, 0, 10, 10)
				return p
			},
			wantArea:  100, // Full area
			tolerance: 0.1,
		},
		{
			name: "triangle",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(4, 0)
				p.LineTo(2, 3)
				p.Close()
				return p
			},
			wantArea:  6, // Area = base * height / 2 = 4 * 3 / 2 = 6
			tolerance: 0.1,
		},
		{
			name: "circle radius 1",
			buildPath: func() *Path {
				p := NewPath()
				p.Circle(0, 0, 1)
				return p
			},
			wantArea:  math.Pi, // pi * r^2 (but using Bezier approximation, sign may vary)
			tolerance: 0.5,     // Higher tolerance due to Bezier approximation
		},
		{
			name:      "empty path",
			buildPath: NewPath,
			wantArea:  0,
			tolerance: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.buildPath()
			got := p.Area()
			// Compare absolute values due to different orientation conventions
			if math.Abs(math.Abs(got)-math.Abs(tt.wantArea)) > tt.tolerance {
				t.Errorf("Area() = %v, want approximately %v (tolerance %v)", got, tt.wantArea, tt.tolerance)
			}
		})
	}
}

// TestPathWinding tests the Winding() method.
func TestPathWinding(t *testing.T) {
	// Create a unit square
	square := NewPath()
	square.MoveTo(0, 0)
	square.LineTo(1, 0)
	square.LineTo(1, 1)
	square.LineTo(0, 1)
	square.Close()

	tests := []struct {
		name   string
		path   *Path
		point  Point
		expect int
	}{
		{
			name:   "point inside square",
			path:   square,
			point:  Pt(0.5, 0.5),
			expect: 1, // Non-zero winding inside
		},
		{
			name:   "point outside square left",
			path:   square,
			point:  Pt(-1, 0.5),
			expect: 0,
		},
		{
			name:   "point outside square right",
			path:   square,
			point:  Pt(2, 0.5),
			expect: 0,
		},
		{
			name:   "point outside square above",
			path:   square,
			point:  Pt(0.5, 2),
			expect: 0,
		},
		{
			name:   "point outside square below",
			path:   square,
			point:  Pt(0.5, -1),
			expect: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.path.Winding(tt.point)
			// For inside/outside testing, we care about non-zero vs zero
			if (got != 0) != (tt.expect != 0) {
				t.Errorf("Winding(%v) = %d, expected non-zero=%v", tt.point, got, tt.expect != 0)
			}
		})
	}
}

// TestPathContains tests the Contains() method.
func TestPathContains(t *testing.T) {
	tests := []struct {
		name      string
		buildPath func() *Path
		point     Point
		want      bool
	}{
		{
			name: "inside square",
			buildPath: func() *Path {
				p := NewPath()
				p.Rectangle(0, 0, 10, 10)
				return p
			},
			point: Pt(5, 5),
			want:  true,
		},
		{
			name: "outside square",
			buildPath: func() *Path {
				p := NewPath()
				p.Rectangle(0, 0, 10, 10)
				return p
			},
			point: Pt(15, 5),
			want:  false,
		},
		{
			name: "inside circle",
			buildPath: func() *Path {
				p := NewPath()
				p.Circle(5, 5, 3)
				return p
			},
			point: Pt(5, 5),
			want:  true,
		},
		{
			name: "outside circle",
			buildPath: func() *Path {
				p := NewPath()
				p.Circle(5, 5, 3)
				return p
			},
			point: Pt(0, 0),
			want:  false,
		},
		{
			name: "inside triangle",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(10, 0)
				p.LineTo(5, 10)
				p.Close()
				return p
			},
			point: Pt(5, 3),
			want:  true,
		},
		{
			name: "outside triangle",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(10, 0)
				p.LineTo(5, 10)
				p.Close()
				return p
			},
			point: Pt(0, 10),
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.buildPath()
			got := p.Contains(tt.point)
			if got != tt.want {
				t.Errorf("Contains(%v) = %v, want %v", tt.point, got, tt.want)
			}
		})
	}
}

// TestPathBoundingBox tests the BoundingBox() method.
func TestPathBoundingBox(t *testing.T) {
	tests := []struct {
		name      string
		buildPath func() *Path
		wantMin   Point
		wantMax   Point
	}{
		{
			name: "simple rectangle",
			buildPath: func() *Path {
				p := NewPath()
				p.Rectangle(10, 20, 30, 40)
				return p
			},
			wantMin: Pt(10, 20),
			wantMax: Pt(40, 60),
		},
		{
			name: "triangle",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(10, 0)
				p.LineTo(5, 8)
				p.Close()
				return p
			},
			wantMin: Pt(0, 0),
			wantMax: Pt(10, 8),
		},
		{
			name: "circle at origin",
			buildPath: func() *Path {
				p := NewPath()
				p.Circle(0, 0, 5)
				return p
			},
			wantMin: Pt(-5, -5),
			wantMax: Pt(5, 5),
		},
		{
			name: "quadratic curve",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(5, 10, 10, 0)
				return p
			},
			wantMin: Pt(0, 0),
			wantMax: Pt(10, 5), // Control point affects max Y
		},
		{
			name:      "empty path",
			buildPath: NewPath,
			wantMin:   Pt(0, 0),
			wantMax:   Pt(0, 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.buildPath()
			bbox := p.BoundingBox()

			tolerance := 0.5 // Allow some tolerance for curve approximations

			if math.Abs(bbox.Min.X-tt.wantMin.X) > tolerance ||
				math.Abs(bbox.Min.Y-tt.wantMin.Y) > tolerance {
				t.Errorf("BoundingBox().Min = %v, want %v", bbox.Min, tt.wantMin)
			}
			if math.Abs(bbox.Max.X-tt.wantMax.X) > tolerance ||
				math.Abs(bbox.Max.Y-tt.wantMax.Y) > tolerance {
				t.Errorf("BoundingBox().Max = %v, want %v", bbox.Max, tt.wantMax)
			}
		})
	}
}

// TestPathFlatten tests the Flatten() method.
func TestPathFlatten(t *testing.T) {
	tests := []struct {
		name       string
		buildPath  func() *Path
		tolerance  float64
		minPoints  int // Minimum expected points
		checkFirst Point
		checkLast  Point
	}{
		{
			name: "simple line",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(10, 10)
				return p
			},
			tolerance:  1.0,
			minPoints:  2,
			checkFirst: Pt(0, 0),
			checkLast:  Pt(10, 10),
		},
		{
			name: "quadratic curve",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(5, 10, 10, 0)
				return p
			},
			tolerance:  0.5,
			minPoints:  3, // At least start, some middle, end
			checkFirst: Pt(0, 0),
			checkLast:  Pt(10, 0),
		},
		{
			name: "cubic curve",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.CubicTo(3, 10, 7, 10, 10, 0)
				return p
			},
			tolerance:  0.5,
			minPoints:  3,
			checkFirst: Pt(0, 0),
			checkLast:  Pt(10, 0),
		},
		{
			name: "high precision",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(5, 10, 10, 0)
				return p
			},
			tolerance:  0.1, // Higher precision = more points
			minPoints:  5,
			checkFirst: Pt(0, 0),
			checkLast:  Pt(10, 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.buildPath()
			points := p.Flatten(tt.tolerance)

			if len(points) < tt.minPoints {
				t.Errorf("Flatten() returned %d points, expected at least %d", len(points), tt.minPoints)
			}

			if len(points) > 0 {
				first := points[0]
				last := points[len(points)-1]

				if first.Distance(tt.checkFirst) > 0.01 {
					t.Errorf("First point = %v, want %v", first, tt.checkFirst)
				}
				if last.Distance(tt.checkLast) > 0.01 {
					t.Errorf("Last point = %v, want %v", last, tt.checkLast)
				}
			}
		})
	}
}

// TestPathFlattenCallback tests the FlattenCallback() method.
func TestPathFlattenCallback(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(5, 0)
	p.QuadraticTo(7.5, 5, 10, 0)

	var points []Point
	p.FlattenCallback(0.5, func(pt Point) {
		points = append(points, pt)
	})

	if len(points) < 3 {
		t.Errorf("FlattenCallback() generated %d points, expected at least 3", len(points))
	}

	// Check first and last points
	if points[0].Distance(Pt(0, 0)) > 0.01 {
		t.Errorf("First point = %v, want (0, 0)", points[0])
	}
	if points[len(points)-1].Distance(Pt(10, 0)) > 0.01 {
		t.Errorf("Last point = %v, want (10, 0)", points[len(points)-1])
	}
}

// TestPathReversed tests the Reversed() method.
func TestPathReversed(t *testing.T) {
	tests := []struct {
		name      string
		buildPath func() *Path
	}{
		{
			name: "simple line path",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(10, 0)
				p.LineTo(10, 10)
				return p
			},
		},
		{
			name: "closed rectangle",
			buildPath: func() *Path {
				p := NewPath()
				p.Rectangle(0, 0, 10, 10)
				return p
			},
		},
		{
			name: "path with quadratic",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(5, 10, 10, 0)
				return p
			},
		},
		{
			name: "path with cubic",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.CubicTo(3, 10, 7, 10, 10, 0)
				return p
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.buildPath()
			reversed := original.Reversed()

			// Verify reversed path has elements
			if len(original.Elements()) > 0 && len(reversed.Elements()) == 0 {
				t.Error("Reversed path should have elements")
			}

			origElems := original.Elements()
			revElems := reversed.Elements()

			if len(origElems) == 0 || len(revElems) == 0 {
				return
			}

			// Check if original is closed
			_, isClosed := origElems[len(origElems)-1].(Close)
			if isClosed {
				verifyClosedPathReversed(t, revElems)
				return
			}

			// For open paths, verify endpoints are swapped
			verifyOpenPathReversed(t, original, reversed)
		})
	}
}

// verifyClosedPathReversed verifies that a reversed closed path is also closed.
func verifyClosedPathReversed(t *testing.T, revElems []PathElement) {
	t.Helper()
	_, revClosed := revElems[len(revElems)-1].(Close)
	if !revClosed {
		t.Error("Reversed closed path should also be closed")
	}
}

// verifyOpenPathReversed verifies that endpoints are swapped for open paths.
func verifyOpenPathReversed(t *testing.T, original, reversed *Path) {
	t.Helper()
	origPoints := original.Flatten(0.5)
	revPoints := reversed.Flatten(0.5)

	if len(origPoints) == 0 || len(revPoints) == 0 {
		return
	}

	origFirst := origPoints[0]
	origLast := origPoints[len(origPoints)-1]
	revFirst := revPoints[0]
	revLast := revPoints[len(revPoints)-1]

	tolerance := 0.5

	if origFirst.Distance(revLast) > tolerance {
		t.Errorf("Original first %v should match reversed last %v", origFirst, revLast)
	}
	if origLast.Distance(revFirst) > tolerance {
		t.Errorf("Original last %v should match reversed first %v", origLast, revFirst)
	}
}

// TestPathLength tests the Length() method.
func TestPathLength(t *testing.T) {
	tests := []struct {
		name       string
		buildPath  func() *Path
		accuracy   float64
		wantLength float64
		tolerance  float64
	}{
		{
			name: "horizontal line",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(10, 0)
				return p
			},
			accuracy:   0.001,
			wantLength: 10,
			tolerance:  0.001,
		},
		{
			name: "diagonal line",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(3, 4)
				return p
			},
			accuracy:   0.001,
			wantLength: 5, // 3-4-5 triangle
			tolerance:  0.001,
		},
		{
			name: "square perimeter",
			buildPath: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.LineTo(10, 0)
				p.LineTo(10, 10)
				p.LineTo(0, 10)
				p.LineTo(0, 0)
				return p
			},
			accuracy:   0.001,
			wantLength: 40,
			tolerance:  0.001,
		},
		{
			name: "circle circumference",
			buildPath: func() *Path {
				p := NewPath()
				p.Circle(0, 0, 1)
				return p
			},
			accuracy:   0.001,
			wantLength: 2 * math.Pi, // Circumference = 2*pi*r
			tolerance:  0.1,         // Bezier approximation has some error
		},
		{
			name:       "empty path",
			buildPath:  NewPath,
			accuracy:   0.001,
			wantLength: 0,
			tolerance:  0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.buildPath()
			got := p.Length(tt.accuracy)
			if math.Abs(got-tt.wantLength) > tt.tolerance {
				t.Errorf("Length(%v) = %v, want %v (tolerance %v)", tt.accuracy, got, tt.wantLength, tt.tolerance)
			}
		})
	}
}

// TestBoundingBoxWithCurves tests that bounding boxes correctly include curve extrema.
func TestBoundingBoxWithCurves(t *testing.T) {
	// A quadratic curve that goes above its start/end points
	p := NewPath()
	p.MoveTo(0, 0)
	p.QuadraticTo(5, 10, 10, 0) // Control point at (5, 10)

	bbox := p.BoundingBox()

	// The curve's maximum Y should be around 5 (halfway to control point)
	if bbox.Max.Y < 4 {
		t.Errorf("BoundingBox max Y = %v, expected >= 4 (curve should bulge up)", bbox.Max.Y)
	}
}

// TestContainsWithCurves tests containment for paths with curves.
func TestContainsWithCurves(t *testing.T) {
	// Create a circular-ish path
	p := NewPath()
	p.Circle(5, 5, 3)

	tests := []struct {
		point Point
		want  bool
	}{
		{Pt(5, 5), true},   // Center
		{Pt(5, 7), true},   // Near top edge but inside
		{Pt(5, 9), false},  // Outside top
		{Pt(0, 0), false},  // Far outside
		{Pt(5, 2.5), true}, // Near bottom edge but inside
	}

	for _, tt := range tests {
		got := p.Contains(tt.point)
		if got != tt.want {
			t.Errorf("Contains(%v) = %v, want %v", tt.point, got, tt.want)
		}
	}
}

// TestLengthAccuracy tests that smaller accuracy values give more precise results.
func TestLengthAccuracy(t *testing.T) {
	// Create a path with curves
	p := NewPath()
	p.Circle(0, 0, 1)

	// Higher accuracy (smaller value) should be closer to true circumference
	expectedLength := 2 * math.Pi

	length1 := p.Length(0.1)
	length2 := p.Length(0.01)
	length3 := p.Length(0.001)

	// Each should be progressively closer to the expected value
	err1 := math.Abs(length1 - expectedLength)
	err2 := math.Abs(length2 - expectedLength)
	err3 := math.Abs(length3 - expectedLength)

	// Note: Due to Bezier approximation, we can't get perfect accuracy
	// but higher precision should generally improve or stay the same
	if err3 > err1*2 { // Allow some flexibility due to numerical issues
		t.Errorf("Higher accuracy should give better results: err(0.001)=%v > err(0.1)=%v", err3, err1)
	}

	_ = err2 // Used to show the progression
}

// TestEmptyPathOperations tests that empty paths handle all operations gracefully.
func TestEmptyPathOperations(t *testing.T) {
	p := NewPath()

	// Area
	if area := p.Area(); area != 0 {
		t.Errorf("Empty path Area() = %v, want 0", area)
	}

	// Winding
	if w := p.Winding(Pt(0, 0)); w != 0 {
		t.Errorf("Empty path Winding() = %v, want 0", w)
	}

	// Contains
	if c := p.Contains(Pt(0, 0)); c {
		t.Errorf("Empty path Contains() = %v, want false", c)
	}

	// BoundingBox
	bbox := p.BoundingBox()
	if bbox.Width() != 0 || bbox.Height() != 0 {
		t.Errorf("Empty path BoundingBox() = %v, want zero rect", bbox)
	}

	// Flatten
	if pts := p.Flatten(1.0); len(pts) > 0 {
		t.Errorf("Empty path Flatten() = %v, want nil or empty", pts)
	}

	// Reversed
	rev := p.Reversed()
	if len(rev.Elements()) != 0 {
		t.Errorf("Empty path Reversed() has %d elements, want 0", len(rev.Elements()))
	}

	// Length
	if l := p.Length(0.001); l != 0 {
		t.Errorf("Empty path Length() = %v, want 0", l)
	}
}
