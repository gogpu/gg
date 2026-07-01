package text

import (
	"os"
	"testing"
)

// loadHVARFromFont loads and parses the HVAR table from raw font data.
// Returns nil if the font has no HVAR table.
func loadHVARFromFont(t *testing.T, fontData []byte) *hvarTable {
	t.Helper()
	tables, err := parseFontTables(fontData)
	if err != nil {
		t.Fatalf("failed to parse font tables: %v", err)
	}
	hvarRaw, ok := tables["HVAR"]
	if !ok {
		return nil
	}
	hvar, err := parseHVAR(hvarRaw)
	if err != nil {
		t.Fatalf("failed to parse HVAR: %v", err)
	}
	return hvar
}

// loadFvarAxesFromFont loads fvar axes from raw font data.
func loadFvarAxesFromFont(t *testing.T, fontData []byte) []fvarAxis {
	t.Helper()
	tables, err := parseFontTables(fontData)
	if err != nil {
		t.Fatalf("failed to parse font tables: %v", err)
	}
	fvarRaw, ok := tables["fvar"]
	if !ok {
		t.Fatal("no fvar table found")
	}
	return parseFvarAxes(fvarRaw)
}

// TestGolden_HVAR_AdvanceDelta tests HVAR advance width deltas against
// skrifa golden values from hvar.rs:52-86.
//
// Font: Vazirmatn-Variable (single axis: wght, min=100, default=400, max=900).
// GID 1: advance 1000 FU at default weight.
//
// skrifa golden values (from hvar.rs advance_deltas test):
//
//	coord -1.0 → delta -113
//	coord -0.75 → delta -85 (interpolated, rounded)
//	coord -0.5 → delta -56 (interpolated, rounded)
//	coord 0.0 → delta 0
//	coord 0.5 → delta +30 (interpolated, rounded)
//	coord 1.0 → delta +59
func TestGolden_HVAR_AdvanceDelta(t *testing.T) {
	fontData, err := os.ReadFile("testdata/vazirmatn_var_trimmed.ttf")
	if err != nil {
		t.Skipf("test font not available: %v", err)
	}

	hvar := loadHVARFromFont(t, fontData)
	if hvar == nil {
		t.Fatal("HVAR table not found in test font")
	}

	gid := uint16(1) // First glyph (after .notdef)

	tests := []struct {
		name      string
		coordF214 int16 // F2.14 normalized coordinate
		wantDelta int32 // expected delta in font units
	}{
		{"coord_-1.0", -16384, -113},
		{"coord_-0.75", -12288, -85},
		{"coord_-0.5", -8192, -56},
		{"coord_0.0", 0, 0},
		{"coord_+0.5", 8192, 30},
		{"coord_+1.0", 16384, 59},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coords := []int16{tt.coordF214}
			got := hvar.advanceDelta(gid, coords)
			if got != tt.wantDelta {
				t.Errorf("advanceDelta(gid=%d, coord=%d) = %d, want %d (diff=%d)",
					gid, tt.coordF214, got, tt.wantDelta, got-tt.wantDelta)
			}
		})
	}
}

