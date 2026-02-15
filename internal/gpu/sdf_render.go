//go:build !nogpu

package gpu

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

//go:embed shaders/sdf_render.wgsl
var sdfRenderShaderSource string

// sdfRenderVertexStride is the byte stride per vertex in the SDF render pipeline.
// Layout per vertex:
//
//	position (vec2<f32>) = 8 bytes  (location 0)
//	local    (vec2<f32>) = 8 bytes  (location 1)
//	shape_kind (f32)     = 4 bytes  (location 2)
//	param1   (f32)       = 4 bytes  (location 3)
//	param2   (f32)       = 4 bytes  (location 4)
//	param3   (f32)       = 4 bytes  (location 5)
//	half_stroke (f32)    = 4 bytes  (location 6)
//	is_stroked (f32)     = 4 bytes  (location 7)
//	color    (vec4<f32>) = 16 bytes (location 8)
//
// Total = 56 bytes per vertex.
const sdfRenderVertexStride = 56

// sdfRenderUniformSize is the byte size of the SDF render uniform buffer.
// Layout: viewport (vec2<f32>) + padding (vec2<f32>) = 16 bytes.
const sdfRenderUniformSize = 16

// sdfRenderAAMargin is the anti-aliasing padding in pixels added to each side
// of the bounding quad. This ensures the smoothstep transition zone at shape
// edges is fully covered.
const sdfRenderAAMargin = 1.5

// SDFRenderPipeline manages GPU resources for SDF-based shape rendering
// via a vertex+fragment render pipeline. Instead of a compute shader that
// writes to a storage buffer, this approach draws bounding quads with a
// fragment shader that evaluates the SDF per pixel.
//
// Advantages over the compute shader approach:
//   - No readback needed when rendering to a surface (future optimization)
//   - MSAA hardware resolve for free anti-aliasing
//   - Simpler pipeline (no storage buffer barriers)
//   - Works around naga compute shader bugs
//
// The pipeline uses the same MSAA+resolve texture pattern as StencilRenderer.
// For unified rendering via GPURenderSession, pipelineWithStencil is used
// when the render pass includes a depth/stencil attachment.
type SDFRenderPipeline struct {
	device hal.Device
	queue  hal.Queue

	// GPU objects for the render pipeline.
	shader        hal.ShaderModule
	uniformLayout hal.BindGroupLayout
	pipeLayout    hal.PipelineLayout
	pipeline      hal.RenderPipeline

	// Session-compatible pipeline variant with depth/stencil state.
	// This is used when the SDF pipeline participates in a unified render
	// pass that includes a stencil attachment (for stencil-then-cover paths).
	// The stencil test is Always/Keep (SDF shapes don't interact with stencil).
	pipelineWithStencil hal.RenderPipeline

	// MSAA and resolve textures for offscreen rendering (standalone mode).
	// When used via GPURenderSession, these are nil -- the session owns textures.
	msaaTex     hal.Texture
	msaaView    hal.TextureView
	resolveTex  hal.Texture
	resolveView hal.TextureView

	width, height uint32
}

// NewSDFRenderPipeline creates a new SDF render pipeline with the given device
// and queue. The render pipeline and textures are not created until
// ensureReady is called with the desired dimensions.
func NewSDFRenderPipeline(device hal.Device, queue hal.Queue) *SDFRenderPipeline {
	return &SDFRenderPipeline{
		device: device,
		queue:  queue,
	}
}

// Destroy releases all GPU resources held by the pipeline. Safe to call
// multiple times or on a pipeline with no allocated resources.
func (p *SDFRenderPipeline) Destroy() {
	p.destroyPipeline()
	p.destroyTextures()
}

