package gg

import (
	"math"
	"testing"
)

// TestCustomBrushColorAt tests CustomBrush sampling.
func TestCustomBrushColorAt(t *testing.T) {
	tests := []struct {
		name  string
		brush CustomBrush
		x, y  float64
		want  RGBA
	}{
		{
			"constant color",
			NewCustomBrush(func(_, _ float64) RGBA { return Red }),
			50, 50, Red,
		},
		{
			"x-based",
			NewCustomBrush(func(x, _ float64) RGBA {
				if x > 50 {
					return Blue
				}
				return Red
			}),
			100, 0, Blue,
		},
		{
			"y-based",
			NewCustomBrush(func(_, y float64) RGBA {
				if y > 50 {
					return Green
				}
				return Red
			}),
			0, 100, Green,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.brush.ColorAt(tt.x, tt.y)
			if got != tt.want {
				t.Errorf("ColorAt(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestCustomBrushNilFunc tests that nil function returns transparent.
func TestCustomBrushNilFunc(t *testing.T) {
	brush := CustomBrush{Func: nil}
	got := brush.ColorAt(50, 50)
	if got != Transparent {
		t.Errorf("nil Func ColorAt = %v, want Transparent", got)
	}
}

// TestCustomBrushWithName tests the WithName method.
func TestCustomBrushWithName(t *testing.T) {
	brush := NewCustomBrush(func(_, _ float64) RGBA { return Red })
	named := brush.WithName("test_brush")

	if named.Name != "test_brush" {
		t.Errorf("Name = %q, want %q", named.Name, "test_brush")
	}

	// Verify function still works
	if named.ColorAt(0, 0) != Red {
		t.Error("WithName broke the color function")
	}
}

// TestCustomBrushInterface verifies CustomBrush implements Brush.
func TestCustomBrushInterface(t *testing.T) {
	var _ Brush = CustomBrush{}
	var _ Brush = NewCustomBrush(func(_, _ float64) RGBA { return Red })
}

// TestHorizontalGradient tests horizontal gradient creation.
func TestHorizontalGradient(t *testing.T) {
	gradient := HorizontalGradient(Red, Blue, 0, 100)

	tests := []struct {
		name  string
		x     float64
		wantR float64
		wantB float64
	}{
		{"left edge", 0, 1, 0},
		{"middle", 50, 0.5, 0.5},
		{"right edge", 100, 0, 1},
		{"before left (clamped)", -50, 1, 0},
		{"after right (clamped)", 150, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gradient.ColorAt(tt.x, 50)
			if !near(c.R, tt.wantR) {
				t.Errorf("x=%v: R = %v, want %v", tt.x, c.R, tt.wantR)
			}
			if !near(c.B, tt.wantB) {
				t.Errorf("x=%v: B = %v, want %v", tt.x, c.B, tt.wantB)
			}
		})
	}
}

// TestVerticalGradient tests vertical gradient creation.
func TestVerticalGradient(t *testing.T) {
	gradient := VerticalGradient(White, Black, 0, 100)

	tests := []struct {
		name    string
		y       float64
		wantRGB float64
	}{
		{"top edge", 0, 1},
		{"middle", 50, 0.5},
		{"bottom edge", 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gradient.ColorAt(50, tt.y)
			if !near(c.R, tt.wantRGB) {
				t.Errorf("y=%v: R = %v, want %v", tt.y, c.R, tt.wantRGB)
			}
		})
	}
}

// TestLinearGradient tests arbitrary linear gradient.
func TestLinearGradient(t *testing.T) {
	// Diagonal gradient from (0,0) to (100,100)
	gradient := LinearGradient(Red, Blue, 0, 0, 100, 100)

	tests := []struct {
		name  string
		x, y  float64
		wantR float64
		wantB float64
	}{
		{"origin", 0, 0, 1, 0},
		{"end", 100, 100, 0, 1},
		{"middle", 50, 50, 0.5, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gradient.ColorAt(tt.x, tt.y)
			if !near2(c.R, tt.wantR, 0.01) {
				t.Errorf("(%v,%v): R = %v, want %v", tt.x, tt.y, c.R, tt.wantR)
			}
			if !near2(c.B, tt.wantB, 0.01) {
				t.Errorf("(%v,%v): B = %v, want %v", tt.x, tt.y, c.B, tt.wantB)
			}
		})
	}
}

// TestLinearGradientZeroLength tests gradient with same start/end point.
func TestLinearGradientZeroLength(t *testing.T) {
	gradient := LinearGradient(Red, Blue, 50, 50, 50, 50)
	c := gradient.ColorAt(50, 50)
	// Should return start color
	if c != Red {
		t.Errorf("Zero length gradient = %v, want Red", c)
	}
}

// TestRadialGradient tests radial gradient creation.
func TestRadialGradient(t *testing.T) {
	gradient := RadialGradient(White, Black, 50, 50, 50)

	tests := []struct {
		name    string
		x, y    float64
		wantRGB float64
	}{
		{"center", 50, 50, 1},
		{"edge right", 100, 50, 0},
		{"edge top", 50, 0, 0},
		{"halfway", 75, 50, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gradient.ColorAt(tt.x, tt.y)
			if !near2(c.R, tt.wantRGB, 0.01) {
				t.Errorf("(%v,%v): R = %v, want %v", tt.x, tt.y, c.R, tt.wantRGB)
			}
		})
	}
}

// TestRadialGradientZeroRadius tests gradient with zero radius.
func TestRadialGradientZeroRadius(t *testing.T) {
	gradient := RadialGradient(White, Black, 50, 50, 0)
	c := gradient.ColorAt(50, 50)
	// Should return center color
	if c != White {
		t.Errorf("Zero radius gradient = %v, want White", c)
	}
}

// TestCheckerboard tests checkerboard pattern.
func TestCheckerboard(t *testing.T) {
	checker := Checkerboard(Black, White, 10)

	tests := []struct {
		name string
		x, y float64
		want RGBA
	}{
		{"origin", 0, 0, Black},
		{"first white", 10, 0, White},
		{"next black", 20, 0, Black},
		{"diag white", 10, 10, Black},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.ColorAt(tt.x, tt.y)
			if got != tt.want {
				t.Errorf("(%v,%v) = %v, want %v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestCheckerboardZeroSize tests checkerboard with zero size.
func TestCheckerboardZeroSize(t *testing.T) {
	checker := Checkerboard(Black, White, 0)
	// Should default to size 1
	c1 := checker.ColorAt(0, 0)
	c2 := checker.ColorAt(1, 0)
	if c1 == c2 {
		t.Error("Zero size should default to 1, producing alternating colors")
	}
}

// TestStripes tests stripe pattern.
func TestStripes(t *testing.T) {
	stripes := Stripes(Red, Blue, 10, 0) // Vertical stripes

	tests := []struct {
		name string
		x    float64
		want RGBA
	}{
		{"first stripe", 5, Red},
		{"second stripe", 15, Blue},
		{"third stripe", 25, Red},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripes.ColorAt(tt.x, 50)
			if got != tt.want {
				t.Errorf("x=%v: got %v, want %v", tt.x, got, tt.want)
			}
		})
	}
}

// TestStripesRotated tests rotated stripe pattern.
func TestStripesRotated(t *testing.T) {
	// 90 degree rotation = horizontal stripes
	stripes := Stripes(Red, Blue, 10, math.Pi/2)

	// At 90 degrees, the pattern is based on y, not x
	c1 := stripes.ColorAt(0, 5)
	c2 := stripes.ColorAt(0, 15)
	if c1 == c2 {
		t.Error("Rotated stripes should produce different colors at different y")
	}
}

// TestStripesZeroWidth tests stripes with zero width.
func TestStripesZeroWidth(t *testing.T) {
	stripes := Stripes(Red, Blue, 0, 0)
	// Should default to width 1
	c1 := stripes.ColorAt(0, 0)
	c2 := stripes.ColorAt(1, 0)
	if c1 == c2 {
		t.Error("Zero width should default to 1, producing alternating colors")
	}
}

// TestClampT tests the clampT helper function.
func TestClampT(t *testing.T) {
	tests := []struct {
		name string
		val  float64
		want float64
	}{
		{"below zero", -0.5, 0},
		{"zero", 0, 0},
		{"middle", 0.5, 0.5},
		{"one", 1, 1},
		{"above one", 1.5, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampT(tt.val)
			if got != tt.want {
				t.Errorf("clampT(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

// TestSolidBrushToCustomBrush tests conversion.
func TestSolidBrushToCustomBrush(t *testing.T) {
	solid := Solid(Red)
	custom := solid.toCustomBrush()

	if custom.Name != "solid" {
		t.Errorf("Name = %q, want %q", custom.Name, "solid")
	}

	// Should return same color everywhere
	if custom.ColorAt(0, 0) != Red {
		t.Error("Expected Red at origin")
	}
	if custom.ColorAt(1000, 1000) != Red {
		t.Error("Expected Red at (1000, 1000)")
	}
}

// BenchmarkCustomBrushColorAt benchmarks CustomBrush sampling.
func BenchmarkCustomBrushColorAt(b *testing.B) {
	brush := NewCustomBrush(func(x, y float64) RGBA {
		return RGBA{R: x / 100, G: y / 100, B: 0.5, A: 1}
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = brush.ColorAt(float64(i%100), float64(i%100))
	}
}

// BenchmarkHorizontalGradient benchmarks gradient sampling.
func BenchmarkHorizontalGradient(b *testing.B) {
	gradient := HorizontalGradient(Red, Blue, 0, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gradient.ColorAt(float64(i%100), 50)
	}
}

// BenchmarkRadialGradient benchmarks radial gradient sampling.
func BenchmarkRadialGradient(b *testing.B) {
	gradient := RadialGradient(White, Black, 50, 50, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := float64(i % 100)
		y := float64((i / 100) % 100)
		_ = gradient.ColorAt(x, y)
	}
}

// BenchmarkCheckerboard benchmarks checkerboard sampling.
func BenchmarkCheckerboard(b *testing.B) {
	checker := Checkerboard(Black, White, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = checker.ColorAt(float64(i%100), float64(i%100))
	}
}
