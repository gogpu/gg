//go:build !nogpu

package gpu

import (
	"fmt"

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

	// nonZeroCoverPipeline draws the fill color where stencil != 0,
	// then resets stencil to zero via PassOp.
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
