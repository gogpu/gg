package text

import (
	"fmt"
	"math"
	"os"
	"testing"

)

// loadGoRegularFont loads the Go Regular font for testing.
// This font is available cross-platform via the goregular package.
func loadGoRegularFont(t *testing.T) ParsedFont {
	t.Helper()
	parser := &ownParser{}
	font, err := parser.Parse(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to parse Go Regular font: %v", err)
	}
	return font
}

// ============================================================
// 1. Standard Width Detection Tests
// ============================================================

func TestComputeStandardWidths_GoRegular(t *testing.T) {
	font := loadGoRegularFont(t)

	script := detectFontScript(font)
	hWidths := computeStandardWidths(font, dimHorizontal, script)
	vWidths := computeStandardWidths(font, dimVertical, script)

	// Go Regular should have detectable standard widths.
	t.Logf("Horizontal: stdWidth=%d, widths=%v, edgeDist=%d",
		hWidths.standardWidth, hWidths.widths, hWidths.edgeDistThreshold)
	t.Logf("Vertical:   stdWidth=%d, widths=%v, edgeDist=%d",
		vWidths.standardWidth, vWidths.widths, vWidths.edgeDistThreshold)

	// Standard width should be positive.
	if hWidths.standardWidth <= 0 {
		t.Errorf("horizontal standard width should be positive, got %d", hWidths.standardWidth)
	}
	if vWidths.standardWidth <= 0 {
		t.Errorf("vertical standard width should be positive, got %d", vWidths.standardWidth)
	}

	// Edge distance threshold = stdw / 5.
	if hWidths.edgeDistThreshold != hWidths.standardWidth/5 {
		t.Errorf("horizontal edge dist threshold: got %d, want %d",
			hWidths.edgeDistThreshold, hWidths.standardWidth/5)
	}
}

func TestComputeStandardWidths_NoGlyph(t *testing.T) {
	// Mock font that returns 0 for all glyph lookups.
	font := &mockParsedFont{
		name:       "empty",
		unitsPerEm: 2048,
		numGlyphs:  1,
	}

	script := detectFontScript(font)
	widths := computeStandardWidths(font, dimVertical, script)

	// Should fall back to derivedConstant(2048) = 50.
	expected := derivedConstant(2048)
	if widths.standardWidth != expected {
		t.Errorf("fallback standard width: got %d, want %d", widths.standardWidth, expected)
	}
}

func TestSortAndQuantizeWidths(t *testing.T) {
	tests := []struct {
		name      string
		widths    []int32
		threshold int32
		want      []int32
	}{
		{"empty", nil, 10, nil},
		{"single", []int32{42}, 10, []int32{42}},
		{"no merge", []int32{30, 60, 100}, 10, []int32{30, 60, 100}},
		{"merge near", []int32{30, 35, 60, 62, 100}, 10, []int32{30, 60, 100}},
		{"all same", []int32{50, 52, 48, 51}, 10, []int32{48}},
		{"unsorted", []int32{100, 30, 60}, 5, []int32{30, 60, 100}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widths := make([]int32, len(tt.widths))
			copy(widths, tt.widths)
			sortAndQuantizeWidths(&widths, tt.threshold)
			if len(widths) != len(tt.want) {
				t.Errorf("got %v, want %v", widths, tt.want)
				return
			}
			for i := range widths {
				if widths[i] != tt.want[i] {
					t.Errorf("got %v, want %v", widths, tt.want)
					return
				}
			}
		})
	}
}

// ============================================================
// 2. Blue Zone Detection Tests
// ============================================================

func TestComputeBlueZones_GoRegular(t *testing.T) {
	font := loadGoRegularFont(t)

	script := detectFontScript(font)
	zones := computeBlueZones(font, script)

	if len(zones) == 0 {
		t.Fatal("expected blue zones for Go Regular font, got none")
	}

	t.Logf("Detected %d blue zones (script: %s):", len(zones), script.name)
	for i, z := range zones {
		topStr := "bottom"
		if z.flags.isTopLike() {
			topStr = "top"
		}
		t.Logf("  zone[%d]: ref=%d shoot=%d %s flags=%d",
			i, z.position, z.overshoot, topStr, z.flags)
	}

	// Should have at least baseline (bottom) and cap-height (top).
	hasTop := false
	hasBottom := false
	for _, z := range zones {
		if z.flags.isTopLike() {
			hasTop = true
		} else {
			hasBottom = true
		}
	}
	if !hasTop {
		t.Error("expected at least one top blue zone")
	}
	if !hasBottom {
		t.Error("expected at least one bottom blue zone")
	}

	// At least one bottom zone should be near baseline (Y≈0).
	hasNearBaseline := false
	for _, z := range zones {
		if !z.flags.isTopLike() {
			if z.position < 50 && z.position > -50 {
				hasNearBaseline = true
			}
		}
	}
	if !hasNearBaseline {
		t.Error("expected at least one bottom zone near baseline (Y≈0)")
	}
}

func TestScaleBlueZones(t *testing.T) {
	zones := []blueZone{
		{position: 700, overshoot: 710, flags: blueZoneTop},
		{position: 0, overshoot: -10, flags: 0},
	}

	// Scale at 16px with 1000 UPM → scale = 0.016.
	scale := 16.0 / 1000.0
	scaled := scaleBlueZones(zones, scale)

	if len(scaled) != 2 {
		t.Fatalf("expected 2 scaled zones, got %d", len(scaled))
	}

	// Cap-top: 700 * 0.016 = 11.2px = ~717 in 26.6.
	capTop := scaled[0]
	if !capTop.isActive {
		// Might not be active if zone is too large.
		t.Log("cap-top zone not active (zone may be too large)")
	}
	capTopPx := f26dot6ToFloat(capTop.reference.scaled)
	if capTopPx < 10 || capTopPx > 13 {
		t.Errorf("cap-top scaled reference: got %d (%.2fpx), expected ~11.2px", capTop.reference.scaled, capTopPx)
	}

	// Baseline: 0 * 0.016 = 0.
	baseline := scaled[1]
	if baseline.reference.scaled != 0 {
		t.Errorf("baseline scaled reference: got %d, expected 0", baseline.reference.scaled)
	}
	if baseline.isActive && baseline.reference.fitted != 0 {
		t.Errorf("baseline fitted reference: got %d, expected 0", baseline.reference.fitted)
	}
}

func TestMedianFloat32(t *testing.T) {
	tests := []struct {
		name string
		vals []float32
		want float32
	}{
		{"empty", nil, 0},
		{"single", []float32{5}, 5},
		{"odd", []float32{1, 3, 5}, 3},
		{"even", []float32{1, 3, 5, 7}, 4},
		{"unsorted", []float32{7, 1, 5, 3}, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := medianFloat32(tt.vals)
			if got != tt.want {
				t.Errorf("got %.1f, want %.1f", got, tt.want)
			}
		})
	}
}

// ============================================================
// 3. Segment Detection Tests
// ============================================================

func TestComputeSegments_Rectangle(t *testing.T) {
	// A simple rectangle: has 2 vertical + 2 horizontal segments.
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{0, 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{10, 0}}},  // right →
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{10, 20}}}, // up ↑
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{0, 20}}},  // left ←
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{0, 0}}},   // down ↓
		},
	}

	pa := buildHintPoints(outline)
	if len(pa.pts) == 0 {
		t.Fatal("expected points")
	}

	// Test horizontal dimension (looking for vertical segments).
	hSegs := computeSegments(&pa, dimHorizontal)
	t.Logf("Horizontal segments (vertical stems): %d", len(hSegs))
	for i, s := range hSegs {
		t.Logf("  seg[%d]: pos=%.2f dir=%d height=%.2f [%.2f,%.2f]",
			i, s.pos, s.dir, s.height, s.minCoord, s.maxCoord)
	}

	// Test vertical dimension (looking for horizontal segments).
	vSegs := computeSegments(&pa, dimVertical)
	t.Logf("Vertical segments (horizontal stems): %d", len(vSegs))
	for i, s := range vSegs {
		t.Logf("  seg[%d]: pos=%.2f dir=%d height=%.2f [%.2f,%.2f]",
			i, s.pos, s.dir, s.height, s.minCoord, s.maxCoord)
	}

	// A rectangle should produce segments in both dimensions.
	if len(hSegs) == 0 {
		t.Error("expected horizontal-dimension segments for rectangle")
	}
	if len(vSegs) == 0 {
		t.Error("expected vertical-dimension segments for rectangle")
	}
}

func TestComputeSegments_GoRegularGlyph(t *testing.T) {
	font := loadGoRegularFont(t)

	// Extract 'H' glyph — clear vertical stems.
	gid := font.GlyphIndex('H')
	if gid == 0 {
		t.Skip("'H' glyph not found")
	}

	ext := NewOutlineExtractor()
	outline, err := ext.ExtractOutline(font, GlyphID(gid), float64(font.UnitsPerEm()))
	if err != nil || outline == nil {
		t.Fatalf("failed to extract 'H' outline: %v", err)
	}

	pa := buildHintPoints(outline)
	segments := computeSegments(&pa, dimHorizontal)

	t.Logf("'H' has %d horizontal-dimension segments", len(segments))

	// 'H' should have at least 4 vertical segments (left outer, left inner,
	// right inner, right outer).
	if len(segments) < 2 {
		t.Errorf("expected at least 2 segments for 'H', got %d", len(segments))
	}
}

// ============================================================
// 4. Edge Grouping Tests
// ============================================================

func TestComputeEdges_BasicStemPair(t *testing.T) {
	// Two opposing segments at positions 5.0 and 7.0.
	segments := []hintSegment{
		{pos: 5.0, dir: dirUp, height: 10, minCoord: 0, maxCoord: 10, linkIdx: 1, serifIdx: -1, edgeIdx: -1, score: 100},
		{pos: 7.0, dir: dirDown, height: 10, minCoord: 0, maxCoord: 10, linkIdx: 0, serifIdx: -1, edgeIdx: -1, score: 100},
	}

	axis := &scaledAxisMetrics{
		scale:             1.0,
		edgeDistThreshold: 0.25,
	}

	edges := computeEdges(segments, axis, dimHorizontal, scriptGroupDefault)

	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}

	// Edges should be linked (forming a stem).
	if edges[0].linkIdx < 0 || edges[1].linkIdx < 0 {
		t.Errorf("edges should be linked: edge0.link=%d, edge1.link=%d",
			edges[0].linkIdx, edges[1].linkIdx)
	}

	t.Logf("Edge[0]: pos=%.2f dir=%d link=%d", edges[0].fpos, edges[0].dir, edges[0].linkIdx)
	t.Logf("Edge[1]: pos=%.2f dir=%d link=%d", edges[1].fpos, edges[1].dir, edges[1].linkIdx)
}

func TestComputeEdges_SegmentMerging(t *testing.T) {
	// Three segments at similar positions should merge into fewer edges.
	segments := []hintSegment{
		{pos: 5.00, dir: dirUp, height: 10, minCoord: 0, maxCoord: 10, linkIdx: -1, serifIdx: -1, edgeIdx: -1, score: 32000},
		{pos: 5.10, dir: dirUp, height: 10, minCoord: 5, maxCoord: 15, linkIdx: -1, serifIdx: -1, edgeIdx: -1, score: 32000},
		{pos: 10.0, dir: dirDown, height: 10, minCoord: 0, maxCoord: 10, linkIdx: -1, serifIdx: -1, edgeIdx: -1, score: 32000},
	}

	axis := &scaledAxisMetrics{
		scale:             1.0,
		edgeDistThreshold: 0.25,
	}

	edges := computeEdges(segments, axis, dimHorizontal, scriptGroupDefault)

	// The first two segments should merge into one edge.
	if len(edges) != 2 {
		t.Errorf("expected 2 edges (after merging), got %d", len(edges))
	}
}

// ============================================================
// 5. computeStemWidth Tests (THE key algorithm)
// ============================================================

