// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/gpu/tilecompute"
)

// TestConvertPathToPathDef_NilPath verifies nil/empty paths produce empty PathDef.
func TestConvertPathToPathDef_NilPath(t *testing.T) {
	pd := convertPathToPathDef(nil, nil)
	if len(pd.Lines) != 0 {
		t.Errorf("nil path: expected 0 lines, got %d", len(pd.Lines))
	}

	emptyPath := gg.NewPath()
	pd = convertPathToPathDef(emptyPath, nil)
	if len(pd.Lines) != 0 {
		t.Errorf("empty path: expected 0 lines, got %d", len(pd.Lines))
	}
}

// TestConvertPathToPathDef_SingleLine verifies a single line segment.
func TestConvertPathToPathDef_SingleLine(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(10, 20)
	path.LineTo(30, 40)

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	pd := convertPathToPathDef(path, paint)
	if len(pd.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(pd.Lines))
	}

	line := pd.Lines[0]
	if !approxEq32(line.P0[0], 10) || !approxEq32(line.P0[1], 20) {
		t.Errorf("P0: expected (10, 20), got (%f, %f)", line.P0[0], line.P0[1])
	}
	if !approxEq32(line.P1[0], 30) || !approxEq32(line.P1[1], 40) {
		t.Errorf("P1: expected (30, 40), got (%f, %f)", line.P1[0], line.P1[1])
	}
}

// TestConvertPathToPathDef_Triangle verifies a closed triangle produces 3 line segments.
func TestConvertPathToPathDef_Triangle(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 50)
	path.LineTo(10, 90)
	path.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Green))

	pd := convertPathToPathDef(path, paint)
	if len(pd.Lines) != 3 {
		t.Fatalf("expected 3 lines for triangle, got %d", len(pd.Lines))
	}

	// Line 1: (10,10) -> (90,50)
	if !approxEq32(pd.Lines[0].P0[0], 10) || !approxEq32(pd.Lines[0].P0[1], 10) {
		t.Errorf("line 0 P0: expected (10, 10), got (%f, %f)", pd.Lines[0].P0[0], pd.Lines[0].P0[1])
	}
	if !approxEq32(pd.Lines[0].P1[0], 90) || !approxEq32(pd.Lines[0].P1[1], 50) {
		t.Errorf("line 0 P1: expected (90, 50), got (%f, %f)", pd.Lines[0].P1[0], pd.Lines[0].P1[1])
	}

	// Line 2: (90,50) -> (10,90)
	if !approxEq32(pd.Lines[1].P0[0], 90) || !approxEq32(pd.Lines[1].P0[1], 50) {
		t.Errorf("line 1 P0: expected (90, 50), got (%f, %f)", pd.Lines[1].P0[0], pd.Lines[1].P0[1])
	}
	if !approxEq32(pd.Lines[1].P1[0], 10) || !approxEq32(pd.Lines[1].P1[1], 90) {
		t.Errorf("line 1 P1: expected (10, 90), got (%f, %f)", pd.Lines[1].P1[0], pd.Lines[1].P1[1])
	}

	// Line 3 (close): (10,90) -> (10,10)
	if !approxEq32(pd.Lines[2].P0[0], 10) || !approxEq32(pd.Lines[2].P0[1], 90) {
		t.Errorf("line 2 P0: expected (10, 90), got (%f, %f)", pd.Lines[2].P0[0], pd.Lines[2].P0[1])
	}
	if !approxEq32(pd.Lines[2].P1[0], 10) || !approxEq32(pd.Lines[2].P1[1], 10) {
		t.Errorf("line 2 P1: expected (10, 10), got (%f, %f)", pd.Lines[2].P1[0], pd.Lines[2].P1[1])
	}
}

// TestConvertPathToPathDef_Rectangle verifies a rectangle produces 4 lines.
func TestConvertPathToPathDef_Rectangle(t *testing.T) {
	path := gg.NewPath()
	path.Rectangle(10, 20, 100, 50)

	paint := gg.NewPaint()
	pd := convertPathToPathDef(path, paint)

	if len(pd.Lines) != 4 {
		t.Fatalf("expected 4 lines for rectangle, got %d", len(pd.Lines))
	}
}

