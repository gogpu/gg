//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"log"
	"testing"
	"time"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestSDFPipelineCircle(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	w, h := 200, 200
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	accel := &SDFAccelerator{}
	if err := accel.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer accel.Close()

	target := gg.GPURenderTarget{
		Width:  w,
		Height: h,
		Data:   img.Pix,
	}

	shape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 100,
		CenterY: 100,
		RadiusX: 60,
		RadiusY: 60,
	}
	paint := &gg.Paint{
		Brush: gg.SolidBrush{Color: gg.RGBA{R: 1, G: 0, B: 0, A: 1}},
	}

	if err := accel.FillShape(target, shape, paint); err != nil {
		t.Fatalf("FillShape: %v", err)
	}
	if err := accel.Flush(target); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	nonWhite := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 255 || img.Pix[i+1] != 255 || img.Pix[i+2] != 255 {
			nonWhite++
		}
	}

	t.Logf("Result: %d non-white pixels out of %d", nonWhite, w*h)
	if nonWhite == 0 {
		t.Fatal("GPU SDF produced zero non-white pixels")
	}
	t.Logf("PASS: GPU SDF circle rendered %d pixels", nonWhite)
}

func TestSDFPipelineLargeCanvas(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	accel := &SDFAccelerator{}
	if err := accel.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer accel.Close()

	// Test multiple sizes with SINGLE accelerator
	sizes := []struct{ w, h int }{
		{200, 200},
		{400, 400},
		{784, 561},
	}

	for _, sz := range sizes {
		w, h := sz.w, sz.h
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
		target := gg.GPURenderTarget{Width: w, Height: h, Data: img.Pix}
		shape := gg.DetectedShape{
			Kind: gg.ShapeCircle, CenterX: float64(w) / 2, CenterY: float64(h) / 2,
			RadiusX: 60, RadiusY: 60,
		}
		paint := &gg.Paint{Brush: gg.SolidBrush{Color: gg.RGBA{R: 1, G: 0, B: 0, A: 1}}}
		if err := accel.FillShape(target, shape, paint); err != nil {
			t.Fatalf("FillShape %dx%d: %v", w, h, err)
		}
		if err := accel.Flush(target); err != nil {
			t.Fatalf("Flush %dx%d: %v", w, h, err)
		}
		nonWhite := 0
		for i := 0; i < len(img.Pix); i += 4 {
			if img.Pix[i] != 255 || img.Pix[i+1] != 255 || img.Pix[i+2] != 255 {
				nonWhite++
			}
		}
		status := "PASS"
		if nonWhite == 0 {
			status = "FAIL"
		}
		t.Logf("%s: %dx%d → %d non-white pixels", status, w, h, nonWhite)
	}
}

// TestSDFPipelineZeroInput tests if FillShape works with all-zero input data.
// This isolates whether the shader handles zero (transparent) input correctly.
func TestSDFPipelineZeroInput(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	accel := &SDFAccelerator{}
	if err := accel.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer accel.Close()

	w, h := 200, 200
	shape := gg.DetectedShape{
		Kind: gg.ShapeCircle, CenterX: 100, CenterY: 100,
		RadiusX: 60, RadiusY: 60,
	}
	paint := &gg.Paint{Brush: gg.SolidBrush{Color: gg.RGBA{R: 1, G: 0, B: 0, A: 1}}}

	// Test 1: known pattern input (0xAA per byte)
	// If readback has 0xAA → shader didn't run
	// If readback has red circle → shader ran
	// If readback is all zeros → copy or readback is broken
	patternData := make([]byte, w*h*4)
	for i := range patternData {
		patternData[i] = 0xAA
	}
	target1 := gg.GPURenderTarget{Width: w, Height: h, Data: patternData}
	if err := accel.FillShape(target1, shape, paint); err != nil {
		t.Fatalf("FillShape (pattern): %v", err)
	}
	if err := accel.Flush(target1); err != nil {
		t.Fatalf("Flush (pattern): %v", err)
	}
	patternAA := 0
	patternZero := 0
	patternOther := 0
	for i := 0; i < len(patternData); i++ {
		switch patternData[i] {
		case 0xAA:
			patternAA++
		case 0x00:
			patternZero++
		default:
			patternOther++
		}
	}
	t.Logf("Pattern test: 0xAA=%d  zero=%d  other=%d  total=%d", patternAA, patternZero, patternOther, len(patternData))

	// Test 2: zero input
	zeroData := make([]byte, w*h*4)
	target2 := gg.GPURenderTarget{Width: w, Height: h, Data: zeroData}
	if err := accel.FillShape(target2, shape, paint); err != nil {
		t.Fatalf("FillShape (zero): %v", err)
	}
	if err := accel.Flush(target2); err != nil {
		t.Fatalf("Flush (zero): %v", err)
	}
	zeroNonZero := 0
	for i := 0; i < len(zeroData); i++ {
		if zeroData[i] != 0 {
			zeroNonZero++
		}
	}
	t.Logf("Zero-input: %d non-zero bytes out of %d", zeroNonZero, len(zeroData))

	// Test 3: 0xFF input (known working)
	whiteData := make([]byte, w*h*4)
	for i := range whiteData {
		whiteData[i] = 0xFF
	}
	target3 := gg.GPURenderTarget{Width: w, Height: h, Data: whiteData}
	if err := accel.FillShape(target3, shape, paint); err != nil {
		t.Fatalf("FillShape (white): %v", err)
	}
	if err := accel.Flush(target3); err != nil {
		t.Fatalf("Flush (white): %v", err)
	}
	whiteNonFF := 0
	for i := 0; i < len(whiteData); i++ {
		if whiteData[i] != 0xFF {
			whiteNonFF++
		}
	}
	t.Logf("White-input: %d non-0xFF bytes out of %d", whiteNonFF, len(whiteData))

	// Print center pixel value for each test
	centerIdx := (100*w + 100) * 4
	t.Logf("Center pixel (100,100): pattern=[%02x,%02x,%02x,%02x] zero=[%02x,%02x,%02x,%02x] white=[%02x,%02x,%02x,%02x]",
		patternData[centerIdx], patternData[centerIdx+1], patternData[centerIdx+2], patternData[centerIdx+3],
		zeroData[centerIdx], zeroData[centerIdx+1], zeroData[centerIdx+2], zeroData[centerIdx+3],
		whiteData[centerIdx], whiteData[centerIdx+1], whiteData[centerIdx+2], whiteData[centerIdx+3])
	// Print edge pixel value (100+59, 100) — just inside circle edge
	edgeIdx := (100*w + 159) * 4
	t.Logf("Edge pixel (159,100): pattern=[%02x,%02x,%02x,%02x] zero=[%02x,%02x,%02x,%02x] white=[%02x,%02x,%02x,%02x]",
		patternData[edgeIdx], patternData[edgeIdx+1], patternData[edgeIdx+2], patternData[edgeIdx+3],
		zeroData[edgeIdx], zeroData[edgeIdx+1], zeroData[edgeIdx+2], zeroData[edgeIdx+3],
		whiteData[edgeIdx], whiteData[edgeIdx+1], whiteData[edgeIdx+2], whiteData[edgeIdx+3])
	// Print outside pixel (100+70, 100)
	outIdx := (100*w + 170) * 4
	t.Logf("Outside pixel (170,100): pattern=[%02x,%02x,%02x,%02x] zero=[%02x,%02x,%02x,%02x] white=[%02x,%02x,%02x,%02x]",
		patternData[outIdx], patternData[outIdx+1], patternData[outIdx+2], patternData[outIdx+3],
		zeroData[outIdx], zeroData[outIdx+1], zeroData[outIdx+2], zeroData[outIdx+3],
		whiteData[outIdx], whiteData[outIdx+1], whiteData[outIdx+2], whiteData[outIdx+3])

	if patternOther == 0 && patternZero == len(patternData) {
		t.Fatal("Pattern test: ALL zeros — shader didn't run AND copy/readback broken")
	}
	if patternOther == 0 && patternAA == len(patternData) {
		t.Fatal("Pattern test: ALL 0xAA — shader didn't run, but copy+readback works")
	}
	if zeroNonZero == 0 {
		t.Error("Zero-input: readback all zeros")
	}
	if whiteNonFF == 0 {
		t.Error("White-input: readback all 0xFF (no change)")
	}
}

