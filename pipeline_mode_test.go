package gg

import "testing"

func TestPipelineModeString(t *testing.T) {
	tests := []struct {
		name string
		mode PipelineMode
		want string
	}{
		{"Auto", PipelineModeAuto, "Auto"},
		{"RenderPass", PipelineModeRenderPass, "RenderPass"},
		{"Compute", PipelineModeCompute, "Compute"},
		{"Unknown", PipelineMode(99), "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("PipelineMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestPipelineModeDefault(t *testing.T) {
	dc := NewContext(100, 100)
	if got := dc.PipelineMode(); got != PipelineModeAuto {
		t.Errorf("new context PipelineMode() = %v, want PipelineModeAuto", got)
	}
}

func TestPipelineModeSetGet(t *testing.T) {
	dc := NewContext(100, 100)

	dc.SetPipelineMode(PipelineModeCompute)
	if got := dc.PipelineMode(); got != PipelineModeCompute {
		t.Errorf("after SetPipelineMode(Compute): got %v, want PipelineModeCompute", got)
	}

	dc.SetPipelineMode(PipelineModeRenderPass)
	if got := dc.PipelineMode(); got != PipelineModeRenderPass {
		t.Errorf("after SetPipelineMode(RenderPass): got %v, want PipelineModeRenderPass", got)
	}

	dc.SetPipelineMode(PipelineModeAuto)
	if got := dc.PipelineMode(); got != PipelineModeAuto {
		t.Errorf("after SetPipelineMode(Auto): got %v, want PipelineModeAuto", got)
	}
}

func TestWithPipelineMode(t *testing.T) {
	tests := []struct {
		name string
		mode PipelineMode
	}{
		{"Auto", PipelineModeAuto},
		{"RenderPass", PipelineModeRenderPass},
		{"Compute", PipelineModeCompute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := NewContext(100, 100, WithPipelineMode(tt.mode))
			if got := dc.PipelineMode(); got != tt.mode {
				t.Errorf("NewContext with WithPipelineMode(%v): got %v", tt.mode, got)
			}
		})
	}
}

func TestSelectPipelineNoCompute(t *testing.T) {
	// Without compute support, always returns RenderPass regardless of stats.
	stats := SceneStats{ShapeCount: 100, ClipDepth: 10, OverlapFactor: 0.9}
	got := SelectPipeline(stats, false)
	if got != PipelineModeRenderPass {
		t.Errorf("SelectPipeline(complex, noCompute) = %v, want RenderPass", got)
	}
}

func TestSelectPipelineSimpleScene(t *testing.T) {
	// < 10 shapes and shallow clips -> RenderPass
	stats := SceneStats{ShapeCount: 5, ClipDepth: 1}
	got := SelectPipeline(stats, true)
	if got != PipelineModeRenderPass {
		t.Errorf("SelectPipeline(simple) = %v, want RenderPass", got)
	}
}

func TestSelectPipelineComplexScene(t *testing.T) {
	// > 50 shapes -> Compute
	stats := SceneStats{ShapeCount: 60}
	got := SelectPipeline(stats, true)
	if got != PipelineModeCompute {
		t.Errorf("SelectPipeline(60 shapes) = %v, want Compute", got)
	}
}

func TestSelectPipelineDeepClips(t *testing.T) {
	// ClipDepth > 3 -> Compute
	stats := SceneStats{ShapeCount: 20, ClipDepth: 5}
	got := SelectPipeline(stats, true)
	if got != PipelineModeCompute {
		t.Errorf("SelectPipeline(deep clips) = %v, want Compute", got)
	}
}

func TestSelectPipelineHighOverlap(t *testing.T) {
	// OverlapFactor > 0.5 -> Compute
	stats := SceneStats{ShapeCount: 30, OverlapFactor: 0.7}
	got := SelectPipeline(stats, true)
	if got != PipelineModeCompute {
		t.Errorf("SelectPipeline(high overlap) = %v, want Compute", got)
	}
}

func TestSelectPipelineTextHeavy(t *testing.T) {
	// TextRatio > 0.6 -> RenderPass (MSDF Text tier)
	stats := SceneStats{ShapeCount: 10, TextCount: 20}
	got := SelectPipeline(stats, true)
	if got != PipelineModeRenderPass {
		t.Errorf("SelectPipeline(text heavy) = %v, want RenderPass", got)
	}
}

func TestSelectPipelineMediumComplexity(t *testing.T) {
	// 10-50 shapes, no text, moderate clips -> Compute (default)
	stats := SceneStats{ShapeCount: 30, PathCount: 10, ClipDepth: 2}
	got := SelectPipeline(stats, true)
	if got != PipelineModeCompute {
		t.Errorf("SelectPipeline(medium) = %v, want Compute", got)
	}
}

// --- Pipeline mode propagation tests ---

// mockPipelineAwareAccel is a test accelerator that records pipeline mode changes.
type mockPipelineAwareAccel struct {
	mode         PipelineMode
	initCalled   bool
	canCompute   bool
	fillCount    int
	fillPathMode string // "shape" or "path" — records which path was taken
}

func (m *mockPipelineAwareAccel) Name() string                        { return "mock-pipeline" }
func (m *mockPipelineAwareAccel) Init() error                         { m.initCalled = true; return nil }
func (m *mockPipelineAwareAccel) Close()                              {}
func (m *mockPipelineAwareAccel) CanAccelerate(op AcceleratedOp) bool { return true }
func (m *mockPipelineAwareAccel) FillPath(target GPURenderTarget, path *Path, paint *Paint) error {
	m.fillCount++
	m.fillPathMode = "path"
	return nil
}
func (m *mockPipelineAwareAccel) StrokePath(target GPURenderTarget, path *Path, paint *Paint) error {
	m.fillCount++
	m.fillPathMode = "path"
	return nil
}
func (m *mockPipelineAwareAccel) FillShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	m.fillCount++
	m.fillPathMode = "shape"
	return nil
}
func (m *mockPipelineAwareAccel) StrokeShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	m.fillCount++
	m.fillPathMode = "shape"
	return nil
}
func (m *mockPipelineAwareAccel) Flush(target GPURenderTarget) error { return nil }