// TestConvertPathToPathDef_CurveFlattening verifies cubic curves are flattened.
func TestConvertPathToPathDef_CurveFlattening(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.CubicTo(0, 100, 100, 100, 100, 0)

	paint := gg.NewPaint()
	pd := convertPathToPathDef(path, paint)

	// A cubic curve should be flattened into multiple line segments.
	if len(pd.Lines) < 2 {
		t.Fatalf("expected multiple lines from cubic flattening, got %d", len(pd.Lines))
	}

	// Verify the flattened lines form a continuous chain.
	for i := 1; i < len(pd.Lines); i++ {
		prev := pd.Lines[i-1].P1
		curr := pd.Lines[i].P0
		if !approxEq32(prev[0], curr[0]) || !approxEq32(prev[1], curr[1]) {
			t.Errorf("line chain break at %d: prev P1=(%f,%f), curr P0=(%f,%f)",
				i, prev[0], prev[1], curr[0], curr[1])
		}
	}

	// First line starts near origin, last line ends near (100,0).
	if !approxEq32(pd.Lines[0].P0[0], 0) || !approxEq32(pd.Lines[0].P0[1], 0) {
		t.Errorf("first line should start near (0,0), got (%f,%f)",
			pd.Lines[0].P0[0], pd.Lines[0].P0[1])
	}
	lastLine := pd.Lines[len(pd.Lines)-1]
	if !approxEq32(lastLine.P1[0], 100) || !approxEq32(lastLine.P1[1], 0) {
		t.Errorf("last line should end near (100,0), got (%f,%f)",
			lastLine.P1[0], lastLine.P1[1])
	}
}

// TestConvertPathToPathDef_QuadElevation verifies quadratic curves are elevated to cubic.
func TestConvertPathToPathDef_QuadElevation(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.QuadraticTo(50, 100, 100, 0)

	paint := gg.NewPaint()
	pd := convertPathToPathDef(path, paint)

	// Quadratic → cubic → flatten → lines.
	if len(pd.Lines) < 2 {
		t.Fatalf("expected multiple lines from quad flattening, got %d", len(pd.Lines))
	}

	// First line starts near origin.
	if !approxEq32(pd.Lines[0].P0[0], 0) || !approxEq32(pd.Lines[0].P0[1], 0) {
		t.Errorf("first line should start near (0,0)")
	}
	// Last line ends near (100,0).
	lastLine := pd.Lines[len(pd.Lines)-1]
	if !approxEq32(lastLine.P1[0], 100) || !approxEq32(lastLine.P1[1], 0) {
		t.Errorf("last line should end near (100,0)")
	}
}

// TestConvertPathToPathDef_Circle verifies a circle produces many flattened lines.
func TestConvertPathToPathDef_Circle(t *testing.T) {
	path := gg.NewPath()
	path.Circle(50, 50, 40)

	paint := gg.NewPaint()
	pd := convertPathToPathDef(path, paint)

	// A circle is 4 cubics → many flattened lines.
	if len(pd.Lines) < 8 {
		t.Fatalf("expected many lines for circle, got %d", len(pd.Lines))
	}

	// Verify it's a closed shape: last P1 should be close to first P0.
	firstP0 := pd.Lines[0].P0
	lastP1 := pd.Lines[len(pd.Lines)-1].P1
	if !approxEq32(firstP0[0], lastP1[0]) || !approxEq32(firstP0[1], lastP1[1]) {
		t.Errorf("circle not closed: first P0=(%f,%f), last P1=(%f,%f)",
			firstP0[0], firstP0[1], lastP1[0], lastP1[1])
	}
}

// TestConvertPathToPathDef_ColorExtraction verifies RGBA color conversion.
func TestConvertPathToPathDef_ColorExtraction(t *testing.T) {
	tests := []struct {
		name  string
		color gg.RGBA
		want  [4]uint8
	}{
		{"black", gg.Black, [4]uint8{0, 0, 0, 255}},
		{"white", gg.White, [4]uint8{255, 255, 255, 255}},
		{"red", gg.Red, [4]uint8{255, 0, 0, 255}},
		{"green", gg.Green, [4]uint8{0, 255, 0, 255}},
		{"blue", gg.Blue, [4]uint8{0, 0, 255, 255}},
		{"half-transparent", gg.RGBA2(1, 0, 0, 0.5), [4]uint8{255, 0, 0, 128}},
		{"transparent", gg.Transparent, [4]uint8{0, 0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := gg.NewPath()
			path.MoveTo(0, 0)
			path.LineTo(10, 10)

			paint := gg.NewPaint()
			paint.SetBrush(gg.Solid(tt.color))

			pd := convertPathToPathDef(path, paint)

			// Allow +/- 1 for rounding.
			for i := 0; i < 4; i++ {
				diff := int(pd.Color[i]) - int(tt.want[i])
				if diff < -1 || diff > 1 {
					t.Errorf("color[%d]: expected %d, got %d", i, tt.want[i], pd.Color[i])
				}
			}
		})
	}
}

