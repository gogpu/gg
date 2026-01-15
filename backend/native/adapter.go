//go:build !nogpu

// Package native provides a Pure Go GPU-accelerated rendering backend using gogpu/wgpu.
package native

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gg/gpucore"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

// HALAdapter implements gpucore.GPUAdapter using gogpu/wgpu/hal directly.
// It provides a bridge between the gpucore abstraction and the HAL layer.
//
// Thread Safety: HALAdapter is safe for concurrent use from multiple goroutines.
// All resource operations are protected by a mutex.
type HALAdapter struct {
	mu     sync.RWMutex
	device hal.Device
	queue  hal.Queue

	// Adapter limits and capabilities
	limits       types.Limits
	hasCompute   bool
	maxBufferSz  uint64
	maxWorkgroup [3]uint32

	// ID generation
	nextID atomic.Uint64

	// Resource tracking maps gpucore IDs to hal resources
	buffers          map[gpucore.BufferID]hal.Buffer
	textures         map[gpucore.TextureID]hal.Texture
	shaderModules    map[gpucore.ShaderModuleID]hal.ShaderModule
	computePipelines map[gpucore.ComputePipelineID]hal.ComputePipeline
	bindGroupLayouts map[gpucore.BindGroupLayoutID]hal.BindGroupLayout
	pipelineLayouts  map[gpucore.PipelineLayoutID]hal.PipelineLayout
	bindGroups       map[gpucore.BindGroupID]hal.BindGroup

	// Command encoder for current frame
	encoder    hal.CommandEncoder
	hasEncoder bool
}

// NewHALAdapter creates a new HALAdapter wrapping the given device and queue.
// The limits parameter provides the adapter's capability limits.
// If limits is nil, default limits are used.
func NewHALAdapter(device hal.Device, queue hal.Queue, limits *types.Limits) *HALAdapter {
	var lim types.Limits
	if limits != nil {
		lim = *limits
	} else {
		lim = types.DefaultLimits()
	}

	adapter := &HALAdapter{
		device:           device,
		queue:            queue,
		limits:           lim,
		hasCompute:       true, // Assume compute support by default
		maxBufferSz:      lim.MaxBufferSize,
		maxWorkgroup:     [3]uint32{lim.MaxComputeWorkgroupSizeX, lim.MaxComputeWorkgroupSizeY, lim.MaxComputeWorkgroupSizeZ},
		buffers:          make(map[gpucore.BufferID]hal.Buffer),
		textures:         make(map[gpucore.TextureID]hal.Texture),
		shaderModules:    make(map[gpucore.ShaderModuleID]hal.ShaderModule),
		computePipelines: make(map[gpucore.ComputePipelineID]hal.ComputePipeline),
		bindGroupLayouts: make(map[gpucore.BindGroupLayoutID]hal.BindGroupLayout),
		pipelineLayouts:  make(map[gpucore.PipelineLayoutID]hal.PipelineLayout),
		bindGroups:       make(map[gpucore.BindGroupID]hal.BindGroup),
	}

	// Start ID generation at 1 (0 is invalid)
	adapter.nextID.Store(1)

	return adapter
}

// newID generates a unique resource ID.
func (a *HALAdapter) newID() uint64 {
	return a.nextID.Add(1) - 1
}

// === Capabilities ===

// SupportsCompute returns whether compute shaders are supported.
func (a *HALAdapter) SupportsCompute() bool {
	return a.hasCompute
}

// MaxWorkgroupSize returns the maximum workgroup size in each dimension.
func (a *HALAdapter) MaxWorkgroupSize() [3]uint32 {
	return a.maxWorkgroup
}

// MaxBufferSize returns the maximum buffer size in bytes.
func (a *HALAdapter) MaxBufferSize() uint64 {
	return a.maxBufferSz
}

// === Shader Compilation ===

// CreateShaderModule creates a shader module from SPIR-V bytecode.
func (a *HALAdapter) CreateShaderModule(spirv []uint32, label string) (gpucore.ShaderModuleID, error) {
	if len(spirv) == 0 {
		return gpucore.InvalidID, fmt.Errorf("empty SPIR-V bytecode")
	}

	desc := &hal.ShaderModuleDescriptor{
		Label: label,
		Source: hal.ShaderSource{
			SPIRV: spirv,
		},
	}

	module, err := a.device.CreateShaderModule(desc)
	if err != nil {
		return gpucore.InvalidID, fmt.Errorf("failed to create shader module: %w", err)
	}

	id := gpucore.ShaderModuleID(a.newID())

	a.mu.Lock()
	a.shaderModules[id] = module
	a.mu.Unlock()

	return id, nil
}

