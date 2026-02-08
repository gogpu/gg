package gg

import (
	"sync"
	"testing"
)

// trackingAccelerator is a mock that tracks which methods were called.
type trackingAccelerator struct {
	mu            sync.Mutex
	fillShapeCt   int
	strokeShapeCt int
	fillPathCt    int
	strokePathCt  int
	lastShape     DetectedShape
}

func (a *trackingAccelerator) Name() string { return "tracking" }
func (a *trackingAccelerator) Init() error  { return nil }
func (a *trackingAccelerator) Close()       {}

func (a *trackingAccelerator) CanAccelerate(op AcceleratedOp) bool {
	return op&(AccelCircleSDF|AccelRRectSDF) != 0
}

func (a *trackingAccelerator) FillPath(_ GPURenderTarget, _ *Path, _ *Paint) error {
	a.mu.Lock()
	a.fillPathCt++
	a.mu.Unlock()
	return ErrFallbackToCPU
}

func (a *trackingAccelerator) StrokePath(_ GPURenderTarget, _ *Path, _ *Paint) error {
	a.mu.Lock()
	a.strokePathCt++
	a.mu.Unlock()
	return ErrFallbackToCPU
}

func (a *trackingAccelerator) FillShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	a.mu.Lock()
	a.fillShapeCt++
	a.lastShape = shape
	a.mu.Unlock()

	// Actually render using the SDF accelerator for verification.
	sdf := &SDFAccelerator{}
	return sdf.FillShape(target, shape, paint)
}

func (a *trackingAccelerator) StrokeShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	a.mu.Lock()
	a.strokeShapeCt++
	a.lastShape = shape
	a.mu.Unlock()

	sdf := &SDFAccelerator{}
	return sdf.StrokeShape(target, shape, paint)
}

func TestContextWithSDFAcceleratorFillCircle(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &trackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.SetColor(Red.Color())
	dc.DrawCircle(100, 100, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() = %v", err)
	}

	tracker.mu.Lock()
	fc := tracker.fillShapeCt
	lastKind := tracker.lastShape.Kind
	tracker.mu.Unlock()

	if fc == 0 {
		t.Error("expected FillShape to be called for circle, but it was not")
	}
	if lastKind != ShapeCircle {
		t.Errorf("expected shape kind ShapeCircle, got %d", lastKind)
	}

	// Verify pixels were actually drawn.
	px := dc.pixmap.GetPixel(100, 100)
	if px.A < 0.5 {
		t.Errorf("center pixel alpha = %f, want >= 0.5", px.A)
	}
}

func TestContextWithSDFAcceleratorStrokeCircle(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &trackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.SetColor(Blue.Color())
	dc.SetLineWidth(2.0)
	dc.DrawCircle(100, 100, 30)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke() = %v", err)
	}

	tracker.mu.Lock()
	sc := tracker.strokeShapeCt
	lastKind := tracker.lastShape.Kind
	tracker.mu.Unlock()

	if sc == 0 {
		t.Error("expected StrokeShape to be called for circle, but it was not")
	}
	if lastKind != ShapeCircle {
		t.Errorf("expected shape kind ShapeCircle, got %d", lastKind)
	}
}

func TestContextFallbackToSoftware(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &trackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	// Draw an arbitrary path that is NOT a recognized shape.
	dc.SetColor(Green.Color())
	dc.MoveTo(10, 10)
	dc.LineTo(100, 10)
	dc.QuadraticTo(100, 100, 10, 100)
	dc.ClosePath()
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() = %v", err)
	}

	// The accelerator does not support AccelFill, so it should fall back.
	tracker.mu.Lock()
	fc := tracker.fillShapeCt
	fpc := tracker.fillPathCt
	tracker.mu.Unlock()

	if fc != 0 {
		t.Errorf("expected FillShape not to be called for arbitrary path, got %d calls", fc)
	}
	// fillPathCt should be 0 because CanAccelerate(AccelFill) returns false.
	if fpc != 0 {
		t.Errorf("expected FillPath not to be called (unsupported op), got %d calls", fpc)
	}

	// Verify pixels were drawn by software renderer.
	px := dc.pixmap.GetPixel(50, 50)
	if px.A < 0.5 {
		t.Errorf("center pixel alpha = %f, want >= 0.5 (software fallback)", px.A)
	}
}

func TestContextNoAcceleratorSoftwarePath(t *testing.T) {
	resetAccelerator()

	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetColor(Red.Color())
	dc.DrawCircle(50, 50, 20)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() = %v", err)
	}

	// Verify software rendering works when no accelerator is registered.
	px := dc.pixmap.GetPixel(50, 50)
	if px.A < 0.9 {
		t.Errorf("center pixel alpha = %f, want >= 0.9", px.A)
	}
	if px.R < 0.9 {
		t.Errorf("center pixel red = %f, want >= 0.9", px.R)
	}
}

func TestContextFillPreserveWithAccelerator(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &trackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.SetColor(Red.Color())
	dc.DrawCircle(100, 100, 30)

	// FillPreserve should use accelerator but NOT clear the path.
	if err := dc.FillPreserve(); err != nil {
		t.Fatalf("FillPreserve() = %v", err)
	}

	tracker.mu.Lock()
	fc := tracker.fillShapeCt
	tracker.mu.Unlock()

	if fc == 0 {
		t.Error("expected FillShape to be called for FillPreserve")
	}

	// Path should still have elements.
	if dc.path.isEmpty() {
		t.Error("path should not be cleared after FillPreserve")
	}
}

func TestContextStrokePreserveWithAccelerator(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &trackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.SetColor(Blue.Color())
	dc.SetLineWidth(2.0)
	dc.DrawCircle(100, 100, 30)

	// StrokePreserve should use accelerator but NOT clear the path.
	if err := dc.StrokePreserve(); err != nil {
		t.Fatalf("StrokePreserve() = %v", err)
	}

	tracker.mu.Lock()
	sc := tracker.strokeShapeCt
	tracker.mu.Unlock()

	if sc == 0 {
		t.Error("expected StrokeShape to be called for StrokePreserve")
	}

	// Path should still have elements.
	if dc.path.isEmpty() {
		t.Error("path should not be cleared after StrokePreserve")
	}
}
