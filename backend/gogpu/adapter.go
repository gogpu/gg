//go:build !nogpu

// Package gogpu provides a GPU-accelerated rendering backend for gg
// using the gogpu/gogpu framework.
package gogpu

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gg/gpucore"
	"github.com/gogpu/gogpu/gpu"
	"github.com/gogpu/gogpu/gpu/types"
)

// GoGPUAdapter implements gpucore.GPUAdapter using gogpu/gogpu's gpu.Backend.
// It provides a bridge between the gpucore abstraction and the gpu.Backend interface,
// which supports both Rust (wgpu-native) and Pure Go (gogpu/wgpu) implementations.
//
// Thread Safety: GoGPUAdapter is safe for concurrent use from multiple goroutines.
// All resource operations are protected by a mutex.
//
// Current Limitations:
// The gpu.Backend interface currently focuses on render operations. Compute shader
// support (CreateShaderModule with SPIR-V, compute pipelines, compute passes) is
// not yet implemented in the Backend interface. These methods return ErrNotImplemented.
type GoGPUAdapter struct {
	mu      sync.RWMutex
	backend gpu.Backend
	device  types.Device
	queue   types.Queue

	// Adapter limits and capabilities
	hasCompute   bool
	maxBufferSz  uint64
	maxWorkgroup [3]uint32

	// ID generation
	nextID atomic.Uint64

	// Resource tracking maps gpucore IDs to gpu/types handles
	buffers          map[gpucore.BufferID]types.Buffer
	textures         map[gpucore.TextureID]types.Texture
	textureInfo      map[gpucore.TextureID]textureMetadata
	shaderModules    map[gpucore.ShaderModuleID]types.ShaderModule
	computePipelines map[gpucore.ComputePipelineID]computePipelineInfo
	bindGroupLayouts map[gpucore.BindGroupLayoutID]types.BindGroupLayout
	pipelineLayouts  map[gpucore.PipelineLayoutID]types.PipelineLayout
	bindGroups       map[gpucore.BindGroupID]types.BindGroup
}

// textureMetadata stores texture dimensions for write/read operations.
type textureMetadata struct {
	width  int
	height int
	format gpucore.TextureFormat
}

// computePipelineInfo stores compute pipeline state.
// Currently a placeholder since gpu.Backend doesn't support compute pipelines.
// When compute support is added, this will contain layout, module, and entry point.
type computePipelineInfo struct{}

// NewGoGPUAdapter creates a new GoGPUAdapter wrapping the given gpu.Backend,
// device, and queue handles.
//
// The adapter assumes compute support is not available by default since
// the current gpu.Backend interface doesn't expose compute operations.
// When compute support is added to gpu.Backend, this can be updated.
func NewGoGPUAdapter(backend gpu.Backend, device types.Device, queue types.Queue) *GoGPUAdapter {
	adapter := &GoGPUAdapter{
		backend:          backend,
		device:           device,
		queue:            queue,
		hasCompute:       false,             // gpu.Backend doesn't support compute yet
		maxBufferSz:      256 * 1024 * 1024, // 256 MB default
		maxWorkgroup:     [3]uint32{256, 256, 64},
		buffers:          make(map[gpucore.BufferID]types.Buffer),
		textures:         make(map[gpucore.TextureID]types.Texture),
		textureInfo:      make(map[gpucore.TextureID]textureMetadata),
		shaderModules:    make(map[gpucore.ShaderModuleID]types.ShaderModule),
		computePipelines: make(map[gpucore.ComputePipelineID]computePipelineInfo),
		bindGroupLayouts: make(map[gpucore.BindGroupLayoutID]types.BindGroupLayout),
		pipelineLayouts:  make(map[gpucore.PipelineLayoutID]types.PipelineLayout),
		bindGroups:       make(map[gpucore.BindGroupID]types.BindGroup),
	}

	// Start ID generation at 1 (0 is invalid)
	adapter.nextID.Store(1)

	return adapter
}

// newID generates a unique resource ID.
func (a *GoGPUAdapter) newID() uint64 {
	return a.nextID.Add(1) - 1
}

// === Capabilities ===

