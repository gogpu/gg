// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package ggcanvas

import (
	"errors"
	"fmt"
	"image"
	"io"

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

// resourceTracker is a duck-typed interface matching gogpu.ResourceTracker.
// Using a local interface avoids importing gogpu (which would create a
// circular dependency gg -> gogpu). Go's structural typing ensures that
// gogpu.App satisfies this interface without explicit declaration.
type resourceTracker interface {
	TrackResource(io.Closer)
	UntrackResource(io.Closer)
}

// Canvas wraps gg.Context with gogpu integration.
// It manages the CPU-to-GPU pipeline automatically.
//
// Canvas is NOT safe for concurrent use. Create one Canvas per goroutine,
// or use external synchronization.
type Canvas struct {
	ctx          *gg.Context
	provider     gpucontext.DeviceProvider
	texture      any             // Lazy-created texture (*gogpu.Texture)
	oldTexture   any             // Previous texture awaiting deferred destruction
	dirty        bool            // Needs GPU upload
	dirtyRect    image.Rectangle // Accumulated dirty region (zero = full upload)
	regionBuf    []byte          // Reusable buffer for partial texture upload
	sizeChanged  bool            // Resize pending — texture must be recreated
	width        int
	height       int
	closed       bool
	tracked      bool               // true if auto-registered with a ResourceTracker
	damageFlashs damageOverlayState // debug overlay fade state

	// presentDamageRects holds damage rects for the next present call (ADR-021 Phase 4).
	// Set by caller (e.g. ui retained tree) via SetPresentDamage().
	// Consumed and cleared by Render() after forwarding to SetDamageRects().
	presentDamageRects []image.Rectangle
}

// New creates a Canvas for integrated mode.
// The provider should come from gogpu.App.GPUContextProvider().
// The width and height are logical dimensions.
//
// If the provider also implements gpucontext.WindowProvider, the device
// scale is auto-detected for HiDPI/Retina support. Otherwise defaults to 1.0.
// Use Context() to access and configure the drawing context.
//
// Returns error if dimensions are invalid or provider is nil.
func New(provider gpucontext.DeviceProvider, width, height int) (*Canvas, error) {
	scale := 1.0
	if wp, ok := provider.(gpucontext.WindowProvider); ok {
		if s := wp.ScaleFactor(); s > 0 {
			scale = s
		}
	}
	return NewWithScale(provider, width, height, scale)
}

// NewWithScale creates a Canvas with HiDPI device scale support.
// The width and height are logical dimensions. The internal pixmap is
// allocated at physical resolution (width*scale x height*scale).
//
// The provider should come from gogpu.App.GPUContextProvider().
// Scale factor should come from the platform (e.g., gogpu.Context.ScaleFactor()).
// Typical values: 1.0 (standard), 2.0 (macOS Retina), 3.0 (mobile HiDPI).
//
// Example:
//
//	scale := dc.ScaleFactor()  // from gogpu.Context
//	canvas, err := ggcanvas.NewWithScale(provider, 800, 600, scale)
//
// Returns error if dimensions are invalid, provider is nil, or scale <= 0.
func NewWithScale(provider gpucontext.DeviceProvider, width, height int, scale float64) (*Canvas, error) {
	if provider == nil {
		return nil, ErrNilProvider
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("%w: width=%d, height=%d", ErrInvalidDimensions, width, height)
	}
	if scale <= 0 {
		scale = 1.0
	}

	physW := int(float64(width) * scale)
	physH := int(float64(height) * scale)
	gg.Logger().Info("ggcanvas.NewWithScale",
		"logical_w", width, "logical_h", height,
		"scale", scale,
		"physical_w", physW, "physical_h", physH,
	)

	// Share GPU device with accelerator if registered.
	// Error is non-fatal: accelerator may not support device sharing or
	// provider may not expose HAL types. GPU will initialize its own device.
	if err := gg.SetAcceleratorDeviceProvider(provider); err != nil {
		gg.Logger().Warn("SetAcceleratorDeviceProvider failed", "err", err)
	}

	var opts []gg.ContextOption
	if scale != 1.0 {
		opts = append(opts, gg.WithDeviceScale(scale))
	}

	c := &Canvas{
		ctx:      gg.NewContext(width, height, opts...),
		provider: provider,
		width:    width,
		height:   height,
		dirty:    true, // Mark dirty so first Flush creates texture
	}

	// Auto-detect LCD subpixel layout from platform (ADR-024).
	// PlatformProvider exposes OS-level display properties; SubpixelLayout
	// enables ClearType rendering matching native Windows DirectWrite quality.
	if pp, ok := provider.(gpucontext.PlatformProvider); ok {
		switch pp.SubpixelLayout() {
		case gpucontext.SubpixelRGB:
			c.ctx.SetLCDLayout(gg.LCDLayoutRGB)
		case gpucontext.SubpixelBGR:
			c.ctx.SetLCDLayout(gg.LCDLayoutBGR)
		}
	}

	// Auto-register with ResourceTracker if the provider supports it.
	// This enables automatic cleanup on application shutdown without
	// requiring manual OnClose callbacks.
	if tracker, ok := provider.(resourceTracker); ok {
		tracker.TrackResource(c)
		c.tracked = true
	}

	return c, nil
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

// MustNewWithScale is like NewWithScale but panics on error.
func MustNewWithScale(provider gpucontext.DeviceProvider, width, height int, scale float64) *Canvas {
	c, err := NewWithScale(provider, width, height, scale)
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

// Width returns the canvas logical width.
func (c *Canvas) Width() int {
	return c.width
}

// Height returns the canvas logical height.
func (c *Canvas) Height() int {
	return c.height
}

// Size returns logical width and height as a convenience.
func (c *Canvas) Size() (width, height int) {
	return c.width, c.height
}

// DeviceScale returns the current device scale factor.
// Returns 1.0 if the canvas was created without HiDPI support.
func (c *Canvas) DeviceScale() float64 {
	if c.ctx == nil {
		return 1.0
	}
	return c.ctx.DeviceScale()
}

// PixmapTextureView returns the GPU texture view of the uploaded pixmap.
// Returns nil if the pixmap has not been uploaded yet (call FlushPixmap first)
// or if the texture does not expose a view (e.g., pendingTexture before promotion).
//
// Use this with DrawGPUTextureBase for single-pass zero-readback compositing:
//
//	canvas.FlushPixmap()                          // upload, no GPU readback
//	view := canvas.PixmapTextureView()            // get GPU texture view
//	cc.DrawGPUTextureBase(view, 0, 0, w, h)       // base layer
//	cc.FlushGPUWithView(surfaceView, sw, sh)      // single pass compositor
//
// The view is valid until the texture is destroyed (resize, close).
// Uses Go structural typing — no gogpu import required.
func (c *Canvas) PixmapTextureView() gpucontext.TextureView {
	if c.texture == nil {
		return gpucontext.TextureView{}
	}
	type viewProvider interface {
		TextureView() gpucontext.TextureView
	}
	if vp, ok := c.texture.(viewProvider); ok {
		return vp.TextureView()
	}
	return gpucontext.TextureView{}
}

// SetDeviceScale changes the device scale factor on the canvas.
// This delegates to the gg.Context and marks the canvas for re-upload.
// Scale must be > 0; values <= 0 are ignored.
func (c *Canvas) SetDeviceScale(scale float64) {
	if c.closed || c.ctx == nil || scale <= 0 {
		return
	}
	if scale == c.ctx.DeviceScale() {
		return
	}
	c.ctx.SetDeviceScale(scale)
	c.sizeChanged = true
	c.dirty = true
}

// MarkDirty flags the canvas for GPU upload on next Flush().
// Call this after drawing operations if you want explicit control
// over when uploads happen.
//
// MarkDirty invalidates the entire canvas. For partial invalidation,
// use MarkDirtyRegion to upload only the changed region.
func (c *Canvas) MarkDirty() {
	c.dirty = true
	c.dirtyRect = image.Rect(0, 0, c.ctx.Width(), c.ctx.Height())
}

// LastDamage returns the damage rectangle (union) from the most recent frame.
func (c *Canvas) LastDamage() image.Rectangle {
	return c.dirtyRect
}

// LastDamageRects returns individual damage rectangles from the most recent frame.
func (c *Canvas) LastDamageRects() []image.Rectangle {
	return c.ctx.FrameDamage()
}

// NeedsAnimationFrame reports whether the canvas needs another frame
// for debug overlay fade animation. Caller should RequestRedraw if true.
func (c *Canvas) NeedsAnimationFrame() bool {
	return c.damageFlashs.needsAnimationFrame()
}

// SetPresentDamage sets damage rectangles for the next present call (ADR-021 Level 4).
// Rects are in physical pixels with top-left origin. They are forwarded to
// gogpu SetDamageRects() → wgpu PresentWithDamage() → OS compositor hint
// (VK_KHR_incremental_present, DX12 Present1, eglSwapBuffersWithDamage).
//
// Callers with retained-mode knowledge (e.g. ui widget tree) should provide
// BOTH old and new bounds of moved/resized objects. Immediate-mode callers
// can pass FrameDamage() rects (new positions only) when old positions are
// covered by full-surface redraw.
//
// Rects are consumed after one present and do not persist across frames.
// When nil or empty, the full surface is presented (backward compatible).
func (c *Canvas) SetPresentDamage(rects []image.Rectangle) {
	c.presentDamageRects = rects
}

// forwardDamageRects sends damage rects to the OS compositor via SetDamageRects
// (ADR-021 Level 4). Uses explicit rects from SetPresentDamage if available,
// otherwise falls back to immediate-mode FrameDamage rects.
// Clears presentDamageRects after forwarding (one-shot per frame).
func (c *Canvas) forwardDamageRects(dc RenderTarget, frameDamage []image.Rectangle) {
	setter, ok := dc.(DamageRectSetter)
	if !ok {
		c.presentDamageRects = nil
		return
	}
	rects := c.presentDamageRects
	if len(rects) == 0 {
		rects = frameDamage
	}
	if len(rects) > 0 {
		setter.SetDamageRects(rects)
	}
	c.presentDamageRects = nil
}

// MarkDirtyRegion flags a rectangular region of the canvas as dirty.
// On the next Flush(), only the accumulated dirty region is uploaded
// to the GPU (if the texture supports partial upload), which can be
// significantly faster than uploading the entire pixmap.
//
// Multiple calls accumulate into the bounding rectangle of all dirty regions.
// The region is in physical pixel coordinates (after device scale).
func (c *Canvas) MarkDirtyRegion(r image.Rectangle) {
	if r.Empty() {
		return
	}
	if c.dirtyRect.Empty() {
		c.dirtyRect = r
	} else {
		c.dirtyRect = c.dirtyRect.Union(r)
	}
	c.dirty = true
}

// Draw calls fn with the gg context and marks the canvas as dirty.
// This is the recommended way to update canvas content, as it ensures
// the dirty flag is set correctly for GPU upload on next Flush/RenderTo.
//
// BeginAcceleratorFrame is called before fn to reset per-frame GPU state.
// This ensures the first render pass clears the surface while mid-frame
// CPU fallback flushes (bitmap text, gradient fill) use LoadOpLoad to
// preserve previously drawn content. See RENDER-DIRECT-003.
func (c *Canvas) Draw(fn func(*gg.Context)) error {
	if c.closed {
		return ErrCanvasClosed
	}
	gg.BeginAcceleratorFrame()
	fn(c.ctx)
	c.MarkDirty()
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
// This first calls FlushGPU() to render any pending GPU-accelerated shapes
// (SDF, stencil, text) back into the CPU pixmap, then uploads the pixmap
// to a GPU texture. For zero-readback rendering, use FlushPixmap() instead.
//
// The texture is created lazily on first Flush().
// Subsequent calls only upload data if dirty flag is set.
//
// Returns error if texture creation or update fails, or if canvas is closed.
func (c *Canvas) Flush() (any, error) {
	if c.closed {
		return nil, ErrCanvasClosed
	}

	// Flush pending GPU shapes to pixel buffer before reading pixel data.
	// Errors are logged but not fatal — CPU fallback may have already rendered.
	if err := c.ctx.FlushGPU(); err != nil {
		gg.Logger().Warn("FlushGPU error", "err", err)
	}

	return c.FlushPixmap()
}

// FlushPixmap uploads the CPU pixmap to GPU texture without flushing GPU shapes.
// Unlike Flush(), this does NOT call FlushGPU() — pending GPU-accelerated shapes
// remain queued in GPURenderContext for the caller to flush separately (e.g., via
// FlushGPUWithView for zero-readback rendering to a surface view).
//
// Use this when GPU shapes should render directly to the display surface
// instead of being read back into the CPU pixmap. See ADR-006.
func (c *Canvas) FlushPixmap() (any, error) {
	if c.closed {
		return nil, ErrCanvasClosed
	}

	if c.sizeChanged {
		c.deferTextureDestruction()
		c.sizeChanged = false
	}

	if !c.dirty && c.texture != nil {
		return c.texture, nil
	}

	pixmap := c.ctx.ResizeTarget()
	data := pixmap.Data()

	if c.texture == nil {
		c.texture = c.createTexture(data)
		c.dirty = false
		c.dirtyRect = image.Rectangle{}
		return c.texture, nil
	}

	if err := c.uploadTexture(pixmap, data); err != nil {
		return nil, err
	}

	c.dirty = false
	c.dirtyRect = image.Rectangle{}
	return c.texture, nil
}

// RenderDirect renders canvas content directly to the given surface view,
// bypassing the GPU->CPU->GPU readback. This is the zero-copy rendering path
// for use with gogpu's surface texture view.
//
// When the GPU accelerator supports direct surface rendering, shapes are
// rendered directly to the provided surface view via MSAA resolve. No
// staging buffers, no ReadBuffer, no texture upload -- pure GPU-to-GPU.
//
// If the accelerator doesn't support surface rendering, or if no GPU
// accelerator is registered, this method falls back to the readback path
// via Flush().
//
// The surfaceView is a type-safe opaque handle obtained from
// dc.RenderTarget().SurfaceView(). Pass a zero-value (IsNil() == true)
// to use the readback path.
//
// Example:
//
//	app.OnDraw(func(dc *gogpu.Context) {
//	    canvas.Draw(func(cc *gg.Context) { ... })
//	    w, h := dc.SurfaceSize()
//	    canvas.RenderDirect(dc.RenderTarget().SurfaceView(), w, h)
//	})
func (c *Canvas) RenderDirect(surfaceView gpucontext.TextureView, width, height uint32) error {
	if c.closed {
		return ErrCanvasClosed
	}
	if surfaceView.IsNil() {
		return nil
	}
	if !c.dirty {
		return nil
	}

	gg.Logger().Debug("ggcanvas.RenderDirect",
		"width", width, "height", height,
		"hasSurfaceView", !surfaceView.IsNil(),
	)

	// Flush GPU shapes directly to the surface view (no readback).
	// FlushGPUWithView passes the view through GPURenderTarget.View,
	// which takes priority over session-level surfaceView in the
	// render session's routing logic.
	//
	// NOTE: BeginAcceleratorFrame is NOT called here -- it must be called
	// BEFORE canvas.Draw(), not after. If Draw triggers a mid-frame CPU
	// fallback (bitmap text, gradient fill), flushGPUAccelerator submits
	// GPU commands with LoadOpClear. Calling BeginFrame here would reset
	// frameRendered=false, causing the final FlushGPU to wipe that content
	// with a second LoadOpClear. See RENDER-DIRECT-003.
	err := c.ctx.FlushGPUWithView(surfaceView, width, height)

	c.dirty = false
	c.dirtyRect = image.Rectangle{}
	return err
}

// RenderDirectWithDamage renders canvas content to a surface view with a damage
// rect hint. Only the damaged region is re-rendered; the rest preserves the
// previous frame (LoadOpLoad + scissor). This enables per-boundary incremental
// updates without full-frame re-render.
//
// Use when only a subset of GPU content changed (e.g., a single RepaintBoundary
// item in a compositor). Pass image.Rectangle{} for full-frame render.
func (c *Canvas) RenderDirectWithDamage(surfaceView gpucontext.TextureView, width, height uint32, damage image.Rectangle) error {
	if c.closed {
		return ErrCanvasClosed
	}
	if surfaceView.IsNil() {
		return nil
	}
	if !c.dirty {
		return nil
	}

	err := c.ctx.FlushGPUWithViewDamage(surfaceView, width, height, damage)
	c.dirty = false
	c.dirtyRect = image.Rectangle{}
	return err
}

// RenderDirectWithDamageRects renders with multiple damage rects (ADR-028).
// Each rect gets its own scissor — per-draw dynamic scissor for distant dirty
// regions. Falls back to single-rect behavior when len(rects) <= 1.
func (c *Canvas) RenderDirectWithDamageRects(surfaceView gpucontext.TextureView, width, height uint32, rects []image.Rectangle) error {
	if c.closed {
		return ErrCanvasClosed
	}
	if surfaceView.IsNil() {
		return nil
	}
	if !c.dirty {
		return nil
	}

	err := c.ctx.FlushGPUWithViewDamageRects(surfaceView, width, height, rects)
	c.dirty = false
	c.dirtyRect = image.Rectangle{}
	return err
}

// RenderTarget is the interface for presenting canvas content on screen.
// Implement this on your application context. *gogpu.Context satisfies this
// via the gogpu.RenderTarget() adapter.
type RenderTarget interface {
	SurfaceView() gpucontext.TextureView
	SurfaceSize() (uint32, uint32)
	PresentTexture(tex any) error
}

// DamageRectSetter is an optional interface for RenderTargets that support
// damage-aware presentation (ADR-021 Level 3-4). gogpu.ContextRenderTarget
// implements this via Context.SetDamageRects().
type DamageRectSetter interface {
	SetDamageRects(rects []image.Rectangle)
}

// Render presents canvas content to the screen. Works on all backends.
//
// On GPU backends (Vulkan, DX12, Metal, GLES): renders directly to surface
// via GPU shaders (zero-copy, optimal performance).
//
// On software backend or when GPU-direct fails: falls back to universal path
// where gg CPU rasterizer renders to pixmap, uploads to texture, and presents
// via textured quad.
//
// This is the recommended way to present canvas content — one call, all backends.
//
//	canvas.Draw(func(cc *gg.Context) { ... })
//	canvas.Render(dc) // dc is *gogpu.Context
func (c *Canvas) Render(dc RenderTarget) error {
	if c.closed {
		return ErrCanvasClosed
	}
	if !c.dirty {
		return nil
	}

	// Collect per-frame damage rects BEFORE GPU-direct path attempt.
	// Damage overlay needs these regardless of which present path is used.
	damageRects := c.ctx.FrameDamage()
	c.ctx.ResetFrameDamage()

	// Debug damage overlay (ADR-021 Phase 6a).
	// Draw overlay BEFORE present so it's visible on ALL backends.
	// Android SurfaceFlinger pattern: flash-and-fade on dirty regions.
	if isDebugDamageEnabled() {
		c.damageFlashs.update(damageRects)
		if len(c.damageFlashs.flashes) > 0 {
			c.ctx.SetDamageTracking(false)
			c.damageFlashs.drawAll(c.ctx)
			c.ctx.SetDamageTracking(true)
		}
	}

	// Try GPU-direct path (zero-copy surface rendering).
	// Only attempt if the accelerator is actually capable — on CPU-only
	// adapters (llvmpipe, SwiftShader) the accelerator stays uninitialized
	// and RenderDirect would silently succeed without rendering anything.
	sv := dc.SurfaceView()
	if !sv.IsNil() && gg.AcceleratorCanRenderDirect() {
		sw, sh := dc.SurfaceSize()
		if err := c.RenderDirect(sv, sw, sh); err == nil {
			c.forwardDamageRects(dc, damageRects)
			return nil
		}
	}

	// Universal path: CPU rasterizer → pixmap → texture → present.
	tex, err := c.Flush()
	if err != nil {
		return err
	}

	// Promote pendingTexture to real GPU texture if needed.
	// Flush() returns pendingTexture on first call (lazy creation).
	tex, err = c.promoteIfPending(tex, dc)
	if err != nil {
		return err
	}

	c.forwardDamageRects(dc, damageRects)

	return dc.PresentTexture(tex)
}

// EnsureGPUTexture promotes the internal pendingTexture to a real GPU texture
// if needed. After this call, PixmapTextureView() returns non-nil.
//
// Call this once after the first FlushPixmap() to create the GPU texture.
// Subsequent calls are no-ops (texture already promoted). The RenderTarget
// provides TextureCreator for GPU texture allocation.
//
// This is the setup step for zero-readback compositing:
//
//	canvas.FlushPixmap()                    // upload pixmap (no GPU readback)
//	canvas.EnsureGPUTexture(dc.RenderTarget()) // promote once
//	view := canvas.PixmapTextureView()      // now non-nil
//	cc.DrawGPUTextureBase(view, ...)        // base layer
//	cc.FlushGPUWithView(surface, ...)       // single pass
func (c *Canvas) EnsureGPUTexture(dc RenderTarget) error {
	if c.texture == nil || c.closed {
		return nil
	}
	if _, ok := c.texture.(*pendingTexture); !ok {
		return nil
	}
	promoted, err := c.promoteIfPending(c.texture, dc)
	if err != nil {
		return err
	}
	c.texture = promoted
	return nil
}

// promoteIfPending promotes a pendingTexture to a real GPU texture if needed.
// Returns the texture unchanged if it is not pending.
func (c *Canvas) promoteIfPending(tex any, dc RenderTarget) (any, error) {
	if _, ok := tex.(*pendingTexture); !ok {
		return tex, nil
	}
	type textureCreatorProvider interface {
		TextureCreator() gpucontext.TextureCreator
	}
	tcp, ok := dc.(textureCreatorProvider)
	if !ok {
		return nil, fmt.Errorf("ggcanvas: RenderTarget does not provide TextureCreator, cannot promote pending texture")
	}
	creator := tcp.TextureCreator()
	if creator == nil {
		return nil, ErrInvalidRenderer
	}
	pending := tex.(*pendingTexture)
	realTex, err := creator.NewTextureFromRGBA(pending.width, pending.height, pending.data)
	if err != nil {
		return nil, fmt.Errorf("ggcanvas: NewTextureFromRGBA failed: %w", err)
	}
	if pt, ok := realTex.(interface{ SetPremultiplied(bool) }); ok {
		pt.SetPremultiplied(true)
	}
	c.texture = realTex
	destroyTexture(c.oldTexture)
	c.oldTexture = nil
	return realTex, nil
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

	// Untrack from ResourceTracker if auto-registered, to prevent double-close.
	if c.tracked {
		if tracker, ok := c.provider.(resourceTracker); ok {
			tracker.UntrackResource(c)
		}
		c.tracked = false
	}

	// Note: no need to clear surface target — per-pass View routing handles
	// target selection. Session-level surfaceView is no longer set.

	// Destroy textures (current and any deferred old texture).
	destroyTexture(c.oldTexture)
	c.oldTexture = nil
	destroyTexture(c.texture)
	c.texture = nil

	// Close gg context
	if c.ctx != nil {
		_ = c.ctx.Close()
		c.ctx = nil
	}

	c.provider = nil
	return nil
}

// uploadTexture uploads pixmap data to the existing texture. When the texture
// supports partial region upload and a specific dirty rect is set, only the
// dirty region is extracted and uploaded. Otherwise falls back to full upload.
func (c *Canvas) uploadTexture(pixmap *gg.Pixmap, fullData []byte) error {
	dr := c.dirtyRect
	regionUpdater, hasRegion := c.texture.(gpucontext.TextureRegionUpdater)

	// Use partial upload when: texture supports it, dirty rect is set (non-empty),
	// and the dirty rect is strictly smaller than the full pixmap.
	if hasRegion && !dr.Empty() {
		// Clamp dirty rect to pixmap bounds.
		bounds := image.Rect(0, 0, pixmap.Width(), pixmap.Height())
		dr = dr.Intersect(bounds)
		if !dr.Empty() && dr != bounds {
			regionData := c.extractRegion(fullData, pixmap.Width(), dr)
			if err := regionUpdater.UpdateRegion(dr.Min.X, dr.Min.Y, dr.Dx(), dr.Dy(), regionData); err != nil {
				return fmt.Errorf("ggcanvas: region update failed: %w", err)
			}
			return nil
		}
	}

	// Full upload fallback.
	if updater, ok := c.texture.(gpucontext.TextureUpdater); ok {
		if err := updater.UpdateData(fullData); err != nil {
			return fmt.Errorf("ggcanvas: texture update failed: %w", err)
		}
	}
	return nil
}

// extractRegion copies a rectangular sub-region from RGBA row-major pixel data
// into a densely packed buffer suitable for UpdateRegion.
// Reuses c.regionBuf to avoid allocation on the 60fps hot path.
func (c *Canvas) extractRegion(data []byte, pixmapWidth int, r image.Rectangle) []byte {
	const bytesPerPixel = 4
	stride := pixmapWidth * bytesPerPixel
	regionW := r.Dx() * bytesPerPixel
	needed := r.Dx() * r.Dy() * bytesPerPixel
	if cap(c.regionBuf) < needed {
		c.regionBuf = make([]byte, needed)
	}
	buf := c.regionBuf[:needed]
	dst := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		srcStart := y*stride + r.Min.X*bytesPerPixel
		copy(buf[dst:dst+regionW], data[srcStart:srcStart+regionW])
		dst += regionW
	}
	return buf
}

// deferTextureDestruction moves the current texture to oldTexture so it can
// be destroyed later (after the GPU is idle). Any previously deferred texture
// is destroyed immediately.
func (c *Canvas) deferTextureDestruction() {
	if c.texture == nil {
		return
	}
	destroyTexture(c.oldTexture)
	c.oldTexture = c.texture
	c.texture = nil
}

// destroyTexture destroys a texture if it implements the textureDestroyer
// interface. Safe to call with nil.
func destroyTexture(tex any) {
	if tex == nil {
		return
	}
	if d, ok := tex.(textureDestroyer); ok {
		d.Destroy()
	}
}

// createTexture creates a pending texture placeholder from pixel data.
// This is called lazily on first Flush().
// The actual GPU texture is created during RenderTo when a renderer is available.
// Uses physical pixel dimensions (PixelWidth/PixelHeight) for the texture.
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
	// Use physical pixel dimensions since the pixmap is at physical resolution.
	return &pendingTexture{
		width:  c.ctx.PixelWidth(),
		height: c.ctx.PixelHeight(),
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
