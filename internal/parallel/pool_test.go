package parallel

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// WorkerPool Creation Tests
// =============================================================================

func TestWorkerPool_Create(t *testing.T) {
	pool := NewWorkerPool(4)
	defer pool.Close()

	if pool.Workers() != 4 {
		t.Errorf("Workers() = %d, want 4", pool.Workers())
	}

	if !pool.IsRunning() {
		t.Error("Pool should be running after creation")
	}
}

func TestWorkerPool_CreateZeroWorkers(t *testing.T) {
	pool := NewWorkerPool(0)
	defer pool.Close()

	expected := runtime.GOMAXPROCS(0)
	if pool.Workers() != expected {
		t.Errorf("Workers() = %d, want %d (GOMAXPROCS)", pool.Workers(), expected)
	}
}

func TestWorkerPool_CreateNegativeWorkers(t *testing.T) {
	pool := NewWorkerPool(-5)
	defer pool.Close()

	expected := runtime.GOMAXPROCS(0)
	if pool.Workers() != expected {
		t.Errorf("Workers() = %d, want %d (GOMAXPROCS)", pool.Workers(), expected)
	}
}

// =============================================================================
// ExecuteAll Tests
// =============================================================================

func TestWorkerPool_ExecuteAll(t *testing.T) {
	pool := NewWorkerPool(4)
	defer pool.Close()

	var counter atomic.Int64
	numTasks := 100

	work := make([]func(), numTasks)
	for i := range work {
		work[i] = func() {
			counter.Add(1)
		}
	}

	pool.ExecuteAll(work)

	if counter.Load() != int64(numTasks) {
		t.Errorf("counter = %d, want %d", counter.Load(), numTasks)
	}
}

func TestWorkerPool_ExecuteAll_Order(t *testing.T) {
	pool := NewWorkerPool(4)
	defer pool.Close()

	var mu sync.Mutex
	results := make([]int, 0, 10)

	work := make([]func(), 10)
	for i := range work {
		idx := i
		work[i] = func() {
			mu.Lock()
			results = append(results, idx)
			mu.Unlock()
		}
	}

	pool.ExecuteAll(work)

	// All items should be executed (order may vary due to parallelism)
	if len(results) != 10 {
		t.Errorf("results length = %d, want 10", len(results))
	}

	// Verify all indices are present
	seen := make(map[int]bool)
	for _, v := range results {
		seen[v] = true
	}
	for i := 0; i < 10; i++ {
		if !seen[i] {
			t.Errorf("missing index %d in results", i)
		}
	}
}

func TestWorkerPool_ExecuteAll_Empty(t *testing.T) {
	pool := NewWorkerPool(4)
	defer pool.Close()

	// Should not panic or block
	pool.ExecuteAll(nil)
	pool.ExecuteAll([]func(){})
}

func TestWorkerPool_ExecuteAll_Single(t *testing.T) {
	pool := NewWorkerPool(4)
	defer pool.Close()

	var executed atomic.Bool

	pool.ExecuteAll([]func(){
		func() { executed.Store(true) },
	})

	if !executed.Load() {
		t.Error("single task was not executed")
	}
}

// =============================================================================
// ExecuteAsync Tests
// =============================================================================

func TestWorkerPool_ExecuteAsync(t *testing.T) {
	pool := NewWorkerPool(4)

	var counter atomic.Int64
	numTasks := 50
	done := make(chan struct{})

	work := make([]func(), numTasks)
	for i := range work {
		work[i] = func() {
			if counter.Add(1) == int64(numTasks) {
				close(done)
			}
		}
	}

	pool.ExecuteAsync(work)

	// Wait for completion with timeout
	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Errorf("timeout waiting for async work, counter = %d", counter.Load())
	}

	pool.Close()
}

func TestWorkerPool_ExecuteAsync_Empty(t *testing.T) {
	pool := NewWorkerPool(4)
	defer pool.Close()

	// Should not panic or block
	pool.ExecuteAsync(nil)
	pool.ExecuteAsync([]func(){})
}

// =============================================================================
// Submit Tests
// =============================================================================

func TestWorkerPool_Submit(t *testing.T) {
	pool := NewWorkerPool(4)

	var counter atomic.Int64
	numTasks := 20
	done := make(chan struct{})

	for i := 0; i < numTasks; i++ {
		pool.Submit(func() {
			if counter.Add(1) == int64(numTasks) {
				close(done)
			}
		})
	}

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Errorf("timeout waiting for submitted work, counter = %d", counter.Load())
	}

	pool.Close()
}

