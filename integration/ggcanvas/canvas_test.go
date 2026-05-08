// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package ggcanvas

import (
	"errors"
	"image"
	"testing"

	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
)

// mockDevice implements gpucontext.Device for testing.
type mockDevice struct{}

func (m *mockDevice) Poll(wait bool) {}
func (m *mockDevice) Destroy()       {}

// mockQueue implements gpucontext.Queue for testing.
type mockQueue struct{}

// mockAdapter implements gpucontext.Adapter for testing.
type mockAdapter struct{}

// mockProvider implements gpucontext.DeviceProvider for testing.
type mockProvider struct {
	device  gpucontext.Device
	queue   gpucontext.Queue
	adapter gpucontext.Adapter
	format  gputypes.TextureFormat
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		device:  &mockDevice{},
		queue:   &mockQueue{},
		adapter: &mockAdapter{},
		format:  gputypes.TextureFormatBGRA8Unorm,
	}
}

func (m *mockProvider) Device() gpucontext.Device             { return m.device }
func (m *mockProvider) Queue() gpucontext.Queue               { return m.queue }
func (m *mockProvider) Adapter() gpucontext.Adapter           { return m.adapter }
func (m *mockProvider) SurfaceFormat() gputypes.TextureFormat { return m.format }
func (m *mockProvider) AdapterInfo() gpucontext.AdapterInfo {
	return gpucontext.AdapterInfo{Type: gpucontext.AdapterTypeUnknown}
}

// regionUpdate records the parameters of a single UpdateRegion call.
type regionUpdate struct {
	x, y, w, h int
	data       []byte
}

// mockTexture implements the texture interfaces for testing.
// Implements gpucontext.Texture, gpucontext.TextureUpdater, and gpucontext.TextureRegionUpdater.
type mockTexture struct {
	width         int
	height        int
	data          []byte
	destroyed     bool
	updated       int
	regionUpdates []regionUpdate
}

func (m *mockTexture) Width() int  { return m.width }
func (m *mockTexture) Height() int { return m.height }

func (m *mockTexture) UpdateData(data []byte) error {
	m.data = make([]byte, len(data))
	copy(m.data, data)
	m.updated++
	return nil
}

func (m *mockTexture) UpdateRegion(x, y, w, h int, data []byte) error {
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	m.regionUpdates = append(m.regionUpdates, regionUpdate{x: x, y: y, w: w, h: h, data: dataCopy})
	return nil
}

func (m *mockTexture) Destroy() {
	m.destroyed = true
}

// Compile-time checks.
var (
	_ gpucontext.TextureUpdater       = (*mockTexture)(nil)
	_ gpucontext.TextureRegionUpdater = (*mockTexture)(nil)
)

// mockRenderer implements gpucontext.TextureCreator for testing.
type mockRenderer struct {
	textures []*mockTexture
	failNext bool
}

func (m *mockRenderer) NewTextureFromRGBA(width, height int, data []byte) (gpucontext.Texture, error) {
	if m.failNext {
		m.failNext = false
		return nil, errors.New("mock texture creation failed")
	}
	tex := &mockTexture{
		width:  width,
		height: height,
		data:   make([]byte, len(data)),
	}
	copy(tex.data, data)
	m.textures = append(m.textures, tex)
	return tex, nil
}

// mockDrawContext implements gpucontext.TextureDrawer for testing.
type mockDrawContext struct {
	renderer     *mockRenderer
	drawnTexture gpucontext.Texture
	drawnX       float32
	drawnY       float32
	drawCount    int
}

func (m *mockDrawContext) DrawTexture(tex gpucontext.Texture, x, y float32) error {
	m.drawnTexture = tex
	m.drawnX = x
	m.drawnY = y
	m.drawCount++
	return nil
}

func (m *mockDrawContext) TextureCreator() gpucontext.TextureCreator {
	if m.renderer == nil {
		return nil
	}
	return m.renderer
}

func (m *mockDrawContext) Renderer() any {
	return m.renderer
}

