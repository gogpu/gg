// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
)

// DeviceHandle provides GPU device access from the host application.
//
// This interface is the primary integration point between gg and GPU frameworks
// like gogpu. The host application (e.g., gogpu.App) implements DeviceHandle
// and passes it to gg renderers, allowing gg to use the shared GPU device.
//
// Key principle: gg RECEIVES the device from the host, it does NOT create one.
// This enables:
//   - Shared GPU resources between gg and the host application
//   - Zero device creation overhead in gg
//   - Consistent resource management across the stack
//
// Example implementation in gogpu:
//
//	type contextDeviceHandle struct {
//	    ctx *gogpu.Context
//	}
//
//	func (h *contextDeviceHandle) Device() gpucontext.Device {
//	    return h.ctx.device
//	}
//
//	func (h *contextDeviceHandle) Queue() gpucontext.Queue {
//	    return h.ctx.queue
//	}
//
// DeviceHandle is an alias for gpucontext.DeviceProvider, providing a
// gg-specific name for the interface while maintaining full compatibility
// with the gpucontext ecosystem.
type DeviceHandle = gpucontext.DeviceProvider

// TextureDescriptor describes parameters for creating a texture.
// This mirrors the WebGPU GPUTextureDescriptor specification.
type TextureDescriptor struct {
	// Label is an optional debug label for the texture.
	Label string

	// Width is the texture width in pixels.
	Width uint32

	// Height is the texture height in pixels.
	Height uint32

	// Depth is the texture depth for 3D textures, or array layer count.
	// Use 1 for regular 2D textures.
	Depth uint32

	// MipLevelCount is the number of mipmap levels.
	// Use 1 for no mipmaps.
	MipLevelCount uint32

	// SampleCount is the number of samples for multisampling.
	// Use 1 for no multisampling.
	SampleCount uint32

	// Format is the texture pixel format.
	Format gputypes.TextureFormat

	// Usage specifies how the texture will be used.
	Usage TextureUsage
}

// TextureUsage specifies how a texture can be used.
// These flags can be combined with bitwise OR.
type TextureUsage uint32

const (
	// TextureUsageCopySrc allows the texture to be used as a copy source.
	TextureUsageCopySrc TextureUsage = 1 << iota

	// TextureUsageCopyDst allows the texture to be used as a copy destination.
	TextureUsageCopyDst

	// TextureUsageTextureBinding allows the texture to be used in a texture binding.
	TextureUsageTextureBinding

	// TextureUsageStorageBinding allows the texture to be used in a storage binding.
	TextureUsageStorageBinding

	// TextureUsageRenderAttachment allows the texture to be used as a render attachment.
	TextureUsageRenderAttachment
)

// Texture represents a GPU texture resource.
// This interface wraps the underlying WebGPU texture.
type Texture interface {
	// Width returns the texture width in pixels.
	Width() uint32

	// Height returns the texture height in pixels.
	Height() uint32

	// Format returns the texture pixel format.
	Format() gputypes.TextureFormat

	// CreateView creates a view for this texture.
	CreateView() TextureView

	// Destroy releases GPU resources associated with this texture.
	Destroy()
}

// TextureView represents a view into a texture.
// Views are used to bind textures to shader stages.
type TextureView interface {
	// Destroy releases resources associated with this view.
	Destroy()
}

// DefaultTextureDescriptor returns a TextureDescriptor with sensible defaults.
// Only Width, Height, and Format need to be set.
func DefaultTextureDescriptor(width, height uint32, format gputypes.TextureFormat) TextureDescriptor {
	return TextureDescriptor{
		Width:         width,
		Height:        height,
		Depth:         1,
		MipLevelCount: 1,
		SampleCount:   1,
		Format:        format,
		Usage:         TextureUsageTextureBinding | TextureUsageRenderAttachment,
	}
}

// DeviceCapabilities describes the capabilities of a GPU device.
// Used to determine available features and limits for rendering decisions.
type DeviceCapabilities struct {
	// MaxTextureSize is the maximum texture dimension supported.
	MaxTextureSize uint32

	// MaxBindGroups is the maximum number of bind groups.
	MaxBindGroups uint32

	// SupportsCompute indicates if compute shaders are supported.
	SupportsCompute bool

	// SupportsStorageTextures indicates if storage textures are supported.
	SupportsStorageTextures bool

	// VendorName is the GPU vendor name.
	VendorName string

	// DeviceName is the GPU device name.
	DeviceName string
}

// NullDeviceHandle is a DeviceHandle that provides nil implementations.
// Used for CPU-only rendering where no GPU is available.
type NullDeviceHandle struct{}

// Device returns nil for the null device.
func (NullDeviceHandle) Device() gpucontext.Device { return nil }

// Queue returns nil for the null device.
func (NullDeviceHandle) Queue() gpucontext.Queue { return nil }

// Adapter returns nil for the null device.
func (NullDeviceHandle) Adapter() gpucontext.Adapter { return nil }

// SurfaceFormat returns undefined format for the null device.
func (NullDeviceHandle) SurfaceFormat() gputypes.TextureFormat {
	return gputypes.TextureFormatUndefined
}

// Ensure NullDeviceHandle implements DeviceHandle.
var _ DeviceHandle = NullDeviceHandle{}
