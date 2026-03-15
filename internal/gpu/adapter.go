//go:build !nogpu

// Package gpu provides a Pure Go GPU-accelerated rendering backend using gogpu/wgpu.
package gpu

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogpu/gg/internal/gpucore"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// HALAdapter implements gpucore.GPUAdapter using gogpu/wgpu/hal directly.
// It provides a bridge between the gpucore abstraction and the HAL layer.
//
// Thread Safety: HALAdapter is safe for concurrent use from multiple goroutines.
// All resource operations are protected by a mutex.
type HALAdapter struct {
	mu     sync.RWMutex
	device *wgpu.Device
	queue  *wgpu.Queue

	// Adapter limits and capabilities
	limits       gputypes.Limits
	hasCompute   bool
	maxBufferSz  uint64
	maxWorkgroup [3]uint32

	// ID generation
	nextID atomic.Uint64

	// Resource tracking maps gpucore IDs to hal resources
	buffers          map[gpucore.BufferID]*wgpu.Buffer
	textures         map[gpucore.TextureID]*wgpu.Texture
	shaderModules    map[gpucore.ShaderModuleID]*wgpu.ShaderModule
	computePipelines map[gpucore.ComputePipelineID]*wgpu.ComputePipeline
	bindGroupLayouts map[gpucore.BindGroupLayoutID]*wgpu.BindGroupLayout
	pipelineLayouts  map[gpucore.PipelineLayoutID]*wgpu.PipelineLayout
	bindGroups       map[gpucore.BindGroupID]*wgpu.BindGroup

	// Command encoder for current frame
	encoder    *wgpu.CommandEncoder
	hasEncoder bool
}

