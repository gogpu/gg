//go:build !nogpu

package gpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
)

// makeTrianglePath returns a simple triangle path (3 LineTo, no curves).
func makeTrianglePath() []gg.PathElement {
	return []gg.PathElement{
		gg.MoveTo{Point: gg.Pt(0, 0)},
		gg.LineTo{Point: gg.Pt(100, 0)},
		gg.LineTo{Point: gg.Pt(50, 100)},
		gg.Close{},
	}
}

// makeSquarePath returns a square path (4 LineTo).
func makeSquarePath() []gg.PathElement {
	return []gg.PathElement{
		gg.MoveTo{Point: gg.Pt(10, 10)},
		gg.LineTo{Point: gg.Pt(60, 10)},
		gg.LineTo{Point: gg.Pt(60, 60)},
		gg.LineTo{Point: gg.Pt(10, 60)},
		gg.Close{},
	}
}

// makeCirclePath returns a circle using 4 cubic Beziers (standard kappa approximation).
func makeCirclePath(cx, cy, r float64) []gg.PathElement {
	const k = 0.5522847498307936
	off := r * k
	return []gg.PathElement{
		gg.MoveTo{Point: gg.Pt(cx+r, cy)},
		gg.CubicTo{Control1: gg.Pt(cx+r, cy+off), Control2: gg.Pt(cx+off, cy+r), Point: gg.Pt(cx, cy+r)},
		gg.CubicTo{Control1: gg.Pt(cx-off, cy+r), Control2: gg.Pt(cx-r, cy+off), Point: gg.Pt(cx-r, cy)},
		gg.CubicTo{Control1: gg.Pt(cx-r, cy-off), Control2: gg.Pt(cx-off, cy-r), Point: gg.Pt(cx, cy-r)},
		gg.CubicTo{Control1: gg.Pt(cx+off, cy-r), Control2: gg.Pt(cx+r, cy-off), Point: gg.Pt(cx+r, cy)},
		gg.Close{},
	}
}

// makeStarPath returns a 5-pointed star (10 LineTo, concave).
func makeStarPath() []gg.PathElement {
	const (
		cx, cy  = 100.0, 100.0
		outerR  = 80.0
		innerR  = 30.0
		nPoints = 5
	)
	elems := make([]gg.PathElement, 0, 2*nPoints+2)

	for i := 0; i < nPoints; i++ {
		// Outer point
		outerAngle := float64(i)*2*math.Pi/nPoints - math.Pi/2
		ox := cx + outerR*math.Cos(outerAngle)
		oy := cy + outerR*math.Sin(outerAngle)
		if i == 0 {
			elems = append(elems, gg.MoveTo{Point: gg.Pt(ox, oy)})
		} else {
			elems = append(elems, gg.LineTo{Point: gg.Pt(ox, oy)})
		}
		// Inner point
		innerAngle := outerAngle + math.Pi/nPoints
		ix := cx + innerR*math.Cos(innerAngle)
		iy := cy + innerR*math.Sin(innerAngle)
		elems = append(elems, gg.LineTo{Point: gg.Pt(ix, iy)})
	}
	elems = append(elems, gg.Close{})
	return elems
}

// makeDonutPath returns two concentric contours (outer CW, inner CCW).
func makeDonutPath() []gg.PathElement {
	outer := makeCirclePath(100, 100, 80)
	inner := makeCirclePath(100, 100, 30)
	// Combine both contours into one path
	elems := make([]gg.PathElement, 0, len(outer)+len(inner))
	elems = append(elems, outer...)
	elems = append(elems, inner...)
	return elems
}

func TestFanTessellateEmpty(t *testing.T) {
	ft := NewFanTessellator()
	count := ft.TessellatePath(nil)
	if count != 0 {
		t.Errorf("nil path: got %d vertices, want 0", count)
	}

	count = ft.TessellatePath([]gg.PathElement{})
	if count != 0 {
		t.Errorf("empty path: got %d vertices, want 0", count)
	}

	if len(ft.Vertices()) != 0 {
		t.Errorf("vertices should be empty, got %d floats", len(ft.Vertices()))
	}
}

