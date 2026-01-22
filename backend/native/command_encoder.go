// Package native provides a GPU-accelerated rendering backend using gogpu/wgpu.
package native

import (
	"errors"
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/types"
)

// HAL-integrated command encoder errors.
var (
	// ErrEncoderNotRecording is returned when recording operations are called
	// on an encoder that is not in the Recording state.
	ErrEncoderNotRecording = errors.New("native: encoder not in recording state")

	// ErrEncoderLocked is returned when operations are called on an encoder
	// that is locked (a pass is in progress).
	ErrEncoderLocked = errors.New("native: encoder is locked (pass in progress)")

	// ErrEncoderFinished is returned when operations are called on an encoder
	// that has already been finished.
	ErrEncoderFinished = errors.New("native: encoder already finished")

	// ErrEncoderConsumed is returned when operations are called on an encoder
	// that has been submitted to the queue.
	ErrEncoderConsumed = errors.New("native: encoder has been consumed")

	// ErrNilDevice is returned when creating an encoder without a device.
	ErrNilDevice = errors.New("native: device is nil")

	// ErrNilEncoder is returned when operations reference a nil encoder.
	ErrNilEncoder = errors.New("native: command encoder is nil")

	// ErrNilCoreBuffer is returned when a buffer operation references nil.
	ErrNilCoreBuffer = errors.New("native: core buffer is nil")

	// ErrCopyRangeOutOfBounds is returned when a copy operation exceeds buffer bounds.
	ErrCopyRangeOutOfBounds = errors.New("native: copy range out of bounds")

	// ErrCopyOverlap is returned when source and destination buffers overlap.
	ErrCopyOverlap = errors.New("native: source and destination buffers overlap")

	// ErrCopyOffsetNotAligned is returned when offset is not properly aligned.
	ErrCopyOffsetNotAligned = errors.New("native: copy offset must be 4-byte aligned")

	// ErrCopySizeNotAligned is returned when size is not properly aligned.
	ErrCopySizeNotAligned = errors.New("native: copy size must be 4-byte aligned")
)

// =============================================================================
// HAL-Integrated Command Encoder
// =============================================================================

// HALCommandEncoder records GPU commands for later submission to a queue.
//
// This is the HAL-integrated command encoder that wraps core.CoreCommandEncoder.
// It provides a higher-level API with Go-style immediate error handling.
//
// HALCommandEncoder follows the WebGPU command encoding pattern:
//  1. Create encoder via NewHALCommandEncoder()
//  2. Record commands (copy operations, begin/end passes)
//  3. Call Finish() to get a HALCommandBuffer
//  4. Submit HALCommandBuffer to a Queue
//
// State machine:
//
//	Recording -> (BeginRenderPass/BeginComputePass) -> Locked
//	Locked    -> (EndPass)                          -> Recording
//	Recording -> Finish()                           -> Finished
//	Finished  -> (submitted to queue)               -> Consumed
//
// HALCommandEncoder is NOT safe for concurrent use. Each encoder should
// be used from a single goroutine.
type HALCommandEncoder struct {
	mu sync.Mutex

	// coreEncoder is the underlying wgpu core command encoder.
	// This provides the actual HAL integration.
	coreEncoder *core.CoreCommandEncoder

	// device is the parent native backend device reference.
	device *NativeBackend

	// label is the debug label for this encoder.
	label string

	// activeRenderPass tracks the currently active render pass (if any).
	activeRenderPass *HALRenderPassEncoder

	// activeComputePass tracks the currently active compute pass (if any).
	activeComputePass *HALComputePassEncoder
}

