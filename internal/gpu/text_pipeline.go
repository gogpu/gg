//go:build !nogpu

package gpu

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
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
	device      *wgpu.Device
	queue       *wgpu.Queue
	sampleCount uint32 // MSAA sample count (4 or 1), from GPUShared

	// GPU objects for the render pipeline.
	shader        *wgpu.ShaderModule
	uniformLayout *wgpu.BindGroupLayout
	pipeLayout    *wgpu.PipelineLayout
	pipeline      *wgpu.RenderPipeline

	// Session-compatible pipeline variant with depth/stencil state.
	// Used when text participates in a unified render pass that includes
	// a stencil attachment (for stencil-then-cover paths).
	// Stencil test is Always/Keep (text does not interact with stencil).
	pipelineWithStencil *wgpu.RenderPipeline

	// Depth-clipped pipeline variant (GPU-CLIP-003a).
	pipelineWithDepthClip *wgpu.RenderPipeline

	// Default sampler for MSDF textures (linear filtering).
	sampler *wgpu.Sampler

	// clipBindLayout is the shared @group(1) bind group layout for RRect clip.
	// Set by the session before ensurePipelineWithStencil.
	clipBindLayout *wgpu.BindGroupLayout
	// pipeLayoutHasClip tracks whether the current pipeLayout was created
	// with clipBindLayout included. If clipBindLayout is set after the
	// layout was created, the pipeline must be recreated.
	pipeLayoutHasClip bool
}

// NewMSDFTextPipeline creates a new MSDF text pipeline with the given device
// and queue. The render pipeline and GPU objects are not created until
// ensurePipeline or ensurePipelineWithStencil is called.
func NewMSDFTextPipeline(device *wgpu.Device, queue *wgpu.Queue, sampleCount uint32) *MSDFTextPipeline {
	return &MSDFTextPipeline{
		device:      device,
		queue:       queue,
		sampleCount: sampleCount,
	}
}

// SetClipBindLayout sets the bind group layout for the @group(1) RRect clip
// uniform. Must be called before ensurePipelineWithStencil. The layout is
// owned by the session and must not be destroyed by the pipeline.
func (p *MSDFTextPipeline) SetClipBindLayout(layout *wgpu.BindGroupLayout) {
	p.clipBindLayout = layout
}

// Destroy releases all GPU resources held by the pipeline. Safe to call
// multiple times or on a pipeline with no allocated resources.
func (p *MSDFTextPipeline) Destroy() {
	p.destroyPipeline()
	if p.sampler != nil {
		p.sampler.Release()
		p.sampler = nil
	}
}

