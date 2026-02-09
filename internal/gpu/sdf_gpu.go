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
type SDFAccelerator struct {
	mu sync.Mutex

	instance hal.Instance
	device   hal.Device
	queue    hal.Queue

	circleShader     hal.ShaderModule
	circleBindLayout hal.BindGroupLayout
	circlePipeLayout hal.PipelineLayout
	circlePipeline   hal.ComputePipeline

	rrectShader     hal.ShaderModule
	rrectBindLayout hal.BindGroupLayout
	rrectPipeLayout hal.PipelineLayout
	rrectPipeline   hal.ComputePipeline

	cpuFallback    gg.SDFAccelerator
	gpuReady       bool
	externalDevice bool // true when using shared device (don't destroy on Close)
}

var _ gg.GPUAccelerator = (*SDFAccelerator)(nil)

func (a *SDFAccelerator) Name() string { return "sdf-gpu" }

func (a *SDFAccelerator) CanAccelerate(op gg.AcceleratedOp) bool {
	return op&(gg.AccelCircleSDF|gg.AccelRRectSDF) != 0
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
	a.gpuReady = true
	log.Printf("gpu-sdf: switched to shared GPU device")
	return nil
}

func (a *SDFAccelerator) FillPath(_ gg.GPURenderTarget, _ *gg.Path, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

func (a *SDFAccelerator) StrokePath(_ gg.GPURenderTarget, _ *gg.Path, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

func (a *SDFAccelerator) FillShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.gpuReady {
		return a.cpuFallback.FillShape(target, shape, paint)
	}
	var err error
	switch shape.Kind {
	case gg.ShapeCircle, gg.ShapeEllipse:
		err = a.dispatchCircleSDF(target, shape, paint, false)
	case gg.ShapeRect, gg.ShapeRRect:
		err = a.dispatchRRectSDF(target, shape, paint, false)
	default:
		return gg.ErrFallbackToCPU
	}
	if err != nil {
		log.Printf("gpu-sdf: FillShape dispatch error: %v (shape=%d, external=%v)", err, shape.Kind, a.externalDevice)
	}
	return err
}

func (a *SDFAccelerator) StrokeShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.gpuReady {
		return a.cpuFallback.StrokeShape(target, shape, paint)
	}
	switch shape.Kind {
	case gg.ShapeCircle, gg.ShapeEllipse:
		return a.dispatchCircleSDF(target, shape, paint, true)
	case gg.ShapeRect, gg.ShapeRRect:
		return a.dispatchRRectSDF(target, shape, paint, true)
	default:
		return gg.ErrFallbackToCPU
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
	a.gpuReady = true
	log.Printf("gpu-sdf: GPU accelerator initialized (%s)", selected.Info.Name)
	return nil
}

func (a *SDFAccelerator) createPipelines() error {
	circleShader, err := a.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "sdf_circle",
		Source: hal.ShaderSource{WGSL: sdfCircleShaderSource},
	})
	if err != nil {
		return fmt.Errorf("compile sdf_circle shader: %w", err)
	}
	a.circleShader = circleShader

	rrectShader, err := a.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "sdf_rrect",
		Source: hal.ShaderSource{WGSL: sdfRRectShaderSource},
	})
	if err != nil {
		return fmt.Errorf("compile sdf_rrect shader: %w", err)
	}
	a.rrectShader = rrectShader

	circleBindLayout, err := a.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "sdf_circle_bind_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		return fmt.Errorf("create circle bind group layout: %w", err)
	}
	a.circleBindLayout = circleBindLayout

	rrectBindLayout, err := a.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "sdf_rrect_bind_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		return fmt.Errorf("create rrect bind group layout: %w", err)
	}
	a.rrectBindLayout = rrectBindLayout

	circlePipeLayout, err := a.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label: "sdf_circle_pipe_layout", BindGroupLayouts: []hal.BindGroupLayout{a.circleBindLayout},
	})
	if err != nil {
		return fmt.Errorf("create circle pipeline layout: %w", err)
	}
	a.circlePipeLayout = circlePipeLayout

	rrectPipeLayout, err := a.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label: "sdf_rrect_pipe_layout", BindGroupLayouts: []hal.BindGroupLayout{a.rrectBindLayout},
	})
	if err != nil {
		return fmt.Errorf("create rrect pipeline layout: %w", err)
	}
	a.rrectPipeLayout = rrectPipeLayout

	circlePipeline, err := a.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "sdf_circle_pipeline", Layout: a.circlePipeLayout,
		Compute: hal.ComputeState{Module: a.circleShader, EntryPoint: "main"},
	})
	if err != nil {
		return fmt.Errorf("create circle compute pipeline: %w", err)
	}
	a.circlePipeline = circlePipeline

	rrectPipeline, err := a.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "sdf_rrect_pipeline", Layout: a.rrectPipeLayout,
		Compute: hal.ComputeState{Module: a.rrectShader, EntryPoint: "main"},
	})
	if err != nil {
		return fmt.Errorf("create rrect compute pipeline: %w", err)
	}
	a.rrectPipeline = rrectPipeline

	return nil
}

