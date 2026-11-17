//go:build !nogpu

// Package gpu provides the Pure Go GPU accelerator for gg.
//
// This is an internal package. GPU rendering is opt-in for library users
// through the public package github.com/gogpu/gg/gpu, which registers the
// SDF accelerator and the coverage filler on import.
//
// The accelerator builds on the gogpu/wgpu Pure Go WebGPU implementation
// (zero CGO): Vulkan, Metal, DX12 and OpenGL ES backends, or the software
// backend where no GPU is available. wgpu registers the platform hardware
// backends by default; gg's production code imports no HAL backend.
//
// # Architecture Overview
//
// gg's CPU rasterizer is the core; the GPU is an optional accelerator. When a
// device is available the accelerator takes over per-tier:
//
//	Context.Fill/Stroke -> SDF shapes, stencil-then-cover, MSDF/glyph-mask text,
//	                       Vello-style compute (scene-level) -> present
//
// Key components:
//
//   - GPUShared: the process-wide device owner (wgpu device and queue, adapter
//     capabilities, render strategy) and the factory for render contexts
//   - GPURenderContext: one frame's encoder, render session and GPU state
//   - SDF render pipeline, convex polygon renderer and stencil-then-cover
//     renderer: render-pass tiers for shapes
//   - MSDF text and glyph-mask pipelines: GPU text tiers
//   - VelloAccelerator and the tilecompute package: the 9-stage Vello compute
//     pipeline for scene-level rendering (GPU dispatch plus a CPU reference)
//   - SparseStrips and TileCompute coverage fillers: CPU tile rasterizers ported
//     from Vello alongside the GPU pipeline
//
// The render strategy is chosen once from the adapter's capabilities
// (full MSAA, no-MSAA, or CPU raster with GPU textures only) and never changes.
//
// # Blend Modes
//
// All 29 blend modes are supported by the GPU compositing path: the Porter-Duff
// modes, the separable modes and the HSL modes.
//
// # Error Handling
//
// Common errors returned by this package:
//
//   - ErrNotInitialized: a GPU operation ran before the device was ready
//   - ErrNoGPU: no compatible GPU adapter was found
//   - ErrDeviceLost: the GPU device was lost (requires re-initialization)
//   - ErrNotImplemented: the operation has no GPU path and the caller falls back
//     to the CPU core
//   - ErrInvalidDimensions: a zero or oversized extent
//   - ErrNilTarget, ErrNilScene: a required argument is nil
//
// A GPU tier that cannot handle a draw returns an error the Context treats as
// a fallback signal; the CPU core always remains correct on its own.
//
// # Thread Safety
//
// GPUShared is safe for concurrent use; a GPURenderContext is owned by the
// goroutine that records its frame.
//
// # Requirements
//
//   - gogpu/wgpu (github.com/gogpu/wgpu)
//   - a platform where the Pure Go backends build (windows, linux, darwin,
//     android); elsewhere the accelerator reports no adapter and gg stays on
//     the CPU core
//
// # Related Packages
//
//   - github.com/gogpu/gg: core 2D graphics library
//   - github.com/gogpu/gg/scene: scene graph and retained-mode API
//   - github.com/gogpu/wgpu: Pure Go WebGPU implementation
//
// # References
//
//   - W3C WebGPU Specification: https://www.w3.org/TR/webgpu/
//   - gogpu organization: https://github.com/gogpu
package gpu
