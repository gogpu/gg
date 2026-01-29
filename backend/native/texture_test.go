package native

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// =============================================================================
// Mock Types for Testing
// =============================================================================

// mockHALDevice is a test double for hal.Device.
type mockHALDevice struct {
	createTextureFunc      func(*hal.TextureDescriptor) (hal.Texture, error)
	createTextureViewFunc  func(hal.Texture, *hal.TextureViewDescriptor) (hal.TextureView, error)
	destroyTextureFunc     func(hal.Texture)
	destroyTextureViewFunc func(hal.TextureView)

	// Track calls for verification
	texturesCreated     int32
	textureViewsCreated int32
	texturesDestroyed   int32
	viewsDestroyed      int32
}

//nolint:nilnil // Mock: intentionally returns nil for unused interface methods.
func (d *mockHALDevice) CreateBuffer(_ *hal.BufferDescriptor) (hal.Buffer, error) {
	return nil, nil
}

func (d *mockHALDevice) DestroyBuffer(_ hal.Buffer) {}

func (d *mockHALDevice) CreateTexture(desc *hal.TextureDescriptor) (hal.Texture, error) {
	atomic.AddInt32(&d.texturesCreated, 1)
	if d.createTextureFunc != nil {
		return d.createTextureFunc(desc)
	}
	return &mockHALTexture{
		width:  desc.Size.Width,
		height: desc.Size.Height,
		format: desc.Format,
	}, nil
}

func (d *mockHALDevice) DestroyTexture(texture hal.Texture) {
	atomic.AddInt32(&d.texturesDestroyed, 1)
	if d.destroyTextureFunc != nil {
		d.destroyTextureFunc(texture)
	}
}

func (d *mockHALDevice) CreateTextureView(texture hal.Texture, desc *hal.TextureViewDescriptor) (hal.TextureView, error) {
	atomic.AddInt32(&d.textureViewsCreated, 1)
	if d.createTextureViewFunc != nil {
		return d.createTextureViewFunc(texture, desc)
	}
	return &mockHALTextureView{
		texture: texture,
		label:   desc.Label,
	}, nil
}

func (d *mockHALDevice) DestroyTextureView(view hal.TextureView) {
	atomic.AddInt32(&d.viewsDestroyed, 1)
	if d.destroyTextureViewFunc != nil {
		d.destroyTextureViewFunc(view)
	}
}

// Implement remaining hal.Device interface methods as no-ops.
// All return nil,nil as mocks - these are not called in texture tests.

//nolint:nilnil // Mock: unused interface methods.
func (d *mockHALDevice) CreateSampler(_ *hal.SamplerDescriptor) (hal.Sampler, error) {
	return nil, nil
}
func (d *mockHALDevice) DestroySampler(_ hal.Sampler) {}

//nolint:nilnil // Mock: unused interface methods.
func (d *mockHALDevice) CreateBindGroupLayout(_ *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	return nil, nil
}
func (d *mockHALDevice) DestroyBindGroupLayout(_ hal.BindGroupLayout) {}

//nolint:nilnil // Mock: unused interface methods.
func (d *mockHALDevice) CreateBindGroup(_ *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	return nil, nil
}
func (d *mockHALDevice) DestroyBindGroup(_ hal.BindGroup) {}

//nolint:nilnil // Mock: unused interface methods.
func (d *mockHALDevice) CreatePipelineLayout(_ *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	return nil, nil
}
func (d *mockHALDevice) DestroyPipelineLayout(_ hal.PipelineLayout) {}

//nolint:nilnil // Mock: unused interface methods.
func (d *mockHALDevice) CreateShaderModule(_ *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	return nil, nil
}
func (d *mockHALDevice) DestroyShaderModule(_ hal.ShaderModule) {}

//nolint:nilnil // Mock: unused interface methods.
func (d *mockHALDevice) CreateRenderPipeline(_ *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	return nil, nil
}
func (d *mockHALDevice) DestroyRenderPipeline(_ hal.RenderPipeline) {}

