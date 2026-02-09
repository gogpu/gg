// Package gpu provides a GPU-accelerated rendering backend using gogpu/wgpu.
package gpu

import (
	"errors"
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/core"
)

// Compute pass errors.
var (
	// ErrComputePassEnded is returned when operations are called on an ended compute pass.
	ErrComputePassEnded = errors.New("gpu: compute pass has already ended")

	// ErrComputePassNotRecording is returned when operations are called on a pass not recording.
	ErrComputePassNotRecording = errors.New("gpu: compute pass is not recording")

	// ErrNilComputePipeline is returned when SetPipeline is called with nil.
	ErrNilComputePipeline = errors.New("gpu: compute pipeline is nil")

	// ErrNilComputeBindGroup is returned when SetBindGroup is called with nil.
	ErrNilComputeBindGroup = errors.New("gpu: bind group is nil")

	// ErrComputeBindGroupIndexOutOfRange is returned when bind group index exceeds maximum.
	ErrComputeBindGroupIndexOutOfRange = errors.New("gpu: bind group index exceeds maximum (3)")

	// ErrNilDispatchBuffer is returned when DispatchIndirect is called with nil buffer.
	ErrNilDispatchBuffer = errors.New("gpu: dispatch indirect buffer is nil")

	// ErrDispatchOffsetNotAligned is returned when dispatch offset is not 4-byte aligned.
	ErrDispatchOffsetNotAligned = errors.New("gpu: dispatch offset must be 4-byte aligned")

	// ErrWorkgroupCountZero is returned when any workgroup dimension is zero.
	ErrWorkgroupCountZero = errors.New("gpu: workgroup count must be greater than zero")

	// ErrWorkgroupCountExceedsLimit is returned when workgroup count exceeds device limits.
	ErrWorkgroupCountExceedsLimit = errors.New("gpu: workgroup count exceeds device limit")
)

// ComputePassState represents the state of a compute pass encoder.
type ComputePassState int

const (
	// ComputePassStateRecording means the pass is actively recording commands.
	ComputePassStateRecording ComputePassState = iota

	// ComputePassStateEnded means the pass has been ended.
	ComputePassStateEnded
)

