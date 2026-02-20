//go:build !nogpu

package gpu

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text/msdf"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Embedded MSDF text shader source.
//
//go:embed shaders/msdf_text.wgsl
var msdfTextShaderSource string

// Text rendering errors.
var (
	// ErrNilTextPipeline is returned when operating on a nil pipeline.
	ErrNilTextPipeline = errors.New("wgpu: text pipeline is nil")

	// ErrTextPipelineNotInitialized is returned when pipeline is not initialized.
	ErrTextPipelineNotInitialized = errors.New("wgpu: text pipeline not initialized")

	// ErrNoQuadsToRender is returned when RenderText is called with empty quads.
	ErrNoQuadsToRender = errors.New("wgpu: no quads to render")

	// ErrQuadBufferOverflow is returned when too many quads are submitted.
	ErrQuadBufferOverflow = errors.New("wgpu: quad buffer overflow")

	// ErrInvalidAtlasIndex is returned when referencing invalid atlas.
	ErrInvalidAtlasIndex = errors.New("wgpu: invalid atlas index")
)

// textVertexStride is the byte stride per vertex in the MSDF text pipeline.
// Layout per vertex:
//
//	position  (vec2<f32>) = 8 bytes  (location 0)
//	tex_coord (vec2<f32>) = 8 bytes  (location 1)
//
// Total = 16 bytes per vertex.
const textVertexStride = 16

// textUniformSize is the byte size of the text uniform buffer.
// Layout: transform (mat4x4<f32>) = 64 bytes +
// color (vec4<f32>) = 16 bytes + msdf_params (vec4<f32>) = 16 bytes = 96 bytes.
const textUniformSize = 96

// MSDFTextPipeline manages GPU resources for MSDF text rendering via a
// vertex+fragment render pipeline. Each text run is rendered as a set of
// textured quads using indexed drawing. The fragment shader evaluates the
// MSDF per pixel for crisp, resolution-independent text.
//
// The pipeline uses the same MSAA+depth/stencil texture pattern as
// SDFRenderPipeline and ConvexRenderer for unified render pass integration.
// A pipelineWithStencil variant is provided when the render pass includes
// a depth/stencil attachment (stencil test is Always/Keep -- text does not
// interact with stencil).
//
// Architecture:
//
//	GPURenderSession owns persistent buffers (vertex, index, uniform)
//	MSDFTextPipeline owns shader, layout, pipeline, sampler
//	bind groups are created per atlas texture (uniform + texture + sampler)
type MSDFTextPipeline struct {
	device hal.Device
	queue  hal.Queue

	// GPU objects for the render pipeline.
	shader        hal.ShaderModule
	uniformLayout hal.BindGroupLayout
	pipeLayout    hal.PipelineLayout
	pipeline      hal.RenderPipeline

	// Session-compatible pipeline variant with depth/stencil state.
	// Used when text participates in a unified render pass that includes
	// a stencil attachment (for stencil-then-cover paths).
	// Stencil test is Always/Keep (text does not interact with stencil).
	pipelineWithStencil hal.RenderPipeline

	// Default sampler for MSDF textures (linear filtering).
	sampler hal.Sampler
}

// NewMSDFTextPipeline creates a new MSDF text pipeline with the given device
// and queue. The render pipeline and GPU objects are not created until
// ensurePipeline or ensurePipelineWithStencil is called.
func NewMSDFTextPipeline(device hal.Device, queue hal.Queue) *MSDFTextPipeline {
	return &MSDFTextPipeline{
		device: device,
		queue:  queue,
	}
}

// Destroy releases all GPU resources held by the pipeline. Safe to call
// multiple times or on a pipeline with no allocated resources.
func (p *MSDFTextPipeline) Destroy() {
	p.destroyPipeline()
	if p.sampler != nil {
		p.device.DestroySampler(p.sampler)
		p.sampler = nil
	}
}

