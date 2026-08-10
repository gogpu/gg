package clip

import "testing"

func TestMaskClipperFillRulesAcrossRasterModes(t *testing.T) {
	for _, antiAlias := range []bool{false, true} {
		for _, rule := range []FillRule{FillRuleNonZero, FillRuleEvenOdd} {
			for _, reverseInner := range []bool{false, true} {
				name := "nonzero"
				if rule == FillRuleEvenOdd {
					name = "evenodd"
				}
				if reverseInner {
					name += "-reverse-inner"
				}
				t.Run(name+"/aa="+boolName(antiAlias), func(t *testing.T) {
					verbs, coords := nestedRectPath(reverseInner)
					mask, err := NewMaskClipperWithRule(verbs, coords, NewRect(0, 0, 100, 100), antiAlias, rule)
					if err != nil {
						t.Fatalf("NewMaskClipperWithRule: %v", err)
					}
					center := mask.Coverage(50, 50)
					wantCenter := byte(0)
					if rule == FillRuleNonZero && !reverseInner {
						wantCenter = 255
					}
					if center != wantCenter {
						t.Errorf("center coverage = %d, want %d", center, wantCenter)
					}
					if got := mask.Coverage(15, 50); got == 0 {
						t.Errorf("outer-only coverage = %d, want non-zero", got)
					}
				})
			}
		}
	}
}

func TestMaskClipperDefaultsToNonZero(t *testing.T) {
	verbs, coords := nestedRectPath(false)
	for _, antiAlias := range []bool{false, true} {
		mask, err := NewMaskClipper(verbs, coords, NewRect(0, 0, 100, 100), antiAlias)
		if err != nil {
			t.Fatalf("NewMaskClipper(aa=%v): %v", antiAlias, err)
		}
		if got := mask.Coverage(50, 50); got == 0 {
			t.Errorf("default center coverage (aa=%v) = %d, want non-zero", antiAlias, got)
		}
	}

	stack := NewClipStack(NewRect(0, 0, 100, 100))
	if err := stack.PushPath(verbs, coords, true); err != nil {
		t.Fatalf("ClipStack.PushPath: %v", err)
	}
	if got := stack.Coverage(50, 50); got == 0 {
		t.Errorf("default stack center coverage = %d, want non-zero", got)
	}
}

func TestMaskClipperDegenerateSubpathsProduceEmptyMask(t *testing.T) {
	tests := []struct {
		name   string
		verbs  []PathVerb
		coords []float64
	}{
		{
			name:  "close-without-subpath",
			verbs: []PathVerb{VerbClose},
		},
		{
			name:   "move-only-subpath",
			verbs:  []PathVerb{VerbMoveTo},
			coords: []float64{50, 50},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mask, err := NewMaskClipperWithRule(tc.verbs, tc.coords, NewRect(0, 0, 100, 100), true, FillRuleNonZero)
			if err != nil {
				t.Fatalf("NewMaskClipperWithRule: %v", err)
			}
			if got := mask.Coverage(50, 50); got != 0 {
				t.Errorf("coverage = %d, want 0", got)
			}
		})
	}
}

func TestMaskClipperDuplicateXAndTouchingEdges(t *testing.T) {
	paths := []struct {
		name   string
		verbs  []PathVerb
		coords []float64
	}{
		{
			name:   "duplicate-x",
			verbs:  []PathVerb{VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose, VerbMoveTo, VerbLineTo, VerbLineTo, VerbClose},
			coords: []float64{10, 10, 50, 50, 10, 90, 50, 50, 90, 10, 90, 90},
		},
		{
			name: "touching-edges",
			verbs: []PathVerb{
				VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo, VerbClose,
				VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo, VerbClose,
			},
			coords: []float64{10, 10, 50, 10, 50, 90, 10, 90, 50, 10, 90, 10, 90, 90, 50, 90},
		},
	}

	for _, tc := range paths {
		t.Run(tc.name, func(t *testing.T) {
			mask, err := NewMaskClipperWithRule(tc.verbs, tc.coords, NewRect(0, 0, 100, 100), true, FillRuleNonZero)
			if err != nil {
				t.Fatalf("NewMaskClipperWithRule: %v", err)
			}
			if got := mask.Coverage(25, 50); got == 0 {
				t.Errorf("left coverage = %d, want non-zero", got)
			}
			if got := mask.Coverage(75, 50); got == 0 {
				t.Errorf("right coverage = %d, want non-zero", got)
			}
		})
	}
}

