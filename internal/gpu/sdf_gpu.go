//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"

	// Import Vulkan backend so it registers via init().
	_ "github.com/gogpu/wgpu/hal/vulkan"
)

// SDFAccelerator provides GPU-accelerated SDF rendering using wgpu/hal
// compute shaders. It implements the gg.GPUAccelerator interface.
//
// Shapes submitted via FillShape/StrokeShape are accumulated into a batch
// and dispatched as a single GPU compute pass on Flush(). This avoids
// per-shape fence waits and buffer allocations.
//
// For general path fills (non-SDF shapes), the accelerator delegates to
// a StencilRenderer that implements the stencil-then-cover algorithm.
type SDFAccelerator struct {
	mu sync.Mutex

	instance hal.Instance
	device   hal.Device
	queue    hal.Queue

	// Batch compute dispatch pipeline (legacy, kept for fallback).
	batchShader     hal.ShaderModule
	batchBindLayout hal.BindGroupLayout
	batchPipeLayout hal.PipelineLayout
	batchPipeline   hal.ComputePipeline

	// SDF render pipeline (vertex+fragment, preferred over compute).
	sdfRenderPipeline *SDFRenderPipeline

	// Stencil-then-cover renderer for general path fills.
	stencilRenderer *StencilRenderer

	// Pending shapes for batch dispatch.
	pendingShapes      []SDFRenderShape
	pendingBatchShapes []SDFBatchShape     // legacy compute path
	pendingTarget      *gg.GPURenderTarget // nil if no pending shapes
	useRenderPipeline  bool                // true = use render pipeline, false = compute

	cpuFallback    gg.SDFAccelerator
	gpuReady       bool
	externalDevice bool // true when using shared device (don't destroy on Close)
}

var _ gg.GPUAccelerator = (*SDFAccelerator)(nil)

func (a *SDFAccelerator) Name() string { return "sdf-gpu" }

func (a *SDFAccelerator) CanAccelerate(op gg.AcceleratedOp) bool {
	return op&(gg.AccelCircleSDF|gg.AccelRRectSDF|gg.AccelFill) != 0
}

func (a *SDFAccelerator) Init() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.initGPU(); err != nil {
		log.Printf("gpu-sdf: GPU init failed, using CPU fallback: %v", err)
	}
	return nil
}

func (a *SDFAccelerator) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pendingShapes = nil
	a.pendingBatchShapes = nil
	a.pendingTarget = nil
	if a.sdfRenderPipeline != nil {
		a.sdfRenderPipeline.Destroy()
		a.sdfRenderPipeline = nil
	}
	if a.stencilRenderer != nil {
		a.stencilRenderer.Destroy()
		a.stencilRenderer = nil
	}
	a.destroyPipelines()
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
		// Don't destroy shared resources — we don't own them
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

	// Destroy own resources if we created them
	if a.sdfRenderPipeline != nil {
		a.sdfRenderPipeline.Destroy()
		a.sdfRenderPipeline = nil
	}
	a.destroyPipelines()
	if !a.externalDevice && a.device != nil {
		a.device.Destroy()
	}
	if a.instance != nil {
		a.instance.Destroy()
		a.instance = nil
	}

	// Use provided resources
	a.device = device
	a.queue = queue
	a.externalDevice = true

	// Recreate pipelines with shared device
	if err := a.createPipelines(); err != nil {
		a.gpuReady = false
		return fmt.Errorf("gpu-sdf: create pipelines with shared device: %w", err)
	}

	// Create render pipeline (preferred path).
	a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)
	a.useRenderPipeline = true

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

// Flush dispatches all pending shapes in a single GPU compute pass.
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
	if a.useRenderPipeline {
		return len(a.pendingShapes)
	}
	return len(a.pendingBatchShapes)
}

func (a *SDFAccelerator) flushLocked(target gg.GPURenderTarget) error {
	if a.useRenderPipeline {
		return a.flushRenderPipeline(target)
	}
	return a.flushComputePipeline(target)
}

