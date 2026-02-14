//go:build !nogpu

package gpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// textureSet holds a set of MSAA color, depth/stencil, and resolve textures
// for offscreen rendering. This is shared by GPURenderSession and
// StencilRenderer to avoid code duplication.
//
// The texture set supports stencil-then-cover and SDF rendering:
//   - MSAA color: 4x samples, BGRA8Unorm, RenderAttachment
//   - Depth/stencil: 4x samples, Depth24PlusStencil8, RenderAttachment
//   - Resolve: 1x sample, BGRA8Unorm, RenderAttachment | CopySrc
type textureSet struct {
	msaaTex     hal.Texture
	msaaView    hal.TextureView
	stencilTex  hal.Texture
	stencilView hal.TextureView
	resolveTex  hal.Texture
	resolveView hal.TextureView
	width       uint32
	height      uint32
}

// ensureTextures creates or recreates textures if the requested dimensions
// differ from the current size. If dimensions match and textures exist,
// this is a no-op. The labelPrefix parameter distinguishes GPU debug labels
// between different owners (e.g., "session" vs "stencil").
func (ts *textureSet) ensureTextures(device hal.Device, w, h uint32, labelPrefix string) error {
	if ts.width == w && ts.height == h && ts.msaaTex != nil {
		return nil
	}
	ts.destroyTextures(device)

	size := hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1}

	// MSAA color texture (4x samples, BGRA8Unorm).
	msaaTex, err := device.CreateTexture(&hal.TextureDescriptor{
		Label:         labelPrefix + "_msaa_color",
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
	ts.msaaTex = msaaTex

	msaaView, err := device.CreateTextureView(msaaTex, &hal.TextureViewDescriptor{
		Label: labelPrefix + "_msaa_color_view",
	})
	if err != nil {
		ts.destroyTextures(device)
		return fmt.Errorf("create MSAA color view: %w", err)
	}
	ts.msaaView = msaaView

	// Depth/stencil texture (4x samples, Depth24PlusStencil8).
	stencilTex, err := device.CreateTexture(&hal.TextureDescriptor{
		Label:         labelPrefix + "_depth_stencil",
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   sampleCount,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatDepth24PlusStencil8,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		ts.destroyTextures(device)
		return fmt.Errorf("create depth/stencil texture: %w", err)
	}
	ts.stencilTex = stencilTex

	stencilView, err := device.CreateTextureView(stencilTex, &hal.TextureViewDescriptor{
		Label: labelPrefix + "_depth_stencil_view",
	})
	if err != nil {
		ts.destroyTextures(device)
		return fmt.Errorf("create depth/stencil view: %w", err)
	}
	ts.stencilView = stencilView

	// Single-sample resolve target (CopySrc for readback).
	resolveTex, err := device.CreateTexture(&hal.TextureDescriptor{
		Label:         labelPrefix + "_resolve",
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		ts.destroyTextures(device)
		return fmt.Errorf("create resolve texture: %w", err)
	}
	ts.resolveTex = resolveTex

	resolveView, err := device.CreateTextureView(resolveTex, &hal.TextureViewDescriptor{
		Label: labelPrefix + "_resolve_view",
	})
	if err != nil {
		ts.destroyTextures(device)
		return fmt.Errorf("create resolve view: %w", err)
	}
	ts.resolveView = resolveView

	ts.width = w
	ts.height = h
	return nil
}

// ensureSurfaceTextures creates or recreates only the MSAA color and
// depth/stencil textures for surface rendering mode. The resolve texture
// is NOT created because the caller-provided surface view serves as the
// MSAA resolve target.
//
// If a resolve texture exists from a previous offscreen mode, it is destroyed.
// If dimensions match and textures exist, this is a no-op.
func (ts *textureSet) ensureSurfaceTextures(device hal.Device, w, h uint32, labelPrefix string) error {
	if ts.width == w && ts.height == h && ts.msaaTex != nil {
		return nil
	}
	ts.destroyTextures(device)

	size := hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1}

	// MSAA color texture (4x samples, BGRA8Unorm).
	msaaTex, err := device.CreateTexture(&hal.TextureDescriptor{
		Label:         labelPrefix + "_msaa_color",
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
	ts.msaaTex = msaaTex

	msaaView, err := device.CreateTextureView(msaaTex, &hal.TextureViewDescriptor{
		Label: labelPrefix + "_msaa_color_view",
	})
	if err != nil {
		ts.destroyTextures(device)
		return fmt.Errorf("create MSAA color view: %w", err)
	}
	ts.msaaView = msaaView

	// Depth/stencil texture (4x samples, Depth24PlusStencil8).
	stencilTex, err := device.CreateTexture(&hal.TextureDescriptor{
		Label:         labelPrefix + "_depth_stencil",
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   sampleCount,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatDepth24PlusStencil8,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		ts.destroyTextures(device)
		return fmt.Errorf("create depth/stencil texture: %w", err)
	}
	ts.stencilTex = stencilTex

	stencilView, err := device.CreateTextureView(stencilTex, &hal.TextureViewDescriptor{
		Label: labelPrefix + "_depth_stencil_view",
	})
	if err != nil {
		ts.destroyTextures(device)
		return fmt.Errorf("create depth/stencil view: %w", err)
	}
	ts.stencilView = stencilView

	// No resolve texture -- surface view is the resolve target.
	ts.width = w
	ts.height = h
	return nil
}

// destroyTextures releases all texture resources and resets dimensions.
func (ts *textureSet) destroyTextures(device hal.Device) {
	if ts.resolveView != nil {
		device.DestroyTextureView(ts.resolveView)
		ts.resolveView = nil
	}
	if ts.resolveTex != nil {
		device.DestroyTexture(ts.resolveTex)
		ts.resolveTex = nil
	}
	if ts.stencilView != nil {
		device.DestroyTextureView(ts.stencilView)
		ts.stencilView = nil
	}
	if ts.stencilTex != nil {
		device.DestroyTexture(ts.stencilTex)
		ts.stencilTex = nil
	}
	if ts.msaaView != nil {
		device.DestroyTextureView(ts.msaaView)
		ts.msaaView = nil
	}
	if ts.msaaTex != nil {
		device.DestroyTexture(ts.msaaTex)
		ts.msaaTex = nil
	}
	ts.width = 0
	ts.height = 0
}
