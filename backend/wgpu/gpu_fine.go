//go:build !nogpu

// Package wgpu provides GPU-accelerated rendering using WebGPU.
package wgpu

import (
	_ "embed"
	"fmt"
	"math"
	"sync"

	"github.com/gogpu/gg/scene"
	"github.com/gogpu/naga"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

//go:embed shaders/fine.wgsl
var fineShaderWGSL string

// GPUSegment is the GPU-compatible layout of LineSegment.
// Must match the Segment struct in fine.wgsl.
type GPUSegment struct {
	X0      float32 // Start X coordinate
	Y0      float32 // Start Y coordinate
	X1      float32 // End X coordinate
	Y1      float32 // End Y coordinate
	Winding int32   // Winding direction: +1 or -1
	TileY0  int32   // Starting tile Y (precomputed)
	TileY1  int32   // Ending tile Y (precomputed)
	Padding int32   // Padding for alignment
}

// GPUTileSegmentRef maps a segment to a tile.
// Must match TileSegmentRef in fine.wgsl.
type GPUTileSegmentRef struct {
	TileX       uint32 // Tile X coordinate
	TileY       uint32 // Tile Y coordinate
	SegmentIdx  uint32 // Index into segments array
	WindingFlag uint32 // Whether this contributes winding (0 or 1)
}

// GPUTileInfo contains tile processing information.
// Must match TileInfo in fine.wgsl.
type GPUTileInfo struct {
	TileX    uint32 // Tile X coordinate
	TileY    uint32 // Tile Y coordinate
	StartIdx uint32 // Start index in tile_segments
	Count    uint32 // Number of segments for this tile
	Backdrop int32  // Accumulated winding from left
	Padding1 uint32 // Padding for alignment
	Padding2 uint32 // Padding for alignment
	Padding3 uint32 // Padding for alignment
}

// GPUFineConfig contains GPU fine rasterization configuration.
// Must match Config in fine.wgsl.
type GPUFineConfig struct {
	ViewportWidth  uint32 // Viewport width in pixels
	ViewportHeight uint32 // Viewport height in pixels
	TileColumns    uint32 // Number of tile columns
	TileRows       uint32 // Number of tile rows
	TileCount      uint32 // Number of tiles to process
	FillRule       uint32 // 0 = NonZero, 1 = EvenOdd
	Padding1       uint32 // Padding for alignment
	Padding2       uint32 // Padding for alignment
}

// FillRuleToGPU converts scene.FillStyle to GPU constant.
func FillRuleToGPU(rule scene.FillStyle) uint32 {
	switch rule {
	case scene.FillEvenOdd:
		return 1
	default:
		return 0 // NonZero
	}
}

// GPUFineRasterizer performs fine rasterization on the GPU.
// It creates compute pipelines and manages GPU buffers for coverage calculation.
//
// Note: This is Phase 6.1 implementation. Full GPU buffer binding requires
// HAL API extensions to expose buffer handles. Currently this serves as
// infrastructure and data flow verification.
type GPUFineRasterizer struct {
	mu sync.Mutex

	device hal.Device
	queue  hal.Queue

	// Compute pipelines
	finePipeline      hal.ComputePipeline
	fineSolidPipeline hal.ComputePipeline
	clearPipeline     hal.ComputePipeline

	// Shader module (cached)
	shaderModule hal.ShaderModule

	// Pipeline layout and bind group layouts
	pipelineLayout   hal.PipelineLayout
	inputBindLayout  hal.BindGroupLayout
	outputBindLayout hal.BindGroupLayout

	// Compiled SPIR-V (cached for verification)
	spirvCode []uint32

	// Viewport dimensions
	width  uint16
	height uint16

	// State
	initialized bool
	shaderReady bool
}

// NewGPUFineRasterizer creates a new GPU fine rasterizer.
// Returns an error if GPU compute is not supported.
func NewGPUFineRasterizer(device hal.Device, queue hal.Queue, width, height uint16) (*GPUFineRasterizer, error) {
	if device == nil || queue == nil {
		return nil, fmt.Errorf("gpu_fine: device and queue are required")
	}

	r := &GPUFineRasterizer{
		device: device,
		queue:  queue,
		width:  width,
		height: height,
	}

	if err := r.init(); err != nil {
		r.Destroy()
		return nil, err
	}

	return r, nil
}

// init initializes GPU resources (pipelines, layouts).
func (r *GPUFineRasterizer) init() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Compile WGSL to SPIR-V
	spirvBytes, err := naga.Compile(fineShaderWGSL)
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to compile shader: %w", err)
	}

	// Convert bytes to uint32 slice for SPIR-V
	r.spirvCode = make([]uint32, len(spirvBytes)/4)
	for i := range r.spirvCode {
		r.spirvCode[i] = uint32(spirvBytes[i*4]) |
			uint32(spirvBytes[i*4+1])<<8 |
			uint32(spirvBytes[i*4+2])<<16 |
			uint32(spirvBytes[i*4+3])<<24
	}

	r.shaderReady = true

	// Create shader module
	shaderModule, err := r.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label: "fine_shader",
		Source: hal.ShaderSource{
			SPIRV: r.spirvCode,
		},
	})
	if err != nil {
		// Shader module creation may fail if compute shaders are not supported
		// Log but don't fail - allow CPU fallback
		return fmt.Errorf("gpu_fine: failed to create shader module: %w", err)
	}
	r.shaderModule = shaderModule

	// Create bind group layouts
	if err := r.createBindGroupLayouts(); err != nil {
		return err
	}

	// Create pipeline layout
	if err := r.createPipelineLayout(); err != nil {
		return err
	}

	// Create compute pipelines
	if err := r.createPipelines(); err != nil {
		return err
	}

	r.initialized = true
	return nil
}

