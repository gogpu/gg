//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// sampleCount is the MSAA sample count used for stencil-then-cover rendering.
// 4x MSAA provides good quality antialiasing at reasonable cost.
const sampleCount = 4

// StencilRenderer manages GPU resources for stencil-then-cover path rendering.
// It creates and maintains MSAA color, stencil, and resolve textures that are
// resized automatically when the surface dimensions change.
//
// The stencil-then-cover algorithm (OpenGL Red Book / NV_path_rendering / Skia Ganesh)
// renders arbitrary paths in two passes within a single render pass:
//
//	Pass 1 (Stencil Fill): Triangle fan from an anchor point fills the stencil buffer
//	  with winding number increments. Non-zero or even-odd rules determine inside/outside.
//	Pass 2 (Cover): A bounding quad reads the stencil buffer and writes the fill color
//	  to the MSAA color attachment for pixels with non-zero stencil values.
//
// Both passes execute in a single render pass via pipeline switching. The MSAA color
// attachment resolves to the single-sample resolve texture, which can be read back
// to the CPU via CopySrc usage.
type StencilRenderer struct {
	device hal.Device
	queue  hal.Queue

	// msaaTex is the MSAA color texture (4x samples, BGRA8Unorm).
	// Used as the main render target during stencil-then-cover passes.
	msaaTex  hal.Texture
	msaaView hal.TextureView

	// stencilTex is the depth/stencil texture (4x samples, Depth24PlusStencil8).
	// The stencil component stores winding numbers during the stencil fill pass.
	// The depth component is unused but required by the format.
	stencilTex  hal.Texture
	stencilView hal.TextureView

	// resolveTex is the single-sample resolve target (1x sample, BGRA8Unorm, CopySrc).
	// MSAA color data is resolved here at the end of the render pass.
	// CopySrc usage enables GPU-to-CPU readback for software compositing.
	resolveTex  hal.Texture
	resolveView hal.TextureView

	width, height uint32

	// Shader modules for stencil-then-cover rendering.
	stencilFillShader hal.ShaderModule
	coverShader       hal.ShaderModule

	// Bind group layout and pipeline layouts shared by both passes.
	uniformLayout     hal.BindGroupLayout
	stencilPipeLayout hal.PipelineLayout
	coverPipeLayout   hal.PipelineLayout

	// Render pipelines.
	// nonZeroStencilPipeline implements the non-zero winding fill rule:
	// front faces increment stencil, back faces decrement.
	nonZeroStencilPipeline hal.RenderPipeline

	// evenOddStencilPipeline implements the even-odd fill rule:
	// both front and back faces invert the stencil value.
	evenOddStencilPipeline hal.RenderPipeline

	// nonZeroCoverPipeline draws the fill color where stencil != 0,
	// then resets stencil to zero via PassOp. Shared by both fill rules.
	nonZeroCoverPipeline hal.RenderPipeline
}

// NewStencilRenderer creates a new StencilRenderer with the given device and queue.
// Textures are not allocated until EnsureTextures is called with the desired dimensions.
func NewStencilRenderer(device hal.Device, queue hal.Queue) *StencilRenderer {
	return &StencilRenderer{
		device: device,
		queue:  queue,
	}
}

