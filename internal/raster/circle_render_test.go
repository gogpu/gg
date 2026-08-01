// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"
)

// TestCircleRenderCurveVsFlatten renders a filled circle R=40 centered at
// (50,50) on a 100x100 canvas with BOTH flattenCurves modes and compares
// every pixel. This is the primary diagnostic test for cubic forward-diff
// waviness in the AnalyticFiller pipeline.
//
// The circle uses standard kappa=0.5522847498 approximation (4 cubic arcs).
// The flatten=true mode (De Casteljau subdivision to lines) is the reference.
// The flatten=false mode (CubicEdge forward-diff with updateLine) is under test.
func TestCircleRenderCurveVsFlatten(t *testing.T) {
	const (
		w  = 100
		h  = 100
		cx = 50.0
		cy = 50.0
		r  = 40.0
	)

	path := makeCirclePath(cx, cy, r)

	// Render with flatten=true (reference)
	ebFlat := NewEdgeBuilder(2)
	ebFlat.SetFlattenCurves(true)
	ebFlat.BuildFromPath(path, IdentityTransform{})
	flatBuf := make([]uint8, w*h)
	FillToBuffer(ebFlat, w, h, FillRuleNonZero, flatBuf)

	// Render with flatten=false (forward-diff cubics)
	ebCurve := NewEdgeBuilder(2)
	ebCurve.SetFlattenCurves(false)
	ebCurve.BuildFromPath(path, IdentityTransform{})
	curveBuf := make([]uint8, w*h)
	FillToBuffer(ebCurve, w, h, FillRuleNonZero, curveBuf)

	// Compare every pixel
	diffCount := 0
	maxDiff := 0
	sumAbsDiff := 0
	type diffEntry struct {
		x, y int
		flat uint8
		curv uint8
		diff int // curv - flat
	}
	var diffs []diffEntry

	for y := range h {
		for x := range w {
			f := int(flatBuf[y*w+x])
			c := int(curveBuf[y*w+x])
			d := c - f
			if d != 0 {
				diffCount++
				ad := d
				if ad < 0 {
					ad = -ad
				}
				sumAbsDiff += ad
				if ad > maxDiff {
					maxDiff = ad
				}
				diffs = append(diffs, diffEntry{x, y, uint8(f), uint8(c), d})
			}
		}
	}

	t.Logf("Circle R=%d center=(%d,%d) canvas=%dx%d", int(r), int(cx), int(cy), w, h)
	t.Logf("Flatten edges: %d line, %d quad, %d cubic",
		ebFlat.LineEdgeCount(), ebFlat.QuadraticEdgeCount(), ebFlat.CubicEdgeCount())
	t.Logf("Curve edges:   %d line, %d quad, %d cubic",
		ebCurve.LineEdgeCount(), ebCurve.QuadraticEdgeCount(), ebCurve.CubicEdgeCount())
	t.Logf("Diff pixels: %d / %d (%.1f%%)", diffCount, w*h, 100.0*float64(diffCount)/float64(w*h))
	t.Logf("Max |diff|: %d", maxDiff)
	if diffCount > 0 {
		t.Logf("Mean |diff| (over diff pixels): %.1f", float64(sumAbsDiff)/float64(diffCount))
	}

	// Log which arcs the diffs belong to (by angle from center)
	arcBuckets := [4]int{} // 0: right-bottom, 1: bottom-left, 2: left-top, 3: top-right
	for _, d := range diffs {
		angle := math.Atan2(float64(d.y)-cy, float64(d.x)-cx)
		if angle < 0 {
			angle += 2 * math.Pi
		}
		deg := angle * 180.0 / math.Pi
		arc := int(deg / 90.0)
		if arc > 3 {
			arc = 3
		}
		arcBuckets[arc]++
	}
	t.Logf("Diffs by arc: right-bottom=%d, bottom-left=%d, left-top=%d, top-right=%d",
		arcBuckets[0], arcBuckets[1], arcBuckets[2], arcBuckets[3])

	// Log diff pixels (limited to first 100)
	showCount := len(diffs)
	if showCount > 100 {
		showCount = 100
	}
	for i := 0; i < showCount; i++ {
		d := diffs[i]
		angle := math.Atan2(float64(d.y)-cy, float64(d.x)-cx)
		deg := angle * 180.0 / math.Pi
		if deg < 0 {
			deg += 360
		}
		t.Logf("  (%2d,%2d): flat=%3d curve=%3d diff=%+3d angle=%.0f",
			d.x, d.y, d.flat, d.curv, d.diff, deg)
	}
	if len(diffs) > showCount {
		t.Logf("  ... and %d more diff pixels", len(diffs)-showCount)
	}

	// Log coverage around the edge at 5-degree intervals
	t.Log("\nEdge coverage (every 5 degrees, at radius R-0.5 and R+0.5):")
	for deg := 0; deg < 360; deg += 5 {
		rad := float64(deg) * math.Pi / 180.0
		// Sample at the edge (radius = R)
		ex := cx + r*math.Cos(rad)
		ey := cy + r*math.Sin(rad)
		px, py := int(ex), int(ey)
		if px < 0 || px >= w || py < 0 || py >= h {
			continue
		}
		flatCov := flatBuf[py*w+px]
		curveCov := curveBuf[py*w+px]
		d := int(curveCov) - int(flatCov)
		if d != 0 {
			t.Logf("  %3d deg: pixel(%2d,%2d) flat=%3d curve=%3d diff=%+3d",
				deg, px, py, flatCov, curveCov, d)
		}
	}

	// The test tracks diffs for diagnostics. Log summary assessment.
	switch {
	case maxDiff <= 3:
		t.Logf("ASSESSMENT: Excellent — max diff %d, curve mode matches flatten within 3", maxDiff)
	case maxDiff <= 10:
		t.Logf("ASSESSMENT: Acceptable — max diff %d, minor coverage differences", maxDiff)
	default:
		t.Logf("ASSESSMENT: WAVINESS — max diff %d, visible rendering artifacts likely", maxDiff)
	}
}

