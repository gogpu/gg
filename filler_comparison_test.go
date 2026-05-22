//go:build !nogpu

package gg_test

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/raster" // registers AdaptiveFiller (SparseStrips + TileCompute)
)

// TestStrokeSineWave_AnalyticVsSparseStrips measures the rendering difference
// between AnalyticFiller and SparseStripsFiller for a stroke-expanded 100-segment
// sine wave polyline. This is the exact reproducer for issue #347.
//
// The test strokes the sine wave path with both fillers by toggling the global
// CoverageFiller registration, and compares pixmaps pixel-by-pixel. It reports:
//   - Total non-white pixels (stroke footprint)
//   - Maximum Y spread at each X column (visual stroke width)
//   - Coverage distribution histograms
//   - Exact pixel difference count and locations
func TestStrokeSineWave_AnalyticVsSparseStrips(t *testing.T) {
	const (
		canvasW     = 800
		canvasH     = 500
		lineWidth   = 2.0
		numSegments = 100
	)

	// Build the damped sine wave path (100 segments = 101 points).
	sinePath := buildTestSinePath(numSegments)
	t.Logf("Source sine wave path: %d verbs (MoveTo + %d LineTo)", sinePath.NumVerbs(), numSegments)

	// --- Threshold analysis ---
	// Stroke expansion produces ~4x verbs (outline + cap/join geometry).
	// The adaptive threshold formula is: threshold = clamp(2048/sqrt(bboxArea), 32, 256).
	// For a 2px stroke on this sine wave, the expanded path has ~397 verbs
	// spanning roughly 700x400 pixels = 280,000 px^2 area.
	// threshold = 2048 / sqrt(280000) ~ 3.87 -> clamped to 32.
	// Since 397 >> 32, shouldUseTileRasterizer returns true when a CoverageFiller
	// is registered, routing to SparseStrips instead of AnalyticFiller.
	t.Logf("")
	t.Logf("=== ROUTING ANALYSIS ===")
	t.Logf("When CoverageFiller is registered (import _ gg/gpu or gg/raster),")
	t.Logf("Fill() routes complex stroke-expanded paths to the tile rasterizer.")
	t.Logf("The adaptive threshold is ~32 for large bboxes; stroke expansion of")
	t.Logf("100-segment polyline produces ~397 verbs >> 32, triggering SparseStrips.")

	// --- Render 1: AnalyticFiller (no CoverageFiller registered) ---
	pmAnalytic := gg.NewPixmap(canvasW, canvasH)
	pmAnalytic.Clear(gg.White)

	// Save the filler registered by the raster package init(), then clear it.
	savedFiller := gg.GetCoverageFiller()
	gg.RegisterCoverageFiller(nil)

	rAnalytic := gg.NewSoftwareRenderer(canvasW, canvasH)
	paintA := makeTestStrokePaint(lineWidth)
	if err := rAnalytic.Stroke(pmAnalytic, sinePath, paintA); err != nil {
		t.Fatalf("AnalyticFiller Stroke failed: %v", err)
	}

	// --- Render 2: SparseStripsFiller (AdaptiveFiller registered by raster/) ---
	pmSparse := gg.NewPixmap(canvasW, canvasH)
	pmSparse.Clear(gg.White)

	// Restore the AdaptiveFiller (which uses SparseStrips for this path size).
	gg.RegisterCoverageFiller(savedFiller)

	rSparse := gg.NewSoftwareRenderer(canvasW, canvasH)
	paintS := makeTestStrokePaint(lineWidth)
	if err := rSparse.Stroke(pmSparse, sinePath, paintS); err != nil {
		t.Fatalf("SparseStripsFiller Stroke failed: %v", err)
	}

	// --- Measure and compare ---
	analyticStats := measurePixmap(pmAnalytic, canvasW, canvasH)
	sparseStats := measurePixmap(pmSparse, canvasW, canvasH)

	t.Logf("")
	t.Logf("=== ANALYTIC FILLER ===")
	logMeasurements(t, analyticStats)

	t.Logf("")
	t.Logf("=== SPARSE STRIPS FILLER (via AdaptiveFiller) ===")
	logMeasurements(t, sparseStats)

	t.Logf("")
	t.Logf("=== COMPARISON ===")
	t.Logf("Non-white pixels: Analytic=%d, SparseStrips=%d, delta=%d (%.1f%%)",
		analyticStats.nonWhiteCount, sparseStats.nonWhiteCount,
		sparseStats.nonWhiteCount-analyticStats.nonWhiteCount,
		pctDelta(analyticStats.nonWhiteCount, sparseStats.nonWhiteCount))

	t.Logf("Full-black pixels (coverage=255): Analytic=%d, SparseStrips=%d",
		analyticStats.fullBlackCount, sparseStats.fullBlackCount)

	t.Logf("Mean coverage (non-white): Analytic=%.2f, SparseStrips=%.2f",
		analyticStats.meanCov, sparseStats.meanCov)

	// Column-wise Y-spread analysis (visual stroke width).
	analyticSpread := measureColumnSpread(pmAnalytic, canvasW, canvasH)
	sparseSpread := measureColumnSpread(pmSparse, canvasW, canvasH)

	var maxASpread, maxSSpread int
	var sumASpread, sumSSpread int
	var activeColumns int
	for x := 0; x < canvasW; x++ {
		if analyticSpread[x] > 0 || sparseSpread[x] > 0 {
			activeColumns++
			sumASpread += analyticSpread[x]
			sumSSpread += sparseSpread[x]
		}
		if analyticSpread[x] > maxASpread {
			maxASpread = analyticSpread[x]
		}
		if sparseSpread[x] > maxSSpread {
			maxSSpread = sparseSpread[x]
		}
	}

	avgA, avgS := 0.0, 0.0
	if activeColumns > 0 {
		avgA = float64(sumASpread) / float64(activeColumns)
		avgS = float64(sumSSpread) / float64(activeColumns)
	}

	t.Logf("")
	t.Logf("=== COLUMN Y-SPREAD (visual stroke width in pixels) ===")
	t.Logf("Max Y-spread:  Analytic=%d, SparseStrips=%d", maxASpread, maxSSpread)
	t.Logf("Avg Y-spread:  Analytic=%.2f, SparseStrips=%.2f", avgA, avgS)
	t.Logf("Active columns: %d", activeColumns)

	// Coverage histogram comparison.
	t.Logf("")
	t.Logf("=== COVERAGE HISTOGRAM (non-white pixels, 8 bins) ===")
	t.Logf("Range           | Analytic  | SparseStrips | Delta")
	t.Logf("----------------|-----------|--------------|------")
	for i := 0; i < 8; i++ {
		lo := i * 32
		hi := lo + 31
		if i == 7 {
			hi = 255
		}
		t.Logf("[%3d-%3d]        | %9d | %12d | %+d",
			lo, hi,
			analyticStats.histBins[i], sparseStats.histBins[i],
			sparseStats.histBins[i]-analyticStats.histBins[i])
	}

	// Per-pixel difference map.
	diffCount, maxDiff, diffBuckets := comparePixmaps(pmAnalytic, pmSparse, canvasW, canvasH)
	t.Logf("")
	t.Logf("=== PIXEL DIFFERENCES ===")
	unionNonWhite := analyticStats.nonWhiteCount
	if sparseStats.nonWhiteCount > unionNonWhite {
		unionNonWhite = sparseStats.nonWhiteCount
	}
	t.Logf("Pixels that differ: %d out of %d non-white (union)", diffCount, unionNonWhite)
	t.Logf("Max channel difference: %d", maxDiff)
	t.Logf("Diff magnitude distribution:")
	t.Logf("  1-10:    %d pixels", diffBuckets[0])
	t.Logf("  11-50:   %d pixels", diffBuckets[1])
	t.Logf("  51-100:  %d pixels", diffBuckets[2])
	t.Logf("  101-200: %d pixels", diffBuckets[3])
	t.Logf("  201-255: %d pixels", diffBuckets[4])

	// Print sample columns where spread differs most.
	t.Logf("")
	t.Logf("=== SAMPLE COLUMNS WITH LARGEST SPREAD DIFFERENCE ===")
	type spreadDiff struct {
		x    int
		a, s int
	}
	var spreadDiffs []spreadDiff
	for x := 0; x < canvasW; x++ {
		d := sparseSpread[x] - analyticSpread[x]
		if d < 0 {
			d = -d
		}
		if d > 0 {
			spreadDiffs = append(spreadDiffs, spreadDiff{x, analyticSpread[x], sparseSpread[x]})
		}
	}
	shown := 0
	for _, sd := range spreadDiffs {
		if shown >= 20 {
			break
		}
		t.Logf("  x=%d: Analytic spread=%d, SparseStrips spread=%d (delta=%+d)",
			sd.x, sd.a, sd.s, sd.s-sd.a)
		shown++
	}
	remaining := len(spreadDiffs) - shown
	if remaining > 0 {
		t.Logf("  ... and %d more columns with spread differences", remaining)
	}
}

