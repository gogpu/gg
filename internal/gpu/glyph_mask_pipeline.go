//go:build !nogpu

package gpu

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// Embedded glyph mask shader source.
//
//go:embed shaders/glyph_mask.wgsl
var glyphMaskShaderSource string

// glyphMaskVertexStride is the byte stride per vertex in the glyph mask pipeline.
// Layout per vertex (matches MSDF text pipeline for Intel Vulkan driver compat):
//
//	position  (vec2<f32>) =  8 bytes  (location 0)
//	tex_coord (vec2<f32>) =  8 bytes  (location 1)
//
// Total = 16 bytes per vertex.
// Color is passed via per-batch uniform buffer (not per-vertex).
const glyphMaskVertexStride = 16

// glyphMaskUniformSize is the byte size of the glyph mask uniform buffer.
// Layout:
//
//	transform (mat4x4<f32>) = 64 bytes
//	color     (vec4<f32>)   = 16 bytes
//
// Total = 80 bytes.
const glyphMaskUniformSize = 80

// GlyphMaskPipeline manages GPU resources for alpha mask text rendering
// (Tier 6). Each text run is rendered as a set of textured quads using
// indexed drawing. The fragment shader samples a single-channel (R8) alpha
// atlas and multiplies by the text color for premultiplied output.
//
// The pipeline uses the same MSAA+depth/stencil texture pattern as
// MSDFTextPipeline for unified render pass integration. A pipelineWithStencil
// variant is provided when the render pass includes a depth/stencil
// attachment (stencil test is Always/Keep -- text does not interact with stencil).
//
// Architecture:
//
//	GPURenderSession owns persistent buffers (vertex, index, uniform)
//	GlyphMaskPipeline owns shader, layout, pipeline, sampler
//	bind groups are created per atlas texture (uniform + texture + sampler)
type GlyphMaskPipeline struct {
	device *wgpu.Device
	queue  *wgpu.Queue

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

	// Default sampler for R8 atlas textures (linear filtering for smooth
	// alpha interpolation at subpixel positions).
	sampler *wgpu.Sampler

	// clipBindLayout is the shared @group(1) bind group layout for RRect clip.
	// Set by the session before ensurePipelineWithStencil.
	clipBindLayout *wgpu.BindGroupLayout
	// pipeLayoutHasClip tracks whether the current pipeLayout was created
	// with clipBindLayout included. If clipBindLayout is set after the
	// layout was created, the pipeline must be recreated.
	pipeLayoutHasClip bool
}

// NewGlyphMaskPipeline creates a new glyph mask pipeline with the given device
// and queue. The render pipeline and GPU objects are not created until
// ensurePipelineWithStencil is called.
func NewGlyphMaskPipeline(device *wgpu.Device, queue *wgpu.Queue) *GlyphMaskPipeline {
	return &GlyphMaskPipeline{
		device: device,
		queue:  queue,
	}
}

// SetClipBindLayout sets the bind group layout for the @group(1) RRect clip
// uniform. Must be called before ensurePipelineWithStencil. The layout is
// owned by the session and must not be destroyed by the pipeline.
func (p *GlyphMaskPipeline) SetClipBindLayout(layout *wgpu.BindGroupLayout) {
	p.clipBindLayout = layout
}

// Destroy releases all GPU resources held by the pipeline. Safe to call
// multiple times or on a pipeline with no allocated resources.
func (p *GlyphMaskPipeline) Destroy() {
	p.destroyPipeline()
	if p.sampler != nil {
		p.sampler.Release()
		p.sampler = nil
	}
}

// ensureSharedResources compiles the shader and creates the bind group layout,
// pipeline layout, and sampler. These are shared between the base and stencil
// pipeline variants. Separated from pipeline creation to allow the stencil
// variant to be created even if the base (non-stencil) pipeline fails on
// some Intel drivers.
func (p *GlyphMaskPipeline) ensureSharedResources() error {
	if p.shader != nil && p.uniformLayout != nil && p.pipeLayout != nil && p.sampler != nil {
		return nil
	}

	if glyphMaskShaderSource == "" {
		return fmt.Errorf("glyph_mask shader source is empty")
	}

	shader, err := p.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "glyph_mask_shader",
		WGSL:  glyphMaskShaderSource,
	})
	if err != nil {
		return fmt.Errorf("compile glyph_mask shader: %w", err)
	}
	p.shader = shader

	// Bind group layout:
	//   Binding 0: GlyphMaskUniforms (uniform buffer, vertex+fragment)
	//   Binding 1: R8 atlas texture (texture_2d, fragment)
	//   Binding 2: Sampler (fragment)
	uniformLayout, err := p.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "glyph_mask_uniform_layout",
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
		return fmt.Errorf("create glyph_mask uniform layout: %w", err)
	}
	p.uniformLayout = uniformLayout

	glyphBGLayouts := []*wgpu.BindGroupLayout{p.uniformLayout}
	hasClip := p.clipBindLayout != nil
	if hasClip {
		glyphBGLayouts = append(glyphBGLayouts, p.clipBindLayout)
	}
	pipeLayout, err := p.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "glyph_mask_pipe_layout",
		BindGroupLayouts: glyphBGLayouts,
	})
	if err != nil {
		return fmt.Errorf("create glyph_mask pipeline layout: %w", err)
	}
	p.pipeLayout = pipeLayout
	p.pipeLayoutHasClip = hasClip

	// Create sampler for R8 atlas textures (linear filtering for smooth
	// alpha interpolation at fractional positions).
	sampler, err := p.device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:        "glyph_mask_sampler",
		AddressModeU: gputypes.AddressModeClampToEdge,
		AddressModeV: gputypes.AddressModeClampToEdge,
		AddressModeW: gputypes.AddressModeClampToEdge,
		MagFilter:    gputypes.FilterModeLinear,
		MinFilter:    gputypes.FilterModeLinear,
		MipmapFilter: gputypes.FilterModeLinear,
	})
	if err != nil {
		return fmt.Errorf("create glyph_mask sampler: %w", err)
	}
	p.sampler = sampler

	return nil
}

