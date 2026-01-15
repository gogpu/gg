// Package gpucore provides shared GPU abstractions for the gg rendering pipeline.
//
// This package defines the [GPUAdapter] interface, which abstracts over different
// GPU backend implementations, allowing the same rendering algorithms to work with:
//   - gogpu/wgpu (Pure Go WebGPU via HAL)
//   - gogpu/gogpu (High-level GPU framework with dual backends)
//
// # Architecture
//
// The gpucore package implements Option C from the GPU Backend Architecture:
// shared gpucore + thin adapters. The core rendering pipeline (flatten -> coarse -> fine)
// is implemented once in this package, while thin adapters translate between
// the [GPUAdapter] interface and specific backend APIs.
//
//	               +-----------------+
//	               |    gpucore     |
//	               | (HybridPipeline)|
//	               +--------+--------+
//	                        |
//	         +--------------+--------------+
//	         |                             |
//	+--------v--------+          +--------v--------+
//	|   wgpu adapter  |          |  gogpu adapter  |
//	|  (hal.Device)   |          | (gpu.Backend)   |
//	+--------+--------+          +--------+--------+
//	         |                             |
//	+--------v--------+          +--------v--------+
//	|   gogpu/wgpu    |          |   gogpu/gogpu   |
//	|   (Pure Go)     |          | (Rust or Go)    |
//	+-----------------+          +-----------------+
//
// # Pipeline Stages
//
// The [HybridPipeline] orchestrates three stages:
//
//  1. Flatten: Converts Bezier curves to monotonic line segments using
//     Wang's formula for adaptive subdivision.
//
//  2. Coarse: Bins segments into tiles for efficient parallel processing.
//     Each tile tracks which segments intersect it.
//
//  3. Fine: Calculates per-pixel coverage using analytical area computation.
//     Supports both NonZero and EvenOdd fill rules.
//
// # Resource Management
//
// GPU resources are managed via opaque IDs ([BufferID], [TextureID], etc.).
// The [GPUAdapter] interface provides creation and destruction methods for
// each resource type. Adapters are responsible for tracking the mapping
// between IDs and actual GPU resources.
//
// # CPU Fallback
//
// When GPU compute is unavailable or for debugging purposes, the pipeline
// can run entirely on CPU. Set [PipelineConfig.UseCPUFallback] to true
// to force CPU execution of all stages.
//
// # Usage Example
//
//	// Create adapter (implementation-specific)
//	adapter := wgpuadapter.New(device, queue)
//
//	// Create pipeline
//	config := &gpucore.PipelineConfig{
//	    Width:      1920,
//	    Height:     1080,
//	    MaxPaths:   10000,
//	    MaxSegments: 100000,
//	}
//	pipeline, err := gpucore.NewHybridPipeline(adapter, config)
//	if err != nil {
//	    return err
//	}
//	defer pipeline.Destroy()
//
//	// Render paths
//	result, err := pipeline.Execute(paths, transform, fillRule)
package gpucore
