package gg

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseSVGPath parses an SVG path data string (d attribute) into a Path.
// It supports all SVG path commands: M/m, L/l, H/h, V/v, C/c, S/s, Q/q, T/t, A/a, Z/z.
// Uppercase commands use absolute coordinates; lowercase use relative coordinates
// (offsets from the current point).
//
// The parser follows the SVG 1.1 specification for path data grammar, including:
//   - Comma and whitespace separation between numbers
//   - Implicit repeat of commands when extra coordinate pairs are provided
//   - Implicit LineTo after MoveTo with extra coordinate pairs
//   - Negative sign as number separator (e.g., "10-20" means "10, -20")
//   - Scientific notation in numbers (e.g., "1e-5")
//   - Flag values (0 or 1) in arc commands without separators
//
// Reference: https://www.w3.org/TR/SVG11/paths.html#PathData
func ParseSVGPath(d string) (*Path, error) {
	p := &svgParser{
		input: d,
		path:  NewPath(),
	}
	if err := p.parse(); err != nil {
		return nil, err
	}
	return p.path, nil
}

// svgParser is a state machine for parsing SVG path data.
type svgParser struct {
	input string
	pos   int
	path  *Path

	// Current point in absolute coordinates.
	cx, cy float64

	// Start of current subpath (for Z command).
	sx, sy float64

	// Previous control point for smooth curves (S/s, T/t).
	// These store the last control point used, in absolute coords.
	prevCmdType byte // 'C','c','S','s','Q','q','T','t' or 0
	prevCtrlX   float64
	prevCtrlY   float64
}

func (p *svgParser) parse() error {
	p.skipWhitespace()
	if p.pos >= len(p.input) {
		// Empty path is valid — returns an empty Path.
		return nil
	}

	for p.pos < len(p.input) {
		p.skipWhitespace()
		if p.pos >= len(p.input) {
			break
		}

		ch := p.input[p.pos]
		if !isCommandChar(ch) {
			return fmt.Errorf("gg: ParseSVGPath: expected command at position %d, got %q", p.pos, string(ch))
		}
		p.pos++

		if err := p.executeCommand(ch); err != nil {
			return err
		}
	}
	return nil
}

func (p *svgParser) executeCommand(cmd byte) error {
	switch cmd {
	case 'M', 'm':
		return p.parseMoveTo(cmd == 'm')
	case 'L', 'l':
		return p.parseLineTo(cmd == 'l')
	case 'H', 'h':
		return p.parseHorizontalLine(cmd == 'h')
	case 'V', 'v':
		return p.parseVerticalLine(cmd == 'v')
	case 'C', 'c':
		return p.parseCubicBezier(cmd == 'c')
	case 'S', 's':
		return p.parseSmoothCubic(cmd == 's')
	case 'Q', 'q':
		return p.parseQuadBezier(cmd == 'q')
	case 'T', 't':
		return p.parseSmoothQuad(cmd == 't')
	case 'A', 'a':
		return p.parseArc(cmd == 'a')
	case 'Z', 'z':
		p.path.Close()
		p.cx = p.sx
		p.cy = p.sy
		p.prevCmdType = 0
		return nil
	default:
		return fmt.Errorf("gg: ParseSVGPath: unknown command %q", string(cmd))
	}
}

func (p *svgParser) parseMoveTo(relative bool) error {
	x, y, err := p.readCoordPair()
	if err != nil {
		return fmt.Errorf("gg: ParseSVGPath: MoveTo: %w", err)
	}
	if relative {
		x += p.cx
		y += p.cy
	}
	p.path.MoveTo(x, y)
	p.cx = x
	p.cy = y
	p.sx = x
	p.sy = y
	p.prevCmdType = 0

	// Implicit LineTo for subsequent coordinate pairs after MoveTo.
	for p.hasMoreNumbers() {
		x, y, err = p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: implicit LineTo after MoveTo: %w", err)
		}
		if relative {
			x += p.cx
			y += p.cy
		}
		p.path.LineTo(x, y)
		p.cx = x
		p.cy = y
	}
	return nil
}