// EnsureTextures creates or recreates the MSAA color, stencil, and resolve textures
// if the requested dimensions differ from the current size. If dimensions match the
// current textures, this is a no-op.
//
// On resize, existing textures are destroyed before creating new ones to avoid
// GPU memory leaks. Returns an error if any texture or view creation fails;
// in that case, partially created resources are cleaned up.
func (sr *StencilRenderer) EnsureTextures(width, height uint32) error {
	if sr.width == width && sr.height == height && sr.msaaTex != nil {
		return nil
	}

	// Destroy old textures if they exist (resize path).
	sr.destroyTextures()

	size := hal.Extent3D{
		Width:              width,
		Height:             height,
		DepthOrArrayLayers: 1,
	}

	// Create MSAA color texture (4x samples, BGRA8Unorm).
	msaaTex, err := sr.device.CreateTexture(&hal.TextureDescriptor{
		Label:         "stencil_msaa_color",
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   sampleCount,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		return fmt.Errorf("create MSAA color texture: %w", err)
	}
	sr.msaaTex = msaaTex

	msaaView, err := sr.device.CreateTextureView(msaaTex, &hal.TextureViewDescriptor{
		Label: "stencil_msaa_color_view",
	})
	if err != nil {
		sr.destroyTextures()
		return fmt.Errorf("create MSAA color texture view: %w", err)
	}
	sr.msaaView = msaaView

	// Create depth/stencil texture (4x samples, Depth24PlusStencil8).
	stencilTex, err := sr.device.CreateTexture(&hal.TextureDescriptor{
		Label:         "stencil_depth_stencil",
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   sampleCount,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatDepth24PlusStencil8,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		sr.destroyTextures()
		return fmt.Errorf("create depth/stencil texture: %w", err)
	}
	sr.stencilTex = stencilTex

	stencilView, err := sr.device.CreateTextureView(stencilTex, &hal.TextureViewDescriptor{
		Label: "stencil_depth_stencil_view",
	})
	if err != nil {
		sr.destroyTextures()
		return fmt.Errorf("create depth/stencil texture view: %w", err)
	}
	sr.stencilView = stencilView

	// Create resolve target (1x sample, BGRA8Unorm, CopySrc for readback).
	resolveTex, err := sr.device.CreateTexture(&hal.TextureDescriptor{
		Label:         "stencil_resolve",
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		sr.destroyTextures()
		return fmt.Errorf("create resolve texture: %w", err)
	}
	sr.resolveTex = resolveTex

	resolveView, err := sr.device.CreateTextureView(resolveTex, &hal.TextureViewDescriptor{
		Label: "stencil_resolve_view",
	})
	if err != nil {
		sr.destroyTextures()
		return fmt.Errorf("create resolve texture view: %w", err)
	}
	sr.resolveView = resolveView

	sr.width = width
	sr.height = height
	return nil
}

// Destroy releases all GPU resources held by the renderer: pipelines, shaders,
// layouts, textures, and views. Safe to call multiple times or on a renderer
// with no allocated resources. After Destroy, createPipelines and EnsureTextures
// must be called again before rendering.
func (sr *StencilRenderer) Destroy() {
	sr.destroyPipelines()
	sr.destroyTextures()
}

// destroyTextures releases all texture views and textures, resetting dimensions to zero.
// Each resource is nil-checked before destruction to support partial cleanup.
func (sr *StencilRenderer) destroyTextures() {
	if sr.resolveView != nil {
		sr.device.DestroyTextureView(sr.resolveView)
		sr.resolveView = nil
	}
	if sr.resolveTex != nil {
		sr.device.DestroyTexture(sr.resolveTex)
		sr.resolveTex = nil
	}
	if sr.stencilView != nil {
		sr.device.DestroyTextureView(sr.stencilView)
		sr.stencilView = nil
	}
	if sr.stencilTex != nil {
		sr.device.DestroyTexture(sr.stencilTex)
		sr.stencilTex = nil
	}
	if sr.msaaView != nil {
		sr.device.DestroyTextureView(sr.msaaView)
		sr.msaaView = nil
	}
	if sr.msaaTex != nil {
		sr.device.DestroyTexture(sr.msaaTex)
		sr.msaaTex = nil
	}
	sr.width = 0
	sr.height = 0
}

// RenderPassDescriptor returns a configured render pass descriptor for
// stencil-then-cover rendering. The descriptor sets up:
//
//   - Color attachment: MSAA color texture with resolve to single-sample target.
//     LoadOp is Clear (transparent black) to start with a clean canvas.
//     StoreOp is Store to trigger MSAA resolve.
//
//   - Depth/stencil attachment: Stencil cleared to 0 at pass start, discarded at
//     pass end (stencil data is transient within the pass). Depth is unused but
//     must be configured; it is cleared to 1.0 and discarded.
//
// EnsureTextures must be called before this method. Returns nil if textures
// have not been allocated.
func (sr *StencilRenderer) RenderPassDescriptor() *hal.RenderPassDescriptor {
	if sr.msaaView == nil || sr.stencilView == nil || sr.resolveView == nil {
		return nil
	}
	return &hal.RenderPassDescriptor{
		Label: "stencil_cover_pass",
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:          sr.msaaView,
				ResolveTarget: sr.resolveView,
				LoadOp:        gputypes.LoadOpClear,
				StoreOp:       gputypes.StoreOpStore,
				ClearValue:    gputypes.Color{R: 0, G: 0, B: 0, A: 0},
			},
		},
		DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{
			View:              sr.stencilView,
			DepthLoadOp:       gputypes.LoadOpClear,
			DepthStoreOp:      gputypes.StoreOpDiscard,
			DepthClearValue:   1.0,
			StencilLoadOp:     gputypes.LoadOpClear,
			StencilStoreOp:    gputypes.StoreOpDiscard,
			StencilClearValue: 0,
		},
	}
}

// ResolveTexture returns the single-sample resolve target texture.
// This texture contains the final rendered output after MSAA resolve and
// has CopySrc usage for GPU-to-CPU readback.
// Returns nil if textures have not been allocated via EnsureTextures.
func (sr *StencilRenderer) ResolveTexture() hal.Texture {
	return sr.resolveTex
}

// Size returns the current texture dimensions. Returns (0, 0) if textures
// have not been allocated.
func (sr *StencilRenderer) Size() (uint32, uint32) {
	return sr.width, sr.height
}

// stencilCoverBuffers holds all GPU buffers and bind groups for a single
// stencil-then-cover render pass. Created by createRenderBuffers and cleaned
// up via destroy.
type stencilCoverBuffers struct {
	fanVertBuf       hal.Buffer
	coverVertBuf     hal.Buffer
	stencilUniBuf    hal.Buffer
	coverUniBuf      hal.Buffer
	stencilBindGroup hal.BindGroup
	coverBindGroup   hal.BindGroup
	fanVertexCount   uint32
}

// destroy releases all GPU resources.
func (b *stencilCoverBuffers) destroy(device hal.Device) {
	if b.coverBindGroup != nil {
		device.DestroyBindGroup(b.coverBindGroup)
	}
	if b.stencilBindGroup != nil {
		device.DestroyBindGroup(b.stencilBindGroup)
	}
	if b.coverUniBuf != nil {
		device.DestroyBuffer(b.coverUniBuf)
	}
	if b.stencilUniBuf != nil {
		device.DestroyBuffer(b.stencilUniBuf)
	}
	if b.coverVertBuf != nil {
		device.DestroyBuffer(b.coverVertBuf)
	}
	if b.fanVertBuf != nil {
		device.DestroyBuffer(b.fanVertBuf)
	}
}

// RenderPath renders a filled path using the stencil-then-cover algorithm.
//
// The algorithm works in two passes within a single render pass:
//
//  1. Stencil fill: Tessellate the path into triangle fan vertices and draw them
//     with the stencil fill pipeline. The fill rule determines stencil operations:
//     - NonZero: front faces increment, back faces decrement (winding number).
//     - EvenOdd: both faces invert the stencil value (parity count).
//
//  2. Cover: Draw a bounding quad with the cover pipeline. Only pixels with
//     non-zero stencil values pass the stencil test and receive the fill color.
//     PassOp=Zero resets the stencil buffer for subsequent paths.
//
// After the render pass, the MSAA color attachment resolves to the single-sample
// resolve texture. The resolve texture is then copied to a staging buffer for
// CPU readback. Pixel data is converted from BGRA (GPU format) to RGBA and
// written to target.Data.
//
// Returns nil for empty paths (no triangles after tessellation).
func (sr *StencilRenderer) RenderPath(target gg.GPURenderTarget, elements []gg.PathElement, color gg.RGBA, fillRule gg.FillRule) error {
	w, h := uint32(target.Width), uint32(target.Height) //nolint:gosec // dimensions always fit uint32

	if err := sr.ensureReady(w, h); err != nil {
		return err
	}

	// Tessellate path into fan triangles.
	tess := NewFanTessellator()
	tess.TessellatePath(elements)
	fanVerts := tess.Vertices()
	if len(fanVerts) == 0 {
		return nil // empty path, nothing to render
	}

	// Create GPU buffers and bind groups for the render pass.
	bufs, err := sr.createRenderBuffers(w, h, fanVerts, tess.CoverQuad(), color)
	if err != nil {
		return err
	}
	defer bufs.destroy(sr.device)

	// Encode, submit, and read back pixels.
	return sr.encodeAndReadback(w, h, bufs, target, fillRule)
}

// ensureReady ensures textures and pipelines are created for the given dimensions.
func (sr *StencilRenderer) ensureReady(w, h uint32) error {
	if err := sr.EnsureTextures(w, h); err != nil {
		return fmt.Errorf("ensure textures: %w", err)
	}
	if sr.nonZeroStencilPipeline == nil {
		if err := sr.createPipelines(); err != nil {
			return fmt.Errorf("create pipelines: %w", err)
		}
	}
	return nil
}

// createRenderBuffers creates vertex buffers, uniform buffers, and bind groups
// for a stencil-then-cover render pass.
func (sr *StencilRenderer) createRenderBuffers(
	w, h uint32, fanVerts []float32, coverQuad [12]float32, color gg.RGBA,
) (*stencilCoverBuffers, error) {
	b := &stencilCoverBuffers{
		fanVertexCount: uint32(len(fanVerts) / 2), //nolint:gosec // len/2 fits uint32
	}

	var err error

	// Vertex buffers.
	b.fanVertBuf, err = sr.createAndUploadVertexBuffer("stencil_fan_verts", float32SliceToBytes(fanVerts))
	if err != nil {
		b.destroy(sr.device)
		return nil, err
	}
	b.coverVertBuf, err = sr.createAndUploadVertexBuffer("stencil_cover_verts", float32SliceToBytes(coverQuad[:]))
	if err != nil {
		b.destroy(sr.device)
		return nil, err
	}

	// Uniform buffers + bind groups.
	b.stencilUniBuf, b.stencilBindGroup, err = sr.createUniformAndBindGroup(
		"stencil_fill", makeStencilFillUniform(w, h), stencilFillUniformSize,
	)
	if err != nil {
		b.destroy(sr.device)
		return nil, err
	}
	b.coverUniBuf, b.coverBindGroup, err = sr.createUniformAndBindGroup(
		"cover", makeCoverUniform(w, h, color), coverUniformSize,
	)
	if err != nil {
		b.destroy(sr.device)
		return nil, err
	}

	return b, nil
}

// createAndUploadVertexBuffer creates a vertex buffer and uploads data.
func (sr *StencilRenderer) createAndUploadVertexBuffer(label string, data []byte) (hal.Buffer, error) {
	buf, err := sr.device.CreateBuffer(&hal.BufferDescriptor{
		Label: label, Size: uint64(len(data)),
		Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("create %s buffer: %w", label, err)
	}
	sr.queue.WriteBuffer(buf, 0, data)
	return buf, nil
}

// createUniformAndBindGroup creates a uniform buffer, uploads data, and creates
// a bind group with a single buffer binding at group(0) binding(0).
func (sr *StencilRenderer) createUniformAndBindGroup(
	label string, data []byte, size uint64,
) (hal.Buffer, hal.BindGroup, error) {
	buf, err := sr.device.CreateBuffer(&hal.BufferDescriptor{
		Label: label + "_uniform", Size: size,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create %s uniform: %w", label, err)
	}
	sr.queue.WriteBuffer(buf, 0, data)

	bg, err := sr.device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label: label + "_bind", Layout: sr.uniformLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{
				Buffer: buf.NativeHandle(), Offset: 0, Size: size,
			}},
		},
	})
	if err != nil {
		sr.device.DestroyBuffer(buf)
		return nil, nil, fmt.Errorf("create %s bind group: %w", label, err)
	}
	return buf, bg, nil
}