func TestFanTessellateTriangle(t *testing.T) {
	ft := NewFanTessellator()
	vertexCount := ft.TessellatePath(makeTrianglePath())

	// Triangle with 3 edges from fan center:
	// MoveTo(0,0) is fan center
	// LineTo(100,0) -> no triangle yet (only 2 vertices)
	// LineTo(50,100) -> triangle (0,0), (100,0), (50,100)
	// Close -> triangle (0,0), (50,100), (0,0) -> degenerate (v0==v2), skipped
	// So we expect 1 triangle = 3 vertices = 6 floats
	if vertexCount != 3 {
		t.Errorf("triangle: got %d vertices, want 3", vertexCount)
	}
	if ft.TriangleCount() != 1 {
		t.Errorf("triangle: got %d triangles, want 1", ft.TriangleCount())
	}

	// Verify bounds
	bounds := ft.Bounds()
	if bounds[0] != 0 || bounds[1] != 0 || bounds[2] != 100 || bounds[3] != 100 {
		t.Errorf("triangle bounds: got %v, want [0 0 100 100]", bounds)
	}
}

func TestFanTessellateSquare(t *testing.T) {
	ft := NewFanTessellator()
	vertexCount := ft.TessellatePath(makeSquarePath())

	// Square with 4 edges from fan center (10,10):
	// LineTo(60,10) -> no triangle (2nd vertex)
	// LineTo(60,60) -> triangle (10,10), (60,10), (60,60)
	// LineTo(10,60) -> triangle (10,10), (60,60), (10,60)
	// Close -> triangle (10,10), (10,60), (10,10) -> degenerate, skipped
	// So we expect 2 triangles = 6 vertices = 12 floats
	if vertexCount != 6 {
		t.Errorf("square: got %d vertices, want 6", vertexCount)
	}
	if ft.TriangleCount() != 2 {
		t.Errorf("square: got %d triangles, want 2", ft.TriangleCount())
	}

	bounds := ft.Bounds()
	if bounds[0] != 10 || bounds[1] != 10 || bounds[2] != 60 || bounds[3] != 60 {
		t.Errorf("square bounds: got %v, want [10 10 60 60]", bounds)
	}
}

func TestFanTessellateCircle(t *testing.T) {
	ft := NewFanTessellator()
	vertexCount := ft.TessellatePath(makeCirclePath(200, 200, 100))

	// A circle made of 4 cubics should produce many fan triangles after flattening.
	// With tolerance 0.25 we expect roughly 32-128 triangles depending on subdivision.
	triangles := ft.TriangleCount()
	if triangles < 16 {
		t.Errorf("circle: got %d triangles, expected at least 16", triangles)
	}
	if triangles > 512 {
		t.Errorf("circle: got %d triangles, expected at most 512 (over-tessellated?)", triangles)
	}
	if vertexCount != triangles*3 {
		t.Errorf("circle: vertex count %d != triangles*3 %d", vertexCount, triangles*3)
	}

	// Bounds should approximately contain the circle
	bounds := ft.Bounds()
	const eps = 1.0
	if bounds[0] > 100+eps || bounds[1] > 100+eps {
		t.Errorf("circle bounds min: got (%f, %f), want near (100, 100)", bounds[0], bounds[1])
	}
	if bounds[2] < 300-eps || bounds[3] < 300-eps {
		t.Errorf("circle bounds max: got (%f, %f), want near (300, 300)", bounds[2], bounds[3])
	}
}

func TestFanTessellateStar(t *testing.T) {
	ft := NewFanTessellator()
	vertexCount := ft.TessellatePath(makeStarPath())

	// Star has 10 LineTo edges forming the star shape.
	// Fan from first vertex (outer top point):
	// 10 edges -> 10-1=9 potential fan triangles from LineTo
	// Close adds 1 closing triangle if not degenerate
	// Some may be degenerate if collinear, but unlikely for a star.
	// Expected: ~9 triangles (first LineTo doesn't produce a triangle)
	triangles := ft.TriangleCount()
	if triangles < 7 {
		t.Errorf("star: got %d triangles, expected at least 7", triangles)
	}
	if triangles > 12 {
		t.Errorf("star: got %d triangles, expected at most 12", triangles)
	}
	if vertexCount != triangles*3 {
		t.Errorf("star: vertex count %d != triangles*3 %d", vertexCount, triangles*3)
	}
}

func TestFanTessellateDonut(t *testing.T) {
	ft := NewFanTessellator()
	vertexCount := ft.TessellatePath(makeDonutPath())

	// Donut has 2 contours, each with 4 cubics.
	// Each contour should produce separate fan triangles.
	triangles := ft.TriangleCount()
	if triangles < 32 {
		t.Errorf("donut: got %d triangles, expected at least 32 (two contours)", triangles)
	}
	if vertexCount != triangles*3 {
		t.Errorf("donut: vertex count %d != triangles*3 %d", vertexCount, triangles*3)
	}
}

