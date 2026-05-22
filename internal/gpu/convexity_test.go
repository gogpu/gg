//go:build !nogpu

package gpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/stroke"
)

// --- Helper functions for generating test polygons ---

// makeRegularPolygon generates a regular polygon with n vertices centered at (cx, cy)
// with the given radius. Vertices are placed counter-clockwise starting from the top.
func makeRegularPolygon(cx, cy, radius float64, n int) []gg.Point {
	points := make([]gg.Point, n)
	for i := 0; i < n; i++ {
		angle := float64(i)*2*math.Pi/float64(n) - math.Pi/2
		points[i] = gg.Pt(cx+radius*math.Cos(angle), cy+radius*math.Sin(angle))
	}
	return points
}

// reversePoints returns a copy of points in reverse order (flips winding).
func reversePoints(pts []gg.Point) []gg.Point {
	n := len(pts)
	out := make([]gg.Point, n)
	for i, p := range pts {
		out[n-1-i] = p
	}
	return out
}

// makeStarPoints generates a star polygon with the given number of outer points.
// Alternates between outer and inner radii.
func makeStarPoints(cx, cy, outerR, innerR float64, nPoints int) []gg.Point {
	pts := make([]gg.Point, 0, nPoints*2)
	for i := 0; i < nPoints; i++ {
		outerAngle := float64(i)*2*math.Pi/float64(nPoints) - math.Pi/2
		pts = append(pts, gg.Pt(cx+outerR*math.Cos(outerAngle), cy+outerR*math.Sin(outerAngle)))
		innerAngle := outerAngle + math.Pi/float64(nPoints)
		pts = append(pts, gg.Pt(cx+innerR*math.Cos(innerAngle), cy+innerR*math.Sin(innerAngle)))
	}
	return pts
}

func TestIsConvex(t *testing.T) {
	tests := []struct {
		name string
		pts  []gg.Point
		want bool
	}{
		{
			name: "triangle CCW",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(100, 0),
				gg.Pt(50, 100),
			},
			want: true,
		},
		{
			name: "triangle CW",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(50, 100),
				gg.Pt(100, 0),
			},
			want: true,
		},
		{
			name: "square",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(100, 0),
				gg.Pt(100, 100),
				gg.Pt(0, 100),
			},
			want: true,
		},
		{
			name: "regular pentagon",
			pts:  makeRegularPolygon(100, 100, 50, 5),
			want: true,
		},
		{
			name: "regular hexagon",
			pts:  makeRegularPolygon(100, 100, 50, 6),
			want: true,
		},
		{
			name: "regular octagon",
			pts:  makeRegularPolygon(100, 100, 50, 8),
			want: true,
		},
		{
			name: "L-shape (concave)",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(100, 0),
				gg.Pt(100, 50),
				gg.Pt(50, 50),
				gg.Pt(50, 100),
				gg.Pt(0, 100),
			},
			want: false,
		},
		{
			name: "5-pointed star (concave)",
			pts:  makeStarPoints(100, 100, 80, 30, 5),
			want: false,
		},
		{
			name: "arrow shape (concave)",
			pts: []gg.Point{
				gg.Pt(50, 0),
				gg.Pt(100, 40),
				gg.Pt(75, 40),
				gg.Pt(75, 100),
				gg.Pt(25, 100),
				gg.Pt(25, 40),
				gg.Pt(0, 40),
			},
			want: false,
		},
		{
			name: "single point",
			pts:  []gg.Point{gg.Pt(10, 20)},
			want: false,
		},
		{
			name: "two points",
			pts:  []gg.Point{gg.Pt(0, 0), gg.Pt(100, 100)},
			want: false,
		},
		{
			name: "collinear 3 points",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(50, 0),
				gg.Pt(100, 0),
			},
			want: false,
		},
		{
			name: "collinear 5 points",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(25, 0),
				gg.Pt(50, 0),
				gg.Pt(75, 0),
				gg.Pt(100, 0),
			},
			want: false,
		},
		{
			name: "nil slice",
			pts:  nil,
			want: false,
		},
		{
			name: "empty slice",
			pts:  []gg.Point{},
			want: false,
		},
		{
			name: "rectangle with collinear midpoints (still convex)",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(50, 0), // collinear on top edge
				gg.Pt(100, 0),
				gg.Pt(100, 100),
				gg.Pt(0, 100),
			},
			want: true,
		},
		{
			name: "diamond",
			pts: []gg.Point{
				gg.Pt(50, 0),
				gg.Pt(100, 50),
				gg.Pt(50, 100),
				gg.Pt(0, 50),
			},
			want: true,
		},
		{
			name: "T-shape (concave)",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(100, 0),
				gg.Pt(100, 30),
				gg.Pt(65, 30),
				gg.Pt(65, 100),
				gg.Pt(35, 100),
				gg.Pt(35, 30),
				gg.Pt(0, 30),
			},
			want: false,
		},
		{
			name: "U-shape (concave)",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(30, 0),
				gg.Pt(30, 70),
				gg.Pt(70, 70),
				gg.Pt(70, 0),
				gg.Pt(100, 0),
				gg.Pt(100, 100),
				gg.Pt(0, 100),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConvex(tt.pts)
			if got != tt.want {
				t.Errorf("IsConvex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnalyzeConvexity(t *testing.T) {
	tests := []struct {
		name        string
		pts         []gg.Point
		wantConvex  bool
		wantWinding int
	}{
		{
			name: "CCW triangle",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(100, 0),
				gg.Pt(50, 100),
			},
			wantConvex:  true,
			wantWinding: 1,
		},
		{
			name: "CW triangle",
			pts: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(50, 100),
				gg.Pt(100, 0),
			},
			wantConvex:  true,
			wantWinding: -1,
		},
		{
			name:        "CCW square",
			pts:         []gg.Point{gg.Pt(0, 0), gg.Pt(100, 0), gg.Pt(100, 100), gg.Pt(0, 100)},
			wantConvex:  true,
			wantWinding: 1,
		},
		{
			name:        "CW square",
			pts:         reversePoints([]gg.Point{gg.Pt(0, 0), gg.Pt(100, 0), gg.Pt(100, 100), gg.Pt(0, 100)}),
			wantConvex:  true,
			wantWinding: -1,
		},
		{
			name:        "CCW regular hexagon",
			pts:         makeRegularPolygon(0, 0, 50, 6),
			wantConvex:  true,
			wantWinding: 1,
		},
		{
			name:        "CW regular hexagon",
			pts:         reversePoints(makeRegularPolygon(0, 0, 50, 6)),
			wantConvex:  true,
			wantWinding: -1,
		},
		{
			name:        "L-shape (concave)",
			pts:         []gg.Point{gg.Pt(0, 0), gg.Pt(100, 0), gg.Pt(100, 50), gg.Pt(50, 50), gg.Pt(50, 100), gg.Pt(0, 100)},
			wantConvex:  false,
			wantWinding: 0,
		},
		{
			name:        "degenerate collinear",
			pts:         []gg.Point{gg.Pt(0, 0), gg.Pt(1, 0), gg.Pt(2, 0)},
			wantConvex:  false,
			wantWinding: 0,
		},
		{
			name:        "single point",
			pts:         []gg.Point{gg.Pt(5, 5)},
			wantConvex:  false,
			wantWinding: 0,
		},
		{
			name:        "two points",
			pts:         []gg.Point{gg.Pt(0, 0), gg.Pt(1, 1)},
			wantConvex:  false,
			wantWinding: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnalyzeConvexity(tt.pts)
			if result.Convex != tt.wantConvex {
				t.Errorf("Convex = %v, want %v", result.Convex, tt.wantConvex)
			}
			if result.Winding != tt.wantWinding {
				t.Errorf("Winding = %d, want %d", result.Winding, tt.wantWinding)
			}
			if result.NumPoints != len(tt.pts) {
				t.Errorf("NumPoints = %d, want %d", result.NumPoints, len(tt.pts))
			}
		})
	}
}