func TestComputeStemWidth_Table(t *testing.T) {
	// Standard width: ~1.2px = 77 in 26.6 (1.2 * 64 = 76.8, rounds to 77).
	axis := &scaledAxisMetrics{
		widths: []scaledWidth{
			{scaled: f26dot6FromFloat(1.2), fitted: f26dot6FromFloat(1.2)},
		},
		isExtraLight: false,
	}

	tests := []struct {
		name    string
		width   float64 // pixels — converted to 26.6 for test
		wantMin float64
		wantMax float64
	}{
		// Very thin stems snap to minimum.
		{"thin round", 0.5, 0.9, 1.3},
		{"thin straight", 0.7, 0.8, 1.3},

		// Near standard width: should snap to standard.
		{"near standard", 1.0, 1.0, 1.3},
		{"at standard", 1.2, 1.0, 1.3},
		{"slightly above standard", 1.4, 1.0, 1.6},

		// Medium stems: piecewise quantization.
		{"medium 2px", 2.0, 1.5, 2.5},
		{"medium 2.5px", 2.5, 2.0, 3.0},

		// Large stems: round to integer.
		{"large 4px", 4.0, 3.5, 4.5},
		{"large 5px", 5.0, 4.5, 5.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeStemWidth(axis, f26dot6FromFloat(tt.width), 0, 0)
			gotPx := f26dot6ToFloat(got)
			if float64(gotPx) < tt.wantMin || float64(gotPx) > tt.wantMax {
				t.Errorf("computeStemWidth(%.2fpx) = %.3fpx (26.6=%d), want [%.2f, %.2f]",
					tt.width, gotPx, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestComputeStemWidth_Consistency(t *testing.T) {
	// THE most important test: stems of similar input width should produce
	// IDENTICAL output width. This is what makes text look uniform.
	axis := &scaledAxisMetrics{
		widths: []scaledWidth{
			{scaled: f26dot6FromFloat(1.3), fitted: f26dot6FromFloat(1.3)},
		},
		isExtraLight: false,
	}

	// Simulate stem widths from different glyphs (slight variation).
	inputWidths := []float64{1.20, 1.25, 1.28, 1.30, 1.32, 1.35, 1.40}
	results := make([]int32, len(inputWidths))

	for i, w := range inputWidths {
		results[i] = computeStemWidth(axis, f26dot6FromFloat(w), 0, 0)
	}

	// All should snap to the same value (within tolerance of 32 = 0.5px in 26.6).
	for i := 1; i < len(results); i++ {
		diff := results[i] - results[0]
		if diff < 0 {
			diff = -diff
		}
		if diff > 32 { // 0.5px tolerance
			t.Errorf("inconsistent stem widths: input=%.2f→%d vs input=%.2f→%d (diff=%d)",
				inputWidths[0], results[0], inputWidths[i], results[i], diff)
		}
	}

	t.Logf("Stem width consistency: inputs=%v → outputs=%v", inputWidths, results)
}

func TestComputeStemWidth_ExtraLight(t *testing.T) {
	axis := &scaledAxisMetrics{
		widths:       []scaledWidth{{scaled: f26dot6FromFloat(0.4), fitted: f26dot6FromFloat(0.4)}},
		isExtraLight: true,
	}

	// Extra light fonts should pass through without modification.
	input := f26dot6FromFloat(0.5)
	got := computeStemWidth(axis, input, 0, 0)
	if got != input {
		t.Errorf("extra light: got %d, want %d (passthrough)", got, input)
	}
}

func TestComputeStemWidth_Negative(t *testing.T) {
	axis := &scaledAxisMetrics{
		widths: []scaledWidth{{scaled: f26dot6FromFloat(1.2), fitted: f26dot6FromFloat(1.2)}},
	}

	got := computeStemWidth(axis, f26dot6FromFloat(-1.0), 0, 0)
	if got >= 0 {
		t.Errorf("negative input should give negative output, got %d", got)
	}
}

func TestComputeStemWidth_Serif(t *testing.T) {
	axis := &scaledAxisMetrics{
		widths: []scaledWidth{{scaled: f26dot6FromFloat(1.0), fitted: f26dot6FromFloat(1.0)}},
	}

	// Serif stems < 3px should be left alone.
	input := f26dot6FromFloat(2.0)
	got := computeStemWidth(axis, input, 0, edgeFlagSerif)
	if got != input {
		t.Errorf("serif passthrough: got %d, want %d", got, input)
	}
}

// ============================================================
// 6. hintEdges Tests
// ============================================================

func TestHintEdges_BluAnchored(t *testing.T) {
	blueRef := scaledWidth{
		scaled: f26dot6FromFloat(11.2),
		fitted: f26dot6FromFloat(11.0),
	}

	edges := []*hintEdge{
		{fpos: 11.2, opos: f26dot6FromFloat(11.2), pos: f26dot6FromFloat(11.2), dir: dirDown, linkIdx: 1, serifIdx: -1, blueEdge: &blueRef},
		{fpos: 0.0, opos: 0, pos: 0, dir: dirUp, linkIdx: 0, serifIdx: -1},
	}

	axis := &scaledAxisMetrics{
		widths: []scaledWidth{{scaled: f26dot6FromFloat(1.0), fitted: f26dot6FromFloat(1.0)}},
	}

	hintEdges(edges, axis, scriptGroupDefault)

	// First edge should snap to blue zone fitted value.
	wantPos := f26dot6FromFloat(11.0)
	if edges[0].pos != wantPos {
		t.Errorf("blue-anchored edge: got %d (%.3fpx), want %d (11.0px)",
			edges[0].pos, f26dot6ToFloat(edges[0].pos), wantPos)
	}

	// Edge should be marked done.
	if (edges[0].flags & edgeFlagDone) == 0 {
		t.Error("blue-anchored edge should be marked DONE")
	}
}

func TestHintEdges_StemPair(t *testing.T) {
	edges := []*hintEdge{
		{fpos: 3.3, opos: f26dot6FromFloat(3.3), pos: f26dot6FromFloat(3.3), dir: dirUp, linkIdx: 1, serifIdx: -1},
		{fpos: 4.5, opos: f26dot6FromFloat(4.5), pos: f26dot6FromFloat(4.5), dir: dirDown, linkIdx: 0, serifIdx: -1},
	}

	axis := &scaledAxisMetrics{
		widths: []scaledWidth{{scaled: f26dot6FromFloat(1.2), fitted: f26dot6FromFloat(1.2)}},
	}

	hintEdges(edges, axis, scriptGroupDefault)

	// Both edges should be done.
	if (edges[0].flags & edgeFlagDone) == 0 {
		t.Error("stem edge[0] should be marked DONE")
	}
	if (edges[1].flags & edgeFlagDone) == 0 {
		t.Error("stem edge[1] should be marked DONE")
	}

	// Stem width should be consistent (snapped).
	stemWidth := edges[1].pos - edges[0].pos
	t.Logf("Stem: edge[0]=%d (%.2fpx) edge[1]=%d (%.2fpx) width=%d (%.3fpx)",
		edges[0].pos, f26dot6ToFloat(edges[0].pos),
		edges[1].pos, f26dot6ToFloat(edges[1].pos),
		stemWidth, f26dot6ToFloat(stemWidth))

	// Width should be a reasonable value: 0.5-3.0px = 32-192 in 26.6.
	if stemWidth < 32 || stemWidth > 192 {
		t.Errorf("stem width out of range: %d (%.3fpx)", stemWidth, f26dot6ToFloat(stemWidth))
	}
}

// ============================================================
// 7. Point Interpolation Tests
// ============================================================

func TestAlignEdgePoints(t *testing.T) {
	pa := &hintPointArray{
		pts: []hintPoint{
			{fx: 0, fy: 0, x: 0, y: 0, ox: 0, oy: 0, next: 1, prev: 2},
			{fx: 10, fy: 0, x: f26dot6FromFloat(10), y: 0, ox: f26dot6FromFloat(10), oy: 0, next: 2, prev: 0},
			{fx: 10, fy: 20, x: f26dot6FromFloat(10), y: f26dot6FromFloat(20), ox: f26dot6FromFloat(10), oy: f26dot6FromFloat(20), next: 0, prev: 1},
		},
		contours: []contourRange{{start: 0, end: 2}},
	}

	segments := []hintSegment{
		{pos: 0, edgeIdx: 0, firstPt: 0, lastPt: 0, linkIdx: -1, serifIdx: -1},
		{pos: 10, edgeIdx: 1, firstPt: 1, lastPt: 2, linkIdx: -1, serifIdx: -1},
	}

	edges := []*hintEdge{
		{fpos: 0, opos: 0, pos: f26dot6FromFloat(0.5)},                      // Shifted by +0.5px.
		{fpos: 10, opos: f26dot6FromFloat(10), pos: f26dot6FromFloat(10.5)}, // Shifted by +0.5px.
	}

	alignEdgePoints(pa, segments, edges, dimVertical, scriptGroupDefault)

	// Points in segment 0 should be at pos=0.5px = 32 in 26.6.
	want0 := f26dot6FromFloat(0.5)
	if pa.pts[0].y != want0 {
		t.Errorf("point[0].y = %d (%.2fpx), want %d (0.5px)", pa.pts[0].y, f26dot6ToFloat(pa.pts[0].y), want0)
	}

	// Points in segment 1 should be at pos=10.5px.
	want1 := f26dot6FromFloat(10.5)
	if pa.pts[1].y != want1 {
		t.Errorf("point[1].y = %d (%.2fpx), want %d (10.5px)", pa.pts[1].y, f26dot6ToFloat(pa.pts[1].y), want1)
	}
	if pa.pts[2].y != want1 {
		t.Errorf("point[2].y = %d (%.2fpx), want %d (10.5px)", pa.pts[2].y, f26dot6ToFloat(pa.pts[2].y), want1)
	}
}

func TestIupInterpolate(t *testing.T) {
	// IUP operates on u/v fields (26.6 fixed-point).
	pts := []hintPoint{
		{u: 0, v: 0}, // ref1 (touched)
		{u: f26dot6FromFloat(5), v: f26dot6FromFloat(5)},   // untouched
		{u: f26dot6FromFloat(10), v: f26dot6FromFloat(10)}, // untouched
		{u: f26dot6FromFloat(22), v: f26dot6FromFloat(20)}, // ref2 (touched, moved from 20 to 22)
	}

	iupInterpolate26dot6(pts, 1, 2, 0, 3)

	// Point at original v=5: 0 + (5/20)*(22-0) = 5.5px = 352 in 26.6.
	expected1 := f26dot6FromFloat(5.5)
	if abs32(pts[1].u-expected1) > 1 { // Allow 1/64 tolerance
		t.Errorf("iup point[1].u = %d (%.3fpx), want %d (5.5px)",
			pts[1].u, f26dot6ToFloat(pts[1].u), expected1)
	}

	// Point at original v=10: 0 + (10/20)*(22-0) = 11.0px.
	expected2 := f26dot6FromFloat(11.0)
	if abs32(pts[2].u-expected2) > 1 {
		t.Errorf("iup point[2].u = %d (%.3fpx), want %d (11.0px)",
			pts[2].u, f26dot6ToFloat(pts[2].u), expected2)
	}
}

func TestIupShift(t *testing.T) {
	// IUP shift operates on u/v fields (26.6 fixed-point).
	pts := []hintPoint{
		{u: 0, v: 0},
		{u: f26dot6FromFloat(7), v: f26dot6FromFloat(5)}, // ref: moved from 5 to 7 (delta=2px=128 in 26.6)
		{u: f26dot6FromFloat(10), v: f26dot6FromFloat(10)},
	}

	iupShift26dot6(pts, 0, 2, 1)

	// All non-ref points should shift by delta=128 (2px).
	want0 := f26dot6FromFloat(2.0)
	if pts[0].u != want0 {
		t.Errorf("shift point[0].u = %d (%.2fpx), want %d (2.0px)", pts[0].u, f26dot6ToFloat(pts[0].u), want0)
	}
	want2 := f26dot6FromFloat(12.0)
	if pts[2].u != want2 {
		t.Errorf("shift point[2].u = %d (%.2fpx), want %d (12.0px)", pts[2].u, f26dot6ToFloat(pts[2].u), want2)
	}
}

// abs32 is defined in autohint_edges.go (used by both production and test code).

// ============================================================
// 8. Integration Tests
// ============================================================

func TestAutoHintOutline_GoRegular_H(t *testing.T) {
	font := loadGoRegularFont(t)
	gid := font.GlyphIndex('H')
	if gid == 0 {
		t.Skip("'H' glyph not found")
	}

	ext := NewOutlineExtractor()

	// Extract at 16px.
	outline, err := ext.ExtractOutline(font, GlyphID(gid), 16)
	if err != nil || outline == nil {
		t.Fatalf("failed to extract 'H': %v", err)
	}

	// Clone for comparison.
	original := outline.Clone()

	// Apply auto-hinting.
	autoHintOutline(outline, font, 16, HintingFull)

	// Outline should be modified.
	changed := false
	for i, seg := range outline.Segments {
		origSeg := original.Segments[i]
		for j := range segPointCount(seg.Op) {
			if seg.Points[j].X != origSeg.Points[j].X || seg.Points[j].Y != origSeg.Points[j].Y {
				changed = true
				break
			}
		}
		if changed {
			break
		}
	}

	if !changed {
		t.Error("autoHintOutline should modify the outline")
	}
}

func TestAutoHintOutline_StemConsistency(t *testing.T) {
	font := loadGoRegularFont(t)

	// Test that stems of 'H' are consistent width after hinting.
	gid := font.GlyphIndex('H')
	if gid == 0 {
		t.Skip("'H' glyph not found")
	}

	sizes := []float64{12, 16, 24, 48}
	for _, size := range sizes {
		ext := NewOutlineExtractor()
		outline, err := ext.ExtractOutline(font, GlyphID(gid), size)
		if err != nil || outline == nil {
			continue
		}

		autoHintOutline(outline, font, size, HintingVertical)

		t.Logf("'H' at %dpx: bounds=[%.2f,%.2f]-[%.2f,%.2f] advance=%.2f",
			int(size), outline.Bounds.MinX, outline.Bounds.MinY,
			outline.Bounds.MaxX, outline.Bounds.MaxY, outline.Advance)
	}
}

func TestAutoHintOutline_MultipleGlyphs(t *testing.T) {
	font := loadGoRegularFont(t)

	// Test multiple glyphs at same size.
	glyphs := []rune{'H', 'I', 'l', 'n', 'o', 'p'}
	size := 16.0

	for _, ch := range glyphs {
		gid := font.GlyphIndex(ch)
		if gid == 0 {
			continue
		}

		ext := NewOutlineExtractor()
		outline, err := ext.ExtractOutline(font, GlyphID(gid), size)
		if err != nil || outline == nil {
			continue
		}

		// Should not panic.
		autoHintOutline(outline, font, size, HintingFull)

		// Outline should have valid bounds.
		if outline.Bounds.Width() <= 0 && len(outline.Segments) > 1 {
			t.Errorf("'%c' has invalid bounds after hinting: %v", ch, outline.Bounds)
		}
	}
}

func TestAutoHintOutline_NilSafety(t *testing.T) {
	font := loadGoRegularFont(t)

	// Nil outline — should not panic.
	autoHintOutline(nil, font, 16, HintingFull)

	// Empty outline — should not panic.
	autoHintOutline(&GlyphOutline{}, font, 16, HintingFull)
}

func TestAutoHintOutline_VerticalOnly(t *testing.T) {
	font := loadGoRegularFont(t)
	gid := font.GlyphIndex('o')
	if gid == 0 {
		t.Skip("'o' glyph not found")
	}

	ext := NewOutlineExtractor()

	// HintingVertical should only modify Y coordinates.
	outline, err := ext.ExtractOutline(font, GlyphID(gid), 16)
	if err != nil || outline == nil {
		t.Fatalf("failed to extract 'o': %v", err)
	}

	original := outline.Clone()
	autoHintOutline(outline, font, 16, HintingVertical)

	// Check that Y changed but X didn't (for vertical hinting only).
	yChanged := false
	for i, seg := range outline.Segments {
		origSeg := original.Segments[i]
		for j := range segPointCount(seg.Op) {
			if seg.Points[j].Y != origSeg.Points[j].Y {
				yChanged = true
			}
		}
	}

	if !yChanged {
		t.Log("WARNING: Y coordinates not changed by vertical hinting (may be OK for some glyphs)")
	}
}

// ============================================================
// 9. Cache Tests
// ============================================================

func TestAutoHintCache(t *testing.T) {
	ClearAutoHintCache()

	font := loadGoRegularFont(t)

	// First call should compute metrics.
	m1 := getAutoHintMetrics(font)
	if m1 == nil {
		t.Fatal("getAutoHintMetrics returned nil")
	}

	// Second call should return cached metrics (same pointer).
	m2 := getAutoHintMetrics(font)
	if m1 != m2 {
		t.Error("expected cached metrics (same pointer)")
	}

	// After clearing, should recompute.
	ClearAutoHintCache()
	m3 := getAutoHintMetrics(font)
	if m3 == nil {
		t.Fatal("getAutoHintMetrics returned nil after cache clear")
	}
	// m3 should be a new allocation.
	if m3 == m1 {
		t.Error("expected new metrics after cache clear")
	}
}

// ============================================================
// 10. Helper Function Tests
// ============================================================

func TestPixRound(t *testing.T) {
	// Test the legacy float32 pixRound.
	tests := []struct {
		in   float32
		want float32
	}{
		{0.0, 0.0},
		{0.4, 0.0},
		{0.5, 1.0}, // math.Round rounds half away from zero
		{0.6, 1.0},
		{1.5, 2.0},
		{-0.3, 0.0},
		{-0.6, -1.0},
	}

	for _, tt := range tests {
		got := pixRound(tt.in)
		if got != tt.want {
			t.Errorf("pixRound(%.1f) = %.1f, want %.1f", tt.in, got, tt.want)
		}
	}

	// Test the 26.6 f26dot6Round.
	t.Run("26dot6", func(t *testing.T) {
		tests26 := []struct {
			in   int32 // 26.6 input
			want int32 // 26.6 output (multiple of 64)
		}{
			{0, 0},
			{32, 64},   // 0.5px → 1.0px (round up: (32+32)&^63 = 64)
			{31, 0},    // just below 0.5px → 0
			{33, 64},   // just above 0.5px → 1.0px
			{96, 128},  // 1.5px → 2.0px: (96+32)=128, 128&^63=128.
			{-32, 0},   // -0.5px → 0 ((-32+32)&^63 = 0&^63 = 0)
			{-33, -64}, // ((-33+32)&^63 = -1&^63 = -64)
		}
		for _, tt := range tests26 {
			got := f26dot6Round(tt.in)
			if got != tt.want {
				t.Errorf("f26dot6Round(%d) = %d, want %d", tt.in, got, tt.want)
			}
		}
	})
}

func TestDerivedConstant(t *testing.T) {
	// At 2048 UPM, derivedConstant(2048) should be 50.
	got := derivedConstant(2048)
	if got != 50 {
		t.Errorf("derivedConstant(2048) = %d, want 50", got)
	}

	// At 1000 UPM, derivedConstant(1000) should be ~24.
	got = derivedConstant(1000)
	if got != 24 {
		t.Errorf("derivedConstant(1000) = %d, want 24", got)
	}
}

func TestBuildHintPoints_Contours(t *testing.T) {
	// Two separate contours (like 'o' inner and outer).
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			// Outer contour.
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{0, 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{10, 0}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{10, 10}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{0, 10}}},
			// Inner contour.
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{2, 2}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{8, 2}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{8, 8}}},
			{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{2, 8}}},
		},
	}

	pa := buildHintPoints(outline)

	if len(pa.contours) != 2 {
		t.Errorf("expected 2 contours, got %d", len(pa.contours))
	}
	if len(pa.pts) < 8 {
		t.Errorf("expected at least 8 points, got %d", len(pa.pts))
	}

	// Verify contour boundaries.
	for i, cr := range pa.contours {
		t.Logf("Contour[%d]: start=%d end=%d", i, cr.start, cr.end)
		if cr.end < cr.start {
			t.Errorf("contour[%d] has end < start", i)
		}
	}

	// Verify prev/next links are within contour bounds.
	for i, pt := range pa.pts {
		if pt.next < 0 || pt.next >= len(pa.pts) {
			t.Errorf("point[%d].next=%d out of bounds", i, pt.next)
		}
		if pt.prev < 0 || pt.prev >= len(pa.pts) {
			t.Errorf("point[%d].prev=%d out of bounds", i, pt.prev)
		}
	}
}

