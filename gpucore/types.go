package gpucore

// Resource IDs
//
// These opaque IDs represent GPU resources. Each adapter implementation
// maintains a mapping between IDs and actual backend resources.
// IDs are uint64 to accommodate various backend handle sizes.

// BufferID is an opaque handle to a GPU buffer.
type BufferID uint64

// TextureID is an opaque handle to a GPU texture.
type TextureID uint64

// ShaderModuleID is an opaque handle to a compiled shader module.
type ShaderModuleID uint64

// ComputePipelineID is an opaque handle to a compute pipeline.
type ComputePipelineID uint64

// BindGroupLayoutID is an opaque handle to a bind group layout.
type BindGroupLayoutID uint64

// BindGroupID is an opaque handle to a bind group.
type BindGroupID uint64

// PipelineLayoutID is an opaque handle to a pipeline layout.
type PipelineLayoutID uint64

// InvalidID is the zero value, representing an invalid/null resource.
const InvalidID = 0

// BufferUsage is a bitmask specifying how a buffer will be used.
type BufferUsage uint32

// Buffer usage flags.
const (
	// BufferUsageMapRead indicates the buffer can be mapped for reading.
	BufferUsageMapRead BufferUsage = 1 << 0

	// BufferUsageMapWrite indicates the buffer can be mapped for writing.
	BufferUsageMapWrite BufferUsage = 1 << 1

	// BufferUsageCopySrc indicates the buffer can be used as a copy source.
	BufferUsageCopySrc BufferUsage = 1 << 2

	// BufferUsageCopyDst indicates the buffer can be used as a copy destination.
	BufferUsageCopyDst BufferUsage = 1 << 3

	// BufferUsageIndex indicates the buffer can be used as an index buffer.
	BufferUsageIndex BufferUsage = 1 << 4

	// BufferUsageVertex indicates the buffer can be used as a vertex buffer.
	BufferUsageVertex BufferUsage = 1 << 5

	// BufferUsageUniform indicates the buffer can be used as a uniform buffer.
	BufferUsageUniform BufferUsage = 1 << 6

	// BufferUsageStorage indicates the buffer can be used as a storage buffer.
	BufferUsageStorage BufferUsage = 1 << 7

	// BufferUsageIndirect indicates the buffer can be used for indirect dispatch/draw.
	BufferUsageIndirect BufferUsage = 1 << 8
)

// TextureFormat specifies the format of texture data.
type TextureFormat uint32

// Texture formats.
const (
	// TextureFormatRGBA8Unorm is 8-bit RGBA, normalized unsigned integer.
	TextureFormatRGBA8Unorm TextureFormat = iota + 1

	// TextureFormatRGBA8UnormSRGB is 8-bit RGBA, normalized unsigned integer in sRGB color space.
	TextureFormatRGBA8UnormSRGB

	// TextureFormatBGRA8Unorm is 8-bit BGRA, normalized unsigned integer.
	TextureFormatBGRA8Unorm

	// TextureFormatBGRA8UnormSRGB is 8-bit BGRA, normalized unsigned integer in sRGB color space.
	TextureFormatBGRA8UnormSRGB

	// TextureFormatR8Unorm is 8-bit red channel only, normalized unsigned integer.
	TextureFormatR8Unorm

	// TextureFormatR32Float is 32-bit red channel only, floating point.
	TextureFormatR32Float

	// TextureFormatRG32Float is 32-bit RG, floating point.
	TextureFormatRG32Float

	// TextureFormatRGBA32Float is 32-bit RGBA, floating point.
	TextureFormatRGBA32Float
)

// TextureUsage is a bitmask specifying how a texture will be used.
type TextureUsage uint32

