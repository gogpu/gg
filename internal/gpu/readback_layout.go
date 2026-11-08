package gpu

// copyBytesPerRowAlignment is the WebGPU COPY_BYTES_PER_ROW_ALIGNMENT: a texture-to-buffer copy must use a
// BytesPerRow that is a multiple of 256. wgpu validates it since v0.34.7, so an unpadded w*4 pitch fails the
// encoder for every width that is not a multiple of 64 pixels (P1-GPU-037).
const copyBytesPerRowAlignment = 256

// readbackRowPitch returns the padded bytes per row for reading back a w-pixel row of a 4-byte-per-pixel texture.
func readbackRowPitch(w uint32) uint32 {
	return (w*4 + copyBytesPerRowAlignment - 1) &^ (copyBytesPerRowAlignment - 1)
}

// unpadRows copies h rows of w 4-byte pixels from src, whose rows are pitch bytes apart, into a tightly packed
// slice. When pitch already equals w*4 it returns src as is.
func unpadRows(src []byte, w, h, pitch uint32) []byte {
	rowBytes := w * 4
	if pitch == rowBytes {
		return src[:uint64(rowBytes)*uint64(h)]
	}
	out := make([]byte, uint64(rowBytes)*uint64(h))
	for y := uint32(0); y < h; y++ {
		copy(out[y*rowBytes:(y+1)*rowBytes], src[y*pitch:y*pitch+rowBytes])
	}
	return out
}
