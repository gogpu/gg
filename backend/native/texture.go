// Package native provides a GPU-accelerated rendering backend using gogpu/wgpu.
package native

import (
	"errors"
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

// Texture errors.
var (
	// ErrTextureDestroyed is returned when operating on a destroyed texture.
	ErrTextureDestroyed = errors.New("native: texture has been destroyed")

	// ErrTextureViewDestroyed is returned when operating on a destroyed texture view.
	ErrTextureViewDestroyed = errors.New("native: texture view has been destroyed")

	// ErrNilHALDevice is returned when creating a texture without a device.
	ErrNilHALDevice = errors.New("native: device is nil")

	// ErrNilTexture is returned when creating a view without a texture.
	ErrNilTexture = errors.New("native: texture is nil")

	// ErrInvalidTextureSize is returned when texture dimensions are invalid.
	ErrInvalidTextureSize = errors.New("native: invalid texture size")

	// ErrDefaultViewCreationFailed is returned when lazy default view creation fails.
	ErrDefaultViewCreationFailed = errors.New("native: failed to create default view")
)

// Texture represents a GPU texture resource.
//
// Texture wraps a hal.Texture and provides Go-idiomatic access with
// lazy default view creation using sync.Once. This follows the wgpu pattern
// where textures have a default view that is created on-demand.
//
// Thread Safety:
// Texture is safe for concurrent read access. The default view is
// lazily created using sync.Once, making GetDefaultView() thread-safe.
// Destroy() should only be called once, typically when the texture is
// no longer needed.
//
// Lifecycle:
//  1. Create via Device.CreateTexture() or CreateTexture()
//  2. Use GetDefaultView() for simple render targets
//  3. Use CreateView() for custom views (mip levels, array layers, etc.)
//  4. Call Destroy() when done
type Texture struct {
	// mu protects mutable state.
	mu sync.RWMutex

	// halTexture is the underlying texture handle.
	halTexture hal.Texture

	// device is the parent device.
	device hal.Device

	// descriptor holds the texture configuration (immutable after creation).
	descriptor TextureDescriptor

	// defaultViewOnce ensures the default view is created exactly once.
	defaultViewOnce sync.Once

	// defaultView is the lazily-created default texture view.
	defaultView *TextureView

	// defaultViewErr stores any error from default view creation.
	defaultViewErr error

	// destroyed indicates whether the texture has been destroyed.
	destroyed bool
}

// TextureDescriptor describes a texture to create.
type TextureDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Size is the texture dimensions.
	Size types.Extent3D

	// MipLevelCount is the number of mip levels (1+ required).
	MipLevelCount uint32

	// SampleCount is the number of samples per pixel (1 for non-MSAA).
	SampleCount uint32

	// Dimension is the texture dimension (1D, 2D, 3D).
	Dimension types.TextureDimension

	// Format is the texture pixel format.
	Format types.TextureFormat

	// Usage specifies how the texture will be used.
	Usage types.TextureUsage

	// ViewFormats are additional formats for texture views.
	ViewFormats []types.TextureFormat
}

// NewTexture creates a new Texture from a texture handle.
//
// This is typically called by Device.CreateTexture() after successfully
// creating the underlying texture.
//
// Parameters:
//   - halTexture: The underlying texture (ownership transferred)
//   - device: The parent device (retained for view creation)
//   - desc: The texture descriptor (copied)
//
// Returns the new Texture.
func NewTexture(halTexture hal.Texture, device hal.Device, desc *TextureDescriptor) *Texture {
	return &Texture{
		halTexture: halTexture,
		device:     device,
		descriptor: *desc,
	}
}

// Label returns the texture's debug label.
func (t *Texture) Label() string {
	return t.descriptor.Label
}

// Size returns the texture dimensions.
func (t *Texture) Size() types.Extent3D {
	return t.descriptor.Size
}

// Width returns the texture width in pixels.
func (t *Texture) Width() uint32 {
	return t.descriptor.Size.Width
}

// Height returns the texture height in pixels.
func (t *Texture) Height() uint32 {
	return t.descriptor.Size.Height
}

