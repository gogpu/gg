// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package ggcanvas

import (
	"errors"
	"image"
	"testing"

	"github.com/gogpu/gpucontext"
)

// Mock types for Render() tests. These extend mockRenderTarget from canvas_test.go
// with additional capabilities (SurfacePixelWriter).

// renderMockPixelWriter implements RenderTarget + SurfacePixelWriter.
type renderMockPixelWriter struct {
	presentedTex   any
	presentCount   int
	writtenPixels  []byte
	writtenW       uint32
	writtenH       uint32
	writeCallCount int
	writeErr       error
	damageRects    []image.Rectangle
	damageSetCount int
}

func (m *renderMockPixelWriter) SurfaceView() gpucontext.TextureView { return gpucontext.TextureView{} }
func (m *renderMockPixelWriter) SurfaceSize() (uint32, uint32)       { return 0, 0 }
func (m *renderMockPixelWriter) PresentTexture(tex any) error {
	m.presentedTex = tex
	m.presentCount++
	return nil
}
func (m *renderMockPixelWriter) WriteSurfacePixels(data []byte, width, height uint32) error {
	m.writtenPixels = make([]byte, len(data))
	copy(m.writtenPixels, data)
	m.writtenW = width
	m.writtenH = height
	m.writeCallCount++
	return m.writeErr
}
func (m *renderMockPixelWriter) SetDamageRects(rects []image.Rectangle) {
	m.damageRects = rects
	m.damageSetCount++
}

// TextureCreator returns a mock renderer so promoteIfPending can create a real texture.
func (m *renderMockPixelWriter) TextureCreator() gpucontext.TextureCreator {
	return &mockRenderer{}
}

// renderMockWithCreator adds TextureCreator to mockRenderTarget so the
// universal path (Flush -> promoteIfPending -> PresentTexture) works.
type renderMockWithCreator struct {
	mockRenderTarget
	renderer *mockRenderer
}

func (m *renderMockWithCreator) TextureCreator() gpucontext.TextureCreator {
	return m.renderer
}

// TestRender_NotDirty_Noop verifies that Render returns nil without presenting
// when the canvas is not dirty.
func TestRender_NotDirty_Noop(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Clear dirty flag manually.
	c.dirty = false

	dc := &mockRenderTarget{}
	if err := c.Render(dc); err != nil {
		t.Fatalf("Render on clean canvas: %v", err)
	}
	if dc.presentedTex != nil {
		t.Error("PresentTexture should not be called when not dirty")
	}
}

// TestRender_Closed_ReturnsError verifies that Render returns ErrCanvasClosed
// when the canvas is closed.
func TestRender_Closed_ReturnsError(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()

	dc := &mockRenderTarget{}
	err = c.Render(dc)
	if !errors.Is(err, ErrCanvasClosed) {
		t.Errorf("Render on closed canvas: err = %v, want ErrCanvasClosed", err)
	}
}

// TestRender_SurfacePixelWriter_Success verifies that when WriteSurfacePixels
// succeeds, Render sets dirty=false and does NOT call PresentTexture.
func TestRender_SurfacePixelWriter_Success(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.dirty = true

	dc := &renderMockPixelWriter{}
	if err := c.Render(dc); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if c.dirty {
		t.Error("dirty should be false after successful WriteSurfacePixels")
	}
	if dc.presentCount != 0 {
		t.Errorf("PresentTexture called %d times, want 0 (pixel upload succeeded)", dc.presentCount)
	}
	if dc.writeCallCount != 1 {
		t.Errorf("WriteSurfacePixels called %d times, want 1", dc.writeCallCount)
	}
}

// TestRender_SurfacePixelWriter_Error_FallsThrough verifies that when
// WriteSurfacePixels returns an error, Render falls through to the universal
// path and calls PresentTexture.
func TestRender_SurfacePixelWriter_Error_FallsThrough(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.dirty = true

	dc := &renderMockPixelWriter{
		writeErr: errors.New("mock write error"),
	}

	// Render should fall through to universal path (Flush -> promoteIfPending -> PresentTexture).
	err = c.Render(dc)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if dc.writeCallCount != 1 {
		t.Errorf("WriteSurfacePixels called %d times, want 1", dc.writeCallCount)
	}
	if dc.presentCount != 1 {
		t.Errorf("PresentTexture called %d times, want 1 (fallback after write error)", dc.presentCount)
	}
}

// TestRender_UniversalPath verifies the universal path: Flush -> PresentTexture.
// No SurfaceView, no PixelWriter -> goes through Flush + promoteIfPending.
func TestRender_UniversalPath(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.dirty = true

	dc := &renderMockWithCreator{renderer: &mockRenderer{}}
	if err := c.Render(dc); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if dc.presentedTex == nil {
		t.Error("PresentTexture should have been called with non-nil texture")
	}
}

// TestRender_DamageRectsForwardedOnPixelUpload verifies that damage rects ARE
// forwarded to the render target before WriteSurfacePixels for partial blit
// optimization. WritePixels writes full pixmap to DIB, blitDamageRectsToWindow
// BitBlts only changed areas.
func TestRender_DamageRectsForwardedOnPixelUpload(t *testing.T) {
	provider := newMockProvider()
	c, err := New(provider, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.dirty = true

	// Set present damage rects.
	c.SetPresentDamage([]image.Rectangle{
		image.Rect(0, 0, 5, 5),
	})

	dc := &renderMockPixelWriter{}
	if err := c.Render(dc); err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Damage rects forwarded for partial blit optimization.
	if dc.damageSetCount == 0 {
		t.Error("SetDamageRects not called — damage rects should be forwarded for partial blit")
	}
}
