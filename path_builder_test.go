package gg

import (
	"testing"
)

func TestPathBuilder_Basic(t *testing.T) {
	path := BuildPath().
		MoveTo(0, 0).
		LineTo(100, 0).
		LineTo(100, 100).
		Close().
		Build()

	if path == nil {
		t.Fatal("expected non-nil path")
	}

	// Check path has elements
	count := len(path.Elements())
	if count != 4 { // MoveTo, LineTo, LineTo, Close
		t.Errorf("expected 4 elements, got %d", count)
	}
}

func TestPathBuilder_Shapes(t *testing.T) {
	tests := []struct {
		name     string
		builder  func() *PathBuilder
		minElems int
	}{
		{"Rect", func() *PathBuilder { return BuildPath().Rect(0, 0, 100, 100) }, 5},
		{"Circle", func() *PathBuilder { return BuildPath().Circle(50, 50, 25) }, 5},
		{"Ellipse", func() *PathBuilder { return BuildPath().Ellipse(50, 50, 30, 20) }, 5},
		{"Polygon5", func() *PathBuilder { return BuildPath().Polygon(50, 50, 25, 5) }, 6},
		{"Star5", func() *PathBuilder { return BuildPath().Star(50, 50, 30, 15, 5) }, 11},
		{"RoundRect", func() *PathBuilder { return BuildPath().RoundRect(0, 0, 100, 100, 10) }, 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.builder().Build()
			count := len(path.Elements())
			if count < tt.minElems {
				t.Errorf("expected at least %d elements, got %d", tt.minElems, count)
			}
		})
	}
}

func TestPathBuilder_Chaining(t *testing.T) {
	// Test that chaining works correctly
	path := BuildPath().
		Circle(100, 100, 50).
		Rect(200, 50, 100, 100).
		Star(400, 100, 40, 20, 5).
		Build()

	if path == nil {
		t.Fatal("expected non-nil path")
	}

	// Should have elements from all three shapes
	count := len(path.Elements())
	// Circle: 6 (MoveTo + 4 CubicTo + Close)
	// Rect: 5 (MoveTo + 3 LineTo + Close)
	// Star: 11 (MoveTo + 9 LineTo + Close)
	// Total: 22
	if count < 20 {
		t.Errorf("expected at least 20 elements from chained shapes, got %d", count)
	}
}

func TestPathBuilder_InvalidPolygon(t *testing.T) {
	// Polygon with < 3 sides should do nothing
	path := BuildPath().Polygon(50, 50, 25, 2).Build()

	count := len(path.Elements())
	if count != 0 {
		t.Errorf("expected 0 elements for invalid polygon, got %d", count)
	}
}

func TestPathBuilder_InvalidStar(t *testing.T) {
	// Star with < 3 points should do nothing
	path := BuildPath().Star(50, 50, 30, 15, 2).Build()

	count := len(path.Elements())
	if count != 0 {
		t.Errorf("expected 0 elements for invalid star, got %d", count)
	}
}

func TestPathBuilder_QuadTo(t *testing.T) {
	path := BuildPath().
		MoveTo(0, 0).
		QuadTo(50, 100, 100, 0).
		Build()

	if path == nil {
		t.Fatal("expected non-nil path")
	}

	count := len(path.Elements())
	if count != 2 { // MoveTo, QuadTo
		t.Errorf("expected 2 elements, got %d", count)
	}
}

func TestPathBuilder_CubicTo(t *testing.T) {
	path := BuildPath().
		MoveTo(0, 0).
		CubicTo(25, 100, 75, 100, 100, 0).
		Build()

	if path == nil {
		t.Fatal("expected non-nil path")
	}

	count := len(path.Elements())
	if count != 2 { // MoveTo, CubicTo
		t.Errorf("expected 2 elements, got %d", count)
	}
}

func TestPathBuilder_PathAlias(t *testing.T) {
	builder := BuildPath().MoveTo(0, 0).LineTo(100, 100)

	// Both Build() and Path() should return the same path
	pathFromBuild := builder.Build()
	pathFromPath := builder.Path()

	if pathFromBuild != pathFromPath {
		t.Error("Build() and Path() should return the same path")
	}
}

func TestPathBuilder_RoundRectRadiusClamping(t *testing.T) {
	// Radius larger than half the smaller dimension should be clamped
	path := BuildPath().RoundRect(0, 0, 100, 50, 100).Build()

	if path == nil {
		t.Fatal("expected non-nil path")
	}

	// Should still produce a valid path (essentially a pill shape)
	count := len(path.Elements())
	if count < 9 {
		t.Errorf("expected at least 9 elements for rounded rect, got %d", count)
	}
}

func TestPathBuilder_EmptyPath(t *testing.T) {
	path := BuildPath().Build()

	if path == nil {
		t.Fatal("expected non-nil path")
	}

	count := len(path.Elements())
	if count != 0 {
		t.Errorf("expected 0 elements for empty path, got %d", count)
	}
}
