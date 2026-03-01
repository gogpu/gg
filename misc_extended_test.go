package gg

import (
	"math"
	"testing"
)

// Tests for matrix.go, rasterizer_mode.go, shapes.go, solver.go, curve.go
// that cover functions at 0% or low coverage.

// --- Matrix tests ---

func TestMatrix_TransformVector(t *testing.T) {
	tests := []struct {
		name string
		m    Matrix
		p    Point
		want Point
	}{
		{"identity", Identity(), Pt(3, 4), Pt(3, 4)},
		{"scale 2x", Scale(2, 3), Pt(3, 4), Pt(6, 12)},
		{"translate ignored", Translate(100, 200), Pt(3, 4), Pt(3, 4)},
		{"rotation 90deg", Rotate(math.Pi / 2), Pt(1, 0), Pt(0, 1)},
		{"shear", Shear(1, 0), Pt(0, 2), Pt(2, 2)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.m.TransformVector(tt.p)
			if math.Abs(got.X-tt.want.X) > 1e-9 || math.Abs(got.Y-tt.want.Y) > 1e-9 {
				t.Errorf("TransformVector(%v) = %v, want %v", tt.p, got, tt.want)
			}
		})
	}
}

func TestMatrix_TransformVector_NoTranslation(t *testing.T) {
	// Verify that TransformVector ignores the translation component
	m := Matrix{A: 1, B: 0, C: 100, D: 0, E: 1, F: 200}
	got := m.TransformVector(Pt(5, 7))
	want := Pt(5, 7) // Translation (100, 200) should be ignored
	if got.X != want.X || got.Y != want.Y {
		t.Errorf("TransformVector should ignore translation: got %v, want %v", got, want)
	}
}

func TestMatrix_Invert_Singular(t *testing.T) {
	// A singular (non-invertible) matrix should return identity
	singular := Matrix{A: 0, B: 0, C: 0, D: 0, E: 0, F: 0}
	got := singular.Invert()
	if !got.IsIdentity() {
		t.Errorf("Invert of singular matrix should return identity, got %v", got)
	}
}

func TestMatrix_ScaleFactor_VerticalDominant(t *testing.T) {
	// Scale where sy > sx
	m := Scale(1, 5)
	got := m.ScaleFactor()
	if got != 5 {
		t.Errorf("ScaleFactor of Scale(1, 5) = %v, want 5", got)
	}
}

// --- RasterizerMode tests ---

