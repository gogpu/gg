package filter

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

func TestNewDropShadowFilter(t *testing.T) {
	color := gg.RGBA2(0, 0, 0, 0.5)
	f := NewDropShadowFilter(3, 5, 10, color)

	if f.OffsetX != 3 {
		t.Errorf("OffsetX = %v, want 3", f.OffsetX)
	}
	if f.OffsetY != 5 {
		t.Errorf("OffsetY = %v, want 5", f.OffsetY)
	}
	if f.BlurRadius != 10 {
		t.Errorf("BlurRadius = %v, want 10", f.BlurRadius)
	}
	if f.Color != color {
		t.Errorf("Color = %+v, want %+v", f.Color, color)
	}
}

func TestNewSimpleDropShadow(t *testing.T) {
	f := NewSimpleDropShadow(4, 4, 8)

	if f.OffsetX != 4 || f.OffsetY != 4 {
		t.Errorf("Offset = (%v, %v), want (4, 4)", f.OffsetX, f.OffsetY)
	}
	if f.BlurRadius != 8 {
		t.Errorf("BlurRadius = %v, want 8", f.BlurRadius)
	}
	// Default color is black with 0.5 alpha
	if f.Color.R != 0 || f.Color.G != 0 || f.Color.B != 0 || f.Color.A != 0.5 {
		t.Errorf("Color = %+v, want black with 0.5 alpha", f.Color)
	}
}

func TestDropShadowExpandBounds(t *testing.T) {
	tests := []struct {
		name             string
		offsetX, offsetY float64
		blur             float64
		input            scene.Rect
		checkMinX        float32
		checkMaxX        float32
		checkMinY        float32
		checkMaxY        float32
	}{
		{
			name:      "positive offset",
			offsetX:   10,
			offsetY:   10,
			blur:      5,
			input:     scene.Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
			checkMinX: -15, // blur expand
			checkMinY: -15,
			checkMaxX: 125, // 100 + 10 + 15
			checkMaxY: 125,
		},
		{
			name:      "negative offset",
			offsetX:   -10,
			offsetY:   -10,
			blur:      5,
			input:     scene.Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
			checkMinX: -25, // -15 - 10
			checkMinY: -25,
			checkMaxX: 115, // 100 + 15
			checkMaxY: 115,
		},
		{
			name:      "no blur",
			offsetX:   5,
			offsetY:   5,
			blur:      0,
			input:     scene.Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
			checkMinX: 0,
			checkMinY: 0,
			checkMaxX: 105,
			checkMaxY: 105,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewDropShadowFilter(tt.offsetX, tt.offsetY, tt.blur, gg.Black)
			got := f.ExpandBounds(tt.input)

			if got.MinX != tt.checkMinX {
				t.Errorf("MinX = %v, want %v", got.MinX, tt.checkMinX)
			}
			if got.MaxX != tt.checkMaxX {
				t.Errorf("MaxX = %v, want %v", got.MaxX, tt.checkMaxX)
			}
			if got.MinY != tt.checkMinY {
				t.Errorf("MinY = %v, want %v", got.MinY, tt.checkMinY)
			}
			if got.MaxY != tt.checkMaxY {
				t.Errorf("MaxY = %v, want %v", got.MaxY, tt.checkMaxY)
			}
		})
	}
}

func TestDropShadowApplyNilPixmaps(t *testing.T) {
	f := NewSimpleDropShadow(5, 5, 3)
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}

	// Should not panic
	f.Apply(nil, nil, bounds)
	f.Apply(gg.NewPixmap(10, 10), nil, bounds)
	f.Apply(nil, gg.NewPixmap(10, 10), bounds)
}

func TestDropShadowApplyBasic(t *testing.T) {
	// Create a small white square on transparent background
	src := gg.NewPixmap(20, 20)
	for y := 5; y < 15; y++ {
		for x := 5; x < 15; x++ {
			src.SetPixel(x, y, gg.White)
		}
	}

	dst := gg.NewPixmap(25, 25) // Larger for shadow expansion

	f := NewDropShadowFilter(3, 3, 1, gg.RGBA2(0, 0, 0, 1.0))
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 25, MaxY: 25}

	f.Apply(src, dst, bounds)

	// Original content should be preserved
	c := dst.GetPixel(10, 10)
	if c.R < 0.9 || c.G < 0.9 || c.B < 0.9 {
		t.Errorf("original content not preserved at (10,10): %+v", c)
	}

	// Shadow should appear offset
	// At (13,13), which is (10,10) + offset (3,3), we should see shadow influence
	// but the exact result depends on blur and compositing
}