// createBindGroupLayouts creates the bind group layouts for the pipeline.
func (r *GPUFineRasterizer) createBindGroupLayouts() error {
	// Input bind group layout (group 0)
	inputLayout, err := r.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "fine_input_layout",
		Entries: []types.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type:           types.BufferBindingTypeUniform,
					MinBindingSize: 32, // sizeof(Config)
				},
			},
			{
				Binding:    1,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type: types.BufferBindingTypeReadOnlyStorage,
				},
			},
			{
				Binding:    2,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type: types.BufferBindingTypeReadOnlyStorage,
				},
			},
			{
				Binding:    3,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type: types.BufferBindingTypeReadOnlyStorage,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create input bind group layout: %w", err)
	}
	r.inputBindLayout = inputLayout

	// Output bind group layout (group 1)
	outputLayout, err := r.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "fine_output_layout",
		Entries: []types.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type: types.BufferBindingTypeStorage,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create output bind group layout: %w", err)
	}
	r.outputBindLayout = outputLayout

	return nil
}

// createPipelineLayout creates the pipeline layout.
func (r *GPUFineRasterizer) createPipelineLayout() error {
	layout, err := r.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "fine_pipeline_layout",
		BindGroupLayouts: []hal.BindGroupLayout{r.inputBindLayout, r.outputBindLayout},
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create pipeline layout: %w", err)
	}
	r.pipelineLayout = layout
	return nil
}

// createPipelines creates the compute pipelines.
func (r *GPUFineRasterizer) createPipelines() error {
	// Main fine rasterization pipeline
	finePipeline, err := r.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "fine_pipeline",
		Layout: r.pipelineLayout,
		Compute: hal.ComputeState{
			Module:     r.shaderModule,
			EntryPoint: "cs_fine",
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create fine pipeline: %w", err)
	}
	r.finePipeline = finePipeline

	// Solid tile pipeline
	solidPipeline, err := r.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "fine_solid_pipeline",
		Layout: r.pipelineLayout,
		Compute: hal.ComputeState{
			Module:     r.shaderModule,
			EntryPoint: "cs_fine_solid",
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create solid pipeline: %w", err)
	}
	r.fineSolidPipeline = solidPipeline

	// Clear coverage pipeline
	clearPipeline, err := r.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "clear_coverage_pipeline",
		Layout: r.pipelineLayout,
		Compute: hal.ComputeState{
			Module:     r.shaderModule,
			EntryPoint: "cs_clear_coverage",
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create clear pipeline: %w", err)
	}
	r.clearPipeline = clearPipeline

	return nil
}

