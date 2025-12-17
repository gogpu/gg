package blend

import "github.com/gogpu/gg/internal/wide"

// BlendBatch blends src over dst for n pixels using batch operations.
// Automatically uses batch (16px) or scalar based on count.
//
// This is the main entry point for optimized blending operations.
// It processes pixels in batches of 16 where possible, falling back
// to scalar operations for remaining pixels.
//
// Parameters:
//   - dst: destination buffer (RGBA, premultiplied alpha)
//   - src: source buffer (RGBA, premultiplied alpha)
//   - n: number of pixels to blend
//   - mode: blend mode to use
func BlendBatch(dst, src []byte, n int, mode BlendMode) {
	if n <= 0 {
		return
	}

	batchFunc := GetBatchBlendFunc(mode)
	scalarFunc := GetBlendFunc(mode)

	// Process in batches of 16 pixels
	batchCount := n / 16
	remainder := n % 16

	var batch wide.BatchState
	offset := 0

	// Process full batches
	for i := 0; i < batchCount; i++ {
		batch.LoadSrc(src[offset:])
		batch.LoadDst(dst[offset:])
		batchFunc(&batch)
		batch.StoreDst(dst[offset:])
		offset += 64 // 16 pixels * 4 bytes
	}

	// Process remaining pixels with scalar operations
	for i := 0; i < remainder; i++ {
		sr := src[offset+0]
		sg := src[offset+1]
		sb := src[offset+2]
		sa := src[offset+3]
		dr := dst[offset+0]
		dg := dst[offset+1]
		db := dst[offset+2]
		da := dst[offset+3]

		r, g, b, a := scalarFunc(sr, sg, sb, sa, dr, dg, db, da)

		dst[offset+0] = r
		dst[offset+1] = g
		dst[offset+2] = b
		dst[offset+3] = a

		offset += 4
	}
}

// BlendSourceOverBatch is an optimized version of BlendBatch specifically for SourceOver.
// This is the most common blend mode and deserves special optimization.
//
// SourceOver formula: Result = S + D * (1 - Sa)
func BlendSourceOverBatch(dst, src []byte, n int) {
	if n <= 0 {
		return
	}

	// Process in batches of 16 pixels
	batchCount := n / 16
	remainder := n % 16

	var batch wide.BatchState
	offset := 0

	// Process full batches
	for i := 0; i < batchCount; i++ {
		batch.LoadSrc(src[offset:])
		batch.LoadDst(dst[offset:])
		SourceOverBatch(&batch)
		batch.StoreDst(dst[offset:])
		offset += 64 // 16 pixels * 4 bytes
	}

	// Process remaining pixels with scalar operations
	for i := 0; i < remainder; i++ {
		sr := src[offset+0]
		sg := src[offset+1]
		sb := src[offset+2]
		sa := src[offset+3]
		dr := dst[offset+0]
		dg := dst[offset+1]
		db := dst[offset+2]
		da := dst[offset+3]

		invSa := 255 - sa
		dst[offset+0] = addDiv255(sr, mulDiv255(dr, invSa))
		dst[offset+1] = addDiv255(sg, mulDiv255(dg, invSa))
		dst[offset+2] = addDiv255(sb, mulDiv255(db, invSa))
		dst[offset+3] = addDiv255(sa, mulDiv255(da, invSa))

		offset += 4
	}
}

// BlendBatchAligned is a variant of BlendBatch that assumes dst and src are
// properly aligned and n is a multiple of 16. This removes all boundary checks
// and scalar fallback code for maximum performance.
//
// REQUIREMENTS:
//   - n must be a multiple of 16
//   - dst and src must have at least n*4 bytes available
//
// Use this for inner loops where alignment is guaranteed.
func BlendBatchAligned(dst, src []byte, n int, mode BlendMode) {
	if n <= 0 || n%16 != 0 {
		// Fall back to safe version
		BlendBatch(dst, src, n, mode)
		return
	}

	batchFunc := GetBatchBlendFunc(mode)
	batchCount := n / 16

	var batch wide.BatchState
	offset := 0

	for i := 0; i < batchCount; i++ {
		batch.LoadSrc(src[offset:])
		batch.LoadDst(dst[offset:])
		batchFunc(&batch)
		batch.StoreDst(dst[offset:])
		offset += 64
	}
}

// BlendSourceOverBatchAligned is the aligned version of BlendSourceOverBatch.
// See BlendBatchAligned for requirements.
func BlendSourceOverBatchAligned(dst, src []byte, n int) {
	if n <= 0 || n%16 != 0 {
		// Fall back to safe version
		BlendSourceOverBatch(dst, src, n)
		return
	}

	batchCount := n / 16

	var batch wide.BatchState
	offset := 0

	for i := 0; i < batchCount; i++ {
		batch.LoadSrc(src[offset:])
		batch.LoadDst(dst[offset:])
		SourceOverBatch(&batch)
		batch.StoreDst(dst[offset:])
		offset += 64
	}
}
