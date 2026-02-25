// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !nogpu

// vello_compute.go defines the GPU dispatch orchestration for the Vello-style
// compute pipeline. It manages shader compilation, buffer allocation, and the
// 8-stage dispatch sequence that mirrors the CPU reference in tilecompute/.

package gpu

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// =============================================================================
// Embedded WGSL Shader Sources
// =============================================================================

// Shader sources are embedded from the tilecompute/shaders directory.
// Each file corresponds to one stage of the Vello compute pipeline.

//go:embed tilecompute/shaders/pathtag_reduce.wgsl
var shaderPathtagReduce string

//go:embed tilecompute/shaders/pathtag_scan.wgsl
var shaderPathtagScan string

//go:embed tilecompute/shaders/draw_reduce.wgsl
var shaderDrawReduce string

//go:embed tilecompute/shaders/draw_leaf.wgsl
var shaderDrawLeaf string

//go:embed tilecompute/shaders/path_count.wgsl
var shaderPathCount string

//go:embed tilecompute/shaders/backdrop.wgsl
var shaderBackdrop string

//go:embed tilecompute/shaders/coarse.wgsl
var shaderVelloCoarse string

//go:embed tilecompute/shaders/path_tiling.wgsl
var shaderPathTiling string

//go:embed tilecompute/shaders/fine.wgsl
var shaderVelloFine string

// =============================================================================
// Constants
// =============================================================================

const (
	// velloWGSize is the workgroup size used by all Vello compute shaders.
	// This matches the WG_SIZE constant in every WGSL shader.
	velloWGSize = 256

	// Tile dimensions (VelloTileWidth, VelloTileHeight) are defined in vello_tiles.go.
	// They match TILE_WIDTH/TILE_HEIGHT in path_count.wgsl, coarse.wgsl, fine.wgsl.

	// velloPTCLMaxPerTile is the maximum PTCL words per tile.
	// Each tile's command list is bounded to this size.
	velloPTCLMaxPerTile = 1024

	// velloFenceTimeout is the maximum time to wait for GPU work to complete.
	velloFenceTimeout = 5 * time.Second
)

// =============================================================================
// VelloComputeStage
// =============================================================================

// VelloComputeStage identifies one of the 8 stages in the Vello compute pipeline.
// All constants are prefixed with "Vello" to avoid conflicts with the existing
// PipelineStage enum (StageCoarse, StageFine) in sparse_strips_gpu.go.
type VelloComputeStage int

const (
	// VelloStagePathtagReduce performs parallel reduction of PathMonoid over path tag words.
	// Input: scene (path tags). Output: reduced (per-workgroup PathMonoid sums).
	VelloStagePathtagReduce VelloComputeStage = iota

	// VelloStagePathtagScan performs a two-level prefix scan of PathMonoid.
	// Input: scene + reduced. Output: tag_monoids (per-tag-word exclusive prefix sums).
	VelloStagePathtagScan

	// VelloStageDrawReduce performs parallel reduction of DrawMonoid over draw tags.
	// Input: scene (draw tags). Output: draw_reduced (per-workgroup DrawMonoid sums).
	VelloStageDrawReduce

	// VelloStageDrawLeaf performs DrawMonoid scan and extracts per-draw info (colors).
	// Input: scene + draw_reduced. Output: draw_monoids + info.
	VelloStageDrawLeaf

	// VelloStagePathCount performs DDA tile walk, backdrop computation, and segment counting.
	// Input: lines + paths. Output: tiles (backdrop + segment counts via atomics).
	VelloStagePathCount

	// VelloStageBackdrop accumulates backdrop values left-to-right per tile row.
	// Input: paths + tiles. Output: tiles (with accumulated backdrop).
	VelloStageBackdrop

	// VelloStageCoarse generates Per-Tile Command Lists (PTCL) for each tile.
	// It also allocates segment slots via atomicAdd and writes inverted indices
	// to tiles for path_tiling to consume.
	// Input: draw_monoids + info + paths + tiles + bump. Output: ptcl + tiles (inverted indices).
	VelloStageCoarse

	// VelloStagePathTiling clips line segments to tile boundaries and writes PathSegment data.
	// Reads inverted indices (~seg_ix) from tiles (written by coarse) to know where to write.
	// Input: bump + seg_counts + lines + paths + tiles. Output: segments.
	VelloStagePathTiling

	// VelloStageFine performs per-pixel rasterization driven by PTCL command streams.
	// Input: ptcl + segments + tiles. Output: output pixel buffer.
	VelloStageFine

	// VelloStageCount is the total number of pipeline stages.
	VelloStageCount
)