// DepthOrArrayLayers returns the texture depth or array layer count.
func (t *Texture) DepthOrArrayLayers() uint32 {
	return t.descriptor.Size.DepthOrArrayLayers
}

// MipLevelCount returns the number of mip levels.
func (t *Texture) MipLevelCount() uint32 {
	return t.descriptor.MipLevelCount
}

// SampleCount returns the number of samples per pixel.
func (t *Texture) SampleCount() uint32 {
	return t.descriptor.SampleCount
}

// Dimension returns the texture dimension (1D, 2D, 3D).
func (t *Texture) Dimension() types.TextureDimension {
	return t.descriptor.Dimension
}

// Format returns the texture pixel format.
func (t *Texture) Format() types.TextureFormat {
	return t.descriptor.Format
}

// Usage returns the texture usage flags.
func (t *Texture) Usage() types.TextureUsage {
	return t.descriptor.Usage
}

// Descriptor returns a copy of the texture descriptor.
func (t *Texture) Descriptor() TextureDescriptor {
	return t.descriptor
}

// IsDestroyed returns true if the texture has been destroyed.
func (t *Texture) IsDestroyed() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.destroyed
}

// Raw returns the underlying texture handle.
//
// Returns nil if the texture has been destroyed.
// Use with caution - the caller should ensure the texture is not destroyed
// while the handle is in use.
func (t *Texture) Raw() hal.Texture {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.destroyed {
		return nil
	}
	return t.halTexture
}

// GetDefaultView returns the default texture view, creating it lazily on first call.
//
// The default view covers all mip levels and array layers with the texture's
// native format. This is the most common use case for render targets and
// texture bindings.
//
// Thread Safety: This method is thread-safe. Multiple goroutines can call
// GetDefaultView() concurrently, and the view will be created exactly once.
//
// Returns the default view and nil on success.
// Returns nil and an error if:
//   - The texture has been destroyed
//   - View creation failed
func (t *Texture) GetDefaultView() (*TextureView, error) {
	// Check if already destroyed before attempting view creation
	t.mu.RLock()
	if t.destroyed {
		t.mu.RUnlock()
		return nil, ErrTextureDestroyed
	}
	t.mu.RUnlock()

	// Create default view exactly once
	t.defaultViewOnce.Do(func() {
		t.defaultView, t.defaultViewErr = t.createDefaultView()
	})

	if t.defaultViewErr != nil {
		return nil, t.defaultViewErr
	}
	return t.defaultView, nil
}

// createDefaultView creates the default texture view.
//
// This is called by GetDefaultView via sync.Once.
func (t *Texture) createDefaultView() (*TextureView, error) {
	t.mu.RLock()
	device := t.device
	halTex := t.halTexture
	destroyed := t.destroyed
	t.mu.RUnlock()

	if destroyed {
		return nil, ErrTextureDestroyed
	}

	if device == nil {
		return nil, ErrNilHALDevice
	}

	// Create default view descriptor - use zero values to inherit from texture
	halDesc := &hal.TextureViewDescriptor{
		Label:           t.descriptor.Label + " (default view)",
		Format:          types.TextureFormatUndefined, // Inherit from texture
		Dimension:       types.TextureViewDimensionUndefined,
		Aspect:          types.TextureAspectAll,
		BaseMipLevel:    0,
		MipLevelCount:   0, // 0 means all remaining levels
		BaseArrayLayer:  0,
		ArrayLayerCount: 0, // 0 means all remaining layers
	}

	halView, err := device.CreateTextureView(halTex, halDesc)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDefaultViewCreationFailed, err)
	}

	view := &TextureView{
		halView:    halView,
		texture:    t,
		device:     device,
		descriptor: halViewDescToViewDesc(halDesc, t),
		isDefault:  true,
	}

	return view, nil
}

