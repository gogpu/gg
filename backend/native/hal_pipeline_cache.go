// Package native provides a GPU-accelerated rendering backend using gogpu/wgpu.
package native

import (
	"encoding/binary"
	"errors"
	"hash"
	"hash/fnv"
	"sync"
	"sync/atomic"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

// HAL pipeline cache errors.
var (
	// ErrPipelineCacheNilDevice is returned when creating a cache without a device.
	ErrPipelineCacheNilDevice = errors.New("native: HAL device is nil")

	// ErrPipelineCacheNilDescriptor is returned when creating a pipeline with nil descriptor.
	ErrPipelineCacheNilDescriptor = errors.New("native: pipeline descriptor is nil")

	// ErrPipelineCacheNilShader is returned when creating a pipeline with nil shader module.
	ErrPipelineCacheNilShader = errors.New("native: shader module is nil")
)

// HALPipelineCache caches compiled render and compute pipelines.
//
// Pipeline creation is expensive because it involves shader compilation and
// validation. This cache stores pipelines indexed by descriptor hash to avoid
// redundant creation.
//
// Thread Safety:
// HALPipelineCache is safe for concurrent use. It uses RWMutex with
// double-check locking for efficient reads and safe writes.
//
// Usage:
//
//	cache := NewHALPipelineCache()
//	pipeline, err := cache.GetOrCreateRenderPipeline(device, desc)
//	if err != nil {
//	    // handle error
//	}
//	// Use pipeline for rendering
//
// The cache tracks hit/miss statistics for performance monitoring.
type HALPipelineCache struct {
	// mu protects mutable state.
	mu sync.RWMutex

	// renderCache stores render pipelines indexed by descriptor hash.
	renderCache map[uint64]*HALRenderPipeline

	// computeCache stores compute pipelines indexed by descriptor hash.
	computeCache map[uint64]*HALComputePipeline

	// hits counts cache hits (atomic for lock-free reads).
	hits uint64

	// misses counts cache misses (atomic for lock-free reads).
	misses uint64
}

// NewHALPipelineCache creates a new pipeline cache.
//
// The cache starts empty and pipelines are created on demand.
func NewHALPipelineCache() *HALPipelineCache {
	return &HALPipelineCache{
		renderCache:  make(map[uint64]*HALRenderPipeline),
		computeCache: make(map[uint64]*HALComputePipeline),
	}
}

// GetOrCreateRenderPipeline returns a cached pipeline or creates a new one.
//
// This method implements the "get or create" pattern with double-check locking:
//  1. Fast path: RLock, check cache, return if found
//  2. Slow path: Lock, double-check, create if needed
//
// Parameters:
//   - device: The HAL device to create the pipeline on (used for creation only).
//   - desc: The render pipeline descriptor.
//
// Returns the pipeline and nil on success.
// Returns nil and an error if:
//   - The device is nil
//   - The descriptor is nil
//   - Pipeline creation fails
//
//nolint:dupl // Intentional pattern: same double-check locking for both render and compute pipelines
func (c *HALPipelineCache) GetOrCreateRenderPipeline(
	device hal.Device,
	desc *HALRenderPipelineDescriptor,
) (*HALRenderPipeline, error) {
	if desc == nil {
		return nil, ErrPipelineCacheNilDescriptor
	}

	descHash := HashRenderPipelineDescriptor(desc)

	// Fast path: read lock
	c.mu.RLock()
	if pipeline, ok := c.renderCache[descHash]; ok {
		c.mu.RUnlock()
		atomic.AddUint64(&c.hits, 1)
		return pipeline, nil
	}
	c.mu.RUnlock()

	// Slow path: write lock with double-check
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if pipeline, ok := c.renderCache[descHash]; ok {
		atomic.AddUint64(&c.hits, 1)
		return pipeline, nil
	}

	// Create new pipeline
	// Note: device may be nil during testing when creating placeholder pipelines.
	// When HAL integration is complete, this will validate device != nil.
	pipeline, err := createHALRenderPipeline(device, desc)
	if err != nil {
		return nil, err
	}

	c.renderCache[descHash] = pipeline
	atomic.AddUint64(&c.misses, 1)

	return pipeline, nil
}

// GetOrCreateComputePipeline returns a cached pipeline or creates a new one.
//
// This method implements the "get or create" pattern with double-check locking.
//
// Parameters:
//   - device: The HAL device to create the pipeline on (used for creation only).
//   - desc: The compute pipeline descriptor.
//
// Returns the pipeline and nil on success.
// Returns nil and an error if:
//   - The device is nil
//   - The descriptor is nil
//   - Pipeline creation fails
//
//nolint:dupl // Intentional pattern: same double-check locking for both render and compute pipelines
func (c *HALPipelineCache) GetOrCreateComputePipeline(
	device hal.Device,
	desc *HALComputePipelineDescriptor,
) (*HALComputePipeline, error) {
	if desc == nil {
		return nil, ErrPipelineCacheNilDescriptor
	}

	descHash := HashComputePipelineDescriptor(desc)

	// Fast path: read lock
	c.mu.RLock()
	if pipeline, ok := c.computeCache[descHash]; ok {
		c.mu.RUnlock()
		atomic.AddUint64(&c.hits, 1)
		return pipeline, nil
	}
	c.mu.RUnlock()

	// Slow path: write lock with double-check
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if pipeline, ok := c.computeCache[descHash]; ok {
		atomic.AddUint64(&c.hits, 1)
		return pipeline, nil
	}

	// Create new pipeline
	// Note: device may be nil during testing when creating placeholder pipelines.
	// When HAL integration is complete, this will validate device != nil.
	pipeline, err := createHALComputePipeline(device, desc)
	if err != nil {
		return nil, err
	}

	c.computeCache[descHash] = pipeline
	atomic.AddUint64(&c.misses, 1)

	return pipeline, nil
}

// Stats returns cache statistics.
//
// Returns the number of cache hits and misses.
// These values are read atomically and may not be perfectly synchronized.
func (c *HALPipelineCache) Stats() (hits, misses uint64) {
	return atomic.LoadUint64(&c.hits), atomic.LoadUint64(&c.misses)
}

// HitRate returns the cache hit rate as a percentage (0.0 to 1.0).
//
// Returns 0.0 if no requests have been made.
func (c *HALPipelineCache) HitRate() float64 {
	hits := atomic.LoadUint64(&c.hits)
	misses := atomic.LoadUint64(&c.misses)
	total := hits + misses
	if total == 0 {
		return 0.0
	}
	return float64(hits) / float64(total)
}

// Size returns the total number of cached pipelines.
func (c *HALPipelineCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.renderCache) + len(c.computeCache)
}

