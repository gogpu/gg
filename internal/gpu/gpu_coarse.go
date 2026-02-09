//go:build !nogpu

// Package wgpu provides GPU-accelerated rendering using WebGPU.
package gpu

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

//go:embed shaders/coarse.wgsl
var coarseShaderWGSL string

// GPUCoarseConfig contains GPU coarse rasterization configuration.
// Must match CoarseConfig in coarse.wgsl.
type GPUCoarseConfig struct {
	ViewportWidth  uint32 // Viewport width in pixels
	ViewportHeight uint32 // Viewport height in pixels
	TileColumns    uint32 // Number of tile columns
	TileRows       uint32 // Number of tile rows
	SegmentCount   uint32 // Number of segments to process
	MaxEntries     uint32 // Maximum number of tile entries
	Padding1       uint32 // Padding for alignment
	Padding2       uint32 // Padding for alignment
}

// GPUCoarseRasterizer performs coarse rasterization (tile binning) on the GPU.
// It creates compute pipelines and manages GPU buffers for segment-to-tile mapping.
type GPUCoarseRasterizer struct {
	mu sync.Mutex

	device hal.Device
	queue  hal.Queue

	// Compute pipelines
	coarsePipeline hal.ComputePipeline
	clearPipeline  hal.ComputePipeline

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

	// Tile dimensions (computed from viewport)
	tileColumns uint16
	tileRows    uint16

	// State
	initialized bool
	shaderReady bool
}

// NewGPUCoarseRasterizer creates a new GPU coarse rasterizer.
// Returns an error if GPU compute is not supported.
func NewGPUCoarseRasterizer(device hal.Device, queue hal.Queue, width, height uint16) (*GPUCoarseRasterizer, error) {
	if device == nil || queue == nil {
		return nil, fmt.Errorf("gpu_coarse: device and queue are required")
	}

	r := &GPUCoarseRasterizer{
		device:      device,
		queue:       queue,
		width:       width,
		height:      height,
		tileColumns: (width + TileWidth - 1) / TileWidth,
		tileRows:    (height + TileHeight - 1) / TileHeight,
	}

	if err := r.init(); err != nil {
		r.Destroy()
		return nil, err
	}

	return r, nil
}

// init initializes GPU resources (pipelines, layouts).
func (r *GPUCoarseRasterizer) init() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Compile WGSL to SPIR-V using shared helper
	spirvCode, err := CompileShaderToSPIRV(coarseShaderWGSL)
	if err != nil {
		return fmt.Errorf("gpu_coarse: %w", err)
	}
	r.spirvCode = spirvCode
	r.shaderReady = true

	// Create shader module using shared helper
	shaderModule, err := CreateShaderModule(r.device, "coarse_shader", r.spirvCode)
	if err != nil {
		return fmt.Errorf("gpu_coarse: failed to create shader module: %w", err)
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
func (r *GPUCoarseRasterizer) createBindGroupLayouts() error {
	// Input bind group layout (group 0)
	inputLayout, err := r.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "coarse_input_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeUniform,
					MinBindingSize: 32, // sizeof(CoarseConfig)
				},
			},
			{
				Binding:    1,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeReadOnlyStorage,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_coarse: failed to create input bind group layout: %w", err)
	}
	r.inputBindLayout = inputLayout

	// Output bind group layout (group 1)
	outputLayout, err := r.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "coarse_output_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeStorage,
				},
			},
			{
				Binding:    1,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeStorage,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_coarse: failed to create output bind group layout: %w", err)
	}
	r.outputBindLayout = outputLayout

	return nil
}

// createPipelineLayout creates the pipeline layout.
func (r *GPUCoarseRasterizer) createPipelineLayout() error {
	layout, err := r.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "coarse_pipeline_layout",
		BindGroupLayouts: []hal.BindGroupLayout{r.inputBindLayout, r.outputBindLayout},
	})
	if err != nil {
		return fmt.Errorf("gpu_coarse: failed to create pipeline layout: %w", err)
	}
	r.pipelineLayout = layout
	return nil
}

// createPipelines creates the compute pipelines.
func (r *GPUCoarseRasterizer) createPipelines() error {
	// Main coarse rasterization pipeline
	coarsePipeline, err := r.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "coarse_pipeline",
		Layout: r.pipelineLayout,
		Compute: hal.ComputeState{
			Module:     r.shaderModule,
			EntryPoint: "cs_coarse",
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_coarse: failed to create coarse pipeline: %w", err)
	}
	r.coarsePipeline = coarsePipeline

	// Clear counter pipeline
	clearPipeline, err := r.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "coarse_clear_pipeline",
		Layout: r.pipelineLayout,
		Compute: hal.ComputeState{
			Module:     r.shaderModule,
			EntryPoint: "cs_clear_counter",
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_coarse: failed to create clear pipeline: %w", err)
	}
	r.clearPipeline = clearPipeline

	return nil
}

