package gg

import (
	"errors"
	"image"
	"sync"

	"github.com/gogpu/gpucontext"
)

// ErrFallbackToCPU indicates the GPU accelerator cannot handle this operation.
// The caller should transparently fall back to CPU rendering.
var ErrFallbackToCPU = errors.New("gg: falling back to CPU rendering")

// AcceleratedOp describes operation types for GPU capability checking.
type AcceleratedOp uint32

const (
	// AccelFill represents path fill operations.
	AccelFill AcceleratedOp = 1 << iota

	// AccelStroke represents path stroke operations.
	AccelStroke

	// AccelScene represents full scene rendering.
	AccelScene

	// AccelText represents text rendering.
	AccelText

	// AccelImage represents image compositing.
	AccelImage

	// AccelGradient represents gradient rendering.
	AccelGradient

	// AccelCircleSDF represents SDF-based circle rendering.
	AccelCircleSDF

	// AccelRRectSDF represents SDF-based rounded rectangle rendering.
	AccelRRectSDF
)

// GPURenderTarget provides pixel buffer access for GPU output.
//
// Two mutually exclusive modes:
//
// GPU-direct mode: when View is non-nil, the render pass resolves directly
// to this texture view. Data/Stride are ignored and no CPU readback occurs.
// ViewWidth and ViewHeight specify the view dimensions in device pixels.
//
// CPU readback mode: when View is nil, rendering goes to an internal resolve
// texture and pixels are copied back into Data. Width, Height, and Stride
// describe the destination pixel buffer layout.
//
// The Data slice (when used) must be in premultiplied RGBA format, 4 bytes
// per pixel, laid out row by row with the given Stride.
type GPURenderTarget struct {
	// GPU-direct path: resolve directly to this view.
	// When non-nil, Data/Stride are ignored -- no CPU readback.
	// Type-assert to *wgpu.TextureView in internal/gpu consumers.
	View       gpucontext.TextureView
	ViewWidth  uint32
	ViewHeight uint32

	// Damage-aware compositing (ADR-016 Phase 2): when non-empty, compositor
	// uses LoadOpLoad (preserve previous frame) and scissor-clips to this rect.
	DamageRect image.Rectangle

	// CPU readback path.
	Data          []uint8
	Width, Height int
	Stride        int // bytes per row
}

// GPUAccelerator is an optional GPU acceleration provider.
//
// When registered via RegisterAccelerator, the Context tries GPU acceleration
// first for supported operations. If the accelerator returns ErrFallbackToCPU
// or any error, rendering transparently falls back to CPU.
//
// Implementations should be provided by GPU backend packages (e.g., gg/gpu/).
// Users opt in to GPU acceleration via blank import:
//
//	import _ "github.com/gogpu/gg/gpu" // enables GPU acceleration
type GPUAccelerator interface {
	// Name returns the accelerator name (e.g., "wgpu", "vulkan").
	Name() string

	// Init initializes GPU resources. Called once during registration.
	Init() error

	// Close releases GPU resources.
	Close()

	// CanAccelerate reports whether the accelerator supports the given operation.
	// This is a fast check used to skip GPU entirely for unsupported operations.
	CanAccelerate(op AcceleratedOp) bool

	// FillPath renders a filled path to the target.
	// Returns ErrFallbackToCPU if the path cannot be GPU-accelerated.
	FillPath(target GPURenderTarget, path *Path, paint *Paint) error

	// StrokePath renders a stroked path to the target.
	// Returns ErrFallbackToCPU if the path cannot be GPU-accelerated.
	StrokePath(target GPURenderTarget, path *Path, paint *Paint) error

	// FillShape renders a detected shape using SDF.
	// This is the fast path for circles and rounded rectangles.
	// Returns ErrFallbackToCPU if the shape is not supported.
	FillShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error

	// StrokeShape renders a detected shape outline using SDF.
	// Returns ErrFallbackToCPU if the shape is not supported.
	StrokeShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error

	// Flush dispatches any pending GPU operations to the target pixel buffer.
	// Batch-capable accelerators accumulate shapes during FillShape/StrokeShape
	// and dispatch them all in a single GPU pass on Flush.
	// Immediate-mode accelerators (e.g., CPU SDF) return nil.
	Flush(target GPURenderTarget) error
}