func TestMaskClipperDisjointSubpathsDoNotConnect(t *testing.T) {
	verbs := []PathVerb{
		VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo, VerbClose,
		VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo, VerbClose,
	}
	coords := []float64{
		10, 10, 30, 10, 30, 30, 10, 30,
		70, 70, 90, 70, 90, 90, 70, 90,
	}

	for _, antiAlias := range []bool{false, true} {
		for _, rule := range []FillRule{FillRuleNonZero, FillRuleEvenOdd} {
			name := "nonzero"
			if rule == FillRuleEvenOdd {
				name = "evenodd"
			}
			t.Run(name+"/aa="+boolName(antiAlias), func(t *testing.T) {
				mask, err := NewMaskClipperWithRule(verbs, coords, NewRect(0, 0, 100, 100), antiAlias, rule)
				if err != nil {
					t.Fatalf("NewMaskClipperWithRule: %v", err)
				}
				if got := mask.Coverage(20, 20); got == 0 {
					t.Errorf("first subpath coverage = %d, want non-zero", got)
				}
				if got := mask.Coverage(80, 80); got == 0 {
					t.Errorf("second subpath coverage = %d, want non-zero", got)
				}
				if got := mask.Coverage(50, 50); got != 0 {
					t.Errorf("between-subpaths coverage = %d, want 0", got)
				}
			})
		}
	}
}

func TestMaskClipperImplicitlyClosesOpenSubpaths(t *testing.T) {
	// Neither contour has a VerbClose. Fill semantics still close each contour
	// independently at the following MoveTo and at end of path.
	verbs := []PathVerb{
		VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo,
		VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo,
	}
	coords := []float64{
		10, 10, 30, 10, 30, 30, 10, 30,
		70, 70, 90, 70, 90, 90, 70, 90,
	}

	for _, antiAlias := range []bool{false, true} {
		for _, rule := range []FillRule{FillRuleNonZero, FillRuleEvenOdd} {
			name := "nonzero"
			if rule == FillRuleEvenOdd {
				name = "evenodd"
			}
			t.Run(name+"/aa="+boolName(antiAlias), func(t *testing.T) {
				mask, err := NewMaskClipperWithRule(verbs, coords, NewRect(0, 0, 100, 100), antiAlias, rule)
				if err != nil {
					t.Fatalf("NewMaskClipperWithRule: %v", err)
				}
				if got := mask.Coverage(20, 20); got == 0 {
					t.Errorf("first open subpath coverage = %d, want non-zero", got)
				}
				if got := mask.Coverage(80, 80); got == 0 {
					t.Errorf("second open subpath coverage = %d, want non-zero", got)
				}
				if got := mask.Coverage(50, 50); got != 0 {
					t.Errorf("between-subpaths coverage = %d, want 0", got)
				}
			})
		}
	}
}

func nestedRectPath(reverseInner bool) ([]PathVerb, []float64) {
	verbs := []PathVerb{
		VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo, VerbClose,
		VerbMoveTo, VerbLineTo, VerbLineTo, VerbLineTo, VerbClose,
	}
	coords := []float64{10, 10, 90, 10, 90, 90, 10, 90}
	if reverseInner {
		coords = append(coords, 30, 30, 30, 70, 70, 70, 70, 30)
	} else {
		coords = append(coords, 30, 30, 70, 30, 70, 70, 30, 70)
	}
	return verbs, coords
}

func boolName(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
