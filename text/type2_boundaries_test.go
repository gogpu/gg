package text

import (
	"strings"
	"testing"
)

func requireType2Error(t *testing.T, program []byte, contains string) {
	t.Helper()
	result, err := decodeType2(program, nil, nil, 0)
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("result=%#v err=%v, want error containing %q", result, err, contains)
	}
	if result.segments != nil {
		t.Fatalf("decoder leaked partial geometry: %#v", result.segments)
	}
}

func TestType2BooleanAndStackAlternativeBranches(t *testing.T) {
	tests := map[string]struct {
		program []byte
		want    OutlinePoint
	}{
		"and false":      {t2prog(0, 3, byte(12), byte(3), 0, byte(21), byte(14)), OutlinePoint{0, 0}},
		"or false":       {t2prog(0, 0, byte(12), byte(4), 0, byte(21), byte(14)), OutlinePoint{0, 0}},
		"not true input": {t2prog(7, byte(12), byte(5), 0, byte(21), byte(14)), OutlinePoint{0, 0}},
		"eq false":       {t2prog(7, 8, byte(12), byte(15), 0, byte(21), byte(14)), OutlinePoint{0, 0}},
		"ifelse second":  {t2prog(11, 22, 3, 2, byte(12), byte(22), 0, byte(21), byte(14)), OutlinePoint{22, 0}},
		"index negative": {t2prog(10, -1, byte(12), byte(29), byte(21), byte(14)), OutlinePoint{10, 10}},
		"roll negative":  {t2prog(1, 2, 2, -1, byte(12), byte(30), byte(21), byte(14)), OutlinePoint{2, 1}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := decodeType2(test.program, nil, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.segments) != 1 || result.segments[0].Points[0] != test.want {
				t.Fatalf("segments=%#v, want endpoint %v", result.segments, test.want)
			}
		})
	}

	requireType2Error(t, t2prog([]byte{255, 0, 1, 0x80, 0}, byte(12), byte(29), byte(14)), "not an integer")
}

func TestType2LateOperatorArityAndMaskErrors(t *testing.T) {
	for name, program := range map[string][]byte{
		"hintmask odd after width": t2prog(10, 20, byte(1), 30, byte(19), byte(0), byte(14)),
		"vmoveto after width":      t2prog(0, 0, byte(21), 1, 2, byte(4), byte(14)),
		"hmoveto after width":      t2prog(0, 0, byte(21), 1, 2, byte(22), byte(14)),
	} {
		t.Run(name, func(t *testing.T) { requireType2Error(t, program, "operator") })
	}
}

func TestType2Flex1HorizontalDominantAxis(t *testing.T) {
	program := t2prog(0, 0, byte(21), 10, 0, 10, 0, 10, 0, 10, 0, 10, 0, 5, byte(12), byte(37), byte(14))
	result, err := decodeType2(program, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.segments) != 3 {
		t.Fatalf("segments=%#v", result.segments)
	}
	if endpoint := result.segments[2].Points[2]; endpoint != (OutlinePoint{X: 55, Y: 0}) {
		t.Fatalf("endpoint=%v, want (55,0)", endpoint)
	}
}

func TestType2IndependentSegmentLimits(t *testing.T) {
	t.Run("move", func(t *testing.T) {
		program := make([]byte, 0, (type2MaxSegments+1)*3)
		for range type2MaxSegments + 1 {
			program = append(program, 139, 139, 21)
		}
		requireType2Error(t, program, "segment limit")
	})

	t.Run("alternating line", func(t *testing.T) {
		program := t2prog(0, 0, byte(21))
		for emitted := 0; emitted <= type2MaxSegments; emitted += type2MaxStack {
			for range type2MaxStack {
				program = append(program, 140) // delta 1
			}
			program = append(program, 6)
		}
		requireType2Error(t, program, "segment limit")
	})

	t.Run("curve", func(t *testing.T) {
		program := t2prog(0, 0, byte(21))
		const curvesPerOp = type2MaxStack / 6
		for emitted := 0; emitted <= type2MaxSegments; emitted += curvesPerOp {
			for range type2MaxStack {
				program = append(program, 139)
			}
			program = append(program, 8)
		}
		requireType2Error(t, program, "segment limit")
	})
}

func TestType2NumberReaderDirectBounds(t *testing.T) {
	if _, _, err := readType2Number(nil, 0); err == nil {
		t.Fatal("out-of-range number read accepted")
	}
	if _, _, err := readType2Number([]byte{0}, 0); err == nil {
		t.Fatal("operator byte accepted as number")
	}
}
