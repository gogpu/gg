// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"image/color"
	"testing"

	"github.com/gogpu/gg/raster"
)

func TestNewScene(t *testing.T) {
	scene := NewScene()

	if scene == nil {
		t.Fatal("NewScene() returned nil")
	}
	if !scene.IsEmpty() {
		t.Error("New scene should be empty")
	}
	if scene.CommandCount() != 0 {
		t.Errorf("CommandCount() = %d, want 0", scene.CommandCount())
	}
}

func TestSceneReset(t *testing.T) {
	scene := NewScene()

	// Add some commands
	scene.SetFillColor(color.RGBA{255, 0, 0, 255})
	scene.Circle(100, 100, 50)
	scene.Fill()

	if scene.IsEmpty() {
		t.Error("Scene should not be empty after Fill()")
	}

	// Reset
	scene.Reset()

	if !scene.IsEmpty() {
		t.Error("Scene should be empty after Reset()")
	}
}

func TestSceneClear(t *testing.T) {
	scene := NewScene()

	scene.Clear(color.White)

	if scene.IsEmpty() {
		t.Error("Scene should not be empty after Clear()")
	}
	if scene.CommandCount() != 1 {
		t.Errorf("CommandCount() = %d, want 1", scene.CommandCount())
	}

	cmd := scene.drawCommands()[0]
	if cmd.op != opClear {
		t.Error("Command should be opClear")
	}
}

func TestSceneFill(t *testing.T) {
	scene := NewScene()

	scene.SetFillColor(color.RGBA{255, 0, 0, 255})
	scene.SetFillRule(raster.FillRuleEvenOdd)

	scene.MoveTo(0, 0)
	scene.LineTo(100, 0)
	scene.LineTo(100, 100)
	scene.LineTo(0, 100)
	scene.ClosePath()
	scene.Fill()

	if scene.IsEmpty() {
		t.Error("Scene should not be empty after Fill()")
	}
	if scene.CommandCount() != 1 {
		t.Errorf("CommandCount() = %d, want 1", scene.CommandCount())
	}

	cmd := scene.drawCommands()[0]
	if cmd.op != opFill {
		t.Error("Command should be opFill")
	}
	if cmd.path == nil {
		t.Error("Command path should not be nil")
	}
	if cmd.fillRule != raster.FillRuleEvenOdd {
		t.Errorf("fillRule = %v, want EvenOdd", cmd.fillRule)
	}

	// Verify path has expected verbs
	if len(cmd.path.verbs) != 5 {
		t.Errorf("path.verbs length = %d, want 5", len(cmd.path.verbs))
	}
}

func TestSceneStroke(t *testing.T) {
	scene := NewScene()

	scene.SetStrokeColor(color.RGBA{0, 0, 255, 255})
	scene.SetStrokeWidth(2.5)

	scene.MoveTo(10, 10)
	scene.LineTo(90, 90)
	scene.Stroke()

	if scene.IsEmpty() {
		t.Error("Scene should not be empty after Stroke()")
	}

	cmd := scene.drawCommands()[0]
	if cmd.op != opStroke {
		t.Error("Command should be opStroke")
	}
	if cmd.width != 2.5 {
		t.Errorf("stroke width = %f, want 2.5", cmd.width)
	}
}

func TestSceneRectangle(t *testing.T) {
	scene := NewScene()

	scene.Rectangle(10, 20, 100, 50)
	scene.Fill()

	cmd := scene.drawCommands()[0]
	if cmd.path == nil {
		t.Fatal("Command path should not be nil")
	}

	// Rectangle should have: MoveTo, LineTo, LineTo, LineTo, Close
	if len(cmd.path.verbs) != 5 {
		t.Errorf("Rectangle verbs = %d, want 5", len(cmd.path.verbs))
	}

	expectedVerbs := []raster.PathVerb{
		raster.VerbMoveTo,
		raster.VerbLineTo,
		raster.VerbLineTo,
		raster.VerbLineTo,
		raster.VerbClose,
	}

	for i, expected := range expectedVerbs {
		if cmd.path.verbs[i] != expected {
			t.Errorf("verb[%d] = %v, want %v", i, cmd.path.verbs[i], expected)
		}
	}
}

func TestSceneCircle(t *testing.T) {
	scene := NewScene()

	scene.Circle(100, 100, 50)
	scene.Fill()

	cmd := scene.drawCommands()[0]
	if cmd.path == nil {
		t.Fatal("Command path should not be nil")
	}

	// Circle should have: MoveTo, CubicTo x4
	if len(cmd.path.verbs) != 5 {
		t.Errorf("Circle verbs = %d, want 5", len(cmd.path.verbs))
	}

	if cmd.path.verbs[0] != raster.VerbMoveTo {
		t.Error("Circle should start with MoveTo")
	}

	for i := 1; i < 5; i++ {
		if cmd.path.verbs[i] != raster.VerbCubicTo {
			t.Errorf("verb[%d] = %v, want CubicTo", i, cmd.path.verbs[i])
		}
	}
}

