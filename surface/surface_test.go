// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"image"
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

// --- Registry tests ---

func TestRegistryRegisterAndList(t *testing.T) {
	reg := NewRegistry()

	factory := func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}

	reg.Register("test-backend", 50, factory, nil)

	names := reg.List()
	if len(names) != 1 || names[0] != "test-backend" {
		t.Errorf("List() = %v, want [test-backend]", names)
	}
}

func TestRegistryGet(t *testing.T) {
	reg := NewRegistry()

	factory := func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}

	reg.Register("my-backend", 10, factory, nil)

	entry, ok := reg.Get("my-backend")
	if !ok {
		t.Fatal("Get returned false for registered backend")
	}
	if entry.Name != "my-backend" {
		t.Errorf("entry.Name = %q, want %q", entry.Name, "my-backend")
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("Get returned true for unregistered backend")
	}
}

func TestRegistryUnregisterExtended(t *testing.T) {
	reg := NewRegistry()

	factory := func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}

	reg.Register("temp-backend", 10, factory, nil)
	reg.Unregister("temp-backend")

	_, ok := reg.Get("temp-backend")
	if ok {
		t.Error("Get returned true after Unregister")
	}
}

func TestRegistryNewSurfaceExtended(t *testing.T) {
	reg := NewRegistry()

	factory := func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}

	reg.Register("sw", 10, factory, nil)

	s, err := reg.NewSurface(Options{Width: 100, Height: 100})
	if err != nil {
		t.Fatalf("NewSurface: %v", err)
	}
	if s == nil {
		t.Fatal("NewSurface returned nil")
	}
	_ = s.Close()
}

func TestRegistryNewSurfaceByNameExtended(t *testing.T) {
	reg := NewRegistry()

	factory := func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}

	reg.Register("named-sw", 10, factory, nil)

	s, err := reg.NewSurfaceByName("named-sw", Options{Width: 50, Height: 50})
	if err != nil {
		t.Fatalf("NewSurfaceByName: %v", err)
	}
	if s == nil {
		t.Fatal("NewSurfaceByName returned nil")
	}
	_ = s.Close()

	// Non-existent should error
	_, err = reg.NewSurfaceByName("nonexistent", Options{Width: 50, Height: 50})
	if err == nil {
		t.Error("NewSurfaceByName should error for unknown backend")
	}
}

func TestRegistryPriority(t *testing.T) {
	reg := NewRegistry()

	factory := func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}

	reg.Register("low", 10, factory, nil)
	reg.Register("high", 100, factory, nil)
	reg.Register("medium", 50, factory, nil)

	names := reg.List()
	if len(names) != 3 {
		t.Fatalf("List() returned %d entries, want 3", len(names))
	}
	if names[0] != "high" {
		t.Errorf("first entry = %q, want %q (highest priority)", names[0], "high")
	}
}

func TestRegistryAvailability(t *testing.T) {
	reg := NewRegistry()

	factory := func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}

	reg.Register("available", 50, factory, func() bool { return true })
	reg.Register("unavailable", 100, factory, func() bool { return false })

	available := reg.Available()
	if len(available) != 1 || available[0] != "available" {
		t.Errorf("Available() = %v, want [available]", available)
	}
}

func TestGlobalRegistryFunctions(t *testing.T) {
	// Test global wrappers (they use globalRegistry)
	names := List()
	_ = names // just verify it doesn't panic

	avail := Available()
	_ = avail
}

// --- FillStyle/StrokeStyle extended tests ---

func TestFillStyleWithPattern(t *testing.T) {
	style := DefaultFillStyle()
	pat := SolidPattern{Color: color.RGBA{255, 0, 0, 255}}
	style = style.WithPattern(&pat)
	if style.Pattern == nil {
		t.Error("expected pattern to be set")
	}
}

func TestStrokeStyleWithColor(t *testing.T) {
	style := DefaultStrokeStyle()
	style = style.WithColor(color.RGBA{0, 255, 0, 255})
	if style.Color == nil {
		t.Error("expected color to be set")
	}
}