// encodeAndReadback encodes the stencil-then-cover render pass, copies the
// resolve texture to a staging buffer, submits GPU commands, waits for
// completion, and writes pixel data to the target.
func (sr *StencilRenderer) encodeAndReadback(
	w, h uint32, bufs *stencilCoverBuffers, target gg.GPURenderTarget, fillRule gg.FillRule,
) error {
	encoder, err := sr.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "stencil_cover_encoder",
	})
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	if err := encoder.BeginEncoding("stencil_cover"); err != nil {
		return fmt.Errorf("begin encoding: %w", err)
	}

	// Render pass: stencil fill + cover in a single pass.
	rp := encoder.BeginRenderPass(sr.RenderPassDescriptor())

	// Select stencil pipeline based on fill rule.
	stencilPipeline := sr.nonZeroStencilPipeline
	if fillRule == gg.FillRuleEvenOdd {
		stencilPipeline = sr.evenOddStencilPipeline
	}

	rp.SetPipeline(stencilPipeline)
	rp.SetBindGroup(0, bufs.stencilBindGroup, nil)
	rp.SetVertexBuffer(0, bufs.fanVertBuf, 0)
	rp.Draw(bufs.fanVertexCount, 1, 0, 0)

	rp.SetPipeline(sr.nonZeroCoverPipeline)
	rp.SetBindGroup(0, bufs.coverBindGroup, nil)
	rp.SetVertexBuffer(0, bufs.coverVertBuf, 0)
	rp.SetStencilReference(0)
	rp.Draw(6, 1, 0, 0)

	rp.End()

	// Copy resolve texture to staging buffer for CPU readback.
	pixelBufSize := uint64(w) * uint64(h) * 4
	stagingBuf, err := sr.device.CreateBuffer(&hal.BufferDescriptor{
		Label: "stencil_staging", Size: pixelBufSize,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		encoder.DiscardEncoding()
		return fmt.Errorf("create staging buffer: %w", err)
	}
	defer sr.device.DestroyBuffer(stagingBuf)

	encoder.CopyTextureToBuffer(sr.resolveTex, stagingBuf, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{Offset: 0, BytesPerRow: w * 4, RowsPerImage: h},
		TextureBase:  hal.ImageCopyTexture{Texture: sr.resolveTex, MipLevel: 0},
		Size:         hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
	}})

	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("end encoding: %w", err)
	}
	defer sr.device.FreeCommandBuffer(cmdBuf)

	return sr.submitAndReadback(cmdBuf, stagingBuf, pixelBufSize, target)
}

