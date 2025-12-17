package scene

import (
	"math"
	"testing"
)

func TestRectShape(t *testing.T) {
	rect := NewRectShape(10, 20, 100, 50)

	// Test Bounds
	bounds := rect.Bounds()
	if bounds.MinX != 10 || bounds.MinY != 20 || bounds.MaxX != 110 || bounds.MaxY != 70 {
		t.Errorf("bounds = %+v, want (10,20)-(110,70)", bounds)
	}

	// Test ToPath
	path := rect.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// Rectangle path should have 5 verbs: MoveTo + 3 LineTo + Close
	if path.VerbCount() != 5 {
		t.Errorf("path VerbCount() = %d, want 5", path.VerbCount())
	}

	// Test Contains
	if !rect.Contains(50, 50) {
		t.Error("Contains(50, 50) = false, want true (center)")
	}
	if !rect.Contains(10, 20) {
		t.Error("Contains(10, 20) = false, want true (corner)")
	}
	if rect.Contains(0, 0) {
		t.Error("Contains(0, 0) = true, want false (outside)")
	}
	if rect.Contains(200, 200) {
		t.Error("Contains(200, 200) = true, want false (outside)")
	}
}

func TestRoundedRectShape(t *testing.T) {
	rect := NewRoundedRectShape(0, 0, 100, 50, 10)

	bounds := rect.Bounds()
	if bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 100 || bounds.MaxY != 50 {
		t.Errorf("bounds = %+v, want (0,0)-(100,50)", bounds)
	}

	path := rect.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// Should have curves for corners
	hasCubic := false
	for _, verb := range path.Verbs() {
		if verb == VerbCubicTo {
			hasCubic = true
			break
		}
	}
	if !hasCubic {
		t.Error("rounded rectangle should have cubic bezier curves")
	}
}

func TestCircleShape(t *testing.T) {
	circle := NewCircleShape(100, 100, 50)

	bounds := circle.Bounds()
	if bounds.MinX != 50 || bounds.MinY != 50 || bounds.MaxX != 150 || bounds.MaxY != 150 {
		t.Errorf("bounds = %+v, want (50,50)-(150,150)", bounds)
	}

	path := circle.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// Test Contains
	if !circle.Contains(100, 100) {
		t.Error("Contains(100, 100) = false, want true (center)")
	}
	if !circle.Contains(100, 50) {
		t.Error("Contains(100, 50) = false, want true (edge)")
	}
	if circle.Contains(0, 0) {
		t.Error("Contains(0, 0) = true, want false (outside)")
	}
}

func TestEllipseShape(t *testing.T) {
	ellipse := NewEllipseShape(100, 100, 50, 30)

	bounds := ellipse.Bounds()
	if bounds.MinX != 50 || bounds.MinY != 70 || bounds.MaxX != 150 || bounds.MaxY != 130 {
		t.Errorf("bounds = %+v, want (50,70)-(150,130)", bounds)
	}

	path := ellipse.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// Test Contains
	if !ellipse.Contains(100, 100) {
		t.Error("Contains(100, 100) = false, want true (center)")
	}
	if ellipse.Contains(0, 0) {
		t.Error("Contains(0, 0) = true, want false (outside)")
	}

	// Edge case: zero radius
	zeroEllipse := NewEllipseShape(0, 0, 0, 0)
	if zeroEllipse.Contains(0, 0) {
		t.Error("zero radius ellipse should not contain anything")
	}
}

func TestLineShape(t *testing.T) {
	line := NewLineShape(0, 0, 100, 100)

	bounds := line.Bounds()
	if bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 100 || bounds.MaxY != 100 {
		t.Errorf("bounds = %+v, want (0,0)-(100,100)", bounds)
	}

	path := line.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// Line path should have 2 verbs: MoveTo + LineTo
	if path.VerbCount() != 2 {
		t.Errorf("path VerbCount() = %d, want 2", path.VerbCount())
	}

	// Test Length
	length := line.Length()
	expected := float32(math.Sqrt(20000)) // sqrt(100^2 + 100^2)
	if math.Abs(float64(length-expected)) > 0.01 {
		t.Errorf("Length() = %f, want %f", length, expected)
	}

	// Horizontal line
	hLine := NewLineShape(0, 0, 100, 0)
	if hLine.Length() != 100 {
		t.Errorf("horizontal line Length() = %f, want 100", hLine.Length())
	}
}

func TestPathShape(t *testing.T) {
	// Create underlying path
	path := NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 0)
	path.LineTo(50, 100)
	path.Close()

	shape := NewPathShape(path)

	// Test Bounds
	bounds := shape.Bounds()
	if bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 100 || bounds.MaxY != 100 {
		t.Errorf("bounds = %+v, want (0,0)-(100,100)", bounds)
	}

	// Test ToPath returns same path
	if shape.ToPath() != path {
		t.Error("ToPath() should return the underlying path")
	}

	// Test nil path
	nilShape := NewPathShape(nil)
	if !nilShape.Bounds().IsEmpty() {
		t.Error("nil path shape should have empty bounds")
	}
}