func TestWorkerPool_Submit_Nil(t *testing.T) {
	pool := NewWorkerPool(4)
	defer pool.Close()

	// Should not panic
	pool.Submit(nil)
}

// =============================================================================
// Close Tests
// =============================================================================

func TestWorkerPool_Close(t *testing.T) {
	pool := NewWorkerPool(4)

	if !pool.IsRunning() {
		t.Error("Pool should be running before close")
	}

	pool.Close()

	if pool.IsRunning() {
		t.Error("Pool should not be running after close")
	}
}

func TestWorkerPool_CloseIdempotent(t *testing.T) {
	pool := NewWorkerPool(4)

	// Multiple closes should not panic
	pool.Close()
	pool.Close()
	pool.Close()

	if pool.IsRunning() {
		t.Error("Pool should not be running after close")
	}
}

func TestWorkerPool_CloseWithPendingWork(t *testing.T) {
	pool := NewWorkerPool(2)

	var counter atomic.Int64

	// Submit work that will be pending
	work := make([]func(), 100)
	for i := range work {
		work[i] = func() {
			counter.Add(1)
		}
	}

	pool.ExecuteAsync(work)
	pool.Close()

	// Some or all work should have been completed before close finished
	// (Close waits for queued work to drain)
	t.Logf("Completed %d tasks before pool closed", counter.Load())
}

func TestWorkerPool_OperationsAfterClose(t *testing.T) {
	pool := NewWorkerPool(4)
	pool.Close()

	var executed atomic.Bool

	// These should be no-ops, not panic
	pool.ExecuteAll([]func(){
		func() { executed.Store(true) },
	})
	pool.ExecuteAsync([]func(){
		func() { executed.Store(true) },
	})
	pool.Submit(func() { executed.Store(true) })

	// Give time for potential incorrect execution
	time.Sleep(50 * time.Millisecond)

	if executed.Load() {
		t.Error("Work was executed on closed pool")
	}
}

// =============================================================================
// Concurrency Tests
// =============================================================================

func TestWorkerPool_Concurrent(t *testing.T) {
	pool := NewWorkerPool(4)
	defer pool.Close()

	var counter atomic.Int64
	numGoroutines := 10
	numTasksPerGoroutine := 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()

			work := make([]func(), numTasksPerGoroutine)
			for i := range work {
				work[i] = func() {
					counter.Add(1)
				}
			}

			pool.ExecuteAll(work)
		}()
	}

	wg.Wait()

	expected := int64(numGoroutines * numTasksPerGoroutine)
	if counter.Load() != expected {
		t.Errorf("counter = %d, want %d", counter.Load(), expected)
	}
}

func TestWorkerPool_WorkStealing(t *testing.T) {
	pool := NewWorkerPool(4)
	defer pool.Close()

	// Create uneven work distribution - some tasks are much slower
	var fastCount, slowCount atomic.Int64

	work := make([]func(), 100)
	for i := range work {
		if i%10 == 0 {
			// Slow task
			work[i] = func() {
				time.Sleep(10 * time.Millisecond)
				slowCount.Add(1)
			}
		} else {
			// Fast task
			work[i] = func() {
				fastCount.Add(1)
			}
		}
	}

	start := time.Now()
	pool.ExecuteAll(work)
	elapsed := time.Since(start)

	if slowCount.Load() != 10 {
		t.Errorf("slowCount = %d, want 10", slowCount.Load())
	}
	if fastCount.Load() != 90 {
		t.Errorf("fastCount = %d, want 90", fastCount.Load())
	}

	// Work stealing should help complete faster than sequential
	// 10 slow tasks * 10ms = 100ms sequential minimum
	// With 4 workers and work stealing, should be closer to 30-40ms
	t.Logf("Elapsed time: %v (work stealing should help)", elapsed)
}