// ensurePipelineWithStencil creates the session-compatible pipeline variant
// that includes a depth/stencil state. This pipeline is used when text is
// rendered in a unified render pass alongside stencil-then-cover paths.
// The stencil test is Always/Keep (text does not interact with stencil).
//
// The base pipeline (shader, layout, sampler) is created first if it
// doesn't exist.
func (p *GlyphMaskPipeline) ensurePipelineWithStencil() error {
	// Ensure shared resources exist (shader, layouts, sampler).
	if err := p.ensureSharedResources(); err != nil {
		return err
	}
	// If the pipeline layout was created without clip but clip is now set,
	// destroy and recreate so the layout includes @group(1). Without this,
	// SetBindGroup(1, clipBG) crashes on AMD/NVIDIA (Intel tolerates it).
	if p.clipBindLayout != nil && !p.pipeLayoutHasClip {
		p.destroyPipeline()
		if err := p.ensureSharedResources(); err != nil {
			return err
		}
	}
	if p.pipelineWithStencil != nil {
		return nil
	}

	premulBlend := gputypes.BlendStatePremultiplied()
	pipeline, err := p.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "glyph_mask_pipeline_with_stencil",
		Layout: p.pipeLayout,
		Vertex: wgpu.VertexState{
			Module:     p.shader,
			EntryPoint: "vs_main",
			Buffers:    glyphMaskVertexLayout(),
		},
		Fragment: &wgpu.FragmentState{
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
		DepthStencil: &wgpu.DepthStencilState{
			Format:            gputypes.TextureFormatDepth24PlusStencil8,
			DepthWriteEnabled: false,
			DepthCompare:      gputypes.CompareFunctionAlways,
			StencilFront: wgpu.StencilFaceState{
				Compare:     gputypes.CompareFunctionAlways,
				FailOp:      wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep,
				PassOp:      wgpu.StencilOperationKeep,
			},
			StencilBack: wgpu.StencilFaceState{
				Compare:     gputypes.CompareFunctionAlways,
				FailOp:      wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep,
				PassOp:      wgpu.StencilOperationKeep,
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
		return fmt.Errorf("create glyph mask pipeline with stencil: %w", err)
	}
	p.pipelineWithStencil = pipeline
	return nil
}

// RecordDraws records glyph mask draw commands into an existing render pass.
// The render pass is owned by GPURenderSession. This method uses the
// pipelineWithStencil variant because the session's render pass includes
// a depth/stencil attachment.
//
// The resources parameter holds pre-built vertex/index buffers, uniform buffer,
// and bind group for the current frame.
func (p *GlyphMaskPipeline) RecordDraws(rp *wgpu.RenderPassEncoder, resources *glyphMaskFrameResources, clipBG *wgpu.BindGroup) {
	if resources == nil || len(resources.drawCalls) == 0 {
		return
	}
	rp.SetPipeline(p.pipelineWithStencil)
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
		rp.DrawIndexed(dc.indexCount, 1, dc.indexOffset, 0, 0)
	}
}

// destroyPipeline releases all pipeline resources in reverse creation order.
func (p *GlyphMaskPipeline) destroyPipeline() {
	if p.device == nil {
		return
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

// ---- Per-frame GPU resources ----

// glyphMaskDrawCall represents a single draw call within a glyph mask batch.
type glyphMaskDrawCall struct {
	indexOffset uint32 // first index in the shared index buffer
	indexCount  uint32 // number of indices for this draw
	bindGroup   *wgpu.BindGroup
}

// glyphMaskFrameResources holds per-frame GPU resources for glyph mask rendering.
type glyphMaskFrameResources struct {
	vertBuf   *wgpu.Buffer
	idxBuf    *wgpu.Buffer
	drawCalls []glyphMaskDrawCall
}

// ---- Vertex layout ----

// glyphMaskVertexLayout returns the vertex buffer layout for the glyph mask pipeline.
// Matches VertexInput in glyph_mask.wgsl:
//
//	location 0: position  (vec2<f32>)
//	location 1: tex_coord (vec2<f32>)
//
// Color and is_lcd are in the uniform buffer (per-batch, not per-vertex).
// This matches the MSDF text pipeline layout and avoids Intel Vulkan driver
// issues with >2 vertex attributes.
func glyphMaskVertexLayout() []gputypes.VertexBufferLayout {
	return []gputypes.VertexBufferLayout{
		{
			ArrayStride: glyphMaskVertexStride,
			StepMode:    gputypes.VertexStepModeVertex,
			Attributes: []gputypes.VertexAttribute{
				{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0}, // position
				{Format: gputypes.VertexFormatFloat32x2, Offset: 8, ShaderLocation: 1}, // tex_coord
			},
		},
	}
}

// ---- Data types for GlyphMaskPipeline ----

// GlyphMaskQuad represents a single glyph quad for alpha mask rendering.
// Each glyph is rendered as a textured quad with position and UV.
type GlyphMaskQuad struct {
	// Position of quad corners in screen/local space.
	X0, Y0, X1, Y1 float32

	// UV coordinates in R8 atlas [0, 1].
	// For LCD glyphs, UVs span the 3x-wide region in the atlas.
	U0, V0, U1, V1 float32
}

// GlyphMaskBatch represents a batch of glyph mask quads with shared
// rendering parameters. Multiple batches may use different atlas pages,
// transforms, or colors.
type GlyphMaskBatch struct {
	// Quads is the list of glyph quads to render.
	Quads []GlyphMaskQuad

	// Transform is the 2D affine transform for this batch.
	Transform gg.Matrix

	// Color is the text color (RGBA, premultiplied alpha) for this batch.
	// All glyphs in a batch share the same color (set per DrawString call).
	Color [4]float32

	// IsLCD indicates this batch uses LCD subpixel rendering.
	// Currently unused at the shader level (grayscale-only path) due to
	// Intel Vulkan driver compatibility. Retained for future LCD restore.
	IsLCD bool

	// AtlasPageIndex identifies which atlas page (R8 texture) to use.
	AtlasPageIndex int
}

// ---- Vertex/index/uniform data builders ----

// buildGlyphMaskVertexData serializes GlyphMaskQuad slices into raw vertex
// bytes suitable for GPU upload. Each quad produces 4 vertices x 16 bytes = 64 bytes.
func buildGlyphMaskVertexData(quads []GlyphMaskQuad) []byte {
	if len(quads) == 0 {
		return nil
	}
	data := make([]byte, len(quads)*4*glyphMaskVertexStride)
	off := 0
	for _, q := range quads {
		// Vertex 0: top-left
		writeGlyphMaskVertex(data[off:], q.X0, q.Y0, q.U0, q.V0)
		off += glyphMaskVertexStride
		// Vertex 1: top-right
		writeGlyphMaskVertex(data[off:], q.X1, q.Y0, q.U1, q.V0)
		off += glyphMaskVertexStride
		// Vertex 2: bottom-right
		writeGlyphMaskVertex(data[off:], q.X1, q.Y1, q.U1, q.V1)
		off += glyphMaskVertexStride
		// Vertex 3: bottom-left
		writeGlyphMaskVertex(data[off:], q.X0, q.Y1, q.U0, q.V1)
		off += glyphMaskVertexStride
	}
	return data
}

// writeGlyphMaskVertex writes a single glyph mask vertex into buf.
// Only position and texcoord are per-vertex; color/isLCD are per-batch uniform.
func writeGlyphMaskVertex(buf []byte, x, y, u, v float32) {
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(x))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(y))
	binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(u))
	binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(v))
}

// buildGlyphMaskIndexData serializes quad indices into raw bytes for GPU upload.
// Uses the same index pattern as MSDF text: 0,1,2, 2,3,0 per quad.
func buildGlyphMaskIndexData(numQuads int) []byte {
	indices := generateQuadIndices(numQuads) // reuse from text_pipeline.go
	data := make([]byte, len(indices)*2)
	for i, idx := range indices {
		binary.LittleEndian.PutUint16(data[i*2:], idx)
	}
	return data
}

// makeGlyphMaskUniform creates the 80-byte uniform buffer for a glyph mask batch.
// The uniform contains the transform matrix and text color.
func makeGlyphMaskUniform(transform gg.Matrix, color [4]float32) []byte {
	buf := make([]byte, glyphMaskUniformSize)
	off := 0

	// Transform: WGSL mat4x4<f32> is stored COLUMN-MAJOR in memory.
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

	// Color (vec4<f32>): premultiplied RGBA.
	for i := range 4 {
		binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(color[i]))
		off += 4
	}

	return buf
}