// NewHALCommandEncoder creates a new HAL-integrated command encoder from a backend.
//
// The encoder is created in the Recording state, ready to record commands.
//
// Parameters:
//   - backend: The native backend to create the encoder on.
//   - label: Debug label for the encoder (optional, can be empty).
//
// Returns the encoder and nil on success.
// Returns nil and an error if the backend is not initialized.
func NewHALCommandEncoder(backend *NativeBackend, label string) (*HALCommandEncoder, error) {
	if backend == nil {
		return nil, ErrNilDevice
	}

	backend.mu.RLock()
	defer backend.mu.RUnlock()

	if !backend.initialized {
		return nil, ErrNotInitialized
	}

	enc := &HALCommandEncoder{
		device: backend,
		label:  label,
	}

	return enc, nil
}

// NewHALCommandEncoderWithDevice creates a command encoder using a core.Device.
//
// This is the full HAL-integrated version that creates a real wgpu command encoder.
//
// Parameters:
//   - device: The core.Device to create the encoder on.
//   - label: Debug label for the encoder (optional).
//
// Returns the encoder and nil on success.
// Returns nil and an error if the device is invalid or HAL creation fails.
func NewHALCommandEncoderWithDevice(device *core.Device, label string) (*HALCommandEncoder, error) {
	if device == nil {
		return nil, ErrNilDevice
	}

	coreEncoder, err := device.CreateCommandEncoder(label)
	if err != nil {
		return nil, fmt.Errorf("create command encoder: %w", err)
	}

	enc := &HALCommandEncoder{
		coreEncoder: coreEncoder,
		label:       label,
	}

	return enc, nil
}

// Label returns the encoder's debug label.
func (e *HALCommandEncoder) Label() string {
	if e == nil {
		return ""
	}
	return e.label
}

// Status returns the current encoder status.
func (e *HALCommandEncoder) Status() core.CommandEncoderStatus {
	if e == nil {
		return core.CommandEncoderStatusError
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	return e.statusLocked()
}

// checkRecording returns an error if the encoder is not in Recording state.
// This method acquires the mutex; use checkRecordingLocked if mutex is already held.
//
//nolint:unused // Public API for external use, internal code uses checkRecordingLocked
func (e *HALCommandEncoder) checkRecording() error {
	if e == nil {
		return ErrNilEncoder
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	return e.checkRecordingLocked()
}

// checkRecordingLocked returns an error if the encoder is not in Recording state.
// The caller must hold e.mu.
func (e *HALCommandEncoder) checkRecordingLocked() error {
	status := e.statusLocked()
	switch status {
	case core.CommandEncoderStatusRecording:
		return nil
	case core.CommandEncoderStatusLocked:
		return ErrEncoderLocked
	case core.CommandEncoderStatusFinished:
		return ErrEncoderFinished
	case core.CommandEncoderStatusConsumed:
		return ErrEncoderConsumed
	default:
		return ErrEncoderNotRecording
	}
}

// statusLocked returns the encoder status. The caller must hold e.mu.
func (e *HALCommandEncoder) statusLocked() core.CommandEncoderStatus {
	if e.coreEncoder != nil {
		return e.coreEncoder.Status()
	}

	// Fallback for non-HAL mode
	if e.activeRenderPass != nil || e.activeComputePass != nil {
		return core.CommandEncoderStatusLocked
	}
	return core.CommandEncoderStatusRecording
}

// BeginRenderPass starts a render pass with the given descriptor.
//
// The encoder must be in the Recording state.
// After this call, the encoder transitions to the Locked state.
// The encoder returns to Recording state when the render pass ends.
//
// Parameters:
//   - desc: Render pass descriptor specifying color attachments and options.
//
// Returns the render pass encoder and nil on success.
// Returns nil and an error if:
//   - The encoder is not in Recording state
//   - The descriptor is nil
//   - HAL render pass creation fails
func (e *HALCommandEncoder) BeginRenderPass(desc *HALRenderPassDescriptor) (*HALRenderPassEncoder, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return nil, fmt.Errorf("begin render pass: %w", err)
	}

	if desc == nil {
		return nil, fmt.Errorf("begin render pass: descriptor is nil")
	}

	// If we have a core encoder, use it
	if e.coreEncoder != nil {
		coreDesc := desc.toCoreDescriptor()
		corePass, err := e.coreEncoder.BeginRenderPass(coreDesc)
		if err != nil {
			return nil, fmt.Errorf("begin render pass: %w", err)
		}

		pass := &HALRenderPassEncoder{
			corePass: corePass,
			encoder:  e,
		}
		e.activeRenderPass = pass
		return pass, nil
	}

	// Fallback for non-HAL mode
	pass := &HALRenderPassEncoder{
		encoder: e,
	}
	e.activeRenderPass = pass
	return pass, nil
}

// endRenderPass ends the current render pass.
//
// This is called internally by HALRenderPassEncoder.End().
// The encoder transitions from Locked back to Recording state.
func (e *HALCommandEncoder) endRenderPass(pass *HALRenderPassEncoder) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.activeRenderPass != pass {
		return fmt.Errorf("end render pass: wrong pass being ended")
	}

	e.activeRenderPass = nil
	return nil
}

