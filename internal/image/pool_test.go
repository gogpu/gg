package image

import (
	"sync"
	"testing"
)

func TestNewPool(t *testing.T) {
	tests := []struct {
		name         string
		maxPerBucket int
		wantMaxSize  int
	}{
		{
			name:         "zero means unlimited",
			maxPerBucket: 0,
			wantMaxSize:  0,
		},
		{
			name:         "positive limit",
			maxPerBucket: 5,
			wantMaxSize:  5,
		},
		{
			name:         "negative means unlimited (edge case)",
			maxPerBucket: -1,
			wantMaxSize:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewPool(tt.maxPerBucket)
			if pool == nil {
				t.Fatal("NewPool returned nil")
			}
			if pool.maxSize != tt.wantMaxSize {
				t.Errorf("maxSize = %d, want %d", pool.maxSize, tt.wantMaxSize)
			}
			if pool.buckets == nil {
				t.Error("buckets map is nil")
			}
		})
	}
}

func TestPool_GetPut_Basic(t *testing.T) {
	pool := NewPool(4)

	// Get a buffer from empty pool (should create new)
	buf1 := pool.Get(100, 100, FormatRGBA8)
	if buf1 == nil {
		t.Fatal("Get returned nil")
	}
	if buf1.Width() != 100 || buf1.Height() != 100 {
		t.Errorf("got dimensions %dx%d, want 100x100", buf1.Width(), buf1.Height())
	}
	if buf1.Format() != FormatRGBA8 {
		t.Errorf("got format %v, want FormatRGBA8", buf1.Format())
	}

	// Modify buffer to verify it gets cleared on return
	_ = buf1.SetRGBA(0, 0, 255, 128, 64, 200)

	// Return buffer to pool
	pool.Put(buf1)

	// Get it back - should be same buffer but cleared
	buf2 := pool.Get(100, 100, FormatRGBA8)
	if buf2 == nil {
		t.Fatal("Get returned nil after Put")
	}

	// Verify it's cleared
	r, g, b, a := buf2.GetRGBA(0, 0)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("buffer not cleared: got RGBA(%d,%d,%d,%d), want (0,0,0,0)", r, g, b, a)
	}
}

func TestPool_GetPut_DifferentSizes(t *testing.T) {
	pool := NewPool(2)

	// Create buffers of different sizes
	buf1 := pool.Get(100, 100, FormatRGBA8)
	buf2 := pool.Get(200, 200, FormatRGBA8)
	buf3 := pool.Get(100, 100, FormatBGRA8) // Same size, different format

	if buf1 == nil || buf2 == nil || buf3 == nil {
		t.Fatal("Get returned nil")
	}

	// Return all to pool
	pool.Put(buf1)
	pool.Put(buf2)
	pool.Put(buf3)

	// Verify buckets are separate
	pool.mu.Lock()
	key1 := poolKey{100, 100, FormatRGBA8}
	key2 := poolKey{200, 200, FormatRGBA8}
	key3 := poolKey{100, 100, FormatBGRA8}

	if len(pool.buckets[key1]) != 1 {
		t.Errorf("bucket[100x100 RGBA8] has %d buffers, want 1", len(pool.buckets[key1]))
	}
	if len(pool.buckets[key2]) != 1 {
		t.Errorf("bucket[200x200 RGBA8] has %d buffers, want 1", len(pool.buckets[key2]))
	}
	if len(pool.buckets[key3]) != 1 {
		t.Errorf("bucket[100x100 BGRA8] has %d buffers, want 1", len(pool.buckets[key3]))
	}
	pool.mu.Unlock()
}

func TestPool_MaxSize(t *testing.T) {
	maxSize := 3
	pool := NewPool(maxSize)

	// Create and return more buffers than maxSize
	buffers := make([]*ImageBuf, 5)
	for i := range buffers {
		buffers[i] = pool.Get(50, 50, FormatRGBA8)
		if buffers[i] == nil {
			t.Fatalf("Get[%d] returned nil", i)
		}
	}

	// Return all buffers
	for i := range buffers {
		pool.Put(buffers[i])
	}

	// Check pool size is limited
	pool.mu.Lock()
	key := poolKey{50, 50, FormatRGBA8}
	bucketSize := len(pool.buckets[key])
	pool.mu.Unlock()

	if bucketSize != maxSize {
		t.Errorf("bucket size = %d, want %d (maxSize)", bucketSize, maxSize)
	}
}

func TestPool_Put_Nil(t *testing.T) {
	pool := NewPool(4)

	// Should not panic
	pool.Put(nil)

	// Verify pool is still empty
	pool.mu.Lock()
	totalBuffers := 0
	for _, bucket := range pool.buckets {
		totalBuffers += len(bucket)
	}
	pool.mu.Unlock()

	if totalBuffers != 0 {
		t.Errorf("pool has %d buffers after Put(nil), want 0", totalBuffers)
	}
}