//nolint:nilnil // Mock: unused interface methods.
func (d *mockHALDevice) CreateComputePipeline(_ *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	return nil, nil
}
func (d *mockHALDevice) DestroyComputePipeline(_ hal.ComputePipeline) {}

//nolint:nilnil // Mock: unused interface methods.
func (d *mockHALDevice) CreateCommandEncoder(_ *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return nil, nil
}

//nolint:nilnil // Mock: unused interface methods.
func (d *mockHALDevice) CreateFence() (hal.Fence, error) { return nil, nil }
func (d *mockHALDevice) DestroyFence(_ hal.Fence)        {}
func (d *mockHALDevice) Wait(_ hal.Fence, _ uint64, _ time.Duration) (bool, error) {
	return true, nil
}
func (d *mockHALDevice) Destroy() {}

// mockHALTexture is a test double for hal.Texture.
type mockHALTexture struct {
	width  uint32
	height uint32
	format gputypes.TextureFormat
}

// Destroy implements hal.Resource.
func (t *mockHALTexture) Destroy() {}

// NativeHandle implements hal.NativeHandle.
func (t *mockHALTexture) NativeHandle() uintptr { return 0 }

// mockHALTextureView is a test double for hal.TextureView.
type mockHALTextureView struct {
	texture hal.Texture
	label   string
}

// Destroy implements hal.Resource.
func (v *mockHALTextureView) Destroy() {}

// NativeHandle implements hal.NativeHandle.
func (v *mockHALTextureView) NativeHandle() uintptr { return 0 }

// =============================================================================
// Texture Tests
// =============================================================================

func TestNewTexture(t *testing.T) {
	device := &mockHALDevice{}
	halTex := &mockHALTexture{width: 256, height: 256, format: gputypes.TextureFormatRGBA8Unorm}
	desc := &TextureDescriptor{
		Label: "test-texture",
		Size: gputypes.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
	}

	tex := NewTexture(halTex, device, desc)

	if tex == nil {
		t.Fatal("NewTexture returned nil")
	}
	if tex.Label() != "test-texture" {
		t.Errorf("Label = %q, want %q", tex.Label(), "test-texture")
	}
	if tex.Width() != 256 {
		t.Errorf("Width = %d, want 256", tex.Width())
	}
	if tex.Height() != 256 {
		t.Errorf("Height = %d, want 256", tex.Height())
	}
	if tex.Format() != gputypes.TextureFormatRGBA8Unorm {
		t.Errorf("Format = %v, want RGBA8Unorm", tex.Format())
	}
	if tex.IsDestroyed() {
		t.Error("IsDestroyed = true, want false")
	}
}

func TestTexture_GetDefaultView(t *testing.T) {
	device := &mockHALDevice{}
	halTex := &mockHALTexture{width: 512, height: 512}
	desc := &TextureDescriptor{
		Label: "test-texture",
		Size: gputypes.Extent3D{
			Width:              512,
			Height:             512,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 4,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}

	tex := NewTexture(halTex, device, desc)

	// Get default view
	view1, err := tex.GetDefaultView()
	if err != nil {
		t.Fatalf("GetDefaultView failed: %v", err)
	}
	if view1 == nil {
		t.Fatal("GetDefaultView returned nil view")
	}
	if !view1.IsDefault() {
		t.Error("view.IsDefault() = false, want true")
	}
	if device.textureViewsCreated != 1 {
		t.Errorf("textureViewsCreated = %d, want 1", device.textureViewsCreated)
	}

	// Get default view again - should return same instance
	view2, err := tex.GetDefaultView()
	if err != nil {
		t.Fatalf("GetDefaultView (second call) failed: %v", err)
	}
	if view2 != view1 {
		t.Error("GetDefaultView returned different view on second call")
	}
	if device.textureViewsCreated != 1 {
		t.Errorf("textureViewsCreated = %d, want 1 (should not create again)", device.textureViewsCreated)
	}
}

func TestTexture_GetDefaultView_Concurrent(t *testing.T) {
	device := &mockHALDevice{}
	halTex := &mockHALTexture{width: 256, height: 256}
	desc := &TextureDescriptor{
		Label: "concurrent-test",
		Size: gputypes.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}

	tex := NewTexture(halTex, device, desc)

	const numGoroutines = 10
	var wg sync.WaitGroup
	views := make([]*TextureView, numGoroutines)
	errs := make([]error, numGoroutines)

	// Launch multiple goroutines to call GetDefaultView concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			views[idx], errs[idx] = tex.GetDefaultView()
		}(i)
	}

	wg.Wait()

	// All should succeed and return the same view
	for i := 0; i < numGoroutines; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: GetDefaultView failed: %v", i, errs[i])
		}
		if views[i] != views[0] {
			t.Errorf("goroutine %d: got different view than goroutine 0", i)
		}
	}

	// Should only create one view
	if device.textureViewsCreated != 1 {
		t.Errorf("textureViewsCreated = %d, want 1", device.textureViewsCreated)
	}
}