// SupportsCompute returns whether compute shaders are supported.
// Currently returns false since gpu.Backend doesn't expose compute operations.
func (a *GoGPUAdapter) SupportsCompute() bool {
	return a.hasCompute
}

// MaxWorkgroupSize returns the maximum workgroup size in each dimension.
func (a *GoGPUAdapter) MaxWorkgroupSize() [3]uint32 {
	return a.maxWorkgroup
}

// MaxBufferSize returns the maximum buffer size in bytes.
func (a *GoGPUAdapter) MaxBufferSize() uint64 {
	return a.maxBufferSz
}

// === Shader Compilation ===

// CreateShaderModule creates a shader module from SPIR-V bytecode.
//
// NOTE: This method returns ErrNotImplemented because gpu.Backend currently
// only supports WGSL shaders (CreateShaderModuleWGSL), not SPIR-V.
// When SPIR-V support is added to gpu.Backend, this will be implemented.
func (a *GoGPUAdapter) CreateShaderModule(spirv []uint32, label string) (gpucore.ShaderModuleID, error) {
	if len(spirv) == 0 {
		return gpucore.InvalidID, fmt.Errorf("empty SPIR-V bytecode")
	}

	// gpu.Backend only supports WGSL shaders via CreateShaderModuleWGSL
	// SPIR-V shader creation is not yet available
	return gpucore.InvalidID, fmt.Errorf("%w: SPIR-V shader modules not supported by gpu.Backend", ErrNotImplemented)
}

// DestroyShaderModule releases a shader module.
func (a *GoGPUAdapter) DestroyShaderModule(id gpucore.ShaderModuleID) {
	a.mu.Lock()
	_, ok := a.shaderModules[id]
	if ok {
		delete(a.shaderModules, id)
	}
	a.mu.Unlock()

	// Note: gpu.Backend doesn't have a ReleaseShaderModule method
	// Shader modules are managed by the backend
}

// === Buffer Management ===

// CreateBuffer creates a GPU buffer.
func (a *GoGPUAdapter) CreateBuffer(size int, usage gpucore.BufferUsage) (gpucore.BufferID, error) {
	if size <= 0 {
		return gpucore.InvalidID, fmt.Errorf("buffer size must be positive")
	}

	desc := &types.BufferDescriptor{
		Label: "",
		Size:  uint64(size),
		Usage: convertBufferUsage(usage),
	}

	buffer, err := a.backend.CreateBuffer(a.device, desc)
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
func (a *GoGPUAdapter) DestroyBuffer(id gpucore.BufferID) {
	a.mu.Lock()
	buffer, ok := a.buffers[id]
	if ok {
		delete(a.buffers, id)
	}
	a.mu.Unlock()

	if ok {
		a.backend.ReleaseBuffer(buffer)
	}
}

// WriteBuffer writes data to a buffer.
func (a *GoGPUAdapter) WriteBuffer(id gpucore.BufferID, offset uint64, data []byte) {
	a.mu.RLock()
	buffer, ok := a.buffers[id]
	a.mu.RUnlock()

	if ok && len(data) > 0 {
		a.backend.WriteBuffer(a.queue, buffer, offset, data)
	}
}

// ReadBuffer reads data from a buffer.
// This operation requires a staging buffer and GPU-CPU synchronization.
//
// NOTE: This method returns ErrNotImplemented because gpu.Backend doesn't
// currently expose buffer readback operations. When buffer mapping is
// added to gpu.Backend, this will be implemented.
func (a *GoGPUAdapter) ReadBuffer(id gpucore.BufferID, offset, size uint64) ([]byte, error) {
	a.mu.RLock()
	_, ok := a.buffers[id]
	a.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("buffer %d not found", id)
	}

	// gpu.Backend doesn't expose buffer readback operations
	return nil, fmt.Errorf("%w: buffer readback not supported by gpu.Backend", ErrNotImplemented)
}

// === Texture Management ===