// Rasterize performs fine rasterization on the GPU.
// It takes the coarse rasterizer output and produces coverage values.
//
// Note: Phase 6.1 implementation. Full GPU dispatch requires buffer binding
// which needs HAL API extensions. Currently falls back to CPU-computed coverage.
func (r *GPUFineRasterizer) Rasterize(
	coarse *CoarseRasterizer,
	segments *SegmentList,
	backdrop []int32,
	fillRule scene.FillStyle,
) ([]uint8, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return nil, fmt.Errorf("gpu_fine: rasterizer not initialized")
	}

	if coarse == nil || segments == nil {
		return nil, nil
	}

	entries := coarse.Entries()
	if len(entries) == 0 {
		return nil, nil
	}

	// Build tile info from coarse entries
	tiles, tileSegRefs := r.buildTileData(coarse, segments, backdrop)
	if len(tiles) == 0 {
		return nil, nil
	}

	// Prepare GPU data structures (validates data conversion)
	gpuSegments := r.convertSegments(segments)
	gpuTileRefs := r.convertTileSegmentRefs(tileSegRefs)
	gpuTiles := r.convertTileInfos(tiles)

	// Phase 6.1: GPU infrastructure is ready, but buffer binding needs HAL extension.
	// For now, compute coverage on CPU using the same algorithm as the shader.
	coverage := r.computeCoverageCPU(gpuSegments, gpuTileRefs, gpuTiles, fillRule)

	return coverage, nil
}

// computeCoverageCPU computes coverage using CPU (mirrors GPU shader algorithm).
// This serves as reference implementation and fallback.
func (r *GPUFineRasterizer) computeCoverageCPU(
	segments []GPUSegment,
	tileRefs []GPUTileSegmentRef,
	tiles []GPUTileInfo,
	fillRule scene.FillStyle,
) []uint8 {
	coverageSize := len(tiles) * TileSize * TileSize
	coverage := make([]uint8, coverageSize)

	for tileIdx, tile := range tiles {
		// Process each pixel in the tile
		for py := uint32(0); py < TileSize; py++ {
			for px := uint32(0); px < TileSize; px++ {
				winding := float32(tile.Backdrop)

				// Process all segments for this tile
				for i := tile.StartIdx; i < tile.StartIdx+tile.Count; i++ {
					if int(i) >= len(tileRefs) {
						break
					}
					ref := tileRefs[i]
					if int(ref.SegmentIdx) >= len(segments) {
						continue
					}
					seg := segments[ref.SegmentIdx]

					// Compute segment contribution to this pixel
					area := r.computePixelArea(
						seg,
						float32(tile.TileX*TileSize),
						float32(tile.TileY*TileSize),
						px, py,
					)
					winding += area
				}

				// Convert winding to coverage
				cov := r.windingToCoverage(winding, fillRule)

				// Store coverage
				pixelIdx := tileIdx*TileSize*TileSize + int(py)*TileSize + int(px)
				if pixelIdx < len(coverage) {
					coverage[pixelIdx] = uint8(cov*255 + 0.5)
				}
			}
		}
	}

	return coverage
}

// computePixelArea computes segment's area contribution to a pixel.
func (r *GPUFineRasterizer) computePixelArea(
	seg GPUSegment,
	tileLeftX, tileTopY float32,
	pxX, pxY uint32,
) float32 {
	// Convert to tile-relative coordinates
	p0x := seg.X0 - tileLeftX
	p0y := seg.Y0 - tileTopY
	p1x := seg.X1 - tileLeftX
	p1y := seg.Y1 - tileTopY

	// Skip horizontal segments
	if p0y == p1y {
		return 0
	}

	sign := float32(seg.Winding)

	// Line is monotonic (Y0 <= Y1)
	lineTopY := p0y
	lineTopX := p0x
	lineBottomY := p1y
	// lineBottomX := p1x

	// Calculate slopes
	dy := lineBottomY - lineTopY
	dx := p1x - p0x

	var ySlope float32
	if dx == 0 {
		if lineBottomY > lineTopY {
			ySlope = 1e10
		} else {
			ySlope = -1e10
		}
	} else {
		ySlope = dy / dx
	}
	xSlope := 1.0 / ySlope

	// Pixel row bounds
	pxTopY := float32(pxY)
	pxBottomY := pxTopY + 1.0
	pxLeftX := float32(pxX)
	pxRightX := pxLeftX + 1.0

	// Clamp line Y range to this pixel row
	yMin := maxf32(lineTopY, pxTopY)
	yMax := minf32(lineBottomY, pxBottomY)

	// Check if line crosses this row
	if yMin >= yMax {
		return 0
	}

	// Calculate Y coordinates where line intersects pixel left and right edges
	linePxLeftY := lineTopY + (pxLeftX-lineTopX)*ySlope
	linePxRightY := lineTopY + (pxRightX-lineTopX)*ySlope

	// Clamp to pixel row bounds and line Y bounds
	linePxLeftY = clampf32(linePxLeftY, yMin, yMax)
	linePxRightY = clampf32(linePxRightY, yMin, yMax)

	// Calculate X coordinates at the clamped Y values
	linePxLeftYX := lineTopX + (linePxLeftY-lineTopY)*xSlope
	linePxRightYX := lineTopX + (linePxRightY-lineTopY)*xSlope

	// Height of line segment within this pixel
	pixelH := absf32(linePxRightY - linePxLeftY)

	// Trapezoidal area: area between line and pixel right edge
	area := 0.5 * pixelH * (2.0*pxRightX - linePxRightYX - linePxLeftYX)

	return area * sign
}

