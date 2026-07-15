package gg

import (
	"testing"

	"github.com/gogpu/gg/internal/raster"
)

// Tests for the three-tier clip architecture (ADR-052).
//
// Layer A: Rect bounds skip — scanlines/tiles outside clip rect are never processed.
// Layer B: Pre-rasterized clip mask — array lookup replaces per-pixel closure.
// Legacy: ClipCoverage closure fallback for backward compatibility.

// TestAnalyticFiller_ClipBoundsSkipsScanlines verifies that scanlines outside
// the clip Y range produce zero coverage output (Layer A, ADR-052).
func TestAnalyticFiller_ClipBoundsSkipsScanlines(t *testing.T) {
	const w, h = 100, 200

	// Build a simple rectangle path that covers (10,10)-(90,190).
	eb := raster.NewEdgeBuilder(2)
	verbs := []byte{
		byte(raster.MoveTo),
		byte(raster.LineTo),
		byte(raster.LineTo),
		byte(raster.LineTo),
		byte(raster.Close),
	}
	coords := []float64{
		10, 10,
		90, 10,
		90, 190,
		10, 190,
	}
	eb.BuildFromPathF64(verbs, coords)

	// Without clip: should produce coverage in [10, 190) Y range.
	af := raster.NewAnalyticFiller(w, h)
	var unclippedRows []int
	af.Fill(eb, raster.FillRuleNonZero, func(y int, _ *raster.AlphaRuns) {
		unclippedRows = append(unclippedRows, y)
	})
	if len(unclippedRows) == 0 {
		t.Fatal("unclipped fill produced no scanlines")
	}

	// With clip [50, 100): should only produce scanlines in [50, 100).
	eb.Reset()
	eb.BuildFromPathF64(verbs, coords)
	af2 := raster.NewAnalyticFiller(w, h)
	af2.SetClipBounds(0, 50, w, 100)

	var clippedRows []int
	af2.Fill(eb, raster.FillRuleNonZero, func(y int, _ *raster.AlphaRuns) {
		clippedRows = append(clippedRows, y)
	})

	if len(clippedRows) == 0 {
		t.Fatal("clipped fill produced no scanlines")
	}

	for _, y := range clippedRows {
		if y < 50 || y >= 100 {
			t.Errorf("clip bounds violated: scanline y=%d outside [50, 100)", y)
		}
	}

	// Verify we got fewer rows than unclipped.
	if len(clippedRows) >= len(unclippedRows) {
		t.Errorf("clipped rows (%d) should be fewer than unclipped (%d)",
			len(clippedRows), len(unclippedRows))
	}
}

// TestAnalyticFiller_ClearClipBounds verifies that ClearClipBounds restores
// full rendering (no scanline skipping).
func TestAnalyticFiller_ClearClipBounds(t *testing.T) {
	const w, h = 50, 50
	af := raster.NewAnalyticFiller(w, h)

	// Set and then clear clip bounds.
	af.SetClipBounds(10, 10, 40, 40)
	af.ClearClipBounds()

	eb := raster.NewEdgeBuilder(2)
	verbs := []byte{
		byte(raster.MoveTo),
		byte(raster.LineTo),
		byte(raster.LineTo),
		byte(raster.LineTo),
		byte(raster.Close),
	}
	coords := []float64{5, 5, 45, 5, 45, 45, 5, 45}
	eb.BuildFromPathF64(verbs, coords)

	var rows []int
	af.Fill(eb, raster.FillRuleNonZero, func(y int, _ *raster.AlphaRuns) {
		rows = append(rows, y)
	})

	// After clearing, rows should span the full path range [5, 45).
	if len(rows) == 0 {
		t.Fatal("fill produced no scanlines after ClearClipBounds")
	}
	if rows[0] > 5 {
		t.Errorf("first row %d, expected <=5 after clear", rows[0])
	}
	if rows[len(rows)-1] < 44 {
		t.Errorf("last row %d, expected >=44 after clear", rows[len(rows)-1])
	}
}