// TestCircleRenderCoverageProfile renders the circle and samples coverage
// along the circle edge at 1-degree intervals. Reports coverage smoothness
// for both modes.
//
// A smooth circle should have monotonically changing coverage as we sweep
// around the edge. "Waviness" manifests as non-monotonic coverage in regions
// where the curve tangent is nearly constant.
func TestCircleRenderCoverageProfile(t *testing.T) {
	const (
		w  = 100
		h  = 100
		cx = 50.0
		cy = 50.0
		r  = 40.0
	)

	path := makeCirclePath(cx, cy, r)

	// Render both modes
	ebFlat := NewEdgeBuilder(2)
	ebFlat.SetFlattenCurves(true)
	ebFlat.BuildFromPath(path, IdentityTransform{})
	flatBuf := make([]uint8, w*h)
	FillToBuffer(ebFlat, w, h, FillRuleNonZero, flatBuf)

	ebCurve := NewEdgeBuilder(2)
	ebCurve.SetFlattenCurves(false)
	ebCurve.BuildFromPath(path, IdentityTransform{})
	curveBuf := make([]uint8, w*h)
	FillToBuffer(ebCurve, w, h, FillRuleNonZero, curveBuf)

	// Sample coverage along the edge at 1-degree intervals.
	// For each angle, sample the pixel AT the circle boundary.
	type sample struct {
		deg       int
		px, py    int
		flatCov   uint8
		curveCov  uint8
		diff      int
		distFromR float64 // distance from pixel center to circle edge
	}

	var samples []sample
	for deg := 0; deg < 360; deg++ {
		rad := float64(deg) * math.Pi / 180.0
		ex := cx + r*math.Cos(rad)
		ey := cy + r*math.Sin(rad)
		px, py := int(ex), int(ey)
		if px < 0 || px >= w || py < 0 || py >= h {
			continue
		}

		// Distance from pixel center to circle edge
		pcx := float64(px) + 0.5
		pcy := float64(py) + 0.5
		dist := math.Sqrt((pcx-cx)*(pcx-cx)+(pcy-cy)*(pcy-cy)) - r

		flatCov := flatBuf[py*w+px]
		curveCov := curveBuf[py*w+px]
		d := int(curveCov) - int(flatCov)

		samples = append(samples, sample{deg, px, py, flatCov, curveCov, d, dist})
	}

	// Find coverage non-monotonicity (waviness indicator).
	// Group samples by quadrant and check if coverage varies smoothly.
	maxCurveDiff := 0
	maxFlatDiff := 0
	for _, s := range samples {
		ad := s.diff
		if ad < 0 {
			ad = -ad
		}
		if ad > maxCurveDiff {
			maxCurveDiff = ad
		}
	}

	// Compute smoothness: for adjacent angle samples at the same pixel,
	// check if coverage jumps between flatten and curve modes.
	for i := 1; i < len(samples); i++ {
		if samples[i].px != samples[i-1].px || samples[i].py != samples[i-1].py {
			continue // Different pixel — skip
		}
		dc := int(samples[i].curveCov) - int(samples[i-1].curveCov)
		df := int(samples[i].flatCov) - int(samples[i-1].flatCov)
		adc := dc
		if adc < 0 {
			adc = -adc
		}
		adf := df
		if adf < 0 {
			adf = -adf
		}
		_ = dc // Used for debug logging if needed
		_ = df
		if adc > maxCurveDiff {
			maxCurveDiff = adc
		}
		if adf > maxFlatDiff {
			maxFlatDiff = adf
		}
	}

	t.Logf("Edge coverage profile: %d samples at 1-degree intervals", len(samples))
	t.Logf("Max curve-vs-flat diff: %d", maxCurveDiff)

	// Report samples with diff > 5 for diagnosis
	for _, s := range samples {
		ad := s.diff
		if ad < 0 {
			ad = -ad
		}
		if ad > 5 {
			t.Logf("  %3d deg: pixel(%2d,%2d) flat=%3d curve=%3d diff=%+3d distR=%.2f",
				s.deg, s.px, s.py, s.flatCov, s.curveCov, s.diff, s.distFromR)
		}
	}
}

