// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package ggcanvas

import (
	"errors"
	"io"
	"sync"
	"testing"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
)

// stableProvider is intentionally a fresh wrapper around the same opaque
// handles.  This models App.GPUContextProvider(), which returns a new adapter
// on every call while its device resources remain stable.
type stableProvider struct {
	*mockProvider
	device  gpucontext.Device
	queue   gpucontext.Queue
	adapter gpucontext.Adapter
	gpucontext.NullWindowProvider
}

type closeOnTrackProvider struct {
	*stableProvider
}

func (p *closeOnTrackProvider) TrackResource(c io.Closer) { _ = c.Close() }
func (p *closeOnTrackProvider) UntrackResource(io.Closer) {}

func newStableProvider(device, queue, adapter unsafe.Pointer) *stableProvider {
	return &stableProvider{
		mockProvider: newMockProvider(),
		device:       gpucontext.NewDevice(device),
		queue:        gpucontext.NewQueue(queue),
		adapter:      gpucontext.NewAdapter(adapter),
		NullWindowProvider: gpucontext.NullWindowProvider{
			W:  100,
			H:  100,
			SF: 1,
		},
	}
}

func (p *stableProvider) Device() gpucontext.Device   { return p.device }
func (p *stableProvider) Queue() gpucontext.Queue     { return p.queue }
func (p *stableProvider) Adapter() gpucontext.Adapter { return p.adapter }
func (p *stableProvider) Size() (int, int)            { return p.NullWindowProvider.Size() }
func (p *stableProvider) ScaleFactor() float64        { return p.NullWindowProvider.ScaleFactor() }
func (p *stableProvider) RequestRedraw()              {}