// windingToCoverage converts winding number to coverage based on fill rule.
func (r *GPUFineRasterizer) windingToCoverage(winding float32, fillRule scene.FillStyle) float32 {
	var cov float32

	if fillRule == scene.FillNonZero {
		// NonZero: coverage = |winding|, clamped to [0, 1]
		cov = absf32(winding)
		if cov > 1.0 {
			cov = 1.0
		}
	} else {
		// EvenOdd: coverage based on fractional part
		absWinding := absf32(winding)
		im1 := float32(int32(absWinding*0.5 + 0.5))
		cov = absf32(absWinding - 2.0*im1)
		if cov > 1.0 {
			cov = 1.0
		}
	}

	return cov
}

// buildTileData builds tile info and segment references from coarse entries.
func (r *GPUFineRasterizer) buildTileData(
	coarse *CoarseRasterizer,
	_ *SegmentList, // segments not used directly, entries contain indices
	backdrop []int32,
) ([]GPUTileInfo, []GPUTileSegmentRef) {
	entries := coarse.Entries()
	if len(entries) == 0 {
		return nil, nil
	}

	coarse.SortEntries()

	// Group entries by tile
	type tileKey struct {
		x, y uint16
	}
	tileMap := make(map[tileKey][]int) // tile -> entry indices

	for i, e := range entries {
		key := tileKey{e.X, e.Y}
		tileMap[key] = append(tileMap[key], i)
	}

	// Build tile info and segment refs
	tiles := make([]GPUTileInfo, 0, len(tileMap))
	refs := make([]GPUTileSegmentRef, 0, len(entries))

	tileColumns := int(coarse.TileColumns())

	for key, indices := range tileMap {
		//nolint:gosec // len(refs) is bounded by number of coarse entries
		startIdx := uint32(len(refs))

		// Get backdrop for this tile
		var bd int32
		if backdrop != nil {
			idx := int(key.y)*tileColumns + int(key.x)
			if idx >= 0 && idx < len(backdrop) {
				bd = backdrop[idx]
			}
		}

		// Add segment refs for this tile
		for _, entryIdx := range indices {
			e := entries[entryIdx]
			refs = append(refs, GPUTileSegmentRef{
				TileX:       uint32(e.X),
				TileY:       uint32(e.Y),
				SegmentIdx:  e.LineIdx,
				WindingFlag: boolToUint32(e.Winding),
			})
		}

		tiles = append(tiles, GPUTileInfo{
			TileX:    uint32(key.x),
			TileY:    uint32(key.y),
			StartIdx: startIdx,
			//nolint:gosec // len(indices) is bounded by segment count
			Count:    uint32(len(indices)),
			Backdrop: bd,
		})
	}

	return tiles, refs
}

// convertSegments converts CPU segments to GPU format.
func (r *GPUFineRasterizer) convertSegments(segments *SegmentList) []GPUSegment {
	lines := segments.Segments()
	result := make([]GPUSegment, len(lines))

	for i, seg := range lines {
		result[i] = GPUSegment{
			X0:      seg.X0,
			Y0:      seg.Y0,
			X1:      seg.X1,
			Y1:      seg.Y1,
			Winding: int32(seg.Winding),
			TileY0:  seg.TileY0,
			TileY1:  seg.TileY1,
		}
	}

	return result
}

// convertTileSegmentRefs is a no-op since GPUTileSegmentRef is already the right type.
func (r *GPUFineRasterizer) convertTileSegmentRefs(refs []GPUTileSegmentRef) []GPUTileSegmentRef {
	return refs
}

