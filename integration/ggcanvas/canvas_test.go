// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package ggcanvas

import (
	"errors"
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

// mockTexture implements the texture interfaces for testing.
type mockTexture struct {
	width     int
	height    int
	data      []byte
	destroyed bool
	updated   int
}

func (m *mockTexture) UpdateData(data []byte) {
	m.data = make([]byte, len(data))
	copy(m.data, data)
	m.updated++
}

func (m *mockTexture) Destroy() {
	m.destroyed = true
}

// mockRenderer implements rendererWithTextureCreation for testing.
type mockRenderer struct {
	textures []*mockTexture
	failNext bool
}

func (m *mockRenderer) NewTextureFromRGBA(width, height int, data []byte) (any, error) {
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

// mockDrawContext implements textureDrawer for testing.
type mockDrawContext struct {
	renderer     *mockRenderer
	drawnTexture any
	drawnX       float32
	drawnY       float32
	drawCount    int
}

func (m *mockDrawContext) DrawTexture(tex any, x, y float32) error {
	m.drawnTexture = tex
	m.drawnX = x
	m.drawnY = y
	m.drawCount++
	return nil
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

// TestRenderToInvalidContext tests error handling for invalid contexts.
func TestRenderToInvalidContext(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	// Pass non-drawer type
	err = c.RenderTo("not a drawer")
	if !errors.Is(err, ErrInvalidDrawContext) {
		t.Errorf("RenderTo(string) error = %v, want %v", err, ErrInvalidDrawContext)
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
