package svg

import (
	"math"
	"strings"
	"testing"

	"github.com/gogpu/gg"
)

func TestTransformMatrixSupportedForms(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  gg.Matrix
	}{
		{name: "empty", input: "", want: gg.Identity()},
		{name: "translate x only", input: "translate(3)", want: gg.Translate(3, 0)},
		{name: "translate x y", input: "translate(3 4)", want: gg.Translate(3, 4)},
		{name: "rotate origin", input: "rotate(90)", want: gg.Rotate(math.Pi / 2)},
		{
			name:  "rotate about point",
			input: "rotate(90 2 3)",
			want: gg.Translate(2, 3).Multiply(gg.Rotate(math.Pi / 2)).
				Multiply(gg.Translate(-2, -3)),
		},
		{name: "uniform scale", input: "scale(2)", want: gg.Scale(2, 2)},
		{name: "nonuniform scale", input: "scale(2 3)", want: gg.Scale(2, 3)},
		{
			name:  "svg matrix ordering",
			input: "matrix(1 2 3 4 5 6)",
			want:  gg.Matrix{A: 1, B: 3, C: 5, D: 2, E: 4, F: 6},
		},
		{
			name:  "skew x",
			input: "skewX(45)",
			want:  gg.Matrix{A: 1, B: 1, E: 1},
		},
		{
			name:  "skew y",
			input: "skewY(45)",
			want:  gg.Matrix{A: 1, D: 1, E: 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := transformMatrix(test.input)
			if err != nil {
				t.Fatalf("transformMatrix(%q): %v", test.input, err)
			}
			assertMatrixNear(t, got, test.want)
		})
	}

	got, err := transformMatrix("translate(1 2) \n scale (2 3)")
	if err != nil {
		t.Fatalf("whitespace-separated transform list: %v", err)
	}
	assertMatrixNear(t, got, gg.Translate(1, 2).Multiply(gg.Scale(2, 3)))
}

func TestTransformMatrixRejectsMalformedForms(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		message string
		want    gg.Matrix
	}{
		{name: "missing open parenthesis", input: "translate 1", message: "expected '('", want: gg.Identity()},
		{name: "missing close parenthesis", input: "translate(1", message: "missing ')'", want: gg.Identity()},
		{name: "invalid number", input: "translate(nope)", message: "invalid number", want: gg.Identity()},
		{name: "translate arity", input: "translate()", message: "translate requires", want: gg.Identity()},
		{name: "rotate arity", input: "rotate(1 2)", message: "rotate requires", want: gg.Identity()},
		{name: "scale arity", input: "scale()", message: "scale requires", want: gg.Identity()},
		{name: "matrix arity", input: "matrix(1 2 3)", message: "matrix requires", want: gg.Identity()},
		{name: "skew x arity", input: "skewX(1 2)", message: "skewX requires", want: gg.Identity()},
		{name: "skew y arity", input: "skewY()", message: "skewY requires", want: gg.Identity()},
		{name: "unsupported", input: "perspective(1)", message: "unsupported transform", want: gg.Identity()},
		{
			name: "preserves valid prefix", input: "translate(4 5) scale()", message: "scale requires",
			want: gg.Translate(4, 5),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := transformMatrix(test.input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("transformMatrix(%q) error=%v, want containing %q", test.input, err, test.message)
			}
			assertMatrixNear(t, got, test.want)
		})
	}

	args, err := parseTransformArgs(" \t\n ")
	if err != nil || args != nil {
		t.Fatalf("parseTransformArgs(whitespace)=(%v,%v), want (nil,nil)", args, err)
	}
}

func assertMatrixNear(t *testing.T, got, want gg.Matrix) {
	t.Helper()
	gotValues := [...]float64{got.A, got.B, got.C, got.D, got.E, got.F}
	wantValues := [...]float64{want.A, want.B, want.C, want.D, want.E, want.F}
	for i := range gotValues {
		if math.Abs(gotValues[i]-wantValues[i]) > 1e-12 {
			t.Fatalf("matrix=%+v, want %+v", got, want)
		}
	}
}
