package svg

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/gogpu/gg"
)

// transformMatrix parses an SVG transform list into a matrix. The returned
// matrix includes all valid transforms before an error, preserving the
// renderer's historical best-effort handling of malformed lists.
//
// Supported transform functions:
//   - translate(tx [, ty])
//   - rotate(angle [, cx, cy]) — angle in degrees
//   - scale(sx [, sy])
//   - matrix(a, b, c, d, e, f)
func transformMatrix(transform string) (gg.Matrix, error) {
	m := gg.Identity()
	transform = strings.TrimSpace(transform)
	if transform == "" {
		return m, nil
	}

	// Parse sequence of transform functions: "translate(1 3) rotate(-45 3 3)"
	pos := 0
	for pos < len(transform) {
		// Skip whitespace.
		for pos < len(transform) && isSpace(transform[pos]) {
			pos++
		}
		if pos >= len(transform) {
			break
		}

		// Read function name.
		nameStart := pos
		for pos < len(transform) && transform[pos] != '(' && !isSpace(transform[pos]) {
			pos++
		}
		name := transform[nameStart:pos]

		// Skip whitespace before '('.
		for pos < len(transform) && isSpace(transform[pos]) {
			pos++
		}
		if pos >= len(transform) || transform[pos] != '(' {
			return m, fmt.Errorf("svg: expected '(' after transform function %q", name)
		}
		pos++ // skip '('

		// Find closing ')'.
		parenStart := pos
		for pos < len(transform) && transform[pos] != ')' {
			pos++
		}
		if pos >= len(transform) {
			return m, fmt.Errorf("svg: missing ')' in transform %q", name)
		}
		argsStr := transform[parenStart:pos]
		pos++ // skip ')'

		// Parse args.
		args, err := parseTransformArgs(argsStr)
		if err != nil {
			return m, fmt.Errorf("svg: transform %s: %w", name, err)
		}

		part, err := transformFuncMatrix(name, args)
		if err != nil {
			return m, err
		}
		m = m.Multiply(part)
	}
	return m, nil
}

// transformFuncMatrix returns one SVG transform function as a gg matrix.
func transformFuncMatrix(name string, args []float64) (gg.Matrix, error) {
	switch name {
	case "translate":
		if len(args) < 1 {
			return gg.Identity(), fmt.Errorf("svg: translate requires at least 1 arg, got %d", len(args))
		}
		tx := args[0]
		ty := 0.0
		if len(args) >= 2 {
			ty = args[1]
		}
		return gg.Translate(tx, ty), nil

	case "rotate":
		switch {
		case len(args) == 1:
			// rotate(angle) — angle in degrees, about origin
			return gg.Rotate(args[0] * math.Pi / 180.0), nil
		case len(args) >= 3:
			angle := args[0] * math.Pi / 180.0
			return gg.Translate(args[1], args[2]).Multiply(gg.Rotate(angle)).Multiply(gg.Translate(-args[1], -args[2])), nil
		default:
			return gg.Identity(), fmt.Errorf("svg: rotate requires 1 or 3 args, got %d", len(args))
		}

	case "scale":
		if len(args) < 1 {
			return gg.Identity(), fmt.Errorf("svg: scale requires at least 1 arg, got %d", len(args))
		}
		sx := args[0]
		sy := sx
		if len(args) >= 2 {
			sy = args[1]
		}
		return gg.Scale(sx, sy), nil

	case "matrix":
		if len(args) != 6 {
			return gg.Identity(), fmt.Errorf("svg: matrix requires 6 args, got %d", len(args))
		}
		// SVG matrix(a,b,c,d,e,f): x' = a*x + c*y + e, y' = b*x + d*y + f
		// gg Matrix{A,B,C,D,E,F}: x' = A*x + B*y + C, y' = D*x + E*y + F
		return gg.Matrix{
			A: args[0], B: args[2], C: args[4],
			D: args[1], E: args[3], F: args[5],
		}, nil

	case "skewX":
		if len(args) != 1 {
			return gg.Identity(), fmt.Errorf("svg: skewX requires 1 arg, got %d", len(args))
		}
		angle := args[0] * math.Pi / 180.0
		return gg.Matrix{
			A: 1, B: math.Tan(angle), C: 0,
			D: 0, E: 1, F: 0,
		}, nil

	case "skewY":
		if len(args) != 1 {
			return gg.Identity(), fmt.Errorf("svg: skewY requires 1 arg, got %d", len(args))
		}
		angle := args[0] * math.Pi / 180.0
		return gg.Matrix{
			A: 1, B: 0, C: 0,
			D: math.Tan(angle), E: 1, F: 0,
		}, nil

	default:
		return gg.Identity(), fmt.Errorf("svg: unsupported transform function %q", name)
	}
}

// parseTransformArgs splits a comma/space-separated argument string into float64 values.
func parseTransformArgs(s string) ([]float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	// Replace commas with spaces, then split on whitespace.
	s = strings.ReplaceAll(s, ",", " ")
	parts := strings.Fields(s)

	args := make([]float64, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", p, err)
		}
		args = append(args, v)
	}
	return args, nil
}

// isSpace returns true for ASCII whitespace characters.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}
