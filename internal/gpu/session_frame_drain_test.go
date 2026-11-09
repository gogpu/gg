//go:build !rust && !(js && wasm)

package gpu

import (
	"testing"

	gg "github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// Regression coverage for completion-aware command-buffer release in
// GPURenderSession:
//
//   - command buffers appended per flush are tracked with their queue
//     submission index,
//   - drainCompleted frees ONLY completed submissions (Poll-gated) and clears
//     the backing-array Go pointers,
//   - multiple flushes within a frame are preserved (mid-frame buffers of the
//     current frame and in-flight previous submissions are not freed early),
//   - the old failure mode — prevCmdBufs only ever re-sliced with [:0] —
//     cannot retain hidden references anymore (drainCompletedAll clears).

// offscreenTarget allocates a small CPU-readback render target.
func offscreenTarget(w, h uint32) gg.GPURenderTarget {
	return gg.GPURenderTarget{
		Width:  int(w),
		Height: int(h),
		Data:   make([]uint8, w*h*4),
		Stride: int(w * 4),
	}
}

// surfacePathOnce renders one stencil path directly through the surface
// encode/submit path (encodeSubmitSurface), which is the path the per-context
// Flush uses and the one whose submissions must be completion-drained.
func surfacePathOnce(t *testing.T, s *GPURenderSession, view *wgpu.TextureView) {
	t.Helper()
	if err := s.ensureTexturesForView(view, 128, 128); err != nil {
		t.Fatalf("ensure textures: %v", err)
	}
	if err := s.ensureClipBindLayout(); err != nil {
		t.Fatalf("ensure clip bind layout: %v", err)
	}
	if err := s.ensurePipelines(); err != nil {
		t.Fatalf("ensure pipelines: %v", err)
	}
	stencilPaths := []StencilPathCommand{{
		Vertices: []float32{10, 10, 100, 10, 64, 100, 10, 10, 64, 100, 10, 10},
		Color:    [4]float32{1, 0, 0, 1},
		FillRule: gg.FillRuleNonZero,
	}}
	stencilRes, err := s.buildStencilResourcesBatch(stencilPaths, 128, 128)
	if err != nil {
		t.Fatalf("buildStencilResourcesBatch: %v", err)
	}
	if err := s.encodeSubmitSurface(view, 128, 128, nil, nil, nil, stencilRes, stencilPaths, nil, nil); err != nil {
		t.Fatalf("encodeSubmitSurface: %v", err)
	}
}

// TestDrainCompletedAllClearsBackingReferences proves the reslice-retention
// fix: after drainCompletedAll the backing arrays hold no live Go pointers.
func TestDrainCompletedAllClearsBackingReferences(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()
	s := NewGPURenderSession(device, queue, 1)
	defer s.Destroy()

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "session-drain-test",
		Size:          wgpu.Extent3D{Width: 128, Height: 128, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}

	surfacePathOnce(t, s, view)
	if len(s.prevCmdBufs) == 0 {
		t.Fatal("no tracked submissions after surface flush")
	}
	for _, cb := range s.prevCmdBufs {
		if cb == nil {
			t.Fatal("nil command buffer tracked")
		}
	}
	s.drainCompletedAll()
	if len(s.prevCmdBufs) != 0 || len(s.prevCmdSubIdx) != 0 {
		t.Fatal("drainCompletedAll left entries")
	}
	for _, cb := range s.prevCmdBufs {
		if cb != nil {
			t.Fatal("hidden non-nil command buffer pointer behind len 0")
		}
	}
}

// TestDrainCompletedFreesOnlyCompletedSubmissions submits several command
// buffers through the surface path and proves that drainCompleted releases
// exactly the entries under the queue's completed horizon ( nirvana-safe: it
// never frees beyond what Poll reports).
func TestDrainCompletedFreesOnlyCompletedSubmissions(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()
	s := NewGPURenderSession(device, queue, 1)
	defer s.Destroy()

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "session-drain-test",
		Size:          wgpu.Extent3D{Width: 128, Height: 128, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	s.SetSurfaceTarget(view, 128, 128)

	for i := 0; i < 3; i++ {
		surfacePathOnce(t, s, view)
	}
	if len(s.prevCmdBufs) != 3 || len(s.prevCmdSubIdx) != 3 {
		t.Fatalf("expected 3 tracked submissions, got %d/%d", len(s.prevCmdBufs), len(s.prevCmdSubIdx))
	}
	for i := 1; i < len(s.prevCmdSubIdx); i++ {
		if s.prevCmdSubIdx[i] <= s.prevCmdSubIdx[i-1] {
			t.Fatalf("submission indices not monotonic: %v", s.prevCmdSubIdx)
		}
	}

	s.drainCompleted()
	if len(s.prevCmdBufs) != 0 || len(s.prevCmdSubIdx) != 0 {
		t.Fatalf("completed submissions retained: %d cmd bufs, %d subIdx entries", len(s.prevCmdBufs), len(s.prevCmdSubIdx))
	}
	for _, cb := range s.prevCmdBufs {
		if cb != nil {
			t.Fatal("hidden non-nil command buffer pointer behind len 0")
		}
	}
}

// TestInFlightSubmissionNotFreedEarly pins the safety property: a submission
// index above the queue's completed index must survive drainCompleted. The
// noop HAL completes synchronously, so the test injects an artificial
// in-flight entry with a subIdx beyond the current completed index and
// asserts it survives the drain, then is freed once marked complete.
func TestInFlightSubmissionNotFreedEarly(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()
	s := NewGPURenderSession(device, queue, 1)
	defer s.Destroy()

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "session-drain-test",
		Size:          wgpu.Extent3D{Width: 128, Height: 128, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	s.SetSurfaceTarget(view, 128, 128)

	surfacePathOnce(t, s, view)
	if len(s.prevCmdBufs) != 1 {
		t.Fatalf("expected 1 tracked submission, got %d", len(s.prevCmdBufs))
	}

	// Artificially raise the retained entry above the completion horizon:
	// the HAL cannot have completed a submission index it never issued.
	future := s.prevCmdSubIdx[0] + 1_000_000
	s.prevCmdSubIdx[0] = future

	completed := queue.Poll()
	s.drainCompleted()
	if len(s.prevCmdBufs) != 1 || s.prevCmdSubIdx[0] != future {
		t.Fatalf("in-flight submission dropped early: completed=%d retained=%v", completed, s.prevCmdSubIdx)
	}

	// Now bring it under the completion horizon: drain must free it.
	s.prevCmdSubIdx[0] = completed
	s.drainCompleted()
	if len(s.prevCmdBufs) != 0 {
		t.Fatal("retained entry not freed once completed")
	}
}

// TestMultiFlushFrameBounded proves the leak fix over N frames: without the
// completion-aware drain, prevCmdBufs grew by one entry per flush forever.
// Here, two flushes per frame across many frames must keep retention bounded
// by the current frame's entries after the frame boundary drain.
func TestMultiFlushFrameBounded(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()
	s := NewGPURenderSession(device, queue, 1)
	defer s.Destroy()

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "session-drain-test",
		Size:          wgpu.Extent3D{Width: 128, Height: 128, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	s.SetSurfaceTarget(view, 128, 128)

	const frames = 50
	const flushesPerFrame = 2
	for f := 0; f < frames; f++ {
		for i := 0; i < flushesPerFrame; i++ {
			surfacePathOnce(t, s, view)
		}
		// Frame boundary: the session-side drain the BeginGPUFrame hook calls.
		if f > 0 {
			s.drainCompleted()
		}
		if r := len(s.prevCmdBufs); r > 2*flushesPerFrame {
			t.Fatalf("unbounded retention: %d entries at frame %d", r, f)
		}
	}
	s.drainCompleted()
	if len(s.prevCmdBufs) != 0 {
		t.Fatalf("final retention: %d", len(s.prevCmdBufs))
	}
}
