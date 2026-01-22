package stroke

import (
	"math"
	"testing"
)

func TestNewStrokeExpander(t *testing.T) {
	style := DefaultStroke()
	expander := NewStrokeExpander(style)

	if expander == nil {
		t.Fatal("NewStrokeExpander returned nil")
	}
	if expander.style.Width != 1.0 {
		t.Errorf("style.Width = %v, want 1.0", expander.style.Width)
	}
	if expander.tolerance != 0.25 {
		t.Errorf("tolerance = %v, want 0.25", expander.tolerance)
	}
}

func TestStrokeExpander_SetTolerance(t *testing.T) {
	expander := NewStrokeExpander(DefaultStroke())

	expander.SetTolerance(0.1)
	if expander.tolerance != 0.1 {
		t.Errorf("tolerance = %v, want 0.1", expander.tolerance)
	}

	// Negative tolerance should be ignored
	expander.SetTolerance(-1.0)
	if expander.tolerance != 0.1 {
		t.Error("negative tolerance should be ignored")
	}

	// Zero tolerance should be ignored
	expander.SetTolerance(0)
	if expander.tolerance != 0.1 {
		t.Error("zero tolerance should be ignored")
	}
}

func TestStrokeExpander_ExpandSimpleLine(t *testing.T) {
	style := Stroke{
		Width:      2.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	// Simple horizontal line from (0,0) to (10,0)
	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 0}},
	}

	result := expander.Expand(input)

	// Should produce a closed rectangle
	if len(result) < 4 {
		t.Fatalf("expected at least 4 elements, got %d", len(result))
	}

	// First element should be MoveTo
	if _, ok := result[0].(MoveTo); !ok {
		t.Error("first element should be MoveTo")
	}

	// Should have a Close element
	hasClose := false
	for _, el := range result {
		if _, ok := el.(Close); ok {
			hasClose = true
			break
		}
	}
	if !hasClose {
		t.Error("result should contain Close element")
	}
}

func TestStrokeExpander_ExpandSquare(t *testing.T) {
	style := Stroke{
		Width:      2.0,
		Cap:        LineCapButt,
		Join:       LineJoinBevel,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	// Square path
	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 10}},
		LineTo{Point: Point{X: 0, Y: 10}},
		Close{},
	}

	result := expander.Expand(input)

	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}

	// Closed path should produce two closed subpaths (outer and inner)
	closeCount := 0
	for _, el := range result {
		if _, ok := el.(Close); ok {
			closeCount++
		}
	}
	if closeCount != 2 {
		t.Errorf("closed path should produce 2 Close elements, got %d", closeCount)
	}
}

func TestStrokeExpander_ExpandWithRoundCap(t *testing.T) {
	style := Stroke{
		Width:      4.0,
		Cap:        LineCapRound,
		Join:       LineJoinRound,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 0}},
	}

	result := expander.Expand(input)

	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}

	// Round caps produce CubicTo elements
	hasCubic := false
	for _, el := range result {
		if _, ok := el.(CubicTo); ok {
			hasCubic = true
			break
		}
	}
	if !hasCubic {
		t.Error("round cap should produce CubicTo elements")
	}
}

func TestStrokeExpander_ExpandWithSquareCap(t *testing.T) {
	style := Stroke{
		Width:      4.0,
		Cap:        LineCapSquare,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 0}},
	}

	result := expander.Expand(input)

	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}

	// Count LineTo elements (square cap adds extra line segments)
	lineCount := 0
	for _, el := range result {
		if _, ok := el.(LineTo); ok {
			lineCount++
		}
	}
	// Should have more lines than a butt cap
	if lineCount < 4 {
		t.Errorf("square cap should produce at least 4 LineTo elements, got %d", lineCount)
	}
}

func TestStrokeExpander_MiterJoin(t *testing.T) {
	style := Stroke{
		Width:      2.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	// Two lines at 90 degrees
	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 10}},
	}

	result := expander.Expand(input)

	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}
}

func TestStrokeExpander_BevelJoin(t *testing.T) {
	style := Stroke{
		Width:      2.0,
		Cap:        LineCapButt,
		Join:       LineJoinBevel,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	// Two lines at 90 degrees
	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 10}},
	}

	result := expander.Expand(input)

	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}
}

