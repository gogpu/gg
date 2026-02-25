// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package render

import (
	"image/color"
	"testing"
)

func TestNewGPURenderer(t *testing.T) {
	renderer, err := NewGPURenderer(NullDeviceHandle{})
	if err != nil {
		t.Fatalf("NewGPURenderer() error = %v", err)
	}
	if renderer == nil {
		t.Fatal("NewGPURenderer() returned nil")
	}
}

func TestNewGPURendererNilHandle(t *testing.T) {
	_, err := NewGPURenderer(nil)
	if err == nil {
		t.Error("NewGPURenderer(nil) should return error")
	}
}

func TestGPURendererCapabilities(t *testing.T) {
	renderer, _ := NewGPURenderer(NullDeviceHandle{})
	caps := renderer.Capabilities()

	if !caps.IsGPU {
		t.Error("GPURenderer.Capabilities().IsGPU should be true")
	}
	if !caps.SupportsAntialiasing {
		t.Error("GPURenderer should support antialiasing")
	}
	if caps.MaxTextureSize == 0 {
		t.Error("MaxTextureSize should not be 0")
	}
}

func TestGPURendererFlush(t *testing.T) {
	renderer, _ := NewGPURenderer(NullDeviceHandle{})

	err := renderer.Flush()
	if err != nil {
		t.Errorf("Flush() error = %v, want nil", err)
	}
}

func TestGPURendererDeviceHandle(t *testing.T) {
	handle := NullDeviceHandle{}
	renderer, _ := NewGPURenderer(handle)

	if renderer.DeviceHandle() != handle {
		t.Error("DeviceHandle() should return the provided handle")
	}
}

func TestGPURendererCPUTarget(t *testing.T) {
	renderer, _ := NewGPURenderer(NullDeviceHandle{})
	target := NewPixmapTarget(100, 100)
	scene := NewScene()

	scene.Clear(color.RGBA{255, 0, 0, 255})

	// Should fall back to software rendering
	err := renderer.Render(target, scene)
	if err != nil {
		t.Errorf("Render() to CPU target error = %v", err)
	}

	// Verify rendering worked
	pixel := target.GetPixel(50, 50).(color.RGBA)
	if pixel.R != 255 || pixel.G != 0 || pixel.B != 0 {
		t.Errorf("Pixel = %v, want red", pixel)
	}
}

func TestGPURendererGPUTarget(t *testing.T) {
	renderer, _ := NewGPURenderer(NullDeviceHandle{})
	target := NewSurfaceTarget(100, 100, 0, nil)
	scene := NewScene()

	scene.Clear(color.White)

	// Phase 1: GPU targets not implemented
	err := renderer.Render(target, scene)
	if err == nil {
		t.Error("Render() to GPU target should return error in Phase 1")
	}
}

func TestGPURendererNilTarget(t *testing.T) {
	renderer, _ := NewGPURenderer(NullDeviceHandle{})
	scene := NewScene()

	err := renderer.Render(nil, scene)
	if err == nil {
		t.Error("Render(nil, _) should return error")
	}
}

func TestGPURendererCreateTextureTarget(t *testing.T) {
	renderer, _ := NewGPURenderer(NullDeviceHandle{})

	// Phase 1: Should return error
	_, err := renderer.CreateTextureTarget(256, 256)
	if err == nil {
		t.Error("CreateTextureTarget() should return error in Phase 1")
	}
}

func TestGPURendererFillWithSoftwareFallback(t *testing.T) {
	renderer, _ := NewGPURenderer(NullDeviceHandle{})
	target := NewPixmapTarget(200, 200)
	scene := NewScene()

	scene.Clear(color.White)
	scene.SetFillColor(color.RGBA{0, 0, 255, 255})
	scene.Circle(100, 100, 50)
	scene.Fill()

	err := renderer.Render(target, scene)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Check center (should be blue)
	center := target.GetPixel(100, 100).(color.RGBA)
	if center.B != 255 || center.R != 0 {
		t.Errorf("Center = %v, want blue", center)
	}

	// Check corner (should be white)
	corner := target.GetPixel(10, 10).(color.RGBA)
	if corner.R != 255 || corner.G != 255 || corner.B != 255 {
		t.Errorf("Corner = %v, want white", corner)
	}
}

func TestGPURendererImplementsRenderer(t *testing.T) {
	renderer, _ := NewGPURenderer(NullDeviceHandle{})

	// Verify interface implementation
	var _ Renderer = renderer
	var _ CapableRenderer = renderer
}