// DestroyShaderModule releases a shader module.
func (a *HALAdapter) DestroyShaderModule(id gpucore.ShaderModuleID) {
	a.mu.Lock()
	module, ok := a.shaderModules[id]
	if ok {
		delete(a.shaderModules, id)
	}
	a.mu.Unlock()

	if ok {
		a.device.DestroyShaderModule(module)
	}
}

// === Buffer Management ===

// CreateBuffer creates a GPU buffer.
func (a *HALAdapter) CreateBuffer(size int, usage gpucore.BufferUsage) (gpucore.BufferID, error) {
	if size <= 0 {
		return gpucore.InvalidID, fmt.Errorf("buffer size must be positive")
	}

	desc := &hal.BufferDescriptor{
		Label: "",
		Size:  uint64(size),
		Usage: convertBufferUsage(usage),
	}

	buffer, err := a.device.CreateBuffer(desc)
	if err != nil {
		return gpucore.InvalidID, fmt.Errorf("failed to create buffer: %w", err)
	}

	id := gpucore.BufferID(a.newID())

	a.mu.Lock()
	a.buffers[id] = buffer
	a.mu.Unlock()

	return id, nil
}

// DestroyBuffer releases a GPU buffer.
func (a *HALAdapter) DestroyBuffer(id gpucore.BufferID) {
	a.mu.Lock()
	buffer, ok := a.buffers[id]
	if ok {
		delete(a.buffers, id)
	}
	a.mu.Unlock()

	if ok {
		a.device.DestroyBuffer(buffer)
	}
}

// WriteBuffer writes data to a buffer.
func (a *HALAdapter) WriteBuffer(id gpucore.BufferID, offset uint64, data []byte) {
	a.mu.RLock()
	buffer, ok := a.buffers[id]
	a.mu.RUnlock()

	if ok && len(data) > 0 {
		a.queue.WriteBuffer(buffer, offset, data)
	}
}

// ReadBuffer reads data from a buffer.
// This operation requires a staging buffer and GPU-CPU synchronization.
func (a *HALAdapter) ReadBuffer(id gpucore.BufferID, offset, size uint64) ([]byte, error) {
	a.mu.RLock()
	buffer, ok := a.buffers[id]
	a.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("buffer %d not found", id)
	}

	// Create a staging buffer for readback
	stagingDesc := &hal.BufferDescriptor{
		Label:            "staging-readback",
		Size:             size,
		Usage:            types.BufferUsageMapRead | types.BufferUsageCopyDst,
		MappedAtCreation: true,
	}

	stagingBuffer, err := a.device.CreateBuffer(stagingDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to create staging buffer: %w", err)
	}
	defer a.device.DestroyBuffer(stagingBuffer)

	// Create command encoder for copy
	encoder, err := a.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "buffer-read-encoder",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create command encoder: %w", err)
	}

	if err := encoder.BeginEncoding("buffer-read"); err != nil {
		return nil, fmt.Errorf("failed to begin encoding: %w", err)
	}

	// Copy from source buffer to staging buffer
	encoder.CopyBufferToBuffer(buffer, stagingBuffer, []hal.BufferCopy{
		{
			SrcOffset: offset,
			DstOffset: 0,
			Size:      size,
		},
	})

	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		return nil, fmt.Errorf("failed to end encoding: %w", err)
	}
	defer cmdBuffer.Destroy()

	// Submit and wait
	fence, err := a.device.CreateFence()
	if err != nil {
		return nil, fmt.Errorf("failed to create fence: %w", err)
	}
	defer a.device.DestroyFence(fence)

	if err := a.queue.Submit([]hal.CommandBuffer{cmdBuffer}, fence, 1); err != nil {
		return nil, fmt.Errorf("failed to submit commands: %w", err)
	}

	// Wait for completion (timeout 5 seconds)
	_, err = a.device.Wait(fence, 1, 5_000_000_000)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for fence: %w", err)
	}

	// TODO: Actual buffer mapping is not yet implemented in HAL
	// For now, return empty data as placeholder
	return make([]byte, size), nil
}

// === Texture Management ===