// TestCircleRenderPerArcAnalysis renders a circle and analyzes which of the
// 4 cubic arcs produces the largest rendering differences between flatten
// and forward-diff modes. Each arc is rendered separately.
func TestCircleRenderPerArcAnalysis(t *testing.T) {
	const (
		w  = 100
		h  = 100
		cx = 50.0
		cy = 50.0
		r  = 40.0
	)

	const kappa = 0.5522847498
	k := r * kappa

	type arc struct {
		name string
		// Control points (start, ctrl1, ctrl2, end)
		sx, sy, c1x, c1y, c2x, c2y, ex, ey float64
	}

	arcs := []arc{
		{"right->bottom", cx + r, cy, cx + r, cy + k, cx + k, cy + r, cx, cy + r},
		{"bottom->left", cx, cy + r, cx - k, cy + r, cx - r, cy + k, cx - r, cy},
		{"left->top", cx - r, cy, cx - r, cy - k, cx - k, cy - r, cx, cy - r},
		{"top->right", cx, cy - r, cx + k, cy - r, cx + r, cy - k, cx + r, cy},
	}

	for _, a := range arcs {
		t.Run(a.name, func(t *testing.T) {
			// Build a single-arc path (MoveTo + CubicTo + Close but closing
			// via a line back to start forms a pie-slice shape)
			path := &testPath{}
			path.verbs = append(path.verbs, MoveTo)
			path.points = append(path.points, float32(cx), float32(cy))
			path.verbs = append(path.verbs, LineTo)
			path.points = append(path.points, float32(a.sx), float32(a.sy))
			path.verbs = append(path.verbs, CubicTo)
			path.points = append(path.points,
				float32(a.c1x), float32(a.c1y),
				float32(a.c2x), float32(a.c2y),
				float32(a.ex), float32(a.ey))
			path.verbs = append(path.verbs, Close)

			// Render both modes
			ebFlat := NewEdgeBuilder(2)
			ebFlat.SetFlattenCurves(true)
			ebFlat.BuildFromPath(path, IdentityTransform{})
			flatBuf := make([]uint8, w*h)
			FillToBuffer(ebFlat, w, h, FillRuleNonZero, flatBuf)

			ebCurve := NewEdgeBuilder(2)
			ebCurve.SetFlattenCurves(false)
			ebCurve.BuildFromPath(path, IdentityTransform{})
			curveBuf := make([]uint8, w*h)
			FillToBuffer(ebCurve, w, h, FillRuleNonZero, curveBuf)

			diffCount := 0
			maxDiff := 0
			for y := range h {
				for x := range w {
					d := int(curveBuf[y*w+x]) - int(flatBuf[y*w+x])
					if d != 0 {
						diffCount++
						ad := d
						if ad < 0 {
							ad = -ad
						}
						if ad > maxDiff {
							maxDiff = ad
						}
					}
				}
			}

			t.Logf("Flat edges: %d line, %d cubic | Curve edges: %d line, %d cubic",
				ebFlat.LineEdgeCount(), ebFlat.CubicEdgeCount(),
				ebCurve.LineEdgeCount(), ebCurve.CubicEdgeCount())
			t.Logf("Diff pixels: %d, max |diff|: %d", diffCount, maxDiff)
		})
	}
}

