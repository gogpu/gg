package text

import (
	"strings"
	"testing"
)

func t2num(v int) []byte {
	if v >= -107 && v <= 107 {
		return []byte{byte(v + 139)}
	}
	if v >= 108 && v <= 1131 {
		n := v - 108
		return []byte{byte(n/256 + 247), byte(n % 256)}
	}
	if v >= -1131 && v <= -108 {
		n := -v - 108
		return []byte{byte(n/256 + 251), byte(n % 256)}
	}
	return []byte{28, byte(uint16(v) >> 8), byte(v)}
}
func t2prog(parts ...any) []byte {
	var p []byte
	for _, part := range parts {
		switch v := part.(type) {
		case int:
			p = append(p, t2num(v)...)
		case byte:
			p = append(p, v)
		case []byte:
			p = append(p, v...)
		}
	}
	return p
}

func TestType2NumericEncodings(t *testing.T) {
	tests := []struct {
		data []byte
		want float64
	}{{[]byte{32}, -107}, {[]byte{246}, 107}, {[]byte{247, 0}, 108}, {[]byte{250, 255}, 1131}, {[]byte{251, 0}, -108}, {[]byte{254, 255}, -1131}, {[]byte{28, 0x80, 0}, -32768}, {[]byte{255, 0, 1, 0x80, 0}, 1.5}}
	for _, tt := range tests {
		got, next, err := readType2Number(tt.data, 0)
		if err != nil || got != tt.want || next != len(tt.data) {
			t.Fatalf("readType2Number(%v)=(%v,%d,%v), want %v", tt.data, got, next, err, tt.want)
		}
	}
	for _, data := range [][]byte{{28}, {255, 0}, {247}, {254}} {
		if _, _, err := readType2Number(data, 0); err == nil {
			t.Errorf("expected truncation for %v", data)
		}
	}
}

func TestType2MoveLineCurveFamilies(t *testing.T) {
	p := t2prog(10, 20, byte(21), 30, 0, 0, 40, -10, 0, byte(5), 5, 10, 10, 5, 5, -5, byte(8), byte(14))
	r, err := decodeType2(p, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []OutlineSegment{
		{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{10, 20}}},
		{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{40, 20}}},
		{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{40, 60}}},
		{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{30, 60}}},
		{Op: OutlineOpCubicTo, Points: [3]OutlinePoint{{35, 70}, {45, 75}, {50, 70}}},
	}
	if len(r.segments) != len(want) {
		t.Fatalf("segments=%#v", r.segments)
	}
	for i := range want {
		if r.segments[i] != want[i] {
			t.Errorf("segment %d=%#v want %#v", i, r.segments[i], want[i])
		}
	}

	for name, p := range map[string][]byte{
		"h/v line":   t2prog(0, 0, byte(21), 10, 20, 30, byte(6), byte(14)),
		"curve-line": t2prog(0, 0, byte(21), 1, 2, 3, 4, 5, 6, 7, 8, byte(24), byte(14)),
		"line-curve": t2prog(0, 0, byte(21), 1, 2, 3, 4, 5, 6, 7, 8, byte(25), byte(14)),
		"vv":         t2prog(0, 0, byte(21), 1, 2, 3, 4, byte(26), byte(14)),
		"hh":         t2prog(0, 0, byte(21), 1, 2, 3, 4, byte(27), byte(14)),
		"vh":         t2prog(0, 0, byte(21), 1, 2, 3, 4, byte(30), byte(14)),
		"hv":         t2prog(0, 0, byte(21), 1, 2, 3, 4, byte(31), byte(14)),
	} {
		t.Run(name, func(t *testing.T) {
			r, e := decodeType2(p, nil, nil, 0)
			if e != nil {
				t.Fatal(e)
			}
			if len(r.segments) < 2 {
				t.Fatalf("no drawn segment: %#v", r)
			}
		})
	}
}

