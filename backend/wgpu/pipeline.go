package wgpu

import (
	"sync"

	"github.com/gogpu/gg/scene"
	"github.com/gogpu/wgpu/core"
)

// PipelineCache caches compiled GPU pipelines for rendering operations.
// It manages bind group layouts and pipelines for blit, blend, strip
// rasterization, and compositing operations.
//
// PipelineCache is safe for concurrent read access. Pipeline creation
// is synchronized internally.
type PipelineCache struct {
	mu sync.RWMutex

	// GPU device for pipeline creation
	device core.DeviceID

	// Shader modules reference
	shaders *ShaderModules

	// Cached render pipelines
	blitPipeline      StubPipelineID
	compositePipeline StubPipelineID

	// Blend mode pipelines (one per blend mode for now)
	blendPipelines map[scene.BlendMode]StubPipelineID

	// Compute pipeline for strip rasterization
	stripPipeline StubComputePipelineID

	// Bind group layouts
	blitLayout      StubBindGroupLayoutID
	blendLayout     StubBindGroupLayoutID
	stripLayout     StubBindGroupLayoutID
	compositeLayout StubBindGroupLayoutID

	// State
	initialized bool
}

// StubPipelineID is a placeholder for actual wgpu RenderPipelineID.
// This will be replaced with core.RenderPipelineID when wgpu support is complete.
type StubPipelineID uint64

// StubComputePipelineID is a placeholder for actual wgpu ComputePipelineID.
type StubComputePipelineID uint64

// StubBindGroupLayoutID is a placeholder for actual wgpu BindGroupLayoutID.
type StubBindGroupLayoutID uint64

// StubBindGroupID is a placeholder for actual wgpu BindGroupID.
type StubBindGroupID uint64

// InvalidPipelineID represents an invalid/uninitialized pipeline.
const InvalidPipelineID StubPipelineID = 0

// NewPipelineCache creates a new pipeline cache for the given device.
// It initializes all base pipelines using the provided shader modules.
//
// Returns an error if pipeline creation fails.
func NewPipelineCache(device core.DeviceID, shaders *ShaderModules) (*PipelineCache, error) {
	if shaders == nil || !shaders.IsValid() {
		return nil, ErrNotImplemented
	}

	pc := &PipelineCache{
		device:         device,
		shaders:        shaders,
		blendPipelines: make(map[scene.BlendMode]StubPipelineID),
	}

	// Create base pipelines
	if err := pc.createBlitPipeline(); err != nil {
		return nil, err
	}

	if err := pc.createStripPipeline(); err != nil {
		return nil, err
	}

	if err := pc.createCompositePipeline(); err != nil {
		return nil, err
	}

	pc.initialized = true
	return pc, nil
}

// createBlitPipeline creates the blit (texture copy) pipeline.
//
//nolint:unparam // error return prepared for when wgpu implementation is complete
func (pc *PipelineCache) createBlitPipeline() error {
	// Create bind group layout for blit: texture + sampler
	// Layout binding 0: texture
	// Layout binding 1: sampler
	pc.blitLayout = StubBindGroupLayoutID(1)

	// TODO: When wgpu is ready, create actual bind group layout:
	// layoutDesc := &types.BindGroupLayoutDescriptor{
	//     Entries: []types.BindGroupLayoutEntry{
	//         {
	//             Binding:    0,
	//             Visibility: types.ShaderStageFragment,
	//             Texture: &types.TextureBindingLayout{
	//                 SampleType:    types.TextureSampleTypeFloat,
	//                 ViewDimension: types.TextureViewDimension2D,
	//             },
	//         },
	//         {
	//             Binding:    1,
	//             Visibility: types.ShaderStageFragment,
	//             Sampler: &types.SamplerBindingLayout{
	//                 Type: types.SamplerBindingTypeFiltering,
	//             },
	//         },
	//     },
	// }
	// pc.blitLayout, err = core.CreateBindGroupLayout(pc.device, layoutDesc)

	// Create pipeline
	// TODO: Actual pipeline creation when wgpu is ready
	pc.blitPipeline = StubPipelineID(1)

	return nil
}

