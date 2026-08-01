package text

import (
	"fmt"
	"math"
)

// The limits below are deliberately modest. Type 2 programs are untrusted
// input and none of these limits is approached by normal desktop fonts.
const (
	type2MaxStack      = 48
	type2MaxCallDepth  = 16
	type2MaxOperations = 100000
	// Keep the geometry ceiling below the operation ceiling so every path
	// producer, including one-segment moveto programs, is independently
	// bounded by the segment budget.
	type2MaxSegments = 65536
)

type type2Result struct {
	segments []OutlineSegment
	width    float64
	hasWidth bool
}

type type2Frame struct {
	code     []byte
	pc       int
	subr     int
	isGlobal bool
	isSubr   bool
}

// decodeType2 executes one CFF1 Type 2 charstring in font units. The caller
// supplies the local subroutines selected by the glyph's FD and the font-wide
// global subroutines.
//
//nolint:gocognit,gocyclo,cyclop,funlen,gocritic,maintidx,nestif,revive // One bounded interpreter keeps the shared Type 2 operand/call state explicit across operators.
func decodeType2(charstring []byte, localSubrs, globalSubrs [][]byte, nominalWidth float64) (type2Result, error) {
	var result type2Result
	stack := make([]float64, 0, type2MaxStack)
	transient := [32]float64{}
	frames := []type2Frame{{code: charstring, subr: -1}}
	activeLocal := make(map[int]bool)
	activeGlobal := make(map[int]bool)
	var x, y float64
	stems := 0
	widthSeen := false
	operations := 0

	fail := func(format string, args ...any) (type2Result, error) {
		return type2Result{}, fmt.Errorf("text: Type 2 charstring: "+format, args...)
	}
	setWidth := func(n int) error {
		if widthSeen {
			return nil
		}
		widthSeen = true
		if len(stack) > n {
			if len(stack) != n+1 {
				return fmt.Errorf("invalid operand count %d", len(stack))
			}
			result.width = nominalWidth + stack[0]
			result.hasWidth = true
			stack = stack[1:]
		}
		return nil
	}
	setStemWidth := func() {
		if widthSeen {
			return
		}
		widthSeen = true
		if len(stack)%2 != 0 {
			result.width = nominalWidth + stack[0]
			result.hasWidth = true
			stack = stack[1:]
		}
	}
	clear := func() { stack = stack[:0] }
	exactInt := func(v float64, label string) (int, error) {
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v > float64(math.MaxInt) || v < float64(math.MinInt) {
			return 0, fmt.Errorf("%s %v is not an integer", label, v)
		}
		return int(v), nil
	}
	line := func(nx, ny float64) error {
		if math.IsNaN(nx) || math.IsNaN(ny) || math.IsInf(nx, 0) || math.IsInf(ny, 0) || math.Abs(nx) > math.MaxFloat32 || math.Abs(ny) > math.MaxFloat32 {
			return fmt.Errorf("non-finite line coordinate")
		}
		if len(result.segments) >= type2MaxSegments {
			return fmt.Errorf("segment limit exceeded")
		}
		x, y = nx, ny
		result.segments = append(result.segments, OutlineSegment{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: float32(x), Y: float32(y)}}})
		return nil
	}
	move := func(nx, ny float64) error {
		if math.IsNaN(nx) || math.IsNaN(ny) || math.IsInf(nx, 0) || math.IsInf(ny, 0) || math.Abs(nx) > math.MaxFloat32 || math.Abs(ny) > math.MaxFloat32 {
			return fmt.Errorf("non-finite move coordinate")
		}
		if len(result.segments) >= type2MaxSegments {
			return fmt.Errorf("segment limit exceeded")
		}
		x, y = nx, ny
		result.segments = append(result.segments, OutlineSegment{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: float32(x), Y: float32(y)}}})
		return nil
	}
	curve := func(dx1, dy1, dx2, dy2, dx3, dy3 float64) error {
		if len(result.segments) >= type2MaxSegments {
			return fmt.Errorf("segment limit exceeded")
		}
		p1x, p1y := x+dx1, y+dy1
		p2x, p2y := p1x+dx2, p1y+dy2
		nx, ny := p2x+dx3, p2y+dy3
		for _, v := range []float64{p1x, p1y, p2x, p2y, nx, ny} {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > math.MaxFloat32 {
				return fmt.Errorf("non-finite curve coordinate")
			}
		}
		p1 := OutlinePoint{X: float32(p1x), Y: float32(p1y)}
		p2 := OutlinePoint{X: float32(p2x), Y: float32(p2y)}
		x, y = nx, ny
		result.segments = append(result.segments, OutlineSegment{Op: OutlineOpCubicTo, Points: [3]OutlinePoint{p1, p2, {X: float32(x), Y: float32(y)}}})
		return nil
	}

	for len(frames) != 0 {
		fr := &frames[len(frames)-1]
		if fr.pc >= len(fr.code) {
			if fr.isSubr {
				return fail("subroutine %d ended without return", fr.subr)
			}
			return fail("charstring ended without endchar")
		}
		b := fr.code[fr.pc]
		if b == 28 || b == 255 || b >= 32 {
			v, next, err := readType2Number(fr.code, fr.pc)
			if err != nil {
				return fail("at byte %d: %v", fr.pc, err)
			}
			if len(stack) >= type2MaxStack {
				return fail("operand stack overflow")
			}
			stack = append(stack, v)
			fr.pc = next
			continue
		}
		fr.pc++
		operations++
		if operations > type2MaxOperations {
			return fail("operation limit exceeded")
		}
		op := int(b)
		if b == 12 {
			if fr.pc >= len(fr.code) {
				return fail("truncated escaped operator")
			}
			op = 1200 + int(fr.code[fr.pc])
			fr.pc++
		}

		need := func(n int) error {
			if len(stack) < n {
				return fmt.Errorf("operator %d: operand stack underflow", op)
			}
			return nil
		}
		binary := func(fn func(float64, float64) float64) error {
			if err := need(2); err != nil {
				return err
			}
			n := len(stack)
			stack[n-2] = fn(stack[n-2], stack[n-1])
			stack = stack[:n-1]
			return nil
		}
		var err error
		switch op {
		case 1, 3, 18, 23: // stem operators
			setStemWidth()
			if len(stack)%2 != 0 {
				err = fmt.Errorf("operator %d: odd stem operands", op)
			} else {
				stems += len(stack) / 2
				clear()
			}
		case 19, 20: // hintmask/cntrmask
			if !widthSeen {
				setStemWidth()
			}
			if err == nil {
				if len(stack)%2 != 0 {
					err = fmt.Errorf("operator %d: odd stem operands", op)
				} else {
					stems += len(stack) / 2
					clear()
					n := (stems + 7) / 8
					if fr.pc+n > len(fr.code) {
						err = fmt.Errorf("operator %d: truncated mask", op)
					} else {
						fr.pc += n
					}
				}
			}
		case 4: // vmoveto
			if err = setWidth(1); err == nil {
				if len(stack) != 1 {
					err = fmt.Errorf("vmoveto: want 1 operand")
				} else {
					err = move(x, y+stack[0])
					clear()
				}
			}
		case 21:
			if err = setWidth(2); err == nil {
				if len(stack) != 2 {
					err = fmt.Errorf("rmoveto: want 2 operands")
				} else {
					err = move(x+stack[0], y+stack[1])
					clear()
				}
			}
		case 22:
			if err = setWidth(1); err == nil {
				if len(stack) != 1 {
					err = fmt.Errorf("hmoveto: want 1 operand")
				} else {
					err = move(x+stack[0], y)
					clear()
				}
			}
		case 5:
			if len(stack) < 2 || len(stack)%2 != 0 {
				err = fmt.Errorf("rlineto: invalid operand count")
			} else {
				for i := 0; i < len(stack) && err == nil; i += 2 {
					err = line(x+stack[i], y+stack[i+1])
				}
				clear()
			}
		case 6, 7:
			if len(stack) < 1 {
				err = fmt.Errorf("operator %d: operand stack underflow", op)
			} else {
				horizontal := op == 6
				for _, d := range stack {
					if horizontal {
						err = line(x+d, y)
					} else {
						err = line(x, y+d)
					}
					if err != nil {
						break
					}
					horizontal = !horizontal
				}
				clear()
			}
		case 8:
			if len(stack) < 6 || len(stack)%6 != 0 {
				err = fmt.Errorf("rrcurveto: invalid operand count")
			} else {
				for i := 0; i < len(stack) && err == nil; i += 6 {
					err = curve(stack[i], stack[i+1], stack[i+2], stack[i+3], stack[i+4], stack[i+5])
				}
				clear()
			}
		case 24:
			if len(stack) < 8 || (len(stack)-2)%6 != 0 {
				err = fmt.Errorf("rcurveline: invalid operand count")
			} else {
				i := 0
				for i < len(stack)-2 && err == nil {
					err = curve(stack[i], stack[i+1], stack[i+2], stack[i+3], stack[i+4], stack[i+5])
					i += 6
				}
				if err == nil {
					err = line(x+stack[i], y+stack[i+1])
				}
				clear()
			}
		case 25:
			if len(stack) < 8 || (len(stack)-6)%2 != 0 {
				err = fmt.Errorf("rlinecurve: invalid operand count")
			} else {
				i := 0
				for i < len(stack)-6 && err == nil {
					err = line(x+stack[i], y+stack[i+1])
					i += 2
				}
				if err == nil {
					err = curve(stack[i], stack[i+1], stack[i+2], stack[i+3], stack[i+4], stack[i+5])
				}
				clear()
			}
		case 26, 27:
			if len(stack) < 4 {
				err = fmt.Errorf("operator %d: operand stack underflow", op)
			} else {
				i := 0
				extra := 0.0
				if len(stack)%4 == 1 {
					extra = stack[0]
					i = 1
				}
				if (len(stack)-i)%4 != 0 {
					err = fmt.Errorf("operator %d: invalid operand count", op)
				} else {
					for i < len(stack) && err == nil {
						if op == 26 {
							err = curve(extra, stack[i], stack[i+1], stack[i+2], 0, stack[i+3])
						} else {
							err = curve(stack[i], extra, stack[i+1], stack[i+2], stack[i+3], 0)
						}
						extra = 0
						i += 4
					}
				}
				clear()
			}
		case 30, 31:
			if len(stack) < 4 {
				err = fmt.Errorf("operator %d: operand stack underflow", op)
			} else {
				i := 0
				vertical := op == 30
				for i < len(stack) && err == nil {
					rem := len(stack) - i
					if rem < 4 {
						err = fmt.Errorf("operator %d: invalid operand count", op)
						break
					}
					extra := 0.0
					take := 4
					if rem == 5 {
						extra = stack[i+4]
						take = 5
					}
					if vertical {
						err = curve(0, stack[i], stack[i+1], stack[i+2], stack[i+3], extra)
					} else {
						err = curve(stack[i], 0, stack[i+1], stack[i+2], extra, stack[i+3])
					}
					i += take
					vertical = !vertical
				}
				clear()
			}
		case 10, 29: // callsubr/callgsubr
			if err = need(1); err == nil {
				n := len(stack)
				raw, intErr := exactInt(stack[n-1], "subroutine operand")
				stack = stack[:n-1]
				list := localSubrs
				global := false
				active := activeLocal
				if op == 29 {
					list = globalSubrs
					global = true
					active = activeGlobal
				}
				idx := raw + type2SubrBias(len(list))
				if intErr != nil {
					err = intErr
				} else if idx < 0 || idx >= len(list) {
					err = fmt.Errorf("operator %d: subroutine %d out of range", op, idx)
				} else if len(frames) >= type2MaxCallDepth {
					err = fmt.Errorf("subroutine recursion limit exceeded")
				} else if active[idx] {
					err = fmt.Errorf("subroutine cycle at %d", idx)
				} else {
					active[idx] = true
					frames = append(frames, type2Frame{code: list[idx], subr: idx, isGlobal: global, isSubr: true})
				}
			}
		case 11:
			if !fr.isSubr {
				err = fmt.Errorf("return outside subroutine")
			} else {
				if fr.isGlobal {
					delete(activeGlobal, fr.subr)
				} else {
					delete(activeLocal, fr.subr)
				}
				frames = frames[:len(frames)-1]
			}
		case 14:
			// Some valid CFF1 fonts tail-call a subroutine that owns endchar.
			// endchar terminates the entire glyph program, not only the current
			// call frame, so handle it identically at every call depth.
			if !widthSeen {
				if len(stack) == 1 || len(stack) == 5 {
					result.width = nominalWidth + stack[0]
					result.hasWidth = true
					stack = stack[1:]
				}
				widthSeen = true
			}
			if len(stack) != 0 && len(stack) != 4 {
				err = fmt.Errorf("endchar: invalid operand count %d", len(stack))
			} else if len(stack) == 4 {
				err = fmt.Errorf("endchar: deprecated seac operands unsupported")
			} else {
				frames = nil
				clear()
			}
		case 1200:
			clear() // dotsection, obsolete but harmless
		case 1203:
			err = binary(func(a, b float64) float64 {
				if a != 0 && b != 0 {
					return 1
				}
				return 0
			})
		case 1204:
			err = binary(func(a, b float64) float64 {
				if a != 0 || b != 0 {
					return 1
				}
				return 0
			})
		case 1205:
			if err = need(1); err == nil {
				if stack[len(stack)-1] == 0 {
					stack[len(stack)-1] = 1
				} else {
					stack[len(stack)-1] = 0
				}
			}
		case 1209:
			if err = need(1); err == nil {
				stack[len(stack)-1] = math.Abs(stack[len(stack)-1])
			}
		case 1210:
			err = binary(func(a, b float64) float64 { return a + b })
		case 1211:
			err = binary(func(a, b float64) float64 { return a - b })
		case 1212:
			if err = need(2); err == nil {
				if stack[len(stack)-1] == 0 {
					err = fmt.Errorf("division by zero")
				} else {
					err = binary(func(a, b float64) float64 { return a / b })
				}
			}
		case 1214:
			if err = need(1); err == nil {
				stack[len(stack)-1] = -stack[len(stack)-1]
			}
		case 1215:
			err = binary(func(a, b float64) float64 {
				if a == b {
					return 1
				}
				return 0
			})
		case 1218:
			if err = need(1); err == nil {
				stack = stack[:len(stack)-1]
			}
		case 1220:
			if err = need(2); err == nil {
				n := len(stack)
				i, intErr := exactInt(stack[n-1], "put index")
				v := stack[n-2]
				stack = stack[:n-2]
				if intErr != nil {
					err = intErr
				} else if i < 0 || i >= len(transient) {
					err = fmt.Errorf("put index %d out of range", i)
				} else {
					transient[i] = v
				}
			}
		case 1221:
			if err = need(1); err == nil {
				i, intErr := exactInt(stack[len(stack)-1], "get index")
				if intErr != nil {
					err = intErr
				} else if i < 0 || i >= len(transient) {
					err = fmt.Errorf("get index %d out of range", i)
				} else {
					stack[len(stack)-1] = transient[i]
				}
			}
		case 1222:
			if err = need(4); err == nil {
				n := len(stack)
				v1, v2, s1, s2 := stack[n-4], stack[n-3], stack[n-2], stack[n-1]
				stack = stack[:n-4]
				if s1 <= s2 {
					stack = append(stack, v1)
				} else {
					stack = append(stack, v2)
				}
			}
		case 1223:
			if len(stack) >= type2MaxStack {
				err = fmt.Errorf("operand stack overflow")
			} else {
				stack = append(stack, 0.5)
			} // deterministic random
		case 1224:
			err = binary(func(a, b float64) float64 { return a * b })
		case 1226:
			if err = need(1); err == nil {
				if stack[len(stack)-1] < 0 {
					err = fmt.Errorf("sqrt of negative value")
				} else {
					stack[len(stack)-1] = math.Sqrt(stack[len(stack)-1])
				}
			}
		case 1227:
			if err = need(1); err == nil {
				if len(stack) >= type2MaxStack {
					err = fmt.Errorf("operand stack overflow")
				} else {
					stack = append(stack, stack[len(stack)-1])
				}
			}
		case 1228:
			if err = need(2); err == nil {
				n := len(stack)
				stack[n-1], stack[n-2] = stack[n-2], stack[n-1]
			}
		case 1229:
			if err = need(1); err == nil {
				n := len(stack)
				i, intErr := exactInt(stack[n-1], "index operand")
				stack = stack[:n-1]
				if intErr != nil {
					err = intErr
				} else {
					if i < 0 {
						i = 0
					}
					if i >= len(stack) {
						i = len(stack) - 1
					}
					if i < 0 {
						err = fmt.Errorf("index on empty stack")
						break
					}
					// index consumes its own operand before duplicating, so a valid
					// input stack always has room for the result.
					stack = append(stack, stack[len(stack)-1-i])
				}
			}
		case 1230:
			if err = need(2); err == nil {
				n := len(stack)
				count, countErr := exactInt(stack[n-2], "roll count")
				j, jErr := exactInt(stack[n-1], "roll shift")
				stack = stack[:n-2]
				if countErr != nil {
					err = countErr
				} else if jErr != nil {
					err = jErr
				} else if count < 0 || count > len(stack) {
					err = fmt.Errorf("roll count %d out of range", count)
				} else if count > 0 {
					j %= count
					if j < 0 {
						j += count
					}
					base := len(stack) - count
					tmp := append([]float64(nil), stack[base:]...)
					for k := 0; k < count; k++ {
						stack[base+(k+j)%count] = tmp[k]
					}
				}
			}
		case 1234: // hflex
			if len(stack) != 7 {
				err = fmt.Errorf("hflex: want 7 operands")
			} else {
				err = curve(stack[0], 0, stack[1], stack[2], stack[3], 0)
				if err == nil {
					err = curve(stack[4], 0, stack[5], -stack[2], stack[6], 0)
				}
				clear()
			}
		case 1235:
			if len(stack) != 13 {
				err = fmt.Errorf("flex: want 13 operands")
			} else {
				err = curve(stack[0], stack[1], stack[2], stack[3], stack[4], stack[5])
				if err == nil {
					err = curve(stack[6], stack[7], stack[8], stack[9], stack[10], stack[11])
				}
				clear()
			}
		case 1236:
			if len(stack) != 9 {
				err = fmt.Errorf("hflex1: want 9 operands")
			} else {
				dy := stack[1] + stack[3] + stack[7]
				err = curve(stack[0], stack[1], stack[2], stack[3], stack[4], 0)
				if err == nil {
					err = curve(stack[5], 0, stack[6], stack[7], stack[8], -dy)
				}
				clear()
			}
		case 1237:
			if len(stack) != 11 {
				err = fmt.Errorf("flex1: want 11 operands")
			} else {
				dx := stack[0] + stack[2] + stack[4] + stack[6] + stack[8]
				dy := stack[1] + stack[3] + stack[5] + stack[7] + stack[9]
				d6x, d6y := 0.0, 0.0
				if math.Abs(dx) > math.Abs(dy) {
					d6x = stack[10]
					d6y = -dy
				} else {
					d6x = -dx
					d6y = stack[10]
				}
				err = curve(stack[0], stack[1], stack[2], stack[3], stack[4], stack[5])
				if err == nil {
					err = curve(stack[6], stack[7], stack[8], stack[9], d6x, d6y)
				}
				clear()
			}
		default:
			err = fmt.Errorf("reserved or unsupported operator %d", op)
		}
		if err != nil {
			return fail("at operator byte %d: %v", fr.pc-1, err)
		}
	}
	return result, nil
}

