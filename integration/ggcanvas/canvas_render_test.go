// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package ggcanvas

import (
	"errors"
	"image"
	"testing"
	"unsafe"

	"github.com/gogpu/gg"
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

type renderMockContentPreserver struct {
	mockRenderTarget
	preserveContent bool
}

func (m *renderMockContentPreserver) PreserveContent() bool { return m.preserveContent }

type renderMockCommandEncoderProvider struct {
	mockRenderTarget
	encoder      gpucontext.CommandEncoder
	encoderCalls int
}

func (m *renderMockCommandEncoderProvider) CommandEncoder() gpucontext.CommandEncoder {
	m.encoderCalls++
	return m.encoder
}

// renderMockFrameTarget models a target where borrowing the frame encoder
// records deferred work and changes whether the next pass must preserve the
// surface. gogpu.ContextRenderTarget has this behavior when CommandEncoder
// flushes a deferred clear.
type renderMockFrameTarget struct {
	mockRenderTarget
	encoder         gpucontext.CommandEncoder
	preserveContent bool
	calls           []string
}

func (m *renderMockFrameTarget) CommandEncoder() gpucontext.CommandEncoder {
	m.calls = append(m.calls, "encoder")
	m.preserveContent = true
	return m.encoder
}

func (m *renderMockFrameTarget) PreserveContent() bool {
	m.calls = append(m.calls, "preserve")
	return m.preserveContent
}

type renderTargetCaptureAccelerator struct {
	lastTarget gg.GPURenderTarget
	flushes    int
}

func (a *renderTargetCaptureAccelerator) Name() string { return "render-target-capture" }
func (a *renderTargetCaptureAccelerator) Init() error  { return nil }
func (a *renderTargetCaptureAccelerator) Close()       {}
func (a *renderTargetCaptureAccelerator) CanAccelerate(gg.AcceleratedOp) bool {
	return false
}
func (a *renderTargetCaptureAccelerator) FillPath(gg.GPURenderTarget, *gg.Path, *gg.Paint) error {
	return gg.ErrFallbackToCPU
}
func (a *renderTargetCaptureAccelerator) StrokePath(gg.GPURenderTarget, *gg.Path, *gg.Paint) error {
	return gg.ErrFallbackToCPU
}
func (a *renderTargetCaptureAccelerator) FillShape(gg.GPURenderTarget, gg.DetectedShape, *gg.Paint) error {
	return gg.ErrFallbackToCPU
}
func (a *renderTargetCaptureAccelerator) StrokeShape(gg.GPURenderTarget, gg.DetectedShape, *gg.Paint) error {
	return gg.ErrFallbackToCPU
}
func (a *renderTargetCaptureAccelerator) Flush(target gg.GPURenderTarget) error {
	a.lastTarget = target
	a.flushes++
	return nil
}
func (a *renderTargetCaptureAccelerator) CanRenderDirect() bool { return true }

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

func TestRender_PreserveContentForwardedToGPU(t *testing.T) {
	gg.CloseAccelerator()
	accelerator := &renderTargetCaptureAccelerator{}
	if err := gg.RegisterAccelerator(accelerator); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}
	t.Cleanup(gg.CloseAccelerator)

	view := gpucontext.NewTextureView(unsafe.Pointer(new(int))) //nolint:gosec // non-nil opaque handle for routing test
	defaultTarget := &mockRenderTarget{surfaceView: view, surfaceW: 10, surfaceH: 10}
	preservingTarget := &renderMockContentPreserver{
		mockRenderTarget: *defaultTarget,
		preserveContent:  true,
	}

	tests := []struct {
		name             string
		render           func(*Canvas) error
		wantPreservation bool
	}{
		{
			name:   "RenderDirect defaults to clear",
			render: func(c *Canvas) error { return c.RenderDirect(view, 10, 10) },
		},
		{
			name:   "RenderTarget without capability defaults to clear",
			render: func(c *Canvas) error { return c.Render(defaultTarget) },
		},
		{
			name:             "RenderTarget forwards preservation",
			render:           func(c *Canvas) error { return c.Render(preservingTarget) },
			wantPreservation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accelerator.lastTarget = gg.GPURenderTarget{}
			accelerator.flushes = 0

			canvas, err := New(newMockProvider(), 10, 10)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			defer canvas.Close()

			if err := tt.render(canvas); err != nil {
				t.Fatalf("render: %v", err)
			}
			if accelerator.flushes != 1 {
				t.Fatalf("accelerator Flush calls = %d, want 1", accelerator.flushes)
			}
			if got := accelerator.lastTarget.PreserveContent; got != tt.wantPreservation {
				t.Errorf("GPURenderTarget.PreserveContent = %v, want %v", got, tt.wantPreservation)
			}
		})
	}
}

func TestRender_BorrowsCommandEncoderFromTarget(t *testing.T) {
	gg.CloseAccelerator()
	accelerator := &renderTargetCaptureAccelerator{}
	if err := gg.RegisterAccelerator(accelerator); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}
	t.Cleanup(gg.CloseAccelerator)

	view := gpucontext.NewTextureView(unsafe.Pointer(new(int)))       //nolint:gosec // non-nil opaque handle for routing test
	encoder := gpucontext.NewCommandEncoder(unsafe.Pointer(new(int))) //nolint:gosec // non-nil opaque handle for routing test
	target := &renderMockCommandEncoderProvider{
		mockRenderTarget: mockRenderTarget{surfaceView: view, surfaceW: 10, surfaceH: 10},
		encoder:          encoder,
	}
	canvas, err := New(newMockProvider(), 10, 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer canvas.Close()

	if err := canvas.Render(target); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if target.encoderCalls != 1 {
		t.Fatalf("CommandEncoder calls = %d, want 1", target.encoderCalls)
	}
	if accelerator.flushes != 1 {
		t.Fatalf("accelerator Flush calls = %d, want 1", accelerator.flushes)
	}
}

func TestRender_SamplesContentAfterBorrowingEncoder(t *testing.T) {
	gg.CloseAccelerator()
	accelerator := &renderTargetCaptureAccelerator{}
	if err := gg.RegisterAccelerator(accelerator); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}
	t.Cleanup(gg.CloseAccelerator)

	view := gpucontext.NewTextureView(unsafe.Pointer(new(int)))       //nolint:gosec // non-nil opaque handle for routing test
	encoder := gpucontext.NewCommandEncoder(unsafe.Pointer(new(int))) //nolint:gosec // non-nil opaque handle for routing test
	target := &renderMockFrameTarget{
		mockRenderTarget: mockRenderTarget{surfaceView: view, surfaceW: 10, surfaceH: 10},
		encoder:          encoder,
	}
	canvas, err := New(newMockProvider(), 10, 10)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer canvas.Close()

	if err := canvas.Render(target); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(target.calls) != 2 || target.calls[0] != "encoder" || target.calls[1] != "preserve" {
		t.Fatalf("capability call order = %v, want [encoder preserve]", target.calls)
	}
	if !accelerator.lastTarget.PreserveContent {
		t.Error("GPURenderTarget.PreserveContent = false after encoder acquisition, want true")
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