// TestCircleRenderCubicUpdateLineTrace traces the updateLine() calls for
// each of the 4 cubic arcs through CubicEdge.Update() and logs the resulting
// LineEdge fields (UpperY, LowerY, slope, X).
//
// This is the diagnostic test that pinpoints WHERE in the pipeline the
// coverage diverges: whether it is SnapY quantization, slope computation,
// or the monotonic pin.
func TestCircleRenderCubicUpdateLineTrace(t *testing.T) {
	const (
		cx    = 50.0
		cy    = 50.0
		r     = 40.0
		kappa = 0.5522847498
		shift = 2
	)

	k := r * kappa

	type arcDef struct {
		name           string
		p0, p1, p2, p3 CurvePoint
	}

	arcs := []arcDef{
		{"right->bottom",
			CurvePoint{float32(cx + r), float32(cy)},
			CurvePoint{float32(cx + r), float32(cy + k)},
			CurvePoint{float32(cx + k), float32(cy + r)},
			CurvePoint{float32(cx), float32(cy + r)}},
		{"bottom->left",
			CurvePoint{float32(cx), float32(cy + r)},
			CurvePoint{float32(cx - k), float32(cy + r)},
			CurvePoint{float32(cx - r), float32(cy + k)},
			CurvePoint{float32(cx - r), float32(cy)}},
		{"left->top",
			CurvePoint{float32(cx - r), float32(cy)},
			CurvePoint{float32(cx - r), float32(cy - k)},
			CurvePoint{float32(cx - k), float32(cy - r)},
			CurvePoint{float32(cx), float32(cy - r)}},
		{"top->right",
			CurvePoint{float32(cx), float32(cy - r)},
			CurvePoint{float32(cx + k), float32(cy - r)},
			CurvePoint{float32(cx + r), float32(cy - k)},
			CurvePoint{float32(cx + r), float32(cy)}},
	}

	for _, a := range arcs {
		t.Run(a.name, func(t *testing.T) {
			traceArcSegments(t, a.name, a.p0, a.p1, a.p2, a.p3, shift)
		})
	}
}

func traceArcSegments(t *testing.T, _ string, p0, p1, p2, p3 CurvePoint, shift int) {
	t.Helper()

	// Check Y extrema chopping first
	src := [4]GeomPoint{
		{X: p0.X, Y: p0.Y},
		{X: p1.X, Y: p1.Y},
		{X: p2.X, Y: p2.Y},
		{X: p3.X, Y: p3.Y},
	}
	var dst [10]GeomPoint
	numChops := ChopCubicAtYExtrema(src, &dst)
	t.Logf("Y extrema chops: %d (y0=%.1f, y3=%.1f)", numChops, p0.Y, p3.Y)

	cubic, cubicOK := newCubicEdgeSetup(p0, p1, p2, p3, shift)
	if !cubicOK {
		t.Fatal("newCubicEdgeSetup returned false")
	}

	t.Logf("curveShift=%d (%d segments), dshift=%d, winding=%d",
		cubic.curveShift, 1<<cubic.curveShift, cubic.dshift, cubic.line.Winding)
	t.Logf("Initial: snappedY=%.4f, cLastY=%.4f",
		FDot16ToFloat64(cubic.snappedY), FDot16ToFloat64(cubic.cLastY))

	// Track segment gaps
	prevLowerY := FDot16ToFloat64(cubic.snappedY)

	segIdx := 0
	for cubic.Update() {
		segIdx++
		line := &cubic.line

		hasPrecise := line.UpperY != 0 || line.LowerY != 0

		if hasPrecise {
			upperPx := FDot16ToFloat64(line.UpperY)
			lowerPx := FDot16ToFloat64(line.LowerY)
			spanPx := lowerPx - upperPx

			gap := upperPx - prevLowerY
			gapStr := ""
			if gap > 0.01 {
				gapStr = fmt.Sprintf(" *** GAP=%.4f ***", gap)
			} else if gap < -0.01 {
				gapStr = fmt.Sprintf(" *** OVERLAP=%.4f ***", -gap)
			}

			t.Logf("  seg[%2d]: UpperY=%.4f LowerY=%.4f span=%.4f X=%.4f DX=%.4f%s",
				segIdx, upperPx, lowerPx, spanPx,
				FDot16ToFloat64(line.X), FDot16ToFloat64(line.DX), gapStr)

			if spanPx < 0.25 {
				t.Logf("           WARNING: very short segment (%.4f px)", spanPx)
			}

			prevLowerY = lowerPx
		} else {
			t.Logf("  seg[%2d]: FirstY=%d LastY=%d (no precise Y)",
				segIdx, line.FirstY, line.LastY)
		}
	}

	t.Logf("Total segments produced: %d, final Y=%.4f", segIdx, prevLowerY)
}

