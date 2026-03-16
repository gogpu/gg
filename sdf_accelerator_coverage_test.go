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
	// Out of bounds should not panic
	blendPixel(target, -1, 5, Red, 1.0)
	blendPixel(target, 10, 5, Red, 1.0)
	blendPixel(target, 5, -1, Red, 1.0)
	blendPixel(target, 5, 10, Red, 1.0)
}
