package text

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type cffPrivate struct {
	localSubrs   [][]byte
	defaultWidth float64
	nominalWidth float64
}

// cffFont contains only immutable data required to execute CFF1 glyphs.
type cffFont struct {
	charStrings [][]byte
	globalSubrs [][]byte
	private     cffPrivate
	fdSelect    []uint16
	fds         []cffPrivate
}

//nolint:gocognit,gocyclo,cyclop,funlen,nestif // CFF parsing is a linear, bounded validation pipeline; splitting it would obscure offset ownership.
func parseCFF1(data []byte) (*cffFont, error) {
	fail := func(format string, args ...any) (*cffFont, error) {
		return nil, fmt.Errorf("text: CFF1: "+format, args...)
	}
	if len(data) < 4 {
		return fail("truncated header")
	}
	if data[0] != 1 {
		return fail("unsupported major version %d (CFF2 is unsupported)", data[0])
	}
	hdrSize := int(data[2])
	if hdrSize < 4 || hdrSize > len(data) {
		return fail("invalid header size %d", hdrSize)
	}
	if data[3] < 1 || data[3] > 4 {
		return fail("invalid header offSize %d", data[3])
	}
	pos := hdrSize
	names, pos, err := parseCFFIndex(data, pos, "Name INDEX")
	if err != nil {
		return fail("%v", err)
	}
	if len(names) != 1 {
		return fail("Name INDEX has %d entries, want 1", len(names))
	}
	tops, pos, err := parseCFFIndex(data, pos, "Top DICT INDEX")
	if err != nil {
		return fail("%v", err)
	}
	if len(tops) != 1 {
		return fail("Top DICT INDEX has %d entries, want 1", len(tops))
	}
	top, err := parseCFFDict(tops[0])
	if err != nil {
		return fail("Top DICT: %v", err)
	}
	_, pos, err = parseCFFIndex(data, pos, "String INDEX")
	if err != nil {
		return fail("%v", err)
	}
	global, _, err := parseCFFIndex(data, pos, "Global Subrs INDEX")
	if err != nil {
		return fail("%v", err)
	}

	charOff, err := dictOneInt(top, 17, true)
	if err != nil {
		return fail("CharStrings: %v", err)
	}
	charStrings, _, err := parseCFFIndex(data, charOff, "CharStrings INDEX")
	if err != nil {
		return fail("%v", err)
	}
	if len(charStrings) == 0 {
		return fail("empty CharStrings INDEX")
	}

	f := &cffFont{charStrings: charStrings, globalSubrs: global}
	if vals, ok := top[18]; ok {
		if len(vals) != 2 {
			return fail("Private DICT requires size and offset")
		}
		size, sizeErr := cffExactInt(vals[0])
		off, offErr := cffExactInt(vals[1])
		if sizeErr != nil || offErr != nil {
			return fail("Private DICT has non-integer size or offset")
		}
		f.private, err = parseCFFPrivate(data, size, off)
		if err != nil {
			return fail("Top Private DICT: %v", err)
		}
	}
	_, cid := top[1230] // ROS
	if cid {
		if len(top[1230]) != 3 {
			return fail("ROS requires Registry, Ordering, and Supplement")
		}
		fdArrayOff, e := dictOneInt(top, 1236, true)
		if e != nil {
			return fail("FDArray: %v", e)
		}
		fdSelectOff, e := dictOneInt(top, 1237, true)
		if e != nil {
			return fail("FDSelect: %v", e)
		}
		fdDicts, _, e := parseCFFIndex(data, fdArrayOff, "FDArray INDEX")
		if e != nil {
			return fail("%v", e)
		}
		if len(fdDicts) == 0 {
			return fail("empty FDArray INDEX")
		}
		f.fds = make([]cffPrivate, len(fdDicts))
		for i, raw := range fdDicts {
			d, e := parseCFFDict(raw)
			if e != nil {
				return fail("FDArray DICT %d: %v", i, e)
			}
			vals, ok := d[18]
			if !ok || len(vals) != 2 {
				return fail("FDArray DICT %d missing valid Private", i)
			}
			size, sizeErr := cffExactInt(vals[0])
			off, offErr := cffExactInt(vals[1])
			if sizeErr != nil || offErr != nil {
				return fail("FDArray DICT %d Private has non-integer size or offset", i)
			}
			f.fds[i], e = parseCFFPrivate(data, size, off)
			if e != nil {
				return fail("FDArray DICT %d Private: %v", i, e)
			}
		}
		f.fdSelect, e = parseCFFFDSelect(data, fdSelectOff, len(charStrings), len(f.fds))
		if e != nil {
			return fail("FDSelect: %v", e)
		}
	}
	if off, ok := dictOptionalInt(top, 15); ok && off > 2 {
		if err = parseCFFCharset(data, off, len(charStrings)); err != nil {
			return fail("charset: %v", err)
		}
	}
	if off, ok := dictOptionalInt(top, 16); ok && off > 1 && !cid {
		if err = parseCFFEncoding(data, off); err != nil {
			return fail("Encoding: %v", err)
		}
	}
	return f, nil
}

