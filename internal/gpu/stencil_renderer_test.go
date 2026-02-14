//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// createNoopDevice creates a noop device and queue for testing.
// Returns the device, queue, and a cleanup function.
func createNoopDevice(t *testing.T) (hal.Device, hal.Queue, func()) {
	t.Helper()
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	adapters := instance.EnumerateAdapters(nil)
	openDev, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		instance.Destroy()
		t.Fatalf("Open failed: %v", err)
	}
	cleanup := func() {
		openDev.Device.Destroy()
		instance.Destroy()
	}
	return openDev.Device, openDev.Queue, cleanup
}

func TestStencilRendererNew(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	if sr == nil {
		t.Fatal("expected non-nil StencilRenderer")
	}
	if sr.device != device {
		t.Error("device not stored correctly")
	}
	if sr.queue != queue {
		t.Error("queue not stored correctly")
	}
	if sr.textures.msaaTex != nil {
		t.Error("expected nil msaaTex before EnsureTextures")
	}
	if sr.textures.stencilTex != nil {
		t.Error("expected nil stencilTex before EnsureTextures")
	}
	if sr.textures.resolveTex != nil {
		t.Error("expected nil resolveTex before EnsureTextures")
	}

	w, h := sr.Size()
	if w != 0 || h != 0 {
		t.Errorf("expected size (0, 0) before EnsureTextures, got (%d, %d)", w, h)
	}
}

func TestStencilRendererEnsureTextures(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	err := sr.EnsureTextures(800, 600)
	if err != nil {
		t.Fatalf("EnsureTextures failed: %v", err)
	}

	if sr.textures.msaaTex == nil {
		t.Error("expected non-nil msaaTex after EnsureTextures")
	}
	if sr.textures.msaaView == nil {
		t.Error("expected non-nil msaaView after EnsureTextures")
	}
	if sr.textures.stencilTex == nil {
		t.Error("expected non-nil stencilTex after EnsureTextures")
	}
	if sr.textures.stencilView == nil {
		t.Error("expected non-nil stencilView after EnsureTextures")
	}
	if sr.textures.resolveTex == nil {
		t.Error("expected non-nil resolveTex after EnsureTextures")
	}
	if sr.textures.resolveView == nil {
		t.Error("expected non-nil resolveView after EnsureTextures")
	}

	w, h := sr.Size()
	if w != 800 || h != 600 {
		t.Errorf("expected size (800, 600), got (%d, %d)", w, h)
	}

	if sr.ResolveTexture() == nil {
		t.Error("expected non-nil ResolveTexture")
	}
}

func TestStencilRendererEnsureTexturesIdempotent(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	err := sr.EnsureTextures(640, 480)
	if err != nil {
		t.Fatalf("first EnsureTextures failed: %v", err)
	}

	// Save references to verify they remain the same.
	origMSAA := sr.textures.msaaTex
	origStencil := sr.textures.stencilTex
	origResolve := sr.textures.resolveTex

	// Call again with same dimensions — should be a no-op.
	err = sr.EnsureTextures(640, 480)
	if err != nil {
		t.Fatalf("second EnsureTextures failed: %v", err)
	}

	if sr.textures.msaaTex != origMSAA {
		t.Error("MSAA texture was recreated unnecessarily")
	}
	if sr.textures.stencilTex != origStencil {
		t.Error("stencil texture was recreated unnecessarily")
	}
	if sr.textures.resolveTex != origResolve {
		t.Error("resolve texture was recreated unnecessarily")
	}

	w, h := sr.Size()
	if w != 640 || h != 480 {
		t.Errorf("expected size (640, 480), got (%d, %d)", w, h)
	}
}

func TestStencilRendererEnsureTexturesResize(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	// Create initial textures at 800x600.
	err := sr.EnsureTextures(800, 600)
	if err != nil {
		t.Fatalf("initial EnsureTextures failed: %v", err)
	}

	w, h := sr.Size()
	if w != 800 || h != 600 {
		t.Errorf("expected initial size (800, 600), got (%d, %d)", w, h)
	}

	// Resize to 1920x1080 — old textures should be destroyed and new ones created.
	err = sr.EnsureTextures(1920, 1080)
	if err != nil {
		t.Fatalf("resize EnsureTextures failed: %v", err)
	}

	w, h = sr.Size()
	if w != 1920 || h != 1080 {
		t.Errorf("expected resized size (1920, 1080), got (%d, %d)", w, h)
	}

	if sr.textures.msaaTex == nil {
		t.Error("expected non-nil msaaTex after resize")
	}
	if sr.textures.stencilTex == nil {
		t.Error("expected non-nil stencilTex after resize")
	}
	if sr.textures.resolveTex == nil {
		t.Error("expected non-nil resolveTex after resize")
	}
}

