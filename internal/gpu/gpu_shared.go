//go:build !nogpu

package gpu

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// GPUShared holds GPU resources that are shared across all gg.Context instances.
// This includes the device, queue, pipelines, and atlas engines — expensive to
// create, immutable or append-only after initialization.
//
// This follows the enterprise pattern observed in Skia (GrDirectContext),
// Vello (Renderer), Qt Quick (QRhi), and Flutter Impeller (Context):
// shared device + pipelines + glyph atlas, per-context pending commands.
//
// GPUShared is created once per application via RegisterAccelerator and
// accessed by all gg.Context instances through the global singleton.
type GPUShared struct {
	mu sync.Mutex

	instance *wgpu.Instance // standalone mode only; nil when using external device
	device   *wgpu.Device
	queue    *wgpu.Queue

	// Pipelines (immutable after creation, safe to share).
	sdfRenderPipeline *SDFRenderPipeline
	convexRenderer    *ConvexRenderer
	stencilRenderer   *StencilRenderer

	// Text/glyph atlas engines (append-only, shared across contexts).
	textEngine      *GPUTextEngine   // MSDF atlas (Tier 4)
	glyphMaskEngine *GlyphMaskEngine // R8 alpha atlas (Tier 6)

	// Compute pipeline.
	velloAccel *VelloAccelerator

	// Texture pool for per-context MSAA/stencil textures (Flutter RenderTargetCache pattern).
	texturePool *TexturePool

	// CPU SDF fallback accelerator.
	cpuFallback gg.SDFAccelerator

	gpuReady       bool
	externalDevice bool // true when using shared device (don't destroy on Close)
}

// NewGPUShared creates a new shared GPU resource holder. GPU initialization
// is deferred until the first render or SetDeviceProvider call to avoid
// creating a standalone Vulkan device that may interfere with an external
// DX12/Metal device.
func NewGPUShared() *GPUShared {
	return &GPUShared{
		texturePool: NewTexturePool(defaultTexturePoolBudgetMB),
	}
}

// NewRenderContext creates a new per-context GPU render context that references
// this shared resource holder. Each gg.Context should have its own
// GPURenderContext for isolated pending command queues and frame tracking.
func (s *GPUShared) NewRenderContext() *GPURenderContext {
	return &GPURenderContext{
		shared: s,
	}
}

// IsReady reports whether the GPU is initialized and ready for rendering.
func (s *GPUShared) IsReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gpuReady
}

// Device returns the shared wgpu device, or nil if not initialized.
func (s *GPUShared) Device() *wgpu.Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.device
}

// Queue returns the shared wgpu queue, or nil if not initialized.
func (s *GPUShared) Queue() *wgpu.Queue {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue
}

// SetLogger sets the logger for the GPU subsystem.
func (s *GPUShared) SetLogger(l *slog.Logger) {
	setLogger(l)
}

// SetLCDLayout propagates the LCD subpixel layout to the glyph mask engine.
func (s *GPUShared) SetLCDLayout(layout text.LCDLayout) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureGlyphMaskEngine()
	s.glyphMaskEngine.SetLCDLayout(layout)
}

// SetForceSDF propagates the force-SDF flag to the CPU fallback accelerator.
func (s *GPUShared) SetForceSDF(force bool) {
	s.cpuFallback.SetForceSDF(force)
}

// SetDeviceProvider switches to a shared GPU device from an external provider
// (e.g., gogpu). The provider's Device() must return a *wgpu.Device.
func (s *GPUShared) SetDeviceProvider(provider gpucontext.DeviceProvider) error {
	// Check if adapter is software/CPU — GPU shaders don't work on CPU backends.
	if adapter := provider.Adapter(); adapter != nil {
		if wgpuAdapter, ok := adapter.(*wgpu.Adapter); ok {
			if wgpuAdapter.Info().DeviceType == gputypes.DeviceTypeCPU {
				slogger().Info("gpu-shared: software adapter detected, GPU acceleration disabled")
				return nil
			}
		}
	}

	dev := provider.Device()
	if dev == nil {
		return fmt.Errorf("gpu-shared: provider Device is nil")
	}

	wgpuDev, ok := dev.(*wgpu.Device)
	if !ok {
		return fmt.Errorf("gpu-shared: provider Device is not *wgpu.Device (got %T)", dev)
	}
	wgpuQueue := wgpuDev.Queue()
	if wgpuQueue == nil {
		return fmt.Errorf("gpu-shared: provider Queue is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Destroy own resources if we created them.
	s.destroyPipelinesLocked()
	if !s.externalDevice && s.device != nil {
		s.device.Release()
	}
	if s.instance != nil {
		s.instance.Release()
		s.instance = nil
	}

	// Use provided resources.
	s.device = wgpuDev
	s.queue = wgpuQueue
	s.externalDevice = true

	// Create pipelines with shared device.
	s.sdfRenderPipeline = NewSDFRenderPipeline(s.device, s.queue)
	s.convexRenderer = NewConvexRenderer(s.device, s.queue)
	s.stencilRenderer = NewStencilRenderer(s.device, s.queue)

	s.gpuReady = true

	// Initialize internal VelloAccelerator with the shared device.
	s.initVelloAccelerator(s.device, s.queue)

	slogger().Info("gpu-shared: switched to shared GPU device",
		"adapter", fmt.Sprintf("%T", s.device),
	)
	return nil
}

// CanRenderDirect reports whether the GPU is initialized and can render
// directly to a surface. Returns false on CPU-only adapters.
func (s *GPUShared) CanRenderDirect() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gpuReady
}

// CanCompute reports whether the compute pipeline is available.
func (s *GPUShared) CanCompute() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.velloAccel != nil && s.velloAccel.CanCompute()
}

