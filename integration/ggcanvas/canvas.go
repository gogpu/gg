// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package ggcanvas

import (
	"errors"
	"fmt"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
)

// Common errors returned by Canvas operations.
var (
	// ErrCanvasClosed is returned when operations are attempted on a closed canvas.
	ErrCanvasClosed = errors.New("ggcanvas: canvas is closed")

	// ErrInvalidDimensions is returned when width or height is invalid.
	ErrInvalidDimensions = errors.New("ggcanvas: invalid dimensions")

	// ErrNilProvider is returned when a nil DeviceProvider is passed.
	ErrNilProvider = errors.New("ggcanvas: nil DeviceProvider")

	// ErrTextureCreationFailed is returned when texture creation fails.
	ErrTextureCreationFailed = errors.New("ggcanvas: texture creation failed")
)

// textureDestroyer is the interface for destroying textures.
// This matches the gogpu.Texture.Destroy signature.
type textureDestroyer interface {
	Destroy()
}

// Canvas wraps gg.Context with gogpu integration.
// It manages the CPU-to-GPU pipeline automatically.
//
// Canvas is NOT safe for concurrent use. Create one Canvas per goroutine,
// or use external synchronization.
type Canvas struct {
	ctx         *gg.Context
	provider    gpucontext.DeviceProvider
	texture     any  // Lazy-created texture (*gogpu.Texture)
	oldTexture  any  // Previous texture awaiting deferred destruction
	dirty       bool // Needs GPU upload
	sizeChanged bool // Resize pending — texture must be recreated
	width       int
	height      int
	closed      bool
}

// New creates a Canvas for integrated mode.
// The provider should come from gogpu.App.GPUContextProvider().
//
// The Canvas is created with default gg.Context settings.
// Use Context() to access and configure the drawing context.
//
// Returns error if dimensions are invalid or provider is nil.
func New(provider gpucontext.DeviceProvider, width, height int) (*Canvas, error) {
	if provider == nil {
		return nil, ErrNilProvider
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("%w: width=%d, height=%d", ErrInvalidDimensions, width, height)
	}

	// Share GPU device with accelerator if registered.
	// Error is non-fatal: accelerator may not support device sharing or
	// provider may not implement HalProvider. GPU will initialize its own device.
	_ = gg.SetAcceleratorDeviceProvider(provider)

	return &Canvas{
		ctx:      gg.NewContext(width, height),
		provider: provider,
		width:    width,
		height:   height,
		dirty:    true, // Mark dirty so first Flush creates texture
	}, nil
}

// MustNew is like New but panics on error.
// Use only when errors are programming mistakes (e.g., hardcoded dimensions).
func MustNew(provider gpucontext.DeviceProvider, width, height int) *Canvas {
	c, err := New(provider, width, height)
	if err != nil {
		panic(err)
	}
	return c
}

// Context returns the gg drawing context.
// All gg drawing methods are available through this context.
//
// After drawing, call MarkDirty() to flag the canvas for GPU upload,
// or call Flush() which handles this automatically.
//
// Returns nil if the canvas is closed.
func (c *Canvas) Context() *gg.Context {
	if c.closed {
		return nil
	}
	return c.ctx
}

// Width returns the canvas width in pixels.
func (c *Canvas) Width() int {
	return c.width
}

// Height returns the canvas height in pixels.
func (c *Canvas) Height() int {
	return c.height
}

// Size returns width and height as a convenience.
func (c *Canvas) Size() (width, height int) {
	return c.width, c.height
}

// MarkDirty flags the canvas for GPU upload on next Flush().
// Call this after drawing operations if you want explicit control
// over when uploads happen.
func (c *Canvas) MarkDirty() {
	c.dirty = true
}

// Draw calls fn with the gg context and marks the canvas as dirty.
// This is the recommended way to update canvas content, as it ensures
// the dirty flag is set correctly for GPU upload on next Flush/RenderTo.
func (c *Canvas) Draw(fn func(*gg.Context)) error {
	if c.closed {
		return ErrCanvasClosed
	}
	fn(c.ctx)
	c.dirty = true
	return nil
}

// IsDirty returns true if the canvas has pending changes
// that need to be uploaded to the GPU.
func (c *Canvas) IsDirty() bool {
	return c.dirty
}

// Resize changes canvas dimensions.
// This recreates internal buffers and clears the canvas.
//
// Returns error if dimensions are invalid or canvas is closed.
func (c *Canvas) Resize(width, height int) error {
	if c.closed {
		return ErrCanvasClosed
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: width=%d, height=%d", ErrInvalidDimensions, width, height)
	}

	// No-op if dimensions haven't changed
	if c.width == width && c.height == height {
		return nil
	}

	// Resize gg context
	if err := c.ctx.Resize(width, height); err != nil {
		return fmt.Errorf("ggcanvas: context resize failed: %w", err)
	}

	c.width = width
	c.height = height
	c.sizeChanged = true
	c.dirty = true

	return nil
}

