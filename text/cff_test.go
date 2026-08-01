package text

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"testing"
)

func cffIndexForTest(objects [][]byte, offSize int) []byte {
	if len(objects) == 0 {
		return []byte{0, 0}
	}
	payloadLen := 0
	for _, object := range objects {
		payloadLen += len(object)
	}
	headerLen := 3 + (len(objects)+1)*offSize
	b := make([]byte, headerLen, headerLen+payloadLen)
	binary.BigEndian.PutUint16(b, uint16(len(objects)))
	b[2] = byte(offSize)
	pos := 1
	write := func(at, v int) {
		for j := offSize - 1; j >= 0; j-- {
			b[at+j] = byte(v)
			v >>= 8
		}
	}
	at := 3
	write(at, pos)
	at += offSize
	for _, o := range objects {
		pos += len(o)
		write(at, pos)
		at += offSize
	}
	for _, o := range objects {
		b = append(b, o...)
	}
	return b
}
func cffLong(v int) []byte {
	return []byte{29, byte(uint32(v) >> 24), byte(uint32(v) >> 16), byte(uint32(v) >> 8), byte(v)}
}

func syntheticCFF1() []byte {
	name := cffIndexForTest([][]byte{[]byte("T")}, 1)
	strings := cffIndexForTest(nil, 1)
	globals := cffIndexForTest([][]byte{t2prog(300, 0, byte(5), byte(11))}, 1)
	g0 := []byte{14}
	g1 := t2prog(100, 200, byte(21), -107, byte(29), 0, 400, -300, 0, byte(5), byte(14))
	chars := cffIndexForTest([][]byte{g0, g1}, 1)
	// The fixed-width long DICT operand makes the offset independent of value.
	topLen := 2 + 1 + 2 + 6
	charOff := 4 + len(name) + topLen + len(strings) + len(globals)
	topObj := append(cffLong(charOff), 17)
	top := cffIndexForTest([][]byte{topObj}, 1)
	b := make([]byte, 0, 4+len(name)+len(top)+len(strings)+len(globals)+len(chars))
	b = append(b, 1, 0, 4, 1)
	b = append(b, name...)
	b = append(b, top...)
	b = append(b, strings...)
	b = append(b, globals...)
	b = append(b, chars...)
	return b
}

func syntheticCIDCFF(fdSelectFormat byte) []byte {
	name := cffIndexForTest([][]byte{[]byte("C")}, 1)
	strings, globals := cffIndexForTest(nil, 1), cffIndexForTest(nil, 1)
	chars := cffIndexForTest([][]byte{{14}, t2prog(0, 0, byte(21), -107, byte(10), byte(14))}, 1)
	topObjectLen := 5 + 6 + 7 + 7 // ROS, CharStrings, FDArray, FDSelect.
	prefixLen := 4 + len(name) + (2 + 1 + 2 + topObjectLen) + len(strings) + len(globals)
	charOff := prefixLen
	fdArrayOff := charOff + len(chars)
	var fdSelect []byte
	if fdSelectFormat == 0 {
		fdSelect = []byte{0, 0, 1}
	} else {
		fdSelect = []byte{3, 0, 2, 0, 0, 0, 0, 1, 1, 0, 2}
	}
	// Each FD DICT is seven bytes (size, long offset, Private operator), so
	// the two-entry INDEX is twenty bytes. Each four-byte Private DICT points
	// to the Local Subrs INDEX immediately following it.
	private0Off := fdArrayOff + 20 + len(fdSelect)
	private0 := []byte{143, 19, 149, 21} // Subrs=+4, nominalWidthX=10.
	local0 := cffIndexForTest([][]byte{{11}}, 1)
	private1Off := private0Off + len(private0) + len(local0)
	private1 := []byte{143, 19, 159, 21} // Subrs=+4, nominalWidthX=20.
	local1 := cffIndexForTest([][]byte{{11}, {11}}, 1)
	fd0 := append([]byte{143}, cffLong(private0Off)...)
	fd0 = append(fd0, 18)
	fd1 := append([]byte{143}, cffLong(private1Off)...)
	fd1 = append(fd1, 18)
	fdArray := cffIndexForTest([][]byte{fd0, fd1}, 1)
	fdSelectOff := fdArrayOff + len(fdArray)
	topObj := make([]byte, 0, topObjectLen)
	topObj = append(topObj, 139, 139, 139, 12, 30)
	topObj = append(topObj, cffLong(charOff)...)
	topObj = append(topObj, 17)
	topObj = append(topObj, cffLong(fdArrayOff)...)
	topObj = append(topObj, 12, 36)
	topObj = append(topObj, cffLong(fdSelectOff)...)
	topObj = append(topObj, 12, 37)
	top := cffIndexForTest([][]byte{topObj}, 1)
	b := []byte{1, 0, 4, 1}
	for _, part := range [][]byte{name, top, strings, globals, chars, fdArray, fdSelect, private0, local0, private1, local1} {
		b = append(b, part...)
	}
	return b
}