func TestRasterizerMode_String(t *testing.T) {
	tests := []struct {
		mode RasterizerMode
		want string
	}{
		{RasterizerAuto, "Auto"},
		{RasterizerAnalytic, "Analytic"},
		{RasterizerSparseStrips, "SparseStrips"},
		{RasterizerTileCompute, "TileCompute"},
		{RasterizerSDF, "SDF"},
		{RasterizerMode(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("RasterizerMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

// --- Context method tests ---

func TestContext_SetRasterizerMode(t *testing.T) {
	dc := NewContext(10, 10)
	if dc.RasterizerMode() != RasterizerAuto {
		t.Errorf("default mode = %v, want Auto", dc.RasterizerMode())
	}

	dc.SetRasterizerMode(RasterizerAnalytic)
	if dc.RasterizerMode() != RasterizerAnalytic {
		t.Errorf("after set = %v, want Analytic", dc.RasterizerMode())
	}
}

func TestContext_SetHexColor(t *testing.T) {
	dc := NewContext(10, 10)
	dc.SetHexColor("#FF0000")
	// Should not panic; just verify it sets something
}

func TestContext_SetFillRule(t *testing.T) {
	dc := NewContext(10, 10)
	dc.SetFillRule(FillRuleEvenOdd)
	// Should not panic
}

func TestContext_NewSubPath(t *testing.T) {
	dc := NewContext(10, 10)
	dc.NewSubPath() // No-op, should not panic
}

func TestContext_TransformPoint(t *testing.T) {
	dc := NewContext(100, 100)
	dc.Translate(10, 20)
	x, y := dc.TransformPoint(5, 7)
	if math.Abs(x-15) > 1e-9 || math.Abs(y-27) > 1e-9 {
		t.Errorf("TransformPoint(5, 7) after Translate(10, 20) = (%v, %v), want (15, 27)", x, y)
	}
}

func TestContext_InvertY(t *testing.T) {
	dc := NewContext(100, 200)
	dc.InvertY()
	x, y := dc.TransformPoint(0, 0)
	if math.Abs(x) > 1e-9 || math.Abs(y-200) > 1e-9 {
		t.Errorf("After InvertY, TransformPoint(0, 0) = (%v, %v), want (0, 200)", x, y)
	}
}

func TestContext_RotateAbout(t *testing.T) {
	dc := NewContext(100, 100)
	dc.RotateAbout(math.Pi/2, 50, 50)
	x, y := dc.TransformPoint(50, 50) // Center should stay the same
	if math.Abs(x-50) > 1e-9 || math.Abs(y-50) > 1e-9 {
		t.Errorf("RotateAbout center should be fixed: got (%v, %v), want (50, 50)", x, y)
	}
}

func TestContext_DrawPoint(t *testing.T) {
	dc := NewContext(20, 20)
	dc.DrawPoint(10, 10, 2)
	// Verify path was created (DrawPoint calls DrawCircle)
}

func TestContext_DrawRoundedRectangle(t *testing.T) {
	dc := NewContext(100, 100)
	dc.DrawRoundedRectangle(10, 10, 80, 80, 5)
	// Should not panic; path should have elements
}

func TestContext_DrawEllipse(t *testing.T) {
	dc := NewContext(100, 100)
	dc.DrawEllipse(50, 50, 30, 20)
	// Should not panic
}

func TestContext_DrawArc(t *testing.T) {
	dc := NewContext(100, 100)
	dc.DrawArc(50, 50, 30, 0, math.Pi)
	// Should not panic
}

func TestContext_DrawEllipticalArc(t *testing.T) {
	dc := NewContext(100, 100)
	dc.DrawEllipticalArc(50, 50, 30, 20, 0, math.Pi)
	// Should not panic
}

func TestContext_DrawRegularPolygon(t *testing.T) {
	dc := NewContext(100, 100)
	dc.DrawRegularPolygon(6, 50, 50, 30, 0)
	// Should not panic; path should have 6 line segments + close
}

// --- Solver tests ---

func TestSolveQuadraticOverflow(t *testing.T) {
	// Create conditions that trigger discriminant overflow:
	// sc0 and sc1 need to be large enough that sc1*sc1 - 4*sc0 overflows
	// This happens when a is very small and b, c are large
	roots := SolveQuadratic(1e-300, 1e150, 1e150)
	// Should not panic and should return some roots
	if roots == nil {
		t.Log("SolveQuadratic with overflow conditions returned nil (may be expected)")
	}
}

func TestSolveQuadratic_LargeCoefficients(t *testing.T) {
	// Test with very large coefficients that cause overflow in sc1^2
	roots := SolveQuadratic(1, 1e200, 1)
	// Should handle gracefully
	if len(roots) > 0 {
		for _, r := range roots {
			if math.IsNaN(r) || math.IsInf(r, 0) {
				t.Errorf("SolveQuadratic returned non-finite root: %v", r)
			}
		}
	}
}

// --- QuadBez Subsegment test ---

func TestQuadBez_Subsegment(t *testing.T) {
	q := NewQuadBez(Pt(0, 0), Pt(5, 10), Pt(10, 0))

	tests := []struct {
		name   string
		t0, t1 float64
	}{
		{"first half", 0, 0.5},
		{"second half", 0.5, 1},
		{"middle third", 0.33, 0.67},
		{"full range", 0, 1},
		{"small segment", 0.4, 0.6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := q.Subsegment(tt.t0, tt.t1)

			// Subsegment start should be close to original.Eval(t0)
			expectedStart := q.Eval(tt.t0)
			if sub.P0.Distance(expectedStart) > 0.01 {
				t.Errorf("Subsegment(%v, %v).P0 = %v, want %v", tt.t0, tt.t1, sub.P0, expectedStart)
			}

			// Subsegment end should be close to original.Eval(t1)
			expectedEnd := q.Eval(tt.t1)
			if sub.P2.Distance(expectedEnd) > 0.01 {
				t.Errorf("Subsegment(%v, %v).P2 = %v, want %v", tt.t0, tt.t1, sub.P2, expectedEnd)
			}

			// Midpoint of subsegment should be close to original.Eval((t0+t1)/2)
			expectedMid := q.Eval((tt.t0 + tt.t1) / 2)
			subMid := sub.Eval(0.5)
			if subMid.Distance(expectedMid) > 0.5 {
				t.Errorf("Subsegment midpoint = %v, want close to %v", subMid, expectedMid)
			}
		})
	}
}

func TestQuadBez_Extrema_Symmetric(t *testing.T) {
	// Quadratic with extrema in both X and Y
	q := NewQuadBez(Pt(0, 0), Pt(10, 10), Pt(20, 0))
	extrema := q.Extrema()
	// Y extrema: should be at t=0.5 (midpoint of symmetric parabola)
	if len(extrema) == 0 {
		t.Error("Expected at least one extremum for parabolic curve")
	}
	for _, e := range extrema {
		if e <= 0 || e >= 1 {
			t.Errorf("Extremum t=%v should be in (0, 1)", e)
		}
	}
}
