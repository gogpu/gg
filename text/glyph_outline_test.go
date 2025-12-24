package text

import (
	"testing"
)

func TestOutlineOp_String(t *testing.T) {
	tests := []struct {
		op   OutlineOp
		want string
	}{
		{OutlineOpMoveTo, "MoveTo"},
		{OutlineOpLineTo, "LineTo"},
		{OutlineOpQuadTo, "QuadTo"},
		{OutlineOpCubicTo, "CubicTo"},
		{OutlineOp(255), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.op.String(); got != tt.want {
				t.Errorf("OutlineOp.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGlyphOutline_IsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		outline *GlyphOutline
		want    bool
	}{
		{
			name:    "nil segments",
			outline: &GlyphOutline{Segments: nil},
			want:    true,
		},
		{
			name:    "empty segments",
			outline: &GlyphOutline{Segments: []OutlineSegment{}},
			want:    true,
		},
		{
			name: "has segments",
			outline: &GlyphOutline{
				Segments: []OutlineSegment{
					{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.outline.IsEmpty(); got != tt.want {
				t.Errorf("GlyphOutline.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGlyphOutline_SegmentCount(t *testing.T) {
	tests := []struct {
		name    string
		outline *GlyphOutline
		want    int
	}{
		{
			name:    "nil segments",
			outline: &GlyphOutline{Segments: nil},
			want:    0,
		},
		{
			name: "two segments",
			outline: &GlyphOutline{
				Segments: []OutlineSegment{
					{Op: OutlineOpMoveTo},
					{Op: OutlineOpLineTo},
				},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.outline.SegmentCount(); got != tt.want {
				t.Errorf("GlyphOutline.SegmentCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGlyphOutline_Clone(t *testing.T) {
	t.Run("nil outline", func(t *testing.T) {
		var o *GlyphOutline
		clone := o.Clone()
		if clone != nil {
			t.Errorf("Clone of nil should be nil")
		}
	})

	t.Run("normal outline", func(t *testing.T) {
		o := &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 10, Y: 20}}},
				{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 30, Y: 40}}},
			},
			Bounds:  Rect{MinX: 10, MinY: 20, MaxX: 30, MaxY: 40},
			Advance: 50,
			LSB:     5,
			GID:     42,
			Type:    GlyphTypeOutline,
		}

		clone := o.Clone()

		// Check clone is not same reference
		if clone == o {
			t.Errorf("Clone should not be same reference")
		}

		// Check values match
		if clone.Advance != o.Advance {
			t.Errorf("Advance mismatch: got %v, want %v", clone.Advance, o.Advance)
		}
		if clone.GID != o.GID {
			t.Errorf("GID mismatch: got %v, want %v", clone.GID, o.GID)
		}
		if clone.Type != o.Type {
			t.Errorf("Type mismatch: got %v, want %v", clone.Type, o.Type)
		}
		if len(clone.Segments) != len(o.Segments) {
			t.Errorf("Segments length mismatch: got %v, want %v", len(clone.Segments), len(o.Segments))
		}

		// Modify original, clone should be unaffected
		o.Segments[0].Points[0].X = 999
		if clone.Segments[0].Points[0].X == 999 {
			t.Errorf("Clone should be independent of original")
		}
	})
}

func TestGlyphOutline_Scale(t *testing.T) {
	t.Run("nil outline", func(t *testing.T) {
		var o *GlyphOutline
		scaled := o.Scale(2.0)
		if scaled != nil {
			t.Errorf("Scale of nil should be nil")
		}
	})

	t.Run("scale by 2", func(t *testing.T) {
		o := &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 10, Y: 20}}},
				{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 30, Y: 40}}},
			},
			Bounds:  Rect{MinX: 10, MinY: 20, MaxX: 30, MaxY: 40},
			Advance: 50,
			LSB:     5,
		}

		scaled := o.Scale(2.0)

		// Check scaled values
		if scaled.Segments[0].Points[0].X != 20 {
			t.Errorf("First segment X should be 20, got %v", scaled.Segments[0].Points[0].X)
		}
		if scaled.Segments[0].Points[0].Y != 40 {
			t.Errorf("First segment Y should be 40, got %v", scaled.Segments[0].Points[0].Y)
		}
		if scaled.Advance != 100 {
			t.Errorf("Advance should be 100, got %v", scaled.Advance)
		}
		if scaled.Bounds.MinX != 20 {
			t.Errorf("Bounds.MinX should be 20, got %v", scaled.Bounds.MinX)
		}
	})
}

func TestGlyphOutline_Translate(t *testing.T) {
	t.Run("nil outline", func(t *testing.T) {
		var o *GlyphOutline
		translated := o.Translate(10, 20)
		if translated != nil {
			t.Errorf("Translate of nil should be nil")
		}
	})

	t.Run("translate by 10,20", func(t *testing.T) {
		o := &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 10, Y: 20}}},
			},
			Bounds: Rect{MinX: 10, MinY: 20, MaxX: 30, MaxY: 40},
		}

		translated := o.Translate(10, 20)

		// Check translated values
		if translated.Segments[0].Points[0].X != 20 {
			t.Errorf("First segment X should be 20, got %v", translated.Segments[0].Points[0].X)
		}
		if translated.Segments[0].Points[0].Y != 40 {
			t.Errorf("First segment Y should be 40, got %v", translated.Segments[0].Points[0].Y)
		}
		if translated.Bounds.MinX != 20 {
			t.Errorf("Bounds.MinX should be 20, got %v", translated.Bounds.MinX)
		}
	})
}