func TestFanTessellateAABB(t *testing.T) {
	tests := []struct {
		name     string
		elements []gg.PathElement
		wantMinX float32
		wantMinY float32
		wantMaxX float32
		wantMaxY float32
		approx   bool // use approximate comparison
		eps      float32
	}{
		{
			name:     "triangle",
			elements: makeTrianglePath(),
			wantMinX: 0, wantMinY: 0, wantMaxX: 100, wantMaxY: 100,
		},
		{
			name:     "square",
			elements: makeSquarePath(),
			wantMinX: 10, wantMinY: 10, wantMaxX: 60, wantMaxY: 60,
		},
		{
			name:     "circle at origin",
			elements: makeCirclePath(0, 0, 50),
			wantMinX: -50, wantMinY: -50, wantMaxX: 50, wantMaxY: 50,
			approx: true,
			eps:    1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := NewFanTessellator()
			ft.TessellatePath(tt.elements)
			bounds := ft.Bounds()

			eps := tt.eps
			if !tt.approx {
				eps = 0.001
			}

			if math.Abs(float64(bounds[0]-tt.wantMinX)) > float64(eps) {
				t.Errorf("minX: got %f, want %f (eps=%f)", bounds[0], tt.wantMinX, eps)
			}
			if math.Abs(float64(bounds[1]-tt.wantMinY)) > float64(eps) {
				t.Errorf("minY: got %f, want %f (eps=%f)", bounds[1], tt.wantMinY, eps)
			}
			if math.Abs(float64(bounds[2]-tt.wantMaxX)) > float64(eps) {
				t.Errorf("maxX: got %f, want %f (eps=%f)", bounds[2], tt.wantMaxX, eps)
			}
			if math.Abs(float64(bounds[3]-tt.wantMaxY)) > float64(eps) {
				t.Errorf("maxY: got %f, want %f (eps=%f)", bounds[3], tt.wantMaxY, eps)
			}
		})
	}
}

func TestFanTessellateCoverQuad(t *testing.T) {
	ft := NewFanTessellator()
	ft.TessellatePath(makeSquarePath())
	quad := ft.CoverQuad()

	// Square bounds: [10, 10, 60, 60]
	// With 1px padding: [9, 9, 61, 61]
	wantMinX := float32(10 - fanCoverPadding)
	wantMinY := float32(10 - fanCoverPadding)
	wantMaxX := float32(60 + fanCoverPadding)
	wantMaxY := float32(60 + fanCoverPadding)

	// Triangle 1: (minX, minY), (maxX, minY), (maxX, maxY)
	if quad[0] != wantMinX || quad[1] != wantMinY {
		t.Errorf("quad[0:2] = (%f, %f), want (%f, %f)", quad[0], quad[1], wantMinX, wantMinY)
	}
	if quad[2] != wantMaxX || quad[3] != wantMinY {
		t.Errorf("quad[2:4] = (%f, %f), want (%f, %f)", quad[2], quad[3], wantMaxX, wantMinY)
	}
	if quad[4] != wantMaxX || quad[5] != wantMaxY {
		t.Errorf("quad[4:6] = (%f, %f), want (%f, %f)", quad[4], quad[5], wantMaxX, wantMaxY)
	}

	// Triangle 2: (minX, minY), (maxX, maxY), (minX, maxY)
	if quad[6] != wantMinX || quad[7] != wantMinY {
		t.Errorf("quad[6:8] = (%f, %f), want (%f, %f)", quad[6], quad[7], wantMinX, wantMinY)
	}
	if quad[8] != wantMaxX || quad[9] != wantMaxY {
		t.Errorf("quad[8:10] = (%f, %f), want (%f, %f)", quad[8], quad[9], wantMaxX, wantMaxY)
	}
	if quad[10] != wantMinX || quad[11] != wantMaxY {
		t.Errorf("quad[10:12] = (%f, %f), want (%f, %f)", quad[10], quad[11], wantMinX, wantMaxY)
	}
}

func TestFanTessellateReset(t *testing.T) {
	ft := NewFanTessellator()

	// First tessellation
	ft.TessellatePath(makeSquarePath())
	if ft.TriangleCount() == 0 {
		t.Fatal("expected triangles after first tessellation")
	}

	// Reset and tessellate again
	ft.Reset()
	if len(ft.Vertices()) != 0 {
		t.Errorf("after reset: vertices should be empty, got %d", len(ft.Vertices()))
	}
	if ft.hasBounds {
		t.Error("after reset: hasBounds should be false")
	}

	// Tessellate a different path
	ft.TessellatePath(makeTrianglePath())
	if ft.TriangleCount() != 1 {
		t.Errorf("after reset+triangle: got %d triangles, want 1", ft.TriangleCount())
	}
}