func TestCFFIndexOffSizesAndMalformed(t *testing.T) {
	for offSize := 1; offSize <= 4; offSize++ {
		t.Run(fmt.Sprintf("offSize%d", offSize), func(t *testing.T) {
			raw := cffIndexForTest([][]byte{[]byte("a"), []byte("bc")}, offSize)
			got, next, err := parseCFFIndex(raw, 0, "test")
			if err != nil {
				t.Fatal(err)
			}
			if next != len(raw) || string(got[0]) != "a" || string(got[1]) != "bc" {
				t.Fatalf("got=%q next=%d", got, next)
			}
		})
	}
	bad := [][]byte{{0}, {0, 1}, {0, 1, 0}, {0, 1, 1, 2, 1}, {0, 1, 1, 1, 5, 0}}
	for i, raw := range bad {
		if _, _, err := parseCFFIndex(raw, 0, "bad"); err == nil {
			t.Errorf("bad case %d accepted", i)
		}
	}
}

func TestCFFDictNumbersAndReal(t *testing.T) {
	// -1, 108, int16(-32768), int32(65536), real 1.5, then operator 17.
	raw := []byte{138, 247, 0, 28, 0x80, 0, 29, 0, 1, 0, 0, 30, 0x1a, 0x5f, 17}
	d, err := parseCFFDict(raw)
	if err != nil {
		t.Fatal(err)
	}
	v := d[17]
	want := []float64{-1, 108, -32768, 65536, 1.5}
	if len(v) != len(want) {
		t.Fatalf("values=%v", v)
	}
	for i := range want {
		if v[i] != want[i] {
			t.Errorf("value %d=%v want %v", i, v[i], want[i])
		}
	}
	for _, raw := range [][]byte{{28, 1}, {29, 0}, {30, 0x1a}, {12}, {31}} {
		if _, err := parseCFFDict(raw); err == nil {
			t.Errorf("accepted malformed DICT %v", raw)
		}
	}
}

