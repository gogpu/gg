//go:build !nogpu

package gpu

import (
	"sync"
)

// defaultTexturePoolBudgetMB is the default memory budget for the texture pool.
// 128MB supports ~5 concurrent 1080p MSAA4x contexts.
const defaultTexturePoolBudgetMB = 128

// textureKey identifies a texture set by its dimensions and sample count.
type textureKey struct {
	width       uint32
	height      uint32
	sampleCount uint32
}

// TexturePool manages reusable MSAA/stencil texture sets across GPURenderContexts.
// It follows the Flutter RenderTargetCache pattern: Acquire during flush,
// Release at frame end, EndFrame frees unused entries.
//
// This avoids creating expensive GPU textures (MSAA, depth/stencil) per context
// per frame when multiple contexts share the same dimensions.
type TexturePool struct {
	mu       sync.Mutex
	pool     map[textureKey][]*textureSet // available textures by size
	inUse    map[textureKey]int           // count in use this frame
	budgetMB int                          // max memory in megabytes
}

// NewTexturePool creates a texture pool with the given memory budget in MB.
func NewTexturePool(budgetMB int) *TexturePool {
	if budgetMB <= 0 {
		budgetMB = defaultTexturePoolBudgetMB
	}
	return &TexturePool{
		pool:     make(map[textureKey][]*textureSet),
		inUse:    make(map[textureKey]int),
		budgetMB: budgetMB,
	}
}

// SetBudget changes the memory budget. Excess textures are freed on EndFrame.
func (tp *TexturePool) SetBudget(mb int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	if mb > 0 {
		tp.budgetMB = mb
	}
}

// Acquire returns a textureSet matching the requested dimensions.
// If a matching set exists in the pool, it is reused. Otherwise nil is
// returned and the caller should create a new textureSet.
func (tp *TexturePool) Acquire(w, h, samples uint32) *textureSet { //nolint:revive // textureSet is internal; exported for GPUShared access within the package
	tp.mu.Lock()
	defer tp.mu.Unlock()
	key := textureKey{width: w, height: h, sampleCount: samples}
	if sets := tp.pool[key]; len(sets) > 0 {
		ts := sets[len(sets)-1]
		tp.pool[key] = sets[:len(sets)-1]
		tp.inUse[key]++
		return ts
	}
	tp.inUse[key]++
	return nil
}

// Release returns a textureSet to the pool for reuse. The caller must not
// use the textureSet after calling Release.
func (tp *TexturePool) Release(ts *textureSet) {
	if ts == nil {
		return
	}
	tp.mu.Lock()
	defer tp.mu.Unlock()
	key := textureKey{width: ts.width, height: ts.height, sampleCount: sampleCount}
	tp.pool[key] = append(tp.pool[key], ts)
	if tp.inUse[key] > 0 {
		tp.inUse[key]--
	}
}

// EndFrame frees unused texture sets that were not acquired during this frame.
// Call this once per frame after all contexts have flushed.
func (tp *TexturePool) EndFrame() {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	// Reset in-use counts for next frame.
	for k := range tp.inUse {
		tp.inUse[k] = 0
	}
	// Free excess sets that exceed budget (keep at most 2 per key).
	for key, sets := range tp.pool {
		if len(sets) > 2 {
			for _, ts := range sets[2:] {
				ts.destroyTextures()
			}
			tp.pool[key] = sets[:2]
		}
	}
}

// DestroyAll releases all pooled textures.
func (tp *TexturePool) DestroyAll() {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	for key, sets := range tp.pool {
		for _, ts := range sets {
			ts.destroyTextures()
		}
		delete(tp.pool, key)
	}
	tp.inUse = make(map[textureKey]int)
}

// textureSetEstimatedBytes returns the estimated GPU memory usage for a
// textureSet at the given dimensions. Used for budget calculations.
func textureSetEstimatedBytes(w, h uint32) uint64 {
	// MSAA 4x BGRA8 = w * h * 4 * 4
	// Depth/stencil 4x = w * h * 4 * 4 (approx)
	// Resolve 1x BGRA8 = w * h * 4
	pixels := uint64(w) * uint64(h)
	return pixels*4*uint64(sampleCount) + pixels*4*uint64(sampleCount) + pixels*4
}

// EstimatedUsageMB returns the estimated GPU memory used by all pooled textures in MB.
func (tp *TexturePool) EstimatedUsageMB() int {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	var total uint64
	for _, sets := range tp.pool {
		for _, ts := range sets {
			total += textureSetEstimatedBytes(ts.width, ts.height)
		}
	}
	return int(total / (1024 * 1024))
}

// PooledCount returns the total number of texture sets currently in the pool
// (available for reuse, not currently in use).
func (tp *TexturePool) PooledCount() int {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	count := 0
	for _, sets := range tp.pool {
		count += len(sets)
	}
	return count
}
