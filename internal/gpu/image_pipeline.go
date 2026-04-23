//go:build !nogpu

package gpu

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

//go:embed shaders/textured_quad.wgsl
var texturedQuadShaderSource string

// imageVertexStride is the byte stride per vertex in the textured quad pipeline.
// Layout per vertex:
//
//	position  (vec2<f32>) =  8 bytes  (location 0)
//	tex_coord (vec2<f32>) =  8 bytes  (location 1)
//
// Total = 16 bytes per vertex.
const imageVertexStride = 16

// imageUniformSize is the byte size of the image uniform buffer.
// Layout:
//
//	transform (mat4x4<f32>) = 64 bytes
//	opacity   (f32)         =  4 bytes
//	_pad      (vec3<f32>)   = 12 bytes
//
// Total = 80 bytes.
const imageUniformSize = 80

// ImageDrawCommand holds everything needed to render one image as a textured
// quad on the GPU. Populated by context_image.go and consumed by the render
// session. All coordinates are in device pixels (post-CTM).
type ImageDrawCommand struct {
	// Image pixel data (premultiplied RGBA, row-major).
	PixelData    []byte
	GenerationID uint64 // Pixmap.GenerationID() — GPU cache key (ADR-014)
	ImgWidth     int
	ImgHeight    int
	ImgStride    int

	// Destination quad in device pixels (4 corners: TL, TR, BL, BR).
	DstX, DstY     float32 // top-left position
	DstW, DstH     float32 // width, height
	Opacity        float32
	ViewportWidth  uint32
	ViewportHeight uint32

	// Source UV rectangle (normalized 0..1 within the image).
	// For full-image draws: u0=0, v0=0, u1=1, v1=1.
	U0, V0, U1, V1 float32
}

// TexturedQuadPipeline manages GPU resources for image rendering (Tier 3).
// Each image draw is a textured quad: 6 vertices (2 triangles) with UV mapping.
// The fragment shader samples the image texture with bilinear filtering and
// applies opacity as a uniform multiplier.
//
// Architecture:
//
//	GPURenderSession owns persistent vertex/index buffers (if needed)
//	TexturedQuadPipeline owns shader, layout, pipeline, sampler
//	ImageCache (on GPUShared) owns per-image GPU textures
//	Bind groups are created per-batch (uniform + texture + sampler)
type TexturedQuadPipeline struct {
	device *wgpu.Device
	queue  *wgpu.Queue

	// GPU objects for the render pipeline.
	shader        *wgpu.ShaderModule
	uniformLayout *wgpu.BindGroupLayout
	pipeLayout    *wgpu.PipelineLayout

	// Session-compatible pipeline variant with depth/stencil state.
	// Used when images participate in a unified render pass that includes
	// a stencil attachment. Stencil test is Always/Keep (images do not
	// interact with stencil).
	pipelineWithStencil *wgpu.RenderPipeline

	// Default sampler for image textures (bilinear filtering, clamp-to-edge).
	sampler *wgpu.Sampler

	// clipBindLayout is the shared @group(1) bind group layout for RRect clip.
	// Set by the session before ensurePipelineWithStencil.
	clipBindLayout    *wgpu.BindGroupLayout
	pipeLayoutHasClip bool
}

// NewTexturedQuadPipeline creates a new textured quad pipeline.
func NewTexturedQuadPipeline(device *wgpu.Device, queue *wgpu.Queue) *TexturedQuadPipeline {
	return &TexturedQuadPipeline{
		device: device,
		queue:  queue,
	}
}

// SetClipBindLayout sets the bind group layout for the @group(1) RRect clip
// uniform. Must be called before ensurePipelineWithStencil.
func (p *TexturedQuadPipeline) SetClipBindLayout(layout *wgpu.BindGroupLayout) {
	p.clipBindLayout = layout
}

// Destroy releases all GPU resources held by the pipeline.
func (p *TexturedQuadPipeline) Destroy() {
	p.destroyPipeline()
}