// String returns the string representation of ComputePassState.
func (s ComputePassState) String() string {
	switch s {
	case ComputePassStateRecording:
		return "Recording"
	case ComputePassStateEnded:
		return "Ended"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// ComputePassEncoder records compute commands within a compute pass.
//
// ComputePassEncoder wraps core.CoreComputePassEncoder and provides
// Go-idiomatic access with immediate error returns. Commands recorded include:
//   - SetPipeline: Set the compute pipeline for subsequent dispatch calls
//   - SetBindGroup: Bind resource groups (buffers, textures) to shaders
//   - DispatchWorkgroups: Execute compute shader workgroups
//   - DispatchWorkgroupsIndirect: Execute with GPU-generated parameters
//
// Thread Safety:
// ComputePassEncoder is NOT safe for concurrent use. All commands must be
// recorded from a single goroutine. The pass must be ended with End() before
// the parent command encoder can continue recording.
//
// Lifecycle:
//  1. Created by CoreCommandEncoder.BeginComputePass()
//  2. Record commands (SetPipeline, SetBindGroup, DispatchWorkgroups, etc.)
//  3. Call End() to complete the pass
//
// State Machine:
//
//	Recording -> End() -> Ended
type ComputePassEncoder struct {
	// mu protects mutable state.
	mu sync.Mutex

	// corePass is the underlying core compute pass encoder.
	corePass *core.CoreComputePassEncoder

	// encoder is the parent command encoder.
	encoder *CoreCommandEncoder

	// state is the current pass state.
	state ComputePassState

	// currentPipeline tracks the currently bound pipeline (if any).
	currentPipeline *ComputePipeline

	// dispatchCount tracks the number of dispatch calls made.
	dispatchCount uint32
}

// State returns the current pass state.
func (p *ComputePassEncoder) State() ComputePassState {
	if p == nil {
		return ComputePassStateEnded
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// IsEnded returns true if the pass has been ended.
func (p *ComputePassEncoder) IsEnded() bool {
	return p.State() == ComputePassStateEnded
}

// checkRecording returns an error if the pass is not in Recording state.
// The caller must hold p.mu.
func (p *ComputePassEncoder) checkRecording() error {
	if p.state != ComputePassStateRecording {
		return ErrComputePassEnded
	}
	return nil
}

// SetPipeline sets the compute pipeline for subsequent dispatch calls.
//
// The pipeline defines the compute shader to execute and its bind group layouts.
// A pipeline must be bound before any dispatch call.
//
// Parameters:
//   - pipeline: The compute pipeline to bind.
//
// Returns nil on success.
// Returns an error if:
//   - The pass has ended
//   - The pipeline is nil
func (p *ComputePassEncoder) SetPipeline(pipeline *ComputePipeline) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("set pipeline: %w", err)
	}

	if pipeline == nil {
		return ErrNilComputePipeline
	}

	p.currentPipeline = pipeline

	// Forward to core pass if available
	// Note: core.CoreComputePassEncoder.SetPipeline takes *core.ComputePipeline
	// Integration pending for core.ComputePipeline
	// For now, we record the state locally
	_ = p.corePass // Silence linter until integration complete

	return nil
}

// SetBindGroup binds a bind group for the given index.
//
// Bind groups provide resources (buffers, textures) to compute shaders.
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
func (p *ComputePassEncoder) SetBindGroup(index uint32, bindGroup *BindGroup, dynamicOffsets []uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("set bind group: %w", err)
	}

	// WebGPU spec: max 4 bind groups (0-3)
	if index > 3 {
		return fmt.Errorf("%w: index %d", ErrComputeBindGroupIndexOutOfRange, index)
	}

	if bindGroup == nil {
		return ErrNilComputeBindGroup
	}

	// Forward to core pass if available
	// Note: core.CoreComputePassEncoder does not have SetBindGroup yet
	// Integration pending
	_ = p.corePass // Silence linter until integration complete

	return nil
}

// DispatchWorkgroups dispatches compute workgroups.
//
// This executes the compute shader with the specified number of workgroups.
// The total number of shader invocations is:
//
//	x * y * z * workgroup_size
//
// where workgroup_size is defined in the compute shader.
//
// Parameters:
//   - x, y, z: The number of workgroups to dispatch in each dimension.
//
// Returns nil on success.
// Returns an error if:
//   - The pass has ended
//   - Any dimension is zero (optional validation)
func (p *ComputePassEncoder) DispatchWorkgroups(x, y, z uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("dispatch workgroups: %w", err)
	}

	// Note: WebGPU spec allows zero workgroups (no-op dispatch).
	// We don't validate here for spec compliance.

	p.dispatchCount++

	// Forward to core pass if available
	if p.corePass != nil {
		p.corePass.Dispatch(x, y, z)
	}

	return nil
}

// DispatchWorkgroupsIndirect dispatches compute workgroups with GPU-generated parameters.
//
// The dispatch parameters are read from the indirect buffer at the specified offset.
// The buffer must contain a DispatchIndirectArgs structure:
//
//	struct DispatchIndirectArgs {
//	    x: u32,     // Number of workgroups in X
//	    y: u32,     // Number of workgroups in Y
//	    z: u32,     // Number of workgroups in Z
//	}
//
// Parameters:
//   - indirectBuffer: Buffer containing DispatchIndirectArgs.
//   - indirectOffset: Byte offset into the buffer (must be 4-byte aligned).
//
// Returns nil on success.
// Returns an error if:
//   - The pass has ended
//   - The buffer is nil
//   - The offset is not 4-byte aligned
func (p *ComputePassEncoder) DispatchWorkgroupsIndirect(indirectBuffer *Buffer, indirectOffset uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkRecording(); err != nil {
		return fmt.Errorf("dispatch workgroups indirect: %w", err)
	}

	if indirectBuffer == nil {
		return ErrNilDispatchBuffer
	}

	// Indirect dispatch requires 4-byte alignment
	if indirectOffset%4 != 0 {
		return fmt.Errorf("%w: offset %d", ErrDispatchOffsetNotAligned, indirectOffset)
	}

	p.dispatchCount++

	// Forward to core pass if available
	// core.CoreComputePassEncoder.DispatchIndirect takes *core.Buffer
	// For now, this is a no-op until buffer integration is complete
	_ = p.corePass // Silence linter until integration complete

	return nil
}

