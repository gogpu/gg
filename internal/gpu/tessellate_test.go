//go:build !nogpu

package gpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/stroke"
)

// makeTrianglePath returns a simple triangle path (3 LineTo, no curves).
func makeTrianglePath() *gg.Path {
	p := gg.NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(50, 100)
	p.Close()
	return p
}

// makeSquarePath returns a square path (4 LineTo).
func makeSquarePath() *gg.Path {
	p := gg.NewPath()
	p.MoveTo(10, 10)
	p.LineTo(60, 10)
	p.LineTo(60, 60)
	p.LineTo(10, 60)
	p.Close()
	return p
}

// makeCirclePath returns a circle using gg.Path.Circle.
func makeCirclePath(cx, cy, r float64) *gg.Path {
	p := gg.NewPath()
	p.Circle(cx, cy, r)
	return p
}

// makeStarPath returns a 5-pointed star (10 LineTo, concave).
func makeStarPath() *gg.Path {
	const (
		cx, cy  = 100.0, 100.0
		outerR  = 80.0
		innerR  = 30.0
		nPoints = 5
	)
	p := gg.NewPath()
	for i := 0; i < nPoints; i++ {
		outerAngle := float64(i)*2*math.Pi/nPoints - math.Pi/2
		ox := cx + outerR*math.Cos(outerAngle)
		oy := cy + outerR*math.Sin(outerAngle)
		if i == 0 {
			p.MoveTo(ox, oy)
		} else {
			p.LineTo(ox, oy)
		}
		innerAngle := outerAngle + math.Pi/nPoints
		ix := cx + innerR*math.Cos(innerAngle)
		iy := cy + innerR*math.Sin(innerAngle)
		p.LineTo(ix, iy)
	}
	p.Close()
	return p
}

// makeDonutPath returns two concentric contours (outer CW, inner CCW).
func makeDonutPath() *gg.Path {
	p := gg.NewPath()
	p.Circle(100, 100, 80)
	p.Circle(100, 100, 30)
	return p
}

