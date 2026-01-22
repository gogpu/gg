// Package native provides a GPU-accelerated rendering backend using gogpu/wgpu.
package native

import (
	"errors"
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/types"
)

// HAL render pass errors.
var (
	// ErrPassEnded is returned when operations are called on an ended pass.
	ErrPassEnded = errors.New("native: render pass has already ended")

	// ErrPassNotRecording is returned when operations are called on a pass that is not recording.
	ErrPassNotRecording = errors.New("native: render pass is not recording")

	// ErrNilPipeline is returned when SetPipeline is called with nil.
	ErrNilPipeline = errors.New("native: pipeline is nil")

	// ErrNilBindGroup is returned when SetBindGroup is called with nil.
	ErrNilBindGroup = errors.New("native: bind group is nil")

	// ErrBindGroupIndexOutOfRange is returned when bind group index exceeds maximum.
	ErrBindGroupIndexOutOfRange = errors.New("native: bind group index exceeds maximum (3)")

	// ErrNilVertexBuffer is returned when SetVertexBuffer is called with nil.
	ErrNilVertexBuffer = errors.New("native: vertex buffer is nil")

	// ErrNilIndexBuffer is returned when SetIndexBuffer is called with nil.
	ErrNilIndexBuffer = errors.New("native: index buffer is nil")

	// ErrNilIndirectBuffer is returned when indirect draw is called with nil buffer.
	ErrNilIndirectBuffer = errors.New("native: indirect buffer is nil")

	// ErrIndirectOffsetNotAligned is returned when indirect offset is not 4-byte aligned.
	ErrIndirectOffsetNotAligned = errors.New("native: indirect offset must be 4-byte aligned")
)

// HALRenderPassState represents the state of a render pass encoder.
type HALRenderPassState int

const (
	// HALRenderPassStateRecording means the pass is actively recording commands.
	HALRenderPassStateRecording HALRenderPassState = iota

	// HALRenderPassStateEnded means the pass has been ended.
	HALRenderPassStateEnded
)

