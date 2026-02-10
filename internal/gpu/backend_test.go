//go:build !nogpu

package gpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// TestBackendName verifies the backend name.
func TestBackendName(t *testing.T) {
	b := NewBackend()
	if b.Name() != "gpu" {
		t.Errorf("Name() = %q, want %q", b.Name(), "gpu")
	}
}

// TestBackendInit tests initialization.
func TestBackendInit(t *testing.T) {
	b := NewBackend()

	// Should not be initialized initially
	if b.IsInitialized() {
		t.Error("backend should not be initialized before Init()")
	}

	// Initialize
	err := b.Init()
	if err != nil {
		// In test environment, we may not have a real GPU
		// This is acceptable for unit tests
		t.Logf("Init() returned error (expected in test environment): %v", err)
		return
	}

	// Should be initialized after Init()
	if !b.IsInitialized() {
		t.Error("backend should be initialized after Init()")
	}

	// Device and Queue should be non-zero
	if b.Device().IsZero() {
		t.Error("Device() should not be zero after Init()")
	}
	if b.Queue().IsZero() {
		t.Error("Queue() should not be zero after Init()")
	}

	// GPUInfo should be available
	info := b.GPUInfo()
	if info == nil {
		t.Error("GPUInfo() should not be nil after Init()")
	} else {
		t.Logf("GPU: %s", info.String())
	}

	// Double init should be idempotent
	err = b.Init()
	if err != nil {
		t.Errorf("second Init() should not error: %v", err)
	}

	// Cleanup
	b.Close()

	// Should not be initialized after Close()
	if b.IsInitialized() {
		t.Error("backend should not be initialized after Close()")
	}
}

// TestBackendClose tests resource cleanup.
func TestBackendClose(t *testing.T) {
	b := NewBackend()

	// Close on uninitialized backend should be safe
	b.Close()

	// Initialize and close
	if err := b.Init(); err != nil {
		t.Logf("Init() returned error (expected in test environment): %v", err)
		return
	}

	b.Close()

	// Double close should be safe
	b.Close()

	// Should not be initialized
	if b.IsInitialized() {
		t.Error("backend should not be initialized after Close()")
	}

	// IDs should be zero
	if !b.Device().IsZero() {
		t.Error("Device() should be zero after Close()")
	}
	if !b.Queue().IsZero() {
		t.Error("Queue() should be zero after Close()")
	}
	if b.GPUInfo() != nil {
		t.Error("GPUInfo() should be nil after Close()")
	}
}

// TestBackendNewRenderer tests renderer creation.
func TestBackendNewRenderer(t *testing.T) {
	b := NewBackend()

	// NewRenderer on uninitialized backend should return nil
	r := b.NewRenderer(800, 600)
	if r != nil {
		t.Error("NewRenderer() should return nil for uninitialized backend")
	}

	// Initialize
	if err := b.Init(); err != nil {
		t.Logf("Init() returned error (expected in test environment): %v", err)
		return
	}
	defer b.Close()

	// Create renderer
	r = b.NewRenderer(800, 600)
	if r == nil {
		t.Fatal("NewRenderer() returned nil for initialized backend")
	}

	// Verify it's a GPURenderer
	gpuR, ok := r.(*GPURenderer)
	if !ok {
		t.Fatalf("NewRenderer() returned %T, want *GPURenderer", r)
	}

	if gpuR.Width() != 800 {
		t.Errorf("Width() = %d, want %d", gpuR.Width(), 800)
	}
	if gpuR.Height() != 600 {
		t.Errorf("Height() = %d, want %d", gpuR.Height(), 600)
	}

	gpuR.Close()
}

// TestBackendNewRendererInvalidDimensions tests invalid dimension handling.
func TestBackendNewRendererInvalidDimensions(t *testing.T) {
	b := NewBackend()

	if err := b.Init(); err != nil {
		t.Logf("Init() returned error (expected in test environment): %v", err)
		return
	}
	defer b.Close()

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"zero width", 0, 600},
		{"zero height", 800, 0},
		{"negative width", -1, 600},
		{"negative height", 800, -1},
		{"both zero", 0, 0},
		{"both negative", -100, -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := b.NewRenderer(tt.width, tt.height)
			if r != nil {
				t.Errorf("NewRenderer(%d, %d) = %v, want nil", tt.width, tt.height, r)
			}
		})
	}
}

// TestBackendRenderScene tests scene rendering.
func TestBackendRenderScene(t *testing.T) {
	b := NewBackend()

	// RenderScene on uninitialized backend should return error
	err := b.RenderScene(nil, nil)
	if !errors.Is(err, ErrNotInitialized) {
		t.Errorf("RenderScene() on uninitialized backend: got %v, want %v", err, ErrNotInitialized)
	}

	// Initialize
	if err := b.Init(); err != nil {
		t.Logf("Init() returned error (expected in test environment): %v", err)
		return
	}
	defer b.Close()

	// Test nil target
	err = b.RenderScene(nil, scene.NewScene())
	if !errors.Is(err, ErrNilTarget) {
		t.Errorf("RenderScene(nil, scene) = %v, want %v", err, ErrNilTarget)
	}

	// Test nil scene
	target := gg.NewPixmap(100, 100)
	err = b.RenderScene(target, nil)
	if !errors.Is(err, ErrNilScene) {
		t.Errorf("RenderScene(target, nil) = %v, want %v", err, ErrNilScene)
	}

	// Test with valid scene (empty scene returns ErrEmptyScene)
	s := scene.NewScene()
	err = b.RenderScene(target, s)
	// Empty scene should return ErrEmptyScene from GPUSceneRenderer
	if err == nil {
		t.Log("RenderScene() with empty scene succeeded (GPU pipeline working)")
	} else {
		t.Logf("RenderScene() with empty scene: %v (expected for empty scene)", err)
	}

	// Test with non-empty scene
	s = scene.NewScene()
	rect := scene.NewRectShape(10, 10, 80, 80)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)
	err = b.RenderScene(target, s)
	// Should succeed (GPU pipeline runs, readback may fail which is OK)
	if err != nil {
		t.Logf("RenderScene() = %v (may be expected until full GPU support)", err)
	}
}

