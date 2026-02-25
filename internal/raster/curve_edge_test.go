// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"testing"
)

// TestDiffToShift tests the subdivision count calculation.
// The algorithm heuristically determines how many subdivisions (1 << shift)
// are needed to approximate a curve to within sub-pixel accuracy.
func TestDiffToShift(t *testing.T) {
	tests := []struct {
		name    string
		dx, dy  FDot6
		shiftAA int
		wantMin int
		wantMax int
	}{
		{"flat curve", 0, 0, 0, 0, 0},
		{"small deviation", 1, 1, 0, 0, 1},
		{"medium deviation", 64, 64, 0, 2, 4},  // 1 pixel deviation
		{"large deviation", 640, 640, 0, 2, 4}, // 10 pixel deviation (heuristic rounds down)
		{"with AA", 64, 64, 2, 1, 3},           // AA reduces required shift
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffToShift(tt.dx, tt.dy, tt.shiftAA)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("diffToShift(%v, %v, %v) = %v, want [%v, %v]",
					tt.dx, tt.dy, tt.shiftAA, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestCheapDistance tests the distance approximation.
func TestCheapDistance(t *testing.T) {
	tests := []struct {
		name    string
		dx, dy  FDot6
		wantMin FDot6
		wantMax FDot6
	}{
		{"zero", 0, 0, 0, 0},
		{"horizontal", 100, 0, 100, 100},
		{"vertical", 0, 100, 100, 100},
		{"diagonal", 100, 100, 140, 160},                // sqrt(2)*100 ~ 141
		{"3-4-5", 64 * 3, 64 * 4, 64*5 - 64, 64*5 + 64}, // ~ 5
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cheapDistance(tt.dx, tt.dy)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("cheapDistance(%v, %v) = %v, want [%v, %v]",
					tt.dx, tt.dy, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestCubicDeltaFromLine tests the cubic deviation calculation.
func TestCubicDeltaFromLine(t *testing.T) {
	tests := []struct {
		name       string
		a, b, c, d FDot6
		expectZero bool
	}{
		{"flat line", 0, 0, 0, 0, true},
		{"collinear", 0, 100, 200, 300, true}, // All points on a line
		{"curved", 0, 200, 100, 300, false},   // Control points off the chord
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cubicDeltaFromLine(tt.a, tt.b, tt.c, tt.d)
			if tt.expectZero && got != 0 {
				t.Errorf("expected zero delta, got %v", got)
			}
			if !tt.expectZero && got == 0 {
				t.Error("expected non-zero delta, got 0")
			}
		})
	}
}

// BenchmarkDiffToShift benchmarks the subdivision calculation.
func BenchmarkDiffToShift(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diffToShift(FDot6(i%1000), FDot6((i*7)%1000), 2)
	}
}
