package gg

import (
	"errors"
	"testing"
)

func TestSDFAcceleratorName(t *testing.T) {
	a := &SDFAccelerator{}
	if a.Name() != "sdf-cpu" {
		t.Errorf("Name() = %q, want %q", a.Name(), "sdf-cpu")
	}
}

func TestSDFAcceleratorInit(t *testing.T) {
	a := &SDFAccelerator{}
	if err := a.Init(); err != nil {
		t.Errorf("Init() = %v, want nil", err)
	}
}

func TestSDFAcceleratorCanAccelerate(t *testing.T) {
	a := &SDFAccelerator{}

	tests := []struct {
		name string
		op   AcceleratedOp
		want bool
	}{
		{"circle sdf", AccelCircleSDF, true},
		{"rrect sdf", AccelRRectSDF, true},
		{"both sdf", AccelCircleSDF | AccelRRectSDF, true},
		{"fill", AccelFill, false},
		{"stroke", AccelStroke, false},
		{"scene", AccelScene, false},
		{"text", AccelText, false},
		{"image", AccelImage, false},
		{"gradient", AccelGradient, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.CanAccelerate(tt.op)
			if got != tt.want {
				t.Errorf("CanAccelerate(%d) = %v, want %v", tt.op, got, tt.want)
			}
		})
	}
}

func TestSDFAcceleratorFallback(t *testing.T) {
	a := &SDFAccelerator{}
	target := GPURenderTarget{
		Data:   make([]uint8, 100*100*4),
		Width:  100,
		Height: 100,
		Stride: 400,
	}
	p := NewPath()
	paint := NewPaint()

	// FillPath should always return ErrFallbackToCPU.
	if err := a.FillPath(target, p, paint); !errors.Is(err, ErrFallbackToCPU) {
		t.Errorf("FillPath() = %v, want ErrFallbackToCPU", err)
	}

	// StrokePath should always return ErrFallbackToCPU.
	if err := a.StrokePath(target, p, paint); !errors.Is(err, ErrFallbackToCPU) {
		t.Errorf("StrokePath() = %v, want ErrFallbackToCPU", err)
	}

	// FillShape with unknown shape should return ErrFallbackToCPU.
	unknownShape := DetectedShape{Kind: ShapeUnknown}
	if err := a.FillShape(target, unknownShape, paint); !errors.Is(err, ErrFallbackToCPU) {
		t.Errorf("FillShape(unknown) = %v, want ErrFallbackToCPU", err)
	}

	// StrokeShape with unknown shape should return ErrFallbackToCPU.
	if err := a.StrokeShape(target, unknownShape, paint); !errors.Is(err, ErrFallbackToCPU) {
		t.Errorf("StrokeShape(unknown) = %v, want ErrFallbackToCPU", err)
	}
}

func TestSDFAcceleratorFillCircle(t *testing.T) {
	a := &SDFAccelerator{}
	width, height := 100, 100
	target := GPURenderTarget{
		Data:   make([]uint8, width*height*4),
		Width:  width,
		Height: height,
		Stride: width * 4,
	}

	shape := DetectedShape{
		Kind:    ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 20,
		RadiusY: 20,
	}

	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	err := a.FillShape(target, shape, paint)
	if err != nil {
		t.Fatalf("FillShape(circle) = %v, want nil", err)
	}

	// Verify center pixel is filled (should be red, fully opaque).
	idx := (50*width + 50) * 4
	if target.Data[idx+3] == 0 {
		t.Error("center pixel alpha should be non-zero after fill circle")
	}
	if target.Data[idx+0] == 0 {
		t.Error("center pixel red should be non-zero after fill circle")
	}

	// Verify far corner is still transparent.
	idx = 0 // pixel (0, 0)
	if target.Data[idx+3] != 0 {
		t.Errorf("corner pixel (0,0) alpha = %d, want 0", target.Data[idx+3])
	}
}

func TestSDFAcceleratorStrokeCircle(t *testing.T) {
	a := &SDFAccelerator{}
	width, height := 100, 100
	target := GPURenderTarget{
		Data:   make([]uint8, width*height*4),
		Width:  width,
		Height: height,
		Stride: width * 4,
	}

	shape := DetectedShape{
		Kind:    ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 20,
		RadiusY: 20,
	}

	paint := NewPaint()
	paint.SetBrush(Solid(Blue))
	paint.LineWidth = 2.0

	err := a.StrokeShape(target, shape, paint)
	if err != nil {
		t.Fatalf("StrokeShape(circle) = %v, want nil", err)
	}

	// The stroke ring is at radius=20 from center (50,50).
	// Pixel at (70, 50) is on the ring.
	idx := (50*width + 70) * 4
	if target.Data[idx+3] == 0 {
		t.Error("on-ring pixel (70,50) alpha should be non-zero after stroke")
	}
	if target.Data[idx+2] == 0 {
		t.Error("on-ring pixel (70,50) blue should be non-zero after stroke")
	}

	// Center should be empty (stroke, not fill).
	idx = (50*width + 50) * 4
	if target.Data[idx+3] != 0 {
		t.Errorf("center pixel (50,50) alpha = %d, want 0 for stroke-only", target.Data[idx+3])
	}
}