// TestSDFConstantWrite uses a minimal shader that writes 0xDEADBEEF to every
// pixel in the storage buffer. This isolates whether OpStore/OpAccessChain in
// naga's SPIR-V output works correctly.
//
// Result interpretation:
//   - Readback contains 0xDEADBEEF  -> OpStore works, bug is in expression computation
//   - Readback contains zeros       -> OpStore is broken (naga codegen issue)
//   - Readback contains 0xAA pattern -> shader didn't run at all
func TestSDFConstantWrite(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	// --- GPU init (same as SDFAccelerator.initGPU) ---
	backend, ok := hal.GetBackend(gputypes.BackendVulkan)
	if !ok {
		t.Fatal("vulkan backend not available")
	}
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{Flags: 0})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Fatal("no GPU adapters found")
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
		t.Fatalf("open device: %v", err)
	}
	device := openDev.Device
	queue := openDev.Queue
	defer device.Destroy()

	t.Logf("GPU: %s", selected.Info.Name)

	// --- Compile the constant-write test shader ---
	shader, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "sdf_test_const",
		Source: hal.ShaderSource{WGSL: sdfTestConstShaderSource},
	})
	if err != nil {
		t.Fatalf("compile test shader: %v", err)
	}
	defer device.DestroyShaderModule(shader)

	// --- Create pipeline (same bind layout as circle shader) ---
	bindLayout, err := device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "test_bind_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		t.Fatalf("create bind layout: %v", err)
	}
	defer device.DestroyBindGroupLayout(bindLayout)

	pipeLayout, err := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label: "test_pipe_layout", BindGroupLayouts: []hal.BindGroupLayout{bindLayout},
	})
	if err != nil {
		t.Fatalf("create pipe layout: %v", err)
	}
	defer device.DestroyPipelineLayout(pipeLayout)

	pipeline, err := device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "test_pipeline", Layout: pipeLayout,
		Compute: hal.ComputeState{Module: shader, EntryPoint: "main"},
	})
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	defer device.DestroyComputePipeline(pipeline)

	// --- Prepare buffers ---
	w, h := uint32(200), uint32(200)
	pixelCount := w * h
	pixelBufSize := uint64(pixelCount * 4)

	// Fill params with valid data (target_width, target_height must be correct)
	params := SDFCircleParams{
		CenterX: 100, CenterY: 100, RadiusX: 60, RadiusY: 60,
		ColorR: 1, ColorG: 0, ColorB: 0, ColorA: 1,
		TargetWidth: w, TargetHeight: h,
	}
	paramsBytes := structToBytes(unsafe.Pointer(&params), unsafe.Sizeof(params))

	// Fill initial pixel data with 0xAA pattern so we can detect changes
	initialPixels := make([]byte, pixelBufSize)
	for i := range initialPixels {
		initialPixels[i] = 0xAA
	}

	uniformBuf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "test_params", Size: uint64(len(paramsBytes)),
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("create uniform buffer: %v", err)
	}
	defer device.DestroyBuffer(uniformBuf)

	storageBuf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "test_pixels", Size: pixelBufSize,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("create storage buffer: %v", err)
	}
	defer device.DestroyBuffer(storageBuf)

	stagingBuf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "test_staging", Size: pixelBufSize,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("create staging buffer: %v", err)
	}
	defer device.DestroyBuffer(stagingBuf)

	// Upload data
	queue.WriteBuffer(uniformBuf, 0, paramsBytes)
	queue.WriteBuffer(storageBuf, 0, initialPixels)

	// --- Create bind group ---
	bindGroup, err := device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label: "test_bind_group", Layout: bindLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: uniformBuf.NativeHandle(), Offset: 0, Size: uint64(len(paramsBytes))}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: storageBuf.NativeHandle(), Offset: 0, Size: pixelBufSize}},
		},
	})
	if err != nil {
		t.Fatalf("create bind group: %v", err)
	}
	defer device.DestroyBindGroup(bindGroup)

	// --- Dispatch compute ---
	encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "test_encoder"})
	if err != nil {
		t.Fatalf("create encoder: %v", err)
	}
	if err := encoder.BeginEncoding("test_compute"); err != nil {
		t.Fatalf("begin encoding: %v", err)
	}
	computePass := encoder.BeginComputePass(&hal.ComputePassDescriptor{Label: "test_pass"})
	computePass.SetPipeline(pipeline)
	computePass.SetBindGroup(0, bindGroup, nil)
	wgX := (w + 7) / 8
	wgY := (h + 7) / 8
	computePass.Dispatch(wgX, wgY, 1)
	computePass.End()
	encoder.CopyBufferToBuffer(storageBuf, stagingBuf, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 0, Size: pixelBufSize},
	})
	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		t.Fatalf("end encoding: %v", err)
	}
	defer device.FreeCommandBuffer(cmdBuf)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("create fence: %v", err)
	}
	defer device.DestroyFence(fence)
	if err := queue.Submit([]hal.CommandBuffer{cmdBuf}, fence, 1); err != nil {
		t.Fatalf("submit: %v", err)
	}
	fenceOK, err := device.Wait(fence, 1, 5*time.Second)
	if err != nil || !fenceOK {
		t.Fatalf("wait: ok=%v err=%v", fenceOK, err)
	}

	// --- Readback ---
	readback := make([]byte, pixelBufSize)
	if err := queue.ReadBuffer(stagingBuf, 0, readback); err != nil {
		t.Fatalf("readback: %v", err)
	}

	// --- Analyze results ---
	deadbeefCount := 0
	zeroCount := 0
	aaCount := 0
	otherCount := 0
	for i := uint32(0); i < pixelCount; i++ {
		val := binary.LittleEndian.Uint32(readback[i*4:])
		switch val {
		case 0xDEADBEEF:
			deadbeefCount++
		case 0x00000000:
			zeroCount++
		case 0xAAAAAAAA:
			aaCount++
		default:
			otherCount++
		}
	}

	t.Logf("=== CONSTANT WRITE TEST RESULTS ===")
	t.Logf("Total pixels: %d", pixelCount)
	t.Logf("0xDEADBEEF: %d (OpStore works!)", deadbeefCount)
	t.Logf("0x00000000: %d (zeros)", zeroCount)
	t.Logf("0xAAAAAAAA: %d (unchanged pattern)", aaCount)
	t.Logf("Other:      %d", otherCount)

	// Print first 10 u32 values for diagnostics
	t.Logf("First 10 u32 values from readback:")
	for i := 0; i < 10 && i < int(pixelCount); i++ {
		val := binary.LittleEndian.Uint32(readback[i*4:])
		t.Logf("  [%d] = 0x%08X", i, val)
	}
	// Also print some middle values (center of image)
	centerStart := int(100*w + 95)
	t.Logf("Center row values (y=100, x=95..104):")
	for i := centerStart; i < centerStart+10 && i < int(pixelCount); i++ {
		val := binary.LittleEndian.Uint32(readback[i*4:])
		t.Logf("  [%d] = 0x%08X", i, val)
	}

	// Verdict
	switch {
	case deadbeefCount == int(pixelCount):
		t.Log("VERDICT: OpStore WORKS. Bug is in expression computation (bitwise OR, shifts, u32 conversion).")
	case zeroCount == int(pixelCount):
		t.Log("VERDICT: All zeros. OpStore is BROKEN or shader did not run.")
	case aaCount == int(pixelCount):
		t.Log("VERDICT: All 0xAA. Shader did NOT execute at all. Dispatch or pipeline issue.")
	case deadbeefCount > 0:
		t.Logf("VERDICT: Partial success. %d pixels have 0xDEADBEEF, %d zeros, %d unchanged. Possible dispatch size mismatch.", deadbeefCount, zeroCount, aaCount)
	default:
		t.Logf("VERDICT: Unexpected mix. Investigate: deadbeef=%d zero=%d aa=%d other=%d", deadbeefCount, zeroCount, aaCount, otherCount)
	}

	if deadbeefCount == 0 {
		t.Fatal("FAIL: No pixels contain 0xDEADBEEF - constant write test failed")
	}
}