// CreateTexture creates a GPU texture.
func (a *HALAdapter) CreateTexture(width, height int, format gpucore.TextureFormat) (gpucore.TextureID, error) {
	if width <= 0 || height <= 0 {
		return gpucore.InvalidID, fmt.Errorf("texture dimensions must be positive")
	}

	desc := &hal.TextureDescriptor{
		Label: "",
		Size: hal.Extent3D{
			Width:              uint32(width),
			Height:             uint32(height),
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     types.TextureDimension2D,
		Format:        convertTextureFormat(format),
		Usage:         types.TextureUsageCopySrc | types.TextureUsageCopyDst | types.TextureUsageStorageBinding,
	}

	texture, err := a.device.CreateTexture(desc)
	if err != nil {
		return gpucore.InvalidID, fmt.Errorf("failed to create texture: %w", err)
	}

	id := gpucore.TextureID(a.newID())

	a.mu.Lock()
	a.textures[id] = texture
	a.mu.Unlock()

	return id, nil
}

// DestroyTexture releases a GPU texture.
func (a *HALAdapter) DestroyTexture(id gpucore.TextureID) {
	a.mu.Lock()
	texture, ok := a.textures[id]
	if ok {
		delete(a.textures, id)
	}
	a.mu.Unlock()

	if ok {
		a.device.DestroyTexture(texture)
	}
}

// WriteTexture writes data to a texture.
func (a *HALAdapter) WriteTexture(id gpucore.TextureID, data []byte) {
	a.mu.RLock()
	texture, ok := a.textures[id]
	a.mu.RUnlock()

	if !ok || len(data) == 0 {
		return
	}

	// TODO: Need texture dimensions to properly set up the write
	// For now, assume RGBA8 format and calculate dimensions from data size
	// This is a simplified implementation

	dst := &hal.ImageCopyTexture{
		Texture:  texture,
		MipLevel: 0,
		Origin:   hal.Origin3D{X: 0, Y: 0, Z: 0},
		Aspect:   types.TextureAspectAll,
	}

	// Placeholder dimensions - real implementation needs texture metadata
	layout := &hal.ImageDataLayout{
		Offset:       0,
		BytesPerRow:  0, // Will be calculated based on texture width
		RowsPerImage: 0,
	}

	size := &hal.Extent3D{
		Width:              0, // Placeholder
		Height:             0,
		DepthOrArrayLayers: 1,
	}

	a.queue.WriteTexture(dst, data, layout, size)
}

// ReadTexture reads data from a texture.
func (a *HALAdapter) ReadTexture(id gpucore.TextureID) ([]byte, error) {
	a.mu.RLock()
	_, ok := a.textures[id]
	a.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("texture %d not found", id)
	}

	// TODO: Texture readback requires knowing texture dimensions
	// and setting up proper staging buffers
	return nil, fmt.Errorf("texture readback not yet implemented")
}

// === Pipeline Management ===

// CreateBindGroupLayout creates a bind group layout.
func (a *HALAdapter) CreateBindGroupLayout(desc *gpucore.BindGroupLayoutDesc) (gpucore.BindGroupLayoutID, error) {
	if desc == nil {
		return gpucore.InvalidID, fmt.Errorf("nil bind group layout descriptor")
	}

	halEntries := make([]types.BindGroupLayoutEntry, len(desc.Entries))
	for i, entry := range desc.Entries {
		halEntries[i] = convertBindGroupLayoutEntry(entry)
	}

	halDesc := &hal.BindGroupLayoutDescriptor{
		Label:   desc.Label,
		Entries: halEntries,
	}

	layout, err := a.device.CreateBindGroupLayout(halDesc)
	if err != nil {
		return gpucore.InvalidID, fmt.Errorf("failed to create bind group layout: %w", err)
	}

	id := gpucore.BindGroupLayoutID(a.newID())

	a.mu.Lock()
	a.bindGroupLayouts[id] = layout
	a.mu.Unlock()

	return id, nil
}

// DestroyBindGroupLayout releases a bind group layout.
func (a *HALAdapter) DestroyBindGroupLayout(id gpucore.BindGroupLayoutID) {
	a.mu.Lock()
	layout, ok := a.bindGroupLayouts[id]
	if ok {
		delete(a.bindGroupLayouts, id)
	}
	a.mu.Unlock()

	if ok {
		a.device.DestroyBindGroupLayout(layout)
	}
}