func (a *SDFAccelerator) destroyPipelines() {
	if a.device == nil {
		return
	}
	if a.circlePipeline != nil {
		a.device.DestroyComputePipeline(a.circlePipeline)
	}
	if a.rrectPipeline != nil {
		a.device.DestroyComputePipeline(a.rrectPipeline)
	}
	if a.circlePipeLayout != nil {
		a.device.DestroyPipelineLayout(a.circlePipeLayout)
	}
	if a.rrectPipeLayout != nil {
		a.device.DestroyPipelineLayout(a.rrectPipeLayout)
	}
	if a.circleBindLayout != nil {
		a.device.DestroyBindGroupLayout(a.circleBindLayout)
	}
	if a.rrectBindLayout != nil {
		a.device.DestroyBindGroupLayout(a.rrectBindLayout)
	}
	if a.circleShader != nil {
		a.device.DestroyShaderModule(a.circleShader)
	}
	if a.rrectShader != nil {
		a.device.DestroyShaderModule(a.rrectShader)
	}
}

func (a *SDFAccelerator) dispatchCircleSDF(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint, stroked bool) error {
	color := getColorFromPaint(paint)
	var halfStroke float32
	var isStroked uint32
	if stroked {
		halfStroke = float32(paint.EffectiveLineWidth() / 2)
		isStroked = 1
	}
	params := SDFCircleParams{
		CenterX: float32(shape.CenterX), CenterY: float32(shape.CenterY),
		RadiusX: float32(shape.RadiusX), RadiusY: float32(shape.RadiusY),
		HalfStrokeWidth: halfStroke, IsStroked: isStroked,
		ColorR: float32(color.R), ColorG: float32(color.G),
		ColorB: float32(color.B), ColorA: float32(color.A),
		TargetWidth: uint32(target.Width), TargetHeight: uint32(target.Height),
	}
	return a.dispatchCompute(a.circlePipeline, a.circleBindLayout,
		structToBytes(unsafe.Pointer(&params), unsafe.Sizeof(params)), target)
}

func (a *SDFAccelerator) dispatchRRectSDF(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint, stroked bool) error {
	color := getColorFromPaint(paint)
	var halfStroke float32
	var isStroked uint32
	if stroked {
		halfStroke = float32(paint.EffectiveLineWidth() / 2)
		isStroked = 1
	}
	params := SDFRRectParams{
		CenterX: float32(shape.CenterX), CenterY: float32(shape.CenterY),
		HalfWidth: float32(shape.Width / 2), HalfHeight: float32(shape.Height / 2),
		CornerRadius: float32(shape.CornerRadius), HalfStrokeWidth: halfStroke,
		IsStroked: isStroked,
		ColorR:    float32(color.R), ColorG: float32(color.G),
		ColorB: float32(color.B), ColorA: float32(color.A),
		TargetWidth: uint32(target.Width), TargetHeight: uint32(target.Height),
	}
	return a.dispatchCompute(a.rrectPipeline, a.rrectBindLayout,
		structToBytes(unsafe.Pointer(&params), unsafe.Sizeof(params)), target)
}

func (a *SDFAccelerator) dispatchCompute(
	pipeline hal.ComputePipeline, bindLayout hal.BindGroupLayout,
	paramsBytes []byte, target gg.GPURenderTarget,
) error {
	w, h := uint32(target.Width), uint32(target.Height)
	pixelCount := w * h
	pixelBufSize := uint64(pixelCount * 4)
	packedPixels := packPixelsForGPU(target.Data, int(pixelCount))

	uniformBuf, err := a.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "sdf_params", Size: uint64(len(paramsBytes)),
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create uniform buffer: %w", err)
	}
	defer a.device.DestroyBuffer(uniformBuf)

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

	a.queue.WriteBuffer(uniformBuf, 0, paramsBytes)
	a.queue.WriteBuffer(storageBuf, 0, packedPixels)

	bindGroup, err := a.device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label: "sdf_bind_group", Layout: bindLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: uniformBuf.NativeHandle(), Offset: 0, Size: uint64(len(paramsBytes))}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: storageBuf.NativeHandle(), Offset: 0, Size: pixelBufSize}},
		},
	})
	if err != nil {
		return fmt.Errorf("create bind group: %w", err)
	}
	defer a.device.DestroyBindGroup(bindGroup)

	encoder, err := a.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "sdf_encoder"})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	if err := encoder.BeginEncoding("sdf_compute"); err != nil {
		return fmt.Errorf("begin encoding: %w", err)
	}

	computePass := encoder.BeginComputePass(&hal.ComputePassDescriptor{Label: "sdf_pass"})
	computePass.SetPipeline(pipeline)
	computePass.SetBindGroup(0, bindGroup, nil)
	wgX := (w + 7) / 8
	wgY := (h + 7) / 8
	computePass.Dispatch(wgX, wgY, 1)
	computePass.End()

	// Copy compute output to readback staging buffer
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
	unpackPixelsFromGPU(readback, target.Data, int(pixelCount))
	return nil
}

func getColorFromPaint(paint *gg.Paint) gg.RGBA {
	if paint.Brush != nil {
		if sb, isSolid := paint.Brush.(gg.SolidBrush); isSolid {
			return sb.Color
		}
		return paint.Brush.ColorAt(0, 0)
	}
	if paint.Pattern != nil {
		return paint.Pattern.ColorAt(0, 0)
	}
	return gg.Black
}

func structToBytes(ptr unsafe.Pointer, size uintptr) []byte {
	return unsafe.Slice((*byte)(ptr), size)
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
		dst[dstIdx+0] = uint8(val & 0xFF)
		dst[dstIdx+1] = uint8((val >> 8) & 0xFF)
		dst[dstIdx+2] = uint8((val >> 16) & 0xFF)
		dst[dstIdx+3] = uint8((val >> 24) & 0xFF)
	}
}