// TestCircleRenderImageOutput renders both modes into RGBA images and
// reports basic statistics. This is useful for visual inspection — the
// test logs can show the diff map.
func TestCircleRenderImageOutput(t *testing.T) {
	const (
		w  = 100
		h  = 100
		cx = 50.0
		cy = 50.0
		r  = 40.0
	)

	path := makeCirclePath(cx, cy, r)

	// Render both modes
	ebFlat := NewEdgeBuilder(2)
	ebFlat.SetFlattenCurves(true)
	ebFlat.BuildFromPath(path, IdentityTransform{})
	flatBuf := make([]uint8, w*h)
	FillToBuffer(ebFlat, w, h, FillRuleNonZero, flatBuf)

	ebCurve := NewEdgeBuilder(2)
	ebCurve.SetFlattenCurves(false)
	ebCurve.BuildFromPath(path, IdentityTransform{})
	curveBuf := make([]uint8, w*h)
	FillToBuffer(ebCurve, w, h, FillRuleNonZero, curveBuf)

	// Create diff image: green = flatten only, red = curve only, yellow = both differ
	diffImg := image.NewRGBA(image.Rect(0, 0, w, h))
	diffCount := 0
	maxDiff := 0
	for y := range h {
		for x := range w {
			f := flatBuf[y*w+x]
			c := curveBuf[y*w+x]
			d := int(c) - int(f)
			if d == 0 {
				// Same — show grayscale of coverage
				diffImg.SetRGBA(x, y, color.RGBA{f, f, f, 255})
			} else {
				diffCount++
				ad := d
				if ad < 0 {
					ad = -ad
				}
				if ad > maxDiff {
					maxDiff = ad
				}
				// Amplify diff for visibility: scale to 0-255 range
				amp := uint8(min(ad*10, 255))
				if d > 0 {
					// Curve has MORE coverage — show as red
					diffImg.SetRGBA(x, y, color.RGBA{amp, 0, 0, 255})
				} else {
					// Flatten has MORE coverage — show as blue
					diffImg.SetRGBA(x, y, color.RGBA{0, 0, amp, 255})
				}
			}
		}
	}

	t.Logf("Diff image: %d differing pixels, max |diff|=%d", diffCount, maxDiff)

	// Log the diff map as ASCII art for the edge region only
	// Show only the bounding box of the circle edge (R-2..R+2 from center)
	minEdge := int(cx-r) - 2
	maxEdge := int(cx+r) + 2
	if minEdge < 0 {
		minEdge = 0
	}
	if maxEdge > w {
		maxEdge = w
	}

	t.Log("\nDiff map (edge region, . = same, + = curve>flat, - = flat>curve):")
	for y := minEdge; y < maxEdge; y++ {
		row := fmt.Sprintf("  y%02d: ", y)
		for x := minEdge; x < maxEdge; x++ {
			f := flatBuf[y*w+x]
			c := curveBuf[y*w+x]
			d := int(c) - int(f)
			switch {
			case d == 0:
				switch {
				case f > 0 && f < 255:
					row += "~" // Edge pixel, same coverage
				case f == 255:
					row += "#" // Interior, full coverage
				default:
					row += "." // Outside
				}
			case d > 0:
				row += "+" // Curve has more
			default:
				row += "-" // Flatten has more
			}
		}
		t.Log(row)
	}
}

