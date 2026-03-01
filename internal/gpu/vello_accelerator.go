// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

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
	"github.com/gogpu/gg/internal/stroke"
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

	// Scene accumulation for Tier 5 compute pipeline.
	// Paths are accumulated via FillPath/StrokePath/FillShape and dispatched on Flush.
	pendingPaths  []tilecompute.PathDef
	pendingTarget *gg.GPURenderTarget // target for the pending scene (nil if empty)

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

	a.pendingPaths = nil
	a.pendingTarget = nil

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
// The Vello compute pipeline supports fill, stroke, and full scene rendering.
func (a *VelloAccelerator) CanAccelerate(op gg.AcceleratedOp) bool {
	return op&(gg.AccelScene|gg.AccelFill|gg.AccelStroke|gg.AccelCircleSDF|gg.AccelRRectSDF) != 0
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

// FillPath converts a path to a PathDef and accumulates it for the next Flush.
// The actual GPU dispatch happens when Flush is called. Returns ErrFallbackToCPU
// if the GPU is not ready or the path is empty.
func (a *VelloAccelerator) FillPath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	if path == nil || len(path.Elements()) == 0 {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.gpuReady {
		return gg.ErrFallbackToCPU
	}

	// If target changed, flush previous scene first.
	if a.pendingTarget != nil && !velloSameTarget(a.pendingTarget, &target) {
		if err := a.flushLocked(*a.pendingTarget); err != nil {
			return err
		}
	}

	pathDef := convertPathToPathDef(path, paint)
	if len(pathDef.Lines) == 0 {
		return nil
	}

	a.pendingPaths = append(a.pendingPaths, pathDef)
	targetCopy := target
	a.pendingTarget = &targetCopy
	return nil
}

// StrokePath expands the stroke to a filled outline and accumulates the result
// for the next Flush. Dashed strokes fall back to CPU rendering.
func (a *VelloAccelerator) StrokePath(target gg.GPURenderTarget, path *gg.Path, paint *gg.Paint) error {
	if path == nil || len(path.Elements()) == 0 {
		return nil
	}
	if paint != nil && paint.IsDashed() {
		return gg.ErrFallbackToCPU
	}

	// Convert gg path elements to stroke package elements.
	strokeElems := convertPathToStrokeElements(path.Elements())

	// Expand stroke to filled outline.
	style := stroke.Stroke{
		Width:      paint.EffectiveLineWidth(),
		Cap:        stroke.LineCap(paint.EffectiveLineCap()),
		Join:       stroke.LineJoin(paint.EffectiveLineJoin()),
		MiterLimit: paint.EffectiveMiterLimit(),
	}
	expander := stroke.NewStrokeExpander(style)
	expanded := expander.Expand(strokeElems)
	if len(expanded) == 0 {
		return nil
	}

	// Build a gg.Path from the expanded outline.
	fillPath := gg.NewPath()
	for _, e := range expanded {
		switch el := e.(type) {
		case stroke.MoveTo:
			fillPath.MoveTo(el.Point.X, el.Point.Y)
		case stroke.LineTo:
			fillPath.LineTo(el.Point.X, el.Point.Y)
		case stroke.QuadTo:
			fillPath.QuadraticTo(el.Control.X, el.Control.Y, el.Point.X, el.Point.Y)
		case stroke.CubicTo:
			fillPath.CubicTo(el.Control1.X, el.Control1.Y, el.Control2.X, el.Control2.Y, el.Point.X, el.Point.Y)
		case stroke.Close:
			fillPath.Close()
		}
	}

	// Route through FillPath (accumulates into pending scene).
	return a.FillPath(target, fillPath, paint)
}

// FillShape converts a detected shape to a path and accumulates it for the next Flush.
// Returns ErrFallbackToCPU for unknown shapes or if the GPU is not ready.
func (a *VelloAccelerator) FillShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	if shape.Kind == gg.ShapeUnknown {
		return gg.ErrFallbackToCPU
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.gpuReady {
		return gg.ErrFallbackToCPU
	}

	// If target changed, flush previous scene first.
	if a.pendingTarget != nil && !velloSameTarget(a.pendingTarget, &target) {
		if err := a.flushLocked(*a.pendingTarget); err != nil {
			return err
		}
	}

	pathDef := convertShapeToPathDef(shape, paint)
	if len(pathDef.Lines) == 0 {
		return gg.ErrFallbackToCPU
	}

	a.pendingPaths = append(a.pendingPaths, pathDef)
	targetCopy := target
	a.pendingTarget = &targetCopy
	return nil
}

// StrokeShape converts a detected shape to a path, expands the stroke, and
// accumulates the result for the next Flush. Returns ErrFallbackToCPU for
// unknown shapes or if the GPU is not ready.
func (a *VelloAccelerator) StrokeShape(target gg.GPURenderTarget, shape gg.DetectedShape, paint *gg.Paint) error {
	if shape.Kind == gg.ShapeUnknown {
		return gg.ErrFallbackToCPU
	}

	// Build a gg.Path from the shape, then route through StrokePath.
	path := gg.NewPath()
	switch shape.Kind {
	case gg.ShapeCircle:
		path.Circle(shape.CenterX, shape.CenterY, shape.RadiusX)
	case gg.ShapeEllipse:
		path.Ellipse(shape.CenterX, shape.CenterY, shape.RadiusX, shape.RadiusY)
	case gg.ShapeRect:
		x := shape.CenterX - shape.Width/2
		y := shape.CenterY - shape.Height/2
		path.Rectangle(x, y, shape.Width, shape.Height)
	case gg.ShapeRRect:
		x := shape.CenterX - shape.Width/2
		y := shape.CenterY - shape.Height/2
		path.RoundedRectangle(x, y, shape.Width, shape.Height, shape.CornerRadius)
	default:
		return gg.ErrFallbackToCPU
	}

	return a.StrokePath(target, path, paint)
}

// Flush dispatches all accumulated paths through the 9-stage compute pipeline
// and writes the result to the target pixel buffer. Returns nil if there are
// no pending paths. After Flush, the accumulated scene is cleared.
func (a *VelloAccelerator) Flush(target gg.GPURenderTarget) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.flushLocked(target)
}

