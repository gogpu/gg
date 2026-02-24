// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"math"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/gpu/tilecompute"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"

	// Import Vulkan backend so it registers via init().
	_ "github.com/gogpu/wgpu/hal/vulkan"
)

// VelloAccelerator provides GPU-accelerated scene rendering using the Vello-style
// compute pipeline. It implements gg.GPUAccelerator and gg.ComputePipelineAware.
//
// Unlike SDFAccelerator which uses render passes (vertex/fragment shaders),
// VelloAccelerator uses 8 compute shader stages to rasterize entire scenes:
// pathtag_reduce -> pathtag_scan -> draw_reduce -> draw_leaf ->
// path_count -> backdrop -> coarse -> fine
//
// The compute pipeline excels at complex scenes with many overlapping paths,
// deep clip stacks, and high shape counts (>50 shapes per frame).
type VelloAccelerator struct {
	mu sync.Mutex

	instance hal.Instance
	device   hal.Device
	queue    hal.Queue

	dispatcher *VelloComputeDispatcher

	// TODO: scene encoding accumulation
	// pendingScene *tilecompute.SceneEncoding

	gpuReady       bool
	externalDevice bool // true when using shared device (don't destroy on Close)
}

// Interface compliance checks.
var _ gg.GPUAccelerator = (*VelloAccelerator)(nil)
var _ gg.DeviceProviderAware = (*VelloAccelerator)(nil)
var _ gg.ComputePipelineAware = (*VelloAccelerator)(nil)

// Name returns the accelerator identifier.
func (a *VelloAccelerator) Name() string { return "vello-compute" }

// Init registers the accelerator. GPU device initialization is deferred
// until the first use or until SetDeviceProvider is called, to avoid
// creating a standalone Vulkan device that may interfere with an external
// DX12/Metal device provided later. This lazy approach prevents Intel iGPU
// driver issues where destroying a Vulkan device kills a coexisting DX12 device.
func (a *VelloAccelerator) Init() error {
	return nil
}

// Close releases all GPU resources held by the accelerator.
func (a *VelloAccelerator) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.dispatcher != nil {
		a.dispatcher.Close()
		a.dispatcher = nil
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

// SetLogger sets the logger for the GPU accelerator and its internal packages.
// Called by gg.SetLogger to propagate logging configuration.
func (a *VelloAccelerator) SetLogger(l *slog.Logger) {
	setLogger(l)
}

// CanAccelerate reports whether this accelerator supports the given operation.
// The Vello compute pipeline operates at scene level, not per-shape.
func (a *VelloAccelerator) CanAccelerate(op gg.AcceleratedOp) bool {
	return op&gg.AccelScene != 0
}

// CanCompute reports whether the compute pipeline is available and ready.
func (a *VelloAccelerator) CanCompute() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.gpuReady && a.dispatcher != nil && a.dispatcher.initialized
}

