// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"testing"

	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
)

func TestNullDeviceHandle(t *testing.T) {
	var handle DeviceHandle = NullDeviceHandle{}

	if handle.Device() != nil {
		t.Error("NullDeviceHandle.Device() should return nil")
	}
	if handle.Queue() != nil {
		t.Error("NullDeviceHandle.Queue() should return nil")
	}
	if handle.Adapter() != nil {
		t.Error("NullDeviceHandle.Adapter() should return nil")
	}
	if handle.SurfaceFormat() != gputypes.TextureFormatUndefined {
		t.Error("NullDeviceHandle.SurfaceFormat() should return Undefined")
	}
}

func TestTextureDescriptorDefault(t *testing.T) {
	desc := DefaultTextureDescriptor(256, 128, gputypes.TextureFormatRGBA8Unorm)

	if desc.Width != 256 {
		t.Errorf("Width = %d, want 256", desc.Width)
	}
	if desc.Height != 128 {
		t.Errorf("Height = %d, want 128", desc.Height)
	}
	if desc.Depth != 1 {
		t.Errorf("Depth = %d, want 1", desc.Depth)
	}
	if desc.MipLevelCount != 1 {
		t.Errorf("MipLevelCount = %d, want 1", desc.MipLevelCount)
	}
	if desc.SampleCount != 1 {
		t.Errorf("SampleCount = %d, want 1", desc.SampleCount)
	}
	if desc.Format != gputypes.TextureFormatRGBA8Unorm {
		t.Errorf("Format = %v, want RGBA8Unorm", desc.Format)
	}

	expectedUsage := TextureUsageTextureBinding | TextureUsageRenderAttachment
	if desc.Usage != expectedUsage {
		t.Errorf("Usage = %v, want %v", desc.Usage, expectedUsage)
	}
}

func TestTextureUsageFlags(t *testing.T) {
	// Test that flags can be combined
	usage := TextureUsageCopySrc | TextureUsageCopyDst | TextureUsageRenderAttachment

	if usage&TextureUsageCopySrc == 0 {
		t.Error("Missing CopySrc flag")
	}
	if usage&TextureUsageCopyDst == 0 {
		t.Error("Missing CopyDst flag")
	}
	if usage&TextureUsageRenderAttachment == 0 {
		t.Error("Missing RenderAttachment flag")
	}
	if usage&TextureUsageTextureBinding != 0 {
		t.Error("Should not have TextureBinding flag")
	}
}

func TestDeviceHandleAlias(t *testing.T) {
	// DeviceHandle should be an alias for gpucontext.DeviceProvider
	// This test verifies type compatibility at compile time
	handle := NullDeviceHandle{}

	// Verify handle is usable as DeviceHandle
	var dh DeviceHandle = handle
	if dh.Device() != nil {
		t.Error("NullDeviceHandle.Device() should return nil")
	}

	// Verify DeviceHandle is compatible with gpucontext.DeviceProvider
	// This is a compile-time check - if it compiles, types are compatible
	acceptProvider := func(_ gpucontext.DeviceProvider) {}
	acceptProvider(handle)
}

func TestDeviceCapabilities(t *testing.T) {
	caps := DeviceCapabilities{
		MaxTextureSize:          16384,
		MaxBindGroups:           8,
		SupportsCompute:         true,
		SupportsStorageTextures: true,
		VendorName:              "TestVendor",
		DeviceName:              "TestDevice",
	}

	if caps.MaxTextureSize != 16384 {
		t.Errorf("MaxTextureSize = %d, want 16384", caps.MaxTextureSize)
	}
	if caps.MaxBindGroups != 8 {
		t.Errorf("MaxBindGroups = %d, want 8", caps.MaxBindGroups)
	}
	if !caps.SupportsCompute {
		t.Error("SupportsCompute should be true")
	}
	if !caps.SupportsStorageTextures {
		t.Error("SupportsStorageTextures should be true")
	}
	if caps.VendorName != "TestVendor" {
		t.Errorf("VendorName = %s, want TestVendor", caps.VendorName)
	}
	if caps.DeviceName != "TestDevice" {
		t.Errorf("DeviceName = %s, want TestDevice", caps.DeviceName)
	}
}