func TestStrokeExpander_RoundJoin(t *testing.T) {
	style := Stroke{
		Width:      4.0,
		Cap:        LineCapButt,
		Join:       LineJoinRound,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	// Two lines at 90 degrees
	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 10}},
	}

	result := expander.Expand(input)

	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}

	// Round join produces CubicTo elements
	hasCubic := false
	for _, el := range result {
		if _, ok := el.(CubicTo); ok {
			hasCubic = true
			break
		}
	}
	if !hasCubic {
		t.Error("round join should produce CubicTo elements")
	}
}

func TestStrokeExpander_QuadraticCurve(t *testing.T) {
	style := Stroke{
		Width:      2.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		QuadTo{Control: Point{X: 5, Y: 5}, Point: Point{X: 10, Y: 0}},
	}

	result := expander.Expand(input)

	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}
}

func TestStrokeExpander_CubicCurve(t *testing.T) {
	style := Stroke{
		Width:      2.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		CubicTo{Control1: Point{X: 3, Y: 5}, Control2: Point{X: 7, Y: 5}, Point: Point{X: 10, Y: 0}},
	}

	result := expander.Expand(input)

	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}
}

func TestStrokeExpander_EmptyPath(t *testing.T) {
	style := DefaultStroke()
	expander := NewStrokeExpander(style)

	result := expander.Expand(nil)
	if len(result) != 0 {
		t.Error("empty input should produce empty output")
	}

	result = expander.Expand([]PathElement{})
	if len(result) != 0 {
		t.Error("empty input should produce empty output")
	}
}

func TestStrokeExpander_SingleMoveTo(t *testing.T) {
	style := DefaultStroke()
	expander := NewStrokeExpander(style)

	input := []PathElement{
		MoveTo{Point: Point{X: 5, Y: 5}},
	}

	result := expander.Expand(input)

	// A single MoveTo with no actual drawing should produce no output
	// (no segments to expand)
	if len(result) != 0 {
		t.Errorf("single MoveTo should produce no output, got %d elements", len(result))
	}
}

func TestStrokeExpander_ZeroLengthLine(t *testing.T) {
	style := DefaultStroke()
	expander := NewStrokeExpander(style)

	// Zero-length line (same start and end point)
	input := []PathElement{
		MoveTo{Point: Point{X: 5, Y: 5}},
		LineTo{Point: Point{X: 5, Y: 5}},
	}

	result := expander.Expand(input)

	// Zero-length lines should be skipped
	if len(result) != 0 {
		t.Errorf("zero-length line should produce no output, got %d elements", len(result))
	}
}

func TestStrokeExpander_MultipleSubpaths(t *testing.T) {
	style := Stroke{
		Width:      2.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		LineTo{Point: Point{X: 10, Y: 0}},
		MoveTo{Point: Point{X: 0, Y: 10}},
		LineTo{Point: Point{X: 10, Y: 10}},
	}

	result := expander.Expand(input)

	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}

	// Should have 2 MoveTo elements (one for each subpath)
	moveCount := 0
	for _, el := range result {
		if _, ok := el.(MoveTo); ok {
			moveCount++
		}
	}
	if moveCount != 2 {
		t.Errorf("expected 2 MoveTo elements, got %d", moveCount)
	}
}