func (p *svgParser) parseLineTo(relative bool) error {
	for {
		x, y, err := p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: LineTo: %w", err)
		}
		if relative {
			x += p.cx
			y += p.cy
		}
		p.path.LineTo(x, y)
		p.cx = x
		p.cy = y
		p.prevCmdType = 0

		if !p.hasMoreNumbers() {
			break
		}
	}
	return nil
}

func (p *svgParser) parseHorizontalLine(relative bool) error {
	for {
		x, err := p.readNumber()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: HorizontalLine: %w", err)
		}
		if relative {
			x += p.cx
		}
		p.path.LineTo(x, p.cy)
		p.cx = x
		p.prevCmdType = 0

		if !p.hasMoreNumbers() {
			break
		}
	}
	return nil
}

func (p *svgParser) parseVerticalLine(relative bool) error {
	for {
		y, err := p.readNumber()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: VerticalLine: %w", err)
		}
		if relative {
			y += p.cy
		}
		p.path.LineTo(p.cx, y)
		p.cy = y
		p.prevCmdType = 0

		if !p.hasMoreNumbers() {
			break
		}
	}
	return nil
}

func (p *svgParser) parseCubicBezier(relative bool) error {
	for {
		c1x, c1y, err := p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: CubicBezier: control1: %w", err)
		}
		c2x, c2y, err := p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: CubicBezier: control2: %w", err)
		}
		x, y, err := p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: CubicBezier: endpoint: %w", err)
		}
		if relative {
			c1x += p.cx
			c1y += p.cy
			c2x += p.cx
			c2y += p.cy
			x += p.cx
			y += p.cy
		}
		p.path.CubicTo(c1x, c1y, c2x, c2y, x, y)
		p.prevCtrlX = c2x
		p.prevCtrlY = c2y
		p.prevCmdType = 'C'
		p.cx = x
		p.cy = y

		if !p.hasMoreNumbers() {
			break
		}
	}
	return nil
}

func (p *svgParser) parseSmoothCubic(relative bool) error {
	for {
		// Reflect previous control point.
		c1x, c1y := p.reflectControlPoint()

		c2x, c2y, err := p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: SmoothCubic: control2: %w", err)
		}
		x, y, err := p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: SmoothCubic: endpoint: %w", err)
		}
		if relative {
			c2x += p.cx
			c2y += p.cy
			x += p.cx
			y += p.cy
		}
		p.path.CubicTo(c1x, c1y, c2x, c2y, x, y)
		p.prevCtrlX = c2x
		p.prevCtrlY = c2y
		p.prevCmdType = 'S'
		p.cx = x
		p.cy = y

		if !p.hasMoreNumbers() {
			break
		}
	}
	return nil
}

func (p *svgParser) parseQuadBezier(relative bool) error {
	for {
		qx, qy, err := p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: QuadBezier: control: %w", err)
		}
		x, y, err := p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: QuadBezier: endpoint: %w", err)
		}
		if relative {
			qx += p.cx
			qy += p.cy
			x += p.cx
			y += p.cy
		}
		p.path.QuadraticTo(qx, qy, x, y)
		p.prevCtrlX = qx
		p.prevCtrlY = qy
		p.prevCmdType = 'Q'
		p.cx = x
		p.cy = y

		if !p.hasMoreNumbers() {
			break
		}
	}
	return nil
}

func (p *svgParser) parseSmoothQuad(relative bool) error {
	for {
		qx, qy := p.reflectControlPointQuad()

		x, y, err := p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: SmoothQuad: endpoint: %w", err)
		}
		if relative {
			x += p.cx
			y += p.cy
		}
		p.path.QuadraticTo(qx, qy, x, y)
		p.prevCtrlX = qx
		p.prevCtrlY = qy
		p.prevCmdType = 'T'
		p.cx = x
		p.cy = y

		if !p.hasMoreNumbers() {
			break
		}
	}
	return nil
}

