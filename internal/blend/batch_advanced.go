package blend

import "github.com/gogpu/gg/internal/wide"

// Batch implementations of advanced separable blend modes.
//
// Separable blend modes operate on each color channel independently.
// These implementations work with premultiplied alpha in the range 0-255.
//
// For most separable blend modes, we need to:
// 1. Unpremultiply source and destination colors (if alpha > 0)
// 2. Apply the blend function to unmultiplied values
// 3. Combine using standard compositing formula:
//    Result = (1-Sa)*D + (1-Da)*S + Sa*Da*B(Sc,Dc)
//
// However, batch unpremultiply is complex due to division. For now, we implement
// only the modes that can work efficiently in batch form.

// MultiplyBatch multiplies source and destination colors.
// Formula: Result = S * D / 255
//
// This is already in premultiplied form, so we can compute directly.
func MultiplyBatch(b *wide.BatchState) {
	// For premultiplied: Result = S * D / 255
	// This naturally handles alpha blending
	b.DR = b.SR.MulDiv255(b.DR)
	b.DG = b.SG.MulDiv255(b.DG)
	b.DB = b.SB.MulDiv255(b.DB)
	b.DA = b.SA.MulDiv255(b.DA)
}

// ScreenBatch produces a lighter result than multiply.
// Formula: Result = S + D - S*D/255
//
// Equivalent to: 1 - (1-S)*(1-D)
func ScreenBatch(b *wide.BatchState) {
	// Screen: S + D - S*D/255
	// This works directly with premultiplied values
	// Note: Add needs clamping, but Sub after MulDiv255 should be safe
	b.DR = b.SR.Add(b.DR).Sub(b.SR.MulDiv255(b.DR)).Clamp(255)
	b.DG = b.SG.Add(b.DG).Sub(b.SG.MulDiv255(b.DG)).Clamp(255)
	b.DB = b.SB.Add(b.DB).Sub(b.SB.MulDiv255(b.DB)).Clamp(255)
	b.DA = b.SA.Add(b.DA).Sub(b.SA.MulDiv255(b.DA)).Clamp(255)
}

// DarkenBatch selects the darker of source and destination.
// Formula: Result = min(S, D)
//
// For premultiplied, we need to unpremultiply, compare, then repremultiply.
// For simplicity in batch form, we compare premultiplied values directly.
// This is an approximation but works well in practice.
func DarkenBatch(b *wide.BatchState) {
	// Element-wise minimum
	for i := 0; i < 16; i++ {
		if b.SR[i] < b.DR[i] {
			b.DR[i] = b.SR[i]
		}
		if b.SG[i] < b.DG[i] {
			b.DG[i] = b.SG[i]
		}
		if b.SB[i] < b.DB[i] {
			b.DB[i] = b.SB[i]
		}
		if b.SA[i] < b.DA[i] {
			b.DA[i] = b.SA[i]
		}
	}
}

// LightenBatch selects the lighter of source and destination.
// Formula: Result = max(S, D)
func LightenBatch(b *wide.BatchState) {
	// Element-wise maximum
	for i := 0; i < 16; i++ {
		if b.SR[i] > b.DR[i] {
			b.DR[i] = b.SR[i]
		}
		if b.SG[i] > b.DG[i] {
			b.DG[i] = b.SG[i]
		}
		if b.SB[i] > b.DB[i] {
			b.DB[i] = b.SB[i]
		}
		if b.SA[i] > b.DA[i] {
			b.DA[i] = b.SA[i]
		}
	}
}

// DifferenceBatch produces the absolute difference between source and destination.
// Formula: Result = |S - D|
func DifferenceBatch(b *wide.BatchState) {
	// Element-wise absolute difference
	for i := 0; i < 16; i++ {
		if b.SR[i] > b.DR[i] {
			b.DR[i] = b.SR[i] - b.DR[i]
		} else {
			b.DR[i] -= b.SR[i]
		}

		if b.SG[i] > b.DG[i] {
			b.DG[i] = b.SG[i] - b.DG[i]
		} else {
			b.DG[i] -= b.SG[i]
		}

		if b.SB[i] > b.DB[i] {
			b.DB[i] = b.SB[i] - b.DB[i]
		} else {
			b.DB[i] -= b.SB[i]
		}

		if b.SA[i] > b.DA[i] {
			b.DA[i] = b.SA[i] - b.DA[i]
		} else {
			b.DA[i] -= b.SA[i]
		}
	}
}