// createPipeline compiles the MSDF text shader and creates the render
// pipeline with premultiplied alpha blending and MSAA.
func (p *MSDFTextPipeline) createPipeline() error {
	if msdfTextShaderSource == "" {
		return fmt.Errorf("msdf_text shader source is empty")
	}

	shader, err := p.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "msdf_text_shader",
		WGSL:  msdfTextShaderSource,
	})
	if err != nil {
		return fmt.Errorf("compile msdf_text shader: %w", err)
	}
	p.shader = shader

	// Bind group layout:
	//   Binding 0: TextUniforms (uniform buffer, vertex+fragment)
	//   Binding 1: MSDF atlas texture (texture_2d, fragment)
	//   Binding 2: Sampler (fragment)
	uniformLayout, err := p.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "msdf_text_uniform_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: textUniformSize},
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

	textBGLayouts := []*wgpu.BindGroupLayout{p.uniformLayout}
	hasClip := p.clipBindLayout != nil
	if hasClip {
		textBGLayouts = append(textBGLayouts, p.clipBindLayout)
	}
	pipeLayout, err := p.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "msdf_text_pipe_layout",
		BindGroupLayouts: textBGLayouts,
	})
	if err != nil {
		return fmt.Errorf("create msdf_text pipeline layout: %w", err)
	}
	p.pipeLayout = pipeLayout
	p.pipeLayoutHasClip = hasClip

	// Create sampler for MSDF textures (linear filtering for smooth
	// distance field interpolation). The sampler does not depend on the clip
	// layout, so a pipeline recreated when the clip layout arrives keeps it;
	// creating a new one here leaked the old one (P2-GPU-039).
	if p.sampler == nil {
		sampler, err := p.device.CreateSampler(&wgpu.SamplerDescriptor{
			Label:        "msdf_text_sampler",
			AddressModeU: gputypes.AddressModeClampToEdge,
			AddressModeV: gputypes.AddressModeClampToEdge,
			AddressModeW: gputypes.AddressModeClampToEdge,
			MagFilter:    gputypes.FilterModeLinear,
			MinFilter:    gputypes.FilterModeLinear,
			MipmapFilter: gputypes.FilterModeLinear,
			Anisotropy:   4, // reduces blurring at small text sizes
		})
		if err != nil {
			return fmt.Errorf("create msdf_text sampler: %w", err)
		}
		p.sampler = sampler
	}

	premulBlend := gputypes.BlendStatePremultiplied()
	pipeline, err := p.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "msdf_text_pipeline",
		Layout: p.pipeLayout,
		Vertex: wgpu.VertexState{
			Module:     p.shader,
			EntryPoint: shaderEntryVS,
			Buffers:    textVertexLayout(),
		},
		Fragment: &wgpu.FragmentState{
			Module:     p.shader,
			EntryPoint: shaderEntryFS,
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					Blend:     &premulBlend,
					WriteMask: gputypes.ColorWriteMaskAll,
				},
			},
		},
		Primitive:   triangleListPrimitive(),
		Multisample: multisampleState(p.sampleCount),
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
func (p *MSDFTextPipeline) ensurePipelineWithStencil() error { // Ensure base resources exist (shader, layouts, sampler).
	if p.shader == nil || p.uniformLayout == nil || p.pipeLayout == nil {
		if err := p.createPipeline(); err != nil {
			return err
		}
	}
	// If the pipeline layout was created without clip but clip is now set,
	// destroy and recreate so the layout includes @group(1). Without this,
	// SetBindGroup(1, clipBG) crashes on AMD/NVIDIA (Intel tolerates it).
	if p.clipBindLayout != nil && !p.pipeLayoutHasClip {
		p.destroyPipeline()
		if err := p.createPipeline(); err != nil {
			return err
		}
	}
	if p.pipelineWithStencil != nil {
		return nil
	}

	premulBlend := gputypes.BlendStatePremultiplied()
	pipeline, err := p.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "msdf_text_pipeline_with_stencil",
		Layout: p.pipeLayout,
		Vertex: wgpu.VertexState{
			Module:     p.shader,
			EntryPoint: shaderEntryVS,
			Buffers:    textVertexLayout(),
		},
		Fragment: &wgpu.FragmentState{
			Module:     p.shader,
			EntryPoint: shaderEntryFS,
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					Blend:     &premulBlend,
					WriteMask: gputypes.ColorWriteMaskAll,
				},
			},
		},
		DepthStencil: stencilPassthroughDepthStencil(),
		Primitive:    triangleListPrimitive(),
		Multisample:  multisampleState(p.sampleCount),
	})
	if err != nil {
		return fmt.Errorf("create MSDF text pipeline with stencil: %w", err)
	}
	p.pipelineWithStencil = pipeline
	return nil
}

// ensureDepthClipPipeline creates the depth-clipped pipeline variant if needed.
func (p *MSDFTextPipeline) ensureDepthClipPipeline() error {
	if p.pipelineWithDepthClip != nil {
		return nil
	}
	if err := p.ensurePipelineWithStencil(); err != nil {
		return err
	}

	premulBlend := gputypes.BlendStatePremultiplied()
	pipeline, err := p.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "msdf_text_pipeline_depth_clip",
		Layout: p.pipeLayout,
		Vertex: wgpu.VertexState{
			Module:     p.shader,
			EntryPoint: shaderEntryVS,
			Buffers:    textVertexLayout(),
		},
		Fragment: &wgpu.FragmentState{
			Module:     p.shader,
			EntryPoint: shaderEntryFS,
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					Blend:     &premulBlend,
					WriteMask: gputypes.ColorWriteMaskAll,
				},
			},
		},
		DepthStencil: depthClipDepthStencil(),
		Primitive:    triangleListPrimitive(),
		Multisample:  multisampleState(p.sampleCount),
	})
	if err != nil {
		return fmt.Errorf("create MSDF text pipeline with depth clip: %w", err)
	}
	p.pipelineWithDepthClip = pipeline
	return nil
}

// RecordDraws records MSDF text draw commands into an existing render pass.
// When depthClipped is true (GPU-CLIP-003a), the depth-clipped pipeline
// variant is used to test fragments against the depth clip buffer.
func (p *MSDFTextPipeline) RecordDraws(rp *wgpu.RenderPassEncoder, resources *textFrameResources, clipBG *wgpu.BindGroup, depthClipped ...bool) {
	if resources == nil || len(resources.drawCalls) == 0 {
		return
	}
	useDepthClip := len(depthClipped) > 0 && depthClipped[0] && p.pipelineWithDepthClip != nil
	if useDepthClip {
		rp.SetPipeline(p.pipelineWithDepthClip)
	} else {
		rp.SetPipeline(p.pipelineWithStencil)
	}
	if clipBG != nil {
		rp.SetBindGroup(1, clipBG, nil)
	}
	rp.SetVertexBuffer(0, resources.vertBuf, 0)
	rp.SetIndexBuffer(resources.idxBuf, gputypes.IndexFormatUint16, 0)
	for _, dc := range resources.drawCalls {
		if dc.indexCount == 0 {
			continue
		}
		rp.SetBindGroup(0, dc.bindGroup, nil)
		rp.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: dc.indexCount, InstanceCount: 1, FirstIndex: dc.indexOffset, BaseVertex: 0, FirstInstance: 0})
	}
}