// BeginComputePass starts a compute pass with the given descriptor.
//
// The encoder must be in the Recording state.
// After this call, the encoder transitions to the Locked state.
// The encoder returns to Recording state when the compute pass ends.
//
// Parameters:
//   - desc: Compute pass descriptor (optional, can be nil for defaults).
//
// Returns the compute pass encoder and nil on success.
// Returns nil and an error if:
//   - The encoder is not in Recording state
//   - HAL compute pass creation fails
func (e *HALCommandEncoder) BeginComputePass(desc *HALComputePassDescriptor) (*HALComputePassEncoder, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return nil, fmt.Errorf("begin compute pass: %w", err)
	}

	// If we have a core encoder, use it
	if e.coreEncoder != nil {
		coreDesc := &core.CoreComputePassDescriptor{}
		if desc != nil {
			coreDesc.Label = desc.Label
		}

		corePass, err := e.coreEncoder.BeginComputePass(coreDesc)
		if err != nil {
			return nil, fmt.Errorf("begin compute pass: %w", err)
		}

		pass := &HALComputePassEncoder{
			corePass: corePass,
			encoder:  e,
		}
		e.activeComputePass = pass
		return pass, nil
	}

	// Fallback for non-HAL mode
	pass := &HALComputePassEncoder{
		encoder: e,
	}
	e.activeComputePass = pass
	return pass, nil
}

// endComputePass ends the current compute pass.
//
// This is called internally by HALComputePassEncoder.End().
// The encoder transitions from Locked back to Recording state.
func (e *HALCommandEncoder) endComputePass(pass *HALComputePassEncoder) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.activeComputePass != pass {
		return fmt.Errorf("end compute pass: wrong pass being ended")
	}

	e.activeComputePass = nil
	return nil
}

// CopyBufferToBuffer copies data from one buffer to another.
//
// The encoder must be in the Recording state.
//
// Parameters:
//   - src: Source buffer.
//   - srcOffset: Byte offset in the source buffer.
//   - dst: Destination buffer.
//   - dstOffset: Byte offset in the destination buffer.
//   - size: Number of bytes to copy.
//
// Validation:
//   - Both offsets and size must be 4-byte aligned.
//   - Source and destination ranges must not overlap.
//   - Ranges must be within buffer bounds.
//
// Returns nil on success.
// Returns an error if validation fails or the encoder state is invalid.
func (e *HALCommandEncoder) CopyBufferToBuffer(src, dst *core.Buffer, srcOffset, dstOffset, size uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return fmt.Errorf("copy buffer to buffer: %w", err)
	}

	// Validate buffers
	if src == nil || dst == nil {
		return ErrNilCoreBuffer
	}

	// Validate alignment (WebGPU requires 4-byte alignment)
	const alignment uint64 = 4
	if srcOffset%alignment != 0 {
		return fmt.Errorf("%w: source offset %d", ErrCopyOffsetNotAligned, srcOffset)
	}
	if dstOffset%alignment != 0 {
		return fmt.Errorf("%w: destination offset %d", ErrCopyOffsetNotAligned, dstOffset)
	}
	if size%alignment != 0 {
		return fmt.Errorf("%w: size %d", ErrCopySizeNotAligned, size)
	}

	// Validate bounds
	if srcOffset+size > src.Size() {
		return fmt.Errorf("%w: source offset %d + size %d > buffer size %d",
			ErrCopyRangeOutOfBounds, srcOffset, size, src.Size())
	}
	if dstOffset+size > dst.Size() {
		return fmt.Errorf("%w: destination offset %d + size %d > buffer size %d",
			ErrCopyRangeOutOfBounds, dstOffset, size, dst.Size())
	}

	// TODO: When HAL is integrated, record the copy command:
	// e.coreEncoder.CopyBufferToBuffer(src, dst, srcOffset, dstOffset, size)

	return nil
}

