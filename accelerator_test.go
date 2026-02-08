package gg

import (
	"errors"
	"sync"
	"testing"
)

// mockAccelerator implements GPUAccelerator for testing.
type mockAccelerator struct {
	name     string
	initErr  error
	closed   bool
	canAccel AcceleratedOp
	mu       sync.Mutex
}

func (m *mockAccelerator) Name() string { return m.name }

func (m *mockAccelerator) Init() error { return m.initErr }

func (m *mockAccelerator) Close() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
}

func (m *mockAccelerator) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *mockAccelerator) CanAccelerate(op AcceleratedOp) bool {
	return m.canAccel&op != 0
}

func (m *mockAccelerator) FillPath(_ GPURenderTarget, _ *Path, _ *Paint) error {
	return ErrFallbackToCPU
}

func (m *mockAccelerator) StrokePath(_ GPURenderTarget, _ *Path, _ *Paint) error {
	return ErrFallbackToCPU
}

func (m *mockAccelerator) FillShape(_ GPURenderTarget, _ DetectedShape, _ *Paint) error {
	return ErrFallbackToCPU
}

func (m *mockAccelerator) StrokeShape(_ GPURenderTarget, _ DetectedShape, _ *Paint) error {
	return ErrFallbackToCPU
}

// resetAccelerator clears the global accelerator state between tests.
func resetAccelerator() {
	accelMu.Lock()
	accel = nil
	accelMu.Unlock()
}

func TestRegisterAcceleratorNil(t *testing.T) {
	resetAccelerator()

	err := RegisterAccelerator(nil)
	if err == nil {
		t.Fatal("expected error when registering nil accelerator")
	}
	if err.Error() != "gg: accelerator must not be nil" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
	if Accelerator() != nil {
		t.Error("accelerator should remain nil after failed registration")
	}
}

func TestRegisterAcceleratorInitError(t *testing.T) {
	resetAccelerator()

	initErr := errors.New("GPU init failed")
	mock := &mockAccelerator{name: "failing", initErr: initErr}

	err := RegisterAccelerator(mock)
	if err == nil {
		t.Fatal("expected error when Init fails")
	}
	if !errors.Is(err, initErr) {
		t.Errorf("expected init error, got: %v", err)
	}
	if Accelerator() != nil {
		t.Error("accelerator should remain nil after Init failure")
	}
}

func TestRegisterAcceleratorSuccess(t *testing.T) {
	resetAccelerator()

	mock := &mockAccelerator{name: "test-gpu", canAccel: AccelFill | AccelStroke}
	err := RegisterAccelerator(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	a := Accelerator()
	if a == nil {
		t.Fatal("expected non-nil accelerator after registration")
	}
	if a.Name() != "test-gpu" {
		t.Errorf("expected name %q, got %q", "test-gpu", a.Name())
	}

	resetAccelerator()
}

func TestRegisterAcceleratorReplacesOld(t *testing.T) {
	resetAccelerator()

	first := &mockAccelerator{name: "first"}
	second := &mockAccelerator{name: "second"}

	if err := RegisterAccelerator(first); err != nil {
		t.Fatalf("unexpected error registering first: %v", err)
	}
	if err := RegisterAccelerator(second); err != nil {
		t.Fatalf("unexpected error registering second: %v", err)
	}

	// First accelerator should be closed.
	if !first.isClosed() {
		t.Error("expected first accelerator to be closed after replacement")
	}

	// Second should be current.
	a := Accelerator()
	if a == nil {
		t.Fatal("expected non-nil accelerator")
	}
	if a.Name() != "second" {
		t.Errorf("expected name %q, got %q", "second", a.Name())
	}

	// Second should NOT be closed.
	if second.isClosed() {
		t.Error("second accelerator should not be closed")
	}

	resetAccelerator()
}

func TestAcceleratorReturnsNilWhenNoneRegistered(t *testing.T) {
	resetAccelerator()

	a := Accelerator()
	if a != nil {
		t.Errorf("expected nil accelerator, got %v", a)
	}
}

func TestAcceleratedOpBitfield(t *testing.T) {
	tests := []struct {
		name     string
		combined AcceleratedOp
		check    AcceleratedOp
		want     bool
	}{
		{"fill in fill", AccelFill, AccelFill, true},
		{"stroke in stroke", AccelStroke, AccelStroke, true},
		{"fill in fill|stroke", AccelFill | AccelStroke, AccelFill, true},
		{"stroke in fill|stroke", AccelFill | AccelStroke, AccelStroke, true},
		{"scene not in fill|stroke", AccelFill | AccelStroke, AccelScene, false},
		{"text not in fill", AccelFill, AccelText, false},
		{"all ops combined", AccelFill | AccelStroke | AccelScene | AccelText | AccelImage | AccelGradient | AccelCircleSDF | AccelRRectSDF, AccelGradient, true},
		{"circle sdf in sdf ops", AccelCircleSDF | AccelRRectSDF, AccelCircleSDF, true},
		{"rrect sdf in sdf ops", AccelCircleSDF | AccelRRectSDF, AccelRRectSDF, true},
		{"empty has nothing", 0, AccelFill, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.combined&tt.check != 0
			if got != tt.want {
				t.Errorf("(%b & %b != 0) = %v, want %v", tt.combined, tt.check, got, tt.want)
			}
		})
	}
}