// Flush uploads the canvas content to GPU texture if dirty.
// Returns the texture for manual drawing if needed.
//
// The texture is created lazily on first Flush().
// Subsequent calls only upload data if dirty flag is set.
//
// Returns error if texture creation or update fails, or if canvas is closed.
func (c *Canvas) Flush() (any, error) {
	if c.closed {
		return nil, ErrCanvasClosed
	}

	// If size changed, defer old texture destruction until after GPU is idle.
	// The old texture may still be referenced by in-flight GPU command buffers.
	// Destroying it now would free descriptor heap entries that the GPU is reading,
	// causing it to sample zeros (transparent). Instead, keep it alive and destroy
	// it in RenderToEx after WriteTexture (which calls waitForGPU internally).
	if c.sizeChanged {
		if c.texture != nil {
			// Destroy any previously deferred texture first
			if c.oldTexture != nil {
				if destroyer, ok := c.oldTexture.(textureDestroyer); ok {
					destroyer.Destroy()
				}
			}
			c.oldTexture = c.texture
			c.texture = nil
		}
		c.sizeChanged = false
	}

	// Skip if not dirty
	if !c.dirty && c.texture != nil {
		return c.texture, nil
	}

	// Flush pending GPU shapes to pixel buffer before reading pixel data.
	// Errors are logged but not fatal — CPU fallback may have already rendered.
	if err := c.ctx.FlushGPU(); err != nil {
		// FlushGPU can fail if GPU accelerator has issues (e.g., compute dispatch failure).
		// This is non-fatal: CPU-rendered content is still in the pixmap.
		_ = err
	}

	// Get pixel data from gg context
	pixmap := c.ctx.ResizeTarget()
	data := pixmap.Data()

	// Create texture if needed (lazy initialization)
	if c.texture == nil {
		c.texture = c.createTexture(data)
		c.dirty = false
		return c.texture, nil
	}

	// Update existing texture
	if updater, ok := c.texture.(gpucontext.TextureUpdater); ok {
		if err := updater.UpdateData(data); err != nil {
			return nil, fmt.Errorf("ggcanvas: texture update failed: %w", err)
		}
	}

	c.dirty = false
	return c.texture, nil
}

// Texture returns the current GPU texture without flushing.
// Returns nil if texture hasn't been created yet.
//
// Use Flush() to ensure the texture exists and is up-to-date.
func (c *Canvas) Texture() any {
	return c.texture
}

// Close releases all resources associated with the Canvas.
// After Close, the Canvas should not be used.
// Close is idempotent - multiple calls are safe.
func (c *Canvas) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true

	// Destroy textures (current and any deferred old texture)
	if c.oldTexture != nil {
		if destroyer, ok := c.oldTexture.(textureDestroyer); ok {
			destroyer.Destroy()
		}
		c.oldTexture = nil
	}
	if c.texture != nil {
		if destroyer, ok := c.texture.(textureDestroyer); ok {
			destroyer.Destroy()
		}
		c.texture = nil
	}

	// Close gg context
	if c.ctx != nil {
		_ = c.ctx.Close()
		c.ctx = nil
	}

	c.provider = nil
	return nil
}

// createTexture creates a pending texture placeholder from pixel data.
// This is called lazily on first Flush().
// The actual GPU texture is created during RenderTo when a renderer is available.
func (c *Canvas) createTexture(data []byte) *pendingTexture {
	// We store the creation request and let RenderTo handle it
	// when it has access to the actual renderer.
	//
	// This is a limitation: texture can only be created during RenderTo.
	// Alternative designs:
	// 1. Pass textureCreator to New()
	// 2. Extend DeviceProvider to include texture creation
	// 3. Store data and create texture on-demand in RenderTo
	//
	// We choose option 3: store a placeholder and create in RenderTo.
	return &pendingTexture{
		width:  c.width,
		height: c.height,
		data:   data,
	}
}

// pendingTexture is a placeholder for texture creation.
// It holds the data needed to create a real texture when we have
// access to a textureCreator (during RenderTo).
type pendingTexture struct {
	width  int
	height int
	data   []byte
}

// Provider returns the DeviceProvider associated with this canvas.
// Returns nil if the canvas is closed.
func (c *Canvas) Provider() gpucontext.DeviceProvider {
	if c.closed {
		return nil
	}
	return c.provider
}
