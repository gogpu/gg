// Package gpu provides a GPU-accelerated rendering backend using gogpu/wgpu.
package gpu

import (
	"errors"
	"fmt"
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Buffer errors.
var (
	// ErrBufferDestroyed is returned when operating on a destroyed buffer.
	ErrBufferDestroyed = errors.New("gpu: buffer has been destroyed")

	// ErrNilBuffer is returned when creating operations without a buffer.
	ErrNilBuffer = errors.New("gpu: buffer is nil")

	// ErrInvalidBufferSize is returned when buffer size is invalid.
	ErrInvalidBufferSize = errors.New("gpu: invalid buffer size")

	// ErrBufferAlreadyMapped is returned when attempting to map an already mapped buffer.
	ErrBufferAlreadyMapped = errors.New("gpu: buffer is already mapped or mapping is pending")

	// ErrBufferNotMapped is returned when attempting to access unmapped buffer data.
	ErrBufferNotMapped = errors.New("gpu: buffer is not mapped")

	// ErrBufferMapPending is returned when accessing a buffer with pending map operation.
	ErrBufferMapPending = errors.New("gpu: buffer mapping is pending")

	// ErrInvalidMapMode is returned when mapping with an invalid mode.
	ErrInvalidMapMode = errors.New("gpu: invalid map mode")

	// ErrInvalidMapRange is returned when the map range is out of bounds.
	ErrInvalidMapRange = errors.New("gpu: map range out of bounds")

	// ErrMapUsageMismatch is returned when mapping mode doesn't match buffer usage.
	ErrMapUsageMismatch = errors.New("gpu: map mode does not match buffer usage flags")

	// ErrMappingFailed is returned when buffer mapping fails.
	ErrMappingFailed = errors.New("gpu: buffer mapping failed")

	// ErrCallbackNil is returned when MapAsync is called with nil callback.
	ErrCallbackNil = errors.New("gpu: map callback is nil")
)

// BufferMapState represents the mapping state of a buffer.
type BufferMapState int

const (
	// BufferMapStateUnmapped means the buffer is not mapped.
	BufferMapStateUnmapped BufferMapState = iota
	// BufferMapStatePending means a map operation is pending.
	BufferMapStatePending
	// BufferMapStateMapped means the buffer is mapped.
	BufferMapStateMapped
)

// String returns the string representation of BufferMapState.
func (s BufferMapState) String() string {
	switch s {
	case BufferMapStateUnmapped:
		return "Unmapped"
	case BufferMapStatePending:
		return "Pending"
	case BufferMapStateMapped:
		return "Mapped"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// BufferMapAsyncStatus represents the result of an async map operation.
type BufferMapAsyncStatus int

const (
	// BufferMapAsyncStatusSuccess indicates mapping completed successfully.
	BufferMapAsyncStatusSuccess BufferMapAsyncStatus = iota
	// BufferMapAsyncStatusValidationError indicates a validation error.
	BufferMapAsyncStatusValidationError
	// BufferMapAsyncStatusUnknown indicates an unknown error.
	BufferMapAsyncStatusUnknown
	// BufferMapAsyncStatusDeviceLost indicates the device was lost.
	BufferMapAsyncStatusDeviceLost
	// BufferMapAsyncStatusDestroyedBeforeCallback indicates buffer was destroyed.
	BufferMapAsyncStatusDestroyedBeforeCallback
	// BufferMapAsyncStatusUnmappedBeforeCallback indicates buffer was unmapped.
	BufferMapAsyncStatusUnmappedBeforeCallback
	// BufferMapAsyncStatusMappingAlreadyPending indicates another map is pending.
	BufferMapAsyncStatusMappingAlreadyPending
	// BufferMapAsyncStatusOffsetOutOfRange indicates offset is out of range.
	BufferMapAsyncStatusOffsetOutOfRange
	// BufferMapAsyncStatusSizeOutOfRange indicates size is out of range.
	BufferMapAsyncStatusSizeOutOfRange
)

// String returns the string representation of BufferMapAsyncStatus.
func (s BufferMapAsyncStatus) String() string {
	switch s {
	case BufferMapAsyncStatusSuccess:
		return "Success"
	case BufferMapAsyncStatusValidationError:
		return "ValidationError"
	case BufferMapAsyncStatusUnknown:
		return "Unknown"
	case BufferMapAsyncStatusDeviceLost:
		return "DeviceLost"
	case BufferMapAsyncStatusDestroyedBeforeCallback:
		return "DestroyedBeforeCallback"
	case BufferMapAsyncStatusUnmappedBeforeCallback:
		return "UnmappedBeforeCallback"
	case BufferMapAsyncStatusMappingAlreadyPending:
		return "MappingAlreadyPending"
	case BufferMapAsyncStatusOffsetOutOfRange:
		return "OffsetOutOfRange"
	case BufferMapAsyncStatusSizeOutOfRange:
		return "SizeOutOfRange"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// Buffer represents a GPU buffer resource.
//
// Buffer wraps a hal.Buffer and provides Go-idiomatic access with
// async buffer mapping support. This follows the wgpu pattern where
// buffer mapping is asynchronous and requires device polling.
//
// Thread Safety:
// Buffer is safe for concurrent access. All state mutations are
// protected by a mutex. The mapping callback is invoked from the
// polling goroutine.
//
// Lifecycle:
//  1. Create via CreateBuffer()
//  2. Use MapAsync() to initiate mapping
//  3. Poll with PollMapAsync() until complete
//  4. Access data with GetMappedRange()
//  5. Call Unmap() when done
//  6. Call Destroy() when the buffer is no longer needed
type Buffer struct {
	// mu protects mutable state.
	mu sync.RWMutex

	// halBuffer is the underlying buffer handle.
	halBuffer hal.Buffer

	// device is the parent device.
	device hal.Device

	// descriptor holds the buffer configuration (immutable after creation).
	descriptor BufferDescriptor

	// mapState is the current mapping state.
	mapState BufferMapState

	// mapMode is the mode used for the current mapping.
	mapMode gputypes.MapMode

	// mapOffset is the offset of the current mapping.
	mapOffset uint64

	// mapSize is the size of the current mapping.
	mapSize uint64

	// mappedData holds the mapped memory slice (only valid when mapped).
	mappedData []byte

	// mapCallback is the callback for async map operations.
	mapCallback func(BufferMapAsyncStatus)

	// destroyed indicates whether the buffer has been destroyed.
	destroyed bool
}

// BufferDescriptor describes a buffer to create.
type BufferDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Size is the buffer size in bytes.
	Size uint64

	// Usage specifies how the buffer will be used.
	Usage gputypes.BufferUsage

	// MappedAtCreation creates the buffer pre-mapped for writing.
	MappedAtCreation bool
}

// NewBuffer creates a new Buffer from a buffer handle.
//
// This is typically called by CreateBuffer() after successfully
// creating the underlying buffer.
//
// Parameters:
//   - halBuffer: The underlying buffer (ownership transferred)
//   - device: The parent device (retained for operations)
//   - desc: The buffer descriptor (copied)
//
// Returns the new Buffer.
func NewBuffer(halBuffer hal.Buffer, device hal.Device, desc *BufferDescriptor) *Buffer {
	buf := &Buffer{
		halBuffer:  halBuffer,
		device:     device,
		descriptor: *desc,
		mapState:   BufferMapStateUnmapped,
	}

	// If mapped at creation, set state to mapped
	if desc.MappedAtCreation {
		buf.mapState = BufferMapStateMapped
		buf.mapMode = gputypes.MapModeWrite
		buf.mapOffset = 0
		buf.mapSize = desc.Size
	}

	return buf
}

// Label returns the buffer's debug label.
func (b *Buffer) Label() string {
	return b.descriptor.Label
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 {
	return b.descriptor.Size
}

// Usage returns the buffer usage flags.
func (b *Buffer) Usage() gputypes.BufferUsage {
	return b.descriptor.Usage
}

// Descriptor returns a copy of the buffer descriptor.
func (b *Buffer) Descriptor() BufferDescriptor {
	return b.descriptor
}

// MapState returns the current mapping state.
func (b *Buffer) MapState() BufferMapState {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.mapState
}

// IsDestroyed returns true if the buffer has been destroyed.
func (b *Buffer) IsDestroyed() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.destroyed
}

// Raw returns the underlying buffer handle.
//
// Returns nil if the buffer has been destroyed.
// Use with caution - the caller should ensure the buffer is not destroyed
// while the handle is in use.
func (b *Buffer) Raw() hal.Buffer {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.destroyed {
		return nil
	}
	return b.halBuffer
}

// MapAsync initiates an async map operation.
//
// The callback is invoked when mapping completes or fails. The buffer
// must have appropriate usage flags (MapRead for read, MapWrite for write).
//
// After MapAsync returns successfully, the map state transitions to Pending.
// Poll the device with PollMapAsync() until the callback is invoked and
// the state becomes Mapped.
//
// Parameters:
//   - mode: MapModeRead or MapModeWrite
//   - offset: Byte offset in the buffer (must be aligned)
//   - size: Number of bytes to map (must be aligned)
//   - callback: Function called when mapping completes
//
// Returns nil on success (mapping initiated).
// Returns an error if:
//   - The buffer has been destroyed
//   - The buffer is already mapped or mapping is pending
//   - The mode doesn't match buffer usage flags
//   - The range is out of bounds
//   - The callback is nil
func (b *Buffer) MapAsync(mode gputypes.MapMode, offset, size uint64, callback func(BufferMapAsyncStatus)) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if destroyed
	if b.destroyed {
		return ErrBufferDestroyed
	}

	// Check if already mapped or pending
	if b.mapState != BufferMapStateUnmapped {
		if callback != nil {
			callback(BufferMapAsyncStatusMappingAlreadyPending)
		}
		return ErrBufferAlreadyMapped
	}

	// Validate callback
	if callback == nil {
		return ErrCallbackNil
	}

	// Validate mode
	if mode == 0 {
		callback(BufferMapAsyncStatusValidationError)
		return ErrInvalidMapMode
	}

	// Validate mode matches usage
	if mode == gputypes.MapModeRead && !b.descriptor.Usage.Contains(gputypes.BufferUsageMapRead) {
		callback(BufferMapAsyncStatusValidationError)
		return fmt.Errorf("%w: buffer does not have MapRead usage", ErrMapUsageMismatch)
	}
	if mode == gputypes.MapModeWrite && !b.descriptor.Usage.Contains(gputypes.BufferUsageMapWrite) {
		callback(BufferMapAsyncStatusValidationError)
		return fmt.Errorf("%w: buffer does not have MapWrite usage", ErrMapUsageMismatch)
	}

	// Validate range
	if offset > b.descriptor.Size {
		callback(BufferMapAsyncStatusOffsetOutOfRange)
		return fmt.Errorf("%w: offset %d > buffer size %d", ErrInvalidMapRange, offset, b.descriptor.Size)
	}
	if offset+size > b.descriptor.Size {
		callback(BufferMapAsyncStatusSizeOutOfRange)
		return fmt.Errorf("%w: offset %d + size %d > buffer size %d", ErrInvalidMapRange, offset, size, b.descriptor.Size)
	}

	// Validate alignment (WebGPU requires 8-byte alignment for map operations)
	const mapAlignment uint64 = 8
	if offset%mapAlignment != 0 {
		callback(BufferMapAsyncStatusValidationError)
		return fmt.Errorf("%w: offset %d must be %d-byte aligned", ErrInvalidMapRange, offset, mapAlignment)
	}
	if size%mapAlignment != 0 && size != b.descriptor.Size-offset {
		// Size doesn't need alignment if mapping to end of buffer
		callback(BufferMapAsyncStatusValidationError)
		return fmt.Errorf("%w: size %d must be %d-byte aligned", ErrInvalidMapRange, size, mapAlignment)
	}

	// Transition to pending state
	b.mapState = BufferMapStatePending
	b.mapMode = mode
	b.mapOffset = offset
	b.mapSize = size
	b.mapCallback = callback

	// Note: In a real implementation, this would initiate the buffer map
	// operation. For now, we simulate immediate completion in PollMapAsync.

	return nil
}

// PollMapAsync polls for map completion.
//
// Call this method repeatedly after MapAsync() until it returns true,
// indicating that mapping is complete (either success or failure).
// The callback provided to MapAsync will be invoked when mapping completes.
//
// Returns true if mapping is complete (success or failure).
// Returns false if mapping is still pending.
func (b *Buffer) PollMapAsync() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If not pending, return immediately
	if b.mapState != BufferMapStatePending {
		return b.mapState == BufferMapStateMapped || b.mapState == BufferMapStateUnmapped
	}

	// Check if destroyed during pending
	if b.destroyed {
		if b.mapCallback != nil {
			callback := b.mapCallback
			b.mapCallback = nil
			b.mapState = BufferMapStateUnmapped
			// Call callback outside lock to avoid deadlock
			b.mu.Unlock()
			callback(BufferMapAsyncStatusDestroyedBeforeCallback)
			b.mu.Lock()
		}
		return true
	}

	// Simulate mapping completion.
	// In a real implementation, this would check the buffer state
	// via device polling (e.g., device.Poll()).

	// For now, simulate successful mapping by creating a slice.
	// In production, this would get the actual mapped pointer.
	b.mappedData = make([]byte, b.mapSize)
	b.mapState = BufferMapStateMapped

	// Invoke callback
	if b.mapCallback != nil {
		callback := b.mapCallback
		b.mapCallback = nil
		// Call callback outside lock to avoid deadlock
		b.mu.Unlock()
		callback(BufferMapAsyncStatusSuccess)
		b.mu.Lock()
	}

	return true
}

// GetMappedRange returns the mapped data slice.
//
// The returned slice is only valid while the buffer is mapped.
// Do not use the slice after calling Unmap().
//
// Parameters:
//   - offset: Byte offset within the mapped range
//   - size: Number of bytes to access
//
// Returns the data slice and nil on success.
// Returns nil and an error if:
//   - The buffer has been destroyed
//   - The buffer is not mapped
//   - The range is outside the mapped region
func (b *Buffer) GetMappedRange(offset, size uint64) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Check if destroyed
	if b.destroyed {
		return nil, ErrBufferDestroyed
	}

	// Check if mapped
	if b.mapState == BufferMapStatePending {
		return nil, ErrBufferMapPending
	}
	if b.mapState != BufferMapStateMapped {
		return nil, ErrBufferNotMapped
	}

	// Validate range is within mapped region
	// The offset and size are relative to the buffer, not the mapped region
	if offset < b.mapOffset {
		return nil, fmt.Errorf("%w: offset %d is before mapped region start %d",
			ErrInvalidMapRange, offset, b.mapOffset)
	}
	if offset+size > b.mapOffset+b.mapSize {
		return nil, fmt.Errorf("%w: offset %d + size %d exceeds mapped region end %d",
			ErrInvalidMapRange, offset, size, b.mapOffset+b.mapSize)
	}

	// Calculate slice indices within mappedData
	relOffset := offset - b.mapOffset
	return b.mappedData[relOffset : relOffset+size], nil
}

// Unmap unmaps the buffer, making changes visible to GPU.
//
// After unmapping, the buffer returns to the Unmapped state.
// Any slices returned by GetMappedRange become invalid.
//
// Returns nil on success.
// Returns an error if the buffer has been destroyed.
// If the buffer is already unmapped, this is a no-op.
func (b *Buffer) Unmap() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if destroyed
	if b.destroyed {
		return ErrBufferDestroyed
	}

	// If pending, cancel and invoke callback
	if b.mapState == BufferMapStatePending {
		if b.mapCallback != nil {
			callback := b.mapCallback
			b.mapCallback = nil
			b.mapState = BufferMapStateUnmapped
			b.mappedData = nil
			// Call callback outside lock
			b.mu.Unlock()
			callback(BufferMapAsyncStatusUnmappedBeforeCallback)
			b.mu.Lock()
		}
		return nil
	}

	// If not mapped, nothing to do
	if b.mapState != BufferMapStateMapped {
		return nil
	}

	// Note: In a real implementation, this would call buffer unmap
	// to flush changes to GPU memory.

	// Clear state
	b.mapState = BufferMapStateUnmapped
	b.mappedData = nil
	b.mapCallback = nil

	return nil
}

