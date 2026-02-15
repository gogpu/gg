//go:build !nogpu

package gpu

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

//go:embed shaders/convex.wgsl
var convexShaderSource string

// convexVertexStride is the byte stride per vertex in the convex render pipeline.
// Layout per vertex:
//
//	position (vec2<f32>) = 8 bytes  (location 0)
//	coverage (f32)       = 4 bytes  (location 1)
//	color    (vec4<f32>) = 16 bytes (location 2)
//
// Total = 28 bytes per vertex.
const convexVertexStride = 28

// convexAAExpand is the outward expansion distance in pixels for the
// anti-aliasing fringe around convex polygon edges. 0.5px provides a
// smooth one-pixel transition zone.
const convexAAExpand = 0.5

// ConvexDrawCommand holds the geometry and paint for a single convex polygon
// to be rendered via the convex fast-path renderer. Points must form a convex
// polygon (verified by IsConvex before queuing).
type ConvexDrawCommand struct {
	// Points are the convex polygon vertices in pixel coordinates,
	// after any curve flattening. The polygon is treated as closed
	// (last point connects to first).
	Points []gg.Point

	// Color is the premultiplied RGBA fill color.
	Color [4]float32
}

// ConvexRenderer renders convex polygons in a single draw call with per-edge
// analytic anti-aliasing. No stencil buffer is needed.
//
// This is Tier 2a in the GPU rendering hierarchy:
//
//	Tier 1:  SDF fragment shader (circles, rects, rrects)
//	Tier 2a: Convex fast-path (this) -- single draw, per-edge AA
//	Tier 2b: Stencil-then-cover -- arbitrary paths
//
// The algorithm fans from the polygon centroid, generating interior triangles
// with coverage=1.0 and AA fringe strips (0.5px outward expansion) with
// coverage ramping from 1.0 to 0.0 at the outermost edge.
//
// For the unified render pass (GPURenderSession), use pipelineWithStencil
// which includes a depth/stencil state that ignores the stencil buffer
// (Compare=Always, all ops=Keep, masks=0x00).
type ConvexRenderer struct {
	device hal.Device
	queue  hal.Queue

	// GPU objects for the render pipeline.
	shader        hal.ShaderModule
	uniformLayout hal.BindGroupLayout
	pipeLayout    hal.PipelineLayout
	pipeline      hal.RenderPipeline

	// Session-compatible pipeline variant with depth/stencil state.
	// Used when this renderer participates in a unified render pass that
	// includes a stencil attachment (for stencil-then-cover paths).
	// The stencil test is Always/Keep (convex draws don't interact with stencil).
	pipelineWithStencil hal.RenderPipeline
}

// NewConvexRenderer creates a new convex polygon renderer with the given
// device and queue. Pipelines are not created until ensurePipeline or
// ensurePipelineWithStencil is called.
func NewConvexRenderer(device hal.Device, queue hal.Queue) *ConvexRenderer {
	return &ConvexRenderer{
		device: device,
		queue:  queue,
	}
}

// Destroy releases all GPU resources held by the renderer. Safe to call
// multiple times or on a renderer with no allocated resources.
func (cr *ConvexRenderer) Destroy() {
	cr.destroyPipeline()
}

// ensurePipeline creates the shader, layouts, and standalone render pipeline
// if they don't already exist.
func (cr *ConvexRenderer) ensurePipeline() error {
	if cr.pipeline != nil {
		return nil
	}
	return cr.createPipeline()
}

