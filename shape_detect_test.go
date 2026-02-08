package gg

import (
	"math"
	"testing"
)

func TestDetectCircle(t *testing.T) {
	p := NewPath()
	p.Circle(100, 100, 50)

	shape := DetectShape(p)
	if shape.Kind != ShapeCircle {
		t.Fatalf("expected ShapeCircle, got %d", shape.Kind)
	}
	if math.Abs(shape.CenterX-100) > shapeDetectTolerance {
		t.Errorf("CenterX = %f, want 100", shape.CenterX)
	}
	if math.Abs(shape.CenterY-100) > shapeDetectTolerance {
		t.Errorf("CenterY = %f, want 100", shape.CenterY)
	}
	if math.Abs(shape.RadiusX-50) > shapeDetectTolerance {
		t.Errorf("RadiusX = %f, want 50", shape.RadiusX)
	}
	if math.Abs(shape.RadiusY-50) > shapeDetectTolerance {
		t.Errorf("RadiusY = %f, want 50", shape.RadiusY)
	}
}

func TestDetectCircleViaContext(t *testing.T) {
	// DrawCircle in Context applies identity transform by default.
	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.DrawCircle(100, 100, 50)

	shape := DetectShape(dc.path)
	if shape.Kind != ShapeCircle {
		t.Fatalf("expected ShapeCircle from DrawCircle, got %d", shape.Kind)
	}
	if math.Abs(shape.RadiusX-50) > shapeDetectTolerance {
		t.Errorf("RadiusX = %f, want 50", shape.RadiusX)
	}
}

func TestDetectEllipse(t *testing.T) {
	p := NewPath()
	p.Ellipse(200, 150, 80, 40)

	shape := DetectShape(p)
	if shape.Kind != ShapeEllipse {
		t.Fatalf("expected ShapeEllipse, got %d", shape.Kind)
	}
	if math.Abs(shape.CenterX-200) > shapeDetectTolerance {
		t.Errorf("CenterX = %f, want 200", shape.CenterX)
	}
	if math.Abs(shape.CenterY-150) > shapeDetectTolerance {
		t.Errorf("CenterY = %f, want 150", shape.CenterY)
	}
	if math.Abs(shape.RadiusX-80) > shapeDetectTolerance {
		t.Errorf("RadiusX = %f, want 80", shape.RadiusX)
	}
	if math.Abs(shape.RadiusY-40) > shapeDetectTolerance {
		t.Errorf("RadiusY = %f, want 40", shape.RadiusY)
	}
}

func TestDetectRect(t *testing.T) {
	p := NewPath()
	p.Rectangle(10, 20, 100, 50)

	shape := DetectShape(p)
	if shape.Kind != ShapeRect {
		t.Fatalf("expected ShapeRect, got %d", shape.Kind)
	}
	if math.Abs(shape.CenterX-60) > shapeDetectTolerance {
		t.Errorf("CenterX = %f, want 60", shape.CenterX)
	}
	if math.Abs(shape.CenterY-45) > shapeDetectTolerance {
		t.Errorf("CenterY = %f, want 45", shape.CenterY)
	}
	if math.Abs(shape.Width-100) > shapeDetectTolerance {
		t.Errorf("Width = %f, want 100", shape.Width)
	}
	if math.Abs(shape.Height-50) > shapeDetectTolerance {
		t.Errorf("Height = %f, want 50", shape.Height)
	}
}

func TestDetectRRect(t *testing.T) {
	p := NewPath()
	p.RoundedRectangle(10, 20, 100, 80, 15)

	shape := DetectShape(p)
	if shape.Kind != ShapeRRect {
		t.Fatalf("expected ShapeRRect, got %d", shape.Kind)
	}
	if math.Abs(shape.CenterX-60) > shapeDetectTolerance {
		t.Errorf("CenterX = %f, want 60", shape.CenterX)
	}
	if math.Abs(shape.CenterY-60) > shapeDetectTolerance {
		t.Errorf("CenterY = %f, want 60", shape.CenterY)
	}
	if math.Abs(shape.Width-100) > shapeDetectTolerance {
		t.Errorf("Width = %f, want 100", shape.Width)
	}
	if math.Abs(shape.Height-80) > shapeDetectTolerance {
		t.Errorf("Height = %f, want 80", shape.Height)
	}
	if math.Abs(shape.CornerRadius-15) > shapeDetectTolerance {
		t.Errorf("CornerRadius = %f, want 15", shape.CornerRadius)
	}
}

func TestDetectUnknownArbitraryPath(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.QuadraticTo(100, 50, 50, 100)
	p.Close()

	shape := DetectShape(p)
	if shape.Kind != ShapeUnknown {
		t.Errorf("expected ShapeUnknown for arbitrary path, got %d", shape.Kind)
	}
}

func TestDetectUnknownEmptyPath(t *testing.T) {
	p := NewPath()
	shape := DetectShape(p)
	if shape.Kind != ShapeUnknown {
		t.Errorf("expected ShapeUnknown for empty path, got %d", shape.Kind)
	}
}

func TestDetectUnknownNilPath(t *testing.T) {
	shape := DetectShape(nil)
	if shape.Kind != ShapeUnknown {
		t.Errorf("expected ShapeUnknown for nil path, got %d", shape.Kind)
	}
}

func TestDetectNonAxisAlignedRect(t *testing.T) {
	// A diamond shape (not axis-aligned) should not be detected as a rect.
	p := NewPath()
	p.MoveTo(50, 0)
	p.LineTo(100, 50)
	p.LineTo(50, 100)
	p.Close() // Only 3 lines + close = 4 elements (not 5)

	shape := DetectShape(p)
	if shape.Kind != ShapeUnknown {
		t.Errorf("expected ShapeUnknown for diamond, got %d", shape.Kind)
	}
}

func TestDetectSmallCircle(t *testing.T) {
	p := NewPath()
	p.Circle(5, 5, 2)

	shape := DetectShape(p)
	if shape.Kind != ShapeCircle {
		t.Fatalf("expected ShapeCircle for small circle, got %d", shape.Kind)
	}
	if math.Abs(shape.RadiusX-2) > shapeDetectTolerance {
		t.Errorf("RadiusX = %f, want 2", shape.RadiusX)
	}
}

func BenchmarkDetectCircle(b *testing.B) {
	p := NewPath()
	p.Circle(100, 100, 50)

	b.ReportAllocs()
	for b.Loop() {
		_ = DetectShape(p)
	}
}

func BenchmarkDetectRect(b *testing.B) {
	p := NewPath()
	p.Rectangle(10, 20, 100, 50)

	b.ReportAllocs()
	for b.Loop() {
		_ = DetectShape(p)
	}
}