// SetDeviceProvider switches the accelerator to use a shared GPU device
// from an external provider (e.g., gogpu). The provider must implement
// HalDevice() any and HalQueue() any returning hal.Device and hal.Queue.
func (a *VelloAccelerator) SetDeviceProvider(provider any) error {
	type halProvider interface {
		HalDevice() any
		HalQueue() any
	}
	hp, ok := provider.(halProvider)
	if !ok {
		return fmt.Errorf("vello-compute: provider does not expose HAL types")
	}
	device, ok := hp.HalDevice().(hal.Device)
	if !ok || device == nil {
		return fmt.Errorf("vello-compute: provider HalDevice is not hal.Device")
	}
	queue, ok := hp.HalQueue().(hal.Queue)
	if !ok || queue == nil {
		return fmt.Errorf("vello-compute: provider HalQueue is not hal.Queue")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Destroy own resources if we created them.
	if a.dispatcher != nil {
		a.dispatcher.Close()
		a.dispatcher = nil
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

	// Create dispatcher with the provided device/queue.
	dispatcher := NewVelloComputeDispatcher(device, queue)
	if err := dispatcher.Init(); err != nil {
		slogger().Warn("vello-compute: pipeline init failed, compute unavailable", "error", err)
		// Still mark gpuReady — device is valid, just compute isn't available.
		a.gpuReady = true
		return nil
	}
	a.dispatcher = dispatcher

	a.gpuReady = true
	slogger().Debug("vello-compute: switched to shared GPU device")
	return nil
}

// FillPath returns ErrFallbackToCPU. The Vello compute pipeline operates at
// scene level; individual path operations are not supported in Phase 1.
// In Phase 2, paths will be accumulated into a scene encoding.
func (a *VelloAccelerator) FillPath(_ gg.GPURenderTarget, _ *gg.Path, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

// StrokePath returns ErrFallbackToCPU. The Vello compute pipeline operates at
// scene level; individual path operations are not supported in Phase 1.
// In Phase 2, paths will be accumulated into a scene encoding.
func (a *VelloAccelerator) StrokePath(_ gg.GPURenderTarget, _ *gg.Path, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

// FillShape returns ErrFallbackToCPU. The Vello compute pipeline operates at
// scene level; individual shape operations are not supported in Phase 1.
// In Phase 2, shapes will be accumulated into a scene encoding.
func (a *VelloAccelerator) FillShape(_ gg.GPURenderTarget, _ gg.DetectedShape, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

// StrokeShape returns ErrFallbackToCPU. The Vello compute pipeline operates at
// scene level; individual shape operations are not supported in Phase 1.
// In Phase 2, shapes will be accumulated into a scene encoding.
func (a *VelloAccelerator) StrokeShape(_ gg.GPURenderTarget, _ gg.DetectedShape, _ *gg.Paint) error {
	return gg.ErrFallbackToCPU
}

// Flush dispatches any pending GPU operations to the target pixel buffer.
// In Phase 1, there is no scene accumulation, so Flush is a no-op.
// In Phase 2, this will pack the accumulated scene encoding and dispatch
// the 8-stage compute pipeline via VelloComputeDispatcher.
func (a *VelloAccelerator) Flush(_ gg.GPURenderTarget) error {
	return nil
}

// RenderSceneCompute renders paths through the GPU compute pipeline.
// This is exported for golden test use -- not part of the public accelerator API.
// It runs the full 8-stage Vello compute pipeline (pathtag_reduce through fine)
// on the given paths and returns the resulting RGBA image.
func (a *VelloAccelerator) RenderSceneCompute(
	width, height int,
	bgColor [4]uint8,
	paths []tilecompute.PathDef,
) (*image.RGBA, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.gpuReady || a.dispatcher == nil {
		return nil, fmt.Errorf("vello-compute: GPU not ready")
	}

	return a.dispatchComputeScene(width, height, bgColor, paths)
}

// dispatchComputeScene runs the 8-stage compute pipeline on the given paths
// and returns the resulting pixel buffer as an RGBA image.
//
// The flow:
//  1. Encode scene via tilecompute.EncodeScene + PackScene.
//  2. Build flat line and path arrays as uint32 slices for GPU upload.
//  3. Compute VelloComputeConfig from scene layout.
//  4. Allocate GPU buffers and upload scene data.
//  5. Upload per-path metadata (pathTotalSegs, pathSegBase, pathStyles).
//  6. Dispatch the 8-stage pipeline.
//  7. Readback: copy output buffer to staging, read pixels.
//  8. Convert packed premultiplied RGBA u32 to image.RGBA.
func (a *VelloAccelerator) dispatchComputeScene(
	width, height int,
	bgColor [4]uint8,
	paths []tilecompute.PathDef,
) (*image.RGBA, error) {
	if len(paths) == 0 {
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		bg := color.RGBA{R: bgColor[0], G: bgColor[1], B: bgColor[2], A: bgColor[3]}
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				img.SetRGBA(x, y, bg)
			}
		}
		return img, nil
	}

	// Step 1: Encode and pack scene.
	enc := tilecompute.EncodeScene(paths)
	scene := tilecompute.PackScene(enc)

	// Step 2: Collect all lines with correct PathIx.
	var allLines []tilecompute.LineSoup
	for pathIx, pd := range paths {
		for _, line := range pd.Lines {
			allLines = append(allLines, tilecompute.LineSoup{
				PathIx: uint32(pathIx),
				P0:     line.P0,
				P1:     line.P1,
			})
		}
	}
	numLines := uint32(len(allLines))

	// Step 3: Pack lines as flat uint32 array (5 words per line).
	linesU32 := make([]uint32, len(allLines)*5)
	for i, line := range allLines {
		off := i * 5
		linesU32[off] = line.PathIx
		linesU32[off+1] = math.Float32bits(line.P0[0])
		linesU32[off+2] = math.Float32bits(line.P0[1])
		linesU32[off+3] = math.Float32bits(line.P1[0])
		linesU32[off+4] = math.Float32bits(line.P1[1])
	}

	// Step 4: Build per-path metadata.
	widthInTiles := uint32((width + tilecompute.TileWidth - 1) / tilecompute.TileWidth)
	heightInTiles := uint32((height + tilecompute.TileHeight - 1) / tilecompute.TileHeight)

	pathsU32, pathStylesU32, _ := buildPathMetadata(
		paths, allLines, width, height, widthInTiles, heightInTiles,
	)

	// Step 5: Compute config.
	config := VelloComputeConfig{
		WidthInTiles:  widthInTiles,
		HeightInTiles: heightInTiles,
		TargetWidth:   uint32(width),
		TargetHeight:  uint32(height),
		NumDrawObj:    scene.Layout.NumDrawObjects,
		NumPaths:      scene.Layout.NumPaths,
		NumClips:      scene.Layout.NumClips,
		PathTagBase:   scene.Layout.PathTagBase,
		PathDataBase:  scene.Layout.PathDataBase,
		DrawTagBase:   scene.Layout.DrawTagBase,
		DrawDataBase:  scene.Layout.DrawDataBase,
		TransformBase: scene.Layout.TransformBase,
		StyleBase:     scene.Layout.StyleBase,
		NumLines:      numLines,
	}

	// Step 6: Allocate GPU buffers.
	bufs, err := a.dispatcher.AllocateBuffers(config, scene.Data, linesU32, pathsU32, numLines)
	if err != nil {
		return nil, fmt.Errorf("vello-compute: allocate buffers: %w", err)
	}
	defer a.dispatcher.DestroyBuffers(bufs)

	// Step 7: Upload per-path auxiliary data.
	numPaths := int(scene.Layout.NumPaths)
	a.uploadPathAuxData(bufs, numPaths, pathStylesU32)

	// Step 8: Dispatch all 8 stages.
	if err := a.dispatcher.Dispatch(bufs, config); err != nil {
		return nil, fmt.Errorf("vello-compute: dispatch: %w", err)
	}

	// Step 9: Readback output pixels.
	outputSize := uint64(width) * uint64(height) * 4
	resultBytes, err := a.readbackBuffer(bufs.Output, outputSize)
	if err != nil {
		return nil, fmt.Errorf("vello-compute: readback: %w", err)
	}

	// Step 10: Convert packed premultiplied RGBA u32 to image.RGBA.
	img := unpackPixels(resultBytes, width, height)

	slogger().Debug("vello-compute: scene rendered",
		"width", width, "height", height,
		"paths", numPaths, "lines", numLines)

	return img, nil
}

// buildPathMetadata computes per-path bounding boxes, tile offsets, and styles.
// Returns (pathsU32, pathStylesU32, tilesOffsetByPath).
func buildPathMetadata(
	paths []tilecompute.PathDef,
	allLines []tilecompute.LineSoup,
	widthPx, heightPx int,
	widthInTiles, heightInTiles uint32,
) (pathsU32 []uint32, pathStylesU32 []uint32, tilesOffsetByPath []uint32) {
	numPaths := len(paths)
	pathsU32 = make([]uint32, numPaths*5)
	pathStylesU32 = make([]uint32, numPaths)
	tilesOffsetByPath = make([]uint32, numPaths)

	// Group lines by PathIx to compute per-path bounding boxes.
	linesByPath := make([][]tilecompute.LineSoup, numPaths)
	for i := range allLines {
		pix := int(allLines[i].PathIx)
		if pix < numPaths {
			linesByPath[pix] = append(linesByPath[pix], allLines[i])
		}
	}

	currentTileOffset := uint32(0)
	for pathIx := 0; pathIx < numPaths; pathIx++ {
		pathLines := linesByPath[pathIx]

		var bboxX0, bboxY0, bboxX1, bboxY1 uint32
		if len(pathLines) > 0 {
			bbox := computePathBBox(pathLines, widthPx, heightPx, widthInTiles, heightInTiles)
			bboxX0 = bbox[0]
			bboxY0 = bbox[1]
			bboxX1 = bbox[2]
			bboxY1 = bbox[3]
		}

		off := pathIx * 5
		pathsU32[off] = bboxX0
		pathsU32[off+1] = bboxY0
		pathsU32[off+2] = bboxX1
		pathsU32[off+3] = bboxY1
		pathsU32[off+4] = currentTileOffset

		tilesOffsetByPath[pathIx] = currentTileOffset
		bboxW := bboxX1 - bboxX0
		bboxH := bboxY1 - bboxY0
		currentTileOffset += bboxW * bboxH

		// Style flags: bit 1 = even-odd.
		var styleFlags uint32
		if paths[pathIx].FillRule == tilecompute.FillRuleEvenOdd {
			styleFlags = 0x02
		}
		pathStylesU32[pathIx] = styleFlags
	}

	return pathsU32, pathStylesU32, tilesOffsetByPath
}

// computePathBBox computes a bounding box in tile coordinates from lines.
// Returns [x0, y0, x1, y1] in tile coordinates, clamped to the canvas.
func computePathBBox(
	lines []tilecompute.LineSoup,
	widthPx, heightPx int,
	widthInTiles, heightInTiles uint32,
) [4]uint32 {
	minX := float32(math.MaxFloat32)
	minY := float32(math.MaxFloat32)
	maxX := -float32(math.MaxFloat32)
	maxY := -float32(math.MaxFloat32)

	for _, line := range lines {
		for _, p := range [][2]float32{line.P0, line.P1} {
			if p[0] < minX {
				minX = p[0]
			}
			if p[0] > maxX {
				maxX = p[0]
			}
			if p[1] < minY {
				minY = p[1]
			}
			if p[1] > maxY {
				maxY = p[1]
			}
		}
	}

	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > float32(widthPx) {
		maxX = float32(widthPx)
	}
	if maxY > float32(heightPx) {
		maxY = float32(heightPx)
	}

	bboxX0 := uint32(math.Floor(float64(minX / float32(tilecompute.TileWidth))))
	bboxY0 := uint32(math.Floor(float64(minY / float32(tilecompute.TileHeight))))
	bboxX1 := uint32(math.Ceil(float64(maxX / float32(tilecompute.TileWidth))))
	bboxY1 := uint32(math.Ceil(float64(maxY / float32(tilecompute.TileHeight))))

	if bboxX1 > widthInTiles {
		bboxX1 = widthInTiles
	}
	if bboxY1 > heightInTiles {
		bboxY1 = heightInTiles
	}

	return [4]uint32{bboxX0, bboxY0, bboxX1, bboxY1}
}

// uploadPathAuxData uploads per-path auxiliary buffers: pathTotalSegs, pathSegBase,
// and pathStyles. PathTotalSegs and pathSegBase are initialized to zero (filled by
// the path_count stage on GPU). PathStyles contains fill rule flags.
func (a *VelloAccelerator) uploadPathAuxData(
	bufs *VelloComputeBuffers,
	numPaths int,
	pathStylesU32 []uint32,
) {
	// PathTotalSegs and PathSegBase are zero-initialized by the GPU allocator.
	// Write zeros explicitly to ensure correct state.
	zeros := make([]byte, numPaths*4)
	a.queue.WriteBuffer(bufs.PathTotalSegs, 0, zeros)
	a.queue.WriteBuffer(bufs.PathSegBase, 0, zeros)

	// Write path styles.
	stylesBytes := make([]byte, len(pathStylesU32)*4)
	for i, v := range pathStylesU32 {
		binary.LittleEndian.PutUint32(stylesBytes[i*4:(i+1)*4], v)
	}
	a.queue.WriteBuffer(bufs.PathStyles, 0, stylesBytes)
}

// readbackBuffer copies a GPU output buffer to a staging buffer and reads the
// result back to CPU memory. The output buffer has CopySrc usage but not MapRead;
// a temporary staging buffer with MapRead|CopyDst is created for the transfer.
func (a *VelloAccelerator) readbackBuffer(outputBuffer hal.Buffer, size uint64) ([]byte, error) {
	// Create staging buffer for readback.
	stagingBuffer, err := a.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "vello_staging_readback",
		Size:  size,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("create staging buffer: %w", err)
	}
	defer a.device.DestroyBuffer(stagingBuffer)

	// Record copy command.
	encoder, err := a.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "vello_readback",
	})
	if err != nil {
		return nil, fmt.Errorf("create readback encoder: %w", err)
	}

	if err := encoder.BeginEncoding("vello_readback"); err != nil {
		return nil, fmt.Errorf("begin readback encoding: %w", err)
	}

	encoder.CopyBufferToBuffer(outputBuffer, stagingBuffer, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 0, Size: size},
	})

	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return nil, fmt.Errorf("end readback encoding: %w", err)
	}
	defer a.device.FreeCommandBuffer(cmdBuf)

	// Submit and wait.
	fence, err := a.device.CreateFence()
	if err != nil {
		return nil, fmt.Errorf("create readback fence: %w", err)
	}
	defer a.device.DestroyFence(fence)

	if err := a.queue.Submit([]hal.CommandBuffer{cmdBuf}, fence, 1); err != nil {
		return nil, fmt.Errorf("submit readback: %w", err)
	}

	ok, err := a.device.Wait(fence, 1, velloFenceTimeout)
	if err != nil {
		return nil, fmt.Errorf("wait for readback: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("readback timeout after %v", velloFenceTimeout)
	}

	// Read data.
	resultBytes := make([]byte, size)
	if err := a.queue.ReadBuffer(stagingBuffer, 0, resultBytes); err != nil {
		return nil, fmt.Errorf("read staging buffer: %w", err)
	}

	return resultBytes, nil
}

