// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package render

import (
	"image/color"

	"github.com/gogpu/gg/internal/raster"
)

// DirtyRect represents a region that needs redraw.
// Used for damage tracking to enable efficient partial redraws.
type DirtyRect struct {
	X, Y, Width, Height float64
}

// maxDirtyRects is the threshold after which we switch to full redraw.
// When more than this many rects accumulate, it's more efficient to redraw everything.
const maxDirtyRects = 16

// Scene represents a retained-mode drawing tree.
//
// Unlike immediate-mode drawing (Context.DrawCircle, etc.), a Scene captures
// drawing commands that can be:
//   - Rendered multiple times without rebuilding
//   - Partially invalidated for efficient UI updates
//   - Optimized by the renderer for batching
//
// Scene is designed to work with the Renderer interface, allowing both
// CPU and GPU rendering backends to process the same scene data.
//
// Example:
//
//	scene := render.NewScene()
//	scene.SetFillColor(color.RGBA{255, 0, 0, 255})
//	scene.MoveTo(100, 50)
//	scene.LineTo(150, 150)
//	scene.LineTo(50, 150)
//	scene.ClosePath()
//	scene.Fill()
//
//	// Render to any target
//	renderer.Render(target1, scene)
//	renderer.Render(target2, scene)
type Scene struct {
	// commands stores the drawing command stream.
	commands []drawCommand

	// currentPath builds the current path being constructed.
	currentPath pathBuilder

	// currentFillColor is the color for fill operations.
	currentFillColor color.Color

	// currentStrokeColor is the color for stroke operations.
	currentStrokeColor color.Color

	// currentStrokeWidth is the width for stroke operations.
	currentStrokeWidth float64

	// currentFillRule is the fill rule for fill operations.
	currentFillRule raster.FillRule

	// Damage tracking for efficient partial redraws
	dirtyRects []DirtyRect
	fullRedraw bool
}

// drawCommand represents a single drawing operation.
type drawCommand struct {
	op       drawOp
	path     *pathBuilder // Snapshot of path at command time
	color    color.Color
	width    float64
	fillRule raster.FillRule
}

// drawOp is the type of drawing operation.
type drawOp uint8

const (
	opFill drawOp = iota
	opStroke
	opClear
)

// pathBuilder accumulates path construction commands.
type pathBuilder struct {
	verbs  []raster.PathVerb
	points []float32
}

// NewScene creates a new empty Scene.
func NewScene() *Scene {
	return &Scene{
		commands:           make([]drawCommand, 0, 16),
		currentFillColor:   color.Black,
		currentStrokeColor: color.Black,
		currentStrokeWidth: 1.0,
		currentFillRule:    raster.FillRuleNonZero,
	}
}

// Reset clears the scene for reuse.
func (s *Scene) Reset() {
	s.commands = s.commands[:0]
	s.currentPath = pathBuilder{}
	s.currentFillColor = color.Black
	s.currentStrokeColor = color.Black
	s.currentStrokeWidth = 1.0
	s.currentFillRule = raster.FillRuleNonZero
	s.ClearDirty()
}

// Invalidate marks a rectangular region as needing redraw.
// This is used for damage tracking to enable efficient partial redraws.
// If the accumulated dirty rects exceed maxDirtyRects, the scene switches
// to full redraw mode for efficiency.
func (s *Scene) Invalidate(rect DirtyRect) {
	if s.fullRedraw {
		return // Already in full redraw mode
	}

	// Validate rect has positive dimensions
	if rect.Width <= 0 || rect.Height <= 0 {
		return
	}

	s.dirtyRects = append(s.dirtyRects, rect)

	// If too many rects, switch to full redraw
	if len(s.dirtyRects) > maxDirtyRects {
		s.fullRedraw = true
		s.dirtyRects = s.dirtyRects[:0] // Clear rects since we're doing full redraw
	}
}

// InvalidateAll marks the entire scene as needing redraw.
// This forces a full redraw on the next render pass.
func (s *Scene) InvalidateAll() {
	s.fullRedraw = true
	s.dirtyRects = s.dirtyRects[:0] // Clear individual rects
}

// DirtyRects returns the accumulated dirty rectangles.
// Returns nil if the scene needs a full redraw (check NeedsFullRedraw first).
// The returned slice should not be modified by the caller.
func (s *Scene) DirtyRects() []DirtyRect {
	if s.fullRedraw {
		return nil
	}
	return s.dirtyRects
}

// ClearDirty resets the dirty state after rendering.
// This should be called after each render pass.
func (s *Scene) ClearDirty() {
	s.dirtyRects = s.dirtyRects[:0]
	s.fullRedraw = false
}

// NeedsFullRedraw returns true if the scene should be fully redrawn.
// This is true when InvalidateAll was called or when too many dirty rects
// have accumulated (more than maxDirtyRects).
func (s *Scene) NeedsFullRedraw() bool {
	return s.fullRedraw
}

// HasDirtyRegions returns true if there are any dirty regions to redraw.
// This includes both individual rects and full redraw state.
func (s *Scene) HasDirtyRegions() bool {
	return s.fullRedraw || len(s.dirtyRects) > 0
}

// SetFillColor sets the color for subsequent fill operations.
func (s *Scene) SetFillColor(c color.Color) {
	s.currentFillColor = c
}

// SetStrokeColor sets the color for subsequent stroke operations.
func (s *Scene) SetStrokeColor(c color.Color) {
	s.currentStrokeColor = c
}

