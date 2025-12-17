package scene

import "sync"

// EncodingPool manages a pool of reusable Encoding objects.
// After warmup, allocations are minimized by reusing encodings.
//
// Usage:
//
//	pool := NewEncodingPool()
//	enc := pool.Get()
//	defer pool.Put(enc)
//	// use enc...
type EncodingPool struct {
	pool sync.Pool
}

// NewEncodingPool creates a new encoding pool.
func NewEncodingPool() *EncodingPool {
	return &EncodingPool{
		pool: sync.Pool{
			New: func() any {
				return NewEncoding()
			},
		},
	}
}

// Get retrieves an encoding from the pool.
// The encoding is reset and ready for use.
func (p *EncodingPool) Get() *Encoding {
	enc := p.pool.Get().(*Encoding)
	enc.Reset()
	return enc
}

// Put returns an encoding to the pool for reuse.
// The encoding will be reset on the next Get.
func (p *EncodingPool) Put(enc *Encoding) {
	if enc == nil {
		return
	}
	p.pool.Put(enc)
}

// Warmup pre-allocates encodings to avoid allocation during critical paths.
// Call this during initialization if allocation-free operation is required.
func (p *EncodingPool) Warmup(count int) {
	encodings := make([]*Encoding, count)
	for i := 0; i < count; i++ {
		encodings[i] = p.Get()
	}
	for i := 0; i < count; i++ {
		p.Put(encodings[i])
	}
}

// DefaultPool is a global encoding pool for convenience.
// For performance-critical code, consider creating dedicated pools.
var DefaultPool = NewEncodingPool()

// GetEncoding retrieves an encoding from the default pool.
func GetEncoding() *Encoding {
	return DefaultPool.Get()
}

// PutEncoding returns an encoding to the default pool.
func PutEncoding(enc *Encoding) {
	DefaultPool.Put(enc)
}