// SetTexturePoolBudget sets the maximum memory budget for the texture pool
// in megabytes. Default is 128MB (~5 concurrent 1080p MSAA4x contexts).
func (s *GPUShared) SetTexturePoolBudget(mb int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.texturePool.SetBudget(mb)
}

// Close releases all shared GPU resources. After this call, GPU rendering
// is no longer possible. Idempotent.
func (s *GPUShared) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.textEngine = nil
	if s.glyphMaskEngine != nil && s.device != nil {
		s.glyphMaskEngine.Destroy(s.device)
		s.glyphMaskEngine = nil
	}
	if s.velloAccel != nil {
		s.velloAccel.Close()
		s.velloAccel = nil
	}
	if s.texturePool != nil {
		s.texturePool.DestroyAll()
	}
	s.destroyPipelinesLocked()
	if !s.externalDevice {
		if s.device != nil {
			s.device.Release()
			s.device = nil
		}
		if s.instance != nil {
			s.instance.Release()
			s.instance = nil
		}
	} else {
		s.device = nil
		s.instance = nil
	}
	s.queue = nil
	s.gpuReady = false
	s.externalDevice = false
}

// ensureGPU lazily initializes a standalone GPU device if no shared device
// was provided. Must be called with s.mu held.
func (s *GPUShared) ensureGPU() error {
	if s.device != nil {
		return nil
	}
	return s.initGPU()
}

// ensurePipelines lazily creates pipelines if they don't exist.
// Must be called with s.mu held and device initialized.
func (s *GPUShared) ensurePipelines() {
	if s.sdfRenderPipeline == nil {
		s.sdfRenderPipeline = NewSDFRenderPipeline(s.device, s.queue)
	}
	if s.convexRenderer == nil {
		s.convexRenderer = NewConvexRenderer(s.device, s.queue)
	}
	if s.stencilRenderer == nil {
		s.stencilRenderer = NewStencilRenderer(s.device, s.queue)
	}
}

// ensureGlyphMaskEngine lazily creates the glyph mask engine. Must be called
// with s.mu held.
func (s *GPUShared) ensureGlyphMaskEngine() {
	if s.glyphMaskEngine == nil {
		s.glyphMaskEngine = NewGlyphMaskEngine()
	}
}

// ensureTextEngine lazily creates the text engine. Must be called with s.mu held.
func (s *GPUShared) ensureTextEngine() {
	if s.textEngine == nil {
		s.textEngine = NewGPUTextEngine()
	}
}

func (s *GPUShared) initGPU() error {
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{
		Backends: wgpu.BackendsVulkan,
	})
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	s.instance = instance

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: wgpu.PowerPreferenceHighPerformance,
	})
	if err != nil {
		return fmt.Errorf("request adapter: %w", err)
	}

	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{Label: "gg-shared"})
	if err != nil {
		return fmt.Errorf("request device: %w", err)
	}
	s.device = device
	s.queue = device.Queue()

	// Create pipelines.
	s.sdfRenderPipeline = NewSDFRenderPipeline(s.device, s.queue)
	s.convexRenderer = NewConvexRenderer(s.device, s.queue)
	s.stencilRenderer = NewStencilRenderer(s.device, s.queue)

	s.gpuReady = true

	// Initialize internal VelloAccelerator for compute routing.
	s.initVelloAccelerator(s.device, s.queue)

	slogger().Info("gpu-shared: GPU initialized", "adapter", adapter.Info().Name)
	return nil
}

func (s *GPUShared) initVelloAccelerator(device *wgpu.Device, queue *wgpu.Queue) {
	va := &VelloAccelerator{}
	va.device = device
	va.queue = queue
	va.externalDevice = true
	va.gpuReady = true

	dispatcher := NewVelloComputeDispatcher(device, queue)
	if err := dispatcher.Init(); err != nil {
		slogger().Debug("gpu-shared: compute pipeline unavailable", "error", err)
		s.velloAccel = va
		return
	}
	va.dispatcher = dispatcher
	s.velloAccel = va
	slogger().Debug("gpu-shared: VelloAccelerator initialized for compute routing")
}

func (s *GPUShared) destroyPipelinesLocked() {
	if s.sdfRenderPipeline != nil {
		s.sdfRenderPipeline.Destroy()
		s.sdfRenderPipeline = nil
	}
	if s.convexRenderer != nil {
		s.convexRenderer.Destroy()
		s.convexRenderer = nil
	}
	if s.stencilRenderer != nil {
		s.stencilRenderer.Destroy()
		s.stencilRenderer = nil
	}
}