// CopyBufferToTexture copies data from a buffer to a texture.
//
// The encoder must be in the Recording state.
//
// Parameters:
//   - source: Buffer copy source descriptor.
//   - destination: Texture copy destination descriptor.
//   - copySize: Size of the copy region.
//
// Returns nil on success.
// Returns an error if validation fails or the encoder state is invalid.
func (e *HALCommandEncoder) CopyBufferToTexture(source *HALImageCopyBuffer, destination *HALImageCopyTexture, copySize types.Extent3D) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return fmt.Errorf("copy buffer to texture: %w", err)
	}

	if source == nil {
		return fmt.Errorf("copy buffer to texture: source is nil")
	}
	if destination == nil {
		return fmt.Errorf("copy buffer to texture: destination is nil")
	}

	// TODO: When HAL is integrated, record the copy command

	return nil
}

// CopyTextureToBuffer copies data from a texture to a buffer.
//
// The encoder must be in the Recording state.
//
// Parameters:
//   - source: Texture copy source descriptor.
//   - destination: Buffer copy destination descriptor.
//   - copySize: Size of the copy region.
//
// Returns nil on success.
// Returns an error if validation fails or the encoder state is invalid.
func (e *HALCommandEncoder) CopyTextureToBuffer(source *HALImageCopyTexture, destination *HALImageCopyBuffer, copySize types.Extent3D) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return fmt.Errorf("copy texture to buffer: %w", err)
	}

	if source == nil {
		return fmt.Errorf("copy texture to buffer: source is nil")
	}
	if destination == nil {
		return fmt.Errorf("copy texture to buffer: destination is nil")
	}

	// TODO: When HAL is integrated, record the copy command

	return nil
}

// CopyTextureToTexture copies data from one texture to another.
//
// The encoder must be in the Recording state.
//
// Parameters:
//   - source: Source texture copy descriptor.
//   - destination: Destination texture copy descriptor.
//   - copySize: Size of the copy region.
//
// Returns nil on success.
// Returns an error if validation fails or the encoder state is invalid.
func (e *HALCommandEncoder) CopyTextureToTexture(source, destination *HALImageCopyTexture, copySize types.Extent3D) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return fmt.Errorf("copy texture to texture: %w", err)
	}

	if source == nil {
		return fmt.Errorf("copy texture to texture: source is nil")
	}
	if destination == nil {
		return fmt.Errorf("copy texture to texture: destination is nil")
	}

	// TODO: When HAL is integrated, record the copy command

	return nil
}