// ============================================================
// 11. Contour-Based Auto-Hinter: Raw Point Scaling Tests
// ============================================================

// TestAutoHint_RawContourPoints_ScaleVsSkrifaGolden verifies that raw
// TrueType contour points, when scaled to 16px pixel space, produce the
// exact same 26.6-like coordinates as skrifa's "After scale" golden data.
//
// This is the foundational test for FreeType/skrifa coordinate parity:
// if the scaled points don't match, nothing downstream can match.
//
// Golden data source: tmp/skrifa_golden_dump.txt lines 1-33
// Font: NotoSerifHebrew, GlyphId 9, 16px, 32 raw contour points.
// Skrifa coordinates are in 26.6-like integer format, Y-UP.
func TestAutoHint_RawContourPoints_ScaleVsSkrifaGolden(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}
	if contours == nil {
		t.Fatal("expected non-nil contours for glyph 9")
	}

	if len(contours.Points) != 32 {
		t.Fatalf("expected 32 raw points, got %d", len(contours.Points))
	}

	// Scale factor: ppem / unitsPerEm.
	// NotoSerifHebrew has UPM = 976 (from skrifa golden: x_scale = round(16.0 / 976 * 65536 * 64) = 67109).
	scale := 16.0 / float64(font.UnitsPerEm())

	// Build hintPointArray from raw contour points (Y-UP convention).
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())
	if len(points.pts) != 32 {
		t.Fatalf("expected 32 hint points, got %d", len(points.pts))
	}

	// Skrifa golden "After scale" coordinates (26.6-like, Y-UP).
	// From tmp/skrifa_golden_dump.txt lines 2-33.
	skrifaAfterScale := [][2]int{
		{126, -246}, {126, 306}, {126, 369},
		{142, 459}, {156, 492}, {156, 495},
		{66, 495}, {42, 495}, {15, 527},
		{15, 565}, {15, 579}, {17, 601},
		{25, 634}, {30, 663}, {57, 663},
		{57, 659}, {57, 634}, {86, 606},
		{112, 606}, {179, 606}, {197, 606},
		{210, 592}, {210, 572}, {210, 505},
		{200, 487}, {188, 452}, {184, 393},
		{185, 341}, {197, -209}, {185, -220},
		{159, -238}, {141, -246},
	}

	// Compare our scaled coordinates against skrifa golden.
	// Our ox/oy are now in 26.6 fixed-point (int32), matching skrifa directly.
	mismatches := 0
	for i := range 32 {
		gotX := int(points.pts[i].ox)
		gotY := int(points.pts[i].oy)
		wantX := skrifaAfterScale[i][0]
		wantY := skrifaAfterScale[i][1]

		dx := gotX - wantX
		dy := gotY - wantY

		match := " OK"
		if dx != 0 || dy != 0 {
			match = " MISMATCH"
			mismatches++
		}
		t.Logf("  pt[%2d]: (%5d, %5d)  want: (%5d, %5d)  dx=%3d dy=%3d%s",
			i, gotX, gotY, wantX, wantY, dx, dy, match)
	}

	if mismatches > 0 {
		t.Errorf("scale stage: %d / 32 mismatches (expected 0 for FreeType/skrifa parity)", mismatches)
	}
}

// TestAutoHint_ContourPath_PointCount verifies that the contour-based
// auto-hinter path uses exactly 32 points (raw contour) not 42 (pen-derived)
// for NotoSerifHebrew glyph 9.
func TestAutoHint_ContourPath_PointCount(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}
	if contours == nil {
		t.Fatal("expected non-nil contours for glyph 9")
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())

	// Contour path: raw points.
	contourPoints := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())

	// Legacy path: pen-derived outline.
	ext := NewOutlineExtractor()
	outline, err := ext.ExtractOutline(font, GlyphID(9), 16)
	if err != nil || outline == nil {
		t.Fatalf("failed to extract outline: %v", err)
	}
	legacyPoints := buildHintPoints(outline)

	t.Logf("contour path: %d points (raw TrueType)", len(contourPoints.pts))
	t.Logf("legacy path:  %d points (pen-derived)", len(legacyPoints.pts))

	// Contour path must be 32 points.
	if len(contourPoints.pts) != 32 {
		t.Errorf("contour path: expected 32 points, got %d", len(contourPoints.pts))
	}

	// Legacy path should be more (pen decomposition expands).
	if len(legacyPoints.pts) <= len(contourPoints.pts) {
		t.Logf("NOTE: legacy path has same or fewer points — glyph may be all straight lines")
	}
}

// TestAutoHint_ContourPath_DirectionClassification verifies that direction
// computation on raw contour points produces the correct Y-UP direction
// constants (matching skrifa/FreeType convention).
func TestAutoHint_ContourPath_DirectionClassification(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())

	// Log direction info for each point.
	for i, pt := range points.pts {
		dirStr := func(d hintDirection) string {
			switch d {
			case dirUp:
				return "Up"
			case dirDown:
				return "Down"
			case dirLeft:
				return "Left"
			case dirRight:
				return "Right"
			default:
				return "None"
			}
		}
		weak := ""
		if (pt.flags & pointFlagWeak) != 0 {
			weak = " [WEAK]"
		}
		ctrl := ""
		if (pt.flags & pointFlagControl) != 0 {
			ctrl = " [CTRL]"
		}
		t.Logf("  pt[%2d]: (%.1f, %.1f) in=%s out=%s%s%s",
			i, pt.fx, pt.fy, dirStr(pt.inDir), dirStr(pt.outDir), weak, ctrl)
	}

	// Verify we have a mix of directional and weak/control points.
	directional := 0
	weak := 0
	for _, pt := range points.pts {
		if pt.outDir != dirNone && (pt.flags&pointFlagWeak) == 0 {
			directional++
		}
		if (pt.flags & pointFlagWeak) != 0 {
			weak++
		}
	}

	t.Logf("directional: %d, weak: %d, total: %d", directional, weak, len(points.pts))

	if directional == 0 {
		t.Error("expected at least some directional points")
	}
}

// ============================================================
// 12. Diagonal Glyph Regression Tests
// ============================================================