// RenderShapes renders detected SDF shapes via the fragment shader pipeline.
// Each shape is expanded into a bounding quad; the fragment shader evaluates
// the SDF per pixel for smooth anti-aliased coverage.
//
// The shapes are rendered in a single render pass. The MSAA color attachment
// resolves to a single-sample texture, which is then copied to a staging
// buffer for CPU readback into target.Data.
//
// Returns nil for empty shape slices. Returns ErrFallbackToCPU if the shader
// source is empty (build-time issue).
func (p *SDFRenderPipeline) RenderShapes(target gg.GPURenderTarget, shapes []SDFRenderShape) error {
	if len(shapes) == 0 {
		return nil
	}

	w, h := uint32(target.Width), uint32(target.Height) //nolint:gosec // dimensions always fit uint32
	if err := p.ensureReady(w, h); err != nil {
		return err
	}

	// Build vertex data for all shapes (6 vertices per quad = 2 triangles).
	vertexData := buildSDFRenderVertices(shapes, w, h)
	vertexCount := uint32(len(shapes) * 6) //nolint:gosec // shape count fits uint32

	// Create per-frame GPU resources.
	vertBuf, err := p.createAndUploadBuffer("sdf_render_verts", vertexData,
		gputypes.BufferUsageVertex|gputypes.BufferUsageCopyDst)
	if err != nil {
		return fmt.Errorf("create vertex buffer: %w", err)
	}
	defer p.device.DestroyBuffer(vertBuf)

	uniformData := makeSDFRenderUniform(w, h)
	uniformBuf, err := p.createAndUploadBuffer("sdf_render_uniform", uniformData,
		gputypes.BufferUsageUniform|gputypes.BufferUsageCopyDst)
	if err != nil {
		return fmt.Errorf("create uniform buffer: %w", err)
	}
	defer p.device.DestroyBuffer(uniformBuf)

	bindGroup, err := p.device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "sdf_render_bind",
		Layout: p.uniformLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{
				Buffer: uniformBuf.NativeHandle(), Offset: 0, Size: sdfRenderUniformSize,
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("create bind group: %w", err)
	}
	defer p.device.DestroyBindGroup(bindGroup)

	// Encode render pass + readback.
	return p.encodeAndReadback(w, h, vertBuf, vertexCount, bindGroup, target)
}

// ensureReady creates textures and the pipeline if needed.
func (p *SDFRenderPipeline) ensureReady(w, h uint32) error {
	if err := p.ensureTextures(w, h); err != nil {
		return fmt.Errorf("ensure textures: %w", err)
	}
	if p.pipeline == nil {
		if err := p.createPipeline(); err != nil {
			return fmt.Errorf("create pipeline: %w", err)
		}
	}
	return nil
}