// submitAndReadback submits the command buffer, waits for GPU completion,
// reads back pixel data, and converts BGRA to RGBA into the target buffer.
func (sr *StencilRenderer) submitAndReadback(
	cmdBuf hal.CommandBuffer, stagingBuf hal.Buffer,
	pixelBufSize uint64, target gg.GPURenderTarget,
) error {
	fence, err := sr.device.CreateFence()
	if err != nil {
		return fmt.Errorf("create fence: %w", err)
	}
	defer sr.device.DestroyFence(fence)

	if err := sr.queue.Submit([]hal.CommandBuffer{cmdBuf}, fence, 1); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	fenceOK, err := sr.device.Wait(fence, 1, 5*time.Second)
	if err != nil || !fenceOK {
		return fmt.Errorf("wait for GPU: ok=%v err=%w", fenceOK, err)
	}

	readback := make([]byte, pixelBufSize)
	if err := sr.queue.ReadBuffer(stagingBuf, 0, readback); err != nil {
		return fmt.Errorf("readback: %w", err)
	}

	convertBGRAToRGBA(readback, target.Data, target.Width*target.Height)
	return nil
}

// makeStencilFillUniform creates the 16-byte uniform buffer for the stencil fill pass.
// Layout: viewport (vec2<f32>) + padding (vec2<f32>).
func makeStencilFillUniform(w, h uint32) []byte {
	buf := make([]byte, stencilFillUniformSize)
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(float32(w)))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(float32(h)))
	// padding bytes 8..15 are zero
	return buf
}