func TestType2WidthStemsMasksAndSubrBias(t *testing.T) {
	// Width 50, one stem, one-byte hint mask, then a move.
	p := t2prog(50, 10, 20, byte(1), byte(19), byte(0x80), 0, 0, byte(21), byte(14))
	r, err := decodeType2(p, nil, nil, 500)
	if err != nil {
		t.Fatal(err)
	}
	if !r.hasWidth || r.width != 550 {
		t.Fatalf("width=%v has=%v", r.width, r.hasWidth)
	}

	for _, n := range []int{0, 1239, 1240, 33899, 33900} {
		want := 107
		if n >= 1240 {
			want = 1131
		}
		if n >= 33900 {
			want = 32768
		}
		if got := type2SubrBias(n); got != want {
			t.Errorf("bias(%d)=%d want %d", n, got, want)
		}
	}
	local := make([][]byte, 1)
	local[0] = t2prog(10, 0, byte(5), byte(11))
	global := make([][]byte, 1)
	global[0] = t2prog(0, 20, byte(5), byte(11))
	p = t2prog(0, 0, byte(21), -107, byte(10), -107, byte(29), byte(14))
	r, err = decodeType2(p, local, global, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.segments[len(r.segments)-1].Points[0]; got != (OutlinePoint{10, 20}) {
		t.Fatalf("subr endpoint=%v", got)
	}
}

func TestType2EndcharInSubroutineTerminatesGlyph(t *testing.T) {
	local := [][]byte{t2prog(10, 0, byte(5), byte(14))}
	// The unsupported operator after callsubr must remain unreachable: endchar
	// in the subroutine terminates the complete glyph program.
	program := t2prog(0, 0, byte(21), -107, byte(10), byte(2))
	result, err := decodeType2(program, local, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.segments[len(result.segments)-1].Points[0]; got != (OutlinePoint{10, 0}) {
		t.Fatalf("subroutine endchar endpoint=%v, want (10,0)", got)
	}
}

func TestType2FlexOperators(t *testing.T) {
	programs := map[string][]byte{
		"hflex":  t2prog(0, 0, byte(21), 10, 20, 5, 30, 40, 5, 10, byte(12), byte(34), byte(14)),
		"flex":   t2prog(0, 0, byte(21), 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 50, byte(12), byte(35), byte(14)),
		"hflex1": t2prog(0, 0, byte(21), 1, 2, 3, 4, 5, 6, 7, 8, 9, byte(12), byte(36), byte(14)),
		"flex1":  t2prog(0, 0, byte(21), 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, byte(12), byte(37), byte(14)),
	}
	for name, p := range programs {
		t.Run(name, func(t *testing.T) {
			r, err := decodeType2(p, nil, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.segments) != 3 || r.segments[1].Op != OutlineOpCubicTo || r.segments[2].Op != OutlineOpCubicTo {
				t.Fatalf("segments=%#v", r.segments)
			}
		})
	}
}

func TestType2EscapedStackAndArithmeticOperators(t *testing.T) {
	tests := map[string]struct {
		program []byte
		want    OutlinePoint
	}{
		"and":     {t2prog(2, 3, byte(12), byte(3), 0, byte(21), byte(14)), OutlinePoint{1, 0}},
		"or":      {t2prog(0, 3, byte(12), byte(4), 0, byte(21), byte(14)), OutlinePoint{1, 0}},
		"not":     {t2prog(0, byte(12), byte(5), 0, byte(21), byte(14)), OutlinePoint{1, 0}},
		"abs":     {t2prog(-7, byte(12), byte(9), 0, byte(21), byte(14)), OutlinePoint{7, 0}},
		"add":     {t2prog(7, 5, byte(12), byte(10), 0, byte(21), byte(14)), OutlinePoint{12, 0}},
		"sub":     {t2prog(7, 5, byte(12), byte(11), 0, byte(21), byte(14)), OutlinePoint{2, 0}},
		"div":     {t2prog(12, 3, byte(12), byte(12), 0, byte(21), byte(14)), OutlinePoint{4, 0}},
		"neg":     {t2prog(7, byte(12), byte(14), 0, byte(21), byte(14)), OutlinePoint{-7, 0}},
		"eq":      {t2prog(7, 7, byte(12), byte(15), 0, byte(21), byte(14)), OutlinePoint{1, 0}},
		"drop":    {t2prog(7, 99, byte(12), byte(18), 0, byte(21), byte(14)), OutlinePoint{7, 0}},
		"put-get": {t2prog(42, 3, byte(12), byte(20), 3, byte(12), byte(21), 0, byte(21), byte(14)), OutlinePoint{42, 0}},
		"ifelse":  {t2prog(11, 22, 1, 2, byte(12), byte(22), 0, byte(21), byte(14)), OutlinePoint{11, 0}},
		"random":  {t2prog(byte(12), byte(23), 0, byte(21), byte(14)), OutlinePoint{0.5, 0}},
		"mul":     {t2prog(6, 7, byte(12), byte(24), 0, byte(21), byte(14)), OutlinePoint{42, 0}},
		"sqrt":    {t2prog(81, byte(12), byte(26), 0, byte(21), byte(14)), OutlinePoint{9, 0}},
		"dup":     {t2prog(7, byte(12), byte(27), byte(21), byte(14)), OutlinePoint{7, 7}},
		"exch":    {t2prog(1, 2, byte(12), byte(28), byte(21), byte(14)), OutlinePoint{2, 1}},
		"index":   {t2prog(10, 0, byte(12), byte(29), byte(21), byte(14)), OutlinePoint{10, 10}},
		"roll":    {t2prog(1, 2, 2, 1, byte(12), byte(30), byte(21), byte(14)), OutlinePoint{2, 1}},
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
}

func TestType2MoveStemAndCurveVariants(t *testing.T) {
	for name, program := range map[string][]byte{
		"vmoveto":     t2prog(10, byte(4), byte(14)),
		"hmoveto":     t2prog(10, byte(22), byte(14)),
		"vlineto":     t2prog(0, 0, byte(21), 10, 20, byte(7), byte(14)),
		"vv-extra":    t2prog(0, 0, byte(21), 1, 2, 3, 4, 5, byte(26), byte(14)),
		"hh-extra":    t2prog(0, 0, byte(21), 1, 2, 3, 4, 5, byte(27), byte(14)),
		"vh-extra":    t2prog(0, 0, byte(21), 1, 2, 3, 4, 5, byte(30), byte(14)),
		"hv-extra":    t2prog(0, 0, byte(21), 1, 2, 3, 4, 5, byte(31), byte(14)),
		"multi-curve": t2prog(0, 0, byte(21), 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, byte(8), byte(14)),
	} {
		t.Run(name, func(t *testing.T) {
			result, err := decodeType2(program, nil, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.segments) == 0 {
				t.Fatal("operator produced no geometry")
			}
		})
	}

	for _, stemOp := range []byte{1, 3, 18, 23} {
		program := t2prog(10, 20, stemOp, 0, 0, byte(21), byte(14))
		if _, err := decodeType2(program, nil, nil, 0); err != nil {
			t.Errorf("stem operator %d: %v", stemOp, err)
		}
	}
	// cntrmask consumes one byte for the single declared stem.
	if _, err := decodeType2(t2prog(10, 20, byte(20), byte(0x80), 0, 0, byte(21), byte(14)), nil, nil, 0); err != nil {
		t.Fatal(err)
	}
}

func TestType2RejectsInvalidEscapedOperators(t *testing.T) {
	tests := map[string][]byte{
		"truncated escape": {12},
		"and underflow":    t2prog(1, byte(12), byte(3), byte(14)),
		"not underflow":    t2prog(byte(12), byte(5), byte(14)),
		"division by zero": t2prog(1, 0, byte(12), byte(12), byte(14)),
		"negative sqrt":    t2prog(-1, byte(12), byte(26), byte(14)),
		"put range":        t2prog(1, 32, byte(12), byte(20), byte(14)),
		"put non-integer":  t2prog(1, []byte{255, 0, 1, 0x80, 0}, byte(12), byte(20), byte(14)),
		"get range":        t2prog(32, byte(12), byte(21), byte(14)),
		"get non-integer":  t2prog([]byte{255, 0, 1, 0x80, 0}, byte(12), byte(21), byte(14)),
		"index empty":      t2prog(0, byte(12), byte(29), byte(14)),
		"roll range":       t2prog(1, 2, 3, 1, byte(12), byte(30), byte(14)),
		"roll count float": t2prog(1, []byte{255, 0, 1, 0x80, 0}, 1, byte(12), byte(30), byte(14)),
		"roll shift float": t2prog(1, 1, []byte{255, 0, 1, 0x80, 0}, byte(12), byte(30), byte(14)),
		"hflex arity":      t2prog(1, byte(12), byte(34), byte(14)),
		"flex arity":       t2prog(1, byte(12), byte(35), byte(14)),
		"hflex1 arity":     t2prog(1, byte(12), byte(36), byte(14)),
		"flex1 arity":      t2prog(1, byte(12), byte(37), byte(14)),
	}
	for name, program := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeType2(program, nil, nil, 0); err == nil {
				t.Fatal("invalid program accepted")
			}
		})
	}
}