func TestStrokeStyleWithPattern(t *testing.T) {
	style := DefaultStrokeStyle()
	pat := SolidPattern{Color: color.RGBA{0, 0, 255, 255}}
	style = style.WithPattern(&pat)
	if style.Pattern == nil {
		t.Error("expected pattern to be set")
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions(800, 600)
	if opts.Width != 800 || opts.Height != 600 {
		t.Errorf("DefaultOptions = %dx%d, want 800x600", opts.Width, opts.Height)
	}
	if !opts.Antialias {
		t.Error("expected Antialias=true by default")
	}
}

// --- Path extended tests ---

func TestPathArc(t *testing.T) {
	p := NewPath()
	p.Arc(100, 100, 50, 0, 3.14159)
	if p.IsEmpty() {
		t.Error("arc path should not be empty")
	}
}

func TestPathBoundsEmpty(t *testing.T) {
	p := NewPath()
	minX, minY, maxX, maxY := p.Bounds()
	if minX != 0 || minY != 0 || maxX != 0 || maxY != 0 {
		t.Errorf("empty path bounds = (%v,%v,%v,%v), want (0,0,0,0)", minX, minY, maxX, maxY)
	}
}

// --- ImageSurface extended ---

func TestImageSurfaceDrawImageWithAlpha(t *testing.T) {
	s := NewImageSurface(100, 100)
	defer func() { _ = s.Close() }()

	s.Clear(color.White)

	// Create source image
	src := newTestImageRGBA(20, 20, color.RGBA{255, 0, 0, 255})

	// Draw with alpha < 1.0
	s.DrawImage(src, Pt(10, 10), &DrawImageOptions{
		Alpha: 0.5,
	})

	img := s.Snapshot()
	c := img.RGBAAt(15, 15)
	// Should be blended (not pure red due to 50% alpha)
	if c.R == 255 && c.G == 0 {
		t.Error("expected alpha-blended pixel, got pure red")
	}
}

func TestImageSurfaceDrawImageWithSrcRect(t *testing.T) {
	s := NewImageSurface(100, 100)
	defer func() { _ = s.Close() }()

	s.Clear(color.White)

	// Create 20x20 source image
	src := newTestImageRGBA(20, 20, color.RGBA{0, 0, 255, 255})

	// Draw only a portion
	srcRect := newRect(5, 5, 10, 10)
	s.DrawImage(src, Pt(30, 30), &DrawImageOptions{
		SrcRect: &srcRect,
	})
}

func TestImageSurfaceStrokeWithCurves(t *testing.T) {
	s := NewImageSurface(100, 100)
	defer func() { _ = s.Close() }()

	s.Clear(color.White)

	path := NewPath()
	path.MoveTo(10, 50)
	path.QuadTo(50, 10, 90, 50)
	s.Stroke(path, StrokeStyle{
		Color: color.RGBA{255, 0, 0, 255},
		Width: 3,
	})
}

func TestImageSurfaceStrokeWithCubicCurves(t *testing.T) {
	s := NewImageSurface(100, 100)
	defer func() { _ = s.Close() }()

	s.Clear(color.White)

	path := NewPath()
	path.MoveTo(10, 50)
	path.CubicTo(30, 10, 70, 90, 90, 50)
	s.Stroke(path, StrokeStyle{
		Color: color.RGBA{0, 255, 0, 255},
		Width: 2,
	})
}

func TestImageSurfaceSnapshotAfterClose(t *testing.T) {
	s := NewImageSurface(10, 10)
	_ = s.Close()
	snap := s.Snapshot()
	if snap != nil {
		t.Error("Snapshot after Close should return nil")
	}
}

func TestImageSurfaceNilPathFill(t *testing.T) {
	s := NewImageSurface(10, 10)
	defer func() { _ = s.Close() }()
	s.Fill(nil, FillStyle{Color: color.Black})
}

func TestImageSurfaceEmptyPathFill(t *testing.T) {
	s := NewImageSurface(10, 10)
	defer func() { _ = s.Close() }()
	s.Fill(NewPath(), FillStyle{Color: color.Black})
}

func TestImageSurfaceNilPathStroke(t *testing.T) {
	s := NewImageSurface(10, 10)
	defer func() { _ = s.Close() }()
	s.Stroke(nil, StrokeStyle{Color: color.Black, Width: 2})
}

func TestImageSurfaceNilImageDraw(t *testing.T) {
	s := NewImageSurface(10, 10)
	defer func() { _ = s.Close() }()
	s.DrawImage(nil, Pt(0, 0), nil)
}

func TestImageSurfaceDrawImageOutOfBounds(t *testing.T) {
	s := NewImageSurface(10, 10)
	defer func() { _ = s.Close() }()

	src := newTestImageRGBA(5, 5, color.RGBA{255, 0, 0, 255})

	// Draw partially out of bounds - should not panic
	s.DrawImage(src, Pt(-3, -3), nil)
	s.DrawImage(src, Pt(8, 8), nil)
}

func TestImageSurfaceStrokeZeroWidth(t *testing.T) {
	s := NewImageSurface(100, 100)
	defer func() { _ = s.Close() }()

	path := NewPath()
	path.MoveTo(10, 50)
	path.LineTo(90, 50)
	// Zero width should result in nil stroke path
	s.Stroke(path, StrokeStyle{
		Color: color.Black,
		Width: 0,
	})
}

// --- SolidPattern tests ---

// helper: create test image
func newTestImageRGBA(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func newRect(x, y, w, h int) image.Rectangle {
	return image.Rect(x, y, x+w, y+h)
}

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