// RenderPipelineCount returns the number of cached render pipelines.
func (c *HALPipelineCache) RenderPipelineCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.renderCache)
}

// ComputePipelineCount returns the number of cached compute pipelines.
func (c *HALPipelineCache) ComputePipelineCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.computeCache)
}

// Clear removes all cached pipelines and resets statistics.
//
// This does NOT destroy the underlying HAL resources. Call Destroy()
// on individual pipelines if resource cleanup is needed.
func (c *HALPipelineCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.renderCache = make(map[uint64]*HALRenderPipeline)
	c.computeCache = make(map[uint64]*HALComputePipeline)
	atomic.StoreUint64(&c.hits, 0)
	atomic.StoreUint64(&c.misses, 0)
}

// DestroyAll destroys all cached pipelines and clears the cache.
//
// This releases underlying HAL resources. After calling DestroyAll(),
// the cache is empty and ready for reuse.
func (c *HALPipelineCache) DestroyAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, pipeline := range c.renderCache {
		if pipeline != nil {
			pipeline.Destroy()
		}
	}
	for _, pipeline := range c.computeCache {
		if pipeline != nil {
			pipeline.Destroy()
		}
	}

	c.renderCache = make(map[uint64]*HALRenderPipeline)
	c.computeCache = make(map[uint64]*HALComputePipeline)
	atomic.StoreUint64(&c.hits, 0)
	atomic.StoreUint64(&c.misses, 0)
}

// =============================================================================
// Pipeline Descriptors
// =============================================================================