// ensurePipelineWithStencil creates the session-compatible pipeline variant
// that includes a depth/stencil state. The convex pipeline ignores the stencil
// buffer (Compare=Always, all ops=Keep, write mask=0).
//
// The base pipeline (shader, layout) is created first if it doesn't exist.
func (cr *ConvexRenderer) ensurePipelineWithStencil() error { //nolint:dupl // GPU pipeline descriptors share structure but differ in labels, shaders, and vertex layouts
	// Ensure base resources exist (shader, layouts).
	if cr.shader == nil || cr.uniformLayout == nil || cr.pipeLayout == nil {
		if err := cr.createPipeline(); err != nil {
			return err
		}
	}
	if cr.pipelineWithStencil != nil {
		return nil
	}

	premulBlend := gputypes.BlendStatePremultiplied()
	pipeline, err := cr.device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label:  "convex_pipeline_with_stencil",
		Layout: cr.pipeLayout,
		Vertex: hal.VertexState{
			Module:     cr.shader,
			EntryPoint: "vs_main",
			Buffers:    convexVertexLayout(),
		},
		Fragment: &hal.FragmentState{
			Module:     cr.shader,
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
		return fmt.Errorf("create convex pipeline with stencil: %w", err)
	}
	cr.pipelineWithStencil = pipeline
	return nil
}

// RecordDraws records convex polygon draw commands into an existing render pass.
// The render pass is owned by GPURenderSession. This method uses the
// pipelineWithStencil variant because the session's render pass includes a
// depth/stencil attachment.
//
// The resources parameter holds pre-built vertex buffer, uniform buffer, and
// bind group for the current frame. This is a no-op if resources is nil.
func (cr *ConvexRenderer) RecordDraws(rp hal.RenderPassEncoder, resources *convexFrameResources) {
	if resources == nil || resources.vertCount == 0 {
		return
	}
	rp.SetPipeline(cr.pipelineWithStencil)
	rp.SetBindGroup(0, resources.bindGroup, nil)
	rp.SetVertexBuffer(0, resources.vertBuf, 0)
	rp.Draw(resources.vertCount, 1, 0, 0)
}

// createPipeline compiles the convex render shader and creates the render
// pipeline with premultiplied alpha blending and MSAA.
func (cr *ConvexRenderer) createPipeline() error { //nolint:dupl // GPU pipeline descriptors share structure but differ in labels, shaders, and vertex layouts
	if convexShaderSource == "" {
		return fmt.Errorf("convex shader source is empty")
	}

	shader, err := cr.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "convex_shader",
		Source: hal.ShaderSource{WGSL: convexShaderSource},
	})
	if err != nil {
		return fmt.Errorf("compile convex shader: %w", err)
	}
	cr.shader = shader

	uniformLayout, err := cr.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "convex_uniform_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create convex uniform layout: %w", err)
	}
	cr.uniformLayout = uniformLayout

	pipeLayout, err := cr.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "convex_pipe_layout",
		BindGroupLayouts: []hal.BindGroupLayout{cr.uniformLayout},
	})
	if err != nil {
		return fmt.Errorf("create convex pipeline layout: %w", err)
	}
	cr.pipeLayout = pipeLayout

	premulBlend := gputypes.BlendStatePremultiplied()
	pipeline, err := cr.device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label:  "convex_pipeline",
		Layout: cr.pipeLayout,
		Vertex: hal.VertexState{
			Module:     cr.shader,
			EntryPoint: "vs_main",
			Buffers:    convexVertexLayout(),
		},
		Fragment: &hal.FragmentState{
			Module:     cr.shader,
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
		return fmt.Errorf("create convex pipeline: %w", err)
	}
	cr.pipeline = pipeline

	return nil
}

// destroyPipeline releases all pipeline resources in reverse creation order.
func (cr *ConvexRenderer) destroyPipeline() {
	if cr.device == nil {
		return
	}
	if cr.pipelineWithStencil != nil {
		cr.device.DestroyRenderPipeline(cr.pipelineWithStencil)
		cr.pipelineWithStencil = nil
	}
	if cr.pipeline != nil {
		cr.device.DestroyRenderPipeline(cr.pipeline)
		cr.pipeline = nil
	}
	if cr.pipeLayout != nil {
		cr.device.DestroyPipelineLayout(cr.pipeLayout)
		cr.pipeLayout = nil
	}
	if cr.uniformLayout != nil {
		cr.device.DestroyBindGroupLayout(cr.uniformLayout)
		cr.uniformLayout = nil
	}
	if cr.shader != nil {
		cr.device.DestroyShaderModule(cr.shader)
		cr.shader = nil
	}
}

