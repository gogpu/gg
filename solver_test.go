package gg

import (
	"math"
	"sort"
	"testing"
)

func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func verifySolverRoots(t *testing.T, name string, roots, expected []float64, epsilon float64) {
	t.Helper()

	if len(roots) != len(expected) {
		t.Errorf("%s: got %d roots, want %d. roots=%v, expected=%v",
			name, len(roots), len(expected), roots, expected)
		return
	}

	// Sort both for comparison
	sortedRoots := make([]float64, len(roots))
	copy(sortedRoots, roots)
	sort.Float64s(sortedRoots)

	sortedExpected := make([]float64, len(expected))
	copy(sortedExpected, expected)
	sort.Float64s(sortedExpected)

	for i := range sortedRoots {
		if !almostEqual(sortedRoots[i], sortedExpected[i], epsilon) {
			t.Errorf("%s: root[%d] = %v, want %v (roots=%v, expected=%v)",
				name, i, sortedRoots[i], sortedExpected[i], sortedRoots, sortedExpected)
		}
	}
}

func TestSolveQuadratic(t *testing.T) {
	tests := []struct {
		name     string
		a, b, c  float64
		expected []float64
		epsilon  float64
	}{
		// ax^2 + bx + c = 0
		// Two distinct roots
		{
			name: "x^2 - 5 = 0 (two roots)",
			a:    1, b: 0, c: -5,
			expected: []float64{-math.Sqrt(5), math.Sqrt(5)},
			epsilon:  1e-10,
		},
		// No real roots
		{
			name: "x^2 + 5 = 0 (no real roots)",
			a:    1, b: 0, c: 5,
			expected: nil,
			epsilon:  1e-10,
		},
		// Linear equation (a=0)
		{
			name: "x + 5 = 0 (linear)",
			a:    0, b: 1, c: 5,
			expected: []float64{-5},
			epsilon:  1e-10,
		},
		// One double root
		{
			name: "x^2 + 2x + 1 = 0 (double root at -1)",
			a:    1, b: 2, c: 1,
			expected: []float64{-1},
			epsilon:  1e-10,
		},
		// General case
		{
			name: "x^2 - 5x + 6 = 0 (roots at 2 and 3)",
			a:    1, b: -5, c: 6,
			expected: []float64{2, 3},
			epsilon:  1e-10,
		},
		// Scaled coefficients
		{
			name: "2x^2 - 10x + 12 = 0 (roots at 2 and 3)",
			a:    2, b: -10, c: 12,
			expected: []float64{2, 3},
			epsilon:  1e-10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := SolveQuadratic(tt.a, tt.b, tt.c)
			verifySolverRoots(t, tt.name, roots, tt.expected, tt.epsilon)

			// Verify roots by substitution
			for _, r := range roots {
				val := tt.a*r*r + tt.b*r + tt.c
				if math.Abs(val) > 1e-8 {
					t.Errorf("%s: root %v gives f(x) = %v, want 0", tt.name, r, val)
				}
			}
		})
	}
}

func TestSolveCubic(t *testing.T) {
	tests := []struct {
		name        string
		a, b, c, d  float64
		expected    []float64
		epsilon     float64
		skipSubstit bool // skip substitution check for edge cases
	}{
		// ax^3 + bx^2 + cx + d = 0
		// One real root
		{
			name: "x^3 - 5 = 0 (one real root)",
			a:    1, b: 0, c: 0, d: -5,
			expected: []float64{math.Cbrt(5)},
			epsilon:  1e-10,
		},
		// Three real roots
		{
			name: "x^3 - x = 0 (three roots: -1, 0, 1)",
			a:    1, b: 0, c: -1, d: 0,
			expected: []float64{-1, 0, 1},
			epsilon:  1e-10,
		},
		// Double root
		{
			name: "x^3 - 3x + 2 = 0 (roots: -2, 1 double)",
			a:    1, b: 0, c: -3, d: 2,
			expected: []float64{-2, 1},
			epsilon:  1e-10,
		},
		// General case with three distinct roots
		{
			name: "(x-1)(x-2)(x-3) = x^3 - 6x^2 + 11x - 6",
			a:    1, b: -6, c: 11, d: -6,
			expected: []float64{1, 2, 3},
			epsilon:  1e-8,
		},
		// Degenerate to quadratic (a=0)
		{
			name: "x^2 - 5x + 6 = 0 (degenerate, a=0)",
			a:    0, b: 1, c: -5, d: 6,
			expected: []float64{2, 3},
			epsilon:  1e-10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := SolveCubic(tt.a, tt.b, tt.c, tt.d)
			verifySolverRoots(t, tt.name, roots, tt.expected, tt.epsilon)

			// Verify roots by substitution
			if !tt.skipSubstit {
				for _, r := range roots {
					val := tt.a*r*r*r + tt.b*r*r + tt.c*r + tt.d
					if math.Abs(val) > 1e-6 {
						t.Errorf("%s: root %v gives f(x) = %v, want 0", tt.name, r, val)
					}
				}
			}
		})
	}
}