// shaderPreamble is the common Params struct and bindings for all progressive test shaders.
// Matches the sdf_circle.wgsl layout: binding 0 = uniform params, binding 1 = storage pixels.
const shaderPreamble = `
struct Params {
    center_x: f32,
    center_y: f32,
    radius_x: f32,
    radius_y: f32,
    half_stroke_width: f32,
    is_stroked: u32,
    color_r: f32,
    color_g: f32,
    color_b: f32,
    color_a: f32,
    target_width: u32,
    target_height: u32,
}

@group(0) @binding(0) var<uniform> params: Params;
@group(0) @binding(1) var<storage, read_write> pixels: array<u32>;
`

// testGPUEnv holds shared GPU resources for test helpers.
type testGPUEnv struct {
	t        *testing.T
	device   hal.Device
	queue    hal.Queue
	instance hal.Instance
}

// newTestGPUEnv creates a Vulkan device for testing. Call env.destroy() when done.
func newTestGPUEnv(t *testing.T) *testGPUEnv {
	t.Helper()
	backend, ok := hal.GetBackend(gputypes.BackendVulkan)
	if !ok {
		t.Fatal("vulkan backend not available")
	}
	inst, err := backend.CreateInstance(&hal.InstanceDescriptor{Flags: 0})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		inst.Destroy()
		t.Fatal("no GPU adapters found")
	}
	var sel *hal.ExposedAdapter
	for i := range adapters {
		if adapters[i].Info.DeviceType == gputypes.DeviceTypeDiscreteGPU ||
			adapters[i].Info.DeviceType == gputypes.DeviceTypeIntegratedGPU {
			sel = &adapters[i]
			break
		}
	}
	if sel == nil {
		sel = &adapters[0]
	}
	od, err := sel.Adapter.Open(gputypes.Features(0), gputypes.DefaultLimits())
	if err != nil {
		inst.Destroy()
		t.Fatalf("open device: %v", err)
	}
	t.Logf("GPU: %s", sel.Info.Name)
	return &testGPUEnv{t: t, device: od.Device, queue: od.Queue, instance: inst}
}

func (e *testGPUEnv) destroy() {
	e.device.Destroy()
	e.instance.Destroy()
}

