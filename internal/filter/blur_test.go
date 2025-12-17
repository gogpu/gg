package filter

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

func TestNewBlurFilter(t *testing.T) {
	f := NewBlurFilter(5)

	if f.RadiusX != 5 {
		t.Errorf("RadiusX = %v, want 5", f.RadiusX)
	}
	if f.RadiusY != 5 {
		t.Errorf("RadiusY = %v, want 5", f.RadiusY)
	}
}

func TestNewBlurFilterXY(t *testing.T) {
	f := NewBlurFilterXY(3, 7)

	if f.RadiusX != 3 {
		t.Errorf("RadiusX = %v, want 3", f.RadiusX)
	}
	if f.RadiusY != 7 {
		t.Errorf("RadiusY = %v, want 7", f.RadiusY)
	}
}

func TestBlurFilterExpandBounds(t *testing.T) {
	tests := []struct {
		name   string
		rx, ry float64
		input  scene.Rect
		want   scene.Rect
	}{
		{
			name:  "zero radius",
			rx:    0,
			ry:    0,
			input: scene.Rect{MinX: 10, MinY: 10, MaxX: 100, MaxY: 100},
			want:  scene.Rect{MinX: 10, MinY: 10, MaxX: 100, MaxY: 100},
		},
		{
			name:  "symmetric radius",
			rx:    5,
			ry:    5,
			input: scene.Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
			want:  scene.Rect{MinX: -15, MinY: -15, MaxX: 115, MaxY: 115}, // ceil(5*3) = 15
		},
		{
			name:  "asymmetric radius",
			rx:    3,
			ry:    10,
			input: scene.Rect{MinX: 50, MinY: 50, MaxX: 150, MaxY: 150},
			want:  scene.Rect{MinX: 41, MinY: 20, MaxX: 159, MaxY: 180}, // ceil(3*3)=9, ceil(10*3)=30
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewBlurFilterXY(tt.rx, tt.ry)
			got := f.ExpandBounds(tt.input)

			if got.MinX != tt.want.MinX || got.MinY != tt.want.MinY ||
				got.MaxX != tt.want.MaxX || got.MaxY != tt.want.MaxY {
				t.Errorf("ExpandBounds = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBlurFilterApplyZeroRadius(t *testing.T) {
	src := createTestPixmap(10, 10, gg.Red)
	dst := gg.NewPixmap(10, 10)

	f := NewBlurFilter(0)
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}

	f.Apply(src, dst, bounds)

	// With zero radius, should copy unchanged
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			got := dst.GetPixel(x, y)
			if !colorApproxEqual(got, gg.Red, 0.01) {
				t.Errorf("pixel (%d,%d) = %+v, want Red", x, y, got)
			}
		}
	}
}

func TestBlurFilterApplyNilPixmaps(t *testing.T) {
	f := NewBlurFilter(5)
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}

	// Should not panic
	f.Apply(nil, nil, bounds)
	f.Apply(gg.NewPixmap(10, 10), nil, bounds)
	f.Apply(nil, gg.NewPixmap(10, 10), bounds)
}

func TestBlurFilterApplySmallImage(t *testing.T) {
	// Create a small image with center pixel different
	src := createTestPixmap(5, 5, gg.Black)
	src.SetPixel(2, 2, gg.White)

	dst := gg.NewPixmap(5, 5)

	f := NewBlurFilter(1)
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 5, MaxY: 5}

	f.Apply(src, dst, bounds)

	// Center should be blurred (not fully white)
	center := dst.GetPixel(2, 2)
	if center.R >= 1.0 || center.R <= 0.0 {
		t.Errorf("center pixel should be partially blurred, got R=%v", center.R)
	}

	// Adjacent pixels should have some white spread to them
	adj := dst.GetPixel(2, 1)
	if adj.R <= 0.0 {
		t.Error("blur should spread to adjacent pixels")
	}
}

func TestBlurFilterApplyPreservesAlpha(t *testing.T) {
	// Create image with varying alpha
	src := gg.NewPixmap(10, 10)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			src.SetPixel(x, y, gg.RGBA2(1, 0, 0, 0.5))
		}
	}

	dst := gg.NewPixmap(10, 10)

	f := NewBlurFilter(2)
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}

	f.Apply(src, dst, bounds)

	// Interior pixels should have approximately same alpha
	// (edge effects may change corners)
	c := dst.GetPixel(5, 5)
	if c.A < 0.4 || c.A > 0.6 {
		t.Errorf("center alpha = %v, expected ~0.5", c.A)
	}
}

