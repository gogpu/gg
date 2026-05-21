package gg

import (
	"math"
	"testing"
)

// TestNaN_StrokeCubic_NoCrash verifies that stroking a cubic with NaN control
// points does not crash. The depth guard in flattenCubicRec (stroke/expander.go)
// terminates recursion when NaN prevents the flatness check from converging.
func TestNaN_StrokeCubic_NoCrash(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()
	dc.SetLineWidth(2)
	dc.MoveTo(0, 0)
	dc.CubicTo(math.NaN(), math.NaN(), 50, 50, 100, 100)
	dc.Stroke() // must not crash
}

// TestNaN_StrokeQuad_NoCrash verifies that stroking a quad with NaN control
// points does not crash.
func TestNaN_StrokeQuad_NoCrash(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()
	dc.SetLineWidth(2)
	dc.MoveTo(0, 0)
	dc.QuadraticTo(math.NaN(), math.NaN(), 100, 100)
	dc.Stroke() // must not crash
}

// TestNaN_FillCubic_NoCrash verifies that filling a path with NaN cubic
// control points does not crash. The depth guard in analytic filler's
// flattenCubicRecursive (edge_builder.go) terminates recursion.
func TestNaN_FillCubic_NoCrash(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()
	dc.MoveTo(0, 0)
	dc.CubicTo(math.NaN(), math.NaN(), 50, 50, 100, 100)
	dc.LineTo(100, 0)
	dc.Fill() // must not crash
}

// TestNaN_FillQuad_NoCrash verifies that filling a path with NaN quad
// control points does not crash.
func TestNaN_FillQuad_NoCrash(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()
	dc.MoveTo(0, 0)
	dc.QuadraticTo(math.NaN(), math.NaN(), 100, 100)
	dc.LineTo(100, 0)
	dc.Fill() // must not crash
}

// TestNaN_PathFlatten_NoCrash verifies that Path.Flatten with NaN coordinates
// does not crash. The depth guard in flattenCubicRecursive/flattenQuadRecursive
// (path_ops.go) terminates recursion.
func TestNaN_PathFlatten_NoCrash(t *testing.T) {
	tests := []struct {
		name  string
		build func() *Path
	}{
		{
			name: "cubic NaN control point 1",
			build: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.CubicTo(math.NaN(), 0, 0, math.NaN(), 100, 100)
				return p
			},
		},
		{
			name: "cubic all NaN",
			build: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.CubicTo(math.NaN(), math.NaN(), math.NaN(), math.NaN(), 100, 100)
				return p
			},
		},
		{
			name: "quad NaN control point",
			build: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(math.NaN(), math.NaN(), 100, 100)
				return p
			},
		},
		{
			name: "cubic NaN endpoint",
			build: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.CubicTo(50, 50, 75, 75, math.NaN(), math.NaN())
				return p
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.build()
			p.Flatten(0.25) // must not crash
		})
	}
}

// TestNaN_PathLength_NoCrash verifies that Path.Length with NaN coordinates
// does not crash. The depth guard in quadLengthRecursive/cubicLengthRecursive
// (path_ops.go, depth > 16) terminates recursion.
func TestNaN_PathLength_NoCrash(t *testing.T) {
	tests := []struct {
		name  string
		build func() *Path
	}{
		{
			name: "cubic NaN control point",
			build: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.CubicTo(math.NaN(), 0, 0, 0, 100, 100)
				return p
			},
		},
		{
			name: "quad NaN control point",
			build: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(math.NaN(), math.NaN(), 100, 100)
				return p
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.build().Length(0.01) // must not crash, returns some value
		})
	}
}

// TestNaN_PathContains_NoCrash verifies that Path.Contains (winding number)
// with NaN coordinates does not crash. The depth guard in
// flattenQuadWindingRecursive/flattenCubicWindingRecursive (path_ops.go)
// terminates recursion.
func TestNaN_PathContains_NoCrash(t *testing.T) {
	tests := []struct {
		name  string
		build func() *Path
	}{
		{
			name: "cubic NaN control point",
			build: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.CubicTo(math.NaN(), 0, 50, 50, 100, 100)
				p.LineTo(100, 0)
				p.Close()
				return p
			},
		},
		{
			name: "quad NaN control point",
			build: func() *Path {
				p := NewPath()
				p.MoveTo(0, 0)
				p.QuadraticTo(math.NaN(), math.NaN(), 100, 100)
				p.LineTo(100, 0)
				p.Close()
				return p
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.build().Contains(Pt(50, 50)) // must not crash
		})
	}
}

// TestNaN_DashStroke_NoCrash verifies that dashed stroking with NaN coordinates
// does not crash. The depth guard in flattenQuadRecForDash/flattenCubicRecForDash
// (software.go) terminates recursion.
func TestNaN_DashStroke_NoCrash(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()
	dc.SetDash(5, 3)
	dc.SetLineWidth(2)
	dc.MoveTo(0, 0)
	dc.CubicTo(math.NaN(), math.NaN(), 50, 50, 100, 100)
	dc.Stroke() // must not crash
}

// TestNaN_DashStrokeQuad_NoCrash verifies dashed quad stroking with NaN.
func TestNaN_DashStrokeQuad_NoCrash(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()
	dc.SetDash(5, 3)
	dc.SetLineWidth(2)
	dc.MoveTo(0, 0)
	dc.QuadraticTo(math.NaN(), math.NaN(), 100, 100)
	dc.Stroke() // must not crash
}

// TestInf_StrokeCubic_NoCrash verifies that +/-Inf control points are also
// handled safely. Inf can cause the same non-convergent flatness check.
func TestInf_StrokeCubic_NoCrash(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()
	dc.SetLineWidth(2)
	dc.MoveTo(0, 0)
	dc.CubicTo(math.Inf(1), 0, 0, math.Inf(-1), 100, 100)
	dc.Stroke() // must not crash
}

// TestInf_FillCubic_NoCrash verifies that +/-Inf filling does not crash.
func TestInf_FillCubic_NoCrash(t *testing.T) {
	dc := NewContext(100, 100)
	defer dc.Close()
	dc.MoveTo(0, 0)
	dc.CubicTo(math.Inf(1), 0, 0, math.Inf(-1), 100, 100)
	dc.LineTo(100, 0)
	dc.Fill() // must not crash
}

// TestInf_PathFlatten_NoCrash verifies that Flatten with Inf does not crash.
func TestInf_PathFlatten_NoCrash(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.CubicTo(math.Inf(1), math.Inf(-1), 50, 50, 100, 100)
	p.Flatten(0.25) // must not crash
}

// TestInf_PathLength_NoCrash verifies that Length with Inf does not crash.
func TestInf_PathLength_NoCrash(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.CubicTo(math.Inf(1), 0, 0, math.Inf(-1), 100, 100)
	_ = p.Length(0.01) // must not crash
}