// CreateView creates a texture view with explicit parameters.
//
// Use this method when you need a custom view that differs from the default,
// such as:
//   - A specific mip level for mipmap generation
//   - A subset of array layers for layered rendering
//   - A different format for format reinterpretation
//   - A different aspect (depth or stencil only)
//
// Parameters:
//   - desc: View descriptor specifying the view configuration
//
// Returns the texture view and nil on success.
// Returns nil and an error if:
//   - The texture has been destroyed
//   - The descriptor is nil
//   - View creation failed
func (t *Texture) CreateView(desc *TextureViewDescriptor) (*TextureView, error) {
	t.mu.RLock()
	device := t.device
	halTex := t.halTexture
	destroyed := t.destroyed
	t.mu.RUnlock()

	if destroyed {
		return nil, ErrTextureDestroyed
	}

	if desc == nil {
		// Use default descriptor
		return t.GetDefaultView()
	}

	if device == nil {
		return nil, ErrNilHALDevice
	}

	// Convert to descriptor
	halDesc := &hal.TextureViewDescriptor{
		Label:           desc.Label,
		Format:          desc.Format,
		Dimension:       desc.Dimension,
		Aspect:          desc.Aspect,
		BaseMipLevel:    desc.BaseMipLevel,
		MipLevelCount:   desc.MipLevelCount,
		BaseArrayLayer:  desc.BaseArrayLayer,
		ArrayLayerCount: desc.ArrayLayerCount,
	}

	halView, err := device.CreateTextureView(halTex, halDesc)
	if err != nil {
		return nil, fmt.Errorf("create texture view: %w", err)
	}

	view := &TextureView{
		halView:    halView,
		texture:    t,
		device:     device,
		descriptor: *desc,
		isDefault:  false,
	}

	return view, nil
}

// Destroy releases the texture and any associated resources.
//
// After calling Destroy(), the texture and any views created from it
// should not be used.
//
// This method is idempotent - calling it multiple times is safe.
func (t *Texture) Destroy() {
	t.mu.Lock()
	if t.destroyed {
		t.mu.Unlock()
		return
	}
	t.destroyed = true
	device := t.device
	halTex := t.halTexture
	defaultView := t.defaultView
	t.halTexture = nil
	t.mu.Unlock()

	// Destroy the default view if it was created
	if defaultView != nil {
		defaultView.destroy()
	}

	// Destroy the texture
	if device != nil && halTex != nil {
		device.DestroyTexture(halTex)
	}
}

// TextureView represents a view into a GPU texture.
//
// Texture views provide different ways to access texture data, such as:
//   - Different mip levels
//   - Different array layers
//   - Different formats (for format reinterpretation)
//   - Different aspects (depth, stencil, color)
//
// Thread Safety:
// TextureView is safe for concurrent read access. Destroy() should
// only be called once, and only for non-default views.
type TextureView struct {
	// mu protects mutable state.
	mu sync.RWMutex

	// halView is the underlying texture view handle.
	halView hal.TextureView

	// texture is the parent texture (retained reference).
	texture *Texture

	// device is the device (retained for destruction).
	device hal.Device

	// descriptor holds the view configuration.
	descriptor TextureViewDescriptor

	// isDefault indicates if this is the texture's default view.
	// Default views are destroyed when the texture is destroyed.
	isDefault bool

	// destroyed indicates whether the view has been destroyed.
	destroyed bool
}

// TextureViewDescriptor describes a texture view to create.
type TextureViewDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Format is the view format (use TextureFormatUndefined to inherit from texture).
	Format types.TextureFormat

	// Dimension is the view dimension (use TextureViewDimensionUndefined to inherit).
	Dimension types.TextureViewDimension

	// Aspect specifies which aspect to view (color, depth, stencil).
	Aspect types.TextureAspect

	// BaseMipLevel is the first mip level in the view.
	BaseMipLevel uint32

	// MipLevelCount is the number of mip levels (0 means all remaining levels).
	MipLevelCount uint32

	// BaseArrayLayer is the first array layer in the view.
	BaseArrayLayer uint32

	// ArrayLayerCount is the number of array layers (0 means all remaining layers).
	ArrayLayerCount uint32
}

// Label returns the view's debug label.
func (v *TextureView) Label() string {
	return v.descriptor.Label
}

