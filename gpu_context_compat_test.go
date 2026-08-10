package gg

import (
	"testing"

	"github.com/gogpu/gpucontext"
)

// legacyGPUContext implements the original per-context rendering contract.
// It intentionally omits optional text capabilities such as
// DrawGlyphMaskTextAliased: adding those methods to gpuContextOps would make
// contexts returned by third-party GPURenderContextProvider implementations
// fail the whole structural type assertion.
type legacyGPUContext struct {
	fillPathCalls  int
	glyphMaskCalls int
}

var _ gpuContextOps = (*legacyGPUContext)(nil)

func (*legacyGPUContext) FillShape(GPURenderTarget, DetectedShape, *Paint) error {
	return ErrFallbackToCPU
}

func (*legacyGPUContext) StrokeShape(GPURenderTarget, DetectedShape, *Paint) error {
	return ErrFallbackToCPU
}

func (c *legacyGPUContext) FillPath(GPURenderTarget, *Path, *Paint) error {
	c.fillPathCalls++
	return nil
}

func (*legacyGPUContext) StrokePath(GPURenderTarget, *Path, *Paint) error {
	return ErrFallbackToCPU
}

func (*legacyGPUContext) DrawText(GPURenderTarget, any, string, float64, float64, RGBA, Matrix, float64) error {
	return ErrFallbackToCPU
}

func (c *legacyGPUContext) DrawGlyphMaskText(GPURenderTarget, any, string, float64, float64, RGBA, Matrix, float64) error {
	c.glyphMaskCalls++
	return nil
}

func (*legacyGPUContext) QueueImageDraw(
	GPURenderTarget, []byte, uint64, int, int, int,
	float32, float32, float32, float32, float32, uint32, uint32,
	float32, float32, float32, float32,
) {
}

func (*legacyGPUContext) QueueGPUTextureDraw(
	GPURenderTarget, gpucontext.TextureView,
	float32, float32, float32, float32, float32, uint32, uint32,
) {
}

func (*legacyGPUContext) QueueBaseLayer(
	GPURenderTarget, gpucontext.TextureView,
	float32, float32, float32, float32, float32, uint32, uint32,
) {
}

func (*legacyGPUContext) Flush(GPURenderTarget) error                { return nil }
func (*legacyGPUContext) SetClipRect(uint32, uint32, uint32, uint32) {}
func (*legacyGPUContext) ClearClipRect()                             {}
func (*legacyGPUContext) SetClipRRect(float32, float32, float32, float32, float32) {
}
func (*legacyGPUContext) ClearClipRRect()              {}
func (*legacyGPUContext) SetClipPath(*Path)            {}
func (*legacyGPUContext) ClearClipPath()               {}
func (*legacyGPUContext) BeginFrame()                  {}
func (*legacyGPUContext) SetPipelineMode(PipelineMode) {}
func (*legacyGPUContext) SetAntiAlias(bool)            {}
func (*legacyGPUContext) PendingCount() int            { return 0 }
func (*legacyGPUContext) Close()                       {}

type legacyGPUContextProvider struct {
	*mockAccelerator
	context         *legacyGPUContext
	globalPathCalls int
}

func (p *legacyGPUContextProvider) NewGPURenderContext() any {
	return p.context
}

func (p *legacyGPUContextProvider) FillPath(GPURenderTarget, *Path, *Paint) error {
	p.globalPathCalls++
	return nil
}

func TestGPURenderContextProviderAcceptsLegacyContext(t *testing.T) {
	resetAccelerator()
	context := &legacyGPUContext{}
	provider := &legacyGPUContextProvider{
		mockAccelerator: &mockAccelerator{name: "legacy-context", canAccel: AccelFill},
		context:         context,
	}
	if err := RegisterAccelerator(provider); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}
	t.Cleanup(CloseAccelerator)

	dc := NewContext(20, 20)
	t.Cleanup(func() { _ = dc.Close() })
	dc.MoveTo(1, 1)
	dc.LineTo(18, 1)
	dc.LineTo(10, 18)
	dc.ClosePath()
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	if context.fillPathCalls != 1 {
		t.Errorf("per-context FillPath calls = %d, want 1", context.fillPathCalls)
	}
	if provider.globalPathCalls != 0 {
		t.Errorf("global FillPath calls = %d, want 0", provider.globalPathCalls)
	}
	if !dc.tryGPUGlyphMaskText("A", 2, 10) {
		t.Fatal("non-aliased glyph-mask text did not use the legacy per-context route")
	}
	if context.glyphMaskCalls != 1 {
		t.Errorf("per-context DrawGlyphMaskText calls = %d, want 1", context.glyphMaskCalls)
	}
}
