// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Package velloport is a direct 1:1 port of Vello's CPU rasterization pipeline.
// Variable names and logic match the Rust originals for easy cross-reference.
// This is intentionally NOT idiomatic Go — it prioritizes pixel-perfect matching.
//
// Source: linebender/vello vello_shaders/src/cpu/
package velloport

import "math"

// Constants from util.rs
const (
	// oneMinusULP is the largest f32 strictly less than 1.
	// Ensures floor(a * i + b) == 0 for i == 0.
	oneMinusULP float32 = 0.99999994

	// robustEpsilon is applied when floor(a*(n-1)+b) doesn't match expected count_x.
	robustEpsilon float32 = 2e-7
)

// vec2 matches Vello's cpu::util::Vec2.
type vec2 struct {
	x, y float32
}

func newVec2(x, y float32) vec2       { return vec2{x, y} }
func vec2FromArray(a [2]float32) vec2 { return vec2{a[0], a[1]} }
func (v vec2) toArray() [2]float32    { return [2]float32{v.x, v.y} }
func (v vec2) add(o vec2) vec2        { return vec2{v.x + o.x, v.y + o.y} }
func (v vec2) sub(o vec2) vec2        { return vec2{v.x - o.x, v.y - o.y} }
func (v vec2) mul(s float32) vec2     { return vec2{v.x * s, v.y * s} }

// span matches Vello's cpu::util::span.
// span(a, b) = max(ceil(max(a,b)) - floor(min(a,b)), 1)
func span(a, b float32) uint32 {
	mx := a
	mn := a
	if b > mx {
		mx = b
	}
	if b < mn {
		mn = b
	}
	result := float32(math.Ceil(float64(mx))) - float32(math.Floor(float64(mn)))
	if result < 1.0 {
		result = 1.0
	}
	return uint32(result)
}

func floor32(x float32) float32 { return float32(math.Floor(float64(x))) }
func ceil32(x float32) float32  { return float32(math.Ceil(float64(x))) }
func round32(x float32) float32 { return float32(math.Round(float64(x))) }
func abs32(x float32) float32   { return float32(math.Abs(float64(x))) }

// min32 matches Rust's f32::min — returns non-NaN value if one is NaN.
func min32(a, b float32) float32 {
	if a != a { //nolint:gocritic // NaN check (a is NaN)
		return b
	}
	if b != b { //nolint:gocritic // NaN check (b is NaN)
		return a
	}
	if a < b {
		return a
	}
	return b
}

// max32 matches Rust's f32::max — returns non-NaN value if one is NaN.
func max32(a, b float32) float32 {
	if a != a { //nolint:gocritic // NaN check (a is NaN)
		return b
	}
	if b != b { //nolint:gocritic // NaN check (b is NaN)
		return a
	}
	if a > b {
		return a
	}
	return b
}

func clamp32(x, lo, hi float32) float32 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func copysign32(x, y float32) float32 {
	return float32(math.Copysign(float64(x), float64(y)))
}

// signum32 matches Rust's f32::signum exactly:
// positive → 1.0, negative → -1.0, +0.0 → 1.0, -0.0 → -1.0
func signum32(x float32) float32 {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	// Match Rust: 0.0 → 1.0, -0.0 → -1.0
	if math.Float32bits(x)&(1<<31) != 0 {
		return -1
	}
	return 1
}

func mini32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func maxi32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func minu32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func maxu32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}