// ensureTextures creates or recreates MSAA and resolve textures if the
// requested dimensions differ from the current size.
func (p *SDFRenderPipeline) ensureTextures(w, h uint32) error {
	if p.width == w && p.height == h && p.msaaTex != nil {
		return nil
	}
	p.destroyTextures()

	size := hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1}

	// MSAA color texture (4x samples, BGRA8Unorm).
	msaaTex, err := p.device.CreateTexture(&hal.TextureDescriptor{
		Label:         "sdf_render_msaa",
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   sampleCount,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		return fmt.Errorf("create MSAA texture: %w", err)
	}
	p.msaaTex = msaaTex

	msaaView, err := p.device.CreateTextureView(msaaTex, &hal.TextureViewDescriptor{
		Label:         "sdf_render_msaa_view",
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Dimension:     gputypes.TextureViewDimension2D,
		Aspect:        gputypes.TextureAspectAll,
		MipLevelCount: 1,
	})
	if err != nil {
		p.destroyTextures()
		return fmt.Errorf("create MSAA view: %w", err)
	}
	p.msaaView = msaaView

	// Single-sample resolve target (CopySrc for readback).
	resolveTex, err := p.device.CreateTexture(&hal.TextureDescriptor{
		Label:         "sdf_render_resolve",
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		p.destroyTextures()
		return fmt.Errorf("create resolve texture: %w", err)
	}
	p.resolveTex = resolveTex

	resolveView, err := p.device.CreateTextureView(resolveTex, &hal.TextureViewDescriptor{
		Label:         "sdf_render_resolve_view",
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Dimension:     gputypes.TextureViewDimension2D,
		Aspect:        gputypes.TextureAspectAll,
		MipLevelCount: 1,
	})
	if err != nil {
		p.destroyTextures()
		return fmt.Errorf("create resolve view: %w", err)
	}
	p.resolveView = resolveView

	p.width = w
	p.height = h
	return nil
}

// destroyTextures releases all texture resources and resets dimensions.
func (p *SDFRenderPipeline) destroyTextures() {
	if p.resolveView != nil {
		p.device.DestroyTextureView(p.resolveView)
		p.resolveView = nil
	}
	if p.resolveTex != nil {
		p.device.DestroyTexture(p.resolveTex)
		p.resolveTex = nil
	}
	if p.msaaView != nil {
		p.device.DestroyTextureView(p.msaaView)
		p.msaaView = nil
	}
	if p.msaaTex != nil {
		p.device.DestroyTexture(p.msaaTex)
		p.msaaTex = nil
	}
	p.width = 0
	p.height = 0
}

// createPipeline compiles the SDF render shader and creates the render
// pipeline with premultiplied alpha blending and MSAA.
func (p *SDFRenderPipeline) createPipeline() error { //nolint:dupl // GPU pipeline descriptors share structure but differ in labels, shaders, and vertex layouts
	if sdfRenderShaderSource == "" {
		return fmt.Errorf("sdf_render shader source is empty")
	}

	shader, err := p.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "sdf_render_shader",
		Source: hal.ShaderSource{WGSL: sdfRenderShaderSource},
	})
	if err != nil {
		return fmt.Errorf("compile sdf_render shader: %w", err)
	}
	p.shader = shader

	uniformLayout, err := p.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "sdf_render_uniform_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create uniform layout: %w", err)
	}
	p.uniformLayout = uniformLayout

	pipeLayout, err := p.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "sdf_render_pipe_layout",
		BindGroupLayouts: []hal.BindGroupLayout{p.uniformLayout},
	})
	if err != nil {
		return fmt.Errorf("create pipeline layout: %w", err)
	}
	p.pipeLayout = pipeLayout

	premulBlend := gputypes.BlendStatePremultiplied()
	pipeline, err := p.device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label:  "sdf_render_pipeline",
		Layout: p.pipeLayout,
		Vertex: hal.VertexState{
			Module:     p.shader,
			EntryPoint: "vs_main",
			Buffers:    sdfRenderVertexLayout(),
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
		return fmt.Errorf("create render pipeline: %w", err)
	}
	p.pipeline = pipeline

	return nil
}

// ensurePipelineWithStencil creates the session-compatible pipeline variant
// that includes a depth/stencil state. This pipeline is used when the SDF
// shapes are rendered in a unified render pass alongside stencil-then-cover
// paths. The SDF pipeline ignores the stencil buffer (Compare=Always, all
// ops=Keep, write mask=0).
//
// The base pipeline (shader, layout, bind group layout) is created first
// if it doesn't exist.
func (p *SDFRenderPipeline) ensurePipelineWithStencil() error { //nolint:dupl // GPU pipeline descriptors share structure but differ in labels, shaders, and vertex layouts
	// Ensure base resources exist (shader, layouts).
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
		Label:  "sdf_render_pipeline_with_stencil",
		Layout: p.pipeLayout,
		Vertex: hal.VertexState{
			Module:     p.shader,
			EntryPoint: "vs_main",
			Buffers:    sdfRenderVertexLayout(),
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
		return fmt.Errorf("create SDF pipeline with stencil: %w", err)
	}
	p.pipelineWithStencil = pipeline
	return nil
}