// Format returns the view's format.
// Returns TextureFormatUndefined if the view inherits from the texture.
func (v *TextureView) Format() types.TextureFormat {
	return v.descriptor.Format
}

// Dimension returns the view's dimension.
func (v *TextureView) Dimension() types.TextureViewDimension {
	return v.descriptor.Dimension
}

// Aspect returns the view's aspect.
func (v *TextureView) Aspect() types.TextureAspect {
	return v.descriptor.Aspect
}

// BaseMipLevel returns the first mip level in the view.
func (v *TextureView) BaseMipLevel() uint32 {
	return v.descriptor.BaseMipLevel
}

// MipLevelCount returns the number of mip levels in the view.
func (v *TextureView) MipLevelCount() uint32 {
	return v.descriptor.MipLevelCount
}

// BaseArrayLayer returns the first array layer in the view.
func (v *TextureView) BaseArrayLayer() uint32 {
	return v.descriptor.BaseArrayLayer
}

// ArrayLayerCount returns the number of array layers in the view.
func (v *TextureView) ArrayLayerCount() uint32 {
	return v.descriptor.ArrayLayerCount
}

// Texture returns the parent texture.
func (v *TextureView) Texture() *Texture {
	return v.texture
}

// Descriptor returns a copy of the view descriptor.
func (v *TextureView) Descriptor() TextureViewDescriptor {
	return v.descriptor
}

// IsDefault returns true if this is the texture's default view.
func (v *TextureView) IsDefault() bool {
	return v.isDefault
}

// IsDestroyed returns true if the view has been destroyed.
func (v *TextureView) IsDestroyed() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.destroyed
}

// Raw returns the underlying texture view handle.
//
// Returns nil if the view has been destroyed.
// Use with caution - the caller should ensure the view is not destroyed
// while the handle is in use.
func (v *TextureView) Raw() hal.TextureView {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.destroyed {
		return nil
	}
	return v.halView
}

// Destroy releases the texture view.
//
// Note: Default views should not be manually destroyed - they are
// automatically destroyed when the parent texture is destroyed.
// Calling Destroy() on a default view has no effect.
//
// This method is idempotent - calling it multiple times is safe.
func (v *TextureView) Destroy() {
	// Don't allow destroying default views via public API
	if v.isDefault {
		return
	}
	v.destroy()
}

// destroy is the internal destroy method that works for both default and custom views.
func (v *TextureView) destroy() {
	v.mu.Lock()
	if v.destroyed {
		v.mu.Unlock()
		return
	}
	v.destroyed = true
	device := v.device
	halView := v.halView
	v.halView = nil
	v.mu.Unlock()

	if device != nil && halView != nil {
		device.DestroyTextureView(halView)
	}
}

// halViewDescToViewDesc converts a hal.TextureViewDescriptor to TextureViewDescriptor.
func halViewDescToViewDesc(halDesc *hal.TextureViewDescriptor, tex *Texture) TextureViewDescriptor {
	desc := TextureViewDescriptor{
		Label:           halDesc.Label,
		Format:          halDesc.Format,
		Dimension:       halDesc.Dimension,
		Aspect:          halDesc.Aspect,
		BaseMipLevel:    halDesc.BaseMipLevel,
		MipLevelCount:   halDesc.MipLevelCount,
		BaseArrayLayer:  halDesc.BaseArrayLayer,
		ArrayLayerCount: halDesc.ArrayLayerCount,
	}

	// Resolve inherited values
	if desc.Format == types.TextureFormatUndefined {
		desc.Format = tex.Format()
	}
	if desc.Dimension == types.TextureViewDimensionUndefined {
		desc.Dimension = textureViewDimensionFromTexture(tex.Dimension())
	}
	if desc.MipLevelCount == 0 {
		desc.MipLevelCount = tex.MipLevelCount() - desc.BaseMipLevel
	}
	if desc.ArrayLayerCount == 0 {
		desc.ArrayLayerCount = tex.DepthOrArrayLayers() - desc.BaseArrayLayer
	}

	return desc
}