// Destroy releases the buffer and any associated resources.
//
// After calling Destroy(), the buffer should not be used.
// If the buffer is mapped, it will be unmapped first.
//
// This method is idempotent - calling it multiple times is safe.
func (b *Buffer) Destroy() {
	b.mu.Lock()
	if b.destroyed {
		b.mu.Unlock()
		return
	}
	b.destroyed = true
	device := b.device
	halBuf := b.halBuffer
	callback := b.mapCallback
	wasMapping := b.mapState == BufferMapStatePending
	b.halBuffer = nil
	b.mappedData = nil
	b.mapCallback = nil
	b.mapState = BufferMapStateUnmapped
	b.mu.Unlock()

	// Invoke pending callback if any
	if wasMapping && callback != nil {
		callback(BufferMapAsyncStatusDestroyedBeforeCallback)
	}

	// Destroy the buffer
	if device != nil && halBuf != nil {
		device.DestroyBuffer(halBuf)
	}
}

// =============================================================================
// Device Buffer Creation
// =============================================================================

// CreateBuffer creates a new buffer from a device.
//
// This is a helper function for creating buffers using the HAL API directly.
// It handles validation and wraps the buffer in a Buffer.
//
// Parameters:
//   - device: The device to create the buffer on.
//   - desc: The buffer descriptor.
//
// Returns the new Buffer and nil on success.
// Returns nil and an error if:
//   - The device is nil
//   - The descriptor is nil
//   - Buffer size is invalid
//   - Buffer creation fails
func CreateBuffer(device hal.Device, desc *BufferDescriptor) (*Buffer, error) {
	if device == nil {
		return nil, ErrNilHALDevice
	}

	if desc == nil {
		return nil, fmt.Errorf("buffer descriptor is nil")
	}

	// Validate size
	if desc.Size == 0 {
		return nil, fmt.Errorf("%w: size is 0", ErrInvalidBufferSize)
	}

	// Validate usage
	if desc.Usage == 0 {
		return nil, fmt.Errorf("buffer usage is empty")
	}

	// Validate MappedAtCreation requires MapWrite usage
	if desc.MappedAtCreation {
		if !desc.Usage.Contains(gputypes.BufferUsageMapWrite) &&
			!desc.Usage.Contains(gputypes.BufferUsageCopyDst) {
			return nil, fmt.Errorf("MappedAtCreation requires MapWrite or CopyDst usage")
		}
	}

	// Calculate aligned size (align to 4 bytes for copy operations)
	const copyBufferAlignment uint64 = 4
	alignedSize := (desc.Size + copyBufferAlignment - 1) &^ (copyBufferAlignment - 1)

	// Convert to descriptor
	halDesc := &hal.BufferDescriptor{
		Label:            desc.Label,
		Size:             alignedSize,
		Usage:            desc.Usage,
		MappedAtCreation: desc.MappedAtCreation,
	}

	// Create buffer
	halBuffer, err := device.CreateBuffer(halDesc)
	if err != nil {
		return nil, fmt.Errorf("buffer creation failed: %w", err)
	}

	// Update descriptor with aligned size
	resolvedDesc := *desc
	resolvedDesc.Size = alignedSize

	return NewBuffer(halBuffer, device, &resolvedDesc), nil
}