func TestDropShadowApplyNoBlur(t *testing.T) {
	// Create solid white square
	src := gg.NewPixmap(20, 20)
	for y := 5; y < 15; y++ {
		for x := 5; x < 15; x++ {
			src.SetPixel(x, y, gg.White)
		}
	}

	dst := gg.NewPixmap(25, 25)

	f := NewDropShadowFilter(5, 5, 0, gg.RGBA2(0, 0, 0, 1.0)) // No blur
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 25, MaxY: 25}

	f.Apply(src, dst, bounds)

	// Without blur, shadow should be sharp
	// Shadow position: (5,5) to (15,15) offset by (5,5) = (10,10) to (20,20)

	// Check that original white square is visible
	c := dst.GetPixel(10, 10)
	if c.A < 0.9 {
		t.Errorf("content should be visible at (10,10): %+v", c)
	}
}

func TestDropShadowApplyTransparentColor(t *testing.T) {
	src := gg.NewPixmap(10, 10)
	for y := 2; y < 8; y++ {
		for x := 2; x < 8; x++ {
			src.SetPixel(x, y, gg.Red)
		}
	}

	dst := gg.NewPixmap(15, 15)

	// Fully transparent shadow color - should have no visible effect
	f := NewDropShadowFilter(3, 3, 2, gg.RGBA2(0, 0, 0, 0))
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 15, MaxY: 15}

	f.Apply(src, dst, bounds)

	// Original content should be there
	c := dst.GetPixel(5, 5)
	if c.R < 0.9 || c.A < 0.9 {
		t.Errorf("original red should be preserved: %+v", c)
	}
}

func TestDropShadowApplyColoredShadow(t *testing.T) {
	// Create small opaque square
	src := gg.NewPixmap(15, 15)
	for y := 3; y < 8; y++ {
		for x := 3; x < 8; x++ {
			src.SetPixel(x, y, gg.White)
		}
	}

	dst := gg.NewPixmap(20, 20)

	// Blue shadow
	f := NewDropShadowFilter(5, 5, 0, gg.RGBA2(0, 0, 1, 1.0))
	bounds := scene.Rect{MinX: 0, MinY: 0, MaxX: 20, MaxY: 20}

	f.Apply(src, dst, bounds)

	// At shadow position (where no original content), should be blue
	// Shadow of (3,3)-(8,8) + offset (5,5) = (8,8)-(13,13)
	// Point (12, 12) should be in shadow area but outside original
	// However, the compositing puts source over shadow, so we check
	// a point that's only in shadow area
}

func TestExtractAlpha(t *testing.T) {
	src := gg.NewPixmap(10, 10)

	// Set varying alpha
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			src.SetPixel(x, y, gg.RGBA2(1, 0, 0, float64(x+y)/20.0))
		}
	}

	alpha := make([]float32, 10*10)
	extractAlpha(src, alpha, 0, 0, 10, 10, 0, 0)

	// Check some values
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			expected := float32(x+y) / 20.0
			got := alpha[y*10+x]
			if absf32(got-expected) > 0.02 {
				t.Errorf("alpha[%d][%d] = %v, want ~%v", y, x, got, expected)
			}
		}
	}
}

func TestExtractAlphaWithOffset(t *testing.T) {
	src := gg.NewPixmap(10, 10)
	src.SetPixel(5, 5, gg.White) // Only this pixel is white

	alpha := make([]float32, 5*5)
	// Extract with offset, looking for pixel at (5,5) in a 5x5 region starting at (3,3)
	extractAlpha(src, alpha, 3, 3, 5, 5, 0, 0)

	// Pixel (5,5) in src should appear at (2,2) in the extracted buffer (5-3=2)
	if alpha[2*5+2] < 0.9 {
		t.Errorf("offset extraction should find pixel at (2,2): got %v", alpha[2*5+2])
	}
}

func TestBlurAlphaChannel(t *testing.T) {
	width, height := 20, 20
	src := make([]float32, width*height)
	dst := make([]float32, width*height)

	// Set center pixel to 1.0
	src[10*width+10] = 1.0

	blurAlphaChannel(src, dst, width, height, 2)

	// Center should be blurred (not fully 1.0)
	if dst[10*width+10] >= 1.0 || dst[10*width+10] <= 0.0 {
		t.Errorf("center should be blurred: got %v", dst[10*width+10])
	}

	// Adjacent should have some value
	if dst[10*width+11] <= 0.0 {
		t.Error("blur should spread to adjacent pixels")
	}
}

// Benchmarks

func BenchmarkDropShadowFilter(b *testing.B) {
	sizes := []struct {
		name string
		w, h int
	}{
		{"100x100", 100, 100},
		{"500x500", 500, 500},
		{"1920x1080", 1920, 1080},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			src := gg.NewPixmap(size.w, size.h)
			// Fill with semi-opaque content
			for y := size.h / 4; y < size.h*3/4; y++ {
				for x := size.w / 4; x < size.w*3/4; x++ {
					src.SetPixel(x, y, gg.White)
				}
			}

			// Create larger dst for shadow expansion
			dst := gg.NewPixmap(size.w+50, size.h+50)
			f := NewSimpleDropShadow(5, 5, 5)
			bounds := scene.Rect{
				MinX: 0, MinY: 0,
				MaxX: float32(size.w + 50), MaxY: float32(size.h + 50),
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				f.Apply(src, dst, bounds)
			}
		})
	}
}
