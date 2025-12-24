package gg

import (
	"testing"
)

func TestDefaultStroke(t *testing.T) {
	s := DefaultStroke()

	if s.Width != 1.0 {
		t.Errorf("DefaultStroke().Width = %v, want 1.0", s.Width)
	}
	if s.Cap != LineCapButt {
		t.Errorf("DefaultStroke().Cap = %v, want LineCapButt", s.Cap)
	}
	if s.Join != LineJoinMiter {
		t.Errorf("DefaultStroke().Join = %v, want LineJoinMiter", s.Join)
	}
	if s.MiterLimit != 4.0 {
		t.Errorf("DefaultStroke().MiterLimit = %v, want 4.0", s.MiterLimit)
	}
	if s.Dash != nil {
		t.Errorf("DefaultStroke().Dash = %v, want nil", s.Dash)
	}
}

func TestStroke_WithWidth(t *testing.T) {
	tests := []struct {
		name  string
		width float64
	}{
		{"thin", 0.5},
		{"normal", 1.0},
		{"thick", 5.0},
		{"zero", 0.0},
		{"negative", -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultStroke().WithWidth(tt.width)
			if s.Width != tt.width {
				t.Errorf("WithWidth(%v).Width = %v", tt.width, s.Width)
			}
		})
	}
}

func TestStroke_WithCap(t *testing.T) {
	tests := []struct {
		name string
		cap  LineCap
	}{
		{"butt", LineCapButt},
		{"round", LineCapRound},
		{"square", LineCapSquare},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultStroke().WithCap(tt.cap)
			if s.Cap != tt.cap {
				t.Errorf("WithCap(%v).Cap = %v", tt.cap, s.Cap)
			}
		})
	}
}

func TestStroke_WithJoin(t *testing.T) {
	tests := []struct {
		name string
		join LineJoin
	}{
		{"miter", LineJoinMiter},
		{"round", LineJoinRound},
		{"bevel", LineJoinBevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultStroke().WithJoin(tt.join)
			if s.Join != tt.join {
				t.Errorf("WithJoin(%v).Join = %v", tt.join, s.Join)
			}
		})
	}
}

func TestStroke_WithMiterLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit float64
	}{
		{"one", 1.0},
		{"default", 4.0},
		{"high", 10.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultStroke().WithMiterLimit(tt.limit)
			if s.MiterLimit != tt.limit {
				t.Errorf("WithMiterLimit(%v).MiterLimit = %v", tt.limit, s.MiterLimit)
			}
		})
	}
}

func TestStroke_WithDash(t *testing.T) {
	t.Run("with nil dash", func(t *testing.T) {
		s := DefaultStroke().WithDash(nil)
		if s.Dash != nil {
			t.Errorf("WithDash(nil).Dash = %v, want nil", s.Dash)
		}
	})

	t.Run("with valid dash", func(t *testing.T) {
		dash := NewDash(5, 3)
		s := DefaultStroke().WithDash(dash)
		if s.Dash == nil {
			t.Fatal("WithDash(dash).Dash = nil")
		}
		if s.Dash == dash {
			t.Error("WithDash should clone the dash")
		}
		if len(s.Dash.Array) != 2 {
			t.Errorf("WithDash(dash).Dash.Array length = %d, want 2", len(s.Dash.Array))
		}
	})

	t.Run("clears dash with nil", func(t *testing.T) {
		s := DefaultStroke().WithDashPattern(5, 3).WithDash(nil)
		if s.Dash != nil {
			t.Errorf("WithDash(nil) should clear dash")
		}
	})
}

func TestStroke_WithDashPattern(t *testing.T) {
	tests := []struct {
		name      string
		lengths   []float64
		wantNil   bool
		wantArray []float64
	}{
		{
			name:    "empty pattern",
			lengths: []float64{},
			wantNil: true,
		},
		{
			name:      "simple pattern",
			lengths:   []float64{5, 3},
			wantNil:   false,
			wantArray: []float64{5, 3},
		},
		{
			name:      "single value",
			lengths:   []float64{5},
			wantNil:   false,
			wantArray: []float64{5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultStroke().WithDashPattern(tt.lengths...)
			if tt.wantNil {
				if s.Dash != nil {
					t.Errorf("WithDashPattern().Dash = %v, want nil", s.Dash)
				}
				return
			}
			if s.Dash == nil {
				t.Fatal("WithDashPattern().Dash = nil")
			}
			if len(s.Dash.Array) != len(tt.wantArray) {
				t.Errorf("Dash.Array length = %d, want %d", len(s.Dash.Array), len(tt.wantArray))
			}
		})
	}
}

func TestStroke_WithDashOffset(t *testing.T) {
	t.Run("with dash set", func(t *testing.T) {
		s := DefaultStroke().WithDashPattern(5, 3).WithDashOffset(2)
		if s.Dash == nil {
			t.Fatal("Dash = nil")
		}
		if s.Dash.Offset != 2 {
			t.Errorf("Dash.Offset = %v, want 2", s.Dash.Offset)
		}
	})

	t.Run("without dash set", func(t *testing.T) {
		s := DefaultStroke().WithDashOffset(2)
		// Should have no effect since no dash is set
		if s.Dash != nil {
			t.Errorf("Dash = %v, want nil", s.Dash)
		}
	})
}