// String returns the human-readable name of the compute stage.
func (s VelloComputeStage) String() string {
	switch s {
	case VelloStagePathtagReduce:
		return "pathtag_reduce"
	case VelloStagePathtagScan:
		return "pathtag_scan"
	case VelloStageDrawReduce:
		return "draw_reduce"
	case VelloStageDrawLeaf:
		return "draw_leaf"
	case VelloStagePathCount:
		return "path_count"
	case VelloStageBackdrop:
		return "backdrop"
	case VelloStageCoarse:
		return "coarse"
	case VelloStagePathTiling:
		return "path_tiling"
	case VelloStageFine:
		return "fine"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// =============================================================================
// VelloComputeConfig
// =============================================================================

// VelloComputeConfig holds the parameters for the Vello compute pipeline
// that map to the Config uniform buffer in WGSL shaders.
//
// This struct must match the Config struct defined in every WGSL shader:
// 14 consecutive u32 fields in the same order. The struct is uploaded
// as a uniform buffer at binding(0) of group(0) for all stages.
type VelloComputeConfig struct {
	// WidthInTiles is the number of tile columns in the render target.
	WidthInTiles uint32

	// HeightInTiles is the number of tile rows in the render target.
	HeightInTiles uint32

	// TargetWidth is the render target width in pixels.
	TargetWidth uint32

	// TargetHeight is the render target height in pixels.
	TargetHeight uint32

	// NumDrawObj is the total number of draw objects in the scene.
	NumDrawObj uint32

	// NumPaths is the total number of paths in the scene.
	NumPaths uint32

	// NumClips is the total number of clip operations.
	NumClips uint32

	// PathTagBase is the offset (in u32 words) of path tags in the scene buffer.
	PathTagBase uint32

	// PathDataBase is the offset (in u32 words) of path data in the scene buffer.
	PathDataBase uint32

	// DrawTagBase is the offset (in u32 words) of draw tags in the scene buffer.
	DrawTagBase uint32

	// DrawDataBase is the offset (in u32 words) of draw data in the scene buffer.
	DrawDataBase uint32

	// TransformBase is the offset (in u32 words) of transforms in the scene buffer.
	TransformBase uint32

	// StyleBase is the offset (in u32 words) of styles in the scene buffer.
	StyleBase uint32

	// NumLines is the total number of flattened line segments.
	NumLines uint32

	// BgColor is the background color packed as RGBA u32 (R in low byte).
	BgColor uint32
}

// sizeInBytes returns the byte size of VelloComputeConfig.
// 15 fields * 4 bytes = 60 bytes.
func (c VelloComputeConfig) sizeInBytes() uint64 {
	return 15 * 4 // 60 bytes
}

// toBytes serializes VelloComputeConfig to a byte slice in little-endian format.
// The layout matches the WGSL Config struct: 14 consecutive u32 fields.
func (c VelloComputeConfig) toBytes() []byte {
	buf := make([]byte, c.sizeInBytes())
	le := binary.LittleEndian
	le.PutUint32(buf[0:4], c.WidthInTiles)
	le.PutUint32(buf[4:8], c.HeightInTiles)
	le.PutUint32(buf[8:12], c.TargetWidth)
	le.PutUint32(buf[12:16], c.TargetHeight)
	le.PutUint32(buf[16:20], c.NumDrawObj)
	le.PutUint32(buf[20:24], c.NumPaths)
	le.PutUint32(buf[24:28], c.NumClips)
	le.PutUint32(buf[28:32], c.PathTagBase)
	le.PutUint32(buf[32:36], c.PathDataBase)
	le.PutUint32(buf[36:40], c.DrawTagBase)
	le.PutUint32(buf[40:44], c.DrawDataBase)
	le.PutUint32(buf[44:48], c.TransformBase)
	le.PutUint32(buf[48:52], c.StyleBase)
	le.PutUint32(buf[52:56], c.NumLines)
	le.PutUint32(buf[56:60], c.BgColor)
	return buf
}

// =============================================================================
// VelloComputeBuffers
// =============================================================================

// VelloComputeBuffers holds all GPU buffer references for the Vello pipeline.
// Buffers are allocated once per frame and reused across stages. Each buffer
// maps to one or more shader bindings across the 8 pipeline stages.
type VelloComputeBuffers struct {
	// Config is the uniform buffer containing VelloComputeConfig.
	// Bound at group(0) binding(0) in all stages.
	Config hal.Buffer

	// Scene is the packed scene data buffer containing path tags, path data,
	// draw tags, draw data, transforms, and styles as a flat u32 array.
	// Bound as storage(read) in pathtag_reduce, pathtag_scan, draw_reduce,
	// draw_leaf, coarse.
	Scene hal.Buffer

	// Reduced holds per-workgroup PathMonoid sums from pathtag_reduce.
	// Size: ceil(n_tag_words / 256) * sizeof(PathMonoid).
	// Written by pathtag_reduce, read by pathtag_scan.
	Reduced hal.Buffer

	// TagMonoids holds per-tag-word PathMonoid exclusive prefix sums.
	// Size: n_tag_words * sizeof(PathMonoid).
	// Written by pathtag_scan.
	TagMonoids hal.Buffer

	// DrawReduced holds per-workgroup DrawMonoid sums from draw_reduce.
	// Size: ceil(n_drawobj / 256) * sizeof(DrawMonoid).
	// Written by draw_reduce, read by draw_leaf.
	DrawReduced hal.Buffer

	// DrawMonoids holds per-draw DrawMonoid exclusive prefix sums.
	// Size: n_drawobj * sizeof(DrawMonoid).
	// Written by draw_leaf, read by coarse.
	DrawMonoids hal.Buffer

	// Info holds extracted draw info (packed RGBA colors).
	// Size: n_drawobj * sizeof(u32).
	// Written by draw_leaf, read by coarse, fine.
	Info hal.Buffer

	// Lines holds flattened line segments (LineSoup structs).
	// Size: n_lines * sizeof(LineSoup) = n_lines * 5 * sizeof(u32).
	// Read by path_count.
	Lines hal.Buffer

	// Paths holds per-path metadata (bounding box + tiles offset).
	// Size: n_paths * sizeof(Path) = n_paths * 5 * sizeof(u32).
	// Read by path_count, backdrop, coarse.
	Paths hal.Buffer

	// Tiles holds per-tile backdrop and segment count/index.
	// Size: total_tiles * sizeof(Tile) = total_tiles * 2 * sizeof(u32).
	// Written by path_count (atomics), modified by backdrop, read by coarse/fine.
	Tiles hal.Buffer

	// SegCounts holds segment counts per tile for path_count.
	// Size: estimated from segment count heuristic.
	// Written by path_count.
	SegCounts hal.Buffer

	// Segments holds clipped path segments (PathSegment, tile-relative coordinates).
	// Size: total_segments * sizeof(PathSegment) = total_segments * 5 * sizeof(f32).
	// Written by path_count segment allocation, read by coarse, fine.
	Segments hal.Buffer

	// PTCL holds per-tile command lists as a flat u32 stream.
	// Size: total_tiles * velloPTCLMaxPerTile * sizeof(u32).
	// Written by coarse, read by fine.
	PTCL hal.Buffer

	// BumpAlloc holds bump allocator counters for dynamic allocation.
	// Used by path_count (segment counts) and coarse (PTCL offsets).
	BumpAlloc hal.Buffer

	// TilePTCLOffsets holds per-tile PTCL write positions.
	// Size: total_tiles * sizeof(u32).
	// Written by coarse.
	TilePTCLOffsets hal.Buffer

	// PathStyles holds style flags per path (bit 1 = even-odd fill rule).
	// Size: n_paths * sizeof(u32).
	// Read by coarse.
	PathStyles hal.Buffer

	// Output holds the output pixel buffer (packed RGBA u32 per pixel).
	// Size: target_width * target_height * sizeof(u32).
	// Written by fine.
	Output hal.Buffer
}

// =============================================================================
// VelloComputeDispatcher
// =============================================================================

// VelloComputeDispatcher orchestrates the Vello-style compute pipeline.
// It manages shader compilation, buffer allocation, and the 8-stage
// dispatch sequence that mirrors the CPU reference in tilecompute/.
//
// Pipeline stages (in dispatch order):
//  1. pathtag_reduce -- parallel reduction of PathMonoid over path tags
//  2. pathtag_scan   -- prefix scan of PathMonoid (two-level)
//  3. draw_reduce    -- parallel reduction of DrawMonoid over draw tags
//  4. draw_leaf      -- DrawMonoid scan + draw info extraction
//  5. path_count     -- DDA tile walk, backdrop, segment counting
//  6. backdrop       -- left-to-right backdrop accumulation per tile row
//  7. coarse         -- PTCL generation per tile
//  8. fine           -- per-pixel rasterization via PTCL commands
//
// Each stage's output feeds into subsequent stages through GPU storage buffers.
// All inter-stage data flows through the buffers in VelloComputeBuffers.
//
// Reference: internal/gpu/tilecompute/ (CPU implementation)
// Reference: internal/gpu/tilecompute/shaders/ (WGSL shaders)
type VelloComputeDispatcher struct {
	mu sync.RWMutex

	// device is the HAL device providing GPU resource creation.
	device hal.Device

	// queue is the HAL queue for command submission and buffer writes.
	queue hal.Queue

	// pipelines are the compiled compute pipelines, one per stage.
	pipelines [VelloStageCount]hal.ComputePipeline

	// pipelineLayouts are the pipeline layouts, one per stage.
	pipelineLayouts [VelloStageCount]hal.PipelineLayout

	// bgLayouts are the bind group layouts, one per stage.
	bgLayouts [VelloStageCount]hal.BindGroupLayout

	// shaderModules are the compiled shader modules, one per stage.
	shaderModules [VelloStageCount]hal.ShaderModule

	// shaderSources are the embedded WGSL shader sources, indexed by stage.
	shaderSources [VelloStageCount]string

	// initialized indicates whether shaders have been compiled.
	initialized bool

	// wgSize is the workgroup size used by all shaders.
	wgSize uint32
}

// NewVelloComputeDispatcher creates a new dispatcher attached to the given
// HAL device and queue. The dispatcher must be initialized with Init()
// before Dispatch() can be called.
func NewVelloComputeDispatcher(device hal.Device, queue hal.Queue) *VelloComputeDispatcher {
	d := &VelloComputeDispatcher{
		device: device,
		queue:  queue,
		wgSize: velloWGSize,
	}

	// Map embedded shader sources to stages.
	d.shaderSources = [VelloStageCount]string{
		VelloStagePathtagReduce: shaderPathtagReduce,
		VelloStagePathtagScan:   shaderPathtagScan,
		VelloStageDrawReduce:    shaderDrawReduce,
		VelloStageDrawLeaf:      shaderDrawLeaf,
		VelloStagePathCount:     shaderPathCount,
		VelloStageBackdrop:      shaderBackdrop,
		VelloStageCoarse:        shaderVelloCoarse,
		VelloStagePathTiling:    shaderPathTiling,
		VelloStageFine:          shaderVelloFine,
	}

	return d
}

// stageBindGroupLayoutEntries returns the bind group layout entries for a
// given compute stage. These entries match the @group(0) @binding(N) annotations
// in the corresponding WGSL shader files exactly.
func stageBindGroupLayoutEntries(stage VelloComputeStage) []gputypes.BindGroupLayoutEntry {
	// configUniform returns the layout entry for binding(0) = Config uniform buffer.
	// Every stage has this at binding 0.
	configUniform := gputypes.BindGroupLayoutEntry{
		Binding:    0,
		Visibility: gputypes.ShaderStageCompute,
		Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
	}
	storageRO := func(binding uint32) gputypes.BindGroupLayoutEntry {
		return gputypes.BindGroupLayoutEntry{
			Binding:    binding,
			Visibility: gputypes.ShaderStageCompute,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage},
		}
	}
	storageRW := func(binding uint32) gputypes.BindGroupLayoutEntry {
		return gputypes.BindGroupLayoutEntry{
			Binding:    binding,
			Visibility: gputypes.ShaderStageCompute,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
		}
	}

	switch stage {
	case VelloStagePathtagReduce:
		// @binding(0) uniform config
		// @binding(1) storage(read) scene
		// @binding(2) storage(read_write) reduced
		return []gputypes.BindGroupLayoutEntry{
			configUniform, storageRO(1), storageRW(2),
		}

	case VelloStagePathtagScan:
		// @binding(0) uniform config
		// @binding(1) storage(read) scene
		// @binding(2) storage(read) reduced
		// @binding(3) storage(read_write) tag_monoids
		return []gputypes.BindGroupLayoutEntry{
			configUniform, storageRO(1), storageRO(2), storageRW(3),
		}

	case VelloStageDrawReduce:
		// @binding(0) uniform config
		// @binding(1) storage(read) scene
		// @binding(2) storage(read_write) draw_reduced
		return []gputypes.BindGroupLayoutEntry{
			configUniform, storageRO(1), storageRW(2),
		}

	case VelloStageDrawLeaf:
		// @binding(0) uniform config
		// @binding(1) storage(read) scene
		// @binding(2) storage(read) draw_reduced
		// @binding(3) storage(read_write) draw_monoids
		// @binding(4) storage(read_write) info
		return []gputypes.BindGroupLayoutEntry{
			configUniform, storageRO(1), storageRO(2), storageRW(3), storageRW(4),
		}

	case VelloStagePathCount:
		// @binding(0) uniform config
		// @binding(1) storage(read) lines
		// @binding(2) storage(read) paths
		// @binding(3) storage(read_write) tiles
		// @binding(4) storage(read_write) seg_counts
		// @binding(5) storage(read_write) bump
		return []gputypes.BindGroupLayoutEntry{
			configUniform, storageRO(1), storageRO(2),
			storageRW(3), storageRW(4), storageRW(5),
		}

	case VelloStageBackdrop:
		// @binding(0) uniform config
		// @binding(1) storage(read) paths
		// @binding(2) storage(read_write) tiles
		return []gputypes.BindGroupLayoutEntry{
			configUniform, storageRO(1), storageRW(2),
		}

	case VelloStageCoarse:
		// @binding(0) uniform config
		// @binding(1) storage(read) scene
		// @binding(2) storage(read) draw_monoids
		// @binding(3) storage(read) info
		// @binding(4) storage(read) paths
		// @binding(5) storage(read_write) tiles       -- coarse writes inverted indices
		// @binding(6) storage(read_write) ptcl
		// @binding(7) storage(read_write) tile_ptcl_offsets
		// @binding(8) storage(read) path_styles
		// @binding(9) storage(read_write) bump        -- atomicAdd for segment allocation
		return []gputypes.BindGroupLayoutEntry{
			configUniform,
			storageRO(1), storageRO(2), storageRO(3),
			storageRO(4), storageRW(5),
			storageRW(6), storageRW(7),
			storageRO(8), storageRW(9),
		}

	case VelloStagePathTiling:
		// @binding(0) uniform config
		// @binding(1) storage(read_write) bump        -- reads seg_counts
		// @binding(2) storage(read) seg_counts
		// @binding(3) storage(read) lines
		// @binding(4) storage(read) paths
		// @binding(5) storage(read) tiles             -- reads inverted indices from coarse
		// @binding(6) storage(read_write) segments    -- writes PathSegment data
		return []gputypes.BindGroupLayoutEntry{
			configUniform,
			storageRW(1), storageRO(2),
			storageRO(3), storageRO(4), storageRO(5),
			storageRW(6),
		}

	case VelloStageFine:
		// @binding(0) uniform config
		// @binding(1) storage(read) ptcl
		// @binding(2) storage(read) segments
		// @binding(3) storage(read) info
		// @binding(4) storage(read_write) output
		return []gputypes.BindGroupLayoutEntry{
			configUniform, storageRO(1), storageRO(2), storageRO(3), storageRW(4),
		}

	default:
		return nil
	}
}

// Init compiles all WGSL shaders and creates compute pipelines.
// Must be called before Dispatch. It is safe to call Init multiple times;
// subsequent calls are no-ops if already initialized.
//
// Returns an error if any shader fails to compile or pipeline creation fails.
func (d *VelloComputeDispatcher) Init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.initialized {
		return nil
	}

	for i := VelloComputeStage(0); i < VelloStageCount; i++ {
		src := d.shaderSources[i]
		if src == "" {
			return fmt.Errorf("vello compute: missing shader source for stage %s", i)
		}

		stageName := fmt.Sprintf("vello_%s", i)

		// 1. Create shader module from WGSL source.
		module, err := d.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
			Label:  stageName,
			Source: hal.ShaderSource{WGSL: src},
		})
		if err != nil {
			d.destroyPartialInit(i)
			return fmt.Errorf("vello compute: create shader module for %s: %w", i, err)
		}
		d.shaderModules[i] = module

		// 2. Create bind group layout for this stage's bindings.
		entries := stageBindGroupLayoutEntries(i)
		bgLayout, err := d.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
			Label:   stageName + "_bgl",
			Entries: entries,
		})
		if err != nil {
			d.destroyPartialInit(i + 1) // module was already stored
			return fmt.Errorf("vello compute: create bind group layout for %s: %w", i, err)
		}
		d.bgLayouts[i] = bgLayout

		// 3. Create pipeline layout.
		pipelineLayout, err := d.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
			Label:            stageName + "_pl",
			BindGroupLayouts: []hal.BindGroupLayout{bgLayout},
		})
		if err != nil {
			d.destroyPartialInit(i + 1)
			return fmt.Errorf("vello compute: create pipeline layout for %s: %w", i, err)
		}
		d.pipelineLayouts[i] = pipelineLayout

		// 4. Create compute pipeline.
		pipeline, err := d.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
			Label:  stageName,
			Layout: pipelineLayout,
			Compute: hal.ComputeState{
				Module:     module,
				EntryPoint: "main",
			},
		})
		if err != nil {
			d.destroyPartialInit(i + 1)
			return fmt.Errorf("vello compute: create compute pipeline for %s: %w", i, err)
		}
		d.pipelines[i] = pipeline

		slogger().Debug("vello compute: pipeline created",
			"stage", i.String(),
			"bindings", len(entries),
			"shader_bytes", len(src))
	}

	slogger().Info("vello compute: all pipelines initialized",
		"stages", int(VelloStageCount))

	d.initialized = true
	return nil
}