func TestAffineTransform(t *testing.T) {
	t.Run("identity", func(t *testing.T) {
		m := IdentityTransform()
		x, y := m.TransformPoint(10, 20)
		if x != 10 || y != 20 {
			t.Errorf("Identity transform should not change point, got (%v, %v)", x, y)
		}
	})

	t.Run("scale", func(t *testing.T) {
		m := ScaleTransform(2, 3)
		x, y := m.TransformPoint(10, 20)
		if x != 20 || y != 60 {
			t.Errorf("Scale transform expected (20, 60), got (%v, %v)", x, y)
		}
	})

	t.Run("translate", func(t *testing.T) {
		m := TranslateTransform(5, 10)
		x, y := m.TransformPoint(10, 20)
		if x != 15 || y != 30 {
			t.Errorf("Translate transform expected (15, 30), got (%v, %v)", x, y)
		}
	})

	t.Run("multiply", func(t *testing.T) {
		scale := ScaleTransform(2, 2)
		translate := TranslateTransform(5, 5)
		combined := scale.Multiply(translate)

		// Multiply applies left transform first (scale), then right transform (translate)
		// So: (10, 10) * 2 = (20, 20), then + (5*2, 5*2) = (30, 30)
		// Actually, the matrix multiplication formula is: M1 * M2 * point
		// which means: apply M2 first (translate: add 5,5), then M1 (scale: multiply)
		// (10, 10) + (5, 5) = (15, 15) * 2 = (30, 30)
		x, y := combined.TransformPoint(10, 10)
		if x != 30 || y != 30 {
			t.Errorf("Combined transform expected (30, 30), got (%v, %v)", x, y)
		}
	})
}