// Rasterize performs coarse rasterization on the GPU.
// It takes segments and produces tile entries.
//
// Note: Phase 6.2 implementation. Full GPU dispatch requires buffer binding
// which needs HAL API extensions. Currently falls back to CPU-computed entries.
func (r *GPUCoarseRasterizer) Rasterize(segments *SegmentList) ([]GPUTileSegmentRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return nil, fmt.Errorf("gpu_coarse: rasterizer not initialized")
	}

	if segments == nil || segments.Len() == 0 {
		return nil, nil
	}

	// Convert segments to GPU format
	gpuSegments := r.convertSegments(segments)

	// Calculate max entries (worst case: each segment touches multiple tiles)
	// Estimate: 4 entries per segment average (conservative)
	maxEntries := len(gpuSegments) * 4

	// Phase 6.2: GPU infrastructure is ready, but buffer binding needs HAL extension.
	// For now, compute tile entries on CPU using the same algorithm as the shader.
	entries := r.computeEntriesCPU(gpuSegments, maxEntries)

	return entries, nil
}

// computeEntriesCPU computes tile entries using CPU (mirrors GPU shader algorithm).
// This serves as reference implementation and fallback.
func (r *GPUCoarseRasterizer) computeEntriesCPU(
	segments []GPUSegment,
	maxEntries int,
) []GPUTileSegmentRef {
	entries := make([]GPUTileSegmentRef, 0, maxEntries)

	for segIdx, seg := range segments {
		// Convert to tile coordinates
		p0x := seg.X0 / float32(TileSize)
		p0y := seg.Y0 / float32(TileSize)
		p1x := seg.X1 / float32(TileSize)
		p1y := seg.Y1 / float32(TileSize)

		// Determine left/right bounds
		lineLeftX := minf32(p0x, p1x)
		lineRightX := maxf32(p0x, p1x)

		// Cull lines fully to the right of viewport
		if lineLeftX > float32(r.tileColumns) {
			continue
		}

		// Line is monotonic (Y0 <= Y1)
		lineTopY := p0y
		lineTopX := p0x
		lineBottomY := p1y
		lineBottomX := p1x

		// Clamp to viewport rows
		yTopTiles := clampU16(int32(lineTopY), 0, int32(r.tileRows))
		yBottomTiles := clampU16(int32(lineBottomY+0.999999), 0, int32(r.tileRows))

		// Skip horizontal lines or lines fully outside viewport
		if yTopTiles >= yBottomTiles {
			continue
		}

		// Get tile coordinates for endpoints
		p0TileX := int32(lineTopX)
		p0TileY := int32(lineTopY)
		p1TileX := int32(lineBottomX)
		p1TileY := int32(lineBottomY)

		// Check if both endpoints are in the same tile
		sameX := p0TileX == p1TileX
		sameY := p0TileY == p1TileY

		if sameX && sameY {
			// Line fully contained in single tile
			x := clampU16(int32(lineLeftX), 0, int32(r.tileColumns-1))
			winding := uint32(0)
			if p0TileY >= int32(yTopTiles) {
				winding = 1
			}
			//nolint:gosec // segIdx is bounded by segments length which fits in uint32
			entries = append(entries, GPUTileSegmentRef{
				TileX:       uint32(x),
				TileY:       uint32(yTopTiles),
				SegmentIdx:  uint32(segIdx),
				WindingFlag: winding,
			})
			continue
		}

		// Handle vertical lines specially
		if lineLeftX == lineRightX {
			//nolint:gosec // segIdx is bounded by segments length which fits in uint32
			entries = r.processVerticalLineCPU(entries, uint32(segIdx), seg.X0, seg.Y0, seg.Y1)
			continue
		}

		// General sloped line
		//nolint:gosec // segIdx is bounded by segments length which fits in uint32
		entries = r.processSlopedLineCPU(entries, uint32(segIdx), seg.X0, seg.Y0, seg.X1, seg.Y1, yTopTiles)
	}

	return entries
}

