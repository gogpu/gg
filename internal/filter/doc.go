// Package filter provides image filter implementations for the scene graph.
//
// This package contains enterprise-grade filter effects:
//   - Gaussian blur (separable, O(n) per radius)
//   - Drop shadow (blur + offset + colorize)
//   - Color matrix transformations
//
// All filters are designed for:
//   - Zero-allocation hot paths where possible
//   - Cache-friendly memory access patterns
//   - SIMD-compatible data layouts
//
// Performance targets (1080p):
//   - Blur (r=5): <5ms
//   - Blur (r=20): <15ms
//   - Drop Shadow: <10ms
//   - Color Matrix: <2ms
package filter
