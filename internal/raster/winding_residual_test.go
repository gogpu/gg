package raster

import (
	"math"
	"testing"
)

// Winding residual tests verify that closed contours produce zero net winding
// at pixels far outside the shape. Non-zero residuals cause horizontal line
// artifacts extending rightward from the shape (#148).
//
// Root cause: forward differencing in QuadraticEdge/CubicEdge can produce
// zero-height segments after FDot6 rounding, silently losing winding.
// Fix: EdgeBuilder.SetFlattenCurves(true) converts curves to line segments
// before scanline processing.

// TestWindingResidualLines verifies line-segment contours produce zero residual.
func TestWindingResidualLines(t *testing.T) {
	testEllipseResidual(t, "lines", false, 2)
}

// TestWindingResidualCurves verifies quadratic curve contours with aaShift=2.
func TestWindingResidualCurves(t *testing.T) {
	testEllipseResidual(t, "curves_aa2", true, 2)
}

// TestWindingResidualGlyphCounter is the critical regression test for #148.
// It reproduces the exact conditions that caused horizontal line artifacts:
//   - aaShift=4 (16x AA, same as SoftwareRenderer)
//   - Tight counter curves (outer rx=4, inner rx=2.5 — like letter 'o')
//   - Small rotation angle (3 degrees)
//   - flattenCurves=true (the production fix)
//
// This test MUST pass. If it fails, the #148 fix has regressed.
func TestWindingResidualGlyphCounter(t *testing.T) {
	const w, h = 200, 40
	const aaShift = 4 // Same as SoftwareRenderer

	angle := 3.0 * math.Pi / 180.0
	cosA, sinA := math.Cos(angle), math.Sin(angle)

	eb := NewEdgeBuilder(aaShift)
	eb.SetFlattenCurves(true) // Production configuration
	scale := float64(1 << aaShift)

	// Tight "o"-like counter: outer rx=4,ry=5 inner rx=2.5,ry=3.5
	// Small radii + small rotation = the exact trigger for #148
	cx, cy := 30.0, 20.0
	addEllipseQuads(eb, cx, cy, 4.0, 5.0, cosA, sinA, 12, scale)
	addEllipseQuadsReverse(eb, cx, cy, 2.5, 3.5, cosA, sinA, 12, scale)

	af := NewAnalyticFiller(w, h)

	problemScanlines := 0
	maxOverall := float32(0)
	af.WindingCallback = func(y int, winding []float32) {
		maxResidual := float32(0)
		// Check far-right pixels (well outside the shape at x=30)
		for x := 80; x < w; x++ {
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

	t.Logf("glyph_counter: max_overall_residual=%.6f problem_scanlines=%d", maxOverall, problemScanlines)
	if problemScanlines > 0 {
		t.Errorf("glyph_counter: Found %d scanlines with residual > 0.01 (regression of #148 fix)", problemScanlines)
	}
}

// TestWindingResidualMultipleAngles tests a range of rotation angles.
// Small angles (1-10 degrees) were the most problematic for #148.
func TestWindingResidualMultipleAngles(t *testing.T) {
	const w, h = 200, 40
	const aaShift = 4

	angles := []float64{1, 2, 3, 5, 7, 10, 15, 30, 45, 60, 89}

	for _, deg := range angles {
		t.Run(angleName(deg), func(t *testing.T) {
			angle := deg * math.Pi / 180.0
			cosA, sinA := math.Cos(angle), math.Sin(angle)

			eb := NewEdgeBuilder(aaShift)
			eb.SetFlattenCurves(true)
			scale := float64(1 << aaShift)

			cx, cy := 30.0, 20.0
			addEllipseQuads(eb, cx, cy, 4.0, 5.0, cosA, sinA, 12, scale)
			addEllipseQuadsReverse(eb, cx, cy, 2.5, 3.5, cosA, sinA, 12, scale)

			af := NewAnalyticFiller(w, h)
			maxOverall := float32(0)
			af.WindingCallback = func(y int, winding []float32) {
				for x := 80; x < w; x++ {
					v := winding[x]
					if v < 0 {
						v = -v
					}
					if v > maxOverall {
						maxOverall = v
					}
				}
			}

			af.Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {})

			t.Logf("angle=%v°: max_residual=%.6f", deg, maxOverall)
			if maxOverall > 0.01 {
				t.Errorf("angle=%v°: residual %.6f > 0.01", deg, maxOverall)
			}
		})
	}
}

func angleName(deg float64) string {
	return func() string {
		s := ""
		d := int(deg)
		if d < 10 {
			s = "0"
		}
		return s + itoa(d) + "deg"
	}()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// --- Helper functions ---

func testEllipseResidual(t *testing.T, label string, useCurves bool, aaShift int) {
	const w, h = 200, 40

	cx, cy := 50.0, 20.0
	rx, ry := 5.0, 7.0
	angle := 5.0 * math.Pi / 180.0
	cosA, sinA := math.Cos(angle), math.Sin(angle)
	n := 16
	scale := float64(int(1) << aaShift)

	eb := NewEdgeBuilder(aaShift)

	if useCurves {
		addEllipseQuads(eb, cx, cy, rx, ry, cosA, sinA, n, scale)
		addEllipseQuadsReverse(eb, cx, cy, rx-1.5, ry-1.5, cosA, sinA, n, scale)
	} else {
		addEllipseLines(eb, cx, cy, rx, ry, cosA, sinA, 32, scale)
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
func addEllipseQuads(eb *EdgeBuilder, cx, cy, rx, ry, cosA, sinA float64, n int, scale float64) {
	for i := 0; i < n; i++ {
		t0 := 2 * math.Pi * float64(i) / float64(n)
		tMid := 2 * math.Pi * (float64(i) + 0.5) / float64(n)
		t1 := 2 * math.Pi * float64(i+1) / float64(n)

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