// destroyPartialInit cleans up resources for stages [0, upTo) during
// a failed Init(). This ensures no resource leaks on partial initialization.
func (d *VelloComputeDispatcher) destroyPartialInit(upTo VelloComputeStage) {
	for j := VelloComputeStage(0); j < upTo; j++ {
		if d.pipelines[j] != nil {
			d.device.DestroyComputePipeline(d.pipelines[j])
			d.pipelines[j] = nil
		}
		if d.pipelineLayouts[j] != nil {
			d.device.DestroyPipelineLayout(d.pipelineLayouts[j])
			d.pipelineLayouts[j] = nil
		}
		if d.bgLayouts[j] != nil {
			d.device.DestroyBindGroupLayout(d.bgLayouts[j])
			d.bgLayouts[j] = nil
		}
		if d.shaderModules[j] != nil {
			d.device.DestroyShaderModule(d.shaderModules[j])
			d.shaderModules[j] = nil
		}
	}
}

// Close releases all GPU resources held by the dispatcher.
// After Close, the dispatcher must be re-initialized with Init() before use.
func (d *VelloComputeDispatcher) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := VelloComputeStage(0); i < VelloStageCount; i++ {
		if d.pipelines[i] != nil {
			d.device.DestroyComputePipeline(d.pipelines[i])
			d.pipelines[i] = nil
		}
		if d.pipelineLayouts[i] != nil {
			d.device.DestroyPipelineLayout(d.pipelineLayouts[i])
			d.pipelineLayouts[i] = nil
		}
		if d.bgLayouts[i] != nil {
			d.device.DestroyBindGroupLayout(d.bgLayouts[i])
			d.bgLayouts[i] = nil
		}
		if d.shaderModules[i] != nil {
			d.device.DestroyShaderModule(d.shaderModules[i])
			d.shaderModules[i] = nil
		}
	}

	d.initialized = false
}