func TestNew_ReusesStableGPUHandlesAcrossProviderWrappers(t *testing.T) {
	device, queue, adapter := new(int), new(int), new(int)
	p1 := newStableProvider(unsafe.Pointer(device), unsafe.Pointer(queue), unsafe.Pointer(adapter)) //nolint:gosec // opaque test handles
	p2 := newStableProvider(unsafe.Pointer(device), unsafe.Pointer(queue), unsafe.Pointer(adapter)) //nolint:gosec // opaque test handles
	p1.W, p1.H = 80, 60
	p2.W, p2.H = 120, 90

	c1, err := NewWithScale(p1, 80, 60, 2)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := NewWithScale(p2, 80, 60, 2.0)
	if err != nil {
		t.Fatal(err)
	}
	if c1 != c2 {
		t.Fatal("provider wrappers with the same GPU handles should reuse the Canvas")
	}
	if err := c2.Draw(func(dc *gg.Context) {
		if dc.Width() != 120 || dc.Height() != 90 {
			t.Errorf("latest provider wrapper size = %dx%d, want 120x90", dc.Width(), dc.Height())
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := c1.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNew_ZeroHandleProvidersUsePointerFallback(t *testing.T) {
	p1 := newMockProvider()
	p2 := newMockProvider()
	c1, err := New(p1, 40, 30)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := New(p2, 40, 30)
	if err != nil {
		t.Fatal(err)
	}
	if c1 == c2 {
		t.Fatal("independent zero-handle providers must not alias")
	}
	defer c1.Close()
	defer c2.Close()

	c3, err := New(p1, 40, 30)
	if err != nil {
		t.Fatal(err)
	}
	if c3 != c1 {
		t.Fatal("same zero-handle provider pointer should reuse the Canvas")
	}
}

func TestNew_PreservesDistinctGeometryAndScale(t *testing.T) {
	p := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	c1, err := New(p, 80, 60)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := New(p, 100, 60)
	if err != nil {
		t.Fatal(err)
	}
	c3, err := NewWithScale(p, 80, 60, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	defer c2.Close()
	defer c3.Close()
	if c1 == c2 || c1 == c3 || c2 == c3 {
		t.Fatal("different dimensions or scale must retain independent canvases")
	}
}

func TestNew_CloseEvictsBeforeRecreate(t *testing.T) {
	p := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	c1, err := New(p, 80, 60)
	if err != nil {
		t.Fatal(err)
	}
	if err := c1.Close(); err != nil {
		t.Fatal(err)
	}
	c2, err := New(p, 80, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	if c1 == c2 {
		t.Fatal("a closed Canvas must not be returned from the cache")
	}
	if c2.Context() == nil {
		t.Fatal("recreated Canvas has no drawing context")
	}
}

func TestResize_RekeysCacheWithoutEvictingOtherGeometry(t *testing.T) {
	p := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	c1, err := New(p, 80, 60)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := New(p, 120, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	defer c2.Close()

	if err := c1.Resize(160, 120); err != nil {
		t.Fatal(err)
	}
	old, err := New(p, 80, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if old == c1 || old == c2 {
		t.Fatal("old geometry key still references a resized Canvas")
	}
	current, err := New(p, 160, 120)
	if err != nil {
		t.Fatal(err)
	}
	if current != c1 {
		t.Fatal("resized Canvas was not published under its new geometry key")
	}
	other, err := New(p, 120, 90)
	if err != nil {
		t.Fatal(err)
	}
	if other != c2 {
		t.Fatal("resize evicted an unrelated geometry key")
	}
}

func TestDraw_AutoResizesFromWindowProvider(t *testing.T) {
	p := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	c, err := New(p, 80, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p.W, p.H = 120, 90
	if err := c.Draw(func(dc *gg.Context) {
		if dc.Width() != 120 || dc.Height() != 90 {
			t.Errorf("Draw callback context size = %dx%d, want 120x90", dc.Width(), dc.Height())
		}
	}); err != nil {
		t.Fatal(err)
	}
	if gotW, gotH := c.Size(); gotW != 120 || gotH != 90 {
		t.Fatalf("Size after Draw = %dx%d, want 120x90", gotW, gotH)
	}
	// The resized Canvas is rekeyed, while the old geometry can be recreated.
	old, err := New(p, 80, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if old == c {
		t.Fatal("Draw resize left the old cache key pointing at the resized Canvas")
	}
}

func TestDraw_ZeroWindowSizeDoesNotResize(t *testing.T) {
	p := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	c, err := New(p, 80, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p.W, p.H = 0, 0
	if err := c.Draw(func(_ *gg.Context) {}); err != nil {
		t.Fatal(err)
	}
	if gotW, gotH := c.Size(); gotW != 80 || gotH != 60 {
		t.Fatalf("Size after zero-window Draw = %dx%d, want 80x60", gotW, gotH)
	}
}

func TestNew_ConcurrentDuplicateConstruction(t *testing.T) {
	p := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	const workers = 32
	results := make(chan *Canvas, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := NewWithScale(p, 100, 100, 1)
			results <- c
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	var first *Canvas
	for c := range results {
		if c == nil {
			t.Fatal("concurrent constructor returned nil Canvas")
		}
		if first == nil {
			first = c
		} else if c != first {
			t.Fatal("concurrent constructors returned duplicate live canvases")
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent constructor error: %v", err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNew_StaleClosedCacheEntryRetries(t *testing.T) {
	p := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	original, err := New(p, 64, 48)
	if err != nil {
		t.Fatal(err)
	}

	// Model the small interleave where a cache lookup observes the entry just
	// before Close marks it closed. The production Close path evicts normally;
	// retaining the entry here makes the retry branch deterministic.
	original.lifecycleMu.Lock()
	original.closed = true
	original.lifecycleMu.Unlock()
	canvasCache.Lock()
	original.cached = true
	canvasCache.entries[original.cacheKey] = original
	canvasCache.Unlock()

	recreated, err := New(p, 64, 48)
	if err != nil {
		t.Fatal(err)
	}
	if recreated == original {
		t.Fatal("stale closed cache entry was returned instead of recreated")
	}

	// Restore the synthetic state so the original context is torn down by its
	// normal idempotent Close path as well.
	original.lifecycleMu.Lock()
	original.closed = false
	original.lifecycleMu.Unlock()
	_ = original.Close()
	_ = recreated.Close()
}

func TestNew_TrackerCloseDoesNotPublishClosedCanvas(t *testing.T) {
	p := &closeOnTrackProvider{stableProvider: newStableProvider(
		unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), //nolint:gosec // opaque test handles
	)}
	canvas, err := NewWithScale(p, 64, 48, 1)
	if canvas != nil {
		t.Fatal("tracker-closed construction returned a Canvas")
	}
	if err == nil || !errors.Is(err, ErrCanvasClosed) {
		t.Fatalf("tracker-closed construction error = %v, want ErrCanvasClosed", err)
	}
	key, cacheable := cacheKeyFor(p, 64, 48, 1)
	if cacheable {
		if cached, ok := lookupCachedCanvas(p, key); ok || cached != nil {
			t.Fatal("tracker-closed construction left a stale cache entry")
		}
	}
}
