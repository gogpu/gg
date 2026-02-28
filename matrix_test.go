package gg

import (
	"math"
	"testing"
)

func TestIsTranslationOnly(t *testing.T) {
	tests := []struct {
		name string
		m    Matrix
		want bool
	}{
		{"identity", Identity(), true},
		{"pure translation", Translate(10, 20), true},
		{"zero translation", Translate(0, 0), true},
		{"negative translation", Translate(-5, -3), true},
		{"large translation", Translate(1e6, -1e6), true},
		{"uniform scale", Scale(2, 2), false},
		{"non-uniform scale", Scale(3, 0.5), false},
		{"scale 1,1 (identity via Scale)", Scale(1, 1), true},
		{"rotation 45deg", Rotate(math.Pi / 4), false},
		{"rotation 90deg", Rotate(math.Pi / 2), false},
		{"shear x", Shear(0.5, 0), false},
		{"shear y", Shear(0, 0.5), false},
		{"scale + translate", Scale(2, 3).Multiply(Translate(10, 20)), false},
		{"zero matrix", Matrix{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.m.IsTranslationOnly()
			if got != tt.want {
				t.Errorf("Matrix%+v.IsTranslationOnly() = %v, want %v", tt.m, got, tt.want)
			}
		})
	}
}

func TestIsScaleOnly(t *testing.T) {
	tests := []struct {
		name string
		m    Matrix
		want bool
	}{
		{"identity", Identity(), true},
		{"pure translation", Translate(10, 20), true},
		{"uniform scale", Scale(2, 2), true},
		{"non-uniform scale", Scale(3, 0.5), true},
		{"negative scale x", Scale(-1, 1), true},
		{"negative scale y", Scale(1, -1), true},
		{"negative scale both", Scale(-2, -3), true},
		{"zero scale x", Scale(0, 1), true},
		{"zero scale y", Scale(1, 0), true},
		{"zero scale both", Scale(0, 0), true},
		{"scale + translate", Scale(2, 3).Multiply(Translate(10, 20)), true},
		{"rotation 45deg", Rotate(math.Pi / 4), false},
		{"rotation 90deg", Rotate(math.Pi / 2), false},
		{"shear x", Shear(0.5, 0), false},
		{"shear y", Shear(0, 0.5), false},
		{"shear both", Shear(0.3, 0.7), false},
		{"scale then rotate", Scale(2, 2).Multiply(Rotate(math.Pi / 6)), false},
		{"zero matrix", Matrix{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.m.IsScaleOnly()
			if got != tt.want {
				t.Errorf("Matrix%+v.IsScaleOnly() = %v, want %v", tt.m, got, tt.want)
			}
		})
	}
}

func TestMaxScaleFactor(t *testing.T) {
	const epsilon = 1e-10

	tests := []struct {
		name string
		m    Matrix
		want float64
	}{
		{"identity", Identity(), 1.0},
		{"pure translation", Translate(10, 20), 1.0},
		{"uniform scale 2", Scale(2, 2), 2.0},
		{"uniform scale 0.5", Scale(0.5, 0.5), 0.5},
		{"non-uniform scale 3,1", Scale(3, 1), 3.0},
		{"non-uniform scale 1,4", Scale(1, 4), 4.0},
		{"non-uniform scale 2,5", Scale(2, 5), 5.0},
		{"negative scale -2,1", Scale(-2, 1), 2.0},
		{"negative scale 1,-3", Scale(1, -3), 3.0},
		{"negative scale -2,-3", Scale(-2, -3), 3.0},
		{"zero scale x", Scale(0, 1), 1.0},
		{"zero scale y", Scale(1, 0), 1.0},
		{"zero scale both", Scale(0, 0), 0.0},
		{"rotation 45deg", Rotate(math.Pi / 4), 1.0},
		{"rotation 90deg", Rotate(math.Pi / 2), 1.0},
		{"rotation 180deg", Rotate(math.Pi), 1.0},
		{"rotation arbitrary", Rotate(1.23), 1.0},
		{"scale 2 then rotate 45deg", Scale(2, 2).Multiply(Rotate(math.Pi / 4)), 2.0},
		{"scale 3,1 then rotate 45deg", Scale(3, 1).Multiply(Rotate(math.Pi / 4)), 3.0},
		{"scale 1,4 then rotate 30deg", Scale(1, 4).Multiply(Rotate(math.Pi / 6)), 4.0},
		{"shear x=1", Shear(1, 0), math.Sqrt((3 + math.Sqrt(5)) / 2)},
		{"scale + translate", Scale(3, 2).Multiply(Translate(100, 200)), 3.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.m.MaxScaleFactor()
			if math.Abs(got-tt.want) > epsilon {
				t.Errorf("Matrix%+v.MaxScaleFactor() = %v, want %v (diff=%e)",
					tt.m, got, tt.want, math.Abs(got-tt.want))
			}
		})
	}
}

