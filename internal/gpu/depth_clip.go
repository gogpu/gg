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

//go:embed shaders/depth_clip.wgsl
var depthClipShaderSource string

// depthClipUniformSize is the byte size of the depth clip uniform buffer.
// Layout: viewport (vec2<f32>) + pad (vec2<f32>) = 16 bytes.
// Same layout as stencil_fill.wgsl uniforms for consistency.
const depthClipUniformSize = 16

// DepthClipPipeline manages the GPU pipeline for depth-based arbitrary path
// clipping (GPU-CLIP-003a). Clip paths are fan-tessellated and rendered to
// the depth buffer with ColorWriteMask=None, leaving stencil untouched.
//
// This follows the Flutter Impeller pattern (PR #50856): depth buffer for
// clip discrimination, stencil exclusively for Tier 2b path fill.
//
// Depth model:
//   - DepthClearValue = 1.0 (existing, unchanged)
//   - Clip path writes Z = 0.0 where geometry exists → depth buffer = 0.0
//   - Where clip absent, depth buffer remains 1.0 (clear value)
//   - Content pipelines use DepthCompare=GreaterEqual, fragment Z = 0.0
//   - Where clip drawn:     0.0 >= 0.0 → PASS
//   - Where clip NOT drawn: 0.0 >= 1.0 → FAIL
//
// Nested clips:
//
//	All clip levels write Z=0.0. The intersection of nested clips happens
//	GEOMETRICALLY — clip 2's path only covers pixels where clip 1 already
//	wrote 0.0. Content at any depth tests GreaterEqual(0.0, buffer): passes
//	where ANY clip wrote 0.0, fails where no clip touched. This is the
//	simplest correct nested model.
//
//	Clip restore (pop) limitation: within a single ScissorGroup, depth writes
//	cannot be "undone" without redrawing. For v1, each ScissorGroup has at
//	most ONE ClipPath. Nested clips from the scene graph create nested groups
//	or use the Context CPU clip stack. This matches other renderers (SDF,
//	convex, text) which each have one depth clip state per group.
//
// Architecture:
//
//	ScissorGroup.ClipPath → FanTessellator → depth-only draw (before content)
//	  → DepthCompare=Always, DepthWriteEnabled=true
//	  → ColorWriteMask=None, StencilMask=0x00
//	  → writes Z=0.0 to depth buffer
type DepthClipPipeline struct {
	device *wgpu.Device
	queue  *wgpu.Queue

	shader       *wgpu.ShaderModule
	uniformBGL   *wgpu.BindGroupLayout
	pipeLayout   *wgpu.PipelineLayout
	pipeline     *wgpu.RenderPipeline
	tessellator  *FanTessellator
	uniformBuf   *wgpu.Buffer
	bindGroup    *wgpu.BindGroup
	vertBuf      *wgpu.Buffer
	vertBufCap   uint64
	vertexStaged []byte // CPU staging buffer for vertex data
}

// NewDepthClipPipeline creates a new depth clip pipeline for the given device.
// The pipeline is not created until ensurePipeline() is called.
func NewDepthClipPipeline(device *wgpu.Device, queue *wgpu.Queue) *DepthClipPipeline {
	return &DepthClipPipeline{
		device:      device,
		queue:       queue,
		tessellator: NewFanTessellator(),
	}
}

// ensurePipeline compiles the shader and creates the GPU pipeline if not
// already created. The pipeline writes depth only (ColorWriteMask=None)
// with DepthCompare=Always, DepthWriteEnabled=true.
func (p *DepthClipPipeline) ensurePipeline() error {
	if p.pipeline != nil {
		return nil
	}

	// Compile shader.
	shader, err := p.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "depth_clip_shader",
		WGSL:  depthClipShaderSource,
	})
	if err != nil {
		return fmt.Errorf("compile depth clip shader: %w", err)
	}
	p.shader = shader

	// Bind group layout: one uniform buffer at group(0) binding(0).
	bgl, err := p.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "depth_clip_uniform_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create depth clip bind group layout: %w", err)
	}
	p.uniformBGL = bgl

	// Pipeline layout: just the uniform bind group (no clip @group(1) needed
	// for the clip pipeline itself -- it IS the clip).
	pipeLayout, err := p.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "depth_clip_pipe_layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{p.uniformBGL},
	})
	if err != nil {
		return fmt.Errorf("create depth clip pipeline layout: %w", err)
	}
	p.pipeLayout = pipeLayout

	// Vertex buffer layout: float32x2 position at location(0).
	vertexBufLayout := []gputypes.VertexBufferLayout{
		{
			ArrayStride: vertexStride, // 8 bytes (2 x float32)
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

	// Create render pipeline: depth-only, no color, no stencil interaction.
	pipeline, err := p.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "depth_clip_pipeline",
		Layout: p.pipeLayout,
		Vertex: wgpu.VertexState{
			Module:     p.shader,
			EntryPoint: "vs_main",
			Buffers:    vertexBufLayout,
		},
		Fragment: &wgpu.FragmentState{
			Module:     p.shader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					WriteMask: gputypes.ColorWriteMaskNone, // no color output
				},
			},
		},
		DepthStencil: &wgpu.DepthStencilState{
			Format:            gputypes.TextureFormatDepth24PlusStencil8,
			DepthWriteEnabled: true,
			DepthCompare:      gputypes.CompareFunctionAlways, // always write clip depth
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
			StencilReadMask:  0x00, // don't read stencil
			StencilWriteMask: 0x00, // don't write stencil
		},
		Multisample: gputypes.MultisampleState{
			Count: sampleCount,
			Mask:  0xFFFFFFFF,
		},
		Primitive: gputypes.PrimitiveState{
			Topology: gputypes.PrimitiveTopologyTriangleList,
			CullMode: gputypes.CullModeNone,
		},
	})
	if err != nil {
		return fmt.Errorf("create depth clip pipeline: %w", err)
	}
	p.pipeline = pipeline

	return nil
}