// PipelineModeAware implementation.
func (m *mockPipelineAwareAccel) SetPipelineMode(mode PipelineMode) {
	m.mode = mode
}

// ComputePipelineAware implementation.
func (m *mockPipelineAwareAccel) CanCompute() bool {
	return m.canCompute
}

func TestSetPipelineModePropagation(t *testing.T) {
	mock := &mockPipelineAwareAccel{}
	oldAccel := Accelerator()
	defer func() {
		// Restore previous accelerator.
		accelMu.Lock()
		accel = oldAccel
		accelMu.Unlock()
	}()

	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 100)

	// Setting pipeline mode on context should propagate to accelerator.
	dc.SetPipelineMode(PipelineModeCompute)
	if mock.mode != PipelineModeCompute {
		t.Errorf("after SetPipelineMode(Compute): mock.mode = %v, want Compute", mock.mode)
	}

	dc.SetPipelineMode(PipelineModeRenderPass)
	if mock.mode != PipelineModeRenderPass {
		t.Errorf("after SetPipelineMode(RenderPass): mock.mode = %v, want RenderPass", mock.mode)
	}

	dc.SetPipelineMode(PipelineModeAuto)
	if mock.mode != PipelineModeAuto {
		t.Errorf("after SetPipelineMode(Auto): mock.mode = %v, want Auto", mock.mode)
	}
}

func TestComputeModeSkipsShapeDetection(t *testing.T) {
	mock := &mockPipelineAwareAccel{canCompute: true}
	oldAccel := Accelerator()
	defer func() {
		accelMu.Lock()
		accel = oldAccel
		accelMu.Unlock()
	}()

	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 100)
	dc.SetPipelineMode(PipelineModeCompute)

	// Draw a circle — would normally be detected as a shape and use SDF.
	// In Compute mode, it should go directly to FillPath (skip shape detection).
	dc.DrawCircle(50, 50, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	if mock.fillPathMode != "path" {
		t.Errorf("in Compute mode: expected fillPathMode='path' (skip shape detection), got %q", mock.fillPathMode)
	}
}

func TestRenderPassModeUsesShapeDetection(t *testing.T) {
	mock := &mockPipelineAwareAccel{canCompute: true}
	oldAccel := Accelerator()
	defer func() {
		accelMu.Lock()
		accel = oldAccel
		accelMu.Unlock()
	}()

	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 100)
	dc.SetPipelineMode(PipelineModeRenderPass)

	// Draw a circle — should be detected as shape and use FillShape.
	dc.DrawCircle(50, 50, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	if mock.fillPathMode != "shape" {
		t.Errorf("in RenderPass mode: expected fillPathMode='shape' (via shape detection), got %q", mock.fillPathMode)
	}
}

func TestAutoModeDefaultsBehavior(t *testing.T) {
	mock := &mockPipelineAwareAccel{canCompute: false}
	oldAccel := Accelerator()
	defer func() {
		accelMu.Lock()
		accel = oldAccel
		accelMu.Unlock()
	}()

	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 100)
	// Auto mode with no compute support — should use render pass path.

	dc.DrawCircle(50, 50, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	// Auto mode without compute = render pass = shape detection used.
	if mock.fillPathMode != "shape" {
		t.Errorf("in Auto mode (no compute): expected fillPathMode='shape', got %q", mock.fillPathMode)
	}
}

func TestComputeModeWithoutComputeFallsThrough(t *testing.T) {
	// If Compute is requested but CanCompute() returns false,
	// it should fall through to the render pass path.
	mock := &mockPipelineAwareAccel{canCompute: false}
	oldAccel := Accelerator()
	defer func() {
		accelMu.Lock()
		accel = oldAccel
		accelMu.Unlock()
	}()

	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 100)
	dc.SetPipelineMode(PipelineModeCompute)

	dc.DrawCircle(50, 50, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	// Compute not available — falls through to render pass (shape detection).
	if mock.fillPathMode != "shape" {
		t.Errorf("in Compute mode (no compute support): expected fallthrough to shape, got %q", mock.fillPathMode)
	}
}
