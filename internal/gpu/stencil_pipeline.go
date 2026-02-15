//go:build !nogpu

package gpu

import (
	_ "embed"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
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
	stencilShader, err := sr.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "stencil_fill_shader",
		Source: hal.ShaderSource{WGSL: stencilFillShaderSource},
	})
	if err != nil {
		return fmt.Errorf("compile stencil fill shader: %w", err)
	}
	sr.stencilFillShader = stencilShader

	coverShader, err := sr.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "cover_shader",
		Source: hal.ShaderSource{WGSL: coverShaderSource},
	})
	if err != nil {
		return fmt.Errorf("compile cover shader: %w", err)
	}
	sr.coverShader = coverShader

	// Create bind group layout shared by both pipelines.
	// One uniform buffer at group(0) binding(0), visible to vertex + fragment stages.
	uniformLayout, err := sr.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
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
	stencilPipeLayout, err := sr.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "stencil_fill_pipe_layout",
		BindGroupLayouts: []hal.BindGroupLayout{sr.uniformLayout},
	})
	if err != nil {
		return fmt.Errorf("create stencil pipeline layout: %w", err)
	}
	sr.stencilPipeLayout = stencilPipeLayout

	coverPipeLayout, err := sr.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "cover_pipe_layout",
		BindGroupLayouts: []hal.BindGroupLayout{sr.uniformLayout},
	})
	if err != nil {
		return fmt.Errorf("create cover pipeline layout: %w", err)
	}
	sr.coverPipeLayout = coverPipeLayout

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
	nonZeroStencilPipeline, err := sr.device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{ //nolint:dupl // NonZero vs EvenOdd differ only in stencil ops
		Label:  "stencil_fill_pipeline",
		Layout: sr.stencilPipeLayout,
		Vertex: hal.VertexState{
			Module:     sr.stencilFillShader,
			EntryPoint: "vs_main",
			Buffers:    vertexBufferLayout,
		},
		Fragment: &hal.FragmentState{
			Module:     sr.stencilFillShader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					WriteMask: gputypes.ColorWriteMaskNone,
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
				PassOp:      hal.StencilOperationIncrementWrap,
			},
			StencilBack: hal.StencilFaceState{
				Compare:     gputypes.CompareFunctionAlways,
				FailOp:      hal.StencilOperationKeep,
				DepthFailOp: hal.StencilOperationKeep,
				PassOp:      hal.StencilOperationDecrementWrap,
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
	evenOddStencilPipeline, err := sr.device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{ //nolint:dupl // EvenOdd vs NonZero differ only in stencil ops
		Label:  "stencil_fill_even_odd_pipeline",
		Layout: sr.stencilPipeLayout,
		Vertex: hal.VertexState{
			Module:     sr.stencilFillShader,
			EntryPoint: "vs_main",
			Buffers:    vertexBufferLayout,
		},
		Fragment: &hal.FragmentState{
			Module:     sr.stencilFillShader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					WriteMask: gputypes.ColorWriteMaskNone,
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
				PassOp:      hal.StencilOperationInvert,
			},
			StencilBack: hal.StencilFaceState{
				Compare:     gputypes.CompareFunctionAlways,
				FailOp:      hal.StencilOperationKeep,
				DepthFailOp: hal.StencilOperationKeep,
				PassOp:      hal.StencilOperationInvert,
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
	nonZeroCoverPipeline, err := sr.device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label:  "cover_pipeline",
		Layout: sr.coverPipeLayout,
		Vertex: hal.VertexState{
			Module:     sr.coverShader,
			EntryPoint: "vs_main",
			Buffers:    vertexBufferLayout,
		},
		Fragment: &hal.FragmentState{
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
		DepthStencil: &hal.DepthStencilState{
			Format:            gputypes.TextureFormatDepth24PlusStencil8,
			DepthWriteEnabled: false,
			DepthCompare:      gputypes.CompareFunctionAlways,
			StencilFront: hal.StencilFaceState{
				Compare:     gputypes.CompareFunctionNotEqual,
				FailOp:      hal.StencilOperationKeep,
				DepthFailOp: hal.StencilOperationKeep,
				PassOp:      hal.StencilOperationZero,
			},
			StencilBack: hal.StencilFaceState{
				Compare:     gputypes.CompareFunctionNotEqual,
				FailOp:      hal.StencilOperationKeep,
				DepthFailOp: hal.StencilOperationKeep,
				PassOp:      hal.StencilOperationZero,
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

// destroyPipelines releases all pipeline resources in reverse creation order.
// Safe to call on a renderer with no pipelines or with partially created pipelines.
func (sr *StencilRenderer) destroyPipelines() {
	if sr.device == nil {
		return
	}
	if sr.nonZeroCoverPipeline != nil {
		sr.device.DestroyRenderPipeline(sr.nonZeroCoverPipeline)
		sr.nonZeroCoverPipeline = nil
	}
	if sr.evenOddStencilPipeline != nil {
		sr.device.DestroyRenderPipeline(sr.evenOddStencilPipeline)
		sr.evenOddStencilPipeline = nil
	}
	if sr.nonZeroStencilPipeline != nil {
		sr.device.DestroyRenderPipeline(sr.nonZeroStencilPipeline)
		sr.nonZeroStencilPipeline = nil
	}
	if sr.coverPipeLayout != nil {
		sr.device.DestroyPipelineLayout(sr.coverPipeLayout)
		sr.coverPipeLayout = nil
	}
	if sr.stencilPipeLayout != nil {
		sr.device.DestroyPipelineLayout(sr.stencilPipeLayout)
		sr.stencilPipeLayout = nil
	}
	if sr.uniformLayout != nil {
		sr.device.DestroyBindGroupLayout(sr.uniformLayout)
		sr.uniformLayout = nil
	}
	if sr.coverShader != nil {
		sr.device.DestroyShaderModule(sr.coverShader)
		sr.coverShader = nil
	}
	if sr.stencilFillShader != nil {
		sr.device.DestroyShaderModule(sr.stencilFillShader)
		sr.stencilFillShader = nil
	}
}