// buildTestSinePath creates the exact 100-segment damped sine wave from issue #347.
func buildTestSinePath(numSegments int) *gg.Path {
	p := gg.NewPath()
	for i := 0; i <= numSegments; i++ {
		tt := float64(i) * 0.1
		x := 50 + tt*70
		y := 250 - math.Sin(tt)*math.Exp(-tt*0.1)*200
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return p
}

// makeTestStrokePaint creates a paint configured for a black solid stroke.
func makeTestStrokePaint(width float64) *gg.Paint {
	p := gg.NewPaint()
	p.SetBrush(gg.SolidBrush{Color: gg.Black})
	p.Stroke = &gg.Stroke{
		Width:      width,
		Cap:        gg.LineCapButt,
		Join:       gg.LineJoinMiter,
		MiterLimit: 4.0,
	}
	p.TransformScale = 1.0
	return p
}

// measurements holds statistical measurements of a rendered pixmap.
type measurements struct {
	nonWhiteCount  int
	fullBlackCount int
	meanCov        float64
	histBins       [8]int // 8 bins of 32 levels each
}

// logMeasurements prints measurements to the test log.
func logMeasurements(t *testing.T, m measurements) {
	t.Helper()
	t.Logf("Non-white pixels:   %d", m.nonWhiteCount)
	t.Logf("Full-black (cov=255): %d", m.fullBlackCount)
	t.Logf("Mean coverage:      %.2f", m.meanCov)
}

// measurePixmap computes rendering statistics from a white-background pixmap
// with a black stroke. Uses luminance as a proxy for coverage since
// black-on-white blending produces R=G=B after source-over compositing.
func measurePixmap(pm *gg.Pixmap, w, h int) measurements {
	var m measurements
	var covSum int64
	data := pm.Data()

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			r, g, b, a := data[i], data[i+1], data[i+2], data[i+3]
			// White premultiplied = (255, 255, 255, 255).
			if r == 255 && g == 255 && b == 255 && a == 255 {
				continue
			}
			m.nonWhiteCount++

			// coverage ~ 255 - luminance (black=full coverage, white=zero).
			cov := 255 - int(r)
			if cov < 0 {
				cov = 0
			}

			covSum += int64(cov)
			if cov == 255 {
				m.fullBlackCount++
			}

			bin := cov / 32
			if bin > 7 {
				bin = 7
			}
			m.histBins[bin]++
		}
	}

	if m.nonWhiteCount > 0 {
		m.meanCov = float64(covSum) / float64(m.nonWhiteCount)
	}
	return m
}

