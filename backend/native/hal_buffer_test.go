package native

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

// =============================================================================
// Mock HAL Buffer for Testing
// =============================================================================

// mockHALBuffer is a test double for hal.Buffer.
type mockHALBuffer struct {
	size   uint64
	usage  types.BufferUsage
	label  string
	mapped bool
}

// Destroy implements hal.Resource.
func (b *mockHALBuffer) Destroy() {}

// =============================================================================
// Extended mockHALDevice for Buffer Tests
// =============================================================================

// bufferMockHALDevice extends mockHALDevice with buffer creation.
type bufferMockHALDevice struct {
	mockHALDevice

	createBufferFunc  func(*hal.BufferDescriptor) (hal.Buffer, error)
	destroyBufferFunc func(hal.Buffer)

	buffersCreated   int32
	buffersDestroyed int32
}

func (d *bufferMockHALDevice) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	atomic.AddInt32(&d.buffersCreated, 1)
	if d.createBufferFunc != nil {
		return d.createBufferFunc(desc)
	}
	return &mockHALBuffer{
		size:   desc.Size,
		usage:  desc.Usage,
		label:  desc.Label,
		mapped: desc.MappedAtCreation,
	}, nil
}

func (d *bufferMockHALDevice) DestroyBuffer(buffer hal.Buffer) {
	atomic.AddInt32(&d.buffersDestroyed, 1)
	if d.destroyBufferFunc != nil {
		d.destroyBufferFunc(buffer)
	}
}

// =============================================================================
// HALBuffer Construction Tests
// =============================================================================

func TestNewHALBuffer(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageVertex}
	desc := &HALBufferDescriptor{
		Label: "test-buffer",
		Size:  1024,
		Usage: types.BufferUsageVertex,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	if buf == nil {
		t.Fatal("NewHALBuffer returned nil")
	}
	if buf.Label() != "test-buffer" {
		t.Errorf("Label = %q, want %q", buf.Label(), "test-buffer")
	}
	if buf.Size() != 1024 {
		t.Errorf("Size = %d, want 1024", buf.Size())
	}
	if buf.Usage() != types.BufferUsageVertex {
		t.Errorf("Usage = %v, want Vertex", buf.Usage())
	}
	if buf.IsDestroyed() {
		t.Error("IsDestroyed = true, want false")
	}
	if buf.MapState() != BufferMapStateUnmapped {
		t.Errorf("MapState = %v, want Unmapped", buf.MapState())
	}
}

func TestNewHALBuffer_MappedAtCreation(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 512, usage: types.BufferUsageMapWrite, mapped: true}
	desc := &HALBufferDescriptor{
		Label:            "mapped-buffer",
		Size:             512,
		Usage:            types.BufferUsageMapWrite,
		MappedAtCreation: true,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	if buf.MapState() != BufferMapStateMapped {
		t.Errorf("MapState = %v, want Mapped (MappedAtCreation)", buf.MapState())
	}
}

// =============================================================================
// MapAsync Tests
// =============================================================================

func TestHALBuffer_MapAsync_Success(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "map-test",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	var callbackInvoked bool
	var callbackStatus BufferMapAsyncStatus

	err := buf.MapAsync(types.MapModeRead, 0, 1024, func(status BufferMapAsyncStatus) {
		callbackInvoked = true
		callbackStatus = status
	})

	if err != nil {
		t.Fatalf("MapAsync failed: %v", err)
	}
	if buf.MapState() != BufferMapStatePending {
		t.Errorf("MapState = %v, want Pending", buf.MapState())
	}

	// Poll for completion
	complete := buf.PollMapAsync()
	if !complete {
		t.Error("PollMapAsync returned false, expected true")
	}
	if buf.MapState() != BufferMapStateMapped {
		t.Errorf("MapState = %v after poll, want Mapped", buf.MapState())
	}
	if !callbackInvoked {
		t.Error("Callback was not invoked")
	}
	if callbackStatus != BufferMapAsyncStatusSuccess {
		t.Errorf("Callback status = %v, want Success", callbackStatus)
	}
}