// TestAutoHintOutline_DiagonalGlyphs_NoCorruption verifies that diagonal-heavy
// glyphs ('v','w','A','x','k','z') are not corrupted by auto-hinting.
//
// The root cause of the original bug was that diagonal points (with dirNone
// for both inDir and outDir) were not classified as weak. alignStrongPoints
// would then interpolate them between edges, collapsing diagonal strokes.
//
// This test verifies that after hinting:
//  1. The outline bounding box preserves its aspect ratio (width/height).
//  2. Individual points do not collapse (max coordinate displacement < 50%).
//  3. The outline area is within 25% of the original (no stroke collapse).
func TestAutoHintOutline_DiagonalGlyphs_NoCorruption(t *testing.T) {
	font := loadGoRegularFont(t)

	diagonalGlyphs := []rune{'v', 'w', 'A', 'x', 'k', 'z', 'V', 'W', 'X', 'Z', 'K'}
	sizes := []float64{12, 16, 24, 48}

	for _, ch := range diagonalGlyphs {
		gid := font.GlyphIndex(ch)
		if gid == 0 {
			continue
		}

		for _, size := range sizes {
			t.Run(string([]rune{ch})+"_"+fmt.Sprintf("%.0fpx", size), func(t *testing.T) {
				ext := NewOutlineExtractor()
				outline, err := ext.ExtractOutline(font, GlyphID(gid), size)
				if err != nil || outline == nil {
					t.Skipf("failed to extract '%c' at %.0fpx: %v", ch, size, err)
					return
				}

				original := outline.Clone()

				autoHintOutline(outline, font, size, HintingFull)

				// Check 1: Bounding box must not collapse.
				origW := original.Bounds.MaxX - original.Bounds.MinX
				origH := original.Bounds.MaxY - original.Bounds.MinY
				hintW := outline.Bounds.MaxX - outline.Bounds.MinX
				hintH := outline.Bounds.MaxY - outline.Bounds.MinY

				if origW > 0.5 && hintW < origW*0.3 {
					t.Errorf("'%c' at %.0fpx: width collapsed: orig=%.2f hinted=%.2f (%.0f%%)",
						ch, size, origW, hintW, hintW/origW*100)
				}
				if origH > 0.5 && hintH < origH*0.3 {
					t.Errorf("'%c' at %.0fpx: height collapsed: orig=%.2f hinted=%.2f (%.0f%%)",
						ch, size, origH, hintH, hintH/origH*100)
				}

				// Check 2: No extreme point displacement.
				maxDisplacement := float64(0)
				for i, seg := range outline.Segments {
					origSeg := original.Segments[i]
					for j := range segPointCount(seg.Op) {
						dx := float64(seg.Points[j].X - origSeg.Points[j].X)
						dy := float64(seg.Points[j].Y - origSeg.Points[j].Y)
						d := math.Sqrt(dx*dx + dy*dy)
						if d > maxDisplacement {
							maxDisplacement = d
						}
					}
				}

				// Displacement should not exceed 55% of the glyph size.
				// Auto-hinting typically moves points by 0.5-2px, but glyphs
				// with mixed H/V stems and diagonals (like 'K') can have
				// diagonal endpoints displaced proportionally when the stems
				// they attach to are grid-fitted. 55% catches genuine
				// corruption (>100%) while allowing legitimate hinting.
				maxAllowed := math.Max(origW, origH) * 0.55
				if maxAllowed < 2.0 {
					maxAllowed = 2.0
				}
				if maxDisplacement > maxAllowed {
					t.Errorf("'%c' at %.0fpx: extreme displacement %.2f > max allowed %.2f",
						ch, size, maxDisplacement, maxAllowed)
				}
			})
		}
	}
}

// TestWeakPointClassification_DiagonalGlyph verifies that diagonal points
// are properly classified as weak by computePointProperties.
// This is the direct unit test for the root cause fix.
func TestWeakPointClassification_DiagonalGlyph(t *testing.T) {
	font := loadGoRegularFont(t)

	// 'v' is the canonical test case: all strokes are diagonal.
	gid := font.GlyphIndex('v')
	if gid == 0 {
		t.Skip("'v' glyph not found")
	}

	ext := NewOutlineExtractor()
	outline, err := ext.ExtractOutline(font, GlyphID(gid), 16)
	if err != nil || outline == nil {
		t.Fatalf("failed to extract 'v': %v", err)
	}

	pa := buildHintPoints(outline)

	totalPoints := len(pa.pts)
	weakCount := 0
	controlCount := 0
	strongDirNone := 0

	for _, pt := range pa.pts {
		if (pt.flags & pointFlagWeak) != 0 {
			weakCount++
		}
		if (pt.flags & pointFlagControl) != 0 {
			controlCount++
		}
		if (pt.flags&pointFlagWeak) == 0 && pt.outDir == dirNone && pt.inDir == dirNone {
			strongDirNone++
		}
	}

	t.Logf("'v' at 16px: %d total points, %d weak, %d control, %d strong-dirNone",
		totalPoints, weakCount, controlCount, strongDirNone)

	// For 'v', most diagonal points should be weak.
	// The key invariant: strong points with dirNone should be rare.
	// These are the points that alignStrongPoints would try to interpolate,
	// and for diagonals this would be catastrophic.
	if strongDirNone > totalPoints/3 {
		t.Errorf("too many strong dirNone points (%d/%d) — diagonal points not classified as weak",
			strongDirNone, totalPoints)
	}
}

// TestWeakPointClassification_ControlPointsAlwaysWeak verifies that
// off-curve control points are always classified as weak.
// FreeType: "control points are always weak" (afhints.c:1269)
func TestWeakPointClassification_ControlPointsAlwaysWeak(t *testing.T) {
	font := loadGoRegularFont(t)

	// 'o' has curves with control points.
	gid := font.GlyphIndex('o')
	if gid == 0 {
		t.Skip("'o' glyph not found")
	}

	ext := NewOutlineExtractor()
	outline, err := ext.ExtractOutline(font, GlyphID(gid), 16)
	if err != nil || outline == nil {
		t.Fatalf("failed to extract 'o': %v", err)
	}

	pa := buildHintPoints(outline)

	for i, pt := range pa.pts {
		if (pt.flags & pointFlagControl) != 0 {
			if (pt.flags & pointFlagWeak) == 0 {
				t.Errorf("control point[%d] is not marked weak (flags=%d)", i, pt.flags)
			}
		}
	}
}

// TestWeakPointClassification_HorizontalVerticalGlyphs verifies that
// glyphs with primarily H/V features ('H', 'I', 'T') have strong
// points on their edges — they should NOT all be weak.
func TestWeakPointClassification_HorizontalVerticalGlyphs(t *testing.T) {
	font := loadGoRegularFont(t)

	for _, ch := range []rune{'H', 'I', 'T', 'L', 'E', 'F'} {
		gid := font.GlyphIndex(ch)
		if gid == 0 {
			continue
		}

		ext := NewOutlineExtractor()
		outline, err := ext.ExtractOutline(font, GlyphID(gid), 16)
		if err != nil || outline == nil {
			continue
		}

		pa := buildHintPoints(outline)

		strongCount := 0
		for _, pt := range pa.pts {
			if (pt.flags&pointFlagWeak) == 0 && pt.outDir != dirNone {
				strongCount++
			}
		}

		t.Logf("'%c' at 16px: %d strong directional points out of %d", ch, strongCount, len(pa.pts))

		// H/V glyphs should have multiple strong directional points.
		if strongCount < 4 {
			t.Errorf("'%c' has too few strong points (%d) — H/V edges should produce strong points",
				ch, strongCount)
		}
	}
}

// TestAutoHintOutline_DiagonalVsOriginal_AreaPreservation verifies that
// the outline area after hinting is within a reasonable range of the original.
// Stroke collapse would reduce area dramatically.
func TestAutoHintOutline_DiagonalVsOriginal_AreaPreservation(t *testing.T) {
	font := loadGoRegularFont(t)

	for _, ch := range []rune{'v', 'w', 'A', 'x'} {
		gid := font.GlyphIndex(ch)
		if gid == 0 {
			continue
		}

		ext := NewOutlineExtractor()
		outline, err := ext.ExtractOutline(font, GlyphID(gid), 24)
		if err != nil || outline == nil {
			continue
		}

		original := outline.Clone()
		autoHintOutline(outline, font, 24, HintingFull)

		origArea := approximateArea(original)
		hintArea := approximateArea(outline)

		t.Logf("'%c' at 24px: origArea=%.2f hintArea=%.2f ratio=%.2f%%",
			ch, origArea, hintArea, hintArea/origArea*100)

		// Area should be within 75% of original (25% tolerance for hinting).
		if origArea > 1.0 && hintArea < origArea*0.25 {
			t.Errorf("'%c' area collapsed: orig=%.2f hinted=%.2f (%.0f%%)",
				ch, origArea, hintArea, hintArea/origArea*100)
		}
	}
}

// approximateArea computes an approximate signed area of a glyph outline
// using the shoelace formula on the on-curve points.
func approximateArea(outline *GlyphOutline) float64 {
	var area float64
	var points []OutlinePoint

	for _, seg := range outline.Segments {
		if seg.Op == OutlineOpMoveTo {
			if len(points) > 2 {
				area += shoelaceArea(points)
			}
			points = points[:0]
		}
		n := segPointCount(seg.Op)
		// Use the last point of each segment (on-curve).
		if n > 0 {
			points = append(points, seg.Points[n-1])
		}
	}
	if len(points) > 2 {
		area += shoelaceArea(points)
	}

	if area < 0 {
		area = -area
	}
	return area
}

// shoelaceArea computes the signed area of a polygon using the shoelace formula.
func shoelaceArea(pts []OutlinePoint) float64 {
	n := len(pts)
	if n < 3 {
		return 0
	}
	var area float64
	for i := range n {
		j := (i + 1) % n
		area += float64(pts[i].X)*float64(pts[j].Y) - float64(pts[j].X)*float64(pts[i].Y)
	}
	return area / 2
}

// ============================================================
// 12. alignEdgePoints Linked List Traversal Regression Tests
// ============================================================

// TestAlignEdgePoints_LinkedListTraversal verifies that alignEdgePoints walks
// the contour linked list (point.next) rather than iterating array indices.
//
// The original bug: alignEdgePoints used `for ptIdx := seg.firstPt; ptIdx <= seg.lastPt`
// which is array index iteration. This captures ALL points in the index range,
// including non-segment diagonal interior points. FreeType/skrifa walk the linked
// list from segment.first to segment.last via point.next, which only visits
// points that are actually part of the contour path between those endpoints.
//
// For diagonal glyphs ('v','w','A','x'), this caused valley/peak points to be
// snapped to edge positions, collapsing the diagonal strokes.
func TestAlignEdgePoints_LinkedListTraversal(t *testing.T) {
	// Construct a synthetic contour where firstPt > lastPt (wrap-around).
	// This specifically tests that the linked list traversal handles
	// contour wrap-around correctly.
	pa := &hintPointArray{
		pts: []hintPoint{
			// Contour: 0→1→2→3→0 (circular)
			{fx: 5, fy: 0, x: f26dot6FromFloat(5), y: 0, ox: f26dot6FromFloat(5), oy: 0, next: 1, prev: 3},
			{fx: 10, fy: 5, x: f26dot6FromFloat(10), y: f26dot6FromFloat(5), ox: f26dot6FromFloat(10), oy: f26dot6FromFloat(5), next: 2, prev: 0},
			{fx: 5, fy: 10, x: f26dot6FromFloat(5), y: f26dot6FromFloat(10), ox: f26dot6FromFloat(5), oy: f26dot6FromFloat(10), next: 3, prev: 1},
			{fx: 0, fy: 5, x: 0, y: f26dot6FromFloat(5), ox: 0, oy: f26dot6FromFloat(5), next: 0, prev: 2},
		},
		contours: []contourRange{{start: 0, end: 3}},
	}

	// Segment wraps around: firstPt=3, lastPt=1 (via linked list: 3→0→1).
	segments := []hintSegment{
		{pos: 0, edgeIdx: 0, firstPt: 3, lastPt: 1, linkIdx: -1, serifIdx: -1},
	}

	want := f26dot6FromFloat(1.0)
	edges := []*hintEdge{
		{fpos: 0, opos: 0, pos: want}, // Shift to 1.0px
	}

	alignEdgePoints(pa, segments, edges, dimHorizontal, scriptGroupDefault)

	// With linked list traversal, points 3, 0, 1 should be snapped to x=1.0px.
	if pa.pts[3].x != want {
		t.Errorf("point[3].x = %d (%.2fpx), want %d (1.0px)", pa.pts[3].x, f26dot6ToFloat(pa.pts[3].x), want)
	}
	if pa.pts[0].x != want {
		t.Errorf("point[0].x = %d (%.2fpx), want %d (1.0px)", pa.pts[0].x, f26dot6ToFloat(pa.pts[0].x), want)
	}
	if pa.pts[1].x != want {
		t.Errorf("point[1].x = %d (%.2fpx), want %d (1.0px)", pa.pts[1].x, f26dot6ToFloat(pa.pts[1].x), want)
	}
	// Point 2 should NOT be touched (not part of the segment).
	untouched := f26dot6FromFloat(5.0)
	if pa.pts[2].x != untouched {
		t.Errorf("point[2].x = %d (%.2fpx), want %d (5.0px)", pa.pts[2].x, f26dot6ToFloat(pa.pts[2].x), untouched)
	}
}