func TestIsConvexRegularPolygons(t *testing.T) {
	// All regular polygons from 3 to 64 sides should be convex.
	for n := 3; n <= 64; n++ {
		pts := makeRegularPolygon(0, 0, 100, n)
		if !IsConvex(pts) {
			t.Errorf("regular %d-gon: IsConvex = false, want true", n)
		}
		// Reversed winding should also be convex.
		if !IsConvex(reversePoints(pts)) {
			t.Errorf("reversed regular %d-gon: IsConvex = false, want true", n)
		}
	}
}

func TestIsConvexStarPolygons(t *testing.T) {
	// Stars with 3 to 12 points should all be concave.
	for n := 3; n <= 12; n++ {
		pts := makeStarPoints(0, 0, 100, 40, n)
		if IsConvex(pts) {
			t.Errorf("%d-pointed star: IsConvex = true, want false", n)
		}
	}
}

func TestIsConvexIdenticalPoints(t *testing.T) {
	// All identical points: all cross products are zero, degenerate.
	pts := []gg.Point{gg.Pt(5, 5), gg.Pt(5, 5), gg.Pt(5, 5)}
	if IsConvex(pts) {
		t.Error("identical points: IsConvex = true, want false")
	}
}

func TestAnalyzeConvexityNumPoints(t *testing.T) {
	pts := makeRegularPolygon(0, 0, 50, 7)
	result := AnalyzeConvexity(pts)
	if result.NumPoints != 7 {
		t.Errorf("NumPoints = %d, want 7", result.NumPoints)
	}
}

// --- Benchmarks ---

func BenchmarkIsConvexTriangle(b *testing.B) {
	pts := []gg.Point{gg.Pt(0, 0), gg.Pt(100, 0), gg.Pt(50, 100)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsConvex(pts)
	}
}