func TestHALBuffer_MapAsync_WriteMode(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 512, usage: types.BufferUsageMapWrite}
	desc := &HALBufferDescriptor{
		Label: "write-map-test",
		Size:  512,
		Usage: types.BufferUsageMapWrite,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	var status BufferMapAsyncStatus
	err := buf.MapAsync(types.MapModeWrite, 0, 512, func(s BufferMapAsyncStatus) {
		status = s
	})

	if err != nil {
		t.Fatalf("MapAsync (write) failed: %v", err)
	}

	buf.PollMapAsync()

	if status != BufferMapAsyncStatusSuccess {
		t.Errorf("Write map status = %v, want Success", status)
	}
}

func TestHALBuffer_MapAsync_AlreadyMapped(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label:            "already-mapped",
		Size:             1024,
		Usage:            types.BufferUsageMapRead | types.BufferUsageMapWrite,
		MappedAtCreation: true,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	var status BufferMapAsyncStatus
	err := buf.MapAsync(types.MapModeRead, 0, 1024, func(s BufferMapAsyncStatus) {
		status = s
	})

	if !errors.Is(err, ErrBufferAlreadyMapped) {
		t.Errorf("MapAsync on mapped buffer: got %v, want ErrBufferAlreadyMapped", err)
	}
	if status != BufferMapAsyncStatusMappingAlreadyPending {
		t.Errorf("Callback status = %v, want MappingAlreadyPending", status)
	}
}

func TestHALBuffer_MapAsync_UsageMismatch(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageVertex}
	desc := &HALBufferDescriptor{
		Label: "usage-mismatch",
		Size:  1024,
		Usage: types.BufferUsageVertex, // No MapRead or MapWrite
	}

	buf := NewHALBuffer(halBuf, device, desc)

	var status BufferMapAsyncStatus
	err := buf.MapAsync(types.MapModeRead, 0, 1024, func(s BufferMapAsyncStatus) {
		status = s
	})

	if err == nil {
		t.Error("MapAsync should fail with usage mismatch")
	}
	if status != BufferMapAsyncStatusValidationError {
		t.Errorf("Callback status = %v, want ValidationError", status)
	}
}

func TestHALBuffer_MapAsync_RangeValidation(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "range-test",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	tests := []struct {
		name           string
		offset         uint64
		size           uint64
		wantErr        bool
		expectedStatus BufferMapAsyncStatus
	}{
		{"valid full", 0, 1024, false, BufferMapAsyncStatusSuccess},
		{"valid partial", 256, 512, false, BufferMapAsyncStatusSuccess},
		{"offset out of range", 2000, 100, true, BufferMapAsyncStatusOffsetOutOfRange},
		{"size out of range", 0, 2000, true, BufferMapAsyncStatusSizeOutOfRange},
		{"offset+size overflow", 512, 600, true, BufferMapAsyncStatusSizeOutOfRange},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset buffer state
			buf.mu.Lock()
			buf.mapState = BufferMapStateUnmapped
			buf.mappedData = nil
			buf.mu.Unlock()

			var status BufferMapAsyncStatus
			err := buf.MapAsync(types.MapModeRead, tt.offset, tt.size, func(s BufferMapAsyncStatus) {
				status = s
			})

			if tt.wantErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if status != tt.expectedStatus {
				// If not wantErr, poll to get the status
				if !tt.wantErr {
					buf.PollMapAsync()
				}
			}
		})
	}
}

func TestHALBuffer_MapAsync_NilCallback(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "nil-callback",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	err := buf.MapAsync(types.MapModeRead, 0, 1024, nil)

	if !errors.Is(err, ErrCallbackNil) {
		t.Errorf("MapAsync with nil callback: got %v, want ErrCallbackNil", err)
	}
}