// NewHALAdapter creates a new HALAdapter wrapping the given device and queue.
// The limits parameter provides the adapter's capability limits.
// If limits is nil, default limits are used.
func NewHALAdapter(device *wgpu.Device, queue *wgpu.Queue, limits *gputypes.Limits) *HALAdapter {
	var lim gputypes.Limits
	if limits != nil {
		lim = *limits
	} else {
		lim = gputypes.DefaultLimits()
	}

	adapter := &HALAdapter{
		device:           device,
		queue:            queue,
		limits:           lim,
		hasCompute:       true, // Assume compute support by default
		maxBufferSz:      lim.MaxBufferSize,
		maxWorkgroup:     [3]uint32{lim.MaxComputeWorkgroupSizeX, lim.MaxComputeWorkgroupSizeY, lim.MaxComputeWorkgroupSizeZ},
		buffers:          make(map[gpucore.BufferID]*wgpu.Buffer),
		textures:         make(map[gpucore.TextureID]*wgpu.Texture),
		shaderModules:    make(map[gpucore.ShaderModuleID]*wgpu.ShaderModule),
		computePipelines: make(map[gpucore.ComputePipelineID]*wgpu.ComputePipeline),
		bindGroupLayouts: make(map[gpucore.BindGroupLayoutID]*wgpu.BindGroupLayout),
		pipelineLayouts:  make(map[gpucore.PipelineLayoutID]*wgpu.PipelineLayout),
		bindGroups:       make(map[gpucore.BindGroupID]*wgpu.BindGroup),
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

	desc := &wgpu.ShaderModuleDescriptor{
		Label: label,
		SPIRV: spirv,
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
		module.Release()
	}
}

// === Buffer Management ===

// CreateBuffer creates a GPU buffer.
func (a *HALAdapter) CreateBuffer(size int, usage gpucore.BufferUsage) (gpucore.BufferID, error) {
	if size <= 0 {
		return gpucore.InvalidID, fmt.Errorf("buffer size must be positive")
	}

	desc := &wgpu.BufferDescriptor{
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
		buffer.Release()
	}
}

// WriteBuffer writes data to a buffer.
func (a *HALAdapter) WriteBuffer(id gpucore.BufferID, offset uint64, data []byte) {
	a.mu.RLock()
	buffer, ok := a.buffers[id]
	a.mu.RUnlock()

	if ok && len(data) > 0 {
		if err := a.queue.WriteBuffer(buffer, offset, data); err != nil {
			slogger().Warn("WriteBuffer failed", "error", err)
		}
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
	stagingDesc := &wgpu.BufferDescriptor{
		Label:            "staging-readback",
		Size:             size,
		Usage:            gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
		MappedAtCreation: true,
	}

	stagingBuffer, err := a.device.CreateBuffer(stagingDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to create staging buffer: %w", err)
	}
	defer stagingBuffer.Release()

	// Create command encoder for copy
	encoder, err := a.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "buffer-read-encoder",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create command encoder: %w", err)
	}

	// Copy from source buffer to staging buffer
	encoder.CopyBufferToBuffer(buffer, offset, stagingBuffer, 0, size)

	cmdBuffer, err := encoder.Finish()
	if err != nil {
		return nil, fmt.Errorf("failed to end encoding: %w", err)
	}

	// Submit and wait
	fence, err := a.device.CreateFence()
	if err != nil {
		return nil, fmt.Errorf("failed to create fence: %w", err)
	}
	defer fence.Release()

	if err := a.queue.SubmitWithFence([]*wgpu.CommandBuffer{cmdBuffer}, fence, 1); err != nil {
		return nil, fmt.Errorf("failed to submit commands: %w", err)
	}

	// Wait for completion (5 seconds)
	_, err = a.device.WaitForFence(fence, 1, 5*time.Second)
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
	// Bounds check for uint32 conversion (gosec G115)
	const maxDim = 1 << 24 // 16M pixels max per dimension (realistic GPU limit)
	if width > maxDim || height > maxDim {
		return gpucore.InvalidID, fmt.Errorf("texture dimensions exceed maximum (%d)", maxDim)
	}

	desc := &wgpu.TextureDescriptor{
		Label: "",
		Size: wgpu.Extent3D{
			Width:              uint32(width),
			Height:             uint32(height),
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        convertTextureFormat(format),
		Usage:         gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst | gputypes.TextureUsageStorageBinding,
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
		texture.Release()
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

	dst := &wgpu.ImageCopyTexture{
		Texture:  texture,
		MipLevel: 0,
		Origin:   wgpu.Origin3D{X: 0, Y: 0, Z: 0},
		Aspect:   gputypes.TextureAspectAll,
	}

	// Placeholder dimensions - real implementation needs texture metadata
	layout := &wgpu.ImageDataLayout{
		Offset:       0,
		BytesPerRow:  0, // Will be calculated based on texture width
		RowsPerImage: 0,
	}

	size := &wgpu.Extent3D{
		Width:              0, // Placeholder
		Height:             0,
		DepthOrArrayLayers: 1,
	}

	if err := a.queue.WriteTexture(dst, data, layout, size); err != nil {
		slogger().Warn("WriteTexture failed", "error", err)
	}
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

	halEntries := make([]gputypes.BindGroupLayoutEntry, len(desc.Entries))
	for i, entry := range desc.Entries {
		halEntries[i] = convertBindGroupLayoutEntry(entry)
	}

	halDesc := &wgpu.BindGroupLayoutDescriptor{
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
		layout.Release()
	}
}

// CreatePipelineLayout creates a pipeline layout.
func (a *HALAdapter) CreatePipelineLayout(layouts []gpucore.BindGroupLayoutID) (gpucore.PipelineLayoutID, error) {
	a.mu.RLock()
	halLayouts := make([]*wgpu.BindGroupLayout, len(layouts))
	for i, id := range layouts {
		layout, ok := a.bindGroupLayouts[id]
		if !ok {
			a.mu.RUnlock()
			return gpucore.InvalidID, fmt.Errorf("bind group layout %d not found", id)
		}
		halLayouts[i] = layout
	}
	a.mu.RUnlock()

	halDesc := &wgpu.PipelineLayoutDescriptor{
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
		layout.Release()
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

	halDesc := &wgpu.ComputePipelineDescriptor{
		Label:      desc.Label,
		Layout:     pipelineLayout,
		Module:     shaderModule,
		EntryPoint: desc.EntryPoint,
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
		pipeline.Release()
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

	halEntries := make([]wgpu.BindGroupEntry, len(entries))
	for i, entry := range entries {
		halEntry, err := a.convertBindGroupEntry(entry)
		if err != nil {
			a.mu.RUnlock()
			return gpucore.InvalidID, fmt.Errorf("failed to convert bind group entry %d: %w", entry.Binding, err)
		}
		halEntries[i] = halEntry
	}
	a.mu.RUnlock()

	halDesc := &wgpu.BindGroupDescriptor{
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
		group.Release()
	}
}

// === Command Recording and Execution ===

// BeginComputePass begins a compute pass.
func (a *HALAdapter) BeginComputePass() gpucore.ComputePassEncoder {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Create a new encoder if we don't have one
	if !a.hasEncoder {
		encoder, err := a.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
			Label: "compute-encoder",
		})
		if err != nil {
			// Return a no-op encoder on error
			return &halComputePassEncoder{adapter: a}
		}

		a.encoder = encoder
		a.hasEncoder = true
	}

	// Begin compute pass
	halPass, cpErr := a.encoder.BeginComputePass(&wgpu.ComputePassDescriptor{
		Label: "compute",
	})
	if cpErr != nil {
		return &halComputePassEncoder{adapter: a}
	}

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

	cmdBuffer, err := a.encoder.Finish()
	if err != nil {
		a.encoder = nil
		a.hasEncoder = false
		return
	}

	// Submit without fence (fire and forget)
	_ = a.queue.Submit(cmdBuffer)

	// Clean up
	// cmdBuffer consumed by Submit
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
	defer fence.Release()

	if err := a.queue.SubmitWithFence(nil, fence, 1); err != nil {
		return
	}

	// Wait for fence (5 second timeout)
	_, _ = a.device.WaitForFence(fence, 1, 5*time.Second)
}

// === Type Conversion Helpers ===

// convertBufferUsage converts gpucore.BufferUsage to gputypes.BufferUsage.
func convertBufferUsage(usage gpucore.BufferUsage) gputypes.BufferUsage {
	var result gputypes.BufferUsage

	if usage&gpucore.BufferUsageMapRead != 0 {
		result |= gputypes.BufferUsageMapRead
	}
	if usage&gpucore.BufferUsageMapWrite != 0 {
		result |= gputypes.BufferUsageMapWrite
	}
	if usage&gpucore.BufferUsageCopySrc != 0 {
		result |= gputypes.BufferUsageCopySrc
	}
	if usage&gpucore.BufferUsageCopyDst != 0 {
		result |= gputypes.BufferUsageCopyDst
	}
	if usage&gpucore.BufferUsageIndex != 0 {
		result |= gputypes.BufferUsageIndex
	}
	if usage&gpucore.BufferUsageVertex != 0 {
		result |= gputypes.BufferUsageVertex
	}
	if usage&gpucore.BufferUsageUniform != 0 {
		result |= gputypes.BufferUsageUniform
	}
	if usage&gpucore.BufferUsageStorage != 0 {
		result |= gputypes.BufferUsageStorage
	}
	if usage&gpucore.BufferUsageIndirect != 0 {
		result |= gputypes.BufferUsageIndirect
	}

	return result
}

// convertTextureFormat converts gpucore.TextureFormat to gputypes.TextureFormat.
func convertTextureFormat(format gpucore.TextureFormat) gputypes.TextureFormat {
	switch format {
	case gpucore.TextureFormatRGBA8Unorm:
		return gputypes.TextureFormatRGBA8Unorm
	case gpucore.TextureFormatRGBA8UnormSRGB:
		return gputypes.TextureFormatRGBA8UnormSrgb
	case gpucore.TextureFormatBGRA8Unorm:
		return gputypes.TextureFormatBGRA8Unorm
	case gpucore.TextureFormatBGRA8UnormSRGB:
		return gputypes.TextureFormatBGRA8UnormSrgb
	case gpucore.TextureFormatR8Unorm:
		return gputypes.TextureFormatR8Unorm
	case gpucore.TextureFormatR32Float:
		return gputypes.TextureFormatR32Float
	case gpucore.TextureFormatRG32Float:
		return gputypes.TextureFormatRG32Float
	case gpucore.TextureFormatRGBA32Float:
		return gputypes.TextureFormatRGBA32Float
	default:
		return gputypes.TextureFormatRGBA8Unorm
	}
}

// convertBindGroupLayoutEntry converts gpucore.BindGroupLayoutEntry to gputypes.BindGroupLayoutEntry.
func convertBindGroupLayoutEntry(entry gpucore.BindGroupLayoutEntry) gputypes.BindGroupLayoutEntry {
	result := gputypes.BindGroupLayoutEntry{
		Binding:    entry.Binding,
		Visibility: gputypes.ShaderStageCompute, // Default to compute for gpucore
	}

	switch entry.Type {
	case gpucore.BindingTypeUniformBuffer:
		result.Buffer = &gputypes.BufferBindingLayout{
			Type:           gputypes.BufferBindingTypeUniform,
			MinBindingSize: entry.MinBindingSize,
		}
	case gpucore.BindingTypeStorageBuffer:
		result.Buffer = &gputypes.BufferBindingLayout{
			Type:           gputypes.BufferBindingTypeStorage,
			MinBindingSize: entry.MinBindingSize,
		}
	case gpucore.BindingTypeReadOnlyStorageBuffer:
		result.Buffer = &gputypes.BufferBindingLayout{
			Type:           gputypes.BufferBindingTypeReadOnlyStorage,
			MinBindingSize: entry.MinBindingSize,
		}
	case gpucore.BindingTypeStorageTexture:
		result.StorageTexture = &gputypes.StorageTextureBindingLayout{
			Access:        gputypes.StorageTextureAccessReadWrite,
			Format:        gputypes.TextureFormatRGBA8Unorm,
			ViewDimension: gputypes.TextureViewDimension2D,
		}
	}

	return result
}

// convertBindGroupEntry converts gpucore.BindGroupEntry to wgpu.BindGroupEntry.
// Must be called with mu.RLock held.
func (a *HALAdapter) convertBindGroupEntry(entry gpucore.BindGroupEntry) (wgpu.BindGroupEntry, error) {
	result := wgpu.BindGroupEntry{
		Binding: entry.Binding,
	}

	// Determine resource type based on which ID is non-zero
	if entry.Buffer != gpucore.InvalidID {
		buffer, ok := a.buffers[entry.Buffer]
		if !ok {
			return result, fmt.Errorf("buffer %d not found", entry.Buffer)
		}
		result.Buffer = buffer
		result.Offset = entry.Offset
		result.Size = entry.Size
	}
	// Note: texture bindings would use result.TextureView with a *wgpu.TextureView.
	// Currently gpucore doesn't support texture view bindings through this path.

	return result, nil
}

// === Compute Pass Encoder ===

// halComputePassEncoder implements gpucore.ComputePassEncoder.
type halComputePassEncoder struct {
	adapter *HALAdapter
	pass    *wgpu.ComputePassEncoder
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
	_ = e.pass.End()
}
