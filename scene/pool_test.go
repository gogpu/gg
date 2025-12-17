package scene

import (
	"sync"
	"testing"

	"github.com/gogpu/gg"
)

func TestEncodingPool(t *testing.T) {
	pool := NewEncodingPool()

	enc := pool.Get()
	if enc == nil {
		t.Fatal("Get() returned nil")
	}

	if !enc.IsEmpty() {
		t.Error("Get() should return empty encoding")
	}

	// Use the encoding
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)
	enc.EncodePath(path)

	// Return to pool
	pool.Put(enc)

	// Get again - should be reset
	enc2 := pool.Get()
	if !enc2.IsEmpty() {
		t.Error("Get() after Put() should return reset encoding")
	}
}

func TestEncodingPoolNilPut(t *testing.T) {
	pool := NewEncodingPool()

	// Should not panic
	pool.Put(nil)
}

func TestEncodingPoolWarmup(t *testing.T) {
	pool := NewEncodingPool()

	// Warmup
	pool.Warmup(10)

	// Get should work after warmup
	enc := pool.Get()
	if enc == nil {
		t.Fatal("Get() returned nil after Warmup()")
	}
	if !enc.IsEmpty() {
		t.Error("Get() should return empty encoding after Warmup()")
	}
}

func TestEncodingPoolConcurrent(t *testing.T) {
	pool := NewEncodingPool()
	pool.Warmup(10)

	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 100

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				enc := pool.Get()

				// Use encoding
				path := gg.NewPath()
				path.MoveTo(0, 0)
				path.LineTo(100, 100)
				enc.EncodePath(path)

				pool.Put(enc)
			}
		}()
	}

	wg.Wait()
}

func TestDefaultPool(t *testing.T) {
	enc := GetEncoding()
	if enc == nil {
		t.Fatal("GetEncoding() returned nil")
	}

	if !enc.IsEmpty() {
		t.Error("GetEncoding() should return empty encoding")
	}

	// Use it
	path := gg.NewPath()
	path.Rectangle(0, 0, 50, 50)
	enc.EncodePath(path)

	// Return
	PutEncoding(enc)

	// Should work again
	enc2 := GetEncoding()
	if !enc2.IsEmpty() {
		t.Error("GetEncoding() after PutEncoding() should return reset encoding")
	}
}

func TestPoolPreservesCapacity(t *testing.T) {
	pool := NewEncodingPool()

	// Get and populate with significant data
	enc := pool.Get()
	for i := 0; i < 100; i++ {
		path := gg.NewPath()
		path.Rectangle(float64(i*10), float64(i*10), 50, 50)
		enc.EncodePath(path)
	}

	initialCap := enc.Capacity()

	// Return and get again
	pool.Put(enc)
	enc2 := pool.Get()

	// Capacity should be preserved (this tests zero-allocation reuse)
	if enc2.Capacity() < initialCap {
		t.Logf("Warning: capacity not fully preserved (got %d, had %d)", enc2.Capacity(), initialCap)
		// This is not necessarily an error - the pool may return a different object
	}
}

// Benchmarks

func BenchmarkPoolGetPut(b *testing.B) {
	pool := NewEncodingPool()
	pool.Warmup(10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc := pool.Get()
		pool.Put(enc)
	}
}

func BenchmarkPoolGetPutWithWork(b *testing.B) {
	pool := NewEncodingPool()
	pool.Warmup(10)

	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc := pool.Get()
		enc.EncodePath(path)
		enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)
		pool.Put(enc)
	}
}

func BenchmarkNewEncodingVsPool(b *testing.B) {
	pool := NewEncodingPool()
	pool.Warmup(10)

	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)

	b.Run("NewEncoding", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			enc := NewEncoding()
			enc.EncodePath(path)
			enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)
			_ = enc.Hash()
		}
	})

	b.Run("Pool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			enc := pool.Get()
			enc.EncodePath(path)
			enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)
			_ = enc.Hash()
			pool.Put(enc)
		}
	})
}

func BenchmarkPoolConcurrent(b *testing.B) {
	pool := NewEncodingPool()
	pool.Warmup(100)

	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			enc := pool.Get()
			enc.EncodePath(path)
			enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)
			pool.Put(enc)
		}
	})
}