// String returns the string representation of HALRenderPassState.
func (s HALRenderPassState) String() string {
	switch s {
	case HALRenderPassStateRecording:
		return "Recording"
	case HALRenderPassStateEnded:
		return "Ended"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// HALRenderPassEncoder records render commands within a render pass.
//
// HALRenderPassEncoder wraps core.CoreRenderPassEncoder and provides
// Go-idiomatic access with immediate error returns. Commands recorded include:
//   - SetPipeline: Set the render pipeline for subsequent draw calls
//   - SetBindGroup: Bind resource groups (textures, buffers, samplers)
//   - SetVertexBuffer: Bind vertex buffers to slots
//   - SetIndexBuffer: Bind the index buffer for indexed draws
//   - SetViewport: Set the viewport transformation
//   - SetScissorRect: Set the scissor rectangle for clipping
//   - SetBlendConstant: Set the blend constant color
//   - SetStencilReference: Set the stencil reference value
//   - Draw: Draw primitives
//   - DrawIndexed: Draw indexed primitives
//   - DrawIndirect: Draw with GPU-generated parameters
//   - DrawIndexedIndirect: Draw indexed with GPU-generated parameters
//
// Thread Safety:
// HALRenderPassEncoder is NOT safe for concurrent use. All commands must be
// recorded from a single goroutine. The pass must be ended with End() before
// the parent command encoder can continue recording.
//
// Lifecycle:
//  1. Created by HALCommandEncoder.BeginRenderPass()
//  2. Record commands (SetPipeline, SetVertexBuffer, Draw, etc.)
//  3. Call End() to complete the pass
//
// State Machine:
//
//	Recording -> End() -> Ended
type HALRenderPassEncoder struct {
	// mu protects mutable state.
	mu sync.Mutex

	// corePass is the underlying core render pass encoder.
	// This provides the actual HAL integration.
	corePass *core.CoreRenderPassEncoder

	// encoder is the parent command encoder.
	encoder *HALCommandEncoder

	// state is the current pass state.
	state HALRenderPassState

	// currentPipeline tracks the currently bound pipeline (if any).
	currentPipeline *HALRenderPipeline

	// vertexBufferCount tracks the number of vertex buffer slots used.
	vertexBufferCount uint32

	// hasIndexBuffer tracks whether an index buffer is bound.
	hasIndexBuffer bool
}

// State returns the current pass state.
func (p *HALRenderPassEncoder) State() HALRenderPassState {
	if p == nil {
		return HALRenderPassStateEnded
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// IsEnded returns true if the pass has been ended.
func (p *HALRenderPassEncoder) IsEnded() bool {
	return p.State() == HALRenderPassStateEnded
}

// checkRecording returns an error if the pass is not in Recording state.
// The caller must hold p.mu.
func (p *HALRenderPassEncoder) checkRecording() error {
	if p.state != HALRenderPassStateRecording {
		return ErrPassEnded
	}
	return nil
}

// SetPipeline binds a render pipeline for subsequent draw calls.
//
// The pipeline defines:
//   - Vertex and fragment shaders
//   - Primitive topology (triangles, lines, points)
//   - Rasterization state (culling, depth bias)
//   - Depth/stencil state
//   - Blend state for color attachments
//
// Parameters:
//   - pipeline: The render pipeline to bind.
//
// Returns nil on success.
// Returns an error if:
//   - The pass has ended
//   - The pipeline is nil
func (p *HALRenderPassEncoder) SetPipeline(pipeline *HALRenderPipeline) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("set pipeline: %w", err)
	}

	if pipeline == nil {
		return ErrNilPipeline
	}

	p.currentPipeline = pipeline

	// Forward to core pass if available
	// Note: core.CoreRenderPassEncoder.SetPipeline takes *core.RenderPipeline
	// HAL integration pending for core.RenderPipeline
	// For now, we record the state locally
	_ = p.corePass // Silence linter until HAL integration

	return nil
}

// SetBindGroup binds a bind group for the given index.
//
// Bind groups provide resources (buffers, textures, samplers) to shaders.
// WebGPU supports up to 4 bind groups (indices 0-3).
//
// Parameters:
//   - index: The bind group index (0, 1, 2, or 3).
//   - bindGroup: The bind group to bind.
//   - dynamicOffsets: Dynamic offsets for dynamic uniform/storage buffers (optional).
//
// Returns nil on success.
// Returns an error if:
//   - The pass has ended
//   - The index exceeds maximum (3)
//   - The bind group is nil
func (p *HALRenderPassEncoder) SetBindGroup(index uint32, bindGroup *HALBindGroup, dynamicOffsets []uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("set bind group: %w", err)
	}

	// WebGPU spec: max 4 bind groups (0-3)
	if index > 3 {
		return fmt.Errorf("%w: index %d", ErrBindGroupIndexOutOfRange, index)
	}

	if bindGroup == nil {
		return ErrNilBindGroup
	}

	// Forward to core pass if available
	// Note: core.CoreRenderPassEncoder does not have SetBindGroup yet
	// HAL integration pending
	_ = p.corePass // Silence linter until HAL integration

	return nil
}

// SetVertexBuffer binds a vertex buffer to a slot.
//
// Vertex buffers provide per-vertex data to vertex shaders. Multiple vertex
// buffers can be bound to different slots for interleaved or separate vertex
// attributes.
//
// Parameters:
//   - slot: The vertex buffer slot (0 to maxVertexBuffers-1).
//   - buffer: The buffer to bind.
//   - offset: Byte offset into the buffer.
//   - size: Size of the vertex data in bytes. Use 0 to bind the remaining buffer.
//
// Returns nil on success.
// Returns an error if:
//   - The pass has ended
//   - The buffer is nil
func (p *HALRenderPassEncoder) SetVertexBuffer(slot uint32, buffer *HALBuffer, offset, size uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("set vertex buffer: %w", err)
	}

	if buffer == nil {
		return ErrNilVertexBuffer
	}

	// Track vertex buffer count
	if slot >= p.vertexBufferCount {
		p.vertexBufferCount = slot + 1
	}

	// Forward to core pass if available
	// core.CoreRenderPassEncoder.SetVertexBuffer takes *core.Buffer
	// We need to convert HALBuffer to core.Buffer
	// For now, this is a no-op until HAL buffer integration is complete
	_ = p.corePass // Silence linter until HAL integration

	return nil
}

// SetIndexBuffer binds the index buffer for indexed draw calls.
//
// The index buffer provides vertex indices for indexed drawing. Only one
// index buffer can be bound at a time.
//
// Parameters:
//   - buffer: The buffer containing indices.
//   - format: The index format (Uint16 or Uint32).
//   - offset: Byte offset into the buffer.
//   - size: Size of the index data in bytes. Use 0 to bind the remaining buffer.
//
// Returns nil on success.
// Returns an error if:
//   - The pass has ended
//   - The buffer is nil
func (p *HALRenderPassEncoder) SetIndexBuffer(buffer *HALBuffer, format IndexFormat, offset, size uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("set index buffer: %w", err)
	}

	if buffer == nil {
		return ErrNilIndexBuffer
	}

	p.hasIndexBuffer = true

	// Forward to core pass if available
	// core.CoreRenderPassEncoder.SetIndexBuffer takes *core.Buffer
	// Convert format and forward
	// For now, this is a no-op until HAL buffer integration is complete
	_ = p.corePass // Silence linter until HAL integration

	return nil
}

