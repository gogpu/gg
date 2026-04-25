//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"math"
	"testing"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
)

// --- Vertex data tests ---

func TestBuildImageVertices_OverlayPosition(t *testing.T) {
	cmd := &ImageDrawCommand{
		DstX: 100, DstY: 100, DstW: 48, DstH: 48,
		U0: 0, V0: 0, U1: 1, V1: 1,
	}
	buf := buildImageVertices(cmd)

	verts := decodeVertices(t, buf)
	if len(verts) != 6 {
		t.Fatalf("expected 6 vertices, got %d", len(verts))
	}

	// TL = (100, 100), TR = (148, 100), BL = (100, 148)
	assertVertex(t, verts[0], 100, 100, 0, 0, "TL")
	assertVertex(t, verts[1], 148, 100, 1, 0, "TR")
	assertVertex(t, verts[2], 100, 148, 0, 1, "BL")
	assertVertex(t, verts[3], 148, 100, 1, 0, "TR2")
	assertVertex(t, verts[4], 148, 148, 1, 1, "BR")
	assertVertex(t, verts[5], 100, 148, 0, 1, "BL2")
}

func TestBuildImageVertices_FullScreen(t *testing.T) {
	cmd := &ImageDrawCommand{
		DstX: 0, DstY: 0, DstW: 600, DstH: 400,
		U0: 0, V0: 0, U1: 1, V1: 1,
	}
	buf := buildImageVertices(cmd)
	verts := decodeVertices(t, buf)

	assertVertex(t, verts[0], 0, 0, 0, 0, "TL")
	assertVertex(t, verts[4], 600, 400, 1, 1, "BR")
}

func TestBuildImageVertices_ZeroSize(t *testing.T) {
	cmd := &ImageDrawCommand{DstX: 50, DstY: 50, DstW: 0, DstH: 0}
	buf := buildImageVertices(cmd)
	verts := decodeVertices(t, buf)

	// Degenerate quad: all corners collapse to same point.
	for i, v := range verts {
		if v.px != 50 || v.py != 50 {
			t.Errorf("vertex %d: expected (50, 50), got (%.1f, %.1f)", i, v.px, v.py)
		}
	}
}

// --- Ortho uniform tests ---

func TestMakeImageUniform_OrthoProjection(t *testing.T) {
	tests := []struct {
		name   string
		w, h   uint32
		wantSx float32 // expected 2/w
		wantSy float32 // expected -2/h
		wantTx float32 // expected -1
		wantTy float32 // expected 1
	}{
		{"600x400", 600, 400, 2.0 / 600, -2.0 / 400, -1, 1},
		{"48x48", 48, 48, 2.0 / 48, -2.0 / 48, -1, 1},
		{"1920x1080", 1920, 1080, 2.0 / 1920, -2.0 / 1080, -1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := makeImageUniform(tt.w, tt.h, 1.0)
			if len(buf) != int(imageUniformSize) {
				t.Fatalf("uniform size: got %d, want %d", len(buf), imageUniformSize)
			}

			sx := math.Float32frombits(binary.LittleEndian.Uint32(buf[0:]))
			sy := math.Float32frombits(binary.LittleEndian.Uint32(buf[20:]))
			tx := math.Float32frombits(binary.LittleEndian.Uint32(buf[48:]))
			ty := math.Float32frombits(binary.LittleEndian.Uint32(buf[52:]))

			assertFloat(t, sx, tt.wantSx, "sx (2/w)")
			assertFloat(t, sy, tt.wantSy, "sy (-2/h)")
			assertFloat(t, tx, tt.wantTx, "tx")
			assertFloat(t, ty, tt.wantTy, "ty")
		})
	}
}

