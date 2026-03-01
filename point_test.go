package gg

import (
	"math"
	"testing"
)

const pointEpsilon = 1e-10

func pointsNear(a, b Point, eps float64) bool {
	return math.Abs(a.X-b.X) < eps && math.Abs(a.Y-b.Y) < eps
}

func TestPt(t *testing.T) {
	p := Pt(3.5, -2.1)
	if p.X != 3.5 || p.Y != -2.1 {
		t.Errorf("Pt(3.5, -2.1) = %v, want {3.5, -2.1}", p)
	}
}

func TestPoint_Add(t *testing.T) {
	tests := []struct {
		name string
		p, q Point
		want Point
	}{
		{"positive", Pt(1, 2), Pt(3, 4), Pt(4, 6)},
		{"negative", Pt(-1, -2), Pt(-3, -4), Pt(-4, -6)},
		{"mixed", Pt(1, -2), Pt(-3, 4), Pt(-2, 2)},
		{"zero + zero", Pt(0, 0), Pt(0, 0), Pt(0, 0)},
		{"zero + value", Pt(0, 0), Pt(5, 7), Pt(5, 7)},
		{"fractional", Pt(0.1, 0.2), Pt(0.3, 0.4), Pt(0.4, 0.6)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Add(tt.q)
			if !pointsNear(got, tt.want, pointEpsilon) {
				t.Errorf("(%v).Add(%v) = %v, want %v", tt.p, tt.q, got, tt.want)
			}
		})
	}
}

func TestPoint_Sub(t *testing.T) {
	tests := []struct {
		name string
		p, q Point
		want Point
	}{
		{"positive", Pt(4, 6), Pt(1, 2), Pt(3, 4)},
		{"negative result", Pt(1, 2), Pt(3, 4), Pt(-2, -2)},
		{"same point", Pt(5, 5), Pt(5, 5), Pt(0, 0)},
		{"zero", Pt(0, 0), Pt(0, 0), Pt(0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Sub(tt.q)
			if !pointsNear(got, tt.want, pointEpsilon) {
				t.Errorf("(%v).Sub(%v) = %v, want %v", tt.p, tt.q, got, tt.want)
			}
		})
	}
}

func TestPoint_Mul(t *testing.T) {
	tests := []struct {
		name string
		p    Point
		s    float64
		want Point
	}{
		{"positive scalar", Pt(2, 3), 4, Pt(8, 12)},
		{"zero scalar", Pt(5, 7), 0, Pt(0, 0)},
		{"negative scalar", Pt(2, 3), -1, Pt(-2, -3)},
		{"fractional scalar", Pt(10, 20), 0.5, Pt(5, 10)},
		{"one scalar", Pt(3, 4), 1, Pt(3, 4)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Mul(tt.s)
			if !pointsNear(got, tt.want, pointEpsilon) {
				t.Errorf("(%v).Mul(%v) = %v, want %v", tt.p, tt.s, got, tt.want)
			}
		})
	}
}

func TestPoint_Div(t *testing.T) {
	tests := []struct {
		name string
		p    Point
		s    float64
		want Point
	}{
		{"positive divisor", Pt(8, 12), 4, Pt(2, 3)},
		{"fractional divisor", Pt(5, 10), 0.5, Pt(10, 20)},
		{"negative divisor", Pt(6, 9), -3, Pt(-2, -3)},
		{"one divisor", Pt(3, 4), 1, Pt(3, 4)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Div(tt.s)
			if !pointsNear(got, tt.want, pointEpsilon) {
				t.Errorf("(%v).Div(%v) = %v, want %v", tt.p, tt.s, got, tt.want)
			}
		})
	}
}

func TestPoint_Div_ByZero(t *testing.T) {
	got := Pt(1, 2).Div(0)
	if !math.IsInf(got.X, 1) || !math.IsInf(got.Y, 1) {
		t.Errorf("Div(0) = %v, want (+Inf, +Inf)", got)
	}
}

func TestPoint_Dot(t *testing.T) {
	tests := []struct {
		name string
		p, q Point
		want float64
	}{
		{"perpendicular", Pt(1, 0), Pt(0, 1), 0},
		{"parallel same", Pt(1, 0), Pt(2, 0), 2},
		{"parallel opposite", Pt(1, 0), Pt(-1, 0), -1},
		{"general", Pt(2, 3), Pt(4, 5), 23},
		{"zero vectors", Pt(0, 0), Pt(0, 0), 0},
		{"unit at 45", Pt(1, 1), Pt(1, 1), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Dot(tt.q)
			if math.Abs(got-tt.want) > pointEpsilon {
				t.Errorf("(%v).Dot(%v) = %v, want %v", tt.p, tt.q, got, tt.want)
			}
		})
	}
}