// TestNew tests canvas creation.
func TestNew(t *testing.T) {
	provider := newMockProvider()

	tests := []struct {
		name      string
		provider  gpucontext.DeviceProvider
		width     int
		height    int
		wantErr   error
		checkFunc func(*testing.T, *Canvas)
	}{
		{
			name:     "valid creation",
			provider: provider,
			width:    800,
			height:   600,
			wantErr:  nil,
			checkFunc: func(t *testing.T, c *Canvas) {
				if c.Width() != 800 {
					t.Errorf("Width() = %d, want 800", c.Width())
				}
				if c.Height() != 600 {
					t.Errorf("Height() = %d, want 600", c.Height())
				}
				if c.Context() == nil {
					t.Error("Context() = nil, want non-nil")
				}
				if !c.IsDirty() {
					t.Error("IsDirty() = false, want true (newly created)")
				}
			},
		},
		{
			name:     "nil provider",
			provider: nil,
			width:    800,
			height:   600,
			wantErr:  ErrNilProvider,
		},
		{
			name:     "zero width",
			provider: provider,
			width:    0,
			height:   600,
			wantErr:  ErrInvalidDimensions,
		},
		{
			name:     "negative height",
			provider: provider,
			width:    800,
			height:   -1,
			wantErr:  ErrInvalidDimensions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.provider, tt.width, tt.height)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("New() error = nil, want %v", tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("New() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("New() unexpected error = %v", err)
				return
			}

			defer c.Close()

			if tt.checkFunc != nil {
				tt.checkFunc(t, c)
			}
		})
	}
}

// TestMustNew tests panic behavior.
func TestMustNew(t *testing.T) {
	provider := newMockProvider()

	t.Run("success", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustNew() panicked unexpectedly: %v", r)
			}
		}()

		c := MustNew(provider, 100, 100)
		defer c.Close()

		if c == nil {
			t.Error("MustNew() returned nil")
		}
	})

	t.Run("panic on nil provider", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustNew() did not panic with nil provider")
			}
		}()

		_ = MustNew(nil, 100, 100)
	})
}

// TestCanvasContext tests context access.
func TestCanvasContext(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 200, 200)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	ctx := c.Context()
	if ctx == nil {
		t.Fatal("Context() = nil, want non-nil")
	}

	// Verify context dimensions match canvas
	if ctx.Width() != 200 || ctx.Height() != 200 {
		t.Errorf("Context dimensions = %dx%d, want 200x200", ctx.Width(), ctx.Height())
	}
}

// TestCanvasResize tests resize functionality.
func TestCanvasResize(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	// Verify initial size
	w, h := c.Size()
	if w != 100 || h != 100 {
		t.Errorf("Size() = %dx%d, want 100x100", w, h)
	}

	// Resize
	if err := c.Resize(200, 150); err != nil {
		t.Errorf("Resize() error = %v", err)
	}

	// Verify new size
	w, h = c.Size()
	if w != 200 || h != 150 {
		t.Errorf("Size() after resize = %dx%d, want 200x150", w, h)
	}

	// Verify dirty flag is set
	if !c.IsDirty() {
		t.Error("IsDirty() after resize = false, want true")
	}

	// Resize to same size should be no-op
	c.dirty = false
	if err := c.Resize(200, 150); err != nil {
		t.Errorf("Resize() same size error = %v", err)
	}
	if c.IsDirty() {
		t.Error("IsDirty() after same-size resize = true, want false")
	}

	// Invalid resize
	if err := c.Resize(0, 100); err == nil {
		t.Error("Resize(0, 100) error = nil, want error")
	}
}

// TestCanvasDirtyTracking tests the dirty flag behavior.
func TestCanvasDirtyTracking(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	// Newly created should be dirty
	if !c.IsDirty() {
		t.Error("new canvas should be dirty")
	}

	// Manual mark
	c.dirty = false
	c.MarkDirty()
	if !c.IsDirty() {
		t.Error("MarkDirty() should set dirty flag")
	}
}

