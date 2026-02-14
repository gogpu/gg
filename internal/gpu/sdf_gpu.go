//go:build !nogpu

package gpu

import (
	"fmt"
	"log"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"

	// Import Vulkan backend so it registers via init().
	_ "github.com/gogpu/wgpu/hal/vulkan"
)

// SDFAccelerator provides GPU-accelerated SDF rendering using wgpu/hal
// render pipelines. It implements the gg.GPUAccelerator interface.
//
// Shapes submitted via FillShape/StrokeShape are accumulated and rendered
// in a single render pass on Flush() via the SDFRenderPipeline.
//
// For general path fills (non-SDF shapes), the accelerator delegates to
// a StencilRenderer that implements the stencil-then-cover algorithm.
type SDFAccelerator struct {
	mu sync.Mutex

	instance hal.Instance
	device   hal.Device
	queue    hal.Queue

	// SDF render pipeline (vertex+fragment).
	sdfRenderPipeline *SDFRenderPipeline

	// Stencil-then-cover renderer for general path fills.
	stencilRenderer *StencilRenderer

	// Pending shapes for render pipeline dispatch.
	pendingShapes []SDFRenderShape
	pendingTarget *gg.GPURenderTarget // nil if no pending shapes

	cpuFallback    gg.SDFAccelerator
	gpuReady       bool
	externalDevice bool // true when using shared device (don't destroy on Close)
}

var _ gg.GPUAccelerator = (*SDFAccelerator)(nil)

// Name returns the accelerator identifier.
func (a *SDFAccelerator) Name() string { return "sdf-gpu" }

// CanAccelerate reports whether this accelerator supports the given operation.
func (a *SDFAccelerator) CanAccelerate(op gg.AcceleratedOp) bool {
	return op&(gg.AccelCircleSDF|gg.AccelRRectSDF|gg.AccelFill) != 0
}

// Init initializes the GPU device and render pipelines.
// On failure, the accelerator silently falls back to CPU rendering.
func (a *SDFAccelerator) Init() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.initGPU(); err != nil {
		log.Printf("gpu-sdf: GPU init failed, using CPU fallback: %v", err)
	}
	return nil
}

// Close releases all GPU resources held by the accelerator.
func (a *SDFAccelerator) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pendingShapes = nil
	a.pendingTarget = nil
	if a.sdfRenderPipeline != nil {
		a.sdfRenderPipeline.Destroy()
		a.sdfRenderPipeline = nil
	}
	if a.stencilRenderer != nil {
		a.stencilRenderer.Destroy()
		a.stencilRenderer = nil
	}
	if !a.externalDevice {
		if a.device != nil {
			a.device.Destroy()
			a.device = nil
		}
		if a.instance != nil {
			a.instance.Destroy()
			a.instance = nil
		}
	} else {
		// Don't destroy shared resources -- we don't own them.
		a.device = nil
		a.instance = nil
	}
	a.queue = nil
	a.gpuReady = false
	a.externalDevice = false
}

// SetDeviceProvider switches the accelerator to use a shared GPU device
// from an external provider (e.g., gogpu). The provider must implement
// HalDevice() any and HalQueue() any returning hal.Device and hal.Queue.
func (a *SDFAccelerator) SetDeviceProvider(provider any) error {
	type halProvider interface {
		HalDevice() any
		HalQueue() any
	}
	hp, ok := provider.(halProvider)
	if !ok {
		return fmt.Errorf("gpu-sdf: provider does not expose HAL types")
	}
	device, ok := hp.HalDevice().(hal.Device)
	if !ok || device == nil {
		return fmt.Errorf("gpu-sdf: provider HalDevice is not hal.Device")
	}
	queue, ok := hp.HalQueue().(hal.Queue)
	if !ok || queue == nil {
		return fmt.Errorf("gpu-sdf: provider HalQueue is not hal.Queue")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Destroy own resources if we created them.
	if a.sdfRenderPipeline != nil {
		a.sdfRenderPipeline.Destroy()
		a.sdfRenderPipeline = nil
	}
	if !a.externalDevice && a.device != nil {
		a.device.Destroy()
	}
	if a.instance != nil {
		a.instance.Destroy()
		a.instance = nil
	}

	// Use provided resources.
	a.device = device
	a.queue = queue
	a.externalDevice = true

	// Create render pipeline with shared device.
	a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)

	a.gpuReady = true
	log.Printf("gpu-sdf: switched to shared GPU device")
	return nil
}

