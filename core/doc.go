// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Package core provides CPU-based rendering primitives for gg.
//
// This package contains the core algorithms for scanline conversion,
// edge processing, and anti-aliased filling. It is independent of any
// GPU backend and can be used for pure software rendering.
//
// # Architecture
//
// The core package follows the Skia/tiny-skia design where CPU rendering
// code is separated from GPU code. This allows:
//
//   - Clear separation of concerns (CPU vs GPU)
//   - Reuse of CPU algorithms in different contexts
//   - Testing of rendering logic without GPU dependencies
//
// # Key Components
//
//   - Fixed-point math (FDot6, FDot16) for precise curve rasterization
//   - Edge types (Line, Quadratic, Cubic) with forward differencing
//   - EdgeBuilder for converting paths to edges
//   - AET (Active Edge Table) for scanline processing
//   - Filler for analytic anti-aliased rendering
//   - AlphaRuns for RLE-compressed coverage storage
//
// # Usage
//
// The typical flow is:
//
//  1. Convert path to edges using EdgeBuilder
//  2. Process edges through AET (Active Edge Table)
//  3. Accumulate coverage using AlphaRuns
//  4. Call Filler to render with anti-aliasing
//
// # References
//
//   - Skia: https://skia.googlesource.com/skia/+/main/src/core/
//   - tiny-skia: https://github.com/AhornGraphics/tiny-skia
package core