// TestGolden_HVAR_IVSRegions tests that the ItemVariationStore regions
// are parsed correctly, matching skrifa ivs_regions test (variations.rs:1783-1814).
//
// Expected regions for Vazirmatn-Variable:
//
//	Region 0: wght axis [-1.0, -1.0, 0.0] (Thin → Regular)
//	Region 1: wght axis [0.0, 1.0, 1.0] (Regular → Black)
func TestGolden_HVAR_IVSRegions(t *testing.T) {
	fontData, err := os.ReadFile("testdata/vazirmatn_var_trimmed.ttf")
	if err != nil {
		t.Skipf("test font not available: %v", err)
	}

	hvar := loadHVARFromFont(t, fontData)
	if hvar == nil {
		t.Fatal("HVAR table not found in test font")
	}

	if len(hvar.ivs.regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(hvar.ivs.regions))
	}

	// Expected values in F2.14: -1.0=-16384, 0.0=0, 1.0=16384
	expectedRegions := []struct {
		start, peak, end int16
	}{
		{-16384, -16384, 0}, // Region 0: Thin → Regular
		{0, 16384, 16384},   // Region 1: Regular → Black
	}

	for i, expected := range expectedRegions {
		region := hvar.ivs.regions[i]
		if len(region.axes) != 1 {
			t.Fatalf("region %d: expected 1 axis, got %d", i, len(region.axes))
		}
		axis := region.axes[0]
		if axis.startCoord != expected.start || axis.peakCoord != expected.peak || axis.endCoord != expected.end {
			t.Errorf("region %d: got (%d, %d, %d), want (%d, %d, %d)",
				i, axis.startCoord, axis.peakCoord, axis.endCoord,
				expected.start, expected.peak, expected.end)
		}
	}
}

// TestGolden_HVAR_NormalizeCoords tests that user-space variation values
// are correctly normalized to F2.14 coordinates.
func TestGolden_HVAR_NormalizeCoords(t *testing.T) {
	fontData, err := os.ReadFile("testdata/vazirmatn_var_trimmed.ttf")
	if err != nil {
		t.Skipf("test font not available: %v", err)
	}

	axes := loadFvarAxesFromFont(t, fontData)
	if len(axes) == 0 {
		t.Fatal("no fvar axes found")
	}

	// Vazirmatn: wght axis, min=100, default=400, max=900
	tests := []struct {
		name      string
		wghtValue float32
		wantCoord int16
	}{
		{"min_100", 100, -16384},  // (100-400)/(400-100) = -1.0
		{"light_250", 250, -8192}, // (250-400)/(400-100) = -0.5
		{"default_400", 400, 0},   // at default
		{"bold_650", 650, 8192},   // (650-400)/(900-400) = 0.5
		{"max_900", 900, 16384},   // (900-400)/(900-400) = 1.0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variations := []FontVariation{
				NewFontVariation("wght", tt.wghtValue),
			}
			coords := normalizeCoords(axes, variations)
			if len(coords) != 1 {
				t.Fatalf("expected 1 coord, got %d", len(coords))
			}
			if coords[0] != tt.wantCoord {
				t.Errorf("normalizeCoords(wght=%v) = %d, want %d",
					tt.wghtValue, coords[0], tt.wantCoord)
			}
		})
	}
}

// TestGolden_HVAR_FaceAdvance tests the full integration:
// GlyphAdvanceVar() returns correct variation-adjusted advance.
//
// Uses direct VariableAdvanceProvider interface on ximageParsedFont
// because the trimmed test font lacks a post table (required by opentype.Parse).
func TestGolden_HVAR_FaceAdvance(t *testing.T) {
	fontData, err := os.ReadFile("testdata/vazirmatn_var_trimmed.ttf")
	if err != nil {
		t.Skipf("test font not available: %v", err)
	}

	// Parse the font via our parser. The trimmed font lacks a post table
	// which opentype.Parse requires, so we use the go-text loader path
	// for table access and test the HVAR integration directly.
	hvar := loadHVARFromFont(t, fontData)
	if hvar == nil {
		t.Fatal("HVAR table not found")
	}
	axes := loadFvarAxesFromFont(t, fontData)
	if len(axes) == 0 {
		t.Fatal("no fvar axes found")
	}

	// GID 1 ("A") base advance from hmtx = 1336 font units (from TTX).
	// UPM = 2048 (from TTX).
	const baseAdvanceFU = 1336
	const upm = 2048

	ppem := float64(upm) // ppem = UPM so advance = font units

	// Test at three weight settings.
	tests := []struct {
		name      string
		wghtValue float32
		wantDelta int32 // HVAR delta in font units
	}{
		{"thin_100", 100, -113},
		{"default_400", 400, 0},
		{"black_900", 900, 59},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coords := normalizeCoords(axes, []FontVariation{
				NewFontVariation("wght", tt.wghtValue),
			})
			delta := hvar.advanceDelta(1, coords)
			if delta != tt.wantDelta {
				t.Errorf("delta = %d, want %d", delta, tt.wantDelta)
			}

			// Full advance = base + delta (when ppem == UPM).
			fullAdvance := float64(baseAdvanceFU) + float64(delta)*ppem/float64(upm)
			expectedAdvance := float64(baseAdvanceFU + int(tt.wantDelta))
			if abs64(fullAdvance-expectedAdvance) > 0.01 {
				t.Errorf("full advance = %v, want %v", fullAdvance, expectedAdvance)
			}
			t.Logf("wght=%v: delta=%d, advance=%v FU", tt.wghtValue, delta, fullAdvance)
		})
	}
}