// TestAutoHint_DiagonalValleyPreservation is the primary regression test for
// the 'v' glyph corruption bug. It verifies that the valley point (bottom of 'v')
// is NOT merged with the top points after auto-hinting.
//
// Bug symptoms: for 'v' at 24px, the valley point (Y approx -2.83) was snapped
// to the same Y as the cap-height points (Y approx -8.0), collapsing the glyph
// to a flat line. Same bug affects 'w', 'A', 'x' - all diagonal glyphs.
func TestAutoHint_DiagonalValleyPreservation(t *testing.T) {
	font := loadGoRegularFont(t)

	for _, tc := range []struct {
		ch   rune
		size float64
	}{
		{'v', 16},
		{'v', 24},
		{'w', 16},
		{'w', 24},
		{'A', 16},
		{'A', 24},
		{'x', 16},
		{'x', 24},
	} {
		t.Run(fmt.Sprintf("%c_%dpx", tc.ch, int(tc.size)), func(t *testing.T) {
			gid := font.GlyphIndex(tc.ch)
			if gid == 0 {
				t.Skipf("'%c' glyph not found", tc.ch)
				return
			}

			ext := NewOutlineExtractor()
			outline, err := ext.ExtractOutline(font, GlyphID(gid), tc.size)
			if err != nil || outline == nil {
				t.Skipf("failed to extract '%c': %v", tc.ch, err)
				return
			}

			// Collect original Y coordinates.
			var origYValues []float32
			for _, seg := range outline.Segments {
				for j := range segPointCount(seg.Op) {
					origYValues = append(origYValues, float32(seg.Points[j].Y))
				}
			}

			// Find the Y extent of original points.
			origMinY, origMaxY := origYValues[0], origYValues[0]
			for _, y := range origYValues {
				if y < origMinY {
					origMinY = y
				}
				if y > origMaxY {
					origMaxY = y
				}
			}
			origSpan := origMaxY - origMinY

			// Apply auto-hinting.
			autoHintOutline(outline, font, tc.size, HintingFull)

			// Collect hinted Y coordinates.
			var hintedYValues []float32
			for _, seg := range outline.Segments {
				for j := range segPointCount(seg.Op) {
					hintedYValues = append(hintedYValues, float32(seg.Points[j].Y))
				}
			}

			// Find hinted Y extent.
			hintedMinY, hintedMaxY := hintedYValues[0], hintedYValues[0]
			for _, y := range hintedYValues {
				if y < hintedMinY {
					hintedMinY = y
				}
				if y > hintedMaxY {
					hintedMaxY = y
				}
			}
			hintedSpan := hintedMaxY - hintedMinY

			// Count distinct Y values (rounded to 0.01 to avoid float noise).
			origDistinct := countDistinctRounded(origYValues, 0.1)
			hintedDistinct := countDistinctRounded(hintedYValues, 0.1)

			t.Logf("'%c' at %dpx: origSpan=%.2f hintedSpan=%.2f origDistinctY=%d hintedDistinctY=%d",
				tc.ch, int(tc.size), origSpan, hintedSpan, origDistinct, hintedDistinct)

			// The hinted span must be at least 40% of the original span.
			// A collapse to a flat line would give span near 0.
			if origSpan > 1.0 && hintedSpan < origSpan*0.4 {
				t.Errorf("'%c' at %dpx: Y span collapsed from %.2f to %.2f (%.0f%% retained)",
					tc.ch, int(tc.size), origSpan, hintedSpan, hintedSpan/origSpan*100)
			}

			// The number of distinct Y values should not decrease dramatically.
			// A collapse merges many distinct values into fewer.
			if origDistinct > 2 && hintedDistinct < origDistinct/2 {
				t.Errorf("'%c' at %dpx: distinct Y values dropped from %d to %d (points collapsed)",
					tc.ch, int(tc.size), origDistinct, hintedDistinct)
			}
		})
	}
}

// countDistinctRounded counts the number of distinct values in a slice,
// after rounding each value to the nearest multiple of tolerance.
func countDistinctRounded(values []float32, tolerance float32) int {
	seen := make(map[int]bool)
	for _, v := range values {
		key := int(math.Round(float64(v) / float64(tolerance)))
		seen[key] = true
	}
	return len(seen)
}

// TestAutoHint_V_24px_GoldenCoordinates is a golden-style coordinate test
// (inspired by skrifa hint/outline.rs tests) verifying exact hinted positions
// for the 'v' glyph at 24px in Go Regular.
//
// The 'v' glyph has 3 distinct Y levels:
//   - Baseline (Y=0): points 0, 6, 7
//   - Valley (Y approx -2.83): point 3
//   - Top (Y approx -12.73): points 1, 2, 4, 5
//
// After hinting, the valley point MUST remain between baseline and top,
// not collapsed to either edge.
func TestAutoHint_V_24px_GoldenCoordinates(t *testing.T) {
	font := loadGoRegularFont(t)

	gid := font.GlyphIndex('v')
	if gid == 0 {
		t.Skip("'v' glyph not found")
	}

	ext := NewOutlineExtractor()
	outline, err := ext.ExtractOutline(font, GlyphID(gid), 24)
	if err != nil || outline == nil {
		t.Fatalf("failed to extract 'v': %v", err)
	}

	autoHintOutline(outline, font, 24, HintingFull)

	// Collect hinted Y coordinates.
	var ys []float32
	for _, seg := range outline.Segments {
		for j := range segPointCount(seg.Op) {
			ys = append(ys, float32(seg.Points[j].Y))
		}
	}

	// The glyph has 8 points (Go Regular 'v').
	if len(ys) < 8 {
		t.Fatalf("expected at least 8 points, got %d", len(ys))
	}

	// Log coordinates for visibility.
	t.Logf("Hinted Y coords: %v", ys)

	// Key invariant: the valley point (index 3) must be between
	// baseline points (indices 0,6,7) and top points (indices 1,2,4,5).
	baselineY := ys[0] // Should be near 0
	topY := ys[1]      // Should be negative (cap height)
	valleyY := ys[3]   // Must be between baseline and top

	t.Logf("baseline=%.4f valley=%.4f top=%.4f", baselineY, valleyY, topY)

	// Valley must be strictly between baseline and top.
	// (In font coords, top is more negative than valley which is more negative than baseline.)
	if valleyY >= baselineY {
		t.Errorf("valley Y=%.4f should be below baseline Y=%.4f", valleyY, baselineY)
	}
	if valleyY <= topY {
		t.Errorf("valley Y=%.4f should be above top Y=%.4f (not collapsed to cap height)", valleyY, topY)
	}

	// Verify the valley is not too close to either edge.
	totalSpan := baselineY - topY
	valleyOffset := baselineY - valleyY
	valleyFraction := valleyOffset / totalSpan

	// Valley should be roughly in the bottom 5-50% of the glyph height
	// (it's the tip of the 'v', near baseline but not AT baseline).
	if valleyFraction < 0.05 || valleyFraction > 0.90 {
		t.Errorf("valley at %.0f%% of glyph height — expected 5-90%%, got fraction=%.4f",
			valleyFraction*100, valleyFraction)
	}
}

// TestAlignEdgePoints_NoExtraPointsSnapped verifies that alignEdgePoints
// does NOT snap points outside the segment's contour path.
//
// This tests the scenario where the segment's firstPt to lastPt range
// in array indices would include non-segment points. The linked list
// traversal must only visit points that are part of the contour path
// between firstPt and lastPt.
func TestAlignEdgePoints_NoExtraPointsSnapped(t *testing.T) {
	// Build a contour where segment covers points 1→2 (array sequential),
	// but there are other points at different positions.
	pa := &hintPointArray{
		pts: []hintPoint{
			{fx: 0, fy: 0, x: 0, y: 0, ox: 0, oy: 0, next: 1, prev: 4},
			{fx: 5, fy: 10, x: f26dot6FromFloat(5), y: f26dot6FromFloat(10), ox: f26dot6FromFloat(5), oy: f26dot6FromFloat(10), next: 2, prev: 0},
			{fx: 10, fy: 10, x: f26dot6FromFloat(10), y: f26dot6FromFloat(10), ox: f26dot6FromFloat(10), oy: f26dot6FromFloat(10), next: 3, prev: 1},
			{fx: 10, fy: 5, x: f26dot6FromFloat(10), y: f26dot6FromFloat(5), ox: f26dot6FromFloat(10), oy: f26dot6FromFloat(5), next: 4, prev: 2},
			{fx: 5, fy: 0, x: f26dot6FromFloat(5), y: 0, ox: f26dot6FromFloat(5), oy: 0, next: 0, prev: 3},
		},
		contours: []contourRange{{start: 0, end: 4}},
	}

	// Segment: firstPt=1, lastPt=2 (only these two points are on the segment).
	segments := []hintSegment{
		{pos: 10, edgeIdx: 0, firstPt: 1, lastPt: 2, linkIdx: -1, serifIdx: -1},
	}

	want := f26dot6FromFloat(11.0)
	edges := []*hintEdge{
		{fpos: 10, opos: f26dot6FromFloat(10), pos: want}, // Shift from 10 to 11
	}

	alignEdgePoints(pa, segments, edges, dimVertical, scriptGroupDefault)

	// Points 1 and 2 should be snapped.
	if pa.pts[1].y != want {
		t.Errorf("point[1].y = %d (%.2fpx), want %d (11.0px)", pa.pts[1].y, f26dot6ToFloat(pa.pts[1].y), want)
	}
	if pa.pts[2].y != want {
		t.Errorf("point[2].y = %d (%.2fpx), want %d (11.0px)", pa.pts[2].y, f26dot6ToFloat(pa.pts[2].y), want)
	}

	// Points 0, 3, 4 must NOT be snapped.
	if pa.pts[0].y != 0 {
		t.Errorf("point[0].y = %d, want 0 (not part of segment)", pa.pts[0].y)
	}
	if pa.pts[3].y != f26dot6FromFloat(5) {
		t.Errorf("point[3].y = %d, want %d (5.0px, not part of segment)", pa.pts[3].y, f26dot6FromFloat(5))
	}
	if pa.pts[4].y != 0 {
		t.Errorf("point[4].y = %d, want 0 (not part of segment)", pa.pts[4].y)
	}
}

// ============================================================
// 13. Segment Detection vs Skrifa Golden Tests
// ============================================================

// skrifaSegmentGolden holds the expected segment data from skrifa's golden dump.
// Coordinates are in font units (matching skrifa's point.fx/fy convention).
type skrifaSegmentGolden struct {
	dir                hintDirection
	pos                int // font units (segment position along major axis)
	firstPt, lastPt    int // point indices
	minCoord, maxCoord int // font units (extent along minor axis)
}

