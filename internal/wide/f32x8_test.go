package wide

import (
	"math"
	"testing"
)

func TestSplatF32(t *testing.T) {
	tests := []struct {
		name  string
		value float32
	}{
		{"zero", 0.0},
		{"one", 1.0},
		{"half", 0.5},
		{"negative", -1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplatF32(tt.value)
			for i, v := range result {
				if v != tt.value {
					t.Errorf("element %d = %f, want %f", i, v, tt.value)
				}
			}
		})
	}
}

func TestF32x8_Add(t *testing.T) {
	tests := []struct {
		name string
		a    F32x8
		b    F32x8
		want F32x8
	}{
		{
			name: "zeros",
			a:    SplatF32(0.0),
			b:    SplatF32(0.0),
			want: SplatF32(0.0),
		},
		{
			name: "ones",
			a:    SplatF32(1.0),
			b:    SplatF32(1.0),
			want: SplatF32(2.0),
		},
		{
			name: "mixed",
			a:    SplatF32(1.5),
			b:    SplatF32(2.5),
			want: SplatF32(4.0),
		},
		{
			name: "negative",
			a:    SplatF32(-1.0),
			b:    SplatF32(1.0),
			want: SplatF32(0.0),
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

func TestF32x8_Sub(t *testing.T) {
	tests := []struct {
		name string
		a    F32x8
		b    F32x8
		want F32x8
	}{
		{
			name: "zeros",
			a:    SplatF32(0.0),
			b:    SplatF32(0.0),
			want: SplatF32(0.0),
		},
		{
			name: "equal",
			a:    SplatF32(5.0),
			b:    SplatF32(5.0),
			want: SplatF32(0.0),
		},
		{
			name: "mixed",
			a:    SplatF32(10.0),
			b:    SplatF32(3.0),
			want: SplatF32(7.0),
		},
		{
			name: "negative result",
			a:    SplatF32(1.0),
			b:    SplatF32(2.0),
			want: SplatF32(-1.0),
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

func TestF32x8_Mul(t *testing.T) {
	tests := []struct {
		name string
		a    F32x8
		b    F32x8
		want F32x8
	}{
		{
			name: "zeros",
			a:    SplatF32(0.0),
			b:    SplatF32(100.0),
			want: SplatF32(0.0),
		},
		{
			name: "ones",
			a:    SplatF32(1.0),
			b:    SplatF32(5.0),
			want: SplatF32(5.0),
		},
		{
			name: "mixed",
			a:    SplatF32(2.5),
			b:    SplatF32(4.0),
			want: SplatF32(10.0),
		},
		{
			name: "negative",
			a:    SplatF32(-2.0),
			b:    SplatF32(3.0),
			want: SplatF32(-6.0),
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

func TestF32x8_Div(t *testing.T) {
	tests := []struct {
		name string
		a    F32x8
		b    F32x8
		want F32x8
	}{
		{
			name: "ones",
			a:    SplatF32(1.0),
			b:    SplatF32(1.0),
			want: SplatF32(1.0),
		},
		{
			name: "mixed",
			a:    SplatF32(10.0),
			b:    SplatF32(2.0),
			want: SplatF32(5.0),
		},
		{
			name: "fractional",
			a:    SplatF32(1.0),
			b:    SplatF32(2.0),
			want: SplatF32(0.5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Div(tt.b)
			if got != tt.want {
				t.Errorf("Div() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("divide by zero", func(t *testing.T) {
		a := SplatF32(1.0)
		b := SplatF32(0.0)
		got := a.Div(b)
		for i, v := range got {
			if !math.IsInf(float64(v), 1) {
				t.Errorf("element %d = %f, want +Inf", i, v)
			}
		}
	})
}

func TestF32x8_Sqrt(t *testing.T) {
	tests := []struct {
		name  string
		input F32x8
		want  F32x8
	}{
		{
			name:  "zero",
			input: SplatF32(0.0),
			want:  SplatF32(0.0),
		},
		{
			name:  "one",
			input: SplatF32(1.0),
			want:  SplatF32(1.0),
		},
		{
			name:  "four",
			input: SplatF32(4.0),
			want:  SplatF32(2.0),
		},
		{
			name:  "nine",
			input: SplatF32(9.0),
			want:  SplatF32(3.0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.Sqrt()
			if got != tt.want {
				t.Errorf("Sqrt() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("negative", func(t *testing.T) {
		input := SplatF32(-1.0)
		got := input.Sqrt()
		for i, v := range got {
			if !math.IsNaN(float64(v)) {
				t.Errorf("element %d = %f, want NaN", i, v)
			}
		}
	})
}

func TestF32x8_Clamp(t *testing.T) {
	tests := []struct {
		name  string
		input F32x8
		min   float32
		max   float32
		want  F32x8
	}{
		{
			name:  "within range",
			input: SplatF32(0.5),
			min:   0.0,
			max:   1.0,
			want:  SplatF32(0.5),
		},
		{
			name:  "below min",
			input: SplatF32(-0.5),
			min:   0.0,
			max:   1.0,
			want:  SplatF32(0.0),
		},
		{
			name:  "above max",
			input: SplatF32(1.5),
			min:   0.0,
			max:   1.0,
			want:  SplatF32(1.0),
		},
		{
			name:  "at min",
			input: SplatF32(0.0),
			min:   0.0,
			max:   1.0,
			want:  SplatF32(0.0),
		},
		{
			name:  "at max",
			input: SplatF32(1.0),
			min:   0.0,
			max:   1.0,
			want:  SplatF32(1.0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.Clamp(tt.min, tt.max)
			if got != tt.want {
				t.Errorf("Clamp(%f, %f) = %v, want %v", tt.min, tt.max, got, tt.want)
			}
		})
	}
}

func TestF32x8_Lerp(t *testing.T) {
	tests := []struct {
		name string
		a    F32x8
		b    F32x8
		t    F32x8
		want F32x8
	}{
		{
			name: "t=0",
			a:    SplatF32(0.0),
			b:    SplatF32(10.0),
			t:    SplatF32(0.0),
			want: SplatF32(0.0),
		},
		{
			name: "t=1",
			a:    SplatF32(0.0),
			b:    SplatF32(10.0),
			t:    SplatF32(1.0),
			want: SplatF32(10.0),
		},
		{
			name: "t=0.5",
			a:    SplatF32(0.0),
			b:    SplatF32(10.0),
			t:    SplatF32(0.5),
			want: SplatF32(5.0),
		},
		{
			name: "t=0.25",
			a:    SplatF32(0.0),
			b:    SplatF32(100.0),
			t:    SplatF32(0.25),
			want: SplatF32(25.0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Lerp(tt.b, tt.t)
			if got != tt.want {
				t.Errorf("Lerp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestF32x8_Min(t *testing.T) {
	tests := []struct {
		name string
		a    F32x8
		b    F32x8
		want F32x8
	}{
		{
			name: "equal",
			a:    SplatF32(5.0),
			b:    SplatF32(5.0),
			want: SplatF32(5.0),
		},
		{
			name: "a smaller",
			a:    SplatF32(3.0),
			b:    SplatF32(7.0),
			want: SplatF32(3.0),
		},
		{
			name: "b smaller",
			a:    SplatF32(9.0),
			b:    SplatF32(2.0),
			want: SplatF32(2.0),
		},
		{
			name: "negative",
			a:    SplatF32(-5.0),
			b:    SplatF32(5.0),
			want: SplatF32(-5.0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Min(tt.b)
			if got != tt.want {
				t.Errorf("Min() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestF32x8_Max(t *testing.T) {
	tests := []struct {
		name string
		a    F32x8
		b    F32x8
		want F32x8
	}{
		{
			name: "equal",
			a:    SplatF32(5.0),
			b:    SplatF32(5.0),
			want: SplatF32(5.0),
		},
		{
			name: "a larger",
			a:    SplatF32(7.0),
			b:    SplatF32(3.0),
			want: SplatF32(7.0),
		},
		{
			name: "b larger",
			a:    SplatF32(2.0),
			b:    SplatF32(9.0),
			want: SplatF32(9.0),
		},
		{
			name: "negative",
			a:    SplatF32(-5.0),
			b:    SplatF32(5.0),
			want: SplatF32(5.0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Max(tt.b)
			if got != tt.want {
				t.Errorf("Max() = %v, want %v", got, tt.want)
			}
		})
	}
}