// ComputeWorkgroupCount returns the number of workgroups needed for a stage
// given the number of elements to process. This performs ceiling division:
//
//	workgroups = (elementCount + wgSize - 1) / wgSize
//
// Special cases:
//   - VelloStageBackdrop: one workgroup per path (elementCount = n_paths),
//     dispatched as (n_paths, 1, 1).
//   - VelloStageFine: one workgroup per tile (elementCount = width_in_tiles * height_in_tiles).
func (d *VelloComputeDispatcher) ComputeWorkgroupCount(stage VelloComputeStage, elementCount uint32) uint32 {
	if elementCount == 0 {
		return 0
	}

	switch stage {
	case VelloStageBackdrop:
		// Backdrop dispatches one workgroup per path. Thread 0 of each
		// workgroup performs the sequential left-to-right scan for that path.
		return elementCount

	case VelloStageFine:
		// Fine dispatches one workgroup per tile. Each workgroup has 256 threads
		// covering 16x16 = 256 pixels.
		return elementCount

	default:
		// Standard ceiling division for parallel reduction/scan stages.
		return (elementCount + d.wgSize - 1) / d.wgSize
	}
}

// velloBufSizes holds computed buffer sizes for a single frame.
type velloBufSizes struct {
	config          uint64
	scene           uint64
	reduced         uint64
	tagMonoids      uint64
	drawReduced     uint64
	drawMonoids     uint64
	info            uint64
	lines           uint64
	paths           uint64
	tiles           uint64
	segCounts       uint64
	segments        uint64
	ptcl            uint64
	bumpAlloc       uint64
	tilePTCLOffsets uint64
	pathStyles      uint64
	output          uint64
}