func TestPoint_Cross(t *testing.T) {
	tests := []struct {
		name string
		p, q Point
		want float64
	}{
		{"perpendicular", Pt(1, 0), Pt(0, 1), 1},
		{"perpendicular reverse", Pt(0, 1), Pt(1, 0), -1},
		{"parallel", Pt(2, 0), Pt(3, 0), 0},
		{"general", Pt(2, 3), Pt(4, 5), -2}, // 2*5 - 3*4 = 10 - 12 = -2
		{"zero vectors", Pt(0, 0), Pt(0, 0), 0},
		{"same vector", Pt(3, 4), Pt(3, 4), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Cross(tt.q)
			if math.Abs(got-tt.want) > pointEpsilon {
				t.Errorf("(%v).Cross(%v) = %v, want %v", tt.p, tt.q, got, tt.want)
			}
		})
	}
}

func TestPoint_Length(t *testing.T) {
	tests := []struct {
		name string
		p    Point
		want float64
	}{
		{"unit x", Pt(1, 0), 1},
		{"unit y", Pt(0, 1), 1},
		{"3-4-5", Pt(3, 4), 5},
		{"zero", Pt(0, 0), 0},
		{"negative", Pt(-3, -4), 5},
		{"unit diagonal", Pt(1, 1), math.Sqrt(2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Length()
			if math.Abs(got-tt.want) > pointEpsilon {
				t.Errorf("(%v).Length() = %v, want %v", tt.p, got, tt.want)
			}
		})
	}
}

func TestPoint_LengthSquared(t *testing.T) {
	tests := []struct {
		name string
		p    Point
		want float64
	}{
		{"3-4-5", Pt(3, 4), 25},
		{"zero", Pt(0, 0), 0},
		{"unit", Pt(1, 0), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.LengthSquared()
			if math.Abs(got-tt.want) > pointEpsilon {
				t.Errorf("(%v).LengthSquared() = %v, want %v", tt.p, got, tt.want)
			}
		})
	}
}

func TestPoint_Distance(t *testing.T) {
	tests := []struct {
		name string
		p, q Point
		want float64
	}{
		{"same point", Pt(1, 1), Pt(1, 1), 0},
		{"horizontal", Pt(0, 0), Pt(10, 0), 10},
		{"vertical", Pt(0, 0), Pt(0, 7), 7},
		{"3-4-5", Pt(0, 0), Pt(3, 4), 5},
		{"symmetric", Pt(1, 2), Pt(4, 6), 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Distance(tt.q)
			if math.Abs(got-tt.want) > pointEpsilon {
				t.Errorf("(%v).Distance(%v) = %v, want %v", tt.p, tt.q, got, tt.want)
			}
		})
	}
}

func TestPoint_Distance_Symmetry(t *testing.T) {
	p := Pt(1, 2)
	q := Pt(4, 6)
	d1 := p.Distance(q)
	d2 := q.Distance(p)
	if math.Abs(d1-d2) > pointEpsilon {
		t.Errorf("Distance not symmetric: %v vs %v", d1, d2)
	}
}

func TestPoint_Normalize(t *testing.T) {
	tests := []struct {
		name       string
		p          Point
		wantLength float64
	}{
		{"unit x", Pt(5, 0), 1},
		{"unit y", Pt(0, 3), 1},
		{"diagonal", Pt(3, 4), 1},
		{"negative", Pt(-3, -4), 1},
		{"already unit", Pt(1, 0), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Normalize()
			gotLen := got.Length()
			if math.Abs(gotLen-tt.wantLength) > pointEpsilon {
				t.Errorf("(%v).Normalize().Length() = %v, want %v", tt.p, gotLen, tt.wantLength)
			}
		})
	}
}

func TestPoint_Normalize_Zero(t *testing.T) {
	got := Pt(0, 0).Normalize()
	if got.X != 0 || got.Y != 0 {
		t.Errorf("Pt(0,0).Normalize() = %v, want (0, 0)", got)
	}
}