// PendingCount returns the number of accumulated paths (for testing).
func (a *VelloAccelerator) PendingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pendingPaths)
}

// flushLocked dispatches the accumulated scene. Must be called with a.mu held.
func (a *VelloAccelerator) flushLocked(target gg.GPURenderTarget) error {
	if len(a.pendingPaths) == 0 {
		return nil
	}

	// Take ownership of pending data and reset.
	paths := a.pendingPaths
	a.pendingPaths = nil
	a.pendingTarget = nil

	// Lazy GPU initialization if no external device was provided.
	if a.device == nil {
		if err := a.initGPU(); err != nil {
			slogger().Warn("vello-compute: GPU init failed, falling back to CPU", "err", err)
			return gg.ErrFallbackToCPU
		}
	}

	if a.dispatcher == nil || !a.dispatcher.initialized {
		return gg.ErrFallbackToCPU
	}

	// Dispatch the compute scene with a transparent background so that the
	// result composites over the existing target content.
	bgColor := [4]uint8{0, 0, 0, 0}
	img, err := a.dispatchComputeScene(target.Width, target.Height, bgColor, paths)
	if err != nil {
		slogger().Warn("vello-compute: dispatch failed", "err", err)
		return gg.ErrFallbackToCPU
	}

	// Composite the GPU result over the existing target pixels.
	// The GPU produces premultiplied RGBA; the target is also premultiplied RGBA.
	// We use source-over compositing so previously rendered CPU content is preserved.
	compositeOver(target, img)

	return nil
}

// compositeOver composites src (premultiplied RGBA image) over the target pixel buffer
// using Porter-Duff source-over: dst' = src + dst * (1 - src.A).
// Both target.Data and src are premultiplied RGBA, 4 bytes per pixel.
func compositeOver(target gg.GPURenderTarget, src *image.RGBA) {
	w := target.Width
	h := target.Height
	if w > src.Bounds().Dx() {
		w = src.Bounds().Dx()
	}
	if h > src.Bounds().Dy() {
		h = src.Bounds().Dy()
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			srcOff := src.PixOffset(x, y)
			sR := src.Pix[srcOff]
			sG := src.Pix[srcOff+1]
			sB := src.Pix[srcOff+2]
			sA := src.Pix[srcOff+3]

			// Skip fully transparent source pixels.
			if sA == 0 {
				continue
			}

			dstOff := y*target.Stride + x*4
			if dstOff+3 >= len(target.Data) {
				continue
			}

			// Fully opaque source pixel: overwrite.
			if sA == 255 {
				target.Data[dstOff] = sR
				target.Data[dstOff+1] = sG
				target.Data[dstOff+2] = sB
				target.Data[dstOff+3] = sA
				continue
			}

			// Source-over: dst' = src + dst * (1 - src.A)
			// All values are premultiplied.
			invA := 255 - uint16(sA)
			dR := target.Data[dstOff]
			dG := target.Data[dstOff+1]
			dB := target.Data[dstOff+2]
			dA := target.Data[dstOff+3]

			target.Data[dstOff] = sR + uint8((uint16(dR)*invA+127)/255)
			target.Data[dstOff+1] = sG + uint8((uint16(dG)*invA+127)/255)
			target.Data[dstOff+2] = sB + uint8((uint16(dB)*invA+127)/255)
			target.Data[dstOff+3] = sA + uint8((uint16(dA)*invA+127)/255)
		}
	}
}