// textureViewDimensionFromTexture returns the default view dimension for a texture dimension.
func textureViewDimensionFromTexture(dim types.TextureDimension) types.TextureViewDimension {
	switch dim {
	case types.TextureDimension1D:
		return types.TextureViewDimension1D
	case types.TextureDimension2D:
		return types.TextureViewDimension2D
	case types.TextureDimension3D:
		return types.TextureViewDimension3D
	default:
		return types.TextureViewDimension2D
	}
}

// =============================================================================
// Device Texture Creation
// =============================================================================

// CreateCoreTexture creates a new texture from a device.
//
// This is a helper function for creating textures using the HAL API directly.
// It handles validation and wraps the texture in a Texture.
//
// Parameters:
//   - device: The device to create the texture on.
//   - desc: The texture descriptor.
//
// Returns the new Texture and nil on success.
// Returns nil and an error if:
//   - The device is nil
//   - The descriptor is nil
//   - Texture dimensions are invalid
//   - Texture creation fails
func CreateCoreTexture(device hal.Device, desc *TextureDescriptor) (*Texture, error) {
	if device == nil {
		return nil, ErrNilHALDevice
	}

	if desc == nil {
		return nil, fmt.Errorf("texture descriptor is nil")
	}

	// Validate dimensions
	if desc.Size.Width == 0 || desc.Size.Height == 0 {
		return nil, fmt.Errorf("%w: width=%d, height=%d",
			ErrInvalidTextureSize, desc.Size.Width, desc.Size.Height)
	}

	// Default values
	mipLevelCount := desc.MipLevelCount
	if mipLevelCount == 0 {
		mipLevelCount = 1
	}

	sampleCount := desc.SampleCount
	if sampleCount == 0 {
		sampleCount = 1
	}

	depthOrArrayLayers := desc.Size.DepthOrArrayLayers
	if depthOrArrayLayers == 0 {
		depthOrArrayLayers = 1
	}

	// Convert to descriptor
	halDesc := &hal.TextureDescriptor{
		Label: desc.Label,
		Size: hal.Extent3D{
			Width:              desc.Size.Width,
			Height:             desc.Size.Height,
			DepthOrArrayLayers: depthOrArrayLayers,
		},
		MipLevelCount: mipLevelCount,
		SampleCount:   sampleCount,
		Dimension:     desc.Dimension,
		Format:        desc.Format,
		Usage:         desc.Usage,
		ViewFormats:   desc.ViewFormats,
	}

	// Create texture
	halTexture, err := device.CreateTexture(halDesc)
	if err != nil {
		return nil, fmt.Errorf("texture creation failed: %w", err)
	}

	// Update descriptor with resolved values
	resolvedDesc := *desc
	resolvedDesc.MipLevelCount = mipLevelCount
	resolvedDesc.SampleCount = sampleCount
	resolvedDesc.Size.DepthOrArrayLayers = depthOrArrayLayers

	return NewTexture(halTexture, device, &resolvedDesc), nil
}

// CreateCoreTextureSimple creates a 2D texture with common defaults.
//
// This is a convenience function for creating simple 2D textures with:
//   - Dimension: 2D
//   - MipLevelCount: 1
//   - SampleCount: 1
//   - DepthOrArrayLayers: 1
//
// Parameters:
//   - device: The device to create the texture on.
//   - width: Texture width in pixels.
//   - height: Texture height in pixels.
//   - format: Texture pixel format.
//   - usage: Texture usage flags.
//   - label: Optional debug label.
//
// Returns the new Texture and nil on success.
// Returns nil and an error if creation fails.
func CreateCoreTextureSimple(
	device hal.Device,
	width, height uint32,
	format types.TextureFormat,
	usage types.TextureUsage,
	label string,
) (*Texture, error) {
	desc := &TextureDescriptor{
		Label: label,
		Size: types.Extent3D{
			Width:              width,
			Height:             height,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     types.TextureDimension2D,
		Format:        format,
		Usage:         usage,
	}

	return CreateCoreTexture(device, desc)
}