// flushRenderPipeline dispatches pending shapes via the vertex+fragment render pipeline.
func (a *SDFAccelerator) flushRenderPipeline(target gg.GPURenderTarget) error {
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

// flushComputePipeline dispatches pending shapes via the legacy compute pipeline.
func (a *SDFAccelerator) flushComputePipeline(target gg.GPURenderTarget) error {
	if len(a.pendingBatchShapes) == 0 {
		return nil
	}
	// Temporarily swap pendingShapes with pendingBatchShapes for dispatchBatch.
	origPending := a.pendingShapes
	a.pendingShapes = nil

	origBatch := a.pendingBatchShapes
	err := a.dispatchBatchLegacy(target, origBatch)
	a.pendingBatchShapes = a.pendingBatchShapes[:0]
	a.pendingShapes = origPending
	a.pendingTarget = nil
	if err != nil {
		log.Printf("gpu-sdf: batch dispatch error (%d shapes): %v", len(origBatch), err)
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

	if a.useRenderPipeline {
		rs, ok := DetectedShapeToRenderShape(shape, paint, stroked)
		if !ok {
			return gg.ErrFallbackToCPU
		}
		a.pendingShapes = append(a.pendingShapes, rs)
	} else {
		// Legacy compute path.
		color := getColorFromPaint(paint)
		var kind uint32
		var p1, p2, p3 float32
		switch shape.Kind {
		case gg.ShapeCircle, gg.ShapeEllipse:
			kind = 0
			p1 = float32(shape.RadiusX)
			p2 = float32(shape.RadiusY)
		case gg.ShapeRect, gg.ShapeRRect:
			kind = 1
			p1 = float32(shape.Width / 2)
			p2 = float32(shape.Height / 2)
			p3 = float32(shape.CornerRadius)
		default:
			return gg.ErrFallbackToCPU
		}
		var halfStroke float32
		var isStroked uint32
		if stroked {
			halfStroke = float32(paint.EffectiveLineWidth() / 2)
			isStroked = 1
		}
		a.pendingBatchShapes = append(a.pendingBatchShapes, SDFBatchShape{
			Kind: kind, CenterX: float32(shape.CenterX), CenterY: float32(shape.CenterY),
			Param1: p1, Param2: p2, Param3: p3,
			HalfStroke: halfStroke, IsStroked: isStroked,
			ColorR: float32(color.R), ColorG: float32(color.G),
			ColorB: float32(color.B), ColorA: float32(color.A),
		})
	}

	targetCopy := target
	a.pendingTarget = &targetCopy
	return nil
}

func sameTarget(a *gg.GPURenderTarget, b *gg.GPURenderTarget) bool {
	return a.Width == b.Width && a.Height == b.Height &&
		len(a.Data) == len(b.Data) && len(a.Data) > 0 && &a.Data[0] == &b.Data[0]
}

// packShapesData serializes batch shapes into a byte slice for GPU upload.
func packBatchShapesData(shapes []SDFBatchShape) []byte {
	shapeSize := int(unsafe.Sizeof(SDFBatchShape{}))
	shapesBytes := make([]byte, shapeSize*len(shapes))
	for i := range shapes {
		src := structToBytes(unsafe.Pointer(&shapes[i]), unsafe.Sizeof(shapes[i])) //nolint:gosec // safe struct access
		copy(shapesBytes[i*shapeSize:], src)
	}
	return shapesBytes
}

// makeFrameParams returns a 16-byte FrameParams for a single shape index.
func makeFrameParams(w, h, shapeIndex uint32) []byte {
	params := SDFBatchFrameParams{
		TargetWidth: w, TargetHeight: h,
		ShapeIndex: shapeIndex,
	}
	return structToBytes(unsafe.Pointer(&params), unsafe.Sizeof(params)) //nolint:gosec // safe struct access
}

// encodeMultiPass creates N compute passes (one per shape) in a single command
// encoder. Each pass processes one shape, with implicit storage buffer barriers
// between passes ensuring correct compositing order.
// This avoids naga SPIR-V bug #5 (loops only execute first iteration).
func (a *SDFAccelerator) encodeMultiPass(
	bindGroups []hal.BindGroup, storageBuf, stagingBuf hal.Buffer,
	w, h uint32, pixelBufSize uint64, target gg.GPURenderTarget,
) error {
	encoder, err := a.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "sdf_batch_encoder"})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	if err := encoder.BeginEncoding("sdf_batch"); err != nil {
		return fmt.Errorf("begin encoding: %w", err)
	}

	// One compute pass per shape — same pipeline, different uniform (shape_index).
	for i, bg := range bindGroups {
		_ = i
		computePass := encoder.BeginComputePass(&hal.ComputePassDescriptor{Label: "sdf_pass"})
		computePass.SetPipeline(a.batchPipeline)
		computePass.SetBindGroup(0, bg, nil)
		computePass.Dispatch((w+7)/8, (h+7)/8, 1)
		computePass.End()
	}

	encoder.CopyBufferToBuffer(storageBuf, stagingBuf, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 0, Size: pixelBufSize},
	})
	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("end encoding: %w", err)
	}
	defer a.device.FreeCommandBuffer(cmdBuf)

	fence, err := a.device.CreateFence()
	if err != nil {
		return fmt.Errorf("create fence: %w", err)
	}
	defer a.device.DestroyFence(fence)
	if err := a.queue.Submit([]hal.CommandBuffer{cmdBuf}, fence, 1); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	fenceOK, err := a.device.Wait(fence, 1, 5*time.Second)
	if err != nil || !fenceOK {
		return fmt.Errorf("wait for GPU: ok=%v err=%w", fenceOK, err)
	}

	readback := make([]byte, pixelBufSize)
	if err := a.queue.ReadBuffer(stagingBuf, 0, readback); err != nil {
		return fmt.Errorf("readback: %w", err)
	}
	unpackPixelsFromGPU(readback, target.Data, int(w*h))
	return nil
}