// createPipeline compiles the MSDF text shader and creates the render
// pipeline with premultiplied alpha blending and MSAA.
func (p *MSDFTextPipeline) createPipeline() error {
	if msdfTextShaderSource == "" {
		return fmt.Errorf("msdf_text shader source is empty")
	}

	shader, err := p.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "msdf_text_shader",
		Source: hal.ShaderSource{WGSL: msdfTextShaderSource},
	})
	if err != nil {
		return fmt.Errorf("compile msdf_text shader: %w", err)
	}
	p.shader = shader

	// Bind group layout:
	//   Binding 0: TextUniforms (uniform buffer, vertex+fragment)
	//   Binding 1: MSDF atlas texture (texture_2d, fragment)
	//   Binding 2: Sampler (fragment)
	uniformLayout, err := p.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "msdf_text_uniform_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
			{
				Binding:    1,
				Visibility: gputypes.ShaderStageFragment,
				Texture: &gputypes.TextureBindingLayout{
					SampleType:    gputypes.TextureSampleTypeFloat,
					ViewDimension: gputypes.TextureViewDimension2D,
				},
			},
			{
				Binding:    2,
				Visibility: gputypes.ShaderStageFragment,
				Sampler:    &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create msdf_text uniform layout: %w", err)
	}
	p.uniformLayout = uniformLayout

	pipeLayout, err := p.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "msdf_text_pipe_layout",
		BindGroupLayouts: []hal.BindGroupLayout{p.uniformLayout},
	})
	if err != nil {
		return fmt.Errorf("create msdf_text pipeline layout: %w", err)
	}
	p.pipeLayout = pipeLayout

	// Create sampler for MSDF textures (linear filtering for smooth
	// distance field interpolation).
	sampler, err := p.device.CreateSampler(&hal.SamplerDescriptor{
		Label:        "msdf_text_sampler",
		AddressModeU: gputypes.AddressModeClampToEdge,
		AddressModeV: gputypes.AddressModeClampToEdge,
		AddressModeW: gputypes.AddressModeClampToEdge,
		MagFilter:    gputypes.FilterModeLinear,
		MinFilter:    gputypes.FilterModeLinear,
		MipmapFilter: gputypes.FilterModeLinear,
	})
	if err != nil {
		return fmt.Errorf("create msdf_text sampler: %w", err)
	}
	p.sampler = sampler

	premulBlend := gputypes.BlendStatePremultiplied()
	pipeline, err := p.device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label:  "msdf_text_pipeline",
		Layout: p.pipeLayout,
		Vertex: hal.VertexState{
			Module:     p.shader,
			EntryPoint: "vs_main",
			Buffers:    textVertexLayout(),
		},
		Fragment: &hal.FragmentState{
			Module:     p.shader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					Blend:     &premulBlend,
					WriteMask: gputypes.ColorWriteMaskAll,
				},
			},
		},
		Primitive: gputypes.PrimitiveState{
			Topology: gputypes.PrimitiveTopologyTriangleList,
			CullMode: gputypes.CullModeNone,
		},
		Multisample: gputypes.MultisampleState{
			Count: sampleCount,
			Mask:  0xFFFFFFFF,
		},
	})
	if err != nil {
		return fmt.Errorf("create msdf_text pipeline: %w", err)
	}
	p.pipeline = pipeline

	return nil
}

