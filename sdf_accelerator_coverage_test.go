package gg

import (
	"errors"
	"testing"
)

// --- SDFAccelerator extended tests ---

func TestSDFAcceleratorClose(t *testing.T) {
	a := &SDFAccelerator{}
	a.Close() // should be no-op
}

func TestSDFAcceleratorFlush(t *testing.T) {
	a := &SDFAccelerator{}
	target := GPURenderTarget{
		Data:   make([]uint8, 10*10*4),
		Width:  10,
		Height: 10,
		Stride: 40,
	}
	if err := a.Flush(target); err != nil {
		t.Errorf("Flush() = %v, want nil", err)
	}
}

func TestSDFAcceleratorSetForceSDF(t *testing.T) {
	a := &SDFAccelerator{}
	if a.forceSDF {
		t.Error("forceSDF should be false by default")
	}
	a.SetForceSDF(true)
	if !a.forceSDF {
		t.Error("forceSDF should be true after SetForceSDF(true)")
	}
}

func TestSDFAcceleratorFillShapeTooSmall(t *testing.T) {
	a := &SDFAccelerator{}
	target := GPURenderTarget{
		Data:   make([]uint8, 100*100*4),
		Width:  100,
		Height: 100,
		Stride: 400,
	}
	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	// Small circle (radius 5, diameter 10 < sdfMinSize=16)
	small := DetectedShape{
		Kind:    ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 5,
		RadiusY: 5,
	}
	err := a.FillShape(target, small, paint)
	if !errors.Is(err, ErrFallbackToCPU) {
		t.Errorf("FillShape(small circle) = %v, want ErrFallbackToCPU", err)
	}
}

func TestSDFAcceleratorFillShapeTooSmallForced(t *testing.T) {
	a := &SDFAccelerator{}
	a.SetForceSDF(true)
	target := GPURenderTarget{
		Data:   make([]uint8, 100*100*4),
		Width:  100,
		Height: 100,
		Stride: 400,
	}
	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	// Small circle but forced SDF
	small := DetectedShape{
		Kind:    ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 5,
		RadiusY: 5,
	}
	err := a.FillShape(target, small, paint)
	if err != nil {
		t.Errorf("FillShape(small circle forced) = %v, want nil", err)
	}
}

func TestSDFAcceleratorStrokeShapeTooSmall(t *testing.T) {
	a := &SDFAccelerator{}
	target := GPURenderTarget{
		Data:   make([]uint8, 100*100*4),
		Width:  100,
		Height: 100,
		Stride: 400,
	}
	paint := NewPaint()
	paint.SetBrush(Solid(Red))
	paint.LineWidth = 2.0

	// Small circle
	small := DetectedShape{
		Kind:    ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 5,
		RadiusY: 5,
	}
	err := a.StrokeShape(target, small, paint)
	if !errors.Is(err, ErrFallbackToCPU) {
		t.Errorf("StrokeShape(small) = %v, want ErrFallbackToCPU", err)
	}
}

func TestSDFAcceleratorStrokeShapeTooSmallForced(t *testing.T) {
	a := &SDFAccelerator{}
	a.SetForceSDF(true)
	target := GPURenderTarget{
		Data:   make([]uint8, 100*100*4),
		Width:  100,
		Height: 100,
		Stride: 400,
	}
	paint := NewPaint()
	paint.SetBrush(Solid(Red))
	paint.LineWidth = 2.0

	small := DetectedShape{
		Kind:    ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 5,
		RadiusY: 5,
	}
	err := a.StrokeShape(target, small, paint)
	if err != nil {
		t.Errorf("StrokeShape(small forced) = %v, want nil", err)
	}
}

// --- Ellipse SDF tests ---

func TestSDFAcceleratorFillEllipse(t *testing.T) {
	a := &SDFAccelerator{}
	width, height := 200, 200
	target := GPURenderTarget{
		Data:   make([]uint8, width*height*4),
		Width:  width,
		Height: height,
		Stride: width * 4,
	}
	shape := DetectedShape{
		Kind:    ShapeEllipse,
		CenterX: 100,
		CenterY: 100,
		RadiusX: 60,
		RadiusY: 30,
	}
	paint := NewPaint()
	paint.SetBrush(Solid(Green))

	err := a.FillShape(target, shape, paint)
	if err != nil {
		t.Fatalf("FillShape(ellipse) = %v, want nil", err)
	}

	// Center should be filled
	idx := (100*width + 100) * 4
	if target.Data[idx+3] == 0 {
		t.Error("center pixel should be non-transparent after fill ellipse")
	}

	// Far corner should be empty
	if target.Data[0+3] != 0 {
		t.Errorf("corner pixel alpha = %d, want 0", target.Data[3])
	}
}

