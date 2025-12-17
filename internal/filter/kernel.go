package filter

import (
	"math"
	"sync"
)

// GaussianKernel generates a 1D Gaussian kernel for the given radius.
// The kernel is normalized so all values sum to 1.0.
//
// The kernel size is computed as 2 * ceil(radius * 3) + 1, which covers
// 99.7% of the Gaussian distribution (3 standard deviations).
//
// For radius <= 0, returns a single-element kernel [1.0] (identity).
func GaussianKernel(radius float64) []float32 {
	if radius <= 0 {
		return []float32{1.0}
	}

	// Kernel size: 2 * ceil(sigma * 3) + 1
	// Using radius as sigma
	sigma := radius
	halfSize := int(math.Ceil(sigma * 3))
	size := halfSize*2 + 1

	kernel := make([]float32, size)

	// Gaussian formula: G(x) = exp(-x²/(2σ²)) / (σ√(2π))
	// We skip the normalization constant since we'll normalize sum to 1
	twoSigmaSq := 2 * sigma * sigma
	sum := float64(0)

	for i := 0; i < size; i++ {
		x := float64(i - halfSize)
		val := math.Exp(-(x * x) / twoSigmaSq)
		kernel[i] = float32(val)
		sum += val
	}

	// Normalize so kernel sums to 1.0
	if sum > 0 {
		invSum := float32(1.0 / sum)
		for i := range kernel {
			kernel[i] *= invSum
		}
	}

	return kernel
}

// BoxKernel generates a 1D box (uniform) kernel for the given radius.
// All values are equal: 1/(2*radius+1).
//
// Box blur is faster than Gaussian but produces blocky results.
// Three passes of box blur approximate Gaussian blur well.
func BoxKernel(radius int) []float32 {
	if radius <= 0 {
		return []float32{1.0}
	}

	size := radius*2 + 1
	kernel := make([]float32, size)
	val := float32(1.0) / float32(size)

	for i := range kernel {
		kernel[i] = val
	}

	return kernel
}

// kernelCache caches computed Gaussian kernels to avoid recomputation.
// Key is radius * 100 (to handle float precision), value is kernel.
type kernelCache struct {
	mu     sync.RWMutex
	cache  map[int][]float32
	maxLen int
}

var defaultKernelCache = newKernelCache(64)

// newKernelCache creates a kernel cache with the given maximum entries.
func newKernelCache(maxLen int) *kernelCache {
	return &kernelCache{
		cache:  make(map[int][]float32),
		maxLen: maxLen,
	}
}

// get retrieves a kernel from cache or generates and caches it.
func (c *kernelCache) get(radius float64) []float32 {
	// Quantize radius to 0.01 precision
	key := int(radius * 100)

	// Try read lock first
	c.mu.RLock()
	if kernel, ok := c.cache[key]; ok {
		c.mu.RUnlock()
		return kernel
	}
	c.mu.RUnlock()

	// Generate kernel
	kernel := GaussianKernel(radius)

	// Cache with write lock
	c.mu.Lock()
	if len(c.cache) >= c.maxLen {
		// Simple eviction: clear half the cache
		// In production, LRU would be better
		count := 0
		for k := range c.cache {
			delete(c.cache, k)
			count++
			if count >= c.maxLen/2 {
				break
			}
		}
	}
	c.cache[key] = kernel
	c.mu.Unlock()

	return kernel
}

// CachedGaussianKernel returns a cached Gaussian kernel for the radius.
// This is more efficient when the same radius is used repeatedly.
func CachedGaussianKernel(radius float64) []float32 {
	return defaultKernelCache.get(radius)
}

// OptimalKernelSize returns the optimal kernel size for a given radius.
// This is useful for pre-allocating buffers.
func OptimalKernelSize(radius float64) int {
	if radius <= 0 {
		return 1
	}
	halfSize := int(math.Ceil(radius * 3))
	return halfSize*2 + 1
}

// KernelCenter returns the center index of a kernel of the given size.
func KernelCenter(kernelSize int) int {
	return kernelSize / 2
}
