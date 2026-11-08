package gpu

import (
	"bytes"
	"testing"
)

func TestReadbackRowPitch(t *testing.T) {
	for _, tc := range []struct{ w, want uint32 }{
		{1, 256}, {64, 256}, {65, 512}, {100, 512}, {128, 512}, {129, 768},
	} {
		if got := readbackRowPitch(tc.w); got != tc.want {
			t.Errorf("readbackRowPitch(%d) = %d, want %d", tc.w, got, tc.want)
		}
		if got := readbackRowPitch(tc.w); got%copyBytesPerRowAlignment != 0 || got < tc.w*4 {
			t.Errorf("readbackRowPitch(%d) = %d: not aligned or too small", tc.w, got)
		}
	}
}

func TestUnpadRows(t *testing.T) {
	const w, h = 3, 2
	pitch := readbackRowPitch(w)
	src := make([]byte, pitch*h)
	for y := uint32(0); y < h; y++ {
		for i := uint32(0); i < w*4; i++ {
			src[y*pitch+i] = byte(y*100 + i)
		}
		src[y*pitch+w*4] = 0xEE // padding must not leak into the output
	}
	got := unpadRows(src, w, h, pitch)
	want := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111}
	if !bytes.Equal(got, want) {
		t.Fatalf("unpadRows = %v, want %v", got, want)
	}
	if packed := unpadRows(want, 3, 2, 12); &packed[0] != &want[0] {
		t.Error("unpadRows copied a tightly packed buffer")
	}
}