func TestCFFHelperBoundaryValidation(t *testing.T) {
	for name, data := range map[string][]byte{
		"header size":    {1, 0, 3, 1},
		"header offSize": {1, 0, 4, 0},
		"empty name":     append([]byte{1, 0, 4, 1}, cffIndexForTest(nil, 1)...),
		"two names":      append([]byte{1, 0, 4, 1}, cffIndexForTest([][]byte{[]byte("A"), []byte("B")}, 1)...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCFF1(data); err == nil {
				t.Fatal("malformed CFF accepted")
			}
		})
	}

	validName := cffIndexForTest([][]byte{[]byte("A")}, 1)
	for name, top := range map[string][]byte{
		"empty top":      cffIndexForTest(nil, 1),
		"two top dicts":  cffIndexForTest([][]byte{{17}, {17}}, 1),
		"malformed dict": cffIndexForTest([][]byte{{31}}, 1),
	} {
		t.Run(name, func(t *testing.T) {
			data := append([]byte{1, 0, 4, 1}, validName...)
			data = append(data, top...)
			if _, err := parseCFF1(data); err == nil {
				t.Fatal("malformed Top DICT accepted")
			}
		})
	}

	if value, next, err := readCFFDictNumber([]byte{251, 0}, 0); err != nil || value != -108 || next != 2 {
		t.Fatalf("negative DICT number = (%g,%d,%v), want (-108,2,nil)", value, next, err)
	}
	for name, raw := range map[string][]byte{
		"positive truncated": {247},
		"negative truncated": {251},
		"reserved nibble":    {30, 0xd0},
		"invalid byte":       {31},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := readCFFDictNumber(raw, 0); err == nil {
				t.Fatal("malformed DICT number accepted")
			}
		})
	}
	for name, raw := range map[string][]byte{
		"exponent":          {30, 0x1b, 0x2f},
		"negative exponent": {30, 0x1c, 0x2f},
		"minus":             {30, 0xe1, 0xf0},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := readCFFDictNumber(raw, 0); err != nil {
				t.Fatal(err)
			}
		})
	}

	for _, value := range []float64{math.NaN(), math.Inf(1), 1.5} {
		if _, err := cffExactInt(value); err == nil {
			t.Errorf("cffExactInt(%v) accepted", value)
		}
	}
	if _, err := dictOneInt(map[int][]float64{}, 17, true); err == nil {
		t.Fatal("missing required DICT operator accepted")
	}
	if value, err := dictOneInt(map[int][]float64{}, 17, false); err != nil || value != 0 {
		t.Fatalf("optional DICT operator = (%d,%v), want (0,nil)", value, err)
	}
	if _, err := dictOneInt(map[int][]float64{17: {1, 2}}, 17, true); err == nil {
		t.Fatal("multi-operand singleton operator accepted")
	}
	if _, err := dictOneInt(map[int][]float64{17: {-1}}, 17, true); err == nil {
		t.Fatal("negative DICT offset accepted")
	}

	private, err := parseCFFPrivate([]byte{140, 20, 141, 21}, 4, 0)
	if err != nil || private.defaultWidth != 1 || private.nominalWidth != 2 {
		t.Fatalf("private=%#v err=%v", private, err)
	}
	for name, data := range map[string][]byte{
		"bad dict":        {31},
		"default arity":   {139, 140, 20},
		"nominal arity":   {139, 140, 21},
		"subrs out range": {239, 19},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCFFPrivate(data, len(data), 0); err == nil {
				t.Fatal("malformed Private DICT accepted")
			}
		})
	}

	if err := parseCFFCharset([]byte{0}, 0, 1); err != nil {
		t.Fatalf("single-glyph predefined charset: %v", err)
	}
	if err := parseCFFCharset(nil, -1, 2); err == nil {
		t.Fatal("negative charset offset accepted")
	}
	if err := parseCFFEncoding(nil, -1); err == nil {
		t.Fatal("negative encoding offset accepted")
	}

	for name, test := range map[string]struct {
		data    []byte
		nGlyphs int
		nFDs    int
	}{
		"offset":           {[]byte{0}, 1, 1},
		"format0 fd":       {[]byte{0, 2}, 1, 2},
		"format3 count":    {[]byte{3}, 1, 1},
		"format3 fd":       {[]byte{3, 0, 1, 0, 0, 2, 0, 1}, 1, 2},
		"format3 order":    {[]byte{3, 0, 2, 0, 0, 0, 0, 0, 1, 0, 2}, 2, 2},
		"format3 sentinel": {[]byte{3, 0, 1, 0, 0, 0}, 1, 1},
		"format3 glyphs":   {[]byte{3, 0, 1, 0, 0, 0, 0, 1}, 2, 1},
	} {
		t.Run("fdselect "+name, func(t *testing.T) {
			offset := 0
			if name == "offset" {
				offset = 1
			}
			if _, err := parseCFFFDSelect(test.data, offset, test.nGlyphs, test.nFDs); err == nil {
				t.Fatal("malformed FDSelect accepted")
			}
		})
	}

	for name, raw := range map[string][]byte{
		"operand overflow": append(make([]byte, 49), 17),
		"trailing operand": {139},
	} {
		if name == "operand overflow" {
			for i := range raw[:49] {
				raw[i] = 139
			}
		}
		t.Run("dict "+name, func(t *testing.T) {
			if _, err := parseCFFDict(raw); err == nil {
				t.Fatal("malformed DICT accepted")
			}
		})
	}

	for name, font := range map[string]*cffFont{
		"missing selection": {charStrings: [][]byte{{14}}, fdSelect: nil, fds: []cffPrivate{{}}},
		"fd out of range":   {charStrings: [][]byte{{14}}, fdSelect: []uint16{1}, fds: []cffPrivate{{}}},
	} {
		t.Run("glyph "+name, func(t *testing.T) {
			if _, _, _, err := font.glyph(0); err == nil {
				t.Fatal("invalid CID glyph selection accepted")
			}
		})
	}
}

