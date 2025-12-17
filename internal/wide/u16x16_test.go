package wide

import "testing"

func TestSplatU16(t *testing.T) {
	tests := []struct {
		name  string
		value uint16
	}{
		{"zero", 0},
		{"max", 255},
		{"mid", 128},
		{"one", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplatU16(tt.value)
			for i, v := range result {
				if v != tt.value {
					t.Errorf("element %d = %d, want %d", i, v, tt.value)
				}
			}
		})
	}
}

func TestU16x16_Add(t *testing.T) {
	tests := []struct {
		name string
		a    U16x16
		b    U16x16
		want U16x16
	}{
		{
			name: "zeros",
			a:    SplatU16(0),
			b:    SplatU16(0),
			want: SplatU16(0),
		},
		{
			name: "ones",
			a:    SplatU16(1),
			b:    SplatU16(1),
			want: SplatU16(2),
		},
		{
			name: "mixed",
			a:    SplatU16(100),
			b:    SplatU16(50),
			want: SplatU16(150),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Add(tt.b)
			if got != tt.want {
				t.Errorf("Add() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestU16x16_Sub(t *testing.T) {
	tests := []struct {
		name string
		a    U16x16
		b    U16x16
		want U16x16
	}{
		{
			name: "zeros",
			a:    SplatU16(0),
			b:    SplatU16(0),
			want: SplatU16(0),
		},
		{
			name: "equal",
			a:    SplatU16(100),
			b:    SplatU16(100),
			want: SplatU16(0),
		},
		{
			name: "mixed",
			a:    SplatU16(200),
			b:    SplatU16(50),
			want: SplatU16(150),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Sub(tt.b)
			if got != tt.want {
				t.Errorf("Sub() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestU16x16_Mul(t *testing.T) {
	tests := []struct {
		name string
		a    U16x16
		b    U16x16
		want U16x16
	}{
		{
			name: "zeros",
			a:    SplatU16(0),
			b:    SplatU16(100),
			want: SplatU16(0),
		},
		{
			name: "ones",
			a:    SplatU16(1),
			b:    SplatU16(255),
			want: SplatU16(255),
		},
		{
			name: "mixed",
			a:    SplatU16(10),
			b:    SplatU16(20),
			want: SplatU16(200),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Mul(tt.b)
			if got != tt.want {
				t.Errorf("Mul() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestU16x16_Div255(t *testing.T) {
	tests := []struct {
		name  string
		input U16x16
		want  U16x16
	}{
		{
			name:  "zero",
			input: SplatU16(0),
			want:  SplatU16(0),
		},
		{
			name:  "255",
			input: SplatU16(255),
			want:  SplatU16(1),
		},
		{
			name:  "510",
			input: SplatU16(510),
			want:  SplatU16(2),
		},
		{
			name:  "max product",
			input: SplatU16(255 * 255),
			want:  SplatU16(255),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.Div255()
			if got != tt.want {
				t.Errorf("Div255() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestU16x16_Inv(t *testing.T) {
	tests := []struct {
		name  string
		input U16x16
		want  U16x16
	}{
		{
			name:  "zero",
			input: SplatU16(0),
			want:  SplatU16(255),
		},
		{
			name:  "max",
			input: SplatU16(255),
			want:  SplatU16(0),
		},
		{
			name:  "mid",
			input: SplatU16(128),
			want:  SplatU16(127),
		},
		{
			name:  "one",
			input: SplatU16(1),
			want:  SplatU16(254),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.Inv()
			if got != tt.want {
				t.Errorf("Inv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestU16x16_MulDiv255(t *testing.T) {
	tests := []struct {
		name string
		a    U16x16
		b    U16x16
		want U16x16
	}{
		{
			name: "zero",
			a:    SplatU16(0),
			b:    SplatU16(255),
			want: SplatU16(0),
		},
		{
			name: "max",
			a:    SplatU16(255),
			b:    SplatU16(255),
			want: SplatU16(255),
		},
		{
			name: "half alpha",
			a:    SplatU16(200),
			b:    SplatU16(128),
			want: SplatU16(100), // (200 * 128) / 255 ≈ 100
		},
		{
			name: "one",
			a:    SplatU16(255),
			b:    SplatU16(1),
			want: SplatU16(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.MulDiv255(tt.b)
			if got != tt.want {
				t.Errorf("MulDiv255() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestU16x16_Clamp(t *testing.T) {
	tests := []struct {
		name  string
		input U16x16
		max   uint16
		want  U16x16
	}{
		{
			name:  "under max",
			input: SplatU16(100),
			max:   255,
			want:  SplatU16(100),
		},
		{
			name:  "over max",
			input: SplatU16(300),
			max:   255,
			want:  SplatU16(255),
		},
		{
			name:  "equal max",
			input: SplatU16(255),
			max:   255,
			want:  SplatU16(255),
		},
		{
			name:  "zero max",
			input: SplatU16(100),
			max:   0,
			want:  SplatU16(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.Clamp(tt.max)
			if got != tt.want {
				t.Errorf("Clamp(%d) = %v, want %v", tt.max, got, tt.want)
			}
		})
	}
}

func TestU16x16_EdgeCases(t *testing.T) {
	t.Run("overflow wrapping", func(t *testing.T) {
		a := SplatU16(65535)
		b := SplatU16(1)
		got := a.Add(b)
		// Should wrap around due to uint16 overflow
		want := SplatU16(0)
		if got != want {
			t.Errorf("overflow Add() = %v, want %v", got, want)
		}
	})

	t.Run("underflow wrapping", func(t *testing.T) {
		a := SplatU16(0)
		b := SplatU16(1)
		got := a.Sub(b)
		// Should wrap around due to uint16 underflow
		want := SplatU16(65535)
		if got != want {
			t.Errorf("underflow Sub() = %v, want %v", got, want)
		}
	})
}