func TestFanTessellateQuadBezier(t *testing.T) {
	// A simple quadratic curve that should be subdivided
	elements := []gg.PathElement{
		gg.MoveTo{Point: gg.Pt(0, 0)},
		gg.QuadTo{Control: gg.Pt(50, 100), Point: gg.Pt(100, 0)},
		gg.Close{},
	}

	ft := NewFanTessellator()
	ft.TessellatePath(elements)

	// A quadratic with high curvature should produce multiple fan triangles
	triangles := ft.TriangleCount()
	if triangles < 2 {
		t.Errorf("quad bezier: got %d triangles, expected at least 2", triangles)
	}

	// Bounds should include the control point's influence area
	bounds := ft.Bounds()
	if bounds[3] < 40 {
		t.Errorf("quad bezier: maxY=%f, expected > 40 (curve should bulge)", bounds[3])
	}
}

func TestFanTessellateVertexLayout(t *testing.T) {
	ft := NewFanTessellator()
	ft.TessellatePath(makeTrianglePath())

	verts := ft.Vertices()
	// 1 triangle = 6 floats
	if len(verts) != 6 {
		t.Fatalf("expected 6 floats for 1 triangle, got %d", len(verts))
	}

	// The single triangle should be (0,0), (100,0), (50,100)
	// Fan origin is (0,0), first LineTo is (100,0), second LineTo is (50,100)
	const eps = 0.001
	checkFloat := func(name string, got, want float32) {
		if math.Abs(float64(got-want)) > eps {
			t.Errorf("%s: got %f, want %f", name, got, want)
		}
	}

	checkFloat("v0.x", verts[0], 0)
	checkFloat("v0.y", verts[1], 0)
	checkFloat("v1.x", verts[2], 100)
	checkFloat("v1.y", verts[3], 0)
	checkFloat("v2.x", verts[4], 50)
	checkFloat("v2.y", verts[5], 100)
}

func TestFanTessellateOnlyMoveTo(t *testing.T) {
	// Path with only MoveTo elements should produce no triangles
	elements := []gg.PathElement{
		gg.MoveTo{Point: gg.Pt(10, 20)},
		gg.MoveTo{Point: gg.Pt(30, 40)},
	}

	ft := NewFanTessellator()
	count := ft.TessellatePath(elements)
	if count != 0 {
		t.Errorf("moveto-only: got %d vertices, want 0", count)
	}
}

func TestFanTessellatePentagon(t *testing.T) {
	// Regular pentagon: 5 edges -> 3 fan triangles (fan from vertex 0)
	const (
		cx, cy = 100.0, 100.0
		r      = 50.0
		n      = 5
	)
	elems := make([]gg.PathElement, 0, n+2)
	for i := 0; i < n; i++ {
		angle := float64(i)*2*math.Pi/float64(n) - math.Pi/2
		px := cx + r*math.Cos(angle)
		py := cy + r*math.Sin(angle)
		if i == 0 {
			elems = append(elems, gg.MoveTo{Point: gg.Pt(px, py)})
		} else {
			elems = append(elems, gg.LineTo{Point: gg.Pt(px, py)})
		}
	}
	elems = append(elems, gg.Close{})

	ft := NewFanTessellator()
	ft.TessellatePath(elems)

	// Pentagon: 5 vertices, fan from v0:
	// LineTo v1: no triangle yet
	// LineTo v2: triangle (v0, v1, v2)
	// LineTo v3: triangle (v0, v2, v3)
	// LineTo v4: triangle (v0, v3, v4)
	// Close: triangle (v0, v4, v0) -> degenerate, skipped
	// Expected: 3 triangles
	if ft.TriangleCount() != 3 {
		t.Errorf("pentagon: got %d triangles, want 3", ft.TriangleCount())
	}
}

func BenchmarkFanTessellateCircle(b *testing.B) {
	elements := makeCirclePath(200, 200, 100)
	ft := NewFanTessellator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.Reset()
		ft.TessellatePath(elements)
	}
}

func BenchmarkFanTessellateSquare(b *testing.B) {
	elements := makeSquarePath()
	ft := NewFanTessellator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.Reset()
		ft.TessellatePath(elements)
	}
}
