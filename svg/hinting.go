package svg

import (
	"math"
	"os"

	"github.com/gogpu/gg"
)

// strokeHintPolicy is deliberately local to SVG lowering. Generic gg paths and
// Scene commands remain independent of target size and pixel alignment.
type strokeHintPolicy struct {
	physical gg.Matrix
	eligible bool
	scale    float64
}

func newStrokeHintPolicy(width, height, deviceScale float64, transform gg.Matrix) strokeHintPolicy {
	if strokeHintingDisabled() {
		return strokeHintPolicy{}
	}
	if deviceScale <= 0 {
		deviceScale = 1
	}
	if width <= 0 || height <= 0 || math.Max(width*deviceScale, height*deviceScale) > 32 {
		return strokeHintPolicy{}
	}

	// Only translation plus a uniform, axis-preserving scale is safe: a
	// perpendicular SVG coordinate must still be perpendicular in device space.
	sx, sy := math.Abs(transform.A), math.Abs(transform.E)
	if transform.B != 0 || transform.D != 0 || sx == 0 || sy == 0 || sx != sy {
		return strokeHintPolicy{}
	}

	physical := gg.Scale(deviceScale, deviceScale).Multiply(transform)
	return strokeHintPolicy{physical: physical, eligible: true, scale: sx * deviceScale}
}

func strokeHintingDisabled() bool {
	return os.Getenv("GOGPU_SVG_NO_HINT") != ""
}

func (p strokeHintPolicy) permits(strokeWidth float64) bool {
	physicalWidth := strokeWidth * p.scale
	return p.eligible && physicalWidth > 0 && physicalWidth <= 1.5
}

type hintCommand struct {
	verb   gg.PathVerb
	coords []float64
}

type hintPoint struct {
	command int
	blocked bool
}

type hintSegment struct {
	from, to   int
	horizontal bool
	vertical   bool
}

type hintPathBuilder struct {
	policy       strokeHintPolicy
	commands     []hintCommand
	points       []hintPoint
	segments     []hintSegment
	current      int
	subpathStart int
}

func newHintPathBuilder(capacity int, policy strokeHintPolicy) *hintPathBuilder {
	return &hintPathBuilder{
		policy:       policy,
		commands:     make([]hintCommand, 0, capacity),
		points:       make([]hintPoint, 0, capacity),
		segments:     make([]hintSegment, 0, capacity),
		current:      -1,
		subpathStart: -1,
	}
}

// hintStrokePath returns an independent stroke path. Ineligible input is
// returned unchanged; eligible input is copied even when it has no cardinal
// segments, which makes source immutability explicit at the lowering boundary.
func hintStrokePath(path *gg.Path, policy strokeHintPolicy, strokeWidth float64) *gg.Path {
	if path == nil || !policy.permits(strokeWidth) {
		return path
	}

	builder := newHintPathBuilder(path.NumVerbs(), policy)
	path.Iterate(func(verb gg.PathVerb, coords []float64) {
		builder.addCommand(verb, coords)
	})

	for _, segment := range builder.segments {
		if builder.points[segment.from].blocked || builder.points[segment.to].blocked {
			continue
		}
		if segment.horizontal {
			snapHintPoint(builder.commands, builder.points[segment.from], policy.physical, false)
			snapHintPoint(builder.commands, builder.points[segment.to], policy.physical, false)
		}
		if segment.vertical {
			snapHintPoint(builder.commands, builder.points[segment.from], policy.physical, true)
			snapHintPoint(builder.commands, builder.points[segment.to], policy.physical, true)
		}
	}
	return buildHintedPath(builder.commands)
}

func (b *hintPathBuilder) addCommand(verb gg.PathVerb, coords []float64) {
	b.commands = append(b.commands, hintCommand{verb: verb, coords: append([]float64(nil), coords...)})
	commandIndex := len(b.commands) - 1

	switch verb {
	case gg.MoveTo:
		b.points = append(b.points, hintPoint{command: commandIndex})
		b.current = len(b.points) - 1
		b.subpathStart = b.current
	case gg.LineTo:
		b.addLine(commandIndex)
	case gg.QuadTo, gg.CubicTo:
		b.blockCurveEndpoint()
	case gg.Close:
		b.closeSubpath()
	}
}

func (b *hintPathBuilder) addLine(commandIndex int) {
	b.points = append(b.points, hintPoint{command: commandIndex})
	destination := len(b.points) - 1
	if b.current >= 0 {
		// A zero-length line blocks its node just like any other non-cardinal line.
		b.connect(b.current, destination, true)
	}
	b.current = destination
}

func (b *hintPathBuilder) blockCurveEndpoint() {
	if b.current >= 0 {
		b.points[b.current].blocked = true
	}
	// A curve endpoint is intentionally not exposed as a snappable node.
	b.current = -1
}

func (b *hintPathBuilder) closeSubpath() {
	if b.current >= 0 && b.subpathStart >= 0 {
		// A coincident close is structural rather than a zero-length line.
		b.connect(b.current, b.subpathStart, false)
	}
	b.current = b.subpathStart
}

func (b *hintPathBuilder) connect(fromIndex, toIndex int, blockCoincident bool) {
	from := b.physicalPoint(fromIndex)
	to := b.physicalPoint(toIndex)
	horizontal := from.Y == to.Y && from.X != to.X
	vertical := from.X == to.X && from.Y != to.Y
	b.segments = append(b.segments, hintSegment{
		from: fromIndex, to: toIndex, horizontal: horizontal, vertical: vertical,
	})
	if !horizontal && !vertical && (blockCoincident || from != to) {
		b.points[fromIndex].blocked = true
		b.points[toIndex].blocked = true
	}
}

func (b *hintPathBuilder) physicalPoint(pointIndex int) gg.Point {
	coords := b.commands[b.points[pointIndex].command].coords
	return b.policy.physical.TransformPoint(gg.Pt(coords[len(coords)-2], coords[len(coords)-1]))
}

func buildHintedPath(commands []hintCommand) *gg.Path {
	result := gg.NewPath()
	for _, command := range commands {
		switch command.verb {
		case gg.MoveTo:
			result.MoveTo(command.coords[0], command.coords[1])
		case gg.LineTo:
			result.LineTo(command.coords[0], command.coords[1])
		case gg.QuadTo:
			result.QuadraticTo(command.coords[0], command.coords[1], command.coords[2], command.coords[3])
		case gg.CubicTo:
			result.CubicTo(command.coords[0], command.coords[1], command.coords[2], command.coords[3], command.coords[4], command.coords[5])
		case gg.Close:
			result.Close()
		}
	}
	return result
}

func snapHintPoint(commands []hintCommand, point hintPoint, physical gg.Matrix, xAxis bool) {
	coords := commands[point.command].coords
	x, y := coords[len(coords)-2], coords[len(coords)-1]
	device := physical.TransformPoint(gg.Pt(x, y))
	if xAxis {
		device.X = snapPixelCenter(device.X)
		coords[len(coords)-2] = (device.X - physical.C) / physical.A
	} else {
		device.Y = snapPixelCenter(device.Y)
		coords[len(coords)-1] = (device.Y - physical.F) / physical.E
	}
}

func snapPixelCenter(value float64) float64 {
	return math.Floor(value) + 0.5
}