func (f *cffFont) glyph(gid uint16) ([]byte, [][]byte, cffPrivate, error) {
	if int(gid) >= len(f.charStrings) {
		return nil, nil, cffPrivate{}, fmt.Errorf("text: CFF1: glyph ID %d out of range", gid)
	}
	p := f.private
	if len(f.fds) != 0 {
		if int(gid) >= len(f.fdSelect) {
			return nil, nil, cffPrivate{}, fmt.Errorf("text: CFF1: missing FD selection for glyph %d", gid)
		}
		fd := int(f.fdSelect[gid])
		if fd < 0 || fd >= len(f.fds) {
			return nil, nil, cffPrivate{}, fmt.Errorf("text: CFF1: FD %d out of range", fd)
		}
		p = f.fds[fd]
	}
	return f.charStrings[gid], f.globalSubrs, p, nil
}

func parseCFFIndex(data []byte, pos int, label string) ([][]byte, int, error) {
	if pos < 0 || pos+2 > len(data) {
		return nil, pos, fmt.Errorf("%s truncated count", label)
	}
	count := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2
	if count == 0 {
		return nil, pos, nil
	}
	if pos >= len(data) {
		return nil, pos, fmt.Errorf("%s truncated offSize", label)
	}
	offSize := int(data[pos])
	pos++
	if offSize < 1 || offSize > 4 {
		return nil, pos, fmt.Errorf("%s invalid offSize %d", label, offSize)
	}
	if count > 65535 || count+1 > (len(data)-pos)/offSize {
		return nil, pos, fmt.Errorf("%s truncated offsets", label)
	}
	offs := make([]uint32, count+1)
	for i := range offs {
		var v uint32
		for j := 0; j < offSize; j++ {
			v = v<<8 | uint32(data[pos])
			pos++
		}
		offs[i] = v
	}
	if offs[0] != 1 {
		return nil, pos, fmt.Errorf("%s first offset is %d, want 1", label, offs[0])
	}
	last := uint64(offs[count])
	if last < 1 {
		return nil, pos, fmt.Errorf("%s invalid final offset", label)
	}
	dataLen := last - 1
	if dataLen > uint64(len(data)-pos) {
		return nil, pos, fmt.Errorf("%s data truncated", label)
	}
	objects := make([][]byte, count)
	for i := range count {
		if offs[i] < 1 || offs[i] > offs[i+1] || uint64(offs[i+1]) > last {
			return nil, pos, fmt.Errorf("%s invalid offsets at entry %d", label, i)
		}
		start := pos + int(offs[i]-1)
		end := pos + int(offs[i+1]-1)
		objects[i] = data[start:end]
	}
	return objects, pos + int(dataLen), nil
}

func parseCFFDict(data []byte) (map[int][]float64, error) {
	out := make(map[int][]float64)
	operands := make([]float64, 0, 16)
	for pos := 0; pos < len(data); {
		b := data[pos]
		if b >= 32 || b == 28 || b == 29 || b == 30 {
			v, next, err := readCFFDictNumber(data, pos)
			if err != nil {
				return nil, err
			}
			if len(operands) >= 48 {
				return nil, fmt.Errorf("operand stack overflow")
			}
			operands = append(operands, v)
			pos = next
			continue
		}
		pos++
		op := int(b)
		if b == 12 {
			if pos >= len(data) {
				return nil, fmt.Errorf("truncated escaped operator")
			}
			op = 1200 + int(data[pos])
			pos++
		}
		if b == 31 || b == 255 {
			return nil, fmt.Errorf("reserved operator byte %d", b)
		}
		vals := append([]float64(nil), operands...)
		out[op] = vals
		operands = operands[:0]
	}
	if len(operands) != 0 {
		return nil, fmt.Errorf("trailing operands without operator")
	}
	return out, nil
}