func TestTexture_CreateView(t *testing.T) {
	device := &mockHALDevice{}
	halTex := &mockHALTexture{width: 512, height: 512}
	desc := &TextureDescriptor{
		Label: "test-texture",
		Size: gputypes.Extent3D{
			Width:              512,
			Height:             512,
			DepthOrArrayLayers: 4,
		},
		MipLevelCount: 4,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}

	tex := NewTexture(halTex, device, desc)

	// Create custom view
	viewDesc := &TextureViewDescriptor{
		Label:           "custom-view",
		Format:          gputypes.TextureFormatRGBA8Unorm,
		Dimension:       gputypes.TextureViewDimension2D,
		Aspect:          gputypes.TextureAspectAll,
		BaseMipLevel:    1,
		MipLevelCount:   2,
		BaseArrayLayer:  0,
		ArrayLayerCount: 1,
	}

	view, err := tex.CreateView(viewDesc)
	if err != nil {
		t.Fatalf("CreateView failed: %v", err)
	}
	if view == nil {
		t.Fatal("CreateView returned nil view")
	}
	if view.IsDefault() {
		t.Error("view.IsDefault() = true, want false")
	}
	if view.Label() != "custom-view" {
		t.Errorf("view.Label() = %q, want %q", view.Label(), "custom-view")
	}
	if view.BaseMipLevel() != 1 {
		t.Errorf("view.BaseMipLevel() = %d, want 1", view.BaseMipLevel())
	}
	if view.MipLevelCount() != 2 {
		t.Errorf("view.MipLevelCount() = %d, want 2", view.MipLevelCount())
	}
	if view.Texture() != tex {
		t.Error("view.Texture() does not match parent texture")
	}
}

func TestTexture_CreateView_NilDescriptor(t *testing.T) {
	device := &mockHALDevice{}
	halTex := &mockHALTexture{width: 256, height: 256}
	desc := &TextureDescriptor{
		Label: "test-texture",
		Size: gputypes.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}

	tex := NewTexture(halTex, device, desc)

	// CreateView with nil descriptor should return default view
	view, err := tex.CreateView(nil)
	if err != nil {
		t.Fatalf("CreateView(nil) failed: %v", err)
	}
	if view == nil {
		t.Fatal("CreateView(nil) returned nil view")
	}
	if !view.IsDefault() {
		t.Error("view.IsDefault() = false, want true (nil desc should return default)")
	}
}

func TestTexture_Destroy(t *testing.T) {
	device := &mockHALDevice{}
	halTex := &mockHALTexture{width: 256, height: 256}
	desc := &TextureDescriptor{
		Label: "test-texture",
		Size: gputypes.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}

	tex := NewTexture(halTex, device, desc)

	// Get default view first
	_, _ = tex.GetDefaultView()

	// Destroy the texture
	tex.Destroy()

	if !tex.IsDestroyed() {
		t.Error("IsDestroyed = false after Destroy()")
	}
	if tex.Raw() != nil {
		t.Error("Raw() should return nil after Destroy()")
	}
	if device.texturesDestroyed != 1 {
		t.Errorf("texturesDestroyed = %d, want 1", device.texturesDestroyed)
	}
	if device.viewsDestroyed != 1 {
		t.Errorf("viewsDestroyed = %d, want 1 (default view should be destroyed)", device.viewsDestroyed)
	}
}