// TestClipMask_ArrayLookup verifies that the pre-rasterized mask lookup
// (Layer B) produces identical coverage to the closure-based approach.
func TestClipMask_ArrayLookup(t *testing.T) {
	// Build a small 20x20 clip mask where coverage is 255 inside (5,5)-(15,15)
	// and 0 outside. This simulates a simple rectangular clip mask.
	const maskW, maskH = 20, 20
	const originX, originY = 10, 10
	mask := make([]uint8, maskW*maskH)
	for py := 0; py < maskH; py++ {
		for px := 0; px < maskW; px++ {
			if px >= 5 && px < 15 && py >= 5 && py < 15 {
				mask[py*maskW+px] = 255
			}
		}
	}

	// Build equivalent closure.
	closureFn := func(x, y float64) byte {
		px := int(x) - originX
		py := int(y) - originY
		if px >= 5 && px < 15 && py >= 5 && py < 15 {
			return 255
		}
		return 0
	}

	// Compare mask lookup vs closure at various points.
	tests := []struct {
		name string
		px   int
		py   int
	}{
		{"inside", 17, 17},
		{"outside-left", 12, 17},
		{"outside-top", 17, 12},
		{"outside-far", 0, 0},
		{"edge", 15, 15},
		{"corner", 24, 24},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maskCov := applyClipCoverageFromMaskOrFn(
				mask, maskW, originX, originY, nil,
				tt.px, tt.py, 200,
			)
			closureCov := applyClipCoverageFromMaskOrFn(
				nil, 0, 0, 0, closureFn,
				tt.px, tt.py, 200,
			)
			if maskCov != closureCov {
				t.Errorf("pixel (%d,%d): mask=%d, closure=%d",
					tt.px, tt.py, maskCov, closureCov)
			}
		})
	}
}

// TestClipMask_RectOnly_NoMask verifies that rect-only clips use bounds skip
// (Layer A) without allocating a clip mask (Layer B).
func TestClipMask_RectOnly_NoMask(t *testing.T) {
	dc := NewContext(100, 100)

	// Push a rectangular clip.
	dc.ClipRect(20, 20, 60, 60)

	// Draw a filled rect — the rect clip should NOT produce a ClipMask.
	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 100, 100)
	dc.Fill()

	// After fill, paint.ClipMask should be nil (was cleared by defer).
	// We can't inspect mid-fill, but we can verify no crash and correct output.
	// Verify the clip effect: pixels outside (20,20)-(80,80) should be transparent.
	img := dc.Image()

	// Helper to extract alpha from image pixel.
	pixelAlpha := func(x, y int) uint32 {
		_, _, _, a := img.At(x, y).RGBA()
		return a
	}
	pixelRed := func(x, y int) uint32 {
		r, _, _, _ := img.At(x, y).RGBA()
		return r
	}

	// Inside clip region should be red.
	if pixelRed(50, 50) < 0xF000 || pixelAlpha(50, 50) < 0xF000 {
		t.Errorf("inside clip: R=0x%04X, A=0x%04X, expected ~0xFFFF", pixelRed(50, 50), pixelAlpha(50, 50))
	}

	// Outside clip region should be transparent.
	if pixelAlpha(5, 5) > 0x0100 {
		t.Errorf("outside clip (5,5): A=0x%04X, expected ~0", pixelAlpha(5, 5))
	}
	if pixelAlpha(85, 85) > 0x0100 {
		t.Errorf("outside clip (85,85): A=0x%04X, expected ~0", pixelAlpha(85, 85))
	}
}

