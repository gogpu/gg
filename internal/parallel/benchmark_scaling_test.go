package parallel

import (
	"image/color"
	"runtime"
	"testing"
)

// =============================================================================
// Core Scaling Benchmarks
// =============================================================================
//
// These benchmarks test how parallel rendering scales with different core counts.
// Use runtime.GOMAXPROCS to control the number of available cores.
//
// Run with: go test -bench=BenchmarkScaling -benchmem -benchtime=1s ./internal/parallel/...
//
// =============================================================================

// setMaxProcs sets GOMAXPROCS and returns a cleanup function to restore it.
func setMaxProcs(n int) func() {
	old := runtime.GOMAXPROCS(n)
	return func() {
		runtime.GOMAXPROCS(old)
	}
}

// =============================================================================
// Clear Operation Scaling
// =============================================================================

// BenchmarkScaling_Clear_1Core benchmarks Clear with 1 core.
func BenchmarkScaling_Clear_1Core(b *testing.B) {
	cleanup := setMaxProcs(1)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 1)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// BenchmarkScaling_Clear_2Cores benchmarks Clear with 2 cores.
func BenchmarkScaling_Clear_2Cores(b *testing.B) {
	cleanup := setMaxProcs(2)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 2)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// BenchmarkScaling_Clear_4Cores benchmarks Clear with 4 cores.
func BenchmarkScaling_Clear_4Cores(b *testing.B) {
	cleanup := setMaxProcs(4)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 4)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// BenchmarkScaling_Clear_8Cores benchmarks Clear with 8 cores.
func BenchmarkScaling_Clear_8Cores(b *testing.B) {
	cleanup := setMaxProcs(8)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 8)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// BenchmarkScaling_Clear_MaxCores benchmarks Clear with all available cores.
func BenchmarkScaling_Clear_MaxCores(b *testing.B) {
	numCPU := runtime.NumCPU()
	cleanup := setMaxProcs(numCPU)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, numCPU)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// =============================================================================
// FillRect Operation Scaling
// =============================================================================

// BenchmarkScaling_FillRect_1Core benchmarks FillRect with 1 core.
func BenchmarkScaling_FillRect_1Core(b *testing.B) {
	cleanup := setMaxProcs(1)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 1)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.FillRect(0, 0, 960, 540, red) // Quarter screen
	}
}

// BenchmarkScaling_FillRect_2Cores benchmarks FillRect with 2 cores.
func BenchmarkScaling_FillRect_2Cores(b *testing.B) {
	cleanup := setMaxProcs(2)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 2)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.FillRect(0, 0, 960, 540, red)
	}
}

// BenchmarkScaling_FillRect_4Cores benchmarks FillRect with 4 cores.
func BenchmarkScaling_FillRect_4Cores(b *testing.B) {
	cleanup := setMaxProcs(4)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 4)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.FillRect(0, 0, 960, 540, red)
	}
}

// BenchmarkScaling_FillRect_8Cores benchmarks FillRect with 8 cores.
func BenchmarkScaling_FillRect_8Cores(b *testing.B) {
	cleanup := setMaxProcs(8)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 8)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.FillRect(0, 0, 960, 540, red)
	}
}

// BenchmarkScaling_FillRect_MaxCores benchmarks FillRect with all available cores.
func BenchmarkScaling_FillRect_MaxCores(b *testing.B) {
	numCPU := runtime.NumCPU()
	cleanup := setMaxProcs(numCPU)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, numCPU)
	defer pr.Close()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.FillRect(0, 0, 960, 540, red)
	}
}

// =============================================================================
// Composite Operation Scaling
// =============================================================================

// BenchmarkScaling_Composite_1Core benchmarks Composite with 1 core.
func BenchmarkScaling_Composite_1Core(b *testing.B) {
	cleanup := setMaxProcs(1)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 1)
	defer pr.Close()

	pr.Clear(color.White)

	stride := 1920 * 4
	dst := make([]byte, 1080*stride)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Composite(dst, stride)
	}
}

// BenchmarkScaling_Composite_2Cores benchmarks Composite with 2 cores.
func BenchmarkScaling_Composite_2Cores(b *testing.B) {
	cleanup := setMaxProcs(2)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 2)
	defer pr.Close()

	pr.Clear(color.White)

	stride := 1920 * 4
	dst := make([]byte, 1080*stride)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Composite(dst, stride)
	}
}

// BenchmarkScaling_Composite_4Cores benchmarks Composite with 4 cores.
func BenchmarkScaling_Composite_4Cores(b *testing.B) {
	cleanup := setMaxProcs(4)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 4)
	defer pr.Close()

	pr.Clear(color.White)

	stride := 1920 * 4
	dst := make([]byte, 1080*stride)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Composite(dst, stride)
	}
}

// BenchmarkScaling_Composite_8Cores benchmarks Composite with 8 cores.
func BenchmarkScaling_Composite_8Cores(b *testing.B) {
	cleanup := setMaxProcs(8)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, 8)
	defer pr.Close()

	pr.Clear(color.White)

	stride := 1920 * 4
	dst := make([]byte, 1080*stride)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Composite(dst, stride)
	}
}