// CreateTexture creates a GPU texture.
func (a *GoGPUAdapter) CreateTexture(width, height int, format gpucore.TextureFormat) (gpucore.TextureID, error) {
	if width <= 0 || height <= 0 {
		return gpucore.InvalidID, fmt.Errorf("texture dimensions must be positive")
	}

	desc := &types.TextureDescriptor{
		Label: "",
		Size: types.Extent3D{
			Width:              safeIntToUint32(width),
			Height:             safeIntToUint32(height),
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     types.TextureDimension2D,
		Format:        convertTextureFormat(format),
		Usage:         types.TextureUsageCopySrc | types.TextureUsageCopyDst | types.TextureUsageStorageBinding,
	}

	texture, err := a.backend.CreateTexture(a.device, desc)
	if err != nil {
		return gpucore.InvalidID, fmt.Errorf("failed to create texture: %w", err)
	}

	id := gpucore.TextureID(a.newID())

	a.mu.Lock()
	a.textures[id] = texture
	a.textureInfo[id] = textureMetadata{
		width:  width,
		height: height,
		format: format,
	}
	a.mu.Unlock()

	return id, nil
}

// DestroyTexture releases a GPU texture.
func (a *GoGPUAdapter) DestroyTexture(id gpucore.TextureID) {
	a.mu.Lock()
	texture, ok := a.textures[id]
	if ok {
		delete(a.textures, id)
		delete(a.textureInfo, id)
	}
	a.mu.Unlock()

	if ok {
		a.backend.ReleaseTexture(texture)
	}
}

// WriteTexture writes data to a texture.
func (a *GoGPUAdapter) WriteTexture(id gpucore.TextureID, data []byte) {
	a.mu.RLock()
	texture, ok := a.textures[id]
	info, hasInfo := a.textureInfo[id]
	a.mu.RUnlock()

	if !ok || !hasInfo || len(data) == 0 {
		return
	}

	bytesPerPixel := getBytesPerPixel(info.format)
	bytesPerRow := safeIntToUint32(info.width) * bytesPerPixel

	dst := &types.ImageCopyTexture{
		Texture:  texture,
		MipLevel: 0,
		Origin:   types.Origin3D{X: 0, Y: 0, Z: 0},
		Aspect:   types.TextureAspectAll,
	}

	layout := &types.ImageDataLayout{
		Offset:       0,
		BytesPerRow:  bytesPerRow,
		RowsPerImage: safeIntToUint32(info.height),
	}

	size := &types.Extent3D{
		Width:              safeIntToUint32(info.width),
		Height:             safeIntToUint32(info.height),
		DepthOrArrayLayers: 1,
	}

	a.backend.WriteTexture(a.queue, dst, data, layout, size)
}

// ReadTexture reads data from a texture.
//
// NOTE: This method returns ErrNotImplemented because gpu.Backend doesn't
// currently expose texture readback operations.
func (a *GoGPUAdapter) ReadTexture(id gpucore.TextureID) ([]byte, error) {
	a.mu.RLock()
	_, ok := a.textures[id]
	a.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("texture %d not found", id)
	}

	// gpu.Backend doesn't expose texture readback operations
	return nil, fmt.Errorf("%w: texture readback not supported by gpu.Backend", ErrNotImplemented)
}

// === Pipeline Management ===