// DeviceProviderAware is an optional interface for accelerators that can share
// GPU resources with an external provider (e.g., gogpu window).
// When SetDeviceProvider is called, the accelerator reuses the provided GPU
// device instead of creating its own.
//
// The provider's Device() returns gpucontext.Device which consumers
// type-assert to *wgpu.Device for full HAL access:
//
//	dev := provider.Device().(*wgpu.Device)
//	halDevice := dev.HalDevice()
type DeviceProviderAware interface {
	SetDeviceProvider(provider gpucontext.DeviceProvider) error
}

// GPURenderContextProvider is an optional interface for accelerators that
// support per-context GPU rendering. When the accelerator implements this
// interface, each gg.Context lazily creates its own GPURenderContext for
// isolated pending command queues, clip state, and frame tracking.
//
// This follows the Skia GrContext pattern: shared device/pipelines/atlas,
// per-context pending ops. Without this, all contexts share one default
// render context (backward compatible, single-context behavior).
type GPURenderContextProvider interface {
	// NewGPURenderContext creates a new per-context GPU render context.
	// The returned value should be stored on the gg.Context and closed
	// when the Context is closed.
	NewGPURenderContext() any
}

// FrameAware is an optional interface for accelerators that need per-frame
// lifecycle management. BeginFrame resets per-frame state so that the first
// render pass of each frame clears the surface (LoadOpClear), while
// subsequent mid-frame flushes preserve content (LoadOpLoad).
//
// Without calling BeginFrame, the surface is only cleared on the very first
// frame and all subsequent frames composite on top of previous content,
// causing progressive accumulation artifacts.
type FrameAware interface {
	BeginFrame()
}

// GPUTextAccelerator is an optional interface for accelerators that support
// GPU-accelerated text rendering via MSDF (Multi-channel Signed Distance
// Field). When the registered accelerator implements this interface,
// Context.DrawString will use the GPU pipeline instead of CPU freetype.
//
// Text is rendered as Tier 4 in the unified render pass, after SDF shapes,
// convex polygons, and stencil-then-cover paths. The MSDF approach provides
// resolution-independent, crisp text at any scale.
type GPUTextAccelerator interface {
	// DrawText renders text at position (x, y) in user space where y is the
	// baseline. The face provides font metrics and glyph iteration. Color is
	// the text color in RGBA. The matrix parameter is the context's current
	// transformation matrix (CTM), which the GPU pipeline composes into the
	// vertex shader uniform so that Scale, Rotate, and Skew transforms
	// affect text rendering, not just position.
	//
	// The deviceScale parameter is the ratio of physical pixels to logical
	// pixels (e.g., 2.0 on a Retina display). The MSDF pipeline uses this
	// to compute an effective font size (logical size * deviceScale), which
	// produces a higher screenPxRange and crisper text on HiDPI displays.
	// Pass 1.0 for standard (non-HiDPI) rendering.
	//
	// Returns ErrFallbackToCPU if GPU text rendering is not available.
	DrawText(target GPURenderTarget, face any, text string, x, y float64, color RGBA, matrix Matrix, deviceScale float64) error
}

// GPUGlyphMaskAccelerator is an optional interface for accelerators that
// support Tier 6 glyph mask text rendering. When the registered accelerator
// implements this interface, Context.DrawString routes small horizontal text
// through the glyph mask pipeline instead of MSDF.
//
// Glyph mask rendering CPU-rasterizes glyphs at the exact device pixel size
// into an R8 alpha atlas, then draws them as textured quads on the GPU. This
// produces pixel-perfect hinted text for horizontal layouts at small sizes
// (typically <48px), matching the quality of native platform text renderers.
//
// The MSDF pipeline (GPUTextAccelerator) remains the preferred path for
// rotated, scaled, or large text where resolution-independence matters.
type GPUGlyphMaskAccelerator interface {
	// DrawGlyphMaskText renders text at position (x, y) in user space where
	// y is the baseline. The face provides font metrics and glyph iteration.
	//
	// The paint parameter provides the text color. The matrix parameter is
	// the context's current transformation matrix (CTM). The deviceScale
	// parameter is the ratio of physical to logical pixels.
	//
	// viewportW and viewportH are the viewport dimensions for building the
	// ortho projection in the vertex shader.
	//
	// Returns ErrFallbackToCPU if glyph mask rendering is not available.
	DrawGlyphMaskText(target GPURenderTarget, face any, s string, x, y float64, color RGBA, matrix Matrix, deviceScale float64) error
}

var (
	accelMu sync.RWMutex
	accel   GPUAccelerator
)

