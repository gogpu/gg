package blend

import "github.com/gogpu/gg/internal/wide"

// BatchBlendFunc is signature for batch blend operations on 16 pixels.
// It operates on a BatchState which contains source and destination RGBA channels
// stored in Structure-of-Arrays (SoA) format for SIMD-friendly processing.
//
// The function modifies b.DR, b.DG, b.DB, b.DA in-place with the blend result.
type BatchBlendFunc func(b *wide.BatchState)

// GetBatchBlendFunc returns the batch blend function for the given mode.
// Returns SourceOverBatch for unknown modes (safe default).
func GetBatchBlendFunc(mode BlendMode) BatchBlendFunc {
	switch mode {
	// Porter-Duff modes
	case BlendClear:
		return ClearBatch
	case BlendSource:
		return SourceBatch
	case BlendDestination:
		return DestinationBatch
	case BlendSourceOver:
		return SourceOverBatch
	case BlendDestinationOver:
		return DestinationOverBatch
	case BlendSourceIn:
		return SourceInBatch
	case BlendDestinationIn:
		return DestinationInBatch
	case BlendSourceOut:
		return SourceOutBatch
	case BlendDestinationOut:
		return DestinationOutBatch
	case BlendSourceAtop:
		return SourceAtopBatch
	case BlendDestinationAtop:
		return DestinationAtopBatch
	case BlendXor:
		return XorBatch
	case BlendPlus:
		return PlusBatch
	case BlendModulate:
		return ModulateBatch

	default:
		return SourceOverBatch
	}
}

// ClearBatch clears the destination to transparent black.
// Formula: Result = 0
func ClearBatch(b *wide.BatchState) {
	zero := wide.SplatU16(0)
	b.DR = zero
	b.DG = zero
	b.DB = zero
	b.DA = zero
}

// SourceBatch replaces destination with source.
// Formula: Result = S
func SourceBatch(b *wide.BatchState) {
	b.DR = b.SR
	b.DG = b.SG
	b.DB = b.SB
	b.DA = b.SA
}

// DestinationBatch keeps destination unchanged (no-op).
// Formula: Result = D
func DestinationBatch(b *wide.BatchState) {
	// No-op: destination already contains the result
}

// SourceOverBatch composites source over destination (default blend mode).
// Formula: Result = S + D * (1 - Sa)
//
// This is the most common blend mode, equivalent to CSS "normal" blending.
// It places the source "over" the destination, with source alpha determining visibility.
func SourceOverBatch(b *wide.BatchState) {
	invSA := b.SA.Inv()
	b.DR = b.SR.Add(b.DR.MulDiv255(invSA)).Clamp(255)
	b.DG = b.SG.Add(b.DG.MulDiv255(invSA)).Clamp(255)
	b.DB = b.SB.Add(b.DB.MulDiv255(invSA)).Clamp(255)
	b.DA = b.SA.Add(b.DA.MulDiv255(invSA)).Clamp(255)
}

// DestinationOverBatch composites destination over source.
// Formula: Result = S * (1 - Da) + D
func DestinationOverBatch(b *wide.BatchState) {
	invDA := b.DA.Inv()
	b.DR = b.SR.MulDiv255(invDA).Add(b.DR).Clamp(255)
	b.DG = b.SG.MulDiv255(invDA).Add(b.DG).Clamp(255)
	b.DB = b.SB.MulDiv255(invDA).Add(b.DB).Clamp(255)
	b.DA = b.SA.MulDiv255(invDA).Add(b.DA).Clamp(255)
}

// SourceInBatch shows source where destination is opaque.
// Formula: Result = S * Da
func SourceInBatch(b *wide.BatchState) {
	b.DR = b.SR.MulDiv255(b.DA)
	b.DG = b.SG.MulDiv255(b.DA)
	b.DB = b.SB.MulDiv255(b.DA)
	b.DA = b.SA.MulDiv255(b.DA)
}