// CreatePipelineLayout creates a pipeline layout.
func (a *HALAdapter) CreatePipelineLayout(layouts []gpucore.BindGroupLayoutID) (gpucore.PipelineLayoutID, error) {
	a.mu.RLock()
	halLayouts := make([]hal.BindGroupLayout, len(layouts))
	for i, id := range layouts {
		layout, ok := a.bindGroupLayouts[id]
		if !ok {
			a.mu.RUnlock()
			return gpucore.InvalidID, fmt.Errorf("bind group layout %d not found", id)
		}
		halLayouts[i] = layout
	}
	a.mu.RUnlock()

	halDesc := &hal.PipelineLayoutDescriptor{
		Label:            "",
		BindGroupLayouts: halLayouts,
	}

	pipelineLayout, err := a.device.CreatePipelineLayout(halDesc)
	if err != nil {
		return gpucore.InvalidID, fmt.Errorf("failed to create pipeline layout: %w", err)
	}

	id := gpucore.PipelineLayoutID(a.newID())

	a.mu.Lock()
	a.pipelineLayouts[id] = pipelineLayout
	a.mu.Unlock()

	return id, nil
}

// DestroyPipelineLayout releases a pipeline layout.
func (a *HALAdapter) DestroyPipelineLayout(id gpucore.PipelineLayoutID) {
	a.mu.Lock()
	layout, ok := a.pipelineLayouts[id]
	if ok {
		delete(a.pipelineLayouts, id)
	}
	a.mu.Unlock()

	if ok {
		a.device.DestroyPipelineLayout(layout)
	}
}

// CreateComputePipeline creates a compute pipeline.
func (a *HALAdapter) CreateComputePipeline(desc *gpucore.ComputePipelineDesc) (gpucore.ComputePipelineID, error) {
	if desc == nil {
		return gpucore.InvalidID, fmt.Errorf("nil compute pipeline descriptor")
	}

	a.mu.RLock()
	pipelineLayout, layoutOK := a.pipelineLayouts[desc.Layout]
	shaderModule, moduleOK := a.shaderModules[desc.ShaderModule]
	a.mu.RUnlock()

	if !layoutOK {
		return gpucore.InvalidID, fmt.Errorf("pipeline layout %d not found", desc.Layout)
	}
	if !moduleOK {
		return gpucore.InvalidID, fmt.Errorf("shader module %d not found", desc.ShaderModule)
	}

	halDesc := &hal.ComputePipelineDescriptor{
		Label:  desc.Label,
		Layout: pipelineLayout,
		Compute: hal.ComputeState{
			Module:     shaderModule,
			EntryPoint: desc.EntryPoint,
		},
	}

	pipeline, err := a.device.CreateComputePipeline(halDesc)
	if err != nil {
		return gpucore.InvalidID, fmt.Errorf("failed to create compute pipeline: %w", err)
	}

	id := gpucore.ComputePipelineID(a.newID())

	a.mu.Lock()
	a.computePipelines[id] = pipeline
	a.mu.Unlock()

	return id, nil
}

// DestroyComputePipeline releases a compute pipeline.
func (a *HALAdapter) DestroyComputePipeline(id gpucore.ComputePipelineID) {
	a.mu.Lock()
	pipeline, ok := a.computePipelines[id]
	if ok {
		delete(a.computePipelines, id)
	}
	a.mu.Unlock()

	if ok {
		a.device.DestroyComputePipeline(pipeline)
	}
}

// CreateBindGroup creates a bind group.
func (a *HALAdapter) CreateBindGroup(layout gpucore.BindGroupLayoutID, entries []gpucore.BindGroupEntry) (gpucore.BindGroupID, error) {
	a.mu.RLock()
	halLayout, ok := a.bindGroupLayouts[layout]
	if !ok {
		a.mu.RUnlock()
		return gpucore.InvalidID, fmt.Errorf("bind group layout %d not found", layout)
	}

	halEntries := make([]types.BindGroupEntry, len(entries))
	for i, entry := range entries {
		halEntry, err := a.convertBindGroupEntry(entry)
		if err != nil {
			a.mu.RUnlock()
			return gpucore.InvalidID, fmt.Errorf("failed to convert bind group entry %d: %w", entry.Binding, err)
		}
		halEntries[i] = halEntry
	}
	a.mu.RUnlock()

	halDesc := &hal.BindGroupDescriptor{
		Label:   "",
		Layout:  halLayout,
		Entries: halEntries,
	}

	bindGroup, err := a.device.CreateBindGroup(halDesc)
	if err != nil {
		return gpucore.InvalidID, fmt.Errorf("failed to create bind group: %w", err)
	}

	id := gpucore.BindGroupID(a.newID())

	a.mu.Lock()
	a.bindGroups[id] = bindGroup
	a.mu.Unlock()

	return id, nil
}