// SetStrokeWidth sets the width for subsequent stroke operations.
func (s *Scene) SetStrokeWidth(width float64) {
	s.currentStrokeWidth = width
}

// SetFillRule sets the fill rule for subsequent fill operations.
func (s *Scene) SetFillRule(rule raster.FillRule) {
	s.currentFillRule = rule
}

// MoveTo starts a new subpath at the given point.
func (s *Scene) MoveTo(x, y float64) {
	s.currentPath.verbs = append(s.currentPath.verbs, raster.VerbMoveTo)
	s.currentPath.points = append(s.currentPath.points, float32(x), float32(y))
}

// LineTo draws a line from the current point to the given point.
func (s *Scene) LineTo(x, y float64) {
	s.currentPath.verbs = append(s.currentPath.verbs, raster.VerbLineTo)
	s.currentPath.points = append(s.currentPath.points, float32(x), float32(y))
}

// QuadTo draws a quadratic Bezier curve.
func (s *Scene) QuadTo(cx, cy, x, y float64) {
	s.currentPath.verbs = append(s.currentPath.verbs, raster.VerbQuadTo)
	s.currentPath.points = append(s.currentPath.points,
		float32(cx), float32(cy),
		float32(x), float32(y))
}

// CubicTo draws a cubic Bezier curve.
func (s *Scene) CubicTo(c1x, c1y, c2x, c2y, x, y float64) {
	s.currentPath.verbs = append(s.currentPath.verbs, raster.VerbCubicTo)
	s.currentPath.points = append(s.currentPath.points,
		float32(c1x), float32(c1y),
		float32(c2x), float32(c2y),
		float32(x), float32(y))
}

// ClosePath closes the current subpath.
func (s *Scene) ClosePath() {
	s.currentPath.verbs = append(s.currentPath.verbs, raster.VerbClose)
}

// Rectangle adds a rectangle to the current path.
func (s *Scene) Rectangle(x, y, width, height float64) {
	s.MoveTo(x, y)
	s.LineTo(x+width, y)
	s.LineTo(x+width, y+height)
	s.LineTo(x, y+height)
	s.ClosePath()
}

// Circle adds a circle to the current path using cubic Bezier approximation.
func (s *Scene) Circle(cx, cy, r float64) {
	// Cubic Bezier circle approximation using kappa = 4 * (sqrt(2) - 1) / 3
	const kappa = 0.5522847498307936

	k := r * kappa

	s.MoveTo(cx+r, cy)
	s.CubicTo(cx+r, cy+k, cx+k, cy+r, cx, cy+r)
	s.CubicTo(cx-k, cy+r, cx-r, cy+k, cx-r, cy)
	s.CubicTo(cx-r, cy-k, cx-k, cy-r, cx, cy-r)
	s.CubicTo(cx+k, cy-r, cx+r, cy-k, cx+r, cy)
}

// Fill fills the current path and clears it.
func (s *Scene) Fill() {
	if len(s.currentPath.verbs) == 0 {
		return
	}

	// Snapshot the current path
	path := s.snapshotPath()

	s.commands = append(s.commands, drawCommand{
		op:       opFill,
		path:     path,
		color:    s.currentFillColor,
		fillRule: s.currentFillRule,
	})

	// Clear the current path
	s.currentPath = pathBuilder{}
}

// Stroke strokes the current path and clears it.
func (s *Scene) Stroke() {
	if len(s.currentPath.verbs) == 0 {
		return
	}

	// Snapshot the current path
	path := s.snapshotPath()

	s.commands = append(s.commands, drawCommand{
		op:    opStroke,
		path:  path,
		color: s.currentStrokeColor,
		width: s.currentStrokeWidth,
	})

	// Clear the current path
	s.currentPath = pathBuilder{}
}

// Clear adds a clear operation to fill the entire target.
func (s *Scene) Clear(c color.Color) {
	s.commands = append(s.commands, drawCommand{
		op:    opClear,
		color: c,
	})
}

// snapshotPath creates a copy of the current path.
func (s *Scene) snapshotPath() *pathBuilder {
	if len(s.currentPath.verbs) == 0 {
		return nil
	}

	path := &pathBuilder{
		verbs:  make([]raster.PathVerb, len(s.currentPath.verbs)),
		points: make([]float32, len(s.currentPath.points)),
	}
	copy(path.verbs, s.currentPath.verbs)
	copy(path.points, s.currentPath.points)
	return path
}

// IsEmpty returns true if the scene has no commands.
func (s *Scene) IsEmpty() bool {
	return len(s.commands) == 0
}

// CommandCount returns the number of drawing commands in the scene.
func (s *Scene) CommandCount() int {
	return len(s.commands)
}

// drawCommands returns an iterator over the scene's drawing commands.
// This is used by Renderer implementations within this package.
func (s *Scene) drawCommands() []drawCommand {
	return s.commands
}

// pathBuilder implements raster.PathLike for use with EdgeBuilder.
func (p *pathBuilder) IsEmpty() bool {
	return len(p.verbs) == 0
}

func (p *pathBuilder) Verbs() []raster.PathVerb {
	return p.verbs
}

func (p *pathBuilder) Points() []float32 {
	return p.points
}

// Ensure pathBuilder implements raster.PathLike.
var _ raster.PathLike = (*pathBuilder)(nil)