func BenchmarkIsConvexSquare(b *testing.B) {
	pts := []gg.Point{gg.Pt(0, 0), gg.Pt(100, 0), gg.Pt(100, 100), gg.Pt(0, 100)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsConvex(pts)
	}
}

func BenchmarkIsConvex100(b *testing.B) {
	pts := makeRegularPolygon(0, 0, 100, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsConvex(pts)
	}
}

func BenchmarkIsConvex1000(b *testing.B) {
	pts := makeRegularPolygon(0, 0, 100, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsConvex(pts)
	}
}

func BenchmarkIsConvex10000(b *testing.B) {
	pts := makeRegularPolygon(0, 0, 100, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsConvex(pts)
	}
}

func BenchmarkIsConvexConcave1000(b *testing.B) {
	// Worst case for concave: failure detected at the last vertex.
	// Build an almost-convex polygon with one concavity at the end.
	pts := makeRegularPolygon(0, 0, 100, 1000)
	// Push the second-to-last point inward to make it concave.
	pts[998] = gg.Pt(0, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsConvex(pts)
	}
}

func BenchmarkAnalyzeConvexity1000(b *testing.B) {
	pts := makeRegularPolygon(0, 0, 100, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AnalyzeConvexity(pts)
	}
}

// TestStrokeExpandedSineWaveNotConvex verifies that the stroke-expanded outline
// of a 100-segment damped sine wave polyline is correctly rejected by IsConvex.
// This is the root cause of issue #347: the stroke outline has consistent
// cross-product signs (all positive) but self-intersects at inner join pivots,
// producing many Y-direction flips that the direction-flip check catches.
func TestStrokeExpandedSineWaveNotConvex(t *testing.T) {
	points := makeStrokeExpandedSineWave()
	if len(points) < 100 {
		t.Fatalf("expected many points, got %d", len(points))
	}

	result := AnalyzeConvexity(points)
	if result.Convex {
		t.Errorf("stroke-expanded sine wave should NOT be convex (issue #347), "+
			"got Convex=true with %d points", len(points))
	}

	xFlips, yFlips := countDirectionFlips(points)
	if yFlips <= maxConvexDirectionFlips {
		t.Errorf("expected many Y-direction flips for sine wave stroke, got %d", yFlips)
	}
	t.Logf("direction flips: x=%d y=%d (threshold=%d)", xFlips, yFlips, maxConvexDirectionFlips)
}

// TestDirectionFlipCheckPreservesConvexPolygons ensures the direction-flip
// check does NOT reject genuinely convex polygons (triangles, hexagons, etc).
func TestDirectionFlipCheckPreservesConvexPolygons(t *testing.T) {
	tests := []struct {
		name   string
		points []gg.Point
	}{
		{"triangle", makeRegularPolygon(0, 0, 10, 3)},
		{"square", makeRegularPolygon(0, 0, 10, 4)},
		{"pentagon", makeRegularPolygon(0, 0, 10, 5)},
		{"hexagon", makeRegularPolygon(0, 0, 10, 6)},
		{"dodecagon", makeRegularPolygon(0, 0, 10, 12)},
		{"circle_100", makeRegularPolygon(0, 0, 10, 100)},
		{"axis_aligned_rect", []gg.Point{
			gg.Pt(0, 0), gg.Pt(10, 0), gg.Pt(10, 5), gg.Pt(0, 5),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsConvex(tt.points) {
				t.Errorf("genuine convex polygon %q should pass IsConvex", tt.name)
			}
		})
	}
}

// makeStrokeExpandedSineWave reproduces the exact geometry from issue #347:
// 100-segment damped sine wave, 2px stroke, butt cap, miter join, miter limit 10.
func makeStrokeExpandedSineWave() []gg.Point {
	const n = 100

	verbs := make([]stroke.PathVerb, 0, n)
	coords := make([]float64, 0, n*2)
	for i := 0; i < n; i++ {
		t := float64(i) * 0.1
		x := 50 + t*70
		y := 250 - math.Sin(t)*math.Exp(-t*0.1)*200
		if i == 0 {
			verbs = append(verbs, stroke.VerbMoveTo)
		} else {
			verbs = append(verbs, stroke.VerbLineTo)
		}
		coords = append(coords, x, y)
	}

	style := stroke.Stroke{
		Width:      2.0,
		Cap:        stroke.LineCapButt,
		Join:       stroke.LineJoinMiter,
		MiterLimit: 10.0,
	}
	expander := stroke.NewStrokeExpander(style)
	outVerbs, outCoords := expander.Expand(verbs, coords)

	var points []gg.Point
	ci := 0
	for _, v := range outVerbs {
		switch v {
		case stroke.VerbMoveTo, stroke.VerbLineTo:
			points = append(points, gg.Pt(outCoords[ci], outCoords[ci+1]))
			ci += 2
		case stroke.VerbQuadTo:
			ci += 4
		case stroke.VerbCubicTo:
			ci += 6
		case stroke.VerbClose:
		}
	}
	return points
}
