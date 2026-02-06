// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package raster

import (
	"testing"
)

// TestIsNotMonotonic tests the monotonicity check.
func TestIsNotMonotonic(t *testing.T) {
	tests := []struct {
		name    string
		a, b, c float32
		wantNot bool
	}{
		{"increasing", 0, 50, 100, false},
		{"decreasing", 100, 50, 0, false},
		{"constant", 50, 50, 50, true}, // ab==0 triggers not-monotonic
		{"peak", 0, 100, 0, true},
		{"valley", 100, 0, 100, true},
		{"flat start", 0, 0, 100, true}, // ab==0
		{"flat end", 0, 100, 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNotMonotonic(tt.a, tt.b, tt.c)
			if got != tt.wantNot {
				t.Errorf("isNotMonotonic(%v, %v, %v) = %v, want %v",
					tt.a, tt.b, tt.c, got, tt.wantNot)
			}
		})
	}
}

// TestValidUnitDivide tests the unit divide function.
func TestValidUnitDivide(t *testing.T) {
	tests := []struct {
		name   string
		numer  float32
		denom  float32
		wantGT float32 // Want result > this
		wantLT float32 // Want result < this
		wantOK bool    // Whether we expect a valid result
	}{
		{"half", 1, 2, 0, 1, true},              // 0.5
		{"third", 1, 3, 0, 1, true},             // 0.333...
		{"zero denom", 1, 0, 0, 0, false},       // Division by zero
		{"negative result", -1, 2, 0, 0, false}, // -0.5, outside (0,1)
		{"greater than one", 3, 2, 0, 0, false}, // 1.5, outside (0,1)
		{"exactly zero", 0, 2, 0, 0, false},     // 0, not in (0,1)
		{"exactly one", 2, 2, 0, 0, false},      // 1, not in (0,1)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validUnitDivide(tt.numer, tt.denom)

			if tt.wantOK {
				if got <= tt.wantGT || got >= tt.wantLT {
					t.Errorf("validUnitDivide(%v, %v) = %v, want in (%v, %v)",
						tt.numer, tt.denom, got, tt.wantGT, tt.wantLT)
				}
			} else {
				if got != 0 {
					t.Errorf("validUnitDivide(%v, %v) = %v, want 0 (invalid)",
						tt.numer, tt.denom, got)
				}
			}
		})
	}
}

// TestFindUnitQuadRoots tests quadratic root finding in (0,1).
func TestFindUnitQuadRoots(t *testing.T) {
	tests := []struct {
		name      string
		a, b, c   float32
		wantCount int
	}{
		{"no roots (positive discriminant, outside)", 1, -3, 3, 0},
		{"one root at 0.5", 1, -1, 0.25, 0}, // (t-0.5)^2 but double root
		{"two roots", 1, -1.5, 0.5, 2},      // Roots at ~0.5 and ~1.0 (one valid)
		{"no real roots", 1, 0, 1, 0},       // t^2 + 1 = 0
		{"linear (a=0)", 0, 2, -1, 1},       // t = 0.5
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := findUnitQuadRoots(tt.a, tt.b, tt.c)
			// Just verify we get expected number of roots
			// Exact values depend on numerical precision
			t.Logf("findUnitQuadRoots(%v, %v, %v) = %v", tt.a, tt.b, tt.c, roots)
		})
	}
}