// convertTileInfos is a no-op since GPUTileInfo is already the right type.
func (r *GPUFineRasterizer) convertTileInfos(tiles []GPUTileInfo) []GPUTileInfo {
	return tiles
}

// IsInitialized returns whether the rasterizer is initialized.
func (r *GPUFineRasterizer) IsInitialized() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.initialized
}

// IsShaderReady returns whether the shader compiled successfully.
func (r *GPUFineRasterizer) IsShaderReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shaderReady
}

// SPIRVCode returns the compiled SPIR-V code (for debugging/verification).
func (r *GPUFineRasterizer) SPIRVCode() []uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spirvCode
}

// Destroy releases all GPU resources.
func (r *GPUFineRasterizer) Destroy() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.device == nil {
		return
	}

	// Destroy pipelines
	if r.finePipeline != nil {
		r.device.DestroyComputePipeline(r.finePipeline)
		r.finePipeline = nil
	}
	if r.fineSolidPipeline != nil {
		r.device.DestroyComputePipeline(r.fineSolidPipeline)
		r.fineSolidPipeline = nil
	}
	if r.clearPipeline != nil {
		r.device.DestroyComputePipeline(r.clearPipeline)
		r.clearPipeline = nil
	}

	// Destroy pipeline layout
	if r.pipelineLayout != nil {
		r.device.DestroyPipelineLayout(r.pipelineLayout)
		r.pipelineLayout = nil
	}

	// Destroy bind group layouts
	if r.inputBindLayout != nil {
		r.device.DestroyBindGroupLayout(r.inputBindLayout)
		r.inputBindLayout = nil
	}
	if r.outputBindLayout != nil {
		r.device.DestroyBindGroupLayout(r.outputBindLayout)
		r.outputBindLayout = nil
	}

	// Destroy shader module
	if r.shaderModule != nil {
		r.device.DestroyShaderModule(r.shaderModule)
		r.shaderModule = nil
	}

	r.initialized = false
}

// Helper functions

func boolToUint32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// Note: absf32 and clampf32 are defined in fine.go/flatten.go

// Byte serialization helpers (for future GPU buffer upload)

func writeUint32(buf []byte, offset int, val uint32) {
	buf[offset] = byte(val)
	buf[offset+1] = byte(val >> 8)
	buf[offset+2] = byte(val >> 16)
	buf[offset+3] = byte(val >> 24)
}

func writeInt32(buf []byte, offset int, val int32) {
	//nolint:gosec // Intentional bit-cast for GPU buffer serialization
	writeUint32(buf, offset, uint32(val))
}

func writeFloat32(buf []byte, offset int, val float32) {
	bits := math.Float32bits(val)
	writeUint32(buf, offset, bits)
}

func segmentsToBytes(segments []GPUSegment) []byte {
	buf := make([]byte, len(segments)*32)
	for i, seg := range segments {
		off := i * 32
		writeFloat32(buf, off+0, seg.X0)
		writeFloat32(buf, off+4, seg.Y0)
		writeFloat32(buf, off+8, seg.X1)
		writeFloat32(buf, off+12, seg.Y1)
		writeInt32(buf, off+16, seg.Winding)
		writeInt32(buf, off+20, seg.TileY0)
		writeInt32(buf, off+24, seg.TileY1)
		writeInt32(buf, off+28, seg.Padding)
	}
	return buf
}

func tileRefsToBytes(refs []GPUTileSegmentRef) []byte {
	buf := make([]byte, len(refs)*16)
	for i, ref := range refs {
		off := i * 16
		writeUint32(buf, off+0, ref.TileX)
		writeUint32(buf, off+4, ref.TileY)
		writeUint32(buf, off+8, ref.SegmentIdx)
		writeUint32(buf, off+12, ref.WindingFlag)
	}
	return buf
}

func tilesToBytes(tiles []GPUTileInfo) []byte {
	buf := make([]byte, len(tiles)*32)
	for i, tile := range tiles {
		off := i * 32
		writeUint32(buf, off+0, tile.TileX)
		writeUint32(buf, off+4, tile.TileY)
		writeUint32(buf, off+8, tile.StartIdx)
		writeUint32(buf, off+12, tile.Count)
		writeInt32(buf, off+16, tile.Backdrop)
		writeUint32(buf, off+20, tile.Padding1)
		writeUint32(buf, off+24, tile.Padding2)
		writeUint32(buf, off+28, tile.Padding3)
	}
	return buf
}