// DestroyBindGroup releases a bind group.
func (a *HALAdapter) DestroyBindGroup(id gpucore.BindGroupID) {
	a.mu.Lock()
	group, ok := a.bindGroups[id]
	if ok {
		delete(a.bindGroups, id)
	}
	a.mu.Unlock()

	if ok {
		a.device.DestroyBindGroup(group)
	}
}

// === Command Recording and Execution ===

// BeginComputePass begins a compute pass.
func (a *HALAdapter) BeginComputePass() gpucore.ComputePassEncoder {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Create a new encoder if we don't have one
	if !a.hasEncoder {
		encoder, err := a.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
			Label: "compute-encoder",
		})
		if err != nil {
			// Return a no-op encoder on error
			return &halComputePassEncoder{adapter: a}
		}

		if err := encoder.BeginEncoding("compute-pass"); err != nil {
			return &halComputePassEncoder{adapter: a}
		}

		a.encoder = encoder
		a.hasEncoder = true
	}

	// Begin compute pass
	halPass := a.encoder.BeginComputePass(&hal.ComputePassDescriptor{
		Label: "compute",
	})

	return &halComputePassEncoder{
		adapter: a,
		pass:    halPass,
	}
}

// Submit submits recorded commands to the GPU.
func (a *HALAdapter) Submit() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasEncoder || a.encoder == nil {
		return
	}

	cmdBuffer, err := a.encoder.EndEncoding()
	if err != nil {
		a.encoder = nil
		a.hasEncoder = false
		return
	}

	// Submit without fence (fire and forget)
	_ = a.queue.Submit([]hal.CommandBuffer{cmdBuffer}, nil, 0)

	// Clean up
	cmdBuffer.Destroy()
	a.encoder = nil
	a.hasEncoder = false
}

// WaitIdle waits for all GPU operations to complete.
func (a *HALAdapter) WaitIdle() {
	// Submit any pending work first
	a.Submit()

	// Create a fence and submit empty work to synchronize
	fence, err := a.device.CreateFence()
	if err != nil {
		return
	}
	defer a.device.DestroyFence(fence)

	if err := a.queue.Submit(nil, fence, 1); err != nil {
		return
	}

	// Wait for fence (5 second timeout)
	_, _ = a.device.Wait(fence, 1, 5_000_000_000)
}

// === Type Conversion Helpers ===

// convertBufferUsage converts gpucore.BufferUsage to types.BufferUsage.
func convertBufferUsage(usage gpucore.BufferUsage) types.BufferUsage {
	var result types.BufferUsage

	if usage&gpucore.BufferUsageMapRead != 0 {
		result |= types.BufferUsageMapRead
	}
	if usage&gpucore.BufferUsageMapWrite != 0 {
		result |= types.BufferUsageMapWrite
	}
	if usage&gpucore.BufferUsageCopySrc != 0 {
		result |= types.BufferUsageCopySrc
	}
	if usage&gpucore.BufferUsageCopyDst != 0 {
		result |= types.BufferUsageCopyDst
	}
	if usage&gpucore.BufferUsageIndex != 0 {
		result |= types.BufferUsageIndex
	}
	if usage&gpucore.BufferUsageVertex != 0 {
		result |= types.BufferUsageVertex
	}
	if usage&gpucore.BufferUsageUniform != 0 {
		result |= types.BufferUsageUniform
	}
	if usage&gpucore.BufferUsageStorage != 0 {
		result |= types.BufferUsageStorage
	}
	if usage&gpucore.BufferUsageIndirect != 0 {
		result |= types.BufferUsageIndirect
	}

	return result
}

// convertTextureFormat converts gpucore.TextureFormat to types.TextureFormat.
func convertTextureFormat(format gpucore.TextureFormat) types.TextureFormat {
	switch format {
	case gpucore.TextureFormatRGBA8Unorm:
		return types.TextureFormatRGBA8Unorm
	case gpucore.TextureFormatRGBA8UnormSRGB:
		return types.TextureFormatRGBA8UnormSrgb
	case gpucore.TextureFormatBGRA8Unorm:
		return types.TextureFormatBGRA8Unorm
	case gpucore.TextureFormatBGRA8UnormSRGB:
		return types.TextureFormatBGRA8UnormSrgb
	case gpucore.TextureFormatR8Unorm:
		return types.TextureFormatR8Unorm
	case gpucore.TextureFormatR32Float:
		return types.TextureFormatR32Float
	case gpucore.TextureFormatRG32Float:
		return types.TextureFormatRG32Float
	case gpucore.TextureFormatRGBA32Float:
		return types.TextureFormatRGBA32Float
	default:
		return types.TextureFormatRGBA8Unorm
	}
}