// runShader compiles wgslSource, dispatches it on a w*h pixel buffer with
// the given SDFCircleParams, and returns the readback as a []uint32.
// It accepts a *testing.T so subtests can use their own t for Fatalf.
func (e *testGPUEnv) runShader(t *testing.T, wgslSource string, params SDFCircleParams, w, h uint32) []uint32 {
	t.Helper()

	shader, err := e.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label: "prog_test", Source: hal.ShaderSource{WGSL: wgslSource},
	})
	if err != nil {
		t.Fatalf("compile shader: %v", err)
	}
	defer e.device.DestroyShaderModule(shader)

	bindLayout, err := e.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "prog_bind",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		t.Fatalf("create bind layout: %v", err)
	}
	defer e.device.DestroyBindGroupLayout(bindLayout)

	pipeLayout, err := e.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label: "prog_pipe", BindGroupLayouts: []hal.BindGroupLayout{bindLayout},
	})
	if err != nil {
		t.Fatalf("create pipe layout: %v", err)
	}
	defer e.device.DestroyPipelineLayout(pipeLayout)

	pipeline, err := e.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "prog_pipeline", Layout: pipeLayout,
		Compute: hal.ComputeState{Module: shader, EntryPoint: "main"},
	})
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	defer e.device.DestroyComputePipeline(pipeline)

	pixelCount := w * h
	pixelBufSize := uint64(pixelCount * 4)
	paramsBytes := structToBytes(unsafe.Pointer(&params), unsafe.Sizeof(params))

	// Initialize storage with 0xAA pattern
	initData := make([]byte, pixelBufSize)
	for i := range initData {
		initData[i] = 0xAA
	}

	uniformBuf, err := e.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "prog_params", Size: uint64(len(paramsBytes)),
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("create uniform: %v", err)
	}
	defer e.device.DestroyBuffer(uniformBuf)

	storageBuf, err := e.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "prog_pixels", Size: pixelBufSize,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer e.device.DestroyBuffer(storageBuf)

	stagingBuf, err := e.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "prog_staging", Size: pixelBufSize,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("create staging: %v", err)
	}
	defer e.device.DestroyBuffer(stagingBuf)

	e.queue.WriteBuffer(uniformBuf, 0, paramsBytes)
	e.queue.WriteBuffer(storageBuf, 0, initData)

	bindGroup, err := e.device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label: "prog_bg", Layout: bindLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: uniformBuf.NativeHandle(), Offset: 0, Size: uint64(len(paramsBytes))}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: storageBuf.NativeHandle(), Offset: 0, Size: pixelBufSize}},
		},
	})
	if err != nil {
		t.Fatalf("create bind group: %v", err)
	}
	defer e.device.DestroyBindGroup(bindGroup)

	encoder, err := e.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "prog_enc"})
	if err != nil {
		t.Fatalf("create encoder: %v", err)
	}
	if err := encoder.BeginEncoding("prog_compute"); err != nil {
		t.Fatalf("begin encoding: %v", err)
	}
	pass := encoder.BeginComputePass(&hal.ComputePassDescriptor{Label: "prog_pass"})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bindGroup, nil)
	pass.Dispatch((w+7)/8, (h+7)/8, 1)
	pass.End()
	encoder.CopyBufferToBuffer(storageBuf, stagingBuf, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 0, Size: pixelBufSize},
	})
	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		t.Fatalf("end encoding: %v", err)
	}
	defer e.device.FreeCommandBuffer(cmdBuf)

	fence, err := e.device.CreateFence()
	if err != nil {
		t.Fatalf("create fence: %v", err)
	}
	defer e.device.DestroyFence(fence)
	if err := e.queue.Submit([]hal.CommandBuffer{cmdBuf}, fence, 1); err != nil {
		t.Fatalf("submit: %v", err)
	}
	ok, err := e.device.Wait(fence, 1, 5*time.Second)
	if err != nil || !ok {
		t.Fatalf("wait: ok=%v err=%v", ok, err)
	}

	readback := make([]byte, pixelBufSize)
	if err := e.queue.ReadBuffer(stagingBuf, 0, readback); err != nil {
		t.Fatalf("readback: %v", err)
	}

	result := make([]uint32, pixelCount)
	for i := uint32(0); i < pixelCount; i++ {
		result[i] = binary.LittleEndian.Uint32(readback[i*4:])
	}
	return result
}