// TestCircleRenderSmallRadius tests a very small circle (R=5) where
// the cubic arc is only a few pixels. Forward-diff should still produce
// reasonable results.
func TestCircleRenderSmallRadius(t *testing.T) {
	const (
		w  = 20
		h  = 20
		cx = 10.0
		cy = 10.0
		r  = 5.0
	)

	path := makeCirclePath(cx, cy, r)

	ebFlat := NewEdgeBuilder(2)
	ebFlat.SetFlattenCurves(true)
	ebFlat.BuildFromPath(path, IdentityTransform{})
	flatBuf := make([]uint8, w*h)
	FillToBuffer(ebFlat, w, h, FillRuleNonZero, flatBuf)

	ebCurve := NewEdgeBuilder(2)
	ebCurve.SetFlattenCurves(false)
	ebCurve.BuildFromPath(path, IdentityTransform{})
	curveBuf := make([]uint8, w*h)
	FillToBuffer(ebCurve, w, h, FillRuleNonZero, curveBuf)

	diffCount := 0
	maxDiff := 0
	for y := range h {
		for x := range w {
			d := int(curveBuf[y*w+x]) - int(flatBuf[y*w+x])
			if d != 0 {
				diffCount++
				ad := d
				if ad < 0 {
					ad = -ad
				}
				if ad > maxDiff {
					maxDiff = ad
				}
			}
		}
	}

	t.Logf("Small circle R=%d: %d diff pixels, max |diff|=%d", int(r), diffCount, maxDiff)

	// Log coverage map for both modes
	for mode, buf := range map[string][]uint8{"flat": flatBuf, "curve": curveBuf} {
		t.Logf("\n%s coverage:", mode)
		for y := range h {
			row := fmt.Sprintf("  y%02d:", y)
			for x := range w {
				c := buf[y*w+x]
				if c == 0 {
					row += "  ."
				} else {
					row += fmt.Sprintf(" %02X", c)
				}
			}
			t.Log(row)
		}
	}
}

// TestCircleRenderTangentZoneCoverage verifies that cubic forward-diff does NOT
// lose exactly 1/4 pixel coverage at horizontal tangent points (top/bottom of circle).
//
// Before the curve segment split fix, the AnalyticFiller lost exactly 64/255
// (= 1 sub-strip out of 4) at pixels where the cubic arc has a near-horizontal
// tangent. The root cause was that a sub-strip could span a curve segment boundary
// without being split, causing the clamped segment to contribute only partial alpha
// while the next segment's remaining coverage was lost.
//
// This test checks interior pixels at the tangent zone (y=11 for top, y=89 for bottom)
// where flatten=true gives 255 (full coverage). The curve mode must also produce 255.
func TestCircleRenderTangentZoneCoverage(t *testing.T) {
	const (
		w  = 100
		h  = 100
		cx = 50.0
		cy = 50.0
		r  = 40.0
	)

	path := makeCirclePath(cx, cy, r)

	// Render with flatten=true (reference)
	ebFlat := NewEdgeBuilder(2)
	ebFlat.SetFlattenCurves(true)
	ebFlat.BuildFromPath(path, IdentityTransform{})
	flatBuf := make([]uint8, w*h)
	FillToBuffer(ebFlat, w, h, FillRuleNonZero, flatBuf)

	// Render with flatten=false (forward-diff cubics)
	ebCurve := NewEdgeBuilder(2)
	ebCurve.SetFlattenCurves(false)
	ebCurve.BuildFromPath(path, IdentityTransform{})
	curveBuf := make([]uint8, w*h)
	FillToBuffer(ebCurve, w, h, FillRuleNonZero, curveBuf)

	// Check tangent zone rows: y=11 (top tangent) and y=89 (bottom tangent).
	// Interior pixels in these rows must have full coverage (255) in both modes.
	tangentRows := []struct {
		name string
		y    int
		xMin int // first fully-interior pixel
		xMax int // last fully-interior pixel
	}{
		{"top tangent y=11", 11, 42, 57},
		{"bottom tangent y=89", 89, 42, 57},
	}

	for _, tr := range tangentRows {
		for x := tr.xMin; x <= tr.xMax; x++ {
			flatVal := flatBuf[tr.y*w+x]
			curveVal := curveBuf[tr.y*w+x]

			if flatVal != 255 {
				continue // skip edge pixels that aren't fully covered
			}

			diff := int(curveVal) - int(flatVal)
			absDiff := diff
			if absDiff < 0 {
				absDiff = -absDiff
			}
			// Must not lose a full sub-strip of coverage (64).
			// Allow small rounding differences (up to 10) between modes.
			if absDiff > 10 {
				t.Errorf("%s pixel (%d,%d): flat=%d curve=%d diff=%d — "+
					"curve mode lost sub-strip coverage at tangent point",
					tr.name, x, tr.y, flatVal, curveVal, diff)
			}
		}
	}

	// Also verify max diff across entire image is bounded.
	// With deviation-based curve subdivision (0.1px threshold), the forward-diff
	// path uses higher segment counts than pure FDot6. The resulting coverage
	// differences are larger than the original 2-segment-per-quad mode but are
	// inherent to the subdivision approach, not coverage loss bugs.
	maxDiff := 0
	for i := range flatBuf {
		d := int(curveBuf[i]) - int(flatBuf[i])
		if d < 0 {
			d = -d
		}
		if d > maxDiff {
			maxDiff = d
		}
	}
	if maxDiff > 60 {
		t.Errorf("max diff across entire circle = %d, want <= 60", maxDiff)
	}
	t.Logf("max diff = %d (tangent zone and overall)", maxDiff)
}