func TestGlyphOutline_Transform(t *testing.T) {
	t.Run("nil outline", func(t *testing.T) {
		var o *GlyphOutline
		transformed := o.Transform(IdentityTransform())
		if transformed != nil {
			t.Errorf("Transform of nil should be nil")
		}
	})

	t.Run("nil transform", func(t *testing.T) {
		o := &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 10, Y: 20}}},
			},
		}
		transformed := o.Transform(nil)
		if transformed.Segments[0].Points[0].X != 10 {
			t.Errorf("Nil transform should return clone")
		}
	})

	t.Run("scale transform", func(t *testing.T) {
		o := &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 10, Y: 20}}},
				{Op: OutlineOpQuadTo, Points: [3]OutlinePoint{{X: 15, Y: 25}, {X: 20, Y: 30}}},
				{Op: OutlineOpCubicTo, Points: [3]OutlinePoint{{X: 25, Y: 35}, {X: 30, Y: 40}, {X: 35, Y: 45}}},
			},
		}

		transformed := o.Transform(ScaleTransform(2, 2))

		// Check MoveTo
		if transformed.Segments[0].Points[0].X != 20 || transformed.Segments[0].Points[0].Y != 40 {
			t.Errorf("MoveTo point should be (20, 40), got (%v, %v)",
				transformed.Segments[0].Points[0].X, transformed.Segments[0].Points[0].Y)
		}

		// Check QuadTo
		if transformed.Segments[1].Points[1].X != 40 || transformed.Segments[1].Points[1].Y != 60 {
			t.Errorf("QuadTo target should be (40, 60), got (%v, %v)",
				transformed.Segments[1].Points[1].X, transformed.Segments[1].Points[1].Y)
		}

		// Check CubicTo
		if transformed.Segments[2].Points[2].X != 70 || transformed.Segments[2].Points[2].Y != 90 {
			t.Errorf("CubicTo target should be (70, 90), got (%v, %v)",
				transformed.Segments[2].Points[2].X, transformed.Segments[2].Points[2].Y)
		}
	})
}

func TestOutlineExtractor_New(t *testing.T) {
	e := NewOutlineExtractor()
	if e == nil {
		t.Errorf("NewOutlineExtractor should not return nil")
	}
}

func TestFontError(t *testing.T) {
	err := &FontError{Reason: "test error"}
	expected := "text: test error"
	if err.Error() != expected {
		t.Errorf("FontError.Error() = %v, want %v", err.Error(), expected)
	}
}

func TestErrUnsupportedFontType(t *testing.T) {
	if ErrUnsupportedFontType == nil {
		t.Errorf("ErrUnsupportedFontType should not be nil")
	}

	expected := "text: unsupported font type for outline extraction"
	if ErrUnsupportedFontType.Error() != expected {
		t.Errorf("ErrUnsupportedFontType.Error() = %v, want %v", ErrUnsupportedFontType.Error(), expected)
	}
}

// BenchmarkOutlineClone benchmarks outline cloning.
func BenchmarkOutlineClone(b *testing.B) {
	outline := &GlyphOutline{
		Segments: make([]OutlineSegment, 100),
		Bounds:   Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
		Advance:  50,
		GID:      42,
	}
	for i := range outline.Segments {
		outline.Segments[i] = OutlineSegment{
			Op: OutlineOpCubicTo,
			Points: [3]OutlinePoint{
				{X: float32(i), Y: float32(i)},
				{X: float32(i + 1), Y: float32(i + 1)},
				{X: float32(i + 2), Y: float32(i + 2)},
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = outline.Clone()
	}
}

// BenchmarkOutlineScale benchmarks outline scaling.
func BenchmarkOutlineScale(b *testing.B) {
	outline := &GlyphOutline{
		Segments: make([]OutlineSegment, 100),
		Bounds:   Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
		Advance:  50,
		GID:      42,
	}
	for i := range outline.Segments {
		outline.Segments[i] = OutlineSegment{
			Op:     OutlineOpLineTo,
			Points: [3]OutlinePoint{{X: float32(i), Y: float32(i)}},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = outline.Scale(2.0)
	}
}

// BenchmarkOutlineTransform benchmarks outline transformation.
func BenchmarkOutlineTransform(b *testing.B) {
	outline := &GlyphOutline{
		Segments: make([]OutlineSegment, 100),
		Bounds:   Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
		Advance:  50,
		GID:      42,
	}
	for i := range outline.Segments {
		outline.Segments[i] = OutlineSegment{
			Op:     OutlineOpLineTo,
			Points: [3]OutlinePoint{{X: float32(i), Y: float32(i)}},
		}
	}
	transform := ScaleTransform(2.0, 2.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = outline.Transform(transform)
	}
}