// RegisterAccelerator registers a GPU accelerator for optional GPU rendering.
//
// Only one accelerator can be registered. Subsequent calls replace the previous one.
// The accelerator's Init() method is called during registration.
// If Init() fails, the accelerator is not registered and the error is returned.
//
// Typical usage via blank import in GPU backend packages:
//
//	func init() {
//	    gg.RegisterAccelerator(NewWGPUAccelerator())
//	}
func RegisterAccelerator(a GPUAccelerator) error {
	if a == nil {
		return errors.New("gg: accelerator must not be nil")
	}
	if err := a.Init(); err != nil {
		return err
	}
	accelMu.Lock()
	old := accel
	accel = a
	accelMu.Unlock()

	// Propagate current logger to the new accelerator so it inherits
	// any SetLogger configuration that was applied before registration.
	propagateLogger(a, Logger())

	if old != nil {
		old.Close()
	}
	return nil
}

// Accelerator returns the currently registered GPU accelerator, or nil if none.
func Accelerator() GPUAccelerator {
	accelMu.RLock()
	a := accel
	accelMu.RUnlock()
	return a
}

// CloseAccelerator shuts down the global GPU accelerator, releasing all GPU
// resources (textures, pipelines, device, instance). After this call,
// [Accelerator] returns nil and rendering falls back to CPU.
//
// Call this at application shutdown to prevent VkImage and other GPU memory
// leaks. It is safe to call when no accelerator is registered (no-op).
// CloseAccelerator is idempotent.
//
// Example:
//
//	defer gg.CloseAccelerator()
func CloseAccelerator() {
	accelMu.Lock()
	a := accel
	accel = nil
	accelMu.Unlock()
	if a != nil {
		a.Close()
	}
}

// SetAcceleratorDeviceProvider passes a device provider to the registered
// accelerator, enabling GPU device sharing. If no accelerator is registered
// or it doesn't support device sharing, this is a no-op.
//
// The provider's Device() returns gpucontext.Device; accelerators type-assert
// to *wgpu.Device to obtain HAL access for GPU operations.
func SetAcceleratorDeviceProvider(provider gpucontext.DeviceProvider) error {
	a := Accelerator()
	if a == nil {
		return nil
	}
	if dpa, ok := a.(DeviceProviderAware); ok {
		return dpa.SetDeviceProvider(provider)
	}
	return nil
}

// AcceleratorCanRenderDirect returns true if the registered GPU accelerator
// is initialized and capable of rendering directly to a surface target.
// Returns false if no accelerator is registered, GPU init failed (e.g.,
// CPU-only adapter like llvmpipe), or the accelerator doesn't support
// direct surface rendering.
//
// Use this to decide whether to attempt RenderDirect or go straight to
// the universal CPU→texture→present path.
func AcceleratorCanRenderDirect() bool {
	a := Accelerator()
	if a == nil {
		return false
	}
	if drc, ok := a.(DirectRenderCapable); ok {
		return drc.CanRenderDirect()
	}
	return false
}

// DirectRenderCapable is an optional interface for accelerators that can
// report whether they are capable of rendering directly to a surface.
// CPU-only adapters (llvmpipe, SwiftShader) return false because SDF/MSDF
// shaders should not run on CPU — the accelerator stays uninitialized.
type DirectRenderCapable interface {
	CanRenderDirect() bool
}

// BeginAcceleratorFrame signals the start of a new rendering frame.
// This resets per-frame state so that the first render pass clears the
// surface. Must be called once per frame before any drawing operations
// when using direct surface rendering (RenderDirect).
//
// If no accelerator is registered or it doesn't implement FrameAware,
// this is a no-op.
func BeginAcceleratorFrame() {
	a := Accelerator()
	if a == nil {
		return
	}
	if fa, ok := a.(FrameAware); ok {
		fa.BeginFrame()
	}
}

// ComputePipelineAware is an optional interface for accelerators that support
// the Vello-style compute pipeline. When the accelerator implements this interface
// and PipelineMode is Auto or Compute, the framework uses the compute pipeline
// for complex scenes instead of the render pass pipeline.
type ComputePipelineAware interface {
	// CanCompute reports whether the compute pipeline is available and ready.
	CanCompute() bool
}

// PipelineModeAware is an optional interface for accelerators that support
// pipeline mode selection (render pass vs compute). When the registered
// accelerator implements this interface, the Context propagates pipeline mode
// changes so the accelerator can route operations accordingly.
type PipelineModeAware interface {
	// SetPipelineMode sets the pipeline mode for subsequent operations.
	SetPipelineMode(mode PipelineMode)
}

