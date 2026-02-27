package recording

import (
	"image"
	"math"
	"strings"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// Recorder captures drawing operations as commands.
// It mirrors the gg.Context drawing API but generates commands
// instead of rasterizing pixels. Use FinishRecording to obtain
// an immutable Recording that can be replayed to different backends.
//
// Example:
//
//	rec := recording.NewRecorder(800, 600)
//	rec.SetRGB(1, 0, 0)
//	rec.DrawCircle(100, 100, 50)
//	rec.Fill()
//	recording := rec.FinishRecording()
//
// The Recorder is not safe for concurrent use.
type Recorder struct {
	width, height int
	commands      []Command
	resources     *ResourcePool

	// Current path being built
	currentPath *gg.Path

	// Current state
	fillBrush   Brush
	strokeBrush Brush
	lineWidth   float64
	lineCap     LineCap
	lineJoin    LineJoin
	miterLimit  float64
	dashPattern []float64
	dashOffset  float64
	fillRule    FillRule
	transform   Matrix

	// Current font
	fontFace   text.Face
	fontFamily string
	fontSize   float64

	// State stack
	stateStack []recorderState
}

// recorderState stores the graphics state for Save/Restore.
type recorderState struct {
	fillBrush   Brush
	strokeBrush Brush
	lineWidth   float64
	lineCap     LineCap
	lineJoin    LineJoin
	miterLimit  float64
	dashPattern []float64
	dashOffset  float64
	fillRule    FillRule
	transform   Matrix
	fontFace    text.Face
	fontFamily  string
	fontSize    float64
}

// NewRecorder creates a new Recorder for the given dimensions.
// The Recorder starts with default state: black fill/stroke, 1px line width,
// butt caps, miter joins, non-zero fill rule, and identity transform.
func NewRecorder(width, height int) *Recorder {
	defaultBrush := NewSolidBrush(gg.Black)
	return &Recorder{
		width:       width,
		height:      height,
		commands:    make([]Command, 0, 256),
		resources:   NewResourcePool(),
		currentPath: gg.NewPath(),
		fillBrush:   defaultBrush,
		strokeBrush: defaultBrush,
		lineWidth:   1.0,
		lineCap:     LineCapButt,
		lineJoin:    LineJoinMiter,
		miterLimit:  4.0,
		fillRule:    FillRuleNonZero,
		transform:   Identity(),
		stateStack:  make([]recorderState, 0, 8),
	}
}

// FinishRecording returns an immutable Recording containing all recorded commands.
// After calling FinishRecording, the Recorder should not be used again.
func (r *Recorder) FinishRecording() *Recording {
	return &Recording{
		width:     r.width,
		height:    r.height,
		commands:  r.commands,
		resources: r.resources,
	}
}

// Recording is an immutable container for recorded drawing commands.
// It can be replayed to any Backend implementation.
type Recording struct {
	width, height int
	commands      []Command
	resources     *ResourcePool
}

// Width returns the width of the recording canvas.
func (r *Recording) Width() int {
	return r.width
}

// Height returns the height of the recording canvas.
func (r *Recording) Height() int {
	return r.height
}

// Commands returns the recorded commands.
func (r *Recording) Commands() []Command {
	return r.commands
}

// Resources returns the resource pool.
func (r *Recording) Resources() *ResourcePool {
	return r.resources
}

// Playback replays the recording to the given backend.
func (r *Recording) Playback(backend Backend) error {
	// Initialize backend
	if err := backend.Begin(r.width, r.height); err != nil {
		return err
	}

	// Replay each command
	for _, cmd := range r.commands {
		switch c := cmd.(type) {
		case SaveCommand:
			backend.Save()
		case RestoreCommand:
			backend.Restore()
		case SetTransformCommand:
			backend.SetTransform(c.Matrix)
		case SetClipCommand:
			path := r.resources.GetPath(c.Path)
			backend.SetClip(path, c.Rule)
		case ClearClipCommand:
			backend.ClearClip()
		case FillPathCommand:
			path := r.resources.GetPath(c.Path)
			brush := r.resources.GetBrush(c.Brush)
			backend.FillPath(path, brush, c.Rule)
		case StrokePathCommand:
			path := r.resources.GetPath(c.Path)
			brush := r.resources.GetBrush(c.Brush)
			backend.StrokePath(path, brush, c.Stroke)
		case FillRectCommand:
			brush := r.resources.GetBrush(c.Brush)
			backend.FillRect(c.Rect, brush)
		case DrawImageCommand:
			img := r.resources.GetImage(c.Image)
			backend.DrawImage(img, c.SrcRect, c.DstRect, c.Options)
		case DrawTextCommand:
			brush := r.resources.GetBrush(c.Brush)
			// Font face lookup would need additional handling
			backend.DrawText(c.Text, c.X, c.Y, nil, brush)
		// Style commands are handled by the backend's internal state
		// during the actual drawing operations
		case SetFillStyleCommand, SetStrokeStyleCommand,
			SetLineWidthCommand, SetLineCapCommand, SetLineJoinCommand,
			SetMiterLimitCommand, SetDashCommand, SetFillRuleCommand:
			// These are state commands that were recorded but the actual
			// style is captured in the drawing commands themselves
		}
	}

	return backend.End()
}

// --------------------------------------------------------------------------
// Dimensions
// --------------------------------------------------------------------------

// Width returns the width of the recording canvas.
func (r *Recorder) Width() int {
	return r.width
}

// Height returns the height of the recording canvas.
func (r *Recorder) Height() int {
	return r.height
}

// --------------------------------------------------------------------------
// State Management
// --------------------------------------------------------------------------

// Save saves the current graphics state to the stack.
// The state includes transform, fill style, stroke style, and line properties.
func (r *Recorder) Save() {
	// Clone dash pattern
	var dashCopy []float64
	if r.dashPattern != nil {
		dashCopy = make([]float64, len(r.dashPattern))
		copy(dashCopy, r.dashPattern)
	}

	r.stateStack = append(r.stateStack, recorderState{
		fillBrush:   r.fillBrush,
		strokeBrush: r.strokeBrush,
		lineWidth:   r.lineWidth,
		lineCap:     r.lineCap,
		lineJoin:    r.lineJoin,
		miterLimit:  r.miterLimit,
		dashPattern: dashCopy,
		dashOffset:  r.dashOffset,
		fillRule:    r.fillRule,
		transform:   r.transform,
		fontFace:    r.fontFace,
		fontFamily:  r.fontFamily,
		fontSize:    r.fontSize,
	})

	r.commands = append(r.commands, SaveCommand{})
}

// Restore restores the previously saved graphics state.
// If the state stack is empty, this is a no-op.
func (r *Recorder) Restore() {
	if len(r.stateStack) == 0 {
		return
	}

	state := r.stateStack[len(r.stateStack)-1]
	r.stateStack = r.stateStack[:len(r.stateStack)-1]

	r.fillBrush = state.fillBrush
	r.strokeBrush = state.strokeBrush
	r.lineWidth = state.lineWidth
	r.lineCap = state.lineCap
	r.lineJoin = state.lineJoin
	r.miterLimit = state.miterLimit
	r.dashPattern = state.dashPattern
	r.dashOffset = state.dashOffset
	r.fillRule = state.fillRule
	r.transform = state.transform
	r.fontFace = state.fontFace
	r.fontFamily = state.fontFamily
	r.fontSize = state.fontSize

	r.commands = append(r.commands, RestoreCommand{})
}

// Push is an alias for Save, matching gg.Context API.
func (r *Recorder) Push() {
	r.Save()
}

// Pop is an alias for Restore, matching gg.Context API.
func (r *Recorder) Pop() {
	r.Restore()
}

// --------------------------------------------------------------------------
// Transform
// --------------------------------------------------------------------------

// Identity resets the transformation matrix to identity.
func (r *Recorder) Identity() {
	r.transform = Identity()
	r.commands = append(r.commands, SetTransformCommand{Matrix: r.transform})
}

// Translate applies a translation to the transformation matrix.
func (r *Recorder) Translate(x, y float64) {
	r.transform = r.transform.Multiply(Translate(x, y))
	r.commands = append(r.commands, SetTransformCommand{Matrix: r.transform})
}

// Scale applies a scaling transformation.
func (r *Recorder) Scale(sx, sy float64) {
	r.transform = r.transform.Multiply(Scale(sx, sy))
	r.commands = append(r.commands, SetTransformCommand{Matrix: r.transform})
}

// Rotate applies a rotation (angle in radians).
func (r *Recorder) Rotate(angle float64) {
	r.transform = r.transform.Multiply(Rotate(angle))
	r.commands = append(r.commands, SetTransformCommand{Matrix: r.transform})
}

// RotateAbout rotates around a specific point.
func (r *Recorder) RotateAbout(angle, x, y float64) {
	r.Translate(x, y)
	r.Rotate(angle)
	r.Translate(-x, -y)
}

// Shear applies a shear transformation.
func (r *Recorder) Shear(x, y float64) {
	r.transform = r.transform.Multiply(Shear(x, y))
	r.commands = append(r.commands, SetTransformCommand{Matrix: r.transform})
}

// Transform multiplies the current transformation matrix by the given matrix.
func (r *Recorder) Transform(m Matrix) {
	r.transform = r.transform.Multiply(m)
	r.commands = append(r.commands, SetTransformCommand{Matrix: r.transform})
}

// SetTransform replaces the current transformation matrix.
func (r *Recorder) SetTransform(m Matrix) {
	r.transform = m
	r.commands = append(r.commands, SetTransformCommand{Matrix: r.transform})
}

// GetTransform returns a copy of the current transformation matrix.
func (r *Recorder) GetTransform() Matrix {
	return r.transform
}

// TransformPoint transforms a point by the current matrix.
func (r *Recorder) TransformPoint(x, y float64) (float64, float64) {
	return r.transform.TransformPoint(x, y)
}

// InvertY inverts the Y axis (useful for coordinate system changes).
func (r *Recorder) InvertY() {
	r.Translate(0, float64(r.height))
	r.Scale(1, -1)
}

// --------------------------------------------------------------------------
// Color/Style
// --------------------------------------------------------------------------

// SetColor sets both fill and stroke color from a color.Color.
func (r *Recorder) SetColor(c gg.RGBA) {
	brush := NewSolidBrush(c)
	r.fillBrush = brush
	r.strokeBrush = brush
	brushRef := r.resources.AddBrush(brush)
	r.commands = append(r.commands,
		SetFillStyleCommand{Brush: brushRef},
		SetStrokeStyleCommand{Brush: brushRef})
}

// SetRGB sets both fill and stroke color using RGB values (0-1).
func (r *Recorder) SetRGB(red, green, blue float64) {
	r.SetColor(gg.RGB(red, green, blue))
}

// SetRGBA sets both fill and stroke color using RGBA values (0-1).
func (r *Recorder) SetRGBA(red, green, blue, alpha float64) {
	r.SetColor(gg.RGBA2(red, green, blue, alpha))
}

// SetHexColor sets both fill and stroke color using a hex string.
func (r *Recorder) SetHexColor(hex string) {
	r.SetColor(gg.Hex(hex))
}

// SetFillStyle sets the fill brush.
func (r *Recorder) SetFillStyle(brush Brush) {
	r.fillBrush = brush
	brushRef := r.resources.AddBrush(brush)
	r.commands = append(r.commands, SetFillStyleCommand{Brush: brushRef})
}

// SetStrokeStyle sets the stroke brush.
func (r *Recorder) SetStrokeStyle(brush Brush) {
	r.strokeBrush = brush
	brushRef := r.resources.AddBrush(brush)
	r.commands = append(r.commands, SetStrokeStyleCommand{Brush: brushRef})
}

// SetFillBrush sets the fill brush from a gg.Brush.
func (r *Recorder) SetFillBrush(brush gg.Brush) {
	r.SetFillStyle(BrushFromGG(brush))
}

// SetStrokeBrush sets the stroke brush from a gg.Brush.
func (r *Recorder) SetStrokeBrush(brush gg.Brush) {
	r.SetStrokeStyle(BrushFromGG(brush))
}

// SetFillRGB sets the fill color using RGB values (0-1).
func (r *Recorder) SetFillRGB(red, green, blue float64) {
	r.SetFillStyle(NewSolidBrush(gg.RGB(red, green, blue)))
}

// SetFillRGBA sets the fill color using RGBA values (0-1).
func (r *Recorder) SetFillRGBA(red, green, blue, alpha float64) {
	r.SetFillStyle(NewSolidBrush(gg.RGBA2(red, green, blue, alpha)))
}

// SetStrokeRGB sets the stroke color using RGB values (0-1).
func (r *Recorder) SetStrokeRGB(red, green, blue float64) {
	r.SetStrokeStyle(NewSolidBrush(gg.RGB(red, green, blue)))
}

// SetStrokeRGBA sets the stroke color using RGBA values (0-1).
func (r *Recorder) SetStrokeRGBA(red, green, blue, alpha float64) {
	r.SetStrokeStyle(NewSolidBrush(gg.RGBA2(red, green, blue, alpha)))
}

// --------------------------------------------------------------------------
// Line Properties
// --------------------------------------------------------------------------

// SetLineWidth sets the line width for stroking.
func (r *Recorder) SetLineWidth(width float64) {
	r.lineWidth = width
	r.commands = append(r.commands, SetLineWidthCommand{Width: width})
}

// SetLineCap sets the line cap style.
func (r *Recorder) SetLineCap(lc LineCap) {
	r.lineCap = lc
	r.commands = append(r.commands, SetLineCapCommand{Cap: lc})
}

// SetLineCapGG sets the line cap style from gg.LineCap.
func (r *Recorder) SetLineCapGG(lc gg.LineCap) {
	// #nosec G115 -- LineCap enum values are within uint8 range
	r.SetLineCap(LineCap(lc))
}

// SetLineJoin sets the line join style.
func (r *Recorder) SetLineJoin(join LineJoin) {
	r.lineJoin = join
	r.commands = append(r.commands, SetLineJoinCommand{Join: join})
}

// SetLineJoinGG sets the line join style from gg.LineJoin.
func (r *Recorder) SetLineJoinGG(join gg.LineJoin) {
	// #nosec G115 -- LineJoin enum values are within uint8 range
	r.SetLineJoin(LineJoin(join))
}

// SetMiterLimit sets the miter limit for line joins.
func (r *Recorder) SetMiterLimit(limit float64) {
	r.miterLimit = limit
	r.commands = append(r.commands, SetMiterLimitCommand{Limit: limit})
}

// SetDash sets the dash pattern for stroking.
// Pass alternating dash and gap lengths.
// Passing no arguments clears the dash pattern (returns to solid lines).
func (r *Recorder) SetDash(lengths ...float64) {
	if len(lengths) == 0 {
		r.ClearDash()
		return
	}

	r.dashPattern = make([]float64, len(lengths))
	copy(r.dashPattern, lengths)
	r.commands = append(r.commands, SetDashCommand{Pattern: r.dashPattern, Offset: r.dashOffset})
}

// SetDashOffset sets the starting offset into the dash pattern.
func (r *Recorder) SetDashOffset(offset float64) {
	r.dashOffset = offset
	if r.dashPattern != nil {
		r.commands = append(r.commands, SetDashCommand{Pattern: r.dashPattern, Offset: r.dashOffset})
	}
}

// ClearDash removes the dash pattern, returning to solid lines.
func (r *Recorder) ClearDash() {
	r.dashPattern = nil
	r.dashOffset = 0
	r.commands = append(r.commands, SetDashCommand{Pattern: nil, Offset: 0})
}

// SetFillRule sets the fill rule.
func (r *Recorder) SetFillRule(rule FillRule) {
	r.fillRule = rule
	r.commands = append(r.commands, SetFillRuleCommand{Rule: rule})
}

// SetFillRuleGG sets the fill rule from gg.FillRule.
func (r *Recorder) SetFillRuleGG(rule gg.FillRule) {
	// #nosec G115 -- FillRule enum values are within uint8 range
	r.SetFillRule(FillRule(rule))
}

// --------------------------------------------------------------------------
// Path Building
// --------------------------------------------------------------------------

// MoveTo starts a new subpath at the given point.
func (r *Recorder) MoveTo(x, y float64) {
	px, py := r.transform.TransformPoint(x, y)
	r.currentPath.MoveTo(px, py)
}

// LineTo adds a line to the current path.
func (r *Recorder) LineTo(x, y float64) {
	px, py := r.transform.TransformPoint(x, y)
	r.currentPath.LineTo(px, py)
}

// QuadraticTo adds a quadratic Bezier curve to the current path.
func (r *Recorder) QuadraticTo(cx, cy, x, y float64) {
	cpx, cpy := r.transform.TransformPoint(cx, cy)
	px, py := r.transform.TransformPoint(x, y)
	r.currentPath.QuadraticTo(cpx, cpy, px, py)
}

// CubicTo adds a cubic Bezier curve to the current path.
func (r *Recorder) CubicTo(c1x, c1y, c2x, c2y, x, y float64) {
	cp1x, cp1y := r.transform.TransformPoint(c1x, c1y)
	cp2x, cp2y := r.transform.TransformPoint(c2x, c2y)
	px, py := r.transform.TransformPoint(x, y)
	r.currentPath.CubicTo(cp1x, cp1y, cp2x, cp2y, px, py)
}

// ClosePath closes the current subpath.
func (r *Recorder) ClosePath() {
	r.currentPath.Close()
}

// ClearPath clears the current path.
func (r *Recorder) ClearPath() {
	r.currentPath.Clear()
}

// NewSubPath starts a new subpath without closing the previous one.
// This is a no-op as MoveTo already creates a new subpath.
func (r *Recorder) NewSubPath() {
	// No-op, provided for API compatibility
}

// --------------------------------------------------------------------------
// Drawing
// --------------------------------------------------------------------------

// Fill fills the current path and clears it.
func (r *Recorder) Fill() {
	if len(r.currentPath.Elements()) == 0 {
		return
	}

	pathRef := r.resources.AddPath(r.currentPath)
	brushRef := r.resources.AddBrush(r.fillBrush)

	r.commands = append(r.commands, FillPathCommand{
		Path:  pathRef,
		Brush: brushRef,
		Rule:  r.fillRule,
	})

	r.currentPath = gg.NewPath()
}

// FillPreserve fills the current path without clearing it.
func (r *Recorder) FillPreserve() {
	if len(r.currentPath.Elements()) == 0 {
		return
	}

	pathRef := r.resources.AddPath(r.currentPath)
	brushRef := r.resources.AddBrush(r.fillBrush)

	r.commands = append(r.commands, FillPathCommand{
		Path:  pathRef,
		Brush: brushRef,
		Rule:  r.fillRule,
	})
}

// Stroke strokes the current path and clears it.
func (r *Recorder) Stroke() {
	if len(r.currentPath.Elements()) == 0 {
		return
	}

	pathRef := r.resources.AddPath(r.currentPath)
	brushRef := r.resources.AddBrush(r.strokeBrush)

	stroke := Stroke{
		Width:       r.lineWidth,
		Cap:         r.lineCap,
		Join:        r.lineJoin,
		MiterLimit:  r.miterLimit,
		DashPattern: r.dashPattern,
		DashOffset:  r.dashOffset,
	}

	r.commands = append(r.commands, StrokePathCommand{
		Path:   pathRef,
		Brush:  brushRef,
		Stroke: stroke,
	})

	r.currentPath = gg.NewPath()
}

// StrokePreserve strokes the current path without clearing it.
func (r *Recorder) StrokePreserve() {
	if len(r.currentPath.Elements()) == 0 {
		return
	}

	pathRef := r.resources.AddPath(r.currentPath)
	brushRef := r.resources.AddBrush(r.strokeBrush)

	stroke := Stroke{
		Width:       r.lineWidth,
		Cap:         r.lineCap,
		Join:        r.lineJoin,
		MiterLimit:  r.miterLimit,
		DashPattern: r.dashPattern,
		DashOffset:  r.dashOffset,
	}

	r.commands = append(r.commands, StrokePathCommand{
		Path:   pathRef,
		Brush:  brushRef,
		Stroke: stroke,
	})
}

// FillStroke fills and then strokes the current path, then clears it.
func (r *Recorder) FillStroke() {
	r.FillPreserve()
	r.Stroke()
}

// --------------------------------------------------------------------------
// Shapes
// --------------------------------------------------------------------------

// DrawPoint draws a single point at the given coordinates.
func (r *Recorder) DrawPoint(x, y, radius float64) {
	r.DrawCircle(x, y, radius)
}

// DrawLine draws a line between two points.
func (r *Recorder) DrawLine(x1, y1, x2, y2 float64) {
	r.MoveTo(x1, y1)
	r.LineTo(x2, y2)
}

// DrawRectangle draws a rectangle.
func (r *Recorder) DrawRectangle(x, y, w, h float64) {
	r.MoveTo(x, y)
	r.LineTo(x+w, y)
	r.LineTo(x+w, y+h)
	r.LineTo(x, y+h)
	r.ClosePath()
}

// DrawRoundedRectangle draws a rectangle with rounded corners.
func (r *Recorder) DrawRoundedRectangle(x, y, w, h, radius float64) {
	// Clamp radius to half of the smaller dimension
	maxR := math.Min(w, h) / 2
	if radius > maxR {
		radius = maxR
	}

	r.MoveTo(x+radius, y)
	r.LineTo(x+w-radius, y)
	r.drawArcPath(x+w-radius, y+radius, radius, -math.Pi/2, 0)
	r.LineTo(x+w, y+h-radius)
	r.drawArcPath(x+w-radius, y+h-radius, radius, 0, math.Pi/2)
	r.LineTo(x+radius, y+h)
	r.drawArcPath(x+radius, y+h-radius, radius, math.Pi/2, math.Pi)
	r.LineTo(x, y+radius)
	r.drawArcPath(x+radius, y+radius, radius, math.Pi, 3*math.Pi/2)
	r.ClosePath()
}

// DrawCircle draws a circle.
func (r *Recorder) DrawCircle(x, y, radius float64) {
	const k = 0.5522847498307936 // 4/3 * (sqrt(2) - 1)
	offset := radius * k

	r.MoveTo(x+radius, y)
	r.CubicTo(x+radius, y+offset, x+offset, y+radius, x, y+radius)
	r.CubicTo(x-offset, y+radius, x-radius, y+offset, x-radius, y)
	r.CubicTo(x-radius, y-offset, x-offset, y-radius, x, y-radius)
	r.CubicTo(x+offset, y-radius, x+radius, y-offset, x+radius, y)
	r.ClosePath()
}

// DrawEllipse draws an ellipse.
func (r *Recorder) DrawEllipse(x, y, rx, ry float64) {
	const k = 0.5522847498307936
	ox := rx * k
	oy := ry * k

	r.MoveTo(x+rx, y)
	r.CubicTo(x+rx, y+oy, x+ox, y+ry, x, y+ry)
	r.CubicTo(x-ox, y+ry, x-rx, y+oy, x-rx, y)
	r.CubicTo(x-rx, y-oy, x-ox, y-ry, x, y-ry)
	r.CubicTo(x+ox, y-ry, x+rx, y-oy, x+rx, y)
	r.ClosePath()
}

// DrawArc draws a circular arc.
func (r *Recorder) DrawArc(x, y, radius, angle1, angle2 float64) {
	r.drawArcPath(x, y, radius, angle1, angle2)
}

// drawArcPath adds arc segments to the current path.
func (r *Recorder) drawArcPath(cx, cy, radius, angle1, angle2 float64) {
	const twoPi = 2 * math.Pi
	for angle2 < angle1 {
		angle2 += twoPi
	}

	const maxAngle = math.Pi / 2
	numSegments := int(math.Ceil((angle2 - angle1) / maxAngle))
	angleStep := (angle2 - angle1) / float64(numSegments)

	for i := 0; i < numSegments; i++ {
		a1 := angle1 + float64(i)*angleStep
		a2 := a1 + angleStep
		r.arcSegment(cx, cy, radius, a1, a2)
	}
}

// arcSegment adds a single arc segment using cubic Bezier curves.
func (r *Recorder) arcSegment(cx, cy, radius, a1, a2 float64) {
	alpha := math.Sin(a2-a1) * (math.Sqrt(4+3*math.Tan((a2-a1)/2)*math.Tan((a2-a1)/2)) - 1) / 3

	cos1, sin1 := math.Cos(a1), math.Sin(a1)
	cos2, sin2 := math.Cos(a2), math.Sin(a2)

	x1 := cx + radius*cos1
	y1 := cy + radius*sin1
	x2 := cx + radius*cos2
	y2 := cy + radius*sin2

	c1x := x1 - alpha*radius*sin1
	c1y := y1 + alpha*radius*cos1
	c2x := x2 + alpha*radius*sin2
	c2y := y2 - alpha*radius*cos2

	if len(r.currentPath.Elements()) == 0 {
		r.MoveTo(x1, y1)
	}
	r.CubicTo(c1x, c1y, c2x, c2y, x2, y2)
}

// DrawEllipticalArc draws an elliptical arc.
func (r *Recorder) DrawEllipticalArc(x, y, rx, ry, angle1, angle2 float64) {
	r.Save()
	r.Translate(x, y)
	r.Scale(rx, ry)
	r.DrawArc(0, 0, 1, angle1, angle2)
	// Restore state but keep path changes
	if len(r.stateStack) > 0 {
		state := r.stateStack[len(r.stateStack)-1]
		r.stateStack = r.stateStack[:len(r.stateStack)-1]
		r.fillBrush = state.fillBrush
		r.strokeBrush = state.strokeBrush
		r.lineWidth = state.lineWidth
		r.lineCap = state.lineCap
		r.lineJoin = state.lineJoin
		r.miterLimit = state.miterLimit
		r.dashPattern = state.dashPattern
		r.dashOffset = state.dashOffset
		r.fillRule = state.fillRule
		r.transform = state.transform
	}
	r.commands = append(r.commands, RestoreCommand{})
}

// --------------------------------------------------------------------------
// Rectangles (Optimized)
// --------------------------------------------------------------------------

// FillRectangle fills a rectangle without adding it to the path.
// This is an optimized operation for the common case of axis-aligned rectangles.
func (r *Recorder) FillRectangle(x, y, w, h float64) {
	// Transform corners
	x1, y1 := r.transform.TransformPoint(x, y)
	x2, y2 := r.transform.TransformPoint(x+w, y+h)

	rect := NewRectFromPoints(x1, y1, x2, y2)
	brushRef := r.resources.AddBrush(r.fillBrush)

	r.commands = append(r.commands, FillRectCommand{
		Rect:  rect,
		Brush: brushRef,
	})
}

// StrokeRectangle strokes a rectangle without adding it to the path.
func (r *Recorder) StrokeRectangle(x, y, w, h float64) {
	// Transform corners
	x1, y1 := r.transform.TransformPoint(x, y)
	x2, y2 := r.transform.TransformPoint(x+w, y+h)

	rect := NewRectFromPoints(x1, y1, x2, y2)
	brushRef := r.resources.AddBrush(r.strokeBrush)

	stroke := Stroke{
		Width:       r.lineWidth,
		Cap:         r.lineCap,
		Join:        r.lineJoin,
		MiterLimit:  r.miterLimit,
		DashPattern: r.dashPattern,
		DashOffset:  r.dashOffset,
	}

	r.commands = append(r.commands, StrokeRectCommand{
		Rect:   rect,
		Brush:  brushRef,
		Stroke: stroke,
	})
}

// --------------------------------------------------------------------------
// Clipping
// --------------------------------------------------------------------------

// Clip sets the current path as the clipping region and clears the path.
func (r *Recorder) Clip() {
	if len(r.currentPath.Elements()) == 0 {
		return
	}

	pathRef := r.resources.AddPath(r.currentPath)
	r.commands = append(r.commands, SetClipCommand{
		Path: pathRef,
		Rule: r.fillRule,
	})

	r.currentPath = gg.NewPath()
}

// ClipPreserve sets the current path as the clipping region but keeps the path.
func (r *Recorder) ClipPreserve() {
	if len(r.currentPath.Elements()) == 0 {
		return
	}

	pathRef := r.resources.AddPath(r.currentPath)
	r.commands = append(r.commands, SetClipCommand{
		Path: pathRef,
		Rule: r.fillRule,
	})
}

// ResetClip removes all clipping regions.
func (r *Recorder) ResetClip() {
	r.commands = append(r.commands, ClearClipCommand{})
}

// --------------------------------------------------------------------------
// Image
// --------------------------------------------------------------------------

// DrawImage draws an image at the specified position.
func (r *Recorder) DrawImage(img image.Image, x, y int) {
	if img == nil {
		return
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	imageRef := r.resources.AddImage(img)

	// Transform destination rectangle
	x1, y1 := r.transform.TransformPoint(float64(x), float64(y))
	x2, y2 := r.transform.TransformPoint(float64(x+w), float64(y+h))

	srcRect := NewRect(0, 0, float64(w), float64(h))
	dstRect := NewRectFromPoints(x1, y1, x2, y2)

	r.commands = append(r.commands, DrawImageCommand{
		Image:   imageRef,
		SrcRect: srcRect,
		DstRect: dstRect,
		Options: DefaultImageOptions(),
	})
}

// DrawImageAnchored draws an image with an anchor point.
func (r *Recorder) DrawImageAnchored(img image.Image, x, y int, ax, ay float64) {
	if img == nil {
		return
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Adjust position based on anchor
	drawX := float64(x) - float64(w)*ax
	drawY := float64(y) - float64(h)*ay

	imageRef := r.resources.AddImage(img)

	// Transform destination rectangle
	x1, y1 := r.transform.TransformPoint(drawX, drawY)
	x2, y2 := r.transform.TransformPoint(drawX+float64(w), drawY+float64(h))

	srcRect := NewRect(0, 0, float64(w), float64(h))
	dstRect := NewRectFromPoints(x1, y1, x2, y2)

	r.commands = append(r.commands, DrawImageCommand{
		Image:   imageRef,
		SrcRect: srcRect,
		DstRect: dstRect,
		Options: DefaultImageOptions(),
	})
}

// DrawImageScaled draws an image scaled to fit the specified rectangle.
func (r *Recorder) DrawImageScaled(img image.Image, x, y, w, h float64) {
	if img == nil {
		return
	}

	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	imageRef := r.resources.AddImage(img)

	// Transform destination rectangle
	x1, y1 := r.transform.TransformPoint(x, y)
	x2, y2 := r.transform.TransformPoint(x+w, y+h)

	srcRect := NewRect(0, 0, float64(srcW), float64(srcH))
	dstRect := NewRectFromPoints(x1, y1, x2, y2)

	r.commands = append(r.commands, DrawImageCommand{
		Image:   imageRef,
		SrcRect: srcRect,
		DstRect: dstRect,
		Options: DefaultImageOptions(),
	})
}

// --------------------------------------------------------------------------
// Text
// --------------------------------------------------------------------------

// SetFont sets the current font face for text drawing.
func (r *Recorder) SetFont(face text.Face) {
	r.fontFace = face
}

// SetFontSize sets the current font size in points.
func (r *Recorder) SetFontSize(size float64) {
	r.fontSize = size
}

// SetFontFamily sets the current font family name.
func (r *Recorder) SetFontFamily(family string) {
	r.fontFamily = family
}

// DrawString draws text at position (x, y) where y is the baseline.
func (r *Recorder) DrawString(s string, x, y float64) {
	// Transform position
	px, py := r.transform.TransformPoint(x, y)

	brushRef := r.resources.AddBrush(r.fillBrush)

	r.commands = append(r.commands, DrawTextCommand{
		Text:       s,
		X:          px,
		Y:          py,
		FontSize:   r.fontSize,
		FontFamily: r.fontFamily,
		Brush:      brushRef,
	})
}

// DrawStringAnchored draws text with an anchor point.
// The anchor point is specified by ax and ay, which are in the range [0, 1].
func (r *Recorder) DrawStringAnchored(s string, x, y, ax, ay float64) {
	// For recording, we store the base position and let the backend handle anchoring
	// This is a simplification; a full implementation would measure text
	r.DrawString(s, x, y)
}

// MeasureString returns approximate dimensions of text.
// Note: Actual measurement depends on the backend and font.
// Returns (0, 0) if no font is set.
func (r *Recorder) MeasureString(s string) (w, h float64) {
	if r.fontFace == nil {
		// Approximate measurement based on font size
		// Average character width is roughly 0.6 * font size
		w = float64(len(s)) * r.fontSize * 0.6
		h = r.fontSize * 1.2
		return
	}
	return text.Measure(s, r.fontFace)
}

// WordWrap wraps text to fit within the given width using word boundaries.
// Uses the current font face for text measurement.
// If no font face is set, returns the input string as a single-element slice.
func (r *Recorder) WordWrap(s string, w float64) []string {
	if r.fontFace == nil {
		return []string{s}
	}
	results := text.WrapText(s, r.fontFace, w, text.WrapWord)
	lines := make([]string, len(results))
	for i, res := range results {
		lines[i] = res.Text
	}
	return lines
}

// MeasureMultilineString measures text that may contain newlines.
// Returns (width, height) where width is the maximum line width.
// If no font face is set, returns (0, 0).
func (r *Recorder) MeasureMultilineString(s string, lineSpacing float64) (width, height float64) {
	if r.fontFace == nil {
		return 0, 0
	}
	lines := recorderSplitLines(s)
	fh := r.fontFace.Metrics().LineHeight()
	for _, line := range lines {
		lw, _ := text.Measure(line, r.fontFace)
		if lw > width {
			width = lw
		}
	}
	n := float64(len(lines))
	height = n*fh*lineSpacing - (lineSpacing-1)*fh
	return
}

// DrawStringWrapped wraps text to the given width and draws it with alignment.
// Each wrapped line is recorded as a separate DrawText command.
func (r *Recorder) DrawStringWrapped(s string, x, y, ax, ay, width, lineSpacing float64, align text.Alignment) {
	lines := r.WordWrap(s, width)
	if len(lines) == 0 {
		return
	}

	var fh float64
	if r.fontFace != nil {
		fh = r.fontFace.Metrics().LineHeight()
	} else {
		fh = r.fontSize * 1.2
	}

	// Total height (same formula as MeasureMultilineString)
	n := float64(len(lines))
	h := n*fh*lineSpacing - (lineSpacing-1)*fh

	// Adjust starting position by anchor
	x -= ax * width
	y -= ay * h

	// Adjust x base for alignment
	switch align {
	case text.AlignCenter:
		x += width / 2
	case text.AlignRight:
		x += width
	}

	for _, line := range lines {
		drawX := x
		switch align {
		case text.AlignCenter:
			lw, _ := r.MeasureString(line)
			drawX = x - lw/2
		case text.AlignRight:
			lw, _ := r.MeasureString(line)
			drawX = x - lw
		}
		r.DrawString(line, drawX, y)
		y += fh * lineSpacing
	}
}

// recorderSplitLines splits text by line breaks, normalizing \r\n and \r to \n.
func recorderSplitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

// --------------------------------------------------------------------------
// Utility Methods
// --------------------------------------------------------------------------

// Clear fills the entire canvas with the current fill color.
// This is equivalent to drawing a rectangle covering the entire canvas.
func (r *Recorder) Clear() {
	r.FillRectangle(0, 0, float64(r.width), float64(r.height))
}

// ClearWithColor fills the entire canvas with a specific color.
func (r *Recorder) ClearWithColor(c gg.RGBA) {
	oldBrush := r.fillBrush
	r.fillBrush = NewSolidBrush(c)
	r.FillRectangle(0, 0, float64(r.width), float64(r.height))
	r.fillBrush = oldBrush
}

// GetCurrentPoint returns the current point of the path.
// Returns (0, 0, false) if there is no current point.
func (r *Recorder) GetCurrentPoint() (x, y float64, ok bool) {
	if r.currentPath == nil || !r.currentPath.HasCurrentPoint() {
		return 0, 0, false
	}
	pt := r.currentPath.CurrentPoint()
	return pt.X, pt.Y, true
}