func TestFanTessellateEmpty(t *testing.T) {
	ft := NewFanTessellator()
	count := ft.TessellatePath(nil)
	if count != 0 {
		t.Errorf("nil path: got %d vertices, want 0", count)
	}

	count = ft.TessellatePath(gg.NewPath())
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
		elements *gg.Path
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
	p := gg.NewPath()
	p.MoveTo(0, 0)
	p.QuadraticTo(50, 100, 100, 0)
	p.Close()

	ft := NewFanTessellator()
	ft.TessellatePath(p)

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
	p := gg.NewPath()
	p.MoveTo(10, 20)
	p.MoveTo(30, 40)

	ft := NewFanTessellator()
	count := ft.TessellatePath(p)
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
	p := gg.NewPath()
	for i := 0; i < n; i++ {
		angle := float64(i)*2*math.Pi/float64(n) - math.Pi/2
		px := cx + r*math.Cos(angle)
		py := cy + r*math.Sin(angle)
		if i == 0 {
			p.MoveTo(px, py)
		} else {
			p.LineTo(px, py)
		}
	}
	p.Close()

	ft := NewFanTessellator()
	ft.TessellatePath(p)

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

// --- Stroke-expanded polygon tests (bug #347 verification) ---

// makeStrokeExpandedSineWavePath creates a stroke-expanded polygon from a
// 100-segment damped sine wave with 2px stroke width, butt cap, miter join.
// This reproduces the exact geometry from bug #347 where the convex fast-path
// incorrectly intercepts a self-intersecting stroke-expanded polygon.
//
// The stroke expander produces a single closed contour with:
//   - Forward path (outer edge)
//   - Butt cap at end
//   - Reversed backward path (inner edge)
//   - Butt cap at start (close)
//
// Inner join V-shapes (through pivot) create self-intersections.
// Adjacent duplicate points arise where forward/backward paths meet caps.
func makeStrokeExpandedSineWavePath() *gg.Path {
	// Build a damped sine wave: 100 segments, amplitude decaying from 50 to ~5.
	const (
		nSegments  = 100
		startX     = 50.0
		endX       = 744.0
		baseY      = 150.0
		startAmp   = 50.0
		dampFactor = 0.97
	)

	srcPath := gg.NewPath()
	dx := (endX - startX) / float64(nSegments)

	srcPath.MoveTo(startX, baseY)
	amp := startAmp
	for i := 1; i <= nSegments; i++ {
		x := startX + dx*float64(i)
		y := baseY + amp*math.Sin(float64(i)*0.3)
		srcPath.LineTo(x, y)
		amp *= dampFactor
	}

	// Convert to stroke verbs/coords, expand, convert back to gg.Path.
	verbs := srcPath.Verbs()
	coords := srcPath.Coords()

	strokeVerbs := make([]strokePathVerb, len(verbs))
	for i, v := range verbs {
		strokeVerbs[i] = strokePathVerb(v)
	}

	style := strokeStyle{
		Width:      2.0,
		Cap:        strokeLineCapButt,
		Join:       strokeLineJoinMiter,
		MiterLimit: 4.0,
	}
	expander := newStrokeExpander(style)
	outVerbs, outCoords := expander.Expand(strokeVerbs, coords)

	result := gg.NewPath()
	ci := 0
	for _, v := range outVerbs {
		switch v {
		case strokeVerbMoveTo:
			result.MoveTo(outCoords[ci], outCoords[ci+1])
			ci += 2
		case strokeVerbLineTo:
			result.LineTo(outCoords[ci], outCoords[ci+1])
			ci += 2
		case strokeVerbClose:
			result.Close()
		}
	}
	return result
}

// strokePathVerb, strokeStyle, etc. are type aliases to access internal/stroke
// without creating an import cycle in this test file. The test exercises
// FanTessellator directly with the geometry that StrokePath would produce.
type strokePathVerb = stroke.PathVerb
type strokeStyle = stroke.Stroke
type strokeLineCap = stroke.LineCap
type strokeLineJoin = stroke.LineJoin

const (
	strokeVerbMoveTo    strokePathVerb = stroke.VerbMoveTo
	strokeVerbLineTo    strokePathVerb = stroke.VerbLineTo
	strokeVerbClose     strokePathVerb = stroke.VerbClose
	strokeLineCapButt   strokeLineCap  = stroke.LineCapButt
	strokeLineJoinMiter strokeLineJoin = stroke.LineJoinMiter
)

var newStrokeExpander = stroke.NewStrokeExpander

// TestFanTessellateStrokeExpandedSineWave verifies that the FanTessellator
// produces a reasonable tessellation for a stroke-expanded polygon.
//
// Context: bug #347 — the convex fast-path incorrectly intercepts stroke-expanded
// fills because it doesn't check fill rule. Stroke fills use EvenOdd, which
// requires stencil-then-cover. This test verifies that FanTessellator handles
// the specific geometry correctly:
//   - ~396 point polygon with self-intersections (from inner join V-shapes)
//   - Bounding box ~694x100 but actual stroke band only 2px wide
//   - Multiple adjacent near-duplicate points (at join pivots)
func TestFanTessellateStrokeExpandedSineWave(t *testing.T) {
	path := makeStrokeExpandedSineWavePath()
	if path.NumVerbs() == 0 {
		t.Fatal("stroke-expanded path is empty")
	}

	ft := NewFanTessellator()
	vertexCount := ft.TessellatePath(path)

	// The path should have many points (outer + inner edge + joins).
	// With 100 segments, butt caps, and miter joins, we expect ~200 outer +
	// ~200 inner + join vertices + cap vertices. Total should be in the
	// range of 300-600 vertices in the path, producing many fan triangles.
	triangles := ft.TriangleCount()
	if triangles < 50 {
		t.Errorf("stroke-expanded sine wave: got %d triangles, expected at least 50", triangles)
	}
	if vertexCount != triangles*3 {
		t.Errorf("vertex count %d != triangles*3 %d", vertexCount, triangles*3)
	}

	// Verify bounds are reasonable: the sine wave spans ~50..744 in X.
	// Y range depends on damped sine: baseY=150, startAmp=50, dampFactor=0.97.
	// The sine is sampled at 0.3 rad increments, so full excursion requires
	// ~10 segments to reach sin(3.0)=0.14 — the actual peak is at sin(~1.5)=~1.0
	// with amp*sin(~5*0.3) at ~segment 5. With damping, vertical range is roughly
	// 150 +/- 45. Stroke adds 1px on each side.
	bounds := ft.Bounds()
	if bounds[0] > 52 {
		t.Errorf("minX = %f, expected < 52 (sine wave starts at x=50)", bounds[0])
	}
	if bounds[2] < 742 {
		t.Errorf("maxX = %f, expected > 742 (sine wave ends at x=744)", bounds[2])
	}
	// Y bounds: just verify they're within the plausible range of the damped sine.
	// The actual minimum Y depends on the sine phase and damping, so we use
	// generous bounds rather than exact values.
	if bounds[1] > 125 {
		t.Errorf("minY = %f, expected < 125 (damped sine should dip below baseY=150)", bounds[1])
	}
	if bounds[3] < 185 {
		t.Errorf("maxY = %f, expected > 185 (damped sine should rise above baseY=150)", bounds[3])
	}

	t.Logf("stroke-expanded sine wave: %d triangles, %d vertices, bounds=%v",
		triangles, vertexCount, bounds)
}

// TestStrokeExpandedPathIsNotConvex verifies that the stroke-expanded polygon
// from a sine wave is NOT detected as convex by IsConvex.
// This is the root cause of bug #347: if IsConvex returns true for this
// geometry, the convex fast-path (Tier 2a) would incorrectly render it.
func TestStrokeExpandedPathIsNotConvex(t *testing.T) {
	path := makeStrokeExpandedSineWavePath()

	// Extract all LineTo points from the path (same logic as extractConvexPolygon).
	var points []gg.Point
	moveCount := 0
	hasCurves := false
	closed := false

	path.Iterate(func(verb gg.PathVerb, coords []float64) {
		if hasCurves {
			return
		}
		switch verb {
		case gg.MoveTo:
			moveCount++
			if moveCount > 1 {
				hasCurves = true
				return
			}
			points = append(points, gg.Pt(coords[0], coords[1]))
		case gg.LineTo:
			points = append(points, gg.Pt(coords[0], coords[1]))
		case gg.QuadTo, gg.CubicTo:
			hasCurves = true
		case gg.Close:
			closed = true
		}
	})

	if hasCurves {
		t.Log("stroke-expanded path contains curves — convex check N/A, stencil will handle it")
		return
	}
	if !closed {
		t.Fatal("stroke-expanded path is not closed — stroke expander bug?")
	}

	t.Logf("stroke-expanded polygon: %d points, closed=%v", len(points), closed)

	// The polygon MUST NOT be convex — it has inner join V-shapes and
	// self-intersections. If IsConvex returns true, the convex fast-path
	// would intercept it, which is bug #347.
	if IsConvex(points) {
		t.Errorf("CRITICAL: stroke-expanded sine wave detected as convex! "+
			"This means the convex fast-path (Tier 2a) would incorrectly "+
			"intercept it instead of stencil-then-cover (Tier 2b). "+
			"Points=%d", len(points))
	}
}

// TestDegenerateTriangleSkipSafety verifies that skipping degenerate triangles
// (cross product == 0) in FanTessellator.emitFanTriangle does NOT break
// EvenOdd stencil parity for paths with duplicate adjacent points.
//
// The stencil-then-cover algorithm counts how many times each pixel is covered
// by the fan triangles. With EvenOdd, odd count = inside, even count = outside.
// A degenerate triangle (zero area) covers zero pixels, so skipping it is
// equivalent to adding it — the stencil count doesn't change either way.
//
// This test constructs a simple diamond path with deliberate duplicate points
// and verifies the tessellation is topologically correct.
func TestDegenerateTriangleSkipSafety(t *testing.T) {
	// Diamond with duplicated vertices at each corner.
	// The inner join V-shape pattern produces sequences like:
	//   ..., P_n, P_pivot, P_pivot, P_n+1, ...
	// where P_pivot appears twice (handleInnerJoin routes through pivot).
	p := gg.NewPath()
	p.MoveTo(100, 50)  // Top
	p.LineTo(150, 100) // Right
	p.LineTo(150, 100) // Duplicate — simulates inner join pivot
	p.LineTo(100, 150) // Bottom
	p.LineTo(100, 150) // Duplicate
	p.LineTo(50, 100)  // Left
	p.LineTo(50, 100)  // Duplicate
	p.LineTo(100, 50)  // Back to top (duplicate of start)
	p.Close()

	ft := NewFanTessellator()
	vertexCount := ft.TessellatePath(p)

	// Fan from (100,50):
	// (100,50)→(150,100): first LineTo, no triangle yet
	// (150,100)→(150,100): triangle (100,50),(150,100),(150,100) — DEGENERATE, skipped
	// (150,100)→(100,150): triangle (100,50),(150,100),(100,150) — valid
	// (100,150)→(100,150): triangle (100,50),(100,150),(100,150) — DEGENERATE, skipped
	// (100,150)→(50,100): triangle (100,50),(100,150),(50,100) — valid
	// (50,100)→(50,100): triangle (100,50),(50,100),(50,100) — DEGENERATE, skipped
	// (50,100)→(100,50): triangle (100,50),(50,100),(100,50) — DEGENERATE (v0==v2), skipped
	// Close: triangle (100,50),(100,50),(100,50) — DEGENERATE, skipped
	//
	// Expected: 2 valid triangles (the diamond body).
	// The duplicates produce degenerate triangles that are correctly skipped.
	// This is SAFE because degenerate triangles have zero area and don't
	// affect stencil parity.
	triangles := ft.TriangleCount()
	if triangles != 2 {
		t.Errorf("diamond with duplicates: got %d triangles, want 2", triangles)
	}
	if vertexCount != triangles*3 {
		t.Errorf("vertex count %d != triangles*3 %d", vertexCount, triangles*3)
	}

	// Also verify bounds encompass the full diamond.
	bounds := ft.Bounds()
	if bounds[0] != 50 || bounds[1] != 50 || bounds[2] != 150 || bounds[3] != 150 {
		t.Errorf("bounds = %v, want [50 50 150 150]", bounds)
	}
}

// TestFanTessellateRingTopology verifies fan tessellation of a ring-shaped
// contour (outer + inner edge). This is the exact topology that stroke
// expansion produces: a single closed contour that traces the outer edge
// forward, caps across, traces the inner edge backward, and caps back.
//
// With EvenOdd fill rule + stencil-then-cover:
//   - Pixels in the stroke band are covered by an odd number of fan triangles
//   - Pixels in the hollow center are covered by an even number
//   - The stencil invert operation correctly toggles parity
func TestFanTessellateRingTopology(t *testing.T) {
	// Construct a simple rectangular ring (stroke-like) manually:
	// Outer rectangle: (0,0)→(100,0)→(100,50)→(0,50)
	// Inner rectangle (reversed): (5,5)←(95,5)←(95,45)←(5,45)
	// Connected by "cap" segments at the ends.
	p := gg.NewPath()
	// Outer edge (forward, top-right)
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 50)
	p.LineTo(0, 50)
	// Right "cap" transition to inner edge
	p.LineTo(0, 45)
	// Inner edge (reversed, going right to left at the bottom, then up)
	p.LineTo(5, 45)
	p.LineTo(5, 5)
	p.LineTo(95, 5)
	p.LineTo(95, 45)
	// Left "cap" transition back
	p.LineTo(0, 45)
	p.Close()

	ft := NewFanTessellator()
	vertexCount := ft.TessellatePath(p)

	triangles := ft.TriangleCount()
	if triangles < 5 {
		t.Errorf("ring topology: got %d triangles, expected at least 5", triangles)
	}
	if vertexCount != triangles*3 {
		t.Errorf("vertex count %d != triangles*3 %d", vertexCount, triangles*3)
	}

	// This ring-shaped contour is NOT convex.
	var points []gg.Point
	p.Iterate(func(verb gg.PathVerb, coords []float64) {
		switch verb {
		case gg.MoveTo, gg.LineTo:
			points = append(points, gg.Pt(coords[0], coords[1]))
		}
	})
	if IsConvex(points) {
		t.Error("ring topology should NOT be convex")
	}

	t.Logf("ring topology: %d points, %d triangles", len(points), triangles)
}

func BenchmarkFanTessellateCircle(b *testing.B) {
	p := makeCirclePath(200, 200, 100)
	ft := NewFanTessellator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.Reset()
		ft.TessellatePath(p)
	}
}

func BenchmarkFanTessellateSquare(b *testing.B) {
	p := makeSquarePath()
	ft := NewFanTessellator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.Reset()
		ft.TessellatePath(p)
	}
}