// destroyPipeline releases all pipeline resources in reverse creation order.
func (p *MSDFTextPipeline) destroyPipeline() {
	if p.device == nil {
		return
	}
	if p.pipelineWithDepthClip != nil {
		p.pipelineWithDepthClip.Release()
		p.pipelineWithDepthClip = nil
	}
	if p.pipelineWithStencil != nil {
		p.pipelineWithStencil.Release()
		p.pipelineWithStencil = nil
	}
	if p.pipeline != nil {
		p.pipeline.Release()
		p.pipeline = nil
	}
	if p.pipeLayout != nil {
		p.pipeLayout.Release()
		p.pipeLayout = nil
		p.pipeLayoutHasClip = false
	}
	if p.uniformLayout != nil {
		p.uniformLayout.Release()
		p.uniformLayout = nil
	}
	if p.shader != nil {
		p.shader.Release()
		p.shader = nil
	}
}

// textFrameResources holds per-frame GPU resources for MSDF text rendering.
// textDrawCall represents a single text draw with its own color/transform.
type textDrawCall struct {
	indexOffset uint32 // first index in the shared index buffer
	indexCount  uint32 // number of indices for this batch
	bindGroup   *wgpu.BindGroup
}

type textFrameResources struct {
	vertBuf   *wgpu.Buffer
	idxBuf    *wgpu.Buffer
	drawCalls []textDrawCall
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

// CanMerge reports whether other can be merged into this batch.
// Batches are mergeable when they share the same visual properties
// (transform, color, atlas, MSDF parameters) — only the quad list differs.
// This enables draw call coalescing: multiple DrawString calls with the
// same style produce a single GPU draw call (ADR-031).
func (b *TextBatch) CanMerge(other TextBatch) bool {
	return b.Transform == other.Transform &&
		b.Color == other.Color &&
		b.AtlasIndex == other.AtlasIndex &&
		b.PxRange == other.PxRange &&
		b.AtlasSize == other.AtlasSize
}

// ---- Vertex/index/uniform data builders ----

// quadsToVertices converts TextQuads to vertex array.
// Each quad becomes 4 vertices (for indexed rendering).
func quadsToVertices(quads []TextQuad) []TextVertex {
	vertices := make([]TextVertex, len(quads)*4)

	for i, q := range quads {
		base := i * 4

		// Vertex 0: top-left
		vertices[base+0] = TextVertex{X: q.X0, Y: q.Y0, U: q.U0, V: q.V0}
		// Vertex 1: top-right
		vertices[base+1] = TextVertex{X: q.X1, Y: q.Y0, U: q.U1, V: q.V0}
		// Vertex 2: bottom-right
		vertices[base+2] = TextVertex{X: q.X1, Y: q.Y1, U: q.U1, V: q.V1}
		// Vertex 3: bottom-left
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
		// Vertex 0: top-left
		writeTextVertex(data[off:], q.X0, q.Y0, q.U0, q.V0)
		off += textVertexStride
		// Vertex 1: top-right
		writeTextVertex(data[off:], q.X1, q.Y0, q.U1, q.V0)
		off += textVertexStride
		// Vertex 2: bottom-right
		writeTextVertex(data[off:], q.X1, q.Y1, q.U1, q.V1)
		off += textVertexStride
		// Vertex 3: bottom-left
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

	// Transform: WGSL mat4x4<f32> is stored COLUMN-MAJOR in memory.
	// Each group of 4 floats is one column vector.
	//
	// Affine 2D transform: x' = Ax + By + C,  y' = Dx + Ey + F
	// As a 4x4 matrix (math notation, row-major):
	//   | A  B  0  C |
	//   | D  E  0  F |
	//   | 0  0  1  0 |
	//   | 0  0  0  1 |
	//
	// Column-major storage for WGSL:
	//   col0=[A,D,0,0]  col1=[B,E,0,0]  col2=[0,0,1,0]  col3=[C,F,0,1]
	t := [16]float32{
		float32(transform.A), float32(transform.D), 0, 0, // column 0
		float32(transform.B), float32(transform.E), 0, 0, // column 1
		0, 0, 1, 0, // column 2
		float32(transform.C), float32(transform.F), 0, 1, // column 3
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