// convexFrameResources holds per-frame GPU resources for convex rendering.
type convexFrameResources struct {
	vertBuf    hal.Buffer
	uniformBuf hal.Buffer
	bindGroup  hal.BindGroup
	vertCount  uint32
}

func (r *convexFrameResources) destroy(device hal.Device) {
	if r.bindGroup != nil {
		device.DestroyBindGroup(r.bindGroup)
	}
	if r.uniformBuf != nil {
		device.DestroyBuffer(r.uniformBuf)
	}
	if r.vertBuf != nil {
		device.DestroyBuffer(r.vertBuf)
	}
}

// convexVertexLayout returns the vertex buffer layout for the convex pipeline.
func convexVertexLayout() []gputypes.VertexBufferLayout {
	return []gputypes.VertexBufferLayout{
		{
			ArrayStride: convexVertexStride,
			StepMode:    gputypes.VertexStepModeVertex,
			Attributes: []gputypes.VertexAttribute{
				{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0},  // position
				{Format: gputypes.VertexFormatFloat32, Offset: 8, ShaderLocation: 1},    // coverage
				{Format: gputypes.VertexFormatFloat32x4, Offset: 12, ShaderLocation: 2}, // color
			},
		},
	}
}

// BuildConvexVertices generates vertex data for all convex polygon draw commands.
// For each polygon, interior fan triangles (coverage=1.0) are generated from
// the centroid, followed by AA fringe strips (coverage ramping 1.0 to 0.0)
// along each edge.
//
// Each polygon with N edges produces:
//   - N interior triangles (3N vertices)
//   - N AA fringe quads = 2N fringe triangles (6N vertices)
//   - Total: 9N vertices per polygon
func BuildConvexVertices(commands []ConvexDrawCommand) []byte {
	_, data := buildConvexVerticesReuse(commands, nil)
	return data
}

