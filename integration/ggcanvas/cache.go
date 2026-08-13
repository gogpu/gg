// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package ggcanvas

import (
	"math"
	"reflect"
	"sync"

	"github.com/gogpu/gpucontext"
)

// canvasCacheKey identifies the resources and logical geometry owned by a
// Canvas.  DeviceProvider values returned by gogpu.App are short-lived
// adapters, so the provider interface itself cannot be used as the identity.
// The opaque GPU handles, on the other hand, refer to the long-lived device,
// queue, and adapter and are comparable value types.
type canvasCacheKey struct {
	device  gpucontext.Device
	queue   gpucontext.Queue
	adapter gpucontext.Adapter

	// providerType/providerPtr are used only when all GPU handles are zero.
	// This keeps independent zero-device test providers from aliasing one
	// another while still allowing repeated calls with the same pointer mock to
	// reuse a Canvas.
	providerType reflect.Type
	providerPtr  uintptr

	width  int
	height int
	scale  float64
}

var canvasCache = struct {
	sync.Mutex
	entries map[canvasCacheKey]*Canvas
}{
	entries: make(map[canvasCacheKey]*Canvas),
}

// normalizeScale is shared by the cache and Canvas construction.  NaN is not
// a useful device scale and cannot be matched as a map key, so treat it like
// the existing non-positive fallback.  Infinite values are likewise rejected
// here before they can produce an invalid physical allocation.
func normalizeScale(scale float64) float64 {
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return 1.0
	}
	return scale
}

func cacheKeyFor(provider gpucontext.DeviceProvider, width, height int, scale float64) (canvasCacheKey, bool) {
	key := canvasCacheKey{
		width:  width,
		height: height,
		scale:  normalizeScale(scale),
	}
	if provider == nil {
		return key, false
	}

	key.device = provider.Device()
	key.queue = provider.Queue()
	key.adapter = provider.Adapter()
	if !key.device.IsNil() || !key.queue.IsNil() || !key.adapter.IsNil() {
		return key, true
	}

	// A zero-handle provider is common in CPU-only and unit-test setups.  Do
	// not collapse every such provider into one global Canvas.  Pointer-backed
	// providers have stable identity for the lifetime of the Canvas; value
	// providers have no portable identity, so conservatively disable caching.
	v := reflect.ValueOf(provider)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return key, false
	}
	key.providerType = v.Type()
	key.providerPtr = v.Pointer()
	return key, key.providerPtr != 0
}

func (c *Canvas) cacheKeyFor(width, height int, scale float64) (canvasCacheKey, bool) {
	return cacheKeyFor(c.provider, width, height, scale)
}

// lookupCachedCanvas returns a live Canvas for key, updating its window
// provider to the latest adapter wrapper. Cache and lifecycle locks are never
// held together: Close takes lifecycle then cache, while this helper releases
// cache before taking lifecycle. A candidate that closes in that interval is
// discarded and looked up again.
func lookupCachedCanvas(provider gpucontext.DeviceProvider, key canvasCacheKey) (*Canvas, bool) {
	for {
		canvasCache.Lock()
		cached := canvasCache.entries[key]
		if cached == nil {
			canvasCache.Unlock()
			return nil, false
		}
		canvasCache.Unlock()

		cached.lifecycleMu.Lock()
		if cached.closed {
			cached.lifecycleMu.Unlock()
			canvasCache.Lock()
			if canvasCache.entries[key] == cached {
				delete(canvasCache.entries, key)
				cached.cached = false
			}
			canvasCache.Unlock()
			continue
		}
		cached.windowProvider = nil
		if wp, ok := provider.(gpucontext.WindowProvider); ok {
			cached.windowProvider = wp
		}
		cached.lifecycleMu.Unlock()
		return cached, true
	}
}

// removeFromCanvasCache removes c only if it is still the owner of its key.
// The owner check is important when a resized Canvas lost a key to another
// Canvas or when a concurrent constructor won the same key.
func (c *Canvas) removeFromCanvasCacheLocked() {
	if !c.cached {
		return
	}
	if current := canvasCache.entries[c.cacheKey]; current == c {
		delete(canvasCache.entries, c.cacheKey)
	}
	c.cached = false
}