func TestSDFAcceleratorStrokeEllipse(t *testing.T) {
	a := &SDFAccelerator{}
	width, height := 200, 200
	target := GPURenderTarget{
		Data:   make([]uint8, width*height*4),
		Width:  width,
		Height: height,
		Stride: width * 4,
	}
	shape := DetectedShape{
		Kind:    ShapeEllipse,
		CenterX: 100,
		CenterY: 100,
		RadiusX: 60,
		RadiusY: 30,
	}
	paint := NewPaint()
	paint.SetBrush(Solid(Blue))
	paint.LineWidth = 3.0

	err := a.StrokeShape(target, shape, paint)
	if err != nil {
		t.Fatalf("StrokeShape(ellipse) = %v, want nil", err)
	}

	// On the ring (at major axis endpoint)
	idx := (100*width + 160) * 4
	if target.Data[idx+3] == 0 {
		t.Error("on-ring pixel should be non-transparent after stroke ellipse")
	}

	// Center should be empty (stroke only)
	idx = (100*width + 100) * 4
	if target.Data[idx+3] != 0 {
		t.Errorf("center pixel alpha = %d, want 0 for stroke-only", target.Data[idx+3])
	}
}

func TestSDFAcceleratorStrokeRRect(t *testing.T) {
	a := &SDFAccelerator{}
	width, height := 200, 200
	target := GPURenderTarget{
		Data:   make([]uint8, width*height*4),
		Width:  width,
		Height: height,
		Stride: width * 4,
	}
	shape := DetectedShape{
		Kind:         ShapeRRect,
		CenterX:      100,
		CenterY:      100,
		Width:        80,
		Height:       60,
		CornerRadius: 10,
	}
	paint := NewPaint()
	paint.SetBrush(Solid(Red))
	paint.LineWidth = 3.0

	err := a.StrokeShape(target, shape, paint)
	if err != nil {
		t.Fatalf("StrokeShape(rrect) = %v, want nil", err)
	}

	// On the border
	idx := (100*width + 140) * 4 // right edge
	if target.Data[idx+3] == 0 {
		t.Error("border pixel should be non-transparent after stroke rrect")
	}
}

func TestSDFAcceleratorStrokeRect(t *testing.T) {
	a := &SDFAccelerator{}
	width, height := 100, 100
	target := GPURenderTarget{
		Data:   make([]uint8, width*height*4),
		Width:  width,
		Height: height,
		Stride: width * 4,
	}
	shape := DetectedShape{
		Kind:    ShapeRect,
		CenterX: 50,
		CenterY: 50,
		Width:   60,
		Height:  40,
	}
	paint := NewPaint()
	paint.SetBrush(Solid(Green))
	paint.LineWidth = 2.0

	err := a.StrokeShape(target, shape, paint)
	if err != nil {
		t.Fatalf("StrokeShape(rect) = %v, want nil", err)
	}
}

// --- shapeTooSmallForSDF tests ---