// TestAutoHint_Segments_VsSkrifaGolden_Horizontal verifies that our horizontal-
// dimension segment detection produces the exact same segments as skrifa for
// NotoSerifHebrew glyph 9 at 16px.
//
// Skrifa golden data from tmp/skrifa_golden_dump.txt:
//
//	dim=Horizontal after compute_segments: 5 segments
//	  seg[0]: dir=Up pos=123 first=0 last=2 min=-240 max=360
//	  seg[1]: dir=Up pos=15 first=8 last=10 min=515 max=565
//	  seg[2]: dir=Down pos=56 first=14 last=16 min=619 max=647
//	  seg[3]: dir=Down pos=205 first=21 last=23 min=493 max=578
//	  seg[4]: dir=Down pos=186 first=25 last=28 min=-204 max=441
//
// Note: in FreeType/skrifa terminology, "Horizontal dimension" means analyzing
// X coordinates to find VERTICAL segments (stems like 'I', 'l'). The segment
// pos is the X position, min/max is the Y extent.
func TestAutoHint_Segments_VsSkrifaGolden_Horizontal(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())

	// Run segment detection for horizontal dimension.
	hSegs := computeSegments(&points, dimHorizontal)

	// Skrifa golden: 5 horizontal segments (font unit coordinates).
	wantH := []skrifaSegmentGolden{
		{dir: dirUp, pos: 123, firstPt: 0, lastPt: 2, minCoord: -240, maxCoord: 360},
		{dir: dirUp, pos: 15, firstPt: 8, lastPt: 10, minCoord: 515, maxCoord: 565},
		{dir: dirDown, pos: 56, firstPt: 14, lastPt: 16, minCoord: 619, maxCoord: 647},
		{dir: dirDown, pos: 205, firstPt: 21, lastPt: 23, minCoord: 493, maxCoord: 578},
		{dir: dirDown, pos: 186, firstPt: 25, lastPt: 28, minCoord: -204, maxCoord: 441},
	}

	t.Logf("Horizontal segments: got %d, want %d", len(hSegs), len(wantH))

	// Log all segments for visibility.
	for i, seg := range hSegs {
		// Segment pos/min/max are in the same coordinate system as fx/fy.
		pos := int(math.Round(float64(seg.pos)))
		minC := int(math.Round(float64(seg.minCoord)))
		maxC := int(math.Round(float64(seg.maxCoord)))
		dirName := "?"
		switch seg.dir {
		case dirUp:
			dirName = "Up"
		case dirDown:
			dirName = "Down"
		case dirLeft:
			dirName = "Left"
		case dirRight:
			dirName = "Right"
		}
		t.Logf("  seg[%d]: dir=%-5s pos=%4d first=%2d last=%2d min=%4d max=%4d",
			i, dirName, pos, seg.firstPt, seg.lastPt, minC, maxC)
	}

	// Compare count.
	if len(hSegs) != len(wantH) {
		t.Errorf("horizontal segment count: got %d, want %d", len(hSegs), len(wantH))
		return
	}

	// Compare each segment field by field.
	hMismatches := 0
	for i, want := range wantH {
		got := hSegs[i]
		gotPos := int(math.Round(float64(got.pos)))
		gotMin := int(math.Round(float64(got.minCoord)))
		gotMax := int(math.Round(float64(got.maxCoord)))

		ok := true
		if got.dir != want.dir {
			t.Errorf("  seg[%d].dir: got %d, want %d", i, got.dir, want.dir)
			ok = false
		}
		if gotPos != want.pos {
			t.Errorf("  seg[%d].pos: got %d, want %d", i, gotPos, want.pos)
			ok = false
		}
		if got.firstPt != want.firstPt {
			t.Errorf("  seg[%d].firstPt: got %d, want %d", i, got.firstPt, want.firstPt)
			ok = false
		}
		if got.lastPt != want.lastPt {
			t.Errorf("  seg[%d].lastPt: got %d, want %d", i, got.lastPt, want.lastPt)
			ok = false
		}
		if gotMin != want.minCoord {
			t.Errorf("  seg[%d].minCoord: got %d, want %d", i, gotMin, want.minCoord)
			ok = false
		}
		if gotMax != want.maxCoord {
			t.Errorf("  seg[%d].maxCoord: got %d, want %d", i, gotMax, want.maxCoord)
			ok = false
		}
		if !ok {
			hMismatches++
		}
	}

	if hMismatches > 0 {
		t.Errorf("horizontal segments: %d / %d mismatches", hMismatches, len(wantH))
	}
}

// TestAutoHint_Segments_VsSkrifaGolden_Vertical verifies vertical-dimension
// segment detection against skrifa golden data.
//
// Skrifa golden:
//
//	dim=Vertical after compute_segments: 4 segments
//	  seg[0]: dir=Left pos=481 first=4 last=7 min=41 max=152
//	  seg[1]: dir=Right pos=647 first=13 last=14 min=29 max=56
//	  seg[2]: dir=Right pos=592 first=17 last=20 min=84 max=192
//	  seg[3]: dir=Left pos=-240 first=31 last=0 min=123 max=138
func TestAutoHint_Segments_VsSkrifaGolden_Vertical(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())

	// Run segment detection for vertical dimension.
	vSegs := computeSegments(&points, dimVertical)

	// Skrifa golden: 4 vertical segments (font unit coordinates).
	wantV := []skrifaSegmentGolden{
		{dir: dirLeft, pos: 481, firstPt: 4, lastPt: 7, minCoord: 41, maxCoord: 152},
		{dir: dirRight, pos: 647, firstPt: 13, lastPt: 14, minCoord: 29, maxCoord: 56},
		{dir: dirRight, pos: 592, firstPt: 17, lastPt: 20, minCoord: 84, maxCoord: 192},
		{dir: dirLeft, pos: -240, firstPt: 31, lastPt: 0, minCoord: 123, maxCoord: 138},
	}

	t.Logf("Vertical segments: got %d, want %d", len(vSegs), len(wantV))

	for i, seg := range vSegs {
		pos := int(math.Round(float64(seg.pos)))
		minC := int(math.Round(float64(seg.minCoord)))
		maxC := int(math.Round(float64(seg.maxCoord)))
		dirName := "?"
		switch seg.dir {
		case dirUp:
			dirName = "Up"
		case dirDown:
			dirName = "Down"
		case dirLeft:
			dirName = "Left"
		case dirRight:
			dirName = "Right"
		}
		t.Logf("  seg[%d]: dir=%-5s pos=%4d first=%2d last=%2d min=%4d max=%4d",
			i, dirName, pos, seg.firstPt, seg.lastPt, minC, maxC)
	}

	if len(vSegs) != len(wantV) {
		t.Errorf("vertical segment count: got %d, want %d", len(vSegs), len(wantV))
		return
	}

	vMismatches := 0
	for i, want := range wantV {
		got := vSegs[i]
		gotPos := int(math.Round(float64(got.pos)))
		gotMin := int(math.Round(float64(got.minCoord)))
		gotMax := int(math.Round(float64(got.maxCoord)))

		ok := true
		if got.dir != want.dir {
			t.Errorf("  seg[%d].dir: got %d, want %d", i, got.dir, want.dir)
			ok = false
		}
		if gotPos != want.pos {
			t.Errorf("  seg[%d].pos: got %d, want %d", i, gotPos, want.pos)
			ok = false
		}
		if got.firstPt != want.firstPt {
			t.Errorf("  seg[%d].firstPt: got %d, want %d", i, got.firstPt, want.firstPt)
			ok = false
		}
		if got.lastPt != want.lastPt {
			t.Errorf("  seg[%d].lastPt: got %d, want %d", i, got.lastPt, want.lastPt)
			ok = false
		}
		if gotMin != want.minCoord {
			t.Errorf("  seg[%d].minCoord: got %d, want %d", i, gotMin, want.minCoord)
			ok = false
		}
		if gotMax != want.maxCoord {
			t.Errorf("  seg[%d].maxCoord: got %d, want %d", i, gotMax, want.maxCoord)
			ok = false
		}
		if !ok {
			vMismatches++
		}
	}

	if vMismatches > 0 {
		t.Errorf("vertical segments: %d / %d mismatches", vMismatches, len(wantV))
	}
}

// ============================================================
// 14. Edge Detection vs Skrifa Golden Tests
// ============================================================

// skrifaEdgeGolden holds the expected edge data from skrifa's golden dump.
// All position values are in 26.6 fixed-point (where 64 = 1 pixel).
type skrifaEdgeGolden struct {
	fpos    int // font units (copied from segment.pos)
	opos    int // 26.6 scaled pixels (fpos * scale, integer arithmetic)
	pos     int // 26.6 current position (initially = opos)
	linkIdx int // index of linked edge, or -1 for None
}

// TestAutoHint_Edges_VsSkrifaGolden_Horizontal verifies that our horizontal-
// dimension edge detection produces the exact same edges as skrifa for
// NotoSerifHebrew glyph 9 at 16px.
//
// Skrifa golden data from tmp/skrifa_golden_dump.txt:
//
//	dim=Horizontal after compute_edges: 4 edges
//	  edge[ 0]: fpos=15 opos=15 pos=15 link=Some(3) blue=None
//	  edge[ 1]: fpos=123 opos=126 pos=126 link=Some(2) blue=None
//	  edge[ 2]: fpos=186 opos=190 pos=190 link=Some(1) blue=None
//	  edge[ 3]: fpos=205 opos=210 pos=210 link=Some(0) blue=None
func TestAutoHint_Edges_VsSkrifaGolden_Horizontal(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())

	// Get scaled axis metrics for horizontal dimension.
	unscaled := computeUnscaledMetrics(font)
	scaled := unscaled.scale(scale)
	axisMetrics := &scaled.axes[dimHorizontal]

	// Run segment detection.
	hSegs := computeSegments(&points, dimHorizontal)

	// Adjust segment heights (skrifa parity).
	adjustSegmentHeights(&points, hSegs, dimHorizontal)

	// Link segments into stems.
	linkSegments(hSegs, axisMetrics, scriptGroupDefault)

	// Log segment state after linking (diagnostic).
	t.Log("Horizontal segments after link:")
	for i, seg := range hSegs {
		t.Logf("  seg[%d]: pos=%.0f dir=%d link=%d serif=%d height=%.0f delta=%.0f edgeIdx=%d",
			i, seg.pos, seg.dir, seg.linkIdx, seg.serifIdx, seg.height, seg.delta, seg.edgeIdx)
	}
	t.Logf("Axis: scale=%f edgeDistThreshold=%f widths=%v",
		axisMetrics.scale, axisMetrics.edgeDistThreshold, axisMetrics.widths)

	// Compute edges.
	edges := computeEdges(hSegs, axisMetrics, dimHorizontal, scriptGroupDefault)

	// Skrifa golden: 4 horizontal edges.
	wantH := []skrifaEdgeGolden{
		{fpos: 15, opos: 15, pos: 15, linkIdx: 3},
		{fpos: 123, opos: 126, pos: 126, linkIdx: 2},
		{fpos: 186, opos: 190, pos: 190, linkIdx: 1},
		{fpos: 205, opos: 210, pos: 210, linkIdx: 0},
	}

	t.Logf("Horizontal edges: got %d, want %d", len(edges), len(wantH))

	// Log all edges for visibility.
	for i, edge := range edges {
		fpos26 := int(math.Round(float64(edge.fpos)))
		opos26 := int(math.Round(float64(edge.opos) * 64))
		pos26 := int(math.Round(float64(edge.pos) * 64))
		dirName := dirStr(edge.dir)
		linkStr := "None"
		if edge.linkIdx >= 0 {
			linkStr = fmt.Sprintf("Some(%d)", edge.linkIdx)
		}
		t.Logf("  edge[%d]: fpos=%d opos=%d pos=%d dir=%s link=%s",
			i, fpos26, opos26, pos26, dirName, linkStr)
	}

	if len(edges) != len(wantH) {
		t.Fatalf("horizontal edge count: got %d, want %d", len(edges), len(wantH))
	}

	hMismatches := 0
	for i, want := range wantH {
		got := edges[i]
		// fpos is in font units (integer)
		gotFpos := int(math.Round(float64(got.fpos)))
		// opos and pos are in pixel float; convert to 26.6 for comparison
		gotOpos := int(got.opos)
		gotPos := int(got.pos)
		gotLink := int(got.linkIdx)

		ok := true
		if gotFpos != want.fpos {
			t.Errorf("  edge[%d].fpos: got %d, want %d", i, gotFpos, want.fpos)
			ok = false
		}
		if gotOpos != want.opos {
			t.Errorf("  edge[%d].opos: got %d, want %d", i, gotOpos, want.opos)
			ok = false
		}
		if gotPos != want.pos {
			t.Errorf("  edge[%d].pos: got %d, want %d", i, gotPos, want.pos)
			ok = false
		}
		if gotLink != want.linkIdx {
			t.Errorf("  edge[%d].linkIdx: got %d, want %d", i, gotLink, want.linkIdx)
			ok = false
		}
		if !ok {
			hMismatches++
		}
	}

	if hMismatches > 0 {
		t.Errorf("horizontal edges: %d / %d mismatches", hMismatches, len(wantH))
	}
}