func TestHALBuffer_MapAsync_AfterDestroy(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "destroyed",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)
	buf.Destroy()

	err := buf.MapAsync(types.MapModeRead, 0, 1024, func(_ BufferMapAsyncStatus) {})

	if !errors.Is(err, ErrBufferDestroyed) {
		t.Errorf("MapAsync after destroy: got %v, want ErrBufferDestroyed", err)
	}
}

// =============================================================================
// GetMappedRange Tests
// =============================================================================

func TestHALBuffer_GetMappedRange_Success(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "range-test",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	// Map the buffer
	_ = buf.MapAsync(types.MapModeRead, 0, 1024, func(_ BufferMapAsyncStatus) {})
	buf.PollMapAsync()

	// Get mapped range
	data, err := buf.GetMappedRange(0, 512)
	if err != nil {
		t.Fatalf("GetMappedRange failed: %v", err)
	}
	if len(data) != 512 {
		t.Errorf("Got %d bytes, want 512", len(data))
	}
}

func TestHALBuffer_GetMappedRange_PartialMap(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "partial-map",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	// Map partial range (256 to 768)
	_ = buf.MapAsync(types.MapModeRead, 256, 512, func(_ BufferMapAsyncStatus) {})
	buf.PollMapAsync()

	// Valid access within mapped region
	data, err := buf.GetMappedRange(256, 256)
	if err != nil {
		t.Fatalf("GetMappedRange (within) failed: %v", err)
	}
	if len(data) != 256 {
		t.Errorf("Got %d bytes, want 256", len(data))
	}

	// Access outside mapped region should fail
	_, err = buf.GetMappedRange(0, 256)
	if err == nil {
		t.Error("GetMappedRange outside mapped region should fail")
	}
}

func TestHALBuffer_GetMappedRange_NotMapped(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "not-mapped",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	_, err := buf.GetMappedRange(0, 512)

	if !errors.Is(err, ErrBufferNotMapped) {
		t.Errorf("GetMappedRange on unmapped buffer: got %v, want ErrBufferNotMapped", err)
	}
}

func TestHALBuffer_GetMappedRange_Pending(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "pending",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	// Start mapping but don't poll
	_ = buf.MapAsync(types.MapModeRead, 0, 1024, func(_ BufferMapAsyncStatus) {})

	_, err := buf.GetMappedRange(0, 512)

	if !errors.Is(err, ErrBufferMapPending) {
		t.Errorf("GetMappedRange while pending: got %v, want ErrBufferMapPending", err)
	}
}

// =============================================================================
// Unmap Tests
// =============================================================================

func TestHALBuffer_Unmap_Success(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "unmap-test",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	// Map and poll
	_ = buf.MapAsync(types.MapModeRead, 0, 1024, func(_ BufferMapAsyncStatus) {})
	buf.PollMapAsync()

	// Unmap
	err := buf.Unmap()
	if err != nil {
		t.Fatalf("Unmap failed: %v", err)
	}
	if buf.MapState() != BufferMapStateUnmapped {
		t.Errorf("MapState after Unmap = %v, want Unmapped", buf.MapState())
	}
}

func TestHALBuffer_Unmap_Pending(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "unmap-pending",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	var callbackStatus BufferMapAsyncStatus
	_ = buf.MapAsync(types.MapModeRead, 0, 1024, func(s BufferMapAsyncStatus) {
		callbackStatus = s
	})

	// Unmap while pending
	err := buf.Unmap()
	if err != nil {
		t.Fatalf("Unmap pending failed: %v", err)
	}
	if callbackStatus != BufferMapAsyncStatusUnmappedBeforeCallback {
		t.Errorf("Callback status = %v, want UnmappedBeforeCallback", callbackStatus)
	}
	if buf.MapState() != BufferMapStateUnmapped {
		t.Errorf("MapState = %v, want Unmapped", buf.MapState())
	}
}