func TestSolveQuadraticInUnitInterval(t *testing.T) {
	tests := []struct {
		name     string
		a, b, c  float64
		expected []float64
		epsilon  float64
	}{
		{
			name: "roots outside [0,1]",
			a:    1, b: 0, c: -100,
			expected: nil,
			epsilon:  1e-10,
		},
		{
			name: "roots at boundaries",
			a:    1, b: -1, c: 0, // x(x-1) = 0
			expected: []float64{0, 1},
			epsilon:  1e-10,
		},
		{
			name: "one root inside",
			a:    1, b: -0.5, c: 0, // x(x-0.5) = 0
			expected: []float64{0, 0.5},
			epsilon:  1e-10,
		},
		{
			name: "both roots inside",
			a:    1, b: -0.6, c: 0.08, // roots at ~0.2 and ~0.4
			expected: []float64{0.2, 0.4},
			epsilon:  1e-10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := SolveQuadraticInUnitInterval(tt.a, tt.b, tt.c)

			if len(roots) != len(tt.expected) {
				t.Errorf("%s: got %d roots, want %d", tt.name, len(roots), len(tt.expected))
				return
			}

			for _, r := range roots {
				if r < 0 || r > 1 {
					t.Errorf("%s: root %v outside [0,1]", tt.name, r)
				}
			}
		})
	}
}

func TestSolveCubicInUnitInterval(t *testing.T) {
	tests := []struct {
		name        string
		a, b, c, d  float64
		minExpected int
		maxExpected int
	}{
		{
			name: "x^3 - x = 0 (roots at -1, 0, 1)",
			a:    1, b: 0, c: -1, d: 0,
			minExpected: 2, maxExpected: 2, // Only 0 and 1 are in [0,1]
		},
		{
			name: "roots outside [0,1]",
			a:    1, b: 0, c: 0, d: -8, // x^3 = 8, root at 2
			minExpected: 0, maxExpected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := SolveCubicInUnitInterval(tt.a, tt.b, tt.c, tt.d)

			if len(roots) < tt.minExpected || len(roots) > tt.maxExpected {
				t.Errorf("%s: got %d roots, want between %d and %d",
					tt.name, len(roots), tt.minExpected, tt.maxExpected)
			}

			for _, r := range roots {
				if r < 0 || r > 1 {
					t.Errorf("%s: root %v outside [0,1]", tt.name, r)
				}
			}
		})
	}
}

func TestSolveQuadratic_EdgeCases(t *testing.T) {
	// Test degenerate cases

	// All zero
	roots := SolveQuadratic(0, 0, 0)
	if len(roots) != 1 || roots[0] != 0 {
		t.Errorf("Degenerate case (all zero): got %v, want [0]", roots)
	}

	// Very small discriminant (nearly double root)
	roots = SolveQuadratic(1, -2, 1+1e-15)
	// Should have 0 or 1 root near 1
	for _, r := range roots {
		if math.Abs(r-1) > 0.001 {
			t.Errorf("Nearly double root: root %v not close to 1", r)
		}
	}
}

func TestIsFinite(t *testing.T) {
	tests := []struct {
		name   string
		x      float64
		expect bool
	}{
		{"positive", 1.0, true},
		{"negative", -1.0, true},
		{"zero", 0.0, true},
		{"inf", math.Inf(1), false},
		{"neg inf", math.Inf(-1), false},
		{"nan", math.NaN(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFinite(tt.x)
			if result != tt.expect {
				t.Errorf("isFinite(%v) = %v, want %v", tt.x, result, tt.expect)
			}
		})
	}
}