func TestCFFOrdinaryParseAndGlyph(t *testing.T) {
	f, err := parseCFF1(syntheticCFF1())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.charStrings) != 2 {
		t.Fatalf("glyph count=%d", len(f.charStrings))
	}
	cs, global, p, err := f.glyph(1)
	if err != nil {
		t.Fatal(err)
	}
	r, err := decodeType2(cs, p.localSubrs, global, p.nominalWidth)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.segments) != 4 {
		t.Fatalf("segments=%#v", r.segments)
	}
	if _, _, _, err = f.glyph(2); err == nil {
		t.Fatal("expected glyph range error")
	}
	bad := syntheticCFF1()
	bad[0] = 2
	if _, err = parseCFF1(bad); err == nil {
		t.Fatal("CFF2 major version accepted")
	}
	for n := 0; n < len(syntheticCFF1()); n++ {
		if _, err := parseCFF1(syntheticCFF1()[:n]); err == nil {
			t.Fatalf("truncation at %d accepted", n)
		}
	}
}

func TestCFFFDSelectFormats(t *testing.T) {
	f0 := []byte{0, 0, 1, 1}
	got, err := parseCFFFDSelect(f0, 0, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != "[0 1 1]" {
		t.Fatalf("format0=%v", got)
	}
	f3 := []byte{3, 0, 2, 0, 0, 0, 0, 2, 1, 0, 4}
	got, err = parseCFFFDSelect(f3, 0, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != "[0 0 1 1]" {
		t.Fatalf("format3=%v", got)
	}
	for _, bad := range [][]byte{{2}, {0, 2}, {3, 0, 0}, {3, 0, 1, 0, 1, 0, 0, 2}} {
		if _, err := parseCFFFDSelect(bad, 0, 2, 2); err == nil {
			t.Errorf("accepted malformed FDSelect %v", bad)
		}
	}
	// Selection must carry the per-FD local subroutine set and widths.
	c := &cffFont{charStrings: [][]byte{{14}, {14}}, fdSelect: []uint16{0, 1}, fds: []cffPrivate{{nominalWidth: 10, localSubrs: [][]byte{{11}}}, {nominalWidth: 20, localSubrs: [][]byte{{11}, {11}}}}}
	_, _, p, err := c.glyph(1)
	if err != nil || p.nominalWidth != 20 || len(p.localSubrs) != 2 {
		t.Fatalf("selected private=%#v err=%v", p, err)
	}
	for _, format := range []byte{0, 3} {
		parsed, err := parseCFF1(syntheticCIDCFF(format))
		if err != nil {
			t.Fatalf("CID format %d: %v", format, err)
		}
		_, _, private, err := parsed.glyph(1)
		if err != nil || private.nominalWidth != 20 || len(private.localSubrs) != 2 {
			t.Fatalf("CID format %d selected %#v, %v", format, private, err)
		}
		if _, err := decodeType2(parsed.charStrings[1], private.localSubrs, parsed.globalSubrs, private.nominalWidth); err != nil {
			t.Fatalf("CID format %d decode: %v", format, err)
		}
	}
}

func TestCFFCharsetEncodingAndPrivateBounds(t *testing.T) {
	for name, data := range map[string][]byte{
		"charset0": {0, 0, 1, 0, 2},
		"charset1": {1, 0, 1, 1},
		"charset2": {2, 0, 1, 0, 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := parseCFFCharset(data, 0, 3); err != nil {
				t.Fatal(err)
			}
		})
	}
	for name, data := range map[string][]byte{
		"encoding0":  {0, 2, 65, 66},
		"encoding1":  {1, 1, 65, 1},
		"supplement": {0x80, 1, 65, 1, 66, 0, 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := parseCFFEncoding(data, 0); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, data := range [][]byte{{3}, {0, 0}, {1, 0, 1}, {2, 0, 1, 0}} {
		if err := parseCFFCharset(data, 0, 3); err == nil {
			t.Errorf("accepted malformed charset %v", data)
		}
	}
	for _, data := range [][]byte{{2}, {0, 2, 65}, {1, 1, 65}, {0x80, 0}} {
		if err := parseCFFEncoding(data, 0); err == nil {
			t.Errorf("accepted malformed encoding %v", data)
		}
	}
	if _, err := parseCFFPrivate([]byte{1, 2}, 3, 0); err == nil {
		t.Fatal("accepted out-of-range Private DICT")
	}
}

func syntheticCFFOTF() []byte {
	tables := map[string][]byte{}
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000)
	tables["head"] = head
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint32(maxp[:4], 0x00005000)
	binary.BigEndian.PutUint16(maxp[4:], 2)
	tables["maxp"] = maxp
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:], 2)
	tables["hhea"] = hhea
	hmtx := make([]byte, 8)
	binary.BigEndian.PutUint16(hmtx[0:2], 500)
	binary.BigEndian.PutUint16(hmtx[4:6], 700)
	binary.BigEndian.PutUint16(hmtx[6:8], int16bits(50))
	tables["hmtx"] = hmtx
	cmap := make([]byte, 12+28)
	binary.BigEndian.PutUint16(cmap[2:4], 1)
	binary.BigEndian.PutUint16(cmap[4:6], 3)
	binary.BigEndian.PutUint16(cmap[6:8], 10)
	binary.BigEndian.PutUint32(cmap[8:12], 12)
	binary.BigEndian.PutUint16(cmap[12:14], 12)
	binary.BigEndian.PutUint32(cmap[16:20], 28)
	binary.BigEndian.PutUint32(cmap[24:28], 1)
	binary.BigEndian.PutUint32(cmap[28:32], 65)
	binary.BigEndian.PutUint32(cmap[32:36], 65)
	binary.BigEndian.PutUint32(cmap[36:40], 1)
	tables["cmap"] = cmap
	tables["CFF "] = syntheticCFF1()
	tags := []string{"CFF ", "cmap", "head", "hhea", "hmtx", "maxp"}
	header := 12 + 16*len(tags)
	total := header
	for _, tag := range tags {
		total = (total + 3) &^ 3
		total += len(tables[tag])
	}
	out := make([]byte, total)
	copy(out[:4], "OTTO")
	binary.BigEndian.PutUint16(out[4:6], uint16(len(tags)))
	off := header
	for i, tag := range tags {
		off = (off + 3) &^ 3
		rec := 12 + i*16
		copy(out[rec:rec+4], tag)
		binary.BigEndian.PutUint32(out[rec+8:rec+12], uint32(off))
		binary.BigEndian.PutUint32(out[rec+12:rec+16], uint32(len(tables[tag])))
		copy(out[off:], tables[tag])
		off += len(tables[tag])
	}
	return out
}
func int16bits(v int16) uint16 { return uint16(v) }