// RecordDraws records SDF shape draws into an existing render pass.
// The render pass is owned by GPURenderSession. This method uses the
// pipelineWithStencil variant because the session's render pass includes
// a depth/stencil attachment.
//
// The resources parameter holds pre-built vertex buffer, uniform buffer,
// and bind group for the current frame.
func (p *SDFRenderPipeline) RecordDraws(rp hal.RenderPassEncoder, resources *sdfFrameResources) {
	rp.SetPipeline(p.pipelineWithStencil)
	rp.SetBindGroup(0, resources.bindGroup, nil)
	rp.SetVertexBuffer(0, resources.vertBuf, 0)
	rp.Draw(resources.vertCount, 1, 0, 0)
}

// destroyPipeline releases all pipeline resources in reverse creation order.
func (p *SDFRenderPipeline) destroyPipeline() {
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

// encodeAndReadback encodes the SDF render pass, copies the resolve texture
// to a staging buffer, submits, waits, and reads back pixels.
func (p *SDFRenderPipeline) encodeAndReadback(
	w, h uint32, vertBuf hal.Buffer, vertexCount uint32,
	bindGroup hal.BindGroup, target gg.GPURenderTarget,
) error {
	encoder, err := p.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "sdf_render_encoder",
	})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	if err := encoder.BeginEncoding("sdf_render"); err != nil {
		return fmt.Errorf("begin encoding: %w", err)
	}

	// Render pass with MSAA resolve.
	rpDesc := &hal.RenderPassDescriptor{
		Label: "sdf_render_pass",
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:          p.msaaView,
				ResolveTarget: p.resolveView,
				LoadOp:        gputypes.LoadOpClear,
				StoreOp:       gputypes.StoreOpStore,
				ClearValue:    gputypes.Color{R: 0, G: 0, B: 0, A: 0},
			},
		},
	}
	rp := encoder.BeginRenderPass(rpDesc)
	rp.SetPipeline(p.pipeline)
	rp.SetBindGroup(0, bindGroup, nil)
	rp.SetVertexBuffer(0, vertBuf, 0)
	rp.Draw(vertexCount, 1, 0, 0)
	rp.End()

	// VK-LAYOUT-001: After MSAA resolve the texture is in
	// COLOR_ATTACHMENT_OPTIMAL layout. CopyTextureToBuffer requires
	// TRANSFER_SRC_OPTIMAL. Insert an explicit barrier to transition.
	// This is a no-op on Metal, GLES, software, and noop backends.
	encoder.TransitionTextures([]hal.TextureBarrier{{
		Texture: p.resolveTex,
		Usage: hal.TextureUsageTransition{
			OldUsage: gputypes.TextureUsageRenderAttachment,
			NewUsage: gputypes.TextureUsageCopySrc,
		},
	}})

	// Copy resolve texture to staging buffer for readback.
	pixelBufSize := uint64(w) * uint64(h) * 4
	stagingBuf, err := p.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "sdf_render_staging",
		Size:  pixelBufSize,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		encoder.DiscardEncoding()
		return fmt.Errorf("create staging buffer: %w", err)
	}
	defer p.device.DestroyBuffer(stagingBuf)

	encoder.CopyTextureToBuffer(p.resolveTex, stagingBuf, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{Offset: 0, BytesPerRow: w * 4, RowsPerImage: h},
		TextureBase:  hal.ImageCopyTexture{Texture: p.resolveTex, MipLevel: 0},
		Size:         hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
	}})

	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("end encoding: %w", err)
	}
	defer p.device.FreeCommandBuffer(cmdBuf)

	// Submit and wait.
	fence, err := p.device.CreateFence()
	if err != nil {
		return fmt.Errorf("create fence: %w", err)
	}
	defer p.device.DestroyFence(fence)

	if err := p.queue.Submit([]hal.CommandBuffer{cmdBuf}, fence, 1); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	fenceOK, err := p.device.Wait(fence, 1, 5*time.Second)
	if err != nil || !fenceOK {
		return fmt.Errorf("wait for GPU: ok=%v err=%w", fenceOK, err)
	}

	readback := make([]byte, pixelBufSize)
	if err := p.queue.ReadBuffer(stagingBuf, 0, readback); err != nil {
		return fmt.Errorf("readback: %w", err)
	}

	compositeBGRAOverRGBA(readback, target.Data, target.Width*target.Height)
	return nil
}