func TestHALBuffer_Unmap_AlreadyUnmapped(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "already-unmapped",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	// Unmap when already unmapped should be a no-op
	err := buf.Unmap()
	if err != nil {
		t.Errorf("Unmap on unmapped buffer: got %v, want nil", err)
	}
}

func TestHALBuffer_Unmap_AfterDestroy(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "unmap-destroyed",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)
	buf.Destroy()

	err := buf.Unmap()

	if !errors.Is(err, ErrBufferDestroyed) {
		t.Errorf("Unmap after destroy: got %v, want ErrBufferDestroyed", err)
	}
}

// =============================================================================
// Destroy Tests
// =============================================================================

func TestHALBuffer_Destroy(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageVertex}
	desc := &HALBufferDescriptor{
		Label: "destroy-test",
		Size:  1024,
		Usage: types.BufferUsageVertex,
	}

	buf := NewHALBuffer(halBuf, device, desc)
	buf.Destroy()

	if !buf.IsDestroyed() {
		t.Error("IsDestroyed = false after Destroy()")
	}
	if buf.Raw() != nil {
		t.Error("Raw() should return nil after Destroy()")
	}
	if device.buffersDestroyed != 1 {
		t.Errorf("buffersDestroyed = %d, want 1", device.buffersDestroyed)
	}
}

func TestHALBuffer_Destroy_Idempotent(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageVertex}
	desc := &HALBufferDescriptor{
		Label: "idempotent-destroy",
		Size:  1024,
		Usage: types.BufferUsageVertex,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	// Destroy multiple times
	buf.Destroy()
	buf.Destroy()
	buf.Destroy()

	// Should only destroy once
	if device.buffersDestroyed != 1 {
		t.Errorf("buffersDestroyed = %d, want 1", device.buffersDestroyed)
	}
}

func TestHALBuffer_Destroy_WhilePending(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "destroy-pending",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	var callbackStatus BufferMapAsyncStatus
	_ = buf.MapAsync(types.MapModeRead, 0, 1024, func(s BufferMapAsyncStatus) {
		callbackStatus = s
	})

	// Destroy while map is pending
	buf.Destroy()

	if callbackStatus != BufferMapAsyncStatusDestroyedBeforeCallback {
		t.Errorf("Callback status = %v, want DestroyedBeforeCallback", callbackStatus)
	}
	if device.buffersDestroyed != 1 {
		t.Errorf("buffersDestroyed = %d, want 1", device.buffersDestroyed)
	}
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestHALBuffer_ConcurrentMapUnmap(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead | types.BufferUsageMapWrite}
	desc := &HALBufferDescriptor{
		Label: "concurrent",
		Size:  1024,
		Usage: types.BufferUsageMapRead | types.BufferUsageMapWrite,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	const numOps = 100
	var wg sync.WaitGroup

	// Concurrent map/unmap operations
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = buf.MapAsync(types.MapModeRead, 0, 1024, func(_ BufferMapAsyncStatus) {})
			buf.PollMapAsync()
			_ = buf.Unmap()
		}()
	}

	wg.Wait()

	// Buffer should be in a consistent state
	state := buf.MapState()
	if state != BufferMapStateUnmapped && state != BufferMapStateMapped && state != BufferMapStatePending {
		t.Errorf("Buffer in invalid state: %v", state)
	}
}

func TestHALBuffer_ConcurrentGetMappedRange(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "concurrent-read",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	// Map the buffer
	_ = buf.MapAsync(types.MapModeRead, 0, 1024, func(_ BufferMapAsyncStatus) {})
	buf.PollMapAsync()

	const numReaders = 10
	var wg sync.WaitGroup
	errs := make([]error, numReaders)

	// Concurrent reads should all succeed
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = buf.GetMappedRange(0, 512)
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Reader %d got error: %v", i, err)
		}
	}
}

// =============================================================================
// CreateHALBuffer Tests
// =============================================================================