func TestOutlineExtractionCFF(t *testing.T) {
	parsed, err := getParser(defaultParserName).Parse(syntheticCFFOTF())
	if err != nil {
		t.Fatal(err)
	}
	if gid := parsed.GlyphIndex('A'); gid != 1 {
		t.Fatalf("gid=%d", gid)
	}
	e := NewOutlineExtractor()
	for _, size := range []float64{10, 20} {
		o, err := e.ExtractOutline(parsed, 1, size)
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Segments) != 4 || o.Advance != float32(700*size/1000) || o.LSB != float32(50*size/1000) {
			t.Fatalf("size %v outline=%#v", size, o)
		}
		wantBounds := Rect{MinX: 0.1 * size, MinY: -0.6 * size, MaxX: 0.4 * size, MaxY: -0.2 * size}
		if math.Abs(o.Bounds.MinX-wantBounds.MinX) > 1e-5 || math.Abs(o.Bounds.MinY-wantBounds.MinY) > 1e-5 || math.Abs(o.Bounds.MaxX-wantBounds.MaxX) > 1e-5 || math.Abs(o.Bounds.MaxY-wantBounds.MaxY) > 1e-5 {
			t.Errorf("size %v bounds=%+.12v want=%+.12v", size, o.Bounds, wantBounds)
		}
		if got := parsed.GlyphBounds(1, size); got != o.Bounds {
			t.Errorf("GlyphBounds=%v outline=%v", got, o.Bounds)
		}
		if _, err = e.ExtractOutlineHinted(parsed, 1, size, HintingVertical); err != nil {
			t.Fatal(err)
		}
		if _, err = e.ExtractOutlineHintedVar(parsed, 1, size, HintingNone, []FontVariation{{Tag: [4]byte{'w', 'g', 'h', 't'}, Value: 700}}); err != nil {
			t.Fatal(err)
		}
	}
	rasterized := RasterizeGlyph(parsed, 1, 32)
	if rasterized == nil || rasterized.Mask == nil {
		t.Fatal("public RasterizeGlyph returned no CFF mask")
	}
	nonEmpty := false
	for _, alpha := range rasterized.Mask.Pix {
		if alpha != 0 {
			nonEmpty = true
			break
		}
	}
	if !nonEmpty {
		t.Fatal("public RasterizeGlyph returned an empty CFF mask")
	}
	const goroutines = 16
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, e := NewOutlineExtractor().ExtractOutline(parsed, 1, 18)
			if e != nil {
				errs <- e
			} else if len(o.Segments) != 4 {
				errs <- fmt.Errorf("got %d segments", len(o.Segments))
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestOutlineCFF2UnsupportedAndNoOutline(t *testing.T) {
	base := syntheticCFFOTF()
	tables, err := parseFontTables(base)
	if err != nil {
		t.Fatal(err)
	}
	f := &ownParsedFont{tables: map[string][]byte{"CFF2": {2, 0, 5, 0, 0}}, rawData: base, upem: 1000, numGlyphs: 2}
	if _, err := NewOutlineExtractor().ExtractOutline(f, 1, 12); err == nil {
		t.Fatal("expected contextual CFF2 unsupported error")
	}
	f = &ownParsedFont{tables: tables, rawData: base, upem: 1000, numGlyphs: 2}
	delete(f.tables, "CFF ")
	o, err := NewOutlineExtractor().ExtractOutline(f, 1, 12)
	if err != nil || o != nil {
		t.Fatalf("missing outline got %#v, %v", o, err)
	}
}

func TestCFFOutlineErrorBoundaries(t *testing.T) {
	valid := syntheticCFF1()
	for name, font := range map[string]*ownParsedFont{
		"zero upem":     {upem: 0, numGlyphs: 2, tables: map[string][]byte{"CFF ": valid}},
		"missing table": {upem: 1000, numGlyphs: 2, tables: map[string][]byte{}},
		"malformed CFF": {upem: 1000, numGlyphs: 2, tables: map[string][]byte{"CFF ": {1}}},
		"glyph range":   {upem: 1000, numGlyphs: 2, tables: map[string][]byte{"CFF ": valid}},
		"empty glyph":   {upem: 1000, numGlyphs: 2, tables: map[string][]byte{"CFF ": valid}},
	} {
		t.Run(name, func(t *testing.T) {
			gid := uint16(1)
			if name == "glyph range" {
				gid = 9
			}
			if name == "empty glyph" {
				gid = 0
			}
			if bounds := font.cffGlyphBounds(gid, 16); bounds != (Rect{}) {
				t.Fatalf("bounds=%v, want empty", bounds)
			}
		})
	}

	mismatch := &ownParsedFont{
		numGlyphs: 3,
		tables:    map[string][]byte{"CFF ": valid},
	}
	if _, err := mismatch.loadCFF(); err == nil {
		t.Fatal("CharStrings/maxp glyph-count mismatch accepted")
	}

	extractor := NewOutlineExtractor()
	for name, font := range map[string]*ownParsedFont{
		"missing CFF": {upem: 1000, numGlyphs: 2, tables: map[string][]byte{}},
		"invalid CFF": {upem: 1000, numGlyphs: 2, tables: map[string][]byte{"CFF ": {1}}},
	} {
		t.Run("extract "+name, func(t *testing.T) {
			outline, err := extractor.extractCFF(font, 1, 16, 10)
			if name == "missing CFF" {
				if err != nil || outline != nil {
					t.Fatalf("outline=%#v err=%v, want nil nil", outline, err)
				}
				return
			}
			if err == nil || outline != nil {
				t.Fatalf("outline=%#v err=%v, want contextual error", outline, err)
			}
		})
	}

	if bounds := outlineSegmentBounds(nil); bounds != (Rect{}) {
		t.Fatalf("empty outline bounds=%v", bounds)
	}
}

func BenchmarkCFFOutlineCold(b *testing.B) {
	data := syntheticCFFOTF()
	b.ReportAllocs()
	for b.Loop() {
		p, err := getParser(defaultParserName).Parse(data)
		if err != nil {
			b.Fatal(err)
		}
		if _, err = NewOutlineExtractor().ExtractOutline(p, 1, 16); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCFFOutlineParsed(b *testing.B) {
	p, err := getParser(defaultParserName).Parse(syntheticCFFOTF())
	if err != nil {
		b.Fatal(err)
	}
	e := NewOutlineExtractor()
	if _, err = e.ExtractOutline(p, 1, 16); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err = e.ExtractOutline(p, 1, 16); err != nil {
			b.Fatal(err)
		}
	}
}