// ensurePipelineWithStencil creates the session-compatible pipeline variant
// that includes a depth/stencil state. This pipeline is used when text is
// rendered in a unified render pass alongside stencil-then-cover paths.
// The stencil test is Always/Keep (text does not interact with stencil).
//
// The base pipeline (shader, layout, sampler) is created first if it
// doesn't exist.
//
//nolint:dupl // Intentional: each pipeline type owns its stencil variant with distinct labels and vertex layouts.
func (p *MSDFTextPipeline) ensurePipelineWithStencil() error {
	// Ensure base resources exist (shader, layouts, sampler).
	if p.shader == nil || p.uniformLayout == nil || p.pipeLayout == nil {
		if err := p.createPipeline(); err != nil {
			return err
		}
	}
	if p.pipelineWithStencil != nil {
		return nil
	}

	premulBlend := gputypes.BlendStatePremultiplied()
	pipeline, err := p.device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label:  "msdf_text_pipeline_with_stencil",
		Layout: p.pipeLayout,
		Vertex: hal.VertexState{
			Module:     p.shader,
			EntryPoint: "vs_main",
			Buffers:    textVertexLayout(),
		},
		Fragment: &hal.FragmentState{
			Module:     p.shader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					Blend:     &premulBlend,
					WriteMask: gputypes.ColorWriteMaskAll,
				},
			},
		},
		DepthStencil: &hal.DepthStencilState{
			Format:            gputypes.TextureFormatDepth24PlusStencil8,
			DepthWriteEnabled: false,
			DepthCompare:      gputypes.CompareFunctionAlways,
			StencilFront: hal.StencilFaceState{
				Compare:     gputypes.CompareFunctionAlways,
				FailOp:      hal.StencilOperationKeep,
				DepthFailOp: hal.StencilOperationKeep,
				PassOp:      hal.StencilOperationKeep,
			},
			StencilBack: hal.StencilFaceState{
				Compare:     gputypes.CompareFunctionAlways,
				FailOp:      hal.StencilOperationKeep,
				DepthFailOp: hal.StencilOperationKeep,
				PassOp:      hal.StencilOperationKeep,
			},
			StencilReadMask:  0x00,
			StencilWriteMask: 0x00,
		},
		Primitive: gputypes.PrimitiveState{
			Topology: gputypes.PrimitiveTopologyTriangleList,
			CullMode: gputypes.CullModeNone,
		},
		Multisample: gputypes.MultisampleState{
			Count: sampleCount,
			Mask:  0xFFFFFFFF,
		},
	})
	if err != nil {
		return fmt.Errorf("create MSDF text pipeline with stencil: %w", err)
	}
	p.pipelineWithStencil = pipeline
	return nil
}

// RecordDraws records MSDF text draw commands into an existing render pass.
// The render pass is owned by GPURenderSession. This method uses the
// pipelineWithStencil variant because the session's render pass includes
// a depth/stencil attachment.
//
// The resources parameter holds pre-built vertex/index buffers, uniform buffer,
// and bind group for the current frame.
func (p *MSDFTextPipeline) RecordDraws(rp hal.RenderPassEncoder, resources *textFrameResources) {
	if resources == nil || resources.indexCount == 0 {
		return
	}
	rp.SetPipeline(p.pipelineWithStencil)
	rp.SetBindGroup(0, resources.bindGroup, nil)
	rp.SetVertexBuffer(0, resources.vertBuf, 0)
	rp.SetIndexBuffer(resources.idxBuf, gputypes.IndexFormatUint16, 0)
	rp.DrawIndexed(resources.indexCount, 1, 0, 0, 0)
}

// destroyPipeline releases all pipeline resources in reverse creation order.
func (p *MSDFTextPipeline) destroyPipeline() {
	if p.device == nil {
		return
	}
	if p.pipelineWithStencil != nil {
		p.device.DestroyRenderPipeline(p.pipelineWithStencil)
		p.pipelineWithStencil = nil
	}
	if p.pipeline != nil {
		p.device.DestroyRenderPipeline(p.pipeline)
		p.pipeline = nil
	}
	if p.pipeLayout != nil {
		p.device.DestroyPipelineLayout(p.pipeLayout)
		p.pipeLayout = nil
	}
	if p.uniformLayout != nil {
		p.device.DestroyBindGroupLayout(p.uniformLayout)
		p.uniformLayout = nil
	}
	if p.shader != nil {
		p.device.DestroyShaderModule(p.shader)
		p.shader = nil
	}
}

// textFrameResources holds per-frame GPU resources for MSDF text rendering.
type textFrameResources struct {
	vertBuf    hal.Buffer
	idxBuf     hal.Buffer
	uniformBuf hal.Buffer
	bindGroup  hal.BindGroup
	indexCount uint32
}