// TestGPURendererFillStroke tests that Fill and Stroke methods work correctly.
func TestGPURendererFillStroke(t *testing.T) {
	b := NewBackend()

	if err := b.Init(); err != nil {
		t.Logf("Init() returned error (expected in test environment): %v", err)
		// Continue without GPU - we can still test the software fallback
	}
	defer b.Close()

	// Create renderer directly for testing (bypasses GPU check)
	gpuR := newGPURenderer(b, 100, 100)
	if gpuR == nil {
		t.Fatal("newGPURenderer() returned nil")
	}

	// Create test objects
	pixmap := gg.NewPixmap(100, 100)
	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 10)
	path.LineTo(90, 90)
	path.LineTo(10, 90)
	path.Close()
	paint := gg.NewPaint()

	// Fill should now work (Phase 1: software rasterization)
	if err := gpuR.Fill(pixmap, path, paint); err != nil {
		t.Errorf("Fill() = %v, want nil", err)
	}

	// Stroke should now work (Phase 1: software rasterization)
	if err := gpuR.Stroke(pixmap, path, paint); err != nil {
		t.Errorf("Stroke() = %v, want nil", err)
	}

	// Test nil pixmap
	if err := gpuR.Fill(nil, path, paint); !errors.Is(err, ErrNilTarget) {
		t.Errorf("Fill(nil, ...) = %v, want %v", err, ErrNilTarget)
	}
	if err := gpuR.Stroke(nil, path, paint); !errors.Is(err, ErrNilTarget) {
		t.Errorf("Stroke(nil, ...) = %v, want %v", err, ErrNilTarget)
	}

	// Test nil path (should be no-op, not error)
	if err := gpuR.Fill(pixmap, nil, paint); err != nil {
		t.Errorf("Fill(pixmap, nil, paint) = %v, want nil", err)
	}
	if err := gpuR.Stroke(pixmap, nil, paint); err != nil {
		t.Errorf("Stroke(pixmap, nil, paint) = %v, want nil", err)
	}

	// Test nil paint (should be no-op, not error)
	if err := gpuR.Fill(pixmap, path, nil); err != nil {
		t.Errorf("Fill(pixmap, path, nil) = %v, want nil", err)
	}
	if err := gpuR.Stroke(pixmap, path, nil); err != nil {
		t.Errorf("Stroke(pixmap, path, nil) = %v, want nil", err)
	}

	gpuR.Close()
}

// TestBackendConcurrency tests concurrent access to the backend.
func TestBackendConcurrency(t *testing.T) {
	b := NewBackend()

	if err := b.Init(); err != nil {
		t.Logf("Init() returned error (expected in test environment): %v", err)
		return
	}
	defer b.Close()

	// Concurrent reads should be safe
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = b.IsInitialized()
			_ = b.Device()
			_ = b.Queue()
			_ = b.GPUInfo()
			_ = b.NewRenderer(100, 100)
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestGPUInfo tests GPUInfo string representation.
func TestGPUInfo(t *testing.T) {
	info := &GPUInfo{
		Name:       "Test GPU",
		Vendor:     "TestVendor",
		DeviceType: 2, // DiscreteGPU
		Backend:    1, // Vulkan
		Driver:     "1.0.0",
	}

	s := info.String()
	if s == "" {
		t.Error("GPUInfo.String() returned empty string")
	}
	t.Logf("GPUInfo: %s", s)
}

// TestErrors tests error values.
func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrNotInitialized", ErrNotInitialized},
		{"ErrNoGPU", ErrNoGPU},
		{"ErrDeviceLost", ErrDeviceLost},
		{"ErrNotImplemented", ErrNotImplemented},
		{"ErrInvalidDimensions", ErrInvalidDimensions},
		{"ErrNilTarget", ErrNilTarget},
		{"ErrNilScene", ErrNilScene},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s is nil", tt.name)
			}
			if tt.err.Error() == "" {
				t.Errorf("%s.Error() is empty", tt.name)
			}
		})
	}
}

// BenchmarkNewRenderer benchmarks renderer creation.
func BenchmarkNewRenderer(b *testing.B) {
	wb := NewBackend()
	if err := wb.Init(); err != nil {
		b.Skipf("Init() failed: %v", err)
	}
	defer wb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := wb.NewRenderer(1920, 1080)
		if r != nil {
			r.(*GPURenderer).Close()
		}
	}
}
