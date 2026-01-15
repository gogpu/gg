package gpucore

// GPUAdapter abstracts over different GPU backend implementations.
//
// This interface is the core abstraction that allows the rendering pipeline
// to work with multiple backends (gogpu/wgpu HAL, gogpu/gogpu backends).
// Implementations must be thread-safe for concurrent use.
//
// Resource lifecycle:
//   - Resources are created via Create* methods
//   - Resources must be explicitly destroyed via Destroy* methods
//   - Destroying a resource while in use is undefined behavior
//   - IDs become invalid after destruction and must not be reused
type GPUAdapter interface {
	// === Capabilities ===

	// SupportsCompute returns whether compute shaders are supported.
	// If false, the pipeline will use CPU fallback for all stages.
	SupportsCompute() bool

	// MaxWorkgroupSize returns the maximum workgroup size in each dimension.
	// Typical values are [256, 256, 64] or [1024, 1024, 1024].
	MaxWorkgroupSize() [3]uint32

	// MaxBufferSize returns the maximum buffer size in bytes.
	// The pipeline will split buffers that exceed this limit.
	MaxBufferSize() uint64

	// === Shader Compilation ===

	// CreateShaderModule creates a shader module from SPIR-V bytecode.
	// The SPIR-V is compiled by naga before being passed here.
	//
	// Parameters:
	//   - spirv: SPIR-V bytecode as uint32 words
	//   - label: optional debug label
	//
	// Returns the module ID or an error if compilation fails.
	CreateShaderModule(spirv []uint32, label string) (ShaderModuleID, error)

	// DestroyShaderModule releases a shader module.
	DestroyShaderModule(id ShaderModuleID)

	// === Buffer Management ===

	// CreateBuffer creates a GPU buffer.
	//
	// Parameters:
	//   - size: buffer size in bytes
	//   - usage: buffer usage flags (bitmask of BufferUsage*)
	//
	// Returns the buffer ID or an error if allocation fails.
	CreateBuffer(size int, usage BufferUsage) (BufferID, error)

	// DestroyBuffer releases a GPU buffer.
	DestroyBuffer(id BufferID)

	// WriteBuffer writes data to a buffer.
	// The data is copied to the GPU immediately or staged for later upload.
	//
	// Parameters:
	//   - id: target buffer
	//   - offset: byte offset into the buffer
	//   - data: data to write
	WriteBuffer(id BufferID, offset uint64, data []byte)

	// ReadBuffer reads data from a buffer.
	// This may cause a GPU-CPU synchronization stall.
	//
	// Parameters:
	//   - id: source buffer
	//   - offset: byte offset into the buffer
	//   - size: number of bytes to read
	//
	// Returns the data or an error if reading fails.
	ReadBuffer(id BufferID, offset, size uint64) ([]byte, error)

	// === Texture Management ===

	// CreateTexture creates a GPU texture.
	//
	// Parameters:
	//   - width: texture width in pixels
	//   - height: texture height in pixels
	//   - format: pixel format
	//
	// Returns the texture ID or an error if allocation fails.
	CreateTexture(width, height int, format TextureFormat) (TextureID, error)

	// DestroyTexture releases a GPU texture.
	DestroyTexture(id TextureID)

	// WriteTexture writes data to a texture.
	// The data must match the texture format and dimensions.
	WriteTexture(id TextureID, data []byte)

	// ReadTexture reads data from a texture.
	// This may cause a GPU-CPU synchronization stall.
	ReadTexture(id TextureID) ([]byte, error)

	// === Pipeline Management ===

	// CreateBindGroupLayout creates a bind group layout.
	// Bind group layouts describe the structure of resource bindings.
	CreateBindGroupLayout(desc *BindGroupLayoutDesc) (BindGroupLayoutID, error)

	// DestroyBindGroupLayout releases a bind group layout.
	DestroyBindGroupLayout(id BindGroupLayoutID)

	// CreatePipelineLayout creates a pipeline layout.
	// Pipeline layouts combine multiple bind group layouts.
	//
	// Parameters:
	//   - layouts: bind group layouts used by the pipeline
	//
	// Returns the layout ID or an error if creation fails.
	CreatePipelineLayout(layouts []BindGroupLayoutID) (PipelineLayoutID, error)

	// DestroyPipelineLayout releases a pipeline layout.
	DestroyPipelineLayout(id PipelineLayoutID)

	// CreateComputePipeline creates a compute pipeline.
	CreateComputePipeline(desc *ComputePipelineDesc) (ComputePipelineID, error)

	// DestroyComputePipeline releases a compute pipeline.
	DestroyComputePipeline(id ComputePipelineID)

	// CreateBindGroup creates a bind group.
	// Bind groups bind actual resources to a bind group layout.
	//
	// Parameters:
	//   - layout: the bind group layout
	//   - entries: resource bindings
	//
	// Returns the bind group ID or an error if creation fails.
	CreateBindGroup(layout BindGroupLayoutID, entries []BindGroupEntry) (BindGroupID, error)

	// DestroyBindGroup releases a bind group.
	DestroyBindGroup(id BindGroupID)

	// === Command Recording and Execution ===

	// BeginComputePass begins a compute pass.
	// Returns an encoder for recording compute commands.
	// The encoder must be ended with ComputePassEncoder.End().
	BeginComputePass() ComputePassEncoder

	// Submit submits recorded commands to the GPU.
	// Call this after ending all compute passes to execute them.
	Submit()

	// WaitIdle waits for all GPU operations to complete.
	// Use sparingly as this causes a full GPU-CPU synchronization.
	WaitIdle()
}

// ComputePassEncoder records compute commands.
//
// Usage:
//  1. Obtain encoder from GPUAdapter.BeginComputePass()
//  2. Set pipeline and bind groups
//  3. Dispatch compute workgroups
//  4. Call End() to finish recording
//  5. Call GPUAdapter.Submit() to execute
//
// The encoder is single-use and cannot be reused after End().
type ComputePassEncoder interface {
	// SetPipeline sets the active compute pipeline.
	SetPipeline(pipeline ComputePipelineID)

	// SetBindGroup sets a bind group at the specified index.
	// Index must be less than the number of bind group layouts in the pipeline.
	SetBindGroup(index uint32, group BindGroupID)

	// Dispatch dispatches compute workgroups.
	// x, y, z are the number of workgroups in each dimension.
	// Total threads = x * y * z * workgroup_size.
	Dispatch(x, y, z uint32)

	// End finishes the compute pass.
	// After this call, the encoder cannot be used again.
	End()
}

// AdapterCapabilities describes GPU adapter capabilities.
type AdapterCapabilities struct {
	// SupportsCompute indicates compute shader support.
	SupportsCompute bool

	// MaxWorkgroupSizeX is the maximum workgroup size in X dimension.
	MaxWorkgroupSizeX uint32

	// MaxWorkgroupSizeY is the maximum workgroup size in Y dimension.
	MaxWorkgroupSizeY uint32

	// MaxWorkgroupSizeZ is the maximum workgroup size in Z dimension.
	MaxWorkgroupSizeZ uint32

	// MaxWorkgroupInvocations is the maximum total invocations per workgroup.
	MaxWorkgroupInvocations uint32

	// MaxBufferSize is the maximum buffer size in bytes.
	MaxBufferSize uint64

	// MaxStorageBufferBindingSize is the maximum storage buffer binding size.
	MaxStorageBufferBindingSize uint64

	// MaxComputeWorkgroupsPerDimension is the maximum workgroups per dispatch dimension.
	MaxComputeWorkgroupsPerDimension uint32
}