func type2SubrBias(n int) int {
	if n < 1240 {
		return 107
	}
	if n < 33900 {
		return 1131
	}
	return 32768
}

func readType2Number(data []byte, pos int) (float64, int, error) {
	if pos >= len(data) {
		return 0, pos, fmt.Errorf("truncated number")
	}
	b := data[pos]
	switch {
	case b >= 32 && b <= 246:
		return float64(int(b) - 139), pos + 1, nil
	case b >= 247 && b <= 250:
		if pos+1 >= len(data) {
			return 0, pos, fmt.Errorf("truncated positive number")
		}
		return float64((int(b)-247)*256 + int(data[pos+1]) + 108), pos + 2, nil
	case b >= 251 && b <= 254:
		if pos+1 >= len(data) {
			return 0, pos, fmt.Errorf("truncated negative number")
		}
		return float64(-(int(b)-251)*256 - int(data[pos+1]) - 108), pos + 2, nil
	case b == 28:
		if pos+3 > len(data) {
			return 0, pos, fmt.Errorf("truncated short number")
		}
		return float64(int16(uint16(data[pos+1])<<8 | uint16(data[pos+2]))), pos + 3, nil
	case b == 255:
		if pos+5 > len(data) {
			return 0, pos, fmt.Errorf("truncated fixed number")
		}
		u := uint32(data[pos+1])<<24 | uint32(data[pos+2])<<16 | uint32(data[pos+3])<<8 | uint32(data[pos+4])
		return float64(int32(u)) / 65536, pos + 5, nil
	default:
		return 0, pos, fmt.Errorf("byte %d is not a number", b)
	}
}