func TestMakeImageUniform_OverlayUsesMainViewport(t *testing.T) {
	// The overlay ortho matrix must use the MAIN viewport (600x400),
	// not the overlay texture size (48x48).
	// If ortho is 48x48 and vertex position is (100,100)-(148,148),
	// the quad would map to NDC > 1.0 and render off-screen or stretched.
	mainVP := makeImageUniform(600, 400, 1.0)
	overlayVP := makeImageUniform(48, 48, 1.0)

	mainSx := math.Float32frombits(binary.LittleEndian.Uint32(mainVP[0:]))
	overlaySx := math.Float32frombits(binary.LittleEndian.Uint32(overlayVP[0:]))

	if mainSx == overlaySx {
		t.Errorf("ortho scale should differ: main=%f overlay=%f", mainSx, overlaySx)
	}

	// Verify: with 600x400 ortho, position (100,100) maps to NDC within [-1,1].
	ndcX := 100*mainSx + (-1.0)
	if ndcX < -1 || ndcX > 1 {
		t.Errorf("NDC x=%.4f out of [-1,1] range for overlay at x=100 in 600px viewport", ndcX)
	}

	// Verify: with 48x48 ortho, position (100,100) maps OUTSIDE [-1,1] — BUG trigger.
	badNdcX := 100*overlaySx + (-1.0)
	if badNdcX >= -1 && badNdcX <= 1 {
		t.Logf("WARNING: 48x48 ortho puts x=100 at NDC %.4f (inside clip) — unexpected", badNdcX)
	}
}

// --- GPU texture command queueing tests ---

func TestQueueGPUTextureDraw_OverlayCoordinates(t *testing.T) {
	rc := &GPURenderContext{shared: NewGPUShared()}
	target := makeTestTarget(600, 400)

	dummyPtr := uintptr(0xDEADBEEF)
	view := gpucontext.NewTextureView(unsafe.Pointer(dummyPtr)) //nolint:gosec // test dummy

	rc.QueueGPUTextureDraw(target, view,
		100, 100, 48, 48, 1.0, 600, 400)

	if len(rc.pendingGPUTextureCommands) != 1 {
		t.Fatalf("expected 1 pending command, got %d", len(rc.pendingGPUTextureCommands))
	}

	cmd := rc.pendingGPUTextureCommands[0]
	assertFloat(t, cmd.DstX, 100, "DstX")
	assertFloat(t, cmd.DstY, 100, "DstY")
	assertFloat(t, cmd.DstW, 48, "DstW")
	assertFloat(t, cmd.DstH, 48, "DstH")
	if cmd.ViewportWidth != 600 || cmd.ViewportHeight != 400 {
		t.Errorf("viewport: got %dx%d, want 600x400", cmd.ViewportWidth, cmd.ViewportHeight)
	}
}

func TestQueueBaseLayer_FullScreen(t *testing.T) {
	rc := &GPURenderContext{shared: NewGPUShared()}
	target := makeTestTarget(600, 400)

	dummyPtr := uintptr(0xDEADBEEF)
	view := gpucontext.NewTextureView(unsafe.Pointer(dummyPtr)) //nolint:gosec // test dummy

	rc.QueueBaseLayer(target, view,
		0, 0, 600, 400, 1.0, 600, 400)

	if rc.baseLayer == nil {
		t.Fatal("expected baseLayer to be set")
	}
	assertFloat(t, rc.baseLayer.DstX, 0, "base DstX")
	assertFloat(t, rc.baseLayer.DstY, 0, "base DstY")
	assertFloat(t, rc.baseLayer.DstW, 600, "base DstW")
	assertFloat(t, rc.baseLayer.DstH, 400, "base DstH")
}

// --- PendingCount tests ---

func TestPendingCount_BaseLayerOnly(t *testing.T) {
	rc := &GPURenderContext{shared: NewGPUShared()}

	if rc.PendingCount() != 0 {
		t.Errorf("empty: got %d, want 0", rc.PendingCount())
	}

	dummyPtr := uintptr(0xDEADBEEF)
	view := gpucontext.NewTextureView(unsafe.Pointer(dummyPtr)) //nolint:gosec // test dummy
	rc.QueueBaseLayer(makeTestTarget(600, 400), view,
		0, 0, 600, 400, 1.0, 600, 400)

	if rc.PendingCount() != 1 {
		t.Errorf("with baseLayer: got %d, want 1", rc.PendingCount())
	}
}