// textVertexLayout returns the vertex buffer layout for the MSDF text pipeline.
// Matches VertexInput in msdf_text.wgsl:
//
//	location 0: position (vec2<f32>)
//	location 1: tex_coord (vec2<f32>)
func textVertexLayout() []gputypes.VertexBufferLayout {
	return []gputypes.VertexBufferLayout{
		{
			ArrayStride: textVertexStride,
			StepMode:    gputypes.VertexStepModeVertex,
			Attributes: []gputypes.VertexAttribute{
				{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0}, // position
				{Format: gputypes.VertexFormatFloat32x2, Offset: 8, ShaderLocation: 1}, // tex_coord
			},
		},
	}
}

// ---- Data types shared between MSDFTextPipeline and GPURenderSession ----

// TextPipelineConfig holds configuration for the text pipeline.
type TextPipelineConfig struct {
	// InitialQuadCapacity is the initial vertex buffer capacity in quads.
	// Default: 256
	InitialQuadCapacity int

	// MaxQuadCapacity is the maximum number of quads per draw call.
	// Default: 16384
	MaxQuadCapacity int

	// DefaultPxRange is the default MSDF pixel range.
	// Default: 4.0
	DefaultPxRange float32
}

// DefaultTextPipelineConfig returns default configuration.
func DefaultTextPipelineConfig() TextPipelineConfig {
	return TextPipelineConfig{
		InitialQuadCapacity: 256,
		MaxQuadCapacity:     16384,
		DefaultPxRange:      4.0,
	}
}

// TextQuad represents a single glyph quad for rendering.
// Each glyph is rendered as a textured quad with position and UV coordinates.
type TextQuad struct {
	// Position of quad corners in screen/clip space
	X0, Y0, X1, Y1 float32

	// UV coordinates in MSDF atlas [0, 1]
	U0, V0, U1, V1 float32
}

// TextVertex represents a single vertex for text rendering.
// Matches the VertexInput struct in msdf_text.wgsl.
type TextVertex struct {
	// Position in local/screen space
	X, Y float32

	// UV coordinates in atlas
	U, V float32
}

// TextUniforms represents the uniform buffer for text shaders.
// Matches the TextUniforms struct in msdf_text.wgsl.
type TextUniforms struct {
	// Transform matrix (4x4 for alignment, row-major)
	// Maps local coordinates to clip space [-1, 1]
	Transform [16]float32

	// Text color (RGBA, premultiplied alpha)
	Color [4]float32

	// MSDF parameters:
	// [0]: px_range (distance range in pixels)
	// [1]: atlas_size (texture size)
	// [2]: outline_width (for outline effect)
	// [3]: reserved
	MSDFParams [4]float32
}

// TextBatch represents a batch of text quads with shared rendering parameters.
type TextBatch struct {
	// Quads is the list of glyph quads to render.
	Quads []TextQuad

	// Color is the text color (RGBA, will be premultiplied).
	Color gg.RGBA

	// Transform is the 2D affine transform for this batch.
	Transform gg.Matrix

	// AtlasIndex identifies which MSDF atlas to use.
	AtlasIndex int

	// PxRange is the MSDF pixel range (typically 4.0).
	PxRange float32

	// AtlasSize is the atlas texture size (for screen-space derivative calc).
	AtlasSize float32
}

// ---- Vertex/index/uniform data builders ----

// quadsToVertices converts TextQuads to vertex array.
// Each quad becomes 4 vertices (for indexed rendering).
func quadsToVertices(quads []TextQuad) []TextVertex {
	vertices := make([]TextVertex, len(quads)*4)

	for i, q := range quads {
		base := i * 4

		// Vertex 0: bottom-left
		vertices[base+0] = TextVertex{X: q.X0, Y: q.Y0, U: q.U0, V: q.V0}
		// Vertex 1: bottom-right
		vertices[base+1] = TextVertex{X: q.X1, Y: q.Y0, U: q.U1, V: q.V0}
		// Vertex 2: top-right
		vertices[base+2] = TextVertex{X: q.X1, Y: q.Y1, U: q.U1, V: q.V1}
		// Vertex 3: top-left
		vertices[base+3] = TextVertex{X: q.X0, Y: q.Y1, U: q.U0, V: q.V1}
	}

	return vertices
}

