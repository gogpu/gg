//go:build !nogpu

package gpu

import (
	_ "embed"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// Embedded WGSL shader sources for stencil-then-cover rendering.

//go:embed shaders/stencil_fill.wgsl
var stencilFillShaderSource string

//go:embed shaders/cover.wgsl
var coverShaderSource string

// stencilFillUniformSize is the byte size of the stencil fill uniform buffer.
// Layout: viewport (vec2<f32>) + padding (vec2<f32>) = 16 bytes.
const stencilFillUniformSize = 16

// coverUniformSize is the byte size of the cover pass uniform buffer.
// Layout: viewport (vec2<f32>) + padding (vec2<f32>) + color (vec4<f32>) = 32 bytes.
const coverUniformSize = 32

// vertexStride is the byte stride per vertex: 2 x float32 (x, y) = 8 bytes.
const vertexStride = 8

// createPipelines compiles shaders and creates the stencil fill and cover
// render pipelines. Both pipelines share the same bind group layout (one
// uniform buffer at group(0) binding(0)) and vertex layout (float32x2 at
// location(0)).
//
// Two stencil fill pipeline variants are created:
//   - NonZero: front=IncrementWrap / back=DecrementWrap (winding number).
//   - EvenOdd: front=Invert / back=Invert (parity count).
//
// The cover pipeline reads the stencil buffer with NotEqual(0) and resets
// stencil values to zero via PassOp=Zero after writing the fill color.
// It is shared by both fill rules.
func (sr *StencilRenderer) createPipelines() error { //nolint:funlen // GPU pipeline descriptors are inherently verbose
	// Compile shaders.
	stencilShader, err := sr.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "stencil_fill_shader",
		WGSL:  stencilFillShaderSource,
	})
	if err != nil {
		return fmt.Errorf("compile stencil fill shader: %w", err)
	}
	sr.stencilFillShader = stencilShader

	coverShader, err := sr.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "cover_shader",
		WGSL:  coverShaderSource,
	})
	if err != nil {
		return fmt.Errorf("compile cover shader: %w", err)
	}
	sr.coverShader = coverShader

	// Create bind group layout shared by both pipelines.
	// One uniform buffer at group(0) binding(0), visible to vertex + fragment stages.
	uniformLayout, err := sr.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "stencil_cover_uniform_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create uniform bind group layout: %w", err)
	}
	sr.uniformLayout = uniformLayout

	// Create pipeline layouts.
	stencilPipeLayout, err := sr.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "stencil_fill_pipe_layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{sr.uniformLayout},
	})
	if err != nil {
		return fmt.Errorf("create stencil pipeline layout: %w", err)
	}
	sr.stencilPipeLayout = stencilPipeLayout

	coverBGLayouts := []*wgpu.BindGroupLayout{sr.uniformLayout}
	hasClip := sr.clipBindLayout != nil
	if hasClip {
		coverBGLayouts = append(coverBGLayouts, sr.clipBindLayout)
	}
	coverPipeLayout, err := sr.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "cover_pipe_layout",
		BindGroupLayouts: coverBGLayouts,
	})
	if err != nil {
		return fmt.Errorf("create cover pipeline layout: %w", err)
	}
	sr.coverPipeLayout = coverPipeLayout
	sr.coverPipeLayoutHasClip = hasClip

	// Shared vertex buffer layout: float32x2 position at location(0).
	vertexBufferLayout := []gputypes.VertexBufferLayout{
		{
			ArrayStride: vertexStride,
			StepMode:    gputypes.VertexStepModeVertex,
			Attributes: []gputypes.VertexAttribute{
				{
					Format:         gputypes.VertexFormatFloat32x2,
					Offset:         0,
					ShaderLocation: 0,
				},
			},
		},
	}

	// Shared multisample state: 4x MSAA.
	multisample := gputypes.MultisampleState{
		Count: sampleCount,
		Mask:  0xFFFFFFFF,
	}

	// Shared primitive state: triangle list, no culling.
	primitive := gputypes.PrimitiveState{
		Topology: gputypes.PrimitiveTopologyTriangleList,
		CullMode: gputypes.CullModeNone,
	}

	// --- Stencil Fill Pipeline ---
	//
	// Non-zero fill rule: front faces increment, back faces decrement.
	// Color writes are suppressed (WriteMask=None) since this pass only
	// updates the stencil buffer. A dummy fragment shader is included for
	// backend compatibility.
	nonZeroStencilPipeline, err := sr.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "stencil_fill_pipeline",
		Layout: sr.stencilPipeLayout,
		Vertex: wgpu.VertexState{
			Module:     sr.stencilFillShader,
			EntryPoint: "vs_main",
			Buffers:    vertexBufferLayout,
		},
		Fragment: &wgpu.FragmentState{
			Module:     sr.stencilFillShader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					WriteMask: gputypes.ColorWriteMaskNone,
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
				PassOp:      wgpu.StencilOperationIncrementWrap,
			},
			StencilBack: wgpu.StencilFaceState{
				Compare:     gputypes.CompareFunctionAlways,
				FailOp:      wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep,
				PassOp:      wgpu.StencilOperationDecrementWrap,
			},
			StencilReadMask:  0xFF,
			StencilWriteMask: 0xFF,
		},
		Multisample: multisample,
		Primitive:   primitive,
	})
	if err != nil {
		return fmt.Errorf("create stencil fill pipeline: %w", err)
	}
	sr.nonZeroStencilPipeline = nonZeroStencilPipeline

	// --- Even-Odd Stencil Fill Pipeline ---
	//
	// Even-odd fill rule: both front and back faces invert the stencil value.
	// A pixel with odd winding count has stencil != 0 (inside), even count
	// wraps back to 0 (outside). Same shader and layout as the non-zero variant.
	evenOddStencilPipeline, err := sr.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "stencil_fill_even_odd_pipeline",
		Layout: sr.stencilPipeLayout,
		Vertex: wgpu.VertexState{
			Module:     sr.stencilFillShader,
			EntryPoint: "vs_main",
			Buffers:    vertexBufferLayout,
		},
		Fragment: &wgpu.FragmentState{
			Module:     sr.stencilFillShader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					WriteMask: gputypes.ColorWriteMaskNone,
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
				PassOp:      wgpu.StencilOperationInvert,
			},
			StencilBack: wgpu.StencilFaceState{
				Compare:     gputypes.CompareFunctionAlways,
				FailOp:      wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep,
				PassOp:      wgpu.StencilOperationInvert,
			},
			StencilReadMask:  0xFF,
			StencilWriteMask: 0xFF,
		},
		Multisample: multisample,
		Primitive:   primitive,
	})
	if err != nil {
		return fmt.Errorf("create even-odd stencil fill pipeline: %w", err)
	}
	sr.evenOddStencilPipeline = evenOddStencilPipeline

	// --- Cover Pipeline ---
	//
	// Reads stencil buffer: only pixels with stencil != 0 pass the test.
	// PassOp=Zero resets stencil to 0 after coloring, clearing it for the
	// next path. Premultiplied alpha blending composites the fill color.
	premulBlend := gputypes.BlendStatePremultiplied()
	nonZeroCoverPipeline, err := sr.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "cover_pipeline",
		Layout: sr.coverPipeLayout,
		Vertex: wgpu.VertexState{
			Module:     sr.coverShader,
			EntryPoint: "vs_main",
			Buffers:    vertexBufferLayout,
		},
		Fragment: &wgpu.FragmentState{
			Module:     sr.coverShader,
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
				Compare:     gputypes.CompareFunctionNotEqual,
				FailOp:      wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep,
				PassOp:      wgpu.StencilOperationZero,
			},
			StencilBack: wgpu.StencilFaceState{
				Compare:     gputypes.CompareFunctionNotEqual,
				FailOp:      wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep,
				PassOp:      wgpu.StencilOperationZero,
			},
			StencilReadMask:  0xFF,
			StencilWriteMask: 0xFF,
		},
		Multisample: multisample,
		Primitive:   primitive,
	})
	if err != nil {
		return fmt.Errorf("create cover pipeline: %w", err)
	}
	sr.nonZeroCoverPipeline = nonZeroCoverPipeline

	return nil
}