func TestPendingCount_OverlayOnly(t *testing.T) {
	rc := &GPURenderContext{shared: NewGPUShared()}

	dummyPtr := uintptr(0xDEADBEEF)
	view := gpucontext.NewTextureView(unsafe.Pointer(dummyPtr)) //nolint:gosec // test dummy
	rc.QueueGPUTextureDraw(makeTestTarget(600, 400), view,
		100, 100, 48, 48, 1.0, 600, 400)

	if rc.PendingCount() != 1 {
		t.Errorf("with overlay: got %d, want 1", rc.PendingCount())
	}
}

func TestPendingCount_BaseLayerPlusOverlay(t *testing.T) {
	rc := &GPURenderContext{shared: NewGPUShared()}

	dummyPtr := uintptr(0xDEADBEEF)
	view := gpucontext.NewTextureView(unsafe.Pointer(dummyPtr)) //nolint:gosec // test dummy
	target := makeTestTarget(600, 400)

	rc.QueueBaseLayer(target, view, 0, 0, 600, 400, 1.0, 600, 400)
	rc.QueueGPUTextureDraw(target, view, 100, 100, 48, 48, 1.0, 600, 400)

	if rc.PendingCount() != 2 {
		t.Errorf("base+overlay: got %d, want 2", rc.PendingCount())
	}
}

// --- isBlitOnly tests ---

func TestIsBlitOnly_BaseLayerOnly(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	baseRes := &imageFrameResources{
		drawCalls: []imageDrawCall{{firstVertex: 0}},
	}

	if !s.isBlitOnly(nil, baseRes) {
		t.Error("base layer only should be blit-only")
	}
}

func TestIsBlitOnly_NoBaseLayer(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	if s.isBlitOnly(nil, nil) {
		t.Error("no base layer should not be blit-only")
	}
}

func TestIsBlitOnly_WithSDFShapes(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	baseRes := &imageFrameResources{
		drawCalls: []imageDrawCall{{firstVertex: 0}},
	}
	grpRes := []groupResources{{
		sdfRes: &sdfFrameResources{vertCount: 6},
	}}

	if s.isBlitOnly(grpRes, baseRes) {
		t.Error("SDF shapes present — should NOT be blit-only")
	}
}

// --- RenderFrameGrouped guard tests ---

func TestRenderFrameGrouped_BaseLayerOnlyGuard(t *testing.T) {
	// Regression test for BUG-GG-BLIT-PATH-001:
	// totalItems==0 && baseLayer!=nil must NOT early-return.
	// We test the guard logic directly without full render pipeline.
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	// Verify: nil groups + nil baseLayer → should return nil (skip).
	target := gg.GPURenderTarget{
		Data:   make([]byte, 600*400*4),
		Width:  600,
		Height: 400,
		Stride: 600 * 4,
	}
	err := s.RenderFrameGrouped(target, nil, nil, nil)
	if err != nil {
		t.Errorf("nil groups + nil baseLayer should return nil, got %v", err)
	}

	// Verify: empty groups (totalItems=0) + nil baseLayer → skip.
	err = s.RenderFrameGrouped(target, []ScissorGroup{{}}, nil, nil)
	if err != nil {
		t.Errorf("empty groups + nil baseLayer should return nil, got %v", err)
	}

	// Verify: isBlitOnly with base layer returns true.
	baseRes := &imageFrameResources{
		drawCalls: []imageDrawCall{{firstVertex: 0}},
	}
	if !s.isBlitOnly(nil, baseRes) {
		t.Error("base layer only should be blit-only")
	}
	if !s.isBlitOnly([]groupResources{{}}, baseRes) {
		t.Error("base layer + empty groups should be blit-only")
	}
}