// TestCanvasFlush tests the flush operation.
func TestCanvasFlush(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	// First flush should create pending texture
	tex, err := c.Flush()
	if err != nil {
		t.Errorf("Flush() error = %v", err)
	}
	if tex == nil {
		t.Error("Flush() returned nil texture")
	}

	// Should be pending
	if _, ok := tex.(*pendingTexture); !ok {
		t.Error("First flush should return pending texture")
	}

	// Dirty should be cleared
	if c.IsDirty() {
		t.Error("IsDirty() after flush = true, want false")
	}

	// Second flush without dirty should return same texture
	tex2, err := c.Flush()
	if err != nil {
		t.Errorf("Second Flush() error = %v", err)
	}
	if tex2 != tex {
		t.Error("Second flush should return same texture when not dirty")
	}
}

// TestCanvasClose tests cleanup behavior.
func TestCanvasClose(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Close should succeed
	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Context should be nil after close
	if c.Context() != nil {
		t.Error("Context() after close should return nil")
	}

	// Provider should be nil after close
	if c.Provider() != nil {
		t.Error("Provider() after close should return nil")
	}

	// Double close should be safe
	if err := c.Close(); err != nil {
		t.Errorf("Second Close() error = %v", err)
	}

	// Operations on closed canvas should fail
	if err := c.Resize(200, 200); !errors.Is(err, ErrCanvasClosed) {
		t.Errorf("Resize() on closed canvas error = %v, want %v", err, ErrCanvasClosed)
	}

	if _, err := c.Flush(); !errors.Is(err, ErrCanvasClosed) {
		t.Errorf("Flush() on closed canvas error = %v, want %v", err, ErrCanvasClosed)
	}
}

// TestRenderTo tests the RenderTo integration.
func TestRenderTo(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	renderer := &mockRenderer{}
	dc := &mockDrawContext{renderer: renderer}

	// Draw something on canvas
	ctx := c.Context()
	ctx.SetRGB(1, 0, 0)
	ctx.DrawCircle(50, 50, 25)
	_ = ctx.Fill()
	c.MarkDirty()

	// Render to mock context
	if err := c.RenderTo(dc); err != nil {
		t.Errorf("RenderTo() error = %v", err)
	}

	// Verify texture was created
	if len(renderer.textures) != 1 {
		t.Errorf("Expected 1 texture created, got %d", len(renderer.textures))
	}

	// Verify draw was called
	if dc.drawCount != 1 {
		t.Errorf("DrawTexture called %d times, want 1", dc.drawCount)
	}

	// Verify position
	if dc.drawnX != 0 || dc.drawnY != 0 {
		t.Errorf("Drawn position = (%f, %f), want (0, 0)", dc.drawnX, dc.drawnY)
	}
}

// TestRenderToPosition tests positioned rendering.
func TestRenderToPosition(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	renderer := &mockRenderer{}
	dc := &mockDrawContext{renderer: renderer}

	if err := c.RenderToPosition(dc, 50, 75); err != nil {
		t.Errorf("RenderToPosition() error = %v", err)
	}

	if dc.drawnX != 50 || dc.drawnY != 75 {
		t.Errorf("Drawn position = (%f, %f), want (50, 75)", dc.drawnX, dc.drawnY)
	}
}

// TestRenderToNilRenderer tests error handling when renderer returns nil.
func TestRenderToNilRenderer(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	dc := &mockDrawContext{renderer: nil}

	err = c.RenderTo(dc)
	if !errors.Is(err, ErrInvalidRenderer) {
		t.Errorf("RenderTo() with nil renderer error = %v, want %v", err, ErrInvalidRenderer)
	}
}

// TestRenderOptions tests default options.
func TestRenderOptions(t *testing.T) {
	opts := DefaultRenderOptions()

	if opts.X != 0 || opts.Y != 0 {
		t.Errorf("Default position = (%f, %f), want (0, 0)", opts.X, opts.Y)
	}
	if opts.ScaleX != 1 || opts.ScaleY != 1 {
		t.Errorf("Default scale = (%f, %f), want (1, 1)", opts.ScaleX, opts.ScaleY)
	}
	if opts.Alpha != 1 {
		t.Errorf("Default alpha = %f, want 1", opts.Alpha)
	}
	if opts.FlipY {
		t.Error("Default FlipY = true, want false")
	}
}