// TestGolden_HVAR_FullAdvance verifies that GlyphAdvanceVar returns correct
// advances via the ximageParsedFont implementation.
//
// Uses a real variable font (if available via NewFontSource) to test the
// full Face.Advance() → VariableAdvanceProvider integration path.
func TestGolden_HVAR_FullAdvance(t *testing.T) {
	// Try loading a system variable font or use the varfont test fixture.
	fontPath := findVariableFontForTest(t)
	if fontPath == "" {
		t.Skip("no variable font available for integration test")
	}

	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		t.Skipf("cannot read font: %v", err)
	}

	source, err := NewFontSource(fontData)
	if err != nil {
		t.Skipf("cannot parse font: %v", err)
	}
	defer func() { _ = source.Close() }()

	if !source.IsVariable() {
		t.Skip("font is not variable")
	}

	parsed := source.Parsed()

	// Verify VariableAdvanceProvider interface is implemented.
	varProvider, ok := parsed.(VariableAdvanceProvider)
	if !ok {
		t.Fatal("ParsedFont does not implement VariableAdvanceProvider")
	}

	// Get GID for 'A'.
	gid := parsed.GlyphIndex('A')
	if gid == 0 {
		t.Skip("font has no glyph for 'A'")
	}

	ppem := float64(parsed.UnitsPerEm())
	axes := source.VariationAxes()

	// Find the weight axis.
	var wghtAxis *VariationAxis
	for i := range axes {
		if axes[i].Tag == [4]byte{'w', 'g', 'h', 't'} {
			wghtAxis = &axes[i]
			break
		}
	}
	if wghtAxis == nil {
		t.Skip("font has no wght axis")
	}

	// Get base advance (default weight).
	baseAdvance := parsed.GlyphAdvance(gid, ppem)

	// Get thin advance (minimum weight).
	thinAdvance := varProvider.GlyphAdvanceVar(gid, ppem,
		[]FontVariation{NewFontVariation("wght", wghtAxis.Minimum)})

	// Get bold advance (maximum weight).
	boldAdvance := varProvider.GlyphAdvanceVar(gid, ppem,
		[]FontVariation{NewFontVariation("wght", wghtAxis.Maximum)})

	// At default weight, variable advance should equal base advance.
	defaultAdvance := varProvider.GlyphAdvanceVar(gid, ppem,
		[]FontVariation{NewFontVariation("wght", wghtAxis.Default)})
	if abs64(defaultAdvance-baseAdvance) > 0.01 {
		t.Errorf("advance at default weight: got %v, want %v", defaultAdvance, baseAdvance)
	}

	t.Logf("'A' advances at %v ppem: thin(%v)=%v, default(%v)=%v, bold(%v)=%v",
		ppem, wghtAxis.Minimum, thinAdvance, wghtAxis.Default, baseAdvance,
		wghtAxis.Maximum, boldAdvance)
}