// createAndUploadBuffer creates a GPU buffer and uploads data.
func (p *SDFRenderPipeline) createAndUploadBuffer(label string, data []byte, usage gputypes.BufferUsage) (hal.Buffer, error) {
	buf, err := p.device.CreateBuffer(&hal.BufferDescriptor{
		Label: label,
		Size:  uint64(len(data)),
		Usage: usage,
	})
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", label, err)
	}
	p.queue.WriteBuffer(buf, 0, data)
	return buf, nil
}

// Size returns the current texture dimensions.
func (p *SDFRenderPipeline) Size() (uint32, uint32) {
	return p.width, p.height
}

// SDFRenderShape holds the parameters for a single shape to be rendered
// via the SDF render pipeline.
type SDFRenderShape struct {
	Kind       uint32  // 0 = circle/ellipse, 1 = rrect
	CenterX    float32 // Shape center X in pixel coordinates.
	CenterY    float32 // Shape center Y in pixel coordinates.
	Param1     float32 // radius_x (circle/ellipse) or half_width (rrect).
	Param2     float32 // radius_y (circle/ellipse) or half_height (rrect).
	Param3     float32 // corner_radius (rrect only, 0 for circle).
	HalfStroke float32 // Half stroke width (0 for filled shapes).
	IsStroked  float32 // 1.0 for stroked, 0.0 for filled.
	ColorR     float32 // Premultiplied red.
	ColorG     float32 // Premultiplied green.
	ColorB     float32 // Premultiplied blue.
	ColorA     float32 // Premultiplied alpha.
}

// DetectedShapeToRenderShape converts a gg.DetectedShape and paint into an
// SDFRenderShape ready for the render pipeline.
func DetectedShapeToRenderShape(shape gg.DetectedShape, paint *gg.Paint, stroked bool) (SDFRenderShape, bool) {
	var rs SDFRenderShape
	switch shape.Kind {
	case gg.ShapeCircle, gg.ShapeEllipse:
		rs.Kind = 0
		rs.Param1 = float32(shape.RadiusX)
		rs.Param2 = float32(shape.RadiusY)
	case gg.ShapeRect, gg.ShapeRRect:
		rs.Kind = 1
		rs.Param1 = float32(shape.Width / 2)
		rs.Param2 = float32(shape.Height / 2)
		rs.Param3 = float32(shape.CornerRadius)
	default:
		return rs, false
	}

	rs.CenterX = float32(shape.CenterX)
	rs.CenterY = float32(shape.CenterY)

	if stroked {
		rs.HalfStroke = float32(paint.EffectiveLineWidth() / 2)
		rs.IsStroked = 1.0
	}

	color := getColorFromPaint(paint)
	// Premultiply color for GPU blending.
	rs.ColorR = float32(color.R * color.A)
	rs.ColorG = float32(color.G * color.A)
	rs.ColorB = float32(color.B * color.A)
	rs.ColorA = float32(color.A)

	return rs, true
}

// sdfRenderVertexLayout returns the vertex buffer layout for the SDF render pipeline.
func sdfRenderVertexLayout() []gputypes.VertexBufferLayout {
	return []gputypes.VertexBufferLayout{
		{
			ArrayStride: sdfRenderVertexStride,
			StepMode:    gputypes.VertexStepModeVertex,
			Attributes: []gputypes.VertexAttribute{
				{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0},  // position
				{Format: gputypes.VertexFormatFloat32x2, Offset: 8, ShaderLocation: 1},  // local
				{Format: gputypes.VertexFormatFloat32, Offset: 16, ShaderLocation: 2},   // shape_kind
				{Format: gputypes.VertexFormatFloat32, Offset: 20, ShaderLocation: 3},   // param1
				{Format: gputypes.VertexFormatFloat32, Offset: 24, ShaderLocation: 4},   // param2
				{Format: gputypes.VertexFormatFloat32, Offset: 28, ShaderLocation: 5},   // param3
				{Format: gputypes.VertexFormatFloat32, Offset: 32, ShaderLocation: 6},   // half_stroke
				{Format: gputypes.VertexFormatFloat32, Offset: 36, ShaderLocation: 7},   // is_stroked
				{Format: gputypes.VertexFormatFloat32x4, Offset: 40, ShaderLocation: 8}, // color
			},
		},
	}
}

