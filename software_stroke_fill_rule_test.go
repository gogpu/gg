package gg

import (
	"bytes"
	"reflect"
	"testing"
)

func TestSoftwareStrokeIgnoresFillRule(t *testing.T) {
	for _, tc := range []struct {
		name  string
		width float64
		aa    bool
	}{
		{"normal", 4, true},
		{"hairline", 0.5, true},
		{"no_antialias", 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := NewPath()
			// The same segment twice: winding strokes stay visible, whereas
			// parity filling cancels the two expanded outlines completely.
			for range 2 {
				path.MoveTo(8, 24)
				path.LineTo(56, 24)
			}
			paint := NewPaint()
			paint.SetStroke(Stroke{Width: tc.width})
			paint.SetBrush(Solid(Red))
			render := func(rule FillRule) []byte {
				paint.FillRule = rule
				before := *paint
				pm := NewPixmap(64, 48)
				r := NewSoftwareRenderer(64, 48)
				r.SetAntiAlias(tc.aa)
				if err := r.Stroke(pm, path, paint); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(*paint, before) {
					t.Error("Stroke mutated the caller's paint")
				}
				return pm.Data()
			}
			want := render(FillRuleNonZero)
			if bytes.Equal(want, make([]byte, len(want))) {
				t.Fatal("reference stroke drew nothing")
			}
			if got := render(FillRuleEvenOdd); !bytes.Equal(got, want) {
				t.Error("stroke pixels depend on the caller's fill rule")
			}
		})
	}
}