func TestCreateHALBuffer(t *testing.T) {
	device := &bufferMockHALDevice{}
	desc := &HALBufferDescriptor{
		Label: "created-buffer",
		Size:  1024,
		Usage: types.BufferUsageVertex | types.BufferUsageCopyDst,
	}

	buf, err := CreateHALBuffer(device, desc)
	if err != nil {
		t.Fatalf("CreateHALBuffer failed: %v", err)
	}
	if buf == nil {
		t.Fatal("CreateHALBuffer returned nil")
	}
	if buf.Label() != "created-buffer" {
		t.Errorf("Label = %q, want %q", buf.Label(), "created-buffer")
	}
	if device.buffersCreated != 1 {
		t.Errorf("buffersCreated = %d, want 1", device.buffersCreated)
	}
}

func TestCreateHALBuffer_NilDevice(t *testing.T) {
	desc := &HALBufferDescriptor{
		Label: "test",
		Size:  1024,
		Usage: types.BufferUsageVertex,
	}

	_, err := CreateHALBuffer(nil, desc)
	if !errors.Is(err, ErrNilHALDevice) {
		t.Errorf("CreateHALBuffer(nil device): got %v, want ErrNilHALDevice", err)
	}
}

func TestCreateHALBuffer_NilDescriptor(t *testing.T) {
	device := &bufferMockHALDevice{}

	_, err := CreateHALBuffer(device, nil)
	if err == nil {
		t.Error("CreateHALBuffer(nil desc) should fail")
	}
}

func TestCreateHALBuffer_ZeroSize(t *testing.T) {
	device := &bufferMockHALDevice{}
	desc := &HALBufferDescriptor{
		Label: "zero-size",
		Size:  0,
		Usage: types.BufferUsageVertex,
	}

	_, err := CreateHALBuffer(device, desc)
	if err == nil {
		t.Error("CreateHALBuffer with zero size should fail")
	}
}

func TestCreateHALBuffer_ZeroUsage(t *testing.T) {
	device := &bufferMockHALDevice{}
	desc := &HALBufferDescriptor{
		Label: "zero-usage",
		Size:  1024,
		Usage: 0,
	}

	_, err := CreateHALBuffer(device, desc)
	if err == nil {
		t.Error("CreateHALBuffer with zero usage should fail")
	}
}

func TestCreateHALBuffer_SizeAlignment(t *testing.T) {
	device := &bufferMockHALDevice{}
	desc := &HALBufferDescriptor{
		Label: "alignment-test",
		Size:  1001, // Not aligned to 4 bytes
		Usage: types.BufferUsageVertex,
	}

	buf, err := CreateHALBuffer(device, desc)
	if err != nil {
		t.Fatalf("CreateHALBuffer failed: %v", err)
	}

	// Size should be aligned up to 4 bytes
	if buf.Size() != 1004 {
		t.Errorf("Size = %d, want 1004 (aligned from 1001)", buf.Size())
	}
}

func TestCreateHALBufferSimple(t *testing.T) {
	device := &bufferMockHALDevice{}

	buf, err := CreateHALBufferSimple(device, 2048, types.BufferUsageStorage, "simple-buffer")
	if err != nil {
		t.Fatalf("CreateHALBufferSimple failed: %v", err)
	}
	if buf.Size() != 2048 {
		t.Errorf("Size = %d, want 2048", buf.Size())
	}
	if buf.Usage() != types.BufferUsageStorage {
		t.Errorf("Usage = %v, want Storage", buf.Usage())
	}
	if buf.Label() != "simple-buffer" {
		t.Errorf("Label = %q, want %q", buf.Label(), "simple-buffer")
	}
}