// TestSDFProgressiveOps runs a sequence of shaders with progressively more
// complex expressions to pinpoint exactly which naga SPIR-V operation
// produces zeros instead of computed values.
//
// Each sub-test writes a single u32 to pixels[idx]. With params.color_r=1.0,
// color_a=1.0, target_width=16, target_height=16, we check pixel [0].
func TestSDFProgressiveOps(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	env := newTestGPUEnv(t)
	defer env.destroy()

	// Small buffer for fast iteration
	const w, h = 16, 16
	params := SDFCircleParams{
		CenterX: 8, CenterY: 8, RadiusX: 4, RadiusY: 4,
		ColorR: 1.0, ColorG: 0.0, ColorB: 0.0, ColorA: 1.0,
		TargetWidth: w, TargetHeight: h,
	}

	tests := []struct {
		name     string
		expected uint32 // expected value at pixel [0]
		body     string // shader body inside main()
	}{
		{
			name:     "1_literal_u32",
			expected: 255,
			body:     `pixels[idx] = 255u;`,
		},
		{
			name:     "2_convert_f32_literal",
			expected: 255,
			body:     `pixels[idx] = u32(255.0);`,
		},
		{
			name:     "3_convert_param_mul",
			expected: 255,
			body:     `pixels[idx] = u32(params.color_r * 255.0);`,
		},
		{
			name:     "4_clamp_param_mul",
			expected: 255,
			body:     `pixels[idx] = u32(clamp(params.color_r * 255.0, 0.0, 255.0));`,
		},
		{
			name:     "5_clamp_with_bias",
			expected: 255,
			body:     `pixels[idx] = u32(clamp(params.color_r * 255.0 + 0.5, 0.0, 255.0));`,
		},
		{
			name:     "6_bitwise_or_shift",
			expected: 0xFF0000FF, // r=255, g=0, b=0, a=255
			body: `let ri = u32(clamp(params.color_r * 255.0 + 0.5, 0.0, 255.0));
    let gi = u32(clamp(params.color_g * 255.0 + 0.5, 0.0, 255.0));
    let bi = u32(clamp(params.color_b * 255.0 + 0.5, 0.0, 255.0));
    let ai = u32(clamp(params.color_a * 255.0 + 0.5, 0.0, 255.0));
    pixels[idx] = ri | (gi << 8u) | (bi << 16u) | (ai << 24u);`,
		},
		{
			name:     "7_simple_shift_only",
			expected: 0xFF000000, // just alpha channel
			body:     `pixels[idx] = 255u << 24u;`,
		},
		{
			name:     "8_or_two_constants",
			expected: 0xFF0000FF, // r | a<<24
			body:     `pixels[idx] = 255u | (255u << 24u);`,
		},
		{
			name:     "9_convert_then_or",
			expected: 0xFF0000FF,
			body: `let ri = u32(255.0);
    let ai = u32(255.0);
    pixels[idx] = ri | (ai << 24u);`,
		},
		{
			name:     "10_param_read_only",
			expected: w, // target_width = 16
			body:     `pixels[idx] = params.target_width;`,
		},
		{
			name:     "11_smoothstep_outside",
			expected: 0, // smoothstep(-0.5, 0.5, 10.0) = 1.0, coverage = 1.0-1.0 = 0.0 -> u32(0.0) = 0
			body:     `pixels[idx] = u32(1.0 - smoothstep(-0.5, 0.5, 10.0));`,
		},
		{
			name:     "12_smoothstep_inside",
			expected: 1, // smoothstep(-0.5, 0.5, -10.0) = 0.0, coverage = 1.0-0.0 = 1.0 -> u32(1.0) = 1
			body:     `pixels[idx] = u32(1.0 - smoothstep(-0.5, 0.5, -10.0));`,
		},
		{
			name:     "13_sdf_distance",
			expected: 0, // distance at pixel (0,0) from center (8,8) radius 4: should be positive (outside)
			body: `let px = f32(x) + 0.5 - params.center_x;
    let py = f32(y) + 0.5 - params.center_y;
    let nx = px / params.radius_x;
    let ny = py / params.radius_y;
    let d = length(vec2<f32>(nx, ny)) - 1.0;
    let dist = d * min(params.radius_x, params.radius_y);
    let coverage = 1.0 - smoothstep(-0.5, 0.5, dist);
    // coverage should be 0 for pixel (0,0) which is outside the circle
    pixels[idx] = u32(coverage * 255.0);`,
		},
		{
			name:     "14_sdf_center_pixel",
			expected: 255, // pixel at center of 16x16 image with circle r=4 at (8,8)
			body: `// Override: compute for the center pixel regardless of gid
    let cx = f32(8u) + 0.5 - params.center_x;
    let cy = f32(8u) + 0.5 - params.center_y;
    let nx = cx / params.radius_x;
    let ny = cy / params.radius_y;
    let d = length(vec2<f32>(nx, ny)) - 1.0;
    let dist = d * min(params.radius_x, params.radius_y);
    let coverage = 1.0 - smoothstep(-0.5, 0.5, dist);
    pixels[idx] = u32(coverage * 255.0);`,
		},
		{
			name:     "15_full_color_pack_center",
			expected: 0xFF0000FF, // red with full alpha, for center pixel
			body: `let cx = f32(8u) + 0.5 - params.center_x;
    let cy = f32(8u) + 0.5 - params.center_y;
    let nx = cx / params.radius_x;
    let ny = cy / params.radius_y;
    let d = length(vec2<f32>(nx, ny)) - 1.0;
    let dist = d * min(params.radius_x, params.radius_y);
    let coverage = 1.0 - smoothstep(-0.5, 0.5, dist);
    let src_a = params.color_a * coverage;
    let src_r = params.color_r * coverage;
    let src_g = params.color_g * coverage;
    let src_b = params.color_b * coverage;
    let ri = u32(clamp(src_r * 255.0 + 0.5, 0.0, 255.0));
    let gi = u32(clamp(src_g * 255.0 + 0.5, 0.0, 255.0));
    let bi = u32(clamp(src_b * 255.0 + 0.5, 0.0, 255.0));
    let ai = u32(clamp(src_a * 255.0 + 0.5, 0.0, 255.0));
    pixels[idx] = ri | (gi << 8u) | (bi << 16u) | (ai << 24u);`,
		},
		{
			name:     "16_full_sdf_with_blend",
			expected: 0xFF0000FF, // should match sdf_circle.wgsl output for center pixel on zero bg
			body: `let cx = f32(8u) + 0.5 - params.center_x;
    let cy = f32(8u) + 0.5 - params.center_y;
    let nx = cx / params.radius_x;
    let ny = cy / params.radius_y;
    let d = length(vec2<f32>(nx, ny)) - 1.0;
    let dist = d * min(params.radius_x, params.radius_y);
    let coverage = 1.0 - smoothstep(-0.5, 0.5, dist);
    let src_a = params.color_a * coverage;
    let src_r = params.color_r * coverage;
    let src_g = params.color_g * coverage;
    let src_b = params.color_b * coverage;
    // Read existing pixel (0xAAAAAAAA pattern)
    let existing = pixels[idx];
    let dst_r = f32(existing & 0xFFu) / 255.0;
    let dst_g = f32((existing >> 8u) & 0xFFu) / 255.0;
    let dst_b = f32((existing >> 16u) & 0xFFu) / 255.0;
    let dst_a = f32((existing >> 24u) & 0xFFu) / 255.0;
    let inv_src_a = 1.0 - src_a;
    let out_r = src_r + dst_r * inv_src_a;
    let out_g = src_g + dst_g * inv_src_a;
    let out_b = src_b + dst_b * inv_src_a;
    let out_a = src_a + dst_a * inv_src_a;
    let ri = u32(clamp(out_r * 255.0 + 0.5, 0.0, 255.0));
    let gi = u32(clamp(out_g * 255.0 + 0.5, 0.0, 255.0));
    let bi = u32(clamp(out_b * 255.0 + 0.5, 0.0, 255.0));
    let ai = u32(clamp(out_a * 255.0 + 0.5, 0.0, 255.0));
    pixels[idx] = ri | (gi << 8u) | (bi << 16u) | (ai << 24u);`,
		},
	}

	t.Logf("=== PROGRESSIVE OPS TEST (params: color_r=1.0, color_g=0.0, color_b=0.0, color_a=1.0, w=%d, h=%d) ===", w, h)

	firstFailure := ""
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build full shader source
			wgsl := shaderPreamble + `
@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= params.target_width || y >= params.target_height {
        return;
    }
    let idx = y * params.target_width + x;
    ` + tt.body + `
}
`
			result := env.runShader(t, wgsl, params, w, h)
			actual := result[0]

			status := "PASS"
			if actual != tt.expected {
				status = "FAIL"
				if firstFailure == "" {
					firstFailure = tt.name
				}
			}

			t.Logf("[%s] %s: expected=0x%08X actual=0x%08X", status, tt.name, tt.expected, actual)

			// Print a few more pixels to catch patterns
			if len(result) >= 4 {
				t.Logf("  pixels[0..3]: 0x%08X 0x%08X 0x%08X 0x%08X", result[0], result[1], result[2], result[3])
			}

			// Count how many match expected
			matchCount := 0
			zeroCount := 0
			aaCount := 0
			for _, v := range result {
				switch v {
				case tt.expected:
					matchCount++
				case 0:
					zeroCount++
				case 0xAAAAAAAA:
					aaCount++
				}
			}
			t.Logf("  total=%d match=%d zero=%d unchanged(0xAA)=%d", len(result), matchCount, zeroCount, aaCount)

			if actual != tt.expected {
				t.Errorf("MISMATCH: expected 0x%08X, got 0x%08X", tt.expected, actual)
			}
		})
	}

	if firstFailure != "" {
		t.Logf("\n=== FIRST FAILURE: %s ===", firstFailure)
		t.Log("The bug is in or before this operation in naga's SPIR-V codegen.")
	} else {
		t.Log("\n=== ALL PASSED === All individual operations work. Bug may be in control flow or SDF-specific code paths.")
	}
}