func (p *svgParser) parseArc(relative bool) error {
	for {
		rx, err := p.readNumber()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: Arc: rx: %w", err)
		}
		ry, err := p.readNumber()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: Arc: ry: %w", err)
		}
		xRot, err := p.readNumber()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: Arc: x-rotation: %w", err)
		}
		largeArc, err := p.readFlag()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: Arc: large-arc-flag: %w", err)
		}
		sweep, err := p.readFlag()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: Arc: sweep-flag: %w", err)
		}
		x, y, err := p.readCoordPair()
		if err != nil {
			return fmt.Errorf("gg: ParseSVGPath: Arc: endpoint: %w", err)
		}
		if relative {
			x += p.cx
			y += p.cy
		}

		p.arcToCubics(rx, ry, xRot, largeArc, sweep, x, y)
		p.cx = x
		p.cy = y
		p.prevCmdType = 0

		if !p.hasMoreNumbers() {
			break
		}
	}
	return nil
}

// reflectControlPoint returns the reflection of the previous control point
// for smooth cubic (S/s) commands. If the previous command was not C/c/S/s,
// the control point equals the current point.
func (p *svgParser) reflectControlPoint() (float64, float64) {
	switch p.prevCmdType {
	case 'C', 'S':
		return 2*p.cx - p.prevCtrlX, 2*p.cy - p.prevCtrlY
	default:
		return p.cx, p.cy
	}
}

// reflectControlPointQuad returns the reflection of the previous control point
// for smooth quadratic (T/t) commands. If the previous command was not Q/q/T/t,
// the control point equals the current point.
func (p *svgParser) reflectControlPointQuad() (float64, float64) {
	switch p.prevCmdType {
	case 'Q', 'T':
		return 2*p.cx - p.prevCtrlX, 2*p.cy - p.prevCtrlY
	default:
		return p.cx, p.cy
	}
}

// arcToCubics converts an SVG arc to one or more cubic Bezier curves.
// This implements the W3C SVG spec F.6.5 endpoint-to-center parameterization,
// then approximates each arc segment (up to pi/2 radians) with a cubic Bezier.
//
// Reference: https://www.w3.org/TR/SVG11/implnote.html#ArcConversionEndpointToCenter
func (p *svgParser) arcToCubics(rx, ry, xRotDeg float64, largeArc, sweep bool, x2, y2 float64) {
	x1 := p.cx
	y1 := p.cy

	// F.6.2: If the endpoints are identical, skip the arc.
	if x1 == x2 && y1 == y2 {
		return
	}

	// F.6.6: If rx or ry is zero, draw a straight line.
	rx = math.Abs(rx)
	ry = math.Abs(ry)
	if rx == 0 || ry == 0 {
		p.path.LineTo(x2, y2)
		return
	}

	xRot := xRotDeg * math.Pi / 180.0
	cosRot := math.Cos(xRot)
	sinRot := math.Sin(xRot)

	// F.6.5.1: Compute (x1', y1') — midpoint in rotated coordinate system.
	dx := (x1 - x2) / 2.0
	dy := (y1 - y2) / 2.0
	x1p := cosRot*dx + sinRot*dy
	y1p := -sinRot*dx + cosRot*dy

	// F.6.6.2: Ensure radii are large enough.
	x1pSq := x1p * x1p
	y1pSq := y1p * y1p
	rxSq := rx * rx
	rySq := ry * ry

	lambda := x1pSq/rxSq + y1pSq/rySq
	if lambda > 1.0 {
		scale := math.Sqrt(lambda)
		rx *= scale
		ry *= scale
		rxSq = rx * rx
		rySq = ry * ry
	}

	// F.6.5.2: Compute (cx', cy') — center in rotated coordinate system.
	num := rxSq*rySq - rxSq*y1pSq - rySq*x1pSq
	den := rxSq*y1pSq + rySq*x1pSq
	sq := 0.0
	if den > 0 {
		sq = math.Sqrt(math.Max(0, num/den))
	}
	if largeArc == sweep {
		sq = -sq
	}
	cxp := sq * rx * y1p / ry
	cyp := -sq * ry * x1p / rx

	// F.6.5.3: Compute (cx, cy) from (cx', cy').
	cx := cosRot*cxp - sinRot*cyp + (x1+x2)/2.0
	cy := sinRot*cxp + cosRot*cyp + (y1+y2)/2.0

	// F.6.5.5 & F.6.5.6: Compute start angle and sweep angle.
	ux := (x1p - cxp) / rx
	uy := (y1p - cyp) / ry
	vx := (-x1p - cxp) / rx
	vy := (-y1p - cyp) / ry

	theta1 := svgAngle(1, 0, ux, uy)
	dtheta := svgAngle(ux, uy, vx, vy)

	if !sweep && dtheta > 0 {
		dtheta -= 2 * math.Pi
	} else if sweep && dtheta < 0 {
		dtheta += 2 * math.Pi
	}

	// Split into segments of at most pi/2 and approximate each with a cubic.
	numSegments := int(math.Ceil(math.Abs(dtheta) / (math.Pi / 2.0)))
	if numSegments == 0 {
		numSegments = 1
	}
	segAngle := dtheta / float64(numSegments)

	for i := 0; i < numSegments; i++ {
		a1 := theta1 + float64(i)*segAngle
		a2 := a1 + segAngle
		p.arcSegmentToCubic(cx, cy, rx, ry, cosRot, sinRot, a1, a2)
	}
}