// processVerticalLineCPU handles vertical line segments.
func (r *GPUCoarseRasterizer) processVerticalLineCPU(
	entries []GPUTileSegmentRef,
	segIdx uint32,
	x, topY, bottomY float32,
) []GPUTileSegmentRef {
	tileX := clampU16(int32(x/float32(TileSize)), 0, int32(r.tileColumns-1))

	yTopTiles := clampU16(int32(topY/float32(TileSize)), 0, int32(r.tileRows))
	yBottomTiles := clampU16(int32((bottomY+0.999999)/float32(TileSize)), 0, int32(r.tileRows))

	if yTopTiles >= yBottomTiles {
		return entries
	}

	// First tile
	isStartCulled := topY < 0
	if !isStartCulled {
		winding := uint32(0)
		if float32(yTopTiles)*float32(TileSize) >= topY {
			winding = 1
		}
		entries = append(entries, GPUTileSegmentRef{
			TileX:       uint32(tileX),
			TileY:       uint32(yTopTiles),
			SegmentIdx:  segIdx,
			WindingFlag: winding,
		})
	}

	// Middle tiles
	yStart := yTopTiles
	if !isStartCulled {
		yStart++
	}
	yEndIdx := clampU16(int32(bottomY/float32(TileSize)), 0, int32(r.tileRows))

	for y := yStart; y < yEndIdx; y++ {
		entries = append(entries, GPUTileSegmentRef{
			TileX:       uint32(tileX),
			TileY:       uint32(y),
			SegmentIdx:  segIdx,
			WindingFlag: 1,
		})
	}

	// Last tile
	bottomFloor := float32(int32(bottomY/float32(TileSize))) * float32(TileSize)
	if bottomY != bottomFloor && yEndIdx < r.tileRows {
		entries = append(entries, GPUTileSegmentRef{
			TileX:       uint32(tileX),
			TileY:       uint32(yEndIdx),
			SegmentIdx:  segIdx,
			WindingFlag: 1,
		})
	}

	return entries
}

// processSlopedLineCPU handles sloped line segments.
func (r *GPUCoarseRasterizer) processSlopedLineCPU(
	entries []GPUTileSegmentRef,
	segIdx uint32,
	lineTopX, lineTopY, lineBottomX, lineBottomY float32,
	yTopTiles uint16,
) []GPUTileSegmentRef {
	dx := lineBottomX - lineTopX
	dy := lineBottomY - lineTopY
	xSlope := dx / dy

	dxDir := lineBottomX >= lineTopX

	lineLeftX := minf32(lineTopX, lineBottomX)
	lineRightX := maxf32(lineTopX, lineBottomX)

	isStartCulled := lineTopY < 0

	// Process first row
	if !isStartCulled {
		y := float32(yTopTiles) * float32(TileSize)
		rowBottomY := minf32(y+float32(TileSize), lineBottomY)
		winding := uint32(0)
		if y >= lineTopY {
			winding = 1
		}
		entries = r.processRowCPU(entries, segIdx, lineTopY, lineTopX, xSlope, lineLeftX, lineRightX, y, rowBottomY, uint32(yTopTiles), winding, dxDir)
	}

	// Process middle rows
	yStartMiddle := yTopTiles
	if !isStartCulled {
		yStartMiddle++
	}
	yEndMiddle := clampU16(int32(lineBottomY/float32(TileSize)), 0, int32(r.tileRows))

	for y := yStartMiddle; y < yEndMiddle; y++ {
		yf := float32(y) * float32(TileSize)
		rowBottomY := minf32(yf+float32(TileSize), lineBottomY)
		entries = r.processRowCPU(entries, segIdx, lineTopY, lineTopX, xSlope, lineLeftX, lineRightX, yf, rowBottomY, uint32(y), 1, dxDir)
	}

	// Process last row
	bottomFloor := float32(int32(lineBottomY/float32(TileSize))) * float32(TileSize)
	if lineBottomY != bottomFloor && yEndMiddle < r.tileRows {
		if isStartCulled || yEndMiddle != yTopTiles {
			yf := float32(yEndMiddle) * float32(TileSize)
			entries = r.processRowCPU(entries, segIdx, lineTopY, lineTopX, xSlope, lineLeftX, lineRightX, yf, lineBottomY, uint32(yEndMiddle), 1, dxDir)
		}
	}

	return entries
}