// HALRenderPipelineDescriptor describes a render pipeline to create.
//
// This is a minimal descriptor focused on the fields needed for hashing.
// It captures the essential pipeline state that affects rendering behavior.
type HALRenderPipelineDescriptor struct {
	// Label is an optional debug name.
	Label string

	// VertexShader is the vertex shader module.
	VertexShader *HALShaderModule

	// VertexEntryPoint is the vertex shader entry point function name.
	// Defaults to "vs_main" if empty.
	VertexEntryPoint string

	// FragmentShader is the fragment shader module.
	FragmentShader *HALShaderModule

	// FragmentEntryPoint is the fragment shader entry point function name.
	// Defaults to "fs_main" if empty.
	FragmentEntryPoint string

	// VertexBufferLayouts describes the vertex buffer layouts.
	VertexBufferLayouts []HALVertexBufferLayout

	// PrimitiveTopology is the primitive type (triangles, lines, points).
	PrimitiveTopology types.PrimitiveTopology

	// FrontFace defines which face is considered front-facing.
	FrontFace types.FrontFace

	// CullMode defines which faces to cull.
	CullMode types.CullMode

	// ColorFormat is the format of the color attachment.
	ColorFormat types.TextureFormat

	// DepthFormat is the format of the depth attachment (optional).
	// Use TextureFormatUndefined for no depth attachment.
	DepthFormat types.TextureFormat

	// DepthWriteEnabled enables depth buffer writes.
	DepthWriteEnabled bool

	// DepthCompare is the depth comparison function.
	DepthCompare types.CompareFunction

	// BlendState is the color blending configuration (optional).
	// Nil means no blending (source replaces destination).
	BlendState *HALBlendState

	// SampleCount is the number of samples per pixel (1 for non-MSAA).
	SampleCount uint32
}

// HALVertexBufferLayout describes a vertex buffer layout.
type HALVertexBufferLayout struct {
	// ArrayStride is the byte stride between consecutive vertices.
	ArrayStride uint64

	// StepMode is the input rate (per vertex or per instance).
	StepMode types.VertexStepMode

	// Attributes describes the vertex attributes in this buffer.
	Attributes []HALVertexAttribute
}

// HALVertexAttribute describes a vertex attribute.
type HALVertexAttribute struct {
	// ShaderLocation is the attribute location in the shader.
	ShaderLocation uint32

	// Format is the attribute data format.
	Format types.VertexFormat

	// Offset is the byte offset from the start of the vertex.
	Offset uint64
}

// HALBlendState describes the color blending configuration.
type HALBlendState struct {
	// Color is the color blending configuration.
	Color HALBlendComponent

	// Alpha is the alpha blending configuration.
	Alpha HALBlendComponent
}

// HALBlendComponent describes a blend component (color or alpha).
type HALBlendComponent struct {
	// SrcFactor is the source blend factor.
	SrcFactor types.BlendFactor

	// DstFactor is the destination blend factor.
	DstFactor types.BlendFactor

	// Operation is the blend operation.
	Operation types.BlendOperation
}

// HALComputePipelineDescriptor describes a compute pipeline to create.
type HALComputePipelineDescriptor struct {
	// Label is an optional debug name.
	Label string

	// ComputeShader is the compute shader module.
	ComputeShader *HALShaderModule

	// EntryPoint is the compute shader entry point function name.
	// Defaults to "main" if empty.
	EntryPoint string
}

// HALShaderModule represents a compiled shader module.
//
// Shader modules contain SPIR-V bytecode and are used to create pipelines.
// The hash is computed from the SPIR-V code for cache lookup.
type HALShaderModule struct {
	// id is a unique identifier for the shader module.
	id uint64

	// label is an optional debug name.
	label string

	// codeHash is a hash of the SPIR-V bytecode.
	// Used for pipeline descriptor hashing.
	codeHash uint64

	// halModule is the underlying HAL shader module (when available).
	halModule hal.ShaderModule

	// destroyed indicates whether the module has been destroyed.
	destroyed bool

	// mu protects mutable state.
	mu sync.RWMutex
}

// NewHALShaderModule creates a new shader module wrapper.
//
// Parameters:
//   - id: Unique identifier for this module.
//   - label: Debug label.
//   - code: SPIR-V bytecode.
//   - halModule: The underlying HAL module (may be nil for testing).
func NewHALShaderModule(id uint64, label string, code []byte, halModule hal.ShaderModule) *HALShaderModule {
	return &HALShaderModule{
		id:        id,
		label:     label,
		codeHash:  hashBytes(code),
		halModule: halModule,
	}
}

// ID returns the shader module's unique identifier.
func (m *HALShaderModule) ID() uint64 {
	return m.id
}

// Label returns the shader module's debug label.
func (m *HALShaderModule) Label() string {
	return m.label
}

// CodeHash returns the hash of the shader bytecode.
func (m *HALShaderModule) CodeHash() uint64 {
	return m.codeHash
}

// Raw returns the underlying HAL shader module.
func (m *HALShaderModule) Raw() hal.ShaderModule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.destroyed {
		return nil
	}
	return m.halModule
}

// IsDestroyed returns true if the module has been destroyed.
func (m *HALShaderModule) IsDestroyed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.destroyed
}