func TestType2WidthAndOperatorErrorBoundaries(t *testing.T) {
	for name, program := range map[string][]byte{
		"vmoveto width": t2prog(50, 10, byte(4), byte(14)),
		"hmoveto width": t2prog(50, 10, byte(22), byte(14)),
		"endchar width": t2prog(50, byte(14)),
	} {
		t.Run(name, func(t *testing.T) {
			result, err := decodeType2(program, nil, nil, 500)
			if err != nil {
				t.Fatal(err)
			}
			if !result.hasWidth || result.width != 550 {
				t.Fatalf("width=%g hasWidth=%v, want 550 true", result.width, result.hasWidth)
			}
		})
	}

	invalid := map[string][]byte{
		"odd stem operands":     t2prog(0, 0, byte(21), 10, byte(1), byte(14)),
		"truncated hint mask":   t2prog(10, 20, byte(19)),
		"vmoveto arity":         t2prog(0, 0, 0, byte(4), byte(14)),
		"rmoveto arity":         t2prog(0, byte(21), byte(14)),
		"hmoveto arity":         t2prog(0, 0, 0, byte(22), byte(14)),
		"hlineto underflow":     t2prog(byte(6), byte(14)),
		"rrcurveto arity":       t2prog(1, 2, byte(8), byte(14)),
		"rcurveline arity":      t2prog(1, 2, 3, 4, 5, 6, byte(24), byte(14)),
		"rlinecurve arity":      t2prog(1, 2, 3, 4, 5, 6, byte(25), byte(14)),
		"vvcurveto underflow":   t2prog(1, 2, 3, byte(26), byte(14)),
		"vvcurveto arity":       t2prog(1, 2, 3, 4, 5, 6, byte(26), byte(14)),
		"vhcurveto underflow":   t2prog(1, 2, 3, byte(30), byte(14)),
		"vhcurveto arity":       t2prog(1, 2, 3, 4, 5, 6, byte(30), byte(14)),
		"fractional subroutine": t2prog([]byte{255, 0, 1, 0x80, 0}, byte(10), byte(14)),
		"endchar operand count": t2prog(1, 2, byte(14)),
		"endchar seac":          t2prog(1, 2, 3, 4, byte(14)),
	}
	for name, program := range invalid {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeType2(program, nil, nil, 0); err == nil {
				t.Fatal("invalid program accepted")
			}
		})
	}

	if _, err := decodeType2(t2prog(-107, byte(10), byte(14)), [][]byte{{}}, nil, 0); err == nil {
		t.Fatal("subroutine without return accepted")
	}
}

