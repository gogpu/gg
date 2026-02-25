// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"image/color"
	"testing"
)

// TestSurfaceInterface verifies the Surface interface contract.
func TestSurfaceInterface(t *testing.T) {
	// Verify ImageSurface implements Surface
	var _ Surface = (*ImageSurface)(nil)
	var _ Surface = (*GPUSurface)(nil)
}

// TestNewPath tests path creation and basic operations.
func TestNewPath(t *testing.T) {
	p := NewPath()
	if p == nil {
		t.Fatal("NewPath returned nil")
	}
	if !p.IsEmpty() {
		t.Error("new path should be empty")
	}

	p.MoveTo(10, 20)
	if p.IsEmpty() {
		t.Error("path with MoveTo should not be empty")
	}

	pt := p.CurrentPoint()
	if pt.X != 10 || pt.Y != 20 {
		t.Errorf("CurrentPoint() = (%v, %v), want (10, 20)", pt.X, pt.Y)
	}
}

// TestPathOperations tests all path operations.
func TestPathOperations(t *testing.T) {
	p := NewPath()

	// Test LineTo
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 100)
	p.LineTo(0, 100)
	p.Close()

	if len(p.Verbs()) != 5 {
		t.Errorf("expected 5 verbs, got %d", len(p.Verbs()))
	}

	// Test QuadTo
	p.Clear()
	p.MoveTo(0, 0)
	p.QuadTo(50, -50, 100, 0)
	if len(p.Verbs()) != 2 {
		t.Errorf("expected 2 verbs after quad, got %d", len(p.Verbs()))
	}

	// Test CubicTo
	p.Clear()
	p.MoveTo(0, 0)
	p.CubicTo(33, -50, 66, -50, 100, 0)
	if len(p.Verbs()) != 2 {
		t.Errorf("expected 2 verbs after cubic, got %d", len(p.Verbs()))
	}
}

// TestPathShapes tests shape convenience methods.
func TestPathShapes(t *testing.T) {
	tests := []struct {
		name      string
		create    func(p *Path)
		minVerbs  int
		minPoints int
	}{
		{
			name:      "Rectangle",
			create:    func(p *Path) { p.Rectangle(0, 0, 100, 50) },
			minVerbs:  5, // MoveTo, 3 LineTo, Close
			minPoints: 8, // 4 points * 2 coords
		},
		{
			name:      "Circle",
			create:    func(p *Path) { p.Circle(50, 50, 25) },
			minVerbs:  6, // MoveTo, 4 CubicTo, Close
			minPoints: 26,
		},
		{
			name:      "Ellipse",
			create:    func(p *Path) { p.Ellipse(50, 50, 30, 20) },
			minVerbs:  6,
			minPoints: 26,
		},
		{
			name:      "RoundedRectangle",
			create:    func(p *Path) { p.RoundedRectangle(0, 0, 100, 50, 10) },
			minVerbs:  10, // Complex path
			minPoints: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPath()
			tt.create(p)

			if len(p.Verbs()) < tt.minVerbs {
				t.Errorf("expected at least %d verbs, got %d", tt.minVerbs, len(p.Verbs()))
			}
			if len(p.Points()) < tt.minPoints {
				t.Errorf("expected at least %d points, got %d", tt.minPoints, len(p.Points()))
			}
		})
	}
}

// TestPathClone tests path cloning.
func TestPathClone(t *testing.T) {
	p := NewPath()
	p.Rectangle(10, 20, 100, 50)

	clone := p.Clone()
	if clone == p {
		t.Error("clone should be a different instance")
	}

	if len(clone.Verbs()) != len(p.Verbs()) {
		t.Error("clone should have same number of verbs")
	}

	// Modify original
	p.LineTo(200, 200)
	if len(clone.Verbs()) == len(p.Verbs()) {
		t.Error("clone should not be affected by original modifications")
	}
}