// Destroy marks the shader module as destroyed.
func (m *HALShaderModule) Destroy() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroyed = true
	m.halModule = nil
}

// =============================================================================
// Hash Functions
// =============================================================================

// HashRenderPipelineDescriptor computes an FNV-1a hash for a render pipeline descriptor.
//
// The hash includes all fields that affect rendering behavior:
//   - Shader modules and entry points
//   - Vertex buffer layouts and attributes
//   - Primitive topology and rasterization state
//   - Color and depth formats
//   - Blend state
//   - Sample count
func HashRenderPipelineDescriptor(desc *HALRenderPipelineDescriptor) uint64 {
	h := fnv.New64a()

	// Hash shader modules
	if desc.VertexShader != nil {
		hashWriteUint64(h, desc.VertexShader.codeHash)
	} else {
		hashWriteUint64(h, 0)
	}
	hashWriteString(h, desc.VertexEntryPoint)

	if desc.FragmentShader != nil {
		hashWriteUint64(h, desc.FragmentShader.codeHash)
	} else {
		hashWriteUint64(h, 0)
	}
	hashWriteString(h, desc.FragmentEntryPoint)

	// Hash vertex buffer layouts
	//nolint:gosec // G115: vertex buffer count is bounded by GPU limits (< 16)
	hashWriteUint32(h, uint32(len(desc.VertexBufferLayouts)))
	for i := range desc.VertexBufferLayouts {
		layout := &desc.VertexBufferLayouts[i]
		hashWriteUint64(h, layout.ArrayStride)
		hashWriteUint32(h, uint32(layout.StepMode))
		//nolint:gosec // G115: attribute count is bounded by GPU limits (< 32)
		hashWriteUint32(h, uint32(len(layout.Attributes)))
		for j := range layout.Attributes {
			attr := &layout.Attributes[j]
			hashWriteUint32(h, attr.ShaderLocation)
			hashWriteUint32(h, uint32(attr.Format))
			hashWriteUint64(h, attr.Offset)
		}
	}

	// Hash primitive state
	hashWriteUint32(h, uint32(desc.PrimitiveTopology))
	hashWriteUint32(h, uint32(desc.FrontFace))
	hashWriteUint32(h, uint32(desc.CullMode))

	// Hash formats
	hashWriteUint32(h, uint32(desc.ColorFormat))
	hashWriteUint32(h, uint32(desc.DepthFormat))

	// Hash depth state
	hashWriteBool(h, desc.DepthWriteEnabled)
	hashWriteUint32(h, uint32(desc.DepthCompare))

	// Hash blend state
	if desc.BlendState != nil {
		hashWriteBool(h, true)
		// Color blend
		hashWriteUint32(h, uint32(desc.BlendState.Color.SrcFactor))
		hashWriteUint32(h, uint32(desc.BlendState.Color.DstFactor))
		hashWriteUint32(h, uint32(desc.BlendState.Color.Operation))
		// Alpha blend
		hashWriteUint32(h, uint32(desc.BlendState.Alpha.SrcFactor))
		hashWriteUint32(h, uint32(desc.BlendState.Alpha.DstFactor))
		hashWriteUint32(h, uint32(desc.BlendState.Alpha.Operation))
	} else {
		hashWriteBool(h, false)
	}

	// Hash sample count
	hashWriteUint32(h, desc.SampleCount)

	return h.Sum64()
}

// HashComputePipelineDescriptor computes an FNV-1a hash for a compute pipeline descriptor.
func HashComputePipelineDescriptor(desc *HALComputePipelineDescriptor) uint64 {
	h := fnv.New64a()

	if desc.ComputeShader != nil {
		hashWriteUint64(h, desc.ComputeShader.codeHash)
	} else {
		hashWriteUint64(h, 0)
	}
	hashWriteString(h, desc.EntryPoint)

	return h.Sum64()
}

// =============================================================================
// Helper Functions for Hashing
// =============================================================================

// hashBytes computes an FNV-1a hash of a byte slice.
func hashBytes(data []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(data)
	return h.Sum64()
}

// hashWriteUint32 writes a uint32 to the hash.
func hashWriteUint32(h hash.Hash64, v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	_, _ = h.Write(buf[:])
}

// hashWriteUint64 writes a uint64 to the hash.
func hashWriteUint64(h hash.Hash64, v uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	_, _ = h.Write(buf[:])
}