// CreateBufferSimple creates a buffer with common defaults.
//
// This is a convenience function for creating simple buffers.
//
// Parameters:
//   - device: The device to create the buffer on.
//   - size: Buffer size in bytes.
//   - usage: Buffer usage flags.
//   - label: Optional debug label.
//
// Returns the new Buffer and nil on success.
// Returns nil and an error if creation fails.
func CreateBufferSimple(
	device hal.Device,
	size uint64,
	usage gputypes.BufferUsage,
	label string,
) (*Buffer, error) {
	desc := &BufferDescriptor{
		Label: label,
		Size:  size,
		Usage: usage,
	}

	return CreateBuffer(device, desc)
}

// CreateStagingBuffer creates a staging buffer for CPU-GPU data transfer.
//
// Staging buffers are used to transfer data between CPU and GPU:
//   - For uploads: Create with MapWrite | CopySrc, map, write, copy to GPU buffer
//   - For readback: Create with MapRead | CopyDst, copy from GPU, map, read
//
// Parameters:
//   - device: The device to create the buffer on.
//   - size: Buffer size in bytes.
//   - forUpload: If true, creates upload staging buffer (MapWrite | CopySrc).
//     If false, creates readback staging buffer (MapRead | CopyDst).
//   - label: Optional debug label.
//
// Returns the new Buffer and nil on success.
// Returns nil and an error if creation fails.
func CreateStagingBuffer(
	device hal.Device,
	size uint64,
	forUpload bool,
	label string,
) (*Buffer, error) {
	var usage gputypes.BufferUsage
	if forUpload {
		usage = gputypes.BufferUsageMapWrite | gputypes.BufferUsageCopySrc
	} else {
		usage = gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst
	}

	desc := &BufferDescriptor{
		Label:            label,
		Size:             size,
		Usage:            usage,
		MappedAtCreation: forUpload, // Pre-map upload buffers for convenience
	}

	return CreateBuffer(device, desc)
}