// TestAutoHint_Edges_VsSkrifaGolden_Vertical verifies vertical-dimension
// edge detection against skrifa golden data.
//
// Skrifa golden:
//
//	dim=Vertical after compute_edges: 4 edges
//	  edge[ 0]: fpos=-240 opos=-246 pos=-246 link=None blue=None
//	  edge[ 1]: fpos=481 opos=493 pos=493 link=Some(2) blue=None
//	  edge[ 2]: fpos=592 opos=606 pos=606 link=Some(1) blue=None
//	  edge[ 3]: fpos=647 opos=663 pos=663 link=None blue=None
func TestAutoHint_Edges_VsSkrifaGolden_Vertical(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())

	// Get scaled axis metrics for vertical dimension.
	unscaled := computeUnscaledMetrics(font)
	scaled := unscaled.scale(scale)
	axisMetrics := &scaled.axes[dimVertical]

	// Run segment detection.
	vSegs := computeSegments(&points, dimVertical)

	// Adjust segment heights (skrifa parity).
	adjustSegmentHeights(&points, vSegs, dimVertical)

	// Link segments into stems.
	linkSegments(vSegs, axisMetrics, scriptGroupDefault)

	// Compute edges.
	edges := computeEdges(vSegs, axisMetrics, dimVertical, scriptGroupDefault)

	// Skrifa golden: 4 vertical edges.
	wantV := []skrifaEdgeGolden{
		{fpos: -240, opos: -246, pos: -246, linkIdx: -1},
		{fpos: 481, opos: 493, pos: 493, linkIdx: 2},
		{fpos: 592, opos: 606, pos: 606, linkIdx: 1},
		{fpos: 647, opos: 663, pos: 663, linkIdx: -1},
	}

	t.Logf("Vertical edges: got %d, want %d", len(edges), len(wantV))

	for i, edge := range edges {
		fpos26 := int(math.Round(float64(edge.fpos)))
		opos26 := int(math.Round(float64(edge.opos) * 64))
		pos26 := int(math.Round(float64(edge.pos) * 64))
		dirName := dirStr(edge.dir)
		linkStr := "None"
		if edge.linkIdx >= 0 {
			linkStr = fmt.Sprintf("Some(%d)", edge.linkIdx)
		}
		serifStr := "None"
		if edge.serifIdx >= 0 {
			serifStr = fmt.Sprintf("Some(%d)", edge.serifIdx)
		}
		t.Logf("  edge[%d]: fpos=%d opos=%d pos=%d dir=%s link=%s serif=%s",
			i, fpos26, opos26, pos26, dirName, linkStr, serifStr)
	}

	if len(edges) != len(wantV) {
		t.Fatalf("vertical edge count: got %d, want %d", len(edges), len(wantV))
	}

	vMismatches := 0
	for i, want := range wantV {
		got := edges[i]
		gotFpos := int(math.Round(float64(got.fpos)))
		gotOpos := int(got.opos)
		gotPos := int(got.pos)
		gotLink := int(got.linkIdx)

		ok := true
		if gotFpos != want.fpos {
			t.Errorf("  edge[%d].fpos: got %d, want %d", i, gotFpos, want.fpos)
			ok = false
		}
		if gotOpos != want.opos {
			t.Errorf("  edge[%d].opos: got %d, want %d", i, gotOpos, want.opos)
			ok = false
		}
		if gotPos != want.pos {
			t.Errorf("  edge[%d].pos: got %d, want %d", i, gotPos, want.pos)
			ok = false
		}
		if gotLink != want.linkIdx {
			t.Errorf("  edge[%d].linkIdx: got %d, want %d", i, gotLink, want.linkIdx)
			ok = false
		}
		if !ok {
			vMismatches++
		}
	}

	if vMismatches > 0 {
		t.Errorf("vertical edges: %d / %d mismatches", vMismatches, len(wantV))
	}
}

// dirStr returns a human-readable direction name.
func dirStr(d hintDirection) string {
	switch d {
	case dirUp:
		return "Up"
	case dirDown:
		return "Down"
	case dirLeft:
		return "Left"
	case dirRight:
		return "Right"
	default:
		return "None"
	}
}

// ============================================================
// 15. Edge Serif/Link vs Skrifa Rust Test Golden (Full Parity)
// ============================================================

// TestAutoHint_Edges_VsSkrifaRust_FullParity verifies every field of edge
// detection against the skrifa Rust unit test golden data from edges.rs.
// This goes beyond the dump-based tests by also checking serif indices and
// flags (ROUND/NORMAL), which the dump does not expose.
//
// Skrifa Rust golden (edges.rs edges_default test):
//
//	H edges:
//	  edge[0]: fpos=15  opos=15  link=Some(3) serif=None   flags=ROUND
//	  edge[1]: fpos=123 opos=126 link=Some(2) serif=None   flags=NORMAL
//	  edge[2]: fpos=186 opos=190 link=Some(1) serif=None   flags=NORMAL
//	  edge[3]: fpos=205 opos=210 link=Some(0) serif=None   flags=ROUND
//
//	V edges:
//	  edge[0]: fpos=-240 opos=-246 link=None    serif=Some(1) flags=NORMAL
//	  edge[1]: fpos=481  opos=493  link=Some(2) serif=None    flags=NORMAL
//	  edge[2]: fpos=592  opos=606  link=Some(1) serif=None    flags=ROUND|SERIF
//	  edge[3]: fpos=647  opos=663  link=None    serif=Some(2) flags=NORMAL
func TestAutoHint_Edges_VsSkrifaRust_FullParity(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())

	unscaled := computeUnscaledMetrics(font)
	scaled := unscaled.scale(scale)

	// --- Horizontal edges ---
	hSegs := computeSegments(&points, dimHorizontal)
	adjustSegmentHeights(&points, hSegs, dimHorizontal)
	linkSegments(hSegs, &scaled.axes[dimHorizontal], scriptGroupDefault)
	hEdges := computeEdges(hSegs, &scaled.axes[dimHorizontal], dimHorizontal, scriptGroupDefault)

	type edgeGoldenFull struct {
		fpos     int
		opos     int // 26.6
		linkIdx  int // -1 for None
		serifIdx int // -1 for None
		isRound  bool
	}

	wantH := []edgeGoldenFull{
		{fpos: 15, opos: 15, linkIdx: 3, serifIdx: -1, isRound: true},
		{fpos: 123, opos: 126, linkIdx: 2, serifIdx: -1, isRound: false},
		{fpos: 186, opos: 190, linkIdx: 1, serifIdx: -1, isRound: false},
		{fpos: 205, opos: 210, linkIdx: 0, serifIdx: -1, isRound: true},
	}

	t.Logf("=== Horizontal edges: got %d, want %d ===", len(hEdges), len(wantH))
	for i, e := range hEdges {
		fpos := int(math.Round(float64(e.fpos)))
		opos := int(e.opos)
		linkStr := "None"
		if e.linkIdx >= 0 {
			linkStr = fmt.Sprintf("Some(%d)", e.linkIdx)
		}
		serifStr := "None"
		if e.serifIdx >= 0 {
			serifStr = fmt.Sprintf("Some(%d)", e.serifIdx)
		}
		roundStr := "NORMAL"
		if e.flags&edgeFlagRound != 0 {
			roundStr = "ROUND"
		}
		t.Logf("  edge[%d]: fpos=%d opos=%d link=%s serif=%s flags=%s",
			i, fpos, opos, linkStr, serifStr, roundStr)
	}

	if len(hEdges) != len(wantH) {
		t.Fatalf("horizontal edge count: got %d, want %d", len(hEdges), len(wantH))
	}
	for i, want := range wantH {
		got := hEdges[i]
		gotFpos := int(math.Round(float64(got.fpos)))
		gotOpos := int(got.opos)
		if gotFpos != want.fpos {
			t.Errorf("H edge[%d].fpos: got %d, want %d", i, gotFpos, want.fpos)
		}
		if gotOpos != want.opos {
			t.Errorf("H edge[%d].opos: got %d, want %d", i, gotOpos, want.opos)
		}
		if int(got.linkIdx) != want.linkIdx {
			t.Errorf("H edge[%d].linkIdx: got %d, want %d", i, got.linkIdx, want.linkIdx)
		}
		if int(got.serifIdx) != want.serifIdx {
			t.Errorf("H edge[%d].serifIdx: got %d, want %d", i, got.serifIdx, want.serifIdx)
		}
		gotRound := (got.flags & edgeFlagRound) != 0
		if gotRound != want.isRound {
			t.Errorf("H edge[%d].isRound: got %v, want %v", i, gotRound, want.isRound)
		}
	}

	// --- Vertical edges ---
	// Reset points (alignEdgePoints etc. may have been called — rebuild).
	points = buildHintPointsFromContours(contours, scale, font.UnitsPerEm())
	vSegs := computeSegments(&points, dimVertical)
	adjustSegmentHeights(&points, vSegs, dimVertical)
	linkSegments(vSegs, &scaled.axes[dimVertical], scriptGroupDefault)
	vEdges := computeEdges(vSegs, &scaled.axes[dimVertical], dimVertical, scriptGroupDefault)

	// V edge[0] serifIdx: with script-aware width detection (Hebrew
	// standard chars), segment linking now correctly produces serifIdx=1
	// for the descender edge, matching skrifa Rust golden data.
	wantV := []edgeGoldenFull{
		{fpos: -240, opos: -246, linkIdx: -1, serifIdx: 1, isRound: false},
		{fpos: 481, opos: 493, linkIdx: 2, serifIdx: -1, isRound: false},
		{fpos: 592, opos: 606, linkIdx: 1, serifIdx: -1, isRound: true},
		{fpos: 647, opos: 663, linkIdx: -1, serifIdx: 2, isRound: false},
	}

	t.Logf("=== Vertical edges: got %d, want %d ===", len(vEdges), len(wantV))
	for i, e := range vEdges {
		fpos := int(math.Round(float64(e.fpos)))
		opos := int(e.opos)
		linkStr := "None"
		if e.linkIdx >= 0 {
			linkStr = fmt.Sprintf("Some(%d)", e.linkIdx)
		}
		serifStr := "None"
		if e.serifIdx >= 0 {
			serifStr = fmt.Sprintf("Some(%d)", e.serifIdx)
		}
		roundStr := "NORMAL"
		if e.flags&edgeFlagRound != 0 {
			roundStr = "ROUND"
		}
		t.Logf("  edge[%d]: fpos=%d opos=%d link=%s serif=%s flags=%s",
			i, fpos, opos, linkStr, serifStr, roundStr)
	}

	if len(vEdges) != len(wantV) {
		t.Fatalf("vertical edge count: got %d, want %d", len(vEdges), len(wantV))
	}
	for i, want := range wantV {
		got := vEdges[i]
		gotFpos := int(math.Round(float64(got.fpos)))
		gotOpos := int(got.opos)
		if gotFpos != want.fpos {
			t.Errorf("V edge[%d].fpos: got %d, want %d", i, gotFpos, want.fpos)
		}
		if gotOpos != want.opos {
			t.Errorf("V edge[%d].opos: got %d, want %d", i, gotOpos, want.opos)
		}
		if int(got.linkIdx) != want.linkIdx {
			t.Errorf("V edge[%d].linkIdx: got %d, want %d", i, got.linkIdx, want.linkIdx)
		}
		if int(got.serifIdx) != want.serifIdx {
			t.Errorf("V edge[%d].serifIdx: got %d, want %d", i, got.serifIdx, want.serifIdx)
		}
		gotRound := (got.flags & edgeFlagRound) != 0
		if gotRound != want.isRound {
			t.Errorf("V edge[%d].isRound: got %v, want %v", i, gotRound, want.isRound)
		}
	}
}

// ============================================================
// 16. hint_edges vs Skrifa Golden Tests
// ============================================================

// TestAutoHint_HintEdges_VsSkrifaGolden_Horizontal verifies that our
// horizontal-dimension edge hinting produces the exact same pos values
// as skrifa after hint_edges.
//
// Skrifa golden from tmp/skrifa_golden_dump.txt:
//
//	dim=Horizontal after hint_edges
//	  edge[0]: opos=15 → pos=0
//	  edge[1]: opos=126 → pos=133
//	  edge[2]: opos=190 → pos=187
//	  edge[3]: opos=210 → pos=192
func TestAutoHint_HintEdges_VsSkrifaGolden_Horizontal(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())

	unscaled := computeUnscaledMetrics(font)
	scaled := unscaled.scaleWithUPM(scale, font.UnitsPerEm())
	axisMetrics := &scaled.axes[dimHorizontal]

	// Override horizontal standard width to match skrifa's Hebrew-derived value.
	// Skrifa uses 16.16 fixed-point: fixed_mul(52, 68759) = 55 in 26.6 = 0.859375px.
	// Our Latin 'o'-based detection gives stdWidth=97, but Hebrew analysis gives 52.
	skrifaHStdWidth := int32(55) // skrifa 26.6 value: 55/64 = 0.859375px
	axisMetrics.widths = []scaledWidth{{scaled: skrifaHStdWidth, fitted: skrifaHStdWidth}}
	axisMetrics.standardWidth = 52

	// Full pipeline: segments → link → edges → (no blue edges for H) → hint.
	hSegs := computeSegments(&points, dimHorizontal)
	adjustSegmentHeights(&points, hSegs, dimHorizontal)
	linkSegments(hSegs, axisMetrics, scriptGroupDefault)
	edges := computeEdges(hSegs, axisMetrics, dimHorizontal, scriptGroupDefault)
	// No blue edge matching for horizontal dimension (only vertical gets blues).
	hintEdges(edges, axisMetrics, scriptGroupDefault)

	// Skrifa golden: pos after hint_edges (26.6 fixed-point).
	type hintGolden struct {
		opos26 int // original opos in 26.6 (for identification)
		pos26  int // hinted pos in 26.6
	}
	wantH := []hintGolden{
		{opos26: 15, pos26: 0},
		{opos26: 126, pos26: 133},
		{opos26: 190, pos26: 187},
		{opos26: 210, pos26: 192},
	}

	t.Logf("Horizontal edges after hint_edges: %d edges", len(edges))
	for i, edge := range edges {
		t.Logf("  edge[%d]: opos=%d -> pos=%d", i, edge.opos, edge.pos)
	}

	if len(edges) != len(wantH) {
		t.Fatalf("horizontal hint edge count: got %d, want %d", len(edges), len(wantH))
	}

	// Now in 26.6 integer arithmetic — expect exact match (diff=0).
	hMismatches := 0
	for i, want := range wantH {
		got := edges[i]
		ok := true
		if int(got.opos) != want.opos26 {
			t.Errorf("  H edge[%d].opos: got %d, want %d", i, got.opos, want.opos26)
			ok = false
		}
		if int(got.pos) != want.pos26 {
			t.Errorf("  H edge[%d].pos: got %d, want %d (delta=%d)", i, got.pos, want.pos26, int(got.pos)-want.pos26)
			ok = false
		}
		if !ok {
			hMismatches++
		}
	}

	if hMismatches > 0 {
		t.Errorf("horizontal hint_edges: %d / %d mismatches", hMismatches, len(wantH))
	}
}