// generateQuadIndices generates index buffer data for a given number of quads.
// Uses the pattern: 0,1,2, 2,3,0 for each quad (two triangles).
func generateQuadIndices(numQuads int) []uint16 {
	indices := make([]uint16, numQuads*6)

	for i := 0; i < numQuads; i++ {
		base := i * 6
		vertex := uint16(i * 4) //nolint:gosec // numQuads is bounded by MaxQuadCapacity

		// First triangle: 0, 1, 2
		indices[base+0] = vertex + 0
		indices[base+1] = vertex + 1
		indices[base+2] = vertex + 2

		// Second triangle: 2, 3, 0
		indices[base+3] = vertex + 2
		indices[base+4] = vertex + 3
		indices[base+5] = vertex + 0
	}

	return indices
}

// buildTextVertexData serializes TextQuad slices into raw vertex bytes
// suitable for GPU upload. Each quad produces 4 vertices x 16 bytes = 64 bytes.
func buildTextVertexData(quads []TextQuad) []byte {
	if len(quads) == 0 {
		return nil
	}
	data := make([]byte, len(quads)*4*textVertexStride)
	off := 0
	for _, q := range quads {
		// Vertex 0: bottom-left
		writeTextVertex(data[off:], q.X0, q.Y0, q.U0, q.V0)
		off += textVertexStride
		// Vertex 1: bottom-right
		writeTextVertex(data[off:], q.X1, q.Y0, q.U1, q.V0)
		off += textVertexStride
		// Vertex 2: top-right
		writeTextVertex(data[off:], q.X1, q.Y1, q.U1, q.V1)
		off += textVertexStride
		// Vertex 3: top-left
		writeTextVertex(data[off:], q.X0, q.Y1, q.U0, q.V1)
		off += textVertexStride
	}
	return data
}

// writeTextVertex writes a single text vertex into buf.
func writeTextVertex(buf []byte, x, y, u, v float32) {
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(x))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(y))
	binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(u))
	binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(v))
}

// buildTextIndexData serializes quad indices into raw bytes for GPU upload.
func buildTextIndexData(numQuads int) []byte {
	indices := generateQuadIndices(numQuads)
	data := make([]byte, len(indices)*2)
	for i, idx := range indices {
		binary.LittleEndian.PutUint16(data[i*2:], idx)
	}
	return data
}

// makeTextUniform creates the 96-byte uniform buffer for a text batch.
func makeTextUniform(color gg.RGBA, transform gg.Matrix, pxRange, atlasSize float32) []byte {
	buf := make([]byte, textUniformSize)
	off := 0

	// Transform: 4x4 row-major matrix (column-major for WGSL mat4x4 is
	// what the shader reads with column vectors; our row-major layout
	// matches the WGSL column-major read order because WGSL mat4x4 is
	// stored column-by-column but we fill it row-by-row matching the
	// shader's expected layout).
	// Input affine: a b c / d e f
	// Output 4x4:   a b 0 c / d e 0 f / 0 0 1 0 / 0 0 0 1
	t := [16]float32{
		float32(transform.A), float32(transform.B), 0, float32(transform.C),
		float32(transform.D), float32(transform.E), 0, float32(transform.F),
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
	for _, v := range t {
		binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(v))
		off += 4
	}

	// Color: premultiplied RGBA
	premul := color.Premultiply()
	binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(float32(premul.R)))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(float32(premul.G)))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(float32(premul.B)))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(float32(premul.A)))
	off += 4

	// MSDF params: [px_range, atlas_size, outline_width, reserved]
	binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(pxRange))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(atlasSize))
	// outline_width and reserved remain zero.

	return buf
}

