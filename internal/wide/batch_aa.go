// Package wide provides SIMD-friendly wide types for batch pixel processing.
// This file implements batch anti-aliased blending operations.
package wide

// BlendBatchAA applies a constant alpha to 16 source pixels and blends them
// over destination pixels using the SourceOver formula.
//
// This is optimized for anti-aliased rendering where many pixels share
// the same coverage alpha value. Instead of blending each pixel individually,
// we process 16 at a time using SIMD-friendly operations.
//
// Formula: Result = S * coverageAlpha + D * (1 - S.A * coverageAlpha)
//
// For premultiplied alpha, the formula simplifies to:
//
//	R_out = S_R * alpha/255 + D_R * (255 - S_A * alpha/255) / 255
//
// Parameters:
//   - b: BatchState containing source and destination pixels in SoA layout
//   - alpha: coverage alpha value (0-255) to apply to all 16 source pixels
func BlendBatchAA(b *BatchState, alpha uint8) {
	if alpha == 0 {
		// No coverage - destination unchanged
		return
	}

	if alpha == 255 {
		// Full coverage - standard SourceOver
		SourceOverBatchAA(b)
		return
	}

	// Scale source by coverage alpha
	// scaledS = S * (alpha / 255)
	alphaVec := SplatU16(uint16(alpha))

	scaledSR := b.SR.MulDiv255(alphaVec)
	scaledSG := b.SG.MulDiv255(alphaVec)
	scaledSB := b.SB.MulDiv255(alphaVec)
	scaledSA := b.SA.MulDiv255(alphaVec)

	// Calculate inverse of scaled alpha: 255 - scaledSA
	invScaledSA := scaledSA.Inv()

	// Apply SourceOver: Result = scaledS + D * invScaledSA
	b.DR = scaledSR.Add(b.DR.MulDiv255(invScaledSA)).Clamp(255)
	b.DG = scaledSG.Add(b.DG.MulDiv255(invScaledSA)).Clamp(255)
	b.DB = scaledSB.Add(b.DB.MulDiv255(invScaledSA)).Clamp(255)
	b.DA = scaledSA.Add(b.DA.MulDiv255(invScaledSA)).Clamp(255)
}

// SourceOverBatchAA performs SourceOver blending on 16 pixels.
// This is identical to SourceOverBatch but duplicated here to avoid
// import cycles between wide and blend packages.
//
// Formula: Result = S + D * (1 - Sa)
func SourceOverBatchAA(b *BatchState) {
	invSA := b.SA.Inv()
	b.DR = b.SR.Add(b.DR.MulDiv255(invSA)).Clamp(255)
	b.DG = b.SG.Add(b.DG.MulDiv255(invSA)).Clamp(255)
	b.DB = b.SB.Add(b.DB.MulDiv255(invSA)).Clamp(255)
	b.DA = b.SA.Add(b.DA.MulDiv255(invSA)).Clamp(255)
}

// BlendSolidColorBatchAA blends a solid color (same for all 16 pixels)
// over destination pixels with a constant coverage alpha.
//
// This is even more optimized than BlendBatchAA when the source color
// is constant across all pixels, which is common in anti-aliased fill
// operations.
//
// Parameters:
//   - dst: destination buffer (16 pixels * 4 bytes = 64 bytes minimum)
//   - r, g, b, a: source color components (premultiplied alpha, 0-255)
//   - alpha: coverage alpha (0-255)
func BlendSolidColorBatchAA(dst []byte, r, g, b, a, alpha uint8) {
	if alpha == 0 {
		return // No coverage - destination unchanged
	}

	// Pre-compute scaled source color
	// scaledS = S * (alpha / 255)
	scaledR := mulDiv255Fast(r, alpha)
	scaledG := mulDiv255Fast(g, alpha)
	scaledB := mulDiv255Fast(b, alpha)
	scaledA := mulDiv255Fast(a, alpha)

	// Pre-compute inverse of scaled alpha
	invScaledA := 255 - scaledA

	// Process 16 pixels
	var batch BatchState
	batch.LoadDst(dst)

	// Splat the solid color to all lanes
	splatR := SplatU16(uint16(scaledR))
	splatG := SplatU16(uint16(scaledG))
	splatB := SplatU16(uint16(scaledB))
	splatA := SplatU16(uint16(scaledA))
	splatInvA := SplatU16(uint16(invScaledA))

	// Apply SourceOver: Result = scaledS + D * invScaledA
	batch.DR = splatR.Add(batch.DR.MulDiv255(splatInvA)).Clamp(255)
	batch.DG = splatG.Add(batch.DG.MulDiv255(splatInvA)).Clamp(255)
	batch.DB = splatB.Add(batch.DB.MulDiv255(splatInvA)).Clamp(255)
	batch.DA = splatA.Add(batch.DA.MulDiv255(splatInvA)).Clamp(255)

	batch.StoreDst(dst)
}