// dispatchBatchLegacy sends shapes to the GPU using multi-pass compute dispatch.
// Each shape gets its own compute pass in a single command encoder, avoiding
// naga SPIR-V bug #5 (loops only execute the first iteration).
// One submit + one fence wait for the entire batch.
func (a *SDFAccelerator) dispatchBatchLegacy(target gg.GPURenderTarget, shapes []SDFBatchShape) error {
	w, h := uint32(target.Width), uint32(target.Height) //nolint:gosec // dimensions always fit uint32
	pixelBufSize := uint64(w * h * 4)
	shapesBytes := packBatchShapesData(shapes)
	packedPixels := packPixelsForGPU(target.Data, int(w*h))
	n := len(shapes)

	// Create shared buffers: shapes (all shapes) + pixels (storage) + staging (readback).
	shapesBuf, err := a.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "sdf_shapes", Size: uint64(len(shapesBytes)),
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create shapes buffer: %w", err)
	}
	defer a.device.DestroyBuffer(shapesBuf)

	storageBuf, err := a.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "sdf_pixels", Size: pixelBufSize,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create storage buffer: %w", err)
	}
	defer a.device.DestroyBuffer(storageBuf)

	stagingBuf, err := a.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "sdf_staging", Size: pixelBufSize,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create staging buffer: %w", err)
	}
	defer a.device.DestroyBuffer(stagingBuf)

	a.queue.WriteBuffer(shapesBuf, 0, shapesBytes)
	a.queue.WriteBuffer(storageBuf, 0, packedPixels)

	// Create per-shape uniform buffers and bind groups.
	uniformBufs, bindGroups, err := a.createPerShapeBindings(n, w, h, shapesBuf, shapesBytes, storageBuf, pixelBufSize)
	if err != nil {
		a.cleanupBindings(uniformBufs, bindGroups)
		return err
	}
	defer a.cleanupBindings(uniformBufs, bindGroups)

	return a.encodeMultiPass(bindGroups, storageBuf, stagingBuf, w, h, pixelBufSize, target)
}

// createPerShapeBindings creates N uniform buffers (one per shape with shape_index)
// and N bind groups. Each bind group shares the same shapes and pixels buffers.
func (a *SDFAccelerator) createPerShapeBindings(
	n int, w, h uint32,
	shapesBuf hal.Buffer, shapesBytes []byte,
	storageBuf hal.Buffer, pixelBufSize uint64,
) ([]hal.Buffer, []hal.BindGroup, error) {
	paramSize := uint64(unsafe.Sizeof(SDFBatchFrameParams{}))
	uniformBufs := make([]hal.Buffer, 0, n)
	bindGroups := make([]hal.BindGroup, 0, n)

	for i := 0; i < n; i++ {
		paramsBytes := makeFrameParams(w, h, uint32(i)) //nolint:gosec // shape index fits uint32

		ub, err := a.device.CreateBuffer(&hal.BufferDescriptor{
			Label: "sdf_params", Size: paramSize,
			Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			return uniformBufs, bindGroups, fmt.Errorf("create uniform buffer %d: %w", i, err)
		}
		uniformBufs = append(uniformBufs, ub)
		a.queue.WriteBuffer(ub, 0, paramsBytes)

		bg, err := a.device.CreateBindGroup(&hal.BindGroupDescriptor{
			Label: "sdf_bind", Layout: a.batchBindLayout,
			Entries: []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.BufferBinding{Buffer: ub.NativeHandle(), Offset: 0, Size: paramSize}},
				{Binding: 1, Resource: gputypes.BufferBinding{Buffer: shapesBuf.NativeHandle(), Offset: 0, Size: uint64(len(shapesBytes))}},
				{Binding: 2, Resource: gputypes.BufferBinding{Buffer: storageBuf.NativeHandle(), Offset: 0, Size: pixelBufSize}},
			},
		})
		if err != nil {
			return uniformBufs, bindGroups, fmt.Errorf("create bind group %d: %w", i, err)
		}
		bindGroups = append(bindGroups, bg)
	}

	return uniformBufs, bindGroups, nil
}