//nolint:gocognit,gocyclo,cyclop // The branches directly mirror CFF DICT's finite numeric encodings.
func readCFFDictNumber(data []byte, pos int) (float64, int, error) {
	if pos >= len(data) {
		return 0, pos, fmt.Errorf("truncated number")
	}
	b := data[pos]
	switch {
	case b >= 32 && b <= 246:
		return float64(int(b) - 139), pos + 1, nil
	case b >= 247 && b <= 250:
		if pos+2 > len(data) {
			return 0, pos, fmt.Errorf("truncated positive integer")
		}
		return float64((int(b)-247)*256 + int(data[pos+1]) + 108), pos + 2, nil
	case b >= 251 && b <= 254:
		if pos+2 > len(data) {
			return 0, pos, fmt.Errorf("truncated negative integer")
		}
		return float64(-(int(b)-251)*256 - int(data[pos+1]) - 108), pos + 2, nil
	case b == 28:
		if pos+3 > len(data) {
			return 0, pos, fmt.Errorf("truncated short integer")
		}
		return float64(int16(binary.BigEndian.Uint16(data[pos+1 : pos+3]))), pos + 3, nil
	case b == 29:
		if pos+5 > len(data) {
			return 0, pos, fmt.Errorf("truncated long integer")
		}
		return float64(int32(binary.BigEndian.Uint32(data[pos+1 : pos+5]))), pos + 5, nil
	case b == 30:
		var s strings.Builder
		done := false
		for i := pos + 1; i < len(data); i++ {
			for _, n := range []byte{data[i] >> 4, data[i] & 15} {
				switch n {
				case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9:
					s.WriteByte('0' + n)
				case 10:
					s.WriteByte('.')
				case 11:
					s.WriteByte('E')
				case 12:
					s.WriteString("E-")
				case 14:
					s.WriteByte('-')
				case 15:
					done = true
				default:
					return 0, pos, fmt.Errorf("reserved real nibble %d", n)
				}
				if done {
					v, e := strconv.ParseFloat(s.String(), 64)
					if e != nil {
						return 0, pos, fmt.Errorf("invalid real %q", s.String())
					}
					return v, i + 1, nil
				}
			}
		}
		return 0, pos, fmt.Errorf("truncated real")
	default:
		return 0, pos, fmt.Errorf("byte %d is not a DICT number", b)
	}
}

func cffExactInt(v float64) (int, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v > float64(math.MaxInt) || v < float64(math.MinInt) || v != math.Trunc(v) {
		return 0, fmt.Errorf("%v is not an integer", v)
	}
	return int(v), nil
}
func dictOneInt(d map[int][]float64, op int, required bool) (int, error) {
	v, ok := d[op]
	if !ok {
		if required {
			return 0, fmt.Errorf("missing operator %d", op)
		}
		return 0, nil
	}
	if len(v) != 1 {
		return 0, fmt.Errorf("operator %d has %d operands", op, len(v))
	}
	n, err := cffExactInt(v[0])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("operator %d has invalid offset %v", op, v[0])
	}
	return n, nil
}
func dictOptionalInt(d map[int][]float64, op int) (int, bool) {
	v, ok := d[op]
	if !ok || len(v) != 1 {
		return 0, false
	}
	n, err := cffExactInt(v[0])
	return n, err == nil && n >= 0
}

func parseCFFPrivate(data []byte, size, off int) (cffPrivate, error) {
	var p cffPrivate
	if size < 0 || off < 0 || off > len(data) || size > len(data)-off {
		return p, fmt.Errorf("range [%d,%d) outside table", off, off+size)
	}
	d, err := parseCFFDict(data[off : off+size])
	if err != nil {
		return p, err
	}
	if v, ok := d[20]; ok {
		if len(v) != 1 {
			return p, fmt.Errorf("defaultWidthX has %d operands", len(v))
		}
		p.defaultWidth = v[0]
	}
	if v, ok := d[21]; ok {
		if len(v) != 1 {
			return p, fmt.Errorf("nominalWidthX has %d operands", len(v))
		}
		p.nominalWidth = v[0]
	}
	if rel, ok := dictOptionalInt(d, 19); ok {
		absolute := off + rel
		if absolute < off || absolute > len(data) {
			return p, fmt.Errorf("subrs offset outside table")
		}
		p.localSubrs, _, err = parseCFFIndex(data, absolute, "Local Subrs INDEX")
		if err != nil {
			return p, err
		}
	}
	return p, nil
}