// unpackPixels converts packed premultiplied RGBA u32 pixels (GPU output)
// to an image.RGBA in straight alpha format. The GPU fine shader stores
// pixels as R | (G << 8) | (B << 16) | (A << 24) in premultiplied alpha.
func unpackPixels(data []byte, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	le := binary.LittleEndian

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixIdx := y*width + x
			byteOff := pixIdx * 4
			if byteOff+4 > len(data) {
				break
			}
			packed := le.Uint32(data[byteOff : byteOff+4])

			// Unpack premultiplied RGBA.
			pmR := float32(packed&0xFF) / 255.0
			pmG := float32((packed>>8)&0xFF) / 255.0
			pmB := float32((packed>>16)&0xFF) / 255.0
			pmA := float32((packed>>24)&0xFF) / 255.0

			// Convert premultiplied to straight alpha.
			var r, g, b uint8
			a := uint8(pmA*255.0 + 0.5)
			if pmA > 0 {
				r = uint8(clampF(pmR/pmA, 0, 1)*255.0 + 0.5)
				g = uint8(clampF(pmG/pmA, 0, 1)*255.0 + 0.5)
				b = uint8(clampF(pmB/pmA, 0, 1)*255.0 + 0.5)
			}

			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}

	return img
}

// clampF clamps a float32 value to the range [lo, hi].
func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// InitStandalone initializes a standalone Vulkan device for compute-only use.
// This is the public entry point for examples and tools that use the compute
// pipeline without an external device provider (gogpu).
func (a *VelloAccelerator) InitStandalone() error {
	return a.initGPU()
}

// initGPU creates a standalone Vulkan device for compute-only use.
// This is the fallback path when no external device is provided via
// SetDeviceProvider (e.g., when gg is used without gogpu).
func (a *VelloAccelerator) initGPU() error {
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

	// Create dispatcher with the standalone device/queue.
	dispatcher := NewVelloComputeDispatcher(a.device, a.queue)
	if err := dispatcher.Init(); err != nil {
		slogger().Warn("vello-compute: pipeline init failed, compute unavailable", "error", err)
		a.gpuReady = true
		return nil
	}
	a.dispatcher = dispatcher

	a.gpuReady = true
	slogger().Info("vello-compute: GPU initialized (standalone)", "adapter", selected.Info.Name)
	return nil
}