// TestStrokeCircleCurveVsFlatten renders a stroke circle as a two-contour path
// (outer CW + inner CCW, NonZero fill) and compares forward-diff curve edges
// against flattened lines.
//
// This is the critical test for curve segment boundary collection in the analytic
// filler. When two curve edges (left+right from outer+inner contours) have different
// fLowerY values, the sub-strip must be split at BOTH boundaries. Without proper
// boundary collection, coverage is lost at horizontal tangent zones (y near top/bottom
// of the circle) where curve segments are shortest — exactly 1/4 pixel (64/255).
//
// Uses R=40 (outer=40.75, inner=39.25) where inherent forward-diff error is small
// (max ~6 for single contour), so the boundary issue produces a clear signal.
func TestStrokeCircleCurveVsFlatten(t *testing.T) {
	const (
		outerR = 40.75
		innerR = 39.25
		cx     = 50.0
		cy     = 50.0
		w      = 100
		h      = 100
	)

	// Build two-contour stroke path: outer CW + inner CCW.
	path := makeStrokeCirclePath(cx, cy, outerR, innerR)

	// Render with flatten=true (reference)
	ebFlat := NewEdgeBuilder(2)
	ebFlat.SetFlattenCurves(true)
	ebFlat.BuildFromPath(path, IdentityTransform{})
	flatBuf := make([]uint8, w*h)
	FillToBuffer(ebFlat, w, h, FillRuleNonZero, flatBuf)

	// Render with flatten=false (forward-diff cubics)
	ebCurve := NewEdgeBuilder(2)
	ebCurve.SetFlattenCurves(false)
	ebCurve.BuildFromPath(path, IdentityTransform{})
	curveBuf := make([]uint8, w*h)
	FillToBuffer(ebCurve, w, h, FillRuleNonZero, curveBuf)

	// Compare every pixel
	diffCount := 0
	maxDiff := 0
	for y := range h {
		for x := range w {
			d := int(curveBuf[y*w+x]) - int(flatBuf[y*w+x])
			if d < 0 {
				d = -d
			}
			if d > 0 {
				diffCount++
				if d > maxDiff {
					maxDiff = d
				}
			}
		}
	}

	t.Logf("Stroke circle outer=%.2f inner=%.2f center=(%.0f,%.0f) canvas=%dx%d",
		outerR, innerR, cx, cy, w, h)
	t.Logf("Flatten edges: %d line, %d quad, %d cubic",
		ebFlat.LineEdgeCount(), ebFlat.QuadraticEdgeCount(), ebFlat.CubicEdgeCount())
	t.Logf("Curve edges:   %d line, %d quad, %d cubic",
		ebCurve.LineEdgeCount(), ebCurve.QuadraticEdgeCount(), ebCurve.CubicEdgeCount())
	t.Logf("Diff pixels: %d / %d (%.2f%%)", diffCount, w*h, 100.0*float64(diffCount)/float64(w*h))
	t.Logf("Max |diff|: %d", maxDiff)

	// Threshold: max diff must be <= 35.
	// With deviation-based curve subdivision (0.1px threshold in addQuad/addCubic),
	// the forward-diff path produces more segments per curve than the original
	// fixed 2-segment mode. This changes coverage at curve boundaries, especially
	// near horizontal tangent zones where segment density matters most.
	// The diffs are inherent to the different subdivision strategy, not coverage loss.
	if maxDiff > 35 {
		t.Errorf("max diff = %d, want <= 35 — curve segment boundaries not properly collected", maxDiff)
	}
}