// TestTextureReuse tests that texture is reused across renders.
func TestTextureReuse(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	renderer := &mockRenderer{}
	dc := &mockDrawContext{renderer: renderer}

	// First render
	if err := c.RenderTo(dc); err != nil {
		t.Errorf("First RenderTo() error = %v", err)
	}

	// Second render without changes
	if err := c.RenderTo(dc); err != nil {
		t.Errorf("Second RenderTo() error = %v", err)
	}

	// Should only create one texture
	if len(renderer.textures) != 1 {
		t.Errorf("Expected 1 texture (reused), got %d", len(renderer.textures))
	}

	// Both draws should use same texture
	if dc.drawCount != 2 {
		t.Errorf("DrawTexture called %d times, want 2", dc.drawCount)
	}
}

// TestTextureUpdateOnDirty tests that texture is updated when dirty.
func TestTextureUpdateOnDirty(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	renderer := &mockRenderer{}
	dc := &mockDrawContext{renderer: renderer}

	// First render creates texture
	if err := c.RenderTo(dc); err != nil {
		t.Errorf("First RenderTo() error = %v", err)
	}

	// Mark dirty and render again
	c.MarkDirty()
	if err := c.RenderTo(dc); err != nil {
		t.Errorf("Second RenderTo() error = %v", err)
	}

	// Should still be one texture (updated, not recreated)
	if len(renderer.textures) != 1 {
		t.Errorf("Expected 1 texture, got %d", len(renderer.textures))
	}

	// Texture should have been updated
	tex := renderer.textures[0]
	if tex.updated != 1 {
		t.Errorf("Texture updated %d times, want 1", tex.updated)
	}
}

// TestMarkDirtyRegion tests dirty region accumulation.
func TestMarkDirtyRegion(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	// Clear initial dirty state.
	c.dirty = false
	c.dirtyRect = image.Rectangle{}

	// Single region.
	r1 := image.Rect(10, 20, 30, 40)
	c.MarkDirtyRegion(r1)
	if !c.IsDirty() {
		t.Error("IsDirty() = false after MarkDirtyRegion")
	}
	if c.dirtyRect != r1 {
		t.Errorf("dirtyRect = %v, want %v", c.dirtyRect, r1)
	}

	// Second region — should be union.
	r2 := image.Rect(50, 60, 70, 80)
	c.MarkDirtyRegion(r2)
	want := r1.Union(r2)
	if c.dirtyRect != want {
		t.Errorf("dirtyRect after union = %v, want %v", c.dirtyRect, want)
	}

	// Empty region should be no-op.
	before := c.dirtyRect
	c.MarkDirtyRegion(image.Rectangle{})
	if c.dirtyRect != before {
		t.Errorf("empty region changed dirtyRect: %v != %v", c.dirtyRect, before)
	}
}

// TestMarkDirtySetsFullCanvasRect verifies that MarkDirty (full invalidation)
// sets dirtyRect to the full canvas dimensions so LastDamage() returns correct bounds.
func TestMarkDirtySetsFullCanvasRect(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	c.dirty = false
	c.dirtyRect = image.Rectangle{}

	c.MarkDirtyRegion(image.Rect(10, 10, 20, 20))
	if c.dirtyRect.Empty() {
		t.Error("dirtyRect should be non-empty after MarkDirtyRegion")
	}

	c.MarkDirty()
	expected := image.Rect(0, 0, 100, 100)
	if c.dirtyRect != expected {
		t.Errorf("dirtyRect after MarkDirty = %v, want %v (full canvas)", c.dirtyRect, expected)
	}
}

