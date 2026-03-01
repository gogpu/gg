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

	// Shared MSAA color + depth/stencil + resolve textures.
	textures textureSet

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
	return sr.textures.ensureTextures(sr.device, width, height, "stencil")
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
func (sr *StencilRenderer) destroyTextures() {
	sr.textures.destroyTextures(sr.device)
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
	if sr.textures.msaaView == nil || sr.textures.stencilView == nil || sr.textures.resolveView == nil {
		return nil
	}
	return &hal.RenderPassDescriptor{
		Label: "stencil_cover_pass",
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:          sr.textures.msaaView,
				ResolveTarget: sr.textures.resolveView,
				LoadOp:        gputypes.LoadOpClear,
				StoreOp:       gputypes.StoreOpStore,
				ClearValue:    gputypes.Color{R: 0, G: 0, B: 0, A: 0},
			},
		},
		DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{
			View:              sr.textures.stencilView,
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
	return sr.textures.resolveTex
}

// Size returns the current texture dimensions. Returns (0, 0) if textures
// have not been allocated.
func (sr *StencilRenderer) Size() (uint32, uint32) {
	return sr.textures.width, sr.textures.height
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
	if err := sr.queue.WriteBuffer(buf, 0, data); err != nil {
		sr.device.DestroyBuffer(buf)
		return nil, fmt.Errorf("write %s buffer: %w", label, err)
	}
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
	if err := sr.queue.WriteBuffer(buf, 0, data); err != nil {
		sr.device.DestroyBuffer(buf)
		return nil, nil, fmt.Errorf("write %s uniform: %w", label, err)
	}

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

	// VK-LAYOUT-001: After MSAA resolve the texture is in
	// COLOR_ATTACHMENT_OPTIMAL layout. CopyTextureToBuffer requires
	// TRANSFER_SRC_OPTIMAL. Insert an explicit barrier to transition.
	// This is a no-op on Metal, GLES, software, and noop backends.
	encoder.TransitionTextures([]hal.TextureBarrier{{
		Texture: sr.textures.resolveTex,
		Usage: hal.TextureUsageTransition{
			OldUsage: gputypes.TextureUsageRenderAttachment,
			NewUsage: gputypes.TextureUsageCopySrc,
		},
	}})

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

	encoder.CopyTextureToBuffer(sr.textures.resolveTex, stagingBuf, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{Offset: 0, BytesPerRow: w * 4, RowsPerImage: h},
		TextureBase:  hal.ImageCopyTexture{Texture: sr.textures.resolveTex, MipLevel: 0},
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

	compositeBGRAOverRGBA(readback, target.Data, target.Width*target.Height)
	return nil
}

// RecordPath records a stencil-then-cover path into an existing render pass.
// The render pass is owned by GPURenderSession. This method performs two
// pipeline switches within the pass:
//
//  1. SetPipeline(stencilFillPipeline) + Draw(fanVertices)
//  2. SetPipeline(coverPipeline) + Draw(6)
//
// The bufs parameter holds pre-built vertex buffers, uniform buffers, and
// bind groups for the current path. The fill rule selects between non-zero
// and even-odd stencil pipelines.
func (sr *StencilRenderer) RecordPath(rp hal.RenderPassEncoder, bufs *stencilCoverBuffers, fillRule gg.FillRule) {
	// Select stencil pipeline based on fill rule.
	stencilPipeline := sr.nonZeroStencilPipeline
	if fillRule == gg.FillRuleEvenOdd {
		stencilPipeline = sr.evenOddStencilPipeline
	}

	// Pass 1: Stencil fill.
	rp.SetPipeline(stencilPipeline)
	rp.SetBindGroup(0, bufs.stencilBindGroup, nil)
	rp.SetVertexBuffer(0, bufs.fanVertBuf, 0)
	rp.Draw(bufs.fanVertexCount, 1, 0, 0)

	// Pass 2: Cover.
	rp.SetPipeline(sr.nonZeroCoverPipeline)
	rp.SetBindGroup(0, bufs.coverBindGroup, nil)
	rp.SetVertexBuffer(0, bufs.coverVertBuf, 0)
	rp.SetStencilReference(0)
	rp.Draw(6, 1, 0, 0)
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

// compositeBGRAOverRGBA composites GPU readback pixels (BGRA, premultiplied
// alpha) over existing pixmap content (RGBA, premultiplied alpha) using the
// Porter-Duff "over" operator: out = src + dst * (1 - src_alpha).
//
// Transparent source pixels (alpha=0) leave the destination unchanged. Fully
// opaque source pixels (alpha=255) replace the destination. Semi-transparent
// source pixels are blended correctly with existing content.
//
// This is essential when the GPU accelerator flushes multiple times per frame
// (e.g., before each CPU text draw). Without compositing, each flush would
// clear the pixmap, losing previously rendered content.
func compositeBGRAOverRGBA(src, dst []byte, pixelCount int) {
	for i := 0; i < pixelCount; i++ {
		off := i * 4
		sa := src[off+3]
		if sa == 0 {
			continue // transparent source: leave dst unchanged
		}
		sr, sg, sb := src[off+2], src[off+1], src[off]
		if sa == 255 {
			// Fully opaque: direct copy (BGRA→RGBA swap).
			dst[off] = sr
			dst[off+1] = sg
			dst[off+2] = sb
			dst[off+3] = 255
			continue
		}
		// Semi-transparent: premultiplied "over" compositing.
		// out_c = src_c + dst_c * (255 - src_a) / 255
		// Max value: 255 + 255*255/255 = 510, but premultiplied invariant
		// guarantees src_c <= src_a, so result fits in uint8.
		invA := uint16(255 - sa)
		dst[off] = sr + uint8(uint16(dst[off])*invA/255)     //nolint:gosec // G115: premultiplied alpha guarantees no overflow
		dst[off+1] = sg + uint8(uint16(dst[off+1])*invA/255) //nolint:gosec // G115: premultiplied alpha guarantees no overflow
		dst[off+2] = sb + uint8(uint16(dst[off+2])*invA/255) //nolint:gosec // G115: premultiplied alpha guarantees no overflow
		dst[off+3] = sa + uint8(uint16(dst[off+3])*invA/255) //nolint:gosec // G115: premultiplied alpha guarantees no overflow
	}
}
