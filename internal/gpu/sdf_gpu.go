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

// SDFAccelerator provides GPU-accelerated rendering using wgpu/hal render
// pipelines. It implements the gg.GPUAccelerator interface.
//
// The accelerator uses a unified GPURenderSession to render all draw commands
// (SDF shapes + stencil-then-cover paths) in a single render pass. Shapes
// submitted via FillShape/StrokeShape and paths via FillPath are accumulated
// and rendered together on Flush().
//
// This unified approach matches enterprise 2D engines (Skia Ganesh/Graphite,
// Flutter Impeller, Gio): one render pass with pipeline switching, shared
// MSAA + stencil textures, single submit + fence wait, single readback.
type SDFAccelerator struct {
	mu sync.Mutex

	instance hal.Instance
	device   hal.Device
	queue    hal.Queue

	// Unified render session managing shared textures and frame encoding.
	session *GPURenderSession

	// SDF render pipeline (vertex+fragment) -- owned by the accelerator,
	// shared with the session.
	sdfRenderPipeline *SDFRenderPipeline

	// Stencil-then-cover renderer -- owned by the accelerator,
	// shared with the session.
	stencilRenderer *StencilRenderer

	// Pending SDF shapes for batch dispatch.
	pendingShapes []SDFRenderShape

	// Pending stencil paths for unified dispatch.
	pendingStencilPaths []StencilPathCommand

	pendingTarget *gg.GPURenderTarget // nil if no pending commands

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
	a.pendingStencilPaths = nil
	a.pendingTarget = nil
	if a.session != nil {
		a.session.Destroy()
		a.session = nil
	}
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
	if a.session != nil {
		a.session.Destroy()
		a.session = nil
	}
	if a.sdfRenderPipeline != nil {
		a.sdfRenderPipeline.Destroy()
		a.sdfRenderPipeline = nil
	}
	if a.stencilRenderer != nil {
		a.stencilRenderer.Destroy()
		a.stencilRenderer = nil
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

	// Create pipelines and session with shared device.
	a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)
	a.stencilRenderer = NewStencilRenderer(a.device, a.queue)
	a.session = NewGPURenderSession(a.device, a.queue)
	a.session.SetSDFPipeline(a.sdfRenderPipeline)
	a.session.SetStencilRenderer(a.stencilRenderer)

	a.gpuReady = true
	log.Printf("gpu-sdf: switched to shared GPU device")
	return nil
}

// FillPath queues a filled path for stencil-then-cover rendering.
// The path is tessellated immediately but rendering is deferred until Flush()
// so it can be combined with SDF shapes in a single render pass.
// Returns ErrFallbackToCPU if the GPU is not ready.
func (a *SDFAccelerator) FillPath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.gpuReady {
		return gg.ErrFallbackToCPU
	}

	// If target changed, flush previous batch first.
	if a.pendingTarget != nil && !sameTarget(a.pendingTarget, &target) {
		if err := a.flushLocked(*a.pendingTarget); err != nil {
			return err
		}
	}

	// Tessellate path into fan triangles.
	tess := NewFanTessellator()
	tess.TessellatePath(path.Elements())
	fanVerts := tess.Vertices()
	if len(fanVerts) == 0 {
		return nil // empty path, nothing to render
	}

	color := getColorFromPaint(paint)
	// Premultiply alpha for GPU blending.
	premulR := float32(color.R * color.A)
	premulG := float32(color.G * color.A)
	premulB := float32(color.B * color.A)
	premulA := float32(color.A)

	cmd := StencilPathCommand{
		Vertices:  make([]float32, len(fanVerts)),
		CoverQuad: tess.CoverQuad(),
		Color:     [4]float32{premulR, premulG, premulB, premulA},
		FillRule:  paint.FillRule,
	}
	copy(cmd.Vertices, fanVerts)
	a.pendingStencilPaths = append(a.pendingStencilPaths, cmd)

	targetCopy := target
	a.pendingTarget = &targetCopy
	return nil
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

// Flush dispatches all pending commands (SDF shapes + stencil paths) via the
// unified render session. All commands are rendered in a single render pass.
// Returns nil if there are no pending commands.
func (a *SDFAccelerator) Flush(target gg.GPURenderTarget) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.flushLocked(target)
}

// PendingCount returns the number of shapes waiting for dispatch (for testing).
func (a *SDFAccelerator) PendingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pendingShapes) + len(a.pendingStencilPaths)
}

func (a *SDFAccelerator) flushLocked(target gg.GPURenderTarget) error {
	if len(a.pendingShapes) == 0 && len(a.pendingStencilPaths) == 0 {
		return nil
	}

	// Ensure session exists.
	if a.session == nil {
		a.session = NewGPURenderSession(a.device, a.queue)
		if a.sdfRenderPipeline == nil {
			a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)
		}
		if a.stencilRenderer == nil {
			a.stencilRenderer = NewStencilRenderer(a.device, a.queue)
		}
		a.session.SetSDFPipeline(a.sdfRenderPipeline)
		a.session.SetStencilRenderer(a.stencilRenderer)
	}

	// Take ownership of pending data.
	shapes := make([]SDFRenderShape, len(a.pendingShapes))
	copy(shapes, a.pendingShapes)
	a.pendingShapes = a.pendingShapes[:0]

	paths := make([]StencilPathCommand, len(a.pendingStencilPaths))
	copy(paths, a.pendingStencilPaths)
	a.pendingStencilPaths = a.pendingStencilPaths[:0]
	a.pendingTarget = nil

	err := a.session.RenderFrame(target, shapes, paths)
	if err != nil {
		log.Printf("gpu-sdf: render session error (%d shapes, %d paths): %v",
			len(shapes), len(paths), err)
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

	// Create pipelines and session.
	a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)
	a.stencilRenderer = NewStencilRenderer(a.device, a.queue)
	a.session = NewGPURenderSession(a.device, a.queue)
	a.session.SetSDFPipeline(a.sdfRenderPipeline)
	a.session.SetStencilRenderer(a.stencilRenderer)

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