func TestPolygonShape(t *testing.T) {
	// Triangle
	poly := NewPolygonShape(0, 0, 100, 0, 50, 100)

	// Test PointCount
	if poly.PointCount() != 3 {
		t.Errorf("PointCount() = %d, want 3", poly.PointCount())
	}

	// Test Point
	x, y, ok := poly.Point(0)
	if !ok || x != 0 || y != 0 {
		t.Errorf("Point(0) = (%f, %f, %v), want (0, 0, true)", x, y, ok)
	}
	x, y, ok = poly.Point(2)
	if !ok || x != 50 || y != 100 {
		t.Errorf("Point(2) = (%f, %f, %v), want (50, 100, true)", x, y, ok)
	}
	_, _, ok = poly.Point(3)
	if ok {
		t.Error("Point(3) should return false for 3-point polygon")
	}
	_, _, ok = poly.Point(-1)
	if ok {
		t.Error("Point(-1) should return false")
	}

	// Test Bounds
	bounds := poly.Bounds()
	if bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 100 || bounds.MaxY != 100 {
		t.Errorf("bounds = %+v, want (0,0)-(100,100)", bounds)
	}

	// Test ToPath
	path := poly.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// Test odd number of points (should truncate)
	oddPoly := NewPolygonShape(0, 0, 100, 0, 50) // 5 values, last ignored
	if oddPoly.PointCount() != 2 {
		t.Errorf("odd points PointCount() = %d, want 2", oddPoly.PointCount())
	}

	// Test too few points
	smallPoly := NewPolygonShape(0, 0) // Only 1 point
	if !smallPoly.ToPath().IsEmpty() {
		t.Error("polygon with 1 point should produce empty path")
	}

	// Test empty polygon
	emptyPoly := NewPolygonShape()
	if !emptyPoly.Bounds().IsEmpty() {
		t.Error("empty polygon should have empty bounds")
	}
}

func TestRegularPolygonShape(t *testing.T) {
	tests := []struct {
		name   string
		sides  int
		expect int
	}{
		{"triangle", 3, 3},
		{"square", 4, 4},
		{"pentagon", 5, 5},
		{"hexagon", 6, 6},
		{"too few sides", 2, 3}, // Should be clamped to 3
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poly := NewRegularPolygonShape(100, 100, 50, tt.sides, 0)

			path := poly.ToPath()
			if path == nil || path.IsEmpty() {
				t.Fatal("ToPath() returned nil or empty path")
			}

			// Count LineTo verbs (each side is a LineTo except first which is MoveTo)
			lineCount := 0
			for _, verb := range path.Verbs() {
				if verb == VerbLineTo {
					lineCount++
				}
			}
			// Expected: sides - 1 (first point is MoveTo, rest are LineTo)
			if lineCount != tt.expect-1 {
				t.Errorf("LineTo count = %d, want %d", lineCount, tt.expect-1)
			}

			// Bounds should contain the polygon
			bounds := poly.Bounds()
			if bounds.IsEmpty() {
				t.Error("bounds should not be empty")
			}
		})
	}
}

func TestStarShape(t *testing.T) {
	star := NewStarShape(100, 100, 50, 25, 5, 0)

	path := star.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// 5-pointed star has 10 vertices (5 outer + 5 inner)
	// That's 1 MoveTo + 9 LineTo + Close
	lineCount := 0
	for _, verb := range path.Verbs() {
		if verb == VerbLineTo {
			lineCount++
		}
	}
	if lineCount != 9 {
		t.Errorf("LineTo count = %d, want 9", lineCount)
	}

	// Bounds
	bounds := star.Bounds()
	if bounds.MinX != 50 || bounds.MinY != 50 || bounds.MaxX != 150 || bounds.MaxY != 150 {
		t.Errorf("bounds = %+v, want (50,50)-(150,150)", bounds)
	}

	// Test with fewer points (should default to 5)
	twoStar := NewStarShape(0, 0, 10, 5, 2, 0)
	if twoStar.Points != 5 {
		t.Errorf("Points with < 3 sides = %d, want 5", twoStar.Points)
	}
}

func TestArcShape(t *testing.T) {
	arc := NewArcShape(100, 100, 50, 50, 0, math.Pi/2, true)

	path := arc.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// Bounds (conservative - full ellipse)
	bounds := arc.Bounds()
	if bounds.IsEmpty() {
		t.Error("bounds should not be empty")
	}
}

func TestPieShape(t *testing.T) {
	pie := NewPieShape(100, 100, 50, 0, math.Pi/2, true)

	path := pie.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// Should have MoveTo at center
	if path.Verbs()[0] != VerbMoveTo {
		t.Error("pie should start with MoveTo")
	}

	// Bounds
	bounds := pie.Bounds()
	if bounds.MinX != 50 || bounds.MinY != 50 || bounds.MaxX != 150 || bounds.MaxY != 150 {
		t.Errorf("bounds = %+v, want (50,50)-(150,150)", bounds)
	}
}