// computeBufferSizes calculates byte sizes for all Vello pipeline buffers.
// totalPathTiles is the sum of per-path tile counts (bboxW*bboxH), used for the
// Tiles buffer. This is different from WidthInTiles*HeightInTiles (the global grid).
func (d *VelloComputeDispatcher) computeBufferSizes(
	config VelloComputeConfig,
	sceneWords, lineWords, pathWords int,
	numLines, totalPathTiles uint32,
) velloBufSizes {
	// Number of path tag words.
	nTagWords := uint32(0)
	if config.DrawTagBase > config.PathTagBase {
		nTagWords = config.DrawTagBase - config.PathTagBase
	}

	nPathtagWG := d.ComputeWorkgroupCount(VelloStagePathtagReduce, nTagWords)
	nDrawWG := d.ComputeWorkgroupCount(VelloStageDrawReduce, config.NumDrawObj)
	// globalTiles is the number of tiles in the render target grid.
	// Used for PTCL, TilePTCLOffsets, and Fine stage dispatch.
	globalTiles := config.WidthInTiles * config.HeightInTiles

	// Struct sizes in bytes (must match WGSL struct layouts).
	const (
		pathMonoidSize   = 5 * 4 // PathMonoid: 5 u32 fields = 20 bytes
		drawMonoidSize   = 4 * 4 // DrawMonoid: 4 u32 fields = 16 bytes
		tileSize         = 2 * 4 // Tile: 2 fields (i32 + u32) = 8 bytes
		pathSegmentSize  = 5 * 4 // PathSegment: 5 f32 fields = 20 bytes
		segmentCountSize = 2 * 4 // SegmentCount: 2 u32 fields = 8 bytes
	)

	// Estimate segment count as 4x the number of lines (heuristic).
	estimatedSegments := uint64(numLines) * 4

	return velloBufSizes{
		config:          config.sizeInBytes(),
		scene:           uint64(sceneWords) * 4,
		reduced:         uint64(nPathtagWG) * pathMonoidSize,
		tagMonoids:      uint64(nTagWords) * pathMonoidSize,
		drawReduced:     uint64(nDrawWG) * drawMonoidSize,
		drawMonoids:     uint64(config.NumDrawObj) * drawMonoidSize,
		info:            uint64(config.NumDrawObj) * 4,
		lines:           uint64(lineWords) * 4,
		paths:           uint64(pathWords) * 4,
		tiles:           uint64(totalPathTiles) * tileSize, // per-path tile allocation, NOT global grid
		segCounts:       estimatedSegments * segmentCountSize,
		segments:        estimatedSegments * pathSegmentSize,
		ptcl:            uint64(globalTiles) * velloPTCLMaxPerTile * 4,
		bumpAlloc:       16,
		tilePTCLOffsets: uint64(globalTiles) * 4,
		pathStyles:      uint64(config.NumPaths) * 4,
		output:          uint64(config.TargetWidth) * uint64(config.TargetHeight) * 4,
	}
}