func TestSceneQuadTo(t *testing.T) {
	scene := NewScene()

	scene.MoveTo(0, 0)
	scene.QuadTo(50, 100, 100, 0)
	scene.Fill()

	cmd := scene.drawCommands()[0]
	if cmd.path == nil {
		t.Fatal("Command path should not be nil")
	}

	if len(cmd.path.verbs) != 2 {
		t.Errorf("verbs length = %d, want 2", len(cmd.path.verbs))
	}

	if cmd.path.verbs[1] != raster.VerbQuadTo {
		t.Errorf("verb[1] = %v, want QuadTo", cmd.path.verbs[1])
	}

	// QuadTo should have 4 floats (control + endpoint)
	// After MoveTo's 2 points, we should have 6 total
	if len(cmd.path.points) != 6 {
		t.Errorf("points length = %d, want 6", len(cmd.path.points))
	}
}

func TestSceneCubicTo(t *testing.T) {
	scene := NewScene()

	scene.MoveTo(0, 0)
	scene.CubicTo(25, 100, 75, 100, 100, 0)
	scene.Fill()

	cmd := scene.drawCommands()[0]
	if cmd.path == nil {
		t.Fatal("Command path should not be nil")
	}

	if cmd.path.verbs[1] != raster.VerbCubicTo {
		t.Errorf("verb[1] = %v, want CubicTo", cmd.path.verbs[1])
	}

	// CubicTo should have 6 floats (2 control + endpoint)
	// After MoveTo's 2 points, we should have 8 total
	if len(cmd.path.points) != 8 {
		t.Errorf("points length = %d, want 8", len(cmd.path.points))
	}
}

func TestSceneMultipleCommands(t *testing.T) {
	scene := NewScene()

	// Clear
	scene.Clear(color.White)

	// Red rectangle
	scene.SetFillColor(color.RGBA{255, 0, 0, 255})
	scene.Rectangle(10, 10, 50, 50)
	scene.Fill()

	// Blue circle
	scene.SetFillColor(color.RGBA{0, 0, 255, 255})
	scene.Circle(200, 200, 30)
	scene.Fill()

	// Green stroke
	scene.SetStrokeColor(color.RGBA{0, 255, 0, 255})
	scene.SetStrokeWidth(3.0)
	scene.MoveTo(0, 0)
	scene.LineTo(100, 100)
	scene.Stroke()

	if scene.CommandCount() != 4 {
		t.Errorf("CommandCount() = %d, want 4", scene.CommandCount())
	}

	cmds := scene.drawCommands()

	// Clear
	if cmds[0].op != opClear {
		t.Error("cmd[0] should be opClear")
	}

	// Red rectangle
	if cmds[1].op != opFill {
		t.Error("cmd[1] should be opFill")
	}

	// Blue circle
	if cmds[2].op != opFill {
		t.Error("cmd[2] should be opFill")
	}

	// Green stroke
	if cmds[3].op != opStroke {
		t.Error("cmd[3] should be opStroke")
	}
}

func TestSceneEmptyFill(t *testing.T) {
	scene := NewScene()

	// Fill without any path commands should be ignored
	scene.Fill()

	if !scene.IsEmpty() {
		t.Error("Scene should be empty after Fill() with no path")
	}
}

func TestSceneEmptyStroke(t *testing.T) {
	scene := NewScene()

	// Stroke without any path commands should be ignored
	scene.Stroke()

	if !scene.IsEmpty() {
		t.Error("Scene should be empty after Stroke() with no path")
	}
}

func TestPathBuilderImplementsPathLike(t *testing.T) {
	scene := NewScene()

	scene.MoveTo(0, 0)
	scene.LineTo(100, 0)
	scene.LineTo(100, 100)
	scene.ClosePath()
	scene.Fill()

	cmd := scene.drawCommands()[0]

	// Verify pathBuilder implements raster.PathLike
	var path raster.PathLike = cmd.path

	if path.IsEmpty() {
		t.Error("path should not be empty")
	}
	if len(path.Verbs()) != 4 {
		t.Errorf("Verbs() length = %d, want 4", len(path.Verbs()))
	}
	if len(path.Points()) != 6 { // 3 points * 2 floats
		t.Errorf("Points() length = %d, want 6", len(path.Points()))
	}
}