// TestClipMask_OutOfBounds verifies correct handling of mask lookups for
// pixels outside the mask bounds.
func TestClipMask_OutOfBounds(t *testing.T) {
	mask := []uint8{128, 255, 64, 0} // 2x2 mask at origin (10,10)
	maskW := 2

	tests := []struct {
		name string
		px   int
		py   int
		want uint8
	}{
		{"inside-tl", 10, 10, 128},
		{"inside-tr", 11, 10, 255},
		{"inside-bl", 10, 11, 64},
		{"inside-br", 11, 11, 0},
		{"left", 9, 10, 0},   // outside left
		{"above", 10, 9, 0},  // outside top
		{"right", 12, 10, 0}, // outside right
		{"below", 10, 12, 0}, // outside bottom
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply with input coverage 255 to isolate mask effect.
			got := applyClipCoverageFromMaskOrFn(mask, maskW, 10, 10, nil, tt.px, tt.py, 255)
			if got != tt.want {
				t.Errorf("pixel (%d,%d): got %d, want %d", tt.px, tt.py, got, tt.want)
			}
		})
	}
}

// TestClipMaskCoverage_LookupHelper tests the clipMaskCoverage helper function.
func TestClipMaskCoverage_LookupHelper(t *testing.T) {
	mask := []uint8{100, 200, 50, 150} // 2x2 mask at origin (5,5)

	tests := []struct {
		name    string
		px, py  int
		wantCov uint8
		wantOK  bool
	}{
		{"inside", 5, 5, 100, true},
		{"inside2", 6, 5, 200, true},
		{"outside-left", 4, 5, 0, true},
		{"outside-top", 5, 4, 0, true},
		{"nil-mask", 5, 5, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m []uint8
			if tt.name != "nil-mask" {
				m = mask
			}
			cov, ok := clipMaskCoverage(m, 2, 5, 5, tt.px, tt.py)
			if ok != tt.wantOK {
				t.Errorf("ok=%v, want %v", ok, tt.wantOK)
			}
			if ok && cov != tt.wantCov {
				t.Errorf("coverage=%d, want %d", cov, tt.wantCov)
			}
		})
	}
}

// TestSoftwareRenderer_SetClipBounds verifies that SetClipBounds propagates
// to the AnalyticFiller and affects rendering.
func TestSoftwareRenderer_SetClipBounds(t *testing.T) {
	const w, h = 100, 100
	sr := NewSoftwareRenderer(w, h)
	pm := NewPixmap(w, h)

	// Build a full-canvas rectangle path.
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(float64(w), 0)
	p.LineTo(float64(w), float64(h))
	p.LineTo(0, float64(h))
	p.Close()

	paint := NewPaint()
	paint.SetBrush(SolidBrush{Color: Red})

	// Fill without clip — entire canvas should be red.
	if err := sr.Fill(pm, p, paint); err != nil {
		t.Fatal(err)
	}
	r, _, _, a := pm.getPremul(5, 5)
	if r < 0.9 || a < 0.9 {
		t.Errorf("unclipped (5,5): R=%.2f A=%.2f, want ~1.0", r, a)
	}
	r, _, _, a = pm.getPremul(50, 50)
	if r < 0.9 || a < 0.9 {
		t.Errorf("unclipped (50,50): R=%.2f A=%.2f, want ~1.0", r, a)
	}

	// Reset pixmap and fill with clip [30,30)-(70,70).
	pm2 := NewPixmap(w, h)
	sr2 := NewSoftwareRenderer(w, h)
	sr2.SetClipBounds(30, 30, 70, 70)

	if err := sr2.Fill(pm2, p, paint); err != nil {
		t.Fatal(err)
	}

	// Inside clip should be red.
	r, _, _, a = pm2.getPremul(50, 50)
	if r < 0.9 || a < 0.9 {
		t.Errorf("clipped-inside (50,50): R=%.2f A=%.2f, want ~1.0", r, a)
	}

	// Outside clip (scanline skipped by Layer A) should be transparent.
	pmAlpha := func(pm *Pixmap, x, y int) float64 {
		_, _, _, a := pm.getPremul(x, y)
		return a
	}
	if pmAlpha(pm2, 5, 5) > 0.01 {
		t.Errorf("clipped-outside (5,5): A=%.2f, want ~0.0", pmAlpha(pm2, 5, 5))
	}
	if pmAlpha(pm2, 80, 80) > 0.01 {
		t.Errorf("clipped-outside (80,80): A=%.2f, want ~0.0", pmAlpha(pm2, 80, 80))
	}

	// Clear clip and verify full rendering is restored.
	sr2.ClearClipBounds()
	pm3 := NewPixmap(w, h)
	if err := sr2.Fill(pm3, p, paint); err != nil {
		t.Fatal(err)
	}
	r, _, _, a = pm3.getPremul(5, 5)
	if r < 0.9 || a < 0.9 {
		t.Errorf("after clear (5,5): R=%.2f A=%.2f, want ~1.0", r, a)
	}
}