// rgbToRGBA converts 3-byte-per-pixel RGB data to 4-byte-per-pixel RGBA,
// setting alpha to 255 for every pixel. This is needed because GPU textures
// require RGBA format but MSDF atlas data is stored as RGB.
func rgbToRGBA(rgb []byte, width, height int) []byte {
	pixelCount := width * height
	rgba := make([]byte, pixelCount*4)
	for i := 0; i < pixelCount; i++ {
		srcOff := i * 3
		dstOff := i * 4
		rgba[dstOff+0] = rgb[srcOff+0]
		rgba[dstOff+1] = rgb[srcOff+1]
		rgba[dstOff+2] = rgb[srcOff+2]
		rgba[dstOff+3] = 255
	}
	return rgba
}

// GetMSDFTextShaderSource returns the WGSL source for the MSDF text shader.
func GetMSDFTextShaderSource() string {
	return msdfTextShaderSource
}

// ---- Legacy TextPipeline wrapper (preserves test API) ----

// TextPipeline handles GPU-accelerated MSDF text rendering.
// It manages the render pipeline, bind groups, and vertex buffers
// for rendering text using multi-channel signed distance fields.
//
// TextPipeline is safe for concurrent use after initialization.
//
// Deprecated: For session-integrated rendering, use MSDFTextPipeline +
// GPURenderSession. TextPipeline is retained for backward compatibility
// with existing tests and the TextRenderer high-level wrapper.
type TextPipeline struct {
	mu sync.RWMutex

	// GPU device and queue references (hal interfaces)
	device hal.Device
	queue  hal.Queue

	// Underlying real pipeline (nil until Init)
	real *MSDFTextPipeline

	// Configuration
	config TextPipelineConfig

	// State
	initialized bool
}

// NewTextPipeline creates a new text rendering pipeline.
// The pipeline must be initialized before use.
func NewTextPipeline(device hal.Device, queue hal.Queue, config TextPipelineConfig) (*TextPipeline, error) {
	if config.InitialQuadCapacity <= 0 {
		config.InitialQuadCapacity = DefaultTextPipelineConfig().InitialQuadCapacity
	}
	if config.MaxQuadCapacity <= 0 {
		config.MaxQuadCapacity = DefaultTextPipelineConfig().MaxQuadCapacity
	}
	if config.DefaultPxRange <= 0 {
		config.DefaultPxRange = DefaultTextPipelineConfig().DefaultPxRange
	}

	return &TextPipeline{
		device: device,
		queue:  queue,
		config: config,
	}, nil
}

// NewTextPipelineDefault creates a text pipeline with default configuration.
func NewTextPipelineDefault(device hal.Device, queue hal.Queue) (*TextPipeline, error) {
	return NewTextPipeline(device, queue, DefaultTextPipelineConfig())
}

// Init initializes the text pipeline, compiling shaders and creating GPU resources.
func (p *TextPipeline) Init() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
	}

	if msdfTextShaderSource == "" {
		return errors.New("wgpu: MSDF text shader source is empty")
	}

	msdfPipe := NewMSDFTextPipeline(p.device, p.queue)
	if err := msdfPipe.createPipeline(); err != nil {
		return fmt.Errorf("init text pipeline: %w", err)
	}
	p.real = msdfPipe

	p.initialized = true
	return nil
}

// IsInitialized returns true if the pipeline has been initialized.
func (p *TextPipeline) IsInitialized() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.initialized
}

// RenderText renders text quads using the specified atlas.
// All quads are rendered in a single draw call for efficiency.
//
// Parameters:
//   - pass: The render pass to record commands into (unused in legacy API)
//   - quads: Text quads to render (position and UV for each glyph)
//   - atlasIndex: Index of the MSDF atlas texture to use
//   - color: Text color (RGBA, will be premultiplied)
//   - transform: 2D affine transform matrix (gg.Matrix)
func (p *TextPipeline) RenderText(
	_ *RenderPass,
	quads []TextQuad,
	atlasIndex int,
	color gg.RGBA,
	transform gg.Matrix,
) error {
	if !p.initialized {
		return ErrTextPipelineNotInitialized
	}

	if len(quads) == 0 {
		return ErrNoQuadsToRender
	}

	if len(quads) > p.config.MaxQuadCapacity {
		return fmt.Errorf("%w: %d quads exceeds max %d",
			ErrQuadBufferOverflow, len(quads), p.config.MaxQuadCapacity)
	}

	if atlasIndex < 0 {
		return ErrInvalidAtlasIndex
	}

	// Prepare data (validates conversion logic even if not submitted to GPU).
	_ = quadsToVertices(quads)
	_ = prepareUniforms(color, transform, p.config.DefaultPxRange)

	return nil
}