func TestPoint_Normalize_Direction(t *testing.T) {
	// Normalizing (3, 4) should point in same direction: (0.6, 0.8)
	got := Pt(3, 4).Normalize()
	want := Pt(0.6, 0.8)
	if !pointsNear(got, want, pointEpsilon) {
		t.Errorf("Pt(3,4).Normalize() = %v, want %v", got, want)
	}
}

func TestPoint_Rotate(t *testing.T) {
	tests := []struct {
		name  string
		p     Point
		angle float64
		want  Point
	}{
		{"0 degrees", Pt(1, 0), 0, Pt(1, 0)},
		{"90 degrees", Pt(1, 0), math.Pi / 2, Pt(0, 1)},
		{"180 degrees", Pt(1, 0), math.Pi, Pt(-1, 0)},
		{"270 degrees", Pt(1, 0), 3 * math.Pi / 2, Pt(0, -1)},
		{"360 degrees", Pt(1, 0), 2 * math.Pi, Pt(1, 0)},
		{"-90 degrees", Pt(1, 0), -math.Pi / 2, Pt(0, -1)},
		{"45 degrees", Pt(1, 0), math.Pi / 4, Pt(math.Sqrt(2)/2, math.Sqrt(2)/2)},
		{"zero vector", Pt(0, 0), math.Pi, Pt(0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Rotate(tt.angle)
			if !pointsNear(got, tt.want, 1e-9) {
				t.Errorf("(%v).Rotate(%v) = %v, want %v", tt.p, tt.angle, got, tt.want)
			}
		})
	}
}

func TestPoint_Rotate_PreservesLength(t *testing.T) {
	p := Pt(3, 4)
	originalLen := p.Length()
	angles := []float64{0.1, 0.5, 1.0, math.Pi, 2 * math.Pi, -0.7}
	for _, angle := range angles {
		rotated := p.Rotate(angle)
		rotatedLen := rotated.Length()
		if math.Abs(rotatedLen-originalLen) > pointEpsilon {
			t.Errorf("Rotate(%v) changed length from %v to %v", angle, originalLen, rotatedLen)
		}
	}
}

func TestPoint_Lerp(t *testing.T) {
	tests := []struct {
		name string
		p, q Point
		t    float64
		want Point
	}{
		{"t=0 returns p", Pt(0, 0), Pt(10, 10), 0, Pt(0, 0)},
		{"t=1 returns q", Pt(0, 0), Pt(10, 10), 1, Pt(10, 10)},
		{"t=0.5 midpoint", Pt(0, 0), Pt(10, 10), 0.5, Pt(5, 5)},
		{"t=0.25 quarter", Pt(0, 0), Pt(8, 4), 0.25, Pt(2, 1)},
		{"same point", Pt(5, 5), Pt(5, 5), 0.5, Pt(5, 5)},
		{"extrapolate t=2", Pt(0, 0), Pt(10, 10), 2, Pt(20, 20)},
		{"extrapolate t=-1", Pt(0, 0), Pt(10, 10), -1, Pt(-10, -10)},
		{"negative coords", Pt(-5, -5), Pt(5, 5), 0.5, Pt(0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Lerp(tt.q, tt.t)
			if !pointsNear(got, tt.want, pointEpsilon) {
				t.Errorf("(%v).Lerp(%v, %v) = %v, want %v", tt.p, tt.q, tt.t, got, tt.want)
			}
		})
	}
}

func TestPoint_NaN(t *testing.T) {
	nan := math.NaN()
	p := Pt(nan, nan)

	// NaN operations should propagate NaN
	sum := p.Add(Pt(1, 1))
	if !math.IsNaN(sum.X) || !math.IsNaN(sum.Y) {
		t.Errorf("NaN + (1,1) should be NaN, got %v", sum)
	}

	length := p.Length()
	if !math.IsNaN(length) {
		t.Errorf("NaN point length should be NaN, got %v", length)
	}

	dot := p.Dot(Pt(1, 0))
	if !math.IsNaN(dot) {
		t.Errorf("NaN dot should be NaN, got %v", dot)
	}
}

func TestPoint_Inf(t *testing.T) {
	inf := math.Inf(1)
	p := Pt(inf, inf)

	length := p.Length()
	if !math.IsInf(length, 1) {
		t.Errorf("Inf point length should be +Inf, got %v", length)
	}

	// Normalize of infinity should produce NaN (inf/inf)
	norm := p.Normalize()
	if !math.IsNaN(norm.X) {
		t.Errorf("Normalize(Inf) should produce NaN, got %v", norm)
	}
}
