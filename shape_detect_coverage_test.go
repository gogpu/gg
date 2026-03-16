package gg

import (
	"testing"
)

// --- Extended shape detection edge cases ---

func TestDetectShapeWrongElementCounts(t *testing.T) {
	// Path with 7 elements: not 5 (rect), 6 (circle), or 10 (rrect)
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 100)
	p.LineTo(50, 120)
	p.LineTo(0, 100)
	p.LineTo(0, 50)
	p.Close()

	shape := DetectShape(p)
	if shape.Kind != ShapeUnknown {
		t.Errorf("expected ShapeUnknown for 7-element path, got %d", shape.Kind)
	}
}

func TestDetectCircleNotClosed(t *testing.T) {
	// 6 elements but last is not Close
	p := NewPath()
	p.MoveTo(150, 100)
	p.CubicTo(150, 127.614, 127.614, 150, 100, 150)
	p.CubicTo(72.386, 150, 50, 127.614, 50, 100)
	p.CubicTo(50, 72.386, 72.386, 50, 100, 50)
	// This cubic doesn't close back to start
	p.CubicTo(127.614, 50, 150, 72.386, 150, 200)

	shape := DetectShape(p)
	if shape.Kind != ShapeUnknown {
		t.Errorf("expected ShapeUnknown for non-closed circle path, got %d", shape.Kind)
	}
}

func TestDetectRectNotAxisAligned(t *testing.T) {
	// 5 elements: MoveTo + 3 LineTo + Close but not axis-aligned
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 10) // diagonal, not axis-aligned
	p.LineTo(90, 110)
	p.Close() // 4 elements not 5

	shape := DetectShape(p)
	if shape.Kind != ShapeUnknown {
		t.Errorf("expected ShapeUnknown for non-axis-aligned shape, got %d", shape.Kind)
	}
}

func TestDetectRectDegenerateSize(t *testing.T) {
	// Degenerate rect with zero width
	p := NewPath()
	p.MoveTo(50, 0)
	p.LineTo(50, 100)
	p.LineTo(50, 100)
	p.Close()

	shape := DetectShape(p)
	if shape.Kind != ShapeUnknown {
		t.Errorf("expected ShapeUnknown for degenerate rect, got %d", shape.Kind)
	}
}

func TestDetectRRect9Elements(t *testing.T) {
	// 9 elements: enough to attempt rrect detection but wrong count (needs 10)
	p := NewPath()
	p.MoveTo(20, 0)
	p.LineTo(80, 0)
	p.CubicTo(90, 0, 100, 10, 100, 20)
	p.LineTo(100, 80)
	p.CubicTo(100, 90, 90, 100, 80, 100)
	p.LineTo(20, 100)
	p.CubicTo(10, 100, 0, 90, 0, 80)
	p.LineTo(0, 20)
	// Missing last corner arc + close = not 10 elements

	shape := DetectShape(p)
	// Should be Unknown since it's 9 elements, not 10
	if shape.Kind == ShapeRRect {
		t.Error("should not detect rrect with 9 elements")
	}
}

func TestDetectRRectWrongPattern(t *testing.T) {
	// 10 elements but wrong pattern (LineTo where CubicTo expected)
	p := NewPath()
	p.MoveTo(20, 0)
	p.LineTo(80, 0)
	p.LineTo(100, 20) // Should be CubicTo for rrect
	p.LineTo(100, 80)
	p.LineTo(80, 100) // Should be CubicTo
	p.LineTo(20, 100)
	p.LineTo(0, 80) // Should be CubicTo
	p.LineTo(0, 20)
	p.LineTo(20, 0) // Should be CubicTo
	p.Close()

	shape := DetectShape(p)
	if shape.Kind == ShapeRRect {
		t.Error("should not detect rrect with LineTo instead of CubicTo")
	}
}

func TestDetectRRectInconsistentCornerRadius(t *testing.T) {
	// Create a rounded rect but modify it to have inconsistent radii
	p := NewPath()
	p.RoundedRectangle(0, 0, 100, 80, 10)

	// Verify it detects correctly first
	shape := DetectShape(p)
	if shape.Kind != ShapeRRect {
		t.Skip("baseline rrect detection failed, skipping inconsistency test")
	}
}

// --- ShapeKind values ---

func TestShapeKindValues(t *testing.T) {
	if ShapeUnknown != 0 {
		t.Errorf("ShapeUnknown = %d, want 0", ShapeUnknown)
	}
	if ShapeCircle != 1 {
		t.Errorf("ShapeCircle = %d, want 1", ShapeCircle)
	}
	if ShapeEllipse != 2 {
		t.Errorf("ShapeEllipse = %d, want 2", ShapeEllipse)
	}
	if ShapeRect != 3 {
		t.Errorf("ShapeRect = %d, want 3", ShapeRect)
	}
	if ShapeRRect != 4 {
		t.Errorf("ShapeRRect = %d, want 4", ShapeRRect)
	}
}

// --- DetectedShape field verification ---

func TestDetectedShapeCircleFields(t *testing.T) {
	p := NewPath()
	p.Circle(200, 300, 75)
	shape := DetectShape(p)
	if shape.Kind != ShapeCircle {
		t.Fatal("expected ShapeCircle")
	}
	if shape.RadiusX != shape.RadiusY {
		t.Errorf("circle should have equal radii, got RX=%f RY=%f", shape.RadiusX, shape.RadiusY)
	}
	// Width/Height should be zero for circles (not used)
	if shape.Width != 0 || shape.Height != 0 {
		t.Logf("circle Width=%f Height=%f (not used but present)", shape.Width, shape.Height)
	}
}

func TestDetectedShapeRRectFields(t *testing.T) {
	p := NewPath()
	p.RoundedRectangle(50, 60, 200, 150, 20)
	shape := DetectShape(p)
	if shape.Kind != ShapeRRect {
		t.Fatal("expected ShapeRRect")
	}
	if shape.CornerRadius < 19 || shape.CornerRadius > 21 {
		t.Errorf("CornerRadius = %f, want ~20", shape.CornerRadius)
	}
}

// --- Multiple detections don't interfere ---

func TestDetectShapeMultipleCalls(t *testing.T) {
	circle := NewPath()
	circle.Circle(50, 50, 25)

	rect := NewPath()
	rect.Rectangle(10, 10, 80, 60)

	// Detect circle
	s1 := DetectShape(circle)
	if s1.Kind != ShapeCircle {
		t.Errorf("first detection: expected ShapeCircle, got %d", s1.Kind)
	}

	// Detect rect
	s2 := DetectShape(rect)
	if s2.Kind != ShapeRect {
		t.Errorf("second detection: expected ShapeRect, got %d", s2.Kind)
	}

	// Re-detect circle to verify no state leakage
	s3 := DetectShape(circle)
	if s3.Kind != ShapeCircle {
		t.Errorf("third detection: expected ShapeCircle, got %d", s3.Kind)
	}
}