// buildSDFRenderVertices generates vertex data for all shapes. Each shape
// produces 6 vertices (2 triangles forming a screen-aligned quad).
// The quad covers the shape's bounding box plus an AA margin.
func buildSDFRenderVertices(shapes []SDFRenderShape, _ uint32, _ uint32) []byte {
	// 6 vertices per shape, sdfRenderVertexStride bytes per vertex.
	buf := make([]byte, len(shapes)*6*sdfRenderVertexStride)
	offset := 0

	for i := range shapes {
		s := &shapes[i]

		// Compute bounding box half-extents.
		// For circles/ellipses, Param1/2 are radii.
		// For rrects, Param1/2 are half dimensions.
		// Both produce the same bounding box calculation.
		halfW := s.Param1
		halfH := s.Param2
		// Expand for stroke + AA margin.
		halfW += s.HalfStroke + sdfRenderAAMargin
		halfH += s.HalfStroke + sdfRenderAAMargin

		// Quad corners in pixel coordinates and local (shape-relative) coordinates.
		// Triangle 1: TL, TR, BL
		// Triangle 2: TR, BR, BL
		type corner struct {
			px, py float32 // pixel position
			lx, ly float32 // local offset from center
		}
		tl := corner{s.CenterX - halfW, s.CenterY - halfH, -halfW, -halfH}
		tr := corner{s.CenterX + halfW, s.CenterY - halfH, halfW, -halfH}
		bl := corner{s.CenterX - halfW, s.CenterY + halfH, -halfW, halfH}
		br := corner{s.CenterX + halfW, s.CenterY + halfH, halfW, halfH}

		corners := [6]corner{tl, tr, bl, tr, br, bl}

		for _, c := range corners {
			writeSDFRenderVertex(buf[offset:], c.px, c.py, c.lx, c.ly, s)
			offset += sdfRenderVertexStride
		}
	}

	return buf
}

// writeSDFRenderVertex writes a single vertex into the buffer at the current position.
func writeSDFRenderVertex(buf []byte, px, py, lx, ly float32, s *SDFRenderShape) {
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(px))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(py))
	binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(lx))
	binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(ly))
	binary.LittleEndian.PutUint32(buf[16:20], math.Float32bits(float32(s.Kind)))
	binary.LittleEndian.PutUint32(buf[20:24], math.Float32bits(s.Param1))
	binary.LittleEndian.PutUint32(buf[24:28], math.Float32bits(s.Param2))
	binary.LittleEndian.PutUint32(buf[28:32], math.Float32bits(s.Param3))
	binary.LittleEndian.PutUint32(buf[32:36], math.Float32bits(s.HalfStroke))
	binary.LittleEndian.PutUint32(buf[36:40], math.Float32bits(s.IsStroked))
	binary.LittleEndian.PutUint32(buf[40:44], math.Float32bits(s.ColorR))
	binary.LittleEndian.PutUint32(buf[44:48], math.Float32bits(s.ColorG))
	binary.LittleEndian.PutUint32(buf[48:52], math.Float32bits(s.ColorB))
	binary.LittleEndian.PutUint32(buf[52:56], math.Float32bits(s.ColorA))
}

// makeSDFRenderUniform creates the 16-byte uniform buffer.
// Layout: viewport (vec2<f32>) + padding (vec2<f32>).
func makeSDFRenderUniform(w, h uint32) []byte {
	buf := make([]byte, sdfRenderUniformSize)
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(float32(w)))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(float32(h)))
	// Padding bytes 8..15 remain zero.
	return buf
}