// TestPathBounds tests bounding box calculation.
func TestPathBounds(t *testing.T) {
	p := NewPath()
	p.MoveTo(10, 20)
	p.LineTo(100, 20)
	p.LineTo(100, 80)
	p.LineTo(10, 80)
	p.Close()

	minX, minY, maxX, maxY := p.Bounds()

	if minX != 10 {
		t.Errorf("minX = %v, want 10", minX)
	}
	if minY != 20 {
		t.Errorf("minY = %v, want 20", minY)
	}
	if maxX != 100 {
		t.Errorf("maxX = %v, want 100", maxX)
	}
	if maxY != 80 {
		t.Errorf("maxY = %v, want 80", maxY)
	}
}

// TestFillStyle tests FillStyle creation and modification.
func TestFillStyle(t *testing.T) {
	style := DefaultFillStyle()

	if style.Rule != FillRuleNonZero {
		t.Errorf("default rule should be NonZero, got %v", style.Rule)
	}

	// Test WithColor
	style = style.WithColor(color.RGBA{255, 0, 0, 255})
	rVal, gVal, bVal, aVal := style.Color.RGBA()
	if rVal>>8 != 255 || gVal>>8 != 0 || bVal>>8 != 0 || aVal>>8 != 255 {
		t.Errorf("color = %v,%v,%v,%v, want 255,0,0,255", rVal>>8, gVal>>8, bVal>>8, aVal>>8)
	}

	// Test WithRule
	style = style.WithRule(FillRuleEvenOdd)
	if style.Rule != FillRuleEvenOdd {
		t.Error("rule should be EvenOdd after WithRule")
	}
}

// TestStrokeStyle tests StrokeStyle creation and modification.
func TestStrokeStyle(t *testing.T) {
	style := DefaultStrokeStyle()

	if style.Width != 1.0 {
		t.Errorf("default width should be 1.0, got %v", style.Width)
	}
	if style.Cap != LineCapButt {
		t.Errorf("default cap should be Butt, got %v", style.Cap)
	}
	if style.Join != LineJoinMiter {
		t.Errorf("default join should be Miter, got %v", style.Join)
	}

	// Test fluent API
	style = style.
		WithWidth(2.5).
		WithCap(LineCapRound).
		WithJoin(LineJoinRound).
		WithMiterLimit(2.0).
		WithDash([]float64{5, 3}, 1.0)

	if style.Width != 2.5 {
		t.Errorf("width = %v, want 2.5", style.Width)
	}
	if style.Cap != LineCapRound {
		t.Errorf("cap = %v, want Round", style.Cap)
	}
	if style.Join != LineJoinRound {
		t.Errorf("join = %v, want Round", style.Join)
	}
	if style.MiterLimit != 2.0 {
		t.Errorf("miterLimit = %v, want 2.0", style.MiterLimit)
	}
	if !style.IsDashed() {
		t.Error("should be dashed")
	}
	if len(style.DashPattern) != 2 {
		t.Errorf("dashPattern length = %d, want 2", len(style.DashPattern))
	}
}

// TestDrawImageOptions tests image drawing options.
func TestDrawImageOptions(t *testing.T) {
	opts := DefaultDrawImageOptions()

	if opts.Alpha != 1.0 {
		t.Errorf("default alpha should be 1.0, got %v", opts.Alpha)
	}
	if opts.Filter != FilterNearest {
		t.Errorf("default filter should be Nearest, got %v", opts.Filter)
	}
}

// TestPointCreation tests Point creation.
func TestPointCreation(t *testing.T) {
	p := Pt(10.5, 20.5)

	if p.X != 10.5 {
		t.Errorf("X = %v, want 10.5", p.X)
	}
	if p.Y != 20.5 {
		t.Errorf("Y = %v, want 20.5", p.Y)
	}
}

// TestSolidPattern tests SolidPattern.
func TestSolidPattern(t *testing.T) {
	pattern := SolidPattern{Color: color.RGBA{128, 64, 32, 255}}

	// ColorAt should return same color regardless of position
	c1 := pattern.ColorAt(0, 0)
	c2 := pattern.ColorAt(100, 200)

	r1, g1, b1, a1 := c1.RGBA()
	r2, g2, b2, a2 := c2.RGBA()

	if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
		t.Error("SolidPattern should return same color at all positions")
	}
}