func TestSDFAcceleratorFillRRect(t *testing.T) {
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
	paint.SetBrush(Solid(Green))

	err := a.FillShape(target, shape, paint)
	if err != nil {
		t.Fatalf("FillShape(rrect) = %v, want nil", err)
	}

	// Center should be filled.
	idx := (100*width + 100) * 4
	if target.Data[idx+3] == 0 {
		t.Error("center pixel alpha should be non-zero after fill rrect")
	}
	if target.Data[idx+1] == 0 {
		t.Error("center pixel green should be non-zero after fill rrect")
	}

	// Far corner should be empty.
	idx = 0
	if target.Data[idx+3] != 0 {
		t.Errorf("corner pixel (0,0) alpha = %d, want 0", target.Data[idx+3])
	}
}

func TestSDFAcceleratorFillRect(t *testing.T) {
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
	paint.SetBrush(Solid(White))

	err := a.FillShape(target, shape, paint)
	if err != nil {
		t.Fatalf("FillShape(rect) = %v, want nil", err)
	}

	// Center should be filled.
	idx := (50*width + 50) * 4
	if target.Data[idx+3] == 0 {
		t.Error("center pixel alpha should be non-zero after fill rect")
	}
}

func TestSDFAcceleratorAlphaBlending(t *testing.T) {
	a := &SDFAccelerator{}
	width, height := 100, 100

	// Pre-fill with solid white.
	data := make([]uint8, width*height*4)
	for i := 0; i < len(data); i += 4 {
		data[i+0] = 255
		data[i+1] = 255
		data[i+2] = 255
		data[i+3] = 255
	}

	target := GPURenderTarget{
		Data:   data,
		Width:  width,
		Height: height,
		Stride: width * 4,
	}

	// Draw a semi-transparent red circle.
	shape := DetectedShape{
		Kind:    ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 20,
		RadiusY: 20,
	}

	paint := NewPaint()
	paint.SetBrush(SolidRGBA(1, 0, 0, 0.5))

	err := a.FillShape(target, shape, paint)
	if err != nil {
		t.Fatalf("FillShape(semi-transparent) = %v, want nil", err)
	}

	// Center pixel should be blended: semi-transparent red over white.
	idx := (50*width + 50) * 4
	// Expected: R ≈ 255, G ≈ 128, B ≈ 128, A = 255
	if target.Data[idx+3] != 255 {
		t.Errorf("alpha after blending = %d, want 255", target.Data[idx+3])
	}
	// R should remain high (255 from white + red contribution).
	if target.Data[idx+0] < 200 {
		t.Errorf("red after blending = %d, want >= 200", target.Data[idx+0])
	}
	// G should be reduced (from 255 white, attenuated by red overlay).
	if target.Data[idx+1] > 200 {
		t.Errorf("green after blending = %d, want <= 200", target.Data[idx+1])
	}
}

func BenchmarkSDFAcceleratorFillCircle(b *testing.B) {
	a := &SDFAccelerator{}
	width, height := 200, 200
	target := GPURenderTarget{
		Data:   make([]uint8, width*height*4),
		Width:  width,
		Height: height,
		Stride: width * 4,
	}
	shape := DetectedShape{
		Kind:    ShapeCircle,
		CenterX: 100,
		CenterY: 100,
		RadiusX: 50,
		RadiusY: 50,
	}
	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	b.ReportAllocs()
	for b.Loop() {
		// Clear target each iteration.
		for i := range target.Data {
			target.Data[i] = 0
		}
		_ = a.FillShape(target, shape, paint)
	}
}

func BenchmarkSDFAcceleratorFillRRect(b *testing.B) {
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
		Width:        120,
		Height:       80,
		CornerRadius: 15,
	}
	paint := NewPaint()
	paint.SetBrush(Solid(Blue))

	b.ReportAllocs()
	for b.Loop() {
		for i := range target.Data {
			target.Data[i] = 0
		}
		_ = a.FillShape(target, shape, paint)
	}
}