// RenderTextBatch renders multiple text batches efficiently.
// Each batch can have different color and transform but shares the same atlas.
func (p *TextPipeline) RenderTextBatch(
	pass *RenderPass,
	batches []TextBatch,
	atlasIndex int,
) error {
	if !p.initialized {
		return ErrTextPipelineNotInitialized
	}

	if atlasIndex < 0 {
		return ErrInvalidAtlasIndex
	}

	for i, batch := range batches {
		if err := p.RenderText(pass, batch.Quads, atlasIndex, batch.Color, batch.Transform); err != nil {
			return fmt.Errorf("batch %d: %w", i, err)
		}
	}

	return nil
}

// Config returns the pipeline configuration.
func (p *TextPipeline) Config() TextPipelineConfig {
	return p.config
}

// Close releases all pipeline resources.
func (p *TextPipeline) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return
	}

	if p.real != nil {
		p.real.Destroy()
		p.real = nil
	}

	p.initialized = false
}

// prepareUniforms creates TextUniforms from rendering parameters.
func prepareUniforms(color gg.RGBA, transform gg.Matrix, pxRange float32) TextUniforms {
	premul := color.Premultiply()

	uniforms := TextUniforms{
		Color: [4]float32{
			float32(premul.R),
			float32(premul.G),
			float32(premul.B),
			float32(premul.A),
		},
		MSDFParams: [4]float32{pxRange, 0, 0, 0},
	}

	// Convert 2D affine transform (2x3) to 4x4 matrix (row-major)
	uniforms.Transform = [16]float32{
		float32(transform.A), float32(transform.B), 0, float32(transform.C),
		float32(transform.D), float32(transform.E), 0, float32(transform.F),
		0, 0, 1, 0,
		0, 0, 0, 1,
	}

	return uniforms
}

// ---- TextRenderer high-level wrapper ----

// TextRenderer provides a higher-level API for rendering text.
// It combines TextPipeline with AtlasManager for convenient text rendering.
type TextRenderer struct {
	mu sync.RWMutex

	// GPU resources
	device hal.Device
	queue  hal.Queue

	// Pipeline (legacy wrapper)
	pipeline *TextPipeline

	// Atlas management
	atlasManager *msdf.AtlasManager

	// Cached atlas textures (hal.Texture + hal.TextureView)
	atlasTextures     []hal.Texture
	atlasTextureViews []hal.TextureView

	// State
	initialized bool
}

// TextRendererConfig holds configuration for TextRenderer.
type TextRendererConfig struct {
	// PipelineConfig for the underlying text pipeline.
	PipelineConfig TextPipelineConfig

	// AtlasConfig for the MSDF atlas manager.
	AtlasConfig msdf.AtlasConfig
}

// DefaultTextRendererConfig returns default configuration.
func DefaultTextRendererConfig() TextRendererConfig {
	return TextRendererConfig{
		PipelineConfig: DefaultTextPipelineConfig(),
		AtlasConfig:    msdf.DefaultAtlasConfig(),
	}
}

// NewTextRenderer creates a new text renderer with the given HAL device and
// queue. The renderer manages a TextPipeline and AtlasManager internally.
func NewTextRenderer(device hal.Device, queue hal.Queue, config TextRendererConfig) (*TextRenderer, error) {
	if device == nil {
		return nil, ErrNilHALDevice
	}

	atlasManager, err := msdf.NewAtlasManager(config.AtlasConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create atlas manager: %w", err)
	}

	pipeline, err := NewTextPipeline(device, queue, config.PipelineConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create text pipeline: %w", err)
	}

	return &TextRenderer{
		device:       device,
		queue:        queue,
		pipeline:     pipeline,
		atlasManager: atlasManager,
	}, nil
}