// createVelloBuffer creates a single GPU buffer with a minimum size guarantee.
func (d *VelloComputeDispatcher) createVelloBuffer(label string, size uint64, usage gputypes.BufferUsage) (hal.Buffer, error) {
	const minBufSize = 4
	if size < minBufSize {
		size = minBufSize
	}
	return d.device.CreateBuffer(&hal.BufferDescriptor{
		Label: label,
		Size:  size,
		Usage: usage,
	})
}

// AllocateBuffers creates GPU buffers for a frame with the given scene layout
// and returns a VelloComputeBuffers struct with live GPU buffer handles.
//
// Parameters:
//   - config: pipeline configuration with tile/scene layout info.
//   - sceneData: packed scene data as a flat u32 slice.
//   - lines: flattened LineSoup data as a flat u32 slice.
//   - paths: per-path metadata as a flat u32 slice.
//   - numLines: total number of line segments.
//   - totalPathTiles: sum of per-path tile counts (bboxW*bboxH for each path).
//     This determines the Tiles buffer size and is NOT the same as
//     widthInTiles*heightInTiles (the global tile grid).
//
// The caller must call DestroyBuffers when the buffers are no longer needed.
func (d *VelloComputeDispatcher) AllocateBuffers(
	config VelloComputeConfig,
	sceneData []uint32,
	lines []uint32,
	paths []uint32,
	numLines uint32,
	totalPathTiles uint32,
) (*VelloComputeBuffers, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.initialized {
		return nil, fmt.Errorf("vello compute: dispatcher not initialized, call Init() first")
	}

	sz := d.computeBufferSizes(config, len(sceneData), len(lines), len(paths), numLines, totalPathTiles)
	bufs := &VelloComputeBuffers{}

	// Buffer usage flags:
	// - storageZero: GPU-side storage that must be zero-initialized (atomics, accumulators).
	// - storageCPU:  GPU storage with CPU write access (scene data uploads).
	// - storageGPU:  GPU-only storage (intermediate results, overwritten by shaders).
	// - uniformCPU:  Uniform buffer with CPU write access (config).
	// - storageOut:  GPU storage with CPU read access (output readback).
	storageZero := gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst
	storageCPU := gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst
	storageGPU := gputypes.BufferUsageStorage
	uniformCPU := gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst
	storageOut := gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc

	// bufSpec maps a label and size to a target pointer, usage flags, and zero-init flag.
	type bufSpec struct {
		target   *hal.Buffer
		label    string
		size     uint64
		usage    gputypes.BufferUsage
		zeroInit bool // Must be zero-filled before dispatch (atomics, PTCL).
	}

	specs := []bufSpec{
		{&bufs.Config, "vello_config", sz.config, uniformCPU, false},
		{&bufs.Scene, "vello_scene", sz.scene, storageCPU, false},
		{&bufs.Reduced, "vello_reduced", sz.reduced, storageGPU, false},
		{&bufs.TagMonoids, "vello_tag_monoids", sz.tagMonoids, storageGPU, false},
		{&bufs.DrawReduced, "vello_draw_reduced", sz.drawReduced, storageGPU, false},
		{&bufs.DrawMonoids, "vello_draw_monoids", sz.drawMonoids, storageGPU, false},
		{&bufs.Info, "vello_info", sz.info, storageGPU, false},
		{&bufs.Lines, "vello_lines", sz.lines, storageCPU | gputypes.BufferUsageCopySrc, false},
		{&bufs.Paths, "vello_paths", sz.paths, storageCPU | gputypes.BufferUsageCopySrc, false},
		{&bufs.Tiles, "vello_tiles", sz.tiles, storageZero | gputypes.BufferUsageCopySrc, true},              // atomicAdd in path_count
		{&bufs.SegCounts, "vello_seg_counts", sz.segCounts, storageGPU, false},                               // written by path_count
		{&bufs.Segments, "vello_segments", sz.segments, storageGPU, false},                                   // written by coarse
		{&bufs.PTCL, "vello_ptcl", sz.ptcl, storageZero, true},                                               // CMD_END=0 sentinel
		{&bufs.BumpAlloc, "vello_bump_alloc", sz.bumpAlloc, storageZero | gputypes.BufferUsageCopySrc, true}, // atomicAdd in path_count
		{&bufs.TilePTCLOffsets, "vello_tile_ptcl_offsets", sz.tilePTCLOffsets, storageZero, true},            // coarse write positions
		{&bufs.PathStyles, "vello_path_styles", sz.pathStyles, storageCPU, false},
		{&bufs.Output, "vello_output", sz.output, storageOut, false},
	}

	for _, s := range specs {
		buf, err := d.createVelloBuffer(s.label, s.size, s.usage)
		if err != nil {
			d.DestroyBuffers(bufs)
			return nil, fmt.Errorf("vello compute: create %s buffer: %w", s.label, err)
		}
		*s.target = buf

		// Zero-fill buffers that use atomics or require sentinel values.
		if s.zeroInit && s.size > 0 {
			zeros := make([]byte, s.size)
			d.queue.WriteBuffer(buf, 0, zeros)
		}
	}

	globalTiles := config.WidthInTiles * config.HeightInTiles
	slogger().Debug("vello compute: buffers allocated",
		"target", fmt.Sprintf("%dx%d", config.TargetWidth, config.TargetHeight),
		"tile_grid", fmt.Sprintf("%dx%d=%d", config.WidthInTiles, config.HeightInTiles, globalTiles),
		"total_path_tiles", totalPathTiles,
		"tiles_buf_bytes", sz.tiles,
		"scene_bytes", sz.scene,
		"output_bytes", sz.output)

	return bufs, nil
}