func TestTexture_Destroy_Idempotent(t *testing.T) {
	device := &mockHALDevice{}
	halTex := &mockHALTexture{width: 256, height: 256}
	desc := &TextureDescriptor{
		Label: "test-texture",
		Size: gputypes.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}

	tex := NewTexture(halTex, device, desc)

	// Destroy multiple times
	tex.Destroy()
	tex.Destroy()
	tex.Destroy()

	// Should only destroy once
	if device.texturesDestroyed != 1 {
		t.Errorf("texturesDestroyed = %d, want 1", device.texturesDestroyed)
	}
}

func TestTexture_GetDefaultView_AfterDestroy(t *testing.T) {
	device := &mockHALDevice{}
	halTex := &mockHALTexture{width: 256, height: 256}
	desc := &TextureDescriptor{
		Label: "test-texture",
		Size: gputypes.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}

	tex := NewTexture(halTex, device, desc)
	tex.Destroy()

	// GetDefaultView after destroy should fail
	_, err := tex.GetDefaultView()
	if !errors.Is(err, ErrTextureDestroyed) {
		t.Errorf("GetDefaultView after Destroy: got %v, want ErrTextureDestroyed", err)
	}
}

// =============================================================================
// TextureView Tests
// =============================================================================

func TestTextureView_Destroy_NonDefault(t *testing.T) {
	device := &mockHALDevice{}
	halTex := &mockHALTexture{width: 256, height: 256}
	desc := &TextureDescriptor{
		Label: "test-texture",
		Size: gputypes.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}

	tex := NewTexture(halTex, device, desc)

	// Create a custom view
	view, _ := tex.CreateView(&TextureViewDescriptor{
		Label: "custom-view",
	})

	// Destroy the custom view
	view.Destroy()

	if !view.IsDestroyed() {
		t.Error("view.IsDestroyed() = false after Destroy()")
	}
	if device.viewsDestroyed != 1 {
		t.Errorf("viewsDestroyed = %d, want 1", device.viewsDestroyed)
	}
}

func TestTextureView_Destroy_Default_NoOp(t *testing.T) {
	device := &mockHALDevice{}
	halTex := &mockHALTexture{width: 256, height: 256}
	desc := &TextureDescriptor{
		Label: "test-texture",
		Size: gputypes.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}

	tex := NewTexture(halTex, device, desc)

	// Get default view
	defaultView, _ := tex.GetDefaultView()

	// Trying to destroy default view via public API should be no-op
	defaultView.Destroy()

	if defaultView.IsDestroyed() {
		t.Error("default view should not be destroyed via public Destroy()")
	}
	if device.viewsDestroyed != 0 {
		t.Errorf("viewsDestroyed = %d, want 0", device.viewsDestroyed)
	}
}

// =============================================================================
// CreateCoreTexture Tests
// =============================================================================

func TestCreateCoreTexture(t *testing.T) {
	device := &mockHALDevice{}
	desc := &TextureDescriptor{
		Label: "created-texture",
		Size: gputypes.Extent3D{
			Width:              512,
			Height:             512,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
	}

	tex, err := CreateCoreTexture(device, desc)
	if err != nil {
		t.Fatalf("CreateCoreTexture failed: %v", err)
	}
	if tex == nil {
		t.Fatal("CreateCoreTexture returned nil")
	}
	if tex.Width() != 512 {
		t.Errorf("Width = %d, want 512", tex.Width())
	}
	if tex.Height() != 512 {
		t.Errorf("Height = %d, want 512", tex.Height())
	}
	if device.texturesCreated != 1 {
		t.Errorf("texturesCreated = %d, want 1", device.texturesCreated)
	}
}

func TestCreateCoreTexture_NilDevice(t *testing.T) {
	desc := &TextureDescriptor{
		Label: "test",
		Size: gputypes.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
	}

	_, err := CreateCoreTexture(nil, desc)
	if !errors.Is(err, ErrNilHALDevice) {
		t.Errorf("CreateCoreTexture(nil device): got %v, want ErrNilHALDevice", err)
	}
}

func TestCreateCoreTexture_NilDescriptor(t *testing.T) {
	device := &mockHALDevice{}

	_, err := CreateCoreTexture(device, nil)
	if err == nil {
		t.Error("CreateCoreTexture(nil desc) should fail")
	}
}

func TestCreateCoreTexture_InvalidSize(t *testing.T) {
	device := &mockHALDevice{}

	tests := []struct {
		name   string
		width  uint32
		height uint32
	}{
		{"zero width", 0, 256},
		{"zero height", 256, 0},
		{"both zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := &TextureDescriptor{
				Label: "test",
				Size: gputypes.Extent3D{
					Width:              tt.width,
					Height:             tt.height,
					DepthOrArrayLayers: 1,
				},
			}

			_, err := CreateCoreTexture(device, desc)
			if err == nil {
				t.Error("CreateCoreTexture with invalid size should fail")
			}
		})
	}
}