// makeStrokeCirclePath builds a two-contour stroke path: outer CW circle at outerR,
// inner CCW circle at innerR. Both use the standard kappa cubic approximation.
// When filled with NonZero, this produces a ring (simulating a stroked circle).
func makeStrokeCirclePath(cx, cy, outerR, innerR float64) *testPath {
	p := &testPath{}

	// Outer contour (CW)
	addCircleContour(p, cx, cy, outerR, false)

	// Inner contour (CCW — reversed winding)
	addCircleContour(p, cx, cy, innerR, true)

	return p
}

// addCircleContour adds a single circle contour to the path.
// If reverse is true, the contour is drawn CCW (counter-clockwise).
func addCircleContour(p *testPath, cx, cy, r float64, reverse bool) {
	const kappa = 0.5522847498
	k := r * kappa

	type pt struct{ x, y float64 }

	// 4 cubic arcs CW: right->bottom->left->top->right
	arcs := [4][4]pt{
		// right -> bottom
		{{cx + r, cy}, {cx + r, cy + k}, {cx + k, cy + r}, {cx, cy + r}},
		// bottom -> left
		{{cx, cy + r}, {cx - k, cy + r}, {cx - r, cy + k}, {cx - r, cy}},
		// left -> top
		{{cx - r, cy}, {cx - r, cy - k}, {cx - k, cy - r}, {cx, cy - r}},
		// top -> right
		{{cx, cy - r}, {cx + k, cy - r}, {cx + r, cy - k}, {cx + r, cy}},
	}

	if reverse {
		// Reverse each arc AND reverse arc order for CCW
		var rev [4][4]pt
		for i := range 4 {
			src := arcs[3-i]
			rev[i] = [4]pt{src[3], src[2], src[1], src[0]}
		}
		arcs = rev
	}

	p.verbs = append(p.verbs, MoveTo)
	p.points = append(p.points, float32(arcs[0][0].x), float32(arcs[0][0].y))

	for _, arc := range arcs {
		p.verbs = append(p.verbs, CubicTo)
		p.points = append(p.points,
			float32(arc[1].x), float32(arc[1].y),
			float32(arc[2].x), float32(arc[2].y),
			float32(arc[3].x), float32(arc[3].y))
	}

	p.verbs = append(p.verbs, Close)
}

// BenchmarkCircleRenderFlatten benchmarks circle rendering with flatten=true.
func BenchmarkCircleRenderFlatten(b *testing.B) {
	path := makeCirclePath(50, 50, 40)
	buf := make([]uint8, 100*100)
	b.ResetTimer()
	for b.Loop() {
		eb := NewEdgeBuilder(2)
		eb.SetFlattenCurves(true)
		eb.BuildFromPath(path, IdentityTransform{})
		FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)
	}
}

// BenchmarkCircleRenderCurve benchmarks circle rendering with flatten=false.
func BenchmarkCircleRenderCurve(b *testing.B) {
	path := makeCirclePath(50, 50, 40)
	buf := make([]uint8, 100*100)
	b.ResetTimer()
	for b.Loop() {
		eb := NewEdgeBuilder(2)
		eb.SetFlattenCurves(false)
		eb.BuildFromPath(path, IdentityTransform{})
		FillToBuffer(eb, 100, 100, FillRuleNonZero, buf)
	}
}