// Texture usage flags.
const (
	// TextureUsageCopySrc indicates the texture can be used as a copy source.
	TextureUsageCopySrc TextureUsage = 1 << 0

	// TextureUsageCopyDst indicates the texture can be used as a copy destination.
	TextureUsageCopyDst TextureUsage = 1 << 1

	// TextureUsageTextureBinding indicates the texture can be bound as a sampled texture.
	TextureUsageTextureBinding TextureUsage = 1 << 2

	// TextureUsageStorageBinding indicates the texture can be bound as a storage texture.
	TextureUsageStorageBinding TextureUsage = 1 << 3

	// TextureUsageRenderAttachment indicates the texture can be used as a render target.
	TextureUsageRenderAttachment TextureUsage = 1 << 4
)

// BindingType specifies the type of a shader binding.
type BindingType uint32

// Binding types.
const (
	// BindingTypeUniformBuffer is a uniform buffer binding.
	BindingTypeUniformBuffer BindingType = iota + 1

	// BindingTypeStorageBuffer is a storage buffer binding (read-write).
	BindingTypeStorageBuffer

	// BindingTypeReadOnlyStorageBuffer is a read-only storage buffer binding.
	BindingTypeReadOnlyStorageBuffer

	// BindingTypeSampler is a texture sampler binding.
	BindingTypeSampler

	// BindingTypeSampledTexture is a sampled texture binding.
	BindingTypeSampledTexture

	// BindingTypeStorageTexture is a storage texture binding.
	BindingTypeStorageTexture
)

// ComputePipelineDesc describes a compute pipeline.
type ComputePipelineDesc struct {
	// Label is an optional debug label.
	Label string

	// Layout is the pipeline layout.
	Layout PipelineLayoutID

	// ShaderModule contains the compute shader.
	ShaderModule ShaderModuleID

	// EntryPoint is the name of the shader entry point function.
	EntryPoint string
}

// BindGroupLayoutDesc describes a bind group layout.
type BindGroupLayoutDesc struct {
	// Label is an optional debug label.
	Label string

	// Entries defines the bindings in this layout.
	Entries []BindGroupLayoutEntry
}

// BindGroupLayoutEntry describes a single binding in a bind group layout.
type BindGroupLayoutEntry struct {
	// Binding is the binding index.
	Binding uint32

	// Type is the type of resource bound at this index.
	Type BindingType

	// MinBindingSize is the minimum buffer size for buffer bindings.
	// Set to 0 for non-buffer bindings.
	MinBindingSize uint64
}

// BindGroupEntry describes a single binding in a bind group.
type BindGroupEntry struct {
	// Binding is the binding index.
	Binding uint32

	// Buffer is the buffer to bind (for buffer bindings).
	Buffer BufferID

	// Offset is the offset into the buffer.
	Offset uint64

	// Size is the size of the buffer range to bind.
	// Use 0 to bind the entire buffer from offset.
	Size uint64

	// Texture is the texture to bind (for texture bindings).
	Texture TextureID
}

// BindGroupDesc describes a bind group.
type BindGroupDesc struct {
	// Label is an optional debug label.
	Label string

	// Layout is the bind group layout.
	Layout BindGroupLayoutID

	// Entries are the resource bindings.
	Entries []BindGroupEntry
}

// GPU Data Structures
//
// These structures match the WGSL shader data layouts and are used
// for CPU-GPU data transfer. All structures use explicit padding
// for alignment compatibility.

// Segment represents a monotonic line segment for GPU processing.
// Must match the Segment struct in fine.wgsl.
type Segment struct {
	X0      float32 // Start X coordinate
	Y0      float32 // Start Y coordinate
	X1      float32 // End X coordinate
	Y1      float32 // End Y coordinate
	Winding int32   // Winding direction: +1 or -1
	TileY0  int32   // Starting tile Y (precomputed)
	TileY1  int32   // Ending tile Y (precomputed)
	Padding int32   // Padding for alignment
}

// TileSegmentRef maps a segment to a tile.
// Must match TileSegmentRef in coarse.wgsl.
type TileSegmentRef struct {
	TileX       uint32 // Tile X coordinate
	TileY       uint32 // Tile Y coordinate
	SegmentIdx  uint32 // Index into segments array
	WindingFlag uint32 // Whether this contributes winding (0 or 1)
}