// SetViewport sets the viewport transformation.
//
// The viewport defines how normalized device coordinates (-1 to 1) are
// transformed to framebuffer coordinates. The depth range is clamped to [0, 1].
//
// Parameters:
//   - x, y: The viewport origin (pixels).
//   - width, height: The viewport size (pixels).
//   - minDepth, maxDepth: The depth range [0, 1].
//
// Returns nil on success.
// Returns an error if the pass has ended.
func (p *HALRenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("set viewport: %w", err)
	}

	// Forward to core pass if available
	if p.corePass != nil {
		p.corePass.SetViewport(x, y, width, height, minDepth, maxDepth)
	}

	return nil
}

// SetScissorRect sets the scissor rectangle for clipping.
//
// Fragments outside the scissor rectangle are discarded. By default,
// the scissor rectangle matches the framebuffer size.
//
// Parameters:
//   - x, y: The scissor origin (pixels).
//   - width, height: The scissor size (pixels).
//
// Returns nil on success.
// Returns an error if the pass has ended.
func (p *HALRenderPassEncoder) SetScissorRect(x, y, width, height uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("set scissor rect: %w", err)
	}

	// Forward to core pass if available
	if p.corePass != nil {
		p.corePass.SetScissorRect(x, y, width, height)
	}

	return nil
}

// SetBlendConstant sets the blend constant color.
//
// The blend constant is used in blend operations when the blend factor
// is set to Constant or OneMinusConstant.
//
// Parameters:
//   - color: The blend constant color.
//
// Returns nil on success.
// Returns an error if the pass has ended.
func (p *HALRenderPassEncoder) SetBlendConstant(color types.Color) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("set blend constant: %w", err)
	}

	// Forward to core pass if available
	if p.corePass != nil {
		p.corePass.SetBlendConstant(&color)
	}

	return nil
}

// SetStencilReference sets the stencil reference value.
//
// The stencil reference is used in stencil comparison and update operations.
//
// Parameters:
//   - reference: The stencil reference value.
//
// Returns nil on success.
// Returns an error if the pass has ended.
func (p *HALRenderPassEncoder) SetStencilReference(reference uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("set stencil reference: %w", err)
	}

	// Forward to core pass if available
	if p.corePass != nil {
		p.corePass.SetStencilReference(reference)
	}

	return nil
}

// Draw issues a non-indexed draw call.
//
// This draws primitives using vertices sequentially from bound vertex buffers.
// A pipeline must be bound before calling Draw.
//
// Parameters:
//   - vertexCount: The number of vertices to draw.
//   - instanceCount: The number of instances to draw.
//   - firstVertex: The first vertex index.
//   - firstInstance: The first instance index.
//
// Returns nil on success.
// Returns an error if the pass has ended.
func (p *HALRenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("draw: %w", err)
	}

	// Forward to core pass if available
	if p.corePass != nil {
		p.corePass.Draw(vertexCount, instanceCount, firstVertex, firstInstance)
	}

	return nil
}

// DrawIndexed issues an indexed draw call.
//
// This draws primitives using indices from the bound index buffer.
// Both a pipeline and index buffer must be bound before calling DrawIndexed.
//
// Parameters:
//   - indexCount: The number of indices to draw.
//   - instanceCount: The number of instances to draw.
//   - firstIndex: The first index in the index buffer.
//   - baseVertex: Value added to each index before vertex lookup.
//   - firstInstance: The first instance index.
//
// Returns nil on success.
// Returns an error if the pass has ended.
func (p *HALRenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("draw indexed: %w", err)
	}

	// Forward to core pass if available
	if p.corePass != nil {
		p.corePass.DrawIndexed(indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
	}

	return nil
}

// DrawIndirect issues a draw call with GPU-generated parameters.
//
// The draw parameters are read from the indirect buffer at the specified offset.
// The buffer must contain a DrawIndirectArgs structure:
//
//	struct DrawIndirectArgs {
//	    vertexCount: u32,
//	    instanceCount: u32,
//	    firstVertex: u32,
//	    firstInstance: u32,
//	}
//
// Parameters:
//   - indirectBuffer: Buffer containing DrawIndirectArgs.
//   - indirectOffset: Byte offset into the buffer (must be 4-byte aligned).
//
// Returns nil on success.
// Returns an error if:
//   - The pass has ended
//   - The buffer is nil
//   - The offset is not 4-byte aligned
func (p *HALRenderPassEncoder) DrawIndirect(indirectBuffer *HALBuffer, indirectOffset uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("draw indirect: %w", err)
	}

	if indirectBuffer == nil {
		return ErrNilIndirectBuffer
	}

	if indirectOffset%4 != 0 {
		return fmt.Errorf("%w: offset %d", ErrIndirectOffsetNotAligned, indirectOffset)
	}

	// Forward to core pass if available
	// core.CoreRenderPassEncoder.DrawIndirect takes *core.Buffer
	// For now, this is a no-op until HAL buffer integration is complete
	_ = p.corePass // Silence linter until HAL integration

	return nil
}