// createStripPipeline creates the strip rasterization compute pipeline.
//
//nolint:unparam // error return prepared for when wgpu implementation is complete
func (pc *PipelineCache) createStripPipeline() error {
	// Create bind group layout for strip compute:
	// Binding 0: Strip headers (storage buffer)
	// Binding 1: Coverage data (storage buffer)
	// Binding 2: Output texture (storage texture)
	// Binding 3: Params uniform
	pc.stripLayout = StubBindGroupLayoutID(2)

	// TODO: When wgpu is ready, create actual compute pipeline:
	// pipelineDesc := &types.ComputePipelineDescriptor{
	//     Layout: pipelineLayoutID,
	//     Compute: types.ProgrammableStageDescriptor{
	//         Module:     pc.shaders.Strip,
	//         EntryPoint: "main",
	//     },
	// }
	// pc.stripPipeline, err = core.CreateComputePipeline(pc.device, pipelineDesc)

	pc.stripPipeline = StubComputePipelineID(1)

	return nil
}

// createCompositePipeline creates the layer compositing pipeline.
//
//nolint:unparam // error return prepared for when wgpu implementation is complete
func (pc *PipelineCache) createCompositePipeline() error {
	// Create bind group layout for composite:
	// Binding 0: Layer textures (texture array or individual bindings)
	// Binding 1: Layer descriptors (uniform buffer)
	// Binding 2: Sampler
	pc.compositeLayout = StubBindGroupLayoutID(3)

	// TODO: Actual pipeline creation
	pc.compositePipeline = StubPipelineID(2)

	return nil
}

// GetBlitPipeline returns the blit pipeline.
func (pc *PipelineCache) GetBlitPipeline() StubPipelineID {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.blitPipeline
}

// GetBlendPipeline returns the pipeline for the specified blend mode.
// Pipelines are created on demand and cached.
func (pc *PipelineCache) GetBlendPipeline(mode scene.BlendMode) StubPipelineID {
	pc.mu.RLock()
	pipeline, ok := pc.blendPipelines[mode]
	pc.mu.RUnlock()

	if ok {
		return pipeline
	}

	// Create pipeline for this blend mode
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Double-check after acquiring write lock
	if pipeline, ok = pc.blendPipelines[mode]; ok {
		return pipeline
	}

	pipeline = pc.createBlendPipeline(mode)
	pc.blendPipelines[mode] = pipeline

	return pipeline
}

// createBlendPipeline creates a render pipeline for a specific blend mode.
func (pc *PipelineCache) createBlendPipeline(mode scene.BlendMode) StubPipelineID {
	// Create bind group layout for blend if not exists
	if pc.blendLayout == 0 {
		// Layout:
		// Binding 0: Source texture
		// Binding 1: Sampler
		// Binding 2: Blend params uniform
		pc.blendLayout = StubBindGroupLayoutID(4)
	}

	// TODO: When wgpu is ready, create actual blend pipeline with appropriate
	// blend state based on mode. For Porter-Duff modes, use hardware blending.
	// For advanced modes, use shader-based blending.

	// For now, return stub ID based on mode (mode is a small enum value)
	return StubPipelineID(100 + uint64(mode))
}

// GetStripPipeline returns the strip rasterization compute pipeline.
func (pc *PipelineCache) GetStripPipeline() StubComputePipelineID {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.stripPipeline
}

// GetCompositePipeline returns the compositing pipeline.
func (pc *PipelineCache) GetCompositePipeline() StubPipelineID {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.compositePipeline
}

// GetBlitLayout returns the bind group layout for blit operations.
func (pc *PipelineCache) GetBlitLayout() StubBindGroupLayoutID {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.blitLayout
}