// TileInfo contains tile processing information.
// Must match TileInfo in fine.wgsl.
type TileInfo struct {
	TileX    uint32 // Tile X coordinate
	TileY    uint32 // Tile Y coordinate
	StartIdx uint32 // Start index in tile_segments
	Count    uint32 // Number of segments for this tile
	Backdrop int32  // Accumulated winding from left
	Padding1 uint32 // Padding for alignment
	Padding2 uint32 // Padding for alignment
	Padding3 uint32 // Padding for alignment
}

// PathElement represents a path element for GPU processing.
// Must match PathElement in flatten.wgsl.
type PathElement struct {
	Verb       uint32 // Path verb type (0=MoveTo, 1=LineTo, 2=QuadTo, 3=CubicTo, 4=Close)
	PointStart uint32 // Start index in points array
	PointCount uint32 // Number of points for this element
	Padding    uint32
}

// AffineTransform represents an affine transform for GPU.
// Must match AffineTransform in flatten.wgsl.
// Matrix layout (column-major):
//
//	| A C E |
//	| B D F |
//	| 0 0 1 |
type AffineTransform struct {
	A        float32 // Scale X
	B        float32 // Shear Y
	C        float32 // Shear X
	D        float32 // Scale Y
	E        float32 // Translate X
	F        float32 // Translate Y
	Padding1 float32
	Padding2 float32
}

// FlattenConfig contains GPU flattening configuration.
// Must match FlattenConfig in flatten.wgsl.
type FlattenConfig struct {
	ElementCount   uint32  // Number of path elements
	Tolerance      float32 // Flattening tolerance
	MaxSegments    uint32  // Maximum total segments
	TileSize       uint32  // Tile size in pixels
	ViewportWidth  uint32  // Viewport width
	ViewportHeight uint32  // Viewport height
	Padding1       uint32
	Padding2       uint32
}

// CoarseConfig contains GPU coarse rasterization configuration.
// Must match CoarseConfig in coarse.wgsl.
type CoarseConfig struct {
	ViewportWidth  uint32 // Viewport width in pixels
	ViewportHeight uint32 // Viewport height in pixels
	TileColumns    uint32 // Number of tile columns
	TileRows       uint32 // Number of tile rows
	SegmentCount   uint32 // Number of segments to process
	MaxEntries     uint32 // Maximum number of tile entries
	Padding1       uint32
	Padding2       uint32
}

// FineConfig contains GPU fine rasterization configuration.
// Must match FineConfig in fine.wgsl.
type FineConfig struct {
	ViewportWidth  uint32 // Viewport width in pixels
	ViewportHeight uint32 // Viewport height in pixels
	TileColumns    uint32 // Number of tile columns
	TileRows       uint32 // Number of tile rows
	TileCount      uint32 // Number of tiles to process
	FillRule       uint32 // 0 = NonZero, 1 = EvenOdd
	Padding1       uint32
	Padding2       uint32
}

// CursorState tracks the cursor position per path element.
// Must match CursorState in flatten.wgsl.
type CursorState struct {
	CurX   float32 // Current cursor X
	CurY   float32 // Current cursor Y
	StartX float32 // Subpath start X (for Close)
	StartY float32 // Subpath start Y (for Close)
}

// SegmentCount holds segment count per path element.
// Must match SegmentCount in flatten.wgsl.
type SegmentCount struct {
	Count    uint32 // Number of segments for this element
	Offset   uint32 // Prefix sum offset
	Padding1 uint32
	Padding2 uint32
}

// FillRule specifies the fill rule for rendering.
type FillRule uint32

// Fill rule values.
const (
	// FillRuleNonZero uses the non-zero winding rule.
	FillRuleNonZero FillRule = 0

	// FillRuleEvenOdd uses the even-odd winding rule.
	FillRuleEvenOdd FillRule = 1
)

// TileSize is the size of a tile in pixels.
const TileSize = 16

// MaxSegmentsPerCurve is the maximum segments generated per curve.
const MaxSegmentsPerCurve = 64

// DefaultTolerance is the default flattening tolerance in pixels.
const DefaultTolerance float32 = 0.25