// DestroyBuffers releases all GPU buffers in the given VelloComputeBuffers.
// After this call, the buffers must not be used.
func (d *VelloComputeDispatcher) DestroyBuffers(bufs *VelloComputeBuffers) {
	if bufs == nil {
		return
	}

	destroyBuf := func(b hal.Buffer) {
		if b != nil {
			d.device.DestroyBuffer(b)
		}
	}

	destroyBuf(bufs.Config)
	destroyBuf(bufs.Scene)
	destroyBuf(bufs.Reduced)
	destroyBuf(bufs.TagMonoids)
	destroyBuf(bufs.DrawReduced)
	destroyBuf(bufs.DrawMonoids)
	destroyBuf(bufs.Info)
	destroyBuf(bufs.Lines)
	destroyBuf(bufs.Paths)
	destroyBuf(bufs.Tiles)
	destroyBuf(bufs.SegCounts)
	destroyBuf(bufs.Segments)
	destroyBuf(bufs.PTCL)
	destroyBuf(bufs.BumpAlloc)
	destroyBuf(bufs.TilePTCLOffsets)
	destroyBuf(bufs.PathStyles)
	destroyBuf(bufs.Output)

	// Zero out all fields to prevent accidental reuse.
	*bufs = VelloComputeBuffers{}
}

// stageBindGroupEntries returns the bind group entries for a given stage,
// mapping each binding index to the correct buffer from VelloComputeBuffers.
func stageBindGroupEntries(stage VelloComputeStage, bufs *VelloComputeBuffers) []gputypes.BindGroupEntry {
	entry := func(binding uint32, buf hal.Buffer) gputypes.BindGroupEntry {
		return gputypes.BindGroupEntry{
			Binding: binding,
			Resource: gputypes.BufferBinding{
				Buffer: buf.NativeHandle(),
				Offset: 0,
				Size:   0, // 0 = entire buffer
			},
		}
	}

	switch stage {
	case VelloStagePathtagReduce:
		return []gputypes.BindGroupEntry{
			entry(0, bufs.Config),
			entry(1, bufs.Scene),
			entry(2, bufs.Reduced),
		}

	case VelloStagePathtagScan:
		return []gputypes.BindGroupEntry{
			entry(0, bufs.Config),
			entry(1, bufs.Scene),
			entry(2, bufs.Reduced),
			entry(3, bufs.TagMonoids),
		}

	case VelloStageDrawReduce:
		return []gputypes.BindGroupEntry{
			entry(0, bufs.Config),
			entry(1, bufs.Scene),
			entry(2, bufs.DrawReduced),
		}

	case VelloStageDrawLeaf:
		return []gputypes.BindGroupEntry{
			entry(0, bufs.Config),
			entry(1, bufs.Scene),
			entry(2, bufs.DrawReduced),
			entry(3, bufs.DrawMonoids),
			entry(4, bufs.Info),
		}

	case VelloStagePathCount:
		return []gputypes.BindGroupEntry{
			entry(0, bufs.Config),
			entry(1, bufs.Lines),
			entry(2, bufs.Paths),
			entry(3, bufs.Tiles),
			entry(4, bufs.SegCounts),
			entry(5, bufs.BumpAlloc),
		}

	case VelloStageBackdrop:
		return []gputypes.BindGroupEntry{
			entry(0, bufs.Config),
			entry(1, bufs.Paths),
			entry(2, bufs.Tiles),
		}

	case VelloStageCoarse:
		return []gputypes.BindGroupEntry{
			entry(0, bufs.Config),
			entry(1, bufs.Scene),
			entry(2, bufs.DrawMonoids),
			entry(3, bufs.Info),
			entry(4, bufs.Paths),
			entry(5, bufs.Tiles),
			entry(6, bufs.PTCL),
			entry(7, bufs.TilePTCLOffsets),
			entry(8, bufs.PathStyles),
			entry(9, bufs.BumpAlloc),
		}

	case VelloStagePathTiling:
		return []gputypes.BindGroupEntry{
			entry(0, bufs.Config),
			entry(1, bufs.BumpAlloc),
			entry(2, bufs.SegCounts),
			entry(3, bufs.Lines),
			entry(4, bufs.Paths),
			entry(5, bufs.Tiles),
			entry(6, bufs.Segments),
		}

	case VelloStageFine:
		return []gputypes.BindGroupEntry{
			entry(0, bufs.Config),
			entry(1, bufs.PTCL),
			entry(2, bufs.Segments),
			entry(3, bufs.Info),
			entry(4, bufs.Output),
		}

	default:
		return nil
	}
}

// dispatchResources tracks per-frame GPU resources for cleanup.
type dispatchResources struct {
	device     hal.Device
	bindGroups []hal.BindGroup
	cmdBuf     hal.CommandBuffer
	fence      hal.Fence
}

// cleanup destroys all tracked per-frame resources.
func (r *dispatchResources) cleanup() {
	if r.fence != nil {
		r.device.DestroyFence(r.fence)
	}
	if r.cmdBuf != nil {
		r.device.FreeCommandBuffer(r.cmdBuf)
	}
	for _, g := range r.bindGroups {
		r.device.DestroyBindGroup(g)
	}
}