// ensureDepthClipPipelines creates the depth-clipped pipeline variants for
// GPU-CLIP-003a. These are identical to the normal pipelines except they use
// DepthCompare=GreaterEqual to restrict rendering to pixels where the depth
// clip geometry previously wrote Z=0.0.
//
// Stencil fill variants: same stencil operations but with depth test. This
// ensures the stencil buffer is only modified within the clip region.
//
// Cover variant: same stencil test + color write but with depth test. Only
// pixels inside the clip AND with non-zero stencil receive fill color.
//
// Created lazily on first use to avoid unnecessary GPU compilation.
func (sr *StencilRenderer) ensureDepthClipPipelines() error { //nolint:funlen // GPU pipeline descriptors are inherently verbose
	if sr.pipelineWithDepthClipNZ != nil {
		return nil // already created
	}
	if sr.stencilFillShader == nil || sr.stencilPipeLayout == nil {
		if err := sr.createPipelines(); err != nil {
			return err
		}
	}

	// Shared vertex buffer layout, primitive, multisample — same as base pipelines.
	vertexBufferLayout := []gputypes.VertexBufferLayout{
		{
			ArrayStride: vertexStride,
			StepMode:    gputypes.VertexStepModeVertex,
			Attributes: []gputypes.VertexAttribute{
				{
					Format:         gputypes.VertexFormatFloat32x2,
					Offset:         0,
					ShaderLocation: 0,
				},
			},
		},
	}
	multisample := gputypes.MultisampleState{Count: sampleCount, Mask: 0xFFFFFFFF}
	primitive := gputypes.PrimitiveState{
		Topology: gputypes.PrimitiveTopologyTriangleList,
		CullMode: gputypes.CullModeNone,
	}

	// --- Non-zero stencil fill + depth clip ---
	nzPipeline, err := sr.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "stencil_fill_depth_clip_pipeline",
		Layout: sr.stencilPipeLayout,
		Vertex: wgpu.VertexState{
			Module:     sr.stencilFillShader,
			EntryPoint: "vs_main",
			Buffers:    vertexBufferLayout,
		},
		Fragment: &wgpu.FragmentState{
			Module:     sr.stencilFillShader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatBGRA8Unorm, WriteMask: gputypes.ColorWriteMaskNone},
			},
		},
		DepthStencil: &wgpu.DepthStencilState{
			Format:            gputypes.TextureFormatDepth24PlusStencil8,
			DepthWriteEnabled: false,
			DepthCompare:      gputypes.CompareFunctionGreaterEqual,
			StencilFront: wgpu.StencilFaceState{
				Compare: gputypes.CompareFunctionAlways, FailOp: wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationIncrementWrap,
			},
			StencilBack: wgpu.StencilFaceState{
				Compare: gputypes.CompareFunctionAlways, FailOp: wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationDecrementWrap,
			},
			StencilReadMask:  0xFF,
			StencilWriteMask: 0xFF,
		},
		Multisample: multisample,
		Primitive:   primitive,
	})
	if err != nil {
		return fmt.Errorf("create stencil fill depth clip pipeline (NZ): %w", err)
	}
	sr.pipelineWithDepthClipNZ = nzPipeline

	// --- Even-odd stencil fill + depth clip ---
	eoPipeline, err := sr.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "stencil_fill_even_odd_depth_clip_pipeline",
		Layout: sr.stencilPipeLayout,
		Vertex: wgpu.VertexState{
			Module:     sr.stencilFillShader,
			EntryPoint: "vs_main",
			Buffers:    vertexBufferLayout,
		},
		Fragment: &wgpu.FragmentState{
			Module:     sr.stencilFillShader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatBGRA8Unorm, WriteMask: gputypes.ColorWriteMaskNone},
			},
		},
		DepthStencil: &wgpu.DepthStencilState{
			Format:            gputypes.TextureFormatDepth24PlusStencil8,
			DepthWriteEnabled: false,
			DepthCompare:      gputypes.CompareFunctionGreaterEqual,
			StencilFront: wgpu.StencilFaceState{
				Compare: gputypes.CompareFunctionAlways, FailOp: wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationInvert,
			},
			StencilBack: wgpu.StencilFaceState{
				Compare: gputypes.CompareFunctionAlways, FailOp: wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationInvert,
			},
			StencilReadMask:  0xFF,
			StencilWriteMask: 0xFF,
		},
		Multisample: multisample,
		Primitive:   primitive,
	})
	if err != nil {
		return fmt.Errorf("create stencil fill depth clip pipeline (EO): %w", err)
	}
	sr.pipelineWithDepthClipEO = eoPipeline

	// --- Cover pipeline + depth clip ---
	premulBlend := gputypes.BlendStatePremultiplied()
	coverPipeline, err := sr.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "cover_depth_clip_pipeline",
		Layout: sr.coverPipeLayout,
		Vertex: wgpu.VertexState{
			Module:     sr.coverShader,
			EntryPoint: "vs_main",
			Buffers:    vertexBufferLayout,
		},
		Fragment: &wgpu.FragmentState{
			Module:     sr.coverShader,
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
			DepthCompare:      gputypes.CompareFunctionGreaterEqual,
			StencilFront: wgpu.StencilFaceState{
				Compare: gputypes.CompareFunctionNotEqual, FailOp: wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationZero,
			},
			StencilBack: wgpu.StencilFaceState{
				Compare: gputypes.CompareFunctionNotEqual, FailOp: wgpu.StencilOperationKeep,
				DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationZero,
			},
			StencilReadMask:  0xFF,
			StencilWriteMask: 0xFF,
		},
		Multisample: multisample,
		Primitive:   primitive,
	})
	if err != nil {
		return fmt.Errorf("create cover depth clip pipeline: %w", err)
	}
	sr.pipelineWithDepthClipCover = coverPipeline

	return nil
}