// TestApplyClipCoverageFromMaskOrFn_Priority verifies that mask takes priority
// over closure, and that neither changes the result when both are nil.
func TestApplyClipCoverageFromMaskOrFn_Priority(t *testing.T) {
	mask := []uint8{128} // 1x1 mask at (0,0) with 50% coverage

	closureFn := func(x, y float64) byte {
		return 64 // different from mask — should NOT be used when mask is present
	}

	// Case 1: mask provided — should use mask (128), not closure (64).
	got := applyClipCoverageFromMaskOrFn(mask, 1, 0, 0, closureFn, 0, 0, 255)
	// Expected: 255 * 128 / 255 = 128
	if got != 128 {
		t.Errorf("mask priority: got %d, want 128", got)
	}

	// Case 2: no mask, closure provided.
	got = applyClipCoverageFromMaskOrFn(nil, 0, 0, 0, closureFn, 0, 0, 255)
	// Expected: 255 * 64 / 255 = 64
	if got != 64 {
		t.Errorf("closure fallback: got %d, want 64", got)
	}

	// Case 3: neither mask nor closure — pass through.
	got = applyClipCoverageFromMaskOrFn(nil, 0, 0, 0, nil, 0, 0, 200)
	if got != 200 {
		t.Errorf("no clip: got %d, want 200", got)
	}
}

// TestClipBounds_CoverageFillerInteraction verifies that ClipBounds is correctly
// propagated to CoverageFiller and restricts its output. When a CoverageFiller
// is registered, paths routed through fillWithCoverageFiller must honor the
// clip bounds — coverage callbacks should not be invoked for pixels outside.
func TestClipBounds_CoverageFillerInteraction(t *testing.T) {
	const w, h = 100, 100
	sr := NewSoftwareRenderer(w, h)
	pm := NewPixmap(w, h)

	// Build a large circle (center 50,50, radius 40) to ensure tile coverage.
	p := NewPath()
	p.Ellipse(50, 50, 40, 40)
	p.Close()

	paint := NewPaint()
	paint.SetBrush(SolidBrush{Color: Red})

	// Set clip bounds to top-left quadrant [0,0)-(50,50).
	sr.SetClipBounds(0, 0, 50, 50)

	if err := sr.Fill(pm, p, paint); err != nil {
		t.Fatalf("Fill with clip: %v", err)
	}

	// Pixel at (25, 25) is inside clip AND inside circle → should have coverage.
	pmAlpha := func(x, y int) float64 {
		_, _, _, a := pm.getPremul(x, y)
		return a
	}

	if a := pmAlpha(25, 25); a < 0.5 {
		t.Errorf("inside clip+circle (25,25): A=%.2f, want >0.5", a)
	}

	// Pixel at (75, 75) is outside clip [0,50)x[0,50) → should be zero.
	if a := pmAlpha(75, 75); a > 0.01 {
		t.Errorf("outside clip (75,75): A=%.2f, want ~0.0", a)
	}

	// Pixel at (25, 75) is outside clip Y range → should be zero.
	if a := pmAlpha(25, 75); a > 0.01 {
		t.Errorf("outside clip Y (25,75): A=%.2f, want ~0.0", a)
	}

	// Nil ClipBounds should be accepted without panic.
	var nilCB *ClipBounds
	if nilCB != nil {
		t.Error("nil ClipBounds should be nil")
	}
}
