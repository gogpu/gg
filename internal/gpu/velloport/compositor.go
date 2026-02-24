// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Compositing helpers for multi-path rendering.
// Implements premultiplied source-over blending matching Vello's fine.rs.

package velloport

// blendSourceOver composites src over dst using premultiplied alpha source-over.
// srcColor is the path color in straight alpha RGBA.
// alpha is the path coverage value (0.0-1.0) from the rasterizer.
// dst is the current pixel color in straight alpha RGBA.
// Returns the blended color in straight alpha RGBA.
//
// The formula matches Vello's fine.rs compositing:
//
//	premultiplied src = srcColor * (srcColor.A/255) * alpha
//	result = src_premul + dst_premul * (1 - src_a)
//
// For fully opaque source colors (A=255) this simplifies to standard alpha blending:
//
//	result.R = srcColor.R * alpha + dst.R * (1 - alpha)
func blendSourceOver(srcColor [4]uint8, alpha float32, dst [4]uint8) [4]uint8 {
	if alpha <= 0 {
		return dst
	}
	if alpha > 1.0 {
		alpha = 1.0
	}

	// Source color opacity factor: coverage * source alpha
	srcA := alpha * float32(srcColor[3]) / 255.0

	// Convert source to premultiplied with coverage applied
	srcR := float32(srcColor[0]) / 255.0 * srcA
	srcG := float32(srcColor[1]) / 255.0 * srcA
	srcB := float32(srcColor[2]) / 255.0 * srcA

	// Convert destination to [0,1] range
	dstR := float32(dst[0]) / 255.0
	dstG := float32(dst[1]) / 255.0
	dstB := float32(dst[2]) / 255.0
	dstA := float32(dst[3]) / 255.0

	// Source-over: result = src + dst * (1 - src_a)
	inv := 1.0 - srcA
	outR := srcR + dstR*inv
	outG := srcG + dstG*inv
	outB := srcB + dstB*inv
	outA := srcA + dstA*inv

	// Convert back to uint8 with rounding (+ 0.5 before truncation)
	return [4]uint8{
		uint8(outR*255.0 + 0.5),
		uint8(outG*255.0 + 0.5),
		uint8(outB*255.0 + 0.5),
		uint8(outA*255.0 + 0.5),
	}
}