// ForceSDFAware is an optional interface for GPU accelerators that support
// forced SDF rendering. When enabled, the accelerator bypasses the minimum
// size check for SDF shapes, allowing RasterizerSDF mode to force SDF
// rendering regardless of shape size.
type ForceSDFAware interface {
	SetForceSDF(force bool)
}

// ClipAware is an optional interface for accelerators that support
// hardware scissor rect clipping. When the Context has an active
// rectangular clip region (ClipRect), it passes the clip bounds to the
// accelerator as a scissor rect. The accelerator maps this to
// hal.RenderPassEncoder.SetScissorRect() for zero-cost GPU clipping.
//
// This covers ~95% of real-world UI clipping (scroll views, panels,
// list items). Path-based clips require GPU-CLIP-002 (stencil buffer).
type ClipAware interface {
	// SetClipRect sets the scissor rect for subsequent GPU draw commands.
	// Coordinates are in device pixels (uint32). The scissor rect clips
	// all rendering to the rectangle (x, y, w, h).
	SetClipRect(x, y, w, h uint32)

	// ClearClipRect removes the scissor rect, restoring full-framebuffer
	// rendering for subsequent draw commands.
	ClearClipRect()
}

// RRectClipAware is an optional interface for accelerators that support
// GPU-accelerated rounded rectangle clipping via analytic SDF in fragment
// shaders. When the Context has an active RRect clip region (ClipRoundRect),
// it passes the clip parameters to the accelerator. The fragment shader
// evaluates the RRect SDF per pixel and discards fragments outside the
// rounded rectangle.
//
// This is a two-level clip strategy: the scissor rect (via ClipAware)
// provides a free coarse clip to the bounding box, while the RRect SDF
// provides fine per-pixel clipping with anti-aliased rounded corners.
//
// This covers ~95% of non-rectangular UI clipping (card views, dialogs,
// avatars, pill buttons). Path-based clips require GPU-CLIP-003 (stencil).
type RRectClipAware interface {
	// SetClipRRect sets the rounded rectangle clip for subsequent GPU draw
	// commands. Coordinates are in device pixels (float32). The clip region
	// is defined by the rectangle (x, y, w, h) with uniform corner radius.
	SetClipRRect(x, y, w, h, radius float32)

	// ClearClipRRect removes the rounded rectangle clip, restoring full
	// rendering for subsequent draw commands.
	ClearClipRRect()
}

// LCDLayoutAware is an optional interface for accelerators that support
// LCD subpixel (ClearType) text rendering. When the Context calls
// SetLCDLayout, it propagates the layout to the accelerator so the
// glyph mask engine can rasterize glyphs with 3x horizontal oversampling
// and the GPU pipeline selects the LCD fragment shader.
//
// Use LCDLayoutRGB for most monitors, LCDLayoutBGR for rare BGR panels,
// or LCDLayoutNone to disable subpixel rendering (grayscale, the default).
type LCDLayoutAware interface {
	// SetLCDLayout sets the LCD subpixel layout for ClearType rendering.
	SetLCDLayout(layout LCDLayout)
}

// MaskAware is an optional interface for accelerators that support
// GPU-accelerated alpha masking. When the Context has an active mask,
// it uploads the mask data to the accelerator as a texture. The fragment
// shader multiplies output alpha by the mask texel, eliminating the need
// to fall back to CPU rendering for masked shapes.
//
// SetMaskTexture uploads the mask data (8-bit per pixel, row-major).
// Pass nil data to clear the mask texture.
// ClearMaskTexture removes the mask, restoring unmasked rendering.
type MaskAware interface {
	// SetMaskTexture uploads an alpha mask for GPU rendering.
	// data is an 8-bit grayscale buffer (width*height bytes).
	// The accelerator creates an R8Unorm texture and samples it in the
	// fragment shader to modulate output alpha.
	SetMaskTexture(data []byte, width, height int)

	// ClearMaskTexture removes the GPU mask texture.
	ClearMaskTexture()
}

// SceneStatsTracker is an optional interface for accelerators that track
// per-frame scene statistics for auto pipeline selection. The Context
// does not call these methods directly — the accelerator uses them internally.
type SceneStatsTracker interface {
	// SceneStats returns the accumulated scene statistics for the current frame.
	SceneStats() SceneStats
}