// cleanupBindings destroys uniform buffers and bind groups.
func (a *SDFAccelerator) cleanupBindings(uniformBufs []hal.Buffer, bindGroups []hal.BindGroup) {
	for _, bg := range bindGroups {
		if bg != nil {
			a.device.DestroyBindGroup(bg)
		}
	}
	for _, ub := range uniformBufs {
		if ub != nil {
			a.device.DestroyBuffer(ub)
		}
	}
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
	if err := a.createPipelines(); err != nil {
		a.device.Destroy()
		a.device = nil
		a.queue = nil
		return fmt.Errorf("create pipelines: %w", err)
	}

	// Create SDF render pipeline (preferred over compute).
	a.sdfRenderPipeline = NewSDFRenderPipeline(a.device, a.queue)
	a.useRenderPipeline = true

	a.gpuReady = true
	log.Printf("gpu-sdf: GPU accelerator initialized (%s)", selected.Info.Name)
	return nil
}

func (a *SDFAccelerator) createPipelines() error {
	batchShader, err := a.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "sdf_batch",
		Source: hal.ShaderSource{WGSL: sdfBatchShaderSource},
	})
	if err != nil {
		return fmt.Errorf("compile sdf_batch shader: %w", err)
	}
	a.batchShader = batchShader

	batchBindLayout, err := a.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "sdf_batch_bind_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 2, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		return fmt.Errorf("create batch bind group layout: %w", err)
	}
	a.batchBindLayout = batchBindLayout

	batchPipeLayout, err := a.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label: "sdf_batch_pipe_layout", BindGroupLayouts: []hal.BindGroupLayout{a.batchBindLayout},
	})
	if err != nil {
		return fmt.Errorf("create batch pipeline layout: %w", err)
	}
	a.batchPipeLayout = batchPipeLayout

	batchPipeline, err := a.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "sdf_batch_pipeline", Layout: a.batchPipeLayout,
		Compute: hal.ComputeState{Module: a.batchShader, EntryPoint: "main"},
	})
	if err != nil {
		return fmt.Errorf("create batch compute pipeline: %w", err)
	}
	a.batchPipeline = batchPipeline

	return nil
}

func (a *SDFAccelerator) destroyPipelines() {
	if a.device == nil {
		return
	}
	if a.batchPipeline != nil {
		a.device.DestroyComputePipeline(a.batchPipeline)
	}
	if a.batchPipeLayout != nil {
		a.device.DestroyPipelineLayout(a.batchPipeLayout)
	}
	if a.batchBindLayout != nil {
		a.device.DestroyBindGroupLayout(a.batchBindLayout)
	}
	if a.batchShader != nil {
		a.device.DestroyShaderModule(a.batchShader)
	}
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

func structToBytes(ptr unsafe.Pointer, size uintptr) []byte {
	return unsafe.Slice((*byte)(ptr), size) //nolint:gosec // safe struct serialization
}

func packPixelsForGPU(data []uint8, pixelCount int) []byte {
	out := make([]byte, pixelCount*4)
	for i := 0; i < pixelCount; i++ {
		srcIdx := i * 4
		r := uint32(data[srcIdx+0])
		g := uint32(data[srcIdx+1])
		b := uint32(data[srcIdx+2])
		a := uint32(data[srcIdx+3])
		packed := r | (g << 8) | (b << 16) | (a << 24)
		binary.LittleEndian.PutUint32(out[i*4:], packed)
	}
	return out
}

func unpackPixelsFromGPU(packed []byte, dst []uint8, pixelCount int) {
	for i := 0; i < pixelCount; i++ {
		val := binary.LittleEndian.Uint32(packed[i*4:])
		dstIdx := i * 4
		dst[dstIdx+0] = uint8(val & 0xFF)         //nolint:gosec // masked to 8 bits
		dst[dstIdx+1] = uint8((val >> 8) & 0xFF)  //nolint:gosec // masked to 8 bits
		dst[dstIdx+2] = uint8((val >> 16) & 0xFF) //nolint:gosec // masked to 8 bits
		dst[dstIdx+3] = uint8((val >> 24) & 0xFF) //nolint:gosec // masked to 8 bits
	}
}
