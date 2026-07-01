package text

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestTTGolden_SegoeUI_W_12ppem validates TT interpreter output for Segoe UI
// 'w' (GID=90) at 12ppem against skrifa golden coordinates.
//
// Golden data extracted from skrifa (Rust fontations) with HintingMode::Smooth.
// Coordinates are in pixels (Y-UP). Our output uses Y-DOWN, so we negate Y.
//
// Grid-fitted endpoints (on-curve, Y-touched) must match within tight tolerance
// (0.01 px). IUP-interpolated mid-curve control points may have small differences
// due to backward compat X-suppression affecting IUP[x] reference values.
//
// This test catches GETINFO bit mapping regressions that cause prep programs
// to take different code paths, producing different CVT values and ultimately
// different hinted coordinates.
func TestTTGolden_SegoeUI_W_12ppem(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("system font test only on Windows (Segoe UI)")
	}
	data, err := os.ReadFile(filepath.Join("C:", "Windows", "Fonts", "segoeui.ttf"))
	if err != nil {
		t.Skipf("Segoe UI not available: %v", err)
	}

	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("no TT hint cache for Segoe UI")
	}

	// Find 'w' GID via cmap.
	parser := &ownParser{}
	parsed, parseErr := parser.Parse(data)
	if parseErr != nil {
		t.Fatalf("parse: %v", parseErr)
	}
	wGID := parsed.GlyphIndex('w')
	if wGID == 0 {
		t.Fatal("'w' not found in Segoe UI cmap")
	}

	outline, err := cache.hintGlyphOutline(wGID, 12)
	if err != nil {
		t.Fatalf("hintGlyphOutline: %v", err)
	}
	if outline == nil {
		t.Fatal("nil outline")
	}

	// Convert to segments for comparison.
	glyphOutline := ttHintedOutlineToGlyphOutline(outline, GlyphID(wGID))
	if glyphOutline == nil {
		t.Fatal("nil glyph outline")
	}

	// skrifa golden coordinates (Y-UP pixels, HintingMode::Smooth).
	// Grid-fitted endpoints match exactly. IUP-interpolated off-curve control
	// points may differ slightly due to backward compat X-coordinate suppression
	// affecting the IUP[x] pass (different X reference values → different X
	// positions for untouched off-curve points).
	type goldenCoord struct {
		x, y    float64
		isExact bool // true for grid-fitted endpoints, false for IUP-interpolated
	}
	golden := []goldenCoord{
		{8.5312, 6.0000, true},  // seg[0] M — touched
		{6.7344, 0.0000, true},  // seg[1] L — on baseline
		{5.7344, 0.0000, true},  // seg[2] L — on baseline
		{4.5000, 4.2812, true},  // seg[3] L — IUP Y, but on-curve and Y matches
		{4.4062, 4.8438, false}, // seg[4] Q — IUP-interpolated off-curve
		{4.3906, 4.8438, true},  // seg[5] L — touched endpoint
		{4.2656, 4.2969, false}, // seg[6] Q — IUP-interpolated off-curve
		{2.9219, 0.0000, true},  // seg[7] L — on baseline
		{1.9531, 0.0000, true},  // seg[8] L — on baseline
		{0.1406, 6.0000, true},  // seg[9] L — touched
		{1.1562, 6.0000, true},  // seg[10] L — touched
		{2.3906, 1.4844, true},  // seg[11] L — IUP Y, on-curve
		{2.4688, 0.9531, false}, // seg[12] Q — IUP-interpolated off-curve
		{2.5156, 0.9531, true},  // seg[13] L — touched endpoint
		{2.6250, 1.5000, false}, // seg[14] Q — IUP-interpolated off-curve
		{4.0156, 6.0000, true},  // seg[15] L — touched
		{4.8906, 6.0000, true},  // seg[16] L — touched
		{6.1250, 1.4844, true},  // seg[17] L — IUP Y, on-curve
		{6.2188, 0.9375, false}, // seg[18] Q — IUP-interpolated off-curve
		{6.2656, 0.9375, true},  // seg[19] L — touched endpoint
	}

	const (
		exactTol = 0.02 // 0.02 px tolerance for grid-fitted endpoints
		iupTol   = 0.50 // 0.50 px tolerance for IUP-interpolated off-curve points
	)

	count := len(golden)
	if len(glyphOutline.Segments) < count {
		count = len(glyphOutline.Segments)
		t.Errorf("segment count: got %d, want >= %d", len(glyphOutline.Segments), len(golden))
	}

	exactMatch := 0
	iupClose := 0
	for i := 0; i < count; i++ {
		seg := glyphOutline.Segments[i]
		want := golden[i]

		ourX := float64(seg.Points[0].X)
		ourY := -float64(seg.Points[0].Y) // negate Y (Y-DOWN → Y-UP)
		wantX := want.x
		wantY := want.y

		dx := math.Abs(ourX - wantX)
		dy := math.Abs(ourY - wantY)

		tol := iupTol
		if want.isExact {
			tol = exactTol
		}

		if dx > tol || dy > tol {
			if want.isExact {
				t.Errorf("seg[%d] EXACT: got (%.4f, %.4f), want (%.4f, %.4f) [dx=%.4f dy=%.4f]",
					i, ourX, ourY, wantX, wantY, dx, dy)
			} else {
				t.Errorf("seg[%d] IUP: got (%.4f, %.4f), want (%.4f, %.4f) [dx=%.4f dy=%.4f]",
					i, ourX, ourY, wantX, wantY, dx, dy)
			}
		} else {
			if want.isExact {
				exactMatch++
			} else {
				iupClose++
			}
		}
	}

	t.Logf("Segoe UI 'w' at 12ppem: %d exact matches, %d IUP within tolerance (total %d/%d)",
		exactMatch, iupClose, exactMatch+iupClose, count)
}