// TestSDFFullShaderStandalone runs the EXACT sdf_circle.wgsl shader through
// a standalone pipeline (not through SDFAccelerator) to determine if the bug
// is in the shader itself or in SDFAccelerator's pipeline management.
func TestSDFFullShaderStandalone(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	env := newTestGPUEnv(t)
	defer env.destroy()

	// Use same params as TestSDFPipelineZeroInput
	const w, h = 200, 200
	params := SDFCircleParams{
		CenterX: 100, CenterY: 100, RadiusX: 60, RadiusY: 60,
		ColorR: 1.0, ColorG: 0.0, ColorB: 0.0, ColorA: 1.0,
		TargetWidth: w, TargetHeight: h,
	}

	// Use the EXACT embedded sdf_circle.wgsl shader
	result := env.runShader(t, sdfCircleShaderSource, params, w, h)

	// Check center pixel (100,100) — should be inside circle
	centerIdx := 100*w + 100
	centerVal := result[centerIdx]
	t.Logf("Center pixel (100,100): 0x%08X", centerVal)

	// Check edge pixel (159,100) — just inside circle edge
	edgeIdx := 100*w + 159
	edgeVal := result[edgeIdx]
	t.Logf("Edge pixel (159,100): 0x%08X", edgeVal)

	// Check outside pixel (170,100)
	outIdx := 100*w + 170
	outVal := result[outIdx]
	t.Logf("Outside pixel (170,100): 0x%08X (expect 0xAAAAAAAA)", outVal)

	// Count non-0xAA pixels
	nonPattern := 0
	for _, v := range result {
		if v != 0xAAAAAAAA {
			nonPattern++
		}
	}
	t.Logf("Non-0xAA pixels: %d / %d", nonPattern, len(result))

	switch centerVal {
	case 0x00000000:
		t.Error("FAIL: center pixel is 0 — shader writes zeros even in standalone pipeline")
	case 0xAAAAAAAA:
		t.Error("FAIL: center pixel unchanged (0xAA) — shader didn't execute for center")
	default:
		t.Logf("PASS: center pixel = 0x%08X (expected ~0xFF0000FF for red)", centerVal)
	}
}

// TestSDFInlineVsFunction compares the sdf_circle.wgsl shader (uses fn sdf_ellipse)
// against an identical shader with inlined math (no function call).
// If inline works but function doesn't → naga function call codegen bug.
func TestSDFInlineVsFunction(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	env := newTestGPUEnv(t)
	defer env.destroy()

	const w, h = 200, 200
	params := SDFCircleParams{
		CenterX: 100, CenterY: 100, RadiusX: 60, RadiusY: 60,
		ColorR: 1.0, ColorG: 0.0, ColorB: 0.0, ColorA: 1.0,
		TargetWidth: w, TargetHeight: h,
	}

	// Version A: WITH function call (exact sdf_circle.wgsl)
	withFn := sdfCircleShaderSource

	// Version B: same logic but WITHOUT function call (inlined sdf_ellipse)
	withoutFn := shaderPreamble + `
@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= params.target_width || y >= params.target_height {
        return;
    }
    let px = f32(x) + 0.5 - params.center_x;
    let py = f32(y) + 0.5 - params.center_y;

    // INLINED sdf_ellipse
    let nx = px / params.radius_x;
    let ny = py / params.radius_y;
    let d = length(vec2<f32>(nx, ny)) - 1.0;
    let dist = d * min(params.radius_x, params.radius_y);

    var coverage: f32;
    if params.is_stroked != 0u {
        let ring_dist = abs(dist) - params.half_stroke_width;
        coverage = 1.0 - smoothstep(-0.5, 0.5, ring_dist);
    } else {
        coverage = 1.0 - smoothstep(-0.5, 0.5, dist);
    }
    if coverage < 1.0 / 255.0 {
        return;
    }
    let src_a = params.color_a * coverage;
    let src_r = params.color_r * coverage;
    let src_g = params.color_g * coverage;
    let src_b = params.color_b * coverage;
    let idx = y * params.target_width + x;
    let existing = pixels[idx];
    let dst_r = f32(existing & 0xFFu) / 255.0;
    let dst_g = f32((existing >> 8u) & 0xFFu) / 255.0;
    let dst_b = f32((existing >> 16u) & 0xFFu) / 255.0;
    let dst_a = f32((existing >> 24u) & 0xFFu) / 255.0;
    let inv_src_a = 1.0 - src_a;
    let out_r = src_r + dst_r * inv_src_a;
    let out_g = src_g + dst_g * inv_src_a;
    let out_b = src_b + dst_b * inv_src_a;
    let out_a = src_a + dst_a * inv_src_a;
    let ri = u32(clamp(out_r * 255.0 + 0.5, 0.0, 255.0));
    let gi = u32(clamp(out_g * 255.0 + 0.5, 0.0, 255.0));
    let bi = u32(clamp(out_b * 255.0 + 0.5, 0.0, 255.0));
    let ai = u32(clamp(out_a * 255.0 + 0.5, 0.0, 255.0));
    pixels[idx] = ri | (gi << 8u) | (bi << 16u) | (ai << 24u);
}
`

	centerIdx := uint32(100*w + 100)

	resultFn := env.runShader(t, withFn, params, w, h)
	centerFn := resultFn[centerIdx]
	t.Logf("WITH function:    center(100,100) = 0x%08X", centerFn)

	resultInline := env.runShader(t, withoutFn, params, w, h)
	centerInline := resultInline[centerIdx]
	t.Logf("WITHOUT function:  center(100,100) = 0x%08X", centerInline)

	switch {
	case centerFn == 0 && centerInline != 0:
		t.Log(">>> BUG CONFIRMED: fn call breaks shader. naga function call codegen is broken!")
	case centerFn == 0 && centerInline == 0:
		t.Log(">>> Both zero — bug is NOT in function call, look for other differences")
	default:
		t.Log(">>> Both work — unexpected")
	}
}