// TestConvertPathToPathDef_FillRule verifies fill rule mapping.
func TestConvertPathToPathDef_FillRule(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(10, 0)
	path.LineTo(10, 10)
	path.Close()

	// NonZero (default)
	paintNZ := gg.NewPaint()
	paintNZ.FillRule = gg.FillRuleNonZero
	pdNZ := convertPathToPathDef(path, paintNZ)
	if pdNZ.FillRule != tilecompute.FillRuleNonZero {
		t.Errorf("expected NonZero fill rule, got %v", pdNZ.FillRule)
	}

	// EvenOdd
	paintEO := gg.NewPaint()
	paintEO.FillRule = gg.FillRuleEvenOdd
	pdEO := convertPathToPathDef(path, paintEO)
	if pdEO.FillRule != tilecompute.FillRuleEvenOdd {
		t.Errorf("expected EvenOdd fill rule, got %v", pdEO.FillRule)
	}
}

// TestConvertPathToPathDef_NilPaint verifies graceful handling of nil paint.
func TestConvertPathToPathDef_NilPaint(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(10, 10)

	pd := convertPathToPathDef(path, nil)
	if len(pd.Lines) != 1 {
		t.Fatalf("expected 1 line with nil paint, got %d", len(pd.Lines))
	}
	// Default color should be black opaque.
	if pd.Color != [4]uint8{0, 0, 0, 255} {
		t.Errorf("expected black color with nil paint, got %v", pd.Color)
	}
	if pd.FillRule != tilecompute.FillRuleNonZero {
		t.Errorf("expected NonZero fill rule with nil paint, got %v", pd.FillRule)
	}
}

// TestConvertPathToPathDef_ZeroLengthLines verifies zero-length lines are skipped.
func TestConvertPathToPathDef_ZeroLengthLines(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(10, 10) // zero-length, should be skipped
	path.LineTo(20, 20)

	pd := convertPathToPathDef(path, nil)
	if len(pd.Lines) != 1 {
		t.Fatalf("expected 1 line (zero-length skipped), got %d", len(pd.Lines))
	}
}

// TestConvertShapeToPathDef_Circle verifies shape conversion for circles.
func TestConvertShapeToPathDef_Circle(t *testing.T) {
	shape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 30,
		RadiusY: 30,
	}
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Blue))

	pd := convertShapeToPathDef(shape, paint)
	if len(pd.Lines) < 8 {
		t.Fatalf("expected many lines for circle shape, got %d", len(pd.Lines))
	}
	// Blue color
	if pd.Color[2] < 254 {
		t.Errorf("expected blue color, got %v", pd.Color)
	}
}

// TestConvertShapeToPathDef_Rect verifies shape conversion for rectangles.
func TestConvertShapeToPathDef_Rect(t *testing.T) {
	shape := gg.DetectedShape{
		Kind:    gg.ShapeRect,
		CenterX: 50,
		CenterY: 50,
		Width:   80,
		Height:  40,
	}
	paint := gg.NewPaint()

	pd := convertShapeToPathDef(shape, paint)
	if len(pd.Lines) != 4 {
		t.Fatalf("expected 4 lines for rect shape, got %d", len(pd.Lines))
	}
}

// TestConvertShapeToPathDef_Unknown verifies unknown shapes return empty PathDef.
func TestConvertShapeToPathDef_Unknown(t *testing.T) {
	shape := gg.DetectedShape{Kind: gg.ShapeUnknown}
	pd := convertShapeToPathDef(shape, nil)
	if len(pd.Lines) != 0 {
		t.Errorf("expected 0 lines for unknown shape, got %d", len(pd.Lines))
	}
}

// TestConvertPathToPathDef_MultipleSubpaths verifies paths with multiple MoveTo.
func TestConvertPathToPathDef_MultipleSubpaths(t *testing.T) {
	path := gg.NewPath()
	// Subpath 1: triangle
	path.MoveTo(0, 0)
	path.LineTo(10, 0)
	path.LineTo(5, 10)
	path.Close()
	// Subpath 2: another triangle
	path.MoveTo(20, 20)
	path.LineTo(30, 20)
	path.LineTo(25, 30)
	path.Close()

	pd := convertPathToPathDef(path, nil)
	// 3 sides + close + 3 sides + close = 6 lines
	if len(pd.Lines) != 6 {
		t.Fatalf("expected 6 lines for two triangles, got %d", len(pd.Lines))
	}
}

func approxEq32(a, b float32) bool {
	return math.Abs(float64(a-b)) < 0.5
}