func TestVec2_Operations(t *testing.T) {
	t.Run("Add", func(t *testing.T) {
		v := Vec2{X: 1, Y: 2}
		w := Vec2{X: 3, Y: 4}
		result := v.Add(w)
		if result.X != 4 || result.Y != 6 {
			t.Errorf("Add = %v, want (4, 6)", result)
		}
	})

	t.Run("Sub", func(t *testing.T) {
		v := Vec2{X: 5, Y: 7}
		w := Vec2{X: 2, Y: 3}
		result := v.Sub(w)
		if result.X != 3 || result.Y != 4 {
			t.Errorf("Sub = %v, want (3, 4)", result)
		}
	})

	t.Run("Scale", func(t *testing.T) {
		v := Vec2{X: 3, Y: 4}
		result := v.Scale(2)
		if result.X != 6 || result.Y != 8 {
			t.Errorf("Scale = %v, want (6, 8)", result)
		}
	})

	t.Run("Neg", func(t *testing.T) {
		v := Vec2{X: 3, Y: -4}
		result := v.Neg()
		if result.X != -3 || result.Y != 4 {
			t.Errorf("Neg = %v, want (-3, 4)", result)
		}
	})

	t.Run("Dot", func(t *testing.T) {
		v := Vec2{X: 1, Y: 2}
		w := Vec2{X: 3, Y: 4}
		result := v.Dot(w)
		if result != 11 {
			t.Errorf("Dot = %v, want 11", result)
		}
	})

	t.Run("Cross", func(t *testing.T) {
		v := Vec2{X: 1, Y: 0}
		w := Vec2{X: 0, Y: 1}
		result := v.Cross(w)
		if result != 1 {
			t.Errorf("Cross = %v, want 1", result)
		}
	})

	t.Run("Length", func(t *testing.T) {
		v := Vec2{X: 3, Y: 4}
		result := v.Length()
		if result != 5 {
			t.Errorf("Length = %v, want 5", result)
		}
	})

	t.Run("Normalize", func(t *testing.T) {
		v := Vec2{X: 3, Y: 4}
		result := v.Normalize()
		if math.Abs(result.X-0.6) > 1e-10 || math.Abs(result.Y-0.8) > 1e-10 {
			t.Errorf("Normalize = %v, want (0.6, 0.8)", result)
		}
	})

	t.Run("Normalize zero vector", func(t *testing.T) {
		v := Vec2{X: 0, Y: 0}
		result := v.Normalize()
		if result.X != 0 || result.Y != 0 {
			t.Errorf("Normalize(0,0) = %v, want (0, 0)", result)
		}
	})

	t.Run("Perp", func(t *testing.T) {
		v := Vec2{X: 1, Y: 0}
		result := v.Perp()
		if result.X != 0 || result.Y != 1 {
			t.Errorf("Perp = %v, want (0, 1)", result)
		}
	})

	t.Run("Angle", func(t *testing.T) {
		v := Vec2{X: 1, Y: 0}
		if v.Angle() != 0 {
			t.Errorf("Angle(1,0) = %v, want 0", v.Angle())
		}

		v = Vec2{X: 0, Y: 1}
		if math.Abs(v.Angle()-math.Pi/2) > 1e-10 {
			t.Errorf("Angle(0,1) = %v, want Pi/2", v.Angle())
		}
	})
}

func TestPoint_Operations(t *testing.T) {
	t.Run("Add", func(t *testing.T) {
		p := Point{X: 1, Y: 2}
		v := Vec2{X: 3, Y: 4}
		result := p.Add(v)
		if result.X != 4 || result.Y != 6 {
			t.Errorf("Add = %v, want (4, 6)", result)
		}
	})

	t.Run("Sub", func(t *testing.T) {
		p := Point{X: 5, Y: 7}
		q := Point{X: 2, Y: 3}
		result := p.Sub(q)
		if result.X != 3 || result.Y != 4 {
			t.Errorf("Sub = %v, want (3, 4)", result)
		}
	})

	t.Run("Distance", func(t *testing.T) {
		p := Point{X: 0, Y: 0}
		q := Point{X: 3, Y: 4}
		result := p.Distance(q)
		if result != 5 {
			t.Errorf("Distance = %v, want 5", result)
		}
	})

	t.Run("Lerp", func(t *testing.T) {
		p := Point{X: 0, Y: 0}
		q := Point{X: 10, Y: 10}
		result := p.Lerp(q, 0.5)
		if result.X != 5 || result.Y != 5 {
			t.Errorf("Lerp = %v, want (5, 5)", result)
		}
	})
}