// TestFlushPartialUpload verifies that Flush uses UpdateRegion when a dirty
// rect is set and the texture supports it.
func TestFlushPartialUpload(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	renderer := &mockRenderer{}
	dc := &mockDrawContext{renderer: renderer}

	// First render: creates texture via full upload.
	if err := c.RenderTo(dc); err != nil {
		t.Fatalf("First RenderTo() error = %v", err)
	}
	if len(renderer.textures) != 1 {
		t.Fatalf("Expected 1 texture, got %d", len(renderer.textures))
	}
	tex := renderer.textures[0]

	// Mark a small dirty region and flush.
	c.MarkDirtyRegion(image.Rect(5, 5, 15, 15))
	if err := c.RenderTo(dc); err != nil {
		t.Fatalf("Second RenderTo() error = %v", err)
	}

	// Should have used UpdateRegion, not UpdateData.
	if tex.updated != 0 {
		t.Errorf("UpdateData called %d times, want 0 (should use UpdateRegion)", tex.updated)
	}
	if len(tex.regionUpdates) != 1 {
		t.Fatalf("UpdateRegion called %d times, want 1", len(tex.regionUpdates))
	}

	ru := tex.regionUpdates[0]
	if ru.x != 5 || ru.y != 5 || ru.w != 10 || ru.h != 10 {
		t.Errorf("UpdateRegion params = (%d,%d,%d,%d), want (5,5,10,10)",
			ru.x, ru.y, ru.w, ru.h)
	}

	// Data should be 10*10*4 = 400 bytes (densely packed RGBA).
	wantBytes := 10 * 10 * 4
	if len(ru.data) != wantBytes {
		t.Errorf("UpdateRegion data len = %d, want %d", len(ru.data), wantBytes)
	}

	// dirtyRect should be reset after flush.
	if !c.dirtyRect.Empty() {
		t.Errorf("dirtyRect should be empty after Flush, got %v", c.dirtyRect)
	}
}

// TestFlushFullUploadWhenNoDirtyRect verifies that MarkDirty() (without region)
// causes a full upload even when the texture supports region updates.
func TestFlushFullUploadWhenNoDirtyRect(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	renderer := &mockRenderer{}
	dc := &mockDrawContext{renderer: renderer}

	// First render.
	if err := c.RenderTo(dc); err != nil {
		t.Fatalf("First RenderTo() error = %v", err)
	}
	tex := renderer.textures[0]

	// MarkDirty (no region) → should do full upload.
	c.MarkDirty()
	if err := c.RenderTo(dc); err != nil {
		t.Fatalf("Second RenderTo() error = %v", err)
	}

	if tex.updated != 1 {
		t.Errorf("UpdateData called %d times, want 1", tex.updated)
	}
	if len(tex.regionUpdates) != 0 {
		t.Errorf("UpdateRegion called %d times, want 0", len(tex.regionUpdates))
	}
}

// TestFlushFullUploadWhenRegionCoversEntirePixmap verifies that a dirty rect
// covering the entire pixmap falls back to full upload (no wasted copy).
func TestFlushFullUploadWhenRegionCoversEntirePixmap(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	renderer := &mockRenderer{}
	dc := &mockDrawContext{renderer: renderer}

	// First render.
	if err := c.RenderTo(dc); err != nil {
		t.Fatalf("First RenderTo() error = %v", err)
	}
	tex := renderer.textures[0]

	// Dirty rect == full pixmap → should use UpdateData (cheaper, no row copy).
	c.MarkDirtyRegion(image.Rect(0, 0, c.ctx.PixelWidth(), c.ctx.PixelHeight()))
	if err := c.RenderTo(dc); err != nil {
		t.Fatalf("Second RenderTo() error = %v", err)
	}

	if tex.updated != 1 {
		t.Errorf("UpdateData called %d times, want 1 (full pixmap region)", tex.updated)
	}
	if len(tex.regionUpdates) != 0 {
		t.Errorf("UpdateRegion called %d times, want 0 (full pixmap → full upload)", len(tex.regionUpdates))
	}
}

// TestFlushClampsDirtyRectToPixmap verifies that a dirty rect extending
// beyond the pixmap is clamped to the actual pixmap dimensions.
func TestFlushClampsDirtyRectToPixmap(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	renderer := &mockRenderer{}
	dc := &mockDrawContext{renderer: renderer}

	// First render.
	if err := c.RenderTo(dc); err != nil {
		t.Fatalf("First RenderTo() error = %v", err)
	}
	tex := renderer.textures[0]

	// Dirty rect extends beyond pixmap → should be clamped.
	c.MarkDirtyRegion(image.Rect(40, 40, 200, 200))
	if err := c.RenderTo(dc); err != nil {
		t.Fatalf("Second RenderTo() error = %v", err)
	}

	if len(tex.regionUpdates) != 1 {
		t.Fatalf("UpdateRegion called %d times, want 1", len(tex.regionUpdates))
	}

	ru := tex.regionUpdates[0]
	pw := c.ctx.PixelWidth()
	ph := c.ctx.PixelHeight()
	wantW := pw - 40
	wantH := ph - 40
	if ru.x != 40 || ru.y != 40 || ru.w != wantW || ru.h != wantH {
		t.Errorf("Clamped region = (%d,%d,%d,%d), want (40,40,%d,%d)",
			ru.x, ru.y, ru.w, ru.h, wantW, wantH)
	}
}