// DrawIndexedIndirect issues an indexed draw call with GPU-generated parameters.
//
// The draw parameters are read from the indirect buffer at the specified offset.
// The buffer must contain a DrawIndexedIndirectArgs structure:
//
//	struct DrawIndexedIndirectArgs {
//	    indexCount: u32,
//	    instanceCount: u32,
//	    firstIndex: u32,
//	    baseVertex: i32,
//	    firstInstance: u32,
//	}
//
// Parameters:
//   - indirectBuffer: Buffer containing DrawIndexedIndirectArgs.
//   - indirectOffset: Byte offset into the buffer (must be 4-byte aligned).
//
// Returns nil on success.
// Returns an error if:
//   - The pass has ended
//   - The buffer is nil
//   - The offset is not 4-byte aligned
func (p *HALRenderPassEncoder) DrawIndexedIndirect(indirectBuffer *HALBuffer, indirectOffset uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("draw indexed indirect: %w", err)
	}

	if indirectBuffer == nil {
		return ErrNilIndirectBuffer
	}

	if indirectOffset%4 != 0 {
		return fmt.Errorf("%w: offset %d", ErrIndirectOffsetNotAligned, indirectOffset)
	}

	// Forward to core pass if available
	// core.CoreRenderPassEncoder.DrawIndexedIndirect takes *core.Buffer
	// For now, this is a no-op until HAL buffer integration is complete
	_ = p.corePass // Silence linter until HAL integration

	return nil
}

// End completes the render pass.
//
// After calling End(), the render pass encoder cannot be used for further
// recording. The parent command encoder returns to the Recording state.
//
// Returns nil on success.
// Returns an error if the pass has already ended.
func (p *HALRenderPassEncoder) End() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == HALRenderPassStateEnded {
		return nil // Idempotent
	}
	p.state = HALRenderPassStateEnded

	// End the core pass if available
	if p.corePass != nil {
		if err := p.corePass.End(); err != nil {
			return fmt.Errorf("end render pass: %w", err)
		}
	}

	// Notify the parent encoder
	if p.encoder != nil {
		return p.encoder.endRenderPass(p)
	}

	return nil
}

// =============================================================================
// Supporting Types for Render Pass
// =============================================================================

// HALRenderPipeline represents a GPU render pipeline.
//
// Render pipelines define the complete rendering state including:
//   - Vertex and fragment shaders
//   - Primitive topology
//   - Rasterization state
//   - Depth/stencil state
//   - Blend state
//
// HALRenderPipeline is a placeholder type that will be expanded when
// pipeline creation is implemented.
type HALRenderPipeline struct {
	// id is a unique identifier for the pipeline.
	id uint64

	// label is an optional debug name.
	label string

	// destroyed indicates whether the pipeline has been destroyed.
	destroyed bool

	// mu protects mutable state.
	mu sync.RWMutex
}

// ID returns the pipeline's unique identifier.
func (p *HALRenderPipeline) ID() uint64 {
	return p.id
}

// Label returns the pipeline's debug label.
func (p *HALRenderPipeline) Label() string {
	return p.label
}

// IsDestroyed returns true if the pipeline has been destroyed.
func (p *HALRenderPipeline) IsDestroyed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.destroyed
}

// Destroy releases the pipeline resources.
func (p *HALRenderPipeline) Destroy() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.destroyed = true
}

// HALBindGroup represents a collection of resources bound together.
//
// Bind groups contain:
//   - Uniform buffers
//   - Storage buffers
//   - Texture bindings
//   - Sampler bindings
//
// HALBindGroup is a placeholder type that will be expanded when
// bind group creation is implemented.
type HALBindGroup struct {
	// id is a unique identifier for the bind group.
	id uint64

	// label is an optional debug name.
	label string

	// destroyed indicates whether the bind group has been destroyed.
	destroyed bool

	// mu protects mutable state.
	mu sync.RWMutex
}

// ID returns the bind group's unique identifier.
func (bg *HALBindGroup) ID() uint64 {
	return bg.id
}

// Label returns the bind group's debug label.
func (bg *HALBindGroup) Label() string {
	return bg.label
}

// IsDestroyed returns true if the bind group has been destroyed.
func (bg *HALBindGroup) IsDestroyed() bool {
	bg.mu.RLock()
	defer bg.mu.RUnlock()
	return bg.destroyed
}

// Destroy releases the bind group resources.
func (bg *HALBindGroup) Destroy() {
	bg.mu.Lock()
	defer bg.mu.Unlock()
	bg.destroyed = true
}

// Note: IndexFormat is defined in commands.go with IndexFormatUint16 and IndexFormatUint32.