// CreateBindGroupLayout creates a bind group layout.
func (a *GoGPUAdapter) CreateBindGroupLayout(desc *gpucore.BindGroupLayoutDesc) (gpucore.BindGroupLayoutID, error) {
	if desc == nil {
		return gpucore.InvalidID, fmt.Errorf("nil bind group layout descriptor")
	}

	entries := make([]types.BindGroupLayoutEntry, len(desc.Entries))
	for i, entry := range desc.Entries {
		entries[i] = convertBindGroupLayoutEntry(entry)
	}

	backendDesc := &types.BindGroupLayoutDescriptor{
		Label:   desc.Label,
		Entries: entries,
	}

	layout, err := a.backend.CreateBindGroupLayout(a.device, backendDesc)
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
func (a *GoGPUAdapter) DestroyBindGroupLayout(id gpucore.BindGroupLayoutID) {
	a.mu.Lock()
	layout, ok := a.bindGroupLayouts[id]
	if ok {
		delete(a.bindGroupLayouts, id)
	}
	a.mu.Unlock()

	if ok {
		a.backend.ReleaseBindGroupLayout(layout)
	}
}

// CreatePipelineLayout creates a pipeline layout.
func (a *GoGPUAdapter) CreatePipelineLayout(layouts []gpucore.BindGroupLayoutID) (gpucore.PipelineLayoutID, error) {
	a.mu.RLock()
	backendLayouts := make([]types.BindGroupLayout, len(layouts))
	for i, id := range layouts {
		layout, ok := a.bindGroupLayouts[id]
		if !ok {
			a.mu.RUnlock()
			return gpucore.InvalidID, fmt.Errorf("bind group layout %d not found", id)
		}
		backendLayouts[i] = layout
	}
	a.mu.RUnlock()

	backendDesc := &types.PipelineLayoutDescriptor{
		Label:            "",
		BindGroupLayouts: backendLayouts,
	}

	pipelineLayout, err := a.backend.CreatePipelineLayout(a.device, backendDesc)
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
func (a *GoGPUAdapter) DestroyPipelineLayout(id gpucore.PipelineLayoutID) {
	a.mu.Lock()
	layout, ok := a.pipelineLayouts[id]
	if ok {
		delete(a.pipelineLayouts, id)
	}
	a.mu.Unlock()

	if ok {
		a.backend.ReleasePipelineLayout(layout)
	}
}

// CreateComputePipeline creates a compute pipeline.
//
// NOTE: This method returns ErrNotImplemented because gpu.Backend doesn't
// currently support compute pipelines. Only render pipelines are available.
func (a *GoGPUAdapter) CreateComputePipeline(desc *gpucore.ComputePipelineDesc) (gpucore.ComputePipelineID, error) {
	if desc == nil {
		return gpucore.InvalidID, fmt.Errorf("nil compute pipeline descriptor")
	}

	// gpu.Backend only supports render pipelines via CreateRenderPipeline
	// Compute pipelines are not yet available
	return gpucore.InvalidID, fmt.Errorf("%w: compute pipelines not supported by gpu.Backend", ErrNotImplemented)
}

// DestroyComputePipeline releases a compute pipeline.
func (a *GoGPUAdapter) DestroyComputePipeline(id gpucore.ComputePipelineID) {
	a.mu.Lock()
	_, ok := a.computePipelines[id]
	if ok {
		delete(a.computePipelines, id)
	}
	a.mu.Unlock()

	// Note: Compute pipelines are not supported, so nothing to release
}

// CreateBindGroup creates a bind group.
func (a *GoGPUAdapter) CreateBindGroup(layout gpucore.BindGroupLayoutID, entries []gpucore.BindGroupEntry) (gpucore.BindGroupID, error) {
	a.mu.RLock()
	backendLayout, ok := a.bindGroupLayouts[layout]
	if !ok {
		a.mu.RUnlock()
		return gpucore.InvalidID, fmt.Errorf("bind group layout %d not found", layout)
	}

	backendEntries := make([]types.BindGroupEntry, len(entries))
	for i, entry := range entries {
		backendEntry, err := a.convertBindGroupEntry(entry)
		if err != nil {
			a.mu.RUnlock()
			return gpucore.InvalidID, fmt.Errorf("failed to convert bind group entry %d: %w", entry.Binding, err)
		}
		backendEntries[i] = backendEntry
	}
	a.mu.RUnlock()

	backendDesc := &types.BindGroupDescriptor{
		Label:   "",
		Layout:  backendLayout,
		Entries: backendEntries,
	}

	bindGroup, err := a.backend.CreateBindGroup(a.device, backendDesc)
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
func (a *GoGPUAdapter) DestroyBindGroup(id gpucore.BindGroupID) {
	a.mu.Lock()
	group, ok := a.bindGroups[id]
	if ok {
		delete(a.bindGroups, id)
	}
	a.mu.Unlock()

	if ok {
		a.backend.ReleaseBindGroup(group)
	}
}

// === Command Recording and Execution ===

// BeginComputePass begins a compute pass.
//
// NOTE: This returns a no-op encoder because gpu.Backend doesn't currently
// support compute passes. Only render passes are available.
func (a *GoGPUAdapter) BeginComputePass() gpucore.ComputePassEncoder {
	// gpu.Backend doesn't support compute passes
	// Return a no-op encoder
	return &goGPUComputePassEncoder{adapter: a}
}

// Submit submits recorded commands to the GPU.
//
// NOTE: This is a no-op because compute passes are not supported.
// When compute support is added to gpu.Backend, this will be implemented.
func (a *GoGPUAdapter) Submit() {
	// gpu.Backend doesn't support compute passes
	// Nothing to submit
}

// WaitIdle waits for all GPU operations to complete.
//
// NOTE: This is a no-op because compute passes are not supported.
// When compute support is added to gpu.Backend, this will be implemented.
func (a *GoGPUAdapter) WaitIdle() {
	// gpu.Backend doesn't expose a wait/sync mechanism for compute
	// Nothing to wait for
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
	case gpucore.TextureFormatBGRA8Unorm:
		return types.TextureFormatBGRA8Unorm
	default:
		return types.TextureFormatRGBA8Unorm
	}
}

// safeIntToUint32 safely converts int to uint32.
// Returns 0 for negative values and clamps values exceeding uint32 max.
func safeIntToUint32(v int) uint32 {
	if v < 0 {
		return 0
	}
	if v > int(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(v)
}

// getBytesPerPixel returns the bytes per pixel for a texture format.
func getBytesPerPixel(format gpucore.TextureFormat) uint32 {
	switch format {
	case gpucore.TextureFormatR8Unorm:
		return 1
	case gpucore.TextureFormatR32Float:
		return 4
	case gpucore.TextureFormatRG32Float:
		return 8
	case gpucore.TextureFormatRGBA8Unorm, gpucore.TextureFormatRGBA8UnormSRGB,
		gpucore.TextureFormatBGRA8Unorm, gpucore.TextureFormatBGRA8UnormSRGB:
		return 4
	case gpucore.TextureFormatRGBA32Float:
		return 16
	default:
		return 4 // Default to RGBA8
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
		// Note: types.BindGroupLayoutEntry doesn't have a Storage field
		// for storage textures. This would need to be added to gpu/types.
		// For now, we leave it as a buffer with undefined type.
		result.Buffer = &types.BufferBindingLayout{
			Type: types.BufferBindingTypeUndefined,
		}
	}

	return result
}

// convertBindGroupEntry converts gpucore.BindGroupEntry to types.BindGroupEntry.
// Must be called with mu.RLock held.
func (a *GoGPUAdapter) convertBindGroupEntry(entry gpucore.BindGroupEntry) (types.BindGroupEntry, error) {
	result := types.BindGroupEntry{
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
	} else if entry.Texture != gpucore.InvalidID {
		texture, ok := a.textures[entry.Texture]
		if !ok {
			return result, fmt.Errorf("texture %d not found", entry.Texture)
		}

		// Create texture view for binding
		view := a.backend.CreateTextureView(texture, nil)
		result.TextureView = view
	}

	return result, nil
}

// === Compute Pass Encoder ===

// goGPUComputePassEncoder implements gpucore.ComputePassEncoder.
// This is a no-op implementation since gpu.Backend doesn't support compute passes.
type goGPUComputePassEncoder struct {
	adapter *GoGPUAdapter
}

// SetPipeline sets the active compute pipeline.
// This is a no-op since compute is not supported.
func (e *goGPUComputePassEncoder) SetPipeline(_ gpucore.ComputePipelineID) {
	// No-op: compute not supported
}

// SetBindGroup sets a bind group at the specified index.
// This is a no-op since compute is not supported.
func (e *goGPUComputePassEncoder) SetBindGroup(_ uint32, _ gpucore.BindGroupID) {
	// No-op: compute not supported
}

// Dispatch dispatches compute workgroups.
// This is a no-op since compute is not supported.
func (e *goGPUComputePassEncoder) Dispatch(_, _, _ uint32) {
	// No-op: compute not supported
}

// End finishes the compute pass.
// This is a no-op since compute is not supported.
func (e *goGPUComputePassEncoder) End() {
	// No-op: compute not supported
}

// Ensure GoGPUAdapter implements gpucore.GPUAdapter.
var _ gpucore.GPUAdapter = (*GoGPUAdapter)(nil)
