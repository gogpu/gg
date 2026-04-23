package scene_test

import (
	"sync/atomic"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// gpuTracker is a mock accelerator that counts FillShape/FillPath calls.
type gpuTracker struct {
	fillShapeCount atomic.Int32
	fillPathCount  atomic.Int32
}

func (t *gpuTracker) Name() string { return "gpu-tracker" }
func (t *gpuTracker) Init() error  { return nil }
func (t *gpuTracker) Close()       {}
func (t *gpuTracker) CanAccelerate(op gg.AcceleratedOp) bool {
	return op&(gg.AccelCircleSDF|gg.AccelRRectSDF|gg.AccelFill) != 0
}

func (t *gpuTracker) FillShape(_ gg.GPURenderTarget, _ gg.DetectedShape, _ *gg.Paint) error {
	t.fillShapeCount.Add(1)
	return gg.ErrFallbackToCPU
}

func (t *gpuTracker) StrokeShape(_ gg.GPURenderTarget, _ gg.DetectedShape, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

func (t *gpuTracker) FillPath(_ gg.GPURenderTarget, _ *gg.Path, _ *gg.Paint) error {
	t.fillPathCount.Add(1)
	return gg.ErrFallbackToCPU
}

func (t *gpuTracker) StrokePath(_ gg.GPURenderTarget, _ *gg.Path, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

func (t *gpuTracker) Flush(_ gg.GPURenderTarget) error { return nil }

// TestSceneRenderer_GPUPathInvoked verifies that scene.Renderer routes
// through the GPU accelerator when one is registered.
func TestSceneRenderer_GPUPathInvoked(t *testing.T) {
	tracker := &gpuTracker{}
	if err := gg.RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}
	defer gg.CloseAccelerator()

	s := scene.NewScene()
	b := scene.NewSceneBuilderFrom(s)
	b.FillRect(10, 10, 100, 100, scene.SolidBrush(gg.Blue))
	b.FillCircle(150, 150, 40, scene.SolidBrush(gg.Red))
	_ = b // builder wrote commands directly into s

	target := gg.NewPixmap(200, 200)
	r := scene.NewRenderer(200, 200)
	if err := r.Render(target, s); err != nil {
		t.Fatalf("Render: %v", err)
	}

	fills := tracker.fillShapeCount.Load()
	paths := tracker.fillPathCount.Load()
	if fills+paths == 0 {
		t.Error("GPU path not invoked: scene rendered CPU-only")
	} else {
		t.Logf("GPU path confirmed: FillShape=%d, FillPath=%d", fills, paths)
	}
}

// TestSceneRenderer_CPUFallbackWithoutGPU verifies CPU fallback when no
// accelerator is registered.
func TestSceneRenderer_CPUFallbackWithoutGPU(t *testing.T) {
	gg.CloseAccelerator()

	s := scene.NewScene()
	b := scene.NewSceneBuilderFrom(s)
	b.FillRect(10, 10, 50, 50, scene.SolidBrush(gg.Green))
	_ = b // builder wrote commands directly into s

	target := gg.NewPixmap(100, 100)
	r := scene.NewRenderer(100, 100)
	if err := r.Render(target, s); err != nil {
		t.Fatalf("Render: %v", err)
	}

	c := target.GetPixel(30, 30)
	if c.G < 0.5 {
		t.Errorf("CPU fallback failed: pixel (30,30) G=%.2f, want > 0.5", c.G)
	}
}