func TestCreateStagingBuffer_Upload(t *testing.T) {
	device := &bufferMockHALDevice{}

	buf, err := CreateStagingBuffer(device, 4096, true, "upload-staging")
	if err != nil {
		t.Fatalf("CreateStagingBuffer (upload) failed: %v", err)
	}

	expectedUsage := types.BufferUsageMapWrite | types.BufferUsageCopySrc
	if buf.Usage() != expectedUsage {
		t.Errorf("Usage = %v, want MapWrite|CopySrc", buf.Usage())
	}

	// Should be pre-mapped for uploads
	if buf.MapState() != BufferMapStateMapped {
		t.Errorf("MapState = %v, want Mapped (pre-mapped for upload)", buf.MapState())
	}
}

func TestCreateStagingBuffer_Readback(t *testing.T) {
	device := &bufferMockHALDevice{}

	buf, err := CreateStagingBuffer(device, 4096, false, "readback-staging")
	if err != nil {
		t.Fatalf("CreateStagingBuffer (readback) failed: %v", err)
	}

	expectedUsage := types.BufferUsageMapRead | types.BufferUsageCopyDst
	if buf.Usage() != expectedUsage {
		t.Errorf("Usage = %v, want MapRead|CopyDst", buf.Usage())
	}

	// Readback buffers are not pre-mapped
	if buf.MapState() != BufferMapStateUnmapped {
		t.Errorf("MapState = %v, want Unmapped (readback not pre-mapped)", buf.MapState())
	}
}

// =============================================================================
// State String Tests
// =============================================================================