// destroyPipelines releases all pipeline resources in reverse creation order.
// Safe to call on a renderer with no pipelines or with partially created pipelines.
func (sr *StencilRenderer) destroyPipelines() {
	if sr.device == nil {
		return
	}
	// Depth-clipped variants (GPU-CLIP-003a).
	if sr.pipelineWithDepthClipCover != nil {
		sr.pipelineWithDepthClipCover.Release()
		sr.pipelineWithDepthClipCover = nil
	}
	if sr.pipelineWithDepthClipEO != nil {
		sr.pipelineWithDepthClipEO.Release()
		sr.pipelineWithDepthClipEO = nil
	}
	if sr.pipelineWithDepthClipNZ != nil {
		sr.pipelineWithDepthClipNZ.Release()
		sr.pipelineWithDepthClipNZ = nil
	}
	// Base pipelines.
	if sr.nonZeroCoverPipeline != nil {
		sr.nonZeroCoverPipeline.Release()
		sr.nonZeroCoverPipeline = nil
	}
	if sr.evenOddStencilPipeline != nil {
		sr.evenOddStencilPipeline.Release()
		sr.evenOddStencilPipeline = nil
	}
	if sr.nonZeroStencilPipeline != nil {
		sr.nonZeroStencilPipeline.Release()
		sr.nonZeroStencilPipeline = nil
	}
	if sr.coverPipeLayout != nil {
		sr.coverPipeLayout.Release()
		sr.coverPipeLayout = nil
		sr.coverPipeLayoutHasClip = false
	}
	if sr.stencilPipeLayout != nil {
		sr.stencilPipeLayout.Release()
		sr.stencilPipeLayout = nil
	}
	if sr.uniformLayout != nil {
		sr.uniformLayout.Release()
		sr.uniformLayout = nil
	}
	if sr.coverShader != nil {
		sr.coverShader.Release()
		sr.coverShader = nil
	}
	if sr.stencilFillShader != nil {
		sr.stencilFillShader.Release()
		sr.stencilFillShader = nil
	}
}