func TestStroke_IsDashed(t *testing.T) {
	tests := []struct {
		name   string
		stroke Stroke
		want   bool
	}{
		{
			name:   "default stroke",
			stroke: DefaultStroke(),
			want:   false,
		},
		{
			name:   "with dash",
			stroke: DefaultStroke().WithDashPattern(5, 3),
			want:   true,
		},
		{
			name:   "with nil dash",
			stroke: DefaultStroke().WithDash(nil),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.stroke.IsDashed()
			if got != tt.want {
				t.Errorf("IsDashed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStroke_Clone(t *testing.T) {
	t.Run("clones simple stroke", func(t *testing.T) {
		original := DefaultStroke().WithWidth(3).WithCap(LineCapRound)
		clone := original.Clone()

		if clone.Width != original.Width {
			t.Errorf("Clone().Width = %v, want %v", clone.Width, original.Width)
		}
		if clone.Cap != original.Cap {
			t.Errorf("Clone().Cap = %v, want %v", clone.Cap, original.Cap)
		}
	})

	t.Run("clones stroke with dash", func(t *testing.T) {
		original := DefaultStroke().WithDashPattern(5, 3).WithDashOffset(2)
		clone := original.Clone()

		if clone.Dash == nil {
			t.Fatal("Clone().Dash = nil")
		}
		if clone.Dash == original.Dash {
			t.Error("Clone() shares Dash pointer")
		}
		if clone.Dash.Offset != original.Dash.Offset {
			t.Errorf("Clone().Dash.Offset = %v, want %v", clone.Dash.Offset, original.Dash.Offset)
		}
	})

	t.Run("modifying clone does not affect original", func(t *testing.T) {
		original := DefaultStroke().WithDashPattern(5, 3)
		clone := original.Clone()

		clone.Width = 100
		clone.Dash.Array[0] = 999

		if original.Width == 100 {
			t.Error("modifying clone.Width affected original")
		}
		if original.Dash.Array[0] == 999 {
			t.Error("modifying clone.Dash.Array affected original")
		}
	})
}

func TestStroke_FluentChaining(t *testing.T) {
	s := DefaultStroke().
		WithWidth(2).
		WithCap(LineCapRound).
		WithJoin(LineJoinRound).
		WithMiterLimit(10).
		WithDashPattern(10, 5, 2, 5).
		WithDashOffset(3)

	if s.Width != 2 {
		t.Errorf("Width = %v, want 2", s.Width)
	}
	if s.Cap != LineCapRound {
		t.Errorf("Cap = %v, want LineCapRound", s.Cap)
	}
	if s.Join != LineJoinRound {
		t.Errorf("Join = %v, want LineJoinRound", s.Join)
	}
	if s.MiterLimit != 10 {
		t.Errorf("MiterLimit = %v, want 10", s.MiterLimit)
	}
	if s.Dash == nil {
		t.Fatal("Dash = nil")
	}
	if len(s.Dash.Array) != 4 {
		t.Errorf("Dash.Array length = %d, want 4", len(s.Dash.Array))
	}
	if s.Dash.Offset != 3 {
		t.Errorf("Dash.Offset = %v, want 3", s.Dash.Offset)
	}
}

func TestPresetStrokes(t *testing.T) {
	t.Run("Thin", func(t *testing.T) {
		s := Thin()
		if s.Width != 0.5 {
			t.Errorf("Thin().Width = %v, want 0.5", s.Width)
		}
	})

	t.Run("Thick", func(t *testing.T) {
		s := Thick()
		if s.Width != 3.0 {
			t.Errorf("Thick().Width = %v, want 3.0", s.Width)
		}
	})

	t.Run("Bold", func(t *testing.T) {
		s := Bold()
		if s.Width != 5.0 {
			t.Errorf("Bold().Width = %v, want 5.0", s.Width)
		}
	})

	t.Run("RoundStroke", func(t *testing.T) {
		s := RoundStroke()
		if s.Cap != LineCapRound {
			t.Errorf("RoundStroke().Cap = %v, want LineCapRound", s.Cap)
		}
		if s.Join != LineJoinRound {
			t.Errorf("RoundStroke().Join = %v, want LineJoinRound", s.Join)
		}
	})

	t.Run("SquareStroke", func(t *testing.T) {
		s := SquareStroke()
		if s.Cap != LineCapSquare {
			t.Errorf("SquareStroke().Cap = %v, want LineCapSquare", s.Cap)
		}
	})

	t.Run("DashedStroke", func(t *testing.T) {
		s := DashedStroke(5, 3)
		if !s.IsDashed() {
			t.Error("DashedStroke().IsDashed() = false")
		}
		if len(s.Dash.Array) != 2 {
			t.Errorf("DashedStroke().Dash.Array length = %d, want 2", len(s.Dash.Array))
		}
	})

	t.Run("DottedStroke", func(t *testing.T) {
		s := DottedStroke()
		if s.Width != 2.0 {
			t.Errorf("DottedStroke().Width = %v, want 2.0", s.Width)
		}
		if s.Cap != LineCapRound {
			t.Errorf("DottedStroke().Cap = %v, want LineCapRound", s.Cap)
		}
		if !s.IsDashed() {
			t.Error("DottedStroke().IsDashed() = false")
		}
	})
}

func TestStroke_ValueSemantics(t *testing.T) {
	// Stroke uses value receivers and returns copies
	// Verify that modifications to one instance don't affect another

	t.Run("WithWidth returns copy", func(t *testing.T) {
		s1 := DefaultStroke()
		s2 := s1.WithWidth(10)

		if s1.Width == s2.Width {
			t.Error("WithWidth modified original")
		}
	})

	t.Run("chained calls preserve independence", func(t *testing.T) {
		base := DefaultStroke()
		thin := base.WithWidth(0.5)
		thick := base.WithWidth(5.0)

		if base.Width != 1.0 {
			t.Errorf("base.Width = %v, want 1.0", base.Width)
		}
		if thin.Width != 0.5 {
			t.Errorf("thin.Width = %v, want 0.5", thin.Width)
		}
		if thick.Width != 5.0 {
			t.Errorf("thick.Width = %v, want 5.0", thick.Width)
		}
	})
}