// mulDiv255Fast multiplies two bytes and divides by 255 using fast approximation.
// Formula: (a * b + 255) >> 8
func mulDiv255Fast(a, b uint8) uint8 {
	x := uint16(a) * uint16(b)
	return uint8((x + 255) >> 8) // #nosec G115 - result bounded by 255
}

// BlendSolidColorSpanAA blends a solid color over a span of pixels with
// constant coverage alpha. This is the main entry point for AA rasterizer.
//
// Automatically uses batch (16px) or scalar based on count.
//
// Parameters:
//   - dst: destination buffer in RGBA format
//   - count: number of pixels to blend
//   - r, g, b, a: source color components (premultiplied alpha, 0-255)
//   - alpha: coverage alpha (0-255)
func BlendSolidColorSpanAA(dst []byte, count int, r, g, b, a, alpha uint8) {
	if count <= 0 || alpha == 0 {
		return
	}

	// Pre-compute scaled source and inverse alpha
	scaledR := mulDiv255Fast(r, alpha)
	scaledG := mulDiv255Fast(g, alpha)
	scaledB := mulDiv255Fast(b, alpha)
	scaledA := mulDiv255Fast(a, alpha)
	invScaledA := 255 - scaledA

	// Process batches of 16 pixels
	offset := 0
	batchCount := count / 16

	if batchCount > 0 {
		splatR := SplatU16(uint16(scaledR))
		splatG := SplatU16(uint16(scaledG))
		splatB := SplatU16(uint16(scaledB))
		splatA := SplatU16(uint16(scaledA))
		splatInvA := SplatU16(uint16(invScaledA))

		var batch BatchState
		for i := 0; i < batchCount; i++ {
			batch.LoadDst(dst[offset:])

			batch.DR = splatR.Add(batch.DR.MulDiv255(splatInvA)).Clamp(255)
			batch.DG = splatG.Add(batch.DG.MulDiv255(splatInvA)).Clamp(255)
			batch.DB = splatB.Add(batch.DB.MulDiv255(splatInvA)).Clamp(255)
			batch.DA = splatA.Add(batch.DA.MulDiv255(splatInvA)).Clamp(255)

			batch.StoreDst(dst[offset:])
			offset += 64 // 16 pixels * 4 bytes
		}
	}

	// Process remaining pixels with scalar operations
	remainder := count % 16
	for i := 0; i < remainder; i++ {
		dr := dst[offset+0]
		dg := dst[offset+1]
		db := dst[offset+2]
		da := dst[offset+3]

		// SourceOver: Result = scaledS + D * invScaledA
		dst[offset+0] = addClamp(scaledR, mulDiv255Fast(dr, invScaledA))
		dst[offset+1] = addClamp(scaledG, mulDiv255Fast(dg, invScaledA))
		dst[offset+2] = addClamp(scaledB, mulDiv255Fast(db, invScaledA))
		dst[offset+3] = addClamp(scaledA, mulDiv255Fast(da, invScaledA))

		offset += 4
	}
}

// addClamp adds two bytes and clamps to 255.
func addClamp(a, b uint8) uint8 {
	sum := uint16(a) + uint16(b)
	if sum > 255 {
		return 255
	}
	return uint8(sum) // #nosec G115 - bounded by 255
}