// DestinationInBatch shows destination where source is opaque.
// Formula: Result = D * Sa
func DestinationInBatch(b *wide.BatchState) {
	b.DR = b.DR.MulDiv255(b.SA)
	b.DG = b.DG.MulDiv255(b.SA)
	b.DB = b.DB.MulDiv255(b.SA)
	b.DA = b.DA.MulDiv255(b.SA)
}

// SourceOutBatch shows source where destination is transparent.
// Formula: Result = S * (1 - Da)
func SourceOutBatch(b *wide.BatchState) {
	invDA := b.DA.Inv()
	b.DR = b.SR.MulDiv255(invDA)
	b.DG = b.SG.MulDiv255(invDA)
	b.DB = b.SB.MulDiv255(invDA)
	b.DA = b.SA.MulDiv255(invDA)
}

// DestinationOutBatch shows destination where source is transparent.
// Formula: Result = D * (1 - Sa)
func DestinationOutBatch(b *wide.BatchState) {
	invSA := b.SA.Inv()
	b.DR = b.DR.MulDiv255(invSA)
	b.DG = b.DG.MulDiv255(invSA)
	b.DB = b.DB.MulDiv255(invSA)
	b.DA = b.DA.MulDiv255(invSA)
}

// SourceAtopBatch composites source over destination, preserving destination alpha.
// Formula: Result = S * Da + D * (1 - Sa)
func SourceAtopBatch(b *wide.BatchState) {
	invSA := b.SA.Inv()
	b.DR = b.SR.MulDiv255(b.DA).Add(b.DR.MulDiv255(invSA)).Clamp(255)
	b.DG = b.SG.MulDiv255(b.DA).Add(b.DG.MulDiv255(invSA)).Clamp(255)
	b.DB = b.SB.MulDiv255(b.DA).Add(b.DB.MulDiv255(invSA)).Clamp(255)
	// Alpha = Da (destination alpha unchanged)
}

// DestinationAtopBatch composites destination over source, preserving source alpha.
// Formula: Result = S * (1 - Da) + D * Sa
func DestinationAtopBatch(b *wide.BatchState) {
	invDA := b.DA.Inv()
	b.DR = b.SR.MulDiv255(invDA).Add(b.DR.MulDiv255(b.SA)).Clamp(255)
	b.DG = b.SG.MulDiv255(invDA).Add(b.DG.MulDiv255(b.SA)).Clamp(255)
	b.DB = b.SB.MulDiv255(invDA).Add(b.DB.MulDiv255(b.SA)).Clamp(255)
	b.DA = b.SA // Alpha = Sa (source alpha)
}

// XorBatch shows source and destination where they don't overlap.
// Formula: Result = S * (1 - Da) + D * (1 - Sa)
func XorBatch(b *wide.BatchState) {
	invDA := b.DA.Inv()
	invSA := b.SA.Inv()
	b.DR = b.SR.MulDiv255(invDA).Add(b.DR.MulDiv255(invSA)).Clamp(255)
	b.DG = b.SG.MulDiv255(invDA).Add(b.DG.MulDiv255(invSA)).Clamp(255)
	b.DB = b.SB.MulDiv255(invDA).Add(b.DB.MulDiv255(invSA)).Clamp(255)
	b.DA = b.SA.MulDiv255(invDA).Add(b.DA.MulDiv255(invSA)).Clamp(255)
}

// PlusBatch adds source and destination colors (clamped to 255).
// Formula: Result = min(S + D, 255)
func PlusBatch(b *wide.BatchState) {
	b.DR = b.SR.Add(b.DR).Clamp(255)
	b.DG = b.SG.Add(b.DG).Clamp(255)
	b.DB = b.SB.Add(b.DB).Clamp(255)
	b.DA = b.SA.Add(b.DA).Clamp(255)
}

// ModulateBatch multiplies source and destination colors.
// Formula: Result = S * D / 255
func ModulateBatch(b *wide.BatchState) {
	b.DR = b.SR.MulDiv255(b.DR)
	b.DG = b.SG.MulDiv255(b.DG)
	b.DB = b.SB.MulDiv255(b.DB)
	b.DA = b.SA.MulDiv255(b.DA)
}