func TestWorkerPool_NoGoroutineLeak(t *testing.T) {
	// Get baseline goroutine count
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	// Create and use pool
	for i := 0; i < 5; i++ {
		pool := NewWorkerPool(4)

		work := make([]func(), 100)
		for j := range work {
			work[j] = func() {}
		}
		pool.ExecuteAll(work)

		pool.Close()
	}

	// Allow goroutines to clean up
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	final := runtime.NumGoroutine()

	// Allow for some variance (test framework goroutines, etc.)
	if final > baseline+2 {
		t.Errorf("goroutine count: baseline=%d, final=%d (leak detected)", baseline, final)
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestWorkerPool_ManySmallTasks(t *testing.T) {
	pool := NewWorkerPool(4)
	defer pool.Close()

	var counter atomic.Int64
	numTasks := 10000

	work := make([]func(), numTasks)
	for i := range work {
		work[i] = func() {
			counter.Add(1)
		}
	}

	pool.ExecuteAll(work)

	if counter.Load() != int64(numTasks) {
		t.Errorf("counter = %d, want %d", counter.Load(), numTasks)
	}
}

func TestWorkerPool_SingleWorker(t *testing.T) {
	pool := NewWorkerPool(1)
	defer pool.Close()

	var counter atomic.Int64

	work := make([]func(), 50)
	for i := range work {
		work[i] = func() {
			counter.Add(1)
		}
	}

	pool.ExecuteAll(work)

	if counter.Load() != 50 {
		t.Errorf("counter = %d, want 50", counter.Load())
	}
}

func TestWorkerPool_ManyWorkers(t *testing.T) {
	pool := NewWorkerPool(32)
	defer pool.Close()

	var counter atomic.Int64

	work := make([]func(), 100)
	for i := range work {
		work[i] = func() {
			counter.Add(1)
		}
	}

	pool.ExecuteAll(work)

	if counter.Load() != 100 {
		t.Errorf("counter = %d, want 100", counter.Load())
	}
}

func TestWorkerPool_QueuedWork(t *testing.T) {
	pool := NewWorkerPool(2)
	defer pool.Close()

	// Initially no queued work
	if pool.QueuedWork() != 0 {
		t.Errorf("initial QueuedWork() = %d, want 0", pool.QueuedWork())
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkWorkerPool_ExecuteAll_Small(b *testing.B) {
	pool := NewWorkerPool(runtime.GOMAXPROCS(0))
	defer pool.Close()

	work := make([]func(), 10)
	for i := range work {
		work[i] = func() {}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

func BenchmarkWorkerPool_ExecuteAll_Medium(b *testing.B) {
	pool := NewWorkerPool(runtime.GOMAXPROCS(0))
	defer pool.Close()

	work := make([]func(), 100)
	for i := range work {
		work[i] = func() {}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

func BenchmarkWorkerPool_ExecuteAll_Large(b *testing.B) {
	pool := NewWorkerPool(runtime.GOMAXPROCS(0))
	defer pool.Close()

	work := make([]func(), 1000)
	for i := range work {
		work[i] = func() {}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

func BenchmarkWorkerPool_Submit(b *testing.B) {
	pool := NewWorkerPool(runtime.GOMAXPROCS(0))
	defer pool.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		pool.Submit(func() {
			close(done)
		})
		<-done
	}
}

func BenchmarkWorkerPool_vs_Goroutines(b *testing.B) {
	numTasks := 100

	b.Run("WorkerPool", func(b *testing.B) {
		pool := NewWorkerPool(runtime.GOMAXPROCS(0))
		defer pool.Close()

		work := make([]func(), numTasks)
		for i := range work {
			work[i] = func() {}
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			pool.ExecuteAll(work)
		}
	})

	b.Run("RawGoroutines", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			wg.Add(numTasks)
			for j := 0; j < numTasks; j++ {
				go func() {
					defer wg.Done()
				}()
			}
			wg.Wait()
		}
	})
}

func BenchmarkWorkerPool_WithWork(b *testing.B) {
	// Benchmark with actual work to simulate realistic usage
	pool := NewWorkerPool(runtime.GOMAXPROCS(0))
	defer pool.Close()

	work := make([]func(), 100)
	for i := range work {
		work[i] = func() {
			// Simulate some work (small computation)
			sum := 0
			for j := 0; j < 1000; j++ {
				sum += j
			}
			_ = sum
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

func BenchmarkWorkerPool_Parallel(b *testing.B) {
	pool := NewWorkerPool(runtime.GOMAXPROCS(0))
	defer pool.Close()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		work := make([]func(), 10)
		for i := range work {
			work[i] = func() {}
		}

		for pb.Next() {
			pool.ExecuteAll(work)
		}
	})
}