// TestSDFBufferSizeRegression tests the SDF circle shader at different buffer sizes.
// Progressive tests pass at 16x16 (256 pixels) but fail at 200x200 (40000 pixels).
// This test finds the threshold.
func TestSDFBufferSizeRegression(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	env := newTestGPUEnv(t)
	defer env.destroy()

	sizes := []struct{ w, h uint32 }{
		{16, 16},   // 256 pixels - works in progressive test
		{32, 32},   // 1024
		{64, 64},   // 4096
		{100, 100}, // 10000
		{128, 128}, // 16384
		{200, 200}, // 40000 - fails in TestSDFPipelineZeroInput
	}

	shader := shaderPreamble + `
@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= params.target_width || y >= params.target_height {
        return;
    }
    let px = f32(x) + 0.5 - params.center_x;
    let py = f32(y) + 0.5 - params.center_y;
    let nx = px / params.radius_x;
    let ny = py / params.radius_y;
    let d = length(vec2<f32>(nx, ny)) - 1.0;
    let dist = d * min(params.radius_x, params.radius_y);
    var coverage: f32;
    if params.is_stroked != 0u {
        let ring_dist = abs(dist) - params.half_stroke_width;
        coverage = 1.0 - smoothstep(-0.5, 0.5, ring_dist);
    } else {
        coverage = 1.0 - smoothstep(-0.5, 0.5, dist);
    }
    if coverage < 1.0 / 255.0 { return; }
    let src_a = params.color_a * coverage;
    let src_r = params.color_r * coverage;
    let src_g = params.color_g * coverage;
    let src_b = params.color_b * coverage;
    let idx = y * params.target_width + x;
    let existing = pixels[idx];
    let dst_r = f32(existing & 0xFFu) / 255.0;
    let dst_g = f32((existing >> 8u) & 0xFFu) / 255.0;
    let dst_b = f32((existing >> 16u) & 0xFFu) / 255.0;
    let dst_a = f32((existing >> 24u) & 0xFFu) / 255.0;
    let inv_src_a = 1.0 - src_a;
    let out_r = src_r + dst_r * inv_src_a;
    let out_g = src_g + dst_g * inv_src_a;
    let out_b = src_b + dst_b * inv_src_a;
    let out_a = src_a + dst_a * inv_src_a;
    let ri = u32(clamp(out_r * 255.0 + 0.5, 0.0, 255.0));
    let gi = u32(clamp(out_g * 255.0 + 0.5, 0.0, 255.0));
    let bi = u32(clamp(out_b * 255.0 + 0.5, 0.0, 255.0));
    let ai = u32(clamp(out_a * 255.0 + 0.5, 0.0, 255.0));
    pixels[idx] = ri | (gi << 8u) | (bi << 16u) | (ai << 24u);
}
`
	// Test C: var + if/else but NO early return (all logic nested)
	shaderNoEarlyReturn := shaderPreamble + `
@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x < params.target_width && y < params.target_height {
        let px = f32(x) + 0.5 - params.center_x;
        let py = f32(y) + 0.5 - params.center_y;
        let nx = px / params.radius_x;
        let ny = py / params.radius_y;
        let d = length(vec2<f32>(nx, ny)) - 1.0;
        let dist = d * min(params.radius_x, params.radius_y);
        var coverage: f32;
        if params.is_stroked != 0u {
            coverage = 1.0 - smoothstep(-0.5, 0.5, abs(dist) - params.half_stroke_width);
        } else {
            coverage = 1.0 - smoothstep(-0.5, 0.5, dist);
        }
        if coverage >= 1.0 / 255.0 {
            let src_r = params.color_r * coverage;
            let src_a = params.color_a * coverage;
            let idx = y * params.target_width + x;
            let ri = u32(clamp(src_r * 255.0 + 0.5, 0.0, 255.0));
            let ai = u32(clamp(src_a * 255.0 + 0.5, 0.0, 255.0));
            pixels[idx] = ri | (ai << 24u);
        }
    }
}
`
	// Test D: select() instead of var + if/else
	shaderWithSelect := shaderPreamble + `
@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= params.target_width || y >= params.target_height { return; }
    let px = f32(x) + 0.5 - params.center_x;
    let py = f32(y) + 0.5 - params.center_y;
    let nx = px / params.radius_x;
    let ny = py / params.radius_y;
    let d = length(vec2<f32>(nx, ny)) - 1.0;
    let dist = d * min(params.radius_x, params.radius_y);
    // Use select() instead of var + if/else
    let effective_dist = select(dist, abs(dist) - params.half_stroke_width, params.is_stroked != 0u);
    let coverage = 1.0 - smoothstep(-0.5, 0.5, effective_dist);
    if coverage < 1.0 / 255.0 { return; }
    let src_r = params.color_r * coverage;
    let src_a = params.color_a * coverage;
    let idx = y * params.target_width + x;
    let ri = u32(clamp(src_r * 255.0 + 0.5, 0.0, 255.0));
    let ai = u32(clamp(src_a * 255.0 + 0.5, 0.0, 255.0));
    pixels[idx] = ri | (ai << 24u);
}
`

	// Test A: shader with var + if/else (like sdf_circle.wgsl)
	shaderWithVar := shaderPreamble + `
@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= params.target_width || y >= params.target_height { return; }
    let px = f32(x) + 0.5 - params.center_x;
    let py = f32(y) + 0.5 - params.center_y;
    let nx = px / params.radius_x;
    let ny = py / params.radius_y;
    let d = length(vec2<f32>(nx, ny)) - 1.0;
    let dist = d * min(params.radius_x, params.radius_y);
    // Use var + if/else like sdf_circle.wgsl
    var coverage: f32;
    if params.is_stroked != 0u {
        coverage = 1.0 - smoothstep(-0.5, 0.5, abs(dist) - params.half_stroke_width);
    } else {
        coverage = 1.0 - smoothstep(-0.5, 0.5, dist);
    }
    if coverage < 1.0 / 255.0 { return; }
    let src_r = params.color_r * coverage;
    let src_a = params.color_a * coverage;
    let idx = y * params.target_width + x;
    let ri = u32(clamp(src_r * 255.0 + 0.5, 0.0, 255.0));
    let ai = u32(clamp(src_a * 255.0 + 0.5, 0.0, 255.0));
    pixels[idx] = ri | (ai << 24u);
}
`
	// Test B: same but without var/if — always filled
	shaderNoVar := shaderPreamble + `
@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= params.target_width || y >= params.target_height { return; }
    let px = f32(x) + 0.5 - params.center_x;
    let py = f32(y) + 0.5 - params.center_y;
    let nx = px / params.radius_x;
    let ny = py / params.radius_y;
    let d = length(vec2<f32>(nx, ny)) - 1.0;
    let dist = d * min(params.radius_x, params.radius_y);
    // No var, no if — just filled
    let coverage = 1.0 - smoothstep(-0.5, 0.5, dist);
    if coverage < 1.0 / 255.0 { return; }
    let src_r = params.color_r * coverage;
    let src_a = params.color_a * coverage;
    let idx = y * params.target_width + x;
    let ri = u32(clamp(src_r * 255.0 + 0.5, 0.0, 255.0));
    let ai = u32(clamp(src_a * 255.0 + 0.5, 0.0, 255.0));
    pixels[idx] = ri | (ai << 24u);
}
`
	{
		paramsSm := SDFCircleParams{
			CenterX: 8, CenterY: 8, RadiusX: 4, RadiusY: 4,
			ColorR: 1.0, ColorA: 1.0, TargetWidth: 16, TargetHeight: 16,
		}
		rA := env.runShader(t, shaderWithVar, paramsSm, 16, 16)
		rB := env.runShader(t, shaderNoVar, paramsSm, 16, 16)
		rC := env.runShader(t, shaderNoEarlyReturn, paramsSm, 16, 16)
		rD := env.runShader(t, shaderWithSelect, paramsSm, 16, 16)
		t.Logf("A) var+if/else+return: center(8,8)=0x%08X", rA[8*16+8])
		t.Logf("B) no var (let):       center(8,8)=0x%08X", rB[8*16+8])
		t.Logf("C) var+if/else NO ret: center(8,8)=0x%08X", rC[8*16+8])
		t.Logf("D) select() no var:    center(8,8)=0x%08X", rD[8*16+8])
		if rA[8*16+8] == 0 && rB[8*16+8] != 0 {
			t.Log(">>> var + if/else is broken")
		}
		if rC[8*16+8] != 0 {
			t.Log(">>> C works: early return in first if causes the bug!")
		} else {
			t.Log(">>> C also zero: bug IS in var+if/else itself")
		}
		if rD[8*16+8] != 0 {
			t.Log(">>> D works: select() is a valid workaround")
		}
	}

	for _, sz := range sizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			params := SDFCircleParams{
				CenterX: float32(sz.w) / 2, CenterY: float32(sz.h) / 2,
				RadiusX: float32(sz.w) / 4, RadiusY: float32(sz.h) / 4,
				ColorR: 1.0, ColorA: 1.0,
				TargetWidth: sz.w, TargetHeight: sz.h,
			}
			result := env.runShader(t, shader, params, sz.w, sz.h)
			centerIdx := (sz.h/2)*sz.w + sz.w/2
			centerVal := result[centerIdx]
			nonAA := 0
			zeroCount := 0
			for _, v := range result {
				if v != 0xAAAAAAAA {
					nonAA++
				}
				if v == 0 {
					zeroCount++
				}
			}
			status := "PASS"
			if centerVal == 0 {
				status = "FAIL"
			}
			t.Logf("[%s] %dx%d: center=0x%08X nonAA=%d zeros=%d total=%d",
				status, sz.w, sz.h, centerVal, nonAA, zeroCount, len(result))
		})
	}
}