// velloSameTarget checks if two render targets point to the same pixel buffer.
func velloSameTarget(a *gg.GPURenderTarget, b *gg.GPURenderTarget) bool {
	return a.Width == b.Width && a.Height == b.Height &&
		len(a.Data) == len(b.Data) && len(a.Data) > 0 && &a.Data[0] == &b.Data[0]
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

	pathsU32, pathStylesU32, totalPathTiles := buildPathMetadata(
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
		BgColor:       uint32(bgColor[0]) | uint32(bgColor[1])<<8 | uint32(bgColor[2])<<16 | uint32(bgColor[3])<<24,
	}

	// Step 6: Allocate GPU buffers.
	bufs, err := a.dispatcher.AllocateBuffers(config, scene.Data, linesU32, pathsU32, numLines, totalPathTiles)
	if err != nil {
		return nil, fmt.Errorf("vello-compute: allocate buffers: %w", err)
	}
	defer a.dispatcher.DestroyBuffers(bufs)

	// Step 7: Upload scene data, line segments, and path metadata to GPU.
	if err := a.queue.WriteBuffer(bufs.Scene, 0, uint32SliceToBytes(scene.Data)); err != nil {
		return nil, fmt.Errorf("vello-compute: write scene buffer: %w", err)
	}
	if err := a.queue.WriteBuffer(bufs.Lines, 0, uint32SliceToBytes(linesU32)); err != nil {
		return nil, fmt.Errorf("vello-compute: write lines buffer: %w", err)
	}
	if err := a.queue.WriteBuffer(bufs.Paths, 0, uint32SliceToBytes(pathsU32)); err != nil {
		return nil, fmt.Errorf("vello-compute: write paths buffer: %w", err)
	}

	// Step 8: Upload per-path auxiliary data.
	numPaths := int(scene.Layout.NumPaths)
	if err := a.uploadPathAuxData(bufs, numPaths, pathStylesU32); err != nil {
		return nil, fmt.Errorf("vello-compute: upload path aux data: %w", err)
	}

	// Step 9: Dispatch all 8 stages.
	if err := a.dispatcher.Dispatch(bufs, config); err != nil {
		return nil, fmt.Errorf("vello-compute: dispatch: %w", err)
	}

	// Step 9b: Diagnostic readback — verify intermediate buffers have data.
	a.logPipelineDiagnostics(bufs, config, totalPathTiles)

	// Step 10: Readback output pixels.
	outputSize := uint64(width) * uint64(height) * 4
	resultBytes, err := a.readbackBuffer(bufs.Output, outputSize)
	if err != nil {
		return nil, fmt.Errorf("vello-compute: readback: %w", err)
	}

	// Step 11: Convert packed premultiplied RGBA u32 to image.RGBA.
	img := unpackPixels(resultBytes, width, height)

	slogger().Debug("vello-compute: scene rendered",
		"width", width, "height", height,
		"paths", numPaths, "lines", numLines)

	return img, nil
}