func TestPool_Concurrent(t *testing.T) {
	pool := NewPool(10)
	numGoroutines := 20
	numOpsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOpsPerGoroutine; j++ {
				// Alternate between different sizes and formats
				width := 50 + (id%3)*50
				height := 50 + (id%3)*50
				format := FormatRGBA8
				if id%2 == 1 {
					format = FormatBGRA8
				}

				buf := pool.Get(width, height, format)
				if buf == nil {
					t.Errorf("goroutine %d: Get returned nil", id)
					continue
				}

				// Do some work with buffer
				_ = buf.SetRGBA(0, 0, byte(id), byte(j), 0, 255)
				r, _, _, _ := buf.GetRGBA(0, 0)
				if r != byte(id) {
					t.Errorf("goroutine %d: expected r=%d, got %d", id, byte(id), r)
				}

				// Return to pool
				pool.Put(buf)
			}
		}(i)
	}

	wg.Wait()

	// Verify pool state is consistent
	pool.mu.Lock()
	for key, bucket := range pool.buckets {
		if len(bucket) > pool.maxSize {
			t.Errorf("bucket %+v has %d buffers, exceeds maxSize %d", key, len(bucket), pool.maxSize)
		}
		// Verify all buffers are cleared
		for i, buf := range bucket {
			r, g, b, a := buf.GetRGBA(0, 0)
			if r != 0 || g != 0 || b != 0 || a != 0 {
				t.Errorf("bucket %+v buffer %d not cleared: RGBA(%d,%d,%d,%d)", key, i, r, g, b, a)
			}
		}
	}
	pool.mu.Unlock()
}

func TestPool_GetInvalidDimensions(t *testing.T) {
	pool := NewPool(4)

	// Test invalid dimensions
	tests := []struct {
		name   string
		width  int
		height int
		format Format
	}{
		{"zero width", 0, 100, FormatRGBA8},
		{"zero height", 100, 0, FormatRGBA8},
		{"negative width", -10, 100, FormatRGBA8},
		{"negative height", 100, -10, FormatRGBA8},
		{"invalid format", 100, 100, Format(99)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := pool.Get(tt.width, tt.height, tt.format)
			if buf != nil {
				t.Errorf("Get with invalid params returned non-nil buffer")
			}
		})
	}
}

func TestDefaultPool(t *testing.T) {
	// Test default pool convenience functions
	buf1 := GetFromDefault(80, 80, FormatRGBA8)
	if buf1 == nil {
		t.Fatal("GetFromDefault returned nil")
	}

	// Modify and return
	_ = buf1.SetRGBA(10, 10, 123, 45, 67, 89)
	PutToDefault(buf1)

	// Get again - should be cleared
	buf2 := GetFromDefault(80, 80, FormatRGBA8)
	if buf2 == nil {
		t.Fatal("GetFromDefault returned nil after Put")
	}

	r, g, b, a := buf2.GetRGBA(10, 10)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("buffer from default pool not cleared: RGBA(%d,%d,%d,%d)", r, g, b, a)
	}
}

func TestPool_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	pool := NewPool(20)
	numGoroutines := 50
	numOpsPerGoroutine := 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOpsPerGoroutine; j++ {
				// Random-ish size
				size := 32 + (id*j)%128
				buf := pool.Get(size, size, FormatRGBA8)
				if buf != nil {
					pool.Put(buf)
				}
			}
		}(i)
	}

	wg.Wait()

	// Final consistency check
	pool.mu.Lock()
	totalBuffers := 0
	for _, bucket := range pool.buckets {
		totalBuffers += len(bucket)
		if len(bucket) > pool.maxSize {
			t.Errorf("bucket exceeds maxSize: %d > %d", len(bucket), pool.maxSize)
		}
	}
	pool.mu.Unlock()

	t.Logf("Stress test complete: %d total buffers pooled across %d buckets",
		totalBuffers, len(pool.buckets))
}

// Benchmark pool vs direct allocation
func BenchmarkPool_GetPut(b *testing.B) {
	pool := NewPool(8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get(256, 256, FormatRGBA8)
		pool.Put(buf)
	}
}

func BenchmarkDirect_NewImageBuf(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, _ := NewImageBuf(256, 256, FormatRGBA8)
		_ = buf
	}
}

func BenchmarkPool_Concurrent(b *testing.B) {
	pool := NewPool(16)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := pool.Get(128, 128, FormatRGBA8)
			pool.Put(buf)
		}
	})
}