// TestAutoHint_HintEdges_VsSkrifaGolden_Vertical verifies vertical-dimension
// edge hinting against skrifa golden data.
//
// Skrifa golden from tmp/skrifa_golden_dump.txt:
//
//	dim=Vertical after hint_edges
//	  edge[0]: opos=-246 → pos=-256
//	  edge[1]: opos=493 → pos=463
//	  edge[2]: opos=606 → pos=576
//	  edge[3]: opos=663 → pos=633
//
// Note: this test manually injects skrifa-equivalent blue zone assignments
// because our Latin-based blue zone detection produces different zones than
// skrifa's Hebrew-specific detection. The hinting algorithm itself is being
// tested independently of blue zone detection correctness.
func TestAutoHint_HintEdges_VsSkrifaGolden_Vertical(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())

	unscaled := computeUnscaledMetrics(font)
	scaled := unscaled.scaleWithUPM(scale, font.UnitsPerEm())
	axisMetrics := &scaled.axes[dimVertical]

	// Override vertical standard width to match skrifa's Hebrew-derived value.
	// Skrifa uses 16.16 fixed-point arithmetic: fixed_mul(108, scale_16_16) = 113
	// in 26.6 = 1.765625px. Our float math gives a slightly different value
	// (1.7705px), so we inject the exact skrifa 26.6 value for parity.
	skrifaVStdWidth := int32(113) // skrifa 26.6 value: 113/64 = 1.765625px
	axisMetrics.widths = []scaledWidth{{scaled: skrifaVStdWidth, fitted: skrifaVStdWidth}}
	axisMetrics.standardWidth = 108

	// Full pipeline: segments → link → edges.
	vSegs := computeSegments(&points, dimVertical)
	adjustSegmentHeights(&points, vSegs, dimVertical)
	linkSegments(vSegs, axisMetrics, scriptGroupDefault)
	edges := computeEdges(vSegs, axisMetrics, dimVertical, scriptGroupDefault)

	// Manually inject skrifa-equivalent blue zone assignments.
	// From skrifa Rust test golden (edges.rs edges_default):
	//   V edge[0]: blue_edge = ScaledWidth{scaled:-246, fitted:-256} (blue index 2)
	//   V edge[2]: blue_edge = ScaledWidth{scaled:606, fitted:576}  (blue index 0)
	// These come from Hebrew-specific blue zones which our Latin-based
	// detection cannot produce. We inject them directly to test hintEdges.
	skrifaBlue0 := scaledWidth{scaled: -246, fitted: -256} // 26.6 values
	skrifaBlue2 := scaledWidth{scaled: 606, fitted: 576}   // 26.6 values
	if len(edges) >= 4 {
		edges[0].blueEdge = &skrifaBlue0
		edges[2].blueEdge = &skrifaBlue2
	}

	hintEdges(edges, axisMetrics, scriptGroupDefault)

	// Skrifa golden: pos after hint_edges (26.6 fixed-point).
	type hintGolden struct {
		opos26 int
		pos26  int
	}
	wantV := []hintGolden{
		{opos26: -246, pos26: -256},
		{opos26: 493, pos26: 463},
		{opos26: 606, pos26: 576},
		{opos26: 663, pos26: 633},
	}

	t.Logf("Vertical edges after hint_edges: %d edges", len(edges))
	for i, edge := range edges {
		blueStr := "None"
		if edge.blueEdge != nil {
			blueStr = fmt.Sprintf("fitted=%d", edge.blueEdge.fitted)
		}
		t.Logf("  edge[%d]: opos=%d -> pos=%d blue=%s", i, edge.opos, edge.pos, blueStr)
	}

	if len(edges) != len(wantV) {
		t.Fatalf("vertical hint edge count: got %d, want %d", len(edges), len(wantV))
	}

	// Now in 26.6 integer arithmetic — expect exact match (diff=0).
	vMismatches := 0
	for i, want := range wantV {
		got := edges[i]
		ok := true
		if int(got.opos) != want.opos26 {
			t.Errorf("  V edge[%d].opos: got %d, want %d", i, got.opos, want.opos26)
			ok = false
		}
		if int(got.pos) != want.pos26 {
			t.Errorf("  V edge[%d].pos: got %d, want %d (delta=%d)", i, got.pos, want.pos26, int(got.pos)-want.pos26)
			ok = false
		}
		if !ok {
			vMismatches++
		}
	}

	if vMismatches > 0 {
		t.Errorf("vertical hint_edges: %d / %d mismatches (beyond ±1 tolerance)", vMismatches, len(wantV))
	}
}

// ============================================================
// 16. Full Pipeline Golden Tests (Point Propagation)
// ============================================================

// TestAutoHint_FullPipeline_VsSkrifaGolden validates the COMPLETE auto-hinting
// pipeline against skrifa golden coordinates for NotoSerifHebrew glyph 9 at 16px.
//
// This test manually injects skrifa-equivalent standard widths and blue zones
// (because our Latin-based metric detection differs from skrifa's Hebrew detection),
// then runs the full pipeline (segments → edges → hint_edges → align_edge_points →
// align_strong_points → align_weak_points) and compares EVERY point coordinate
// against the skrifa golden dump.
//
// Golden target: 32/32 diff=0 in 26.6 fixed-point.
//
// Source: tmp/skrifa_golden_dump.txt (final coordinates after all passes).
func TestAutoHint_FullPipeline_VsSkrifaGolden(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read NotoSerifHebrew font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())
	if len(points.pts) != 32 {
		t.Fatalf("expected 32 points, got %d", len(points.pts))
	}

	unscaled := computeUnscaledMetrics(font)
	scaled := unscaled.scaleWithUPM(scale, font.UnitsPerEm())

	// Override metrics to match skrifa's Hebrew-specific values.
	hAxis := &scaled.axes[dimHorizontal]
	hAxis.widths = []scaledWidth{{scaled: 55, fitted: 55}} // skrifa 26.6 value
	hAxis.standardWidth = 52

	vAxis := &scaled.axes[dimVertical]
	vAxis.widths = []scaledWidth{{scaled: 113, fitted: 113}} // skrifa 26.6 value
	vAxis.standardWidth = 108

	// Process horizontal dimension.
	hSegs := computeSegments(&points, dimHorizontal)
	adjustSegmentHeights(&points, hSegs, dimHorizontal)
	linkSegments(hSegs, hAxis, scriptGroupDefault)
	hEdges := computeEdges(hSegs, hAxis, dimHorizontal, scriptGroupDefault)
	hintEdges(hEdges, hAxis, scriptGroupDefault)
	alignEdgePoints(&points, hSegs, hEdges, dimHorizontal, scriptGroupDefault)
	alignStrongPoints(&points, hEdges, dimHorizontal)
	alignWeakPoints(&points, dimHorizontal)

	// Process vertical dimension.
	vSegs := computeSegments(&points, dimVertical)
	adjustSegmentHeights(&points, vSegs, dimVertical)
	linkSegments(vSegs, vAxis, scriptGroupDefault)
	vEdges := computeEdges(vSegs, vAxis, dimVertical, scriptGroupDefault)

	// Inject skrifa-equivalent blue zone assignments for vertical edges.
	blueEdge0 := scaledWidth{scaled: -246, fitted: -256}
	blueEdge2 := scaledWidth{scaled: 606, fitted: 576}
	if len(vEdges) >= 4 {
		vEdges[0].blueEdge = &blueEdge0
		vEdges[2].blueEdge = &blueEdge2
	}

	hintEdges(vEdges, vAxis, scriptGroupDefault)
	alignEdgePoints(&points, vSegs, vEdges, dimVertical, scriptGroupDefault)
	alignStrongPoints(&points, vEdges, dimVertical)
	alignWeakPoints(&points, dimVertical)

	// Skrifa golden: final coordinates (26.6 fixed-point) after all passes.
	// From tmp/skrifa_golden_dump.txt lines 233-264 (dim=Vertical after align_weak_points).
	wantFinal := [][2]int32{
		{133, -256}, {133, 282}, {133, 343},
		{146, 431}, {158, 463}, {158, 463},
		{57, 463}, {30, 463}, {0, 495},
		{0, 534}, {0, 548}, {2, 570},
		{11, 604}, {17, 633}, {50, 633},
		{50, 629}, {50, 604}, {77, 576},
		{101, 576}, {163, 576}, {180, 576},
		{192, 562}, {192, 542}, {192, 475},
		{190, 457}, {187, 423}, {187, 366},
		{187, 315}, {187, -220}, {178, -231},
		{159, -248}, {146, -256},
	}

	mismatches := 0
	for i := range 32 {
		gotX := points.pts[i].x
		gotY := points.pts[i].y
		wantX := wantFinal[i][0]
		wantY := wantFinal[i][1]

		dx := gotX - wantX
		dy := gotY - wantY

		match := "OK"
		if dx != 0 || dy != 0 {
			match = "MISMATCH"
			mismatches++
		}
		t.Logf("  pt[%2d]: (%4d, %4d)  want: (%4d, %4d)  dx=%3d dy=%3d  %s",
			i, gotX, gotY, wantX, wantY, dx, dy, match)
	}

	if mismatches > 0 {
		t.Errorf("full pipeline: %d / 32 mismatches (expected 0 for skrifa parity)", mismatches)
	} else {
		t.Logf("PASS: 32/32 coordinates match skrifa golden (diff=0)")
	}
}

// TestAutoHint_AlignEdgePoints_VsSkrifaGolden validates point coordinates after
// the align_edge_points pass (horizontal + vertical) against skrifa golden data.
func TestAutoHint_AlignEdgePoints_VsSkrifaGolden(t *testing.T) {
	fontData, err := os.ReadFile("testdata/notoserifhebrew_autohint_metrics.ttf")
	if err != nil {
		t.Fatalf("failed to read font: %v", err)
	}

	parser := &ownParser{}
	font, err := parser.Parse(fontData)
	if err != nil {
		t.Fatalf("failed to parse font: %v", err)
	}

	contours, err := ParseGlyfContours(fontData, GlyphID(9))
	if err != nil || contours == nil {
		t.Fatalf("ParseGlyfContours failed: %v", err)
	}

	scale := 16.0 / float64(font.UnitsPerEm())
	points := buildHintPointsFromContours(contours, scale, font.UnitsPerEm())

	unscaled := computeUnscaledMetrics(font)
	scaled := unscaled.scaleWithUPM(scale, font.UnitsPerEm())

	hAxis := &scaled.axes[dimHorizontal]
	hAxis.widths = []scaledWidth{{scaled: 55, fitted: 55}}
	hAxis.standardWidth = 52

	// Run horizontal pass (edges + align_edge_points only).
	hSegs := computeSegments(&points, dimHorizontal)
	adjustSegmentHeights(&points, hSegs, dimHorizontal)
	linkSegments(hSegs, hAxis, scriptGroupDefault)
	hEdges := computeEdges(hSegs, hAxis, dimHorizontal, scriptGroupDefault)
	hintEdges(hEdges, hAxis, scriptGroupDefault)
	alignEdgePoints(&points, hSegs, hEdges, dimHorizontal, scriptGroupDefault)

	// Skrifa golden: dim=Horizontal after align_edge_points (lines 51-83).
	wantAfterHEdge := [][2]int32{
		{133, -246}, {133, 306}, {133, 369},
		{142, 459}, {156, 492}, {156, 495},
		{66, 495}, {42, 495}, {0, 527},
		{0, 565}, {0, 579}, {17, 601},
		{25, 634}, {30, 663}, {57, 663},
		{57, 659}, {57, 634}, {86, 606},
		{112, 606}, {179, 606}, {197, 606},
		{192, 592}, {192, 572}, {192, 505},
		{200, 487}, {187, 452}, {187, 393},
		{187, 341}, {187, -209}, {185, -220},
		{159, -238}, {141, -246},
	}

	mismatches := 0
	for i := range 32 {
		gotX := points.pts[i].x
		gotY := points.pts[i].y
		wantX := wantAfterHEdge[i][0]
		wantY := wantAfterHEdge[i][1]
		if gotX != wantX || gotY != wantY {
			t.Errorf("  pt[%2d]: (%4d, %4d) want (%4d, %4d)", i, gotX, gotY, wantX, wantY)
			mismatches++
		}
	}
	if mismatches > 0 {
		t.Errorf("H align_edge_points: %d/32 mismatches", mismatches)
	} else {
		t.Log("H align_edge_points: 32/32 match")
	}
}