// buildPathMetadata computes per-path bounding boxes, tile offsets, and styles.
// Returns (pathsU32, pathStylesU32, totalPathTiles).
// totalPathTiles is the sum of per-path tile counts (bboxW*bboxH), which is
// the required size of the flat Tiles buffer. This is NOT the same as
// widthInTiles*heightInTiles (the global tile grid).
func buildPathMetadata(
	paths []tilecompute.PathDef,
	allLines []tilecompute.LineSoup,
	widthPx, heightPx int,
	widthInTiles, heightInTiles uint32,
) (pathsU32 []uint32, pathStylesU32 []uint32, totalPathTiles uint32) {
	numPaths := len(paths)
	pathsU32 = make([]uint32, numPaths*5)
	pathStylesU32 = make([]uint32, numPaths)

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

	totalPathTiles = currentTileOffset
	return pathsU32, pathStylesU32, totalPathTiles
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

// uploadPathAuxData uploads per-path auxiliary buffers: pathStyles.
// PathStyles contains fill rule flags (bit 1 = even-odd).
func (a *VelloAccelerator) uploadPathAuxData(
	bufs *VelloComputeBuffers,
	numPaths int,
	pathStylesU32 []uint32,
) error {
	// Write path styles.
	stylesBytes := make([]byte, len(pathStylesU32)*4)
	for i, v := range pathStylesU32 {
		binary.LittleEndian.PutUint32(stylesBytes[i*4:(i+1)*4], v)
	}
	return a.queue.WriteBuffer(bufs.PathStyles, 0, stylesBytes)
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

// logPipelineDiagnostics reads back key intermediate buffers and logs their state.
// This detects silent pipeline failures (e.g., all-zero output from a broken stage).
func (a *VelloAccelerator) logPipelineDiagnostics(bufs *VelloComputeBuffers, config VelloComputeConfig, totalPathTiles uint32) {
	le := binary.LittleEndian

	// Dump config for debugging.
	slogger().Debug("vello-diag: Config",
		"width_in_tiles", config.WidthInTiles,
		"height_in_tiles", config.HeightInTiles,
		"target", fmt.Sprintf("%dx%d", config.TargetWidth, config.TargetHeight),
		"n_drawobj", config.NumDrawObj,
		"n_path", config.NumPaths,
		"n_lines", config.NumLines,
		"pathtag_base", config.PathTagBase,
		"pathdata_base", config.PathDataBase,
		"drawtag_base", config.DrawTagBase,
		"drawdata_base", config.DrawDataBase,
		"transform_base", config.TransformBase,
		"style_base", config.StyleBase)

	// Check Lines: verify data was uploaded.
	linesSize := uint64(config.NumLines) * 5 * 4 // 5 u32 per LineSoup
	if linesSize > 0 {
		linesBytes, lErr := a.readbackBuffer(bufs.Lines, linesSize)
		if lErr != nil {
			slogger().Warn("vello-diag: cannot read Lines", "error", lErr)
		} else {
			nonZero := 0
			for i := 0; i < len(linesBytes); i += 4 {
				if le.Uint32(linesBytes[i:i+4]) != 0 {
					nonZero++
				}
			}
			slogger().Debug("vello-diag: Lines buffer",
				"n_lines", config.NumLines,
				"total_words", len(linesBytes)/4,
				"non_zero_words", nonZero)
			if nonZero == 0 {
				slogger().Warn("vello-diag: Lines buffer is ALL ZEROS — data not uploaded?")
			}
			// Dump first line for inspection.
			if config.NumLines > 0 && len(linesBytes) >= 20 {
				pathIx := le.Uint32(linesBytes[0:4])
				p0x := math.Float32frombits(le.Uint32(linesBytes[4:8]))
				p0y := math.Float32frombits(le.Uint32(linesBytes[8:12]))
				p1x := math.Float32frombits(le.Uint32(linesBytes[12:16]))
				p1y := math.Float32frombits(le.Uint32(linesBytes[16:20]))
				slogger().Debug("vello-diag: Lines[0]",
					"path_ix", pathIx,
					"p0", fmt.Sprintf("(%.1f, %.1f)", p0x, p0y),
					"p1", fmt.Sprintf("(%.1f, %.1f)", p1x, p1y))
			}
		}
	}

	// Check Paths: verify bboxes are valid (ALL paths, not just first 3).
	pathsSize := uint64(config.NumPaths) * 5 * 4 // 5 u32 per Path
	if pathsSize > 0 {
		pathsBytes, pErr := a.readbackBuffer(bufs.Paths, pathsSize)
		if pErr != nil {
			slogger().Warn("vello-diag: cannot read Paths", "error", pErr)
		} else {
			for i := uint32(0); i < config.NumPaths; i++ {
				off := i * 20
				x0 := le.Uint32(pathsBytes[off : off+4])
				y0 := le.Uint32(pathsBytes[off+4 : off+8])
				x1 := le.Uint32(pathsBytes[off+8 : off+12])
				y1 := le.Uint32(pathsBytes[off+12 : off+16])
				tilesOff := le.Uint32(pathsBytes[off+16 : off+20])
				slogger().Debug("vello-diag: Paths",
					"path", i,
					"bbox", fmt.Sprintf("[%d,%d,%d,%d]", x0, y0, x1, y1),
					"tiles_offset", tilesOff)
			}
		}
	}

	// Check DrawMonoids: verify draw_ix → path_ix mapping.
	dmSize := uint64(config.NumDrawObj) * 4 * 4 // 4 u32 per DrawMonoid
	dmBytes, dmErr := a.readbackBuffer(bufs.DrawMonoids, dmSize)
	if dmErr != nil {
		slogger().Warn("vello-diag: cannot read DrawMonoids", "error", dmErr)
	} else {
		for i := uint32(0); i < config.NumDrawObj; i++ {
			off := i * 16
			pathIx := le.Uint32(dmBytes[off : off+4])
			clipIx := le.Uint32(dmBytes[off+4 : off+8])
			sceneOff := le.Uint32(dmBytes[off+8 : off+12])
			infoOff := le.Uint32(dmBytes[off+12 : off+16])
			slogger().Debug("vello-diag: DrawMonoid",
				"draw", i,
				"path_ix", pathIx,
				"clip_ix", clipIx,
				"scene_offset", sceneOff,
				"info_offset", infoOff)
		}
	}

	// Check Scene draw tags: verify all draws are DRAWTAG_COLOR (0x44).
	sceneTagsOff := uint64(config.DrawTagBase) * 4
	sceneTagsSize := uint64(config.NumDrawObj) * 4
	sceneBytes, scErr := a.readbackBuffer(bufs.Scene, sceneTagsOff+sceneTagsSize)
	if scErr != nil {
		slogger().Warn("vello-diag: cannot read Scene tags", "error", scErr)
	} else {
		for i := uint32(0); i < config.NumDrawObj; i++ {
			off := sceneTagsOff + uint64(i)*4
			tag := le.Uint32(sceneBytes[off : off+4])
			slogger().Debug("vello-diag: DrawTag",
				"draw", i,
				"tag", fmt.Sprintf("0x%02X", tag),
				"is_color", tag == 0x44)
		}
	}

	// Check BumpAlloc: seg_counts should be > 0 if path_count produced segments.
	// segments should be > 0 if coarse allocated segment slots.
	bumpBytes, err := a.readbackBuffer(bufs.BumpAlloc, 16)
	if err != nil {
		slogger().Warn("vello-diag: cannot read BumpAlloc", "error", err)
	} else {
		segCounts := le.Uint32(bumpBytes[0:4])
		segAlloc := le.Uint32(bumpBytes[4:8])
		dbgActiveThreads := le.Uint32(bumpBytes[8:12])
		dbgTilesVisited := le.Uint32(bumpBytes[12:16])
		slogger().Debug("vello-diag: BumpAlloc",
			"seg_counts", segCounts,
			"segments_allocated", segAlloc,
			"coarse_active_threads", dbgActiveThreads,
			"coarse_tiles_visited", dbgTilesVisited)
		if segCounts == 0 {
			slogger().Warn("vello-diag: path_count produced ZERO segments — tiles will be empty")
		}
		if segAlloc == 0 && segCounts > 0 {
			slogger().Warn("vello-diag: coarse allocated ZERO segments — write_path not working?")
		}
	}

	// Check PTCL: read first word of each tile's PTCL stream.
	// Each tile's stream starts at tile_ix * PTCL_MAX_PER_TILE.
	globalTiles := config.WidthInTiles * config.HeightInTiles
	ptclFullSize := uint64(globalTiles) * velloPTCLMaxPerTile * 4
	ptclBytes, ptclErr := a.readbackBuffer(bufs.PTCL, ptclFullSize)
	if ptclErr != nil {
		slogger().Warn("vello-diag: cannot read PTCL", "error", ptclErr)
	} else {
		nonZeroCmds := 0
		fillCmds := 0
		solidCmds := 0
		colorCmds := 0
		for t := uint32(0); t < globalTiles; t++ {
			byteBase := uint64(t) * velloPTCLMaxPerTile * 4
			if byteBase+4 > uint64(len(ptclBytes)) {
				break
			}
			cmd := le.Uint32(ptclBytes[byteBase : byteBase+4])
			if cmd != 0 {
				nonZeroCmds++
			}
			if cmd == 1 {
				fillCmds++
			}
			if cmd == 3 {
				solidCmds++
			}
			if cmd == 5 {
				colorCmds++
			}
		}
		slogger().Debug("vello-diag: PTCL",
			"tiles_with_cmds", nonZeroCmds,
			"fill_cmds", fillCmds,
			"solid_cmds", solidCmds,
			"color_cmds", colorCmds)
	}

	// Check Segments: sample first few for non-zero data.
	segSampleSize := uint64(200) * 5 * 4 // first 200 segments * 5 f32
	segBytes, segErr := a.readbackBuffer(bufs.Segments, segSampleSize)
	if segErr != nil {
		slogger().Warn("vello-diag: cannot read Segments", "error", segErr)
	} else {
		nonZeroSegs := 0
		maxSegs := len(segBytes) / 20
		for i := 0; i < maxSegs; i++ {
			off := i * 20
			p0x := le.Uint32(segBytes[off : off+4])
			p0y := le.Uint32(segBytes[off+4 : off+8])
			if p0x != 0 || p0y != 0 {
				nonZeroSegs++
			}
		}
		slogger().Debug("vello-diag: Segments",
			"sampled", maxSegs,
			"non_zero", nonZeroSegs)
	}

	// Dump first PTCL tile with CMD_FILL for detailed inspection.
	if ptclBytes != nil {
		for t := uint32(0); t < globalTiles; t++ {
			byteBase := uint64(t) * velloPTCLMaxPerTile * 4
			if byteBase+24 > uint64(len(ptclBytes)) {
				break
			}
			cmd0 := le.Uint32(ptclBytes[byteBase : byteBase+4])
			if cmd0 == 1 || cmd0 == 3 { // CMD_FILL or CMD_SOLID
				cmdName := "SOLID"
				if cmd0 == 1 {
					cmdName = "FILL"
					packed := le.Uint32(ptclBytes[byteBase+4 : byteBase+8])
					segIdx := le.Uint32(ptclBytes[byteBase+8 : byteBase+12])
					backdrop := int32(le.Uint32(ptclBytes[byteBase+12 : byteBase+16]))
					segCount := packed >> 1
					evenOdd := packed & 1
					// Read CMD_COLOR after FILL payload (4 words).
					colorBase := byteBase + 16
					nextCmd := le.Uint32(ptclBytes[colorBase : colorBase+4])
					rgba := le.Uint32(ptclBytes[colorBase+4 : colorBase+8])
					slogger().Debug("vello-diag: PTCL detail",
						"global_tile", t,
						"tile_xy", fmt.Sprintf("(%d,%d)", t%config.WidthInTiles, t/config.WidthInTiles),
						"cmd", cmdName,
						"seg_count", segCount, "even_odd", evenOdd,
						"seg_index", segIdx, "backdrop", backdrop,
						"next_cmd", nextCmd, "rgba", fmt.Sprintf("0x%08X", rgba))
					// Dump ALL segments for this tile.
					if segBytes != nil && segCount > 0 {
						for si := uint32(0); si < segCount; si++ {
							sIdx := segIdx + si
							if sIdx >= 200 {
								break
							}
							segOff := uint64(sIdx) * 20
							if segOff+20 <= uint64(len(segBytes)) {
								sp0x := math.Float32frombits(le.Uint32(segBytes[segOff : segOff+4]))
								sp0y := math.Float32frombits(le.Uint32(segBytes[segOff+4 : segOff+8]))
								sp1x := math.Float32frombits(le.Uint32(segBytes[segOff+8 : segOff+12]))
								sp1y := math.Float32frombits(le.Uint32(segBytes[segOff+12 : segOff+16]))
								yEdge := math.Float32frombits(le.Uint32(segBytes[segOff+16 : segOff+20]))
								slogger().Debug("vello-diag: Segment for tile",
									"tile_xy", fmt.Sprintf("(%d,%d)", t%config.WidthInTiles, t/config.WidthInTiles),
									"seg_ix", sIdx,
									"p0", fmt.Sprintf("(%.3f, %.3f)", sp0x, sp0y),
									"p1", fmt.Sprintf("(%.3f, %.3f)", sp1x, sp1y),
									"y_edge", fmt.Sprintf("%.3f", yEdge))
							}
						}
					}
				} else {
					// CMD_SOLID: read CMD_COLOR after SOLID (1 word).
					colorBase := byteBase + 4
					nextCmd := le.Uint32(ptclBytes[colorBase : colorBase+4])
					rgba := le.Uint32(ptclBytes[colorBase+4 : colorBase+8])
					slogger().Debug("vello-diag: PTCL detail",
						"global_tile", t,
						"tile_xy", fmt.Sprintf("(%d,%d)", t%config.WidthInTiles, t/config.WidthInTiles),
						"cmd", cmdName,
						"next_cmd", nextCmd, "rgba", fmt.Sprintf("0x%08X", rgba))
				}
			}
		}
	}

	// Check Tiles: sample per-path tiles for non-zero backdrop/segment_count.
	// Tiles buffer uses per-path allocation (totalPathTiles), NOT global tile grid.
	tileReadSize := uint64(totalPathTiles) * 8 // 2 u32 per tile
	tilesBytes, err := a.readbackBuffer(bufs.Tiles, tileReadSize)
	if err != nil {
		slogger().Warn("vello-diag: cannot read Tiles", "error", err)
	} else {
		nonZeroBackdrop := 0
		nonZeroSegCount := 0
		totalSegSum := uint32(0)
		for i := uint32(0); i < totalPathTiles; i++ {
			off := i * 8
			backdrop := int32(le.Uint32(tilesBytes[off : off+4]))
			segCount := le.Uint32(tilesBytes[off+4 : off+8])
			if backdrop != 0 {
				nonZeroBackdrop++
			}
			if segCount != 0 {
				nonZeroSegCount++
				totalSegSum += segCount
			}
		}
		slogger().Debug("vello-diag: Tiles",
			"total_path_tiles", totalPathTiles,
			"non_zero_backdrop", nonZeroBackdrop,
			"non_zero_seg_count", nonZeroSegCount,
			"total_seg_sum", totalSegSum)
		if nonZeroBackdrop == 0 && nonZeroSegCount == 0 {
			slogger().Warn("vello-diag: ALL tiles are zero — path_count or backdrop stage failed")
		}

		// Dump ALL tiles (up to 100) for small scenes, or first 10 non-zero for large.
		maxDump := uint32(100)
		if totalPathTiles <= maxDump {
			// Small scene: dump every tile.
			for i := uint32(0); i < totalPathTiles; i++ {
				off := i * 8
				backdrop := int32(le.Uint32(tilesBytes[off : off+4]))
				segCount := le.Uint32(tilesBytes[off+4 : off+8])
				isInverted := int32(segCount) < 0
				slogger().Debug("vello-diag: Tile",
					"ix", i,
					"backdrop", backdrop,
					"seg_or_ix", segCount,
					"inverted", isInverted)
			}
		} else {
			dumped := 0
			for i := uint32(0); i < totalPathTiles && dumped < 10; i++ {
				off := i * 8
				backdrop := int32(le.Uint32(tilesBytes[off : off+4]))
				segCount := le.Uint32(tilesBytes[off+4 : off+8])
				if segCount != 0 || backdrop != 0 {
					isInverted := int32(segCount) < 0
					slogger().Debug("vello-diag: Tile sample",
						"tile_ix", i,
						"backdrop", backdrop,
						"seg_count_or_ix", segCount,
						"is_inverted_idx", isInverted)
					dumped++
				}
			}
		}
	}

	// Check output: count non-zero pixels.
	outputSize := uint64(config.TargetWidth) * uint64(config.TargetHeight) * 4
	outBytes, err := a.readbackBuffer(bufs.Output, outputSize)
	if err != nil {
		slogger().Warn("vello-diag: cannot read Output", "error", err)
	} else {
		nonZeroPixels := 0
		totalPixels := int(config.TargetWidth) * int(config.TargetHeight)
		for i := 0; i < totalPixels; i++ {
			if le.Uint32(outBytes[i*4:i*4+4]) != 0 {
				nonZeroPixels++
			}
		}
		slogger().Debug("vello-diag: Output",
			"total_pixels", totalPixels,
			"non_zero_pixels", nonZeroPixels)
		if nonZeroPixels == 0 {
			slogger().Warn("vello-diag: output is ALL ZEROS — fine stage produced no visible pixels")
		}
	}
}

// uint32SliceToBytes converts a []uint32 to []byte in little-endian format.
func uint32SliceToBytes(data []uint32) []byte {
	buf := make([]byte, len(data)*4)
	for i, v := range data {
		binary.LittleEndian.PutUint32(buf[i*4:(i+1)*4], v)
	}
	return buf
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