// arcSegmentToCubic converts a single arc segment (<=pi/2) to a cubic Bezier.
// The arc is on the unit circle from angle a1 to a2, scaled by (rx, ry) and rotated.
func (p *svgParser) arcSegmentToCubic(cx, cy, rx, ry, cosRot, sinRot, a1, a2 float64) {
	// Standard cubic Bezier approximation for a circular arc.
	// alpha = 4 * tan(sweep/4) / 3
	halfSweep := (a2 - a1) / 2.0
	alpha := math.Sin(halfSweep) * (math.Sqrt(4+3*math.Tan(halfSweep)*math.Tan(halfSweep)) - 1) / 3.0

	cos1 := math.Cos(a1)
	sin1 := math.Sin(a1)
	cos2 := math.Cos(a2)
	sin2 := math.Sin(a2)

	// Endpoints and control points on the unit ellipse.
	// P1 = (rx*cos1, ry*sin1), P2 = (rx*cos2, ry*sin2)
	// CP1 = P1 + alpha * tangent at P1
	// CP2 = P2 - alpha * tangent at P2
	p1x := rx * cos1
	p1y := ry * sin1
	p2x := rx * cos2
	p2y := ry * sin2

	cp1x := p1x - alpha*rx*sin1
	cp1y := p1y + alpha*ry*cos1
	cp2x := p2x + alpha*rx*sin2
	cp2y := p2y - alpha*ry*cos2

	// Apply rotation and translation.
	c1x := cosRot*cp1x - sinRot*cp1y + cx
	c1y := sinRot*cp1x + cosRot*cp1y + cy
	c2x := cosRot*cp2x - sinRot*cp2y + cx
	c2y := sinRot*cp2x + cosRot*cp2y + cy
	ex := cosRot*p2x - sinRot*p2y + cx
	ey := sinRot*p2x + cosRot*p2y + cy

	p.path.CubicTo(c1x, c1y, c2x, c2y, ex, ey)
}

// svgAngle computes the angle between two vectors (ux, uy) and (vx, vy).
// The result is in [-pi, pi].
func svgAngle(ux, uy, vx, vy float64) float64 {
	dot := ux*vx + uy*vy
	lenU := math.Sqrt(ux*ux + uy*uy)
	lenV := math.Sqrt(vx*vx + vy*vy)
	cosA := dot / (lenU * lenV)
	// Clamp to [-1, 1] to avoid NaN from acos.
	cosA = math.Max(-1, math.Min(1, cosA))
	angle := math.Acos(cosA)
	if ux*vy-uy*vx < 0 {
		angle = -angle
	}
	return angle
}

