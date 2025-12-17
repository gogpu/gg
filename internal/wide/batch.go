package wide

// BatchState holds 16 RGBA pixels for batch processing.
// Uses Structure-of-Arrays (SoA) layout for SIMD-friendly access.
//
// Traditional Array-of-Structures (AoS) layout:
//
//	[R0, G0, B0, A0, R1, G1, B1, A1, ...]
//
// Structure-of-Arrays (SoA) layout:
//
//	SR: [R0, R1, R2, ..., R15]
//	SG: [G0, G1, G2, ..., G15]
//	SB: [B0, B1, B2, ..., B15]
//	SA: [A0, A1, A2, ..., A15]
//
// SoA layout enables SIMD operations on entire color channels at once.
type BatchState struct {
	SR, SG, SB, SA U16x16 // Source RGBA (16 pixels)
	DR, DG, DB, DA U16x16 // Destination RGBA (16 pixels)
}

// LoadSrc loads 16 RGBA pixels from byte slice into source channels.
// src must have at least 64 bytes (16 pixels * 4 bytes).
// Each pixel is stored as [R, G, B, A] in the byte slice.
func (b *BatchState) LoadSrc(src []byte) {
	for i := 0; i < 16; i++ {
		offset := i * 4
		b.SR[i] = uint16(src[offset+0])
		b.SG[i] = uint16(src[offset+1])
		b.SB[i] = uint16(src[offset+2])
		b.SA[i] = uint16(src[offset+3])
	}
}

// LoadDst loads 16 RGBA pixels from byte slice into destination channels.
// dst must have at least 64 bytes (16 pixels * 4 bytes).
// Each pixel is stored as [R, G, B, A] in the byte slice.
func (b *BatchState) LoadDst(dst []byte) {
	for i := 0; i < 16; i++ {
		offset := i * 4
		b.DR[i] = uint16(dst[offset+0])
		b.DG[i] = uint16(dst[offset+1])
		b.DB[i] = uint16(dst[offset+2])
		b.DA[i] = uint16(dst[offset+3])
	}
}

// StoreDst stores 16 RGBA pixels from destination channels to byte slice.
// dst must have at least 64 bytes (16 pixels * 4 bytes).
// Each pixel is stored as [R, G, B, A] in the byte slice.
func (b *BatchState) StoreDst(dst []byte) {
	for i := 0; i < 16; i++ {
		offset := i * 4
		// Intentional truncation - color values are guaranteed to be in [0, 255] range
		dst[offset+0] = uint8(b.DR[i]) // #nosec G115
		dst[offset+1] = uint8(b.DG[i]) // #nosec G115
		dst[offset+2] = uint8(b.DB[i]) // #nosec G115
		dst[offset+3] = uint8(b.DA[i]) // #nosec G115
	}
}