// ensurePipelineWithStencil creates the pipeline variant that includes
// depth/stencil state (for unified render pass with stencil-then-cover).
func (p *TexturedQuadPipeline) ensurePipelineWithStencil() error {
	if err := p.ensureBase(); err != nil {
		return err
	}
	// If the pipeline layout was created without clip but clip is now set,
	// destroy and recreate.
	if p.clipBindLayout != nil && !p.pipeLayoutHasClip {
		p.destroyPipeline()
		if err := p.ensureBase(); err != nil {
			return err
		}
	}
	if p.pipelineWithStencil != nil {
		return nil
	}

	premulBlend := gputypes.BlendStatePremultiplied()
	pipeline, err := p.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "textured_quad_pipeline_with_stencil",
		Layout: p.pipeLayout,
		Vertex: wgpu.VertexState{
			Module:     p.shader,
			EntryPoint: "vs_main",
			Buffers:    imageVertexLayout(),
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
		DepthStencil: stencilPassthroughDepthStencil(),
		Primitive:    triangleListPrimitive(),
		Multisample:  defaultMultisample(),
	})
	if err != nil {
		return fmt.Errorf("create textured quad pipeline with stencil: %w", err)
	}
	p.pipelineWithStencil = pipeline
	return nil
}

// ensureBase creates the shader, sampler, bind group layout, and pipeline layout
// if they don't exist yet.
func (p *TexturedQuadPipeline) ensureBase() error {
	if p.shader != nil && p.uniformLayout != nil && p.pipeLayout != nil && p.sampler != nil {
		return nil
	}

	if texturedQuadShaderSource == "" {
		return fmt.Errorf("textured_quad shader source is empty")
	}

	// Shader module.
	shader, err := p.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "textured_quad_shader",
		WGSL:  texturedQuadShaderSource,
	})
	if err != nil {
		return fmt.Errorf("compile textured_quad shader: %w", err)
	}
	p.shader = shader

	// Sampler (bilinear, clamp-to-edge — matching Skia default for image rects).
	sampler, err := p.device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:        "image_sampler",
		AddressModeU: gputypes.AddressModeClampToEdge,
		AddressModeV: gputypes.AddressModeClampToEdge,
		AddressModeW: gputypes.AddressModeClampToEdge,
		MagFilter:    gputypes.FilterModeLinear,
		MinFilter:    gputypes.FilterModeLinear,
		MipmapFilter: gputypes.FilterModeNearest,
	})
	if err != nil {
		return fmt.Errorf("create image sampler: %w", err)
	}
	p.sampler = sampler

	// Bind group layout: uniform + texture + sampler.
	uniformLayout, err := p.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "textured_quad_bind_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: imageUniformSize},
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
		return fmt.Errorf("create textured_quad bind layout: %w", err)
	}
	p.uniformLayout = uniformLayout

	// Pipeline layout.
	bgLayouts := []*wgpu.BindGroupLayout{p.uniformLayout}
	hasClip := p.clipBindLayout != nil
	if hasClip {
		bgLayouts = append(bgLayouts, p.clipBindLayout)
	}
	pipeLayout, err := p.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "textured_quad_pipe_layout",
		BindGroupLayouts: bgLayouts,
	})
	if err != nil {
		return fmt.Errorf("create textured_quad pipe layout: %w", err)
	}
	p.pipeLayoutHasClip = hasClip
	p.pipeLayout = pipeLayout

	return nil
}

// RecordDraws records image draw commands into an existing render pass.
// Each draw call renders one textured quad with its own bind group (texture + uniform).
func (p *TexturedQuadPipeline) RecordDraws(rp *wgpu.RenderPassEncoder, res *imageFrameResources, clipBG *wgpu.BindGroup) {
	rp.SetPipeline(p.pipelineWithStencil)
	if clipBG != nil {
		rp.SetBindGroup(1, clipBG, nil)
	}
	rp.SetVertexBuffer(0, res.vertBuf, 0)
	for _, dc := range res.drawCalls {
		rp.SetBindGroup(0, dc.bindGroup, nil)
		rp.Draw(6, 1, dc.firstVertex, 0) //nolint:mnd // 6 vertices per quad (2 triangles)
	}
}