// ---- Tokenizer ----

func (p *svgParser) skipWhitespace() {
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == ',' {
			p.pos++
		} else {
			break
		}
	}
}

// skipSeparator skips optional whitespace and at most one comma.
func (p *svgParser) skipSeparator() {
	p.skipWhitespace()
	if p.pos < len(p.input) && p.input[p.pos] == ',' {
		p.pos++
		p.skipWhitespace()
	}
}

// hasMoreNumbers returns true if the next non-whitespace character
// could be the start of a number (digit, dot, sign, or 'e'/'E').
func (p *svgParser) hasMoreNumbers() bool {
	saved := p.pos
	p.skipSeparator()
	if p.pos >= len(p.input) {
		p.pos = saved
		return false
	}
	ch := p.input[p.pos]
	ok := ch == '.' || ch == '-' || ch == '+' || (ch >= '0' && ch <= '9')
	p.pos = saved
	return ok
}

// readNumber reads a single floating-point number from the input.
func (p *svgParser) readNumber() (float64, error) {
	p.skipSeparator()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of input, expected number")
	}

	start := p.pos
	p.skipSign()

	hasDigit := p.scanDigits()
	hasDigit = p.scanFraction() || hasDigit

	if !hasDigit {
		return 0, fmt.Errorf("expected number at position %d", start)
	}

	if err := p.scanExponent(); err != nil {
		return 0, err
	}

	val, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q at position %d: %w", p.input[start:p.pos], start, err)
	}
	return val, nil
}

// skipSign advances past an optional '+' or '-'.
func (p *svgParser) skipSign() {
	if p.pos < len(p.input) && (p.input[p.pos] == '+' || p.input[p.pos] == '-') {
		p.pos++
	}
}

// scanDigits consumes a run of digits and reports whether any were found.
func (p *svgParser) scanDigits() bool {
	found := false
	for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
		p.pos++
		found = true
	}
	return found
}

// scanFraction consumes a decimal point followed by digits.
// Returns true if any digit was found after the dot.
func (p *svgParser) scanFraction() bool {
	if p.pos >= len(p.input) || p.input[p.pos] != '.' {
		return false
	}
	p.pos++ // skip '.'
	return p.scanDigits()
}

// scanExponent consumes an optional exponent part (e.g., "e-5", "E+3").
func (p *svgParser) scanExponent() error {
	if p.pos >= len(p.input) {
		return nil
	}
	if p.input[p.pos] != 'e' && p.input[p.pos] != 'E' {
		return nil
	}
	p.pos++
	p.skipSign()
	if p.pos >= len(p.input) || p.input[p.pos] < '0' || p.input[p.pos] > '9' {
		return fmt.Errorf("expected exponent digits at position %d", p.pos)
	}
	p.scanDigits()
	return nil
}

// readCoordPair reads two numbers (x, y coordinate pair).
func (p *svgParser) readCoordPair() (float64, float64, error) {
	x, err := p.readNumber()
	if err != nil {
		return 0, 0, err
	}
	y, err := p.readNumber()
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// readFlag reads a flag value (0 or 1) for arc commands.
// Flags can be adjacent without separators per SVG spec.
func (p *svgParser) readFlag() (bool, error) {
	p.skipSeparator()
	if p.pos >= len(p.input) {
		return false, fmt.Errorf("unexpected end of input, expected flag (0 or 1)")
	}
	ch := p.input[p.pos]
	if ch == '0' {
		p.pos++
		return false, nil
	}
	if ch == '1' {
		p.pos++
		return true, nil
	}
	return false, fmt.Errorf("expected flag (0 or 1) at position %d, got %q", p.pos, string(ch))
}

// isCommandChar returns true if ch is an SVG path command letter.
func isCommandChar(ch byte) bool {
	return strings.ContainsRune("MmLlHhVvCcSsQqTtAaZz", rune(ch))
}