// DepthClipResources holds per-frame resources for one depth clip draw.
type DepthClipResources struct {
	vertBuf   *wgpu.Buffer
	bindGroup *wgpu.BindGroup
	vertCount uint32
}

// BuildClipResources tessellates the clip path and uploads vertices + uniforms
// to the GPU. Returns resources needed for RecordDraw, or nil if the path is
// empty (no clip to draw).
func (p *DepthClipPipeline) BuildClipResources(
	clipPath *gg.Path,
	w, h uint32,
) (*DepthClipResources, error) {
	// Tessellate clip path into fan triangles.
	p.tessellator.Reset()
	vertCount := p.tessellator.TessellatePath(clipPath)
	if vertCount == 0 {
		return nil, nil //nolint:nilnil // empty clip path, nothing to draw
	}

	verts := p.tessellator.Vertices()
	vertBytes := uint64(len(verts)) * 4 //nolint:gosec // len(verts) bounded by tessellator

	// Ensure vertex buffer (grow-only).
	if p.vertBuf == nil || p.vertBufCap < vertBytes {
		if p.vertBuf != nil {
			p.vertBuf.Release()
		}
		newCap := vertBytes
		if newCap < 4096 {
			newCap = 4096 // minimum 4KB
		}
		buf, err := p.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "depth_clip_vert",
			Size:  newCap,
			Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			return nil, fmt.Errorf("create depth clip vertex buffer: %w", err)
		}
		p.vertBuf = buf
		p.vertBufCap = newCap
	}

	// Stage vertex data.
	if uint64(cap(p.vertexStaged)) < vertBytes {
		p.vertexStaged = make([]byte, vertBytes)
	}
	staging := p.vertexStaged[:vertBytes]
	for i, v := range verts {
		binary.LittleEndian.PutUint32(staging[i*4:], math.Float32bits(v))
	}
	if err := p.queue.WriteBuffer(p.vertBuf, 0, staging); err != nil {
		return nil, fmt.Errorf("write depth clip vertices: %w", err)
	}

	// Ensure uniform buffer.
	if p.uniformBuf == nil {
		buf, err := p.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "depth_clip_uniform",
			Size:  depthClipUniformSize,
			Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		})
		if err != nil {
			return nil, fmt.Errorf("create depth clip uniform buffer: %w", err)
		}
		p.uniformBuf = buf
	}

	// Write uniforms: viewport (vec2<f32>) + pad (vec2<f32>).
	uniformData := make([]byte, depthClipUniformSize)
	binary.LittleEndian.PutUint32(uniformData[0:4], math.Float32bits(float32(w)))
	binary.LittleEndian.PutUint32(uniformData[4:8], math.Float32bits(float32(h)))
	// bytes 8..15 remain zero (padding).
	if err := p.queue.WriteBuffer(p.uniformBuf, 0, uniformData); err != nil {
		return nil, fmt.Errorf("write depth clip uniforms: %w", err)
	}

	// Ensure bind group (recreated if uniform buffer changed).
	if p.bindGroup == nil {
		bg, err := p.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label:  "depth_clip_bind",
			Layout: p.uniformBGL,
			Entries: []wgpu.BindGroupEntry{
				{Binding: 0, Buffer: p.uniformBuf, Offset: 0, Size: depthClipUniformSize},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create depth clip bind group: %w", err)
		}
		p.bindGroup = bg
	}

	return &DepthClipResources{
		vertBuf:   p.vertBuf,
		bindGroup: p.bindGroup,
		vertCount: uint32(vertCount), //nolint:gosec // bounded by tessellator
	}, nil
}

// RecordDraw records the depth clip draw commands into a render pass.
// This must be called BEFORE any content draws in the group, so the depth
// buffer is populated before content pipelines test against it.
func (p *DepthClipPipeline) RecordDraw(rp *wgpu.RenderPassEncoder, res *DepthClipResources) {
	if res == nil || res.vertCount == 0 {
		return
	}
	rp.SetPipeline(p.pipeline)
	rp.SetBindGroup(0, res.bindGroup, nil)
	rp.SetVertexBuffer(0, res.vertBuf, 0)
	rp.Draw(res.vertCount, 1, 0, 0)
}

// Destroy releases all GPU resources held by the depth clip pipeline.
func (p *DepthClipPipeline) Destroy() {
	if p.bindGroup != nil {
		p.bindGroup.Release()
		p.bindGroup = nil
	}
	if p.uniformBuf != nil {
		p.uniformBuf.Release()
		p.uniformBuf = nil
	}
	if p.vertBuf != nil {
		p.vertBuf.Release()
		p.vertBuf = nil
		p.vertBufCap = 0
	}
	if p.pipeline != nil {
		p.pipeline.Release()
		p.pipeline = nil
	}
	if p.pipeLayout != nil {
		p.pipeLayout.Release()
		p.pipeLayout = nil
	}
	if p.uniformBGL != nil {
		p.uniformBGL.Release()
		p.uniformBGL = nil
	}
	if p.shader != nil {
		p.shader.Release()
		p.shader = nil
	}
}