// processRowCPU processes a single row of tiles for a sloped line.
func (r *GPUCoarseRasterizer) processRowCPU(
	entries []GPUTileSegmentRef,
	segIdx uint32,
	lineTopY, lineTopX, xSlope float32,
	lineLeftX, lineRightX float32,
	rowTopY, rowBottomY float32,
	yIdx uint32,
	winding uint32,
	dxDir bool,
) []GPUTileSegmentRef {
	// Calculate X range for this row
	rowTopX := lineTopX + (rowTopY-lineTopY)*xSlope
	rowBottomX := lineTopX + (rowBottomY-lineTopY)*xSlope

	rowLeftX := maxf32(minf32(rowTopX, rowBottomX), lineLeftX)
	rowRightX := minf32(maxf32(rowTopX, rowBottomX), lineRightX)

	xStart := clampU16(int32(rowLeftX/float32(TileSize)), 0, int32(r.tileColumns-1))
	xEnd := clampU16(int32(rowRightX/float32(TileSize)), 0, int32(r.tileColumns-1))

	if xStart > xEnd {
		return entries
	}

	// Single tile case
	if xStart == xEnd {
		entries = append(entries, GPUTileSegmentRef{
			TileX:       uint32(xStart),
			TileY:       yIdx,
			SegmentIdx:  segIdx,
			WindingFlag: winding,
		})
		return entries
	}

	// Multiple tiles
	if dxDir {
		entries = append(entries, GPUTileSegmentRef{
			TileX:       uint32(xStart),
			TileY:       yIdx,
			SegmentIdx:  segIdx,
			WindingFlag: winding,
		})
	} else {
		entries = append(entries, GPUTileSegmentRef{
			TileX:       uint32(xStart),
			TileY:       yIdx,
			SegmentIdx:  segIdx,
			WindingFlag: 0,
		})
	}

	// Middle tiles
	for x := xStart + 1; x < xEnd; x++ {
		entries = append(entries, GPUTileSegmentRef{
			TileX:       uint32(x),
			TileY:       yIdx,
			SegmentIdx:  segIdx,
			WindingFlag: 0,
		})
	}

	// Last tile
	if dxDir {
		entries = append(entries, GPUTileSegmentRef{
			TileX:       uint32(xEnd),
			TileY:       yIdx,
			SegmentIdx:  segIdx,
			WindingFlag: 0,
		})
	} else {
		entries = append(entries, GPUTileSegmentRef{
			TileX:       uint32(xEnd),
			TileY:       yIdx,
			SegmentIdx:  segIdx,
			WindingFlag: winding,
		})
	}

	return entries
}

// convertSegments converts CPU segments to GPU format.
func (r *GPUCoarseRasterizer) convertSegments(segments *SegmentList) []GPUSegment {
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

// GetTileEntries returns tile entries from a CoarseRasterizer.
// This is a convenience method that converts CPU coarse entries to GPU format.
func (r *GPUCoarseRasterizer) GetTileEntries(coarse *CoarseRasterizer) []GPUTileSegmentRef {
	if coarse == nil {
		return nil
	}

	entries := coarse.Entries()
	result := make([]GPUTileSegmentRef, len(entries))

	for i, e := range entries {
		result[i] = GPUTileSegmentRef{
			TileX:       uint32(e.X),
			TileY:       uint32(e.Y),
			SegmentIdx:  e.LineIdx,
			WindingFlag: boolToUint32(e.Winding),
		}
	}

	return result
}

// IsInitialized returns whether the rasterizer is initialized.
func (r *GPUCoarseRasterizer) IsInitialized() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.initialized
}

// IsShaderReady returns whether the shader compiled successfully.
func (r *GPUCoarseRasterizer) IsShaderReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shaderReady
}

// SPIRVCode returns the compiled SPIR-V code (for debugging/verification).
func (r *GPUCoarseRasterizer) SPIRVCode() []uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spirvCode
}

// TileColumns returns the number of tile columns.
func (r *GPUCoarseRasterizer) TileColumns() uint16 {
	return r.tileColumns
}

// TileRows returns the number of tile rows.
func (r *GPUCoarseRasterizer) TileRows() uint16 {
	return r.tileRows
}

// Destroy releases all GPU resources.
func (r *GPUCoarseRasterizer) Destroy() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.device == nil {
		return
	}

	// Destroy pipelines
	if r.coarsePipeline != nil {
		r.device.DestroyComputePipeline(r.coarsePipeline)
		r.coarsePipeline = nil
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

// Byte serialization helpers for GPU buffer upload

func coarseConfigToBytes(cfg GPUCoarseConfig) []byte {
	buf := make([]byte, 32)
	writeUint32(buf, 0, cfg.ViewportWidth)
	writeUint32(buf, 4, cfg.ViewportHeight)
	writeUint32(buf, 8, cfg.TileColumns)
	writeUint32(buf, 12, cfg.TileRows)
	writeUint32(buf, 16, cfg.SegmentCount)
	writeUint32(buf, 20, cfg.MaxEntries)
	writeUint32(buf, 24, cfg.Padding1)
	writeUint32(buf, 28, cfg.Padding2)
	return buf
}

func tileEntriesToBytes(entries []GPUTileSegmentRef) []byte {
	buf := make([]byte, len(entries)*16)
	for i, entry := range entries {
		off := i * 16
		writeUint32(buf, off+0, entry.TileX)
		writeUint32(buf, off+4, entry.TileY)
		writeUint32(buf, off+8, entry.SegmentIdx)
		writeUint32(buf, off+12, entry.WindingFlag)
	}
	return buf
}