func TestStencilRendererDestroy(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)

	err := sr.EnsureTextures(512, 512)
	if err != nil {
		t.Fatalf("EnsureTextures failed: %v", err)
	}

	// Verify textures exist before destroy.
	if sr.textures.msaaTex == nil {
		t.Fatal("expected non-nil msaaTex before Destroy")
	}

	sr.Destroy()

	if sr.textures.msaaTex != nil {
		t.Error("expected nil msaaTex after Destroy")
	}
	if sr.textures.msaaView != nil {
		t.Error("expected nil msaaView after Destroy")
	}
	if sr.textures.stencilTex != nil {
		t.Error("expected nil stencilTex after Destroy")
	}
	if sr.textures.stencilView != nil {
		t.Error("expected nil stencilView after Destroy")
	}
	if sr.textures.resolveTex != nil {
		t.Error("expected nil resolveTex after Destroy")
	}
	if sr.textures.resolveView != nil {
		t.Error("expected nil resolveView after Destroy")
	}

	w, h := sr.Size()
	if w != 0 || h != 0 {
		t.Errorf("expected size (0, 0) after Destroy, got (%d, %d)", w, h)
	}

	// Double-destroy should be safe.
	sr.Destroy()
}

func TestStencilRendererDestroyBeforeEnsure(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)

	// Destroy without ever calling EnsureTextures — should not panic.
	sr.Destroy()
}

func TestStencilRendererRenderPassDescriptor(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	// Before EnsureTextures, descriptor should be nil.
	if desc := sr.RenderPassDescriptor(); desc != nil {
		t.Error("expected nil RenderPassDescriptor before EnsureTextures")
	}

	err := sr.EnsureTextures(1024, 768)
	if err != nil {
		t.Fatalf("EnsureTextures failed: %v", err)
	}

	desc := sr.RenderPassDescriptor()
	if desc == nil {
		t.Fatal("expected non-nil RenderPassDescriptor after EnsureTextures")
	}

	// Verify label.
	if desc.Label != "stencil_cover_pass" {
		t.Errorf("expected label 'stencil_cover_pass', got %q", desc.Label)
	}

	// Verify color attachment.
	if len(desc.ColorAttachments) != 1 {
		t.Fatalf("expected 1 color attachment, got %d", len(desc.ColorAttachments))
	}
	ca := desc.ColorAttachments[0]
	if ca.View == nil {
		t.Error("expected non-nil color attachment View (MSAA)")
	}
	if ca.ResolveTarget == nil {
		t.Error("expected non-nil color attachment ResolveTarget")
	}
	if ca.LoadOp != gputypes.LoadOpClear {
		t.Errorf("expected color LoadOp Clear, got %v", ca.LoadOp)
	}
	if ca.StoreOp != gputypes.StoreOpStore {
		t.Errorf("expected color StoreOp Store, got %v", ca.StoreOp)
	}
	// Clear to transparent black (premultiplied alpha).
	if ca.ClearValue.A != 0 {
		t.Errorf("expected clear alpha 0 (transparent), got %v", ca.ClearValue.A)
	}

	// Verify depth/stencil attachment.
	dsa := desc.DepthStencilAttachment
	if dsa == nil {
		t.Fatal("expected non-nil DepthStencilAttachment")
	}
	if dsa.View == nil {
		t.Error("expected non-nil depth/stencil View")
	}
	if dsa.StencilLoadOp != gputypes.LoadOpClear {
		t.Errorf("expected stencil LoadOp Clear, got %v", dsa.StencilLoadOp)
	}
	if dsa.StencilStoreOp != gputypes.StoreOpDiscard {
		t.Errorf("expected stencil StoreOp Discard, got %v", dsa.StencilStoreOp)
	}
	if dsa.StencilClearValue != 0 {
		t.Errorf("expected stencil clear value 0, got %d", dsa.StencilClearValue)
	}
	if dsa.DepthLoadOp != gputypes.LoadOpClear {
		t.Errorf("expected depth LoadOp Clear, got %v", dsa.DepthLoadOp)
	}
	if dsa.DepthStoreOp != gputypes.StoreOpDiscard {
		t.Errorf("expected depth StoreOp Discard, got %v", dsa.DepthStoreOp)
	}
	if dsa.DepthClearValue != 1.0 {
		t.Errorf("expected depth clear value 1.0, got %v", dsa.DepthClearValue)
	}
}

func TestStencilRendererResolveTextureNilBeforeEnsure(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	if sr.ResolveTexture() != nil {
		t.Error("expected nil ResolveTexture before EnsureTextures")
	}
}

func TestStencilRendererEnsureAfterDestroy(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)

	// Create, destroy, then recreate.
	err := sr.EnsureTextures(256, 256)
	if err != nil {
		t.Fatalf("first EnsureTextures failed: %v", err)
	}

	sr.Destroy()

	w, h := sr.Size()
	if w != 0 || h != 0 {
		t.Errorf("expected (0, 0) after Destroy, got (%d, %d)", w, h)
	}

	// Re-create with different dimensions.
	err = sr.EnsureTextures(512, 512)
	if err != nil {
		t.Fatalf("EnsureTextures after Destroy failed: %v", err)
	}
	defer sr.Destroy()

	w, h = sr.Size()
	if w != 512 || h != 512 {
		t.Errorf("expected (512, 512) after re-creation, got (%d, %d)", w, h)
	}

	if sr.textures.msaaTex == nil || sr.textures.stencilTex == nil || sr.textures.resolveTex == nil {
		t.Error("expected all textures to be non-nil after re-creation")
	}
}
