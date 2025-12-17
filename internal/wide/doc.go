// Package wide provides SIMD-friendly wide types for batch pixel processing.
//
// This package implements wide types (U16x16, F32x8) that are designed to enable
// Go compiler auto-vectorization. By using fixed-size arrays and simple loops,
// these types allow the compiler to generate SIMD instructions on supported
// architectures (SSE, AVX, NEON).
//
// # Wide Types
//
// U16x16: 16 uint16 values for integer operations (alpha blending, color channels).
// F32x8: 8 float32 values for floating-point operations (gradients, filters).
//
// # BatchState
//
// BatchState provides Structure-of-Arrays (SoA) layout for processing 16 RGBA pixels
// in parallel. This layout is SIMD-friendly and enables efficient batch operations.
//
// # Design Philosophy
//
//   - Use simple loops over fixed-size arrays for auto-vectorization
//   - Avoid unsafe and assembly - rely on compiler optimization
//   - Keep functions small and inlineable
//   - Provide benchmarks to verify SIMD performance gains
//
// # Usage Example
//
//	// Batch blend 16 pixels
//	var batch wide.BatchState
//	batch.LoadSrc(srcPixels)
//	batch.LoadDst(dstPixels)
//
//	// Perform blending operations on batch.SR, batch.SG, etc.
//	// ...
//
//	batch.StoreDst(dstPixels)
package wide