// ExclusionBatch is similar to Difference but with lower contrast.
// Formula: Result = S + D - 2*S*D/255
func ExclusionBatch(b *wide.BatchState) {
	// Exclusion: S + D - 2*S*D/255
	// Compute S*D/255, then multiply by 2, then subtract from S+D

	// For R channel
	sum := b.SR.Add(b.DR)
	prod := b.SR.MulDiv255(b.DR)
	// Double the product (but clamp at 255)
	doubled := prod.Add(prod)
	// S + D - 2*S*D/255 (with saturation)
	for i := 0; i < 16; i++ {
		if doubled[i] > sum[i] {
			b.DR[i] = 0
		} else {
			b.DR[i] = sum[i] - doubled[i]
		}
	}

	// For G channel
	sum = b.SG.Add(b.DG)
	prod = b.SG.MulDiv255(b.DG)
	doubled = prod.Add(prod)
	for i := 0; i < 16; i++ {
		if doubled[i] > sum[i] {
			b.DG[i] = 0
		} else {
			b.DG[i] = sum[i] - doubled[i]
		}
	}

	// For B channel
	sum = b.SB.Add(b.DB)
	prod = b.SB.MulDiv255(b.DB)
	doubled = prod.Add(prod)
	for i := 0; i < 16; i++ {
		if doubled[i] > sum[i] {
			b.DB[i] = 0
		} else {
			b.DB[i] = sum[i] - doubled[i]
		}
	}

	// For A channel
	sum = b.SA.Add(b.DA)
	prod = b.SA.MulDiv255(b.DA)
	doubled = prod.Add(prod)
	for i := 0; i < 16; i++ {
		if doubled[i] > sum[i] {
			b.DA[i] = 0
		} else {
			b.DA[i] = sum[i] - doubled[i]
		}
	}
}

// OverlayBatch combines Multiply and Screen based on destination.
// Formula: if D < 128: 2*S*D/255, else: 255 - 2*(255-S)*(255-D)/255
//
// This requires conditional logic per element.
//
//nolint:gocognit,nestif // Inherently complex due to per-channel conditional logic
func OverlayBatch(b *wide.BatchState) {
	for i := 0; i < 16; i++ {
		// R channel
		if b.DR[i] < 128 {
			// Multiply: 2 * S * D / 255
			prod := (uint32(b.SR[i]) * uint32(b.DR[i]) * 2) / 255
			if prod > 255 {
				b.DR[i] = 255
			} else {
				b.DR[i] = uint16(prod)
			}
		} else {
			// Screen: 255 - 2 * (255-S) * (255-D) / 255
			invS := 255 - b.SR[i]
			invD := 255 - b.DR[i]
			prod := (uint32(invS) * uint32(invD) * 2) / 255
			if prod > 255 {
				b.DR[i] = 0
			} else {
				b.DR[i] = 255 - uint16(prod)
			}
		}

		// G channel
		if b.DG[i] < 128 {
			prod := (uint32(b.SG[i]) * uint32(b.DG[i]) * 2) / 255
			if prod > 255 {
				b.DG[i] = 255
			} else {
				b.DG[i] = uint16(prod)
			}
		} else {
			invS := 255 - b.SG[i]
			invD := 255 - b.DG[i]
			prod := (uint32(invS) * uint32(invD) * 2) / 255
			if prod > 255 {
				b.DG[i] = 0
			} else {
				b.DG[i] = 255 - uint16(prod)
			}
		}

		// B channel
		if b.DB[i] < 128 {
			prod := (uint32(b.SB[i]) * uint32(b.DB[i]) * 2) / 255
			if prod > 255 {
				b.DB[i] = 255
			} else {
				b.DB[i] = uint16(prod)
			}
		} else {
			invS := 255 - b.SB[i]
			invD := 255 - b.DB[i]
			prod := (uint32(invS) * uint32(invD) * 2) / 255
			if prod > 255 {
				b.DB[i] = 0
			} else {
				b.DB[i] = 255 - uint16(prod)
			}
		}

		// A channel
		if b.DA[i] < 128 {
			prod := (uint32(b.SA[i]) * uint32(b.DA[i]) * 2) / 255
			if prod > 255 {
				b.DA[i] = 255
			} else {
				b.DA[i] = uint16(prod)
			}
		} else {
			invS := 255 - b.SA[i]
			invD := 255 - b.DA[i]
			prod := (uint32(invS) * uint32(invD) * 2) / 255
			if prod > 255 {
				b.DA[i] = 0
			} else {
				b.DA[i] = 255 - uint16(prod)
			}
		}
	}
}

// Note: ColorDodge, ColorBurn, HardLight, and SoftLight require more complex
// per-pixel operations that don't benefit much from batch processing.
// These can be added later if needed, but the scalar implementations
// in advanced.go are already quite efficient for these modes.