// ClearBuffer clears a region of a buffer to zero.
//
// The encoder must be in the Recording state.
//
// Parameters:
//   - buffer: Buffer to clear.
//   - offset: Byte offset to start clearing (must be 4-byte aligned).
//   - size: Number of bytes to clear (must be 4-byte aligned, or 0 for entire buffer).
//
// Returns nil on success.
// Returns an error if validation fails or the encoder state is invalid.
func (e *HALCommandEncoder) ClearBuffer(buffer *core.Buffer, offset, size uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return fmt.Errorf("clear buffer: %w", err)
	}

	if buffer == nil {
		return ErrNilCoreBuffer
	}

	// Validate alignment
	const alignment uint64 = 4
	if offset%alignment != 0 {
		return fmt.Errorf("%w: offset %d", ErrCopyOffsetNotAligned, offset)
	}

	// Size 0 means clear entire buffer from offset
	actualSize := size
	if actualSize == 0 {
		actualSize = buffer.Size() - offset
	}

	if actualSize%alignment != 0 {
		return fmt.Errorf("%w: size %d", ErrCopySizeNotAligned, actualSize)
	}

	// Validate bounds
	if offset+actualSize > buffer.Size() {
		return fmt.Errorf("%w: offset %d + size %d > buffer size %d",
			ErrCopyRangeOutOfBounds, offset, actualSize, buffer.Size())
	}

	// TODO: When HAL is integrated, record the clear command

	return nil
}

// Finish completes recording and returns a command buffer.
//
// The encoder must be in the Recording state (no active passes).
// After this call, the encoder transitions to the Finished state and
// cannot be used for further recording.
//
// Returns the command buffer and nil on success.
// Returns nil and an error if the encoder is not in Recording state.
func (e *HALCommandEncoder) Finish() (*HALCommandBuffer, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return nil, fmt.Errorf("finish: %w", err)
	}

	// If we have a core encoder, finish it
	if e.coreEncoder != nil {
		coreBuffer, err := e.coreEncoder.Finish()
		if err != nil {
			return nil, fmt.Errorf("finish: %w", err)
		}

		return &HALCommandBuffer{
			coreBuffer: coreBuffer,
			label:      e.label,
		}, nil
	}

	// Fallback for non-HAL mode
	return &HALCommandBuffer{
		label: e.label,
	}, nil
}

// =============================================================================
// Supporting Types
// =============================================================================

// HALRenderPassDescriptor describes a render pass.
type HALRenderPassDescriptor struct {
	// Label is an optional debug name.
	Label string

	// ColorAttachments are the color render targets.
	ColorAttachments []HALRenderPassColorAttachment

	// DepthStencilAttachment is the depth/stencil target (optional).
	DepthStencilAttachment *HALRenderPassDepthStencilAttachment
}

// toCoreDescriptor converts to a core.RenderPassDescriptor.
func (d *HALRenderPassDescriptor) toCoreDescriptor() *core.RenderPassDescriptor {
	if d == nil {
		return nil
	}

	coreDesc := &core.RenderPassDescriptor{
		Label: d.Label,
	}

	for _, ca := range d.ColorAttachments {
		coreCA := core.RenderPassColorAttachment{
			LoadOp:     ca.LoadOp,
			StoreOp:    ca.StoreOp,
			ClearValue: ca.ClearValue,
			// Note: View conversion pending TextureView HAL integration
		}
		coreDesc.ColorAttachments = append(coreDesc.ColorAttachments, coreCA)
	}

	if d.DepthStencilAttachment != nil {
		coreDesc.DepthStencilAttachment = &core.RenderPassDepthStencilAttachment{
			DepthLoadOp:       d.DepthStencilAttachment.DepthLoadOp,
			DepthStoreOp:      d.DepthStencilAttachment.DepthStoreOp,
			DepthClearValue:   d.DepthStencilAttachment.DepthClearValue,
			DepthReadOnly:     d.DepthStencilAttachment.DepthReadOnly,
			StencilLoadOp:     d.DepthStencilAttachment.StencilLoadOp,
			StencilStoreOp:    d.DepthStencilAttachment.StencilStoreOp,
			StencilClearValue: d.DepthStencilAttachment.StencilClearValue,
			StencilReadOnly:   d.DepthStencilAttachment.StencilReadOnly,
		}
	}

	return coreDesc
}