func TestCanAccelerate(t *testing.T) {
	resetAccelerator()

	mock := &mockAccelerator{
		name:     "capable",
		canAccel: AccelFill | AccelStroke | AccelScene,
	}
	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		op   AcceleratedOp
		want bool
	}{
		{"fill supported", AccelFill, true},
		{"stroke supported", AccelStroke, true},
		{"scene supported", AccelScene, true},
		{"text not supported", AccelText, false},
		{"image not supported", AccelImage, false},
		{"gradient not supported", AccelGradient, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Accelerator()
			got := a.CanAccelerate(tt.op)
			if got != tt.want {
				t.Errorf("CanAccelerate(%d) = %v, want %v", tt.op, got, tt.want)
			}
		})
	}

	resetAccelerator()
}

func TestErrFallbackToCPU(t *testing.T) {
	// Verify ErrFallbackToCPU is usable with errors.Is.
	wrapped := errors.New("gpu: texture too large")
	_ = wrapped // The sentinel itself is the base case.

	if !errors.Is(ErrFallbackToCPU, ErrFallbackToCPU) {
		t.Error("ErrFallbackToCPU should match itself with errors.Is")
	}

	// Verify it works when wrapped.
	wrappedErr := errors.Join(ErrFallbackToCPU, errors.New("detail"))
	if !errors.Is(wrappedErr, ErrFallbackToCPU) {
		t.Error("wrapped ErrFallbackToCPU should be detectable with errors.Is")
	}
}

func TestAcceleratedOpValues(t *testing.T) {
	// Verify each op has a unique power-of-two value.
	ops := []AcceleratedOp{AccelFill, AccelStroke, AccelScene, AccelText, AccelImage, AccelGradient, AccelCircleSDF, AccelRRectSDF}
	seen := make(map[AcceleratedOp]bool)
	for _, op := range ops {
		if op == 0 {
			t.Errorf("op value should not be zero")
		}
		// Verify power of two.
		if op&(op-1) != 0 {
			t.Errorf("op %d is not a power of two", op)
		}
		if seen[op] {
			t.Errorf("duplicate op value: %d", op)
		}
		seen[op] = true
	}
}

func BenchmarkAcceleratorNilCheck(b *testing.B) {
	resetAccelerator()

	b.ReportAllocs()
	for b.Loop() {
		a := Accelerator()
		if a != nil {
			b.Fatal("should be nil")
		}
	}
}

func BenchmarkAcceleratorRegistered(b *testing.B) {
	resetAccelerator()
	mock := &mockAccelerator{name: "bench"}
	if err := RegisterAccelerator(mock); err != nil {
		b.Fatalf("unexpected error: %v", err)
	}
	defer resetAccelerator()

	b.ReportAllocs()
	for b.Loop() {
		a := Accelerator()
		if a == nil {
			b.Fatal("should not be nil")
		}
	}
}

func BenchmarkCanAccelerate(b *testing.B) {
	resetAccelerator()
	mock := &mockAccelerator{name: "bench", canAccel: AccelFill | AccelStroke}
	if err := RegisterAccelerator(mock); err != nil {
		b.Fatalf("unexpected error: %v", err)
	}
	defer resetAccelerator()

	a := Accelerator()
	b.ReportAllocs()
	for b.Loop() {
		_ = a.CanAccelerate(AccelFill)
	}
}