func TestSceneInvalidate(t *testing.T) {
	scene := NewScene()

	// Initially no dirty regions
	if scene.HasDirtyRegions() {
		t.Error("New scene should have no dirty regions")
	}
	if scene.NeedsFullRedraw() {
		t.Error("New scene should not need full redraw")
	}

	// Add a dirty rect
	scene.Invalidate(DirtyRect{X: 10, Y: 20, Width: 50, Height: 30})

	if !scene.HasDirtyRegions() {
		t.Error("Scene should have dirty regions after Invalidate")
	}
	if scene.NeedsFullRedraw() {
		t.Error("Scene should not need full redraw after single Invalidate")
	}

	rects := scene.DirtyRects()
	if len(rects) != 1 {
		t.Fatalf("DirtyRects() length = %d, want 1", len(rects))
	}
	if rects[0].X != 10 || rects[0].Y != 20 || rects[0].Width != 50 || rects[0].Height != 30 {
		t.Errorf("DirtyRect = %v, want {10, 20, 50, 30}", rects[0])
	}
}

func TestSceneInvalidateInvalidRect(t *testing.T) {
	scene := NewScene()

	// Invalid rects should be ignored
	scene.Invalidate(DirtyRect{X: 10, Y: 20, Width: 0, Height: 30})  // zero width
	scene.Invalidate(DirtyRect{X: 10, Y: 20, Width: 50, Height: 0})  // zero height
	scene.Invalidate(DirtyRect{X: 10, Y: 20, Width: -5, Height: 30}) // negative width
	scene.Invalidate(DirtyRect{X: 10, Y: 20, Width: 50, Height: -5}) // negative height

	if scene.HasDirtyRegions() {
		t.Error("Invalid rects should not be added")
	}
}

func TestSceneInvalidateAll(t *testing.T) {
	scene := NewScene()

	// Add some dirty rects first
	scene.Invalidate(DirtyRect{X: 10, Y: 20, Width: 50, Height: 30})
	scene.Invalidate(DirtyRect{X: 100, Y: 200, Width: 50, Height: 30})

	// Mark for full redraw
	scene.InvalidateAll()

	if !scene.HasDirtyRegions() {
		t.Error("Scene should have dirty regions after InvalidateAll")
	}
	if !scene.NeedsFullRedraw() {
		t.Error("Scene should need full redraw after InvalidateAll")
	}
	if scene.DirtyRects() != nil {
		t.Error("DirtyRects should return nil when full redraw is needed")
	}
}

func TestSceneClearDirty(t *testing.T) {
	scene := NewScene()

	// Add dirty regions
	scene.Invalidate(DirtyRect{X: 10, Y: 20, Width: 50, Height: 30})
	scene.InvalidateAll()

	// Clear
	scene.ClearDirty()

	if scene.HasDirtyRegions() {
		t.Error("Scene should have no dirty regions after ClearDirty")
	}
	if scene.NeedsFullRedraw() {
		t.Error("Scene should not need full redraw after ClearDirty")
	}
}

func TestSceneInvalidateTooManyRects(t *testing.T) {
	scene := NewScene()

	// Add more than maxDirtyRects (16) dirty regions
	for i := 0; i < 20; i++ {
		scene.Invalidate(DirtyRect{
			X:      float64(i * 10),
			Y:      float64(i * 10),
			Width:  50,
			Height: 30,
		})
	}

	// Should switch to full redraw mode
	if !scene.NeedsFullRedraw() {
		t.Error("Scene should need full redraw after exceeding maxDirtyRects")
	}
	if scene.DirtyRects() != nil {
		t.Error("DirtyRects should return nil when full redraw is needed")
	}
}

func TestSceneInvalidateAfterFullRedraw(t *testing.T) {
	scene := NewScene()

	// Mark for full redraw
	scene.InvalidateAll()

	// Additional invalidates should be ignored
	scene.Invalidate(DirtyRect{X: 10, Y: 20, Width: 50, Height: 30})

	// Still in full redraw mode, no individual rects
	if !scene.NeedsFullRedraw() {
		t.Error("Scene should still need full redraw")
	}
	if scene.DirtyRects() != nil {
		t.Error("DirtyRects should return nil")
	}
}

func TestSceneResetClearsDirty(t *testing.T) {
	scene := NewScene()

	// Add dirty regions
	scene.Invalidate(DirtyRect{X: 10, Y: 20, Width: 50, Height: 30})
	scene.InvalidateAll()

	// Reset
	scene.Reset()

	if scene.HasDirtyRegions() {
		t.Error("Scene should have no dirty regions after Reset")
	}
	if scene.NeedsFullRedraw() {
		t.Error("Scene should not need full redraw after Reset")
	}
}