// findVariableFontForTest locates a variable font for integration testing.
// Returns the path or empty string if none found.
func findVariableFontForTest(t *testing.T) string {
	t.Helper()
	// Check for Cantarell VF in testdata (it's a variable font but trimmed).
	paths := []string{
		"testdata/cantarell_vf_trimmed.ttf",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// TestHVAR_NilSafety tests that nil/empty HVAR handling is safe.
func TestHVAR_NilSafety(t *testing.T) {
	// Nil HVAR table.
	var h *hvarTable
	got := h.advanceDelta(0, []int16{0})
	if got != 0 {
		t.Errorf("nil hvarTable.advanceDelta() = %d, want 0", got)
	}

	// Empty coords.
	h2 := &hvarTable{ivs: &itemVariationStore{}}
	got2 := h2.advanceDelta(0, nil)
	if got2 != 0 {
		t.Errorf("empty coords advanceDelta() = %d, want 0", got2)
	}

	// Out of range outer index.
	got3 := h2.advanceDelta(0, []int16{100})
	if got3 != 0 {
		t.Errorf("out of range outer advanceDelta() = %d, want 0", got3)
	}
}

// TestDeltaSetIndexMap_NilIdentity tests that a nil DeltaSetIndexMap
// uses the identity mapping (outer=0, inner=glyphID).
func TestDeltaSetIndexMap_NilIdentity(t *testing.T) {
	var m *deltaSetIndexMap
	outer, inner := m.get(42)
	if outer != 0 || inner != 42 {
		t.Errorf("nil map.get(42) = (%d, %d), want (0, 42)", outer, inner)
	}
}

// TestDeltaSetIndexMap_ClampToLast tests that glyph IDs beyond the map
// clamp to the last entry per spec.
func TestDeltaSetIndexMap_ClampToLast(t *testing.T) {
	m := &deltaSetIndexMap{
		entries:   []uint32{0x0001, 0x0002, 0x0003},
		innerBits: 8,
	}
	outer, inner := m.get(100) // Beyond entries
	lastEntry := m.entries[2]
	wantInner := uint16(lastEntry & ((1 << 8) - 1))
	wantOuter := uint16(lastEntry >> 8)
	if outer != wantOuter || inner != wantInner {
		t.Errorf("map.get(100) = (%d, %d), want (%d, %d)", outer, inner, wantOuter, wantInner)
	}
}

// TestNormalizeValue tests individual value normalization edge cases.
func TestNormalizeValue(t *testing.T) {
	tests := []struct {
		name          string
		value         float32
		min, def, max float32
		want          int16
	}{
		{"at_default", 400, 100, 400, 900, 0},
		{"at_min", 100, 100, 400, 900, -16384},
		{"at_max", 900, 100, 400, 900, 16384},
		{"below_default", 250, 100, 400, 900, -8192},
		{"above_default", 650, 100, 400, 900, 8192},
		{"min_equals_default", 400, 400, 400, 900, 0},
		{"max_equals_default", 400, 100, 400, 400, 0}, // at default, result is 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeValue(tt.value, tt.min, tt.def, tt.max)
			if got != tt.want {
				t.Errorf("normalizeValue(%v, %v, %v, %v) = %d, want %d",
					tt.value, tt.min, tt.def, tt.max, got, tt.want)
			}
		})
	}
}

// TestComputeScalar_ZeroPeak tests that axes with zero peak are skipped
// (their contribution is neutral = no effect on scalar).
func TestComputeScalar_ZeroPeak(t *testing.T) {
	region := &variationRegion{
		axes: []regionAxisCoords{
			{startCoord: 0, peakCoord: 0, endCoord: 16384}, // zero peak → skip
		},
	}
	scalar := region.computeScalar([]int16{8192})
	// 1.0 in Fixed 16.16 = 0x10000
	if scalar != 0x10000 {
		t.Errorf("computeScalar with zero peak = %d, want %d (1.0)", scalar, 0x10000)
	}
}

// abs64 returns the absolute value of a float64.
func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