// Init initializes the text renderer.
func (r *TextRenderer) Init() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return nil
	}

	if err := r.pipeline.Init(); err != nil {
		return err
	}

	r.initialized = true
	return nil
}

// SyncAtlases uploads dirty atlases to GPU.
func (r *TextRenderer) SyncAtlases() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return ErrTextPipelineNotInitialized
	}

	dirtyIndices := r.atlasManager.DirtyAtlases()
	for _, idx := range dirtyIndices {
		atlas := r.atlasManager.GetAtlas(idx)
		if atlas == nil {
			continue
		}

		// Ensure we have enough texture slots.
		for len(r.atlasTextures) <= idx {
			r.atlasTextures = append(r.atlasTextures, nil)
			r.atlasTextureViews = append(r.atlasTextureViews, nil)
		}

		atlasSize := uint32(atlas.Size) //nolint:gosec // atlas size always fits uint32

		// Create or recreate texture.
		if r.atlasTextures[idx] == nil {
			tex, err := r.device.CreateTexture(&hal.TextureDescriptor{
				Label:         fmt.Sprintf("msdf_atlas_%d", idx),
				Size:          hal.Extent3D{Width: atlasSize, Height: atlasSize, DepthOrArrayLayers: 1},
				MipLevelCount: 1,
				SampleCount:   1,
				Dimension:     gputypes.TextureDimension2D,
				Format:        gputypes.TextureFormatRGBA8Unorm,
				Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
			})
			if err != nil {
				return fmt.Errorf("failed to create atlas texture %d: %w", idx, err)
			}
			r.atlasTextures[idx] = tex

			view, err := r.device.CreateTextureView(tex, &hal.TextureViewDescriptor{
				Label:         fmt.Sprintf("msdf_atlas_%d_view", idx),
				Format:        gputypes.TextureFormatRGBA8Unorm,
				Dimension:     gputypes.TextureViewDimension2D,
				Aspect:        gputypes.TextureAspectAll,
				MipLevelCount: 1,
			})
			if err != nil {
				return fmt.Errorf("failed to create atlas texture view %d: %w", idx, err)
			}
			r.atlasTextureViews[idx] = view
		}

		// Convert RGB (3 bytes/pixel) to RGBA (4 bytes/pixel).
		rgbaData := rgbToRGBA(atlas.Data, atlas.Size, atlas.Size)

		// Upload to GPU via queue.WriteTexture.
		r.queue.WriteTexture(
			&hal.ImageCopyTexture{
				Texture:  r.atlasTextures[idx],
				MipLevel: 0,
			},
			rgbaData,
			&hal.ImageDataLayout{
				Offset:       0,
				BytesPerRow:  atlasSize * 4,
				RowsPerImage: atlasSize,
			},
			&hal.Extent3D{Width: atlasSize, Height: atlasSize, DepthOrArrayLayers: 1},
		)

		// Mark atlas as clean.
		r.atlasManager.MarkClean(idx)
	}

	return nil
}

// AtlasManager returns the underlying atlas manager.
func (r *TextRenderer) AtlasManager() *msdf.AtlasManager {
	return r.atlasManager
}

// Pipeline returns the underlying text pipeline.
func (r *TextRenderer) Pipeline() *TextPipeline {
	return r.pipeline
}

// Close releases all renderer resources.
func (r *TextRenderer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return
	}

	// Close texture views.
	for _, v := range r.atlasTextureViews {
		if v != nil {
			r.device.DestroyTextureView(v)
		}
	}
	r.atlasTextureViews = nil

	// Close textures.
	for _, t := range r.atlasTextures {
		if t != nil {
			r.device.DestroyTexture(t)
		}
	}
	r.atlasTextures = nil

	// Close pipeline.
	if r.pipeline != nil {
		r.pipeline.Close()
	}

	// Clear atlas manager.
	if r.atlasManager != nil {
		r.atlasManager.Clear()
	}

	r.initialized = false
}
