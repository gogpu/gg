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
	got := selectPipeline(stats, false)
	if got != PipelineModeRenderPass {
		t.Errorf("selectPipeline(complex, noCompute) = %v, want RenderPass", got)
	}
}

func TestSelectPipelineSimpleScene(t *testing.T) {
	// < 10 shapes and shallow clips -> RenderPass
	stats := SceneStats{ShapeCount: 5, ClipDepth: 1}
	got := selectPipeline(stats, true)
	if got != PipelineModeRenderPass {
		t.Errorf("selectPipeline(simple) = %v, want RenderPass", got)
	}
}

func TestSelectPipelineComplexScene(t *testing.T) {
	// > 50 shapes -> Compute
	stats := SceneStats{ShapeCount: 60}
	got := selectPipeline(stats, true)
	if got != PipelineModeCompute {
		t.Errorf("selectPipeline(60 shapes) = %v, want Compute", got)
	}
}

func TestSelectPipelineDeepClips(t *testing.T) {
	// ClipDepth > 3 -> Compute
	stats := SceneStats{ShapeCount: 20, ClipDepth: 5}
	got := selectPipeline(stats, true)
	if got != PipelineModeCompute {
		t.Errorf("selectPipeline(deep clips) = %v, want Compute", got)
	}
}

func TestSelectPipelineHighOverlap(t *testing.T) {
	// OverlapFactor > 0.5 -> Compute
	stats := SceneStats{ShapeCount: 30, OverlapFactor: 0.7}
	got := selectPipeline(stats, true)
	if got != PipelineModeCompute {
		t.Errorf("selectPipeline(high overlap) = %v, want Compute", got)
	}
}

func TestSelectPipelineTextHeavy(t *testing.T) {
	// TextRatio > 0.6 -> RenderPass (MSDF Text tier)
	stats := SceneStats{ShapeCount: 10, TextCount: 20}
	got := selectPipeline(stats, true)
	if got != PipelineModeRenderPass {
		t.Errorf("selectPipeline(text heavy) = %v, want RenderPass", got)
	}
}

func TestSelectPipelineMediumComplexity(t *testing.T) {
	// 10-50 shapes, no text, moderate clips -> Compute (default)
	stats := SceneStats{ShapeCount: 30, PathCount: 10, ClipDepth: 2}
	got := selectPipeline(stats, true)
	if got != PipelineModeCompute {
		t.Errorf("selectPipeline(medium) = %v, want Compute", got)
	}
}