func TestCreateCoreTexture_DefaultValues(t *testing.T) {
	device := &mockHALDevice{}
	desc := &TextureDescriptor{
		Label: "defaults-test",
		Size: gputypes.Extent3D{
			Width:  256,
			Height: 256,
			// DepthOrArrayLayers: 0 (should default to 1)
		},
		// MipLevelCount: 0 (should default to 1)
		// SampleCount: 0 (should default to 1)
		Dimension: gputypes.TextureDimension2D,
		Format:    gputypes.TextureFormatRGBA8Unorm,
		Usage:     gputypes.TextureUsageTextureBinding,
	}

	tex, err := CreateCoreTexture(device, desc)
	if err != nil {
		t.Fatalf("CreateCoreTexture failed: %v", err)
	}

	if tex.MipLevelCount() != 1 {
		t.Errorf("MipLevelCount = %d, want 1 (default)", tex.MipLevelCount())
	}
	if tex.SampleCount() != 1 {
		t.Errorf("SampleCount = %d, want 1 (default)", tex.SampleCount())
	}
	if tex.DepthOrArrayLayers() != 1 {
		t.Errorf("DepthOrArrayLayers = %d, want 1 (default)", tex.DepthOrArrayLayers())
	}
}

func TestCreateCoreTextureSimple(t *testing.T) {
	device := &mockHALDevice{}

	tex, err := CreateCoreTextureSimple(
		device,
		1024, 768,
		gputypes.TextureFormatBGRA8Unorm,
		gputypes.TextureUsageRenderAttachment|gputypes.TextureUsageCopySrc,
		"simple-texture",
	)
	if err != nil {
		t.Fatalf("CreateCoreTextureSimple failed: %v", err)
	}
	if tex.Width() != 1024 {
		t.Errorf("Width = %d, want 1024", tex.Width())
	}
	if tex.Height() != 768 {
		t.Errorf("Height = %d, want 768", tex.Height())
	}
	if tex.Format() != gputypes.TextureFormatBGRA8Unorm {
		t.Errorf("Format = %v, want BGRA8Unorm", tex.Format())
	}
	if tex.Label() != "simple-texture" {
		t.Errorf("Label = %q, want %q", tex.Label(), "simple-texture")
	}
	if tex.Dimension() != gputypes.TextureDimension2D {
		t.Errorf("Dimension = %v, want 2D", tex.Dimension())
	}
}

// =============================================================================
// Helper Tests
// =============================================================================

func TestTextureViewDimensionFromTexture(t *testing.T) {
	tests := []struct {
		input gputypes.TextureDimension
		want  gputypes.TextureViewDimension
	}{
		{gputypes.TextureDimension1D, gputypes.TextureViewDimension1D},
		{gputypes.TextureDimension2D, gputypes.TextureViewDimension2D},
		{gputypes.TextureDimension3D, gputypes.TextureViewDimension3D},
	}

	for _, tt := range tests {
		got := textureViewDimensionFromTexture(tt.input)
		if got != tt.want {
			t.Errorf("textureViewDimensionFromTexture(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