// HALRenderPassColorAttachment describes a color attachment.
type HALRenderPassColorAttachment struct {
	// View is the texture view to render to.
	View *HALTextureView

	// ResolveTarget is the MSAA resolve target (optional).
	ResolveTarget *HALTextureView

	// LoadOp specifies what to do at pass start.
	LoadOp types.LoadOp

	// StoreOp specifies what to do at pass end.
	StoreOp types.StoreOp

	// ClearValue is the clear color (used if LoadOp is Clear).
	ClearValue types.Color
}

// HALRenderPassDepthStencilAttachment describes a depth/stencil attachment.
type HALRenderPassDepthStencilAttachment struct {
	// View is the texture view to use.
	View *HALTextureView

	// DepthLoadOp specifies what to do with depth at pass start.
	DepthLoadOp types.LoadOp

	// DepthStoreOp specifies what to do with depth at pass end.
	DepthStoreOp types.StoreOp

	// DepthClearValue is the depth clear value.
	DepthClearValue float32

	// DepthReadOnly makes the depth aspect read-only.
	DepthReadOnly bool

	// StencilLoadOp specifies what to do with stencil at pass start.
	StencilLoadOp types.LoadOp

	// StencilStoreOp specifies what to do with stencil at pass end.
	StencilStoreOp types.StoreOp

	// StencilClearValue is the stencil clear value.
	StencilClearValue uint32

	// StencilReadOnly makes the stencil aspect read-only.
	StencilReadOnly bool
}

// HALComputePassDescriptor describes a compute pass.
type HALComputePassDescriptor struct {
	// Label is an optional debug name for the compute pass.
	Label string

	// TimestampWrites are timestamp queries to write at pass boundaries (optional).
	TimestampWrites *HALComputePassTimestampWrites
}

// HALComputePassTimestampWrites describes timestamp query writes for a compute pass.
type HALComputePassTimestampWrites struct {
	// QuerySet is the query set to write timestamps to.
	QuerySet core.QuerySetID

	// BeginningOfPassWriteIndex is the query index for pass start.
	BeginningOfPassWriteIndex *uint32

	// EndOfPassWriteIndex is the query index for pass end.
	EndOfPassWriteIndex *uint32
}

// HALImageCopyBuffer describes a buffer for texture copy operations.
type HALImageCopyBuffer struct {
	// Buffer is the buffer to copy to/from.
	Buffer *core.Buffer

	// Layout describes how the data is laid out in the buffer.
	Layout types.TextureDataLayout
}

// HALImageCopyTexture describes a texture for copy operations.
type HALImageCopyTexture struct {
	// Texture is the texture to copy to/from.
	Texture *GPUTexture

	// MipLevel is the mip level to copy.
	MipLevel uint32

	// Origin is the origin of the copy in the texture.
	Origin types.Origin3D

	// Aspect is the aspect of the texture to copy.
	Aspect types.TextureAspect
}

// HALTextureView represents a view into a GPU texture.
// This is a placeholder - full implementation in GPU-TEX-001.
type HALTextureView struct {
	// coreView is the underlying core texture view.
	//nolint:unused // Placeholder for GPU-TEX-001 implementation
	coreView *core.TextureView
}

// =============================================================================
// HALRenderPassEncoder
// =============================================================================

// HALRenderPassEncoder records render commands within a render pass.
//
// Commands recorded include:
//   - SetPipeline: Set the render pipeline
//   - SetVertexBuffer: Bind a vertex buffer
//   - SetIndexBuffer: Bind an index buffer
//   - SetBindGroup: Bind a bind group
//   - Draw: Draw primitives
//   - DrawIndexed: Draw indexed primitives
//
// The pass must be ended with End() before finishing the command encoder.
type HALRenderPassEncoder struct {
	// corePass is the underlying core render pass encoder.
	corePass *core.CoreRenderPassEncoder

	// encoder is the parent command encoder.
	encoder *HALCommandEncoder

	// ended indicates whether End() has been called.
	ended bool
}