func TestType2NonFiniteGeometryAndStackLimits(t *testing.T) {
	// Repeated multiplication stays representable as float64 while exceeding
	// float32, which exercises the decoder's output-coordinate boundary.
	huge := t2prog(32767, 32767, byte(12), byte(24))
	for range 7 {
		huge = append(huge, t2prog(32767, byte(12), byte(24))...)
	}
	for name, program := range map[string][]byte{
		"move":  t2prog(huge, 0, byte(21), byte(14)),
		"line":  t2prog(0, 0, byte(21), huge, 0, byte(5), byte(14)),
		"curve": t2prog(0, 0, byte(21), huge, 0, 0, 0, 0, 0, byte(8), byte(14)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeType2(program, nil, nil, 0); err == nil {
				t.Fatal("out-of-range geometry accepted")
			}
		})
	}

	full := make([]byte, type2MaxStack)
	for i := range full {
		full[i] = 139
	}
	for name, operator := range map[string][]byte{
		"random": {12, 23},
		"dup":    {12, 27},
	} {
		t.Run(name, func(t *testing.T) {
			program := append(append([]byte(nil), full...), operator...)
			program = append(program, 14)
			if _, err := decodeType2(program, nil, nil, 0); err == nil {
				t.Fatal("stack overflow accepted")
			}
		})
	}
}

func TestType2RejectsMalformedPrograms(t *testing.T) {
	deep := make([][]byte, 1)
	deep[0] = t2prog(-107, byte(10), byte(11))
	chain := make([][]byte, type2MaxCallDepth)
	for i := range len(chain) - 1 {
		chain[i] = t2prog(i+1-type2SubrBias(len(chain)), byte(10), byte(11))
	}
	chain[len(chain)-1] = []byte{11}
	many := make([]byte, 0, 2*(type2MaxOperations+1))
	for range type2MaxOperations + 1 {
		many = append(many, 12, 0)
	}
	stackOverflow := make([]byte, 0, 50)
	for range type2MaxStack + 1 {
		stackOverflow = append(stackOverflow, 139)
	}
	tests := map[string]struct {
		p        []byte
		locals   [][]byte
		contains string
	}{
		"truncated number":  {[]byte{28}, nil, "truncated"},
		"reserved operator": {[]byte{2}, nil, "unsupported"},
		"underflow":         {[]byte{5}, nil, "invalid operand"},
		"overflow":          {stackOverflow, nil, "overflow"},
		"invalid subr":      {t2prog(-107, byte(10), byte(14)), nil, "out of range"},
		"cycle":             {t2prog(-107, byte(10), byte(14)), deep, "cycle"},
		"recursion limit":   {t2prog(-107, byte(10), byte(14)), chain, "recursion limit"},
		"operation limit":   {many, nil, "operation limit"},
		"missing endchar":   {t2prog(0, 0, byte(21)), nil, "without endchar"},
		"return top":        {[]byte{11}, nil, "outside"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r, err := decodeType2(tt.p, tt.locals, nil, 0)
			if err == nil || !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("result=%#v err=%v want containing %q", r, err, tt.contains)
			}
			if r.segments != nil || r.hasWidth {
				t.Fatalf("partial result returned: %#v", r)
			}
		})
	}
}