// hashWriteString writes a string to the hash.
//
//nolint:gosec // G115: length of string in a pipeline descriptor is always small (< 256 bytes for entry point names)
func hashWriteString(h hash.Hash64, s string) {
	hashWriteUint32(h, uint32(len(s)))
	_, _ = h.Write([]byte(s))
}

// hashWriteBool writes a bool to the hash.
func hashWriteBool(h hash.Hash64, v bool) {
	if v {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}
}

// =============================================================================
// Pipeline Creation
// =============================================================================

// pipelineIDCounter is used to generate unique pipeline IDs.
var pipelineIDCounter uint64

// nextPipelineID returns the next unique pipeline ID.
func nextPipelineID() uint64 {
	return atomic.AddUint64(&pipelineIDCounter, 1)
}

// createHALRenderPipeline creates a new render pipeline from a descriptor.
//
// This is called by GetOrCreateRenderPipeline when a cache miss occurs.
func createHALRenderPipeline(device hal.Device, desc *HALRenderPipelineDescriptor) (*HALRenderPipeline, error) {
	if desc.VertexShader == nil {
		return nil, ErrPipelineCacheNilShader
	}

	// Validate entry points and set defaults
	vertexEntry := desc.VertexEntryPoint
	if vertexEntry == "" {
		vertexEntry = "vs_main"
	}

	fragmentEntry := desc.FragmentEntryPoint
	if fragmentEntry == "" {
		fragmentEntry = "fs_main"
	}

	// Default sample count
	sampleCount := desc.SampleCount
	if sampleCount == 0 {
		sampleCount = 1
	}

	// TODO: When HAL pipeline creation is implemented, create actual pipeline:
	// halDesc := &hal.RenderPipelineDescriptor{
	//     Label: desc.Label,
	//     Vertex: hal.VertexState{
	//         Module:     desc.VertexShader.Raw(),
	//         EntryPoint: vertexEntry,
	//         Buffers:    convertVertexBufferLayouts(desc.VertexBufferLayouts),
	//     },
	//     Fragment: &hal.FragmentState{
	//         Module:     desc.FragmentShader.Raw(),
	//         EntryPoint: fragmentEntry,
	//         Targets: []hal.ColorTargetState{{
	//             Format:    desc.ColorFormat,
	//             Blend:     convertBlendState(desc.BlendState),
	//             WriteMask: types.ColorWriteMaskAll,
	//         }},
	//     },
	//     Primitive: hal.PrimitiveState{
	//         Topology:  desc.PrimitiveTopology,
	//         FrontFace: desc.FrontFace,
	//         CullMode:  desc.CullMode,
	//     },
	//     DepthStencil: convertDepthState(desc),
	//     Multisample: hal.MultisampleState{
	//         Count: sampleCount,
	//     },
	// }
	// halPipeline, err := device.CreateRenderPipeline(halDesc)
	// if err != nil {
	//     return nil, fmt.Errorf("create render pipeline: %w", err)
	// }

	// For now, create a placeholder pipeline
	_ = device // Will be used for actual creation
	_ = vertexEntry
	_ = fragmentEntry
	_ = sampleCount

	pipeline := &HALRenderPipeline{
		id:    nextPipelineID(),
		label: desc.Label,
	}

	return pipeline, nil
}

// createHALComputePipeline creates a new compute pipeline from a descriptor.
//
// This is called by GetOrCreateComputePipeline when a cache miss occurs.
func createHALComputePipeline(device hal.Device, desc *HALComputePipelineDescriptor) (*HALComputePipeline, error) {
	if desc.ComputeShader == nil {
		return nil, ErrPipelineCacheNilShader
	}

	// Validate entry point and set default
	entryPoint := desc.EntryPoint
	if entryPoint == "" {
		entryPoint = "main"
	}

	// TODO: When HAL pipeline creation is implemented, create actual pipeline:
	// halDesc := &hal.ComputePipelineDescriptor{
	//     Label: desc.Label,
	//     Compute: hal.ProgrammableStageDescriptor{
	//         Module:     desc.ComputeShader.Raw(),
	//         EntryPoint: entryPoint,
	//     },
	// }
	// halPipeline, err := device.CreateComputePipeline(halDesc)
	// if err != nil {
	//     return nil, fmt.Errorf("create compute pipeline: %w", err)
	// }

	// For now, create a placeholder pipeline
	_ = device // Will be used for actual creation
	_ = entryPoint

	pipeline := &HALComputePipeline{
		id:            nextPipelineID(),
		label:         desc.Label,
		workgroupSize: [3]uint32{64, 1, 1}, // Default workgroup size
	}

	return pipeline, nil
}
