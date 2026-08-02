package svg

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/gogpu/gg"
)

func TestHintPolicyTargetAndWidthBoundaries(t *testing.T) {
	for _, target := range []float64{16, 24, 32, 33} {
		for _, width := range []float64{0, .99, 1, 1.5, 1.5001} {
			policy := newStrokeHintPolicy(target, target, 1, gg.Identity())
			want := target <= 32 && width > 0 && width <= 1.5
			if got := policy.permits(width); got != want {
				t.Errorf("target=%g width=%g permits=%v, want %v", target, width, got, want)
			}
		}
	}
}

func TestHintPolicyDefaultsInvalidDeviceScale(t *testing.T) {
	for _, deviceScale := range []float64{0, -1} {
		policy := newStrokeHintPolicy(16, 16, deviceScale, gg.Identity())
		if !policy.permits(1) || policy.scale != 1 {
			t.Fatalf("deviceScale=%g produced policy=%#v, want eligible 1x policy", deviceScale, policy)
		}
	}
}

func TestHintPolicyTransformEligibility(t *testing.T) {
	tests := []struct {
		name string
		m    gg.Matrix
		want bool
	}{
		{"translation", gg.Translate(2, -3), true},
		{"uniform", gg.Translate(1, 2).Multiply(gg.Scale(2, 2)), true},
		{"reflection", gg.Translate(1, 2).Multiply(gg.Scale(-2, 2)), true},
		{"nonuniform", gg.Scale(2, 3), false},
		{"rotation", gg.Rotate(math.Pi / 8), false},
		{"shear", gg.Shear(.2, 0), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := newStrokeHintPolicy(16, 16, 1, test.m).permits(.5)
			if got != test.want {
				t.Fatalf("permits=%v, want %v", got, test.want)
			}
		})
	}
}

func TestHintStrokePathSnapsCardinalCenters(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(-1.2, 2.2)
	path.LineTo(3.8, 2.2)
	path.MoveTo(-2.2, -3.8)
	path.LineTo(-2.2, 4.1)
	original := append([]float64(nil), path.Coords()...)
	policy := newStrokeHintPolicy(16, 16, 1, gg.Translate(.25, -.25))

	hinted := hintStrokePath(path, policy, 1)
	want := []float64{-1.2, 1.75, 3.8, 1.75, -1.75, -3.8, -1.75, 4.1}
	if !reflect.DeepEqual(hinted.Coords(), want) {
		t.Fatalf("hinted coords=%v, want %v", hinted.Coords(), want)
	}
	if !reflect.DeepEqual(path.Coords(), original) {
		t.Fatalf("source mutated: got %v, want %v", path.Coords(), original)
	}

	twice := hintStrokePath(hinted, policy, 1)
	if !reflect.DeepEqual(twice.Coords(), hinted.Coords()) {
		t.Fatalf("hinting not idempotent: once=%v twice=%v", hinted.Coords(), twice.Coords())
	}
}

func TestHintStrokePathPreservesIneligibleAndNonCardinalGeometry(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(.2, .3)
	path.LineTo(2.2, 2.3)
	path.QuadraticTo(3, 4, 5, 6)
	path.CubicTo(7, 8, 9, 10, 11, 12)
	path.Close()
	coords := append([]float64(nil), path.Coords()...)
	verbs := append([]gg.PathVerb(nil), path.Verbs()...)

	for _, policy := range []strokeHintPolicy{
		newStrokeHintPolicy(33, 33, 1, gg.Identity()),
		newStrokeHintPolicy(16, 16, 1, gg.Rotate(.1)),
		newStrokeHintPolicy(16, 16, 1, gg.Scale(1, 2)),
	} {
		if got := hintStrokePath(path, policy, 1); got != path {
			t.Error("ineligible path was copied")
		}
	}

	eligibleCopy := hintStrokePath(path, newStrokeHintPolicy(16, 16, 1, gg.Identity()), 1)
	if !reflect.DeepEqual(eligibleCopy.Coords(), coords) || !reflect.DeepEqual(eligibleCopy.Verbs(), verbs) {
		t.Fatalf("non-cardinal geometry changed: coords=%v verbs=%v", eligibleCopy.Coords(), eligibleCopy.Verbs())
	}
}

func TestHintStrokePathPreservesCloseAfterCurve(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(1, 1)
	path.QuadraticTo(2, 3, 4, 5)
	path.Close()

	hinted := hintStrokePath(path, newStrokeHintPolicy(16, 16, 1, gg.Identity()), 1)
	if !reflect.DeepEqual(hinted.Verbs(), path.Verbs()) || !reflect.DeepEqual(hinted.Coords(), path.Coords()) {
		t.Fatalf("curve-close path changed: verbs=%v coords=%v", hinted.Verbs(), hinted.Coords())
	}

	closedLines := gg.NewPath()
	closedLines.MoveTo(1.2, 1.2)
	closedLines.LineTo(4.2, 1.2)
	closedLines.LineTo(4.2, 4.2)
	closedLines.Close()
	hinted = hintStrokePath(closedLines, newStrokeHintPolicy(16, 16, 1, gg.Identity()), 1)
	if got, want := hinted.Verbs(), closedLines.Verbs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("closed line verbs=%v, want %v", got, want)
	}
}

func TestRenderHintPixelRowsAndColumns(t *testing.T) {
	for _, width := range []float64{1, 1.5} {
		for _, vertical := range []bool{false, true} {
			name := fmt.Sprintf("width_%g/vertical_%v", width, vertical)
			t.Run(name, func(t *testing.T) {
				coordinates := `x1="2" y1="4" x2="14" y2="4"`
				if vertical {
					coordinates = `x1="4" y1="2" x2="4" y2="14"`
				}
				doc, err := Parse([]byte(fmt.Sprintf(`<svg viewBox="0 0 16 16"><line %s stroke="black" stroke-width="%g"/></svg>`, coordinates, width)))
				if err != nil {
					t.Fatal(err)
				}
				img := doc.Render(16, 16)
				coverage := func(offset int) uint32 {
					x, y := 8, 4+offset
					if vertical {
						x, y = 4+offset, 8
					}
					_, _, _, alpha := img.At(x, y).RGBA()
					return alpha / 257
				}
				if coverage(0) != 255 {
					t.Fatalf("center coverage=%d, want 255", coverage(0))
				}
				if width == 1 && (coverage(-1) != 0 || coverage(1) != 0) {
					t.Fatalf("one-pixel neighboring coverage=(%d,%d), want zero", coverage(-1), coverage(1))
				}
				if width == 1.5 && (coverage(-1) == 0 || coverage(1) == 0 || coverage(-1) != coverage(1)) {
					t.Fatalf("1.5-pixel neighboring coverage=(%d,%d), want equal nonzero AA", coverage(-1), coverage(1))
				}
			})
		}
	}
}

func BenchmarkSVGHint(b *testing.B) {
	path, err := gg.ParseSVGPath("M1.2 1.2 H14.2 V14.2 H1.2 Z")
	if err != nil {
		b.Fatal(err)
	}
	policy := newStrokeHintPolicy(16, 16, 2, gg.Identity())
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = hintStrokePath(path, policy, .5)
	}
}
