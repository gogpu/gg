package gg

import (
	"math"
	"testing"
)

func TestNewDash(t *testing.T) {
	tests := []struct {
		name      string
		lengths   []float64
		wantNil   bool
		wantArray []float64
	}{
		{
			name:    "empty input returns nil",
			lengths: []float64{},
			wantNil: true,
		},
		{
			name:    "nil input returns nil",
			lengths: nil,
			wantNil: true,
		},
		{
			name:    "all zeros returns nil",
			lengths: []float64{0, 0, 0},
			wantNil: true,
		},
		{
			name:      "simple dash-gap pattern",
			lengths:   []float64{5, 3},
			wantNil:   false,
			wantArray: []float64{5, 3},
		},
		{
			name:      "single value (becomes duplicated pattern)",
			lengths:   []float64{5},
			wantNil:   false,
			wantArray: []float64{5},
		},
		{
			name:      "complex pattern",
			lengths:   []float64{10, 5, 2, 5},
			wantNil:   false,
			wantArray: []float64{10, 5, 2, 5},
		},
		{
			name:      "negative values become absolute",
			lengths:   []float64{-5, 3},
			wantNil:   false,
			wantArray: []float64{5, 3},
		},
		{
			name:    "all negative zeros returns nil",
			lengths: []float64{-0, 0},
			wantNil: true,
		},
		{
			name:      "mixed positive and zero",
			lengths:   []float64{5, 0, 3},
			wantNil:   false,
			wantArray: []float64{5, 0, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewDash(tt.lengths...)
			if tt.wantNil {
				if got != nil {
					t.Errorf("NewDash() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("NewDash() = nil, want non-nil")
			}
			if len(got.Array) != len(tt.wantArray) {
				t.Errorf("NewDash().Array length = %d, want %d", len(got.Array), len(tt.wantArray))
				return
			}
			for i, v := range got.Array {
				if v != tt.wantArray[i] {
					t.Errorf("NewDash().Array[%d] = %v, want %v", i, v, tt.wantArray[i])
				}
			}
			if got.Offset != 0 {
				t.Errorf("NewDash().Offset = %v, want 0", got.Offset)
			}
		})
	}
}

func TestDash_WithOffset(t *testing.T) {
	tests := []struct {
		name       string
		dash       *Dash
		offset     float64
		wantNil    bool
		wantOffset float64
	}{
		{
			name:    "nil dash returns nil",
			dash:    nil,
			offset:  10,
			wantNil: true,
		},
		{
			name:       "positive offset",
			dash:       NewDash(5, 3),
			offset:     2.5,
			wantOffset: 2.5,
		},
		{
			name:       "negative offset",
			dash:       NewDash(5, 3),
			offset:     -1.5,
			wantOffset: -1.5,
		},
		{
			name:       "zero offset",
			dash:       NewDash(5, 3),
			offset:     0,
			wantOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dash.WithOffset(tt.offset)
			if tt.wantNil {
				if got != nil {
					t.Errorf("WithOffset() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("WithOffset() = nil, want non-nil")
			}
			if got.Offset != tt.wantOffset {
				t.Errorf("WithOffset().Offset = %v, want %v", got.Offset, tt.wantOffset)
			}
			// Original should be unchanged
			if tt.dash.Offset != 0 {
				t.Errorf("original Dash.Offset was modified: %v", tt.dash.Offset)
			}
		})
	}
}

func TestDash_PatternLength(t *testing.T) {
	tests := []struct {
		name      string
		dash      *Dash
		want      float64
		tolerance float64
	}{
		{
			name: "nil dash",
			dash: nil,
			want: 0,
		},
		{
			name: "simple even pattern",
			dash: NewDash(5, 3),
			want: 8,
		},
		{
			name: "odd pattern (duplicated)",
			dash: NewDash(5),
			want: 10, // [5] becomes [5, 5], so 10 total
		},
		{
			name: "complex even pattern",
			dash: NewDash(10, 5, 2, 5),
			want: 22,
		},
		{
			name: "three element odd pattern",
			dash: NewDash(5, 3, 2),
			want: 20, // [5, 3, 2] becomes [5, 3, 2, 5, 3, 2], so 20 total
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dash.PatternLength()
			if got != tt.want {
				t.Errorf("PatternLength() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDash_IsDashed(t *testing.T) {
	tests := []struct {
		name string
		dash *Dash
		want bool
	}{
		{
			name: "nil dash",
			dash: nil,
			want: false,
		},
		{
			name: "valid dash",
			dash: NewDash(5, 3),
			want: true,
		},
		{
			name: "single element dash",
			dash: NewDash(5),
			want: true,
		},
		{
			name: "empty array dash",
			dash: &Dash{Array: []float64{}},
			want: false,
		},
		{
			name: "all zeros dash",
			dash: &Dash{Array: []float64{0, 0}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dash.IsDashed()
			if got != tt.want {
				t.Errorf("IsDashed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDash_Clone(t *testing.T) {
	t.Run("nil dash returns nil", func(t *testing.T) {
		var d *Dash
		got := d.Clone()
		if got != nil {
			t.Errorf("Clone() = %v, want nil", got)
		}
	})

	t.Run("clones array and offset", func(t *testing.T) {
		original := NewDash(5, 3).WithOffset(2)
		clone := original.Clone()

		if clone == nil {
			t.Fatal("Clone() = nil, want non-nil")
		}
		if clone == original {
			t.Error("Clone() returned same pointer")
		}
		if &clone.Array[0] == &original.Array[0] {
			t.Error("Clone() shares array slice")
		}
		if clone.Offset != original.Offset {
			t.Errorf("Clone().Offset = %v, want %v", clone.Offset, original.Offset)
		}
		for i, v := range clone.Array {
			if v != original.Array[i] {
				t.Errorf("Clone().Array[%d] = %v, want %v", i, v, original.Array[i])
			}
		}
	})

	t.Run("modifying clone does not affect original", func(t *testing.T) {
		original := NewDash(5, 3)
		clone := original.Clone()

		clone.Array[0] = 100
		clone.Offset = 50

		if original.Array[0] != 5 {
			t.Errorf("original.Array[0] = %v, want 5", original.Array[0])
		}
		if original.Offset != 0 {
			t.Errorf("original.Offset = %v, want 0", original.Offset)
		}
	})
}

func TestDash_NormalizedOffset(t *testing.T) {
	tests := []struct {
		name   string
		dash   *Dash
		offset float64
		want   float64
	}{
		{
			name: "nil dash",
			dash: nil,
			want: 0,
		},
		{
			name:   "offset within pattern",
			dash:   NewDash(5, 3),
			offset: 2,
			want:   2,
		},
		{
			name:   "offset equals pattern length",
			dash:   NewDash(5, 3),
			offset: 8,
			want:   0,
		},
		{
			name:   "offset greater than pattern",
			dash:   NewDash(5, 3),
			offset: 10,
			want:   2,
		},
		{
			name:   "negative offset",
			dash:   NewDash(5, 3),
			offset: -2,
			want:   6,
		},
		{
			name:   "large negative offset",
			dash:   NewDash(5, 3),
			offset: -18,
			want:   6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.dash
			if d != nil {
				d = d.WithOffset(tt.offset)
			}
			got := d.NormalizedOffset()
			if math.Abs(got-tt.want) > 1e-10 {
				t.Errorf("NormalizedOffset() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDash_effectiveArray(t *testing.T) {
	tests := []struct {
		name string
		dash *Dash
		want []float64
	}{
		{
			name: "nil dash",
			dash: nil,
			want: nil,
		},
		{
			name: "empty array",
			dash: &Dash{Array: []float64{}},
			want: nil,
		},
		{
			name: "even length array unchanged",
			dash: NewDash(5, 3),
			want: []float64{5, 3},
		},
		{
			name: "odd length array duplicated",
			dash: NewDash(5),
			want: []float64{5, 5},
		},
		{
			name: "three element array duplicated",
			dash: NewDash(5, 3, 2),
			want: []float64{5, 3, 2, 5, 3, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dash.effectiveArray()
			if tt.want == nil {
				if got != nil {
					t.Errorf("effectiveArray() = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("effectiveArray() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("effectiveArray()[%d] = %v, want %v", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestDash_EdgeCases(t *testing.T) {
	t.Run("very large values", func(t *testing.T) {
		d := NewDash(1e10, 1e10)
		if d == nil {
			t.Fatal("NewDash with large values = nil")
		}
		if d.PatternLength() != 2e10 {
			t.Errorf("PatternLength() = %v, want 2e10", d.PatternLength())
		}
	})

	t.Run("very small values", func(t *testing.T) {
		d := NewDash(1e-10, 1e-10)
		if d == nil {
			t.Fatal("NewDash with small values = nil")
		}
		if !d.IsDashed() {
			t.Error("IsDashed() = false, want true")
		}
	})

	t.Run("mixed small and large", func(t *testing.T) {
		d := NewDash(1e-10, 1e10)
		if d == nil {
			t.Fatal("NewDash with mixed values = nil")
		}
		patLen := d.PatternLength()
		if patLen < 1e10 {
			t.Errorf("PatternLength() = %v, should be >= 1e10", patLen)
		}
	})
}
