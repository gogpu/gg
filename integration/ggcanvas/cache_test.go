// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package ggcanvas

import (
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
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

// valueProvider deliberately has no stable identity. A value provider is a
// valid CPU-only DeviceProvider, but cacheKeyFor conservatively leaves it
// uncacheable because two equal values need not represent the same resources.
type valueProvider struct {
	format gputypes.TextureFormat
}

func newValueProvider() valueProvider {
	return valueProvider{format: gputypes.TextureFormatBGRA8Unorm}
}

func (p valueProvider) Device() gpucontext.Device             { return gpucontext.Device{} }
func (p valueProvider) Queue() gpucontext.Queue               { return gpucontext.Queue{} }
func (p valueProvider) Adapter() gpucontext.Adapter           { return gpucontext.Adapter{} }
func (p valueProvider) SurfaceFormat() gputypes.TextureFormat { return p.format }
func (valueProvider) AdapterInfo() gpucontext.AdapterInfo {
	return gpucontext.AdapterInfo{Type: gpucontext.AdapterTypeUnknown}
}

type closeValueProvider struct {
	valueProvider
}

func (closeValueProvider) TrackResource(c io.Closer) { _ = c.Close() }
func (closeValueProvider) UntrackResource(io.Closer) {}

// barrierProvider keeps constructors together after resource tracking and
// before cache publication. This makes the loser-cleanup path deterministic
// instead of relying on scheduler timing.
type barrierProvider struct {
	*stableProvider
	arrived chan struct{}
	release <-chan struct{}
}

func (p *barrierProvider) TrackResource(c io.Closer) {
	p.arrived <- struct{}{}
	<-p.release
}

func (p *barrierProvider) UntrackResource(io.Closer) {}

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

func TestNew_ValueProviderSkipsCache(t *testing.T) {
	p := newValueProvider()
	c1, err := NewWithScale(p, 64, 48, 1)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := NewWithScale(p, 64, 48, 1)
	if err != nil {
		c1.Close()
		t.Fatal(err)
	}
	defer c1.Close()
	defer c2.Close()
	if c1 == c2 {
		t.Fatal("value providers must not alias through the Canvas cache")
	}
}

func TestNew_ValueProviderTrackerCloseIsRejected(t *testing.T) {
	p := closeValueProvider{valueProvider: newValueProvider()}
	c, err := NewWithScale(p, 64, 48, 1)
	if c != nil {
		t.Fatal("tracker-closed value-provider construction returned a Canvas")
	}
	if !errors.Is(err, ErrCanvasClosed) {
		t.Fatalf("tracker-closed value-provider construction error = %v, want ErrCanvasClosed", err)
	}
}

func TestNewWithScale_NormalizesNonFiniteScale(t *testing.T) {
	for _, scale := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		t.Run(fmt.Sprintf("scale_%v", scale), func(t *testing.T) {
			p := newStableProvider(
				unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), //nolint:gosec // opaque test handles
			)
			c, err := NewWithScale(p, 64, 48, scale)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if got := c.DeviceScale(); got != 1 {
				t.Fatalf("DeviceScale for %v = %v, want 1", scale, got)
			}
			reused, err := NewWithScale(p, 64, 48, 1)
			if err != nil {
				t.Fatal(err)
			}
			if reused != c {
				t.Fatalf("normalized scale %v did not reuse the scale-1 Canvas", scale)
			}
		})
	}
}

func TestSetDeviceScale_RekeysAndPreservesDestinationOwner(t *testing.T) {
	p := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	c, err := NewWithScale(p, 64, 48, 1)
	if err != nil {
		t.Fatal(err)
	}
	if c.DeviceScale() != 1 {
		t.Fatalf("initial DeviceScale = %v, want 1", c.DeviceScale())
	}
	c.SetDeviceScale(1) // unchanged scale is a no-op
	for _, invalid := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		c.SetDeviceScale(invalid)
	}
	if c.DeviceScale() != 1 {
		t.Fatalf("invalid scale changed DeviceScale to %v", c.DeviceScale())
	}
	c.SetDeviceScale(2)
	if got := c.DeviceScale(); got != 2 {
		t.Fatalf("DeviceScale after change = %v, want 2", got)
	}
	if !c.IsDirty() {
		t.Fatal("changing device scale must mark the Canvas dirty")
	}
	reused, err := NewWithScale(p, 64, 48, 2)
	if err != nil {
		t.Fatal(err)
	}
	if reused != c {
		t.Fatal("scale change did not publish the Canvas under its new key")
	}
	oldScale, err := NewWithScale(p, 64, 48, 1)
	if err != nil {
		t.Fatal(err)
	}
	if oldScale == c {
		t.Fatal("old scale key still points at the rescaled Canvas")
	}
	oldScale.Close()
	c.Close()
	c.SetDeviceScale(3) // closed canvases ignore scale changes

	p2 := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	first, err := NewWithScale(p2, 64, 48, 1)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := NewWithScale(p2, 64, 48, 2)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	first.SetDeviceScale(2)
	if got, err := NewWithScale(p2, 64, 48, 2); err != nil || got != destination {
		t.Fatalf("rescale collision replaced destination Canvas: got=%p want=%p err=%v", got, destination, err)
	}
	first.Close()
	destination.Close()
}

func TestDraw_ClosedReturnsErrCanvasClosed(t *testing.T) {
	p := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	c, err := New(p, 32, 24)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Draw(nil); !errors.Is(err, ErrCanvasClosed) {
		t.Fatalf("Draw on closed Canvas error = %v, want ErrCanvasClosed", err)
	}
}

func TestRender_TracksWindowScaleChange(t *testing.T) {
	p := newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))) //nolint:gosec // opaque test handles
	c, err := New(p, 32, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p.SF = 2
	target := &renderMockPixelWriter{}
	if err := c.Render(target); err != nil {
		t.Fatal(err)
	}
	if got := c.DeviceScale(); got != 2 {
		t.Fatalf("Render did not apply window scale: got %v want 2", got)
	}
	if target.writtenW != 64 || target.writtenH != 48 {
		t.Fatalf("rendered physical size = %dx%d, want 64x48", target.writtenW, target.writtenH)
	}
}

func TestNew_ConcurrentDuplicateConstructionPublishesOneWinner(t *testing.T) {
	const workers = 8
	release := make(chan struct{})
	p := &barrierProvider{
		stableProvider: newStableProvider(unsafe.Pointer(new(int)), unsafe.Pointer(new(int)), unsafe.Pointer(new(int))), //nolint:gosec // opaque test handles
		arrived:        make(chan struct{}, workers),
		release:        release,
	}
	results := make(chan *Canvas, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := NewWithScale(p, 96, 72, 1)
			results <- c
			errs <- err
		}()
	}
	for i := 0; i < workers; i++ {
		select {
		case <-p.arrived:
		case <-time.After(2 * time.Second):
			t.Fatal("constructors did not all reach resource tracking")
		}
	}
	close(release)
	wg.Wait()
	close(results)
	close(errs)

	var winner *Canvas
	for c := range results {
		if c == nil {
			t.Fatal("concurrent constructor returned nil Canvas")
		}
		if winner == nil {
			winner = c
		} else if c != winner {
			t.Fatalf("concurrent constructors returned %p and %p", winner, c)
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent constructor error: %v", err)
		}
	}
	if winner == nil {
		t.Fatal("no winning Canvas")
	}
	if err := winner.Close(); err != nil {
		t.Fatal(err)
	}
}