// BenchmarkScaling_Composite_MaxCores benchmarks Composite with all available cores.
func BenchmarkScaling_Composite_MaxCores(b *testing.B) {
	numCPU := runtime.NumCPU()
	cleanup := setMaxProcs(numCPU)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(1920, 1080, numCPU)
	defer pr.Close()

	pr.Clear(color.White)

	stride := 1920 * 4
	dst := make([]byte, 1080*stride)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Composite(dst, stride)
	}
}

// =============================================================================
// 4K Resolution Scaling
// =============================================================================

// BenchmarkScaling_Clear4K_1Core benchmarks 4K Clear with 1 core.
func BenchmarkScaling_Clear4K_1Core(b *testing.B) {
	cleanup := setMaxProcs(1)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(3840, 2160, 1)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// BenchmarkScaling_Clear4K_4Cores benchmarks 4K Clear with 4 cores.
func BenchmarkScaling_Clear4K_4Cores(b *testing.B) {
	cleanup := setMaxProcs(4)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(3840, 2160, 4)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// BenchmarkScaling_Clear4K_8Cores benchmarks 4K Clear with 8 cores.
func BenchmarkScaling_Clear4K_8Cores(b *testing.B) {
	cleanup := setMaxProcs(8)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(3840, 2160, 8)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// BenchmarkScaling_Clear4K_MaxCores benchmarks 4K Clear with all available cores.
func BenchmarkScaling_Clear4K_MaxCores(b *testing.B) {
	numCPU := runtime.NumCPU()
	cleanup := setMaxProcs(numCPU)
	defer cleanup()

	pr := NewParallelRasterizerWithWorkers(3840, 2160, numCPU)
	defer pr.Close()

	white := color.White

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pr.Clear(white)
	}
}

// =============================================================================
// WorkerPool Scaling
// =============================================================================

// BenchmarkScaling_WorkerPool_1Core benchmarks WorkerPool with 1 worker.
func BenchmarkScaling_WorkerPool_1Core(b *testing.B) {
	cleanup := setMaxProcs(1)
	defer cleanup()

	pool := NewWorkerPool(1)
	defer pool.Close()

	// Create work items simulating tile processing
	work := make([]func(), 510) // HD tile count
	for i := range work {
		work[i] = func() {
			// Simulate minimal work
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

// BenchmarkScaling_WorkerPool_4Cores benchmarks WorkerPool with 4 workers.
func BenchmarkScaling_WorkerPool_4Cores(b *testing.B) {
	cleanup := setMaxProcs(4)
	defer cleanup()

	pool := NewWorkerPool(4)
	defer pool.Close()

	work := make([]func(), 510)
	for i := range work {
		work[i] = func() {}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

// BenchmarkScaling_WorkerPool_8Cores benchmarks WorkerPool with 8 workers.
func BenchmarkScaling_WorkerPool_8Cores(b *testing.B) {
	cleanup := setMaxProcs(8)
	defer cleanup()

	pool := NewWorkerPool(8)
	defer pool.Close()

	work := make([]func(), 510)
	for i := range work {
		work[i] = func() {}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pool.ExecuteAll(work)
	}
}

// =============================================================================
// Efficiency Metrics
// =============================================================================

// BenchmarkScalingEfficiency_Clear measures parallel efficiency for Clear.
// Parallel efficiency = (Sequential time / Parallel time) / num_cores
// Perfect scaling would give efficiency = 1.0 (or 100%)
func BenchmarkScalingEfficiency_Clear(b *testing.B) {
	sizes := []struct {
		name          string
		width, height int
	}{
		{"HD", 1920, 1080},
		{"4K", 3840, 2160},
	}

	coreCounts := []int{1, 2, 4, 8}
	white := color.White

	for _, size := range sizes {
		for _, cores := range coreCounts {
			name := size.name + "_" + string('0'+rune(cores)) + "cores"
			b.Run(name, func(b *testing.B) {
				cleanup := setMaxProcs(cores)
				defer cleanup()

				pr := NewParallelRasterizerWithWorkers(size.width, size.height, cores)
				defer pr.Close()

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					pr.Clear(white)
				}
			})
		}
	}
}

// BenchmarkScalingEfficiency_Composite measures parallel efficiency for Composite.
func BenchmarkScalingEfficiency_Composite(b *testing.B) {
	sizes := []struct {
		name          string
		width, height int
	}{
		{"HD", 1920, 1080},
		{"4K", 3840, 2160},
	}

	coreCounts := []int{1, 2, 4, 8}

	for _, size := range sizes {
		stride := size.width * 4
		dst := make([]byte, size.height*stride)

		for _, cores := range coreCounts {
			name := size.name + "_" + string('0'+rune(cores)) + "cores"
			b.Run(name, func(b *testing.B) {
				cleanup := setMaxProcs(cores)
				defer cleanup()

				pr := NewParallelRasterizerWithWorkers(size.width, size.height, cores)
				defer pr.Close()

				pr.Clear(color.White)

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					pr.Composite(dst, stride)
				}
			})
		}
	}
}