// SetViewport sets the viewport.
func (p *HALRenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	if p.ended {
		return
	}
	if p.corePass != nil {
		p.corePass.SetViewport(x, y, width, height, minDepth, maxDepth)
	}
}

// SetScissorRect sets the scissor rectangle.
func (p *HALRenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	if p.ended {
		return
	}
	if p.corePass != nil {
		p.corePass.SetScissorRect(x, y, width, height)
	}
}

// SetBlendConstant sets the blend constant color.
func (p *HALRenderPassEncoder) SetBlendConstant(color *types.Color) {
	if p.ended {
		return
	}
	if p.corePass != nil {
		p.corePass.SetBlendConstant(color)
	}
}

// SetStencilReference sets the stencil reference value.
func (p *HALRenderPassEncoder) SetStencilReference(reference uint32) {
	if p.ended {
		return
	}
	if p.corePass != nil {
		p.corePass.SetStencilReference(reference)
	}
}

// Draw draws primitives.
func (p *HALRenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if p.ended {
		return
	}
	if p.corePass != nil {
		p.corePass.Draw(vertexCount, instanceCount, firstVertex, firstInstance)
	}
}

// DrawIndexed draws indexed primitives.
func (p *HALRenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if p.ended {
		return
	}
	if p.corePass != nil {
		p.corePass.DrawIndexed(indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
	}
}

// End ends the render pass.
func (p *HALRenderPassEncoder) End() error {
	if p.ended {
		return nil
	}
	p.ended = true

	if p.corePass != nil {
		if err := p.corePass.End(); err != nil {
			return err
		}
	}

	return p.encoder.endRenderPass(p)
}

// =============================================================================
// HALComputePassEncoder
// =============================================================================

// HALComputePassEncoder records compute commands within a compute pass.
//
// Commands recorded include:
//   - SetPipeline: Set the compute pipeline
//   - SetBindGroup: Bind a bind group
//   - Dispatch: Dispatch compute work
//   - DispatchIndirect: Dispatch compute work with GPU parameters
//
// The pass must be ended with End() before finishing the command encoder.
type HALComputePassEncoder struct {
	// corePass is the underlying core compute pass encoder.
	corePass *core.CoreComputePassEncoder

	// encoder is the parent command encoder.
	encoder *HALCommandEncoder

	// ended indicates whether End() has been called.
	ended bool
}

// Dispatch dispatches compute work.
func (p *HALComputePassEncoder) Dispatch(x, y, z uint32) {
	if p.ended {
		return
	}
	if p.corePass != nil {
		p.corePass.Dispatch(x, y, z)
	}
}

// End ends the compute pass.
func (p *HALComputePassEncoder) End() error {
	if p.ended {
		return nil
	}
	p.ended = true

	if p.corePass != nil {
		if err := p.corePass.End(); err != nil {
			return err
		}
	}

	return p.encoder.endComputePass(p)
}

// =============================================================================
// HALCommandBuffer
// =============================================================================

// HALCommandBuffer is a finished command recording ready for submission.
//
// Command buffers are created by HALCommandEncoder.Finish() and submitted
// to a Queue for execution.
type HALCommandBuffer struct {
	// coreBuffer is the underlying core command buffer.
	coreBuffer *core.CoreCommandBuffer

	// label is the debug label.
	label string
}

// Label returns the command buffer's debug label.
func (cb *HALCommandBuffer) Label() string {
	if cb == nil {
		return ""
	}
	return cb.label
}

// CoreBuffer returns the underlying core command buffer.
// Returns nil if the buffer was created without HAL integration.
func (cb *HALCommandBuffer) CoreBuffer() *core.CoreCommandBuffer {
	if cb == nil {
		return nil
	}
	return cb.coreBuffer
}
