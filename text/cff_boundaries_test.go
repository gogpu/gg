package text

import "testing"

// cffWithTop builds the smallest complete CFF around a caller-supplied Top
// DICT. Fixed-width offset operands make the second builder call stable.
func cffWithTop(buildTop func(charStringsOffset, extraOffset int) []byte, charStrings, extra []byte) []byte {
	name := cffIndexForTest([][]byte{[]byte("B")}, 1)
	strings := cffIndexForTest(nil, 1)
	globals := cffIndexForTest(nil, 1)
	topObject := buildTop(0, 0)
	charStringsOffset := 4 + len(name) + 5 + len(topObject) + len(strings) + len(globals)
	extraOffset := charStringsOffset + len(charStrings)
	topObject = buildTop(charStringsOffset, extraOffset)
	top := cffIndexForTest([][]byte{topObject}, 1)
	out := []byte{1, 0, 4, 1}
	for _, part := range [][]byte{name, top, strings, globals, charStrings, extra} {
		out = append(out, part...)
	}
	return out
}

func cffTopCharStrings(charStringsOffset int) []byte {
	top := append([]byte(nil), cffLong(charStringsOffset)...)
	return append(top, 17)
}

func requireCFFParseError(t *testing.T, data []byte) {
	t.Helper()
	if _, err := parseCFF1(data); err == nil {
		t.Fatal("malformed CFF accepted")
	}
}

func TestCFFTopPrivateAndOffsetBoundaries(t *testing.T) {
	chars := cffIndexForTest([][]byte{{14}}, 1)
	tests := map[string][]byte{
		"missing CharStrings operator": cffWithTop(func(_, _ int) []byte { return nil }, chars, nil),
		"malformed CharStrings INDEX":  cffWithTop(func(charOff, _ int) []byte { return cffTopCharStrings(charOff) }, []byte{0, 1, 0}, nil),
		"empty CharStrings INDEX":      cffWithTop(func(charOff, _ int) []byte { return cffTopCharStrings(charOff) }, []byte{0, 0}, nil),
		"Private arity": cffWithTop(func(charOff, _ int) []byte {
			return append(cffTopCharStrings(charOff), 139, 18)
		}, chars, nil),
		"Private fractional size": cffWithTop(func(charOff, extraOff int) []byte {
			top := append(cffTopCharStrings(charOff), 30, 0x1a, 0x5f)
			top = append(top, cffLong(extraOff)...)
			return append(top, 18)
		}, chars, nil),
		"Private fractional offset": cffWithTop(func(charOff, _ int) []byte {
			top := append(cffTopCharStrings(charOff), 139, 30, 0x1a, 0x5f)
			return append(top, 18)
		}, chars, nil),
		"Private outside table": cffWithTop(func(charOff, _ int) []byte {
			top := append(cffTopCharStrings(charOff), 140)
			top = append(top, cffLong(1<<20)...)
			return append(top, 18)
		}, chars, nil),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) { requireCFFParseError(t, data) })
	}

	valid := cffWithTop(func(charOff, extraOff int) []byte {
		top := append(cffTopCharStrings(charOff), 141)
		top = append(top, cffLong(extraOff)...)
		return append(top, 18)
	}, chars, []byte{140, 20})
	font, err := parseCFF1(valid)
	if err != nil {
		t.Fatal(err)
	}
	if font.private.defaultWidth != 1 {
		t.Fatalf("defaultWidth=%g, want 1", font.private.defaultWidth)
	}
}

// cffCIDWithFD builds a one-glyph CID CFF. fdKind selects an FD DICT
// boundary while preserving valid surrounding offsets.
func cffCIDWithFD(fdKind string, fdSelect []byte) []byte {
	name := cffIndexForTest([][]byte{[]byte("D")}, 1)
	strings, globals := cffIndexForTest(nil, 1), cffIndexForTest(nil, 1)
	chars := cffIndexForTest([][]byte{{14}}, 1)
	const topObjectLen = 25
	charOff := 4 + len(name) + 5 + topObjectLen + len(strings) + len(globals)
	fdArrayOff := charOff + len(chars)
	dictLen := 7
	switch fdKind {
	case "malformed":
		dictLen = 1
	case "missing-private":
		dictLen = 1
	case "fractional-private":
		dictLen = 9
	}
	fdArrayLen := 5 + dictLen
	fdSelectOff := fdArrayOff + fdArrayLen
	privateOff := fdSelectOff + len(fdSelect)
	var private []byte
	var fdDict []byte
	switch fdKind {
	case "malformed":
		fdDict = []byte{31}
	case "missing-private":
		fdDict = []byte{17}
	case "fractional-private":
		fdDict = []byte{30, 0x1a, 0x5f}
		fdDict = append(fdDict, cffLong(privateOff)...)
		fdDict = append(fdDict, 18)
	case "bad-private":
		fdDict = append([]byte{140}, cffLong(privateOff)...)
		fdDict = append(fdDict, 18)
		private = []byte{31}
	default:
		fdDict = append([]byte{139}, cffLong(privateOff)...)
		fdDict = append(fdDict, 18)
	}
	fdArray := cffIndexForTest([][]byte{fdDict}, 1)
	top := make([]byte, 0, topObjectLen)
	top = append(top, 139, 139, 139, 12, 30)
	top = append(top, cffLong(charOff)...)
	top = append(top, 17)
	top = append(top, cffLong(fdArrayOff)...)
	top = append(top, 12, 36)
	top = append(top, cffLong(fdSelectOff)...)
	top = append(top, 12, 37)
	topIndex := cffIndexForTest([][]byte{top}, 1)
	out := []byte{1, 0, 4, 1}
	for _, part := range [][]byte{name, topIndex, strings, globals, chars, fdArray, fdSelect, private} {
		out = append(out, part...)
	}
	return out
}