// measureColumnSpread returns per-column Y extent (max-min+1) of non-white pixels.
func measureColumnSpread(pm *gg.Pixmap, w, h int) []int {
	spread := make([]int, w)
	data := pm.Data()

	for x := 0; x < w; x++ {
		minY, maxY := h, -1
		for y := 0; y < h; y++ {
			i := (y*w + x) * 4
			r, g, b, a := data[i], data[i+1], data[i+2], data[i+3]
			if r == 255 && g == 255 && b == 255 && a == 255 {
				continue
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
		if maxY >= minY {
			spread[x] = maxY - minY + 1
		}
	}
	return spread
}

// comparePixmaps compares two pixmaps and returns difference statistics.
// Returns total differing pixels, maximum per-channel difference, and
// a 5-bucket distribution of difference magnitudes.
func comparePixmaps(a, b *gg.Pixmap, w, h int) (diffCount, maxDiff int, buckets [5]int) {
	da, db := a.Data(), b.Data()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			// Compare the red channel (for black on white, R=G=B).
			ra, rb := int(da[i]), int(db[i])
			d := ra - rb
			if d < 0 {
				d = -d
			}
			if d == 0 {
				continue
			}
			diffCount++
			if d > maxDiff {
				maxDiff = d
			}
			switch {
			case d <= 10:
				buckets[0]++
			case d <= 50:
				buckets[1]++
			case d <= 100:
				buckets[2]++
			case d <= 200:
				buckets[3]++
			default:
				buckets[4]++
			}
		}
	}
	return diffCount, maxDiff, buckets
}

func pctDelta(a, b int) float64 {
	if a == 0 {
		if b == 0 {
			return 0
		}
		return 100.0
	}
	return float64(b-a) / float64(a) * 100
}
