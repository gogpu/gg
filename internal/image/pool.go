// Package image provides image buffer management for gogpu/gg.
package image

import "sync"

// Pool is a thread-safe pool for reusing ImageBuf instances.
//
// Pool groups buffers by their dimensions and format, allowing efficient
// reuse of identically-sized buffers. This reduces GC pressure for
// applications that frequently create and destroy images of similar sizes.
//
// Thread safety: All methods are safe for concurrent use.
type Pool struct {
	mu      sync.Mutex
	buckets map[poolKey][]*ImageBuf
	maxSize int // max buffers per bucket
}

// poolKey identifies a bucket of identical image specifications.
type poolKey struct {
	width  int
	height int
	format Format
}

// NewPool creates a new image buffer pool with the given maximum buffers per bucket.
// maxPerBucket limits how many buffers of each size/format are retained.
// A maxPerBucket of 0 means unlimited (use with caution).
func NewPool(maxPerBucket int) *Pool {
	return &Pool{
		buckets: make(map[poolKey][]*ImageBuf),
		maxSize: maxPerBucket,
	}
}

// Get retrieves an image buffer from the pool or creates a new one.
// The returned buffer is guaranteed to have the specified dimensions and format.
// If a buffer is reused from the pool, it will be cleared (all pixels zeroed).
func (p *Pool) Get(width, height int, format Format) *ImageBuf {
	key := poolKey{width: width, height: height, format: format}

	p.mu.Lock()
	bucket := p.buckets[key]
	var buf *ImageBuf

	if len(bucket) > 0 {
		// Pop from pool
		buf = bucket[len(bucket)-1]
		p.buckets[key] = bucket[:len(bucket)-1]
		p.mu.Unlock()

		// Clear buffer before reuse
		buf.Clear()
		return buf
	}
	p.mu.Unlock()

	// Create new buffer if pool is empty
	buf, err := NewImageBuf(width, height, format)
	if err != nil {
		// This should only happen with invalid params, but we need to handle it
		// gracefully. Return nil and let caller handle.
		return nil
	}
	return buf
}

// Put returns an image buffer to the pool for reuse.
// The buffer will be cleared before being stored.
// If buf is nil or the pool bucket is at max capacity, the buffer is discarded.
func (p *Pool) Put(buf *ImageBuf) {
	if buf == nil {
		return
	}

	// Clear buffer data before returning to pool
	buf.Clear()

	key := poolKey{
		width:  buf.width,
		height: buf.height,
		format: buf.format,
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	bucket := p.buckets[key]

	// Check if bucket is at capacity
	if p.maxSize > 0 && len(bucket) >= p.maxSize {
		// Bucket full, discard buffer (GC will clean up)
		return
	}

	// Add to pool
	p.buckets[key] = append(bucket, buf)
}

// defaultPool is the package-level pool for convenient usage.
var defaultPool = NewPool(8)

// GetFromDefault retrieves an image buffer from the default pool.
// This is a convenience wrapper around defaultPool.Get().
func GetFromDefault(width, height int, format Format) *ImageBuf {
	return defaultPool.Get(width, height, format)
}

// PutToDefault returns an image buffer to the default pool.
// This is a convenience wrapper around defaultPool.Put().
func PutToDefault(buf *ImageBuf) {
	defaultPool.Put(buf)
}
