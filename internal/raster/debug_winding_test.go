package raster

import (
	"math"
	"testing"
)

// TestWindingResidualLines — line segments: no issue expected.
func TestWindingResidualLines(t *testing.T) {
	testEllipseResidual(t, "lines", false)
}

// TestWindingResidualCurves — quadratic curves with forward differencing.
// This is the actual code path for glyph rendering.
func TestWindingResidualCurves(t *testing.T) {
	testEllipseResidual(t, "curves", true)
}

func testEllipseResidual(t *testing.T, label string, useCurves bool) {
	const w, h = 200, 40

	cx, cy := 50.0, 20.0
	rx, ry := 5.0, 7.0
	angle := 5.0 * math.Pi / 180.0
	cosA, sinA := math.Cos(angle), math.Sin(angle)
	n := 16 // fewer segments for curves (each is a quadratic)

	eb := NewEdgeBuilder(2)
	scale := float64(1 << 2)

	if useCurves {
		// Outer contour using quadratic curves
		addEllipseQuads(eb, cx, cy, rx, ry, cosA, sinA, n, scale)
		// Inner contour (reversed)
		addEllipseQuadsReverse(eb, cx, cy, rx-1.5, ry-1.5, cosA, sinA, n, scale)
	} else {
		// Outer contour using lines
		addEllipseLines(eb, cx, cy, rx, ry, cosA, sinA, 32, scale)
		// Inner contour (reversed)
		addEllipseLinesReverse(eb, cx, cy, rx-1.5, ry-1.5, cosA, sinA, 32, scale)
	}

	af := NewAnalyticFiller(w, h)

	problemScanlines := 0
	maxOverall := float32(0)
	af.WindingCallback = func(y int, winding []float32) {
		maxResidual := float32(0)
		for x := 100; x < w; x++ {
			v := winding[x]
			if v < 0 {
				v = -v
			}
			if v > maxResidual {
				maxResidual = v
			}
		}
		if maxResidual > maxOverall {
			maxOverall = maxResidual
		}
		if maxResidual > 0.01 {
			problemScanlines++
			t.Logf("y=%d: max_residual=%.6f", y, maxResidual)
		}
	}

	af.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {})

	t.Logf("%s: max_overall_residual=%.6f problem_scanlines=%d", label, maxOverall, problemScanlines)
	if problemScanlines > 0 {
		t.Errorf("%s: Found %d scanlines with residual > 0.01", label, problemScanlines)
	}
}

// addEllipseQuads builds ellipse from quadratic Bézier segments.
// Each arc segment uses a quadratic approximation with a control point.
func addEllipseQuads(eb *EdgeBuilder, cx, cy, rx, ry, cosA, sinA float64, n int, scale float64) {
	for i := 0; i < n; i++ {
		t0 := 2 * math.Pi * float64(i) / float64(n)
		tMid := 2 * math.Pi * (float64(i) + 0.5) / float64(n)
		t1 := 2 * math.Pi * float64(i+1) / float64(n)

		x0, y0 := ellipsePoint(cx, cy, rx, ry, cosA, sinA, t0)
		xM, yM := ellipsePoint(cx, cy, rx, ry, cosA, sinA, tMid)
		x1, y1 := ellipsePoint(cx, cy, rx, ry, cosA, sinA, t1)

		// Quadratic control point: 2*midpoint - 0.5*(start + end)
		ctrlX := 2*xM - 0.5*(x0+x1)
		ctrlY := 2*yM - 0.5*(y0+y1)

		eb.addQuad(
			float32(x0*scale), float32(y0*scale),
			float32(ctrlX*scale), float32(ctrlY*scale),
			float32(x1*scale), float32(y1*scale),
		)
	}
}

func addEllipseQuadsReverse(eb *EdgeBuilder, cx, cy, rx, ry, cosA, sinA float64, n int, scale float64) {
	for i := n - 1; i >= 0; i-- {
		t0 := 2 * math.Pi * float64(i+1) / float64(n)
		tMid := 2 * math.Pi * (float64(i) + 0.5) / float64(n)
		t1 := 2 * math.Pi * float64(i) / float64(n)

		x0, y0 := ellipsePoint(cx, cy, rx, ry, cosA, sinA, t0)
		xM, yM := ellipsePoint(cx, cy, rx, ry, cosA, sinA, tMid)
		x1, y1 := ellipsePoint(cx, cy, rx, ry, cosA, sinA, t1)

		ctrlX := 2*xM - 0.5*(x0+x1)
		ctrlY := 2*yM - 0.5*(y0+y1)

		eb.addQuad(
			float32(x0*scale), float32(y0*scale),
			float32(ctrlX*scale), float32(ctrlY*scale),
			float32(x1*scale), float32(y1*scale),
		)
	}
}

func addEllipseLines(eb *EdgeBuilder, cx, cy, rx, ry, cosA, sinA float64, n int, scale float64) {
	for i := 0; i < n; i++ {
		t0 := 2 * math.Pi * float64(i) / float64(n)
		t1 := 2 * math.Pi * float64(i+1) / float64(n)
		x0, y0 := ellipsePoint(cx, cy, rx, ry, cosA, sinA, t0)
		x1, y1 := ellipsePoint(cx, cy, rx, ry, cosA, sinA, t1)
		eb.addLine(float32(x0*scale), float32(y0*scale), float32(x1*scale), float32(y1*scale))
	}
}

func addEllipseLinesReverse(eb *EdgeBuilder, cx, cy, rx, ry, cosA, sinA float64, n int, scale float64) {
	for i := n - 1; i >= 0; i-- {
		t0 := 2 * math.Pi * float64(i+1) / float64(n)
		t1 := 2 * math.Pi * float64(i) / float64(n)
		x0, y0 := ellipsePoint(cx, cy, rx, ry, cosA, sinA, t0)
		x1, y1 := ellipsePoint(cx, cy, rx, ry, cosA, sinA, t1)
		eb.addLine(float32(x0*scale), float32(y0*scale), float32(x1*scale), float32(y1*scale))
	}
}

func ellipsePoint(cx, cy, rx, ry, cosA, sinA, t float64) (float64, float64) {
	lx := rx * math.Cos(t)
	ly := ry * math.Sin(t)
	return cx + lx*cosA - ly*sinA, cy + lx*sinA + ly*cosA
}