func TestBlurFilterApplyEdgeHandling(t *testing.T) {
	// Create image with white center
	src := createTestPixmap(20, 20, gg.White)
	dst := gg.NewPixmap(20, 20)

	f := NewBlurFilter(5)
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 20, MaxY: 20}

	f.Apply(src, dst, bounds)

	// All pixels should remain white (uniform input)
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			c := dst.GetPixel(x, y)
			if !colorApproxEqual(c, gg.White, 0.01) {
				t.Errorf("pixel (%d,%d) = %+v, expected white (uniform blur)", x, y, c)
				return
			}
		}
	}
}

func TestBlurFilterApplyEmptyBounds(t *testing.T) {
	src := createTestPixmap(10, 10, gg.Red)
	dst := gg.NewPixmap(10, 10)

	f := NewBlurFilter(5)
	bounds := scene.Rect{MinX: 5, MinY: 5, MaxX: 5, MaxY: 5} // Empty bounds

	f.Apply(src, dst, bounds)

	// Destination should remain unchanged (black/transparent)
	c := dst.GetPixel(5, 5)
	if c.A != 0 {
		t.Error("empty bounds should not modify destination")
	}
}

func TestBlurFilterApplyOnlyHorizontal(t *testing.T) {
	// Create vertical stripe
	src := createTestPixmap(20, 20, gg.Black)
	for y := 0; y < 20; y++ {
		src.SetPixel(10, y, gg.White)
	}

	dst := gg.NewPixmap(20, 20)

	f := NewBlurFilterXY(3, 0) // Only horizontal blur
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 20, MaxY: 20}

	f.Apply(src, dst, bounds)

	// Horizontal blur should spread the stripe
	c := dst.GetPixel(9, 10)
	if c.R <= 0.0 {
		t.Error("horizontal blur should spread to adjacent columns")
	}

	// But different rows should have same pattern
	c1 := dst.GetPixel(9, 5)
	c2 := dst.GetPixel(9, 15)
	if !colorApproxEqual(c1, c2, 0.01) {
		t.Errorf("horizontal-only blur should be uniform vertically: %+v vs %+v", c1, c2)
	}
}

func TestBlurFilterApplyOnlyVertical(t *testing.T) {
	// Create horizontal stripe
	src := createTestPixmap(20, 20, gg.Black)
	for x := 0; x < 20; x++ {
		src.SetPixel(x, 10, gg.White)
	}

	dst := gg.NewPixmap(20, 20)

	f := NewBlurFilterXY(0, 3) // Only vertical blur
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 20, MaxY: 20}

	f.Apply(src, dst, bounds)

	// Vertical blur should spread the stripe
	c := dst.GetPixel(10, 9)
	if c.R <= 0.0 {
		t.Error("vertical blur should spread to adjacent rows")
	}

	// But different columns should have same pattern
	c1 := dst.GetPixel(5, 9)
	c2 := dst.GetPixel(15, 9)
	if !colorApproxEqual(c1, c2, 0.01) {
		t.Errorf("vertical-only blur should be uniform horizontally: %+v vs %+v", c1, c2)
	}
}

func TestClampInt(t *testing.T) {
	tests := []struct {
		v, min, max, want int
	}{
		{5, 0, 10, 5},
		{-5, 0, 10, 0},
		{15, 0, 10, 10},
		{10, 0, 10, 10}, // at max
		{0, 0, 10, 0},   // at min
	}

	for _, tt := range tests {
		got := clampInt(tt.v, tt.min, tt.max)
		if got != tt.want {
			t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tt.v, tt.min, tt.max, got, tt.want)
		}
	}
}

func TestClampUint8(t *testing.T) {
	tests := []struct {
		v    float32
		want uint8
	}{
		{0, 0},
		{127.5, 128}, // Rounds up
		{127.4, 127}, // Rounds down
		{255, 255},
		{-10, 0},
		{300, 255},
	}

	for _, tt := range tests {
		got := clampUint8(tt.v)
		if got != tt.want {
			t.Errorf("clampUint8(%v) = %d, want %d", tt.v, got, tt.want)
		}
	}
}

// Benchmarks

func BenchmarkBlurFilter(b *testing.B) {
	sizes := []struct {
		name string
		w, h int
	}{
		{"100x100", 100, 100},
		{"500x500", 500, 500},
		{"1920x1080", 1920, 1080},
	}

	radii := []float64{1, 5, 10, 20}

	for _, size := range sizes {
		for _, r := range radii {
			name := size.name + "_r" + formatFloat(r)
			b.Run(name, func(b *testing.B) {
				src := createTestPixmap(size.w, size.h, gg.Red)
				dst := gg.NewPixmap(size.w, size.h)
				f := NewBlurFilter(r)
				bounds := scene.Rect{
					MinX: 0, MinY: 0,
					MaxX: float32(size.w), MaxY: float32(size.h),
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					f.Apply(src, dst, bounds)
				}
			})
		}
	}
}
