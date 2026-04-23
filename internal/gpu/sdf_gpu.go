//go:build !nogpu

package gpu

import (
	"log/slog"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gpucontext"

	// Import Vulkan backend so it registers via init().
	_ "github.com/gogpu/wgpu/hal/vulkan"
)

// SDFAccelerator provides GPU-accelerated rendering using wgpu/hal render
// pipelines. It implements the gg.GPUAccelerator interface.
//
// Internally, SDFAccelerator holds a GPUShared (shared device, pipelines,
// atlas engines) and a default GPURenderContext (for backward-compatible
// single-context usage). Each gg.Context creates its own GPURenderContext
// via Shared().NewRenderContext() for isolated pending command queues.
//
// This architecture follows the enterprise pattern (Skia GrContext, Vello
// Renderer, Flutter Context): shared device + pipelines + glyph atlas,
// per-context pending commands + session + frame tracking.
type SDFAccelerator struct {
	mu sync.Mutex

	// Shared GPU resources (device, pipelines, atlas engines).
	shared *GPUShared

	// Default render context for backward-compatible single-context usage.
	// When gg.Context does not have its own GPURenderContext (legacy path),
	// operations are routed through this default context.
	defaultCtx *GPURenderContext
}

var _ gg.GPUAccelerator = (*SDFAccelerator)(nil)
var _ gg.GPURenderContextProvider = (*SDFAccelerator)(nil)
var _ gg.DirectRenderCapable = (*SDFAccelerator)(nil)
var _ gg.GPUTextAccelerator = (*SDFAccelerator)(nil)
var _ gg.GPUGlyphMaskAccelerator = (*SDFAccelerator)(nil)
var _ gg.PipelineModeAware = (*SDFAccelerator)(nil)
var _ gg.ComputePipelineAware = (*SDFAccelerator)(nil)
var _ gg.ForceSDFAware = (*SDFAccelerator)(nil)
var _ gg.ClipAware = (*SDFAccelerator)(nil)
var _ gg.RRectClipAware = (*SDFAccelerator)(nil)
var _ gg.LCDLayoutAware = (*SDFAccelerator)(nil)

// Name returns the accelerator identifier.
func (a *SDFAccelerator) Name() string { return "sdf-gpu" }

// Shared returns the GPUShared instance for creating per-context
// GPURenderContexts. This is the primary integration point for gg.Context.
func (a *SDFAccelerator) Shared() *GPUShared {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.shared
}

// NewGPURenderContext creates a new per-context GPU render context.
// Implements gg.GPURenderContextProvider.
func (a *SDFAccelerator) NewGPURenderContext() any {
	return a.shared.NewRenderContext()
}

// SetLCDLayout propagates the LCD subpixel layout to the glyph mask engine.
func (a *SDFAccelerator) SetLCDLayout(layout text.LCDLayout) {
	a.shared.SetLCDLayout(layout)
}

// SetForceSDF propagates the force-SDF flag to the CPU fallback accelerator.
func (a *SDFAccelerator) SetForceSDF(force bool) {
	a.shared.SetForceSDF(force)
}

// SetClipRect sets the scissor rect for the default render context.
func (a *SDFAccelerator) SetClipRect(x, y, w, h uint32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ensureDefaultCtx()
	a.defaultCtx.SetClipRect(x, y, w, h)
}

// ClearClipRect removes the scissor rect from the default render context.
func (a *SDFAccelerator) ClearClipRect() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ensureDefaultCtx()
	a.defaultCtx.ClearClipRect()
}

// SetClipRRect sets the rounded rectangle clip for the default render context.
func (a *SDFAccelerator) SetClipRRect(x, y, w, h, radius float32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ensureDefaultCtx()
	a.defaultCtx.SetClipRRect(x, y, w, h, radius)
}

// ClearClipRRect removes the rounded rectangle clip from the default render context.
func (a *SDFAccelerator) ClearClipRRect() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ensureDefaultCtx()
	a.defaultCtx.ClearClipRRect()
}

// CanAccelerate reports whether this accelerator supports the given operation.
func (a *SDFAccelerator) CanAccelerate(op gg.AcceleratedOp) bool {
	return op&(gg.AccelCircleSDF|gg.AccelRRectSDF|gg.AccelFill|gg.AccelStroke|gg.AccelText) != 0
}

// SetPipelineMode sets the pipeline mode for the default render context.
func (a *SDFAccelerator) SetPipelineMode(mode gg.PipelineMode) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ensureDefaultCtx()
	a.defaultCtx.SetPipelineMode(mode)
}

// CanCompute reports whether the compute pipeline is available and ready.
func (a *SDFAccelerator) CanCompute() bool {
	return a.shared.CanCompute()
}

// SceneStats returns the accumulated scene statistics from the default context.
func (a *SDFAccelerator) SceneStats() gg.SceneStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.defaultCtx == nil {
		return gg.SceneStats{}
	}
	return a.defaultCtx.SceneStats()
}

// Init initializes the accelerator. GPU device initialization is deferred.
func (a *SDFAccelerator) Init() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.shared == nil {
		a.shared = NewGPUShared()
	}
	return nil
}