func TestCFFCIDStructuralErrorBoundaries(t *testing.T) {
	chars := cffIndexForTest([][]byte{{14}}, 1)
	ros := func(values ...byte) func(int, int) []byte {
		return func(charOff, _ int) []byte {
			top := append(cffTopCharStrings(charOff), values...)
			return append(top, 12, 30)
		}
	}
	tests := map[string][]byte{
		"ROS arity":       cffWithTop(ros(139, 139), chars, nil),
		"missing FDArray": cffWithTop(ros(139, 139, 139), chars, nil),
		"missing FDSelect": cffWithTop(func(charOff, extraOff int) []byte {
			top := ros(139, 139, 139)(charOff, extraOff)
			top = append(top, cffLong(extraOff)...)
			return append(top, 12, 36)
		}, chars, nil),
		"malformed FDArray": cffWithTop(func(charOff, extraOff int) []byte {
			top := ros(139, 139, 139)(charOff, extraOff)
			top = append(top, cffLong(extraOff)...)
			top = append(top, 12, 36)
			top = append(top, cffLong(extraOff)...)
			return append(top, 12, 37)
		}, chars, []byte{0}),
		"empty FDArray": cffWithTop(func(charOff, extraOff int) []byte {
			top := ros(139, 139, 139)(charOff, extraOff)
			top = append(top, cffLong(extraOff)...)
			top = append(top, 12, 36)
			top = append(top, cffLong(extraOff+2)...)
			return append(top, 12, 37)
		}, chars, []byte{0, 0}),
		"malformed FD DICT":     cffCIDWithFD("malformed", []byte{0, 0}),
		"missing FD Private":    cffCIDWithFD("missing-private", []byte{0, 0}),
		"fractional FD Private": cffCIDWithFD("fractional-private", []byte{0, 0}),
		"malformed FD Private":  cffCIDWithFD("bad-private", []byte{0, 0}),
		"malformed FDSelect":    cffCIDWithFD("valid", []byte{2}),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) { requireCFFParseError(t, data) })
	}
}

func TestCFFIntegratedCharsetAndEncodingErrors(t *testing.T) {
	chars := cffIndexForTest([][]byte{{14}, {14}}, 1)
	for name, op := range map[string]byte{"charset": 15, "encoding": 16} {
		t.Run(name, func(t *testing.T) {
			data := cffWithTop(func(charOff, extraOff int) []byte {
				top := cffTopCharStrings(charOff)
				top = append(top, cffLong(extraOff)...)
				return append(top, op)
			}, chars, []byte{3})
			requireCFFParseError(t, data)
		})
	}
}

func TestCFFLowLevelUntrustedOffsetBoundaries(t *testing.T) {
	for name, raw := range map[string][]byte{
		"zero final offset":   {0, 1, 1, 1, 0},
		"decreasing offsets":  {0, 2, 1, 1, 0, 1},
		"object beyond final": {0, 2, 1, 1, 2, 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseCFFIndex(raw, 0, "boundary"); err == nil {
				t.Fatal("invalid INDEX accepted")
			}
		})
	}
	if _, _, err := readCFFDictNumber(nil, 0); err == nil {
		t.Fatal("out-of-range DICT number read accepted")
	}
	if _, _, err := readCFFDictNumber([]byte{30, 0xaf}, 0); err == nil {
		t.Fatal("syntactically invalid real accepted")
	}
	if _, err := parseCFFPrivate([]byte{141, 19, 0}, 2, 0); err == nil {
		t.Fatal("truncated Local Subrs INDEX accepted")
	}
	if _, err := parseCFFFDSelect([]byte{3, 0, 1, 0, 0, 0}, 0, 1, 1); err == nil {
		t.Fatal("FDSelect without sentinel accepted")
	}
	if err := parseCFFCharset([]byte{1, 0, 1, 2}, 0, 3); err == nil {
		t.Fatal("charset range exceeding glyph count accepted")
	}
	for name, data := range map[string][]byte{
		"format 0 missing count": {0},
		"format 1 missing count": {1},
		"truncated supplements":  {0x80, 0, 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := parseCFFEncoding(data, 0); err == nil {
				t.Fatal("invalid encoding accepted")
			}
		})
	}
}

func TestCFFOutlineIntegrationErrorPropagation(t *testing.T) {
	extractor := NewOutlineExtractor()
	valid := &ownParsedFont{
		upem:      1000,
		numGlyphs: 2,
		tables:    map[string][]byte{"CFF ": syntheticCFF1()},
	}
	if outline, err := extractor.extractCFF(valid, 9, 16, 10); err == nil || outline != nil {
		t.Fatalf("out-of-range glyph: outline=%#v err=%v", outline, err)
	}

	chars := cffIndexForTest([][]byte{{14}, {2}}, 1)
	malformedType2 := cffWithTop(func(charOff, _ int) []byte { return cffTopCharStrings(charOff) }, chars, nil)
	invalidGlyph := &ownParsedFont{
		upem:      1000,
		numGlyphs: 2,
		tables:    map[string][]byte{"CFF ": malformedType2},
	}
	if outline, err := extractor.extractCFF(invalidGlyph, 1, 16, 10); err == nil || outline != nil {
		t.Fatalf("malformed Type 2: outline=%#v err=%v", outline, err)
	}

	// glyf remains authoritative when present, including its own zero-upem
	// boundary rather than falling through to CFF.
	glyfZeroUPEM := &ownParsedFont{upem: 0, tables: map[string][]byte{"glyf": {}}}
	if bounds := glyfZeroUPEM.GlyphBounds(0, 16); bounds != (Rect{}) {
		t.Fatalf("zero-upem glyf bounds=%v", bounds)
	}
}