//nolint:gocognit,gocyclo,cyclop // Keeping formats 0 and 3 together makes their shared bounds invariant explicit.
func parseCFFFDSelect(data []byte, off, nGlyphs, nFDs int) ([]uint16, error) {
	if off < 0 || off >= len(data) {
		return nil, fmt.Errorf("offset %d outside table", off)
	}
	format := data[off]
	off++
	out := make([]uint16, nGlyphs)
	switch format {
	case 0:
		if nGlyphs > len(data)-off {
			return nil, fmt.Errorf("format 0 truncated")
		}
		for i := range nGlyphs {
			fd := int(data[off+i])
			if fd >= nFDs {
				return nil, fmt.Errorf("glyph %d selects FD %d out of range", i, fd)
			}
			out[i] = uint16(fd)
		}
	case 3:
		if off+2 > len(data) {
			return nil, fmt.Errorf("format 3 truncated range count")
		}
		n := int(binary.BigEndian.Uint16(data[off : off+2]))
		off += 2
		if n == 0 || n > (len(data)-off)/3 {
			return nil, fmt.Errorf("format 3 truncated ranges")
		}
		firsts := make([]int, n)
		fds := make([]int, n)
		for i := range n {
			firsts[i] = int(binary.BigEndian.Uint16(data[off : off+2]))
			fds[i] = int(data[off+2])
			off += 3
			if fds[i] >= nFDs {
				return nil, fmt.Errorf("range %d selects FD %d out of range", i, fds[i])
			}
			if i == 0 && firsts[i] != 0 {
				return nil, fmt.Errorf("first range starts at glyph %d", firsts[i])
			}
			if i > 0 && firsts[i] <= firsts[i-1] {
				return nil, fmt.Errorf("ranges not increasing")
			}
		}
		if off+2 > len(data) {
			return nil, fmt.Errorf("format 3 missing sentinel")
		}
		sentinel := int(binary.BigEndian.Uint16(data[off : off+2]))
		if sentinel != nGlyphs {
			return nil, fmt.Errorf("sentinel %d does not equal glyph count %d", sentinel, nGlyphs)
		}
		for i := range n {
			end := sentinel
			if i+1 < n {
				end = firsts[i+1]
			}
			for g := firsts[i]; g < end; g++ {
				out[g] = uint16(fds[i])
			}
		}
	default:
		return nil, fmt.Errorf("unsupported format %d", format)
	}
	return out, nil
}

func parseCFFCharset(data []byte, off, nGlyphs int) error {
	if off < 0 || off >= len(data) {
		return fmt.Errorf("offset outside table")
	}
	if nGlyphs <= 1 {
		return nil
	}
	format := data[off]
	off++
	left := nGlyphs - 1
	switch format {
	case 0:
		if left > (len(data)-off)/2 {
			return fmt.Errorf("format 0 truncated")
		}
		return nil
	case 1, 2:
		for left > 0 {
			need := 3
			if format == 2 {
				need = 4
			}
			if off+need > len(data) {
				return fmt.Errorf("format %d truncated", format)
			}
			var n int
			if format == 1 {
				n = int(data[off+2])
			} else {
				n = int(binary.BigEndian.Uint16(data[off+2 : off+4]))
			}
			if n+1 > left {
				return fmt.Errorf("format %d range exceeds glyph count", format)
			}
			left -= n + 1
			off += need
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %d", format)
	}
}
func parseCFFEncoding(data []byte, off int) error {
	if off < 0 || off >= len(data) {
		return fmt.Errorf("offset outside table")
	}
	format := data[off]
	off++
	base := format & 0x7f
	switch base {
	case 0:
		if off >= len(data) {
			return fmt.Errorf("format 0 truncated")
		}
		n := int(data[off])
		off++
		if n > len(data)-off {
			return fmt.Errorf("format 0 codes truncated")
		}
		off += n
	case 1:
		if off >= len(data) {
			return fmt.Errorf("format 1 truncated")
		}
		n := int(data[off])
		off++
		if n > (len(data)-off)/2 {
			return fmt.Errorf("format 1 ranges truncated")
		}
		off += n * 2
	default:
		return fmt.Errorf("unsupported format %d", base)
	}
	if format&0x80 != 0 {
		if off >= len(data) {
			return fmt.Errorf("supplements truncated")
		}
		n := int(data[off])
		off++
		if n > (len(data)-off)/3 {
			return fmt.Errorf("supplements truncated")
		}
	}
	return nil
}