// Dispatch runs the complete 9-stage compute pipeline.
//
// Each stage dispatches the appropriate shader with the correct buffer
// bindings and workgroup counts. The stages must execute in order because
// each stage's output is consumed by subsequent stages.
//
// The dispatch sequence is:
//  1. pathtag_reduce: scene -> reduced (ceil(n_tag_words / 256) workgroups)
//  2. pathtag_scan:   scene + reduced -> tag_monoids (ceil(n_tag_words / 256) workgroups)
//  3. draw_reduce:    scene -> draw_reduced (ceil(n_drawobj / 256) workgroups)
//  4. draw_leaf:      scene + draw_reduced -> draw_monoids + info (ceil(n_drawobj / 256) workgroups)
//  5. path_count:     lines + paths -> tiles + seg_counts (ceil(n_lines / 256) workgroups)
//  6. backdrop:       paths + tiles -> tiles (n_paths workgroups)
//  7. coarse:         draw_monoids + paths + tiles + bump -> ptcl + tiles (ceil(n_drawobj / 256) wg)
//  8. path_tiling:    seg_counts + lines + paths + tiles -> segments (ceil(n_lines*4 / 256) wg)
//  9. fine:           ptcl + segments -> output (width_in_tiles * height_in_tiles workgroups)
//
// Returns an error if any stage fails or if the dispatcher is not initialized.
func (d *VelloComputeDispatcher) Dispatch(bufs *VelloComputeBuffers, config VelloComputeConfig) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.initialized {
		return fmt.Errorf("vello compute: dispatcher not initialized, call Init() first")
	}

	if bufs == nil {
		return fmt.Errorf("vello compute: buffers must not be nil")
	}

	// Upload config uniform to the GPU.
	d.queue.WriteBuffer(bufs.Config, 0, config.toBytes())

	// Number of path tag words for pathtag stages.
	nTagWords := uint32(0)
	if config.DrawTagBase > config.PathTagBase {
		nTagWords = config.DrawTagBase - config.PathTagBase
	}

	totalTiles := config.WidthInTiles * config.HeightInTiles

	// path_tiling element count: use estimated segment count (n_lines * 4).
	// The shader itself checks against atomicLoad(&bump.seg_counts) and
	// returns early for excess threads, so over-dispatching is safe.
	pathTilingElements := config.NumLines * 4

	stages := [VelloStageCount]stageDispatch{
		{VelloStagePathtagReduce, nTagWords},
		{VelloStagePathtagScan, nTagWords},
		{VelloStageDrawReduce, config.NumDrawObj},
		{VelloStageDrawLeaf, config.NumDrawObj},
		{VelloStagePathCount, config.NumLines},
		{VelloStageBackdrop, config.NumPaths},
		{VelloStageCoarse, totalTiles},
		{VelloStagePathTiling, pathTilingElements},
		{VelloStageFine, totalTiles},
	}

	res := &dispatchResources{device: d.device}
	defer res.cleanup()

	if err := d.encodeComputeStages(res, bufs, stages[:]); err != nil {
		return err
	}

	return d.submitAndWait(res)
}

// encodeComputeStages records all compute passes into a command buffer.
func (d *VelloComputeDispatcher) encodeComputeStages(
	res *dispatchResources,
	bufs *VelloComputeBuffers,
	stages []stageDispatch,
) error {
	encoder, err := d.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "vello_compute",
	})
	if err != nil {
		return fmt.Errorf("vello compute: create command encoder: %w", err)
	}

	if err := encoder.BeginEncoding("vello_compute"); err != nil {
		return fmt.Errorf("vello compute: begin encoding: %w", err)
	}

	for _, sd := range stages {
		wgCount := d.ComputeWorkgroupCount(sd.stage, sd.elements)
		if wgCount == 0 {
			continue
		}

		bg, bgErr := d.device.CreateBindGroup(&hal.BindGroupDescriptor{
			Label:   fmt.Sprintf("vello_%s_bg", sd.stage),
			Layout:  d.bgLayouts[sd.stage],
			Entries: stageBindGroupEntries(sd.stage, bufs),
		})
		if bgErr != nil {
			encoder.DiscardEncoding()
			return fmt.Errorf("vello compute: create bind group for %s: %w", sd.stage, bgErr)
		}
		res.bindGroups = append(res.bindGroups, bg)

		pass := encoder.BeginComputePass(&hal.ComputePassDescriptor{
			Label: fmt.Sprintf("vello_%s", sd.stage),
		})
		pass.SetPipeline(d.pipelines[sd.stage])
		pass.SetBindGroup(0, bg, nil)
		pass.Dispatch(wgCount, 1, 1)
		pass.End()

		slogger().Debug("vello compute: dispatched stage",
			"stage", sd.stage.String(),
			"elements", sd.elements,
			"workgroups", wgCount)
	}

	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("vello compute: end encoding: %w", err)
	}
	res.cmdBuf = cmdBuf
	return nil
}

// stageDispatch holds parameters for a single compute stage dispatch.
type stageDispatch struct {
	stage    VelloComputeStage
	elements uint32
}

// submitAndWait submits the command buffer and waits for GPU completion.
func (d *VelloComputeDispatcher) submitAndWait(res *dispatchResources) error {
	fence, err := d.device.CreateFence()
	if err != nil {
		return fmt.Errorf("vello compute: create fence: %w", err)
	}
	res.fence = fence

	if err := d.queue.Submit([]hal.CommandBuffer{res.cmdBuf}, fence, 1); err != nil {
		return fmt.Errorf("vello compute: submit: %w", err)
	}

	ok, err := d.device.Wait(fence, 1, velloFenceTimeout)
	if err != nil {
		return fmt.Errorf("vello compute: wait for GPU: %w", err)
	}
	if !ok {
		return fmt.Errorf("vello compute: GPU timeout after %v", velloFenceTimeout)
	}

	slogger().Debug("vello compute: all stages dispatched successfully",
		"stages", int(VelloStageCount))
	return nil
}