func TestBufferMapState_String(t *testing.T) {
	tests := []struct {
		state BufferMapState
		want  string
	}{
		{BufferMapStateUnmapped, "Unmapped"},
		{BufferMapStatePending, "Pending"},
		{BufferMapStateMapped, "Mapped"},
		{BufferMapState(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("BufferMapState(%d).String() = %q, want %q", int(tt.state), got, tt.want)
		}
	}
}

func TestBufferMapAsyncStatus_String(t *testing.T) {
	tests := []struct {
		status BufferMapAsyncStatus
		want   string
	}{
		{BufferMapAsyncStatusSuccess, "Success"},
		{BufferMapAsyncStatusValidationError, "ValidationError"},
		{BufferMapAsyncStatusUnknown, "Unknown"},
		{BufferMapAsyncStatusDeviceLost, "DeviceLost"},
		{BufferMapAsyncStatusDestroyedBeforeCallback, "DestroyedBeforeCallback"},
		{BufferMapAsyncStatusUnmappedBeforeCallback, "UnmappedBeforeCallback"},
		{BufferMapAsyncStatusMappingAlreadyPending, "MappingAlreadyPending"},
		{BufferMapAsyncStatusOffsetOutOfRange, "OffsetOutOfRange"},
		{BufferMapAsyncStatusSizeOutOfRange, "SizeOutOfRange"},
		{BufferMapAsyncStatus(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("BufferMapAsyncStatus(%d).String() = %q, want %q", int(tt.status), got, tt.want)
		}
	}
}

// =============================================================================
// Additional Coverage Tests
// =============================================================================

func TestHALBuffer_Descriptor(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 2048, usage: types.BufferUsageUniform}
	desc := &HALBufferDescriptor{
		Label: "descriptor-test",
		Size:  2048,
		Usage: types.BufferUsageUniform,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	got := buf.Descriptor()
	if got.Label != desc.Label {
		t.Errorf("Descriptor().Label = %q, want %q", got.Label, desc.Label)
	}
	if got.Size != desc.Size {
		t.Errorf("Descriptor().Size = %d, want %d", got.Size, desc.Size)
	}
	if got.Usage != desc.Usage {
		t.Errorf("Descriptor().Usage = %v, want %v", got.Usage, desc.Usage)
	}
}

func TestHALBuffer_Raw_AfterDestroy(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageVertex}
	desc := &HALBufferDescriptor{
		Label: "raw-destroy-test",
		Size:  1024,
		Usage: types.BufferUsageVertex,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	// Raw should return the buffer
	if buf.Raw() == nil {
		t.Error("Raw() should not return nil before destroy")
	}

	buf.Destroy()

	// Raw should return nil after destroy
	if buf.Raw() != nil {
		t.Error("Raw() should return nil after destroy")
	}
}

func TestHALBuffer_MapAsync_InvalidMode(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "invalid-mode",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	var status BufferMapAsyncStatus
	err := buf.MapAsync(0, 0, 1024, func(s BufferMapAsyncStatus) {
		status = s
	})

	if err == nil {
		t.Error("MapAsync with mode 0 should fail")
	}
	if status != BufferMapAsyncStatusValidationError {
		t.Errorf("Callback status = %v, want ValidationError", status)
	}
}

func TestHALBuffer_PollMapAsync_NotPending(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "poll-not-pending",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	// Poll when not pending should return true (nothing to wait for)
	if !buf.PollMapAsync() {
		t.Error("PollMapAsync when unmapped should return true")
	}
}

func TestHALBuffer_PollMapAsync_DestroyedDuringPending(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "poll-destroyed",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	var callbackStatus BufferMapAsyncStatus
	var callbackCalled bool

	// Start mapping
	_ = buf.MapAsync(types.MapModeRead, 0, 1024, func(s BufferMapAsyncStatus) {
		callbackStatus = s
		callbackCalled = true
	})

	// Manually set to destroyed while pending (simulates race condition)
	buf.mu.Lock()
	buf.destroyed = true
	buf.mu.Unlock()

	// Poll should detect destroyed state and invoke callback
	complete := buf.PollMapAsync()
	if !complete {
		t.Error("PollMapAsync should return true when destroyed")
	}
	if !callbackCalled {
		t.Error("Callback should be invoked when destroyed during pending")
	}
	if callbackStatus != BufferMapAsyncStatusDestroyedBeforeCallback {
		t.Errorf("Callback status = %v, want DestroyedBeforeCallback", callbackStatus)
	}
}

func TestHALBuffer_GetMappedRange_AfterDestroy(t *testing.T) {
	device := &bufferMockHALDevice{}
	halBuf := &mockHALBuffer{size: 1024, usage: types.BufferUsageMapRead}
	desc := &HALBufferDescriptor{
		Label: "range-destroy",
		Size:  1024,
		Usage: types.BufferUsageMapRead,
	}

	buf := NewHALBuffer(halBuf, device, desc)

	// Map and poll
	_ = buf.MapAsync(types.MapModeRead, 0, 1024, func(_ BufferMapAsyncStatus) {})
	buf.PollMapAsync()

	// Destroy
	buf.Destroy()

	// GetMappedRange should fail
	_, err := buf.GetMappedRange(0, 512)
	if !errors.Is(err, ErrBufferDestroyed) {
		t.Errorf("GetMappedRange after destroy: got %v, want ErrBufferDestroyed", err)
	}
}

func TestCreateHALBuffer_MappedAtCreationValidation(t *testing.T) {
	device := &bufferMockHALDevice{}
	desc := &HALBufferDescriptor{
		Label:            "mapped-validation",
		Size:             1024,
		Usage:            types.BufferUsageVertex, // No MapWrite or CopyDst
		MappedAtCreation: true,
	}

	_, err := CreateHALBuffer(device, desc)
	if err == nil {
		t.Error("CreateHALBuffer with MappedAtCreation but no MapWrite/CopyDst should fail")
	}
}

func TestCreateHALBuffer_HALError(t *testing.T) {
	device := &bufferMockHALDevice{
		createBufferFunc: func(_ *hal.BufferDescriptor) (hal.Buffer, error) {
			return nil, errors.New("HAL creation failed")
		},
	}
	desc := &HALBufferDescriptor{
		Label: "hal-error",
		Size:  1024,
		Usage: types.BufferUsageVertex,
	}

	_, err := CreateHALBuffer(device, desc)
	if err == nil {
		t.Error("CreateHALBuffer should fail when HAL creation fails")
	}
}