// --- Helpers ---

type testVertex struct {
	px, py, u, v float32
}

func decodeVertices(t *testing.T, buf []byte) []testVertex {
	t.Helper()
	count := len(buf) / imageVertexStride
	verts := make([]testVertex, count)
	for i := range count {
		off := i * imageVertexStride
		verts[i] = testVertex{
			px: math.Float32frombits(binary.LittleEndian.Uint32(buf[off:])),
			py: math.Float32frombits(binary.LittleEndian.Uint32(buf[off+4:])),
			u:  math.Float32frombits(binary.LittleEndian.Uint32(buf[off+8:])),
			v:  math.Float32frombits(binary.LittleEndian.Uint32(buf[off+12:])),
		}
	}
	return verts
}

func assertVertex(t *testing.T, v testVertex, px, py, u, uv float32, label string) {
	t.Helper()
	const eps = 0.001
	if abs32(v.px-px) > eps || abs32(v.py-py) > eps || abs32(v.u-u) > eps || abs32(v.v-uv) > eps {
		t.Errorf("%s: got (%.1f, %.1f, %.3f, %.3f), want (%.1f, %.1f, %.3f, %.3f)",
			label, v.px, v.py, v.u, v.v, px, py, u, uv)
	}
}

func assertFloat(t *testing.T, got, want float32, label string) {
	t.Helper()
	const eps = 0.0001
	if abs32(got-want) > eps {
		t.Errorf("%s: got %f, want %f", label, got, want)
	}
}

// --- Regression: BUG-GG-GPU-TEXTURE-OVERLAY-SIZE ---

func TestBuildGPUTextureResources_SeparateVertexBuffers(t *testing.T) {
	// Regression test for BUG-GG-GPU-TEXTURE-OVERLAY-SIZE:
	// Base layer (full-screen quad) must NOT overwrite overlay vertices.
	// Before fix: both used s.gpuTexVertBuf → base layer overwrote overlay.
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	overlayCmd := GPUTextureDrawCommand{
		DstX: 100, DstY: 100, DstW: 48, DstH: 48,
		Opacity: 1.0, ViewportWidth: 600, ViewportHeight: 400,
	}
	baseCmd := GPUTextureDrawCommand{
		DstX: 0, DstY: 0, DstW: 600, DstH: 400,
		Opacity: 1.0, ViewportWidth: 600, ViewportHeight: 400,
	}

	// Build overlay first, then base layer (same order as RenderFrameGrouped).
	overlayRes, err := s.buildGPUTextureResources(
		[]GPUTextureDrawCommand{overlayCmd}, 600, 400, false)
	if err != nil {
		t.Fatalf("overlay build: %v", err)
	}

	baseRes, err := s.buildGPUTextureResources(
		[]GPUTextureDrawCommand{baseCmd}, 600, 400, true)
	if err != nil {
		t.Fatalf("base build: %v", err)
	}

	// Key assertion: overlay and base layer must use DIFFERENT vertex buffers.
	if overlayRes.vertBuf == baseRes.vertBuf {
		t.Fatal("REGRESSION: overlay and base layer share the same vertex buffer — " +
			"base layer will overwrite overlay vertices (BUG-GG-GPU-TEXTURE-OVERLAY-SIZE)")
	}

	// Verify overlay vertex buffer is the overlay buffer.
	if overlayRes.vertBuf != s.gpuTexVertBuf {
		t.Error("overlay should use s.gpuTexVertBuf")
	}

	// Verify base layer vertex buffer is the base layer buffer.
	if baseRes.vertBuf != s.gpuTexBaseVertBuf {
		t.Error("base layer should use s.gpuTexBaseVertBuf")
	}
}

// abs32 already defined in vello_tiles.go