func TestMaxScaleFactorShearManual(t *testing.T) {
	// Shear(1, 0) = [1 1; 0 1]
	// M^T * M = [1 1; 1 2]
	// eigenvalues: (3 +/- sqrt(5)) / 2
	// max eigenvalue = (3 + sqrt(5)) / 2
	// max singular value = sqrt((3 + sqrt(5)) / 2)
	m := Shear(1, 0)
	want := math.Sqrt((3 + math.Sqrt(5)) / 2)
	got := m.MaxScaleFactor()
	if math.Abs(got-want) > 1e-10 {
		t.Errorf("Shear(1,0).MaxScaleFactor() = %v, want %v", got, want)
	}
}

func TestMaxScaleFactorSkewComposition(t *testing.T) {
	// Scale(2, 3) then Shear(0.5, 0) = [2 1; 0 3]
	// M^T * M = [4 2; 2 10]
	// trace = 14, det = 36
	// eigenvalues: (14 +/- sqrt(196-144)) / 2 = (14 +/- sqrt(52)) / 2
	// max eigenvalue = (14 + sqrt(52)) / 2
	m := Scale(2, 3).Multiply(Shear(0.5, 0))
	p := m.A*m.A + m.D*m.D // 4
	r := m.B*m.B + m.E*m.E // 10
	q := m.A*m.B + m.D*m.E // 2
	sum := p + r
	diff := p - r
	disc := math.Sqrt(diff*diff + 4*q*q)
	want := math.Sqrt((sum + disc) / 2)

	got := m.MaxScaleFactor()
	if math.Abs(got-want) > 1e-10 {
		t.Errorf("Scale(2,3)*Shear(0.5,0).MaxScaleFactor() = %v, want %v", got, want)
	}
}

func TestMaxScaleFactorNearIdentity(t *testing.T) {
	// Near-identity: very small perturbation.
	m := Matrix{A: 1 + 1e-15, B: 1e-15, C: 0, D: -1e-15, E: 1 - 1e-15, F: 0}
	got := m.MaxScaleFactor()
	if math.Abs(got-1.0) > 1e-10 {
		t.Errorf("near-identity.MaxScaleFactor() = %v, want ~1.0", got)
	}
}

func TestIsTranslationOnlyConsistentWithIsTranslation(t *testing.T) {
	// IsTranslationOnly must be consistent with the existing IsTranslation.
	matrices := []Matrix{
		Identity(),
		Translate(5, 10),
		Scale(2, 3),
		Rotate(math.Pi / 3),
		Shear(0.5, 0.5),
		Scale(2, 2).Multiply(Translate(10, 20)),
		{},
	}
	for _, m := range matrices {
		if m.IsTranslationOnly() != m.IsTranslation() {
			t.Errorf("Matrix%+v: IsTranslationOnly()=%v != IsTranslation()=%v",
				m, m.IsTranslationOnly(), m.IsTranslation())
		}
	}
}

func TestMaxScaleFactorPreservesDirectionInvariance(t *testing.T) {
	// For a rotation matrix, MaxScaleFactor should be 1.0 regardless of angle.
	for deg := 0; deg < 360; deg += 15 {
		angle := float64(deg) * math.Pi / 180
		m := Rotate(angle)
		got := m.MaxScaleFactor()
		if math.Abs(got-1.0) > 1e-10 {
			t.Errorf("Rotate(%d deg).MaxScaleFactor() = %v, want 1.0", deg, got)
		}
	}
}

func TestMaxScaleFactorScaleRotateCommutes(t *testing.T) {
	// For uniform scale, MaxScaleFactor should be the same
	// regardless of rotation composition order.
	s := 3.5
	angle := math.Pi / 5

	m1 := Scale(s, s).Multiply(Rotate(angle))
	m2 := Rotate(angle).Multiply(Scale(s, s))

	f1 := m1.MaxScaleFactor()
	f2 := m2.MaxScaleFactor()

	if math.Abs(f1-s) > 1e-10 {
		t.Errorf("Scale*Rotate: MaxScaleFactor() = %v, want %v", f1, s)
	}
	if math.Abs(f2-s) > 1e-10 {
		t.Errorf("Rotate*Scale: MaxScaleFactor() = %v, want %v", f2, s)
	}
}
