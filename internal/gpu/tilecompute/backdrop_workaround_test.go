package tilecompute

import (
	"testing"
)

// TestBackdropNestedVsFlat compares the nested for-loop pattern (Rust Vello original)
// with our flat loop workaround. Both should produce identical results.
// If they do, removing the workaround is safe (at least at the algorithm level).
func TestBackdropNestedVsFlat(t *testing.T) {
	tests := []struct {
		name    string
		bboxW   int
		bboxH   int
		initial []int32 // initial backdrop values (row-major)
	}{
		{
			name:    "single row",
			bboxW:   5,
			bboxH:   1,
			initial: []int32{1, 0, -1, 1, 0},
		},
		{
			name:    "single column",
			bboxW:   1,
			bboxH:   4,
			initial: []int32{1, -1, 1, -1},
		},
		{
			name:  "3x3 grid",
			bboxW: 3,
			bboxH: 3,
			initial: []int32{
				1, 0, -1,
				0, 1, 0,
				-1, 0, 1,
			},
		},
		{
			name:  "4x2 mixed",
			bboxW: 4,
			bboxH: 2,
			initial: []int32{
				1, 1, -1, -1,
				-1, 1, 1, -1,
			},
		},
		{
			name:    "1x1 trivial",
			bboxW:   1,
			bboxH:   1,
			initial: []int32{42},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run nested loop (Rust Vello pattern)
			nested := make([]int32, len(tt.initial))
			copy(nested, tt.initial)
			backdropNested(nested, tt.bboxW, tt.bboxH)

			// Run flat loop (our workaround)
			flat := make([]int32, len(tt.initial))
			copy(flat, tt.initial)
			backdropFlat(flat, tt.bboxW, tt.bboxH)

			// Compare
			for i := range nested {
				if nested[i] != flat[i] {
					t.Errorf("mismatch at index %d: nested=%d flat=%d", i, nested[i], flat[i])
				}
			}
		})
	}
}

// backdropNested is the Rust Vello original pattern (nested for-loops).
func backdropNested(tiles []int32, bboxW, bboxH int) {
	for y := 0; y < bboxH; y++ {
		sum := int32(0)
		for x := 0; x < bboxW; x++ {
			idx := y*bboxW + x
			sum += tiles[idx]
			tiles[idx] = sum
		}
	}
}

// backdropFlat is our WGSL workaround (flat loop with manual row tracking).
func backdropFlat(tiles []int32, bboxW, bboxH int) {
	rowY := 0
	sum := int32(0)
	total := bboxW * bboxH
	for i := 0; i < total; i++ {
		curY := i / bboxW
		if curY != rowY {
			rowY = curY
			sum = 0
		}
		sum += tiles[i]
		tiles[i] = sum
	}
}