// buildConvexVerticesReuse generates vertex data into the provided staging
// buffer, growing it if necessary. Returns the (possibly reallocated) staging
// buffer and the slice of valid vertex data.
func buildConvexVerticesReuse(commands []ConvexDrawCommand, staging []byte) ([]byte, []byte) { //nolint:funlen // vertex generation loop is a single cohesive unit
	totalVerts := 0
	for i := range commands {
		n := len(commands[i].Points)
		if n < 3 {
			continue
		}
		totalVerts += n * 9
	}
	if totalVerts == 0 {
		return staging, nil
	}

	needed := totalVerts * convexVertexStride
	if cap(staging) < needed {
		staging = make([]byte, needed)
	} else {
		staging = staging[:needed]
	}
	buf := staging
	offset := 0

	for i := range commands {
		cmd := &commands[i]
		n := len(cmd.Points)
		if n < 3 {
			continue
		}

		// Compute centroid.
		var cx, cy float64
		for _, p := range cmd.Points {
			cx += p.X
			cy += p.Y
		}
		cx /= float64(n)
		cy /= float64(n)
		centroidX := float32(cx)
		centroidY := float32(cy)

		color := cmd.Color

		for j := 0; j < n; j++ {
			v0 := cmd.Points[j]
			v1 := cmd.Points[(j+1)%n]

			v0x := float32(v0.X)
			v0y := float32(v0.Y)
			v1x := float32(v1.X)
			v1y := float32(v1.Y)

			// Interior fan triangle: centroid, v0, v1.
			writeConvexVertex(buf[offset:], centroidX, centroidY, 1.0, color)
			offset += convexVertexStride
			writeConvexVertex(buf[offset:], v0x, v0y, 1.0, color)
			offset += convexVertexStride
			writeConvexVertex(buf[offset:], v1x, v1y, 1.0, color)
			offset += convexVertexStride

			// AA fringe: outward expansion along edge normal.
			// Edge direction.
			edx := v1x - v0x
			edy := v1y - v0y
			edgeLen := float32(math.Sqrt(float64(edx*edx + edy*edy)))
			if edgeLen < 1e-8 {
				// Degenerate edge; emit degenerate fringe triangles.
				writeConvexVertex(buf[offset:], v0x, v0y, 1.0, color)
				offset += convexVertexStride
				writeConvexVertex(buf[offset:], v1x, v1y, 1.0, color)
				offset += convexVertexStride
				writeConvexVertex(buf[offset:], v0x, v0y, 0.0, color)
				offset += convexVertexStride

				writeConvexVertex(buf[offset:], v1x, v1y, 1.0, color)
				offset += convexVertexStride
				writeConvexVertex(buf[offset:], v1x, v1y, 0.0, color)
				offset += convexVertexStride
				writeConvexVertex(buf[offset:], v0x, v0y, 0.0, color)
				offset += convexVertexStride
				continue
			}

			// Outward normal (perpendicular to edge, pointing outward).
			// For a CCW polygon, the outward normal of edge (dx, dy) is (dy, -dx).
			// For CW, it would be (-dy, dx). We use the centroid to determine direction.
			nx := edy / edgeLen
			ny := -edx / edgeLen

			// Ensure normal points outward (away from centroid).
			// Midpoint of edge.
			midX := (v0x + v1x) * 0.5
			midY := (v0y + v1y) * 0.5
			// Vector from centroid to midpoint.
			toCentroidX := midX - centroidX
			toCentroidY := midY - centroidY
			// Dot product: if normal points toward centroid, flip it.
			if nx*toCentroidX+ny*toCentroidY < 0 {
				nx = -nx
				ny = -ny
			}

			// Expanded vertices (0.5px outward).
			expand := float32(convexAAExpand)
			v0ox := v0x + nx*expand
			v0oy := v0y + ny*expand
			v1ox := v1x + nx*expand
			v1oy := v1y + ny*expand

			// Fringe quad: two triangles.
			// Triangle 1: v0, v1, v0_outer.
			writeConvexVertex(buf[offset:], v0x, v0y, 1.0, color)
			offset += convexVertexStride
			writeConvexVertex(buf[offset:], v1x, v1y, 1.0, color)
			offset += convexVertexStride
			writeConvexVertex(buf[offset:], v0ox, v0oy, 0.0, color)
			offset += convexVertexStride

			// Triangle 2: v1, v1_outer, v0_outer.
			writeConvexVertex(buf[offset:], v1x, v1y, 1.0, color)
			offset += convexVertexStride
			writeConvexVertex(buf[offset:], v1ox, v1oy, 0.0, color)
			offset += convexVertexStride
			writeConvexVertex(buf[offset:], v0ox, v0oy, 0.0, color)
			offset += convexVertexStride
		}
	}

	return staging, buf[:offset]
}

// writeConvexVertex writes a single convex vertex into the buffer.
// Layout: position (vec2<f32>) + coverage (f32) + color (vec4<f32>) = 28 bytes.
func writeConvexVertex(buf []byte, px, py, coverage float32, color [4]float32) {
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(px))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(py))
	binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(coverage))
	binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(color[0]))
	binary.LittleEndian.PutUint32(buf[16:20], math.Float32bits(color[1]))
	binary.LittleEndian.PutUint32(buf[20:24], math.Float32bits(color[2]))
	binary.LittleEndian.PutUint32(buf[24:28], math.Float32bits(color[3]))
}

// convexVertexCount returns the total vertex count for the given commands.
func convexVertexCount(commands []ConvexDrawCommand) uint32 {
	var total uint32
	for i := range commands {
		n := len(commands[i].Points)
		if n < 3 {
			continue
		}
		total += uint32(n) * 9 //nolint:gosec // polygon vertex count fits uint32
	}
	return total
}