// FillPath renders a filled path using the stencil-then-cover algorithm.
// It first flushes any pending SDF shapes, then delegates to StencilRenderer.
// Returns ErrFallbackToCPU if the GPU is not ready.
func (a *SDFAccelerator) FillPath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.gpuReady {
		return gg.ErrFallbackToCPU
	}

	// Flush any pending SDF shapes before the render pass.
	if err := a.flushLocked(target); err != nil {
		return err
	}

	// Ensure stencil renderer exists.
	if a.stencilRenderer == nil {
		a.stencilRenderer = NewStencilRenderer(a.device, a.queue)
	}

	color := getColorFromPaint(paint)
	elements := path.Elements()
	return a.stencilRenderer.RenderPath(target, elements, color, paint.FillRule)
}

// StrokePath is not yet GPU-accelerated; it falls back to CPU rendering.
func (a *SDFAccelerator) StrokePath(_ gg.GPURenderTarget, _ *gg.Path, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

// FillShape accumulates a filled shape for batch dispatch.
// The actual GPU work happens on Flush().
func (a *SDFAccelerator) FillShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.gpuReady {
		return a.cpuFallback.FillShape(target, shape, paint)
	}
	return a.queueShape(target, shape, paint, false)
}

// StrokeShape accumulates a stroked shape for batch dispatch.
// The actual GPU work happens on Flush().
func (a *SDFAccelerator) StrokeShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.gpuReady {
		return a.cpuFallback.StrokeShape(target, shape, paint)
	}
	return a.queueShape(target, shape, paint, true)
}

// Flush dispatches all pending shapes via the render pipeline.
// Returns nil if there are no pending shapes.
func (a *SDFAccelerator) Flush(target gg.GPURenderTarget) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.flushLocked(target)
}

// PendingCount returns the number of shapes waiting for dispatch (for testing).
func (a *SDFAccelerator) PendingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pendingShapes)
}

func (a *SDFAccelerator) flushLocked(target gg.GPURenderTarget) error {
	if len(a.pendingShapes) == 0 {
		return nil
	}
	if a.sdfRenderPipeline == nil {
		a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)
	}
	shapes := make([]SDFRenderShape, len(a.pendingShapes))
	copy(shapes, a.pendingShapes)
	a.pendingShapes = a.pendingShapes[:0]
	a.pendingTarget = nil

	err := a.sdfRenderPipeline.RenderShapes(target, shapes)
	if err != nil {
		log.Printf("gpu-sdf: render pipeline error (%d shapes): %v", len(shapes), err)
	}
	return err
}

func (a *SDFAccelerator) queueShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint, stroked bool) error {
	// If target changed, flush previous batch first.
	if a.pendingTarget != nil && !sameTarget(a.pendingTarget, &target) {
		if err := a.flushLocked(*a.pendingTarget); err != nil {
			return err
		}
	}

	rs, ok := DetectedShapeToRenderShape(shape, paint, stroked)
	if !ok {
		return gg.ErrFallbackToCPU
	}
	a.pendingShapes = append(a.pendingShapes, rs)

	targetCopy := target
	a.pendingTarget = &targetCopy
	return nil
}

func sameTarget(a *gg.GPURenderTarget, b *gg.GPURenderTarget) bool {
	return a.Width == b.Width && a.Height == b.Height &&
		len(a.Data) == len(b.Data) && len(a.Data) > 0 && &a.Data[0] == &b.Data[0]
}

func (a *SDFAccelerator) initGPU() error {
	backend, ok := hal.GetBackend(gputypes.BackendVulkan)
	if !ok {
		return fmt.Errorf("vulkan backend not available")
	}
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{Flags: 0})
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	a.instance = instance
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		return fmt.Errorf("no GPU adapters found")
	}
	var selected *hal.ExposedAdapter
	for i := range adapters {
		if adapters[i].Info.DeviceType == gputypes.DeviceTypeDiscreteGPU ||
			adapters[i].Info.DeviceType == gputypes.DeviceTypeIntegratedGPU {
			selected = &adapters[i]
			break
		}
	}
	if selected == nil {
		selected = &adapters[0]
	}
	openDev, err := selected.Adapter.Open(gputypes.Features(0), gputypes.DefaultLimits())
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	a.device = openDev.Device
	a.queue = openDev.Queue

	// Create SDF render pipeline.
	a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)

	a.gpuReady = true
	log.Printf("gpu-sdf: GPU accelerator initialized (%s)", selected.Info.Name)
	return nil
}

func getColorFromPaint(paint *gg.Paint) gg.RGBA {
	if paint.Brush != nil {
		if sb, isSolid := paint.Brush.(gg.SolidBrush); isSolid {
			return sb.Color
		}
		return paint.Brush.ColorAt(0, 0)
	}
	return gg.Black
}