func TestTransformShape(t *testing.T) {
	rect := NewRectShape(0, 0, 100, 100)
	transform := TranslateAffine(50, 50)
	transformed := NewTransformShape(rect, transform)

	// Test Bounds
	bounds := transformed.Bounds()
	if bounds.MinX != 50 || bounds.MinY != 50 || bounds.MaxX != 150 || bounds.MaxY != 150 {
		t.Errorf("bounds = %+v, want (50,50)-(150,150)", bounds)
	}

	// Test ToPath
	path := transformed.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// First point should be transformed
	points := path.Points()
	if points[0] != 50 || points[1] != 50 {
		t.Errorf("first point = (%f, %f), want (50, 50)", points[0], points[1])
	}

	// Test nil shape
	nilTransform := NewTransformShape(nil, IdentityAffine())
	if !nilTransform.Bounds().IsEmpty() {
		t.Error("nil shape should have empty bounds")
	}
	if !nilTransform.ToPath().IsEmpty() {
		t.Error("nil shape should produce empty path")
	}

	// Test with rotation
	rotated := NewTransformShape(rect, RotateAffine(math.Pi/4))
	rotBounds := rotated.Bounds()
	// After 45-degree rotation, bounds should be larger
	if rotBounds.Width() <= 100 || rotBounds.Height() <= 100 {
		t.Error("rotated rectangle should have larger bounds")
	}
}

func TestCompositeShape(t *testing.T) {
	rect := NewRectShape(0, 0, 50, 50)
	circle := NewCircleShape(100, 100, 25)

	composite := NewCompositeShape(rect, circle)

	// Test Bounds (union of both)
	bounds := composite.Bounds()
	if bounds.MinX != 0 || bounds.MinY != 0 {
		t.Errorf("bounds min = (%f, %f), want (0, 0)", bounds.MinX, bounds.MinY)
	}
	if bounds.MaxX != 125 || bounds.MaxY != 125 {
		t.Errorf("bounds max = (%f, %f), want (125, 125)", bounds.MaxX, bounds.MaxY)
	}

	// Test ShapeCount
	if composite.ShapeCount() != 2 {
		t.Errorf("ShapeCount() = %d, want 2", composite.ShapeCount())
	}

	// Test ToPath
	path := composite.ToPath()
	if path == nil || path.IsEmpty() {
		t.Fatal("ToPath() returned nil or empty path")
	}

	// Test AddShape
	line := NewLineShape(0, 0, 200, 200)
	composite.AddShape(line)
	if composite.ShapeCount() != 3 {
		t.Errorf("ShapeCount() after AddShape = %d, want 3", composite.ShapeCount())
	}

	// Test with nil shape
	composite.AddShape(nil)
	_ = composite.ToPath() // Should not panic

	// Test empty composite
	emptyComposite := NewCompositeShape()
	if !emptyComposite.Bounds().IsEmpty() {
		t.Error("empty composite should have empty bounds")
	}
}

func TestShapeInterface(t *testing.T) {
	// Verify all shapes implement Shape interface
	shapes := []Shape{
		NewRectShape(0, 0, 100, 100),
		NewRoundedRectShape(0, 0, 100, 100, 10),
		NewCircleShape(50, 50, 25),
		NewEllipseShape(50, 50, 30, 20),
		NewLineShape(0, 0, 100, 100),
		NewPathShape(NewPath().Rectangle(0, 0, 50, 50)),
		NewPolygonShape(0, 0, 100, 0, 50, 100),
		NewRegularPolygonShape(50, 50, 30, 6, 0),
		NewStarShape(50, 50, 30, 15, 5, 0),
		NewArcShape(50, 50, 30, 30, 0, math.Pi, true),
		NewPieShape(50, 50, 30, 0, math.Pi/2, true),
		NewTransformShape(NewRectShape(0, 0, 100, 100), TranslateAffine(10, 10)),
		NewCompositeShape(NewRectShape(0, 0, 50, 50)),
	}

	for i, shape := range shapes {
		path := shape.ToPath()
		if path == nil {
			t.Errorf("shape[%d] ToPath() returned nil", i)
		}

		bounds := shape.Bounds()
		// Just verify it doesn't panic
		_ = bounds
	}
}

// Benchmarks

func BenchmarkRectShapeToPath(b *testing.B) {
	rect := NewRectShape(0, 0, 100, 100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = rect.ToPath()
	}
}

func BenchmarkCircleShapeToPath(b *testing.B) {
	circle := NewCircleShape(50, 50, 25)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = circle.ToPath()
	}
}

func BenchmarkPolygonShapeToPath(b *testing.B) {
	poly := NewPolygonShape(0, 0, 100, 0, 100, 100, 50, 150, 0, 100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = poly.ToPath()
	}
}

func BenchmarkTransformShapeToPath(b *testing.B) {
	rect := NewRectShape(0, 0, 100, 100)
	transform := TranslateAffine(50, 50).Multiply(RotateAffine(math.Pi / 4))
	shape := NewTransformShape(rect, transform)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = shape.ToPath()
	}
}

func BenchmarkCompositeShapeToPath(b *testing.B) {
	composite := NewCompositeShape(
		NewRectShape(0, 0, 50, 50),
		NewCircleShape(100, 100, 25),
		NewLineShape(0, 0, 200, 200),
	)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = composite.ToPath()
	}
}