// Close releases all GPU resources held by the accelerator.
func (a *SDFAccelerator) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.defaultCtx != nil {
		a.defaultCtx.Close()
		a.defaultCtx = nil
	}
	if a.shared != nil {
		a.shared.Close()
	}
}

// SetLogger sets the logger for the GPU accelerator.
func (a *SDFAccelerator) SetLogger(l *slog.Logger) {
	a.shared.SetLogger(l)
}

// SetDeviceProvider switches the accelerator to use a shared GPU device.
func (a *SDFAccelerator) SetDeviceProvider(provider gpucontext.DeviceProvider) error {
	a.mu.Lock()
	// Close default context's session since device is changing.
	if a.defaultCtx != nil {
		a.defaultCtx.Close()
		a.defaultCtx = nil
	}
	a.mu.Unlock()
	return a.shared.SetDeviceProvider(provider)
}

// CanRenderDirect reports whether the GPU accelerator can render to a surface.
func (a *SDFAccelerator) CanRenderDirect() bool {
	return a.shared.CanRenderDirect()
}

// BeginFrame resets per-frame state for the default render context.
func (a *SDFAccelerator) BeginFrame() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ensureDefaultCtx()
	a.defaultCtx.BeginFrame()
}

// DrawText queues text for GPU MSDF rendering via the default render context.
func (a *SDFAccelerator) DrawText(target gg.GPURenderTarget, face any, s string, x, y float64, color gg.RGBA, matrix gg.Matrix, deviceScale float64) error {
	a.mu.Lock()
	a.ensureDefaultCtx()
	rc := a.defaultCtx
	a.mu.Unlock()
	return rc.DrawText(target, face, s, x, y, color, matrix, deviceScale)
}

// DrawGlyphMaskText queues text for GPU glyph mask rendering via the default context.
func (a *SDFAccelerator) DrawGlyphMaskText(target gg.GPURenderTarget, face any, s string, x, y float64, color gg.RGBA, matrix gg.Matrix, deviceScale float64) error {
	a.mu.Lock()
	a.ensureDefaultCtx()
	rc := a.defaultCtx
	a.mu.Unlock()
	return rc.DrawGlyphMaskText(target, face, s, x, y, color, matrix, deviceScale)
}

// FillPath queues a filled path for GPU rendering via the default context.
func (a *SDFAccelerator) FillPath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	a.mu.Lock()
	a.ensureDefaultCtx()
	rc := a.defaultCtx
	a.mu.Unlock()
	return rc.FillPath(target, path, paint)
}

// StrokePath renders a stroked path via the default context.
func (a *SDFAccelerator) StrokePath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	a.mu.Lock()
	a.ensureDefaultCtx()
	rc := a.defaultCtx
	a.mu.Unlock()
	return rc.StrokePath(target, path, paint)
}

// FillShape accumulates a filled shape via the default context.
func (a *SDFAccelerator) FillShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	a.mu.Lock()
	a.ensureDefaultCtx()
	rc := a.defaultCtx
	a.mu.Unlock()
	return rc.FillShape(target, shape, paint)
}

// StrokeShape accumulates a stroked shape via the default context.
func (a *SDFAccelerator) StrokeShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	a.mu.Lock()
	a.ensureDefaultCtx()
	rc := a.defaultCtx
	a.mu.Unlock()
	return rc.StrokeShape(target, shape, paint)
}

// Flush dispatches all pending commands for the default context.
func (a *SDFAccelerator) Flush(target gg.GPURenderTarget) error {
	a.mu.Lock()
	a.ensureDefaultCtx()
	rc := a.defaultCtx
	a.mu.Unlock()
	return rc.Flush(target)
}

// PendingCount returns the number of pending commands in the default context.
func (a *SDFAccelerator) PendingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.defaultCtx == nil {
		return 0
	}
	return a.defaultCtx.PendingCount()
}

// ensureDefaultCtx lazily creates the default render context. Must be called
// with a.mu held.
func (a *SDFAccelerator) ensureDefaultCtx() {
	if a.defaultCtx == nil {
		a.defaultCtx = a.shared.NewRenderContext()
	}
}

// getColorFromPaint extracts the solid color from a paint.
func getColorFromPaint(paint *gg.Paint) gg.RGBA {
	if paint.Brush != nil {
		if sb, isSolid := paint.Brush.(gg.SolidBrush); isSolid {
			return sb.Color
		}
		return paint.Brush.ColorAt(0, 0)
	}
	return gg.Black
}

// sameTarget compares two GPU render targets for identity.
func sameTarget(a *gg.GPURenderTarget, b *gg.GPURenderTarget) bool {
	// GPU-direct mode: compare View identity.
	if a.View != nil || b.View != nil {
		return a.View == b.View
	}
	// CPU readback mode: compare data buffer identity.
	return a.Width == b.Width && a.Height == b.Height &&
		len(a.Data) == len(b.Data) && len(a.Data) > 0 && &a.Data[0] == &b.Data[0]
}