// convertBindGroupLayoutEntry converts gpucore.BindGroupLayoutEntry to types.BindGroupLayoutEntry.
func convertBindGroupLayoutEntry(entry gpucore.BindGroupLayoutEntry) types.BindGroupLayoutEntry {
	result := types.BindGroupLayoutEntry{
		Binding:    entry.Binding,
		Visibility: types.ShaderStageCompute, // Default to compute for gpucore
	}

	switch entry.Type {
	case gpucore.BindingTypeUniformBuffer:
		result.Buffer = &types.BufferBindingLayout{
			Type:           types.BufferBindingTypeUniform,
			MinBindingSize: entry.MinBindingSize,
		}
	case gpucore.BindingTypeStorageBuffer:
		result.Buffer = &types.BufferBindingLayout{
			Type:           types.BufferBindingTypeStorage,
			MinBindingSize: entry.MinBindingSize,
		}
	case gpucore.BindingTypeReadOnlyStorageBuffer:
		result.Buffer = &types.BufferBindingLayout{
			Type:           types.BufferBindingTypeReadOnlyStorage,
			MinBindingSize: entry.MinBindingSize,
		}
	case gpucore.BindingTypeStorageTexture:
		result.Storage = &types.StorageTextureBindingLayout{
			Access:        types.StorageTextureAccessReadWrite,
			Format:        types.TextureFormatRGBA8Unorm,
			ViewDimension: types.TextureViewDimension2D,
		}
	}

	return result
}

// convertBindGroupEntry converts gpucore.BindGroupEntry to types.BindGroupEntry.
// Must be called with mu.RLock held.
func (a *HALAdapter) convertBindGroupEntry(entry gpucore.BindGroupEntry) (types.BindGroupEntry, error) {
	result := types.BindGroupEntry{
		Binding: entry.Binding,
	}

	// Determine resource type based on which ID is non-zero
	if entry.Buffer != gpucore.InvalidID {
		buffer, ok := a.buffers[entry.Buffer]
		if !ok {
			return result, fmt.Errorf("buffer %d not found", entry.Buffer)
		}

		// Get buffer handle - this is a placeholder since hal.Buffer doesn't expose handle directly
		// In a real implementation, we'd need to track buffer handles or use type assertions
		_ = buffer

		result.Resource = types.BufferBinding{
			Buffer: types.BufferHandle(entry.Buffer), // Use gpucore ID as placeholder
			Offset: entry.Offset,
			Size:   entry.Size,
		}
	} else if entry.Texture != gpucore.InvalidID {
		texture, ok := a.textures[entry.Texture]
		if !ok {
			return result, fmt.Errorf("texture %d not found", entry.Texture)
		}

		// Create texture view for binding
		_ = texture

		result.Resource = types.TextureViewBinding{
			TextureView: types.TextureViewHandle(entry.Texture), // Placeholder
		}
	}

	return result, nil
}

// === Compute Pass Encoder ===

// halComputePassEncoder implements gpucore.ComputePassEncoder.
type halComputePassEncoder struct {
	adapter *HALAdapter
	pass    hal.ComputePassEncoder
}

// SetPipeline sets the active compute pipeline.
func (e *halComputePassEncoder) SetPipeline(pipeline gpucore.ComputePipelineID) {
	if e.pass == nil {
		return
	}

	e.adapter.mu.RLock()
	halPipeline, ok := e.adapter.computePipelines[pipeline]
	e.adapter.mu.RUnlock()

	if ok {
		e.pass.SetPipeline(halPipeline)
	}
}

// SetBindGroup sets a bind group at the specified index.
func (e *halComputePassEncoder) SetBindGroup(index uint32, group gpucore.BindGroupID) {
	if e.pass == nil {
		return
	}

	e.adapter.mu.RLock()
	halGroup, ok := e.adapter.bindGroups[group]
	e.adapter.mu.RUnlock()

	if ok {
		e.pass.SetBindGroup(index, halGroup, nil)
	}
}

// Dispatch dispatches compute workgroups.
func (e *halComputePassEncoder) Dispatch(x, y, z uint32) {
	if e.pass == nil {
		return
	}
	e.pass.Dispatch(x, y, z)
}

// End finishes the compute pass.
func (e *halComputePassEncoder) End() {
	if e.pass == nil {
		return
	}
	e.pass.End()
}