// GetBlendLayout returns the bind group layout for blend operations.
func (pc *PipelineCache) GetBlendLayout() StubBindGroupLayoutID {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.blendLayout
}

// GetStripLayout returns the bind group layout for strip compute.
func (pc *PipelineCache) GetStripLayout() StubBindGroupLayoutID {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.stripLayout
}

// IsInitialized returns true if the cache has been initialized.
func (pc *PipelineCache) IsInitialized() bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.initialized
}

// Close releases all pipeline resources.
func (pc *PipelineCache) Close() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// TODO: When wgpu is ready, release all pipelines:
	// core.RenderPipelineDrop(pc.blitPipeline)
	// core.ComputePipelineDrop(pc.stripPipeline)
	// for _, p := range pc.blendPipelines {
	//     core.RenderPipelineDrop(p)
	// }

	pc.blitPipeline = InvalidPipelineID
	pc.stripPipeline = 0
	pc.compositePipeline = InvalidPipelineID
	pc.blendPipelines = nil
	pc.blitLayout = 0
	pc.blendLayout = 0
	pc.stripLayout = 0
	pc.compositeLayout = 0
	pc.initialized = false
}

// BlendPipelineCount returns the number of cached blend pipelines.
// Useful for debugging and monitoring.
func (pc *PipelineCache) BlendPipelineCount() int {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return len(pc.blendPipelines)
}

// WarmupBlendPipelines pre-creates pipelines for commonly used blend modes.
// This avoids pipeline compilation stutter during first use.
func (pc *PipelineCache) WarmupBlendPipelines() {
	commonModes := []scene.BlendMode{
		scene.BlendNormal,
		scene.BlendMultiply,
		scene.BlendScreen,
		scene.BlendOverlay,
		scene.BlendSourceOver,
	}

	for _, mode := range commonModes {
		_ = pc.GetBlendPipeline(mode)
	}
}

// BindGroupBuilder helps construct bind groups for rendering.
type BindGroupBuilder struct {
	device core.DeviceID
	layout StubBindGroupLayoutID
}

// NewBindGroupBuilder creates a new bind group builder.
func NewBindGroupBuilder(device core.DeviceID, layout StubBindGroupLayoutID) *BindGroupBuilder {
	return &BindGroupBuilder{
		device: device,
		layout: layout,
	}
}

// Build creates the bind group. Currently returns a stub.
func (b *BindGroupBuilder) Build() StubBindGroupID {
	// TODO: When wgpu is ready, create actual bind group
	return StubBindGroupID(1)
}

// CreateBlitBindGroup creates a bind group for blit operations.
func (pc *PipelineCache) CreateBlitBindGroup(tex *GPUTexture) StubBindGroupID {
	// TODO: When wgpu is ready:
	// entries := []types.BindGroupEntry{
	//     {Binding: 0, TextureView: tex.ViewID()},
	//     {Binding: 1, Sampler: pc.defaultSampler},
	// }
	// return core.CreateBindGroup(pc.device, pc.blitLayout, entries)

	return StubBindGroupID(1)
}

// CreateBlendBindGroup creates a bind group for blend operations.
func (pc *PipelineCache) CreateBlendBindGroup(tex *GPUTexture, params *BlendParams) StubBindGroupID {
	// TODO: When wgpu is ready:
	// Upload params to uniform buffer
	// Create bind group with texture, sampler, and params buffer

	return StubBindGroupID(2)
}

// CreateStripBindGroup creates a bind group for strip compute operations.
func (pc *PipelineCache) CreateStripBindGroup(
	headerBuffer StubBufferID,
	coverageBuffer StubBufferID,
	outputTex *GPUTexture,
	params *StripParams,
) StubBindGroupID {
	// TODO: When wgpu is ready:
	// Create bind group with buffers, texture, and params

	return StubBindGroupID(3)
}

// StubBufferID is a placeholder for actual wgpu BufferID.
type StubBufferID uint64