// TestFlushPixmap tests the pixmap-only flush (no GPU readback).
func TestFlushPixmap(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	tex, err := c.FlushPixmap()
	if err != nil {
		t.Errorf("FlushPixmap() error = %v", err)
	}
	if tex == nil {
		t.Error("FlushPixmap() returned nil texture")
	}

	if _, ok := tex.(*pendingTexture); !ok {
		t.Error("First FlushPixmap should return pending texture")
	}

	if c.IsDirty() {
		t.Error("IsDirty() after FlushPixmap = true, want false")
	}

	tex2, err := c.FlushPixmap()
	if err != nil {
		t.Errorf("Second FlushPixmap() error = %v", err)
	}
	if tex2 != tex {
		t.Error("Second FlushPixmap should return same texture when not dirty")
	}
}

// TestFlushPixmapOnClosedCanvas verifies FlushPixmap fails on closed canvas.
func TestFlushPixmapOnClosedCanvas(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = c.Close()

	_, err = c.FlushPixmap()
	if !errors.Is(err, ErrCanvasClosed) {
		t.Errorf("FlushPixmap() on closed canvas error = %v, want %v", err, ErrCanvasClosed)
	}
}

// TestFlushAndFlushPixmapConsistency verifies Flush and FlushPixmap produce
// identical textures when no GPU shapes are pending (regression test).
func TestFlushAndFlushPixmapConsistency(t *testing.T) {
	provider := newMockProvider()

	c1, err := New(provider, 80, 60)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c1.Close()

	c2, err := New(provider, 80, 60)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c2.Close()

	ctx1 := c1.Context()
	ctx1.SetRGB(1, 0, 0)
	ctx1.DrawRectangle(10, 10, 30, 20)
	ctx1.Fill()

	ctx2 := c2.Context()
	ctx2.SetRGB(1, 0, 0)
	ctx2.DrawRectangle(10, 10, 30, 20)
	ctx2.Fill()

	tex1, err := c1.Flush()
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	tex2, err := c2.FlushPixmap()
	if err != nil {
		t.Fatalf("FlushPixmap() error = %v", err)
	}

	pt1, ok1 := tex1.(*pendingTexture)
	pt2, ok2 := tex2.(*pendingTexture)
	if !ok1 || !ok2 {
		t.Fatal("Both should return pendingTexture")
	}

	if pt1.width != pt2.width || pt1.height != pt2.height {
		t.Errorf("Texture dimensions differ: %dx%d vs %dx%d",
			pt1.width, pt1.height, pt2.width, pt2.height)
	}
	if len(pt1.data) != len(pt2.data) {
		t.Fatalf("Texture data length differs: %d vs %d", len(pt1.data), len(pt2.data))
	}
	for i := range pt1.data {
		if pt1.data[i] != pt2.data[i] {
			t.Errorf("Texture data differs at byte %d: %d vs %d", i, pt1.data[i], pt2.data[i])
			break
		}
	}
}

// TestFlushPixmapPartialUpload verifies FlushPixmap respects dirty regions.
func TestFlushPixmapPartialUpload(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	renderer := &mockRenderer{}
	dc := &mockDrawContext{renderer: renderer}

	if err := c.RenderTo(dc); err != nil {
		t.Fatalf("First RenderTo() error = %v", err)
	}
	tex := renderer.textures[0]

	c.MarkDirtyRegion(image.Rect(5, 5, 15, 15))

	_, err = c.FlushPixmap()
	if err != nil {
		t.Fatalf("FlushPixmap() error = %v", err)
	}

	if tex.updated != 0 {
		t.Errorf("UpdateData called %d times, want 0 (should use UpdateRegion)", tex.updated)
	}
	if len(tex.regionUpdates) != 1 {
		t.Fatalf("UpdateRegion called %d times, want 1", len(tex.regionUpdates))
	}
	ru := tex.regionUpdates[0]
	if ru.x != 5 || ru.y != 5 || ru.w != 10 || ru.h != 10 {
		t.Errorf("UpdateRegion params = (%d,%d,%d,%d), want (5,5,10,10)",
			ru.x, ru.y, ru.w, ru.h)
	}
}

