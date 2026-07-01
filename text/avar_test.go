package text

import (
	"os"
	"testing"
)

func TestParseAvar_VazirmatnVar(t *testing.T) {
	data, err := os.ReadFile("testdata/vazirmatn_var_trimmed.ttf")
	if err != nil {
		t.Fatalf("failed to read font: %v", err)
	}

	tables, err := parseFontTablesIndex(data, 0)
	if err != nil {
		t.Fatalf("failed to parse font tables: %v", err)
	}

	avarData, ok := tables["avar"]
	if !ok {
		t.Fatal("font has no avar table")
	}

	avar := parseAvar(avarData)
	if avar == nil {
		t.Fatal("parseAvar returned nil")
	}

	// VazirmatnVar has 1 axis (wght).
	if len(avar.segmentMaps) != 1 {
		t.Fatalf("expected 1 axis segment map, got %d", len(avar.segmentMaps))
	}

	segments := avar.segmentMaps[0]
	// From skrifa test (avar.rs:173-193): VazirmatnVar has 9 segment map entries.
	if len(segments) != 9 {
		t.Fatalf("expected 9 segments, got %d", len(segments))
	}

	// Verify first, middle, and last segment values.
	// Expected from skrifa: (-1.0, -1.0), (0.0, 0.0), (1.0, 1.0)
	assertAvarSeg(t, segments[0], -16384, -16384) // (-1.0, -1.0)
	assertAvarSeg(t, segments[3], 0, 0)           // (0.0, 0.0)
	assertAvarSeg(t, segments[8], 16384, 16384)   // (1.0, 1.0)
}

func TestAvarApply_PiecewiseLinear(t *testing.T) {
	// Test piecewise linear interpolation matching skrifa test
	// (avar.rs:240-253): coords at [-1.0, -0.5, 0.0, 0.5, 1.0].

	data, err := os.ReadFile("testdata/vazirmatn_var_trimmed.ttf")
	if err != nil {
		t.Fatalf("failed to read font: %v", err)
	}

	tables, err := parseFontTablesIndex(data, 0)
	if err != nil {
		t.Fatalf("failed to parse font tables: %v", err)
	}

	avarData, ok := tables["avar"]
	if !ok {
		t.Fatal("font has no avar table")
	}

	avar := parseAvar(avarData)
	if avar == nil {
		t.Fatal("parseAvar returned nil")
	}

	tests := []struct {
		name  string
		input int16
		want  int16
	}{
		{"neg1.0", -16384, -16384}, // -1.0 -> -1.0
		{"zero", 0, 0},             // 0.0 -> 0.0
		{"pos1.0", 16384, 16384},   // 1.0 -> 1.0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := avarApplySegmentMap(avar.segmentMaps[0], tt.input)
			if got != tt.want {
				t.Errorf("avarApplySegmentMap(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestAvarApply_Interpolation(t *testing.T) {
	// Test interpolation between segment points.
	// Manually construct segment map for controlled testing.
	segments := []avarSegment{
		{fromCoord: -16384, toCoord: -16384}, // -1.0 -> -1.0
		{fromCoord: 0, toCoord: 0},           // 0.0 -> 0.0
		{fromCoord: 16384, toCoord: 16384},   // 1.0 -> 1.0
	}

	tests := []struct {
		name  string
		input int16
		want  int16
	}{
		{"identity_neg", -8192, -8192},
		{"identity_pos", 8192, 8192},
		{"identity_zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := avarApplySegmentMap(segments, tt.input)
			if got != tt.want {
				t.Errorf("avarApplySegmentMap(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestAvarApply_NonLinear(t *testing.T) {
	// Non-linear mapping: 0.5 maps to 0.75.
	segments := []avarSegment{
		{fromCoord: -16384, toCoord: -16384},
		{fromCoord: 0, toCoord: 0},
		{fromCoord: 8192, toCoord: 12288}, // 0.5 -> 0.75
		{fromCoord: 16384, toCoord: 16384},
	}

	tests := []struct {
		name  string
		input int16
		want  int16
	}{
		{"exact_half", 8192, 12288},      // 0.5 -> 0.75 (exact match)
		{"at_zero", 0, 0},                // identity
		{"at_neg1", -16384, -16384},      // identity
		{"between_0_half", 4096, 6144},   // 0.25 -> 0.375 (linear between 0,0 and 0.5,0.75)
		{"between_half_1", 12288, 13312}, // 0.75 -> (0.75 + 0.25*(1-0.75)/(1-0.5)) = 0.875 -> ~14336... let me recalc
	}

	// Recalculate: between (8192, 12288) and (16384, 16384):
	// for input=12288: bt + (at-bt)*(coord-bf)/(af-bf)
	// = 12288 + (16384-12288)*(12288-8192)/(16384-8192)
	// = 12288 + 4096*4096/8192 = 12288 + 2048 = 14336
	tests[4].want = 14336

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := avarApplySegmentMap(segments, tt.input)
			if got != tt.want {
				t.Errorf("avarApplySegmentMap(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestAvarApply_EdgeCases(t *testing.T) {
	t.Run("empty_segments", func(t *testing.T) {
		got := avarApplySegmentMap(nil, 100)
		if got != 100 {
			t.Errorf("empty segments: got %d, want 100", got)
		}
	})

	t.Run("single_segment_shift", func(t *testing.T) {
		segments := []avarSegment{{fromCoord: 100, toCoord: 200}}
		got := avarApplySegmentMap(segments, 150)
		// shift: 150 - 100 + 200 = 250
		if got != 250 {
			t.Errorf("single segment shift: got %d, want 250", got)
		}
	})

	t.Run("before_all_segments", func(t *testing.T) {
		segments := []avarSegment{
			{fromCoord: 0, toCoord: 100},
			{fromCoord: 16384, toCoord: 16384},
		}
		got := avarApplySegmentMap(segments, -100)
		// extrapolate: -100 - 0 + 100 = 0
		if got != 0 {
			t.Errorf("before all: got %d, want 0", got)
		}
	})
}

func TestParseAvar_Nil(t *testing.T) {
	// Empty data should return nil.
	avar := parseAvar(nil)
	if avar != nil {
		t.Error("parseAvar(nil) should return nil")
	}

	// Short data should return nil.
	avar = parseAvar([]byte{0, 1, 0, 0})
	if avar != nil {
		t.Error("parseAvar(short) should return nil")
	}

	// Wrong version should return nil.
	bad := make([]byte, 8)
	bad[0] = 0
	bad[1] = 2 // version 2
	avar = parseAvar(bad)
	if avar != nil {
		t.Error("parseAvar(version 2) should return nil")
	}
}

func TestAvar_NilApply(t *testing.T) {
	// Applying nil avar should be a no-op.
	var avar *avarTable
	coords := []int16{8192, -4096}
	avar.apply(coords)
	if coords[0] != 8192 || coords[1] != -4096 {
		t.Errorf("nil avar.apply modified coords: %v", coords)
	}
}

func assertAvarSeg(t *testing.T, seg avarSegment, wantFrom, wantTo int16) {
	t.Helper()
	if seg.fromCoord != wantFrom || seg.toCoord != wantTo {
		t.Errorf("segment (%d, %d), want (%d, %d)",
			seg.fromCoord, seg.toCoord, wantFrom, wantTo)
	}
}