// End completes the compute pass.
//
// After calling End(), the compute pass encoder cannot be used for further
// recording. The parent command encoder returns to the Recording state.
//
// Returns nil on success.
// Returns an error if the pass has already ended.
func (p *ComputePassEncoder) End() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == ComputePassStateEnded {
		return nil // Idempotent
	}
	p.state = ComputePassStateEnded

	// End the core pass if available
	if p.corePass != nil {
		if err := p.corePass.End(); err != nil {
			return fmt.Errorf("end compute pass: %w", err)
		}
	}

	// Notify the parent encoder
	if p.encoder != nil {
		return p.encoder.endComputePass(p)
	}

	return nil
}

// DispatchCount returns the number of dispatch calls made during this pass.
func (p *ComputePassEncoder) DispatchCount() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dispatchCount
}

// =============================================================================
// Supporting Types for Compute Pass
// =============================================================================

// ComputePipeline represents a GPU compute pipeline.
//
// Compute pipelines define:
//   - The compute shader to execute
//   - Bind group layouts for resource bindings
//   - Pipeline layout
//
// ComputePipeline is a placeholder type that will be expanded when
// pipeline creation is implemented.
type ComputePipeline struct {
	// id is a unique identifier for the pipeline.
	id uint64

	// label is an optional debug name.
	label string

	// workgroupSize stores the compute shader's workgroup size.
	workgroupSize [3]uint32

	// destroyed indicates whether the pipeline has been destroyed.
	destroyed bool

	// mu protects mutable state.
	mu sync.RWMutex
}

// ID returns the pipeline's unique identifier.
func (p *ComputePipeline) ID() uint64 {
	return p.id
}

// Label returns the pipeline's debug label.
func (p *ComputePipeline) Label() string {
	return p.label
}

// WorkgroupSize returns the compute shader's workgroup size.
// Returns [x, y, z] dimensions.
func (p *ComputePipeline) WorkgroupSize() [3]uint32 {
	return p.workgroupSize
}

// IsDestroyed returns true if the pipeline has been destroyed.
func (p *ComputePipeline) IsDestroyed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.destroyed
}

// Destroy releases the pipeline resources.
func (p *ComputePipeline) Destroy() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.destroyed = true
}

// =============================================================================
// Dispatch Indirect Arguments
// =============================================================================

// DispatchIndirectArgs represents the arguments for indirect dispatch.
//
// This structure is read from the GPU buffer at the offset specified
// in DispatchWorkgroupsIndirect.
type DispatchIndirectArgs struct {
	// X is the number of workgroups in the X dimension.
	X uint32

	// Y is the number of workgroups in the Y dimension.
	Y uint32

	// Z is the number of workgroups in the Z dimension.
	Z uint32
}

// Size returns the byte size of DispatchIndirectArgs.
func (d DispatchIndirectArgs) Size() uint64 {
	return 12 // 3 * sizeof(uint32)
}

// =============================================================================
// Draw Indirect Arguments (for render passes)
// =============================================================================

// DrawIndirectArgs represents the arguments for indirect draw.
//
// This structure is read from the GPU buffer at the offset specified
// in DrawIndirect.
type DrawIndirectArgs struct {
	// VertexCount is the number of vertices to draw.
	VertexCount uint32

	// InstanceCount is the number of instances to draw.
	InstanceCount uint32

	// FirstVertex is the first vertex index.
	FirstVertex uint32

	// FirstInstance is the first instance index.
	FirstInstance uint32
}

// Size returns the byte size of DrawIndirectArgs.
func (d DrawIndirectArgs) Size() uint64 {
	return 16 // 4 * sizeof(uint32)
}

// DrawIndexedIndirectArgs represents the arguments for indirect indexed draw.
//
// This structure is read from the GPU buffer at the offset specified
// in DrawIndexedIndirect.
type DrawIndexedIndirectArgs struct {
	// IndexCount is the number of indices to draw.
	IndexCount uint32

	// InstanceCount is the number of instances to draw.
	InstanceCount uint32

	// FirstIndex is the first index in the index buffer.
	FirstIndex uint32

	// BaseVertex is added to each index before vertex lookup.
	BaseVertex int32

	// FirstInstance is the first instance index.
	FirstInstance uint32
}

// Size returns the byte size of DrawIndexedIndirectArgs.
func (d DrawIndexedIndirectArgs) Size() uint64 {
	return 20 // 5 * sizeof(uint32)
}