// TestExtractRegion verifies the pixel extraction helper.
func TestExtractRegion(t *testing.T) {
	const w, h = 4, 4
	const bpp = 4

	// Create a 4x4 pixmap with sequential pixel values.
	data := make([]byte, w*h*bpp)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := (y*w + x) * bpp
			v := byte(y*w + x)
			data[off+0] = v
			data[off+1] = v
			data[off+2] = v
			data[off+3] = 255
		}
	}

	tests := []struct {
		name string
		r    image.Rectangle
		want []byte
	}{
		{
			name: "top-left 2x2",
			r:    image.Rect(0, 0, 2, 2),
			want: []byte{
				0, 0, 0, 255, 1, 1, 1, 255, // row 0: px(0,0), px(1,0)
				4, 4, 4, 255, 5, 5, 5, 255, // row 1: px(0,1), px(1,1)
			},
		},
		{
			name: "center 2x2",
			r:    image.Rect(1, 1, 3, 3),
			want: []byte{
				5, 5, 5, 255, 6, 6, 6, 255, // row 1: px(1,1), px(2,1)
				9, 9, 9, 255, 10, 10, 10, 255, // row 2: px(1,2), px(2,2)
			},
		},
		{
			name: "single pixel",
			r:    image.Rect(3, 3, 4, 4),
			want: []byte{15, 15, 15, 255},
		},
		{
			name: "full row",
			r:    image.Rect(0, 2, 4, 3),
			want: []byte{
				8, 8, 8, 255, 9, 9, 9, 255, 10, 10, 10, 255, 11, 11, 11, 255,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Canvas{}
			got := c.extractRegion(data, w, tt.r)
			if len(got) != len(tt.want) {
				t.Fatalf("len(extractRegion) = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("byte[%d] = %d, want %d", i, got[i], tt.want[i])
					break
				}
			}
		})
	}
}

// TestPixmapTextureView_BeforeFlush returns nil before any flush.
func TestPixmapTextureView_BeforeFlush(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	if v := c.PixmapTextureView(); !v.IsNil() {
		t.Error("PixmapTextureView() before flush should be nil (IsNil)")
	}
}

// TestPixmapTextureView_PendingTexture returns zero-value for pendingTexture
// (not yet promoted to real GPU texture).
func TestPixmapTextureView_PendingTexture(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	_, _ = c.FlushPixmap()

	// pendingTexture does not implement viewProvider → zero-value
	if v := c.PixmapTextureView(); !v.IsNil() {
		t.Error("PixmapTextureView() on pendingTexture should be nil (IsNil)")
	}
}

// TestPixmapTextureView_PromotedTexture returns view after texture promotion.
func TestPixmapTextureView_PromotedTexture(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	renderer := &mockRenderer{}
	dc := &mockDrawContext{renderer: renderer}

	// RenderTo promotes pendingTexture to real mockTexture.
	if err := c.RenderTo(dc); err != nil {
		t.Fatalf("RenderTo() error = %v", err)
	}

	// mockTexture does not implement TextureView() → zero-value.
	// In production, *gogpu.Texture implements TextureView() → non-nil.
	v := c.PixmapTextureView()
	if !v.IsNil() {
		t.Error("mockTexture does not implement viewProvider, expected IsNil")
	}
}

// TestPixmapTextureView_ClosedCanvas returns nil on closed canvas.
func TestPixmapTextureView_ClosedCanvas(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 50, 50)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = c.Close()

	if v := c.PixmapTextureView(); !v.IsNil() {
		t.Error("PixmapTextureView() on closed canvas should be nil (IsNil)")
	}
}