func TestDistanceToLine(t *testing.T) {
	tests := []struct {
		name string
		p    Point
		a    Point
		b    Point
		want float64
	}{
		{
			name: "point on line",
			p:    Point{X: 5, Y: 0},
			a:    Point{X: 0, Y: 0},
			b:    Point{X: 10, Y: 0},
			want: 0,
		},
		{
			name: "point above line",
			p:    Point{X: 5, Y: 3},
			a:    Point{X: 0, Y: 0},
			b:    Point{X: 10, Y: 0},
			want: 3,
		},
		{
			name: "point before line start",
			p:    Point{X: -3, Y: 0},
			a:    Point{X: 0, Y: 0},
			b:    Point{X: 10, Y: 0},
			want: 3,
		},
		{
			name: "point after line end",
			p:    Point{X: 14, Y: 0},
			a:    Point{X: 0, Y: 0},
			b:    Point{X: 10, Y: 0},
			want: 4,
		},
		{
			name: "degenerate line (a == b)",
			p:    Point{X: 3, Y: 4},
			a:    Point{X: 0, Y: 0},
			b:    Point{X: 0, Y: 0},
			want: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := distanceToLine(tt.p, tt.a, tt.b)
			if math.Abs(got-tt.want) > 1e-10 {
				t.Errorf("distanceToLine(%v, %v, %v) = %v, want %v", tt.p, tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestDefaultStroke(t *testing.T) {
	s := DefaultStroke()

	if s.Width != 1.0 {
		t.Errorf("Width = %v, want 1.0", s.Width)
	}
	if s.Cap != LineCapButt {
		t.Errorf("Cap = %v, want LineCapButt", s.Cap)
	}
	if s.Join != LineJoinMiter {
		t.Errorf("Join = %v, want LineJoinMiter", s.Join)
	}
	if s.MiterLimit != 4.0 {
		t.Errorf("MiterLimit = %v, want 4.0", s.MiterLimit)
	}
}

func TestPathBuilder(t *testing.T) {
	pb := newPathBuilder()

	if !pb.isEmpty() {
		t.Error("new pathBuilder should be empty")
	}

	pb.moveTo(Point{X: 0, Y: 0})
	if pb.isEmpty() {
		t.Error("pathBuilder should not be empty after moveTo")
	}

	pb.lineTo(Point{X: 10, Y: 0})
	pb.quadTo(Point{X: 15, Y: 5}, Point{X: 20, Y: 0})
	pb.cubicTo(Point{X: 25, Y: 5}, Point{X: 35, Y: 5}, Point{X: 40, Y: 0})
	pb.close()

	elements := pb.build()
	if len(elements) != 5 {
		t.Errorf("expected 5 elements, got %d", len(elements))
	}
}

// Benchmark for stroke expansion
func BenchmarkStrokeExpander_SimpleLine(b *testing.B) {
	style := DefaultStroke()
	expander := NewStrokeExpander(style)
	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		LineTo{Point: Point{X: 100, Y: 0}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expander.Expand(input)
	}
}

func BenchmarkStrokeExpander_Square(b *testing.B) {
	style := Stroke{
		Width:      2.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)
	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 0}},
		LineTo{Point: Point{X: 100, Y: 0}},
		LineTo{Point: Point{X: 100, Y: 100}},
		LineTo{Point: Point{X: 0, Y: 100}},
		Close{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expander.Expand(input)
	}
}

func BenchmarkStrokeExpander_ComplexPath(b *testing.B) {
	style := Stroke{
		Width:      2.0,
		Cap:        LineCapRound,
		Join:       LineJoinRound,
		MiterLimit: 4.0,
	}
	expander := NewStrokeExpander(style)

	// Create a complex path with many segments
	input := []PathElement{MoveTo{Point: Point{X: 0, Y: 0}}}
	for i := 1; i <= 100; i++ {
		x := float64(i * 10)
		y := float64((i % 2) * 10)
		input = append(input, LineTo{Point: Point{X: x, Y: y}})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expander.Expand(input)
	}
}

func BenchmarkStrokeExpander_CubicCurves(b *testing.B) {
	style := DefaultStroke()
	expander := NewStrokeExpander(style)

	// Path with cubic curves
	input := []PathElement{
		MoveTo{Point: Point{X: 0, Y: 50}},
		CubicTo{
			Control1: Point{X: 25, Y: 0},
			Control2: Point{X: 75, Y: 100},
			Point:    Point{X: 100, Y: 50},
		},
		CubicTo{
			Control1: Point{X: 125, Y: 0},
			Control2: Point{X: 175, Y: 100},
			Point:    Point{X: 200, Y: 50},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expander.Expand(input)
	}
}