func TestSDFPipelineDualInstance(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	// Simulate dual Vulkan instances: create TWO accelerators
	dummy := &SDFAccelerator{}
	if err := dummy.Init(); err != nil {
		t.Fatalf("Dummy Init: %v", err)
	}
	defer dummy.Close()
	t.Log("Dummy Vulkan instance created (simulates gogpu window)")

	accel := &SDFAccelerator{}
	if err := accel.Init(); err != nil {
		t.Fatalf("Accel Init: %v", err)
	}
	defer accel.Close()

	w, h := 200, 200
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	target := gg.GPURenderTarget{Width: w, Height: h, Data: img.Pix}
	shape := gg.DetectedShape{Kind: gg.ShapeCircle, CenterX: 100, CenterY: 100, RadiusX: 60, RadiusY: 60}
	paint := &gg.Paint{Brush: gg.SolidBrush{Color: gg.RGBA{R: 1, G: 0, B: 0, A: 1}}}

	if err := accel.FillShape(target, shape, paint); err != nil {
		t.Fatalf("FillShape: %v", err)
	}
	if err := accel.Flush(target); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	nonWhite := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 255 || img.Pix[i+1] != 255 || img.Pix[i+2] != 255 {
			nonWhite++
		}
	}

	t.Logf("Dual instance result: %d non-white pixels", nonWhite)
	if nonWhite == 0 {
		t.Fatal("GPU SDF FAILED with dual Vulkan instances!")
	}
}

// TestSDFBatchMultiShape verifies that batch dispatch renders ALL shapes,
// not just the first one. This catches shader loop bugs in naga's SPIR-V output.
func TestSDFBatchMultiShape(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	w, h := 300, 100
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
		img.Pix[i+1] = 255
		img.Pix[i+2] = 255
		img.Pix[i+3] = 255
	}

	accel := &SDFAccelerator{}
	if err := accel.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer accel.Close()

	target := gg.GPURenderTarget{Width: w, Height: h, Data: img.Pix}

	// 3 well-separated circles: red at (50,50), green at (150,50), blue at (250,50)
	type circleSpec struct {
		cx, cy, r float64
		c         gg.RGBA
	}
	circles := []circleSpec{
		{50, 50, 30, gg.RGBA{R: 1, G: 0, B: 0, A: 1}},
		{150, 50, 30, gg.RGBA{R: 0, G: 1, B: 0, A: 1}},
		{250, 50, 30, gg.RGBA{R: 0, G: 0, B: 1, A: 1}},
	}

	for _, c := range circles {
		shape := gg.DetectedShape{
			Kind: gg.ShapeCircle, CenterX: c.cx, CenterY: c.cy,
			RadiusX: c.r, RadiusY: c.r,
		}
		paint := &gg.Paint{Brush: gg.SolidBrush{Color: c.c}}
		if err := accel.FillShape(target, shape, paint); err != nil {
			t.Fatalf("FillShape: %v", err)
		}
	}

	// Verify all 3 shapes are pending (not flushed individually).
	if n := accel.PendingCount(); n != 3 {
		t.Fatalf("expected 3 pending shapes, got %d", n)
	}

	t.Logf("Dispatching batch with %d shapes", accel.PendingCount())

	if err := accel.Flush(target); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Check each circle region for colored pixels.
	checkRegion := func(_ string, cx, cy, radius int) int {
		count := 0
		for y := cy - radius; y <= cy+radius; y++ {
			for x := cx - radius; x <= cx+radius; x++ {
				if x < 0 || x >= w || y < 0 || y >= h {
					continue
				}
				idx := (y*w + x) * 4
				if img.Pix[idx] != 255 || img.Pix[idx+1] != 255 || img.Pix[idx+2] != 255 {
					count++
				}
			}
		}
		return count
	}

	redCount := checkRegion("Red", 50, 50, 35)
	greenCount := checkRegion("Green", 150, 50, 35)
	blueCount := checkRegion("Blue", 250, 50, 35)

	t.Logf("Red (shape 0): %d pixels, Green (shape 1): %d pixels, Blue (shape 2): %d pixels",
		redCount, greenCount, blueCount)

	// Also print center pixel of each circle for diagnosis.
	for _, c := range []struct {
		name string
		x, y int
	}{{"Red", 50, 50}, {"Green", 150, 50}, {"Blue", 250, 50}} {
		idx := (c.y*w + c.x) * 4
		t.Logf("  %s center (%d,%d): RGBA=[%d,%d,%d,%d]",
			c.name, c.x, c.y, img.Pix[idx], img.Pix[idx+1], img.Pix[idx+2], img.Pix[idx+3])
	}

	if redCount == 0 {
		t.Error("Red circle (shape 0) missing")
	}
	if greenCount == 0 {
		t.Error("Green circle (shape 1) missing — batch loop does not iterate past first shape")
	}
	if blueCount == 0 {
		t.Error("Blue circle (shape 2) missing — batch loop does not iterate past second shape")
	}
}