// destroyPipeline releases all pipeline resources.
func (p *TexturedQuadPipeline) destroyPipeline() {
	if p.device == nil {
		return
	}
	if p.pipelineWithStencil != nil {
		p.pipelineWithStencil.Release()
		p.pipelineWithStencil = nil
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
	if p.sampler != nil {
		p.sampler.Release()
		p.sampler = nil
	}
	if p.shader != nil {
		p.shader.Release()
		p.shader = nil
	}
}

// imageFrameResources holds pre-built GPU resources for image rendering in
// a single frame. Created by the render session's buildImageResources.
type imageFrameResources struct {
	vertBuf   *wgpu.Buffer
	drawCalls []imageDrawCall
}

// imageDrawCall holds per-image draw parameters within a frame.
type imageDrawCall struct {
	bindGroup   *wgpu.BindGroup
	firstVertex uint32
}

// imageVertexLayout returns the vertex buffer layout for the textured quad pipeline.
func imageVertexLayout() []gputypes.VertexBufferLayout {
	return []gputypes.VertexBufferLayout{
		{
			ArrayStride: imageVertexStride,
			StepMode:    gputypes.VertexStepModeVertex,
			Attributes: []gputypes.VertexAttribute{
				{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0}, // position
				{Format: gputypes.VertexFormatFloat32x2, Offset: 8, ShaderLocation: 1}, // tex_coord
			},
		},
	}
}

// buildImageVertices generates vertex data for a single image quad.
// Returns 6 vertices (2 triangles: TL, TR, BL, TR, BR, BL).
func buildImageVertices(cmd *ImageDrawCommand) []byte {
	const vertsPerQuad = 6
	buf := make([]byte, vertsPerQuad*imageVertexStride)

	// Quad corners in pixel coordinates.
	x0 := cmd.DstX
	y0 := cmd.DstY
	x1 := cmd.DstX + cmd.DstW
	y1 := cmd.DstY + cmd.DstH

	// UV coordinates.
	u0, v0, u1, v1 := cmd.U0, cmd.V0, cmd.U1, cmd.V1

	// Triangle 1: TL, TR, BL
	// Triangle 2: TR, BR, BL
	type vertex struct {
		px, py float32
		u, v   float32
	}
	verts := [6]vertex{
		{x0, y0, u0, v0}, // TL
		{x1, y0, u1, v0}, // TR
		{x0, y1, u0, v1}, // BL
		{x1, y0, u1, v0}, // TR
		{x1, y1, u1, v1}, // BR
		{x0, y1, u0, v1}, // BL
	}

	offset := 0
	for _, v := range verts {
		binary.LittleEndian.PutUint32(buf[offset:], math.Float32bits(v.px))
		binary.LittleEndian.PutUint32(buf[offset+4:], math.Float32bits(v.py))
		binary.LittleEndian.PutUint32(buf[offset+8:], math.Float32bits(v.u))
		binary.LittleEndian.PutUint32(buf[offset+12:], math.Float32bits(v.v))
		offset += imageVertexStride
	}

	return buf
}

// makeImageUniform creates the uniform buffer data for an image draw.
// Contains an orthographic projection matrix and opacity.
func makeImageUniform(viewportW, viewportH uint32, opacity float32) []byte {
	buf := make([]byte, imageUniformSize)

	// Orthographic projection: pixel coords → NDC.
	// Same as glyph_mask/msdf_text: column-major mat4x4.
	w := float32(viewportW)
	h := float32(viewportH)

	// Column 0: [2/w, 0, 0, 0]
	binary.LittleEndian.PutUint32(buf[0:], math.Float32bits(2.0/w))
	// buf[4..12] = 0 (zero-initialized)

	// Column 1: [0, -2/h, 0, 0]
	// buf[16] = 0
	binary.LittleEndian.PutUint32(buf[20:], math.Float32bits(-2.0/h))
	// buf[24..28] = 0

	// Column 2: [0, 0, 1, 0] (identity z)
	binary.LittleEndian.PutUint32(buf[40:], math.Float32bits(1.0))
	// buf[32..36, 44] = 0

	// Column 3: [-1, 1, 0, 1]
	binary.LittleEndian.PutUint32(buf[48:], math.Float32bits(-1.0))
	binary.LittleEndian.PutUint32(buf[52:], math.Float32bits(1.0))
	// buf[56] = 0
	binary.LittleEndian.PutUint32(buf[60:], math.Float32bits(1.0))

	// Opacity (offset 64).
	binary.LittleEndian.PutUint32(buf[64:], math.Float32bits(opacity))
	// Padding bytes 68..79 remain zero.

	return buf
}
