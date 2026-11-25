// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package gg

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gogpu/gpucontext"
)

// raceMockOps is a no-op gpuContextOps stub so the lazy-creation path of
// ensureGPUCtx can run without importing internal/gpu.
type raceMockOps struct{}

var _ gpuContextOps = (*raceMockOps)(nil)

func (*raceMockOps) FillShape(GPURenderTarget, DetectedShape, *Paint) error   { return nil }
func (*raceMockOps) StrokeShape(GPURenderTarget, DetectedShape, *Paint) error { return nil }
func (*raceMockOps) FillPath(GPURenderTarget, *Path, *Paint) error            { return nil }
func (*raceMockOps) StrokePath(GPURenderTarget, *Path, *Paint) error          { return nil }
func (*raceMockOps) DrawText(GPURenderTarget, any, string, float64, float64, RGBA, Matrix, float64) error {
	return nil
}

func (*raceMockOps) DrawGlyphMaskText(GPURenderTarget, any, string, float64, float64, RGBA, Matrix, float64) error {
	return nil
}

func (*raceMockOps) QueueImageDraw(GPURenderTarget, []byte, uint64, int, int, int, float32, float32, float32, float32, float32, uint32, uint32, float32, float32, float32, float32) {
}

func (*raceMockOps) QueueGPUTextureDraw(GPURenderTarget, gpucontext.TextureView, float32, float32, float32, float32, float32, uint32, uint32) {
}

func (*raceMockOps) QueueBaseLayer(GPURenderTarget, gpucontext.TextureView, float32, float32, float32, float32, float32, uint32, uint32) {
}
func (*raceMockOps) Flush(GPURenderTarget) error             { return nil }
func (*raceMockOps) SetClipRect(x, y, w, h uint32)           {}
func (*raceMockOps) ClearClipRect()                          {}
func (*raceMockOps) SetClipRRect(x, y, w, h, radius float32) {}
func (*raceMockOps) ClearClipRRect()                         {}
func (*raceMockOps) SetClipPath(*Path)                       {}
func (*raceMockOps) ClearClipPath()                          {}
func (*raceMockOps) BeginFrame()                             {}
func (*raceMockOps) MarkFrameRendered()                      {}
func (*raceMockOps) SetPipelineMode(PipelineMode)            {}
func (*raceMockOps) SetAntiAlias(bool)                       {}
func (*raceMockOps) PendingCount() int                       { return 0 }
func (*raceMockOps) Close()                                  {}

// raceMockAccelerator is a GPUAccelerator that also provides per-context render
// contexts, counting how many it creates.
type raceMockAccelerator struct {
	mockAccelerator
	created atomic.Int32
}

var _ GPURenderContextProvider = (*raceMockAccelerator)(nil)

func (a *raceMockAccelerator) NewGPURenderContext() any {
	a.created.Add(1)
	return &raceMockOps{}
}

// TestContextCloseConcurrent pins BUG-RACE-002: concurrent Close calls run the
// cleanup exactly once through CompareAndSwap, and every caller gets nil.
// Under -race this also proves the flag itself is accessed race-free.
func TestContextCloseConcurrent(t *testing.T) {
	dc := NewContext(60, 40)

	const n = 32
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = dc.Close()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Close #%d: %v", i, err)
		}
	}
	if err := dc.Close(); err != nil {
		t.Errorf("Close after concurrent closes: %v", err)
	}
}

// TestGPURenderContextConcurrentCreate pins BUG-RACE-003: concurrent first use
// creates exactly one per-context render context rather than one per caller.
func TestGPURenderContextConcurrentCreate(t *testing.T) {
	resetAccelerator()
	accel := &raceMockAccelerator{}
	if err := RegisterAccelerator(accel); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}
	defer resetAccelerator()

	dc := NewContext(60, 40)

	const n = 16
	got := make([]any, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i] = dc.GPURenderContext()
		}(i)
	}
	wg.Wait()

	if c := accel.created.Load(); c != 1 {
		t.Errorf("NewGPURenderContext calls = %d, want 1", c)
	}
	for i, v := range got {
		if v == nil {
			t.Errorf("goroutine %d got a nil render context", i)
		}
	}
}