// makeCoverUniform creates the 32-byte uniform buffer for the cover pass.
// Layout: viewport (vec2<f32>) + padding (vec2<f32>) + color (vec4<f32>, premultiplied alpha).
func makeCoverUniform(w, h uint32, color gg.RGBA) []byte {
	buf := make([]byte, coverUniformSize)
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(float32(w)))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(float32(h)))
	// padding bytes 8..15 are zero

	// Premultiply alpha for GPU blending.
	premulR := float32(color.R * color.A)
	premulG := float32(color.G * color.A)
	premulB := float32(color.B * color.A)
	premulA := float32(color.A)
	binary.LittleEndian.PutUint32(buf[16:20], math.Float32bits(premulR))
	binary.LittleEndian.PutUint32(buf[20:24], math.Float32bits(premulG))
	binary.LittleEndian.PutUint32(buf[24:28], math.Float32bits(premulB))
	binary.LittleEndian.PutUint32(buf[28:32], math.Float32bits(premulA))
	return buf
}

// float32SliceToBytes converts a float32 slice to a byte slice without copying.
func float32SliceToBytes(s []float32) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*4) //nolint:gosec // safe float32 to bytes
}

// convertBGRAToRGBA converts pixel data from BGRA (GPU texture format) to RGBA.
// Both src and dst must have at least pixelCount*4 bytes.
func convertBGRAToRGBA(src, dst []byte, pixelCount int) {
	for i := 0; i < pixelCount; i++ {
		off := i * 4
		b, g, r, a := src[off], src[off+1], src[off+2], src[off+3]
		dst[off] = r
		dst[off+1] = g
		dst[off+2] = b
		dst[off+3] = a
	}
}