func TestShapeTooSmallForSDF(t *testing.T) {
	tests := []struct {
		name  string
		shape DetectedShape
		want  bool
	}{
		{
			name:  "large circle",
			shape: DetectedShape{Kind: ShapeCircle, RadiusX: 50},
			want:  false,
		},
		{
			name:  "small circle",
			shape: DetectedShape{Kind: ShapeCircle, RadiusX: 3},
			want:  true,
		},
		{
			name:  "large ellipse",
			shape: DetectedShape{Kind: ShapeEllipse, RadiusX: 50, RadiusY: 30},
			want:  false,
		},
		{
			name:  "small ellipse",
			shape: DetectedShape{Kind: ShapeEllipse, RadiusX: 5, RadiusY: 3},
			want:  true,
		},
		{
			name:  "large rect",
			shape: DetectedShape{Kind: ShapeRect, Width: 100, Height: 50},
			want:  false,
		},
		{
			name:  "small rect",
			shape: DetectedShape{Kind: ShapeRect, Width: 10, Height: 5},
			want:  true,
		},
		{
			name:  "large rrect",
			shape: DetectedShape{Kind: ShapeRRect, Width: 80, Height: 60},
			want:  false,
		},
		{
			name:  "unknown shape",
			shape: DetectedShape{Kind: ShapeUnknown},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shapeTooSmallForSDF(tt.shape)
			if got != tt.want {
				t.Errorf("shapeTooSmallForSDF() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- getColorFromPaint tests ---

func TestGetColorFromPaint(t *testing.T) {
	t.Run("solid brush", func(t *testing.T) {
		paint := NewPaint()
		paint.SetBrush(Solid(Red))
		c := getColorFromPaint(paint)
		if c.R != 1.0 || c.G != 0 || c.B != 0 {
			t.Errorf("color = %+v, want red", c)
		}
	})

	t.Run("gradient brush", func(t *testing.T) {
		paint := NewPaint()
		grad := NewLinearGradientBrush(0, 0, 100, 0)
		grad.AddColorStop(0, Red)
		grad.AddColorStop(1, Blue)
		paint.SetBrush(grad)
		c := getColorFromPaint(paint)
		// Should return ColorAt(0,0)
		_ = c // just verify no panic
	})

	t.Run("pattern", func(t *testing.T) {
		paint := NewPaint()
		paint.Brush = nil
		paint.Pattern = NewSolidPattern(Green)
		c := getColorFromPaint(paint)
		if c.G != 1.0 {
			t.Errorf("color.G = %f, want 1.0", c.G)
		}
	})

	t.Run("nil everything", func(t *testing.T) {
		paint := NewPaint()
		paint.Brush = nil
		paint.Pattern = nil
		c := getColorFromPaint(paint)
		// Should return Black
		if c.R != 0 || c.G != 0 || c.B != 0 {
			t.Errorf("color = %+v, want black", c)
		}
	})
}

// --- blendPixel bounds test ---

func TestBlendPixelBoundsCheck(t *testing.T) {
	target := GPURenderTarget{
		Data:   make([]uint8, 10*10*4),
		Width:  10,
		Height: 10,
		Stride: 40,
	}
	// Out of bounds should not panic (nil paint = no clip/mask)
	blendPixel(target, -1, 5, Red, 1.0, nil)
	blendPixel(target, 10, 5, Red, 1.0, nil)
	blendPixel(target, 5, -1, Red, 1.0, nil)
	blendPixel(target, 5, 10, Red, 1.0, nil)
}

// --- BUG-CLIP-001: ClipCoverage in CPU SDF path ---

// TestBlendPixelClipCoverage verifies that blendPixel respects paint.ClipCoverage.
func TestBlendPixelClipCoverage(t *testing.T) {
	const w, h = 20, 20
	stride := w * 4
	target := GPURenderTarget{
		Data:   make([]uint8, w*h*4),
		Width:  w,
		Height: h,
		Stride: stride,
	}

	// Clip: only left half (x < 10) has coverage.
	paint := NewPaint()
	paint.ClipCoverage = func(x, _ float64) byte {
		if x < 10 {
			return 255
		}
		return 0
	}

	// Draw red at (5, 5) — inside clip.
	blendPixel(target, 5, 5, Red, 1.0, paint)
	idx := 5*stride + 5*4
	if target.Data[idx+0] == 0 {
		t.Error("pixel at (5,5) should be drawn (inside clip)")
	}

	// Draw red at (15, 5) — outside clip.
	blendPixel(target, 15, 5, Red, 1.0, paint)
	idx = 5*stride + 15*4
	if target.Data[idx+0] != 0 || target.Data[idx+3] != 0 {
		t.Error("pixel at (15,5) should NOT be drawn (outside clip)")
	}
}

// TestBlendPixelMaskCoverage verifies that blendPixel respects paint.MaskCoverage.
func TestBlendPixelMaskCoverage(t *testing.T) {
	const w, h = 20, 20
	stride := w * 4
	target := GPURenderTarget{
		Data:   make([]uint8, w*h*4),
		Width:  w,
		Height: h,
		Stride: stride,
	}

	// Mask: only top half (y < 10) has coverage.
	paint := NewPaint()
	paint.MaskCoverage = func(_, y int) uint8 {
		if y < 10 {
			return 255
		}
		return 0
	}

	// Draw red at (5, 5) — inside mask.
	blendPixel(target, 5, 5, Red, 1.0, paint)
	idx := 5*stride + 5*4
	if target.Data[idx+0] == 0 {
		t.Error("pixel at (5,5) should be drawn (inside mask)")
	}

	// Draw red at (5, 15) — outside mask.
	blendPixel(target, 5, 15, Red, 1.0, paint)
	idx = 15*stride + 5*4
	if target.Data[idx+0] != 0 || target.Data[idx+3] != 0 {
		t.Error("pixel at (5,15) should NOT be drawn (outside mask)")
	}
}

// TestBlendPixelPartialClip verifies that partial clip coverage attenuates the pixel.
func TestBlendPixelPartialClip(t *testing.T) {
	const w, h = 10, 10
	stride := w * 4
	target := GPURenderTarget{
		Data:   make([]uint8, w*h*4),
		Width:  w,
		Height: h,
		Stride: stride,
	}

	// 50% clip coverage everywhere.
	paint := NewPaint()
	paint.ClipCoverage = func(_, _ float64) byte {
		return 128
	}

	// Draw fully opaque red.
	blendPixel(target, 5, 5, RGBA{R: 1, G: 0, B: 0, A: 1}, 1.0, paint)
	idx := 5*stride + 5*4
	alpha := target.Data[idx+3]

	// With 50% clip, alpha should be roughly 128 (not 255).
	if alpha > 140 || alpha < 116 {
		t.Errorf("pixel alpha = %d, want ~128 (50%% clip)", alpha)
	}
}

// TestSDFCircleFillClipped verifies that CPU SDF circle rendering is clipped.
// This is the core regression test for BUG-CLIP-001.
func TestSDFCircleFillClipped(t *testing.T) {
	const w, h = 100, 100
	stride := w * 4
	target := GPURenderTarget{
		Data:   make([]uint8, w*h*4),
		Width:  w,
		Height: h,
		Stride: stride,
	}

	// Clip to left half only (x < 50).
	paint := NewPaint()
	paint.Brush = Solid(Red)
	paint.ClipCoverage = func(x, _ float64) byte {
		if x < 50 {
			return 255
		}
		return 0
	}

	// Draw a circle centered at (50, 50) with radius 30.
	// It should be clipped at x=50, so only the left half is visible.
	shape := DetectedShape{
		Kind:    ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 30,
	}

	a := &SDFAccelerator{}
	if err := a.FillShape(target, shape, paint); err != nil {
		t.Fatalf("FillShape: %v", err)
	}

	// Check: pixel at (30, 50) should be drawn (inside circle, inside clip).
	idx := 50*stride + 30*4
	if target.Data[idx+3] == 0 {
		t.Error("pixel at (30,50) should be drawn (inside circle + clip)")
	}

	// Check: pixel at (70, 50) should NOT be drawn (inside circle, outside clip).
	idx = 50*stride + 70*4
	if target.Data[idx+3] != 0 {
		t.Error("pixel at (70,50) should NOT be drawn (outside clip)")
	}
}

// TestSDFRRectFillClipped verifies that CPU SDF rounded rectangle rendering is clipped.
func TestSDFRRectFillClipped(t *testing.T) {
	const w, h = 100, 100
	stride := w * 4
	target := GPURenderTarget{
		Data:   make([]uint8, w*h*4),
		Width:  w,
		Height: h,
		Stride: stride,
	}

	// Clip to top half only (y < 50).
	paint := NewPaint()
	paint.Brush = Solid(RGBA{R: 0, G: 0, B: 1, A: 1})
	paint.ClipCoverage = func(_, y float64) byte {
		if y < 50 {
			return 255
		}
		return 0
	}

	// Rounded rect centered at (50, 50), 60x60, corner radius 5.
	shape := DetectedShape{
		Kind:         ShapeRRect,
		CenterX:      50,
		CenterY:      50,
		Width:        60,
		Height:       60,
		CornerRadius: 5,
	}

	a := &SDFAccelerator{}
	if err := a.FillShape(target, shape, paint); err != nil {
		t.Fatalf("FillShape: %v", err)
	}

	// Check: pixel at (50, 30) should be drawn (inside rrect, inside clip).
	idx := 30*stride + 50*4
	if target.Data[idx+3] == 0 {
		t.Error("pixel at (50,30) should be drawn (inside rrect + clip)")
	}

	// Check: pixel at (50, 70) should NOT be drawn (inside rrect, outside clip).
	idx = 70*stride + 50*4
	if target.Data[idx+3] != 0 {
		t.Error("pixel at (50,70) should NOT be drawn (outside clip)")
	}
}
